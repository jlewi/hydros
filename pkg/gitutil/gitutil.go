package gitutil

import (
	"bufio"
	"os"
	"path/filepath"

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
func LocateRoot(path string) (string, error) {
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
// N.B this doesn't work with nested .gitignore files.
func AddGitignoreToWorktree(wt *git.Worktree, repositoryPath string) error {
	log := zapr.NewLogger(zap.L())

	path := filepath.Join(repositoryPath, ".gitignore")
	if _, err := os.Stat(path); os.IsNotExist(err) {
		log.Info("No .gitignore file for repository", "repository", repositoryPath)
		return nil
	}
	readFile, err := os.Open(path)
	util.DeferIgnoreError(readFile.Close)

	if err != nil {
		return errors.Wrapf(err, "Failed to read .gitignore: %s", path)
	}

	fileScanner := bufio.NewScanner(readFile)
	fileScanner.Split(bufio.ScanLines)

	for fileScanner.Scan() {
		ignorePattern := fileScanner.Text()
		log.V(util.Debug).Info("add .gitignore pattern to ignore list", "pattern", ignorePattern)
		wt.Excludes = append(wt.Excludes, gitignore.ParsePattern(ignorePattern, nil))
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
