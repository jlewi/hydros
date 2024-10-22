package images

import (
	"github.com/google/go-containerregistry/pkg/authn"
	"github.com/google/go-containerregistry/pkg/authn/github"
	"github.com/google/go-containerregistry/pkg/v1/google"
)

var (
	// TODO(jeremy): Should we add support for Azure and AWS?
	// see https://github.com/google/go-containerregistry/pull/1252/files#diff-d062be9a5715169ccabeaa8a2d525b7340f8ec9a7534b3a27dfd1ae35148de29
	// for how we could do that.
	// TODO(jeremy): Should we use K8s chain? https://github.com/google/go-containerregistry/blob/main/pkg/authn/k8schain/README.md
	keychain = authn.NewMultiKeychain(
		authn.DefaultKeychain,
		google.Keychain,
		github.Keychain,
	)
)
