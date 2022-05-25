package fns

import (
	"strings"
	"testing"

	"github.com/PrimerAI/hydros-public/pkg/kustomize/fns/null"
	"github.com/stretchr/testify/assert"
	filtertest "sigs.k8s.io/kustomize/api/testutils/filtertest"
	"sigs.k8s.io/kustomize/api/types"
)

func Test_KeysClearer(t *testing.T) {
	testCases := map[string]struct {
		input          string
		expectedOutput string
		filter         KeysClearer
	}{
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
data:
  UNMODIFIED: someothervalue 
`,
			filter: KeysClearer{
				Keys: []string{"keytoremove"},
				FsSlice: types.FsSlice{
					{
						Path: "metadata/labels",
					},
				},
			},
		},
		"somemap": {
			input: `
apiVersion: v1
kind: ConfigMap
metadata:
 name: tenant-config
data:
 somemap:
   keytoremove: oldvalue
   valuetokeep: v
`,
			expectedOutput: `
apiVersion: v1
kind: ConfigMap
metadata:
 name: tenant-config
data:
 somemap:
   valuetokeep: v
`,
			filter: KeysClearer{
				Keys: []string{"keytoremove"},
				FsSlice: types.FsSlice{
					{
						Path: "data/somemap",
					},
				},
			},
		},
		"listofmaps": {
			input: `
apiVersion: v1
kind: 
spec:
  items:
  -  name: item1
     valuetokeep: v
     labels:
       toremove: a
  -  name: item2
     valuetokeep: v
     labels:
       toremove: a
`,
			expectedOutput: `
apiVersion: v1
kind: 
spec:
  items:
  -  name: item1
     valuetokeep: v
     labels: {}
  -  name: item2
     valuetokeep: v
     labels: {}
`,
			filter: KeysClearer{
				Keys: []string{"toremove"},
				FsSlice: types.FsSlice{
					{
						Path: "spec/items[]/labels",
					},
				},
			},
		},
	}

	for tn, tc := range testCases {
		t.Run(tn, func(t *testing.T) {
			if !assert.Equal(t,
				strings.TrimSpace(filtertest.RunFilter(t, tc.expectedOutput, &null.Filter{})),
				strings.TrimSpace(filtertest.RunFilter(t, tc.input, tc.filter))) {
				t.FailNow()
			}
		})
	}
}
