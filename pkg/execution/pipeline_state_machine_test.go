package execution

import (
	"errors"
	"testing"
	"time"

	"github.com/shawnbutts/keystone-core/pkg/statemachine"
)

func TestManagedPipeline_InitialState(t *testing.T) {
	pipeline := &Pipeline{
		ID: "test-pipeline-1",
		Stages: []*PipelineStage{
			{ID: "stage-1", Command: "echo hello"},
			{ID: "stage-2", Command: "cat"},
		},
	}

	mp := NewManagedPipeline(pipeline, nil)

	if mp.State() != PipelineStatePending {
		t.Errorf("expected pending state, got %v", mp.State())
	}
	if !mp.IsPending() {
		t.Error("expected IsPending() to be true")
	}
	if mp.IsRunning() {
		t.Error("expected IsRunning() to be false")
	}
	if mp.IsTerminal() {
		t.Error("expected IsTerminal() to be false")
	}
	if mp.StageCount() != 2 {
		t.Errorf("expected 2 stages, got %d", mp.StageCount())
	}
}

func TestManagedPipeline_StartWorkflow(t *testing.T) {
	pipeline := &Pipeline{
		ID: "test-pipeline-1",
		Stages: []*PipelineStage{
			{ID: "stage-1", Command: "echo hello"},
		},
	}

	mp := NewManagedPipeline(pipeline, nil)

	if err := mp.Start(); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if mp.State() != PipelineStateRunning {
		t.Errorf("expected running state, got %v", mp.State())
	}
	if !mp.IsRunning() {
		t.Error("expected IsRunning() to be true")
	}
	if mp.Duration() == 0 {
		t.Error("expected non-zero duration after start")
	}
}

func TestManagedPipeline_CompleteWorkflow(t *testing.T) {
	pipeline := &Pipeline{
		ID: "test-pipeline-1",
		Stages: []*PipelineStage{
			{ID: "stage-1", Command: "echo hello"},
			{ID: "stage-2", Command: "cat"},
		},
	}

	mp := NewManagedPipeline(pipeline, nil)

	mp.Start()

	// Execute stages
	mp.StartStage(0)
	mp.CompleteStage(0, []byte("hello"), nil, 0)

	mp.StartStage(1)
	mp.CompleteStage(1, []byte("hello"), nil, 0)

	// Complete pipeline
	if err := mp.Complete([]byte("hello")); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if mp.State() != PipelineStateCompleted {
		t.Errorf("expected completed state, got %v", mp.State())
	}
	if !mp.IsCompleted() {
		t.Error("expected IsCompleted() to be true")
	}
	if !mp.IsTerminal() {
		t.Error("expected IsTerminal() to be true")
	}
	if !mp.IsSuccessful() {
		t.Error("expected IsSuccessful() to be true")
	}
	if !mp.Result.Success {
		t.Error("expected Result.Success to be true")
	}
}

func TestManagedPipeline_PartialCompleteWorkflow(t *testing.T) {
	pipeline := &Pipeline{
		ID:              "test-pipeline-1",
		ContinueOnError: true,
		Stages: []*PipelineStage{
			{ID: "stage-1", Command: "echo hello"},
			{ID: "stage-2", Command: "failing command"},
			{ID: "stage-3", Command: "echo world"},
		},
	}

	mp := NewManagedPipeline(pipeline, nil)

	mp.Start()

	// Execute stages
	mp.StartStage(0)
	mp.CompleteStage(0, []byte("hello"), nil, 0)

	mp.StartStage(1)
	mp.FailStage(1, errors.New("command failed"), nil, 1)

	mp.StartStage(2)
	mp.CompleteStage(2, []byte("world"), nil, 0)

	// Partial complete
	if err := mp.PartialComplete([]byte("world")); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if mp.State() != PipelineStatePartiallyCompleted {
		t.Errorf("expected partially_completed state, got %v", mp.State())
	}
	if !mp.IsTerminal() {
		t.Error("expected IsTerminal() to be true")
	}
	if mp.IsSuccessful() {
		t.Error("expected IsSuccessful() to be false")
	}
}

func TestManagedPipeline_FailWorkflow(t *testing.T) {
	pipeline := &Pipeline{
		ID:          "test-pipeline-1",
		StopOnError: true,
		Stages: []*PipelineStage{
			{ID: "stage-1", Command: "echo hello"},
			{ID: "stage-2", Command: "failing command"},
		},
	}

	mp := NewManagedPipeline(pipeline, nil)

	mp.Start()

	mp.StartStage(0)
	mp.CompleteStage(0, []byte("hello"), nil, 0)

	mp.StartStage(1)
	mp.FailStage(1, errors.New("command failed"), nil, 1)

	// Fail pipeline
	if err := mp.Fail(errors.New("stage 2 failed")); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if mp.State() != PipelineStateFailed {
		t.Errorf("expected failed state, got %v", mp.State())
	}
	if !mp.IsTerminal() {
		t.Error("expected IsTerminal() to be true")
	}
	if mp.Result.Error == nil {
		t.Error("expected Result.Error to be set")
	}
}

func TestManagedPipeline_CancelWorkflow(t *testing.T) {
	pipeline := &Pipeline{
		ID: "test-pipeline-1",
		Stages: []*PipelineStage{
			{ID: "stage-1", Command: "echo hello"},
			{ID: "stage-2", Command: "long running"},
			{ID: "stage-3", Command: "never executed"},
		},
	}

	mp := NewManagedPipeline(pipeline, nil)

	mp.Start()

	mp.StartStage(0)
	mp.CompleteStage(0, []byte("hello"), nil, 0)

	mp.StartStage(1)

	// Cancel while stage-2 is running
	if err := mp.Cancel(); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if mp.State() != PipelineStateCancelled {
		t.Errorf("expected cancelled state, got %v", mp.State())
	}
	if !mp.IsTerminal() {
		t.Error("expected IsTerminal() to be true")
	}

	// Stage 3 should be skipped
	stage3 := mp.GetStage(2)
	if !stage3.IsSkipped() {
		t.Error("expected stage-3 to be skipped")
	}
}

func TestManagedPipeline_TimeoutWorkflow(t *testing.T) {
	pipeline := &Pipeline{
		ID: "test-pipeline-1",
		Stages: []*PipelineStage{
			{ID: "stage-1", Command: "echo hello"},
			{ID: "stage-2", Command: "sleep forever"},
		},
	}

	mp := NewManagedPipeline(pipeline, nil)

	mp.Start()
	mp.StartStage(0)
	mp.CompleteStage(0, []byte("hello"), nil, 0)
	mp.StartStage(1)

	// Timeout
	if err := mp.Timeout(); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if mp.State() != PipelineStateTimeout {
		t.Errorf("expected timeout state, got %v", mp.State())
	}
	if !mp.IsTerminal() {
		t.Error("expected IsTerminal() to be true")
	}
}

func TestManagedPipeline_InvalidTransitions(t *testing.T) {
	pipeline := &Pipeline{
		ID: "test-pipeline-1",
		Stages: []*PipelineStage{
			{ID: "stage-1", Command: "echo hello"},
		},
	}

	mp := NewManagedPipeline(pipeline, nil)

	// Cannot complete from pending
	err := mp.Complete(nil)
	if err == nil {
		t.Error("expected error for invalid transition")
	}
	if !errors.Is(err, statemachine.ErrInvalidTransition) {
		t.Errorf("expected ErrInvalidTransition, got %v", err)
	}

	// State should not have changed
	if mp.State() != PipelineStatePending {
		t.Errorf("state should not have changed, got %v", mp.State())
	}
}

func TestManagedPipeline_Callbacks(t *testing.T) {
	var startedCalls, completedCalls, failedCalls, stageStartedCalls, stageCompletedCalls int
	var lastStageIndex int

	callbacks := &PipelineCallbacks{
		OnStarted: func(pipelineID string) {
			startedCalls++
		},
		OnCompleted: func(pipelineID string, result *PipelineResult) {
			completedCalls++
		},
		OnFailed: func(pipelineID string, err error) {
			failedCalls++
		},
		OnStageStarted: func(pipelineID string, stageIndex int, stageID string) {
			stageStartedCalls++
			lastStageIndex = stageIndex
		},
		OnStageCompleted: func(pipelineID string, stageIndex int, stageID string, result *StageResult) {
			stageCompletedCalls++
		},
	}

	pipeline := &Pipeline{
		ID: "test-pipeline-1",
		Stages: []*PipelineStage{
			{ID: "stage-1", Command: "echo hello"},
			{ID: "stage-2", Command: "cat"},
		},
	}

	mp := NewManagedPipeline(pipeline, callbacks)

	// Start triggers callback
	mp.Start()
	if startedCalls != 1 {
		t.Errorf("expected OnStarted called once, got %d", startedCalls)
	}

	// Stage callbacks
	mp.StartStage(0)
	if stageStartedCalls != 1 || lastStageIndex != 0 {
		t.Errorf("expected OnStageStarted called with index 0, got %d calls, index %d", stageStartedCalls, lastStageIndex)
	}

	mp.CompleteStage(0, nil, nil, 0)
	if stageCompletedCalls != 1 {
		t.Errorf("expected OnStageCompleted called once, got %d", stageCompletedCalls)
	}

	mp.StartStage(1)
	mp.CompleteStage(1, nil, nil, 0)

	// Complete triggers callback
	mp.Complete(nil)
	if completedCalls != 1 {
		t.Errorf("expected OnCompleted called once, got %d", completedCalls)
	}
}

func TestManagedPipeline_History(t *testing.T) {
	pipeline := &Pipeline{
		ID: "test-pipeline-1",
		Stages: []*PipelineStage{
			{ID: "stage-1", Command: "echo hello"},
		},
	}

	mp := NewManagedPipeline(pipeline, nil)

	mp.Start()
	mp.StartStage(0)
	mp.CompleteStage(0, nil, nil, 0)
	mp.Complete(nil)

	history := mp.History()
	if history == nil {
		t.Fatal("history should not be nil")
	}

	records := history.All()
	if len(records) != 2 {
		t.Errorf("expected 2 history records, got %d", len(records))
	}
}

func TestManagedPipeline_AvailableEvents(t *testing.T) {
	pipeline := &Pipeline{
		ID: "test-pipeline-1",
		Stages: []*PipelineStage{
			{ID: "stage-1", Command: "echo hello"},
		},
	}

	mp := NewManagedPipeline(pipeline, nil)

	// From pending, can start or cancel
	events := mp.AvailableEvents()
	if len(events) != 2 {
		t.Errorf("expected 2 available events from pending, got %d", len(events))
	}

	mp.Start()

	// From running, can complete, partial complete, fail, cancel, or timeout
	events = mp.AvailableEvents()
	if len(events) != 5 {
		t.Errorf("expected 5 available events from running, got %d", len(events))
	}
}

func TestManagedPipeline_Duration(t *testing.T) {
	pipeline := &Pipeline{
		ID: "test-pipeline-1",
		Stages: []*PipelineStage{
			{ID: "stage-1", Command: "echo hello"},
		},
	}

	mp := NewManagedPipeline(pipeline, nil)

	// No duration before start
	if mp.Duration() != 0 {
		t.Error("expected 0 duration before start")
	}

	mp.Start()
	time.Sleep(1 * time.Millisecond)

	// Duration should be non-zero while running
	runningDuration := mp.Duration()
	if runningDuration == 0 {
		t.Error("expected non-zero duration while running")
	}

	mp.StartStage(0)
	mp.CompleteStage(0, nil, nil, 0)
	mp.Complete(nil)

	// Duration should be fixed after completion
	finalDuration := mp.Duration()
	if finalDuration < runningDuration {
		t.Error("expected final duration >= running duration")
	}
}

func TestManagedStage_Workflow(t *testing.T) {
	pipeline := &Pipeline{
		ID: "test-pipeline-1",
		Stages: []*PipelineStage{
			{ID: "stage-1", Command: "echo hello"},
		},
	}

	mp := NewManagedPipeline(pipeline, nil)
	mp.Start()

	stage := mp.GetStage(0)
	if stage == nil {
		t.Fatal("expected stage not to be nil")
	}

	// Initial state
	if stage.State() != StageStatePending {
		t.Errorf("expected pending state, got %v", stage.State())
	}
	if !stage.IsPending() {
		t.Error("expected IsPending() to be true")
	}

	// Start
	mp.StartStage(0)
	if !stage.IsRunning() {
		t.Error("expected IsRunning() to be true")
	}

	// Complete
	mp.CompleteStage(0, []byte("output"), []byte("stderr"), 0)
	if !stage.IsCompleted() {
		t.Error("expected IsCompleted() to be true")
	}
	if !stage.IsTerminal() {
		t.Error("expected IsTerminal() to be true")
	}
	if string(stage.Result.Output) != "output" {
		t.Errorf("expected output to be 'output', got %s", stage.Result.Output)
	}
}

func TestManagedStage_FailWorkflow(t *testing.T) {
	pipeline := &Pipeline{
		ID: "test-pipeline-1",
		Stages: []*PipelineStage{
			{ID: "stage-1", Command: "failing"},
		},
	}

	mp := NewManagedPipeline(pipeline, nil)
	mp.Start()
	mp.StartStage(0)

	stage := mp.GetStage(0)
	mp.FailStage(0, errors.New("command not found"), []byte("error"), 127)

	if !stage.IsFailed() {
		t.Error("expected IsFailed() to be true")
	}
	if stage.Result.ExitCode != 127 {
		t.Errorf("expected exit code 127, got %d", stage.Result.ExitCode)
	}
}

func TestManagedStage_SkipWorkflow(t *testing.T) {
	pipeline := &Pipeline{
		ID: "test-pipeline-1",
		Stages: []*PipelineStage{
			{ID: "stage-1", Command: "never run"},
		},
	}

	mp := NewManagedPipeline(pipeline, nil)
	mp.Start()

	mp.SkipStage(0)

	stage := mp.GetStage(0)
	if !stage.IsSkipped() {
		t.Error("expected IsSkipped() to be true")
	}
	if !stage.IsTerminal() {
		t.Error("expected IsTerminal() to be true")
	}
	if !stage.Result.Skipped {
		t.Error("expected Result.Skipped to be true")
	}
}

func TestManagedPipeline_NilCallbacks(t *testing.T) {
	pipeline := &Pipeline{
		ID: "test-pipeline-1",
		Stages: []*PipelineStage{
			{ID: "stage-1", Command: "echo hello"},
		},
	}

	// Empty callbacks struct
	callbacks := &PipelineCallbacks{}

	mp := NewManagedPipeline(pipeline, callbacks)

	// These should not panic
	mp.Start()
	mp.StartStage(0)
	mp.CompleteStage(0, nil, nil, 0)
	mp.Complete(nil)
}

func TestPipelineStateToString(t *testing.T) {
	tests := []struct {
		state   PipelineState
		display string
	}{
		{PipelineStatePending, "Pending"},
		{PipelineStateRunning, "Running"},
		{PipelineStateCompleted, "Completed"},
		{PipelineStatePartiallyCompleted, "Partially Completed"},
		{PipelineStateFailed, "Failed"},
		{PipelineStateCancelled, "Cancelled"},
		{PipelineStateTimeout, "Timeout"},
		{PipelineState("unknown"), "unknown"},
	}

	for _, tt := range tests {
		t.Run(string(tt.state), func(t *testing.T) {
			if got := PipelineStateToString(tt.state); got != tt.display {
				t.Errorf("PipelineStateToString(%v) = %v, want %v", tt.state, got, tt.display)
			}
		})
	}
}

func TestStageStateToString(t *testing.T) {
	tests := []struct {
		state   StageState
		display string
	}{
		{StageStatePending, "Pending"},
		{StageStateRunning, "Running"},
		{StageStateCompleted, "Completed"},
		{StageStateFailed, "Failed"},
		{StageStateSkipped, "Skipped"},
		{StageState("unknown"), "unknown"},
	}

	for _, tt := range tests {
		t.Run(string(tt.state), func(t *testing.T) {
			if got := StageStateToString(tt.state); got != tt.display {
				t.Errorf("StageStateToString(%v) = %v, want %v", tt.state, got, tt.display)
			}
		})
	}
}

func TestManagedPipeline_FullWorkflow(t *testing.T) {
	pipeline := &Pipeline{
		ID: "test-pipeline-1",
		Stages: []*PipelineStage{
			{ID: "stage-1", Command: "echo hello"},
			{ID: "stage-2", Command: "cat"},
			{ID: "stage-3", Command: "wc -l"},
		},
	}

	mp := NewManagedPipeline(pipeline, nil)

	// Full workflow
	mp.Start()
	if !mp.IsRunning() {
		t.Error("expected running")
	}

	// Stage 1
	mp.StartStage(0)
	mp.CompleteStage(0, []byte("hello\n"), nil, 0)
	if !mp.GetStage(0).IsCompleted() {
		t.Error("expected stage 1 completed")
	}

	// Stage 2
	mp.StartStage(1)
	mp.CompleteStage(1, []byte("hello\n"), nil, 0)
	if !mp.GetStage(1).IsCompleted() {
		t.Error("expected stage 2 completed")
	}

	// Stage 3
	mp.StartStage(2)
	mp.CompleteStage(2, []byte("1\n"), nil, 0)
	if !mp.GetStage(2).IsCompleted() {
		t.Error("expected stage 3 completed")
	}

	// Complete pipeline
	mp.Complete([]byte("1\n"))
	if !mp.IsSuccessful() {
		t.Error("expected successful")
	}
	if !mp.IsTerminal() {
		t.Error("expected terminal")
	}
}
