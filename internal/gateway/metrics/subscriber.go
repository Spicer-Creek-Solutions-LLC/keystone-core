package metrics

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/nats-io/nats.go"
	dto "github.com/prometheus/client_model/go"
	"github.com/prometheus/common/expfmt"
	"google.golang.org/protobuf/proto"
)

// SubscriberConfig holds configuration for the metrics subscriber.
type SubscriberConfig struct {
	// Subject to subscribe to for metrics (supports wildcards)
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
		Subject:        "kscore.telemetry.metrics.>",
		QueueGroup:     "kscore-gateway",
		BufferSize:     1024,
		ProcessTimeout: 5 * time.Second,
	}
}

// MetricsMessage represents a metrics message from an agent.
type MetricsMessage struct {
	// AgentID is the source agent
	AgentID string `json:"agent_id"`

	// Labels are additional labels for this agent
	Labels map[string]string `json:"labels,omitempty"`

	// Format is the metrics format (prometheus, json, protobuf)
	Format string `json:"format"`

	// Timestamp is when the metrics were collected
	Timestamp time.Time `json:"timestamp"`

	// Data contains the metrics payload
	Data []byte `json:"data"`
}

// Subscriber subscribes to NATS for agent metrics.
type Subscriber struct {
	nc     *nats.Conn
	js     nats.JetStreamContext
	store  *MetricsStore
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
	onMessage func(agentID string, familyCount int)
	onError   func(err error)
}

// NewSubscriber creates a new metrics subscriber.
func NewSubscriber(nc *nats.Conn, store *MetricsStore, config SubscriberConfig) *Subscriber {
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
func (s *Subscriber) SetOnMessage(fn func(agentID string, familyCount int)) {
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
		// Create durable consumer
		s.jsSub, err = s.js.QueueSubscribe(
			s.config.Subject,
			s.config.QueueGroup,
			s.handleMessage,
			nats.ManualAck(),
			nats.AckWait(s.config.ProcessTimeout),
			nats.MaxDeliver(3),
		)
		if err == nil {
			log.Printf("Metrics subscriber started with JetStream on subject: %s", s.config.Subject)
			return nil
		}
		// Fall back to regular NATS
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

	log.Printf("Metrics subscriber started on subject: %s", s.config.Subject)
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

// handleMessage handles an incoming metrics message.
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
	var metricsMsg MetricsMessage
	if err := json.Unmarshal(msg.Data, &metricsMsg); err != nil {
		s.recordError(fmt.Errorf("failed to parse metrics message: %w", err))
		s.nak(msg)
		return
	}

	// Parse metrics based on format
	families, err := s.parseMetrics(metricsMsg.Format, metricsMsg.Data)
	if err != nil {
		s.recordError(fmt.Errorf("failed to parse metrics data: %w", err))
		s.nak(msg)
		return
	}

	// Store metrics
	if err := s.store.Store(metricsMsg.AgentID, metricsMsg.Labels, families); err != nil {
		s.recordError(fmt.Errorf("failed to store metrics: %w", err))
		s.nak(msg)
		return
	}

	// Ack the message
	s.ack(msg)

	// Notify callback
	s.mu.RLock()
	onMessage := s.onMessage
	s.mu.RUnlock()
	if onMessage != nil {
		onMessage(metricsMsg.AgentID, len(families))
	}
}

// parseMetrics parses metrics based on format.
func (s *Subscriber) parseMetrics(format string, data []byte) ([]*dto.MetricFamily, error) {
	switch format {
	case "prometheus", "text", "":
		return s.parsePrometheusText(data)
	case "protobuf":
		return s.parseProtobuf(data)
	case "json":
		return s.parseJSON(data)
	default:
		return nil, fmt.Errorf("unsupported metrics format: %s", format)
	}
}

// parsePrometheusText parses Prometheus text format metrics.
func (s *Subscriber) parsePrometheusText(data []byte) ([]*dto.MetricFamily, error) {
	parser := expfmt.TextParser{}
	families, err := parser.TextToMetricFamilies(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("failed to parse prometheus text: %w", err)
	}

	result := make([]*dto.MetricFamily, 0, len(families))
	for _, f := range families {
		result = append(result, f)
	}
	return result, nil
}

// parseProtobuf parses protobuf format metrics.
func (s *Subscriber) parseProtobuf(data []byte) ([]*dto.MetricFamily, error) {
	var family dto.MetricFamily
	if err := proto.Unmarshal(data, &family); err != nil {
		return nil, fmt.Errorf("failed to parse protobuf: %w", err)
	}
	return []*dto.MetricFamily{&family}, nil
}

// parseJSON parses JSON format metrics.
func (s *Subscriber) parseJSON(data []byte) ([]*dto.MetricFamily, error) {
	var families []*dto.MetricFamily
	if err := json.Unmarshal(data, &families); err != nil {
		return nil, fmt.Errorf("failed to parse json: %w", err)
	}
	return families, nil
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

	log.Printf("Metrics subscriber error: %v", err)

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
