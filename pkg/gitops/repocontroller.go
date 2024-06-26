package gitops

import (
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/jlewi/hydros/pkg/controllers"

	"github.com/jlewi/hydros/pkg/config"

	"github.com/bmatcuk/doublestar/v4"
	"github.com/go-git/go-git/v5"
	"github.com/go-logr/logr"
	"github.com/jlewi/hydros/api/v1alpha1"
	"github.com/jlewi/hydros/pkg/github"
	"github.com/jlewi/hydros/pkg/images"
	"github.com/jlewi/hydros/pkg/util"
	"github.com/pkg/errors"
	"golang.org/x/net/context"
	meta "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime/schema"
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
	manager         *github.TransportManager
	registry        *controllers.Registry
	selectors       []labels.Selector
}

func NewRepoController(appConfig config.Config, registry *controllers.Registry, config *v1alpha1.RepoConfig) (*RepoController, error) {
	if config == nil {
		return nil, errors.New("config must be non nil")
	}

	if registry == nil {
		return nil, errors.New("registry must be non nil")
	}

	if errs, ok := config.IsValid(); !ok {
		return nil, errors.New(errs)
	}

	manager, err := github.NewTransportManagerFromConfig(appConfig)
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
		BaseDir: appConfig.GetWorkDir(),
	}

	selectors := make([]labels.Selector, 0, len(config.Spec.Selectors))
	for _, s := range config.Spec.Selectors {
		k8sS, err := s.ToK8s()
		if err != nil {
			return nil, errors.Wrapf(err, "Error converting selector; %v", util.PrettyString(s))
		}
		k8sSelector, err := meta.LabelSelectorAsSelector(k8sS)
		if err != nil {
			return nil, errors.Wrapf(err, "Failed to convert selector to k8s selector; %v", util.PrettyString(s))
		}
		selectors = append(selectors, k8sSelector)
	}

	return &RepoController{
		workDir:         appConfig.GetWorkDir(),
		config:          config,
		cloner:          cloner,
		imageController: imageController,
		manager:         manager,
		selectors:       selectors,
		registry:        registry,
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

	// Update the image controller with the current repo
	if err := c.imageController.SetLocalRepos([]images.GitRepoRef{
		{
			Repo: c.gitRepo,
		},
	}); err != nil {
		return err
	}

	// Apply all the resources in parallel
	// https://github.com/jlewi/hydros/issues/60 is tracking properly ordering dependencies.
	var wg sync.WaitGroup
	for _, r := range resources {
		wg.Add(1)
		go func(rNode *resource) {
			if err := c.applyResource(ctx, rNode); err != nil {
				log.Error(err, "Error applying resource", "path", rNode.path, "name", rNode.node.GetName())
			}
			wg.Done()
		}(r)
	}

	wg.Wait()
	return nil
}

func (c *RepoController) RunPeriodically(ctx context.Context, period time.Duration) error {
	log := util.LogFromContext(ctx)
	log = log.WithValues("repoConfig", c.config.Metadata.Name)
	ctx = logr.NewContext(ctx, log)

	for {
		err := c.Reconcile(ctx)
		if err != nil {
			log.Error(err, "Error reconciling")
		}

		if period == 0 {
			return err
		}
		log.Info("Sleeping", "period", period)
		time.Sleep(period)
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
			yamlFiles = append(yamlFiles, m)
		}
	}

	resources := make([]*resource, 0, len(yamlFiles))

	excludedKinds := map[string]bool{
		v1alpha1.RepoGVK.Kind: true,
	}

	for _, yamlFile := range yamlFiles {
		log.V(util.Debug).Info("Reading YAML file", "yamlFile", yamlFile)

		fullpath := filepath.Join(repoDir, yamlFile)
		nodes, err := util.ReadYaml(fullpath)
		if err != nil {
			log.Error(err, "Error reading YAML file", "yamlFile", fullpath)
			continue
		}

		seen := map[string]bool{}

		for _, node := range nodes {
			s := schema.FromAPIVersionAndKind(node.GetApiVersion(), node.GetKind())

			if s.Group != v1alpha1.Group {
				log.V(util.Debug).Info("Skipping resource with non hydros group", "group", s.Group)
				continue
			}

			if excludedKinds[s.Kind] {
				log.Info("Skipping resource with kind", "kind", s.Kind)
				continue
			}

			// Check it matches a selector
			isMatch := false
			labelsMap := labels.Set(node.GetLabels())
			for _, s := range c.selectors {
				if s.Matches(labelsMap) {
					isMatch = true
					break
				}
			}
			if !isMatch {
				log.V(util.Debug).Info("Skipping resource because it doesn't match any selectors", "kind", s.Kind, "name", node.GetName(), "path", fullpath, "labels", labelsMap)
				continue
			}

			// Ensure the resource has a name that is unique at least within the file.
			if seen[node.GetName()] {
				err := errors.New("Duplicate resource")
				log.Error(err, "Skipping duplicate resource. Each resource in the file should be uniquely named", "kind", s.Kind, "name", node.GetName(), "path", fullpath)
				continue
			}
			seen[node.GetName()] = true
			log.Info("Adding resource", "kind", s.Kind, "name", node.GetName(), "path", fullpath)

			resources = append(resources, &resource{
				node:  node,
				path:  fullpath,
				rPath: yamlFile,
			})
		}
	}
	return resources, nil
}

func (c *RepoController) applyResource(ctx context.Context, r *resource) error {
	log := util.LogFromContext(ctx)
	log = log.WithValues("path", r.path, "name", r.node.GetName())
	ctx = logr.NewContext(ctx, log)
	switch r.node.GetKind() {
	case v1alpha1.ImageGVK.Kind:
		// TODO(jeremy): Can we use the registry here?
		return c.applyImage(ctx, r)
	case v1alpha1.ManifestSyncGVK.Kind:
		// TODO(jeremy): We should move this into the registry?
		return c.applyManifest(ctx, r)
	default:
		return c.registry.ReconcileNode(ctx, r.node)
	}
	return nil
}

func (c *RepoController) applyImage(ctx context.Context, r *resource) error {
	image := &v1alpha1.Image{}

	if err := r.node.YNode().Decode(image); err != nil {
		return errors.Wrapf(err, "Error decoding image")
	}

	headRef, err := c.gitRepo.Head()
	if err != nil {
		return errors.Wrapf(err, "Error getting head ref")
	}

	image.Status.SourceCommit += headRef.Hash().String()

	return c.imageController.Reconcile(ctx, image)
}

func (c *RepoController) applyManifest(ctx context.Context, r *resource) error {
	log := util.LogFromContext(ctx)
	log = log.WithValues("path", r.path, "name", r.node.GetName())

	manifest := &v1alpha1.ManifestSync{}

	if err := r.node.YNode().Decode(manifest); err != nil {
		return errors.Wrapf(err, "Error decoding manifest")
	}

	// Rewrite the source repo if necessary
	if err := rewriteRepos(ctx, manifest, c.config.Spec.RepoMappings); err != nil {
		return err
	}

	pause := c.config.Spec.Pause
	if pause != "" {
		pauseDur, err := time.ParseDuration(pause)
		if err != nil {
			return errors.Wrapf(err, "Error parsing pause duration %v", pause)
		}

		if err := SetTakeOverAnnotations(manifest, pauseDur); err != nil {
			return errors.Wrapf(err, "Failed to set takeover annotations")
		}
		log.Info("Pausing automatic syncs; doing a takeover")
	}

	// Create a workDir for this syncer
	// Each ManifestSync should get its own workDir
	// This should be stable names so that they get reused on each sync
	dirname := strings.Replace(r.rPath, "/", "_", -1) + "_" + r.node.GetName()
	workDir := filepath.Join(c.workDir, dirname)

	syncer, err := NewSyncer(manifest, c.manager, SyncWithWorkDir(workDir), SyncWithLogger(log))
	if err != nil {
		log.Error(err, "Failed to create syncer")
		return err
	}

	return syncer.RunOnce(false)
}

type resource struct {
	node  *yaml.RNode
	path  string
	rPath string
}
