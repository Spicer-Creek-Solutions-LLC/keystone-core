// Package transfer provides bandwidth throttling for file transfers.
package transfer

import (
	"context"
	"errors"
	"io"
	"sync"
	"sync/atomic"
	"time"
)

// Common errors.
var (
	ErrThrottlerStopped = errors.New("throttler is stopped")
	ErrRateLimitExceeded = errors.New("rate limit exceeded")
)

// Config configures bandwidth throttling.
type Config struct {
	// BytesPerSecond is the maximum transfer rate in bytes per second.
	BytesPerSecond int64 `json:"bytesPerSecond"`
	// BurstSize is the maximum burst size in bytes.
	BurstSize int64 `json:"burstSize"`
	// MinChunkSize is the minimum chunk size for reads/writes.
	MinChunkSize int `json:"minChunkSize"`
	// MaxChunkSize is the maximum chunk size for reads/writes.
	MaxChunkSize int `json:"maxChunkSize"`
}

// DefaultConfig returns a default configuration.
func DefaultConfig() *Config {
	return &Config{
		BytesPerSecond: 10 * 1024 * 1024, // 10 MB/s
		BurstSize:      1 * 1024 * 1024,  // 1 MB burst
		MinChunkSize:   4 * 1024,         // 4 KB
		MaxChunkSize:   64 * 1024,        // 64 KB
	}
}

// Throttler controls bandwidth usage.
type Throttler struct {
	config     *Config
	tokens     int64 // atomic, available tokens (bytes)
	lastRefill time.Time
	mu         sync.Mutex
	stopCh     chan struct{}
	running    bool
}

// NewThrottler creates a new throttler.
func NewThrottler(config *Config) *Throttler {
	if config == nil {
		config = DefaultConfig()
	}
	if config.BytesPerSecond <= 0 {
		config.BytesPerSecond = 10 * 1024 * 1024
	}
	if config.BurstSize <= 0 {
		config.BurstSize = config.BytesPerSecond / 10
	}
	if config.MinChunkSize <= 0 {
		config.MinChunkSize = 4 * 1024
	}
	if config.MaxChunkSize <= 0 {
		config.MaxChunkSize = 64 * 1024
	}

	t := &Throttler{
		config:     config,
		tokens:     config.BurstSize,
		lastRefill: time.Now(),
		stopCh:     make(chan struct{}),
		running:    true,
	}

	go t.refillLoop()

	return t
}

// Stop stops the throttler.
func (t *Throttler) Stop() {
	t.mu.Lock()
	if !t.running {
		t.mu.Unlock()
		return
	}
	t.running = false
	close(t.stopCh)
	t.mu.Unlock()
}

// WaitN waits until n bytes can be transferred.
func (t *Throttler) WaitN(ctx context.Context, n int64) error {
	for {
		// Try to acquire tokens
		available := atomic.LoadInt64(&t.tokens)
		if available >= n {
			if atomic.CompareAndSwapInt64(&t.tokens, available, available-n) {
				return nil
			}
			continue
		}

		// Wait for tokens to become available
		waitTime := time.Duration(float64(n-available) / float64(t.config.BytesPerSecond) * float64(time.Second))
		if waitTime < time.Millisecond {
			waitTime = time.Millisecond
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-t.stopCh:
			return ErrThrottlerStopped
		case <-time.After(waitTime):
			// Retry
		}
	}
}

// TryAcquire tries to acquire n bytes without waiting.
func (t *Throttler) TryAcquire(n int64) bool {
	for {
		available := atomic.LoadInt64(&t.tokens)
		if available < n {
			return false
		}
		if atomic.CompareAndSwapInt64(&t.tokens, available, available-n) {
			return true
		}
	}
}

// SetRate updates the rate limit.
func (t *Throttler) SetRate(bytesPerSecond int64) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.config.BytesPerSecond = bytesPerSecond
}

// SetBurstSize updates the burst size.
func (t *Throttler) SetBurstSize(burstSize int64) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.config.BurstSize = burstSize
}

// AvailableTokens returns the current available tokens.
func (t *Throttler) AvailableTokens() int64 {
	return atomic.LoadInt64(&t.tokens)
}

func (t *Throttler) refillLoop() {
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-t.stopCh:
			return
		case <-ticker.C:
			t.refill()
		}
	}
}

func (t *Throttler) refill() {
	t.mu.Lock()
	now := time.Now()
	elapsed := now.Sub(t.lastRefill).Seconds()
	t.lastRefill = now
	bytesPerSecond := t.config.BytesPerSecond
	burstSize := t.config.BurstSize
	t.mu.Unlock()

	tokensToAdd := int64(float64(bytesPerSecond) * elapsed)
	if tokensToAdd <= 0 {
		return
	}

	for {
		current := atomic.LoadInt64(&t.tokens)
		newTokens := current + tokensToAdd
		if newTokens > burstSize {
			newTokens = burstSize
		}
		if atomic.CompareAndSwapInt64(&t.tokens, current, newTokens) {
			break
		}
	}
}

// ThrottledReader wraps an io.Reader with bandwidth throttling.
type ThrottledReader struct {
	reader    io.Reader
	throttler *Throttler
	ctx       context.Context
	stats     *TransferStats
}

// TransferStats contains transfer statistics.
type TransferStats struct {
	BytesTransferred int64     `json:"bytesTransferred"`
	StartTime        time.Time `json:"startTime"`
	LastActivity     time.Time `json:"lastActivity"`
	mu               sync.RWMutex
}

// BytesPerSecond returns the current transfer rate.
func (s *TransferStats) BytesPerSecond() float64 {
	s.mu.RLock()
	defer s.mu.RUnlock()

	elapsed := time.Since(s.StartTime).Seconds()
	if elapsed <= 0 {
		return 0
	}
	return float64(s.BytesTransferred) / elapsed
}

// NewThrottledReader creates a new throttled reader.
func NewThrottledReader(ctx context.Context, reader io.Reader, throttler *Throttler) *ThrottledReader {
	return &ThrottledReader{
		reader:    reader,
		throttler: throttler,
		ctx:       ctx,
		stats: &TransferStats{
			StartTime: time.Now(),
		},
	}
}

// Read reads data with bandwidth throttling.
func (r *ThrottledReader) Read(p []byte) (n int, err error) {
	// Limit read size
	readSize := len(p)
	if readSize > r.throttler.config.MaxChunkSize {
		readSize = r.throttler.config.MaxChunkSize
	}
	if readSize < r.throttler.config.MinChunkSize && len(p) >= r.throttler.config.MinChunkSize {
		readSize = r.throttler.config.MinChunkSize
	}

	// Wait for bandwidth
	if err := r.throttler.WaitN(r.ctx, int64(readSize)); err != nil {
		return 0, err
	}

	// Read data
	n, err = r.reader.Read(p[:readSize])
	if n > 0 {
		r.stats.mu.Lock()
		r.stats.BytesTransferred += int64(n)
		r.stats.LastActivity = time.Now()
		r.stats.mu.Unlock()
	}

	return n, err
}

// Stats returns the transfer statistics.
func (r *ThrottledReader) Stats() TransferStats {
	r.stats.mu.RLock()
	defer r.stats.mu.RUnlock()
	return *r.stats
}

// ThrottledWriter wraps an io.Writer with bandwidth throttling.
type ThrottledWriter struct {
	writer    io.Writer
	throttler *Throttler
	ctx       context.Context
	stats     *TransferStats
}

// NewThrottledWriter creates a new throttled writer.
func NewThrottledWriter(ctx context.Context, writer io.Writer, throttler *Throttler) *ThrottledWriter {
	return &ThrottledWriter{
		writer:    writer,
		throttler: throttler,
		ctx:       ctx,
		stats: &TransferStats{
			StartTime: time.Now(),
		},
	}
}

// Write writes data with bandwidth throttling.
func (w *ThrottledWriter) Write(p []byte) (n int, err error) {
	total := 0
	remaining := p

	for len(remaining) > 0 {
		// Calculate chunk size
		chunkSize := len(remaining)
		if chunkSize > w.throttler.config.MaxChunkSize {
			chunkSize = w.throttler.config.MaxChunkSize
		}

		// Wait for bandwidth
		if err := w.throttler.WaitN(w.ctx, int64(chunkSize)); err != nil {
			return total, err
		}

		// Write chunk
		n, err := w.writer.Write(remaining[:chunkSize])
		total += n

		w.stats.mu.Lock()
		w.stats.BytesTransferred += int64(n)
		w.stats.LastActivity = time.Now()
		w.stats.mu.Unlock()

		if err != nil {
			return total, err
		}

		remaining = remaining[n:]
	}

	return total, nil
}

// Stats returns the transfer statistics.
func (w *ThrottledWriter) Stats() TransferStats {
	w.stats.mu.RLock()
	defer w.stats.mu.RUnlock()
	return *w.stats
}

// BandwidthPool manages shared bandwidth across multiple transfers.
type BandwidthPool struct {
	config      *Config
	throttler   *Throttler
	transfers   map[string]*pooledTransfer
	mu          sync.RWMutex
	totalBytes  int64 // atomic
}

type pooledTransfer struct {
	id       string
	priority int
	bytes    int64 // atomic
}

// NewBandwidthPool creates a new bandwidth pool.
func NewBandwidthPool(config *Config) *BandwidthPool {
	return &BandwidthPool{
		config:    config,
		throttler: NewThrottler(config),
		transfers: make(map[string]*pooledTransfer),
	}
}

// Stop stops the bandwidth pool.
func (bp *BandwidthPool) Stop() {
	bp.throttler.Stop()
}

// RegisterTransfer registers a transfer with the pool.
func (bp *BandwidthPool) RegisterTransfer(id string, priority int) {
	bp.mu.Lock()
	defer bp.mu.Unlock()

	bp.transfers[id] = &pooledTransfer{
		id:       id,
		priority: priority,
	}
}

// UnregisterTransfer removes a transfer from the pool.
func (bp *BandwidthPool) UnregisterTransfer(id string) {
	bp.mu.Lock()
	defer bp.mu.Unlock()
	delete(bp.transfers, id)
}

// GetReader returns a throttled reader for a transfer.
func (bp *BandwidthPool) GetReader(ctx context.Context, id string, reader io.Reader) *ThrottledReader {
	return NewThrottledReader(ctx, reader, bp.throttler)
}

// GetWriter returns a throttled writer for a transfer.
func (bp *BandwidthPool) GetWriter(ctx context.Context, id string, writer io.Writer) *ThrottledWriter {
	return NewThrottledWriter(ctx, writer, bp.throttler)
}

// Stats returns pool statistics.
func (bp *BandwidthPool) Stats() PoolStats {
	bp.mu.RLock()
	defer bp.mu.RUnlock()

	return PoolStats{
		ActiveTransfers:  len(bp.transfers),
		TotalBytes:       atomic.LoadInt64(&bp.totalBytes),
		AvailableTokens:  bp.throttler.AvailableTokens(),
		BytesPerSecond:   bp.config.BytesPerSecond,
	}
}

// PoolStats contains pool statistics.
type PoolStats struct {
	ActiveTransfers int   `json:"activeTransfers"`
	TotalBytes      int64 `json:"totalBytes"`
	AvailableTokens int64 `json:"availableTokens"`
	BytesPerSecond  int64 `json:"bytesPerSecond"`
}

// AdaptiveThrottler adjusts throttling based on network conditions.
type AdaptiveThrottler struct {
	throttler      *Throttler
	minRate        int64
	maxRate        int64
	currentRate    int64 // atomic
	lastAdjust     time.Time
	adjustInterval time.Duration
	mu             sync.RWMutex
	metrics        *AdaptiveMetrics
}

// AdaptiveMetrics contains metrics for adaptive throttling.
type AdaptiveMetrics struct {
	Latency       time.Duration `json:"latency"`
	PacketLoss    float64       `json:"packetLoss"`
	Throughput    int64         `json:"throughput"`
	Retries       int64         `json:"retries"`
	mu            sync.RWMutex
}

// NewAdaptiveThrottler creates a new adaptive throttler.
func NewAdaptiveThrottler(minRate, maxRate int64) *AdaptiveThrottler {
	config := DefaultConfig()
	config.BytesPerSecond = maxRate

	at := &AdaptiveThrottler{
		throttler:      NewThrottler(config),
		minRate:        minRate,
		maxRate:        maxRate,
		currentRate:    maxRate,
		adjustInterval: time.Second,
		metrics:        &AdaptiveMetrics{},
	}

	go at.adjustLoop()

	return at
}

// Stop stops the adaptive throttler.
func (at *AdaptiveThrottler) Stop() {
	at.throttler.Stop()
}

// Reader returns a throttled reader.
func (at *AdaptiveThrottler) Reader(ctx context.Context, r io.Reader) *ThrottledReader {
	return NewThrottledReader(ctx, r, at.throttler)
}

// Writer returns a throttled writer.
func (at *AdaptiveThrottler) Writer(ctx context.Context, w io.Writer) *ThrottledWriter {
	return NewThrottledWriter(ctx, w, at.throttler)
}

// RecordLatency records a latency measurement.
func (at *AdaptiveThrottler) RecordLatency(d time.Duration) {
	at.metrics.mu.Lock()
	at.metrics.Latency = d
	at.metrics.mu.Unlock()
}

// RecordPacketLoss records packet loss.
func (at *AdaptiveThrottler) RecordPacketLoss(loss float64) {
	at.metrics.mu.Lock()
	at.metrics.PacketLoss = loss
	at.metrics.mu.Unlock()
}

// RecordRetry records a retry.
func (at *AdaptiveThrottler) RecordRetry() {
	at.metrics.mu.Lock()
	at.metrics.Retries++
	at.metrics.mu.Unlock()
}

// CurrentRate returns the current rate.
func (at *AdaptiveThrottler) CurrentRate() int64 {
	return atomic.LoadInt64(&at.currentRate)
}

func (at *AdaptiveThrottler) adjustLoop() {
	ticker := time.NewTicker(at.adjustInterval)
	defer ticker.Stop()

	for {
		select {
		case <-at.throttler.stopCh:
			return
		case <-ticker.C:
			at.adjust()
		}
	}
}

func (at *AdaptiveThrottler) adjust() {
	at.metrics.mu.RLock()
	latency := at.metrics.Latency
	packetLoss := at.metrics.PacketLoss
	retries := at.metrics.Retries
	at.metrics.mu.RUnlock()

	currentRate := atomic.LoadInt64(&at.currentRate)
	newRate := currentRate

	// Decrease rate if conditions are poor
	if latency > 500*time.Millisecond || packetLoss > 0.05 || retries > 5 {
		newRate = int64(float64(currentRate) * 0.8) // Reduce by 20%
	} else if latency < 100*time.Millisecond && packetLoss < 0.01 && retries == 0 {
		// Increase rate if conditions are good
		newRate = int64(float64(currentRate) * 1.1) // Increase by 10%
	}

	// Clamp to bounds
	if newRate < at.minRate {
		newRate = at.minRate
	}
	if newRate > at.maxRate {
		newRate = at.maxRate
	}

	if newRate != currentRate {
		atomic.StoreInt64(&at.currentRate, newRate)
		at.throttler.SetRate(newRate)
	}

	// Reset metrics
	at.metrics.mu.Lock()
	at.metrics.Retries = 0
	at.metrics.mu.Unlock()
}

// LimitedCopy copies from src to dst with a size limit.
func LimitedCopy(ctx context.Context, dst io.Writer, src io.Reader, limit int64, throttler *Throttler) (int64, error) {
	limited := io.LimitReader(src, limit)

	var n int64
	buf := make([]byte, throttler.config.MaxChunkSize)

	for {
		select {
		case <-ctx.Done():
			return n, ctx.Err()
		default:
		}

		// Wait for bandwidth
		if err := throttler.WaitN(ctx, int64(len(buf))); err != nil {
			return n, err
		}

		nr, rerr := limited.Read(buf)
		if nr > 0 {
			nw, werr := dst.Write(buf[:nr])
			n += int64(nw)
			if werr != nil {
				return n, werr
			}
			if nr != nw {
				return n, io.ErrShortWrite
			}
		}
		if rerr != nil {
			if rerr == io.EOF {
				return n, nil
			}
			return n, rerr
		}
	}
}

// ProgressReader wraps a reader with progress reporting.
type ProgressReader struct {
	reader   io.Reader
	total    int64
	current  int64 // atomic
	callback func(current, total int64)
}

// NewProgressReader creates a new progress reader.
func NewProgressReader(reader io.Reader, total int64, callback func(current, total int64)) *ProgressReader {
	return &ProgressReader{
		reader:   reader,
		total:    total,
		callback: callback,
	}
}

// Read reads data and reports progress.
func (r *ProgressReader) Read(p []byte) (n int, err error) {
	n, err = r.reader.Read(p)
	if n > 0 {
		current := atomic.AddInt64(&r.current, int64(n))
		if r.callback != nil {
			r.callback(current, r.total)
		}
	}
	return n, err
}

// Progress returns the current progress.
func (r *ProgressReader) Progress() (current, total int64) {
	return atomic.LoadInt64(&r.current), r.total
}
