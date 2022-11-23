//go:build integration

package github

import (
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/go-git/go-git/v5"
	"github.com/go-logr/zapr"
	"github.com/jlewi/hydros/pkg/files"
	"github.com/jlewi/hydros/pkg/util"
	"github.com/kubeflow/testing/go/pkg/ghrepo"
	"github.com/pkg/errors"
	"go.uber.org/zap"
)

const (
	appID      = int64(263648)
	privateKey = "gcpSecretManager:///projects/dev-starling/secrets/annotater-bot/versions/latest"
)

func getTransportManager() (*TransportManager, error) {
	log := zapr.NewLogger(zap.L())

	f := &files.Factory{}
	h, err := f.Get(privateKey)
	if err != nil {
		return nil, err
	}
	r, err := h.NewReader(privateKey)
	if err != nil {
		return nil, err
	}
	secretByte, err := io.ReadAll(r)

	if err != nil {
		return nil, err
	}

	return NewTransportManager(appID, secretByte, log)
}

func checkDirAtCommit(fullDir string, expected string) error {
	// Open the repository and get the current hash to verify it is the expected hash.
	// Open the repository
	r, err := git.PlainOpenWithOptions(fullDir, &git.PlainOpenOptions{})
	if err != nil {
		return errors.Wrapf(err, "Could not open respoistory at %v; ensure the directory contains a git repo", fullDir)
	}

	hash, err := r.Head()
	if err != nil {
		return err
	}

	if hash.Hash().String() != expected {
		return errors.Errorf("Unexpected hash; Got %v; want %v", hash.String(), expected)
	}
	return nil
}

// Test_PrepareBranch is an integration test.
func Test_PrepareBranch(t *testing.T) {
	// This test verifies that we can check out a repository to a clean directory
	log := util.SetupLogger("debug", true)

	tempDir, err := os.MkdirTemp("", "testClone")
	if err != nil {
		t.Fatalf("Failed to create temporary directory; %v", err)
	}
	defer os.RemoveAll(tempDir)

	now := time.Now().Format("20060102-030405")

	// expected should be the current latest commit on the branch we are testing. If new commits are
	// added this will need to be updated
	expected := "a9b473353b73a4cd5e2c8809c4c16a0e9e164129"

	args := &RepoHelperArgs{
		BaseRepo:   ghrepo.New("starlingai", "gitops-test-repo"),
		GhTr:       nil,
		Name:       "notset",
		Email:      "notset@acme.com",
		BaseBranch: "test-cases/clone-1",
		BranchName: "clone-test" + now,
	}

	args.FullDir = filepath.Join(tempDir, args.BaseRepo.RepoOwner(), args.BaseRepo.RepoName())
	err = func() error {
		manager, err := getTransportManager()
		if err != nil {
			return err
		}

		tr, err := manager.Get(args.BaseRepo.RepoOwner(), args.BaseRepo.RepoName())
		if err != nil {
			return err
		}

		args.GhTr = tr

		repo, err := NewGithubRepoHelper(args)

		if err != nil {
			return err
		}

		if err := repo.PrepareBranch(true); err != nil {
			return err
		}

		if err := checkDirAtCommit(args.FullDir, expected); err != nil {
			return err
		}

		log.Info("Initial clone succeeded; checking out a different branch and retrying")

		// Run PrepareBranch a second time this way we can verify we can clone the repository even when we
		// are already checked out. First checkout a different branch
		if err := func() error {
			r, err := git.PlainOpenWithOptions(repo.fullDir, &git.PlainOpenOptions{})
			if err != nil {
				return errors.Wrapf(err, "Could not open respoistory at %v; ensure the directory contains a git repo", repo.fullDir)
			}

			w, err := r.Worktree()
			if err != nil {
				return err
			}
			if err := w.Checkout(&git.CheckoutOptions{
				Branch: "refs/heads/main",
			}); err != nil {
				return err
			}
			return nil
		}(); err != nil {
			return err
		}

		if err := repo.PrepareBranch(true); err != nil {
			return err
		}

		if err := checkDirAtCommit(args.FullDir, expected); err != nil {
			return err
		}

		return nil
	}()

	if err != nil {
		t.Fatalf("Failed to clone the repository; error %+v", err)
	}
}

// Test_PrepareCommitAndPush tests that we can go through the full cycle of checking out a branch,
// modifying it, and then committing and pushing the changes.
func Test_PrepareCommitAndPush(t *testing.T) {
	util.SetupLogger("debug", true)

	tempDir, err := os.MkdirTemp("", "testClone")
	if err != nil {
		t.Fatalf("Failed to create temporary directory; %v", err)
	}
	defer os.RemoveAll(tempDir)

	now := time.Now().Format("20060102-030405")

	args := &RepoHelperArgs{
		BaseRepo:   ghrepo.New("starlingai", "gitops-test-repo"),
		GhTr:       nil,
		Name:       "notset",
		Email:      "notset@acme.com",
		BaseBranch: "test-cases/clone-1",
		BranchName: "clone-test" + now,
	}

	args.FullDir = filepath.Join(tempDir, args.BaseRepo.RepoOwner(), args.BaseRepo.RepoName())
	err = func() error {
		manager, err := getTransportManager()
		if err != nil {
			return err
		}

		tr, err := manager.Get(args.BaseRepo.RepoOwner(), args.BaseRepo.RepoName())
		if err != nil {
			return err
		}

		args.GhTr = tr

		repo, err := NewGithubRepoHelper(args)

		if err != nil {
			return err
		}

		if err := repo.PrepareBranch(true); err != nil {
			return err
		}

		// Write a file
		filename := filepath.Join(repo.fullDir, "test-file-"+now+".txt")
		if err := os.WriteFile(filename, []byte("hello world"), util.FilePermUserGroup); err != nil {
			return err
		}

		if err := repo.CommitAndPush("Commit from test", true); err != nil {
			return err
		}
		return nil
	}()

	if err != nil {
		t.Fatalf("Failed to clone the repository; error %+v", err)
	}
}
