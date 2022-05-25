package github

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"

	ghinstallation "github.com/bradleyfalzon/ghinstallation/v2"
	"github.com/go-logr/logr"

	"github.com/pkg/errors"
)

// GetInstallID returns the installation id for the specified GitHubApp.
// privateKey should be the path to the privateKey.
func GetInstallID(appID int64, privateKey string, owner string, repo string) (int64, error) {
	tr := http.DefaultTransport

	appTr, err := ghinstallation.NewAppsTransportKeyFromFile(tr, appID, privateKey)

	client := &http.Client{Transport: appTr}

	if err != nil {
		return 0, errors.WithStack(errors.Wrapf(err, "there was a problem getting the GitHub installation id"))
	}

	// Get the installtion id
	url := fmt.Sprintf("https://api.github.com/repos/%v/%v/installation", owner, repo)
	resp, err := client.Get(url)
	if err != nil {
		return 0, errors.WithStack(errors.Wrapf(err, "there was a problem getting the GitHub installation id"))
	}

	if resp.StatusCode != http.StatusOK {
		// TODO(jlewi): Should we try to read the body and include that in the error message?
		return 0, errors.WithStack(errors.Wrapf(err, "there was a problem getting the GitHub installation id; Get %v returned statusCode %v; Response:\n%+v", url, resp.StatusCode, resp))
	}

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return 0, errors.WithStack(errors.Wrapf(err, "there was a problem reading the response body"))
	}

	type idResponse struct {
		ID int64 `json:"id"`
	}

	r := &idResponse{}
	err = json.Unmarshal(body, r)

	if err != nil {
		return 0, errors.WithStack(errors.Wrapf(err, "Could not unmarshal json:\n %v", body))
	}
	return r.ID, nil
}

// orgAndRepo is a tuple of GitHub Org and Repo
type orgAndRepo struct {
	Org  string
	Repo string
}

func newOrgAndRepo(org string, repo string) orgAndRepo {
	return orgAndRepo{Org: org, Repo: repo}
}

// TransportManager manages credentials for a GitHub App.
// TODO(jeremy): Can/should we wrap this in the OAuth flow.
// TODO(jeremy): Should we reuse some of palantir built?
// https://github.com/palantir/go-githubapp/blob/develop/githubapp/client_creator.go
type TransportManager struct {
	log            logr.Logger
	appID          int64
	privateKeyFile string

	// Map of orgAndRepo to the transport to talk to that Repo.
	ghTransports map[orgAndRepo]*ghinstallation.Transport
}

// NewTransportManager creates a new transport manager.
func NewTransportManager(appID int64, privateKeyFile string, log logr.Logger) (*TransportManager, error) {
	if appID == 0 {
		return nil, fmt.Errorf("gitHubAppID is required")
	}

	if privateKeyFile == "" {
		return nil, fmt.Errorf("privateKeyFile is required")
	}

	return &TransportManager{
		log:            log,
		appID:          appID,
		privateKeyFile: privateKeyFile,
		ghTransports:   map[orgAndRepo]*ghinstallation.Transport{},
	}, nil
}

// Get returns a transport to talk to the specified Org and Repo.
func (m *TransportManager) Get(org string, repo string) (*ghinstallation.Transport, error) {
	log := m.log.WithValues("Org", org, "Repo", repo)
	key := newOrgAndRepo(org, repo)

	if tr, ok := m.ghTransports[key]; ok {
		return tr, nil
	}

	gitHubInstallID, err := GetInstallID(m.appID, m.privateKeyFile, org, repo)
	if err != nil {
		log.Error(err, "Failed to Get GitHub App InstallId", "AppId", m.appID, "Org", org, "Repo", repo)
		return nil, err
	}

	if gitHubInstallID == 0 {
		err := fmt.Errorf("Could not get GitHub App InstallId")
		log.Error(err, "Failed to Get GitHub App InstallId", "AppId", m.appID, "Org", org, "Repo", repo)
		return nil, err
	}
	// Shared transport to reuse TCP connections.
	tr := http.DefaultTransport

	// Wrap the shared transport for use with the app ID 1 authenticating with installation ID 99.
	ghTr, err := ghinstallation.NewKeyFromFile(tr, m.appID, gitHubInstallID, m.privateKeyFile)
	if err != nil {
		log.Error(err, "Failed to Get GitHub App Transport", "AppId", m.appID, "Org", org, "Repo", repo)
		return nil, err
	}

	m.ghTransports[key] = ghTr
	return ghTr, nil
}
