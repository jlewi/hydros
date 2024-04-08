package images

import (
	"fmt"
	"github.com/go-logr/zapr"
	"github.com/google/go-containerregistry/pkg/crane"
	"github.com/google/go-containerregistry/pkg/gcrane"
	"github.com/google/go-containerregistry/pkg/name"
	"github.com/google/go-containerregistry/pkg/v1/remote"
	"go.uber.org/zap"
	"os"
	"testing"
)

func GetTagsForDigest(imageRef string) ([]string, error) {
	// Parse the image reference
	ref, err := name.ParseReference(imageRef)
	if err != nil {
		return nil, fmt.Errorf("failed to parse image reference: %v", err)
	}

	rOptions := []remote.Option{remote.WithAuthFromKeychain(gcrane.Keychain)}

	// Get the image digest
	img, err := remote.Get(ref, rOptions...)
	if err != nil {
		return nil, fmt.Errorf("failed to get image digest: %v", err)
	}

	// List all tags for the repository
	tags, err := remote.List(ref.Context(), rOptions...)
	if err != nil {
		return nil, fmt.Errorf("failed to list tags: %v", err)
	}

	log := zapr.NewLogger(zap.L())
	// Filter tags for the given digest
	var tagsForDigest []string
	for _, tag := range tags {
		taggedRef := ref.Context().Tag(tag)
		log.Info("Checking tag", "tag", tag)
		desc, err := remote.Get(taggedRef, rOptions...)
		if err != nil {
			return nil, fmt.Errorf("failed to get image description for tag %s: %v", tag, err)
		}

		if desc.Digest == img.Digest {
			tagsForDigest = append(tagsForDigest, tag)
		}
	}

	return tagsForDigest, nil
}

func CopyImage(srcImage, dstImage string) error {
	// Parse the source and destination image references
	src, err := name.ParseReference(srcImage)
	if err != nil {
		return fmt.Errorf("failed to parse source image reference: %v", err)
	}

	//dst, err := name.ParseReference(dstImage)
	//if err != nil {
	//	return fmt.Errorf("failed to parse destination image reference: %v", err)
	//}

	rOptions := []remote.Option{remote.WithAuthFromKeychain(gcrane.Keychain)}

	// Get the source image
	img, err := remote.Image(src, rOptions...)
	if err != nil {
		return fmt.Errorf("failed to get source image: %v", err)
	}

	options := []crane.Option{crane.WithAuthFromKeychain(gcrane.Keychain)}
	// Copy the image to the destination
	err = crane.Push(img, dstImage, options...)
	if err != nil {
		return fmt.Errorf("failed to copy image: %v", err)
	}

	tags, err := GetTagsForDigest(srcImage)
	if err != nil {
		return fmt.Errorf("failed to list tags: %v", err)
	}

	log := zapr.NewLogger(zap.L())
	for _, tag := range tags {
		log.Info("Adding tag", "tag", tag)
		err = crane.Tag(dstImage, tag, options...)
	}
	return nil
}

func TestCopyImage(t *testing.T) {
	if os.Getenv("GITHUB_ACTIONS") != "" {
		t.Skipf("Test_ResolveImageToSHA is a manual test that is skipped in CICD")
	}

	srcImage := "us-west1-docker.pkg.dev/foyle-public/images/vscode-web-assets:latest"
	dstImage := "ghcr.io/jlewi/vscode-web-assets:latest"

	err := CopyImage(srcImage, dstImage)
	if err != nil {
		t.Fatalf("CopyImage() = %v, wanted nil", err)
	}
}
