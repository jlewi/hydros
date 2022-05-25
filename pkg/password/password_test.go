package password

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestEncodePasswordToPBKDF2(t *testing.T) {
	type testCase struct {
		password       string
		iterations     int
		keyLength      int
		salt           string
		expectedString string
	}

	testCases := []testCase{
		{
			password:       "testpassword",
			iterations:     310000,
			keyLength:      32,
			salt:           "some-random-salty-string",
			expectedString: "pbkdf2:sha256:310000$some-random-salty-string$34fe9ecdc975966305a02c138881397f7beebb01a6c8de9f16ca49c9fc64ed1d",
		},
	}

	for _, tc := range testCases {
		finalStr := encodePasswordToPBKDF2(
			tc.password,
			tc.iterations,
			tc.keyLength,
			tc.salt,
		)
		assert.Equal(t, tc.expectedString, finalStr, "Both values should be equal")
	}
}
