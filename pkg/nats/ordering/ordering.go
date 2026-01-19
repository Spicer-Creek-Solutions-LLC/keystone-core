// Package ordering provides message ordering guarantees for NATS messaging.
//
// # Message Ordering Semantics in Keystone
//
// Keystone provides several levels of message ordering guarantees:
//
// 1. Per-Subject Ordering: Messages published to the same subject are
//    delivered in the order they were published (within a single publisher).
//
// 2. Per-Partition Ordering: Messages with the same partition key are
//    delivered in order, regardless of subject.
//
// 3. Global Ordering: All messages across all subjects are delivered
//    in a globally consistent order (requires single consumer).
//
// # NATS JetStream Ordering Guarantees
//
// JetStream provides the following guarantees:
//
// - Stream-level: Messages are stored in the order received
// - Consumer-level: Messages are delivered in storage order
// - Ack-based: Unacked messages block subsequent delivery
//
// # Usage Patterns
//
// For per-agent ordering (recommended for most cases):
//
//	pub := ordering.NewOrderedPublisher(conn, ordering.Config{
//	    OrderingKey: ordering.PartitionByAgentID,
//	    Stream:      "KEYSTONE-EVENTS",
//	})
//	pub.Publish(ctx, "events.agent.status", data, "agent-123")
//
// For strict global ordering:
//
//	consumer := ordering.NewOrderedConsumer(js, ordering.ConsumerConfig{
//	    Stream:       "KEYSTONE-COMMANDS",
//	    MaxAckPending: 1, // Process one at a time
//	    AckWait:      30 * time.Second,
//	})
//
// # Ordering Limitations
//
// - Multiple publishers to the same subject without coordination
//   may interleave messages
// - Network partitions can cause temporary out-of-order delivery
// - Consumer restarts may cause redelivery of unacked messages
// - Exactly-once semantics require idempotent consumers
//
// # Best Practices
//
// 1. Use partition keys to group related messages
// 2. Design consumers to be idempotent
// 3. Use sequence numbers for ordering verification
// 4. Monitor sequence gaps for detecting issues
package ordering

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/nats-io/nats.go"
)

// OrderingMode defines the message ordering strategy
type OrderingMode string

const (
	// OrderingModeNone provides no ordering guarantees (highest throughput)
	OrderingModeNone OrderingMode = "none"

	// OrderingModePerSubject orders messages within the same subject
	OrderingModePerSubject OrderingMode = "per_subject"

	// OrderingModePerPartition orders messages with the same partition key
	OrderingModePerPartition OrderingMode = "per_partition"

	// OrderingModeGlobal provides global ordering (lowest throughput)
	OrderingModeGlobal OrderingMode = "global"
)

// PartitionKeyFunc generates a partition key from message metadata
type PartitionKeyFunc func(subject string, data []byte, headers map[string]string) string

// Common partition key functions
var (
	// PartitionBySubject uses the subject as partition key
	PartitionBySubject PartitionKeyFunc = func(subject string, _ []byte, _ map[string]string) string {
		return subject
	}

	// PartitionByAgentID uses the agent-id header as partition key
	PartitionByAgentID PartitionKeyFunc = func(_ string, _ []byte, headers map[string]string) string {
		if id, ok := headers["agent-id"]; ok {
			return id
		}
		return "default"
	}

	// PartitionByCorrelationID uses correlation-id for request/response ordering
	PartitionByCorrelationID PartitionKeyFunc = func(_ string, _ []byte, headers map[string]string) string {
		if id, ok := headers["correlation-id"]; ok {
			return id
		}
		return "default"
	}
)

// Config configures ordered publishing
type Config struct {
	// Mode is the ordering mode
	Mode OrderingMode

	// PartitionKey is the function to extract partition keys
	PartitionKey PartitionKeyFunc

	// Stream is the JetStream stream name (required for ordering)
	Stream string

	// WindowSize is the max outstanding publishes per partition
	// Smaller values = stronger ordering, lower throughput
	WindowSize int

	// AckTimeout is how long to wait for publish acks
	AckTimeout time.Duration

	// MaxRetries is the max publish retries
	MaxRetries int

	// RetryDelay is the delay between retries
	RetryDelay time.Duration
}

// DefaultConfig returns default ordering configuration
func DefaultConfig() Config {
	return Config{
		Mode:         OrderingModePerPartition,
		PartitionKey: PartitionByAgentID,
		WindowSize:   1, // Strict ordering by default
		AckTimeout:   5 * time.Second,
		MaxRetries:   3,
		RetryDelay:   100 * time.Millisecond,
	}
}

// Validate validates the configuration
func (c Config) Validate() error {
	if c.Mode == OrderingModePerPartition && c.PartitionKey == nil {
		return errors.New("partition key function required for per-partition ordering")
	}
	if c.WindowSize < 1 {
		return errors.New("window size must be at least 1")
	}
	if c.AckTimeout <= 0 {
		return errors.New("ack timeout must be positive")
	}
	return nil
}

// OrderedMessage represents a message with ordering metadata
type OrderedMessage struct {
	// Subject is the message subject
	Subject string

	// Data is the message payload
	Data []byte

	// Headers are optional message headers
	Headers map[string]string

	// PartitionKey groups related messages
	PartitionKey string

	// Sequence is the per-partition sequence number
	Sequence uint64

	// Timestamp is when the message was created
	Timestamp time.Time
}

// partitionState tracks state for a single partition
type partitionState struct {
	mu          sync.Mutex
	sequence    uint64
	pending     int
	waitCh      chan struct{}
	lastPublish time.Time
}

// OrderedPublisher publishes messages with ordering guarantees
type OrderedPublisher struct {
	config Config
	conn   *nats.Conn
	js     nats.JetStreamContext

	partitions   map[string]*partitionState
	partitionsMu sync.RWMutex

	stats       PublisherStats
	running     atomic.Bool
	ctx         context.Context
	cancel      context.CancelFunc
}

// PublisherStats tracks publisher statistics
type PublisherStats struct {
	// TotalPublished is total messages published
	TotalPublished atomic.Int64

	// TotalRetries is total retry attempts
	TotalRetries atomic.Int64

	// TotalFailed is total failed publishes
	TotalFailed atomic.Int64

	// TotalOrdered is total messages with ordering enforced
	TotalOrdered atomic.Int64

	// PartitionCount is the number of active partitions
	PartitionCount atomic.Int64

	// AverageLatency is the average publish latency
	AverageLatency atomic.Int64 // nanoseconds
}

// NewOrderedPublisher creates a new ordered publisher
func NewOrderedPublisher(conn *nats.Conn, config Config) (*OrderedPublisher, error) {
	if err := config.Validate(); err != nil {
		return nil, fmt.Errorf("invalid config: %w", err)
	}

	ctx, cancel := context.WithCancel(context.Background())

	pub := &OrderedPublisher{
		config:     config,
		conn:       conn,
		partitions: make(map[string]*partitionState),
		ctx:        ctx,
		cancel:     cancel,
	}

	// Setup JetStream if ordering is required
	if config.Mode != OrderingModeNone && config.Stream != "" && conn != nil {
		js, err := conn.JetStream()
		if err != nil {
			cancel()
			return nil, fmt.Errorf("JetStream setup failed: %w", err)
		}
		pub.js = js
	}

	pub.running.Store(true)
	return pub, nil
}

// Publish publishes a message with ordering guarantees
func (p *OrderedPublisher) Publish(ctx context.Context, subject string, data []byte, headers map[string]string) error {
	if !p.running.Load() {
		return errors.New("publisher not running")
	}

	// Get partition key
	partitionKey := ""
	if p.config.PartitionKey != nil {
		partitionKey = p.config.PartitionKey(subject, data, headers)
	}

	// Get or create partition state
	state := p.getOrCreatePartition(partitionKey)

	// Wait for window slot if needed
	if p.config.Mode != OrderingModeNone {
		if err := p.waitForWindow(ctx, state); err != nil {
			return err
		}
	}

	// Assign sequence number
	state.mu.Lock()
	state.sequence++
	seq := state.sequence
	state.pending++
	state.lastPublish = time.Now()
	state.mu.Unlock()

	// Build message
	msg := &OrderedMessage{
		Subject:      subject,
		Data:         data,
		Headers:      headers,
		PartitionKey: partitionKey,
		Sequence:     seq,
		Timestamp:    time.Now(),
	}

	// Publish with retries
	start := time.Now()
	err := p.publishWithRetry(ctx, msg)
	latency := time.Since(start)

	// Update stats
	p.stats.AverageLatency.Store(int64(latency))

	// Release window slot
	state.mu.Lock()
	state.pending--
	if state.waitCh != nil && state.pending < p.config.WindowSize {
		select {
		case state.waitCh <- struct{}{}:
		default:
		}
	}
	state.mu.Unlock()

	if err != nil {
		p.stats.TotalFailed.Add(1)
		return err
	}

	p.stats.TotalPublished.Add(1)
	if p.config.Mode != OrderingModeNone {
		p.stats.TotalOrdered.Add(1)
	}

	return nil
}

func (p *OrderedPublisher) getOrCreatePartition(key string) *partitionState {
	p.partitionsMu.RLock()
	state, exists := p.partitions[key]
	p.partitionsMu.RUnlock()

	if exists {
		return state
	}

	p.partitionsMu.Lock()
	defer p.partitionsMu.Unlock()

	// Double-check after acquiring write lock
	if state, exists = p.partitions[key]; exists {
		return state
	}

	state = &partitionState{
		waitCh: make(chan struct{}, 1),
	}
	p.partitions[key] = state
	p.stats.PartitionCount.Add(1)

	return state
}

func (p *OrderedPublisher) waitForWindow(ctx context.Context, state *partitionState) error {
	state.mu.Lock()
	if state.pending < p.config.WindowSize {
		state.mu.Unlock()
		return nil
	}
	waitCh := state.waitCh
	state.mu.Unlock()

	// Wait for window slot
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-p.ctx.Done():
		return errors.New("publisher stopped")
	case <-waitCh:
		return nil
	}
}

func (p *OrderedPublisher) publishWithRetry(ctx context.Context, msg *OrderedMessage) error {
	var lastErr error

	for attempt := 0; attempt <= p.config.MaxRetries; attempt++ {
		if attempt > 0 {
			p.stats.TotalRetries.Add(1)
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(p.config.RetryDelay):
			}
		}

		err := p.doPublish(ctx, msg)
		if err == nil {
			return nil
		}
		lastErr = err
	}

	return fmt.Errorf("publish failed after %d attempts: %w", p.config.MaxRetries+1, lastErr)
}

func (p *OrderedPublisher) doPublish(ctx context.Context, msg *OrderedMessage) error {
	if p.conn == nil {
		return errors.New("not connected")
	}

	// Build NATS message
	natsMsg := &nats.Msg{
		Subject: msg.Subject,
		Data:    msg.Data,
		Header:  make(nats.Header),
	}

	// Add ordering headers
	natsMsg.Header.Set("X-Keystone-Partition", msg.PartitionKey)
	natsMsg.Header.Set("X-Keystone-Sequence", fmt.Sprintf("%d", msg.Sequence))
	natsMsg.Header.Set("X-Keystone-Timestamp", msg.Timestamp.Format(time.RFC3339Nano))

	// Add user headers
	for k, v := range msg.Headers {
		natsMsg.Header.Set(k, v)
	}

	// Use JetStream for guaranteed ordering
	if p.js != nil && p.config.Stream != "" {
		pubCtx, cancel := context.WithTimeout(ctx, p.config.AckTimeout)
		defer cancel()

		// Synchronous publish with ack
		opts := []nats.PubOpt{
			nats.MsgId(fmt.Sprintf("%s-%d", msg.PartitionKey, msg.Sequence)),
			nats.ExpectStream(p.config.Stream),
		}

		_, err := p.js.PublishMsg(natsMsg, opts...)
		if err != nil {
			return fmt.Errorf("JetStream publish failed: %w", err)
		}

		// Wait for ack
		select {
		case <-pubCtx.Done():
			return pubCtx.Err()
		default:
			return nil
		}
	}

	// Regular NATS publish (best-effort ordering)
	return p.conn.PublishMsg(natsMsg)
}

// GetStats returns publisher statistics
func (p *OrderedPublisher) GetStats() PublisherStats {
	return p.stats
}

// Close closes the publisher
func (p *OrderedPublisher) Close() error {
	if !p.running.Load() {
		return nil
	}
	p.running.Store(false)
	p.cancel()
	return nil
}

// ConsumerConfig configures ordered consumption
type ConsumerConfig struct {
	// Stream is the JetStream stream name
	Stream string

	// Consumer is the durable consumer name
	Consumer string

	// Subject is the subject filter
	Subject string

	// MaxAckPending limits concurrent unacked messages (1 = strict ordering)
	MaxAckPending int

	// AckWait is how long to wait before redelivery
	AckWait time.Duration

	// MaxDeliver is the max delivery attempts
	MaxDeliver int

	// ReplayPolicy controls message replay on startup
	ReplayPolicy ReplayPolicy

	// SequenceValidation enables sequence gap detection
	SequenceValidation bool

	// Handler processes received messages
	Handler OrderedMessageHandler
}

// ReplayPolicy controls message replay behavior
type ReplayPolicy string

const (
	// ReplayInstant replays all messages as fast as possible
	ReplayInstant ReplayPolicy = "instant"

	// ReplayOriginal replays at the original rate
	ReplayOriginal ReplayPolicy = "original"
)

// OrderedMessageHandler handles ordered messages
type OrderedMessageHandler func(ctx context.Context, msg *OrderedMessage) error

// DefaultConsumerConfig returns default consumer configuration
func DefaultConsumerConfig() ConsumerConfig {
	return ConsumerConfig{
		MaxAckPending:      1, // Strict ordering
		AckWait:            30 * time.Second,
		MaxDeliver:         5,
		ReplayPolicy:       ReplayInstant,
		SequenceValidation: true,
	}
}

// Validate validates consumer configuration
func (c ConsumerConfig) Validate() error {
	if c.Stream == "" {
		return errors.New("stream name required")
	}
	if c.MaxAckPending < 1 {
		return errors.New("max ack pending must be at least 1")
	}
	if c.Handler == nil {
		return errors.New("handler required")
	}
	return nil
}

// OrderedConsumer consumes messages with ordering guarantees
type OrderedConsumer struct {
	config ConsumerConfig
	js     nats.JetStreamContext
	sub    *nats.Subscription

	// Sequence tracking per partition
	sequences   map[string]uint64
	sequencesMu sync.RWMutex

	stats   ConsumerStats
	running atomic.Bool
	ctx     context.Context
	cancel  context.CancelFunc
	wg      sync.WaitGroup
}

// ConsumerStats tracks consumer statistics
type ConsumerStats struct {
	// TotalReceived is total messages received
	TotalReceived atomic.Int64

	// TotalProcessed is total messages successfully processed
	TotalProcessed atomic.Int64

	// TotalFailed is total messages that failed processing
	TotalFailed atomic.Int64

	// TotalRedelivered is total redelivered messages
	TotalRedelivered atomic.Int64

	// SequenceGaps is total detected sequence gaps
	SequenceGaps atomic.Int64

	// OutOfOrder is total out-of-order messages
	OutOfOrder atomic.Int64

	// AverageProcessingTime is average processing time
	AverageProcessingTime atomic.Int64 // nanoseconds
}

// NewOrderedConsumer creates a new ordered consumer
func NewOrderedConsumer(js nats.JetStreamContext, config ConsumerConfig) (*OrderedConsumer, error) {
	if err := config.Validate(); err != nil {
		return nil, fmt.Errorf("invalid config: %w", err)
	}

	ctx, cancel := context.WithCancel(context.Background())

	consumer := &OrderedConsumer{
		config:    config,
		js:        js,
		sequences: make(map[string]uint64),
		ctx:       ctx,
		cancel:    cancel,
	}

	return consumer, nil
}

// Start starts consuming messages
func (c *OrderedConsumer) Start() error {
	if c.running.Load() {
		return errors.New("already running")
	}

	// Create or bind to consumer
	consumerConfig := &nats.ConsumerConfig{
		Durable:       c.config.Consumer,
		AckPolicy:     nats.AckExplicitPolicy,
		AckWait:       c.config.AckWait,
		MaxAckPending: c.config.MaxAckPending,
		MaxDeliver:    c.config.MaxDeliver,
		FilterSubject: c.config.Subject,
	}

	if c.config.ReplayPolicy == ReplayOriginal {
		consumerConfig.ReplayPolicy = nats.ReplayOriginalPolicy
	} else {
		consumerConfig.ReplayPolicy = nats.ReplayInstantPolicy
	}

	// Subscribe
	sub, err := c.js.PullSubscribe(
		c.config.Subject,
		c.config.Consumer,
		nats.Bind(c.config.Stream, c.config.Consumer),
	)
	if err != nil {
		// Try creating the consumer
		_, err = c.js.AddConsumer(c.config.Stream, consumerConfig)
		if err != nil {
			return fmt.Errorf("failed to create consumer: %w", err)
		}
		sub, err = c.js.PullSubscribe(
			c.config.Subject,
			c.config.Consumer,
			nats.Bind(c.config.Stream, c.config.Consumer),
		)
		if err != nil {
			return fmt.Errorf("failed to subscribe: %w", err)
		}
	}

	c.sub = sub
	c.running.Store(true)

	// Start consumer loop
	c.wg.Add(1)
	go c.consumeLoop()

	return nil
}

func (c *OrderedConsumer) consumeLoop() {
	defer c.wg.Done()

	for c.running.Load() {
		select {
		case <-c.ctx.Done():
			return
		default:
		}

		// Fetch messages
		msgs, err := c.sub.Fetch(c.config.MaxAckPending, nats.MaxWait(time.Second))
		if err != nil {
			if errors.Is(err, nats.ErrTimeout) {
				continue
			}
			// Log error and continue
			continue
		}

		for _, natsMsg := range msgs {
			c.processMessage(natsMsg)
		}
	}
}

func (c *OrderedConsumer) processMessage(natsMsg *nats.Msg) {
	c.stats.TotalReceived.Add(1)

	// Check for redelivery
	meta, err := natsMsg.Metadata()
	if err == nil && meta.NumDelivered > 1 {
		c.stats.TotalRedelivered.Add(1)
	}

	// Extract ordering metadata
	msg := &OrderedMessage{
		Subject:   natsMsg.Subject,
		Data:      natsMsg.Data,
		Headers:   make(map[string]string),
		Timestamp: time.Now(),
	}

	if natsMsg.Header != nil {
		msg.PartitionKey = natsMsg.Header.Get("X-Keystone-Partition")
		if seqStr := natsMsg.Header.Get("X-Keystone-Sequence"); seqStr != "" {
			var seq uint64
			fmt.Sscanf(seqStr, "%d", &seq)
			msg.Sequence = seq
		}
		if tsStr := natsMsg.Header.Get("X-Keystone-Timestamp"); tsStr != "" {
			if ts, err := time.Parse(time.RFC3339Nano, tsStr); err == nil {
				msg.Timestamp = ts
			}
		}

		// Copy user headers
		for k, v := range natsMsg.Header {
			if len(v) > 0 && k[0] != 'X' {
				msg.Headers[k] = v[0]
			}
		}
	}

	// Validate sequence if enabled
	if c.config.SequenceValidation && msg.PartitionKey != "" {
		c.validateSequence(msg)
	}

	// Process message
	start := time.Now()
	err = c.config.Handler(c.ctx, msg)
	processingTime := time.Since(start)

	c.stats.AverageProcessingTime.Store(int64(processingTime))

	if err != nil {
		c.stats.TotalFailed.Add(1)
		// NAK for redelivery
		natsMsg.Nak()
		return
	}

	c.stats.TotalProcessed.Add(1)
	natsMsg.Ack()
}

func (c *OrderedConsumer) validateSequence(msg *OrderedMessage) {
	c.sequencesMu.Lock()
	defer c.sequencesMu.Unlock()

	expectedSeq := c.sequences[msg.PartitionKey] + 1

	if msg.Sequence == 0 {
		// No sequence in message, skip validation
		return
	}

	if msg.Sequence < expectedSeq {
		// Out of order (likely redelivery)
		c.stats.OutOfOrder.Add(1)
	} else if msg.Sequence > expectedSeq {
		// Sequence gap detected
		c.stats.SequenceGaps.Add(1)
	}

	// Update expected sequence
	if msg.Sequence > c.sequences[msg.PartitionKey] {
		c.sequences[msg.PartitionKey] = msg.Sequence
	}
}

// GetStats returns consumer statistics
func (c *OrderedConsumer) GetStats() ConsumerStats {
	return c.stats
}

// Stop stops the consumer
func (c *OrderedConsumer) Stop() error {
	if !c.running.Load() {
		return nil
	}

	c.running.Store(false)
	c.cancel()
	c.wg.Wait()

	if c.sub != nil {
		c.sub.Drain()
	}

	return nil
}

// SequenceTracker tracks message sequences for gap detection
type SequenceTracker struct {
	sequences map[string]*sequenceState
	mu        sync.RWMutex
	maxGap    int
}

type sequenceState struct {
	lastSeen  uint64
	gaps      []uint64
	timestamp time.Time
}

// NewSequenceTracker creates a new sequence tracker
func NewSequenceTracker(maxGap int) *SequenceTracker {
	return &SequenceTracker{
		sequences: make(map[string]*sequenceState),
		maxGap:    maxGap,
	}
}

// Track tracks a sequence number and returns any gaps
func (t *SequenceTracker) Track(partition string, seq uint64) []uint64 {
	t.mu.Lock()
	defer t.mu.Unlock()

	state, exists := t.sequences[partition]
	if !exists {
		t.sequences[partition] = &sequenceState{
			lastSeen:  seq,
			timestamp: time.Now(),
		}
		return nil
	}

	// Detect gaps
	var gaps []uint64
	if seq > state.lastSeen+1 {
		for i := state.lastSeen + 1; i < seq; i++ {
			gaps = append(gaps, i)
			if len(gaps) >= t.maxGap {
				break
			}
		}
		state.gaps = append(state.gaps, gaps...)
	}

	if seq > state.lastSeen {
		state.lastSeen = seq
	}
	state.timestamp = time.Now()

	return gaps
}

// GetGaps returns all detected gaps for a partition
func (t *SequenceTracker) GetGaps(partition string) []uint64 {
	t.mu.RLock()
	defer t.mu.RUnlock()

	if state, exists := t.sequences[partition]; exists {
		gaps := make([]uint64, len(state.gaps))
		copy(gaps, state.gaps)
		return gaps
	}
	return nil
}

// ClearGap removes a filled gap
func (t *SequenceTracker) ClearGap(partition string, seq uint64) {
	t.mu.Lock()
	defer t.mu.Unlock()

	if state, exists := t.sequences[partition]; exists {
		for i, gap := range state.gaps {
			if gap == seq {
				state.gaps = append(state.gaps[:i], state.gaps[i+1:]...)
				return
			}
		}
	}
}

// GetLastSequence returns the last seen sequence for a partition
func (t *SequenceTracker) GetLastSequence(partition string) uint64 {
	t.mu.RLock()
	defer t.mu.RUnlock()

	if state, exists := t.sequences[partition]; exists {
		return state.lastSeen
	}
	return 0
}
