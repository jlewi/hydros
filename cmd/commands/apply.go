package commands

import (
	"context"
	"fmt"
	"time"

	"github.com/jlewi/hydros/pkg/images"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/go-logr/zapr"
	"github.com/jlewi/hydros/api/v1alpha1"
	"github.com/jlewi/hydros/pkg/ecrutil"
	"github.com/jlewi/hydros/pkg/files"
	"github.com/jlewi/hydros/pkg/github"
	"github.com/jlewi/hydros/pkg/gitops"
	"github.com/jlewi/hydros/pkg/hydros"
	"github.com/jlewi/hydros/pkg/util"
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
			log := zapr.NewLogger(zap.L())
			if len(args) == 0 {
				log.Info("apply takes at least one argument which should be the file or directory YAML to apply.")
				return
			}
			logVersion()
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
				err := fmt.Errorf("No hydros resources found")
				log.Error(err, "No hydros resources found", "paths", paths)
				return
			}
			// Wait for ever
			if aOptions.period > 0 {
				select {}
			}
		},
	}

	applyCmd.Flags().StringVarP(&aOptions.workDir, "work-dir", "", "", "Directory where repos should be checked out")
	applyCmd.Flags().StringVarP(&aOptions.secret, "private-key", "", "", "Path to the file containing the secret for the GitHub App to Authenticate as.")
	applyCmd.Flags().IntVarP(&aOptions.githubAppID, "appId", "", hydros.HydrosGitHubAppID, "GitHubAppId.")
	applyCmd.Flags().DurationVarP(&aOptions.period, "period", "p", 0*time.Minute, "The period with which to reapply. If zero run once and exit.")
	applyCmd.Flags().BoolVarP(&aOptions.force, "force", "", false, "Force a sync even if one isn't needed.")

	_ = applyCmd.MarkFlagRequired("secret")

	return applyCmd
}

func apply(a applyOptions, path string, syncNames map[string]string) error {
	log := zapr.NewLogger(zap.L())
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

			secret, err := files.Read(a.secret)
			if err != nil {
				return errors.Wrapf(err, "Could not read file: %v", a.secret)
			}
			manager, err := github.NewTransportManager(int64(a.githubAppID), secret, log)
			if err != nil {
				log.Error(err, "TransportManager creation failed")
				return err
			}

			syncer, err := gitops.NewSyncer(manifestSync, manager, gitops.SyncWithWorkDir(a.workDir), gitops.SyncWithLogger(log))
			if err != nil {
				log.Error(err, "Failed to create syncer")
				allErrors.AddCause(err)
				continue
			}

			if a.period > 0 {
				go syncer.RunPeriodically(a.period)
			} else {
				if err := syncer.RunOnce(a.force); err != nil {
					log.Error(err, "Failed to run Sync")
					allErrors.AddCause(err)
				}
			}

		} else if m.Kind == v1alpha1.RepoGVK.Kind {
			syncNames[m.Name] = path
			repo := &v1alpha1.RepoConfig{}
			if err := n.YNode().Decode(&repo); err != nil {
				log.Error(err, "Failed to decode RepoConfig")
				allErrors.AddCause(err)
				continue
			}

			c, err := gitops.NewRepoController(repo, a.workDir)
			if err != nil {
				return err
			}

			if a.period > 0 {
				go c.RunPeriodically(context.Background(), a.period)
			} else {
				if err := c.Reconcile(context.Background()); err != nil {
					return err
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

				if a.period == 0 {
					break
				}

				log.Info("Sleep", "duration", a.period)
				time.Sleep(a.period)
			}
		} else if m.Kind == v1alpha1.ReplicatedImageGVK.Kind {
			syncNames[m.Name] = path
			replicated := &v1alpha1.ReplicatedImage{}
			if err := n.YNode().Decode(&replicated); err != nil {
				log.Error(err, "Failed to decode ReplicatedImage")
				allErrors.AddCause(err)
				continue
			}

			r, err := images.NewReplicator()
			if err != nil {
				return err
			}

			if err := r.Reconcile(context.Background(), replicated); err != nil {
				return err
			}

		} else {
			err := fmt.Errorf("Unsupported kind: %v", m.Kind)
			log.Error(err, "Unsupported kind", "kind", m.Kind)
			allErrors.AddCause(err)
		}
	}

	if len(allErrors.Causes) == 0 {
		return nil
	}
	allErrors.Final = fmt.Errorf("failed to apply one or more resources")
	return allErrors
}
