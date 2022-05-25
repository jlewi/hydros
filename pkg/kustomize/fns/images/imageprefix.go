package images

import (
	"fmt"

	"sigs.k8s.io/kustomize/kyaml/resid"

	"github.com/PrimerAI/hydros-public/api/v1alpha1"
	"sigs.k8s.io/kustomize/api/filters/filtersutil"
	"sigs.k8s.io/kustomize/api/filters/fsslice"
	"sigs.k8s.io/kustomize/api/types"
	"sigs.k8s.io/kustomize/kyaml/kio"
	"sigs.k8s.io/kustomize/kyaml/yaml"
)

const (
	// Kind is the kind for the kustomize function.
	Kind = "ImagePrefix"
)

var _ kio.Filter = &ImagePrefixFn{}

// DefaultFsSlice are the default FieldSpecs for resources that should be transformed
// N.B. I couldn't find a reference in the kustomize codebase with a list of the default values.
// I think that's because kustomize uses a legacy filter to handle default fields.
// see: https://github.com/kubernetes-sigs/kustomize/blob/57206a628d8096824eded1e574bc85547245fcf8/api/builtins/ImageTagTransformer.go#L28
// I think the new kyaml filter is only used to handle custom kinds.
var DefaultFsSlice = []types.FieldSpec{
	{
		Path: "spec/template/spec/containers[]/image",
	},
	{
		Path: "spec/template/spec/initContainers[]/image",
	},
	{
		Path: "spec/jobTemplate/spec/template/spec/containers[]/image",
	},
	// Include known alternative locations
	{
		Gvk: resid.Gvk{
			Kind: "MLSBackend",
		},
		Path: "spec/runtime/image",
	},
	{
		Gvk: resid.Gvk{
			Kind: "ConfigMap",
		},
		Path: "data/IO_SIDECAR_IMAGE",
	},
	{
		Gvk: resid.Gvk{
			Kind: "ConfigMap",
		},
		Path: "data/SERVER_IMAGE",
	},
	{
		Gvk: resid.Gvk{
			Kind: "ConfigMap",
		},
		Path: "data/REDIS_IMAGE",
	},
	{
		Gvk: resid.Gvk{
			Kind: "ConfigMap",
		},
		Path: "data/MLT_TRAINER_IMAGE",
	},
	{
		Gvk: resid.Gvk{
			Kind: "ConfigMap",
		},
		Path: "data/connection_pooler_image",
	},
	{
		Gvk: resid.Gvk{
			Kind: "ConfigMap",
		},
		Path: "data/docker_image",
	},
	{
		Gvk: resid.Gvk{
			Kind: "ConfigMap",
		},
		Path: "data/logical_backup_docker_image",
	},
	{
		Gvk: resid.Gvk{
			Kind: "Kafka",
		},
		Path: "spec/entityOperator/topicOperator/image",
	},
	{
		Gvk: resid.Gvk{
			Kind: "Kafka",
		},
		Path: "spec/entityOperator/userOperator/image",
	},
	{
		Gvk: resid.Gvk{
			Kind: "Kafka",
		},
		Path: "spec/kafka/image",
	},
	{
		Gvk: resid.Gvk{
			Kind: "Kafka",
		},
		Path: "spec/zookeeper/image",
	},
	{
		Gvk: resid.Gvk{
			Kind: "Kafka",
		},
		Path: "spec/kafkaExporter/image",
	},
	{
		Gvk: resid.Gvk{
			Kind: "ConfigMap",
		},
		Path: "data/ABBA_ABEX_IMAGE",
	},
	{
		Gvk: resid.Gvk{
			Kind: "ConfigMap",
		},
		Path: "data/ONPREM_IMAGE",
	},
	{
		Gvk: resid.Gvk{
			Kind: "ConfigMap",
		},
		Path: "data/ABBA_APP_IMAGE",
	},
	{
		Gvk: resid.Gvk{
			Kind: "RedisCluster",
		},
		Path: "spec/kubernetesConfig/image",
	},
	{
		Gvk: resid.Gvk{
			Kind: "RedisCluster",
		},
		Path: "spec/redisExporter/image",
	},
	{
		Gvk: resid.Gvk{
			Kind: "Redis",
		},
		Path: "spec/kubernetesConfig/image",
	},
	{
		Gvk: resid.Gvk{
			Kind: "Redis",
		},
		Path: "spec/redisExporter/image",
	},
}

// Filter returns a new ImagePrefixFn
func Filter() kio.Filter {
	return &ImagePrefixFn{}
}

// ImagePrefixFn implements the ImagePrefix Function
type ImagePrefixFn struct {
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
	ImageMappings []ImageMapping `yaml:"imageMappings"`

	// FsSlice contains the FieldSpecs to locate an image field,
	// e.g. Path: "spec/myContainers[]/image"
	FsSlice types.FsSlice `json:"fieldSpecs,omitempty" yaml:"fieldSpecs,omitempty"`
}

func (f *ImagePrefixFn) init() error {
	if f.Metadata.Name == "" {
		return fmt.Errorf("must specify ImagePrefixFn name")
	}

	if f.Metadata.Labels == nil {
		f.Metadata.Labels = map[string]string{}
	}

	if f.Spec.ImageMappings == nil || len(f.Spec.ImageMappings) == 0 {
		return fmt.Errorf("must specify ImageMappings")
	}

	// Add the defaults
	if f.Spec.FsSlice == nil {
		f.Spec.FsSlice = types.FsSlice{}
	}
	f.Spec.FsSlice = append(f.Spec.FsSlice, DefaultFsSlice...)
	return nil
}

// ImageMapping represents the mapping from a source
// to a destination repo.
type ImageMapping struct {
	Src  string `yaml:"src"`
	Dest string `yaml:"dest"`
}

// Filter applies the filter to the nodes.
func (f ImagePrefixFn) Filter(nodes []*yaml.RNode) ([]*yaml.RNode, error) {
	if err := f.init(); err != nil {
		return nil, err
	}
	_, err := kio.FilterAll(yaml.FilterFunc(f.filter)).Filter(nodes)
	return nodes, err
}

func (f ImagePrefixFn) filter(node *yaml.RNode) (*yaml.RNode, error) {
	// FsSlice is an allowlist, not a denyList, so to deny
	// something via configuration a new config mechanism is
	// needed. Until then, hardcode it.
	if f.isOnDenyList(node) {
		return node, nil
	}

	for _, m := range f.Spec.ImageMappings {
		if err := node.PipeE(fsslice.Filter{
			FsSlice:  f.Spec.FsSlice,
			SetValue: updateImagePrefixFn(m),
		}); err != nil {
			return nil, err
		}
	}
	return node, nil
}

func (f ImagePrefixFn) isOnDenyList(node *yaml.RNode) bool {
	meta, err := node.GetMeta()
	if err != nil {
		// A missing 'meta' field will cause problems elsewhere;
		// ignore it here to keep the signature simple.
		return false
	}
	// Ignore CRDs
	// https://github.com/kubernetes-sigs/kustomize/issues/890
	return meta.Kind == `CustomResourceDefinition`
}

func updateImagePrefixFn(m ImageMapping) filtersutil.SetFn {
	return func(node *yaml.RNode) error {
		return node.PipeE(imagePrefixUpdater{
			ImageMapping: m,
		})
	}
}
