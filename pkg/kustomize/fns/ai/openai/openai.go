package openai

import (
	"os"
	"path"

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

	// get home directory
	home, err := os.UserHomeDir()
	filesToTry := []string{}
	if err == nil {
		filesToTry = append(filesToTry, path.Join(home, "secrets", "openapi.api.key"))
	}

	for _, f := range filesToTry {
		log.Info("Trying to load API key from file", "file", f)
		key, err := os.ReadFile(f)
		if err != nil {
			log.Info("Failed to read file", "file", f, "err", err)
			continue
		}
		if len(key) != 0 {
			log.Info("Using API key from file", "file", f)
			return string(key)
		}
	}
	log.Info("Failed to find API key")
	return ""
}
