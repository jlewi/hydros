package gitutil

import (
	"os"
	"path/filepath"

	"github.com/pkg/errors"
)

// LocateRoot locates the root of the git repository at path.
// Returns empty string if not a git repo.
func LocateRoot(path string) (string, error) {
	for {
		gDir := filepath.Join(path, ".git")
		_, err := os.Stat(gDir)
		if err == nil {
			return path, nil
		}

		if os.IsNotExist(err) {
			path = filepath.Dir(path)
			if path == string(os.PathSeparator) {
				return "", nil
			}
			continue
		}
		return "", errors.Wrapf(err, "Error checking for directory %v", gDir)
	}
}
