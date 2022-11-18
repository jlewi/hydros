package fns

import (
	"sigs.k8s.io/kustomize/api/filters/filtersutil"
	"sigs.k8s.io/kustomize/api/filters/fsslice"
	"sigs.k8s.io/kustomize/api/types"
	"sigs.k8s.io/kustomize/kyaml/kio"
	"sigs.k8s.io/kustomize/kyaml/yaml"
)

// KeysClearer removes the key from the maps located at the entries specified by FsSlice.
//
// N.B. this implements kio.Filter which takes returns []*yaml.RNode
// not yaml.Filter which takes/returns a single RNode; there's no good reason
// for this. Its just based on copying
// https://github.com/kubernetes-sigs/kustomize/blob/5993eae1aa78017c9e5922b41f2dba911e03ce6a/api/filters/labels/labels.go#L28
//
// N.B jeremy@ couldn't get this to work when you have a list of maps and want to modify a field in the map e.g.
// spec:
//
//	items:
//	-  name: item1
//	   valuetokeep: v
//	   labels:
//	     toremove: a
//
// Specifying a FieldSpec with path spec/items[] doesn't work; we get the wrong type of value
// Specifying a FieldSpec with path spec/items[]/labels does work but removes values from the nested nictionary.
type KeysClearer struct {
	Kind string   `yaml:"kind,omitempty"`
	Keys []string `yaml:"keys,omitempty"`

	// FsSlice identifies the location of the maps.
	FsSlice types.FsSlice `yaml:"fieldSpecs,omitempty"`
}

// Filter applies the filter to the nodes.
func (c KeysClearer) Filter(nodes []*yaml.RNode) ([]*yaml.RNode, error) {
	_, err := kio.FilterAll(yaml.FilterFunc(
		func(node *yaml.RNode) (*yaml.RNode, error) {
			for _, k := range c.Keys {
				if err := node.PipeE(fsslice.Filter{
					FsSlice:    c.FsSlice,
					SetValue:   ClearEntry(k),
					CreateKind: yaml.MappingNode, // Labels are MappingNodes.
					CreateTag:  yaml.NodeTagMap,
				}); err != nil {
					return nil, err
				}
			}
			return node, nil
		})).Filter(nodes)
	return nodes, err
}

// ClearEntry returns a SetFn to clear a map entry
func ClearEntry(key string) filtersutil.SetFn {
	return func(node *yaml.RNode) error {
		return node.PipeE(yaml.Clear(key))
	}
}
