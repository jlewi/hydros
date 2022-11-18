// Copyright 2020 The Kubernetes Authors.
// SPDX-License-Identifier: Apache-2.0

package images

import (
	"strings"
	"testing"

	"github.com/jlewi/hydros/api/v1alpha1"
	"github.com/jlewi/hydros/pkg/kustomize/fns/null"

	"github.com/stretchr/testify/assert"
	filtertest "sigs.k8s.io/kustomize/api/testutils/filtertest"
)

func TestImageTagUpdater_Filter(t *testing.T) {
	testCases := map[string]struct {
		input          string
		expectedOutput string
		filter         ImagePrefixFn
	}{
		"ignore CustomResourceDefinition": {
			input: `
apiVersion: apiextensions.k8s.io/v1
kind: CustomResourceDefinition
metadata:
  name: whatever
spec:
  containers:
  - image: whatever
`,
			expectedOutput: `
apiVersion: apiextensions.k8s.io/v1
kind: CustomResourceDefinition
metadata:
  name: whatever
spec:
  containers:
    - image: whatever
`,
			filter: ImagePrefixFn{
				Metadata: v1alpha1.Metadata{
					Name: "ignorecrd",
				},
				Spec: Spec{
					ImageMappings: []ImageMapping{
						{
							Src:  "old.docker.io",
							Dest: "new.docker.io",
						},
					},
				},
			},
		},
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
     - image: old2.docker.io/nginx:latest
       name: nginx-latest 
     initContainers:
     - image: old.docker.io/nginx
       name: nginx-notag
     - image: old.docker.io/nginx@sha256:111111111111111111
       name: nginx-sha256`,
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
     - image: new.docker.io/nginx:1.7.9
       name: nginx-tagged
     - image: new2.docker.io/nginx:latest
       name: nginx-latest
     initContainers:
     - image: new.docker.io/nginx
       name: nginx-notag
     - image: new.docker.io/nginx@sha256:111111111111111111
       name: nginx-sha256
`,
			filter: ImagePrefixFn{
				Metadata: v1alpha1.Metadata{
					Name: "deployment",
				},
				Spec: Spec{
					ImageMappings: []ImageMapping{
						{
							Src:  "old.docker.io",
							Dest: "new.docker.io",
						},
						{
							Src:  "old2.docker.io",
							Dest: "new2.docker.io",
						},
					},
				},
			},
		},
		"emptyContainers": {
			input: `
group: apps
apiVersion: v1
kind: Deployment
metadata:
name: deploy1
spec:
containers: []
`,
			expectedOutput: `
group: apps
apiVersion: v1
kind: Deployment
metadata:
name: deploy1
spec:
containers: []
`,
			filter: ImagePrefixFn{
				Metadata: v1alpha1.Metadata{
					Name: "deployment",
				},
				Spec: Spec{
					ImageMappings: []ImageMapping{
						{
							Src:  "old.docker.io",
							Dest: "new.docker.io",
						},
						{
							Src:  "old2.docker.io",
							Dest: "new2.docker.io",
						},
					},
				},
			},
		},
		"ConfigMap_ImageReplace": {
			input: `
kind: ConfigMap
data:
 REDIS_IMAGE: old.docker.io/nginx:1.7.9
 IO_SIDECAR_IMAGE: old-image
 SERVER_IMAGE: old-image
 MLT_TRAINER_IMAGE: old-image
 NOT_AN_IMAGE_WE_CARE_ABOUT: old-image
`,
			expectedOutput: `
kind: ConfigMap
data:
 REDIS_IMAGE: new.host/new-repo:newtag@newdigest
 IO_SIDECAR_IMAGE: new-image
 SERVER_IMAGE: new-image
 MLT_TRAINER_IMAGE: new-image
 NOT_AN_IMAGE_WE_CARE_ABOUT: old-image
`,
			filter: ImagePrefixFn{
				Metadata: v1alpha1.Metadata{
					Name: "cmap-image-replace",
				},
				Spec: Spec{
					ImageMappings: []ImageMapping{
						{
							Src:  "old.docker.io/nginx:1.7.9",
							Dest: "new.host/new-repo:newtag@newdigest",
						},
						{
							Src:  "old-image",
							Dest: "new-image",
						},
					},
				},
			},
		},
		"strimzi-kafka-cluster": {
			input: `
apiVersion: kafka.strimzi.io/v1beta2
kind: Kafka
metadata:
  name: some-kafka-cluster
  namespace: some-namespace
spec:
  entityOperator:
    topicOperator:
      image: kafka-image-old
    userOperator:
      image: kafka-image-old
  kafka:
    image: kafka-image-old
  zookeeper:
    image: kafka-image-old
  kafkaExporter:
    image: kafka-image-old
`,
			expectedOutput: `
apiVersion: kafka.strimzi.io/v1beta2
kind: Kafka
metadata:
  name: some-kafka-cluster
  namespace: some-namespace
spec:
  entityOperator:
    topicOperator:
      image: kafka-image-new
    userOperator:
      image: kafka-image-new
  kafka:
    image: kafka-image-new
  zookeeper:
    image: kafka-image-new
  kafkaExporter:
    image: kafka-image-new
`,
			filter: ImagePrefixFn{
				Metadata: v1alpha1.Metadata{
					Name: "strimzi-kafka-image-replace",
				},
				Spec: Spec{
					ImageMappings: []ImageMapping{
						{
							Src:  "kafka-image-old",
							Dest: "kafka-image-new",
						},
					},
				},
			},
		},
		"redis-cluster": {
			input: `
apiVersion: redis.redis.opstreelabs.in/v1beta1
kind: RedisCluster
metadata:
  name: redis-cluster
spec:
  kubernetesConfig:
    image: redis-image-old
  redisExporter:
    image: redis-image-old
`,
			expectedOutput: `
apiVersion: redis.redis.opstreelabs.in/v1beta1
kind: RedisCluster
metadata:
  name: redis-cluster
spec:
  kubernetesConfig:
    image: redis-image-new
  redisExporter:
    image: redis-image-new
`,
			filter: ImagePrefixFn{
				Metadata: v1alpha1.Metadata{
					Name: "redis-cluster-image-replace",
				},
				Spec: Spec{
					ImageMappings: []ImageMapping{
						{
							Src:  "redis-image-old",
							Dest: "redis-image-new",
						},
					},
				},
			},
		},
		"cronjob": {
			input: `
apiVersion: batch/v1beta1
kind: CronJob
metadata:
  name: project-cleanup
  namespace: project
spec:
  jobTemplate:
    spec:
      template:
        metadata:
        spec:
          containers:
          - image: old-cron-container-image
          - image: old-cron-container-image
`,
			expectedOutput: `
apiVersion: batch/v1beta1
kind: CronJob
metadata:
  name: project-cleanup
  namespace: project
spec:
  jobTemplate:
    spec:
      template:
        metadata:
        spec:
          containers:
          - image: new-cron-container-image
          - image: new-cron-container-image
`,
			filter: ImagePrefixFn{
				Metadata: v1alpha1.Metadata{
					Name: "cronjob-image-replace",
				},
				Spec: Spec{
					ImageMappings: []ImageMapping{
						{
							Src:  "old-cron-container-image",
							Dest: "new-cron-container-image",
						},
					},
				},
			},
		},
		"redis-stand-alone": {
			input: `
apiVersion: redis.redis.opstreelabs.in/v1beta1
kind: Redis
metadata:
  name: engines-redis
  namespace: engines
spec:
  kubernetesConfig:
    image: redis-image-old
  redisExporter:
    image: redis-image-old

`,
			expectedOutput: `
apiVersion: redis.redis.opstreelabs.in/v1beta1
kind: Redis
metadata:
  name: engines-redis
  namespace: engines
spec:
  kubernetesConfig:
    image: redis-image-new
  redisExporter:
    image: redis-image-new

`,
			filter: ImagePrefixFn{
				Metadata: v1alpha1.Metadata{
					Name: "redis-standalone-image-replace",
				},
				Spec: Spec{
					ImageMappings: []ImageMapping{
						{
							Src:  "redis-image-old",
							Dest: "redis-image-new",
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
				strings.TrimSpace(filtertest.RunFilter(t, tc.expectedOutput, &null.Filter{})),
				strings.TrimSpace(filtertest.RunFilter(t, tc.input, filter))) {
				t.FailNow()
			}
		})
	}
}
