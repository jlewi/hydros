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
	image := "us-west1-docker.pkg.dev/foyle-public/images/foyle-public/foyle-vscode-ext:latest"

	tarball := filepath.Join(tDir, "foyle.tar")
	if err := ExportImage(image, tarball); err != nil {
		t.Fatalf("Error downloading image %v", err)
	}
	t.Logf("Tarball written to %v", tarball)
}
