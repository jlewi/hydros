package gcp

import (
	cb "cloud.google.com/go/cloudbuild/apiv1"
	cbpb "cloud.google.com/go/cloudbuild/apiv1/v2/cloudbuildpb"
	"cloud.google.com/go/longrunning/autogen/longrunningpb"
	"context"
	"encoding/json"
	"github.com/go-logr/zapr"
	"github.com/pkg/errors"
	"go.uber.org/zap"
	"google.golang.org/protobuf/encoding/protojson"
	"gopkg.in/yaml.v3"
	"os"
)

// BuildImage builds a docker image using GCB
func BuildImage(buildFile string, project string, sourceCommit string) (*longrunningpb.Operation, error) {
	client, err := cb.NewClient(context.Background())

	if err != nil {
		return nil, err
	}

	build, err := parseBuildFile(buildFile)
	if err != nil {
		return nil, err
	}

	build.Source = &cbpb.Source{}
	build.Substitutions["COMMIT_SHA"] = sourceCommit
	req := &cbpb.CreateBuildRequest{
		ProjectId: project,
		Build:     build,
	}

	op, err := client.CreateBuild(context.Background(), req)
	if err != nil {
		return nil, err
	}

	log := zapr.NewLogger(zap.L())
	log.Info("Build started", "id", op.GetName(), "project", project, "sourceCommit", sourceCommit, "buildFile", buildFile)

	return op, nil
}

func parseBuildFile(buildFile string) (*cbpb.Build, error) {
	b, err := os.ReadFile(buildFile)
	if err != nil {
		return nil, errors.Wrapf(err, "Failed to read build file: %v", buildFile)
	}

	parsed := map[string]interface{}{}
	if err := yaml.Unmarshal(b, &parsed); err != nil {
		return nil, errors.Wrapf(err, "Failed to parse build file: %v", buildFile)
	}

	jsonData, err := json.Marshal(parsed)
	if err != nil {
		return nil, errors.Wrapf(err, "Failed to marshal build file: %v", buildFile)
	}

	build := &cbpb.Build{}
	if err := protojson.Unmarshal(jsonData, build); err != nil {
		return build, errors.Wrapf(err, "Failed to unmarshal build file: %v", buildFile)
	}
	return build, nil
}
