package backend

import (
	"context"
	"errors"
	"io"

	"go.keystone-core.io/keystone-core/internal/files"
)

// Store is the seam every kscore file backend implements. The
// service layer (T14 REST handler, T15 CLI, T11 chunked transport)
// depends on this interface and is impl-agnostic.
//
// All methods accept a Path in [files.FileMetadata.Path] format:
// forward-slash delimited, no leading slash, no "..". Backends MAY
// re-validate; callers MUST have validated the path before invoking
// the backend.
type Store interface {
	// Put writes body and assigns the next version atomically.
	// Returned metadata carries the backend-assigned Size, Hash,
	// Version, and CreatedAt. Caller-supplied Size/Hash/Version
	// fields on the input are ignored — the backend recomputes them
	// from the body stream. ContentType and Tags are preserved.
	Put(ctx context.Context, meta files.FileMetadata, body io.Reader) (files.FileMetadata, error)

	// Get returns the latest metadata + a reader for the body. The
	// caller MUST Close the reader. Returns [ErrNotFound] if path
	// is absent.
	Get(ctx context.Context, path string) (files.FileMetadata, io.ReadCloser, error)

	// Stat returns the latest metadata without opening the body.
	// Returns [ErrNotFound] if path is absent.
	Stat(ctx context.Context, path string) (files.FileMetadata, error)

	// List returns the latest metadata for every file whose path
	// starts with prefix. An empty prefix lists everything. The
	// result order is path-sorted (stable across backends so
	// callers can diff lists).
	List(ctx context.Context, prefix string) ([]files.FileMetadata, error)

	// Delete removes the file. Returns [ErrNotFound] if path is
	// absent; safe to call concurrently with other operations on
	// different paths.
	Delete(ctx context.Context, path string) error
}

// ErrNotFound is returned by Get, Stat, and Delete when the path
// has no entry. Callers can errors.Is against it to surface a 404
// at the REST layer.
var ErrNotFound = errors.New("backend: not found")
