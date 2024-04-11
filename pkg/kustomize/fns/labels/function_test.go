package labels

import (
	"strings"
	"testing"

	"sigs.k8s.io/kustomize/kyaml/kio/filters"
	"sigs.k8s.io/kustomize/kyaml/yaml"

	"github.com/jlewi/hydros/api/v1alpha1"
	"github.com/stretchr/testify/assert"
	filtertest "sigs.k8s.io/kustomize/api/testutils/filtertest"
)

func Test_Labels(t *testing.T) {
	testCases := map[string]struct {
		input          string
		expectedOutput string
		filter         CommonLabelsFn
	}{
		"deployment": {
			input: `
apiVersion: v1
kind: deployment
metadata:
  name: tenant-config
  labels:
     somekey: somevalue
     keytoremove: someothervalue
spec:
  template:
    metadata:
      labels:
        keytoremove: someothervalue
    spec:
      containers:
      - name: ghapp
`,
			expectedOutput: `
apiVersion: v1
kind: deployment
metadata:
  name: tenant-config
  labels:
     somekey: somevalue
     newlabel: newvalue
spec:
  template:
    metadata:
      labels:
        newlabel: newvalue
    spec:
      containers:
      - name: ghapp
`,
			filter: CommonLabelsFn{
				Metadata: v1alpha1.Metadata{
					Name: "somefunc",
				},
				Spec: Spec{
					Labels: map[string]string{
						"newlabel": "newvalue",
					},
					RemoveLabels: []string{
						"keytoremove",
					},
				},
			},
		},
		"metadata-labels": {
			input: `
apiVersion: v1
kind: ConfigMap
metadata:
  name: tenant-config
  labels:
     somekey: somevalue
     keytoremove: someothervalue
data: 
  UNMODIFIED: someothervalue`,
			expectedOutput: `
apiVersion: v1
kind: ConfigMap
metadata:
  name: tenant-config
  labels:
     somekey: somevalue
     newlabel: newvalue
data:
  UNMODIFIED: someothervalue 
`,
			filter: CommonLabelsFn{
				Metadata: v1alpha1.Metadata{
					Name: "somefunc",
				},
				Spec: Spec{
					Labels: map[string]string{
						"newlabel": "newvalue",
					},
					RemoveLabels: []string{
						"keytoremove",
					},
				},
			},
		},
		// This test verifies that if metadata.labels is empty metadata.labels will be created.
		"empty_labels": {
			input: `
apiVersion: v1
kind: deployment
metadata:
  name: tenant-config
spec:
  template:
    spec:
      containers:
      - name: ghapp
`,
			expectedOutput: `
apiVersion: v1
kind: deployment
metadata:
  name: tenant-config
  labels:
    newlabel: newvalue
spec:
  template:
    spec:
      containers:
      - name: ghapp
`,
			filter: CommonLabelsFn{
				Metadata: v1alpha1.Metadata{
					Name: "somefunc",
				},
				Spec: Spec{
					Labels: map[string]string{
						"newlabel": "newvalue",
					},
				},
			},
		},
	}
	for tn, tc := range testCases {
		t.Run(tn, func(t *testing.T) {
			if !assert.Equal(t,
				strings.TrimSpace(filtertest.RunFilter(t, tc.expectedOutput, filters.FormatFilter{})),
				strings.TrimSpace(filtertest.RunFilter(t, tc.input, &pipeline{f: tc.filter}))) {
				t.FailNow()
			}
		})
	}
}

// n.b. we need to run formatfilter on the actual output as the formatfilter causes the keys in
// dictionaries to be sorted which is necessary otherwise output will be random
type pipeline struct {
	f CommonLabelsFn
}

func (p pipeline) Filter(inputs []*yaml.RNode) ([]*yaml.RNode, error) {
	_, err := p.f.Filter(inputs)
	if err != nil {
		return inputs, err
	}

	formatter := &filters.FormatFilter{}

	return formatter.Filter(inputs)
}
