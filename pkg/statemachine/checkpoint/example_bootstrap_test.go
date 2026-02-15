package checkpoint_test

import (
	"context"
	"testing"

	"github.com/shawnbutts/keystone-core/pkg/statemachine"
	"github.com/shawnbutts/keystone-core/pkg/statemachine/checkpoint"
)

// Bootstrap state machine types for the example.
type BootPhase string

const (
	PhaseInit       BootPhase = "initializing"
	PhaseValidating BootPhase = "validating"
	PhaseInstalling BootPhase = "installing"
	PhaseConfigured BootPhase = "configured"
	PhaseComplete   BootPhase = "complete"
)

type BootEvent string

const (
	EvValidate BootEvent = "validate"
	EvInstall  BootEvent = "install"
	EvConfigure BootEvent = "configure"
	EvFinish   BootEvent = "finish"
)

func buildBootstrapMachine(initial BootPhase, adapter *checkpoint.Adapter[BootPhase, BootEvent]) *statemachine.Machine[BootPhase, BootEvent] {
	builder := statemachine.New[BootPhase, BootEvent](initial).
		WithName("bootstrap").
		AddTransition(PhaseInit, EvValidate, PhaseValidating).
		AddTransition(PhaseValidating, EvInstall, PhaseInstalling).
		AddTransition(PhaseInstalling, EvConfigure, PhaseConfigured).
		AddTransition(PhaseConfigured, EvFinish, PhaseComplete)
	if adapter != nil {
		adapter.Wrap(builder)
	}
	return builder.MustBuild()
}

// TestExample_Bootstrap_CrashRecovery demonstrates checkpointing a
// bootstrap-like state machine and recovering after a simulated crash.
func TestExample_Bootstrap_CrashRecovery(t *testing.T) {
	store := checkpoint.NewMemoryStore()
	ctx := context.Background()

	// --- First run: progress through several phases, then "crash" ---
	adapter1 := checkpoint.New[BootPhase, BootEvent](store, "node-abc", "bootstrap")
	machine1 := buildBootstrapMachine(PhaseInit, adapter1)

	if err := machine1.Fire(EvValidate); err != nil {
		t.Fatalf("fire validate: %v", err)
	}
	if err := machine1.Fire(EvInstall); err != nil {
		t.Fatalf("fire install: %v", err)
	}

	// Verify checkpoint was saved at "installing".
	r, _ := store.Load(ctx, "node-abc")
	if r == nil || r.State != string(PhaseInstalling) {
		t.Fatalf("expected checkpoint at installing, got %v", r)
	}

	// Simulate crash: discard the machine.
	machine1.Close()

	// --- Second run: restore and continue ---
	adapter2 := checkpoint.New[BootPhase, BootEvent](store, "node-abc", "bootstrap")
	restored, ok := adapter2.Restore(ctx)
	if !ok {
		t.Fatal("expected checkpoint to be found")
	}
	if restored != PhaseInstalling {
		t.Errorf("restored state: got %q, want %q", restored, PhaseInstalling)
	}

	// Build a new machine starting from the restored state.
	machine2 := buildBootstrapMachine(restored, adapter2)
	if machine2.State() != PhaseInstalling {
		t.Errorf("initial state: got %q, want %q", machine2.State(), PhaseInstalling)
	}

	// Continue from where we left off.
	if err := machine2.Fire(EvConfigure); err != nil {
		t.Fatalf("fire configure: %v", err)
	}
	if err := machine2.Fire(EvFinish); err != nil {
		t.Fatalf("fire finish: %v", err)
	}

	if machine2.State() != PhaseComplete {
		t.Errorf("final state: got %q, want %q", machine2.State(), PhaseComplete)
	}

	// Checkpoint reflects final state.
	r, _ = store.Load(ctx, "node-abc")
	if r.State != string(PhaseComplete) {
		t.Errorf("final checkpoint: got %q, want %q", r.State, PhaseComplete)
	}
}

// TestExample_Bootstrap_NoCheckpoint shows that a fresh start works when
// no checkpoint exists.
func TestExample_Bootstrap_NoCheckpoint(t *testing.T) {
	store := checkpoint.NewMemoryStore()
	ctx := context.Background()

	adapter := checkpoint.New[BootPhase, BootEvent](store, "node-new", "bootstrap")
	restored, ok := adapter.Restore(ctx)
	if ok {
		t.Fatalf("expected no checkpoint, got %q", restored)
	}

	// Start from default initial state.
	machine := buildBootstrapMachine(PhaseInit, adapter)
	if err := machine.Fire(EvValidate); err != nil {
		t.Fatalf("fire: %v", err)
	}
	if machine.State() != PhaseValidating {
		t.Errorf("state: got %q, want %q", machine.State(), PhaseValidating)
	}
}
