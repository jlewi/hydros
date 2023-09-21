package s3assets

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/jlewi/hydros/api/v1alpha1"

	"github.com/PrimerAI/go-micro-utils-public/gmu/s3"
	"github.com/stretchr/testify/suite"
)

type S3IOTestSuite struct {
	suite.Suite
}

func TestS3IOSuite(t *testing.T) {
	suite.Run(t, &S3IOTestSuite{})
}

func (s *S3IOTestSuite) TestMakeLocalWriter() {
	testCases := map[string]struct {
		testPath string
		setup    func(string, string)
		err      error
	}{
		"new path": {
			testPath: "s3://path/file.txt",
			setup:    func(_, _ string) {},
			err:      nil,
		},
		"existing path": {
			testPath: "s3://path/file.txt",
			setup: func(dir, testPath string) {
				sPath, _ := s3.FromURI(testPath)
				filePath := filepath.Join(dir, sPath.Join())
				err := os.MkdirAll(filepath.Dir(filePath), 0o777)
				s.Assert().NoError(err, "Error making filePath ", filePath)
				file, _ := os.Create(filePath)
				err = file.Close()
				s.Assert().NoError(err, "Error closing file ", file)
			},
			err: v1alpha1.ErrS3AssetAlreadyDownloaded,
		},
		"existing directory": {
			testPath: "s3://path",
			setup: func(dir, testPath string) {
				sPath, _ := s3.FromURI(testPath)
				err := os.Mkdir(filepath.Join(dir, sPath.Join()), 0o777)
				s.Assert().NoError(err, "Error making filePath ", filepath.Join(dir, sPath.Join()))
			},
			err: v1alpha1.ErrS3IsADirectory,
		},
	}
	for name, tc := range testCases {
		dir, err := os.MkdirTemp(".", "localWriterTest")
		s.Assert().NoError(err, "Error making tmp dir", name)

		tc.setup(dir, tc.testPath)
		_, err = makeLocalWriter(dir)(tc.testPath)
		s.Assert().ErrorIs(err, tc.err, name)
		err = os.RemoveAll(dir)
		s.Assert().NoError(err, "Error removing dir", dir)
	}
}
