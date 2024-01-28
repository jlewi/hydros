package images

import (
	"context"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"strings"
	"sync"
	"time"

	cb "cloud.google.com/go/cloudbuild/apiv1"
	cbpb "cloud.google.com/go/cloudbuild/apiv1/v2/cloudbuildpb"
	longrunning "cloud.google.com/go/longrunning/autogen"
	"cloud.google.com/go/storage"
	"github.com/go-git/go-git/v5"
	"github.com/go-logr/zapr"
	"github.com/jlewi/hydros/api/v1alpha1"
	"github.com/jlewi/hydros/pkg/gcp"
	"github.com/jlewi/hydros/pkg/gitutil"
	"github.com/jlewi/hydros/pkg/tarutil"
	"github.com/jlewi/hydros/pkg/util"
	"github.com/jlewi/monogo/gcp/gcs"
	"github.com/jlewi/monogo/helpers"
	"github.com/pkg/errors"
	"go.uber.org/zap"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/durationpb"
	"gopkg.in/yaml.v3"
)

// Controller for images. A controller is capable of building images and resolving images to shas.
type Controller struct {
	resolver  *gcp.ImageResolver
	cbClient  *cb.Client
	opsClient *longrunning.OperationsClient
	gcsClient *storage.Client
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

	ctx := context.Background()
	gcsClient, err := storage.NewClient(ctx)

	if err != nil {
		return nil, errors.Wrapf(err, "Failed to create GCS storage client")
	}

	return &Controller{
		resolver:  resolver,
		opsClient: c,
		cbClient:  client,
		gcsClient: gcsClient,
	}, nil
}

// Reconcile an image. This will build the image if necessary and resolve the image to a sha.
// Status is updated with status about the image.
// basePath is the basePath to resolve paths against
func (c *Controller) Reconcile(ctx context.Context, image *v1alpha1.Image, basePath string) error {
	log := util.LogFromContext(ctx)
	log.Info("Reconciling image", "image", image.Metadata.Name)

	project := image.Spec.Builder.GCB.Project
	bucket := image.Spec.Builder.GCB.Bucket

	if project == "" {
		return errors.New("Can't build image; project must be set")
	}

	if bucket == "" {
		return errors.New("Can't build image; bucket must be set")
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

	// Create the tarball
	gcsPath := gcs.GcsPath{
		Bucket: image.Spec.Builder.GCB.Bucket,
		Path:   fmt.Sprintf("%s.%s.tgz", imageRef.Repo, image.Status.SourceCommit),
	}

	gcsHelper := gcs.GcsHelper{
		Client: c.gcsClient,
		Ctx:    ctx,
	}

	// TODO(jeremy): We should check if there are any sources which are docker images and if they are and they
	// are missing a tag we should add the sourceCommit tag.

	exists, err := gcsHelper.Exists(gcsPath.ToURI())
	if err != nil {
		return errors.Wrapf(err, "Failed to check if tarball exists %s", gcsPath.ToURI())
	}
	tarFilePath := gcsPath.ToURI()
	if !exists {
		log.Info("Creating tarball", "image", image.Spec.Image, "tarball", tarFilePath)

		tarSources := make([]*tarutil.TarSource, 0, 1+len(image.Spec.ImageSource))

		if len(image.Spec.ImageSource) > 0 {

			sources, err := c.exportImages(ctx, image)
			if err != nil {
				return err
			}
			tarSources = append(tarSources, sources...)
		}

		if len(image.Spec.Source) > 0 {
			tarSources = append(tarSources, &tarutil.TarSource{
				Path:   basePath,
				Source: image.Spec.Source,
			})
		}
		if err := tarutil.Build(tarSources, tarFilePath); err != nil {
			return errors.Wrapf(err, "Failed to create tarball %s", tarFilePath)
		}
	} else {
		log.Info("Tarball exists", "image", image.Spec.Image, "tarball", tarFilePath)
	}

	log.Info("Image doesn't exist; building", "image", image.Spec.Image, "imageRef", imageRef)

	build := gcp.DefaultBuild()

	imageBase := image.Spec.Image

	now := time.Now()
	version := now.Format("v20060102T150405")
	images := []string{
		imageBase + ":" + image.Status.SourceCommit,
		imageBase + ":latest",
		imageBase + ":" + version,
	}

	gcp.AddImages(build, images)
	gcp.AddBuildTags(build, image.Status.SourceCommit, version)
	build.Source = &cbpb.Source{
		Source: &cbpb.Source_StorageSource{
			StorageSource: &cbpb.StorageSource{
				Bucket: bucket,
				Object: gcsPath.Path,
			},
		},
	}

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

// exportImages downloads any images specified as sources
func (c *Controller) exportImages(ctx context.Context, image *v1alpha1.Image) ([]*tarutil.TarSource, error) {
	log := util.LogFromContext(ctx)

	tarResults := make([]*tarutil.TarSource, 0, len(image.Spec.ImageSource))
	if len(image.Spec.ImageSource) == 0 {
		return tarResults, nil
	}

	tmpDir, err := os.MkdirTemp("", "hydrosImageReconciler")
	if err != nil {
		return tarResults, errors.Wrapf(err, "Failed to create temp dir")
	}

	exportErrs := make([]error, len(image.Spec.ImageSource))

	var wg sync.WaitGroup
	for i, source := range image.Spec.ImageSource {
		if source.Image == "" {
			return tarResults, errors.Errorf("ImageSource must specify an image")
		}
		imageRef, err := util.ParseImageURL(source.Image)
		if err != nil {
			log.Error(err, "failed to parse image URL", "sourceImage", source.Image)
			return tarResults, err
		}

		// Construct path to where the image will be saved on disk
		name := imageRef.Registry + "_" + imageRef.Repo + "_" + imageRef.Tag
		name = strings.Replace(name, "/", "_", -1) + ".tar"

		imagePath := path.Join(tmpDir, name)

		tarResults = append(tarResults, &tarutil.TarSource{
			Path:   imagePath,
			Source: source.Source,
		})
		wg.Add(1)
		// Download the images in parallel
		go func(index int, s v1alpha1.ImageSource, path string) {
			defer wg.Done()
			exportErrs[index] = nil
			log.Info("Exporting image", "image", s.Image, "imagePath", path)
			if err := ExportImage(source.Image, source.Image); err != nil {
				log.Error(err, "Failed to export image", "image", s.Image, "path", path)
				exportErrs[index] = err
			}
		}(i, *source, imagePath)
	}

	wg.Wait()

	for _, err := range exportErrs {
		if err != nil {
			return tarResults, errors.Wrapf(err, "Failed to export one or more images; check logs to see which one")
		}
	}

	return tarResults, nil
}

// ReconcileFile reconciles the images defined in a set of files.
// It is a helper function primarily used by the CLI
func ReconcileFile(path string) error {
	log := zapr.NewLogger(zap.L())

	manifestPath, err := filepath.Abs(path)
	if err != nil {
		return errors.Wrapf(err, "Failed to get absolute path for %v", path)
	}

	basePath := filepath.Dir(manifestPath)
	log.Info("Resolved manifest path", "manifestPath", manifestPath, "basePath", basePath)

	f, err := os.Open(manifestPath)
	if err != nil {
		return errors.Wrapf(err, "Failed to open file: %v", manifestPath)
	}

	gitRoot, err := gitutil.LocateRoot(path)
	if err != nil {
		return errors.Wrapf(err, "Failed to locate git root for %v", path)
	}

	gitRepo, err := git.PlainOpenWithOptions(gitRoot, &git.PlainOpenOptions{})
	if err != nil {
		return errors.Wrapf(err, "Error opening git repo")
	}

	w, err := gitRepo.Worktree()
	if err != nil {
		return errors.Wrapf(err, "Error getting worktree")
	}

	if err := gitutil.AddGitignoreToWorktree(w, gitRoot); err != nil {
		return errors.Wrapf(err, "Failed to add gitignore patterns")
	}

	// Commit any changes. Do this before calling headRef
	if err := gitutil.CommitAll(gitRepo, w, "hydros committing changes before build"); err != nil {
		return err
	}

	headRef, err := gitRepo.Head()
	if err != nil {
		return errors.Wrapf(err, "Error getting head ref")
	}

	gitStatus, err := w.Status()
	if err != nil {
		return errors.Wrapf(err, "Error getting git status")
	}

	d := yaml.NewDecoder(f)

	c, err := NewController()

	if err != nil {
		return errors.Wrapf(err, "Error creating controller")
	}

	failures := &helpers.ListOfErrors{}
	for {
		image := &v1alpha1.Image{}
		if err := d.Decode(image); err != nil {
			if err == io.EOF {
				return nil
			} else {
				return errors.Wrapf(err, "Failed to decode image from file %v", manifestPath)
			}
		}

		image.Status.SourceCommit += headRef.Hash().String()

		if !gitStatus.IsClean() {
			log.Info("Git status is not clean; image will be tagged -dirty")
			image.Status.SourceCommit += "-dirty"
		}

		ctx := context.Background()
		if err := c.Reconcile(ctx, image, basePath); err != nil {
			log.Error(err, "Failed to reconcile image", "image", image)
			// Keep going
			failures.AddCause(err)
		}
	}

	if len(failures.Causes) > 0 {
		failures.Final = errors.Errorf("Failed to reconcile %d images", len(failures.Causes))
		return failures
	}
	return nil
}
