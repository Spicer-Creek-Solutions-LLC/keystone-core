// SPDX-License-Identifier: Apache-2.0

package auth

import (
	"errors"
	"sync"
	"time"
)

// ErrRateLimited is returned by RateLimiter.Allow when the caller is
// currently locked out.
var ErrRateLimited = errors.New("auth: rate limited")

// RateLimitConfig configures a RateLimiter.
type RateLimitConfig struct {
	// MaxFailuresPerWindow triggers a lockout when reached. Default 5.
	MaxFailuresPerWindow int

	// FailureWindow is the sliding window in which failures
	// accumulate. Default 1 minute.
	FailureWindow time.Duration

	// InitialLockout is the lockout duration on first trip. Default
	// 1 second.
	InitialLockout time.Duration

	// MaxLockout caps the exponentially-increasing lockout. Default
	// 15 minutes.
	MaxLockout time.Duration
}

// RateLimiter implements per-client lockout with exponential backoff.
//
// Clients are keyed by an opaque string supplied by the caller (the
// interceptor uses the principal ID when authenticated, or the peer
// IP when not). Successful Allow calls clear failures; RecordFailure
// trips a lockout when the failure count crosses the threshold.
//
// Lockout doubles on each repeated trip (1s → 2s → 4s … capped at
// MaxLockout). After a successful authentication the lockout state
// resets.
//
// Safe for concurrent use.
type RateLimiter struct {
	cfg RateLimitConfig
	now func() time.Time

	mu      sync.Mutex
	clients map[string]*clientState
}

type clientState struct {
	failures      []time.Time   // sliding window of failure timestamps
	lockedUntil   time.Time     // zero = not locked
	lockoutLevel  int           // 0 before first lockout; 1, 2, ... after
	lastClearedAt time.Time     // last successful allow
}

// NewRateLimiter returns a RateLimiter with cfg defaults applied.
func NewRateLimiter(cfg RateLimitConfig) *RateLimiter {
	if cfg.MaxFailuresPerWindow <= 0 {
		cfg.MaxFailuresPerWindow = 5
	}
	if cfg.FailureWindow <= 0 {
		cfg.FailureWindow = time.Minute
	}
	if cfg.InitialLockout <= 0 {
		cfg.InitialLockout = time.Second
	}
	if cfg.MaxLockout <= 0 {
		cfg.MaxLockout = 15 * time.Minute
	}
	return &RateLimiter{
		cfg:     cfg,
		now:     time.Now,
		clients: map[string]*clientState{},
	}
}

// SetClock overrides the clock. Tests only.
func (r *RateLimiter) SetClock(now func() time.Time) {
	r.now = now
}

// Allow returns nil if the client may proceed, ErrRateLimited if it is
// currently locked out.
func (r *RateLimiter) Allow(client string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	state := r.stateLocked(client)
	if !state.lockedUntil.IsZero() && r.now().Before(state.lockedUntil) {
		return ErrRateLimited
	}
	return nil
}

// RecordFailure registers an authentication failure for client. May
// trigger a lockout when the rolling failure count crosses the
// threshold.
func (r *RateLimiter) RecordFailure(client string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	state := r.stateLocked(client)
	now := r.now()

	// Drop failures outside the sliding window.
	cutoff := now.Add(-r.cfg.FailureWindow)
	pruned := state.failures[:0]
	for _, t := range state.failures {
		if t.After(cutoff) {
			pruned = append(pruned, t)
		}
	}
	state.failures = append(pruned, now)

	if len(state.failures) >= r.cfg.MaxFailuresPerWindow {
		state.lockoutLevel++
		dur := r.cfg.InitialLockout << (state.lockoutLevel - 1)
		if dur > r.cfg.MaxLockout || dur <= 0 {
			dur = r.cfg.MaxLockout
		}
		state.lockedUntil = now.Add(dur)
		// Clear the failure window so we don't re-trip immediately at
		// lockout expiry; the next failure starts a fresh window.
		state.failures = state.failures[:0]
	}
}

// RecordSuccess resets failure tracking + lockout for client. Call
// from the interceptor on every successful auth.
func (r *RateLimiter) RecordSuccess(client string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	state := r.stateLocked(client)
	state.failures = state.failures[:0]
	state.lockedUntil = time.Time{}
	state.lockoutLevel = 0
	state.lastClearedAt = r.now()
}

func (r *RateLimiter) stateLocked(client string) *clientState {
	s, ok := r.clients[client]
	if !ok {
		s = &clientState{}
		r.clients[client] = s
	}
	return s
}
