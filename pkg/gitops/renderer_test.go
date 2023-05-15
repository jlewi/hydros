package gitops

import (
	"github.com/go-logr/zapr"
	"github.com/jlewi/hydros/api/v1alpha1"
	"github.com/jlewi/hydros/pkg/files"
	"github.com/jlewi/hydros/pkg/github"
	"github.com/jlewi/hydros/pkg/hydros"
	"github.com/pkg/errors"
	"go.uber.org/zap"
	"io"
	"os"
	"testing"
)

func readSecret(secret string) ([]byte, error) {
	f := &files.Factory{}
	h, err := f.Get(secret)
	if err != nil {
		return nil, err
	}
	r, err := h.NewReader(secret)
	if err != nil {
		return nil, err
	}
	return io.ReadAll(r)
}

// This is a manual E2E test mainly intended for development.
func Test_RendererManualE2E(t *testing.T) {
	if os.Getenv("GITHUB_ACTIONS") != "" {
		t.Skipf("Test_RenderManualE2E is a manual test that is skipped in CICD")
	}

	err := func() error {
		log := zapr.NewLogger(zap.L())
		secretURI := "gcpSecretManager:///projects/chat-lewi/secrets/hydros-jlewi/versions/latest"
		secret, err := readSecret(secretURI)
		if err != nil {
			return errors.Wrapf(err, "Could not read file: %v", secretURI)
		}
		manager, err := github.NewTransportManager(int64(hydros.HydrosGitHubAppID), secret, log)
		if err != nil {
			log.Error(err, "TransportManager creation failed")
			return err
		}

		r := Renderer{
			//SourceRepo: &v1alpha1.GitHubRepo{
			//	Org:    "jlewi",
			//	Repo:   "hydros",
			//	Branch: "jlewi/ai",
			//},
			ForkRepo: &v1alpha1.GitHubRepo{
				Org:    "jlewi",
				Repo:   "hydros",
				Branch: "jlewi/ai",
			},
			DestRepo: &v1alpha1.GitHubRepo{
				Org:    "jlewi",
				Repo:   "hydros",
				Branch: "main",
			},
			workDir:    "/tmp/test_renderer",
			repoHelper: nil,
			transports: manager,
			commit:     "",
		}

		return r.Run()
	}()
	if err != nil {
		t.Fatalf("Error running renderer; %v", err)
	}
}