package v1alpha1

import (
	"testing"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"gopkg.in/yaml.v3"

	"github.com/google/go-cmp/cmp"
)

// Test_ParseYaml verifies we can properly deseralize the YAML representation.
// It basically verifies yaml tags are properly added.
func Test_ParseYaml(t *testing.T) {
	type testCase struct {
		input    string
		expected *ManifestSync
	}

	expectedTime := metav1.Date(2024, 1, 30, 16, 36, 10, 0, time.UTC)
	testCases := []testCase{
		{
			input: `apiVersion: hydros.sailplane.ai/v1alpha1
kind: ManifestSync
metadata:
  name: test
status:
  pausedUntil: "2024-01-30T16:36:10Z"
`,
			expected: &ManifestSync{
				APIVersion: "hydros.sailplane.ai/v1alpha1",
				Kind:       "ManifestSync",
				Metadata: Metadata{
					Name: "test",
				},
				Status: ManifestSyncStatus{
					PausedUntil: &expectedTime,
				},
			},
		},
		{
			input: `apiVersion: hydros.sailplane.ai/v1alpha1
kind: ManifestSync
metadata:
  name: mlp-helm-dev
spec:
  sourcePath: configs
  destPath: k8s/some-dir/subpath
  matchAnnotations:
    "hydros.primer.ai/env": "dev-helm"
  imageRegistries:
    - 12345.dkr.ecr.us-west-2.amazonaws.com
  imageTagsToPin:
    - tags:
      - latest
      strategy: sourceCommit
      imageRepoMatch:
        type: exclude
        repos:
          - some/image/repo
    - tags:
      - company-latest 
      - latest
      strategy: mutableTag
      imageRepoMatch:
        type: include
        repos:
          - some/image/repo
`,
			expected: &ManifestSync{
				APIVersion: "hydros.sailplane.ai/v1alpha1",
				Kind:       "ManifestSync",
				Metadata: Metadata{
					Name: "mlp-helm-dev",
				},
				Spec: ManifestSyncSpec{
					SourcePath: "configs",
					DestPath:   "k8s/some-dir/subpath",
					MatchAnnotations: map[string]string{
						"hydros.primer.ai/env": "dev-helm",
					},
					ImageRegistries: []string{"12345.dkr.ecr.us-west-2.amazonaws.com"},
					ImageTagsToPin: []ImageTagToPin{
						{
							Tags:     []string{"latest"},
							Strategy: SourceCommitStrategy,
							ImageRepoMatch: &ImageRepoMatch{
								Repos: []string{"some/image/repo"},
								Type:  ExcludeRepo,
							},
						},
						{
							Tags:     []string{"company-latest", "latest"},
							Strategy: MutableTagStrategy,
							ImageRepoMatch: &ImageRepoMatch{
								Repos: []string{"some/image/repo"},
								Type:  IncludeRepo,
							},
						},
					},
				},
			},
		},
	}

	for _, c := range testCases {
		actual := &ManifestSync{}
		if err := yaml.Unmarshal([]byte(c.input), actual); err != nil {
			t.Errorf("Failed to unmarshal yaml; error %v", err)
			continue
		}

		if d := cmp.Diff(c.expected, actual); d != "" {
			t.Errorf("Unexpected diff;\n%v", d)
		}
	}
}
