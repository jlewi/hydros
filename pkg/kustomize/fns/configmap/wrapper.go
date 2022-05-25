package configmap

import (
	"bytes"
	"path/filepath"
	"strings"

	"github.com/go-logr/zapr"
	"go.uber.org/zap"

	"sigs.k8s.io/kustomize/kyaml/kio"
	"sigs.k8s.io/kustomize/kyaml/yaml"
)

const (
	// ConfigMapAnnotation defines the annotation that should be set on Filters if they should be applied to
	// YAMLs stored in configmaps. Valid values are "only" and "both". Only means only apply to configmap and
	// both means apply to both the configmap contents and all other resources.
	ConfigMapAnnotation = "kustomize.primer.ai/applytocm"
	// ConfigMapOnly is the value of the annotation to only apply filter to configmap contents.
	ConfigMapOnly = "only"
	// ConfigMapBoth is the value of the annotation to apply filter to configmap contents and YAMLs in source directory.
	ConfigMapBoth = "both"
)

// WrappedFilter applies a set of Filters to the data inside a configmap. This is useful if you have
// a ConfigMap containing YAML files and you want to apply the filters to the contents of those YAML files.
//
// To users, users add the annotation "kustomize.primer.ai/applytocm" to their kustomize function definitions.
// The dispatcher will then wrap the specified kustomize function.
//
// Example:
// apiVersion: v1alpha1
// kind: PodEnvs
// metadata:
// annotations:
//   kustomize.primer.ai/applytocm: both
// spec:
//  remove:
//    - DD_AGENT_HOST
//
// Important: If you have a function that accumulates data (see for example readFn in extractor.go) note that the
// init function of the filter gets invoked multiple times. Therefore, its important to not zero out the field doing
// accumulation on each call to init.
type WrappedFilter struct {
	Filters []kio.Filter
}

// Filter applies the filter to the nodes.
func (f WrappedFilter) Filter(nodes []*yaml.RNode) ([]*yaml.RNode, error) {
	_, err := kio.FilterAll(yaml.FilterFunc(f.filter)).Filter(nodes)
	return nodes, err
}

func (f WrappedFilter) filter(node *yaml.RNode) (*yaml.RNode, error) {
	log := zapr.NewLogger(zap.L())
	// TODO(jeremy): Should we check the node is a configmap?
	// Lookup all the data in the configmap.
	data := node.GetDataMap()
	if data == nil {
		return node, nil
	}
	for name, contents := range data {
		isMatch := false

		for _, ext := range kio.MatchAll {
			pattern := "*" + ext
			m, err := filepath.Match(pattern, name)
			if err != nil {
				log.Error(err, "Failed to match file glob", "pattern", pattern, "name", name)
				continue
			}

			if m {
				isMatch = true
				break
			}
		}

		if !isMatch {
			continue
		}

		// Apply the filters to the content.
		reader := &kio.ByteReader{
			Reader: strings.NewReader(contents),
		}

		buf := bytes.NewBuffer([]byte{})
		writer := &kio.ByteWriter{
			Writer: buf,
		}
		p := kio.Pipeline{
			Inputs:  []kio.Reader{reader},
			Filters: f.Filters,
			Outputs: []kio.Writer{writer},
		}
		if err := p.Execute(); err != nil {
			return node, err
		}

		data[name] = buf.String()
	}

	// Update the data. LoadMap sorts the keys inside the configmap. This is useful for deterministic ordering.
	err := node.LoadMapIntoConfigMapData(data)

	return node, err
}
