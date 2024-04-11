package app

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/go-logr/zapr"
	"github.com/jlewi/hydros/api/v1alpha1"
	"github.com/jlewi/hydros/pkg/config"
	"github.com/jlewi/hydros/pkg/ecrutil"
	"github.com/jlewi/hydros/pkg/files"
	"github.com/jlewi/hydros/pkg/github"
	"github.com/jlewi/hydros/pkg/gitops"
	"github.com/jlewi/hydros/pkg/images"
	"github.com/jlewi/hydros/pkg/util"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"
	"go.uber.org/zap"
)

// App is a struct to hold values needed across all commands.
// Intent is to simplify initialization across commands.
type App struct {
	Config *config.Config
}

func NewApp() *App {
	return &App{}
}

// LoadConfig loads the config. It takes an optional command. The command allows values to be overwritten from
// the CLI.
func (a *App) LoadConfig(cmd *cobra.Command) error {
	// N.B. at this point we haven't configured any logging so zap just returns the default logger.
	// TODO(jeremy): Should we just initialize the logger without cfg and then reinitialize it after we've read the config?
	if err := config.InitViper(cmd); err != nil {
		return err
	}
	cfg := config.GetConfig()

	if problems := cfg.IsValid(); len(problems) > 0 {
		fmt.Fprintf(os.Stdout, "Invalid configuration; %s\n", strings.Join(problems, "\n"))
		return fmt.Errorf("invalid configuration; fix the problems and then try again")
	}
	a.Config = cfg

	return nil
}

func (a *App) SetupLogging() error {
	if a.Config == nil {
		return errors.New("Config is nil; call LoadConfig first")
	}
	cfg := a.Config
	// Use a non-json configuration configuration
	c := zap.NewDevelopmentConfig()

	// Use the keys used by cloud logging
	// https://cloud.google.com/logging/docs/structured-logging
	c.EncoderConfig.LevelKey = "severity"
	c.EncoderConfig.TimeKey = "time"
	c.EncoderConfig.MessageKey = "message"

	lvl := cfg.GetLogLevel()
	zapLvl := zap.NewAtomicLevel()

	if err := zapLvl.UnmarshalText([]byte(lvl)); err != nil {
		return errors.Wrapf(err, "Could not convert level %v to ZapLevel", lvl)
	}

	c.Level = zapLvl
	newLogger, err := c.Build()
	if err != nil {
		panic(fmt.Sprintf("Failed to initialize zap logger; error %v", err))
	}

	zap.ReplaceGlobals(newLogger)

	return nil
}

// ApplyPaths applies the resources in the specified paths.
// Paths can be files or directories.
func (a *App) ApplyPaths(ctx context.Context, paths []string, period time.Duration, force bool) error {
	log := util.LogFromContext(ctx)

	if a.Config.GitHub == nil {
		return errors.New("GitHub configuration is missing; You need to run hydros config set github.appID and hydros config set github.privateKey")
	}
	if a.Config.GitHub.PrivateKey == "" {
		return errors.New("GitHub configuration is missing github.privateKey; You need to run hydros config set github.privateKey")
	}

	if a.Config.GitHub.AppID <= 0 {
		return errors.New("GitHub configuration is missing github.appID; You need to run hydros config set github.appID")
	}

	for _, resourcePath := range paths {
		newPaths, err := util.FindYamlFiles(resourcePath)
		if err != nil {
			log.Error(err, "Failed to find YAML files", "path", resourcePath)
			return err
		}

		paths = append(paths, newPaths...)
	}

	syncNames := map[string]string{}

	for _, path := range paths {
		err := a.apply(ctx, path, syncNames, period, force)
		if err != nil {
			log.Error(err, "Apply failed", "path", path)
		}
	}

	if len(syncNames) == 0 {
		err := fmt.Errorf("No hydros resources found")
		log.Error(err, "No hydros resources found", "paths", paths)
		return err
	}
	// Wait for ever
	if period > 0 {
		select {}
	}
	return nil
}

func (a *App) apply(ctx context.Context, path string, syncNames map[string]string, period time.Duration, force bool) error {
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

			secret, err := files.Read(a.Config.GitHub.PrivateKey)
			if err != nil {
				return errors.Wrapf(err, "Could not read file: %v", a.Config.GitHub.PrivateKey)
			}
			manager, err := github.NewTransportManager(int64(a.Config.GitHub.AppID), secret, log)
			if err != nil {
				log.Error(err, "TransportManager creation failed")
				return err
			}

			syncer, err := gitops.NewSyncer(manifestSync, manager, gitops.SyncWithWorkDir(a.Config.GetWorkDir()), gitops.SyncWithLogger(log))
			if err != nil {
				log.Error(err, "Failed to create syncer")
				allErrors.AddCause(err)
				continue
			}

			if period > 0 {
				go syncer.RunPeriodically(period)
			} else {
				if err := syncer.RunOnce(force); err != nil {
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

			c, err := gitops.NewRepoController(repo, a.Config.GetWorkDir())
			if err != nil {
				return err
			}

			if period > 0 {
				go c.RunPeriodically(context.Background(), period)
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

				if period == 0 {
					break
				}

				log.Info("Sleep", "duration", period)
				time.Sleep(period)
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
