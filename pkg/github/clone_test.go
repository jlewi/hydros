package github

import (
	"context"
	"github.com/jlewi/hydros/pkg/files"
	"github.com/jlewi/hydros/pkg/util"
	"os"
	"testing"
)

const (
	sailplaneHydrosKey   = "gcpsecretmanager:///projects/887891891186/secrets/hydros-ghapp-key/versions/latest"
	sailplaneHydrosAppID = int64(384797)
)

// N.B. there is also a test in cmd/commands/clone_test.go
func Test_ReposCloner(t *testing.T) {
	if os.Getenv("GITHUB_ACTIONS") != "" {
		t.Skipf("Test is skipped in GitHub actions")
	}

	privateKey, err := files.Read(sailplaneHydrosKey)
	if err != nil {
		t.Fatalf("Failed to read key; error %v", err)
	}

	log := util.SetupLogger("info", true)

	manager, err := NewTransportManager(sailplaneHydrosAppID, privateKey, log)

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
		URIs:    []string{"https://github.com/sailplaneai/roboweb.git"},
	}

	if err := cloner.Run(context.TODO()); err != nil {
		t.Fatalf("Failed to clone; error %v", err)
	}
	t.Logf("Successfully cloned to %v", tDir)
}
