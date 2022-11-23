//go:build integration

package github

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/go-git/go-git/v5"
)

// Test_AppAuth is an integration test for the AppAuth. It verifies we can clone a private repository using
// a GitHub App's credentials.
func Test_AppAuth(t *testing.T) {
	err := func() error {

		tempDir, err := os.MkdirTemp("", "testClone")
		if err != nil {
			t.Fatalf("Failed to create temporary directory; %v", err)
		}
		defer os.RemoveAll(tempDir)

		Org := "starlingai"
		Repo := "gitops-test-repo"
		manager, err := getTransportManager()
		if err != nil {
			return err
		}

		tr, err := manager.Get(Org, Repo)
		if err != nil {
			return err
		}

		fullDir := filepath.Join(tempDir, Org, Repo)
		_, err = git.PlainClone(fullDir, false, &git.CloneOptions{
			URL: fmt.Sprintf("https://github.com/%v/%v.git", Org, Repo),
			Auth: &AppAuth{
				Tr: tr,
			},
			Progress: os.Stdout,
		})
		return err
	}()

	if err != nil {
		t.Fatalf("Failed with error:%+v", err)
	}
}
