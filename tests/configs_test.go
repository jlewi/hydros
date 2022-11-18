package tests

import (
	"io/ioutil"
	"os"
	"os/exec"
	"path"
	"sort"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/jlewi/hydros/api/v1alpha1"

	"github.com/jlewi/hydros/pkg/util"
	"gopkg.in/yaml.v3"

	kustomize "sigs.k8s.io/kustomize/api/types"
)

var testDirs = []string{"./configs/dev", "./configs/prod"}

// Test_kustomizations verifies that the kustomizations include all the yaml files
// TODO(jeremy): This is an example of a test to validate actual hydros configurations.
// We might want to rethink it. The test won't pass on public repositories because there are no configs.
// However, we might want to provide a test as an example for people looking to use it.
func Test_kustomizations(t *testing.T) {
	for _, d := range testDirs {
		kustomizeFile := path.Join(d, "kustomization.yaml")
		f, err := os.Open(kustomizeFile)
		if err != nil {
			if os.IsNotExist(err) {
				t.Logf("File %v doesn't exist; test skipped", kustomizeFile)
				continue
			}
			t.Errorf("Failed to open kustomization: %v", kustomizeFile)
		}
		decoder := yaml.NewDecoder(f)

		k := &kustomize.Kustomization{}

		if err := decoder.Decode(k); err != nil {
			t.Errorf("Failed to decode kustomization from %v; error: %v", kustomizeFile, err)
			continue
		}

		if !sort.StringsAreSorted(k.ConfigMapGenerator[0].FileSources) {
			actual := util.PrettyString(k.ConfigMapGenerator[0])
			sort.Strings(k.ConfigMapGenerator[0].FileSources)
			want := util.PrettyString(k.ConfigMapGenerator[0])
			d := cmp.Diff(want, actual)
			t.Errorf("Files should be listed in sorted order in the configMapGenerator in %v;\nGot: %v,\nWant: %v\nDiff: %v", kustomizeFile, actual, want, d)
		}

		files, err := ioutil.ReadDir(d)
		if err != nil {
			t.Errorf("Failed to readDir: %v", d)
			continue
		}

		expected := []string{}

		excluded := map[string]bool{
			"kustomization.yaml":      true,
			"hydros_for_testing.yaml": true,
		}
		for _, f := range files {
			if _, ok := excluded[f.Name()]; ok {
				continue
			}
			expected = append(expected, f.Name())
		}

		actual := map[string]bool{}

		for _, c := range k.ConfigMapGenerator {
			if c.Name != "sync-manifests" {
				continue
			}
			for _, n := range c.FileSources {
				actual[n] = true
			}
		}

		missing := []string{}

		for _, n := range expected {
			if _, ok := actual[n]; !ok {
				missing = append(missing, n)
			}
		}

		if len(missing) > 0 {
			t.Errorf("Kustomization %v is missing hydros configs: %v", kustomizeFile, missing)
		}
	}
}

// Test_build verifies that kustomize build doesn't throw an error
func Test_build(t *testing.T) {
	log := util.SetupLogger("info", true)
	h := util.ExecHelper{Log: log}
	for _, d := range testDirs {
		if _, err := os.Stat(d); os.IsNotExist(err) {
			t.Logf("Directory %v doesn't exist; test skipped", d)
			continue
		}
		cmd := exec.Command("kustomize", "build", ".")
		cmd.Dir = d

		if err := h.Run(cmd); err != nil {
			t.Errorf("kustomize build failed for %v; error %v", d, err)
		}
	}
}

// Test_ValidManifestSync validates the manifestsyncs
func Test_ValidManifestSync(t *testing.T) {
	for _, d := range testDirs {
		files, err := ioutil.ReadDir(d)
		if err != nil {
			if os.IsNotExist(err) {
				t.Logf("Directory %v doesn't exist; test skipped", d)
				continue
			}
			t.Errorf("Failed to readDir: %v", d)
			continue
		}

		excluded := map[string]bool{
			"kustomization.yaml":      true,
			"hydros_for_testing.yaml": true,
		}

		for _, f := range files {
			if _, ok := excluded[f.Name()]; ok {
				continue
			}

			fullPath := path.Join(d, f.Name())
			rNodes, err := util.ReadYaml(fullPath)
			if err != nil {
				t.Errorf("Failed to read file %v; error %v", fullPath, err)
				continue
			}

			for _, n := range rNodes {
				m, err := n.GetMeta()
				if err != nil {
					t.Errorf("Failed to get metadata for resource in file %v", fullPath)
					continue
				}
				// TODO(jeremy): Should we be worried about the kind being misspecified
				if m.Kind != v1alpha1.ManifestSyncKind {
					t.Errorf("Unexpected Kind in file %v; want %v; got %v", fullPath, v1alpha1.ManifestSyncKind, m.Kind)
					continue
				}

				manifestSync := &v1alpha1.ManifestSync{}
				err = n.Document().Decode(manifestSync)
				if err != nil {
					t.Errorf("Failed to decode ManifestSync in file %v; error %v", fullPath, err)
					continue
				}

				if err := manifestSync.IsValid(); err != nil {
					t.Errorf("ManifestSync in file %v; isn't valid %v", fullPath, err)
					continue
				}

				if manifestSync.Spec.MatchAnnotations != nil {
					t.Errorf("ManifestSync in file %v; isn't valid. Setting spec.matchAnnotations is no longer allowed in new configurations. Use spec.selector", fullPath)
					continue
				}
			}
		}
	}
}
