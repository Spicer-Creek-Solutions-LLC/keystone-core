package promotion

import (
	"context"
	"fmt"
	"sync"
	"time"
)

// ThresholdMetric defines a metric to evaluate
type ThresholdMetric string

const (
	// MetricErrorRate error rate percentage (0-100)
	MetricErrorRate ThresholdMetric = "error_rate"
	// MetricLatencyP50 50th percentile latency in milliseconds
	MetricLatencyP50 ThresholdMetric = "latency_p50"
	// MetricLatencyP95 95th percentile latency in milliseconds
	MetricLatencyP95 ThresholdMetric = "latency_p95"
	// MetricLatencyP99 99th percentile latency in milliseconds
	MetricLatencyP99 ThresholdMetric = "latency_p99"
	// MetricSuccessRate success rate percentage (0-100)
	MetricSuccessRate ThresholdMetric = "success_rate"
	// MetricThroughput requests per second
	MetricThroughput ThresholdMetric = "throughput"
	// MetricCPUUsage CPU usage percentage (0-100)
	MetricCPUUsage ThresholdMetric = "cpu_usage"
	// MetricMemoryUsage memory usage percentage (0-100)
	MetricMemoryUsage ThresholdMetric = "memory_usage"
	// MetricSaturation system saturation (0-100)
	MetricSaturation ThresholdMetric = "saturation"
)

// ThresholdOperator defines how to compare the metric
type ThresholdOperator string

const (
	// OperatorLessThan metric must be less than threshold
	OperatorLessThan ThresholdOperator = "lt"
	// OperatorLessOrEqual metric must be less than or equal to threshold
	OperatorLessOrEqual ThresholdOperator = "lte"
	// OperatorGreaterThan metric must be greater than threshold
	OperatorGreaterThan ThresholdOperator = "gt"
	// OperatorGreaterOrEqual metric must be greater than or equal to threshold
	OperatorGreaterOrEqual ThresholdOperator = "gte"
	// OperatorEqual metric must equal threshold
	OperatorEqual ThresholdOperator = "eq"
	// OperatorNotEqual metric must not equal threshold
	OperatorNotEqual ThresholdOperator = "neq"
)

// Threshold defines a single metric threshold
type Threshold struct {
	// Metric to evaluate
	Metric ThresholdMetric `json:"metric"`

	// Operator for comparison
	Operator ThresholdOperator `json:"operator"`

	// Value to compare against
	Value float64 `json:"value"`

	// FailureTolerance allows a percentage of samples to fail (0-100)
	FailureTolerance float64 `json:"failure_tolerance,omitempty"`

	// MinSamples required before evaluation
	MinSamples int `json:"min_samples,omitempty"`
}

// Evaluate evaluates whether the metric meets the threshold
func (t *Threshold) Evaluate(value float64) bool {
	switch t.Operator {
	case OperatorLessThan:
		return value < t.Value
	case OperatorLessOrEqual:
		return value <= t.Value
	case OperatorGreaterThan:
		return value > t.Value
	case OperatorGreaterOrEqual:
		return value >= t.Value
	case OperatorEqual:
		return value == t.Value
	case OperatorNotEqual:
		return value != t.Value
	default:
		return false
	}
}

// String returns a human-readable representation
func (t *Threshold) String() string {
	opStr := map[ThresholdOperator]string{
		OperatorLessThan:       "<",
		OperatorLessOrEqual:    "<=",
		OperatorGreaterThan:    ">",
		OperatorGreaterOrEqual: ">=",
		OperatorEqual:          "==",
		OperatorNotEqual:       "!=",
	}
	return fmt.Sprintf("%s %s %.2f", t.Metric, opStr[t.Operator], t.Value)
}

// ThresholdConfig defines a complete threshold configuration
type ThresholdConfig struct {
	// Name for this threshold configuration
	Name string `json:"name"`

	// Description of what this config checks
	Description string `json:"description,omitempty"`

	// Thresholds to evaluate
	Thresholds []Threshold `json:"thresholds"`

	// FailurePolicy determines what happens when thresholds fail
	FailurePolicy FailurePolicy `json:"failure_policy"`

	// EvaluationInterval how often to check thresholds
	EvaluationInterval time.Duration `json:"evaluation_interval,omitempty"`

	// WarmupPeriod before starting evaluation
	WarmupPeriod time.Duration `json:"warmup_period,omitempty"`

	// ConsecutiveFailures before marking as failed
	ConsecutiveFailures int `json:"consecutive_failures,omitempty"`
}

// FailurePolicy defines what to do when thresholds fail
type FailurePolicy string

const (
	// FailurePolicyRollback immediately rollback on threshold failure
	FailurePolicyRollback FailurePolicy = "rollback"
	// FailurePolicyPause pause deployment and wait for manual intervention
	FailurePolicyPause FailurePolicy = "pause"
	// FailurePolicyIgnore continue despite threshold failure (log warning)
	FailurePolicyIgnore FailurePolicy = "ignore"
	// FailurePolicyAbort abort deployment without rollback
	FailurePolicyAbort FailurePolicy = "abort"
)

// DefaultCanaryThresholds returns default thresholds for canary deployments
func DefaultCanaryThresholds() *ThresholdConfig {
	return &ThresholdConfig{
		Name:        "default-canary",
		Description: "Default canary deployment thresholds",
		Thresholds: []Threshold{
			{Metric: MetricErrorRate, Operator: OperatorLessThan, Value: 1.0, FailureTolerance: 0},
			{Metric: MetricLatencyP95, Operator: OperatorLessThan, Value: 500, FailureTolerance: 5},
			{Metric: MetricSuccessRate, Operator: OperatorGreaterOrEqual, Value: 99.0, FailureTolerance: 0},
		},
		FailurePolicy:       FailurePolicyRollback,
		EvaluationInterval:  30 * time.Second,
		WarmupPeriod:        60 * time.Second,
		ConsecutiveFailures: 3,
	}
}

// DefaultBlueGreenThresholds returns default thresholds for blue-green deployments
func DefaultBlueGreenThresholds() *ThresholdConfig {
	return &ThresholdConfig{
		Name:        "default-blue-green",
		Description: "Default blue-green deployment thresholds",
		Thresholds: []Threshold{
			{Metric: MetricErrorRate, Operator: OperatorLessThan, Value: 0.5, FailureTolerance: 0},
			{Metric: MetricLatencyP99, Operator: OperatorLessThan, Value: 1000, FailureTolerance: 5},
			{Metric: MetricSuccessRate, Operator: OperatorGreaterOrEqual, Value: 99.5, FailureTolerance: 0},
			{Metric: MetricCPUUsage, Operator: OperatorLessThan, Value: 80, FailureTolerance: 10},
			{Metric: MetricMemoryUsage, Operator: OperatorLessThan, Value: 85, FailureTolerance: 10},
		},
		FailurePolicy:       FailurePolicyRollback,
		EvaluationInterval:  15 * time.Second,
		WarmupPeriod:        30 * time.Second,
		ConsecutiveFailures: 2,
	}
}

// DeploymentThresholds configures thresholds per deployment/environment
type DeploymentThresholds struct {
	// Environment this applies to (empty for default)
	Environment string `json:"environment,omitempty"`

	// Strategy this applies to (empty for all strategies)
	Strategy Strategy `json:"strategy,omitempty"`

	// Config threshold configuration
	Config *ThresholdConfig `json:"config"`

	// Inherit from parent configuration (merge thresholds)
	Inherit bool `json:"inherit,omitempty"`

	// Priority when multiple configs match (higher wins)
	Priority int `json:"priority,omitempty"`
}

// ThresholdRegistry manages threshold configurations
type ThresholdRegistry struct {
	// Global defaults by strategy
	defaults map[Strategy]*ThresholdConfig

	// Per-environment/deployment overrides
	overrides []*DeploymentThresholds

	mu sync.RWMutex
}

// NewThresholdRegistry creates a new threshold registry with defaults
func NewThresholdRegistry() *ThresholdRegistry {
	return &ThresholdRegistry{
		defaults: map[Strategy]*ThresholdConfig{
			StrategyCanary:    DefaultCanaryThresholds(),
			StrategyBlueGreen: DefaultBlueGreenThresholds(),
		},
		overrides: make([]*DeploymentThresholds, 0),
	}
}

// SetDefault sets the default threshold config for a strategy
func (r *ThresholdRegistry) SetDefault(strategy Strategy, config *ThresholdConfig) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.defaults[strategy] = config
}

// GetDefault returns the default threshold config for a strategy
func (r *ThresholdRegistry) GetDefault(strategy Strategy) *ThresholdConfig {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.defaults[strategy]
}

// AddOverride adds an environment/deployment-specific override
func (r *ThresholdRegistry) AddOverride(override *DeploymentThresholds) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.overrides = append(r.overrides, override)
}

// RemoveOverride removes an override for a specific environment
func (r *ThresholdRegistry) RemoveOverride(environment string, strategy Strategy) bool {
	r.mu.Lock()
	defer r.mu.Unlock()

	for i, o := range r.overrides {
		if o.Environment == environment && o.Strategy == strategy {
			r.overrides = append(r.overrides[:i], r.overrides[i+1:]...)
			return true
		}
	}
	return false
}

// GetThresholds returns the effective threshold config for an environment and strategy
func (r *ThresholdRegistry) GetThresholds(environment string, strategy Strategy) *ThresholdConfig {
	r.mu.RLock()
	defer r.mu.RUnlock()

	// Find matching overrides with priority
	var bestMatch *DeploymentThresholds
	for _, o := range r.overrides {
		// Check environment match (empty matches all)
		envMatch := o.Environment == "" || o.Environment == environment
		// Check strategy match (empty matches all)
		stratMatch := o.Strategy == "" || o.Strategy == strategy

		if envMatch && stratMatch {
			if bestMatch == nil || o.Priority > bestMatch.Priority {
				bestMatch = o
			}
		}
	}

	// Use override if found
	if bestMatch != nil {
		if bestMatch.Inherit {
			// Merge with defaults
			base := r.defaults[strategy]
			if base != nil {
				return mergeThresholds(base, bestMatch.Config)
			}
		}
		return bestMatch.Config
	}

	// Fall back to defaults
	return r.defaults[strategy]
}

// mergeThresholds merges two threshold configs (override takes precedence)
func mergeThresholds(base, override *ThresholdConfig) *ThresholdConfig {
	merged := &ThresholdConfig{
		Name:                override.Name,
		Description:         override.Description,
		FailurePolicy:       override.FailurePolicy,
		EvaluationInterval:  override.EvaluationInterval,
		WarmupPeriod:        override.WarmupPeriod,
		ConsecutiveFailures: override.ConsecutiveFailures,
	}

	// Use base values if override doesn't specify
	if merged.EvaluationInterval == 0 {
		merged.EvaluationInterval = base.EvaluationInterval
	}
	if merged.WarmupPeriod == 0 {
		merged.WarmupPeriod = base.WarmupPeriod
	}
	if merged.ConsecutiveFailures == 0 {
		merged.ConsecutiveFailures = base.ConsecutiveFailures
	}
	if merged.FailurePolicy == "" {
		merged.FailurePolicy = base.FailurePolicy
	}

	// Merge thresholds - override's thresholds for same metric take precedence
	thresholdMap := make(map[ThresholdMetric]Threshold)
	for _, t := range base.Thresholds {
		thresholdMap[t.Metric] = t
	}
	for _, t := range override.Thresholds {
		thresholdMap[t.Metric] = t
	}

	merged.Thresholds = make([]Threshold, 0, len(thresholdMap))
	for _, t := range thresholdMap {
		merged.Thresholds = append(merged.Thresholds, t)
	}

	return merged
}

// MetricsProvider interface for fetching metrics
type MetricsProvider interface {
	// GetMetric returns the current value of a metric
	GetMetric(ctx context.Context, metric ThresholdMetric, labels map[string]string) (float64, error)

	// GetMetricSamples returns multiple samples for evaluation
	GetMetricSamples(ctx context.Context, metric ThresholdMetric, labels map[string]string, duration time.Duration) ([]float64, error)
}

// ThresholdEvaluator evaluates thresholds against metrics
type ThresholdEvaluator struct {
	provider MetricsProvider
	registry *ThresholdRegistry
}

// NewThresholdEvaluator creates a new threshold evaluator
func NewThresholdEvaluator(provider MetricsProvider, registry *ThresholdRegistry) *ThresholdEvaluator {
	return &ThresholdEvaluator{
		provider: provider,
		registry: registry,
	}
}

// EvaluationResult contains the result of threshold evaluation
type EvaluationResult struct {
	// Passed indicates if all thresholds passed
	Passed bool `json:"passed"`

	// ThresholdResults individual threshold results
	ThresholdResults []ThresholdResult `json:"threshold_results"`

	// FailedCount number of failed thresholds
	FailedCount int `json:"failed_count"`

	// EvaluatedAt timestamp
	EvaluatedAt time.Time `json:"evaluated_at"`

	// Duration of evaluation
	Duration time.Duration `json:"duration"`

	// RecommendedAction based on failure policy
	RecommendedAction FailurePolicy `json:"recommended_action,omitempty"`

	// Message summary message
	Message string `json:"message"`
}

// ThresholdResult contains result for a single threshold
type ThresholdResult struct {
	// Threshold being evaluated
	Threshold Threshold `json:"threshold"`

	// Passed indicates if threshold passed
	Passed bool `json:"passed"`

	// ActualValue observed value
	ActualValue float64 `json:"actual_value"`

	// SampleCount number of samples evaluated
	SampleCount int `json:"sample_count"`

	// FailedSamples samples that failed threshold
	FailedSamples int `json:"failed_samples,omitempty"`

	// Message details
	Message string `json:"message"`
}

// Evaluate evaluates all thresholds for a deployment
func (e *ThresholdEvaluator) Evaluate(ctx context.Context, environment string, strategy Strategy, labels map[string]string) (*EvaluationResult, error) {
	startTime := time.Now()
	config := e.registry.GetThresholds(environment, strategy)
	if config == nil {
		return &EvaluationResult{
			Passed:      true,
			EvaluatedAt: startTime,
			Duration:    time.Since(startTime),
			Message:     "No thresholds configured",
		}, nil
	}

	result := &EvaluationResult{
		Passed:           true,
		ThresholdResults: make([]ThresholdResult, 0, len(config.Thresholds)),
		EvaluatedAt:      startTime,
	}

	// Evaluate each threshold
	for _, threshold := range config.Thresholds {
		tr, err := e.evaluateThreshold(ctx, threshold, labels, config.EvaluationInterval)
		if err != nil {
			return nil, fmt.Errorf("failed to evaluate threshold %s: %w", threshold.Metric, err)
		}
		result.ThresholdResults = append(result.ThresholdResults, tr)

		if !tr.Passed {
			result.FailedCount++
			result.Passed = false
		}
	}

	result.Duration = time.Since(startTime)

	if result.Passed {
		result.Message = fmt.Sprintf("All %d thresholds passed", len(config.Thresholds))
	} else {
		result.Message = fmt.Sprintf("%d of %d thresholds failed", result.FailedCount, len(config.Thresholds))
		result.RecommendedAction = config.FailurePolicy
	}

	return result, nil
}

// evaluateThreshold evaluates a single threshold
func (e *ThresholdEvaluator) evaluateThreshold(ctx context.Context, threshold Threshold, labels map[string]string, interval time.Duration) (ThresholdResult, error) {
	result := ThresholdResult{
		Threshold: threshold,
		Passed:    false,
	}

	// Get samples
	samples, err := e.provider.GetMetricSamples(ctx, threshold.Metric, labels, interval)
	if err != nil {
		return result, err
	}

	result.SampleCount = len(samples)

	// Check minimum samples
	if threshold.MinSamples > 0 && result.SampleCount < threshold.MinSamples {
		result.Passed = true // Pass if not enough samples yet
		result.Message = fmt.Sprintf("Insufficient samples: %d < %d required", result.SampleCount, threshold.MinSamples)
		return result, nil
	}

	// Evaluate samples
	failedCount := 0
	var sum float64
	for _, v := range samples {
		if !threshold.Evaluate(v) {
			failedCount++
		}
		sum += v
	}

	result.FailedSamples = failedCount

	// Calculate failure rate
	if result.SampleCount > 0 {
		result.ActualValue = sum / float64(result.SampleCount)
		failureRate := float64(failedCount) / float64(result.SampleCount) * 100

		// Check against tolerance
		result.Passed = failureRate <= threshold.FailureTolerance
		if result.Passed {
			result.Message = fmt.Sprintf("%s: %.2f meets threshold %s (%.1f%% failures, tolerance %.1f%%)",
				threshold.Metric, result.ActualValue, threshold.String(), failureRate, threshold.FailureTolerance)
		} else {
			result.Message = fmt.Sprintf("%s: %.2f violates threshold %s (%.1f%% failures > %.1f%% tolerance)",
				threshold.Metric, result.ActualValue, threshold.String(), failureRate, threshold.FailureTolerance)
		}
	}

	return result, nil
}

// ThresholdPresets provides common threshold configurations
var ThresholdPresets = map[string]*ThresholdConfig{
	"strict": {
		Name:        "strict",
		Description: "Strict thresholds for production-critical deployments",
		Thresholds: []Threshold{
			{Metric: MetricErrorRate, Operator: OperatorLessThan, Value: 0.1, FailureTolerance: 0},
			{Metric: MetricLatencyP99, Operator: OperatorLessThan, Value: 200, FailureTolerance: 0},
			{Metric: MetricSuccessRate, Operator: OperatorGreaterOrEqual, Value: 99.9, FailureTolerance: 0},
		},
		FailurePolicy:       FailurePolicyRollback,
		EvaluationInterval:  10 * time.Second,
		WarmupPeriod:        30 * time.Second,
		ConsecutiveFailures: 1,
	},
	"relaxed": {
		Name:        "relaxed",
		Description: "Relaxed thresholds for non-critical or development deployments",
		Thresholds: []Threshold{
			{Metric: MetricErrorRate, Operator: OperatorLessThan, Value: 5.0, FailureTolerance: 10},
			{Metric: MetricLatencyP95, Operator: OperatorLessThan, Value: 2000, FailureTolerance: 20},
			{Metric: MetricSuccessRate, Operator: OperatorGreaterOrEqual, Value: 95.0, FailureTolerance: 10},
		},
		FailurePolicy:       FailurePolicyPause,
		EvaluationInterval:  60 * time.Second,
		WarmupPeriod:        120 * time.Second,
		ConsecutiveFailures: 5,
	},
	"latency-sensitive": {
		Name:        "latency-sensitive",
		Description: "Thresholds optimized for latency-sensitive services",
		Thresholds: []Threshold{
			{Metric: MetricErrorRate, Operator: OperatorLessThan, Value: 1.0, FailureTolerance: 0},
			{Metric: MetricLatencyP50, Operator: OperatorLessThan, Value: 50, FailureTolerance: 5},
			{Metric: MetricLatencyP95, Operator: OperatorLessThan, Value: 100, FailureTolerance: 5},
			{Metric: MetricLatencyP99, Operator: OperatorLessThan, Value: 200, FailureTolerance: 5},
		},
		FailurePolicy:       FailurePolicyRollback,
		EvaluationInterval:  15 * time.Second,
		WarmupPeriod:        45 * time.Second,
		ConsecutiveFailures: 2,
	},
	"throughput-sensitive": {
		Name:        "throughput-sensitive",
		Description: "Thresholds optimized for high-throughput services",
		Thresholds: []Threshold{
			{Metric: MetricErrorRate, Operator: OperatorLessThan, Value: 0.5, FailureTolerance: 0},
			{Metric: MetricThroughput, Operator: OperatorGreaterOrEqual, Value: 1000, FailureTolerance: 10},
			{Metric: MetricCPUUsage, Operator: OperatorLessThan, Value: 70, FailureTolerance: 5},
			{Metric: MetricSaturation, Operator: OperatorLessThan, Value: 80, FailureTolerance: 5},
		},
		FailurePolicy:       FailurePolicyRollback,
		EvaluationInterval:  20 * time.Second,
		WarmupPeriod:        60 * time.Second,
		ConsecutiveFailures: 3,
	},
}

// GetPreset returns a preset threshold configuration by name
func GetPreset(name string) (*ThresholdConfig, bool) {
	config, ok := ThresholdPresets[name]
	return config, ok
}

// ListPresets returns all available preset names
func ListPresets() []string {
	names := make([]string, 0, len(ThresholdPresets))
	for name := range ThresholdPresets {
		names = append(names, name)
	}
	return names
}
