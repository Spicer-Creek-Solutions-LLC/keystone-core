package controlplane

import (
	"errors"
	"testing"
	"time"

	pb "github.com/shawnbutts/keystone-core/pkg/api/v1"
	"github.com/shawnbutts/keystone-core/pkg/statemachine"
)

func TestManagedAgent_InitialState(t *testing.T) {
	info := &AgentInfo{
		ID:     "test-agent",
		Status: pb.AgentStatus_AGENT_STATUS_UNSPECIFIED,
	}

	ma := NewManagedAgent(info, 3, nil)

	if ma.HealthState() != AgentStateRegistered {
		t.Errorf("expected registered state, got %v", ma.HealthState())
	}
	if ma.ProtoStatus() != pb.AgentStatus_AGENT_STATUS_UNSPECIFIED {
		t.Errorf("expected unspecified status, got %v", ma.ProtoStatus())
	}
}

func TestManagedAgent_HeartbeatTransitions(t *testing.T) {
	info := &AgentInfo{
		ID:     "test-agent",
		Status: pb.AgentStatus_AGENT_STATUS_UNSPECIFIED,
	}

	ma := NewManagedAgent(info, 3, nil)

	// First heartbeat: Registered -> Healthy
	if err := ma.RecordHeartbeat(&pb.SystemMetrics{}); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if ma.HealthState() != AgentStateHealthy {
		t.Errorf("expected healthy state, got %v", ma.HealthState())
	}
	if ma.ProtoStatus() != pb.AgentStatus_AGENT_STATUS_ONLINE {
		t.Errorf("expected online status, got %v", ma.ProtoStatus())
	}

	// Second heartbeat: Healthy -> Healthy
	if err := ma.RecordHeartbeat(&pb.SystemMetrics{}); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if ma.HealthState() != AgentStateHealthy {
		t.Errorf("expected healthy state, got %v", ma.HealthState())
	}
}

func TestManagedAgent_DegradedTransition(t *testing.T) {
	info := &AgentInfo{
		ID:            "test-agent",
		Status:        pb.AgentStatus_AGENT_STATUS_UNSPECIFIED,
		LastHeartbeat: time.Now().Add(-10 * time.Second),
	}

	ma := NewManagedAgent(info, 3, nil)

	// Get to healthy state first
	ma.RecordHeartbeat(&pb.SystemMetrics{})

	// Set last heartbeat to the past
	ma.Info.LastHeartbeat = time.Now().Add(-10 * time.Second)

	// Check health with short timeout
	changed := ma.CheckHealth(1 * time.Second)
	if !changed {
		t.Error("expected state change")
	}
	if ma.HealthState() != AgentStateDegraded {
		t.Errorf("expected degraded state, got %v", ma.HealthState())
	}
	if ma.ProtoStatus() != pb.AgentStatus_AGENT_STATUS_DEGRADED {
		t.Errorf("expected degraded status, got %v", ma.ProtoStatus())
	}
	if ma.MissedCount() != 1 {
		t.Errorf("expected 1 missed, got %d", ma.MissedCount())
	}
}

func TestManagedAgent_OfflineTransition(t *testing.T) {
	info := &AgentInfo{
		ID:            "test-agent",
		Status:        pb.AgentStatus_AGENT_STATUS_UNSPECIFIED,
		LastHeartbeat: time.Now().Add(-10 * time.Second),
	}

	ma := NewManagedAgent(info, 3, nil) // 3 missed heartbeats to go offline

	// Get to healthy state first
	ma.RecordHeartbeat(&pb.SystemMetrics{})
	ma.Info.LastHeartbeat = time.Now().Add(-10 * time.Second)

	// Miss 1st heartbeat -> degraded
	ma.CheckHealth(1 * time.Second)
	if ma.HealthState() != AgentStateDegraded {
		t.Errorf("expected degraded state, got %v", ma.HealthState())
	}

	// Miss 2nd heartbeat -> still degraded
	ma.CheckHealth(1 * time.Second)
	if ma.HealthState() != AgentStateDegraded {
		t.Errorf("expected degraded state, got %v", ma.HealthState())
	}

	// Miss 3rd heartbeat -> offline
	changed := ma.CheckHealth(1 * time.Second)
	if !changed {
		t.Error("expected state change")
	}
	if ma.HealthState() != AgentStateOffline {
		t.Errorf("expected offline state, got %v", ma.HealthState())
	}
	if ma.ProtoStatus() != pb.AgentStatus_AGENT_STATUS_OFFLINE {
		t.Errorf("expected offline status, got %v", ma.ProtoStatus())
	}
}

func TestManagedAgent_RecoveryFromDegraded(t *testing.T) {
	info := &AgentInfo{
		ID:            "test-agent",
		LastHeartbeat: time.Now().Add(-10 * time.Second),
	}

	ma := NewManagedAgent(info, 3, nil)

	// Get to healthy then degraded
	ma.RecordHeartbeat(&pb.SystemMetrics{})
	ma.Info.LastHeartbeat = time.Now().Add(-10 * time.Second)
	ma.CheckHealth(1 * time.Second)

	if ma.HealthState() != AgentStateDegraded {
		t.Errorf("expected degraded state, got %v", ma.HealthState())
	}
	if ma.MissedCount() != 1 {
		t.Errorf("expected 1 missed, got %d", ma.MissedCount())
	}

	// Recover with heartbeat
	if err := ma.RecordHeartbeat(&pb.SystemMetrics{}); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if ma.HealthState() != AgentStateHealthy {
		t.Errorf("expected healthy state, got %v", ma.HealthState())
	}
	if ma.MissedCount() != 0 {
		t.Errorf("expected 0 missed after recovery, got %d", ma.MissedCount())
	}
}

func TestManagedAgent_RecoveryFromOffline(t *testing.T) {
	info := &AgentInfo{
		ID:            "test-agent",
		LastHeartbeat: time.Now().Add(-10 * time.Second),
	}

	ma := NewManagedAgent(info, 2, nil) // 2 missed to go offline

	// Get to healthy then offline
	ma.RecordHeartbeat(&pb.SystemMetrics{})
	ma.Info.LastHeartbeat = time.Now().Add(-10 * time.Second)
	ma.CheckHealth(1 * time.Second) // degraded
	ma.CheckHealth(1 * time.Second) // offline

	if ma.HealthState() != AgentStateOffline {
		t.Errorf("expected offline state, got %v", ma.HealthState())
	}

	// Recover with heartbeat
	if err := ma.RecordHeartbeat(&pb.SystemMetrics{}); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if ma.HealthState() != AgentStateHealthy {
		t.Errorf("expected healthy state after recovery, got %v", ma.HealthState())
	}
}

func TestManagedAgent_Tombstone(t *testing.T) {
	info := &AgentInfo{
		ID:            "test-agent",
		LastHeartbeat: time.Now().Add(-10 * time.Second),
	}

	ma := NewManagedAgent(info, 2, nil)

	// Get to offline state
	ma.RecordHeartbeat(&pb.SystemMetrics{})
	ma.Info.LastHeartbeat = time.Now().Add(-10 * time.Second)
	ma.CheckHealth(1 * time.Second)
	ma.CheckHealth(1 * time.Second)

	if ma.HealthState() != AgentStateOffline {
		t.Errorf("expected offline state, got %v", ma.HealthState())
	}

	// Tombstone
	if err := ma.Tombstone(); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if ma.HealthState() != AgentStateGone {
		t.Errorf("expected gone state, got %v", ma.HealthState())
	}
	if !ma.IsGone() {
		t.Error("expected IsGone() to be true")
	}
}

func TestManagedAgent_TombstoneFromNonOffline(t *testing.T) {
	info := &AgentInfo{
		ID: "test-agent",
	}

	ma := NewManagedAgent(info, 3, nil)

	// Try to tombstone from registered state
	if err := ma.Tombstone(); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	// Should not change state since transition not allowed
	if ma.HealthState() != AgentStateRegistered {
		t.Errorf("expected registered state, got %v", ma.HealthState())
	}
}

func TestManagedAgent_IsHelpers(t *testing.T) {
	info := &AgentInfo{
		ID:            "test-agent",
		LastHeartbeat: time.Now().Add(-10 * time.Second),
	}

	ma := NewManagedAgent(info, 2, nil)

	// Test IsHealthy/IsOnline/IsOffline in registered state
	if ma.IsHealthy() {
		t.Error("should not be healthy in registered state")
	}
	if ma.IsOnline() {
		t.Error("should not be online in registered state")
	}
	if ma.IsOffline() {
		t.Error("should not be offline in registered state")
	}

	// Move to healthy
	ma.RecordHeartbeat(&pb.SystemMetrics{})
	if !ma.IsHealthy() {
		t.Error("should be healthy")
	}
	if !ma.IsOnline() {
		t.Error("should be online")
	}

	// Move to degraded
	ma.Info.LastHeartbeat = time.Now().Add(-10 * time.Second)
	ma.CheckHealth(1 * time.Second)
	if ma.IsHealthy() {
		t.Error("should not be healthy in degraded state")
	}
	if !ma.IsOnline() {
		t.Error("should still be online in degraded state")
	}

	// Move to offline
	ma.CheckHealth(1 * time.Second)
	if ma.IsHealthy() {
		t.Error("should not be healthy in offline state")
	}
	if ma.IsOnline() {
		t.Error("should not be online in offline state")
	}
	if !ma.IsOffline() {
		t.Error("should be offline")
	}
}

func TestManagedAgent_Callbacks(t *testing.T) {
	var healthyCalls, degradedCalls, offlineCalls, reconnectCalls int
	var lastHealthyID, lastDegradedID, lastOfflineID, lastReconnectID string

	callbacks := &AgentStateMachineCallbacks{
		OnHealthy: func(agentID string) {
			healthyCalls++
			lastHealthyID = agentID
		},
		OnDegraded: func(agentID string) {
			degradedCalls++
			lastDegradedID = agentID
		},
		OnOffline: func(agentID string) {
			offlineCalls++
			lastOfflineID = agentID
		},
		OnReconnect: func(agentID string) {
			reconnectCalls++
			lastReconnectID = agentID
		},
	}

	info := &AgentInfo{
		ID:            "test-agent-1",
		LastHeartbeat: time.Now().Add(-10 * time.Second),
	}

	ma := NewManagedAgent(info, 2, callbacks)

	// First heartbeat -> healthy
	ma.RecordHeartbeat(&pb.SystemMetrics{})
	if healthyCalls != 1 || lastHealthyID != "test-agent-1" {
		t.Errorf("expected OnHealthy called once, got %d", healthyCalls)
	}

	// Miss heartbeat -> degraded
	ma.Info.LastHeartbeat = time.Now().Add(-10 * time.Second)
	ma.CheckHealth(1 * time.Second)
	if degradedCalls != 1 || lastDegradedID != "test-agent-1" {
		t.Errorf("expected OnDegraded called once, got %d", degradedCalls)
	}

	// Miss again -> offline
	ma.CheckHealth(1 * time.Second)
	if offlineCalls != 1 || lastOfflineID != "test-agent-1" {
		t.Errorf("expected OnOffline called once, got %d", offlineCalls)
	}

	// Recover -> reconnect (not just healthy)
	ma.RecordHeartbeat(&pb.SystemMetrics{})
	if reconnectCalls != 1 || lastReconnectID != "test-agent-1" {
		t.Errorf("expected OnReconnect called once, got %d", reconnectCalls)
	}
	// OnHealthy should NOT be called again when reconnecting from offline
	if healthyCalls != 1 {
		t.Errorf("OnHealthy should not be called for reconnect, got %d", healthyCalls)
	}
}

func TestManagedAgent_History(t *testing.T) {
	info := &AgentInfo{
		ID:            "test-agent",
		LastHeartbeat: time.Now().Add(-10 * time.Second),
	}

	ma := NewManagedAgent(info, 2, nil)

	// Generate some transitions
	ma.RecordHeartbeat(&pb.SystemMetrics{})
	ma.Info.LastHeartbeat = time.Now().Add(-10 * time.Second)
	ma.CheckHealth(1 * time.Second)
	ma.CheckHealth(1 * time.Second)

	history := ma.History()
	if history == nil {
		t.Fatal("history should not be nil")
	}

	records := history.All()
	if len(records) < 3 {
		t.Errorf("expected at least 3 history records, got %d", len(records))
	}
}

func TestManagedAgent_CheckHealth_NoChange(t *testing.T) {
	info := &AgentInfo{
		ID:            "test-agent",
		LastHeartbeat: time.Now(),
	}

	ma := NewManagedAgent(info, 3, nil)
	ma.RecordHeartbeat(&pb.SystemMetrics{})

	// Check health with recent heartbeat
	changed := ma.CheckHealth(10 * time.Second)
	if changed {
		t.Error("expected no state change")
	}
	if ma.HealthState() != AgentStateHealthy {
		t.Errorf("expected healthy state, got %v", ma.HealthState())
	}
}

func TestManagedAgent_CheckHealth_FromRegistered(t *testing.T) {
	info := &AgentInfo{
		ID:            "test-agent",
		LastHeartbeat: time.Now().Add(-10 * time.Second),
	}

	ma := NewManagedAgent(info, 3, nil)

	// Check health from registered state (never got first heartbeat)
	// This should not cause a transition since HeartbeatMissed isn't valid from Registered
	changed := ma.CheckHealth(1 * time.Second)
	if changed {
		t.Error("expected no state change from registered")
	}
	if ma.HealthState() != AgentStateRegistered {
		t.Errorf("expected registered state, got %v", ma.HealthState())
	}
}

func TestManagedAgent_CheckHealth_OfflineNoChange(t *testing.T) {
	info := &AgentInfo{
		ID:            "test-agent",
		LastHeartbeat: time.Now().Add(-10 * time.Second),
	}

	ma := NewManagedAgent(info, 2, nil)

	// Get to offline
	ma.RecordHeartbeat(&pb.SystemMetrics{})
	ma.Info.LastHeartbeat = time.Now().Add(-10 * time.Second)
	ma.CheckHealth(1 * time.Second)
	ma.CheckHealth(1 * time.Second)

	if ma.HealthState() != AgentStateOffline {
		t.Errorf("expected offline state, got %v", ma.HealthState())
	}

	// Check health again - should not change
	changed := ma.CheckHealth(1 * time.Second)
	if changed {
		t.Error("expected no state change from offline")
	}
}

func TestAgentHealthStateToString(t *testing.T) {
	tests := []struct {
		state AgentHealthState
		want  string
	}{
		{AgentStateRegistered, "Registered"},
		{AgentStateHealthy, "Healthy"},
		{AgentStateDegraded, "Degraded"},
		{AgentStateOffline, "Offline"},
		{AgentStateGone, "Gone"},
		{AgentHealthState("unknown"), "unknown"},
	}

	for _, tt := range tests {
		t.Run(string(tt.state), func(t *testing.T) {
			if got := AgentHealthStateToString(tt.state); got != tt.want {
				t.Errorf("AgentHealthStateToString(%v) = %v, want %v", tt.state, got, tt.want)
			}
		})
	}
}

func TestManagedAgent_ProtoStatus_AllStates(t *testing.T) {
	tests := []struct {
		name   string
		setup  func(*ManagedAgent)
		status pb.AgentStatus
	}{
		{
			"registered",
			func(ma *ManagedAgent) {},
			pb.AgentStatus_AGENT_STATUS_UNSPECIFIED,
		},
		{
			"healthy",
			func(ma *ManagedAgent) {
				ma.RecordHeartbeat(&pb.SystemMetrics{})
			},
			pb.AgentStatus_AGENT_STATUS_ONLINE,
		},
		{
			"degraded",
			func(ma *ManagedAgent) {
				ma.RecordHeartbeat(&pb.SystemMetrics{})
				ma.Info.LastHeartbeat = time.Now().Add(-10 * time.Second)
				ma.CheckHealth(1 * time.Second)
			},
			pb.AgentStatus_AGENT_STATUS_DEGRADED,
		},
		{
			"offline",
			func(ma *ManagedAgent) {
				ma.RecordHeartbeat(&pb.SystemMetrics{})
				ma.Info.LastHeartbeat = time.Now().Add(-10 * time.Second)
				ma.CheckHealth(1 * time.Second)
				ma.CheckHealth(1 * time.Second)
			},
			pb.AgentStatus_AGENT_STATUS_OFFLINE,
		},
		{
			"gone",
			func(ma *ManagedAgent) {
				ma.RecordHeartbeat(&pb.SystemMetrics{})
				ma.Info.LastHeartbeat = time.Now().Add(-10 * time.Second)
				ma.CheckHealth(1 * time.Second)
				ma.CheckHealth(1 * time.Second)
				ma.Tombstone()
			},
			pb.AgentStatus_AGENT_STATUS_OFFLINE, // Gone maps to OFFLINE
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			info := &AgentInfo{
				ID:            "test-agent",
				LastHeartbeat: time.Now(),
			}
			ma := NewManagedAgent(info, 2, nil)
			tt.setup(ma)
			if ma.ProtoStatus() != tt.status {
				t.Errorf("expected %v, got %v", tt.status, ma.ProtoStatus())
			}
		})
	}
}

func TestManagedAgent_InvalidTransition(t *testing.T) {
	info := &AgentInfo{
		ID: "test-agent",
	}

	ma := NewManagedAgent(info, 3, nil)

	// Try to fire ThresholdExceeded from Registered (invalid)
	err := ma.machine.Fire(AgentEventThresholdExceeded)
	if err == nil {
		t.Error("expected error for invalid transition")
	}
	if !errors.Is(err, statemachine.ErrInvalidTransition) {
		t.Errorf("expected ErrInvalidTransition, got %v", err)
	}
}

func TestManagedAgent_NilCallbacks(t *testing.T) {
	info := &AgentInfo{
		ID:            "test-agent",
		LastHeartbeat: time.Now().Add(-10 * time.Second),
	}

	// Empty callbacks struct (not nil, but with nil functions)
	callbacks := &AgentStateMachineCallbacks{}

	ma := NewManagedAgent(info, 2, callbacks)

	// These should not panic
	ma.RecordHeartbeat(&pb.SystemMetrics{})
	ma.Info.LastHeartbeat = time.Now().Add(-10 * time.Second)
	ma.CheckHealth(1 * time.Second)
	ma.CheckHealth(1 * time.Second)
	ma.RecordHeartbeat(&pb.SystemMetrics{})
}

func TestManagedAgent_HeartbeatMissedCounter(t *testing.T) {
	info := &AgentInfo{
		ID:            "test-agent",
		LastHeartbeat: time.Now().Add(-10 * time.Second),
	}

	ma := NewManagedAgent(info, 5, nil) // High threshold

	ma.RecordHeartbeat(&pb.SystemMetrics{})
	ma.Info.LastHeartbeat = time.Now().Add(-10 * time.Second)

	// Miss multiple heartbeats
	for i := 1; i <= 4; i++ {
		ma.CheckHealth(1 * time.Second)
		if ma.MissedCount() != i {
			t.Errorf("expected %d missed, got %d", i, ma.MissedCount())
		}
		if ma.Info.HeartbeatMissed != i {
			t.Errorf("expected Info.HeartbeatMissed=%d, got %d", i, ma.Info.HeartbeatMissed)
		}
	}

	// Recover and verify counter reset
	ma.RecordHeartbeat(&pb.SystemMetrics{})
	if ma.MissedCount() != 0 {
		t.Errorf("expected 0 missed after recovery, got %d", ma.MissedCount())
	}
	if ma.Info.HeartbeatMissed != 0 {
		t.Errorf("expected Info.HeartbeatMissed=0, got %d", ma.Info.HeartbeatMissed)
	}
}
