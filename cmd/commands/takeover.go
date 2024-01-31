package commands

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/go-logr/zapr"
	"github.com/jlewi/hydros/api/v1alpha1"
	"github.com/jlewi/hydros/pkg/files"
	"github.com/jlewi/hydros/pkg/github"
	"github.com/jlewi/hydros/pkg/gitops"
	"github.com/jlewi/hydros/pkg/hydros"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"
	"go.uber.org/zap"
	"gopkg.in/yaml.v3"
)

const (
	maxPause = 12 * time.Hour
)

type TakeOverArgs struct {
	WorkDir     string
	Secret      string
	GithubAppID int64
	Force       bool
	File        string
	KeyFile     string
	RepoDir     string
	Pause       time.Duration
}

func NewTakeOverCmd() *cobra.Command {
	opts := &TakeOverArgs{}
	cmd := &cobra.Command{
		Use:   "takeover -f <resource.yaml>",
		Short: "Take over the dev environment by applying the specified configuration.",
		Run: func(cmd *cobra.Command, args []string) {
			if err := TakeOver(opts); err != nil {
				fmt.Printf("takeover failed; error %+v\n", err)
			}
		},
	}

	cmd.Flags().StringVarP(&opts.WorkDir, "work-dir", "", "", "Directory where repos should be checked out")
	cmd.Flags().StringVarP(&opts.Secret, "private-key", "", "", "Path to the file containing the secret for the GitHub App to Authenticate as.")
	cmd.Flags().Int64VarP(&opts.GithubAppID, "app-id", "", hydros.HydrosGitHubAppID, "GitHubAppId.")
	cmd.Flags().BoolVarP(&opts.Force, "force", "", false, "Force a sync even if one isn't needed.")
	cmd.Flags().StringVarP(&opts.File, "file", "", "", "The file containing the configuration to apply.")
	cmd.Flags().StringVarP(&opts.KeyFile, "ssh-key-file", "", "", "(Optional) Path of PEM file containing ssh key used to push current changes. If blank will try to find key in ${HOME}/.ssh.")
	cmd.Flags().StringVarP(&opts.RepoDir, "repo-dir", "", "", "(Optional) Directory containing the source repo that should be pushed. If blank it is inferred based on the path of the --file argument")
	cmd.Flags().DurationVarP(&opts.Pause, "pause", "", 2*time.Hour, "How long to pause regular syncs. Maximum is 2 hours")
	cmd.MarkFlagRequired("file")
	cmd.MarkFlagRequired("private-key")
	return cmd
}

func TakeOver(args *TakeOverArgs) error {
	log := zapr.NewLogger(zap.L())

	if args.Pause > maxPause {
		return errors.Errorf("Pause duration is too long; maximum is %v", maxPause)
	}

	secret, err := files.Read(args.Secret)
	if err != nil {
		return errors.Wrapf(err, "Could not read file: %v", args.Secret)
	}
	manager, err := github.NewTransportManager(int64(args.GithubAppID), secret, log)
	if err != nil {
		log.Error(err, "TransportManager creation failed")
		return err
	}

	manifestPath, err := filepath.Abs(args.File)
	if err != nil {
		return errors.Wrapf(err, "Failed to get absolute path for %v", args.File)
	}

	log.Info("Resolved manifest path", "manifestPath", manifestPath)

	f, err := os.Open(manifestPath)
	if err != nil {
		return errors.Wrapf(err, "Failed to open file: %v", manifestPath)
	}

	d := yaml.NewDecoder(f)

	m := &v1alpha1.ManifestSync{}

	if err := d.Decode(m); err != nil {
		return errors.Wrapf(err, "Failed to decode ManifestSync from file %v", manifestPath)
	}

	tEnd := time.Now().Add(args.Pause)

	k8sTime := metav1.NewTime(tEnd)
	v, err := k8sTime.MarshalJSON()
	if err != nil {
		return errors.Wrapf(err, "Failed to marshal time %v", tEnd)
	}
	m.Metadata.Annotations = map[string]string{
		// We need to mark it as a takeover otherwise we won't override pauses.
		v1alpha1.TakeoverAnnotation: "true",
		v1alpha1.PauseAnnotation:    string(v),
	}

	log.Info("Pausing automatic syncs", "pauseUntil", string(v))
	syncer, err := gitops.NewSyncer(m, manager, gitops.SyncWithWorkDir(args.WorkDir), gitops.SyncWithLogger(log))
	if err != nil {
		return err
	}

	if args.RepoDir == "" {
		args.RepoDir = filepath.Dir(manifestPath)
		log.Info("RepoDir is using default", "repoDir", args.RepoDir)
	}

	if err := syncer.PushLocal(args.RepoDir, args.KeyFile); err != nil {
		return err
	}

	if err := syncer.RunOnce(args.Force); err != nil {
		return err
	}

	return nil
}
