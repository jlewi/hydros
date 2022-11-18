package github

import (
	"context"
	"fmt"
	"io"
	"io/ioutil"
	"os"

	"github.com/jlewi/hydros/pkg/github"
	"github.com/jlewi/hydros/pkg/hydros"
	"github.com/jlewi/hydros/pkg/util"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"
)

// NewAppTokenCmd creates a command to generate a token for a GitHub App.
func NewAppTokenCmd(w io.Writer, level *string, devLogger *bool) *cobra.Command {
	var org string
	var repo string
	var githubAppID int
	var secret string
	var envFile string

	cmd := &cobra.Command{
		Use:   "github-app-token",
		Short: "Get a github app authorization token.",
		Run: func(cmd *cobra.Command, args []string) {
			log := util.SetupLogger(*level, *devLogger)
			err := func() error {
				manager, err := github.NewTransportManager(int64(githubAppID), secret, log)
				if err != nil {
					return errors.Wrapf(err, "TransportManager creation failed")
				}

				tr, err := manager.Get(org, repo)
				if err != nil {
					return errors.Wrapf(err, "Failed to create transport")
				}

				// Generate an access token
				token, err := tr.Token(context.Background())
				if err != nil {
					return errors.Wrapf(err, "Failed to create token")
				}

				if envFile != "" {
					fmt.Fprintf(w, "Writing token to environment file %v", envFile)
					perm := os.FileMode(int(0o700))
					err := ioutil.WriteFile(envFile, []byte(fmt.Sprintf("export GITHUB_TOKEN=%v", token)), perm)
					if err != nil {
						return errors.Wrapf(err, "Failed to write file %v", envFile)
					}
				} else {
					fmt.Fprintf(w, "%v\n", token)
				}

				return nil
			}()
			if err != nil {
				fmt.Fprintf(w, "Failed to get resource; error:\n%v", err)
			}
		},
	}

	cmd.Flags().StringVarP(&secret, "private-key", "", "", "Path to the file containing the secret for the GitHub App to Authenticate as.")
	cmd.Flags().IntVarP(&githubAppID, "appId", "", hydros.HydrosGitHubAppID, "GitHubAppId.")
	cmd.Flags().StringVarP(&org, "org", "o", "PrimerAI", "The GitHub org to obtain the token for")
	cmd.Flags().StringVarP(&repo, "repo", "r", "", "The repo obtain the token for")
	cmd.Flags().StringVarP(&envFile, "env-file", "f", "", "The file to right the github token to")

	util.IgnoreError(cmd.MarkFlagRequired("private-key"))
	return cmd
}
