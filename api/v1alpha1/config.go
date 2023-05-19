package v1alpha1

import "strings"

// HydrosConfig is hydros GitHub App configuration. This is the configuration that should be checked into
// a repository to configure hydros on that repository
type HydrosConfig struct {
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
	// Paths is the relative paths of the directories to search for KRMFunctions
	// If this is blank then the entire repo will be search.
	Paths []string `yaml:"paths"`
}

// IsValid returns true if the config is valid.
// For invalid config the string will be a message of validation errors
func IsValid(c *HydrosConfig) (string, bool) {
	errors := make([]string, 0, 10)
	baseBranches := make(map[string]bool)
	prBranches := make(map[string]bool)

	// Ensure no duplicates and unique prBranches
	for _, c := range c.Spec.InPlaceConfigs {
		if _, ok := baseBranches[c.BaseBranch]; ok {
			errors = append(errors, "Duplicate baseBranch: "+c.BaseBranch)
		}
		if _, ok := baseBranches[c.PRBranch]; ok {
			errors = append(errors, "Duplicate PRBranch: "+c.BaseBranch+"; each baseBranch should use a unique PRBranch")
		}
		baseBranches[c.BaseBranch] = true
		prBranches[c.PRBranch] = true
	}

	if len(errors) > 0 {
		return "HydrosConfig is invalid. " + strings.Join(errors, ". "), false
	}
	return "", true
}
