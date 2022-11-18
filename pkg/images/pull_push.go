package images

import (
	"fmt"
	"io/ioutil"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/go-logr/logr"
	"github.com/google/go-containerregistry/pkg/crane"
	"github.com/google/go-containerregistry/pkg/name"
	"github.com/jlewi/hydros/api/v1alpha1"
	"github.com/jlewi/hydros/pkg/kustomize/fns/images"
	"github.com/matryer/try"
	"gopkg.in/yaml.v3"
)

// ImageDownloader handles pulling images given ImageList
type ImageDownloader struct {
	Log              logr.Logger
	ImageList        v1alpha1.ImageList
	ImageDir         string
	SkipIfFileExists bool
	DummyDownload    bool
}

// ImageUploader handles upload images to target repo given image dir
type ImageUploader struct {
	Log        logr.Logger
	ImageDir   string
	TargetRepo string
	SkipUpload bool
}

// DownloadImagesWithRetry calls DownloadImagesToDir, retrying if errors occur
func (d *ImageDownloader) DownloadImagesWithRetry(retry int) error {
	err := try.Do(func(attempt int) (bool, error) {
		err := d.downloadImagesToDir()
		return attempt < retry, err
	})
	if err != nil {
		d.Log.Error(err, "retrying downloadImagesToDir still failed", "retry", retry)
		return err
	}
	return err
}

// downloadImagesToDir downloads images from a remote repo, to a specific directory
func (d *ImageDownloader) downloadImagesToDir() error {
	// Loop over the source images
	for _, sourceImage := range d.ImageList.Images {

		// Construct path to where the image will be saved on disk
		imagePath := path.Join(d.ImageDir, sourceImage)

		// If the filepath already exists on disk and we've been told to skip, then don't download
		if _, err := os.Stat(imagePath); err == nil && d.SkipIfFileExists {
			d.Log.Info("Skipping download because file exists", "imagePath", imagePath)
			continue
		}

		// Ensure that the dir where the image will be written exists
		if err := os.MkdirAll(path.Dir(imagePath), os.ModePerm); err != nil {
			d.Log.Error(err, "failed to make dir for image", "imagePath", imagePath, "sourceImage", sourceImage)
			return err
		}

		// If DummyDownload is set to true, simply create an empty file on disk and skip downloading from source URI
		if d.DummyDownload {
			emptyFile, err := os.Create(imagePath)
			if err != nil {
				d.Log.Error(err, "failed to create dummy file", "imagePath", imagePath, "sourceImage", sourceImage)
				return err
			}
			err = emptyFile.Close()
			if err != nil {
				d.Log.Error(err, "failed to close dummy file", "imagePath", imagePath, "sourceImage", sourceImage)
			}
			continue
		}

		// Pull sourceImage and write to the imagePath on disk
		d.Log.Info("Downloading image", "sourceImage", sourceImage, "imagePath", imagePath)
		err := downloadImage(sourceImage, imagePath)
		if err != nil {
			d.Log.Error(err, "downloadImage step failed", "sourceImage", sourceImage, "imagePath", imagePath)
			return err
		}
	}

	return nil
}

// downloadImage uses crane to download the given image reference and write to disk at the provided path
func downloadImage(imageSrc string, outputPath string) error {
	// Pull the image
	image, err := crane.Pull(imageSrc)
	if err != nil {
		return err
	}

	// Save the image as a tar to the outputPath, tagged w initial name
	err = crane.Save(image, imageSrc, outputPath)
	if err != nil {
		return err
	}

	return nil
}

// UploadImagesWithRetry calls UploadImagesFromDir, retrying if errors occur
func (u *ImageUploader) UploadImagesWithRetry(retry int) (imageMappings []images.ImageMapping, err error) {
	err = try.Do(func(attempt int) (bool, error) {
		imageMappings, err = u.uploadImagesFromDir()
		return attempt < retry, err
	})
	if err != nil {
		u.Log.Error(err, "retrying uploadImagesFromDir still failed", "retry", retry)
		return imageMappings, err
	}
	return imageMappings, nil
}

// uploadImagesFromDir uploads the images found in imagesDir to the targetRepo
func (u *ImageUploader) uploadImagesFromDir() (imageMappings []images.ImageMapping, err error) {
	// Get list of files/images from the imagesDir
	imagePaths, err := getImagePaths(u.ImageDir)
	if err != nil {
		u.Log.Error(err, "failed to getImagePaths from imagesDir", "u.ImageDir", u.ImageDir)
		return imageMappings, err
	}

	// For each image file
	for _, imagePath := range imagePaths {

		// Get the source image URI based on the image path
		sourceImage, err := u.getSourceImage(imagePath)
		if err != nil {
			u.Log.Error(err, "Failed to getSourceImage from imagePath", "imagePath", imagePath)
			return imageMappings, err
		}

		// Construct the target URI from the source image URI
		targetURI, err := u.getTargetURI(sourceImage)
		if err != nil {
			u.Log.Error(err, "failed to getTargetURI given sourceImage", "sourceImage", sourceImage)
			return imageMappings, err
		}

		// Add to list of ImageMappings - source image to target image to be used in the manifest transform
		imageMappings = append(imageMappings, images.ImageMapping{
			Src:  sourceImage,
			Dest: targetURI,
		})

		// If option to skip upload step is specified we are done - used if we just want the ImagePrefix fn
		if u.SkipUpload {
			u.Log.Info("Skipping upload", "imagePath", imagePath, "targetURI", targetURI, "sourceImage", sourceImage)
			continue
		}

		// Kick off upload
		u.Log.Info("Uploading image", "imagePath", imagePath, "targetURI", targetURI, "sourceImage", sourceImage)
		err = uploadImage(imagePath, targetURI)
		if err != nil {
			u.Log.Error(err, "uploadImage failed", "imagePath", imagePath, "targetURI", targetURI)
			return imageMappings, err
		}
	}

	return imageMappings, nil
}

// uploadImage uploads an image from disk to the given URI
func uploadImage(imagePath string, targetURI string) error {
	// load the image from disk
	imageV1, err := crane.Load(imagePath)
	if err != nil {
		return err
	}

	// push image to remote registry
	err = crane.Push(imageV1, targetURI)
	if err != nil {
		return err
	}

	return nil
}

// ParseImageList attempts to initialize an ImageList object from the supplied path
func ParseImageList(imageListPath string) (imageList v1alpha1.ImageList, err error) {
	data, err := ioutil.ReadFile(imageListPath)
	if err != nil {
		return imageList, err
	}

	if err = yaml.Unmarshal(data, &imageList); err != nil {
		return imageList, err
	}

	return imageList, nil
}

// getImagePaths walks the given images directory and returns lists of paths to the image files
func getImagePaths(imagesDir string) (imagePaths []string, err error) {
	// Walk the images directory to get the list of image file paths
	// imagePaths := []string{}
	err = filepath.Walk(imagesDir,
		func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return err
			}
			// Add all file paths to our list
			if !info.IsDir() {
				imagePaths = append(imagePaths, path)
			}
			return nil
		})

	if err != nil {
		return nil, err
	}

	return imagePaths, nil
}

// getSourceImage will return the source image URI from the given path on disk
func (u *ImageUploader) getSourceImage(imagePath string) (sourceImage string, err error) {
	// Reconstruct the source image URI from the path by discarding the root imagesDir
	// 'path/to/images-dir/ecr-repo-host/ecr-repo-name:1234@xxxxx' becomes 'ecr-repo-host/ecr-repo-name:1234@xxxxx'
	sourceImage, err = filepath.Rel(u.ImageDir, imagePath)
	if err != nil {
		return sourceImage, fmt.Errorf("error constructing sourceImage from imagePath %s: %w", imagePath, err)
	}
	return sourceImage, nil
}

// getTargetURI will construct a Target URI given the source image URI and ImageUploader.TargetRepo
func (u *ImageUploader) getTargetURI(sourceImage string) (targetURI string, err error) {
	// Discard any digest that may be on the image (chars after @)
	// 'ecr-repo-host/ecr-repo-name:1234@xxxxx' becomes 'ecr-repo-host/ecr-repo-name:1234'
	digestCharIdx := strings.IndexByte(sourceImage, '@')
	if digestCharIdx != -1 {
		sourceImage = sourceImage[:digestCharIdx]
	}

	// Parse the sourceImage string into a proper image reference object
	sourceImageRef, err := name.ParseReference(sourceImage)
	if err != nil {
		return targetURI, fmt.Errorf("error parsing image %s to imageRef: %w", sourceImage, err)
	}

	// Take the image 'context' and remove the host which will be replaced by the target repo.
	// Combine the other parts of the context into a string that will be part of the target tag.
	// The image 'ecr-repo-host/repo-name1/repo-name2:1234' has a context of
	// 'ecr-repo-host/repo-name1/repo-name2', which will here yield a trailingContextStr
	// of 'repo-name1-repo-name2'
	sourceImageContext := sourceImageRef.Context()
	contextElements := strings.Split(sourceImageContext.String(), "/")
	trailingContextStr := strings.Join(contextElements[1:], "-")

	// Construct final target URI for the image
	targetURI = fmt.Sprintf("%s:%s-%s", u.TargetRepo, trailingContextStr, sourceImageRef.Identifier())

	return targetURI, nil
}
