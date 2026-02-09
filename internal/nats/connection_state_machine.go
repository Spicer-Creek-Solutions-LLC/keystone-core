package nats

import (
	"context"
	"time"

	"github.com/shawnbutts/keystone-core/pkg/statemachine"
)

// ConnStateEvent represents events that trigger connection state transitions.
//
// State diagram:
//
// ```mermaid
// stateDiagram-v2
//
//	[*] --> Disconnected
//	Disconnected --> Connecting: Connect
//	Connecting --> Connected: Connected
//	Connecting --> Disconnected: Failed
//	Connected --> Reconnecting: Disconnected
//	Connected --> Disconnected: Disconnect
//	Reconnecting --> Connected: Reconnected
//	Reconnecting --> Disconnected: Failed
//	Disconnected --> Closed: Close
//	Connecting --> Closed: Close
//	Connected --> Closed: Close
//	Reconnecting --> Closed: Close
//	Closed --> [*]
//
// ```
type ConnStateEvent string

const (
	// EventConnect initiates a connection attempt
	EventConnect ConnStateEvent = "connect"
	// EventConnected signals successful connection
	EventConnected ConnStateEvent = "connected"
	// EventFailed signals connection failure
	EventFailed ConnStateEvent = "failed"
	// EventDisconnected signals loss of connection
	EventDisconnected ConnStateEvent = "disconnected"
	// EventReconnected signals successful reconnection
	EventReconnected ConnStateEvent = "reconnected"
	// EventClose permanently closes the connection
	EventClose ConnStateEvent = "close"
)

// NewConnectionStateMachine creates a state machine for managing connection states.
func NewConnectionStateMachine(callbacks *ConnectionCallbacks) *statemachine.Machine[ConnectionState, ConnStateEvent] {
	builder := statemachine.New[ConnectionState, ConnStateEvent](ConnectionStateDisconnected).
		WithName("nats-connection").
		WithHistory(50).
		// Disconnected state transitions
		AddTransition(ConnectionStateDisconnected, EventConnect, ConnectionStateConnecting).
		AddTransition(ConnectionStateDisconnected, EventClose, ConnectionStateClosed).
		// Connecting state transitions
		AddTransition(ConnectionStateConnecting, EventConnected, ConnectionStateConnected).
		AddTransition(ConnectionStateConnecting, EventFailed, ConnectionStateDisconnected).
		AddTransition(ConnectionStateConnecting, EventClose, ConnectionStateClosed).
		// Connected state transitions
		AddTransition(ConnectionStateConnected, EventDisconnected, ConnectionStateReconnecting).
		AddTransition(ConnectionStateConnected, EventClose, ConnectionStateClosed).
		// Reconnecting state transitions
		AddTransition(ConnectionStateReconnecting, EventReconnected, ConnectionStateConnected).
		AddTransition(ConnectionStateReconnecting, EventFailed, ConnectionStateDisconnected).
		AddTransition(ConnectionStateReconnecting, EventClose, ConnectionStateClosed)

	// Add callbacks if provided
	if callbacks != nil {
		if callbacks.OnConnect != nil {
			builder.OnEnter(ConnectionStateConnected, func(ctx context.Context, state, from ConnectionState) {
				// OnConnect callback will be invoked by the connection manager
				// with the endpoint information
			})
		}
	}

	return builder.MustBuild()
}

// SMCircuitBreakerState represents the state of a circuit breaker using state machine pattern.
//
// State diagram:
//
// ```mermaid
// stateDiagram-v2
//
//	[*] --> Closed
//	Closed --> Open: FailureThresholdReached
//	Open --> HalfOpen: TimeoutElapsed
//	HalfOpen --> Closed: ProbeSuccess
//	HalfOpen --> Open: ProbeFailed
//
// ```
type SMCircuitBreakerState string

const (
	// SMCBStateClosed allows requests to flow through
	SMCBStateClosed SMCircuitBreakerState = "closed"
	// SMCBStateOpen blocks all requests
	SMCBStateOpen SMCircuitBreakerState = "open"
	// SMCBStateHalfOpen allows a probe request to test recovery
	SMCBStateHalfOpen SMCircuitBreakerState = "half_open"
)

// SMCircuitBreakerEvent represents events that trigger circuit breaker transitions.
type SMCircuitBreakerEvent string

const (
	// SMCBEventThresholdReached signals failure threshold has been reached
	SMCBEventThresholdReached SMCircuitBreakerEvent = "threshold_reached"
	// SMCBEventTimeoutElapsed signals the open timeout has elapsed
	SMCBEventTimeoutElapsed SMCircuitBreakerEvent = "timeout_elapsed"
	// SMCBEventProbeSuccess signals the probe request succeeded
	SMCBEventProbeSuccess SMCircuitBreakerEvent = "probe_success"
	// SMCBEventProbeFailed signals the probe request failed
	SMCBEventProbeFailed SMCircuitBreakerEvent = "probe_failed"
)

// SMCircuitBreaker manages circuit breaker state using a state machine.
type SMCircuitBreaker struct {
	machine      *statemachine.Machine[SMCircuitBreakerState, SMCircuitBreakerEvent]
	config       *CircuitBreakerConfig
	failureCount int
	successCount int
	openedAt     time.Time
	onOpen       func()
	onClose      func()
}

// NewSMCircuitBreaker creates a new circuit breaker with the given configuration.
func NewSMCircuitBreaker(config *CircuitBreakerConfig, onOpen, onClose func()) *SMCircuitBreaker {
	if config == nil {
		config = DefaultCircuitBreakerConfig()
	}

	cb := &SMCircuitBreaker{
		config:  config,
		onOpen:  onOpen,
		onClose: onClose,
	}

	cb.machine = statemachine.New[SMCircuitBreakerState, SMCircuitBreakerEvent](SMCBStateClosed).
		WithName("circuit-breaker").
		WithHistory(20).
		// Closed state: normal operation
		AddTransition(SMCBStateClosed, SMCBEventThresholdReached, SMCBStateOpen).
		// Open state: blocking requests
		AddTransition(SMCBStateOpen, SMCBEventTimeoutElapsed, SMCBStateHalfOpen).
		// Half-open state: probing
		AddTransition(SMCBStateHalfOpen, SMCBEventProbeSuccess, SMCBStateClosed).
		AddTransition(SMCBStateHalfOpen, SMCBEventProbeFailed, SMCBStateOpen).
		// Callbacks
		OnEnter(SMCBStateOpen, func(ctx context.Context, state, from SMCircuitBreakerState) {
			cb.openedAt = time.Now()
			cb.successCount = 0
			if cb.onOpen != nil {
				cb.onOpen()
			}
		}).
		OnEnter(SMCBStateClosed, func(ctx context.Context, state, from SMCircuitBreakerState) {
			cb.failureCount = 0
			cb.successCount = 0
			if from != SMCBStateClosed && cb.onClose != nil {
				cb.onClose()
			}
		}).
		OnEnter(SMCBStateHalfOpen, func(ctx context.Context, state, from SMCircuitBreakerState) {
			cb.successCount = 0
		}).
		MustBuild()

	return cb
}

// State returns the current circuit breaker state.
func (cb *SMCircuitBreaker) State() SMCircuitBreakerState {
	return cb.machine.State()
}

// IsOpen returns true if the circuit is open (blocking requests).
func (cb *SMCircuitBreaker) IsOpen() bool {
	return cb.machine.IsInState(SMCBStateOpen)
}

// IsClosed returns true if the circuit is closed (allowing requests).
func (cb *SMCircuitBreaker) IsClosed() bool {
	return cb.machine.IsInState(SMCBStateClosed)
}

// IsHalfOpen returns true if the circuit is in half-open state.
func (cb *SMCircuitBreaker) IsHalfOpen() bool {
	return cb.machine.IsInState(SMCBStateHalfOpen)
}

// AllowRequest returns true if a request should be allowed through.
// For open circuits, it checks if the timeout has elapsed and transitions to half-open.
func (cb *SMCircuitBreaker) AllowRequest() bool {
	state := cb.machine.State()

	switch state {
	case SMCBStateClosed:
		return true
	case SMCBStateHalfOpen:
		return true // Allow probe request
	case SMCBStateOpen:
		// Check if timeout has elapsed
		if time.Since(cb.openedAt) >= cb.config.OpenDuration {
			cb.machine.Fire(SMCBEventTimeoutElapsed)
			return true // Allow probe request
		}
		return false
	default:
		return false
	}
}

// RecordSuccess records a successful operation.
func (cb *SMCircuitBreaker) RecordSuccess() {
	state := cb.machine.State()

	switch state {
	case SMCBStateClosed:
		// Reset failure count on success in closed state
		cb.failureCount = 0
	case SMCBStateHalfOpen:
		cb.successCount++
		if cb.successCount >= cb.config.SuccessThreshold {
			cb.machine.Fire(SMCBEventProbeSuccess)
		}
	default:
	}
}

// RecordFailure records a failed operation.
func (cb *SMCircuitBreaker) RecordFailure() {
	state := cb.machine.State()

	switch state {
	case SMCBStateClosed:
		cb.failureCount++
		if cb.failureCount >= cb.config.FailureThreshold {
			cb.machine.Fire(SMCBEventThresholdReached)
		}
	case SMCBStateHalfOpen:
		cb.machine.Fire(SMCBEventProbeFailed)
	default:
	}
}

// Reset resets the circuit breaker to closed state.
func (cb *SMCircuitBreaker) Reset() {
	cb.machine.Reset()
	cb.failureCount = 0
	cb.successCount = 0
}

// NextRetryTime returns when the next retry should be attempted.
// Returns zero time if circuit is not open.
func (cb *SMCircuitBreaker) NextRetryTime() time.Time {
	if cb.IsOpen() {
		return cb.openedAt.Add(cb.config.OpenDuration)
	}
	return time.Time{}
}

// History returns the circuit breaker's transition history.
func (cb *SMCircuitBreaker) History() *statemachine.History[SMCircuitBreakerState, SMCircuitBreakerEvent] {
	return cb.machine.History()
}

// ManagedEndpoint wraps an EndpointState with state machines for connection and circuit breaker.
type ManagedEndpoint struct {
	*EndpointState
	connMachine    *statemachine.Machine[ConnectionState, ConnStateEvent]
	circuitBreaker *SMCircuitBreaker
}

// NewManagedEndpoint creates a new managed endpoint with state machines.
func NewManagedEndpoint(endpoint *Endpoint, cbConfig *CircuitBreakerConfig, onCircuitOpen, onCircuitClose func()) *ManagedEndpoint {
	state := &EndpointState{
		Endpoint: endpoint,
		State:    ConnectionStateDisconnected,
	}

	me := &ManagedEndpoint{
		EndpointState:  state,
		connMachine:    NewConnectionStateMachine(nil),
		circuitBreaker: NewSMCircuitBreaker(cbConfig, onCircuitOpen, onCircuitClose),
	}

	return me
}

// TransitionTo attempts to transition the connection to a new state via an event.
func (me *ManagedEndpoint) TransitionTo(event ConnStateEvent) error {
	err := me.connMachine.Fire(event)
	if err == nil {
		me.State = me.connMachine.State()
	}
	return err
}

// ConnectionState returns the current connection state from the state machine.
func (me *ManagedEndpoint) ConnectionState() ConnectionState {
	return me.connMachine.State()
}

// CanConnect returns true if a connection attempt is allowed.
func (me *ManagedEndpoint) CanConnect() bool {
	// Check circuit breaker first
	if !me.circuitBreaker.AllowRequest() {
		return false
	}
	// Check if we can transition to connecting
	return me.connMachine.CanFire(EventConnect)
}

// RecordConnectionSuccess records a successful connection attempt.
func (me *ManagedEndpoint) RecordConnectionSuccess(latency time.Duration) {
	me.SuccessCount++
	me.TotalLatency += latency
	me.LastConnected = time.Now()
	me.LastError = nil
	me.circuitBreaker.RecordSuccess()
}

// RecordConnectionFailure records a failed connection attempt.
func (me *ManagedEndpoint) RecordConnectionFailure(err error) {
	me.FailureCount++
	me.LastError = err
	me.LastErrorTime = time.Now()
	me.circuitBreaker.RecordFailure()
}

// IsHealthy returns true if the endpoint is connected and circuit is closed.
func (me *ManagedEndpoint) IsHealthy() bool {
	return me.connMachine.IsInState(ConnectionStateConnected) && me.circuitBreaker.IsClosed()
}

// ConnectionHistory returns the connection state machine's transition history.
func (me *ManagedEndpoint) ConnectionHistory() *statemachine.History[ConnectionState, ConnStateEvent] {
	return me.connMachine.History()
}
