package ecrutil

import (
	"fmt"

	"github.com/pkg/errors"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/awserr"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/ecr"
	"github.com/go-logr/zapr"
	"github.com/jlewi/hydros/pkg/util"
	"go.uber.org/zap"
)

// AddTagsToImage adds the tags to the existing ECR image.
// image full URL of the image.
func AddTagsToImage(sess *session.Session, image string, tags []string) error {
	log := zapr.NewLogger(zap.L())

	svc := ecr.New(sess)

	resolved, err := util.ParseImageURL(image)
	if err != nil {
		log.Error(err, "Failed to parse image", "image", image)
		return err
	}

	imageID := &ecr.ImageIdentifier{}

	if resolved.Sha != "" {
		imageID.ImageDigest = aws.String(resolved.Sha)
	} else {
		imageID.ImageTag = aws.String(resolved.Tag)
	}
	input := &ecr.BatchGetImageInput{
		ImageIds: []*ecr.ImageIdentifier{
			{
				ImageTag: aws.String(resolved.Tag),
			},
		},
		RegistryId:     aws.String(resolved.GetAwsRegistryID()),
		RepositoryName: aws.String(resolved.Repo),
	}

	log = log.WithValues("image", image)
	result, err := svc.BatchGetImage(input)
	if err != nil {
		if aerr, ok := err.(awserr.Error); ok {
			log.Error(err, "Failed to fetch image info from ECR", "code", aerr.Code(), "awsError", aerr.Error())
		} else {
			log.Error(err, "Failed to fetch image info from ECR")
		}
		return err
	}

	if len(result.Images) != 1 {
		err := fmt.Errorf("Expected 1 image; got %v", len(result.Images))
		log.Error(err, "Expected only one image details", "imageDetails", result.Images)
		return err
	}

	manifest := result.Images[0].ImageManifest

	for _, label := range tags {
		req := &ecr.PutImageInput{
			ImageManifest:  manifest,
			ImageTag:       aws.String(label),
			RepositoryName: result.Images[0].RepositoryName,
			RegistryId:     result.Images[0].RegistryId,
		}

		_, err := svc.PutImage(req)
		if err != nil {
			if aerr, ok := err.(awserr.Error); ok {
				switch aerr.Code() {
				case ecr.ErrCodeImageAlreadyExistsException:
					log.Info("URI already has tag", "image", image, "tag", label)
				default:
					return errors.Wrapf(err, "Failed to resolve image to sha; image: %v", image)
				}
			} else {
				return err
			}
		}
		log.Info("Successfully tagged image", "tag", label)
	}
	return nil
}

// EnsureRepoExists ensures the repository exists.
// If the repository exists this function does nothing; if it doesn't exist the repo is created.
func EnsureRepoExists(sess *session.Session, registry string, repo string) error {
	svc := ecr.New(sess)
	req := &ecr.CreateRepositoryInput{
		RegistryId:     aws.String(registry),
		RepositoryName: aws.String(repo),
	}
	_, err := svc.CreateRepository(req)
	if err != nil {
		if aerr, ok := err.(awserr.Error); ok {
			switch aerr.Code() {
			case ecr.ErrCodeRepositoryAlreadyExistsException:
				// do nothing
				return nil
			default:
				return errors.Wrapf(err, "Failed to create repo; registry: %v; repo: %v", registry, repo)
			}
		} else {
			return err
		}
	}

	return nil
}
