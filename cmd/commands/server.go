package commands

import (
	"github.com/go-logr/zapr"
	"github.com/jlewi/hydros/pkg/app"
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

	cmd := &cobra.Command{
		Use:   "serve",
		Short: "Run the hydros server",
		Run: func(cmd *cobra.Command, args []string) {
			log := zapr.NewLogger(zap.L())
			err := run(port, webhookSecret)
			if err != nil {
				log.Error(err, "Error running starling service")
				os.Exit(1)
			}
		},
	}

	cmd.Flags().IntVarP(&port, "port", "p", 8080, "Port to serve on")
	cmd.Flags().StringVarP(&webhookSecret, "webhook-secret", "", defaultWebhookSecret, "The HMAC secret used to sign GitHub webhooks. Can be a secret in GCP secret manager")
	return cmd
}

func run(port int, webhookSecret string) error {
	//hmacSecret, err := readSecret(webhookSecret)
	//if err != nil {
	//	return err
	//}

	//handler := &health.AnnotateHandler{Manager: m}
	//
	//dispatcher := githubapp.NewEventDispatcher([]githubapp.EventHandler{handler}, string(hmacSecret), githubapp.WithErrorCallback(health.LogErrorCallback))

	server, err := app.NewServer(port, nil)
	if err != nil {
		return errors.Wrapf(err, "Failed to create server")
	}

	server.StartAndBlock()
	return nil
}

//func (a *AnnotateFlags) getTransportManager() (*github.TransportManager, error) {
//	log := zapr.NewLogger(zap.L())
//
//	secretByte, err := readSecret(a.PrivateKey)
//	if err != nil {
//		return nil, err
//	}
//	return github.NewTransportManager(a.AppID, secretByte, log)
//}
//
//func readSecret(path string) ([]byte, error) {
//	f := &files.Factory{}
//	h, err := f.Get(path)
//	if err != nil {
//		return nil, err
//	}
//	r, err := h.NewReader(path)
//	if err != nil {
//		return nil, err
//	}
//	return io.ReadAll(r)
//}
