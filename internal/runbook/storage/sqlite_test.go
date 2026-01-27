package storage

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/shawnbutts/keystone-core/internal/runbook"
)

func TestSQLiteStorage_SaveAndGetExecution(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "test.db")

	storage, err := NewSQLiteStorage(dbPath)
	if err != nil {
		t.Fatalf("NewSQLiteStorage() error = %v", err)
	}
	defer storage.Close()

	ctx := context.Background()
	now := time.Now()

	exec := &runbook.Execution{
		ID:             "exec-123",
		RunbookName:    "test-runbook",
		RunbookVersion: "1.0.0",
		State:          runbook.ExecutionStateRunning,
		Inputs: map[string]interface{}{
			"target": "localhost",
			"count":  42,
		},
		StartedAt: &now,
		CreatedAt: now,
		Steps: map[string]*runbook.StepExecution{
			"step1": {
				Name:       "step1",
				Type:       runbook.StepTypeNoop,
				State:      runbook.StepStateCompleted,
				StartedAt:  &now,
				RetryCount: 0,
			},
		},
	}

	// Save execution
	if err := storage.SaveExecution(ctx, exec); err != nil {
		t.Fatalf("SaveExecution() error = %v", err)
	}

	// Get execution
	got, err := storage.GetExecution(ctx, "exec-123")
	if err != nil {
		t.Fatalf("GetExecution() error = %v", err)
	}

	if got == nil {
		t.Fatal("GetExecution() returned nil")
	}

	if got.ID != exec.ID {
		t.Errorf("ID = %v, want %v", got.ID, exec.ID)
	}

	if got.RunbookName != exec.RunbookName {
		t.Errorf("RunbookName = %v, want %v", got.RunbookName, exec.RunbookName)
	}

	if got.State != exec.State {
		t.Errorf("State = %v, want %v", got.State, exec.State)
	}

	if v, ok := got.Inputs["target"]; !ok || v != "localhost" {
		t.Errorf("Inputs[target] = %v, want localhost", v)
	}

	if got.StartedAt == nil {
		t.Error("StartedAt should not be nil")
	}

	// Check step was saved
	if len(got.Steps) != 1 {
		t.Errorf("len(Steps) = %d, want 1", len(got.Steps))
	}

	if step, ok := got.Steps["step1"]; ok {
		if step.State != runbook.StepStateCompleted {
			t.Errorf("step1.State = %v, want %v", step.State, runbook.StepStateCompleted)
		}
	} else {
		t.Error("step1 not found in Steps")
	}
}

func TestSQLiteStorage_UpdateExecution(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "test.db")

	storage, err := NewSQLiteStorage(dbPath)
	if err != nil {
		t.Fatalf("NewSQLiteStorage() error = %v", err)
	}
	defer storage.Close()

	ctx := context.Background()
	now := time.Now()

	exec := &runbook.Execution{
		ID:          "exec-456",
		RunbookName: "test-runbook",
		State:       runbook.ExecutionStatePending,
		CreatedAt:   now,
	}

	// Save initial
	if err := storage.SaveExecution(ctx, exec); err != nil {
		t.Fatalf("SaveExecution() error = %v", err)
	}

	// Update
	exec.State = runbook.ExecutionStateCompleted
	completedAt := now.Add(time.Minute)
	exec.CompletedAt = &completedAt

	if err := storage.SaveExecution(ctx, exec); err != nil {
		t.Fatalf("SaveExecution() update error = %v", err)
	}

	// Verify
	got, err := storage.GetExecution(ctx, "exec-456")
	if err != nil {
		t.Fatalf("GetExecution() error = %v", err)
	}

	if got.State != runbook.ExecutionStateCompleted {
		t.Errorf("State = %v, want %v", got.State, runbook.ExecutionStateCompleted)
	}

	if got.CompletedAt == nil {
		t.Error("CompletedAt should not be nil")
	}
}

func TestSQLiteStorage_GetExecution_NotFound(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "test.db")

	storage, err := NewSQLiteStorage(dbPath)
	if err != nil {
		t.Fatalf("NewSQLiteStorage() error = %v", err)
	}
	defer storage.Close()

	ctx := context.Background()

	got, err := storage.GetExecution(ctx, "nonexistent")
	if err != nil {
		t.Fatalf("GetExecution() error = %v", err)
	}

	if got != nil {
		t.Errorf("GetExecution() = %v, want nil", got)
	}
}

func TestSQLiteStorage_ListExecutions(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "test.db")

	storage, err := NewSQLiteStorage(dbPath)
	if err != nil {
		t.Fatalf("NewSQLiteStorage() error = %v", err)
	}
	defer storage.Close()

	ctx := context.Background()
	now := time.Now()

	// Create test executions
	executions := []*runbook.Execution{
		{
			ID:          "exec-1",
			RunbookName: "runbook-a",
			State:       runbook.ExecutionStateCompleted,
			CreatedAt:   now,
		},
		{
			ID:          "exec-2",
			RunbookName: "runbook-a",
			State:       runbook.ExecutionStateFailed,
			CreatedAt:   now.Add(time.Second),
		},
		{
			ID:          "exec-3",
			RunbookName: "runbook-b",
			State:       runbook.ExecutionStateCompleted,
			CreatedAt:   now.Add(2 * time.Second),
		},
	}

	for _, exec := range executions {
		if err := storage.SaveExecution(ctx, exec); err != nil {
			t.Fatalf("SaveExecution() error = %v", err)
		}
	}

	t.Run("list all", func(t *testing.T) {
		list, err := storage.ListExecutions(ctx, ListOptions{})
		if err != nil {
			t.Fatalf("ListExecutions() error = %v", err)
		}

		if len(list) != 3 {
			t.Errorf("len(list) = %d, want 3", len(list))
		}
	})

	t.Run("filter by runbook", func(t *testing.T) {
		list, err := storage.ListExecutions(ctx, ListOptions{
			RunbookName: "runbook-a",
		})
		if err != nil {
			t.Fatalf("ListExecutions() error = %v", err)
		}

		if len(list) != 2 {
			t.Errorf("len(list) = %d, want 2", len(list))
		}
	})

	t.Run("filter by state", func(t *testing.T) {
		list, err := storage.ListExecutions(ctx, ListOptions{
			State: runbook.ExecutionStateCompleted,
		})
		if err != nil {
			t.Fatalf("ListExecutions() error = %v", err)
		}

		if len(list) != 2 {
			t.Errorf("len(list) = %d, want 2", len(list))
		}
	})

	t.Run("with limit", func(t *testing.T) {
		list, err := storage.ListExecutions(ctx, ListOptions{
			Limit: 2,
		})
		if err != nil {
			t.Fatalf("ListExecutions() error = %v", err)
		}

		if len(list) != 2 {
			t.Errorf("len(list) = %d, want 2", len(list))
		}
	})

	t.Run("with offset", func(t *testing.T) {
		list, err := storage.ListExecutions(ctx, ListOptions{
			Limit:  2,
			Offset: 1,
		})
		if err != nil {
			t.Fatalf("ListExecutions() error = %v", err)
		}

		if len(list) != 2 {
			t.Errorf("len(list) = %d, want 2", len(list))
		}
	})
}

func TestSQLiteStorage_DeleteExecution(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "test.db")

	storage, err := NewSQLiteStorage(dbPath)
	if err != nil {
		t.Fatalf("NewSQLiteStorage() error = %v", err)
	}
	defer storage.Close()

	ctx := context.Background()
	now := time.Now()

	exec := &runbook.Execution{
		ID:          "exec-to-delete",
		RunbookName: "test-runbook",
		State:       runbook.ExecutionStateCompleted,
		CreatedAt:   now,
		Steps: map[string]*runbook.StepExecution{
			"step1": {
				Name:  "step1",
				Type:  runbook.StepTypeNoop,
				State: runbook.StepStateCompleted,
			},
		},
	}

	if err := storage.SaveExecution(ctx, exec); err != nil {
		t.Fatalf("SaveExecution() error = %v", err)
	}

	// Delete
	if err := storage.DeleteExecution(ctx, "exec-to-delete"); err != nil {
		t.Fatalf("DeleteExecution() error = %v", err)
	}

	// Verify deleted
	got, err := storage.GetExecution(ctx, "exec-to-delete")
	if err != nil {
		t.Fatalf("GetExecution() error = %v", err)
	}

	if got != nil {
		t.Error("execution should have been deleted")
	}

	// Verify steps also deleted (cascade)
	steps, err := storage.ListStepExecutions(ctx, "exec-to-delete")
	if err != nil {
		t.Fatalf("ListStepExecutions() error = %v", err)
	}

	if len(steps) != 0 {
		t.Errorf("steps should have been deleted, got %d", len(steps))
	}
}

func TestSQLiteStorage_StepExecutions(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "test.db")

	storage, err := NewSQLiteStorage(dbPath)
	if err != nil {
		t.Fatalf("NewSQLiteStorage() error = %v", err)
	}
	defer storage.Close()

	ctx := context.Background()
	now := time.Now()

	// First create the parent execution
	exec := &runbook.Execution{
		ID:          "exec-789",
		RunbookName: "test-runbook",
		State:       runbook.ExecutionStateRunning,
		CreatedAt:   now,
	}

	if err := storage.SaveExecution(ctx, exec); err != nil {
		t.Fatalf("SaveExecution() error = %v", err)
	}

	// Save step execution
	step := &runbook.StepExecution{
		Name:       "test-step",
		Type:       runbook.StepTypeCommand,
		State:      runbook.StepStateCompleted,
		StartedAt:  &now,
		RetryCount: 2,
		Duration:   5 * time.Second,
		Outputs: map[string]interface{}{
			"exitCode": 0,
		},
	}

	if err := storage.SaveStepExecution(ctx, "exec-789", step); err != nil {
		t.Fatalf("SaveStepExecution() error = %v", err)
	}

	// Get step execution
	got, err := storage.GetStepExecution(ctx, "exec-789", "test-step")
	if err != nil {
		t.Fatalf("GetStepExecution() error = %v", err)
	}

	if got == nil {
		t.Fatal("GetStepExecution() returned nil")
	}

	if got.Name != "test-step" {
		t.Errorf("Name = %v, want %v", got.Name, "test-step")
	}

	if got.Type != runbook.StepTypeCommand {
		t.Errorf("Type = %v, want %v", got.Type, runbook.StepTypeCommand)
	}

	if got.State != runbook.StepStateCompleted {
		t.Errorf("State = %v, want %v", got.State, runbook.StepStateCompleted)
	}

	if got.RetryCount != 2 {
		t.Errorf("RetryCount = %d, want 2", got.RetryCount)
	}

	if got.Duration != 5*time.Second {
		t.Errorf("Duration = %v, want %v", got.Duration, 5*time.Second)
	}

	if v, ok := got.Outputs["exitCode"]; !ok || v != float64(0) {
		// JSON unmarshals numbers as float64
		t.Errorf("Outputs[exitCode] = %v, want 0", v)
	}

	// List step executions
	steps, err := storage.ListStepExecutions(ctx, "exec-789")
	if err != nil {
		t.Fatalf("ListStepExecutions() error = %v", err)
	}

	if len(steps) != 1 {
		t.Errorf("len(steps) = %d, want 1", len(steps))
	}
}

func TestSQLiteStorage_GetStepExecution_NotFound(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "test.db")

	storage, err := NewSQLiteStorage(dbPath)
	if err != nil {
		t.Fatalf("NewSQLiteStorage() error = %v", err)
	}
	defer storage.Close()

	ctx := context.Background()

	got, err := storage.GetStepExecution(ctx, "nonexistent", "step")
	if err != nil {
		t.Fatalf("GetStepExecution() error = %v", err)
	}

	if got != nil {
		t.Errorf("GetStepExecution() = %v, want nil", got)
	}
}
