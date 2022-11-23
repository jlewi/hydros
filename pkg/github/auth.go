package github

import (
	"context"
	"fmt"
	"net/http"

	"github.com/bradleyfalzon/ghinstallation/v2"
	"github.com/go-logr/zapr"
	"go.uber.org/zap"
)

const (
	// GitHubAppUsername is the username used by GitHub App's for basic auth.
	GitHubAppUsername = "x-access-token"
)

// AppAuth implements BasicAuth for GitHub Apps for use with GoGit.
// When authenticating as a GitHub App, we need to use basic auth with the username "x-access-token"
//
// Reference:
// https://github.com/go-git/go-git/blob/c798d4a42004b1c8976a6a4f42f131f16d08b6fa/plumbing/transport/http/common.go#L191
//
// We also need to generate an appropriate token to use as the password. Since the token can expire we can't
// use the existing BasicAuth.
//
// It is similar to go-git's BasicAuth.
// https://github.com/go-git/go-git/blob/c798d4a42004b1c8976a6a4f42f131f16d08b6fa/plumbing/transport/http/common.go#L191
type AppAuth struct {
	Tr *ghinstallation.Transport
}

// SetAuth adds the appropriate authentication headers to the request.
func (a *AppAuth) SetAuth(r *http.Request) {
	if a == nil {
		return
	}

	token, err := a.Tr.Token(context.Background())

	if err != nil {
		log := zapr.NewLogger(zap.L())
		log.Error(err, "Failed to generate access token; requests will faill")
	}
	r.SetBasicAuth(GitHubAppUsername, token)
}

// Name is name of the auth method.
func (a *AppAuth) Name() string {
	return "http-basic-auth"
}

// String returns a sanitized string suitable for log messages.
func (a *AppAuth) String() string {
	masked := "*******"
	masked = "<token to be generated>"

	return fmt.Sprintf("%s - %s:%s", a.Name(), GitHubAppUsername, masked)
}
