package metrics

import (
	"fmt"
	"testing"
	"time"
)

func TestNewCardinalityLimiter(t *testing.T) {
	limiter := NewCardinalityLimiter(nil)
	if limiter == nil {
		t.Fatal("Expected limiter to be created")
	}
	if limiter.config == nil {
		t.Error("Expected config to be set")
	}
	if limiter.config.MaxCardinality != 10000 {
		t.Errorf("Expected default MaxCardinality 10000, got %d", limiter.config.MaxCardinality)
	}
	limiter.Stop()
}

func TestCardinalityLimiter_ProcessLabels_Basic(t *testing.T) {
	config := DefaultCardinalityConfig()
	config.CleanupInterval = 0 // Disable cleanup for tests
	limiter := NewCardinalityLimiter(config)
	defer limiter.Stop()

	labels := map[string]string{
		"method":   "GET",
		"endpoint": "/api/users",
		"status":   "200",
	}

	result := limiter.ProcessLabels("http_requests", labels)

	if result["method"] != "GET" {
		t.Errorf("Expected method 'GET', got '%s'", result["method"])
	}
	if limiter.GetMetricCardinality("http_requests") != 1 {
		t.Errorf("Expected cardinality 1, got %d", limiter.GetMetricCardinality("http_requests"))
	}
}

func TestCardinalityLimiter_ProcessLabels_SameCombination(t *testing.T) {
	config := DefaultCardinalityConfig()
	config.CleanupInterval = 0
	limiter := NewCardinalityLimiter(config)
	defer limiter.Stop()

	labels := map[string]string{
		"method": "GET",
		"status": "200",
	}

	// Process the same labels multiple times
	for i := 0; i < 10; i++ {
		limiter.ProcessLabels("http_requests", labels)
	}

	// Should still only have 1 unique combination
	if limiter.GetMetricCardinality("http_requests") != 1 {
		t.Errorf("Expected cardinality 1, got %d", limiter.GetMetricCardinality("http_requests"))
	}
}

func TestCardinalityLimiter_ProcessLabels_DifferentCombinations(t *testing.T) {
	config := DefaultCardinalityConfig()
	config.CleanupInterval = 0
	limiter := NewCardinalityLimiter(config)
	defer limiter.Stop()

	// Add different combinations
	statuses := []string{"200", "201", "400", "401", "404", "500"}
	for _, status := range statuses {
		labels := map[string]string{
			"method": "GET",
			"status": status,
		}
		limiter.ProcessLabels("http_requests", labels)
	}

	if limiter.GetMetricCardinality("http_requests") != 6 {
		t.Errorf("Expected cardinality 6, got %d", limiter.GetMetricCardinality("http_requests"))
	}
}

func TestCardinalityLimiter_ProcessLabels_ExceedLimit(t *testing.T) {
	config := &CardinalityConfig{
		MaxCardinality:        5,
		MaxLabelValueLength:   128,
		HighCardinalityLabels: []string{"user_id"},
		ReplacementValue:      "__exceeded__",
	}
	limiter := NewCardinalityLimiter(config)
	defer limiter.Stop()

	// Add combinations up to the limit
	for i := 0; i < 5; i++ {
		labels := map[string]string{
			"user_id": fmt.Sprintf("user_%d", i),
			"action":  "login",
		}
		limiter.ProcessLabels("user_actions", labels)
	}

	// This should exceed the limit
	labels := map[string]string{
		"user_id": "user_999",
		"action":  "login",
	}
	result := limiter.ProcessLabels("user_actions", labels)

	// High cardinality label should be replaced
	if result["user_id"] != "__exceeded__" {
		t.Errorf("Expected user_id to be replaced with '__exceeded__', got '%s'", result["user_id"])
	}
	// Non-high-cardinality label should be preserved
	if result["action"] != "login" {
		t.Errorf("Expected action to be 'login', got '%s'", result["action"])
	}

	if !limiter.IsExceeded("user_actions") {
		t.Error("Expected metric to be marked as exceeded")
	}
}

func TestCardinalityLimiter_ProcessLabels_TruncateLongValues(t *testing.T) {
	config := &CardinalityConfig{
		MaxCardinality:      1000,
		MaxLabelValueLength: 10,
		ReplacementValue:    "__exceeded__",
	}
	limiter := NewCardinalityLimiter(config)
	defer limiter.Stop()

	labels := map[string]string{
		"long_value": "this_is_a_very_long_label_value",
	}

	result := limiter.ProcessLabels("test_metric", labels)

	if len(result["long_value"]) != 10 {
		t.Errorf("Expected truncated value length 10, got %d", len(result["long_value"]))
	}
	if result["long_value"] != "this_is_a_" {
		t.Errorf("Expected truncated value 'this_is_a_', got '%s'", result["long_value"])
	}
}

func TestCardinalityLimiter_ProcessLabels_ExcludedMetric(t *testing.T) {
	config := &CardinalityConfig{
		MaxCardinality:  5,
		ExcludedMetrics: []string{"excluded_metric"},
		ReplacementValue: "__exceeded__",
	}
	limiter := NewCardinalityLimiter(config)
	defer limiter.Stop()

	// Add many combinations to excluded metric
	for i := 0; i < 10; i++ {
		labels := map[string]string{
			"id": fmt.Sprintf("id_%d", i),
		}
		result := limiter.ProcessLabels("excluded_metric", labels)
		// Should not be modified
		if result["id"] != labels["id"] {
			t.Errorf("Expected excluded metric labels to not be modified")
		}
	}

	// Cardinality should not be tracked for excluded metrics
	if limiter.GetMetricCardinality("excluded_metric") != 0 {
		t.Errorf("Expected 0 cardinality for excluded metric, got %d", limiter.GetMetricCardinality("excluded_metric"))
	}
}

func TestCardinalityLimiter_GetStats(t *testing.T) {
	config := &CardinalityConfig{
		MaxCardinality:   100,
		CleanupInterval:  0,
		ReplacementValue: "__exceeded__",
	}
	limiter := NewCardinalityLimiter(config)
	defer limiter.Stop()

	// Add metrics
	for i := 0; i < 10; i++ {
		labels := map[string]string{"id": fmt.Sprintf("%d", i)}
		limiter.ProcessLabels("metric_a", labels)
	}
	for i := 0; i < 5; i++ {
		labels := map[string]string{"id": fmt.Sprintf("%d", i)}
		limiter.ProcessLabels("metric_b", labels)
	}

	stats := limiter.GetStats()

	if stats.TotalMetrics != 2 {
		t.Errorf("Expected 2 total metrics, got %d", stats.TotalMetrics)
	}
	if stats.TotalLabelCombinations != 15 {
		t.Errorf("Expected 15 total label combinations, got %d", stats.TotalLabelCombinations)
	}
	if stats.MetricCardinalities["metric_a"] != 10 {
		t.Errorf("Expected metric_a cardinality 10, got %d", stats.MetricCardinalities["metric_a"])
	}
}

func TestCardinalityLimiter_Reset(t *testing.T) {
	config := DefaultCardinalityConfig()
	config.CleanupInterval = 0
	limiter := NewCardinalityLimiter(config)
	defer limiter.Stop()

	// Add some data
	for i := 0; i < 10; i++ {
		labels := map[string]string{"id": fmt.Sprintf("%d", i)}
		limiter.ProcessLabels("test_metric", labels)
	}

	if limiter.GetMetricCardinality("test_metric") != 10 {
		t.Error("Expected cardinality 10 before reset")
	}

	limiter.Reset("test_metric")

	if limiter.GetMetricCardinality("test_metric") != 0 {
		t.Error("Expected cardinality 0 after reset")
	}
}

func TestCardinalityLimiter_ResetAll(t *testing.T) {
	config := DefaultCardinalityConfig()
	config.CleanupInterval = 0
	limiter := NewCardinalityLimiter(config)
	defer limiter.Stop()

	// Add data to multiple metrics
	limiter.ProcessLabels("metric_a", map[string]string{"id": "1"})
	limiter.ProcessLabels("metric_b", map[string]string{"id": "2"})

	stats := limiter.GetStats()
	if stats.TotalMetrics != 2 {
		t.Error("Expected 2 metrics before reset")
	}

	limiter.ResetAll()

	stats = limiter.GetStats()
	if stats.TotalMetrics != 0 {
		t.Error("Expected 0 metrics after reset all")
	}
}

func TestCardinalityLimiter_GenerateReport(t *testing.T) {
	config := &CardinalityConfig{
		MaxCardinality:   10,
		CleanupInterval:  0,
		ReplacementValue: "__exceeded__",
		HighCardinalityLabels: []string{"user_id"},
	}
	limiter := NewCardinalityLimiter(config)
	defer limiter.Stop()

	// Add metrics approaching limit
	for i := 0; i < 9; i++ {
		labels := map[string]string{"id": fmt.Sprintf("%d", i)}
		limiter.ProcessLabels("high_card_metric", labels)
	}

	// Add one that exceeds
	for i := 0; i < 15; i++ {
		labels := map[string]string{"user_id": fmt.Sprintf("user_%d", i)}
		limiter.ProcessLabels("exceeded_metric", labels)
	}

	report := limiter.GenerateReport()

	if report.GeneratedAt.IsZero() {
		t.Error("Expected GeneratedAt to be set")
	}
	if len(report.TopMetrics) == 0 {
		t.Error("Expected top metrics in report")
	}
	if len(report.ExceededMetrics) == 0 {
		t.Error("Expected exceeded metrics in report")
	}
	if len(report.Recommendations) == 0 {
		t.Error("Expected recommendations in report")
	}
}

func TestCardinalityLimitingCollector(t *testing.T) {
	wrapped := NewPrometheusCollector()
	// Register a counter
	if err := wrapped.RegisterMetric(MetricDefinition{
		Name:   "test_counter",
		Type:   MetricTypeCounter,
		Help:   "Test counter",
		Labels: []string{"user_id"},
	}); err != nil {
		t.Fatalf("Failed to register metric: %v", err)
	}

	config := &CardinalityConfig{
		MaxCardinality:        5,
		HighCardinalityLabels: []string{"user_id"},
		ReplacementValue:      "__exceeded__",
	}
	collector := NewCardinalityLimitingCollector(wrapped, config)
	defer collector.Stop()

	// Use within limit
	for i := 0; i < 5; i++ {
		collector.IncCounter("test_counter", map[string]string{
			"user_id": fmt.Sprintf("user_%d", i),
		})
	}

	// This should exceed and get replaced
	collector.IncCounter("test_counter", map[string]string{
		"user_id": "user_999",
	})

	if !collector.Limiter().IsExceeded("test_counter") {
		t.Error("Expected counter to be marked as exceeded")
	}
}

func TestCardinalityLimitingCollector_AllMethods(t *testing.T) {
	wrapped := NewPrometheusCollector()
	metrics := []MetricDefinition{
		{Name: "test_counter", Type: MetricTypeCounter, Help: "Test", Labels: []string{"label"}},
		{Name: "test_gauge", Type: MetricTypeGauge, Help: "Test", Labels: []string{"label"}},
		{Name: "test_histogram", Type: MetricTypeHistogram, Help: "Test", Labels: []string{"label"}},
		{Name: "test_summary", Type: MetricTypeSummary, Help: "Test", Labels: []string{"label"}},
	}
	for _, m := range metrics {
		if err := wrapped.RegisterMetric(m); err != nil {
			t.Fatalf("Failed to register metric %s: %v", m.Name, err)
		}
	}

	config := DefaultCardinalityConfig()
	config.CleanupInterval = 0
	collector := NewCardinalityLimitingCollector(wrapped, config)
	defer collector.Stop()

	labels := map[string]string{"label": "value"}

	// Test all methods
	collector.IncCounter("test_counter", labels)
	collector.AddCounter("test_counter", 5.0, labels)
	collector.SetGauge("test_gauge", 10.0, labels)
	collector.IncGauge("test_gauge", labels)
	collector.DecGauge("test_gauge", labels)
	collector.ObserveHistogram("test_histogram", 0.5, labels)
	collector.ObserveSummary("test_summary", 0.5, labels)
	collector.RecordDuration("test_histogram", 100*time.Millisecond, labels)

	// Verify cardinality tracking
	limiter := collector.Limiter()
	for _, m := range []string{"test_counter", "test_gauge", "test_histogram", "test_summary"} {
		if limiter.GetMetricCardinality(m) != 1 {
			t.Errorf("Expected cardinality 1 for %s, got %d", m, limiter.GetMetricCardinality(m))
		}
	}
}

func TestCardinalityLimiter_HashLabelsConsistency(t *testing.T) {
	config := DefaultCardinalityConfig()
	config.CleanupInterval = 0
	limiter := NewCardinalityLimiter(config)
	defer limiter.Stop()

	// Same labels in different order should produce same hash
	labels1 := map[string]string{"a": "1", "b": "2", "c": "3"}
	labels2 := map[string]string{"c": "3", "a": "1", "b": "2"}
	labels3 := map[string]string{"b": "2", "c": "3", "a": "1"}

	hash1 := limiter.hashLabels(labels1)
	hash2 := limiter.hashLabels(labels2)
	hash3 := limiter.hashLabels(labels3)

	if hash1 != hash2 || hash2 != hash3 {
		t.Error("Expected same hash for same labels in different order")
	}
}

func TestCardinalityLimiter_HighCardinalityMetrics(t *testing.T) {
	config := &CardinalityConfig{
		MaxCardinality:  100,
		CleanupInterval: 0,
		ReplacementValue: "__exceeded__",
	}
	limiter := NewCardinalityLimiter(config)
	defer limiter.Stop()

	// Add metrics at 85% of limit (should be flagged as high cardinality)
	for i := 0; i < 85; i++ {
		labels := map[string]string{"id": fmt.Sprintf("%d", i)}
		limiter.ProcessLabels("almost_full", labels)
	}

	stats := limiter.GetStats()

	found := false
	for _, m := range stats.HighCardinalityMetrics {
		if m == "almost_full" {
			found = true
			break
		}
	}
	if !found {
		t.Error("Expected 'almost_full' to be flagged as high cardinality")
	}
}

func TestDefaultCardinalityConfig(t *testing.T) {
	config := DefaultCardinalityConfig()

	if config.MaxCardinality != 10000 {
		t.Errorf("Expected MaxCardinality 10000, got %d", config.MaxCardinality)
	}
	if config.MaxLabelValueLength != 128 {
		t.Errorf("Expected MaxLabelValueLength 128, got %d", config.MaxLabelValueLength)
	}
	if config.ReplacementValue != "__cardinality_exceeded__" {
		t.Errorf("Expected ReplacementValue '__cardinality_exceeded__', got '%s'", config.ReplacementValue)
	}
	if len(config.HighCardinalityLabels) != 4 {
		t.Errorf("Expected 4 high cardinality labels, got %d", len(config.HighCardinalityLabels))
	}
}
