package statemachine

import (
	"context"
	"errors"
	"io"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/shawnbutts/keystone-core/internal/logging"
	"github.com/shawnbutts/keystone-core/internal/testing/helpers"
)

// Test state and event types
type TestState string

const (
	StateIdle       TestState = "idle"
	StateConnecting TestState = "connecting"
	StateConnected  TestState = "connected"
	StateFailed     TestState = "failed"
	StateClosed     TestState = "closed"
)

type TestEvent string

const (
	EventConnect    TestEvent = "connect"
	EventConnected  TestEvent = "connected"
	EventFailed     TestEvent = "failed"
	EventDisconnect TestEvent = "disconnect"
	EventClose      TestEvent = "close"
	EventRetry      TestEvent = "retry"
)

func TestNewMachine(t *testing.T) {
	machine, err := New[TestState, TestEvent](StateIdle).Build()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if machine.State() != StateIdle {
		t.Errorf("expected state %v, got %v", StateIdle, machine.State())
	}
}

func TestBasicTransitions(t *testing.T) {
	machine := New[TestState, TestEvent](StateIdle).
		AddTransition(StateIdle, EventConnect, StateConnecting).
		AddTransition(StateConnecting, EventConnected, StateConnected).
		AddTransition(StateConnecting, EventFailed, StateFailed).
		AddTransition(StateConnected, EventDisconnect, StateIdle).
		MustBuild()

	tests := []struct {
		name          string
		event         TestEvent
		expectedState TestState
		shouldFail    bool
	}{
		{"idle to connecting", EventConnect, StateConnecting, false},
		{"connecting to connected", EventConnected, StateConnected, false},
		{"connected to idle", EventDisconnect, StateIdle, false},
		{"idle to connecting again", EventConnect, StateConnecting, false},
		{"connecting failed", EventFailed, StateFailed, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := machine.Fire(tt.event)
			if tt.shouldFail && err == nil {
				t.Error("expected error, got nil")
			}
			if !tt.shouldFail && err != nil {
				t.Errorf("unexpected error: %v", err)
			}
			if machine.State() != tt.expectedState {
				t.Errorf("expected state %v, got %v", tt.expectedState, machine.State())
			}
		})
	}
}

func TestInvalidTransition(t *testing.T) {
	machine := New[TestState, TestEvent](StateIdle).
		AddTransition(StateIdle, EventConnect, StateConnecting).
		MustBuild()

	// Try an undefined transition
	err := machine.Fire(EventDisconnect)
	if err == nil {
		t.Error("expected error for invalid transition")
	}

	var transErr *TransitionError
	if !errors.As(err, &transErr) {
		t.Error("expected TransitionError")
	}

	if !errors.Is(err, ErrInvalidTransition) {
		t.Error("expected ErrInvalidTransition")
	}
}

func TestCallbackPanicRecovery(t *testing.T) {
	errCh := make(chan error, 1)
	logger := logging.NewLogger(logging.Config{
		Name:    "statemachine-test",
		Level:   logging.LevelError,
		Outputs: []logging.Output{logging.NewWriterOutput(io.Discard)},
	})

	machine := New[TestState, TestEvent](StateIdle).
		AddTransition(StateIdle, EventConnect, StateConnecting).
		OnEnter(StateConnecting, func(ctx context.Context, state TestState, from TestState) {
			panic("boom")
		}).
		WithLogger(logger).
		WithErrorHandler(func(ctx context.Context, err error) {
			errCh <- err
		}).
		MustBuild()

	if err := machine.Fire(EventConnect); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	select {
	case <-errCh:
	case <-time.After(200 * time.Millisecond):
		t.Fatal("expected panic error handler to be called")
	}

	metrics := machine.Metrics()
	if metrics.CallbackPanics != 1 {
		t.Errorf("expected 1 callback panic, got %d", metrics.CallbackPanics)
	}
}

func TestGuardConditions(t *testing.T) {
	canConnect := true

	machine := New[TestState, TestEvent](StateIdle).
		AddTransition(StateIdle, EventConnect, StateConnecting).
		WithGuard(func(ctx context.Context, from TestState, event TestEvent) bool {
			return canConnect
		}).
		MustBuild()

	// Should succeed when guard allows
	if err := machine.Fire(EventConnect); err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	// Reset
	machine.Reset()

	// Disable guard
	canConnect = false

	// Should fail when guard blocks
	err := machine.Fire(EventConnect)
	if err == nil {
		t.Error("expected error when guard blocks")
	}

	if !errors.Is(err, ErrGuardFailed) {
		t.Error("expected ErrGuardFailed")
	}
}

func TestConcurrentTransitionError(t *testing.T) {
	var machine *Machine[TestState, TestEvent]
	ready := make(chan struct{}, 2)
	release := make(chan struct{})

	machine = New[TestState, TestEvent](StateIdle).
		AddTransition(StateIdle, EventConnect, StateConnecting).
		WithGuard(func(ctx context.Context, from TestState, event TestEvent) bool {
			ready <- struct{}{}
			<-release
			return true
		}).
		MustBuild()

	errs := make(chan error, 2)
	go func() { errs <- machine.Fire(EventConnect) }()
	go func() { errs <- machine.Fire(EventConnect) }()

	<-ready
	<-ready
	close(release)

	err1 := <-errs
	err2 := <-errs

	if (err1 == nil) == (err2 == nil) {
		t.Fatalf("expected one success and one error, got err1=%v err2=%v", err1, err2)
	}

	if err1 != nil && !errors.Is(err1, ErrConcurrentTransition) {
		t.Fatalf("expected ErrConcurrentTransition, got %v", err1)
	}
	if err2 != nil && !errors.Is(err2, ErrConcurrentTransition) {
		t.Fatalf("expected ErrConcurrentTransition, got %v", err2)
	}

	metrics := machine.Metrics()
	if metrics.ConcurrentTransitions != 1 {
		t.Errorf("expected 1 concurrent transition, got %d", metrics.ConcurrentTransitions)
	}
}

func TestMultipleGuards(t *testing.T) {
	guard1Called := false
	guard2Called := false
	guard1Pass := true
	guard2Pass := true

	machine := New[TestState, TestEvent](StateIdle).
		AddTransition(StateIdle, EventConnect, StateConnecting).
		WithGuard(func(ctx context.Context, from TestState, event TestEvent) bool {
			guard1Called = true
			return guard1Pass
		}).
		WithGuard(func(ctx context.Context, from TestState, event TestEvent) bool {
			guard2Called = true
			return guard2Pass
		}).
		MustBuild()

	// Both guards should be called and pass
	if err := machine.Fire(EventConnect); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if !guard1Called || !guard2Called {
		t.Error("not all guards were called")
	}

	// Reset
	machine.Reset()
	guard1Called = false
	guard2Called = false

	// First guard fails, second should not be called
	guard1Pass = false
	err := machine.Fire(EventConnect)
	if err == nil {
		t.Error("expected error when first guard fails")
	}
	if !guard1Called {
		t.Error("first guard should be called")
	}
	if guard2Called {
		t.Error("second guard should not be called when first fails")
	}
}

func TestCallbacks(t *testing.T) {
	var (
		onEnterCalled  bool
		onExitCalled   bool
		onTransCalled  bool
		enterFromState TestState
		exitToState    TestState
		transFrom      TestState
		transTo        TestState
		transEvent     TestEvent
	)

	machine := New[TestState, TestEvent](StateIdle).
		AddTransition(StateIdle, EventConnect, StateConnecting).
		AddTransition(StateConnecting, EventConnected, StateConnected).
		OnEnter(StateConnecting, func(ctx context.Context, state TestState, from TestState) {
			onEnterCalled = true
			enterFromState = from
		}).
		OnExit(StateIdle, func(ctx context.Context, state TestState, to TestState) {
			onExitCalled = true
			exitToState = to
		}).
		OnTransition(func(ctx context.Context, from, to TestState, event TestEvent) {
			onTransCalled = true
			transFrom = from
			transTo = to
			transEvent = event
		}).
		MustBuild()

	if err := machine.Fire(EventConnect); err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	if !onEnterCalled {
		t.Error("OnEnter callback not called")
	}
	if enterFromState != StateIdle {
		t.Errorf("OnEnter received wrong 'from' state: %v", enterFromState)
	}

	if !onExitCalled {
		t.Error("OnExit callback not called")
	}
	if exitToState != StateConnecting {
		t.Errorf("OnExit received wrong 'to' state: %v", exitToState)
	}

	if !onTransCalled {
		t.Error("OnTransition callback not called")
	}
	if transFrom != StateIdle || transTo != StateConnecting || transEvent != EventConnect {
		t.Errorf("OnTransition received wrong params: from=%v to=%v event=%v",
			transFrom, transTo, transEvent)
	}
}

func TestCallbacksCanAccessState(t *testing.T) {
	var machine *Machine[TestState, TestEvent]
	machine = New[TestState, TestEvent](StateIdle).
		AddTransition(StateIdle, EventConnect, StateConnecting).
		OnEnter(StateConnecting, func(ctx context.Context, state TestState, from TestState) {
			_ = machine.State()
		}).
		MustBuild()

	done := make(chan struct{})
	go func() {
		_ = machine.Fire(EventConnect)
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(1 * time.Second):
		t.Fatal("expected Fire to complete without deadlock")
	}
}

func TestGuardCanAccessState(t *testing.T) {
	var machine *Machine[TestState, TestEvent]
	machine = New[TestState, TestEvent](StateIdle).
		AddTransition(StateIdle, EventConnect, StateConnecting).
		WithGuard(func(ctx context.Context, from TestState, event TestEvent) bool {
			return machine.State() == StateIdle
		}).
		MustBuild()

	if err := machine.Fire(EventConnect); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestCanFire(t *testing.T) {
	machine := New[TestState, TestEvent](StateIdle).
		AddTransition(StateIdle, EventConnect, StateConnecting).
		AddTransition(StateConnecting, EventConnected, StateConnected).
		MustBuild()

	// Can fire from idle
	if !machine.CanFire(EventConnect) {
		t.Error("should be able to fire EventConnect from StateIdle")
	}

	// Cannot fire EventConnected from idle
	if machine.CanFire(EventConnected) {
		t.Error("should not be able to fire EventConnected from StateIdle")
	}
}

func TestCanFireWithGuard(t *testing.T) {
	canConnect := true

	machine := New[TestState, TestEvent](StateIdle).
		AddTransition(StateIdle, EventConnect, StateConnecting).
		WithGuard(func(ctx context.Context, from TestState, event TestEvent) bool {
			return canConnect
		}).
		MustBuild()

	// Can fire when guard allows
	if !machine.CanFireCtx(context.Background(), EventConnect) {
		t.Error("should be able to fire when guard allows")
	}

	canConnect = false

	// Cannot fire when guard blocks
	if machine.CanFireCtx(context.Background(), EventConnect) {
		t.Error("should not be able to fire when guard blocks")
	}
}

func TestAvailableEvents(t *testing.T) {
	machine := New[TestState, TestEvent](StateIdle).
		AddTransition(StateIdle, EventConnect, StateConnecting).
		AddTransition(StateIdle, EventClose, StateClosed).
		AddTransition(StateConnecting, EventConnected, StateConnected).
		MustBuild()

	events := machine.AvailableEvents()
	if len(events) != 2 {
		t.Errorf("expected 2 available events, got %d", len(events))
	}

	// Check that both expected events are present
	eventSet := make(map[TestEvent]bool)
	for _, e := range events {
		eventSet[e] = true
	}
	if !eventSet[EventConnect] || !eventSet[EventClose] {
		t.Error("missing expected events")
	}
}

func TestHistory(t *testing.T) {
	machine := New[TestState, TestEvent](StateIdle).
		AddTransition(StateIdle, EventConnect, StateConnecting).
		AddTransition(StateConnecting, EventConnected, StateConnected).
		AddTransition(StateConnected, EventDisconnect, StateIdle).
		WithHistory(10).
		MustBuild()

	// Perform some transitions
	machine.Fire(EventConnect)
	machine.Fire(EventConnected)
	machine.Fire(EventDisconnect)
	machine.Fire(EventConnect)

	history := machine.History()
	if history == nil {
		t.Fatal("history should not be nil")
	}

	records := history.All()
	if len(records) != 4 {
		t.Errorf("expected 4 history records, got %d", len(records))
	}

	// Verify order (oldest first)
	if records[0].From != StateIdle || records[0].To != StateConnecting {
		t.Error("first record incorrect")
	}
	if records[3].From != StateIdle || records[3].To != StateConnecting {
		t.Error("last record incorrect")
	}
}

func TestHistoryCircularBuffer(t *testing.T) {
	machine := New[TestState, TestEvent](StateIdle).
		AddTransition(StateIdle, EventConnect, StateConnecting).
		AddTransition(StateConnecting, EventFailed, StateIdle).
		WithHistory(3). // Small buffer
		MustBuild()

	// Perform more transitions than buffer size
	for i := 0; i < 5; i++ {
		machine.Fire(EventConnect)
		machine.Fire(EventFailed)
	}

	history := machine.History()
	records := history.All()

	// Should only have last 3
	if len(records) != 3 {
		t.Errorf("expected 3 records, got %d", len(records))
	}

	// Total count should reflect all transitions
	if history.Count() != 10 {
		t.Errorf("expected count 10, got %d", history.Count())
	}
}

func TestHistoryFilter(t *testing.T) {
	machine := New[TestState, TestEvent](StateIdle).
		AddTransition(StateIdle, EventConnect, StateConnecting).
		AddTransition(StateConnecting, EventConnected, StateConnected).
		AddTransition(StateConnected, EventDisconnect, StateIdle).
		WithHistory(100).
		MustBuild()

	machine.Fire(EventConnect)
	machine.Fire(EventConnected)
	machine.Fire(EventDisconnect)
	machine.Fire(EventConnect)
	machine.Fire(EventConnected)

	history := machine.History()

	// Filter by from state
	fromIdle := history.TransitionsFrom(StateIdle)
	if len(fromIdle) != 2 {
		t.Errorf("expected 2 transitions from idle, got %d", len(fromIdle))
	}

	// Filter by to state
	toConnected := history.TransitionsTo(StateConnected)
	if len(toConnected) != 2 {
		t.Errorf("expected 2 transitions to connected, got %d", len(toConnected))
	}

	// Filter by event
	byConnect := history.TransitionsByEvent(EventConnect)
	if len(byConnect) != 2 {
		t.Errorf("expected 2 transitions via connect, got %d", len(byConnect))
	}
}

func TestReset(t *testing.T) {
	machine := New[TestState, TestEvent](StateIdle).
		AddTransition(StateIdle, EventConnect, StateConnecting).
		MustBuild()

	machine.Fire(EventConnect)
	if machine.State() != StateConnecting {
		t.Errorf("expected connecting, got %v", machine.State())
	}

	machine.Reset()
	if machine.State() != StateIdle {
		t.Errorf("expected idle after reset, got %v", machine.State())
	}
}

func TestResetWithCallbacks(t *testing.T) {
	exitCalled := false
	enterCalled := false

	machine := New[TestState, TestEvent](StateIdle).
		AddTransition(StateIdle, EventConnect, StateConnecting).
		OnExit(StateConnecting, func(ctx context.Context, state, to TestState) {
			exitCalled = true
		}).
		OnEnter(StateIdle, func(ctx context.Context, state, from TestState) {
			enterCalled = true
		}).
		MustBuild()

	machine.Fire(EventConnect)
	machine.ResetWithCallbacks(context.Background())

	if !exitCalled {
		t.Error("OnExit should be called during reset")
	}
	if !enterCalled {
		t.Error("OnEnter should be called during reset")
	}
}

func TestClose(t *testing.T) {
	machine := New[TestState, TestEvent](StateIdle).
		AddTransition(StateIdle, EventConnect, StateConnecting).
		MustBuild()

	machine.Close()

	err := machine.Fire(EventConnect)
	if err == nil {
		t.Error("expected error after close")
	}
	if !errors.Is(err, ErrMachineClosed) {
		t.Error("expected ErrMachineClosed")
	}

	if !machine.IsClosed() {
		t.Error("machine should report as closed")
	}
}

func TestConcurrency(t *testing.T) {
	machine := New[TestState, TestEvent](StateIdle).
		AddTransition(StateIdle, EventConnect, StateConnecting).
		AddTransition(StateConnecting, EventConnected, StateConnected).
		AddTransition(StateConnected, EventDisconnect, StateIdle).
		AddTransition(StateConnecting, EventFailed, StateIdle).
		WithHistory(1000).
		MustBuild()

	var wg sync.WaitGroup
	var successCount int64

	// Multiple goroutines trying to transition
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 100; j++ {
				// Try random transitions
				events := []TestEvent{EventConnect, EventConnected, EventDisconnect, EventFailed}
				for _, event := range events {
					if machine.Fire(event) == nil {
						atomic.AddInt64(&successCount, 1)
					}
				}
			}
		}()
	}

	wg.Wait()

	// Should have some successful transitions
	if successCount == 0 {
		t.Error("expected some successful transitions")
	}

	// Machine should be in a valid state
	state := machine.State()
	validStates := map[TestState]bool{
		StateIdle:       true,
		StateConnecting: true,
		StateConnected:  true,
	}
	if !validStates[state] {
		t.Errorf("machine in invalid state: %v", state)
	}
}

func TestStateConfiguration(t *testing.T) {
	machine := New[TestState, TestEvent](StateIdle).
		Configure(StateIdle, func(cfg *StateConfiguration[TestState, TestEvent]) {
			cfg.Permit(EventConnect, StateConnecting).
				OnEnter(func(ctx context.Context, state, from TestState) {
					// Entry callback
				}).
				OnExit(func(ctx context.Context, state, to TestState) {
					// Exit callback
				})
		}).
		Configure(StateConnecting, func(cfg *StateConfiguration[TestState, TestEvent]) {
			cfg.Permit(EventConnected, StateConnected).
				Permit(EventFailed, StateFailed)
		}).
		MustBuild()

	if err := machine.Fire(EventConnect); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if machine.State() != StateConnecting {
		t.Errorf("expected connecting, got %v", machine.State())
	}
}

func TestPermitReentry(t *testing.T) {
	enterCount := 0

	machine := New[TestState, TestEvent](StateConnected).
		PermitReentry(StateConnected, EventRetry).
		OnEnter(StateConnected, func(ctx context.Context, state, from TestState) {
			enterCount++
		}).
		MustBuild()

	// Fire reentry event multiple times
	for i := 0; i < 3; i++ {
		if err := machine.Fire(EventRetry); err != nil {
			t.Errorf("unexpected error: %v", err)
		}
	}

	if enterCount != 3 {
		t.Errorf("expected 3 entry callbacks, got %d", enterCount)
	}

	// State should remain the same
	if machine.State() != StateConnected {
		t.Errorf("state should remain connected")
	}
}

func TestIsInState(t *testing.T) {
	machine := New[TestState, TestEvent](StateIdle).
		AddTransition(StateIdle, EventConnect, StateConnecting).
		MustBuild()

	if !machine.IsInState(StateIdle) {
		t.Error("expected to be in idle state")
	}
	if machine.IsInState(StateConnecting) {
		t.Error("should not be in connecting state")
	}

	machine.Fire(EventConnect)

	if machine.IsInState(StateIdle) {
		t.Error("should not be in idle state after transition")
	}
	if !machine.IsInState(StateConnecting) {
		t.Error("expected to be in connecting state")
	}
}

func TestIsInAnyState(t *testing.T) {
	machine := New[TestState, TestEvent](StateIdle).
		AddTransition(StateIdle, EventConnect, StateConnecting).
		MustBuild()

	if !machine.IsInAnyState(StateIdle, StateConnecting) {
		t.Error("expected to match idle")
	}
	if machine.IsInAnyState(StateConnecting, StateConnected) {
		t.Error("should not match connecting or connected")
	}

	machine.Fire(EventConnect)

	if !machine.IsInAnyState(StateIdle, StateConnecting) {
		t.Error("expected to match connecting")
	}
}

func TestAllStates(t *testing.T) {
	machine := New[TestState, TestEvent](StateIdle).
		AddTransition(StateIdle, EventConnect, StateConnecting).
		AddTransition(StateConnecting, EventConnected, StateConnected).
		AddTransition(StateConnected, EventDisconnect, StateIdle).
		MustBuild()

	states := machine.AllStates()

	stateSet := make(map[TestState]bool)
	for _, s := range states {
		stateSet[s] = true
	}

	expected := []TestState{StateIdle, StateConnecting, StateConnected}
	for _, e := range expected {
		if !stateSet[e] {
			t.Errorf("missing state: %v", e)
		}
	}
}

func TestAllEvents(t *testing.T) {
	machine := New[TestState, TestEvent](StateIdle).
		AddTransition(StateIdle, EventConnect, StateConnecting).
		AddTransition(StateConnecting, EventConnected, StateConnected).
		MustBuild()

	events := machine.AllEvents()

	eventSet := make(map[TestEvent]bool)
	for _, e := range events {
		eventSet[e] = true
	}

	expected := []TestEvent{EventConnect, EventConnected}
	for _, e := range expected {
		if !eventSet[e] {
			t.Errorf("missing event: %v", e)
		}
	}
}

func TestTransitionsFrom(t *testing.T) {
	machine := New[TestState, TestEvent](StateIdle).
		AddTransition(StateIdle, EventConnect, StateConnecting).
		AddTransition(StateIdle, EventClose, StateClosed).
		AddTransition(StateConnecting, EventConnected, StateConnected).
		MustBuild()

	transitions := machine.TransitionsFrom(StateIdle)

	if len(transitions) != 2 {
		t.Errorf("expected 2 transitions from idle, got %d", len(transitions))
	}

	if transitions[EventConnect] != StateConnecting {
		t.Error("missing connect transition")
	}
	if transitions[EventClose] != StateClosed {
		t.Error("missing close transition")
	}
}

func TestIgnoreNoOp(t *testing.T) {
	enterCount := 0
	exitCount := 0
	transitionCount := 0

	machine := New[TestState, TestEvent](StateIdle).
		AddTransition(StateIdle, EventConnect, StateConnecting).
		Ignore(StateIdle, EventRetry).
		OnEnter(StateIdle, func(ctx context.Context, state, from TestState) {
			enterCount++
		}).
		OnExit(StateIdle, func(ctx context.Context, state, to TestState) {
			exitCount++
		}).
		OnTransition(func(ctx context.Context, from, to TestState, event TestEvent) {
			transitionCount++
		}).
		WithHistory(10).
		MustBuild()

	if !machine.CanFire(EventRetry) {
		t.Fatal("expected ignored event to be fireable")
	}

	err := machine.Fire(EventRetry)
	if err != nil {
		t.Fatalf("unexpected error firing ignored event: %v", err)
	}

	if machine.State() != StateIdle {
		t.Fatalf("expected state to remain idle, got %v", machine.State())
	}

	if enterCount != 0 || exitCount != 0 || transitionCount != 0 {
		t.Fatalf("ignored event should not trigger callbacks")
	}

	if machine.History().Size() != 0 {
		t.Fatal("ignored event should not be recorded in history")
	}

	events := machine.AvailableEvents()
	eventSet := make(map[TestEvent]bool)
	for _, e := range events {
		eventSet[e] = true
	}
	if !eventSet[EventRetry] {
		t.Fatal("expected ignored event to be available")
	}
}

func TestFireWithMetadata(t *testing.T) {
	machine := New[TestState, TestEvent](StateIdle).
		AddTransition(StateIdle, EventConnect, StateConnecting).
		WithHistory(10).
		MustBuild()

	metadata := map[string]any{
		"reason":    "user requested",
		"requestID": "12345",
	}

	err := machine.FireWithMetadata(context.Background(), EventConnect, metadata)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	history := machine.History()
	latest := history.Latest()
	if latest == nil {
		t.Fatal("expected history record")
	}

	if latest.Metadata["reason"] != "user requested" {
		t.Error("metadata not recorded")
	}
	if latest.Metadata["requestID"] != "12345" {
		t.Error("metadata not recorded")
	}

	metadata["reason"] = "changed"
	if latest.Metadata["reason"] != "user requested" {
		t.Error("metadata should be copied to history")
	}
}

func TestStateDuration(t *testing.T) {
	machine := New[TestState, TestEvent](StateIdle).
		AddTransition(StateIdle, EventConnect, StateConnecting).
		MustBuild()

	if err := helpers.WaitForTimeout(100*time.Millisecond, 2*time.Millisecond, func() (bool, error) {
		return machine.StateDuration() >= 10*time.Millisecond, nil
	}); err != nil {
		t.Fatalf("expected state duration to advance: %v", err)
	}

	duration := machine.StateDuration()
	if duration < 10*time.Millisecond {
		t.Errorf("expected duration >= 10ms, got %v", duration)
	}

	machine.Fire(EventConnect)

	// Duration should reset
	if machine.StateDuration() > 5*time.Millisecond {
		t.Error("duration should reset after transition")
	}
}

func TestMachineName(t *testing.T) {
	machine := New[TestState, TestEvent](StateIdle).
		WithName("connection-state").
		MustBuild()

	if machine.Name() != "connection-state" {
		t.Errorf("expected name 'connection-state', got '%s'", machine.Name())
	}
}

func TestHistoryLast(t *testing.T) {
	machine := New[TestState, TestEvent](StateIdle).
		AddTransition(StateIdle, EventConnect, StateConnecting).
		AddTransition(StateConnecting, EventFailed, StateIdle).
		WithHistory(100).
		MustBuild()

	// Make 10 transitions
	for i := 0; i < 5; i++ {
		machine.Fire(EventConnect)
		machine.Fire(EventFailed)
	}

	history := machine.History()

	// Get last 3
	last3 := history.Last(3)
	if len(last3) != 3 {
		t.Errorf("expected 3 records, got %d", len(last3))
	}

	// Should be in chronological order (oldest first)
	// The last 3 transitions would be: failed, connect, failed
	if last3[2].Event != EventFailed {
		t.Error("last record should be EventFailed")
	}
}

func TestHistoryLatest(t *testing.T) {
	machine := New[TestState, TestEvent](StateIdle).
		AddTransition(StateIdle, EventConnect, StateConnecting).
		WithHistory(10).
		MustBuild()

	// No transitions yet
	if machine.History().Latest() != nil {
		t.Error("expected nil for empty history")
	}

	machine.Fire(EventConnect)

	latest := machine.History().Latest()
	if latest == nil {
		t.Fatal("expected latest record")
	}

	if latest.Event != EventConnect {
		t.Error("latest should be EventConnect")
	}
}
