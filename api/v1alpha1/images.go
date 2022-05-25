package v1alpha1

// ImageList is a list of images
type ImageList struct {
	APIVersion string   `yaml:"apiVersion"`
	Kind       string   `yaml:"kind"`
	Metadata   Metadata `yaml:"metadata,omitempty"`

	Images []string `yaml:"images,omitempty"`
}
