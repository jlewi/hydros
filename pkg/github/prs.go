package github

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/config"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"

	"github.com/bradleyfalzon/ghinstallation/v2"
	"github.com/cli/cli/api"
	"github.com/go-logr/logr"
	"github.com/go-logr/zapr"
	"github.com/kubeflow/testing/go/pkg/ghrepo"
	"github.com/pkg/errors"
	"github.com/shurcooL/githubv4"
	"go.uber.org/zap"
)

// RepoHelper manages the local and remote operations involved in creating a PR.
// RepoHelper is used to create a local working version of a repository where files can be modified.
// Once those files have been modified they can be pushed to the remote repository and a PR  can be created
//
// TODO(jeremy): the code currently assumes the PR is created from a branch in the repository as opposed to creating
// the PR from a fork. We should update the code to support using a fork.
//
// TODO(https://github.com/jlewi/hydros/issues/2): Migrage to github.com/shurcooL/githubv4
// The functions CreatePR and PullRequestForBranch are inspired by the higher level API in GitHub's GoLang CLI.
// A lot of the code is modified from that.
//
// We don't use the CLI. The CLI authors suggested (https://github.com/cli/cli/issues/1327) that it would be better to
// use the API client  libraries directly; github.com/shurcooL/githubv4. The CLI API is providing higher level
// functions ontop of the underlying GraphQL API; originally it seemed silly to redo that rather than just import it
// and reuse it.  https://github.com/cli/cli/blob/4d28c791921621550f19a4c6bcc13778a7525025/api/queries_pr.go.
// However, I think the CLI pulls in some dependencies (BlueMonday?) that we'd like to avoid pulling in if we can
// so that's a good reason to try to migrate of the CLI package.
type RepoHelper struct {
	log        logr.Logger
	transport  *ghinstallation.Transport
	client     *api.Client
	baseRepo   ghrepo.Interface
	fullDir    string
	name       string
	email      string
	remote     string
	BranchName string
	BaseBranch string
}

// RepoHelperArgs is the arguments used to instantiate the object.
type RepoHelperArgs struct {
	// BaseRepo is the repository from which the branch is created. This is also the repository used to create the PR.
	BaseRepo ghrepo.Interface
	// GhTr is the GitHub transport used to authenticate as a GitHub App. If nil a transport will not be used.
	GhTr    *ghinstallation.Transport
	FullDir string
	// Name is the name attached to commits.
	Name string
	// Email is the email attached to commits
	Email string
	// Remote is the name to use for the remote repository.
	// Defaults to origin
	Remote string

	// BranchName is the name for the branch to be created
	BranchName string

	// BaseBranch is the name of the branch to use as the base.
	// This is all the branch to which the PR will be merged
	BaseBranch string
}

// NewGithubRepoHelper creates a helper for a specific repository.
// transport - must be a transport configured with permission to access the referenced repository.
// baseRepo - the repository to access.
func NewGithubRepoHelper(args *RepoHelperArgs) (*RepoHelper, error) {
	log := zapr.NewLogger(zap.L())

	if args.GhTr == nil {
		return nil, fmt.Errorf("GhTr is required")
	}
	if args.BaseRepo == nil {
		return nil, fmt.Errorf("BaseRepo is required")
	}

	if args.Remote == "" {
		log.Info("Remote not set resorting to default", "remote", "origin")
		args.Remote = "origin"
	}

	if args.BranchName == "" {
		return nil, errors.New("BranchName is required")
	}

	if args.BaseBranch == "" {
		return nil, errors.New("BaseBranch is required")
	}

	if args.FullDir == "" {
		tempDir, err := os.MkdirTemp("", "syncer")
		if err != nil {
			return nil, errors.Wrapf(err, "Failed to create temporary dir to host the clone")
		}
		args.FullDir = filepath.Join(tempDir, args.BaseRepo.RepoOwner(), args.BaseRepo.RepoName())
		log.Info("FullDir isn't set using a default value", "dir", args.FullDir)
	}
	if args.Name == "" {
		args.Name = "unidentified-bot"
		log.Info("No name specified; using default", "name", args.Name)
	}

	if args.Email == "" {
		args.Email = "unidentified@nota.real.domain.com"
		log.Info("No email specified; using default", "name", args.Email)
	}

	h := &RepoHelper{
		transport:  args.GhTr,
		client:     api.NewClient(api.ReplaceTripper(args.GhTr)),
		baseRepo:   args.BaseRepo,
		log:        zapr.NewLogger(zap.L()),
		fullDir:    args.FullDir,
		email:      args.Email,
		remote:     args.Remote,
		BranchName: args.BranchName,
		BaseBranch: args.BaseBranch,
	}

	return h, nil
}

// CreatePr creates a pull request
// baseBranch the branch into which your code should be merged.
// forkRef the reference to the fork from which to create the PR
//
//	Forkref will either be OWNER:BRANCH when a different repository is used as the fork.
//	or it will be just BRANCH when merging from a branch in the same Repo as Repo
func (h *RepoHelper) CreatePr(prMessage string, labels []string) error {
	log := h.log.WithValues("Repo", h.baseRepo.RepoName(), "Org", h.baseRepo.RepoOwner())
	lines := strings.SplitN(prMessage, "\n", 2)

	title := ""
	body := ""

	// Forkref will either be OWNER:BRANCH when a different repository is used as the fork.
	//	or it will be just BRANCH when merging from a branch in the same Repo as Repo
	// TODO(jeremy): forkRef should be turned into a struct variable and properly set so as to support
	// creating PRs from forks.
	forkRef := h.BranchName

	if len(lines) >= 1 {
		title = lines[0]
	}

	if len(lines) >= 2 {
		body = lines[1]
	}

	labelIds := []string{}
	if len(labels) > 0 {
		repoLabels, err := api.RepoLabels(h.client, h.baseRepo)
		if err != nil {
			log.Error(err, "Failed to fetch Repo labels")
			return err
		}

		labelNameToID := map[string]string{}
		for _, l := range repoLabels {
			labelNameToID[l.Name] = l.ID
		}

		for _, l := range labels {
			id, ok := labelNameToID[l]
			if !ok {
				log.Error(fmt.Errorf("Missing label %v", l), "Repo is missing label", "label", l)
			}
			labelIds = append(labelIds, id)
		}
	}
	// For more info see:
	// https://developer.github.com/v4/input_object/createpullrequestinput/
	//
	// body and title can't be blank.
	params := map[string]interface{}{
		"title": title,
		"body":  body,
		"draft": false,
		// The name of the branch to merge changes into. This is also the branch we branched from.
		"baseRefName": h.BaseBranch,
		// The name of the reference to merge changes from; typically in the form $user:$branch
		"headRefName": h.BranchName,
	}

	if len(labelIds) > 0 {
		params["labelIds"] = labelIds
	}

	// Query the GitHub API to get actual repository info.
	baseRepository, err := api.GitHubRepo(h.client, h.baseRepo)
	if err != nil {
		return errors.WithStack(errors.Wrapf(err, "there was an error getting repository information"))
	}
	pr, err := api.CreatePullRequest(h.client, baseRepository, params)
	if err != nil {
		graphErr, ok := err.(*api.GraphQLErrorResponse)

		if !ok {
			h.log.Error(err, "There was a problem creating the PR,")
			return err
		}

		matcher, cErr := regexp.Compile("A pull request already exists.*")
		if cErr != nil {
			log.Error(cErr, "Failed to compile regex; could not check if err is PR exists", "graphErr", graphErr)
			return err
		}
		for _, gErr := range graphErr.Errors {
			if !matcher.MatchString(gErr.Message) {
				h.log.Error(err, "There was a problem creating the PR,")
				return err
			}
			h.log.Info(gErr.Message)

			// Try to fetch and print out the URL of the existing PR.
			existingPR, err := h.PullRequestForBranch()
			if err != nil {
				h.log.Error(err, "Failed to locate existing PR", "forkRef", forkRef, "baseBranch", h.BaseBranch)
				return err
			}

			url := ""
			if existingPR != nil {
				url = existingPR.URL
			}
			h.log.Info("A pull request for the branch already exists", "forkRef", forkRef, "baseBranch", h.BaseBranch, "prUrl", url)
		}
	}
	h.log.Info("Created PR", "url", pr.URL)
	return nil
}

// PullRequestForBranch returns the PR for the given branch if it exists and nil if no PR exists.
func (h *RepoHelper) PullRequestForBranch() (*PullRequest, error) {
	baseBranch := h.BaseBranch
	headBranch := h.BranchName
	type response struct {
		Repository struct {
			PullRequests struct {
				ID    githubv4.ID
				Nodes []PullRequest
			}
		}
	}

	query := `
	query($owner: String!, $Repo: String!, $headRefName: String!) {
		repository(owner: $owner, name: $Repo) {
			pullRequests(headRefName: $headRefName, states: OPEN, first: 30) {
				nodes {
					id
					number
					title
					state
					body
					mergeable
					author {
						login
					}
					commits {
						totalCount
					}
					url
					baseRefName
					headRefName	
				}
			}
		}
	}`

	branchWithoutOwner := headBranch
	if idx := strings.Index(headBranch, ":"); idx >= 0 {
		branchWithoutOwner = headBranch[idx+1:]
	}

	variables := map[string]interface{}{
		"owner":       h.baseRepo.RepoOwner(),
		"Repo":        h.baseRepo.RepoName(),
		"headRefName": branchWithoutOwner,
	}

	var resp response
	err := h.client.GraphQL(query, variables, &resp)
	if err != nil {
		return nil, err
	}

	for _, pr := range resp.Repository.PullRequests.Nodes {
		h.log.Info("found", "pr", pr)
		if pr.HeadLabel() == headBranch {
			if baseBranch != "" {
				if pr.BaseRefName != baseBranch {
					continue
				}
			}
			return &pr, nil
		}
	}

	return nil, nil
}

// PrepareBranch prepares a branch. This will do the following
// 1. Clone the repository if it hasn't already been cloned
// 2. Create a branch if one doesn't already exist.
//
// If dropChanges is true if the working tree is direty the changes will be ignored.
func (h *RepoHelper) PrepareBranch(dropChanges bool) error {
	log := zapr.NewLogger(zap.L())
	log = log.WithValues("org", h.baseRepo.RepoOwner(), "repo", h.baseRepo.RepoName(), "dir", h.fullDir)

	// Generate an access token
	url := fmt.Sprintf("https://github.com/%v/%v.git", h.baseRepo.RepoOwner(), h.baseRepo.RepoName())
	var appAuth *AppAuth
	if h.transport != nil {
		appAuth = &AppAuth{
			Tr: h.transport,
		}
	}

	log.Info("URL and Auth configured", "url", url, "appAuth", appAuth)

	// Clone the repository if it hasn't already been cloned.
	err := func() error {
		if _, err := os.Stat(h.fullDir); err == nil {
			log.Info("Directory exists; repository will not be cloned", "directory", h.fullDir)
			return nil
		}

		opts := &git.CloneOptions{
			URL:      url,
			Auth:     appAuth,
			Progress: os.Stdout,
		}

		_, err := git.PlainClone(h.fullDir, false, opts)
		return err
	}()

	if err != nil {
		return err
	}

	// Open the repository
	r, err := git.PlainOpenWithOptions(h.fullDir, &git.PlainOpenOptions{})
	if err != nil {
		return errors.Wrapf(err, "Could not open respoistory at %v; ensure the directory contains a git repo", h.fullDir)
	}

	// Do a fetch to make sure the remote is up to date.
	log.Info("Fetching remote", "remote", h.remote)
	if err := r.Fetch(&git.FetchOptions{
		RemoteName: h.remote,
		Auth:       appAuth,
	}); err != nil {
		// Fetch returns an error if its already up to date and we want to ignore that.
		if err.Error() != "already up-to-date" {
			return err
		}
	}

	// config reads .git/config
	// We can use this to determine how the repository is setup to figure out what we need to do
	cfg, err := r.Config()
	if err != nil {
		return err
	}

	// Set email and name of the author
	// This is equivalent to git config user.email
	log.Info("Updating email and name for commits")
	cfg.User.Email = h.email
	cfg.User.Name = h.name

	// Need to update the config for the changes to take effect
	if err := r.Storer.SetConfig(cfg); err != nil {
		return err
	}

	// Check the status and error out if the try is dirty. We might want to add options to controll
	// the behavior in the event the tree is dirty.
	w, err := r.Worktree()
	if err != nil {
		return err
	}

	status, err := w.Status()
	if err != nil {
		return err
	}

	if !status.IsClean() {
		if dropChanges {
			log.Info("Working tree is dirty but dropChanges is true so changes will be dropped")
		} else {
			return errors.Errorf("Repository is dirty; this blocks branch creation")
		}
	}

	// We need to get a reference to the remote base because that'h the hash we want to create the branch from
	// Do we want to set resolved to true?
	baseRef, err := r.Reference(h.RemoteBaseRef(), false)
	if err != nil {
		return err
	}

	// Try to get a reference to the local branch. We use this to determine whether the branch already exists
	branchRef, err := r.Reference(h.BranchRef(), false)

	checkoutOptions := &git.CheckoutOptions{
		Branch: h.BranchRef(),
		// TODO(jeremy): add an option to set Force to true to drop local changes?
		Force: dropChanges,
	}
	if err != nil {
		if err.Error() != "reference not found" {
			return err
		}
		// Since the branch doesn't exist create it.
		// We specify the Hash to be the latest hash on the remote branch
		checkoutOptions.Create = true
		checkoutOptions.Hash = baseRef.Hash()
	} else {
		log.Info("Got local and base references", "local", branchRef.Hash(), "base", baseRef.Hash())
		if branchRef.Hash() != baseRef.Hash() {
			// TODO(jeremy): Can/should we update the local branch in this case? Rather than just overwriting the
			// branch.
			if dropChanges {
				log.Info("Branch already exists but is not based on the baseref; the changes will be overwritten", "local", branchRef.Hash(), "base", baseRef.Hash())
			} else {
				return errors.Errorf("PrepareBranch failed. The branch %v exists at hash %v but the baseRef %v is at hash %v", h.BranchName, branchRef.Hash(), h.BaseBranch, baseRef.Hash())
			}
		}
		// since the branch already exists we just have to check it out
		checkoutOptions.Create = false
	}

	log.Info("Checking out branch", "name", h.BaseBranch, "baseRef", h.RemoteBaseRef())
	err = w.Checkout(checkoutOptions)

	if err != nil {
		return err
	}

	return nil
}

// CommitAndPush and push commits and pushes all the working changes
//
// NullOp if nothing to commit.
//
// force means the remote branch will be overwritten if it isn't in sync.
func (h *RepoHelper) CommitAndPush(message string, force bool) error {
	log := zapr.NewLogger(zap.L())
	log = log.WithValues("org", h.baseRepo.RepoOwner(), "repo", h.baseRepo.RepoName(), "dir", h.fullDir)
	// Open the repository
	r, err := git.PlainOpenWithOptions(h.fullDir, &git.PlainOpenOptions{})
	if err != nil {
		return err
	}
	w, err := r.Worktree()
	if err != nil {
		return err
	}
	status, err := w.Status()
	if err != nil {
		return err
	}
	if status.IsClean() {
		log.Info("No changes to commit")
		return nil
	}

	if err := w.AddWithOptions(&git.AddOptions{All: true}); err != nil {
		return err
	}

	commit, err := w.Commit(message, &git.CommitOptions{
		Author: &object.Signature{
			Name:  h.name,
			Email: h.email,
			When:  time.Now(),
		},
	})

	if err != nil {
		return err
	}

	// Prints the current HEAD to verify that all worked well.
	obj, err := r.CommitObject(commit)
	if err != nil {
		return err
	}
	log.Info("Commit succeeded", "commit", obj.String())

	// We will name the remote branch the same as the local branch so src and dest are the same
	refSpec := string(h.BranchRef()) + ":" + string(h.BranchRef())

	var appAuth *AppAuth
	if h.transport != nil {
		appAuth = &AppAuth{
			Tr: h.transport,
		}
	}

	log.Info("Pushing", "refspec", refSpec, "appAuth", appAuth)

	if err := r.Push(&git.PushOptions{
		RemoteName: h.remote,
		RefSpecs: []config.RefSpec{
			config.RefSpec(refSpec),
		},
		Auth:  appAuth,
		Force: force,
	}); err != nil {
		return err
	}

	log.Info("Push succeeded")
	return nil
}

// BranchRef returns reference to the branch we created
func (h *RepoHelper) BranchRef() plumbing.ReferenceName {
	return plumbing.ReferenceName(fmt.Sprintf("refs/heads/%v", h.BranchName))
}

// RemoteBaseRef returns the remote base reference.
// Per https://git-scm.com/book/en/v2/Git-Internals-The-Refspec
// This is the local copy of the remote branch.
func (h *RepoHelper) RemoteBaseRef() plumbing.ReferenceName {
	return plumbing.ReferenceName(fmt.Sprintf("refs/remotes/%v/%v", h.remote, h.BaseBranch))
}
