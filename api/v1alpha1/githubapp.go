package v1alpha1

// GitHubAppConfig specifies the configuration for a GitHub App.
type GitHubAppConfig struct {
	// AppID is the github ghapp id for the ghapp
	AppID int64 `json:"appID,omitempty" yaml:"appID,omitempty"`

	// PrivateKey is the URI of the private key for the ghapp. This can be a secretmanager URI e.g.
	PrivateKey string `json:"privateKey,omitempty" yaml:"privateKey,omitempty"`
}
