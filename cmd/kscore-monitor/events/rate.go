package events

import (
	"sync"
	"time"
)

// RateTracker tracks event rates using a sliding window.
type RateTracker struct {
	mu       sync.Mutex
	window   time.Duration
	buckets  []int64 // per-second counters
	cursor   int
	lastTick time.Time
}

// NewRateTracker creates a rate tracker with the given window in seconds.
func NewRateTracker(windowSeconds int) *RateTracker {
	if windowSeconds < 1 {
		windowSeconds = 1
	}
	return &RateTracker{
		window:   time.Duration(windowSeconds) * time.Second,
		buckets:  make([]int64, windowSeconds),
		lastTick: time.Now(),
	}
}

// Increment records one event.
func (r *RateTracker) Increment() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.advance()
	r.buckets[r.cursor]++
}

// Rate returns the current events per second averaged over the window.
func (r *RateTracker) Rate() float64 {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.advance()

	var total int64
	for _, b := range r.buckets {
		total += b
	}
	return float64(total) / r.window.Seconds()
}

// advance rolls the window forward, zeroing stale buckets.
func (r *RateTracker) advance() {
	now := time.Now()
	elapsed := int(now.Sub(r.lastTick).Seconds())
	if elapsed <= 0 {
		return
	}

	n := len(r.buckets)
	if elapsed >= n {
		// Clear everything
		for i := range r.buckets {
			r.buckets[i] = 0
		}
		r.cursor = 0
	} else {
		for i := 0; i < elapsed; i++ {
			r.cursor = (r.cursor + 1) % n
			r.buckets[r.cursor] = 0
		}
	}
	r.lastTick = now
}
