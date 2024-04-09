package images

import (
	"context"
	"fmt"

	"github.com/go-logr/zapr"
	"github.com/google/go-containerregistry/pkg/crane"
	"github.com/google/go-containerregistry/pkg/gcrane"
	"github.com/google/go-containerregistry/pkg/name"
	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/remote"
	"github.com/jlewi/hydros/api/v1alpha1"
	"github.com/jlewi/hydros/pkg/util"
	"github.com/pkg/errors"
	"go.uber.org/zap"
)

// Replicator is a controller for image replication.
type Replicator struct {
	options  []crane.Option
	rOptions []remote.Option
}

// NewReplicator creates a new Replicator.
func NewReplicator() (*Replicator, error) {
	// TODO(jeremy): What's a better pattern for handling options for crane.
	rOptions := []remote.Option{remote.WithAuthFromKeychain(gcrane.Keychain)}
	options := []crane.Option{crane.WithAuthFromKeychain(gcrane.Keychain)}

	r := &Replicator{
		rOptions: rOptions,
		options:  options,
	}

	return r, nil
}

func (r *Replicator) Reconcile(ctx context.Context, replicated *v1alpha1.ReplicatedImage) error {
	log := util.LogFromContext(ctx)
	log = log.WithValues("namespace", replicated.Metadata.Namespace, "name", replicated.Metadata.Name)

	if replicated.Spec.Source.Repository == "" {
		return fmt.Errorf("ReplicatedImage.Spec.Source.Repository is required")
	}

	// Get a tag for the source repository. This automatically defaults to the latest tag. We can specify a different
	// default tag if needed.
	latestTagRef, err := name.NewTag(replicated.Spec.Source.Repository)
	if err != nil {
		return errors.Wrapf(err, "failed to construct tagged image for  source repository: %v", err)
	}

	// Get the image digest
	latestDesc, err := remote.Get(latestTagRef, r.rOptions...)
	if err != nil {
		return errors.Wrapf(err, "failed to get image: %v", latestTagRef.String())
	}

	latestImg, err := latestDesc.Image()
	if err != nil {
		return errors.Wrapf(err, "failed to get image: %v", latestTagRef.String())
	}
	// Now that we know the digest we can construct a reference to the image using the digest rather than the tag
	latestRef := latestTagRef.Tag(latestDesc.Digest.String())
	log.Info("Latest image", "digest", latestRef.String(), "ref", latestRef.String())

	// Get the tags for the source image
	tags, err := r.getTagsForImage(latestTagRef.Context(), latestDesc.Digest)
	if err != nil {
		return errors.Wrapf(err, "failed to get tags for image: %v", latestTagRef.String())
	}
	log.Info("Tags for image", "tags", tags)

	allErrors := &util.ListOfErrors{
		Causes: []error{},
	}

	for _, dest := range replicated.Spec.Destinations {
		// Copy the image to the destination
		log.Info("Copying image", "destination", dest)
		// Copy the image to the destination
		err = crane.Push(latestImg, dest, r.options...)
		if err != nil {
			return errors.Wrapf(err, "failed to copy image to destination: %s", dest)
		}

		destRef, err := name.NewRepository(dest)
		if err != nil {
			return errors.Wrapf(err, "failed to construct repository for destination: %v", dest)
		}
		destDigestRef := destRef.Digest(latestDesc.Digest.String())
		for _, tag := range tags {
			log.Info("Adding tag", "image", destDigestRef.String(), "tag", tag)
			if err := crane.Tag(destDigestRef.String(), tag, r.options...); err != nil {
				log.Error(err, "Failed to add tag", "tag", tag, "image", destDigestRef.String())
				allErrors.AddCause(errors.Wrapf(err, "failed to add tag: %v", tag))
			}
		}
	}

	if len(allErrors.Causes) == 0 {
		return nil
	}
	allErrors.Final = fmt.Errorf("failed to apply one or more resources")

	return allErrors
}

// getTagsForImage returns the tags for the given image digest.
func (r *Replicator) getTagsForImage(repository name.Repository, digest v1.Hash) ([]string, error) {
	// List all tags for the repository
	// TODO(jeremy): Is there a more efficient way to do this?
	tags, err := remote.List(repository, r.rOptions...)
	if err != nil {
		return nil, fmt.Errorf("failed to list tags: %v", err)
	}

	log := zapr.NewLogger(zap.L())
	// Filter tags for the given digest
	var tagsForDigest []string
	for _, tag := range tags {
		taggedRef := repository.Tag(tag)
		log.Info("Checking tag", "tag", tag)
		desc, err := remote.Get(taggedRef, r.rOptions...)
		if err != nil {
			return nil, fmt.Errorf("failed to get image description for tag %s: %v", tag, err)
		}

		if desc.Digest == digest {
			tagsForDigest = append(tagsForDigest, tag)
		}
	}

	return tagsForDigest, nil
}
