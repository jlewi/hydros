package commands

import (
	"fmt"
	"os"
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

	tDir, err := os.MkdirTemp("", "testClone")
	if err != nil {
		t.Fatalf("Failed to create temporary directory; %v", err)
	}

	cmd := NewCloneCmd()

	cmd.SetArgs([]string{
		"--repo=https://github.com/sailplaneai/roboweb.git",
		"--work-dir=" + tDir,
		"--app-id=" + fmt.Sprintf("%d", sailplaneHydrosAppID),
		"--private-key=" + sailplaneHydrosKey,
	})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("Failed to run clone; error %v", err)
	}
}
