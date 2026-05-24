// SPDX-License-Identifier: Apache-2.0

package cluster

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"
)

func testElectionCfg(ec *EtcdClient, memberID string) ElectionConfig {
	return ElectionConfig{
		Etcd:            ec,
		MemberID:        memberID,
		KeyPrefix:       "/kscore/test/leader",
		SessionTTL:      time.Second, // etcd-min; fast failover for tests
		ReCampaignDelay: 200 * time.Millisecond,
	}
}

func startElector(t *testing.T, ec *EtcdClient, memberID string) *LeaderElector {
	t.Helper()
	le, err := NewLeaderElector(testElectionCfg(ec, memberID))
	if err != nil {
		t.Fatalf("NewLeaderElector(%s): %v", memberID, err)
	}
	if err := le.Start(context.Background()); err != nil {
		t.Fatalf("Start(%s): %v", memberID, err)
	}
	t.Cleanup(func() {
		sctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = le.Stop(sctx)
	})
	return le
}

type leaderRec struct {
	mu sync.Mutex
	ev []LeadershipEvent
}

func (r *leaderRec) OnLeadershipChange(e LeadershipEvent) {
	r.mu.Lock()
	r.ev = append(r.ev, e)
	r.mu.Unlock()
}

func (r *leaderRec) sawElected() bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, e := range r.ev {
		if e.State == LeaderElected && e.Self {
			return true
		}
	}
	return false
}

func TestNewLeaderElector_InvalidConfig(t *testing.T) {
	ec, _ := newEmbedded(t)
	cases := map[string]ElectionConfig{
		"nil etcd":     {MemberID: "m", KeyPrefix: "/p"},
		"no member id": {Etcd: ec, KeyPrefix: "/p"},
		"no prefix":    {Etcd: ec, MemberID: "m"},
	}
	for name, cfg := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := NewLeaderElector(cfg); !errors.Is(err, ErrInvalidConfig) {
				t.Fatalf("err = %v, want ErrInvalidConfig", err)
			}
		})
	}
}

func TestLeaderElector_SingleBecomesLeader(t *testing.T) {
	ec, _ := newEmbedded(t)
	le, err := NewLeaderElector(testElectionCfg(ec, "node-a"))
	if err != nil {
		t.Fatalf("new: %v", err)
	}
	rec := &leaderRec{}
	le.AddObserver(rec)
	if err := le.Start(context.Background()); err != nil {
		t.Fatalf("Start: %v", err)
	}
	t.Cleanup(func() { _ = le.Stop(context.Background()) })

	waitFor(t, le.IsLeader, "node-a to become leader")
	if le.State() != LeaderElected {
		t.Fatalf("State = %q, want leader", le.State())
	}
	id, err := le.LeaderID(context.Background())
	if err != nil || id != "node-a" {
		t.Fatalf("LeaderID = %q, %v; want node-a", id, err)
	}
	if !rec.sawElected() {
		t.Fatal("observer did not see Self ELECTED event")
	}
}

func TestLeaderElector_FailoverOnResign(t *testing.T) {
	ec, _ := newEmbedded(t)
	a := startElector(t, ec, "node-a")
	waitFor(t, a.IsLeader, "A to lead")

	b := startElector(t, ec, "node-b")
	// B is campaigning, not leader, while A holds it.
	time.Sleep(500 * time.Millisecond)
	if b.IsLeader() {
		t.Fatal("B must not be leader while A holds leadership")
	}

	if err := a.Resign(context.Background()); err != nil {
		t.Fatalf("A.Resign: %v", err)
	}
	waitFor(t, b.IsLeader, "B to take over after A resigns")
	if a.IsLeader() {
		t.Fatal("A must not still be leader after resign")
	}
	id, err := b.LeaderID(context.Background())
	if err != nil || id != "node-b" {
		t.Fatalf("LeaderID after failover = %q, %v; want node-b", id, err)
	}
}

func TestLeaderElector_TransferLeadership(t *testing.T) {
	ec, _ := newEmbedded(t)
	a := startElector(t, ec, "node-a")
	waitFor(t, a.IsLeader, "A to lead")
	b := startElector(t, ec, "node-b")
	time.Sleep(400 * time.Millisecond)

	if err := a.TransferLeadership(context.Background()); err != nil {
		t.Fatalf("A.TransferLeadership: %v", err)
	}
	waitFor(t, b.IsLeader, "B to lead after transfer")
}

func TestLeaderElector_StopFailsOver(t *testing.T) {
	ec, _ := newEmbedded(t)
	a := startElector(t, ec, "node-a")
	waitFor(t, a.IsLeader, "A to lead")
	b := startElector(t, ec, "node-b")
	time.Sleep(300 * time.Millisecond)

	if err := a.Stop(context.Background()); err != nil {
		t.Fatalf("A.Stop: %v", err)
	}
	waitFor(t, b.IsLeader, "B to take over after A stops")
}

func TestLeaderElector_ResignWhenNotLeaderNoop(t *testing.T) {
	ec, _ := newEmbedded(t)
	a := startElector(t, ec, "node-a")
	waitFor(t, a.IsLeader, "A to lead")
	b := startElector(t, ec, "node-b")
	time.Sleep(300 * time.Millisecond)

	// B is not leader → Resign is a fast no-op.
	done := make(chan error, 1)
	go func() { done <- b.Resign(context.Background()) }()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("non-leader Resign = %v, want nil", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("non-leader Resign blocked; expected fast no-op")
	}
}

func TestLeaderElector_ObserverAddRemove(t *testing.T) {
	ec, _ := newEmbedded(t)
	le, err := NewLeaderElector(testElectionCfg(ec, "n"))
	if err != nil {
		t.Fatalf("new: %v", err)
	}
	r1 := &leaderRec{}
	r2 := &leaderRec{}
	le.AddObserver(nil) // no-op
	le.AddObserver(r1)
	le.AddObserver(r2)
	le.RemoveObserver(r1)           // present
	le.RemoveObserver(&leaderRec{}) // absent — no-op
	le.RemoveObserver(r1)           // already gone — no-op
}

func TestLeaderElector_DefaultsApplied(t *testing.T) {
	ec, _ := newEmbedded(t)
	// Zero/negative knobs + nil logger exercise fillDefaults and
	// the ttlSeconds default path.
	le, err := NewLeaderElector(ElectionConfig{
		Etcd:            ec,
		MemberID:        "node-a",
		KeyPrefix:       "/kscore/test/leader",
		SessionTTL:      0,
		ReCampaignDelay: -time.Second,
		Logger:          nil,
	})
	if err != nil {
		t.Fatalf("new with defaults: %v", err)
	}
	if err := le.Start(context.Background()); err != nil {
		t.Fatalf("Start: %v", err)
	}
	t.Cleanup(func() { _ = le.Stop(context.Background()) })
	waitFor(t, le.IsLeader, "leader with defaulted config")
}

func TestLeaderElector_LifecycleErrors(t *testing.T) {
	ec, _ := newEmbedded(t)
	le, err := NewLeaderElector(testElectionCfg(ec, "n"))
	if err != nil {
		t.Fatalf("new: %v", err)
	}
	ctx := context.Background()

	if _, err := le.LeaderID(ctx); !errors.Is(err, ErrNotStarted) {
		t.Errorf("LeaderID before Start = %v, want ErrNotStarted", err)
	}
	if err := le.Resign(ctx); !errors.Is(err, ErrNotStarted) {
		t.Errorf("Resign before Start = %v, want ErrNotStarted", err)
	}
	if le.State() != LeaderUnknown {
		t.Errorf("State before Start = %q, want unknown", le.State())
	}

	if err := le.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}
	if err := le.Start(ctx); !errors.Is(err, ErrAlreadyStarted) {
		t.Errorf("double Start = %v, want ErrAlreadyStarted", err)
	}
	if err := le.Stop(ctx); err != nil {
		t.Fatalf("Stop: %v", err)
	}
	if err := le.Stop(ctx); err != nil {
		t.Errorf("idempotent Stop = %v, want nil", err)
	}
	if err := le.Start(ctx); !errors.Is(err, ErrStopped) {
		t.Errorf("Start after Stop = %v, want ErrStopped", err)
	}
}
