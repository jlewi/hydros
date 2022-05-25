package app

import (
	"path/filepath"
	"regexp"

	"github.com/pkg/errors"
)

// reReplacer applies a regex replacement to a matching file
type reReplacer struct {
	find    *regexp.Regexp
	replace string
	glob    string
}

func newReReplacer(r Replacement) (*reReplacer, error) {
	rep := &reReplacer{
		replace: r.Replace,
		glob:    r.FileGlob,
	}
	var err error
	rep.find, err = regexp.Compile(r.Find)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to compile regexp: %v", r.Find)
	}
	return rep, nil
}

func (r *reReplacer) replaceLines(fileName string, lines []string) ([]string, error) {
	if r.glob != "" {
		isMatch, err := filepath.Match(r.glob, fileName)
		if err != nil {
			return lines, errors.Wrapf(err, "Could not apply glob %v to file %v", r.glob, fileName)
		}
		if !isMatch {
			return lines, nil
		}
	}

	for i, l := range lines {
		lines[i] = r.find.ReplaceAllString(l, r.replace)
	}
	return lines, nil
}
