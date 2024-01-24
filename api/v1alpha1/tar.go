package v1alpha1

// Asset describes a package of assets to be included in a tarball
type Asset struct {
	// Source is the source of the asset
	Source AssetSource `json:"source,omitempty" yaml:"source,omitempty"`

	// DestPrefix is the prefix to add to the files when copying them into the tarball for hercules
	DestPrefix string `json:"destPrefix,omitempty" yaml:"destPrefix,omitempty"`
}

// AssetSource describes the source of an asset
//
// N.B. We currently don't support using docker images as assets. If you need to use a docker image as an asset
// You can do that directly in the Dockerfile using COPY --from
type AssetSource string {
	// Path is a path to a file or directory containing the asset
	// TODO(jeremy): Do we need to support includes/excludes?
	Path string `json:"path,omitempty" yaml:"path,omitempty"`
}

// TarManifest describes the manifest for a tarball
type TarManifest struct {
	Assets []Asset `json:"assets,omitempty" yaml:"assets,omitempty"`
}