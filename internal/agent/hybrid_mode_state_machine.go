package agent

import (
	"context"
	"fmt"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/shawnbutts/keystone-core/pkg/statemachine"
)

// HybridModeEvent represents events that can occur in hybrid mode
type HybridModeEvent string

const (
	HybridModeEventStart             HybridModeEvent = "start"
	HybridModeEventDetermined        HybridModeEvent = "determined"
	HybridModeEventConnecting        HybridModeEvent = "connecting"
	HybridModeEventHosting           HybridModeEvent = "hosting"
	HybridModeEventConnected         HybridModeEvent = "connected"
	HybridModeEventServerStarted     HybridModeEvent = "server_started"
	HybridModeEventFallbackToHost    HybridModeEvent = "fallback_to_host"
	HybridModeEventFallbackToClient  HybridModeEvent = "fallback_to_client"
	HybridModeEventConnectionLost    HybridModeEvent = "connection_lost"
	HybridModeEventReconnected       HybridModeEvent = "reconnected"
	HybridModeEventFail              HybridModeEvent = "fail"
	HybridModeEventStop              HybridModeEvent = "stop"
	HybridModeEventReset             HybridModeEvent = "reset"
	HybridModeEventReachabilityCheck HybridModeEvent = "reachability_check"
)

// HybridModeCallbacks defines callbacks for hybrid mode state transitions
type HybridModeCallbacks struct {
	OnStateChange     func(state HybridModeState)
	OnRoleChange      func(role ConnectionRole)
	OnConnectionReady func(role ConnectionRole, conn *nats.Conn)
	OnConnectionLost  func(role ConnectionRole, err error)
	OnDeterminedRole  func(role ConnectionRole, reachability NetworkReachability)
	OnFallback        func(fromRole, toRole ConnectionRole)
	OnFailed          func(state HybridModeState, err error)
}

// ManagedHybridMode wraps a hybrid mode manager with explicit state machine
type ManagedHybridMode struct {
	Manager   *HybridModeManager
	machine   *statemachine.Machine[HybridModeState, HybridModeEvent]
	callbacks *HybridModeCallbacks

	agentID         string
	role            ConnectionRole
	previousRole    ConnectionRole
	reachability    NetworkReachability
	lastError       error
	connectionCount int
	reconnectCount  int
	startTime       time.Time
	activeTime      time.Time
	lastCheckTime   time.Time
	stateChangeTime time.Time
}

// NewManagedHybridMode creates a new managed hybrid mode
func NewManagedHybridMode(config *HybridModeConfig, callbacks *HybridModeCallbacks) (*ManagedHybridMode, error) {
	manager, err := NewHybridModeManager(config)
	if err != nil {
		return nil, err
	}

	mhm := &ManagedHybridMode{
		Manager:         manager,
		callbacks:       callbacks,
		agentID:         config.AgentID,
		role:            ConnectionRoleUndetermined,
		reachability:    NetworkReachabilityUnknown,
		stateChangeTime: time.Now(),
	}

	mhm.machine = mhm.buildStateMachine()
	return mhm, nil
}

// buildStateMachine creates the hybrid mode state machine
func (mhm *ManagedHybridMode) buildStateMachine() *statemachine.Machine[HybridModeState, HybridModeEvent] {
	builder := statemachine.New[HybridModeState, HybridModeEvent](HybridModeStateIdle).
		WithHistory(20)

	// Idle -> Determining (start)
	builder.AddTransition(HybridModeStateIdle, HybridModeEventStart, HybridModeStateDetermining).
		OnEnter(HybridModeStateDetermining, func(_ context.Context, _ HybridModeState, _ HybridModeState) {
			mhm.onStateEnter(HybridModeStateDetermining)
		})

	// Determining -> Connecting (determined client role)
	builder.AddTransition(HybridModeStateDetermining, HybridModeEventConnecting, HybridModeStateConnecting).
		OnEnter(HybridModeStateConnecting, func(_ context.Context, _ HybridModeState, _ HybridModeState) {
			mhm.onStateEnter(HybridModeStateConnecting)
		})

	// Determining -> Hosting (determined host/leaf role)
	builder.AddTransition(HybridModeStateDetermining, HybridModeEventHosting, HybridModeStateHosting).
		OnEnter(HybridModeStateHosting, func(_ context.Context, _ HybridModeState, _ HybridModeState) {
			mhm.onStateEnter(HybridModeStateHosting)
		})

	// Determining -> Failed (cannot determine role)
	builder.AddTransition(HybridModeStateDetermining, HybridModeEventFail, HybridModeStateFailed)

	// Connecting -> Active (connected successfully)
	builder.AddTransition(HybridModeStateConnecting, HybridModeEventConnected, HybridModeStateActive).
		OnEnter(HybridModeStateActive, func(_ context.Context, _ HybridModeState, _ HybridModeState) {
			mhm.onActivated()
		})

	// Connecting -> Hosting (fallback)
	builder.AddTransition(HybridModeStateConnecting, HybridModeEventFallbackToHost, HybridModeStateHosting).
		OnEnter(HybridModeStateHosting, func(_ context.Context, _ HybridModeState, prev HybridModeState) {
			if prev == HybridModeStateConnecting {
				mhm.onFallback(ConnectionRoleClient, mhm.role)
			}
			mhm.onStateEnter(HybridModeStateHosting)
		})

	// Connecting -> Failed
	builder.AddTransition(HybridModeStateConnecting, HybridModeEventFail, HybridModeStateFailed)

	// Hosting -> Active (server started successfully)
	builder.AddTransition(HybridModeStateHosting, HybridModeEventServerStarted, HybridModeStateActive)

	// Hosting -> Connecting (fallback)
	builder.AddTransition(HybridModeStateHosting, HybridModeEventFallbackToClient, HybridModeStateConnecting).
		OnEnter(HybridModeStateConnecting, func(_ context.Context, _ HybridModeState, prev HybridModeState) {
			if prev == HybridModeStateHosting {
				mhm.onFallback(mhm.role, ConnectionRoleClient)
			}
			mhm.onStateEnter(HybridModeStateConnecting)
		})

	// Hosting -> Failed
	builder.AddTransition(HybridModeStateHosting, HybridModeEventFail, HybridModeStateFailed)

	// Active -> Active (reconnected after temporary loss)
	builder.AddTransition(HybridModeStateActive, HybridModeEventReconnected, HybridModeStateActive)

	// Active -> Active (reachability check)
	builder.AddTransition(HybridModeStateActive, HybridModeEventReachabilityCheck, HybridModeStateActive)

	// Active -> Determining (connection lost, need to redetermine)
	builder.AddTransition(HybridModeStateActive, HybridModeEventConnectionLost, HybridModeStateDetermining)

	// Active -> Failed (unrecoverable error)
	builder.AddTransition(HybridModeStateActive, HybridModeEventFail, HybridModeStateFailed)

	// Active -> Idle (stopped)
	builder.AddTransition(HybridModeStateActive, HybridModeEventStop, HybridModeStateIdle).
		OnEnter(HybridModeStateIdle, func(_ context.Context, _ HybridModeState, _ HybridModeState) {
			mhm.onStopped()
		})

	// Failed -> Idle (reset or stop)
	builder.AddTransition(HybridModeStateFailed, HybridModeEventReset, HybridModeStateIdle)
	builder.AddTransition(HybridModeStateFailed, HybridModeEventStop, HybridModeStateIdle)

	// Failed callback
	builder.OnEnter(HybridModeStateFailed, func(_ context.Context, _ HybridModeState, _ HybridModeState) {
		mhm.onFailed()
	})

	// Determining -> Idle (stop before determined)
	builder.AddTransition(HybridModeStateDetermining, HybridModeEventStop, HybridModeStateIdle)

	// Connecting -> Idle (stop before connected)
	builder.AddTransition(HybridModeStateConnecting, HybridModeEventStop, HybridModeStateIdle)

	// Hosting -> Idle (stop before active)
	builder.AddTransition(HybridModeStateHosting, HybridModeEventStop, HybridModeStateIdle)

	return builder.MustBuild()
}

// onStateEnter is called when entering a new state
func (mhm *ManagedHybridMode) onStateEnter(state HybridModeState) {
	mhm.stateChangeTime = time.Now()

	if mhm.callbacks != nil && mhm.callbacks.OnStateChange != nil {
		mhm.callbacks.OnStateChange(state)
	}
}

// onActivated is called when entering Active state
func (mhm *ManagedHybridMode) onActivated() {
	mhm.activeTime = time.Now()
	mhm.connectionCount++
	mhm.stateChangeTime = time.Now()

	if mhm.callbacks != nil && mhm.callbacks.OnStateChange != nil {
		mhm.callbacks.OnStateChange(HybridModeStateActive)
	}
}

// onFallback is called when falling back to another role
func (mhm *ManagedHybridMode) onFallback(fromRole, toRole ConnectionRole) {
	mhm.previousRole = fromRole
	mhm.role = toRole

	if mhm.callbacks != nil && mhm.callbacks.OnFallback != nil {
		mhm.callbacks.OnFallback(fromRole, toRole)
	}
}

// onFailed is called when entering Failed state
func (mhm *ManagedHybridMode) onFailed() {
	mhm.stateChangeTime = time.Now()

	if mhm.callbacks != nil && mhm.callbacks.OnFailed != nil {
		mhm.callbacks.OnFailed(HybridModeStateFailed, mhm.lastError)
	}
}

// onStopped is called when returning to Idle state
func (mhm *ManagedHybridMode) onStopped() {
	mhm.role = ConnectionRoleUndetermined
	mhm.stateChangeTime = time.Now()
}

// Start begins the hybrid mode operation
func (mhm *ManagedHybridMode) Start() error {
	mhm.startTime = time.Now()
	return mhm.machine.Fire(HybridModeEventStart)
}

// MarkDeterminingClient marks that client role was determined
func (mhm *ManagedHybridMode) MarkDeterminingClient(reachability NetworkReachability) error {
	mhm.role = ConnectionRoleClient
	mhm.reachability = reachability

	if mhm.callbacks != nil && mhm.callbacks.OnDeterminedRole != nil {
		mhm.callbacks.OnDeterminedRole(ConnectionRoleClient, reachability)
	}

	return mhm.machine.Fire(HybridModeEventConnecting)
}

// MarkDeterminingHost marks that host role was determined
func (mhm *ManagedHybridMode) MarkDeterminingHost(reachability NetworkReachability) error {
	mhm.role = ConnectionRoleHost
	mhm.reachability = reachability

	if mhm.callbacks != nil && mhm.callbacks.OnDeterminedRole != nil {
		mhm.callbacks.OnDeterminedRole(ConnectionRoleHost, reachability)
	}

	return mhm.machine.Fire(HybridModeEventHosting)
}

// MarkDeterminingLeaf marks that leaf role was determined
func (mhm *ManagedHybridMode) MarkDeterminingLeaf(reachability NetworkReachability) error {
	mhm.role = ConnectionRoleLeaf
	mhm.reachability = reachability

	if mhm.callbacks != nil && mhm.callbacks.OnDeterminedRole != nil {
		mhm.callbacks.OnDeterminedRole(ConnectionRoleLeaf, reachability)
	}

	return mhm.machine.Fire(HybridModeEventHosting)
}

// MarkConnected marks that connection was established
func (mhm *ManagedHybridMode) MarkConnected(conn *nats.Conn) error {
	if mhm.callbacks != nil && mhm.callbacks.OnConnectionReady != nil {
		mhm.callbacks.OnConnectionReady(mhm.role, conn)
	}

	return mhm.machine.Fire(HybridModeEventConnected)
}

// MarkServerStarted marks that embedded server started
func (mhm *ManagedHybridMode) MarkServerStarted(conn *nats.Conn) error {
	if mhm.callbacks != nil && mhm.callbacks.OnConnectionReady != nil {
		mhm.callbacks.OnConnectionReady(mhm.role, conn)
	}

	return mhm.machine.Fire(HybridModeEventServerStarted)
}

// FallbackToHost initiates fallback from client to host
func (mhm *ManagedHybridMode) FallbackToHost(role ConnectionRole) error {
	mhm.previousRole = mhm.role
	mhm.role = role
	return mhm.machine.Fire(HybridModeEventFallbackToHost)
}

// FallbackToClient initiates fallback from host to client
func (mhm *ManagedHybridMode) FallbackToClient() error {
	mhm.previousRole = mhm.role
	mhm.role = ConnectionRoleClient
	return mhm.machine.Fire(HybridModeEventFallbackToClient)
}

// MarkConnectionLost marks that connection was lost
func (mhm *ManagedHybridMode) MarkConnectionLost(err error) error {
	mhm.lastError = err

	if mhm.callbacks != nil && mhm.callbacks.OnConnectionLost != nil {
		mhm.callbacks.OnConnectionLost(mhm.role, err)
	}

	return mhm.machine.Fire(HybridModeEventConnectionLost)
}

// MarkReconnected marks that connection was restored
func (mhm *ManagedHybridMode) MarkReconnected(conn *nats.Conn) error {
	mhm.reconnectCount++

	if mhm.callbacks != nil && mhm.callbacks.OnConnectionReady != nil {
		mhm.callbacks.OnConnectionReady(mhm.role, conn)
	}

	return mhm.machine.Fire(HybridModeEventReconnected)
}

// MarkReachabilityChecked marks that reachability was checked
func (mhm *ManagedHybridMode) MarkReachabilityChecked(reachability NetworkReachability) error {
	mhm.reachability = reachability
	mhm.lastCheckTime = time.Now()
	return mhm.machine.Fire(HybridModeEventReachabilityCheck)
}

// Fail marks the hybrid mode as failed
func (mhm *ManagedHybridMode) Fail(err error) error {
	mhm.lastError = err
	return mhm.machine.Fire(HybridModeEventFail)
}

// Stop stops the hybrid mode manager
func (mhm *ManagedHybridMode) Stop() error {
	return mhm.machine.Fire(HybridModeEventStop)
}

// Reset resets from failed state
func (mhm *ManagedHybridMode) Reset() error {
	return mhm.machine.Fire(HybridModeEventReset)
}

// State returns the current state
func (mhm *ManagedHybridMode) State() HybridModeState {
	return mhm.machine.State()
}

// Role returns the current connection role
func (mhm *ManagedHybridMode) Role() ConnectionRole {
	return mhm.role
}

// Reachability returns the detected network reachability
func (mhm *ManagedHybridMode) Reachability() NetworkReachability {
	return mhm.reachability
}

// IsIdle returns true if manager is idle
func (mhm *ManagedHybridMode) IsIdle() bool {
	return mhm.State() == HybridModeStateIdle
}

// IsDetermining returns true if determining role
func (mhm *ManagedHybridMode) IsDetermining() bool {
	return mhm.State() == HybridModeStateDetermining
}

// IsConnecting returns true if connecting
func (mhm *ManagedHybridMode) IsConnecting() bool {
	return mhm.State() == HybridModeStateConnecting
}

// IsHosting returns true if hosting
func (mhm *ManagedHybridMode) IsHosting() bool {
	return mhm.State() == HybridModeStateHosting
}

// IsActive returns true if active
func (mhm *ManagedHybridMode) IsActive() bool {
	return mhm.State() == HybridModeStateActive
}

// IsFailed returns true if failed
func (mhm *ManagedHybridMode) IsFailed() bool {
	return mhm.State() == HybridModeStateFailed
}

// IsTerminal returns true if in terminal state
func (mhm *ManagedHybridMode) IsTerminal() bool {
	state := mhm.State()
	return state == HybridModeStateIdle || state == HybridModeStateFailed
}

// IsRunning returns true if actively running
func (mhm *ManagedHybridMode) IsRunning() bool {
	state := mhm.State()
	return state != HybridModeStateIdle && state != HybridModeStateFailed
}

// Duration returns how long the manager has been running
func (mhm *ManagedHybridMode) Duration() time.Duration {
	if mhm.startTime.IsZero() {
		return 0
	}
	return time.Since(mhm.startTime)
}

// ActiveDuration returns how long in active state
func (mhm *ManagedHybridMode) ActiveDuration() time.Duration {
	if mhm.activeTime.IsZero() || !mhm.IsActive() {
		return 0
	}
	return time.Since(mhm.activeTime)
}

// StateDuration returns how long in current state
func (mhm *ManagedHybridMode) StateDuration() time.Duration {
	return time.Since(mhm.stateChangeTime)
}

// ConnectionCount returns total connection count
func (mhm *ManagedHybridMode) ConnectionCount() int {
	return mhm.connectionCount
}

// ReconnectCount returns reconnection count
func (mhm *ManagedHybridMode) ReconnectCount() int {
	return mhm.reconnectCount
}

// Error returns the last error
func (mhm *ManagedHybridMode) Error() error {
	return mhm.lastError
}

// History returns the state transition history
func (mhm *ManagedHybridMode) History() *statemachine.History[HybridModeState, HybridModeEvent] {
	return mhm.machine.History()
}

// AvailableEvents returns events valid for the current state
func (mhm *ManagedHybridMode) AvailableEvents() []HybridModeEvent {
	return mhm.machine.AvailableEvents()
}

// CanTransition checks if an event is valid for current state
func (mhm *ManagedHybridMode) CanTransition(event HybridModeEvent) bool {
	return mhm.machine.CanFire(event)
}

// HybridModeStateToString converts a HybridModeState to a display string
func HybridModeStateToString(state HybridModeState) string {
	switch state {
	case HybridModeStateIdle:
		return "Idle"
	case HybridModeStateDetermining:
		return "Determining"
	case HybridModeStateConnecting:
		return "Connecting"
	case HybridModeStateHosting:
		return "Hosting"
	case HybridModeStateActive:
		return "Active"
	case HybridModeStateFailed:
		return "Failed"
	default:
		return fmt.Sprintf("unknown(%d)", state)
	}
}

// StateDiagram returns a Mermaid state diagram
func (mhm *ManagedHybridMode) StateDiagram() string {
	return `stateDiagram-v2
    [*] --> Idle
    Idle --> Determining: start

    Determining --> Connecting: connecting (client role)
    Determining --> Hosting: hosting (host/leaf role)
    Determining --> Failed: fail
    Determining --> Idle: stop

    Connecting --> Active: connected
    Connecting --> Hosting: fallback_to_host
    Connecting --> Failed: fail
    Connecting --> Idle: stop

    Hosting --> Active: server_started
    Hosting --> Connecting: fallback_to_client
    Hosting --> Failed: fail
    Hosting --> Idle: stop

    Active --> Active: reconnected
    Active --> Active: reachability_check
    Active --> Determining: connection_lost
    Active --> Failed: fail
    Active --> Idle: stop

    Failed --> Idle: reset
    Failed --> Idle: stop

    note right of Determining
        Checks network reachability
        Selects best role
    end note

    note right of Connecting
        Client mode: connects
        to external NATS
    end note

    note right of Hosting
        Host/Leaf mode: runs
        embedded NATS server
    end note`
}

// ManagedHybridModeStats contains statistics for the managed hybrid mode
type ManagedHybridModeStats struct {
	State           HybridModeState
	Role            ConnectionRole
	Reachability    NetworkReachability
	Duration        time.Duration
	ActiveDuration  time.Duration
	StateDuration   time.Duration
	ConnectionCount int
	ReconnectCount  int
	LastError       string
	ManagerStats    *HybridModeStats
}

// GetStats returns current statistics
func (mhm *ManagedHybridMode) GetStats() *ManagedHybridModeStats {
	stats := &ManagedHybridModeStats{
		State:           mhm.State(),
		Role:            mhm.Role(),
		Reachability:    mhm.Reachability(),
		Duration:        mhm.Duration(),
		ActiveDuration:  mhm.ActiveDuration(),
		StateDuration:   mhm.StateDuration(),
		ConnectionCount: mhm.connectionCount,
		ReconnectCount:  mhm.reconnectCount,
	}

	if mhm.lastError != nil {
		stats.LastError = mhm.lastError.Error()
	}

	if mhm.Manager != nil {
		stats.ManagerStats = mhm.Manager.GetStats()
	}

	return stats
}
