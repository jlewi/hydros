package labels

import (
	"fmt"

	"github.com/jlewi/hydros/api/v1alpha1"
	"github.com/jlewi/hydros/pkg/kustomize/fns"
	kLabels "sigs.k8s.io/kustomize/api/filters/labels"
	"sigs.k8s.io/kustomize/api/types"
	"sigs.k8s.io/kustomize/kyaml/kio"
	"sigs.k8s.io/kustomize/kyaml/yaml"
)

const (
	// Kind is the kind for the kustomize function.
	Kind = "CommonLabels"
)

var _ kio.Filter = &CommonLabelsFn{}

// Filter returns a new CommonLabelsFn
func Filter() kio.Filter {
	return &CommonLabelsFn{}
}

// defaultFsSlice are the default FieldSpecs for resources that should be transformed
// N.B. I couldn't find a reference in the kustomize codebase with a list of the default values.
// I think that's because kustomize uses a legacy filter to handle default fields.
// see: https://github.com/kubernetes-sigs/kustomize/blob/57206a628d8096824eded1e574bc85547245fcf8/api/builtins/ImageTagTransformer.go#L28
// I think the new kyaml filter is only used to handle custom kinds.
var defaultFsSlice = []types.FieldSpec{
	// TODO(jeremy): Are these paths correct? should it be labels[]
	{
		Path: "metadata/labels",
	},
	{
		Path: "spec/template/metadata/labels",
	},
}

// CommonLabelsFn is a filter to add common labels to resources as well
// as remove labels from resources.
//
// Only the field metadata.labels will be created if it doesn't exist. For all other labels field specified
// by CommonLabelsFn.Spec.FsSlice the labels map will not be created if it doesn't exist and any labels won't be
// added. This is because the function relies on the existence of the labels field e.g "spec.template.metadata.labels"
// to know that the resource has a labels field that should be set. The exception is metadata.labels as all
// K8s resources should have metadata.labels. In practice this should be fine as its expected that other labels fields
// would be non empty (e.g. spec.template.metadata.labels).
//
// TODO(jeremy): We could potentially create the labels field if the FsSlice includes a kind and/or APIVersion because
// in that case we can correctly apply the labels map creation to only the resources that would use that non-standard
// location for labels.
//
// TODO(jeremy): If we wanted to support adding labels I think we would need
// FsSlice's to enumerate the different kinds and then for those we could
// create labels if it doesn't exist.
// In practice I don't think this should be much of an issue.ß
type CommonLabelsFn struct {
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
	// Labels is the labels to set
	Labels map[string]string `json:"labels,omitempty" yaml:"labels,omitempty"`

	// RemoveLabels is a list of keys to remove
	RemoveLabels []string `json:"removeLabel,omitempty" yaml:"removeKeys,omitempty"`

	// FsSlice contains the FieldSpecs to locate labels field,
	FsSlice types.FsSlice `json:"fieldSpecs,omitempty" yaml:"fieldSpecs,omitempty"`
}

func (f *CommonLabelsFn) init() error {
	if f.Metadata.Name == "" {
		return fmt.Errorf("must specify name for CommonLabelsFn")
	}

	slice := types.FsSlice{}
	slice = append(slice, defaultFsSlice...)
	slice = append(slice, f.Spec.FsSlice...)
	f.Spec.FsSlice = slice

	return nil
}

// Filter applies the filter to the nodes.
func (f CommonLabelsFn) Filter(nodes []*yaml.RNode) ([]*yaml.RNode, error) {
	if err := f.init(); err != nil {
		return nil, err
	}

	// Apply the filter to clear labels
	filter := fns.KeysClearer{
		Keys:    f.Spec.RemoveLabels,
		FsSlice: f.Spec.FsSlice,
	}

	if _, err := filter.Filter(nodes); err != nil {
		return nodes, err
	}

	// Create metadata.labels if it doesn't exist.
	if len(f.Spec.Labels) > 0 {
		// N.B. we use pipeline as a convenient way to execute a set of Filters against a list of nodes.
		p := kio.Pipeline{
			Inputs:  []kio.Reader{kio.ResourceNodeSlice(nodes)},
			Filters: []kio.Filter{kio.FilterAll(ensureLabels{})},
			// N.B. leave outputs blank because we won't write nodes anywhere we just modify them in place.
			Outputs:               []kio.Writer{},
			ContinueOnEmptyResult: false,
		}

		if err := p.Execute(); err != nil {
			return nodes, err
		}
	}

	// Apply the filter to set labels; we reuse kustomize's filter for this.
	// This only modifies labels if it exists so we need to create metadata.labels before if it doesn't exist.
	// See comments on CommonLabels for why we don't create other labels.
	addFilter := kLabels.Filter{
		Labels:  f.Spec.Labels,
		FsSlice: f.Spec.FsSlice,
	}

	return addFilter.Filter(nodes)
}

type ensureLabels struct{}

// ensureLabels ensures metadata.Labels exists
func (f ensureLabels) Filter(n *yaml.RNode) (*yaml.RNode, error) {
	_, err := n.Pipe(yaml.LookupCreate(yaml.MappingNode, "metadata"), yaml.LookupCreate(yaml.MappingNode, "labels"))
	return n, err
}
