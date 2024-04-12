package github

import (
	"context"
	"github.com/jlewi/hydros/api/v1alpha1"
	"github.com/jlewi/hydros/pkg/config"
	"go.uber.org/zap"
	"os"
	"testing"
)

func Test_Releaser(t *testing.T) {
	if os.Getenv("GITHUB_ACTIONS") == "true" {
		t.Skip("Skip test on GitHub Actions")
	}

	if err := config.InitViper(nil); err != nil {
		t.Fatalf("Failed to initialize viper: %v", err)
	}
	cfg := config.GetConfig()

	c := zap.NewDevelopmentConfig()
	c.Level.SetLevel(zap.InfoLevel)
	logger, err := c.Build()
	if err != nil {
		t.Fatalf("Failed to create logger: %v", err)
	}
	zap.ReplaceGlobals(logger)

	releaser, err := NewReleaser(*cfg)

	if err != nil {
		t.Fatalf("Failed to create Releaser: %v", err)
	}

	resource := &v1alpha1.GitHubReleaser{
		Metadata: v1alpha1.Metadata{
			Name: "test",
		},
		Spec: v1alpha1.GitHubReleaserSpec{
			Org:  "jlewi",
			Repo: "hydros-hydrated",
		},
	}
	if err := releaser.Reconcile(context.Background(), resource); err != nil {
		t.Fatalf("Failed to reconcile release: %+v", err)
	}
}
