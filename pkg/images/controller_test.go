package images

import (
	"github.com/jlewi/hydros/pkg/util"
	"os"
	"path/filepath"
	"testing"
)

func Test_Controller(t *testing.T) {
	util.SetupLogger("info", true)

	if os.Getenv("GITHUB_ACTIONS") != "" {
		t.Skipf("Test_Build is a manual test that is skipped in CICD because it requires GCB")
	}

	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("Error getting working directory %v", err)
	}

	srcSpec := filepath.Join(cwd, "..", "..", "images.yaml")

	if err := ReconcileFile(srcSpec); err != nil {
		t.Fatalf("Error reconciling file %v", err)
	}
}
