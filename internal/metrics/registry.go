package metrics

import (
	"fmt"
	"log/slog"
	"sync"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"

	"go.keystone-core.io/keystone-core/internal/metrics/cardinality"
)

// Labels binds label names to values for a single observation.
type Labels = map[string]string

// MetricDef declares a metric. Subsystems hand a MetricDef to
// Registry.NewCounter/Gauge/Histogram/Summary; the Registry stores it so
// downstream tooling (Grafana dashboard validation, metric-name CI diff)
// can enumerate every metric the binary may emit even before any
// observation fires.
type MetricDef struct {
	// Name is the fully-qualified Prom metric name, e.g.
	// kscore_commands_executed_total. Must be unique within a Registry.
	Name string

	// Help is the human-readable description scraped by Prometheus.
	Help string

	// Labels is the ordered list of label names. Empty for unlabeled
	// metrics. Observers pass values keyed by these names through
	// Counter.With(Labels{...}).
	Labels []string

	// Buckets is the histogram bucket boundaries (seconds). Empty falls
	// back to prometheus.DefBuckets. Ignored for non-histogram metrics.
	Buckets []float64

	// Objectives is the summary quantile/error pairs. Empty falls back
	// to prometheus.DefObjectives. Ignored for non-summary metrics.
	Objectives map[float64]float64

	// MaxCardinality overrides the Registry-wide default
	// (DefaultMaxCardinality) for this metric. Zero means inherit.
	MaxCardinality int
}

// DefaultMaxCardinality is the per-metric hard limit on distinct
// label-value combinations. Operators tune via MetricDef.MaxCardinality.
const DefaultMaxCardinality = 10_000

// CardinalityMetricName is the well-known self-metric every Registry
// pre-registers so operators can alert on drops. See PROJECT-DETAILS
// §4.16 "monitor kscore_metrics_cardinality_total".
const CardinalityMetricName = "kscore_metrics_cardinality_total"

// Registry is a process-private Prometheus registry plus a cardinality
// limiter and a directory of registered MetricDefs.
//
// Registry is safe for concurrent use after construction.
type Registry struct {
	prom    *prometheus.Registry
	limiter *cardinality.Limiter
	logger  *slog.Logger
	defMax  int // resolved per-Registry default cardinality cap

	mu          sync.RWMutex
	defs        map[string]MetricDef
	cardCounter *counter // bound counter for kscore_metrics_cardinality_total
}

// Options configures Registry construction.
type Options struct {
	// Logger is used for limiter warnings. Defaults to slog.Default().
	Logger *slog.Logger

	// CardinalityMode controls limiter behaviour: cardinality.Drop (the
	// default) silently no-ops new label combinations once the cap is
	// reached; cardinality.Aggregate rewrites them to the "_overflow"
	// sentinel so a single bucket still records the activity.
	CardinalityMode cardinality.Mode

	// DefaultMaxCardinality overrides DefaultMaxCardinality for all
	// metrics in this Registry that do not set MetricDef.MaxCardinality.
	// Zero means use the package default.
	DefaultMaxCardinality int

	// DisableRuntimeCollectors skips the auto-registration of
	// prometheus/client_golang's Go-runtime and process collectors.
	// Defaults to false (collectors are registered). Tests that scrape
	// a deterministic metric set set this true.
	DisableRuntimeCollectors bool
}

// NewRegistry constructs a Registry. It pre-registers the self-metric
// kscore_metrics_cardinality_total and wires the limiter to report into
// it.
func NewRegistry(opts Options) *Registry {
	if opts.Logger == nil {
		opts.Logger = slog.Default()
	}
	max := opts.DefaultMaxCardinality
	if max == 0 {
		max = DefaultMaxCardinality
	}

	r := &Registry{
		prom:   prometheus.NewRegistry(),
		logger: opts.Logger,
		defMax: max,
		defs:   make(map[string]MetricDef),
	}
	r.limiter = cardinality.New(cardinality.Options{
		Mode:           opts.CardinalityMode,
		DefaultMax:     max,
		Logger:         opts.Logger,
		Reporter:       cardinality.ReporterFunc(r.reportCardinality),
		WarnEveryAfter: 0, // package default
	})

	// Self-metric must register without limiter recursion, so we build
	// the *Vec directly and stash a *counter shim that bypasses limiter
	// tracking (its own label set is closed: metric + outcome).
	vec := prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: CardinalityMetricName,
		Help: "Cardinality-limiter outcomes per metric (accepted, dropped, aggregated).",
	}, []string{"metric", "outcome"})
	r.prom.MustRegister(vec)
	def := MetricDef{
		Name:   CardinalityMetricName,
		Help:   "Cardinality-limiter outcomes per metric (accepted, dropped, aggregated).",
		Labels: []string{"metric", "outcome"},
	}
	r.defs[CardinalityMetricName] = def
	r.cardCounter = &counter{vec: vec, def: def, registry: nil} // nil registry => skip limiter

	if !opts.DisableRuntimeCollectors {
		// go_* and process_* metric families. These never participate
		// in the cardinality limiter — they have no high-cardinality
		// labels — so registering them with the underlying Prom
		// registry directly is correct.
		r.prom.MustRegister(collectors.NewGoCollector())
		r.prom.MustRegister(collectors.NewProcessCollector(collectors.ProcessCollectorOpts{}))
	}

	return r
}

// Gatherer returns the underlying prometheus.Gatherer for /metrics
// wiring. Callers should treat it as opaque.
func (r *Registry) Gatherer() prometheus.Gatherer { return r.prom }

// Definitions returns a copy of every MetricDef registered so far,
// keyed by metric name. Used by CI to diff against the dashboard set.
func (r *Registry) Definitions() map[string]MetricDef {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make(map[string]MetricDef, len(r.defs))
	for k, v := range r.defs {
		out[k] = v
	}
	return out
}

// Limiter exposes the cardinality limiter for tests and operator
// tooling. Production code should not poke at it directly.
func (r *Registry) Limiter() *cardinality.Limiter { return r.limiter }

// reserveName records a MetricDef and returns an error if the name is
// already taken. Must be held under the write lock.
func (r *Registry) reserveName(def MetricDef) error {
	if def.Name == "" {
		return fmt.Errorf("metrics: MetricDef.Name is required")
	}
	if def.Help == "" {
		return fmt.Errorf("metrics: MetricDef.Help is required (metric=%s)", def.Name)
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exists := r.defs[def.Name]; exists {
		return fmt.Errorf("metrics: metric %q already registered", def.Name)
	}
	maxC := def.MaxCardinality
	if maxC == 0 {
		maxC = r.defMax
	}
	r.limiter.Configure(def.Name, maxC)
	r.defs[def.Name] = def
	return nil
}

// reportCardinality is wired into the limiter as its Reporter; it
// increments the self-metric counter.
func (r *Registry) reportCardinality(metric string, outcome cardinality.Outcome) {
	if r.cardCounter == nil {
		return
	}
	r.cardCounter.With(Labels{"metric": metric, "outcome": outcome.String()}).Inc()
}
