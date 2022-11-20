package files

import (
	"io"
)

// TODO(jlewi): We should implement a UnionFileHelper that will delegate to the GcsFileHelper or LocalFileHelper

// FileHelper is an interface intended to transparently handle working with GCS, local files, and other filesystems
// e.g. GCP Secret manager.
type FileHelper interface {
	Exists(path string) (bool, error)
	NewReader(path string) (io.Reader, error)
	NewWriter(path string) (io.Writer, error)
}
