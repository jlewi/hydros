package s3assets

import (
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"

	"sigs.k8s.io/kustomize/kyaml/yaml"

	"github.com/PrimerAI/go-micro-utils-public/gmu/s3"
	"github.com/jlewi/hydros/api/v1alpha1"
)

// Load reads s3 assets from an S3AssetsList yaml file.
func Load(assetsFile string) ([]string, error) {
	data, err := ioutil.ReadFile(assetsFile)
	if err != nil {
		return nil, err
	}
	assetsList := &v1alpha1.S3AssetsList{}
	if err = yaml.Unmarshal(data, assetsList); err != nil {
		return nil, err
	}
	return assetsList.S3Assets, nil
}

// List reads a directory and returns a list of paths relative to `assetsDir`.
func List(assetsDir string) (assets []string, err error) {
	err = filepath.Walk(assetsDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() {
			_, innerPath := splitFirstDirPath(path)
			assets = append(assets, innerPath)
		}
		return nil
	})
	return
}

// DownloadWithRetry will try to download all s3 assets to a directory.
// Will accept errors `retries` times. The path in `saveDir` that assets are saved to
// is simply saveDir/bucket/assetKey. E.g. if there's an s3 asset in `assets` with the
// uri "s3://my-bucket/nested/key/file.zip" and `saveDir` is "download_dir" then the
// asset will be saved at the path: `download_dir/my-bucket/nested/key/file.zip` with
// any intermediate directories being created as needed.
func DownloadWithRetry(assets []string, saveDir, s3Endpoint string, retries int) error {
	d, err := NewManyDownloader(assets, saveDir, s3Endpoint, retries)
	if err != nil {
		return err
	}
	return d.Download()
}

// UploadWithRetry will try to upload all assets in a directory to a particular bucket.
// Will accept errors `retries` times. Asset paths in `assets` are expected to be relative to
// the `assetDir` directory. The key created in the bucket for each asset will be the path that
// same relative path in `assetDir`. E.g. if there's an asset path in `assets`
// "my-bucket/nested/key/file.zip", and an `assetDir` "download_dir", a file at the path
// "./download_dir/my-bucket/nested/key/file.zip" is expected to exist. And if the target
// `bucket` is "target-bucket" then the example file will be uploaded to the s3 uri
// "s3://target-bucket/my-bucket/nested/key/file.zip".
func UploadWithRetry(assets []string, assetDir, bucket, s3Endpoint string, retries int) error {
	u, err := NewManyUploader(assets, assetDir, bucket, s3Endpoint, retries)
	if err != nil {
		return err
	}
	return u.Upload()
}

// Downloader implement Download.
type Downloader interface {
	Download() error
}

// Uploader implement Upload.
type Uploader interface {
	Upload() error
}

// NewManyDownloader returns a Downloader that will download many s3 assets to a save directory.
func NewManyDownloader(assets []string, saveDir, s3Endpoint string, retries int) (Downloader, error) {
	m := &many{}
	s3Client, err := newS3Client(s3Endpoint)
	if err != nil {
		return m, err
	}

	m.assets = assets
	m.manager = &downloadManager{
		S3Client:    s3Client,
		MakeWriter:  makeLocalWriter(saveDir),
		downloadDir: saveDir,
	}
	m.retries = retries
	m.results = make(chan result, len(assets))
	m.maxConcurrent = 10
	return m, nil
}

// NewManyUploader returns an Uploader that will upload many assets in a directory to an s3 bucket.
func NewManyUploader(assets []string, assetDir, bucket, s3Endpoint string, retries int) (Uploader, error) {
	m := &many{}
	s3Client, err := newS3Client(s3Endpoint)
	if err != nil {
		return m, err
	}
	m.assets = assets
	m.manager = &uploadManager{
		S3Client: s3Client,
		AssetDir: assetDir,
		Bucket:   bucket,
	}
	m.retries = retries
	m.results = make(chan result, len(assets))
	m.maxConcurrent = 10
	return m, nil
}

// Download many s3 assets.
func (m many) Download() error {
	return m.run()
}

// Upload many assets.
func (m many) Upload() error {
	return m.run()
}

func newS3Client(s3Endpoint string) (s3.Client, error) {
	options := []s3.ClientOption{}
	if s3Endpoint != "" {
		options = []s3.ClientOption{
			s3.WithEndpoint(s3Endpoint),
			s3.WithS3ForcePathStyle(true),
		}
	}
	return s3.NewClient(options...)
}

func (m many) run() error {
	sem := make(chan int, m.maxConcurrent)
	for _, asset := range m.assets {
		s := single{
			asset:   asset,
			manager: m.manager.Copy(),
			retries: m.retries,
			results: m.results,
		}
		sem <- 1
		go func() {
			_ = s.run()
			<-sem
		}()
	}

	failed := []result{}
	for i := 0; i < len(m.assets); i++ {
		r := <-m.results

		if r.err != nil && !errors.Is(r.err, v1alpha1.NonTermErr) {
			failed = append(failed, r)
		}
	}
	if len(failed) > 0 {
		return fmt.Errorf("Failed to handle some assets: %s", failed)
	}
	return nil
}

// NewSingleDownloader returns a Downloader that will download a single s3 asset to a save directory.
func NewSingleDownloader(asset, saveDir string, retries int) (Downloader, error) {
	s := &single{}
	s3Client, err := s3.NewClient()
	if err != nil {
		return s, err
	}
	s.asset = asset
	s.manager = &downloadManager{
		S3Client:   s3Client,
		MakeWriter: makeLocalWriter(saveDir),
	}
	s.retries = retries
	s.results = make(chan result, 1)
	return s, nil
}

// NewSingleUploader returns a Uploader that will upload a single asset in a save directory to
// an s3 bucket. `asset` must be a path relative to `saveDir`.
func NewSingleUploader(asset, saveDir, bucket string, retries int) (Uploader, error) {
	s := &single{}
	s3Client, err := s3.NewClient()
	if err != nil {
		return s, err
	}
	s.asset = asset
	s.manager = &uploadManager{
		S3Client: s3Client,
		AssetDir: saveDir,
		Bucket:   bucket,
	}
	s.retries = retries
	s.results = make(chan result, 1)
	return s, nil
}

// Download a single s3 assets.
func (s single) Download() error {
	return s.run()
}

// Upload a single assets.
func (s single) Upload() error {
	return s.run()
}

func (s single) run() (err error) {
	defer func() {
		err = s.manager.Finish(err)
		if err == nil {
			fmt.Printf("Finished %s\n", s.asset)
		}
		s.results <- result{asset: s.asset, err: err}
	}()
	err = s.manager.Start(s.asset)
	if err != nil {
		return
	}
	for tries := 0; tries < s.retries; tries++ {
		err = s.manager.Run(s.asset)
	}
	return
}

type many struct {
	assets        []string
	manager       manager
	retries       int
	results       chan result
	maxConcurrent int
}

type single struct {
	asset   string
	manager manager
	retries int
	results chan result
}

// manager manages the io of one asset. It's methods are expected to be run in the
// order Start, Run, Finish. If an error occurs in any one of these the remainder
// should result in noops, returning the error from whatever stage errored.
type manager interface {
	Start(string) error
	Run(string) error
	Finish(error) error
	Copy() manager
}

type downloadManager struct {
	S3Client    s3.Client
	MakeWriter  func(string) (io.WriteCloser, error)
	writer      io.WriteCloser
	successful  bool
	finished    bool
	err         error
	downloadDir string
}

func (m *downloadManager) Start(s3Asset string) (err error) {
	if m.writer != nil {
		return m.err
	}
	m.writer, m.err = m.MakeWriter(s3Asset)
	return m.err
}

func (m *downloadManager) Run(s3Asset string) (err error) {
	if m.writer == nil {
		m.err = fmt.Errorf("download manager not started properly, file writer is nil")
		return m.err
	}
	if m.successful || m.err != nil {
		return m.err
	}
	fmt.Printf("Downloading %s\n", s3Asset)
	sPath, err := s3.FromURI(s3Asset)
	if err != nil {
		m.err = err
		return
	}
	if !m.S3Client.Exists(sPath) { // we must be looking at some directory, skip
		m.successful = true
		return
	}

	file, err := os.Create(filepath.Join(m.downloadDir, sPath.Bucket, sPath.Key))
	if err != nil {
		m.err = err
		return
	}

	defer func() {
		err := file.Close()
		if err != nil {
			fmt.Println("failed to close file on defer: %w", err)
		}
	}()

	n, err := m.S3Client.DownloadInFile(sPath, file)
	if err != nil {
		m.err = err
	} else {
		m.successful = true
	}
	fmt.Println("Wrote", n, "bytes for", s3Asset)
	return
}

func (m *downloadManager) Finish(err error) error {
	if !m.finished {
		err = m.writer.Close()
		if err != nil {
			return fmt.Errorf("Failed to close downloadManager writer: %w", err)
		}
		m.finished = true
	}
	if err != nil {
		return err
	}

	return m.err
}

func (m downloadManager) Copy() manager {
	new := &downloadManager{}
	new.S3Client = m.S3Client
	new.MakeWriter = m.MakeWriter
	new.downloadDir = m.downloadDir
	return new
}

// makeLocalWriter makes a function that will take an s3asset string and return a WriteCloser.
// It makes sure that the s3 asset uri can be parsed, and ensures that the write directory exists
// before creating an os.File which is the WriteCloser.
func makeLocalWriter(saveDir string) func(string) (io.WriteCloser, error) {
	return func(s3asset string) (io.WriteCloser, error) {
		sPath, err := s3.FromURI(s3asset)
		if err != nil {
			return DiscardCloser, err
		}
		writePath := filepath.Join(saveDir, sPath.Bucket, sPath.Key)
		if fileInfo, err := os.Stat(writePath); os.IsNotExist(err) {
			err = os.MkdirAll(filepath.Dir(writePath), 0o777)
			if err != nil {
				return DiscardCloser, fmt.Errorf("failed to create %s: %w", writePath, err)
			}
		} else if fileInfo.IsDir() {
			return DiscardCloser, fmt.Errorf("skipping %s: %w", s3asset, v1alpha1.ErrS3IsADirectory)
		} else {
			return DiscardCloser, v1alpha1.ErrS3AssetAlreadyDownloaded
		}
		writeCloser, err := os.Create(writePath)
		if err != nil { // TODO: Consider adding a force option to make downloading always happen
			fileInfo, _ := os.Stat(writePath)
			if fileInfo.IsDir() {
				return DiscardCloser, fmt.Errorf("skipping %s: %w", s3asset, v1alpha1.ErrS3IsADirectory)
			}
			return DiscardCloser, err
		}
		return writeCloser, err
	}
}

type uploadManager struct {
	S3Client    s3.Client
	AssetDir    string
	Bucket      string
	absAssetDir string
	successful  bool
	err         error
}

func (m *uploadManager) Start(_ string) (err error) {
	m.absAssetDir, m.err = filepath.Abs(m.AssetDir)
	return m.err
}

func (m *uploadManager) Run(asset string) (err error) {
	if m.successful || m.err != nil {
		return m.err
	}
	fmt.Printf("Uploading %s\n", asset)
	sPath := s3.Path{Bucket: m.Bucket, Key: asset}
	bytes, err := ioutil.ReadFile(filepath.Join(m.absAssetDir, asset))
	if err != nil {
		m.err = err
		return
	}
	err = m.S3Client.Upload(bytes, sPath)
	if err != nil {
		m.err = err
	} else {
		fmt.Println("Wrote", len(bytes), "bytes to", sPath.ToURI())
		m.successful = true
	}
	return
}

func (m uploadManager) Finish(err error) error {
	if err != nil {
		return err
	}
	return m.err
}

func (m uploadManager) Copy() manager {
	new := &uploadManager{}
	new.S3Client = m.S3Client
	new.AssetDir = m.AssetDir
	new.Bucket = m.Bucket
	return new
}

type result struct {
	asset string
	err   error
}

// splitFirstDirPath takes a `path` and returns the top level directory and
// remaining path as separate strings.
func splitFirstDirPath(path string) (string, string) {
	parts := strings.Split(path, string(os.PathSeparator))
	var (
		dir  string
		file string
	)
	switch len(parts) {
	case 0:
		dir = ""
		file = ""
	case 1:
		dir = ""
		file = parts[0]
	default:
		dir = parts[0]
		file = filepath.Join(parts[1:]...)
	}
	return dir, file
}

// String makes results print in a human readable form when put in logs.
func (r result) String() string {
	return fmt.Sprintf("asset: %s, err: %v.", r.asset, r.err)
}

// DiscardCloser implements a noop Write and Close.
var DiscardCloser io.WriteCloser = discardCloser{}

type discardCloser struct{}

// Write is a noop.
func (discardCloser) Write(b []byte) (int, error) {
	return len(b), nil
}

// Close is a noop.
func (discardCloser) Close() error { return nil }
