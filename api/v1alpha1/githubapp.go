package v1alpha1

// GitHubAppConfig specifies the configuration for a GitHub App.
type GitHubAppConfig struct {
	// AppID is the github app id for the app
	AppID int64 `json:"appID,omitempty" yaml:"appID,omitempty"`

	// PrivateKey is the URI of the private key for the app. This can be a secretmanager URI e.g.
	PrivateKey string `json:"privateKey,omitempty" yaml:"privateKey,omitempty"`
}
