package checkpoint

import (
	"context"
	"testing"

	"github.com/shawnbutts/keystone-core/pkg/statemachine"
)

var (
	_ Store = (*MemoryStore)(nil)
	_ Store = (*SQLiteStore)(nil)
)

type testState string

const (
	stateInit    testState = "init"
	stateRunning testState = "running"
	stateDone    testState = "done"
)

type testEvent string

const (
	eventStart    testEvent = "start"
	eventComplete testEvent = "complete"
)

func buildTestMachine(initial testState, adapter *Adapter[testState, testEvent]) *statemachine.Machine[testState, testEvent] {
	builder := statemachine.New[testState, testEvent](initial).
		AddTransition(stateInit, eventStart, stateRunning).
		AddTransition(stateRunning, eventComplete, stateDone).
		WithName("test")
	if adapter != nil {
		adapter.Wrap(builder)
	}
	return builder.MustBuild()
}

func TestWrap_PersistsOnTransition(t *testing.T) {
	store := NewMemoryStore()
	adapter := New[testState, testEvent](store, "machine-1", "test")
	machine := buildTestMachine(stateInit, adapter)

	if err := machine.Fire(eventStart); err != nil {
		t.Fatalf("fire start: %v", err)
	}

	r, err := store.Load(context.Background(), "machine-1")
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if r == nil {
		t.Fatal("expected checkpoint, got nil")
	}
	if r.State != string(stateRunning) {
		t.Errorf("state: got %q, want %q", r.State, stateRunning)
	}

	if err := machine.Fire(eventComplete); err != nil {
		t.Fatalf("fire complete: %v", err)
	}

	r, err = store.Load(context.Background(), "machine-1")
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if r.State != string(stateDone) {
		t.Errorf("state: got %q, want %q", r.State, stateDone)
	}
}

func TestRestore_AfterSimulatedRestart(t *testing.T) {
	store := NewMemoryStore()
	ctx := context.Background()

	// First "run": build machine, transition, then discard.
	adapter1 := New[testState, testEvent](store, "machine-1", "test")
	machine1 := buildTestMachine(stateInit, adapter1)
	if err := machine1.Fire(eventStart); err != nil {
		t.Fatalf("fire: %v", err)
	}
	machine1.Close()

	// Second "run": restore checkpoint.
	adapter2 := New[testState, testEvent](store, "machine-1", "test")
	restored, ok := adapter2.Restore(ctx)
	if !ok {
		t.Fatal("expected checkpoint")
	}
	if restored != stateRunning {
		t.Errorf("restored: got %q, want %q", restored, stateRunning)
	}

	machine2 := buildTestMachine(restored, adapter2)
	if machine2.State() != stateRunning {
		t.Errorf("state: got %q, want %q", machine2.State(), stateRunning)
	}

	if err := machine2.Fire(eventComplete); err != nil {
		t.Fatalf("fire complete: %v", err)
	}
	if machine2.State() != stateDone {
		t.Errorf("state: got %q, want %q", machine2.State(), stateDone)
	}
}

func TestRestore_NoCheckpoint(t *testing.T) {
	store := NewMemoryStore()
	adapter := New[testState, testEvent](store, "nonexistent", "test")

	_, ok := adapter.Restore(context.Background())
	if ok {
		t.Error("expected no checkpoint")
	}
}

func TestMultipleMachines_Coexist(t *testing.T) {
	store := NewMemoryStore()
	ctx := context.Background()

	a1 := New[testState, testEvent](store, "m1", "test")
	a2 := New[testState, testEvent](store, "m2", "test")

	m1 := buildTestMachine(stateInit, a1)
	m2 := buildTestMachine(stateInit, a2)

	if err := m1.Fire(eventStart); err != nil {
		t.Fatalf("m1 fire: %v", err)
	}
	if err := m2.Fire(eventStart); err != nil {
		t.Fatalf("m2 fire: %v", err)
	}
	if err := m1.Fire(eventComplete); err != nil {
		t.Fatalf("m1 complete: %v", err)
	}

	r1, _ := store.Load(ctx, "m1")
	r2, _ := store.Load(ctx, "m2")

	if r1.State != string(stateDone) {
		t.Errorf("m1 state: got %q, want %q", r1.State, stateDone)
	}
	if r2.State != string(stateRunning) {
		t.Errorf("m2 state: got %q, want %q", r2.State, stateRunning)
	}
}

func TestDelete(t *testing.T) {
	store := NewMemoryStore()
	ctx := context.Background()

	adapter := New[testState, testEvent](store, "machine-1", "test")
	machine := buildTestMachine(stateInit, adapter)
	if err := machine.Fire(eventStart); err != nil {
		t.Fatalf("fire: %v", err)
	}

	r, _ := store.Load(ctx, "machine-1")
	if r == nil {
		t.Fatal("expected checkpoint before delete")
	}

	if err := adapter.Delete(ctx); err != nil {
		t.Fatalf("delete: %v", err)
	}

	r, _ = store.Load(ctx, "machine-1")
	if r != nil {
		t.Error("expected nil after delete")
	}
}

func TestList_ByMachineName(t *testing.T) {
	store := NewMemoryStore()
	ctx := context.Background()

	a1 := New[testState, testEvent](store, "boot-1", "bootstrap")
	a2 := New[testState, testEvent](store, "boot-2", "bootstrap")
	a3 := New[testState, testEvent](store, "upg-1", "upgrade")

	for _, a := range []*Adapter[testState, testEvent]{a1, a2, a3} {
		m := buildTestMachine(stateInit, a)
		if err := m.Fire(eventStart); err != nil {
			t.Fatalf("fire: %v", err)
		}
	}

	list, err := store.List(ctx, "bootstrap")
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(list) != 2 {
		t.Errorf("got %d, want 2", len(list))
	}
}

func TestSaveWithMetadata(t *testing.T) {
	store := NewMemoryStore()
	ctx := context.Background()
	adapter := New[testState, testEvent](store, "meta-1", "test")

	meta := map[string]any{"version": "2.0", "attempt": float64(3)}
	if err := adapter.SaveWithMetadata(ctx, stateRunning, meta); err != nil {
		t.Fatalf("save with metadata: %v", err)
	}

	r, _ := store.Load(ctx, "meta-1")
	if r == nil {
		t.Fatal("expected record")
	}
	if r.Metadata["version"] != "2.0" {
		t.Errorf("metadata version: got %v, want 2.0", r.Metadata["version"])
	}
}

// failingStore always returns an error on Save.
type failingStore struct{ MemoryStore }

func (s *failingStore) Save(_ context.Context, _ *Record) error {
	return context.DeadlineExceeded
}

func TestOnSaveError_CalledOnFailure(t *testing.T) {
	store := &failingStore{}
	var callbackCalled bool
	var callbackErr error

	adapter := New[testState, testEvent](store, "fail-1", "test").
		WithOnSaveError(func(machineID, state string, err error) {
			callbackCalled = true
			callbackErr = err
			if machineID != "fail-1" {
				t.Errorf("machineID: got %q, want %q", machineID, "fail-1")
			}
			if state != string(stateRunning) {
				t.Errorf("state: got %q, want %q", state, stateRunning)
			}
		})

	machine := buildTestMachine(stateInit, adapter)

	// Transition should succeed even though checkpoint fails
	if err := machine.Fire(eventStart); err != nil {
		t.Fatalf("fire should succeed despite checkpoint failure: %v", err)
	}

	if !callbackCalled {
		t.Fatal("OnSaveError callback was not called")
	}
	if callbackErr != context.DeadlineExceeded {
		t.Errorf("callback err: got %v, want DeadlineExceeded", callbackErr)
	}
}

func TestOnSaveError_NotCalledOnSuccess(t *testing.T) {
	store := NewMemoryStore()
	var callbackCalled bool

	adapter := New[testState, testEvent](store, "ok-1", "test").
		WithOnSaveError(func(string, string, error) {
			callbackCalled = true
		})

	machine := buildTestMachine(stateInit, adapter)
	if err := machine.Fire(eventStart); err != nil {
		t.Fatalf("fire: %v", err)
	}

	if callbackCalled {
		t.Error("OnSaveError should not be called on successful save")
	}
}
