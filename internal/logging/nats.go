// Package logging provides NATS log transport for centralized log collection.
// Epic 15: Observability Enhancements - NATS telemetry transport.
package logging

import (
	"encoding/json"
	"fmt"
	"log"
	"sync"
	"sync/atomic"
	"time"

	"github.com/nats-io/nats.go"
)

// NATSOutputConfig configures the NATS log transport.
type NATSOutputConfig struct {
	// URL is the NATS server URL (e.g., "nats://localhost:4222")
	URL string

	// Subject is the base subject for log messages
	// Default: "kscore.logs"
	Subject string

	// SubjectPerLevel uses separate subjects per log level
	// e.g., kscore.logs.info, kscore.logs.error
	SubjectPerLevel bool

	// SubjectPerService uses separate subjects per service
	// e.g., kscore.logs.kscore-server, kscore.logs.kscore-agent
	SubjectPerService bool

	// ServiceName identifies the logging service
	ServiceName string

	// BufferSize is the number of messages to buffer if NATS is disconnected
	// Default: 1000
	BufferSize int

	// FlushInterval is how often to flush buffered messages
	// Default: 100ms
	FlushInterval time.Duration

	// ConnectTimeout is the timeout for connecting to NATS
	// Default: 5s
	ConnectTimeout time.Duration

	// ReconnectWait is the wait time between reconnection attempts
	// Default: 1s
	ReconnectWait time.Duration

	// MaxReconnects is the maximum number of reconnection attempts
	// Default: -1 (unlimited)
	MaxReconnects int

	// Async enables asynchronous publishing (non-blocking)
	// Default: true
	Async bool

	// IncludeRaw includes raw JSON in addition to structured fields
	IncludeRaw bool

	// Credentials for authentication
	Token    string
	User     string
	Password string
	NKeyFile string
	CredFile string
}

// DefaultNATSOutputConfig returns a default NATS output configuration.
func DefaultNATSOutputConfig() *NATSOutputConfig {
	return &NATSOutputConfig{
		URL:               "nats://localhost:4222",
		Subject:           "kscore.logs",
		SubjectPerLevel:   false,
		SubjectPerService: false,
		BufferSize:        1000,
		FlushInterval:     100 * time.Millisecond,
		ConnectTimeout:    5 * time.Second,
		ReconnectWait:     1 * time.Second,
		MaxReconnects:     -1,
		Async:             true,
	}
}

// NATSLogMessage is the structure sent over NATS.
type NATSLogMessage struct {
	// Standard log fields
	Timestamp     string                 `json:"timestamp"`
	Level         string                 `json:"level"`
	Logger        string                 `json:"logger"`
	Message       string                 `json:"message"`
	CorrelationID string                 `json:"correlation_id,omitempty"`
	TraceID       string                 `json:"trace_id,omitempty"`
	SpanID        string                 `json:"span_id,omitempty"`
	Caller        string                 `json:"caller,omitempty"`
	Fields        map[string]interface{} `json:"fields,omitempty"`

	// Metadata
	Host    string `json:"host,omitempty"`
	Service string `json:"service,omitempty"`
	Version string `json:"version,omitempty"`
	PID     int    `json:"pid,omitempty"`

	// Raw JSON if IncludeRaw is enabled
	Raw string `json:"raw,omitempty"`
}

// NATSOutput implements Output for NATS log transport.
type NATSOutput struct {
	config    *NATSOutputConfig
	conn      *nats.Conn
	formatter Formatter
	buffer    chan []byte
	mu        sync.RWMutex
	closed    bool

	// Stats (atomic for lock-free access on hot path)
	messagesSent    atomic.Int64
	messagesDropped atomic.Int64
	lastError       error
	lastErrorTime   time.Time
}

// NewNATSOutput creates a new NATS log output.
func NewNATSOutput(config *NATSOutputConfig) (*NATSOutput, error) {
	if config == nil {
		config = DefaultNATSOutputConfig()
	}

	// Build NATS options
	opts := []nats.Option{
		nats.Name("kscore-logger-" + config.ServiceName),
		nats.Timeout(config.ConnectTimeout),
		nats.ReconnectWait(config.ReconnectWait),
		nats.MaxReconnects(config.MaxReconnects),
		nats.DisconnectErrHandler(func(nc *nats.Conn, err error) {
			// Log disconnection (but avoid circular logging)
		}),
		nats.ReconnectHandler(func(nc *nats.Conn) {
			// Log reconnection
		}),
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
		bufferSize = 1000
	}

	output := &NATSOutput{
		config:    config,
		conn:      conn,
		formatter: &JSONFormatter{},
		buffer:    make(chan []byte, bufferSize),
	}

	// Start buffer flusher if async
	if config.Async {
		go output.flushLoop()
	}

	return output, nil
}

// Write sends log data to NATS.
func (n *NATSOutput) Write(data []byte) error {
	n.mu.RLock()
	if n.closed {
		n.mu.RUnlock()
		return fmt.Errorf("NATS output is closed")
	}
	n.mu.RUnlock()

	if n.config.Async {
		// Try to buffer the message
		select {
		case n.buffer <- data:
			return nil
		default:
			// Buffer full, drop message
			dropped := n.messagesDropped.Add(1)
			if dropped%100 == 1 {
				log.Printf("WARN: NATS log buffer full, messages dropped (total: %d)", dropped)
			}
			return fmt.Errorf("NATS buffer full, message dropped")
		}
	}

	// Synchronous publish
	return n.publish(data)
}

// publish sends data to NATS.
func (n *NATSOutput) publish(data []byte) error {
	if n.conn == nil || !n.conn.IsConnected() {
		return fmt.Errorf("NATS not connected")
	}

	// Parse the log entry to determine subject
	var entry map[string]interface{}
	if err := json.Unmarshal(data, &entry); err != nil {
		// Fall back to base subject
		return n.conn.Publish(n.config.Subject, data)
	}

	subject := n.buildSubject(entry)

	err := n.conn.Publish(subject, data)
	if err != nil {
		n.mu.Lock()
		n.lastError = err
		n.lastErrorTime = time.Now()
		n.mu.Unlock()
		return err
	}

	n.messagesSent.Add(1)

	return nil
}

// buildSubject builds the NATS subject based on configuration.
func (n *NATSOutput) buildSubject(entry map[string]interface{}) string {
	subject := n.config.Subject

	if n.config.SubjectPerService && n.config.ServiceName != "" {
		subject = fmt.Sprintf("%s.%s", subject, n.config.ServiceName)
	}

	if n.config.SubjectPerLevel {
		if level, ok := entry["level"].(string); ok {
			subject = fmt.Sprintf("%s.%s", subject, level)
		}
	}

	return subject
}

// flushLoop periodically flushes the buffer.
func (n *NATSOutput) flushLoop() {
	ticker := time.NewTicker(n.config.FlushInterval)
	defer ticker.Stop()

	for {
		select {
		case data, ok := <-n.buffer:
			if !ok {
				return
			}
			_ = n.publish(data)

		case <-ticker.C:
			// Drain buffer
			for {
				select {
				case data := <-n.buffer:
					_ = n.publish(data)
				default:
					goto done
				}
			}
		done:
		}
	}
}

// Stats returns output statistics.
func (n *NATSOutput) Stats() (sent, dropped int64, lastErrTime time.Time, lastErr error) {
	n.mu.RLock()
	defer n.mu.RUnlock()
	return n.messagesSent.Load(), n.messagesDropped.Load(), n.lastErrorTime, n.lastError
}

// IsConnected returns whether NATS is connected.
func (n *NATSOutput) IsConnected() bool {
	return n.conn != nil && n.conn.IsConnected()
}

// Close closes the NATS output.
func (n *NATSOutput) Close() error {
	n.mu.Lock()
	if n.closed {
		n.mu.Unlock()
		return nil
	}
	n.closed = true
	n.mu.Unlock()

	// Close buffer channel
	close(n.buffer)

	// Drain remaining messages
	for data := range n.buffer {
		_ = n.publish(data)
	}

	// Close NATS connection
	if n.conn != nil {
		n.conn.Drain()
		n.conn.Close()
	}

	return nil
}

// NATSFormatter formats log entries for NATS transport.
type NATSFormatter struct {
	ServiceName string
	IncludeRaw  bool
}

// Format formats a log entry for NATS transport.
func (f *NATSFormatter) Format(entry *Entry) ([]byte, error) {
	msg := NATSLogMessage{
		Timestamp:     entry.Timestamp.Format(time.RFC3339Nano),
		Level:         entry.Level.String(),
		Logger:        entry.Logger,
		Message:       entry.Message,
		CorrelationID: entry.CorrelationID,
		Service:       f.ServiceName,
	}

	// Extract caller from metadata if available
	if entry.Metadata != nil {
		msg.Caller = entry.Metadata.Caller
	}

	// Copy fields
	if len(entry.Fields) > 0 {
		msg.Fields = make(map[string]interface{})
		for k, v := range entry.Fields {
			msg.Fields[k] = v
		}
	}

	// Add metadata
	if entry.Metadata != nil {
		msg.Host = entry.Metadata.Host
		msg.PID = entry.Metadata.PID
		msg.Version = entry.Metadata.Version
		if msg.Service == "" {
			msg.Service = entry.Metadata.Service
		}
	}

	// Include raw JSON if enabled
	if f.IncludeRaw {
		// Create raw JSON from entry
		rawFormatter := &JSONFormatter{}
		raw, err := rawFormatter.Format(entry)
		if err == nil {
			msg.Raw = string(raw)
		}
	}

	return json.Marshal(msg)
}

// NATSSubscriber subscribes to NATS log messages.
type NATSSubscriber struct {
	conn    *nats.Conn
	sub     *nats.Subscription
	handler func(*NATSLogMessage)
	mu      sync.Mutex
}

// NewNATSSubscriber creates a subscriber for NATS log messages.
func NewNATSSubscriber(conn *nats.Conn, subject string, handler func(*NATSLogMessage)) (*NATSSubscriber, error) {
	s := &NATSSubscriber{
		conn:    conn,
		handler: handler,
	}

	sub, err := conn.Subscribe(subject, func(msg *nats.Msg) {
		var logMsg NATSLogMessage
		if err := json.Unmarshal(msg.Data, &logMsg); err != nil {
			return
		}
		if handler != nil {
			handler(&logMsg)
		}
	})
	if err != nil {
		return nil, fmt.Errorf("failed to subscribe: %w", err)
	}

	s.sub = sub
	return s, nil
}

// Unsubscribe unsubscribes from NATS.
func (s *NATSSubscriber) Unsubscribe() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.sub != nil {
		return s.sub.Unsubscribe()
	}
	return nil
}

// Close closes the subscriber.
func (s *NATSSubscriber) Close() error {
	return s.Unsubscribe()
}

// Subscription returns the underlying NATS subscription.
func (s *NATSSubscriber) Subscription() *nats.Subscription {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.sub
}

// NewNATSLogSubscriber is an alias for NewNATSSubscriber for clarity.
func NewNATSLogSubscriber(conn *nats.Conn, subject string, handler func(*NATSLogMessage)) (*NATSSubscriber, error) {
	return NewNATSSubscriber(conn, subject, handler)
}
