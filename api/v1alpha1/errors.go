package v1alpha1

import "fmt"

const (
	// TermErr classified as an error that needs to stop the program
	TermErr TerminalError = "Hit Terminal Error: "
	// NonTermErr classified as an error that we can ignore
	NonTermErr NonTerminalError = "Hit Non Terminal Error: "
)

type (
	// TerminalError error that should cause a panic
	TerminalError string
	// NonTerminalError error that we can safely ignore
	NonTerminalError string
)

func (e NonTerminalError) Error() string {
	return string(e)
}

// S3NonTerminalError a non terminal s3 error
type S3NonTerminalError string

func (e S3NonTerminalError) Error() string {
	return fmt.Sprintf("%s %s", NonTermErr, string(e))
}

// Unwrap func to unwrap a wrapped err
func (e S3NonTerminalError) Unwrap() error {
	return NonTermErr
}

const (
	// ErrS3IsADirectory a non terminal error for a directory that already exists
	ErrS3IsADirectory S3NonTerminalError = "is a directory"
	// ErrS3AssetAlreadyDownloaded a non terminal for an already downloaded s3 asset
	ErrS3AssetAlreadyDownloaded S3NonTerminalError = "Asset already downloaded"
)
