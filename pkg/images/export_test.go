package images

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/jlewi/hydros/pkg/util"
)

func Test_DownloadImage(t *testing.T) {
	if os.Getenv("GITHUB_ACTIONS") != "" {
		t.Skip("Skipping test because running in GHA")
	}

	tDir, err := os.MkdirTemp("", "DownloadImageTest")
	if err != nil {
		t.Fatalf("Error creating temp dir %v", err)
	}

	defer os.RemoveAll(tDir)

	util.SetupLogger("info", true)
	image := "us-west1-docker.pkg.dev/dev-sailplane/images/kubepilot:latest"

	tarball := filepath.Join(tDir, "kubepilot.tar")
	if err := ExportImage(image, tarball); err != nil {
		t.Fatalf("Error downloading image %v", err)
	}
	t.Logf("Tarball written to %v", tarball)
}
