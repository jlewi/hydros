package kustomize

import (
	"io/ioutil"
	"os"
	"path"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"

	"sigs.k8s.io/kustomize/kyaml/kio/filters"

	"sigs.k8s.io/kustomize/kyaml/yaml"

	"github.com/PrimerAI/hydros-public/api/v1alpha1"
	"github.com/PrimerAI/hydros-public/pkg/kustomize/fns/configmap"
	"github.com/PrimerAI/hydros-public/pkg/kustomize/fns/envs"

	"sigs.k8s.io/kustomize/kyaml/kio"

	"github.com/PrimerAI/hydros-public/pkg/util"
	"github.com/google/go-cmp/cmp"
	"github.com/otiai10/copy"
	apps "k8s.io/api/apps/v1"
)

func Test_runOnDir(t *testing.T) {
	log := util.SetupLogger("info", true)

	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("Error getting current directory; error %v", err)
	}

	testDir := path.Join(cwd, "test_data")
	sourceDir := path.Join(testDir, "source")

	// Create a temporary directory because the function will modify the directory.
	outDir, err := ioutil.TempDir("", "runOnDir")
	if err != nil {
		t.Errorf("Failed to create temporary directory; %v", err)
		return
	}

	err = copy.Copy(sourceDir, outDir)

	if err != nil {
		t.Errorf("Failed to copy %v to %v; error %v", sourceDir, outDir, err)
		return
	}

	dis := Dispatcher{
		Log: log,
	}

	log.Info("Processing dir", "dir", outDir)
	functionPaths := []string{path.Join(testDir, "functions")}
	err = dis.RunOnDir(outDir, functionPaths)
	if err != nil {
		t.Errorf("RunOnDir failed; error %v", err)
		return
	}

	// names of expected files
	names := []string{"deploy1.yaml", "deploy2.yaml"}

	var dep1 apps.Deployment
	var dep2 apps.Deployment

	for _, n := range names {
		p := path.Join(outDir, n)
		actualB, err := ioutil.ReadFile(p)
		if err != nil {
			t.Errorf("Failed to read file: %v; error %v", p, err)
			continue
		}
		err = yaml.Unmarshal(actualB, &dep1)
		if err != nil {
			t.Errorf("Failed to unmarshall: %v; error %v", actualB, err)
		}

		ePath := path.Join(testDir, "expected", n)
		expectedB, err := ioutil.ReadFile(ePath)
		if err != nil {
			t.Errorf("Failed to read file: %v; error %v", ePath, err)
			continue
		}
		err = yaml.Unmarshal(expectedB, &dep2)
		if err != nil {
			t.Errorf("Failed to unmarshall: %v; error %v", expectedB, err)
		}

		if !assert.Equal(t, dep2, dep1) {
			t.FailNow()
		}

	}
}

func Test_loadFilters(t *testing.T) {
	log := util.SetupLogger("info", true)

	d := Dispatcher{
		Log: log,
	}
	type testCase struct {
		name        string
		functions   []string
		expectedFns []kio.Filter
	}

	expectedPodFn := envs.PodEnvsFunction{
		Kind:       envs.Kind,
		APIVersion: "v1alpha1",
		Spec: envs.Spec{
			Remove: []string{"DD_AGENT_HOST"},
		},
	}

	expectedBothPodFn := &envs.PodEnvsFunction{
		Kind:       envs.Kind,
		APIVersion: "v1alpha1",
		Metadata: v1alpha1.Metadata{
			Annotations: map[string]string{
				configmap.ConfigMapAnnotation: configmap.ConfigMapBoth,
			},
		},
		Spec: envs.Spec{
			Remove: []string{"DD_AGENT_HOST"},
		},
	}

	expectedDefaultMergeFilter := &filters.MergeFilter{}
	expectedDefaultFileSetterFilter := &filters.FileSetter{FilenamePattern: filepath.Join("config", "%n.yaml")}
	expectedDefaultFormatFilter := &filters.FormatFilter{}

	cases := []testCase{
		{
			name: "basic",
			functions: []string{
				`apiVersion: v1alpha1
kind: PodEnvs
spec:
  remove:
    - DD_AGENT_HOST
`,
			},
			expectedFns: []kio.Filter{&expectedPodFn, expectedDefaultMergeFilter, expectedDefaultFileSetterFilter, expectedDefaultFormatFilter},
		},
		{
			name: "apply-to-configmap",
			functions: []string{
				`apiVersion: v1alpha1
kind: PodEnvs
metadata:
 annotations:
    kustomize.primer.ai/applytocm: only
spec:
  remove:
    - DD_AGENT_HOST
`,
			},
			expectedFns: []kio.Filter{
				configmap.WrappedFilter{
					Filters: []kio.Filter{
						&envs.PodEnvsFunction{
							Kind:       envs.Kind,
							APIVersion: "v1alpha1",
							Metadata: v1alpha1.Metadata{
								Annotations: map[string]string{
									configmap.ConfigMapAnnotation: configmap.ConfigMapOnly,
								},
							},
							Spec: envs.Spec{
								Remove: []string{"DD_AGENT_HOST"},
							},
						},
					},
				},
				expectedDefaultMergeFilter,
				expectedDefaultFileSetterFilter,
				expectedDefaultFormatFilter,
			},
		},
		{
			name: "apply-to-both",
			functions: []string{
				`apiVersion: v1alpha1
kind: PodEnvs
metadata:
 annotations:
    kustomize.primer.ai/applytocm: both
spec:
  remove:
    - DD_AGENT_HOST
`,
			},
			expectedFns: []kio.Filter{
				expectedBothPodFn,
				configmap.WrappedFilter{
					Filters: []kio.Filter{
						&envs.PodEnvsFunction{
							Kind:       envs.Kind,
							APIVersion: "v1alpha1",
							Metadata: v1alpha1.Metadata{
								Annotations: map[string]string{
									configmap.ConfigMapAnnotation: configmap.ConfigMapBoth,
								},
							},
							Spec: envs.Spec{
								Remove: []string{"DD_AGENT_HOST"},
							},
						},
					},
				},
				expectedDefaultMergeFilter,
				expectedDefaultFileSetterFilter,
				expectedDefaultFormatFilter,
			},
		},
	}

	for _, testCase := range cases {
		// capture range variable
		tc := testCase
		t.Run(tc.name, func(t *testing.T) {
			contents := strings.Join(tc.functions, "\n---\n")
			buf := strings.NewReader(contents)

			reader := &kio.ByteReader{
				Reader:                buf,
				OmitReaderAnnotations: true,
			}

			configs, err := reader.Read()
			if err != nil {
				t.Errorf("loadFilters failed; error %v", err)
				return
			}

			fns, err := d.loadFilters(configs)
			if err != nil {
				t.Errorf("readFilter returned error; %v", err)
				return
			}

			if e := cmp.Diff(tc.expectedFns, fns); e != "" {
				t.Errorf("Did not get expected filters; diff:\n%v", e)
			}
		})
	}
}

func Test_getAllFuncs(t *testing.T) {
	log := util.SetupLogger("info", true)

	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("Error getting current directory; error %v", err)
	}

	testDir := path.Join(cwd, "test_data")
	sourceRepo := path.Join(testDir, "test_repo_source")

	d := Dispatcher{
		Log: log,
	}

	kioBuff, err := d.GetAllFuncs([]string{sourceRepo})
	if err != nil {
		t.Errorf("GetAllFuncs failed; error %v", err)
		return
	}

	expectedFunctions := 4

	if expectedFunctions != len(kioBuff.Nodes) {
		t.Errorf("unexpected diff; expected number of functions %v; actual number of functions %v", expectedFunctions, len(kioBuff.Nodes))
	}
}

func Test_SetFuncPaths(t *testing.T) {
	log := util.SetupLogger("info", true)

	d := Dispatcher{
		Log: log,
	}

	type testCase struct {
		name                       string
		buff                       kio.PackageBuffer
		hydratedPath               string
		sourceRoot                 string
		filesToHydrate             map[TargetPath]bool
		expectedSourceFunctionPath string
		expectedFunctionTargetDir  string
	}

	tests := []testCase{
		{
			name: "Test_SetFuncPaths passing case",
			buff: kio.PackageBuffer{
				Nodes: []*yaml.RNode{
					yaml.MustParse(`
metadata:
 annotations:
   config.kubernetes.io/path: configs/newdir/dev/label.yaml
   config.kubernetes.io/index: 0
`),
				},
			},
			hydratedPath: "/usr/hydros/repo/fork/k8s/repo",
			sourceRoot:   "/usr/hydros/repo/source",
			filesToHydrate: map[TargetPath]bool{{
				Dir:         "configs/newdir",
				OverlayName: "dev",
			}: true},
			expectedSourceFunctionPath: "/usr/hydros/repo/source/configs/newdir/dev",
			expectedFunctionTargetDir:  "/usr/hydros/repo/fork/k8s/repo/configs/newdir",
		},
	}

	for _, tc := range tests {
		err := d.SetFuncPaths(tc.buff, tc.hydratedPath, tc.sourceRoot, tc.filesToHydrate)
		if err != nil {
			t.Errorf("SetFuncPaths failed; error %v", err)
			return
		}
		annotations := tc.buff.Nodes[0].GetAnnotations()

		if annotations[SourceFunctionPath] != tc.expectedSourceFunctionPath {
			t.Errorf("Unexpected Source Function path mismatch for test %v; expected %v; actual %v",
				tc.name, tc.expectedSourceFunctionPath, annotations[SourceFunctionPath])
		}

		if annotations[FunctionTargetDir] != tc.expectedFunctionTargetDir {
			t.Errorf("Unexpected Function Target Dir path mismatch for test %v; expected %v; actual %v",
				tc.name, tc.expectedFunctionTargetDir, annotations[FunctionTargetDir])
		}

	}
}

func Test_constructTargetPaths(t *testing.T) {
	log := util.SetupLogger("info", true)

	d := Dispatcher{
		Log: log,
	}

	type testCase struct {
		name           string
		sourceRoot     string
		pathAnnotation string
		hydratedPath   string
		filesToHydrate map[TargetPath]bool
		expectedPath   string
	}

	tests := []testCase{
		{
			name:           "Test_constructTargetPaths passing case with overlay",
			sourceRoot:     "/usr/hydros/repo/source",
			pathAnnotation: "configs/newdir/dev/labels.yaml",
			hydratedPath:   "/usr/hydros/repo/fork/k8s/repo",
			filesToHydrate: map[TargetPath]bool{{
				Dir:         "configs/newdir",
				OverlayName: "dev",
			}: true},
			expectedPath: "/usr/hydros/repo/fork/k8s/repo/configs/newdir",
		},
		{
			name:           "Test_constructTargetPaths passing case without overlay",
			sourceRoot:     "/usr/hydros/repo/source",
			pathAnnotation: "configs/newdir/labels.yaml",
			hydratedPath:   "/usr/hydros/repo/fork/k8s/repo",
			filesToHydrate: map[TargetPath]bool{{
				Dir:         "configs/newdir",
				OverlayName: "dev",
			}: true},
			expectedPath: "/usr/hydros/repo/fork/k8s/repo/configs/newdir",
		},
	}

	for _, tc := range tests {
		targetPath, err := d.constructTargetPath(tc.sourceRoot, tc.pathAnnotation, tc.filesToHydrate, tc.hydratedPath)
		if err != nil {
			t.Errorf("constructTargetPath failed; error %v", err)
			return
		}

		if targetPath != tc.expectedPath {
			t.Errorf("unexpected diff for function %v; expected %v, actual %v", tc.name, tc.expectedPath, targetPath)
		}
	}
}
