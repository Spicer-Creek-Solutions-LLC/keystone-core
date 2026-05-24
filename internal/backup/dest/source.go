// SPDX-License-Identifier: Apache-2.0

package dest

import (
	"context"
	"fmt"
	"io"
	"net/url"
	"strings"
	"time"
)

// Source is the read-side counterpart to [Destination]. The
// kscore-backup verify + restore subcommands call [ResolveSource] to
// turn a URI into a [Source], then open it and (optionally) stack
// age decryption + tar reading on top.
//
// Concrete impls: [LocalSource] (filesystem) and [S3Source]
// (S3-compatible). SFTP/GCS/Azure source backends defer under the
// v1.x ROADMAP entry "Backup destinations: ...".
type Source interface {
	Open(ctx context.Context) (io.ReadCloser, error)
}

// Lister enumerates artifacts at a prefix URI. The kscore-backup
// list subcommand calls [ResolveLister] then [Lister.List].
type Lister interface {
	List(ctx context.Context) ([]Entry, error)
}

// Entry is one row returned by [Lister.List]. Name is the artifact
// identifier (relative to the prefix for S3; basename for local);
// Size is in bytes; LastModified is the server-reported timestamp.
type Entry struct {
	Name         string
	Size         int64
	LastModified time.Time
}

// ResolveSource parses uri and returns the matching [Source].
// Supported schemes mirror [Resolve]:
//
//	/abs/path/foo.tar           — local file, absolute
//	./rel/path/foo.tar          — local file, relative
//	file:///abs/path/foo.tar    — local file, RFC-style
//	s3://bucket/key/path.tar    — S3-compatible object storage
func ResolveSource(uri string, cfg Config) (Source, error) {
	scheme, host, path, err := parseURI(uri)
	if err != nil {
		return nil, err
	}
	switch scheme {
	case "file":
		return &LocalSource{Path: path}, nil
	case "s3":
		key := strings.TrimPrefix(path, "/")
		if key == "" {
			return nil, fmt.Errorf("dest: s3:// source key must not be empty")
		}
		return &S3Source{Bucket: host, Key: key, Config: cfg}, nil
	default:
		return nil, fmt.Errorf("dest: unsupported source scheme %q (want file or s3)", scheme)
	}
}

// ResolveLister parses a prefix URI and returns the matching
// [Lister]. For local destinations the URI is a directory; for S3 it
// is a `s3://bucket/optional-prefix/`.
func ResolveLister(uri string, cfg Config) (Lister, error) {
	scheme, host, path, err := parseURI(uri)
	if err != nil {
		return nil, err
	}
	switch scheme {
	case "file":
		return &LocalLister{Dir: path}, nil
	case "s3":
		prefix := strings.TrimPrefix(path, "/")
		return &S3Lister{Bucket: host, Prefix: prefix, Config: cfg}, nil
	default:
		return nil, fmt.Errorf("dest: unsupported lister scheme %q (want file or s3)", scheme)
	}
}

// parseURI factors the URI dispatch used by [Resolve],
// [ResolveSource], and [ResolveLister]. Returns (scheme, host, path).
// A no-scheme input is treated as a local file path; scheme is
// returned as "file" with host empty and path = uri.
func parseURI(uri string) (scheme, host, path string, err error) {
	if uri == "" {
		return "", "", "", fmt.Errorf("dest: empty URI")
	}
	if !strings.Contains(uri, "://") {
		return "file", "", uri, nil
	}
	u, perr := url.Parse(uri)
	if perr != nil {
		return "", "", "", fmt.Errorf("dest: parse %q: %w", uri, perr)
	}
	switch u.Scheme {
	case "file":
		if u.Host != "" && u.Host != "localhost" {
			return "", "", "", fmt.Errorf("dest: file:// host must be empty or localhost, got %q", u.Host)
		}
		if u.Path == "" {
			return "", "", "", fmt.Errorf("dest: file:// path must not be empty")
		}
		return "file", "", u.Path, nil
	case "s3":
		if u.Host == "" {
			return "", "", "", fmt.Errorf("dest: s3:// bucket must not be empty")
		}
		return "s3", u.Host, u.Path, nil
	default:
		return u.Scheme, u.Host, u.Path, nil
	}
}
