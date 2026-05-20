package metrics

import (
	"fmt"

	"github.com/prometheus/client_golang/prometheus"

	"go.keystone-core.io/keystone-core/internal/metrics/cardinality"
)

// Counter is a monotonically increasing scalar; observations record
// either 1 (Inc) or a positive delta (Add). For labeled metrics, callers
// must bind labels via With before recording.
type Counter interface {
	// Inc records a unit observation.
	Inc()
	// Add records a positive delta. Negative deltas are silently
	// dropped — Prom counters reject them at the underlying *Vec.
	Add(float64)
	// With returns a Counter bound to the given label values. The
	// returned Counter shares the same underlying *Vec; calling Inc on
	// the unbound parent of a labeled metric is a no-op.
	With(Labels) Counter
}

// Gauge is a scalar that can move up or down.
type Gauge interface {
	Set(float64)
	Inc()
	Dec()
	Add(float64)
	Sub(float64)
	With(Labels) Gauge
}

// Histogram observes positive samples into pre-declared buckets.
type Histogram interface {
	Observe(float64)
	With(Labels) Histogram
}

// Summary observes samples and reports quantiles over a sliding window.
type Summary interface {
	Observe(float64)
	With(Labels) Summary
}

// NewCounter constructs a Counter, registers it with the underlying
// Prom registry, and reserves the metric name. Returns an error if the
// name is already taken or the MetricDef is incomplete.
func (r *Registry) NewCounter(def MetricDef) (Counter, error) {
	if err := r.reserveName(def); err != nil {
		return nil, err
	}
	vec := prometheus.NewCounterVec(prometheus.CounterOpts{Name: def.Name, Help: def.Help}, def.Labels)
	if err := r.prom.Register(vec); err != nil {
		return nil, fmt.Errorf("metrics: register %s: %w", def.Name, err)
	}
	return &counter{vec: vec, def: def, registry: r}, nil
}

// NewGauge constructs a Gauge.
func (r *Registry) NewGauge(def MetricDef) (Gauge, error) {
	if err := r.reserveName(def); err != nil {
		return nil, err
	}
	vec := prometheus.NewGaugeVec(prometheus.GaugeOpts{Name: def.Name, Help: def.Help}, def.Labels)
	if err := r.prom.Register(vec); err != nil {
		return nil, fmt.Errorf("metrics: register %s: %w", def.Name, err)
	}
	return &gauge{vec: vec, def: def, registry: r}, nil
}

// NewHistogram constructs a Histogram. Empty Buckets fall back to
// prometheus.DefBuckets (covers 5ms to 10s).
func (r *Registry) NewHistogram(def MetricDef) (Histogram, error) {
	if err := r.reserveName(def); err != nil {
		return nil, err
	}
	buckets := def.Buckets
	if len(buckets) == 0 {
		buckets = prometheus.DefBuckets
	}
	vec := prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Name:    def.Name,
		Help:    def.Help,
		Buckets: buckets,
	}, def.Labels)
	if err := r.prom.Register(vec); err != nil {
		return nil, fmt.Errorf("metrics: register %s: %w", def.Name, err)
	}
	return &histogram{vec: vec, def: def, registry: r}, nil
}

// NewSummary constructs a Summary. Empty Objectives fall back to
// prometheus.DefObjectives.
func (r *Registry) NewSummary(def MetricDef) (Summary, error) {
	if err := r.reserveName(def); err != nil {
		return nil, err
	}
	objectives := def.Objectives
	if len(objectives) == 0 {
		objectives = map[float64]float64{0.5: 0.05, 0.9: 0.01, 0.99: 0.001}
	}
	vec := prometheus.NewSummaryVec(prometheus.SummaryOpts{
		Name:       def.Name,
		Help:       def.Help,
		Objectives: objectives,
	}, def.Labels)
	if err := r.prom.Register(vec); err != nil {
		return nil, fmt.Errorf("metrics: register %s: %w", def.Name, err)
	}
	return &summary{vec: vec, def: def, registry: r}, nil
}

// resolveValues maps a Labels map onto the MetricDef's ordered label
// names. Missing keys are passed as "" so prom still routes them to a
// valid child. The returned slice is fresh.
func resolveValues(def MetricDef, labels Labels) []string {
	out := make([]string, len(def.Labels))
	for i, name := range def.Labels {
		out[i] = labels[name]
	}
	return out
}

// trackThrough runs the limiter and returns the final values to pass to
// the underlying *Vec, or false if the observation should be dropped.
// registry==nil bypasses the limiter (used by the self-metric to avoid
// recursion).
func trackThrough(r *Registry, def MetricDef, values []string) ([]string, bool) {
	if r == nil || len(def.Labels) == 0 {
		return values, true
	}
	outcome, finalValues := r.limiter.Track(def.Name, values)
	if outcome == cardinality.Dropped {
		return nil, false
	}
	return finalValues, true
}

// counter is the concrete Counter implementation. labels==nil means
// "unbound" — Inc/Add only work on no-label metrics.
type counter struct {
	vec      *prometheus.CounterVec
	def      MetricDef
	registry *Registry
	labels   Labels
}

func (c *counter) Inc()           { c.add(1) }
func (c *counter) Add(v float64)  { c.add(v) }
func (c *counter) With(l Labels) Counter {
	return &counter{vec: c.vec, def: c.def, registry: c.registry, labels: l}
}

func (c *counter) add(v float64) {
	if v < 0 {
		return
	}
	if len(c.def.Labels) == 0 {
		c.vec.WithLabelValues().Add(v)
		return
	}
	if c.labels == nil {
		return
	}
	values := resolveValues(c.def, c.labels)
	final, ok := trackThrough(c.registry, c.def, values)
	if !ok {
		return
	}
	c.vec.WithLabelValues(final...).Add(v)
}

type gauge struct {
	vec      *prometheus.GaugeVec
	def      MetricDef
	registry *Registry
	labels   Labels
}

func (g *gauge) Set(v float64) { g.write(func(c prometheus.Gauge) { c.Set(v) }) }
func (g *gauge) Inc()          { g.write(prometheus.Gauge.Inc) }
func (g *gauge) Dec()          { g.write(prometheus.Gauge.Dec) }
func (g *gauge) Add(v float64) { g.write(func(c prometheus.Gauge) { c.Add(v) }) }
func (g *gauge) Sub(v float64) { g.write(func(c prometheus.Gauge) { c.Sub(v) }) }
func (g *gauge) With(l Labels) Gauge {
	return &gauge{vec: g.vec, def: g.def, registry: g.registry, labels: l}
}

func (g *gauge) write(fn func(prometheus.Gauge)) {
	if len(g.def.Labels) == 0 {
		fn(g.vec.WithLabelValues())
		return
	}
	if g.labels == nil {
		return
	}
	values := resolveValues(g.def, g.labels)
	final, ok := trackThrough(g.registry, g.def, values)
	if !ok {
		return
	}
	fn(g.vec.WithLabelValues(final...))
}

type histogram struct {
	vec      *prometheus.HistogramVec
	def      MetricDef
	registry *Registry
	labels   Labels
}

func (h *histogram) Observe(v float64) {
	if len(h.def.Labels) == 0 {
		h.vec.WithLabelValues().Observe(v)
		return
	}
	if h.labels == nil {
		return
	}
	values := resolveValues(h.def, h.labels)
	final, ok := trackThrough(h.registry, h.def, values)
	if !ok {
		return
	}
	h.vec.WithLabelValues(final...).Observe(v)
}

func (h *histogram) With(l Labels) Histogram {
	return &histogram{vec: h.vec, def: h.def, registry: h.registry, labels: l}
}

type summary struct {
	vec      *prometheus.SummaryVec
	def      MetricDef
	registry *Registry
	labels   Labels
}

func (s *summary) Observe(v float64) {
	if len(s.def.Labels) == 0 {
		s.vec.WithLabelValues().Observe(v)
		return
	}
	if s.labels == nil {
		return
	}
	values := resolveValues(s.def, s.labels)
	final, ok := trackThrough(s.registry, s.def, values)
	if !ok {
		return
	}
	s.vec.WithLabelValues(final...).Observe(v)
}

func (s *summary) With(l Labels) Summary {
	return &summary{vec: s.vec, def: s.def, registry: s.registry, labels: l}
}
