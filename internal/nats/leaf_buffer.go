// Package nats provides NATS messaging infrastructure for Keystone Core
package nats

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/shawnbutts/keystone-core/pkg/wait"
)

// BufferConfig configures the message buffer for offline persistence
type BufferConfig struct {
	// Enabled enables message buffering during outages
	Enabled bool

	// MaxSize is the maximum buffer size in bytes (default: 64MB)
	MaxSize int64

	// MaxMessages is the maximum number of messages to buffer
	MaxMessages int

	// MaxAge is the maximum age of buffered messages (default: 1 hour)
	MaxAge time.Duration

	// PersistToDisk enables JetStream persistence (requires JetStream)
	PersistToDisk bool

	// StreamName is the JetStream stream name for persistent buffer
	StreamName string

	// FlushInterval is how often to attempt flushing buffered messages
	FlushInterval time.Duration

	// FlushBatchSize is the number of messages to flush per batch
	FlushBatchSize int

	// RetryDelay is the delay between retries when flushing fails
	RetryDelay time.Duration

	// DeduplicationWindow is the window for message deduplication
	DeduplicationWindow time.Duration
}

// DefaultBufferConfig returns default buffer configuration
func DefaultBufferConfig() *BufferConfig {
	return &BufferConfig{
		Enabled:             true,
		MaxSize:             64 * 1024 * 1024, // 64MB
		MaxMessages:         10000,
		MaxAge:              1 * time.Hour,
		PersistToDisk:       false,
		StreamName:          "KSCORE_BUFFER",
		FlushInterval:       5 * time.Second,
		FlushBatchSize:      100,
		RetryDelay:          1 * time.Second,
		DeduplicationWindow: 2 * time.Minute,
	}
}

// BufferedMessage represents a message waiting to be delivered
type BufferedMessage struct {
	// ID is a unique message identifier for deduplication
	ID string `json:"id"`

	// Subject is the NATS subject
	Subject string `json:"subject"`

	// Data is the message payload
	Data []byte `json:"data"`

	// Header contains optional headers
	Header map[string][]string `json:"header,omitempty"`

	// Timestamp is when the message was buffered
	Timestamp time.Time `json:"timestamp"`

	// RetryCount is the number of delivery attempts
	RetryCount int `json:"retry_count"`

	// Attempts is an alias for RetryCount (for delivery manager compatibility)
	Attempts int `json:"attempts,omitempty"`

	// LastAttempt is the time of the last delivery attempt
	LastAttempt time.Time `json:"last_attempt,omitempty"`
}

// MessageBufferState represents the state of the message buffer
type MessageBufferState int

const (
	// BufferStateIdle no buffering active
	BufferStateIdle MessageBufferState = iota
	// BufferStateBuffering actively buffering messages
	BufferStateBuffering
	// BufferStateFlushing flushing buffered messages
	BufferStateFlushing
	// BufferStateDraining draining buffer (stopping)
	BufferStateDraining
)

// String returns string representation of MessageBufferState
func (s MessageBufferState) String() string {
	switch s {
	case BufferStateIdle:
		return "idle"
	case BufferStateBuffering:
		return "buffering"
	case BufferStateFlushing:
		return "flushing"
	case BufferStateDraining:
		return "draining"
	default:
		return "unknown"
	}
}

// MessageBuffer provides message buffering during connection outages
type MessageBuffer struct {
	config *BufferConfig

	// In-memory buffer (always used)
	messages []*BufferedMessage
	msgMu    sync.RWMutex

	// JetStream for persistence
	js nats.JetStreamContext

	// Statistics
	currentSize atomic.Int64
	totalAdded  atomic.Int64
	totalSent   atomic.Int64
	totalFailed atomic.Int64
	droppedOld  atomic.Int64
	droppedSize atomic.Int64

	// State
	state   atomic.Int32
	running atomic.Bool

	// Callbacks
	onBufferFull   func(subject string, size int64)
	onMessageSent  func(msg *BufferedMessage)
	onMessageError func(msg *BufferedMessage, err error)

	// Deduplication
	seenMessages map[string]time.Time
	seenMu       sync.RWMutex

	// Context
	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup
	mu     sync.RWMutex
}

// NewMessageBuffer creates a new message buffer
func NewMessageBuffer(config *BufferConfig) (*MessageBuffer, error) {
	if config == nil {
		config = DefaultBufferConfig()
	}

	if config.MaxSize <= 0 {
		return nil, errors.New("max size must be positive")
	}
	if config.MaxMessages <= 0 {
		return nil, errors.New("max messages must be positive")
	}

	return &MessageBuffer{
		config:       config,
		messages:     make([]*BufferedMessage, 0, 1000),
		seenMessages: make(map[string]time.Time),
	}, nil
}

// SetJetStream sets the JetStream context for persistent buffering
func (b *MessageBuffer) SetJetStream(js nats.JetStreamContext) error {
	b.mu.Lock()
	defer b.mu.Unlock()

	b.js = js

	if b.config.PersistToDisk && js != nil {
		// Create or update the buffer stream
		streamConfig := &nats.StreamConfig{
			Name:       b.config.StreamName,
			Subjects:   []string{b.config.StreamName + ".>"},
			Storage:    nats.FileStorage,
			MaxBytes:   b.config.MaxSize,
			MaxMsgs:    int64(b.config.MaxMessages),
			MaxAge:     b.config.MaxAge,
			Duplicates: b.config.DeduplicationWindow,
			Retention:  nats.WorkQueuePolicy,
		}

		_, err := js.AddStream(streamConfig)
		if err != nil {
			// Try to update if it already exists
			_, err = js.UpdateStream(streamConfig)
			if err != nil {
				return fmt.Errorf("create/update buffer stream: %w", err)
			}
		}
	}

	return nil
}

// Start starts the message buffer
func (b *MessageBuffer) Start(ctx context.Context) error {
	if b.running.Load() {
		return errors.New("buffer already running")
	}

	b.ctx, b.cancel = context.WithCancel(ctx)
	b.running.Store(true)
	b.setState(BufferStateIdle)

	// Start cleanup goroutine
	b.wg.Add(1)
	go b.cleanupLoop()

	return nil
}

// Stop stops the message buffer
func (b *MessageBuffer) Stop() error {
	if !b.running.Load() {
		return nil
	}

	b.running.Store(false)
	b.setState(BufferStateDraining)

	if b.cancel != nil {
		b.cancel()
	}

	b.wg.Wait()
	b.setState(BufferStateIdle)

	return nil
}

// Buffer adds a message to the buffer
func (b *MessageBuffer) Buffer(msg *BufferedMessage) error {
	if !b.config.Enabled {
		return errors.New("buffering disabled")
	}

	if msg == nil {
		return errors.New("nil message")
	}

	// Check for duplicates
	if msg.ID != "" && b.isDuplicate(msg.ID) {
		return nil // Already seen, ignore
	}

	msgSize := int64(len(msg.Data))

	// Check size limits
	if msgSize > b.config.MaxSize {
		return fmt.Errorf("message size %d exceeds max %d", msgSize, b.config.MaxSize)
	}

	b.msgMu.Lock()
	defer b.msgMu.Unlock()

	// Enforce size limit by removing old messages
	for b.currentSize.Load()+msgSize > b.config.MaxSize && len(b.messages) > 0 {
		oldMsg := b.messages[0]
		b.messages = b.messages[1:]
		b.currentSize.Add(-int64(len(oldMsg.Data)))
		b.droppedSize.Add(1)
	}

	// Enforce message count limit
	for len(b.messages) >= b.config.MaxMessages && len(b.messages) > 0 {
		oldMsg := b.messages[0]
		b.messages = b.messages[1:]
		b.currentSize.Add(-int64(len(oldMsg.Data)))
		b.droppedOld.Add(1)
	}

	// Set timestamp if not set
	if msg.Timestamp.IsZero() {
		msg.Timestamp = time.Now()
	}

	// Add to buffer
	b.messages = append(b.messages, msg)
	b.currentSize.Add(msgSize)
	b.totalAdded.Add(1)

	// Mark as seen for deduplication
	if msg.ID != "" {
		b.markSeen(msg.ID)
	}

	b.setState(BufferStateBuffering)

	// Also persist to JetStream if enabled
	if b.config.PersistToDisk && b.js != nil {
		go b.persistToJetStream(msg)
	}

	return nil
}

// BufferNATSMessage buffers a NATS message
func (b *MessageBuffer) BufferNATSMessage(msg *nats.Msg) error {
	if msg == nil {
		return errors.New("nil message")
	}

	buffered := &BufferedMessage{
		ID:        generateBufferMessageID(),
		Subject:   msg.Subject,
		Data:      msg.Data,
		Timestamp: time.Now(),
	}

	// Copy headers if present
	if msg.Header != nil {
		buffered.Header = make(map[string][]string)
		for k, v := range msg.Header {
			buffered.Header[k] = v
		}
	}

	return b.Buffer(buffered)
}

// Flush flushes buffered messages to the given NATS connection
func (b *MessageBuffer) Flush(nc *nats.Conn) error {
	if nc == nil || !nc.IsConnected() {
		return errors.New("no connection available")
	}

	b.setState(BufferStateFlushing)
	defer b.setState(BufferStateIdle)

	b.msgMu.Lock()
	toSend := make([]*BufferedMessage, len(b.messages))
	copy(toSend, b.messages)
	b.messages = b.messages[:0]
	b.currentSize.Store(0)
	b.msgMu.Unlock()

	var lastErr error
	for _, msg := range toSend {
		// Check message age
		if b.config.MaxAge > 0 && time.Since(msg.Timestamp) > b.config.MaxAge {
			b.droppedOld.Add(1)
			continue
		}

		// Create NATS message
		natsMsg := &nats.Msg{
			Subject: msg.Subject,
			Data:    msg.Data,
		}
		if msg.Header != nil {
			natsMsg.Header = nats.Header(msg.Header)
		}

		// Publish with retry
		err := b.publishWithRetry(nc, natsMsg, 3)
		if err != nil {
			lastErr = err
			b.totalFailed.Add(1)

			b.mu.RLock()
			cb := b.onMessageError
			b.mu.RUnlock()
			if cb != nil {
				cb(msg, err)
			}

			// Re-buffer failed message
			b.msgMu.Lock()
			msg.RetryCount++
			b.messages = append(b.messages, msg)
			b.currentSize.Add(int64(len(msg.Data)))
			b.msgMu.Unlock()
		} else {
			b.totalSent.Add(1)

			b.mu.RLock()
			cb := b.onMessageSent
			b.mu.RUnlock()
			if cb != nil {
				cb(msg)
			}
		}
	}

	return lastErr
}

// FlushBatch flushes a batch of messages
func (b *MessageBuffer) FlushBatch(nc *nats.Conn, batchSize int) (int, error) {
	if nc == nil || !nc.IsConnected() {
		return 0, errors.New("no connection available")
	}

	if batchSize <= 0 {
		batchSize = b.config.FlushBatchSize
	}

	b.msgMu.Lock()
	if len(b.messages) == 0 {
		b.msgMu.Unlock()
		return 0, nil
	}

	// Take a batch
	n := batchSize
	if n > len(b.messages) {
		n = len(b.messages)
	}
	batch := make([]*BufferedMessage, n)
	copy(batch, b.messages[:n])
	b.messages = b.messages[n:]

	// Update size
	var batchSize64 int64
	for _, msg := range batch {
		batchSize64 += int64(len(msg.Data))
	}
	b.currentSize.Add(-batchSize64)
	b.msgMu.Unlock()

	b.setState(BufferStateFlushing)
	defer func() {
		if b.Len() > 0 {
			b.setState(BufferStateBuffering)
		} else {
			b.setState(BufferStateIdle)
		}
	}()

	sent := 0
	var lastErr error
	for _, msg := range batch {
		// Check message age
		if b.config.MaxAge > 0 && time.Since(msg.Timestamp) > b.config.MaxAge {
			b.droppedOld.Add(1)
			continue
		}

		natsMsg := &nats.Msg{
			Subject: msg.Subject,
			Data:    msg.Data,
		}
		if msg.Header != nil {
			natsMsg.Header = nats.Header(msg.Header)
		}

		err := b.publishWithRetry(nc, natsMsg, 3)
		if err != nil {
			lastErr = err
			b.totalFailed.Add(1)

			// Re-buffer failed message
			b.msgMu.Lock()
			msg.RetryCount++
			b.messages = append([]*BufferedMessage{msg}, b.messages...)
			b.currentSize.Add(int64(len(msg.Data)))
			b.msgMu.Unlock()
		} else {
			sent++
			b.totalSent.Add(1)
		}
	}

	return sent, lastErr
}

// publishWithRetry publishes a message with retries
func (b *MessageBuffer) publishWithRetry(nc *nats.Conn, msg *nats.Msg, maxRetries int) error {
	var lastErr error
	for i := 0; i <= maxRetries; i++ {
		if msg.Header != nil {
			err := nc.PublishMsg(msg)
			if err == nil {
				return nil
			}
			lastErr = err
		} else {
			err := nc.Publish(msg.Subject, msg.Data)
			if err == nil {
				return nil
			}
			lastErr = err
		}

		if i < maxRetries {
			if !b.waitForRetry(b.config.RetryDelay) {
				return lastErr
			}
		}
	}
	return lastErr
}

func (b *MessageBuffer) waitForRetry(delay time.Duration) bool {
	if delay <= 0 {
		return true
	}

	ctx := b.ctx
	if ctx == nil {
		ctx = context.Background()
	}
	return wait.ForContext(ctx, delay) == nil
}

// Len returns the number of buffered messages
func (b *MessageBuffer) Len() int {
	b.msgMu.RLock()
	defer b.msgMu.RUnlock()
	return len(b.messages)
}

// Size returns the current buffer size in bytes
func (b *MessageBuffer) Size() int64 {
	return b.currentSize.Load()
}

// State returns the current buffer state
func (b *MessageBuffer) State() MessageBufferState {
	return MessageBufferState(b.state.Load())
}

// IsBuffering returns true if actively buffering
func (b *MessageBuffer) IsBuffering() bool {
	return b.State() == BufferStateBuffering
}

// Clear clears the buffer
func (b *MessageBuffer) Clear() {
	b.msgMu.Lock()
	defer b.msgMu.Unlock()

	b.messages = b.messages[:0]
	b.currentSize.Store(0)
	b.setState(BufferStateIdle)
}

// SetBufferFullCallback sets a callback for when buffer is full
func (b *MessageBuffer) SetBufferFullCallback(cb func(subject string, size int64)) {
	b.mu.Lock()
	b.onBufferFull = cb
	b.mu.Unlock()
}

// SetMessageSentCallback sets a callback for successfully sent messages
func (b *MessageBuffer) SetMessageSentCallback(cb func(msg *BufferedMessage)) {
	b.mu.Lock()
	b.onMessageSent = cb
	b.mu.Unlock()
}

// SetMessageErrorCallback sets a callback for message send errors
func (b *MessageBuffer) SetMessageErrorCallback(cb func(msg *BufferedMessage, err error)) {
	b.mu.Lock()
	b.onMessageError = cb
	b.mu.Unlock()
}

// BufferStats contains buffer statistics
type BufferStats struct {
	State        MessageBufferState
	MessageCount int
	CurrentSize  int64
	MaxSize      int64
	TotalAdded   int64
	TotalSent    int64
	TotalFailed  int64
	DroppedOld   int64
	DroppedSize  int64
}

// GetStats returns current buffer statistics
func (b *MessageBuffer) GetStats() *BufferStats {
	return &BufferStats{
		State:        b.State(),
		MessageCount: b.Len(),
		CurrentSize:  b.currentSize.Load(),
		MaxSize:      b.config.MaxSize,
		TotalAdded:   b.totalAdded.Load(),
		TotalSent:    b.totalSent.Load(),
		TotalFailed:  b.totalFailed.Load(),
		DroppedOld:   b.droppedOld.Load(),
		DroppedSize:  b.droppedSize.Load(),
	}
}

// Enqueue adds a message to the buffer (alias for Buffer)
func (b *MessageBuffer) Enqueue(msg *BufferedMessage) error {
	return b.Buffer(msg)
}

// Dequeue removes and returns the first message from the buffer
func (b *MessageBuffer) Dequeue() *BufferedMessage {
	b.msgMu.Lock()
	defer b.msgMu.Unlock()

	if len(b.messages) == 0 {
		return nil
	}

	msg := b.messages[0]
	b.messages = b.messages[1:]
	b.currentSize.Add(-int64(len(msg.Data)))
	return msg
}

// setState updates the buffer state
func (b *MessageBuffer) setState(state MessageBufferState) {
	b.state.Store(int32(state))
}

// isDuplicate checks if a message ID has been seen recently
func (b *MessageBuffer) isDuplicate(id string) bool {
	b.seenMu.RLock()
	defer b.seenMu.RUnlock()

	if ts, ok := b.seenMessages[id]; ok {
		if time.Since(ts) < b.config.DeduplicationWindow {
			return true
		}
	}
	return false
}

// markSeen marks a message ID as seen
func (b *MessageBuffer) markSeen(id string) {
	b.seenMu.Lock()
	defer b.seenMu.Unlock()
	b.seenMessages[id] = time.Now()
}

// cleanupLoop periodically cleans up old entries
func (b *MessageBuffer) cleanupLoop() {
	defer b.wg.Done()

	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			b.cleanupOldMessages()
			b.cleanupSeenMessages()
		case <-b.ctx.Done():
			return
		}
	}
}

// cleanupOldMessages removes expired messages
func (b *MessageBuffer) cleanupOldMessages() {
	if b.config.MaxAge == 0 {
		return
	}

	b.msgMu.Lock()
	defer b.msgMu.Unlock()

	cutoff := time.Now().Add(-b.config.MaxAge)
	filtered := b.messages[:0]

	for _, msg := range b.messages {
		if msg.Timestamp.After(cutoff) {
			filtered = append(filtered, msg)
		} else {
			b.currentSize.Add(-int64(len(msg.Data)))
			b.droppedOld.Add(1)
		}
	}

	b.messages = filtered
}

// cleanupSeenMessages removes old deduplication entries
func (b *MessageBuffer) cleanupSeenMessages() {
	b.seenMu.Lock()
	defer b.seenMu.Unlock()

	cutoff := time.Now().Add(-b.config.DeduplicationWindow)

	for id, ts := range b.seenMessages {
		if ts.Before(cutoff) {
			delete(b.seenMessages, id)
		}
	}
}

// persistToJetStream persists a message to JetStream
func (b *MessageBuffer) persistToJetStream(msg *BufferedMessage) {
	if b.js == nil {
		return
	}

	data, err := json.Marshal(msg)
	if err != nil {
		return
	}

	subject := fmt.Sprintf("%s.%s", b.config.StreamName, msg.Subject)

	opts := []nats.PubOpt{}
	if msg.ID != "" {
		opts = append(opts, nats.MsgId(msg.ID))
	}

	_, _ = b.js.Publish(subject, data, opts...)
}

// LoadFromJetStream loads messages from JetStream persistence
func (b *MessageBuffer) LoadFromJetStream() error {
	if b.js == nil {
		return errors.New("JetStream not configured")
	}

	// Create a consumer to read the buffer stream
	sub, err := b.js.PullSubscribe(
		b.config.StreamName+".>",
		"kscore-buffer-recovery",
		nats.ManualAck(),
	)
	if err != nil {
		return fmt.Errorf("subscribe to buffer stream: %w", err)
	}
	defer sub.Unsubscribe()

	// Fetch all messages
	for {
		msgs, err := sub.Fetch(100, nats.MaxWait(1*time.Second))
		if err != nil {
			if errors.Is(err, nats.ErrTimeout) {
				break // No more messages
			}
			return fmt.Errorf("fetch messages: %w", err)
		}

		for _, m := range msgs {
			var buffered BufferedMessage
			if err := json.Unmarshal(m.Data, &buffered); err != nil {
				_ = m.Nak()
				continue
			}

			// Add to in-memory buffer
			b.msgMu.Lock()
			b.messages = append(b.messages, &buffered)
			b.currentSize.Add(int64(len(buffered.Data)))
			b.msgMu.Unlock()

			_ = m.Ack()
		}

		if len(msgs) < 100 {
			break // No more messages
		}
	}

	return nil
}

// bufferMsgIDCounter is used to ensure unique message IDs
var bufferMsgIDCounter atomic.Int64

// generateBufferMessageID generates a unique message ID for buffered messages
func generateBufferMessageID() string {
	counter := bufferMsgIDCounter.Add(1)
	return fmt.Sprintf("buf-%d-%d", time.Now().UnixNano(), counter)
}

// LeafBufferManager extends LeafNodeManager with buffering support
type LeafBufferManager struct {
	*LeafNodeManager
	buffer *MessageBuffer

	// Auto-flush when connection restored
	autoFlush bool
	flushMu   sync.Mutex
}

// NewLeafBufferManager creates a leaf node manager with buffering
func NewLeafBufferManager(leafConfig *LeafNodeConfig, bufferConfig *BufferConfig) (*LeafBufferManager, error) {
	manager, err := NewLeafNodeManager(leafConfig)
	if err != nil {
		return nil, err
	}

	buffer, err := NewMessageBuffer(bufferConfig)
	if err != nil {
		return nil, err
	}

	lbm := &LeafBufferManager{
		LeafNodeManager: manager,
		buffer:          buffer,
		autoFlush:       true,
	}

	// Set up auto-flush on reconnect
	manager.SetRemoteConnectCallback(func(remote *LeafRemoteConfig) {
		if lbm.autoFlush && lbm.buffer.Len() > 0 {
			go lbm.flushBuffer()
		}
	})

	return lbm, nil
}

// Start starts the leaf buffer manager
func (lbm *LeafBufferManager) Start(ctx context.Context) error {
	if err := lbm.buffer.Start(ctx); err != nil {
		return err
	}

	if err := lbm.LeafNodeManager.Start(ctx); err != nil {
		lbm.buffer.Stop()
		return err
	}

	// Set up JetStream for persistence if available
	if client := lbm.GetClient(); client != nil {
		js, err := client.JetStream()
		if err == nil {
			_ = lbm.buffer.SetJetStream(js)
		}
	}

	return nil
}

// Stop stops the leaf buffer manager
func (lbm *LeafBufferManager) Stop() error {
	if err := lbm.LeafNodeManager.Stop(); err != nil {
		return err
	}
	return lbm.buffer.Stop()
}

// Publish publishes a message, buffering if disconnected
func (lbm *LeafBufferManager) Publish(subject string, data []byte) error {
	client := lbm.GetClient()

	// If connected, publish directly
	if client != nil && client.IsConnected() {
		err := client.Publish(subject, data)
		if err == nil {
			return nil
		}
		// Connection might have dropped, buffer it
	}

	// Buffer the message
	return lbm.buffer.Buffer(&BufferedMessage{
		ID:        generateBufferMessageID(),
		Subject:   subject,
		Data:      data,
		Timestamp: time.Now(),
	})
}

// PublishMsg publishes a NATS message, buffering if disconnected
func (lbm *LeafBufferManager) PublishMsg(msg *nats.Msg) error {
	client := lbm.GetClient()

	// If connected, publish directly
	if client != nil && client.IsConnected() {
		err := client.PublishMsg(msg)
		if err == nil {
			return nil
		}
	}

	// Buffer the message
	return lbm.buffer.BufferNATSMessage(msg)
}

// GetBuffer returns the message buffer
func (lbm *LeafBufferManager) GetBuffer() *MessageBuffer {
	return lbm.buffer
}

// SetAutoFlush enables or disables auto-flush on reconnect
func (lbm *LeafBufferManager) SetAutoFlush(enabled bool) {
	lbm.autoFlush = enabled
}

// FlushBuffer flushes all buffered messages
func (lbm *LeafBufferManager) FlushBuffer() error {
	return lbm.flushBuffer()
}

// flushBuffer performs the actual flush
func (lbm *LeafBufferManager) flushBuffer() error {
	lbm.flushMu.Lock()
	defer lbm.flushMu.Unlock()

	client := lbm.GetClient()
	if client == nil || !client.IsConnected() {
		return errors.New("not connected")
	}

	return lbm.buffer.Flush(client)
}
