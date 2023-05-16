package app

import (
	"context"
	"encoding/json"
	"github.com/google/go-github/v52/github"
	"github.com/gregjones/httpcache"
	"github.com/jlewi/hydros/pkg/hydros"
	"github.com/jlewi/hydros/pkg/util"
	"github.com/palantir/go-githubapp/githubapp"
	"google.golang.org/protobuf/proto"
	"os"
	"testing"
	"time"
)

func Test_HookManual(t *testing.T) {
	if os.Getenv("GITHUB_ACTIONS") != "" {
		t.Skip("Skipping test because we are running in GitHub Actions and this is a manual test")
	}

	util.SetupLogger("debug", true)

	fullName := "jlewi/hydros-hydrated"

	// N.B. values for the push event were obtained by looking at the webhook sent in the GitHub UI
	// https://github.com/settings/apps/hydros-bot/advanced
	event := &github.PushEvent{
		After: proto.String("2422841179bc6928be43f9d0108632c673c87364"),
		Ref:   proto.String("refs/heads/jlewi"),
		Repo: &github.PushEventRepository{
			FullName: &fullName,
			Owner: &github.User{
				Login: proto.String("jlewi"),
			},
			Name: proto.String("hydros-hydrated"),
		},
		Installation: &github.Installation{
			ID: proto.Int64(31632014),
		},
	}

	payload, err := json.Marshal(event)

	if err != nil {
		t.Fatalf("Failed to marshal payload; error %v", err)
	}

	webhookSecret := "gcpSecretManager:///projects/chat-lewi/secrets/hydros-webhook/versions/latest"
	privateKeySecret := "gcpSecretManager:///projects/chat-lewi/secrets/hydros-jlewi/versions/latest"
	config, err := BuildConfig(hydros.HydrosGitHubAppID, webhookSecret, privateKeySecret)
	if err != nil {
		t.Fatalf("Failed to create config; error %v", err)
	}
	cc, err := githubapp.NewDefaultCachingClientCreator(
		*config,
		githubapp.WithClientUserAgent(UserAgent),
		githubapp.WithClientTimeout(3*time.Second),
		githubapp.WithClientCaching(false, func() httpcache.Cache { return httpcache.NewMemoryCache() }),
		//githubapp.WithClientMiddleware(
		//	githubapp.ClientMetrics(metricsRegistry),
		//),
	)

	handler := &HydrosHandler{
		ClientCreator: cc,
	}

	if err := handler.Handle(context.Background(), "push", "1234", payload); err != nil {
		t.Fatalf("Failed to handle the push")
	}
	//
	//if waitTimeout(&s.wg, time.Minute) {
	//	t.Fatalf("Timeout waiting for syncer's process to be called twice")
	//}
}
