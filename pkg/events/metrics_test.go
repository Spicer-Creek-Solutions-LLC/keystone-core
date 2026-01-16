package events

import (
	"sync"
	"testing"
	"time"

	"github.com/shawnbutts/keystone-core/pkg/testing/helpers"
)

func TestMetricsCollector_RecordEventPublished(t *testing.T) {
	collector := NewMetricsCollector()

	collector.RecordEventPublished(EventTypeAgentConnect, SeverityInfo)
	collector.RecordEventPublished(EventTypeAgentConnect, SeverityInfo)
	collector.RecordEventPublished(EventTypeJobStart, SeverityInfo)

	metrics := collector.GetMetrics()

	if metrics.EventsPublished[EventTypeAgentConnect] != 2 {
		t.Errorf("Expected 2 AgentConnect events, got %d", metrics.EventsPublished[EventTypeAgentConnect])
	}

	if metrics.EventsPublished[EventTypeJobStart] != 1 {
		t.Errorf("Expected 1 JobStart event, got %d", metrics.EventsPublished[EventTypeJobStart])
	}

	if metrics.EventsBySeverity[SeverityInfo] != 3 {
		t.Errorf("Expected 3 Info severity events, got %d", metrics.EventsBySeverity[SeverityInfo])
	}
}

func TestMetricsCollector_RecordEventReceived(t *testing.T) {
	collector := NewMetricsCollector()

	collector.RecordEventReceived(EventTypeAgentConnect, SeverityInfo)
	collector.RecordEventReceived(EventTypeAgentDisconnect, SeverityWarning)

	metrics := collector.GetMetrics()

	if metrics.EventsReceived[EventTypeAgentConnect] != 1 {
		t.Errorf("Expected 1 AgentConnect event, got %d", metrics.EventsReceived[EventTypeAgentConnect])
	}

	if metrics.EventsReceived[EventTypeAgentDisconnect] != 1 {
		t.Errorf("Expected 1 AgentDisconnect event, got %d", metrics.EventsReceived[EventTypeAgentDisconnect])
	}
}

func TestMetricsCollector_RecordEventProcessed(t *testing.T) {
	collector := NewMetricsCollector()

	collector.RecordEventProcessed(EventTypeAgentConnect, 100*time.Millisecond, true)
	collector.RecordEventProcessed(EventTypeAgentConnect, 200*time.Millisecond, true)
	collector.RecordEventProcessed(EventTypeJobStart, 50*time.Millisecond, false)

	metrics := collector.GetMetrics()

	if metrics.EventsProcessed[EventTypeAgentConnect] != 2 {
		t.Errorf("Expected 2 processed events, got %d", metrics.EventsProcessed[EventTypeAgentConnect])
	}

	if metrics.EventsFailed[EventTypeJobStart] != 1 {
		t.Errorf("Expected 1 failed event, got %d", metrics.EventsFailed[EventTypeJobStart])
	}

	if metrics.ProcessingDuration.Count != 3 {
		t.Errorf("Expected 3 duration records, got %d", metrics.ProcessingDuration.Count)
	}

	if metrics.ProcessingDuration.Avg == 0 {
		t.Error("Expected non-zero average processing duration")
	}
}

func TestMetricsCollector_RecordPublisherError(t *testing.T) {
	collector := NewMetricsCollector()

	collector.RecordPublisherError(EventTypeAgentConnect)
	collector.RecordPublisherError(EventTypeAgentConnect)
	collector.RecordPublisherError(EventTypeJobStart)

	metrics := collector.GetMetrics()

	if metrics.PublisherErrors != 3 {
		t.Errorf("Expected 3 publisher errors, got %d", metrics.PublisherErrors)
	}
}

func TestMetricsCollector_RecordSubscriberError(t *testing.T) {
	collector := NewMetricsCollector()

	collector.RecordSubscriberError("agent.>")
	collector.RecordSubscriberError("job.>")

	metrics := collector.GetMetrics()

	if metrics.SubscriberErrors != 2 {
		t.Errorf("Expected 2 subscriber errors, got %d", metrics.SubscriberErrors)
	}
}

func TestMetricsCollector_RecordReactorExecution(t *testing.T) {
	collector := NewMetricsCollector()

	collector.RecordReactorExecution("restart-service", 50*time.Millisecond, true)
	collector.RecordReactorExecution("restart-service", 100*time.Millisecond, true)
	collector.RecordReactorExecution("send-alert", 25*time.Millisecond, false)

	metrics := collector.GetMetrics()

	if metrics.ReactorExecutions["restart-service"] != 2 {
		t.Errorf("Expected 2 reactor executions, got %d", metrics.ReactorExecutions["restart-service"])
	}

	if metrics.ReactorFailures["send-alert"] != 1 {
		t.Errorf("Expected 1 reactor failure, got %d", metrics.ReactorFailures["send-alert"])
	}

	if metrics.ReactorDurations["restart-service"] == nil {
		t.Error("Expected reactor duration stats")
	}

	if metrics.ReactorDurations["restart-service"].Count != 2 {
		t.Errorf("Expected 2 duration records, got %d", metrics.ReactorDurations["restart-service"].Count)
	}
}

func TestMetricsCollector_RecordActionExecution(t *testing.T) {
	collector := NewMetricsCollector()

	collector.RecordActionExecution("restart-nginx", "service", 150*time.Millisecond, true)
	collector.RecordActionExecution("restart-nginx", "service", 200*time.Millisecond, true)
	collector.RecordActionExecution("send-webhook", "http", 50*time.Millisecond, false)

	metrics := collector.GetMetrics()

	if metrics.ActionExecutions["service:restart-nginx"] != 2 {
		t.Errorf("Expected 2 action executions, got %d", metrics.ActionExecutions["service:restart-nginx"])
	}

	if metrics.ActionFailures["http:send-webhook"] != 1 {
		t.Errorf("Expected 1 action failure, got %d", metrics.ActionFailures["http:send-webhook"])
	}

	if metrics.ActionDurations["service:restart-nginx"] == nil {
		t.Error("Expected action duration stats")
	}
}

func TestMetricsCollector_RecordStorageOperation(t *testing.T) {
	collector := NewMetricsCollector()

	collector.RecordStorageOperation("Store", 10*time.Millisecond, true)
	collector.RecordStorageOperation("Store", 15*time.Millisecond, true)
	collector.RecordStorageOperation("Query", 50*time.Millisecond, false)

	metrics := collector.GetMetrics()

	if metrics.StorageOperations["Store"] != 2 {
		t.Errorf("Expected 2 storage operations, got %d", metrics.StorageOperations["Store"])
	}

	if metrics.StorageFailures["Query"] != 1 {
		t.Errorf("Expected 1 storage failure, got %d", metrics.StorageFailures["Query"])
	}

	if metrics.StorageDurations["Store"] == nil {
		t.Error("Expected storage duration stats")
	}
}

func TestMetricsCollector_GetMetrics_Uptime(t *testing.T) {
	collector := NewMetricsCollector()

	if err := helpers.WaitForTimeout(2*time.Second, 10*time.Millisecond, func() (bool, error) {
		metrics := collector.GetMetrics()
		return metrics.Uptime >= 100*time.Millisecond, nil
	}); err != nil {
		t.Fatalf("Expected uptime >= 100ms: %v", err)
	}

	metrics := collector.GetMetrics()

	if metrics.Uptime < 100*time.Millisecond {
		t.Errorf("Expected uptime >= 100ms, got %v", metrics.Uptime)
	}

	if metrics.StartTime.IsZero() {
		t.Error("Expected non-zero start time")
	}
}

func TestMetricsCollector_GetMetrics_EventRate(t *testing.T) {
	collector := NewMetricsCollector()

	// Record some events
	for i := 0; i < 10; i++ {
		collector.RecordEventPublished(EventTypeAgentConnect, SeverityInfo)
	}

	if err := helpers.WaitForTimeout(2*time.Second, 10*time.Millisecond, func() (bool, error) {
		metrics := collector.GetMetrics()
		return metrics.EventRate > 0, nil
	}); err != nil {
		t.Fatalf("Expected positive event rate: %v", err)
	}
}

func TestMetricsCollector_Concurrency(t *testing.T) {
	collector := NewMetricsCollector()

	var wg sync.WaitGroup
	iterations := 100

	// Concurrent event publishing
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < iterations; j++ {
				collector.RecordEventPublished(EventTypeAgentConnect, SeverityInfo)
			}
		}()
	}

	// Concurrent event processing
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < iterations; j++ {
				collector.RecordEventProcessed(EventTypeAgentConnect, 10*time.Millisecond, true)
			}
		}()
	}

	wg.Wait()

	metrics := collector.GetMetrics()

	expectedPublished := 10 * iterations
	if metrics.EventsPublished[EventTypeAgentConnect] != int64(expectedPublished) {
		t.Errorf("Expected %d published events, got %d", expectedPublished, metrics.EventsPublished[EventTypeAgentConnect])
	}

	expectedProcessed := 10 * iterations
	if metrics.EventsProcessed[EventTypeAgentConnect] != int64(expectedProcessed) {
		t.Errorf("Expected %d processed events, got %d", expectedProcessed, metrics.EventsProcessed[EventTypeAgentConnect])
	}
}

func TestDurationStats_Record(t *testing.T) {
	stats := NewDurationStats()

	stats.Record(100 * time.Millisecond)
	stats.Record(200 * time.Millisecond)
	stats.Record(150 * time.Millisecond)

	if stats.Count != 3 {
		t.Errorf("Expected count 3, got %d", stats.Count)
	}

	if stats.Min != 100*time.Millisecond {
		t.Errorf("Expected min 100ms, got %v", stats.Min)
	}

	if stats.Max != 200*time.Millisecond {
		t.Errorf("Expected max 200ms, got %v", stats.Max)
	}

	expectedAvg := 150 * time.Millisecond
	if stats.Avg != expectedAvg {
		t.Errorf("Expected avg %v, got %v", expectedAvg, stats.Avg)
	}

	expectedTotal := 450 * time.Millisecond
	if stats.Total != expectedTotal {
		t.Errorf("Expected total %v, got %v", expectedTotal, stats.Total)
	}
}

func TestDurationStats_Percentiles(t *testing.T) {
	stats := NewDurationStats()

	// Record 101 values from 1ms to 101ms (need > 100 for P99)
	for i := 1; i <= 101; i++ {
		stats.Record(time.Duration(i) * time.Millisecond)
	}

	// P50 should be around 50ms
	if stats.P50 < 40*time.Millisecond || stats.P50 > 60*time.Millisecond {
		t.Errorf("Expected P50 around 50ms, got %v", stats.P50)
	}

	// P95 should be around 95ms
	if stats.P95 < 90*time.Millisecond || stats.P95 > 100*time.Millisecond {
		t.Errorf("Expected P95 around 95ms, got %v", stats.P95)
	}

	// P99 should be around 99ms
	if stats.P99 < 95*time.Millisecond || stats.P99 > 105*time.Millisecond {
		t.Errorf("Expected P99 around 99ms, got %v", stats.P99)
	}
}

func TestDurationStats_RollingWindow(t *testing.T) {
	stats := NewDurationStats()

	// Record more than 1000 values
	for i := 1; i <= 1500; i++ {
		stats.Record(time.Duration(i) * time.Millisecond)
	}

	// Should keep only last 1000
	if len(stats.recent) != 1000 {
		t.Errorf("Expected 1000 recent values, got %d", len(stats.recent))
	}

	// All stats should still be correct
	if stats.Count != 1500 {
		t.Errorf("Expected count 1500, got %d", stats.Count)
	}
}

func TestHealthMonitor_RegisterCheck(t *testing.T) {
	monitor := NewHealthMonitor()

	check := &mockHealthChecker{
		name:   "test",
		status: HealthStatusHealthy,
	}

	monitor.RegisterCheck("test", check)

	results := monitor.CheckAll()

	if len(results) != 1 {
		t.Errorf("Expected 1 health check result, got %d", len(results))
	}

	if results["test"].Status != HealthStatusHealthy {
		t.Errorf("Expected healthy status, got %s", results["test"].Status)
	}
}

func TestHealthMonitor_UnregisterCheck(t *testing.T) {
	monitor := NewHealthMonitor()

	check := &mockHealthChecker{
		name:   "test",
		status: HealthStatusHealthy,
	}

	monitor.RegisterCheck("test", check)
	monitor.UnregisterCheck("test")

	results := monitor.CheckAll()

	if len(results) != 0 {
		t.Errorf("Expected 0 health check results, got %d", len(results))
	}
}

func TestHealthMonitor_GetOverallStatus(t *testing.T) {
	monitor := NewHealthMonitor()

	// All healthy
	monitor.RegisterCheck("check1", &mockHealthChecker{status: HealthStatusHealthy})
	monitor.RegisterCheck("check2", &mockHealthChecker{status: HealthStatusHealthy})

	if monitor.GetOverallStatus() != HealthStatusHealthy {
		t.Error("Expected overall healthy status")
	}

	// One degraded
	monitor.RegisterCheck("check3", &mockHealthChecker{status: HealthStatusDegraded})

	if monitor.GetOverallStatus() != HealthStatusDegraded {
		t.Error("Expected overall degraded status")
	}

	// One unhealthy
	monitor.RegisterCheck("check4", &mockHealthChecker{status: HealthStatusUnhealthy})

	if monitor.GetOverallStatus() != HealthStatusUnhealthy {
		t.Error("Expected overall unhealthy status")
	}
}

func TestEventSystemHealthCheck_Healthy(t *testing.T) {
	metrics := &Metrics{
		LastEvent: time.Now(),
	}

	check := NewEventSystemHealthCheck(metrics, 5*time.Minute, 100)

	result := check.Check()

	if result.Status != HealthStatusHealthy {
		t.Errorf("Expected healthy status, got %s: %s", result.Status, result.Message)
	}

	if result.Name != "event_system" {
		t.Errorf("Expected name 'event_system', got %s", result.Name)
	}
}

func TestEventSystemHealthCheck_Degraded_NoRecentEvents(t *testing.T) {
	metrics := &Metrics{
		LastEvent: time.Now().Add(-10 * time.Minute),
	}

	check := NewEventSystemHealthCheck(metrics, 5*time.Minute, 100)

	result := check.Check()

	if result.Status != HealthStatusDegraded {
		t.Errorf("Expected degraded status, got %s", result.Status)
	}

	if result.Message != "No recent events" {
		t.Errorf("Expected 'No recent events' message, got %s", result.Message)
	}
}

func TestEventSystemHealthCheck_Degraded_HighErrorRate(t *testing.T) {
	metrics := &Metrics{
		LastEvent:        time.Now(),
		PublisherErrors:  150,
		SubscriberErrors: 50,
	}

	check := NewEventSystemHealthCheck(metrics, 5*time.Minute, 100)

	result := check.Check()

	if result.Status != HealthStatusDegraded {
		t.Errorf("Expected degraded status, got %s", result.Status)
	}

	if result.Message != "High error rate" {
		t.Errorf("Expected 'High error rate' message, got %s", result.Message)
	}
}

func TestStorageHealthCheck_Healthy(t *testing.T) {
	// Create in-memory store with default config to ensure proper initialization
	config := DefaultEventStoreConfig()
	config.Path = ":memory:"
	config.AutoRetention = false

	store, err := NewSQLiteEventStore(config)
	if err != nil {
		t.Fatalf("Failed to create store: %v", err)
	}
	defer store.Close()

	check := NewStorageHealthCheck(store)

	result := check.Check()

	if result.Status != HealthStatusHealthy {
		t.Errorf("Expected healthy status, got %s: %s", result.Status, result.Message)
	}

	if result.Name != "event_storage" {
		t.Errorf("Expected name 'event_storage', got %s", result.Name)
	}

	if result.Duration <= 0 {
		t.Error("Expected positive duration")
	}
}

func TestStorageHealthCheck_Unhealthy_ClosedStore(t *testing.T) {
	// Create and immediately close store
	config := DefaultEventStoreConfig()
	config.Path = ":memory:"
	config.AutoRetention = false

	store, err := NewSQLiteEventStore(config)
	if err != nil {
		t.Fatalf("Failed to create store: %v", err)
	}
	store.Close()

	check := NewStorageHealthCheck(store)

	result := check.Check()

	if result.Status != HealthStatusUnhealthy {
		t.Errorf("Expected unhealthy status, got %s", result.Status)
	}

	if result.Message == "" {
		t.Error("Expected error message")
	}
}

func TestGetSummary(t *testing.T) {
	metrics := &Metrics{
		EventsPublished: map[EventType]int64{
			EventTypeAgentConnect:    100,
			EventTypeJobStart:        50,
			EventTypeStateApplyStart: 25,
		},
		EventsReceived: map[EventType]int64{
			EventTypeAgentConnect: 100,
			EventTypeJobStart:     50,
		},
		EventsProcessed: map[EventType]int64{
			EventTypeAgentConnect: 95,
			EventTypeJobStart:     45,
		},
		EventsFailed: map[EventType]int64{
			EventTypeAgentConnect: 5,
			EventTypeJobStart:     5,
		},
		ProcessingDuration: &DurationStats{
			Count: 140,
			Total: 14 * time.Second,
			Avg:   100 * time.Millisecond,
		},
		Uptime:    1 * time.Hour,
		EventRate: 2.5,
	}

	summary := GetSummary(metrics)

	if summary.TotalPublished != 175 {
		t.Errorf("Expected 175 total published, got %d", summary.TotalPublished)
	}

	if summary.TotalReceived != 150 {
		t.Errorf("Expected 150 total received, got %d", summary.TotalReceived)
	}

	if summary.TotalProcessed != 140 {
		t.Errorf("Expected 140 total processed, got %d", summary.TotalProcessed)
	}

	if summary.TotalFailed != 10 {
		t.Errorf("Expected 10 total failed, got %d", summary.TotalFailed)
	}

	// Error rate = 10 / (140 + 10) * 100 = 6.67%
	expectedErrorRate := 6.666666666666667
	if summary.ErrorRate < expectedErrorRate-0.01 || summary.ErrorRate > expectedErrorRate+0.01 {
		t.Errorf("Expected error rate %.2f%%, got %.2f%%", expectedErrorRate, summary.ErrorRate)
	}

	if len(summary.TopEventTypes) != 3 {
		t.Errorf("Expected 3 top event types, got %d", len(summary.TopEventTypes))
	}

	// Should be sorted by count descending
	if summary.TopEventTypes[0].Type != EventTypeAgentConnect {
		t.Errorf("Expected first top event to be AgentConnect, got %s", summary.TopEventTypes[0].Type)
	}

	if summary.AverageProcessingTime != 100*time.Millisecond {
		t.Errorf("Expected avg processing time 100ms, got %v", summary.AverageProcessingTime)
	}
}

func TestFormatSummary(t *testing.T) {
	summary := &MetricsSummary{
		TotalPublished:  100,
		TotalReceived:   95,
		TotalProcessed:  90,
		TotalFailed:     5,
		ErrorRate:       5.26,
		Uptime:          1 * time.Hour,
		EventsPerSecond: 0.025,
		TopEventTypes: []EventTypeCount{
			{Type: EventTypeAgentConnect, Count: 50},
			{Type: EventTypeJobStart, Count: 25},
		},
		AverageProcessingTime: 100 * time.Millisecond,
	}

	output := FormatSummary(summary)

	if output == "" {
		t.Error("Expected non-empty formatted output")
	}

	// Check for key elements
	if !contains(output, "Keystone Core Event System Metrics") {
		t.Error("Expected header in output")
	}

	if !contains(output, "Uptime") {
		t.Error("Expected uptime in output")
	}

	if !contains(output, "Event Rate") {
		t.Error("Expected event rate in output")
	}

	if !contains(output, "Published:  100") {
		t.Error("Expected published count in output")
	}

	if !contains(output, "Error Rate: 5.26%") {
		t.Error("Expected error rate in output")
	}

	if !contains(output, "Top Event Types") {
		t.Error("Expected top event types section in output")
	}
}

// Mock health checker for testing
type mockHealthChecker struct {
	name   string
	status HealthStatus
}

func (m *mockHealthChecker) Check() *HealthCheck {
	return &HealthCheck{
		Name:        m.name,
		Status:      m.status,
		LastChecked: time.Now(),
		Duration:    1 * time.Millisecond,
	}
}

// Helper function
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > len(substr) && containsHelper(s, substr))
}

func containsHelper(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

func BenchmarkMetricsCollector_RecordEventPublished(b *testing.B) {
	collector := NewMetricsCollector()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		collector.RecordEventPublished(EventTypeAgentConnect, SeverityInfo)
	}
}

func BenchmarkMetricsCollector_RecordEventProcessed(b *testing.B) {
	collector := NewMetricsCollector()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		collector.RecordEventProcessed(EventTypeAgentConnect, 10*time.Millisecond, true)
	}
}

func BenchmarkDurationStats_Record(b *testing.B) {
	stats := NewDurationStats()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		stats.Record(10 * time.Millisecond)
	}
}

func BenchmarkHealthMonitor_CheckAll(b *testing.B) {
	monitor := NewHealthMonitor()

	for i := 0; i < 10; i++ {
		monitor.RegisterCheck("check", &mockHealthChecker{status: HealthStatusHealthy})
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		monitor.CheckAll()
	}
}
