package v1alpha1

// ImageList is a list of images
type ImageList struct {
	APIVersion string   `yaml:"apiVersion"`
	Kind       string   `yaml:"kind"`
	Metadata   Metadata `yaml:"metadata,omitempty"`

	Images []string `yaml:"images,omitempty"`
}

// Image defines an image to be continuously built
type Image struct {
	APIVersion string   `yaml:"apiVersion" yamltags:"required"`
	Kind       string   `yaml:"kind" yamltags:"required"`
	Metadata   Metadata `yaml:"metadata,omitempty"`

	Spec ImageSpec `yaml:"spec,omitempty"`
}

type ImageSpec struct {
	Source  []*Source        `yaml:"source,omitempty"`
	Builder *ArtifactBuilder `yaml:"builder,omitempty"`
}

// Source is a local path to include as an artifact.
// It is inspired by skaffold; https://skaffold.dev/docs/references/yaml/
type Source struct {
	// Src is a glob pattern to match local paths against. Directories should be delimited by / on all platforms.
	// e.g. "css/**/*.css"
	Src string `yaml:"src,omitempty"`
	// Dest is the path to copy the files to in the artifact.
	// e.g. "app"
	Dest string `yaml:"dest,omitempty"`
	// Strip is the path prefix to strip from all paths
	Strip string `yaml:"strip,omitempty"`
}

type ArtifactBuilder struct {
	// GCB is the configuration to build with GoogleCloud Build
	GCB *GCBConfig `yaml:"gcb,omitempty"`
}

// GCBConfig is the configuration for building with GoogleCloud Build
type GCBConfig struct {
	// BuildFile is the path of the cloudbuild.yaml file
	// should be relative to the location of the YAML
	BuildFile string `yaml:"buildFile,omitempty"`
}
