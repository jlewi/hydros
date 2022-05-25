package regexp

import (
	"regexp"

	"sigs.k8s.io/kustomize/kyaml/yaml"
)

// SetFn takes a Regexp and a string and returns an error
type SetFn func(*regexp.Regexp, string) error

// SliceFilter is the spec for the kustomize function.
type SliceFilter struct {
	// ReSlice is a slice of regular expressions for string extraction
	ReSlice []string `yaml:"reSlice"`

	// SetValue is called with for each regular expression and RNode as a string
	SetValue SetFn
}

// Filter makes an regexp Extractor out of each regexps in ReSlice runs them all
func (f SliceFilter) Filter(node *yaml.RNode) (*yaml.RNode, error) {
	regexpExtractors := []yaml.Filter{}
	for _, re := range f.ReSlice {
		extractor := &Filter{
			Regexp:   re,
			SetValue: f.SetValue,
		}
		regexpExtractors = append(regexpExtractors, extractor)
	}
	if err := node.PipeE(regexpExtractors...); err != nil {
		return nil, err
	}
	return node, nil
}

// Filter is a kyaml Filter that applies regular expressions
type Filter struct {
	// Regexp a regular expression to match with
	Regexp string `yaml:"regexp"`

	// SetValue is called with the regular expression and RNode as a string
	SetValue SetFn
}

// Filter turns a RNode into a string and creates a Regexp from a string to run SetValue on
func (f *Filter) Filter(node *yaml.RNode) (*yaml.RNode, error) {
	s, err := node.String()
	compiledRegex := regexp.MustCompile(f.Regexp)
	if err != nil {
		return nil, err
	}
	if err := f.SetValue(compiledRegex, s); err != nil {
		return nil, err
	}
	return node, nil
}
