package gitops

import (
	"context"
	"github.com/jlewi/hydros/api/v1alpha1"
	"github.com/jlewi/hydros/pkg/util"
	"os"
	"path/filepath"
	"sigs.k8s.io/kustomize/kyaml/yaml"
	"testing"
)

func Test_repoController(t *testing.T) {
	if os.Getenv("GITHUB_ACTIONS") != "" {
		t.Skip("Skipping test because running in GHA")
	}
	
	util.SetupLogger("info", true)

	cwd, err := os.Getwd()
	if err != nil {
		t.Errorf("Getwd failed: %v", err)
	}

	repoFile := filepath.Join(cwd, "test_data", "repo.yaml")
	f, err := os.Open(repoFile)

	repo := &v1alpha1.RepoConfig{}
	if err := yaml.NewDecoder(f).Decode(&repo); err != nil {
		t.Errorf("yaml decode failed: %v", err)
	}

	// Use the same workDir accross tests so we don't have to keep checking it out
	workDir := "/tmp/hydros/repo_controller_test"
	c, err := NewRepoController(repo, workDir)
	if err != nil {
		t.Errorf("NewRepoController failed: %+v", err)
	}

	if err := c.Reconcile(context.Background()); err != nil {
		t.Errorf("Reconcile failed: %v", err)
	}
}
