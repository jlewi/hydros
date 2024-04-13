package e2etests

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/jlewi/hydros/pkg/app"
	"github.com/jlewi/hydros/pkg/gitops"

	"github.com/jlewi/hydros/api/v1alpha1"
	"sigs.k8s.io/kustomize/kyaml/yaml"
)

// This test isn't in the gitops package because we want to use the app package to load the registry
// and this would create a circular dependency if it lived in the gitops package

func Test_repoController(t *testing.T) {
	if os.Getenv("GITHUB_ACTIONS") != "" {
		t.Skip("Skipping test because running in GHA")
	}

	hydrosApp := app.NewApp()
	if err := hydrosApp.LoadConfig(nil); err != nil {
		t.Fatalf("LoadConfig failed: %v", err)
	}

	if err := hydrosApp.SetupLogging(); err != nil {
		t.Fatalf("SetupLogging failed: %v", err)

	}

	if err := hydrosApp.SetupRegistry(); err != nil {
		t.Fatalf("SetupRegistry failed: %v", err)
	}

	cwd, err := os.Getwd()
	if err != nil {
		t.Errorf("Getwd failed: %v", err)
	}

	repoFile := filepath.Join(cwd, "test_data", "repo.yaml")
	f, err := os.Open(repoFile)
	if err != nil {
		t.Fatalf("os.Open(%v) failed: %v", repoFile, err)
	}
	repo := &v1alpha1.RepoConfig{}
	if err := yaml.NewDecoder(f).Decode(&repo); err != nil {
		t.Fatalf("yaml decode failed: %v", err)
	}

	// N.B. To use the same work directory and avoid checking out repos multiple times configure the workDir
	// in your hydros config
	c, err := gitops.NewRepoController(*hydrosApp.Config, hydrosApp.Registry, repo)
	if err != nil {
		t.Errorf("NewRepoController failed: %+v", err)
	}

	if err := c.Reconcile(context.Background()); err != nil {
		t.Errorf("Reconcile failed: %v", err)
	}
}
