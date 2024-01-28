package images

import (
	"github.com/go-logr/zapr"
	"github.com/jlewi/hydros/api/v1alpha1"
	"github.com/jlewi/hydros/pkg/util"
	"go.uber.org/zap"
	"os"
	"testing"
)

func Test_ImageDownloader(t *testing.T) {
	if os.Getenv("GITHUB_ACTIONS") != "" {
		t.Skip("Skipping test because running in GHA")
	}

	tDir, err := os.MkdirTemp("", "DownloadImageTest")
	if err != nil {
		t.Fatalf("Error creating temp dir %v", err)
	}

	defer os.RemoveAll(tDir)

	log := zapr.NewLogger(zap.L())
	util.SetupLogger("info", true)
	downloader := ImageDownloader{
		Log: log,
		ImageList: v1alpha1.ImageList{
			Images: []string{
				"us-west1-docker.pkg.dev/dev-sailplane/images/kubepilot",
			},
		},
		ImageDir: tDir,
	}

	if err := downloader.DownloadImagesWithRetry(5); err != nil {
		t.Fatalf("Error downloading image %v", err)
	}
	t.Logf("Tarball written to %v", tDir)
}
