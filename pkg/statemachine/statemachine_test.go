package statemachine

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"
)

type state string

const (
	idle    state = "idle"
	running state = "running"
	done    state = "done"
	failed  state = "failed"
)

type event string

const (
	start  event = "start"
	finish event = "finish"
	fail   event = "fail"
	reset  event = "reset"
)

var fixedClock = func() time.Time { return time.Date(2026, 5, 18, 12, 0, 0, 0, time.UTC) }

// trafficMachine is the reusable fixture: idle→running→done with a
// failure path and a reset back to idle.
func trafficMachine(t *testing.T, opts ...func(*Builder[state, event])) *Machine[state, event] {
	t.Helper()
	b := NewBuilder[state, event]().
		Initial(idle).
		Transition(idle, start, running).
		Transition(running, finish, done).
		Transition(running, fail, failed).
		Transition(failed, reset, idle).
		State(done).
		Clock(fixedClock)
	for _, o := range opts {
		o(b)
	}
	m, err := b.Build()
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	return m
}

func TestBuild_Errors(t *testing.T) {
	t.Run("no initial state", func(t *testing.T) {
		_, err := NewBuilder[state, event]().Transition(idle, start, running).Build()
		if !errors.Is(err, ErrNoInitialState) {
			t.Fatalf("got %v, want ErrNoInitialState", err)
		}
	})

	t.Run("duplicate transition", func(t *testing.T) {
		_, err := NewBuilder[state, event]().
			Initial(idle).
			Transition(idle, start, running).
			Transition(idle, start, failed).
			Build()
		if !errors.Is(err, ErrDuplicateTransition) {
			t.Fatalf("got %v, want ErrDuplicateTransition", err)
		}
	})

	t.Run("MustBuild panics", func(t *testing.T) {
		defer func() {
			if recover() == nil {
				t.Fatal("expected panic")
			}
		}()
		NewBuilder[state, event]().MustBuild()
	})

	t.Run("MustBuild ok", func(t *testing.T) {
		m := NewBuilder[state, event]().Initial(idle).MustBuild()
		if m.Current() != idle {
			t.Fatalf("Current=%v want idle", m.Current())
		}
	})
}

func TestFire_HappyPath(t *testing.T) {
	m := trafficMachine(t)
	ctx := context.Background()

	if got := m.Current(); got != idle {
		t.Fatalf("initial Current=%v want idle", got)
	}
	if !m.Can(start) {
		t.Fatal("Can(start) = false, want true")
	}
	if m.Can(finish) {
		t.Fatal("Can(finish) from idle = true, want false")
	}

	if err := m.Fire(ctx, start); err != nil {
		t.Fatalf("Fire(start): %v", err)
	}
	if err := m.Fire(ctx, finish); err != nil {
		t.Fatalf("Fire(finish): %v", err)
	}
	if got := m.Current(); got != done {
		t.Fatalf("Current=%v want done", got)
	}

	hist := m.History()
	if len(hist) != 2 {
		t.Fatalf("history len=%d want 2", len(hist))
	}
	want := []Record[state, event]{
		{From: idle, To: running, Event: start, At: fixedClock()},
		{From: running, To: done, Event: finish, At: fixedClock()},
	}
	for i, w := range want {
		if hist[i] != w {
			t.Errorf("history[%d]=%+v want %+v", i, hist[i], w)
		}
	}

	mtr := m.Metrics()
	if mtr.Transitions != 2 {
		t.Errorf("Transitions=%d want 2", mtr.Transitions)
	}
}

func TestFire_Routing(t *testing.T) {
	tests := []struct {
		name    string
		fire    event
		wantErr error
		counter func(MetricsSnapshot) uint64
	}{
		{"unknown event", event("bogus"), ErrUnknownEvent, func(s MetricsSnapshot) uint64 { return s.UnknownEvent }},
		{"no transition from state", finish, ErrNoTransition, func(s MetricsSnapshot) uint64 { return s.NoTransition }},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			m := trafficMachine(t)
			err := m.Fire(context.Background(), tc.fire)
			if !errors.Is(err, tc.wantErr) {
				t.Fatalf("got %v, want %v", err, tc.wantErr)
			}
			if m.Current() != idle {
				t.Fatalf("state changed to %v on rejected Fire", m.Current())
			}
			if n := tc.counter(m.Metrics()); n != 1 {
				t.Fatalf("counter=%d want 1", n)
			}
		})
	}
}

func TestFire_GuardVeto(t *testing.T) {
	guardErr := errors.New("not allowed")
	denied := true
	gm := NewBuilder[state, event]().
		Initial(idle).
		Transition(idle, start, running, WithGuard(func(_ context.Context, _ Transition[state, event]) error {
			if denied {
				return guardErr
			}
			return nil
		})).
		MustBuild()

	err := gm.Fire(context.Background(), start)
	if !errors.Is(err, ErrGuardRejected) || !errors.Is(err, guardErr) {
		t.Fatalf("got %v, want wraps ErrGuardRejected and guardErr", err)
	}
	if gm.Current() != idle {
		t.Fatalf("state changed despite guard veto: %v", gm.Current())
	}
	if gm.Metrics().GuardRejections != 1 {
		t.Fatalf("GuardRejections=%d want 1", gm.Metrics().GuardRejections)
	}

	denied = false
	if err := gm.Fire(context.Background(), start); err != nil {
		t.Fatalf("Fire after guard allows: %v", err)
	}
	if gm.Current() != running {
		t.Fatalf("Current=%v want running", gm.Current())
	}
}

func TestFire_CallbackOrderAndObserveSemantics(t *testing.T) {
	var order []string
	cbErr := errors.New("enter boom")
	m := NewBuilder[state, event]().
		Initial(idle).
		OnExit(idle, func(_ context.Context, _ Transition[state, event]) error {
			order = append(order, "exit:idle")
			return nil
		}).
		OnEnter(running, func(_ context.Context, _ Transition[state, event]) error {
			order = append(order, "enter:running")
			return cbErr
		}).
		OnTransition(func(_ context.Context, _ Transition[state, event]) error {
			order = append(order, "global:transition")
			return nil
		}).
		Transition(idle, start, running, WithOnTransition(func(_ context.Context, _ Transition[state, event]) error {
			order = append(order, "per:transition")
			return nil
		})).
		Clock(fixedClock).
		MustBuild()

	err := m.Fire(context.Background(), start)
	if !errors.Is(err, cbErr) {
		t.Fatalf("got %v, want joined cbErr", err)
	}
	// Observe semantics: callback error does not roll back.
	if m.Current() != running {
		t.Fatalf("Current=%v want running (transition must stand)", m.Current())
	}
	if len(m.History()) != 1 {
		t.Fatalf("history len=%d want 1", len(m.History()))
	}
	want := []string{"exit:idle", "enter:running", "global:transition", "per:transition"}
	if len(order) != len(want) {
		t.Fatalf("order=%v want %v", order, want)
	}
	for i := range want {
		if order[i] != want[i] {
			t.Fatalf("order=%v want %v", order, want)
		}
	}
}

func TestStates(t *testing.T) {
	m := trafficMachine(t)
	got := map[state]bool{}
	for _, s := range m.States() {
		got[s] = true
	}
	for _, s := range []state{idle, running, done, failed} {
		if !got[s] {
			t.Errorf("States() missing %v", s)
		}
	}
}

func TestDefaultClock(t *testing.T) {
	m := NewBuilder[state, event]().
		Initial(idle).
		Transition(idle, start, running).
		MustBuild()
	before := time.Now()
	if err := m.Fire(context.Background(), start); err != nil {
		t.Fatal(err)
	}
	at := m.History()[0].At
	if at.Before(before) || at.After(time.Now()) {
		t.Fatalf("default clock timestamp %v out of range", at)
	}
}

func TestCheckpoint_RoundTrip(t *testing.T) {
	cp := NewMemoryCheckpointer[state, event]()
	build := func() *Machine[state, event] {
		return NewBuilder[state, event]().
			Initial(idle).
			Transition(idle, start, running).
			Transition(running, finish, done).
			Clock(fixedClock).
			Checkpointer(cp).
			MustBuild()
	}
	ctx := context.Background()

	m1 := build()
	if err := m1.Fire(ctx, start); err != nil {
		t.Fatal(err)
	}
	if err := m1.Fire(ctx, finish); err != nil {
		t.Fatal(err)
	}
	if err := m1.Checkpoint(ctx); err != nil {
		t.Fatalf("Checkpoint: %v", err)
	}

	m2 := build()
	found, err := m2.RestoreFrom(ctx)
	if err != nil || !found {
		t.Fatalf("RestoreFrom: found=%v err=%v", found, err)
	}
	if m2.Current() != done {
		t.Fatalf("restored Current=%v want done", m2.Current())
	}
	if len(m2.History()) != 2 {
		t.Fatalf("restored history len=%d want 2", len(m2.History()))
	}
}

func TestCheckpoint_Errors(t *testing.T) {
	ctx := context.Background()

	t.Run("no checkpointer", func(t *testing.T) {
		m := trafficMachine(t)
		if err := m.Checkpoint(ctx); !errors.Is(err, ErrNoCheckpointer) {
			t.Fatalf("Checkpoint err=%v want ErrNoCheckpointer", err)
		}
		if _, err := m.RestoreFrom(ctx); !errors.Is(err, ErrNoCheckpointer) {
			t.Fatalf("RestoreFrom err=%v want ErrNoCheckpointer", err)
		}
	})

	t.Run("restore with empty checkpointer", func(t *testing.T) {
		cp := NewMemoryCheckpointer[state, event]()
		m := NewBuilder[state, event]().Initial(idle).Checkpointer(cp).MustBuild()
		found, err := m.RestoreFrom(ctx)
		if found || err != nil {
			t.Fatalf("found=%v err=%v want false,nil", found, err)
		}
	})

	t.Run("restore unknown state", func(t *testing.T) {
		cp := NewMemoryCheckpointer[state, event]()
		if err := cp.Save(ctx, Snapshot[state, event]{State: state("ghost")}); err != nil {
			t.Fatal(err)
		}
		m := NewBuilder[state, event]().Initial(idle).Checkpointer(cp).MustBuild()
		found, err := m.RestoreFrom(ctx)
		if !found || !errors.Is(err, ErrUnknownState) {
			t.Fatalf("found=%v err=%v want true,ErrUnknownState", found, err)
		}
		if m.Current() != idle {
			t.Fatalf("machine mutated on failed restore: %v", m.Current())
		}
	})
}

func TestMemoryCheckpointer_Empty(t *testing.T) {
	cp := NewMemoryCheckpointer[state, event]()
	_, found, err := cp.Load(context.Background())
	if found || err != nil {
		t.Fatalf("Load empty: found=%v err=%v", found, err)
	}
}

func TestConcurrentFire(t *testing.T) {
	// idle --tick--> idle self-loop; hammer it from many goroutines.
	tick := event("tick")
	m := NewBuilder[state, event]().
		Initial(idle).
		Transition(idle, tick, idle).
		MustBuild()

	const n = 50
	var wg sync.WaitGroup
	wg.Add(n)
	for i := 0; i < n; i++ {
		go func() {
			defer wg.Done()
			if err := m.Fire(context.Background(), tick); err != nil {
				t.Errorf("Fire: %v", err)
			}
		}()
	}
	wg.Wait()
	if got := m.Metrics().Transitions; got != n {
		t.Fatalf("Transitions=%d want %d", got, n)
	}
	if len(m.History()) != n {
		t.Fatalf("history len=%d want %d", len(m.History()), n)
	}
}
