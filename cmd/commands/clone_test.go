package commands

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

const (
	sailplaneHydrosKey   = "gcpsecretmanager:///projects/887891891186/secrets/hydros-ghapp-key/versions/latest"
	sailplaneHydrosAppID = int64(384797)
)

func Test_CloneCmd(t *testing.T) {
	if os.Getenv("GITHUB_ACTIONS") != "" {
		t.Skipf("Test is skipped in GitHub actions")
	}

	type testCase struct {
		repo string
	}

	cases := []testCase{
		{
			repo: "https://github.com/sailplaneai/roboweb.git",
		},
		{
			repo: "https://github.com/sailplaneai/roboweb.git?ref=jlewi/cicd",
		},
	}

	for i, c := range cases {
		t.Run(fmt.Sprintf("case %d", i), func(t *testing.T) {
			tDir, err := os.MkdirTemp("", "testClone")
			if err != nil {
				t.Fatalf("Failed to create temporary directory; %v", err)
			}

			t.Logf("Cloning to %v", tDir)
			// TODO(jeremy): We should be using the root command but that requires refactoring the code to make it testable.
			cmd := NewCloneCmd()
			cmd.SetArgs([]string{
				"--repo=" + c.repo,
				"--work-dir=" + tDir,
				"--app-id=" + fmt.Sprintf("%d", sailplaneHydrosAppID),
				"--private-key=" + sailplaneHydrosKey,
			})
			if err := cmd.Execute(); err != nil {
				t.Fatalf("Failed to run clone; error %v", err)
			}

			outDir := filepath.Join(tDir, "github.com/sailplaneai/roboweb")
			if _, err := os.Stat(outDir); err != nil {
				t.Fatalf("Failed to find cloned repo; error %v", err)
			}
		})
	}
}
