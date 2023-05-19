package v1alpha1

import (
	"fmt"

	"github.com/pkg/errors"
)

type (
	// Strategy is an enum for the strategy to use to tag images.
	Strategy string
	// RepoMatchType is an enum for how repo matching rules should be applied.
	RepoMatchType string
)

const (
	// ManifestSyncKind is the kind for ManifestSync resources.
	ManifestSyncKind = "ManifestSync"

	// SourceCommitStrategy indicates the source image should have a tag
	// equal to the source commit tag.
	SourceCommitStrategy Strategy = "sourceCommit"
	// MutableTagStrategy means you should pin to whatever image has that commit.
	MutableTagStrategy Strategy = "mutableTag"

	// UnknownStrategy indicates unknown tag matching strategy.
	UnknownStrategy Strategy = "unknown"

	// LatestTagPrefix means you should look for the most recent image
	// which has that tag as a prefix.
	LatestTagPrefix Strategy = "latestTagPrefix"

	// IncludeRepo is the enum value indicating a repo list is an include list.
	IncludeRepo RepoMatchType = "include"
	// ExcludeRepo is the enum value indicating a repo list is an exclude list.
	ExcludeRepo RepoMatchType = "exclude"
)

// ManifestSync continually syncs unhyrated manifests to a hydrated repo.
// As part of that images are replaced with the latest images.
type ManifestSync struct {
	APIVersion string   `yaml:"apiVersion" yamltags:"required"`
	Kind       string   `yaml:"kind" yamltags:"required"`
	Metadata   Metadata `yaml:"metadata,omitempty"`

	Spec   ManifestSyncSpec   `yaml:"spec,omitempty"`
	Status ManifestSyncStatus `yaml:"status,omitempty"`
}

// ManifestSyncSpec is the spec for ManifestSync.
type ManifestSyncSpec struct {
	SourceRepo GitHubRepo `yaml:"sourceRepo,omitempty"`
	// ForkRepo is the repo into which the hydrated manifests will be pushed
	ForkRepo GitHubRepo `yaml:"forkRepo,omitempty"`
	// DestRepo is the repo into which a PR will be created to merge hydrated
	// manifests from the ForkRepo
	DestRepo GitHubRepo `yaml:"destRepo,omitempty"`

	// SourcePath is relative to root of SourceRepo. It is the director
	// to search for manifests to hydrate
	SourcePath string `yaml:"sourcePath,omitempty"`

	// Selector selects which kustomizations to be used by matching kustomize labels.
	Selector *LabelSelector `yaml:"selector,omitempty"`

	// Only kustomizations which include these annotations as commonAnnotations will be hydrated.
	// Deprecated; selector should be used instead.
	MatchAnnotations map[string]string `yaml:"matchAnnotations,omitempty"`

	// DestPath is the directory in the destination repo where hydrated manifests should be emitted.
	// This directory will be deleted and recreated for each PR to ensure pruned resources are removed
	DestPath string `yaml:"destPath,omitempty"`

	// ImageTagsToPin is a list of image tags whose images should be pinned.
	// It is a replacement for ImageTags.
	ImageTagsToPin []ImageTagToPin `yaml:"imageTagsToPin,omitempty"`

	// ImageRegistries is a list of image registries. Only images in these registries with one of
	// the ImageTags labeled above is eligible for replacement.
	ImageRegistries []string `yaml:"imageRegistries,omitempty"`

	// ImageBuilder configures the image building.
	ImageBuilder *ImageBuilder `yaml:"imageBuilder,omitempty"`

	// ExcludeDirs is a list of paths relative to the repo root exclude. This is typically directories that
	// store templates. These directories will not be considered at all; e.g.
	//  1. Manifests are not eligible for image replacement
	//  2. Manifests are not eligible for hydration
	// If you need to #1 but not #2 use MatchAnnotations to exclude a kustomization from hydration but not image
	// replacement.
	ExcludeDirs []string `yaml:"excludeDirs,omitempty"`

	// PrLabels is a list of labels to add to the PR.
	PrLabels []string `yaml:"prLabels,omitempty"`

	// Functions is a list of kustomize functions to apply to the hydrated manifests
	Functions []Function `yaml:"functions,omitempty"`
}

// GitHubRepo represents a GitHub repo.
type GitHubRepo struct {
	// Org that owns the repo.
	Org    string `yaml:"org,omitempty"`
	Repo   string `yaml:"repo,omitempty"`
	Branch string `yaml:"branch,omitempty"`
}

// ManifestSyncStatus is the status for ManifestSync resources.
type ManifestSyncStatus struct {
	SourceURL    string        `yaml:"sourceUrl,omitempty"`
	SourceCommit string        `yaml:"sourceCommit,omitempty"`
	PinnedImages []PinnedImage `yaml:"pinnedImages,omitempty"`
}

// PinnedImage represents the mapping of an image to the value it should be pinned to.
type PinnedImage struct {
	Image    string `yaml:"image,omitempty"`
	NewImage string `yaml:"newImage,omitempty"`
}

// ImageTagToPin describes an image tag to pin.
type ImageTagToPin struct {
	// Tags of the images to look for to pin.
	Tags []string `yaml:"tags,omitempty"`
	// Strategy is an enum indicating how the image should be pinned.
	Strategy Strategy `yaml:"strategy,omitempty"`

	// ImageRepoMatch describes the image repos to match
	// If nil all repos are matched.
	ImageRepoMatch *ImageRepoMatch `yaml:"imageRepoMatch,omitempty"`
}

// ImageRepoMatch describes how to match repos.
type ImageRepoMatch struct {
	Repos []string `yaml:"repos,omitempty"`
	// RepoMatchType indicates whether this is an include or exclude list.
	Type RepoMatchType `yaml:"type,omitempty"`
}

// ImageBuilder configures the image builder.
type ImageBuilder struct {
	// Enabled is a boolean indicating whether the image builder is enabled or not
	Enabled bool `yaml:"enabled,omitempty"`
	// Registry is the registry to use with the images.
	Registry string `yaml:"registry,omitempty"`
}

// Function is a list of functions to apply
type Function struct {
	// RepoKey can be source or dest and indicates whether the functions are sourced from the source
	// or dest repo.
	RepoKey string `yaml:"repoKey,omitempty"`
	// Paths is the path to the YAML files or directories to apply.
	Paths []string `yaml:"paths,omitempty"`
}

// IsValid verifies this is a fully valid manifest
func (m *ManifestSync) IsValid() error {
	if m.Metadata.Name == "" {
		return fmt.Errorf("ManifestSync must include a name")
	}

	if (m.Spec.MatchAnnotations == nil || len(m.Spec.MatchAnnotations) == 0) && m.Spec.Selector == nil {
		return fmt.Errorf("ManifestSync.Spec must include matchAnnotations or Selector")
	}

	if m.Spec.Selector != nil {
		if len(m.Spec.Selector.MatchLabels) == 0 && len(m.Spec.Selector.MatchExpressions) == 0 {
			return fmt.Errorf("ManifestSync.Spec.Selector must include matchLabels or MatchExpressions")
		}
	}

	for key, r := range map[string]GitHubRepo{"ForkRepo": m.Spec.ForkRepo, "SourceRepo": m.Spec.SourceRepo, "DestRepo": m.Spec.DestRepo} {
		if err := r.IsValid(); err != nil {
			return errors.Wrapf(err, "ManifestSync has invalid %v", key)
		}
	}

	for _, s := range m.Spec.ImageTagsToPin {
		if s.Strategy == "" {
			return fmt.Errorf("ManifestSync.Spec.ImageTagsToPin must specify a strategy; %v", s)
		}
	}
	return nil
}

// IsValid checks if this is a valid resource.
func (r *GitHubRepo) IsValid() error {
	if r.Org == "" {
		return fmt.Errorf("Org must be specified")
	}
	if r.Repo == "" {
		return fmt.Errorf("Repo must be specified")
	}
	if r.Branch == "" {
		return fmt.Errorf("Branch must be specified")
	}
	return nil
}
