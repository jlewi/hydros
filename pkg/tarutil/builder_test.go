package tarutil

import (
	"archive/tar"
	"compress/gzip"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/google/go-cmp/cmp"

	"github.com/go-logr/zapr"
	"github.com/jlewi/hydros/api/v1alpha1"
	"github.com/jlewi/hydros/pkg/util"
	"github.com/pkg/errors"
	"go.uber.org/zap"
	"gopkg.in/yaml.v3"
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

	type testCase struct {
		name     string
		srcSpec  string
		basePath string
		expected []string
	}

	cases := []testCase{
		{
			// This test case sets the base path to dirA and then we check that we can include
			// the parent directory
			name:     "test-relative-paths",
			srcSpec:  filepath.Join(cwd, "test_data", "image.yaml"),
			basePath: filepath.Join(cwd, "test_data", "dirA"),
			expected: []string{
				"file1.txt",
				"dirB/file2.txt",
			},
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			f, err := os.Open(c.srcSpec)
			if err != nil {
				t.Fatalf("Error opening spec %v", err)
			}

			image := &v1alpha1.Image{}
			if err := yaml.NewDecoder(f).Decode(image); err != nil {
				t.Fatalf("Error decoding image %v", err)
			}

			oFile := filepath.Join(tDir, "test.tar.gz")
			if err := Build(image, c.basePath, oFile); err != nil {
				t.Fatalf("Error building tarball for image %+v", err)
			}

			t.Logf("Tarball written to %v", oFile)

			manifest, err := readTarball(oFile)
			if err != nil {
				t.Fatalf("Error reading tarball %v", err)
			}

			missing := []string{}

			for _, e := range c.expected {
				if _, ok := manifest[e]; !ok {
					missing = append(missing, e)
				}
			}

			if len(missing) > 0 {
				t.Errorf("Missing files %v", missing)
			}
		})
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

		manifest[header.Name] = true
	}

	return manifest, nil
}

func Test_matchGlob(t *testing.T) {
	type testCase struct {
		files    []string
		glob     string
		expected []string
	}

	cases := []testCase{
		{
			files: []string{
				"pkg/app/app.go",
				"pkg/app/text.tmpl",
			},
			glob: "**/*",
			expected: []string{
				"pkg",
				"pkg/app",
				"pkg/app/app.go",
				"pkg/app/text.tmpl",
			},
		},
		// Test ".." in a pattern
		{
			files: []string{
				"pkg/app/app.go",
				"pkg/app/text.tmpl",
				"pkg/b/file2.go",
			},
			glob: "pkg/b/../**/*",
			expected: []string{
				"pkg/app",
				"pkg/b",
				"pkg/app/app.go",
				"pkg/app/text.tmpl",
				"pkg/b/file2.go",
			},
		},
	}

	for i, c := range cases {
		t.Run(fmt.Sprintf("case-%v", i), func(t *testing.T) {
			tDir, err := os.MkdirTemp("", "")
			if err != nil {
				t.Fatalf("Error creating temp dir %v", err)
			}
			t.Logf("Created temp dir %v", tDir)
			// Create the files
			for _, f := range c.files {
				fullPath := filepath.Join(tDir, f)
				dirname := filepath.Dir(fullPath)
				if err := os.MkdirAll(dirname, 0755); err != nil {
					t.Fatalf("Error creating directory %v", dirname)
				}

				if err := os.WriteFile(fullPath, []byte("foo"), 0644); err != nil {
					t.Fatalf("Error writing file %v", fullPath)
				}
			}

			fs := os.DirFS(tDir)
			actual, err := matchGlob(fs, c.glob)
			if err != nil {
				t.Fatalf("Error matching glob %v", err)
			}
			if d := cmp.Diff(c.expected, actual); d != "" {
				t.Errorf("Unexpected result (-want +got):\n%s", d)
			}
		})
	}
}

func Test_splitParent(t *testing.T) {
	type testCase struct {
		input  string
		parent string
		glob   string
	}

	cases := []testCase{
		{
			input:  "**/*.go",
			parent: "",
			glob:   "**/*.go",
		},
		{
			input:  "../../**/*.go",
			parent: "../..",
			glob:   "**/*.go",
		},
	}

	for i, c := range cases {
		t.Run(fmt.Sprintf("case-%v", i), func(t *testing.T) {
			parent, glob := splitIntoParent(c.input)
			if parent != c.parent {
				t.Errorf("Expected parent %v; got %v", c.parent, parent)
			}
			if glob != c.glob {
				t.Errorf("Expected glob %v; got %v", c.glob, glob)
			}
		})
	}
}
