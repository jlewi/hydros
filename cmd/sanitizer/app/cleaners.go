package app

import (
	"path/filepath"
	"regexp"

	"github.com/go-logr/zapr"
	"github.com/pkg/errors"
	"go.uber.org/zap"
)

// lineCleaner looks for markers e.g. sanitizer:begin and sanitizer:end
// Note it should be '+sanitizer' but the plus sign is excluded from the comment to avoid the comment with the
// marker being stripped out.
// and removes the lines from the files.
type lineCleaner struct {
	name  string
	begin *regexp.Regexp
	end   *regexp.Regexp
}

func newLineCleaner(name string, begin string, end string) (*lineCleaner, error) {
	beginRe, err := regexp.Compile(begin)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to compile begin regexp: %v", begin)
	}

	endRe, err := regexp.Compile(end)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to compile end regexp: %v", end)
	}

	return &lineCleaner{
		name:  name,
		begin: beginRe,
		end:   endRe,
	}, nil
}

func (c *lineCleaner) removeSanitized(original []string) ([]string, error) {
	final := make([]string, 0, len(original))
	index := 0
	for index < len(original) {
		l := original[index]
		index++
		if !c.begin.MatchString(l) {
			final = append(final, l)
			continue
		}
		// Loop until we find the end expression or run out lines
		for index < len(original) {
			l := original[index]
			index++
			if c.end.MatchString(l) {
				break
			}
		}
		if index >= len(original) {
			return final, errors.Errorf("Didn't find closing marker %v", c.end.String())
		}
	}

	return final, nil
}

// cleaners contains rules for dispatching to various cleaners based on file.
type cleaners struct {
	// Cleaner files that use <!-- comment -->; i.e. html markdown
	htmlCleaner *lineCleaner

	// Cleaner for files that use // for comments i.e. go
	goCleaner *lineCleaner

	// Cleaner for files that use # for comments
	hashCleaner *lineCleaner
}

func newCleaners() (*cleaners, error) {
	c := &cleaners{}

	var err error
	c.goCleaner, err = newLineCleaner("goCleaner", `.*//.*[+]sanitizer:begin.*`, `.*//.*[+]sanitizer:end.*`)

	if err != nil {
		return nil, errors.Wrapf(err, "failed to create goCleaner")
	}

	c.htmlCleaner, err = newLineCleaner("htmlCleaner", `<!--.*[+]sanitizer:begin.*-->`, `<!--.*[+]sanitizer:end.*--`)

	if err != nil {
		return nil, errors.Wrapf(err, "failed to create htmlCleaner")
	}

	c.hashCleaner, err = newLineCleaner("hashCleaner", `#.*[+]sanitizer:begin.*`, `#.*[+]sanitizer:end.*`)

	if err != nil {
		return nil, errors.Wrapf(err, "failed to create hashCleaner")
	}
	return c, nil
}

// getCleaner returns the appropriate cleaner based on the filename
// or nil if no cleaner is registered for that filename.
func (c *cleaners) getCleaner(fileName string) *lineCleaner {
	if isFileMatch(fileName, []string{"*.yaml", "*.yml", "Makefile", "*.py", "Dockerfile*"}) {
		return c.hashCleaner
	}
	if isFileMatch(fileName, []string{"*.md", "*.html"}) {
		return c.htmlCleaner
	}
	if isFileMatch(fileName, []string{"*.go"}) {
		return c.goCleaner
	}
	return nil
}

func isFileMatch(fileName string, globs []string) bool {
	log := zapr.NewLogger(zap.L())
	for _, g := range globs {
		if match, err := filepath.Match(g, filepath.Base(fileName)); err != nil {
			log.Error(err, "filepath.match failed", "file", fileName, "glob", g)
		} else if match {
			return true
		}
	}
	return false
}
