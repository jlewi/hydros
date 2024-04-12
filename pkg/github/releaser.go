package github

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-logr/zapr"
	"github.com/google/go-github/v52/github"
	"github.com/jlewi/hydros/api/v1alpha1"
	"github.com/jlewi/hydros/pkg/config"
	"github.com/pkg/errors"
	"go.uber.org/zap"
	"golang.org/x/mod/semver"
)

// Releaser creates github releases if needed.
type Releaser struct {
	Transports *TransportManager
}

func NewReleaser(cfg config.Config) (*Releaser, error) {
	t, err := NewTransportManagerFromConfig(cfg)
	if err != nil {
		return nil, err
	}
	return &Releaser{
		Transports: t,
	}, nil
}

func (r *Releaser) Reconcile(ctx context.Context, resource *v1alpha1.GitHubReleaser) error {
	log := zapr.NewLogger(zap.L())
	log = log.WithValues("namespace", resource.Metadata.Namespace, "name", resource.Metadata.Name)
	org := resource.Spec.Org
	repo := resource.Spec.Repo

	client, err := createClient(r.Transports, org, repo)
	if err != nil {
		return err
	}

	// Get the latest commit on the repository
	latestCommit, err := getLatestCommit(ctx, client, org, repo)
	if err != nil {
		return err
	}
	log.Info("Got latest commit", "sha", latestCommit.GetSHA())

	latestSrc, latestReleaseCommit, err := getLatestRelease(ctx, client, org, repo)
	if err != nil {
		return err
	}

	if latestReleaseCommit == nil {
		log.Info("No releases found; creating first release")
	}
	// Check if the latest release on the source repository is the same as the latest commit
	// If not create a new release
	if latestReleaseCommit == nil || latestCommit.GetSHA() != latestReleaseCommit.GetSHA() {

		if err := createNewRelease(ctx, client, org, repo, latestSrc, latestCommit); err != nil {
			return err
		}
	} else {
		log.Info("No new commits since last release", "commit", latestCommit.GetSHA(), "tag", latestSrc.GetTagName())
	}

	return nil
}

// createNewRelease creates a new release if lastRelease is nil then it creates the first release
func createNewRelease(ctx context.Context, client *github.Client, org string, repo string, lastRelease *github.RepositoryRelease, commit *github.RepositoryCommit) error {
	log := zapr.NewLogger(zap.L())

	// Convention is to prefix with v
	newTag := "v0.0.1"

	if lastRelease != nil {
		pieces := strings.Split(lastRelease.GetTagName(), ".")
		minor := pieces[len(pieces)-1]
		minorInt, err := strconv.Atoi(minor)
		if err != nil {
			return errors.Wrapf(err, "Error parsing minor version %v", minor)
		}
		pieces[len(pieces)-1] = fmt.Sprintf("%d", minorInt+1)
		newTag = strings.Join(pieces, ".")
	}

	notes, _, err := client.Repositories.GenerateReleaseNotes(ctx, org, repo, &github.GenerateNotesOptions{
		TagName:         newTag,
		PreviousTagName: github.String(lastRelease.GetTagName()),
		TargetCommitish: github.String(commit.GetSHA()),
	})

	if err != nil {
		return errors.Wrapf(err, "there was a problem creating the release notes")
	}

	log.Info("creating GitHub release", "name", newTag, "commit", commit.GetSHA(), "tag", newTag, "notes", notes)
	// Create the release
	newRelease := &github.RepositoryRelease{
		Name:       github.String(notes.Name),
		TagName:    github.String(newTag),
		Body:       github.String(notes.Body),
		Prerelease: github.Bool(false),
		Draft:      github.Bool(false),
		MakeLatest: github.String("true"),
	}
	newRelease, _, err = client.Repositories.CreateRelease(ctx, org, repo, newRelease)
	if err != nil {
		return errors.Wrapf(err, "Failed to create release %v", newRelease.GetTagName())
	}

	return nil
}

func getLatestCommit(ctx context.Context, client *github.Client, org string, repo string) (*github.RepositoryCommit, error) {
	commit, _, err := client.Repositories.GetCommit(ctx, org, repo, "heads/main", &github.ListOptions{})

	if err != nil {
		return nil, errors.Wrapf(err, "failed to get latest commit")
	}

	return commit, nil
}

// getLatestRelease returns the latest release for the repository or nil if there isn't one
func getLatestRelease(ctx context.Context, client *github.Client, org string, repo string) (*github.RepositoryRelease, *github.RepositoryCommit, error) {
	releases, _, err := client.Repositories.ListReleases(ctx, org, repo, nil)

	if err != nil {
		return nil, nil, errors.Wrapf(err, "Error listing releases")
	}

	if len(releases) == 0 {
		return nil, nil, nil
	}

	var latestRelease *github.RepositoryRelease
	for _, release := range releases {
		// Skip draft and and prerelease releases
		if release.GetDraft() {
			continue
		}
		if release.GetPrerelease() {
			continue
		}

		if !semver.IsValid(release.GetTagName()) {
			continue
		}

		if latestRelease == nil {
			latestRelease = release
			continue
		}

		if semver.Compare(release.GetTagName(), latestRelease.GetTagName()) > 0 {
			latestRelease = release
		}
	}

	if latestRelease == nil {
		return latestRelease, nil, errors.Errorf("Could not find latest release")
	}

	releaseCommit, _, err := client.Repositories.GetCommit(ctx, org, repo, "tags/"+latestRelease.GetTagName(), &github.ListOptions{})
	if err != nil {
		return nil, nil, errors.Wrapf(err, "Error getting commit for release %v", latestRelease.GetTagName())
	}

	log := zapr.NewLogger(zap.L())
	log.Info("Latest release", "name", latestRelease.GetName(), "Assets URL", latestRelease.GetAssetsURL(), "commit", releaseCommit.GetSHA())
	return latestRelease, releaseCommit, nil
}

func createClient(transports *TransportManager, org string, repo string) (*github.Client, error) {
	tr, err := transports.Get(org, repo)
	if err != nil {
		return nil, errors.Wrapf(err, "Could not get transport for %v/%v", org, repo)
	}

	httpClient := &http.Client{
		Transport: tr,
	}
	client := github.NewClient(httpClient)
	return client, nil
}
