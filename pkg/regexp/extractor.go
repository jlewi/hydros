package regexp

import (
	"regexp"
	"sort"

	"github.com/go-logr/logr"
	"sigs.k8s.io/kustomize/kyaml/kio"
	"sigs.k8s.io/kustomize/kyaml/yaml"

	"github.com/jlewi/hydros/api/v1alpha1"
)

// Extractor extracts all the S3 urls in a directory of manifests
type Extractor struct {
	Log logr.Logger
}

// Extract takes a source directory of manifests and slice of strings used to extract strings and
// returns a list of extracted strings
func (e *Extractor) Extract(sourceDir string, regexps []string) ([]string, error) {
	inputs := kio.LocalPackageReader{PackagePath: sourceDir, MatchFilesGlob: kio.MatchAll}

	r := &regexpReader{
		Metadata: v1alpha1.Metadata{
			Name: "extract",
		},
		Spec:             spec{ReSlice: regexps},
		extractedStrings: map[string]bool{},
	}

	p := kio.Pipeline{
		Inputs:  []kio.Reader{inputs},
		Filters: []kio.Filter{r},
	}

	if err := p.Execute(); err != nil {
		return []string{}, err
	}
	return r.GetExtracted(), nil
}

// regexpReader implements a Filter function
type regexpReader struct {
	// Kind is the API name.
	Kind string `yaml:"kind"`

	// APIVersion is the API version.  Must be examples.kpt.dev/v1alpha1
	APIVersion string `yaml:"apiVersion"`

	// Metadata defines instance metadata.
	Metadata v1alpha1.Metadata `yaml:"metadata"`

	// Spec defines the desired declarative configuration.
	Spec spec `yaml:"spec"`

	extractedStrings map[string]bool `yaml:"extractedStrings"`
}

type spec struct {
	ReSlice []string
}

// GetExtracted returns a list of unique strings found
func (r regexpReader) GetExtracted() []string {
	extractedSlice := make([]string, len(r.extractedStrings))
	i := 0
	for s3Path := range r.extractedStrings {
		extractedSlice[i] = s3Path
		i++
	}
	sort.Strings(extractedSlice)
	return extractedSlice
}

// Filter applies the filter to the nodes.
func (r *regexpReader) Filter(nodes []*yaml.RNode) ([]*yaml.RNode, error) {
	_, err := kio.FilterAll(yaml.FilterFunc(r.filter)).Filter(nodes)
	return nodes, err
}

func (r *regexpReader) filter(node *yaml.RNode) (*yaml.RNode, error) {
	if err := node.PipeE(SliceFilter{
		ReSlice:  r.Spec.ReSlice,
		SetValue: r.SetValue,
	}); err != nil {
		return nil, err
	}

	return node, nil
}

// SetValue takes all the strings matched by the regexp and adds them to the s3Paths map
func (r *regexpReader) SetValue(s3RegEx *regexp.Regexp, s string) error {
	s3Paths := s3RegEx.FindAllString(s, -1)
	for _, s3Path := range s3Paths {
		r.extractedStrings[s3Path] = true
	}
	return nil
}
