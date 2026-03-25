// Package tracing provides NATS trace exporter for centralized trace collection.
// Epic 15: Observability Enhancements - NATS telemetry transport.
package tracing

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/nats-io/nats.go"
)

// NATSExporterConfig configures the NATS trace exporter.
type NATSExporterConfig struct {
	// URL is the NATS server URL (e.g., "nats://localhost:4222")
	URL string

	// Subject is the base subject for traces
	// Default: "kscore.traces"
	Subject string

	// SubjectPerService uses separate subjects per service
	// e.g., kscore.traces.kscore-server
	SubjectPerService bool

	// ServiceName identifies the trace source
	ServiceName string

	// BatchSize is the number of spans to batch before publishing
	// Default: 100
	BatchSize int

	// FlushInterval is how often to flush spans
	// Default: 5s
	FlushInterval time.Duration

	// BufferSize is the number of spans to buffer
	// Default: 10000
	BufferSize int

	// ConnectTimeout is the timeout for connecting to NATS
	// Default: 5s
	ConnectTimeout time.Duration

	// ReconnectWait is the wait time between reconnection attempts
	// Default: 1s
	ReconnectWait time.Duration

	// MaxReconnects is the maximum number of reconnection attempts
	// Default: -1 (unlimited)
	MaxReconnects int

	// Credentials for authentication
	Token    string
	User     string
	Password string
	NKeyFile string
	CredFile string
}

// DefaultNATSExporterConfig returns a default NATS exporter configuration.
func DefaultNATSExporterConfig() *NATSExporterConfig {
	return &NATSExporterConfig{
		URL:               "nats://localhost:4222",
		Subject:           "kscore.traces",
		SubjectPerService: false,
		BatchSize:         100,
		FlushInterval:     5 * time.Second,
		BufferSize:        10000,
		ConnectTimeout:    5 * time.Second,
		ReconnectWait:     1 * time.Second,
		MaxReconnects:     -1,
	}
}

// NATSSpan represents a trace span sent over NATS.
type NATSSpan struct {
	// Trace identification
	TraceID      string `json:"trace_id"`
	SpanID       string `json:"span_id"`
	ParentSpanID string `json:"parent_span_id,omitempty"`

	// Span info
	Name      string `json:"name"`
	Kind      string `json:"kind"` // internal, server, client, producer, consumer
	StartTime string `json:"start_time"`
	EndTime   string `json:"end_time"`
	Duration  int64  `json:"duration_ns"`

	// Status
	Status    string `json:"status"` // unset, ok, error
	StatusMsg string `json:"status_message,omitempty"`

	// Attributes
	Attributes map[string]interface{} `json:"attributes,omitempty"`

	// Events
	Events []NATSSpanEvent `json:"events,omitempty"`

	// Links
	Links []NATSSpanLink `json:"links,omitempty"`

	// Resource info
	Service string `json:"service,omitempty"`
	Host    string `json:"host,omitempty"`
	Version string `json:"version,omitempty"`
}

// NATSSpanEvent represents a span event.
type NATSSpanEvent struct {
	Name       string                 `json:"name"`
	Timestamp  string                 `json:"timestamp"`
	Attributes map[string]interface{} `json:"attributes,omitempty"`
}

// NATSSpanLink represents a link to another span.
type NATSSpanLink struct {
	TraceID    string                 `json:"trace_id"`
	SpanID     string                 `json:"span_id"`
	Attributes map[string]interface{} `json:"attributes,omitempty"`
}

// NATSSpanBatch is a batch of spans sent together.
type NATSSpanBatch struct {
	Spans     []NATSSpan `json:"spans"`
	Timestamp string     `json:"timestamp"`
	Service   string     `json:"service"`
	Host      string     `json:"host"`
}

// NATSSpanExporter exports spans to NATS.
type NATSSpanExporter struct {
	config   *NATSExporterConfig
	conn     *nats.Conn
	spans    chan *NATSSpan
	batch    []*NATSSpan
	mu       sync.Mutex
	closed   bool
	hostname string
	stopCh   chan struct{}
	doneCh   chan struct{}

	// Stats
	spansExported int64
	spansDropped  int64
	batchesSent   int64
	lastError     error
	lastErrorTime time.Time
}

// NewNATSSpanExporter creates a new NATS span exporter.
func NewNATSSpanExporter(config *NATSExporterConfig) (*NATSSpanExporter, error) {
	if config == nil {
		config = DefaultNATSExporterConfig()
	}

	// Build NATS options
	opts := []nats.Option{
		nats.Name("kscore-traces-" + config.ServiceName),
		nats.Timeout(config.ConnectTimeout),
		nats.ReconnectWait(config.ReconnectWait),
		nats.MaxReconnects(config.MaxReconnects),
	}

	// Add authentication
	if config.Token != "" {
		opts = append(opts, nats.Token(config.Token))
	}
	if config.User != "" && config.Password != "" {
		opts = append(opts, nats.UserInfo(config.User, config.Password))
	}
	if config.NKeyFile != "" {
		opt, err := nats.NkeyOptionFromSeed(config.NKeyFile)
		if err != nil {
			return nil, fmt.Errorf("failed to load NKey: %w", err)
		}
		opts = append(opts, opt)
	}
	if config.CredFile != "" {
		opts = append(opts, nats.UserCredentials(config.CredFile))
	}

	// Connect to NATS
	conn, err := nats.Connect(config.URL, opts...)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to NATS: %w", err)
	}

	bufferSize := config.BufferSize
	if bufferSize <= 0 {
		bufferSize = 10000
	}

	batchSize := config.BatchSize
	if batchSize <= 0 {
		batchSize = 100
	}

	exporter := &NATSSpanExporter{
		config: config,
		conn:   conn,
		spans:  make(chan *NATSSpan, bufferSize),
		batch:  make([]*NATSSpan, 0, batchSize),
		stopCh: make(chan struct{}),
		doneCh: make(chan struct{}),
	}

	// Start the export loop
	go exporter.exportLoop()

	return exporter, nil
}

// ExportSpan exports a single span. The lock is held across the closed
// check and the channel send to prevent a race with Shutdown closing
// the spans channel between the check and the send.
func (e *NATSSpanExporter) ExportSpan(span *NATSSpan) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	if e.closed {
		return fmt.Errorf("exporter is closed")
	}

	select {
	case e.spans <- span:
		return nil
	default:
		e.spansDropped++
		return fmt.Errorf("span buffer full, span dropped")
	}
}

// ExportSpans exports multiple spans.
func (e *NATSSpanExporter) ExportSpans(ctx context.Context, spans []NATSSpan) error {
	for i := range spans {
		if err := e.ExportSpan(&spans[i]); err != nil {
			return err
		}
	}
	return nil
}

// Flush forces a flush of buffered spans.
func (e *NATSSpanExporter) Flush(ctx context.Context) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	if len(e.batch) > 0 {
		return e.sendBatch()
	}
	return nil
}

// exportLoop processes spans and sends batches.
func (e *NATSSpanExporter) exportLoop() {
	defer close(e.doneCh)

	ticker := time.NewTicker(e.config.FlushInterval)
	defer ticker.Stop()

	for {
		select {
		case span, ok := <-e.spans:
			if !ok {
				// Channel closed, flush remaining
				e.mu.Lock()
				if len(e.batch) > 0 {
					_ = e.sendBatch()
				}
				e.mu.Unlock()
				return
			}

			e.mu.Lock()
			e.batch = append(e.batch, span)

			if len(e.batch) >= e.config.BatchSize {
				_ = e.sendBatch()
			}
			e.mu.Unlock()

		case <-ticker.C:
			e.mu.Lock()
			if len(e.batch) > 0 {
				_ = e.sendBatch()
			}
			e.mu.Unlock()

		case <-e.stopCh:
			// Flush remaining
			e.mu.Lock()
			if len(e.batch) > 0 {
				_ = e.sendBatch()
			}
			e.mu.Unlock()
			return
		}
	}
}

// sendBatch sends the current batch to NATS.
// Must be called with lock held.
func (e *NATSSpanExporter) sendBatch() error {
	if len(e.batch) == 0 {
		return nil
	}

	if e.conn == nil || !e.conn.IsConnected() {
		e.lastError = fmt.Errorf("NATS not connected")
		e.lastErrorTime = time.Now()
		return e.lastError
	}

	// Build batch
	spans := make([]NATSSpan, len(e.batch))
	for i, s := range e.batch {
		spans[i] = *s
	}

	batch := NATSSpanBatch{
		Spans:     spans,
		Timestamp: time.Now().Format(time.RFC3339Nano),
		Service:   e.config.ServiceName,
		Host:      e.hostname,
	}

	data, err := json.Marshal(batch)
	if err != nil {
		e.lastError = err
		e.lastErrorTime = time.Now()
		return err
	}

	subject := e.buildSubject()
	if err := e.conn.Publish(subject, data); err != nil {
		e.lastError = err
		e.lastErrorTime = time.Now()
		return err
	}

	e.spansExported += int64(len(e.batch))
	e.batchesSent++

	// Clear batch
	e.batch = e.batch[:0]

	return nil
}

// buildSubject builds the NATS subject for traces.
func (e *NATSSpanExporter) buildSubject() string {
	subject := e.config.Subject
	if e.config.SubjectPerService && e.config.ServiceName != "" {
		subject = fmt.Sprintf("%s.%s", subject, e.config.ServiceName)
	}
	return subject
}

// Stats returns exporter statistics.
func (e *NATSSpanExporter) Stats() (exported, dropped, batches int64, lastErrTime time.Time, lastErr error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.spansExported, e.spansDropped, e.batchesSent, e.lastErrorTime, e.lastError
}

// IsConnected returns whether NATS is connected.
func (e *NATSSpanExporter) IsConnected() bool {
	return e.conn != nil && e.conn.IsConnected()
}

// Shutdown gracefully shuts down the exporter.
func (e *NATSSpanExporter) Shutdown(ctx context.Context) error {
	e.mu.Lock()
	if e.closed {
		e.mu.Unlock()
		return nil
	}
	e.closed = true
	e.mu.Unlock()

	// Signal stop
	close(e.stopCh)

	// Wait for export loop to finish or context to expire
	select {
	case <-e.doneCh:
	case <-ctx.Done():
		return ctx.Err()
	}

	// Close spans channel
	close(e.spans)

	// Close NATS connection
	if e.conn != nil {
		e.conn.Drain()
		e.conn.Close()
	}

	return nil
}

// NATSSpanSubscriber subscribes to NATS span messages.
type NATSSpanSubscriber struct {
	conn    *nats.Conn
	sub     *nats.Subscription
	handler func(*NATSSpanBatch)
	mu      sync.Mutex
}

// NewNATSSpanSubscriber creates a subscriber for NATS spans.
func NewNATSSpanSubscriber(conn *nats.Conn, subject string, handler func(*NATSSpanBatch)) (*NATSSpanSubscriber, error) {
	s := &NATSSpanSubscriber{
		conn:    conn,
		handler: handler,
	}

	sub, err := conn.Subscribe(subject, func(msg *nats.Msg) {
		var batch NATSSpanBatch
		if err := json.Unmarshal(msg.Data, &batch); err != nil {
			return
		}
		if handler != nil {
			handler(&batch)
		}
	})
	if err != nil {
		return nil, fmt.Errorf("failed to subscribe: %w", err)
	}

	s.sub = sub
	return s, nil
}

// Unsubscribe unsubscribes from NATS.
func (s *NATSSpanSubscriber) Unsubscribe() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.sub != nil {
		return s.sub.Unsubscribe()
	}
	return nil
}

// Close closes the subscriber.
func (s *NATSSpanSubscriber) Close() error {
	return s.Unsubscribe()
}

// Subscription returns the underlying NATS subscription.
func (s *NATSSpanSubscriber) Subscription() *nats.Subscription {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.sub
}

// SpanBuilder helps construct NATSSpan objects.
type SpanBuilder struct {
	span NATSSpan
}

// NewSpanBuilder creates a new span builder.
func NewSpanBuilder(traceID, spanID, name string) *SpanBuilder {
	return &SpanBuilder{
		span: NATSSpan{
			TraceID:    traceID,
			SpanID:     spanID,
			Name:       name,
			Kind:       "internal",
			Status:     "unset",
			Attributes: make(map[string]interface{}),
		},
	}
}

// WithParentSpanID sets the parent span ID.
func (b *SpanBuilder) WithParentSpanID(id string) *SpanBuilder {
	b.span.ParentSpanID = id
	return b
}

// WithKind sets the span kind.
func (b *SpanBuilder) WithKind(kind string) *SpanBuilder {
	b.span.Kind = kind
	return b
}

// WithStartTime sets the start time.
func (b *SpanBuilder) WithStartTime(t time.Time) *SpanBuilder {
	b.span.StartTime = t.Format(time.RFC3339Nano)
	return b
}

// WithEndTime sets the end time and calculates duration.
func (b *SpanBuilder) WithEndTime(t time.Time) *SpanBuilder {
	b.span.EndTime = t.Format(time.RFC3339Nano)
	if b.span.StartTime != "" {
		startTime, err := time.Parse(time.RFC3339Nano, b.span.StartTime)
		if err == nil {
			b.span.Duration = t.Sub(startTime).Nanoseconds()
		}
	}
	return b
}

// WithDuration sets the duration in nanoseconds.
func (b *SpanBuilder) WithDuration(d time.Duration) *SpanBuilder {
	b.span.Duration = d.Nanoseconds()
	return b
}

// WithStatus sets the span status.
func (b *SpanBuilder) WithStatus(status, message string) *SpanBuilder {
	b.span.Status = status
	b.span.StatusMsg = message
	return b
}

// WithAttribute adds an attribute.
func (b *SpanBuilder) WithAttribute(key string, value interface{}) *SpanBuilder {
	b.span.Attributes[key] = value
	return b
}

// WithEvent adds an event.
func (b *SpanBuilder) WithEvent(name string, timestamp time.Time, attrs map[string]interface{}) *SpanBuilder {
	event := NATSSpanEvent{
		Name:       name,
		Timestamp:  timestamp.Format(time.RFC3339Nano),
		Attributes: attrs,
	}
	b.span.Events = append(b.span.Events, event)
	return b
}

// WithLink adds a link to another span.
func (b *SpanBuilder) WithLink(traceID, spanID string, attrs map[string]interface{}) *SpanBuilder {
	link := NATSSpanLink{
		TraceID:    traceID,
		SpanID:     spanID,
		Attributes: attrs,
	}
	b.span.Links = append(b.span.Links, link)
	return b
}

// WithService sets the service name.
func (b *SpanBuilder) WithService(service string) *SpanBuilder {
	b.span.Service = service
	return b
}

// WithHost sets the host name.
func (b *SpanBuilder) WithHost(host string) *SpanBuilder {
	b.span.Host = host
	return b
}

// WithVersion sets the version.
func (b *SpanBuilder) WithVersion(version string) *SpanBuilder {
	b.span.Version = version
	return b
}

// Build returns the constructed span.
func (b *SpanBuilder) Build() *NATSSpan {
	return &b.span
}
