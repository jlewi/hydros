package fields

import (
	"strings"
	"testing"

	"github.com/jlewi/hydros/api/v1alpha1"
	"github.com/stretchr/testify/assert"
	filtertest "sigs.k8s.io/kustomize/api/testutils/filtertest"
	"sigs.k8s.io/kustomize/kyaml/kio/filters"
	"sigs.k8s.io/kustomize/kyaml/yaml"
)

func Test_Fields(t *testing.T) {
	testCases := map[string]struct {
		name           string
		input          string
		expectedOutput string
		filter         Fields
	}{
		"deployment": {
			name: "test remove affinity and tolerations; deployment",
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
      affinity:
        nodeAffinity:
          requiredDuringSchedulingIgnoredDuringExecution:
            nodeSelectorTerms:
            - matchExpressions:
              - key: dd-apm
                operator: In
                values:
                - "true"
              - key: instancetype
                operator: In
                values:
                - gpu
      tolerations:
      - effect: NoSchedule
        key: instancetype
        operator: Equal
        value: gpu
`,
			expectedOutput: `
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
			filter: Fields{
				Metadata: v1alpha1.Metadata{
					Name: "testing",
				},
				Spec: Spec{
					Remove: []string{
						"spec.template.spec.affinity",
						"spec.template.spec.tolerations",
					},
				},
			},
		},
		"configmap": {
			name: "test remove affinity; configmap",
			input: `
apiVersion: v1
kind: ConfigMap
metadata:
 name: tenant-config
 labels:
    somekey: somevalue
spec:
 template:
   spec:
     containers:
     - name: ghapp
     affinity:
       nodeAffinity:
         requiredDuringSchedulingIgnoredDuringExecution:
           nodeSelectorTerms:
           - matchExpressions:
             - key: dd-apm
               operator: In
               values:
               - "true"
             - key: instancetype
               operator: In
               values:
               - gpu
     tolerations:
     - effect: NoSchedule
       key: instancetype
       operator: Equal
       value: gpu

data:
 UNMODIFIED: someothervalue`,
			expectedOutput: `
apiVersion: v1
kind: ConfigMap
metadata:
 name: tenant-config
 labels:
    somekey: somevalue
spec:
 template:
   spec:
     containers:
     - name: ghapp
     tolerations:
     - effect: NoSchedule
       key: instancetype
       operator: Equal
       value: gpu
data:
 UNMODIFIED: someothervalue
`,
			filter: Fields{
				Metadata: v1alpha1.Metadata{
					Name: "testing",
				},
				Spec: Spec{
					Remove: []string{
						"spec.template.spec.affinity",
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
	f Fields
}

func (p pipeline) Filter(inputs []*yaml.RNode) ([]*yaml.RNode, error) {
	_, err := p.f.Filter(inputs)
	if err != nil {
		return inputs, err
	}

	formatter := &filters.FormatFilter{}

	return formatter.Filter(inputs)
}
