package commands

import (
	"context"
	"fmt"
	"github.com/go-logr/zapr"
	"github.com/jlewi/hydros/pkg/files"
	"github.com/jlewi/hydros/pkg/github"
	"github.com/spf13/cobra"
	"go.uber.org/zap"
	"strings"
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

				privateKey, err := files.Read(privateKeyFile)
				if err != nil {
					return err
				}
				manager, err := github.NewTransportManager(appID, privateKey, log)
				if err != nil {
					return err
				}

				cloner := github.ReposCloner{
					URIs:    repo,
					Manager: manager,
					BaseDir: "",
				}

				return cloner.Run(context.TODO())
			}()
			if err != nil {
				fmt.Printf("clone failed; error %+v\n", err)
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
