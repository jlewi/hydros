package app

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/go-logr/zapr"
	"github.com/google/go-github/v52/github"
	"github.com/jlewi/hydros/api/v1alpha1"
	hGithub "github.com/jlewi/hydros/pkg/github"
	"github.com/jlewi/hydros/pkg/github/ghrepo"
	"github.com/jlewi/hydros/pkg/gitops"
	"github.com/jlewi/hydros/pkg/util"
	"github.com/palantir/go-githubapp/appconfig"
	"github.com/palantir/go-githubapp/githubapp"
	"github.com/pkg/errors"
	"go.uber.org/zap"
	"google.golang.org/protobuf/proto"
)

const (
	// HydrosConfigPath default path to look for the hydros repository configuration file.
	// TODO(jeremy): We should expose this as a configuration option for hydros.
	HydrosConfigPath = "hydros.yaml"
	// SharedRepository is the name of the repository containing the shared hydros configurationf or all repositories
	SharedRepository = ".github"
)

type HydrosHandler struct {
	githubapp.ClientCreator
	Manager *gitops.Manager
	// TODO(jeremy): ClientCreator and TransportManager are somewhat redundant
	transports *hGithub.TransportManager

	workDir string

	fetcher *ConfigFetcher
}

// NewHandler starts a new HydrosHandler for GitHub.
// config: The GitHub configuration
// workDir: The directory to use for storing temporary files and checking out repositories
// numWorkers: The number of workers to use for processing events
func NewHandler(cc githubapp.ClientCreator, transports *hGithub.TransportManager, workDir string, numWorkers int) (*HydrosHandler, error) {
	log := zapr.NewLogger(zap.L())

	fetcher := &ConfigFetcher{Loader: appconfig.NewLoader(
		[]string{HydrosConfigPath},
		appconfig.WithOwnerDefault(SharedRepository, []string{
			HydrosConfigPath,
		}),
	)}

	if workDir == "" {
		tDir, err := os.MkdirTemp("", "hydros")
		if err != nil {
			return nil, errors.Wrapf(err, "Failed to create temporary directory")
		}
		log.Info("Setting workDir to temporary directory", "workDir", tDir)
		workDir = tDir
	}

	manager, err := gitops.NewManager([]gitops.Reconciler{})
	if err != nil {
		return nil, err
	}

	if err := manager.Start(numWorkers, 1*time.Hour); err != nil {
		return nil, err
	}

	handler := &HydrosHandler{
		ClientCreator: cc,
		transports:    transports,
		fetcher:       fetcher,
		workDir:       workDir,
		Manager:       manager,
	}

	return handler, nil
}

func (h *HydrosHandler) Handles() []string {
	return []string{"push"}
}

func (h *HydrosHandler) Handle(ctx context.Context, eventType, deliveryID string, payload []byte) error {
	// TODO(jeremy): How do we distinguish between pushing a PR and merging a PR.
	log := zapr.NewLogger(zap.L()).WithValues("eventType", eventType, "deliverID", deliveryID)
	log.V(util.Debug).Info("Got github webhook")
	r := bytes.NewBuffer(payload)
	d := json.NewDecoder(r)
	event := &github.PushEvent{}
	if err := d.Decode(event); err != nil {
		log.Error(err, "Failed to decode a PushEvent")
		return err
	}

	repo := event.GetRepo()
	action := event.GetAction()
	log.Info("Got push event", "url", repo.GetURL(), "action", action)

	repoName, err := ghrepo.FromFullName(repo.GetFullName())
	if err != nil {
		return err
	}

	// TODO(jeremy): We can use the checks client to create checks.
	installationID := githubapp.GetInstallationIDFromEvent(event)
	client, err := h.NewInstallationClient(installationID)
	if err != nil {
		return err
	}

	refsPrefix := "refs/heads/"
	branch := event.GetRef()[len(refsPrefix):]

	config := h.fetcher.ConfigForRepositoryBranch(context.Background(), client, repoName.RepoOwner(), repoName.RepoName(), branch)

	if config.LoadError != nil {
		log.Error(config.LoadError, "Error loading config")
		return config.LoadError
	}
	if config.Config == nil {
		log.V(util.Debug).Info("No config found", "owner", repoName.RepoOwner(), "repo", repoName.RepoName(), "branch", branch)
		return nil
	}

	if msg, ok := v1alpha1.IsValid(config.Config); !ok {
		log.Error(errors.Errorf(msg), "Invalid configuration", repoName.RepoOwner(), "repo", repoName.RepoName(), "branch", branch)
		_, _, err := client.Checks.CreateCheckRun(ctx, repoName.RepoOwner(), repoName.RepoName(), github.CreateCheckRunOptions{
			Name:       "hydros",
			HeadSHA:    event.GetAfter(),
			DetailsURL: proto.String("https://url.not.set.yet"),
			Status:     proto.String("completed"),
			Conclusion: proto.String("failure"),
			Output: &github.CheckRunOutput{
				Title:   proto.String("Hydros failed"),
				Summary: proto.String("Hydros invalid configuration"),
				Text:    proto.String(fmt.Sprintf("Hydros failed because config file, %v, is invalid. %s", HydrosConfigPath, msg)),
			},
		})
		if err != nil {
			log.Error(err, "Failed to create check run")
		}
		return errors.Errorf(msg)
	}
	// TODO(jeremy): Payload in push_event contains a list of added/removed/modified files. We could use that
	// to determine whether hydros needs to run.

	// Check if its a branch for which we do in place configuration.
	var inPlaceConfig *v1alpha1.InPlaceConfig
	for _, inPlace := range config.Config.Spec.InPlaceConfigs {
		if inPlace.BaseBranch == branch {
			inPlaceConfig = &inPlace
			break
		}
	}

	if inPlaceConfig == nil {
		log.Info("branch isn't configured for inplace changes.  Skipping", "ref", event.GetRef())
		// Update the PR with a CreateCheckRun
		check, response, err := client.Checks.CreateCheckRun(ctx, repoName.RepoOwner(), repoName.RepoName(), github.CreateCheckRunOptions{
			Name:       "hydros",
			HeadSHA:    event.GetAfter(),
			DetailsURL: proto.String("https://url.not.set.yet"),
			Status:     proto.String("completed"),
			Conclusion: proto.String("skipped"),
			Output: &github.CheckRunOutput{
				Title:   proto.String("Hydros skipped"),
				Summary: proto.String("Hydros skipped"),
				Text:    proto.String("Hydros skipped because this is not the main branch."),
			},
		})

		if err != nil {
			log.Error(err, "Failed to create check")
			return err
		}

		log.Info("Created check", "check", check, "response", response)
		return nil
	}

	// Determine the name for the reconciler
	// It should be unique for each repo and also particular type of reconciler.
	rName := gitops.RendererName(repoName.RepoOwner(), repoName.RepoName())

	if !h.Manager.HasReconciler(rName) {

		if ghrepo.FullName(repoName) != "jlewi/hydros-hydrated" {
			return errors.Errorf("Currently only the repository jlewi/hydros-hydrated. Code needs to be updated to support other repositories")
		}
		log.Info("Creating reconciler", "name", rName)

		// TODO(jeremy): This information should come from the configuration checked into the repository.
		// e.g.from the file .github/hydros.yaml

		org := "jlewi"
		repo := "hydros-hydrated"
		// Make sure workdir is unique for each reconciler.
		workDir := filepath.Join(h.workDir, rName)

		r, err := gitops.NewRenderer(org, repo, workDir, h.transports, client)
		if err != nil {
			return err
		}

		if err := h.Manager.AddReconciler(r); err != nil {
			if !gitops.IsDuplicateReconciler(err) {
				return err
			}
			log.Info("Ignoring AddReconciler DuplicateReconciler error; assuming its a race condition caused by simultaneous webhooks", "name", rName)
		}
	}

	// Enqueue a sync event.
	h.Manager.Enqueue(rName, gitops.RenderEvent{
		// https://docs.github.com/en/webhooks-and-events/webhooks/webhook-events-and-payloads#push
		// "After" is the commit after the push.
		Commit: event.GetAfter(),
		// Config could potentially be different for different commits
		// So we pass it along with the event
		BranchConfig: inPlaceConfig,
	})

	if err != nil {
		log.Error(err, "Failed to create check")
		return err
	}

	return nil
}
