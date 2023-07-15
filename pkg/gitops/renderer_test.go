package gitops

import (
	"os"
	"testing"

	"github.com/go-logr/zapr"
	"github.com/jlewi/hydros/api/v1alpha1"
	"github.com/jlewi/hydros/pkg/files"
	"github.com/jlewi/hydros/pkg/github"
	"github.com/jlewi/hydros/pkg/hydros"
	"github.com/jlewi/hydros/pkg/util"
	"github.com/pkg/errors"
	"go.uber.org/zap"
)

// This is a manual E2E test mainly intended for development.
func Test_RendererManualE2E(t *testing.T) {
	if os.Getenv("GITHUB_ACTIONS") != "" {
		t.Skipf("Test_RenderManualE2E is a manual test that is skipped in CICD")
	}

	err := func() error {
		util.SetupLogger("info", true)
		log := zapr.NewLogger(zap.L())
		secretURI := "gcpSecretManager:///projects/chat-lewi/secrets/hydros-jlewi/versions/latest"
		secret, err := files.Read(secretURI)
		if err != nil {
			return errors.Wrapf(err, "Could not read file: %v", secretURI)
		}
		manager, err := github.NewTransportManager(int64(hydros.HydrosGitHubAppID), secret, log)
		if err != nil {
			log.Error(err, "TransportManager creation failed")
			return err
		}

		org := "jlewi"
		repo := "hydros-hydrated"

		r, err := NewRenderer(org, repo, "/tmp/render_test_manual", manager)
		if err != nil {
			return err
		}

		event := RenderEvent{
			Commit: "",
			BranchConfig: &v1alpha1.InPlaceConfig{
				BaseBranch: "main",
				PRBranch:   "hydros/manual-test",
				AutoMerge:  false,
			},
		}
		return r.Run(event)
	}()
	if err != nil {
		t.Fatalf("Error running renderer; %+v", err)
	}
}
