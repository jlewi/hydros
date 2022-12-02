package gitops

import (
	"context"
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/config"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/go-git/go-git/v5/plumbing/transport/ssh"
	"github.com/jlewi/hydros/pkg/gitutil"

	"github.com/jlewi/hydros/pkg/ecrutil"
	"github.com/jlewi/hydros/pkg/skaffold"

	kustomize2 "github.com/jlewi/hydros/pkg/kustomize"
	meta "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"

	"github.com/google/uuid"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/awserr"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/ecr"
	"github.com/go-logr/logr"
	"github.com/go-logr/zapr"
	"github.com/jlewi/hydros/api/v1alpha1"
	"github.com/jlewi/hydros/pkg/github"
	"github.com/jlewi/hydros/pkg/github/ghrepo"
	"github.com/jlewi/hydros/pkg/util"
	"github.com/pkg/errors"
	"go.uber.org/zap"
	"gopkg.in/yaml.v3"

	kustomize "sigs.k8s.io/kustomize/api/types"
)

// Syncer keeps two repos in sync by creating PRs
// to publish hydrated manifests from one repo to the other.
type Syncer struct {
	log        logr.Logger
	manifest   *v1alpha1.ManifestSync
	workDir    string
	sess       *session.Session
	transports *github.TransportManager

	repoHelper *github.RepoHelper

	execHelper *util.ExecHelper

	// imageStrategies is a cache of how images should be resolved
	imageStrategies map[util.DockerImageRef]v1alpha1.Strategy

	selector *meta.LabelSelector
}

const (
	lastSyncFile      = ".lastsync.yaml"
	destKey           = "dest"
	sourceKey         = "source"
	forkKey           = "fork"
	kustomizationFile = "kustomization.yaml"
)

// NewSyncer creates a new syncer.
func NewSyncer(m *v1alpha1.ManifestSync, manager *github.TransportManager, opts ...SyncerOption) (*Syncer, error) {
	if m == nil {
		return nil, errors.Errorf("ManifestSync is required")
	}

	if err := m.IsValid(); err != nil {
		return nil, err
	}

	if manager == nil {
		return nil, fmt.Errorf("TransportManager is required")
	}

	s := &Syncer{
		log:        zapr.NewLogger(zap.L()),
		workDir:    "",
		manifest:   m,
		transports: manager,
	}

	for _, o := range opts {
		if err := o(s); err != nil {
			return nil, err
		}
	}
	s.log.Info("Creating NewSyncer", "manifest", m)
	if s.workDir == "" {
		newDir, err := ioutil.TempDir("", "manifestSync")
		if err != nil {
			return nil, errors.Wrapf(err, "Failed to create a temporary working directorry")
		}
		s.workDir = newDir
	}

	s.workDir = filepath.Join(s.workDir, m.Metadata.Name)
	s.log.Info("workdir is set.", "workDir", s.workDir)
	if s.sess == nil {
		// TODO(jeremy): Is there a better way to set the default region?
		// The session can be injected via an option.
		region := "us-west-2"
		s.log.Info("Creating an AWS session using defaults.", "region", region)
		sess, err := session.NewSession(&aws.Config{
			Region: aws.String(region),
		})
		if err != nil {
			s.log.Error(err, "Failed to create AWS session")
			return nil, err
		}
		s.sess = sess
	}
	s.log = s.log.WithValues("ManifestSync.Name", s.manifest.Metadata.Name)

	s.execHelper = &util.ExecHelper{
		Log: s.log,
	}

	if s.manifest.Spec.Selector != nil {
		selector, err := s.manifest.Spec.Selector.ToK8s()
		if err != nil {
			s.log.Error(err, "Failed to convert selector")
			return nil, err
		}
		s.selector = selector
	}

	// Ensure we can get transports for each repo; this basically verfies the app is authorized
	// for each repo.
	for _, repo := range getRepos(*s.manifest) {
		if _, err := s.transports.Get(repo.Org, repo.Repo); err != nil {
			return nil, errors.Wrapf(err, "Failed to get transport for repo %v/%v; Is the GitHub app installed in that repo?", repo.Org, repo.Repo)
		}
	}

	// Create a repo helper for the destRepo
	dRepo := s.manifest.Spec.DestRepo
	tr, err := s.transports.Get(dRepo.Org, dRepo.Repo)
	if err != nil {
		return nil, errors.Wrapf(err, "Failed to get transport for repo %v/%v; Is the GitHub app installed in that repo?", dRepo.Org, dRepo.Repo)
	}

	args := &github.RepoHelperArgs{
		BaseRepo:   ghrepo.New(dRepo.Org, dRepo.Repo),
		GhTr:       tr,
		FullDir:    filepath.Join(s.workDir, destKey),
		Name:       "hydros",
		Email:      "hydros@yourdomain.com",
		Remote:     "origin",
		BranchName: s.manifest.Spec.ForkRepo.Branch,
		BaseBranch: dRepo.Branch,
	}

	repoHelper, err := github.NewGithubRepoHelper(args)
	if err != nil {
		return nil, err
	}

	s.repoHelper = repoHelper
	s.log.Info("Successfuly created Syncer")
	return s, nil
}

func getRepos(m v1alpha1.ManifestSync) map[string]v1alpha1.GitHubRepo {
	return map[string]v1alpha1.GitHubRepo{
		sourceKey: m.Spec.SourceRepo,
		destKey:   m.Spec.DestRepo,
		forkKey:   m.Spec.ForkRepo,
	}
}

// SyncerOption is an option for instantiating the syncer.
type SyncerOption func(s *Syncer) error

// SyncWithLogger creates an option to use the supplied logger.
func SyncWithLogger(log logr.Logger) SyncerOption {
	return func(s *Syncer) error {
		s.log = log
		return nil
	}
}

// SyncWithWorkDir creates an option to use the supplied working directory.
func SyncWithWorkDir(wDir string) SyncerOption {
	return func(s *Syncer) error {
		s.workDir = wDir
		return nil
	}
}

// SyncWithAwsSession creates an option to use the supplied session.
func SyncWithAwsSession(sess *session.Session) SyncerOption {
	return func(s *Syncer) error {
		s.sess = sess
		return nil
	}
}

// getPinStrategy returns the strategy to resolve the image.
func (s *Syncer) getPinStrategy(source util.DockerImageRef) v1alpha1.Strategy {
	if s.imageStrategies == nil {
		s.imageStrategies = map[util.DockerImageRef]v1alpha1.Strategy{}
	}

	if _, ok := s.imageStrategies[source]; !ok {
		for _, imageToPin := range s.manifest.Spec.ImageTagsToPin {
			tagMatch := false
			for _, t := range imageToPin.Tags {
				if t == source.Tag {
					tagMatch = true
					break
				}
			}

			if !tagMatch {
				continue
			}

			if imageToPin.ImageRepoMatch == nil {
				s.imageStrategies[source] = imageToPin.Strategy
				break
			}

			repoMatches := false
			for _, r := range imageToPin.ImageRepoMatch.Repos {
				if r == source.Repo {
					repoMatches = true
					break
				}
			}

			if repoMatches && imageToPin.ImageRepoMatch.Type == v1alpha1.IncludeRepo {
				s.imageStrategies[source] = imageToPin.Strategy
			}

			if !repoMatches && imageToPin.ImageRepoMatch.Type == v1alpha1.ExcludeRepo {
				s.imageStrategies[source] = imageToPin.Strategy
			}
		}
	}

	strategy, ok := s.imageStrategies[source]

	if ok {
		return strategy
	}

	return v1alpha1.UnknownStrategy
}

// RunOnce runs the syncer once. If force is true a sync is run even if none is needed.
func (s *Syncer) RunOnce(force bool) error {
	// We need to reset the logger after RunOnce runs. Otherwise we will end up accumulating fields
	// like "run".
	oldLogger := s.log
	defer func() {
		s.log = oldLogger
	}()

	// Generate a unique run id for each run so that its easy to group log entries about a single run.
	s.log = s.log.WithValues("run", uuid.New().String()[0:5])
	s.execHelper.Log = s.log
	log := s.log
	if _, err := os.Stat(s.workDir); os.IsNotExist(err) {
		log.V(util.Debug).Info("Creating work directory.", "directory", s.workDir)

		err = os.MkdirAll(s.workDir, util.FilePermUserGroup)

		if err != nil {
			return errors.Wrapf(err, "Failed to create dir: %v", s.workDir)
		}
	}

	// Check if there is a PR already pending from the branch and if there is don't do a sync.

	// If the fork is in a different repo then the head reference is OWNER:BRANCH
	// If we are creating the PR from a different branch in the same repo as where we are creating
	// the PR then we just use BRANCH as the ref
	headBranchRef := s.manifest.Spec.ForkRepo.Branch

	if s.manifest.Spec.ForkRepo.Org != s.manifest.Spec.DestRepo.Org {
		headBranchRef = s.manifest.Spec.ForkRepo.Org + ":" + headBranchRef
	}
	existingPR, err := s.repoHelper.PullRequestForBranch()
	if err != nil {
		log.Error(err, "Failed to check if there is an existing PR", "headBranchRef", headBranchRef)
		return err
	}

	if existingPR != nil {
		log.Info("PR Already Exists; attempting to merge it.", "pr", existingPR.URL)
		if err := s.repoHelper.MergePR(existingPR.Number); err != nil {
			log.Error(err, "Failed to merge or enable automerge on pr", "number", existingPR.Number, "pr", existingPR.URL)
			return err
		}
		// TODO(jeremy): Ideally if the PR is merged immediately we should keep going with the sync. Since
		// there could be new changes to sync. On the other hand if it was enqueued then we should return because
		// we want to wait for it to merge.
		return nil
	}

	if err := s.cloneRepos(); err != nil {
		return err
	}

	sourceRepoRoot := filepath.Join(s.workDir, sourceKey)
	sourceRoot := filepath.Join(sourceRepoRoot, s.manifest.Spec.SourcePath)

	sourceCommit := s.getSourceCommit()

	if err := s.buildImages(sourceRoot, sourceCommit); err != nil {
		return err
	}

	lastStatus := s.lastStatusFromManifest(filepath.Join(s.workDir, destKey, s.manifest.Spec.DestPath, lastSyncFile))

	// Walk the source repository and find all kustomization files.
	kustomizeFiles, err := findKustomizationFiles(sourceRoot, sourceRepoRoot, s.manifest.Spec.ExcludeDirs, log)
	if err != nil {
		log.Error(err, "Failed to find kustomization files", "sourceRoot", sourceRoot)
		return err
	}

	allImages, filesToHydrate, err := s.findImagesToPin(kustomizeFiles)
	if err != nil {
		return err
	}

	imagesToPin := map[util.DockerImageRef]v1alpha1.Strategy{}

	// find the images to pin to.
	pinnedImages := map[util.DockerImageRef]util.DockerImageRef{}

	unResolved := []util.DockerImageRef{}

	for source := range allImages {
		// N.B. We make a copy of the tagged image because we will potentially modify its tag before
		// resolving it. However, we want to preserve the original key when storing in pinnedImages.
		taggedImage := source

		strategy := s.getPinStrategy(source)

		if strategy == v1alpha1.UnknownStrategy {
			log.V(util.Debug).Info("Skipping image; doesn't need to be pinned", "image", source)
			continue
		}

		imagesToPin[source] = strategy
		// If the image is built from source then we want to change the tag of the image
		// to be the source commit
		if strategy == v1alpha1.SourceCommitStrategy {
			log.V(util.Debug).Info("Image built from source", "image", source, "oldTag", source.Tag, "newTag", sourceCommit)
			taggedImage.Tag = sourceCommit
		}

		// All strategies require calling resolveImageToSha to resolve the image
		// to a particular sha.
		resolved, err := s.resolveImageToSha(taggedImage, strategy)
		if err != nil {
			// We want to accumulate a list of all unresolved images because its helpful to print a list of them
			// all in the logs.
			unResolved = append(unResolved, source)
			log.Error(err, "Failed to resolve image.", "image", source, "strategy", strategy)
			continue
		}
		pinnedImages[source] = resolved
		log.V(util.Debug).Info("Resolved image", "source", source, "resolved", resolved)
	}

	if len(unResolved) > 0 {
		return fmt.Errorf("Not all images could be resolved; unresolved images: %v", unResolved)
	}

	// Check if the pinned images have changed.
	changedImages := s.didImagesChange(lastStatus.PinnedImages, pinnedImages)

	if sourceCommit == lastStatus.SourceCommit && len(changedImages) == 0 {
		if !force {
			log.Info("Sync not needed; manifests and images up to date", "sourceCommit", sourceCommit)
			return nil
		}
		log.Info("Sync not needed but force is true", "sourceCommit", sourceCommit)
	}

	log.Info("Hydrated manifests need sync", "sourceCommit", sourceCommit, "lastSync", lastStatus.SourceCommit, "changedImages", changedImages)

	// Set the images in the kustomization files.
	for source, resolved := range pinnedImages {
		// Loop over all the files containing this image
		for _, t := range allImages[source] {
			k, err := readKustomization(t.Kustomization)
			if err != nil {
				return err
			}

			// N.B wrap in a function to ensure defer is closed.
			err = func() error {
				err := util.SetKustomizeImage(k, t.ImageName, resolved)
				if err != nil {
					return err
				}

				w, err := os.Create(t.Kustomization)
				if err != nil {
					return errors.Wrapf(err, "Failed to Create file: %v", t.Kustomization)
				}
				defer func() { util.IgnoreError(w.Close()) }()

				e := yaml.NewEncoder(w)

				if err := e.Encode(k); err != nil {
					log.Error(err, "Failed to marshal kustomization", "kustomization", k, "file", t.Kustomization)
					return errors.Wrapf(err, "Failed Kustomization to file %v", t.Kustomization)
				}

				return nil
			}()

			if err != nil {
				return err
			}
		}
	}

	// Create a local branch from the fork repo
	forkDir := filepath.Join(s.workDir, forkKey)
	// N.B We check out the branch of the destination repo.
	cmd := exec.Command("git", "checkout", "-B", s.manifest.Spec.ForkRepo.Branch, "origin/"+s.manifest.Spec.DestRepo.Branch)
	cmd.Dir = forkDir

	if err := s.execHelper.Run(cmd); err != nil {
		log.Error(err, "Failed to create a local branch for the forked repo")
		return err
	}

	// Delete the target directory
	baseHydratePath := filepath.Join(forkDir, s.manifest.Spec.DestPath)
	if _, err := os.Stat(baseHydratePath); err == nil || os.IsExist(err) {
		log.V(util.Debug).Info("Deleting dest path", "destPath", baseHydratePath)
		if err := os.RemoveAll(baseHydratePath); err != nil {
			return err
		}
	}

	log.V(util.Debug).Info("Creating directory", "dir", baseHydratePath)
	if err := os.MkdirAll(baseHydratePath, util.FilePermUserGroup); err != nil {
		return errors.Wrapf(err, "Failed to create directory: %v", baseHydratePath)
	}

	// Hydrate overlay dirs
	log.Info("Hydrating kustomizations", "kustomizations", filesToHydrate)
	for _, k := range filesToHydrate {
		targetPath, err := kustomize2.GenerateTargetPath(sourceRoot, k)
		if err != nil {
			log.Error(err, "Failed to generate target path", "kustomization", k)
			return err
		}

		hydratePath := filepath.Join(baseHydratePath, targetPath.Dir)

		if _, err := os.Stat(hydratePath); os.IsExist(err) {
			newErr := fmt.Errorf("Hydrated path already exists; %v; kustomization:%v", hydratePath, k)
			log.Error(newErr, "Hydrated directory already exists; This indicates two kustomizations are trying to hydrate the same package", "hydratePath", hydratePath, "kustomization", k)
			return newErr
		}

		log.V(util.Debug).Info("Create kustomize output dir", "dir", hydratePath)
		if err := os.MkdirAll(hydratePath, util.FilePermUserGroup); err != nil {
			return errors.Wrapf(err, "Failed to create directory: %v", hydratePath)
		}

		overlayDir := path.Dir(k)
		cmd := exec.Command("kustomize", "build", "--enable-helm", "--load-restrictor=LoadRestrictionsNone", "-o", hydratePath, overlayDir)

		if err := s.execHelper.Run(cmd); err != nil {
			log.Error(err, "Failed to hydrate kustomization", "overlayDir", overlayDir, "output", hydratePath)
			return err
		}
		log.Info("Successfully hydrated package", "kustomization", k)
	}

	// Write the updated manifest to the dest
	s.manifest.Status.SourceCommit = sourceCommit
	s.manifest.Status.PinnedImages = []v1alpha1.PinnedImage{}
	sourceRepo := s.manifest.Spec.SourceRepo
	sourceURL := fmt.Sprintf("https://github.com/%v/%v/tree/%v", sourceRepo.Org, sourceRepo.Repo, sourceCommit)
	s.manifest.Status.SourceURL = sourceURL
	for old, new := range pinnedImages {
		s.manifest.Status.PinnedImages = append(s.manifest.Status.PinnedImages, v1alpha1.PinnedImage{
			Image:    old.ToURL(),
			NewImage: new.ToURL(),
		})
	}

	err = s.applyKustomizeFns(baseHydratePath, sourceRoot, filesToHydrate)

	if err != nil {
		log.Error(err, "applyKustomizeFns failed")
		return err
	}

	newSyncFile := filepath.Join(baseHydratePath, lastSyncFile)
	w, err := os.Create(newSyncFile)
	if err != nil {
		log.Error(err, "Failed to write manifest", "path", newSyncFile)
		return err
	}
	e := yaml.NewEncoder(w)
	e.SetIndent(2)
	if err := e.Encode(s.manifest); err != nil {
		log.Error(err, "Failed to update manifest", "path", newSyncFile)
		return err
	}

	// Commit and push the changes.
	commands := [][]string{
		{"git", "add", "."},
		{"git", "commit", "-m", fmt.Sprintf("Update hydrated manifests to %v", sourceCommit)},
		{"git", "push", "-f", "-u", "origin", "HEAD"},
	}
	for _, c := range commands {
		cmd := exec.Command(c[0],
			c[1:]...,
		)
		cmd.Dir = forkDir
		if err := s.execHelper.Run(cmd); err != nil {
			return err
		}

	}

	// Create the PR.
	prMessage := buildPrMessage(s.manifest, changedImages)

	pr, err := s.repoHelper.CreatePr(prMessage, s.manifest.Spec.PrLabels)
	if err != nil {
		log.Error(err, "Failed to create pr")
		return err
	}

	// EnableAutoMerge or merge the PR automatically. If you don't want the PR to be automerged you should
	// set up appropriate branch protections e.g. require approvers.
	if err := s.repoHelper.MergePR(pr.Number); err != nil {
		log.Error(err, "Failed to merge or enable automerge on pr", "number", pr.Number, "url", pr.URL)
		return err
	}

	log.Info("Sync succeeded")
	return nil
}

// PushLocal commits any changes in wDir and then pushes those changes to the branch of the sourceRepo
// A sync can then be applied.
// keyFile is the private PEM key file to use. If not specified it will try to load one from the home directory
func (s *Syncer) PushLocal(wDir string, keyFile string) error {
	log := s.log

	if wDir == "" {
		var err error
		wDir, err = os.Getwd()
		if err != nil {
			return errors.Wrapf(err, "Failed to get current directory")
		}
	}

	root, err := gitutil.LocateRoot(wDir)
	if err != nil {
		return errors.Wrapf(err, "Failed to locate git repo for %v", wDir)
	}

	if keyFile == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return errors.Wrapf(err, "Could not get home directory")
		}
		keyFile = filepath.Join(home, ".ssh", "id_ed25519")
		log.Info("No keyfile specified using default", "keyfile", keyFile)
	}
	// GitHub uses git for the username.
	appAuth, err := ssh.NewPublicKeysFromFile("git", keyFile, "")
	if err != nil {
		return errors.Wrapf(err, "Failed to load ssh key")
	}
	log.Info("Located root of git repository", "root", root, "wDir", wDir)
	// Open the repository
	r, err := git.PlainOpenWithOptions(root, &git.PlainOpenOptions{})
	if err != nil {
		return errors.Wrapf(err, "Could not open respoistory at %v; ensure the directory contains a git repo", root)
	}

	w, err := r.Worktree()
	if err != nil {
		return err
	}

	if err := gitutil.AddGitignoreToWorktree(w, root); err != nil {
		return errors.Wrapf(err, "Failed to add gitignore patterns")
	}

	status, err := w.Status()
	if err != nil {
		return err
	}

	// We need to identify the remote name for the source branch
	// config reads .git/config
	// We can use this to determine how the repository is setup to figure out what we need to do
	cfg, err := r.Config()
	if err != nil {
		return err
	}

	if !status.IsClean() {
		log.Info("committing all files")
		if err := w.AddWithOptions(&git.AddOptions{All: true}); err != nil {
			return err
		}

		user, err := gitutil.LoadUser(r)
		if err != nil {
			return err
		}
		message := "hydros automatically committing all files before running a sync."
		commit, err := w.Commit(message, &git.CommitOptions{
			Author: &object.Signature{
				// Use the name and email as specified in the cfg file.
				Name:  user.Name,
				Email: user.Email,
				When:  time.Now(),
			},
		})

		if err != nil {
			return err
		}

		// Prints the current HEAD to verify that all worked well.
		obj, err := r.CommitObject(commit)
		if err != nil {
			return err
		}
		log.Info("Commit succeeded", "commit", obj.String())
	}

	org := s.manifest.Spec.SourceRepo.Org
	repo := s.manifest.Spec.SourceRepo.Repo
	sourceRepo := ghrepo.New(org, repo)
	remoteName := func() string {
		for _, r := range cfg.Remotes {
			for _, u := range r.URLs {
				remote, err := ghrepo.FromFullName(u)
				if err != nil {
					log.Error(err, "Could not parse URL for remote repository", "name", r.Name, "url", u)
				}
				if ghrepo.IsSame(sourceRepo, remote) {
					return r.Name
				}
			}
		}
		return ""
	}()

	if remoteName == "" {
		return errors.Errorf("Could not find remote repo for repository %v/%v", org, repo)
	}

	head, err := r.Head()
	if err != nil {
		return err
	}
	// The refspec to push
	dst := fmt.Sprintf("refs/heads/%v", s.manifest.Spec.SourceRepo.Branch)
	refSpec := head.Name().String() + ":" + dst

	// Push changes to the remote branch.
	if err := r.Push(&git.PushOptions{
		RemoteName: remoteName,
		RefSpecs: []config.RefSpec{
			config.RefSpec(refSpec),
		},
		Progress: os.Stdout,
		Force:    true,
		Auth:     appAuth,
	}); err != nil && err.Error() != "already up-to-date" {
		return err
	}

	log.Info("Push succeeded")
	return nil
}

// repoKeyToDir takes the key identifying a repo (e.g. "source", "dest", "fork") and returns the path where it is
// checked out.
func (s *Syncer) repoKeyToDir(name string) string {
	return filepath.Join(s.workDir, name)
}

// cloneRepos clones all the repos
func (s *Syncer) cloneRepos() error {
	log := s.log
	// Clone the repos if its not already cloned.
	for name, repoSpec := range getRepos(*s.manifest) {
		fullDir := s.repoKeyToDir(name)

		ghTr, err := s.transports.Get(repoSpec.Org, repoSpec.Repo)
		if err != nil {
			return fmt.Errorf("Missing transport for %v/%v", repoSpec.Org, repoSpec.Repo)
		}

		// Generate an access token
		token, err := ghTr.Token(context.Background())
		if err != nil {
			return err
		}

		url := fmt.Sprintf("https://x-access-token:%v@github.com/%v/%v.git", token, repoSpec.Org, repoSpec.Repo)

		log := log.WithValues("org", repoSpec.Org, "repo", repoSpec.Repo, "dir", fullDir)

		err = func() error {
			if _, err := os.Stat(fullDir); err == nil {
				log.V(util.Debug).Info("Directory already exists")
				return nil
			}

			commands := [][]string{
				{"git", "clone", url, fullDir},
			}

			err := s.execHelper.RunCommands(commands, nil)
			if err != nil {
				log.Error(err, "git clone failed")
				return err
			}

			return nil
		}()

		if err != nil {
			return err
		}

		// Update the remote URL to refresh the token
		// Then fetch it. Also make sure user name is set.
		commands := [][]string{
			{"git", "config", "user.name", "hydros"},
			{"git", "config", "user.email", "hydros@notvalid.primer.ai"},
			{"git", "remote", "set-url", "origin", url},
			{"git", "fetch", "origin"},
			// if we don't force code.abbrev to be 7 digits then we might get a variable
			// number. We need the short hash to be consistent with the docker image
			// tag otherwise we will fail to resolve images.
			{"git", "config", "--local", "--add", "core.abbrev", "7"},
		}

		if err := s.execHelper.RunCommands(commands, func(cmd *exec.Cmd) {
			cmd.Dir = fullDir
		}); err != nil {
			return err
		}

		// Drop any local changes that might be lingering from a previous run.
		if err := s.resetBranch(fullDir); err != nil {
			return err
		}

		cmd := exec.Command("git", "checkout", "origin/"+repoSpec.Branch)
		cmd.Dir = fullDir
		// N.B. use RunQuietly because we don't want to spam the logs when everything is working correctly.
		if data, err := s.execHelper.RunQuietly(cmd); err != nil {
			if name == forkKey {
				// The checkout will fail if the origin branch doesn't already exist. This is fine.
				// It means the manifests are out of sync and we will create the branch below.
				log.V(util.Debug).Info("Ignoring failed checkout of forked branch; assuming it doesn't exist")
			} else if name == destKey {
				log.Error(err, "git checkout failed; the branch to merge the PR into doesn't exist. This usually means this is a new branch and you need to create it manually", "command", cmd.String(), "output", data)
				return err
			} else {
				log.Error(err, "git checkout failed", "command", cmd.String(), "output", data)
				return err
			}
		}
	}
	return nil
}

// didImagesChange checks whether the images are no longer pinned to the correct value.
func (s *Syncer) didImagesChange(lastSync []v1alpha1.PinnedImage, current map[util.DockerImageRef]util.DockerImageRef) []util.DockerImageRef {
	log := s.log
	changed := []util.DockerImageRef{}

	lastImages := map[util.DockerImageRef]util.DockerImageRef{}

	for _, image := range lastSync {
		key, err := util.ParseImageURL(image.Image)
		if err != nil {
			log.Error(err, "Could not parse image", "image", image.Image)
			continue
		}
		lastImage, err := util.ParseImageURL(image.NewImage)
		if err != nil {
			log.Error(err, "Could not parse image", "image", image.NewImage)
			continue
		}

		lastImages[*key] = *lastImage
	}

	for image, newPinned := range current {
		lastPinned, ok := lastImages[image]
		if !ok {
			log.V(util.Debug).Info("Found new image that needs pinning", "image", image)
			changed = append(changed, newPinned)
			continue
		}

		if lastPinned.ToURL() != newPinned.ToURL() {
			log.V(util.Debug).Info("Image changed", "mutable", image, "lastPinned", lastPinned, "newPinned", newPinned)
			changed = append(changed, newPinned)
		}
	}

	return changed
}

// RunPeriodically runs periodically with the specified period.
func (s *Syncer) RunPeriodically(period time.Duration) {
	for {
		err := s.RunOnce(false)
		if err != nil {
			s.log.Error(err, "Sync failed")
		}
		s.log.V(util.Debug).Info("sleep", "duration", period)
		time.Sleep(period)
	}
}

// lastStatusFromManifest reads the commit of the source from a YAML file containing a ManifestSync object
func (s *Syncer) lastStatusFromManifest(syncFile string) *v1alpha1.ManifestSyncStatus {
	lastStatus := &v1alpha1.ManifestSyncStatus{
		PinnedImages: []v1alpha1.PinnedImage{},
	}

	log := s.log
	if _, err := os.Stat(syncFile); os.IsNotExist(err) {
		log.Info("SyncFile doesn't exist", "syncFile", syncFile)
		return lastStatus
	}

	r, err := os.Open(syncFile)
	if err != nil {
		// Just force a sync
		log.Error(err, "Could not read sync file", "syncFile")
		return lastStatus
	}

	d := yaml.NewDecoder(r)

	lastSync := &v1alpha1.ManifestSync{}
	if err := d.Decode(lastSync); err != nil {
		log.Error(err, "Could not decode ManifestSync")
		return lastStatus
	}
	return &lastSync.Status
}

func (s *Syncer) getSourceCommit() string {
	log := s.log
	// Get the latest commit on the source repo
	cmd := exec.Command("git", "rev-parse", "--short", "origin/"+s.manifest.Spec.SourceRepo.Branch)
	cmd.Dir = filepath.Join(s.workDir, sourceKey)

	output, err := cmd.CombinedOutput()
	if err != nil {
		log.Error(err, "Get latest commit failed", "command", cmd.String())
		return ""
	}

	sourceCommit := strings.TrimSpace(string(output))
	return sourceCommit
}

// imageAndFile is a tuple keeping track of a kustomization file
// and the name of an image to be replace
type imageAndFile struct {
	ImageName     string
	Kustomization string
}

func (s *Syncer) resetBranch(repoDir string) error {
	// Stash any changes
	cmd := exec.Command("git", "stash", "save", "--keep-index", "--include-untracked")
	cmd.Dir = repoDir

	if err := s.execHelper.Run(cmd); err != nil {
		return err
	}

	// Drop the changes
	cmd = exec.Command("git", "stash", "drop")
	cmd.Dir = repoDir

	// Ignore git stash error it will return an error if there is nothing in the stash to drop
	_ = cmd.Run()

	// Hard reset is needed in case we have any tracked changes; this won't be dropped by stash
	cmd = exec.Command("git", "reset", "--hard")
	cmd.Dir = repoDir

	if err := s.execHelper.Run(cmd); err != nil {
		return err
	}
	return nil
}

// resolveImageToSha resolves the provided DockerImageRef to an image and gets the sha.
// If the image isn't found err will be an AwsError with code ecr.ErrCodeImageNotFoundException.
// See http://docs.aws.amazon.com/AmazonS3/latest/API/ErrorResponses.html for example of how to process it.
func (s *Syncer) resolveImageToSha(r util.DockerImageRef, strategy v1alpha1.Strategy) (util.DockerImageRef, error) {
	svc := ecr.New(s.sess)

	resolved := r

	input := &ecr.DescribeImagesInput{
		ImageIds: []*ecr.ImageIdentifier{
			{
				ImageTag: aws.String(r.Tag),
			},
		},
		RegistryId:     aws.String(r.GetAwsRegistryID()),
		RepositoryName: aws.String(r.Repo),
	}

	result, err := svc.DescribeImages(input)
	if err != nil {
		return resolved, err
	}

	if len(result.ImageDetails) != 1 {
		s.log.Info("DescribeImages didn't return exactly one image", "image", r, "result", result)
		return resolved, fmt.Errorf("BatchGetImage didn't return exactly one image")
	}

	image := result.ImageDetails[0]

	resolved.Sha = *image.ImageDigest

	if strategy == v1alpha1.MutableTagStrategy {
		// we want try to replace it with a tag that is less
		// likely to be mutable. This is mostly a hack for the case where the image is not built from the source repo.
		// So we resolve it based on the mutable tag but we'd like to map its latest tag to the git commit tag.
		excludedTags := map[string]bool{"latest": true, "live": true, "prod": true, "dev": true, "staging": true}
		// Find the first tag if any not in excluded tags
		// This is just so we can display images in the form of
		// 1234.dkr.ecr.us-west-2.amazonaws.com/some-repo/some-image:d891862@sha256:1eaea2d03772c90f262bc17879e7a98129cec0d1db89611ed1ec6b206f5f1609
		// The tag is helpful for humans but the sha takes precedence
		for _, t := range image.ImageTags {
			// Skip the original mutable tag.
			if r.Tag == *t {
				continue
			}
			if _, ok := excludedTags[*t]; !ok {
				resolved.Tag = *t
				break
			}
		}
	}

	return resolved, nil
}

// findImagesToPin searches the kustomzie files to find all images that might need to be pinned.
// Result is a mapping from docker images. Also returns a list of kustomization files that match the annotations
// and should be hydrated.
func (s *Syncer) findImagesToPin(kustomizeFiles []string) (map[util.DockerImageRef][]imageAndFile, []string, error) {
	log := s.log
	// Define some sets to look up the images to replace.
	registrySet := map[string]bool{}

	for _, i := range s.manifest.Spec.ImageRegistries {
		registrySet[i] = true
	}

	results := map[util.DockerImageRef][]imageAndFile{}

	filesToHydrate := []string{}

	for _, f := range kustomizeFiles {
		log.V(util.Debug).Info("Found kustomization", "kustomization", f)
		k, err := readKustomization(f)
		if err != nil {
			log.Error(err, "Failed to read kustomization", "kustomization", f)
			return results, filesToHydrate, err
		}

		overlayMatches := false
		if s.selector != nil {
			var err error
			overlayMatches, err = matches(k, s.selector)

			if err != nil {
				log.Error(err, "Failed to apply selector to kustomization", "kustomization", f)
			}
		}

		// Fallback to trying the annotations. We want to allow a package to use a selector and matchAnnotations
		// as we transition because not all kustomize packages may be updated to specify metadata.labels at once.
		if !overlayMatches && s.manifest.Spec.MatchAnnotations != nil {
			// N.B. this codepath is deprecated new code paths should use selector
			overlayMatches = isMatch(k, s.manifest.Spec.MatchAnnotations)
		}

		if !overlayMatches {
			log.V(util.Debug).Info("Kustomization didn't match selector; it will not be hydrated", "kustomization", f)
			continue
		}

		// Important we should only consider pinning images if we are hydrating the kustomize file.
		filesToHydrate = append(filesToHydrate, f)
		// Loop over all images and see if there is an image eligible for replacement
		for _, i := range k.Images {
			// N.B. the code is assuming that the images as listed in YAML resources (e.g. a Deployment)
			// don't refer to a registry. In which case newName gets set and should contain the tag.
			if i.NewName == "" {
				continue
			}
			url := i.NewName
			if i.NewTag != "" {
				url = url + ":" + i.NewTag
			}
			r, err := util.ParseImageURL(url)
			if err != nil {
				log.Error(err, "Failed to parse image url", "image", url)
			}

			if _, ok := registrySet[r.Registry]; !ok {
				continue
			}

			if _, ok := results[*r]; !ok {
				results[*r] = []imageAndFile{}
			}
			results[*r] = append(results[*r], imageAndFile{
				ImageName:     i.Name,
				Kustomization: f,
			})
		}
	}
	return results, filesToHydrate, nil
}

func (s *Syncer) applyKustomizeFns(hydratedPath string, sourceRoot string, filesToHydrate []string) error {
	log := s.log
	functionPaths := []string{}
	for _, f := range s.manifest.Spec.Functions {
		for _, p := range f.Paths {
			full := path.Join(s.repoKeyToDir(f.RepoKey), p)
			functionPaths = append(functionPaths, full)
		}
	}

	d := kustomize2.Dispatcher{
		Log: log,
	}

	// get all functions based on sourcedir
	funcs, err := d.GetAllFuncs([]string{sourceRoot})
	if err != nil {
		s.log.Error(err, "hit unexpected error while trying to parse all functions")
		return err
	}

	// sort functions by longest path first
	err = d.SortFns(funcs)
	if err != nil {
		return err
	}

	// get leaf paths to help generate the right target path by checking paths if they
	// are an overlay or not
	leafPaths, err := d.RemoveOverlayOnHydratedFiles(filesToHydrate, sourceRoot)
	if err != nil {
		return err
	}

	// set respective annotation paths for each function
	err = d.SetFuncPaths(funcs, hydratedPath, sourceRoot, leafPaths)
	if err != nil {
		return err
	}

	// run function specified by function path, on hydrated source directory
	err = d.RunOnDir(hydratedPath, functionPaths)
	if err != nil {
		return err
	}

	// apply all filtered function on their respective dirs
	return d.ApplyFilteredFuncs(funcs.Nodes)
}

func (s *Syncer) buildImages(sourcePath string, sourceCommit string) error {
	// Give each run of buildImages a unique id so its easy to group all the messages about image building
	// for a particular run.
	log := s.log.WithValues("skaffoldId", uuid.New().String()[0:5])

	if s.manifest.Spec.ImageBuilder == nil || !s.manifest.Spec.ImageBuilder.Enabled {
		log.Info("Image builder not enabled")
		return nil
	}

	// Find all the skaffold files
	configs, err := skaffold.LoadSkaffoldConfigs(log, sourcePath, nil, s.manifest.Spec.ExcludeDirs)
	if err != nil {
		log.Error(err, "Failed to load skaffold configs", "sourcePath", sourcePath)
		return err
	}

	skaffoldErrs := &util.ListOfErrors{}
	errsMu := sync.Mutex{}

	// Explicitly tag the image with source so even if the tagging strategy is different we still have the
	// tag expected by hydros.
	tags := []string{sourceCommit, "latest"}

	var wg sync.WaitGroup
	// Determine which images don't exist
	for _, c := range configs {
		log := log.WithValues("skaffoldFile", c.Path)
		err := skaffold.ChangeRegistry(c.Config, s.manifest.Spec.ImageBuilder.Registry)
		if err != nil {
			return errors.Wrapf(err, "Failed to change registry in file: %v", c.Path)
		}

		missingImages := false
		for _, a := range c.Config.Build.Artifacts {
			image, err := util.ParseImageURL(a.ImageName)
			if err != nil {
				log.Error(err, "Failed to parse image.", "image", a.ImageName)
				return errors.Wrapf(err, "Failed to parse image: %v", a.ImageName)
			}

			// Ensure the repo exists
			if err := ecrutil.EnsureRepoExists(s.sess, image.GetAwsRegistryID(), image.Repo); err != nil {
				return errors.Wrapf(err, "Failed to ensure the repo exists; registry: %v; repo: %v", image.Registry, image.Repo)
			}

			// Ensure cache repo exists
			cacheRepo := image.Repo + "/cache"
			if err := ecrutil.EnsureRepoExists(s.sess, image.GetAwsRegistryID(), cacheRepo); err != nil {
				return errors.Wrapf(err, "Failed to ensure the repo exists; registry: %v; repo: %v", image.Registry, cacheRepo)
			}
			// Check if the image exists.
			image.Tag = sourceCommit

			// TODO(jeremy): Should we return the resolved image so that hydros doesn't have to resolve them
			// a second time?
			resolved, err := s.resolveImageToSha(*image, v1alpha1.MutableTagStrategy)

			if err != nil {
				// code returned by the service in code. The error code can be used
				// to switch on context specific functionality. In this case a context
				// specific error message is printed to the user based on the bucket
				// and key existing.
				//
				// For information on other S3 API error codes see:
				// http://docs.aws.amazon.com/AmazonS3/latest/API/ErrorResponses.html
				if aerr, ok := err.(awserr.Error); ok {
					switch aerr.Code() {
					case ecr.ErrCodeImageNotFoundException:
						missingImages = true
					default:
						return errors.Wrapf(err, "Failed to resolve image to sha; image: %v", image.ToURL())
					}
				}
			} else {
				log.V(util.Debug).Info("Resolved image", "image", image.ToURL(), "resolved", resolved)
			}
		}

		if missingImages {
			// Since the skaffold config could have been modified to change the registry we need to write it back out
			f, err := ioutil.TempFile("", "skaffold.*.yaml")

			newFile := f.Name()

			if err != nil {
				return errors.Wrapf(err, "Could not create temporary file to write updated skaffold config for %v", c.Path)
			}

			b, err := yaml.Marshal(c.Config)
			if err != nil {
				return errors.Wrapf(err, "Could not marshal skaffold config")
			}

			log.V(util.Debug).Info("Writing updated skaffold file", "config", string(b), "path", newFile)
			if _, err := f.Write(b); err != nil {
				return errors.Wrapf(err, "Failed to write skaffold config to file: %v", f.Name())
			}

			if err := f.Close(); err != nil {
				return errors.Wrapf(err, "Failed close file: %v", f.Name())
			}

			// Since at least one image is missing run skaffold build to build the image.
			wg.Add(1)
			go func() {
				defer wg.Done()
				// Build directory should be location of the original skaffold file
				buildDir := path.Dir(c.Path)
				err := skaffold.RunBuild(newFile, buildDir, tags, s.sess, log)
				if err != nil {
					errsMu.Lock()
					defer errsMu.Unlock()
					skaffoldErrs.AddCause(err)
				}
			}()
		}
	}

	wg.Wait()

	if len(skaffoldErrs.Causes) > 0 {
		skaffoldErrs.Final = errors.Errorf("Failed to build images using skaffold")
		return skaffoldErrs
	}
	return nil
}

func matches(k *kustomize.Kustomization, selector *meta.LabelSelector) (bool, error) {
	if k.MetaData == nil {
		return false, nil
	}

	if k.MetaData.Labels == nil {
		return false, nil
	}

	s, err := meta.LabelSelectorAsSelector(selector)
	if err != nil {
		return false, err
	}

	return s.Matches(labels.Set(k.MetaData.Labels)), nil
}

func isMatch(k *kustomize.Kustomization, toMatch map[string]string) bool {
	if k.CommonAnnotations == nil {
		return false
	}
	for key, expected := range toMatch {
		actual, ok := k.CommonAnnotations[key]
		if !ok {
			return false
		}

		if actual != expected {
			return false
		}
	}
	return true
}

// findKustomizationFiles finds all kustomization files below the specified path
func findKustomizationFiles(root string, repoRoot string, excludes []string, log logr.Logger) ([]string, error) {
	files := []string{}

	excludesSet := map[string]bool{}

	for _, e := range excludes {
		excludesSet[e] = true
	}

	err := filepath.Walk(root,
		func(path string, info os.FileInfo, err error) error {
			if info == nil {
				// N.B. I think this happens if path is the empty string.
				return fmt.Errorf("No info returned for path %v", info)
			}
			// skip directories
			if info.IsDir() {
				rPath, err := filepath.Rel(repoRoot, path)
				if err != nil {
					log.Error(err, "Could not compute relative path", "basePath", root, "path", path)
				}

				if _, ok := excludesSet[rPath]; ok {
					log.V(util.Debug).Info("Excluding directory", "dir", path)
					return filepath.SkipDir
				}

				return nil
			}

			// Skip non YAML files
			ext := strings.ToLower(filepath.Ext(info.Name()))

			if ext != ".yaml" && ext != ".yml" {
				return nil
			}

			if info.Name() != kustomizationFile {
				return nil
			}

			files = append(files, path)
			return nil
		})

	return files, err
}

// readKustomization will read a kustomization.yaml and return the kustomize object
func readKustomization(kfDefFile string) (*kustomize.Kustomization, error) {
	data, err := ioutil.ReadFile(kfDefFile)
	if err != nil {
		return nil, err
	}
	def := &kustomize.Kustomization{}
	if err = yaml.Unmarshal(data, def); err != nil {
		return nil, err
	}
	return def, nil
}
