package ai

import (
	"github.com/google/go-cmp/cmp"
	"github.com/jlewi/hydros/pkg/util"
	"os"
	"path/filepath"
	"testing"
)

func Test_BuildPromt(t *testing.T) {
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

	expected := `You are a Kubernetes and Google Cloud expert. Your speciality is in generating
YAML definitions of resources using the Kubernetes Resource Model. Your job is to translate natural language
descriptions of infrastructure into the corresponding YAML definitions. In addition to the built in resources
you can also use custom resources. Included below is a list of openapi schemas for custom resources. Each
schema defines a single resource and is encoded in JSON one per line.

--- Begin schemas ---
{"components":{"schemas":{"WorkloadIdentity":{"properties":{"spec":{"description":"The spec provides the high level API for the workload resources.","properties":{"gsa":{"properties":{"create":{"description":"Whether the google service account should be created if it doesn't exist","type":"boolean"},"iamBindings":{"description":"A list of the GCP iam roles that should be assigned to the GSA if they aren't already. For a list of roles refer to https://cloud.google.com/iam/docs/understanding-roles","items":{"type":"string"},"type":"array"},"name":{"description":"The name of the google service account to bind to the kubernetes service account","type":"string"}},"type":"object"},"ksa":{"properties":{"create":{"description":"Whether the kubernetes service account should be created if it doesn't exist","type":"boolean"},"name":{"description":"The name of the kubernetes service account to bind to the Google service account","type":"string"}},"type":"object"},"requirement":{"description":"This should be a natural language description of what this WorkloadIdentity is doing; for example \"Create a KSA foo bound to GSA dev@acme.com with cloud storage permissions\"","type":"string"}},"type":"object"}},"type":"object"}}},"info":{"description":"A high level API for generating the resources needed to enable workload identity on a GKE cluster. The API takes care of creating the Kubernetes and Google service accounts and IAM bindings as needed.","title":"Workload Identity Generator","version":"1.0.0"},"openapi":"3.0.0","paths":{}}
--- End schemas ---
`

	if d := cmp.Diff(expected, prompt); d != "" {
		t.Fatalf("Unexpected prompt; diff %v", d)
	}
	t.Logf("Prompt:\n%v", prompt)
}
