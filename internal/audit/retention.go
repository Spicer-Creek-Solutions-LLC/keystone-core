package audit

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

// LeaderCheck reports whether the calling node is currently the
// cluster leader. The [RetentionEnforcer] consults this before
// running each tick — only the leader actually deletes rows, so
// multi-node deployments don't race on the same row set.
//
// Mirrors `internal/events.LeaderCheck`. Epic 13 (Clustering / HA)
// ships the production leader-election impl. v1.0 single-node
// deployments use the default [AlwaysLeader].
type LeaderCheck func() bool

// AlwaysLeader is the v1.0 default — single-node deployments
// don't have an election to consult.
func AlwaysLeader() bool { return true }

// RetentionEnforcer runs [AuditStore.ApplyRetention] on a
// configurable interval per §4.12. Mirrors Epic 11 task 8's
// `events.RetentionEnforcer` shape; audit's single-policy model
// (vs events' policy list) is the only structural difference.
//
// Lifecycle: Start once, then Stop once. RunOnce works regardless
// of scheduler state — operators drive a manual pass from the
// future task-14 CLI.
type RetentionEnforcer struct {
	store       AuditStore
	policy      RetentionPolicy
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

// RetentionEnforcerOption configures a [RetentionEnforcer]. Required:
// [WithRetentionStore]. Every other knob has a sensible default.
type RetentionEnforcerOption func(*retentionConfig)

type retentionConfig struct {
	store       AuditStore
	policy      RetentionPolicy
	interval    time.Duration
	jitter      float64
	leaderCheck LeaderCheck
	logger      *slog.Logger
}

func defaultRetentionConfig() retentionConfig {
	return retentionConfig{
		policy:      DefaultRetentionPolicy(),
		interval:    DefaultRetentionInterval,
		jitter:      DefaultRetentionJitter,
		leaderCheck: AlwaysLeader,
		logger:      slog.Default(),
	}
}

// WithRetentionStore wires the [AuditStore]. Required.
func WithRetentionStore(s AuditStore) RetentionEnforcerOption {
	return func(c *retentionConfig) { c.store = s }
}

// WithRetentionPolicy overrides the retention policy. Defaults to
// [DefaultRetentionPolicy] (90d / 100k / no MinSeverity / 1h).
func WithRetentionPolicy(p RetentionPolicy) RetentionEnforcerOption {
	return func(c *retentionConfig) { c.policy = p }
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

// WithRetentionJitter sets the ±fraction-of-interval randomisation.
// Out-of-range (negative or > 0.5) falls back to default.
func WithRetentionJitter(fraction float64) RetentionEnforcerOption {
	return func(c *retentionConfig) {
		if fraction >= 0 && fraction <= 0.5 {
			c.jitter = fraction
		}
	}
}

// WithRetentionLeaderCheck supplies the leader-election predicate.
// Nil falls back to [AlwaysLeader].
func WithRetentionLeaderCheck(fn LeaderCheck) RetentionEnforcerOption {
	return func(c *retentionConfig) {
		if fn != nil {
			c.leaderCheck = fn
		}
	}
}

// WithRetentionLogger overrides the slog.Logger. Nil leaves the
// default.
func WithRetentionLogger(l *slog.Logger) RetentionEnforcerOption {
	return func(c *retentionConfig) {
		if l != nil {
			c.logger = l
		}
	}
}

// NewRetentionEnforcer builds an enforcer from options. Returns an
// error when the store is missing.
func NewRetentionEnforcer(opts ...RetentionEnforcerOption) (*RetentionEnforcer, error) {
	cfg := defaultRetentionConfig()
	for _, opt := range opts {
		opt(&cfg)
	}
	if cfg.store == nil {
		return nil, errors.New("audit: retention: store is required (use WithRetentionStore)")
	}
	return &RetentionEnforcer{
		store:       cfg.store,
		policy:      cfg.policy,
		interval:    cfg.interval,
		jitter:      cfg.jitter,
		leaderCheck: cfg.leaderCheck,
		logger:      cfg.logger,
	}, nil
}

// Start launches the scheduler goroutine. Double-Start rejected.
// First tick fires after `interval ± jitter` — no boot storm.
func (e *RetentionEnforcer) Start(_ context.Context) error {
	if !e.started.CompareAndSwap(false, true) {
		return errors.New("audit: retention: already started")
	}
	e.workerCtx, e.cancel = context.WithCancel(context.Background())
	e.wg.Add(1)
	go e.run(e.workerCtx)
	return nil
}

// Stop signals the scheduler to exit and waits for the current
// tick (if any) to complete OR for the caller's context to
// expire. Idempotent.
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
// silently (returns 0, nil) when the leader check reports
// non-leader. Available regardless of scheduler state.
func (e *RetentionEnforcer) RunOnce(ctx context.Context) (int, error) {
	if !e.leaderCheck() {
		return 0, nil
	}
	deleted, err := e.store.ApplyRetention(ctx, e.policy)
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
// completed retention pass.
func (e *RetentionEnforcer) LastRunAt() time.Time {
	if t := e.lastRunAt.Load(); t != nil {
		return *t
	}
	return time.Time{}
}

// LastRunDeleted returns the deletion count from the most recent
// successful pass.
func (e *RetentionEnforcer) LastRunDeleted() int64 {
	return e.lastRunDeleted.Load()
}

// TotalDeleted returns cumulative deletions since process start.
func (e *RetentionEnforcer) TotalDeleted() int64 {
	return e.totalDeleted.Load()
}

// RunsFailed returns cumulative failed-pass count.
func (e *RetentionEnforcer) RunsFailed() int64 {
	return e.runsFailed.Load()
}

// run is the scheduler goroutine. First tick fires AFTER the
// first jittered interval (no boot storm).
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
					"audit: retention pass failed",
					slog.Any("error", err),
				)
				continue
			}
			if deleted > 0 {
				e.logger.LogAttrs(ctx, slog.LevelInfo,
					"audit: retention pass",
					slog.Int("deleted", deleted),
				)
			}
		}
	}
}

// nextWait returns the wait time for the next tick. Zero jitter
// degenerates to the exact interval.
func (e *RetentionEnforcer) nextWait() time.Duration {
	if e.jitter <= 0 {
		return e.interval
	}
	jitterRange := float64(e.interval) * e.jitter
	//nolint:gosec // anti-thundering-herd timing, not security-sensitive
	offset := (rand.Float64()*2 - 1) * jitterRange
	d := time.Duration(float64(e.interval) + offset)
	if d <= 0 {
		return e.interval
	}
	return d
}

// String makes the enforcer self-describing in operator logs.
func (e *RetentionEnforcer) String() string {
	return fmt.Sprintf("AuditRetentionEnforcer{interval=%s, jitter=%.2f, max_age=%s, max_count=%d, min_severity=%s}",
		e.interval, e.jitter, e.policy.MaxAge, e.policy.MaxCount, e.policy.MinSeverity)
}
