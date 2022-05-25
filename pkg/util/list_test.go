package util

import (
	"sort"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestUnique(t *testing.T) {
	input := []string{"hello", "world", "hello", "there", "world"}
	expected := []string{"hello", "world", "there"}
	actual := UniqueStrings(input)
	sort.Strings(expected)
	sort.Strings(actual)
	assert.Equal(t, expected, actual)
}
