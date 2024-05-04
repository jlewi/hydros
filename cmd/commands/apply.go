package commands

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/jlewi/hydros/pkg/app"
	"github.com/jlewi/hydros/pkg/config"

	"github.com/go-logr/zapr"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"
	"go.uber.org/zap"
)

type applyOptions struct {
	workDir     string
	secret      string
	githubAppID int
	period      time.Duration
	force       bool
}

// NewApplyCmd create an apply command
func NewApplyCmd() *cobra.Command {
	aOptions := applyOptions{}

	// TODO(jeremy): We should update apply to support the image resource.
	applyCmd := &cobra.Command{
		Use:   "apply <resource.yaml> <resourceDir> <resource.yaml> ...",
		Short: "Apply the specified resource.",
		Run: func(cmd *cobra.Command, args []string) {
			err := func() error {
				app := app.NewApp()
				defer app.Shutdown()
				if err := app.LoadConfig(cmd); err != nil {
					return err
				}
				if err := app.SetupLogging(); err != nil {
					return err
				}
				log := zapr.NewLogger(zap.L())
				if len(args) == 0 {
					log.Info("apply takes at least one argument which should be the file or directory YAML to apply.")
					return errors.New("apply takes at least one argument which should be the file or directory YAML to apply.")
				}
				logVersion()

				if err := app.SetupRegistry(); err != nil {
					return err
				}

				return app.ApplyPaths(context.Background(), args, aOptions.period, aOptions.force)
			}()
			if err != nil {
				fmt.Printf("Error running apply;\n %+v\n", err)
				os.Exit(1)
			}
		},
	}

	applyCmd.Flags().StringVarP(&aOptions.workDir, config.WorkDirFlagName, "", "", "Directory where repos should be checked out")
	applyCmd.Flags().StringVarP(&aOptions.secret, config.PrivateKeyFlagName, "", "", "Path to the file containing the secret for the GitHub App to Authenticate as.")
	applyCmd.Flags().IntVarP(&aOptions.githubAppID, config.AppIDFlagName, "", 0, "GitHubAppId.")
	applyCmd.Flags().DurationVarP(&aOptions.period, "period", "p", 0*time.Minute, "The period with which to reapply. If zero run once and exit.")
	applyCmd.Flags().BoolVarP(&aOptions.force, "force", "", false, "Force a sync even if one isn't needed.")

	return applyCmd
}
