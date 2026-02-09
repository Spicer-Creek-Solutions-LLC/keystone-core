// Package metrics provides metrics collection for runbook executions.
package metrics

import (
	"sync"
	"sync/atomic"
	"time"

	"github.com/shawnbutts/keystone-core/internal/runbook"
)

// Collector tracks runbook execution metrics.
type Collector struct {
	mu sync.RWMutex

	// Execution counts
	totalExecutions  int64
	activeExecutions int64
	completedCount   int64
	failedCount      int64
	cancelledCount   int64

	// Timing metrics
	executionDurations []time.Duration
	stepDurations      []time.Duration

	// Step metrics
	totalSteps     int64
	completedSteps int64
	failedSteps    int64
	skippedSteps   int64
	retriedSteps   int64

	// Per-runbook metrics
	runbookMetrics map[string]*RunbookMetrics

	// Per-step-type metrics
	stepTypeMetrics map[runbook.StepType]*StepTypeMetrics

	// Histogram buckets for latency distribution
	latencyBuckets []LatencyBucket
}

// RunbookMetrics tracks metrics for a specific runbook.
type RunbookMetrics struct {
	Name            string
	ExecutionCount  int64
	SuccessCount    int64
	FailureCount    int64
	TotalDuration   time.Duration
	AverageDuration time.Duration
	MinDuration     time.Duration
	MaxDuration     time.Duration
	LastExecutedAt  time.Time
}

// StepTypeMetrics tracks metrics for a specific step type.
type StepTypeMetrics struct {
	Type            runbook.StepType
	ExecutionCount  int64
	SuccessCount    int64
	FailureCount    int64
	TotalDuration   time.Duration
	AverageDuration time.Duration
	RetryCount      int64
}

// LatencyBucket represents a histogram bucket for latency distribution.
type LatencyBucket struct {
	UpperBound time.Duration
	Count      int64
}

// NewCollector creates a new metrics collector.
func NewCollector() *Collector {
	return &Collector{
		runbookMetrics:  make(map[string]*RunbookMetrics),
		stepTypeMetrics: make(map[runbook.StepType]*StepTypeMetrics),
		latencyBuckets: []LatencyBucket{
			{UpperBound: 10 * time.Millisecond},
			{UpperBound: 50 * time.Millisecond},
			{UpperBound: 100 * time.Millisecond},
			{UpperBound: 250 * time.Millisecond},
			{UpperBound: 500 * time.Millisecond},
			{UpperBound: 1 * time.Second},
			{UpperBound: 5 * time.Second},
			{UpperBound: 30 * time.Second},
			{UpperBound: 60 * time.Second},
			{UpperBound: 5 * time.Minute},
		},
	}
}

// RecordExecutionStart records the start of an execution.
func (c *Collector) RecordExecutionStart(runbookName string) {
	atomic.AddInt64(&c.totalExecutions, 1)
	atomic.AddInt64(&c.activeExecutions, 1)

	c.mu.Lock()
	defer c.mu.Unlock()

	if _, ok := c.runbookMetrics[runbookName]; !ok {
		c.runbookMetrics[runbookName] = &RunbookMetrics{Name: runbookName}
	}
	c.runbookMetrics[runbookName].ExecutionCount++
}

// RecordExecutionComplete records a completed execution.
func (c *Collector) RecordExecutionComplete(runbookName string, duration time.Duration) {
	atomic.AddInt64(&c.activeExecutions, -1)
	atomic.AddInt64(&c.completedCount, 1)

	c.mu.Lock()
	defer c.mu.Unlock()

	c.executionDurations = append(c.executionDurations, duration)
	c.recordLatencyBucket(duration)

	if metrics, ok := c.runbookMetrics[runbookName]; ok {
		metrics.SuccessCount++
		metrics.TotalDuration += duration
		metrics.LastExecutedAt = time.Now()

		if metrics.MinDuration == 0 || duration < metrics.MinDuration {
			metrics.MinDuration = duration
		}
		if duration > metrics.MaxDuration {
			metrics.MaxDuration = duration
		}

		if metrics.SuccessCount > 0 {
			metrics.AverageDuration = metrics.TotalDuration / time.Duration(metrics.SuccessCount)
		}
	}
}

// RecordExecutionFailed records a failed execution.
func (c *Collector) RecordExecutionFailed(runbookName string, duration time.Duration) {
	atomic.AddInt64(&c.activeExecutions, -1)
	atomic.AddInt64(&c.failedCount, 1)

	c.mu.Lock()
	defer c.mu.Unlock()

	c.executionDurations = append(c.executionDurations, duration)
	c.recordLatencyBucket(duration)

	if metrics, ok := c.runbookMetrics[runbookName]; ok {
		metrics.FailureCount++
		metrics.LastExecutedAt = time.Now()
	}
}

// RecordExecutionCancelled records a cancelled execution.
func (c *Collector) RecordExecutionCancelled(runbookName string) {
	atomic.AddInt64(&c.activeExecutions, -1)
	atomic.AddInt64(&c.cancelledCount, 1)

	c.mu.Lock()
	defer c.mu.Unlock()

	if metrics, ok := c.runbookMetrics[runbookName]; ok {
		metrics.LastExecutedAt = time.Now()
	}
}

// RecordStepStart records the start of a step.
func (c *Collector) RecordStepStart(stepType runbook.StepType) {
	atomic.AddInt64(&c.totalSteps, 1)

	c.mu.Lock()
	defer c.mu.Unlock()

	if _, ok := c.stepTypeMetrics[stepType]; !ok {
		c.stepTypeMetrics[stepType] = &StepTypeMetrics{Type: stepType}
	}
	c.stepTypeMetrics[stepType].ExecutionCount++
}

// RecordStepComplete records a completed step.
func (c *Collector) RecordStepComplete(stepType runbook.StepType, duration time.Duration) {
	atomic.AddInt64(&c.completedSteps, 1)

	c.mu.Lock()
	defer c.mu.Unlock()

	c.stepDurations = append(c.stepDurations, duration)

	if metrics, ok := c.stepTypeMetrics[stepType]; ok {
		metrics.SuccessCount++
		metrics.TotalDuration += duration

		if metrics.SuccessCount > 0 {
			metrics.AverageDuration = metrics.TotalDuration / time.Duration(metrics.SuccessCount)
		}
	}
}

// RecordStepFailed records a failed step.
func (c *Collector) RecordStepFailed(stepType runbook.StepType, duration time.Duration) {
	atomic.AddInt64(&c.failedSteps, 1)

	c.mu.Lock()
	defer c.mu.Unlock()

	if metrics, ok := c.stepTypeMetrics[stepType]; ok {
		metrics.FailureCount++
	}
}

// RecordStepSkipped records a skipped step.
func (c *Collector) RecordStepSkipped(stepType runbook.StepType) {
	atomic.AddInt64(&c.skippedSteps, 1)
}

// RecordStepRetry records a step retry.
func (c *Collector) RecordStepRetry(stepType runbook.StepType) {
	atomic.AddInt64(&c.retriedSteps, 1)

	c.mu.Lock()
	defer c.mu.Unlock()

	if metrics, ok := c.stepTypeMetrics[stepType]; ok {
		metrics.RetryCount++
	}
}

// recordLatencyBucket records a duration in the appropriate histogram bucket.
func (c *Collector) recordLatencyBucket(duration time.Duration) {
	for i := range c.latencyBuckets {
		if duration <= c.latencyBuckets[i].UpperBound {
			c.latencyBuckets[i].Count++
			return
		}
	}
	// Duration exceeds all buckets
	if len(c.latencyBuckets) > 0 {
		c.latencyBuckets[len(c.latencyBuckets)-1].Count++
	}
}

// Summary returns a summary of collected metrics.
type Summary struct {
	TotalExecutions  int64
	ActiveExecutions int64
	CompletedCount   int64
	FailedCount      int64
	CancelledCount   int64
	SuccessRate      float64

	TotalSteps      int64
	CompletedSteps  int64
	FailedSteps     int64
	SkippedSteps    int64
	RetriedSteps    int64
	StepSuccessRate float64

	AverageExecutionDuration time.Duration
	MedianExecutionDuration  time.Duration
	P95ExecutionDuration     time.Duration
	P99ExecutionDuration     time.Duration

	AverageStepDuration time.Duration

	RunbookMetrics  map[string]*RunbookMetrics
	StepTypeMetrics map[runbook.StepType]*StepTypeMetrics
	LatencyBuckets  []LatencyBucket
}

// GetSummary returns a summary of all collected metrics.
func (c *Collector) GetSummary() *Summary {
	c.mu.RLock()
	defer c.mu.RUnlock()

	total := atomic.LoadInt64(&c.totalExecutions)
	completed := atomic.LoadInt64(&c.completedCount)
	failed := atomic.LoadInt64(&c.failedCount)
	cancelled := atomic.LoadInt64(&c.cancelledCount)

	totalSteps := atomic.LoadInt64(&c.totalSteps)
	completedSteps := atomic.LoadInt64(&c.completedSteps)

	summary := &Summary{
		TotalExecutions:  total,
		ActiveExecutions: atomic.LoadInt64(&c.activeExecutions),
		CompletedCount:   completed,
		FailedCount:      failed,
		CancelledCount:   cancelled,

		TotalSteps:     totalSteps,
		CompletedSteps: completedSteps,
		FailedSteps:    atomic.LoadInt64(&c.failedSteps),
		SkippedSteps:   atomic.LoadInt64(&c.skippedSteps),
		RetriedSteps:   atomic.LoadInt64(&c.retriedSteps),

		RunbookMetrics:  make(map[string]*RunbookMetrics),
		StepTypeMetrics: make(map[runbook.StepType]*StepTypeMetrics),
		LatencyBuckets:  make([]LatencyBucket, len(c.latencyBuckets)),
	}

	// Calculate success rates
	finishedCount := completed + failed
	if finishedCount > 0 {
		summary.SuccessRate = float64(completed) / float64(finishedCount) * 100
	}

	finishedSteps := completedSteps + atomic.LoadInt64(&c.failedSteps)
	if finishedSteps > 0 {
		summary.StepSuccessRate = float64(completedSteps) / float64(finishedSteps) * 100
	}

	// Calculate duration percentiles
	if len(c.executionDurations) > 0 {
		summary.AverageExecutionDuration = calculateAverage(c.executionDurations)
		summary.MedianExecutionDuration = calculatePercentile(c.executionDurations, 50)
		summary.P95ExecutionDuration = calculatePercentile(c.executionDurations, 95)
		summary.P99ExecutionDuration = calculatePercentile(c.executionDurations, 99)
	}

	if len(c.stepDurations) > 0 {
		summary.AverageStepDuration = calculateAverage(c.stepDurations)
	}

	// Copy runbook metrics
	for name, metrics := range c.runbookMetrics {
		copied := *metrics
		summary.RunbookMetrics[name] = &copied
	}

	// Copy step type metrics
	for stepType, metrics := range c.stepTypeMetrics {
		copied := *metrics
		summary.StepTypeMetrics[stepType] = &copied
	}

	// Copy latency buckets
	copy(summary.LatencyBuckets, c.latencyBuckets)

	return summary
}

// Reset resets all metrics.
func (c *Collector) Reset() {
	c.mu.Lock()
	defer c.mu.Unlock()

	atomic.StoreInt64(&c.totalExecutions, 0)
	atomic.StoreInt64(&c.activeExecutions, 0)
	atomic.StoreInt64(&c.completedCount, 0)
	atomic.StoreInt64(&c.failedCount, 0)
	atomic.StoreInt64(&c.cancelledCount, 0)

	atomic.StoreInt64(&c.totalSteps, 0)
	atomic.StoreInt64(&c.completedSteps, 0)
	atomic.StoreInt64(&c.failedSteps, 0)
	atomic.StoreInt64(&c.skippedSteps, 0)
	atomic.StoreInt64(&c.retriedSteps, 0)

	c.executionDurations = nil
	c.stepDurations = nil
	c.runbookMetrics = make(map[string]*RunbookMetrics)
	c.stepTypeMetrics = make(map[runbook.StepType]*StepTypeMetrics)

	for i := range c.latencyBuckets {
		c.latencyBuckets[i].Count = 0
	}
}

// calculateAverage calculates the average of durations.
func calculateAverage(durations []time.Duration) time.Duration {
	if len(durations) == 0 {
		return 0
	}

	var total time.Duration
	for _, d := range durations {
		total += d
	}
	return total / time.Duration(len(durations))
}

// calculatePercentile calculates the nth percentile of durations.
func calculatePercentile(durations []time.Duration, n int) time.Duration {
	if len(durations) == 0 {
		return 0
	}

	// Make a copy and sort
	sorted := make([]time.Duration, len(durations))
	copy(sorted, durations)

	// Simple insertion sort for small slices
	for i := 1; i < len(sorted); i++ {
		key := sorted[i]
		j := i - 1
		for j >= 0 && sorted[j] > key {
			sorted[j+1] = sorted[j]
			j--
		}
		sorted[j+1] = key
	}

	// Calculate index
	index := (n * len(sorted)) / 100
	if index >= len(sorted) {
		index = len(sorted) - 1
	}

	return sorted[index]
}

// PrometheusMetrics returns metrics in Prometheus exposition format.
func (c *Collector) PrometheusMetrics() string {
	summary := c.GetSummary()

	var result string

	// Execution metrics
	result += "# HELP runbook_executions_total Total number of runbook executions\n"
	result += "# TYPE runbook_executions_total counter\n"
	result += formatMetric("runbook_executions_total", float64(summary.TotalExecutions))

	result += "# HELP runbook_executions_active Current number of active executions\n"
	result += "# TYPE runbook_executions_active gauge\n"
	result += formatMetric("runbook_executions_active", float64(summary.ActiveExecutions))

	result += "# HELP runbook_executions_completed_total Total number of completed executions\n"
	result += "# TYPE runbook_executions_completed_total counter\n"
	result += formatMetric("runbook_executions_completed_total", float64(summary.CompletedCount))

	result += "# HELP runbook_executions_failed_total Total number of failed executions\n"
	result += "# TYPE runbook_executions_failed_total counter\n"
	result += formatMetric("runbook_executions_failed_total", float64(summary.FailedCount))

	result += "# HELP runbook_executions_cancelled_total Total number of cancelled executions\n"
	result += "# TYPE runbook_executions_cancelled_total counter\n"
	result += formatMetric("runbook_executions_cancelled_total", float64(summary.CancelledCount))

	result += "# HELP runbook_success_rate Percentage of successful executions\n"
	result += "# TYPE runbook_success_rate gauge\n"
	result += formatMetric("runbook_success_rate", summary.SuccessRate)

	// Duration metrics
	result += "# HELP runbook_execution_duration_seconds Average execution duration\n"
	result += "# TYPE runbook_execution_duration_seconds gauge\n"
	result += formatMetric("runbook_execution_duration_seconds", summary.AverageExecutionDuration.Seconds())

	result += "# HELP runbook_execution_duration_p95_seconds 95th percentile execution duration\n"
	result += "# TYPE runbook_execution_duration_p95_seconds gauge\n"
	result += formatMetric("runbook_execution_duration_p95_seconds", summary.P95ExecutionDuration.Seconds())

	result += "# HELP runbook_execution_duration_p99_seconds 99th percentile execution duration\n"
	result += "# TYPE runbook_execution_duration_p99_seconds gauge\n"
	result += formatMetric("runbook_execution_duration_p99_seconds", summary.P99ExecutionDuration.Seconds())

	// Step metrics
	result += "# HELP runbook_steps_total Total number of steps executed\n"
	result += "# TYPE runbook_steps_total counter\n"
	result += formatMetric("runbook_steps_total", float64(summary.TotalSteps))

	result += "# HELP runbook_steps_completed_total Total number of completed steps\n"
	result += "# TYPE runbook_steps_completed_total counter\n"
	result += formatMetric("runbook_steps_completed_total", float64(summary.CompletedSteps))

	result += "# HELP runbook_steps_failed_total Total number of failed steps\n"
	result += "# TYPE runbook_steps_failed_total counter\n"
	result += formatMetric("runbook_steps_failed_total", float64(summary.FailedSteps))

	result += "# HELP runbook_steps_retried_total Total number of step retries\n"
	result += "# TYPE runbook_steps_retried_total counter\n"
	result += formatMetric("runbook_steps_retried_total", float64(summary.RetriedSteps))

	return result
}

func formatMetric(name string, value float64) string {
	return name + " " + formatFloat(value) + "\n"
}

func formatFloat(f float64) string {
	if f == float64(int64(f)) {
		return formatInt(int64(f))
	}
	return formatFloatWithPrecision(f, 6)
}

func formatInt(i int64) string {
	if i == 0 {
		return "0"
	}

	negative := i < 0
	if negative {
		i = -i
	}

	var buf [20]byte
	pos := len(buf)

	for i > 0 {
		pos--
		buf[pos] = byte('0' + i%10)
		i /= 10
	}

	if negative {
		pos--
		buf[pos] = '-'
	}

	return string(buf[pos:])
}

func formatFloatWithPrecision(f float64, precision int) string {
	// Simple float formatting
	negative := f < 0
	if negative {
		f = -f
	}

	intPart := int64(f)
	fracPart := f - float64(intPart)

	// Build fractional part
	for i := 0; i < precision; i++ {
		fracPart *= 10
	}
	fracInt := int64(fracPart + 0.5)

	result := formatInt(intPart) + "."

	// Pad with leading zeros
	fracStr := formatInt(fracInt)
	for len(fracStr) < precision {
		fracStr = "0" + fracStr
	}
	result += fracStr

	if negative {
		result = "-" + result
	}

	return result
}
