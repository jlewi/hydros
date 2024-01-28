package tarutil

import (
	"archive/tar"
	"compress/gzip"
	"fmt"
	"github.com/google/go-containerregistry/pkg/crane"
	v1 "github.com/google/go-containerregistry/pkg/v1"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/bmatcuk/doublestar/v4"
	"github.com/go-logr/zapr"
	"github.com/jlewi/hydros/api/v1alpha1"
	"github.com/jlewi/hydros/pkg/files"
	"github.com/jlewi/monogo/util"
	"github.com/pkg/errors"
	"go.uber.org/zap"
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

	//if err := addDockerImages(tw, image); err != nil {
	//	return err
	//}

	for _, a := range image.Spec.Source {
		log.Info("Adding asset", "asset", a)

		sBase := basePath
		// We need to adjust the basepath if we have a relative path
		parent, glob := splitIntoParent(a.Src)

		if parent != "" {
			sBase = filepath.Clean(filepath.Join(sBase, parent))
		}

		// Match the glob
		// matchGlob can handle globs with ../. However DirFs returns a filesystem rooted at the directory
		// so we need to adjust the glob so that all paths occur under the directory used as the root of the DirFs
		fs := os.DirFS(sBase)
		matches, err := matchGlob(fs, glob)
		if err != nil {
			log.Error(err, "Failed to search glob", "glob", a.Src, "basePath", sBase)
			return err
		}
		log.Info("Matched glob", "glob", a.Src, "numMatches", len(matches), "basePath", sBase)
		for _, m := range matches {
			if err := addFileToTarGenerator(tw, sBase, m, a.Strip, a.Dest); err != nil {
				log.Error(err, "Error adding file to tarball", "file", m, "basePath", sBase, "strip", a.Strip, "dest", a.Dest)
				return err
			}
		}

	}
	return nil
}

// addDockerImages adds assets from docker images to the tarball
//func addDockerImages(tw *tar.Writer, image *v1alpha1.Image) error {
//	log := zapr.NewLogger(zap.L())
//	for _, a := range image.Spec.ImageSource {
//
//	}
//	return nil
//}

// DownloadImage uses crane to download an image to a tarball
// It is basically the same code as crane export
// https://github.com/google/go-containerregistry/blob/a0658aa1d0cc7a7f1bcc4a3af9155335b6943f40/cmd/crane/cmd/export.go#L55
func DownloadImage(src string, tarFilePath string) error {
	var img v1.Image
	desc, err := crane.Get(src)
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
			return fmt.Errorf("pulling Image %s: %w", src, err)
		}
	}

	f, err := os.Create(tarFilePath)
	if err != nil {
		return err
	}
	defer f.Close()
	return crane.Export(img, f)
}

// splitIntoParent splits a path into a parent and glob
// e.g. ../foo/bar/*.txt -> ../foo/bar, *.txt
func splitIntoParent(path string) (string, string) {
	pieces := strings.Split(path, string(filepath.Separator))

	index := 0
	for ; index < len(pieces); index++ {
		if pieces[index] != ".." {
			break
		}
	}

	parent := filepath.Join(pieces[:index]...)
	glob := filepath.Join(pieces[index:]...)
	return parent, glob
}

// matchGlob matches a glob against a filesystem
// It supports ** and ../
func matchGlob(dirFS fs.FS, glob string) ([]string, error) {
	glob = filepath.Clean(glob)
	return doublestar.Glob(dirFS, glob)
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
		return nil
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
