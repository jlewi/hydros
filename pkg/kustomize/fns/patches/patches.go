package patches

import (
	"fmt"

	"github.com/PrimerAI/hydros-public/api/v1alpha1"
	jsonpatch "github.com/evanphx/json-patch/v5"
	"github.com/pkg/errors"
	"sigs.k8s.io/kustomize/api/filters/patchjson6902"
	"sigs.k8s.io/kustomize/api/filters/patchstrategicmerge"
	"sigs.k8s.io/kustomize/kyaml/fn/framework"
	"sigs.k8s.io/kustomize/kyaml/kio"
	kYaml "sigs.k8s.io/kustomize/kyaml/yaml"
	"sigs.k8s.io/yaml"
)

const (
	// Kind is the kind for this function.
	Kind = "Patch"
)

// Filter returns a filter for the Patches Fn
func Filter() kio.Filter {
	return &PatchFn{}
}

// PatchFn is a filter to apply patches to resources.
// The patches can be either strategic merge patch or JSON6902 patches.
// Patches can be targeted to resources a number of ways (name, matching resource type, labels).
// This allows for bulk applying a patch to multiple resources.
// The semantics should be the same as kustomize's patch transform:
//
// https://kubectl.docs.kubernetes.io/references/kustomize/kustomization/patches/
//
// This is based on the Patch struct used in kustomize to represent it
// https://github.com/kubernetes-sigs/kustomize/blob/f61b075d3bd670b7bcd5d58ce13e88a6f25977f2/api/types/patch.go
type PatchFn struct {
	// Kind is the API name.  Must be PodEnvs.
	Kind string `yaml:"kind"`

	// APIVersion is the API version.  Must be examples.kpt.dev/v1alpha1
	APIVersion string `yaml:"apiVersion"`

	// Metadata defines instance metadata.
	Metadata v1alpha1.Metadata `yaml:"metadata"`

	// Spec defines the desired declarative configuration.
	Spec Spec `yaml:"spec"`
}

// Spec is the spec for PatchFn.
type Spec struct {
	// Patches is the content of the patches to apply
	Patches []string `json:"patches,omitempty" yaml:"patches,omitempty"`

	// Target points to the resources that the patch is applied to
	Selector *framework.Selector `json:"selector,omitempty" yaml:"selector,omitempty"`
}

func (f *PatchFn) init() error {
	if f.Metadata.Name == "" {
		return fmt.Errorf("must specify name for PatchFn")
	}

	return nil
}

// Filter applies the filter to the nodes.
func (f PatchFn) Filter(items []*kYaml.RNode) ([]*kYaml.RNode, error) {
	var err error
	target := items
	if f.Spec.Selector != nil {
		target, err = f.Spec.Selector.Filter(items)
		if err != nil {
			return nil, err
		}
	}
	if len(target) == 0 {
		// nothing to do
		return items, nil
	}

	for _, p := range f.Spec.Patches {
		// We follow what kustomize does and try to decode it as JSON6902 and if it is treat it as JSON6902
		// patch. This works for kustomize so it should work for us
		// https://github.com/kubernetes-sigs/kustomize/blob/48f21e920acd4ac3909714fc0caf8796648014fc/api/internal/builtins/PatchTransformer.go#L52
		// Check if the patch is a JSON6902
		var patchFn kio.Filter
		var patchType string
		if jsonPatch, err := isJSON6902Patch(p); err == nil {
			patchFn = patchjson6902.Filter{
				// n.b the patch must always be expressed as json list in string form. isJSON6902Patch normalizes
				// YAML to json
				Patch: jsonPatch,
			}
			patchType = "JSON6902"
		} else {
			n, err := kYaml.Parse(p)
			if err != nil {
				return items, errors.Wrapf(err, "Failed to parse patch YAML")
			}
			patchFn = patchstrategicmerge.Filter{
				Patch: n,
			}
			patchType = "Strategic Merge"
		}

		if _, err := patchFn.Filter(target); err != nil {
			return nil, errors.Wrapf(err, "Failed to apply %v patch", patchType)
		}

	}
	return items, nil
}

// isJSON6902Patch attempts to load a Json6902 patch.
// Returns an error if it is not a json6902 patch.
// If it is a json6902 patch it returns the patch as a json string.
// The input could be in YAML or JSON but the output is always in json.
//
// Copied from: https://github.com/kubernetes-sigs/kustomize/blob/48f21e920acd4ac3909714fc0caf8796648014fc/api/internal/builtins/PatchTransformer.go#L134
func isJSON6902Patch(in string) (string, error) {
	ops := string(in)
	if ops == "" {
		return "", fmt.Errorf("empty json patch operations")
	}

	if ops[0] != '[' {
		jsonOps, err := yaml.YAMLToJSON([]byte(in))
		if err != nil {
			return "", err
		}
		ops = string(jsonOps)
	}
	_, err := jsonpatch.DecodePatch([]byte(ops))
	return in, err
}
