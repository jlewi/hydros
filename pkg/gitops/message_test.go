package gitops

import (
	"testing"

	"github.com/jlewi/hydros/api/v1alpha1"
	"github.com/jlewi/hydros/pkg/util"
)

func Test_BuildPrMessage(t *testing.T) {
	type testCase struct {
		manifest      *v1alpha1.ManifestSync
		changedImages []util.DockerImageRef
		expected      string
	}

	testManifest := &v1alpha1.ManifestSync{
		Spec: v1alpha1.ManifestSyncSpec{
			SourceRepo: v1alpha1.GitHubRepo{
				Org:    "PrimerAI",
				Repo:   "some-git-repo",
				Branch: "master",
			},
			DestRepo: v1alpha1.GitHubRepo{
				Org:    "PrimerAI",
				Repo:   "hydrated-repo",
				Branch: "env/dev",
			},
		},
		Status: v1alpha1.ManifestSyncStatus{
			SourceURL:    "https://github.com/PrimerAI/some-git-repo/tree/bf51fd1",
			SourceCommit: "bf51fd1",
		},
	}

	testCases := []testCase{
		{
			manifest: testManifest,
			changedImages: []util.DockerImageRef{
				{
					Registry: "12345",
					Repo:     "some-repo/some-image",
					Tag:      "latest",
					Sha:      "9876",
				},
			},
			expected: `[Auto] Hydrate env/dev with PrimerAI/some-git-repo@bf51fd1; 1 images changed
Update hydrated manifests to [PrimerAI/some-git-repo@bf51fd1](https://github.com/PrimerAI/some-git-repo/tree/bf51fd1)
Source: [PrimerAI/some-git-repo@bf51fd1](https://github.com/PrimerAI/some-git-repo/tree/bf51fd1)
Source Branch: master
Changed ImageList:
* 12345/some-repo/some-image:latest@9876`,
		},
		{
			manifest:      testManifest,
			changedImages: []util.DockerImageRef{},
			expected: `[Auto] Hydrate env/dev with PrimerAI/some-git-repo@bf51fd1; 0 images changed
Update hydrated manifests to [PrimerAI/some-git-repo@bf51fd1](https://github.com/PrimerAI/some-git-repo/tree/bf51fd1)
Source: [PrimerAI/some-git-repo@bf51fd1](https://github.com/PrimerAI/some-git-repo/tree/bf51fd1)
Source Branch: master
Changed ImageList: None`,
		},
	}

	for _, c := range testCases {
		actual := buildPrMessage(c.manifest, c.changedImages)

		if actual != c.expected {
			t.Errorf("Got\n%v;\nwant\n%v", actual, c.expected)
		}
	}
}
