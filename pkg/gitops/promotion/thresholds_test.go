package promotion

import (
	"context"
	"testing"
	"time"
)

func TestThreshold_Evaluate(t *testing.T) {
	tests := []struct {
		name      string
		threshold Threshold
		value     float64
		want      bool
	}{
		{
			name:      "less than - passes",
			threshold: Threshold{Metric: MetricErrorRate, Operator: OperatorLessThan, Value: 1.0},
			value:     0.5,
			want:      true,
		},
		{
			name:      "less than - fails at boundary",
			threshold: Threshold{Metric: MetricErrorRate, Operator: OperatorLessThan, Value: 1.0},
			value:     1.0,
			want:      false,
		},
		{
			name:      "less than - fails over",
			threshold: Threshold{Metric: MetricErrorRate, Operator: OperatorLessThan, Value: 1.0},
			value:     1.5,
			want:      false,
		},
		{
			name:      "less or equal - passes at boundary",
			threshold: Threshold{Metric: MetricLatencyP95, Operator: OperatorLessOrEqual, Value: 500},
			value:     500,
			want:      true,
		},
		{
			name:      "greater than - passes",
			threshold: Threshold{Metric: MetricSuccessRate, Operator: OperatorGreaterThan, Value: 99.0},
			value:     99.5,
			want:      true,
		},
		{
			name:      "greater or equal - passes at boundary",
			threshold: Threshold{Metric: MetricSuccessRate, Operator: OperatorGreaterOrEqual, Value: 99.0},
			value:     99.0,
			want:      true,
		},
		{
			name:      "equal - passes",
			threshold: Threshold{Metric: MetricErrorRate, Operator: OperatorEqual, Value: 0},
			value:     0,
			want:      true,
		},
		{
			name:      "not equal - passes",
			threshold: Threshold{Metric: MetricErrorRate, Operator: OperatorNotEqual, Value: 0},
			value:     0.1,
			want:      true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.threshold.Evaluate(tt.value)
			if got != tt.want {
				t.Errorf("Evaluate() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestThreshold_String(t *testing.T) {
	threshold := Threshold{
		Metric:   MetricErrorRate,
		Operator: OperatorLessThan,
		Value:    1.0,
	}

	got := threshold.String()
	want := "error_rate < 1.00"
	if got != want {
		t.Errorf("String() = %q, want %q", got, want)
	}
}

func TestDefaultCanaryThresholds(t *testing.T) {
	config := DefaultCanaryThresholds()

	if config.Name != "default-canary" {
		t.Errorf("Name = %q, want %q", config.Name, "default-canary")
	}

	if len(config.Thresholds) != 3 {
		t.Errorf("Thresholds count = %d, want 3", len(config.Thresholds))
	}

	if config.FailurePolicy != FailurePolicyRollback {
		t.Errorf("FailurePolicy = %q, want %q", config.FailurePolicy, FailurePolicyRollback)
	}

	if config.ConsecutiveFailures != 3 {
		t.Errorf("ConsecutiveFailures = %d, want 3", config.ConsecutiveFailures)
	}
}

func TestDefaultBlueGreenThresholds(t *testing.T) {
	config := DefaultBlueGreenThresholds()

	if config.Name != "default-blue-green" {
		t.Errorf("Name = %q, want %q", config.Name, "default-blue-green")
	}

	if len(config.Thresholds) != 5 {
		t.Errorf("Thresholds count = %d, want 5", len(config.Thresholds))
	}

	// Blue-green should have stricter error rate
	for _, threshold := range config.Thresholds {
		if threshold.Metric == MetricErrorRate && threshold.Value != 0.5 {
			t.Errorf("Blue-green error rate threshold = %.2f, want 0.5", threshold.Value)
		}
	}
}

func TestThresholdRegistry_DefaultsAndOverrides(t *testing.T) {
	registry := NewThresholdRegistry()

	// Test default canary thresholds
	canaryConfig := registry.GetThresholds("production", StrategyCanary)
	if canaryConfig == nil {
		t.Fatal("Expected default canary thresholds")
	}
	if canaryConfig.Name != "default-canary" {
		t.Errorf("Name = %q, want %q", canaryConfig.Name, "default-canary")
	}

	// Test default blue-green thresholds
	blueGreenConfig := registry.GetThresholds("production", StrategyBlueGreen)
	if blueGreenConfig == nil {
		t.Fatal("Expected default blue-green thresholds")
	}
	if blueGreenConfig.Name != "default-blue-green" {
		t.Errorf("Name = %q, want %q", blueGreenConfig.Name, "default-blue-green")
	}

	// Add environment-specific override
	customConfig := &ThresholdConfig{
		Name: "production-strict",
		Thresholds: []Threshold{
			{Metric: MetricErrorRate, Operator: OperatorLessThan, Value: 0.1},
		},
		FailurePolicy:       FailurePolicyRollback,
		ConsecutiveFailures: 1,
	}

	registry.AddOverride(&DeploymentThresholds{
		Environment: "production",
		Strategy:    StrategyCanary,
		Config:      customConfig,
		Priority:    10,
	})

	// Should now get the override
	prodConfig := registry.GetThresholds("production", StrategyCanary)
	if prodConfig.Name != "production-strict" {
		t.Errorf("Name = %q, want %q", prodConfig.Name, "production-strict")
	}

	// Staging should still use defaults
	stagingConfig := registry.GetThresholds("staging", StrategyCanary)
	if stagingConfig.Name != "default-canary" {
		t.Errorf("Name = %q, want %q", stagingConfig.Name, "default-canary")
	}
}

func TestThresholdRegistry_InheritMerge(t *testing.T) {
	registry := NewThresholdRegistry()

	// Add override with inheritance
	customConfig := &ThresholdConfig{
		Name:                "production-extended",
		EvaluationInterval:  10 * time.Second,
		ConsecutiveFailures: 1,
		Thresholds: []Threshold{
			// Override error rate
			{Metric: MetricErrorRate, Operator: OperatorLessThan, Value: 0.1},
			// Add new threshold
			{Metric: MetricCPUUsage, Operator: OperatorLessThan, Value: 70},
		},
	}

	registry.AddOverride(&DeploymentThresholds{
		Environment: "production",
		Strategy:    StrategyCanary,
		Config:      customConfig,
		Inherit:     true,
		Priority:    10,
	})

	merged := registry.GetThresholds("production", StrategyCanary)

	// Should have merged thresholds (default has 3, we override 1 and add 1 = 4 total)
	if len(merged.Thresholds) < 3 {
		t.Errorf("Merged thresholds count = %d, expected at least 3", len(merged.Thresholds))
	}

	// Should use override's evaluation interval
	if merged.EvaluationInterval != 10*time.Second {
		t.Errorf("EvaluationInterval = %v, want %v", merged.EvaluationInterval, 10*time.Second)
	}

	// Should use override's consecutive failures
	if merged.ConsecutiveFailures != 1 {
		t.Errorf("ConsecutiveFailures = %d, want 1", merged.ConsecutiveFailures)
	}
}

func TestThresholdRegistry_Priority(t *testing.T) {
	registry := NewThresholdRegistry()

	// Add low priority override
	lowPriority := &ThresholdConfig{
		Name:          "low-priority",
		FailurePolicy: FailurePolicyPause,
	}
	registry.AddOverride(&DeploymentThresholds{
		Environment: "production",
		Config:      lowPriority,
		Priority:    5,
	})

	// Add high priority override
	highPriority := &ThresholdConfig{
		Name:          "high-priority",
		FailurePolicy: FailurePolicyRollback,
	}
	registry.AddOverride(&DeploymentThresholds{
		Environment: "production",
		Config:      highPriority,
		Priority:    10,
	})

	// Should get high priority config
	config := registry.GetThresholds("production", StrategyCanary)
	if config.Name != "high-priority" {
		t.Errorf("Name = %q, want %q", config.Name, "high-priority")
	}
}

func TestThresholdRegistry_RemoveOverride(t *testing.T) {
	registry := NewThresholdRegistry()

	customConfig := &ThresholdConfig{Name: "custom"}
	registry.AddOverride(&DeploymentThresholds{
		Environment: "production",
		Strategy:    StrategyCanary,
		Config:      customConfig,
	})

	// Verify override is applied
	config := registry.GetThresholds("production", StrategyCanary)
	if config.Name != "custom" {
		t.Fatal("Override not applied")
	}

	// Remove override
	removed := registry.RemoveOverride("production", StrategyCanary)
	if !removed {
		t.Error("RemoveOverride should return true")
	}

	// Should fall back to defaults
	config = registry.GetThresholds("production", StrategyCanary)
	if config.Name != "default-canary" {
		t.Errorf("Name = %q, want %q", config.Name, "default-canary")
	}
}

// mockMetricsProvider for testing
type mockMetricsProvider struct {
	metrics map[ThresholdMetric][]float64
}

func (m *mockMetricsProvider) GetMetric(ctx context.Context, metric ThresholdMetric, labels map[string]string) (float64, error) {
	samples := m.metrics[metric]
	if len(samples) == 0 {
		return 0, nil
	}
	return samples[0], nil
}

func (m *mockMetricsProvider) GetMetricSamples(ctx context.Context, metric ThresholdMetric, labels map[string]string, duration time.Duration) ([]float64, error) {
	return m.metrics[metric], nil
}

func TestThresholdEvaluator_Evaluate(t *testing.T) {
	registry := NewThresholdRegistry()
	provider := &mockMetricsProvider{
		metrics: map[ThresholdMetric][]float64{
			MetricErrorRate:   {0.5, 0.3, 0.4, 0.6, 0.2},
			MetricLatencyP95:  {100, 150, 120, 180, 90},
			MetricSuccessRate: {99.5, 99.2, 99.8, 99.1, 99.6},
		},
	}

	evaluator := NewThresholdEvaluator(provider, registry)

	result, err := evaluator.Evaluate(context.Background(), "production", StrategyCanary, nil)
	if err != nil {
		t.Fatalf("Evaluate() error = %v", err)
	}

	if !result.Passed {
		t.Errorf("Expected evaluation to pass, but failed: %s", result.Message)
		for _, tr := range result.ThresholdResults {
			if !tr.Passed {
				t.Logf("Failed threshold: %s", tr.Message)
			}
		}
	}

	if len(result.ThresholdResults) != 3 {
		t.Errorf("ThresholdResults count = %d, want 3", len(result.ThresholdResults))
	}
}

func TestThresholdEvaluator_FailingThresholds(t *testing.T) {
	registry := NewThresholdRegistry()
	provider := &mockMetricsProvider{
		metrics: map[ThresholdMetric][]float64{
			MetricErrorRate:   {5.0, 6.0, 7.0, 8.0, 10.0}, // High error rate
			MetricLatencyP95:  {100, 150, 120, 180, 90},
			MetricSuccessRate: {99.5, 99.2, 99.8, 99.1, 99.6},
		},
	}

	evaluator := NewThresholdEvaluator(provider, registry)

	result, err := evaluator.Evaluate(context.Background(), "production", StrategyCanary, nil)
	if err != nil {
		t.Fatalf("Evaluate() error = %v", err)
	}

	if result.Passed {
		t.Error("Expected evaluation to fail due to high error rate")
	}

	if result.FailedCount != 1 {
		t.Errorf("FailedCount = %d, want 1", result.FailedCount)
	}

	if result.RecommendedAction != FailurePolicyRollback {
		t.Errorf("RecommendedAction = %q, want %q", result.RecommendedAction, FailurePolicyRollback)
	}
}

func TestThresholdEvaluator_InsufficientSamples(t *testing.T) {
	registry := NewThresholdRegistry()

	// Set custom config with minimum samples
	customConfig := &ThresholdConfig{
		Name: "custom",
		Thresholds: []Threshold{
			{Metric: MetricErrorRate, Operator: OperatorLessThan, Value: 1.0, MinSamples: 10},
		},
		FailurePolicy: FailurePolicyRollback,
	}
	registry.AddOverride(&DeploymentThresholds{
		Environment: "production",
		Strategy:    StrategyCanary,
		Config:      customConfig,
	})

	provider := &mockMetricsProvider{
		metrics: map[ThresholdMetric][]float64{
			MetricErrorRate: {0.5, 0.3, 0.4}, // Only 3 samples, need 10
		},
	}

	evaluator := NewThresholdEvaluator(provider, registry)

	result, err := evaluator.Evaluate(context.Background(), "production", StrategyCanary, nil)
	if err != nil {
		t.Fatalf("Evaluate() error = %v", err)
	}

	// Should pass because insufficient samples
	if !result.Passed {
		t.Error("Expected evaluation to pass with insufficient samples")
	}
}

func TestThresholdEvaluator_NoConfig(t *testing.T) {
	registry := NewThresholdRegistry()
	provider := &mockMetricsProvider{}

	evaluator := NewThresholdEvaluator(provider, registry)

	// Rolling strategy has no thresholds by default
	result, err := evaluator.Evaluate(context.Background(), "production", StrategyRolling, nil)
	if err != nil {
		t.Fatalf("Evaluate() error = %v", err)
	}

	if !result.Passed {
		t.Error("Expected evaluation to pass with no thresholds")
	}

	if result.Message != "No thresholds configured" {
		t.Errorf("Message = %q, want %q", result.Message, "No thresholds configured")
	}
}

func TestGetPreset(t *testing.T) {
	tests := []struct {
		name       string
		presetName string
		wantOK     bool
	}{
		{"strict preset", "strict", true},
		{"relaxed preset", "relaxed", true},
		{"latency-sensitive preset", "latency-sensitive", true},
		{"throughput-sensitive preset", "throughput-sensitive", true},
		{"unknown preset", "unknown", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config, ok := GetPreset(tt.presetName)
			if ok != tt.wantOK {
				t.Errorf("GetPreset() ok = %v, want %v", ok, tt.wantOK)
			}
			if tt.wantOK && config == nil {
				t.Error("Expected config to be non-nil")
			}
		})
	}
}

func TestListPresets(t *testing.T) {
	presets := ListPresets()

	if len(presets) != 4 {
		t.Errorf("ListPresets() returned %d presets, want 4", len(presets))
	}

	expected := map[string]bool{
		"strict":               true,
		"relaxed":              true,
		"latency-sensitive":    true,
		"throughput-sensitive": true,
	}

	for _, name := range presets {
		if !expected[name] {
			t.Errorf("Unexpected preset: %s", name)
		}
	}
}

func TestThresholdConfig_Presets(t *testing.T) {
	// Test strict preset has tighter thresholds
	strict, _ := GetPreset("strict")
	relaxed, _ := GetPreset("relaxed")

	var strictErrorRate, relaxedErrorRate float64
	for _, th := range strict.Thresholds {
		if th.Metric == MetricErrorRate {
			strictErrorRate = th.Value
		}
	}
	for _, th := range relaxed.Thresholds {
		if th.Metric == MetricErrorRate {
			relaxedErrorRate = th.Value
		}
	}

	if strictErrorRate >= relaxedErrorRate {
		t.Errorf("Strict error rate (%.2f) should be lower than relaxed (%.2f)",
			strictErrorRate, relaxedErrorRate)
	}

	// Strict should have fewer consecutive failures allowed
	if strict.ConsecutiveFailures >= relaxed.ConsecutiveFailures {
		t.Errorf("Strict consecutive failures (%d) should be less than relaxed (%d)",
			strict.ConsecutiveFailures, relaxed.ConsecutiveFailures)
	}
}

func TestThresholdWithFailureTolerance(t *testing.T) {
	registry := NewThresholdRegistry()

	// Config with 10% failure tolerance
	customConfig := &ThresholdConfig{
		Name: "tolerant",
		Thresholds: []Threshold{
			{Metric: MetricErrorRate, Operator: OperatorLessThan, Value: 1.0, FailureTolerance: 20},
		},
		FailurePolicy:      FailurePolicyRollback,
		EvaluationInterval: 30 * time.Second,
	}
	registry.AddOverride(&DeploymentThresholds{
		Environment: "production",
		Strategy:    StrategyCanary,
		Config:      customConfig,
	})

	provider := &mockMetricsProvider{
		metrics: map[ThresholdMetric][]float64{
			// 10 samples, 1 exceeds threshold (10% failure rate, within 20% tolerance)
			MetricErrorRate: {0.5, 0.3, 0.4, 0.6, 0.2, 0.5, 0.3, 0.4, 1.5, 0.2},
		},
	}

	evaluator := NewThresholdEvaluator(provider, registry)

	result, err := evaluator.Evaluate(context.Background(), "production", StrategyCanary, nil)
	if err != nil {
		t.Fatalf("Evaluate() error = %v", err)
	}

	if !result.Passed {
		t.Errorf("Expected evaluation to pass with 10%% failures within 20%% tolerance")
		for _, tr := range result.ThresholdResults {
			t.Logf("Result: %s", tr.Message)
		}
	}
}
