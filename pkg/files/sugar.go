package files

import (
	"io"
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
	return io.ReadAll(r)
}
