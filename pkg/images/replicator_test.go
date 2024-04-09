package images

import (
	"context"
	"github.com/jlewi/hydros/api/v1alpha1"
	"os"
	"testing"
)

func TestReplicator(t *testing.T) {
	if os.Getenv("GITHUB_ACTIONS") != "" {
		t.Skipf("TestReplicator is a manual test that is skipped in CICD")
	}

	replicated := &v1alpha1.ReplicatedImage{
		APIVersion: "",
		Kind:       "",
		Metadata: v1alpha1.Metadata{
			Name:      "foyle-vscode-ext",
			Namespace: "foyle",
		},
		Spec: v1alpha1.ReplicatedImageSpec{
			Source: v1alpha1.ReplicatedImageSource{
				Repository: "us-west1-docker.pkg.dev/foyle-public/images/foyle-vscode-ext",
			},
			Destinations: []string{
				"ghcr.io/jlewi/foyle-vscode-ext",
			},
		},
	}

	r, err := NewReplicator()
	if err != nil {
		t.Fatalf("NewReplicator() = %v, wanted nil", err)
	}

	if err := r.Reconcile(context.Background(), replicated); err != nil {
		t.Fatalf("Reconcile() = %v, wanted nil", err)
	}
}
