package util

import (
	"context"
	"fmt"
	"path/filepath"
	"runtime"

	"github.com/ghodss/yaml"
	"github.com/go-logr/logr"
	"github.com/go-logr/zapr"
	"go.uber.org/zap"
)

const (
	// Constants to be used with verbosity levels with logr.
	// Note that with logr the verbosity is additive
	// e.g. log.V(1).Info() means log at verbosity = info verbosity + 1

	// Debug indicates debug verbosity level
	Debug = 1
)

// VerbosityDescription returns a string to be used as the description for verbosity flags.
func VerbosityDescription() string {
	return fmt.Sprintf("Verbosity error=%v warning=%v info=%v, debug=%v", -1*int(zap.ErrorLevel), -1*int(zap.WarnLevel), -1*int(zap.InfoLevel), -1*int(zap.DebugLevel))
}

// SetupLogger performs common setup of a logger.
func SetupLogger(level string, devLogger bool) logr.Logger {
	// Start with a production logger config.
	config := zap.NewProductionConfig()

	if devLogger {
		config = zap.NewDevelopmentConfig()
	}

	// Increment the logging level.
	l := zap.NewAtomicLevel()
	err := l.UnmarshalText([]byte(level))
	if err != nil {
		panic(fmt.Sprintf("Could not convert level=%v to a ZapCorLevel; error: %v", level, err))
	}
	config.Level = l

	zapLog, err := config.Build()
	if err != nil {
		panic(fmt.Sprintf("Could not create zap instance (%v)?", err))
	}

	// replace the global logger
	zap.ReplaceGlobals(zapLog)

	return zapr.NewLogger(zapLog)
}

func getFrame(skipFrames int) runtime.Frame {
	// We need the frame at index skipFrames+2, since we never want runtime.Callers and getFrame
	targetFrameIndex := skipFrames + 2

	// Set size to targetFrameIndex+2 to ensure we have room for one more caller than we need
	programCounters := make([]uintptr, targetFrameIndex+2)
	n := runtime.Callers(0, programCounters)

	frame := runtime.Frame{Function: "unknown"}
	if n > 0 {
		frames := runtime.CallersFrames(programCounters[:n])
		for more, frameIndex := true, 0; more && frameIndex <= targetFrameIndex; frameIndex++ {
			var frameCandidate runtime.Frame
			frameCandidate, more = frames.Next()
			if frameIndex == targetFrameIndex {
				frame = frameCandidate
			}
		}
	}

	return frame
}

// MyCaller returns the caller of the Function in the form ${package}/${file}:${linenumber}
// e.g.
// file: somemodule/cmd/parent.go
//
//	func parent() {
//	  child()
//	}
//
// file: somemodule/cmd/child.go
//
//	func child() {
//	  c := util.MyCaller()
//	}
//
// The value of c will be "cmd/parent.go:2"
//
// This is useful when you have common error handlers that want to log the location that invoked them.
//
// The function doesn't return the full path of the code because this would be the full path on the machine
// where it was compiled which isn't useful.
func MyCaller() string {
	// Skip GetCallerFunctionName and the function to get the caller of
	frame := getFrame(2)

	fullPackage, fileName := filepath.Split(frame.File)
	packageName := filepath.Base(fullPackage)

	return fmt.Sprintf("%v/%v:%v", packageName, fileName, frame.Line)
}

// PrettyString returns a prettily formatted string of the object.
func PrettyString(v interface{}) string {
	p, err := yaml.Marshal(v)
	if err != nil {
		return fmt.Sprintf("PrettyString returned error; %v", err)
	}
	return string(p)
}

// LogFromContext returns a logr.Logger from the context or an instance of the global logger
func LogFromContext(ctx context.Context) logr.Logger {
	l, err := logr.FromContext(ctx)
	if err != nil {
		return zapr.NewLogger(zap.L())
	}
	return l
}
