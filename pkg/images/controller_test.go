package images

import (
	"context"
	"github.com/go-git/go-git/v5"
	"github.com/jlewi/hydros/api/v1alpha1"
	"github.com/jlewi/hydros/pkg/gitutil"
	"github.com/jlewi/hydros/pkg/util"
	"gopkg.in/yaml.v3"
	"os"
	"path/filepath"
	"testing"
)

func Test_Controller(t *testing.T) {
	util.SetupLogger("info", true)

	if os.Getenv("GITHUB_ACTIONS") != "" {
		t.Skipf("Test_Build is a manual test that is skipped in CICD because it requires GCB")
	}

	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("Error getting working directory %v", err)
	}

	srcSpec := filepath.Join(cwd, "..", "..", "images.yaml")

	f, err := os.Open(srcSpec)
	if err != nil {
		t.Fatalf("Error opening spec %v", err)
	}

	gitRoot, err := gitutil.LocateRoot(cwd)
	if err != nil {
		t.Fatalf("Error locating git root %v", err)
	}
	gitRepo, err := git.PlainOpenWithOptions(gitRoot, &git.PlainOpenOptions{})
	if err != nil {
		t.Fatalf("Error opening git repo %v", err)
	}

	headRef, err := gitRepo.Head()
	if err != nil {
		t.Fatalf("Error getting head ref %v", err)
	}

	w, err := gitRepo.Worktree()
	if err != nil {
		t.Fatalf("Error getting worktree %v", err)
	}

	gitStatus, err := w.Status()
	if err != nil {
		t.Fatalf("Error getting git status %v", err)
	}

	image := &v1alpha1.Image{}
	if err := yaml.NewDecoder(f).Decode(image); err != nil {
		t.Fatalf("Error decoding image %v", err)
	}

	image.Status.SourceCommit += headRef.Hash().String()

	if !gitStatus.IsClean() {
		image.Status.SourceCommit += "-dirty"
	}
	c, err := NewController()
	if err != nil {
		t.Fatalf("Error creating controller %v", err)
	}

	ctx := context.Background()
	if err := c.Reconcile(ctx, image); err != nil {
		t.Fatalf("Error reconciling image %v", err)
	}

	// TODO(jeremy): check that the status is set
}
