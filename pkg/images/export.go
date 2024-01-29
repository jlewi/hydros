package images

import (
	"fmt"
	"os"

	"github.com/google/go-containerregistry/pkg/crane"
	"github.com/google/go-containerregistry/pkg/gcrane"
	v1 "github.com/google/go-containerregistry/pkg/v1"
)

// ExportImage uses crane to export an image to a tarball
// It is basically the same code as crane export
// https://github.com/google/go-containerregistry/blob/a0658aa1d0cc7a7f1bcc4a3af9155335b6943f40/cmd/crane/cmd/export.go#L55
//
// This is different from image downloader because that appears to download the manifest and individual blobs.
func ExportImage(src string, tarFilePath string) error {
	options := []crane.Option{crane.WithAuthFromKeychain(gcrane.Keychain)}
	var img v1.Image
	desc, err := crane.Get(src, options...)
	if err != nil {
		return fmt.Errorf("pulling %s: %w", src, err)
	}
	if desc.MediaType.IsSchema1() {
		img, err = desc.Schema1()
		if err != nil {
			return fmt.Errorf("pulling schema 1 image %s: %w", src, err)
		}
	} else {
		img, err = desc.Image()
		if err != nil {
			return fmt.Errorf("pulling URI %s: %w", src, err)
		}
	}

	f, err := os.Create(tarFilePath)
	if err != nil {
		return err
	}
	defer f.Close()
	return crane.Export(img, f)
}
