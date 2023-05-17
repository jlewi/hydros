package github

import (
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/jlewi/hydros/pkg/gitutil"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/config"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"

	"github.com/bradleyfalzon/ghinstallation/v2"
	"github.com/cli/cli/v2/api"
	ghAPI "github.com/cli/go-gh/pkg/api"
	"github.com/go-logr/logr"
	"github.com/go-logr/zapr"
	"github.com/jlewi/hydros/pkg/github/ghrepo"
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
	// N.B. We aren't guaranteed to be using the same http client for Git that other parts of the code base is using
	// Thats not ideal. However, the cli/cli Client package doesn't give us a way to inject an http client.
	client := &http.Client{Transport: args.GhTr}
	h := &RepoHelper{
		transport:  args.GhTr,
		client:     api.NewClientFromHTTP(client),
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
func (h *RepoHelper) CreatePr(prMessage string, labels []string) (*api.PullRequest, error) {
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
			return nil, err
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
		return nil, errors.WithStack(errors.Wrapf(err, "there was an error getting repository information"))
	}
	pr, err := api.CreatePullRequest(h.client, baseRepository, params)
	if err != nil {
		graphErr, ok := err.(*ghAPI.GQLError)

		if !ok {
			h.log.Error(err, "There was a problem creating the PR,")
			return nil, err
		}

		matcher, cErr := regexp.Compile("A pull request already exists.*")
		if cErr != nil {
			log.Error(cErr, "Failed to compile regex; could not check if err is PR exists", "graphErr", graphErr)
			return nil, err
		}
		for _, gErr := range graphErr.Errors {
			if !matcher.MatchString(gErr.Message) {
				h.log.Error(err, "There was a problem creating the PR,")
				return nil, err
			}
			h.log.Info(gErr.Message)

			// Try to fetch and print out the URL of the existing PR.
			existingPR, err := h.PullRequestForBranch()
			if err != nil {
				h.log.Error(err, "Failed to locate existing PR", "forkRef", forkRef, "baseBranch", h.BaseBranch)
				return nil, err
			}

			url := ""
			if existingPR != nil {
				url = existingPR.URL
			}
			h.log.Info("A pull request for the branch already exists", "forkRef", forkRef, "baseBranch", h.BaseBranch, "prUrl", url)

			// TODO(jeremy): This is pretty kludgy. Can we get rid of our copy of pullrequest and just use the CLI's
			// version.
			pr := &api.PullRequest{
				ID:     existingPR.ID,
				URL:    existingPR.URL,
				Number: existingPR.Number,
			}
			return pr, nil
		}
	}
	h.log.Info("Created PR", "url", pr.URL)

	// When a PR is created number isn't populated but we can get it from the URL
	_, number, err := parsePRURL(pr.URL)
	pr.Number = number

	return pr, err
}

// Email returns the value of email used by this repohelper.
func (h *RepoHelper) Email() string {
	return h.email
}

// PullRequestForBranch returns the PR for the given branch if it exists and nil if no PR exists.
// TODO(jeremy): Can we change this to api.PullRequest?
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
	err := h.client.GraphQL(h.baseRepo.RepoHost(), query, variables, &resp)
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
// If dropChanges is true if the working tree is dirty the changes will be ignored.
//
//	if the tree is dirty and dropChanges is false then an error is returned because we can't check out the local
//	branch.
//
// Typically PrepareBranch should be called with dropChanges=true.
//
// If a local branch already exists it will be deleted and the local branch
// will be recreated from the latest baseBranch. This is to ensure the branch is created from the latest code on
// the base branch.
//
// N.B the semantics are based on how hydros and similar automation is expected to work. Each time hydros runs
// hydrate it should run on the head commit of the baseBranch. So if a local/remote branch already exists it
// represents a previous sync and we want to completely override it. When a hydros sync successfully runs it should
// result in a PR. The existence of the PR should be used to block hydros from recreating or otherwise modifying
// the branch until the PR is merged or closed. These semantics are designed to allow humans to interact with
// the PR and potentially edit it before merging.
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
	cfg, err := r.Config()
	if err != nil {
		return err
	}

	// Set email and name of the author
	// This is equivalent to git config user.email
	// TODO(jeremy): I'm not sure we need to do this. I believe the name and email get specified explicitly in
	// the options to push and don't get inherited from the config automatically.
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

	if err != nil && err.Error() != "reference not found" {
		return err
	}

	if err == nil {
		// Branch already exists
		log.Info("Branch already exists; deleting it.", "branch", h.BranchName, "local", branchRef.Hash(), "base", baseRef.Hash())

		if err := r.Storer.RemoveReference(branchRef.Name()); err != nil {
			return errors.Wrapf(err, "Failed to delete existing local branch %v", branchRef.Name())
		}
	}

	checkoutOptions := &git.CheckoutOptions{
		Branch: h.BranchRef(),
		Force:  dropChanges,
		Create: true,
		Hash:   baseRef.Hash(),
	}

	log.Info("Checking out branch", "name", h.BaseBranch, "baseRef", h.RemoteBaseRef())
	err = w.Checkout(checkoutOptions)

	if err != nil {
		return err
	}

	return nil
}

// HasChanges returns true if there are changes to be committed.
func (h *RepoHelper) HasChanges() (bool, error) {
	log := zapr.NewLogger(zap.L())
	log = log.WithValues("org", h.baseRepo.RepoOwner(), "repo", h.baseRepo.RepoName(), "dir", h.fullDir)

	// Open the repository
	r, err := git.PlainOpenWithOptions(h.fullDir, &git.PlainOpenOptions{})
	if err != nil {
		return false, err
	}
	w, err := r.Worktree()
	if err != nil {
		return false, err
	}
	status, err := w.Status()
	if err != nil {
		return false, err
	}
	if status.IsClean() {
		log.Info("No changes to commit")
		return false, nil
	}
	return true, nil
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

	if err := gitutil.AddGitignoreToWorktree(w, h.fullDir); err != nil {
		return errors.Wrapf(err, "Failed to add gitignore patterns")
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

// Dir returns the directory of the repository.
func (h *RepoHelper) Dir() string {
	return h.fullDir
}

// MergePR tries to merge the PR. This means either
// 1. enabling auto merge if a merge queue is required
// 2. merging right away if able
func (h *RepoHelper) MergePR(prNumber int) (PRMergeState, error) {
	client := &http.Client{Transport: h.transport}

	return MergePR(client, h.baseRepo, prNumber)
}

// MergeAndWait merges the PR and waits for it to be merged.
func (h *RepoHelper) MergeAndWait(prNumber int, timeout time.Duration) (PRMergeState, error) {
	done := false
	log := h.log.WithValues("number", prNumber)
	wait := 10 * time.Second
	for endTime := time.Now().Add(timeout); endTime.After(time.Now()) && !done; {
		state := func() PRMergeState {
			pr, err := h.FetchPR(prNumber)
			if err != nil {
				log.Error(err, "Failed to fetch PR; unable to confirm if its been merged")
				return UnknownState
			}

			if pr.State == MergeStateStatusMerged {
				log.Info("PR has been merged", "pr", pr.URL)
				return MergedState
			}
			if pr.IsInMergeQueue {
				log.Info("PR is in merge queue", "pr", pr.URL)
				return EnqueuedState
			}

			log.Info("PR is not in merge queue; attempting to merge", "pr", pr.URL)
			state, err := h.MergePR(pr.Number)
			if err != nil {
				log.Error(err, "Failed to merge pr", "number", pr.Number, "url", pr.URL)
			}
			return state
		}()

		switch state {
		case ClosedState:
			fallthrough
		case MergedState:
			return state, nil
		case EnqueuedState:
			fallthrough
		case UnknownState:
			fallthrough
		case BlockedState:
			fallthrough
		default:
			if endTime.After(time.Now().Add(wait)) {
				time.Sleep(wait)
			}
		}
	}

	return UnknownState, errors.Errorf("Timed out waiting for PR to merge")
}

func (h *RepoHelper) FetchPR(prNumber int) (*api.PullRequest, error) {
	// We need to set the appropriate header in oder to get merge queue status.
	transport := &addAcceptHeaderTransport{T: h.transport}
	client := &http.Client{Transport: transport}
	fields := []string{"id", "number", "state", "title", "lastCommit", "mergeStateStatus", "headRepositoryOwner", "headRefName", "baseRefName", "headRefOid"}
	return fetchPR(client, h.baseRepo, prNumber, fields)
}

func fetchPR(httpClient *http.Client, repo ghrepo.Interface, number int, fields []string) (*api.PullRequest, error) {
	type response struct {
		Repository struct {
			PullRequest api.PullRequest
		}
	}

	query := fmt.Sprintf(`
	query PullRequestByNumber($owner: String!, $repo: String!, $pr_number: Int!) {
		repository(owner: $owner, name: $repo) {
			pullRequest(number: $pr_number) {%s}
		}
	}`, api.PullRequestGraphQL(fields))

	variables := map[string]interface{}{
		"owner":     repo.RepoOwner(),
		"repo":      repo.RepoName(),
		"pr_number": number,
	}

	var resp response
	client := api.NewClientFromHTTP(httpClient)
	err := client.GraphQL(repo.RepoHost(), query, variables, &resp)
	if err != nil {
		return nil, err
	}

	return &resp.Repository.PullRequest, nil
}
