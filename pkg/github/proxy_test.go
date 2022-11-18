package github

import (
	"testing"

	"github.com/google/go-cmp/cmp"
)

func Test_parseUrl(t *testing.T) {
	type testCase struct {
		path     string
		expected *target
	}

	testCases := []testCase{
		{
			path: "github.com/SomeOrg/hydros/some/path/file.txt",
			expected: &target{
				Host: "github.com",
				Org:  "SomeOrg",
				Repo: "hydros",
				Dest: "some/path/file.txt",
			},
		},
	}

	for _, c := range testCases {
		actual, err := parseURLPath(c.path)
		if err != nil {
			t.Errorf("Unexpected error; %v", err)
			continue
		}

		d := cmp.Diff(c.expected, actual)

		if d != "" {
			t.Errorf("target didn't match expected:\n%v", d)
		}
	}
}
