package envs

import (
	"fmt"

	"github.com/PrimerAI/hydros-public/api/v1alpha1"

	"sigs.k8s.io/kustomize/api/filters/fsslice"
	"sigs.k8s.io/kustomize/api/types"
	"sigs.k8s.io/kustomize/kyaml/kio"
	"sigs.k8s.io/kustomize/kyaml/yaml"
)

const (
	// Kind is the kind for the kustomize function.
	Kind = "PodEnvs"
)

var _ kio.Filter = &PodEnvsFunction{}

// DefaultFsSlice is the set of FieldSpecs where we expect to find container definitions
var DefaultFsSlice = []types.FieldSpec{
	{
		Path: "spec/template/spec/containers",
	},
	{
		Path: "spec/template/spec/initContainers",
	},
	{
		Path: "spec/jobTemplate/spec/template/spec/containers",
	},
	{
		Path: "spec/jobTemplate/spec/template/spec/initContainers",
	},
}

// Filter returns a new PodEnvsFunction
func Filter() kio.Filter {
	return &PodEnvsFunction{}
}

// PodEnvsFunction implements the PodEnvs Function
type PodEnvsFunction struct {
	// Kind is the API name.  Must be PodEnvs.
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
	Set    []EnvVar `yaml:"set"`
}

func (f *PodEnvsFunction) init() error {
	if f.Metadata.Name == "" {
		return fmt.Errorf("must specify PodEnvs name")
	}

	if f.Metadata.Labels == nil {
		f.Metadata.Labels = map[string]string{}
	}

	if f.Spec.Remove == nil {
		f.Spec.Remove = []string{}
	}

	return nil
}

// Filter runs the replaceEnv function on the provided RNodes
func (f PodEnvsFunction) Filter(nodes []*yaml.RNode) ([]*yaml.RNode, error) {
	if err := f.init(); err != nil {
		return nil, err
	}
	_, err := kio.FilterAll(yaml.FilterFunc(f.filter)).Filter(nodes)
	return nodes, err
}

func (f PodEnvsFunction) filter(node *yaml.RNode) (*yaml.RNode, error) {
	if err := node.PipeE(fsslice.Filter{
		FsSlice:  DefaultFsSlice,
		SetValue: f.replaceContainerEnv,
	}); err != nil {
		return nil, err
	}
	return node, nil
}

// replaceContainerEnv should recieve a sequence rnode of container specs
func (f PodEnvsFunction) replaceContainerEnv(node *yaml.RNode) error {
	// get all the container elements
	containers, err := node.Elements()
	// err only if node isn't sequence rnode
	if err != nil {
		return err
	}
	// apply replaceEnv to each container rnode
	_, err = kio.FilterAll(yaml.FilterFunc(f.replaceEnv)).Filter(containers)
	return err
}

// replaceEnv replaces environment variables according to the function configuration.
func (f PodEnvsFunction) replaceEnv(node *yaml.RNode) (*yaml.RNode, error) {
	// Handle removing specified env vars via ElementSetter
	for _, name := range f.Spec.Remove {
		s := yaml.ElementSetter{
			// Set to nil to delete the element.
			Element: nil,
			Keys:    []string{"name"},
			Values:  []string{name},
		}

		err := node.PipeE(
			yaml.Lookup("env"),
			s,
		)
		if err != nil {
			s, _ := node.String()
			return node, fmt.Errorf("%v: %s", err, s)
		}
	}

	// Handle setting specified env vars
	for _, newEnvVar := range f.Spec.Set {

		// Dump env var struct to YAML
		envYAML, err := yaml.Marshal(newEnvVar)
		if err != nil {
			return node, fmt.Errorf("failed to marshal env var to YAML: %v", err)
		}

		// Parse YAML into RNode
		envRNode, err := yaml.Parse(string(envYAML))
		if err != nil {
			return node, fmt.Errorf("failed to parse env var YAML to RNode: %v", err)
		}

		// ElementSetter to update the env var
		s := yaml.ElementSetter{
			Element: envRNode.YNode(),
			Keys:    []string{"name"},
			Values:  []string{newEnvVar.Name},
		}

		err = node.PipeE(
			// We want to create env if it doesn't exist
			yaml.LookupCreate(yaml.SequenceNode, "env"),
			s,
		)
		if err != nil {
			s, _ := node.String()
			return node, fmt.Errorf("%v: %s", err, s)
		}
	}
	return node, nil
}
