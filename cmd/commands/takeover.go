package commands

import (
	"fmt"
	"io"
	"os"
	"path/filepath"

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

type TakeOverArgs struct {
	WorkDir     string
	Secret      string
	GithubAppID int64
	Force       bool
	File        string
	KeyFile     string
	RepoDir     string
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
	cmd.Flags().StringVarP(&opts.File, "file", "", "", "Force a sync even if one isn't needed.")
	cmd.Flags().StringVarP(&opts.KeyFile, "ssh-key-file", "", "", "(Optional) Path of PEM file containing ssh key used to push current changes. If blank will try to find key in ${HOME}/.ssh.")
	cmd.Flags().StringVarP(&opts.RepoDir, "repo-dir", "", "", "(Optional) Directory containing the source repo that should be pushed. If blank it is inferred based on the path of the --file argument")

	cmd.MarkFlagRequired("file")
	cmd.MarkFlagRequired("private-key")
	return cmd
}

func readSecret(secret string) ([]byte, error) {
	f := &files.Factory{}
	h, err := f.Get(secret)
	if err != nil {
		return nil, err
	}
	r, err := h.NewReader(secret)
	if err != nil {
		return nil, err
	}
	return io.ReadAll(r)
}

func TakeOver(args *TakeOverArgs) error {
	log := zapr.NewLogger(zap.L())

	secret, err := readSecret(args.Secret)
	if err != nil {
		return errors.Wrapf(err, "Could not read file: %v", args.Secret)
	}
	manager, err := github.NewTransportManager(int64(args.GithubAppID), secret, log)
	if err != nil {
		log.Error(err, "TransportManager creation failed")
		return err
	}

	f, err := os.Open(args.File)
	if err != nil {
		return errors.Wrapf(err, "Failed to open file: %v", args.File)
	}

	d := yaml.NewDecoder(f)

	m := &v1alpha1.ManifestSync{}

	if err := d.Decode(m); err != nil {
		return errors.Wrapf(err, "Failed to decode ManifestSync from file %v", args.File)
	}

	syncer, err := gitops.NewSyncer(m, manager, gitops.SyncWithWorkDir(args.WorkDir), gitops.SyncWithLogger(log))
	if err != nil {
		return err
	}

	if args.RepoDir == "" {
		args.RepoDir = filepath.Dir(args.File)
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
