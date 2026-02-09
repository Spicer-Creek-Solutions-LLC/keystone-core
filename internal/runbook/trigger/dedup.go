package trigger

import (
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/shawnbutts/keystone-core/internal/events"
)

// Deduplicator tracks seen events to prevent duplicate processing.
type Deduplicator struct {
	mu      sync.RWMutex
	entries map[string]*dedupEntry
	done    chan struct{}
}

// dedupEntry tracks a single dedup key.
type dedupEntry struct {
	key       string
	timestamp time.Time
}

// NewDeduplicator creates a new deduplicator.
func NewDeduplicator() *Deduplicator {
	d := &Deduplicator{
		entries: make(map[string]*dedupEntry),
		done:    make(chan struct{}),
	}

	// Start cleanup goroutine
	go d.cleanupLoop()

	return d
}

// cleanupLoop periodically removes expired entries.
func (d *Deduplicator) cleanupLoop() {
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			d.cleanup()
		case <-d.done:
			return
		}
	}
}

// cleanup removes entries older than 1 hour.
func (d *Deduplicator) cleanup() {
	d.mu.Lock()
	defer d.mu.Unlock()

	cutoff := time.Now().Add(-time.Hour)
	for k, entry := range d.entries {
		if entry.timestamp.Before(cutoff) {
			delete(d.entries, k)
		}
	}
}

// GenerateKey generates a dedup key from a template and event.
func (d *Deduplicator) GenerateKey(template string, event *events.Event) string {
	result := template

	// Replace event fields
	result = strings.ReplaceAll(result, "{{ .ID }}", event.ID)
	result = strings.ReplaceAll(result, "{{ .Type }}", string(event.Type))
	result = strings.ReplaceAll(result, "{{ .Source }}", event.Source)
	result = strings.ReplaceAll(result, "{{ .Severity }}", string(event.Severity))

	// Replace tags
	for k, v := range event.Tags {
		placeholder := fmt.Sprintf("{{ .Tags.%s }}", k)
		result = strings.ReplaceAll(result, placeholder, v)
	}

	// Replace data fields (simple string values only)
	for k, v := range event.Data {
		placeholder := fmt.Sprintf("{{ .Data.%s }}", k)
		if s, ok := v.(string); ok {
			result = strings.ReplaceAll(result, placeholder, s)
		} else {
			result = strings.ReplaceAll(result, placeholder, fmt.Sprintf("%v", v))
		}
	}

	return result
}

// IsDuplicate checks if an event has been seen within the window.
func (d *Deduplicator) IsDuplicate(triggerID, key string, window time.Duration) bool {
	fullKey := triggerID + ":" + key

	d.mu.RLock()
	entry, exists := d.entries[fullKey]
	d.mu.RUnlock()

	if !exists {
		return false
	}

	return time.Since(entry.timestamp) < window
}

// Record records an event as seen.
func (d *Deduplicator) Record(triggerID, key string) {
	fullKey := triggerID + ":" + key

	d.mu.Lock()
	d.entries[fullKey] = &dedupEntry{
		key:       key,
		timestamp: time.Now(),
	}
	d.mu.Unlock()
}

// Clear removes all entries for a trigger.
func (d *Deduplicator) Clear(triggerID string) {
	prefix := triggerID + ":"

	d.mu.Lock()
	defer d.mu.Unlock()

	for k := range d.entries {
		if strings.HasPrefix(k, prefix) {
			delete(d.entries, k)
		}
	}
}

// Close shuts down the deduplicator.
func (d *Deduplicator) Close() {
	close(d.done)
}

// RateLimiter implements token bucket rate limiting per trigger.
type RateLimiter struct {
	mu      sync.RWMutex
	buckets map[string]*rateBucket
}

// rateBucket tracks rate limit state for a trigger.
type rateBucket struct {
	mu          sync.Mutex
	tokens      int
	maxTokens   int
	window      time.Duration
	lastRefill  time.Time
	count       int64 // total requests within current window
	windowStart time.Time
}

// NewRateLimiter creates a new rate limiter.
func NewRateLimiter() *RateLimiter {
	return &RateLimiter{
		buckets: make(map[string]*rateBucket),
	}
}

// Allow checks if an execution is allowed within rate limits.
func (r *RateLimiter) Allow(triggerID string, maxExecutions int, window time.Duration) bool {
	r.mu.Lock()
	bucket, exists := r.buckets[triggerID]
	if !exists {
		bucket = &rateBucket{
			tokens:      maxExecutions,
			maxTokens:   maxExecutions,
			window:      window,
			lastRefill:  time.Now(),
			windowStart: time.Now(),
		}
		r.buckets[triggerID] = bucket
	}
	r.mu.Unlock()

	return bucket.allow(maxExecutions, window)
}

// allow checks and consumes a token from the bucket.
func (b *rateBucket) allow(maxExecutions int, window time.Duration) bool {
	b.mu.Lock()
	defer b.mu.Unlock()

	now := time.Now()

	// Check if we need to reset the window
	if now.Sub(b.windowStart) >= window {
		b.count = 0
		b.windowStart = now
		b.tokens = maxExecutions
	}

	// Refill tokens based on time elapsed
	elapsed := now.Sub(b.lastRefill)
	if elapsed >= window {
		b.tokens = maxExecutions
		b.lastRefill = now
	} else {
		// Partial refill
		refillRate := float64(maxExecutions) / float64(window)
		tokensToAdd := int(float64(elapsed) * refillRate)
		if tokensToAdd > 0 {
			b.tokens += tokensToAdd
			if b.tokens > maxExecutions {
				b.tokens = maxExecutions
			}
			b.lastRefill = now
		}
	}

	// Check if allowed
	if b.tokens <= 0 {
		return false
	}

	b.tokens--
	b.count++
	return true
}

// GetStats returns rate limit statistics for a trigger.
func (r *RateLimiter) GetStats(triggerID string) (remaining int, total int64) {
	r.mu.RLock()
	bucket, exists := r.buckets[triggerID]
	r.mu.RUnlock()

	if !exists {
		return 0, 0
	}

	bucket.mu.Lock()
	defer bucket.mu.Unlock()

	return bucket.tokens, bucket.count
}

// Reset resets the rate limiter for a trigger.
func (r *RateLimiter) Reset(triggerID string) {
	r.mu.Lock()
	delete(r.buckets, triggerID)
	r.mu.Unlock()
}
