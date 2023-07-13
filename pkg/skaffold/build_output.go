package skaffold

// Code vendored from https://github.com/GoogleContainerTools/skaffold/blob/main/cmd/skaffold/app/flags/build_output.go

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
)

// BuildOutputFileFlag describes a flag which contains a BuildOutput.
type BuildOutputFileFlag struct {
	filename    string
	buildOutput BuildOutput
}

// BuildOutput is the output of `skaffold build`.
type BuildOutput struct {
	Builds []Artifact `json:"builds"`
}

func (t *BuildOutputFileFlag) String() string {
	return t.filename
}

// Usage Implements Usage() method for pflag interface
func (t *BuildOutputFileFlag) Usage() string {
	return "Input file with json encoded BuildOutput e.g.`skaffold build -q -o >build.out`"
}

// Set Implements Set() method for pflag interface
func (t *BuildOutputFileFlag) Set(value string) error {
	var (
		buf []byte
		err error
	)

	if value == "-" {
		buf, err = ioutil.ReadAll(os.Stdin)
	} else {
		if _, err := os.Stat(value); os.IsNotExist(err) {
			return err
		}
		buf, err = ioutil.ReadFile(value)
	}
	if err != nil {
		return err
	}

	buildOutput, err := ParseBuildOutput(buf)
	if err != nil {
		return fmt.Errorf("setting template flag: %w", err)
	}

	t.filename = value
	t.buildOutput = *buildOutput
	return nil
}

// Type Implements Type() method for pflag interface
func (t *BuildOutputFileFlag) Type() string {
	return fmt.Sprintf("%T", t)
}

// BuildArtifacts returns the Build Artifacts in the BuildOutputFileFlag
func (t *BuildOutputFileFlag) BuildArtifacts() []Artifact {
	return t.buildOutput.Builds
}

// NewBuildOutputFileFlag returns a new BuildOutputFile without any validation
func NewBuildOutputFileFlag(value string) *BuildOutputFileFlag {
	return &BuildOutputFileFlag{
		filename: value,
	}
}

// ParseBuildOutput parses BuildOutput from bytes
func ParseBuildOutput(b []byte) (*BuildOutput, error) {
	buildOutput := &BuildOutput{}
	if err := json.Unmarshal(b, buildOutput); err != nil {
		return nil, err
	}
	return buildOutput, nil
}
