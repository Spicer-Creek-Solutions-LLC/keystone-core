package capabilities

import (
	"fmt"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// SecretsReadCapability allows reading secrets
type SecretsReadCapability struct {
	AllowedPaths []string // Secret paths allowed (e.g., "app/*", "database/password")
	AuditAll     bool     // Whether to audit all secret reads
	store        SecretsStore
}

// Name returns the capability name
func (c *SecretsReadCapability) Name() string {
	return "secrets.read"
}

// Validate checks if the capability configuration is valid
func (c *SecretsReadCapability) Validate() error {
	if len(c.AllowedPaths) == 0 {
		return fmt.Errorf("%w: at least one allowed path required", ErrInvalidConfiguration)
	}
	return nil
}

// SetStore sets the secrets store backend
func (c *SecretsReadCapability) SetStore(store SecretsStore) {
	c.store = store
}

// ReadSecret reads a secret value
func (c *SecretsReadCapability) ReadSecret(ctx *CapabilityContext, path string) (string, error) {
	if c.store == nil {
		return "", fmt.Errorf("secrets store not configured")
	}

	// Check if path is allowed
	allowed := false
	for _, allowedPath := range c.AllowedPaths {
		if matchesPath(allowedPath, path) {
			allowed = true
			break
		}
	}

	if !allowed {
		return "", fmt.Errorf("%w: %s", ErrPathNotAllowed, path)
	}

	// Read from store
	value, err := c.store.Get(path)
	if err != nil {
		return "", fmt.Errorf("failed to read secret: %w", err)
	}

	return value, nil
}

// SecretsWriteCapability allows writing secrets
type SecretsWriteCapability struct {
	AllowedPaths []string // Secret paths allowed for writing
	AuditAll     bool     // Whether to audit all secret writes
	store        SecretsStore
}

// Name returns the capability name
func (c *SecretsWriteCapability) Name() string {
	return "secrets.write"
}

// Validate checks if the capability configuration is valid
func (c *SecretsWriteCapability) Validate() error {
	if len(c.AllowedPaths) == 0 {
		return fmt.Errorf("%w: at least one allowed path required", ErrInvalidConfiguration)
	}
	return nil
}

// SetStore sets the secrets store backend
func (c *SecretsWriteCapability) SetStore(store SecretsStore) {
	c.store = store
}

// WriteSecret writes a secret value
func (c *SecretsWriteCapability) WriteSecret(ctx *CapabilityContext, path, value string) error {
	if c.store == nil {
		return fmt.Errorf("secrets store not configured")
	}

	// Check if path is allowed
	allowed := false
	for _, allowedPath := range c.AllowedPaths {
		if matchesPath(allowedPath, path) {
			allowed = true
			break
		}
	}

	if !allowed {
		return fmt.Errorf("%w: %s", ErrPathNotAllowed, path)
	}

	// Write to store
	if err := c.store.Set(path, value); err != nil {
		return fmt.Errorf("failed to write secret: %w", err)
	}

	return nil
}

// DeleteSecret deletes a secret
func (c *SecretsWriteCapability) DeleteSecret(ctx *CapabilityContext, path string) error {
	if c.store == nil {
		return fmt.Errorf("secrets store not configured")
	}

	// Check if path is allowed
	allowed := false
	for _, allowedPath := range c.AllowedPaths {
		if matchesPath(allowedPath, path) {
			allowed = true
			break
		}
	}

	if !allowed {
		return fmt.Errorf("%w: %s", ErrPathNotAllowed, path)
	}

	// Delete from store
	if err := c.store.Delete(path); err != nil {
		return fmt.Errorf("failed to delete secret: %w", err)
	}

	return nil
}

// SecretsStore defines the interface for secret storage backends
type SecretsStore interface {
	Get(path string) (string, error)
	Set(path, value string) error
	Delete(path string) error
}

// LogCapability allows structured logging
type LogCapability struct {
	RateLimit *RateLimit // Rate limiting for log messages
	logger    Logger

	rateLimiter *rateLimiter
	once        sync.Once
}

// Name returns the capability name
func (c *LogCapability) Name() string {
	return "log"
}

// Validate checks if the capability configuration is valid
func (c *LogCapability) Validate() error {
	if c.RateLimit != nil {
		if err := c.RateLimit.Validate(); err != nil {
			return fmt.Errorf("%w: invalid rate limit: %w", ErrInvalidConfiguration, err)
		}
	}
	return nil
}

// SetLogger sets the logger backend
func (c *LogCapability) SetLogger(logger Logger) {
	c.logger = logger
}

// Log writes a log message
func (c *LogCapability) Log(ctx *CapabilityContext, level, message string, fields map[string]interface{}) error {
	if c.logger == nil {
		return fmt.Errorf("logger not configured")
	}

	// Check rate limit
	c.once.Do(func() {
		if c.RateLimit != nil {
			c.rateLimiter = newRateLimiter(c.RateLimit)
		}
	})

	if c.rateLimiter != nil {
		if !c.rateLimiter.Allow() {
			return ErrRateLimitExceeded
		}
	}

	// Add module context
	if fields == nil {
		fields = make(map[string]interface{})
	}
	fields["module"] = ctx.ModuleName
	fields["correlation_id"] = ctx.CorrelationID

	// Write log
	c.logger.Log(level, message, fields)
	return nil
}

// Logger defines the interface for log backends
type Logger interface {
	Log(level, message string, fields map[string]interface{})
}

// TimeCapability allows accessing current time
// WARNING: This capability breaks determinism!
type TimeCapability struct {
	// No configuration needed
}

// Name returns the capability name
func (c *TimeCapability) Name() string {
	return "time"
}

// Validate checks if the capability configuration is valid
func (c *TimeCapability) Validate() error {
	return nil
}

// Now returns the current time
func (c *TimeCapability) Now(ctx *CapabilityContext) time.Time {
	return time.Now()
}

// Unix returns the current Unix timestamp
func (c *TimeCapability) Unix(ctx *CapabilityContext) int64 {
	return time.Now().Unix()
}

// KVCapability allows key-value storage
type KVCapability struct {
	Namespace string // Namespace for keys (usually module name)
	store     KVStore
}

// Name returns the capability name
func (c *KVCapability) Name() string {
	return "kv"
}

// Validate checks if the capability configuration is valid
func (c *KVCapability) Validate() error {
	if c.Namespace == "" {
		return fmt.Errorf("%w: namespace is required", ErrInvalidConfiguration)
	}
	return nil
}

// SetStore sets the KV store backend
func (c *KVCapability) SetStore(store KVStore) {
	c.store = store
}

// Get retrieves a value from the KV store
func (c *KVCapability) Get(ctx *CapabilityContext, key string) (string, error) {
	if c.store == nil {
		return "", fmt.Errorf("KV store not configured")
	}

	// Prepend namespace to key
	namespacedKey := c.namespaceKey(key)

	value, err := c.store.Get(namespacedKey)
	if err != nil {
		return "", fmt.Errorf("failed to get value: %w", err)
	}

	return value, nil
}

// Set stores a value in the KV store
func (c *KVCapability) Set(ctx *CapabilityContext, key, value string) error {
	if c.store == nil {
		return fmt.Errorf("KV store not configured")
	}

	// Prepend namespace to key
	namespacedKey := c.namespaceKey(key)

	if err := c.store.Set(namespacedKey, value); err != nil {
		return fmt.Errorf("failed to set value: %w", err)
	}

	return nil
}

// Delete removes a value from the KV store
func (c *KVCapability) Delete(ctx *CapabilityContext, key string) error {
	if c.store == nil {
		return fmt.Errorf("KV store not configured")
	}

	// Prepend namespace to key
	namespacedKey := c.namespaceKey(key)

	if err := c.store.Delete(namespacedKey); err != nil {
		return fmt.Errorf("failed to delete value: %w", err)
	}

	return nil
}

// List lists all keys in the namespace
func (c *KVCapability) List(ctx *CapabilityContext) ([]string, error) {
	if c.store == nil {
		return nil, fmt.Errorf("KV store not configured")
	}

	prefix := c.Namespace + "/"
	keys, err := c.store.List(prefix)
	if err != nil {
		return nil, fmt.Errorf("failed to list keys: %w", err)
	}

	// Remove namespace prefix from keys
	result := make([]string, 0, len(keys))
	for _, key := range keys {
		if strings.HasPrefix(key, prefix) {
			result = append(result, key[len(prefix):])
		}
	}

	return result, nil
}

func (c *KVCapability) namespaceKey(key string) string {
	return c.Namespace + "/" + key
}

// KVStore defines the interface for KV storage backends
type KVStore interface {
	Get(key string) (string, error)
	Set(key, value string) error
	Delete(key string) error
	List(prefix string) ([]string, error)
}

// matchesPath checks if a path matches a pattern (supports * wildcard)
func matchesPath(pattern, path string) bool {
	// Exact match
	if pattern == path {
		return true
	}

	// Wildcard match
	matched, err := filepath.Match(pattern, path)
	if err == nil && matched {
		return true
	}

	// Prefix wildcard (app/* matches app/db/password)
	if strings.HasSuffix(pattern, "/*") {
		prefix := pattern[:len(pattern)-2]
		return path == prefix || strings.HasPrefix(path, prefix+"/")
	}

	return false
}
