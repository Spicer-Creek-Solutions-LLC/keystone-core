// Package audit provides NATS audit transport for centralized audit collection.
// Epic 15: Observability Enhancements - NATS audit transport.
package audit

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/nats-io/nats.go"
)

// NATSAuditConfig configures the NATS audit logger.
type NATSAuditConfig struct {
	// URL is the NATS server URL (e.g., "nats://localhost:4222")
	URL string

	// Subject is the base subject for audit logs
	// Default: "kscore.audit"
	Subject string

	// SubjectPerAction uses separate subjects per action type
	// e.g., kscore.audit.command_executed
	SubjectPerAction bool

	// SubjectPerTool uses separate subjects per CLI tool
	// e.g., kscore.audit.kscore-exec
	SubjectPerTool bool

	// ServiceName identifies the audit source
	ServiceName string

	// BufferSize is the number of entries to buffer
	// Default: 1000
	BufferSize int

	// FlushInterval is how often to flush entries
	// Default: 1s
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

	// Credentials for authentication
	Token    string
	User     string
	Password string
	NKeyFile string
	CredFile string
}

// DefaultNATSAuditConfig returns a default NATS audit configuration.
func DefaultNATSAuditConfig() *NATSAuditConfig {
	return &NATSAuditConfig{
		URL:              "nats://localhost:4222",
		Subject:          "kscore.audit",
		SubjectPerAction: false,
		SubjectPerTool:   false,
		BufferSize:       1000,
		FlushInterval:    1 * time.Second,
		ConnectTimeout:   5 * time.Second,
		ReconnectWait:    1 * time.Second,
		MaxReconnects:    -1,
	}
}

// NATSAuditMessage is the JSON structure sent over NATS.
type NATSAuditMessage struct {
	// Core audit fields
	Timestamp     string      `json:"timestamp"`
	AuditType     string      `json:"audit_type"`
	User          string      `json:"user"`
	UID           int         `json:"uid"`
	TTY           string      `json:"tty,omitempty"`
	PID           int         `json:"pid"`
	Tool          string      `json:"tool"`
	Command       string      `json:"command"`
	Args          []string    `json:"args,omitempty"`
	Target        string      `json:"target,omitempty"`
	AgentsMatched int         `json:"agents_matched,omitempty"`
	Result        string      `json:"result"`
	ExitCode      int         `json:"exit_code"`
	DurationMS    int64       `json:"duration_ms"`
	CorrelationID string      `json:"correlation_id"`
	Error         string      `json:"error,omitempty"`
	RemoteAddr    string      `json:"remote_addr,omitempty"`

	// Service metadata
	Service  string `json:"service,omitempty"`
	Hostname string `json:"hostname,omitempty"`

	// Extra data
	Extra map[string]interface{} `json:"extra,omitempty"`
}

// NATSAuditLogger sends audit entries to NATS.
type NATSAuditLogger struct {
	config   *NATSAuditConfig
	conn     *nats.Conn
	entries  chan *AuditEntry
	mu       sync.Mutex
	closed   bool
	hostname string
	stopCh   chan struct{}
	doneCh   chan struct{}

	// Stats
	entriesPublished int64
	entriesDropped   int64
	lastError        error
	lastErrorTime    time.Time
}

// NewNATSAuditLogger creates a new NATS audit logger.
func NewNATSAuditLogger(config *NATSAuditConfig) (*NATSAuditLogger, error) {
	if config == nil {
		config = DefaultNATSAuditConfig()
	}

	// Build NATS options
	opts := []nats.Option{
		nats.Name("kscore-audit-" + config.ServiceName),
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
		bufferSize = 1000
	}

	logger := &NATSAuditLogger{
		config:  config,
		conn:    conn,
		entries: make(chan *AuditEntry, bufferSize),
		stopCh:  make(chan struct{}),
		doneCh:  make(chan struct{}),
	}

	// Start the publish loop
	go logger.publishLoop()

	return logger, nil
}

// Log sends an audit entry to NATS.
func (n *NATSAuditLogger) Log(ctx context.Context, entry *AuditEntry) error {
	n.mu.Lock()
	if n.closed {
		n.mu.Unlock()
		return fmt.Errorf("logger is closed")
	}
	n.mu.Unlock()

	select {
	case n.entries <- entry:
		return nil
	default:
		n.mu.Lock()
		n.entriesDropped++
		n.mu.Unlock()
		return fmt.Errorf("audit buffer full, entry dropped")
	}
}

// publishLoop processes entries and publishes to NATS.
func (n *NATSAuditLogger) publishLoop() {
	defer close(n.doneCh)

	ticker := time.NewTicker(n.config.FlushInterval)
	defer ticker.Stop()

	batch := make([]*AuditEntry, 0, 100)

	for {
		select {
		case entry, ok := <-n.entries:
			if !ok {
				// Channel closed, publish remaining
				n.publishBatch(batch)
				return
			}

			batch = append(batch, entry)

			// Publish if batch is full
			if len(batch) >= 100 {
				n.publishBatch(batch)
				batch = batch[:0]
			}

		case <-ticker.C:
			if len(batch) > 0 {
				n.publishBatch(batch)
				batch = batch[:0]
			}

		case <-n.stopCh:
			// Drain remaining entries
			for {
				select {
				case entry := <-n.entries:
					batch = append(batch, entry)
				default:
					n.publishBatch(batch)
					return
				}
			}
		}
	}
}

// publishBatch publishes a batch of entries.
func (n *NATSAuditLogger) publishBatch(entries []*AuditEntry) {
	if len(entries) == 0 {
		return
	}

	n.mu.Lock()
	conn := n.conn
	n.mu.Unlock()

	if conn == nil || !conn.IsConnected() {
		n.mu.Lock()
		n.entriesDropped += int64(len(entries))
		n.lastError = fmt.Errorf("NATS not connected")
		n.lastErrorTime = time.Now()
		n.mu.Unlock()
		return
	}

	for _, entry := range entries {
		msg := n.entryToMessage(entry)
		data, err := json.Marshal(msg)
		if err != nil {
			n.mu.Lock()
			n.entriesDropped++
			n.lastError = err
			n.lastErrorTime = time.Now()
			n.mu.Unlock()
			continue
		}

		subject := n.buildSubject(entry)
		if err := conn.Publish(subject, data); err != nil {
			n.mu.Lock()
			n.entriesDropped++
			n.lastError = err
			n.lastErrorTime = time.Now()
			n.mu.Unlock()
			continue
		}

		n.mu.Lock()
		n.entriesPublished++
		n.mu.Unlock()
	}
}

// entryToMessage converts an AuditEntry to a NATSAuditMessage.
func (n *NATSAuditLogger) entryToMessage(entry *AuditEntry) *NATSAuditMessage {
	return &NATSAuditMessage{
		Timestamp:     entry.Timestamp.Format(time.RFC3339Nano),
		AuditType:     string(entry.AuditType),
		User:          entry.User,
		UID:           entry.UID,
		TTY:           entry.TTY,
		PID:           entry.PID,
		Tool:          entry.Tool,
		Command:       entry.Command,
		Args:          entry.Args,
		Target:        entry.Target,
		AgentsMatched: entry.AgentsMatched,
		Result:        string(entry.Result),
		ExitCode:      entry.ExitCode,
		DurationMS:    entry.DurationMS,
		CorrelationID: entry.CorrelationID,
		Error:         entry.Error,
		RemoteAddr:    entry.RemoteAddr,
		Service:       n.config.ServiceName,
		Hostname:      n.hostname,
		Extra:         entry.Extra,
	}
}

// buildSubject builds the NATS subject for an audit entry.
func (n *NATSAuditLogger) buildSubject(entry *AuditEntry) string {
	subject := n.config.Subject

	if n.config.SubjectPerTool && entry.Tool != "" {
		subject = fmt.Sprintf("%s.%s", subject, entry.Tool)
	}

	if n.config.SubjectPerAction {
		subject = fmt.Sprintf("%s.%s", subject, entry.AuditType)
	}

	return subject
}

// Stats returns logger statistics.
func (n *NATSAuditLogger) Stats() (published, dropped int64, lastErr error, lastErrTime time.Time) {
	n.mu.Lock()
	defer n.mu.Unlock()
	return n.entriesPublished, n.entriesDropped, n.lastError, n.lastErrorTime
}

// IsConnected returns whether NATS is connected.
func (n *NATSAuditLogger) IsConnected() bool {
	n.mu.Lock()
	defer n.mu.Unlock()
	return n.conn != nil && n.conn.IsConnected()
}

// Close closes the NATS audit logger.
func (n *NATSAuditLogger) Close() error {
	n.mu.Lock()
	if n.closed {
		n.mu.Unlock()
		return nil
	}
	n.closed = true
	n.mu.Unlock()

	// Signal stop
	close(n.stopCh)

	// Wait for publish loop to finish
	<-n.doneCh

	// Close entries channel
	close(n.entries)

	// Close NATS connection
	n.mu.Lock()
	defer n.mu.Unlock()
	if n.conn != nil {
		n.conn.Drain()
		n.conn.Close()
	}

	return nil
}

// NATSAuditSubscriber subscribes to NATS audit messages.
type NATSAuditSubscriber struct {
	conn    *nats.Conn
	sub     *nats.Subscription
	handler func(*NATSAuditMessage)
	mu      sync.Mutex
}

// NewNATSAuditSubscriber creates a subscriber for NATS audit messages.
func NewNATSAuditSubscriber(conn *nats.Conn, subject string, handler func(*NATSAuditMessage)) (*NATSAuditSubscriber, error) {
	s := &NATSAuditSubscriber{
		conn:    conn,
		handler: handler,
	}

	sub, err := conn.Subscribe(subject, func(msg *nats.Msg) {
		var auditMsg NATSAuditMessage
		if err := json.Unmarshal(msg.Data, &auditMsg); err != nil {
			return
		}
		if handler != nil {
			handler(&auditMsg)
		}
	})
	if err != nil {
		return nil, fmt.Errorf("failed to subscribe: %w", err)
	}

	s.sub = sub
	return s, nil
}

// Unsubscribe unsubscribes from NATS.
func (s *NATSAuditSubscriber) Unsubscribe() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.sub != nil {
		return s.sub.Unsubscribe()
	}
	return nil
}

// Close closes the subscriber.
func (s *NATSAuditSubscriber) Close() error {
	return s.Unsubscribe()
}

// Subscription returns the underlying NATS subscription.
func (s *NATSAuditSubscriber) Subscription() *nats.Subscription {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.sub
}

// NATSAuditBatch is a batch of audit messages.
type NATSAuditBatch struct {
	Messages  []NATSAuditMessage `json:"messages"`
	Timestamp string             `json:"timestamp"`
	Service   string             `json:"service"`
	Count     int                `json:"count"`
}

// NATSAuditAggregator aggregates audit messages for analysis.
type NATSAuditAggregator struct {
	messages []NATSAuditMessage
	mu       sync.Mutex
	maxSize  int
}

// NewNATSAuditAggregator creates a new audit aggregator.
func NewNATSAuditAggregator(maxSize int) *NATSAuditAggregator {
	if maxSize <= 0 {
		maxSize = 10000
	}
	return &NATSAuditAggregator{
		messages: make([]NATSAuditMessage, 0, maxSize),
		maxSize:  maxSize,
	}
}

// Add adds a message to the aggregator.
func (a *NATSAuditAggregator) Add(msg *NATSAuditMessage) {
	a.mu.Lock()
	defer a.mu.Unlock()

	// Evict oldest if at capacity
	if len(a.messages) >= a.maxSize {
		a.messages = a.messages[1:]
	}

	a.messages = append(a.messages, *msg)
}

// Messages returns all aggregated messages.
func (a *NATSAuditAggregator) Messages() []NATSAuditMessage {
	a.mu.Lock()
	defer a.mu.Unlock()

	result := make([]NATSAuditMessage, len(a.messages))
	copy(result, a.messages)
	return result
}

// FilterByUser returns messages for a specific user.
func (a *NATSAuditAggregator) FilterByUser(user string) []NATSAuditMessage {
	a.mu.Lock()
	defer a.mu.Unlock()

	var result []NATSAuditMessage
	for _, msg := range a.messages {
		if msg.User == user {
			result = append(result, msg)
		}
	}
	return result
}

// FilterByAction returns messages for a specific action type.
func (a *NATSAuditAggregator) FilterByAction(action AuditAction) []NATSAuditMessage {
	a.mu.Lock()
	defer a.mu.Unlock()

	var result []NATSAuditMessage
	for _, msg := range a.messages {
		if msg.AuditType == string(action) {
			result = append(result, msg)
		}
	}
	return result
}

// FilterByResult returns messages for a specific result.
func (a *NATSAuditAggregator) FilterByResult(result AuditResult) []NATSAuditMessage {
	a.mu.Lock()
	defer a.mu.Unlock()

	var results []NATSAuditMessage
	for _, msg := range a.messages {
		if msg.Result == string(result) {
			results = append(results, msg)
		}
	}
	return results
}

// FilterByTool returns messages for a specific CLI tool.
func (a *NATSAuditAggregator) FilterByTool(tool string) []NATSAuditMessage {
	a.mu.Lock()
	defer a.mu.Unlock()

	var result []NATSAuditMessage
	for _, msg := range a.messages {
		if msg.Tool == tool {
			result = append(result, msg)
		}
	}
	return result
}

// FilterByTimeRange returns messages within a time range.
func (a *NATSAuditAggregator) FilterByTimeRange(start, end time.Time) []NATSAuditMessage {
	a.mu.Lock()
	defer a.mu.Unlock()

	var result []NATSAuditMessage
	for _, msg := range a.messages {
		ts, err := time.Parse(time.RFC3339Nano, msg.Timestamp)
		if err != nil {
			continue
		}
		if (ts.Equal(start) || ts.After(start)) && (ts.Equal(end) || ts.Before(end)) {
			result = append(result, msg)
		}
	}
	return result
}

// Clear clears all aggregated messages.
func (a *NATSAuditAggregator) Clear() {
	a.mu.Lock()
	defer a.mu.Unlock()

	a.messages = make([]NATSAuditMessage, 0, a.maxSize)
}

// Count returns the number of aggregated messages.
func (a *NATSAuditAggregator) Count() int {
	a.mu.Lock()
	defer a.mu.Unlock()

	return len(a.messages)
}

// Summary returns a summary of aggregated messages.
type NATSAuditSummary struct {
	TotalCount     int            `json:"total_count"`
	SuccessCount   int            `json:"success_count"`
	FailureCount   int            `json:"failure_count"`
	DeniedCount    int            `json:"denied_count"`
	TimeoutCount   int            `json:"timeout_count"`
	CountByAction  map[string]int `json:"count_by_action"`
	CountByTool    map[string]int `json:"count_by_tool"`
	CountByUser    map[string]int `json:"count_by_user"`
	UniqueUsers    int            `json:"unique_users"`
	OldestEntry    string         `json:"oldest_entry,omitempty"`
	NewestEntry    string         `json:"newest_entry,omitempty"`
}

// Summary returns a summary of aggregated messages.
func (a *NATSAuditAggregator) Summary() *NATSAuditSummary {
	a.mu.Lock()
	defer a.mu.Unlock()

	summary := &NATSAuditSummary{
		TotalCount:    len(a.messages),
		CountByAction: make(map[string]int),
		CountByTool:   make(map[string]int),
		CountByUser:   make(map[string]int),
	}

	users := make(map[string]struct{})

	for i, msg := range a.messages {
		// Count by result
		switch msg.Result {
		case string(ResultSuccess):
			summary.SuccessCount++
		case string(ResultFailure):
			summary.FailureCount++
		case string(ResultDenied):
			summary.DeniedCount++
		case string(ResultTimeout):
			summary.TimeoutCount++
		}

		// Count by action
		summary.CountByAction[msg.AuditType]++

		// Count by tool
		summary.CountByTool[msg.Tool]++

		// Count by user
		summary.CountByUser[msg.User]++
		users[msg.User] = struct{}{}

		// Track oldest/newest
		if i == 0 {
			summary.OldestEntry = msg.Timestamp
		}
		summary.NewestEntry = msg.Timestamp
	}

	summary.UniqueUsers = len(users)

	return summary
}
