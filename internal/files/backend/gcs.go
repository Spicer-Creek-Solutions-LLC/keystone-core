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

// GCSConfig is the configuration for the Google Cloud Storage backend.
type GCSConfig struct {
	Config `yaml:",inline"`

	// Bucket is the GCS bucket name.
	Bucket string `yaml:"bucket"`

	// Project is the GCP project ID.
	Project string `yaml:"project,omitempty"`

	// CredentialsFile is the path to a service account JSON file.
	CredentialsFile string `yaml:"credentials_file,omitempty"`

	// CredentialsJSON is the service account JSON content (alternative to file).
	CredentialsJSON string `yaml:"credentials_json,omitempty"`

	// Prefix is an optional prefix for all objects in the bucket.
	Prefix string `yaml:"prefix,omitempty"`

	// StorageClass sets the storage class for uploaded objects.
	// Options: STANDARD, NEARLINE, COLDLINE, ARCHIVE
	StorageClass string `yaml:"storage_class,omitempty"`

	// CustomerEncryptionKey is a base64-encoded AES-256 key for CSEK.
	CustomerEncryptionKey string `yaml:"customer_encryption_key,omitempty"`

	// KMSKeyName is the Cloud KMS key resource name for CMEK.
	KMSKeyName string `yaml:"kms_key_name,omitempty"`
}

// GCSBackend implements the Backend interface for Google Cloud Storage.
// This is a stub implementation that provides the interface without
// actual GCS SDK dependencies. For production use, wire up the cloud.google.com/go/storage client.
type GCSBackend struct {
	config *GCSConfig
	client GCSClient
	mu     sync.RWMutex
}

// GCSClient is an interface for GCS operations.
// This allows for mocking in tests and alternative implementations.
type GCSClient interface {
	// GetObject retrieves an object from GCS.
	GetObject(ctx context.Context, bucket, object string, opts *GCSGetOptions) (*GCSObject, error)

	// PutObject uploads an object to GCS.
	PutObject(ctx context.Context, bucket, object string, body io.Reader, opts *GCSPutOptions) (*GCSPutResult, error)

	// DeleteObject deletes an object from GCS.
	DeleteObject(ctx context.Context, bucket, object string) error

	// GetObjectAttrs gets object metadata without downloading the body.
	GetObjectAttrs(ctx context.Context, bucket, object string) (*GCSObjectAttrs, error)

	// ListObjects lists objects with a prefix.
	ListObjects(ctx context.Context, bucket string, opts *GCSListOptions) (*GCSListResult, error)

	// BucketExists checks if the bucket exists and is accessible.
	BucketExists(ctx context.Context, bucket string) error
}

// GCSGetOptions are options for GetObject.
type GCSGetOptions struct {
	Offset            int64
	Length            int64
	IfGenerationMatch int64
	IfMetagenMatch    int64
}

// GCSPutOptions are options for PutObject.
type GCSPutOptions struct {
	ContentType     string
	StorageClass    string
	KMSKeyName      string
	Metadata        map[string]string
	CacheControl    string
	ContentEncoding string
}

// GCSObject is the result of GetObject.
type GCSObject struct {
	Body           io.ReadCloser
	ContentType    string
	Size           int64
	Generation     int64
	Metageneration int64
	Updated        time.Time
	MD5            []byte
	CRC32C         uint32
	Metadata       map[string]string
}

// GCSPutResult is the result of PutObject.
type GCSPutResult struct {
	Generation     int64
	Metageneration int64
	MD5            []byte
}

// GCSObjectAttrs is the result of GetObjectAttrs.
type GCSObjectAttrs struct {
	Name           string
	ContentType    string
	Size           int64
	Generation     int64
	Metageneration int64
	Updated        time.Time
	MD5            []byte
	CRC32C         uint32
	Metadata       map[string]string
	StorageClass   string
}

// GCSListOptions are options for ListObjects.
type GCSListOptions struct {
	Prefix     string
	Delimiter  string
	MaxResults int
	PageToken  string
}

// GCSListResult is the result of ListObjects.
type GCSListResult struct {
	Objects       []GCSListObject
	Prefixes      []string
	NextPageToken string
}

// GCSListObject is an object in a list result.
type GCSListObject struct {
	Name       string
	Size       int64
	Generation int64
	Updated    time.Time
	MD5        []byte
}

// NewGCSBackend creates a new GCS backend.
func NewGCSBackend(config *GCSConfig) (*GCSBackend, error) {
	if config.Bucket == "" {
		return nil, fmt.Errorf("gcs backend: bucket is required")
	}

	return &GCSBackend{
		config: config,
	}, nil
}

// SetClient sets the GCS client for the backend.
// This should be called after creating the backend to wire up the actual GCS client.
func (b *GCSBackend) SetClient(client GCSClient) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.client = client
}

// Name returns the backend name.
func (b *GCSBackend) Name() string {
	return b.config.Name
}

// Type returns the backend type.
func (b *GCSBackend) Type() Type {
	return TypeGCS
}

// BaseConfig returns the base configuration for path matching and priority.
func (b *GCSBackend) BaseConfig() *Config {
	return &b.config.Config
}

// Get retrieves a file from GCS.
func (b *GCSBackend) Get(ctx context.Context, filePath string, opts *GetOptions) (*GetResult, error) {
	b.mu.RLock()
	client := b.client
	b.mu.RUnlock()

	if client == nil {
		return nil, &Error{Op: "get", Path: filePath, Err: fmt.Errorf("gcs client not configured")}
	}

	object := b.fullObject(filePath)

	gcsOpts := &GCSGetOptions{}
	if opts != nil && opts.Range != nil {
		gcsOpts.Offset = opts.Range.Start
		if opts.Range.End > 0 {
			gcsOpts.Length = opts.Range.End - opts.Range.Start
		}
	}

	// Handle conditional get
	if opts != nil && opts.IfNoneMatch != "" {
		// Check current metadata first
		attrs, err := client.GetObjectAttrs(ctx, b.config.Bucket, object)
		if err != nil {
			if isGCSNotFound(err) {
				return nil, ErrNotFound
			}
			return nil, &Error{Op: "get", Path: filePath, Err: err}
		}
		// Check if SHA256 matches
		if sha, ok := attrs.Metadata["sha256"]; ok && sha == opts.IfNoneMatch {
			return &GetResult{NotModified: true}, nil
		}
	}

	obj, err := client.GetObject(ctx, b.config.Bucket, object, gcsOpts)
	if err != nil {
		if isGCSNotFound(err) {
			return nil, ErrNotFound
		}
		return nil, &Error{Op: "get", Path: filePath, Err: err}
	}

	checksum := ""
	if obj.Metadata != nil {
		if sha, ok := obj.Metadata["sha256"]; ok {
			checksum = sha
		}
	}

	return &GetResult{
		Reader: obj.Body,
		Info: &FileInfo{
			Name:         path.Base(filePath),
			Path:         filePath,
			Size:         obj.Size,
			ContentType:  obj.ContentType,
			Checksum:     checksum,
			ModifiedTime: obj.Updated,
			ETag:         fmt.Sprintf("%d", obj.Generation),
			Metadata:     obj.Metadata,
		},
	}, nil
}

// Put uploads a file to GCS.
func (b *GCSBackend) Put(ctx context.Context, filePath string, reader io.Reader, opts *PutOptions) (*PutResult, error) {
	if b.config.ReadOnly {
		return nil, &Error{Op: "put", Path: filePath, Err: ErrReadOnly}
	}

	b.mu.RLock()
	client := b.client
	b.mu.RUnlock()

	if client == nil {
		return nil, &Error{Op: "put", Path: filePath, Err: fmt.Errorf("gcs client not configured")}
	}

	object := b.fullObject(filePath)

	// Read content to calculate checksum
	content, err := io.ReadAll(reader)
	if err != nil {
		return nil, &Error{Op: "put", Path: filePath, Err: fmt.Errorf("read content: %w", err)}
	}

	// Calculate SHA256
	hash := sha256.Sum256(content)
	checksum := hex.EncodeToString(hash[:])

	gcsOpts := &GCSPutOptions{
		StorageClass: b.config.StorageClass,
		KMSKeyName:   b.config.KMSKeyName,
		Metadata:     map[string]string{"sha256": checksum},
	}
	if opts != nil && opts.ContentType != "" {
		gcsOpts.ContentType = opts.ContentType
	} else {
		gcsOpts.ContentType = http.DetectContentType(content)
	}
	if opts != nil && opts.Metadata != nil {
		for k, v := range opts.Metadata {
			gcsOpts.Metadata[k] = v
		}
	}

	result, err := client.PutObject(ctx, b.config.Bucket, object, bytes.NewReader(content), gcsOpts)
	if err != nil {
		return nil, &Error{Op: "put", Path: filePath, Err: err}
	}

	return &PutResult{
		Size:     int64(len(content)),
		Checksum: checksum,
		ETag:     fmt.Sprintf("%d", result.Generation),
	}, nil
}

// Delete removes a file from GCS.
func (b *GCSBackend) Delete(ctx context.Context, filePath string) error {
	if b.config.ReadOnly {
		return &Error{Op: "delete", Path: filePath, Err: ErrReadOnly}
	}

	b.mu.RLock()
	client := b.client
	b.mu.RUnlock()

	if client == nil {
		return &Error{Op: "delete", Path: filePath, Err: fmt.Errorf("gcs client not configured")}
	}

	object := b.fullObject(filePath)

	if err := client.DeleteObject(ctx, b.config.Bucket, object); err != nil {
		if isGCSNotFound(err) {
			return nil // Idempotent delete
		}
		return &Error{Op: "delete", Path: filePath, Err: err}
	}

	return nil
}

// Exists checks if a file exists in GCS.
func (b *GCSBackend) Exists(ctx context.Context, filePath string) (bool, error) {
	b.mu.RLock()
	client := b.client
	b.mu.RUnlock()

	if client == nil {
		return false, &Error{Op: "exists", Path: filePath, Err: fmt.Errorf("gcs client not configured")}
	}

	object := b.fullObject(filePath)

	_, err := client.GetObjectAttrs(ctx, b.config.Bucket, object)
	if err != nil {
		if isGCSNotFound(err) {
			return false, nil
		}
		return false, &Error{Op: "exists", Path: filePath, Err: err}
	}

	return true, nil
}

// Stat returns file metadata from GCS.
func (b *GCSBackend) Stat(ctx context.Context, filePath string) (*FileInfo, error) {
	b.mu.RLock()
	client := b.client
	b.mu.RUnlock()

	if client == nil {
		return nil, &Error{Op: "stat", Path: filePath, Err: fmt.Errorf("gcs client not configured")}
	}

	object := b.fullObject(filePath)

	attrs, err := client.GetObjectAttrs(ctx, b.config.Bucket, object)
	if err != nil {
		if isGCSNotFound(err) {
			return nil, ErrNotFound
		}
		return nil, &Error{Op: "stat", Path: filePath, Err: err}
	}

	checksum := ""
	if attrs.Metadata != nil {
		if sha, ok := attrs.Metadata["sha256"]; ok {
			checksum = sha
		}
	}

	return &FileInfo{
		Name:         path.Base(filePath),
		Path:         filePath,
		Size:         attrs.Size,
		ContentType:  attrs.ContentType,
		Checksum:     checksum,
		ModifiedTime: attrs.Updated,
		ETag:         fmt.Sprintf("%d", attrs.Generation),
		Metadata:     attrs.Metadata,
	}, nil
}

// List lists files in GCS with a prefix.
func (b *GCSBackend) List(ctx context.Context, prefix string, opts *ListOptions) (*ListResult, error) {
	b.mu.RLock()
	client := b.client
	b.mu.RUnlock()

	if client == nil {
		return nil, &Error{Op: "list", Path: prefix, Err: fmt.Errorf("gcs client not configured")}
	}

	fullPrefix := b.fullObject(prefix)
	// Ensure prefix ends with / for directory-like listing
	if fullPrefix != "" && !strings.HasSuffix(fullPrefix, "/") {
		fullPrefix += "/"
	}

	gcsOpts := &GCSListOptions{
		Prefix:     fullPrefix,
		MaxResults: 1000,
	}
	if opts != nil && !opts.Recursive {
		gcsOpts.Delimiter = "/"
	}

	var files []FileInfo
	pageToken := ""

	for {
		gcsOpts.PageToken = pageToken

		result, err := client.ListObjects(ctx, b.config.Bucket, gcsOpts)
		if err != nil {
			return nil, &Error{Op: "list", Path: prefix, Err: err}
		}

		for _, obj := range result.Objects {
			// Remove the base prefix to get the relative path
			relPath := "/" + strings.TrimPrefix(obj.Name, b.config.Prefix)
			if b.config.Prefix != "" {
				relPath = "/" + strings.TrimPrefix(obj.Name, b.config.Prefix+"/")
			}

			files = append(files, FileInfo{
				Name:         path.Base(obj.Name),
				Path:         relPath,
				Size:         obj.Size,
				ModifiedTime: obj.Updated,
				ETag:         fmt.Sprintf("%d", obj.Generation),
			})
		}

		// Add "directories" from prefixes
		for _, p := range result.Prefixes {
			relPath := "/" + strings.TrimPrefix(p, b.config.Prefix)
			if b.config.Prefix != "" {
				relPath = "/" + strings.TrimPrefix(p, b.config.Prefix+"/")
			}
			relPath = strings.TrimSuffix(relPath, "/")

			files = append(files, FileInfo{
				Name:        path.Base(relPath),
				Path:        relPath,
				IsDirectory: true,
			})
		}

		if result.NextPageToken == "" {
			break
		}
		pageToken = result.NextPageToken
	}

	return &ListResult{
		Files:     files,
		Truncated: false,
	}, nil
}

// Health checks GCS connectivity.
func (b *GCSBackend) Health(ctx context.Context) error {
	b.mu.RLock()
	client := b.client
	b.mu.RUnlock()

	if client == nil {
		return fmt.Errorf("gcs client not configured")
	}

	if err := client.BucketExists(ctx, b.config.Bucket); err != nil {
		return fmt.Errorf("gcs health check failed: %w", err)
	}

	return nil
}

// Close closes the GCS backend.
func (b *GCSBackend) Close() error {
	return nil
}

// fullObject returns the full GCS object name including the prefix.
func (b *GCSBackend) fullObject(filePath string) string {
	// Remove leading slash
	object := strings.TrimPrefix(filePath, "/")

	if b.config.Prefix != "" {
		object = b.config.Prefix + "/" + object
	}

	return object
}

// isGCSNotFound checks if an error indicates the object was not found.
func isGCSNotFound(err error) bool {
	if err == nil {
		return false
	}
	errStr := err.Error()
	return strings.Contains(errStr, "object doesn't exist") ||
		strings.Contains(errStr, "storage: object doesn't exist") ||
		strings.Contains(errStr, "404")
}
