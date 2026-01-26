package policy

import (
	"context"
	"testing"
	"time"

	"github.com/shawnbutts/keystone-core/internal/testing/helpers"
)

func TestDefaultProfilingConfig(t *testing.T) {
	config := DefaultProfilingConfig()

	if !config.Enabled {
		t.Error("Expected profiling to be enabled by default")
	}
	if config.SampleRate != 1.0 {
		t.Errorf("Expected sample rate 1.0, got %f", config.SampleRate)
	}
	if config.MaxHistorySize != 1000 {
		t.Errorf("Expected max history 1000, got %d", config.MaxHistorySize)
	}
	if config.SlowThreshold != 100*time.Millisecond {
		t.Errorf("Expected slow threshold 100ms, got %v", config.SlowThreshold)
	}
}

func TestNewPolicyProfiler(t *testing.T) {
	profiler := NewPolicyProfiler(nil)

	if profiler == nil {
		t.Fatal("Expected profiler to be created")
	}
	if profiler.samples == nil {
		t.Error("Expected samples map to be initialized")
	}
	if profiler.stats == nil {
		t.Error("Expected stats map to be initialized")
	}
}

func TestPolicyProfiler_RecordEvaluation(t *testing.T) {
	profiler := NewPolicyProfiler(nil)

	sample := &EvaluationSample{
		PolicyID:       "test-policy",
		PolicyType:     PolicyTypeCEL,
		Duration:       50 * time.Millisecond,
		Allowed:        true,
		ViolationCount: 0,
		EvaluatedAt:    time.Now(),
	}

	profiler.RecordEvaluation(sample)

	samples := profiler.GetSamples("test-policy")
	if len(samples) != 1 {
		t.Errorf("Expected 1 sample, got %d", len(samples))
	}

	stats := profiler.GetStats("test-policy")
	if stats == nil {
		t.Fatal("Expected stats to be recorded")
	}
	if stats.EvaluationCount != 1 {
		t.Errorf("Expected 1 evaluation, got %d", stats.EvaluationCount)
	}
	if stats.SuccessCount != 1 {
		t.Errorf("Expected 1 success, got %d", stats.SuccessCount)
	}
}

func TestPolicyProfiler_RecordEvaluation_Slow(t *testing.T) {
	config := DefaultProfilingConfig()
	config.SlowThreshold = 10 * time.Millisecond
	profiler := NewPolicyProfiler(config)

	sample := &EvaluationSample{
		PolicyID:    "test-policy",
		PolicyType:  PolicyTypeCEL,
		Duration:    50 * time.Millisecond, // > 10ms threshold
		Allowed:     true,
		EvaluatedAt: time.Now(),
	}

	profiler.RecordEvaluation(sample)

	if !sample.Slow {
		t.Error("Expected sample to be marked as slow")
	}

	slowSamples := profiler.GetSlowSamples()
	if len(slowSamples) != 1 {
		t.Errorf("Expected 1 slow sample, got %d", len(slowSamples))
	}

	stats := profiler.GetStats("test-policy")
	if stats.SlowCount != 1 {
		t.Errorf("Expected 1 slow count, got %d", stats.SlowCount)
	}
}

func TestPolicyProfiler_RecordEvaluation_Disabled(t *testing.T) {
	config := DefaultProfilingConfig()
	config.Enabled = false
	profiler := NewPolicyProfiler(config)

	sample := &EvaluationSample{
		PolicyID:    "test-policy",
		PolicyType:  PolicyTypeCEL,
		Duration:    50 * time.Millisecond,
		Allowed:     true,
		EvaluatedAt: time.Now(),
	}

	profiler.RecordEvaluation(sample)

	samples := profiler.GetSamples("test-policy")
	if len(samples) != 0 {
		t.Error("Expected no samples when profiling is disabled")
	}
}

func TestPolicyProfiler_RecordEvaluation_MaxHistory(t *testing.T) {
	config := DefaultProfilingConfig()
	config.MaxHistorySize = 5
	profiler := NewPolicyProfiler(config)

	// Record more samples than max
	for i := 0; i < 10; i++ {
		sample := &EvaluationSample{
			PolicyID:    "test-policy",
			PolicyType:  PolicyTypeCEL,
			Duration:    time.Duration(i) * time.Millisecond,
			Allowed:     true,
			EvaluatedAt: time.Now(),
		}
		profiler.RecordEvaluation(sample)
	}

	samples := profiler.GetSamples("test-policy")
	if len(samples) != 5 {
		t.Errorf("Expected 5 samples (max), got %d", len(samples))
	}

	stats := profiler.GetStats("test-policy")
	if stats.EvaluationCount != 10 {
		t.Errorf("Expected 10 evaluations in stats, got %d", stats.EvaluationCount)
	}
}

func TestPolicyProfiler_RecordEvaluation_Stats(t *testing.T) {
	profiler := NewPolicyProfiler(nil)

	// Record multiple evaluations
	samples := []struct {
		duration   time.Duration
		allowed    bool
		violations int
		err        string
	}{
		{10 * time.Millisecond, true, 0, ""},
		{20 * time.Millisecond, false, 2, ""},
		{30 * time.Millisecond, true, 0, ""},
		{40 * time.Millisecond, false, 1, "error"},
	}

	for _, s := range samples {
		sample := &EvaluationSample{
			PolicyID:       "test-policy",
			PolicyType:     PolicyTypeCEL,
			Duration:       s.duration,
			Allowed:        s.allowed,
			ViolationCount: s.violations,
			Error:          s.err,
			EvaluatedAt:    time.Now(),
		}
		profiler.RecordEvaluation(sample)
	}

	stats := profiler.GetStats("test-policy")

	if stats.EvaluationCount != 4 {
		t.Errorf("Expected 4 evaluations, got %d", stats.EvaluationCount)
	}
	if stats.SuccessCount != 2 {
		t.Errorf("Expected 2 successes, got %d", stats.SuccessCount)
	}
	if stats.FailureCount != 2 {
		t.Errorf("Expected 2 failures, got %d", stats.FailureCount)
	}
	if stats.ErrorCount != 1 {
		t.Errorf("Expected 1 error, got %d", stats.ErrorCount)
	}
	if stats.TotalViolations != 3 {
		t.Errorf("Expected 3 total violations, got %d", stats.TotalViolations)
	}
	if stats.MinDuration != 10*time.Millisecond {
		t.Errorf("Expected min duration 10ms, got %v", stats.MinDuration)
	}
	if stats.MaxDuration != 40*time.Millisecond {
		t.Errorf("Expected max duration 40ms, got %v", stats.MaxDuration)
	}
	expectedAvg := (10 + 20 + 30 + 40) * time.Millisecond / 4
	if stats.AvgDuration != expectedAvg {
		t.Errorf("Expected avg duration %v, got %v", expectedAvg, stats.AvgDuration)
	}
}

func TestPolicyProfiler_GetAllStats(t *testing.T) {
	profiler := NewPolicyProfiler(nil)

	// Record evaluations for multiple policies
	for i := 0; i < 5; i++ {
		profiler.RecordEvaluation(&EvaluationSample{
			PolicyID:    "policy-a",
			PolicyType:  PolicyTypeCEL,
			Duration:    10 * time.Millisecond,
			Allowed:     true,
			EvaluatedAt: time.Now(),
		})
	}

	for i := 0; i < 3; i++ {
		profiler.RecordEvaluation(&EvaluationSample{
			PolicyID:    "policy-b",
			PolicyType:  PolicyTypeOPA,
			Duration:    20 * time.Millisecond,
			Allowed:     true,
			EvaluatedAt: time.Now(),
		})
	}

	stats := profiler.GetAllStats()

	if len(stats) != 2 {
		t.Errorf("Expected 2 policies, got %d", len(stats))
	}

	// Should be sorted by evaluation count (policy-a first)
	if stats[0].PolicyID != "policy-a" {
		t.Error("Expected policy-a first (most evaluations)")
	}
}

func TestPolicyProfiler_GetTopSlowest(t *testing.T) {
	profiler := NewPolicyProfiler(nil)

	profiler.RecordEvaluation(&EvaluationSample{
		PolicyID:    "fast-policy",
		PolicyType:  PolicyTypeCEL,
		Duration:    10 * time.Millisecond,
		Allowed:     true,
		EvaluatedAt: time.Now(),
	})

	profiler.RecordEvaluation(&EvaluationSample{
		PolicyID:    "slow-policy",
		PolicyType:  PolicyTypeCEL,
		Duration:    100 * time.Millisecond,
		Allowed:     true,
		EvaluatedAt: time.Now(),
	})

	slowest := profiler.GetTopSlowest(1)

	if len(slowest) != 1 {
		t.Fatalf("Expected 1 result, got %d", len(slowest))
	}
	if slowest[0].PolicyID != "slow-policy" {
		t.Error("Expected slow-policy to be slowest")
	}
}

func TestPolicyProfiler_GetTopMostUsed(t *testing.T) {
	profiler := NewPolicyProfiler(nil)

	for i := 0; i < 10; i++ {
		profiler.RecordEvaluation(&EvaluationSample{
			PolicyID:    "popular-policy",
			PolicyType:  PolicyTypeCEL,
			Duration:    10 * time.Millisecond,
			Allowed:     true,
			EvaluatedAt: time.Now(),
		})
	}

	profiler.RecordEvaluation(&EvaluationSample{
		PolicyID:    "rare-policy",
		PolicyType:  PolicyTypeCEL,
		Duration:    10 * time.Millisecond,
		Allowed:     true,
		EvaluatedAt: time.Now(),
	})

	mostUsed := profiler.GetTopMostUsed(1)

	if len(mostUsed) != 1 {
		t.Fatalf("Expected 1 result, got %d", len(mostUsed))
	}
	if mostUsed[0].PolicyID != "popular-policy" {
		t.Error("Expected popular-policy to be most used")
	}
}

func TestPolicyProfiler_GetTopErrorRate(t *testing.T) {
	profiler := NewPolicyProfiler(nil)

	// Policy with high error rate
	for i := 0; i < 10; i++ {
		profiler.RecordEvaluation(&EvaluationSample{
			PolicyID:    "error-policy",
			PolicyType:  PolicyTypeCEL,
			Duration:    10 * time.Millisecond,
			Allowed:     false,
			Error:       "error",
			EvaluatedAt: time.Now(),
		})
	}

	// Policy with no errors
	profiler.RecordEvaluation(&EvaluationSample{
		PolicyID:    "good-policy",
		PolicyType:  PolicyTypeCEL,
		Duration:    10 * time.Millisecond,
		Allowed:     true,
		EvaluatedAt: time.Now(),
	})

	errorRate := profiler.GetTopErrorRate(10)

	if len(errorRate) != 1 {
		t.Fatalf("Expected 1 result (only error policy), got %d", len(errorRate))
	}
	if errorRate[0].PolicyID != "error-policy" {
		t.Error("Expected error-policy")
	}
}

func TestPolicyProfiler_Reset(t *testing.T) {
	profiler := NewPolicyProfiler(nil)

	profiler.RecordEvaluation(&EvaluationSample{
		PolicyID:    "test-policy",
		PolicyType:  PolicyTypeCEL,
		Duration:    10 * time.Millisecond,
		Allowed:     true,
		EvaluatedAt: time.Now(),
	})

	if len(profiler.GetAllStats()) != 1 {
		t.Fatal("Expected 1 policy before reset")
	}

	profiler.Reset()

	if len(profiler.GetAllStats()) != 0 {
		t.Error("Expected 0 policies after reset")
	}
}

func TestPolicyProfiler_ResetPolicy(t *testing.T) {
	profiler := NewPolicyProfiler(nil)

	profiler.RecordEvaluation(&EvaluationSample{
		PolicyID:    "policy-a",
		PolicyType:  PolicyTypeCEL,
		Duration:    10 * time.Millisecond,
		Allowed:     true,
		EvaluatedAt: time.Now(),
	})

	profiler.RecordEvaluation(&EvaluationSample{
		PolicyID:    "policy-b",
		PolicyType:  PolicyTypeCEL,
		Duration:    10 * time.Millisecond,
		Allowed:     true,
		EvaluatedAt: time.Now(),
	})

	profiler.ResetPolicy("policy-a")

	stats := profiler.GetAllStats()
	if len(stats) != 1 {
		t.Errorf("Expected 1 policy after reset, got %d", len(stats))
	}
	if stats[0].PolicyID != "policy-b" {
		t.Error("Expected policy-b to remain")
	}
}

func TestPolicyProfiler_GenerateReport(t *testing.T) {
	config := DefaultProfilingConfig()
	config.SlowThreshold = 50 * time.Millisecond
	profiler := NewPolicyProfiler(config)

	// Fast evaluations
	for i := 0; i < 8; i++ {
		profiler.RecordEvaluation(&EvaluationSample{
			PolicyID:    "fast-policy",
			PolicyType:  PolicyTypeCEL,
			Duration:    10 * time.Millisecond,
			Allowed:     true,
			EvaluatedAt: time.Now(),
		})
	}

	// Slow evaluations
	for i := 0; i < 2; i++ {
		profiler.RecordEvaluation(&EvaluationSample{
			PolicyID:    "slow-policy",
			PolicyType:  PolicyTypeOPA,
			Duration:    100 * time.Millisecond,
			Allowed:     true,
			EvaluatedAt: time.Now(),
		})
	}

	report := profiler.GenerateReport(5)

	if report.TotalPolicies != 2 {
		t.Errorf("Expected 2 policies, got %d", report.TotalPolicies)
	}
	if report.TotalEvaluations != 10 {
		t.Errorf("Expected 10 evaluations, got %d", report.TotalEvaluations)
	}
	if report.SlowEvaluations != 2 {
		t.Errorf("Expected 2 slow evaluations, got %d", report.SlowEvaluations)
	}
	if report.SlowPercentage != 20.0 {
		t.Errorf("Expected 20%% slow, got %.1f%%", report.SlowPercentage)
	}
	if len(report.ByType) != 2 {
		t.Errorf("Expected 2 types, got %d", len(report.ByType))
	}
	if report.ByType[PolicyTypeCEL] != 8 {
		t.Errorf("Expected 8 CEL evaluations, got %d", report.ByType[PolicyTypeCEL])
	}
	if report.ByType[PolicyTypeOPA] != 2 {
		t.Errorf("Expected 2 OPA evaluations, got %d", report.ByType[PolicyTypeOPA])
	}
}

func TestPolicyProfiler_GenerateReport_Recommendations(t *testing.T) {
	config := DefaultProfilingConfig()
	config.SlowThreshold = 10 * time.Millisecond
	profiler := NewPolicyProfiler(config)

	// Create many slow evaluations
	for i := 0; i < 20; i++ {
		profiler.RecordEvaluation(&EvaluationSample{
			PolicyID:    "slow-policy",
			PolicyType:  PolicyTypeCEL,
			Duration:    50 * time.Millisecond,
			Allowed:     true,
			EvaluatedAt: time.Now(),
		})
	}

	report := profiler.GenerateReport(5)

	if len(report.Recommendations) == 0 {
		t.Error("Expected recommendations for slow policies")
	}
}

func TestPolicyProfiler_Sampling(t *testing.T) {
	config := DefaultProfilingConfig()
	config.SampleRate = 0.5 // 50% sampling
	profiler := NewPolicyProfiler(config)

	// Record many evaluations
	for i := 0; i < 100; i++ {
		profiler.RecordEvaluation(&EvaluationSample{
			PolicyID:    "test-policy",
			PolicyType:  PolicyTypeCEL,
			Duration:    10 * time.Millisecond,
			Allowed:     true,
			EvaluatedAt: time.Now(),
		})
	}

	samples := profiler.GetSamples("test-policy")
	stats := profiler.GetStats("test-policy")

	// Samples should be ~50% of total
	if len(samples) == 100 {
		t.Error("Expected some samples to be dropped due to sampling")
	}

	// But stats should still count all 100
	if stats.EvaluationCount != 100 {
		t.Errorf("Expected 100 evaluations in stats, got %d", stats.EvaluationCount)
	}
}

func TestProfiledPolicyEngine(t *testing.T) {
	registry := NewRegistry()

	// Register a CEL policy
	policy := &Policy{
		ID:      "test-policy",
		Name:    "Test Policy",
		Type:    PolicyTypeCEL,
		Policy:  "action == 'read'",
		Enabled: true,
	}
	registry.RegisterPolicy(policy)

	engine := NewPolicyEngine(registry)
	profiled := NewProfiledPolicyEngine(engine, registry, nil)

	input := &EvaluationInput{
		Action:    "read",
		User:      "test-user",
		Timestamp: time.Now(),
	}

	result, err := profiled.Evaluate(context.Background(), "test-policy", input)
	if err != nil {
		t.Fatalf("Evaluation failed: %v", err)
	}

	if !result.Allowed {
		t.Error("Expected evaluation to be allowed")
	}

	// Check profiler recorded the evaluation
	stats := profiled.GetProfiler().GetStats("test-policy")
	if stats == nil {
		t.Fatal("Expected stats to be recorded")
	}
	if stats.EvaluationCount != 1 {
		t.Errorf("Expected 1 evaluation, got %d", stats.EvaluationCount)
	}
}

func TestProfiledPolicyEngine_GetEngine(t *testing.T) {
	registry := NewRegistry()
	engine := NewPolicyEngine(registry)
	profiled := NewProfiledPolicyEngine(engine, registry, nil)

	if profiled.GetEngine() != engine {
		t.Error("Expected GetEngine to return underlying engine")
	}
}

func TestEvaluationTimer(t *testing.T) {
	profiler := NewPolicyProfiler(nil)

	timer := profiler.StartTimer("test-policy", PolicyTypeCEL)

	start := time.Now()
	if err := helpers.WaitForTimeout(100*time.Millisecond, 5*time.Millisecond, func() (bool, error) {
		return time.Since(start) >= 5*time.Millisecond, nil
	}); err != nil {
		t.Fatalf("expected timer to advance: %v", err)
	}

	duration := timer.Stop(true, 0, nil)

	if duration < 5*time.Millisecond {
		t.Error("Expected duration to be at least 5ms")
	}

	stats := profiler.GetStats("test-policy")
	if stats == nil {
		t.Fatal("Expected stats to be recorded")
	}
	if stats.EvaluationCount != 1 {
		t.Errorf("Expected 1 evaluation, got %d", stats.EvaluationCount)
	}
}

func TestEvaluationTimer_WithError(t *testing.T) {
	profiler := NewPolicyProfiler(nil)

	timer := profiler.StartTimer("test-policy", PolicyTypeCEL)
	timer.Stop(false, 2, nil)

	stats := profiler.GetStats("test-policy")
	if stats.FailureCount != 1 {
		t.Errorf("Expected 1 failure, got %d", stats.FailureCount)
	}
	if stats.TotalViolations != 2 {
		t.Errorf("Expected 2 violations, got %d", stats.TotalViolations)
	}
}

func TestPolicyProfiler_Percentiles(t *testing.T) {
	profiler := NewPolicyProfiler(nil)

	// Record samples with known durations
	durations := []time.Duration{
		10 * time.Millisecond,
		20 * time.Millisecond,
		30 * time.Millisecond,
		40 * time.Millisecond,
		50 * time.Millisecond,
		60 * time.Millisecond,
		70 * time.Millisecond,
		80 * time.Millisecond,
		90 * time.Millisecond,
		100 * time.Millisecond,
	}

	for _, d := range durations {
		profiler.RecordEvaluation(&EvaluationSample{
			PolicyID:    "test-policy",
			PolicyType:  PolicyTypeCEL,
			Duration:    d,
			Allowed:     true,
			EvaluatedAt: time.Now(),
		})
	}

	stats := profiler.GetStats("test-policy")

	// P50 should be around 50-60ms
	if stats.P50Duration < 40*time.Millisecond || stats.P50Duration > 70*time.Millisecond {
		t.Errorf("Expected P50 around 50ms, got %v", stats.P50Duration)
	}

	// P90 should be around 90ms
	if stats.P90Duration < 80*time.Millisecond || stats.P90Duration > 100*time.Millisecond {
		t.Errorf("Expected P90 around 90ms, got %v", stats.P90Duration)
	}
}
