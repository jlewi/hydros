package gcp

import (
	"context"
	"encoding/base64"
	"strings"
	"time"

	cb "cloud.google.com/go/cloudbuild/apiv1"
	cbpb "cloud.google.com/go/cloudbuild/apiv1/v2/cloudbuildpb"
	"cloud.google.com/go/longrunning/autogen/longrunningpb"
	"github.com/go-logr/logr"
	"github.com/go-logr/zapr"
	"github.com/pkg/errors"
	"go.uber.org/zap"
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
	now := time.Now()
	nowStr := now.Format(time.RFC3339)
	build := &cbpb.Build{
		Steps: []*cbpb.BuildStep{
			{
				Name: kanikoBuilder,
				Args: []string{
					"--cache=true",
					// Set the date as a build arg
					// This is so that it can be passed to the builder and used to set the date in the image
					// of the build
					"--build-arg=DATE=" + nowStr,
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

	destFlag := "--destination="

	args := make([]string, 0, len(images))

	for _, i := range images {
		args = append(args, destFlag+i)
	}

	return AddKanikoArgs(build, args)
	return nil
}

// AddKanikoArgs adds a build arg to the build
// null-op if its already added
func AddKanikoArgs(build *cbpb.Build, buildArgs []string) error {
	if build.Steps == nil {
		return errors.New("Build.Steps is nil")
	}

	if build.Steps[0].Name != kanikoBuilder {
		return errors.Errorf("Build.Steps[0].Name %s doesn't match expected %s", build.Steps[0].Name, kanikoBuilder)
	}

	existing := make(map[string]bool)

	for _, arg := range build.Steps[0].Args {
		existing[arg] = true
	}

	for _, a := range buildArgs {
		if existing[a] {
			continue
		}

		build.Steps[0].Args = append(build.Steps[0].Args, a)
	}
	return nil
}

// AddBuildTags passes various values as build flags to the build
func AddBuildTags(build *cbpb.Build, sourceCommit string, version string) error {
	args := []string{
		// Pass the values along to Docker
		"--build-arg=COMMIT=" + sourceCommit,
		"--build-arg=VERSION=" + version,
		// Build labels; Does this do anything
		"--label=COMMIT=" + sourceCommit,
		"--label=COMMIT=" + version,
	}

	return AddKanikoArgs(build, args)
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
