package null

import "sigs.k8s.io/kustomize/kyaml/yaml"

// Filter is a filter which does nothing. Its basically the identity function.
// The sole purpose is for use in tests. Applying the null filter allows the expected output to be formatted
// the same way the actual output gets filtered which makes it easy to evaluate by just doing string comparison.
type Filter struct{}

// Filter applies the filter to the nodes.
func (f Filter) Filter(nodes []*yaml.RNode) ([]*yaml.RNode, error) {
	return nodes, nil
}
