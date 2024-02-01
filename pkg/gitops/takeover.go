package gitops

import (
	"context"
	"github.com/jlewi/hydros/api/v1alpha1"
	gh "github.com/jlewi/hydros/pkg/github"
	"github.com/jlewi/hydros/pkg/github/ghrepo"
	"github.com/jlewi/hydros/pkg/util"
	"github.com/pkg/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"net/url"
	"time"
)

// rewriteRepos rewrites the repos in the manifest to the new repos if necessary.
func rewriteRepos(ctx context.Context, m *v1alpha1.ManifestSync, mappings []v1alpha1.RepoMapping) error {
	log := util.LogFromContext(ctx)

	if m == nil {
		return errors.New("Manifest is nil")
	}

	if mappings == nil {
		return nil
	}

	srcRepoURL := gh.GitHubRepoToURI(m.Spec.SourceRepo)

	for _, mapping := range mappings {
		if srcRepoURL.String() != mapping.Input {
			continue
		}

		log.Info("Rewriting source repo", "old", srcRepoURL.String(), "new", mapping.Output, "manifestSync.Name", m.Metadata.Name)

		u, err := url.Parse(mapping.Output)
		if err != nil {
			return errors.Wrapf(err, "Could not parse URL %v", mapping.Output)
		}

		r, err := ghrepo.FromURL(u)
		if err != nil {
			return errors.Wrapf(err, "Could not parse URL %v", srcRepoURL.String())
		}

		// ref parameter specifies the reference to checkout
		// https://github.com/hashicorp/go-getter#protocol-specific-options
		branch := u.Query().Get("ref")
		if branch == "" {
			return errors.Wrapf(err, "Branch is not specified in URL %v; it should be specified as a query argument e.g. ?ref=main", mapping.Output)
		}
		m.Spec.SourceRepo.Org = r.RepoOwner()
		m.Spec.SourceRepo.Repo = r.RepoName()
		m.Spec.SourceRepo.Branch = branch
		return nil
	}
	return nil
}

// SetTakeOverAnnotations sets the takeover annotations on the manifest.
func SetTakeOverAnnotations(m *v1alpha1.ManifestSync, pause time.Duration) error {
	tEnd := time.Now().Add(pause)

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

	return nil
}
