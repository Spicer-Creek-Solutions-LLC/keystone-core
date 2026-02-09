package metrics

import (
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/shawnbutts/keystone-core/internal/runbook"
)

func TestNewCollector(t *testing.T) {
	c := NewCollector()

	if c == nil {
		t.Fatal("expected non-nil collector")
	}

	if c.runbookMetrics == nil {
		t.Error("expected runbookMetrics map to be initialized")
	}

	if c.stepTypeMetrics == nil {
		t.Error("expected stepTypeMetrics map to be initialized")
	}

	if len(c.latencyBuckets) == 0 {
		t.Error("expected latency buckets to be initialized")
	}
}

func TestRecordExecutionLifecycle(t *testing.T) {
	c := NewCollector()

	// Record start
	c.RecordExecutionStart("test-runbook")

	summary := c.GetSummary()
	if summary.TotalExecutions != 1 {
		t.Errorf("expected TotalExecutions=1, got %d", summary.TotalExecutions)
	}
	if summary.ActiveExecutions != 1 {
		t.Errorf("expected ActiveExecutions=1, got %d", summary.ActiveExecutions)
	}

	// Record completion
	c.RecordExecutionComplete("test-runbook", 100*time.Millisecond)

	summary = c.GetSummary()
	if summary.ActiveExecutions != 0 {
		t.Errorf("expected ActiveExecutions=0, got %d", summary.ActiveExecutions)
	}
	if summary.CompletedCount != 1 {
		t.Errorf("expected CompletedCount=1, got %d", summary.CompletedCount)
	}
}

func TestRecordExecutionFailed(t *testing.T) {
	c := NewCollector()

	c.RecordExecutionStart("test-runbook")
	c.RecordExecutionFailed("test-runbook", 50*time.Millisecond)

	summary := c.GetSummary()
	if summary.FailedCount != 1 {
		t.Errorf("expected FailedCount=1, got %d", summary.FailedCount)
	}
	if summary.ActiveExecutions != 0 {
		t.Errorf("expected ActiveExecutions=0, got %d", summary.ActiveExecutions)
	}
}

func TestRecordExecutionCancelled(t *testing.T) {
	c := NewCollector()

	c.RecordExecutionStart("test-runbook")
	c.RecordExecutionCancelled("test-runbook")

	summary := c.GetSummary()
	if summary.CancelledCount != 1 {
		t.Errorf("expected CancelledCount=1, got %d", summary.CancelledCount)
	}
	if summary.ActiveExecutions != 0 {
		t.Errorf("expected ActiveExecutions=0, got %d", summary.ActiveExecutions)
	}
}

func TestRecordStepLifecycle(t *testing.T) {
	c := NewCollector()

	// Record step start
	c.RecordStepStart(runbook.StepTypeCommand)

	summary := c.GetSummary()
	if summary.TotalSteps != 1 {
		t.Errorf("expected TotalSteps=1, got %d", summary.TotalSteps)
	}

	// Record step completion
	c.RecordStepComplete(runbook.StepTypeCommand, 10*time.Millisecond)

	summary = c.GetSummary()
	if summary.CompletedSteps != 1 {
		t.Errorf("expected CompletedSteps=1, got %d", summary.CompletedSteps)
	}
}

func TestRecordStepFailed(t *testing.T) {
	c := NewCollector()

	c.RecordStepStart(runbook.StepTypeAPI)
	c.RecordStepFailed(runbook.StepTypeAPI, 10*time.Millisecond)

	summary := c.GetSummary()
	if summary.FailedSteps != 1 {
		t.Errorf("expected FailedSteps=1, got %d", summary.FailedSteps)
	}
}

func TestRecordStepSkipped(t *testing.T) {
	c := NewCollector()

	c.RecordStepSkipped(runbook.StepTypeNoop)

	summary := c.GetSummary()
	if summary.SkippedSteps != 1 {
		t.Errorf("expected SkippedSteps=1, got %d", summary.SkippedSteps)
	}
}

func TestRecordStepRetry(t *testing.T) {
	c := NewCollector()

	c.RecordStepStart(runbook.StepTypeCommand)
	c.RecordStepRetry(runbook.StepTypeCommand)
	c.RecordStepRetry(runbook.StepTypeCommand)

	summary := c.GetSummary()
	if summary.RetriedSteps != 2 {
		t.Errorf("expected RetriedSteps=2, got %d", summary.RetriedSteps)
	}

	// Check step type metrics
	metrics := summary.StepTypeMetrics[runbook.StepTypeCommand]
	if metrics == nil {
		t.Fatal("expected step type metrics for command")
	}
	if metrics.RetryCount != 2 {
		t.Errorf("expected RetryCount=2, got %d", metrics.RetryCount)
	}
}

func TestRunbookMetrics(t *testing.T) {
	c := NewCollector()

	// Execute multiple runbooks
	c.RecordExecutionStart("runbook-a")
	c.RecordExecutionComplete("runbook-a", 100*time.Millisecond)

	c.RecordExecutionStart("runbook-a")
	c.RecordExecutionComplete("runbook-a", 200*time.Millisecond)

	c.RecordExecutionStart("runbook-b")
	c.RecordExecutionFailed("runbook-b", 50*time.Millisecond)

	summary := c.GetSummary()

	// Check runbook-a metrics
	metricsA := summary.RunbookMetrics["runbook-a"]
	if metricsA == nil {
		t.Fatal("expected metrics for runbook-a")
	}
	if metricsA.ExecutionCount != 2 {
		t.Errorf("expected ExecutionCount=2, got %d", metricsA.ExecutionCount)
	}
	if metricsA.SuccessCount != 2 {
		t.Errorf("expected SuccessCount=2, got %d", metricsA.SuccessCount)
	}

	// Check runbook-b metrics
	metricsB := summary.RunbookMetrics["runbook-b"]
	if metricsB == nil {
		t.Fatal("expected metrics for runbook-b")
	}
	if metricsB.FailureCount != 1 {
		t.Errorf("expected FailureCount=1, got %d", metricsB.FailureCount)
	}
}

func TestStepTypeMetrics(t *testing.T) {
	c := NewCollector()

	// Execute multiple step types
	c.RecordStepStart(runbook.StepTypeCommand)
	c.RecordStepComplete(runbook.StepTypeCommand, 10*time.Millisecond)

	c.RecordStepStart(runbook.StepTypeCommand)
	c.RecordStepComplete(runbook.StepTypeCommand, 20*time.Millisecond)

	c.RecordStepStart(runbook.StepTypeAPI)
	c.RecordStepFailed(runbook.StepTypeAPI, 30*time.Millisecond)

	summary := c.GetSummary()

	// Check command metrics
	cmdMetrics := summary.StepTypeMetrics[runbook.StepTypeCommand]
	if cmdMetrics == nil {
		t.Fatal("expected metrics for command step type")
	}
	if cmdMetrics.ExecutionCount != 2 {
		t.Errorf("expected ExecutionCount=2, got %d", cmdMetrics.ExecutionCount)
	}
	if cmdMetrics.SuccessCount != 2 {
		t.Errorf("expected SuccessCount=2, got %d", cmdMetrics.SuccessCount)
	}

	// Check API metrics
	apiMetrics := summary.StepTypeMetrics[runbook.StepTypeAPI]
	if apiMetrics == nil {
		t.Fatal("expected metrics for API step type")
	}
	if apiMetrics.FailureCount != 1 {
		t.Errorf("expected FailureCount=1, got %d", apiMetrics.FailureCount)
	}
}

func TestSuccessRate(t *testing.T) {
	c := NewCollector()

	// 3 successful, 1 failed
	c.RecordExecutionStart("test")
	c.RecordExecutionComplete("test", 10*time.Millisecond)

	c.RecordExecutionStart("test")
	c.RecordExecutionComplete("test", 10*time.Millisecond)

	c.RecordExecutionStart("test")
	c.RecordExecutionComplete("test", 10*time.Millisecond)

	c.RecordExecutionStart("test")
	c.RecordExecutionFailed("test", 10*time.Millisecond)

	summary := c.GetSummary()

	// Expected: 3/4 = 75%
	if summary.SuccessRate != 75.0 {
		t.Errorf("expected SuccessRate=75, got %.2f", summary.SuccessRate)
	}
}

func TestLatencyPercentiles(t *testing.T) {
	c := NewCollector()

	// Add durations
	durations := []time.Duration{
		10 * time.Millisecond,
		20 * time.Millisecond,
		30 * time.Millisecond,
		40 * time.Millisecond,
		50 * time.Millisecond,
		100 * time.Millisecond,
		200 * time.Millisecond,
		300 * time.Millisecond,
		400 * time.Millisecond,
		500 * time.Millisecond,
	}

	for _, d := range durations {
		c.RecordExecutionStart("test")
		c.RecordExecutionComplete("test", d)
	}

	summary := c.GetSummary()

	// Check median is around middle value
	if summary.MedianExecutionDuration < 40*time.Millisecond || summary.MedianExecutionDuration > 100*time.Millisecond {
		t.Errorf("unexpected MedianExecutionDuration: %v", summary.MedianExecutionDuration)
	}

	// Check p95 is high
	if summary.P95ExecutionDuration < 300*time.Millisecond {
		t.Errorf("P95ExecutionDuration should be >= 300ms, got %v", summary.P95ExecutionDuration)
	}
}

func TestLatencyBuckets(t *testing.T) {
	c := NewCollector()

	// Add executions with various durations
	c.RecordExecutionStart("test")
	c.RecordExecutionComplete("test", 5*time.Millisecond) // <= 10ms bucket

	c.RecordExecutionStart("test")
	c.RecordExecutionComplete("test", 75*time.Millisecond) // <= 100ms bucket

	c.RecordExecutionStart("test")
	c.RecordExecutionComplete("test", 2*time.Second) // <= 5s bucket

	summary := c.GetSummary()

	// Check that buckets have counts
	var totalBucketCount int64
	for _, bucket := range summary.LatencyBuckets {
		totalBucketCount += bucket.Count
	}

	if totalBucketCount != 3 {
		t.Errorf("expected total bucket count=3, got %d", totalBucketCount)
	}
}

func TestReset(t *testing.T) {
	c := NewCollector()

	// Add some metrics
	c.RecordExecutionStart("test")
	c.RecordExecutionComplete("test", 100*time.Millisecond)
	c.RecordStepStart(runbook.StepTypeCommand)
	c.RecordStepComplete(runbook.StepTypeCommand, 10*time.Millisecond)

	// Reset
	c.Reset()

	summary := c.GetSummary()

	if summary.TotalExecutions != 0 {
		t.Errorf("expected TotalExecutions=0 after reset, got %d", summary.TotalExecutions)
	}
	if summary.TotalSteps != 0 {
		t.Errorf("expected TotalSteps=0 after reset, got %d", summary.TotalSteps)
	}
	if len(summary.RunbookMetrics) != 0 {
		t.Errorf("expected empty RunbookMetrics after reset")
	}
}

func TestConcurrentAccess(t *testing.T) {
	c := NewCollector()

	var wg sync.WaitGroup
	numGoroutines := 100
	numOpsPerGoroutine := 100

	wg.Add(numGoroutines)

	for i := 0; i < numGoroutines; i++ {
		go func(id int) {
			defer wg.Done()
			for j := 0; j < numOpsPerGoroutine; j++ {
				c.RecordExecutionStart("test-runbook")
				c.RecordStepStart(runbook.StepTypeNoop)
				c.RecordStepComplete(runbook.StepTypeNoop, time.Millisecond)
				c.RecordExecutionComplete("test-runbook", 10*time.Millisecond)
			}
		}(i)
	}

	wg.Wait()

	summary := c.GetSummary()
	expectedExecutions := int64(numGoroutines * numOpsPerGoroutine)

	if summary.TotalExecutions != expectedExecutions {
		t.Errorf("expected TotalExecutions=%d, got %d", expectedExecutions, summary.TotalExecutions)
	}
	if summary.CompletedCount != expectedExecutions {
		t.Errorf("expected CompletedCount=%d, got %d", expectedExecutions, summary.CompletedCount)
	}
}

func TestPrometheusMetrics(t *testing.T) {
	c := NewCollector()

	// Add some metrics
	c.RecordExecutionStart("test")
	c.RecordExecutionComplete("test", 100*time.Millisecond)
	c.RecordExecutionStart("test")
	c.RecordExecutionFailed("test", 50*time.Millisecond)

	c.RecordStepStart(runbook.StepTypeCommand)
	c.RecordStepComplete(runbook.StepTypeCommand, 10*time.Millisecond)

	output := c.PrometheusMetrics()

	// Check for expected metrics
	expectedMetrics := []string{
		"runbook_executions_total",
		"runbook_executions_active",
		"runbook_executions_completed_total",
		"runbook_executions_failed_total",
		"runbook_success_rate",
		"runbook_steps_total",
		"runbook_steps_completed_total",
	}

	for _, metric := range expectedMetrics {
		if !strings.Contains(output, metric) {
			t.Errorf("expected Prometheus output to contain %q", metric)
		}
	}

	// Check that it contains values
	if !strings.Contains(output, "2") { // total executions
		t.Error("expected output to contain count values")
	}
}

func TestMinMaxDuration(t *testing.T) {
	c := NewCollector()

	c.RecordExecutionStart("test")
	c.RecordExecutionComplete("test", 100*time.Millisecond)

	c.RecordExecutionStart("test")
	c.RecordExecutionComplete("test", 50*time.Millisecond)

	c.RecordExecutionStart("test")
	c.RecordExecutionComplete("test", 200*time.Millisecond)

	summary := c.GetSummary()
	metrics := summary.RunbookMetrics["test"]

	if metrics.MinDuration != 50*time.Millisecond {
		t.Errorf("expected MinDuration=50ms, got %v", metrics.MinDuration)
	}

	if metrics.MaxDuration != 200*time.Millisecond {
		t.Errorf("expected MaxDuration=200ms, got %v", metrics.MaxDuration)
	}
}

func TestAverageDuration(t *testing.T) {
	c := NewCollector()

	c.RecordExecutionStart("test")
	c.RecordExecutionComplete("test", 100*time.Millisecond)

	c.RecordExecutionStart("test")
	c.RecordExecutionComplete("test", 200*time.Millisecond)

	c.RecordExecutionStart("test")
	c.RecordExecutionComplete("test", 300*time.Millisecond)

	summary := c.GetSummary()
	metrics := summary.RunbookMetrics["test"]

	// Average should be 200ms
	expected := 200 * time.Millisecond
	if metrics.AverageDuration != expected {
		t.Errorf("expected AverageDuration=%v, got %v", expected, metrics.AverageDuration)
	}
}

func TestCalculatePercentile(t *testing.T) {
	tests := []struct {
		name       string
		durations  []time.Duration
		percentile int
		expected   time.Duration
	}{
		{
			name:       "empty",
			durations:  []time.Duration{},
			percentile: 50,
			expected:   0,
		},
		{
			name:       "single value",
			durations:  []time.Duration{100 * time.Millisecond},
			percentile: 50,
			expected:   100 * time.Millisecond,
		},
		{
			name: "median of 5",
			durations: []time.Duration{
				10 * time.Millisecond,
				20 * time.Millisecond,
				30 * time.Millisecond,
				40 * time.Millisecond,
				50 * time.Millisecond,
			},
			percentile: 50,
			expected:   30 * time.Millisecond,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := calculatePercentile(tt.durations, tt.percentile)
			if result != tt.expected {
				t.Errorf("expected %v, got %v", tt.expected, result)
			}
		})
	}
}

func TestStepSuccessRate(t *testing.T) {
	c := NewCollector()

	// 4 completed, 1 failed
	for i := 0; i < 4; i++ {
		c.RecordStepStart(runbook.StepTypeNoop)
		c.RecordStepComplete(runbook.StepTypeNoop, 10*time.Millisecond)
	}

	c.RecordStepStart(runbook.StepTypeNoop)
	c.RecordStepFailed(runbook.StepTypeNoop, 10*time.Millisecond)

	summary := c.GetSummary()

	// Expected: 4/5 = 80%
	if summary.StepSuccessRate != 80.0 {
		t.Errorf("expected StepSuccessRate=80, got %.2f", summary.StepSuccessRate)
	}
}
