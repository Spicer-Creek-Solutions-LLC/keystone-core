// Package sampling provides intelligent sampling for Keystone observability.
package sampling

import (
	"context"
	"hash/fnv"
	"math/rand" // nosemgrep: go.lang.security.audit.crypto.math_random.math-random-used -- sampling does not require crypto randomness
	"sync"
	"sync/atomic"
	"time"
)

// Strategy represents a sampling strategy.
type Strategy string

const (
	// StrategyAlways samples all requests.
	StrategyAlways Strategy = "always"
	// StrategyNever samples no requests.
	StrategyNever Strategy = "never"
	// StrategyProbabilistic samples based on probability.
	StrategyProbabilistic Strategy = "probabilistic"
	// StrategyRateLimiting samples based on rate limits.
	StrategyRateLimiting Strategy = "rate_limiting"
	// StrategyAdaptive adjusts sampling based on conditions.
	StrategyAdaptive Strategy = "adaptive"
	// StrategyPriority samples based on priority.
	StrategyPriority Strategy = "priority"
	// StrategyParentBased follows parent sampling decision.
	StrategyParentBased Strategy = "parent_based"
)

// Decision represents a sampling decision.
type Decision int

const (
	// DecisionDrop indicates the request should not be sampled.
	DecisionDrop Decision = iota
	// DecisionRecordOnly indicates metadata should be recorded but not full trace.
	DecisionRecordOnly
	// DecisionRecordAndSample indicates full sampling.
	DecisionRecordAndSample
)

// String returns the string representation.
func (d Decision) String() string {
	switch d {
	case DecisionDrop:
		return "drop"
	case DecisionRecordOnly:
		return "record_only"
	case DecisionRecordAndSample:
		return "record_and_sample"
	default:
		return "unknown"
	}
}

// Span represents span information for sampling decisions.
type Span struct {
	TraceID    string            `json:"traceId"`
	SpanID     string            `json:"spanId"`
	ParentID   string            `json:"parentId,omitempty"`
	Name       string            `json:"name"`
	Service    string            `json:"service"`
	Attributes map[string]string `json:"attributes,omitempty"`
	IsError    bool              `json:"isError"`
	Duration   time.Duration     `json:"duration,omitempty"`
	Priority   int               `json:"priority"`
}

// Config configures sampling behavior.
type Config struct {
	Strategy        Strategy      `json:"strategy"`
	SampleRate      float64       `json:"sampleRate"`      // 0.0-1.0
	RateLimit       float64       `json:"rateLimit"`       // samples per second
	ErrorSampleRate float64       `json:"errorSampleRate"` // rate for errors
	SlowThreshold   time.Duration `json:"slowThreshold"`
	SlowSampleRate  float64       `json:"slowSampleRate"` // rate for slow requests
	MinSampleRate   float64       `json:"minSampleRate"`  // minimum for adaptive
	MaxSampleRate   float64       `json:"maxSampleRate"`  // maximum for adaptive
	TargetRate      float64       `json:"targetRate"`     // target samples/second for adaptive
}

// DefaultConfig returns a default configuration.
func DefaultConfig() *Config {
	return &Config{
		Strategy:        StrategyProbabilistic,
		SampleRate:      0.1,
		RateLimit:       100,
		ErrorSampleRate: 1.0,
		SlowThreshold:   time.Second,
		SlowSampleRate:  0.5,
		MinSampleRate:   0.01,
		MaxSampleRate:   1.0,
		TargetRate:      100,
	}
}

func shouldSample(rate float64) bool {
	//nolint:gosec // G404: math/rand used for probabilistic sampling, not security
	return rand.Float64() < rate // nosemgrep: go.lang.security.audit.crypto.math_random.math-random-used -- sampling does not require crypto randomness
}

// Sampler makes sampling decisions.
type Sampler interface {
	ShouldSample(ctx context.Context, span *Span) Decision
	Description() string
}

// AlwaysSampler always samples.
type AlwaysSampler struct{}

// NewAlwaysSampler creates a new always sampler.
func NewAlwaysSampler() *AlwaysSampler {
	return &AlwaysSampler{}
}

// ShouldSample always returns RecordAndSample.
func (s *AlwaysSampler) ShouldSample(ctx context.Context, span *Span) Decision {
	return DecisionRecordAndSample
}

// Description returns a description.
func (s *AlwaysSampler) Description() string {
	return "AlwaysSampler"
}

// NeverSampler never samples.
type NeverSampler struct{}

// NewNeverSampler creates a new never sampler.
func NewNeverSampler() *NeverSampler {
	return &NeverSampler{}
}

// ShouldSample always returns Drop.
func (s *NeverSampler) ShouldSample(ctx context.Context, span *Span) Decision {
	return DecisionDrop
}

// Description returns a description.
func (s *NeverSampler) Description() string {
	return "NeverSampler"
}

// ProbabilisticSampler samples based on probability.
type ProbabilisticSampler struct {
	rate float64
}

// NewProbabilisticSampler creates a new probabilistic sampler.
func NewProbabilisticSampler(rate float64) *ProbabilisticSampler {
	if rate < 0 {
		rate = 0
	}
	if rate > 1 {
		rate = 1
	}
	return &ProbabilisticSampler{rate: rate}
}

// ShouldSample samples based on trace ID hash.
func (s *ProbabilisticSampler) ShouldSample(ctx context.Context, span *Span) Decision {
	if s.rate >= 1.0 {
		return DecisionRecordAndSample
	}
	if s.rate <= 0 {
		return DecisionDrop
	}

	// Use trace ID for consistent sampling
	if span.TraceID != "" {
		h := fnv.New64a()
		h.Write([]byte(span.TraceID))
		hash := h.Sum64()
		// Use modulo for better distribution
		threshold := uint64(s.rate * float64(1<<32))
		if (hash & 0xFFFFFFFF) < threshold {
			return DecisionRecordAndSample
		}
		return DecisionDrop
	}

	// Fall back to random
	if shouldSample(s.rate) {
		return DecisionRecordAndSample
	}
	return DecisionDrop
}

// Description returns a description.
func (s *ProbabilisticSampler) Description() string {
	return "ProbabilisticSampler"
}

// RateLimitingSampler limits the sample rate.
type RateLimitingSampler struct {
	rate     float64
	tokens   float64
	lastTick time.Time
	mu       sync.Mutex
}

// NewRateLimitingSampler creates a new rate limiting sampler.
func NewRateLimitingSampler(rate float64) *RateLimitingSampler {
	return &RateLimitingSampler{
		rate:     rate,
		tokens:   rate,
		lastTick: time.Now(),
	}
}

// ShouldSample samples based on rate limit.
func (s *RateLimitingSampler) ShouldSample(ctx context.Context, span *Span) Decision {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now()
	elapsed := now.Sub(s.lastTick).Seconds()
	s.lastTick = now

	// Add tokens
	s.tokens += elapsed * s.rate
	if s.tokens > s.rate {
		s.tokens = s.rate
	}

	if s.tokens >= 1.0 {
		s.tokens -= 1.0
		return DecisionRecordAndSample
	}

	return DecisionDrop
}

// Description returns a description.
func (s *RateLimitingSampler) Description() string {
	return "RateLimitingSampler"
}

// AdaptiveSampler adjusts sampling based on conditions.
type AdaptiveSampler struct {
	config      *Config
	currentRate float64
	sampleCount int64 // atomic
	totalCount  int64 // atomic
	errorCount  int64 // atomic
	slowCount   int64 // atomic
	lastAdjust  time.Time
	mu          sync.RWMutex
}

// NewAdaptiveSampler creates a new adaptive sampler.
func NewAdaptiveSampler(config *Config) *AdaptiveSampler {
	if config == nil {
		config = DefaultConfig()
	}

	return &AdaptiveSampler{
		config:      config,
		currentRate: config.SampleRate,
		lastAdjust:  time.Now(),
	}
}

// ShouldSample adaptively samples.
func (s *AdaptiveSampler) ShouldSample(ctx context.Context, span *Span) Decision {
	atomic.AddInt64(&s.totalCount, 1)

	// Always sample errors at high rate
	if span.IsError {
		atomic.AddInt64(&s.errorCount, 1)
		if shouldSample(s.config.ErrorSampleRate) {
			atomic.AddInt64(&s.sampleCount, 1)
			return DecisionRecordAndSample
		}
	}

	// Sample slow requests at higher rate
	if span.Duration > 0 && span.Duration > s.config.SlowThreshold {
		atomic.AddInt64(&s.slowCount, 1)
		if shouldSample(s.config.SlowSampleRate) {
			atomic.AddInt64(&s.sampleCount, 1)
			return DecisionRecordAndSample
		}
	}

	// Use current adaptive rate
	s.mu.RLock()
	rate := s.currentRate
	s.mu.RUnlock()

	if shouldSample(rate) {
		atomic.AddInt64(&s.sampleCount, 1)
		return DecisionRecordAndSample
	}

	return DecisionDrop
}

// Adjust adjusts the sampling rate based on recent metrics.
func (s *AdaptiveSampler) Adjust() {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now()
	elapsed := now.Sub(s.lastAdjust).Seconds()
	if elapsed < 1.0 {
		return
	}

	s.lastAdjust = now

	sampledCount := atomic.SwapInt64(&s.sampleCount, 0)
	totalCount := atomic.SwapInt64(&s.totalCount, 0)

	if totalCount == 0 {
		return
	}

	currentSamplesPerSecond := float64(sampledCount) / elapsed
	targetRate := s.config.TargetRate

	if currentSamplesPerSecond > targetRate*1.2 {
		// Too many samples, reduce rate
		s.currentRate *= 0.9
	} else if currentSamplesPerSecond < targetRate*0.8 {
		// Too few samples, increase rate
		s.currentRate *= 1.1
	}

	// Clamp to bounds
	if s.currentRate < s.config.MinSampleRate {
		s.currentRate = s.config.MinSampleRate
	}
	if s.currentRate > s.config.MaxSampleRate {
		s.currentRate = s.config.MaxSampleRate
	}
}

// CurrentRate returns the current sampling rate.
func (s *AdaptiveSampler) CurrentRate() float64 {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.currentRate
}

// Stats returns sampling statistics.
func (s *AdaptiveSampler) Stats() AdaptiveStats {
	return AdaptiveStats{
		CurrentRate: s.CurrentRate(),
		TotalCount:  atomic.LoadInt64(&s.totalCount),
		SampleCount: atomic.LoadInt64(&s.sampleCount),
		ErrorCount:  atomic.LoadInt64(&s.errorCount),
		SlowCount:   atomic.LoadInt64(&s.slowCount),
	}
}

// Description returns a description.
func (s *AdaptiveSampler) Description() string {
	return "AdaptiveSampler"
}

// AdaptiveStats contains adaptive sampler statistics.
type AdaptiveStats struct {
	CurrentRate float64 `json:"currentRate"`
	TotalCount  int64   `json:"totalCount"`
	SampleCount int64   `json:"sampleCount"`
	ErrorCount  int64   `json:"errorCount"`
	SlowCount   int64   `json:"slowCount"`
}

// PrioritySampler samples based on priority.
type PrioritySampler struct {
	thresholds map[int]float64
	mu         sync.RWMutex
}

// NewPrioritySampler creates a new priority sampler.
func NewPrioritySampler() *PrioritySampler {
	return &PrioritySampler{
		thresholds: map[int]float64{
			0: 0.01, // Low priority
			1: 0.1,  // Normal priority
			2: 0.5,  // High priority
			3: 1.0,  // Critical priority
		},
	}
}

// SetThreshold sets the sampling rate for a priority level.
func (s *PrioritySampler) SetThreshold(priority int, rate float64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.thresholds[priority] = rate
}

// ShouldSample samples based on span priority.
func (s *PrioritySampler) ShouldSample(ctx context.Context, span *Span) Decision {
	s.mu.RLock()
	rate, ok := s.thresholds[span.Priority]
	s.mu.RUnlock()

	if !ok {
		rate = 0.1 // Default
	}

	if shouldSample(rate) {
		return DecisionRecordAndSample
	}
	return DecisionDrop
}

// Description returns a description.
func (s *PrioritySampler) Description() string {
	return "PrioritySampler"
}

// ParentBasedSampler follows parent sampling decision.
type ParentBasedSampler struct {
	root                   Sampler
	remoteParentSampled    Sampler
	remoteParentNotSampled Sampler
	localParentSampled     Sampler
	localParentNotSampled  Sampler
}

// ParentBasedConfig configures the parent-based sampler.
type ParentBasedConfig struct {
	Root                   Sampler
	RemoteParentSampled    Sampler
	RemoteParentNotSampled Sampler
	LocalParentSampled     Sampler
	LocalParentNotSampled  Sampler
}

// NewParentBasedSampler creates a new parent-based sampler.
func NewParentBasedSampler(config *ParentBasedConfig) *ParentBasedSampler {
	root := config.Root
	if root == nil {
		root = NewProbabilisticSampler(0.1)
	}

	return &ParentBasedSampler{
		root:                   root,
		remoteParentSampled:    config.RemoteParentSampled,
		remoteParentNotSampled: config.RemoteParentNotSampled,
		localParentSampled:     config.LocalParentSampled,
		localParentNotSampled:  config.LocalParentNotSampled,
	}
}

// ShouldSample delegates to appropriate sampler based on parent.
func (s *ParentBasedSampler) ShouldSample(ctx context.Context, span *Span) Decision {
	if span.ParentID == "" {
		// Root span
		return s.root.ShouldSample(ctx, span)
	}

	// Has parent - check if sampled from context
	parentSampled := isParentSampled(ctx)

	if parentSampled {
		if s.localParentSampled != nil {
			return s.localParentSampled.ShouldSample(ctx, span)
		}
		return DecisionRecordAndSample
	}

	if s.localParentNotSampled != nil {
		return s.localParentNotSampled.ShouldSample(ctx, span)
	}
	return DecisionDrop
}

// Description returns a description.
func (s *ParentBasedSampler) Description() string {
	return "ParentBasedSampler"
}

type sampledKey struct{}

func isParentSampled(ctx context.Context) bool {
	val := ctx.Value(sampledKey{})
	if val == nil {
		return false
	}
	return val.(bool)
}

// WithSampled returns a context with sampled value.
func WithSampled(ctx context.Context, sampled bool) context.Context {
	return context.WithValue(ctx, sampledKey{}, sampled)
}

// CompositeSampler combines multiple samplers.
type CompositeSampler struct {
	samplers []Sampler
	mode     CompositeMode
}

// CompositeMode determines how decisions are combined.
type CompositeMode int

const (
	// CompositeAny samples if any sampler says yes.
	CompositeAny CompositeMode = iota
	// CompositeAll samples only if all samplers say yes.
	CompositeAll
)

// NewCompositeSampler creates a new composite sampler.
func NewCompositeSampler(mode CompositeMode, samplers ...Sampler) *CompositeSampler {
	return &CompositeSampler{
		samplers: samplers,
		mode:     mode,
	}
}

// ShouldSample combines decisions from all samplers.
func (s *CompositeSampler) ShouldSample(ctx context.Context, span *Span) Decision {
	if len(s.samplers) == 0 {
		return DecisionDrop
	}

	switch s.mode {
	case CompositeAny:
		for _, sampler := range s.samplers {
			if sampler.ShouldSample(ctx, span) == DecisionRecordAndSample {
				return DecisionRecordAndSample
			}
		}
		return DecisionDrop

	case CompositeAll:
		for _, sampler := range s.samplers {
			if sampler.ShouldSample(ctx, span) != DecisionRecordAndSample {
				return DecisionDrop
			}
		}
		return DecisionRecordAndSample

	default:
		return DecisionDrop
	}
}

// Description returns a description.
func (s *CompositeSampler) Description() string {
	return "CompositeSampler"
}

// ErrorBiasSampler increases sampling for errors.
type ErrorBiasSampler struct {
	baseSampler Sampler
	errorRate   float64
}

// NewErrorBiasSampler creates a new error bias sampler.
func NewErrorBiasSampler(baseSampler Sampler, errorRate float64) *ErrorBiasSampler {
	if errorRate > 1.0 {
		errorRate = 1.0
	}
	return &ErrorBiasSampler{
		baseSampler: baseSampler,
		errorRate:   errorRate,
	}
}

// ShouldSample biases sampling toward errors.
func (s *ErrorBiasSampler) ShouldSample(ctx context.Context, span *Span) Decision {
	if span.IsError && shouldSample(s.errorRate) {
		return DecisionRecordAndSample
	}
	return s.baseSampler.ShouldSample(ctx, span)
}

// Description returns a description.
func (s *ErrorBiasSampler) Description() string {
	return "ErrorBiasSampler"
}

// NewSampler creates a sampler from config.
func NewSampler(config *Config) Sampler {
	if config == nil {
		config = DefaultConfig()
	}

	switch config.Strategy {
	case StrategyAlways:
		return NewAlwaysSampler()
	case StrategyNever:
		return NewNeverSampler()
	case StrategyProbabilistic:
		return NewProbabilisticSampler(config.SampleRate)
	case StrategyRateLimiting:
		return NewRateLimitingSampler(config.RateLimit)
	case StrategyAdaptive:
		return NewAdaptiveSampler(config)
	case StrategyPriority:
		return NewPrioritySampler()
	default:
		return NewProbabilisticSampler(config.SampleRate)
	}
}
