package gitops

import (
	"context"
	"github.com/jlewi/hydros/pkg/config"
	"os"
	"path/filepath"
	"testing"

	"github.com/jlewi/hydros/api/v1alpha1"
	"github.com/jlewi/hydros/pkg/util"
	"sigs.k8s.io/kustomize/kyaml/yaml"
)

func Test_repoController(t *testing.T) {
	if os.Getenv("GITHUB_ACTIONS") != "" {
		t.Skip("Skipping test because running in GHA")
	}

	if err := config.InitViper(nil); err != nil {
		t.Fatalf("InitViper failed: %v", err)
	}
	hConfig := config.GetConfig()
	util.SetupLogger("info", true)

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
	c, err := NewRepoController(*hConfig, repo)
	if err != nil {
		t.Errorf("NewRepoController failed: %+v", err)
	}

	if err := c.Reconcile(context.Background()); err != nil {
		t.Errorf("Reconcile failed: %v", err)
	}
}
