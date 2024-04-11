package main

import (
	"encoding/json"
	"fmt"
	"github.com/jlewi/hydros/pkg/config"
	"io"
	"os"
	"strings"

	"github.com/jlewi/hydros/cmd/commands"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	githubCmds "github.com/jlewi/hydros/cmd/github"
	"github.com/jlewi/hydros/pkg/ecrutil"
	"github.com/jlewi/hydros/pkg/util"

	"github.com/go-logr/logr"
	"github.com/spf13/cobra"
)

// N.B these will get set by goreleaser
// https://goreleaser.com/cookbooks/using-main.version
var (
	version = "dev"
	commit  = "none"
	date    = "unknown"
	builtBy = "unknown"
)

type globalOptions struct {
	devLogger bool
	level     string
}

type tagOptions struct {
	skaffold string
	tags     string
	image    string
}

// TODO(jeremy): We should finish refactoring these commands to use the pattern of having functions create the commands
// and not doing it init. And also having one file per command.
var (
	log      logr.Logger
	gOptions = globalOptions{}
	tOptions = tagOptions{}

	rootCmd = &cobra.Command{
		Short: "hydros is a tool to hydrate manifests",
		PersistentPreRun: func(cmd *cobra.Command, args []string) {
			log = util.SetupLogger(gOptions.level, gOptions.devLogger)
		},
	}

	tagCmd = &cobra.Command{
		Use:   "tagImage",
		Short: "Apply the specified tags to an image. Takes as input the image.json file output by skaffold.",
		Run: func(cmd *cobra.Command, args []string) {
			region := "us-west-2"
			log.Info("Creating a default AWS session", "region", region)
			sess, err := session.NewSession(&aws.Config{
				Region: aws.String(region),
			})
			if err != nil {
				log.Error(err, "Failed to create AWS session")
				return
			}

			if tOptions.skaffold == "" && tOptions.image == "" {
				log.Error(err, "Exactly one of --image and --skaffold must be set")
				return
			}

			tags := strings.Split(tOptions.tags, ",")
			if tOptions.image != "" {
				err = ecrutil.AddTagsToImage(sess, tOptions.image, tags)
				if err != nil {
					log.Error(err, "Failed to add tags to image", "image", tOptions.image, "tags", tags)
				}
			} else {
				// TOOD(jeremy): We should use github.com/GoogleContainerTools/skaffold/cmd/skaffold/ghapp/flags
				type Build struct {
					ImageName string `yaml:"imageName,omitempty"`
					Tag       string `yaml:"tag,omitempty"`
				}
				type ImageInfo struct {
					Builds []Build `yaml:"builds,omitempty"`
				}

				info := &ImageInfo{}

				f, err := os.Open(tOptions.skaffold)
				if err != nil {
					log.Error(err, "Failed to open image file", "file", tOptions.skaffold)
					return
				}
				d := json.NewDecoder(f)

				if err := d.Decode(info); err != nil {
					log.Error(err, "Failed to decode skaffold output")
					return
				}

				for _, b := range info.Builds {
					err := ecrutil.AddTagsToImage(sess, b.Tag, tags)
					if err != nil {
						log.Error(err, "Failed to add tags to image", "image", b.Tag, tags)
					}
				}
			}
		},
	}
)

func init() {
	rootCmd.AddCommand(commands.NewApplyCmd())
	rootCmd.AddCommand(tagCmd)
	rootCmd.AddCommand(newVersionCmd(os.Stdout))
	rootCmd.AddCommand(githubCmds.NewAppTokenCmd(os.Stdout, &gOptions.level, &gOptions.devLogger))
	rootCmd.AddCommand(commands.NewBuildCmd())
	rootCmd.AddCommand(commands.NewTakeOverCmd())
	rootCmd.AddCommand(commands.NewHydrosServerCmd())
	rootCmd.AddCommand(commands.NewCloneCmd())
	rootCmd.AddCommand(commands.NewVersionCmd("hydros", os.Stdout))
	rootCmd.AddCommand(commands.NewConfigCmd())

	rootCmd.PersistentFlags().BoolVar(&gOptions.devLogger, "dev-logger", false, "If true configure the logger for development; i.e. non-json output")
	rootCmd.PersistentFlags().StringVarP(&gOptions.level, config.LevelFlagName, "", "info", "Log level: error info or debug")

	var cfgFile string
	rootCmd.PersistentFlags().StringVar(&cfgFile, config.ConfigFlagName, "", fmt.Sprintf("config file (default is $HOME/.%s/config.yaml)", config.AppName))

	tagCmd.Flags().StringVarP(&tOptions.skaffold, "skaffold", "", "", "image.json as output by skaffold")
	tagCmd.Flags().StringVarP(&tOptions.image, "image", "", "", "Url of the image to tag. Can be specified rather than skaffold.")

	tagCmd.Flags().StringVarP(&tOptions.tags, "tags", "", "", "List of tags to apply to the image")

	_ = tagCmd.MarkFlagRequired("tags")
}

func newVersionCmd(w io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "version",
		Short:   "Return version",
		Example: `hydros version`,
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Fprintf(w, "hydros %s, commit %s, built at %s by %s", version, commit, date, builtBy)
		},
	}
	return cmd
}

func main() {
	err := rootCmd.Execute()
	if err != nil {
		log.Error(err, "main failed")
	}
}
