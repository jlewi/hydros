package github

import (
	"net/url"

	"github.com/jlewi/hydros/api/v1alpha1"
)

// GitHubRepoToURI converts a GitHubRepo to a URI in the gogetter form.
// It assumes the protocol is https.
func GitHubRepoToURI(repo v1alpha1.GitHubRepo) url.URL {
	u := url.URL{
		Scheme: "https",
		Host:   "github.com",
		Path:   "/" + repo.Org + "/" + repo.Repo + ".git",
	}

	if repo.Branch != "" {
		u.RawQuery = "ref=" + repo.Branch
	}

	return u
}
