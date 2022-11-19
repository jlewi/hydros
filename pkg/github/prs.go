package github

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/bradleyfalzon/ghinstallation/v2"
	"github.com/cli/cli/api"
	"github.com/go-logr/logr"
	"github.com/go-logr/zapr"
	"github.com/kubeflow/testing/go/pkg/ghrepo"
	"github.com/pkg/errors"
	"github.com/shurcooL/githubv4"
	"go.uber.org/zap"
)

// RepoHelper provides a higher level API ontop of the GraphQL API.
//
// TODO(https://github.com/jlewi/hydros/issues/2): Migrage to github.com/shurcooL/githubv4
// It is inspired by the higher level API in GitHub's GoLang CLI. A lot of the code is modified from that.
//
// We don't use the CLI The CLI authors suggested (https://github.com/cli/cli/issues/1327) that it would be better to use the API client
// libraries directly; github.com/shurcooL/githubv4. The CLI API is providing higher level functions ontop of the
// underlying GraphQL API; it seems silly to redo that rather than just import it and reuse it.
// https://github.com/cli/cli/blob/4d28c791921621550f19a4c6bcc13778a7525025/api/queries_pr.go
type RepoHelper struct {
	log       logr.Logger
	transport *ghinstallation.Transport
	client    *api.Client
	baseRepo  ghrepo.Interface
}

// NewGithubRepoHelper creates a helper for a specific repository.
// transport - must be a transport configured with permission to access the referenced repository.
// baseRepo - the repository to access.
func NewGithubRepoHelper(transport *ghinstallation.Transport, baseRepo ghrepo.Interface, opts ...Option) (*RepoHelper, error) {
	if transport == nil {
		return nil, fmt.Errorf("transport is required")
	}
	if baseRepo == nil {
		return nil, fmt.Errorf("baseRepo is required")
	}

	h := &RepoHelper{
		transport: transport,
		client:    api.NewClient(api.ReplaceTripper(transport)),
		baseRepo:  baseRepo,
		log:       zapr.NewLogger(zap.L()),
	}

	for _, o := range opts {
		o(h)
	}

	return h, nil
}

// Option creates an option for RepoHelper.
type Option func(h *RepoHelper)

// WithLogger creates an option to use the supplied logger.
func WithLogger(log logr.Logger) Option {
	return func(h *RepoHelper) {
		h.log = log
	}
}

// CreatePr creates a pull request
// baseBranch the branch into which your code should be merged.
// forkRef the reference to the fork from which to create the PR
//
//	Forkref will either be OWNER:BRANCH when a different repository is used as the fork.
//	or it will be just BRANCH when merging from a branch in the same Repo as Repo
func (h *RepoHelper) CreatePr(baseBranch, forkRef, prMessage string, labels []string) error {
	log := h.log.WithValues("Repo", h.baseRepo.RepoName(), "Org", h.baseRepo.RepoOwner())
	lines := strings.SplitN(prMessage, "\n", 2)

	title := ""
	body := ""

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
		// The name of the branch to merge changes into.
		"baseRefName": baseBranch,
		// The name of the reference to merge changes from; typically in the form $user:$branch
		"headRefName": forkRef,
	}

	if len(labelIds) > 0 {
		params["labelIds"] = labelIds
	}
	// Forkref will either be OWNER:BRANCH when a different repository is used as the fork.
	// Or it will be just BRANCH when merging from a branch in the same Repo as Repo
	pieces := strings.Split(forkRef, ":")

	forkOwner := h.baseRepo.RepoOwner()
	forkBranch := ""
	if len(pieces) == 1 {
		forkBranch = pieces[0]
	} else {
		forkOwner = pieces[0]
		forkBranch = pieces[1]
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

			// If the fork is in a different Repo then the head reference is OWNER:BRANCH
			// If we are creating the PR from a different branch in the same Repo as where we are creating
			// the PR then we just use BRANCH as the ref
			headBranchRef := forkRef

			if forkOwner == h.baseRepo.RepoOwner() {
				headBranchRef = forkBranch
			}

			existingPR, err := h.PullRequestForBranch(baseBranch, headBranchRef)
			if err != nil {
				h.log.Error(err, "Failed to locate existing PR", "forkRef", forkRef, "baseBranch", baseBranch)
				return err
			}

			url := ""
			if existingPR != nil {
				url = existingPR.URL
			}
			h.log.Info("A pull request for the branch already exists", "forkRef", forkRef, "baseBranch", baseBranch, "prUrl", url)
		}
	}
	h.log.Info("Created PR", "url", pr.URL)
	return nil
}

// PullRequestForBranch returns the PR for the given branch if it exists and nil if no PR exists.
func (h *RepoHelper) PullRequestForBranch(baseBranch, headBranch string) (*PullRequest, error) {
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
