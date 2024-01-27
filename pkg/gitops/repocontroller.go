package gitops

import (
	"github.com/bmatcuk/doublestar/v4"
	"github.com/go-logr/zapr"
	"github.com/jlewi/hydros/api/v1alpha1"
	"github.com/jlewi/hydros/pkg/files"
	"github.com/jlewi/hydros/pkg/github"
	"github.com/jlewi/hydros/pkg/util"
	"github.com/pkg/errors"
	"go.uber.org/zap"
	"golang.org/x/net/context"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"os"
	"path/filepath"
	"sigs.k8s.io/kustomize/kyaml/yaml"
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
	log := util.LogFromContext(ctx)
	log = log.WithValues("repoConfig", c.config.Metadata.Name)
	if err := c.cloner.Run(ctx); err != nil {
		return err
	}

	repoDir, err := c.cloner.GetRepoDir(c.config.Spec.Repo)
	if err != nil {
		return err
	}

	yamlFiles := make([]string, 0, 10)
	// Match globs matches all the globs
	for _, glob := range c.config.Spec.Globs {
		dirFs := os.DirFS(repoDir)

		matches, err := doublestar.Glob(dirFs, glob)
		if err != nil {
			log.Error(err, "Error matching glob", "glob", glob)
			continue
		}

		for _, m := range matches {
			yamlFiles = append(yamlFiles, filepath.Join(repoDir, m))
		}
	}

	resources := make([]*resource, 0, len(yamlFiles))

	allowedKinds := map[string]bool{
		v1alpha1.ImageGVK.Kind:        true,
		v1alpha1.ManifestSyncGVK.Kind: true,
	}
	for _, yamlFile := range yamlFiles {
		log.V(util.Debug).Info("Reading YAML file", "yamlFile", yamlFile)

		nodes, err := util.ReadYaml(yamlFile)
		if err != nil {
			log.Error(err, "Error reading YAML file", "yamlFile", yamlFile)
			continue
		}

		for _, node := range nodes {
			s := schema.FromAPIVersionAndKind(node.GetApiVersion(), node.GetKind())

			if s.Group != v1alpha1.Group {
				log.V(util.Debug).Info("Skipping resource with non hydros group", "group", s.Group)
				continue
			}

			if !allowedKinds[s.Kind] {
				log.V(util.Debug).Info("Skipping resource with kind", "kind", s.Kind)
				continue
			}
			log.Info("Adding resource", "kind", s.Kind, "name", node.GetName(), "path", yamlFile)
			resources = append(resources, &resource{
				node: node,
				path: yamlFile,
			})
		}
	}
	return nil
}

type resource struct {
	node *yaml.RNode
	path string
}
