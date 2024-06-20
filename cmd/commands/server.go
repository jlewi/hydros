package commands

import (
	"os"
	"time"

	"github.com/jlewi/monogo/files"

	"github.com/go-logr/zapr"
	"github.com/gregjones/httpcache"
	"github.com/jlewi/hydros/pkg/ghapp"
	hGithub "github.com/jlewi/hydros/pkg/github"
	"github.com/jlewi/hydros/pkg/hydros"
	"github.com/palantir/go-githubapp/githubapp"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"
	"go.uber.org/zap"
)

const (
	defaultWebhookSecret = "gcpSecretManager:///projects/chat-lewi/secrets/hydros-webhook/versions/latest"
)

// NewHydrosServerCmd creates a command to run the server
func NewHydrosServerCmd() *cobra.Command {
	var port int
	var webhookSecret string
	var privateKeySecret string
	var githubAppID int64
	var workDir string
	var numWorkers int
	var baseHREF string
	cmd := &cobra.Command{
		Use:   "serve",
		Short: "Run the hydros server",
		Run: func(cmd *cobra.Command, args []string) {
			log := zapr.NewLogger(zap.L())
			err := run(baseHREF, port, webhookSecret, privateKeySecret, githubAppID, workDir, numWorkers)
			if err != nil {
				log.Error(err, "Error running hydros")
				os.Exit(1)
			}
		},
	}

	cmd.Flags().IntVarP(&port, "port", "p", 8080, "Port to serve on")
	cmd.Flags().StringVarP(&baseHREF, "base-href", "", "/hydros/", "The base prefix for all URLs should end with a slash or empty string to use no prefix")
	cmd.Flags().StringVarP(&webhookSecret, "webhook-secret", "", defaultWebhookSecret, "The URI of the HMAC secret used to sign GitHub webhooks. Can be a secret in GCP secret manager")
	cmd.Flags().StringVarP(&privateKeySecret, "private-key", "", "", "The URI of the GitHub App private key. Can be a secret in GCP secret manager")
	cmd.Flags().Int64VarP(&githubAppID, "ghapp-id", "", hydros.HydrosGitHubAppID, "GitHubAppId.")
	cmd.Flags().StringVarP(&workDir, "work-dir", "", "", "(Optional) work directory where repositories should be checked out. Leave blank to use a temporary directory.")
	cmd.Flags().IntVarP(&numWorkers, "num-workers", "", 10, "Number of workers to handle events.")
	return cmd
}

func run(baseHREF string, port int, webhookSecret string, privateKeySecret string, githubAppID int64, workDir string, numWorkers int) error {
	log := zapr.NewLogger(zap.L())
	config, err := ghapp.BuildConfig(githubAppID, webhookSecret, privateKeySecret)
	if err != nil {
		return errors.Wrapf(err, "Error building config")
	}

	cc, err := githubapp.NewDefaultCachingClientCreator(
		*config,
		githubapp.WithClientUserAgent(ghapp.UserAgent),
		githubapp.WithClientTimeout(3*time.Second),
		githubapp.WithClientCaching(false, func() httpcache.Cache { return httpcache.NewMemoryCache() }),
	)

	if err != nil {
		return errors.Wrapf(err, "Error creating client creator")
	}

	secret, err := files.Read(privateKeySecret)
	if err != nil {
		return errors.Wrapf(err, "Could not read secret: %v", privateKeySecret)
	}
	transports, err := hGithub.NewTransportManager(githubAppID, secret, log)
	if err != nil {
		return err
	}

	handler, err := ghapp.NewHandler(cc, transports, workDir, 10)
	if err != nil {
		return err
	}

	server, err := ghapp.NewServer(baseHREF, port, *config, handler)
	if err != nil {
		return errors.Wrapf(err, "Failed to create server")
	}

	server.StartAndBlock()
	return nil
}
