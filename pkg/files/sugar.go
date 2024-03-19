package files

import (
	"io"

	"github.com/pkg/errors"
)

// Read reads the given URI
func Read(uri string) ([]byte, error) {
	f := &Factory{}
	h, err := f.Get(uri)
	if err != nil {
		return nil, err
	}
	r, err := h.NewReader(uri)
	if err != nil {
		return nil, err
	}
	if r == nil {
		return nil, errors.Errorf("no reader was returned for %v", uri)
	}
	return io.ReadAll(r)
}
