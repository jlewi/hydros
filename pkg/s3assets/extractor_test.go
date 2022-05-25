package s3assets

import (
	"bytes"
	"os"
	"path"
	"testing"

	"github.com/stretchr/testify/assert"
	"gopkg.in/yaml.v3"

	"github.com/PrimerAI/hydros-public/api/v1alpha1"
	"github.com/PrimerAI/hydros-public/pkg/util"
)

func TestExtract(t *testing.T) {
	log := util.SetupLogger("info", true)

	cwd, err := os.Getwd()
	assert.NoError(t, err, "Error getting current directory")
	testDir := path.Join(cwd, "test_data")

	e := Extractor{Log: log}
	actual, err := e.Extract(testDir)
	assert.NoError(t, err, "Running extract failed")

	expected := []string{"s3://our-bucket/key/model.zip", "s3://test-bucket/key/fake-model.zip"}

	assert.Equal(t, expected, actual)
}

func TestWrite(t *testing.T) {
	input := []string{"s3://path/fake-key", "s3://other/faker-key"}
	output := &bytes.Buffer{}
	err := Write(input, output)
	assert.NoError(t, err, "Writing extract failed")

	actual := &v1alpha1.S3AssetsList{}
	err = yaml.Unmarshal(output.Bytes(), actual)
	assert.NoError(t, err, "Failed to decode ImageList")

	expected := &v1alpha1.S3AssetsList{
		APIVersion: "primer.ai.hydros/v1alpha1",
		Kind:       "S3AssetsList",
		S3Assets:   input,
	}
	assert.Equal(t, expected, actual)
}
