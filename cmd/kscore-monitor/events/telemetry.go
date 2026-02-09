// Package events provides NATS telemetry subscriptions for the TUI monitor.
// Epic 15: Observability Enhancements - TUI Monitor Real-time Updates.
package events

import (
	"context"
	"fmt"
	"sync"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/nats-io/nats.go"

	"github.com/shawnbutts/keystone-core/internal/audit"
	"github.com/shawnbutts/keystone-core/internal/logging"
	"github.com/shawnbutts/keystone-core/internal/metrics"
	"github.com/shawnbutts/keystone-core/internal/tracing"
)

// LogMsg is a Bubble Tea message containing a log entry
type LogMsg struct {
	Log *logging.NATSLogMessage
	Err error
}

// MetricMsg is a Bubble Tea message containing a metric
type MetricMsg struct {
	Metric *metrics.NATSMetricMessage
	Err    error
}

// TraceMsg is a Bubble Tea message containing a span batch
type TraceMsg struct {
	Batch *tracing.NATSSpanBatch
	Err   error
}

// AuditMsg is a Bubble Tea message containing an audit entry
type AuditMsg struct {
	Audit *audit.NATSAuditMessage
	Err   error
}

// TelemetrySubscriber manages all telemetry subscriptions for the TUI
type TelemetrySubscriber struct {
	nc      *nats.Conn
	program *tea.Program
	mu      sync.Mutex
	closed  bool

	// Subscriptions
	logSub    *nats.Subscription
	metricSub *nats.Subscription
	traceSub  *nats.Subscription
	auditSub  *nats.Subscription

	// Configuration
	logSubject    string
	metricSubject string
	traceSubject  string
	auditSubject  string

	// Stats
	logsReceived    int64
	metricsReceived int64
	tracesReceived  int64
	auditsReceived  int64
}

// TelemetryConfig configures the telemetry subscriber
type TelemetryConfig struct {
	NATSURL       string
	LogSubject    string
	MetricSubject string
	TraceSubject  string
	AuditSubject  string
}

// DefaultTelemetryConfig returns default telemetry configuration
func DefaultTelemetryConfig() *TelemetryConfig {
	return &TelemetryConfig{
		NATSURL:       "nats://localhost:4222",
		LogSubject:    "kscore.logs.>",
		MetricSubject: "kscore.metrics.>",
		TraceSubject:  "kscore.traces.>",
		AuditSubject:  "kscore.audit.>",
	}
}

// NewTelemetrySubscriber creates a new telemetry subscriber
func NewTelemetrySubscriber(ctx context.Context, cfg *TelemetryConfig, program *tea.Program) (*TelemetrySubscriber, error) {
	if cfg == nil {
		cfg = DefaultTelemetryConfig()
	}

	// Connect to NATS
	nc, err := nats.Connect(cfg.NATSURL,
		nats.Name("kscore-monitor-telemetry"),
		nats.MaxReconnects(-1),
		nats.ReconnectWait(time.Second),
		nats.DisconnectErrHandler(func(_ *nats.Conn, err error) {
			if program != nil && err != nil {
				program.Send(LogMsg{Err: fmt.Errorf("NATS disconnected: %w", err)})
			}
		}),
		nats.ReconnectHandler(func(_ *nats.Conn) {
			if program != nil {
				program.Send(LogMsg{Log: &logging.NATSLogMessage{
					Timestamp: time.Now().Format(time.RFC3339),
					Level:     "info",
					Message:   "NATS reconnected",
				}})
			}
		}),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to NATS: %w", err)
	}

	ts := &TelemetrySubscriber{
		nc:            nc,
		program:       program,
		logSubject:    cfg.LogSubject,
		metricSubject: cfg.MetricSubject,
		traceSubject:  cfg.TraceSubject,
		auditSubject:  cfg.AuditSubject,
	}

	return ts, nil
}

// SubscribeLogs subscribes to log messages
func (t *TelemetrySubscriber) SubscribeLogs() error {
	t.mu.Lock()
	defer t.mu.Unlock()

	if t.closed {
		return fmt.Errorf("subscriber is closed")
	}

	if t.logSub != nil {
		return nil // Already subscribed
	}

	sub, err := logging.NewNATSLogSubscriber(t.nc, t.logSubject, func(msg *logging.NATSLogMessage) {
		t.mu.Lock()
		t.logsReceived++
		t.mu.Unlock()

		if t.program != nil {
			t.program.Send(LogMsg{Log: msg})
		}
	})
	if err != nil {
		return fmt.Errorf("failed to subscribe to logs: %w", err)
	}

	t.logSub = sub.Subscription()
	return nil
}

// SubscribeMetrics subscribes to metric messages
func (t *TelemetrySubscriber) SubscribeMetrics() error {
	t.mu.Lock()
	defer t.mu.Unlock()

	if t.closed {
		return fmt.Errorf("subscriber is closed")
	}

	if t.metricSub != nil {
		return nil // Already subscribed
	}

	sub, err := metrics.NewNATSMetricsMessageSubscriber(t.nc, t.metricSubject, func(msg *metrics.NATSMetricMessage) {
		t.mu.Lock()
		t.metricsReceived++
		t.mu.Unlock()

		if t.program != nil {
			t.program.Send(MetricMsg{Metric: msg})
		}
	})
	if err != nil {
		return fmt.Errorf("failed to subscribe to metrics: %w", err)
	}

	t.metricSub = sub.Subscription()
	return nil
}

// SubscribeTraces subscribes to trace span batches
func (t *TelemetrySubscriber) SubscribeTraces() error {
	t.mu.Lock()
	defer t.mu.Unlock()

	if t.closed {
		return fmt.Errorf("subscriber is closed")
	}

	if t.traceSub != nil {
		return nil // Already subscribed
	}

	sub, err := tracing.NewNATSSpanSubscriber(t.nc, t.traceSubject, func(batch *tracing.NATSSpanBatch) {
		t.mu.Lock()
		t.tracesReceived++
		t.mu.Unlock()

		if t.program != nil {
			t.program.Send(TraceMsg{Batch: batch})
		}
	})
	if err != nil {
		return fmt.Errorf("failed to subscribe to traces: %w", err)
	}

	t.traceSub = sub.Subscription()
	return nil
}

// SubscribeAudit subscribes to audit messages
func (t *TelemetrySubscriber) SubscribeAudit() error {
	t.mu.Lock()
	defer t.mu.Unlock()

	if t.closed {
		return fmt.Errorf("subscriber is closed")
	}

	if t.auditSub != nil {
		return nil // Already subscribed
	}

	sub, err := audit.NewNATSAuditSubscriber(t.nc, t.auditSubject, func(msg *audit.NATSAuditMessage) {
		t.mu.Lock()
		t.auditsReceived++
		t.mu.Unlock()

		if t.program != nil {
			t.program.Send(AuditMsg{Audit: msg})
		}
	})
	if err != nil {
		return fmt.Errorf("failed to subscribe to audit: %w", err)
	}

	t.auditSub = sub.Subscription()
	return nil
}

// SubscribeAll subscribes to all telemetry streams
func (t *TelemetrySubscriber) SubscribeAll() error {
	if err := t.SubscribeLogs(); err != nil {
		return err
	}
	if err := t.SubscribeMetrics(); err != nil {
		return err
	}
	if err := t.SubscribeTraces(); err != nil {
		return err
	}
	return t.SubscribeAudit()
}

// Stats returns subscription statistics
func (t *TelemetrySubscriber) Stats() (logsCount, metricsCount, tracesCount, auditsCount int64) {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.logsReceived, t.metricsReceived, t.tracesReceived, t.auditsReceived
}

// IsConnected returns whether NATS is connected
func (t *TelemetrySubscriber) IsConnected() bool {
	t.mu.Lock()
	defer t.mu.Unlock()

	if t.nc == nil {
		return false
	}
	return t.nc.IsConnected()
}

// Close closes all subscriptions and the NATS connection
func (t *TelemetrySubscriber) Close() error {
	t.mu.Lock()
	defer t.mu.Unlock()

	if t.closed {
		return nil
	}
	t.closed = true

	// Unsubscribe all
	if t.logSub != nil {
		_ = t.logSub.Unsubscribe()
	}
	if t.metricSub != nil {
		_ = t.metricSub.Unsubscribe()
	}
	if t.traceSub != nil {
		_ = t.traceSub.Unsubscribe()
	}
	if t.auditSub != nil {
		_ = t.auditSub.Unsubscribe()
	}

	// Close NATS connection
	if t.nc != nil {
		t.nc.Close()
	}

	return nil
}

// LogBuffer is a ring buffer for log messages
type LogBuffer struct {
	logs    []*logging.NATSLogMessage
	maxSize int
	mu      sync.Mutex
}

// NewLogBuffer creates a new log buffer
func NewLogBuffer(maxSize int) *LogBuffer {
	if maxSize <= 0 {
		maxSize = 1000
	}
	return &LogBuffer{
		logs:    make([]*logging.NATSLogMessage, 0, maxSize),
		maxSize: maxSize,
	}
}

// Add adds a log message to the buffer
func (b *LogBuffer) Add(log *logging.NATSLogMessage) {
	b.mu.Lock()
	defer b.mu.Unlock()

	if len(b.logs) >= b.maxSize {
		b.logs = b.logs[1:]
	}
	b.logs = append(b.logs, log)
}

// All returns all log messages
func (b *LogBuffer) All() []*logging.NATSLogMessage {
	b.mu.Lock()
	defer b.mu.Unlock()

	result := make([]*logging.NATSLogMessage, len(b.logs))
	copy(result, b.logs)
	return result
}

// Last returns the last n log messages
func (b *LogBuffer) Last(n int) []*logging.NATSLogMessage {
	b.mu.Lock()
	defer b.mu.Unlock()

	if n <= 0 || n > len(b.logs) {
		n = len(b.logs)
	}

	start := len(b.logs) - n
	result := make([]*logging.NATSLogMessage, n)
	copy(result, b.logs[start:])
	return result
}

// FilterByLevel returns logs matching the given level
func (b *LogBuffer) FilterByLevel(level string) []*logging.NATSLogMessage {
	b.mu.Lock()
	defer b.mu.Unlock()

	var result []*logging.NATSLogMessage
	for _, log := range b.logs {
		if log.Level == level {
			result = append(result, log)
		}
	}
	return result
}

// Clear clears the buffer
func (b *LogBuffer) Clear() {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.logs = make([]*logging.NATSLogMessage, 0, b.maxSize)
}

// Count returns the number of logs in the buffer
func (b *LogBuffer) Count() int {
	b.mu.Lock()
	defer b.mu.Unlock()
	return len(b.logs)
}

// MetricBuffer is a buffer for the latest metric values
type MetricBuffer struct {
	metrics map[string]*metrics.NATSMetricMessage
	mu      sync.Mutex
}

// NewMetricBuffer creates a new metric buffer
func NewMetricBuffer() *MetricBuffer {
	return &MetricBuffer{
		metrics: make(map[string]*metrics.NATSMetricMessage),
	}
}

// Add adds or updates a metric
func (b *MetricBuffer) Add(metric *metrics.NATSMetricMessage) {
	b.mu.Lock()
	defer b.mu.Unlock()

	key := fmt.Sprintf("%s:%s", metric.Service, metric.Name)
	b.metrics[key] = metric
}

// Get returns a specific metric
func (b *MetricBuffer) Get(service, name string) *metrics.NATSMetricMessage {
	b.mu.Lock()
	defer b.mu.Unlock()

	key := fmt.Sprintf("%s:%s", service, name)
	return b.metrics[key]
}

// All returns all metrics
func (b *MetricBuffer) All() []*metrics.NATSMetricMessage {
	b.mu.Lock()
	defer b.mu.Unlock()

	result := make([]*metrics.NATSMetricMessage, 0, len(b.metrics))
	for _, m := range b.metrics {
		result = append(result, m)
	}
	return result
}

// FilterByService returns metrics for a specific service
func (b *MetricBuffer) FilterByService(service string) []*metrics.NATSMetricMessage {
	b.mu.Lock()
	defer b.mu.Unlock()

	var result []*metrics.NATSMetricMessage
	for _, m := range b.metrics {
		if m.Service == service {
			result = append(result, m)
		}
	}
	return result
}

// FilterByType returns metrics of a specific type
func (b *MetricBuffer) FilterByType(metricType string) []*metrics.NATSMetricMessage {
	b.mu.Lock()
	defer b.mu.Unlock()

	var result []*metrics.NATSMetricMessage
	for _, m := range b.metrics {
		if m.Type == metricType {
			result = append(result, m)
		}
	}
	return result
}

// Clear clears all metrics
func (b *MetricBuffer) Clear() {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.metrics = make(map[string]*metrics.NATSMetricMessage)
}

// Count returns the number of unique metrics
func (b *MetricBuffer) Count() int {
	b.mu.Lock()
	defer b.mu.Unlock()
	return len(b.metrics)
}
