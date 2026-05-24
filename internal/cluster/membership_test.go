// SPDX-License-Identifier: Apache-2.0

package cluster

import (
	"context"
	"errors"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

func testMembershipCfg(t *testing.T, ec *EtcdClient, name string) MembershipConfig {
	t.Helper()
	return MembershipConfig{
		Etcd:              ec,
		MemberName:        name,
		Addr:              name + ".local:9090",
		MemberIDFile:      filepath.Join(t.TempDir(), "member-id"),
		KeyPrefix:         "/kscore/test",
		HeartbeatInterval: 250 * time.Millisecond,
		// 10s (not 1s): under `go test ./... -race` CPU contention
		// the lease-keepalive goroutine can be starved > 1s, which
		// would spuriously expire the member (→ false MemberLeft →
		// ring churn) during heavier tests. Anti-flap only needs
		// TTL ≥ 3× heartbeat; this stays well above that.
		LeaseTTL: 10 * time.Second,
	}
}

func registered(t *testing.T, ec *EtcdClient, name string) *MembershipManager {
	t.Helper()
	mm, err := NewMembershipManager(testMembershipCfg(t, ec, name))
	if err != nil {
		t.Fatalf("NewMembershipManager: %v", err)
	}
	if err := mm.Register(context.Background()); err != nil {
		t.Fatalf("Register: %v", err)
	}
	t.Cleanup(func() {
		sctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = mm.Stop(sctx)
	})
	return mm
}

// recorder is a test MembershipObserver capturing events.
type recorder struct {
	mu sync.Mutex
	ev []MemberEvent
}

func (r *recorder) OnMembershipChange(e MemberEvent) {
	r.mu.Lock()
	r.ev = append(r.ev, e)
	r.mu.Unlock()
}

func (r *recorder) snapshot() []MemberEvent {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]MemberEvent, len(r.ev))
	copy(out, r.ev)
	return out
}

// waitFor polls cond until true. The deadline is intentionally
// generous: these tests drive real embedded etcd (multi-second
// startup, elections), and under `go test ./... -race` the whole
// suite contends for CPU, so a tight deadline flakes without
// catching real regressions. Fast paths still return immediately
// (poll loop), so the large ceiling costs nothing when healthy.
func waitFor(t *testing.T, cond func() bool, msg string) {
	t.Helper()
	deadline := time.Now().Add(30 * time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(40 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s", msg)
}

func TestNewMembershipManager_InvalidConfig(t *testing.T) {
	ec, _ := newEmbedded(t)
	cases := map[string]MembershipConfig{
		"nil etcd":      {MemberName: "n", MemberIDFile: "/tmp/x", HeartbeatInterval: time.Second, LeaseTTL: 3 * time.Second},
		"no name":       {Etcd: ec, MemberIDFile: "/tmp/x", HeartbeatInterval: time.Second, LeaseTTL: 3 * time.Second},
		"no id source":  {Etcd: ec, MemberName: "n", HeartbeatInterval: time.Second, LeaseTTL: 3 * time.Second},
		"anti-flap ttl": {Etcd: ec, MemberName: "n", MemberID: "x", HeartbeatInterval: 5 * time.Second, LeaseTTL: 10 * time.Second},
	}
	for name, cfg := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := NewMembershipManager(cfg); !errors.Is(err, ErrInvalidConfig) {
				t.Fatalf("err = %v, want ErrInvalidConfig", err)
			}
		})
	}
}

func TestMembership_RegisterLoadGet(t *testing.T) {
	ec, _ := newEmbedded(t)
	mm := registered(t, ec, "node-a")
	ctx := context.Background()

	members, err := mm.LoadMembers(ctx)
	if err != nil {
		t.Fatalf("LoadMembers: %v", err)
	}
	if len(members) != 1 || members[0].Name != "node-a" || members[0].Status != MemberHealthy {
		t.Fatalf("LoadMembers = %+v", members)
	}

	self := mm.Self()
	got, err := mm.GetMember(ctx, self.ID)
	if err != nil {
		t.Fatalf("GetMember: %v", err)
	}
	if got.ID != self.ID || got.Addr != "node-a.local:9090" {
		t.Fatalf("GetMember = %+v", got)
	}
	if _, err := mm.GetMember(ctx, "nope"); !errors.Is(err, ErrMemberNotFound) {
		t.Fatalf("GetMember(nope) = %v, want ErrMemberNotFound", err)
	}
}

func TestMembership_ObserverJoinUpdateLeave(t *testing.T) {
	ec, _ := newEmbedded(t)
	// Observer node B watches; node A joins/updates/leaves.
	b := registered(t, ec, "node-b")
	rec := &recorder{}
	b.AddObserver(rec)

	a, err := NewMembershipManager(testMembershipCfg(t, ec, "node-a"))
	if err != nil {
		t.Fatalf("new A: %v", err)
	}
	if err := a.Register(context.Background()); err != nil {
		t.Fatalf("A Register: %v", err)
	}
	aID := a.Self().ID

	waitFor(t, func() bool {
		for _, e := range rec.snapshot() {
			if e.Type == MemberJoined && e.Member.ID == aID {
				return true
			}
		}
		return false
	}, "A join event")

	// Heartbeat re-PUTs the record → Updated events for A.
	waitFor(t, func() bool {
		for _, e := range rec.snapshot() {
			if e.Type == MemberUpdated && e.Member.ID == aID {
				return true
			}
		}
		return false
	}, "A update (heartbeat) event")

	// Graceful leave → LEAVING update then key delete (Left, with
	// the departing member reconstructed from prev-kv).
	if err := a.Stop(context.Background()); err != nil {
		t.Fatalf("A Stop: %v", err)
	}
	waitFor(t, func() bool {
		for _, e := range rec.snapshot() {
			if e.Type == MemberLeft && e.Member.ID == aID {
				return true
			}
		}
		return false
	}, "A left event")

	b.RemoveObserver(rec)
	before := len(rec.snapshot())
	// A second node toggling should no longer reach the removed observer.
	c := registered(t, ec, "node-c")
	_ = c.Self()
	time.Sleep(600 * time.Millisecond)
	if len(rec.snapshot()) != before {
		t.Fatalf("observer received events after RemoveObserver")
	}
}

func TestMembership_WatchMembersChannelClosesOnCtx(t *testing.T) {
	ec, _ := newEmbedded(t)
	mm := registered(t, ec, "node-a")

	ctx, cancel := context.WithCancel(context.Background())
	ch, err := mm.WatchMembers(ctx)
	if err != nil {
		t.Fatalf("WatchMembers: %v", err)
	}
	cancel()
	select {
	case _, open := <-ch:
		if open {
			// drain until closed
			for range ch {
			}
		}
	case <-time.After(15 * time.Second):
		t.Fatal("WatchMembers channel not closed after ctx cancel")
	}
}

func TestMembership_StableMemberIDPersisted(t *testing.T) {
	ec, _ := newEmbedded(t)
	idFile := filepath.Join(t.TempDir(), "id", "member-id")

	cfg := MembershipConfig{
		Etcd: ec, MemberName: "n", MemberIDFile: idFile,
		KeyPrefix: "/kscore/test", HeartbeatInterval: 250 * time.Millisecond, LeaseTTL: 10 * time.Second,
	}
	m1, err := NewMembershipManager(cfg)
	if err != nil {
		t.Fatalf("new m1: %v", err)
	}
	if err := m1.Register(context.Background()); err != nil {
		t.Fatalf("m1 Register: %v", err)
	}
	id1 := m1.Self().ID
	_ = m1.Stop(context.Background())

	m2, err := NewMembershipManager(cfg)
	if err != nil {
		t.Fatalf("new m2: %v", err)
	}
	if err := m2.Register(context.Background()); err != nil {
		t.Fatalf("m2 Register: %v", err)
	}
	defer func() { _ = m2.Stop(context.Background()) }()
	if m2.Self().ID != id1 {
		t.Fatalf("member ID not stable across restart: %q != %q", m2.Self().ID, id1)
	}
}

func TestMembership_LifecycleErrors(t *testing.T) {
	ec, _ := newEmbedded(t)
	mm, err := NewMembershipManager(testMembershipCfg(t, ec, "n"))
	if err != nil {
		t.Fatalf("new: %v", err)
	}
	ctx := context.Background()

	if _, err := mm.LoadMembers(ctx); !errors.Is(err, ErrNotRegistered) {
		t.Errorf("LoadMembers before Register = %v, want ErrNotRegistered", err)
	}
	if _, err := mm.WatchMembers(ctx); !errors.Is(err, ErrNotRegistered) {
		t.Errorf("WatchMembers before Register = %v, want ErrNotRegistered", err)
	}
	if err := mm.SetStatus(ctx, MemberDegraded); !errors.Is(err, ErrNotRegistered) {
		t.Errorf("SetStatus before Register = %v, want ErrNotRegistered", err)
	}

	if err := mm.Register(ctx); err != nil {
		t.Fatalf("Register: %v", err)
	}
	if err := mm.Register(ctx); !errors.Is(err, ErrAlreadyStarted) {
		t.Errorf("double Register = %v, want ErrAlreadyStarted", err)
	}
	if err := mm.Stop(ctx); err != nil {
		t.Fatalf("Stop: %v", err)
	}
	if err := mm.Stop(ctx); err != nil {
		t.Errorf("idempotent Stop = %v, want nil", err)
	}
	if err := mm.Register(ctx); !errors.Is(err, ErrStopped) {
		t.Errorf("Register after Stop = %v, want ErrStopped", err)
	}
}

func TestMembership_SetStatusTransitions(t *testing.T) {
	ec, _ := newEmbedded(t)
	mm := registered(t, ec, "n")
	ctx := context.Background()

	// healthy → unhealthy is not a direct edge.
	if err := mm.SetStatus(ctx, MemberUnhealthy); !errors.Is(err, ErrInvalidTransition) {
		t.Fatalf("healthy→unhealthy = %v, want ErrInvalidTransition", err)
	}
	// healthy → degraded → healthy (recover) is valid.
	if err := mm.SetStatus(ctx, MemberDegraded); err != nil {
		t.Fatalf("healthy→degraded: %v", err)
	}
	if mm.Self().Status != MemberDegraded {
		t.Fatalf("status = %q, want degraded", mm.Self().Status)
	}
	if err := mm.SetStatus(ctx, MemberHealthy); err != nil {
		t.Fatalf("degraded→healthy (recover): %v", err)
	}
}

func TestCanTransition(t *testing.T) {
	ok := []struct{ from, to MemberStatus }{
		{MemberHealthy, MemberHealthy},
		{MemberHealthy, MemberDegraded},
		{MemberDegraded, MemberUnhealthy},
		{MemberUnhealthy, MemberHealthy},
		{MemberDegraded, MemberLeaving},
	}
	bad := []struct{ from, to MemberStatus }{
		{MemberHealthy, MemberUnhealthy},
		{MemberLeaving, MemberHealthy},
		{MemberLeaving, MemberDegraded},
	}
	for _, c := range ok {
		if !canTransition(c.from, c.to) {
			t.Errorf("canTransition(%s,%s) = false, want true", c.from, c.to)
		}
	}
	for _, c := range bad {
		if canTransition(c.from, c.to) {
			t.Errorf("canTransition(%s,%s) = true, want false", c.from, c.to)
		}
	}
}
