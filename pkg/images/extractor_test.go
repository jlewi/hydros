package images

import (
	"bytes"
	"os"
	"path"
	"sort"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/jlewi/hydros/api/v1alpha1"
	"github.com/jlewi/hydros/pkg/util"
	"gopkg.in/yaml.v3"
)

func Test_extract(t *testing.T) {
	log := util.SetupLogger("info", true)

	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("Error getting current directory; error %v", err)
	}

	testDir := path.Join(cwd, "test_data")

	e := Extractor{
		Log: log,
	}

	output := bytes.NewBufferString("")
	if err := e.Extract(testDir, output); err != nil {
		t.Fatalf("Running extract failed; error %v", err)
	}

	decoder := yaml.NewDecoder(output)

	actual := &v1alpha1.ImageList{}

	if err := decoder.Decode(actual); err != nil {
		t.Fatalf("Failed to decode ImageList; error %v", err)
	}

	expected := &v1alpha1.ImageList{
		APIVersion: "primer.ai.hydros/v1alpha1",
		Kind:       "ImageList",
		Images: []string{
			"cronjob-image",
			"kafka-image-1",
			"kafka-image-2",
			"kafka-image-3",
			"kafka-image-4",
			"kafka-image-5",
			"nginx:1.14.2",
			"nginx:1.15.2",
			"nginx:1.16.2",
			"redis-image-1",
			"redis-image-2",
			"redis-cluster-image-1",
			"redis-cluster-image-2",
		},
	}
	sort.Strings(expected.Images)

	// Sort the actual values because the order returned by the code isn't stable.
	sort.Strings(actual.Images)
	d := cmp.Diff(expected, actual)

	if d != "" {
		t.Errorf("Expected didn't match actual; diff\n%v", d)
	}
}
