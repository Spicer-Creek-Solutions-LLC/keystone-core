package controlplane

import (
	"context"
	"time"

	pb "github.com/shawnbutts/keystone-core/pkg/api/v1"
	"github.com/shawnbutts/keystone-core/pkg/statemachine"
)

// AgentHealthState represents the health state of an agent.
//
// State diagram:
//
// ```mermaid
// stateDiagram-v2
//     [*] --> Registered
//     Registered --> Healthy: HeartbeatReceived
//     Healthy --> Healthy: HeartbeatReceived
//     Healthy --> Degraded: HeartbeatMissed
//     Degraded --> Healthy: HeartbeatReceived
//     Degraded --> Offline: ThresholdExceeded
//     Offline --> Healthy: HeartbeatReceived
//     Offline --> Gone: Tombstone
//     Gone --> [*]
// ```
type AgentHealthState string

const (
	// AgentStateRegistered is the initial state after registration
	AgentStateRegistered AgentHealthState = "registered"
	// AgentStateHealthy means agent is responding normally
	AgentStateHealthy AgentHealthState = "healthy"
	// AgentStateDegraded means agent has missed some heartbeats
	AgentStateDegraded AgentHealthState = "degraded"
	// AgentStateOffline means agent has exceeded stale threshold
	AgentStateOffline AgentHealthState = "offline"
	// AgentStateGone means agent has been tombstoned for removal
	AgentStateGone AgentHealthState = "gone"
)

// AgentHealthEvent represents events that trigger agent state transitions.
type AgentHealthEvent string

const (
	// AgentEventHeartbeatReceived signals a heartbeat was received
	AgentEventHeartbeatReceived AgentHealthEvent = "heartbeat_received"
	// AgentEventHeartbeatMissed signals a heartbeat check found no recent heartbeat
	AgentEventHeartbeatMissed AgentHealthEvent = "heartbeat_missed"
	// AgentEventThresholdExceeded signals too many heartbeats have been missed
	AgentEventThresholdExceeded AgentHealthEvent = "threshold_exceeded"
	// AgentEventTombstone marks the agent for removal
	AgentEventTombstone AgentHealthEvent = "tombstone"
	// AgentEventReconnect signals agent has reconnected after being offline
	AgentEventReconnect AgentHealthEvent = "reconnect"
)

// AgentStateMachineCallbacks holds callbacks for agent state transitions.
type AgentStateMachineCallbacks struct {
	// OnHealthy is called when agent becomes healthy
	OnHealthy func(agentID string)
	// OnDegraded is called when agent becomes degraded
	OnDegraded func(agentID string)
	// OnOffline is called when agent goes offline
	OnOffline func(agentID string)
	// OnReconnect is called when agent reconnects from offline
	OnReconnect func(agentID string)
}

// ManagedAgent wraps AgentInfo with a health state machine.
type ManagedAgent struct {
	Info    *AgentInfo
	machine *statemachine.Machine[AgentHealthState, AgentHealthEvent]

	// Tracking
	missedCount    int
	staleThreshold int
	agentID        string

	// Callbacks
	callbacks *AgentStateMachineCallbacks
}

// NewManagedAgent creates a new managed agent with health state machine.
func NewManagedAgent(info *AgentInfo, staleThreshold int, callbacks *AgentStateMachineCallbacks) *ManagedAgent {
	ma := &ManagedAgent{
		Info:           info,
		staleThreshold: staleThreshold,
		agentID:        info.ID,
		callbacks:      callbacks,
	}

	ma.machine = statemachine.New[AgentHealthState, AgentHealthEvent](AgentStateRegistered).
		WithName("agent-health-" + info.ID).
		WithHistory(20).
		// From Registered
		AddTransition(AgentStateRegistered, AgentEventHeartbeatReceived, AgentStateHealthy).
		// From Healthy
		AddTransition(AgentStateHealthy, AgentEventHeartbeatReceived, AgentStateHealthy).
		AddTransition(AgentStateHealthy, AgentEventHeartbeatMissed, AgentStateDegraded).
		// From Degraded
		AddTransition(AgentStateDegraded, AgentEventHeartbeatReceived, AgentStateHealthy).
		AddTransition(AgentStateDegraded, AgentEventThresholdExceeded, AgentStateOffline).
		// From Offline
		AddTransition(AgentStateOffline, AgentEventHeartbeatReceived, AgentStateHealthy).
		AddTransition(AgentStateOffline, AgentEventTombstone, AgentStateGone).
		// Callbacks
		OnEnter(AgentStateHealthy, func(ctx context.Context, state, from AgentHealthState) {
			ma.missedCount = 0
			ma.Info.HeartbeatMissed = 0
			ma.Info.Status = pb.AgentStatus_AGENT_STATUS_ONLINE

			if from == AgentStateOffline && ma.callbacks != nil && ma.callbacks.OnReconnect != nil {
				ma.callbacks.OnReconnect(ma.agentID)
			} else if ma.callbacks != nil && ma.callbacks.OnHealthy != nil {
				ma.callbacks.OnHealthy(ma.agentID)
			}
		}).
		OnEnter(AgentStateDegraded, func(ctx context.Context, state, from AgentHealthState) {
			ma.Info.Status = pb.AgentStatus_AGENT_STATUS_DEGRADED
			if ma.callbacks != nil && ma.callbacks.OnDegraded != nil {
				ma.callbacks.OnDegraded(ma.agentID)
			}
		}).
		OnEnter(AgentStateOffline, func(ctx context.Context, state, from AgentHealthState) {
			ma.Info.Status = pb.AgentStatus_AGENT_STATUS_OFFLINE
			if ma.callbacks != nil && ma.callbacks.OnOffline != nil {
				ma.callbacks.OnOffline(ma.agentID)
			}
		}).
		MustBuild()

	return ma
}

// HealthState returns the current health state.
func (ma *ManagedAgent) HealthState() AgentHealthState {
	return ma.machine.State()
}

// ProtoStatus returns the protobuf status matching the current health state.
func (ma *ManagedAgent) ProtoStatus() pb.AgentStatus {
	switch ma.machine.State() {
	case AgentStateRegistered:
		return pb.AgentStatus_AGENT_STATUS_UNSPECIFIED
	case AgentStateHealthy:
		return pb.AgentStatus_AGENT_STATUS_ONLINE
	case AgentStateDegraded:
		return pb.AgentStatus_AGENT_STATUS_DEGRADED
	case AgentStateOffline:
		return pb.AgentStatus_AGENT_STATUS_OFFLINE
	case AgentStateGone:
		return pb.AgentStatus_AGENT_STATUS_OFFLINE
	default:
		return pb.AgentStatus_AGENT_STATUS_UNSPECIFIED
	}
}

// RecordHeartbeat records a successful heartbeat.
func (ma *ManagedAgent) RecordHeartbeat(metrics *pb.SystemMetrics) error {
	ma.Info.LastHeartbeat = time.Now()
	ma.Info.LastMetrics = metrics
	return ma.machine.Fire(AgentEventHeartbeatReceived)
}

// CheckHealth checks if a heartbeat has been missed based on timeout.
// Returns true if the agent state changed.
func (ma *ManagedAgent) CheckHealth(timeout time.Duration) bool {
	timeSinceHeartbeat := time.Since(ma.Info.LastHeartbeat)

	if timeSinceHeartbeat <= timeout {
		// Heartbeat is recent, nothing to do
		return false
	}

	// Heartbeat is overdue
	currentState := ma.machine.State()

	switch currentState {
	case AgentStateHealthy:
		// First missed heartbeat, go to degraded
		ma.missedCount++
		ma.Info.HeartbeatMissed = ma.missedCount
		ma.machine.Fire(AgentEventHeartbeatMissed)
		return true

	case AgentStateDegraded:
		// Already degraded, check if we've exceeded threshold
		ma.missedCount++
		ma.Info.HeartbeatMissed = ma.missedCount
		if ma.missedCount >= ma.staleThreshold {
			ma.machine.Fire(AgentEventThresholdExceeded)
			return true
		}
		return false

	case AgentStateRegistered:
		// Never got a heartbeat, go straight to degraded
		ma.missedCount = 1
		ma.Info.HeartbeatMissed = 1
		// First transition to healthy (which sets up proper state)
		// then to degraded
		if ma.machine.CanFire(AgentEventHeartbeatMissed) {
			ma.machine.Fire(AgentEventHeartbeatMissed)
			return true
		}
		return false

	default:
		return false
	}
}

// Tombstone marks the agent for removal.
func (ma *ManagedAgent) Tombstone() error {
	if ma.machine.CanFire(AgentEventTombstone) {
		return ma.machine.Fire(AgentEventTombstone)
	}
	return nil
}

// IsHealthy returns true if the agent is in a healthy state.
func (ma *ManagedAgent) IsHealthy() bool {
	return ma.machine.IsInState(AgentStateHealthy)
}

// IsOnline returns true if the agent is healthy or degraded.
func (ma *ManagedAgent) IsOnline() bool {
	return ma.machine.IsInAnyState(AgentStateHealthy, AgentStateDegraded)
}

// IsOffline returns true if the agent is offline.
func (ma *ManagedAgent) IsOffline() bool {
	return ma.machine.IsInState(AgentStateOffline)
}

// IsGone returns true if the agent has been tombstoned.
func (ma *ManagedAgent) IsGone() bool {
	return ma.machine.IsInState(AgentStateGone)
}

// MissedCount returns the number of missed heartbeats.
func (ma *ManagedAgent) MissedCount() int {
	return ma.missedCount
}

// History returns the state transition history.
func (ma *ManagedAgent) History() *statemachine.History[AgentHealthState, AgentHealthEvent] {
	return ma.machine.History()
}

// AgentHealthStateToString returns a human-readable name for the state.
func AgentHealthStateToString(state AgentHealthState) string {
	switch state {
	case AgentStateRegistered:
		return "Registered"
	case AgentStateHealthy:
		return "Healthy"
	case AgentStateDegraded:
		return "Degraded"
	case AgentStateOffline:
		return "Offline"
	case AgentStateGone:
		return "Gone"
	default:
		return string(state)
	}
}
