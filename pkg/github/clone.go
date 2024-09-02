package github

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"time"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-logr/zapr"
	"github.com/jlewi/hydros/pkg/github/ghrepo"
	"github.com/pkg/errors"
	"go.uber.org/zap"
)

// ReposCloner clones a set of repositories
//
// TODO(jeremy): This is currently GitHub specific we should change that
// TODO(jeremy): How do we support public repositories? right now it always uses the ghapp auth.
type ReposCloner struct {
	// List of repositories to clone
	URIs    []string
	Manager *TransportManager
	BaseDir string
}

// Run clones the repository. If the repository has already been cloned then it will fetch the latest changes
// and checkout the specified branch. Any changes are dropped.
func (r *ReposCloner) Run(ctx context.Context) error {
	// loop over the repos and clone them
	for _, uri := range r.URIs {
		// TODO(jeremy): Make the branch configrable
		if err := r.cloneRepo(ctx, uri); err != nil {
			return err
		}
	}
	return nil
}

// GetRepoDir the directory where the repository will be cloned
func (r *ReposCloner) GetRepoDir(uri string) (string, error) {
	u, err := url.Parse(uri)
	if err != nil {
		return "", errors.Wrapf(err, "Could not parse URI %v", uri)
	}
	orgRepo, err := ghrepo.FromURL(u)
	if err != nil {
		return "", errors.Wrapf(err, "Could not parse URI %v", uri)
	}

	org := orgRepo.RepoOwner()
	repo := orgRepo.RepoName()
	fullDir := filepath.Join(r.BaseDir, u.Hostname(), org, repo)
	return fullDir, nil
}

func (r *ReposCloner) cloneRepo(ctx context.Context, uri string) error {
	log := zapr.NewLogger(zap.L())

	u, err := url.Parse(uri)
	if err != nil {
		return errors.Wrapf(err, "Could not parse URI %v", uri)
	}
	orgRepo, err := ghrepo.FromURL(u)
	if err != nil {
		return errors.Wrapf(err, "Could not parse URI %v", uri)
	}

	// sha parameter specifies the commit to checkout
	sha := u.Query().Get("sha")

	// ref parameter specifies the reference to checkout
	// https://github.com/hashicorp/go-getter#protocol-specific-options
	branch := u.Query().Get("ref")

	if branch != "" && sha != "" {
		log.Info("branch and sha are both specified; branch will be ignored", "branch", branch, "sha", sha)
		branch = ""
	}

	if sha == "" && branch == "" {
		// Default to main
		branch = "main"
		log.Info("neither branch nor sha are specified; setting default branch", "branch", branch)
	}

	org := orgRepo.RepoOwner()
	repo := orgRepo.RepoName()

	tr, err := r.Manager.Get(org, repo)
	if err != nil {
		return err
	}

	log = log.WithValues("org", org, "repo", repo)
	// Generate an access token
	url := fmt.Sprintf("https://github.com/%v/%v.git", org, repo)
	var appAuth *AppAuth
	// TODO(jeremy): How should we deal with public repositories?
	if tr != nil {
		appAuth = &AppAuth{
			Tr: tr,
		}
	}

	fullDir, err := r.GetRepoDir(uri)
	if err != nil {
		return err
	}

	log.Info("Clone configured", "url", url, "appAuth", appAuth, "dir", fullDir, "branch", branch)

	opts := &git.CloneOptions{
		URL:      url,
		Auth:     appAuth,
		Progress: os.Stdout,
	}

	gitRepo, err := cloneOrOpen(ctx, fullDir, opts)
	if err != nil {
		return errors.Wrapf(err, "Could not clone or open repository %v", fullDir)
	}

	// N.B. It should generally be ok to hard code the name of the origin because we should be cloning the repository
	// and this is what it would be by default.
	remote := "origin"
	// Do a fetch to make sure the remote is up to date.
	log.Info("Fetching remote", "remote", remote)
	if err := gitRepo.Fetch(&git.FetchOptions{
		RemoteName: remote,
		Auth:       appAuth,
		// TODO(jeremy): Do we need to specify refspec?
		// RefSpecs:   []config.RefSpec{config.RefSpec(fmt.Sprintf("refs/heads/*:refs/remotes/%v/*", h.remote))},
	}); err != nil {
		// Fetch returns an error if its already up to date and we want to ignore that.
		if err.Error() != "already up-to-date" {
			return err
		}
	}

	// config reads .git/config
	// We can use this to determine how the repository is setup to figure out what we need to do
	cfg, err := gitRepo.Config()
	if err != nil {
		return err
	}

	// Set email and name of the author
	// This is equivalent to git config user.email
	// TODO(jeremy): I'm not sure we need to do this. I believe the name and email get specified explicitly in
	// the options to push and don't get inherited from the config automatically.
	log.Info("Updating email and name for commits")
	cfg.User.Email = "hydros@YOURORG.ai"
	cfg.User.Name = "hydros"

	// Need to update the config for the changes to take effect
	if err := gitRepo.Storer.SetConfig(cfg); err != nil {
		return err
	}

	// Check the status and error out if the try is dirty. We might want to add options to controll
	// the behavior in the event the tree is dirty.
	w, err := gitRepo.Worktree()
	if err != nil {
		return err
	}

	status, err := w.Status()
	if err != nil {
		return err
	}

	dropChanges := true
	if !status.IsClean() {
		if dropChanges {
			log.Info("Working tree is dirty but dropChanges is true so changes will be dropped")
		} else {
			return errors.Errorf("Repository is dirty; this blocks branch creation")
		}
	}

	checkoutOptions := &git.CheckoutOptions{
		Force:  dropChanges,
		Create: false,
	}

	if sha != "" {
		// Resolve the short hash to a full hash (SHA-1).
		fullSha, err := gitRepo.ResolveRevision(plumbing.Revision(sha))
		if err != nil {
			log.Error(err, "Failed to resolve revision", "sha", sha)
		}
		checkoutOptions.Hash = *fullSha
	} else {
		checkoutOptions.Branch = plumbing.NewRemoteReferenceName(remote, branch)
	}

	log.Info("Checking out code", "branch", branch, "sha", sha)

	err = w.Checkout(checkoutOptions)

	if err != nil {
		return err
	}

	return nil
}

// cloneOrOpen clones the repository if it hasn't already been cloned. If the repository has already been cloned
// it opens it. If the directory exists but doesn't contain a git repository it will try to delete the directory.
// This is necessary when using temporary directories for the working directory because the files can be periodically
// cleaned up. https://github.com/jlewi/hydros/issues/98
func cloneOrOpen(ctx context.Context, fullDir string, opts *git.CloneOptions) (*git.Repository, error) {
	log := zapr.NewLogger(zap.L())
	// Clone the repository if it hasn't already been cloned.
	if _, err := os.Stat(fullDir); err == nil {
		log.Info("Directory exists; repository will not be cloned", "directory", fullDir)
	} else {
		_, err := git.PlainClone(fullDir, false, opts)
		if err != nil {
			return nil, err
		}
	}

	// Open the repository
	gitRepo, err := git.PlainOpenWithOptions(fullDir, &git.PlainOpenOptions{})
	if err == nil {
		return gitRepo, nil
	}

	log.Error(err, "Could not open repository; it may have been corrupted by automatic garbage collection", "repository", fullDir)
	// Get the current timestamp
	currentTime := time.Now()
	timestamp := currentTime.Format("20060102_150405") // Format: YYYYMMDD_HHMMSS

	// Create the new directory name with the timestamp
	newName := fullDir + "." + timestamp

	log.Info("Renaming directory", "old", fullDir, "new", newName)
	if err := os.Rename(fullDir, newName); err != nil {
		return nil, errors.Wrapf(err, "Renaming directory %v to %v failed. %v doesn't appear to be a valid git repository. Try deleting it so that hydros will reclone it", fullDir, newName, fullDir)
	}

	// Clone the repository
	if _, err := git.PlainClone(fullDir, false, opts); err != nil {
		return nil, errors.Wrapf(err, "Failed to clone repository %v", fullDir)
	}
	return git.PlainOpenWithOptions(fullDir, &git.PlainOpenOptions{})
}
