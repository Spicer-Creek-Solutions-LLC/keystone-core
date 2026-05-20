package dest

import (
	"context"
	"errors"
	"fmt"
	"io"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

// S3Destination writes the artifact to an S3-compatible object
// store. The configured [Config] supplies access credentials and the
// endpoint hostname; an empty [Config.Endpoint] selects AWS S3, any
// other host selects the named S3-compatible service (MinIO, B2,
// Wasabi, R2, etc.).
type S3Destination struct {
	Bucket string
	Key    string
	Config Config
}

// s3ContentType is the MIME the destination stamps on the uploaded
// object. The artifact is always an uncompressed tar in v1.0;
// task 4 (age envelope) does not change that — the Content-Type
// describes the byte stream the receiver sees over HTTP.
const s3ContentType = "application/x-tar"

// Open returns a writer that, when written to and Closed, uploads
// the bytes as a single object at Bucket/Key. Internally an io.Pipe
// bridges the writer-shaped seam to minio-go's reader-shaped
// PutObject; Close waits for the upload goroutine to finish so the
// caller learns about upload errors synchronously.
//
// Size -1 lets minio-go pick multipart upload automatically once the
// stream exceeds its threshold (64 MiB by default). Operators do not
// configure that — backups for v1.0 are well under typical
// single-PUT object-size limits but multipart still applies for the
// large-snapshot case.
func (s *S3Destination) Open(ctx context.Context) (io.WriteCloser, error) {
	client, err := s.newClient()
	if err != nil {
		return nil, err
	}

	pr, pw := io.Pipe()
	errCh := make(chan error, 1)
	go func() {
		_, putErr := client.PutObject(ctx, s.Bucket, s.Key, pr, -1, minio.PutObjectOptions{
			ContentType: s3ContentType,
		})
		if putErr != nil {
			_ = pr.CloseWithError(putErr)
		}
		errCh <- putErr
	}()

	return &s3Writer{pw: pw, errCh: errCh}, nil
}

// newClient builds the minio-go client from Config. Endpoint may be
// "" for AWS default; UseSSL must match the endpoint scheme.
func (s *S3Destination) newClient() (*minio.Client, error) {
	endpoint := s.Config.Endpoint
	if endpoint == "" {
		endpoint = "s3.amazonaws.com"
	}
	if s.Config.AccessKey == "" || s.Config.SecretKey == "" {
		return nil, errors.New("dest: S3 access key + secret key are required")
	}
	client, err := minio.New(endpoint, &minio.Options{
		Creds:  credentials.NewStaticV4(s.Config.AccessKey, s.Config.SecretKey, ""),
		Secure: s.Config.UseSSL,
		Region: s.Config.Region,
	})
	if err != nil {
		return nil, fmt.Errorf("dest: minio.New: %w", err)
	}
	return client, nil
}

// s3Writer is the io.WriteCloser returned from S3Destination.Open.
// Write forwards to the pipe; Close signals EOF to the uploader
// goroutine and waits for the result.
type s3Writer struct {
	pw    *io.PipeWriter
	errCh chan error
}

func (w *s3Writer) Write(p []byte) (int, error) { return w.pw.Write(p) }

// Close closes the pipe writer (signalling EOF to PutObject), then
// blocks until the uploader goroutine returns. Any upload error is
// returned here — the caller MUST check this Close.
func (w *s3Writer) Close() error {
	if err := w.pw.Close(); err != nil {
		// Drain errCh so the goroutine can exit even if we return early.
		<-w.errCh
		return fmt.Errorf("dest: close pipe: %w", err)
	}
	if err := <-w.errCh; err != nil {
		return fmt.Errorf("dest: s3 upload: %w", err)
	}
	return nil
}
