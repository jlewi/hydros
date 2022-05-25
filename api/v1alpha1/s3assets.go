package v1alpha1

// S3AssetsList is a list of s3 paths
type S3AssetsList struct {
	APIVersion string   `yaml:"apiVersion"`
	Kind       string   `yaml:"kind"`
	Metadata   Metadata `yaml:"metadata,omitempty"`

	S3Assets []string `yaml:"s3assets,omitempty"`
}
