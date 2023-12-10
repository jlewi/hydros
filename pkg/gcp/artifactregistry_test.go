package gcp

import (
	"fmt"
	"github.com/google/go-cmp/cmp"
	"github.com/jlewi/hydros/api/v1alpha1"
	"github.com/jlewi/hydros/pkg/util"
	"os"
	"testing"
)

func Test_DockerImageRefToArtifactImage(t *testing.T) {
	type testCase struct {
		ref      util.DockerImageRef
		expected ArtifactImage
	}

	testCases := []testCase{
		{
			ref: util.DockerImageRef{
				Registry: "us-west1-docker.pkg.dev",
				Repo:     "acme/images/hercules",
				Tag:      "latest",
				Sha:      "171ac42",
			},
			expected: ArtifactImage{
				Project:    "acme",
				Location:   "us-west1",
				Repository: "images",
				Package:    "hercules",
				Tag:        "latest",
				Sha:        "171ac42",
			},
		},
		{
			ref: util.DockerImageRef{
				Registry: "us-west1-docker.pkg.dev",
				Repo:     "acme/images/vscode/devbox",
				Tag:      "prod",
			},
			expected: ArtifactImage{
				Project:    "acme",
				Location:   "us-west1",
				Repository: "images",
				Package:    "vscode/devbox",
				Tag:        "prod",
			},
		},
	}

	for i, tc := range testCases {
		t.Run(fmt.Sprintf("%d", i), func(t *testing.T) {
			actual, err := FromImageRef(tc.ref)
			if err != nil {
				t.Fatalf("Error converting %v; %v", tc.ref, err)
			}
			if d := cmp.Diff(tc.expected, actual); d != "" {
				t.Fatalf("Unexpected diff:\n%s", d)
			}
		})
	}
}

func Test_ResolveImageToSHA(t *testing.T) {
	if os.Getenv("GITHUB_ACTIONS") != "" {
		t.Skipf("Test_ResolveImageToSHA is a manual test that is skipped in CICD")
	}

	r := &ImageResolver{}

	ref := util.DockerImageRef{
		Registry: "us-west1-docker.pkg.dev",
		//Repo:     "dev-sailplane/images/vscode/webserver",
		Repo: "dev-sailplane/images/hercules",
		Tag:  "latest",
	}
	strategy := v1alpha1.MutableTagStrategy
	sha, err := r.ResolveImageToSha(ref, strategy)

	if err != nil {
		t.Fatalf("Error resolving image; %+v", err)
	}
	if sha.Sha == "" {
		t.Fatalf("SHA is empty")
	}

}
