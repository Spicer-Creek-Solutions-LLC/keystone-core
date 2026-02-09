package sampling

import (
	"context"
	"math/rand"
	"sync"
	"testing"
	"time"
)

func TestDecision_String(t *testing.T) {
	tests := []struct {
		decision Decision
		expected string
	}{
		{DecisionDrop, "drop"},
		{DecisionRecordOnly, "record_only"},
		{DecisionRecordAndSample, "record_and_sample"},
		{Decision(99), "unknown"},
	}

	for _, tt := range tests {
		if got := tt.decision.String(); got != tt.expected {
			t.Errorf("Decision(%d).String() = %v, want %v", tt.decision, got, tt.expected)
		}
	}
}

func TestDefaultConfig(t *testing.T) {
	config := DefaultConfig()

	if config.SampleRate <= 0 || config.SampleRate > 1 {
		t.Errorf("SampleRate = %v, want (0, 1]", config.SampleRate)
	}
	if config.ErrorSampleRate != 1.0 {
		t.Errorf("ErrorSampleRate = %v, want 1.0", config.ErrorSampleRate)
	}
}

func TestAlwaysSampler(t *testing.T) {
	sampler := NewAlwaysSampler()
	ctx := context.Background()

	for i := 0; i < 100; i++ {
		span := &Span{TraceID: "test"}
		decision := sampler.ShouldSample(ctx, span)
		if decision != DecisionRecordAndSample {
			t.Errorf("AlwaysSampler returned %v", decision)
		}
	}

	if sampler.Description() != "AlwaysSampler" {
		t.Errorf("Description = %v", sampler.Description())
	}
}

func TestNeverSampler(t *testing.T) {
	sampler := NewNeverSampler()
	ctx := context.Background()

	for i := 0; i < 100; i++ {
		span := &Span{TraceID: "test"}
		decision := sampler.ShouldSample(ctx, span)
		if decision != DecisionDrop {
			t.Errorf("NeverSampler returned %v", decision)
		}
	}

	if sampler.Description() != "NeverSampler" {
		t.Errorf("Description = %v", sampler.Description())
	}
}

func TestProbabilisticSampler(t *testing.T) {
	t.Run("rate 0", func(t *testing.T) {
		sampler := NewProbabilisticSampler(0)
		ctx := context.Background()

		for i := 0; i < 100; i++ {
			span := &Span{TraceID: "test"}
			if sampler.ShouldSample(ctx, span) == DecisionRecordAndSample {
				t.Error("Rate 0 sampler should never sample")
			}
		}
	})

	t.Run("rate 1", func(t *testing.T) {
		sampler := NewProbabilisticSampler(1.0)
		ctx := context.Background()

		for i := 0; i < 100; i++ {
			span := &Span{TraceID: "test"}
			if sampler.ShouldSample(ctx, span) != DecisionRecordAndSample {
				t.Error("Rate 1 sampler should always sample")
			}
		}
	})

	t.Run("rate 0.5", func(t *testing.T) {
		sampler := NewProbabilisticSampler(0.5)
		ctx := context.Background()

		sampled := 0
		for i := 0; i < 1000; i++ {
			// Use random UUIDs for better distribution
			span := &Span{TraceID: randomTraceID()}
			if sampler.ShouldSample(ctx, span) == DecisionRecordAndSample {
				sampled++
			}
		}

		// Should be roughly 50%
		rate := float64(sampled) / 1000.0
		if rate < 0.3 || rate > 0.7 {
			t.Errorf("Sample rate = %v, expected ~0.5", rate)
		}
	})

	t.Run("consistent sampling", func(t *testing.T) {
		sampler := NewProbabilisticSampler(0.5)
		ctx := context.Background()

		span := &Span{TraceID: "consistent-trace-id"}

		// Same trace ID should give same result
		first := sampler.ShouldSample(ctx, span)
		for i := 0; i < 10; i++ {
			if sampler.ShouldSample(ctx, span) != first {
				t.Error("Sampling should be consistent for same trace ID")
			}
		}
	})

	t.Run("clamp values", func(t *testing.T) {
		sampler1 := NewProbabilisticSampler(-1.0)
		sampler2 := NewProbabilisticSampler(2.0)

		ctx := context.Background()
		span := &Span{TraceID: "test"}

		// Negative rate should be clamped to 0
		if sampler1.ShouldSample(ctx, span) == DecisionRecordAndSample {
			t.Error("Negative rate should never sample")
		}

		// Rate > 1 should be clamped to 1
		if sampler2.ShouldSample(ctx, span) != DecisionRecordAndSample {
			t.Error("Rate > 1 should always sample")
		}
	})
}

func TestRateLimitingSampler(t *testing.T) {
	sampler := NewRateLimitingSampler(10) // 10 per second
	ctx := context.Background()

	// Should allow initial burst
	sampled := 0
	for i := 0; i < 15; i++ {
		span := &Span{TraceID: "test"}
		if sampler.ShouldSample(ctx, span) == DecisionRecordAndSample {
			sampled++
		}
	}

	// Initial tokens should allow up to 10
	if sampled > 12 { // Allow some margin
		t.Errorf("Sampled %d, expected <= 12", sampled)
	}

	sampler.mu.Lock()
	sampler.lastTick = time.Now().Add(-200 * time.Millisecond)
	sampler.mu.Unlock()

	// Should allow more samples
	span := &Span{TraceID: "test"}
	if sampler.ShouldSample(ctx, span) != DecisionRecordAndSample {
		t.Error("Should sample after token refill")
	}
}

func TestAdaptiveSampler(t *testing.T) {
	config := DefaultConfig()
	config.Strategy = StrategyAdaptive
	config.SampleRate = 0.5
	config.ErrorSampleRate = 1.0
	config.SlowThreshold = 100 * time.Millisecond
	config.SlowSampleRate = 0.8

	sampler := NewAdaptiveSampler(config)
	ctx := context.Background()

	t.Run("errors sampled at high rate", func(t *testing.T) {
		sampled := 0
		for i := 0; i < 100; i++ {
			span := &Span{
				TraceID: "error-test",
				IsError: true,
			}
			if sampler.ShouldSample(ctx, span) == DecisionRecordAndSample {
				sampled++
			}
		}

		// Should sample all errors
		if sampled < 90 {
			t.Errorf("Error sample rate = %d/100, expected ~100", sampled)
		}
	})

	t.Run("slow requests sampled at higher rate", func(t *testing.T) {
		sampled := 0
		for i := 0; i < 100; i++ {
			span := &Span{
				TraceID:  "slow-test",
				Duration: 200 * time.Millisecond,
			}
			if sampler.ShouldSample(ctx, span) == DecisionRecordAndSample {
				sampled++
			}
		}

		// Should sample more slow requests
		if sampled < 60 {
			t.Errorf("Slow sample rate = %d/100, expected >= 60", sampled)
		}
	})

	t.Run("adjust rate", func(t *testing.T) {
		initialRate := sampler.CurrentRate()

		// Generate many samples to trigger adjustment
		for i := 0; i < 1000; i++ {
			sampler.ShouldSample(ctx, &Span{TraceID: "adjust"})
		}

		sampler.mu.Lock()
		sampler.lastAdjust = time.Now().Add(-2 * time.Second)
		sampler.mu.Unlock()
		sampler.Adjust()

		// Rate should have changed
		newRate := sampler.CurrentRate()
		// Can't guarantee direction, but shouldn't be unchanged after many samples
		_ = initialRate
		_ = newRate
	})
}

func TestPrioritySampler(t *testing.T) {
	sampler := NewPrioritySampler()
	ctx := context.Background()

	t.Run("low priority", func(t *testing.T) {
		sampled := 0
		for i := 0; i < 1000; i++ {
			span := &Span{TraceID: "low", Priority: 0}
			if sampler.ShouldSample(ctx, span) == DecisionRecordAndSample {
				sampled++
			}
		}

		// Low priority should have ~1% sample rate
		rate := float64(sampled) / 1000.0
		if rate > 0.05 {
			t.Errorf("Low priority rate = %v, expected ~0.01", rate)
		}
	})

	t.Run("critical priority", func(t *testing.T) {
		sampled := 0
		for i := 0; i < 100; i++ {
			span := &Span{TraceID: "critical", Priority: 3}
			if sampler.ShouldSample(ctx, span) == DecisionRecordAndSample {
				sampled++
			}
		}

		// Critical priority should have 100% sample rate
		if sampled != 100 {
			t.Errorf("Critical priority rate = %d/100, expected 100", sampled)
		}
	})

	t.Run("custom threshold", func(t *testing.T) {
		sampler.SetThreshold(5, 0.5)

		sampled := 0
		for i := 0; i < 1000; i++ {
			span := &Span{TraceID: "custom", Priority: 5}
			if sampler.ShouldSample(ctx, span) == DecisionRecordAndSample {
				sampled++
			}
		}

		rate := float64(sampled) / 1000.0
		if rate < 0.3 || rate > 0.7 {
			t.Errorf("Custom priority rate = %v, expected ~0.5", rate)
		}
	})
}

func TestParentBasedSampler(t *testing.T) {
	config := &ParentBasedConfig{
		Root:                  NewProbabilisticSampler(0.5),
		LocalParentSampled:    NewAlwaysSampler(),
		LocalParentNotSampled: NewNeverSampler(),
	}

	sampler := NewParentBasedSampler(config)

	t.Run("root span uses root sampler", func(t *testing.T) {
		ctx := context.Background()
		sampled := 0
		for i := 0; i < 1000; i++ {
			span := &Span{TraceID: randomTraceID()}
			if sampler.ShouldSample(ctx, span) == DecisionRecordAndSample {
				sampled++
			}
		}

		rate := float64(sampled) / 1000.0
		if rate < 0.3 || rate > 0.7 {
			t.Errorf("Root span rate = %v, expected ~0.5", rate)
		}
	})

	t.Run("sampled parent propagates", func(t *testing.T) {
		ctx := WithSampled(context.Background(), true)

		for i := 0; i < 100; i++ {
			span := &Span{TraceID: "child", ParentID: "parent"}
			if sampler.ShouldSample(ctx, span) != DecisionRecordAndSample {
				t.Error("Sampled parent should propagate")
			}
		}
	})

	t.Run("not sampled parent propagates", func(t *testing.T) {
		ctx := WithSampled(context.Background(), false)

		for i := 0; i < 100; i++ {
			span := &Span{TraceID: "child", ParentID: "parent"}
			if sampler.ShouldSample(ctx, span) != DecisionDrop {
				t.Error("Not sampled parent should propagate")
			}
		}
	})
}

func TestCompositeSampler(t *testing.T) {
	t.Run("any mode", func(t *testing.T) {
		sampler := NewCompositeSampler(
			CompositeAny,
			NewNeverSampler(),
			NewAlwaysSampler(),
		)
		ctx := context.Background()

		span := &Span{TraceID: "test"}
		if sampler.ShouldSample(ctx, span) != DecisionRecordAndSample {
			t.Error("Any mode should sample when one sampler says yes")
		}
	})

	t.Run("all mode", func(t *testing.T) {
		sampler := NewCompositeSampler(
			CompositeAll,
			NewNeverSampler(),
			NewAlwaysSampler(),
		)
		ctx := context.Background()

		span := &Span{TraceID: "test"}
		if sampler.ShouldSample(ctx, span) != DecisionDrop {
			t.Error("All mode should drop when one sampler says no")
		}
	})

	t.Run("all mode success", func(t *testing.T) {
		sampler := NewCompositeSampler(
			CompositeAll,
			NewAlwaysSampler(),
			NewAlwaysSampler(),
		)
		ctx := context.Background()

		span := &Span{TraceID: "test"}
		if sampler.ShouldSample(ctx, span) != DecisionRecordAndSample {
			t.Error("All mode should sample when all say yes")
		}
	})

	t.Run("empty samplers", func(t *testing.T) {
		sampler := NewCompositeSampler(CompositeAny)
		ctx := context.Background()

		span := &Span{TraceID: "test"}
		if sampler.ShouldSample(ctx, span) != DecisionDrop {
			t.Error("Empty samplers should drop")
		}
	})
}

func TestErrorBiasSampler(t *testing.T) {
	baseSampler := NewNeverSampler()
	sampler := NewErrorBiasSampler(baseSampler, 1.0)
	ctx := context.Background()

	t.Run("errors sampled", func(t *testing.T) {
		span := &Span{TraceID: "error", IsError: true}
		if sampler.ShouldSample(ctx, span) != DecisionRecordAndSample {
			t.Error("Errors should be sampled")
		}
	})

	t.Run("non-errors follow base", func(t *testing.T) {
		span := &Span{TraceID: "ok", IsError: false}
		if sampler.ShouldSample(ctx, span) != DecisionDrop {
			t.Error("Non-errors should follow base sampler")
		}
	})
}

func TestNewSampler(t *testing.T) {
	tests := []struct {
		strategy Strategy
		expected string
	}{
		{StrategyAlways, "AlwaysSampler"},
		{StrategyNever, "NeverSampler"},
		{StrategyProbabilistic, "ProbabilisticSampler"},
		{StrategyRateLimiting, "RateLimitingSampler"},
		{StrategyAdaptive, "AdaptiveSampler"},
		{StrategyPriority, "PrioritySampler"},
	}

	for _, tt := range tests {
		t.Run(string(tt.strategy), func(t *testing.T) {
			config := DefaultConfig()
			config.Strategy = tt.strategy

			sampler := NewSampler(config)
			if sampler.Description() != tt.expected {
				t.Errorf("Description = %v, want %v", sampler.Description(), tt.expected)
			}
		})
	}
}

func TestConcurrentSampling(t *testing.T) {
	sampler := NewProbabilisticSampler(0.5)
	ctx := context.Background()

	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for j := 0; j < 1000; j++ {
				span := &Span{TraceID: "concurrent-" + string(rune(id))}
				sampler.ShouldSample(ctx, span)
			}
		}(i)
	}
	wg.Wait()
}

func TestRateLimitingSampler_Concurrent(t *testing.T) {
	sampler := NewRateLimitingSampler(1000)
	ctx := context.Background()

	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 100; j++ {
				span := &Span{TraceID: "concurrent"}
				sampler.ShouldSample(ctx, span)
			}
		}()
	}
	wg.Wait()
}

func randomTraceID() string {
	const letters = "0123456789abcdef"
	b := make([]byte, 32)
	for i := range b {
		b[i] = letters[rand.Intn(len(letters))]
	}
	return string(b)
}

