package util

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"

	"github.com/stretchr/testify/assert"

	"github.com/PrimerAI/hydros-public/api/v1alpha1"
)

// Check the relative path of a path pair to determine if one path is a subdir of the other
func isSubDir(relPath string) bool {
	// No problem if the two paths are the same
	if relPath != "." {
		// If the relative path does not contain a ".." then the path pair contains a child dir
		if !strings.Contains(relPath, "..") {
			return true
		}
	}
	return false
}

// Compute relative path for the pairs (path1, path2) and (path2, path1)
// Check both relative paths for indication that one is a child dir of the other
func pairHasSubDir(path1 string, path2 string) bool {
	// Get relative path for the pair (path1, path2)
	relPath1, _ := filepath.Rel(path1, path2)

	// Get relative path for the pair (path2, path1)
	relPath2, _ := filepath.Rel(path2, path1)

	// Check both relative paths and flag if either is subdir
	return isSubDir(relPath1) || isSubDir(relPath2)
}

func Test_IndependentDestPaths(t *testing.T) {
	// Get all yaml files in the configs dir
	yamlFiles, err := FindYamlFiles("../../configs/")
	assert.NoError(t, err)

	// For each YAML file
	destPaths := []string{}
	for _, yFile := range yamlFiles {
		t.Logf("Processing %v", yFile)
		// Read YAML file into list of resource nodes
		rnodes, err := ReadYaml(yFile)
		assert.NoError(t, err)

		// For each rnode
		for _, rnode := range rnodes {
			// Check the metadata, if Kind is ManifestSync then parse into that struct
			m, err := rnode.GetMeta()
			assert.NoError(t, err)
			if m.Kind == v1alpha1.ManifestSyncKind {
				manifestSync := &v1alpha1.ManifestSync{}

				// N.B. we construct a decoder because we want to enable strict decoding.
				r, err := os.Open(yFile)
				if err != nil {
					t.Errorf("Could not open %v; error %v", yFile, err)
				}
				d := yaml.NewDecoder(r)
				d.KnownFields(true)

				err = d.Decode(manifestSync)
				assert.NoError(t, err, "Failed to parse %v", yFile)

				// Add destPath value to our list of all destPaths to check
				destPaths = append(destPaths, manifestSync.Spec.DestPath)

				// Ensure all the configs are valid
				if err := manifestSync.IsValid(); err != nil {
					assert.NoError(t, err, "%v is not a valid ManifestSync", yFile)
				}
			}
		}
	}

	// Check all pairs of paths
	for i := 0; i < len(destPaths)-1; i++ {
		for j := i + 1; j < len(destPaths); j++ {
			// Verify that neither path in the pair of paths is a child dir of the other
			assert.Equal(
				t,
				pairHasSubDir(destPaths[i], destPaths[j]),
				false,
				fmt.Sprintf("ManifestSync.Spec.DestPath is a child directory of another ManifestSync.Spec.DestPath value, path1: %v, path2: %v", destPaths[i], destPaths[j]),
			)
		}
	}
}
