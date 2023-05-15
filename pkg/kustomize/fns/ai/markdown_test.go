package ai

import (
	"testing"

	"github.com/google/go-cmp/cmp"
)

type TestYAMLObject struct {
	APIVersion string `yaml:"apiVersion"`
	Kind       string `yaml:"kind"`
	SomeField  string `yaml:"someField"`
}

func Test_MarkdownToYAML(t *testing.T) {
	type testCase struct {
		name     string
		in       string
		expected []TestYAMLObject
	}

	cases := []testCase{
		{
			name: "basic",
			in: "pre text\n```yaml\n" +
				`apiVersion: v1
kind: Pod
someField: abcd
` + "```\npost text",

			expected: []TestYAMLObject{
				{
					APIVersion: "v1",
					Kind:       "Pod",
					SomeField:  "abcd",
				},
			},
		},
		{
			name: "no-language-yaml",
			in: "```\n" +
				`apiVersion: v1
kind: Pod
someField: abcd
` + "```",

			expected: []TestYAMLObject{
				{
					APIVersion: "v1",
					Kind:       "Pod",
					SomeField:  "abcd",
				},
			},
		},
		{
			name: "no-language-not-yaml",
			in: "```\n" +
				`import os
def function():
  print("hello world")
` + "```",

			expected: []TestYAMLObject{},
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			actualNodes, err := MarkdownToYAML(c.in)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			actual := make([]TestYAMLObject, 0, len(actualNodes))

			for _, n := range actualNodes {
				yN := n.YNode()
				o := &TestYAMLObject{}
				if err := yN.Decode(o); err != nil {
					t.Fatalf("Failed to decode node; error %v", err)
				}
				actual = append(actual, *o)
			}

			if d := cmp.Diff(c.expected, actual); d != "" {
				t.Errorf("unexpected diff: %v", d)
			}
		})
	}
}
