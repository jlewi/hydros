package gitops

import (
	"fmt"
	"github.com/go-git/go-git/v5"
	"github.com/go-logr/zapr"
	"github.com/jlewi/hydros/api/v1alpha1"
	"github.com/jlewi/hydros/pkg/github"
	"github.com/jlewi/hydros/pkg/github/ghrepo"
	hkustomize "github.com/jlewi/hydros/pkg/kustomize"
	"github.com/jlewi/hydros/pkg/util"
	"github.com/pkg/errors"
	"go.uber.org/zap"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Renderer handles in place modification of YAML files.
// It is intended to run a bunch of KRM functions in place and then check the modifications back into the repository
//
// TODO(jeremy): I don't think the semantics for specifying the KRM functions to apply is quite right.
// Right now we apply all KRM functions found at sourcePath. These functions get applied to all YAML below the
// location of the function path. This is ok as long as we don't have a mix of KRM functions that should be applied
// when hydrating into a different repository (e.g. via Syncer) but not when changes are to be checked into the
// source repository.
type Renderer struct {
	// ForkRepo is the repo into which the changes will be pushed and the PR created from
	ForkRepo *v1alpha1.GitHubRepo `yaml:"forkRepo,omitempty"`

	// DestRepo is the repo into which a PR will be created to merge hydrated
	// manifests from the ForkRepo
	DestRepo *v1alpha1.GitHubRepo `yaml:"destRepo,omitempty"`

	workDir string

	// repoHelper helps with creating PRs
	repoHelper *github.RepoHelper
	transports *github.TransportManager

	// sourcePath is the path relative to the root of the repo where the KRM functions should be applied.
	sourcePath string
}

func NewRenderer(forkRepo *v1alpha1.GitHubRepo, destRepo *v1alpha1.GitHubRepo, workDir string, sourcePath string, transports *github.TransportManager) (*Renderer, error) {
	r := &Renderer{
		ForkRepo:   forkRepo,
		DestRepo:   destRepo,
		workDir:    workDir,
		sourcePath: sourcePath,
		transports: transports,
	}

	return r, nil
}
func (r *Renderer) init() error {
	// Create a repo helper for the destRepo
	tr, err := r.transports.Get(r.DestRepo.Org, r.DestRepo.Repo)
	if err != nil {
		return errors.Wrapf(err, "Failed to get transport for repo %v/%v; Is the GitHub app installed in that repo?", r.DestRepo.Org, r.DestRepo.Repo)
	}

	args := &github.RepoHelperArgs{
		BaseRepo:   ghrepo.New(r.DestRepo.Org, r.DestRepo.Repo),
		GhTr:       tr,
		FullDir:    r.cloneDir(),
		Name:       "hydros",
		Email:      "hydros@yourdomain.com",
		Remote:     "origin",
		BranchName: r.ForkRepo.Branch,
		BaseBranch: r.DestRepo.Branch,
	}

	repoHelper, err := github.NewGithubRepoHelper(args)
	if err != nil {
		return err
	}

	r.repoHelper = repoHelper

	return nil
}

// RenderEvent is additional information about the render event
type RenderEvent struct {
	Commit string
	// CheckRunName is the name of current check run.
	// Blank if none exists
	CheckRunName string
}

func (r *Renderer) Name() string {
	// Name should be unique for a repository Reconciler type
	return fmt.Sprintf("renderer-%v-%v", r.DestRepo.Org, r.DestRepo.Repo)
}

func (r *Renderer) Run(anyEvent any) error {
	log := zapr.NewLogger(zap.L())

	event, ok := anyEvent.(RenderEvent)
	if !ok {
		log.Error(fmt.Errorf("Expected RenderEvent but got %v", anyEvent), "Invalid event type", "event", anyEvent)
		return fmt.Errorf("Event is not a RenderEvent")
	}

	if _, err := os.Stat(r.workDir); os.IsNotExist(err) {
		log.V(util.Debug).Info("Creating work directory.", "directory", r.workDir)

		err = os.MkdirAll(r.workDir, util.FilePermUserGroup)

		if err != nil {
			return errors.Wrapf(err, "Failed to create dir: %v", r.workDir)
		}
	}

	if err := r.init(); err != nil {
		return err
	}

	// Check if there is a PR already pending from the branch and if there is don't do a sync.

	// If the fork is in a different repo then the head reference is OWNER:BRANCH
	// If we are creating the PR from a different branch in the same repo as where we are creating
	// the PR then we just use BRANCH as the ref
	headBranchRef := r.ForkRepo.Branch

	if r.ForkRepo.Org != r.DestRepo.Org {
		headBranchRef = r.ForkRepo.Org + ":" + headBranchRef
	}
	existingPR, err := r.repoHelper.PullRequestForBranch()
	if err != nil {
		log.Error(err, "Failed to check if there is an existing PR", "headBranchRef", headBranchRef)
		return err
	}

	if existingPR != nil {
		log.Info("PR Already Exists; attempting to merge it.", "pr", existingPR.URL)
		state, err := r.repoHelper.MergeAndWait(existingPR.Number, 3*time.Minute)
		if err != nil {
			log.Error(err, "Failed to Merge existing PR unable to continue with sync", "number", existingPR.Number, "pr", existingPR.URL)
			return err
		}

		if state != github.ClosedState && state != github.MergedState {
			log.Info("PR hasn't been merged; unable to continue with the sync", "number", existingPR.Number, "pr", existingPR.URL, "state", state)
			return errors.Errorf("Existing PR %v is blocking sync", existingPR.URL)
		}
	}

	if event.Commit != "" {
		// TODO(jeremy): We need to update PrepareBranch to properly create the branch from the commit.
		err := errors.Errorf("Commit isn't properly supported yet. The branch is prepared off HEAD and not the commit")
		log.Error(err, "Commit isn't properly supported yet.", "commit", event.Commit)
	}

	// Checkout the repository.
	if err := r.repoHelper.PrepareBranch(true); err != nil {
		return err
	}

	syncNeeded, err := r.syncNeeded()
	if err != nil {
		return err
	}

	if !syncNeeded {
		log.Info("No sync needed")
		return nil
	}

	if err := r.applyKRMFns(); err != nil {
		return err
	}

	message := "Hydros AI generating configurations"
	if err := r.repoHelper.CommitAndPush(message, false); err != nil {
		return err
	}
	pr, err := r.repoHelper.CreatePr(message, []string{})
	if err != nil {
		return err
	}

	log.Info("PR created", "pr", pr.URL, "number", pr.Number)
	// EnableAutoMerge or merge the PR automatically. If you don't want the PR to be automerged you should
	// set up appropriate branch protections e.g. require approvers.
	// Wait up to 1 minute to try to merge the PR
	// If the PR can't be merged does it make sense to report an error?  in the case of long running tests
	// The syncer can return and the PR will be merged either 1) when syncer is rerun or 2) by auto merge if enabled
	// The desired behavior is potentially different in the takeover and non takeover setting.
	state, err := r.repoHelper.MergeAndWait(pr.Number, 1*time.Minute)
	if err != nil {
		log.Error(err, "Failed to merge pr", "number", pr.Number, "url", pr.URL)
		return err
	}
	if state != github.MergedState && state != github.ClosedState {
		return fmt.Errorf("Failed to merge pr; state: %v", state)
	}

	// TODO(jeremy): We should properly update the checkruns.

	return nil
}

func (r *Renderer) cloneDir() string {
	return filepath.Join(r.workDir, "source")
}

// applyKRMFns applies the KRM functions to the source repo.
func (r *Renderer) applyKRMFns() error {
	log := zapr.NewLogger(zap.L())

	d := hkustomize.Dispatcher{
		Log: log,
	}

	sourceDir := filepath.Join(r.cloneDir(), r.sourcePath)
	// get all functions based on the source directory
	funcs, err := d.GetAllFuncs([]string{sourceDir})
	if err != nil {
		log.Error(err, "hit unexpected error while trying to parse all functions")
		return err
	}

	// sort functions by longest path first
	err = d.SortFns(funcs)
	if err != nil {
		return err
	}

	// set respective annotation paths for each function
	err = d.SetFuncPaths(funcs, sourceDir, sourceDir, map[hkustomize.TargetPath]bool{})
	if err != nil {
		return err
	}

	// run function specified by function path, on hydrated source directory
	err = d.RunOnDir(sourceDir, []string{})
	if err != nil {
		return err
	}

	// apply all filtered function on their respective dirs
	return d.ApplyFilteredFuncs(funcs.Nodes)
}

// syncNeeded checks if a sync is needed. Since we are checking changes into the source repository we need to
// avoid recursively triggering a sync; i.e. if the last change was made by hydros AI don't run
func (r *Renderer) syncNeeded() (bool, error) {
	log := zapr.NewLogger(zap.L())
	// Open the repository
	gitRepo, err := git.PlainOpenWithOptions(r.cloneDir(), &git.PlainOpenOptions{})
	if err != nil {
		return false, errors.Wrapf(err, "Could not open respoistory at %v; ensure the directory contains a git repo", r.cloneDir())
	}

	// Get the current commit
	ref, err := gitRepo.Head()
	if err != nil {
		return false, err
	}

	commit, err := gitRepo.CommitObject(ref.Hash())
	if err != nil {
		return false, err
	}

	// N.B. This is a bit of a hack but couldn't figure out a better way. The email and name don't appear
	// to be what is set in the git config.  I think it depends on the values set in the GitHub app.
	if strings.HasPrefix(commit.Author.Name, "hydros") {
		log.Info("Last commit was made by hydros AI; skipping sync", "name", commit.Author.Name, "email", "else", commit.Author.Email, "commit", commit.Hash.String())
		return false, nil
	} else {
		log.Info("Last commit was not made by hydros AI; sync needed", "name", commit.Author.Name, "email", "else", commit.Author.Email, "commit", commit.Hash.String())
	}
	return true, nil
}

//func (r *Renderer) checkout() error {
//	log := zapr.NewLogger(zap.L())
//	//args := b.Args
//	//fullDir := b.fullDir
//	//url := "https://github.com/jlewi/roboweb.git"
//	//remote := "origin"
//	//
//	//secret, err := readSecret(args.Secret)
//	//if err != nil {
//	//	return errors.Wrapf(err, "Could not read file: %v", args.Secret)
//	//}
//
//	repo := ghrepo.New(r.DestRepo.Org, r.DestRepo.Repo)
//	url := ghrepo.GenerateRepoURL(repo, "https")
//
//	fullDir := r.cloneDir()
//	tr, err := r.transports.Get(r.SourceRepo.Org, r.SourceRepo.Repo)
//	if err != nil {
//		return err
//	}
//
//	appAuth := &github.AppAuth{
//		Tr: tr,
//	}
//	// Clone the repository if it hasn't already been cloned.
//	err = func() error {
//		if _, err := os.Stat(fullDir); err == nil {
//			log.Info("Directory exists; repository will not be cloned", "directory", fullDir)
//			return nil
//		}
//
//		opts := &git.CloneOptions{
//			URL:      url,
//			Auth:     appAuth,
//			Progress: os.Stdout,
//		}
//
//		_, err := git.PlainClone(fullDir, false, opts)
//		return err
//	}()
//
//	if err != nil {
//		return err
//	}
//
//	// Open the repository
//	gitRepo, err := git.PlainOpenWithOptions(fullDir, &git.PlainOpenOptions{})
//	if err != nil {
//		return errors.Wrapf(err, "Could not open respoistory at %v; ensure the directory contains a git repo", fullDir)
//	}
//
//	// Do a fetch to make sure the remote is up to date.
//	remote := "origin"
//	log.Info("Fetching remote", "remote", remote)
//	if err := gitRepo.Fetch(&git.FetchOptions{
//		RemoteName: remote,
//		Auth:       appAuth,
//	}); err != nil {
//		// Fetch returns an error if its already up to date and we want to ignore that.
//		if err.Error() != "already up-to-date" {
//			return err
//		}
//	}
//
//	// If commit is specified check it out
//	if r.commit != "" {
//		hash, err := gitRepo.ResolveRevision(plumbing.Revision(r.commit))
//
//		if err != nil {
//			return errors.Wrapf(err, "Could not resolve commit %v", r.commit)
//		}
//
//		log.Info("Checking out commit", "commit", hash.String())
//		w, err := gitRepo.Worktree()
//		if err != nil {
//			return err
//		}
//		err = w.Checkout(&git.CheckoutOptions{
//			Hash:  *hash,
//			Force: true,
//		})
//		if err != nil {
//			return errors.Wrapf(err, "Failed to checkout commit %s", r.commit)
//		}
//	}
//
//	// TODO(jeremy): We should be checking out the source branch.
//
//	// Get the current commit
//	ref, err := gitRepo.Head()
//	if err != nil {
//		return err
//	}
//
//	// The short tag will be used to tag the artifacts
//	//b.commitTag = ref.Hash().String()[0:7]
//
//	log.Info("Current commit", "commit", ref.Hash().String())
//	return nil
//}

// getRepos is a helper function that returns all the different repos involved in a map to make it easier
// to loop over them.
func (r *Renderer) getRepos() map[string]*v1alpha1.GitHubRepo {
	return map[string]*v1alpha1.GitHubRepo{
		//sourceKey: r.SourceRepo,
		destKey: r.DestRepo,
		forkKey: r.ForkRepo,
	}
}
