package s3assets

import (
	"io"

	"github.com/go-logr/logr"
	"sigs.k8s.io/kustomize/kyaml/yaml"

	"github.com/PrimerAI/hydros-public/api/v1alpha1"
	"github.com/PrimerAI/hydros-public/pkg/regexp"
	"github.com/PrimerAI/hydros-public/pkg/util"
)

// Extractor extracts all the S3 urls in a directory of manifests
type Extractor struct {
	Log logr.Logger
}

// DefaultS3Regexp matches s3://.* until a " or \n
const DefaultS3Regexp = `(s3:\/\/[a-zA-Z0-9\/\-\_\.]+)`

// Extract takes a source directory of manifests and slice of strings used to extract strings and
// returns a list of extracted strings
func (e Extractor) Extract(sourceDir string, extraRegexps ...string) ([]string, error) {
	regexps := util.UniqueStrings(append(extraRegexps, DefaultS3Regexp))
	re := regexp.Extractor{Log: e.Log}
	return re.Extract(sourceDir, regexps)
}

// Write takes a list of s3 assets and writes them as a yaml S3Assets doc to output
func Write(s3Assets []string, output io.Writer) error {
	doc := v1alpha1.S3AssetsList{
		Kind:       "S3AssetsList",
		APIVersion: "primer.ai.hydros/v1alpha1",
		S3Assets:   s3Assets,
	}
	enc := yaml.NewEncoder(output)
	return enc.Encode(doc)
}
