// SPDX-License-Identifier: Apache-2.0

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

// DefaultCARotatorInterval is the §4.10 default polling cadence —
// hourly checks keep the rotation deadline tight enough that a
// missed tick (e.g. during a kscore-server restart) doesn't push
// the signing CA past expiry.
const DefaultCARotatorInterval = 1 * time.Hour

// ErrInvalidCARotator wraps every rejection in this file —
// config validation, lifecycle protocol errors.
var ErrInvalidCARotator = errors.New("identity: invalid CARotator")

// CARotator polls a [*CAManager] on a fixed interval. When
// [CAManager.ShouldRotateSigningCA] flips true the rotator invokes
// [CAManager.RotateSigningCA]. Storage / generation failures are
// logged and the loop continues — the next tick retries. The
// signing CA is still serviceable until its NotAfter, so missing
// one rotation cycle is recoverable.
//
// Lifecycle:
//
//	NewCARotator(cfg) ─► Start(ctx) ─► (running) ─► Stop(ctx)
//
// Start is one-shot — a stopped rotator cannot be restarted.
// Build a fresh CARotator for each restart. Stop is idempotent.
//
// The agent-driven X509SVID auto-rotation (~50% of leaf lifetime,
// per §4.10) is a separate cadence on a separate actor — that
// wiring lands in the agent runtime, not here.
type CARotator struct {
	cfg     CARotatorConfig
	cancel  context.CancelFunc
	wg      sync.WaitGroup
	started atomic.Bool
	stopOnce sync.Once
}

// CARotatorConfig drives [NewCARotator]. Manager is required;
// all other fields fall back to documented defaults when zero.
type CARotatorConfig struct {
	// Manager is the CA the loop polls + rotates. Required.
	Manager *CAManager

	// Interval is the polling cadence. Default
	// DefaultCARotatorInterval (1h). Reject negative values.
	Interval time.Duration

	// Clock returns "now" for the [CAManager.ShouldRotateSigningCA]
	// predicate. Tests inject a deterministic clock; production
	// leaves it nil so [time.Now] is used.
	Clock func() time.Time

	// Logger receives rotation success + failure messages. nil
	// falls back to [slog.Default].
	Logger *slog.Logger

	// OnTick is called (non-blocking) AFTER each tick completes,
	// regardless of whether a rotation fired. Tests subscribe to
	// observe loop progress race-free; production leaves it nil.
	OnTick func()

	// OnRotateSuccess is called (non-blocking) only after a
	// rotation completes without error. EmbeddedProvider (task 7)
	// hooks this to rebuild + republish the trust bundle to its
	// watchers. nil in rotators that don't need a post-rotation
	// hook.
	OnRotateSuccess func()
}

// NewCARotator validates cfg and returns a stopped rotator. Call
// [CARotator.Start] to spawn the loop.
func NewCARotator(cfg CARotatorConfig) (*CARotator, error) {
	if cfg.Manager == nil {
		return nil, fmt.Errorf("%w: Manager is required", ErrInvalidCARotator)
	}
	if cfg.Interval < 0 {
		return nil, fmt.Errorf("%w: Interval must be ≥ 0, got %s", ErrInvalidCARotator, cfg.Interval)
	}
	if cfg.Interval == 0 {
		cfg.Interval = DefaultCARotatorInterval
	}
	if cfg.Clock == nil {
		cfg.Clock = time.Now
	}
	if cfg.Logger == nil {
		cfg.Logger = slog.Default()
	}
	return &CARotator{cfg: cfg}, nil
}

// Start spawns the polling loop in a goroutine and returns
// immediately. Subsequent Start calls return ErrInvalidCARotator
// — a CARotator is one-shot.
//
// The loop runs until [CARotator.Stop] is called or `ctx` is
// canceled. [CARotator.Stop] is the authoritative shutdown path
// (waits for the goroutine); ctx cancellation is supported as a
// convenience for callers that already share a root ctx with the
// rest of their stack.
func (r *CARotator) Start(ctx context.Context) error {
	if !r.started.CompareAndSwap(false, true) {
		return fmt.Errorf("%w: already started", ErrInvalidCARotator)
	}
	loopCtx, cancel := context.WithCancel(ctx)
	r.cancel = cancel
	r.wg.Add(1)
	go r.run(loopCtx)
	return nil
}

// Stop signals the loop to exit and waits for the goroutine to
// finish. Safe to call without a prior Start (no-op). Safe to call
// multiple times (subsequent calls are no-ops).
//
// `ctx` bounds the wait — if the loop's in-flight rotation hangs
// for some reason, Stop returns when ctx is canceled even if the
// goroutine is still draining. The goroutine itself observes the
// internal cancellation and exits at the next loop iteration.
func (r *CARotator) Stop(ctx context.Context) error {
	r.stopOnce.Do(func() {
		if r.cancel != nil {
			r.cancel()
		}
	})

	done := make(chan struct{})
	go func() {
		r.wg.Wait()
		close(done)
	}()
	select {
	case <-done:
		return nil
	case <-ctx.Done():
		return fmt.Errorf("%w: stop wait: %w", ErrInvalidCARotator, ctx.Err())
	}
}

func (r *CARotator) run(ctx context.Context) {
	defer r.wg.Done()

	ticker := time.NewTicker(r.cfg.Interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			r.tick(ctx)
		}
	}
}

// tick is one cycle of the rotation loop. Extracted so any future
// metrics / tracing wraps cleanly around it.
func (r *CARotator) tick(ctx context.Context) {
	defer func() {
		if r.cfg.OnTick != nil {
			r.cfg.OnTick()
		}
	}()

	now := r.cfg.Clock()
	if !r.cfg.Manager.ShouldRotateSigningCA(now) {
		return
	}
	if err := r.cfg.Manager.RotateSigningCA(ctx); err != nil {
		// Don't return — next tick retries. A transient disk
		// failure isn't grounds to abandon rotation; the signing
		// CA is still serviceable until expiry.
		r.cfg.Logger.Error("ca rotation failed; will retry at next tick",
			"err", err,
			"interval", r.cfg.Interval,
		)
		return
	}
	r.cfg.Logger.Info("ca rotation succeeded",
		"now", now,
	)
	if r.cfg.OnRotateSuccess != nil {
		r.cfg.OnRotateSuccess()
	}
}
