package fields

import (
	"fmt"
	"strings"

	"github.com/PrimerAI/hydros-public/api/v1alpha1"
	"sigs.k8s.io/kustomize/kyaml/kio"
	"sigs.k8s.io/kustomize/kyaml/yaml"
)

const (
	// Kind is the kind for the kustomize function.
	Kind = "Fields"
)

var _ kio.Filter = &Fields{}

// Filter returns a new
func Filter() kio.Filter {
	return &Fields{}
}

// Fields is a custom kustomize function that can be used to delete/remove fields
// from manifests
type Fields struct {
	// Kind is the API name.
	Kind string `yaml:"kind"`

	// APIVersion is the API version.  Must be examples.kpt.dev/v1alpha1
	APIVersion string `yaml:"apiVersion"`

	// Metadata defines instance metadata.
	Metadata v1alpha1.Metadata `yaml:"metadata"`

	// Spec defines the desired declarative configuration.
	Spec Spec `yaml:"spec"`
}

// Spec is the spec for the kustomize function.
type Spec struct {
	Remove []string `yaml:"remove"`
}

func (f *Fields) init() error {
	if f.Metadata.Name == "" {
		return fmt.Errorf("must specify Fields Function name")
	}

	if f.Metadata.Labels == nil {
		f.Metadata.Labels = map[string]string{}
	}

	if f.Spec.Remove == nil {
		f.Spec.Remove = []string{}
	}
	return nil
}

// Filter applies the filter to the nodes.
func (f Fields) Filter(nodes []*yaml.RNode) ([]*yaml.RNode, error) {
	if err := f.init(); err != nil {
		return nil, err
	}

	_, err := kio.FilterAll(yaml.FilterFunc(f.filter)).Filter(nodes)
	return nodes, err
}

func (f Fields) filter(node *yaml.RNode) (*yaml.RNode, error) {
	err := f.removeFields(node)
	if err != nil {
		return node, err
	}
	return node, nil
}

// removeFields performs the removal of specified fields
func (f *Fields) removeFields(node *yaml.RNode) error {
	for _, field := range f.Spec.Remove {
		fields := strings.Split(field, ".")

		fieldToRemove := fields[len(fields)-1]
		fieldPath := fields[:len(fields)-1]

		spec, err := node.Pipe(yaml.PathGetter{Path: fieldPath})
		if err != nil {
			s, _ := node.String()
			return fmt.Errorf("%v: %s", err, s)
		}

		if spec != nil && !spec.Field(fieldToRemove).IsNilOrEmpty() {
			if err = spec.PipeE(yaml.Clear(fieldToRemove)); err != nil {
				return err
			}
		}
	}

	return nil
}
