package gitops

import (
	"testing"

	meta "k8s.io/apimachinery/pkg/apis/meta/v1"
	kustomize "sigs.k8s.io/kustomize/api/types"

	kustomize2 "github.com/jlewi/hydros/pkg/kustomize"

	"github.com/go-logr/zapr"
	"github.com/jlewi/hydros/api/v1alpha1"
	"github.com/jlewi/hydros/pkg/util"
	"go.uber.org/zap"

	"github.com/google/go-cmp/cmp"
)

func Test_generateTargetPath(t *testing.T) {
	type testCase struct {
		sourcePath    string
		kustomization string
		expected      kustomize2.TargetPath
	}

	testCases := []testCase{
		{
			sourcePath:    "/tmp/manifests",
			kustomization: "/tmp/manifests/backends/somebackends/dev/kustomization.yaml",
			expected: kustomize2.TargetPath{
				Dir:         "backends/somebackends",
				OverlayName: "dev",
			},
		},
		{
			sourcePath:    "/tmp/hydros/testrepo/configs",
			kustomization: "/tmp/hydros/testrepo/configs/fns/labels.yaml",
			expected: kustomize2.TargetPath{
				Dir:         "",
				OverlayName: "fns",
			},
		},
		{
			sourcePath:    "/tmp/hydros/testrepo/configs",
			kustomization: "/tmp/hydros/testrepo/configs/fns/a/b/c/labels.yaml",
			expected: kustomize2.TargetPath{
				Dir:         "fns/a/b",
				OverlayName: "c",
			},
		},
		{
			// This case corresponds to the user not creating any overlays.
			sourcePath:    "/tmp/hydros/testrepo/",
			kustomization: "/tmp/hydros/testrepo/configs/kustomization.yaml",
			expected: kustomize2.TargetPath{
				Dir:         "configs",
				OverlayName: "",
			},
		},
	}

	for _, c := range testCases {
		actual, err := kustomize2.GenerateTargetPath(c.sourcePath, c.kustomization)
		if err != nil {
			t.Errorf("generateTargetPath generated error: %v", err)
			continue
		}

		d := cmp.Diff(c.expected, actual)

		if d != "" {
			t.Errorf("generateTargetPath actual didn't match expected:\n%v", d)
		}
	}
}

func Test_DidImagesChange(t *testing.T) {
	type testCase struct {
		lastSync []v1alpha1.PinnedImage
		current  map[string]string
		expected []string
	}

	testCases := []testCase{
		{
			lastSync: []v1alpha1.PinnedImage{
				{
					Image:    "some-repo/some-image:latest",
					NewImage: "some-repo/some-image:1234",
				},
			},
			current: map[string]string{
				"some-repo/some-image:latest": "some-repo/some-image:1234",
			},
			expected: []string{},
		},
		{
			lastSync: []v1alpha1.PinnedImage{
				{
					Image:    "some-repo/some-image:latest",
					NewImage: "some-repo/some-image:1234",
				},
			},
			current: map[string]string{
				"some-repo/some-image:latest": "some-repo/some-image:5678",
			},
			expected: []string{"some-repo/some-image:5678"},
		},
		// New URI
		{
			lastSync: []v1alpha1.PinnedImage{
				{
					Image:    "some-repo/some-image:latest",
					NewImage: "some-repo/some-image:1234",
				},
			},
			current: map[string]string{
				"some-repo/some-image:latest": "some-repo/some-image:1234",
				"some-repo/sidecar:latest":    "some-repo/sidecar:1234",
			},
			expected: []string{"some-repo/sidecar:1234"},
		},
	}

	s := &Syncer{
		log: zapr.NewLogger(zap.L()),
	}
	for _, c := range testCases {
		pinned := map[util.DockerImageRef]util.DockerImageRef{}

		for k, v := range c.current {
			key, err := util.ParseImageURL(k)
			if err != nil {
				t.Errorf("Failed to parse image %v", err)
				continue
			}

			value, err := util.ParseImageURL(v)
			if err != nil {
				t.Errorf("Failed to parse image %v", err)
				continue
			}

			pinned[*key] = *value
		}

		actual := s.didImagesChange(c.lastSync, pinned)

		expected := []util.DockerImageRef{}

		for _, u := range c.expected {
			image, err := util.ParseImageURL(u)
			if err != nil {
				t.Errorf("Failed to parse image %v", err)
				continue
			}
			expected = append(expected, *image)
		}

		d := cmp.Diff(expected, actual)

		if d != "" {
			t.Errorf("Expected didn't match actual: %v", d)
		}
	}
}

func Test_GetPinStrategy(t *testing.T) {
	type testCase struct {
		image    util.DockerImageRef
		expected v1alpha1.Strategy
	}

	m := &v1alpha1.ManifestSync{
		Spec: v1alpha1.ManifestSyncSpec{
			ImageTagsToPin: []v1alpha1.ImageTagToPin{
				{
					Tags:     []string{"latest"},
					Strategy: v1alpha1.SourceCommitStrategy,
					ImageRepoMatch: &v1alpha1.ImageRepoMatch{
						Repos: []string{"some/image/repo"},
						Type:  v1alpha1.ExcludeRepo,
					},
				},
				{
					Tags:     []string{"latest"},
					Strategy: v1alpha1.MutableTagStrategy,
					ImageRepoMatch: &v1alpha1.ImageRepoMatch{
						Repos: []string{"some/image/repo"},
						Type:  v1alpha1.IncludeRepo,
					},
				},
			},
		},
	}

	testCases := []testCase{
		{
			image: util.DockerImageRef{
				Registry: "92345",
				Repo:     "source-repo/image",
				Tag:      "latest",
				Sha:      "12345",
			},
			expected: v1alpha1.SourceCommitStrategy,
		},
		{
			image: util.DockerImageRef{
				Registry: "92345",
				Repo:     "some/image/repo",
				Tag:      "latest",
				Sha:      "12345",
			},
			expected: v1alpha1.MutableTagStrategy,
		},
	}

	s := &Syncer{
		log:      zapr.NewLogger(zap.L()),
		manifest: m,
	}
	for _, c := range testCases {
		actual := s.getPinStrategy(c.image)

		if actual != c.expected {
			t.Errorf("URI: %v; Got %v; want %v;", c.image, actual, c.expected)
		}
	}
}

func Test_matches(t *testing.T) {
	type testCase struct {
		input    *kustomize.Kustomization
		selector *meta.LabelSelector
		expected bool
	}

	testCases := []testCase{
		{
			input: &kustomize.Kustomization{},
			selector: &meta.LabelSelector{
				MatchLabels: map[string]string{
					"env": "prod",
				},
			},
			expected: false,
		},
		{
			input: &kustomize.Kustomization{
				MetaData: &kustomize.ObjectMeta{
					Labels: map[string]string{
						"env": "prod",
					},
				},
			},
			selector: &meta.LabelSelector{
				MatchLabels: map[string]string{
					"env": "prod",
				},
			},
			expected: true,
		},
	}

	for _, c := range testCases {
		actual, err := matches(c.input, c.selector)
		if err != nil {
			t.Errorf("Matches failed; error %v", err)
			continue
		}
		if c.expected != actual {
			t.Errorf("Got %v; want %v", actual, c.expected)
		}
	}
}
