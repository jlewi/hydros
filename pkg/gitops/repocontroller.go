package gitops

import (
	"github.com/go-logr/zapr"
	"github.com/jlewi/hydros/api/v1alpha1"
	"github.com/jlewi/hydros/pkg/files"
	"github.com/jlewi/hydros/pkg/github"
	"github.com/pkg/errors"
	"go.uber.org/zap"
	"golang.org/x/net/context"
)

// RepoController is a controller for a repo.
// The controller will periodically checkout the repository and search it for resources.
// It will then sync those resources
type RepoController struct {
	// directory where repositories should be checked out
	workDir string
	config  *v1alpha1.RepoConfig
	cloner  *github.ReposCloner
}

func NewRepoController(config *v1alpha1.RepoConfig, workDir string) (*RepoController, error) {
	log := zapr.NewLogger(zap.L())

	if config == nil {
		return nil, errors.New("config must be non nil")
	}

	if errs, ok := config.IsValid(); !ok {
		return nil, errors.New(errs)
	}
	privateKey, err := files.Read(config.Spec.GitHubAppConfig.PrivateKey)
	if err != nil {
		return nil, errors.Wrapf(err, "Could not read private key %v", config.Spec.GitHubAppConfig.PrivateKey)
	}
	manager, err := github.NewTransportManager(config.Spec.GitHubAppConfig.AppID, privateKey, log)
	if err != nil {
		return nil, err
	}

	cloner := &github.ReposCloner{
		URIs:    []string{config.Spec.Repo},
		Manager: manager,
		BaseDir: workDir,
	}

	return &RepoController{
		workDir: workDir,
		config:  config,
		cloner:  cloner,
	}, nil
}

func (c *RepoController) Reconcile(ctx context.Context) error {
	if err := c.cloner.Run(ctx); err != nil {
		return err
	}
	return nil
}
