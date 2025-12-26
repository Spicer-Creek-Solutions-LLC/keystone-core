package events

import (
	"strings"
	"testing"
	"time"
)

func TestPrometheusExporter_Export(t *testing.T) {
	collector := NewMetricsCollector()

	// Record some metrics
	collector.RecordEventPublished(EventTypeAgentConnect, SeverityInfo)
	collector.RecordEventPublished(EventTypeJobStart, SeverityWarning)
	collector.RecordEventReceived(EventTypeAgentConnect, SeverityInfo)
	collector.RecordEventProcessed(EventTypeAgentConnect, 100*time.Millisecond, true)
	collector.RecordPublisherError(EventTypeAgentConnect)
	collector.RecordSubscriberError("agent.>")
	collector.RecordReactorExecution("restart-service", 50*time.Millisecond, true)
	collector.RecordActionExecution("restart-nginx", "service", 150*time.Millisecond, true)
	collector.RecordStorageOperation("Store", 10*time.Millisecond, true)

	exporter := NewPrometheusExporter(collector)

	var buf strings.Builder
	err := exporter.Export(&buf)
	if err != nil {
		t.Fatalf("Export failed: %v", err)
	}

	output := buf.String()

	// Check for HELP and TYPE comments
	if !strings.Contains(output, "# HELP titananvil_events_published_total") {
		t.Error("Expected HELP comment for events_published_total")
	}

	if !strings.Contains(output, "# TYPE titananvil_events_published_total counter") {
		t.Error("Expected TYPE comment for events_published_total")
	}

	// Check for metric values
	if !strings.Contains(output, "titananvil_events_published_total{type=\"agent.connect\"} 1") {
		t.Error("Expected AgentConnect published metric")
	}

	if !strings.Contains(output, "titananvil_events_published_total{type=\"job.start\"} 1") {
		t.Error("Expected JobStart published metric")
	}

	if !strings.Contains(output, "titananvil_events_received_total{type=\"agent.connect\"} 1") {
		t.Error("Expected AgentConnect received metric")
	}

	if !strings.Contains(output, "titananvil_events_processed_total{type=\"agent.connect\"} 1") {
		t.Error("Expected AgentConnect processed metric")
	}

	if !strings.Contains(output, "titananvil_publisher_errors_total 1") {
		t.Error("Expected publisher errors metric")
	}

	if !strings.Contains(output, "titananvil_subscriber_errors_total 1") {
		t.Error("Expected subscriber errors metric")
	}

	if !strings.Contains(output, "titananvil_reactor_executions_total{reactor=\"restart-service\"} 1") {
		t.Error("Expected reactor executions metric")
	}

	if !strings.Contains(output, "titananvil_action_executions_total{type=\"service\",name=\"restart-nginx\"} 1") {
		t.Error("Expected action executions metric")
	}

	if !strings.Contains(output, "titananvil_storage_operations_total{operation=\"Store\"} 1") {
		t.Error("Expected storage operations metric")
	}
}

func TestPrometheusExporter_Export_EventsBySeverity(t *testing.T) {
	collector := NewMetricsCollector()

	collector.RecordEventPublished(EventTypeAgentConnect, SeverityInfo)
	collector.RecordEventPublished(EventTypeJobStart, SeverityWarning)
	collector.RecordEventPublished(EventTypeJobComplete, SeverityError)

	exporter := NewPrometheusExporter(collector)

	var buf strings.Builder
	exporter.Export(&buf)

	output := buf.String()

	if !strings.Contains(output, "# HELP titananvil_events_severity_total") {
		t.Error("Expected HELP comment for events_severity_total")
	}

	if !strings.Contains(output, "titananvil_events_severity_total{severity=\"info\"} 1") {
		t.Error("Expected info severity metric")
	}

	if !strings.Contains(output, "titananvil_events_severity_total{severity=\"warning\"} 1") {
		t.Error("Expected warning severity metric")
	}

	if !strings.Contains(output, "titananvil_events_severity_total{severity=\"error\"} 1") {
		t.Error("Expected error severity metric")
	}
}

func TestPrometheusExporter_Export_ActiveSubscribers(t *testing.T) {
	collector := NewMetricsCollector().(*DefaultMetricsCollector)

	// Set active subscribers
	collector.metrics.ActiveSubscribers = 5

	exporter := NewPrometheusExporter(collector)

	var buf strings.Builder
	exporter.Export(&buf)

	output := buf.String()

	if !strings.Contains(output, "# TYPE titananvil_active_subscribers gauge") {
		t.Error("Expected gauge type for active_subscribers")
	}

	if !strings.Contains(output, "titananvil_active_subscribers 5") {
		t.Error("Expected active subscribers metric")
	}
}

func TestPrometheusExporter_Export_ReactorDurations(t *testing.T) {
	collector := NewMetricsCollector()

	// Record multiple executions to get meaningful stats
	for i := 0; i < 10; i++ {
		collector.RecordReactorExecution("restart-service", time.Duration(50+i*10)*time.Millisecond, true)
	}

	exporter := NewPrometheusExporter(collector)

	var buf strings.Builder
	exporter.Export(&buf)

	output := buf.String()

	if !strings.Contains(output, "# HELP titananvil_reactor_duration_seconds") {
		t.Error("Expected HELP comment for reactor_duration_seconds")
	}

	if !strings.Contains(output, "# TYPE titananvil_reactor_duration_seconds summary") {
		t.Error("Expected summary type for reactor_duration_seconds")
	}

	if !strings.Contains(output, "titananvil_reactor_duration_seconds{reactor=\"restart-service\",quantile=\"0.5\"}") {
		t.Error("Expected P50 quantile")
	}

	if !strings.Contains(output, "titananvil_reactor_duration_seconds{reactor=\"restart-service\",quantile=\"0.95\"}") {
		t.Error("Expected P95 quantile")
	}

	if !strings.Contains(output, "titananvil_reactor_duration_seconds{reactor=\"restart-service\",quantile=\"0.99\"}") {
		t.Error("Expected P99 quantile")
	}

	if !strings.Contains(output, "titananvil_reactor_duration_seconds_sum{reactor=\"restart-service\"}") {
		t.Error("Expected sum")
	}

	if !strings.Contains(output, "titananvil_reactor_duration_seconds_count{reactor=\"restart-service\"} 10") {
		t.Error("Expected count of 10")
	}
}

func TestPrometheusExporter_Export_ActionDurations(t *testing.T) {
	collector := NewMetricsCollector()

	// Record multiple executions
	for i := 0; i < 5; i++ {
		collector.RecordActionExecution("restart-nginx", "service", time.Duration(100+i*20)*time.Millisecond, true)
	}

	exporter := NewPrometheusExporter(collector)

	var buf strings.Builder
	exporter.Export(&buf)

	output := buf.String()

	// Note: ActionDurations are not currently exported in prometheus.go
	// This test documents expected behavior if we add it
	if strings.Contains(output, "titananvil_action_duration_seconds") {
		// If action durations are exported, verify format
		if !strings.Contains(output, "# TYPE titananvil_action_duration_seconds summary") {
			t.Error("Expected summary type for action_duration_seconds")
		}
	}
}

func TestPrometheusExporter_Export_ProcessingDuration(t *testing.T) {
	collector := NewMetricsCollector()

	// Record multiple events
	for i := 0; i < 20; i++ {
		collector.RecordEventProcessed(EventTypeAgentConnect, time.Duration(50+i*5)*time.Millisecond, true)
	}

	exporter := NewPrometheusExporter(collector)

	var buf strings.Builder
	exporter.Export(&buf)

	output := buf.String()

	if !strings.Contains(output, "# HELP titananvil_event_processing_duration_seconds") {
		t.Error("Expected HELP comment for event_processing_duration_seconds")
	}

	if !strings.Contains(output, "# TYPE titananvil_event_processing_duration_seconds summary") {
		t.Error("Expected summary type for event_processing_duration_seconds")
	}

	if !strings.Contains(output, "titananvil_event_processing_duration_seconds{quantile=\"0.5\"}") {
		t.Error("Expected P50 quantile")
	}

	if !strings.Contains(output, "titananvil_event_processing_duration_seconds_count 20") {
		t.Error("Expected count of 20")
	}
}

func TestPrometheusExporter_Export_Uptime(t *testing.T) {
	collector := NewMetricsCollector()

	time.Sleep(100 * time.Millisecond)

	exporter := NewPrometheusExporter(collector)

	var buf strings.Builder
	exporter.Export(&buf)

	output := buf.String()

	if !strings.Contains(output, "# HELP titananvil_uptime_seconds") {
		t.Error("Expected HELP comment for uptime_seconds")
	}

	if !strings.Contains(output, "# TYPE titananvil_uptime_seconds gauge") {
		t.Error("Expected gauge type for uptime_seconds")
	}

	if !strings.Contains(output, "titananvil_uptime_seconds") {
		t.Error("Expected uptime metric")
	}
}

func TestPrometheusExporter_Export_EventRate(t *testing.T) {
	collector := NewMetricsCollector()

	// Record some events
	for i := 0; i < 10; i++ {
		collector.RecordEventPublished(EventTypeAgentConnect, SeverityInfo)
	}

	time.Sleep(100 * time.Millisecond)

	exporter := NewPrometheusExporter(collector)

	var buf strings.Builder
	exporter.Export(&buf)

	output := buf.String()

	if !strings.Contains(output, "# HELP titananvil_event_rate") {
		t.Error("Expected HELP comment for event_rate")
	}

	if !strings.Contains(output, "# TYPE titananvil_event_rate gauge") {
		t.Error("Expected gauge type for event_rate")
	}

	if !strings.Contains(output, "titananvil_event_rate") {
		t.Error("Expected event rate metric")
	}
}

func TestPrometheusExporter_Export_LastEventTimestamp(t *testing.T) {
	collector := NewMetricsCollector()

	// Record an event to set last event time
	collector.RecordEventPublished(EventTypeAgentConnect, SeverityInfo)

	exporter := NewPrometheusExporter(collector)

	var buf strings.Builder
	exporter.Export(&buf)

	output := buf.String()

	if !strings.Contains(output, "# HELP titananvil_last_event_timestamp_seconds") {
		t.Error("Expected HELP comment for last_event_timestamp_seconds")
	}

	if !strings.Contains(output, "# TYPE titananvil_last_event_timestamp_seconds gauge") {
		t.Error("Expected gauge type for last_event_timestamp_seconds")
	}

	if !strings.Contains(output, "titananvil_last_event_timestamp_seconds") {
		t.Error("Expected last event timestamp metric")
	}
}

func TestPrometheusExporter_Export_ReactorFailures(t *testing.T) {
	collector := NewMetricsCollector()

	collector.RecordReactorExecution("restart-service", 50*time.Millisecond, true)
	collector.RecordReactorExecution("restart-service", 60*time.Millisecond, false)
	collector.RecordReactorExecution("send-alert", 20*time.Millisecond, false)

	exporter := NewPrometheusExporter(collector)

	var buf strings.Builder
	exporter.Export(&buf)

	output := buf.String()

	if !strings.Contains(output, "# HELP titananvil_reactor_failures_total") {
		t.Error("Expected HELP comment for reactor_failures_total")
	}

	if !strings.Contains(output, "titananvil_reactor_failures_total{reactor=\"restart-service\"} 1") {
		t.Error("Expected restart-service failure metric")
	}

	if !strings.Contains(output, "titananvil_reactor_failures_total{reactor=\"send-alert\"} 1") {
		t.Error("Expected send-alert failure metric")
	}
}

func TestPrometheusExporter_Export_ActionFailures(t *testing.T) {
	collector := NewMetricsCollector()

	collector.RecordActionExecution("restart-nginx", "service", 100*time.Millisecond, true)
	collector.RecordActionExecution("restart-nginx", "service", 110*time.Millisecond, false)
	collector.RecordActionExecution("send-webhook", "http", 30*time.Millisecond, false)

	exporter := NewPrometheusExporter(collector)

	var buf strings.Builder
	exporter.Export(&buf)

	output := buf.String()

	if !strings.Contains(output, "# HELP titananvil_action_failures_total") {
		t.Error("Expected HELP comment for action_failures_total")
	}

	if !strings.Contains(output, "titananvil_action_failures_total{type=\"service\",name=\"restart-nginx\"} 1") {
		t.Error("Expected restart-nginx failure metric")
	}

	if !strings.Contains(output, "titananvil_action_failures_total{type=\"http\",name=\"send-webhook\"} 1") {
		t.Error("Expected send-webhook failure metric")
	}
}

func TestPrometheusExporter_Export_StorageFailures(t *testing.T) {
	collector := NewMetricsCollector()

	collector.RecordStorageOperation("Store", 10*time.Millisecond, true)
	collector.RecordStorageOperation("Store", 12*time.Millisecond, false)
	collector.RecordStorageOperation("Query", 50*time.Millisecond, false)

	exporter := NewPrometheusExporter(collector)

	var buf strings.Builder
	exporter.Export(&buf)

	output := buf.String()

	if !strings.Contains(output, "# HELP titananvil_storage_failures_total") {
		t.Error("Expected HELP comment for storage_failures_total")
	}

	if !strings.Contains(output, "titananvil_storage_failures_total{operation=\"Store\"} 1") {
		t.Error("Expected Store failure metric")
	}

	if !strings.Contains(output, "titananvil_storage_failures_total{operation=\"Query\"} 1") {
		t.Error("Expected Query failure metric")
	}
}

func TestPrometheusExporter_ExportString(t *testing.T) {
	collector := NewMetricsCollector()

	collector.RecordEventPublished(EventTypeAgentConnect, SeverityInfo)

	exporter := NewPrometheusExporter(collector)

	output, err := exporter.ExportString()
	if err != nil {
		t.Fatalf("ExportString failed: %v", err)
	}

	if output == "" {
		t.Error("Expected non-empty output")
	}

	if !strings.Contains(output, "titananvil_events_published_total{type=\"agent.connect\"} 1") {
		t.Error("Expected AgentConnect metric in output")
	}
}

func TestPrometheusExporter_Export_EmptyMetrics(t *testing.T) {
	collector := NewMetricsCollector()

	exporter := NewPrometheusExporter(collector)

	var buf strings.Builder
	err := exporter.Export(&buf)
	if err != nil {
		t.Fatalf("Export failed: %v", err)
	}

	output := buf.String()

	// Should still have headers and basic metrics
	if !strings.Contains(output, "# HELP titananvil_events_published_total") {
		t.Error("Expected HELP comments even with no events")
	}

	if !strings.Contains(output, "titananvil_publisher_errors_total 0") {
		t.Error("Expected zero publisher errors")
	}

	if !strings.Contains(output, "titananvil_subscriber_errors_total 0") {
		t.Error("Expected zero subscriber errors")
	}

	if !strings.Contains(output, "titananvil_active_subscribers 0") {
		t.Error("Expected zero active subscribers")
	}
}

func TestGetSummary_TopEventTypes_Sorting(t *testing.T) {
	metrics := &Metrics{
		EventsPublished: map[EventType]int64{
			EventTypeAgentConnect:    10,
			EventTypeJobStart:        100,
			EventTypeStateApplyStart: 50,
			EventTypeJobComplete:     75,
		},
		EventsReceived:     make(map[EventType]int64),
		EventsProcessed:    make(map[EventType]int64),
		EventsFailed:       make(map[EventType]int64),
		ProcessingDuration: NewDurationStats(),
	}

	summary := GetSummary(metrics)

	// Should be sorted by count descending
	if len(summary.TopEventTypes) != 4 {
		t.Errorf("Expected 4 event types, got %d", len(summary.TopEventTypes))
	}

	if summary.TopEventTypes[0].Type != EventTypeJobStart || summary.TopEventTypes[0].Count != 100 {
		t.Errorf("Expected JobStart (100) first, got %s (%d)", summary.TopEventTypes[0].Type, summary.TopEventTypes[0].Count)
	}

	if summary.TopEventTypes[1].Type != EventTypeJobComplete || summary.TopEventTypes[1].Count != 75 {
		t.Errorf("Expected JobComplete (75) second, got %s (%d)", summary.TopEventTypes[1].Type, summary.TopEventTypes[1].Count)
	}

	if summary.TopEventTypes[2].Type != EventTypeStateApplyStart || summary.TopEventTypes[2].Count != 50 {
		t.Errorf("Expected StateApplyStart (50) third, got %s (%d)", summary.TopEventTypes[2].Type, summary.TopEventTypes[2].Count)
	}
}

func TestGetSummary_TopEventTypes_Limit(t *testing.T) {
	metrics := &Metrics{
		EventsPublished:    make(map[EventType]int64),
		EventsReceived:     make(map[EventType]int64),
		EventsProcessed:    make(map[EventType]int64),
		EventsFailed:       make(map[EventType]int64),
		ProcessingDuration: NewDurationStats(),
	}

	// Add more than 10 event types
	for i := 0; i < 15; i++ {
		eventType := EventType("test.event." + string(rune('a'+i)))
		metrics.EventsPublished[eventType] = int64(i + 1)
	}

	summary := GetSummary(metrics)

	// Should only keep top 10
	if len(summary.TopEventTypes) != 10 {
		t.Errorf("Expected 10 event types, got %d", len(summary.TopEventTypes))
	}
}

func TestGetSummary_ErrorRate_Zero(t *testing.T) {
	metrics := &Metrics{
		EventsPublished:    make(map[EventType]int64),
		EventsReceived:     make(map[EventType]int64),
		EventsProcessed:    make(map[EventType]int64),
		EventsFailed:       make(map[EventType]int64),
		ProcessingDuration: NewDurationStats(),
	}

	summary := GetSummary(metrics)

	if summary.ErrorRate != 0 {
		t.Errorf("Expected 0%% error rate with no events, got %.2f%%", summary.ErrorRate)
	}
}

func TestFormatSummary_WithoutProcessingTime(t *testing.T) {
	summary := &MetricsSummary{
		TotalPublished:        100,
		TotalReceived:         95,
		TotalProcessed:        90,
		TotalFailed:           5,
		ErrorRate:             5.26,
		Uptime:                1 * time.Hour,
		EventsPerSecond:       0.025,
		TopEventTypes:         []EventTypeCount{},
		AverageProcessingTime: 0, // No processing time
	}

	output := FormatSummary(summary)

	// Should not contain processing time section
	if strings.Contains(output, "Average Processing Time") {
		t.Error("Expected no processing time section when avg is 0")
	}
}

func TestFormatSummary_WithoutTopEvents(t *testing.T) {
	summary := &MetricsSummary{
		TotalPublished:  100,
		TotalReceived:   95,
		TotalProcessed:  90,
		TotalFailed:     5,
		ErrorRate:       5.26,
		Uptime:          1 * time.Hour,
		EventsPerSecond: 0.025,
		TopEventTypes:   []EventTypeCount{}, // No top events
	}

	output := FormatSummary(summary)

	// Should not contain top events section
	if strings.Contains(output, "Top Event Types") {
		t.Error("Expected no top events section when list is empty")
	}
}

func BenchmarkPrometheusExporter_Export(b *testing.B) {
	collector := NewMetricsCollector()

	// Populate with various metrics
	for i := 0; i < 100; i++ {
		collector.RecordEventPublished(EventTypeAgentConnect, SeverityInfo)
		collector.RecordEventProcessed(EventTypeAgentConnect, 10*time.Millisecond, true)
		collector.RecordReactorExecution("reactor1", 50*time.Millisecond, true)
		collector.RecordActionExecution("action1", "type1", 100*time.Millisecond, true)
	}

	exporter := NewPrometheusExporter(collector)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		var buf strings.Builder
		exporter.Export(&buf)
	}
}

func BenchmarkGetSummary(b *testing.B) {
	metrics := &Metrics{
		EventsPublished: map[EventType]int64{
			EventTypeAgentConnect: 1000,
			EventTypeJobStart:     500,
		},
		EventsReceived: map[EventType]int64{
			EventTypeAgentConnect: 1000,
		},
		EventsProcessed: map[EventType]int64{
			EventTypeAgentConnect: 950,
		},
		EventsFailed: map[EventType]int64{
			EventTypeAgentConnect: 50,
		},
		ProcessingDuration: &DurationStats{
			Count: 1000,
			Total: 100 * time.Second,
			Avg:   100 * time.Millisecond,
		},
		Uptime:    1 * time.Hour,
		EventRate: 0.5,
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		GetSummary(metrics)
	}
}

func BenchmarkFormatSummary(b *testing.B) {
	summary := &MetricsSummary{
		TotalPublished:  1000,
		TotalReceived:   1000,
		TotalProcessed:  950,
		TotalFailed:     50,
		ErrorRate:       5.0,
		Uptime:          1 * time.Hour,
		EventsPerSecond: 0.5,
		TopEventTypes: []EventTypeCount{
			{Type: EventTypeAgentConnect, Count: 500},
			{Type: EventTypeJobStart, Count: 300},
		},
		AverageProcessingTime: 100 * time.Millisecond,
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		FormatSummary(summary)
	}
}
