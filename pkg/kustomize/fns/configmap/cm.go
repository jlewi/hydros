package configmap

import (
	"fmt"

	"github.com/PrimerAI/hydros-public/api/v1alpha1"
	"sigs.k8s.io/kustomize/kyaml/kio"
	"sigs.k8s.io/kustomize/kyaml/yaml"
)

const (
	// Kind is the kind for the kustomize function.
	Kind = "ConfigMapPatch"
)

var _ kio.Filter = &PatchFn{}

// Filter returns a new PatchFn
func Filter() kio.Filter {
	return &PatchFn{}
}

// PatchFn patches a configmap. It works by looking
// for all configmaps that define the specified keys and overriding them.
// This is different from a strategic patch merge because we don't need
// to know the name of the configmap to apply it.
type PatchFn struct {
	// Kind is the API name.  Must be ConfigMapPatch.
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
	// Data is a mapping of the values to set if present.
	Data map[string]string `yaml:"data"`
}

func (f *PatchFn) init() error {
	if f.Metadata.Name == "" {
		return fmt.Errorf("must specify PatchFn name")
	}

	if f.Metadata.Labels == nil {
		f.Metadata.Labels = map[string]string{}
	}

	if f.Spec.Data == nil {
		return fmt.Errorf("must specify Data")
	}

	return nil
}

// Filter applies the filter to the nodes.
func (f PatchFn) Filter(nodes []*yaml.RNode) ([]*yaml.RNode, error) {
	_, err := kio.FilterAll(yaml.FilterFunc(f.filter)).Filter(nodes)
	return nodes, err
}

func (f PatchFn) filter(node *yaml.RNode) (*yaml.RNode, error) {
	for k, v := range f.Spec.Data {
		err := node.PipeE(yaml.Lookup("data", k),
			yaml.FieldSetter{StringValue: v})
		if err != nil {
			return nil, err
		}
	}
	return node, nil
}
