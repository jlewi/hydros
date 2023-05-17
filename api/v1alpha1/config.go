package v1alpha1

// Config stores the hydros configuration.
type Config struct {
	APIVersion string     `yaml:"apiVersion"`
	Kind       string     `yaml:"kind"`
	Metadata   Metadata   `yaml:"metadata"`
	Spec       ConfigSpec `yaml:"spec"`
}

type ConfigSpec struct {
	// TODO(jeremy): Should we add include/exclude directories to look for ManifestSyncs?

	// InPlaceConfigs configure hydrations that are done in-place. This means the hydrated configurations
	// are checked back into the repository
	InPlaceConfigs []InPlaceConfig `yaml:"inPlaceConfigs"`
}

type InPlaceConfig struct {
	// BaseBranch is the branch to use as the base for the hydration.
	// This will be the branch that is checked out and updated by Hydros
	BaseBranch string `yaml:"baseBranch"`
	// PRBranch is the branch hydros will use to prepare the changes
	PRBranch string `yaml:"prBranch"`
	// AutoMerge determines whether Hydros should try to automatically merge the PR.
	// If AutoMerge is true then Hydros will try to enable GitHub AutoMerge on the PR if it is available
	// or it will try to merge the PR if it is immediately mergeable.
	AutoMerge bool `yaml:"autoMerge"`
}
