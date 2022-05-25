package util

import (
	"strings"

	"github.com/go-logr/zapr"
	"go.uber.org/zap"
)

// ListOfErrors is used when we want to return more then one error.
// This happens when we want to keep going and accumulate errors
type ListOfErrors struct {
	Causes []error
	Final  error
}

// Error returns a single error wrapping all the errors.
func (l *ListOfErrors) Error() string {
	m := l.Final.Error() + "; Causes: "

	c := []string{}
	for _, i := range l.Causes {
		c = append(c, i.Error())
	}

	m = m + strings.Join(c, ",")
	return m
}

// AddCause adds an error to the list.
func (l *ListOfErrors) AddCause(e error) {
	l.Causes = append(l.Causes, e)
}

// IgnoreError is a helper function to deal with errors.
func IgnoreError(err error) {
	if err == nil {
		return
	}
	log := zapr.NewLogger(zap.L())
	log.Error(err, "Unexpected error occurred")
}

// DeferIgnoreError is a helper function to ignore errors returned by functions called with defer.
func DeferIgnoreError(f func() error) {
	IgnoreError(f())
}
