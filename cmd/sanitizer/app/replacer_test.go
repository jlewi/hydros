package app

import (
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
)

func Test_replacer(t *testing.T) {
	type testCase struct {
		fileName    string
		replacement Replacement
		unclean     string
		expected    string
	}

	cases := []testCase{
		{
			fileName: "basic.go",
			replacement: Replacement{
				Find:     "github.com/old/package",
				Replace:  "github.com/new/package",
				FileGlob: "*.go",
			},
			unclean: `
"github.com/google/go-cmp/cmp"
"github.com/old/package"
"github.com/other/package"
`,
			expected: `
"github.com/google/go-cmp/cmp"
"github.com/new/package"
"github.com/other/package"
`,
		},
		{
			fileName: "regex.go",
			replacement: Replacement{
				Find:     ".*keyword",
				Replace:  "newtext",
				FileGlob: "*.go",
			},
			unclean: `
someverylongkeyword
"github.com/old/package"
"github.com/other/package"
`,
			expected: `
newtext
"github.com/old/package"
"github.com/other/package"
`,
		},
	}

	for _, c := range cases {
		t.Run(c.fileName, func(t *testing.T) {
			rep, err := newReReplacer(c.replacement)
			if err != nil {
				t.Fatalf("Failed to create regex replacer; error %v", err)
			}
			lines := strings.Split(c.unclean, "\n")
			actual, err := rep.replaceLines(c.fileName, lines)
			if err != nil {
				t.Errorf("replaceLines failed; %v", err)
			}
			expected := strings.Split(c.expected, "\n")
			if d := cmp.Diff(expected, actual); d != "" {
				t.Errorf("replaced lined didn't match;diff:\n%v", d)
			}
		})
	}
}
