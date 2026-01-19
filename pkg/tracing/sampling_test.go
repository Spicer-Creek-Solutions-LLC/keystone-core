package tracing

import (
	"context"
	"testing"
	"time"

	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/trace"
)

func TestDefaultAdaptiveSamplingConfig(t *testing.T) {
	config := DefaultAdaptiveSamplingConfig()

	if config.BaseRate != 0.1 {
		t.Errorf("Expected BaseRate 0.1, got %f", config.BaseRate)
	}
	if config.MinRate != 0.01 {
		t.Errorf("Expected MinRate 0.01, got %f", config.MinRate)
	}
	if config.MaxRate != 1.0 {
		t.Errorf("Expected MaxRate 1.0, got %f", config.MaxRate)
	}
	if !config.AlwaysSampleOnError {
		t.Error("Expected AlwaysSampleOnError to be true")
	}
}

func TestNewAdaptiveSampler(t *testing.T) {
	sampler := NewAdaptiveSampler(nil)
	defer sampler.Stop()

	if sampler == nil {
		t.Fatal("Expected sampler to be created")
	}

	rate := sampler.GetCurrentRate()
	if rate != 0.1 {
		t.Errorf("Expected initial rate 0.1, got %f", rate)
	}
}

func TestAdaptiveSampler_ConfigValidation(t *testing.T) {
	config := &AdaptiveSamplingConfig{
		BaseRate:         -1,  // Invalid, should be normalized to 0
		MinRate:          2.0, // Invalid, should be normalized
		MaxRate:          0.5,
		AdaptationWindow: 0, // Invalid, should default
	}

	sampler := NewAdaptiveSampler(config)
	defer sampler.Stop()

	// Rate should be normalized
	rate := sampler.GetCurrentRate()
	if rate < 0 || rate > 1 {
		t.Errorf("Rate should be between 0 and 1, got %f", rate)
	}
}

func TestAdaptiveSampler_ShouldSample_Basic(t *testing.T) {
	config := &AdaptiveSamplingConfig{
		BaseRate:         1.0, // Always sample for predictable testing
		MinRate:          0,
		MaxRate:          1.0,
		AdaptationWindow: time.Hour, // Long window for testing
	}

	sampler := NewAdaptiveSampler(config)
	defer sampler.Stop()

	params := sdktrace.SamplingParameters{
		ParentContext: context.Background(),
		TraceID:       trace.TraceID{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16},
		Name:          "test-span",
	}

	result := sampler.ShouldSample(params)
	if result.Decision != sdktrace.RecordAndSample {
		t.Error("Expected sample with rate 1.0")
	}
}

func TestAdaptiveSampler_ShouldSample_NeverSampleRule(t *testing.T) {
	config := DefaultAdaptiveSamplingConfig()
	config.PerSpanRules = map[string]*SpanSamplingRule{
		"never-sample-span": {NeverSample: true},
	}

	sampler := NewAdaptiveSampler(config)
	defer sampler.Stop()

	params := sdktrace.SamplingParameters{
		ParentContext: context.Background(),
		TraceID:       trace.TraceID{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16},
		Name:          "never-sample-span",
	}

	result := sampler.ShouldSample(params)
	if result.Decision != sdktrace.Drop {
		t.Error("Expected drop for never-sample span")
	}
}

func TestAdaptiveSampler_ShouldSample_AlwaysSampleRule(t *testing.T) {
	config := &AdaptiveSamplingConfig{
		BaseRate:         0, // Never sample by default
		MinRate:          0,
		MaxRate:          1.0,
		AdaptationWindow: time.Hour,
		PerSpanRules: map[string]*SpanSamplingRule{
			"always-sample-span": {AlwaysSample: true},
		},
	}

	sampler := NewAdaptiveSampler(config)
	defer sampler.Stop()

	params := sdktrace.SamplingParameters{
		ParentContext: context.Background(),
		TraceID:       trace.TraceID{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16},
		Name:          "always-sample-span",
	}

	result := sampler.ShouldSample(params)
	if result.Decision != sdktrace.RecordAndSample {
		t.Error("Expected sample for always-sample span")
	}
}

func TestAdaptiveSampler_ShouldSample_PerSpanRate(t *testing.T) {
	config := &AdaptiveSamplingConfig{
		BaseRate:         0, // Never sample by default
		MinRate:          0,
		MaxRate:          1.0,
		AdaptationWindow: time.Hour,
		PerSpanRules: map[string]*SpanSamplingRule{
			"custom-rate-span": {Rate: 1.0}, // Always sample this one
		},
	}

	sampler := NewAdaptiveSampler(config)
	defer sampler.Stop()

	params := sdktrace.SamplingParameters{
		ParentContext: context.Background(),
		TraceID:       trace.TraceID{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16},
		Name:          "custom-rate-span",
	}

	result := sampler.ShouldSample(params)
	if result.Decision != sdktrace.RecordAndSample {
		t.Error("Expected sample with per-span rate 1.0")
	}
}

func TestAdaptiveSampler_RecordSpanResult(t *testing.T) {
	config := DefaultAdaptiveSamplingConfig()
	config.AdaptationWindow = time.Hour // Long window for testing

	sampler := NewAdaptiveSampler(config)
	defer sampler.Stop()

	// Record some spans
	for i := 0; i < 100; i++ {
		isError := i%10 == 0 // 10% errors
		sampler.RecordSpanResult("test-span", isError)
	}

	stats := sampler.GetStats()

	if stats.TotalSpans != 100 {
		t.Errorf("Expected 100 total spans, got %d", stats.TotalSpans)
	}
	if stats.ErrorSpans != 10 {
		t.Errorf("Expected 10 error spans, got %d", stats.ErrorSpans)
	}
	if stats.ErrorRate != 0.1 {
		t.Errorf("Expected error rate 0.1, got %f", stats.ErrorRate)
	}
}

func TestAdaptiveSampler_GetErrorRate(t *testing.T) {
	config := DefaultAdaptiveSamplingConfig()
	config.AdaptationWindow = time.Hour

	sampler := NewAdaptiveSampler(config)
	defer sampler.Stop()

	// Initially 0
	if sampler.GetErrorRate() != 0 {
		t.Error("Expected initial error rate 0")
	}

	// Record spans
	sampler.RecordSpanResult("span1", false)
	sampler.RecordSpanResult("span2", true)
	sampler.RecordSpanResult("span3", false)
	sampler.RecordSpanResult("span4", false)

	// 1/4 = 0.25
	if sampler.GetErrorRate() != 0.25 {
		t.Errorf("Expected error rate 0.25, got %f", sampler.GetErrorRate())
	}
}

func TestAdaptiveSampler_SetRate(t *testing.T) {
	config := DefaultAdaptiveSamplingConfig()
	sampler := NewAdaptiveSampler(config)
	defer sampler.Stop()

	sampler.SetRate(0.5)
	if sampler.GetCurrentRate() != 0.5 {
		t.Errorf("Expected rate 0.5, got %f", sampler.GetCurrentRate())
	}

	// Should be clamped to max
	sampler.SetRate(2.0)
	if sampler.GetCurrentRate() != config.MaxRate {
		t.Errorf("Expected rate to be clamped to max %f, got %f", config.MaxRate, sampler.GetCurrentRate())
	}

	// Should be clamped to min
	sampler.SetRate(-1)
	if sampler.GetCurrentRate() != config.MinRate {
		t.Errorf("Expected rate to be clamped to min %f, got %f", config.MinRate, sampler.GetCurrentRate())
	}
}

func TestAdaptiveSampler_AddRemoveSpanRule(t *testing.T) {
	sampler := NewAdaptiveSampler(nil)
	defer sampler.Stop()

	// Add rule
	sampler.AddSpanRule("test-span", &SpanSamplingRule{AlwaysSample: true})

	params := sdktrace.SamplingParameters{
		ParentContext: context.Background(),
		TraceID:       trace.TraceID{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16},
		Name:          "test-span",
	}

	result := sampler.ShouldSample(params)
	if result.Decision != sdktrace.RecordAndSample {
		t.Error("Expected sample after adding rule")
	}

	// Remove rule
	sampler.RemoveSpanRule("test-span")

	// Now it should use default rate
	// (may or may not sample depending on rate)
}

func TestAdaptiveSampler_Reset(t *testing.T) {
	config := DefaultAdaptiveSamplingConfig()
	config.AdaptationWindow = time.Hour

	sampler := NewAdaptiveSampler(config)
	defer sampler.Stop()

	// Record some data
	sampler.RecordSpanResult("span1", true)
	sampler.RecordSpanResult("span2", false)

	stats := sampler.GetStats()
	if stats.TotalSpans != 2 {
		t.Error("Expected 2 spans before reset")
	}

	sampler.Reset()

	stats = sampler.GetStats()
	if stats.TotalSpans != 0 {
		t.Error("Expected 0 spans after reset")
	}
}

func TestAdaptiveSampler_DebugSpans(t *testing.T) {
	config := DefaultAdaptiveSamplingConfig()
	config.BaseRate = 1.0 // Always sample non-debug
	config.EnableDebugSpans = false

	sampler := NewAdaptiveSampler(config)
	defer sampler.Stop()

	debugSpans := []string{
		"internal/process",
		"debug/trace",
		"healthcheck",
		"readiness",
		"liveness",
		"metrics",
	}

	for _, spanName := range debugSpans {
		params := sdktrace.SamplingParameters{
			ParentContext: context.Background(),
			TraceID:       trace.TraceID{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16},
			Name:          spanName,
		}

		result := sampler.ShouldSample(params)
		if result.Decision != sdktrace.Drop {
			t.Errorf("Expected drop for debug span %s", spanName)
		}
	}

	// Enable debug spans
	config.EnableDebugSpans = true
	sampler2 := NewAdaptiveSampler(config)
	defer sampler2.Stop()

	params := sdktrace.SamplingParameters{
		ParentContext: context.Background(),
		TraceID:       trace.TraceID{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16},
		Name:          "healthcheck",
	}

	result := sampler2.ShouldSample(params)
	if result.Decision != sdktrace.RecordAndSample {
		t.Error("Expected sample for debug span when enabled")
	}
}

func TestAdaptiveSampler_Description(t *testing.T) {
	sampler := NewAdaptiveSampler(nil)
	defer sampler.Stop()

	if sampler.Description() != "AdaptiveSampler" {
		t.Errorf("Expected description 'AdaptiveSampler', got '%s'", sampler.Description())
	}
}

func TestAdaptiveSampler_PerSpanStats(t *testing.T) {
	config := DefaultAdaptiveSamplingConfig()
	config.AdaptationWindow = time.Hour

	sampler := NewAdaptiveSampler(config)
	defer sampler.Stop()

	// Record spans for different operations
	sampler.RecordSpanResult("api/users", false)
	sampler.RecordSpanResult("api/users", true)
	sampler.RecordSpanResult("api/orders", false)
	sampler.RecordSpanResult("api/orders", false)
	sampler.RecordSpanResult("api/orders", false)

	stats := sampler.GetStats()

	if len(stats.PerSpanStats) != 2 {
		t.Errorf("Expected 2 span stats, got %d", len(stats.PerSpanStats))
	}

	userStats := stats.PerSpanStats["api/users"]
	if userStats == nil {
		t.Fatal("Expected stats for api/users")
	}
	if userStats.TotalSpans != 2 {
		t.Errorf("Expected 2 total spans for api/users, got %d", userStats.TotalSpans)
	}
	if userStats.ErrorSpans != 1 {
		t.Errorf("Expected 1 error span for api/users, got %d", userStats.ErrorSpans)
	}
	if userStats.ErrorRate != 0.5 {
		t.Errorf("Expected 0.5 error rate for api/users, got %f", userStats.ErrorRate)
	}
}

func TestNewRateLimitingSampler(t *testing.T) {
	sampler := NewRateLimitingSampler(10)

	if sampler == nil {
		t.Fatal("Expected sampler to be created")
	}
	if sampler.Description() != "RateLimitingSampler" {
		t.Errorf("Expected description 'RateLimitingSampler', got '%s'", sampler.Description())
	}
}

func TestRateLimitingSampler_WithinLimit(t *testing.T) {
	sampler := NewRateLimitingSampler(100)

	params := sdktrace.SamplingParameters{
		ParentContext: context.Background(),
		TraceID:       trace.TraceID{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16},
		Name:          "test-span",
	}

	// Should sample within limit
	for i := 0; i < 50; i++ {
		result := sampler.ShouldSample(params)
		if result.Decision != sdktrace.RecordAndSample {
			t.Errorf("Expected sample within limit at iteration %d", i)
		}
	}
}

func TestRateLimitingSampler_ExceedsLimit(t *testing.T) {
	sampler := NewRateLimitingSampler(5)

	params := sdktrace.SamplingParameters{
		ParentContext: context.Background(),
		TraceID:       trace.TraceID{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16},
		Name:          "test-span",
	}

	// Sample up to limit
	for i := 0; i < 5; i++ {
		result := sampler.ShouldSample(params)
		if result.Decision != sdktrace.RecordAndSample {
			t.Errorf("Expected sample within limit at iteration %d", i)
		}
	}

	// Should drop after limit
	result := sampler.ShouldSample(params)
	if result.Decision != sdktrace.Drop {
		t.Error("Expected drop after exceeding limit")
	}
}

func TestRateLimitingSampler_DefaultRate(t *testing.T) {
	// Invalid rate should default to 100
	sampler := NewRateLimitingSampler(0)
	if sampler.maxPerSecond != 100 {
		t.Errorf("Expected default rate 100, got %d", sampler.maxPerSecond)
	}

	sampler2 := NewRateLimitingSampler(-10)
	if sampler2.maxPerSecond != 100 {
		t.Errorf("Expected default rate 100 for negative, got %d", sampler2.maxPerSecond)
	}
}

func TestNewCompositeSampler(t *testing.T) {
	sampler1 := sdktrace.AlwaysSample()
	sampler2 := sdktrace.NeverSample()

	composite := NewCompositeSampler(CompositeModeAny, sampler1, sampler2)

	if composite == nil {
		t.Fatal("Expected composite sampler to be created")
	}
	if composite.Description() != "CompositeSampler" {
		t.Errorf("Expected description 'CompositeSampler', got '%s'", composite.Description())
	}
}

func TestCompositeSampler_ModeAny(t *testing.T) {
	// One always, one never - should sample
	sampler1 := sdktrace.AlwaysSample()
	sampler2 := sdktrace.NeverSample()

	composite := NewCompositeSampler(CompositeModeAny, sampler1, sampler2)

	params := sdktrace.SamplingParameters{
		ParentContext: context.Background(),
		TraceID:       trace.TraceID{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16},
		Name:          "test-span",
	}

	result := composite.ShouldSample(params)
	if result.Decision != sdktrace.RecordAndSample {
		t.Error("Expected sample with ModeAny when one sampler samples")
	}

	// Both never - should drop
	composite2 := NewCompositeSampler(CompositeModeAny, sdktrace.NeverSample(), sdktrace.NeverSample())
	result = composite2.ShouldSample(params)
	if result.Decision != sdktrace.Drop {
		t.Error("Expected drop with ModeAny when no sampler samples")
	}
}

func TestCompositeSampler_ModeAll(t *testing.T) {
	// One always, one never - should drop
	sampler1 := sdktrace.AlwaysSample()
	sampler2 := sdktrace.NeverSample()

	composite := NewCompositeSampler(CompositeModeAll, sampler1, sampler2)

	params := sdktrace.SamplingParameters{
		ParentContext: context.Background(),
		TraceID:       trace.TraceID{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16},
		Name:          "test-span",
	}

	result := composite.ShouldSample(params)
	if result.Decision != sdktrace.Drop {
		t.Error("Expected drop with ModeAll when one sampler drops")
	}

	// Both always - should sample
	composite2 := NewCompositeSampler(CompositeModeAll, sdktrace.AlwaysSample(), sdktrace.AlwaysSample())
	result = composite2.ShouldSample(params)
	if result.Decision != sdktrace.RecordAndSample {
		t.Error("Expected sample with ModeAll when all samplers sample")
	}
}

func TestCompositeSampler_ModePriority(t *testing.T) {
	// First sampler's decision should be used
	sampler1 := sdktrace.NeverSample()
	sampler2 := sdktrace.AlwaysSample()

	composite := NewCompositeSampler(CompositeModePriority, sampler1, sampler2)

	params := sdktrace.SamplingParameters{
		ParentContext: context.Background(),
		TraceID:       trace.TraceID{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16},
		Name:          "test-span",
	}

	result := composite.ShouldSample(params)
	if result.Decision != sdktrace.Drop {
		t.Error("Expected drop with ModePriority using first sampler")
	}
}

func TestCompositeSampler_Empty(t *testing.T) {
	composite := NewCompositeSampler(CompositeModeAny)

	params := sdktrace.SamplingParameters{
		ParentContext: context.Background(),
		TraceID:       trace.TraceID{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16},
		Name:          "test-span",
	}

	result := composite.ShouldSample(params)
	if result.Decision != sdktrace.Drop {
		t.Error("Expected drop with empty composite sampler")
	}
}

func TestNewErrorAwareSampler(t *testing.T) {
	sampler := NewErrorAwareSampler(nil)
	defer sampler.Stop()

	if sampler == nil {
		t.Fatal("Expected sampler to be created")
	}
	if sampler.Description() != "ErrorAwareSampler" {
		t.Errorf("Expected description 'ErrorAwareSampler', got '%s'", sampler.Description())
	}
}

func TestErrorAwareSampler_RecordSpan(t *testing.T) {
	sampler := NewErrorAwareSampler(nil)
	defer sampler.Stop()

	sampler.RecordSpan("test-span", false)
	sampler.RecordSpan("test-span", true)
	sampler.RecordSpan("other-span", false)

	stats := sampler.GetStats()

	if stats.TotalSpans != 3 {
		t.Errorf("Expected 3 total spans, got %d", stats.TotalSpans)
	}
	if stats.ErrorSpans != 1 {
		t.Errorf("Expected 1 error span, got %d", stats.ErrorSpans)
	}
}

func TestIsDebugSpan(t *testing.T) {
	tests := []struct {
		name     string
		expected bool
	}{
		{"internal/process", true},
		{"debug/trace", true},
		{"healthcheck", true},
		{"readiness", true},
		{"liveness", true},
		{"metrics", true},
		{"api/users", false},
		{"service/orders", false},
		{"normal-span", false},
	}

	for _, tt := range tests {
		result := isDebugSpan(tt.name)
		if result != tt.expected {
			t.Errorf("isDebugSpan(%s) = %v, want %v", tt.name, result, tt.expected)
		}
	}
}
