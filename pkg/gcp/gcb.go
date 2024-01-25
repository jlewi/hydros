package gcp

import (
	cb "cloud.google.com/go/cloudbuild/apiv1"
	cbpb "cloud.google.com/go/cloudbuild/apiv1/v2/cloudbuildpb"
	"cloud.google.com/go/longrunning/autogen/longrunningpb"
	"context"
	"encoding/base64"
	"github.com/go-logr/logr"
	"github.com/go-logr/zapr"
	"github.com/pkg/errors"
	"go.uber.org/zap"
	"strings"
	"time"
)

const (
	kanikoBuilder = "gcr.io/kaniko-project/executor:latest"
)

// BuildImage builds a docker image using GCB
// Blocks until the build is complete
func BuildImage(project string, build *cbpb.Build) (*longrunningpb.Operation, error) {
	client, err := cb.NewClient(context.Background())

	if err != nil {
		return nil, err
	}

	req := &cbpb.CreateBuildRequest{
		ProjectId: project,
		Build:     build,
	}

	op, err := client.CreateBuild(context.Background(), req)
	if err != nil {
		return nil, err
	}

	log := zapr.NewLogger(zap.L())
	log.Info("Build started", "id", op.GetName(), "project", project, "build", build)

	op.GetDone()

	return op, nil
}

// DefaultBuild constructs a default BuildFile
// image should be the URI of the image with out the tag
func DefaultBuild() *cbpb.Build {
	build := &cbpb.Build{
		Steps: []*cbpb.BuildStep{
			{
				Name: kanikoBuilder,
				Args: []string{
					"--dockerfile=Dockerfile",
					"--cache=true",
				},
			},
		},
		Options: &cbpb.BuildOptions{
			MachineType: cbpb.BuildOptions_UNSPECIFIED,
			// Using CLOUD_LOGGING_ONLY means we can't stream the logs (at least not with GCB) but maybe with
			// Cloud Logging? But we shouldn't need that
			Logging: cbpb.BuildOptions_CLOUD_LOGGING_ONLY,
		},
	}

	return build
}

// AddImages adds images to the build
func AddImages(build *cbpb.Build, images []string) error {
	if build.Steps == nil {
		return errors.New("Build.Steps is nil")
	}

	if build.Steps[0].Name != kanikoBuilder {
		return errors.Errorf("Build.Steps[0].Name %s doesn't match expected %s", build.Steps[0].Name, kanikoBuilder)
	}

	existing := make(map[string]bool)

	destFlag := "--destination="

	for _, arg := range build.Steps[0].Args {
		if strings.HasPrefix(arg, destFlag) {
			existing[arg[len(destFlag):]] = true
		}
	}

	for _, image := range images {
		if existing[image] {
			continue
		}

		build.Steps[0].Args = append(build.Steps[0].Args, destFlag+image)
	}
	return nil
}

// OPNameToBuildID converts an operation name to a build id
func OPNameToBuildID(name string) (string, error) {
	// The operation name is of the form projects/<project>/operations/<id>
	// The id will be the build id base64 encoded
	pieces := strings.Split(name, "/")
	buildId64 := pieces[len(pieces)-1]
	buildId, err := base64.StdEncoding.DecodeString(buildId64)
	if err != nil {
		return "", errors.Wrapf(err, "Failed to decode build id %v", buildId64)
	}

	return string(buildId), nil
}

// WaitForBuild waits for a build to complete. Caller should set the deadline on the context.
// On timeout error is nil and the last operation is returned but Done won't be true.
func WaitForBuild(ctx context.Context, client *cb.Client, project string, buildId string) (*cbpb.Build, error) {
	// TODO(jeremy): We should get the logger from the context?
	deadline, ok := ctx.Deadline()
	if !ok {
		// Set a default deadline of 10 minutes
		deadline = time.Now().Add(10 * time.Minute)
	}

	log, err := logr.FromContext(ctx)
	if err != nil {
		log = zapr.NewLogger(zap.L())
	}

	pause := 20 * time.Second

	var last *cbpb.Build
	logged := false
	for time.Now().Before(deadline) {
		req := cbpb.GetBuildRequest{
			ProjectId: project,
			Id:        buildId,
		}

		// N.B. We can't just do opClient.WaitForOp because I think that does a server side wait and will timeout
		// when the http/grpc timeout is reahed.
		last, err := client.GetBuild(ctx, &req)

		if err != nil {
			// TODO(jeremy): We should decide if this is a permanent or retryable error
			log.Error(err, "Failed to get build", "buildId", buildId)

		} else {
			switch last.GetStatus() {
			case cbpb.Build_STATUS_UNKNOWN:
			case cbpb.Build_PENDING:
			case cbpb.Build_QUEUED:
			case cbpb.Build_WORKING:
			default:
				return last, nil
			}
		}

		if !logged && err == nil {
			log.Info("Waiting for build", "buildId", buildId, "logsUrl", last.LogUrl)
			logged = true
		}
		if time.Now().Add(pause).After(deadline) {
			return last, err
		}
		time.Sleep(pause)
		continue

	}

	return last, nil
}
