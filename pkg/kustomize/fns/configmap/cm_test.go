package configmap

import (
	"strings"
	"testing"

	"github.com/jlewi/hydros/api/v1alpha1"
	"github.com/jlewi/hydros/pkg/kustomize/fns/null"
	"github.com/stretchr/testify/assert"
	filtertest "sigs.k8s.io/kustomize/api/testutils/filtertest"
)

func TestCM_Filter(t *testing.T) {
	testCases := map[string]struct {
		input          string
		expectedOutput string
		filter         PatchFn
	}{
		"cm": {
			input: `
apiVersion: v1
kind: ConfigMap
metadata:
  name: tenant-config
data: 
  UNMODIFIED: someothervalue
  GATEWAY_TOPIC: mlp-primer-gateway
  KAFKA_BROKER: kafka-ml-broker-0.dev.primering.net:9092,kafka-ml-broker-1.dev.primering.net:9092,kafka-ml-broker-2.dev.primering.net:9092`,
			expectedOutput: `
apiVersion: v1
kind: ConfigMap
metadata:
  name: tenant-config
data:
  UNMODIFIED: someothervalue
  GATEWAY_TOPIC: newtopic
  KAFKA_BROKER: newbroker
`,
			filter: PatchFn{
				Metadata: v1alpha1.Metadata{
					Name: "cm",
				},
				Spec: Spec{
					Data: map[string]string{
						"KAFKA_BROKER":  "newbroker",
						"GATEWAY_TOPIC": "newtopic",
					},
				},
			},
		},
		// This test case verifies that if a CM doesn't have some values they don't get set
		"dontset": {
			input: `
apiVersion: v1
kind: ConfigMap
metadata:
  name: tenant-config
data: 
  UNMODIFIED: someothervalue 
  KAFKA_BROKER: kafka-ml-broker-0.dev.primering.net:9092,kafka-ml-broker-1.dev.primering.net:9092,kafka-ml-broker-2.dev.primering.net:9092`,
			expectedOutput: `
apiVersion: v1
kind: ConfigMap
metadata:
  name: tenant-config
data:
  UNMODIFIED: someothervalue 
  KAFKA_BROKER: newbroker
`,
			filter: PatchFn{
				Metadata: v1alpha1.Metadata{
					Name: "cm",
				},
				Spec: Spec{
					Data: map[string]string{
						"KAFKA_BROKER": "newbroker",
						// Set a value which isn't in the config map so we can verify it doesn't get set.
						"GATEWAY_TOPIC": "newtopic",
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
				strings.TrimSpace(filtertest.RunFilter(t, tc.expectedOutput, &null.Filter{})),
				strings.TrimSpace(filtertest.RunFilter(t, tc.input, filter))) {
				t.FailNow()
			}
		})
	}
}
