package util

import (
	"fmt"
	"strings"

	kustomize "sigs.k8s.io/kustomize/api/types"
)

// DockerImageRef defines the various pieces of a docker image
type DockerImageRef struct {
	Registry string
	Repo     string
	Tag      string
	Sha      string
}

// ToURL returns the full image URL.
func (d *DockerImageRef) ToURL() string {
	url := d.Registry + "/" + d.Repo
	if d.Tag != "" {
		url = url + ":" + d.Tag
	}
	if d.Sha != "" {
		url = url + "@" + d.Sha
	}
	return url
}

// GetAwsRegistryID returns the registry ID or "" if there isn't one
func (d *DockerImageRef) GetAwsRegistryID() string {
	if !strings.HasSuffix(d.Registry, "amazonaws.com") {
		return ""
	}

	p := strings.Split(d.Registry, ".")
	return p[0]
}

// ParseImageURL parses the URL refering to a docker image
//
// TODO(jeremy): We should support shas as well
func ParseImageURL(url string) (*DockerImageRef, error) {
	r := &DockerImageRef{}

	bySha := strings.Split(url, "@")
	if len(bySha) > 2 {
		return r, fmt.Errorf("Url isn't valid more then one @ in the URL")
	}

	if len(bySha) == 2 {
		r.Sha = bySha[1]
	}

	imageAndTag := bySha[0]

	pieces := strings.Split(imageAndTag, ":")
	if len(pieces) > 2 {
		return r, fmt.Errorf("Url isn't valid more then one : in the URL")
	}

	if len(pieces) == 2 {
		r.Tag = pieces[1]
	}

	p := strings.SplitN(pieces[0], "/", 2)

	if len(p) != 2 {
		return r, fmt.Errorf("Url isn't valid not of the form {REGISTRY/{REPO}")
	}

	r.Registry = p[0]
	r.Repo = p[1]
	return r, nil
}

// SetKustomizeImage sets the specified image in the kustomization.
func SetKustomizeImage(k *kustomize.Kustomization, name string, resolved DockerImageRef) error {
	if k == nil {
		return fmt.Errorf("k can't be nil")
	}
	for index, i := range k.Images {
		if i.Name != name {
			continue
		}

		i.NewName = ""
		i.Digest = ""
		i.NewTag = ""
		name := resolved.Registry + "/" + resolved.Repo
		if resolved.Tag != "" {
			if resolved.Sha != "" {
				// If digest and tag are set then tag should be part of the name
				name = fmt.Sprintf("%v:%v", name, resolved.Tag)
				// Don't set NewTag since we are setting digest
				i.NewTag = ""
			} else {
				// Since there's no digest set the tag.
				i.NewTag = resolved.Tag
			}
		}

		i.NewName = name
		if resolved.Sha != "" {
			i.Digest = resolved.Sha
		}

		k.Images[index] = i

		return nil
	}

	return fmt.Errorf("kustomization is missing image named %v", name)
}
