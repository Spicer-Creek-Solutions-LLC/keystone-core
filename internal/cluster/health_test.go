// SPDX-License-Identifier: Apache-2.0

package cluster

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"
)

// fakeChecker is a HealthChecker with a togglable error.
type fakeChecker struct {
	name string
	mu   sync.Mutex
	err  error
}

func (f *fakeChecker) Name() string { return f.name }
func (f *fakeChecker) Check(context.Context) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.err
}
func (f *fakeChecker) set(err error) {
	f.mu.Lock()
	f.err = err
	f.mu.Unlock()
}

// fakeSink is an in-memory memberStatusSink that enforces the Task 2
// transition rules, so a test can assert HealthMonitor only walks
// valid edges.
type fakeSink struct {
	mu          sync.Mutex
	status      MemberStatus
	loadErr     error
	transitions []MemberStatus
	invalid     int
}

func newFakeSink() *fakeSink { return &fakeSink{status: MemberHealthy} }

func (s *fakeSink) LoadMembers(context.Context) ([]Member, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return nil, s.loadErr
}
func (s *fakeSink) SetStatus(_ context.Context, to MemberStatus) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !canTransition(s.status, to) {
		s.invalid++
		return ErrInvalidTransition
	}
	s.status = to
	s.transitions = append(s.transitions, to)
	return nil
}
func (s *fakeSink) setLoadErr(err error) {
	s.mu.Lock()
	s.loadErr = err
	s.mu.Unlock()
}
func (s *fakeSink) snapshot() (MemberStatus, []MemberStatus, int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.status, append([]MemberStatus(nil), s.transitions...), s.invalid
}

func startHealth(t *testing.T, cfg HealthMonitorConfig) *HealthMonitor {
	t.Helper()
	hm, err := NewHealthMonitor(cfg)
	if err != nil {
		t.Fatalf("NewHealthMonitor: %v", err)
	}
	if err := hm.Start(context.Background()); err != nil {
		t.Fatalf("Start: %v", err)
	}
	t.Cleanup(func() {
		sctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = hm.Stop(sctx)
	})
	return hm
}

func TestNewHealthMonitor_InvalidConfig(t *testing.T) {
	if _, err := NewHealthMonitor(HealthMonitorConfig{}); !errors.Is(err, ErrInvalidConfig) {
		t.Fatalf("err = %v, want ErrInvalidConfig", err)
	}
}

func TestLatencyRing_Percentile(t *testing.T) {
	r := newLatencyRing(100)
	if r.percentile(50) != 0 {
		t.Fatal("empty ring percentile must be 0")
	}
	for i := 1; i <= 100; i++ {
		r.add(time.Duration(i))
	}
	if p := r.percentile(50); p != 50 {
		t.Errorf("p50 = %v, want 50", p)
	}
	if p := r.percentile(99); p != 99 {
		t.Errorf("p99 = %v, want 99", p)
	}
	// Ring wraps: only the most recent 100 are kept.
	for i := 101; i <= 150; i++ {
		r.add(time.Duration(i))
	}
	if p := r.percentile(50); p < 51 {
		t.Errorf("after wrap p50 = %v, want >= 51", p)
	}
}

func TestHealthMonitor_DegradeAndRecover(t *testing.T) {
	chk := &fakeChecker{name: "db", err: errors.New("down")}
	sink := newFakeSink()
	hm := startHealth(t, HealthMonitorConfig{
		Membership:       sink,
		Checkers:         []HealthChecker{chk},
		CriticalCheckers: []string{EtcdCheckerName}, // db is NOT critical
		Interval:         25 * time.Millisecond,
		FailureThreshold: 3,
		LatencyWindow:    16,
	})

	// Non-critical checker over threshold → DEGRADED.
	waitFor(t, func() bool { st, _, _ := sink.snapshot(); return st == MemberDegraded }, "degrade")
	if hm.Status() != MemberDegraded {
		t.Fatalf("Status = %q, want degraded", hm.Status())
	}
	p50, _ := hm.Latency("db")
	if p50 < 0 {
		t.Fatal("latency not recorded")
	}

	// Recover (DEGRADED→HEALTHY is a valid direct edge).
	chk.set(nil)
	waitFor(t, func() bool { st, _, _ := sink.snapshot(); return st == MemberHealthy }, "recover")

	_, _, invalid := sink.snapshot()
	if invalid != 0 {
		t.Fatalf("HealthMonitor attempted %d invalid transitions", invalid)
	}
}

func TestHealthMonitor_CriticalEscalatesViaValidPath(t *testing.T) {
	etcd := &fakeChecker{name: EtcdCheckerName, err: errors.New("etcd unreachable")}
	sink := newFakeSink()
	startHealth(t, HealthMonitorConfig{
		Membership:       sink,
		Checkers:         []HealthChecker{etcd},
		Interval:         25 * time.Millisecond,
		FailureThreshold: 2,
		LatencyWindow:    16,
	})

	// Critical failure ⇒ desired UNHEALTHY, but HEALTHY→UNHEALTHY
	// is not a direct edge: it must step through DEGRADED.
	waitFor(t, func() bool { st, _, _ := sink.snapshot(); return st == MemberUnhealthy }, "unhealthy")
	_, trans, invalid := sink.snapshot()
	if invalid != 0 {
		t.Fatalf("invalid transitions attempted: %d (path=%v)", invalid, trans)
	}
	if len(trans) < 2 || trans[0] != MemberDegraded || trans[1] != MemberUnhealthy {
		t.Fatalf("escalation path = %v, want [degraded unhealthy ...]", trans)
	}

	// Recover: UNHEALTHY→HEALTHY is a valid direct edge.
	etcd.set(nil)
	waitFor(t, func() bool { st, _, _ := sink.snapshot(); return st == MemberHealthy }, "recover")
}

func TestHealthMonitor_QuorumLossOnLoadMembersError(t *testing.T) {
	sink := newFakeSink()
	sink.setLoadErr(errors.New("etcd unreachable"))
	rec := &healthRec{}
	hm, err := NewHealthMonitor(HealthMonitorConfig{
		Membership: sink,
		Interval:   25 * time.Millisecond,
	})
	if err != nil {
		t.Fatalf("new: %v", err)
	}
	hm.AddObserver(rec)
	if err := hm.Start(context.Background()); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer func() { _ = hm.Stop(context.Background()) }()

	// No reachable etcd ⇒ minority ⇒ UNHEALTHY.
	waitFor(t, func() bool { return hm.Quorum() == QuorumMinority }, "minority")
	waitFor(t, func() bool { st, _, _ := sink.snapshot(); return st == MemberUnhealthy }, "unhealthy on quorum loss")
	if !rec.sawMinority() {
		t.Fatal("observer did not see a minority HealthEvent")
	}

	// Quorum restored → recover.
	sink.setLoadErr(nil)
	waitFor(t, func() bool { return hm.Quorum() == QuorumOK }, "quorum restored")
	waitFor(t, func() bool { st, _, _ := sink.snapshot(); return st == MemberHealthy }, "recover")
}

func TestHealthMonitor_ObserverAddRemove(t *testing.T) {
	sink := newFakeSink()
	hm, err := NewHealthMonitor(HealthMonitorConfig{Membership: sink, Interval: time.Second})
	if err != nil {
		t.Fatalf("new: %v", err)
	}
	o := &healthRec{}
	hm.AddObserver(nil) // no-op
	hm.AddObserver(o)
	hm.RemoveObserver(o)            // present
	hm.RemoveObserver(&healthRec{}) // absent — no-op
}

func TestHealthMonitor_LifecycleErrors(t *testing.T) {
	sink := newFakeSink()
	hm, err := NewHealthMonitor(HealthMonitorConfig{Membership: sink, Interval: 50 * time.Millisecond})
	if err != nil {
		t.Fatalf("new: %v", err)
	}
	ctx := context.Background()
	if err := hm.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}
	if err := hm.Start(ctx); !errors.Is(err, ErrAlreadyStarted) {
		t.Errorf("double Start = %v", err)
	}
	if err := hm.Stop(ctx); err != nil {
		t.Fatalf("Stop: %v", err)
	}
	if err := hm.Stop(ctx); err != nil {
		t.Errorf("idempotent Stop = %v", err)
	}
	if err := hm.Start(ctx); !errors.Is(err, ErrStopped) {
		t.Errorf("Start after Stop = %v", err)
	}
}

func TestHealthMonitor_DrivesRealMembershipRecord(t *testing.T) {
	ec, _ := newEmbedded(t)
	mm := registered(t, ec, "node-a")
	aID := mm.Self().ID

	etcd := &fakeChecker{name: EtcdCheckerName, err: errors.New("simulated etcd failure")}
	startHealth(t, HealthMonitorConfig{
		Membership:       mm, // real *MembershipManager
		Checkers:         []HealthChecker{etcd},
		Interval:         30 * time.Millisecond,
		FailureThreshold: 2,
		LatencyWindow:    16,
	})
	ctx := context.Background()

	// The real member record in etcd should walk to UNHEALTHY via
	// the valid SM path (Task 2 SetStatus enforces the edges).
	waitFor(t, func() bool {
		m, err := mm.GetMember(ctx, aID)
		return err == nil && m.Status == MemberUnhealthy
	}, "member record → unhealthy")

	etcd.set(nil)
	waitFor(t, func() bool {
		m, err := mm.GetMember(ctx, aID)
		return err == nil && m.Status == MemberHealthy
	}, "member record → healthy")
}

// healthRec is a pointer-typed (comparable) HealthObserver.
type healthRec struct {
	mu sync.Mutex
	ev []HealthEvent
}

func (r *healthRec) OnHealthChange(e HealthEvent) {
	r.mu.Lock()
	r.ev = append(r.ev, e)
	r.mu.Unlock()
}
func (r *healthRec) sawMinority() bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, e := range r.ev {
		if e.Quorum == QuorumMinority {
			return true
		}
	}
	return false
}
