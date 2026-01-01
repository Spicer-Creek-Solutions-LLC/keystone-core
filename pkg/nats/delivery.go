package nats

import (
	"context"
	"errors"
	"fmt"
	"math"
	"sync"
	"sync/atomic"
	"time"

	"github.com/nats-io/nats.go"
)

// ============================================================================
// Delivery Guarantees - T8.2
// ============================================================================

// DeliveryMode defines the message delivery guarantee level
type DeliveryMode string

const (
	// DeliveryModeAtMostOnce provides no delivery guarantee (fire-and-forget)
	DeliveryModeAtMostOnce DeliveryMode = "at_most_once"
	// DeliveryModeAtLeastOnce guarantees message is delivered at least once
	DeliveryModeAtLeastOnce DeliveryMode = "at_least_once"
	// DeliveryModeExactlyOnce guarantees message is delivered exactly once (requires JetStream)
	DeliveryModeExactlyOnce DeliveryMode = "exactly_once"
)

// DeliveryStatus represents the status of a message delivery
type DeliveryStatus string

const (
	// DeliveryStatusPending means delivery is in progress
	DeliveryStatusPending DeliveryStatus = "pending"
	// DeliveryStatusAcked means delivery was acknowledged
	DeliveryStatusAcked DeliveryStatus = "acked"
	// DeliveryStatusNacked means delivery was negatively acknowledged
	DeliveryStatusNacked DeliveryStatus = "nacked"
	// DeliveryStatusTimeout means delivery timed out
	DeliveryStatusTimeout DeliveryStatus = "timeout"
	// DeliveryStatusFailed means delivery permanently failed
	DeliveryStatusFailed DeliveryStatus = "failed"
	// DeliveryStatusDeadLettered means message was sent to DLQ
	DeliveryStatusDeadLettered DeliveryStatus = "dead_lettered"
)

// DeliveryConfig holds delivery configuration
type DeliveryConfig struct {
	// Mode is the delivery guarantee level
	Mode DeliveryMode

	// Timeout is the acknowledgment timeout
	Timeout time.Duration

	// MaxRetries is the maximum retry attempts
	MaxRetries int

	// RetryBackoff is the initial retry backoff
	RetryBackoff time.Duration

	// MaxRetryBackoff is the maximum retry backoff
	MaxRetryBackoff time.Duration

	// BackoffMultiplier is the backoff multiplier
	BackoffMultiplier float64

	// DeadLetterSubject is the DLQ subject (empty = no DLQ)
	DeadLetterSubject string

	// TrackAcks enables acknowledgment tracking
	TrackAcks bool

	// AckWindow is the dedup window for ack tracking
	AckWindow time.Duration
}

// DefaultDeliveryConfig returns sensible defaults
func DefaultDeliveryConfig() *DeliveryConfig {
	return &DeliveryConfig{
		Mode:              DeliveryModeAtLeastOnce,
		Timeout:           30 * time.Second,
		MaxRetries:        5,
		RetryBackoff:      time.Second,
		MaxRetryBackoff:   time.Minute,
		BackoffMultiplier: 2.0,
		TrackAcks:         true,
		AckWindow:         5 * time.Minute,
	}
}

// Validate validates the configuration
func (c *DeliveryConfig) Validate() error {
	if c.Timeout < 0 {
		return errors.New("timeout must be non-negative")
	}
	if c.MaxRetries < 0 {
		return errors.New("max retries must be non-negative")
	}
	if c.RetryBackoff < 0 {
		return errors.New("retry backoff must be non-negative")
	}
	if c.BackoffMultiplier < 1.0 {
		return errors.New("backoff multiplier must be at least 1.0")
	}
	return nil
}

// DeliveryRecord tracks a message delivery attempt
type DeliveryRecord struct {
	// MessageID is the unique message identifier
	MessageID string

	// Subject is the target subject
	Subject string

	// Status is the current delivery status
	Status DeliveryStatus

	// Attempts is the number of delivery attempts
	Attempts int

	// FirstAttempt is when delivery was first attempted
	FirstAttempt time.Time

	// LastAttempt is when delivery was last attempted
	LastAttempt time.Time

	// AckedAt is when the message was acknowledged
	AckedAt time.Time

	// Error is the last error (if any)
	Error string

	// Latency is the delivery latency (first attempt to ack)
	Latency time.Duration
}

// DeliveryStats holds delivery statistics
type DeliveryStats struct {
	// TotalSent is total messages sent
	TotalSent int64

	// TotalAcked is total messages acknowledged
	TotalAcked int64

	// TotalNacked is total messages negatively acknowledged
	TotalNacked int64

	// TotalTimeout is total messages timed out
	TotalTimeout int64

	// TotalFailed is total messages permanently failed
	TotalFailed int64

	// TotalDeadLettered is total messages sent to DLQ
	TotalDeadLettered int64

	// TotalRetries is total retry attempts
	TotalRetries int64

	// PendingCount is current pending deliveries
	PendingCount int64

	// AverageLatency is average delivery latency
	AverageLatency time.Duration

	// P95Latency is 95th percentile latency
	P95Latency time.Duration

	// P99Latency is 99th percentile latency
	P99Latency time.Duration
}

// DeliveryManager manages message delivery with guarantees
type DeliveryManager struct {
	config *DeliveryConfig

	// NATS connection
	conn *nats.Conn
	js   nats.JetStreamContext

	// Message buffer for at-least-once delivery
	buffer *MessageBuffer

	// Pending deliveries
	pending   map[string]*DeliveryRecord
	pendingMu sync.RWMutex

	// Statistics
	stats     DeliveryStats
	latencies []time.Duration
	statsMu   sync.RWMutex

	// Callbacks
	onDeliveryComplete func(record *DeliveryRecord)
	onDeadLetter       func(msg *BufferedMessage, err error)

	// Lifecycle
	running atomic.Bool
	ctx     context.Context
	cancel  context.CancelFunc
	wg      sync.WaitGroup
}

// NewDeliveryManager creates a new delivery manager
func NewDeliveryManager(config *DeliveryConfig, conn *nats.Conn) (*DeliveryManager, error) {
	if config == nil {
		config = DefaultDeliveryConfig()
	}
	if err := config.Validate(); err != nil {
		return nil, fmt.Errorf("invalid config: %w", err)
	}

	ctx, cancel := context.WithCancel(context.Background())

	dm := &DeliveryManager{
		config:  config,
		conn:    conn,
		pending: make(map[string]*DeliveryRecord),
		ctx:     ctx,
		cancel:  cancel,
	}

	// Setup JetStream for exactly-once
	if conn != nil && config.Mode == DeliveryModeExactlyOnce {
		js, err := conn.JetStream()
		if err != nil {
			cancel()
			return nil, fmt.Errorf("JetStream setup failed: %w", err)
		}
		dm.js = js
	}

	// Setup buffer for at-least-once
	if config.Mode == DeliveryModeAtLeastOnce || config.Mode == DeliveryModeExactlyOnce {
		bufConfig := DefaultBufferConfig()
		buf, err := NewMessageBuffer(bufConfig)
		if err != nil {
			cancel()
			return nil, fmt.Errorf("buffer setup failed: %w", err)
		}
		dm.buffer = buf
	}

	return dm, nil
}

// Start starts the delivery manager
func (dm *DeliveryManager) Start() error {
	if dm.running.Load() {
		return errors.New("already running")
	}
	dm.running.Store(true)

	if dm.buffer != nil {
		if err := dm.buffer.Start(dm.ctx); err != nil {
			return err
		}
	}

	// Start retry loop
	dm.wg.Add(1)
	go dm.retryLoop()

	// Start ack cleanup loop
	if dm.config.TrackAcks {
		dm.wg.Add(1)
		go dm.cleanupLoop()
	}

	return nil
}

// Stop stops the delivery manager
func (dm *DeliveryManager) Stop() error {
	if !dm.running.Load() {
		return nil
	}

	dm.cancel()
	dm.wg.Wait()

	if dm.buffer != nil {
		dm.buffer.Stop()
	}

	dm.running.Store(false)
	return nil
}

// Publish publishes a message with delivery guarantees
func (dm *DeliveryManager) Publish(subject string, data []byte) error {
	return dm.PublishWithID(subject, data, generateDeliveryMessageID())
}

// PublishWithID publishes a message with a specific ID
func (dm *DeliveryManager) PublishWithID(subject string, data []byte, msgID string) error {
	switch dm.config.Mode {
	case DeliveryModeAtMostOnce:
		return dm.publishAtMostOnce(subject, data)

	case DeliveryModeAtLeastOnce:
		return dm.publishAtLeastOnce(subject, data, msgID)

	case DeliveryModeExactlyOnce:
		return dm.publishExactlyOnce(subject, data, msgID)

	default:
		return fmt.Errorf("unsupported delivery mode: %s", dm.config.Mode)
	}
}

func (dm *DeliveryManager) publishAtMostOnce(subject string, data []byte) error {
	if dm.conn == nil {
		return errors.New("not connected")
	}

	err := dm.conn.Publish(subject, data)
	if err == nil {
		atomic.AddInt64(&dm.stats.TotalSent, 1)
		atomic.AddInt64(&dm.stats.TotalAcked, 1) // Assume delivered
	}
	return err
}

func (dm *DeliveryManager) publishAtLeastOnce(subject string, data []byte, msgID string) error {
	msg := &BufferedMessage{
		ID:        msgID,
		Subject:   subject,
		Data:      data,
		Timestamp: time.Now(),
	}

	// Track delivery
	record := &DeliveryRecord{
		MessageID:    msgID,
		Subject:      subject,
		Status:       DeliveryStatusPending,
		FirstAttempt: time.Now(),
	}

	dm.pendingMu.Lock()
	dm.pending[msgID] = record
	dm.pendingMu.Unlock()
	atomic.AddInt64(&dm.stats.PendingCount, 1)

	// Try immediate delivery
	if err := dm.attemptDelivery(msg); err != nil {
		// Buffer for retry
		if dm.buffer != nil {
			return dm.buffer.Enqueue(msg)
		}
		return err
	}

	return nil
}

func (dm *DeliveryManager) publishExactlyOnce(subject string, data []byte, msgID string) error {
	if dm.js == nil {
		return errors.New("JetStream not available")
	}

	// Use JetStream publish with dedup
	opts := []nats.PubOpt{
		nats.MsgId(msgID),
	}

	_, err := dm.js.Publish(subject, data, opts...)
	if err == nil {
		atomic.AddInt64(&dm.stats.TotalSent, 1)
		atomic.AddInt64(&dm.stats.TotalAcked, 1)
	}
	return err
}

func (dm *DeliveryManager) attemptDelivery(msg *BufferedMessage) error {
	if dm.conn == nil {
		return errors.New("not connected")
	}

	// Update record
	dm.pendingMu.Lock()
	record, exists := dm.pending[msg.ID]
	if exists {
		record.Attempts++
		record.LastAttempt = time.Now()
	}
	dm.pendingMu.Unlock()

	atomic.AddInt64(&dm.stats.TotalSent, 1)

	// Request with timeout for ack
	if dm.config.Timeout > 0 {
		reply, err := dm.conn.Request(msg.Subject, msg.Data, dm.config.Timeout)
		if err != nil {
			if errors.Is(err, nats.ErrTimeout) {
				atomic.AddInt64(&dm.stats.TotalTimeout, 1)
				if exists {
					dm.updateRecordStatus(msg.ID, DeliveryStatusTimeout, err.Error())
				}
			}
			return err
		}

		// Check reply for ack/nack
		if len(reply.Data) > 0 && string(reply.Data) == "NACK" {
			atomic.AddInt64(&dm.stats.TotalNacked, 1)
			if exists {
				dm.updateRecordStatus(msg.ID, DeliveryStatusNacked, "message nacked")
			}
			return errors.New("message nacked")
		}
	} else {
		// Fire and forget with no ack expected
		if err := dm.conn.Publish(msg.Subject, msg.Data); err != nil {
			return err
		}
	}

	// Success
	atomic.AddInt64(&dm.stats.TotalAcked, 1)
	atomic.AddInt64(&dm.stats.PendingCount, -1)

	if exists {
		dm.updateRecordStatus(msg.ID, DeliveryStatusAcked, "")

		// Calculate latency
		dm.pendingMu.RLock()
		if record, ok := dm.pending[msg.ID]; ok {
			latency := time.Since(record.FirstAttempt)
			record.Latency = latency
			dm.recordLatency(latency)
		}
		dm.pendingMu.RUnlock()

		// Notify callback
		if dm.onDeliveryComplete != nil {
			dm.pendingMu.RLock()
			recordCopy := *dm.pending[msg.ID]
			dm.pendingMu.RUnlock()
			dm.onDeliveryComplete(&recordCopy)
		}
	}

	return nil
}

func (dm *DeliveryManager) updateRecordStatus(msgID string, status DeliveryStatus, errMsg string) {
	dm.pendingMu.Lock()
	defer dm.pendingMu.Unlock()

	if record, exists := dm.pending[msgID]; exists {
		record.Status = status
		record.Error = errMsg
		if status == DeliveryStatusAcked {
			record.AckedAt = time.Now()
		}
	}
}

func (dm *DeliveryManager) recordLatency(latency time.Duration) {
	dm.statsMu.Lock()
	defer dm.statsMu.Unlock()

	dm.latencies = append(dm.latencies, latency)
	if len(dm.latencies) > 1000 {
		dm.latencies = dm.latencies[1:]
	}
}

func (dm *DeliveryManager) retryLoop() {
	defer dm.wg.Done()

	ticker := time.NewTicker(dm.config.RetryBackoff)
	defer ticker.Stop()

	for {
		select {
		case <-dm.ctx.Done():
			return
		case <-ticker.C:
			dm.processRetries()
		}
	}
}

func (dm *DeliveryManager) processRetries() {
	if dm.buffer == nil {
		return
	}

	// Process pending messages in buffer
	for {
		msg := dm.buffer.Dequeue()
		if msg == nil {
			break
		}

		// Check max retries
		if dm.config.MaxRetries > 0 && msg.Attempts >= dm.config.MaxRetries {
			dm.handleDeadLetter(msg, errors.New("max retries exceeded"))
			continue
		}

		// Calculate backoff
		backoff := dm.calculateBackoff(msg.Attempts)
		if time.Since(msg.LastAttempt) < backoff {
			// Not ready for retry, re-queue
			dm.buffer.Enqueue(msg)
			continue
		}

		// Attempt delivery
		if err := dm.attemptDelivery(msg); err != nil {
			atomic.AddInt64(&dm.stats.TotalRetries, 1)
			msg.Attempts++
			msg.LastAttempt = time.Now()
			dm.buffer.Enqueue(msg)
		}
	}
}

func (dm *DeliveryManager) calculateBackoff(attempts int) time.Duration {
	backoff := float64(dm.config.RetryBackoff) * math.Pow(dm.config.BackoffMultiplier, float64(attempts))
	if backoff > float64(dm.config.MaxRetryBackoff) {
		backoff = float64(dm.config.MaxRetryBackoff)
	}
	return time.Duration(backoff)
}

func (dm *DeliveryManager) handleDeadLetter(msg *BufferedMessage, err error) {
	atomic.AddInt64(&dm.stats.TotalFailed, 1)
	atomic.AddInt64(&dm.stats.PendingCount, -1)

	dm.updateRecordStatus(msg.ID, DeliveryStatusFailed, err.Error())

	// Send to DLQ if configured
	if dm.config.DeadLetterSubject != "" && dm.conn != nil {
		dlqErr := dm.conn.Publish(dm.config.DeadLetterSubject, msg.Data)
		if dlqErr == nil {
			atomic.AddInt64(&dm.stats.TotalDeadLettered, 1)
			dm.updateRecordStatus(msg.ID, DeliveryStatusDeadLettered, err.Error())
		}
	}

	// Notify callback
	if dm.onDeadLetter != nil {
		dm.onDeadLetter(msg, err)
	}
}

func (dm *DeliveryManager) cleanupLoop() {
	defer dm.wg.Done()

	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-dm.ctx.Done():
			return
		case <-ticker.C:
			dm.cleanupOldRecords()
		}
	}
}

func (dm *DeliveryManager) cleanupOldRecords() {
	if dm.config.AckWindow <= 0 {
		return
	}

	cutoff := time.Now().Add(-dm.config.AckWindow)

	dm.pendingMu.Lock()
	defer dm.pendingMu.Unlock()

	for id, record := range dm.pending {
		// Remove completed records older than window
		if record.Status != DeliveryStatusPending && record.LastAttempt.Before(cutoff) {
			delete(dm.pending, id)
		}
	}
}

// GetStats returns delivery statistics
func (dm *DeliveryManager) GetStats() DeliveryStats {
	dm.statsMu.RLock()
	defer dm.statsMu.RUnlock()

	stats := DeliveryStats{
		TotalSent:         atomic.LoadInt64(&dm.stats.TotalSent),
		TotalAcked:        atomic.LoadInt64(&dm.stats.TotalAcked),
		TotalNacked:       atomic.LoadInt64(&dm.stats.TotalNacked),
		TotalTimeout:      atomic.LoadInt64(&dm.stats.TotalTimeout),
		TotalFailed:       atomic.LoadInt64(&dm.stats.TotalFailed),
		TotalDeadLettered: atomic.LoadInt64(&dm.stats.TotalDeadLettered),
		TotalRetries:      atomic.LoadInt64(&dm.stats.TotalRetries),
		PendingCount:      atomic.LoadInt64(&dm.stats.PendingCount),
	}

	// Calculate latency percentiles
	if len(dm.latencies) > 0 {
		sorted := make([]time.Duration, len(dm.latencies))
		copy(sorted, dm.latencies)
		sortDurations(sorted)

		var total time.Duration
		for _, l := range sorted {
			total += l
		}
		stats.AverageLatency = total / time.Duration(len(sorted))

		p95Idx := int(float64(len(sorted)) * 0.95)
		if p95Idx >= len(sorted) {
			p95Idx = len(sorted) - 1
		}
		stats.P95Latency = sorted[p95Idx]

		p99Idx := int(float64(len(sorted)) * 0.99)
		if p99Idx >= len(sorted) {
			p99Idx = len(sorted) - 1
		}
		stats.P99Latency = sorted[p99Idx]
	}

	return stats
}

// GetPendingRecords returns all pending delivery records
func (dm *DeliveryManager) GetPendingRecords() []*DeliveryRecord {
	dm.pendingMu.RLock()
	defer dm.pendingMu.RUnlock()

	records := make([]*DeliveryRecord, 0, len(dm.pending))
	for _, record := range dm.pending {
		if record.Status == DeliveryStatusPending {
			recordCopy := *record
			records = append(records, &recordCopy)
		}
	}
	return records
}

// GetRecord returns a specific delivery record
func (dm *DeliveryManager) GetRecord(msgID string) *DeliveryRecord {
	dm.pendingMu.RLock()
	defer dm.pendingMu.RUnlock()

	if record, exists := dm.pending[msgID]; exists {
		recordCopy := *record
		return &recordCopy
	}
	return nil
}

// SetDeliveryCompleteCallback sets the delivery complete callback
func (dm *DeliveryManager) SetDeliveryCompleteCallback(fn func(*DeliveryRecord)) {
	dm.onDeliveryComplete = fn
}

// SetDeadLetterCallback sets the dead letter callback
func (dm *DeliveryManager) SetDeadLetterCallback(fn func(*BufferedMessage, error)) {
	dm.onDeadLetter = fn
}

// Acknowledge manually acknowledges a message
func (dm *DeliveryManager) Acknowledge(msgID string) {
	dm.updateRecordStatus(msgID, DeliveryStatusAcked, "")
	atomic.AddInt64(&dm.stats.TotalAcked, 1)
	atomic.AddInt64(&dm.stats.PendingCount, -1)
}

// NegativeAcknowledge manually nacks a message
func (dm *DeliveryManager) NegativeAcknowledge(msgID string) {
	dm.updateRecordStatus(msgID, DeliveryStatusNacked, "manually nacked")
	atomic.AddInt64(&dm.stats.TotalNacked, 1)
}

// Helper functions

func generateDeliveryMessageID() string {
	return fmt.Sprintf("%d-%d", time.Now().UnixNano(), time.Now().UnixNano()%1000000)
}

func sortDurations(durations []time.Duration) {
	// Simple insertion sort for small slices
	for i := 1; i < len(durations); i++ {
		j := i
		for j > 0 && durations[j] < durations[j-1] {
			durations[j], durations[j-1] = durations[j-1], durations[j]
			j--
		}
	}
}
