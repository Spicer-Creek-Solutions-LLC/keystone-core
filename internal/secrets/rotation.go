package secrets

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/shawnbutts/keystone-core/pkg/statemachine"
)

// RotationEvent represents events that can occur during secret rotation.
//
// State Diagram (Mermaid):
//
//	stateDiagram-v2
//	    [*] --> Pending
//	    Pending --> InProgress: start
//	    InProgress --> Verifying: batch_complete
//	    InProgress --> Failed: failure
//	    InProgress --> Cancelled: cancel
//	    Verifying --> InProgress: continue_rolling
//	    Verifying --> Completed: verification_passed
//	    Verifying --> Failed: verification_failed
//	    Verifying --> Cancelled: cancel
//	    Failed --> RolledBack: rollback
//	    Failed --> Cancelled: cancel
//	    Completed --> [*]
//	    RolledBack --> [*]
//	    Cancelled --> [*]
//
//	    InProgress --> InProgress: batch_progress
//	    Verifying --> Verifying: health_check
type RotationEvent string

const (
	// RotationEventStart begins the rotation process.
	RotationEventStart RotationEvent = "start"

	// RotationEventBatchProgress indicates progress within a batch.
	RotationEventBatchProgress RotationEvent = "batch_progress"

	// RotationEventBatchComplete indicates a batch has finished.
	RotationEventBatchComplete RotationEvent = "batch_complete"

	// RotationEventContinueRolling continues to the next batch in rolling strategy.
	RotationEventContinueRolling RotationEvent = "continue_rolling"

	// RotationEventHealthCheck triggers a health check during verification.
	RotationEventHealthCheck RotationEvent = "health_check"

	// RotationEventVerificationPassed indicates verification succeeded.
	RotationEventVerificationPassed RotationEvent = "verification_passed"

	// RotationEventVerificationFailed indicates verification failed.
	RotationEventVerificationFailed RotationEvent = "verification_failed"

	// RotationEventFailure indicates a failure during rotation.
	RotationEventFailure RotationEvent = "failure"

	// RotationEventRollback triggers rollback to previous version.
	RotationEventRollback RotationEvent = "rollback"

	// RotationEventCancel cancels a failed rotation without rollback.
	RotationEventCancel RotationEvent = "cancel"
)

// RotationCallbacks defines callbacks for rotation state transitions.
type RotationCallbacks struct {
	// OnStateChange is called when the rotation state changes.
	OnStateChange func(rotation *ManagedRotation, from, to RotationState)

	// OnProgress is called when progress is updated.
	OnProgress func(rotation *ManagedRotation, progress RotationProgress)

	// OnBatchComplete is called when a batch completes.
	OnBatchComplete func(rotation *ManagedRotation, batch int, succeeded, failed int)

	// OnVerificationStart is called when verification begins.
	OnVerificationStart func(rotation *ManagedRotation)

	// OnHealthCheckResult is called with health check results.
	OnHealthCheckResult func(rotation *ManagedRotation, target string, healthy bool, err error)

	// OnComplete is called when rotation completes successfully.
	OnComplete func(rotation *ManagedRotation)

	// OnFailed is called when rotation fails.
	OnFailed func(rotation *ManagedRotation, err error)

	// OnRollback is called when rollback begins.
	OnRollback func(rotation *ManagedRotation)

	// OnRollbackComplete is called when rollback completes.
	OnRollbackComplete func(rotation *ManagedRotation, succeeded, failed int)
}

// RotationProgress tracks the progress of a rotation operation.
type RotationProgress struct {
	// TotalTargets is the total number of targets to update.
	TotalTargets int `json:"total_targets"`

	// UpdatedTargets is the number of successfully updated targets.
	UpdatedTargets int `json:"updated_targets"`

	// FailedTargets is the number of failed target updates.
	FailedTargets int `json:"failed_targets"`

	// CurrentBatch is the current batch number (for rolling strategy).
	CurrentBatch int `json:"current_batch"`

	// TotalBatches is the total number of batches.
	TotalBatches int `json:"total_batches"`

	// Percentage is the overall progress percentage.
	Percentage int `json:"percentage"`

	// Message is a human-readable status message.
	Message string `json:"message"`
}

// RotationTarget represents a target that needs to receive the rotated secret.
type RotationTarget struct {
	// ID is the unique target identifier.
	ID string `json:"id"`

	// Name is a human-readable target name.
	Name string `json:"name"`

	// Type is the target type (agent, service, etc.).
	Type string `json:"type"`

	// AgentID is the agent ID if this is an agent target.
	AgentID string `json:"agent_id,omitempty"`

	// Endpoint is the target endpoint for health checks.
	Endpoint string `json:"endpoint,omitempty"`

	// Status is the target's update status.
	Status TargetStatus `json:"status"`

	// Error is any error that occurred updating this target.
	Error string `json:"error,omitempty"`

	// UpdatedAt is when this target was updated.
	UpdatedAt time.Time `json:"updated_at,omitempty"`
}

// TargetStatus represents the status of a rotation target.
type TargetStatus string

const (
	TargetStatusPending  TargetStatus = "pending"
	TargetStatusUpdating TargetStatus = "updating"
	TargetStatusUpdated  TargetStatus = "updated"
	TargetStatusVerified TargetStatus = "verified"
	TargetStatusFailed   TargetStatus = "failed"
	TargetStatusRolled   TargetStatus = "rolled_back"
)

// ManagedRotation wraps a rotation operation with an explicit state machine.
type ManagedRotation struct {
	mu sync.RWMutex

	// Rotation is the underlying rotation data.
	Rotation *Rotation

	// Config is the rotation configuration.
	Config *RotationConfig

	// Targets is the list of targets to update.
	Targets []*RotationTarget

	// Progress tracks rotation progress.
	Progress RotationProgress

	// machine is the state machine managing transitions.
	machine *statemachine.Machine[RotationState, RotationEvent]

	// callbacks are the rotation callbacks.
	callbacks *RotationCallbacks

	// strategy is the rotation strategy implementation.
	strategy RotationStrategyExecutor

	// broker is the secret broker for reading/writing secrets.
	broker *SecretBroker

	// eventPublisher publishes rotation events.
	eventPublisher RotationEventPublisher

	// lastError is the most recent error.
	lastError error

	// startTime is when the rotation started.
	startTime time.Time

	// endTime is when the rotation ended.
	endTime time.Time

	// oldSecretData stores the old secret for rollback.
	oldSecretData map[string]interface{}

	// newSecretData stores the new secret being rotated to.
	newSecretData map[string]interface{}

	// ctx is the rotation context.
	ctx context.Context

	// cancel cancels the rotation context.
	cancel context.CancelFunc
}

// RotationStrategyExecutor executes a specific rotation strategy.
type RotationStrategyExecutor interface {
	// Name returns the strategy name.
	Name() RotationStrategy

	// Execute executes the rotation for a batch of targets.
	Execute(ctx context.Context, rotation *ManagedRotation, targets []*RotationTarget) error

	// Verify verifies targets after rotation.
	Verify(ctx context.Context, rotation *ManagedRotation, targets []*RotationTarget) error

	// Rollback rolls back targets to the previous version.
	Rollback(ctx context.Context, rotation *ManagedRotation, targets []*RotationTarget) error

	// BatchSize returns the batch size for this strategy.
	BatchSize(config *RotationConfig, totalTargets int) int

	// BatchDelay returns the delay between batches.
	BatchDelay(config *RotationConfig) time.Duration
}

// RotationEventPublisher publishes rotation events.
type RotationEventPublisher interface {
	// Publish publishes a rotation event.
	Publish(ctx context.Context, event *RotationStateEvent) error
}

// RotationStateEvent represents a rotation state change event.
type RotationStateEvent struct {
	// RotationID is the rotation identifier.
	RotationID string `json:"rotation_id"`

	// SecretPath is the secret being rotated.
	SecretPath string `json:"secret_path"`

	// Strategy is the rotation strategy.
	Strategy RotationStrategy `json:"strategy"`

	// FromState is the previous state.
	FromState RotationState `json:"from_state"`

	// ToState is the new state.
	ToState RotationState `json:"to_state"`

	// Event is the event that triggered the transition.
	Event RotationEvent `json:"event"`

	// Progress is the current progress.
	Progress RotationProgress `json:"progress"`

	// Error is any error message.
	Error string `json:"error,omitempty"`

	// Timestamp is when the event occurred.
	Timestamp time.Time `json:"timestamp"`
}

// NewManagedRotation creates a new managed rotation operation.
func NewManagedRotation(
	id string,
	secretPath string,
	config *RotationConfig,
	targets []*RotationTarget,
	callbacks *RotationCallbacks,
) *ManagedRotation {
	ctx, cancel := context.WithCancel(context.Background())

	mr := &ManagedRotation{
		Rotation: &Rotation{
			ID:           id,
			SecretPath:   secretPath,
			State:        RotationStatePending,
			Strategy:     config.Strategy,
			TotalTargets: len(targets),
		},
		Config:    config,
		Targets:   targets,
		callbacks: callbacks,
		Progress: RotationProgress{
			TotalTargets: len(targets),
			Message:      "Pending",
		},
		ctx:    ctx,
		cancel: cancel,
	}

	mr.machine = mr.buildStateMachine()

	// Calculate total batches based on strategy
	if config.Strategy == RotationStrategyRolling {
		batchSize := config.BatchSize
		if batchSize <= 0 {
			batchSize = 1
		}
		mr.Progress.TotalBatches = (len(targets) + batchSize - 1) / batchSize
	} else {
		mr.Progress.TotalBatches = 1
	}

	return mr
}

// buildStateMachine creates the rotation state machine.
func (mr *ManagedRotation) buildStateMachine() *statemachine.Machine[RotationState, RotationEvent] {
	builder := statemachine.New[RotationState, RotationEvent](RotationStatePending).
		WithHistory(50)

	// Pending -> InProgress (start rotation)
	builder.AddTransition(RotationStatePending, RotationEventStart, RotationStateInProgress).
		OnEnter(RotationStateInProgress, func(_ context.Context, _ RotationState, from RotationState) {
			mr.onEnterInProgress(from)
		})

	// InProgress -> InProgress (batch progress - self-loop for progress updates)
	builder.Ignore(RotationStateInProgress, RotationEventBatchProgress)

	// InProgress -> Verifying (batch complete, start verification)
	builder.AddTransition(RotationStateInProgress, RotationEventBatchComplete, RotationStateVerifying).
		OnEnter(RotationStateVerifying, func(_ context.Context, _ RotationState, from RotationState) {
			mr.onEnterVerifying(from)
		})

	// InProgress -> Failed (failure during rotation)
	builder.AddTransition(RotationStateInProgress, RotationEventFailure, RotationStateFailed).
		OnEnter(RotationStateFailed, func(_ context.Context, _ RotationState, from RotationState) {
			mr.onEnterFailed(from)
		})

	// Verifying -> Verifying (health check - self-loop)
	builder.Ignore(RotationStateVerifying, RotationEventHealthCheck)

	// Verifying -> InProgress (continue to next batch in rolling strategy)
	builder.AddTransition(RotationStateVerifying, RotationEventContinueRolling, RotationStateInProgress)

	// Verifying -> Completed (verification passed)
	builder.AddTransition(RotationStateVerifying, RotationEventVerificationPassed, RotationStateCompleted).
		OnEnter(RotationStateCompleted, func(_ context.Context, _ RotationState, from RotationState) {
			mr.onEnterCompleted(from)
		})

	// Verifying -> Failed (verification failed)
	builder.AddTransition(RotationStateVerifying, RotationEventVerificationFailed, RotationStateFailed)

	// Failed -> RolledBack (rollback triggered)
	builder.AddTransition(RotationStateFailed, RotationEventRollback, RotationStateRolledBack).
		OnEnter(RotationStateRolledBack, func(_ context.Context, _ RotationState, from RotationState) {
			mr.onEnterRolledBack(from)
		})

	// Cancel transitions - allowed from InProgress, Verifying, and Failed states
	builder.AddTransition(RotationStateInProgress, RotationEventCancel, RotationStateCancelled).
		OnEnter(RotationStateCancelled, func(_ context.Context, _ RotationState, from RotationState) {
			mr.onEnterCancelled(from)
		})
	builder.AddTransition(RotationStateVerifying, RotationEventCancel, RotationStateCancelled)
	builder.AddTransition(RotationStateFailed, RotationEventCancel, RotationStateCancelled)

	// Add global transition callback for event publishing
	builder.OnTransition(func(_ context.Context, from, to RotationState, event RotationEvent) {
		mr.onTransition(from, to, event)
	})

	return builder.MustBuild()
}

// onEnterInProgress is called when entering the InProgress state.
func (mr *ManagedRotation) onEnterInProgress(from RotationState) {
	mr.mu.Lock()
	if mr.startTime.IsZero() {
		mr.startTime = time.Now()
		mr.Rotation.StartedAt = mr.startTime
	}
	mr.Rotation.State = RotationStateInProgress
	mr.Progress.Message = "Rotation in progress"
	mr.mu.Unlock()

	if mr.callbacks != nil && mr.callbacks.OnStateChange != nil {
		mr.callbacks.OnStateChange(mr, from, RotationStateInProgress)
	}
}

// onEnterVerifying is called when entering the Verifying state.
func (mr *ManagedRotation) onEnterVerifying(from RotationState) {
	mr.mu.Lock()
	mr.Rotation.State = RotationStateVerifying
	mr.Progress.Message = "Verifying rotation"
	mr.mu.Unlock()

	if mr.callbacks != nil && mr.callbacks.OnVerificationStart != nil {
		mr.callbacks.OnVerificationStart(mr)
	}

	if mr.callbacks != nil && mr.callbacks.OnStateChange != nil {
		mr.callbacks.OnStateChange(mr, from, RotationStateVerifying)
	}
}

// onEnterCompleted is called when entering the Completed state.
func (mr *ManagedRotation) onEnterCompleted(from RotationState) {
	mr.mu.Lock()
	mr.endTime = time.Now()
	mr.Rotation.State = RotationStateCompleted
	mr.Rotation.CompletedAt = mr.endTime
	mr.Progress.Message = "Rotation completed successfully"
	mr.Progress.Percentage = 100
	mr.mu.Unlock()

	if mr.callbacks != nil && mr.callbacks.OnComplete != nil {
		mr.callbacks.OnComplete(mr)
	}

	if mr.callbacks != nil && mr.callbacks.OnStateChange != nil {
		mr.callbacks.OnStateChange(mr, from, RotationStateCompleted)
	}
}

// onEnterFailed is called when entering the Failed state.
func (mr *ManagedRotation) onEnterFailed(from RotationState) {
	mr.mu.Lock()
	mr.endTime = time.Now()
	mr.Rotation.State = RotationStateFailed
	if mr.lastError != nil {
		mr.Rotation.Error = mr.lastError.Error()
	}
	mr.Progress.Message = "Rotation failed"
	mr.mu.Unlock()

	if mr.callbacks != nil && mr.callbacks.OnFailed != nil {
		mr.callbacks.OnFailed(mr, mr.lastError)
	}

	if mr.callbacks != nil && mr.callbacks.OnStateChange != nil {
		mr.callbacks.OnStateChange(mr, from, RotationStateFailed)
	}

	// Auto-rollback if configured
	if mr.Config != nil && mr.Config.RollbackOnFailure {
		go func() {
			_ = mr.Rollback()
		}()
	}
}

// onEnterRolledBack is called when entering the RolledBack state.
func (mr *ManagedRotation) onEnterRolledBack(from RotationState) {
	mr.mu.Lock()
	mr.Rotation.State = RotationStateRolledBack
	mr.Progress.Message = "Rotation rolled back"
	mr.mu.Unlock()

	if mr.callbacks != nil && mr.callbacks.OnRollback != nil {
		mr.callbacks.OnRollback(mr)
	}

	if mr.callbacks != nil && mr.callbacks.OnStateChange != nil {
		mr.callbacks.OnStateChange(mr, from, RotationStateRolledBack)
	}
}

// onEnterCancelled is called when entering the Cancelled state.
func (mr *ManagedRotation) onEnterCancelled(from RotationState) {
	mr.mu.Lock()
	mr.endTime = time.Now()
	mr.Rotation.State = RotationStateCancelled
	mr.Progress.Message = "Rotation cancelled"
	mr.mu.Unlock()

	if mr.callbacks != nil && mr.callbacks.OnStateChange != nil {
		mr.callbacks.OnStateChange(mr, from, RotationStateCancelled)
	}
}

// onTransition is called after any state transition.
func (mr *ManagedRotation) onTransition(from, to RotationState, event RotationEvent) {
	if mr.eventPublisher == nil {
		return
	}

	mr.mu.RLock()
	rotationEvent := &RotationStateEvent{
		RotationID: mr.Rotation.ID,
		SecretPath: mr.Rotation.SecretPath,
		Strategy:   mr.Rotation.Strategy,
		FromState:  from,
		ToState:    to,
		Event:      event,
		Progress:   mr.Progress,
		Timestamp:  time.Now(),
	}
	if mr.lastError != nil {
		rotationEvent.Error = mr.lastError.Error()
	}
	mr.mu.RUnlock()

	// Publish asynchronously to not block transitions
	go func() {
		_ = mr.eventPublisher.Publish(mr.ctx, rotationEvent)
	}()
}

// Start begins the rotation process.
func (mr *ManagedRotation) Start() error {
	return mr.machine.Fire(RotationEventStart)
}

// MarkBatchProgress updates progress within the current batch.
func (mr *ManagedRotation) MarkBatchProgress(updated, failed int, message string) {
	mr.mu.Lock()
	mr.Progress.UpdatedTargets = updated
	mr.Progress.FailedTargets = failed
	mr.Progress.Message = message
	mr.Rotation.UpdatedTargets = updated
	mr.Rotation.FailedTargets = failed

	// Calculate percentage
	if mr.Progress.TotalTargets > 0 {
		mr.Progress.Percentage = (updated * 100) / mr.Progress.TotalTargets
	}
	mr.mu.Unlock()

	if mr.callbacks != nil && mr.callbacks.OnProgress != nil {
		mr.callbacks.OnProgress(mr, mr.Progress)
	}

	// Fire event (ignored but triggers callbacks)
	_ = mr.machine.Fire(RotationEventBatchProgress)
}

// MarkBatchComplete marks a batch as complete.
func (mr *ManagedRotation) MarkBatchComplete(batch, succeeded, failed int) error {
	mr.mu.Lock()
	mr.Progress.CurrentBatch = batch
	mr.Progress.UpdatedTargets += succeeded
	mr.Progress.FailedTargets += failed
	mr.Rotation.UpdatedTargets = mr.Progress.UpdatedTargets
	mr.Rotation.FailedTargets = mr.Progress.FailedTargets
	mr.mu.Unlock()

	if mr.callbacks != nil && mr.callbacks.OnBatchComplete != nil {
		mr.callbacks.OnBatchComplete(mr, batch, succeeded, failed)
	}

	return mr.machine.Fire(RotationEventBatchComplete)
}

// MarkHealthCheckResult records a health check result.
func (mr *ManagedRotation) MarkHealthCheckResult(targetID string, healthy bool, err error) {
	if mr.callbacks != nil && mr.callbacks.OnHealthCheckResult != nil {
		mr.callbacks.OnHealthCheckResult(mr, targetID, healthy, err)
	}

	// Fire event (ignored but useful for history)
	_ = mr.machine.Fire(RotationEventHealthCheck)
}

// ContinueRolling continues to the next batch in rolling strategy.
func (mr *ManagedRotation) ContinueRolling() error {
	return mr.machine.Fire(RotationEventContinueRolling)
}

// MarkVerificationPassed marks verification as successful.
func (mr *ManagedRotation) MarkVerificationPassed() error {
	return mr.machine.Fire(RotationEventVerificationPassed)
}

// MarkVerificationFailed marks verification as failed.
func (mr *ManagedRotation) MarkVerificationFailed(err error) error {
	mr.mu.Lock()
	mr.lastError = err
	mr.mu.Unlock()
	return mr.machine.Fire(RotationEventVerificationFailed)
}

// Fail marks the rotation as failed.
func (mr *ManagedRotation) Fail(err error) error {
	mr.mu.Lock()
	mr.lastError = err
	mr.mu.Unlock()
	return mr.machine.Fire(RotationEventFailure)
}

// Rollback initiates rollback to the previous version.
func (mr *ManagedRotation) Rollback() error {
	return mr.machine.Fire(RotationEventRollback)
}

// Cancel cancels a failed rotation without rollback.
func (mr *ManagedRotation) Cancel() error {
	mr.cancel()
	return mr.machine.Fire(RotationEventCancel)
}

// State returns the current rotation state.
func (mr *ManagedRotation) State() RotationState {
	return mr.machine.State()
}

// IsComplete returns true if rotation completed successfully.
func (mr *ManagedRotation) IsComplete() bool {
	return mr.State() == RotationStateCompleted
}

// IsFailed returns true if rotation failed.
func (mr *ManagedRotation) IsFailed() bool {
	return mr.State() == RotationStateFailed
}

// IsCancelled returns true if rotation was cancelled.
func (mr *ManagedRotation) IsCancelled() bool {
	return mr.State() == RotationStateCancelled
}

// IsTerminal returns true if rotation is in a terminal state.
func (mr *ManagedRotation) IsTerminal() bool {
	state := mr.State()
	return state == RotationStateCompleted || state == RotationStateFailed || state == RotationStateRolledBack || state == RotationStateCancelled
}

// Duration returns the elapsed time.
func (mr *ManagedRotation) Duration() time.Duration {
	mr.mu.RLock()
	defer mr.mu.RUnlock()

	if !mr.endTime.IsZero() {
		return mr.endTime.Sub(mr.startTime)
	}
	if mr.startTime.IsZero() {
		return 0
	}
	return time.Since(mr.startTime)
}

// Error returns the last error.
func (mr *ManagedRotation) Error() error {
	mr.mu.RLock()
	defer mr.mu.RUnlock()
	return mr.lastError
}

// GetProgress returns the current progress.
func (mr *ManagedRotation) GetProgress() RotationProgress {
	mr.mu.RLock()
	defer mr.mu.RUnlock()
	return mr.Progress
}

// SetBroker sets the secret broker.
func (mr *ManagedRotation) SetBroker(broker *SecretBroker) {
	mr.mu.Lock()
	defer mr.mu.Unlock()
	mr.broker = broker
}

// SetEventPublisher sets the event publisher.
func (mr *ManagedRotation) SetEventPublisher(publisher RotationEventPublisher) {
	mr.mu.Lock()
	defer mr.mu.Unlock()
	mr.eventPublisher = publisher
}

// SetStrategy sets the rotation strategy executor.
func (mr *ManagedRotation) SetStrategy(strategy RotationStrategyExecutor) {
	mr.mu.Lock()
	defer mr.mu.Unlock()
	mr.strategy = strategy
}

// SetOldSecret stores the old secret data for rollback.
func (mr *ManagedRotation) SetOldSecret(data map[string]interface{}) {
	mr.mu.Lock()
	defer mr.mu.Unlock()
	mr.oldSecretData = data
}

// SetNewSecret stores the new secret data.
func (mr *ManagedRotation) SetNewSecret(data map[string]interface{}) {
	mr.mu.Lock()
	defer mr.mu.Unlock()
	mr.newSecretData = data
}

// GetOldSecret returns the old secret data.
func (mr *ManagedRotation) GetOldSecret() map[string]interface{} {
	mr.mu.RLock()
	defer mr.mu.RUnlock()
	return mr.oldSecretData
}

// GetNewSecret returns the new secret data.
func (mr *ManagedRotation) GetNewSecret() map[string]interface{} {
	mr.mu.RLock()
	defer mr.mu.RUnlock()
	return mr.newSecretData
}

// Context returns the rotation context.
func (mr *ManagedRotation) Context() context.Context {
	return mr.ctx
}

// History returns the state transition history.
func (mr *ManagedRotation) History() *statemachine.History[RotationState, RotationEvent] {
	return mr.machine.History()
}

// CanTransition checks if an event is valid for current state.
func (mr *ManagedRotation) CanTransition(event RotationEvent) bool {
	return mr.machine.CanFire(event)
}

// AvailableEvents returns events valid for the current state.
func (mr *ManagedRotation) AvailableEvents() []RotationEvent {
	return mr.machine.AvailableEvents()
}

// StateDiagram returns a Mermaid state diagram of the rotation flow.
func (mr *ManagedRotation) StateDiagram() string {
	return `stateDiagram-v2
    [*] --> Pending
    Pending --> InProgress: start
    InProgress --> Verifying: batch_complete
    InProgress --> Failed: failure
    InProgress --> Cancelled: cancel
    Verifying --> InProgress: continue_rolling
    Verifying --> Completed: verification_passed
    Verifying --> Failed: verification_failed
    Verifying --> Cancelled: cancel
    Failed --> RolledBack: rollback
    Failed --> Cancelled: cancel
    Completed --> [*]
    RolledBack --> [*]
    Cancelled --> [*]

    note right of InProgress: Updates targets in batches
    note right of Verifying: Health checks all updated targets
    note right of Failed: Auto-rollback if configured`
}

// RotationOrchestrator manages secret rotation operations.
type RotationOrchestrator struct {
	mu sync.RWMutex

	// broker is the secret broker.
	broker *SecretBroker

	// eventPublisher publishes rotation events.
	eventPublisher RotationEventPublisher

	// strategies maps strategy types to executors.
	strategies map[RotationStrategy]RotationStrategyExecutor

	// activeRotations tracks active rotations.
	activeRotations map[string]*ManagedRotation

	// callbacks are the default callbacks.
	callbacks *RotationCallbacks
}

// NewRotationOrchestrator creates a new rotation orchestrator.
func NewRotationOrchestrator(broker *SecretBroker) *RotationOrchestrator {
	ro := &RotationOrchestrator{
		broker:          broker,
		strategies:      make(map[RotationStrategy]RotationStrategyExecutor),
		activeRotations: make(map[string]*ManagedRotation),
	}

	// Register default strategies
	ro.RegisterStrategy(&BlueGreenStrategy{})
	ro.RegisterStrategy(&RollingStrategy{})

	return ro
}

// RegisterStrategy registers a rotation strategy executor.
func (ro *RotationOrchestrator) RegisterStrategy(strategy RotationStrategyExecutor) {
	ro.mu.Lock()
	defer ro.mu.Unlock()
	ro.strategies[strategy.Name()] = strategy
}

// SetEventPublisher sets the event publisher.
func (ro *RotationOrchestrator) SetEventPublisher(publisher RotationEventPublisher) {
	ro.mu.Lock()
	defer ro.mu.Unlock()
	ro.eventPublisher = publisher
}

// SetCallbacks sets the default callbacks.
func (ro *RotationOrchestrator) SetCallbacks(callbacks *RotationCallbacks) {
	ro.mu.Lock()
	defer ro.mu.Unlock()
	ro.callbacks = callbacks
}

// StartRotation starts a new rotation operation.
func (ro *RotationOrchestrator) StartRotation(
	ctx context.Context,
	id string,
	secretPath string,
	config *RotationConfig,
	targets []*RotationTarget,
) (*ManagedRotation, error) {
	ro.mu.Lock()
	if _, exists := ro.activeRotations[id]; exists {
		ro.mu.Unlock()
		return nil, fmt.Errorf("rotation %s already exists", id)
	}

	strategy, ok := ro.strategies[config.Strategy]
	if !ok {
		ro.mu.Unlock()
		return nil, fmt.Errorf("unknown rotation strategy: %s", config.Strategy)
	}

	rotation := NewManagedRotation(id, secretPath, config, targets, ro.callbacks)
	rotation.SetBroker(ro.broker)
	rotation.SetEventPublisher(ro.eventPublisher)
	rotation.SetStrategy(strategy)

	ro.activeRotations[id] = rotation
	ro.mu.Unlock()

	// Start the rotation
	if err := rotation.Start(); err != nil {
		ro.mu.Lock()
		delete(ro.activeRotations, id)
		ro.mu.Unlock()
		return nil, fmt.Errorf("failed to start rotation: %w", err)
	}

	// Execute the rotation asynchronously
	go ro.executeRotation(ctx, rotation)

	return rotation, nil
}

// executeRotation executes the rotation workflow.
func (ro *RotationOrchestrator) executeRotation(ctx context.Context, rotation *ManagedRotation) {
	defer func() {
		// Clean up when done
		if rotation.IsTerminal() {
			ro.mu.Lock()
			delete(ro.activeRotations, rotation.Rotation.ID)
			ro.mu.Unlock()
		}
	}()

	ro.mu.RLock()
	strategy := rotation.strategy
	ro.mu.RUnlock()

	if strategy == nil {
		rotation.Fail(fmt.Errorf("no strategy set"))
		return
	}

	config := rotation.Config
	targets := rotation.Targets
	batchSize := strategy.BatchSize(config, len(targets))

	// Process targets in batches
	for batchNum := 0; batchNum < len(targets); batchNum += batchSize {
		select {
		case <-ctx.Done():
			rotation.Fail(ctx.Err())
			return
		case <-rotation.ctx.Done():
			return
		default:
		}

		// Get batch
		end := batchNum + batchSize
		if end > len(targets) {
			end = len(targets)
		}
		batch := targets[batchNum:end]
		currentBatch := (batchNum / batchSize) + 1

		// Execute batch
		if err := strategy.Execute(ctx, rotation, batch); err != nil {
			rotation.Fail(err)
			return
		}

		// Count results
		succeeded := 0
		failed := 0
		for _, t := range batch {
			if t.Status == TargetStatusUpdated {
				succeeded++
			} else if t.Status == TargetStatusFailed {
				failed++
			}
		}

		// Check failure threshold
		if config.FailureThreshold > 0 {
			failureRate := float64(rotation.Progress.FailedTargets+failed) / float64(rotation.Progress.TotalTargets)
			if failureRate > config.FailureThreshold {
				rotation.Fail(fmt.Errorf("failure rate %.2f exceeds threshold %.2f", failureRate, config.FailureThreshold))
				return
			}
		}

		// Mark batch complete
		if err := rotation.MarkBatchComplete(currentBatch, succeeded, failed); err != nil {
			rotation.Fail(err)
			return
		}

		// Verify batch
		if err := strategy.Verify(ctx, rotation, batch); err != nil {
			rotation.MarkVerificationFailed(err)
			return
		}

		// If more batches remain, continue rolling
		if end < len(targets) {
			if err := rotation.ContinueRolling(); err != nil {
				rotation.Fail(err)
				return
			}

			// Wait between batches
			delay := strategy.BatchDelay(config)
			if delay > 0 {
				select {
				case <-ctx.Done():
					rotation.Fail(ctx.Err())
					return
				case <-time.After(delay):
				}
			}
		}
	}

	// All batches complete and verified
	rotation.MarkVerificationPassed()
}

// GetRotation returns an active rotation by ID.
func (ro *RotationOrchestrator) GetRotation(id string) (*ManagedRotation, bool) {
	ro.mu.RLock()
	defer ro.mu.RUnlock()
	rotation, ok := ro.activeRotations[id]
	return rotation, ok
}

// ListRotations returns all active rotations.
func (ro *RotationOrchestrator) ListRotations() []*ManagedRotation {
	ro.mu.RLock()
	defer ro.mu.RUnlock()

	rotations := make([]*ManagedRotation, 0, len(ro.activeRotations))
	for _, r := range ro.activeRotations {
		rotations = append(rotations, r)
	}
	return rotations
}

// CancelRotation cancels an active rotation.
func (ro *RotationOrchestrator) CancelRotation(id string) error {
	ro.mu.RLock()
	rotation, ok := ro.activeRotations[id]
	ro.mu.RUnlock()

	if !ok {
		return fmt.Errorf("rotation %s not found", id)
	}

	return rotation.Cancel()
}

// RollbackRotation triggers rollback for a failed rotation.
func (ro *RotationOrchestrator) RollbackRotation(id string) error {
	ro.mu.RLock()
	rotation, ok := ro.activeRotations[id]
	ro.mu.RUnlock()

	if !ok {
		return fmt.Errorf("rotation %s not found", id)
	}

	if !rotation.IsFailed() {
		return fmt.Errorf("rotation %s is not in failed state", id)
	}

	return rotation.Rollback()
}

// BlueGreenStrategy implements the blue-green rotation strategy.
// All targets are updated simultaneously, then verified as a group.
type BlueGreenStrategy struct{}

// Name returns the strategy name.
func (s *BlueGreenStrategy) Name() RotationStrategy {
	return RotationStrategyBlueGreen
}

// BatchSize returns all targets as one batch for blue-green.
func (s *BlueGreenStrategy) BatchSize(config *RotationConfig, totalTargets int) int {
	return totalTargets
}

// BatchDelay returns no delay for blue-green (single batch).
func (s *BlueGreenStrategy) BatchDelay(config *RotationConfig) time.Duration {
	return 0
}

// Execute updates all targets simultaneously.
func (s *BlueGreenStrategy) Execute(ctx context.Context, rotation *ManagedRotation, targets []*RotationTarget) error {
	var wg sync.WaitGroup
	var mu sync.Mutex
	var firstErr error

	for _, target := range targets {
		target.Status = TargetStatusUpdating
	}

	for _, target := range targets {
		wg.Add(1)
		go func(t *RotationTarget) {
			defer wg.Done()

			// Simulate target update
			// In real implementation, this would push the secret to the target
			t.UpdatedAt = time.Now()
			t.Status = TargetStatusUpdated

			mu.Lock()
			if firstErr == nil {
				// Update progress
				updated := 0
				for _, tgt := range targets {
					if tgt.Status == TargetStatusUpdated {
						updated++
					}
				}
				rotation.MarkBatchProgress(updated, 0, fmt.Sprintf("Updated %d/%d targets", updated, len(targets)))
			}
			mu.Unlock()
		}(target)
	}

	wg.Wait()
	return firstErr
}

// Verify verifies all targets are healthy.
func (s *BlueGreenStrategy) Verify(ctx context.Context, rotation *ManagedRotation, targets []*RotationTarget) error {
	config := rotation.Config
	if config.HealthCheck == nil {
		// No health check configured, mark all as verified
		for _, t := range targets {
			if t.Status == TargetStatusUpdated {
				t.Status = TargetStatusVerified
			}
		}
		return nil
	}

	var wg sync.WaitGroup
	var mu sync.Mutex
	var errors []error

	for _, target := range targets {
		if target.Status != TargetStatusUpdated {
			continue
		}

		wg.Add(1)
		go func(t *RotationTarget) {
			defer wg.Done()

			// Perform health check with retries
			healthy := false
			var lastErr error

			for attempt := 0; attempt <= config.HealthCheck.Retries; attempt++ {
				// Simulate health check
				// In real implementation, this would make HTTP/TCP/exec calls
				healthy = true
				lastErr = nil

				if healthy {
					break
				}

				if attempt < config.HealthCheck.Retries {
					time.Sleep(config.HealthCheck.Interval)
				}
			}

			rotation.MarkHealthCheckResult(t.ID, healthy, lastErr)

			mu.Lock()
			if healthy {
				t.Status = TargetStatusVerified
			} else {
				t.Status = TargetStatusFailed
				t.Error = lastErr.Error()
				errors = append(errors, fmt.Errorf("target %s health check failed: %w", t.ID, lastErr))
			}
			mu.Unlock()
		}(target)
	}

	wg.Wait()

	if len(errors) > 0 {
		return fmt.Errorf("%d targets failed verification", len(errors))
	}
	return nil
}

// Rollback reverts all targets to the previous version.
func (s *BlueGreenStrategy) Rollback(ctx context.Context, rotation *ManagedRotation, targets []*RotationTarget) error {
	var wg sync.WaitGroup
	succeeded := 0
	failed := 0
	var mu sync.Mutex

	for _, target := range targets {
		if target.Status != TargetStatusUpdated && target.Status != TargetStatusVerified && target.Status != TargetStatusFailed {
			continue
		}

		wg.Add(1)
		go func(t *RotationTarget) {
			defer wg.Done()

			// Simulate rollback
			// In real implementation, this would push the old secret back
			t.Status = TargetStatusRolled

			mu.Lock()
			succeeded++
			mu.Unlock()
		}(target)
	}

	wg.Wait()

	if rotation.callbacks != nil && rotation.callbacks.OnRollbackComplete != nil {
		rotation.callbacks.OnRollbackComplete(rotation, succeeded, failed)
	}

	return nil
}

// RollingStrategy implements the rolling rotation strategy.
// Targets are updated in batches with delays between batches.
type RollingStrategy struct{}

// Name returns the strategy name.
func (s *RollingStrategy) Name() RotationStrategy {
	return RotationStrategyRolling
}

// BatchSize returns the configured batch size.
func (s *RollingStrategy) BatchSize(config *RotationConfig, totalTargets int) int {
	if config.BatchSize <= 0 {
		// Default to 10% of targets, minimum 1
		size := totalTargets / 10
		if size < 1 {
			return 1
		}
		return size
	}
	return config.BatchSize
}

// BatchDelay returns the configured batch delay.
func (s *RollingStrategy) BatchDelay(config *RotationConfig) time.Duration {
	if config.BatchDelay <= 0 {
		return 10 * time.Second // Default delay
	}
	return config.BatchDelay
}

// Execute updates a batch of targets.
func (s *RollingStrategy) Execute(ctx context.Context, rotation *ManagedRotation, targets []*RotationTarget) error {
	for _, target := range targets {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		target.Status = TargetStatusUpdating

		// Simulate target update
		// In real implementation, this would push the secret to the target
		target.UpdatedAt = time.Now()
		target.Status = TargetStatusUpdated

		// Update progress
		mr := rotation
		mr.mu.RLock()
		updated := mr.Progress.UpdatedTargets
		failed := mr.Progress.FailedTargets
		total := mr.Progress.TotalTargets
		mr.mu.RUnlock()

		// Count this batch's progress
		batchUpdated := 0
		for _, t := range targets {
			if t.Status == TargetStatusUpdated {
				batchUpdated++
			}
		}

		rotation.MarkBatchProgress(
			updated+batchUpdated,
			failed,
			fmt.Sprintf("Updated %d/%d targets", updated+batchUpdated, total),
		)
	}

	return nil
}

// Verify verifies a batch of targets.
func (s *RollingStrategy) Verify(ctx context.Context, rotation *ManagedRotation, targets []*RotationTarget) error {
	// Delegate to blue-green verify since the logic is the same
	bg := &BlueGreenStrategy{}
	return bg.Verify(ctx, rotation, targets)
}

// Rollback reverts a batch of targets.
func (s *RollingStrategy) Rollback(ctx context.Context, rotation *ManagedRotation, targets []*RotationTarget) error {
	// Delegate to blue-green rollback since the logic is the same
	bg := &BlueGreenStrategy{}
	return bg.Rollback(ctx, rotation, targets)
}

// Ensure strategies implement the interface.
var (
	_ RotationStrategyExecutor = (*BlueGreenStrategy)(nil)
	_ RotationStrategyExecutor = (*RollingStrategy)(nil)
)
