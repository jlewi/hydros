package commands

import (
	"github.com/go-logr/zapr"
	"github.com/jlewi/hydros/pkg/app"
	"github.com/jlewi/hydros/pkg/hydros"
	"github.com/palantir/go-githubapp/githubapp"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"
	"go.uber.org/zap"
	"os"
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
	cmd := &cobra.Command{
		Use:   "serve",
		Short: "Run the hydros server",
		Run: func(cmd *cobra.Command, args []string) {
			log := zapr.NewLogger(zap.L())
			err := run(port, webhookSecret, privateKeySecret, githubAppID)
			if err != nil {
				log.Error(err, "Error running starling service")
				os.Exit(1)
			}
		},
	}

	cmd.Flags().IntVarP(&port, "port", "p", 8080, "Port to serve on")
	cmd.Flags().StringVarP(&webhookSecret, "webhook-secret", "", defaultWebhookSecret, "The URI of the HMAC secret used to sign GitHub webhooks. Can be a secret in GCP secret manager")
	cmd.Flags().StringVarP(&privateKeySecret, "private-key", "", "", "The URI of the GitHub App private key. Can be a secret in GCP secret manager")
	cmd.Flags().Int64VarP(&githubAppID, "app-id", "", hydros.HydrosGitHubAppID, "GitHubAppId.")
	return cmd
}

type App struct {
	IntegrationID int64  `yaml:"integration_id" json:"integrationId"`
	WebhookSecret string `yaml:"webhook_secret" json:"webhookSecret"`
	PrivateKey    string `yaml:"private_key" json:"privateKey"`
}

func run(port int, webhookSecret string, privateKeySecret string, githubAppID int64) error {
	hmacSecret, err := readSecret(webhookSecret)
	if err != nil {
		return errors.Wrapf(err, "Error reading webhook secret %s", webhookSecret)
	}

	privateKey, err := readSecret(privateKeySecret)
	if err != nil {
		return errors.Wrapf(err, "Error reading private key %s", privateKeySecret)
	}
	config := githubapp.Config{
		WebURL:   "https://github.com",
		V3APIURL: "https://api.github.com",
		V4APIURL: "https://api.github.com/graphql",
		App: App{
			IntegrationID: githubAppID,
			WebhookSecret: string(hmacSecret),
			PrivateKey:    string(privateKey),
		},
	}

	server, err := app.NewServer(port, config)
	if err != nil {
		return errors.Wrapf(err, "Failed to create server")
	}

	server.StartAndBlock()
	return nil
}
