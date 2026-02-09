// Package backend provides storage backend implementations for file distribution.
package backend

import (
	"context"
	"errors"
	"io"
	"time"
)

// Backend is the interface that storage backends must implement.
type Backend interface {
	// Name returns the backend name for identification.
	Name() string

	// Type returns the backend type (filesystem, s3, gcs, azure, nats, git, http).
	Type() Type

	// BaseConfig returns the base configuration for path matching and priority.
	BaseConfig() *Config

	// Get retrieves a file from the backend.
	// Returns a reader that streams the file content and metadata.
	Get(ctx context.Context, path string, opts *GetOptions) (*GetResult, error)

	// Put stores a file in the backend.
	Put(ctx context.Context, path string, reader io.Reader, opts *PutOptions) (*PutResult, error)

	// Delete removes a file from the backend.
	Delete(ctx context.Context, path string) error

	// Exists checks if a file exists.
	Exists(ctx context.Context, path string) (bool, error)

	// Stat returns file metadata without reading content.
	Stat(ctx context.Context, path string) (*FileInfo, error)

	// List returns files matching the given path pattern.
	List(ctx context.Context, path string, opts *ListOptions) (*ListResult, error)

	// Health checks if the backend is healthy.
	Health(ctx context.Context) error

	// Close releases any resources held by the backend.
	Close() error
}

// Type identifies the type of storage backend.
type Type string

// TypeFilesystem constants define the supported types.
const (
	TypeFilesystem Type = "filesystem"
	TypeS3         Type = "s3"
	TypeGCS        Type = "gcs"
	TypeAzure      Type = "azure"
	TypeNATSObject Type = "nats-object-store"
	TypeGit        Type = "git"
	TypeHTTP       Type = "http"
)

// GetOptions specifies options for Get operations.
type GetOptions struct {
	// Version to retrieve (empty for latest).
	Version string

	// Range for partial content retrieval.
	Range *ByteRange

	// IfNoneMatch skips transfer if checksum matches (conditional GET).
	IfNoneMatch string
}

// ByteRange specifies a byte range for partial retrieval.
type ByteRange struct {
	Start int64
	End   int64 // 0 means to end of file
}

// GetResult contains the result of a Get operation.
type GetResult struct {
	// Reader streams the file content. Caller must close.
	Reader io.ReadCloser

	// Info contains file metadata.
	Info *FileInfo

	// NotModified is true if IfNoneMatch matched.
	NotModified bool
}

// PutOptions specifies options for Put operations.
type PutOptions struct {
	// ContentType of the file.
	ContentType string

	// Size of the file (required for some backends).
	Size int64

	// Checksum (SHA-256) for verification.
	Checksum string

	// Tags for the file.
	Tags map[string]string

	// Metadata is additional key-value metadata.
	Metadata map[string]string

	// Version to create (empty for auto).
	Version string

	// Overwrite existing file.
	Overwrite bool
}

// PutResult contains the result of a Put operation.
type PutResult struct {
	// Path where the file was stored.
	Path string

	// Version of the stored file.
	Version string

	// Checksum (SHA-256) of the stored file.
	Checksum string

	// Size of the stored file.
	Size int64

	// ETag is the entity tag for the stored file.
	ETag string
}

// ListOptions specifies options for List operations.
type ListOptions struct {
	// Recursive includes files in subdirectories.
	Recursive bool

	// Limit maximum number of results.
	Limit int

	// Offset for pagination.
	Offset int

	// IncludeMetadata includes full metadata (slower).
	IncludeMetadata bool
}

// ListResult contains the result of a List operation.
type ListResult struct {
	// Files in the listing.
	Files []FileInfo

	// TotalCount of matching files (may be estimated).
	TotalCount int

	// Truncated indicates more results available.
	Truncated bool

	// NextOffset for pagination.
	NextOffset int
}

// FileInfo contains file metadata.
type FileInfo struct {
	// Path is the full path to the file.
	Path string

	// Name is the file name without directory.
	Name string

	// Size in bytes.
	Size int64

	// Checksum (SHA-256).
	Checksum string

	// ContentType of the file.
	ContentType string

	// ModifiedTime is when the file was last modified.
	ModifiedTime time.Time

	// IsDirectory indicates if this is a directory.
	IsDirectory bool

	// Version of the file.
	Version string

	// ETag is the entity tag for conditional requests.
	ETag string

	// Tags associated with the file.
	Tags map[string]string

	// Metadata is additional key-value metadata.
	Metadata map[string]string
}

// Config is the base configuration for backends.
type Config struct {
	// Name identifies this backend instance.
	Name string `yaml:"name"`

	// Type of backend.
	Type Type `yaml:"type"`

	// Priority for backend selection (lower = higher priority).
	Priority int `yaml:"priority"`

	// Paths this backend handles (glob patterns).
	Paths []string `yaml:"paths"`

	// MaxFileSize limit for this backend.
	MaxFileSize int64 `yaml:"max_file_size,omitempty"`

	// ReadOnly prevents writes to this backend.
	ReadOnly bool `yaml:"read_only,omitempty"`
}

// MatchesPath checks if this backend handles the given path.
func (c *Config) MatchesPath(path string) bool {
	for _, pattern := range c.Paths {
		if matchGlob(pattern, path) {
			return true
		}
	}
	return false
}

// matchGlob performs simple glob matching.
// Supports * (any characters except /) and ** (any characters including /).
func matchGlob(pattern, path string) bool {
	return matchGlobRecursive(pattern, path)
}

func matchGlobRecursive(pattern, path string) bool {
	for pattern != "" {
		switch pattern[0] {
		case '*':
			if len(pattern) > 1 && pattern[1] == '*' {
				// ** matches any path
				pattern = pattern[2:]
				if pattern == "" {
					return true
				}
				// Skip optional /
				if pattern != "" && pattern[0] == '/' {
					pattern = pattern[1:]
				}
				// Try matching at each position
				for i := 0; i <= len(path); i++ {
					if matchGlobRecursive(pattern, path[i:]) {
						return true
					}
				}
				return false
			}
			// * matches any non-/ characters
			pattern = pattern[1:]
			if pattern == "" {
				// * at end matches rest if no /
				for _, c := range path {
					if c == '/' {
						return false
					}
				}
				return true
			}
			// Try matching at each position until /
			for i := 0; i <= len(path); i++ {
				if i > 0 && path[i-1] == '/' {
					break
				}
				if matchGlobRecursive(pattern, path[i:]) {
					return true
				}
			}
			return false
		default:
			if path == "" || pattern[0] != path[0] {
				return false
			}
			pattern = pattern[1:]
			path = path[1:]
		}
	}
	return path == ""
}

// Error types for backend operations.
type Error struct {
	Backend string
	Op      string
	Path    string
	Err     error
}

func (e *Error) Error() string {
	if e.Path != "" {
		return e.Backend + ": " + e.Op + " " + e.Path + ": " + e.Err.Error()
	}
	return e.Backend + ": " + e.Op + ": " + e.Err.Error()
}

func (e *Error) Unwrap() error {
	return e.Err
}

// Common errors.
var (
	ErrNotFound     = &Error{Op: "get", Err: notFoundError{}}
	ErrAccessDenied = &Error{Op: "access", Err: accessDeniedError{}}
	ErrReadOnly     = &Error{Op: "write", Err: readOnlyError{}}
)

type notFoundError struct{}

func (notFoundError) Error() string { return "file not found" }

type accessDeniedError struct{}

func (accessDeniedError) Error() string { return "access denied" }

type readOnlyError struct{}

func (readOnlyError) Error() string { return "backend is read-only" }

// IsNotFound checks if an error is a not found error.
func IsNotFound(err error) bool {
	if err == nil {
		return false
	}
	var nfe *notFoundError
	if errors.As(err, &nfe) {
		return true
	}
	var be *Error
	if errors.As(err, &be) {
		var innerNfe notFoundError
		return errors.As(be.Err, &innerNfe)
	}
	return false
}
