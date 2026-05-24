// SPDX-License-Identifier: Apache-2.0

package events

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"math/rand/v2"
	"sync"
	"sync/atomic"
	"time"
)

// Retention defaults per PROJECT-DETAILS §4.9. Operators override
// via [WithRetentionInterval] / [WithRetentionJitter] /
// [WithRetentionPolicies] at construction.
const (
	// DefaultRetentionInterval is the §4.9 "runs hourly on cluster
	// leader" cadence.
	DefaultRetentionInterval = time.Hour

	// DefaultRetentionJitter is ±10% — anti-thundering-herd if a
	// multi-node deployment ever has more than one node running the
	// enforcer (Epic 13 leader-gate eliminates that case, but the
	// jitter is cheap insurance).
	DefaultRetentionJitter = 0.1
)

// DefaultCatchAllPolicy is the [RetentionPolicy] applied when an
// operator enables retention without configuring any policies of
// their own. Matches the §4.9 JetStream stream defaults so the SQL
// store doesn't grow unbounded relative to the bus.
//
// Operators with custom policies still benefit: empty-Type
// (catch-all) is one of the policies they MAY include in their list;
// the enforcer applies every policy in order.
var DefaultCatchAllPolicy = RetentionPolicy{
	Type:     "",
	MaxAge:   7 * 24 * time.Hour,
	MaxCount: 1_000_000,
}

// LeaderCheck reports whether the calling node is currently the
// cluster leader. The [RetentionEnforcer] consults this before
// running each tick — only the leader actually deletes rows, so
// multi-node deployments don't race on the same row set.
//
// Epic 13 (Clustering / HA) ships the production leader-election
// implementation. v1.0 single-node deployments use the default
// [AlwaysLeader] which returns true on every call.
type LeaderCheck func() bool

// AlwaysLeader is the v1.0 default — single-node deployments don't
// have an election to consult. Wrapped via
// [WithRetentionLeaderCheck] when Epic 13 wires real election.
func AlwaysLeader() bool { return true }

// RetentionEnforcer runs [EventStore.ApplyRetention] on a configurable
// interval. Designed for the §4.9 "hourly job; runs on leader"
// invariant: ticks at `interval ± jitter`, consults the leader
// check before each pass, logs + counts errors but never crashes
// the scheduler.
//
// Lifecycle mirrors [JetStreamPublisher]: Start once, Stop once.
// `RunOnce` is independent of the scheduler — operators can drive
// it from a future CLI / RPC (deferred to v1.x ROADMAP per task 7).
type RetentionEnforcer struct {
	store       EventStore
	policies    []RetentionPolicy
	interval    time.Duration
	jitter      float64
	leaderCheck LeaderCheck
	logger      *slog.Logger

	started atomic.Bool

	workerCtx context.Context
	cancel    context.CancelFunc
	wg        sync.WaitGroup

	lastRunAt      atomic.Pointer[time.Time]
	lastRunDeleted atomic.Int64
	totalDeleted   atomic.Int64
	runsFailed     atomic.Int64
}

// RetentionEnforcerOption configures a [RetentionEnforcer] at
// construction. Required: [WithRetentionStore]. Optional: every
// other knob has a sensible default; an empty policy list falls
// back to [DefaultCatchAllPolicy].
type RetentionEnforcerOption func(*retentionConfig)

type retentionConfig struct {
	store       EventStore
	policies    []RetentionPolicy
	interval    time.Duration
	jitter      float64
	leaderCheck LeaderCheck
	logger      *slog.Logger
}

func defaultRetentionConfig() retentionConfig {
	return retentionConfig{
		interval:    DefaultRetentionInterval,
		jitter:      DefaultRetentionJitter,
		leaderCheck: AlwaysLeader,
		logger:      slog.Default(),
	}
}

// WithRetentionStore wires the [EventStore] the enforcer drives.
// Required at construction — [NewRetentionEnforcer] returns an
// error otherwise.
func WithRetentionStore(s EventStore) RetentionEnforcerOption {
	return func(c *retentionConfig) { c.store = s }
}

// WithRetentionPolicies sets the policy list. Empty list (or
// option not used) falls back to [DefaultCatchAllPolicy] when
// retention is enabled — see [NewRetentionEnforcer].
func WithRetentionPolicies(policies []RetentionPolicy) RetentionEnforcerOption {
	return func(c *retentionConfig) {
		// Defensive copy so the caller's slice isn't aliased into the
		// enforcer.
		c.policies = append(c.policies[:0:0], policies...)
	}
}

// WithRetentionInterval overrides the tick cadence. Zero or
// negative falls back to [DefaultRetentionInterval].
func WithRetentionInterval(d time.Duration) RetentionEnforcerOption {
	return func(c *retentionConfig) {
		if d > 0 {
			c.interval = d
		}
	}
}

// WithRetentionJitter sets the ±fraction-of-interval randomisation
// applied to each tick. Out-of-range (negative or > 0.5) falls
// back to [DefaultRetentionJitter] — > 0.5 would make the actual
// interval too unpredictable for hourly cadence.
func WithRetentionJitter(fraction float64) RetentionEnforcerOption {
	return func(c *retentionConfig) {
		if fraction >= 0 && fraction <= 0.5 {
			c.jitter = fraction
		}
	}
}

// WithRetentionLeaderCheck supplies the leader-election predicate.
// Nil falls back to [AlwaysLeader] — appropriate for v1.0 single-
// node deployments. Epic 13 passes its election impl here.
func WithRetentionLeaderCheck(fn LeaderCheck) RetentionEnforcerOption {
	return func(c *retentionConfig) {
		if fn != nil {
			c.leaderCheck = fn
		}
	}
}

// WithRetentionLogger overrides the slog.Logger. Nil leaves the
// default (slog.Default).
func WithRetentionLogger(l *slog.Logger) RetentionEnforcerOption {
	return func(c *retentionConfig) {
		if l != nil {
			c.logger = l
		}
	}
}

// NewRetentionEnforcer builds an enforcer from the options. Returns
// an error when the store is missing — every other field has a
// default. Empty policy list is normalised to
// [DefaultCatchAllPolicy] so "enabled but empty" is never an
// accidental no-op.
func NewRetentionEnforcer(opts ...RetentionEnforcerOption) (*RetentionEnforcer, error) {
	cfg := defaultRetentionConfig()
	for _, opt := range opts {
		opt(&cfg)
	}
	if cfg.store == nil {
		return nil, errors.New("events: retention: store is required (use WithRetentionStore)")
	}
	if len(cfg.policies) == 0 {
		cfg.policies = []RetentionPolicy{DefaultCatchAllPolicy}
	}
	return &RetentionEnforcer{
		store:       cfg.store,
		policies:    cfg.policies,
		interval:    cfg.interval,
		jitter:      cfg.jitter,
		leaderCheck: cfg.leaderCheck,
		logger:      cfg.logger,
	}, nil
}

// Start launches the scheduler goroutine. Double-Start without an
// intervening Stop is rejected. First tick fires after
// `interval ± jitter` — no boot storm; operators can call
// [RetentionEnforcer.RunOnce] for an immediate pass.
//
// The worker derives its own cancelable context from Start
// (gosec-G118 clean, mirroring [JetStreamPublisher]). Stop cancels
// it to release any in-flight ApplyRetention call.
func (e *RetentionEnforcer) Start(_ context.Context) error {
	if !e.started.CompareAndSwap(false, true) {
		return errors.New("events: retention: already started")
	}
	e.workerCtx, e.cancel = context.WithCancel(context.Background())
	e.wg.Add(1)
	go e.run(e.workerCtx)
	return nil
}

// Stop signals the scheduler to exit and waits for the current tick
// (if any) to complete OR for the caller's context to expire.
// Idempotent — calling Stop on a never-started or already-stopped
// enforcer returns nil.
func (e *RetentionEnforcer) Stop(ctx context.Context) error {
	if !e.started.CompareAndSwap(true, false) {
		return nil
	}
	e.cancel()
	done := make(chan struct{})
	go func() {
		e.wg.Wait()
		close(done)
	}()
	select {
	case <-done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// RunOnce executes a single retention pass synchronously. Skips
// silently (returns 0, nil) when the leader check reports the node
// is not the leader. Available regardless of scheduler state so
// operators can trigger a manual pass at any time.
func (e *RetentionEnforcer) RunOnce(ctx context.Context) (int, error) {
	if !e.leaderCheck() {
		return 0, nil
	}
	deleted, err := e.store.ApplyRetention(ctx, e.policies)
	now := time.Now().UTC()
	e.lastRunAt.Store(&now)
	if err != nil {
		e.runsFailed.Add(1)
		return deleted, err
	}
	e.lastRunDeleted.Store(int64(deleted))
	e.totalDeleted.Add(int64(deleted))
	return deleted, nil
}

// LastRunAt returns the wall-clock time of the most recent
// completed retention pass (success OR failure). Zero time means
// no pass has completed yet.
func (e *RetentionEnforcer) LastRunAt() time.Time {
	if t := e.lastRunAt.Load(); t != nil {
		return *t
	}
	return time.Time{}
}

// LastRunDeleted returns the deletion count from the most recent
// SUCCESSFUL retention pass. Failed runs don't update this.
func (e *RetentionEnforcer) LastRunDeleted() int64 {
	return e.lastRunDeleted.Load()
}

// TotalDeleted returns the cumulative deletion count since process
// start. Useful for Epic 17 observability + capacity planning.
func (e *RetentionEnforcer) TotalDeleted() int64 {
	return e.totalDeleted.Load()
}

// RunsFailed returns the cumulative count of retention passes that
// returned an error. Failed runs are logged at WARN.
func (e *RetentionEnforcer) RunsFailed() int64 {
	return e.runsFailed.Load()
}

// run is the scheduler goroutine. Sleeps `interval ± jitter` between
// passes; first tick fires AFTER the first jittered interval so
// startup logs stay clean.
func (e *RetentionEnforcer) run(ctx context.Context) {
	defer e.wg.Done()
	for {
		wait := e.nextWait()
		timer := time.NewTimer(wait)
		select {
		case <-ctx.Done():
			timer.Stop()
			return
		case <-timer.C:
			deleted, err := e.RunOnce(ctx)
			if err != nil {
				e.logger.LogAttrs(ctx, slog.LevelWarn,
					"events: retention pass failed",
					slog.Int("policy_count", len(e.policies)),
					slog.Any("error", err),
				)
				continue
			}
			if deleted > 0 {
				e.logger.LogAttrs(ctx, slog.LevelInfo,
					"events: retention pass",
					slog.Int("deleted", deleted),
					slog.Int("policy_count", len(e.policies)),
				)
			}
		}
	}
}

// nextWait returns the wait time for the next tick — `interval ±
// (jitter * interval)`. Zero jitter degenerates to the exact
// interval.
func (e *RetentionEnforcer) nextWait() time.Duration {
	if e.jitter <= 0 {
		return e.interval
	}
	jitterRange := float64(e.interval) * e.jitter // total ± range
	// #nosec G404 -- jitter is anti-thundering-herd timing, not security-sensitive
	offset := (rand.Float64()*2 - 1) * jitterRange
	d := time.Duration(float64(e.interval) + offset)
	if d <= 0 {
		// Defensive — should be unreachable given jitter ≤ 0.5.
		return e.interval
	}
	return d
}

// String makes the enforcer self-describing in operator logs.
func (e *RetentionEnforcer) String() string {
	return fmt.Sprintf("RetentionEnforcer{interval=%s, jitter=%.2f, policies=%d}",
		e.interval, e.jitter, len(e.policies))
}
