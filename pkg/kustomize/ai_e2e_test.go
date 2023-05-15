package kustomize

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/jlewi/hydros/pkg/util"
	"github.com/otiai10/copy"
)

func Test_AIGeneratorE2E(t *testing.T) {
	if os.Getenv("GITHUB_ACTIONS") != "" {
		t.Skip("Skipping test on GitHub actions; this is an integration test that requires access to OpenAI")
	}
	log := util.SetupLogger("info", true)

	currDir, err := os.Getwd()
	if err != nil {
		t.Fatalf("Failed to get current working directory; error %v", err)
	}

	sourceDir := filepath.Join(currDir, "fns", "ai", "test_data")
	// Create a temporary directory because the function will modify the directory.
	outDir, err := os.MkdirTemp("", "aiGeneratorE2E")
	if err != nil {
		t.Errorf("Failed to create temporary directory; %v", err)
		return
	}

	err = copy.Copy(sourceDir, outDir)

	if err != nil {
		t.Errorf("Failed to copy %v to %v; error %v", sourceDir, outDir, err)
		return
	}

	dis := Dispatcher{
		Log: log,
	}

	log.Info("Processing dir", "dir", outDir)
	functionPaths := []string{sourceDir}
	err = dis.RunOnDir(outDir, functionPaths)
	if err != nil {
		t.Errorf("RunOnDir failed; error %v", err)
		return
	}
}
