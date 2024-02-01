package gitops

import (
	"context"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/jlewi/hydros/api/v1alpha1"
)

func Test_rewriteRepos(t *testing.T) {
	type testCase struct {
		name     string
		manifest *v1alpha1.ManifestSync
		expected v1alpha1.GitHubRepo
		mappings []v1alpha1.RepoMapping
	}

	cases := []testCase{
		{
			name: "basic",
			manifest: &v1alpha1.ManifestSync{
				Spec: v1alpha1.ManifestSyncSpec{
					SourceRepo: v1alpha1.GitHubRepo{
						Org:    "jlewi",
						Repo:   "hydros",
						Branch: "main",
					},
				},
			},
			mappings: []v1alpha1.RepoMapping{
				{
					Input:  "https://github.com/jlewi/hydros.git?ref=main",
					Output: "https://github.com/jlewi/hydros.git?ref=jlewi/cicd",
				},
			},
			expected: v1alpha1.GitHubRepo{
				Org:    "jlewi",
				Repo:   "hydros",
				Branch: "jlewi/cicd",
			},
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			err := rewriteRepos(context.Background(), c.manifest, c.mappings)
			if err != nil {
				t.Fatalf("Error rewriting repos: %v", err)
			}

			if d := cmp.Diff(c.expected, c.manifest.Spec.SourceRepo); d != "" {
				t.Errorf("Unexpected diff:\n%v", d)
			}
		})
	}
}
