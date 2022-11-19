package ecrutil

import (
	"testing"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/ecr"
	"github.com/google/uuid"
	"github.com/jlewi/hydros/pkg/testutil"
)

// This test depends on access to AWS dev; it will be skipped if the environment variable AWS_TESTS isn't set
func Test_EnsureExists(t *testing.T) {
	testutil.SkipAwsTests(t)
	region := "us-west-2"
	sess, err := session.NewSession(&aws.Config{
		Region: aws.String(region),
	})
	if err != nil {
		t.Fatalf("Could not create AWS session; error: %v", err)
	}

	registry := "12345"
	repo := "testing/ensurerepoexists/test-" + uuid.New().String()[0:10]

	defer func() {
		svc := ecr.New(sess)
		req := &ecr.DeleteRepositoryInput{
			Force:          aws.Bool(true),
			RegistryId:     aws.String(registry),
			RepositoryName: aws.String(repo),
		}
		_, err := svc.DeleteRepository(req)
		if err != nil {
			t.Errorf("Failed to delete repository; repository %v; registry %v; error %v", registry, repo, err)
		}
	}()
	if err := EnsureRepoExists(sess, registry, repo); err != nil {
		t.Fatalf("ensurerepoexists failed when repo doesn't exist; %v", err)
	}

	if err := EnsureRepoExists(sess, registry, repo); err != nil {
		t.Fatalf("EnsureRepoExists failed when repo exists; %v", err)
	}
}
