package identity

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"sync/atomic"
	"time"
)

// DefaultJoinTokenCleanupInterval is the §4.10 default polling
// cadence — hourly checks keep storage tidy without taxing the
// store with constant DELETE traffic. Expired tokens are still
// rejected by [JoinTokenAttestor.Attest] at attestation time
// (the ExpiresAt check runs per-request, per task 8), so a
// missed cleanup cycle is a storage-hygiene concern, not a
// security one.
const DefaultJoinTokenCleanupInterval = 1 * time.Hour

// ErrInvalidJoinTokenCleaner wraps every rejection in this file —
// constructor validation, lifecycle protocol violations.
var ErrInvalidJoinTokenCleaner = errors.New("identity: invalid JoinTokenCleaner")

// JoinTokenCleaner polls [JoinTokenStore.Cleanup] on a fixed
// interval, deleting records whose ExpiresAt is at or before
// "now". Failures are logged and the loop continues — the next
// tick retries. Closes the v0.1 join-token lifecycle: tasks
// 8-10 ship Create / Attest / Lookup; this loop reclaims the
// records they leave behind.
//
// Multi-CP HA (Epic 13): only the cluster leader should run
// cleanup so a 3-node cluster doesn't issue three DELETE
// transactions per hour. Wire [JoinTokenCleanerConfig.IsLeader]
// to gate on the cluster's election state; v0.1 single-CP
// leaves it nil (which means "always run").
//
// Lifecycle mirrors [CARotator]:
//
//	NewJoinTokenCleaner(cfg) ─► Start(ctx) ─► (running) ─► Stop(ctx)
//
// Start is one-shot — a stopped cleaner cannot be restarted.
// Build a fresh one for each restart. Stop is idempotent.
type JoinTokenCleaner struct {
	cfg      JoinTokenCleanerConfig
	cancel   context.CancelFunc
	wg       sync.WaitGroup
	started  atomic.Bool
	stopOnce sync.Once
}

// JoinTokenCleanerConfig drives [NewJoinTokenCleaner]. Store is
// required; everything else falls back to documented defaults.
type JoinTokenCleanerConfig struct {
	// Store is the JoinTokenStore the loop calls Cleanup on.
	// Required.
	Store JoinTokenStore

	// Interval is the polling cadence. Default
	// DefaultJoinTokenCleanupInterval (1h). Negative values
	// reject.
	Interval time.Duration

	// Clock returns "now" for the Cleanup cutoff. Tests inject;
	// production leaves nil so [time.Now] is used.
	Clock func() time.Time

	// Logger receives cleanup outcome messages. nil falls back
	// to [slog.Default].
	Logger *slog.Logger

	// IsLeader gates cleanup on the cluster's election state.
	// nil (the v0.1 single-CP default) means "always run." Epic
	// 13 wires this to the cluster's leader-election machinery
	// so followers skip cleanup without disabling the loop.
	IsLeader func() bool

	// OnTick is called (non-blocking) AFTER each tick completes,
	// regardless of leader-gating or cleanup outcome. Tests
	// subscribe to observe loop progress race-free; production
	// leaves it nil.
	OnTick func()
}

// NewJoinTokenCleaner validates cfg and returns a stopped
// cleaner. Call [JoinTokenCleaner.Start] to spawn the loop.
func NewJoinTokenCleaner(cfg JoinTokenCleanerConfig) (*JoinTokenCleaner, error) {
	if cfg.Store == nil {
		return nil, fmt.Errorf("%w: Store is required", ErrInvalidJoinTokenCleaner)
	}
	if cfg.Interval < 0 {
		return nil, fmt.Errorf("%w: Interval must be ≥ 0, got %s", ErrInvalidJoinTokenCleaner, cfg.Interval)
	}
	if cfg.Interval == 0 {
		cfg.Interval = DefaultJoinTokenCleanupInterval
	}
	if cfg.Clock == nil {
		cfg.Clock = time.Now
	}
	if cfg.Logger == nil {
		cfg.Logger = slog.Default()
	}
	return &JoinTokenCleaner{cfg: cfg}, nil
}

// Start spawns the polling loop in a goroutine and returns
// immediately. Subsequent Start calls return
// [ErrInvalidJoinTokenCleaner] — a cleaner is one-shot.
//
// The loop runs until [JoinTokenCleaner.Stop] is called or
// `ctx` is canceled.
func (c *JoinTokenCleaner) Start(ctx context.Context) error {
	if !c.started.CompareAndSwap(false, true) {
		return fmt.Errorf("%w: already started", ErrInvalidJoinTokenCleaner)
	}
	loopCtx, cancel := context.WithCancel(ctx)
	c.cancel = cancel
	c.wg.Add(1)
	go c.run(loopCtx)
	return nil
}

// Stop signals the loop to exit and waits for the goroutine to
// finish. Safe to call without a prior Start (no-op). Safe to
// call multiple times. `ctx` bounds the wait — a hung in-flight
// Cleanup returns the wrapped [context.DeadlineExceeded] when
// the passed ctx expires.
func (c *JoinTokenCleaner) Stop(ctx context.Context) error {
	c.stopOnce.Do(func() {
		if c.cancel != nil {
			c.cancel()
		}
	})

	done := make(chan struct{})
	go func() {
		c.wg.Wait()
		close(done)
	}()
	select {
	case <-done:
		return nil
	case <-ctx.Done():
		return fmt.Errorf("%w: stop wait: %w", ErrInvalidJoinTokenCleaner, ctx.Err())
	}
}

func (c *JoinTokenCleaner) run(ctx context.Context) {
	defer c.wg.Done()

	ticker := time.NewTicker(c.cfg.Interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			c.tick(ctx)
		}
	}
}

// tick is one cycle of the cleanup loop. Extracted so future
// metrics / tracing wraps cleanly around it.
func (c *JoinTokenCleaner) tick(ctx context.Context) {
	defer func() {
		if c.cfg.OnTick != nil {
			c.cfg.OnTick()
		}
	}()

	if c.cfg.IsLeader != nil && !c.cfg.IsLeader() {
		// Follower — skip; the leader handles cleanup. Log at
		// DEBUG so followers don't spam INFO every hour.
		c.cfg.Logger.Debug("join-token cleanup skipped — not leader")
		return
	}

	now := c.cfg.Clock()
	n, err := c.cfg.Store.Cleanup(ctx, now)
	if err != nil {
		c.cfg.Logger.Error("join-token cleanup failed; will retry at next tick",
			"err", err,
			"interval", c.cfg.Interval,
		)
		return
	}
	if n > 0 {
		c.cfg.Logger.Info("join-token cleanup", "removed", n)
	}
}
