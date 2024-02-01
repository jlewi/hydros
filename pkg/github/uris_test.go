package github

import (
	"github.com/google/go-cmp/cmp"
	"github.com/jlewi/hydros/api/v1alpha1"
	"net/url"
	"testing"
)

func Test_GitHubRepoToURL(t *testing.T) {
	type testCase struct {
		name string
		repo v1alpha1.GitHubRepo

		// Compare the actual URL to one parsed from a string
		expected string
	}

	testCases := []testCase{
		{
			name: "main",
			repo: v1alpha1.GitHubRepo{
				Org:    "jlewi",
				Repo:   "hydros",
				Branch: "jlewi/cicd",
			},
			expected: "https://github.com/jlewi/hydros.git?ref=jlewi/cicd",
		},
	}

	for _, c := range testCases {
		t.Run(c.name, func(t *testing.T) {
			actual := GitHubRepoToURI(c.repo)

			expectedU, err := url.Parse(c.expected)
			if err != nil {
				t.Fatalf("Error parsing expected URL %v", err)
			}

			if d := cmp.Diff(expectedU, &actual); d != "" {
				t.Errorf("Unexpected diff:\n%v", d)
			}
		})
	}
}
