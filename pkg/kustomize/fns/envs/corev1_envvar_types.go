// These types are copied from the corev1 library
// https://github.com/kubernetes/api/blob/v0.22.4/core/v1/types.go#L1931
//
// The corev1 library defines these types to only have JSON tags. We would like our PodEnvs function
// to support embedding instances of corev1.EnvVar. Difficulties arise when trying to do so
// directly, as the result is a function definition with some fields only supporting either YAML
// or JSON encoding/decoding.
//
// Here we redefine the types needed to define a corev1.EnvVar with the simple modification of
// defining with a YAML tag (instead of JSON)

package envs

import (
	"k8s.io/apimachinery/pkg/api/resource"
)

// EnvVar represents an environment variable present in a Container.
type EnvVar struct {
	// Name of the environment variable. Must be a C_IDENTIFIER.
	Name string `yaml:"name" json:"name" protobuf:"bytes,1,opt,name=name"`

	// Optional: no more than one of the following may be specified.

	// Variable references $(VAR_NAME) are expanded
	// using the previously defined environment variables in the container and
	// any service environment variables. If a variable cannot be resolved,
	// the reference in the input string will be unchanged. Double $$ are reduced
	// to a single $, which allows for escaping the $(VAR_NAME) syntax: i.e.
	// "$$(VAR_NAME)" will produce the string literal "$(VAR_NAME)".
	// Escaped references will never be expanded, regardless of whether the variable
	// exists or not.
	// Defaults to "".
	// +optional
	Value string `yaml:"value,omitempty" json:"value,omitempty" protobuf:"bytes,2,opt,name=value"`
	// Source for the environment variable's value. Cannot be used if value is not empty.
	// +optional
	ValueFrom *EnvVarSource `yaml:"valueFrom,omitempty" json:"valueFrom,omitempty" protobuf:"bytes,3,opt,name=valueFrom"`
}

// EnvVarSource represents a source for the value of an EnvVar.
type EnvVarSource struct {
	// Selects a field of the pod: supports metadata.name, metadata.namespace, `metadata.labels['<KEY>']`, `metadata.annotations['<KEY>']`,
	// spec.nodeName, spec.serviceAccountName, status.hostIP, status.podIP, status.podIPs.
	// +optional
	FieldRef *ObjectFieldSelector `yaml:"fieldRef,omitempty" json:"fieldRef,omitempty" protobuf:"bytes,1,opt,name=fieldRef"`
	// Selects a resource of the container: only resources limits and requests
	// (limits.cpu, limits.memory, limits.ephemeral-storage, requests.cpu, requests.memory and requests.ephemeral-storage) are currently supported.
	// +optional
	ResourceFieldRef *ResourceFieldSelector `yaml:"resourceFieldRef,omitempty" json:"resourceFieldRef,omitempty" protobuf:"bytes,2,opt,name=resourceFieldRef"`
	// Selects a key of a ConfigMap.
	// +optional
	ConfigMapKeyRef *ConfigMapKeySelector `yaml:"configMapKeyRef,omitempty" json:"configMapKeyRef,omitempty" protobuf:"bytes,3,opt,name=configMapKeyRef"`
	// Selects a key of a secret in the pod's namespace
	// +optional
	SecretKeyRef *SecretKeySelector `yaml:"secretKeyRef,omitempty" json:"secretKeyRef,omitempty" protobuf:"bytes,4,opt,name=secretKeyRef"`
}

// ObjectFieldSelector selects an APIVersioned field of an object.
// +structType=atomic
type ObjectFieldSelector struct {
	// Version of the schema the FieldPath is written in terms of, defaults to "v1".
	// +optional
	APIVersion string `yaml:"apiVersion,omitempty" json:"apiVersion,omitempty" protobuf:"bytes,1,opt,name=apiVersion"`
	// Path of the field to select in the specified API version.
	FieldPath string `yaml:"fieldPath" json:"fieldPath" protobuf:"bytes,2,opt,name=fieldPath"`
}

// ResourceFieldSelector represents container resources (cpu, memory) and their output format
// +structType=atomic
type ResourceFieldSelector struct {
	// Container name: required for volumes, optional for env vars
	// +optional
	ContainerName string `yaml:"containerName,omitempty" json:"containerName,omitempty" protobuf:"bytes,1,opt,name=containerName"`
	// Required: resource to select
	Resource string `yaml:"resource" json:"resource" protobuf:"bytes,2,opt,name=resource"`
	// Specifies the output format of the exposed resources, defaults to "1"
	// +optional
	Divisor resource.Quantity `yaml:"divisor,omitempty" json:"divisor,omitempty" protobuf:"bytes,3,opt,name=divisor"`
}

// ConfigMapKeySelector selects a key from a ConfigMap.
// +structType=atomic
type ConfigMapKeySelector struct {
	// The ConfigMap to select from.
	LocalObjectReference `yaml:",inline" json:",inline" protobuf:"bytes,1,opt,name=localObjectReference"`
	// The key to select.
	Key string `yaml:"key" json:"key" protobuf:"bytes,2,opt,name=key"`
	// Specify whether the ConfigMap or its key must be defined
	// +optional
	Optional *bool `yaml:"optional,omitempty" json:"optional,omitempty" protobuf:"varint,3,opt,name=optional"`
}

// SecretKeySelector selects a key of a Secret.
// +structType=atomic
type SecretKeySelector struct {
	// The name of the secret in the pod's namespace to select from.
	LocalObjectReference `yaml:",inline" json:",inline" protobuf:"bytes,1,opt,name=localObjectReference"`
	// The key of the secret to select from.  Must be a valid secret key.
	Key string `yaml:"key" json:"key" protobuf:"bytes,2,opt,name=key"`
	// Specify whether the Secret or its key must be defined
	// +optional
	Optional *bool `yaml:"optional,omitempty" json:"optional,omitempty" protobuf:"varint,3,opt,name=optional"`
}

// LocalObjectReference contains enough information to let you locate the
// referenced object inside the same namespace.
// +structType=atomic
type LocalObjectReference struct {
	// Name of the referent.
	// More info: https://kubernetes.io/docs/concepts/overview/working-with-objects/names/#names
	// TODO: Add other useful fields. apiVersion, kind, uid?
	// +optional
	Name string `yaml:"name,omitempty" json:"name,omitempty" protobuf:"bytes,1,opt,name=name"`
}
