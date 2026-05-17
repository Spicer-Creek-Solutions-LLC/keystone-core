package cluster

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func newShardMgr(t *testing.T, src membershipSource, ss *ShardStore, leader *atomic.Bool) *ShardManager {
	t.Helper()
	cfg := ShardManagerConfig{
		Membership:        src,
		Store:             ss,
		VNodes:            64,
		RebalanceCooldown: 250 * time.Millisecond,
	}
	if leader != nil {
		cfg.LeaderCheck = leader.Load
	}
	sm, err := NewShardManager(cfg)
	if err != nil {
		t.Fatalf("NewShardManager: %v", err)
	}
	if err := sm.Start(context.Background()); err != nil {
		t.Fatalf("ShardManager.Start: %v", err)
	}
	t.Cleanup(func() {
		sctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = sm.Stop(sctx)
	})
	return sm
}

func ownersByMember(t *testing.T, ss *ShardStore) map[string]int {
	t.Helper()
	list, err := ss.List(context.Background())
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	c := map[string]int{}
	for _, a := range list {
		c[a.MemberID]++
	}
	return c
}

func TestNewShardManager_InvalidConfig(t *testing.T) {
	ec, _ := newEmbedded(t)
	ss, _ := NewShardStore(ShardStoreConfig{Etcd: ec, KeyPrefix: "/kscore/test"})
	mm := registered(t, ec, "node-a")

	if _, err := NewShardManager(ShardManagerConfig{Store: ss}); !errors.Is(err, ErrInvalidConfig) {
		t.Fatalf("nil membership: %v, want ErrInvalidConfig", err)
	}
	if _, err := NewShardManager(ShardManagerConfig{Membership: mm}); !errors.Is(err, ErrInvalidConfig) {
		t.Fatalf("nil store: %v, want ErrInvalidConfig", err)
	}
}

func TestShardManager_OwnerAndAssign(t *testing.T) {
	ec, _ := newEmbedded(t)
	ss, _ := NewShardStore(ShardStoreConfig{Etcd: ec, KeyPrefix: "/kscore/test"})
	mm := registered(t, ec, "node-a")
	aID := mm.Self().ID
	sm := newShardMgr(t, mm, ss, nil) // leader = always (nil)
	ctx := context.Background()

	owner, err := sm.AssignAgent(ctx, "agent-1")
	if err != nil || owner != aID {
		t.Fatalf("AssignAgent = %q,%v; want %s", owner, err, aID)
	}
	// Persisted + sticky.
	got, err := ss.Get(ctx, "agent-1")
	if err != nil || got.MemberID != aID {
		t.Fatalf("not persisted: %+v %v", got, err)
	}
	if o, err := sm.Owner(ctx, "agent-1"); err != nil || o != aID {
		t.Fatalf("Owner(known) = %q,%v", o, err)
	}
	// Unknown agent → deterministic ring fallback (only member).
	if o, err := sm.Owner(ctx, "never-seen"); err != nil || o != aID {
		t.Fatalf("Owner(unknown) = %q,%v; want %s", o, err, aID)
	}
	// Idempotent.
	if o, _ := sm.AssignAgent(ctx, "agent-1"); o != aID {
		t.Fatalf("re-AssignAgent = %q", o)
	}
}

func TestShardManager_RebalanceOnJoinAndLeave(t *testing.T) {
	ec, _ := newEmbedded(t)
	ss, _ := NewShardStore(ShardStoreConfig{Etcd: ec, KeyPrefix: "/kscore/test"})
	a := registered(t, ec, "node-a")
	aID := a.Self().ID
	sm := newShardMgr(t, a, ss, nil)
	ctx := context.Background()

	// Kept modest: this is the package's heaviest test (n agents ×
	// {Get,AssignIf} + repeated List polling). 60 still demonstrates
	// minimal rebalance + split ownership while converging well
	// within the deadline under full-suite `-race` contention.
	const n = 60
	for i := 0; i < n; i++ {
		if _, err := sm.AssignAgent(ctx, fmt.Sprintf("agent-%d", i)); err != nil {
			t.Fatalf("AssignAgent: %v", err)
		}
	}
	if c := ownersByMember(t, ss); c[aID] != n {
		t.Fatalf("pre-join owners = %v, want all %d on %s", c, n, aID)
	}

	// node-b joins → a's membership watch → sm observer → debounced
	// rebalance moves a minimal subset to b.
	b := registered(t, ec, "node-b")
	bID := b.Self().ID
	waitFor(t, func() bool { return ownersByMember(t, ss)[bID] > 0 }, "some agents to move to node-b")

	c := ownersByMember(t, ss)
	if c[aID] == 0 {
		t.Fatalf("all agents moved off node-a (not minimal): %v", c)
	}
	if c[aID]+c[bID] != n || len(c) != 2 {
		t.Fatalf("owner set wrong after join: %v", c)
	}

	// node-b leaves → its agents reassign back to node-a; none left on b.
	if err := b.Stop(ctx); err != nil {
		t.Fatalf("b.Stop: %v", err)
	}
	waitFor(t, func() bool {
		cc := ownersByMember(t, ss)
		return cc[bID] == 0 && cc[aID] == n
	}, "all agents back on node-a after node-b leaves")
}

func TestShardManager_LeaderGate(t *testing.T) {
	ec, _ := newEmbedded(t)
	ss, _ := NewShardStore(ShardStoreConfig{Etcd: ec, KeyPrefix: "/kscore/test"})
	mm := registered(t, ec, "node-a")
	aID := mm.Self().ID

	var leader atomic.Bool // starts false
	sm := newShardMgr(t, mm, ss, &leader)
	ctx := context.Background()

	// Non-leader: returns computed owner but does NOT persist.
	owner, err := sm.AssignAgent(ctx, "agent-1")
	if err != nil || owner != aID {
		t.Fatalf("non-leader AssignAgent = %q,%v", owner, err)
	}
	if _, err := ss.Get(ctx, "agent-1"); !errors.Is(err, ErrShardNotFound) {
		t.Fatalf("non-leader must not persist; Get = %v", err)
	}
	if _, err := sm.Rebalance(ctx); err != nil {
		t.Fatalf("non-leader Rebalance: %v", err)
	}
	if l, _ := ss.List(ctx); len(l) != 0 {
		t.Fatalf("non-leader Rebalance wrote %d entries", len(l))
	}

	// Promote → now it persists.
	leader.Store(true)
	if _, err := sm.AssignAgent(ctx, "agent-1"); err != nil {
		t.Fatalf("leader AssignAgent: %v", err)
	}
	if got, err := ss.Get(ctx, "agent-1"); err != nil || got.MemberID != aID {
		t.Fatalf("leader must persist; Get = %+v %v", got, err)
	}
}

func TestShardManager_RebalanceCooldownSpacing(t *testing.T) {
	ec, _ := newEmbedded(t)
	ss, _ := NewShardStore(ShardStoreConfig{Etcd: ec, KeyPrefix: "/kscore/test"})
	a := registered(t, ec, "node-a")
	sm := newShardMgr(t, a, ss, nil) // cooldown 250ms
	ctx := context.Background()

	for i := 0; i < 120; i++ {
		if _, err := sm.AssignAgent(ctx, fmt.Sprintf("agent-%d", i)); err != nil {
			t.Fatalf("AssignAgent: %v", err)
		}
	}

	var mu sync.Mutex
	var times []time.Time
	sm.AddObserver(obsFunc(func(_ []ShardMove) {
		mu.Lock()
		times = append(times, time.Now())
		mu.Unlock()
	}))

	// node-b join then quick leave: two topology changes that both
	// produce moves; the cooldown must space the two rebalances.
	b := registered(t, ec, "node-b")
	bID := b.Self().ID
	waitFor(t, func() bool { return ownersByMember(t, ss)[bID] > 0 }, "join rebalance")
	if err := b.Stop(ctx); err != nil {
		t.Fatalf("b.Stop: %v", err)
	}
	waitFor(t, func() bool {
		mu.Lock()
		defer mu.Unlock()
		return len(times) >= 2
	}, "leave rebalance")

	mu.Lock()
	defer mu.Unlock()
	gap := times[1].Sub(times[0])
	if gap < 200*time.Millisecond { // ~cooldown, small slack
		t.Fatalf("rebalances spaced %v, want >= cooldown (250ms)", gap)
	}
}

func TestShardManager_RebalanceSafeUnderConcurrentWrites(t *testing.T) {
	ec, _ := newEmbedded(t)
	ss, _ := NewShardStore(ShardStoreConfig{Etcd: ec, KeyPrefix: "/kscore/test"})
	a := registered(t, ec, "node-a")
	sm := newShardMgr(t, a, ss, nil)
	ctx := context.Background()

	for i := 0; i < 80; i++ {
		if _, err := sm.AssignAgent(ctx, fmt.Sprintf("agent-%d", i)); err != nil {
			t.Fatalf("AssignAgent: %v", err)
		}
	}
	b := registered(t, ec, "node-b")
	_ = b.Self().ID

	// Hammer the same keys while rebalances run → version conflicts
	// must be skipped, never crash or error out.
	stop := make(chan struct{})
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		for {
			select {
			case <-stop:
				return
			default:
				_, _ = ss.Assign(ctx, "agent-1", "node-x")
				// Throttle: still races every Rebalance pass to
				// force CAS conflicts, without pathologically
				// starving the shared embedded etcd / race
				// detector (which flakes the whole package).
				time.Sleep(5 * time.Millisecond)
			}
		}
	}()
	for i := 0; i < 5; i++ {
		if _, err := sm.Rebalance(ctx); err != nil {
			t.Fatalf("Rebalance under contention: %v", err)
		}
	}
	close(stop)
	wg.Wait()
}

func TestShardManager_LifecycleErrors(t *testing.T) {
	ec, _ := newEmbedded(t)
	ss, _ := NewShardStore(ShardStoreConfig{Etcd: ec, KeyPrefix: "/kscore/test"})
	mm := registered(t, ec, "node-a")
	sm, err := NewShardManager(ShardManagerConfig{Membership: mm, Store: ss, RebalanceCooldown: 100 * time.Millisecond})
	if err != nil {
		t.Fatalf("new: %v", err)
	}
	ctx := context.Background()

	if _, err := sm.Owner(ctx, "a"); !errors.Is(err, ErrNotStarted) {
		t.Errorf("Owner before Start = %v", err)
	}
	if _, err := sm.Rebalance(ctx); !errors.Is(err, ErrNotStarted) {
		t.Errorf("Rebalance before Start = %v", err)
	}
	if _, err := sm.AssignAgent(ctx, "a"); !errors.Is(err, ErrNotStarted) {
		t.Errorf("AssignAgent before Start = %v", err)
	}

	if err := sm.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}
	if err := sm.Start(ctx); !errors.Is(err, ErrAlreadyStarted) {
		t.Errorf("double Start = %v", err)
	}
	if err := sm.Stop(ctx); err != nil {
		t.Fatalf("Stop: %v", err)
	}
	if err := sm.Stop(ctx); err != nil {
		t.Errorf("idempotent Stop = %v", err)
	}
	if err := sm.Start(ctx); !errors.Is(err, ErrStopped) {
		t.Errorf("Start after Stop = %v", err)
	}
}

func TestShardManager_ObserverRemoveAndMembers(t *testing.T) {
	ec, _ := newEmbedded(t)
	ss, _ := NewShardStore(ShardStoreConfig{Etcd: ec, KeyPrefix: "/kscore/test"})
	mm := registered(t, ec, "node-a")
	aID := mm.Self().ID
	sm := newShardMgr(t, mm, ss, nil)

	o := &moveRec{}
	sm.AddObserver(nil) // no-op
	sm.AddObserver(o)
	sm.RemoveObserver(o)         // present
	sm.RemoveObserver(&moveRec{}) // absent — no-op

	got := sm.Members()
	if len(got) != 1 || got[0] != aID {
		t.Fatalf("Members = %v, want [%s]", got, aID)
	}
}

// obsFunc adapts a func to RebalanceObserver. Func values are not
// comparable, so a func observer must not be passed to
// RemoveObserver (interface == panics) — only AddObserver. Tests
// needing removal use the pointer-typed moveRec instead.
type obsFunc func([]ShardMove)

func (f obsFunc) OnRebalance(m []ShardMove) { f(m) }

// moveRec is a pointer-typed (comparable) RebalanceObserver, safe
// for AddObserver/RemoveObserver.
type moveRec struct {
	mu sync.Mutex
	n  int
}

func (r *moveRec) OnRebalance(m []ShardMove) {
	r.mu.Lock()
	r.n += len(m)
	r.mu.Unlock()
}
