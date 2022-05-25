package gitutil

import (
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"
)

func Test_LocateRoot(t *testing.T) {
	type testCase struct {
		name   string
		dirs   []string
		isRepo bool
		input  string
	}

	cases := []testCase{
		{
			name:   "subdirectory",
			dirs:   []string{filepath.Join("top", "middle")},
			isRepo: true,
			input:  filepath.Join("top", "middle"),
		},
		{
			name:   "root",
			dirs:   []string{filepath.Join("top", "middle")},
			isRepo: true,
			input:  filepath.Join(""),
		},
		{
			name:   "not a repo",
			dirs:   []string{filepath.Join("top", "middle")},
			isRepo: false,
			input:  filepath.Join("top", "middle"),
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			outDir, err := ioutil.TempDir("", "LocateRoot")
			if err != nil {
				t.Errorf("Failed to create temporary directory; %v", err)
				return
			}
			if c.isRepo {
				if err := os.MkdirAll(filepath.Join(outDir, ".git"), 0o777); err != nil {
					t.Fatalf("Could not create .git directory %v", filepath.Join(outDir, ".git"))
				}
			}
			for _, d := range c.dirs {
				target := filepath.Join(outDir, d)
				if err := os.MkdirAll(target, 0o777); err != nil {
					t.Fatalf("Could not create .git directory %v", target)
				}
			}

			expected := outDir

			if !c.isRepo {
				expected = ""
			}

			actual, err := LocateRoot(filepath.Join(outDir, c.input))
			if err != nil {
				t.Fatalf("LocateRoot returned error: %v", err)
			}

			if actual != expected {
				t.Errorf("Got %v; want %v", actual, expected)
			}
		})
	}
}
