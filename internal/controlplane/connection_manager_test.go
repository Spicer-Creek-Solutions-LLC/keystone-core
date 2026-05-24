// SPDX-License-Identifier: Apache-2.0

package controlplane_test

import (
	"context"
	"errors"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"go.keystone-core.io/keystone-core/internal/controlplane"
	"go.keystone-core.io/keystone-core/internal/state"
)

// fakeClock is a monotonic, manually-advanced clock used to drive the
// stale-detection sweep deterministically without sleeping in tests.
type fakeClock struct {
	mu  sync.Mutex
	now time.Time
}

func newFakeClock(t time.Time) *fakeClock { return &fakeClock{now: t} }

func (c *fakeClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

func (c *fakeClock) Advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.now = c.now.Add(d)
}

// newTestStore returns a real SQLite-backed Store with a per-test temp
// dir. Mirrors the pattern used elsewhere in internal/state tests.
func newTestStore(t *testing.T) state.Store {
	t.Helper()
	cfg := &state.Config{
		Backend: state.BackendSQLite,
		SQLite:  state.SQLiteConfig{Path: filepath.Join(t.TempDir(), "store.db")},
	}
	store, err := state.NewStore(cfg)
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	return store
}

func newAgent(id string) *state.AgentRecord {
	return &state.AgentRecord{
		ID:           id,
		Hostname:     id + ".example.com",
		OS:           "linux",
		Architecture: "amd64",
		IPAddresses:  []string{"10.0.0.1"},
		AgentVersion: "1.0.0",
		Labels:       map[string]string{"role": "web"},
	}
}

func TestNew_DefaultsAndValidation(t *testing.T) {
	store := newTestStore(t)

	if _, err := controlplane.New(controlplane.Config{}); err == nil {
		t.Fatal("New with nil Store should error")
	}
	if _, err := controlplane.New(controlplane.Config{Store: store, HeartbeatInterval: -time.Second}); err == nil {
		t.Fatal("negative HeartbeatInterval should error")
	}
	if _, err := controlplane.New(controlplane.Config{Store: store, StaleThreshold: -1}); err == nil {
		t.Fatal("negative StaleThreshold should error")
	}

	mgr, err := controlplane.New(controlplane.Config{Store: store})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if mgr == nil {
		t.Fatal("New returned nil manager")
	}
}

func TestStart_HydratesCacheFromStore(t *testing.T) {
	ctx := context.Background()
	store := newTestStore(t)

	now := time.Date(2026, 5, 7, 12, 0, 0, 0, time.UTC)
	for _, id := range []string{"a1", "a2", "a3"} {
		rec := newAgent(id)
		rec.Status = state.AgentStatusConnected
		rec.RegisteredAt = now
		rec.LastHeartbeatAt = now
		if err := store.CreateAgent(ctx, rec); err != nil {
			t.Fatalf("CreateAgent %s: %v", id, err)
		}
	}

	clk := newFakeClock(now.Add(time.Minute))
	mgr := mustNew(t, controlplane.Config{
		Store:             store,
		HeartbeatInterval: time.Hour, // disable monitor sweeps for this test
		StaleThreshold:    3,
		Clock:             clk.Now,
	})
	if err := mgr.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer stopOK(t, mgr)

	got := mgr.List(state.AgentFilter{})
	if len(got) != 3 {
		t.Fatalf("cache size = %d, want 3", len(got))
	}
	counts := mgr.Counts()
	if counts.Total != 3 || counts.Connected != 3 {
		t.Fatalf("counts = %+v, want Total=3 Connected=3", counts)
	}
}

func TestRegister_UpsertsAndCaches(t *testing.T) {
	ctx := context.Background()
	store := newTestStore(t)
	clk := newFakeClock(time.Date(2026, 5, 7, 0, 0, 0, 0, time.UTC))
	mgr := mustNew(t, controlplane.Config{
		Store:             store,
		HeartbeatInterval: time.Hour,
		Clock:             clk.Now,
	})
	mustStart(t, mgr)
	defer stopOK(t, mgr)

	rec := newAgent("a1")
	if err := mgr.Register(ctx, rec); err != nil {
		t.Fatalf("Register: %v", err)
	}
	stored, err := store.GetAgent(ctx, "a1")
	if err != nil {
		t.Fatalf("GetAgent: %v", err)
	}
	if stored.Status != state.AgentStatusConnected {
		t.Errorf("stored.Status = %q, want connected", stored.Status)
	}
	originalReg := stored.RegisteredAt

	// Re-register: hostname changes, status should remain connected,
	// and RegisteredAt must not be rewritten.
	clk.Advance(5 * time.Minute)
	rec2 := newAgent("a1")
	rec2.Hostname = "renamed.example.com"
	if err := mgr.Register(ctx, rec2); err != nil {
		t.Fatalf("re-Register: %v", err)
	}
	stored2, err := store.GetAgent(ctx, "a1")
	if err != nil {
		t.Fatalf("GetAgent: %v", err)
	}
	if !stored2.RegisteredAt.Equal(originalReg) {
		t.Errorf("RegisteredAt rewritten: %v -> %v", originalReg, stored2.RegisteredAt)
	}
	if stored2.Hostname != "renamed.example.com" {
		t.Errorf("Hostname not updated: %q", stored2.Hostname)
	}
	if !stored2.LastHeartbeatAt.Equal(clk.Now()) {
		t.Errorf("LastHeartbeatAt = %v, want %v", stored2.LastHeartbeatAt, clk.Now())
	}
}

func TestRegister_NilOrEmptyIDRejected(t *testing.T) {
	ctx := context.Background()
	store := newTestStore(t)
	mgr := mustNew(t, controlplane.Config{Store: store, HeartbeatInterval: time.Hour})
	mustStart(t, mgr)
	defer stopOK(t, mgr)

	if err := mgr.Register(ctx, nil); err == nil {
		t.Error("nil agent should error")
	}
	if err := mgr.Register(ctx, &state.AgentRecord{}); err == nil {
		t.Error("empty ID should error")
	}
}

func TestHeartbeat_UpdatesTimestamp(t *testing.T) {
	ctx := context.Background()
	store := newTestStore(t)
	clk := newFakeClock(time.Date(2026, 5, 7, 0, 0, 0, 0, time.UTC))
	mgr := mustNew(t, controlplane.Config{
		Store:             store,
		HeartbeatInterval: time.Hour,
		Clock:             clk.Now,
	})
	mustStart(t, mgr)
	defer stopOK(t, mgr)

	if err := mgr.Register(ctx, newAgent("a1")); err != nil {
		t.Fatalf("Register: %v", err)
	}
	clk.Advance(time.Minute)
	if err := mgr.Heartbeat(ctx, "a1"); err != nil {
		t.Fatalf("Heartbeat: %v", err)
	}
	stored, err := store.GetAgent(ctx, "a1")
	if err != nil {
		t.Fatalf("GetAgent: %v", err)
	}
	if !stored.LastHeartbeatAt.Equal(clk.Now()) {
		t.Errorf("LastHeartbeatAt = %v, want %v", stored.LastHeartbeatAt, clk.Now())
	}
}

func TestHeartbeat_UnknownAgent(t *testing.T) {
	ctx := context.Background()
	store := newTestStore(t)
	mgr := mustNew(t, controlplane.Config{Store: store, HeartbeatInterval: time.Hour})
	mustStart(t, mgr)
	defer stopOK(t, mgr)

	err := mgr.Heartbeat(ctx, "ghost")
	if !errors.Is(err, controlplane.ErrNotRegistered) {
		t.Fatalf("err = %v, want ErrNotRegistered", err)
	}
}

func TestHeartbeat_DisabledRejected(t *testing.T) {
	ctx := context.Background()
	store := newTestStore(t)
	mgr := mustNew(t, controlplane.Config{Store: store, HeartbeatInterval: time.Hour})
	mustStart(t, mgr)
	defer stopOK(t, mgr)

	if err := mgr.Register(ctx, newAgent("a1")); err != nil {
		t.Fatalf("Register: %v", err)
	}
	if err := mgr.Disable(ctx, "a1"); err != nil {
		t.Fatalf("Disable: %v", err)
	}
	if err := mgr.Heartbeat(ctx, "a1"); !errors.Is(err, controlplane.ErrAgentDisabled) {
		t.Fatalf("err = %v, want ErrAgentDisabled", err)
	}
}

func TestHeartbeat_RecoversFromStale(t *testing.T) {
	ctx := context.Background()
	store := newTestStore(t)
	clk := newFakeClock(time.Date(2026, 5, 7, 0, 0, 0, 0, time.UTC))
	mgr := mustNew(t, controlplane.Config{
		Store:             store,
		HeartbeatInterval: 10 * time.Millisecond,
		StaleThreshold:    3,
		Clock:             clk.Now,
	})
	mustStart(t, mgr)
	defer stopOK(t, mgr)

	if err := mgr.Register(ctx, newAgent("a1")); err != nil {
		t.Fatalf("Register: %v", err)
	}
	// Advance past the stale window then wait for the sweep to mark it.
	clk.Advance(time.Minute)
	waitForStatus(t, mgr, "a1", state.AgentStatusStale, time.Second)

	// Heartbeat should bring it back to connected.
	if err := mgr.Heartbeat(ctx, "a1"); err != nil {
		t.Fatalf("Heartbeat: %v", err)
	}
	rec, err := mgr.Get(ctx, "a1")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if rec.Status != state.AgentStatusConnected {
		t.Errorf("Status = %q, want connected", rec.Status)
	}
	stored, err := store.GetAgent(ctx, "a1")
	if err != nil {
		t.Fatalf("GetAgent: %v", err)
	}
	if stored.Status != state.AgentStatusConnected {
		t.Errorf("stored.Status = %q, want connected", stored.Status)
	}
}

func TestMonitor_TransitionsConnectedToStale(t *testing.T) {
	ctx := context.Background()
	store := newTestStore(t)
	clk := newFakeClock(time.Date(2026, 5, 7, 0, 0, 0, 0, time.UTC))
	mgr := mustNew(t, controlplane.Config{
		Store:             store,
		HeartbeatInterval: 10 * time.Millisecond,
		StaleThreshold:    3,
		Clock:             clk.Now,
	})
	mustStart(t, mgr)
	defer stopOK(t, mgr)

	for _, id := range []string{"a1", "a2"} {
		if err := mgr.Register(ctx, newAgent(id)); err != nil {
			t.Fatalf("Register %s: %v", id, err)
		}
	}

	// Below the stale window — sweep must NOT mark either agent.
	clk.Advance(20 * time.Millisecond)
	time.Sleep(50 * time.Millisecond) // let several sweep ticks fire
	for _, id := range []string{"a1", "a2"} {
		rec, err := mgr.Get(ctx, id)
		if err != nil {
			t.Fatalf("Get %s: %v", id, err)
		}
		if rec.Status != state.AgentStatusConnected {
			t.Errorf("agent %s prematurely transitioned to %q", id, rec.Status)
		}
	}

	// Past the stale window — both should transition.
	clk.Advance(time.Hour)
	waitForStatus(t, mgr, "a1", state.AgentStatusStale, time.Second)
	waitForStatus(t, mgr, "a2", state.AgentStatusStale, time.Second)

	c := mgr.Counts()
	if c.Stale != 2 || c.Connected != 0 {
		t.Errorf("counts = %+v, want Stale=2 Connected=0", c)
	}
}

func TestMonitor_DisabledAgentNeverStaled(t *testing.T) {
	ctx := context.Background()
	store := newTestStore(t)
	clk := newFakeClock(time.Date(2026, 5, 7, 0, 0, 0, 0, time.UTC))
	mgr := mustNew(t, controlplane.Config{
		Store:             store,
		HeartbeatInterval: 10 * time.Millisecond,
		StaleThreshold:    3,
		Clock:             clk.Now,
	})
	mustStart(t, mgr)
	defer stopOK(t, mgr)

	if err := mgr.Register(ctx, newAgent("a1")); err != nil {
		t.Fatalf("Register: %v", err)
	}
	if err := mgr.Disable(ctx, "a1"); err != nil {
		t.Fatalf("Disable: %v", err)
	}
	clk.Advance(time.Hour)
	time.Sleep(80 * time.Millisecond) // let several sweeps fire

	rec, err := mgr.Get(ctx, "a1")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if rec.Status != state.AgentStatusDisabled {
		t.Errorf("Status = %q, want disabled", rec.Status)
	}
}

func TestStop_IsIdempotentAndBoundedByCtx(t *testing.T) {
	ctx := context.Background()
	store := newTestStore(t)
	mgr := mustNew(t, controlplane.Config{Store: store, HeartbeatInterval: time.Hour})
	mustStart(t, mgr)

	stopCtx, cancel := context.WithTimeout(ctx, time.Second)
	defer cancel()
	if err := mgr.Stop(stopCtx); err != nil {
		t.Fatalf("Stop: %v", err)
	}
	if err := mgr.Stop(stopCtx); err != nil {
		t.Fatalf("second Stop: %v", err)
	}

	if err := mgr.Register(ctx, newAgent("a1")); !errors.Is(err, controlplane.ErrClosed) {
		t.Fatalf("Register after Stop = %v, want ErrClosed", err)
	}
}

func TestStop_BeforeStartIsSafe(t *testing.T) {
	store := newTestStore(t)
	mgr := mustNew(t, controlplane.Config{Store: store, HeartbeatInterval: time.Hour})
	if err := mgr.Stop(context.Background()); err != nil {
		t.Fatalf("Stop before Start: %v", err)
	}
}

func TestRegister_BeforeStartFails(t *testing.T) {
	store := newTestStore(t)
	mgr := mustNew(t, controlplane.Config{Store: store, HeartbeatInterval: time.Hour})
	if err := mgr.Register(context.Background(), newAgent("a1")); !errors.Is(err, controlplane.ErrNotStarted) {
		t.Fatalf("err = %v, want ErrNotStarted", err)
	}
}

func TestList_FilterByStatusAndLabel(t *testing.T) {
	ctx := context.Background()
	store := newTestStore(t)
	mgr := mustNew(t, controlplane.Config{Store: store, HeartbeatInterval: time.Hour})
	mustStart(t, mgr)
	defer stopOK(t, mgr)

	web := newAgent("web1")
	db := newAgent("db1")
	db.Labels = map[string]string{"role": "db"}
	for _, a := range []*state.AgentRecord{web, db} {
		if err := mgr.Register(ctx, a); err != nil {
			t.Fatalf("Register %s: %v", a.ID, err)
		}
	}

	got := mgr.List(state.AgentFilter{LabelKey: "role", LabelValue: "web"})
	if len(got) != 1 || got[0].ID != "web1" {
		t.Fatalf("List by label = %v, want [web1]", got)
	}
	got = mgr.List(state.AgentFilter{Status: state.AgentStatusConnected})
	if len(got) != 2 {
		t.Fatalf("List by status = %d agents, want 2", len(got))
	}
}

func TestDelete_RemovesFromStoreAndCache(t *testing.T) {
	ctx := context.Background()
	store := newTestStore(t)
	mgr := mustNew(t, controlplane.Config{Store: store, HeartbeatInterval: time.Hour})
	mustStart(t, mgr)
	defer stopOK(t, mgr)

	if err := mgr.Register(ctx, newAgent("a1")); err != nil {
		t.Fatalf("Register: %v", err)
	}
	if err := mgr.Delete(ctx, "a1"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if _, err := store.GetAgent(ctx, "a1"); !errors.Is(err, state.ErrNotFound) {
		t.Errorf("store still has agent: %v", err)
	}
	if mgr.Counts().Total != 0 {
		t.Errorf("cache still has agents: %+v", mgr.Counts())
	}
	if err := mgr.Delete(ctx, ""); err == nil {
		t.Error("empty ID should error")
	}
}

func TestGet_CacheMissFallsThroughToStore(t *testing.T) {
	ctx := context.Background()
	store := newTestStore(t)

	// Seed an agent directly in the store, but don't start the manager
	// before Get — the fallback path is what we want to verify.
	now := time.Date(2026, 5, 7, 0, 0, 0, 0, time.UTC)
	rec := newAgent("only-in-store")
	rec.Status = state.AgentStatusConnected
	rec.RegisteredAt = now
	rec.LastHeartbeatAt = now
	if err := store.CreateAgent(ctx, rec); err != nil {
		t.Fatalf("CreateAgent: %v", err)
	}

	// Use a manager that has not been Started — cache is empty.
	mgr := mustNew(t, controlplane.Config{Store: store, HeartbeatInterval: time.Hour})
	got, err := mgr.Get(ctx, "only-in-store")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.ID != "only-in-store" {
		t.Errorf("Get returned %+v", got)
	}

	if _, err := mgr.Get(ctx, "ghost"); !errors.Is(err, state.ErrNotFound) {
		t.Errorf("Get ghost = %v, want ErrNotFound", err)
	}
}

func TestConcurrentRegisterAndHeartbeat(t *testing.T) {
	ctx := context.Background()
	store := newTestStore(t)
	clk := newFakeClock(time.Date(2026, 5, 7, 0, 0, 0, 0, time.UTC))
	mgr := mustNew(t, controlplane.Config{
		Store:             store,
		HeartbeatInterval: 5 * time.Millisecond,
		Clock:             clk.Now,
	})
	mustStart(t, mgr)
	defer stopOK(t, mgr)

	const n = 16
	for i := 0; i < n; i++ {
		if err := mgr.Register(ctx, newAgent(idForN(i))); err != nil {
			t.Fatalf("Register %d: %v", i, err)
		}
	}

	var wg sync.WaitGroup
	var beats atomic.Int64
	stop := make(chan struct{})
	for i := 0; i < n; i++ {
		i := i
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				select {
				case <-stop:
					return
				default:
					if err := mgr.Heartbeat(ctx, idForN(i)); err != nil && !errors.Is(err, controlplane.ErrAgentDisabled) {
						t.Errorf("Heartbeat %d: %v", i, err)
						return
					}
					beats.Add(1)
				}
			}
		}()
	}
	time.Sleep(50 * time.Millisecond)
	close(stop)
	wg.Wait()

	if beats.Load() == 0 {
		t.Fatal("no heartbeats recorded")
	}
	c := mgr.Counts()
	if c.Total != n {
		t.Errorf("Total = %d, want %d", c.Total, n)
	}
}

// ---- helpers --------------------------------------------------------------

func mustNew(t *testing.T, cfg controlplane.Config) *controlplane.ConnectionManager {
	t.Helper()
	mgr, err := controlplane.New(cfg)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return mgr
}

func mustStart(t *testing.T, mgr *controlplane.ConnectionManager) {
	t.Helper()
	if err := mgr.Start(context.Background()); err != nil {
		t.Fatalf("Start: %v", err)
	}
}

func stopOK(t *testing.T, mgr *controlplane.ConnectionManager) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := mgr.Stop(ctx); err != nil {
		t.Errorf("Stop: %v", err)
	}
}

func waitForStatus(t *testing.T, mgr *controlplane.ConnectionManager, id string, want state.AgentStatus, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		rec, err := mgr.Get(context.Background(), id)
		if err == nil && rec.Status == want {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	rec, _ := mgr.Get(context.Background(), id)
	gotStatus := state.AgentStatus("<missing>")
	if rec != nil {
		gotStatus = rec.Status
	}
	t.Fatalf("agent %s status = %q, want %q after %s", id, gotStatus, want, timeout)
}

func idForN(i int) string {
	const letters = "abcdefghijklmnopqrstuvwxyz"
	if i < len(letters) {
		return string(letters[i])
	}
	return string(letters[i%len(letters)]) + string(letters[(i/len(letters))%len(letters)])
}
