// SPDX-License-Identifier: Apache-2.0

package cluster

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"
)

type fakeShutMembership struct {
	mu           sync.Mutex
	calls        []string
	setStatusErr error
	deregErr     error
	lastStatus   MemberStatus
}

func (m *fakeShutMembership) SetStatus(_ context.Context, to MemberStatus) error {
	m.mu.Lock()
	m.calls = append(m.calls, "setstatus")
	m.lastStatus = to
	m.mu.Unlock()
	return m.setStatusErr
}
func (m *fakeShutMembership) Deregister(context.Context) error {
	m.mu.Lock()
	m.calls = append(m.calls, "deregister")
	m.mu.Unlock()
	return m.deregErr
}
func (m *fakeShutMembership) order() []string {
	m.mu.Lock()
	defer m.mu.Unlock()
	return append([]string(nil), m.calls...)
}

type fakeShutLeadership struct {
	leader      bool
	transferErr error
	mu          sync.Mutex
	transferred bool
}

func (l *fakeShutLeadership) IsLeader() bool { return l.leader }
func (l *fakeShutLeadership) TransferLeadership(context.Context) error {
	l.mu.Lock()
	l.transferred = true
	l.mu.Unlock()
	return l.transferErr
}
func (l *fakeShutLeadership) didTransfer() bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.transferred
}

type fakeShutDrainer struct {
	mu       sync.Mutex
	called   bool
	err      error
	blockCtx bool // block until ctx is done, then return ctx.Err()
}

func (d *fakeShutDrainer) Drain(ctx context.Context) error {
	d.mu.Lock()
	d.called = true
	block := d.blockCtx
	derr := d.err
	d.mu.Unlock()
	if block {
		<-ctx.Done()
		return ctx.Err()
	}
	return derr
}
func (d *fakeShutDrainer) wasCalled() bool {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.called
}

type shutRec struct {
	mu sync.Mutex
	ph []ShutdownPhase
}

func (r *shutRec) OnShutdown(e ShutdownEvent) {
	r.mu.Lock()
	r.ph = append(r.ph, e.Phase)
	r.mu.Unlock()
}
func (r *shutRec) phases() []ShutdownPhase {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]ShutdownPhase(nil), r.ph...)
}

func TestNewGracefulShutdown_InvalidConfig(t *testing.T) {
	if _, err := NewGracefulShutdown(GracefulShutdownConfig{}); !errors.Is(err, ErrInvalidConfig) {
		t.Fatalf("nil Membership = %v, want ErrInvalidConfig", err)
	}
}

func TestGracefulShutdown_HappyLeader(t *testing.T) {
	mem := &fakeShutMembership{}
	ld := &fakeShutLeadership{leader: true}
	dr := &fakeShutDrainer{}
	var stopped bool
	g, err := NewGracefulShutdown(GracefulShutdownConfig{
		Membership:    mem,
		Leadership:    ld,
		Drainer:       dr,
		StopAccepting: func(context.Context) error { stopped = true; return nil },
		Timeout:       2 * time.Second,
	})
	if err != nil {
		t.Fatalf("new: %v", err)
	}
	rec := &shutRec{}
	g.AddObserver(rec)

	if err := g.Shutdown(context.Background()); err != nil {
		t.Fatalf("Shutdown: %v", err)
	}
	if g.Phase() != ShutdownCompleted {
		t.Fatalf("Phase = %q, want completed", g.Phase())
	}
	want := []ShutdownPhase{
		ShutdownInitiated, ShutdownDraining, ShutdownTransferring,
		ShutdownDeregistering, ShutdownCompleted,
	}
	got := rec.phases()
	if len(got) != len(want) {
		t.Fatalf("phases = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("phase[%d] = %q, want %q (%v)", i, got[i], want[i], got)
		}
	}
	if !stopped {
		t.Error("StopAccepting not called")
	}
	if mem.lastStatus != MemberLeaving {
		t.Errorf("SetStatus called with %q, want leaving", mem.lastStatus)
	}
	if o := mem.order(); len(o) != 2 || o[0] != "setstatus" || o[1] != "deregister" {
		t.Fatalf("membership call order = %v, want [setstatus deregister]", o)
	}
	if !ld.didTransfer() {
		t.Error("leader did not transfer leadership")
	}
	if !dr.wasCalled() {
		t.Error("Drainer.Drain not called")
	}
}

func TestGracefulShutdown_NonLeaderSkipsTransfer(t *testing.T) {
	mem := &fakeShutMembership{}
	ld := &fakeShutLeadership{leader: false}
	g, _ := NewGracefulShutdown(GracefulShutdownConfig{Membership: mem, Leadership: ld})
	rec := &shutRec{}
	g.AddObserver(rec)

	if err := g.Shutdown(context.Background()); err != nil {
		t.Fatalf("Shutdown: %v", err)
	}
	// TRANSFERRING phase still emitted, but no transfer attempted.
	saw := false
	for _, p := range rec.phases() {
		if p == ShutdownTransferring {
			saw = true
		}
	}
	if !saw {
		t.Fatal("TRANSFERRING phase not emitted")
	}
	if ld.didTransfer() {
		t.Fatal("non-leader must not transfer leadership")
	}
}

func TestGracefulShutdown_NilOptionalCollaborators(t *testing.T) {
	mem := &fakeShutMembership{}
	g, _ := NewGracefulShutdown(GracefulShutdownConfig{Membership: mem}) // no Leadership/Drainer/StopAccepting
	if err := g.Shutdown(context.Background()); err != nil {
		t.Fatalf("Shutdown: %v", err)
	}
	if g.Phase() != ShutdownCompleted {
		t.Fatalf("Phase = %q, want completed", g.Phase())
	}
	if o := mem.order(); len(o) != 2 || o[0] != "setstatus" || o[1] != "deregister" {
		t.Fatalf("call order = %v", o)
	}
}

func TestGracefulShutdown_DrainTimeoutStillDeregisters(t *testing.T) {
	mem := &fakeShutMembership{}
	dr := &fakeShutDrainer{blockCtx: true} // never finishes; waits out the budget
	g, _ := NewGracefulShutdown(GracefulShutdownConfig{
		Membership: mem, Drainer: dr, Timeout: 60 * time.Millisecond,
	})
	start := time.Now()
	if err := g.Shutdown(context.Background()); err != nil {
		t.Fatalf("Shutdown should not fail on drain timeout: %v", err)
	}
	if time.Since(start) < 50*time.Millisecond {
		t.Fatal("drain budget not waited out")
	}
	if !dr.wasCalled() {
		t.Error("Drain not called")
	}
	if o := mem.order(); len(o) == 0 || o[len(o)-1] != "deregister" {
		t.Fatalf("Deregister not called after drain timeout: %v", o)
	}
	if g.Phase() != ShutdownCompleted {
		t.Fatalf("Phase = %q, want completed", g.Phase())
	}
}

func TestGracefulShutdown_DeregisterErrorFails(t *testing.T) {
	mem := &fakeShutMembership{deregErr: errors.New("revoke failed")}
	g, _ := NewGracefulShutdown(GracefulShutdownConfig{Membership: mem})
	rec := &shutRec{}
	g.AddObserver(rec)

	if err := g.Shutdown(context.Background()); err == nil {
		t.Fatal("expected error when Deregister fails")
	}
	if g.Phase() != ShutdownFailed {
		t.Fatalf("Phase = %q, want failed", g.Phase())
	}
	ph := rec.phases()
	if ph[len(ph)-1] != ShutdownFailed {
		t.Fatalf("last phase = %q, want failed (%v)", ph[len(ph)-1], ph)
	}
}

func TestGracefulShutdown_SetStatusErrorBestEffort(t *testing.T) {
	mem := &fakeShutMembership{setStatusErr: errors.New("etcd hiccup")}
	g, _ := NewGracefulShutdown(GracefulShutdownConfig{Membership: mem})
	if err := g.Shutdown(context.Background()); err != nil {
		t.Fatalf("SetStatus error must not fail shutdown: %v", err)
	}
	if g.Phase() != ShutdownCompleted {
		t.Fatalf("Phase = %q, want completed", g.Phase())
	}
	if o := mem.order(); o[len(o)-1] != "deregister" {
		t.Fatalf("Deregister still must run: %v", o)
	}
}

func TestGracefulShutdown_SingleUse(t *testing.T) {
	g, _ := NewGracefulShutdown(GracefulShutdownConfig{Membership: &fakeShutMembership{}})
	if err := g.Shutdown(context.Background()); err != nil {
		t.Fatalf("first Shutdown: %v", err)
	}
	if err := g.Shutdown(context.Background()); !errors.Is(err, ErrAlreadyStarted) {
		t.Fatalf("second Shutdown = %v, want ErrAlreadyStarted", err)
	}
}

func TestGracefulShutdown_ObserverAddRemove(t *testing.T) {
	g, _ := NewGracefulShutdown(GracefulShutdownConfig{Membership: &fakeShutMembership{}})
	o := &shutRec{}
	g.AddObserver(nil)
	g.AddObserver(o)
	g.RemoveObserver(o)
	g.RemoveObserver(&shutRec{})
}
