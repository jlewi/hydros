package tarutil

import (
	"archive/tar"
	"compress/gzip"
	"github.com/bmatcuk/doublestar/v4"
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

	// Create a new tarutil archive
	log.Info("Creating tarball", "basePath", basePath, "tarFilePath", tarFilePath)

	// If you want GZIP compression, wrap the tarutil writer in a gzip writer
	gzWriter := gzip.NewWriter(w)
	defer gzWriter.Close()

	// Create a tarutil writer
	tw := tar.NewWriter(gzWriter)
	defer tw.Close()

	dirFS := os.DirFS(basePath)
	for _, a := range image.Spec.Source {
		log.Info("Adding asset", "asset", a)

		matches, err := doublestar.Glob(dirFS, a.Src)
		if err != nil {
			log.Error(err, "Failed to search glob", "glob", a.Src, "basePath", basePath)
			return err
		}
		log.Info("Matched glob", "glob", a.Src, "numMatches", len(matches), "basePath", basePath)
		for _, m := range matches {
			if err := addFileToTarGenerator(tw, basePath, m, a.Strip, a.Dest); err != nil {
				log.Error(err, "Error adding file to tarball", "file", m, "basePath", basePath, "strip", a.Strip, "dest", a.Dest)
				return err
			}
		}

	}
	return nil
}

// addFileToTarGenerator adds a file to the tarball
// fs should be a filesystem rooted at the base directory
// path should be relative to basePath
func addFileToTarGenerator(tw *tar.Writer, basePath string, path string, strip string, destPrefix string) error {
	log := zapr.NewLogger(zap.L())

	fullPath := filepath.Join(basePath, path)
	info, err := os.Stat(fullPath)
	if err != nil {
		return err
	}
	if info.IsDir() {
		log.Info("Skipping directory", "path", fullPath)
		return filepath.SkipDir
	}

	// return on non-regular files
	if !info.Mode().IsRegular() {
		log.Info("Skipping not regular path", "path", fullPath)
		return nil
	}

	// Create a tarutil header
	header, err := tar.FileInfoHeader(info, fullPath)
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
	file, err := os.Open(fullPath)
	if err != nil {
		return errors.Wrapf(err, "Failed to openfile %v", fullPath)
	}
	defer file.Close()

	// Copy file contents to the archive
	_, err = io.Copy(tw, file)
	if err != nil {
		return err
	}

	return nil
}
