package app

import (
	"fmt"
	"io/ioutil"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/jlewi/hydros/pkg/util"

	"github.com/go-logr/logr"
	"github.com/pkg/errors"
)

// Explicitly skip copying the git directories.
var skippedDir = map[string]bool{
	".git":        true,
	".gitmodules": true,
}

// Sanitizer is used to prepare code for open source.
type Sanitizer struct {
	log    logr.Logger
	config Sanitize

	unsafeRe     []*regexp.Regexp
	fileCleaners *cleaners
	replacements []*reReplacer
}

// New creates a new Sanitizer
func New(config Sanitize, log logr.Logger) (*Sanitizer, error) {
	unsafeRe := make([]*regexp.Regexp, len(config.UnsafeRegex))

	for i, s := range config.UnsafeRegex {
		var err error
		unsafeRe[i], err = regexp.Compile(s)
		if err != nil {
			return nil, errors.Wrapf(err, "failed to compile regexp: %v", s)
		}
	}

	replacements := make([]*reReplacer, len(config.Replacements))

	for i, r := range config.Replacements {
		var err error
		replacements[i], err = newReReplacer(r)
		if err != nil {
			return nil, errors.Wrapf(err, "failed to create regex replacer")
		}
	}

	fileCleaners, err := newCleaners()
	if err != nil {
		return nil, errors.Wrapf(err, "failed to create file cleaners")
	}

	return &Sanitizer{
		log:          log,
		config:       config,
		unsafeRe:     unsafeRe,
		fileCleaners: fileCleaners,
		replacements: replacements,
	}, nil
}

// Run sanitizes the provided directory
func (s *Sanitizer) Run(source string, dest string) error {
	log := s.log

	// We copy each toplevel directory separately.
	// This way we can skip over entire directories.
	// This is more efficient (and easier) then copying the full
	// directory and then trying to exclude all files in that directory.
	excludeFiles := map[string]bool{}

	for _, f := range s.config.Remove {
		excludeFiles[filepath.Join(source, f)] = true
	}

	for k := range skippedDir {
		excludeFiles[filepath.Join(source, k)] = true
		log.Info("Adding directory to list of excludes", "dir", filepath.Join(source, k))
	}

	topFiles, err := ioutil.ReadDir(source)
	if err != nil {
		return errors.Wrapf(err, "failed to read directory: %v", source)
	}

	log.Info("Deleting exsiting directories in the destination", "destination", dest)
	for _, f := range topFiles {
		// skippedDir are a list of directories in the target we don't want to delete like .git
		if ok := skippedDir[f.Name()]; ok {
			log.Info("Special directory will not be deleted", "dir", f.Name())
			continue
		}

		fullDest := filepath.Join(dest, f.Name())
		err := os.RemoveAll(fullDest)
		if err != nil {
			log.Error(err, "Failed to delete directory: %v", fullDest)
		}
	}

	unsafeFiles := []string{}
	err = filepath.Walk(source, func(path string, info os.FileInfo, pathError error) error {
		if ok := excludeFiles[path]; ok {
			log.Info("Skipping dir or file", "path", path)
			if info.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}

		rPath, err := filepath.Rel(source, path)
		if err != nil {
			return errors.Wrapf(err, "Could not get relative path of %v to %v", path, source)
		}
		fullSource := filepath.Join(source, rPath)
		fullDest := filepath.Join(dest, rPath)

		if info.IsDir() {
			err := os.MkdirAll(fullDest, info.Mode())
			if err != nil {
				return errors.Wrapf(err, "failed to create directory: %v", fullDest)
			}
			return nil
		}

		err = s.cleanFile(fullSource, fullDest)
		if err != nil {
			if isUnsafeError(err) {
				unsafeFiles = append(unsafeFiles, path)
			} else {
				return err
			}
		}
		return nil
	})

	if err != nil {
		return errors.Wrapf(err, "failed to walk directory: %v", source)
	}

	if len(unsafeFiles) > 0 {
		log.Info("The following files contained expressions which are not allowed in the open source code.", "files", unsafeFiles)
		return fmt.Errorf("files contained expressions not allowed in the open source code")
	}
	for _, d := range s.config.Remove {
		path := path.Join(dest, d)
		log.Info("Removing target", "target", path)
		err := os.RemoveAll(path)
		if err != nil {
			log.Error(err, "Failed to remove target", "target", path)
		}
	}

	return nil
}

type unsafeError struct {
	path string
}

func (e *unsafeError) Error() string {
	return fmt.Sprintf("File %v contains expressions which can't be included in the open source code", e.path)
}

func isUnsafeError(e error) bool {
	_, ok := e.(*unsafeError)
	return ok
}

func (s *Sanitizer) cleanFile(path string, dest string) error {
	s.log.Info("Cleaning target", "target", path)
	b, err := ioutil.ReadFile(path)
	if err != nil {
		return errors.Wrapf(err, "Failed to clean file; %v", path)
	}

	original := strings.Split(string(b), "\n")

	c := s.fileCleaners.getCleaner(path)

	cleaned := original
	if c != nil {
		s.log.V(util.Debug).Info("Cleaner registered for file", "path", path, "cleaner", c.name)
		var err error
		cleaned, err = c.removeSanitized(original)
		if err != nil {
			return errors.Wrapf(err, "failed to clean file: %v", path)
		}
	} else {
		s.log.V(util.Debug).Info("No cleaner registered for file", "path", path)
	}

	name := filepath.Base(path)
	for _, c := range s.replacements {
		cleaned, err = c.replaceLines(name, cleaned)
		if err != nil {
			return errors.Wrapf(err, "Failed to apply replacement to file: %v; regex: %v", path, c.find.String())
		}
	}

	if !s.checkNoUnsafeLines(path, cleaned) {
		return &unsafeError{
			path: path,
		}
	}

	finfo, err := os.Stat(path)
	if err != nil {
		return errors.Wrapf(err, "Failed to stat file; %v", path)
	}
	err = ioutil.WriteFile(dest, []byte(strings.Join(cleaned, "\n")), finfo.Mode())
	if err != nil {
		return errors.Wrapf(err, "Failed to write file; %v", path)
	}

	return nil
}

// checkNoUnsafeLines verifies the file doesn't contain any unsafe lines
func (s *Sanitizer) checkNoUnsafeLines(path string, lines []string) bool {
	result := true
	for _, m := range s.unsafeRe {
		for _, l := range lines {
			if m.MatchString(l) {
				result = false
				s.log.Info("File contains unsafe text", "file", path, "regex", m.String())
			}
		}
	}
	return result
}
