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

// AzureConfig is the configuration for the Azure Blob Storage backend.
type AzureConfig struct {
	Config `yaml:",inline"`

	// Container is the Azure Blob container name.
	Container string `yaml:"container"`

	// AccountName is the Azure storage account name.
	AccountName string `yaml:"account_name"`

	// AccountKey is the Azure storage account access key.
	AccountKey string `yaml:"account_key,omitempty"`

	// ConnectionString is the full Azure connection string (alternative to account name/key).
	ConnectionString string `yaml:"connection_string,omitempty"`

	// SASToken is a Shared Access Signature token.
	SASToken string `yaml:"sas_token,omitempty"`

	// TenantID is the Azure AD tenant ID for service principal auth.
	TenantID string `yaml:"tenant_id,omitempty"`

	// ClientID is the Azure AD client ID for service principal auth.
	ClientID string `yaml:"client_id,omitempty"`

	// ClientSecret is the Azure AD client secret for service principal auth.
	ClientSecret string `yaml:"client_secret,omitempty"`

	// UseManagedIdentity enables Azure Managed Identity authentication.
	UseManagedIdentity bool `yaml:"use_managed_identity,omitempty"`

	// Endpoint is a custom Azure Blob endpoint (for Azure Stack or Azurite).
	Endpoint string `yaml:"endpoint,omitempty"`

	// Prefix is an optional prefix for all blobs in the container.
	Prefix string `yaml:"prefix,omitempty"`

	// AccessTier sets the access tier for uploaded blobs.
	// Options: Hot, Cool, Archive
	AccessTier string `yaml:"access_tier,omitempty"`
}

// AzureBackend implements the Backend interface for Azure Blob Storage.
// This is a stub implementation that provides the interface without
// actual Azure SDK dependencies. For production use, wire up the github.com/Azure/azure-sdk-for-go client.
type AzureBackend struct {
	config *AzureConfig
	client AzureClient
	mu     sync.RWMutex
}

// AzureClient is an interface for Azure Blob operations.
// This allows for mocking in tests and alternative implementations.
type AzureClient interface {
	// GetBlob downloads a blob from Azure.
	GetBlob(ctx context.Context, container, blob string, opts *AzureGetOptions) (*AzureBlob, error)

	// PutBlob uploads a blob to Azure.
	PutBlob(ctx context.Context, container, blob string, body io.Reader, opts *AzurePutOptions) (*AzurePutResult, error)

	// DeleteBlob deletes a blob from Azure.
	DeleteBlob(ctx context.Context, container, blob string) error

	// GetBlobProperties gets blob metadata without downloading the body.
	GetBlobProperties(ctx context.Context, container, blob string) (*AzureBlobProperties, error)

	// ListBlobs lists blobs with a prefix.
	ListBlobs(ctx context.Context, container string, opts *AzureListOptions) (*AzureListResult, error)

	// ContainerExists checks if the container exists and is accessible.
	ContainerExists(ctx context.Context, container string) error
}

// AzureGetOptions are options for GetBlob.
type AzureGetOptions struct {
	Offset int64
	Count  int64
	IfNoneMatch string
}

// AzurePutOptions are options for PutBlob.
type AzurePutOptions struct {
	ContentType string
	AccessTier  string
	Metadata    map[string]string
}

// AzureBlob is the result of GetBlob.
type AzureBlob struct {
	Body            io.ReadCloser
	ContentType     string
	ContentLength   int64
	ETag            string
	LastModified    time.Time
	Metadata        map[string]string
}

// AzurePutResult is the result of PutBlob.
type AzurePutResult struct {
	ETag string
}

// AzureBlobProperties is the result of GetBlobProperties.
type AzureBlobProperties struct {
	ContentType   string
	ContentLength int64
	ETag          string
	LastModified  time.Time
	Metadata      map[string]string
	AccessTier    string
	BlobType      string
}

// AzureListOptions are options for ListBlobs.
type AzureListOptions struct {
	Prefix     string
	Delimiter  string
	MaxResults int32
	Marker     string
}

// AzureListResult is the result of ListBlobs.
type AzureListResult struct {
	Blobs      []AzureListBlob
	Prefixes   []string
	NextMarker string
}

// AzureListBlob is a blob in a list result.
type AzureListBlob struct {
	Name         string
	Size         int64
	ETag         string
	LastModified time.Time
	Metadata     map[string]string
}

// NewAzureBackend creates a new Azure Blob Storage backend.
func NewAzureBackend(config *AzureConfig) (*AzureBackend, error) {
	if config.Container == "" {
		return nil, fmt.Errorf("azure backend: container is required")
	}
	if config.AccountName == "" && config.ConnectionString == "" {
		return nil, fmt.Errorf("azure backend: account_name or connection_string is required")
	}

	return &AzureBackend{
		config: config,
	}, nil
}

// SetClient sets the Azure client for the backend.
// This should be called after creating the backend to wire up the actual Azure SDK client.
func (b *AzureBackend) SetClient(client AzureClient) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.client = client
}

// Name returns the backend name.
func (b *AzureBackend) Name() string {
	return b.config.Name
}

// Type returns the backend type.
func (b *AzureBackend) Type() BackendType {
	return BackendTypeAzure
}

// BaseConfig returns the base configuration for path matching and priority.
func (b *AzureBackend) BaseConfig() *Config {
	return &b.config.Config
}

// Get retrieves a file from Azure Blob Storage.
func (b *AzureBackend) Get(ctx context.Context, filePath string, opts *GetOptions) (*GetResult, error) {
	b.mu.RLock()
	client := b.client
	b.mu.RUnlock()

	if client == nil {
		return nil, &BackendError{Op: "get", Path: filePath, Err: fmt.Errorf("azure client not configured")}
	}

	blob := b.fullBlob(filePath)

	azureOpts := &AzureGetOptions{}
	if opts != nil && opts.Range != nil {
		azureOpts.Offset = opts.Range.Start
		if opts.Range.End > 0 {
			azureOpts.Count = opts.Range.End - opts.Range.Start
		}
	}
	if opts != nil && opts.IfNoneMatch != "" {
		azureOpts.IfNoneMatch = opts.IfNoneMatch
	}

	result, err := client.GetBlob(ctx, b.config.Container, blob, azureOpts)
	if err != nil {
		if isAzureNotFound(err) {
			return nil, ErrNotFound
		}
		if isAzureNotModified(err) {
			return &GetResult{NotModified: true}, nil
		}
		return nil, &BackendError{Op: "get", Path: filePath, Err: err}
	}

	checksum := ""
	if result.Metadata != nil {
		if sha, ok := result.Metadata["sha256"]; ok {
			checksum = sha
		}
	}

	return &GetResult{
		Reader: result.Body,
		Info: &FileInfo{
			Name:         path.Base(filePath),
			Path:         filePath,
			Size:         result.ContentLength,
			ContentType:  result.ContentType,
			Checksum:     checksum,
			ModifiedTime: result.LastModified,
			ETag:         result.ETag,
			Metadata:     result.Metadata,
		},
	}, nil
}

// Put uploads a file to Azure Blob Storage.
func (b *AzureBackend) Put(ctx context.Context, filePath string, reader io.Reader, opts *PutOptions) (*PutResult, error) {
	if b.config.ReadOnly {
		return nil, &BackendError{Op: "put", Path: filePath, Err: ErrReadOnly}
	}

	b.mu.RLock()
	client := b.client
	b.mu.RUnlock()

	if client == nil {
		return nil, &BackendError{Op: "put", Path: filePath, Err: fmt.Errorf("azure client not configured")}
	}

	blob := b.fullBlob(filePath)

	// Read content to calculate checksum
	content, err := io.ReadAll(reader)
	if err != nil {
		return nil, &BackendError{Op: "put", Path: filePath, Err: fmt.Errorf("read content: %w", err)}
	}

	// Calculate SHA256
	hash := sha256.Sum256(content)
	checksum := hex.EncodeToString(hash[:])

	azureOpts := &AzurePutOptions{
		AccessTier: b.config.AccessTier,
		Metadata:   map[string]string{"sha256": checksum},
	}
	if opts != nil && opts.ContentType != "" {
		azureOpts.ContentType = opts.ContentType
	} else {
		azureOpts.ContentType = http.DetectContentType(content)
	}
	if opts != nil && opts.Metadata != nil {
		for k, v := range opts.Metadata {
			azureOpts.Metadata[k] = v
		}
	}

	result, err := client.PutBlob(ctx, b.config.Container, blob, bytes.NewReader(content), azureOpts)
	if err != nil {
		return nil, &BackendError{Op: "put", Path: filePath, Err: err}
	}

	return &PutResult{
		Size:     int64(len(content)),
		Checksum: checksum,
		ETag:     result.ETag,
	}, nil
}

// Delete removes a file from Azure Blob Storage.
func (b *AzureBackend) Delete(ctx context.Context, filePath string) error {
	if b.config.ReadOnly {
		return &BackendError{Op: "delete", Path: filePath, Err: ErrReadOnly}
	}

	b.mu.RLock()
	client := b.client
	b.mu.RUnlock()

	if client == nil {
		return &BackendError{Op: "delete", Path: filePath, Err: fmt.Errorf("azure client not configured")}
	}

	blob := b.fullBlob(filePath)

	if err := client.DeleteBlob(ctx, b.config.Container, blob); err != nil {
		if isAzureNotFound(err) {
			return nil // Idempotent delete
		}
		return &BackendError{Op: "delete", Path: filePath, Err: err}
	}

	return nil
}

// Exists checks if a file exists in Azure Blob Storage.
func (b *AzureBackend) Exists(ctx context.Context, filePath string) (bool, error) {
	b.mu.RLock()
	client := b.client
	b.mu.RUnlock()

	if client == nil {
		return false, &BackendError{Op: "exists", Path: filePath, Err: fmt.Errorf("azure client not configured")}
	}

	blob := b.fullBlob(filePath)

	_, err := client.GetBlobProperties(ctx, b.config.Container, blob)
	if err != nil {
		if isAzureNotFound(err) {
			return false, nil
		}
		return false, &BackendError{Op: "exists", Path: filePath, Err: err}
	}

	return true, nil
}

// Stat returns file metadata from Azure Blob Storage.
func (b *AzureBackend) Stat(ctx context.Context, filePath string) (*FileInfo, error) {
	b.mu.RLock()
	client := b.client
	b.mu.RUnlock()

	if client == nil {
		return nil, &BackendError{Op: "stat", Path: filePath, Err: fmt.Errorf("azure client not configured")}
	}

	blob := b.fullBlob(filePath)

	props, err := client.GetBlobProperties(ctx, b.config.Container, blob)
	if err != nil {
		if isAzureNotFound(err) {
			return nil, ErrNotFound
		}
		return nil, &BackendError{Op: "stat", Path: filePath, Err: err}
	}

	checksum := ""
	if props.Metadata != nil {
		if sha, ok := props.Metadata["sha256"]; ok {
			checksum = sha
		}
	}

	return &FileInfo{
		Name:         path.Base(filePath),
		Path:         filePath,
		Size:         props.ContentLength,
		ContentType:  props.ContentType,
		Checksum:     checksum,
		ModifiedTime: props.LastModified,
		ETag:         props.ETag,
		Metadata:     props.Metadata,
	}, nil
}

// List lists files in Azure Blob Storage with a prefix.
func (b *AzureBackend) List(ctx context.Context, prefix string, opts *ListOptions) (*ListResult, error) {
	b.mu.RLock()
	client := b.client
	b.mu.RUnlock()

	if client == nil {
		return nil, &BackendError{Op: "list", Path: prefix, Err: fmt.Errorf("azure client not configured")}
	}

	fullPrefix := b.fullBlob(prefix)
	// Ensure prefix ends with / for directory-like listing
	if fullPrefix != "" && !strings.HasSuffix(fullPrefix, "/") {
		fullPrefix += "/"
	}

	azureOpts := &AzureListOptions{
		Prefix:     fullPrefix,
		MaxResults: 5000,
	}
	if opts != nil && !opts.Recursive {
		azureOpts.Delimiter = "/"
	}

	var files []FileInfo
	marker := ""

	for {
		azureOpts.Marker = marker

		result, err := client.ListBlobs(ctx, b.config.Container, azureOpts)
		if err != nil {
			return nil, &BackendError{Op: "list", Path: prefix, Err: err}
		}

		for _, blob := range result.Blobs {
			// Remove the base prefix to get the relative path
			relPath := "/" + strings.TrimPrefix(blob.Name, b.config.Prefix)
			if b.config.Prefix != "" {
				relPath = "/" + strings.TrimPrefix(blob.Name, b.config.Prefix+"/")
			}

			checksum := ""
			if blob.Metadata != nil {
				if sha, ok := blob.Metadata["sha256"]; ok {
					checksum = sha
				}
			}

			files = append(files, FileInfo{
				Name:         path.Base(blob.Name),
				Path:         relPath,
				Size:         blob.Size,
				Checksum:     checksum,
				ModifiedTime: blob.LastModified,
				ETag:         blob.ETag,
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

		if result.NextMarker == "" {
			break
		}
		marker = result.NextMarker
	}

	return &ListResult{
		Files:    files,
		Truncated: false,
	}, nil
}

// Health checks Azure Blob Storage connectivity.
func (b *AzureBackend) Health(ctx context.Context) error {
	b.mu.RLock()
	client := b.client
	b.mu.RUnlock()

	if client == nil {
		return fmt.Errorf("azure client not configured")
	}

	if err := client.ContainerExists(ctx, b.config.Container); err != nil {
		return fmt.Errorf("azure health check failed: %w", err)
	}

	return nil
}

// Close closes the Azure backend.
func (b *AzureBackend) Close() error {
	return nil
}

// fullBlob returns the full Azure blob name including the prefix.
func (b *AzureBackend) fullBlob(filePath string) string {
	// Remove leading slash
	blob := strings.TrimPrefix(filePath, "/")

	if b.config.Prefix != "" {
		blob = b.config.Prefix + "/" + blob
	}

	return blob
}

// isAzureNotFound checks if an error indicates the blob was not found.
func isAzureNotFound(err error) bool {
	if err == nil {
		return false
	}
	errStr := err.Error()
	return strings.Contains(errStr, "BlobNotFound") ||
		strings.Contains(errStr, "blob does not exist") ||
		strings.Contains(errStr, "404")
}

// isAzureNotModified checks if an error indicates 304 Not Modified.
func isAzureNotModified(err error) bool {
	if err == nil {
		return false
	}
	errStr := err.Error()
	return strings.Contains(errStr, "ConditionNotMet") ||
		strings.Contains(errStr, "304")
}
