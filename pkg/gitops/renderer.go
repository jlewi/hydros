package gitops

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/bradleyfalzon/ghinstallation/v2"

	"github.com/go-git/go-git/v5"
	"github.com/go-logr/zapr"
	ghAPI "github.com/google/go-github/v52/github"
	"github.com/jlewi/hydros/api/v1alpha1"
	"github.com/jlewi/hydros/pkg/github"
	"github.com/jlewi/hydros/pkg/github/ghrepo"
	hkustomize "github.com/jlewi/hydros/pkg/kustomize"
	"github.com/jlewi/hydros/pkg/util"
	"github.com/pkg/errors"
	"go.uber.org/zap"
	"google.golang.org/protobuf/proto"
)

const (
	// 	RendererCheckName is the name "hydros-ai" name of the check run to use for the renderer
	RendererCheckName = "hydros-ai"
)

// Renderer is a reconciler that handles in place modification of YAML files.
// It is intended to run a bunch of KRM functions in place and then check the modifications back into the repository.
//
// There is currently one renderer per repository. A single renderer can handle multiple branches but not
// concurrently.
//
// TODO(jeremy): I don't think the semantics for specifying the KRM functions to apply is quite right.
// Right now we apply all KRM functions found at sourcePath. These functions get applied to all YAML below the
// location of the function path. This is ok as long as we don't have a mix of KRM functions that should be applied
// when hydrating into a different repository (e.g. via Syncer) but not when changes are to be checked into the
// source repository.
type Renderer struct {
	org        string
	repo       string
	workDir    string
	transports *github.TransportManager

	client *ghAPI.Client
}

func NewRenderer(org string, name string, workDir string, transports *github.TransportManager) (*Renderer, error) {
	ghTr, err := transports.Get(org, name)
	if err != nil {
		return nil, err
	}
	hClient := &http.Client{Transport: ghTr}

	client := ghAPI.NewClient(hClient)
	r := &Renderer{
		org:        org,
		repo:       name,
		workDir:    workDir,
		transports: transports,
		client:     client,
	}

	return r, nil
}
func (r *Renderer) init() error {

	return nil
}

func RendererName(org string, repo string) string {
	return fmt.Sprintf("renderer-%v-%v", org, repo)
}

// RenderEvent is additional information about the render event
type RenderEvent struct {
	Commit string
	// BranchConfig is the branch config for the branch being rendered
	// N.B. We don't actually verify that commit is on basebranch
	BranchConfig *v1alpha1.InPlaceConfig
}

func (r *Renderer) Name() string {
	// Name should be unique for a repository Reconciler type
	return RendererName(r.org, r.repo)
}

func (r *Renderer) Run(anyEvent any) error {
	log := zapr.NewLogger(zap.L()).WithValues("renderer", r.Name(), "org", r.org, "repo", r.repo)
	event, ok := anyEvent.(RenderEvent)
	if !ok {
		log.Error(fmt.Errorf("Expected RenderEvent but got %v", anyEvent), "Invalid event type", "event", anyEvent)
		return fmt.Errorf("Event is not a RenderEvent")
	}

	if event.Commit == "" {
		repos := r.client.Repositories
		branch, _, err := repos.GetBranch(context.Background(), r.org, r.repo, event.BranchConfig.BaseBranch, false)
		if err != nil {
			log.Error(err, "Failed to get branch; unable to determine the latest commit", "branch", event.BranchConfig.BaseBranch)
			return err
		}

		if branch.Commit.SHA == nil {
			err := fmt.Errorf("Branch %v doesn't have a commit SHA", event.BranchConfig.BaseBranch)
			log.Error(err, "Failed to get branch; unable to determine the latest commit", "branch", event.BranchConfig.BaseBranch)
			return err
		}
		event.Commit = *branch.Commit.SHA
		log.Info("Using latest commit from branch", "commit", event.Commit)
	}

	// CreateCheckRun requires a commit in order to attach a run to.
	// N.B. There is a bit of a race condition here. We risk reporting
	// the run as queued when it isn't actually because we crash before calling enqueue. However, its always
	// possible that the ghapp crashes after it was enqueued but before it succeeds. That should eventually be handled
	// by appropriate level based semantics. If we don't call CreateCheckRun we won't know the
	check, response, err := r.client.Checks.CreateCheckRun(context.Background(), r.org, r.repo, ghAPI.CreateCheckRunOptions{
		Name:       RendererCheckName,
		HeadSHA:    event.Commit,
		DetailsURL: proto.String("https://url.not.set.yet"),
		Status:     proto.String("queued"),
		Output: &ghAPI.CheckRunOutput{
			Title:   proto.String("Hydros queued"),
			Summary: proto.String("Hydros AI queued"),
			Text:    proto.String("Hydros AI enqueued."),
		},
	})

	if err != nil {
		return err
	}
	log.Info("Created check", "check", check, "response", response)

	if event.BranchConfig == nil {
		return errors.New("BranchConfig is nil")
	}

	if event.BranchConfig.BaseBranch == "" {
		return errors.New("BaseBranch is empty")
	}

	if event.BranchConfig.PRBranch == "" {
		return errors.New("PRBranch is empty")
	}

	clientTr := r.client.Client().Transport

	// TODO(jeremy): This is brittle.
	tr, ok := clientTr.(*ghinstallation.Transport)
	if !ok {
		return errors.New("Failed to get transport for repo; TR is not of type ghinstallation.Transport")
	}

	args := &github.RepoHelperArgs{
		BaseRepo:   ghrepo.New(r.org, r.repo),
		GhTr:       tr,
		FullDir:    r.cloneDir(),
		Name:       "hydros",
		Email:      "hydros@yourdomain.com",
		Remote:     "origin",
		BranchName: event.BranchConfig.PRBranch,
		BaseBranch: event.BranchConfig.BaseBranch,
	}

	repoHelper, err := github.NewGithubRepoHelper(args)
	if err != nil {
		return err
	}

	runErr := func() error {
		if _, err := os.Stat(r.workDir); os.IsNotExist(err) {
			log.V(util.Debug).Info("Creating work directory.", "directory", r.workDir)

			err = os.MkdirAll(r.workDir, util.FilePermUserGroup)

			if err != nil {
				return errors.Wrapf(err, "Failed to create dir: %v", r.workDir)
			}
		}

		// TODO(jeremy): We should probably do this once in a start function.
		if err := r.init(); err != nil {
			return err
		}

		// Check if there is a PR already pending from the branch and if there is don't do a sync.

		// If the fork is in a different repo then the head reference is OWNER:BRANCH
		// If we are creating the PR from a different branch in the same repo as where we are creating
		// the PR then we just use BRANCH as the ref
		headBranchRef := event.BranchConfig.PRBranch

		existingPR, err := repoHelper.PullRequestForBranch()
		if err != nil {
			log.Error(err, "Failed to check if there is an existing PR", "headBranchRef", headBranchRef)
			return err
		}

		if existingPR != nil {
			if !event.BranchConfig.AutoMerge {
				log.Info("PR Already Exists; and automerge isn't enabled. PR must be merged before sync can continue.", "pr", existingPR.URL)
				return nil
			}
			log.Info("PR Already Exists; attempting to merge it.", "pr", existingPR.URL)
			state, err := repoHelper.MergeAndWait(existingPR.Number, 3*time.Minute)
			if err != nil {
				log.Error(err, "Failed to Merge existing PR unable to continue with sync", "number", existingPR.Number, "pr", existingPR.URL)
				return err
			}

			if state != github.ClosedState && state != github.MergedState {
				log.Info("PR hasn't been merged; unable to continue with the sync", "number", existingPR.Number, "pr", existingPR.URL, "state", state)
				return errors.Errorf("Existing PR %v is blocking sync", existingPR.URL)
			}
		}

		// Checkout the repository.
		if err := repoHelper.PrepareBranch(true); err != nil {
			return err
		}

		if event.Commit != "" {
			ref, err := repoHelper.Head()
			if err != nil {
				return err
			}

			if ref.Hash().String() != event.Commit {
				log.Info("Commit doesn't match the head commit of the target branch; skipping sync because we only want to run on the latest changes", "commit", event.Commit, "head", ref.Hash().String())
				return nil
			}
		}

		syncNeeded, err := r.syncNeeded()
		if err != nil {
			return err
		}

		if !syncNeeded {
			log.Info("No sync needed")
			return nil
		}

		paths := event.BranchConfig.Paths
		if len(paths) == 0 {
			paths = []string{""}
		}
		for _, path := range paths {
			if err := r.applyKRMFns(path); err != nil {
				return err
			}
		}

		hasChanges, err := repoHelper.HasChanges()
		if err != nil {
			return err
		}

		if !hasChanges {
			// We should update he checkRun message to report this.
			log.Info("No changes to sync")
			return nil
		}

		message := "Hydros AI generating configurations"

		// Do a force pushed because we want to overwrite the branch with any changes.
		// If we don't do force push then pushes get blocked if there was a previous PR which was merged and
		// the branch wasn't deleted.
		if err := repoHelper.CommitAndPush(message, true); err != nil {
			return err
		}
		pr, err := repoHelper.CreatePr(message, []string{})
		if err != nil {
			return err
		}

		if !event.BranchConfig.AutoMerge {
			return nil
		}
		log.Info("PR created", "pr", pr.URL, "number", pr.Number)
		// Wait up to 1 minute to try to merge the PR
		// If the PR can't be merged does it make sense to report an error?  in the case of long running tests
		// The syncer can return and the PR will be merged either 1) when syncer is rerun or 2) by auto merge if enabled
		// The desired behavior is potentially different in the takeover and non takeover setting.
		state, err := repoHelper.MergeAndWait(pr.Number, 1*time.Minute)
		if err != nil {
			log.Error(err, "Failed to merge pr", "number", pr.Number, "url", pr.URL)
			return err
		}
		if state != github.MergedState && state != github.ClosedState {
			return fmt.Errorf("Failed to merge pr; state: %v", state)
		}
		return nil
	}()

	if event.Commit == "" {
		// N.B. This should happen after a regular sync. In that case we need to get the head commit and pass commit
		// along
		log.Error(errors.New("Commit is empty can't update checkrun"), "can't update checkrun")
	}

	// Update the check run
	conclusion := "success"
	// TODO(jeremy): We should provide a more detailed conclusion
	// e.g. we should include information about whether a PR was created.
	text := "Hydros AI generated configurations"
	if runErr != nil {
		conclusion = "failure"
		text = fmt.Sprintf("Failed to run Hydros AI; error %v", runErr)
	}

	uCheck, _, err := r.client.Checks.UpdateCheckRun(context.Background(), r.org, r.repo, *check.ID, ghAPI.UpdateCheckRunOptions{
		Name:       RendererCheckName,
		DetailsURL: proto.String("https://url.not.set.yet"),
		Status:     proto.String("completed"),
		Conclusion: proto.String(conclusion),
		Output: &ghAPI.CheckRunOutput{
			Title:   proto.String("Hydros complete"),
			Summary: proto.String("Hydros AI complete"),
			Text:    proto.String(text),
		},
	})
	if err != nil {
		log.Error(err, "Failed to update check run")
	}
	log.Info("Updated check", "check", uCheck)

	return runErr
}

func (r *Renderer) cloneDir() string {
	return filepath.Join(r.workDir, "source")
}

// applyKRMFns applies the KRM functions to the source repo.
func (r *Renderer) applyKRMFns(sourcePath string) error {
	log := zapr.NewLogger(zap.L())

	d := hkustomize.Dispatcher{
		Log: log,
	}

	sourceDir := filepath.Join(r.cloneDir(), sourcePath)
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
	// to be what is set in the git config.  I think it depends on the values set in the GitHub ghapp.
	if strings.HasPrefix(commit.Author.Name, "hydros") {
		log.Info("Last commit was made by hydros AI; skipping sync", "name", commit.Author.Name, "email", commit.Author.Email, "commit", commit.Hash.String())
		return false, nil
	} else {
		log.Info("Last commit was not made by hydros AI; sync needed", "name", commit.Author.Name, "email", commit.Author.Email, "commit", commit.Hash.String())
	}
	return true, nil
}
