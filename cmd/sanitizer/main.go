package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/PrimerAI/go-micro-utils-public/gmu/logging"
	"github.com/go-logr/zapr"
	"github.com/jlewi/hydros/cmd/sanitizer/app"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"
	"go.uber.org/zap"
	"gopkg.in/yaml.v3"
)

const (
	// ProgramName name.
	ProgramName = "sanitizer"
)

// NewDefaultCommand returns the default (aka root) command for sanitizer.
func NewDefaultCommand() *cobra.Command {
	var level string
	var jsonLogger bool
	c := &cobra.Command{
		Use: ProgramName,
		PersistentPreRun: func(cmd *cobra.Command, args []string) {
			logging.SetupLogger(level, !jsonLogger)
		},
		Short: "Sanitize the code for open source release",
	}

	c.AddCommand(
		NewRunCmd(),
	)

	c.PersistentFlags().AddGoFlagSet(flag.CommandLine)
	c.PersistentFlags().BoolVar(&jsonLogger, "json-logger", false, "If true use json logging.")
	c.PersistentFlags().StringVarP(&level, "log-level", "", "info", "Log level: error info or debug")
	return c
}

// NewRunCmd makes a new run command.
func NewRunCmd() *cobra.Command {
	var config string
	var input string
	var output string
	cmd := cobra.Command{
		Use:     "run -c <config path> -d <target dir>",
		Short:   "Sanitize the directory using the provided config",
		Example: `sanitize run -c sanitize.yaml -d .`,
		Run: func(cmd *cobra.Command, args []string) {
			err := func() error {
				log := zapr.NewLogger(zap.L())
				if input == "" {
					log.Info("input not provided; defaulting to working directory")
					wDir, err := os.Getwd()
					if err != nil {
						return errors.Wrapf(err, "failed to get current working directory")
					}
					input = wDir
				}

				b, err := os.ReadFile(config)
				if err != nil {
					return errors.Wrapf(err, "Failed to read config from path; %v", config)
				}
				config := &app.Sanitize{}
				if err := yaml.Unmarshal(b, config); err != nil {
					return errors.Wrapf(err, "Failed to unmarshal config from path; %v", config)
				}
				s, err := app.New(*config, log)
				if err != nil {
					return err
				}

				if err := s.Run(input, output); err != nil {
					return errors.Wrapf(err, "Failed to sanitize directory: %v; output %v", input, output)
				}
				return nil
			}()
			if err != nil {
				fmt.Printf("Failed to run sanitize; error:\n%v", err)
				os.Exit(1)
			}
		},
	}

	cmd.Flags().StringVarP(&config, "config", "c", "", "File containing the sanitization config")
	cmd.Flags().StringVarP(&input, "input", "i", "", "Input directory containing unsanitized code; defaults to current working directory")
	cmd.Flags().StringVarP(&output, "output", "o", "", "Output directory to write to.")

	if err := cmd.MarkFlagRequired("config"); err != nil {
		fmt.Printf("Failed to mark config as required; error %v", err)
	}
	if err := cmd.MarkFlagRequired("output"); err != nil {
		fmt.Printf("Failed to mark output as required; error %v", err)
	}
	return &cmd
}

func main() {
	if err := NewDefaultCommand().Execute(); err != nil {
		os.Exit(1)
	}
	os.Exit(0)
}
