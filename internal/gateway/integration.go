// Package gateway provides the telemetry gateway for aggregating observability data.
package gateway

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"time"

	"github.com/nats-io/nats.go"
)

// Integration provides integration between the control plane and telemetry gateway.
type Integration struct {
	nc     *nats.Conn
	config Config

	// Publishers for control plane to send telemetry
	metricsSubject string
	logsSubject    string
	tracesSubject  string
}

// NewIntegration creates a new integration instance.
func NewIntegration(nc *nats.Conn, config Config) *Integration {
	return &Integration{
		nc:             nc,
		config:         config,
		metricsSubject: "kscore.telemetry.metrics.controlplane",
		logsSubject:    "kscore.telemetry.logs.controlplane",
		tracesSubject:  "kscore.telemetry.traces.controlplane",
	}
}

// PublishMetrics publishes metrics to the gateway.
func (i *Integration) PublishMetrics(ctx context.Context, data []byte, format string) error {
	if i.nc == nil || !i.nc.IsConnected() {
		return fmt.Errorf("not connected to NATS")
	}

	msg := struct {
		AgentID   string    `json:"agent_id"`
		Format    string    `json:"format"`
		Timestamp time.Time `json:"timestamp"`
		Data      []byte    `json:"data"`
	}{
		AgentID:   "controlplane",
		Format:    format,
		Timestamp: time.Now(),
		Data:      data,
	}

	payload, err := marshalJSON(msg)
	if err != nil {
		return fmt.Errorf("failed to marshal metrics: %w", err)
	}

	return i.nc.Publish(i.metricsSubject, payload)
}

// PublishLogs publishes logs to the gateway.
func (i *Integration) PublishLogs(ctx context.Context, entries []LogEntry) error {
	if i.nc == nil || !i.nc.IsConnected() {
		return fmt.Errorf("not connected to NATS")
	}

	msg := struct {
		AgentID   string     `json:"agent_id"`
		Timestamp time.Time  `json:"timestamp"`
		Entries   []LogEntry `json:"entries"`
	}{
		AgentID:   "controlplane",
		Timestamp: time.Now(),
		Entries:   entries,
	}

	payload, err := marshalJSON(msg)
	if err != nil {
		return fmt.Errorf("failed to marshal logs: %w", err)
	}

	return i.nc.Publish(i.logsSubject, payload)
}

// LogEntry represents a log entry for publishing.
type LogEntry struct {
	ID        string            `json:"id,omitempty"`
	Timestamp time.Time         `json:"timestamp"`
	Level     string            `json:"level"`
	Source    string            `json:"source"`
	Message   string            `json:"message"`
	Labels    map[string]string `json:"labels,omitempty"`
}

// PublishTraces publishes traces to the gateway.
func (i *Integration) PublishTraces(ctx context.Context, spans []Span) error {
	if i.nc == nil || !i.nc.IsConnected() {
		return fmt.Errorf("not connected to NATS")
	}

	msg := struct {
		AgentID   string    `json:"agent_id"`
		Timestamp time.Time `json:"timestamp"`
		Spans     []Span    `json:"spans"`
	}{
		AgentID:   "controlplane",
		Timestamp: time.Now(),
		Spans:     spans,
	}

	payload, err := marshalJSON(msg)
	if err != nil {
		return fmt.Errorf("failed to marshal traces: %w", err)
	}

	return i.nc.Publish(i.tracesSubject, payload)
}

// Span represents a trace span for publishing.
type Span struct {
	TraceID       string                 `json:"trace_id"`
	SpanID        string                 `json:"span_id"`
	ParentSpanID  string                 `json:"parent_span_id,omitempty"`
	OperationName string                 `json:"operation_name"`
	ServiceName   string                 `json:"service_name"`
	StartTime     time.Time              `json:"start_time"`
	EndTime       time.Time              `json:"end_time"`
	Duration      time.Duration          `json:"duration"`
	Status        string                 `json:"status"`
	Attributes    map[string]interface{} `json:"attributes,omitempty"`
}

// marshalJSON is a helper to marshal JSON.
func marshalJSON(v interface{}) ([]byte, error) {
	return json.Marshal(v)
}

// Status represents the status of the telemetry gateway.
type Status struct {
	// Enabled indicates if the gateway is enabled
	Enabled bool `json:"enabled"`

	// Healthy indicates if the gateway is healthy
	Healthy bool `json:"healthy"`

	// Endpoints lists the available endpoints
	Endpoints []EndpointStatus `json:"endpoints"`

	// Stats contains gateway statistics
	Stats Stats `json:"stats"`
}

// EndpointStatus represents the status of an endpoint.
type EndpointStatus struct {
	Name    string `json:"name"`
	URL     string `json:"url"`
	Healthy bool   `json:"healthy"`
}

// Stats contains gateway statistics.
type Stats struct {
	// Metrics stats
	MetricsAgents   int   `json:"metrics_agents"`
	MetricsSeries   int   `json:"metrics_series"`
	MetricsReceived int64 `json:"metrics_received"`

	// Logs stats
	LogsEntries  int   `json:"logs_entries"`
	LogsReceived int64 `json:"logs_received"`
	LogsDropped  int64 `json:"logs_dropped"`

	// Traces stats
	TracesCount    int   `json:"traces_count"`
	TracesReceived int64 `json:"traces_received"`
	SpansReceived  int64 `json:"spans_received"`

	// Uptime
	UptimeSeconds int64 `json:"uptime_seconds"`
}

// LogWriter wraps the integration to provide a log.Logger-compatible writer.
type LogWriter struct {
	integration *Integration
	source      string
	level       string
}

// NewLogWriter creates a new log writer.
func NewLogWriter(integration *Integration, source, level string) *LogWriter {
	return &LogWriter{
		integration: integration,
		source:      source,
		level:       level,
	}
}

// Write implements io.Writer.
func (w *LogWriter) Write(p []byte) (n int, err error) {
	entry := LogEntry{
		Timestamp: time.Now(),
		Level:     w.level,
		Source:    w.source,
		Message:   string(p),
	}

	if err := w.integration.PublishLogs(context.Background(), []LogEntry{entry}); err != nil {
		log.Printf("Failed to publish log: %v", err)
		// Don't return error to avoid breaking log callers
	}

	return len(p), nil
}
