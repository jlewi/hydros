package kustomize

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const (
	// KustomizationType the type of kustomization file
	KustomizationType = "kustomization.yaml"
)

// GenerateTargetPath is a helper function to generate the path where
// hydrated manifests should be generated.
// sourceBase base for searching for overlays to generate
func GenerateTargetPath(sourceBase string, kustomization string) (TargetPath, error) {
	result := TargetPath{}

	rPath, err := filepath.Rel(sourceBase, kustomization)
	if err != nil {
		return result, err
	}
	pieces := strings.Split(rPath, string(os.PathSeparator))

	if len(pieces) < 3 && strings.Contains(kustomization, KustomizationType) {
		if len(pieces) == 2 {
			// Path is of the form {PATH}/kustomization.yaml. This is uncommon but happens occassionally when
			// a user hasn't bothered to create multiple overlays.
			result.OverlayName = ""
			result.Dir = pieces[0]
			return result, nil
		}
		// Something went wrong
		return result, fmt.Errorf("Path %v; is not of the form {PATH}/{OVERLAY}/kustomization.yaml or {PATH}/kustomization.yaml", kustomization)
	}

	result.OverlayName = pieces[len(pieces)-2]
	// N.B we strip out the final directory because we assume its an overlay dir like "dev" or "prod" and we don't
	// want that in the output path. We might want to make that a configurable option.
	result.Dir = filepath.Join(pieces[0 : len(pieces)-2]...)

	return result, nil
}

// TargetPath to get target Dir and OverlayName by calling GenerateTargetPath
type TargetPath struct {
	Dir         string
	OverlayName string
}
