package files

import (
	"net/url"

	"github.com/pkg/errors"
)

// Factory returns the correct filehelper based on a files scheme
type Factory struct{}

func (f *Factory) Get(uri string) (FileHelper, error) {
	u, err := url.Parse(uri)
	if err != nil {
		return nil, errors.Wrapf(err, "Failed to parse URI %v", uri)
	}

	switch u.Scheme {
	case "":
		return &LocalFileHelper{}, nil
	case "file":
		return &LocalFileHelper{}, nil
	case SecretManagerScheme:
		return &GCPSecretManager{}, nil
	default:
		return nil, errors.Errorf("Scheme %v is not supported", u.Scheme)
	}
}
