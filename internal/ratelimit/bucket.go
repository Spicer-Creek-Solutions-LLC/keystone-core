package ratelimit

import (
	"fmt"
	"time"

	"golang.org/x/time/rate"
)

// Config carries operator-supplied limits.
//
// RequestsPerMinute = events the bucket allows per minute.
// 0 disables limiting (every Allow returns true). Negative is
// rejected by Validate. The package converts to events/sec for
// the underlying [rate.Limiter] (PROJECT-DETAILS §4.20 names the
// per-minute field; events/sec is what x/time/rate accepts).
//
// Burst = burst capacity. 0 → defaults to RequestsPerMinute so a
// quiet minute can absorb a brief spike of one minute's worth of
// requests. Operators who want tighter burst control set it
// explicitly.
type Config struct {
	RequestsPerMinute int
	Burst             int
}

// Validate rejects negative values; zero is allowed for either
// field per the documented semantics.
func (c Config) Validate() error {
	if c.RequestsPerMinute < 0 {
		return fmt.Errorf("ratelimit: requests_per_minute must be >= 0, got %d", c.RequestsPerMinute)
	}
	if c.Burst < 0 {
		return fmt.Errorf("ratelimit: burst must be >= 0, got %d", c.Burst)
	}
	return nil
}

// effectiveBurst returns the burst capacity to apply, defaulting
// to RequestsPerMinute when Burst is unset.
func (c Config) effectiveBurst() int {
	if c.Burst > 0 {
		return c.Burst
	}
	return c.RequestsPerMinute
}

// Bucket is one token bucket. Construct with [New]; safe for
// concurrent use.
type Bucket struct {
	cfg     Config
	limiter *rate.Limiter // nil when RPM <= 0 (passthrough)
}

// New returns a Bucket configured from cfg. A cfg with
// RequestsPerMinute == 0 produces a passthrough bucket — every
// Allow returns true and RetryAfter returns 0. The constructor
// does not return an error; callers should call cfg.Validate
// upstream if they want to reject negatives early.
func New(cfg Config) *Bucket {
	if cfg.RequestsPerMinute <= 0 {
		return &Bucket{cfg: cfg, limiter: nil}
	}
	// rate.Limit is events/sec; RPM ÷ 60 = events/sec.
	eventsPerSec := rate.Limit(float64(cfg.RequestsPerMinute) / 60.0)
	return &Bucket{
		cfg:     cfg,
		limiter: rate.NewLimiter(eventsPerSec, cfg.effectiveBurst()),
	}
}

// Allow consumes one token if available. Returns true if the
// request is permitted, false if the bucket is exhausted.
func (b *Bucket) Allow() bool {
	return b.AllowN(time.Now(), 1)
}

// AllowN consumes n tokens at time t. The explicit-time form is
// the test seam: tests inject deterministic times instead of
// sleeping. Passthrough buckets (RPM <= 0) always return true.
func (b *Bucket) AllowN(t time.Time, n int) bool {
	if b.limiter == nil {
		return true
	}
	return b.limiter.AllowN(t, n)
}

// RetryAfter returns the duration until the next token would be
// available. Returns 0 for passthrough buckets and for buckets
// with at least one token available right now. Used by the
// middleware to populate the HTTP Retry-After header (Task 19).
//
// Implementation note: an earlier draft used Reserve + Cancel for
// this lookup, but rate.Reservation.Cancel is a no-op when even
// nanoseconds have elapsed between Reserve and Cancel (it treats
// late cancels as "already happened") — that path silently
// consumed a token. The current implementation is purely
// observational: it reads Tokens() and divides by the configured
// rate.
func (b *Bucket) RetryAfter() time.Duration {
	if b.limiter == nil {
		return 0
	}
	tokens := b.limiter.Tokens()
	if tokens >= 1 {
		return 0
	}
	shortage := 1 - tokens
	// rate.Limit is events/sec. duration = shortage / rate.
	r := float64(b.limiter.Limit())
	if r <= 0 {
		// Defensive: a passthrough limiter shouldn't reach here
		// (b.limiter==nil short-circuits above), but if a future
		// edit creates a zero-rate limiter we don't divide by zero.
		return 0
	}
	return time.Duration(shortage / r * float64(time.Second))
}

// AllowOrRetryAfter is the combined non-blocking primitive the
// middleware uses: returns (true, 0) when the request is allowed
// and (false, delay) when it is rejected. delay is the duration
// until the next token will be available — the value to put in
// the HTTP Retry-After header.
func (b *Bucket) AllowOrRetryAfter() (bool, time.Duration) {
	if b.Allow() {
		return true, 0
	}
	return false, b.RetryAfter()
}
