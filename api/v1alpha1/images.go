package v1alpha1

import "k8s.io/apimachinery/pkg/runtime/schema"

var (
	ImageGVK = schema.FromAPIVersionAndKind(Group+"/"+Version, "URI")
)

// ImageList is a list of images
type ImageList struct {
	APIVersion string   `yaml:"apiVersion"`
	Kind       string   `yaml:"kind"`
	Metadata   Metadata `yaml:"metadata,omitempty"`

	Images []string `yaml:"images,omitempty"`
}

type ImageSourceType string

const (
	// ImageSourceTypeDocker is a docker image
	ImageSourceTypeDocker ImageSourceType = "docker"
	// ImageSourceTypeDir is a local directory
	ImageSourceTypeDir ImageSourceType = "dir"
)

// Image defines an image to be continuously built
type Image struct {
	APIVersion string   `yaml:"apiVersion" yamltags:"required"`
	Kind       string   `yaml:"kind" yamltags:"required"`
	Metadata   Metadata `yaml:"metadata,omitempty"`

	Spec   ImageSpec   `yaml:"spec,omitempty"`
	Status ImageStatus `yaml:"status,omitempty"`
}

type ImageSpec struct {
	// Image is the full path of the image to be built
	// e.g.us-west1-docker.pkg.dev/dev-sailplane/images/hydros/agent
	// So it includes the registry and repository but not the tag or digest
	Image string `yaml:"image,omitempty"`
	// Source are the source for the image
	Source  []*ImageSource   `yaml:"imageSource,omitempty"`
	Builder *ArtifactBuilder `yaml:"builder,omitempty"`
}

type ImageSource struct {
	// URI is the path of the resource to use as a source
	// This can be a local path or a docker image. If its a local path relative paths will be interpreted
	// relative to the location of the YAML file containing the resource.
	// e.g.us-west1-docker.pkg.dev/dev-sailplane/images/hydros/agent
	//
	// Use file:// to specify a local path e.g. file:///path/to/dir. Note the third "/" indicates its an absolute path
	// If its "//" then its a relative path. I'm not sure it makes sense to support relative paths because what
	// would they be relative to?
	// TODO(jeremy): If the tag isn't specified we should look for the same tag at which the new image is being built
	URI string `yaml:"uri,omitempty"`
	// Type specifies the type of image source
	// TODO(jeremy): Can we just get rid of this and rely on scheme in the URI? e.g. file:/// or docker://
	Type           ImageSourceType  `yaml:"type,omitempty"`
	SourceMappings []*SourceMapping `yaml:"source,omitempty"`
}

// SourceMapping specifies how source files are mapped into the destination artifact
// It is inspired by skaffold; https://skaffold.dev/docs/references/yaml/
// When building images from a YAML file the src is a relative path to the location of the YAML file.
// src can start with a parent prefix e.g. ".." to obtain files higher in the directory tree relative to the
// YAML file. The parent directory wil then be used when computing the destination path.
// e.g. if we have
// /p/a/image.yaml
// /p/b/file.txt
// And image.yaml contains src "../b/file.txt" then the file will be copied to b/file.txt by default in the
// tarball
type SourceMapping struct {
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
	// Project is the GCP project to use for building
	Project string `yaml:"project,omitempty"`

	// Timeout is a string understood by time.ParseDuration
	// e.g. 10m
	Timeout string `yaml:"timeout,omitempty"`

	// Bucket where to store the build logs
	Bucket string `yaml:"bucket,omitempty"
`
	// MachineType is optional. If specified its the machine type to use for building.
	// Increasing VCPU can increase build times but also comes with a provisioning
	// delay since they are only started on demand (they are also more expensive).
	// Private pools could potentially fix the delay cost
	// See: https://cloud.google.com/build/docs/optimize-builds/increase-vcpu-for-builds#increase_vcpu_for_default_pools
	// See: https://cloud.google.com/build/pricing
	// See: https://cloud.google.com/build/docs/api/reference/rest/v1/projects.builds#machinetype
	// For values. UNSPECIFIED uses the default value which has 1 CPU
	MachineType string `yaml:"machineType,omitempty"`
}

type ImageStatus struct {
	// SourceCommit is the commit hash of the source code
	SourceCommit string `yaml:"sourceCommit,omitempty"`
	// URI is the URI of the image
	URI string `yaml:"uri,omitempty"`
	// SHA is the SHA of the image
	SHA string `yaml:"sha,omitempty"`
}
