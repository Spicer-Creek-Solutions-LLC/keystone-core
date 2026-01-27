package handlers

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/shawnbutts/keystone-core/internal/runbook"
	"github.com/shawnbutts/keystone-core/internal/statemgmt"
)

// mockStateExecutor implements StateExecutor for testing.
type mockStateExecutor struct {
	executeFunc func(ctx context.Context, stateFile *statemgmt.StateFile) (*statemgmt.StateRun, error)
}

func (m *mockStateExecutor) ExecuteState(ctx context.Context, stateFile *statemgmt.StateFile) (*statemgmt.StateRun, error) {
	if m.executeFunc != nil {
		return m.executeFunc(ctx, stateFile)
	}
	return &statemgmt.StateRun{
		RunID:     "test-run-id",
		StartTime: time.Now(),
		EndTime:   time.Now(),
		Summary: &statemgmt.RunSummary{
			Total:     1,
			Succeeded: 1,
			Changed:   0,
			Success:   true,
		},
		Results: []*statemgmt.StateResult{
			{
				StateID: "file_test",
				Module:  "file",
				Success: true,
				Comment: "File is present",
			},
		},
	}, nil
}

func TestStateHandler_Type(t *testing.T) {
	h := NewStateHandler(nil)
	if got := h.Type(); got != runbook.StepTypeState {
		t.Errorf("Type() = %v, want %v", got, runbook.StepTypeState)
	}
}

func TestStateHandler_Validate(t *testing.T) {
	h := NewStateHandler(nil)

	tests := []struct {
		name    string
		config  map[string]interface{}
		wantErr bool
	}{
		{
			name: "valid inline config",
			config: map[string]interface{}{
				"inline": "file:\n  /tmp/test:\n    state: present\n",
			},
			wantErr: false,
		},
		{
			name: "valid file config",
			config: map[string]interface{}{
				"file": "/path/to/states.yaml",
			},
			wantErr: false,
		},
		{
			name:    "missing inline and file",
			config:  map[string]interface{}{},
			wantErr: true,
		},
		{
			name: "both inline and file",
			config: map[string]interface{}{
				"inline": "file:\n  /tmp/test:\n    state: present\n",
				"file":   "/path/to/states.yaml",
			},
			wantErr: true,
		},
		{
			name: "inline not string",
			config: map[string]interface{}{
				"inline": 123,
			},
			wantErr: true,
		},
		{
			name: "invalid inline YAML",
			config: map[string]interface{}{
				"inline": "invalid: yaml: content:",
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			step := &runbook.Step{
				Name:   "test",
				Type:   runbook.StepTypeState,
				Config: tt.config,
			}

			err := h.Validate(step)
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestStateHandler_Execute(t *testing.T) {
	t.Run("successful execution", func(t *testing.T) {
		executor := &mockStateExecutor{}
		h := NewStateHandler(executor)
		varCtx := newMockVariableContext()

		step := &runbook.Step{
			Name: "apply-config",
			Type: runbook.StepTypeState,
			Config: map[string]interface{}{
				"inline": "file:\n  /tmp/test:\n    state: present\n",
			},
		}

		result, err := h.Execute(context.Background(), step, varCtx)
		if err != nil {
			t.Fatalf("Execute() error = %v", err)
		}

		if !result.Success {
			t.Errorf("Expected success, got failure: %s", result.Message)
		}

		if result.Outputs["run_id"] != "test-run-id" {
			t.Errorf("Expected run_id = test-run-id, got %v", result.Outputs["run_id"])
		}
	})

	t.Run("execution failure", func(t *testing.T) {
		executor := &mockStateExecutor{
			executeFunc: func(ctx context.Context, stateFile *statemgmt.StateFile) (*statemgmt.StateRun, error) {
				return &statemgmt.StateRun{
					RunID: "failed-run",
					Summary: &statemgmt.RunSummary{
						Total:   1,
						Failed:  1,
						Success: false,
					},
				}, errors.New("state execution failed")
			},
		}
		h := NewStateHandler(executor)
		varCtx := newMockVariableContext()

		step := &runbook.Step{
			Name: "apply-config",
			Type: runbook.StepTypeState,
			Config: map[string]interface{}{
				"inline": "file:\n  /tmp/test:\n    state: present\n",
			},
		}

		result, err := h.Execute(context.Background(), step, varCtx)
		if err == nil {
			t.Fatal("Expected error, got nil")
		}

		if result.Success {
			t.Error("Expected failure, got success")
		}
	})

	t.Run("no executor configured", func(t *testing.T) {
		h := NewStateHandler(nil)
		varCtx := newMockVariableContext()

		step := &runbook.Step{
			Name: "apply-config",
			Type: runbook.StepTypeState,
			Config: map[string]interface{}{
				"inline": "file:\n  /tmp/test:\n    state: present\n",
			},
		}

		result, err := h.Execute(context.Background(), step, varCtx)
		if err == nil {
			t.Fatal("Expected error, got nil")
		}

		if result.Success {
			t.Error("Expected failure, got success")
		}
	})

	t.Run("with changes", func(t *testing.T) {
		executor := &mockStateExecutor{
			executeFunc: func(ctx context.Context, stateFile *statemgmt.StateFile) (*statemgmt.StateRun, error) {
				return &statemgmt.StateRun{
					RunID:     "run-with-changes",
					StartTime: time.Now(),
					EndTime:   time.Now(),
					Summary: &statemgmt.RunSummary{
						Total:     2,
						Succeeded: 2,
						Changed:   1,
						Unchanged: 1,
						Success:   true,
					},
					Results: []*statemgmt.StateResult{
						{
							StateID: "file_config",
							Module:  "file",
							Success: true,
							Changed: true,
							Comment: "File created",
							Changes: map[string]interface{}{
								"old": nil,
								"new": "/etc/config.yaml",
							},
						},
						{
							StateID: "pkg_nginx",
							Module:  "pkg",
							Success: true,
							Changed: false,
							Comment: "Package already installed",
						},
					},
				}, nil
			},
		}
		h := NewStateHandler(executor)
		varCtx := newMockVariableContext()

		step := &runbook.Step{
			Name: "apply-config",
			Type: runbook.StepTypeState,
			Config: map[string]interface{}{
				"inline": "file:\n  /etc/config.yaml:\n    state: present\n",
			},
		}

		result, err := h.Execute(context.Background(), step, varCtx)
		if err != nil {
			t.Fatalf("Execute() error = %v", err)
		}

		if !result.Success {
			t.Errorf("Expected success, got failure")
		}

		if result.Outputs["changed"] != 1 {
			t.Errorf("Expected changed = 1, got %v", result.Outputs["changed"])
		}
	})

	t.Run("context cancellation", func(t *testing.T) {
		executor := &mockStateExecutor{
			executeFunc: func(ctx context.Context, stateFile *statemgmt.StateFile) (*statemgmt.StateRun, error) {
				return nil, ctx.Err()
			},
		}
		h := NewStateHandler(executor)
		varCtx := newMockVariableContext()

		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		step := &runbook.Step{
			Name: "apply-config",
			Type: runbook.StepTypeState,
			Config: map[string]interface{}{
				"inline": "file:\n  /tmp/test:\n    state: present\n",
			},
		}

		_, err := h.Execute(ctx, step, varCtx)
		if err == nil {
			t.Fatal("Expected error for cancelled context")
		}
	})
}

func TestStateHandler_BuildStateFile(t *testing.T) {
	t.Run("inline YAML parsing", func(t *testing.T) {
		executor := &mockStateExecutor{
			executeFunc: func(ctx context.Context, stateFile *statemgmt.StateFile) (*statemgmt.StateRun, error) {
				// Verify state file was parsed correctly
				if len(stateFile.States) == 0 {
					return nil, errors.New("no states parsed from inline YAML")
				}
				return &statemgmt.StateRun{
					RunID: "test",
					Summary: &statemgmt.RunSummary{
						Total:   1,
						Success: true,
					},
				}, nil
			},
		}
		h := NewStateHandler(executor)
		varCtx := newMockVariableContext()

		step := &runbook.Step{
			Name: "test-inline",
			Type: runbook.StepTypeState,
			Config: map[string]interface{}{
				"inline": "file:\n  /tmp/test.txt:\n    state: present\n    contents: hello\n",
			},
		}

		result, err := h.Execute(context.Background(), step, varCtx)
		if err != nil {
			t.Fatalf("Execute() error = %v", err)
		}

		if !result.Success {
			t.Errorf("Expected success, got failure")
		}
	})

	t.Run("with target", func(t *testing.T) {
		var capturedTarget string
		executor := &mockStateExecutor{
			executeFunc: func(ctx context.Context, stateFile *statemgmt.StateFile) (*statemgmt.StateRun, error) {
				// Target is stored in Variables["__target__"]
				if stateFile.Variables != nil {
					if t, ok := stateFile.Variables["__target__"].(string); ok {
						capturedTarget = t
					}
				}
				return &statemgmt.StateRun{
					RunID: "test",
					Summary: &statemgmt.RunSummary{
						Total:   1,
						Success: true,
					},
				}, nil
			},
		}
		h := NewStateHandler(executor)
		varCtx := newMockVariableContext()

		step := &runbook.Step{
			Name: "test-target",
			Type: runbook.StepTypeState,
			Config: map[string]interface{}{
				"inline": "file:\n  /tmp/test:\n    state: present\n",
				"target": "os:linux",
			},
		}

		_, err := h.Execute(context.Background(), step, varCtx)
		if err != nil {
			t.Fatalf("Execute() error = %v", err)
		}

		if capturedTarget != "os:linux" {
			t.Errorf("Expected target = os:linux, got %s", capturedTarget)
		}
	})
}
