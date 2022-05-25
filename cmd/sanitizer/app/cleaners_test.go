package app

import (
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
)

func Test_Cleaners(t *testing.T) {
	type testCase struct {
		fileName    string
		unclean     string
		expected    string
		cleanerName string
	}

	cases := []testCase{
		{
			fileName: "basic.go",
			unclean: `
line1
line2
remaining code
`,
			expected: `
line1
line2
remaining code
`,
			cleanerName: "goCleaner",
		},
		{
			fileName: "lastline.go",
			unclean: `
line1
line2
`,
			expected: `
line1
line2
`,
			cleanerName: "goCleaner",
		},
		{
			fileName: "Readme.md",
			unclean: `
line1
<!-- +sanitizer:begin -->
some
toremove
<!-- +sanitizer:end -->
line2
`,
			expected: `
line1
line2
`,
			cleanerName: "htmlCleaner",
		},
		{
			fileName: "Makefile",
			unclean: `
line1
# +sanitizer:begin
some
toremove
# +sanitizer:end
line2
`,
			expected: `
line1
line2
`,
			cleanerName: "hashCleaner",
		},
		{
			fileName: "Dockerfile",
			unclean: `
line1
# +sanitizer:begin
some
toremove
# +sanitizer:end
line2
`,
			expected: `
line1
line2
`,
			cleanerName: "hashCleaner",
		},
	}

	cleaners, err := newCleaners()
	if err != nil {
		t.Fatalf("Failed to create cleaners; %v", err)
	}
	for _, c := range cases {
		t.Run(c.fileName, func(t *testing.T) {
			cleaner := cleaners.getCleaner(c.fileName)
			if c.cleanerName != cleaner.name {
				t.Fatalf("Expected cleaner %v; got %v", c.cleanerName, cleaner.name)
			}
			lines := strings.Split(c.unclean, "\n")
			actual, err := cleaner.removeSanitized(lines)
			if err != nil {
				t.Errorf("cleanString failed; %v", err)
			}
			expected := strings.Split(c.expected, "\n")
			if d := cmp.Diff(expected, actual); d != "" {
				t.Errorf("Cleaned didn't match;diff:\n%v", d)
			}
		})
	}
}
