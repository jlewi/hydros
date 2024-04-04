package gitutil

import (
	"bufio"
	"os"
	"path/filepath"
	"time"

	"github.com/go-git/go-git/v5/plumbing/object"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/config"
	"github.com/go-git/go-git/v5/plumbing/format/gitignore"
	"github.com/go-logr/zapr"
	"github.com/jlewi/hydros/pkg/util"
	"go.uber.org/zap"

	"github.com/pkg/errors"
)

// LocateRoot locates the root of the git repository at path.
// Returns empty string if not a git repo.
func LocateRoot(origPath string) (string, error) {
	// If we don't get the absolute path then for a relative path such as "image.yaml" we end up returning "." as the
	// dir and the loop never terminates
	path, err := filepath.Abs(origPath)
	if err != nil {
		return "", errors.Wrapf(err, "Could not locate git root for %v because the absolute path could not be obtained", origPath)

	}
	fInfo, err := os.Stat(path)
	if err != nil {
		return "", errors.Wrapf(err, "Error stating path %v", path)
	}
	if !fInfo.IsDir() {
		path = filepath.Dir(path)
	}
	for {
		gDir := filepath.Join(path, ".git")
		_, err := os.Stat(gDir)
		if err == nil {
			return path, nil
		}

		if os.IsNotExist(err) {
			path = filepath.Dir(path)
			if path == string(os.PathSeparator) {
				return "", nil
			}
			continue
		}
		return "", errors.Wrapf(err, "Error checking for directory %v", gDir)
	}
}

// AddGitignoreToWorktree adds the exclude paths in the .gitignore file to the worktree.
// This is a workaround for https://github.com/go-git/go-git/issues/597. If we don't call this before doing add
// all then the ignore patterns won't be respected.
//
// N.B. It looks like we also need to call this function if we want IsClean to ignore files
// N.B this doesn't work with nested .gitignore files.
func AddGitignoreToWorktree(wt *git.Worktree, repositoryPath string) error {
	log := zapr.NewLogger(zap.L())

	path := filepath.Join(repositoryPath, ".gitignore")
	if _, err := os.Stat(path); os.IsNotExist(err) {
		log.Info("No .gitignore file for repository", "repository", repositoryPath)
		return nil
	}
	readFile, err := os.Open(path)
	defer util.DeferIgnoreError(readFile.Close)

	if err != nil {
		return errors.Wrapf(err, "Failed to read .gitignore: %s", path)
	}

	fileScanner := bufio.NewScanner(readFile)
	fileScanner.Split(bufio.ScanLines)

	for fileScanner.Scan() {
		ignorePattern := fileScanner.Text()
		if ignorePattern == "" {
			continue
		}
		log.V(util.Debug).Info("add .gitignore pattern to ignore list", "pattern", ignorePattern)
		wt.Excludes = append(wt.Excludes, gitignore.ParsePattern(ignorePattern, nil))
	}
	if err := fileScanner.Err(); err != nil {
		return err
	}
	return nil
}

type User struct {
	Name  string
	Email string
}

// LoadUser gets the user information for the repository.
func LoadUser(r *git.Repository) (*User, error) {
	cfg, err := r.Config()
	if err != nil {
		return nil, err
	}

	user := &User{
		Name:  cfg.User.Name,
		Email: cfg.User.Email,
	}

	if user.Name != "" && user.Email != "" {
		return user, nil
	}

	// Since Name and/or Email aren't set in the local scope. Try the global scope
	gCfg, err := config.LoadConfig(config.GlobalScope)
	if err != nil {
		return user, errors.Wrapf(err, "Failed to load GlobalConfig")
	}

	if user.Name == "" {
		user.Name = gCfg.User.Name
	}
	if user.Email == "" {
		user.Email = gCfg.User.Email
	}

	// N.B it doesn't make sense to check the system configuration because that would apply to all users
	// so why would you set the name and email their?
	return user, nil
}

// CommitAll is a helper function to commit all changes in the repository.
// null op if its clean.
// w is the worktree
// You should call AddGitIgnore on the worktree before calling this function if you want to ensure files are ignored.
// TODO(jeremy): I'm not sure this is the right API. I did it this way because on some repositories it was really slow
// to get the worktree and add .gitignore so I didn't want to do that more than once.
// https://github.com/jlewi/hydros/issues/84 is tracking the slowness.
func CommitAll(r *git.Repository, w *git.Worktree, message string) error {
	log := zapr.NewLogger(zap.L())
	log.Info("Getting git status")
	status, err := w.Status()
	if err != nil {
		return err
	}

	if status.IsClean() {
		log.Info("tree is clean; no commit needed")
		return nil
	}
	log.Info("committing all files")
	if err := w.AddWithOptions(&git.AddOptions{All: true}); err != nil {
		return err
	}

	user, err := LoadUser(r)
	if err != nil {
		return err
	}
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

	return nil
}

// TrackedIsClean returns true if the repository is clean except for untracked files.
// git.IsClean doesn't work because it doesn't ignore untracked files.
func TrackedIsClean(gitStatus git.Status) bool {
	for _, s := range gitStatus {
		if s.Staging == git.Untracked || s.Worktree == git.Untracked {
			continue
		}
		if s.Staging != git.Unmodified || s.Worktree != git.Unmodified {
			return false
		}
	}

	return true
}
