package policy

import (
	"context"
	"fmt"
	"sort"
	"sync"
	"time"
)

// ProfilingConfig configures policy profiling
type ProfilingConfig struct {
	// Enabled enables profiling
	Enabled bool `json:"enabled"`

	// SampleRate is the fraction of evaluations to profile (0.0 to 1.0)
	SampleRate float64 `json:"sample_rate"`

	// MaxHistorySize is the maximum number of samples to keep per policy
	MaxHistorySize int `json:"max_history_size"`

	// SlowThreshold is the duration above which an evaluation is considered slow
	SlowThreshold time.Duration `json:"slow_threshold"`

	// TrackMemory enables memory tracking (more overhead)
	TrackMemory bool `json:"track_memory"`

	// AggregationInterval is how often to aggregate statistics
	AggregationInterval time.Duration `json:"aggregation_interval"`
}

// DefaultProfilingConfig returns sensible defaults
func DefaultProfilingConfig() *ProfilingConfig {
	return &ProfilingConfig{
		Enabled:             true,
		SampleRate:          1.0, // Profile everything by default
		MaxHistorySize:      1000,
		SlowThreshold:       100 * time.Millisecond,
		TrackMemory:         false,
		AggregationInterval: time.Minute,
	}
}

// EvaluationSample represents a single evaluation sample
type EvaluationSample struct {
	// PolicyID that was evaluated
	PolicyID string `json:"policy_id"`

	// PolicyType (OPA, CEL, Builtin)
	PolicyType PolicyType `json:"policy_type"`

	// Duration of the evaluation
	Duration time.Duration `json:"duration"`

	// Allowed result
	Allowed bool `json:"allowed"`

	// ViolationCount number of violations
	ViolationCount int `json:"violation_count"`

	// InputSize approximate size of input in bytes
	InputSize int `json:"input_size,omitempty"`

	// MemoryUsed during evaluation (if tracked)
	MemoryUsed int64 `json:"memory_used,omitempty"`

	// EvaluatedAt when the evaluation occurred
	EvaluatedAt time.Time `json:"evaluated_at"`

	// Slow indicates if the evaluation exceeded slow threshold
	Slow bool `json:"slow"`

	// Error if evaluation failed
	Error string `json:"error,omitempty"`
}

// Stats contains aggregated statistics for a policy
type Stats struct {
	// PolicyID being tracked
	PolicyID string `json:"policy_id"`

	// PolicyType (OPA, CEL, Builtin)
	PolicyType PolicyType `json:"policy_type"`

	// PolicyName for display
	PolicyName string `json:"policy_name,omitempty"`

	// EvaluationCount total evaluations
	EvaluationCount int64 `json:"evaluation_count"`

	// SuccessCount evaluations that returned allowed=true
	SuccessCount int64 `json:"success_count"`

	// FailureCount evaluations that returned allowed=false
	FailureCount int64 `json:"failure_count"`

	// ErrorCount evaluations that had errors
	ErrorCount int64 `json:"error_count"`

	// SlowCount evaluations that exceeded slow threshold
	SlowCount int64 `json:"slow_count"`

	// TotalDuration sum of all evaluation durations
	TotalDuration time.Duration `json:"total_duration"`

	// MinDuration fastest evaluation
	MinDuration time.Duration `json:"min_duration"`

	// MaxDuration slowest evaluation
	MaxDuration time.Duration `json:"max_duration"`

	// AvgDuration average evaluation duration
	AvgDuration time.Duration `json:"avg_duration"`

	// P50Duration 50th percentile
	P50Duration time.Duration `json:"p50_duration"`

	// P90Duration 90th percentile
	P90Duration time.Duration `json:"p90_duration"`

	// P99Duration 99th percentile
	P99Duration time.Duration `json:"p99_duration"`

	// TotalViolations sum of all violations
	TotalViolations int64 `json:"total_violations"`

	// LastEvaluatedAt when the policy was last evaluated
	LastEvaluatedAt time.Time `json:"last_evaluated_at"`

	// FirstEvaluatedAt when the policy was first evaluated
	FirstEvaluatedAt time.Time `json:"first_evaluated_at"`
}

// Profiler profiles policy evaluations
type Profiler struct {
	config *ProfilingConfig

	// samples per policy
	samples map[string][]*EvaluationSample

	// aggregated stats per policy
	stats map[string]*Stats

	// slow evaluations
	slowSamples []*EvaluationSample

	mu sync.RWMutex

	// For sampling decision
	sampleCounter uint64
}

// NewProfiler creates a new policy profiler
func NewProfiler(config *ProfilingConfig) *Profiler {
	if config == nil {
		config = DefaultProfilingConfig()
	}

	return &Profiler{
		config:      config,
		samples:     make(map[string][]*EvaluationSample),
		stats:       make(map[string]*Stats),
		slowSamples: make([]*EvaluationSample, 0),
	}
}

// RecordEvaluation records an evaluation sample
func (p *Profiler) RecordEvaluation(sample *EvaluationSample) {
	if !p.config.Enabled {
		return
	}

	// Apply sampling
	if p.config.SampleRate < 1.0 {
		p.mu.Lock()
		p.sampleCounter++
		shouldSample := float64(p.sampleCounter%100) < (p.config.SampleRate * 100)
		p.mu.Unlock()

		if !shouldSample {
			// Still update stats even if not keeping the sample
			p.updateStatsOnly(sample)
			return
		}
	}

	p.mu.Lock()
	defer p.mu.Unlock()

	// Check if slow
	sample.Slow = sample.Duration >= p.config.SlowThreshold

	// Add to policy samples
	samples := p.samples[sample.PolicyID]
	if len(samples) >= p.config.MaxHistorySize {
		// Remove oldest sample
		samples = samples[1:]
	}
	p.samples[sample.PolicyID] = append(samples, sample)

	// Track slow samples separately
	if sample.Slow {
		if len(p.slowSamples) >= p.config.MaxHistorySize {
			p.slowSamples = p.slowSamples[1:]
		}
		p.slowSamples = append(p.slowSamples, sample)
	}

	// Update stats
	p.updateStats(sample)
}

// updateStatsOnly updates stats without storing the sample
func (p *Profiler) updateStatsOnly(sample *EvaluationSample) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.updateStats(sample)
}

// updateStats updates aggregated statistics (caller must hold lock)
func (p *Profiler) updateStats(sample *EvaluationSample) {
	stats, ok := p.stats[sample.PolicyID]
	if !ok {
		stats = &Stats{
			PolicyID:         sample.PolicyID,
			PolicyType:       sample.PolicyType,
			MinDuration:      sample.Duration,
			MaxDuration:      sample.Duration,
			FirstEvaluatedAt: sample.EvaluatedAt,
		}
		p.stats[sample.PolicyID] = stats
	}

	stats.EvaluationCount++
	stats.TotalDuration += sample.Duration
	stats.TotalViolations += int64(sample.ViolationCount)
	stats.LastEvaluatedAt = sample.EvaluatedAt

	if sample.Allowed {
		stats.SuccessCount++
	} else {
		stats.FailureCount++
	}

	if sample.Error != "" {
		stats.ErrorCount++
	}

	if sample.Slow {
		stats.SlowCount++
	}

	if sample.Duration < stats.MinDuration {
		stats.MinDuration = sample.Duration
	}
	if sample.Duration > stats.MaxDuration {
		stats.MaxDuration = sample.Duration
	}

	// Calculate average
	stats.AvgDuration = stats.TotalDuration / time.Duration(stats.EvaluationCount)
}

// GetStats returns stats for a specific policy
func (p *Profiler) GetStats(policyID string) *Stats {
	p.mu.RLock()
	defer p.mu.RUnlock()

	stats, ok := p.stats[policyID]
	if !ok {
		return nil
	}

	// Calculate percentiles from samples
	samples := p.samples[policyID]
	if len(samples) > 0 {
		stats.P50Duration = p.calculatePercentile(samples, 50)
		stats.P90Duration = p.calculatePercentile(samples, 90)
		stats.P99Duration = p.calculatePercentile(samples, 99)
	}

	return stats
}

// GetAllStats returns stats for all policies
func (p *Profiler) GetAllStats() []*Stats {
	p.mu.RLock()
	defer p.mu.RUnlock()

	result := make([]*Stats, 0, len(p.stats))
	for policyID, stats := range p.stats {
		// Calculate percentiles
		samples := p.samples[policyID]
		if len(samples) > 0 {
			stats.P50Duration = p.calculatePercentile(samples, 50)
			stats.P90Duration = p.calculatePercentile(samples, 90)
			stats.P99Duration = p.calculatePercentile(samples, 99)
		}
		result = append(result, stats)
	}

	// Sort by evaluation count (most used first)
	sort.Slice(result, func(i, j int) bool {
		return result[i].EvaluationCount > result[j].EvaluationCount
	})

	return result
}

// GetSamples returns samples for a specific policy
func (p *Profiler) GetSamples(policyID string) []*EvaluationSample {
	p.mu.RLock()
	defer p.mu.RUnlock()

	samples := p.samples[policyID]
	if samples == nil {
		return nil
	}

	result := make([]*EvaluationSample, len(samples))
	copy(result, samples)
	return result
}

// GetSlowSamples returns slow evaluation samples
func (p *Profiler) GetSlowSamples() []*EvaluationSample {
	p.mu.RLock()
	defer p.mu.RUnlock()

	result := make([]*EvaluationSample, len(p.slowSamples))
	copy(result, p.slowSamples)
	return result
}

// GetTopSlowest returns the top N slowest policies by average duration
func (p *Profiler) GetTopSlowest(n int) []*Stats {
	stats := p.GetAllStats()

	sort.Slice(stats, func(i, j int) bool {
		return stats[i].AvgDuration > stats[j].AvgDuration
	})

	if n > len(stats) {
		n = len(stats)
	}
	return stats[:n]
}

// GetTopMostUsed returns the top N most frequently evaluated policies
func (p *Profiler) GetTopMostUsed(n int) []*Stats {
	stats := p.GetAllStats()

	// Already sorted by evaluation count
	if n > len(stats) {
		n = len(stats)
	}
	return stats[:n]
}

// GetTopErrorRate returns policies with highest error rates
func (p *Profiler) GetTopErrorRate(n int) []*Stats {
	stats := p.GetAllStats()

	// Filter out policies with 0 errors
	withErrors := make([]*Stats, 0)
	for _, s := range stats {
		if s.ErrorCount > 0 {
			withErrors = append(withErrors, s)
		}
	}

	sort.Slice(withErrors, func(i, j int) bool {
		rateI := float64(withErrors[i].ErrorCount) / float64(withErrors[i].EvaluationCount)
		rateJ := float64(withErrors[j].ErrorCount) / float64(withErrors[j].EvaluationCount)
		return rateI > rateJ
	})

	if n > len(withErrors) {
		n = len(withErrors)
	}
	return withErrors[:n]
}

// calculatePercentile calculates the percentile duration from samples
func (p *Profiler) calculatePercentile(samples []*EvaluationSample, percentile float64) time.Duration {
	if len(samples) == 0 {
		return 0
	}

	// Extract durations and sort
	durations := make([]time.Duration, len(samples))
	for i, s := range samples {
		durations[i] = s.Duration
	}
	sort.Slice(durations, func(i, j int) bool {
		return durations[i] < durations[j]
	})

	// Calculate index
	index := int(float64(len(durations)-1) * (percentile / 100.0))
	return durations[index]
}

// Reset resets all profiling data
func (p *Profiler) Reset() {
	p.mu.Lock()
	defer p.mu.Unlock()

	p.samples = make(map[string][]*EvaluationSample)
	p.stats = make(map[string]*Stats)
	p.slowSamples = make([]*EvaluationSample, 0)
	p.sampleCounter = 0
}

// ResetPolicy resets profiling data for a specific policy
func (p *Profiler) ResetPolicy(policyID string) {
	p.mu.Lock()
	defer p.mu.Unlock()

	delete(p.samples, policyID)
	delete(p.stats, policyID)
}

// ProfilingReport provides a summary of profiling data
type ProfilingReport struct {
	// GeneratedAt when the report was generated
	GeneratedAt time.Time `json:"generated_at"`

	// TotalPolicies profiled
	TotalPolicies int `json:"total_policies"`

	// TotalEvaluations across all policies
	TotalEvaluations int64 `json:"total_evaluations"`

	// TotalDuration sum of all evaluation durations
	TotalDuration time.Duration `json:"total_duration"`

	// AvgDuration average evaluation duration
	AvgDuration time.Duration `json:"avg_duration"`

	// SlowEvaluations count
	SlowEvaluations int64 `json:"slow_evaluations"`

	// SlowPercentage percentage of slow evaluations
	SlowPercentage float64 `json:"slow_percentage"`

	// TopSlowest policies
	TopSlowest []*Stats `json:"top_slowest,omitempty"`

	// TopMostUsed policies
	TopMostUsed []*Stats `json:"top_most_used,omitempty"`

	// TopErrorRate policies
	TopErrorRate []*Stats `json:"top_error_rate,omitempty"`

	// ByType evaluation counts by policy type
	ByType map[PolicyType]int64 `json:"by_type"`

	// Recommendations for optimization
	Recommendations []string `json:"recommendations,omitempty"`
}

// GenerateReport generates a profiling report
func (p *Profiler) GenerateReport(topN int) *ProfilingReport {
	if topN <= 0 {
		topN = 5
	}

	stats := p.GetAllStats()

	report := &ProfilingReport{
		GeneratedAt:     time.Now(),
		TotalPolicies:   len(stats),
		ByType:          make(map[PolicyType]int64),
		Recommendations: make([]string, 0),
	}

	for _, s := range stats {
		report.TotalEvaluations += s.EvaluationCount
		report.TotalDuration += s.TotalDuration
		report.SlowEvaluations += s.SlowCount
		report.ByType[s.PolicyType] += s.EvaluationCount
	}

	if report.TotalEvaluations > 0 {
		report.AvgDuration = report.TotalDuration / time.Duration(report.TotalEvaluations)
		report.SlowPercentage = float64(report.SlowEvaluations) / float64(report.TotalEvaluations) * 100
	}

	report.TopSlowest = p.GetTopSlowest(topN)
	report.TopMostUsed = p.GetTopMostUsed(topN)
	report.TopErrorRate = p.GetTopErrorRate(topN)

	// Generate recommendations
	if report.SlowPercentage > 10 {
		report.Recommendations = append(report.Recommendations,
			fmt.Sprintf("%.1f%% of evaluations are slow (>%s). Consider optimizing policies.",
				report.SlowPercentage, p.config.SlowThreshold))
	}

	if len(report.TopSlowest) > 0 && report.TopSlowest[0].AvgDuration > p.config.SlowThreshold {
		report.Recommendations = append(report.Recommendations,
			fmt.Sprintf("Policy '%s' has average duration of %s. Consider reviewing and optimizing.",
				report.TopSlowest[0].PolicyID, report.TopSlowest[0].AvgDuration))
	}

	if len(report.TopErrorRate) > 0 {
		errPolicy := report.TopErrorRate[0]
		errorRate := float64(errPolicy.ErrorCount) / float64(errPolicy.EvaluationCount) * 100
		if errorRate > 5 {
			report.Recommendations = append(report.Recommendations,
				fmt.Sprintf("Policy '%s' has %.1f%% error rate. Investigate and fix issues.",
					errPolicy.PolicyID, errorRate))
		}
	}

	return report
}

// ProfiledEngine wraps an Engine with profiling
type ProfiledEngine struct {
	engine   *Engine
	profiler *Profiler
	registry *Registry
}

// NewProfiledEngine creates a profiled policy engine
func NewProfiledEngine(engine *Engine, registry *Registry, config *ProfilingConfig) *ProfiledEngine {
	return &ProfiledEngine{
		engine:   engine,
		profiler: NewProfiler(config),
		registry: registry,
	}
}

// Evaluate evaluates a policy and profiles the execution
func (e *ProfiledEngine) Evaluate(ctx context.Context, policyID string, input *EvaluationInput) (*EvaluationResult, error) {
	start := time.Now()

	result, err := e.engine.Evaluate(ctx, policyID, input)

	duration := time.Since(start)

	// Get policy type
	policyType := PolicyTypeCEL // Default
	if policy, ok := e.registry.GetPolicy(policyID); ok {
		policyType = policy.Type
	}

	sample := &EvaluationSample{
		PolicyID:    policyID,
		PolicyType:  policyType,
		Duration:    duration,
		EvaluatedAt: start,
	}

	if result != nil {
		sample.Allowed = result.Allowed
		sample.ViolationCount = len(result.Violations)
	}

	if err != nil {
		sample.Error = err.Error()
	}

	e.profiler.RecordEvaluation(sample)

	return result, err
}

// GetProfiler returns the profiler instance
func (e *ProfiledEngine) GetProfiler() *Profiler {
	return e.profiler
}

// GetEngine returns the underlying engine
func (e *ProfiledEngine) GetEngine() *Engine {
	return e.engine
}

// EvaluationTimer helps measure evaluation time
type EvaluationTimer struct {
	policyID   string
	policyType Type
	startTime  time.Time
	profiler   *Profiler
}

// StartTimer starts an evaluation timer
func (p *Profiler) StartTimer(policyID string, policyType Type) *EvaluationTimer {
	return &EvaluationTimer{
		policyID:   policyID,
		policyType: policyType,
		startTime:  time.Now(),
		profiler:   p,
	}
}

// Stop stops the timer and records the sample
func (t *EvaluationTimer) Stop(allowed bool, violationCount int, err error) time.Duration {
	duration := time.Since(t.startTime)

	sample := &EvaluationSample{
		PolicyID:       t.policyID,
		PolicyType:     t.policyType,
		Duration:       duration,
		Allowed:        allowed,
		ViolationCount: violationCount,
		EvaluatedAt:    t.startTime,
	}

	if err != nil {
		sample.Error = err.Error()
	}

	t.profiler.RecordEvaluation(sample)

	return duration
}

// Deprecated aliases for backward compatibility
//
//nolint:revive,staticcheck // stuttering names kept for backward compatibility
type (
	// PolicyStats is deprecated: Use Stats instead.
	PolicyStats = Stats
	// PolicyProfiler is deprecated: Use Profiler instead.
	PolicyProfiler = Profiler
	// ProfiledPolicyEngine is deprecated: Use ProfiledEngine instead.
	ProfiledPolicyEngine = ProfiledEngine
)

// NewPolicyProfiler is deprecated: Use NewProfiler instead.
var NewPolicyProfiler = NewProfiler

// NewProfiledPolicyEngine is deprecated: Use NewProfiledEngine instead.
var NewProfiledPolicyEngine = NewProfiledEngine
