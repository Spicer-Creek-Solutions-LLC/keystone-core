package secrets

import (
	"context"
	"fmt"
	"sync"
	"time"
)

// RateLimiter provides rate limiting for secret requests.
type RateLimiter struct {
	config *RateLimitConfig

	mu       sync.RWMutex
	buckets  map[string]*tokenBucket
	global   *tokenBucket
	closed   bool
	stopCh   chan struct{}

	stats RateLimitStats
}

// RateLimitConfig configures the rate limiter.
type RateLimitConfig struct {
	// Enabled enables rate limiting.
	Enabled bool `json:"enabled" yaml:"enabled"`

	// GlobalLimit is the global requests per second limit.
	GlobalLimit float64 `json:"global_limit" yaml:"global_limit"`

	// GlobalBurst is the global burst size.
	GlobalBurst int `json:"global_burst" yaml:"global_burst"`

	// PerClientLimit is the per-client requests per second limit.
	PerClientLimit float64 `json:"per_client_limit" yaml:"per_client_limit"`

	// PerClientBurst is the per-client burst size.
	PerClientBurst int `json:"per_client_burst" yaml:"per_client_burst"`

	// PerPathLimit is the per-path requests per second limit.
	PerPathLimit float64 `json:"per_path_limit" yaml:"per_path_limit"`

	// PerPathBurst is the per-path burst size.
	PerPathBurst int `json:"per_path_burst" yaml:"per_path_burst"`

	// CleanupInterval is how often to clean up expired buckets.
	CleanupInterval time.Duration `json:"cleanup_interval" yaml:"cleanup_interval"`

	// BucketExpiry is how long to keep unused buckets.
	BucketExpiry time.Duration `json:"bucket_expiry" yaml:"bucket_expiry"`

	// PathRules contains path-specific rate limits.
	PathRules []PathRateLimitRule `json:"path_rules,omitempty" yaml:"path_rules,omitempty"`

	// ClientRules contains client-specific rate limits.
	ClientRules []ClientRateLimitRule `json:"client_rules,omitempty" yaml:"client_rules,omitempty"`
}

// PathRateLimitRule defines a rate limit for a specific path pattern.
type PathRateLimitRule struct {
	// PathPrefix is the path prefix to match.
	PathPrefix string `json:"path_prefix" yaml:"path_prefix"`

	// Limit is the requests per second limit.
	Limit float64 `json:"limit" yaml:"limit"`

	// Burst is the burst size.
	Burst int `json:"burst" yaml:"burst"`
}

// ClientRateLimitRule defines a rate limit for a specific client.
type ClientRateLimitRule struct {
	// ClientID is the client identifier (agent ID, SPIFFE ID, etc.).
	ClientID string `json:"client_id" yaml:"client_id"`

	// Limit is the requests per second limit.
	Limit float64 `json:"limit" yaml:"limit"`

	// Burst is the burst size.
	Burst int `json:"burst" yaml:"burst"`
}

// DefaultRateLimitConfig returns a rate limit configuration with sensible defaults.
func DefaultRateLimitConfig() *RateLimitConfig {
	return &RateLimitConfig{
		Enabled:         true,
		GlobalLimit:     1000,  // 1000 requests/second globally
		GlobalBurst:     2000,  // Allow bursts up to 2000
		PerClientLimit:  100,   // 100 requests/second per client
		PerClientBurst:  200,   // Allow bursts up to 200 per client
		PerPathLimit:    50,    // 50 requests/second per path
		PerPathBurst:    100,   // Allow bursts up to 100 per path
		CleanupInterval: time.Minute,
		BucketExpiry:    5 * time.Minute,
	}
}

// RateLimitStats contains rate limiter statistics.
type RateLimitStats struct {
	// TotalRequests is the total number of requests processed.
	TotalRequests int64 `json:"total_requests"`

	// AllowedRequests is the number of allowed requests.
	AllowedRequests int64 `json:"allowed_requests"`

	// RejectedRequests is the number of rejected requests.
	RejectedRequests int64 `json:"rejected_requests"`

	// GlobalRejections is rejections due to global limit.
	GlobalRejections int64 `json:"global_rejections"`

	// ClientRejections is rejections due to client limit.
	ClientRejections int64 `json:"client_rejections"`

	// PathRejections is rejections due to path limit.
	PathRejections int64 `json:"path_rejections"`

	// ActiveBuckets is the current number of active buckets.
	ActiveBuckets int `json:"active_buckets"`
}

// tokenBucket implements a token bucket rate limiter.
type tokenBucket struct {
	mu         sync.Mutex
	tokens     float64
	capacity   float64
	rate       float64  // tokens per second
	lastUpdate time.Time
	lastUsed   time.Time
}

// newTokenBucket creates a new token bucket.
func newTokenBucket(rate float64, burst int) *tokenBucket {
	now := time.Now()
	return &tokenBucket{
		tokens:     float64(burst),
		capacity:   float64(burst),
		rate:       rate,
		lastUpdate: now,
		lastUsed:   now,
	}
}

// allow checks if a request is allowed and consumes a token if so.
func (b *tokenBucket) allow() bool {
	b.mu.Lock()
	defer b.mu.Unlock()

	now := time.Now()
	b.lastUsed = now

	// Add tokens based on elapsed time
	elapsed := now.Sub(b.lastUpdate).Seconds()
	b.tokens += elapsed * b.rate
	if b.tokens > b.capacity {
		b.tokens = b.capacity
	}
	b.lastUpdate = now

	// Check if we have a token
	if b.tokens >= 1 {
		b.tokens--
		return true
	}

	return false
}

// NewRateLimiter creates a new rate limiter.
func NewRateLimiter(config *RateLimitConfig) *RateLimiter {
	if config == nil {
		config = DefaultRateLimitConfig()
	}

	rl := &RateLimiter{
		config:  config,
		buckets: make(map[string]*tokenBucket),
		stopCh:  make(chan struct{}),
	}

	// Create global bucket
	if config.GlobalLimit > 0 {
		rl.global = newTokenBucket(config.GlobalLimit, config.GlobalBurst)
	}

	return rl
}

// Start starts the rate limiter background tasks.
func (r *RateLimiter) Start(ctx context.Context) error {
	if !r.config.Enabled {
		return nil
	}

	go r.cleanupLoop(ctx)
	return nil
}

// Stop stops the rate limiter.
func (r *RateLimiter) Stop() error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.closed {
		return nil
	}
	r.closed = true
	close(r.stopCh)
	return nil
}

// Allow checks if a request is allowed.
func (r *RateLimiter) Allow(ctx context.Context, req *RateLimitRequest) (*RateLimitResult, error) {
	if !r.config.Enabled {
		return &RateLimitResult{Allowed: true}, nil
	}

	r.mu.Lock()
	r.stats.TotalRequests++
	r.mu.Unlock()

	result := &RateLimitResult{
		Allowed: true,
	}

	// Check global limit
	if r.global != nil && !r.global.allow() {
		r.mu.Lock()
		r.stats.RejectedRequests++
		r.stats.GlobalRejections++
		r.mu.Unlock()

		result.Allowed = false
		result.Reason = "global rate limit exceeded"
		result.RetryAfter = r.calculateRetryAfter(r.global)
		return result, nil
	}

	// Check path limit
	if req.Path != "" {
		pathBucket := r.getPathBucket(req.Path)
		if pathBucket != nil && !pathBucket.allow() {
			r.mu.Lock()
			r.stats.RejectedRequests++
			r.stats.PathRejections++
			r.mu.Unlock()

			result.Allowed = false
			result.Reason = fmt.Sprintf("path rate limit exceeded for %s", req.Path)
			result.RetryAfter = r.calculateRetryAfter(pathBucket)
			return result, nil
		}
	}

	// Check client limit
	if req.ClientID != "" {
		clientBucket := r.getClientBucket(req.ClientID)
		if clientBucket != nil && !clientBucket.allow() {
			r.mu.Lock()
			r.stats.RejectedRequests++
			r.stats.ClientRejections++
			r.mu.Unlock()

			result.Allowed = false
			result.Reason = fmt.Sprintf("client rate limit exceeded for %s", req.ClientID)
			result.RetryAfter = r.calculateRetryAfter(clientBucket)
			return result, nil
		}
	}

	r.mu.Lock()
	r.stats.AllowedRequests++
	r.mu.Unlock()

	return result, nil
}

// Stats returns the rate limiter statistics.
func (r *RateLimiter) Stats() RateLimitStats {
	r.mu.RLock()
	defer r.mu.RUnlock()

	stats := r.stats
	stats.ActiveBuckets = len(r.buckets)
	return stats
}

func (r *RateLimiter) getPathBucket(path string) *tokenBucket {
	// Check for path-specific rules first
	for _, rule := range r.config.PathRules {
		if len(path) >= len(rule.PathPrefix) && path[:len(rule.PathPrefix)] == rule.PathPrefix {
			return r.getOrCreateBucket("path:"+rule.PathPrefix, rule.Limit, rule.Burst)
		}
	}

	// Use default path limit
	if r.config.PerPathLimit > 0 {
		return r.getOrCreateBucket("path:"+path, r.config.PerPathLimit, r.config.PerPathBurst)
	}

	return nil
}

func (r *RateLimiter) getClientBucket(clientID string) *tokenBucket {
	// Check for client-specific rules first
	for _, rule := range r.config.ClientRules {
		if rule.ClientID == clientID {
			return r.getOrCreateBucket("client:"+clientID, rule.Limit, rule.Burst)
		}
	}

	// Use default client limit
	if r.config.PerClientLimit > 0 {
		return r.getOrCreateBucket("client:"+clientID, r.config.PerClientLimit, r.config.PerClientBurst)
	}

	return nil
}

func (r *RateLimiter) getOrCreateBucket(key string, rate float64, burst int) *tokenBucket {
	r.mu.Lock()
	defer r.mu.Unlock()

	if bucket, ok := r.buckets[key]; ok {
		return bucket
	}

	bucket := newTokenBucket(rate, burst)
	r.buckets[key] = bucket
	return bucket
}

func (r *RateLimiter) calculateRetryAfter(bucket *tokenBucket) time.Duration {
	bucket.mu.Lock()
	defer bucket.mu.Unlock()

	// Calculate how long until we have 1 token
	tokensNeeded := 1.0 - bucket.tokens
	if tokensNeeded <= 0 {
		return 0
	}

	seconds := tokensNeeded / bucket.rate
	return time.Duration(seconds * float64(time.Second))
}

func (r *RateLimiter) cleanupLoop(ctx context.Context) {
	interval := r.config.CleanupInterval
	if interval <= 0 {
		interval = time.Minute
	}

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-r.stopCh:
			return
		case <-ticker.C:
			r.cleanup()
		}
	}
}

func (r *RateLimiter) cleanup() {
	r.mu.Lock()
	defer r.mu.Unlock()

	expiry := r.config.BucketExpiry
	if expiry <= 0 {
		expiry = 5 * time.Minute
	}

	now := time.Now()
	for key, bucket := range r.buckets {
		bucket.mu.Lock()
		if now.Sub(bucket.lastUsed) > expiry {
			delete(r.buckets, key)
		}
		bucket.mu.Unlock()
	}
}

// RateLimitRequest represents a rate limit check request.
type RateLimitRequest struct {
	// Path is the secret path being accessed.
	Path string `json:"path"`

	// ClientID is the client identifier (agent ID, SPIFFE ID).
	ClientID string `json:"client_id"`

	// RequestID is the unique request identifier.
	RequestID string `json:"request_id,omitempty"`
}

// RateLimitResult represents the result of a rate limit check.
type RateLimitResult struct {
	// Allowed indicates if the request is allowed.
	Allowed bool `json:"allowed"`

	// Reason is the rejection reason if not allowed.
	Reason string `json:"reason,omitempty"`

	// RetryAfter is when to retry if rate limited.
	RetryAfter time.Duration `json:"retry_after,omitempty"`
}

// =============================================================================
// Rate-Limited Broker Wrapper
// =============================================================================

// RateLimitedBroker wraps a SecretBroker with rate limiting.
type RateLimitedBroker struct {
	broker  *SecretBroker
	limiter *RateLimiter
}

// NewRateLimitedBroker creates a new rate-limited broker.
func NewRateLimitedBroker(broker *SecretBroker, config *RateLimitConfig) *RateLimitedBroker {
	return &RateLimitedBroker{
		broker:  broker,
		limiter: NewRateLimiter(config),
	}
}

// Start starts the rate-limited broker.
func (r *RateLimitedBroker) Start(ctx context.Context) error {
	return r.limiter.Start(ctx)
}

// Stop stops the rate-limited broker.
func (r *RateLimitedBroker) Stop() error {
	return r.limiter.Stop()
}

// Read reads a secret with rate limiting.
func (r *RateLimitedBroker) Read(ctx context.Context, req *SecretRequest) (*Secret, error) {
	result, err := r.limiter.Allow(ctx, &RateLimitRequest{
		Path:      req.Path,
		ClientID:  req.AgentID,
		RequestID: req.RequestID,
	})
	if err != nil {
		return nil, err
	}
	if !result.Allowed {
		return nil, &RateLimitError{
			Reason:     result.Reason,
			RetryAfter: result.RetryAfter,
		}
	}

	return r.broker.Read(ctx, req)
}

// ReadDynamic reads a dynamic secret with rate limiting.
func (r *RateLimitedBroker) ReadDynamic(ctx context.Context, req *SecretRequest) (*Secret, error) {
	result, err := r.limiter.Allow(ctx, &RateLimitRequest{
		Path:      req.Path,
		ClientID:  req.AgentID,
		RequestID: req.RequestID,
	})
	if err != nil {
		return nil, err
	}
	if !result.Allowed {
		return nil, &RateLimitError{
			Reason:     result.Reason,
			RetryAfter: result.RetryAfter,
		}
	}

	return r.broker.ReadDynamic(ctx, req)
}

// List lists secrets with rate limiting.
func (r *RateLimitedBroker) List(ctx context.Context, path string) ([]string, error) {
	result, err := r.limiter.Allow(ctx, &RateLimitRequest{
		Path: path,
	})
	if err != nil {
		return nil, err
	}
	if !result.Allowed {
		return nil, &RateLimitError{
			Reason:     result.Reason,
			RetryAfter: result.RetryAfter,
		}
	}

	return r.broker.List(ctx, path)
}

// Broker returns the underlying broker.
func (r *RateLimitedBroker) Broker() *SecretBroker {
	return r.broker
}

// RateLimitStats returns the rate limiter statistics.
func (r *RateLimitedBroker) RateLimitStats() RateLimitStats {
	return r.limiter.Stats()
}

// RateLimitError represents a rate limit error.
type RateLimitError struct {
	Reason     string
	RetryAfter time.Duration
}

func (e *RateLimitError) Error() string {
	if e.RetryAfter > 0 {
		return fmt.Sprintf("rate limited: %s (retry after %v)", e.Reason, e.RetryAfter)
	}
	return fmt.Sprintf("rate limited: %s", e.Reason)
}

// IsRateLimitError returns true if the error is a rate limit error.
func IsRateLimitError(err error) bool {
	_, ok := err.(*RateLimitError)
	return ok
}
