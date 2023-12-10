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
				Package:    "vscode%2Fdevbox",
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
		Repo:     "dev-sailplane/images/vscode/webserver",
		Tag:      "78eb2ca7-1690-4711-a97e-188d27866635",
	}
	expectedSha := "sha256:f2709b8a04f7ee03c7a1b5ce014e480b568661d7383dfbd9578ffca531c9184a"
	strategy := v1alpha1.MutableTagStrategy
	sha, err := r.ResolveImageToSha(ref, strategy)

	if err != nil {
		t.Fatalf("Error resolving image; %+v", err)
	}
	if sha.Sha != expectedSha {
		t.Fatalf("SHA is incorrect; got %s; want %s", sha.Sha, expectedSha)
	}

}
