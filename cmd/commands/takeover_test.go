//go:build integration

package commands

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/jlewi/hydros/pkg/util"
)

const (
	appID      = int64(266158)
	privateKey = "secrets/hydros-bot.2022-11-27.private-key.pem"
)

// Test_TakeOver is an integration test for the PushLocal function.
func Test_TakeOver(t *testing.T) {
	util.SetupLogger("info", true)
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("Failed to get cwd; %v", err)
	}

	syncFile := filepath.Join(cwd, "test_data", "devsync.yaml")

	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatalf("Could not get home directory; %v", err)
	}

	hydrosKeyFile := filepath.Join(home, privateKey)

	args := &TakeOverArgs{
		WorkDir:     "",
		Secret:      hydrosKeyFile,
		GithubAppID: appID,
		Force:       false,
		File:        syncFile,
		KeyFile:     "",
		RepoDir:     "",
	}

	if err := TakeOver(args); err != nil {
		t.Fatalf("Takeover failed; error %+v", err)
	}
}
