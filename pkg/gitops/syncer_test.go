package gitops

import (
	"context"
	"fmt"
	"testing"
	"time"

	meta "k8s.io/apimachinery/pkg/apis/meta/v1"
	kustomize "sigs.k8s.io/kustomize/api/types"

	kustomize2 "github.com/jlewi/hydros/pkg/kustomize"

	"github.com/go-logr/zapr"
	"github.com/jlewi/hydros/api/v1alpha1"
	"github.com/jlewi/hydros/pkg/util"
	"go.uber.org/zap"

	"github.com/google/go-cmp/cmp"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
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

func Test_matchesAnnotations(t *testing.T) {

	type testCase struct {
		m        *v1alpha1.ManifestSync
		expected metav1.Time
	}

	expectedTime := metav1.Date(2024, 1, 30, 16, 36, 10, 0, time.UTC)

	jsonTime, err := expectedTime.MarshalJSON()
	if err != nil {
		t.Errorf("Failed to marshal time: %v", err)
	}

	cases := []testCase{
		{
			m: &v1alpha1.ManifestSync{
				Metadata: v1alpha1.Metadata{
					Annotations: map[string]string{
						v1alpha1.PauseAnnotation: string(jsonTime),
					},
				},
			},
			expected: expectedTime,
		},
	}

	for i, c := range cases {
		t.Run(fmt.Sprintf("Test case %d", i), func(t *testing.T) {
			if err := setPausedUntil(c.m); err != nil {
				t.Errorf("setPausedUntil failed; error %v", err)
				return
			}

			actual := c.m.Status.PausedUntil
			if !c.expected.Equal(actual) {
				t.Errorf("Test case %d: Got %v; want %v", i, actual, c.expected)
			}
		})
	}
}

func Test_IsPaused(t *testing.T) {
	type testCase struct {
		name       string
		m          *v1alpha1.ManifestSync
		lastStatus *v1alpha1.ManifestSyncStatus
		now        time.Time
		expected   bool
	}

	fakeNow := time.Date(2024, 1, 30, 16, 36, 10, 0, time.UTC)

	cases := []testCase{
		{
			name:       "Not paused",
			m:          &v1alpha1.ManifestSync{},
			lastStatus: &v1alpha1.ManifestSyncStatus{},
			now:        fakeNow,
			expected:   false,
		},
		{
			name: "Paused",
			m:    &v1alpha1.ManifestSync{},
			lastStatus: &v1alpha1.ManifestSyncStatus{
				PausedUntil: &metav1.Time{Time: fakeNow.Add(1 * time.Hour)},
			},
			now:      fakeNow,
			expected: true,
		},
		{
			name: "Takeover-overrides-paused",
			m: &v1alpha1.ManifestSync{
				Metadata: v1alpha1.Metadata{
					Annotations: map[string]string{v1alpha1.TakeoverAnnotation: "true"},
				},
			},
			lastStatus: &v1alpha1.ManifestSyncStatus{
				PausedUntil: &metav1.Time{Time: fakeNow.Add(1 * time.Hour)},
			},
			now:      fakeNow,
			expected: false,
		},
		{
			name: "Expired-Pause",
			m:    &v1alpha1.ManifestSync{},
			lastStatus: &v1alpha1.ManifestSyncStatus{
				PausedUntil: &metav1.Time{Time: fakeNow.Add(-1 * time.Hour)},
			},
			now:      fakeNow,
			expected: false,
		},
	}

	for _, c := range cases {
		t.Run(fmt.Sprintf(c.name), func(t *testing.T) {
			actual := isPaused(context.Background(), *c.m, *c.lastStatus, c.now)
			if actual != c.expected {
				t.Errorf("Test case %v: Got %v; want %v", c.name, actual, c.expected)
			}
		})
	}
}
