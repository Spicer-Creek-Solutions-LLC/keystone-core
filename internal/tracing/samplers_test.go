// SPDX-License-Identifier: Apache-2.0

package tracing

import (
	"context"
	"strings"
	"testing"

	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/trace"
)

func newParentCtx(t *testing.T, sampled bool) context.Context {
	t.Helper()
	traceID, err := trace.TraceIDFromHex("0102030405060708090a0b0c0d0e0f10")
	if err != nil {
		t.Fatal(err)
	}
	spanID, err := trace.SpanIDFromHex("0102030405060708")
	if err != nil {
		t.Fatal(err)
	}
	flags := trace.TraceFlags(0)
	if sampled {
		flags = flags.WithSampled(true)
	}
	sc := trace.NewSpanContext(trace.SpanContextConfig{
		TraceID:    traceID,
		SpanID:     spanID,
		TraceFlags: flags,
		Remote:     true,
	})
	return trace.ContextWithSpanContext(context.Background(), sc)
}

func TestRateLimitingSampler_RootUnderRate_Samples(t *testing.T) {
	s := NewRateLimitingSampler(100)
	res := s.ShouldSample(sdktrace.SamplingParameters{
		ParentContext: context.Background(),
		Name:          "op",
	})
	if res.Decision != sdktrace.RecordAndSample {
		t.Errorf("decision = %v, want RecordAndSample", res.Decision)
	}
}

func TestRateLimitingSampler_RootOverRate_Drops(t *testing.T) {
	// perSecond=0 → every root drops.
	s := NewRateLimitingSampler(0)
	res := s.ShouldSample(sdktrace.SamplingParameters{
		ParentContext: context.Background(),
		Name:          "op",
	})
	if res.Decision != sdktrace.Drop {
		t.Errorf("decision = %v, want Drop", res.Decision)
	}
}

func TestRateLimitingSampler_BurstThenDrop(t *testing.T) {
	// perSecond=2 → first 2 root spans accepted, then drop until refill.
	s := NewRateLimitingSampler(2)
	for i := 0; i < 2; i++ {
		if got := s.ShouldSample(sdktrace.SamplingParameters{ParentContext: context.Background()}).Decision; got != sdktrace.RecordAndSample {
			t.Errorf("burst %d: decision = %v, want RecordAndSample", i, got)
		}
	}
	if got := s.ShouldSample(sdktrace.SamplingParameters{ParentContext: context.Background()}).Decision; got != sdktrace.Drop {
		t.Errorf("post-burst decision = %v, want Drop", got)
	}
}

func TestRateLimitingSampler_HonorsSampledParent(t *testing.T) {
	// perSecond=0 — root spans would drop, but a sampled parent must
	// still keep its children recorded so the trace stays linked.
	s := NewRateLimitingSampler(0)
	res := s.ShouldSample(sdktrace.SamplingParameters{
		ParentContext: newParentCtx(t, true),
		Name:          "child",
	})
	if res.Decision != sdktrace.RecordAndSample {
		t.Errorf("sampled-parent child decision = %v, want RecordAndSample", res.Decision)
	}
}

func TestRateLimitingSampler_HonorsUnsampledParent(t *testing.T) {
	// perSecond=1000 — root would happily accept, but an unsampled
	// parent must stay unsampled.
	s := NewRateLimitingSampler(1000)
	res := s.ShouldSample(sdktrace.SamplingParameters{
		ParentContext: newParentCtx(t, false),
		Name:          "child",
	})
	if res.Decision != sdktrace.Drop {
		t.Errorf("unsampled-parent child decision = %v, want Drop", res.Decision)
	}
}

func TestRateLimitingSampler_Description(t *testing.T) {
	s := NewRateLimitingSampler(50)
	if d := s.Description(); !strings.Contains(d, "RateLimiting") || !strings.Contains(d, "50") {
		t.Errorf("Description = %q, want to contain RateLimiting and 50", d)
	}
}
