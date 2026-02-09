package bootstrap

import (
	"context"
	"fmt"
	"time"

	"github.com/shawnbutts/keystone-core/pkg/statemachine"
)

// Event represents events that can occur during bootstrap
type Event string

// Event constants define events that can occur during bootstrap.
const (
	BootstrapEventStart            Event = "start"
	BootstrapEventValidated        Event = "validated"
	BootstrapEventDepsInstalled    Event = "deps_installed"
	BootstrapEventServerInstalled  Event = "server_installed"
	BootstrapEventServerConfigured Event = "server_configured"
	BootstrapEventServerStarted    Event = "server_started"
	BootstrapEventClusterFormed    Event = "cluster_formed"
	BootstrapEventAgentsInstalled  Event = "agents_installed"
	BootstrapEventStatesApplied    Event = "states_applied"
	BootstrapEventVerified         Event = "verified"
	BootstrapEventHandoffComplete  Event = "handoff_complete"
	BootstrapEventFail             Event = "fail"
	BootstrapEventSkipAgents       Event = "skip_agents"
	BootstrapEventSkipStates       Event = "skip_states"
	BootstrapEventSkipVerification Event = "skip_verification"
	BootstrapEventRollback         Event = "rollback"
)

// Callbacks defines callbacks for bootstrap state transitions
type Callbacks struct {
	OnPhaseStarted   func(phase Phase)
	OnPhaseCompleted func(phase Phase)
	OnFailed         func(phase Phase, err error)
	OnProgress       func(phase Phase, progress int, message string)
	OnCleanupStarted func()
	OnComplete       func(result *Result)
}

// ManagedBootstrap wraps a bootstrap operation with explicit state machine
type ManagedBootstrap struct {
	Status    *Status
	Result    *Result
	machine   *statemachine.Machine[Phase, Event]
	callbacks *Callbacks

	mode        Mode
	clusterName string
	progress    int
	message     string
	lastError   error
	startTime   time.Time
	endTime     time.Time
	phaseStart  time.Time
}

// NewManagedBootstrap creates a new managed bootstrap operation
func NewManagedBootstrap(mode Mode, clusterName string, callbacks *Callbacks) *ManagedBootstrap {
	mb := &ManagedBootstrap{
		Status: &Status{
			Phase:   PhaseInitializing,
			Message: "Initializing bootstrap",
		},
		Result: &Result{
			Success: false,
		},
		callbacks:   callbacks,
		mode:        mode,
		clusterName: clusterName,
		startTime:   time.Now(),
		phaseStart:  time.Now(),
	}

	mb.machine = mb.buildStateMachine()
	return mb
}

// buildStateMachine creates the bootstrap state machine
func (mb *ManagedBootstrap) buildStateMachine() *statemachine.Machine[Phase, Event] {
	builder := statemachine.New[Phase, Event](PhaseInitializing).
		WithHistory(20)

	// Initializing -> Validating
	builder.AddTransition(PhaseInitializing, BootstrapEventStart, PhaseValidating).
		OnEnter(PhaseValidating, func(_ context.Context, _ Phase, _ Phase) {
			mb.onPhaseEnter(PhaseValidating)
		})

	// Validating -> InstallingDeps
	builder.AddTransition(PhaseValidating, BootstrapEventValidated, PhaseInstallingDeps).
		OnEnter(PhaseInstallingDeps, func(_ context.Context, _ Phase, _ Phase) {
			mb.onPhaseEnter(PhaseInstallingDeps)
		})

	// InstallingDeps -> InstallingServer
	builder.AddTransition(PhaseInstallingDeps, BootstrapEventDepsInstalled, PhaseInstallingServer).
		OnEnter(PhaseInstallingServer, func(_ context.Context, _ Phase, _ Phase) {
			mb.onPhaseEnter(PhaseInstallingServer)
		})

	// InstallingServer -> ConfiguringServer
	builder.AddTransition(PhaseInstallingServer, BootstrapEventServerInstalled, PhaseConfiguringServer).
		OnEnter(PhaseConfiguringServer, func(_ context.Context, _ Phase, _ Phase) {
			mb.onPhaseEnter(PhaseConfiguringServer)
		})

	// ConfiguringServer -> StartingServer
	builder.AddTransition(PhaseConfiguringServer, BootstrapEventServerConfigured, PhaseStartingServer).
		OnEnter(PhaseStartingServer, func(_ context.Context, _ Phase, _ Phase) {
			mb.onPhaseEnter(PhaseStartingServer)
		})

	// StartingServer -> FormingCluster
	builder.AddTransition(PhaseStartingServer, BootstrapEventServerStarted, PhaseFormingCluster).
		OnEnter(PhaseFormingCluster, func(_ context.Context, _ Phase, _ Phase) {
			mb.onPhaseEnter(PhaseFormingCluster)
		})

	// FormingCluster -> InstallingAgents
	builder.AddTransition(PhaseFormingCluster, BootstrapEventClusterFormed, PhaseInstallingAgents).
		OnEnter(PhaseInstallingAgents, func(_ context.Context, _ Phase, _ Phase) {
			mb.onPhaseEnter(PhaseInstallingAgents)
		})

	// FormingCluster -> ApplyingStates (skip agents)
	builder.AddTransition(PhaseFormingCluster, BootstrapEventSkipAgents, PhaseApplyingStates).
		OnEnter(PhaseApplyingStates, func(_ context.Context, _ Phase, _ Phase) {
			mb.onPhaseEnter(PhaseApplyingStates)
		})

	// InstallingAgents -> ApplyingStates
	builder.AddTransition(PhaseInstallingAgents, BootstrapEventAgentsInstalled, PhaseApplyingStates)

	// ApplyingStates -> Verifying
	builder.AddTransition(PhaseApplyingStates, BootstrapEventStatesApplied, PhaseVerifying).
		OnEnter(PhaseVerifying, func(_ context.Context, _ Phase, _ Phase) {
			mb.onPhaseEnter(PhaseVerifying)
		})

	// ApplyingStates -> Handoff (skip verification)
	builder.AddTransition(PhaseApplyingStates, BootstrapEventSkipVerification, PhaseHandoff).
		OnEnter(PhaseHandoff, func(_ context.Context, _ Phase, _ Phase) {
			mb.onPhaseEnter(PhaseHandoff)
		})

	// Verifying -> Handoff
	builder.AddTransition(PhaseVerifying, BootstrapEventVerified, PhaseHandoff)

	// Handoff -> Complete
	builder.AddTransition(PhaseHandoff, BootstrapEventHandoffComplete, PhaseComplete).
		OnEnter(PhaseComplete, func(_ context.Context, _ Phase, _ Phase) {
			mb.onComplete()
		})

	// Fail transitions from any non-terminal state
	failablePhases := []Phase{
		PhaseInitializing, PhaseValidating, PhaseInstallingDeps, PhaseInstallingServer,
		PhaseConfiguringServer, PhaseStartingServer, PhaseFormingCluster,
		PhaseInstallingAgents, PhaseApplyingStates, PhaseVerifying, PhaseHandoff,
	}

	for _, phase := range failablePhases {
		builder.AddTransition(phase, BootstrapEventFail, PhaseFailed)
	}

	builder.OnEnter(PhaseFailed, func(_ context.Context, _ Phase, _ Phase) {
		mb.onFailed()
	})

	return builder.MustBuild()
}

// onPhaseEnter is called when entering a new phase
func (mb *ManagedBootstrap) onPhaseEnter(phase Phase) {
	mb.phaseStart = time.Now()
	mb.Status.Phase = phase
	mb.Status.CurrentStep = string(phase)

	// Update progress based on phase
	mb.progress = phaseProgress(phase)
	mb.Status.Progress = mb.progress

	if mb.callbacks != nil && mb.callbacks.OnPhaseStarted != nil {
		mb.callbacks.OnPhaseStarted(phase)
	}
}

// onComplete is called when bootstrap completes
func (mb *ManagedBootstrap) onComplete() {
	mb.endTime = time.Now()
	mb.progress = 100
	mb.Status.Phase = PhaseComplete
	mb.Status.Progress = 100
	mb.Status.Message = "Bootstrap complete"
	mb.Result.Success = true
	mb.Result.Duration = mb.Duration()

	if mb.callbacks != nil && mb.callbacks.OnComplete != nil {
		mb.callbacks.OnComplete(mb.Result)
	}
}

// onFailed is called when bootstrap fails
func (mb *ManagedBootstrap) onFailed() {
	mb.endTime = time.Now()
	mb.Status.Phase = PhaseFailed
	mb.Result.Success = false
	mb.Result.Duration = mb.Duration()

	if mb.lastError != nil {
		mb.Status.Error = mb.lastError.Error()
		mb.Result.Error = mb.lastError.Error()
	}

	if mb.callbacks != nil && mb.callbacks.OnFailed != nil {
		mb.callbacks.OnFailed(mb.Status.Phase, mb.lastError)
	}
}

// phaseProgress returns the progress percentage for a phase
func phaseProgress(phase Phase) int {
	switch phase {
	case PhaseInitializing:
		return 0
	case PhaseValidating:
		return 10
	case PhaseInstallingDeps:
		return 20
	case PhaseInstallingServer:
		return 30
	case PhaseConfiguringServer:
		return 40
	case PhaseStartingServer:
		return 50
	case PhaseFormingCluster:
		return 60
	case PhaseInstallingAgents:
		return 70
	case PhaseApplyingStates:
		return 80
	case PhaseVerifying:
		return 85
	case PhaseHandoff:
		return 90
	case PhaseComplete:
		return 100
	case PhaseFailed:
		return 0
	default:
		return 0
	}
}

// Start begins the bootstrap process
func (mb *ManagedBootstrap) Start() error {
	mb.Status.StartTime = time.Now()
	mb.startTime = time.Now()
	return mb.machine.Fire(BootstrapEventStart)
}

// MarkValidated marks validation as complete
func (mb *ManagedBootstrap) MarkValidated() error {
	if mb.callbacks != nil && mb.callbacks.OnPhaseCompleted != nil {
		mb.callbacks.OnPhaseCompleted(PhaseValidating)
	}
	return mb.machine.Fire(BootstrapEventValidated)
}

// MarkDepsInstalled marks dependency installation as complete
func (mb *ManagedBootstrap) MarkDepsInstalled() error {
	if mb.callbacks != nil && mb.callbacks.OnPhaseCompleted != nil {
		mb.callbacks.OnPhaseCompleted(PhaseInstallingDeps)
	}
	return mb.machine.Fire(BootstrapEventDepsInstalled)
}

// MarkServerInstalled marks server installation as complete
func (mb *ManagedBootstrap) MarkServerInstalled() error {
	if mb.callbacks != nil && mb.callbacks.OnPhaseCompleted != nil {
		mb.callbacks.OnPhaseCompleted(PhaseInstallingServer)
	}
	return mb.machine.Fire(BootstrapEventServerInstalled)
}

// MarkServerConfigured marks server configuration as complete
func (mb *ManagedBootstrap) MarkServerConfigured() error {
	if mb.callbacks != nil && mb.callbacks.OnPhaseCompleted != nil {
		mb.callbacks.OnPhaseCompleted(PhaseConfiguringServer)
	}
	return mb.machine.Fire(BootstrapEventServerConfigured)
}

// MarkServerStarted marks server start as complete
func (mb *ManagedBootstrap) MarkServerStarted() error {
	if mb.callbacks != nil && mb.callbacks.OnPhaseCompleted != nil {
		mb.callbacks.OnPhaseCompleted(PhaseStartingServer)
	}
	return mb.machine.Fire(BootstrapEventServerStarted)
}

// MarkClusterFormed marks cluster formation as complete
func (mb *ManagedBootstrap) MarkClusterFormed() error {
	if mb.callbacks != nil && mb.callbacks.OnPhaseCompleted != nil {
		mb.callbacks.OnPhaseCompleted(PhaseFormingCluster)
	}
	return mb.machine.Fire(BootstrapEventClusterFormed)
}

// SkipAgents skips agent installation
func (mb *ManagedBootstrap) SkipAgents() error {
	if mb.callbacks != nil && mb.callbacks.OnPhaseCompleted != nil {
		mb.callbacks.OnPhaseCompleted(PhaseFormingCluster)
	}
	return mb.machine.Fire(BootstrapEventSkipAgents)
}

// MarkAgentsInstalled marks agent installation as complete
func (mb *ManagedBootstrap) MarkAgentsInstalled() error {
	if mb.callbacks != nil && mb.callbacks.OnPhaseCompleted != nil {
		mb.callbacks.OnPhaseCompleted(PhaseInstallingAgents)
	}
	return mb.machine.Fire(BootstrapEventAgentsInstalled)
}

// MarkStatesApplied marks state application as complete
func (mb *ManagedBootstrap) MarkStatesApplied() error {
	if mb.callbacks != nil && mb.callbacks.OnPhaseCompleted != nil {
		mb.callbacks.OnPhaseCompleted(PhaseApplyingStates)
	}
	return mb.machine.Fire(BootstrapEventStatesApplied)
}

// SkipVerification skips the verification phase
func (mb *ManagedBootstrap) SkipVerification() error {
	if mb.callbacks != nil && mb.callbacks.OnPhaseCompleted != nil {
		mb.callbacks.OnPhaseCompleted(PhaseApplyingStates)
	}
	return mb.machine.Fire(BootstrapEventSkipVerification)
}

// MarkVerified marks verification as complete
func (mb *ManagedBootstrap) MarkVerified() error {
	if mb.callbacks != nil && mb.callbacks.OnPhaseCompleted != nil {
		mb.callbacks.OnPhaseCompleted(PhaseVerifying)
	}
	return mb.machine.Fire(BootstrapEventVerified)
}

// MarkHandoffComplete marks handoff as complete
func (mb *ManagedBootstrap) MarkHandoffComplete() error {
	if mb.callbacks != nil && mb.callbacks.OnPhaseCompleted != nil {
		mb.callbacks.OnPhaseCompleted(PhaseHandoff)
	}
	return mb.machine.Fire(BootstrapEventHandoffComplete)
}

// Fail marks the bootstrap as failed with an error
func (mb *ManagedBootstrap) Fail(err error) error {
	mb.lastError = err
	mb.Status.Error = err.Error()
	return mb.machine.Fire(BootstrapEventFail)
}

// SetResult sets the bootstrap result
func (mb *ManagedBootstrap) SetResult(result *Result) {
	mb.Result = result
}

// UpdateProgress updates progress within the current phase
func (mb *ManagedBootstrap) UpdateProgress(progress int, message string) {
	mb.progress = progress
	mb.message = message
	mb.Status.Progress = progress
	mb.Status.Message = message

	if mb.callbacks != nil && mb.callbacks.OnProgress != nil {
		mb.callbacks.OnProgress(mb.Phase(), progress, message)
	}
}

// Phase returns the current bootstrap phase
func (mb *ManagedBootstrap) Phase() Phase {
	return mb.machine.State()
}

// IsComplete returns true if bootstrap completed successfully
func (mb *ManagedBootstrap) IsComplete() bool {
	return mb.Phase() == PhaseComplete
}

// IsFailed returns true if bootstrap failed
func (mb *ManagedBootstrap) IsFailed() bool {
	return mb.Phase() == PhaseFailed
}

// IsTerminal returns true if bootstrap is in a terminal state
func (mb *ManagedBootstrap) IsTerminal() bool {
	return mb.IsComplete() || mb.IsFailed()
}

// IsRunning returns true if bootstrap is in progress
func (mb *ManagedBootstrap) IsRunning() bool {
	phase := mb.Phase()
	return phase != PhaseInitializing && phase != PhaseComplete && phase != PhaseFailed
}

// Duration returns the elapsed time
func (mb *ManagedBootstrap) Duration() time.Duration {
	if !mb.endTime.IsZero() {
		return mb.endTime.Sub(mb.startTime)
	}
	return time.Since(mb.startTime)
}

// PhaseDuration returns how long the current phase has been running
func (mb *ManagedBootstrap) PhaseDuration() time.Duration {
	return time.Since(mb.phaseStart)
}

// Progress returns the current progress percentage
func (mb *ManagedBootstrap) Progress() int {
	return mb.progress
}

// Error returns the last error if any
func (mb *ManagedBootstrap) Error() error {
	return mb.lastError
}

// History returns the state transition history
func (mb *ManagedBootstrap) History() *statemachine.History[Phase, Event] {
	return mb.machine.History()
}

// AvailableEvents returns events valid for the current state
func (mb *ManagedBootstrap) AvailableEvents() []Event {
	return mb.machine.AvailableEvents()
}

// CanTransition checks if an event is valid for current state
func (mb *ManagedBootstrap) CanTransition(event Event) bool {
	return mb.machine.CanFire(event)
}

// PhaseToString converts a Phase to a display string
func PhaseToString(phase Phase) string {
	switch phase {
	case PhaseInitializing:
		return "Initializing"
	case PhaseValidating:
		return "Validating"
	case PhaseInstallingDeps:
		return "Installing Dependencies"
	case PhaseInstallingServer:
		return "Installing Server"
	case PhaseConfiguringServer:
		return "Configuring Server"
	case PhaseStartingServer:
		return "Starting Server"
	case PhaseFormingCluster:
		return "Forming Cluster"
	case PhaseInstallingAgents:
		return "Installing Agents"
	case PhaseApplyingStates:
		return "Applying States"
	case PhaseVerifying:
		return "Verifying"
	case PhaseHandoff:
		return "Handoff"
	case PhaseComplete:
		return "Complete"
	case PhaseFailed:
		return "Failed"
	default:
		return string(phase)
	}
}

// StateDiagram returns a Mermaid state diagram of the bootstrap flow
func (mb *ManagedBootstrap) StateDiagram() string {
	return `stateDiagram-v2
    [*] --> Initializing
    Initializing --> Validating: start
    Validating --> InstallingDeps: validated
    InstallingDeps --> InstallingServer: deps_installed
    InstallingServer --> ConfiguringServer: server_installed
    ConfiguringServer --> StartingServer: server_configured
    StartingServer --> FormingCluster: server_started
    FormingCluster --> InstallingAgents: cluster_formed
    FormingCluster --> ApplyingStates: skip_agents
    InstallingAgents --> ApplyingStates: agents_installed
    ApplyingStates --> Verifying: states_applied
    ApplyingStates --> Handoff: skip_verification
    Verifying --> Handoff: verified
    Handoff --> Complete: handoff_complete
    Complete --> [*]

    Initializing --> Failed: fail
    Validating --> Failed: fail
    InstallingDeps --> Failed: fail
    InstallingServer --> Failed: fail
    ConfiguringServer --> Failed: fail
    StartingServer --> Failed: fail
    FormingCluster --> Failed: fail
    InstallingAgents --> Failed: fail
    ApplyingStates --> Failed: fail
    Verifying --> Failed: fail
    Handoff --> Failed: fail
    Failed --> [*]`
}

// RunFullBootstrapWorkflow executes a complete bootstrap workflow with state machine management
func RunFullBootstrapWorkflow(mb *ManagedBootstrap, opts Options, doPhase func(Phase) error) error {
	// Start
	if err := mb.Start(); err != nil {
		return fmt.Errorf("failed to start: %w", err)
	}

	// Validating
	if err := doPhase(PhaseValidating); err != nil {
		_ = mb.Fail(err) //nolint:errcheck // best-effort state transition
		return err
	}
	if err := mb.MarkValidated(); err != nil {
		return err
	}

	// Installing dependencies
	if err := doPhase(PhaseInstallingDeps); err != nil {
		_ = mb.Fail(err) //nolint:errcheck // best-effort state transition
		return err
	}
	if err := mb.MarkDepsInstalled(); err != nil {
		return err
	}

	// Installing server
	if err := doPhase(PhaseInstallingServer); err != nil {
		_ = mb.Fail(err) //nolint:errcheck // best-effort state transition
		return err
	}
	if err := mb.MarkServerInstalled(); err != nil {
		return err
	}

	// Configuring server
	if err := doPhase(PhaseConfiguringServer); err != nil {
		_ = mb.Fail(err) //nolint:errcheck // best-effort state transition
		return err
	}
	if err := mb.MarkServerConfigured(); err != nil {
		return err
	}

	// Starting server
	if err := doPhase(PhaseStartingServer); err != nil {
		_ = mb.Fail(err) //nolint:errcheck // best-effort state transition
		return err
	}
	if err := mb.MarkServerStarted(); err != nil {
		return err
	}

	// Forming cluster
	if err := doPhase(PhaseFormingCluster); err != nil {
		_ = mb.Fail(err) //nolint:errcheck // best-effort state transition
		return err
	}
	if err := mb.MarkClusterFormed(); err != nil {
		return err
	}

	// Installing agents
	if err := doPhase(PhaseInstallingAgents); err != nil {
		_ = mb.Fail(err) //nolint:errcheck // best-effort state transition
		return err
	}
	if err := mb.MarkAgentsInstalled(); err != nil {
		return err
	}

	// Applying states
	if err := doPhase(PhaseApplyingStates); err != nil {
		_ = mb.Fail(err) //nolint:errcheck // best-effort state transition
		return err
	}

	// Verifying (can be skipped)
	if !opts.SkipVerification {
		if err := mb.MarkStatesApplied(); err != nil {
			return err
		}
		if err := doPhase(PhaseVerifying); err != nil {
			_ = mb.Fail(err) //nolint:errcheck // best-effort state transition
			return err
		}
		if err := mb.MarkVerified(); err != nil {
			return err
		}
	} else {
		// Skip verification - go directly from ApplyingStates to Handoff
		if err := mb.SkipVerification(); err != nil {
			return err
		}
	}

	// Handoff
	if err := doPhase(PhaseHandoff); err != nil {
		_ = mb.Fail(err) //nolint:errcheck // best-effort state transition
		return err
	}
	return mb.MarkHandoffComplete()
}
