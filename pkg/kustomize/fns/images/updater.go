// Copyright 2020 The Kubernetes Authors.
// SPDX-License-Identifier: Apache-2.0

package images

import (
	"strings"

	"sigs.k8s.io/kustomize/kyaml/yaml"
)

// imagePrefixUpdater is an implementation of the kio.Filter interface
// that will update the value of the yaml node based on the provided
// ImageTag if the current value matches the format of an image reference.
type imagePrefixUpdater struct {
	Kind         string       `yaml:"kind,omitempty"`
	ImageMapping ImageMapping `yaml:"imageMapping,omitempty"`
}

func (u imagePrefixUpdater) Filter(rn *yaml.RNode) (*yaml.RNode, error) {
	if err := yaml.ErrorIfInvalid(rn, yaml.ScalarNode); err != nil {
		return nil, err
	}

	value := rn.YNode().Value
	if !strings.HasPrefix(value, u.ImageMapping.Src) {
		return rn, nil
	}
	newImage := u.ImageMapping.Dest + value[len(u.ImageMapping.Src):]
	return rn.Pipe(yaml.FieldSetter{StringValue: newImage})
}
