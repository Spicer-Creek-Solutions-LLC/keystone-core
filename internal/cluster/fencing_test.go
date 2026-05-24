// SPDX-License-Identifier: Apache-2.0

package cluster

import (
	"context"
	"errors"
	"strconv"
	"sync"
	"testing"
	"time"

	"go.etcd.io/etcd/api/v3/mvccpb"
	clientv3 "go.etcd.io/etcd/client/v3"
)

// --- fakes -------------------------------------------------------------

type fakeFenceQuorum struct {
	mu  sync.Mutex
	q   QuorumState
	obs []HealthObserver
}

func (f *fakeFenceQuorum) AddObserver(o HealthObserver) {
	f.mu.Lock()
	f.obs = append(f.obs, o)
	f.mu.Unlock()
}
func (f *fakeFenceQuorum) RemoveObserver(o HealthObserver) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for i, x := range f.obs {
		if x == o {
			f.obs = append(f.obs[:i], f.obs[i+1:]...)
			return
		}
	}
}
func (f *fakeFenceQuorum) Quorum() QuorumState {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.q
}
func (f *fakeFenceQuorum) fire(q QuorumState) {
	f.mu.Lock()
	f.q = q
	obs := append([]HealthObserver(nil), f.obs...)
	f.mu.Unlock()
	for _, o := range obs {
		o.OnHealthChange(HealthEvent{Quorum: q})
	}
}

type fakeFenceLeadership struct {
	mu  sync.Mutex
	obs []LeadershipObserver
}

func (f *fakeFenceLeadership) AddObserver(o LeadershipObserver) {
	f.mu.Lock()
	f.obs = append(f.obs, o)
	f.mu.Unlock()
}
func (f *fakeFenceLeadership) RemoveObserver(o LeadershipObserver) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for i, x := range f.obs {
		if x == o {
			f.obs = append(f.obs[:i], f.obs[i+1:]...)
			return
		}
	}
}
func (f *fakeFenceLeadership) fire(self bool) {
	f.mu.Lock()
	obs := append([]LeadershipObserver(nil), f.obs...)
	f.mu.Unlock()
	st := LeaderCampaigning
	if self {
		st = LeaderElected
	}
	for _, o := range obs {
		o.OnLeadershipChange(LeadershipEvent{State: st, Self: self})
	}
}

// fakeFenceTxn implements clientv3.Txn; Commit always succeeds and
// bumps the parent fake's epoch (the only Txn use is the epoch
// bump put).
type fakeFenceTxn struct{ e *fakeFenceEtcd }

func (t *fakeFenceTxn) If(...clientv3.Cmp) clientv3.Txn  { return t }
func (t *fakeFenceTxn) Then(...clientv3.Op) clientv3.Txn { return t }
func (t *fakeFenceTxn) Else(...clientv3.Op) clientv3.Txn { return t }
func (t *fakeFenceTxn) Commit() (*clientv3.TxnResponse, error) {
	t.e.mu.Lock()
	t.e.epoch++
	t.e.modRev++
	t.e.mu.Unlock()
	return &clientv3.TxnResponse{Succeeded: true}, nil
}

type fakeFenceEtcd struct {
	mu     sync.Mutex
	epoch  int64
	modRev int64
	wch    chan clientv3.WatchResponse
}

func newFakeFenceEtcd() *fakeFenceEtcd {
	return &fakeFenceEtcd{wch: make(chan clientv3.WatchResponse, 8)}
}

func (e *fakeFenceEtcd) Get(context.Context, string, ...clientv3.OpOption) (*clientv3.GetResponse, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.epoch == 0 && e.modRev == 0 {
		return &clientv3.GetResponse{}, nil
	}
	return &clientv3.GetResponse{Kvs: []*mvccpb.KeyValue{{
		Value:       []byte(strconv.FormatInt(e.epoch, 10)),
		ModRevision: e.modRev,
	}}}, nil
}
func (e *fakeFenceEtcd) Txn(context.Context) (clientv3.Txn, error) {
	return &fakeFenceTxn{e: e}, nil
}
func (e *fakeFenceEtcd) Watch(ctx context.Context, _ string, _ ...clientv3.OpOption) (clientv3.WatchChan, error) {
	out := make(chan clientv3.WatchResponse)
	go func() {
		defer close(out)
		for {
			select {
			case <-ctx.Done():
				return
			case r, ok := <-e.wch:
				if !ok {
					return
				}
				select {
				case out <- r:
				case <-ctx.Done():
					return
				}
			}
		}
	}()
	return out, nil
}
func (e *fakeFenceEtcd) pushEpoch(v int64) {
	e.wch <- clientv3.WatchResponse{Events: []*clientv3.Event{{
		Type: clientv3.EventTypePut,
		Kv:   &mvccpb.KeyValue{Value: []byte(strconv.FormatInt(v, 10))},
	}}}
}

type fenceRec struct {
	mu sync.Mutex
	ev []bool
}

func (r *fenceRec) OnFence(b bool) {
	r.mu.Lock()
	r.ev = append(r.ev, b)
	r.mu.Unlock()
}
func (r *fenceRec) last() (bool, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if len(r.ev) == 0 {
		return false, false
	}
	return r.ev[len(r.ev)-1], true
}

func startFencing(t *testing.T, mode FenceMode) (*FencingManager, *fakeFenceQuorum, *fakeFenceLeadership, *fakeFenceEtcd) {
	t.Helper()
	q := &fakeFenceQuorum{q: QuorumOK}
	l := &fakeFenceLeadership{}
	e := newFakeFenceEtcd()
	fm, err := NewFencingManager(FencingManagerConfig{
		Quorum: q, Leadership: l, Etcd: e, KeyPrefix: "/kscore/test", Mode: mode,
	})
	if err != nil {
		t.Fatalf("NewFencingManager: %v", err)
	}
	if err := fm.Start(context.Background()); err != nil {
		t.Fatalf("Start: %v", err)
	}
	t.Cleanup(func() {
		sctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = fm.Stop(sctx)
	})
	return fm, q, l, e
}

// --- tests -------------------------------------------------------------

func TestNewFencingManager_InvalidConfig(t *testing.T) {
	good := FencingManagerConfig{
		Quorum: &fakeFenceQuorum{}, Leadership: &fakeFenceLeadership{},
		Etcd: newFakeFenceEtcd(), KeyPrefix: "/p",
	}
	mut := []func(*FencingManagerConfig){
		func(c *FencingManagerConfig) { c.Quorum = nil },
		func(c *FencingManagerConfig) { c.Leadership = nil },
		func(c *FencingManagerConfig) { c.Etcd = nil },
		func(c *FencingManagerConfig) { c.KeyPrefix = "" },
		func(c *FencingManagerConfig) { c.Mode = "halt" },
	}
	for i, m := range mut {
		c := good
		m(&c)
		if _, err := NewFencingManager(c); !errors.Is(err, ErrInvalidConfig) {
			t.Fatalf("case %d: err = %v, want ErrInvalidConfig", i, err)
		}
	}
}

func TestFencing_DefaultModeReadOnly(t *testing.T) {
	fm, _, _, _ := startFencing(t, "")
	if fm.Mode() != FenceReadOnly {
		t.Fatalf("default mode = %q, want read_only", fm.Mode())
	}
}

func TestFencing_GuardBeforeStart(t *testing.T) {
	fm, _ := mustFencing(t)
	if _, err := fm.Guard(OpRead); !errors.Is(err, ErrNotStarted) {
		t.Fatalf("Guard before Start = %v, want ErrNotStarted", err)
	}
}

func mustFencing(t *testing.T) (*FencingManager, *fakeFenceEtcd) {
	t.Helper()
	e := newFakeFenceEtcd()
	fm, err := NewFencingManager(FencingManagerConfig{
		Quorum: &fakeFenceQuorum{}, Leadership: &fakeFenceLeadership{},
		Etcd: e, KeyPrefix: "/kscore/test",
	})
	if err != nil {
		t.Fatalf("new: %v", err)
	}
	return fm, e
}

func TestFencing_ModesUnderQuorumLoss(t *testing.T) {
	cases := []struct {
		mode              FenceMode
		readErr, writeErr bool // expected ErrFenced when fenced
	}{
		{FenceStrict, true, true},
		{FenceReadOnly, false, true},
		{FenceGraceful, true, true},
	}
	for _, tc := range cases {
		t.Run(string(tc.mode), func(t *testing.T) {
			fm, q, _, _ := startFencing(t, tc.mode)

			// Healthy: both allowed.
			if rel, err := fm.Guard(OpWrite); err != nil {
				t.Fatalf("write while healthy: %v", err)
			} else {
				rel()
			}

			rec := &fenceRec{}
			fm.AddObserver(rec)
			q.fire(QuorumMinority)
			if !fm.Fenced() {
				t.Fatal("not fenced after QuorumMinority")
			}
			if b, ok := rec.last(); !ok || !b {
				t.Fatalf("observer last = %v,%v want true", b, ok)
			}

			_, rErr := fm.Guard(OpRead)
			_, wErr := fm.Guard(OpWrite)
			if tc.readErr != errors.Is(rErr, ErrFenced) {
				t.Fatalf("read fenced=%v, want %v", errors.Is(rErr, ErrFenced), tc.readErr)
			}
			if tc.writeErr != errors.Is(wErr, ErrFenced) {
				t.Fatalf("write fenced=%v, want %v", errors.Is(wErr, ErrFenced), tc.writeErr)
			}

			// Quorum restored → unfenced.
			q.fire(QuorumOK)
			if fm.Fenced() {
				t.Fatal("still fenced after quorum restored")
			}
			if rel, err := fm.Guard(OpWrite); err != nil {
				t.Fatalf("write after restore: %v", err)
			} else {
				rel()
			}
		})
	}
}

func TestFencing_GracefulDrainWaitsInFlight(t *testing.T) {
	fm, q, _, _ := startFencing(t, FenceGraceful)

	rel1, err := fm.Guard(OpWrite)
	if err != nil {
		t.Fatalf("guard1: %v", err)
	}
	rel2, err := fm.Guard(OpRead)
	if err != nil {
		t.Fatalf("guard2: %v", err)
	}

	q.fire(QuorumMinority) // now fenced; new ops blocked
	if _, err := fm.Guard(OpRead); !errors.Is(err, ErrFenced) {
		t.Fatalf("new op not blocked under GRACEFUL: %v", err)
	}

	drained := make(chan error, 1)
	go func() {
		dctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		drained <- fm.Drain(dctx)
	}()
	select {
	case <-drained:
		t.Fatal("Drain returned before in-flight released")
	case <-time.After(150 * time.Millisecond):
	}
	rel1()
	rel2()
	if err := <-drained; err != nil {
		t.Fatalf("Drain after release: %v", err)
	}
}

func TestFencing_DrainCtxCancel(t *testing.T) {
	fm, _, _, _ := startFencing(t, FenceGraceful)
	rel, _ := fm.Guard(OpWrite)
	defer rel()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := fm.Drain(ctx); !errors.Is(err, context.Canceled) {
		t.Fatalf("Drain ctx-cancel = %v, want context.Canceled", err)
	}
}

func TestFencing_EpochStalenessViaWatch(t *testing.T) {
	fm, _, _, e := startFencing(t, FenceReadOnly)

	cap0 := fm.CurrentEpoch()
	if !fm.ValidEpoch(cap0) {
		t.Fatal("captured epoch should be valid immediately")
	}
	e.pushEpoch(7)
	waitFor(t, func() bool { return fm.CurrentEpoch() == 7 }, "epoch advance via watch")
	if fm.ValidEpoch(cap0) {
		t.Fatal("stale captured epoch must be invalid after advance")
	}
}

func TestFencing_DeposedLeaderSelfFences(t *testing.T) {
	fm, q, l, e := startFencing(t, FenceReadOnly)
	q.fire(QuorumOK)

	// We win election → bump epoch (fake Txn → epoch 1, myEpoch 1).
	l.fire(true)
	waitFor(t, func() bool { return fm.CurrentEpoch() >= 1 }, "epoch bumped on election")
	if fm.Fenced() {
		t.Fatal("fenced right after winning election")
	}

	// Another node bumps the epoch beyond ours while we still
	// believe we lead → self-fence.
	e.pushEpoch(9)
	waitFor(t, func() bool { return fm.Fenced() }, "deposed leader self-fence")

	// Learning we are no longer leader clears the epoch fence.
	l.fire(false)
	waitFor(t, func() bool { return !fm.Fenced() }, "epoch fence cleared on losing leadership")
}

func TestFencing_LifecycleErrors(t *testing.T) {
	e := newFakeFenceEtcd()
	fm, err := NewFencingManager(FencingManagerConfig{
		Quorum: &fakeFenceQuorum{}, Leadership: &fakeFenceLeadership{},
		Etcd: e, KeyPrefix: "/kscore/test",
	})
	if err != nil {
		t.Fatalf("new: %v", err)
	}
	ctx := context.Background()
	if err := fm.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}
	if err := fm.Start(ctx); !errors.Is(err, ErrAlreadyStarted) {
		t.Errorf("double Start = %v", err)
	}
	if err := fm.Stop(ctx); err != nil {
		t.Fatalf("Stop: %v", err)
	}
	if err := fm.Stop(ctx); err != nil {
		t.Errorf("idempotent Stop = %v", err)
	}
	if err := fm.Start(ctx); !errors.Is(err, ErrStopped) {
		t.Errorf("Start after Stop = %v", err)
	}
}

func TestFencing_ObserverAddRemove(t *testing.T) {
	fm, _, _, _ := startFencing(t, FenceStrict)
	o := &fenceRec{}
	fm.AddObserver(nil)
	fm.AddObserver(o)
	fm.RemoveObserver(o)
	fm.RemoveObserver(&fenceRec{})
}

func TestFencing_IntegrationEpochBumpRealEtcd(t *testing.T) {
	ec, _ := newEmbedded(t)
	q1, l1 := &fakeFenceQuorum{q: QuorumOK}, &fakeFenceLeadership{}
	fm1, err := NewFencingManager(FencingManagerConfig{
		Quorum: q1, Leadership: l1, Etcd: ec, KeyPrefix: "/kscore/test", Mode: FenceReadOnly,
	})
	if err != nil {
		t.Fatalf("fm1 new: %v", err)
	}
	if err := fm1.Start(context.Background()); err != nil {
		t.Fatalf("fm1 Start: %v", err)
	}
	t.Cleanup(func() { _ = fm1.Stop(context.Background()) })

	// fm1 wins election → real Txn CAS bump in etcd.
	l1.fire(true)
	waitFor(t, func() bool { return fm1.CurrentEpoch() >= 1 }, "fm1 epoch bump (real etcd)")

	// A second node (fm2, same etcd + prefix) wins a later election
	// → epoch advances; fm1's watch observes it and, still
	// believing it leads, self-fences.
	q2, l2 := &fakeFenceQuorum{q: QuorumOK}, &fakeFenceLeadership{}
	fm2, err := NewFencingManager(FencingManagerConfig{
		Quorum: q2, Leadership: l2, Etcd: ec, KeyPrefix: "/kscore/test", Mode: FenceReadOnly,
	})
	if err != nil {
		t.Fatalf("fm2 new: %v", err)
	}
	if err := fm2.Start(context.Background()); err != nil {
		t.Fatalf("fm2 Start: %v", err)
	}
	t.Cleanup(func() { _ = fm2.Stop(context.Background()) })

	l2.fire(true)
	waitFor(t, func() bool { return fm1.Fenced() }, "fm1 self-fences when fm2 bumps the epoch")
}
