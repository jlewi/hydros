package app

import "github.com/jlewi/hydros/api/v1alpha1"

const (
	// SanitizeKind is the kind for SanitizeKind resources.
	SanitizeKind = "Sanitize"
)

// Sanitize describes a bunch of transformations to be applied to prepare a directory for open source.
type Sanitize struct {
	APIVersion string            `yaml:"apiVersion" yamltags:"required"`
	Kind       string            `yaml:"kind" yamltags:"required"`
	Metadata   v1alpha1.Metadata `yaml:"metadata,omitempty"`

	// List of directories and files to remove.
	Remove []string `yaml:"remove,omitempty"`

	// UnsafeRegex is a list of regular expressions matching strings that shouldn't be allowed in the open
	// sourced files.
	UnsafeRegex []string `yaml:"unsafeRegex,omitempty"`

	// Replacement is a list of regexes to find and replace
	Replacements []Replacement `yaml:"replacements,omitempty"`
}

// Replacement is a string to match and replace.
type Replacement struct {
	Find    string `yaml:"find,omitempty"`
	Replace string `yaml:"replace,omitempty"`
	// Glob expression used to match files
	FileGlob string `yaml:"fileGlob,omitempty"`
}
