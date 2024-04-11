package configmap

import (
	"strings"
	"testing"

	"github.com/jlewi/hydros/pkg/kustomize/fns/null"
	"github.com/stretchr/testify/assert"
	labelsFn "sigs.k8s.io/kustomize/api/filters/labels"
	filtertest "sigs.k8s.io/kustomize/api/testutils/filtertest"
	"sigs.k8s.io/kustomize/api/types"
	"sigs.k8s.io/kustomize/kyaml/kio"
	"sigs.k8s.io/kustomize/kyaml/yaml"
)

func Test_CMPipeline(t *testing.T) {
	testCases := map[string]struct {
		inputData    map[string]string
		expectedData map[string]string
		filter       WrappedFilter
	}{
		"cm": {
			inputData: map[string]string{
				"deploy.yaml": `apiVersion: apps/v1
kind: Deployment
metadata:
  name: gateway
  namespace: mlp-automl-1
  labels:
    mlp-tenant: automl-1
    owner: ml-platform
spec:
  replicas: 1
  selector:
    matchLabels:
      ghapp: gateway
      mlp-tenant: automl-1
`,
			},
			expectedData: map[string]string{
				"deploy.yaml": `apiVersion: apps/v1
kind: Deployment
metadata:
  name: gateway
  namespace: mlp-automl-1
  labels:
    mlp-tenant: automl-1
    owner: ml-platform
    newlabel: newvalue
spec:
  replicas: 1
  selector:
    matchLabels:
      ghapp: gateway
      mlp-tenant: automl-1
`,
			},
			// We don't use our kustomize fns because we want to avoid cross dependencies between the packages.
			filter: WrappedFilter{
				Filters: []kio.Filter{
					labelsFn.Filter{
						Labels: map[string]string{
							"newlabel": "newvalue",
						},
						FsSlice: types.FsSlice{
							{
								Path: "metadata/labels",
							},
						},
					},
				},
			},
		},
		"multiple-values": {
			inputData: map[string]string{
				"deploy.yaml": `apiVersion: apps/v1
kind: Deployment
metadata:
  name: gateway
  namespace: mlp-automl-1
  labels:
    mlp-tenant: automl-1
    owner: ml-platform
`,
				"deploy2.yaml": `apiVersion: apps/v1
kind: Deployment
metadata:
  name: gateway
  namespace: mlp-automl-1
  labels:
    mlp-tenant: automl-1
    owner: ml-platform
`,
			},
			expectedData: map[string]string{
				"deploy.yaml": `apiVersion: apps/v1
kind: Deployment
metadata:
  name: gateway
  namespace: mlp-automl-1
  labels:
    mlp-tenant: automl-1
    owner: ml-platform
    newlabel: newvalue
`,
				"deploy2.yaml": `apiVersion: apps/v1
kind: Deployment
metadata:
  name: gateway
  namespace: mlp-automl-1
  labels:
    mlp-tenant: automl-1
    owner: ml-platform
    newlabel: newvalue
`,
			},
			// We don't use our kustomize fns because we want to avoid cross dependencies between the packages.
			filter: WrappedFilter{
				Filters: []kio.Filter{
					labelsFn.Filter{
						Labels: map[string]string{
							"newlabel": "newvalue",
						},
						FsSlice: types.FsSlice{
							{
								Path: "metadata/labels",
							},
						},
					},
				},
			},
		},
	}

	cmBase := `
apiVersion: v1
kind: ConfigMap
metadata:
name: tenant-config
`

	for tn, tc := range testCases {
		t.Run(tn, func(t *testing.T) {
			filter := tc.filter

			// Read in the configmap and create input and expected.
			nodes, err := kio.FromBytes([]byte(cmBase))
			if err != nil {
				t.Fatalf("Failed to read ConfigMap; %v", err)
			}

			if len(nodes) != 1 {
				t.Fatalf("Got %v nodes; want 1", len(nodes))
			}

			cm := nodes[0]

			if err := cm.LoadMapIntoConfigMapData(tc.inputData); err != nil {
				t.Errorf("Failed to load configmap; %v", err)
				return
			}

			input, err := kio.StringAll([]*yaml.RNode{cm})
			if err != nil {
				t.Fatalf("Failed to serialize input; %v", input)
			}

			if err := cm.LoadMapIntoConfigMapData(tc.expectedData); err != nil {
				t.Errorf("Failed to load configmap; %v", err)
				return
			}
			expected, err := kio.StringAll([]*yaml.RNode{cm})
			if err != nil {
				t.Fatalf("Failed to serialize expected; %v", input)
			}

			if !assert.Equal(t,
				strings.TrimSpace(filtertest.RunFilter(t, expected, &null.Filter{})),
				strings.TrimSpace(filtertest.RunFilter(t, input, filter))) {
				t.FailNow()
			}
		})
	}
}
