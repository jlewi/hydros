package s3assets

import (
	"fmt"
	"regexp"

	"sigs.k8s.io/kustomize/api/filters/fsslice"
	ktypes "sigs.k8s.io/kustomize/api/types"
	"sigs.k8s.io/kustomize/kyaml/kio"
	"sigs.k8s.io/kustomize/kyaml/resid"
	"sigs.k8s.io/kustomize/kyaml/yaml"

	"github.com/PrimerAI/go-micro-utils-public/gmu/s3"
	"github.com/PrimerAI/hydros-public/api/v1alpha1"
)

const (
	// Kind is the api name for this kustomize function.
	Kind = "S3BucketTransformer"
	// DefaultS3Regexp matches s3://.* until a " or \n
	DefaultS3Regexp = `(s3:\/\/[^\n"]+)`
)

// DefaultFieldSpecs are the kustomize locations that this function
// looks for s3 uris to transform by default.
var DefaultFieldSpecs = ktypes.FsSlice{
	{
		Gvk:  resid.Gvk{Kind: "ConfigMap"},
		Path: "data",
	},
}

// Filter implements the logic for this custom transform
func Filter() kio.Filter {
	return &BucketTransformer{}
}

// BucketTransformer defines our custom s3 function
type BucketTransformer struct {
	// Kind is the API name.  Must be S3BucketTransformer.
	Kind string `yaml:"kind"`

	// APIVersion is the API version.  Must be v1alpha1.
	APIVersion string `yaml:"apiVersion"`

	// Metadata defines instance metadata.
	Metadata v1alpha1.Metadata `yaml:"metadata"`

	// Spec defines the desired declarative configuration.
	Spec Spec `yaml:"spec"`
}

// Spec defines fields of our BucketTransformer
type Spec struct {
	Bucket     string         `yaml:"bucket"`
	FieldSpecs ktypes.FsSlice `yaml:"fieldSpecs"`
	Regexps    []string       `yaml:"regexps"`
	// regexps are the compiled forms of Regexps
	regexps []*regexp.Regexp
}

func (f *BucketTransformer) setup() error {
	if f.Spec.Bucket == "" {
		return fmt.Errorf("must specify bucket in S3BucketTransformer")
	}
	if f.Spec.FieldSpecs == nil {
		f.Spec.FieldSpecs = ktypes.FsSlice{}
	}
	f.Spec.FieldSpecs = append(f.Spec.FieldSpecs, DefaultFieldSpecs...)
	if f.Spec.Regexps == nil {
		f.Spec.Regexps = []string{}
	}
	f.Spec.Regexps = append(f.Spec.Regexps, DefaultS3Regexp)

	f.Spec.regexps = []*regexp.Regexp{}
	if err := f.compileRegexps(); err != nil {
		return err
	}

	return nil
}

func (f *BucketTransformer) compileRegexps() error {
	for _, re := range f.Spec.Regexps {
		cre, err := regexp.Compile(re)
		if err != nil {
			return err
		}
		f.Spec.regexps = append(f.Spec.regexps, cre)
	}
	return nil
}

// Filter applies the filter to the nodes.
func (f BucketTransformer) Filter(nodes []*yaml.RNode) ([]*yaml.RNode, error) {
	if err := f.setup(); err != nil {
		return nodes, err
	}
	_, err := kio.FilterAll(yaml.FilterFunc(f.filter)).Filter(nodes)
	return nodes, err
}

func (f BucketTransformer) filter(node *yaml.RNode) (*yaml.RNode, error) {
	if err := node.PipeE(fsslice.Filter{
		FsSlice:  f.Spec.FieldSpecs,
		SetValue: f.setValue,
	}); err != nil {
		return nil, err
	}
	return node, nil
}

func (f BucketTransformer) setValue(node *yaml.RNode) error {
	nodeString, err := node.String()
	if err != nil {
		return err
	}
	for _, re := range f.Spec.regexps {
		nodeString = re.ReplaceAllStringFunc(nodeString, f.addBucket)
	}
	newNode, err := yaml.Parse(nodeString)
	if err != nil {
		return err
	}
	node.SetYNode(newNode.YNode())
	return nil
}

func (f BucketTransformer) addBucket(s3Uri string) string {
	sPath, err := s3.FromURI(s3Uri)
	if err != nil {
		// a panic here indicates that there's something wrong with the regexp
		panic(fmt.Errorf("cannot parse s3 uri - %s: %w", s3Uri, err))
	}
	// in case there's more than one regexp in the future we want to make sure
	// we don't add the new bucket more than once. This condition makes the
	// addition idempotent.
	if sPath.Bucket == f.Spec.Bucket {
		return s3Uri
	}
	newSPath := s3.Path{
		Bucket: f.Spec.Bucket,
		Key:    sPath.Join(),
	}
	return newSPath.ToURI()
}
