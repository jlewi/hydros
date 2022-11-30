//go:build integration

package github

import (
	"net/http"
	"testing"

	"github.com/cli/cli/v2/api"
	"github.com/jlewi/hydros/pkg/util"
)

// This is an integration test. It will try to merge the PR specified. This is useful for manual development.
// The PR is hardcoded so once the PR is successfully merged you would need to create a new one and then update the
// test.
// To run the full lifecycle of creating a PR run prs_test.go
func Test_merge_pr(t *testing.T) {
	util.SetupLogger("info", true)
	prURL := "https://github.com/jlewi/hydros-hydrated/pull/17"
	repo, number, err := parsePRURL(prURL)
	if err != nil {
		t.Fatalf("Failed to parse URL %v; error %v", prURL, err)
	}

	pr := &api.PullRequest{

		URL:    prURL,
		Number: number,
	}

	manager, err := getTransportManager()
	if err != nil {
		t.Fatalf("Failed to get github transport manager; error %v", err)
	}

	tr, err := manager.Get(repo.RepoOwner(), repo.RepoName())
	if err != nil {
		t.Fatalf("Failed to get github transport manager; error %v", err)
	}

	client := &http.Client{Transport: tr}

	if err := MergePR(client, repo, pr.Number); err != nil {
		t.Fatalf("Failed to merge the pr; error %v", err)
	}
}
