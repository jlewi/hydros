package skaffold

import (
	"os"
	"path"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/jlewi/hydros/pkg/util"
)

func Test_LoadSkaffoldConfigs(t *testing.T) {
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("Error getting current directory; error %v", err)
	}

	testDir := path.Join(cwd, "test_data")

	log := util.SetupLogger("debug", true)

	configs, err := LoadSkaffoldConfigs(log, testDir, nil, []string{})
	if err != nil {
		t.Errorf("LoadSkaffoldConfigs returned error; %v", err)
	}

	expectedCount := 2
	if len(configs) != expectedCount {
		t.Errorf("len(configs); Got %v; Want %v", len(configs), expectedCount)
	}
}

func Test_LoadSkaffoldConfigs_Skip(t *testing.T) {
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("Error getting current directory; error %v", err)
	}

	testDir := path.Join(cwd, "test_data")

	log := util.SetupLogger("debug", true)

	configs, err := LoadSkaffoldConfigs(log, testDir, nil, []string{"skip_data"})
	if err != nil {
		t.Errorf("LoadSkaffoldConfigs returned error; %v", err)
	}

	expectedCount := 1
	if len(configs) != expectedCount {
		t.Errorf("len(configs); Got %v; Want %v", len(configs), expectedCount)
	}
}

func TestChangeRegistry(t *testing.T) {
	type testCase struct {
		Name     string
		Config   *SkaffoldConfig
		Expected *SkaffoldConfig
		Registry string
	}

	testCases := []testCase{
		{
			Name: "basic",
			Config: &SkaffoldConfig{
				Pipeline: Pipeline{
					Build: BuildConfig{
						Artifacts: []*Artifact{
							{
								ImageName: "12345.dkr.ecr.us-west-2.amazonaws.com/hydros/hydros",
							},
							{
								ImageName: "12345.dkr.ecr.us-west-2.amazonaws.com/echo-server",
							},
						},
					},
				},
			},
			Registry: "newregistry",
			Expected: &SkaffoldConfig{
				Pipeline: Pipeline{
					Build: BuildConfig{
						Artifacts: []*Artifact{
							{
								ImageName: "newregistry/hydros/hydros",
							},
							{
								ImageName: "newregistry/echo-server",
							},
						},
					},
				},
			},
		},
		{
			Name: "empty-registry",
			Config: &SkaffoldConfig{
				Pipeline: Pipeline{
					Build: BuildConfig{
						Artifacts: []*Artifact{
							{
								ImageName: "12345.dkr.ecr.us-west-2.amazonaws.com/hydros/hydros",
							},
							{
								ImageName: "12345.dkr.ecr.us-west-2.amazonaws.com/echo-server",
							},
						},
					},
				},
			},
			Registry: "",
			Expected: &SkaffoldConfig{
				Pipeline: Pipeline{
					Build: BuildConfig{
						Artifacts: []*Artifact{
							{
								ImageName: "12345.dkr.ecr.us-west-2.amazonaws.com/hydros/hydros",
							},
							{
								ImageName: "12345.dkr.ecr.us-west-2.amazonaws.com/echo-server",
							},
						},
					},
				},
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.Name, func(t *testing.T) {
			if err := ChangeRegistry(tc.Config, tc.Registry); err != nil {
				t.Errorf("ChangeRegistry failed; error %v", err)
				return
			}
			d := cmp.Diff(tc.Expected, tc.Config)

			if d != "" {
				t.Errorf("Actual didn't match expected; diff:\n%v", d)
			}
		})
	}
}
