package files

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/nats-io/nats.go"
)

// Client handles file retrieval from the file server.
type Client struct {
	config  *ClientConfig
	nc      *nats.Conn
	cache   *FileCache
	metrics *ClientMetrics
	mu      sync.RWMutex
}

// ClientConfig configures the file client.
type ClientConfig struct {
	// ClusterID identifies the cluster for NATS subjects.
	ClusterID string `yaml:"cluster_id"`

	// AgentID identifies this agent.
	AgentID string `yaml:"agent_id"`

	// DefaultNamespace is the default namespace for file requests.
	DefaultNamespace string `yaml:"default_namespace"`

	// RequestTimeout is the timeout for file requests.
	RequestTimeout time.Duration `yaml:"request_timeout"`

	// ChunkTimeout is the timeout for receiving individual chunks.
	ChunkTimeout time.Duration `yaml:"chunk_timeout"`

	// MaxRetries is the maximum number of retry attempts.
	MaxRetries int `yaml:"max_retries"`

	// RetryDelay is the base delay between retries.
	RetryDelay time.Duration `yaml:"retry_delay"`

	// CacheDir is the directory for caching files.
	CacheDir string `yaml:"cache_dir"`

	// CacheSize is the maximum cache size in bytes.
	CacheSize int64 `yaml:"cache_size"`

	// CacheTTL is the time-to-live for cached files.
	CacheTTL time.Duration `yaml:"cache_ttl"`
}

// ClientMetrics tracks client metrics.
type ClientMetrics struct {
	RequestsTotal     int64
	RequestsSucceeded int64
	RequestsFailed    int64
	BytesReceived     int64
	CacheHits         int64
	CacheMisses       int64
	mu                sync.RWMutex
}

// NewClient creates a new file client.
func NewClient(config *ClientConfig) (*Client, error) {
	if config.ClusterID == "" {
		return nil, fmt.Errorf("cluster_id is required")
	}
	if config.AgentID == "" {
		return nil, fmt.Errorf("agent_id is required")
	}
	if config.DefaultNamespace == "" {
		config.DefaultNamespace = "default"
	}
	if config.RequestTimeout <= 0 {
		config.RequestTimeout = 5 * time.Minute
	}
	if config.ChunkTimeout <= 0 {
		config.ChunkTimeout = 30 * time.Second
	}
	if config.MaxRetries <= 0 {
		config.MaxRetries = DefaultRetryAttempts
	}
	if config.RetryDelay <= 0 {
		config.RetryDelay = DefaultRetryDelay
	}

	client := &Client{
		config:  config,
		metrics: &ClientMetrics{},
	}

	// Initialize cache if configured
	if config.CacheDir != "" {
		cacheConfig := &CacheConfig{
			Dir:     config.CacheDir,
			MaxSize: config.CacheSize,
			TTL:     config.CacheTTL,
		}
		if cacheConfig.MaxSize <= 0 {
			cacheConfig.MaxSize = DefaultCacheSize
		}
		if cacheConfig.TTL <= 0 {
			cacheConfig.TTL = DefaultCacheTTL
		}
		cache, err := NewFileCache(cacheConfig)
		if err != nil {
			return nil, fmt.Errorf("failed to initialize cache: %w", err)
		}
		client.cache = cache
	}

	return client, nil
}

// Connect connects the client to NATS.
func (c *Client) Connect(nc *nats.Conn) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.nc = nc
	return nil
}

// Close closes the client.
func (c *Client) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.cache != nil {
		return c.cache.Close()
	}
	return nil
}

// GetFileOptions specifies options for file retrieval.
type GetFileOptions struct {
	// Version to retrieve (empty for latest).
	Version string

	// Checksum for conditional GET (skip if matches).
	Checksum string

	// DestPath is the local destination path.
	DestPath string

	// UseCache enables cache lookup before requesting.
	UseCache bool

	// Priority for the request.
	Priority int

	// ChunkSize overrides the default chunk size.
	ChunkSize int

	// ProgressFunc is called with progress updates.
	ProgressFunc func(progress TransferProgress)
}

// GetFile retrieves a file from the file server.
func (c *Client) GetFile(ctx context.Context, path string, opts *GetFileOptions) (*GetFileResult, error) {
	c.mu.RLock()
	nc := c.nc
	c.mu.RUnlock()

	if nc == nil {
		return nil, fmt.Errorf("client not connected")
	}

	if opts == nil {
		opts = &GetFileOptions{}
	}

	c.metrics.mu.Lock()
	c.metrics.RequestsTotal++
	c.metrics.mu.Unlock()

	// Check cache first
	if opts.UseCache && c.cache != nil {
		if entry, err := c.cache.Get(path, opts.Version); err == nil {
			c.metrics.mu.Lock()
			c.metrics.CacheHits++
			c.metrics.RequestsSucceeded++
			c.metrics.mu.Unlock()

			return &GetFileResult{
				Path:       path,
				Version:    entry.Version,
				Checksum:   entry.Checksum,
				Size:       entry.Size,
				LocalPath:  entry.LocalPath,
				FromCache:  true,
				Downloaded: false,
			}, nil
		}
		c.metrics.mu.Lock()
		c.metrics.CacheMisses++
		c.metrics.mu.Unlock()
	}

	// Build request
	req := FileRequest{
		RequestID: uuid.New().String(),
		Path:      path,
		Version:   opts.Version,
		Checksum:  opts.Checksum,
		ChunkSize: opts.ChunkSize,
		Priority:  opts.Priority,
		AgentID:   c.config.AgentID,
	}

	// Determine namespace from path
	namespace := NamespaceFromPath(path)
	if namespace == "" {
		namespace = c.config.DefaultNamespace
	}

	// Send request and receive file
	result, err := c.requestFile(ctx, namespace, &req, opts)
	if err != nil {
		c.metrics.mu.Lock()
		c.metrics.RequestsFailed++
		c.metrics.mu.Unlock()
		return nil, err
	}

	c.metrics.mu.Lock()
	c.metrics.RequestsSucceeded++
	c.metrics.BytesReceived += result.Size
	c.metrics.mu.Unlock()

	// Cache the result
	if c.cache != nil && result.Downloaded {
		c.cache.Put(path, result.Version, result.Checksum, result.LocalPath, result.Size)
	}

	return result, nil
}

// GetFileResult contains the result of a file retrieval.
type GetFileResult struct {
	Path       string
	Version    string
	Checksum   string
	Size       int64
	LocalPath  string
	FromCache  bool
	Downloaded bool
}

// requestFile sends a file request and receives the response.
func (c *Client) requestFile(ctx context.Context, namespace string, req *FileRequest, opts *GetFileOptions) (*GetFileResult, error) {
	subject := fmt.Sprintf(SubjectFileRequest, c.config.ClusterID, namespace)

	reqData, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	// Create inbox for response
	inbox := c.nc.NewRespInbox()
	sub, err := c.nc.SubscribeSync(inbox)
	if err != nil {
		return nil, fmt.Errorf("failed to subscribe to inbox: %w", err)
	}
	defer sub.Unsubscribe()

	// Send request
	if err := c.nc.PublishRequest(subject, inbox, reqData); err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}

	// Receive metadata response
	metaCtx, cancel := context.WithTimeout(ctx, c.config.RequestTimeout)
	defer cancel()

	msg, err := c.receiveWithContext(sub, metaCtx)
	if err != nil {
		return nil, fmt.Errorf("failed to receive metadata: %w", err)
	}

	// Check for error response
	var fileErr FileError
	if json.Unmarshal(msg.Data, &fileErr) == nil && fileErr.Code != "" {
		return nil, &fileErr
	}

	// Parse metadata
	var metadata FileMetadata
	if err := json.Unmarshal(msg.Data, &metadata); err != nil {
		return nil, fmt.Errorf("failed to parse metadata: %w", err)
	}

	// Handle conditional GET (not modified)
	if metadata.NotModified {
		return &GetFileResult{
			Path:       req.Path,
			Version:    metadata.Version,
			Checksum:   metadata.Checksum,
			Downloaded: false,
		}, nil
	}

	// Prepare destination
	destPath := opts.DestPath
	if destPath == "" {
		// Create temp file
		tmpFile, err := os.CreateTemp("", "kscore-file-*")
		if err != nil {
			return nil, fmt.Errorf("failed to create temp file: %w", err)
		}
		destPath = tmpFile.Name()
		tmpFile.Close()
	}

	// Receive chunks
	if err := c.receiveChunks(ctx, sub, &metadata, destPath, opts.ProgressFunc); err != nil {
		os.Remove(destPath)
		return nil, err
	}

	// Verify checksum
	if metadata.Checksum != "" {
		actualChecksum, err := c.calculateChecksum(destPath)
		if err != nil {
			os.Remove(destPath)
			return nil, fmt.Errorf("failed to calculate checksum: %w", err)
		}
		if actualChecksum != metadata.Checksum {
			os.Remove(destPath)
			return nil, &FileError{
				RequestID: req.RequestID,
				Code:      ErrCodeChecksumFailed,
				Message:   fmt.Sprintf("checksum mismatch: expected %s, got %s", metadata.Checksum, actualChecksum),
			}
		}
	}

	return &GetFileResult{
		Path:       req.Path,
		Version:    metadata.Version,
		Checksum:   metadata.Checksum,
		Size:       metadata.Size,
		LocalPath:  destPath,
		FromCache:  false,
		Downloaded: true,
	}, nil
}

// receiveChunks receives and assembles file chunks.
func (c *Client) receiveChunks(ctx context.Context, sub *nats.Subscription, metadata *FileMetadata, destPath string, progressFunc func(TransferProgress)) error {
	file, err := os.Create(destPath)
	if err != nil {
		return fmt.Errorf("failed to create destination file: %w", err)
	}
	defer file.Close()

	var bytesReceived int64
	receivedChunks := make(map[int]bool)

	for i := 0; i < metadata.ChunkCount; i++ {
		chunkCtx, cancel := context.WithTimeout(ctx, c.config.ChunkTimeout)

		msg, err := c.receiveWithContext(sub, chunkCtx)
		cancel()

		if err != nil {
			return fmt.Errorf("failed to receive chunk %d: %w", i, err)
		}

		var chunk FileChunk
		if err := json.Unmarshal(msg.Data, &chunk); err != nil {
			return fmt.Errorf("failed to parse chunk %d: %w", i, err)
		}

		// Verify chunk index
		if chunk.Index != i {
			return fmt.Errorf("unexpected chunk index: expected %d, got %d", i, chunk.Index)
		}

		// Verify chunk checksum
		hash := sha256.Sum256(chunk.Data)
		actualChecksum := hex.EncodeToString(hash[:])
		if actualChecksum != chunk.Checksum {
			return fmt.Errorf("chunk %d checksum mismatch", i)
		}

		// Write chunk
		if _, err := file.Write(chunk.Data); err != nil {
			return fmt.Errorf("failed to write chunk %d: %w", i, err)
		}

		bytesReceived += int64(len(chunk.Data))
		receivedChunks[i] = true

		// Report progress
		if progressFunc != nil {
			progressFunc(TransferProgress{
				RequestID:        metadata.RequestID,
				Path:             metadata.Path,
				BytesTransferred: bytesReceived,
				TotalBytes:       metadata.Size,
				ChunksCompleted:  i + 1,
				TotalChunks:      metadata.ChunkCount,
				PercentComplete:  float64(bytesReceived) / float64(metadata.Size) * 100,
			})
		}
	}

	return nil
}

// receiveWithContext receives a message with context cancellation.
func (c *Client) receiveWithContext(sub *nats.Subscription, ctx context.Context) (*nats.Msg, error) {
	deadline, ok := ctx.Deadline()
	if !ok {
		// No deadline set, use a long timeout
		deadline = time.Now().Add(c.config.RequestTimeout)
	}
	timeout := time.Until(deadline)
	if timeout <= 0 {
		return nil, context.DeadlineExceeded
	}

	msg, err := sub.NextMsg(timeout)
	if err != nil {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
			return nil, err
		}
	}

	return msg, nil
}

// calculateChecksum calculates SHA-256 checksum of a file.
func (c *Client) calculateChecksum(path string) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer file.Close()

	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return "", err
	}

	return hex.EncodeToString(hash.Sum(nil)), nil
}

// GetMetadata retrieves file metadata without downloading the file.
func (c *Client) GetMetadata(ctx context.Context, path string) (*FileMetadata, error) {
	c.mu.RLock()
	nc := c.nc
	c.mu.RUnlock()

	if nc == nil {
		return nil, fmt.Errorf("client not connected")
	}

	namespace := NamespaceFromPath(path)
	if namespace == "" {
		namespace = c.config.DefaultNamespace
	}

	req := FileRequest{
		RequestID: uuid.New().String(),
		Path:      path,
		AgentID:   c.config.AgentID,
	}

	reqData, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	subject := fmt.Sprintf(SubjectFileMetadata, c.config.ClusterID, namespace)
	msg, err := nc.RequestWithContext(ctx, subject, reqData)
	if err != nil {
		return nil, fmt.Errorf("metadata request failed: %w", err)
	}

	// Check for error response
	var fileErr FileError
	if json.Unmarshal(msg.Data, &fileErr) == nil && fileErr.Code != "" {
		return nil, &fileErr
	}

	var metadata FileMetadata
	if err := json.Unmarshal(msg.Data, &metadata); err != nil {
		return nil, fmt.Errorf("failed to parse metadata: %w", err)
	}

	return &metadata, nil
}

// Metrics returns current client metrics.
func (c *Client) Metrics() ClientMetrics {
	c.metrics.mu.RLock()
	defer c.metrics.mu.RUnlock()

	return ClientMetrics{
		RequestsTotal:     c.metrics.RequestsTotal,
		RequestsSucceeded: c.metrics.RequestsSucceeded,
		RequestsFailed:    c.metrics.RequestsFailed,
		BytesReceived:     c.metrics.BytesReceived,
		CacheHits:         c.metrics.CacheHits,
		CacheMisses:       c.metrics.CacheMisses,
	}
}

// FileCache handles local file caching with LRU eviction.
type FileCache struct {
	config    *CacheConfig
	entries   map[string]*CacheEntry
	totalSize int64
	mu        sync.RWMutex
}

// CacheConfig configures the file cache.
type CacheConfig struct {
	Dir     string
	MaxSize int64
	TTL     time.Duration
}

// CacheEntry represents a cached file.
type CacheEntry struct {
	Path         string
	Version      string
	Checksum     string
	LocalPath    string
	Size         int64
	CachedAt     time.Time
	LastAccessed time.Time
}

// NewFileCache creates a new file cache.
func NewFileCache(config *CacheConfig) (*FileCache, error) {
	if config.Dir == "" {
		return nil, fmt.Errorf("cache directory is required")
	}

	if err := os.MkdirAll(config.Dir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create cache directory: %w", err)
	}

	return &FileCache{
		config:  config,
		entries: make(map[string]*CacheEntry),
	}, nil
}

// Get retrieves a file from the cache.
func (fc *FileCache) Get(path, version string) (*CacheEntry, error) {
	fc.mu.Lock()
	defer fc.mu.Unlock()

	key := fc.cacheKey(path, version)
	entry, ok := fc.entries[key]
	if !ok {
		return nil, fmt.Errorf("not in cache")
	}

	// Check TTL
	if time.Since(entry.CachedAt) > fc.config.TTL {
		// Remove expired entry
		fc.removeEntryLocked(key, entry)
		return nil, fmt.Errorf("cache entry expired")
	}

	// Verify file exists
	if _, err := os.Stat(entry.LocalPath); os.IsNotExist(err) {
		// Remove missing entry
		fc.removeEntryLocked(key, entry)
		return nil, fmt.Errorf("cached file missing")
	}

	// Update last accessed time for LRU
	entry.LastAccessed = time.Now()

	return entry, nil
}

// Put adds a file to the cache.
func (fc *FileCache) Put(path, version, checksum, localPath string, size int64) error {
	fc.mu.Lock()
	defer fc.mu.Unlock()

	key := fc.cacheKey(path, version)

	// Check if entry already exists - if so, just update access time
	if existing, ok := fc.entries[key]; ok {
		existing.LastAccessed = time.Now()
		return nil
	}

	// Evict old entries if adding this would exceed size limit
	if fc.config.MaxSize > 0 {
		for fc.totalSize+size > fc.config.MaxSize && len(fc.entries) > 0 {
			fc.evictLRUEntryLocked()
		}
	}

	// Copy to cache directory
	cacheFile := filepath.Join(fc.config.Dir, fmt.Sprintf("%s-%s", filepath.Base(path), checksum[:8]))
	if localPath != cacheFile {
		src, err := os.Open(localPath)
		if err != nil {
			return err
		}
		defer src.Close()

		dst, err := os.Create(cacheFile)
		if err != nil {
			return err
		}
		defer dst.Close()

		if _, err := io.Copy(dst, src); err != nil {
			os.Remove(cacheFile)
			return err
		}
	}

	now := time.Now()
	fc.entries[key] = &CacheEntry{
		Path:         path,
		Version:      version,
		Checksum:     checksum,
		LocalPath:    cacheFile,
		Size:         size,
		CachedAt:     now,
		LastAccessed: now,
	}
	fc.totalSize += size

	return nil
}

// removeEntryLocked removes a cache entry (must be called with lock held).
func (fc *FileCache) removeEntryLocked(key string, entry *CacheEntry) {
	// Remove from entries map
	delete(fc.entries, key)
	fc.totalSize -= entry.Size

	// Remove the cached file
	os.Remove(entry.LocalPath)
}

// evictLRUEntryLocked evicts the least recently used entry (must be called with lock held).
func (fc *FileCache) evictLRUEntryLocked() {
	if len(fc.entries) == 0 {
		return
	}

	var oldestKey string
	var oldestEntry *CacheEntry
	var oldestTime time.Time

	for key, entry := range fc.entries {
		if oldestEntry == nil || entry.LastAccessed.Before(oldestTime) {
			oldestKey = key
			oldestEntry = entry
			oldestTime = entry.LastAccessed
		}
	}

	if oldestEntry != nil {
		fc.removeEntryLocked(oldestKey, oldestEntry)
	}
}

// cacheKey generates a cache key from path and version.
func (fc *FileCache) cacheKey(path, version string) string {
	if version == "" {
		version = "latest"
	}
	return fmt.Sprintf("%s@%s", path, version)
}

// Size returns the current total size of cached files in bytes.
func (fc *FileCache) Size() int64 {
	fc.mu.RLock()
	defer fc.mu.RUnlock()
	return fc.totalSize
}

// Count returns the number of entries in the cache.
func (fc *FileCache) Count() int {
	fc.mu.RLock()
	defer fc.mu.RUnlock()
	return len(fc.entries)
}

// Clear removes all entries from the cache.
func (fc *FileCache) Clear() {
	fc.mu.Lock()
	defer fc.mu.Unlock()

	// Remove all cached files
	for _, entry := range fc.entries {
		os.Remove(entry.LocalPath)
	}

	// Reset state
	fc.entries = make(map[string]*CacheEntry)
	fc.totalSize = 0
}

// Close closes the cache.
func (fc *FileCache) Close() error {
	return nil
}

// ListFilesOptions specifies options for listing files.
type ListFilesOptions struct {
	Namespace string
	Recursive bool
	Limit     int
	Offset    int
}

// ListFilesResult contains the result of a file listing.
type ListFilesResult struct {
	Path       string       `json:"path"`
	Entries    []*FileEntry `json:"entries"`
	TotalCount int          `json:"total_count"`
	Truncated  bool         `json:"truncated"`
}

// FileEntry represents a file or directory in a listing.
type FileEntry struct {
	Path     string    `json:"path"`
	Name     string    `json:"name"`
	Size     int64     `json:"size"`
	Checksum string    `json:"checksum"`
	ModTime  time.Time `json:"mod_time"`
	IsDir    bool      `json:"is_dir"`
	Version  string    `json:"version,omitempty"`
}

// ListFiles lists files at the given path.
func (c *Client) ListFiles(ctx context.Context, path string, opts *ListFilesOptions) (*ListFilesResult, error) {
	c.mu.RLock()
	nc := c.nc
	c.mu.RUnlock()

	if nc == nil {
		return nil, fmt.Errorf("client not connected")
	}

	if opts == nil {
		opts = &ListFilesOptions{}
	}

	namespace := opts.Namespace
	if namespace == "" {
		namespace = NamespaceFromPath(path)
		if namespace == "" {
			namespace = c.config.DefaultNamespace
		}
	}

	req := FileListRequest{
		RequestID: uuid.New().String(),
		Path:      path,
		Recursive: opts.Recursive,
		Limit:     opts.Limit,
		Offset:    opts.Offset,
		AgentID:   c.config.AgentID,
	}

	reqData, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	subject := fmt.Sprintf("kscore.%s.files.list.%s", c.config.ClusterID, namespace)
	msg, err := nc.RequestWithContext(ctx, subject, reqData)
	if err != nil {
		return nil, fmt.Errorf("list request failed: %w", err)
	}

	// Check for error response
	var fileErr FileError
	if json.Unmarshal(msg.Data, &fileErr) == nil && fileErr.Code != "" {
		return nil, &fileErr
	}

	var resp FileListResponse
	if err := json.Unmarshal(msg.Data, &resp); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	// Convert FileInfo to FileEntry
	entries := make([]*FileEntry, len(resp.Files))
	for i, fi := range resp.Files {
		entries[i] = &FileEntry{
			Path:     fi.Path,
			Name:     fi.Name,
			Size:     fi.Size,
			Checksum: fi.Checksum,
			ModTime:  fi.ModifiedTime,
			IsDir:    fi.IsDirectory,
			Version:  fi.Version,
		}
	}

	return &ListFilesResult{
		Path:       resp.Path,
		Entries:    entries,
		TotalCount: resp.TotalCount,
		Truncated:  resp.Truncated,
	}, nil
}

// GetFileInfoOptions specifies options for getting file info.
type GetFileInfoOptions struct {
	Namespace string
	Version   string
}

// GetFileInfo retrieves detailed information about a file.
func (c *Client) GetFileInfo(ctx context.Context, path string, opts *GetFileInfoOptions) (*FileInfo, error) {
	c.mu.RLock()
	nc := c.nc
	c.mu.RUnlock()

	if nc == nil {
		return nil, fmt.Errorf("client not connected")
	}

	if opts == nil {
		opts = &GetFileInfoOptions{}
	}

	namespace := opts.Namespace
	if namespace == "" {
		namespace = NamespaceFromPath(path)
		if namespace == "" {
			namespace = c.config.DefaultNamespace
		}
	}

	req := FileRequest{
		RequestID: uuid.New().String(),
		Path:      path,
		Version:   opts.Version,
		AgentID:   c.config.AgentID,
	}

	reqData, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	subject := fmt.Sprintf(SubjectFileMetadata, c.config.ClusterID, namespace)
	msg, err := nc.RequestWithContext(ctx, subject, reqData)
	if err != nil {
		return nil, fmt.Errorf("info request failed: %w", err)
	}

	// Check for error response
	var fileErr FileError
	if json.Unmarshal(msg.Data, &fileErr) == nil && fileErr.Code != "" {
		return nil, &fileErr
	}

	var metadata FileMetadata
	if err := json.Unmarshal(msg.Data, &metadata); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	return &FileInfo{
		Path:         metadata.Path,
		Name:         filepath.Base(metadata.Path),
		Size:         metadata.Size,
		Checksum:     metadata.Checksum,
		ContentType:  metadata.ContentType,
		ModifiedTime: metadata.ModifiedTime,
		Version:      metadata.Version,
		Tags:         metadata.Tags,
	}, nil
}

// PutFileOptions specifies options for uploading a file.
type PutFileOptions struct {
	Namespace   string
	ContentType string
	Checksum    string
	Tags        map[string]string
	Overwrite   bool
}

// PutFile uploads a file to the file server.
func (c *Client) PutFile(ctx context.Context, path string, reader io.Reader, size int64, opts *PutFileOptions) error {
	c.mu.RLock()
	nc := c.nc
	c.mu.RUnlock()

	if nc == nil {
		return fmt.Errorf("client not connected")
	}

	if opts == nil {
		opts = &PutFileOptions{}
	}

	namespace := opts.Namespace
	if namespace == "" {
		namespace = NamespaceFromPath(path)
		if namespace == "" {
			namespace = c.config.DefaultNamespace
		}
	}

	// Send upload request
	uploadReq := FileUploadRequest{
		RequestID:   uuid.New().String(),
		Path:        path,
		Size:        size,
		Checksum:    opts.Checksum,
		ContentType: opts.ContentType,
		AgentID:     c.config.AgentID,
		Tags:        opts.Tags,
		Overwrite:   opts.Overwrite,
	}

	reqData, err := json.Marshal(uploadReq)
	if err != nil {
		return fmt.Errorf("failed to marshal request: %w", err)
	}

	subject := fmt.Sprintf(SubjectFileUpload, c.config.ClusterID, namespace)
	msg, err := nc.RequestWithContext(ctx, subject, reqData)
	if err != nil {
		return fmt.Errorf("upload request failed: %w", err)
	}

	var resp FileUploadResponse
	if err := json.Unmarshal(msg.Data, &resp); err != nil {
		return fmt.Errorf("failed to parse response: %w", err)
	}

	if !resp.Accepted {
		return fmt.Errorf("upload rejected: %s", resp.Error)
	}

	// Send file chunks
	chunkSize := resp.ChunkSize
	if chunkSize <= 0 {
		chunkSize = DefaultChunkSize
	}

	buf := make([]byte, chunkSize)
	chunkIndex := 0
	uploadSubject := fmt.Sprintf("kscore.%s.files.upload.%s.%s", c.config.ClusterID, namespace, resp.UploadID)

	for {
		n, err := reader.Read(buf)
		if err != nil && err != io.EOF {
			return fmt.Errorf("failed to read data: %w", err)
		}

		if n == 0 {
			break
		}

		// Calculate chunk checksum
		hash := sha256.Sum256(buf[:n])
		chunkChecksum := hex.EncodeToString(hash[:])

		chunk := FileChunk{
			RequestID:  resp.UploadID,
			Index:      chunkIndex,
			TotalCount: resp.ChunkCount,
			Data:       buf[:n],
			Checksum:   chunkChecksum,
			Final:      err == io.EOF || chunkIndex == resp.ChunkCount-1,
		}

		chunkData, err := json.Marshal(chunk)
		if err != nil {
			return fmt.Errorf("failed to marshal chunk: %w", err)
		}

		// Send chunk
		if err := nc.Publish(uploadSubject, chunkData); err != nil {
			return fmt.Errorf("failed to send chunk %d: %w", chunkIndex, err)
		}

		chunkIndex++

		if chunk.Final {
			break
		}
	}

	return nil
}

// DeleteFileOptions specifies options for deleting a file.
type DeleteFileOptions struct {
	Namespace string
	Version   string
	Recursive bool
}

// DeleteFile deletes a file from the file server.
func (c *Client) DeleteFile(ctx context.Context, path string, opts *DeleteFileOptions) error {
	c.mu.RLock()
	nc := c.nc
	c.mu.RUnlock()

	if nc == nil {
		return fmt.Errorf("client not connected")
	}

	if opts == nil {
		opts = &DeleteFileOptions{}
	}

	namespace := opts.Namespace
	if namespace == "" {
		namespace = NamespaceFromPath(path)
		if namespace == "" {
			namespace = c.config.DefaultNamespace
		}
	}

	req := FileDeleteRequest{
		RequestID: uuid.New().String(),
		Path:      path,
		Version:   opts.Version,
		AgentID:   c.config.AgentID,
	}

	reqData, err := json.Marshal(req)
	if err != nil {
		return fmt.Errorf("failed to marshal request: %w", err)
	}

	subject := fmt.Sprintf("kscore.%s.files.delete.%s", c.config.ClusterID, namespace)
	msg, err := nc.RequestWithContext(ctx, subject, reqData)
	if err != nil {
		return fmt.Errorf("delete request failed: %w", err)
	}

	var resp FileDeleteResponse
	if err := json.Unmarshal(msg.Data, &resp); err != nil {
		return fmt.Errorf("failed to parse response: %w", err)
	}

	if !resp.Deleted {
		return fmt.Errorf("delete failed: %s", resp.Error)
	}

	return nil
}

// BackendSyncOptions specifies options for syncing backends.
type BackendSyncOptions struct {
	DryRun bool
	Force  bool
}

// BackendSyncResult contains the result of a backend sync operation.
type BackendSyncResult struct {
	FilesCopied    int64         `json:"files_copied"`
	BytesCopied    int64         `json:"bytes_copied"`
	FilesDeleted   int64         `json:"files_deleted"`
	FilesUnchanged int64         `json:"files_unchanged"`
	Errors         int64         `json:"errors"`
	Duration       time.Duration `json:"duration"`
}
