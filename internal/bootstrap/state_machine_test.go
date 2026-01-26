package bootstrap

import (
	"errors"
	"testing"
	"time"

	"github.com/shawnbutts/keystone-core/internal/testing/helpers"
	"github.com/shawnbutts/keystone-core/pkg/statemachine"
)

func TestManagedBootstrap_InitialState(t *testing.T) {
	mb := NewManagedBootstrap(BootstrapModeSeed, "test-cluster", nil)

	if mb.Phase() != PhaseInitializing {
		t.Errorf("expected initializing phase, got %v", mb.Phase())
	}
	if mb.IsRunning() {
		t.Error("expected IsRunning() to be false for initializing")
	}
	if mb.IsComplete() {
		t.Error("expected IsComplete() to be false")
	}
	if mb.IsFailed() {
		t.Error("expected IsFailed() to be false")
	}
	if mb.IsTerminal() {
		t.Error("expected IsTerminal() to be false")
	}
	if mb.Progress() != 0 {
		t.Errorf("expected progress 0, got %d", mb.Progress())
	}
}

func TestManagedBootstrap_StartTransition(t *testing.T) {
	mb := NewManagedBootstrap(BootstrapModeSeed, "test-cluster", nil)

	if err := mb.Start(); err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	if mb.Phase() != PhaseValidating {
		t.Errorf("expected validating phase, got %v", mb.Phase())
	}
	if !mb.IsRunning() {
		t.Error("expected IsRunning() to be true")
	}
	if mb.Progress() != 10 {
		t.Errorf("expected progress 10, got %d", mb.Progress())
	}
}

func TestManagedBootstrap_FullWorkflow(t *testing.T) {
	mb := NewManagedBootstrap(BootstrapModeSeed, "test-cluster", nil)

	steps := []struct {
		action        func() error
		expectedPhase BootstrapPhase
		expectedProg  int
	}{
		{mb.Start, PhaseValidating, 10},
		{mb.MarkValidated, PhaseInstallingDeps, 20},
		{mb.MarkDepsInstalled, PhaseInstallingServer, 30},
		{mb.MarkServerInstalled, PhaseConfiguringServer, 40},
		{mb.MarkServerConfigured, PhaseStartingServer, 50},
		{mb.MarkServerStarted, PhaseFormingCluster, 60},
		{mb.MarkClusterFormed, PhaseInstallingAgents, 70},
		{mb.MarkAgentsInstalled, PhaseApplyingStates, 80},
		{mb.MarkStatesApplied, PhaseVerifying, 85},
		{mb.MarkVerified, PhaseHandoff, 90},
		{mb.MarkHandoffComplete, PhaseComplete, 100},
	}

	for i, step := range steps {
		if err := step.action(); err != nil {
			t.Errorf("step %d: unexpected error: %v", i, err)
		}
		if mb.Phase() != step.expectedPhase {
			t.Errorf("step %d: expected phase %v, got %v", i, step.expectedPhase, mb.Phase())
		}
		if mb.Progress() != step.expectedProg {
			t.Errorf("step %d: expected progress %d, got %d", i, step.expectedProg, mb.Progress())
		}
	}

	if !mb.IsComplete() {
		t.Error("expected IsComplete() to be true")
	}
	if !mb.IsTerminal() {
		t.Error("expected IsTerminal() to be true")
	}
	if mb.Result.Success != true {
		t.Error("expected Result.Success to be true")
	}
}

func TestManagedBootstrap_SkipAgents(t *testing.T) {
	mb := NewManagedBootstrap(BootstrapModeSeed, "test-cluster", nil)

	mb.Start()
	mb.MarkValidated()
	mb.MarkDepsInstalled()
	mb.MarkServerInstalled()
	mb.MarkServerConfigured()
	mb.MarkServerStarted()

	// Skip agents instead of installing them
	if err := mb.SkipAgents(); err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	// Should go directly to ApplyingStates
	if mb.Phase() != PhaseApplyingStates {
		t.Errorf("expected applying_states phase, got %v", mb.Phase())
	}
}

func TestManagedBootstrap_SkipVerification(t *testing.T) {
	mb := NewManagedBootstrap(BootstrapModeSeed, "test-cluster", nil)

	mb.Start()
	mb.MarkValidated()
	mb.MarkDepsInstalled()
	mb.MarkServerInstalled()
	mb.MarkServerConfigured()
	mb.MarkServerStarted()
	mb.SkipAgents()

	// Skip verification
	if err := mb.SkipVerification(); err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	// Should go directly to Handoff
	if mb.Phase() != PhaseHandoff {
		t.Errorf("expected handoff phase, got %v", mb.Phase())
	}
}

func TestManagedBootstrap_FailFromEachPhase(t *testing.T) {
	phases := []struct {
		name  string
		setup func(*ManagedBootstrap)
	}{
		{"fail from initializing", func(mb *ManagedBootstrap) {}},
		{"fail from validating", func(mb *ManagedBootstrap) { mb.Start() }},
		{"fail from installing_deps", func(mb *ManagedBootstrap) { mb.Start(); mb.MarkValidated() }},
		{"fail from installing_server", func(mb *ManagedBootstrap) {
			mb.Start()
			mb.MarkValidated()
			mb.MarkDepsInstalled()
		}},
		{"fail from configuring_server", func(mb *ManagedBootstrap) {
			mb.Start()
			mb.MarkValidated()
			mb.MarkDepsInstalled()
			mb.MarkServerInstalled()
		}},
		{"fail from starting_server", func(mb *ManagedBootstrap) {
			mb.Start()
			mb.MarkValidated()
			mb.MarkDepsInstalled()
			mb.MarkServerInstalled()
			mb.MarkServerConfigured()
		}},
		{"fail from forming_cluster", func(mb *ManagedBootstrap) {
			mb.Start()
			mb.MarkValidated()
			mb.MarkDepsInstalled()
			mb.MarkServerInstalled()
			mb.MarkServerConfigured()
			mb.MarkServerStarted()
		}},
		{"fail from installing_agents", func(mb *ManagedBootstrap) {
			mb.Start()
			mb.MarkValidated()
			mb.MarkDepsInstalled()
			mb.MarkServerInstalled()
			mb.MarkServerConfigured()
			mb.MarkServerStarted()
			mb.MarkClusterFormed()
		}},
		{"fail from applying_states", func(mb *ManagedBootstrap) {
			mb.Start()
			mb.MarkValidated()
			mb.MarkDepsInstalled()
			mb.MarkServerInstalled()
			mb.MarkServerConfigured()
			mb.MarkServerStarted()
			mb.MarkClusterFormed()
			mb.MarkAgentsInstalled()
		}},
		{"fail from verifying", func(mb *ManagedBootstrap) {
			mb.Start()
			mb.MarkValidated()
			mb.MarkDepsInstalled()
			mb.MarkServerInstalled()
			mb.MarkServerConfigured()
			mb.MarkServerStarted()
			mb.MarkClusterFormed()
			mb.MarkAgentsInstalled()
			mb.MarkStatesApplied()
		}},
		{"fail from handoff", func(mb *ManagedBootstrap) {
			mb.Start()
			mb.MarkValidated()
			mb.MarkDepsInstalled()
			mb.MarkServerInstalled()
			mb.MarkServerConfigured()
			mb.MarkServerStarted()
			mb.MarkClusterFormed()
			mb.MarkAgentsInstalled()
			mb.MarkStatesApplied()
			mb.MarkVerified()
		}},
	}

	for _, tt := range phases {
		t.Run(tt.name, func(t *testing.T) {
			mb := NewManagedBootstrap(BootstrapModeSeed, "test-cluster", nil)
			tt.setup(mb)

			testErr := errors.New("test error")
			if err := mb.Fail(testErr); err != nil {
				t.Errorf("unexpected error: %v", err)
			}

			if !mb.IsFailed() {
				t.Error("expected IsFailed() to be true")
			}
			if !mb.IsTerminal() {
				t.Error("expected IsTerminal() to be true")
			}
			if mb.Error() != testErr {
				t.Errorf("expected error to be %v, got %v", testErr, mb.Error())
			}
			if mb.Result.Success {
				t.Error("expected Result.Success to be false")
			}
		})
	}
}

func TestManagedBootstrap_InvalidTransitions(t *testing.T) {
	mb := NewManagedBootstrap(BootstrapModeSeed, "test-cluster", nil)

	// Cannot mark validated before starting
	err := mb.MarkValidated()
	if err == nil {
		t.Error("expected error for invalid transition")
	}
	if !errors.Is(err, statemachine.ErrInvalidTransition) {
		t.Errorf("expected ErrInvalidTransition, got %v", err)
	}

	// Start properly
	mb.Start()

	// Cannot skip to server installed
	err = mb.MarkServerInstalled()
	if err == nil {
		t.Error("expected error for invalid transition")
	}
}

func TestManagedBootstrap_CannotTransitionFromTerminal(t *testing.T) {
	// Test from Complete
	mb1 := NewManagedBootstrap(BootstrapModeSeed, "test-cluster", nil)
	mb1.Start()
	mb1.MarkValidated()
	mb1.MarkDepsInstalled()
	mb1.MarkServerInstalled()
	mb1.MarkServerConfigured()
	mb1.MarkServerStarted()
	mb1.SkipAgents()
	mb1.SkipVerification()
	mb1.MarkHandoffComplete()

	err := mb1.Start()
	if err == nil {
		t.Error("expected error when transitioning from complete")
	}

	// Test from Failed
	mb2 := NewManagedBootstrap(BootstrapModeSeed, "test-cluster", nil)
	mb2.Fail(errors.New("test"))

	err = mb2.Start()
	if err == nil {
		t.Error("expected error when transitioning from failed")
	}
}

func TestManagedBootstrap_Callbacks(t *testing.T) {
	var phaseStarted, phaseCompleted []BootstrapPhase
	var failedPhase BootstrapPhase
	var failedErr error
	var completeCalled bool

	callbacks := &BootstrapCallbacks{
		OnPhaseStarted: func(phase BootstrapPhase) {
			phaseStarted = append(phaseStarted, phase)
		},
		OnPhaseCompleted: func(phase BootstrapPhase) {
			phaseCompleted = append(phaseCompleted, phase)
		},
		OnFailed: func(phase BootstrapPhase, err error) {
			failedPhase = phase
			failedErr = err
		},
		OnComplete: func(result *BootstrapResult) {
			completeCalled = true
		},
	}

	mb := NewManagedBootstrap(BootstrapModeSeed, "test-cluster", callbacks)

	// Run through workflow
	mb.Start()
	mb.MarkValidated()
	mb.MarkDepsInstalled()
	mb.MarkServerInstalled()
	mb.MarkServerConfigured()
	mb.MarkServerStarted()
	mb.SkipAgents()
	mb.SkipVerification()
	mb.MarkHandoffComplete()

	if len(phaseStarted) == 0 {
		t.Error("expected OnPhaseStarted to be called")
	}
	if len(phaseCompleted) == 0 {
		t.Error("expected OnPhaseCompleted to be called")
	}
	if !completeCalled {
		t.Error("expected OnComplete to be called")
	}

	// Test failure callback
	mb2 := NewManagedBootstrap(BootstrapModeSeed, "test-cluster", callbacks)
	mb2.Start()
	testErr := errors.New("validation failed")
	mb2.Fail(testErr)

	if failedPhase != PhaseFailed {
		t.Errorf("expected failed phase callback, got %v", failedPhase)
	}
	if failedErr != testErr {
		t.Errorf("expected error %v, got %v", testErr, failedErr)
	}
}

func TestManagedBootstrap_ProgressCallback(t *testing.T) {
	var progressUpdates []struct {
		phase    BootstrapPhase
		progress int
		message  string
	}

	callbacks := &BootstrapCallbacks{
		OnProgress: func(phase BootstrapPhase, progress int, message string) {
			progressUpdates = append(progressUpdates, struct {
				phase    BootstrapPhase
				progress int
				message  string
			}{phase, progress, message})
		},
	}

	mb := NewManagedBootstrap(BootstrapModeSeed, "test-cluster", callbacks)
	mb.Start()

	mb.UpdateProgress(15, "Loading configuration...")
	mb.UpdateProgress(18, "Checking dependencies...")

	if len(progressUpdates) != 2 {
		t.Errorf("expected 2 progress updates, got %d", len(progressUpdates))
	}
	if progressUpdates[0].progress != 15 {
		t.Errorf("expected progress 15, got %d", progressUpdates[0].progress)
	}
	if progressUpdates[1].message != "Checking dependencies..." {
		t.Errorf("unexpected message: %s", progressUpdates[1].message)
	}
}

func TestManagedBootstrap_Duration(t *testing.T) {
	mb := NewManagedBootstrap(BootstrapModeSeed, "test-cluster", nil)

	// Duration should be non-zero immediately
	if err := helpers.WaitForTimeout(100*time.Millisecond, 5*time.Millisecond, func() (bool, error) {
		return mb.Duration() > 0, nil
	}); err != nil {
		t.Fatalf("expected duration to start: %v", err)
	}
	if mb.Duration() == 0 {
		t.Error("expected non-zero duration")
	}

	// Complete the workflow
	mb.Start()
	if err := helpers.WaitForTimeout(100*time.Millisecond, 5*time.Millisecond, func() (bool, error) {
		return mb.Duration() > 0, nil
	}); err != nil {
		t.Fatalf("expected duration to advance: %v", err)
	}
	mb.MarkValidated()
	mb.MarkDepsInstalled()
	mb.MarkServerInstalled()
	mb.MarkServerConfigured()
	mb.MarkServerStarted()
	mb.SkipAgents()
	mb.SkipVerification()
	mb.MarkHandoffComplete()

	finalDuration := mb.Duration()
	if finalDuration == 0 {
		t.Error("expected non-zero final duration")
	}

	// Duration should be fixed after completion
	waitStart := time.Now()
	if err := helpers.WaitForTimeout(100*time.Millisecond, 5*time.Millisecond, func() (bool, error) {
		return time.Since(waitStart) >= 10*time.Millisecond, nil
	}); err != nil {
		t.Fatalf("expected time to advance: %v", err)
	}
	if mb.Duration() != finalDuration {
		t.Error("expected duration to be fixed after completion")
	}
}

func TestManagedBootstrap_PhaseDuration(t *testing.T) {
	mb := NewManagedBootstrap(BootstrapModeSeed, "test-cluster", nil)

	mb.Start()
	if err := helpers.WaitForTimeout(100*time.Millisecond, 5*time.Millisecond, func() (bool, error) {
		return mb.PhaseDuration() > 0, nil
	}); err != nil {
		t.Fatalf("expected phase duration to start: %v", err)
	}
	phaseDuration := mb.PhaseDuration()
	if phaseDuration == 0 {
		t.Error("expected non-zero phase duration")
	}

	// Move to next phase
	mb.MarkValidated()
	newPhaseDuration := mb.PhaseDuration()
	if newPhaseDuration >= phaseDuration {
		t.Error("expected phase duration to reset on new phase")
	}
}

func TestManagedBootstrap_History(t *testing.T) {
	mb := NewManagedBootstrap(BootstrapModeSeed, "test-cluster", nil)

	mb.Start()
	mb.MarkValidated()
	mb.MarkDepsInstalled()

	history := mb.History()
	if history == nil {
		t.Fatal("history should not be nil")
	}

	records := history.All()
	if len(records) != 3 {
		t.Errorf("expected 3 history records, got %d", len(records))
	}
}

func TestManagedBootstrap_AvailableEvents(t *testing.T) {
	mb := NewManagedBootstrap(BootstrapModeSeed, "test-cluster", nil)

	// From initializing, can only start or fail
	events := mb.AvailableEvents()
	if len(events) != 2 {
		t.Errorf("expected 2 available events from initializing, got %d", len(events))
	}

	mb.Start()

	// From validating, can validate or fail
	events = mb.AvailableEvents()
	if len(events) != 2 {
		t.Errorf("expected 2 available events from validating, got %d", len(events))
	}

	mb.MarkValidated()
	mb.MarkDepsInstalled()
	mb.MarkServerInstalled()
	mb.MarkServerConfigured()
	mb.MarkServerStarted()

	// From forming_cluster, can form cluster, skip agents, or fail
	events = mb.AvailableEvents()
	if len(events) != 3 {
		t.Errorf("expected 3 available events from forming_cluster, got %d", len(events))
	}
}

func TestManagedBootstrap_CanTransition(t *testing.T) {
	mb := NewManagedBootstrap(BootstrapModeSeed, "test-cluster", nil)

	if !mb.CanTransition(BootstrapEventStart) {
		t.Error("expected CanTransition(start) to be true from initializing")
	}
	if mb.CanTransition(BootstrapEventValidated) {
		t.Error("expected CanTransition(validated) to be false from initializing")
	}
	if !mb.CanTransition(BootstrapEventFail) {
		t.Error("expected CanTransition(fail) to be true from initializing")
	}
}

func TestManagedBootstrap_NilCallbacks(t *testing.T) {
	mb := NewManagedBootstrap(BootstrapModeSeed, "test-cluster", nil)

	// These should not panic with nil callbacks
	mb.Start()
	mb.UpdateProgress(50, "test")
	mb.MarkValidated()
	mb.MarkDepsInstalled()
	mb.MarkServerInstalled()
	mb.MarkServerConfigured()
	mb.MarkServerStarted()
	mb.SkipAgents()
	mb.SkipVerification()
	mb.MarkHandoffComplete()
}

func TestManagedBootstrap_EmptyCallbacks(t *testing.T) {
	callbacks := &BootstrapCallbacks{}
	mb := NewManagedBootstrap(BootstrapModeSeed, "test-cluster", callbacks)

	// These should not panic with empty callbacks
	mb.Start()
	mb.UpdateProgress(50, "test")
	mb.Fail(errors.New("test"))
}

func TestManagedBootstrap_StatusUpdates(t *testing.T) {
	mb := NewManagedBootstrap(BootstrapModeSeed, "test-cluster", nil)

	// Initial status
	if mb.Status.Phase != PhaseInitializing {
		t.Errorf("expected initializing phase in status, got %v", mb.Status.Phase)
	}

	mb.Start()

	// Status should reflect current phase
	if mb.Status.Phase != PhaseValidating {
		t.Errorf("expected validating phase in status, got %v", mb.Status.Phase)
	}
	if mb.Status.Progress != 10 {
		t.Errorf("expected progress 10 in status, got %d", mb.Status.Progress)
	}

	// Test failure status
	testErr := errors.New("test error")
	mb.Fail(testErr)

	if mb.Status.Error != "test error" {
		t.Errorf("expected error in status, got %v", mb.Status.Error)
	}
}

func TestPhaseToString(t *testing.T) {
	tests := []struct {
		phase    BootstrapPhase
		expected string
	}{
		{PhaseInitializing, "Initializing"},
		{PhaseValidating, "Validating"},
		{PhaseInstallingDeps, "Installing Dependencies"},
		{PhaseInstallingServer, "Installing Server"},
		{PhaseConfiguringServer, "Configuring Server"},
		{PhaseStartingServer, "Starting Server"},
		{PhaseFormingCluster, "Forming Cluster"},
		{PhaseInstallingAgents, "Installing Agents"},
		{PhaseApplyingStates, "Applying States"},
		{PhaseVerifying, "Verifying"},
		{PhaseHandoff, "Handoff"},
		{PhaseComplete, "Complete"},
		{PhaseFailed, "Failed"},
		{BootstrapPhase("unknown"), "unknown"},
	}

	for _, tt := range tests {
		t.Run(string(tt.phase), func(t *testing.T) {
			if got := PhaseToString(tt.phase); got != tt.expected {
				t.Errorf("PhaseToString(%v) = %v, want %v", tt.phase, got, tt.expected)
			}
		})
	}
}

func TestManagedBootstrap_StateDiagram(t *testing.T) {
	mb := NewManagedBootstrap(BootstrapModeSeed, "test-cluster", nil)
	diagram := mb.StateDiagram()

	if diagram == "" {
		t.Error("expected non-empty state diagram")
	}
	if len(diagram) < 100 {
		t.Error("state diagram seems too short")
	}
}

func TestRunFullBootstrapWorkflow(t *testing.T) {
	mb := NewManagedBootstrap(BootstrapModeSeed, "test-cluster", nil)
	opts := BootstrapOptions{
		SkipVerification: false,
	}

	phasesExecuted := make([]BootstrapPhase, 0)
	doPhase := func(phase BootstrapPhase) error {
		phasesExecuted = append(phasesExecuted, phase)
		return nil
	}

	err := RunFullBootstrapWorkflow(mb, opts, doPhase)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	if !mb.IsComplete() {
		t.Error("expected bootstrap to be complete")
	}

	expectedPhases := []BootstrapPhase{
		PhaseValidating,
		PhaseInstallingDeps,
		PhaseInstallingServer,
		PhaseConfiguringServer,
		PhaseStartingServer,
		PhaseFormingCluster,
		PhaseInstallingAgents,
		PhaseApplyingStates,
		PhaseVerifying,
		PhaseHandoff,
	}

	if len(phasesExecuted) != len(expectedPhases) {
		t.Errorf("expected %d phases, got %d", len(expectedPhases), len(phasesExecuted))
	}

	for i, phase := range expectedPhases {
		if phasesExecuted[i] != phase {
			t.Errorf("phase %d: expected %v, got %v", i, phase, phasesExecuted[i])
		}
	}
}

func TestRunFullBootstrapWorkflow_SkipVerification(t *testing.T) {
	mb := NewManagedBootstrap(BootstrapModeSeed, "test-cluster", nil)
	opts := BootstrapOptions{
		SkipVerification: true,
	}

	phasesExecuted := make([]BootstrapPhase, 0)
	doPhase := func(phase BootstrapPhase) error {
		phasesExecuted = append(phasesExecuted, phase)
		return nil
	}

	err := RunFullBootstrapWorkflow(mb, opts, doPhase)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	// Verify that PhaseVerifying was not executed
	for _, phase := range phasesExecuted {
		if phase == PhaseVerifying {
			t.Error("PhaseVerifying should have been skipped")
		}
	}
}

func TestRunFullBootstrapWorkflow_FailureHandling(t *testing.T) {
	mb := NewManagedBootstrap(BootstrapModeSeed, "test-cluster", nil)
	opts := BootstrapOptions{}

	expectedErr := errors.New("server install failed")
	doPhase := func(phase BootstrapPhase) error {
		if phase == PhaseInstallingServer {
			return expectedErr
		}
		return nil
	}

	err := RunFullBootstrapWorkflow(mb, opts, doPhase)
	if err != expectedErr {
		t.Errorf("expected error %v, got %v", expectedErr, err)
	}

	if !mb.IsFailed() {
		t.Error("expected bootstrap to be failed")
	}
}

func TestManagedBootstrap_ResultTracking(t *testing.T) {
	mb := NewManagedBootstrap(BootstrapModeSeed, "test-cluster", nil)

	// Set result during workflow
	result := &BootstrapResult{
		ClusterID:     "test-123",
		APIEndpoint:   "localhost:8080",
		AdminToken:    "token",
		CAFingerprint: "sha256:abc",
	}
	mb.SetResult(result)

	// Complete workflow
	mb.Start()
	mb.MarkValidated()
	mb.MarkDepsInstalled()
	mb.MarkServerInstalled()
	mb.MarkServerConfigured()
	mb.MarkServerStarted()
	mb.SkipAgents()
	mb.SkipVerification()
	mb.MarkHandoffComplete()

	// Result should be updated with success
	if !mb.Result.Success {
		t.Error("expected Result.Success to be true")
	}
	if mb.Result.ClusterID != "test-123" {
		t.Errorf("expected ClusterID test-123, got %s", mb.Result.ClusterID)
	}
	if mb.Result.Duration == 0 {
		t.Error("expected non-zero duration in result")
	}
}
