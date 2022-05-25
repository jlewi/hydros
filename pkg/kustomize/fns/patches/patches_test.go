package patches

import (
	"strings"
	"testing"

	"github.com/PrimerAI/hydros-public/api/v1alpha1"
	"github.com/PrimerAI/hydros-public/pkg/kustomize/fns/null"
	"github.com/stretchr/testify/assert"
	filtertest "sigs.k8s.io/kustomize/api/testutils/filtertest"
	"sigs.k8s.io/kustomize/kyaml/fn/framework"
	"sigs.k8s.io/kustomize/kyaml/yaml"
)

func TestPatches_Filter(t *testing.T) {
	testCases := map[string]struct {
		input          string
		expectedOutput string
		filter         PatchFn
	}{
		"deployment": {
			input: `
apiVersion: apps/v1
kind: Deployment
metadata:
 name: deploy1
spec:
 template:
   spec:
     containers:
     - image: nginx
       name: nginx-tagged
`,
			expectedOutput: `
apiVersion: apps/v1
kind: Deployment
metadata:
 name: deploy1
spec:
 template:
   spec:
     containers:
     - image: nginx
       name: nginx-tagged
       resources:
         cpu: 5
`,
			filter: PatchFn{
				Metadata: v1alpha1.Metadata{
					Name: "deployment",
				},
				Spec: Spec{
					Patches: []string{
						`
spec:
 template:
   spec: 
     containers:
     - name: nginx-tagged
       resources:
         cpu: 5`,
					},
					Selector: &framework.Selector{
						Names: []string{"deploy1"},
					},
				},
			},
		},
		"json6902-add": {
			input: `
apiVersion: apps/v1
kind: Deployment
metadata:
    name: deploy1
spec:
    template:
      spec:
        containers:
        - image: nginx
          name: nginx-tagged`,
			expectedOutput: `
apiVersion: apps/v1
kind: Deployment
metadata:
    name: deploy1
spec:
    template:
      spec:
        containers:
        - image: nginx
          name: nginx-tagged
          port: 10`,
			filter: PatchFn{
				Metadata: v1alpha1.Metadata{
					Name: "deployment",
				},
				Spec: Spec{
					Patches: []string{`
- op: add
  path: "/spec/template/spec/containers/0/port"
  value: 10`},
					Selector: &framework.Selector{
						Names: []string{"deploy1"},
					},
				},
			},
		},
		"json6902-remove": {
			input: `
apiVersion: redis.redis.opstreelabs.in/v1beta1
kind: Redis
metadata:
  name: engines-redis
  namespace: engines
spec:
  affinity:
    nodeAffinity:
      requiredDuringSchedulingIgnoredDuringExecution:
        nodeSelectorTerms:
        - matchExpressions:
          - key: instancetype
            operator: In
            values:
            - ondemand
          - key: spotinst.io/node-lifecycle
            operator: In
            values:
            - od`,
			expectedOutput: `
apiVersion: redis.redis.opstreelabs.in/v1beta1
kind: Redis
metadata:
  name: engines-redis
  namespace: engines
spec: {}`,
			filter: PatchFn{
				Metadata: v1alpha1.Metadata{
					Name: "redis-rm-affinity",
				},
				Spec: Spec{
					Patches: []string{`
- op: remove
  path: "/spec/affinity"`},
					Selector: &framework.Selector{
						Kinds: []string{"Redis"},
					},
				},
			},
		},
	}

	for tn, tc := range testCases {
		t.Run(tn, func(t *testing.T) {
			if _, err := yaml.Parse(tc.input); err != nil {
				t.Fatalf("Failed to parse input YAML; %v", err)
			}
			if _, err := yaml.Parse(tc.expectedOutput); err != nil {
				t.Fatalf("Failed to parse expected output YAML; %v", err)
			}
			filter := tc.filter
			if err := filter.init(); err != nil {
				t.Errorf("init failed; error %v", err)
				return
			}

			if !assert.Equal(t,
				strings.TrimSpace(filtertest.RunFilter(t, tc.expectedOutput, &null.Filter{})),
				strings.TrimSpace(filtertest.RunFilter(t, tc.input, filter))) {
				t.FailNow()
			}
		})
	}
}
