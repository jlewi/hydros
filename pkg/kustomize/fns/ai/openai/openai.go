package openai

import (
	"os"
	"path"

	"github.com/jlewi/hydros/pkg/files"

	"github.com/go-logr/zapr"
	"go.uber.org/zap"
)

// GetAPIKey tries to get the APIKey from a bunch of different places
func GetAPIKey() string {
	log := zapr.NewLogger(zap.L())
	key := os.Getenv("OPENAI_API_KEY")
	if key != "" {
		log.Info("Using OPENAI_API_KEY from environment")
		return key
	}

	uri := os.Getenv("OPENAI_API_KEY_URI")
	if uri != "" {
		key, err := files.Read(uri)
		if err == nil && len(key) > 0 {
			log.Info("Obtained OpenAI API Key from URI specified in environment variable OPENAI_API_KEY_URI", "uri", uri)
			return string(key)
		}
		if err != nil || len(key) == 0 {
			log.Info("Failed to read OpenAI API Key from URI in OPENAI_API_KEY_URI environment variable", "uri", uri, "err", err)
		}
	}

	// get home directory
	home, err := os.UserHomeDir()
	filesToTry := []string{}
	if err == nil {
		filesToTry = append(filesToTry, path.Join(home, "secrets", "openapi.api.key"))
	}

	for _, f := range filesToTry {
		log.Info("Trying to load API key from uri", "file", f)
		key, err := files.Read(f)
		if err != nil {
			log.Info("Failed to read URI", "uri", f, "err", err)
			continue
		}
		if len(key) != 0 {
			log.Info("Using API key from URI", "uri", f)
			return string(key)
		}
	}
	log.Info("Failed to find API key")
	return ""
}
