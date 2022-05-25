package s3assets

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/suite"
	ktypes "sigs.k8s.io/kustomize/api/types"
	"sigs.k8s.io/kustomize/kyaml/resid"
	"sigs.k8s.io/kustomize/kyaml/yaml"
)

type S3BucketTransformTestSuite struct {
	suite.Suite
}

func TestS3BucketTransformSuite(t *testing.T) {
	suite.Run(t, &S3BucketTransformTestSuite{})
}

func (s *S3BucketTransformTestSuite) TestYamlLoad() {
	testCases := map[string]struct {
		yaml     string
		expected Spec
	}{
		"simple": {
			yaml: strings.TrimSpace(`
apiVersion: v1alpha1
kind: S3BucketTransformer
spec:
  bucket: test-bucket
  fieldSpecs:
    - path: data
      version: v1
      kind: ConfigMap
`),
			expected: Spec{
				Bucket: "test-bucket",
				FieldSpecs: ktypes.FsSlice{
					{
						Gvk:  resid.Gvk{Version: "v1", Kind: "ConfigMap"},
						Path: "data",
					},
				},
			},
		},

		"empty spec": {
			yaml: strings.TrimSpace(`
apiVersion: v1alpha1
kind: S3BucketTransformer
`),
			expected: Spec{
				Bucket:     "",
				FieldSpecs: nil,
			},
		},
	}
	for name, tc := range testCases {
		s.Run(name, func() {
			fnStruct := &BucketTransformer{}
			s.Assert().NoError(yaml.MustParse(tc.yaml).YNode().Decode(fnStruct))
			s.Assert().Equal(tc.expected, fnStruct.Spec)
		})
	}
}

func (s *S3BucketTransformTestSuite) TestFilter() {
	testCases := map[string]struct {
		yaml     string
		spec     Spec
		expected string
	}{
		"configmap": {
			yaml: `
apiVersion: v1
kind: ConfigMap
data:
  prefix-config-file.json: |
    {
       "DEFAULT_BUCKET_S3_PATH": "s3://prefix-models",
       "SIF": {"weights_latest": "s3://prefix-models/titles.npy"}
    }
`,
			spec: Spec{
				Bucket: "test-bucket",
			},
			expected: `
apiVersion: v1
kind: ConfigMap
data:
  prefix-config-file.json: |
    {
       "DEFAULT_BUCKET_S3_PATH": "s3://test-bucket/prefix-models",
       "SIF": {"weights_latest": "s3://test-bucket/prefix-models/titles.npy"}
    }
`,
		},


		"non default": {
			yaml: `
apiVersion: fake
kind: FakeKind
spec:
  unknown:
  - path:
      s3Uri: s3://bucket-blah/fake/testing/key/model.zip
  - path:
      s3Uri: s3://bucket-blah/fake/testing/other/model.zip
`,
			spec: Spec{
				Bucket: "test-bucket",
				FieldSpecs: ktypes.FsSlice{
					{
						Gvk:  resid.Gvk{Version: "fake", Kind: "FakeKind"},
						Path: "spec/unknown/path/s3Uri",
					},
				},
			},
			expected: `
apiVersion: fake
kind: FakeKind
spec:
  unknown:
  - path:
      s3Uri: s3://test-bucket/bucket-blah/fake/testing/key/model.zip
  - path:
      s3Uri: s3://test-bucket/bucket-blah/fake/testing/other/model.zip
`,
		},

		"do not transform": {
			yaml: `
apiVersion: fake
kind: FakeKind
spec:
  unknown:
  - path:
      s3Uri: s3:/not-enough-slashes
  - untouched:
      s3Uri: s3://bucket-blah/fake/testing/key/model.zip
`,
			spec: Spec{
				Bucket: "test-bucket",
				FieldSpecs: ktypes.FsSlice{
					{
						Gvk:  resid.Gvk{Version: "fake", Kind: "FakeKind"},
						Path: "spec/unknown/path/s3Uri",
					},
				},
			},
			expected: `
apiVersion: fake
kind: FakeKind
spec:
  unknown:
  - path:
      s3Uri: s3:/not-enough-slashes
  - untouched:
      s3Uri: s3://bucket-blah/fake/testing/key/model.zip
`,
		},
	}
	for name, tc := range testCases {
		s.Run(name, func() {
			node := yaml.MustParse(tc.yaml)
			bt := BucketTransformer{Spec: tc.spec}
			transformed, err := bt.Filter([]*yaml.RNode{node})
			s.Assert().NoError(err)
			newYaml, err := transformed[0].String()
			s.Assert().NoError(err)
			s.Assert().Equal(strings.TrimSpace(tc.expected), strings.TrimSpace(newYaml))
		})
	}
}
