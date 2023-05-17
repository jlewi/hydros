package app

import (
	"context"
	"github.com/jlewi/hydros/api/v1alpha1"

	"github.com/google/go-github/v52/github"
	"github.com/palantir/go-githubapp/appconfig"
	"gopkg.in/yaml.v2"
)

// FetchedConfig represents the hydros configuration fetched from GitHub and used to configure hydros
type FetchedConfig struct {
	Config     *v1alpha1.Config
	LoadError  error
	ParseError error

	Source string
	Path   string
}

type ConfigFetcher struct {
	Loader *appconfig.Loader
}

func (cf *ConfigFetcher) ConfigForRepositoryBranch(ctx context.Context, client *github.Client, owner, repository, branch string) FetchedConfig {
	c, err := cf.Loader.LoadConfig(ctx, client, owner, repository, branch)
	fc := FetchedConfig{
		Source: c.Source,
		Path:   c.Path,
	}

	switch {
	case err != nil:
		fc.LoadError = err
		return fc
	case c.IsUndefined():
		return fc
	}

	var pc v1alpha1.Config
	if err := yaml.UnmarshalStrict(c.Content, &pc); err != nil {
		fc.ParseError = err
	} else {
		fc.Config = &pc
	}
	return fc
}
