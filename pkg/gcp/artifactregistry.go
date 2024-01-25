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
	"net/url"
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
	// If the docker image contains slashes these are query escaped
	image.Package = url.QueryEscape(strings.Join(pieces[2:], "/"))
	return image, nil
}

func NewImageResolver(ctx context.Context) (*ImageResolver, error) {
	// This snippet has been automatically generated and should be regarded as a code template only.
	// It will require modifications to work:
	// - It may require correct/in-range values for request initialization.
	// - It may require specifying regional endpoints when creating the service client as shown in:
	//   https://pkg.go.dev/cloud.google.com/go#hdr-Client_Options
	c, err := artifactregistry.NewClient(ctx)
	if err != nil {
		return nil, err
	}

	return &ImageResolver{
		client: c,
	}, nil
}

type ImageResolver struct {
	client *artifactregistry.Client
}

// ResolveImageToSha resolves the image to a sha.
// TODO(jeremy): We need to standardize how not found/doesn't exist errors are returned. We need to support multiple
// registries and resolvers. Right now it will return a notfound Status wrapped in an error
// you can check it using status.Code(err) == codes.NotFound
func (i *ImageResolver) ResolveImageToSha(ref util.DockerImageRef, strategy v1alpha1.Strategy) (util.DockerImageRef, error) {
	if strategy != v1alpha1.MutableTagStrategy {
		return util.DockerImageRef{}, fmt.Errorf("Only MutableTagStrategy is currently implemented for artifact registry")
	}

	image, err := FromImageRef(ref)
	if err != nil {
		return ref, err
	}

	req := &artifactregistrypb.GetTagRequest{
		Name: image.NameForTag(),
	}

	log := zapr.NewLogger(zap.L())
	log.Info("Getting tag", "name", req.Name)
	resp, err := i.client.GetTag(context.Background(), req)
	if err != nil {
		return ref, err
	}

	version := resp.GetVersion()

	pieces := strings.Split(version, "/")
	ref.Sha = pieces[len(pieces)-1]

	return ref, err
}

// IsArtifactRegistry returns true if the URL is a valid artifact registry URL
func IsArtifactRegistry(url string) bool {
	return strings.HasSuffix(url, gcpRegistrySuffix)
}
