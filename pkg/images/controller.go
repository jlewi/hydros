package images

import (
	"context"
	"fmt"
	"github.com/jlewi/hydros/pkg/github/ghrepo"
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

// TODO(jeremy): Is this neccessary? Can we retrieve the repo from the work tree?
// I don't think we want to always reconstruct the workTree from gitRepos because that could be expensive
// in particular updating the ignore patterns is expensive.
type gitRepoRef struct {
	repo *git.Repository
	w    *git.Worktree
}

// Controller for images. A controller is capable of building images and resolving images to shas.
type Controller struct {
	resolver  *gcp.ImageResolver
	cbClient  *cb.Client
	opsClient *longrunning.OperationsClient
	gcsClient *storage.Client

	// pointers to one or more repositories that have already been cloned.
	localRepos []gitRepoRef
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
		resolver:   resolver,
		opsClient:  c,
		cbClient:   client,
		gcsClient:  gcsClient,
		localRepos: make([]gitRepoRef, 0),
	}, nil
}

// Reconcile an image. This will build the image if necessary and resolve the image to a sha.
// Status is updated with status about the image.
// basePath is the basePath to resolve paths against
func (c *Controller) Reconcile(ctx context.Context, image *v1alpha1.Image) error {
	log := util.LogFromContext(ctx)
	log.Info("Reconciling image", "image", image.Metadata.Name)

	if errs, valid := image.IsValid(); !valid {
		return errors.New(errs)
	}

	project := image.Spec.Builder.GCB.Project
	bucket := image.Spec.Builder.GCB.Bucket

	if image.Status.SourceCommit == "" {
		return errors.New("Can't build image; sourceCommit must be set")
	}

	imageRef, err := util.ParseImageURL(image.Spec.Image)
	if err != nil {
		log.Error(err, "Failed to parse image.", "image", image.Spec.Image)
		return errors.Wrapf(err, "Failed to parse image: %v", image.Spec.Image)
	}

	if !gcp.IsArtifactRegistry(imageRef.Registry) {
		return errors.Errorf("URI %v is not in Artifact Registry", imageRef)
	}

	// Tag should be the image
	imageRef.Tag = image.Status.SourceCommit

	// Check if the image already exists
	resolved, err := c.resolver.ResolveImageToSha(*imageRef, v1alpha1.MutableTagStrategy)

	if err == nil {
		log.Info("URI already exists", "image", image.Spec.Image, "sha", resolved.Sha)
		image.Status.URI = resolved.ToURL()
		image.Status.SHA = resolved.Sha
		return nil
	}

	if status.Code(err) != codes.NotFound {
		log.Error(err, "There was an error checking if the image already exists")
		return err
	}

	// Replace remotes with local directories if the remotes correspond to the current directory
	if err := c.replaceRemotes(ctx, image); err != nil {
		return errors.Wrapf(err, "Failed to replace remotes")
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

	exists, err := gcsHelper.Exists(gcsPath.ToURI())
	if err != nil {
		return errors.Wrapf(err, "Failed to check if tarball exists %s", gcsPath.ToURI())
	}
	tarFilePath := gcsPath.ToURI()
	if !exists {
		log.Info("Creating tarball", "image", image.Spec.Image, "tarball", tarFilePath)

		// N.B. we need export any docker images specified as sources
		// This will rewrite the image.Spec.ImageSource to point to the tarballs
		transformed, err := c.exportImages(ctx, image)
		if err != nil {
			return err
		}

		if err := tarutil.Build(transformed, tarFilePath); err != nil {
			return errors.Wrapf(err, "Failed to create tarball %s", tarFilePath)
		}
	} else {
		log.Info("Tarball exists", "image", image.Spec.Image, "tarball", tarFilePath)
	}

	log.Info("URI doesn't exist; building", "image", image.Spec.Image, "imageRef", imageRef)

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
func (c *Controller) exportImages(ctx context.Context, image *v1alpha1.Image) ([]*v1alpha1.ImageSource, error) {
	log := util.LogFromContext(ctx)

	tarResults := make([]*v1alpha1.ImageSource, 0, len(image.Spec.Source))

	tmpDir, err := os.MkdirTemp("", "hydrosImageReconciler")
	if err != nil {
		return tarResults, errors.Wrapf(err, "Failed to create temp dir")
	}

	exportErrs := make([]error, len(image.Spec.Source))
	numToExport := 0
	var wg sync.WaitGroup
	for i, source := range image.Spec.Source {
		if !util.IsDockerURI(source.URI) {
			tarResults = append(tarResults, source)
			continue
		}

		imageRef, err := util.ParseImageURL(source.URI)
		if err != nil {
			log.Error(err, "failed to parse image URL", "sourceImage", source.URI)
			return tarResults, err
		}

		if imageRef.Tag == "" {
			log.Info("URI doesn't have a tag; setting to sourceCommit", "image", imageRef)
			imageRef.Tag = image.Status.SourceCommit
		}

		imageURI := imageRef.ToURL()

		// Construct path to where the image will be saved on disk
		name := imageRef.Registry + "_" + imageRef.Repo + "_" + imageRef.Tag
		name = strings.Replace(name, "/", "_", -1) + ".tar"

		imagePath := path.Join(tmpDir, name)

		newSource := *source
		newSource.URI = "file://" + imagePath
		tarResults = append(tarResults, &newSource)
		numToExport += 1
		wg.Add(1)
		// Download the images in parallel
		go func(index int, imageUri, path string) {
			defer wg.Done()
			exportErrs[index] = nil
			log.Info("Exporting image", "image", imageUri, "imagePath", path)
			if err := ExportImage(imageUri, path); err != nil {
				log.Error(err, "Failed to export image", "image", imageUri, "path", path)
				exportErrs[index] = err
			}
		}(i, imageURI, imagePath)
	}

	wg.Wait()

	for i := 0; i < numToExport; i++ {
		err := exportErrs[i]
		if err != nil {
			return tarResults, errors.Wrapf(err, "Failed to export one or more images; check logs to see which one")
		}
	}

	return tarResults, nil
}

// replaceRemotes looks for all the images using a git repository and if it correspods to the current directory
// then it replaces the remotes with the location of the gitRoot
func (c *Controller) replaceRemotes(ctx context.Context, image *v1alpha1.Image) error {
	log := util.LogFromContext(ctx)
	for i, s := range image.Spec.Source {
		if !strings.HasSuffix(s.URI, ".git") {
			continue
		}

		sourceRepo, err := ghrepo.FromFullName(s.URI)
		if err != nil {
			return errors.Wrapf(err, "Failed to parse source URI; %v", s.URI)
		}

		for _, ref := range c.localRepos {
			remotes, err := ref.repo.Remotes()
			if err != nil {
				return errors.Wrapf(err, "Error getting remotes")
			}
			gitRoot := ref.w.Filesystem.Root()
			replaceErr := func() error {
				for _, r := range remotes {
					for _, u := range r.Config().URLs {
						remote, err := ghrepo.FromFullName(u)
						if err != nil {
							return errors.Wrapf(err, "Could not parse URL for remote repository name:%v url:%v", r.Config().Name, u)
						}
						if ghrepo.IsSame(sourceRepo, remote) {
							log.Info("Replacing git image source with local directory", "sourceUri", s.URI, "remote", r.Config().Name, "url", u, "gitRoot", gitRoot)
							image.Spec.Source[i].URI = "file://" + gitRoot
							return nil
						}
					}
				}
				return nil
			}()

			if replaceErr != nil {
				return replaceErr
			}
		}

	}
	return nil
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
	c.localRepos = append(c.localRepos, gitRepoRef{repo: gitRepo, w: w})
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
		if err := c.Reconcile(ctx, image); err != nil {
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
