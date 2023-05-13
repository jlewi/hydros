package openai

import (
	"os"
	"time"

	"github.com/PullRequestInc/go-gpt3"
	"github.com/go-logr/zapr"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"
	"go.uber.org/zap"
)

type Client struct{}

type ClientFlags struct {
	KeyFile string
}

func (f *ClientFlags) NewClient() (gpt3.Client, error) {
	log := zapr.NewLogger(zap.L())

	var apiKey string
	if f.KeyFile != "" {
		log.Info("Using OpenAI API key from file", "file", f.KeyFile)

		contents, err := os.ReadFile(f.KeyFile)
		if err != nil {
			return nil, errors.Wrapf(err, "Failed to read file %v", f.KeyFile)
		}
		apiKey = string(contents)
	} else {
		apiKey = GetAPIKey()
		if apiKey == "" {
			return nil, errors.New("Failed to find OpenAI API key")
		}
	}

	// Increase the OpenAI timeout.
	gClient := gpt3.NewClient(string(apiKey), gpt3.WithTimeout(3*time.Minute))

	return gClient, nil
}

func (f *ClientFlags) AddFlags(cmd *cobra.Command) {
	cmd.Flags().StringVarP(&f.KeyFile, "openai-key-file", "", "", "File containing OpenAI API key")
}
