package images

import (
	cb "cloud.google.com/go/cloudbuild/apiv1"
	cbpb "cloud.google.com/go/cloudbuild/apiv1/v2/cloudbuildpb"
	longrunning "cloud.google.com/go/longrunning/autogen"
	"context"
	"github.com/jlewi/hydros/api/v1alpha1"
	"github.com/jlewi/hydros/pkg/gcp"
	"github.com/jlewi/hydros/pkg/util"
	"github.com/pkg/errors"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/durationpb"
	"strings"
	"time"
)

// Controller for images. A controller is capable of building images and resolving images to shas.
type Controller struct {
	resolver  *gcp.ImageResolver
	cbClient  *cb.Client
	opsClient *longrunning.OperationsClient
}

func NewController() (*Controller, error) {
	resolver, err := gcp.NewImageResolver(context.Background())
	if err != nil {
		return nil, errors.Wrap(err, "Failed to create image resolver")
	}

	c, err := longrunning.NewOperationsClient(context.Background())
	if err != nil {
		return nil, errors.Wrapf(err, "Failed to create operations client")
	}

	client, err := cb.NewClient(context.Background())

	if err != nil {
		return nil, errors.Wrapf(err, "Failed to create Cloud Build client")
	}

	return &Controller{
		resolver:  resolver,
		opsClient: c,
		cbClient:  client,
	}, nil
}

// Reconcile an image. This will build the image if necessary and resolve the image to a sha.
// Status is updated with status about the image.
func (c *Controller) Reconcile(ctx context.Context, image *v1alpha1.Image) error {
	log := util.LogFromContext(ctx)
	log.Info("Reconciling image", "image", image.Metadata.Name)

	project := image.Spec.Builder.GCB.Project

	if project == "" {
		return errors.New("Can't build image; project must be set")
	}

	if image.Status.SourceCommit == "" {
		return errors.New("Can't build image; sourceCommit must be set")
	}

	imageRef, err := util.ParseImageURL(image.Spec.Image)
	if err != nil {
		log.Error(err, "Failed to parse image.", "image", image.Spec.Image)
		return errors.Wrapf(err, "Failed to parse image: %v", image.Spec.Image)
	}

	if !gcp.IsArtifactRegistry(imageRef.Registry) {
		return errors.Errorf("Image %v is not in Artifact Registry", imageRef)
	}

	// Tag should be the image
	imageRef.Tag = image.Status.SourceCommit

	// Check if the image already exists
	resolved, err := c.resolver.ResolveImageToSha(*imageRef, v1alpha1.MutableTagStrategy)

	if err == nil {
		log.Info("Image already exists", "image", image.Spec.Image, "sha", resolved.Sha)
		image.Status.URI = resolved.ToURL()
		image.Status.SHA = resolved.Sha
		return nil
	}

	if status.Code(err) != codes.NotFound {
		log.Error(err, "There was an error checking if the image already exists")
		return err
	}

	log.Info("Image doesn't exist; building", "image", image.Spec.Image)

	build := gcp.DefaultBuild()

	imageBase := image.Spec.Image
	images := []string{
		imageBase + ":" + image.Status.SourceCommit,
		imageBase + ":latest",
	}
	gcp.AddImages(build, images)

	if image.Spec.Builder.GCB.MachineType != "" {
		val, ok := cbpb.BuildOptions_MachineType_value[image.Spec.Builder.GCB.MachineType]
		if !ok {
			allowed := make([]string, 0, len(cbpb.BuildOptions_MachineType_value))
			for k := range cbpb.BuildOptions_MachineType_value {
				allowed = append(allowed, k)
			}
			return errors.Errorf("Invalid machine type %v; allowed values: %s", image.Spec.Builder.GCB.MachineType, strings.Join(allowed, ", "))
		}
		build.Options.MachineType = cbpb.BuildOptions_MachineType(val)
	}

	if image.Spec.Builder.GCB.Timeout != "" {
		t, err := time.ParseDuration(image.Spec.Builder.GCB.Timeout)
		if err != nil {
			return errors.Wrapf(err, "Invalid timeout %v; value must satisfy time.ParseDuration", image.Spec.Builder.GCB.Timeout)
		}

		build.Timeout = durationpb.New(t)
	}

	req := &cbpb.CreateBuildRequest{
		ProjectId: project,
		Build:     build,
	}

	op, err := c.cbClient.CreateBuild(context.Background(), req)
	if err != nil {
		return err
	}

	// The operation name is of the form projects/<project>/operations/<id>
	// THe id will be the build id base64 encoded
	buildId, err := gcp.OPNameToBuildID(op.GetName())
	if err != nil {
		return errors.Wrapf(err, "Failed to decode build id %v", op.GetName())
	}

	log.Info("Build started", "id", op.GetName(), "project", project, "buildId", buildId, "operation", op.GetName())

	opCtx, _ := context.WithTimeout(ctx, 1*time.Hour)
	finalBuild, err := gcp.WaitForBuild(opCtx, c.cbClient, project, buildId)

	if err != nil {
		return errors.Wrapf(err, "Failed to wait for GCB build operation")
	}

	if finalBuild.Status != cbpb.Build_SUCCESS {
		log.Info("Build failed", "project", project, "buildId", buildId, "logsUrl", finalBuild.LogUrl)
		return errors.Errorf("Build failed with status %v", finalBuild.Status)
	}

	return nil
}
