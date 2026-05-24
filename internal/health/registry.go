// SPDX-License-Identifier: Apache-2.0

package health

import (
	"context"
	"log/slog"
	"sync"
	"time"
)

// Status is the per-component health verdict.
type Status string

const (
	// StatusHealthy — last Check returned nil.
	StatusHealthy Status = "healthy"

	// StatusDegraded — reserved. v1.0 never produces it; consecutive-
	// failure / partial-availability tracking is future work.
	StatusDegraded Status = "degraded"

	// StatusUnhealthy — Check returned an error or timed out.
	StatusUnhealthy Status = "unhealthy"

	// StatusUnknown — no observation yet. Snapshot never returns this
	// in v1.0 (every check fires inline); reserved for a future
	// background-poller mode.
	StatusUnknown Status = "unknown"
)

// String implements fmt.Stringer.
func (s Status) String() string { return string(s) }

// Result is one Checker's observation at one point in time.
type Result struct {
	Name    string
	Status  Status
	Latency time.Duration
	Err     error // nil iff Status == StatusHealthy
}

// Snapshot is the aggregate of every Checker's Result plus the boot-
// state envelope. Ready reflects "all healthy AND past grace period".
type Snapshot struct {
	Ready         bool
	InGracePeriod bool
	StartedAt     time.Time
	Uptime        time.Duration
	Results       []Result
}

// Checker is what every component plugs in.
type Checker interface {
	// Name is the stable identifier used in the snapshot map (and the
	// public JSON). Lowercase, no spaces. Examples: "nats", "db",
	// "jetstream".
	Name() string

	// Interval is the suggested poll cadence for background pollers.
	// v1.0 Snapshot ignores this — it runs checks inline at request
	// time. Zero means "no hint".
	Interval() time.Duration

	// Check probes the component. Returns nil on success and any error
	// otherwise. Implementations MUST honour ctx; Snapshot bounds every
	// call with a per-check timeout.
	Check(ctx context.Context) error
}

// Options configures a Registry.
type Options struct {
	// CheckTimeout caps each Checker.Check call. Default 2 seconds.
	CheckTimeout time.Duration

	// StartupGracePeriod is the post-boot window during which Snapshot
	// reports InGracePeriod=true and Ready=false even if every checker
	// is healthy. Default 30 seconds.
	StartupGracePeriod time.Duration

	// StartedAt is the process boot time. Defaults to Now() at
	// construction.
	StartedAt time.Time

	// Now is the clock; tests override. Defaults to time.Now.
	Now func() time.Time

	// Logger receives per-check failure logs at warn level. Defaults
	// to slog.Default().
	Logger *slog.Logger
}

// Registry runs a set of Checkers and produces Snapshots.
//
// Safe for concurrent use after construction.
type Registry struct {
	checkTimeout time.Duration
	grace        time.Duration
	startedAt    time.Time
	now          func() time.Time
	logger       *slog.Logger

	mu       sync.RWMutex
	checkers []Checker
}

// NewRegistry constructs a Registry from opts. Zero opts is valid and
// produces a Registry with documented defaults.
func NewRegistry(opts Options) *Registry {
	if opts.CheckTimeout <= 0 {
		opts.CheckTimeout = 2 * time.Second
	}
	if opts.StartupGracePeriod <= 0 {
		opts.StartupGracePeriod = 30 * time.Second
	}
	if opts.Now == nil {
		opts.Now = time.Now
	}
	if opts.StartedAt.IsZero() {
		opts.StartedAt = opts.Now()
	}
	if opts.Logger == nil {
		opts.Logger = slog.Default()
	}
	return &Registry{
		checkTimeout: opts.CheckTimeout,
		grace:        opts.StartupGracePeriod,
		startedAt:    opts.StartedAt,
		now:          opts.Now,
		logger:       opts.Logger,
	}
}

// Register adds checkers to the registry. Duplicate names overwrite;
// callers wanting strict deduplication should check Names() first.
// Safe for concurrent use; typically called once at boot.
func (r *Registry) Register(checkers ...Checker) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.checkers = append(r.checkers, checkers...)
}

// Names returns the registered checker names in registration order.
// Used by tests and operator-tooling that wants to enumerate the set
// without firing probes.
func (r *Registry) Names() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]string, len(r.checkers))
	for i, c := range r.checkers {
		out[i] = c.Name()
	}
	return out
}

// StartedAt returns the boot timestamp the Registry was constructed
// with. The HTTP layer uses this to fill the "started_at" field.
func (r *Registry) StartedAt() time.Time { return r.startedAt }

// Snapshot runs every registered Checker in parallel under a per-check
// timeout and returns the aggregate Snapshot. Total wall-time is
// bounded by CheckTimeout (NOT N×CheckTimeout); a slow checker does
// not delay a fast one.
//
// Ready is true iff every Result is StatusHealthy AND uptime >=
// StartupGracePeriod.
func (r *Registry) Snapshot(ctx context.Context) Snapshot {
	r.mu.RLock()
	checkers := append([]Checker(nil), r.checkers...)
	r.mu.RUnlock()

	now := r.now()
	uptime := now.Sub(r.startedAt)
	inGrace := uptime < r.grace

	results := make([]Result, len(checkers))
	var wg sync.WaitGroup
	wg.Add(len(checkers))
	for i, c := range checkers {
		i, c := i, c
		go func() {
			defer wg.Done()
			results[i] = r.runOne(ctx, c)
		}()
	}
	wg.Wait()

	allHealthy := true
	for _, res := range results {
		if res.Status != StatusHealthy {
			allHealthy = false
			break
		}
	}
	return Snapshot{
		Ready:         allHealthy && !inGrace,
		InGracePeriod: inGrace,
		StartedAt:     r.startedAt,
		Uptime:        uptime,
		Results:       results,
	}
}

// runOne fires a single Checker under the configured timeout, captures
// latency, and maps the outcome to a Result.
func (r *Registry) runOne(parent context.Context, c Checker) Result {
	ctx, cancel := context.WithTimeout(parent, r.checkTimeout)
	defer cancel()

	start := r.now()
	err := c.Check(ctx)
	latency := r.now().Sub(start)

	res := Result{Name: c.Name(), Latency: latency}
	if err != nil {
		res.Status = StatusUnhealthy
		res.Err = err
		r.logger.Warn("health: check failed",
			"component", c.Name(),
			"latency_ms", latency.Milliseconds(),
			"err", err,
		)
		return res
	}
	res.Status = StatusHealthy
	return res
}
