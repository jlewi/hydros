package app

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"go.uber.org/zap/zapcore"

	"github.com/jlewi/hydros/pkg/controllers"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/go-logr/zapr"
	"github.com/jlewi/hydros/api/v1alpha1"
	"github.com/jlewi/hydros/pkg/config"
	"github.com/jlewi/hydros/pkg/ecrutil"
	"github.com/jlewi/hydros/pkg/github"
	"github.com/jlewi/hydros/pkg/gitops"
	"github.com/jlewi/hydros/pkg/images"
	"github.com/jlewi/hydros/pkg/util"
	"github.com/jlewi/monogo/files"
	"github.com/jlewi/monogo/gcp/logging"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"
	"go.uber.org/zap"
)

// App is a struct to hold values needed across all commands.
// Intent is to simplify initialization across commands.
type App struct {
	Config     *config.Config
	Registry   *controllers.Registry
	logClosers []logCloser
}

type logCloser func()

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

	cores := make([]zapcore.Core, 0, 2)

	consolePaths := make([]string, 0, 1)
	jsonPaths := make([]string, 0, 1)

	for _, sink := range a.Config.Logging.Sinks {
		if sink.JSON {
			jsonPaths = append(jsonPaths, sink.Path)
		} else {
			consolePaths = append(consolePaths, sink.Path)
		}

		project, logName, isLog := logging.ParseURI(sink.Path)
		if isLog {
			if err := logging.RegisterSink(project, logName, nil); err != nil {
				return err
			}
		}
	}

	if len(consolePaths) == 0 && len(jsonPaths) == 0 {
		// If no sinks are specified we default to console logging.
		consolePaths = []string{"stderr"}
	}

	if len(consolePaths) > 0 {
		consoleCore, err := a.createCoreForConsole(consolePaths)
		if err != nil {
			return errors.Wrap(err, "Could not create core logger for console")
		}
		cores = append(cores, consoleCore)
	}

	if len(jsonPaths) > 0 {
		jsonCore, err := a.createJSONCoreLogger(jsonPaths)
		if err != nil {
			return errors.Wrap(err, "Could not create core logger for JSON paths")
		}
		cores = append(cores, jsonCore)
	}

	// Create a multi-core logger with different encodings
	core := zapcore.NewTee(cores...)

	// Create the logger
	newLogger := zap.New(core)
	// Record the caller of the log message
	newLogger = newLogger.WithOptions(zap.AddCaller())
	zap.ReplaceGlobals(newLogger)

	return nil
}

func (a *App) createCoreForConsole(paths []string) (zapcore.Core, error) {
	// Configure encoder for non-JSON format (console-friendly)
	c := zap.NewDevelopmentEncoderConfig()

	// Use the keys used by cloud logging
	// https://cloud.google.com/logging/docs/structured-logging
	c.LevelKey = logging.SeverityField
	c.TimeKey = logging.TimeField
	c.MessageKey = logging.MessageField
	// We attach the function key to the logs because that is useful for identifying the function that generated the log.
	c.FunctionKey = "function"

	lvl := a.Config.GetLogLevel()
	zapLvl := zap.NewAtomicLevel()

	if err := zapLvl.UnmarshalText([]byte(lvl)); err != nil {
		return nil, errors.Wrapf(err, "Could not convert level %v to ZapLevel", lvl)
	}

	oFile, closer, err := zap.Open(paths...)
	if err != nil {
		return nil, errors.Wrapf(err, "could not create writer for stderr")
	}
	if a.logClosers == nil {
		a.logClosers = []logCloser{}
	}
	a.logClosers = append(a.logClosers, closer)

	encoder := zapcore.NewConsoleEncoder(c)
	core := zapcore.NewCore(encoder, zapcore.AddSync(oFile), zapLvl)
	return core, nil
}

// createCoreLoggerForFiles creates a core logger uses json. This is useful if you want to send the logs
// directly to Cloud Logging.
func (a *App) createJSONCoreLogger(paths []string) (zapcore.Core, error) {
	// Configure encoder for JSON format
	c := zap.NewProductionEncoderConfig()
	// Use the keys used by cloud logging
	// https://cloud.google.com/logging/docs/structured-logging
	c.LevelKey = logging.SeverityField
	c.TimeKey = logging.TimeField
	c.MessageKey = logging.MessageField

	// We attach the function key to the logs because that is useful for identifying the function that generated the log.
	c.FunctionKey = "function"

	jsonEncoder := zapcore.NewJSONEncoder(c)

	oFile, closer, err := zap.Open(paths...)
	if err != nil {
		return nil, errors.Wrapf(err, "could not open log paths %v", paths)
	}
	if a.logClosers == nil {
		a.logClosers = []logCloser{}
	}
	a.logClosers = append(a.logClosers, closer)

	zapLvl := zap.NewAtomicLevel()

	if err := zapLvl.UnmarshalText([]byte(a.Config.GetLogLevel())); err != nil {
		return nil, errors.Wrapf(err, "Could not convert level %v to ZapLevel", a.Config.GetLogLevel())
	}

	// Force log level to be at least info. Because info is the level at which we capture the logs we need for
	// tracing.
	if zapLvl.Level() > zapcore.InfoLevel {
		zapLvl.SetLevel(zapcore.InfoLevel)
	}

	core := zapcore.NewCore(jsonEncoder, zapcore.AddSync(oFile), zapLvl)

	return core, nil
}

// SetupRegistry sets up the registry with a list of registered controllers
func (a *App) SetupRegistry() error {
	if a.Config == nil {
		return errors.New("Config is nil; call LoadConfig first")
	}
	a.Registry = &controllers.Registry{}

	// Register controllers
	image, err := images.NewController()
	if err != nil {
		return err
	}
	if err := a.Registry.Register(v1alpha1.ImageGVK, image); err != nil {
		return err
	}

	replicator, err := images.NewReplicator()
	if err != nil {
		return err
	}
	if err := a.Registry.Register(v1alpha1.ReplicatedImageGVK, replicator); err != nil {
		return err
	}

	releaser, err := github.NewReleaser(*a.Config)
	if err != nil {
		return err
	}

	if err := a.Registry.Register(v1alpha1.GitHubReleaserGVK, releaser); err != nil {
		return err
	}

	return nil
}

// ApplyPaths applies the resources in the specified paths.
// Paths can be files or directories.
func (a *App) ApplyPaths(ctx context.Context, inPaths []string, period time.Duration, force bool) error {
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

	paths := make([]string, 0, len(inPaths))
	for _, resourcePath := range inPaths {
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
	if a.Registry == nil {
		return errors.New("Registry is nil; call SetupRegistry first")
	}

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
		switch m.Kind {
		case v1alpha1.ManifestSyncKind:
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
		case v1alpha1.RepoGVK.Kind:
			syncNames[m.Name] = path
			repo := &v1alpha1.RepoConfig{}
			if err := n.YNode().Decode(&repo); err != nil {
				log.Error(err, "Failed to decode RepoConfig")
				allErrors.AddCause(err)
				continue
			}

			c, err := gitops.NewRepoController(*a.Config, a.Registry, repo)
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
		case v1alpha1.EcrPolicySyncKind:
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
		default:
			syncNames[m.Name] = path
			// Going forward we should be using the registry
			gvk := schema.FromAPIVersionAndKind(m.APIVersion, m.Kind)
			controller, err := a.Registry.GetController(gvk)
			if err != nil {
				log.Error(err, "Unsupported kind", "gvk", gvk)
				allErrors.AddCause(err)
				continue
			}

			if err := controller.ReconcileNode(ctx, n); err != nil {
				log.Error(err, "Failed to reconcile resource", "name", m.Name, "namespace", m.Namespace, "gvk", gvk)
				allErrors.AddCause(err)
			}
		}
	}

	if len(allErrors.Causes) == 0 {
		return nil
	}
	allErrors.Final = fmt.Errorf("failed to apply one or more resources")
	return allErrors
}

// Shutdown the application.
func (a *App) Shutdown() error {
	l := zap.L()
	log := zapr.NewLogger(l)

	log.Info("Shutting down the application")
	// Flush the logs
	for _, closer := range a.logClosers {
		closer()
	}
	return nil
}
