// Package ratelimit provides rate limiting strategies for Keystone.
package ratelimit

import (
	"context"
	"fmt"
	"sync"
	"time"
)

// Strategy represents a rate limiting strategy.
type Strategy string

const (
	// StrategyTokenBucket uses a token bucket algorithm.
	StrategyTokenBucket Strategy = "token_bucket"
	// StrategySlidingWindow uses a sliding window algorithm.
	StrategySlidingWindow Strategy = "sliding_window"
	// StrategyFixedWindow uses a fixed window algorithm.
	StrategyFixedWindow Strategy = "fixed_window"
	// StrategyLeakyBucket uses a leaky bucket algorithm.
	StrategyLeakyBucket Strategy = "leaky_bucket"
)

// Result represents the result of a rate limit check.
type Result struct {
	Allowed    bool          `json:"allowed"`
	Remaining  int64         `json:"remaining"`
	Limit      int64         `json:"limit"`
	RetryAfter time.Duration `json:"retryAfter,omitempty"`
	ResetAt    time.Time     `json:"resetAt,omitempty"`
}

// Limiter is the interface for rate limiters.
type Limiter interface {
	Allow(ctx context.Context, key string) (*Result, error)
	AllowN(ctx context.Context, key string, n int64) (*Result, error)
	Reset(ctx context.Context, key string) error
}

// Config configures a rate limiter.
type Config struct {
	Strategy        Strategy      `json:"strategy"`
	Limit           int64         `json:"limit"`
	Window          time.Duration `json:"window"`
	BurstSize       int64         `json:"burstSize,omitempty"`
	RefillRate      float64       `json:"refillRate,omitempty"` // tokens per second
	CleanupInterval time.Duration `json:"cleanupInterval,omitempty"`
}

// DefaultConfig returns a default rate limiter configuration.
func DefaultConfig() *Config {
	return &Config{
		Strategy:        StrategyTokenBucket,
		Limit:           100,
		Window:          time.Minute,
		BurstSize:       10,
		RefillRate:      1.0,
		CleanupInterval: 5 * time.Minute,
	}
}

// TokenBucket implements the token bucket algorithm.
type TokenBucket struct {
	config  *Config
	buckets map[string]*bucket
	mu      sync.RWMutex
	stopCh  chan struct{}
}

type bucket struct {
	tokens     float64
	lastRefill time.Time
	mu         sync.Mutex
}

// NewTokenBucket creates a new token bucket limiter.
func NewTokenBucket(config *Config) *TokenBucket {
	if config.BurstSize == 0 {
		config.BurstSize = config.Limit
	}
	if config.Window == 0 {
		config.Window = time.Minute
	}
	if config.RefillRate == 0 {
		config.RefillRate = float64(config.Limit) / config.Window.Seconds()
	}

	tb := &TokenBucket{
		config:  config,
		buckets: make(map[string]*bucket),
		stopCh:  make(chan struct{}),
	}

	// Start cleanup goroutine
	if config.CleanupInterval > 0 {
		go tb.cleanup()
	}

	return tb
}

// Allow checks if a single request is allowed.
func (tb *TokenBucket) Allow(ctx context.Context, key string) (*Result, error) {
	return tb.AllowN(ctx, key, 1)
}

// AllowN checks if n requests are allowed.
func (tb *TokenBucket) AllowN(ctx context.Context, key string, n int64) (*Result, error) {
	b := tb.getBucket(key)
	b.mu.Lock()
	defer b.mu.Unlock()

	now := time.Now()
	tb.refill(b, now)

	if b.tokens >= float64(n) {
		b.tokens -= float64(n)
		return &Result{
			Allowed:   true,
			Remaining: int64(b.tokens),
			Limit:     tb.config.BurstSize,
		}, nil
	}

	// Calculate retry after
	needed := float64(n) - b.tokens
	retryAfter := time.Duration(needed / tb.config.RefillRate * float64(time.Second))

	return &Result{
		Allowed:    false,
		Remaining:  0,
		Limit:      tb.config.BurstSize,
		RetryAfter: retryAfter,
	}, nil
}

// Reset resets the rate limit for a key.
func (tb *TokenBucket) Reset(ctx context.Context, key string) error {
	tb.mu.Lock()
	defer tb.mu.Unlock()

	delete(tb.buckets, key)
	return nil
}

// Stop stops the token bucket limiter.
func (tb *TokenBucket) Stop() {
	close(tb.stopCh)
}

func (tb *TokenBucket) getBucket(key string) *bucket {
	tb.mu.RLock()
	b, ok := tb.buckets[key]
	tb.mu.RUnlock()

	if ok {
		return b
	}

	tb.mu.Lock()
	defer tb.mu.Unlock()

	// Double check
	if b, ok := tb.buckets[key]; ok {
		return b
	}

	b = &bucket{
		tokens:     float64(tb.config.BurstSize),
		lastRefill: time.Now(),
	}
	tb.buckets[key] = b
	return b
}

func (tb *TokenBucket) refill(b *bucket, now time.Time) {
	elapsed := now.Sub(b.lastRefill).Seconds()
	tokens := b.tokens + (elapsed * tb.config.RefillRate)

	if tokens > float64(tb.config.BurstSize) {
		tokens = float64(tb.config.BurstSize)
	}

	b.tokens = tokens
	b.lastRefill = now
}

func (tb *TokenBucket) cleanup() {
	ticker := time.NewTicker(tb.config.CleanupInterval)
	defer ticker.Stop()

	for {
		select {
		case <-tb.stopCh:
			return
		case <-ticker.C:
			tb.doCleanup()
		}
	}
}

func (tb *TokenBucket) doCleanup() {
	tb.mu.Lock()
	defer tb.mu.Unlock()

	now := time.Now()
	threshold := tb.config.Window * 2

	for key, b := range tb.buckets {
		b.mu.Lock()
		if now.Sub(b.lastRefill) > threshold {
			delete(tb.buckets, key)
		}
		b.mu.Unlock()
	}
}

// SlidingWindow implements the sliding window algorithm.
type SlidingWindow struct {
	config  *Config
	windows map[string]*window
	mu      sync.RWMutex
	stopCh  chan struct{}
}

type window struct {
	counts []windowCount
	mu     sync.Mutex
}

type windowCount struct {
	timestamp time.Time
	count     int64
}

// NewSlidingWindow creates a new sliding window limiter.
func NewSlidingWindow(config *Config) *SlidingWindow {
	sw := &SlidingWindow{
		config:  config,
		windows: make(map[string]*window),
		stopCh:  make(chan struct{}),
	}

	if config.CleanupInterval > 0 {
		go sw.cleanup()
	}

	return sw
}

// Allow checks if a single request is allowed.
func (sw *SlidingWindow) Allow(ctx context.Context, key string) (*Result, error) {
	return sw.AllowN(ctx, key, 1)
}

// AllowN checks if n requests are allowed.
func (sw *SlidingWindow) AllowN(ctx context.Context, key string, n int64) (*Result, error) {
	w := sw.getWindow(key)
	w.mu.Lock()
	defer w.mu.Unlock()

	now := time.Now()
	windowStart := now.Add(-sw.config.Window)

	// Clean old entries and count
	var validCounts []windowCount
	var total int64

	for _, wc := range w.counts {
		if wc.timestamp.After(windowStart) {
			validCounts = append(validCounts, wc)
			total += wc.count
		}
	}
	w.counts = validCounts

	if total+n <= sw.config.Limit {
		w.counts = append(w.counts, windowCount{
			timestamp: now,
			count:     n,
		})

		return &Result{
			Allowed:   true,
			Remaining: sw.config.Limit - total - n,
			Limit:     sw.config.Limit,
			ResetAt:   now.Add(sw.config.Window),
		}, nil
	}

	// Find oldest entry to calculate retry
	var oldestTime time.Time
	for _, wc := range w.counts {
		if oldestTime.IsZero() || wc.timestamp.Before(oldestTime) {
			oldestTime = wc.timestamp
		}
	}

	retryAfter := oldestTime.Add(sw.config.Window).Sub(now)
	if retryAfter < 0 {
		retryAfter = 0
	}

	return &Result{
		Allowed:    false,
		Remaining:  0,
		Limit:      sw.config.Limit,
		RetryAfter: retryAfter,
		ResetAt:    now.Add(sw.config.Window),
	}, nil
}

// Reset resets the rate limit for a key.
func (sw *SlidingWindow) Reset(ctx context.Context, key string) error {
	sw.mu.Lock()
	defer sw.mu.Unlock()

	delete(sw.windows, key)
	return nil
}

// Stop stops the sliding window limiter.
func (sw *SlidingWindow) Stop() {
	close(sw.stopCh)
}

func (sw *SlidingWindow) getWindow(key string) *window {
	sw.mu.RLock()
	w, ok := sw.windows[key]
	sw.mu.RUnlock()

	if ok {
		return w
	}

	sw.mu.Lock()
	defer sw.mu.Unlock()

	if w, ok := sw.windows[key]; ok {
		return w
	}

	w = &window{}
	sw.windows[key] = w
	return w
}

func (sw *SlidingWindow) cleanup() {
	ticker := time.NewTicker(sw.config.CleanupInterval)
	defer ticker.Stop()

	for {
		select {
		case <-sw.stopCh:
			return
		case <-ticker.C:
			sw.doCleanup()
		}
	}
}

func (sw *SlidingWindow) doCleanup() {
	sw.mu.Lock()
	defer sw.mu.Unlock()

	now := time.Now()
	windowStart := now.Add(-sw.config.Window)

	for key, w := range sw.windows {
		w.mu.Lock()
		hasValid := false
		for _, wc := range w.counts {
			if wc.timestamp.After(windowStart) {
				hasValid = true
				break
			}
		}
		if !hasValid {
			delete(sw.windows, key)
		}
		w.mu.Unlock()
	}
}

// FixedWindow implements the fixed window algorithm.
type FixedWindow struct {
	config  *Config
	windows map[string]*fixedWindowEntry
	mu      sync.RWMutex
	stopCh  chan struct{}
}

type fixedWindowEntry struct {
	count     int64
	windowEnd time.Time
	mu        sync.Mutex
}

// NewFixedWindow creates a new fixed window limiter.
func NewFixedWindow(config *Config) *FixedWindow {
	fw := &FixedWindow{
		config:  config,
		windows: make(map[string]*fixedWindowEntry),
		stopCh:  make(chan struct{}),
	}

	if config.CleanupInterval > 0 {
		go fw.cleanup()
	}

	return fw
}

// Allow checks if a single request is allowed.
func (fw *FixedWindow) Allow(ctx context.Context, key string) (*Result, error) {
	return fw.AllowN(ctx, key, 1)
}

// AllowN checks if n requests are allowed.
func (fw *FixedWindow) AllowN(ctx context.Context, key string, n int64) (*Result, error) {
	w := fw.getWindow(key)
	w.mu.Lock()
	defer w.mu.Unlock()

	now := time.Now()

	// Check if window has expired
	if now.After(w.windowEnd) {
		w.count = 0
		w.windowEnd = now.Add(fw.config.Window)
	}

	if w.count+n <= fw.config.Limit {
		w.count += n
		return &Result{
			Allowed:   true,
			Remaining: fw.config.Limit - w.count,
			Limit:     fw.config.Limit,
			ResetAt:   w.windowEnd,
		}, nil
	}

	retryAfter := w.windowEnd.Sub(now)

	return &Result{
		Allowed:    false,
		Remaining:  0,
		Limit:      fw.config.Limit,
		RetryAfter: retryAfter,
		ResetAt:    w.windowEnd,
	}, nil
}

// Reset resets the rate limit for a key.
func (fw *FixedWindow) Reset(ctx context.Context, key string) error {
	fw.mu.Lock()
	defer fw.mu.Unlock()

	delete(fw.windows, key)
	return nil
}

// Stop stops the fixed window limiter.
func (fw *FixedWindow) Stop() {
	close(fw.stopCh)
}

func (fw *FixedWindow) getWindow(key string) *fixedWindowEntry {
	fw.mu.RLock()
	w, ok := fw.windows[key]
	fw.mu.RUnlock()

	if ok {
		return w
	}

	fw.mu.Lock()
	defer fw.mu.Unlock()

	if w, ok := fw.windows[key]; ok {
		return w
	}

	w = &fixedWindowEntry{
		windowEnd: time.Now().Add(fw.config.Window),
	}
	fw.windows[key] = w
	return w
}

func (fw *FixedWindow) cleanup() {
	ticker := time.NewTicker(fw.config.CleanupInterval)
	defer ticker.Stop()

	for {
		select {
		case <-fw.stopCh:
			return
		case <-ticker.C:
			fw.doCleanup()
		}
	}
}

func (fw *FixedWindow) doCleanup() {
	fw.mu.Lock()
	defer fw.mu.Unlock()

	now := time.Now()

	for key, w := range fw.windows {
		w.mu.Lock()
		if now.After(w.windowEnd.Add(fw.config.Window)) {
			delete(fw.windows, key)
		}
		w.mu.Unlock()
	}
}

// LeakyBucket implements the leaky bucket algorithm.
type LeakyBucket struct {
	config  *Config
	buckets map[string]*leakyBucketEntry
	mu      sync.RWMutex
	stopCh  chan struct{}
}

type leakyBucketEntry struct {
	level    float64
	lastLeak time.Time
	mu       sync.Mutex
}

// NewLeakyBucket creates a new leaky bucket limiter.
func NewLeakyBucket(config *Config) *LeakyBucket {
	if config.RefillRate == 0 {
		config.RefillRate = float64(config.Limit) / config.Window.Seconds()
	}

	lb := &LeakyBucket{
		config:  config,
		buckets: make(map[string]*leakyBucketEntry),
		stopCh:  make(chan struct{}),
	}

	if config.CleanupInterval > 0 {
		go lb.cleanup()
	}

	return lb
}

// Allow checks if a single request is allowed.
func (lb *LeakyBucket) Allow(ctx context.Context, key string) (*Result, error) {
	return lb.AllowN(ctx, key, 1)
}

// AllowN checks if n requests are allowed.
func (lb *LeakyBucket) AllowN(ctx context.Context, key string, n int64) (*Result, error) {
	b := lb.getBucket(key)
	b.mu.Lock()
	defer b.mu.Unlock()

	now := time.Now()
	lb.leak(b, now)

	capacity := float64(lb.config.Limit)

	if b.level+float64(n) <= capacity {
		b.level += float64(n)
		return &Result{
			Allowed:   true,
			Remaining: int64(capacity - b.level),
			Limit:     lb.config.Limit,
		}, nil
	}

	// Calculate retry after
	overflow := b.level + float64(n) - capacity
	retryAfter := time.Duration(overflow/lb.config.RefillRate) * time.Second

	return &Result{
		Allowed:    false,
		Remaining:  0,
		Limit:      lb.config.Limit,
		RetryAfter: retryAfter,
	}, nil
}

// Reset resets the rate limit for a key.
func (lb *LeakyBucket) Reset(ctx context.Context, key string) error {
	lb.mu.Lock()
	defer lb.mu.Unlock()

	delete(lb.buckets, key)
	return nil
}

// Stop stops the leaky bucket limiter.
func (lb *LeakyBucket) Stop() {
	close(lb.stopCh)
}

func (lb *LeakyBucket) getBucket(key string) *leakyBucketEntry {
	lb.mu.RLock()
	b, ok := lb.buckets[key]
	lb.mu.RUnlock()

	if ok {
		return b
	}

	lb.mu.Lock()
	defer lb.mu.Unlock()

	if b, ok := lb.buckets[key]; ok {
		return b
	}

	b = &leakyBucketEntry{
		lastLeak: time.Now(),
	}
	lb.buckets[key] = b
	return b
}

func (lb *LeakyBucket) leak(b *leakyBucketEntry, now time.Time) {
	elapsed := now.Sub(b.lastLeak).Seconds()
	leaked := elapsed * lb.config.RefillRate

	b.level -= leaked
	if b.level < 0 {
		b.level = 0
	}
	b.lastLeak = now
}

func (lb *LeakyBucket) cleanup() {
	ticker := time.NewTicker(lb.config.CleanupInterval)
	defer ticker.Stop()

	for {
		select {
		case <-lb.stopCh:
			return
		case <-ticker.C:
			lb.doCleanup()
		}
	}
}

func (lb *LeakyBucket) doCleanup() {
	lb.mu.Lock()
	defer lb.mu.Unlock()

	now := time.Now()
	threshold := lb.config.Window * 2

	for key, b := range lb.buckets {
		b.mu.Lock()
		if now.Sub(b.lastLeak) > threshold && b.level <= 0 {
			delete(lb.buckets, key)
		}
		b.mu.Unlock()
	}
}

// NewLimiter creates a new limiter based on the strategy.
func NewLimiter(config *Config) (Limiter, error) {
	switch config.Strategy {
	case StrategyTokenBucket:
		return NewTokenBucket(config), nil
	case StrategySlidingWindow:
		return NewSlidingWindow(config), nil
	case StrategyFixedWindow:
		return NewFixedWindow(config), nil
	case StrategyLeakyBucket:
		return NewLeakyBucket(config), nil
	default:
		return nil, fmt.Errorf("unknown strategy: %s", config.Strategy)
	}
}

// MultiLimiter combines multiple limiters.
type MultiLimiter struct {
	limiters []Limiter
}

// NewMultiLimiter creates a new multi-limiter.
func NewMultiLimiter(limiters ...Limiter) *MultiLimiter {
	return &MultiLimiter{limiters: limiters}
}

// Allow checks if request is allowed by all limiters.
func (ml *MultiLimiter) Allow(ctx context.Context, key string) (*Result, error) {
	return ml.AllowN(ctx, key, 1)
}

// AllowN checks if n requests are allowed by all limiters.
func (ml *MultiLimiter) AllowN(ctx context.Context, key string, n int64) (*Result, error) {
	var results []*Result

	for _, limiter := range ml.limiters {
		result, err := limiter.AllowN(ctx, key, n)
		if err != nil {
			return nil, err
		}
		results = append(results, result)

		if !result.Allowed {
			// Return the most restrictive result
			return result, nil
		}
	}

	// All allowed - return the most restrictive remaining
	if len(results) == 0 {
		return &Result{Allowed: true}, nil
	}

	most := results[0]
	for _, r := range results[1:] {
		if r.Remaining < most.Remaining {
			most = r
		}
	}

	return most, nil
}

// Reset resets all limiters for a key.
func (ml *MultiLimiter) Reset(ctx context.Context, key string) error {
	for _, limiter := range ml.limiters {
		if err := limiter.Reset(ctx, key); err != nil {
			return err
		}
	}
	return nil
}

// KeyFunc extracts a rate limit key from context.
type KeyFunc func(ctx context.Context) string

// Middleware provides rate limiting middleware functionality.
type Middleware struct {
	limiter Limiter
	keyFunc KeyFunc
}

// NewMiddleware creates a new rate limiting middleware.
func NewMiddleware(limiter Limiter, keyFunc KeyFunc) *Middleware {
	return &Middleware{
		limiter: limiter,
		keyFunc: keyFunc,
	}
}

// Check checks if a request is allowed.
func (m *Middleware) Check(ctx context.Context) (*Result, error) {
	key := m.keyFunc(ctx)
	return m.limiter.Allow(ctx, key)
}
