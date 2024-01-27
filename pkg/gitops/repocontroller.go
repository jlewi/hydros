package gitops

import (
	"github.com/bmatcuk/doublestar/v4"
	"github.com/go-git/go-git/v5"
	"github.com/go-logr/logr"
	"github.com/go-logr/zapr"
	"github.com/jlewi/hydros/api/v1alpha1"
	"github.com/jlewi/hydros/pkg/files"
	"github.com/jlewi/hydros/pkg/github"
	"github.com/jlewi/hydros/pkg/images"
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
	workDir         string
	config          *v1alpha1.RepoConfig
	cloner          *github.ReposCloner
	imageController *images.Controller
	gitRepo         *git.Repository
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

	imageController, err := images.NewController()
	if err != nil {
		return nil, errors.Wrap(err, "Failed to create image controller")
	}

	cloner := &github.ReposCloner{
		URIs:    []string{config.Spec.Repo},
		Manager: manager,
		BaseDir: workDir,
	}

	return &RepoController{
		workDir:         workDir,
		config:          config,
		cloner:          cloner,
		imageController: imageController,
	}, nil
}

func (c *RepoController) Reconcile(ctx context.Context) error {
	log := util.LogFromContext(ctx)
	log = log.WithValues("repoConfig", c.config.Metadata.Name)
	ctx = logr.NewContext(ctx, log)

	if err := c.cloner.Run(ctx); err != nil {
		return err
	}

	repoDir, err := c.cloner.GetRepoDir(c.config.Spec.Repo)
	if err != nil {
		return err
	}

	c.gitRepo, err = git.PlainOpenWithOptions(repoDir, &git.PlainOpenOptions{})
	if err != nil {
		return errors.Wrapf(err, "Error opening git repo")
	}

	resources, err := c.findResources(ctx)
	if err != nil {
		return err
	}

	for _, r := range resources {
		if err := c.applyResource(ctx, r); err != nil {
			log.Error(err, "Error applying resource", "path", r.path, "name", r.node.GetName())
		}
	}
	return nil
}

func (c *RepoController) findResources(ctx context.Context) ([]*resource, error) {
	log := util.LogFromContext(ctx)
	repoDir, err := c.cloner.GetRepoDir(c.config.Spec.Repo)
	if err != nil {
		return nil, err
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
	return resources, nil
}

func (c *RepoController) applyResource(ctx context.Context, r *resource) error {
	log := util.LogFromContext(ctx)
	log = log.WithValues("path", r.path, "name", r.node.GetName())

	switch r.node.GetKind() {
	case v1alpha1.ImageGVK.Kind:
		return c.applyImage(ctx, r)
	case v1alpha1.ManifestSyncGVK.Kind:
		return errors.Errorf("ManifestSync is not yet implemented")
	default:
		return errors.Errorf("Unknown kind %v", r.node.GetKind())
	}
	return nil
}

func (c *RepoController) applyImage(ctx context.Context, r *resource) error {
	log := util.LogFromContext(ctx)
	log = log.WithValues("path", r.path, "name", r.node.GetName())

	image := &v1alpha1.Image{}

	if err := r.node.YNode().Decode(image); err != nil {
		return errors.Wrapf(err, "Error decoding image")
	}

	headRef, err := c.gitRepo.Head()
	if err != nil {
		return errors.Wrapf(err, "Error getting head ref")
	}

	image.Status.SourceCommit += headRef.Hash().String()

	basePath := filepath.Dir(r.path)
	return c.imageController.Reconcile(ctx, image, basePath)
}

type resource struct {
	node *yaml.RNode
	path string
}
