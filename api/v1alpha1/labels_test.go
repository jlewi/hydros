package v1alpha1

import (
	"testing"

	"github.com/google/go-cmp/cmp"
	meta "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func Test_LabelSelectorConvert(t *testing.T) {
	type testCase struct {
		input    *LabelSelector
		expected *meta.LabelSelector
	}

	testCases := []testCase{
		{
			input:    &LabelSelector{},
			expected: &meta.LabelSelector{},
		},
		{
			input: &LabelSelector{
				MatchLabels: map[string]string{
					"label1": "val1",
					"label2": "val2",
				},
			},
			expected: &meta.LabelSelector{
				MatchLabels: map[string]string{
					"label1": "val1",
					"label2": "val2",
				},
			},
		},
	}

	for _, c := range testCases {
		actual, err := c.input.ToK8s()
		if err != nil {
			t.Errorf("ToK8s failed; error %v", err)
			continue
		}
		if d := cmp.Diff(c.expected, actual); d != "" {
			t.Errorf("Unexpected diff;\n%v", d)
		}
	}
}
