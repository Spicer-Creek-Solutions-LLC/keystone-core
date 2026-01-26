package agent

import (
	"errors"
	"testing"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/shawnbutts/keystone-core/internal/testing/helpers"
	"github.com/shawnbutts/keystone-core/pkg/statemachine"
)

func TestManagedHybridMode_InitialState(t *testing.T) {
	config := DefaultHybridModeConfig("test-agent")
	config.ExternalNATSURLs = []string{"nats://localhost:4222"}

	mhm, err := NewManagedHybridMode(config, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if mhm.State() != HybridModeStateIdle {
		t.Errorf("expected idle state, got %v", mhm.State())
	}
	if !mhm.IsIdle() {
		t.Error("expected IsIdle() to be true")
	}
	if mhm.IsRunning() {
		t.Error("expected IsRunning() to be false")
	}
	if mhm.IsActive() {
		t.Error("expected IsActive() to be false")
	}
	if mhm.Role() != ConnectionRoleUndetermined {
		t.Errorf("expected undetermined role, got %v", mhm.Role())
	}
}

func TestManagedHybridMode_StartTransition(t *testing.T) {
	config := DefaultHybridModeConfig("test-agent")
	config.ExternalNATSURLs = []string{"nats://localhost:4222"}

	mhm, err := NewManagedHybridMode(config, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if err := mhm.Start(); err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	if mhm.State() != HybridModeStateDetermining {
		t.Errorf("expected determining state, got %v", mhm.State())
	}
	if !mhm.IsDetermining() {
		t.Error("expected IsDetermining() to be true")
	}
	if !mhm.IsRunning() {
		t.Error("expected IsRunning() to be true")
	}
}

func TestManagedHybridMode_ClientWorkflow(t *testing.T) {
	config := DefaultHybridModeConfig("test-agent")
	config.ExternalNATSURLs = []string{"nats://localhost:4222"}

	mhm, err := NewManagedHybridMode(config, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Start
	mhm.Start()

	// Mark as client
	if err := mhm.MarkDeterminingClient(NetworkReachabilityRestricted); err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	if mhm.State() != HybridModeStateConnecting {
		t.Errorf("expected connecting state, got %v", mhm.State())
	}
	if mhm.Role() != ConnectionRoleClient {
		t.Errorf("expected client role, got %v", mhm.Role())
	}
	if mhm.Reachability() != NetworkReachabilityRestricted {
		t.Errorf("expected restricted reachability, got %v", mhm.Reachability())
	}

	// Mark connected
	if err := mhm.MarkConnected(nil); err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	if !mhm.IsActive() {
		t.Error("expected IsActive() to be true")
	}
	if mhm.ConnectionCount() != 1 {
		t.Errorf("expected connection count 1, got %d", mhm.ConnectionCount())
	}
}

func TestManagedHybridMode_HostWorkflow(t *testing.T) {
	config := DefaultHybridModeConfig("test-agent")
	config.EmbeddedConfig = DefaultEmbeddedNATSConfig()

	mhm, err := NewManagedHybridMode(config, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Start
	mhm.Start()

	// Mark as host
	if err := mhm.MarkDeterminingHost(NetworkReachabilityDirect); err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	if mhm.State() != HybridModeStateHosting {
		t.Errorf("expected hosting state, got %v", mhm.State())
	}
	if mhm.Role() != ConnectionRoleHost {
		t.Errorf("expected host role, got %v", mhm.Role())
	}

	// Mark server started
	if err := mhm.MarkServerStarted(nil); err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	if !mhm.IsActive() {
		t.Error("expected IsActive() to be true")
	}
}

func TestManagedHybridMode_LeafWorkflow(t *testing.T) {
	config := DefaultHybridModeConfig("test-agent")
	config.EmbeddedConfig = DefaultEmbeddedNATSConfig()
	config.EmbeddedConfig.Mode = EmbeddedNATSModeLeaf

	mhm, err := NewManagedHybridMode(config, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	mhm.Start()
	mhm.MarkDeterminingLeaf(NetworkReachabilityNAT)

	if mhm.Role() != ConnectionRoleLeaf {
		t.Errorf("expected leaf role, got %v", mhm.Role())
	}
	if mhm.State() != HybridModeStateHosting {
		t.Errorf("expected hosting state for leaf, got %v", mhm.State())
	}

	mhm.MarkServerStarted(nil)
	if !mhm.IsActive() {
		t.Error("expected IsActive() to be true")
	}
}

func TestManagedHybridMode_FallbackToHost(t *testing.T) {
	config := DefaultHybridModeConfig("test-agent")
	config.ExternalNATSURLs = []string{"nats://localhost:4222"}
	config.EmbeddedConfig = DefaultEmbeddedNATSConfig()
	config.FallbackToHost = true

	mhm, err := NewManagedHybridMode(config, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	mhm.Start()
	mhm.MarkDeterminingClient(NetworkReachabilityRestricted)

	// Connection fails, fallback to host
	if err := mhm.FallbackToHost(ConnectionRoleHost); err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	if mhm.State() != HybridModeStateHosting {
		t.Errorf("expected hosting state, got %v", mhm.State())
	}
	if mhm.Role() != ConnectionRoleHost {
		t.Errorf("expected host role after fallback, got %v", mhm.Role())
	}

	// Server starts
	mhm.MarkServerStarted(nil)
	if !mhm.IsActive() {
		t.Error("expected IsActive() to be true after fallback")
	}
}

func TestManagedHybridMode_FallbackToClient(t *testing.T) {
	config := DefaultHybridModeConfig("test-agent")
	config.ExternalNATSURLs = []string{"nats://localhost:4222"}
	config.EmbeddedConfig = DefaultEmbeddedNATSConfig()
	config.FallbackToClient = true

	mhm, err := NewManagedHybridMode(config, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	mhm.Start()
	mhm.MarkDeterminingHost(NetworkReachabilityDirect)

	// Server fails, fallback to client
	if err := mhm.FallbackToClient(); err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	if mhm.State() != HybridModeStateConnecting {
		t.Errorf("expected connecting state, got %v", mhm.State())
	}
	if mhm.Role() != ConnectionRoleClient {
		t.Errorf("expected client role after fallback, got %v", mhm.Role())
	}

	// Connection succeeds
	mhm.MarkConnected(nil)
	if !mhm.IsActive() {
		t.Error("expected IsActive() to be true after fallback")
	}
}

func TestManagedHybridMode_Reconnection(t *testing.T) {
	config := DefaultHybridModeConfig("test-agent")
	config.ExternalNATSURLs = []string{"nats://localhost:4222"}

	mhm, err := NewManagedHybridMode(config, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Get to active state
	mhm.Start()
	mhm.MarkDeterminingClient(NetworkReachabilityRestricted)
	mhm.MarkConnected(nil)

	// Reconnect
	if err := mhm.MarkReconnected(nil); err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	if !mhm.IsActive() {
		t.Error("expected to stay active after reconnect")
	}
	if mhm.ReconnectCount() != 1 {
		t.Errorf("expected reconnect count 1, got %d", mhm.ReconnectCount())
	}
}

func TestManagedHybridMode_ConnectionLost(t *testing.T) {
	config := DefaultHybridModeConfig("test-agent")
	config.ExternalNATSURLs = []string{"nats://localhost:4222"}

	mhm, err := NewManagedHybridMode(config, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Get to active state
	mhm.Start()
	mhm.MarkDeterminingClient(NetworkReachabilityRestricted)
	mhm.MarkConnected(nil)

	// Connection lost
	lostErr := errors.New("connection lost")
	if err := mhm.MarkConnectionLost(lostErr); err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	if mhm.State() != HybridModeStateDetermining {
		t.Errorf("expected determining state after connection lost, got %v", mhm.State())
	}
	if mhm.Error() != lostErr {
		t.Errorf("expected error to be stored")
	}
}

func TestManagedHybridMode_Failure(t *testing.T) {
	states := []struct {
		name  string
		setup func(*ManagedHybridMode)
	}{
		{"fail from determining", func(mhm *ManagedHybridMode) {
			mhm.Start()
		}},
		{"fail from connecting", func(mhm *ManagedHybridMode) {
			mhm.Start()
			mhm.MarkDeterminingClient(NetworkReachabilityRestricted)
		}},
		{"fail from hosting", func(mhm *ManagedHybridMode) {
			mhm.Start()
			mhm.MarkDeterminingHost(NetworkReachabilityDirect)
		}},
		{"fail from active", func(mhm *ManagedHybridMode) {
			mhm.Start()
			mhm.MarkDeterminingClient(NetworkReachabilityRestricted)
			mhm.MarkConnected(nil)
		}},
	}

	for _, tt := range states {
		t.Run(tt.name, func(t *testing.T) {
			config := DefaultHybridModeConfig("test-agent")
			config.ExternalNATSURLs = []string{"nats://localhost:4222"}
			config.EmbeddedConfig = DefaultEmbeddedNATSConfig()

			mhm, _ := NewManagedHybridMode(config, nil)
			tt.setup(mhm)

			testErr := errors.New("test failure")
			if err := mhm.Fail(testErr); err != nil {
				t.Errorf("unexpected error: %v", err)
			}

			if !mhm.IsFailed() {
				t.Error("expected IsFailed() to be true")
			}
			if mhm.Error() != testErr {
				t.Errorf("expected error to be stored")
			}
		})
	}
}

func TestManagedHybridMode_Stop(t *testing.T) {
	states := []struct {
		name  string
		setup func(*ManagedHybridMode)
	}{
		{"stop from determining", func(mhm *ManagedHybridMode) {
			mhm.Start()
		}},
		{"stop from connecting", func(mhm *ManagedHybridMode) {
			mhm.Start()
			mhm.MarkDeterminingClient(NetworkReachabilityRestricted)
		}},
		{"stop from hosting", func(mhm *ManagedHybridMode) {
			mhm.Start()
			mhm.MarkDeterminingHost(NetworkReachabilityDirect)
		}},
		{"stop from active", func(mhm *ManagedHybridMode) {
			mhm.Start()
			mhm.MarkDeterminingClient(NetworkReachabilityRestricted)
			mhm.MarkConnected(nil)
		}},
	}

	for _, tt := range states {
		t.Run(tt.name, func(t *testing.T) {
			config := DefaultHybridModeConfig("test-agent")
			config.ExternalNATSURLs = []string{"nats://localhost:4222"}
			config.EmbeddedConfig = DefaultEmbeddedNATSConfig()

			mhm, _ := NewManagedHybridMode(config, nil)
			tt.setup(mhm)

			if err := mhm.Stop(); err != nil {
				t.Errorf("unexpected error: %v", err)
			}

			if !mhm.IsIdle() {
				t.Error("expected IsIdle() to be true after stop")
			}
			if mhm.Role() != ConnectionRoleUndetermined {
				t.Errorf("expected role to be undetermined after stop, got %v", mhm.Role())
			}
		})
	}
}

func TestManagedHybridMode_Reset(t *testing.T) {
	config := DefaultHybridModeConfig("test-agent")
	config.ExternalNATSURLs = []string{"nats://localhost:4222"}

	mhm, _ := NewManagedHybridMode(config, nil)

	// Get to failed state
	mhm.Start()
	mhm.Fail(errors.New("test"))

	// Reset
	if err := mhm.Reset(); err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	if !mhm.IsIdle() {
		t.Error("expected IsIdle() to be true after reset")
	}
}

func TestManagedHybridMode_InvalidTransitions(t *testing.T) {
	config := DefaultHybridModeConfig("test-agent")
	config.ExternalNATSURLs = []string{"nats://localhost:4222"}

	mhm, _ := NewManagedHybridMode(config, nil)

	// Cannot connect before starting
	err := mhm.MarkConnected(nil)
	if err == nil {
		t.Error("expected error for invalid transition")
	}
	if !errors.Is(err, statemachine.ErrInvalidTransition) {
		t.Errorf("expected ErrInvalidTransition, got %v", err)
	}

	// Start properly
	mhm.Start()

	// Cannot connect before determining
	err = mhm.MarkConnected(nil)
	if err == nil {
		t.Error("expected error for invalid transition")
	}
}

func TestManagedHybridMode_Callbacks(t *testing.T) {
	var stateChanges []HybridModeState
	var roleChanges []ConnectionRole
	var connectionReadyCalls int
	var connectionLostCalls int
	var fallbackCalls int
	var failedCalls int

	callbacks := &HybridModeCallbacks{
		OnStateChange: func(state HybridModeState) {
			stateChanges = append(stateChanges, state)
		},
		OnRoleChange: func(role ConnectionRole) {
			roleChanges = append(roleChanges, role)
		},
		OnConnectionReady: func(role ConnectionRole, conn *nats.Conn) {
			connectionReadyCalls++
		},
		OnConnectionLost: func(role ConnectionRole, err error) {
			connectionLostCalls++
		},
		OnFallback: func(from, to ConnectionRole) {
			fallbackCalls++
		},
		OnFailed: func(state HybridModeState, err error) {
			failedCalls++
		},
	}

	config := DefaultHybridModeConfig("test-agent")
	config.ExternalNATSURLs = []string{"nats://localhost:4222"}
	config.EmbeddedConfig = DefaultEmbeddedNATSConfig()

	mhm, _ := NewManagedHybridMode(config, callbacks)

	// Run through workflow
	mhm.Start()
	mhm.MarkDeterminingClient(NetworkReachabilityRestricted)
	mhm.MarkConnected(nil)

	if len(stateChanges) < 2 {
		t.Errorf("expected at least 2 state changes, got %d", len(stateChanges))
	}
	if connectionReadyCalls != 1 {
		t.Errorf("expected 1 connection ready call, got %d", connectionReadyCalls)
	}

	// Test fallback
	mhm2, _ := NewManagedHybridMode(config, callbacks)
	mhm2.Start()
	mhm2.MarkDeterminingClient(NetworkReachabilityRestricted)
	mhm2.FallbackToHost(ConnectionRoleHost)

	if fallbackCalls != 1 {
		t.Errorf("expected 1 fallback call, got %d", fallbackCalls)
	}

	// Test failure
	mhm3, _ := NewManagedHybridMode(config, callbacks)
	mhm3.Start()
	mhm3.Fail(errors.New("test"))

	if failedCalls != 1 {
		t.Errorf("expected 1 failed call, got %d", failedCalls)
	}
}

func TestManagedHybridMode_Duration(t *testing.T) {
	config := DefaultHybridModeConfig("test-agent")
	config.ExternalNATSURLs = []string{"nats://localhost:4222"}

	mhm, _ := NewManagedHybridMode(config, nil)

	// No duration before start
	if mhm.Duration() != 0 {
		t.Error("expected 0 duration before start")
	}

	mhm.Start()
	if err := helpers.WaitForTimeout(100*time.Millisecond, 5*time.Millisecond, func() (bool, error) {
		return mhm.Duration() > 0, nil
	}); err != nil {
		t.Fatalf("expected duration to start: %v", err)
	}

	// Should have non-zero duration
	if mhm.Duration() == 0 {
		t.Error("expected non-zero duration after start")
	}

	mhm.MarkDeterminingClient(NetworkReachabilityRestricted)
	mhm.MarkConnected(nil)
	if err := helpers.WaitForTimeout(100*time.Millisecond, 5*time.Millisecond, func() (bool, error) {
		return mhm.ActiveDuration() > 0, nil
	}); err != nil {
		t.Fatalf("expected active duration to start: %v", err)
	}

	// Should have active duration
	if mhm.ActiveDuration() == 0 {
		t.Error("expected non-zero active duration")
	}

	// State duration should be non-zero
	if mhm.StateDuration() == 0 {
		t.Error("expected non-zero state duration")
	}
}

func TestManagedHybridMode_ReachabilityCheck(t *testing.T) {
	config := DefaultHybridModeConfig("test-agent")
	config.ExternalNATSURLs = []string{"nats://localhost:4222"}

	mhm, _ := NewManagedHybridMode(config, nil)

	// Get to active state
	mhm.Start()
	mhm.MarkDeterminingClient(NetworkReachabilityRestricted)
	mhm.MarkConnected(nil)

	// Check reachability
	if err := mhm.MarkReachabilityChecked(NetworkReachabilityNAT); err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	if mhm.Reachability() != NetworkReachabilityNAT {
		t.Errorf("expected NAT reachability, got %v", mhm.Reachability())
	}
	if !mhm.IsActive() {
		t.Error("expected to stay active after reachability check")
	}
}

func TestManagedHybridMode_History(t *testing.T) {
	config := DefaultHybridModeConfig("test-agent")
	config.ExternalNATSURLs = []string{"nats://localhost:4222"}

	mhm, _ := NewManagedHybridMode(config, nil)

	mhm.Start()
	mhm.MarkDeterminingClient(NetworkReachabilityRestricted)
	mhm.MarkConnected(nil)

	history := mhm.History()
	if history == nil {
		t.Fatal("history should not be nil")
	}

	records := history.All()
	if len(records) != 3 {
		t.Errorf("expected 3 history records, got %d", len(records))
	}
}

func TestManagedHybridMode_AvailableEvents(t *testing.T) {
	config := DefaultHybridModeConfig("test-agent")
	config.ExternalNATSURLs = []string{"nats://localhost:4222"}

	mhm, _ := NewManagedHybridMode(config, nil)

	// From idle, can only start
	events := mhm.AvailableEvents()
	if len(events) != 1 {
		t.Errorf("expected 1 available event from idle, got %d", len(events))
	}

	mhm.Start()

	// From determining, can connect, host, fail, or stop
	events = mhm.AvailableEvents()
	if len(events) != 4 {
		t.Errorf("expected 4 available events from determining, got %d", len(events))
	}

	mhm.MarkDeterminingClient(NetworkReachabilityRestricted)

	// From connecting, can connect, fallback, fail, or stop
	events = mhm.AvailableEvents()
	if len(events) != 4 {
		t.Errorf("expected 4 available events from connecting, got %d", len(events))
	}
}

func TestManagedHybridMode_CanTransition(t *testing.T) {
	config := DefaultHybridModeConfig("test-agent")
	config.ExternalNATSURLs = []string{"nats://localhost:4222"}

	mhm, _ := NewManagedHybridMode(config, nil)

	if !mhm.CanTransition(HybridModeEventStart) {
		t.Error("expected CanTransition(start) to be true from idle")
	}
	if mhm.CanTransition(HybridModeEventConnected) {
		t.Error("expected CanTransition(connected) to be false from idle")
	}
}

func TestManagedHybridMode_NilCallbacks(t *testing.T) {
	config := DefaultHybridModeConfig("test-agent")
	config.ExternalNATSURLs = []string{"nats://localhost:4222"}
	config.EmbeddedConfig = DefaultEmbeddedNATSConfig()

	mhm, _ := NewManagedHybridMode(config, nil)

	// These should not panic
	mhm.Start()
	mhm.MarkDeterminingClient(NetworkReachabilityRestricted)
	mhm.FallbackToHost(ConnectionRoleHost)
	mhm.MarkServerStarted(nil)
	mhm.MarkReconnected(nil)
	mhm.Stop()
}

func TestManagedHybridMode_EmptyCallbacks(t *testing.T) {
	config := DefaultHybridModeConfig("test-agent")
	config.ExternalNATSURLs = []string{"nats://localhost:4222"}

	callbacks := &HybridModeCallbacks{}
	mhm, _ := NewManagedHybridMode(config, callbacks)

	// These should not panic
	mhm.Start()
	mhm.MarkDeterminingClient(NetworkReachabilityRestricted)
	mhm.Fail(errors.New("test"))
}

func TestHybridModeStateToString(t *testing.T) {
	tests := []struct {
		state    HybridModeState
		expected string
	}{
		{HybridModeStateIdle, "Idle"},
		{HybridModeStateDetermining, "Determining"},
		{HybridModeStateConnecting, "Connecting"},
		{HybridModeStateHosting, "Hosting"},
		{HybridModeStateActive, "Active"},
		{HybridModeStateFailed, "Failed"},
		{HybridModeState(99), "unknown(99)"},
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			if got := HybridModeStateToString(tt.state); got != tt.expected {
				t.Errorf("HybridModeStateToString(%v) = %v, want %v", tt.state, got, tt.expected)
			}
		})
	}
}

func TestManagedHybridMode_StateDiagram(t *testing.T) {
	config := DefaultHybridModeConfig("test-agent")
	config.ExternalNATSURLs = []string{"nats://localhost:4222"}

	mhm, _ := NewManagedHybridMode(config, nil)
	diagram := mhm.StateDiagram()

	if diagram == "" {
		t.Error("expected non-empty state diagram")
	}
	if len(diagram) < 100 {
		t.Error("state diagram seems too short")
	}
}

func TestManagedHybridMode_GetStats(t *testing.T) {
	config := DefaultHybridModeConfig("test-agent")
	config.ExternalNATSURLs = []string{"nats://localhost:4222"}

	mhm, _ := NewManagedHybridMode(config, nil)

	mhm.Start()
	mhm.MarkDeterminingClient(NetworkReachabilityRestricted)
	mhm.MarkConnected(nil)

	stats := mhm.GetStats()
	if stats == nil {
		t.Fatal("stats should not be nil")
	}

	if stats.State != HybridModeStateActive {
		t.Errorf("expected active state in stats, got %v", stats.State)
	}
	if stats.Role != ConnectionRoleClient {
		t.Errorf("expected client role in stats, got %v", stats.Role)
	}
	if stats.ConnectionCount != 1 {
		t.Errorf("expected connection count 1 in stats, got %d", stats.ConnectionCount)
	}
}

func TestManagedHybridMode_DeterminedRoleCallback(t *testing.T) {
	var determinedRole ConnectionRole
	var determinedReach NetworkReachability

	callbacks := &HybridModeCallbacks{
		OnDeterminedRole: func(role ConnectionRole, reach NetworkReachability) {
			determinedRole = role
			determinedReach = reach
		},
	}

	config := DefaultHybridModeConfig("test-agent")
	config.ExternalNATSURLs = []string{"nats://localhost:4222"}

	mhm, _ := NewManagedHybridMode(config, callbacks)

	mhm.Start()
	mhm.MarkDeterminingClient(NetworkReachabilityDirect)

	if determinedRole != ConnectionRoleClient {
		t.Errorf("expected client role in callback, got %v", determinedRole)
	}
	if determinedReach != NetworkReachabilityDirect {
		t.Errorf("expected direct reachability in callback, got %v", determinedReach)
	}
}

func TestManagedHybridMode_IsTerminal(t *testing.T) {
	config := DefaultHybridModeConfig("test-agent")
	config.ExternalNATSURLs = []string{"nats://localhost:4222"}

	// Idle is terminal
	mhm1, _ := NewManagedHybridMode(config, nil)
	if !mhm1.IsTerminal() {
		t.Error("expected idle to be terminal")
	}

	// Failed is terminal
	mhm2, _ := NewManagedHybridMode(config, nil)
	mhm2.Start()
	mhm2.Fail(errors.New("test"))
	if !mhm2.IsTerminal() {
		t.Error("expected failed to be terminal")
	}

	// Running states are not terminal
	mhm3, _ := NewManagedHybridMode(config, nil)
	mhm3.Start()
	if mhm3.IsTerminal() {
		t.Error("expected determining to not be terminal")
	}
}
