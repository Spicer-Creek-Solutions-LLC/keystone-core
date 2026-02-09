package events

import (
	"fmt"
	"io"
	"sort"
	"strings"
	"time"
)

// PrometheusExporter exports metrics in Prometheus format
type PrometheusExporter struct {
	collector MetricsCollector
}

// NewPrometheusExporter creates a new Prometheus exporter
func NewPrometheusExporter(collector MetricsCollector) *PrometheusExporter {
	return &PrometheusExporter{
		collector: collector,
	}
}

// Export writes metrics in Prometheus format
func (e *PrometheusExporter) Export(w io.Writer) error {
	metrics := e.collector.GetMetrics()

	// Write header
	fmt.Fprintf(w, "# HELP kscore_events_published_total Total number of events published by type\n")
	fmt.Fprintf(w, "# TYPE kscore_events_published_total counter\n")
	for eventType, count := range metrics.EventsPublished {
		fmt.Fprintf(w, "kscore_events_published_total{type=\"%s\"} %d\n", eventType, count)
	}
	fmt.Fprintln(w)

	// Events received
	fmt.Fprintf(w, "# HELP kscore_events_received_total Total number of events received by type\n")
	fmt.Fprintf(w, "# TYPE kscore_events_received_total counter\n")
	for eventType, count := range metrics.EventsReceived {
		fmt.Fprintf(w, "kscore_events_received_total{type=\"%s\"} %d\n", eventType, count)
	}
	fmt.Fprintln(w)

	// Events processed
	fmt.Fprintf(w, "# HELP kscore_events_processed_total Total number of events processed by type\n")
	fmt.Fprintf(w, "# TYPE kscore_events_processed_total counter\n")
	for eventType, count := range metrics.EventsProcessed {
		fmt.Fprintf(w, "kscore_events_processed_total{type=\"%s\"} %d\n", eventType, count)
	}
	fmt.Fprintln(w)

	// Events failed
	fmt.Fprintf(w, "# HELP kscore_events_failed_total Total number of events that failed processing\n")
	fmt.Fprintf(w, "# TYPE kscore_events_failed_total counter\n")
	for eventType, count := range metrics.EventsFailed {
		fmt.Fprintf(w, "kscore_events_failed_total{type=\"%s\"} %d\n", eventType, count)
	}
	fmt.Fprintln(w)

	// Events by severity
	fmt.Fprintf(w, "# HELP kscore_events_severity_total Total number of events by severity\n")
	fmt.Fprintf(w, "# TYPE kscore_events_severity_total counter\n")
	for severity, count := range metrics.EventsBySeverity {
		fmt.Fprintf(w, "kscore_events_severity_total{severity=\"%s\"} %d\n", severity, count)
	}
	fmt.Fprintln(w)

	// Publisher errors
	fmt.Fprintf(w, "# HELP kscore_publisher_errors_total Total number of publisher errors\n")
	fmt.Fprintf(w, "# TYPE kscore_publisher_errors_total counter\n")
	fmt.Fprintf(w, "kscore_publisher_errors_total %d\n", metrics.PublisherErrors)
	fmt.Fprintln(w)

	// Subscriber errors
	fmt.Fprintf(w, "# HELP kscore_subscriber_errors_total Total number of subscriber errors\n")
	fmt.Fprintf(w, "# TYPE kscore_subscriber_errors_total counter\n")
	fmt.Fprintf(w, "kscore_subscriber_errors_total %d\n", metrics.SubscriberErrors)
	fmt.Fprintln(w)

	// Active subscribers
	fmt.Fprintf(w, "# HELP kscore_active_subscribers Number of active subscribers\n")
	fmt.Fprintf(w, "# TYPE kscore_active_subscribers gauge\n")
	fmt.Fprintf(w, "kscore_active_subscribers %d\n", metrics.ActiveSubscribers)
	fmt.Fprintln(w)

	// Reactor executions
	if len(metrics.ReactorExecutions) > 0 {
		fmt.Fprintf(w, "# HELP kscore_reactor_executions_total Total number of reactor executions\n")
		fmt.Fprintf(w, "# TYPE kscore_reactor_executions_total counter\n")
		for reactorID, count := range metrics.ReactorExecutions {
			fmt.Fprintf(w, "kscore_reactor_executions_total{reactor=\"%s\"} %d\n", reactorID, count)
		}
		fmt.Fprintln(w)
	}

	// Reactor failures
	if len(metrics.ReactorFailures) > 0 {
		fmt.Fprintf(w, "# HELP kscore_reactor_failures_total Total number of reactor failures\n")
		fmt.Fprintf(w, "# TYPE kscore_reactor_failures_total counter\n")
		for reactorID, count := range metrics.ReactorFailures {
			fmt.Fprintf(w, "kscore_reactor_failures_total{reactor=\"%s\"} %d\n", reactorID, count)
		}
		fmt.Fprintln(w)
	}

	// Reactor duration summary
	if len(metrics.ReactorDurations) > 0 {
		fmt.Fprintf(w, "# HELP kscore_reactor_duration_seconds Reactor execution duration\n")
		fmt.Fprintf(w, "# TYPE kscore_reactor_duration_seconds summary\n")
		for reactorID, stats := range metrics.ReactorDurations {
			fmt.Fprintf(w, "kscore_reactor_duration_seconds{reactor=\"%s\",quantile=\"0.5\"} %.6f\n",
				reactorID, stats.P50.Seconds())
			fmt.Fprintf(w, "kscore_reactor_duration_seconds{reactor=\"%s\",quantile=\"0.95\"} %.6f\n",
				reactorID, stats.P95.Seconds())
			fmt.Fprintf(w, "kscore_reactor_duration_seconds{reactor=\"%s\",quantile=\"0.99\"} %.6f\n",
				reactorID, stats.P99.Seconds())
			fmt.Fprintf(w, "kscore_reactor_duration_seconds_sum{reactor=\"%s\"} %.6f\n",
				reactorID, stats.Total.Seconds())
			fmt.Fprintf(w, "kscore_reactor_duration_seconds_count{reactor=\"%s\"} %d\n",
				reactorID, stats.Count)
		}
		fmt.Fprintln(w)
	}

	// Action executions
	if len(metrics.ActionExecutions) > 0 {
		fmt.Fprintf(w, "# HELP kscore_action_executions_total Total number of action executions\n")
		fmt.Fprintf(w, "# TYPE kscore_action_executions_total counter\n")
		for action, count := range metrics.ActionExecutions {
			parts := strings.SplitN(action, ":", 2)
			actionType := parts[0]
			actionName := parts[1]
			fmt.Fprintf(w, "kscore_action_executions_total{type=\"%s\",name=\"%s\"} %d\n",
				actionType, actionName, count)
		}
		fmt.Fprintln(w)
	}

	// Action failures
	if len(metrics.ActionFailures) > 0 {
		fmt.Fprintf(w, "# HELP kscore_action_failures_total Total number of action failures\n")
		fmt.Fprintf(w, "# TYPE kscore_action_failures_total counter\n")
		for action, count := range metrics.ActionFailures {
			parts := strings.SplitN(action, ":", 2)
			actionType := parts[0]
			actionName := parts[1]
			fmt.Fprintf(w, "kscore_action_failures_total{type=\"%s\",name=\"%s\"} %d\n",
				actionType, actionName, count)
		}
		fmt.Fprintln(w)
	}

	// Storage operations
	if len(metrics.StorageOperations) > 0 {
		fmt.Fprintf(w, "# HELP kscore_storage_operations_total Total number of storage operations\n")
		fmt.Fprintf(w, "# TYPE kscore_storage_operations_total counter\n")
		for operation, count := range metrics.StorageOperations {
			fmt.Fprintf(w, "kscore_storage_operations_total{operation=\"%s\"} %d\n", operation, count)
		}
		fmt.Fprintln(w)
	}

	// Storage failures
	if len(metrics.StorageFailures) > 0 {
		fmt.Fprintf(w, "# HELP kscore_storage_failures_total Total number of storage failures\n")
		fmt.Fprintf(w, "# TYPE kscore_storage_failures_total counter\n")
		for operation, count := range metrics.StorageFailures {
			fmt.Fprintf(w, "kscore_storage_failures_total{operation=\"%s\"} %d\n", operation, count)
		}
		fmt.Fprintln(w)
	}

	// Processing duration
	if metrics.ProcessingDuration != nil && metrics.ProcessingDuration.Count > 0 {
		fmt.Fprintf(w, "# HELP kscore_event_processing_duration_seconds Event processing duration\n")
		fmt.Fprintf(w, "# TYPE kscore_event_processing_duration_seconds summary\n")
		fmt.Fprintf(w, "kscore_event_processing_duration_seconds{quantile=\"0.5\"} %.6f\n",
			metrics.ProcessingDuration.P50.Seconds())
		fmt.Fprintf(w, "kscore_event_processing_duration_seconds{quantile=\"0.95\"} %.6f\n",
			metrics.ProcessingDuration.P95.Seconds())
		fmt.Fprintf(w, "kscore_event_processing_duration_seconds{quantile=\"0.99\"} %.6f\n",
			metrics.ProcessingDuration.P99.Seconds())
		fmt.Fprintf(w, "kscore_event_processing_duration_seconds_sum %.6f\n",
			metrics.ProcessingDuration.Total.Seconds())
		fmt.Fprintf(w, "kscore_event_processing_duration_seconds_count %d\n",
			metrics.ProcessingDuration.Count)
		fmt.Fprintln(w)
	}

	// Uptime
	fmt.Fprintf(w, "# HELP kscore_uptime_seconds System uptime in seconds\n")
	fmt.Fprintf(w, "# TYPE kscore_uptime_seconds gauge\n")
	fmt.Fprintf(w, "kscore_uptime_seconds %.2f\n", metrics.Uptime.Seconds())
	fmt.Fprintln(w)

	// Event rate
	fmt.Fprintf(w, "# HELP kscore_event_rate Events per second\n")
	fmt.Fprintf(w, "# TYPE kscore_event_rate gauge\n")
	fmt.Fprintf(w, "kscore_event_rate %.2f\n", metrics.EventRate)
	fmt.Fprintln(w)

	// Last event timestamp
	if !metrics.LastEvent.IsZero() {
		fmt.Fprintf(w, "# HELP kscore_last_event_timestamp_seconds Unix timestamp of last event\n")
		fmt.Fprintf(w, "# TYPE kscore_last_event_timestamp_seconds gauge\n")
		fmt.Fprintf(w, "kscore_last_event_timestamp_seconds %d\n", metrics.LastEvent.Unix())
		fmt.Fprintln(w)
	}

	return nil
}

// ExportString returns metrics as a Prometheus-formatted string
func (e *PrometheusExporter) ExportString() (string, error) {
	var builder strings.Builder
	err := e.Export(&builder)
	if err != nil {
		return "", err
	}
	return builder.String(), nil
}

// MetricsSummary provides a human-readable summary of metrics
type MetricsSummary struct {
	TotalPublished        int64
	TotalReceived         int64
	TotalProcessed        int64
	TotalFailed           int64
	TopEventTypes         []EventTypeCount
	ErrorRate             float64
	AverageProcessingTime time.Duration
	Uptime                time.Duration
	EventsPerSecond       float64
}

// EventTypeCount holds event type and count for sorting
type EventTypeCount struct {
	Type  EventType
	Count int64
}

// GetSummary returns a metrics summary
func GetSummary(metrics *Metrics) *MetricsSummary {
	summary := &MetricsSummary{
		Uptime:          metrics.Uptime,
		EventsPerSecond: metrics.EventRate,
	}

	// Calculate totals
	for _, count := range metrics.EventsPublished {
		summary.TotalPublished += count
	}
	for _, count := range metrics.EventsReceived {
		summary.TotalReceived += count
	}
	for _, count := range metrics.EventsProcessed {
		summary.TotalProcessed += count
	}
	for _, count := range metrics.EventsFailed {
		summary.TotalFailed += count
	}

	// Calculate error rate
	totalAttempted := summary.TotalProcessed + summary.TotalFailed
	if totalAttempted > 0 {
		summary.ErrorRate = float64(summary.TotalFailed) / float64(totalAttempted) * 100
	}

	// Get top event types
	var typeCounts []EventTypeCount
	for eventType, count := range metrics.EventsPublished {
		typeCounts = append(typeCounts, EventTypeCount{Type: eventType, Count: count})
	}

	// Sort by count descending
	sort.Slice(typeCounts, func(i, j int) bool {
		return typeCounts[i].Count > typeCounts[j].Count
	})

	// Keep top 10
	if len(typeCounts) > 10 {
		typeCounts = typeCounts[:10]
	}
	summary.TopEventTypes = typeCounts

	// Average processing time
	if metrics.ProcessingDuration != nil && metrics.ProcessingDuration.Count > 0 {
		summary.AverageProcessingTime = metrics.ProcessingDuration.Avg
	}

	return summary
}

// FormatSummary formats a metrics summary as a human-readable string
func FormatSummary(summary *MetricsSummary) string {
	var builder strings.Builder

	builder.WriteString("=== Keystone Core Event System Metrics ===\n\n")

	builder.WriteString(fmt.Sprintf("Uptime: %s\n", summary.Uptime))
	builder.WriteString(fmt.Sprintf("Event Rate: %.2f events/sec\n\n", summary.EventsPerSecond))

	builder.WriteString("Event Counts:\n")
	builder.WriteString(fmt.Sprintf("  Published:  %d\n", summary.TotalPublished))
	builder.WriteString(fmt.Sprintf("  Received:   %d\n", summary.TotalReceived))
	builder.WriteString(fmt.Sprintf("  Processed:  %d\n", summary.TotalProcessed))
	builder.WriteString(fmt.Sprintf("  Failed:     %d\n", summary.TotalFailed))
	builder.WriteString(fmt.Sprintf("  Error Rate: %.2f%%\n\n", summary.ErrorRate))

	if summary.AverageProcessingTime > 0 {
		builder.WriteString(fmt.Sprintf("Average Processing Time: %s\n\n", summary.AverageProcessingTime))
	}

	if len(summary.TopEventTypes) > 0 {
		builder.WriteString("Top Event Types:\n")
		for i, tc := range summary.TopEventTypes {
			builder.WriteString(fmt.Sprintf("  %d. %-30s %d\n", i+1, tc.Type, tc.Count))
		}
	}

	return builder.String()
}
