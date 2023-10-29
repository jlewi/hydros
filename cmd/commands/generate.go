package commands

import (
	"fmt"

	"github.com/go-logr/zapr"
	"github.com/jlewi/hydros/pkg/kustomize"
	"github.com/spf13/cobra"
	"go.uber.org/zap"
)

func NewGenerateCmd() *cobra.Command {
	var functionPath string
	// TODO(jeremy): This is a bug.  We aren't setting the function path.
	cmd := &cobra.Command{
		Use:   "generate -f hydros_ai.yaml  <config directory>",
		Short: "Use OpenAI to generate KRM functions from NL descriptions in ai.hydros.io/${TAG} annotations",
		// Require at least one argument
		Args: cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			dir := args[0]

			if err := Generate(dir, functionPath); err != nil {
				fmt.Printf("generate failed; error %+v\n", err)
			}
		},
	}

	return cmd
}

func Generate(dir string, functionPath string) error {
	log := zapr.NewLogger(zap.L())

	log.Info("Processing dir", "dir", dir)
	functionPaths := []string{dir}
	dis := kustomize.Dispatcher{Log: log}
	return dis.RunOnDir(dir, functionPaths)
}
