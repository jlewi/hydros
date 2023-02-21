package app

import (
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"

	"github.com/jlewi/hydros/pkg/util"
	"gopkg.in/yaml.v3"
)

// Not really a unittest as we try to sanitize the hydros directory
func Test_sanitize(t *testing.T) {
	dir, err := os.MkdirTemp("", "testSanitize")
	if err != nil {
		t.Fatalf("Failed to create temporary directory; error %v", err)
	}
	t.Logf("Using directory: %v", dir)

	wDir, err := os.Getwd()
	if err != nil {
		t.Fatalf("Failed to get working directory; error %v", err)
	}

	// TODO: This is a giant hack. sanitizer configs are stored in a different repository for safety.
	// We assume that repository is checked out as git_oss-sanitizer-configs. If it isn't we will just skip the test
	source, err := filepath.Abs(filepath.Join(wDir, "..", "..", ".."))
	if err != nil {
		t.Fatalf("Failed to get absolute path; error %v", err)
	}

	log := util.SetupLogger("info", true)

	configPath, err := filepath.Abs(filepath.Join(source, "..", "git_oss-sanitizer-configs", "hydros.yaml"))
	if err != nil {
		t.Fatalf("Failed to get absolute path; error %v", err)
	}

	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		t.Logf("ConfigPath %v doesn't exist; skipping test", configPath)
		return
	}

	b, err := ioutil.ReadFile(configPath)
	if err != nil {
		t.Fatalf("Failed to read config from path; %v; error %v", configPath, err)
	}

	config := &Sanitize{}
	if err := yaml.Unmarshal(b, config); err != nil {
		t.Fatalf("Failed to unmarshal config from path; %v;  error %v", configPath, err)
	}

	s, err := New(*config, log)
	if err != nil {
		t.Fatalf("Failed to create new Sanitizer; error %v", err)
	}

	if err := s.Run(source, dir); err != nil {
		t.Fatalf("Sanitzation failed; error %+v", err)
	}
}
