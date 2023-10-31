package commands

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/viper"

	"github.com/go-logr/zapr"
	"github.com/jlewi/hydros/pkg/files"
	"github.com/jlewi/hydros/pkg/github"
	"github.com/spf13/cobra"
	"go.uber.org/zap"
)

// NewCloneCmd create a clone command
func NewCloneCmd() *cobra.Command {
	var repo []string
	var appID int64
	var workDir string
	var privateKeyFile string
	cmd := &cobra.Command{
		Use:   "clone --repo <repo>  --target ",
		Short: "Clone one or more repositories to the target directory",
		Long: strings.Join([]string{
			"Clone one or more repositories to the target directory. ",
			"This is useful for checking out a bunch of repositories in a container. ",
			"The main purpose of this file is to handle auth e.g. using a GitHub App",
		}, ""),
		Run: func(cmd *cobra.Command, args []string) {
			err := func() error {
				log := zapr.NewLogger(zap.L())

				if err := InitViper(cmd); err != nil {
					return err
				}
				config := GetConfig()
				privateKey, err := files.Read(privateKeyFile)
				if err != nil {
					return err
				}
				manager, err := github.NewTransportManager(appID, privateKey, log)
				if err != nil {
					return err
				}

				cloner := github.ReposCloner{
					URIs:    config.Repos,
					Manager: manager,
					BaseDir: workDir,
				}

				return cloner.Run(context.TODO())
			}()
			if err != nil {
				fmt.Printf("clone failed; error %+v\n", err)
				os.Exit(1)
			}
		},
	}

	// TODO(jeremy): We should support using a config file.
	cmd.Flags().StringArrayVarP(&repo, "repo", "", []string{}, "The URLs of the repos to clone.")
	cmd.Flags().StringVarP(&workDir, "work-dir", "", "", "Directory where repos should be checked out")
	cmd.Flags().StringVarP(&privateKeyFile, "private-key", "", "", "Path to the file containing the secret for the GitHub App to Authenticate as. Can be stored in gcpSecretManager.")
	cmd.Flags().Int64VarP(&appID, "app-id", "", 0, "GitHubAppId.")

	return cmd
}

type CloneConfig struct {
	Repos []string `json:"repos" yaml:"repos"`
}

// InitViper reads in config file and ENV variables if set.
// The results are stored inside viper. Call GetConfig to get a configuration.
// The cmd is passed in so we can bind to command flags
func InitViper(cmd *cobra.Command) error {
	// TODO(jeremy): Should we be setting defaults?
	// see https://github.com/spf13/viper#establishing-defaults
	viper.SetEnvPrefix("hydros")

	// TODO(jeremy): Automic env doesn't seem to be working as expected. We should be able to set the environment
	// variables to override the keys but that doesn't seem to be working.
	viper.AutomaticEnv() // read in environment variables that match

	if err := viper.BindEnv("repos", "GIT_REPOS"); err != nil {
		return err
	}

	// Bind to the command line flag if it was specified.
	keyToflagName := map[string]string{
		"repos": "repo",
	}

	if cmd != nil {
		for key, flag := range keyToflagName {
			if err := viper.BindPFlag(key, cmd.Flags().Lookup(flag)); err != nil {
				return err
			}
		}
	}

	return nil
}

// GetConfig returns the configuration instantiated from the viper configuration.
func GetConfig() *CloneConfig {
	// N.B. THis is a bit of a hacky way to load the configuration while allowing values to be overwritten by viper
	cfg := &CloneConfig{}

	if err := viper.Unmarshal(cfg); err != nil {
		panic(fmt.Errorf("Failed to unmarshal configuration; error %v", err))
	}

	return cfg
}
