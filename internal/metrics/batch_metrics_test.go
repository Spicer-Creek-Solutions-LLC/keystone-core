package metrics

import (
	"errors"
	"testing"
	"time"
)

func TestBatchMetricsAggregator_NewAggregator(t *testing.T) {
	agg := NewBatchMetricsAggregator()

	if agg == nil {
		t.Fatal("Expected non-nil aggregator")
	}

	if agg.failuresByType == nil {
		t.Error("Expected initialized failuresByType map")
	}

	if agg.executionTimes == nil {
		t.Error("Expected initialized executionTimes slice")
	}
}

func TestBatchMetricsAggregator_RecordAgentResult(t *testing.T) {
	agg := NewBatchMetricsAggregator()

	// Record some successful results
	agg.RecordAgentResult(true, 100*time.Millisecond, "")
	agg.RecordAgentResult(true, 150*time.Millisecond, "")
	agg.RecordAgentResult(true, 200*time.Millisecond, "")

	// Record some failed results
	agg.RecordAgentResult(false, 50*time.Millisecond, ErrorTypeTimeout)
	agg.RecordAgentResult(false, 75*time.Millisecond, ErrorTypeNetwork)

	metrics := agg.GetAggregateMetrics()

	if metrics.TotalExecutions != 5 {
		t.Errorf("Expected 5 total executions, got %d", metrics.TotalExecutions)
	}

	if metrics.SuccessfulAgents != 3 {
		t.Errorf("Expected 3 successful agents, got %d", metrics.SuccessfulAgents)
	}

	if metrics.FailedAgents != 2 {
		t.Errorf("Expected 2 failed agents, got %d", metrics.FailedAgents)
	}

	// Check success rate
	expectedRate := 60.0 // 3/5 = 60%
	if metrics.OverallSuccessRate != expectedRate {
		t.Errorf("Expected success rate %.1f%%, got %.1f%%", expectedRate, metrics.OverallSuccessRate)
	}
}

func TestBatchMetricsAggregator_GetSuccessRate(t *testing.T) {
	tests := []struct {
		name     string
		success  int
		failed   int
		expected float64
	}{
		{"all success", 10, 0, 100.0},
		{"all failed", 0, 10, 0.0},
		{"half and half", 5, 5, 50.0},
		{"no executions", 0, 0, 100.0},
		{"70% success", 7, 3, 70.0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			agg := NewBatchMetricsAggregator()

			for i := 0; i < tt.success; i++ {
				agg.RecordAgentResult(true, 100*time.Millisecond, "")
			}
			for i := 0; i < tt.failed; i++ {
				agg.RecordAgentResult(false, 100*time.Millisecond, ErrorTypeCommand)
			}

			rate := agg.GetSuccessRate()
			if rate != tt.expected {
				t.Errorf("Expected success rate %.1f%%, got %.1f%%", tt.expected, rate)
			}
		})
	}
}

func TestBatchMetricsAggregator_FailuresByType(t *testing.T) {
	agg := NewBatchMetricsAggregator()

	// Record failures with different error types
	agg.RecordAgentResult(false, 100*time.Millisecond, ErrorTypeTimeout)
	agg.RecordAgentResult(false, 100*time.Millisecond, ErrorTypeTimeout)
	agg.RecordAgentResult(false, 100*time.Millisecond, ErrorTypeNetwork)
	agg.RecordAgentResult(false, 100*time.Millisecond, ErrorTypeAuth)

	failures := agg.GetFailuresByType()

	if failures[ErrorTypeTimeout] != 2 {
		t.Errorf("Expected 2 timeout failures, got %d", failures[ErrorTypeTimeout])
	}

	if failures[ErrorTypeNetwork] != 1 {
		t.Errorf("Expected 1 network failure, got %d", failures[ErrorTypeNetwork])
	}

	if failures[ErrorTypeAuth] != 1 {
		t.Errorf("Expected 1 auth failure, got %d", failures[ErrorTypeAuth])
	}
}

func TestBatchMetricsAggregator_BatchTracking(t *testing.T) {
	agg := NewBatchMetricsAggregator()

	// Start a batch
	agg.StartBatch("batch-001", 5)

	// Record results
	agg.RecordAgentResult(true, 100*time.Millisecond, "")
	agg.RecordAgentResult(true, 150*time.Millisecond, "")
	agg.RecordAgentResult(false, 200*time.Millisecond, ErrorTypeCommand)
	agg.RecordAgentSkipped()
	agg.RecordAgentResult(true, 120*time.Millisecond, "")

	// Complete the batch
	batch := agg.CompleteBatch(true)

	if batch == nil {
		t.Fatal("Expected non-nil batch")
	}

	if batch.BatchJobID != "batch-001" {
		t.Errorf("Expected batch ID 'batch-001', got '%s'", batch.BatchJobID)
	}

	if batch.SuccessCount != 3 {
		t.Errorf("Expected 3 successes, got %d", batch.SuccessCount)
	}

	if batch.FailedCount != 1 {
		t.Errorf("Expected 1 failure, got %d", batch.FailedCount)
	}

	if batch.SkippedCount != 1 {
		t.Errorf("Expected 1 skipped, got %d", batch.SkippedCount)
	}

	// Check aggregate batch job counts
	metrics := agg.GetAggregateMetrics()
	if metrics.TotalBatchJobs != 1 {
		t.Errorf("Expected 1 total batch job, got %d", metrics.TotalBatchJobs)
	}

	if metrics.CompletedBatchJobs != 1 {
		t.Errorf("Expected 1 completed batch job, got %d", metrics.CompletedBatchJobs)
	}
}

func TestBatchMetricsAggregator_Percentiles(t *testing.T) {
	agg := NewBatchMetricsAggregator()

	// Record 100 execution times from 1ms to 100ms
	for i := 1; i <= 100; i++ {
		agg.RecordAgentResult(true, time.Duration(i)*time.Millisecond, "")
	}

	metrics := agg.GetAggregateMetrics()

	// P50 should be around 50ms
	if metrics.P50DurationMs < 45 || metrics.P50DurationMs > 55 {
		t.Errorf("Expected P50 around 50ms, got %.1fms", metrics.P50DurationMs)
	}

	// P90 should be around 90ms
	if metrics.P90DurationMs < 85 || metrics.P90DurationMs > 95 {
		t.Errorf("Expected P90 around 90ms, got %.1fms", metrics.P90DurationMs)
	}

	// P95 should be around 95ms
	if metrics.P95DurationMs < 90 || metrics.P95DurationMs > 100 {
		t.Errorf("Expected P95 around 95ms, got %.1fms", metrics.P95DurationMs)
	}

	// P99 should be around 99ms
	if metrics.P99DurationMs < 95 || metrics.P99DurationMs > 105 {
		t.Errorf("Expected P99 around 99ms, got %.1fms", metrics.P99DurationMs)
	}

	// Min should be 1ms
	if metrics.MinDurationMs != 1 {
		t.Errorf("Expected min 1ms, got %.1fms", metrics.MinDurationMs)
	}

	// Max should be 100ms
	if metrics.MaxDurationMs != 100 {
		t.Errorf("Expected max 100ms, got %.1fms", metrics.MaxDurationMs)
	}
}

func TestBatchMetricsAggregator_Reset(t *testing.T) {
	agg := NewBatchMetricsAggregator()

	// Record some data
	agg.RecordAgentResult(true, 100*time.Millisecond, "")
	agg.RecordAgentResult(false, 100*time.Millisecond, ErrorTypeTimeout)

	// Reset
	agg.Reset()

	metrics := agg.GetAggregateMetrics()

	if metrics.TotalExecutions != 0 {
		t.Errorf("Expected 0 executions after reset, got %d", metrics.TotalExecutions)
	}

	if metrics.SuccessfulAgents != 0 {
		t.Errorf("Expected 0 successful agents after reset, got %d", metrics.SuccessfulAgents)
	}

	if len(metrics.FailuresByType) != 0 {
		t.Errorf("Expected empty failures map after reset, got %d entries", len(metrics.FailuresByType))
	}
}

func TestClassifyError(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		expected string
	}{
		{"nil error", nil, ""},
		{"timeout error", errors.New("context deadline exceeded"), ErrorTypeTimeout},
		{"network error", errors.New("connection refused"), ErrorTypeNetwork},
		{"auth error", errors.New("unauthorized access"), ErrorTypeAuth},
		{"permission error", errors.New("permission denied"), ErrorTypePermission},
		{"not found error", errors.New("file not found"), ErrorTypeNotFound},
		{"command error", errors.New("command failed with exit code 1"), ErrorTypeCommand},
		{"offline error", errors.New("agent is offline"), ErrorTypeAgentOffline},
		{"unknown error", errors.New("some random error"), ErrorTypeInternal},
		{"timed out variation", errors.New("operation timed out"), ErrorTypeTimeout},
		{"network unreachable", errors.New("host unreachable"), ErrorTypeNetwork},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ClassifyError(tt.err)
			if result != tt.expected {
				t.Errorf("Expected error type '%s', got '%s'", tt.expected, result)
			}
		})
	}
}

func TestBatchMetricsAggregator_ConcurrentAccess(t *testing.T) {
	agg := NewBatchMetricsAggregator()

	// Concurrent writes
	done := make(chan bool)
	for i := 0; i < 10; i++ {
		go func() {
			for j := 0; j < 100; j++ {
				agg.RecordAgentResult(j%2 == 0, 100*time.Millisecond, ErrorTypeCommand)
			}
			done <- true
		}()
	}

	// Wait for all goroutines
	for i := 0; i < 10; i++ {
		<-done
	}

	metrics := agg.GetAggregateMetrics()

	if metrics.TotalExecutions != 1000 {
		t.Errorf("Expected 1000 total executions, got %d", metrics.TotalExecutions)
	}

	if metrics.SuccessfulAgents+metrics.FailedAgents != 1000 {
		t.Errorf("Expected 1000 total agents, got %d",
			metrics.SuccessfulAgents+metrics.FailedAgents)
	}
}

func TestBatchMetricsAggregator_GetFailureRatio(t *testing.T) {
	agg := NewBatchMetricsAggregator()

	// 3 successes, 2 failures
	agg.RecordAgentResult(true, 100*time.Millisecond, "")
	agg.RecordAgentResult(true, 100*time.Millisecond, "")
	agg.RecordAgentResult(true, 100*time.Millisecond, "")
	agg.RecordAgentResult(false, 100*time.Millisecond, ErrorTypeTimeout)
	agg.RecordAgentResult(false, 100*time.Millisecond, ErrorTypeNetwork)

	ratio := agg.GetFailureRatio()
	expected := 40.0 // 2/5 = 40%

	if ratio != expected {
		t.Errorf("Expected failure ratio %.1f%%, got %.1f%%", expected, ratio)
	}
}

func BenchmarkBatchMetricsAggregator_RecordAgentResult(b *testing.B) {
	agg := NewBatchMetricsAggregator()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		agg.RecordAgentResult(i%2 == 0, 100*time.Millisecond, ErrorTypeCommand)
	}
}

func BenchmarkBatchMetricsAggregator_GetAggregateMetrics(b *testing.B) {
	agg := NewBatchMetricsAggregator()

	// Pre-populate with data
	for i := 0; i < 10000; i++ {
		agg.RecordAgentResult(i%2 == 0, time.Duration(i)*time.Microsecond, ErrorTypeCommand)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		agg.GetAggregateMetrics()
	}
}
