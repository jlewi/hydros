package envs

import (
	"bytes"
	"os"
	"path"
	"strings"
	"testing"

	"github.com/PrimerAI/hydros-public/api/v1alpha1"
	"github.com/stretchr/testify/assert"

	filtertest "sigs.k8s.io/kustomize/api/testutils/filtertest"
	"sigs.k8s.io/kustomize/kyaml/kio/filters"

	"github.com/PrimerAI/hydros-public/pkg/util"
	"github.com/google/go-cmp/cmp"

	utilyaml "k8s.io/apimachinery/pkg/util/yaml"
	"sigs.k8s.io/kustomize/kyaml/kio"
	"sigs.k8s.io/kustomize/kyaml/yaml"
)

func writeYaml(nodes []*yaml.RNode) ([]byte, error) {
	var b bytes.Buffer
	writer := kio.ByteWriter{
		Writer: &b,
	}

	if err := writer.Write(nodes); err != nil {
		return []byte{}, err
	}

	return b.Bytes(), nil
}

func Test_remove_envs(t *testing.T) {
	type testCase struct {
		InputFile    string
		ExpectedFile string
	}

	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("Error getting current directory; error %v", err)
	}

	testDir := path.Join(cwd, "test_data")

	cases := []testCase{
		{
			InputFile:    path.Join(testDir, "deployment_input.yaml"),
			ExpectedFile: path.Join(testDir, "deployment_expected.yaml"),
		},
		{
			InputFile:    path.Join(testDir, "cronjob_input.yaml"),
			ExpectedFile: path.Join(testDir, "cronjob_expected.yaml"),
		},
	}

	f := &PodEnvsFunction{
		Spec: Spec{
			Remove: []string{"DD_AGENT_HOST", "SENTRY_KEY"},
		},
	}

	for _, c := range cases {
		nodes, err := util.ReadYaml(c.InputFile)
		if err != nil {
			t.Errorf("Error reading YAML: %v", err)
		}

		if len(nodes) != 1 {
			t.Errorf("Expected 1 node in file %v", c.InputFile)
		}
		node := nodes[0]

		_, err = f.filter(node)
		if err != nil {
			t.Errorf("PodEnvs failed; error %v", err)
			continue
		}

		b, err := writeYaml([]*yaml.RNode{node})
		if err != nil {
			t.Errorf("Error writing yaml; error %v", err)
			continue
		}

		actual := string(b)

		// TODO(jeremy): I should clean this test case up to make it look like Test_SetEnvs.
		// In particular insteady of reading and writing the YAML file we can call RunFilter and use
		// filter.FormatFilter on the expected and actual input to ensure they are formatted consistently.

		// read the expected yaml and then rewrites using kio.ByteWriter.
		// We do this because ByteWriter makes some formatting decisions and we
		// we want to apply the same formatting to the expected values
		eNode, err := util.ReadYaml(c.ExpectedFile)
		if err != nil {
			t.Errorf("Could not read expected file %v; error %v", c.ExpectedFile, err)
		}

		eBytes, err := writeYaml(eNode)
		if err != nil {
			t.Errorf("Could not format expected file %v; error %v", c.ExpectedFile, err)
		}

		expected := string(eBytes)

		d := cmp.Diff(expected, actual)
		if d != "" {
			t.Errorf("Unexpected diff:\n%v", d)
		}
	}
}

func Test_SetEnvs(t *testing.T) {
	tBool := true
	testCases := map[string]struct {
		input          string
		expectedOutput string
		filter         PodEnvsFunction
	}{
		"deployment": {
			input: `
group: apps
apiVersion: v1
kind: Deployment
metadata:
 name: deploy1
spec:
 template:
   spec:
     containers:
     - image: old.docker.io/nginx:1.7.9
       name: nginx-tagged
       env:
       - name: A
         value: old
       - name: B
         value: unchanged
     - image: old2.docker.io/nginx:latest
       name: nginx-latest
`,
			expectedOutput: `
group: apps
apiVersion: v1
kind: Deployment
metadata:
 name: deploy1
spec:
 template:
   spec:
     containers:
     - image: old.docker.io/nginx:1.7.9
       name: nginx-tagged
       env:
       - name: A
         value: new
       - name: B
         value: unchanged
       - name: C
         value: newC
       - name: D
         valueFrom:
           secretKeyRef:
             name: local-secret-name
             key: key-in-secret-object
       - name: E
         valueFrom:
           configMapKeyRef:
             name: local-cmap-name
             key: key-in-cmap
             optional: true
       - name: F
         valueFrom:
           fieldRef:
             fieldPath: metadata.namespace
       - name: G
         valueFrom:
           resourceFieldRef:
             containerName: nginx-tagged
             resource: mem
     - image: old2.docker.io/nginx:latest
       name: nginx-latest
       env:
       - name: A
         value: new
       - name: C
         value: newC
       - name: D
         valueFrom:
           secretKeyRef:
             name: local-secret-name
             key: key-in-secret-object
       - name: E
         valueFrom:
           configMapKeyRef:
             name: local-cmap-name
             key: key-in-cmap
             optional: true
       - name: F
         valueFrom:
           fieldRef:
             fieldPath: metadata.namespace
       - name: G
         valueFrom:
           resourceFieldRef:
             containerName: nginx-tagged
             resource: mem
`,
			filter: PodEnvsFunction{
				Metadata: v1alpha1.Metadata{
					Name: "deployment",
				},
				Spec: Spec{
					Set: []EnvVar{
						{
							Name:  "A",
							Value: "new",
						},
						{
							Name:  "C",
							Value: "newC",
						},
						{
							Name: "D",
							ValueFrom: &EnvVarSource{
								SecretKeyRef: &SecretKeySelector{
									Key: "key-in-secret-object",
									LocalObjectReference: LocalObjectReference{
										Name: "local-secret-name",
									},
								},
							},
						},
						{
							Name: "E",
							ValueFrom: &EnvVarSource{
								ConfigMapKeyRef: &ConfigMapKeySelector{
									Key: "key-in-cmap",
									LocalObjectReference: LocalObjectReference{
										Name: "local-cmap-name",
									},
									Optional: &tBool,
								},
							},
						},
						{
							Name: "F",
							ValueFrom: &EnvVarSource{
								FieldRef: &ObjectFieldSelector{
									FieldPath: "metadata.namespace",
								},
							},
						},
						{
							Name: "G",
							ValueFrom: &EnvVarSource{
								ResourceFieldRef: &ResourceFieldSelector{
									ContainerName: "nginx-tagged",
									Resource:      "mem",
								},
							},
						},
					},
				},
			},
		},
		// This test case ensures numeric values are properly quoted
		"numeric": {
			input: `
group: apps
apiVersion: v1
kind: Deployment
metadata:
 name: deploy1
spec:
 template:
   spec:
     containers:
     - image: old.docker.io/nginx:1.7.9
       name: nginx-tagged
`,
			expectedOutput: `
group: apps
apiVersion: v1
kind: Deployment
metadata:
 name: deploy1
spec:
 template:
   spec:
     containers:
     - image: old.docker.io/nginx:1.7.9
       name: nginx-tagged
       env:
       - name: SENTRY
         value: "1234" 
`,
			filter: PodEnvsFunction{
				Metadata: v1alpha1.Metadata{
					Name: "deployment",
				},
				Spec: Spec{
					Set: []EnvVar{
						{
							Name:  "SENTRY",
							Value: "1234",
						},
					},
				},
			},
		},
		// This test case ensures bool values are properly quoted
		"bool": {
			input: `
group: apps
apiVersion: v1
kind: Deployment
metadata:
 name: deploy1
spec:
 template:
   spec:
     containers:
     - image: old.docker.io/nginx:1.7.9
       name: nginx-tagged
`,
			expectedOutput: `
group: apps
apiVersion: v1
kind: Deployment
metadata:
 name: deploy1
spec:
 template:
   spec:
     containers:
     - image: old.docker.io/nginx:1.7.9
       name: nginx-tagged
       env:
       - name: SOME_BOOL_VAR
         value: "false"
`,
			filter: PodEnvsFunction{
				Metadata: v1alpha1.Metadata{
					Name: "deployment",
				},
				Spec: Spec{
					Set: []EnvVar{
						{
							Name:  "SOME_BOOL_VAR",
							Value: "false",
						},
					},
				},
			},
		},
	}

	for tn, tc := range testCases {
		t.Run(tn, func(t *testing.T) {
			filter := tc.filter
			if err := filter.init(); err != nil {
				t.Errorf("init failed; error %v", err)
				return
			}

			if !assert.Equal(t,
				strings.TrimSpace(filtertest.RunFilter(t, tc.expectedOutput, &filters.FormatFilter{})),
				strings.TrimSpace(filtertest.RunFilter(t, tc.input, &pipeline{f: filter}))) {
				t.FailNow()
			}
		})
	}
}

func TestYamlLoad(t *testing.T) {
	tBool := true
	testCases := map[string]struct {
		yaml     string
		expected Spec
	}{
		"simple-value": {
			yaml: strings.TrimSpace(`
apiVersion: v1alpha1
kind: PodEnvs
metadata:
  name: pod-envs
spec:
  set:
    - name: SOME_VAR
      value: abc123
`),
			expected: Spec{
				Set: []EnvVar{
					{
						Name:  "SOME_VAR",
						Value: "abc123",
					},
				},
			},
		},
		"value-from": {
			yaml: strings.TrimSpace(`
apiVersion: v1alpha1
kind: PodEnvs
metadata:
  name: pod-envs
spec:
  set:
    - name: SOME_VAR
      value: abc123
    - name: VALUE_FROM_VAR
      valueFrom:
        secretKeyRef:
          name: some-secret
          key: SOME_KEY_IN_SECRET
    - name: E
      valueFrom:
        configMapKeyRef:
          name: local-cmap-name
          key: key-in-cmap
          optional: true
    - name: F
      valueFrom:
        fieldRef:
          fieldPath: metadata.namespace
    - name: G
      valueFrom:
        resourceFieldRef:
          containerName: nginx-tagged
          resource: mem
`),
			expected: Spec{
				Set: []EnvVar{
					{
						Name:  "SOME_VAR",
						Value: "abc123",
					},
					{
						Name: "VALUE_FROM_VAR",
						ValueFrom: &EnvVarSource{
							SecretKeyRef: &SecretKeySelector{
								Key: "SOME_KEY_IN_SECRET",
								LocalObjectReference: LocalObjectReference{
									Name: "some-secret",
								},
							},
						},
					},
					{
						Name: "E",
						ValueFrom: &EnvVarSource{
							ConfigMapKeyRef: &ConfigMapKeySelector{
								Key: "key-in-cmap",
								LocalObjectReference: LocalObjectReference{
									Name: "local-cmap-name",
								},
								Optional: &tBool,
							},
						},
					},
					{
						Name: "F",
						ValueFrom: &EnvVarSource{
							FieldRef: &ObjectFieldSelector{
								FieldPath: "metadata.namespace",
							},
						},
					},
					{
						Name: "G",
						ValueFrom: &EnvVarSource{
							ResourceFieldRef: &ResourceFieldSelector{
								ContainerName: "nginx-tagged",
								Resource:      "mem",
							},
						},
					},
				},
			},
		},
	}

	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			fnStruct := &PodEnvsFunction{}

			bSlice := []byte(tc.yaml)
			bReader := bytes.NewReader(bSlice)
			yjDecoder := utilyaml.NewYAMLOrJSONDecoder(bReader, len(bSlice))
			assert.NoError(t, yjDecoder.Decode(fnStruct))
			assert.Equal(t, tc.expected, fnStruct.Spec, "Both structs should be equal")
		})
	}
}

// n.b. we need to run formatfilter on the actual output as the formatfilter causes the keys in
// dictionaries to be sorted which is necessary otherwise output will be random
type pipeline struct {
	f PodEnvsFunction
}

func (p pipeline) Filter(inputs []*yaml.RNode) ([]*yaml.RNode, error) {
	_, err := p.f.Filter(inputs)
	if err != nil {
		return inputs, err
	}

	formatter := &filters.FormatFilter{}

	return formatter.Filter(inputs)
}
