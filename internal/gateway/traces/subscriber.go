package traces

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/nats-io/nats.go"
)

// SubscriberConfig holds configuration for the traces subscriber.
type SubscriberConfig struct {
	// Subject to subscribe to for traces (supports wildcards)
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
		Subject:        "kscore.telemetry.traces.>",
		QueueGroup:     "kscore-gateway",
		BufferSize:     1024,
		ProcessTimeout: 5 * time.Second,
	}
}

// TracesMessage represents a traces message from an agent.
type TracesMessage struct {
	// AgentID is the source agent
	AgentID string `json:"agent_id"`

	// Spans are the spans in this message
	Spans []SpanMessage `json:"spans"`
}

// SpanMessage represents a span in a message.
type SpanMessage struct {
	// TraceID is the trace identifier
	TraceID string `json:"trace_id"`

	// SpanID is the span identifier
	SpanID string `json:"span_id"`

	// ParentSpanID is the parent span
	ParentSpanID string `json:"parent_span_id,omitempty"`

	// OperationName is the operation name
	OperationName string `json:"operation_name"`

	// ServiceName is the service name
	ServiceName string `json:"service_name"`

	// StartTime is when the span started
	StartTime time.Time `json:"start_time"`

	// EndTime is when the span ended
	EndTime time.Time `json:"end_time"`

	// Duration is the span duration in nanoseconds
	DurationNanos int64 `json:"duration_nanos,omitempty"`

	// Status is the span status (0=unset, 1=ok, 2=error)
	Status int `json:"status"`

	// Attributes are span attributes
	Attributes map[string]interface{} `json:"attributes,omitempty"`

	// Events are span events
	Events []SpanEventMessage `json:"events,omitempty"`
}

// SpanEventMessage represents a span event in a message.
type SpanEventMessage struct {
	Name       string                 `json:"name"`
	Timestamp  time.Time              `json:"timestamp"`
	Attributes map[string]interface{} `json:"attributes,omitempty"`
}

// Subscriber subscribes to NATS for agent traces.
type Subscriber struct {
	nc     *nats.Conn
	js     nats.JetStreamContext
	store  *TracesStore
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
	onMessage func(agentID string, spanCount int)
	onError   func(err error)
}

// NewSubscriber creates a new traces subscriber.
func NewSubscriber(nc *nats.Conn, store *TracesStore, config SubscriberConfig) *Subscriber {
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
func (s *Subscriber) SetOnMessage(fn func(agentID string, spanCount int)) {
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
			log.Printf("Traces subscriber started with JetStream on subject: %s", s.config.Subject)
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

	log.Printf("Traces subscriber started on subject: %s", s.config.Subject)
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

// handleMessage handles an incoming traces message.
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
	var tracesMsg TracesMessage
	if err := json.Unmarshal(msg.Data, &tracesMsg); err != nil {
		s.recordError(fmt.Errorf("failed to parse traces message: %w", err))
		s.nak(msg)
		return
	}

	// Convert and store spans
	spans := make([]Span, 0, len(tracesMsg.Spans))
	for _, sm := range tracesMsg.Spans {
		span := Span{
			TraceID:       sm.TraceID,
			SpanID:        sm.SpanID,
			ParentSpanID:  sm.ParentSpanID,
			OperationName: sm.OperationName,
			ServiceName:   sm.ServiceName,
			StartTime:     sm.StartTime,
			EndTime:       sm.EndTime,
			Status:        SpanStatus(sm.Status),
			Attributes:    sm.Attributes,
			AgentID:       tracesMsg.AgentID,
		}

		// Calculate duration
		if sm.DurationNanos > 0 {
			span.Duration = time.Duration(sm.DurationNanos)
		} else if !sm.StartTime.IsZero() && !sm.EndTime.IsZero() {
			span.Duration = sm.EndTime.Sub(sm.StartTime)
		}

		// Convert events
		if len(sm.Events) > 0 {
			span.Events = make([]SpanEvent, len(sm.Events))
			for i, e := range sm.Events {
				span.Events[i] = SpanEvent{
					Name:       e.Name,
					Timestamp:  e.Timestamp,
					Attributes: e.Attributes,
				}
			}
		}

		spans = append(spans, span)
	}

	stored := s.store.Store(spans)

	// Ack the message
	s.ack(msg)

	// Notify callback
	s.mu.RLock()
	onMessage := s.onMessage
	s.mu.RUnlock()
	if onMessage != nil && stored > 0 {
		onMessage(tracesMsg.AgentID, stored)
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

	log.Printf("Traces subscriber error: %v", err)

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
