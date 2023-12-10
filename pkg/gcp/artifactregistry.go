package gcp

import (
	"cloud.google.com/go/artifactregistry/apiv1"
	"cloud.google.com/go/artifactregistry/apiv1/artifactregistrypb"
	"context"
	"fmt"
	"github.com/go-logr/zapr"
	"github.com/jlewi/hydros/api/v1alpha1"
	"github.com/jlewi/hydros/pkg/util"
	"github.com/pkg/errors"
	"go.uber.org/zap"
	"strings"
)

const (
	gcpRegistrySuffix = "-docker.pkg.dev"
)

type ArtifactImage struct {
	Project    string
	Location   string
	Repository string
	Package    string
	Tag        string
	Sha        string
}

func (a ArtifactImage) NameForTag() string {
	return fmt.Sprintf("projects/%s/locations/%s/repositories/%s/packages/%s/tags/%s", a.Project, a.Location, a.Repository, a.Package, a.Tag)
}

func FromImageRef(r util.DockerImageRef) (ArtifactImage, error) {
	if !strings.HasSuffix(r.Registry, gcpRegistrySuffix) {
		return ArtifactImage{}, errors.New("Registry must end with " + gcpRegistrySuffix)
	}

	image := ArtifactImage{
		Tag: r.Tag,
		Sha: r.Sha,
	}

	// Location should be registry stripped of the suffix
	image.Location = strings.TrimSuffix(r.Registry, gcpRegistrySuffix)

	pieces := strings.Split(r.Repo, "/")

	// First piece is the project
	image.Project = pieces[0]
	// Second pieces is the repository name
	image.Repository = pieces[1]
	image.Package = strings.Join(pieces[2:], "/")
	return image, nil
}

type ImageResolver struct {
}

func (i *ImageResolver) ResolveImageToSha(ref util.DockerImageRef, strategy v1alpha1.Strategy) (util.DockerImageRef, error) {
	if strategy != v1alpha1.MutableTagStrategy {
		return util.DockerImageRef{}, fmt.Errorf("Only MutableTagStrategy is currently implemented for artifact registry")
	}

	image, err := FromImageRef(ref)
	if err != nil {
		return ref, err
	}

	ctx := context.Background()
	// This snippet has been automatically generated and should be regarded as a code template only.
	// It will require modifications to work:
	// - It may require correct/in-range values for request initialization.
	// - It may require specifying regional endpoints when creating the service client as shown in:
	//   https://pkg.go.dev/cloud.google.com/go#hdr-Client_Options
	c, err := artifactregistry.NewClient(ctx)
	if err != nil {
		// TODO: Handle error.
	}
	defer c.Close()

	req := &artifactregistrypb.GetTagRequest{
		Name: image.NameForTag(),
	}

	log := zapr.NewLogger(zap.L())
	log.Info("Getting tag", "name", req.Name)
	resp, err := c.GetTag(ctx, req)
	if err != nil {
		return ref, err
	}

	ref.Sha = resp.GetVersion()

	return ref, err
}
