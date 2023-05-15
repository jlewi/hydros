package gitops

import (
	"github.com/go-logr/zapr"
	"github.com/jlewi/hydros/api/v1alpha1"
	"github.com/jlewi/hydros/pkg/github"
	"github.com/jlewi/hydros/pkg/github/ghrepo"
	"github.com/jlewi/hydros/pkg/util"
	"github.com/pkg/errors"
	"go.uber.org/zap"
	"os"
	"path/filepath"
	"time"
)

// Renderer handles in place modification of YAML files.
// It is intended to run a bunch of KRM functions in place and then check the modifications back into the repository
type Renderer struct {
	// ForkRepo is the repo into which the changes will be pushed and the PR created from
	ForkRepo *v1alpha1.GitHubRepo `yaml:"forkRepo,omitempty"`

	// DestRepo is the repo into which a PR will be created to merge hydrated
	// manifests from the ForkRepo
	DestRepo *v1alpha1.GitHubRepo `yaml:"destRepo,omitempty"`

	workDir string

	// repoHelper helps with creating PRs
	repoHelper *github.RepoHelper
	transports *github.TransportManager

	// commit is the commit to checkout
	commit string
}

func (r *Renderer) init() error {
	// Create a repo helper for the destRepo
	tr, err := r.transports.Get(r.DestRepo.Org, r.DestRepo.Repo)
	if err != nil {
		return errors.Wrapf(err, "Failed to get transport for repo %v/%v; Is the GitHub app installed in that repo?", r.DestRepo.Org, r.DestRepo.Repo)
	}

	args := &github.RepoHelperArgs{
		BaseRepo:   ghrepo.New(r.DestRepo.Org, r.DestRepo.Repo),
		GhTr:       tr,
		FullDir:    r.cloneDir(),
		Name:       "hydros",
		Email:      "hydros@yourdomain.com",
		Remote:     "origin",
		BranchName: r.ForkRepo.Branch,
		BaseBranch: r.DestRepo.Branch,
	}

	repoHelper, err := github.NewGithubRepoHelper(args)
	if err != nil {
		return err
	}

	r.repoHelper = repoHelper

	return nil
}
func (r *Renderer) Run() error {
	log := zapr.NewLogger(zap.L())

	if _, err := os.Stat(r.workDir); os.IsNotExist(err) {
		log.V(util.Debug).Info("Creating work directory.", "directory", r.workDir)

		err = os.MkdirAll(r.workDir, util.FilePermUserGroup)

		if err != nil {
			return errors.Wrapf(err, "Failed to create dir: %v", r.workDir)
		}
	}

	if err := r.init(); err != nil {
		return err
	}

	// Check if there is a PR already pending from the branch and if there is don't do a sync.

	// If the fork is in a different repo then the head reference is OWNER:BRANCH
	// If we are creating the PR from a different branch in the same repo as where we are creating
	// the PR then we just use BRANCH as the ref
	headBranchRef := r.ForkRepo.Branch

	if r.ForkRepo.Org != r.DestRepo.Org {
		headBranchRef = r.ForkRepo.Org + ":" + headBranchRef
	}
	existingPR, err := r.repoHelper.PullRequestForBranch()
	if err != nil {
		log.Error(err, "Failed to check if there is an existing PR", "headBranchRef", headBranchRef)
		return err
	}

	if existingPR != nil {
		log.Info("PR Already Exists; attempting to merge it.", "pr", existingPR.URL)
		state, err := r.repoHelper.MergeAndWait(existingPR.Number, 3*time.Minute)
		if err != nil {
			log.Error(err, "Failed to Merge existing PR unable to continue with sync", "number", existingPR.Number, "pr", existingPR.URL)
			return err
		}

		if state != github.ClosedState && state != github.MergedState {
			log.Info("PR hasn't been merged; unable to continue with the sync", "number", existingPR.Number, "pr", existingPR.URL, "state", state)
			return errors.Errorf("Existing PR %v is blocking sync", existingPR.URL)
		}
	}

	// Checkout the repository.
	if err := r.repoHelper.PrepareBranch(true); err != nil {
		return err
	}
	//if err := r.checkout(); err != nil {
	//	return err
	//}
	return nil
}

func (r *Renderer) cloneDir() string {
	return filepath.Join(r.workDir, "source")
}

//func (r *Renderer) checkout() error {
//	log := zapr.NewLogger(zap.L())
//	//args := b.Args
//	//fullDir := b.fullDir
//	//url := "https://github.com/jlewi/roboweb.git"
//	//remote := "origin"
//	//
//	//secret, err := readSecret(args.Secret)
//	//if err != nil {
//	//	return errors.Wrapf(err, "Could not read file: %v", args.Secret)
//	//}
//
//	repo := ghrepo.New(r.DestRepo.Org, r.DestRepo.Repo)
//	url := ghrepo.GenerateRepoURL(repo, "https")
//
//	fullDir := r.cloneDir()
//	tr, err := r.transports.Get(r.SourceRepo.Org, r.SourceRepo.Repo)
//	if err != nil {
//		return err
//	}
//
//	appAuth := &github.AppAuth{
//		Tr: tr,
//	}
//	// Clone the repository if it hasn't already been cloned.
//	err = func() error {
//		if _, err := os.Stat(fullDir); err == nil {
//			log.Info("Directory exists; repository will not be cloned", "directory", fullDir)
//			return nil
//		}
//
//		opts := &git.CloneOptions{
//			URL:      url,
//			Auth:     appAuth,
//			Progress: os.Stdout,
//		}
//
//		_, err := git.PlainClone(fullDir, false, opts)
//		return err
//	}()
//
//	if err != nil {
//		return err
//	}
//
//	// Open the repository
//	gitRepo, err := git.PlainOpenWithOptions(fullDir, &git.PlainOpenOptions{})
//	if err != nil {
//		return errors.Wrapf(err, "Could not open respoistory at %v; ensure the directory contains a git repo", fullDir)
//	}
//
//	// Do a fetch to make sure the remote is up to date.
//	remote := "origin"
//	log.Info("Fetching remote", "remote", remote)
//	if err := gitRepo.Fetch(&git.FetchOptions{
//		RemoteName: remote,
//		Auth:       appAuth,
//	}); err != nil {
//		// Fetch returns an error if its already up to date and we want to ignore that.
//		if err.Error() != "already up-to-date" {
//			return err
//		}
//	}
//
//	// If commit is specified check it out
//	if r.commit != "" {
//		hash, err := gitRepo.ResolveRevision(plumbing.Revision(r.commit))
//
//		if err != nil {
//			return errors.Wrapf(err, "Could not resolve commit %v", r.commit)
//		}
//
//		log.Info("Checking out commit", "commit", hash.String())
//		w, err := gitRepo.Worktree()
//		if err != nil {
//			return err
//		}
//		err = w.Checkout(&git.CheckoutOptions{
//			Hash:  *hash,
//			Force: true,
//		})
//		if err != nil {
//			return errors.Wrapf(err, "Failed to checkout commit %s", r.commit)
//		}
//	}
//
//	// TODO(jeremy): We should be checking out the source branch.
//
//	// Get the current commit
//	ref, err := gitRepo.Head()
//	if err != nil {
//		return err
//	}
//
//	// The short tag will be used to tag the artifacts
//	//b.commitTag = ref.Hash().String()[0:7]
//
//	log.Info("Current commit", "commit", ref.Hash().String())
//	return nil
//}

// getRepos is a helper function that returns all the different repos involved in a map to make it easier
// to loop over them.
func (r *Renderer) getRepos() map[string]*v1alpha1.GitHubRepo {
	return map[string]*v1alpha1.GitHubRepo{
		//sourceKey: r.SourceRepo,
		destKey: r.DestRepo,
		forkKey: r.ForkRepo,
	}
}
