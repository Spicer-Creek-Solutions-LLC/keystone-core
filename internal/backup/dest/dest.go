// SPDX-License-Identifier: Apache-2.0

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
	"strings"

	"go.keystone-core.io/keystone-core/internal/s3client"
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
// one are ignored. The shape lives in [s3client.Config]; this alias
// keeps the dest.Config name in callers' struct literals while the
// minio-go client construction is shared with
// internal/files/backend/s3.
type Config = s3client.Config

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
	scheme, host, path, err := parseURI(uri)
	if err != nil {
		return nil, err
	}
	switch scheme {
	case "file":
		return &LocalDestination{Path: path}, nil
	case "s3":
		key := strings.TrimPrefix(path, "/")
		if key == "" {
			return nil, fmt.Errorf("dest: s3:// key must not be empty")
		}
		return &S3Destination{Bucket: host, Key: key, Config: cfg}, nil
	default:
		return nil, fmt.Errorf("dest: unsupported scheme %q (want file or s3)", scheme)
	}
}
