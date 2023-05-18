package app

import (
	"context"
	"encoding/json"
	"os"
	"testing"
	"time"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-logr/zapr"
	"github.com/google/go-github/v52/github"
	"github.com/gregjones/httpcache"
	hGithub "github.com/jlewi/hydros/pkg/github"
	"github.com/jlewi/hydros/pkg/github/ghrepo"
	"github.com/jlewi/hydros/pkg/hydros"
	"github.com/jlewi/hydros/pkg/util"
	"github.com/palantir/go-githubapp/githubapp"
	"github.com/pkg/errors"
	"go.uber.org/zap"
	"google.golang.org/protobuf/proto"
)

func Test_HookManual(t *testing.T) {
	if os.Getenv("GITHUB_ACTIONS") != "" {
		t.Skip("Skipping test because we are running in GitHub Actions and this is a manual test")
	}

	log := util.SetupLogger("debug", true)
	secretURI := "gcpSecretManager:///projects/chat-lewi/secrets/hydros-jlewi/versions/latest"
	secret, err := readSecret(secretURI)
	if err != nil {
		t.Fatalf("Could not read file: %v; error: %+v", secretURI, err)
	}
	transports, err := hGithub.NewTransportManager(int64(hydros.HydrosGitHubAppID), secret, log)
	if err != nil {
		t.Errorf("Failed to create transport manager; error %v", err)
	}

	repo := ghrepo.New("jlewi", "hydros-hydrated")
	branch := "main"

	commit, err := latestCommit(transports, repo, "/tmp/test_hook_manual_get_repo", branch)
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

	handler, err := NewHandler(cc, transports, "/tmp/hydros_handler_test", 1)
	if err != nil {
		t.Fatalf("Failed to create handler; error %v", err)
	}

	if err := handler.Handle(context.Background(), "push", "1234", payload); err != nil {
		t.Fatalf("Failed to handle the push")
	}

	//time.Sleep(10 * time.Minute)
	// Wait for it to finish processing.
	handler.Manager.Shutdown()
}

// Get the latest commit
func latestCommit(transports *hGithub.TransportManager, repo ghrepo.Interface, workDir string, branch string) (string, error) {
	log := zapr.NewLogger(zap.L())
	url := ghrepo.FormatRemoteURL(repo, "https")

	tr, err := transports.Get(repo.RepoOwner(), repo.RepoName())
	if err != nil {
		return "", nil
	}

	appAuth := &hGithub.AppAuth{
		Tr: tr,
	}
	// Clone the repository if it hasn't already been cloned.
	err = func() error {
		if _, err := os.Stat(workDir); err == nil {
			log.Info("Directory exists; repository will not be cloned", "directory", workDir)
			return nil
		}

		opts := &git.CloneOptions{
			URL:      url,
			Auth:     appAuth,
			Progress: os.Stdout,
		}

		_, err := git.PlainClone(workDir, false, opts)
		return err
	}()

	if err != nil {
		return "", err
	}

	// Open the repository
	gitRepo, err := git.PlainOpenWithOptions(workDir, &git.PlainOpenOptions{})
	if err != nil {
		return "", errors.Wrapf(err, "Could not open respoistory at %v; ensure the directory contains a git repo", workDir)
	}

	// Do a fetch to make sure the remote is up to date.
	remote := "origin"
	log.Info("Fetching remote", "remote", remote)
	if err := gitRepo.Fetch(&git.FetchOptions{
		RemoteName: remote,
		Auth:       appAuth,
	}); err != nil {
		// Fetch returns an error if its already up to date and we want to ignore that.
		if err.Error() != "already up-to-date" {
			return "", err
		}
	}

	// If commit is specified check it out

	hash, err := gitRepo.ResolveRevision(plumbing.Revision(branch))

	if err != nil {
		return "", errors.Wrapf(err, "Could not resolve branch %s", branch)
	}

	log.Info("Checking out branch", "branch", branch)
	w, err := gitRepo.Worktree()
	if err != nil {
		return "", err
	}
	err = w.Checkout(&git.CheckoutOptions{
		Hash:  *hash,
		Force: true,
	})
	if err != nil {
		return "", errors.Wrapf(err, "Failed to checkout branh %s", branch)
	}

	// Get the current commit
	ref, err := gitRepo.Head()
	if err != nil {
		return "", err
	}

	// The short tag will be used to tag the artifacts
	//b.commitTag = ref.Hash().String()[0:7]

	log.Info("Current commit", "commit", ref.Hash().String())
	return ref.Hash().String(), nil
}
