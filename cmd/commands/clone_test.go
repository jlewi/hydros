package commands

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/jlewi/hydros/pkg/util"
)

const (
	sailplaneHydrosKey   = "gcpsecretmanager:///projects/887891891186/secrets/hydros-ghapp-key/versions/latest"
	sailplaneHydrosAppID = int64(384797)
)

func Test_CloneConfig(t *testing.T) {
	type testCase struct {
		envs     map[string]string
		expected CloneConfig
	}

	cases := []testCase{
		{
			envs: map[string]string{
				"GIT_REPOS": "https://some/uri,https://some/other/uri",
			},
			expected: CloneConfig{
				Repos: []string{"https://some/uri", "https://some/other/uri"},
			},
		},
	}

	// N.B. This test actually modifies the environment variables which could be a problem
	for i, c := range cases {
		t.Run(fmt.Sprintf("case %v", i), func(t *testing.T) {
			// N.B. I'm not sure how to set the args without calling Execute
			// For now we rely on the test_clone function to test the args argument.
			cmd := NewCloneCmd()

			for k, v := range c.envs {
				os.Setenv(k, v)
			}

			if err := InitViper(cmd); err != nil {
				t.Fatalf("Failed to initialize viper; error %v", err)
			}
			actual := GetConfig()
			if d := cmp.Diff(c.expected, *actual); d != "" {
				t.Fatalf("Config mismatch; diff\n%v", d)
			}
		})

	}
}

func Test_CloneCmd(t *testing.T) {
	util.SetupLogger("info", true)
	if os.Getenv("GITHUB_ACTIONS") != "" {
		t.Skipf("Test is skipped in GitHub actions")
	}

	type testCase struct {
		repo   string
		outDir string
	}

	cases := []testCase{
		{
			repo:   "https://github.com/sailplaneai/roboweb.git",
			outDir: "sailplaneai/roboweb",
		},
		{
			repo:   "https://github.com/sailplaneai/roboweb.git?ref=jlewi/cicd",
			outDir: "sailplaneai/roboweb",
		},
		{
			repo:   "https://github.com/sailplaneai/kubepilot?sha=9fa5bc0",
			outDir: "sailplaneai/kubepilot",
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

			outDir := filepath.Join(tDir, "github.com", c.outDir)
			if _, err := os.Stat(outDir); err != nil {
				t.Fatalf("Failed to find cloned repo; error %v", err)
			}
		})
	}
}
