package testutil

import (
	"os"
	"testing"
)

const (
	// AwsTestsEnv is an env var used to determine if a test requires AWS access
	AwsTestsEnv = "AWS_TESTS"
)

// SkipAwsTests determines whether the tests requiring AWS should be skipped.
// The tests will be skipped if the environment variable AWS_TESTS
func SkipAwsTests(t *testing.T) {
	if v, ok := os.LookupEnv(AwsTestsEnv); !ok || v == "false" {
		t.Skip("Skipping AWS_TESTS")
	}
}
