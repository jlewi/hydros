package v1alpha1

import "k8s.io/apimachinery/pkg/runtime/schema"

var (
	GitHubReleaserGVK = schema.FromAPIVersionAndKind(Group+"/"+Version, "GitHubReleaser")
)

// GitHubReleaser continuously cuts GitHub releases when conditions are
// met. It takes care of setting the release notes and the version.
type GitHubReleaser struct {
	APIVersion string             `yaml:"apiVersion" yamltags:"required"`
	Kind       string             `yaml:"kind" yamltags:"required"`
	Metadata   Metadata           `yaml:"metadata,omitempty"`
	Spec       GitHubReleaserSpec `yaml:"spec,omitempty"`
}

type GitHubReleaserSpec struct {
	Org string `yaml:"org,omitempty"`
	// Repo is the repository to release
	Repo string `yaml:"repo,omitempty"`

	// TODO(jeremy): We should probably support branch
}
