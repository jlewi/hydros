package files

import (
	"bytes"
	"context"
	"io"
	"net/url"

	secretmanager "cloud.google.com/go/secretmanager/apiv1"
	secretmanagerpb "cloud.google.com/go/secretmanager/apiv1/secretmanagerpb"
	"github.com/go-logr/zapr"
	"github.com/pkg/errors"
	"go.uber.org/zap"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

const (
	// SecretManagerScheme URI scheme for gcp secrets. These need to be lowercase.
	SecretManagerScheme = "gcpsecretmanager"
)

// GCPSecretManager implements the interface but for GCP secrets
// URIs should look like gcpsecretmanager:///projects/${project}/secrets/${secret}/versions/${version}
type GCPSecretManager struct {
	Client *secretmanager.Client
}

// NewReader creates a new Reader for local file.
func (h *GCPSecretManager) NewReader(uri string) (io.Reader, error) {
	log := zapr.NewLogger(zap.L())

	if h.Client == nil {
		log.Info("No client set attempting to create default client")
		client, err := secretmanager.NewClient(context.Background())

		if err != nil {
			return nil, err
		}
		h.Client = client
	}

	u, err := url.Parse(uri)
	if err != nil {
		return nil, errors.Wrapf(err, "Couldn't parse URI %v", uri)
	}
	if u.Scheme != SecretManagerScheme {
		return nil, errors.Wrapf(err, "URI %v doesn't have scheme %v", uri, u.Scheme)
	}

	secret := u.Path
	if u.Host == "projects" {
		secret = u.Host + u.Path
	}
	if secret[0:1] == "/" {
		secret = secret[1:]
	}

	accessRequest := &secretmanagerpb.AccessSecretVersionRequest{
		Name: secret,
	}

	ctx := context.Background()

	// Call the API.
	result, err := h.Client.AccessSecretVersion(ctx, accessRequest)
	if err != nil {
		status, ok := status.FromError(err)

		if ok {
			if status.Code() == codes.NotFound {
				log.Info("Secret doesn't exist", "secret", secret)
				return nil, errors.Errorf("secret %v doesn't exist", secret)
			}

			if status.Code() == codes.FailedPrecondition {
				log.Info("here was a problem trying to access the latest version of secret", "secret", secret, "status_message", status.Message())
				return nil, errors.Errorf("There was a problem trying to access the latest version of secret %v", secret)
			}
		}
		return nil, errors.Wrapf(err, "failed to access secret %v", secret)
	}

	return bytes.NewReader(result.Payload.Data), nil
}

// NewWriter creates a new Writer
func (h *GCPSecretManager) NewWriter(uri string) (io.Writer, error) {
	return nil, errors.New("Exists isn't implemented for GCPSecretManager")
}

// Exists checks whether the file exists.
func (h *GCPSecretManager) Exists(uri string) (bool, error) {
	return false, errors.New("Exists isn't implemented for GCPSecretManager")
}
