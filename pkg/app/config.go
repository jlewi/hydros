package app

import (
	"github.com/pkg/errors"
	"os"
	"sigs.k8s.io/kustomize/kyaml/yaml"
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

func ReadConfig(path string) (*Config, error) {
	var c Config

	bytes, err := os.ReadFile(path)
	if err != nil {
		return nil, errors.Wrapf(err, "failed reading server config file: %s", path)
	}

	if err := yaml.Unmarshal(bytes, &c); err != nil {
		return nil, errors.Wrap(err, "failed parsing configuration file")
	}

	return &c, nil
}
