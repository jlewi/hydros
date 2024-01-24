package gcp

import (
	"os"
	"path/filepath"
	"testing"
)

func Test_BuildImage(t *testing.T) {
	if os.Getenv("GITHUB_ACTIONS") != "" {
		t.Skipf("Test_BuildImage is a manual test that is skipped in CICD because it requires GCB")
	}

	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("Error getting working directory %v", err)
	}

	buildFile := filepath.Join(cwd, "..", "..", "cloudbuild.yaml")

	project := "dev-sailplane"
	sourceCommit := "test-build"
	op, err := BuildImage(buildFile, project, sourceCommit)
	if err != nil {
		t.Fatalf("Error building image %v", err)
	}

	t.Logf("Build operation %v", op.GetName())
}
