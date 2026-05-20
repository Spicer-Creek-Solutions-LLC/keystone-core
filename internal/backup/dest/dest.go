// Package dest resolves a kscore-backup destination URI to an
// [io.WriteCloser] the rest of the backup stack (encrypter +
// tar.Writer + manifest) writes through.
//
// v1.0 ships two backends: local filesystem and S3-compatible
// object storage. The "S3" backend is any service speaking the
// S3 API — AWS S3, MinIO, Backblaze B2, Wasabi, Cloudflare R2,
// DigitalOcean Spaces, etc. — by setting [Config.Endpoint] to the
// service's S3 hostname. SFTP, GCS, and Azure Blob defer to v1.x
// per the ROADMAP entry "Backup destinations: ...".
//
// Backblaze B2 example:
//
//	cfg := dest.Config{
//	    AccessKey: "<B2 application key ID>",
//	    SecretKey: "<B2 application key>",
//	    Region:    "us-west-002",
//	    Endpoint:  "s3.us-west-002.backblazeb2.com",
//	    UseSSL:    true,
//	}
//	d, err := dest.Resolve("s3://my-kscore-backups/2026-05-20.tar", cfg)
//
// Resolved [Destination] values are short-lived: open them once per
// artifact, write the bytes, Close. The S3 backend's Close is
// load-bearing — it waits for the upload goroutine to finish and
// surfaces upload errors. Always check it.
package dest

import (
	"context"
	"fmt"
	"io"
	"net/url"
	"strings"
)

// Destination is the single seam this package exposes. Concrete
// implementations live in this same package (LocalDestination,
// S3Destination); future backends (SFTP / GCS / Azure) implement
// the same interface.
type Destination interface {
	// Open returns a writer for the artifact bytes. The caller MUST
	// Close the returned writer — for S3 the Close is where the
	// upload completes and any error surfaces.
	Open(ctx context.Context) (io.WriteCloser, error)
}

// Config is the operator-supplied connection state shared across
// resolved destinations. Fields for backends other than the resolved
// one are ignored.
type Config struct {
	// S3 settings.
	AccessKey string
	SecretKey string
	Region    string
	Endpoint  string // empty = AWS default endpoint; non-empty = MinIO/B2/etc.
	UseSSL    bool
}

// Resolve dispatches a destination URI to the matching backend.
// Supported forms:
//
//	/abs/path/foo.tar           — local file, absolute
//	./rel/path/foo.tar          — local file, relative
//	file:///abs/path/foo.tar    — local file, RFC-style
//	s3://bucket/key/foo.tar     — S3-compatible object storage
//
// Any other scheme returns an error. The URI is structural only;
// directory existence / bucket validity is checked at Open time.
func Resolve(uri string, cfg Config) (Destination, error) {
	if uri == "" {
		return nil, fmt.Errorf("dest: empty URI")
	}

	// Plain path with no scheme — treat as local.
	if !strings.Contains(uri, "://") {
		return &LocalDestination{Path: uri}, nil
	}

	u, err := url.Parse(uri)
	if err != nil {
		return nil, fmt.Errorf("dest: parse %q: %w", uri, err)
	}

	switch u.Scheme {
	case "file":
		// file:// uses the Path (Host should be empty or "localhost").
		if u.Host != "" && u.Host != "localhost" {
			return nil, fmt.Errorf("dest: file:// host must be empty or localhost, got %q", u.Host)
		}
		if u.Path == "" {
			return nil, fmt.Errorf("dest: file:// path must not be empty")
		}
		return &LocalDestination{Path: u.Path}, nil

	case "s3":
		bucket := u.Host
		if bucket == "" {
			return nil, fmt.Errorf("dest: s3:// bucket must not be empty")
		}
		key := strings.TrimPrefix(u.Path, "/")
		if key == "" {
			return nil, fmt.Errorf("dest: s3:// key must not be empty")
		}
		return &S3Destination{
			Bucket:   bucket,
			Key:      key,
			Config:   cfg,
		}, nil

	default:
		return nil, fmt.Errorf("dest: unsupported scheme %q (want file or s3)", u.Scheme)
	}
}
