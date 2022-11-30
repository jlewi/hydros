package github

// Code to merge PRs.
// It is based on GitHub's CLI code.
// https://github.com/cli/cli/blob/trunk/pkg/cmd/pr/merge/merge.go

import (
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"strconv"

	"github.com/cli/cli/v2/api"
	"github.com/go-logr/logr"
	"github.com/go-logr/zapr"
	"github.com/jlewi/hydros/pkg/github/ghrepo"
	"github.com/pkg/errors"
	"github.com/shurcooL/githubv4"
	"go.uber.org/zap"
)

const (
	MergeStateStatusBehind   = "BEHIND"
	MergeStateStatusBlocked  = "BLOCKED"
	MergeStateStatusClosed   = "CLOSED"
	MergeStateStatusClean    = "CLEAN"
	MergeStateStatusDirty    = "DIRTY"
	MergeStateStatusHasHooks = "HAS_HOOKS"
	MergeStateStatusMerged   = "MERGED"
	MergeStateStatusUnstable = "UNSTABLE"
)

// MergeOptions are the options used to construct a context.
type MergeOptions struct {
	HttpClient *http.Client
	// The number for the PR
	PRNumber int
	Repo     ghrepo.Interface
}

// ErrAlreadyInMergeQueue indicates that the pull request is already in a merge queue
var ErrAlreadyInMergeQueue = errors.New("already in merge queue")

// prMerger contains state and dependencies to merge a pull request.
//
// It is oppinionated about how merges should be done
// i) If a PR can't be merged e.g. because of status checks then it will enable autoMerge so it will be merged as soon
//
//	as possible
//
// ii) It uses squash method to do the merge to preserve linear history.
type prMerger struct {
	pr         *api.PullRequest
	HttpClient *http.Client
	Repo       ghrepo.Interface
	log        logr.Logger
}

// Check if this pull request is in a merge queue
func (m *prMerger) inMergeQueue() error {
	log := m.log
	// if the pull request is in a merge queue no further action is possible
	if m.pr.IsInMergeQueue {
		log.Info("Pull request already in merge queue")
		return ErrAlreadyInMergeQueue
	}

	return nil
}

// Merge the pull request.
func (m *prMerger) merge() error {
	log := m.log
	pr := m.pr

	if pr.State == MergeStateStatusClosed {
		log.Info("PR can't be merged it has been closed")
		return errors.Errorf("Can't merge PR %v it has been closed", pr.URL)
	}
	if pr.State == MergeStateStatusMerged {
		log.Info("PR has already been merged")
		return nil
	}
	if err := m.inMergeQueue(); err != nil {
		log.Info("PR is already in merge queue")
		return nil
	}

	if reason, blocked := blockedReason(m.pr.MergeStateStatus); blocked {
		log.Info("PR merging is blocked", "reason", reason)
		return errors.Errorf("PR merging is blocked; MergeStateStatus: %v reason: %v", m.pr.MergeStateStatus, reason)
	}

	payload := mergePayload{
		repo:          m.Repo,
		pullRequestID: m.pr.ID,
		// N.B. We are oppionated and use squash merge to give linear history.
		method: githubv4.PullRequestMergeMethodSquash,
	}

	// We need to set payload.auto which controls whether an
	// https://docs.github.com/en/graphql/reference/mutations#enablepullrequestautomerge
	// or a https://docs.github.com/en/graphql/reference/mutations#mergepullrequest is issued
	if m.pr.IsMergeQueueEnabled {
		// If a MergeQueue is enabled then we need to add it to one.
		log.Info("MergeQueue enabled so PR will be added to merge queue")
		payload.auto = true
	} else {
		if isImmediatelyMergeable(m.pr.MergeStateStatus) {
			// It is an error to try to enable auto merge if the PR is ready to be merged.
			log.Info("PR is immediately mergeable")
			payload.auto = false
		} else {
			log.Info("PR auto-merge will be enabled and the PR will be merged when ready; this will fail if auto-merge is not allowed for the branch.")
			payload.auto = true
		}
	}

	err := mergePullRequest(m.HttpClient, payload)
	if err != nil {
		return err
	}

	if payload.auto {
		log.Info("Pull request was added to merge queue and will be automatically merged when all requirements are met")
	} else {
		log.Info("pull request was merged", "title", m.pr.Title)
	}

	return nil
}

type addAcceptHeaderTransport struct {
	T http.RoundTripper
}

func (adt *addAcceptHeaderTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	// Per https://docs.github.com/en/graphql/overview/schema-previews#merge-info-preview
	// we need to enable previw mode to get mergeStateStatus
	req.Header.Add("Accept", "application/vnd.github.merge-info-preview+json")
	return adt.T.RoundTrip(req)
}

// newPRMerger creates a new prMerger.
// This will locate the PR and get its current status.
func newPRMerger(client *http.Client, repo ghrepo.Interface, number int) (*prMerger, error) {
	client.Transport = &addAcceptHeaderTransport{T: client.Transport}

	// N.B github/cli/cli was also fetching the fields "isInMergeQueue", "isMergeQueueEnabled" but when I tried
	// I was getting an error those fields don't exist. I think that might be a preview feature and access to those
	// fields might be restricted.
	fields := []string{"id", "number", "state", "title", "lastCommit", "mergeStateStatus", "headRepositoryOwner", "headRefName", "baseRefName", "headRefOid"}
	pr, err := fetchPR(client, repo, number, fields)
	if err != nil {
		return nil, err
	}

	log := zapr.NewLogger(zap.L()).WithValues("prNumber", pr.Number)
	return &prMerger{
		Repo:       repo,
		HttpClient: client,
		pr:         pr,
		log:        log,
	}, nil
}

// MergePR merges a PR.
// client - http client to use to talk to github
// repo - the repo that owns the PR
// number - the PR number to merge
func MergePR(client *http.Client, repo ghrepo.Interface, number int) error {
	m, err := newPRMerger(client, repo, number)

	if err != nil {
		return err
	}
	if err := m.merge(); err != nil {
		return err
	}

	return nil
}

// blockedReason translates various MergeStateStatus GraphQL values into human-readable reason
// The bool indicates whether merging is blocked
func blockedReason(status string) (string, bool) {
	switch status {
	case MergeStateStatusBlocked:
		return "the base branch policy prohibits the merge", true
	case MergeStateStatusBehind:
		return "the head branch is not up to date with the base branch", true
	case MergeStateStatusDirty:
		return "the merge commit cannot be cleanly created", true
	default:
		return "", false
	}
}

func isImmediatelyMergeable(status string) bool {
	switch status {
	case MergeStateStatusClean, MergeStateStatusHasHooks, MergeStateStatusUnstable:
		return true
	default:
		return false
	}
}

type mergePayload struct {
	repo            ghrepo.Interface
	pullRequestID   string
	method          githubv4.PullRequestMergeMethod
	auto            bool
	commitSubject   string
	commitBody      string
	setCommitBody   bool
	expectedHeadOid string
	authorEmail     string
}

// TODO: drop after githubv4 gets updated
type EnablePullRequestAutoMergeInput struct {
	githubv4.MergePullRequestInput
}

// mergePullRequest is a helper function to actually merge the payload.
// N.B. This function supports all the different merge methods because the code was inherited from GitHub's cli
// so why not? But the higher APIs that call it don't support that.
//
// This will either issue an https://docs.github.com/en/graphql/reference/mutations#enablepullrequestautomerge
// or a https://docs.github.com/en/graphql/reference/mutations#mergepullrequest depending on the value of auto.
func mergePullRequest(client *http.Client, payload mergePayload) error {
	input := githubv4.MergePullRequestInput{
		PullRequestID: githubv4.ID(payload.pullRequestID),
	}

	input.MergeMethod = &payload.method
	if payload.authorEmail != "" {
		authorEmail := githubv4.String(payload.authorEmail)
		input.AuthorEmail = &authorEmail
	}
	if payload.commitSubject != "" {
		commitHeadline := githubv4.String(payload.commitSubject)
		input.CommitHeadline = &commitHeadline
	}
	if payload.setCommitBody {
		commitBody := githubv4.String(payload.commitBody)
		input.CommitBody = &commitBody
	}

	// expectedHeadOid is the expected git commit (object id) on the branch being merged. If it doesn't
	// match then no commit is performed.
	// https://docs.github.com/en/graphql/reference/input-objects
	if payload.expectedHeadOid != "" {
		expectedHeadOid := githubv4.GitObjectID(payload.expectedHeadOid)
		input.ExpectedHeadOid = &expectedHeadOid
	}

	variables := map[string]interface{}{
		"input": input,
	}

	gql := api.NewClientFromHTTP(client)

	if payload.auto {
		var mutation struct {
			EnablePullRequestAutoMerge struct {
				ClientMutationId string
			} `graphql:"enablePullRequestAutoMerge(input: $input)"`
		}
		variables["input"] = EnablePullRequestAutoMergeInput{input}
		return gql.Mutate(payload.repo.RepoHost(), "PullRequestAutoMerge", &mutation, variables)
	}

	var mutation struct {
		MergePullRequest struct {
			ClientMutationId string
		} `graphql:"mergePullRequest(input: $input)"`
	}
	return gql.Mutate(payload.repo.RepoHost(), "PullRequestMerge", &mutation, variables)
}

var pullURLRE = regexp.MustCompile(`^/([^/]+)/([^/]+)/pull/(\d+)`)

func parsePRURL(prURL string) (ghrepo.Interface, int, error) {
	if prURL == "" {
		return nil, 0, fmt.Errorf("invalid URL: %q", prURL)
	}

	u, err := url.Parse(prURL)
	if err != nil {
		return nil, 0, err
	}

	if u.Scheme != "https" && u.Scheme != "http" {
		return nil, 0, fmt.Errorf("invalid scheme: %s", u.Scheme)
	}

	m := pullURLRE.FindStringSubmatch(u.Path)
	if m == nil {
		return nil, 0, fmt.Errorf("not a pull request URL: %s", prURL)
	}

	repo := ghrepo.NewWithHost(m[1], m[2], u.Hostname())
	prNumber, _ := strconv.Atoi(m[3])
	return repo, prNumber, nil
}
