// Package cardinality enforces hard limits on the number of distinct
// label-value combinations a Prom metric may emit.
//
// The limiter is consulted on every labeled observation. It either
// accepts the value tuple (under cap, or already seen), drops it
// (Drop mode, over cap), or rewrites it to OverflowSentinel before
// accepting (Aggregate mode, over cap). Outcomes are reported via the
// injected Reporter so the parent metrics package can attribute them
// to kscore_metrics_cardinality_total.
//
// This package never imports the parent metrics package; the parent
// drives it through Reporter, breaking the import cycle that would
// otherwise arise from the self-metric.
package cardinality

import (
	"log/slog"
	"strings"
	"sync"
	"time"
)

// Mode selects what the limiter does when a metric is at its cap and
// a new combination of label values arrives.
type Mode int

const (
	// Drop silently no-ops the observation. Default. Best for
	// high-cardinality risk where a single bucket per combo is more
	// misleading than nothing.
	Drop Mode = iota

	// Aggregate rewrites every label value in the new tuple to
	// OverflowSentinel before recording. Operators see one bucket
	// labelled {label1: "_overflow", ...} accumulating over-cap activity.
	Aggregate
)

// Outcome is what the limiter decided for a single Track call.
type Outcome int

const (
	Accepted Outcome = iota
	Dropped
	Aggregated
)

// String returns the lower-case label value used by the parent
// metrics package on kscore_metrics_cardinality_total{outcome=...}.
func (o Outcome) String() string {
	switch o {
	case Accepted:
		return "accepted"
	case Dropped:
		return "dropped"
	case Aggregated:
		return "aggregated"
	default:
		return "unknown"
	}
}

// OverflowSentinel is the label value the limiter substitutes in
// Aggregate mode once a metric is at its cardinality cap. Chosen so it
// sorts and reads obviously in Grafana.
const OverflowSentinel = "_overflow"

// Reporter receives one notification per Track call. The parent metrics
// package implements this to increment kscore_metrics_cardinality_total.
type Reporter interface {
	Report(metric string, outcome Outcome)
}

// ReporterFunc adapts a plain function to Reporter.
type ReporterFunc func(metric string, outcome Outcome)

// Report implements Reporter.
func (f ReporterFunc) Report(metric string, outcome Outcome) { f(metric, outcome) }

// Options configures a Limiter.
type Options struct {
	// Mode is Drop (default) or Aggregate.
	Mode Mode

	// DefaultMax is the per-metric cap applied when Configure has not
	// been called for the metric. Zero falls back to defaultMax (10_000).
	DefaultMax int

	// Logger is used for first-drop warnings. Defaults to slog.Default().
	Logger *slog.Logger

	// Reporter sinks outcome notifications. May be nil (notifications
	// are then dropped silently — useful in tests).
	Reporter Reporter

	// WarnEveryAfter throttles the "metric M dropped a value" warning to
	// at most one log per metric per WarnEveryAfter window. Zero falls
	// back to defaultWarnEvery (1m).
	WarnEveryAfter time.Duration
}

const (
	defaultMax       = 10_000
	defaultWarnEvery = time.Minute
)

// Limiter is safe for concurrent use.
type Limiter struct {
	mode     Mode
	defMax   int
	logger   *slog.Logger
	reporter Reporter
	warnEvery time.Duration

	mu       sync.RWMutex
	caps     map[string]int                 // metric -> max combinations
	seen     map[string]map[string]struct{} // metric -> set of joined tuples
	lastWarn map[string]time.Time           // metric -> last warn timestamp

	now func() time.Time // for tests; nil means time.Now
}

// New constructs a Limiter.
func New(opts Options) *Limiter {
	if opts.Logger == nil {
		opts.Logger = slog.Default()
	}
	if opts.DefaultMax == 0 {
		opts.DefaultMax = defaultMax
	}
	if opts.WarnEveryAfter == 0 {
		opts.WarnEveryAfter = defaultWarnEvery
	}
	return &Limiter{
		mode:      opts.Mode,
		defMax:    opts.DefaultMax,
		logger:    opts.Logger,
		reporter:  opts.Reporter,
		warnEvery: opts.WarnEveryAfter,
		caps:      make(map[string]int),
		seen:      make(map[string]map[string]struct{}),
		lastWarn:  make(map[string]time.Time),
	}
}

// Configure sets the per-metric cap. Re-configuring shrinks or grows
// the cap; combinations already accepted stay accepted (the limiter
// never evicts to honour a shrunken cap — that would silently lose
// time-series data).
func (l *Limiter) Configure(metric string, max int) {
	if max <= 0 {
		max = l.defMax
	}
	l.mu.Lock()
	l.caps[metric] = max
	if _, ok := l.seen[metric]; !ok {
		l.seen[metric] = make(map[string]struct{})
	}
	l.mu.Unlock()
}

// SetClock overrides time.Now for deterministic tests.
func (l *Limiter) SetClock(now func() time.Time) {
	l.mu.Lock()
	l.now = now
	l.mu.Unlock()
}

// Mode returns the configured Mode (for tests).
func (l *Limiter) ModeValue() Mode { return l.mode }

// Track records the observation and returns the outcome plus the label
// values the caller should ultimately pass to the underlying Prom
// collector. In Drop mode and over-cap, returns (Dropped, nil) and the
// caller skips the observation. In Aggregate mode and over-cap, the
// returned slice has every value rewritten to OverflowSentinel.
//
// values is consumed by reference but not retained.
func (l *Limiter) Track(metric string, values []string) (Outcome, []string) {
	key := joinValues(values)

	l.mu.RLock()
	seen := l.seen[metric]
	if seen != nil {
		if _, ok := seen[key]; ok {
			l.mu.RUnlock()
			l.report(metric, Accepted)
			return Accepted, values
		}
	}
	cap := l.caps[metric]
	if cap == 0 {
		cap = l.defMax
	}
	atCap := seen != nil && len(seen) >= cap
	l.mu.RUnlock()

	if !atCap {
		// Slow path: lock-upgrade and double-check.
		l.mu.Lock()
		seen = l.seen[metric]
		if seen == nil {
			seen = make(map[string]struct{})
			l.seen[metric] = seen
		}
		cap = l.caps[metric]
		if cap == 0 {
			cap = l.defMax
		}
		if _, ok := seen[key]; ok {
			l.mu.Unlock()
			l.report(metric, Accepted)
			return Accepted, values
		}
		if len(seen) < cap {
			seen[key] = struct{}{}
			l.mu.Unlock()
			l.report(metric, Accepted)
			return Accepted, values
		}
		l.mu.Unlock()
	}

	// At cap and not previously seen → decide by mode.
	if l.mode == Aggregate {
		out := make([]string, len(values))
		for i := range out {
			out[i] = OverflowSentinel
		}
		l.maybeWarn(metric)
		l.report(metric, Aggregated)
		return Aggregated, out
	}
	l.maybeWarn(metric)
	l.report(metric, Dropped)
	return Dropped, nil
}

// Snapshot returns the number of accepted label combinations per metric.
// Tests use this to verify caps without scraping the Prom registry.
func (l *Limiter) Snapshot() map[string]int {
	l.mu.RLock()
	defer l.mu.RUnlock()
	out := make(map[string]int, len(l.seen))
	for k, v := range l.seen {
		out[k] = len(v)
	}
	return out
}

func (l *Limiter) report(metric string, o Outcome) {
	if l.reporter != nil {
		l.reporter.Report(metric, o)
	}
}

// maybeWarn logs at most once per metric per warnEvery.
func (l *Limiter) maybeWarn(metric string) {
	now := l.timeNow()
	l.mu.Lock()
	last := l.lastWarn[metric]
	if now.Sub(last) < l.warnEvery {
		l.mu.Unlock()
		return
	}
	l.lastWarn[metric] = now
	l.mu.Unlock()
	l.logger.Warn("metrics: cardinality cap reached",
		"metric", metric, "mode", l.modeString())
}

func (l *Limiter) timeNow() time.Time {
	l.mu.RLock()
	fn := l.now
	l.mu.RUnlock()
	if fn != nil {
		return fn()
	}
	return time.Now()
}

func (l *Limiter) modeString() string {
	if l.mode == Aggregate {
		return "aggregate"
	}
	return "drop"
}

// joinValues collapses the value slice into a single key. We use \x1e
// (record separator) so any reasonable label value joins unambiguously;
// label values containing \x1e are pathological and accepted as-is.
func joinValues(values []string) string {
	if len(values) == 0 {
		return ""
	}
	return strings.Join(values, "\x1e")
}
