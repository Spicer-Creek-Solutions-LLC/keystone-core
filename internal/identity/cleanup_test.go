package identity

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"
)

// ---- NewJoinTokenCleaner ----------------------------------------

func TestNewJoinTokenCleaner_RejectsNilStore(t *testing.T) {
	t.Parallel()
	_, err := NewJoinTokenCleaner(JoinTokenCleanerConfig{})
	if err == nil || !errors.Is(err, ErrInvalidJoinTokenCleaner) {
		t.Fatalf("err = %v", err)
	}
}

func TestNewJoinTokenCleaner_RejectsNegativeInterval(t *testing.T) {
	t.Parallel()
	_, err := NewJoinTokenCleaner(JoinTokenCleanerConfig{
		Store:    NewInMemoryJoinTokenStore(),
		Interval: -time.Second,
	})
	if err == nil || !errors.Is(err, ErrInvalidJoinTokenCleaner) {
		t.Fatalf("err = %v", err)
	}
}

func TestNewJoinTokenCleaner_DefaultsInterval(t *testing.T) {
	t.Parallel()
	c, err := NewJoinTokenCleaner(JoinTokenCleanerConfig{
		Store: NewInMemoryJoinTokenStore(),
	})
	if err != nil {
		t.Fatalf("NewJoinTokenCleaner: %v", err)
	}
	if c.cfg.Interval != DefaultJoinTokenCleanupInterval {
		t.Errorf("Interval = %s, want %s", c.cfg.Interval, DefaultJoinTokenCleanupInterval)
	}
	if c.cfg.Clock == nil {
		t.Error("Clock not defaulted")
	}
	if c.cfg.Logger == nil {
		t.Error("Logger not defaulted")
	}
}

// ---- Lifecycle ---------------------------------------------------

func TestJoinTokenCleaner_StartStop(t *testing.T) {
	t.Parallel()
	ticks := make(chan struct{}, 16)
	c, _ := NewJoinTokenCleaner(JoinTokenCleanerConfig{
		Store:    NewInMemoryJoinTokenStore(),
		Interval: 5 * time.Millisecond,
		Logger:   silentLogger(),
		OnTick:   func() { ticks <- struct{}{} },
	})
	if err := c.Start(context.Background()); err != nil {
		t.Fatalf("Start: %v", err)
	}
	waitForTicks(t, ticks, 2, time.Second)

	stopCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := c.Stop(stopCtx); err != nil {
		t.Fatalf("Stop: %v", err)
	}
}

func TestJoinTokenCleaner_DoubleStart(t *testing.T) {
	t.Parallel()
	c, _ := NewJoinTokenCleaner(JoinTokenCleanerConfig{
		Store:    NewInMemoryJoinTokenStore(),
		Interval: time.Hour,
		Logger:   silentLogger(),
	})
	if err := c.Start(context.Background()); err != nil {
		t.Fatalf("first Start: %v", err)
	}
	t.Cleanup(func() { _ = c.Stop(context.Background()) })

	if err := c.Start(context.Background()); err == nil || !errors.Is(err, ErrInvalidJoinTokenCleaner) {
		t.Errorf("second Start err = %v", err)
	}
}

func TestJoinTokenCleaner_StopWithoutStart(t *testing.T) {
	t.Parallel()
	c, _ := NewJoinTokenCleaner(JoinTokenCleanerConfig{
		Store:    NewInMemoryJoinTokenStore(),
		Interval: time.Hour,
		Logger:   silentLogger(),
	})
	if err := c.Stop(context.Background()); err != nil {
		t.Errorf("Stop without Start: %v", err)
	}
}

func TestJoinTokenCleaner_DoubleStop(t *testing.T) {
	t.Parallel()
	c, _ := NewJoinTokenCleaner(JoinTokenCleanerConfig{
		Store:    NewInMemoryJoinTokenStore(),
		Interval: 5 * time.Millisecond,
		Logger:   silentLogger(),
	})
	if err := c.Start(context.Background()); err != nil {
		t.Fatalf("Start: %v", err)
	}
	if err := c.Stop(context.Background()); err != nil {
		t.Errorf("first Stop: %v", err)
	}
	if err := c.Stop(context.Background()); err != nil {
		t.Errorf("second Stop: %v", err)
	}
}

func TestJoinTokenCleaner_CtxCancellationStopsLoop(t *testing.T) {
	t.Parallel()
	ticks := make(chan struct{}, 16)
	c, _ := NewJoinTokenCleaner(JoinTokenCleanerConfig{
		Store:    NewInMemoryJoinTokenStore(),
		Interval: 5 * time.Millisecond,
		Logger:   silentLogger(),
		OnTick:   func() { ticks <- struct{}{} },
	})

	ctx, cancel := context.WithCancel(context.Background())
	if err := c.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}
	waitForTicks(t, ticks, 1, time.Second)
	cancel()

	if err := c.Stop(context.Background()); err != nil {
		t.Errorf("Stop after ctx cancel: %v", err)
	}
}

// ---- Cleanup actually invoked ------------------------------------

func TestJoinTokenCleaner_CleanupRemovesExpired(t *testing.T) {
	t.Parallel()
	store := NewInMemoryJoinTokenStore()
	ctx := context.Background()
	now := time.Now().Truncate(time.Second)

	expiredA := sampleInMemToken("exp-a", "kscore-join-EXPIRED1")
	expiredA.ExpiresAt = now.Add(-2 * time.Hour)
	expiredB := sampleInMemToken("exp-b", "kscore-join-EXPIRED2")
	expiredB.ExpiresAt = now.Add(-time.Hour)
	live := sampleInMemToken("live", "kscore-join-LIVE0001")
	live.ExpiresAt = now.Add(time.Hour)

	for _, tok := range []JoinToken{expiredA, expiredB, live} {
		if err := store.Create(ctx, tok); err != nil {
			t.Fatalf("Create %s: %v", tok.ID, err)
		}
	}

	ticks := make(chan struct{}, 16)
	c, _ := NewJoinTokenCleaner(JoinTokenCleanerConfig{
		Store:    store,
		Interval: 5 * time.Millisecond,
		Clock:    func() time.Time { return now },
		Logger:   silentLogger(),
		OnTick:   func() { ticks <- struct{}{} },
	})
	if err := c.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}
	t.Cleanup(func() { _ = c.Stop(ctx) })

	// One tick is enough; observe two to be sure.
	waitForTicks(t, ticks, 2, time.Second)

	got, err := store.List(ctx, ListJoinTokensFilter{})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(got) != 1 || got[0].ID != "live" {
		t.Errorf("post-cleanup list = %v, want [live]", got)
	}
}

// ---- Error resilience --------------------------------------------

// failingCleanupStore makes Cleanup return a synthetic error on
// every call. All other methods delegate to an inner store so
// pre-test seeding works.
type failingCleanupStore struct {
	*InMemoryJoinTokenStore
	err error
}

func (f *failingCleanupStore) Cleanup(_ context.Context, _ time.Time) (int, error) {
	return 0, f.err
}

func TestJoinTokenCleaner_StorageErrorDoesNotKillLoop(t *testing.T) {
	t.Parallel()
	store := &failingCleanupStore{
		InMemoryJoinTokenStore: NewInMemoryJoinTokenStore(),
		err:                    errors.New("synthetic disk failure"),
	}
	ticks := make(chan struct{}, 16)
	var tickCount atomic.Int64
	c, _ := NewJoinTokenCleaner(JoinTokenCleanerConfig{
		Store:    store,
		Interval: 5 * time.Millisecond,
		Logger:   silentLogger(),
		OnTick: func() {
			tickCount.Add(1)
			ticks <- struct{}{}
		},
	})
	if err := c.Start(context.Background()); err != nil {
		t.Fatalf("Start: %v", err)
	}
	t.Cleanup(func() { _ = c.Stop(context.Background()) })

	// Must see ≥ 3 ticks even though every Cleanup call errors.
	waitForTicks(t, ticks, 3, time.Second)
	if got := tickCount.Load(); got < 3 {
		t.Errorf("tickCount = %d, want ≥ 3 (loop dropped out on storage error)", got)
	}
}

// ---- Leader gating -----------------------------------------------

func TestJoinTokenCleaner_FollowerSkipsCleanup(t *testing.T) {
	t.Parallel()
	store := NewInMemoryJoinTokenStore()
	ctx := context.Background()
	now := time.Now().Truncate(time.Second)
	expired := sampleInMemToken("exp", "kscore-join-EXP00001")
	expired.ExpiresAt = now.Add(-time.Hour)
	if err := store.Create(ctx, expired); err != nil {
		t.Fatalf("Create: %v", err)
	}

	ticks := make(chan struct{}, 16)
	c, _ := NewJoinTokenCleaner(JoinTokenCleanerConfig{
		Store:    store,
		Interval: 5 * time.Millisecond,
		Clock:    func() time.Time { return now },
		Logger:   silentLogger(),
		IsLeader: func() bool { return false }, // ← never leader
		OnTick:   func() { ticks <- struct{}{} },
	})
	if err := c.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}
	t.Cleanup(func() { _ = c.Stop(ctx) })

	// Wait for several ticks; the expired token must still exist.
	waitForTicks(t, ticks, 3, time.Second)

	got, _ := store.List(ctx, ListJoinTokensFilter{})
	if len(got) != 1 {
		t.Errorf("follower cleaned up records anyway; list = %v", got)
	}
}

func TestJoinTokenCleaner_LeaderRunsCleanup(t *testing.T) {
	t.Parallel()
	store := NewInMemoryJoinTokenStore()
	ctx := context.Background()
	now := time.Now().Truncate(time.Second)
	expired := sampleInMemToken("exp", "kscore-join-EXP00001")
	expired.ExpiresAt = now.Add(-time.Hour)
	if err := store.Create(ctx, expired); err != nil {
		t.Fatalf("Create: %v", err)
	}

	ticks := make(chan struct{}, 16)
	c, _ := NewJoinTokenCleaner(JoinTokenCleanerConfig{
		Store:    store,
		Interval: 5 * time.Millisecond,
		Clock:    func() time.Time { return now },
		Logger:   silentLogger(),
		IsLeader: func() bool { return true }, // ← explicit leader
		OnTick:   func() { ticks <- struct{}{} },
	})
	if err := c.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}
	t.Cleanup(func() { _ = c.Stop(ctx) })

	waitForTicks(t, ticks, 2, time.Second)
	got, _ := store.List(ctx, ListJoinTokensFilter{})
	if len(got) != 0 {
		t.Errorf("leader didn't run cleanup; list = %v", got)
	}
}

// ---- Stop ctx deadline ------------------------------------------

func TestJoinTokenCleaner_StopCtxDeadlineExceededReturnsError(t *testing.T) {
	t.Parallel()
	release := make(chan struct{})
	t.Cleanup(func() { close(release) }) // unblock at test end
	tickStarted := make(chan struct{}, 1)

	c, _ := NewJoinTokenCleaner(JoinTokenCleanerConfig{
		Store:    NewInMemoryJoinTokenStore(),
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
	if err := c.Start(context.Background()); err != nil {
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
	err := c.Stop(stopCtx)
	if err == nil || !errors.Is(err, ErrInvalidJoinTokenCleaner) {
		t.Errorf("Stop err = %v, want wrapped ErrInvalidJoinTokenCleaner", err)
	}
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("Stop err = %v, want context.DeadlineExceeded chained", err)
	}
}
