package images

import (
	"io"
	"sort"
	"strings"

	"github.com/go-logr/logr"
	"github.com/jlewi/hydros/api/v1alpha1"
	"github.com/jlewi/hydros/pkg/kustomize/fns/configmap"
	imgFns "github.com/jlewi/hydros/pkg/kustomize/fns/images"
	"github.com/jlewi/hydros/pkg/util"

	"sigs.k8s.io/kustomize/api/filters/fsslice"
	"sigs.k8s.io/kustomize/api/types"
	"sigs.k8s.io/kustomize/kyaml/kio"
	"sigs.k8s.io/kustomize/kyaml/yaml"
)

// Extractor extracts the URLs of all the docker images in a directory of manifests
type Extractor struct {
	Log logr.Logger
}

// ExtractImages returns all the images in a source directory
func (e *Extractor) ExtractImages(sourceDir string) (*v1alpha1.ImageList, error) {
	inputs := kio.LocalPackageReader{PackagePath: sourceDir, MatchFilesGlob: kio.MatchAll}

	f := &readFn{
		Metadata: v1alpha1.Metadata{
			Name: "dump",
		},
	}
	fns := []kio.Filter{f, configmap.WrappedFilter{Filters: []kio.Filter{f}}}
	images := []string{}

	for _, fn := range fns {
		p := kio.Pipeline{
			Inputs:  []kio.Reader{inputs},
			Filters: []kio.Filter{fn},
		}

		if err := p.Execute(); err != nil {
			return nil, err
		}

		for i := range f.images {
			// Ignore image values prefixed with 'temp-', we don't want to fail downloading invalid
			// image references that are intentionally not replaced as is done with child resources
			// defined in configmaps.
			if !strings.HasPrefix(i, "temp-") {
				images = append(images, i)
			}
		}
	}

	sort.Strings(images)

	doc := &v1alpha1.ImageList{
		Kind:       "ImageList",
		APIVersion: "primer.ai.hydros/v1alpha1",
		Images:     []string{},
	}

	doc.Images = util.UniqueStrings(append(doc.Images, images...))
	return doc, nil
}

// Extract all the images
func (e *Extractor) Extract(sourceDir string, output io.Writer) error {
	doc, err := e.ExtractImages(sourceDir)
	if err != nil {
		return err
	}

	enc := yaml.NewEncoder(output)
	return enc.Encode(doc)
}

// readFn implements a read Function
type readFn struct {
	// Kind is the API name.
	Kind string `yaml:"kind"`

	// APIVersion is the API version.  Must be examples.kpt.dev/v1alpha1
	APIVersion string `yaml:"apiVersion"`

	// Metadata defines instance metadata.
	Metadata v1alpha1.Metadata `yaml:"metadata"`

	// Spec defines the desired declarative configuration.
	Spec spec `yaml:"spec"`

	images map[string]bool `yaml:"images"`
}

// spec is the spec for the kustomize function.
type spec struct {
	// FsSlice contains the FieldSpecs to locate an image field,
	// e.g. Path: "spec/myContainers[]/image"
	FsSlice types.FsSlice `json:"fieldSpecs,omitempty" yaml:"fieldSpecs,omitempty"`
}

func (f *readFn) init() error {
	if f.Metadata.Labels == nil {
		f.Metadata.Labels = map[string]string{}
	}

	// Add the defaults
	if f.Spec.FsSlice == nil {
		f.Spec.FsSlice = types.FsSlice{}
	}
	f.Spec.FsSlice = append(f.Spec.FsSlice, imgFns.DefaultFsSlice...)

	// n.b. When using wrapped filter init gets called multiple times. We don't want to reinitialize images because
	// that would cause us to forget some of the images we already saw.
	if f.images == nil {
		f.images = map[string]bool{}
	}
	return nil
}

// Filter applies the filter to the nodes.
func (f *readFn) Filter(nodes []*yaml.RNode) ([]*yaml.RNode, error) {
	if err := f.init(); err != nil {
		return nil, err
	}
	_, err := kio.FilterAll(yaml.FilterFunc(f.filter)).Filter(nodes)
	return nodes, err
}

func (f *readFn) filter(node *yaml.RNode) (*yaml.RNode, error) {
	if err := node.PipeE(fsslice.Filter{
		FsSlice:  f.Spec.FsSlice,
		SetValue: f.getValue,
	}); err != nil {
		return nil, err
	}

	return node, nil
}

func (f *readFn) getValue(node *yaml.RNode) error {
	f.images[node.YNode().Value] = true
	return nil
}
