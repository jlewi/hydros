package ai

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/PullRequestInc/go-gpt3"
	"github.com/google/go-cmp/cmp"
	"github.com/jlewi/hydros/api/v1alpha1"
	"github.com/jlewi/hydros/pkg/kustomize/fns/ai/openai"
	"github.com/jlewi/hydros/pkg/util"
	"github.com/pkg/errors"
	"sigs.k8s.io/kustomize/kyaml/kio"
	"sigs.k8s.io/kustomize/kyaml/kio/kioutil"
	"sigs.k8s.io/kustomize/kyaml/yaml"
)

// N.B kustomize/ai_e2e_test.go provides an E2E test using the dispatcher.
// Its not in this directory to avoid circular imports

func Test_BuildPrompt(t *testing.T) {
	currDir, err := os.Getwd()
	if err != nil {
		t.Fatalf("Failed to get current working directory; error %v", err)
	}

	specFile := filepath.Join(currDir, "test_data", "workload_identity.yaml")
	spec, err := util.ReadYaml(specFile)
	if err != nil {
		t.Fatalf("Failed to read spec file %v; error %v", specFile, err)
	}

	prompt, err := buildPrompt(spec)
	if err != nil {
		t.Fatalf("Failed to build prompt; error %v", err)
	}
	expected := `You are a Kubernetes and Google Cloud expert. Your speciality is in generating
YAML definitions of resources using the Kubernetes Resource Model. Your job is to translate natural language
descriptions of infrastructure into the corresponding YAML definitions. In addition to the built in resources
you can also use custom resources. Included below is a list of openapi schemas for custom resources. Each
schema defines a single resource and is encoded in JSON one per line.

--- Begin schemas ---
{"kind":"HydrosAI","metadata":{"name":"Inflate hydros AI annotaions"},"spec":{"filterSpecs":[{"components":{"schemas":{"WorkloadIdentity":{"properties":{"spec":{"description":"The spec provides the high level API for the workload resources.","properties":{"gsa":{"properties":{"create":{"description":"Whether the google service account should be created if it doesn't exist","type":"boolean"},"iamBindings":{"description":"A list of the GCP iam roles that should be assigned to the GSA if they aren't already. For a list of roles refer to https://cloud.google.com/iam/docs/understanding-roles","items":{"type":"string"},"type":"array"},"name":{"description":"The name of the google service account to bind to the kubernetes service account","type":"string"}},"type":"object"},"ksa":{"properties":{"create":{"description":"Whether the kubernetes service account should be created if it doesn't exist","type":"boolean"},"name":{"description":"The name of the kubernetes service account to bind to the Google service account","type":"string"}},"type":"object"},"requirement":{"description":"This should be a natural language description of what this WorkloadIdentity is doing; for example \"Create a KSA foo bound to GSA dev@acme.com with cloud storage permissions\"","type":"string"}},"type":"object"}},"type":"object"}}},"info":{"description":"A high level API for generating the resources needed to enable workload identity on a GKE cluster. The API takes care of creating the Kubernetes and Google service accounts and IAM bindings as needed.","title":"Workload Identity Generator","version":"1.0.0"},"openapi":"3.0.0","paths":{}}]}}
--- End schemas ---
`

	if d := cmp.Diff(expected, prompt); d != "" {
		t.Fatalf("Unexpected prompt; diff %v", d)
	}
}

type TestObject struct {
	Metadata      v1alpha1.Metadata `json:"metadata,omitempty" yaml:"metadata,omitempty"`
	GeneratedFrom string            `json:"generatedFrom,omitempty" yaml:"generatedFrom,omitempty"`
}

// CopyCompleter implements the Completer interface in a manner suitable for testing
func CopyCompleter(prompt string) ([]*yaml.RNode, string, error) {
	o := TestObject{
		GeneratedFrom: prompt,
	}

	data, err := yaml.Marshal(o)
	if err != nil {
		return nil, "", errors.Wrapf(err, "Failed to marshal object")
	}
	input := bytes.NewReader(data)
	reader := kio.ByteReader{
		Reader: input,
		// TODO(jeremy): Do we want to exclude them?
		OmitReaderAnnotations: true,
	}

	nodes, err := reader.Read()
	return nodes, "", err
}

func TestGeneratorFn_Filter(t *testing.T) {
	type testCase struct {
		name     string
		in       []TestObject
		expected []TestObject
	}

	testCases := []testCase{
		{
			name: "basic",
			in: []TestObject{
				{
					Metadata: v1alpha1.Metadata{
						Annotations: map[string]string{
							AnnotationPrefix:       "prompt1",
							kioutil.PathAnnotation: "foo.yaml",
						},
					},
				},
			},
			expected: []TestObject{
				{
					Metadata: v1alpha1.Metadata{
						Labels: map[string]string{},
						Annotations: map[string]string{AnnotationPrefix: "prompt1",
							kioutil.PathAnnotation: "foo.yaml",
						},
					},
				},
				{
					Metadata: v1alpha1.Metadata{
						Annotations: map[string]string{
							kioutil.IndexAnnotation: "0",
							kioutil.PathAnnotation:  "foo_ai_generated.yaml",
							OwnerPrefix:             `{"hash":"ba703f8abdd5b1300c39b95f3892b64a83b98efa6fdd737d5d15b021844a79b7","prompt":"prompt1","response":""}`,
						},
					},
					GeneratedFrom: "prompt1",
				},
			},
		},
		{
			// Make sure we don't add a generated from annotation if one already exists.
			// We verify this by creating a testobject whos generated from value is different from what completer
			// will set so if it gets changed we know completer was invoked and it should have been.
			name: "already_generated",
			in: []TestObject{
				{
					Metadata: v1alpha1.Metadata{
						Annotations: map[string]string{
							AnnotationPrefix:       "prompt1",
							kioutil.PathAnnotation: "foo.yaml",
						},
					},
				},
				{
					Metadata: v1alpha1.Metadata{
						Annotations: map[string]string{
							kioutil.IndexAnnotation: "0",
							kioutil.PathAnnotation:  "foo_ai_generated.yaml",
							OwnerPrefix:             `{"hash":"ba703f8abdd5b1300c39b95f3892b64a83b98efa6fdd737d5d15b021844a79b7","prompt":"prompt1","response":""}`,
						},
					},
					GeneratedFrom: "already_generated",
				},
			},
			expected: []TestObject{
				{
					Metadata: v1alpha1.Metadata{
						Labels: map[string]string{},
						Annotations: map[string]string{AnnotationPrefix: "prompt1",
							kioutil.PathAnnotation: "foo.yaml",
						},
					},
				},
				{
					Metadata: v1alpha1.Metadata{
						Labels: map[string]string{},
						Annotations: map[string]string{
							kioutil.IndexAnnotation: "0",
							kioutil.PathAnnotation:  "foo_ai_generated.yaml",
							OwnerPrefix:             `{"hash":"ba703f8abdd5b1300c39b95f3892b64a83b98efa6fdd737d5d15b021844a79b7","prompt":"prompt1","response":""}`,
						},
					},
					GeneratedFrom: "already_generated",
				},
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			generator := GeneratorFn{
				completer: CopyCompleter,
				apiKey:    "fakekey",
			}

			in := make([]*yaml.RNode, 0, len(tc.in))
			for _, o := range tc.in {
				b, err := yaml.Marshal(o)
				if err != nil {
					t.Fatalf("Failed to marshal object; error %v", err)
				}

				r, err := yaml.Parse(string(b))
				if err != nil {
					t.Fatalf("Failed to parse object; error %v", err)
				}
				in = append(in, r)
			}

			nodes, err := generator.Filter(in)
			if err != nil {
				t.Fatalf("Filter returned error %v", err)
			}

			actual := make([]TestObject, 0, len(nodes))
			for _, n := range nodes {
				node := n.YNode()
				o := &TestObject{}
				if err := node.Decode(o); err != nil {
					t.Fatalf("Failed to decode node; error %v", err)
				}
				actual = append(actual, *o)
			}

			if d := cmp.Diff(tc.expected, actual); d != "" {
				t.Fatalf("Unexpected output; diff %v", d)
			}
		})
	}
}

func Test_Prompt(t *testing.T) {
	if os.Getenv("GITHUB_ACTIONS") != "" {
		t.Skip("Skipping test on GitHub actions; this is an integration test that requires access to OpenAI")
	}

	currDir, err := os.Getwd()
	if err != nil {
		t.Fatalf("Failed to get current working directory; error %v", err)
	}

	specsDir := filepath.Join(currDir, "test_data")
	system, err := buildPromptFromDirs([]string{specsDir})
	if err != nil {
		t.Fatalf("Failed to build prompt; error %v", err)
	}
	apiKey := openai.GetAPIKey()
	if apiKey == "" {
		t.Fatalf("API key must be set")
	}

	gClient := gpt3.NewClient(string(apiKey), gpt3.WithTimeout(1*time.Minute))

	req := gpt3.ChatCompletionRequest{
		Model: "gpt-3.5-turbo",
		Messages: []gpt3.ChatCompletionRequestMessage{
			{
				Role:    "system",
				Content: system,
			},
			{
				Role:    "user",
				Content: "Use workload identity to bind the Kubernetes service account jupyter to the GCP service account dev@acme.com and grant it cloud storage editor and bigquery editor permissions",
			},
		},
	}

	resp, err := gClient.ChatCompletion(context.Background(), req)
	if err != nil {
		t.Fatalf("ChatCompletion failed; error %v", err)
	}

	t.Logf("Result:\n%v", util.PrettyString(resp))
}
