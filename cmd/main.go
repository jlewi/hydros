package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	githubCmds "github.com/jlewi/hydros/cmd/github"
	"github.com/jlewi/hydros/pkg/github"
	"github.com/jlewi/hydros/pkg/hydros"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/jlewi/hydros/api/v1alpha1"
	"github.com/jlewi/hydros/pkg/ecrutil"
	"github.com/jlewi/hydros/pkg/gitops"
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

type applyOptions struct {
	workDir     string
	secret      string
	githubAppID int
	period      time.Duration
	force       bool
}

type tagOptions struct {
	skaffold string
	tags     string
	image    string
}

var (
	log      logr.Logger
	gOptions = globalOptions{}
	aOptions = applyOptions{}
	tOptions = tagOptions{}

	rootCmd = &cobra.Command{
		Short: "hydros is a tool to hydrate manifests",
		PersistentPreRun: func(cmd *cobra.Command, args []string) {
			log = util.SetupLogger(gOptions.level, gOptions.devLogger)
		},
	}

	applyCmd = &cobra.Command{
		Use:   "apply <resource.yaml> <resourceDir> <resource.yaml> ...",
		Short: "Apply the specified resource.",
		Run: func(cmd *cobra.Command, args []string) {
			if len(args) == 0 {
				log.Info("apply takes at least one argument which should be the file or directory YAML to apply.")
				return
			}

			paths := []string{}

			for _, resourcePath := range args {
				newPaths, err := util.FindYamlFiles(resourcePath)
				if err != nil {
					log.Error(err, "Failed to find YAML files", "path", resourcePath)
					return
				}

				paths = append(paths, newPaths...)
			}

			syncNames := map[string]string{}

			for _, path := range paths {
				err := apply(aOptions, path, syncNames)
				if err != nil {
					log.Error(err, "Apply failed", "path", path)
				}
			}

			if len(syncNames) == 0 {
				err := fmt.Errorf("No ManifestSyncs found")
				log.Error(err, "No ManifestSync objects found", "paths", paths)
				return
			}
			// Wait for ever
			if aOptions.period > 0 {
				select {}
			}
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
				// TOOD(jeremy): We should use github.com/GoogleContainerTools/skaffold/cmd/skaffold/app/flags
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
	rootCmd.AddCommand(applyCmd)
	rootCmd.AddCommand(tagCmd)
	rootCmd.AddCommand(newVersionCmd(os.Stdout))
	rootCmd.AddCommand(githubCmds.NewAppTokenCmd(os.Stdout, &gOptions.level, &gOptions.devLogger))

	rootCmd.PersistentFlags().BoolVar(&gOptions.devLogger, "dev-logger", false, "If true configure the logger for development; i.e. non-json output")
	rootCmd.PersistentFlags().StringVarP(&gOptions.level, "log-level", "", "info", "Log level: error info or debug")

	applyCmd.Flags().StringVarP(&aOptions.workDir, "work-dir", "", "", "Directory where repos should be checked out")
	applyCmd.Flags().StringVarP(&aOptions.secret, "private-key", "", "", "Path to the file containing the secret for the GitHub App to Authenticate as.")
	applyCmd.Flags().IntVarP(&aOptions.githubAppID, "appId", "", hydros.HydrosGitHubAppID, "GitHubAppId.")
	applyCmd.Flags().DurationVarP(&aOptions.period, "period", "p", 0*time.Minute, "The period with which to reapply. If zero run once and exit.")
	applyCmd.Flags().BoolVarP(&aOptions.force, "force", "", false, "Force a sync even if one isn't needed.")

	_ = applyCmd.MarkFlagRequired("secret")

	tagCmd.Flags().StringVarP(&tOptions.skaffold, "skaffold", "", "", "image.json as output by skaffold")
	tagCmd.Flags().StringVarP(&tOptions.image, "image", "", "", "Url of the image to tag. Can be specified rather than skaffold.")

	tagCmd.Flags().StringVarP(&tOptions.tags, "tags", "", "", "List of tags to apply to the image")

	_ = tagCmd.MarkFlagRequired("tags")
}

func newVersionCmd(w io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "version",
		Short:   "Return version",
		Example: `kap version`,
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Fprintf(w, "hydros %s, commit %s, built at %s by %s", version, commit, date, builtBy)
		},
	}
	return cmd
}

func apply(a applyOptions, path string, syncNames map[string]string) error {
	log.Info("Reading file", "path", path)
	rNodes, err := util.ReadYaml(path)
	if err != nil {
		return err
	}

	allErrors := &util.ListOfErrors{
		Causes: []error{},
	}

	for _, n := range rNodes {
		m, err := n.GetMeta()
		if err != nil {
			log.Error(err, "Failed to get metadata", "n", n)
			continue
		}
		log.Info("Read resource", "meta", m)
		if m.Kind == v1alpha1.ManifestSyncKind {
			manifestSync := &v1alpha1.ManifestSync{}
			err := n.Document().Decode(manifestSync)
			if err != nil {
				log.Error(err, "Failed to decode ManifestSync")
				continue
			}

			name := manifestSync.Metadata.Name
			if f, ok := syncNames[name]; ok {
				err := fmt.Errorf("Multiple ManifestSync with name %v", name)
				log.Error(err, "Multiple ManifestSync objects found with the same name", "name", name, "using", f, "new", path)
				allErrors.AddCause(err)
				continue
			}
			syncNames[name] = path

			manager, err := github.NewTransportManager(int64(aOptions.githubAppID), aOptions.secret, log)
			if err != nil {
				log.Error(err, "TransportManager creation failed")
				return err
			}

			syncer, err := gitops.NewSyncer(manifestSync, manager, gitops.SyncWithWorkDir(aOptions.workDir), gitops.SyncWithLogger(log))
			if err != nil {
				log.Error(err, "Failed to create syncer")
				allErrors.AddCause(err)
				continue
			}

			if aOptions.period > 0 {
				go syncer.RunPeriodically(aOptions.period)
			} else {
				if err := syncer.RunOnce(aOptions.force); err != nil {
					log.Error(err, "Failed to run Sync")
					allErrors.AddCause(err)
				}
			}

		} else if m.Kind == v1alpha1.EcrPolicySyncKind {
			syncNames[m.Name] = path
			region := "us-west-2"
			log.Info("Creating a default AWS session", "region", region)
			sess, err := session.NewSession(&aws.Config{
				Region: aws.String(region),
			})
			if err != nil {
				log.Error(err, "Failed to create AWS session")
				allErrors.AddCause(err)
				continue
			}

			c, err := ecrutil.NewEcrPolicySyncController(sess, ecrutil.EcrPolicySyncWithLogger(log))
			if err != nil {
				log.Error(err, "Failed to create EcrPolicySyncController")
				allErrors.AddCause(err)
				continue
			}

			for {
				if err := c.Apply(n); err != nil {
					log.Error(err, "Failed to create apply resource", "name", m.Name)
					allErrors.AddCause(err)
					continue
				}

				if aOptions.period == 0 {
					break
				}

				log.Info("Sleep", "duration", aOptions.period)
				time.Sleep(aOptions.period)
			}
		}
	}

	if len(allErrors.Causes) == 0 {
		return nil
	}
	allErrors.Final = fmt.Errorf("failed to apply one or more resources")
	return allErrors
}

func main() {
	err := rootCmd.Execute()
	if err != nil {
		log.Error(err, "main failed")
	}
}
