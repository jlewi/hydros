package tar

import (
	"archive/tar"
	"compress/gzip"
	"github.com/go-logr/zapr"
	"github.com/jlewi/hydros/api/v1alpha1"
	"github.com/jlewi/hydros/pkg/util"
	"github.com/pkg/errors"
	"go.uber.org/zap"
	"gopkg.in/yaml.v3"
	"io"
	"os"
	"path/filepath"
	"testing"
)

func Test_Build(t *testing.T) {
	util.SetupLogger("info", true)

	tDir, err := os.MkdirTemp("", "")

	if err != nil {
		t.Fatalf("Error creating temp dir %v", err)
	}

	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("Error getting working directory %v", err)
	}

	srcSpec := filepath.Join(cwd, "..", "..", "images.yaml")

	basePath := filepath.Dir(srcSpec)

	f, err := os.Open(srcSpec)
	if err != nil {
		t.Fatalf("Error opening spec %v", err)
	}

	image := &v1alpha1.Image{}
	if err := yaml.NewDecoder(f).Decode(image); err != nil {
		t.Fatalf("Error decoding image %v", err)
	}

	oFile := filepath.Join(tDir, "test.tar.gz")
	if err := Build(image, basePath, oFile); err != nil {
		t.Fatalf("Error building tarball for image %+v", err)
	}

	t.Logf("Tarball written to %v", oFile)

	manifest, err := readTarball(oFile)
	if err != nil {
		t.Fatalf("Error reading tarball %v", err)
	}

	// Check a subset of files are in the manifest are in the tarball
	expected := []string{
		"Dockerfile",
		"pkg/util/yaml.go",
	}

	missing := []string{}

	for _, e := range expected {
		if _, ok := manifest[e]; !ok {
			missing = append(missing, e)
		}
	}

	if len(missing) > 0 {
		t.Errorf("Missing files %v", missing)
	}
}

// readTarball reads a tarball and returns a manifest of the contents
func readTarball(srcTarball string) (map[string]bool, error) {
	manifest := make(map[string]bool)

	// Open the tarball file
	file, err := os.Open(srcTarball)
	if err != nil {
		return manifest, errors.Wrapf(err, "Error opening tarball %v", srcTarball)
	}
	defer file.Close()

	gzipReader, err := gzip.NewReader(file)

	if err != nil {
		return manifest, errors.Wrapf(err, "Error creating gzip reader")
	}

	// Create a tar reader
	tarReader := tar.NewReader(gzipReader)

	log := zapr.NewLogger(zap.L())

	// Iterate over each file in the tarball
	for {
		header, err := tarReader.Next()

		if err == io.EOF {
			// Reached the end of the tarball
			return manifest, nil
		}

		if err != nil {
			return manifest, errors.Wrapf(err, "Error reading tar header:")
		}

		log.Info("Reading tarball entry", "header", header.Name, "size", header.Size)

		if header.Size == 0 {
			log.Info("Skipping empty file", "header", header.Name)
			continue
		}

		manifest[header.Name] = true
	}

	return manifest, nil
}
