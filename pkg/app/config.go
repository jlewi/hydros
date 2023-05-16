package app

import (
	"github.com/jlewi/hydros/pkg/files"
	"github.com/palantir/go-githubapp/githubapp"
	"github.com/pkg/errors"
	"io"
)

// Config is the configuration for the Hydros application
// For an example see here:
// https://github.com/palantir/go-githubapp/blob/6c13ccc79f901e0c04c7530df9c775a30347f3cd/example/config.yml
type Config struct {
	Server HTTPConfig `yaml:"server"`

	// TODO(jeremy): N.B. we won't actually use the webhookSecret here because we will load it from GCP
	// TODO(jeremy): We don't actually use this value
	// Github githubapp.Config `yaml:"github"`

	AppConfig HydrosConfig `yaml:"app_configuration"`
}

type HTTPConfig struct {
	Address string `yaml:"address"`
	Port    int    `yaml:"port"`
}

// HydrosConfig is configuration specific to hydros
type HydrosConfig struct {
	// WebhookSecretURI allows the webhook secret to be provided as a URI
	// e.g. gcpSecretManager:///projects/${PROJECT}/secrets/${SECRET}/versions/latest
	WebhookSecretURI string `yaml:"webhook_secret_uri"`
}

type App struct {
	IntegrationID int64  `yaml:"integration_id" json:"integrationId"`
	WebhookSecret string `yaml:"webhook_secret" json:"webhookSecret"`
	PrivateKey    string `yaml:"private_key" json:"privateKey"`
}

// BuildConfig is a helper function to build the configuration
func BuildConfig(appId int64, webhookSecret string, privateKeySecret string) (*githubapp.Config, error) {
	hmacSecret, err := readSecret(webhookSecret)
	if err != nil {
		return nil, errors.Wrapf(err, "Error reading webhook secret %s", webhookSecret)
	}

	privateKey, err := readSecret(privateKeySecret)
	if err != nil {
		return nil, errors.Wrapf(err, "Error reading private key %s", privateKeySecret)
	}
	config := &githubapp.Config{
		WebURL:   "https://github.com",
		V3APIURL: "https://api.github.com",
		V4APIURL: "https://api.github.com/graphql",
		App: App{
			IntegrationID: appId,
			WebhookSecret: string(hmacSecret),
			PrivateKey:    string(privateKey),
		},
	}
	return config, nil
}

func readSecret(secret string) ([]byte, error) {
	f := &files.Factory{}
	h, err := f.Get(secret)
	if err != nil {
		return nil, err
	}
	r, err := h.NewReader(secret)
	if err != nil {
		return nil, err
	}
	return io.ReadAll(r)
}
