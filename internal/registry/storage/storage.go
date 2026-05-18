// Package storage is the registry blob-storage backend interface
// and its v1.0 filesystem implementation (Epic 14 task 8). S3, OCI,
// and NATS Object Store backends are post-v1.0 (epic non-goals).
package storage

import (
	"context"
	"errors"
	"io"
	"time"
)

var (
	// ErrNotExist — no object at the key.
	ErrNotExist = errors.New("storage: object does not exist")
	// ErrInvalidKey — a key escapes the backend root (traversal /
	// absolute) or is otherwise malformed.
	ErrInvalidKey = errors.New("storage: invalid key")
)

// Info is object metadata.
type Info struct {
	Size    int64
	ModTime time.Time
}

// Storage is the registry's blob backend. Keys are slash-separated
// logical paths (e.g. "vendor/pkg/1.2.3/module.zip"); backends map
// them to their own namespace. All methods are safe for concurrent
// use.
type Storage interface {
	Get(ctx context.Context, key string) (io.ReadCloser, error)
	Put(ctx context.Context, key string, r io.Reader) error
	Delete(ctx context.Context, key string) error
	// List returns the keys under prefix (recursively).
	List(ctx context.Context, prefix string) ([]string, error)
	Exists(ctx context.Context, key string) (bool, error)
	Stat(ctx context.Context, key string) (Info, error)
	// Health reports whether the backend is usable.
	Health(ctx context.Context) error
}
