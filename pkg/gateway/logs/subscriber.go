package logs

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/nats-io/nats.go"
)

// SubscriberConfig holds configuration for the logs subscriber.
type SubscriberConfig struct {
	// Subject to subscribe to for logs (supports wildcards)
	Subject string

	// QueueGroup for load balancing across multiple gateways
	QueueGroup string

	// BufferSize for the subscription
	BufferSize int

	// ProcessTimeout is the maximum time to process a message
	ProcessTimeout time.Duration
}

// DefaultSubscriberConfig returns a configuration with sensible defaults.
func DefaultSubscriberConfig() SubscriberConfig {
	return SubscriberConfig{
		Subject:        "kscore.telemetry.logs.>",
		QueueGroup:     "kscore-gateway",
		BufferSize:     1024,
		ProcessTimeout: 5 * time.Second,
	}
}

// LogsMessage represents a logs message from an agent.
type LogsMessage struct {
	// AgentID is the source agent
	AgentID string `json:"agent_id"`

	// Entries are the log entries
	Entries []LogEntryMessage `json:"entries"`
}

// LogEntryMessage represents a single log entry in a message.
type LogEntryMessage struct {
	// Timestamp is when the log was generated
	Timestamp time.Time `json:"timestamp"`

	// Level is the log level (debug, info, warn, error)
	Level string `json:"level"`

	// Source is the log source
	Source string `json:"source"`

	// Message is the log message
	Message string `json:"message"`

	// Labels are additional labels
	Labels map[string]string `json:"labels,omitempty"`

	// Fields are structured fields
	Fields map[string]interface{} `json:"fields,omitempty"`
}

// Subscriber subscribes to NATS for agent logs.
type Subscriber struct {
	nc     *nats.Conn
	js     nats.JetStreamContext
	store  *LogsStore
	config SubscriberConfig

	sub    *nats.Subscription
	jsSub  *nats.Subscription
	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup

	// Stats
	mu             sync.RWMutex
	messagesRecv   int64
	messagesFailed int64
	bytesRecv      int64
	lastError      error
	lastErrorTime  time.Time

	// Callbacks
	onMessage func(agentID string, entryCount int)
	onError   func(err error)
}

// NewSubscriber creates a new logs subscriber.
func NewSubscriber(nc *nats.Conn, store *LogsStore, config SubscriberConfig) *Subscriber {
	ctx, cancel := context.WithCancel(context.Background())
	return &Subscriber{
		nc:     nc,
		store:  store,
		config: config,
		ctx:    ctx,
		cancel: cancel,
	}
}

// SetOnMessage sets the callback for when a message is processed.
func (s *Subscriber) SetOnMessage(fn func(agentID string, entryCount int)) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.onMessage = fn
}

// SetOnError sets the callback for when an error occurs.
func (s *Subscriber) SetOnError(fn func(err error)) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.onError = fn
}

// Start starts the subscriber.
func (s *Subscriber) Start() error {
	var err error

	// Try JetStream first
	s.js, err = s.nc.JetStream()
	if err == nil {
		s.jsSub, err = s.js.QueueSubscribe(
			s.config.Subject,
			s.config.QueueGroup,
			s.handleMessage,
			nats.ManualAck(),
			nats.AckWait(s.config.ProcessTimeout),
			nats.MaxDeliver(3),
		)
		if err == nil {
			log.Printf("Logs subscriber started with JetStream on subject: %s", s.config.Subject)
			return nil
		}
		log.Printf("JetStream subscription failed, falling back to core NATS: %v", err)
	}

	// Fall back to regular NATS subscription
	if s.config.QueueGroup != "" {
		s.sub, err = s.nc.QueueSubscribe(s.config.Subject, s.config.QueueGroup, s.handleMessage)
	} else {
		s.sub, err = s.nc.Subscribe(s.config.Subject, s.handleMessage)
	}
	if err != nil {
		return fmt.Errorf("failed to subscribe: %w", err)
	}

	if err := s.sub.SetPendingLimits(s.config.BufferSize, s.config.BufferSize*1024*1024); err != nil {
		log.Printf("Warning: failed to set pending limits: %v", err)
	}

	log.Printf("Logs subscriber started on subject: %s", s.config.Subject)
	return nil
}

// Stop stops the subscriber.
func (s *Subscriber) Stop() error {
	s.cancel()

	if s.jsSub != nil {
		if err := s.jsSub.Drain(); err != nil {
			log.Printf("Warning: failed to drain JetStream subscription: %v", err)
		}
	}

	if s.sub != nil {
		if err := s.sub.Drain(); err != nil {
			log.Printf("Warning: failed to drain subscription: %v", err)
		}
	}

	s.wg.Wait()
	return nil
}

// handleMessage handles an incoming logs message.
func (s *Subscriber) handleMessage(msg *nats.Msg) {
	s.wg.Add(1)
	defer s.wg.Done()

	select {
	case <-s.ctx.Done():
		return
	default:
	}

	s.mu.Lock()
	s.messagesRecv++
	s.bytesRecv += int64(len(msg.Data))
	s.mu.Unlock()

	// Parse the message
	var logsMsg LogsMessage
	if err := json.Unmarshal(msg.Data, &logsMsg); err != nil {
		s.recordError(fmt.Errorf("failed to parse logs message: %w", err))
		s.nak(msg)
		return
	}

	// Convert and store entries
	stored := 0
	for _, entry := range logsMsg.Entries {
		logEntry := LogEntry{
			ID:        uuid.New().String(),
			Timestamp: entry.Timestamp,
			AgentID:   logsMsg.AgentID,
			Level:     ParseLevel(entry.Level),
			Source:    entry.Source,
			Message:   entry.Message,
			Labels:    entry.Labels,
			Fields:    entry.Fields,
		}

		if logEntry.Timestamp.IsZero() {
			logEntry.Timestamp = time.Now()
		}

		if s.store.Store(logEntry) {
			stored++
		}
	}

	// Ack the message
	s.ack(msg)

	// Notify callback
	s.mu.RLock()
	onMessage := s.onMessage
	s.mu.RUnlock()
	if onMessage != nil && stored > 0 {
		onMessage(logsMsg.AgentID, stored)
	}
}

// ack acknowledges a message.
func (s *Subscriber) ack(msg *nats.Msg) {
	if msg.Reply != "" {
		_ = msg.Ack()
	}
}

// nak negative-acknowledges a message.
func (s *Subscriber) nak(msg *nats.Msg) {
	s.mu.Lock()
	s.messagesFailed++
	s.mu.Unlock()

	if msg.Reply != "" {
		_ = msg.Nak()
	}
}

// recordError records an error.
func (s *Subscriber) recordError(err error) {
	s.mu.Lock()
	s.lastError = err
	s.lastErrorTime = time.Now()
	onError := s.onError
	s.mu.Unlock()

	log.Printf("Logs subscriber error: %v", err)

	if onError != nil {
		onError(err)
	}
}

// SubscriberStats holds subscriber statistics.
type SubscriberStats struct {
	MessagesReceived int64
	MessagesFailed   int64
	BytesReceived    int64
	LastError        error
	LastErrorTime    time.Time
}

// Stats returns subscriber statistics.
func (s *Subscriber) Stats() SubscriberStats {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return SubscriberStats{
		MessagesReceived: s.messagesRecv,
		MessagesFailed:   s.messagesFailed,
		BytesReceived:    s.bytesRecv,
		LastError:        s.lastError,
		LastErrorTime:    s.lastErrorTime,
	}
}
