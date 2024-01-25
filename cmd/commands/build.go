package commands

import (
	"fmt"
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
			if err := images.ReconcileFile(opts.File); err != nil {
				fmt.Printf("takeover failed; error %+v\n", err)
			}
		},
	}

	cmd.Flags().StringVarP(&opts.File, "file", "f", "", "The file containing the images to apply")

	cmd.MarkFlagRequired("file")
	cmd.MarkFlagRequired("private-key")
	return cmd
}
