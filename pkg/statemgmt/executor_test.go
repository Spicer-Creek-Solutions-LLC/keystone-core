package statemgmt

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestExecutor_ExecuteState(t *testing.T) {
	executor := NewExecutor()
	ctx := context.Background()

	tmpFile := filepath.Join(os.TempDir(), "test-executor-file.txt")
	os.Remove(tmpFile)
	defer os.Remove(tmpFile)

	stateFile := &StateFile{
		Path: "test.yaml",
		States: map[string][]StateDeclaration{
			"file": {
				{
					ID:     tmpFile,
					Module: "file",
					State:  "present",
					Parameters: map[string]interface{}{
						"contents": "test content",
					},
				},
			},
		},
	}

	run, err := executor.ExecuteState(ctx, stateFile)
	if err != nil {
		t.Fatalf("ExecuteState failed: %v", err)
	}

	if run.Summary.Total != 1 {
		t.Errorf("Expected 1 state, got %d", run.Summary.Total)
	}

	if run.Summary.Succeeded != 1 {
		t.Errorf("Expected 1 succeeded, got %d", run.Summary.Succeeded)
	}

	if run.Summary.Failed != 0 {
		t.Errorf("Expected 0 failed, got %d", run.Summary.Failed)
	}

	if !run.Summary.Success {
		t.Error("Expected overall success")
	}

	// Verify file was created
	content, err := os.ReadFile(tmpFile)
	if err != nil {
		t.Fatalf("Failed to read created file: %v", err)
	}

	if string(content) != "test content" {
		t.Errorf("Expected 'test content', got '%s'", string(content))
	}
}

func TestExecutor_DryRun(t *testing.T) {
	executor := NewExecutor()
	executor.DryRun = true
	ctx := context.Background()

	tmpFile := filepath.Join(os.TempDir(), "test-executor-dryrun.txt")
	os.Remove(tmpFile)
	defer os.Remove(tmpFile)

	stateFile := &StateFile{
		Path: "test.yaml",
		States: map[string][]StateDeclaration{
			"file": {
				{
					ID:     tmpFile,
					Module: "file",
					State:  "present",
					Parameters: map[string]interface{}{
						"contents": "test content",
					},
				},
			},
		},
	}

	run, err := executor.ExecuteState(ctx, stateFile)
	if err != nil {
		t.Fatalf("ExecuteState failed: %v", err)
	}

	if run.Summary.Total != 1 {
		t.Errorf("Expected 1 state, got %d", run.Summary.Total)
	}

	if run.Summary.Changed != 1 {
		t.Errorf("Expected 1 changed (dry run), got %d", run.Summary.Changed)
	}

	// Verify file was NOT created (dry run)
	if _, err := os.Stat(tmpFile); !os.IsNotExist(err) {
		t.Error("Expected file to not be created in dry run")
	}
}

func TestExecutor_Idempotency(t *testing.T) {
	executor := NewExecutor()
	ctx := context.Background()

	tmpFile := filepath.Join(os.TempDir(), "test-executor-idempotent.txt")
	// Create file with desired state
	if err := os.WriteFile(tmpFile, []byte("test content"), 0644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}
	defer os.Remove(tmpFile)

	stateFile := &StateFile{
		Path: "test.yaml",
		States: map[string][]StateDeclaration{
			"file": {
				{
					ID:     tmpFile,
					Module: "file",
					State:  "present",
					Parameters: map[string]interface{}{
						"contents": "test content",
					},
				},
			},
		},
	}

	run, err := executor.ExecuteState(ctx, stateFile)
	if err != nil {
		t.Fatalf("ExecuteState failed: %v", err)
	}

	if run.Summary.Changed != 0 {
		t.Errorf("Expected 0 changed (idempotent), got %d", run.Summary.Changed)
	}

	if run.Summary.Unchanged != 1 {
		t.Errorf("Expected 1 unchanged, got %d", run.Summary.Unchanged)
	}
}

func TestExecutor_MultipleStates(t *testing.T) {
	executor := NewExecutor()
	ctx := context.Background()

	tmpFile1 := filepath.Join(os.TempDir(), "test-executor-multi1.txt")
	tmpFile2 := filepath.Join(os.TempDir(), "test-executor-multi2.txt")
	os.Remove(tmpFile1)
	os.Remove(tmpFile2)
	defer os.Remove(tmpFile1)
	defer os.Remove(tmpFile2)

	stateFile := &StateFile{
		Path: "test.yaml",
		States: map[string][]StateDeclaration{
			"file": {
				{
					ID:     tmpFile1,
					Module: "file",
					State:  "present",
					Parameters: map[string]interface{}{
						"contents": "file 1",
					},
				},
				{
					ID:     tmpFile2,
					Module: "file",
					State:  "present",
					Parameters: map[string]interface{}{
						"contents": "file 2",
					},
				},
			},
		},
	}

	run, err := executor.ExecuteState(ctx, stateFile)
	if err != nil {
		t.Fatalf("ExecuteState failed: %v", err)
	}

	if run.Summary.Total != 2 {
		t.Errorf("Expected 2 states, got %d", run.Summary.Total)
	}

	if run.Summary.Succeeded != 2 {
		t.Errorf("Expected 2 succeeded, got %d", run.Summary.Succeeded)
	}

	// Verify both files were created
	content1, err := os.ReadFile(tmpFile1)
	if err != nil {
		t.Fatalf("Failed to read file 1: %v", err)
	}
	if string(content1) != "file 1" {
		t.Errorf("Expected 'file 1', got '%s'", string(content1))
	}

	content2, err := os.ReadFile(tmpFile2)
	if err != nil {
		t.Fatalf("Failed to read file 2: %v", err)
	}
	if string(content2) != "file 2" {
		t.Errorf("Expected 'file 2', got '%s'", string(content2))
	}
}

func TestExecutor_FailHard(t *testing.T) {
	executor := NewExecutor()
	ctx := context.Background()

	tmpFile := filepath.Join(os.TempDir(), "test-executor-failhard.txt")
	defer os.Remove(tmpFile)

	stateFile := &StateFile{
		Path: "test.yaml",
		States: map[string][]StateDeclaration{
			"file": {
				{
					ID:     tmpFile,
					Module: "file",
					State:  "present",
					Parameters: map[string]interface{}{
						"contents": "test",
					},
					FailHard: true,
				},
			},
		},
	}

	// First run should succeed
	run, err := executor.ExecuteState(ctx, stateFile)
	if err != nil {
		t.Fatalf("ExecuteState failed: %v", err)
	}

	if !run.Summary.Success {
		t.Error("Expected success on first run")
	}
}

func TestExecutor_Retry(t *testing.T) {
	executor := NewExecutor()
	ctx := context.Background()

	tmpFile := filepath.Join(os.TempDir(), "test-executor-retry.txt")
	os.Remove(tmpFile)
	defer os.Remove(tmpFile)

	stateFile := &StateFile{
		Path: "test.yaml",
		States: map[string][]StateDeclaration{
			"file": {
				{
					ID:     tmpFile,
					Module: "file",
					State:  "present",
					Parameters: map[string]interface{}{
						"contents": "test",
					},
					Retry: &RetryConfig{
						Attempts:          3,
						Delay:             100 * time.Millisecond,
						BackoffMultiplier: 1.5,
						MaxDelay:          1 * time.Second,
					},
				},
			},
		},
	}

	run, err := executor.ExecuteState(ctx, stateFile)
	if err != nil {
		t.Fatalf("ExecuteState failed: %v", err)
	}

	if !run.Summary.Success {
		t.Error("Expected success")
	}
}

func TestApplyState(t *testing.T) {
	ctx := context.Background()

	tmpFile := filepath.Join(os.TempDir(), "test-applystate.txt")
	os.Remove(tmpFile)
	defer os.Remove(tmpFile)

	stateFile := &StateFile{
		Path: "test.yaml",
		States: map[string][]StateDeclaration{
			"file": {
				{
					ID:     tmpFile,
					Module: "file",
					State:  "present",
					Parameters: map[string]interface{}{
						"contents": "test",
					},
				},
			},
		},
	}

	run, err := ApplyState(ctx, stateFile, false)
	if err != nil {
		t.Fatalf("ApplyState failed: %v", err)
	}

	if !run.Summary.Success {
		t.Error("Expected success")
	}

	// Verify file was created
	if _, err := os.Stat(tmpFile); os.IsNotExist(err) {
		t.Error("Expected file to be created")
	}
}

func TestCheckState(t *testing.T) {
	ctx := context.Background()

	tmpFile := filepath.Join(os.TempDir(), "test-checkstate.txt")
	os.Remove(tmpFile)
	defer os.Remove(tmpFile)

	stateFile := &StateFile{
		Path: "test.yaml",
		States: map[string][]StateDeclaration{
			"file": {
				{
					ID:     tmpFile,
					Module: "file",
					State:  "present",
					Parameters: map[string]interface{}{
						"contents": "test",
					},
				},
			},
		},
	}

	run, err := CheckState(ctx, stateFile)
	if err != nil {
		t.Fatalf("CheckState failed: %v", err)
	}

	if !run.DryRun {
		t.Error("Expected dry run mode")
	}

	// Verify file was NOT created
	if _, err := os.Stat(tmpFile); !os.IsNotExist(err) {
		t.Error("Expected file to not be created in check mode")
	}
}

func TestExecutor_CalculateSummary(t *testing.T) {
	executor := NewExecutor()

	run := &StateRun{
		StartTime: time.Now(),
		Results: []*StateResult{
			{Success: true, Changed: true},
			{Success: true, Changed: false},
			{Success: false, Changed: false},
		},
	}
	run.EndTime = time.Now()

	summary := executor.calculateSummary(run)

	if summary.Total != 3 {
		t.Errorf("Expected 3 total, got %d", summary.Total)
	}

	if summary.Succeeded != 2 {
		t.Errorf("Expected 2 succeeded, got %d", summary.Succeeded)
	}

	if summary.Failed != 1 {
		t.Errorf("Expected 1 failed, got %d", summary.Failed)
	}

	if summary.Changed != 1 {
		t.Errorf("Expected 1 changed, got %d", summary.Changed)
	}

	if summary.Unchanged != 1 {
		t.Errorf("Expected 1 unchanged, got %d", summary.Unchanged)
	}

	if summary.Success {
		t.Error("Expected overall failure (has 1 failed)")
	}
}
