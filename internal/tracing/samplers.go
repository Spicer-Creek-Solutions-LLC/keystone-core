package tracing

import (
	"fmt"

	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/trace"
	"golang.org/x/time/rate"

	"go.keystone-core.io/keystone-core/internal/config"
)

// newSampler builds the configured sdktrace.Sampler from cfg.
func newSampler(cfg config.TracingConfig) (sdktrace.Sampler, error) {
	switch cfg.Sampler {
	case config.TracingSamplerAlwaysOn:
		return sdktrace.AlwaysSample(), nil
	case config.TracingSamplerAlwaysOff:
		return sdktrace.NeverSample(), nil
	case config.TracingSamplerProbabilistic:
		return sdktrace.TraceIDRatioBased(cfg.SampleRate), nil
	case config.TracingSamplerParentBased:
		// Parent-based defaults to the probabilistic sampler for root
		// spans; child spans honour the parent's sampled flag.
		return sdktrace.ParentBased(sdktrace.TraceIDRatioBased(cfg.SampleRate)), nil
	case config.TracingSamplerRateLimiting:
		return NewRateLimitingSampler(cfg.RateLimitPerSecond), nil
	default:
		return nil, fmt.Errorf("tracing: unknown sampler %q", cfg.Sampler)
	}
}

// RateLimitingSampler caps the number of accepted root spans per second.
// Above the cap, ShouldSample returns Drop. Spans whose parent is
// already sampled are always recorded — once a trace is being sampled,
// dropping mid-trace would produce broken parent/child links.
//
// burst sets the token-bucket burst capacity. We default to perSecond
// so a quiet second can absorb a brief spike up to the configured cap;
// callers wanting a tighter ceiling can construct with NewRateLimiting
// SamplerBurst.
type RateLimitingSampler struct {
	limiter  *rate.Limiter
	perSec   int
	innerOff sdktrace.Sampler
	innerOn  sdktrace.Sampler
}

// NewRateLimitingSampler builds a sampler that accepts at most
// perSecond root spans per second. perSecond <= 0 disables sampling
// entirely (returns Drop for every span).
func NewRateLimitingSampler(perSecond int) *RateLimitingSampler {
	burst := perSecond
	if burst < 1 {
		burst = 1
	}
	return &RateLimitingSampler{
		limiter:  rate.NewLimiter(rate.Limit(perSecond), burst),
		perSec:   perSecond,
		innerOff: sdktrace.NeverSample(),
		innerOn:  sdktrace.AlwaysSample(),
	}
}

// ShouldSample implements sdktrace.Sampler. Parent decisions win: a
// sampled parent never has its children dropped by this sampler, and
// a dropped parent never has its children up-sampled.
func (s *RateLimitingSampler) ShouldSample(p sdktrace.SamplingParameters) sdktrace.SamplingResult {
	psc := trace.SpanContextFromContext(p.ParentContext)
	if psc.IsValid() {
		if psc.IsSampled() {
			return s.innerOn.ShouldSample(p)
		}
		return s.innerOff.ShouldSample(p)
	}
	// Root span: consult the rate limiter.
	if s.perSec <= 0 || !s.limiter.Allow() {
		return s.innerOff.ShouldSample(p)
	}
	return s.innerOn.ShouldSample(p)
}

// Description implements sdktrace.Sampler.
func (s *RateLimitingSampler) Description() string {
	return fmt.Sprintf("RateLimitingSampler{rate=%d/s}", s.perSec)
}

// Compile-time interface compliance.
var _ sdktrace.Sampler = (*RateLimitingSampler)(nil)
