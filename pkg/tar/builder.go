package tar

import (
	"archive/tar"
	"compress/gzip"
	"github.com/go-logr/zapr"
	"github.com/jlewi/hydros/api/v1alpha1"
	"github.com/jlewi/hydros/pkg/files"
	"github.com/jlewi/monogo/util"
	"github.com/pkg/errors"
	"go.uber.org/zap"
	"io"
	"os"
	"path/filepath"
)

// Build builds an archive from the manifest
// basePath is the basePath to resolve relative paths against
// tarball is the path to the tarball to create
func Build(image *v1alpha1.Image, basePath string, tarFilePath string) error {
	log := zapr.NewLogger(zap.L())

	factory := &files.Factory{}

	helper, err := factory.Get(tarFilePath)

	if err != nil {
		return errors.Wrapf(err, "Error creating helper for %v", tarFilePath)
	}

	w, err := helper.NewWriter(tarFilePath)
	if err != nil {
		return err
	}
	defer util.MaybeClose(w)

	// Create a new tar archive
	log.Info("Creating tarball", "basePath", basePath, "tarFilePath", tarFilePath)

	// If you want GZIP compression, wrap the tar writer in a gzip writer
	gzWriter := gzip.NewWriter(w)
	defer gzWriter.Close()

	// Create a tar writer
	tw := tar.NewWriter(gzWriter)
	defer tw.Close()

	for _, a := range image.Spec.Source {
		log.Info("Adding asset", "asset", a)
		matches, err := filepath.Glob(a.Src)
		if err != nil {
			return err
		}
		for _, m := range matches {
			if err := addFileToTarGenerator(tw, m, filepath.Join(basePath, a.Strip), a.Dest); err != nil {
				log.Error(err, "Error adding file to tarball", "file", m, "basePath", basePath, "strip", a.Strip, "dest", a.Dest)
				return err
			}
		}

	}
	return nil
}

// copyTarball copies all the assets in the source tarbell to the destination tarball
//func copyTarBall(srcTarball string, tw *tar.Writer, prefix string) error {
//	log := zapr.NewLogger(zap.L())
//	// Open the tarball file
//	file, err := os.Open(srcTarball)
//	if err != nil {
//		return errors.Wrapf(err, "Error opening tarball %v", srcTarball)
//	}
//	defer file.Close()
//
//	// Create a tar reader
//	tarReader := tar.NewReader(file)
//
//	// Iterate over each file in the tarball
//	for {
//		header, err := tarReader.Next()
//
//		if err == io.EOF {
//			// Reached the end of the tarball
//			return nil
//		}
//
//		if err != nil {
//			return errors.Wrapf(err, "Error reading tar header:")
//		}
//
//		log.Info("Reading tarball entry", "header", header.Name, "size", header.Size)
//
//		if header.Size == 0 {
//			log.Info("Skipping empty file", "header", header.Name)
//			continue
//		}
//
//		path := header.Name
//		if prefix != "" {
//			path = filepath.Join(prefix, header.Name)
//		}
//
//		// Create a tar header
//		newHeader := header
//		newHeader.Name = path
//
//		if err := tw.WriteHeader(newHeader); err != nil {
//			return errors.Wrapf(err, "Error writing tar header: %v", newHeader.Name)
//		}
//		// Read the file contents
//		_, err = io.CopyN(tw, tarReader, header.Size)
//		if err != nil {
//			return errors.Wrapf(err, "Error reading file contents")
//		}
//	}
//}

// addFileToTarGenerator adds a file to the tarball
func addFileToTarGenerator(tw *tar.Writer, path string, strip string, destPrefix string) error {
	log := zapr.NewLogger(zap.L())

	info, err := os.Stat(path)
	if err != nil {
		return err
	}
	base := filepath.Base(path)
	if info.IsDir() && base[0] == '.' {
		log.Info("Skipping dot file", "path", path)
		return filepath.SkipDir
	}

	// return on non-regular files
	if !info.Mode().IsRegular() {
		return nil
	}

	// Create a tar header
	header, err := tar.FileInfoHeader(info, path)
	if err != nil {
		return err
	}

	// Adjust header name if necessary (e.g., relative paths)
	relPath, err := filepath.Rel(strip, path)
	if err != nil {
		return err
	}
	if destPrefix != "" {
		relPath = filepath.Join(destPrefix, relPath)
	}
	header.Name = filepath.ToSlash(relPath)

	// Write header to the archive
	err = tw.WriteHeader(header)
	if err != nil {
		return err
	}

	// Only write file contents for regular files
	log.Info("Writing tarball entry", "header", header.Name, "path", path)
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer file.Close()

	// Copy file contents to the archive
	_, err = io.Copy(tw, file)
	if err != nil {
		return err
	}

	return nil
}
