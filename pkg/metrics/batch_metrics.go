package metrics

import (
	"sort"
	"sync"
	"sync/atomic"
	"time"
)

// BatchMetricsAggregator aggregates metrics for batch command execution.
// It tracks success/failure ratios, execution times, and error categorization.
type BatchMetricsAggregator struct {
	mu sync.RWMutex

	// Counters
	totalExecutions    int64
	successfulAgents   int64
	failedAgents       int64
	skippedAgents      int64
	totalBatchJobs     int64
	completedBatchJobs int64
	failedBatchJobs    int64

	// Error categorization
	failuresByType map[string]int64

	// Execution time tracking
	executionTimes []time.Duration

	// Per-batch tracking
	currentBatch *BatchMetrics

	// Time window tracking for rate calculations
	windowStart time.Time
	windowSize  time.Duration
}

// BatchMetrics represents metrics for a single batch execution
type BatchMetrics struct {
	BatchJobID     string
	StartTime      time.Time
	EndTime        time.Time
	TotalAgents    int32
	SuccessCount   int32
	FailedCount    int32
	SkippedCount   int32
	ErrorsByType   map[string]int32
	AgentDurations []time.Duration
}

// BatchAggregateMetrics provides a snapshot of aggregated metrics
type BatchAggregateMetrics struct {
	// Overall totals
	TotalExecutions    int64   `json:"total_executions"`
	TotalBatchJobs     int64   `json:"total_batch_jobs"`
	CompletedBatchJobs int64   `json:"completed_batch_jobs"`
	FailedBatchJobs    int64   `json:"failed_batch_jobs"`
	SuccessfulAgents   int64   `json:"successful_agents"`
	FailedAgents       int64   `json:"failed_agents"`
	SkippedAgents      int64   `json:"skipped_agents"`
	OverallSuccessRate float64 `json:"overall_success_rate"`

	// Execution time percentiles (in milliseconds)
	P50DurationMs float64 `json:"p50_duration_ms"`
	P90DurationMs float64 `json:"p90_duration_ms"`
	P95DurationMs float64 `json:"p95_duration_ms"`
	P99DurationMs float64 `json:"p99_duration_ms"`
	AvgDurationMs float64 `json:"avg_duration_ms"`
	MinDurationMs float64 `json:"min_duration_ms"`
	MaxDurationMs float64 `json:"max_duration_ms"`

	// Error breakdown
	FailuresByType map[string]int64 `json:"failures_by_type"`

	// Rate metrics (per minute)
	ExecutionsPerMinute float64 `json:"executions_per_minute"`
	SuccessRateRecent   float64 `json:"success_rate_recent"`

	// Timestamp
	CollectedAt time.Time `json:"collected_at"`
}

// NewBatchMetricsAggregator creates a new batch metrics aggregator
func NewBatchMetricsAggregator() *BatchMetricsAggregator {
	return &BatchMetricsAggregator{
		failuresByType: make(map[string]int64),
		executionTimes: make([]time.Duration, 0, 10000),
		windowStart:    time.Now(),
		windowSize:     5 * time.Minute,
	}
}

// StartBatch begins tracking a new batch execution
func (b *BatchMetricsAggregator) StartBatch(batchJobID string, totalAgents int32) {
	b.mu.Lock()
	defer b.mu.Unlock()

	atomic.AddInt64(&b.totalBatchJobs, 1)

	b.currentBatch = &BatchMetrics{
		BatchJobID:     batchJobID,
		StartTime:      time.Now(),
		TotalAgents:    totalAgents,
		ErrorsByType:   make(map[string]int32),
		AgentDurations: make([]time.Duration, 0, totalAgents),
	}
}

// RecordAgentResult records the result of a command execution on a single agent
func (b *BatchMetricsAggregator) RecordAgentResult(success bool, duration time.Duration, errorType string) {
	b.mu.Lock()
	defer b.mu.Unlock()

	atomic.AddInt64(&b.totalExecutions, 1)

	if success {
		atomic.AddInt64(&b.successfulAgents, 1)
	} else {
		atomic.AddInt64(&b.failedAgents, 1)
		if errorType != "" {
			b.failuresByType[errorType]++
		} else {
			b.failuresByType["unknown"]++
		}
	}

	b.executionTimes = append(b.executionTimes, duration)

	// Update current batch if active
	if b.currentBatch != nil {
		if success {
			b.currentBatch.SuccessCount++
		} else {
			b.currentBatch.FailedCount++
			if errorType != "" {
				b.currentBatch.ErrorsByType[errorType]++
			}
		}
		b.currentBatch.AgentDurations = append(b.currentBatch.AgentDurations, duration)
	}
}

// RecordAgentSkipped records a skipped agent (e.g., offline)
func (b *BatchMetricsAggregator) RecordAgentSkipped() {
	atomic.AddInt64(&b.skippedAgents, 1)

	b.mu.Lock()
	if b.currentBatch != nil {
		b.currentBatch.SkippedCount++
	}
	b.mu.Unlock()
}

// CompleteBatch marks the current batch as completed
func (b *BatchMetricsAggregator) CompleteBatch(success bool) *BatchMetrics {
	b.mu.Lock()
	defer b.mu.Unlock()

	if b.currentBatch == nil {
		return nil
	}

	b.currentBatch.EndTime = time.Now()

	if success {
		atomic.AddInt64(&b.completedBatchJobs, 1)
	} else {
		atomic.AddInt64(&b.failedBatchJobs, 1)
	}

	batch := b.currentBatch
	b.currentBatch = nil
	return batch
}

// GetAggregateMetrics returns a snapshot of all aggregated metrics
func (b *BatchMetricsAggregator) GetAggregateMetrics() *BatchAggregateMetrics {
	b.mu.RLock()
	defer b.mu.RUnlock()

	metrics := &BatchAggregateMetrics{
		TotalExecutions:    atomic.LoadInt64(&b.totalExecutions),
		TotalBatchJobs:     atomic.LoadInt64(&b.totalBatchJobs),
		CompletedBatchJobs: atomic.LoadInt64(&b.completedBatchJobs),
		FailedBatchJobs:    atomic.LoadInt64(&b.failedBatchJobs),
		SuccessfulAgents:   atomic.LoadInt64(&b.successfulAgents),
		FailedAgents:       atomic.LoadInt64(&b.failedAgents),
		SkippedAgents:      atomic.LoadInt64(&b.skippedAgents),
		FailuresByType:     make(map[string]int64),
		CollectedAt:        time.Now(),
	}

	// Calculate success rate
	totalAgents := metrics.SuccessfulAgents + metrics.FailedAgents
	if totalAgents > 0 {
		metrics.OverallSuccessRate = float64(metrics.SuccessfulAgents) / float64(totalAgents) * 100.0
	}

	// Copy failures by type
	for k, v := range b.failuresByType {
		metrics.FailuresByType[k] = v
	}

	// Calculate execution time percentiles
	if len(b.executionTimes) > 0 {
		metrics.P50DurationMs, metrics.P90DurationMs, metrics.P95DurationMs, metrics.P99DurationMs = b.calculatePercentiles()
		metrics.AvgDurationMs, metrics.MinDurationMs, metrics.MaxDurationMs = b.calculateStats()
	}

	// Calculate rate metrics
	elapsed := time.Since(b.windowStart)
	if elapsed > 0 {
		metrics.ExecutionsPerMinute = float64(metrics.TotalExecutions) / elapsed.Minutes()
	}

	return metrics
}

// GetSuccessRate returns the current success rate as a percentage
func (b *BatchMetricsAggregator) GetSuccessRate() float64 {
	successful := atomic.LoadInt64(&b.successfulAgents)
	failed := atomic.LoadInt64(&b.failedAgents)
	total := successful + failed
	if total == 0 {
		return 100.0
	}
	return float64(successful) / float64(total) * 100.0
}

// GetFailureRatio returns the current failure ratio as a percentage
func (b *BatchMetricsAggregator) GetFailureRatio() float64 {
	successful := atomic.LoadInt64(&b.successfulAgents)
	failed := atomic.LoadInt64(&b.failedAgents)
	total := successful + failed
	if total == 0 {
		return 0.0
	}
	return float64(failed) / float64(total) * 100.0
}

// GetFailuresByType returns a copy of the failures by error type
func (b *BatchMetricsAggregator) GetFailuresByType() map[string]int64 {
	b.mu.RLock()
	defer b.mu.RUnlock()

	result := make(map[string]int64)
	for k, v := range b.failuresByType {
		result[k] = v
	}
	return result
}

// calculatePercentiles calculates p50, p90, p95, p99 for execution times
func (b *BatchMetricsAggregator) calculatePercentiles() (p50, p90, p95, p99 float64) {
	if len(b.executionTimes) == 0 {
		return 0, 0, 0, 0
	}

	// Make a copy and sort
	sorted := make([]time.Duration, len(b.executionTimes))
	copy(sorted, b.executionTimes)
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i] < sorted[j]
	})

	p50 = float64(sorted[percentileIndex(len(sorted), 50)].Milliseconds())
	p90 = float64(sorted[percentileIndex(len(sorted), 90)].Milliseconds())
	p95 = float64(sorted[percentileIndex(len(sorted), 95)].Milliseconds())
	p99 = float64(sorted[percentileIndex(len(sorted), 99)].Milliseconds())

	return p50, p90, p95, p99
}

// calculateStats calculates average, min, max for execution times
func (b *BatchMetricsAggregator) calculateStats() (avg, min, max float64) {
	if len(b.executionTimes) == 0 {
		return 0, 0, 0
	}

	var sum time.Duration
	minDur := b.executionTimes[0]
	maxDur := b.executionTimes[0]

	for _, d := range b.executionTimes {
		sum += d
		if d < minDur {
			minDur = d
		}
		if d > maxDur {
			maxDur = d
		}
	}

	avg = float64(sum.Milliseconds()) / float64(len(b.executionTimes))
	min = float64(minDur.Milliseconds())
	max = float64(maxDur.Milliseconds())

	return avg, min, max
}

// percentileIndex calculates the index for a given percentile
func percentileIndex(n int, percentile int) int {
	if n == 0 {
		return 0
	}
	idx := (percentile * n) / 100
	if idx >= n {
		idx = n - 1
	}
	return idx
}

// Reset clears all metrics
func (b *BatchMetricsAggregator) Reset() {
	b.mu.Lock()
	defer b.mu.Unlock()

	atomic.StoreInt64(&b.totalExecutions, 0)
	atomic.StoreInt64(&b.successfulAgents, 0)
	atomic.StoreInt64(&b.failedAgents, 0)
	atomic.StoreInt64(&b.skippedAgents, 0)
	atomic.StoreInt64(&b.totalBatchJobs, 0)
	atomic.StoreInt64(&b.completedBatchJobs, 0)
	atomic.StoreInt64(&b.failedBatchJobs, 0)

	b.failuresByType = make(map[string]int64)
	b.executionTimes = b.executionTimes[:0]
	b.currentBatch = nil
	b.windowStart = time.Now()
}

// Common error types for categorization
const (
	ErrorTypeTimeout     = "timeout"
	ErrorTypeNetwork     = "network"
	ErrorTypeAuth        = "auth"
	ErrorTypePermission  = "permission"
	ErrorTypeNotFound    = "not_found"
	ErrorTypeCommand     = "command"
	ErrorTypeInternal    = "internal"
	ErrorTypeAgentOffline = "agent_offline"
)

// ClassifyError categorizes an error into a standard type
func ClassifyError(err error) string {
	if err == nil {
		return ""
	}

	errStr := err.Error()

	// Check for common error patterns
	switch {
	case contains(errStr, "timeout", "timed out", "deadline exceeded"):
		return ErrorTypeTimeout
	case contains(errStr, "connection", "network", "refused", "unreachable"):
		return ErrorTypeNetwork
	case contains(errStr, "auth", "unauthorized", "forbidden", "credential"):
		return ErrorTypeAuth
	case contains(errStr, "permission", "denied", "access"):
		return ErrorTypePermission
	case contains(errStr, "not found", "no such", "does not exist"):
		return ErrorTypeNotFound
	case contains(errStr, "exit code", "command failed", "execution"):
		return ErrorTypeCommand
	case contains(errStr, "offline", "disconnected"):
		return ErrorTypeAgentOffline
	default:
		return ErrorTypeInternal
	}
}

// contains checks if s contains any of the substrings (case-insensitive)
func contains(s string, substrs ...string) bool {
	lower := lowerCase(s)
	for _, sub := range substrs {
		if lowerContains(lower, sub) {
			return true
		}
	}
	return false
}

// lowerCase converts a string to lowercase (simple ASCII)
func lowerCase(s string) string {
	b := make([]byte, len(s))
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c >= 'A' && c <= 'Z' {
			c += 'a' - 'A'
		}
		b[i] = c
	}
	return string(b)
}

// lowerContains checks if s contains substr (both assumed lowercase)
func lowerContains(s, substr string) bool {
	if len(substr) > len(s) {
		return false
	}
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == lowerCase(substr) {
			return true
		}
	}
	return false
}
