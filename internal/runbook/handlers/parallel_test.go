package handlers

import (
	"context"
	"errors"
	"testing"

	"github.com/shawnbutts/keystone-core/internal/runbook"
)

func TestParallelHandler(t *testing.T) {
	t.Run("validate_without_steps", func(t *testing.T) {
		h := NewParallelHandler(nil)

		step := &runbook.Step{
			Name:   "test-parallel",
			Type:   runbook.StepTypeParallel,
			Config: map[string]interface{}{},
		}

		err := h.Validate(step)
		if err == nil {
			t.Error("expected error for missing steps")
		}
	})

	t.Run("validate_with_steps", func(t *testing.T) {
		h := NewParallelHandler(nil)

		step := &runbook.Step{
			Name: "test-parallel",
			Type: runbook.StepTypeParallel,
			Config: map[string]interface{}{
				"steps": []interface{}{
					map[string]interface{}{
						"name":   "step1",
						"type":   "noop",
						"config": map[string]interface{}{},
					},
				},
			},
		}

		err := h.Validate(step)
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}
	})

	t.Run("validate_with_negative_maxParallel", func(t *testing.T) {
		h := NewParallelHandler(nil)

		step := &runbook.Step{
			Name: "test-parallel",
			Type: runbook.StepTypeParallel,
			Config: map[string]interface{}{
				"steps":       []interface{}{},
				"maxParallel": -1,
			},
		}

		err := h.Validate(step)
		if err == nil {
			t.Error("expected error for negative maxParallel")
		}
	})

	t.Run("execute_empty_steps", func(t *testing.T) {
		h := NewParallelHandler(nil)

		varCtx := newMockVarContextWithCondition()

		step := &runbook.Step{
			Name: "test-parallel",
			Type: runbook.StepTypeParallel,
			Config: map[string]interface{}{
				"steps": []interface{}{},
			},
		}

		result, err := h.Execute(context.Background(), step, varCtx)

		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}
		if !result.Success {
			t.Errorf("expected success, got failure: %s", result.Message)
		}
		if result.Outputs["completed_count"] != 0 {
			t.Errorf("expected 0 completed, got %v", result.Outputs["completed_count"])
		}
	})

	t.Run("execute_multiple_steps", func(t *testing.T) {
		executor := &mockStepExecutor{}
		h := NewParallelHandler(executor)

		varCtx := newMockVarContextWithCondition()

		step := &runbook.Step{
			Name: "test-parallel",
			Type: runbook.StepTypeParallel,
			Config: map[string]interface{}{
				"steps": []interface{}{
					map[string]interface{}{
						"name":   "step1",
						"type":   "noop",
						"config": map[string]interface{}{},
					},
					map[string]interface{}{
						"name":   "step2",
						"type":   "noop",
						"config": map[string]interface{}{},
					},
					map[string]interface{}{
						"name":   "step3",
						"type":   "noop",
						"config": map[string]interface{}{},
					},
				},
			},
		}

		result, err := h.Execute(context.Background(), step, varCtx)

		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}
		if !result.Success {
			t.Errorf("expected success, got failure: %s", result.Message)
		}
		if result.Outputs["completed_count"] != 3 {
			t.Errorf("expected 3 completed, got %v", result.Outputs["completed_count"])
		}
		if result.Outputs["failed_count"] != 0 {
			t.Errorf("expected 0 failed, got %v", result.Outputs["failed_count"])
		}
	})

	t.Run("execute_with_failure", func(t *testing.T) {
		executor := &mockStepExecutor{returnErr: errors.New("step failed")}
		h := NewParallelHandler(executor)

		varCtx := newMockVarContextWithCondition()

		step := &runbook.Step{
			Name: "test-parallel",
			Type: runbook.StepTypeParallel,
			Config: map[string]interface{}{
				"steps": []interface{}{
					map[string]interface{}{
						"name":   "step1",
						"type":   "noop",
						"config": map[string]interface{}{},
					},
					map[string]interface{}{
						"name":   "step2",
						"type":   "noop",
						"config": map[string]interface{}{},
					},
				},
			},
		}

		result, err := h.Execute(context.Background(), step, varCtx)

		if err == nil {
			t.Error("expected error")
		}
		if result.Success {
			t.Error("expected failure")
		}
		// All steps should have been attempted (no failFast)
		if result.Outputs["completed_count"] != 2 {
			t.Errorf("expected 2 completed, got %v", result.Outputs["completed_count"])
		}
		if result.Outputs["failed_count"] != 2 {
			t.Errorf("expected 2 failed, got %v", result.Outputs["failed_count"])
		}
	})

	t.Run("execute_with_maxParallel", func(t *testing.T) {
		executor := &mockStepExecutor{}
		h := NewParallelHandler(executor)

		varCtx := newMockVarContextWithCondition()

		step := &runbook.Step{
			Name: "test-parallel",
			Type: runbook.StepTypeParallel,
			Config: map[string]interface{}{
				"maxParallel": 2,
				"steps": []interface{}{
					map[string]interface{}{
						"name":   "step1",
						"type":   "noop",
						"config": map[string]interface{}{},
					},
					map[string]interface{}{
						"name":   "step2",
						"type":   "noop",
						"config": map[string]interface{}{},
					},
					map[string]interface{}{
						"name":   "step3",
						"type":   "noop",
						"config": map[string]interface{}{},
					},
					map[string]interface{}{
						"name":   "step4",
						"type":   "noop",
						"config": map[string]interface{}{},
					},
				},
			},
		}

		result, err := h.Execute(context.Background(), step, varCtx)

		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}
		if !result.Success {
			t.Errorf("expected success, got failure: %s", result.Message)
		}
		if result.Outputs["completed_count"] != 4 {
			t.Errorf("expected 4 completed, got %v", result.Outputs["completed_count"])
		}
	})

	t.Run("execute_cancelled", func(t *testing.T) {
		executor := &mockStepExecutor{}
		h := NewParallelHandler(executor)

		varCtx := newMockVarContextWithCondition()

		ctx, cancel := context.WithCancel(context.Background())
		cancel() // Cancel immediately

		step := &runbook.Step{
			Name: "test-parallel",
			Type: runbook.StepTypeParallel,
			Config: map[string]interface{}{
				"steps": []interface{}{
					map[string]interface{}{
						"name":   "step1",
						"type":   "noop",
						"config": map[string]interface{}{},
					},
				},
			},
		}

		result, err := h.Execute(ctx, step, varCtx)

		if err == nil {
			t.Error("expected cancellation error")
		}
		if result.Success {
			t.Error("expected failure due to cancellation")
		}
	})
}
