// Package agent provides the agent-side secret client for Keystone Core agents.
// It handles secret retrieval from the broker, local caching, automatic refresh,
// and SPIFFE-based authentication.
package agent

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/shawnbutts/keystone-core/internal/secrets"
)

// ClientState represents the state of the secret client.
type ClientState string

const (
	ClientStateDisconnected ClientState = "disconnected"
	ClientStateConnecting   ClientState = "connecting"
	ClientStateConnected    ClientState = "connected"
	ClientStateClosed       ClientState = "closed"
)

// ClientConfig holds configuration for the agent secret client.
type ClientConfig struct {
	// BrokerAddress is the address of the secrets broker.
	BrokerAddress string

	// AgentID is the unique identifier for this agent.
	AgentID string

	// SpiffeID is the SPIFFE identity of the agent (e.g., spiffe://domain/agent/id).
	SpiffeID string

	// CacheConfig configures the local secret cache.
	CacheConfig *CacheConfig

	// RefreshConfig configures automatic secret refresh.
	RefreshConfig *RefreshConfig

	// BatchConfig configures request batching.
	BatchConfig *BatchConfig

	// ConnectTimeout is the timeout for connecting to the broker.
	ConnectTimeout time.Duration

	// RequestTimeout is the default timeout for secret requests.
	RequestTimeout time.Duration

	// TLSCertPath is the path to the TLS certificate for broker connection.
	TLSCertPath string

	// TLSKeyPath is the path to the TLS key for broker connection.
	TLSKeyPath string

	// TLSCAPath is the path to the TLS CA certificate.
	TLSCAPath string
}

// CacheConfig configures the local secret cache.
type CacheConfig struct {
	// Enabled indicates if caching is enabled.
	Enabled bool

	// MemoryEnabled enables in-memory cache.
	MemoryEnabled bool

	// DiskEnabled enables encrypted disk cache.
	DiskEnabled bool

	// DiskPath is the path to store encrypted secrets.
	DiskPath string

	// MaxMemoryEntries is the maximum number of entries in memory cache.
	MaxMemoryEntries int

	// DefaultTTL is the default TTL for cached secrets.
	DefaultTTL time.Duration

	// EncryptionKey is the key for encrypting disk cache (32 bytes for AES-256).
	EncryptionKey []byte
}

// RefreshConfig configures automatic secret refresh.
type RefreshConfig struct {
	// Enabled indicates if automatic refresh is enabled.
	Enabled bool

	// RefreshThreshold is the percentage of TTL at which to refresh (e.g., 0.75 = 75%).
	RefreshThreshold float64

	// CheckInterval is how often to check for secrets needing refresh.
	CheckInterval time.Duration

	// MaxConcurrentRefresh is the maximum number of concurrent refresh operations.
	MaxConcurrentRefresh int
}

// BatchConfig configures request batching.
type BatchConfig struct {
	// Enabled indicates if batching is enabled.
	Enabled bool

	// MaxBatchSize is the maximum number of requests per batch.
	MaxBatchSize int

	// BatchTimeout is the maximum time to wait for a batch to fill.
	BatchTimeout time.Duration
}

// Client is the agent-side secret client.
type Client struct {
	config *ClientConfig

	mu    sync.RWMutex
	state ClientState

	// Cache
	memoryCache *MemoryCache
	diskCache   *DiskCache

	// Refresh scheduler
	refreshScheduler *RefreshScheduler

	// Request batcher
	batcher *RequestBatcher

	// Broker connection
	broker SecretBrokerClient

	// Callbacks
	onStateChange func(oldState, newState ClientState)
	onSecretRefresh func(path string, secret *secrets.Secret)
	onError func(err error)

	// Background workers
	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup

	// Stats
	stats ClientStats
}

// ClientStats contains statistics about the client.
type ClientStats struct {
	RequestCount      int64
	CacheHits         int64
	CacheMisses       int64
	RefreshCount      int64
	RefreshErrors     int64
	BatchCount        int64
	ConnectAttempts   int64
	ConnectSuccesses  int64
	ConnectFailures   int64
	LastRequestTime   time.Time
	LastRefreshTime   time.Time
	LastConnectTime   time.Time
}

// SecretBrokerClient defines the interface for communicating with the broker.
type SecretBrokerClient interface {
	// Connect establishes a connection to the broker.
	Connect(ctx context.Context) error

	// Close closes the connection.
	Close() error

	// GetSecret retrieves a secret from the broker.
	GetSecret(ctx context.Context, req *SecretRequest) (*secrets.Secret, error)

	// GetSecretBatch retrieves multiple secrets in a single request.
	GetSecretBatch(ctx context.Context, reqs []*SecretRequest) ([]*secrets.Secret, error)

	// RenewLease renews a lease.
	RenewLease(ctx context.Context, leaseID string, increment time.Duration) (*secrets.Lease, error)

	// ListSecrets lists secrets under a path prefix.
	ListSecrets(ctx context.Context, prefix string) ([]string, error)

	// Healthy returns true if the connection is healthy.
	Healthy() bool
}

// SecretRequest represents a request for a secret.
type SecretRequest struct {
	// Path is the secret path.
	Path string

	// Version is the specific version to retrieve (0 = latest).
	Version int

	// Dynamic indicates if this is a dynamic secret request.
	Dynamic bool

	// Role is the role for dynamic secrets (database, AWS, etc.).
	Role string

	// TTL is the requested TTL for dynamic secrets.
	TTL time.Duration

	// Metadata contains request metadata.
	Metadata map[string]string
}

// NewClient creates a new agent secret client.
func NewClient(config *ClientConfig) (*Client, error) {
	if config == nil {
		return nil, fmt.Errorf("config is required")
	}

	if config.AgentID == "" {
		return nil, fmt.Errorf("agent ID is required")
	}

	// Apply defaults
	if config.ConnectTimeout == 0 {
		config.ConnectTimeout = 30 * time.Second
	}
	if config.RequestTimeout == 0 {
		config.RequestTimeout = 10 * time.Second
	}

	if config.CacheConfig == nil {
		config.CacheConfig = &CacheConfig{
			Enabled:          true,
			MemoryEnabled:    true,
			MaxMemoryEntries: 1000,
			DefaultTTL:       5 * time.Minute,
		}
	}

	if config.RefreshConfig == nil {
		config.RefreshConfig = &RefreshConfig{
			Enabled:              true,
			RefreshThreshold:     0.75,
			CheckInterval:        30 * time.Second,
			MaxConcurrentRefresh: 5,
		}
	}

	if config.BatchConfig == nil {
		config.BatchConfig = &BatchConfig{
			Enabled:      true,
			MaxBatchSize: 10,
			BatchTimeout: 50 * time.Millisecond,
		}
	}

	ctx, cancel := context.WithCancel(context.Background())

	c := &Client{
		config: config,
		state:  ClientStateDisconnected,
		ctx:    ctx,
		cancel: cancel,
	}

	// Initialize memory cache
	if config.CacheConfig.Enabled && config.CacheConfig.MemoryEnabled {
		c.memoryCache = NewMemoryCache(config.CacheConfig.MaxMemoryEntries, config.CacheConfig.DefaultTTL)
	}

	// Initialize disk cache
	if config.CacheConfig.Enabled && config.CacheConfig.DiskEnabled && config.CacheConfig.DiskPath != "" {
		var err error
		c.diskCache, err = NewDiskCache(config.CacheConfig.DiskPath, config.CacheConfig.EncryptionKey)
		if err != nil {
			cancel()
			return nil, fmt.Errorf("failed to initialize disk cache: %w", err)
		}
	}

	// Initialize refresh scheduler
	if config.RefreshConfig.Enabled {
		c.refreshScheduler = NewRefreshScheduler(c, config.RefreshConfig)
	}

	// Initialize batcher
	if config.BatchConfig.Enabled {
		c.batcher = NewRequestBatcher(c, config.BatchConfig)
	}

	return c, nil
}

// SetBrokerClient sets the broker client implementation.
func (c *Client) SetBrokerClient(broker SecretBrokerClient) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.broker = broker
}

// SetStateChangeCallback sets the callback for state changes.
func (c *Client) SetStateChangeCallback(cb func(oldState, newState ClientState)) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.onStateChange = cb
}

// SetSecretRefreshCallback sets the callback for secret refreshes.
func (c *Client) SetSecretRefreshCallback(cb func(path string, secret *secrets.Secret)) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.onSecretRefresh = cb
}

// SetErrorCallback sets the callback for errors.
func (c *Client) SetErrorCallback(cb func(err error)) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.onError = cb
}

// Connect establishes a connection to the secrets broker.
func (c *Client) Connect(ctx context.Context) error {
	c.mu.Lock()
	if c.state == ClientStateClosed {
		c.mu.Unlock()
		return fmt.Errorf("client is closed")
	}
	if c.state == ClientStateConnected {
		c.mu.Unlock()
		return nil
	}
	c.setState(ClientStateConnecting)
	c.stats.ConnectAttempts++
	c.mu.Unlock()

	if c.broker == nil {
		c.mu.Lock()
		c.setState(ClientStateDisconnected)
		c.stats.ConnectFailures++
		c.mu.Unlock()
		return fmt.Errorf("broker client not set")
	}

	connectCtx, cancel := context.WithTimeout(ctx, c.config.ConnectTimeout)
	defer cancel()

	if err := c.broker.Connect(connectCtx); err != nil {
		c.mu.Lock()
		c.setState(ClientStateDisconnected)
		c.stats.ConnectFailures++
		c.mu.Unlock()
		return fmt.Errorf("failed to connect to broker: %w", err)
	}

	c.mu.Lock()
	c.setState(ClientStateConnected)
	c.stats.ConnectSuccesses++
	c.stats.LastConnectTime = time.Now()
	c.mu.Unlock()

	// Start background workers
	c.startWorkers()

	return nil
}

// Close closes the client and releases resources.
func (c *Client) Close() error {
	c.mu.Lock()
	if c.state == ClientStateClosed {
		c.mu.Unlock()
		return nil
	}
	c.setState(ClientStateClosed)
	c.mu.Unlock()

	// Cancel background workers
	c.cancel()
	c.wg.Wait()

	// Close broker connection
	if c.broker != nil {
		_ = c.broker.Close()
	}

	// Close disk cache
	if c.diskCache != nil {
		_ = c.diskCache.Close()
	}

	return nil
}

// State returns the current client state.
func (c *Client) State() ClientState {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.state
}

// Stats returns the current client statistics.
func (c *Client) Stats() ClientStats {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.stats
}

// setState changes the client state and invokes callback.
func (c *Client) setState(newState ClientState) {
	oldState := c.state
	c.state = newState

	if c.onStateChange != nil && oldState != newState {
		go c.onStateChange(oldState, newState)
	}
}

// startWorkers starts background workers.
func (c *Client) startWorkers() {
	// Start refresh scheduler
	if c.refreshScheduler != nil {
		c.wg.Add(1)
		go func() {
			defer c.wg.Done()
			c.refreshScheduler.Run(c.ctx)
		}()
	}

	// Start batcher
	if c.batcher != nil {
		c.wg.Add(1)
		go func() {
			defer c.wg.Done()
			c.batcher.Run(c.ctx)
		}()
	}
}

// Get retrieves a secret by path.
func (c *Client) Get(ctx context.Context, path string) (*secrets.Secret, error) {
	return c.GetWithOptions(ctx, &SecretRequest{Path: path})
}

// GetWithOptions retrieves a secret with full request options.
func (c *Client) GetWithOptions(ctx context.Context, req *SecretRequest) (*secrets.Secret, error) {
	if req == nil || req.Path == "" {
		return nil, fmt.Errorf("path is required")
	}

	c.mu.Lock()
	c.stats.RequestCount++
	c.stats.LastRequestTime = time.Now()
	c.mu.Unlock()

	// Check memory cache first
	if c.memoryCache != nil {
		if secret, ok := c.memoryCache.Get(req.Path); ok {
			c.mu.Lock()
			c.stats.CacheHits++
			c.mu.Unlock()
			return secret, nil
		}
	}

	// Check disk cache
	if c.diskCache != nil {
		if secret, err := c.diskCache.Get(req.Path); err == nil && secret != nil {
			c.mu.Lock()
			c.stats.CacheHits++
			c.mu.Unlock()
			// Populate memory cache
			if c.memoryCache != nil {
				c.memoryCache.Set(req.Path, secret)
			}
			return secret, nil
		}
	}

	c.mu.Lock()
	c.stats.CacheMisses++
	c.mu.Unlock()

	// Fetch from broker
	return c.fetchFromBroker(ctx, req)
}

// GetBatch retrieves multiple secrets in a single call.
func (c *Client) GetBatch(ctx context.Context, paths []string) (map[string]*secrets.Secret, error) {
	if len(paths) == 0 {
		return make(map[string]*secrets.Secret), nil
	}

	reqs := make([]*SecretRequest, len(paths))
	for i, path := range paths {
		reqs[i] = &SecretRequest{Path: path}
	}

	return c.GetBatchWithOptions(ctx, reqs)
}

// GetBatchWithOptions retrieves multiple secrets with full request options.
func (c *Client) GetBatchWithOptions(ctx context.Context, reqs []*SecretRequest) (map[string]*secrets.Secret, error) {
	if len(reqs) == 0 {
		return make(map[string]*secrets.Secret), nil
	}

	result := make(map[string]*secrets.Secret)
	var toFetch []*SecretRequest

	// Check caches first
	for _, req := range reqs {
		found := false

		// Check memory cache
		if c.memoryCache != nil {
			if secret, ok := c.memoryCache.Get(req.Path); ok {
				result[req.Path] = secret
				c.mu.Lock()
				c.stats.CacheHits++
				c.mu.Unlock()
				found = true
			}
		}

		// Check disk cache
		if !found && c.diskCache != nil {
			if secret, err := c.diskCache.Get(req.Path); err == nil && secret != nil {
				result[req.Path] = secret
				c.mu.Lock()
				c.stats.CacheHits++
				c.mu.Unlock()
				// Populate memory cache
				if c.memoryCache != nil {
					c.memoryCache.Set(req.Path, secret)
				}
				found = true
			}
		}

		if !found {
			toFetch = append(toFetch, req)
			c.mu.Lock()
			c.stats.CacheMisses++
			c.mu.Unlock()
		}
	}

	// Fetch remaining from broker
	if len(toFetch) > 0 {
		fetched, err := c.fetchBatchFromBroker(ctx, toFetch)
		if err != nil {
			return result, err
		}
		for path, secret := range fetched {
			result[path] = secret
		}
	}

	return result, nil
}

// List lists secrets under a path prefix.
func (c *Client) List(ctx context.Context, prefix string) ([]string, error) {
	c.mu.RLock()
	state := c.state
	c.mu.RUnlock()

	if state != ClientStateConnected {
		return nil, fmt.Errorf("client not connected")
	}

	reqCtx, cancel := context.WithTimeout(ctx, c.config.RequestTimeout)
	defer cancel()

	return c.broker.ListSecrets(reqCtx, prefix)
}

// RenewLease renews a lease for a dynamic secret.
func (c *Client) RenewLease(ctx context.Context, leaseID string, increment time.Duration) (*secrets.Lease, error) {
	c.mu.RLock()
	state := c.state
	c.mu.RUnlock()

	if state != ClientStateConnected {
		return nil, fmt.Errorf("client not connected")
	}

	reqCtx, cancel := context.WithTimeout(ctx, c.config.RequestTimeout)
	defer cancel()

	return c.broker.RenewLease(reqCtx, leaseID, increment)
}

// Invalidate removes a secret from the cache.
func (c *Client) Invalidate(path string) {
	if c.memoryCache != nil {
		c.memoryCache.Delete(path)
	}
	if c.diskCache != nil {
		_ = c.diskCache.Delete(path)
	}
}

// InvalidateAll clears all cached secrets.
func (c *Client) InvalidateAll() {
	if c.memoryCache != nil {
		c.memoryCache.Clear()
	}
	if c.diskCache != nil {
		_ = c.diskCache.Clear()
	}
}

// fetchFromBroker fetches a secret from the broker.
func (c *Client) fetchFromBroker(ctx context.Context, req *SecretRequest) (*secrets.Secret, error) {
	c.mu.RLock()
	state := c.state
	c.mu.RUnlock()

	if state != ClientStateConnected {
		return nil, fmt.Errorf("client not connected")
	}

	reqCtx, cancel := context.WithTimeout(ctx, c.config.RequestTimeout)
	defer cancel()

	secret, err := c.broker.GetSecret(reqCtx, req)
	if err != nil {
		return nil, err
	}

	// Cache the secret
	c.cacheSecret(req.Path, secret)

	// Schedule refresh if applicable
	if c.refreshScheduler != nil && !secret.ExpiresAt.IsZero() {
		c.refreshScheduler.Schedule(req.Path, secret.ExpiresAt)
	}

	return secret, nil
}

// fetchBatchFromBroker fetches multiple secrets from the broker.
func (c *Client) fetchBatchFromBroker(ctx context.Context, reqs []*SecretRequest) (map[string]*secrets.Secret, error) {
	c.mu.RLock()
	state := c.state
	c.mu.RUnlock()

	if state != ClientStateConnected {
		return nil, fmt.Errorf("client not connected")
	}

	reqCtx, cancel := context.WithTimeout(ctx, c.config.RequestTimeout)
	defer cancel()

	fetched, err := c.broker.GetSecretBatch(reqCtx, reqs)
	if err != nil {
		return nil, err
	}

	result := make(map[string]*secrets.Secret)
	for i, secret := range fetched {
		if secret != nil {
			path := reqs[i].Path
			result[path] = secret
			c.cacheSecret(path, secret)

			// Schedule refresh if applicable
			if c.refreshScheduler != nil && !secret.ExpiresAt.IsZero() {
				c.refreshScheduler.Schedule(path, secret.ExpiresAt)
			}
		}
	}

	c.mu.Lock()
	c.stats.BatchCount++
	c.mu.Unlock()

	return result, nil
}

// cacheSecret stores a secret in the cache.
func (c *Client) cacheSecret(path string, secret *secrets.Secret) {
	if c.memoryCache != nil {
		c.memoryCache.Set(path, secret)
	}
	if c.diskCache != nil {
		_ = c.diskCache.Set(path, secret)
	}
}

// refreshSecret refreshes a secret from the broker.
func (c *Client) refreshSecret(ctx context.Context, path string) error {
	secret, err := c.fetchFromBroker(ctx, &SecretRequest{Path: path})
	if err != nil {
		c.mu.Lock()
		c.stats.RefreshErrors++
		c.mu.Unlock()
		if c.onError != nil {
			c.onError(fmt.Errorf("failed to refresh secret %s: %w", path, err))
		}
		return err
	}

	c.mu.Lock()
	c.stats.RefreshCount++
	c.stats.LastRefreshTime = time.Now()
	c.mu.Unlock()

	if c.onSecretRefresh != nil {
		c.onSecretRefresh(path, secret)
	}

	return nil
}

// =============================================================================
// Memory Cache
// =============================================================================

// MemoryCache is an in-memory secret cache with TTL support.
type MemoryCache struct {
	mu         sync.RWMutex
	entries    map[string]*memoryCacheEntry
	maxEntries int
	defaultTTL time.Duration
}

type memoryCacheEntry struct {
	secret    *secrets.Secret
	expiresAt time.Time
}

// NewMemoryCache creates a new memory cache.
func NewMemoryCache(maxEntries int, defaultTTL time.Duration) *MemoryCache {
	if maxEntries <= 0 {
		maxEntries = 1000
	}
	if defaultTTL <= 0 {
		defaultTTL = 5 * time.Minute
	}
	return &MemoryCache{
		entries:    make(map[string]*memoryCacheEntry),
		maxEntries: maxEntries,
		defaultTTL: defaultTTL,
	}
}

// Get retrieves a secret from the cache.
func (c *MemoryCache) Get(path string) (*secrets.Secret, bool) {
	c.mu.RLock()
	entry, ok := c.entries[path]
	c.mu.RUnlock()

	if !ok {
		return nil, false
	}

	if time.Now().After(entry.expiresAt) {
		c.Delete(path)
		return nil, false
	}

	return entry.secret, true
}

// Set stores a secret in the cache.
func (c *MemoryCache) Set(path string, secret *secrets.Secret) {
	c.mu.Lock()
	defer c.mu.Unlock()

	// Evict if at capacity (simple random eviction)
	if len(c.entries) >= c.maxEntries && c.entries[path] == nil {
		for k := range c.entries {
			delete(c.entries, k)
			break
		}
	}

	expiresAt := time.Now().Add(c.defaultTTL)
	if !secret.ExpiresAt.IsZero() && secret.ExpiresAt.Before(expiresAt) {
		expiresAt = secret.ExpiresAt
	}

	c.entries[path] = &memoryCacheEntry{
		secret:    secret,
		expiresAt: expiresAt,
	}
}

// Delete removes a secret from the cache.
func (c *MemoryCache) Delete(path string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.entries, path)
}

// Clear removes all entries from the cache.
func (c *MemoryCache) Clear() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.entries = make(map[string]*memoryCacheEntry)
}

// Size returns the number of entries in the cache.
func (c *MemoryCache) Size() int {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return len(c.entries)
}

// =============================================================================
// Disk Cache
// =============================================================================

// DiskCache is an encrypted disk-based secret cache.
type DiskCache struct {
	mu            sync.Mutex
	path          string
	encryptionKey []byte
}

// NewDiskCache creates a new disk cache.
func NewDiskCache(path string, encryptionKey []byte) (*DiskCache, error) {
	if path == "" {
		return nil, fmt.Errorf("path is required")
	}

	// Generate a key if not provided
	if len(encryptionKey) == 0 {
		encryptionKey = make([]byte, 32)
		if _, err := io.ReadFull(rand.Reader, encryptionKey); err != nil {
			return nil, fmt.Errorf("failed to generate encryption key: %w", err)
		}
	}

	if len(encryptionKey) != 32 {
		return nil, fmt.Errorf("encryption key must be 32 bytes")
	}

	// Create directory if it doesn't exist
	if err := os.MkdirAll(path, 0700); err != nil {
		return nil, fmt.Errorf("failed to create cache directory: %w", err)
	}

	return &DiskCache{
		path:          path,
		encryptionKey: encryptionKey,
	}, nil
}

// Get retrieves a secret from the disk cache.
func (c *DiskCache) Get(path string) (*secrets.Secret, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	filename := c.pathToFilename(path)
	data, err := os.ReadFile(filename)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	// Decrypt
	plaintext, err := c.decrypt(data)
	if err != nil {
		return nil, fmt.Errorf("failed to decrypt: %w", err)
	}

	var secret secrets.Secret
	if err := json.Unmarshal(plaintext, &secret); err != nil {
		return nil, fmt.Errorf("failed to unmarshal: %w", err)
	}

	// Check expiration
	if !secret.ExpiresAt.IsZero() && time.Now().After(secret.ExpiresAt) {
		_ = c.Delete(path)
		return nil, nil
	}

	return &secret, nil
}

// Set stores a secret in the disk cache.
func (c *DiskCache) Set(path string, secret *secrets.Secret) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	data, err := json.Marshal(secret)
	if err != nil {
		return fmt.Errorf("failed to marshal: %w", err)
	}

	// Encrypt
	ciphertext, err := c.encrypt(data)
	if err != nil {
		return fmt.Errorf("failed to encrypt: %w", err)
	}

	filename := c.pathToFilename(path)
	if err := os.WriteFile(filename, ciphertext, 0600); err != nil {
		return fmt.Errorf("failed to write: %w", err)
	}

	return nil
}

// Delete removes a secret from the disk cache.
func (c *DiskCache) Delete(path string) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	filename := c.pathToFilename(path)
	if err := os.Remove(filename); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

// Clear removes all entries from the disk cache.
func (c *DiskCache) Clear() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	entries, err := os.ReadDir(c.path)
	if err != nil {
		return err
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			_ = os.Remove(filepath.Join(c.path, entry.Name()))
		}
	}

	return nil
}

// Close closes the disk cache.
func (c *DiskCache) Close() error {
	return nil
}

func (c *DiskCache) pathToFilename(path string) string {
	// Hash the path to create a valid filename
	hash := sha256.Sum256([]byte(path))
	return filepath.Join(c.path, fmt.Sprintf("%x.enc", hash[:16]))
}

func (c *DiskCache) encrypt(plaintext []byte) ([]byte, error) {
	block, err := aes.NewCipher(c.encryptionKey)
	if err != nil {
		return nil, err
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}

	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, err
	}

	return gcm.Seal(nonce, nonce, plaintext, nil), nil
}

func (c *DiskCache) decrypt(ciphertext []byte) ([]byte, error) {
	block, err := aes.NewCipher(c.encryptionKey)
	if err != nil {
		return nil, err
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}

	if len(ciphertext) < gcm.NonceSize() {
		return nil, fmt.Errorf("ciphertext too short")
	}

	nonce, ciphertext := ciphertext[:gcm.NonceSize()], ciphertext[gcm.NonceSize():]
	return gcm.Open(nil, nonce, ciphertext, nil)
}
