package util

import (
	"testing"

	kustomize "sigs.k8s.io/kustomize/api/types"

	"github.com/google/go-cmp/cmp"
)

func Test_ParseImage(t *testing.T) {
	type testCase struct {
		In       string
		Expected *DockerImageRef
	}

	cases := []testCase{
		{
			In: "12345.dkr.ecr.us-west-2.amazonaws.com/some-repo/some-image:latest",
			Expected: &DockerImageRef{
				Registry: "12345.dkr.ecr.us-west-2.amazonaws.com",
				Repo:     "some-repo/some-image",
				Tag:      "latest",
			},
		},
		{
			In: "12345.dkr.ecr.us-west-2.amazonaws.com/some-repo/some-image:df182f2@sha256:3fd974b119b0874074db59dd83cb721b82122140fe5df98fda9dd8b2acf84b1c",
			Expected: &DockerImageRef{
				Registry: "12345.dkr.ecr.us-west-2.amazonaws.com",
				Repo:     "some-repo/some-image",
				Tag:      "df182f2",
				Sha:      "sha256:3fd974b119b0874074db59dd83cb721b82122140fe5df98fda9dd8b2acf84b1c",
			},
		},
		{
			In: "12345.dkr.ecr.us-west-2.amazonaws.com/some-repo/some-image@sha256:3fd974b119b0874074db59dd83cb721b82122140fe5df98fda9dd8b2acf84b1c",
			Expected: &DockerImageRef{
				Registry: "12345.dkr.ecr.us-west-2.amazonaws.com",
				Repo:     "some-repo/some-image",
				Tag:      "",
				Sha:      "sha256:3fd974b119b0874074db59dd83cb721b82122140fe5df98fda9dd8b2acf84b1c",
			},
		},
		{
			In: "docker://us-west1-docker.pkg.dev/acme-public/images/someimage:latest",
			Expected: &DockerImageRef{
				Registry: "us-west1-docker.pkg.dev",
				Repo:     "acme-public/images/someimage",
				Tag:      "latest",
			},
		},
	}

	for _, c := range cases {
		a, err := ParseImageURL(c.In)
		if err != nil {
			t.Errorf("%v", err)
			continue
		}

		d := cmp.Diff(c.Expected, a)

		if d != "" {
			t.Errorf("Expected didn't match expected;\n%v", d)
		}
	}
}

func Test_ImageToUrl(t *testing.T) {
	type testCase struct {
		In       *DockerImageRef
		Expected string
	}

	cases := []testCase{
		{
			In: &DockerImageRef{
				Registry: "12345.dkr.ecr.us-west-2.amazonaws.com",
				Repo:     "some-repo/some-image",
				Tag:      "latest",
			},
			Expected: "12345.dkr.ecr.us-west-2.amazonaws.com/some-repo/some-image:latest",
		},
		{
			In: &DockerImageRef{
				Registry: "12345.dkr.ecr.us-west-2.amazonaws.com",
				Repo:     "some-repo/some-image",
				Tag:      "df182f2",
				Sha:      "sha256:3fd974b119b0874074db59dd83cb721b82122140fe5df98fda9dd8b2acf84b1c",
			},
			Expected: "12345.dkr.ecr.us-west-2.amazonaws.com/some-repo/some-image:df182f2@sha256:3fd974b119b0874074db59dd83cb721b82122140fe5df98fda9dd8b2acf84b1c",
		},
	}

	for _, c := range cases {
		actual := c.In.ToURL()

		if actual != c.Expected {
			t.Errorf("DockerImageRef.ToURL: Got %v; Want %v", actual, c.Expected)
		}
	}
}

func Test_GetAwsRegistryId(t *testing.T) {
	type testCase struct {
		In       DockerImageRef
		Expected string
	}

	cases := []testCase{
		{
			In: DockerImageRef{
				Registry: "12345.dkr.ecr.us-west-2.amazonaws.com",
				Repo:     "some-repo/some-image",
				Tag:      "latest",
			},
			Expected: "12345",
		},
		{
			In: DockerImageRef{
				Registry: "gcr.io/kubeflow",
				Repo:     "some-repo/some-image",
				Tag:      "df182f2",
				Sha:      "sha256:3fd974b119b0874074db59dd83cb721b82122140fe5df98fda9dd8b2acf84b1c",
			},
			Expected: "",
		},
	}

	for _, c := range cases {
		actual := c.In.GetAwsRegistryID()

		if actual != c.Expected {
			t.Errorf("GetAwsRegistryID: Got %v; Want %v", actual, c.Expected)
		}
	}
}

func Test_SetKustomizeImage(t *testing.T) {
	type testCase struct {
		images   []kustomize.Image
		name     string
		new      DockerImageRef
		expected []kustomize.Image
	}

	testCases := []testCase{
		{
			images: []kustomize.Image{
				{
					Name:   "repo/image",
					NewTag: "sometag",
					Digest: "somedigest",
				},
				{
					Name:   "repo/other/image",
					NewTag: "sometag",
					Digest: "somedigest",
				},
			},
			name: "repo/image",
			new: DockerImageRef{
				Registry: "someregistry",
				Repo:     "newrepo/newimage",
				Tag:      "1234abcd",
				Sha:      "sha256@11222",
			},
			expected: []kustomize.Image{
				{
					Name:    "repo/image",
					NewName: "someregistry/newrepo/newimage:1234abcd",
					Digest:  "sha256@11222",
				},
				{
					Name:   "repo/other/image",
					NewTag: "sometag",
					Digest: "somedigest",
				},
			},
		},
		{
			images: []kustomize.Image{
				{
					Name:   "repo/image",
					NewTag: "sometag",
					Digest: "somedigest",
				},
			},
			name: "repo/image",
			new: DockerImageRef{
				Registry: "someregistry",
				Repo:     "newrepo/newimage",
				Tag:      "1234abcd",
			},
			expected: []kustomize.Image{
				{
					Name:    "repo/image",
					NewName: "someregistry/newrepo/newimage",
					NewTag:  "1234abcd",
				},
			},
		},
	}

	for _, c := range testCases {
		k := &kustomize.Kustomization{
			Images: c.images,
		}

		err := SetKustomizeImage(k, c.name, c.new)
		if err != nil {
			t.Errorf("SetKustomizeImage failed; error %v", err)
			continue
		}

		if d := cmp.Diff(c.expected, k.Images); d != "" {
			t.Errorf("Unexpected diff;\n%v", d)
		}
	}
}
