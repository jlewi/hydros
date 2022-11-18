package gitops

import (
	"fmt"
	"strings"

	"github.com/jlewi/hydros/api/v1alpha1"
	"github.com/jlewi/hydros/pkg/util"
)

// buildPrMessage generates the message for the PR.
func buildPrMessage(manifest *v1alpha1.ManifestSync, changedImages []util.DockerImageRef) string {
	sourceKey := fmt.Sprintf("%v/%v@%v", manifest.Spec.SourceRepo.Org, manifest.Spec.SourceRepo.Repo, manifest.Status.SourceCommit)
	lines := []string{
		fmt.Sprintf("[Auto] Hydrate %v with %v; %v images changed", manifest.Spec.DestRepo.Branch, sourceKey, len(changedImages)),
		fmt.Sprintf("Update hydrated manifests to [%v](%v)", sourceKey, manifest.Status.SourceURL),
		fmt.Sprintf("Source: [%v](%v)", sourceKey, manifest.Status.SourceURL),
		fmt.Sprintf("Source Branch: %v", manifest.Spec.SourceRepo.Branch),
	}

	if len(changedImages) == 0 {
		lines = append(lines, "Changed ImageList: None")
	} else {
		lines = append(lines, "Changed ImageList:")
		for _, i := range changedImages {
			lines = append(lines, fmt.Sprintf("* %v", i.ToURL()))
		}
	}

	return strings.Join(lines, "\n")
}
