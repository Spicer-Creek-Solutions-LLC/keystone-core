// Package backend provides storage backend implementations for the file server.
package backend

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"path"
	"strings"
	"sync"
	"time"
)

// NATSConfig is the configuration for the NATS Object Store backend.
type NATSConfig struct {
	Config `yaml:",inline"`

	// BucketName is the NATS object store bucket name.
	BucketName string `yaml:"bucket_name"`

	// URL is the NATS server URL.
	URL string `yaml:"url"`

	// Credentials is the path to the NATS credentials file.
	Credentials string `yaml:"credentials,omitempty"`

	// NKey is the NKey seed for authentication.
	NKey string `yaml:"nkey,omitempty"`

	// Token is the authentication token.
	Token string `yaml:"token,omitempty"`

	// Username is the username for basic auth.
	Username string `yaml:"username,omitempty"`

	// Password is the password for basic auth.
	Password string `yaml:"password,omitempty"`

	// TLSCert is the path to the TLS certificate.
	TLSCert string `yaml:"tls_cert,omitempty"`

	// TLSKey is the path to the TLS key.
	TLSKey string `yaml:"tls_key,omitempty"`

	// TLSCA is the path to the TLS CA certificate.
	TLSCA string `yaml:"tls_ca,omitempty"`

	// Prefix is an optional prefix for all objects in the bucket.
	Prefix string `yaml:"prefix,omitempty"`

	// Replicas is the number of replicas for the object store.
	Replicas int `yaml:"replicas,omitempty"`

	// MaxBytes is the maximum size of the object store.
	MaxBytes int64 `yaml:"max_bytes,omitempty"`

	// TTL is the time-to-live for objects.
	TTL time.Duration `yaml:"ttl,omitempty"`

	// Description is a description for the object store bucket.
	Description string `yaml:"description,omitempty"`
}

// NATSBackend implements the Backend interface for NATS Object Store.
// This is a stub implementation that provides the interface without
// actual NATS SDK dependencies. For production use, wire up the github.com/nats-io/nats.go client.
type NATSBackend struct {
	config *NATSConfig
	store  NATSObjectStore
	mu     sync.RWMutex
}

// NATSObjectStore is an interface for NATS Object Store operations.
// This allows for mocking in tests and alternative implementations.
type NATSObjectStore interface {
	// Get retrieves an object from the store.
	Get(ctx context.Context, name string) (*NATSObject, error)

	// Put stores an object in the store.
	Put(ctx context.Context, name string, body io.Reader, opts *NATSPutOptions) (*NATSObjectInfo, error)

	// Delete removes an object from the store.
	Delete(ctx context.Context, name string) error

	// GetInfo returns object metadata without the body.
	GetInfo(ctx context.Context, name string) (*NATSObjectInfo, error)

	// List lists objects with an optional filter.
	List(ctx context.Context, opts *NATSListOptions) ([]*NATSObjectInfo, error)

	// Status returns the object store status.
	Status(ctx context.Context) (*NATSBucketStatus, error)
}

// NATSPutOptions are options for Put operations.
type NATSPutOptions struct {
	Description string
	Headers     map[string]string
}

// NATSListOptions are options for List operations.
type NATSListOptions struct {
	// FilterPrefix filters objects by name prefix.
	FilterPrefix string
}

// NATSObject represents a retrieved object.
type NATSObject struct {
	// Body is the object content.
	Body io.ReadCloser

	// Info is the object metadata.
	Info *NATSObjectInfo
}

// NATSObjectInfo contains object metadata.
type NATSObjectInfo struct {
	// Name is the object name.
	Name string

	// Size is the object size in bytes.
	Size uint64

	// Digest is the SHA-256 digest of the object.
	Digest string

	// ModTime is the modification time.
	ModTime time.Time

	// Description is the object description.
	Description string

	// Headers are custom headers/metadata.
	Headers map[string][]string

	// Chunks is the number of chunks.
	Chunks uint32

	// Deleted indicates if the object is deleted.
	Deleted bool
}

// NATSBucketStatus contains object store bucket status.
type NATSBucketStatus struct {
	// Bucket is the bucket name.
	Bucket string

	// Description is the bucket description.
	Description string

	// Replicas is the number of replicas.
	Replicas int

	// Size is the total size of objects.
	Size uint64

	// Objects is the number of objects.
	Objects uint64

	// TTL is the default TTL.
	TTL time.Duration
}

// NewNATSBackend creates a new NATS Object Store backend.
func NewNATSBackend(config *NATSConfig) (*NATSBackend, error) {
	if config.BucketName == "" {
		return nil, fmt.Errorf("nats backend: bucket_name is required")
	}
	if config.URL == "" {
		return nil, fmt.Errorf("nats backend: url is required")
	}

	return &NATSBackend{
		config: config,
	}, nil
}

// SetStore sets the NATS object store for the backend.
// This should be called after creating the backend to wire up the actual NATS client.
func (b *NATSBackend) SetStore(store NATSObjectStore) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.store = store
}

// Name returns the backend name.
func (b *NATSBackend) Name() string {
	return b.config.Name
}

// Type returns the backend type.
func (b *NATSBackend) Type() BackendType {
	return BackendTypeNATSObject
}

// BaseConfig returns the base configuration for path matching and priority.
func (b *NATSBackend) BaseConfig() *Config {
	return &b.config.Config
}

// Get retrieves a file from NATS Object Store.
func (b *NATSBackend) Get(ctx context.Context, filePath string, opts *GetOptions) (*GetResult, error) {
	b.mu.RLock()
	store := b.store
	b.mu.RUnlock()

	if store == nil {
		return nil, &BackendError{Op: "get", Path: filePath, Err: fmt.Errorf("nats object store not configured")}
	}

	name := b.fullName(filePath)

	// Handle conditional get
	if opts != nil && opts.IfNoneMatch != "" {
		info, err := store.GetInfo(ctx, name)
		if err != nil {
			if isNATSNotFound(err) {
				return nil, ErrNotFound
			}
			return nil, &BackendError{Op: "get", Path: filePath, Err: err}
		}
		// Check if digest matches
		if info.Digest == opts.IfNoneMatch {
			return &GetResult{NotModified: true}, nil
		}
	}

	obj, err := store.Get(ctx, name)
	if err != nil {
		if isNATSNotFound(err) {
			return nil, ErrNotFound
		}
		return nil, &BackendError{Op: "get", Path: filePath, Err: err}
	}

	// Extract content type from headers
	contentType := ""
	if obj.Info.Headers != nil {
		if ct, ok := obj.Info.Headers["Content-Type"]; ok && len(ct) > 0 {
			contentType = ct[0]
		}
	}

	// Convert headers to metadata map
	metadata := make(map[string]string)
	for k, v := range obj.Info.Headers {
		if len(v) > 0 {
			metadata[k] = v[0]
		}
	}

	return &GetResult{
		Reader: obj.Body,
		Info: &FileInfo{
			Name:         path.Base(filePath),
			Path:         filePath,
			Size:         int64(obj.Info.Size),
			ContentType:  contentType,
			Checksum:     obj.Info.Digest,
			ModifiedTime: obj.Info.ModTime,
			ETag:         obj.Info.Digest,
			Metadata:     metadata,
		},
	}, nil
}

// Put uploads a file to NATS Object Store.
func (b *NATSBackend) Put(ctx context.Context, filePath string, reader io.Reader, opts *PutOptions) (*PutResult, error) {
	if b.config.ReadOnly {
		return nil, &BackendError{Op: "put", Path: filePath, Err: ErrReadOnly}
	}

	b.mu.RLock()
	store := b.store
	b.mu.RUnlock()

	if store == nil {
		return nil, &BackendError{Op: "put", Path: filePath, Err: fmt.Errorf("nats object store not configured")}
	}

	name := b.fullName(filePath)

	// Read content to calculate checksum
	content, err := io.ReadAll(reader)
	if err != nil {
		return nil, &BackendError{Op: "put", Path: filePath, Err: fmt.Errorf("read content: %w", err)}
	}

	// Calculate SHA256
	hash := sha256.Sum256(content)
	checksum := hex.EncodeToString(hash[:])

	natsOpts := &NATSPutOptions{
		Headers: map[string]string{
			"sha256": checksum,
		},
	}

	// Set content type
	if opts != nil && opts.ContentType != "" {
		natsOpts.Headers["Content-Type"] = opts.ContentType
	} else {
		natsOpts.Headers["Content-Type"] = http.DetectContentType(content)
	}

	// Merge additional metadata
	if opts != nil && opts.Metadata != nil {
		for k, v := range opts.Metadata {
			natsOpts.Headers[k] = v
		}
	}

	info, err := store.Put(ctx, name, bytes.NewReader(content), natsOpts)
	if err != nil {
		return nil, &BackendError{Op: "put", Path: filePath, Err: err}
	}

	return &PutResult{
		Size:     int64(info.Size),
		Checksum: checksum,
		ETag:     info.Digest,
	}, nil
}

// Delete removes a file from NATS Object Store.
func (b *NATSBackend) Delete(ctx context.Context, filePath string) error {
	if b.config.ReadOnly {
		return &BackendError{Op: "delete", Path: filePath, Err: ErrReadOnly}
	}

	b.mu.RLock()
	store := b.store
	b.mu.RUnlock()

	if store == nil {
		return &BackendError{Op: "delete", Path: filePath, Err: fmt.Errorf("nats object store not configured")}
	}

	name := b.fullName(filePath)

	if err := store.Delete(ctx, name); err != nil {
		if isNATSNotFound(err) {
			return nil // Idempotent delete
		}
		return &BackendError{Op: "delete", Path: filePath, Err: err}
	}

	return nil
}

// Exists checks if a file exists in NATS Object Store.
func (b *NATSBackend) Exists(ctx context.Context, filePath string) (bool, error) {
	b.mu.RLock()
	store := b.store
	b.mu.RUnlock()

	if store == nil {
		return false, &BackendError{Op: "exists", Path: filePath, Err: fmt.Errorf("nats object store not configured")}
	}

	name := b.fullName(filePath)

	info, err := store.GetInfo(ctx, name)
	if err != nil {
		if isNATSNotFound(err) {
			return false, nil
		}
		return false, &BackendError{Op: "exists", Path: filePath, Err: err}
	}

	return !info.Deleted, nil
}

// Stat returns file metadata from NATS Object Store.
func (b *NATSBackend) Stat(ctx context.Context, filePath string) (*FileInfo, error) {
	b.mu.RLock()
	store := b.store
	b.mu.RUnlock()

	if store == nil {
		return nil, &BackendError{Op: "stat", Path: filePath, Err: fmt.Errorf("nats object store not configured")}
	}

	name := b.fullName(filePath)

	info, err := store.GetInfo(ctx, name)
	if err != nil {
		if isNATSNotFound(err) {
			return nil, ErrNotFound
		}
		return nil, &BackendError{Op: "stat", Path: filePath, Err: err}
	}

	if info.Deleted {
		return nil, ErrNotFound
	}

	// Extract content type from headers
	contentType := ""
	if info.Headers != nil {
		if ct, ok := info.Headers["Content-Type"]; ok && len(ct) > 0 {
			contentType = ct[0]
		}
	}

	// Convert headers to metadata map
	metadata := make(map[string]string)
	for k, v := range info.Headers {
		if len(v) > 0 {
			metadata[k] = v[0]
		}
	}

	// Extract SHA256 from headers or use digest
	checksum := info.Digest
	if info.Headers != nil {
		if sha, ok := info.Headers["sha256"]; ok && len(sha) > 0 {
			checksum = sha[0]
		}
	}

	return &FileInfo{
		Name:         path.Base(filePath),
		Path:         filePath,
		Size:         int64(info.Size),
		ContentType:  contentType,
		Checksum:     checksum,
		ModifiedTime: info.ModTime,
		ETag:         info.Digest,
		Metadata:     metadata,
	}, nil
}

// List lists files in NATS Object Store with a prefix.
func (b *NATSBackend) List(ctx context.Context, prefix string, opts *ListOptions) (*ListResult, error) {
	b.mu.RLock()
	store := b.store
	b.mu.RUnlock()

	if store == nil {
		return nil, &BackendError{Op: "list", Path: prefix, Err: fmt.Errorf("nats object store not configured")}
	}

	fullPrefix := b.fullName(prefix)

	natsOpts := &NATSListOptions{}
	if fullPrefix != "" {
		natsOpts.FilterPrefix = fullPrefix
	}

	objects, err := store.List(ctx, natsOpts)
	if err != nil {
		return nil, &BackendError{Op: "list", Path: prefix, Err: err}
	}

	var files []FileInfo
	for _, obj := range objects {
		if obj.Deleted {
			continue
		}

		// Remove the base prefix to get the relative path
		relPath := "/" + strings.TrimPrefix(obj.Name, b.config.Prefix)
		if b.config.Prefix != "" {
			relPath = "/" + strings.TrimPrefix(obj.Name, b.config.Prefix+"/")
		}

		// Extract content type from headers
		contentType := ""
		if obj.Headers != nil {
			if ct, ok := obj.Headers["Content-Type"]; ok && len(ct) > 0 {
				contentType = ct[0]
			}
		}

		// Extract SHA256 from headers
		checksum := obj.Digest
		if obj.Headers != nil {
			if sha, ok := obj.Headers["sha256"]; ok && len(sha) > 0 {
				checksum = sha[0]
			}
		}

		files = append(files, FileInfo{
			Name:         path.Base(obj.Name),
			Path:         relPath,
			Size:         int64(obj.Size),
			ContentType:  contentType,
			Checksum:     checksum,
			ModifiedTime: obj.ModTime,
			ETag:         obj.Digest,
		})
	}

	return &ListResult{
		Files:     files,
		Truncated: false,
	}, nil
}

// Health checks NATS Object Store connectivity.
func (b *NATSBackend) Health(ctx context.Context) error {
	b.mu.RLock()
	store := b.store
	b.mu.RUnlock()

	if store == nil {
		return fmt.Errorf("nats object store not configured")
	}

	_, err := store.Status(ctx)
	if err != nil {
		return fmt.Errorf("nats health check failed: %w", err)
	}

	return nil
}

// Close closes the NATS backend.
func (b *NATSBackend) Close() error {
	return nil
}

// fullName returns the full object name including the prefix.
func (b *NATSBackend) fullName(filePath string) string {
	// Remove leading slash
	name := strings.TrimPrefix(filePath, "/")

	if b.config.Prefix != "" {
		name = b.config.Prefix + "/" + name
	}

	return name
}

// isNATSNotFound checks if an error indicates the object was not found.
func isNATSNotFound(err error) bool {
	if err == nil {
		return false
	}
	errStr := err.Error()
	return strings.Contains(errStr, "object not found") ||
		strings.Contains(errStr, "not found") ||
		strings.Contains(errStr, "no message found")
}
