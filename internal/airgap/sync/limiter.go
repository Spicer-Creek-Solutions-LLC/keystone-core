package sync

import (
	"context"
	"sync"
	"time"
)

// BandwidthLimiter implements a token bucket rate limiter for bandwidth control.
// A rate of 0 means unlimited.
type BandwidthLimiter struct {
	mu         sync.Mutex
	tokens     int64
	maxTokens  int64
	rate       int64
	lastRefill time.Time
}

// NewBandwidthLimiter creates a limiter that allows bytesPerSecond throughput.
// A rate of 0 means unlimited (WaitN returns immediately).
func NewBandwidthLimiter(bytesPerSecond int64) *BandwidthLimiter {
	if bytesPerSecond <= 0 {
		return &BandwidthLimiter{rate: 0}
	}
	return &BandwidthLimiter{
		tokens:     bytesPerSecond,
		maxTokens:  bytesPerSecond,
		rate:       bytesPerSecond,
		lastRefill: time.Now(),
	}
}

// WaitN blocks until n bytes worth of tokens are available, or the context is cancelled.
func (l *BandwidthLimiter) WaitN(ctx context.Context, n int) error {
	if l.rate == 0 {
		return nil
	}

	needed := int64(n)
	for {
		l.mu.Lock()
		l.refill()
		if l.tokens >= needed {
			l.tokens -= needed
			l.mu.Unlock()
			return nil
		}
		l.mu.Unlock()

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(10 * time.Millisecond):
		}
	}
}

// SetRate changes the bandwidth limit. A rate of 0 means unlimited.
func (l *BandwidthLimiter) SetRate(bytesPerSecond int64) {
	l.mu.Lock()
	defer l.mu.Unlock()

	if bytesPerSecond <= 0 {
		l.rate = 0
		return
	}
	l.rate = bytesPerSecond
	l.maxTokens = bytesPerSecond
	if l.tokens > l.maxTokens {
		l.tokens = l.maxTokens
	}
	l.lastRefill = time.Now()
}

// Rate returns the current rate limit in bytes per second.
func (l *BandwidthLimiter) Rate() int64 {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.rate
}

func (l *BandwidthLimiter) refill() {
	now := time.Now()
	elapsed := now.Sub(l.lastRefill)
	if elapsed <= 0 {
		return
	}

	add := int64(elapsed.Seconds() * float64(l.rate))
	if add <= 0 {
		return
	}

	l.tokens += add
	if l.tokens > l.maxTokens {
		l.tokens = l.maxTokens
	}
	l.lastRefill = now
}
