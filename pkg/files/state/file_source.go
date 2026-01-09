// Package state provides integration between the file distribution system and state management.
package state

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/shawnbutts/keystone-core/pkg/files"
)

// FileSource represents a source for file content.
type FileSource interface {
	// Get retrieves the file content.
	Get(ctx context.Context) (io.ReadCloser, error)

	// GetChecksum returns the expected checksum if known.
	GetChecksum() string

	// GetVersion returns the version if known.
	GetVersion() string
}

// KSCoreFileSource handles kscore:// URLs.
type KSCoreFileSource struct {
	// url is the original URL.
	url string

	// namespace is the file namespace.
	namespace string

	// path is the file path within the namespace.
	path string

	// version is the pinned version (empty for latest).
	version string

	// checksum is the expected checksum (empty to skip verification).
	checksum string

	// client is the file distribution client.
	client *files.Client
}

// ParseKSCoreURL parses a kscore:// URL.
// Format: kscore://namespace/path?version=v1&checksum=sha256:abc123
func ParseKSCoreURL(rawURL string) (*KSCoreFileSource, error) {
	if !strings.HasPrefix(rawURL, "kscore://") {
		return nil, fmt.Errorf("not a kscore URL: %s", rawURL)
	}

	parsed, err := url.Parse(rawURL)
	if err != nil {
		return nil, fmt.Errorf("invalid URL: %w", err)
	}

	if parsed.Host == "" {
		return nil, fmt.Errorf("missing namespace in URL: %s", rawURL)
	}

	source := &KSCoreFileSource{
		url:       rawURL,
		namespace: parsed.Host,
		path:      parsed.Path,
	}

	// Parse query parameters.
	query := parsed.Query()
	source.version = query.Get("version")
	source.checksum = query.Get("checksum")

	return source, nil
}

// SetClient sets the file distribution client.
func (s *KSCoreFileSource) SetClient(client *files.Client) {
	s.client = client
}

// Get retrieves the file content.
func (s *KSCoreFileSource) Get(ctx context.Context) (io.ReadCloser, error) {
	if s.client == nil {
		return nil, fmt.Errorf("file client not configured")
	}

	// Build the full path including namespace.
	fullPath := "/" + s.namespace + s.path

	// Build options.
	opts := &files.GetFileOptions{
		Version:  s.version,
		Checksum: s.checksum,
		UseCache: true,
	}

	// Get the file.
	result, err := s.client.GetFile(ctx, fullPath, opts)
	if err != nil {
		return nil, err
	}

	// Open the local file and return it as a reader.
	file, err := os.Open(result.LocalPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open downloaded file: %w", err)
	}

	return file, nil
}

// GetChecksum returns the expected checksum.
func (s *KSCoreFileSource) GetChecksum() string {
	return s.checksum
}

// GetVersion returns the pinned version.
func (s *KSCoreFileSource) GetVersion() string {
	return s.version
}

// GetNamespace returns the namespace.
func (s *KSCoreFileSource) GetNamespace() string {
	return s.namespace
}

// GetPath returns the path.
func (s *KSCoreFileSource) GetPath() string {
	return s.path
}

// LocalFileCache provides local caching for downloaded files.
type LocalFileCache struct {
	// cacheDir is the directory for cached files.
	cacheDir string

	// entries tracks cached files.
	entries map[string]*CacheEntry

	// mu protects entries.
	mu sync.RWMutex
}

// CacheEntry represents a cached file.
type CacheEntry struct {
	// Path is the local file path.
	Path string

	// Checksum is the file checksum.
	Checksum string

	// Version is the file version.
	Version string

	// CachedAt is when the file was cached.
	CachedAt time.Time

	// Size is the file size.
	Size int64
}

// NewLocalFileCache creates a new local file cache.
func NewLocalFileCache(cacheDir string) (*LocalFileCache, error) {
	if err := os.MkdirAll(cacheDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create cache directory: %w", err)
	}

	return &LocalFileCache{
		cacheDir: cacheDir,
		entries:  make(map[string]*CacheEntry),
	}, nil
}

// Get retrieves a file from the cache.
func (c *LocalFileCache) Get(key string) (*CacheEntry, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	entry, ok := c.entries[key]
	if !ok {
		return nil, false
	}

	// Check if file still exists.
	if _, err := os.Stat(entry.Path); os.IsNotExist(err) {
		return nil, false
	}

	return entry, true
}

// Put stores a file in the cache.
func (c *LocalFileCache) Put(key string, reader io.Reader, checksum, version string) (*CacheEntry, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	// Generate cache file path.
	filename := checksumToFilename(key)
	filePath := filepath.Join(c.cacheDir, filename)

	// Create the file.
	file, err := os.Create(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to create cache file: %w", err)
	}
	defer file.Close()

	// Write content and compute checksum.
	hash := sha256.New()
	writer := io.MultiWriter(file, hash)

	size, err := io.Copy(writer, reader)
	if err != nil {
		os.Remove(filePath)
		return nil, fmt.Errorf("failed to write cache file: %w", err)
	}

	computedChecksum := "sha256:" + hex.EncodeToString(hash.Sum(nil))

	// Verify checksum if provided.
	if checksum != "" && checksum != computedChecksum {
		os.Remove(filePath)
		return nil, fmt.Errorf("checksum mismatch: expected %s, got %s", checksum, computedChecksum)
	}

	entry := &CacheEntry{
		Path:     filePath,
		Checksum: computedChecksum,
		Version:  version,
		CachedAt: time.Now(),
		Size:     size,
	}

	c.entries[key] = entry
	return entry, nil
}

// Remove removes a file from the cache.
func (c *LocalFileCache) Remove(key string) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	entry, ok := c.entries[key]
	if !ok {
		return nil
	}

	// Remove the file.
	if err := os.Remove(entry.Path); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to remove cache file: %w", err)
	}

	delete(c.entries, key)
	return nil
}

// Clear removes all cached files.
func (c *LocalFileCache) Clear() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	for key, entry := range c.entries {
		os.Remove(entry.Path)
		delete(c.entries, key)
	}

	return nil
}

// checksumToFilename converts a key to a safe filename.
func checksumToFilename(key string) string {
	hash := sha256.Sum256([]byte(key))
	return hex.EncodeToString(hash[:])
}

// FileSourceResolver resolves file sources and manages caching.
type FileSourceResolver struct {
	// client is the file distribution client.
	client *files.Client

	// cache is the local file cache.
	cache *LocalFileCache

	// verifyChecksums enables checksum verification.
	verifyChecksums bool
}

// NewFileSourceResolver creates a new file source resolver.
func NewFileSourceResolver(client *files.Client, cache *LocalFileCache) *FileSourceResolver {
	return &FileSourceResolver{
		client:          client,
		cache:           cache,
		verifyChecksums: true,
	}
}

// SetVerifyChecksums enables or disables checksum verification.
func (r *FileSourceResolver) SetVerifyChecksums(verify bool) {
	r.verifyChecksums = verify
}

// Resolve resolves a source URL and returns the local file path.
func (r *FileSourceResolver) Resolve(ctx context.Context, sourceURL string) (string, error) {
	// Parse the URL.
	source, err := ParseKSCoreURL(sourceURL)
	if err != nil {
		// Not a kscore URL - return as-is (local file).
		return sourceURL, nil
	}

	// Set the client.
	source.SetClient(r.client)

	// Build cache key.
	cacheKey := sourceURL
	if source.GetVersion() != "" {
		cacheKey = sourceURL // Version is already in URL
	}

	// Check cache.
	if r.cache != nil {
		if entry, ok := r.cache.Get(cacheKey); ok {
			// Verify checksum if required.
			if r.verifyChecksums && source.GetChecksum() != "" {
				if entry.Checksum != source.GetChecksum() {
					// Checksum mismatch - remove cached file and re-download.
					r.cache.Remove(cacheKey)
				} else {
					return entry.Path, nil
				}
			} else {
				return entry.Path, nil
			}
		}
	}

	// Download the file.
	reader, err := source.Get(ctx)
	if err != nil {
		return "", fmt.Errorf("failed to download file: %w", err)
	}
	defer reader.Close()

	// Cache the file.
	if r.cache != nil {
		entry, err := r.cache.Put(cacheKey, reader, source.GetChecksum(), source.GetVersion())
		if err != nil {
			return "", err
		}
		return entry.Path, nil
	}

	// No cache - write to temp file.
	tmpFile, err := os.CreateTemp("", "kscore-file-*")
	if err != nil {
		return "", fmt.Errorf("failed to create temp file: %w", err)
	}
	defer tmpFile.Close()

	hash := sha256.New()
	writer := io.MultiWriter(tmpFile, hash)

	if _, err := io.Copy(writer, reader); err != nil {
		os.Remove(tmpFile.Name())
		return "", fmt.Errorf("failed to write temp file: %w", err)
	}

	// Verify checksum if required.
	if r.verifyChecksums && source.GetChecksum() != "" {
		computedChecksum := "sha256:" + hex.EncodeToString(hash.Sum(nil))
		if computedChecksum != source.GetChecksum() {
			os.Remove(tmpFile.Name())
			return "", fmt.Errorf("checksum mismatch: expected %s, got %s", source.GetChecksum(), computedChecksum)
		}
	}

	return tmpFile.Name(), nil
}

// VerifyChecksum verifies the checksum of a local file.
func VerifyChecksum(path string, expected string) error {
	if expected == "" {
		return nil
	}

	file, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("failed to open file: %w", err)
	}
	defer file.Close()

	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return fmt.Errorf("failed to read file: %w", err)
	}

	computed := "sha256:" + hex.EncodeToString(hash.Sum(nil))

	// Handle expected checksum with or without prefix.
	normalizedExpected := expected
	if !strings.HasPrefix(expected, "sha256:") {
		normalizedExpected = "sha256:" + expected
	}

	if computed != normalizedExpected {
		return fmt.Errorf("checksum mismatch: expected %s, got %s", normalizedExpected, computed)
	}

	return nil
}

// ComputeChecksum computes the SHA256 checksum of a file.
func ComputeChecksum(path string) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", fmt.Errorf("failed to open file: %w", err)
	}
	defer file.Close()

	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return "", fmt.Errorf("failed to read file: %w", err)
	}

	return "sha256:" + hex.EncodeToString(hash.Sum(nil)), nil
}
