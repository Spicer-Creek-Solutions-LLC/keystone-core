package auth

import (
	"context"
	"fmt"
	"net"
	"sync"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/peer"
	"google.golang.org/grpc/status"
)

// Standard rate limiting errors
var (
	ErrRateLimited = status.Error(codes.ResourceExhausted, "too many failed authentication attempts, please try again later")
)

// RateLimitConfig configures the authentication rate limiter
type RateLimitConfig struct {
	// MaxFailures is the maximum number of failed attempts before lockout
	// Default: 5
	MaxFailures int

	// LockoutDuration is how long a client is locked out after MaxFailures
	// Default: 15 minutes
	LockoutDuration time.Duration

	// CleanupInterval is how often to clean up expired entries
	// Default: 5 minutes
	CleanupInterval time.Duration

	// Enabled determines if rate limiting is active
	// Default: true
	Enabled bool
}

// DefaultRateLimitConfig returns sensible defaults for rate limiting
func DefaultRateLimitConfig() RateLimitConfig {
	return RateLimitConfig{
		MaxFailures:     5,
		LockoutDuration: 15 * time.Minute,
		CleanupInterval: 5 * time.Minute,
		Enabled:         true,
	}
}

// RateLimiter tracks failed authentication attempts and implements lockout
type RateLimiter struct {
	mu       sync.RWMutex
	config   RateLimitConfig
	failures map[string]*failureRecord
	stopCh   chan struct{}
}

// failureRecord tracks failures for a specific client
type failureRecord struct {
	count       int
	firstSeen   time.Time
	lastSeen    time.Time
	lockedUntil time.Time
}

// NewRateLimiter creates a new rate limiter with the given config
func NewRateLimiter(config RateLimitConfig) *RateLimiter {
	if config.MaxFailures <= 0 {
		config.MaxFailures = 5
	}
	if config.LockoutDuration <= 0 {
		config.LockoutDuration = 15 * time.Minute
	}
	if config.CleanupInterval <= 0 {
		config.CleanupInterval = 5 * time.Minute
	}

	rl := &RateLimiter{
		config:   config,
		failures: make(map[string]*failureRecord),
		stopCh:   make(chan struct{}),
	}

	// Start cleanup goroutine
	go rl.cleanup()

	return rl
}

// Stop stops the rate limiter cleanup goroutine
func (rl *RateLimiter) Stop() {
	close(rl.stopCh)
}

// cleanup periodically removes expired entries
func (rl *RateLimiter) cleanup() {
	ticker := time.NewTicker(rl.config.CleanupInterval)
	defer ticker.Stop()

	for {
		select {
		case <-rl.stopCh:
			return
		case <-ticker.C:
			rl.cleanupExpired()
		}
	}
}

// cleanupExpired removes records that are past their lockout period
func (rl *RateLimiter) cleanupExpired() {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	now := time.Now()
	for key, record := range rl.failures {
		// Remove if lockout has expired and enough time has passed since last failure
		if now.After(record.lockedUntil) && now.Sub(record.lastSeen) > rl.config.LockoutDuration {
			delete(rl.failures, key)
		}
	}
}

// IsAllowed checks if a client is allowed to attempt authentication
func (rl *RateLimiter) IsAllowed(clientID string) bool {
	if !rl.config.Enabled {
		return true
	}

	rl.mu.RLock()
	defer rl.mu.RUnlock()

	record, exists := rl.failures[clientID]
	if !exists {
		return true
	}

	// Check if currently locked out
	if time.Now().Before(record.lockedUntil) {
		return false
	}

	return true
}

// RecordFailure records a failed authentication attempt
func (rl *RateLimiter) RecordFailure(clientID string) {
	if !rl.config.Enabled {
		return
	}

	rl.mu.Lock()
	defer rl.mu.Unlock()

	now := time.Now()
	record, exists := rl.failures[clientID]
	if !exists {
		record = &failureRecord{
			firstSeen: now,
		}
		rl.failures[clientID] = record
	}

	record.count++
	record.lastSeen = now

	// Check if we've exceeded max failures
	if record.count >= rl.config.MaxFailures {
		record.lockedUntil = now.Add(rl.config.LockoutDuration)
	}
}

// RecordSuccess resets the failure count for a client (successful auth)
func (rl *RateLimiter) RecordSuccess(clientID string) {
	if !rl.config.Enabled {
		return
	}

	rl.mu.Lock()
	defer rl.mu.Unlock()

	delete(rl.failures, clientID)
}

// GetLockoutRemaining returns how long until the lockout expires
// Returns 0 if not locked out
func (rl *RateLimiter) GetLockoutRemaining(clientID string) time.Duration {
	rl.mu.RLock()
	defer rl.mu.RUnlock()

	record, exists := rl.failures[clientID]
	if !exists {
		return 0
	}

	remaining := time.Until(record.lockedUntil)
	if remaining < 0 {
		return 0
	}
	return remaining
}

// GetFailureCount returns the current failure count for a client
func (rl *RateLimiter) GetFailureCount(clientID string) int {
	rl.mu.RLock()
	defer rl.mu.RUnlock()

	record, exists := rl.failures[clientID]
	if !exists {
		return 0
	}
	return record.count
}

// Reset clears all rate limit records (useful for testing)
func (rl *RateLimiter) Reset() {
	rl.mu.Lock()
	defer rl.mu.Unlock()
	rl.failures = make(map[string]*failureRecord)
}

// RateLimiterStats holds rate limiter statistics.
type RateLimiterStats struct {
	TrackedClients int
	LockedClients  int
	TotalFailures  int
}

// Stats returns current rate limiter statistics
func (rl *RateLimiter) Stats() RateLimiterStats {
	rl.mu.RLock()
	defer rl.mu.RUnlock()

	stats := RateLimiterStats{
		TrackedClients: len(rl.failures),
	}

	now := time.Now()
	for _, record := range rl.failures {
		stats.TotalFailures += record.count
		if now.Before(record.lockedUntil) {
			stats.LockedClients++
		}
	}

	return stats
}

// ClientIDFromContext extracts a client identifier from the gRPC context
// Uses client IP address as the primary identifier
func ClientIDFromContext(ctx context.Context) string {
	// Try to get peer info (contains client address)
	if p, ok := peer.FromContext(ctx); ok && p.Addr != nil {
		addr := p.Addr.String()
		// Extract just the IP without port
		if host, _, err := net.SplitHostPort(addr); err == nil {
			return host
		}
		return addr
	}

	// Fallback: try to get from metadata (e.g., X-Forwarded-For for proxied requests)
	if md, ok := metadata.FromIncomingContext(ctx); ok {
		if xff := md.Get("x-forwarded-for"); len(xff) > 0 {
			// Take the first IP in the chain (original client)
			return xff[0]
		}
		if realIP := md.Get("x-real-ip"); len(realIP) > 0 {
			return realIP[0]
		}
	}

	// Last resort: unknown client
	return "unknown"
}

// CheckRateLimit is a helper that checks rate limit and returns appropriate error
func (rl *RateLimiter) CheckRateLimit(ctx context.Context) error {
	if !rl.config.Enabled {
		return nil
	}

	clientID := ClientIDFromContext(ctx)
	if !rl.IsAllowed(clientID) {
		remaining := rl.GetLockoutRemaining(clientID)
		return status.Errorf(codes.ResourceExhausted,
			"too many failed authentication attempts, please try again in %s",
			remaining.Round(time.Second))
	}
	return nil
}

// WrapAuthError wraps an authentication error and records the failure
func (rl *RateLimiter) WrapAuthError(ctx context.Context, err error) error {
	if err == nil {
		// Success - clear failures
		if rl.config.Enabled {
			clientID := ClientIDFromContext(ctx)
			rl.RecordSuccess(clientID)
		}
		return nil
	}

	// Record failure
	if rl.config.Enabled {
		clientID := ClientIDFromContext(ctx)
		rl.RecordFailure(clientID)

		// Check if this failure triggered a lockout
		if !rl.IsAllowed(clientID) {
			remaining := rl.GetLockoutRemaining(clientID)
			return status.Errorf(codes.ResourceExhausted,
				"authentication failed; account locked for %s due to too many failed attempts",
				remaining.Round(time.Second))
		}

		// Add remaining attempts to error message
		failCount := rl.GetFailureCount(clientID)
		attemptsRemaining := rl.config.MaxFailures - failCount
		if attemptsRemaining > 0 {
			// Get original error message
			if s, ok := status.FromError(err); ok {
				return status.Errorf(s.Code(),
					"%s (%d attempts remaining before lockout)",
					s.Message(), attemptsRemaining)
			}
		}
	}

	return err
}

// String returns a string representation of the rate limiter config
func (c RateLimitConfig) String() string {
	return fmt.Sprintf("RateLimitConfig{MaxFailures: %d, LockoutDuration: %s, Enabled: %t}",
		c.MaxFailures, c.LockoutDuration, c.Enabled)
}
