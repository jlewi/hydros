package tar

import (
	"github.com/jlewi/hydros/api/v1alpha1"
	"gopkg.in/yaml.v3"
	"os"
	"path/filepath"
	"testing"
)

func Test_Build(t *testing.T) {
	//if os.Getenv("GITHUB_ACTIONS") != "" {
	//	t.Skipf("Test_Build is a manual test that is skipped in CICD because it requires GCB")
	//}

	tDir, err := os.CreateTemp("", "")

	if err != nil {
		t.Fatalf("Error creating temp dir %v", err)
	}

	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("Error getting working directory %v", err)
	}

	srcSpec := filepath.Join(cwd, "..", "..", "images.yaml")

	f, err := os.Open(srcSpec)
	if err != nil {
		t.Fatalf("Error opening spec %v", err)
	}

	image := &v1alpha1.Image{}
	if err := yaml.NewDecoder(f).Decode(image); err != nil {
		t.Fatalf("Error decoding image %v", err)
	}

	oFile := filepath.Join(tDir.Name(), "test.tar.gz")
	if err := Build(image, cwd, oFile); err != nil {
		t.Fatalf("Error building tarball for image %v", err)
	}
}
