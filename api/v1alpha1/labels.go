package v1alpha1

import (
	"bytes"
	"encoding/json"

	meta "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// LabelSelector is a label query over a set of resources. The result of matchLabels and
// matchExpressions are ANDed. An empty label selector matches all objects. A null
// label selector matches no objects.
// +structType=atomic
//
// N.B. Based on https://github.com/kubernetes/apimachinery/blob/460d10991a520527026863efae34f49f89c2f4e1/pkg/apis/meta/v1/types.go#L1096
// We copy the struct so we can add YAML annotations
type LabelSelector struct {
	// matchLabels is a map of {key,value} pairs. A single {key,value} in the matchLabels
	// map is equivalent to an element of matchExpressions, whose key field is "key", the
	// operator is "In", and the values array contains only "value". The requirements are ANDed.
	// +optional
	MatchLabels map[string]string `json:"matchLabels,omitempty" yaml:"matchLabels,omitempty"`
	// MatchExpressions is a list of label selector requirements. The requirements are ANDed.
	// +optional
	MatchExpressions []LabelSelectorRequirement `json:"matchExpressions,omitempty" yaml:"matchExpressions,omitempty"`
}

// LabelSelectorRequirement is a selector that contains values, a key, and an operator that
// relates the key and values.
type LabelSelectorRequirement struct {
	// key is the label key that the selector applies to.
	// +patchMergeKey=key
	// +patchStrategy=merge
	Key string `json:"key" yaml:"key" patchStrategy:"merge" patchMergeKey:"key"`
	// operator represents a key's relationship to a set of values.
	// Valid operators are In, NotIn, Exists and DoesNotExist.
	Operator LabelSelectorOperator `json:"operator" yaml:"operator"`
	// values is an array of string values. If the operator is In or NotIn,
	// the values array must be non-empty. If the operator is Exists or DoesNotExist,
	// the values array must be empty. This array is replaced during a strategic
	// merge patch.
	// +optional
	Values []string `json:"values,omitempty" yaml:"values,omitempty"`
}

// LabelSelectorOperator is the set of operators that can be used in a selector requirement.
type LabelSelectorOperator string

const (
	// LabelSelectorOpIn is the in operator
	LabelSelectorOpIn LabelSelectorOperator = "In"
	// LabelSelectorOpNotIn is the not in operator
	LabelSelectorOpNotIn LabelSelectorOperator = "NotIn"
	// LabelSelectorOpExists is the exists operator
	LabelSelectorOpExists LabelSelectorOperator = "Exists"
	// LabelSelectorOpDoesNotExist is the does not exist operator
	LabelSelectorOpDoesNotExist LabelSelectorOperator = "DoesNotExist"
)

// ToK8s converts it to a K8s Label selector.
// This indirection is necessary because we need to change how the LabelSelector is serialized to YAML by adding
// YAML tags.
func (s *LabelSelector) ToK8s() (*meta.LabelSelector, error) {
	// Convert by serializing and then deserializing.
	b := bytes.NewBufferString("")
	e := json.NewEncoder(b)
	if err := e.Encode(s); err != nil {
		return nil, err
	}

	d := json.NewDecoder(b)

	selector := &meta.LabelSelector{}

	if err := d.Decode(selector); err != nil {
		return nil, err
	}

	return selector, nil
}
