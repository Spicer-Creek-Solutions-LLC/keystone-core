package events

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/shawnbutts/keystone-core/internal/tracing"
	"github.com/shawnbutts/keystone-core/pkg/wait"
)

const (
	// StreamName is the JetStream stream name for Keystone Core events
	StreamName = "KSCORE_EVENTS"

	// StreamSubjects defines the subject pattern for all events
	StreamSubjects = "kscore.events.>"

	// SubjectPrefix is the prefix for all event subjects
	SubjectPrefix = "kscore.events"
)

// JetStreamPublisherConfig holds configuration for the JetStream event publisher.
type JetStreamPublisherConfig struct {
	// Retention is the maximum age of stored events.
	Retention time.Duration
	// MaxBytes is the maximum total size of stored events in bytes.
	MaxBytes int64
	// MaxMessages is the maximum number of stored events.
	MaxMessages int64
}

// DefaultJetStreamPublisherConfig returns the default publisher configuration.
func DefaultJetStreamPublisherConfig() *JetStreamPublisherConfig {
	return &JetStreamPublisherConfig{
		Retention:   7 * 24 * time.Hour,
		MaxBytes:    10 * 1024 * 1024 * 1024, // 10GB
		MaxMessages: 1000000,
	}
}

// JetStreamPublisher implements EventPublisher using NATS JetStream
type JetStreamPublisher struct {
	js     nats.JetStreamContext
	config *JetStreamPublisherConfig
	mu     sync.RWMutex
	ctx    context.Context
	cancel context.CancelFunc
	closed bool
}

// NewJetStreamPublisher creates a new JetStream-based event publisher with default configuration.
func NewJetStreamPublisher(js nats.JetStreamContext) (*JetStreamPublisher, error) {
	return NewJetStreamPublisherWithConfig(js, nil)
}

// NewJetStreamPublisherWithConfig creates a new JetStream-based event publisher with custom configuration.
func NewJetStreamPublisherWithConfig(js nats.JetStreamContext, cfg *JetStreamPublisherConfig) (*JetStreamPublisher, error) {
	if js == nil {
		return nil, fmt.Errorf("JetStream context is required")
	}

	if cfg == nil {
		cfg = DefaultJetStreamPublisherConfig()
	}

	ctx, cancel := context.WithCancel(context.Background())

	p := &JetStreamPublisher{
		js:     js,
		config: cfg,
		ctx:    ctx,
		cancel: cancel,
	}

	// Ensure the event stream exists
	if err := p.ensureStream(); err != nil {
		cancel()
		return nil, fmt.Errorf("failed to ensure event stream: %w", err)
	}

	return p, nil
}

// ensureStream creates the event stream if it doesn't exist
func (p *JetStreamPublisher) ensureStream() error {
	// Check if stream already exists
	stream, err := p.js.StreamInfo(StreamName)
	if err == nil && stream != nil {
		// Stream exists, verify configuration
		return nil
	}

	// Create the stream using configuration values
	streamConfig := &nats.StreamConfig{
		Name:        StreamName,
		Description: "Keystone Core event stream",
		Subjects:    []string{StreamSubjects},
		Retention:   nats.LimitsPolicy,
		MaxAge:      p.config.Retention,
		MaxBytes:    p.config.MaxBytes,
		MaxMsgs:     p.config.MaxMessages,
		Storage:     nats.FileStorage,
		Replicas:    1,
		Discard:     nats.DiscardOld,
	}

	_, err = p.js.AddStream(streamConfig)
	if err != nil {
		return fmt.Errorf("failed to create stream: %w", err)
	}

	return nil
}

// Publish publishes an event synchronously
func (p *JetStreamPublisher) Publish(event *Event) error {
	p.mu.RLock()
	defer p.mu.RUnlock()

	if p.closed {
		return fmt.Errorf("publisher is closed")
	}

	if event == nil {
		return fmt.Errorf("event is nil")
	}

	// Start tracing span
	ctx, span := tracing.StartEventSpan(p.ctx, tracing.SpanEventPublish,
		tracing.StringAttr(tracing.AttrEventType, string(event.Type)),
		tracing.StringAttr("event.source", event.Source),
		tracing.StringAttr("event.severity", string(event.Severity)),
	)
	defer span.End()

	// Add correlation ID if present
	if event.CorrelationID != "" {
		tracing.SetAttributes(span, tracing.StringAttr("event.correlation_id", event.CorrelationID))
	}

	// Set subject if not already set
	if event.Subject == "" {
		event.Subject = buildSubject(event.Type)
	}

	// Serialize event to JSON
	data, err := json.Marshal(event)
	if err != nil {
		tracing.RecordError(span, err)
		return fmt.Errorf("failed to marshal event: %w", err)
	}

	// Publish to JetStream
	subject := fmt.Sprintf("%s.%s", SubjectPrefix, event.Subject)
	tracing.SetAttributes(span, tracing.StringAttr("event.subject", subject))

	_, err = p.js.Publish(subject, data)
	if err != nil {
		tracing.RecordError(span, err)
		return fmt.Errorf("failed to publish event to %s: %w", subject, err)
	}

	tracing.RecordSuccess(span, "event published successfully")
	// Extract context for potential propagation
	_ = ctx
	return nil
}

// PublishAsync publishes an event asynchronously
func (p *JetStreamPublisher) PublishAsync(event *Event) error {
	p.mu.RLock()
	defer p.mu.RUnlock()

	if p.closed {
		return fmt.Errorf("publisher is closed")
	}

	if event == nil {
		return fmt.Errorf("event is nil")
	}

	// Start tracing span
	ctx, span := tracing.StartEventSpan(p.ctx, "event.publish.async",
		tracing.StringAttr(tracing.AttrEventType, string(event.Type)),
		tracing.StringAttr("event.source", event.Source),
		tracing.StringAttr("event.severity", string(event.Severity)),
	)
	defer span.End()

	// Add correlation ID if present
	if event.CorrelationID != "" {
		tracing.SetAttributes(span, tracing.StringAttr("event.correlation_id", event.CorrelationID))
	}

	// Set subject if not already set
	if event.Subject == "" {
		event.Subject = buildSubject(event.Type)
	}

	// Serialize event to JSON
	data, err := json.Marshal(event)
	if err != nil {
		tracing.RecordError(span, err)
		return fmt.Errorf("failed to marshal event: %w", err)
	}

	// Publish asynchronously to JetStream
	subject := fmt.Sprintf("%s.%s", SubjectPrefix, event.Subject)
	tracing.SetAttributes(span, tracing.StringAttr("event.subject", subject))

	_, err = p.js.PublishAsync(subject, data)
	if err != nil {
		tracing.RecordError(span, err)
		return fmt.Errorf("failed to publish event asynchronously to %s: %w", subject, err)
	}

	tracing.RecordSuccess(span, "event queued for async publish")
	// Extract context for potential propagation
	_ = ctx

	return nil
}

// Close closes the publisher and waits for pending async publishes
func (p *JetStreamPublisher) Close() error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.closed {
		return nil
	}

	p.closed = true

	// Wait for any pending async publishes to complete
	if wait.ForSignal(p.js.PublishAsyncComplete(), 5*time.Second) {
		return fmt.Errorf("timeout waiting for async publishes to complete")
	}

	p.cancel()
	return nil
}

// buildSubject constructs a NATS subject from an event type
// Example: "agent.connect" -> "agent.connect"
// The full subject will be: "kscore.events.agent.connect"
func buildSubject(eventType EventType) string {
	return string(eventType)
}
