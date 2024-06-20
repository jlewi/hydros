package github

import (
	"context"
	"os"
	"testing"

	"github.com/jlewi/monogo/files"
	"github.com/jlewi/hydros/pkg/util"
)

const (
	hydrosKey   = "gcpsecretmanager:///projects/513170322007/secrets/hydros-ghapp-key/versions/latest"
	hydrosAppID = int64(384797)
)

// N.B. there is also a test in cmd/commands/clone_test.go
func Test_ReposCloner(t *testing.T) {
	if os.Getenv("GITHUB_ACTIONS") != "" {
		t.Skipf("Test is skipped in GitHub actions")
	}

	privateKey, err := files.Read(hydrosKey)
	if err != nil {
		t.Fatalf("Failed to read key; error %v", err)
	}

	log := util.SetupLogger("info", true)

	manager, err := NewTransportManager(hydrosAppID, privateKey, log)

	if err != nil {
		t.Fatalf("Failed to create transport manager; error %v", err)
	}

	tDir, err := os.MkdirTemp("", "testClone")
	if err != nil {
		t.Fatalf("Failed to create temporary directory; %v", err)
	}

	cloner := &ReposCloner{
		Manager: manager,
		BaseDir: tDir,
		URIs:    []string{"https://github.com/jlewi/hydros-hydrated.git"},
	}

	if err := cloner.Run(context.TODO()); err != nil {
		t.Fatalf("Failed to clone; error %v", err)
	}
	t.Logf("Successfully cloned to %v", tDir)
}
