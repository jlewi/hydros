package v1alpha1

import "strings"

var (
	RepoGVK = Gvk{
		Group:   Group,
		Version: Version,
		Kind:    "RepoConfig",
	}
)

// RepoConfig specifies a repository that should be checked out and periodically sync'd.
type RepoConfig struct {
	APIVersion string   `yaml:"apiVersion"`
	Kind       string   `yaml:"kind"`
	Metadata   Metadata `yaml:"metadata"`
	Spec       RepoSpec `yaml:"spec"`
}

// RepoSpec is the spec for a repository to synchronize
type RepoSpec struct {
	// Repo is the URI of the repository to use.
	// You can specify a branch using the ref parameter specifies the reference to checkout
	// https://github.com/hashicorp/go-getter#protocol-specific-options
	Repo string `yaml:"repo"`
	// GitHubAppConfig is the configuration for the GitHub App to use to access the repo.
	GitHubAppConfig GitHubAppConfig `yaml:"gitHubAppConfig"`

	// Globs is a list of globs to search for resources to sync.
	Globs []string `yaml:"globs"`
}

// IsValid returns true if the config is valid.
// For invalid config the string will be a message of validation errors
func (c *RepoConfig) IsValid() (string, bool) {
	errors := make([]string, 0, 10)

	if c.Spec.Repo == "" {
		errors = append(errors, "Repo must be specified")
	}

	if !strings.HasPrefix(c.Spec.Repo, "https://") {
		// We use https because we are using a GitHub App
		errors = append(errors, "Repo must be an https URL; currently only https is supported for cloning repositories")
	}

	if c.Spec.GitHubAppConfig.AppID == 0 {
		errors = append(errors, "GitHubAppConfig.AppID must be specified and non-zero")
	}

	if c.Spec.GitHubAppConfig.PrivateKey == "" {
		errors = append(errors, "GitHubAppConfig.PrivateKey is required")
	}

	if len(errors) > 0 {
		return "RepoConfig is invalid. " + strings.Join(errors, ". "), false
	}
	return "", true
}
