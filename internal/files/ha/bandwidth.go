// Package ha provides high availability and scaling for the file distribution system.
package ha

import (
	"context"
	"errors"
	"io"
	"sync"
	"sync/atomic"
	"time"
)

// ErrRateLimited is returned when a request is rate limited.
var ErrRateLimited = errors.New("rate limited")

// Priority represents the priority of a transfer.
type Priority int

const (
	PriorityLow      Priority = 0
	PriorityNormal   Priority = 1
	PriorityHigh     Priority = 2
	PriorityCritical Priority = 3
)

// BandwidthConfig configures the bandwidth manager.
type BandwidthConfig struct {
	// GlobalRateLimit is the maximum bytes per second for all transfers.
	// 0 means unlimited.
	GlobalRateLimit int64

	// PerAgentRateLimit is the maximum bytes per second per agent.
	// 0 means unlimited.
	PerAgentRateLimit int64

	// MaxConcurrentTransfers is the maximum number of concurrent transfers.
	// 0 means unlimited.
	MaxConcurrentTransfers int

	// MaxConcurrentPerAgent is the maximum concurrent transfers per agent.
	// 0 means unlimited.
	MaxConcurrentPerAgent int

	// BurstMultiplier allows bursting up to this multiple of the rate limit.
	// Default is 1.0 (no burst).
	BurstMultiplier float64
}

// DefaultBandwidthConfig returns a default bandwidth configuration.
func DefaultBandwidthConfig() *BandwidthConfig {
	return &BandwidthConfig{
		GlobalRateLimit:        0, // Unlimited
		PerAgentRateLimit:      0, // Unlimited
		MaxConcurrentTransfers: 100,
		MaxConcurrentPerAgent:  10,
		BurstMultiplier:        1.5,
	}
}

// BandwidthManager manages bandwidth for file transfers.
type BandwidthManager struct {
	config *BandwidthConfig

	// globalLimiter limits global bandwidth.
	globalLimiter *TokenBucket

	// agentLimiters limits per-agent bandwidth.
	agentLimiters map[string]*TokenBucket

	// mu protects agentLimiters.
	mu sync.RWMutex

	// activeTransfers is the total number of active transfers.
	activeTransfers int64

	// agentTransfers tracks active transfers per agent.
	agentTransfers map[string]int64

	// transfersMu protects agentTransfers.
	transfersMu sync.Mutex

	// priorityQueues contains queued transfers by priority.
	priorityQueues [4]chan *QueuedTransfer

	// stats tracks bandwidth statistics.
	stats *BandwidthStats
}

// BandwidthStats contains bandwidth statistics.
type BandwidthStats struct {
	// BytesTransferred is the total bytes transferred.
	BytesTransferred int64

	// TransfersCompleted is the total number of completed transfers.
	TransfersCompleted int64

	// TransfersQueued is the current number of queued transfers.
	TransfersQueued int64

	// TransfersActive is the current number of active transfers.
	TransfersActive int64

	// TransfersRateLimited is the total number of rate limited transfers.
	TransfersRateLimited int64

	// BytesPerSecond is the current transfer rate.
	BytesPerSecond int64

	// mu protects concurrent updates.
	mu sync.Mutex

	// lastUpdate tracks the last update time for rate calculation.
	lastUpdate time.Time

	// lastBytes tracks bytes at last update for rate calculation.
	lastBytes int64
}

// QueuedTransfer represents a transfer waiting in queue.
type QueuedTransfer struct {
	// AgentID is the agent requesting the transfer.
	AgentID string

	// Priority is the transfer priority.
	Priority Priority

	// Ready is closed when the transfer can proceed.
	Ready chan struct{}

	// Cancel is used to cancel the queued transfer.
	Cancel context.CancelFunc
}

// NewBandwidthManager creates a new bandwidth manager.
func NewBandwidthManager(config *BandwidthConfig) *BandwidthManager {
	if config == nil {
		config = DefaultBandwidthConfig()
	}
	if config.BurstMultiplier == 0 {
		config.BurstMultiplier = 1.0
	}

	bm := &BandwidthManager{
		config:         config,
		agentLimiters:  make(map[string]*TokenBucket),
		agentTransfers: make(map[string]int64),
		stats:          &BandwidthStats{lastUpdate: time.Now()},
	}

	// Initialize global limiter if configured.
	if config.GlobalRateLimit > 0 {
		burstSize := int64(float64(config.GlobalRateLimit) * config.BurstMultiplier)
		bm.globalLimiter = NewTokenBucket(config.GlobalRateLimit, burstSize)
	}

	// Initialize priority queues.
	for i := range bm.priorityQueues {
		bm.priorityQueues[i] = make(chan *QueuedTransfer, 1000)
	}

	return bm
}

// AcquireTransfer attempts to acquire a transfer slot.
// Returns nil if the transfer should proceed, or an error if it should not.
func (bm *BandwidthManager) AcquireTransfer(ctx context.Context, agentID string, priority Priority) (*TransferPermit, error) {
	// Check global concurrent transfer limit.
	if bm.config.MaxConcurrentTransfers > 0 {
		current := atomic.LoadInt64(&bm.activeTransfers)
		if current >= int64(bm.config.MaxConcurrentTransfers) {
			// Queue the transfer.
			permit, err := bm.queueTransfer(ctx, agentID, priority)
			if err != nil {
				return nil, err
			}
			return permit, nil
		}
	}

	// Check per-agent concurrent transfer limit.
	if bm.config.MaxConcurrentPerAgent > 0 {
		bm.transfersMu.Lock()
		current := bm.agentTransfers[agentID]
		if current >= int64(bm.config.MaxConcurrentPerAgent) {
			bm.transfersMu.Unlock()
			// Queue the transfer.
			permit, err := bm.queueTransfer(ctx, agentID, priority)
			if err != nil {
				return nil, err
			}
			return permit, nil
		}
		bm.agentTransfers[agentID]++
		bm.transfersMu.Unlock()
	}

	// Increment active transfers.
	atomic.AddInt64(&bm.activeTransfers, 1)
	atomic.AddInt64(&bm.stats.TransfersActive, 1)

	return &TransferPermit{
		bm:      bm,
		agentID: agentID,
	}, nil
}

// queueTransfer queues a transfer until a slot is available.
func (bm *BandwidthManager) queueTransfer(ctx context.Context, agentID string, priority Priority) (*TransferPermit, error) {
	ctx, cancel := context.WithCancel(ctx)

	qt := &QueuedTransfer{
		AgentID:  agentID,
		Priority: priority,
		Ready:    make(chan struct{}),
		Cancel:   cancel,
	}

	atomic.AddInt64(&bm.stats.TransfersQueued, 1)

	// Add to priority queue.
	select {
	case bm.priorityQueues[priority] <- qt:
	default:
		cancel()
		atomic.AddInt64(&bm.stats.TransfersQueued, -1)
		return nil, ErrRateLimited
	}

	// Wait for ready signal or cancellation.
	select {
	case <-ctx.Done():
		atomic.AddInt64(&bm.stats.TransfersQueued, -1)
		return nil, ctx.Err()
	case <-qt.Ready:
		atomic.AddInt64(&bm.stats.TransfersQueued, -1)
		atomic.AddInt64(&bm.activeTransfers, 1)
		atomic.AddInt64(&bm.stats.TransfersActive, 1)

		bm.transfersMu.Lock()
		bm.agentTransfers[agentID]++
		bm.transfersMu.Unlock()

		return &TransferPermit{
			bm:      bm,
			agentID: agentID,
		}, nil
	}
}

// ReleaseTransfer releases a transfer slot.
func (bm *BandwidthManager) ReleaseTransfer(agentID string) {
	atomic.AddInt64(&bm.activeTransfers, -1)
	atomic.AddInt64(&bm.stats.TransfersActive, -1)

	if bm.config.MaxConcurrentPerAgent > 0 {
		bm.transfersMu.Lock()
		bm.agentTransfers[agentID]--
		if bm.agentTransfers[agentID] <= 0 {
			delete(bm.agentTransfers, agentID)
		}
		bm.transfersMu.Unlock()
	}

	// Try to dequeue waiting transfers (highest priority first).
	for p := PriorityCritical; p >= PriorityLow; p-- {
		select {
		case qt := <-bm.priorityQueues[p]:
			close(qt.Ready)
			return
		default:
			continue
		}
	}
}

// AcquireBytes attempts to acquire bytes for transfer.
// This implements rate limiting.
func (bm *BandwidthManager) AcquireBytes(ctx context.Context, agentID string, bytes int64) error {
	// Check global rate limit.
	if bm.globalLimiter != nil {
		if !bm.globalLimiter.TakeMax(ctx, bytes) {
			atomic.AddInt64(&bm.stats.TransfersRateLimited, 1)
			return ErrRateLimited
		}
	}

	// Check per-agent rate limit.
	if bm.config.PerAgentRateLimit > 0 {
		limiter := bm.getAgentLimiter(agentID)
		if !limiter.TakeMax(ctx, bytes) {
			atomic.AddInt64(&bm.stats.TransfersRateLimited, 1)
			return ErrRateLimited
		}
	}

	return nil
}

// RecordTransfer records a completed transfer.
func (bm *BandwidthManager) RecordTransfer(bytes int64) {
	atomic.AddInt64(&bm.stats.BytesTransferred, bytes)
	atomic.AddInt64(&bm.stats.TransfersCompleted, 1)

	// Update rate calculation.
	bm.stats.mu.Lock()
	defer bm.stats.mu.Unlock()

	now := time.Now()
	elapsed := now.Sub(bm.stats.lastUpdate).Seconds()
	if elapsed >= 1.0 {
		currentBytes := atomic.LoadInt64(&bm.stats.BytesTransferred)
		bytesDiff := currentBytes - bm.stats.lastBytes
		bm.stats.BytesPerSecond = int64(float64(bytesDiff) / elapsed)
		bm.stats.lastUpdate = now
		bm.stats.lastBytes = currentBytes
	}
}

// GetStats returns the current bandwidth statistics.
func (bm *BandwidthManager) GetStats() *BandwidthStats {
	return bm.stats
}

// getAgentLimiter gets or creates a rate limiter for an agent.
func (bm *BandwidthManager) getAgentLimiter(agentID string) *TokenBucket {
	bm.mu.RLock()
	limiter, ok := bm.agentLimiters[agentID]
	bm.mu.RUnlock()

	if ok {
		return limiter
	}

	bm.mu.Lock()
	defer bm.mu.Unlock()

	// Double-check after acquiring write lock.
	if limiter, ok = bm.agentLimiters[agentID]; ok {
		return limiter
	}

	burstSize := int64(float64(bm.config.PerAgentRateLimit) * bm.config.BurstMultiplier)
	limiter = NewTokenBucket(bm.config.PerAgentRateLimit, burstSize)
	bm.agentLimiters[agentID] = limiter
	return limiter
}

// TransferPermit represents permission to perform a transfer.
type TransferPermit struct {
	bm      *BandwidthManager
	agentID string
}

// Release releases the transfer permit.
func (p *TransferPermit) Release() {
	p.bm.ReleaseTransfer(p.agentID)
}

// TokenBucket implements a token bucket rate limiter.
type TokenBucket struct {
	rate       int64 // tokens per second
	bucketSize int64 // maximum tokens
	tokens     int64 // current tokens (scaled by 1000 for precision)
	lastUpdate int64 // last update time (unix nano)
	mu         sync.Mutex
}

// NewTokenBucket creates a new token bucket.
func NewTokenBucket(rate, bucketSize int64) *TokenBucket {
	return &TokenBucket{
		rate:       rate,
		bucketSize: bucketSize,
		tokens:     bucketSize * 1000, // Start with full bucket (scaled)
		lastUpdate: time.Now().UnixNano(),
	}
}

// TakeMax attempts to take up to n tokens.
// Returns true if successful, false if not enough tokens.
func (tb *TokenBucket) TakeMax(ctx context.Context, n int64) bool {
	tb.mu.Lock()
	defer tb.mu.Unlock()

	// Refill tokens based on time elapsed.
	now := time.Now().UnixNano()
	elapsed := now - tb.lastUpdate
	tb.lastUpdate = now

	// Add tokens for elapsed time (rate * elapsed / 1e9 seconds, scaled by 1000).
	newTokens := tb.rate * elapsed / 1000000 // Simplified calculation
	tb.tokens += newTokens
	if tb.tokens > tb.bucketSize*1000 {
		tb.tokens = tb.bucketSize * 1000
	}

	// Check if we have enough tokens.
	needed := n * 1000 // Scale requested tokens
	if tb.tokens >= needed {
		tb.tokens -= needed
		return true
	}

	return false
}

// RateLimitedReader wraps an io.Reader with rate limiting.
type RateLimitedReader struct {
	reader  io.Reader
	bm      *BandwidthManager
	agentID string
	ctx     context.Context
}

// NewRateLimitedReader creates a new rate-limited reader.
func NewRateLimitedReader(ctx context.Context, reader io.Reader, bm *BandwidthManager, agentID string) *RateLimitedReader {
	return &RateLimitedReader{
		reader:  reader,
		bm:      bm,
		agentID: agentID,
		ctx:     ctx,
	}
}

// Read reads up to len(p) bytes with rate limiting.
func (r *RateLimitedReader) Read(p []byte) (n int, err error) {
	select {
	case <-r.ctx.Done():
		return 0, r.ctx.Err()
	default:
	}

	if r.bm != nil {
		if err := r.bm.AcquireBytes(r.ctx, r.agentID, int64(len(p))); err != nil {
			return 0, err
		}
	}

	n, err = r.reader.Read(p)

	if r.bm != nil && n > 0 {
		r.bm.RecordTransfer(int64(n))
	}

	return n, err
}

// RateLimitedWriter wraps an io.Writer with rate limiting.
type RateLimitedWriter struct {
	writer  io.Writer
	bm      *BandwidthManager
	agentID string
	ctx     context.Context
}

// NewRateLimitedWriter creates a new rate-limited writer.
func NewRateLimitedWriter(ctx context.Context, writer io.Writer, bm *BandwidthManager, agentID string) *RateLimitedWriter {
	return &RateLimitedWriter{
		writer:  writer,
		bm:      bm,
		agentID: agentID,
		ctx:     ctx,
	}
}

// Write writes len(p) bytes with rate limiting.
func (w *RateLimitedWriter) Write(p []byte) (n int, err error) {
	select {
	case <-w.ctx.Done():
		return 0, w.ctx.Err()
	default:
	}

	if w.bm != nil {
		if err := w.bm.AcquireBytes(w.ctx, w.agentID, int64(len(p))); err != nil {
			return 0, err
		}
	}

	n, err = w.writer.Write(p)

	if w.bm != nil && n > 0 {
		w.bm.RecordTransfer(int64(n))
	}

	return n, err
}
