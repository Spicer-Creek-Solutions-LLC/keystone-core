package execution

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/shawnbutts/keystone-core/internal/runbook"
)

func TestManager_Track(t *testing.T) {
	manager := NewManager(nil, nil)

	rb := &runbook.Runbook{
		Metadata: runbook.Metadata{Name: "test-runbook"},
		Spec: runbook.Spec{
			Steps: []runbook.Step{
				{Name: "step1", Type: "noop"},
			},
		},
	}

	execCtx := NewContext("exec-1", rb, nil)
	managed := manager.Track(execCtx)

	if managed == nil {
		t.Fatal("Track() returned nil")
	}

	if managed.ExecutionID() != "exec-1" {
		t.Errorf("ExecutionID = %v, want exec-1", managed.ExecutionID())
	}

	// Should be retrievable
	got, ok := manager.Get("exec-1")
	if !ok {
		t.Error("Get() returned false")
	}
	if got != managed {
		t.Error("Get() returned different execution")
	}

	// List should contain it
	list := manager.List()
	if len(list) != 1 {
		t.Errorf("List() len = %d, want 1", len(list))
	}

	// Untrack
	manager.Untrack("exec-1")

	_, ok = manager.Get("exec-1")
	if ok {
		t.Error("Get() should return false after Untrack")
	}
}

func TestManager_PauseResume(t *testing.T) {
	manager := NewManager(nil, nil)

	rb := &runbook.Runbook{
		Metadata: runbook.Metadata{Name: "test-runbook"},
		Spec: runbook.Spec{
			Steps: []runbook.Step{
				{Name: "step1", Type: "noop"},
			},
		},
	}

	execCtx := NewContext("exec-1", rb, nil)
	_ = execCtx.Start(context.Background())
	managed := manager.Track(execCtx)

	// Initially not paused
	if managed.IsPaused() {
		t.Error("Should not be paused initially")
	}

	// Pause
	err := manager.Pause("exec-1")
	if err != nil {
		t.Fatalf("Pause() error = %v", err)
	}

	if !managed.IsPaused() {
		t.Error("Should be paused after Pause()")
	}

	if managed.PausedAt() == nil {
		t.Error("PausedAt should be set")
	}

	// Pause again should error
	err = manager.Pause("exec-1")
	if err == nil {
		t.Error("Pause() should error when already paused")
	}

	// Resume
	err = manager.Resume("exec-1")
	if err != nil {
		t.Fatalf("Resume() error = %v", err)
	}

	if managed.IsPaused() {
		t.Error("Should not be paused after Resume()")
	}

	if managed.ResumedAt() == nil {
		t.Error("ResumedAt should be set")
	}

	// Resume again should error
	err = manager.Resume("exec-1")
	if err == nil {
		t.Error("Resume() should error when not paused")
	}
}

func TestManager_PauseTerminal(t *testing.T) {
	manager := NewManager(nil, nil)

	rb := &runbook.Runbook{
		Metadata: runbook.Metadata{Name: "test-runbook"},
		Spec:     runbook.Spec{},
	}

	execCtx := NewContext("exec-1", rb, nil)
	_ = execCtx.Start(context.Background())
	_ = execCtx.Complete(context.Background())
	manager.Track(execCtx)

	// Cannot pause terminal execution
	err := manager.Pause("exec-1")
	if err == nil {
		t.Error("Pause() should error for terminal execution")
	}
}

func TestManager_WaitIfPaused(t *testing.T) {
	manager := NewManager(nil, nil)

	rb := &runbook.Runbook{
		Metadata: runbook.Metadata{Name: "test-runbook"},
		Spec:     runbook.Spec{},
	}

	execCtx := NewContext("exec-1", rb, nil)
	_ = execCtx.Start(context.Background())
	manager.Track(execCtx)

	// Pause the execution
	_ = manager.Pause("exec-1")

	// WaitIfPaused should block until resumed
	var wg sync.WaitGroup
	wg.Add(1)

	var resumed bool
	go func() {
		defer wg.Done()
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		resumed = manager.WaitIfPaused(ctx, "exec-1")
	}()

	// Give goroutine time to start waiting
	time.Sleep(50 * time.Millisecond)

	// Resume
	_ = manager.Resume("exec-1")

	wg.Wait()

	if !resumed {
		t.Error("WaitIfPaused should return true after resume")
	}
}

func TestManager_WaitIfPaused_Cancelled(t *testing.T) {
	manager := NewManager(nil, nil)

	rb := &runbook.Runbook{
		Metadata: runbook.Metadata{Name: "test-runbook"},
		Spec:     runbook.Spec{},
	}

	execCtx := NewContext("exec-1", rb, nil)
	_ = execCtx.Start(context.Background())
	manager.Track(execCtx)

	// Pause the execution
	_ = manager.Pause("exec-1")

	// WaitIfPaused with short timeout should return false
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	resumed := manager.WaitIfPaused(ctx, "exec-1")
	if resumed {
		t.Error("WaitIfPaused should return false on context cancellation")
	}
}

func TestManager_SkipStep(t *testing.T) {
	manager := NewManager(nil, nil)

	rb := &runbook.Runbook{
		Metadata: runbook.Metadata{Name: "test-runbook"},
		Spec: runbook.Spec{
			Steps: []runbook.Step{
				{Name: "step1", Type: "noop"},
				{Name: "step2", Type: "noop"},
			},
		},
	}

	execCtx := NewContext("exec-1", rb, nil)
	_ = execCtx.Start(context.Background())
	managed := manager.Track(execCtx)

	// Skip step1 (which is pending)
	err := manager.SkipStep(context.Background(), "exec-1", "step1")
	if err != nil {
		t.Fatalf("SkipStep() error = %v", err)
	}

	// Check step state
	stepCtx, _ := managed.GetStep("step1")
	if !stepCtx.IsSkipped() {
		t.Error("Step should be skipped")
	}

	// Skipped steps should be tracked
	skipped := managed.GetSkippedSteps()
	if len(skipped) != 1 || skipped[0] != "step1" {
		t.Errorf("GetSkippedSteps() = %v, want [step1]", skipped)
	}

	// Cannot skip already skipped step
	err = manager.SkipStep(context.Background(), "exec-1", "step1")
	if err == nil {
		t.Error("SkipStep() should error for non-pending step")
	}
}

func TestManager_RetryStep(t *testing.T) {
	manager := NewManager(nil, nil)

	rb := &runbook.Runbook{
		Metadata: runbook.Metadata{Name: "test-runbook"},
		Spec: runbook.Spec{
			Steps: []runbook.Step{
				{Name: "step1", Type: "noop"},
			},
		},
	}

	execCtx := NewContext("exec-1", rb, nil)
	_ = execCtx.Start(context.Background())
	managed := manager.Track(execCtx)

	// Start and fail step
	stepCtx, _ := managed.GetStep("step1")
	_ = stepCtx.Start(context.Background())
	_ = stepCtx.Fail(context.Background(), "test error")

	// Retry step
	err := manager.RetryStep(context.Background(), "exec-1", "step1")
	if err != nil {
		t.Fatalf("RetryStep() error = %v", err)
	}

	// Step should be reset to pending
	if stepCtx.IsFailed() {
		t.Error("Step should not be failed after retry reset")
	}

	// Retry count should be tracked
	retried := managed.GetRetriedSteps()
	if retried["step1"] != 1 {
		t.Errorf("GetRetriedSteps() = %v, want step1:1", retried)
	}
}

func TestManager_RetryStep_NotFailed(t *testing.T) {
	manager := NewManager(nil, nil)

	rb := &runbook.Runbook{
		Metadata: runbook.Metadata{Name: "test-runbook"},
		Spec: runbook.Spec{
			Steps: []runbook.Step{
				{Name: "step1", Type: "noop"},
			},
		},
	}

	execCtx := NewContext("exec-1", rb, nil)
	manager.Track(execCtx)

	// Try to retry pending step
	err := manager.RetryStep(context.Background(), "exec-1", "step1")
	if err == nil {
		t.Error("RetryStep() should error for non-failed step")
	}
}

func TestManager_Cancel(t *testing.T) {
	manager := NewManager(nil, nil)

	rb := &runbook.Runbook{
		Metadata: runbook.Metadata{Name: "test-runbook"},
		Spec:     runbook.Spec{},
	}

	execCtx := NewContext("exec-1", rb, nil)
	ctx, cancel := context.WithCancel(context.Background())
	execCtx.SetCancel(cancel)
	_ = execCtx.Start(ctx)
	manager.Track(execCtx)

	// Cancel
	err := manager.Cancel(context.Background(), "exec-1")
	if err != nil {
		t.Fatalf("Cancel() error = %v", err)
	}

	// Context should be cancelled
	select {
	case <-ctx.Done():
		// Expected
	default:
		t.Error("Context should be cancelled")
	}
}

func TestManager_CancelWithRollback(t *testing.T) {
	manager := NewManager(nil, nil)

	rollbackCalled := false
	manager.RegisterRollbackHandler("test", RollbackFunc(func(ctx context.Context, step *runbook.Step, stepExec *runbook.StepExecution) error {
		rollbackCalled = true
		return nil
	}))

	rb := &runbook.Runbook{
		Metadata: runbook.Metadata{Name: "test-runbook"},
		Spec: runbook.Spec{
			Steps: []runbook.Step{
				{Name: "step1", Type: "test"},
			},
		},
	}

	execCtx := NewContext("exec-1", rb, nil)
	ctx, cancel := context.WithCancel(context.Background())
	execCtx.SetCancel(cancel)
	_ = execCtx.Start(ctx)
	managed := manager.Track(execCtx)

	// Complete step1
	stepCtx, _ := managed.GetStep("step1")
	_ = stepCtx.Start(context.Background())
	_ = stepCtx.Complete(context.Background())

	// Cancel with rollback
	err := manager.CancelWithRollback(context.Background(), "exec-1")
	if err != nil {
		t.Fatalf("CancelWithRollback() error = %v", err)
	}

	if !rollbackCalled {
		t.Error("Rollback handler should be called")
	}
}

func TestManager_Clone(t *testing.T) {
	manager := NewManager(nil, nil)

	rb := &runbook.Runbook{
		Metadata: runbook.Metadata{Name: "test-runbook", Version: "1.0"},
		Spec: runbook.Spec{
			Steps: []runbook.Step{
				{Name: "step1", Type: "noop"},
			},
		},
	}

	inputs := map[string]interface{}{
		"param1": "value1",
		"param2": 42,
	}

	execCtx := NewContext("exec-1", rb, inputs)
	managed := manager.Track(execCtx)

	// Clone
	cloned, err := manager.CloneExecution(context.Background(), managed)
	if err != nil {
		t.Fatalf("CloneExecution() error = %v", err)
	}

	// Cloned should have new ID
	if cloned.ExecutionID() == managed.ExecutionID() {
		t.Error("Cloned should have different ID")
	}

	// Should have same runbook info
	if cloned.RunbookName() != managed.RunbookName() {
		t.Error("Cloned should have same runbook name")
	}

	// Should have same inputs
	clonedInputs := cloned.Inputs()
	if clonedInputs["param1"] != "value1" {
		t.Errorf("Cloned input param1 = %v, want value1", clonedInputs["param1"])
	}

	// Should track clone source
	if cloned.GetCloneSource() != "exec-1" {
		t.Errorf("GetCloneSource() = %v, want exec-1", cloned.GetCloneSource())
	}

	// Should be tracked
	got, ok := manager.Get(cloned.ExecutionID())
	if !ok || got != cloned {
		t.Error("Cloned execution should be tracked")
	}
}

func TestManager_NotFound(t *testing.T) {
	manager := NewManager(nil, nil)

	// All operations should return errors for non-existent execution
	err := manager.Pause("nonexistent")
	if err == nil {
		t.Error("Pause() should error for non-existent execution")
	}

	err = manager.Resume("nonexistent")
	if err == nil {
		t.Error("Resume() should error for non-existent execution")
	}

	err = manager.Cancel(context.Background(), "nonexistent")
	if err == nil {
		t.Error("Cancel() should error for non-existent execution")
	}

	err = manager.SkipStep(context.Background(), "nonexistent", "step1")
	if err == nil {
		t.Error("SkipStep() should error for non-existent execution")
	}

	err = manager.RetryStep(context.Background(), "nonexistent", "step1")
	if err == nil {
		t.Error("RetryStep() should error for non-existent execution")
	}
}

func TestManagedExecution_AddRollbackFn(t *testing.T) {
	manager := NewManager(nil, nil)

	rb := &runbook.Runbook{
		Metadata: runbook.Metadata{Name: "test-runbook"},
		Spec:     runbook.Spec{},
	}

	execCtx := NewContext("exec-1", rb, nil)
	ctx, cancel := context.WithCancel(context.Background())
	execCtx.SetCancel(cancel)
	_ = execCtx.Start(ctx)
	managed := manager.Track(execCtx)

	// Add rollback functions
	var order []int
	managed.AddRollbackFn(func(ctx context.Context) error {
		order = append(order, 1)
		return nil
	})
	managed.AddRollbackFn(func(ctx context.Context) error {
		order = append(order, 2)
		return nil
	})

	// Cancel with rollback
	_ = manager.CancelWithRollback(context.Background(), "exec-1")

	// Rollback functions should be called in reverse order
	if len(order) != 2 || order[0] != 2 || order[1] != 1 {
		t.Errorf("Rollback order = %v, want [2, 1]", order)
	}
}

func TestManagedExecution_ToManagedExecutionInfo(t *testing.T) {
	manager := NewManager(nil, nil)

	rb := &runbook.Runbook{
		Metadata: runbook.Metadata{Name: "test-runbook"},
		Spec: runbook.Spec{
			Steps: []runbook.Step{
				{Name: "step1", Type: "noop"},
			},
		},
	}

	execCtx := NewContext("exec-1", rb, map[string]interface{}{"key": "value"})
	_ = execCtx.Start(context.Background())
	managed := manager.Track(execCtx)

	// Pause and skip a step
	_ = manager.Pause("exec-1")
	_ = manager.SkipStep(context.Background(), "exec-1", "step1")
	_ = manager.Resume("exec-1")

	info := managed.ToManagedExecutionInfo()

	if info.Execution.ID != "exec-1" {
		t.Errorf("Execution.ID = %v, want exec-1", info.Execution.ID)
	}
	if info.PausedAt == nil {
		t.Error("PausedAt should be set")
	}
	if info.ResumedAt == nil {
		t.Error("ResumedAt should be set")
	}
	if len(info.SkippedSteps) != 1 {
		t.Errorf("SkippedSteps len = %d, want 1", len(info.SkippedSteps))
	}
}

func TestPauseCheckpoint(t *testing.T) {
	manager := NewManager(nil, nil)

	rb := &runbook.Runbook{
		Metadata: runbook.Metadata{Name: "test-runbook"},
		Spec:     runbook.Spec{},
	}

	execCtx := NewContext("exec-1", rb, nil)
	_ = execCtx.Start(context.Background())
	manager.Track(execCtx)

	checkpoint := NewPauseCheckpoint(manager, "exec-1")

	// Should return immediately when not paused
	ctx := context.Background()
	err := checkpoint.Check(ctx)
	if err != nil {
		t.Errorf("Check() error = %v", err)
	}

	// Pause the execution
	_ = manager.Pause("exec-1")

	// Check with cancelled context should return error
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	err = checkpoint.Check(ctx)
	if err == nil {
		t.Error("Check() should error when context cancelled while paused")
	}
}
