package gitutil

import (
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing/format/gitignore"
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

func Test_GitIgnore(t *testing.T) {
	dir, err := os.MkdirTemp("", "testGitignore")
	defer os.RemoveAll(dir)

	if err != nil {
		t.Fatalf("Could not create temp dir %v", err)
	}

	r, err := git.PlainInit(dir, false)
	if err != nil {
		t.Fatalf("Could not initialize git repo %v", err)
	}

	// Creat a .gitignore file
	gitIgnoreContents := `
**/.build
`
	if err := os.WriteFile(filepath.Join(dir, ".gitignore"), []byte(gitIgnoreContents), 0o644); err != nil {
		t.Fatalf("Could not create .gitignore file %v", err)
	}

	// Create a file to be ignored
	if err := os.MkdirAll(filepath.Join(dir, ".build"), 0o777); err != nil {
		t.Fatalf("Could not create .build directory %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, ".build", "notcommited"), []byte("foo"), 0o644); err != nil {
		t.Fatalf("Could not create file %v", err)
	}

	w, err := r.Worktree()
	if err != nil {
		t.Fatalf("Could not get worktree %v", err)
	}

	if err := AddGitignoreToWorktree(w, dir); err != nil {
		t.Fatalf("Failed to add gitignore patterns %v", err)
	}

	if len(w.Excludes) != 1 {
		t.Fatalf("Expected 1 exclude pattern, got %v", len(w.Excludes))
	}

	// N.B. I think the arguments to match is an array of the path parts
	if result := w.Excludes[0].Match([]string{"other", ".build"}, true); result != gitignore.Exclude {
		t.Errorf("Expected Exclude; got %v", result)
	}
}
