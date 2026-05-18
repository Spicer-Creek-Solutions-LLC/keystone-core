package cluster

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	clientv3 "go.etcd.io/etcd/client/v3"
)

type fakeRecEtcd struct {
	mu        sync.Mutex
	failN     int // fail this many Gets, then succeed
	calls     int
	alwaysErr error
}

func (e *fakeRecEtcd) Get(ctx context.Context, _ string, _ ...clientv3.OpOption) (*clientv3.GetResponse, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.calls++
	if e.alwaysErr != nil {
		return nil, e.alwaysErr
	}
	if e.calls <= e.failN {
		return nil, errors.New("etcd transient")
	}
	return &clientv3.GetResponse{}, nil
}

type fakeRecMembership struct {
	self        Member
	registerErr error
	loadErr     error
	registered  bool
	mu          sync.Mutex
}

func (m *fakeRecMembership) Register(context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.registerErr != nil {
		return m.registerErr
	}
	m.registered = true
	return nil
}
func (m *fakeRecMembership) LoadMembers(context.Context) ([]Member, error) {
	return nil, m.loadErr
}
func (m *fakeRecMembership) Self() Member { return m.self }

type fakeRecShards struct {
	mu   sync.Mutex
	list []ShardAssignment
	err  error
}

func (s *fakeRecShards) List(context.Context) ([]ShardAssignment, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.list, s.err
}

type recReclaimer struct {
	mu    sync.Mutex
	owned []ShardAssignment
	err   error
}

func (r *recReclaimer) ReclaimAgents(_ context.Context, owned []ShardAssignment) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.err != nil {
		return r.err
	}
	r.owned = owned
	return nil
}

type recObs struct {
	mu     sync.Mutex
	phases []RecoveryPhase
}

func (o *recObs) OnRecovery(e RecoveryEvent) {
	o.mu.Lock()
	o.phases = append(o.phases, e.Phase)
	o.mu.Unlock()
}
func (o *recObs) snap() []RecoveryPhase {
	o.mu.Lock()
	defer o.mu.Unlock()
	return append([]RecoveryPhase(nil), o.phases...)
}

func baseRecoveryCfg() RecoveryManagerConfig {
	return RecoveryManagerConfig{
		Etcd:           &fakeRecEtcd{},
		Membership:     &fakeRecMembership{self: Member{ID: "X"}},
		Shards:         &fakeRecShards{},
		ConnectTimeout: time.Second,
		ConnectRetries: 3,
	}
}

func TestNewRecoveryManager_InvalidConfig(t *testing.T) {
	c := baseRecoveryCfg()
	c.Etcd = nil
	if _, err := NewRecoveryManager(c); !errors.Is(err, ErrInvalidConfig) {
		t.Fatalf("nil etcd: %v", err)
	}
	c = baseRecoveryCfg()
	c.Membership = nil
	if _, err := NewRecoveryManager(c); !errors.Is(err, ErrInvalidConfig) {
		t.Fatalf("nil membership: %v", err)
	}
	c = baseRecoveryCfg()
	c.Shards = nil
	if _, err := NewRecoveryManager(c); !errors.Is(err, ErrInvalidConfig) {
		t.Fatalf("nil shards: %v", err)
	}
}

func TestRecovery_HappyPathPhasesAndReclaim(t *testing.T) {
	rcl := &recReclaimer{}
	cfg := baseRecoveryCfg()
	cfg.Shards = &fakeRecShards{list: []ShardAssignment{
		{AgentID: "a1", MemberID: "X"},
		{AgentID: "a2", MemberID: "Y"},
		{AgentID: "a3", MemberID: "X"},
	}}
	cfg.Reclaimer = rcl
	rm, err := NewRecoveryManager(cfg)
	if err != nil {
		t.Fatalf("new: %v", err)
	}
	obs := &recObs{}
	rm.AddObserver(obs)

	if err := rm.Recover(context.Background()); err != nil {
		t.Fatalf("Recover: %v", err)
	}
	if rm.Phase() != RecoveryCompleted {
		t.Fatalf("Phase = %q, want completed", rm.Phase())
	}
	want := []RecoveryPhase{
		RecoveryStarting, RecoveryConnecting, RecoverySyncing,
		RecoveryVerifying, RecoveryRejoining, RecoveryReclaiming, RecoveryCompleted,
	}
	got := obs.snap()
	if len(got) != len(want) {
		t.Fatalf("phases = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("phase[%d] = %q, want %q (%v)", i, got[i], want[i], got)
		}
	}
	rcl.mu.Lock()
	defer rcl.mu.Unlock()
	if len(rcl.owned) != 2 || rcl.owned[0].AgentID != "a1" || rcl.owned[1].AgentID != "a3" {
		t.Fatalf("reclaimed %+v, want only X-owned a1,a3", rcl.owned)
	}
}

func TestRecovery_SingleUse(t *testing.T) {
	rm, _ := NewRecoveryManager(baseRecoveryCfg())
	if err := rm.Recover(context.Background()); err != nil {
		t.Fatalf("first Recover: %v", err)
	}
	if err := rm.Recover(context.Background()); !errors.Is(err, ErrAlreadyStarted) {
		t.Fatalf("second Recover = %v, want ErrAlreadyStarted", err)
	}
}

func TestRecovery_ConnectRetryThenSuccess(t *testing.T) {
	cfg := baseRecoveryCfg()
	cfg.Etcd = &fakeRecEtcd{failN: 2}
	cfg.ConnectRetries = 3
	rm, _ := NewRecoveryManager(cfg)
	if err := rm.Recover(context.Background()); err != nil {
		t.Fatalf("Recover with transient etcd: %v", err)
	}
	if rm.Phase() != RecoveryCompleted {
		t.Fatalf("Phase = %q, want completed", rm.Phase())
	}
}

func TestRecovery_ConnectExhaustedFails(t *testing.T) {
	cfg := baseRecoveryCfg()
	cfg.Etcd = &fakeRecEtcd{alwaysErr: errors.New("etcd down")}
	cfg.ConnectRetries = 2
	cfg.ConnectTimeout = 100 * time.Millisecond
	rm, _ := NewRecoveryManager(cfg)
	obs := &recObs{}
	rm.AddObserver(obs)

	if err := rm.Recover(context.Background()); err == nil {
		t.Fatal("Recover should fail when etcd unreachable")
	}
	if rm.Phase() != RecoveryFailed {
		t.Fatalf("Phase = %q, want failed", rm.Phase())
	}
	ph := obs.snap()
	if ph[len(ph)-1] != RecoveryFailed {
		t.Fatalf("last phase = %q, want failed (%v)", ph[len(ph)-1], ph)
	}
}

func TestRecovery_PhaseFailures(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*RecoveryManagerConfig)
		want   RecoveryPhase // phase that failed (for context)
	}{
		{"load members error", func(c *RecoveryManagerConfig) {
			c.Membership = &fakeRecMembership{self: Member{ID: "X"}, loadErr: errors.New("boom")}
		}, RecoverySyncing},
		{"list shards error", func(c *RecoveryManagerConfig) {
			c.Shards = &fakeRecShards{err: errors.New("boom")}
		}, RecoverySyncing},
		{"verify corrupt", func(c *RecoveryManagerConfig) {
			c.Shards = &fakeRecShards{list: []ShardAssignment{{AgentID: "a", MemberID: ""}}}
		}, RecoveryVerifying},
		{"register error", func(c *RecoveryManagerConfig) {
			c.Membership = &fakeRecMembership{self: Member{ID: "X"}, registerErr: errors.New("boom")}
		}, RecoveryRejoining},
		{"reclaim error", func(c *RecoveryManagerConfig) {
			c.Shards = &fakeRecShards{list: []ShardAssignment{{AgentID: "a", MemberID: "X"}}}
			c.Reclaimer = &recReclaimer{err: errors.New("boom")}
		}, RecoveryReclaiming},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := baseRecoveryCfg()
			tt.mutate(&cfg)
			rm, err := NewRecoveryManager(cfg)
			if err != nil {
				t.Fatalf("new: %v", err)
			}
			if err := rm.Recover(context.Background()); err == nil {
				t.Fatal("expected Recover error")
			}
			if rm.Phase() != RecoveryFailed {
				t.Fatalf("Phase = %q, want failed", rm.Phase())
			}
		})
	}
}

func TestRecovery_NilReclaimerSkips(t *testing.T) {
	cfg := baseRecoveryCfg()
	cfg.Shards = &fakeRecShards{list: []ShardAssignment{{AgentID: "a", MemberID: "X"}}}
	cfg.Reclaimer = nil
	rm, _ := NewRecoveryManager(cfg)
	if err := rm.Recover(context.Background()); err != nil {
		t.Fatalf("Recover: %v", err)
	}
	if rm.Phase() != RecoveryCompleted {
		t.Fatalf("Phase = %q, want completed", rm.Phase())
	}
}

func TestRecovery_LoadMembersNotRegisteredTolerated(t *testing.T) {
	cfg := baseRecoveryCfg()
	cfg.Membership = &fakeRecMembership{self: Member{ID: "X"}, loadErr: ErrNotRegistered}
	rm, _ := NewRecoveryManager(cfg)
	if err := rm.Recover(context.Background()); err != nil {
		t.Fatalf("ErrNotRegistered from LoadMembers must be tolerated: %v", err)
	}
	if rm.Phase() != RecoveryCompleted {
		t.Fatalf("Phase = %q, want completed", rm.Phase())
	}
}

func TestRecovery_ObserverAddRemove(t *testing.T) {
	rm, _ := NewRecoveryManager(baseRecoveryCfg())
	o := &recObs{}
	rm.AddObserver(nil)
	rm.AddObserver(o)
	rm.RemoveObserver(o)
	rm.RemoveObserver(&recObs{})
}

func TestRecovery_IntegrationStableIDReclaim(t *testing.T) {
	ec, _ := newEmbedded(t)
	ss, _ := NewShardStore(ShardStoreConfig{Etcd: ec, KeyPrefix: "/kscore/test"})
	ctx := context.Background()

	const myID = "node-A-stable"
	// Pre-seed the shard map as it would be after this node owned
	// some agents before its crash (+ one owned by another node).
	for _, a := range []struct{ agent, member string }{
		{"agent-1", myID}, {"agent-2", "node-B"}, {"agent-3", myID},
	} {
		if _, err := ss.Assign(ctx, a.agent, a.member); err != nil {
			t.Fatalf("seed Assign: %v", err)
		}
	}

	// Fresh MembershipManager (process restart) with the SAME
	// stable member ID — this is what makes RECLAIMING recognise
	// the pre-crash assignments as ours.
	mm, err := NewMembershipManager(MembershipConfig{
		Etcd: ec, MemberName: "node-A", MemberID: myID,
		KeyPrefix: "/kscore/test", HeartbeatInterval: 250 * time.Millisecond,
		LeaseTTL: 10 * time.Second,
	})
	if err != nil {
		t.Fatalf("NewMembershipManager: %v", err)
	}
	t.Cleanup(func() { _ = mm.Stop(context.Background()) })

	rcl := &recReclaimer{}
	rm, err := NewRecoveryManager(RecoveryManagerConfig{
		Etcd: ec, Membership: mm, Shards: ss, Reclaimer: rcl,
		ConnectTimeout: 3 * time.Second, ConnectRetries: 3,
	})
	if err != nil {
		t.Fatalf("NewRecoveryManager: %v", err)
	}
	if err := rm.Recover(ctx); err != nil {
		t.Fatalf("Recover: %v", err)
	}
	if rm.Phase() != RecoveryCompleted {
		t.Fatalf("Phase = %q, want completed", rm.Phase())
	}
	// Re-registered under the stable ID, and reclaimed exactly the
	// two agents that were ours.
	if mm.Self().ID != myID {
		t.Fatalf("Self().ID = %q, want %q", mm.Self().ID, myID)
	}
	rcl.mu.Lock()
	defer rcl.mu.Unlock()
	if len(rcl.owned) != 2 {
		t.Fatalf("reclaimed %d agents, want 2 (%+v)", len(rcl.owned), rcl.owned)
	}
	for _, a := range rcl.owned {
		if a.MemberID != myID {
			t.Fatalf("reclaimed a non-owned agent: %+v", a)
		}
	}
}
