package ghapp

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"testing"
	"time"

	"github.com/jlewi/hydros/pkg/files"

	"github.com/go-logr/zapr"
	"github.com/google/go-github/v52/github"
	"github.com/gregjones/httpcache"
	hGithub "github.com/jlewi/hydros/pkg/github"
	"github.com/jlewi/hydros/pkg/github/ghrepo"
	"github.com/jlewi/hydros/pkg/hydros"
	"github.com/jlewi/hydros/pkg/util"
	"github.com/palantir/go-githubapp/githubapp"
	"go.uber.org/zap"
	"google.golang.org/protobuf/proto"
)

func Test_HookManual(t *testing.T) {
	if os.Getenv("GITHUB_ACTIONS") != "" {
		t.Skip("Skipping test because we are running in GitHub Actions and this is a manual test")
	}

	log := util.SetupLogger("debug", true)
	secretURI := "gcpSecretManager:///projects/chat-lewi/secrets/hydros-jlewi/versions/latest"
	secret, err := files.Read(secretURI)
	if err != nil {
		t.Fatalf("Could not read file: %v; error: %+v", secretURI, err)
	}
	transports, err := hGithub.NewTransportManager(int64(hydros.HydrosGitHubAppID), secret, log)
	if err != nil {
		t.Errorf("Failed to create transport manager; error %v", err)
	}

	repo := ghrepo.New("jlewi", "hydros-hydrated")
	branch := "main"

	commit, err := latestCommit(transports, repo, branch)
	if err != nil {
		t.Fatalf("Could not get latest commit; error: %v", err)
	}

	// N.B. values for the push event were obtained by looking at the webhook sent in the GitHub UI
	// https://github.com/settings/apps/hydros-bot/advanced
	event := &github.PushEvent{
		After: proto.String(commit),
		Ref:   proto.String("refs/heads/" + branch),
		Repo: &github.PushEventRepository{
			FullName: proto.String(ghrepo.FullName(repo)),
			Owner: &github.User{
				Login: proto.String(repo.RepoOwner()),
			},
			Name: proto.String(repo.RepoName()),
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
	)

	if err != nil {
		t.Fatalf("Failed to create client creator; error %v", err)
	}

	handler, err := NewHandler(cc, transports, "/tmp/hydros_handler_test", 1)
	if err != nil {
		t.Fatalf("Failed to create handler; error %v", err)
	}

	if err := handler.Handle(context.Background(), "push", "1234", payload); err != nil {
		t.Fatalf("Failed to handle the push")
	}

	// Wait for it to finish processing.
	handler.Manager.Shutdown()
}

// Get the latest commit
func latestCommit(transports *hGithub.TransportManager, repo ghrepo.Interface, branch string) (string, error) {
	log := zapr.NewLogger(zap.L())

	tr, err := transports.Get(repo.RepoOwner(), repo.RepoName())
	if err != nil {
		return "", nil
	}

	client := github.NewClient(&http.Client{Transport: tr})

	repos := client.Repositories
	b, _, err := repos.GetBranch(context.Background(), repo.RepoOwner(), repo.RepoName(), branch, false)
	if err != nil {
		log.Error(err, "Failed to get branch; unable to determine the latest commit", "branch", branch)
	}

	if b.Commit.SHA == nil {
		err := fmt.Errorf("Branch %v doesn't have a commit SHA", branch)
		log.Error(err, "Failed to get branch; unable to determine the latest commit", "branch", branch)
		return "", err
	}

	return *b.Commit.SHA, nil
}
