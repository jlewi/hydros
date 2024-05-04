package commands

import (
	"fmt"
	"os"

	"github.com/jlewi/hydros/pkg/app"

	"github.com/jlewi/hydros/pkg/images"
	"github.com/spf13/cobra"
)

type BuildArgs struct {
	File string
}

func NewBuildCmd() *cobra.Command {
	opts := &BuildArgs{}
	cmd := &cobra.Command{
		Use:   "build -f <resource.yaml>",
		Short: "Build any images defined in the specified file",
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
				logVersion()
				return images.ReconcileFile(opts.File)
			}()

			if err != nil {
				fmt.Printf("build failed; error %+v\n", err)
				os.Exit(1)
			}
		},
	}

	cmd.Flags().StringVarP(&opts.File, "file", "f", "", "The file containing the images to apply")

	cmd.MarkFlagRequired("file")
	cmd.MarkFlagRequired("private-key")
	return cmd
}
