package v1alpha1

import "k8s.io/apimachinery/pkg/runtime/schema"

const (
	// EcrPolicySyncKind is the kind for EcrPolicySync resources.
	EcrPolicySyncKind = "EcrPolicySync"
)

// EcrPolicySyncGVK is the GVK for EcrPolicySync.
var EcrPolicySyncGVK = schema.GroupVersionKind{
	Group:   Group,
	Version: Version,
	Kind:    EcrPolicySyncKind,
}

// EcrPolicySync continually ensures a set of ECR repos exists and has the specified policy
type EcrPolicySync struct {
	APIVersion string   `yaml:"apiVersion" yamltags:"required"`
	Kind       string   `yaml:"kind" yamltags:"required"`
	Metadata   Metadata `yaml:"metadata,omitempty"`

	Spec EcrPolicySyncSpec `yaml:"spec,omitempty"`
}

// EcrPolicySyncSpec spec for the resource to sync ECR policies.
type EcrPolicySyncSpec struct {
	// Policy is the JSON representation of the policy to apply
	Policy string `yaml:"policy,omitempty"`

	// ImageRegistry is the registry in which to apply it.
	ImageRegistry string `yaml:"imageRegistry,omitempty"`

	// ImageRepos is a list of repos to apply the changes to.
	ImageRepos []string `yaml:"imageRepos,omitempty"`
}
