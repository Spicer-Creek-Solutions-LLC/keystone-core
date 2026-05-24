// SPDX-License-Identifier: Apache-2.0

package identity

import (
	"context"
	"crypto"
	"crypto/x509"
	"errors"
	"io"
	"log/slog"
	"sync/atomic"
	"testing"
	"time"
)

// silentLogger discards everything below ERROR so test output stays
// quiet unless rotation breaks unexpectedly.
func silentLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, &slog.HandlerOptions{Level: slog.LevelError}))
}

// ---- NewCARotator ------------------------------------------------

func TestNewCARotator_RejectsNilManager(t *testing.T) {
	t.Parallel()
	_, err := NewCARotator(CARotatorConfig{})
	if err == nil || !errors.Is(err, ErrInvalidCARotator) {
		t.Fatalf("err = %v", err)
	}
}

func TestNewCARotator_RejectsNegativeInterval(t *testing.T) {
	t.Parallel()
	m, _ := newInitializedManager(t, newFastCAConfig(DefaultTrustDomain))
	_, err := NewCARotator(CARotatorConfig{
		Manager:  m,
		Interval: -time.Second,
	})
	if err == nil || !errors.Is(err, ErrInvalidCARotator) {
		t.Fatalf("err = %v", err)
	}
}

func TestNewCARotator_DefaultsInterval(t *testing.T) {
	t.Parallel()
	m, _ := newInitializedManager(t, newFastCAConfig(DefaultTrustDomain))
	r, err := NewCARotator(CARotatorConfig{Manager: m})
	if err != nil {
		t.Fatalf("NewCARotator: %v", err)
	}
	if r.cfg.Interval != DefaultCARotatorInterval {
		t.Errorf("Interval = %s, want %s", r.cfg.Interval, DefaultCARotatorInterval)
	}
}

func TestNewCARotator_DefaultsClockAndLogger(t *testing.T) {
	t.Parallel()
	m, _ := newInitializedManager(t, newFastCAConfig(DefaultTrustDomain))
	r, err := NewCARotator(CARotatorConfig{Manager: m, Interval: time.Hour})
	if err != nil {
		t.Fatalf("NewCARotator: %v", err)
	}
	if r.cfg.Clock == nil {
		t.Error("Clock not defaulted")
	}
	if r.cfg.Logger == nil {
		t.Error("Logger not defaulted")
	}
}

// ---- Lifecycle ---------------------------------------------------

// waitForTicks subscribes via OnTick + blocks until n signals arrive
// or `timeout` elapses. Race-free way to know the loop has fired.
func waitForTicks(t *testing.T, ch <-chan struct{}, n int, timeout time.Duration) {
	t.Helper()
	deadline := time.After(timeout)
	for i := 0; i < n; i++ {
		select {
		case <-ch:
		case <-deadline:
			t.Fatalf("only saw %d/%d ticks before deadline", i, n)
		}
	}
}

func TestCARotator_StartStop(t *testing.T) {
	t.Parallel()
	m, _ := newInitializedManager(t, newFastCAConfig(DefaultTrustDomain))
	ticks := make(chan struct{}, 16)
	r, err := NewCARotator(CARotatorConfig{
		Manager:  m,
		Interval: 5 * time.Millisecond,
		Logger:   silentLogger(),
		OnTick:   func() { ticks <- struct{}{} },
	})
	if err != nil {
		t.Fatalf("NewCARotator: %v", err)
	}
	if err := r.Start(context.Background()); err != nil {
		t.Fatalf("Start: %v", err)
	}
	waitForTicks(t, ticks, 2, time.Second)

	stopCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := r.Stop(stopCtx); err != nil {
		t.Fatalf("Stop: %v", err)
	}
}

func TestCARotator_DoubleStart(t *testing.T) {
	t.Parallel()
	m, _ := newInitializedManager(t, newFastCAConfig(DefaultTrustDomain))
	r, _ := NewCARotator(CARotatorConfig{
		Manager:  m,
		Interval: time.Hour,
		Logger:   silentLogger(),
	})
	if err := r.Start(context.Background()); err != nil {
		t.Fatalf("first Start: %v", err)
	}
	t.Cleanup(func() { _ = r.Stop(context.Background()) })

	if err := r.Start(context.Background()); err == nil || !errors.Is(err, ErrInvalidCARotator) {
		t.Errorf("second Start err = %v, want ErrInvalidCARotator", err)
	}
}

func TestCARotator_StopWithoutStart(t *testing.T) {
	t.Parallel()
	m, _ := newInitializedManager(t, newFastCAConfig(DefaultTrustDomain))
	r, _ := NewCARotator(CARotatorConfig{
		Manager:  m,
		Interval: time.Hour,
		Logger:   silentLogger(),
	})
	if err := r.Stop(context.Background()); err != nil {
		t.Errorf("Stop without Start should be safe; got %v", err)
	}
}

func TestCARotator_DoubleStop(t *testing.T) {
	t.Parallel()
	m, _ := newInitializedManager(t, newFastCAConfig(DefaultTrustDomain))
	r, _ := NewCARotator(CARotatorConfig{
		Manager:  m,
		Interval: 5 * time.Millisecond,
		Logger:   silentLogger(),
	})
	if err := r.Start(context.Background()); err != nil {
		t.Fatalf("Start: %v", err)
	}
	if err := r.Stop(context.Background()); err != nil {
		t.Errorf("first Stop: %v", err)
	}
	// Second Stop must be a no-op.
	if err := r.Stop(context.Background()); err != nil {
		t.Errorf("second Stop: %v", err)
	}
}

func TestCARotator_CtxCancellationStopsLoop(t *testing.T) {
	t.Parallel()
	m, _ := newInitializedManager(t, newFastCAConfig(DefaultTrustDomain))
	ticks := make(chan struct{}, 16)
	r, _ := NewCARotator(CARotatorConfig{
		Manager:  m,
		Interval: 5 * time.Millisecond,
		Logger:   silentLogger(),
		OnTick:   func() { ticks <- struct{}{} },
	})

	ctx, cancel := context.WithCancel(context.Background())
	if err := r.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}
	waitForTicks(t, ticks, 1, time.Second)
	cancel()

	// Stop must return cleanly because the goroutine exits on ctx.
	if err := r.Stop(context.Background()); err != nil {
		t.Errorf("Stop after ctx cancel: %v", err)
	}
}

// ---- Rotation behavior -------------------------------------------

func TestCARotator_DoesNotRotateWhenPredicateFalse(t *testing.T) {
	t.Parallel()
	c := newFastCAConfig(DefaultTrustDomain)
	// Clock fixed at issuance time → ShouldRotateSigningCA = false.
	frozen := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	c.Clock = func() time.Time { return frozen }
	m, _ := newInitializedManager(t, c)
	signingBefore := m.signingCert

	ticks := make(chan struct{}, 16)
	r, _ := NewCARotator(CARotatorConfig{
		Manager:  m,
		Interval: 5 * time.Millisecond,
		Clock:    c.Clock, // rotator + manager share the frozen clock
		Logger:   silentLogger(),
		OnTick:   func() { ticks <- struct{}{} },
	})
	if err := r.Start(context.Background()); err != nil {
		t.Fatalf("Start: %v", err)
	}
	t.Cleanup(func() { _ = r.Stop(context.Background()) })

	waitForTicks(t, ticks, 3, time.Second)

	if m.signingCert != signingBefore {
		t.Error("signing CA was rotated even though predicate was false")
	}
}

func TestCARotator_RotatesWhenPredicateTrue(t *testing.T) {
	t.Parallel()
	c := newFastCAConfig(DefaultTrustDomain)
	frozen := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	c.Clock = func() time.Time { return frozen }
	m, _ := newInitializedManager(t, c)
	signingBefore := m.signingCert

	// Advance the clock past the rotate-before boundary.
	// Signing CA NotAfter = frozen + 2h; RotateBefore = 30m;
	// rotation window opens at frozen + 1h30m.
	advanced := frozen.Add(time.Hour + 31*time.Minute)
	c.Clock = func() time.Time { return advanced }
	// Re-point the manager's clock so RotateSigningCA itself uses
	// the new now when minting the replacement.
	m.cfg.Clock = c.Clock

	ticks := make(chan struct{}, 16)
	r, _ := NewCARotator(CARotatorConfig{
		Manager:  m,
		Interval: 5 * time.Millisecond,
		Clock:    c.Clock,
		Logger:   silentLogger(),
		OnTick:   func() { ticks <- struct{}{} },
	})
	if err := r.Start(context.Background()); err != nil {
		t.Fatalf("Start: %v", err)
	}
	t.Cleanup(func() { _ = r.Stop(context.Background()) })

	// One tick is enough to fire the rotation; observe two to be
	// sure (first might race with Start's goroutine setup).
	waitForTicks(t, ticks, 2, time.Second)

	if m.signingCert == signingBefore {
		t.Error("signing CA was NOT rotated despite predicate being true")
	}
}

// ---- Error resilience --------------------------------------------

// armedSaveFailer wraps a permissive CAStorage and unconditionally
// fails SaveSigningCA. Lets a test Initialize a real CA pair, then
// swap in this wrapper before starting the rotation loop so every
// rotation attempt errors but the loop must keep ticking.
type armedSaveFailer struct {
	CAStorage
	err error
}

func (a *armedSaveFailer) SaveSigningCA(c *x509.Certificate, k crypto.Signer) error {
	return a.err
}

func TestCARotator_StorageErrorDoesNotKillLoop(t *testing.T) {
	t.Parallel()
	c := newFastCAConfig(DefaultTrustDomain)
	frozen := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	c.Clock = func() time.Time { return frozen }
	advanced := frozen.Add(time.Hour + 31*time.Minute)

	// Initialize with a permissive storage, then wrap it in an
	// armed failer so SaveSigningCA fails only when rotation tries
	// to persist.
	base := newTempStorage(t)
	m, err := NewCAManager(c, base)
	if err != nil {
		t.Fatalf("NewCAManager: %v", err)
	}
	if err := m.Initialize(context.Background()); err != nil {
		t.Fatalf("Initialize: %v", err)
	}
	// Swap in the failing storage now that init is complete.
	m.storage = &armedSaveFailer{
		CAStorage: base,
		err:       errors.New("synthetic disk failure"),
	}
	m.cfg.Clock = func() time.Time { return advanced }

	var failureCount atomic.Int64
	ticks := make(chan struct{}, 16)
	r, _ := NewCARotator(CARotatorConfig{
		Manager:  m,
		Interval: 5 * time.Millisecond,
		Clock:    func() time.Time { return advanced },
		Logger: slog.New(slog.NewTextHandler(io.Discard, &slog.HandlerOptions{
			Level: slog.LevelDebug,
		})),
		OnTick: func() {
			// Count completed ticks; rotation failures still
			// fire OnTick.
			failureCount.Add(1)
			ticks <- struct{}{}
		},
	})
	if err := r.Start(context.Background()); err != nil {
		t.Fatalf("Start: %v", err)
	}
	t.Cleanup(func() { _ = r.Stop(context.Background()) })

	// At least three ticks must complete even though each rotation
	// fails. If the loop died on the first error, this hits the
	// deadline.
	waitForTicks(t, ticks, 3, time.Second)
	if got := failureCount.Load(); got < 3 {
		t.Errorf("ticks completed = %d, want ≥ 3 (loop dropped out on storage error)", got)
	}
}

func TestCARotator_StopCtxDeadlineExceededReturnsError(t *testing.T) {
	t.Parallel()
	m, _ := newInitializedManager(t, newFastCAConfig(DefaultTrustDomain))

	// OnTick blocks the loop indefinitely — Stop's ctx will fire
	// before the goroutine can exit.
	release := make(chan struct{})
	t.Cleanup(func() { close(release) }) // unblock at test end so the goroutine exits cleanly
	tickStarted := make(chan struct{}, 1)

	r, _ := NewCARotator(CARotatorConfig{
		Manager:  m,
		Interval: 5 * time.Millisecond,
		Logger:   silentLogger(),
		OnTick: func() {
			select {
			case tickStarted <- struct{}{}:
			default:
			}
			<-release
		},
	})
	if err := r.Start(context.Background()); err != nil {
		t.Fatalf("Start: %v", err)
	}
	// Wait until the loop is parked inside OnTick.
	select {
	case <-tickStarted:
	case <-time.After(time.Second):
		t.Fatal("loop never entered OnTick")
	}

	stopCtx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	err := r.Stop(stopCtx)
	if err == nil || !errors.Is(err, ErrInvalidCARotator) {
		t.Errorf("Stop err = %v, want wrapped ErrInvalidCARotator", err)
	}
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("Stop err = %v, want context.DeadlineExceeded chained", err)
	}
}
