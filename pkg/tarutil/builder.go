package tarutil

import (
	"archive/tar"
	"compress/gzip"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/bmatcuk/doublestar/v4"
	"github.com/go-logr/zapr"
	"github.com/jlewi/hydros/api/v1alpha1"
	"github.com/jlewi/hydros/pkg/files"
	"github.com/jlewi/hydros/pkg/util"
	mutil "github.com/jlewi/monogo/util"
	"github.com/pkg/errors"
	"go.uber.org/zap"
)

// TarSource describes the source to include in a tarball
//
// TODO(jeremy): Should we directly support using docker images as a source?
// Currently we don't because the code to export images to a tarball is in the images package. If we moved that
// code into the tarutil package then we could directly support using docker images as a source but that would
// create a circular dependency.
type TarSource struct {
	// Path is the path to resolve against.
	// It can be a filesystem path in which case sources are matched relative to that path
	// It can be a tarball in which case sources are matched against the contents of the tarball
	Path   string
	Source []*v1alpha1.Source
}

// Build builds an archive from the manifest
// basePath is the basePath to resolve relative paths against
// tarball is the path to the tarball to create
// fileSource is a list of files to include in the tarball
// tarSource is a list of tarballs and corresponding matches to include
func Build(tarSources []*TarSource, tarFilePath string) error {
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
	defer mutil.MaybeClose(w)

	// Create a new tarutil archive
	log.Info("Creating tarball", "tarFilePath", tarFilePath)

	// If you want GZIP compression, wrap the tarutil writer in a gzip writer
	gzWriter := gzip.NewWriter(w)
	defer gzWriter.Close()

	// Create a tarutil writer
	tw := tar.NewWriter(gzWriter)
	defer tw.Close()

	// Currently copyTarball doesn't support compressed tarballs
	tarSuffixes := []string{".tar"}

	for _, s := range tarSources {

		isTar := false

		for _, suffix := range tarSuffixes {
			if strings.HasSuffix(s.Path, suffix) {
				isTar = true
				break
			}
		}

		if isTar {
			log.Info("Adding tarball", "tarball", s.Path, "pattern", s.Source)
			if err := copyTarBall(tw, s); err != nil {
				log.Error(err, "Error copying tarball", "tarball", s.Path, "source", s.Source)
				return err
			}
			continue
		} else {
			log.Info("Adding base path", "path", s.Path, "pattern", s.Source)
			if err := copyBasePath(tw, s); err != nil {
				log.Error(err, "Error copying base path", "path", s.Path, "pattern", s.Source)
				return err
			}
		}

	}
	return nil
}

func copyBasePath(tw *tar.Writer, s *TarSource) error {
	log := zapr.NewLogger(zap.L())
	for _, a := range s.Source {
		log.Info("Adding asset", "asset", a)
		sBase := s.Path
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

// copyTarball copies assets in the source tarbell matching the glob to the destination tarball
// glob is a glob pattern to match against the tarball
// strip is a path prefix to strip from all paths
// destPrefix is a path prefix to add to all paths
func copyTarBall(tw *tar.Writer, s *TarSource) error {
	log := zapr.NewLogger(zap.L())
	// Open the tarball file
	file, err := os.Open(s.Path)
	if err != nil {
		return errors.Wrapf(err, "Error opening tarball %v", s.Path)
	}
	defer file.Close()

	// Create a tar reader
	tarReader := tar.NewReader(file)

	// Iterate over each file in the tarball
	for {
		header, err := tarReader.Next()

		if err == io.EOF {
			// Reached the end of the tarball
			return nil
		}

		if err != nil {
			return errors.Wrapf(err, "Error reading tar header:")
		}

		// Check if any of the patterns match
		var source *v1alpha1.Source
		for _, s := range s.Source {
			isMatch, err := doublestar.Match(s.Src, header.Name)
			if err != nil {
				return errors.Wrapf(err, "Error matching glob %v against %v", s.Src, header.Name)
			}

			if isMatch {
				source = s
				break
			}
		}

		if source == nil {
			log.V(util.Debug).Info("Skipping file because it doesn't match any source globs", "file", header.Name)
			continue
		}

		log.Info("Reading tarball entry", "header", header.Name, "size", header.Size)

		path := header.Name
		if source.Strip != "" {
			newPath, err := filepath.Rel(source.Strip, header.Name)
			if err != nil {
				// Keep going
				log.Error(err, "Error stripping prefix", "prefix", source.Strip, "path", header.Name)
			} else {
				path = newPath
			}
		}

		if source.Dest != "" {
			path = filepath.Join(source.Dest, path)
		}

		// Create a tar header
		newHeader := header
		newHeader.Name = path

		if err := tw.WriteHeader(newHeader); err != nil {
			return errors.Wrapf(err, "Error writing tar header: %v", newHeader.Name)
		}

		// We create headers for empty files but since the size is 0 we don't copy any bytes.
		if header.Size == 0 {
			continue
		}
		// Read the file contents
		_, err = io.CopyN(tw, tarReader, header.Size)
		if err != nil {
			return errors.Wrapf(err, "Error reading file contents")
		}
	}
}

// splitIntoParent splits a path into a parent and glob
// e.g. ../foo/bar/*.txt -> ../foo/bar, *.txt
// doublestar.Glob has a comment about a function SplitPattern that we could potentially use
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
	// Clean resolves ".." and "." in the path
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
