package nats

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"
)

// ============================================================================
// Phase 9: Observability - T9.3 Connection Debugging
// ============================================================================

// DebugLevel defines the logging level for debug output
type DebugLevel int

const (
	DebugLevelOff DebugLevel = iota
	DebugLevelError
	DebugLevelWarn
	DebugLevelInfo
	DebugLevelDebug
	DebugLevelTrace
)

// String returns the string representation of the debug level
func (l DebugLevel) String() string {
	switch l {
	case DebugLevelOff:
		return "off"
	case DebugLevelError:
		return "error"
	case DebugLevelWarn:
		return "warn"
	case DebugLevelInfo:
		return "info"
	case DebugLevelDebug:
		return "debug"
	case DebugLevelTrace:
		return "trace"
	default:
		return "unknown"
	}
}

// ParseDebugLevel parses a debug level string
func ParseDebugLevel(s string) DebugLevel {
	switch strings.ToLower(s) {
	case "error":
		return DebugLevelError
	case "warn", "warning":
		return DebugLevelWarn
	case "info":
		return DebugLevelInfo
	case "debug":
		return DebugLevelDebug
	case "trace":
		return DebugLevelTrace
	default:
		return DebugLevelOff
	}
}

// DebugConfig holds debug configuration
type DebugConfig struct {
	// Level is the minimum log level
	Level DebugLevel

	// MaxEvents is the maximum number of events to retain
	MaxEvents int

	// EnableMessageTracing enables message flow tracing
	EnableMessageTracing bool

	// EnableLatencyBreakdown enables per-hop latency tracking
	EnableLatencyBreakdown bool

	// TraceSubjects is a list of subjects to trace (empty = all)
	TraceSubjects []string

	// TraceAgents is a list of agent IDs to trace (empty = all)
	TraceAgents []string

	// OutputFunc is called with debug output (nil = discard)
	OutputFunc func(level DebugLevel, msg string, fields map[string]interface{})
}

// DefaultDebugConfig returns default debug configuration
func DefaultDebugConfig() *DebugConfig {
	return &DebugConfig{
		Level:                  DebugLevelInfo,
		MaxEvents:              10000,
		EnableMessageTracing:   false,
		EnableLatencyBreakdown: true,
		TraceSubjects:          nil,
		TraceAgents:            nil,
		OutputFunc:             nil,
	}
}

// ConnectionEvent represents a connection state change event
type ConnectionEvent struct {
	Timestamp   time.Time              `json:"timestamp"`
	Type        ConnectionEventType    `json:"type"`
	Endpoint    string                 `json:"endpoint,omitempty"`
	Strategy    string                 `json:"strategy,omitempty"`
	FromState   string                 `json:"from_state,omitempty"`
	ToState     string                 `json:"to_state,omitempty"`
	Error       string                 `json:"error,omitempty"`
	Latency     time.Duration          `json:"latency,omitempty"`
	Details     map[string]interface{} `json:"details,omitempty"`
}

// ConnectionEventType defines the type of connection event
type ConnectionEventType string

const (
	EventTypeConnect       ConnectionEventType = "connect"
	EventTypeDisconnect    ConnectionEventType = "disconnect"
	EventTypeReconnect     ConnectionEventType = "reconnect"
	EventTypeError         ConnectionEventType = "error"
	EventTypeFailover      ConnectionEventType = "failover"
	EventTypeStateChange   ConnectionEventType = "state_change"
	EventTypeLatencySpike  ConnectionEventType = "latency_spike"
	EventTypeBufferWarning ConnectionEventType = "buffer_warning"
	EventTypeCircuitOpen   ConnectionEventType = "circuit_open"
	EventTypeCircuitClose  ConnectionEventType = "circuit_close"
)

// MessageTrace represents a traced message flow
type MessageTrace struct {
	TraceID       string          `json:"trace_id"`
	MessageID     string          `json:"message_id"`
	Subject       string          `json:"subject"`
	Source        string          `json:"source"`
	Destination   string          `json:"destination,omitempty"`
	StartTime     time.Time       `json:"start_time"`
	EndTime       time.Time       `json:"end_time,omitempty"`
	TotalLatency  time.Duration   `json:"total_latency,omitempty"`
	Hops          []MessageHop    `json:"hops"`
	Status        string          `json:"status"`
	Error         string          `json:"error,omitempty"`
	Size          int64           `json:"size"`
}

// MessageHop represents a single hop in a message trace
type MessageHop struct {
	HopNumber   int           `json:"hop_number"`
	Component   string        `json:"component"`
	Endpoint    string        `json:"endpoint,omitempty"`
	EntryTime   time.Time     `json:"entry_time"`
	ExitTime    time.Time     `json:"exit_time,omitempty"`
	Latency     time.Duration `json:"latency,omitempty"`
	Action      string        `json:"action"`
	Error       string        `json:"error,omitempty"`
}

// ConnectionTimeline represents a timeline of connection events
type ConnectionTimeline struct {
	Endpoint    string            `json:"endpoint"`
	StartTime   time.Time         `json:"start_time"`
	EndTime     time.Time         `json:"end_time,omitempty"`
	Events      []ConnectionEvent `json:"events"`
	Summary     TimelineSummary   `json:"summary"`
}

// TimelineSummary summarizes a connection timeline
type TimelineSummary struct {
	TotalConnections int64         `json:"total_connections"`
	TotalErrors      int64         `json:"total_errors"`
	TotalFailovers   int64         `json:"total_failovers"`
	Uptime           time.Duration `json:"uptime"`
	Downtime         time.Duration `json:"downtime"`
	MTBF             time.Duration `json:"mtbf"` // Mean Time Between Failures
	MTTR             time.Duration `json:"mttr"` // Mean Time To Recovery
	AvgLatency       time.Duration `json:"avg_latency"`
}

// ConnectionDebugger provides connection debugging capabilities
type ConnectionDebugger struct {
	config *DebugConfig

	events       []ConnectionEvent
	traces       map[string]*MessageTrace
	timelines    map[string]*ConnectionTimeline
	activeTraces map[string]*MessageTrace

	mu sync.RWMutex
}

// NewConnectionDebugger creates a new connection debugger
func NewConnectionDebugger(config *DebugConfig) *ConnectionDebugger {
	if config == nil {
		config = DefaultDebugConfig()
	}

	return &ConnectionDebugger{
		config:       config,
		events:       make([]ConnectionEvent, 0, config.MaxEvents),
		traces:       make(map[string]*MessageTrace),
		timelines:    make(map[string]*ConnectionTimeline),
		activeTraces: make(map[string]*MessageTrace),
	}
}

// RecordEvent records a connection event
func (d *ConnectionDebugger) RecordEvent(event ConnectionEvent) {
	if event.Timestamp.IsZero() {
		event.Timestamp = time.Now()
	}

	d.mu.Lock()
	defer d.mu.Unlock()

	// Append event
	d.events = append(d.events, event)

	// Trim if over max
	if len(d.events) > d.config.MaxEvents {
		d.events = d.events[len(d.events)-d.config.MaxEvents:]
	}

	// Update timeline
	if event.Endpoint != "" {
		timeline, exists := d.timelines[event.Endpoint]
		if !exists {
			timeline = &ConnectionTimeline{
				Endpoint:  event.Endpoint,
				StartTime: event.Timestamp,
				Events:    make([]ConnectionEvent, 0),
			}
			d.timelines[event.Endpoint] = timeline
		}
		timeline.Events = append(timeline.Events, event)
		timeline.EndTime = event.Timestamp

		// Update summary
		d.updateTimelineSummary(timeline)
	}

	// Output if configured
	if d.config.OutputFunc != nil && d.config.Level >= DebugLevelDebug {
		fields := map[string]interface{}{
			"type":     event.Type,
			"endpoint": event.Endpoint,
		}
		if event.Error != "" {
			fields["error"] = event.Error
		}
		if event.Latency > 0 {
			fields["latency"] = event.Latency.String()
		}
		d.config.OutputFunc(DebugLevelDebug, "connection event", fields)
	}
}

func (d *ConnectionDebugger) updateTimelineSummary(timeline *ConnectionTimeline) {
	summary := TimelineSummary{}

	var uptime, downtime time.Duration
	var lastConnected, lastDisconnected time.Time
	var latencySum time.Duration
	var latencyCount int
	var failureIntervals []time.Duration
	var recoveryDurations []time.Duration

	for _, event := range timeline.Events {
		switch event.Type {
		case EventTypeConnect:
			summary.TotalConnections++
			if !lastDisconnected.IsZero() {
				recovery := event.Timestamp.Sub(lastDisconnected)
				recoveryDurations = append(recoveryDurations, recovery)
				downtime += recovery
			}
			lastConnected = event.Timestamp
		case EventTypeDisconnect, EventTypeError:
			summary.TotalErrors++
			if !lastConnected.IsZero() {
				interval := event.Timestamp.Sub(lastConnected)
				failureIntervals = append(failureIntervals, interval)
				uptime += interval
			}
			lastDisconnected = event.Timestamp
		case EventTypeFailover:
			summary.TotalFailovers++
		}

		if event.Latency > 0 {
			latencySum += event.Latency
			latencyCount++
		}
	}

	// Calculate final uptime if still connected
	if !lastConnected.IsZero() && lastDisconnected.Before(lastConnected) {
		uptime += time.Since(lastConnected)
	}

	summary.Uptime = uptime
	summary.Downtime = downtime

	if latencyCount > 0 {
		summary.AvgLatency = latencySum / time.Duration(latencyCount)
	}

	if len(failureIntervals) > 0 {
		var total time.Duration
		for _, i := range failureIntervals {
			total += i
		}
		summary.MTBF = total / time.Duration(len(failureIntervals))
	}

	if len(recoveryDurations) > 0 {
		var total time.Duration
		for _, r := range recoveryDurations {
			total += r
		}
		summary.MTTR = total / time.Duration(len(recoveryDurations))
	}

	timeline.Summary = summary
}

// StartTrace starts a message trace
func (d *ConnectionDebugger) StartTrace(traceID, messageID, subject, source string, size int64) {
	if !d.config.EnableMessageTracing {
		return
	}

	// Check if subject should be traced
	if len(d.config.TraceSubjects) > 0 {
		matched := false
		for _, ts := range d.config.TraceSubjects {
			if matchSubject(ts, subject) {
				matched = true
				break
			}
		}
		if !matched {
			return
		}
	}

	trace := &MessageTrace{
		TraceID:   traceID,
		MessageID: messageID,
		Subject:   subject,
		Source:    source,
		StartTime: time.Now(),
		Status:    "in_progress",
		Size:      size,
		Hops:      make([]MessageHop, 0),
	}

	d.mu.Lock()
	d.activeTraces[traceID] = trace
	d.mu.Unlock()
}

// AddHop adds a hop to an active trace
func (d *ConnectionDebugger) AddHop(traceID, component, endpoint, action string) {
	d.mu.Lock()
	trace, exists := d.activeTraces[traceID]
	if !exists {
		d.mu.Unlock()
		return
	}

	hop := MessageHop{
		HopNumber: len(trace.Hops) + 1,
		Component: component,
		Endpoint:  endpoint,
		EntryTime: time.Now(),
		Action:    action,
	}
	trace.Hops = append(trace.Hops, hop)
	d.mu.Unlock()
}

// CompleteHop completes the current hop
func (d *ConnectionDebugger) CompleteHop(traceID string, err error) {
	d.mu.Lock()
	trace, exists := d.activeTraces[traceID]
	if !exists || len(trace.Hops) == 0 {
		d.mu.Unlock()
		return
	}

	lastIdx := len(trace.Hops) - 1
	trace.Hops[lastIdx].ExitTime = time.Now()
	trace.Hops[lastIdx].Latency = trace.Hops[lastIdx].ExitTime.Sub(trace.Hops[lastIdx].EntryTime)
	if err != nil {
		trace.Hops[lastIdx].Error = err.Error()
	}
	d.mu.Unlock()
}

// EndTrace ends a message trace
func (d *ConnectionDebugger) EndTrace(traceID, destination, status string, err error) {
	d.mu.Lock()
	defer d.mu.Unlock()

	trace, exists := d.activeTraces[traceID]
	if !exists {
		return
	}

	trace.EndTime = time.Now()
	trace.TotalLatency = trace.EndTime.Sub(trace.StartTime)
	trace.Destination = destination
	trace.Status = status
	if err != nil {
		trace.Error = err.Error()
	}

	// Move to completed traces
	d.traces[traceID] = trace
	delete(d.activeTraces, traceID)

	// Trim old traces
	if len(d.traces) > d.config.MaxEvents {
		// Remove oldest traces
		var oldest []string
		for id := range d.traces {
			oldest = append(oldest, id)
		}
		sort.Slice(oldest, func(i, j int) bool {
			return d.traces[oldest[i]].StartTime.Before(d.traces[oldest[j]].StartTime)
		})
		for i := 0; i < len(oldest)-d.config.MaxEvents; i++ {
			delete(d.traces, oldest[i])
		}
	}
}

// GetEvents returns recent connection events
func (d *ConnectionDebugger) GetEvents(filter *EventFilter) []ConnectionEvent {
	d.mu.RLock()
	defer d.mu.RUnlock()

	var result []ConnectionEvent
	for _, event := range d.events {
		if filter != nil && !filter.matches(event) {
			continue
		}
		result = append(result, event)
	}
	return result
}

// GetTrace returns a specific message trace
func (d *ConnectionDebugger) GetTrace(traceID string) *MessageTrace {
	d.mu.RLock()
	defer d.mu.RUnlock()

	if trace, exists := d.traces[traceID]; exists {
		return trace
	}
	if trace, exists := d.activeTraces[traceID]; exists {
		return trace
	}
	return nil
}

// GetTraces returns message traces matching a filter
func (d *ConnectionDebugger) GetTraces(filter *TraceFilter) []*MessageTrace {
	d.mu.RLock()
	defer d.mu.RUnlock()

	var result []*MessageTrace
	for _, trace := range d.traces {
		if filter != nil && !filter.matches(trace) {
			continue
		}
		result = append(result, trace)
	}
	return result
}

// GetTimeline returns a connection timeline for an endpoint
func (d *ConnectionDebugger) GetTimeline(endpoint string) *ConnectionTimeline {
	d.mu.RLock()
	defer d.mu.RUnlock()

	return d.timelines[endpoint]
}

// GetAllTimelines returns all connection timelines
func (d *ConnectionDebugger) GetAllTimelines() map[string]*ConnectionTimeline {
	d.mu.RLock()
	defer d.mu.RUnlock()

	result := make(map[string]*ConnectionTimeline)
	for k, v := range d.timelines {
		result[k] = v
	}
	return result
}

// Clear clears all debug data
func (d *ConnectionDebugger) Clear() {
	d.mu.Lock()
	defer d.mu.Unlock()

	d.events = d.events[:0]
	d.traces = make(map[string]*MessageTrace)
	d.activeTraces = make(map[string]*MessageTrace)
	d.timelines = make(map[string]*ConnectionTimeline)
}

// EventFilter filters connection events
type EventFilter struct {
	Types     []ConnectionEventType
	Endpoints []string
	StartTime time.Time
	EndTime   time.Time
	HasError  *bool
}

func (f *EventFilter) matches(event ConnectionEvent) bool {
	if len(f.Types) > 0 {
		matched := false
		for _, t := range f.Types {
			if event.Type == t {
				matched = true
				break
			}
		}
		if !matched {
			return false
		}
	}

	if len(f.Endpoints) > 0 {
		matched := false
		for _, e := range f.Endpoints {
			if event.Endpoint == e {
				matched = true
				break
			}
		}
		if !matched {
			return false
		}
	}

	if !f.StartTime.IsZero() && event.Timestamp.Before(f.StartTime) {
		return false
	}

	if !f.EndTime.IsZero() && event.Timestamp.After(f.EndTime) {
		return false
	}

	if f.HasError != nil {
		hasErr := event.Error != ""
		if *f.HasError != hasErr {
			return false
		}
	}

	return true
}

// TraceFilter filters message traces
type TraceFilter struct {
	Subjects  []string
	Sources   []string
	Statuses  []string
	StartTime time.Time
	EndTime   time.Time
	MinHops   int
	MaxHops   int
}

func (f *TraceFilter) matches(trace *MessageTrace) bool {
	if len(f.Subjects) > 0 {
		matched := false
		for _, s := range f.Subjects {
			if matchSubject(s, trace.Subject) {
				matched = true
				break
			}
		}
		if !matched {
			return false
		}
	}

	if len(f.Sources) > 0 {
		matched := false
		for _, s := range f.Sources {
			if trace.Source == s {
				matched = true
				break
			}
		}
		if !matched {
			return false
		}
	}

	if len(f.Statuses) > 0 {
		matched := false
		for _, s := range f.Statuses {
			if trace.Status == s {
				matched = true
				break
			}
		}
		if !matched {
			return false
		}
	}

	if !f.StartTime.IsZero() && trace.StartTime.Before(f.StartTime) {
		return false
	}

	if !f.EndTime.IsZero() && trace.EndTime.After(f.EndTime) {
		return false
	}

	if f.MinHops > 0 && len(trace.Hops) < f.MinHops {
		return false
	}

	if f.MaxHops > 0 && len(trace.Hops) > f.MaxHops {
		return false
	}

	return true
}

// matchSubject checks if a subject matches a pattern with wildcards
func matchSubject(pattern, subject string) bool {
	if pattern == "*" || pattern == ">" {
		return true
	}
	if pattern == subject {
		return true
	}
	// Simple wildcard matching
	if strings.HasSuffix(pattern, ">") {
		prefix := strings.TrimSuffix(pattern, ">")
		return strings.HasPrefix(subject, prefix)
	}
	return false
}

// DiagnosticReport generates a diagnostic report
type DiagnosticReport struct {
	GeneratedAt     time.Time                      `json:"generated_at"`
	Endpoints       map[string]EndpointDiagnostic  `json:"endpoints"`
	ActiveTraces    int                            `json:"active_traces"`
	RecentErrors    []ConnectionEvent              `json:"recent_errors"`
	LatencySpikes   []ConnectionEvent              `json:"latency_spikes"`
	Recommendations []string                       `json:"recommendations"`
}

// EndpointDiagnostic contains diagnostic info for an endpoint
type EndpointDiagnostic struct {
	Endpoint    string          `json:"endpoint"`
	Status      string          `json:"status"`
	Uptime      time.Duration   `json:"uptime"`
	LastError   *ConnectionEvent `json:"last_error,omitempty"`
	AvgLatency  time.Duration   `json:"avg_latency"`
	ErrorRate   float64         `json:"error_rate"`
	Healthy     bool            `json:"healthy"`
}

// GenerateDiagnosticReport generates a comprehensive diagnostic report
func (d *ConnectionDebugger) GenerateDiagnosticReport() *DiagnosticReport {
	d.mu.RLock()
	defer d.mu.RUnlock()

	report := &DiagnosticReport{
		GeneratedAt:     time.Now(),
		Endpoints:       make(map[string]EndpointDiagnostic),
		ActiveTraces:    len(d.activeTraces),
		RecentErrors:    make([]ConnectionEvent, 0),
		LatencySpikes:   make([]ConnectionEvent, 0),
		Recommendations: make([]string, 0),
	}

	// Analyze endpoints
	for endpoint, timeline := range d.timelines {
		diag := EndpointDiagnostic{
			Endpoint:   endpoint,
			Uptime:     timeline.Summary.Uptime,
			AvgLatency: timeline.Summary.AvgLatency,
		}

		// Calculate error rate
		totalEvents := len(timeline.Events)
		if totalEvents > 0 {
			diag.ErrorRate = float64(timeline.Summary.TotalErrors) / float64(totalEvents)
		}

		// Determine status
		if len(timeline.Events) > 0 {
			lastEvent := timeline.Events[len(timeline.Events)-1]
			switch lastEvent.Type {
			case EventTypeConnect:
				diag.Status = "connected"
				diag.Healthy = true
			case EventTypeDisconnect:
				diag.Status = "disconnected"
				diag.Healthy = false
			case EventTypeError:
				diag.Status = "error"
				diag.Healthy = false
				diag.LastError = &lastEvent
			default:
				diag.Status = "unknown"
			}
		}

		report.Endpoints[endpoint] = diag
	}

	// Collect recent errors and latency spikes
	for _, event := range d.events {
		if event.Type == EventTypeError && len(report.RecentErrors) < 10 {
			report.RecentErrors = append(report.RecentErrors, event)
		}
		if event.Type == EventTypeLatencySpike && len(report.LatencySpikes) < 10 {
			report.LatencySpikes = append(report.LatencySpikes, event)
		}
	}

	// Generate recommendations
	for _, diag := range report.Endpoints {
		if diag.ErrorRate > 0.1 {
			report.Recommendations = append(report.Recommendations,
				fmt.Sprintf("High error rate (%.1f%%) on endpoint %s - consider investigating network issues", diag.ErrorRate*100, diag.Endpoint))
		}
		if diag.AvgLatency > 100*time.Millisecond {
			report.Recommendations = append(report.Recommendations,
				fmt.Sprintf("High latency (%s) on endpoint %s - consider using a closer endpoint", diag.AvgLatency, diag.Endpoint))
		}
		if !diag.Healthy {
			report.Recommendations = append(report.Recommendations,
				fmt.Sprintf("Endpoint %s is unhealthy - check connection and credentials", diag.Endpoint))
		}
	}

	if report.ActiveTraces > 100 {
		report.Recommendations = append(report.Recommendations,
			fmt.Sprintf("High number of active traces (%d) - possible message delivery delays", report.ActiveTraces))
	}

	return report
}

// ExportJSON exports debug data as JSON
func (d *ConnectionDebugger) ExportJSON() (string, error) {
	d.mu.RLock()
	defer d.mu.RUnlock()

	data := struct {
		Events    []ConnectionEvent              `json:"events"`
		Traces    map[string]*MessageTrace       `json:"traces"`
		Timelines map[string]*ConnectionTimeline `json:"timelines"`
	}{
		Events:    d.events,
		Traces:    d.traces,
		Timelines: d.timelines,
	}

	bytes, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return "", err
	}
	return string(bytes), nil
}

// DiagnosticCLI provides CLI commands for diagnostics
type DiagnosticCLI struct {
	debugger  *ConnectionDebugger
	collector *NATSMetricsCollector
}

// NewDiagnosticCLI creates a new diagnostic CLI
func NewDiagnosticCLI(debugger *ConnectionDebugger, collector *NATSMetricsCollector) *DiagnosticCLI {
	return &DiagnosticCLI{
		debugger:  debugger,
		collector: collector,
	}
}

// StatusCommand returns connection status
func (cli *DiagnosticCLI) StatusCommand() string {
	var output strings.Builder

	output.WriteString("=== NATS Mesh Connection Status ===\n\n")

	// Endpoint status
	timelines := cli.debugger.GetAllTimelines()
	if len(timelines) == 0 {
		output.WriteString("No endpoints tracked.\n")
	} else {
		output.WriteString("Endpoints:\n")
		for endpoint, timeline := range timelines {
			status := "unknown"
			if len(timeline.Events) > 0 {
				lastEvent := timeline.Events[len(timeline.Events)-1]
				switch lastEvent.Type {
				case EventTypeConnect:
					status = "✓ connected"
				case EventTypeDisconnect:
					status = "✗ disconnected"
				case EventTypeError:
					status = "⚠ error: " + lastEvent.Error
				}
			}
			output.WriteString(fmt.Sprintf("  %s: %s\n", endpoint, status))
			output.WriteString(fmt.Sprintf("    Uptime: %s, Avg Latency: %s\n", timeline.Summary.Uptime, timeline.Summary.AvgLatency))
		}
	}

	// Metrics summary
	if cli.collector != nil {
		stats := cli.collector.GetStats()
		output.WriteString(fmt.Sprintf("\nMessages: sent=%d, received=%d\n", stats.TotalMessagesSent, stats.TotalMessagesReceived))
		output.WriteString(fmt.Sprintf("Delivery: pending=%d, acked=%d, failed=%d\n", stats.DeliveryPending, stats.DeliveryAcked, stats.DeliveryFailed))
		output.WriteString(fmt.Sprintf("Duplicates detected: %d\n", stats.DuplicatesDetected))
	}

	return output.String()
}

// EventsCommand returns recent events
func (cli *DiagnosticCLI) EventsCommand(limit int) string {
	var output strings.Builder

	events := cli.debugger.GetEvents(nil)
	if len(events) == 0 {
		return "No events recorded.\n"
	}

	output.WriteString("=== Recent Connection Events ===\n\n")

	start := 0
	if limit > 0 && len(events) > limit {
		start = len(events) - limit
	}

	for _, event := range events[start:] {
		output.WriteString(fmt.Sprintf("[%s] %s on %s",
			event.Timestamp.Format("15:04:05.000"),
			event.Type,
			event.Endpoint))
		if event.Error != "" {
			output.WriteString(fmt.Sprintf(" - error: %s", event.Error))
		}
		if event.Latency > 0 {
			output.WriteString(fmt.Sprintf(" - latency: %s", event.Latency))
		}
		output.WriteString("\n")
	}

	return output.String()
}

// TraceCommand returns message trace info
func (cli *DiagnosticCLI) TraceCommand(traceID string) string {
	trace := cli.debugger.GetTrace(traceID)
	if trace == nil {
		return fmt.Sprintf("Trace %s not found.\n", traceID)
	}

	var output strings.Builder
	output.WriteString(fmt.Sprintf("=== Message Trace: %s ===\n\n", traceID))
	output.WriteString(fmt.Sprintf("Message ID: %s\n", trace.MessageID))
	output.WriteString(fmt.Sprintf("Subject: %s\n", trace.Subject))
	output.WriteString(fmt.Sprintf("Source: %s\n", trace.Source))
	output.WriteString(fmt.Sprintf("Destination: %s\n", trace.Destination))
	output.WriteString(fmt.Sprintf("Status: %s\n", trace.Status))
	output.WriteString(fmt.Sprintf("Total Latency: %s\n", trace.TotalLatency))
	output.WriteString(fmt.Sprintf("Size: %d bytes\n", trace.Size))

	if len(trace.Hops) > 0 {
		output.WriteString("\nHops:\n")
		for _, hop := range trace.Hops {
			output.WriteString(fmt.Sprintf("  %d. %s @ %s: %s (%s)",
				hop.HopNumber, hop.Component, hop.Endpoint, hop.Action, hop.Latency))
			if hop.Error != "" {
				output.WriteString(fmt.Sprintf(" - error: %s", hop.Error))
			}
			output.WriteString("\n")
		}
	}

	if trace.Error != "" {
		output.WriteString(fmt.Sprintf("\nError: %s\n", trace.Error))
	}

	return output.String()
}

// DiagnoseCommand runs diagnostics and returns a report
func (cli *DiagnosticCLI) DiagnoseCommand() string {
	report := cli.debugger.GenerateDiagnosticReport()

	var output strings.Builder
	output.WriteString("=== NATS Mesh Diagnostic Report ===\n")
	output.WriteString(fmt.Sprintf("Generated: %s\n\n", report.GeneratedAt.Format(time.RFC3339)))

	// Endpoint summary
	output.WriteString("Endpoints:\n")
	for _, diag := range report.Endpoints {
		healthIcon := "✓"
		if !diag.Healthy {
			healthIcon = "✗"
		}
		output.WriteString(fmt.Sprintf("  %s %s: %s (uptime: %s, latency: %s, error rate: %.1f%%)\n",
			healthIcon, diag.Endpoint, diag.Status, diag.Uptime, diag.AvgLatency, diag.ErrorRate*100))
	}

	// Recent errors
	if len(report.RecentErrors) > 0 {
		output.WriteString("\nRecent Errors:\n")
		for _, err := range report.RecentErrors {
			output.WriteString(fmt.Sprintf("  [%s] %s: %s\n",
				err.Timestamp.Format("15:04:05"), err.Endpoint, err.Error))
		}
	}

	// Recommendations
	if len(report.Recommendations) > 0 {
		output.WriteString("\nRecommendations:\n")
		for _, rec := range report.Recommendations {
			output.WriteString(fmt.Sprintf("  • %s\n", rec))
		}
	} else {
		output.WriteString("\nNo issues detected.\n")
	}

	return output.String()
}

// LatencyTest performs a latency test
func (cli *DiagnosticCLI) LatencyTest(ctx context.Context, endpoint string, count int) string {
	var output strings.Builder
	output.WriteString(fmt.Sprintf("=== Latency Test: %s (%d samples) ===\n\n", endpoint, count))

	// This is a placeholder - in real implementation, this would:
	// 1. Send test messages to the endpoint
	// 2. Measure round-trip time
	// 3. Calculate statistics

	output.WriteString("Note: Latency test requires active connection to endpoint.\n")
	output.WriteString("Use 'kscore nats ping <endpoint>' for live latency testing.\n")

	// Return cached latency data if available
	if timeline := cli.debugger.GetTimeline(endpoint); timeline != nil {
		output.WriteString(fmt.Sprintf("\nCached Statistics:\n"))
		output.WriteString(fmt.Sprintf("  Average Latency: %s\n", timeline.Summary.AvgLatency))
		output.WriteString(fmt.Sprintf("  Total Connections: %d\n", timeline.Summary.TotalConnections))
		output.WriteString(fmt.Sprintf("  MTBF: %s\n", timeline.Summary.MTBF))
		output.WriteString(fmt.Sprintf("  MTTR: %s\n", timeline.Summary.MTTR))
	}

	return output.String()
}
