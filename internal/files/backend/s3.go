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

// S3Config is the configuration for the S3 backend.
type S3Config struct {
	Config `yaml:",inline"`

	// Bucket is the S3 bucket name.
	Bucket string `yaml:"bucket"`

	// Region is the AWS region.
	Region string `yaml:"region"`

	// Endpoint is the S3-compatible endpoint URL (for MinIO, etc.).
	Endpoint string `yaml:"endpoint,omitempty"`

	// AccessKeyID is the AWS access key ID.
	AccessKeyID string `yaml:"access_key_id,omitempty"`

	// SecretAccessKey is the AWS secret access key.
	SecretAccessKey string `yaml:"secret_access_key,omitempty"`

	// SessionToken is the AWS session token (for temporary credentials).
	SessionToken string `yaml:"session_token,omitempty"`

	// Profile is the AWS credentials profile to use.
	Profile string `yaml:"profile,omitempty"`

	// UsePathStyle forces path-style URLs (for MinIO compatibility).
	UsePathStyle bool `yaml:"use_path_style,omitempty"`

	// Prefix is an optional prefix for all keys in the bucket.
	Prefix string `yaml:"prefix,omitempty"`

	// ServerSideEncryption sets the SSE mode (AES256, aws:kms).
	ServerSideEncryption string `yaml:"server_side_encryption,omitempty"`

	// KMSKeyID is the KMS key ID for SSE-KMS.
	KMSKeyID string `yaml:"kms_key_id,omitempty"`

	// StorageClass sets the storage class for uploaded objects.
	StorageClass string `yaml:"storage_class,omitempty"`
}

// S3Backend implements the Backend interface for AWS S3.
// This is a stub implementation that provides the interface without
// actual AWS SDK dependencies. For production use, wire up the AWS SDK.
type S3Backend struct {
	config *S3Config
	client S3Client
	mu     sync.RWMutex
}

// S3Client is an interface for S3 operations.
// This allows for mocking in tests and alternative implementations.
type S3Client interface {
	// GetObject retrieves an object from S3.
	GetObject(ctx context.Context, bucket, key string, opts *S3GetOptions) (*S3Object, error)

	// PutObject uploads an object to S3.
	PutObject(ctx context.Context, bucket, key string, body io.Reader, opts *S3PutOptions) (*S3PutResult, error)

	// DeleteObject deletes an object from S3.
	DeleteObject(ctx context.Context, bucket, key string) error

	// HeadObject gets object metadata without downloading the body.
	HeadObject(ctx context.Context, bucket, key string) (*S3ObjectInfo, error)

	// ListObjects lists objects with a prefix.
	ListObjects(ctx context.Context, bucket, prefix string, opts *S3ListOptions) (*S3ListResult, error)

	// HeadBucket checks if the bucket exists and is accessible.
	HeadBucket(ctx context.Context, bucket string) error
}

// S3GetOptions are options for GetObject.
type S3GetOptions struct {
	Range       *ByteRange
	IfNoneMatch string
}

// S3PutOptions are options for PutObject.
type S3PutOptions struct {
	ContentType          string
	ServerSideEncryption string
	KMSKeyID             string
	StorageClass         string
	Metadata             map[string]string
}

// S3Object is the result of GetObject.
type S3Object struct {
	Body         io.ReadCloser
	ContentType  string
	Size         int64
	ETag         string
	LastModified time.Time
	Metadata     map[string]string
}

// S3PutResult is the result of PutObject.
type S3PutResult struct {
	ETag string
}

// S3ObjectInfo is the result of HeadObject.
type S3ObjectInfo struct {
	ContentType  string
	Size         int64
	ETag         string
	LastModified time.Time
	Metadata     map[string]string
}

// S3ListOptions are options for ListObjects.
type S3ListOptions struct {
	Delimiter        string
	MaxKeys          int32
	ContinuationToken string
}

// S3ListResult is the result of ListObjects.
type S3ListResult struct {
	Contents              []S3ListObject
	CommonPrefixes        []string
	NextContinuationToken string
	IsTruncated           bool
}

// S3ListObject is an object in a list result.
type S3ListObject struct {
	Key          string
	Size         int64
	ETag         string
	LastModified time.Time
}

// NewS3Backend creates a new S3 backend.
func NewS3Backend(config *S3Config) (*S3Backend, error) {
	if config.Bucket == "" {
		return nil, fmt.Errorf("s3 backend: bucket is required")
	}
	if config.Region == "" && config.Endpoint == "" {
		return nil, fmt.Errorf("s3 backend: region or endpoint is required")
	}

	return &S3Backend{
		config: config,
	}, nil
}

// SetClient sets the S3 client for the backend.
// This should be called after creating the backend to wire up the actual AWS SDK client.
func (b *S3Backend) SetClient(client S3Client) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.client = client
}

// Name returns the backend name.
func (b *S3Backend) Name() string {
	return b.config.Name
}

// Type returns the backend type.
func (b *S3Backend) Type() BackendType {
	return BackendTypeS3
}

// BaseConfig returns the base configuration for path matching and priority.
func (b *S3Backend) BaseConfig() *Config {
	return &b.config.Config
}

// Get retrieves a file from S3.
func (b *S3Backend) Get(ctx context.Context, filePath string, opts *GetOptions) (*GetResult, error) {
	b.mu.RLock()
	client := b.client
	b.mu.RUnlock()

	if client == nil {
		return nil, &BackendError{Op: "get", Path: filePath, Err: fmt.Errorf("s3 client not configured")}
	}

	key := b.fullKey(filePath)

	s3Opts := &S3GetOptions{}
	if opts != nil {
		s3Opts.Range = opts.Range
		s3Opts.IfNoneMatch = opts.IfNoneMatch
	}

	obj, err := client.GetObject(ctx, b.config.Bucket, key, s3Opts)
	if err != nil {
		if isS3NotFound(err) {
			return nil, ErrNotFound
		}
		// Check for conditional get (304 Not Modified)
		if isS3NotModified(err) {
			return &GetResult{NotModified: true}, nil
		}
		return nil, &BackendError{Op: "get", Path: filePath, Err: err}
	}

	// Parse checksum from ETag (S3 ETags for single-part uploads are MD5, but we store SHA256 in metadata)
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
			ModifiedTime: obj.LastModified,
			ETag:         obj.ETag,
			Metadata:     obj.Metadata,
		},
	}, nil
}

// Put uploads a file to S3.
func (b *S3Backend) Put(ctx context.Context, filePath string, reader io.Reader, opts *PutOptions) (*PutResult, error) {
	if b.config.ReadOnly {
		return nil, &BackendError{Op: "put", Path: filePath, Err: ErrReadOnly}
	}

	b.mu.RLock()
	client := b.client
	b.mu.RUnlock()

	if client == nil {
		return nil, &BackendError{Op: "put", Path: filePath, Err: fmt.Errorf("s3 client not configured")}
	}

	key := b.fullKey(filePath)

	// Read content to calculate checksum
	content, err := io.ReadAll(reader)
	if err != nil {
		return nil, &BackendError{Op: "put", Path: filePath, Err: fmt.Errorf("read content: %w", err)}
	}

	// Calculate SHA256
	hash := sha256.Sum256(content)
	checksum := hex.EncodeToString(hash[:])

	s3Opts := &S3PutOptions{
		ServerSideEncryption: b.config.ServerSideEncryption,
		KMSKeyID:             b.config.KMSKeyID,
		StorageClass:         b.config.StorageClass,
		Metadata:             map[string]string{"sha256": checksum},
	}
	if opts != nil && opts.ContentType != "" {
		s3Opts.ContentType = opts.ContentType
	} else {
		s3Opts.ContentType = http.DetectContentType(content)
	}
	if opts != nil && opts.Metadata != nil {
		for k, v := range opts.Metadata {
			s3Opts.Metadata[k] = v
		}
	}

	result, err := client.PutObject(ctx, b.config.Bucket, key, bytes.NewReader(content), s3Opts)
	if err != nil {
		return nil, &BackendError{Op: "put", Path: filePath, Err: err}
	}

	return &PutResult{
		Size:     int64(len(content)),
		Checksum: checksum,
		ETag:     result.ETag,
	}, nil
}

// Delete removes a file from S3.
func (b *S3Backend) Delete(ctx context.Context, filePath string) error {
	if b.config.ReadOnly {
		return &BackendError{Op: "delete", Path: filePath, Err: ErrReadOnly}
	}

	b.mu.RLock()
	client := b.client
	b.mu.RUnlock()

	if client == nil {
		return &BackendError{Op: "delete", Path: filePath, Err: fmt.Errorf("s3 client not configured")}
	}

	key := b.fullKey(filePath)

	if err := client.DeleteObject(ctx, b.config.Bucket, key); err != nil {
		if isS3NotFound(err) {
			return nil // Idempotent delete
		}
		return &BackendError{Op: "delete", Path: filePath, Err: err}
	}

	return nil
}

// Exists checks if a file exists in S3.
func (b *S3Backend) Exists(ctx context.Context, filePath string) (bool, error) {
	b.mu.RLock()
	client := b.client
	b.mu.RUnlock()

	if client == nil {
		return false, &BackendError{Op: "exists", Path: filePath, Err: fmt.Errorf("s3 client not configured")}
	}

	key := b.fullKey(filePath)

	_, err := client.HeadObject(ctx, b.config.Bucket, key)
	if err != nil {
		if isS3NotFound(err) {
			return false, nil
		}
		return false, &BackendError{Op: "exists", Path: filePath, Err: err}
	}

	return true, nil
}

// Stat returns file metadata from S3.
func (b *S3Backend) Stat(ctx context.Context, filePath string) (*FileInfo, error) {
	b.mu.RLock()
	client := b.client
	b.mu.RUnlock()

	if client == nil {
		return nil, &BackendError{Op: "stat", Path: filePath, Err: fmt.Errorf("s3 client not configured")}
	}

	key := b.fullKey(filePath)

	info, err := client.HeadObject(ctx, b.config.Bucket, key)
	if err != nil {
		if isS3NotFound(err) {
			return nil, ErrNotFound
		}
		return nil, &BackendError{Op: "stat", Path: filePath, Err: err}
	}

	checksum := ""
	if info.Metadata != nil {
		if sha, ok := info.Metadata["sha256"]; ok {
			checksum = sha
		}
	}

	return &FileInfo{
		Name:         path.Base(filePath),
		Path:         filePath,
		Size:         info.Size,
		ContentType:  info.ContentType,
		Checksum:     checksum,
		ModifiedTime: info.LastModified,
		ETag:         info.ETag,
		Metadata:     info.Metadata,
	}, nil
}

// List lists files in S3 with a prefix.
func (b *S3Backend) List(ctx context.Context, prefix string, opts *ListOptions) (*ListResult, error) {
	b.mu.RLock()
	client := b.client
	b.mu.RUnlock()

	if client == nil {
		return nil, &BackendError{Op: "list", Path: prefix, Err: fmt.Errorf("s3 client not configured")}
	}

	fullPrefix := b.fullKey(prefix)
	// Ensure prefix ends with / for directory-like listing
	if fullPrefix != "" && !strings.HasSuffix(fullPrefix, "/") {
		fullPrefix += "/"
	}

	s3Opts := &S3ListOptions{
		MaxKeys: 1000,
	}
	if opts != nil && !opts.Recursive {
		s3Opts.Delimiter = "/"
	}

	var files []FileInfo
	continuationToken := ""

	for {
		s3Opts.ContinuationToken = continuationToken

		result, err := client.ListObjects(ctx, b.config.Bucket, fullPrefix, s3Opts)
		if err != nil {
			return nil, &BackendError{Op: "list", Path: prefix, Err: err}
		}

		for _, obj := range result.Contents {
			// Remove the base prefix to get the relative path
			relPath := "/" + strings.TrimPrefix(obj.Key, b.config.Prefix)
			if b.config.Prefix != "" {
				relPath = "/" + strings.TrimPrefix(obj.Key, b.config.Prefix+"/")
			}

			files = append(files, FileInfo{
				Name:         path.Base(obj.Key),
				Path:         relPath,
				Size:         obj.Size,
				ModifiedTime: obj.LastModified,
				ETag:         obj.ETag,
			})
		}

		// Add "directories" from common prefixes
		for _, cp := range result.CommonPrefixes {
			relPath := "/" + strings.TrimPrefix(cp, b.config.Prefix)
			if b.config.Prefix != "" {
				relPath = "/" + strings.TrimPrefix(cp, b.config.Prefix+"/")
			}
			relPath = strings.TrimSuffix(relPath, "/")

			files = append(files, FileInfo{
				Name:        path.Base(relPath),
				Path:        relPath,
				IsDirectory: true,
			})
		}

		if !result.IsTruncated {
			break
		}
		continuationToken = result.NextContinuationToken
	}

	return &ListResult{
		Files:    files,
		Truncated: false, // We paginate through all results
	}, nil
}

// Health checks S3 connectivity.
func (b *S3Backend) Health(ctx context.Context) error {
	b.mu.RLock()
	client := b.client
	b.mu.RUnlock()

	if client == nil {
		return fmt.Errorf("s3 client not configured")
	}

	if err := client.HeadBucket(ctx, b.config.Bucket); err != nil {
		return fmt.Errorf("s3 health check failed: %w", err)
	}

	return nil
}

// Close closes the S3 backend.
func (b *S3Backend) Close() error {
	return nil
}

// fullKey returns the full S3 key including the prefix.
func (b *S3Backend) fullKey(filePath string) string {
	// Remove leading slash
	key := strings.TrimPrefix(filePath, "/")

	if b.config.Prefix != "" {
		key = b.config.Prefix + "/" + key
	}

	return key
}

// isS3NotFound checks if an error indicates the object was not found.
func isS3NotFound(err error) bool {
	if err == nil {
		return false
	}
	errStr := err.Error()
	return strings.Contains(errStr, "NotFound") ||
		strings.Contains(errStr, "NoSuchKey") ||
		strings.Contains(errStr, "404")
}

// isS3NotModified checks if an error indicates 304 Not Modified.
func isS3NotModified(err error) bool {
	if err == nil {
		return false
	}
	errStr := err.Error()
	return strings.Contains(errStr, "NotModified") ||
		strings.Contains(errStr, "304")
}
