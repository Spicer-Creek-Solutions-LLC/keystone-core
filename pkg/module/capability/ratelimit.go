// SPDX-License-Identifier: Apache-2.0

package capability

import (
	"sync"
	"time"

	"go.keystone-core.io/keystone-core/pkg/module/manifest"
)

// tokenBucket is a minimal stdlib token bucket (no x/time/rate dep —
// it is not in go.mod). Capacity = burst = the configured count;
// refills `count` tokens every `per` window, proportionally.
type tokenBucket struct {
	mu       sync.Mutex
	capacity float64
	tokens   float64
	rate     float64 // tokens per second
	last     time.Time
	now      func() time.Time // injectable for tests
}

// newRateLimiter builds a limiter from a CapabilityConfig rate
// string ("100/s"). An empty string → nil (unlimited).
func newRateLimiter(raw string) (*tokenBucket, error) {
	if raw == "" {
		return nil, nil
	}
	n, per, err := manifest.ParseRate(raw)
	if err != nil {
		return nil, err
	}
	c := float64(n)
	return &tokenBucket{
		capacity: c,
		tokens:   c,
		rate:     c / per.Seconds(),
		last:     time.Now(),
		now:      time.Now,
	}, nil
}

// allow consumes one token, refilling first. nil limiter → always
// allowed (unlimited).
func (b *tokenBucket) allow() bool {
	if b == nil {
		return true
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	t := b.now()
	elapsed := t.Sub(b.last).Seconds()
	if elapsed > 0 {
		b.tokens += elapsed * b.rate
		if b.tokens > b.capacity {
			b.tokens = b.capacity
		}
		b.last = t
	}
	if b.tokens >= 1 {
		b.tokens--
		return true
	}
	return false
}
