package tracing

import (
	"sync"
	"sync/atomic"
	"time"

	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/trace"
)

// AdaptiveSamplingConfig configures adaptive sampling behavior
type AdaptiveSamplingConfig struct {
	// BaseRate is the baseline sampling rate (0.0 to 1.0)
	BaseRate float64 `json:"base_rate"`

	// MinRate is the minimum sampling rate even during low error periods
	MinRate float64 `json:"min_rate"`

	// MaxRate is the maximum sampling rate even during high error periods
	MaxRate float64 `json:"max_rate"`

	// ErrorThreshold is the error rate (0.0 to 1.0) above which to increase sampling
	ErrorThreshold float64 `json:"error_threshold"`

	// LowErrorThreshold is the error rate below which to decrease sampling
	LowErrorThreshold float64 `json:"low_error_threshold"`

	// AdaptationWindow is how long to observe before adapting
	AdaptationWindow time.Duration `json:"adaptation_window"`

	// RateIncrement is how much to increase rate when errors are high
	RateIncrement float64 `json:"rate_increment"`

	// RateDecrement is how much to decrease rate when errors are low
	RateDecrement float64 `json:"rate_decrement"`

	// PerSpanRules allows configuring sampling per span name
	PerSpanRules map[string]*SpanSamplingRule `json:"per_span_rules,omitempty"`

	// AlwaysSampleOnError ensures error spans are always sampled
	AlwaysSampleOnError bool `json:"always_sample_on_error"`

	// EnableDebugSpans enables sampling of debug/internal spans
	EnableDebugSpans bool `json:"enable_debug_spans"`
}

// SpanSamplingRule defines sampling rules for specific span names
type SpanSamplingRule struct {
	// Rate is the fixed sampling rate for this span (overrides adaptive)
	Rate float64 `json:"rate"`

	// AlwaysSample always samples this span
	AlwaysSample bool `json:"always_sample"`

	// NeverSample never samples this span
	NeverSample bool `json:"never_sample"`

	// SampleOnError samples this span when it has an error
	SampleOnError bool `json:"sample_on_error"`

	// MinRate is the minimum sampling rate for this span
	MinRate float64 `json:"min_rate,omitempty"`

	// MaxRate is the maximum sampling rate for this span
	MaxRate float64 `json:"max_rate,omitempty"`
}

// DefaultAdaptiveSamplingConfig returns sensible defaults
func DefaultAdaptiveSamplingConfig() *AdaptiveSamplingConfig {
	return &AdaptiveSamplingConfig{
		BaseRate:            0.1,  // 10% baseline
		MinRate:             0.01, // Never go below 1%
		MaxRate:             1.0,  // Can go up to 100% on errors
		ErrorThreshold:      0.05, // Increase sampling above 5% error rate
		LowErrorThreshold:   0.01, // Decrease sampling below 1% error rate
		AdaptationWindow:    time.Minute,
		RateIncrement:       0.1, // Increase by 10% per adaptation
		RateDecrement:       0.05, // Decrease by 5% per adaptation
		PerSpanRules:        make(map[string]*SpanSamplingRule),
		AlwaysSampleOnError: true,
		EnableDebugSpans:    false,
	}
}

// AdaptiveSampler implements adaptive trace sampling based on error rates
type AdaptiveSampler struct {
	config *AdaptiveSamplingConfig

	// Current sampling rate (atomic for lock-free reads)
	currentRate atomic.Value // float64

	// Error tracking
	mu            sync.RWMutex
	totalSpans    int64
	errorSpans    int64
	windowStart   time.Time
	lastAdaptTime time.Time

	// Per-span tracking
	perSpanStats map[string]*spanStats

	// Underlying probabilistic sampler
	sampler sdktrace.Sampler

	// Stop channel for cleanup goroutine
	stopCh chan struct{}
}

// spanStats tracks statistics for a specific span name
type spanStats struct {
	total  int64
	errors int64
}

// NewAdaptiveSampler creates a new adaptive sampler
func NewAdaptiveSampler(config *AdaptiveSamplingConfig) *AdaptiveSampler {
	if config == nil {
		config = DefaultAdaptiveSamplingConfig()
	}

	// Validate and normalize config
	if config.BaseRate < 0 {
		config.BaseRate = 0
	}
	if config.BaseRate > 1 {
		config.BaseRate = 1
	}
	if config.MinRate < 0 {
		config.MinRate = 0
	}
	if config.MaxRate > 1 {
		config.MaxRate = 1
	}
	if config.MinRate > config.MaxRate {
		config.MinRate = config.MaxRate
	}
	if config.AdaptationWindow <= 0 {
		config.AdaptationWindow = time.Minute
	}

	s := &AdaptiveSampler{
		config:       config,
		windowStart:  time.Now(),
		perSpanStats: make(map[string]*spanStats),
		sampler:      sdktrace.TraceIDRatioBased(config.BaseRate),
		stopCh:       make(chan struct{}),
	}

	s.currentRate.Store(config.BaseRate)

	// Start background adaptation goroutine
	go s.adaptLoop()

	return s
}

// ShouldSample implements sdktrace.Sampler
func (s *AdaptiveSampler) ShouldSample(parameters sdktrace.SamplingParameters) sdktrace.SamplingResult {
	spanName := parameters.Name

	// Check per-span rules first
	if rule, ok := s.config.PerSpanRules[spanName]; ok {
		if rule.NeverSample {
			return sdktrace.SamplingResult{
				Decision:   sdktrace.Drop,
				Tracestate: trace.SpanContextFromContext(parameters.ParentContext).TraceState(),
			}
		}
		if rule.AlwaysSample {
			return sdktrace.SamplingResult{
				Decision:   sdktrace.RecordAndSample,
				Tracestate: trace.SpanContextFromContext(parameters.ParentContext).TraceState(),
			}
		}

		// Use per-span rate if specified
		if rule.Rate > 0 {
			sampler := sdktrace.TraceIDRatioBased(rule.Rate)
			return sampler.ShouldSample(parameters)
		}
	}

	// Check if parent is sampled (parent-based behavior)
	parentSpanContext := trace.SpanContextFromContext(parameters.ParentContext)
	if parentSpanContext.IsValid() && parentSpanContext.IsSampled() {
		return sdktrace.SamplingResult{
			Decision:   sdktrace.RecordAndSample,
			Tracestate: parentSpanContext.TraceState(),
		}
	}

	// Check for debug/internal spans
	if !s.config.EnableDebugSpans {
		// Skip common debug/internal span patterns
		if isDebugSpan(spanName) {
			return sdktrace.SamplingResult{
				Decision:   sdktrace.Drop,
				Tracestate: parentSpanContext.TraceState(),
			}
		}
	}

	// Use current adaptive rate
	currentRate := s.currentRate.Load().(float64)
	sampler := sdktrace.TraceIDRatioBased(currentRate)
	return sampler.ShouldSample(parameters)
}

// Description implements sdktrace.Sampler
func (s *AdaptiveSampler) Description() string {
	return "AdaptiveSampler"
}

// RecordSpanResult records the result of a span for adaptive sampling
func (s *AdaptiveSampler) RecordSpanResult(spanName string, isError bool) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.totalSpans++
	if isError {
		s.errorSpans++
	}

	// Track per-span stats
	stats, ok := s.perSpanStats[spanName]
	if !ok {
		stats = &spanStats{}
		s.perSpanStats[spanName] = stats
	}
	stats.total++
	if isError {
		stats.errors++
	}
}

// GetCurrentRate returns the current sampling rate
func (s *AdaptiveSampler) GetCurrentRate() float64 {
	return s.currentRate.Load().(float64)
}

// GetErrorRate returns the current error rate
func (s *AdaptiveSampler) GetErrorRate() float64 {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if s.totalSpans == 0 {
		return 0
	}
	return float64(s.errorSpans) / float64(s.totalSpans)
}

// GetStats returns sampling statistics
func (s *AdaptiveSampler) GetStats() *SamplingStats {
	s.mu.RLock()
	defer s.mu.RUnlock()

	stats := &SamplingStats{
		CurrentRate:      s.currentRate.Load().(float64),
		TotalSpans:       s.totalSpans,
		ErrorSpans:       s.errorSpans,
		WindowStart:      s.windowStart,
		LastAdaptation:   s.lastAdaptTime,
		PerSpanStats:     make(map[string]*PerSpanSamplingStats),
	}

	if s.totalSpans > 0 {
		stats.ErrorRate = float64(s.errorSpans) / float64(s.totalSpans)
	}

	for name, ps := range s.perSpanStats {
		errorRate := float64(0)
		if ps.total > 0 {
			errorRate = float64(ps.errors) / float64(ps.total)
		}
		stats.PerSpanStats[name] = &PerSpanSamplingStats{
			TotalSpans: ps.total,
			ErrorSpans: ps.errors,
			ErrorRate:  errorRate,
		}
	}

	return stats
}

// SamplingStats contains sampling statistics
type SamplingStats struct {
	CurrentRate    float64                            `json:"current_rate"`
	TotalSpans     int64                              `json:"total_spans"`
	ErrorSpans     int64                              `json:"error_spans"`
	ErrorRate      float64                            `json:"error_rate"`
	WindowStart    time.Time                          `json:"window_start"`
	LastAdaptation time.Time                          `json:"last_adaptation"`
	PerSpanStats   map[string]*PerSpanSamplingStats   `json:"per_span_stats,omitempty"`
}

// PerSpanSamplingStats contains per-span statistics
type PerSpanSamplingStats struct {
	TotalSpans int64   `json:"total_spans"`
	ErrorSpans int64   `json:"error_spans"`
	ErrorRate  float64 `json:"error_rate"`
}

// Reset resets the sampling statistics
func (s *AdaptiveSampler) Reset() {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.totalSpans = 0
	s.errorSpans = 0
	s.windowStart = time.Now()
	s.perSpanStats = make(map[string]*spanStats)
}

// Stop stops the adaptive sampling goroutine
func (s *AdaptiveSampler) Stop() {
	close(s.stopCh)
}

// adaptLoop runs the periodic adaptation logic
func (s *AdaptiveSampler) adaptLoop() {
	ticker := time.NewTicker(s.config.AdaptationWindow)
	defer ticker.Stop()

	for {
		select {
		case <-s.stopCh:
			return
		case <-ticker.C:
			s.adapt()
		}
	}
}

// adapt adjusts the sampling rate based on error rate
func (s *AdaptiveSampler) adapt() {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Calculate current error rate
	var errorRate float64
	if s.totalSpans > 0 {
		errorRate = float64(s.errorSpans) / float64(s.totalSpans)
	}

	currentRate := s.currentRate.Load().(float64)
	newRate := currentRate

	// Adjust rate based on error rate
	if errorRate >= s.config.ErrorThreshold {
		// High error rate - increase sampling
		newRate = currentRate + s.config.RateIncrement
		if newRate > s.config.MaxRate {
			newRate = s.config.MaxRate
		}
	} else if errorRate <= s.config.LowErrorThreshold {
		// Low error rate - can decrease sampling
		newRate = currentRate - s.config.RateDecrement
		if newRate < s.config.MinRate {
			newRate = s.config.MinRate
		}
	}
	// If between thresholds, maintain current rate

	s.currentRate.Store(newRate)
	s.lastAdaptTime = time.Now()

	// Reset window counters
	s.totalSpans = 0
	s.errorSpans = 0
	s.windowStart = time.Now()
	s.perSpanStats = make(map[string]*spanStats)
}

// SetRate manually sets the sampling rate (overrides adaptive)
func (s *AdaptiveSampler) SetRate(rate float64) {
	if rate < s.config.MinRate {
		rate = s.config.MinRate
	}
	if rate > s.config.MaxRate {
		rate = s.config.MaxRate
	}
	s.currentRate.Store(rate)
}

// AddSpanRule adds or updates a per-span sampling rule
func (s *AdaptiveSampler) AddSpanRule(spanName string, rule *SpanSamplingRule) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.config.PerSpanRules[spanName] = rule
}

// RemoveSpanRule removes a per-span sampling rule
func (s *AdaptiveSampler) RemoveSpanRule(spanName string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.config.PerSpanRules, spanName)
}

// isDebugSpan checks if a span name is a debug/internal span
func isDebugSpan(name string) bool {
	debugPrefixes := []string{
		"internal/",
		"debug/",
		"healthcheck",
		"readiness",
		"liveness",
		"metrics",
	}

	for _, prefix := range debugPrefixes {
		if len(name) >= len(prefix) && name[:len(prefix)] == prefix {
			return true
		}
	}
	return false
}

// RateLimitingSampler implements rate-limited sampling
type RateLimitingSampler struct {
	maxPerSecond int64
	currentCount atomic.Int64
	lastReset    atomic.Value // time.Time
	mu           sync.Mutex
}

// NewRateLimitingSampler creates a sampler that limits traces per second
func NewRateLimitingSampler(maxPerSecond int64) *RateLimitingSampler {
	if maxPerSecond <= 0 {
		maxPerSecond = 100 // Default to 100 traces/second
	}

	s := &RateLimitingSampler{
		maxPerSecond: maxPerSecond,
	}
	s.lastReset.Store(time.Now())

	return s
}

// ShouldSample implements sdktrace.Sampler
func (s *RateLimitingSampler) ShouldSample(parameters sdktrace.SamplingParameters) sdktrace.SamplingResult {
	parentSpanContext := trace.SpanContextFromContext(parameters.ParentContext)

	// Check if parent is sampled (parent-based behavior)
	if parentSpanContext.IsValid() && parentSpanContext.IsSampled() {
		return sdktrace.SamplingResult{
			Decision:   sdktrace.RecordAndSample,
			Tracestate: parentSpanContext.TraceState(),
		}
	}

	// Check rate limit
	now := time.Now()
	lastReset := s.lastReset.Load().(time.Time)

	// Reset counter if a second has passed
	if now.Sub(lastReset) >= time.Second {
		s.mu.Lock()
		// Double-check after acquiring lock
		lastReset = s.lastReset.Load().(time.Time)
		if now.Sub(lastReset) >= time.Second {
			s.currentCount.Store(0)
			s.lastReset.Store(now)
		}
		s.mu.Unlock()
	}

	// Increment and check if within limit
	count := s.currentCount.Add(1)
	if count <= s.maxPerSecond {
		return sdktrace.SamplingResult{
			Decision:   sdktrace.RecordAndSample,
			Tracestate: parentSpanContext.TraceState(),
		}
	}

	return sdktrace.SamplingResult{
		Decision:   sdktrace.Drop,
		Tracestate: parentSpanContext.TraceState(),
	}
}

// Description implements sdktrace.Sampler
func (s *RateLimitingSampler) Description() string {
	return "RateLimitingSampler"
}

// CompositeSampler combines multiple sampling strategies
type CompositeSampler struct {
	samplers []sdktrace.Sampler
	mode     CompositeSamplerMode
}

// CompositeSamplerMode defines how to combine sampler decisions
type CompositeSamplerMode int

const (
	// CompositeModeAny samples if any sampler decides to sample
	CompositeModeAny CompositeSamplerMode = iota

	// CompositeModeAll samples only if all samplers decide to sample
	CompositeModeAll

	// CompositeModePriority uses the first sampler that makes a definitive decision
	CompositeModePriority
)

// NewCompositeSampler creates a composite sampler
func NewCompositeSampler(mode CompositeSamplerMode, samplers ...sdktrace.Sampler) *CompositeSampler {
	return &CompositeSampler{
		samplers: samplers,
		mode:     mode,
	}
}

// ShouldSample implements sdktrace.Sampler
func (s *CompositeSampler) ShouldSample(parameters sdktrace.SamplingParameters) sdktrace.SamplingResult {
	parentSpanContext := trace.SpanContextFromContext(parameters.ParentContext)

	if len(s.samplers) == 0 {
		return sdktrace.SamplingResult{
			Decision:   sdktrace.Drop,
			Tracestate: parentSpanContext.TraceState(),
		}
	}

	switch s.mode {
	case CompositeModeAny:
		for _, sampler := range s.samplers {
			result := sampler.ShouldSample(parameters)
			if result.Decision == sdktrace.RecordAndSample {
				return result
			}
		}
		return sdktrace.SamplingResult{
			Decision:   sdktrace.Drop,
			Tracestate: parentSpanContext.TraceState(),
		}

	case CompositeModeAll:
		for _, sampler := range s.samplers {
			result := sampler.ShouldSample(parameters)
			if result.Decision != sdktrace.RecordAndSample {
				return sdktrace.SamplingResult{
					Decision:   sdktrace.Drop,
					Tracestate: parentSpanContext.TraceState(),
				}
			}
		}
		return sdktrace.SamplingResult{
			Decision:   sdktrace.RecordAndSample,
			Tracestate: parentSpanContext.TraceState(),
		}

	case CompositeModePriority:
		// Return first sampler's decision
		if len(s.samplers) > 0 {
			return s.samplers[0].ShouldSample(parameters)
		}
	}

	return sdktrace.SamplingResult{
		Decision:   sdktrace.Drop,
		Tracestate: parentSpanContext.TraceState(),
	}
}

// Description implements sdktrace.Sampler
func (s *CompositeSampler) Description() string {
	return "CompositeSampler"
}

// ErrorAwareSampler wraps a sampler and records span results for adaptive sampling
type ErrorAwareSampler struct {
	inner   sdktrace.Sampler
	sampler *AdaptiveSampler
}

// NewErrorAwareSampler creates an error-aware sampler that records span results
func NewErrorAwareSampler(adaptive *AdaptiveSampler) *ErrorAwareSampler {
	if adaptive == nil {
		adaptive = NewAdaptiveSampler(nil)
	}

	return &ErrorAwareSampler{
		inner:   adaptive,
		sampler: adaptive,
	}
}

// ShouldSample implements sdktrace.Sampler
func (s *ErrorAwareSampler) ShouldSample(parameters sdktrace.SamplingParameters) sdktrace.SamplingResult {
	return s.inner.ShouldSample(parameters)
}

// Description implements sdktrace.Sampler
func (s *ErrorAwareSampler) Description() string {
	return "ErrorAwareSampler"
}

// RecordSpan records a span result for adaptive sampling
func (s *ErrorAwareSampler) RecordSpan(spanName string, isError bool) {
	s.sampler.RecordSpanResult(spanName, isError)
}

// GetStats returns the underlying sampler's statistics
func (s *ErrorAwareSampler) GetStats() *SamplingStats {
	return s.sampler.GetStats()
}

// Stop stops the underlying adaptive sampler
func (s *ErrorAwareSampler) Stop() {
	s.sampler.Stop()
}
