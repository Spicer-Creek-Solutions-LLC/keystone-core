package handlers

import (
	"context"
	"errors"
	"testing"

	"github.com/shawnbutts/keystone-core/internal/runbook"
)

func TestLoopHandler(t *testing.T) {
	t.Run("validate_without_items", func(t *testing.T) {
		h := NewLoopHandler(nil)

		step := &runbook.Step{
			Name:   "test-loop",
			Type:   runbook.StepTypeLoop,
			Config: map[string]interface{}{},
		}

		err := h.Validate(step)
		if err == nil {
			t.Error("expected error for missing items")
		}
	})

	t.Run("validate_without_steps", func(t *testing.T) {
		h := NewLoopHandler(nil)

		step := &runbook.Step{
			Name: "test-loop",
			Type: runbook.StepTypeLoop,
			Config: map[string]interface{}{
				"items": "{{ .inputs.servers }}",
			},
		}

		err := h.Validate(step)
		if err == nil {
			t.Error("expected error for missing steps")
		}
	})

	t.Run("validate_with_items_and_steps", func(t *testing.T) {
		h := NewLoopHandler(nil)

		step := &runbook.Step{
			Name: "test-loop",
			Type: runbook.StepTypeLoop,
			Config: map[string]interface{}{
				"items": "{{ .inputs.servers }}",
				"steps": []interface{}{
					map[string]interface{}{
						"name":   "deploy",
						"type":   "command",
						"config": map[string]interface{}{"command": "deploy.sh"},
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
		h := NewLoopHandler(nil)

		step := &runbook.Step{
			Name: "test-loop",
			Type: runbook.StepTypeLoop,
			Config: map[string]interface{}{
				"items":       "{{ .inputs.servers }}",
				"steps":       []interface{}{},
				"maxParallel": -1,
			},
		}

		err := h.Validate(step)
		if err == nil {
			t.Error("expected error for negative maxParallel")
		}
	})

	t.Run("execute_sequential", func(t *testing.T) {
		executor := &mockStepExecutor{}
		h := NewLoopHandler(executor)

		varCtx := newMockVarContextWithCondition()

		step := &runbook.Step{
			Name: "test-loop",
			Type: runbook.StepTypeLoop,
			Config: map[string]interface{}{
				"items": []interface{}{"server1", "server2", "server3"},
				"as":    "server",
				"steps": []interface{}{
					map[string]interface{}{
						"name":   "deploy",
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
		// 3 iterations, 1 step each
		if len(executor.executedSteps) != 3 {
			t.Errorf("expected 3 steps executed, got %d", len(executor.executedSteps))
		}
	})

	t.Run("execute_with_failure", func(t *testing.T) {
		executor := &mockStepExecutor{returnErr: errors.New("step failed")}
		h := NewLoopHandler(executor)

		varCtx := newMockVarContextWithCondition()

		step := &runbook.Step{
			Name: "test-loop",
			Type: runbook.StepTypeLoop,
			Config: map[string]interface{}{
				"items": []interface{}{"server1", "server2"},
				"steps": []interface{}{
					map[string]interface{}{
						"name":   "deploy",
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
		if result.Outputs["failed_count"] != 1 {
			t.Errorf("expected 1 failed, got %v", result.Outputs["failed_count"])
		}
	})

	t.Run("execute_with_continueOnError", func(t *testing.T) {
		executor := &mockStepExecutor{returnErr: errors.New("step failed")}
		h := NewLoopHandler(executor)

		varCtx := newMockVarContextWithCondition()

		step := &runbook.Step{
			Name: "test-loop",
			Type: runbook.StepTypeLoop,
			Config: map[string]interface{}{
				"items":           []interface{}{"server1", "server2", "server3"},
				"continueOnError": true,
				"steps": []interface{}{
					map[string]interface{}{
						"name":   "deploy",
						"type":   "noop",
						"config": map[string]interface{}{},
					},
				},
			},
		}

		result, err := h.Execute(context.Background(), step, varCtx)

		// Should succeed despite failures because continueOnError is true
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}
		if !result.Success {
			t.Errorf("expected success with continueOnError, got failure: %s", result.Message)
		}
		if result.Outputs["completed_count"] != 3 {
			t.Errorf("expected 3 completed, got %v", result.Outputs["completed_count"])
		}
		if result.Outputs["failed_count"] != 3 {
			t.Errorf("expected 3 failed, got %v", result.Outputs["failed_count"])
		}
	})

	t.Run("execute_parallel", func(t *testing.T) {
		executor := &mockStepExecutor{}
		h := NewLoopHandler(executor)

		varCtx := newMockVarContextWithCondition()

		step := &runbook.Step{
			Name: "test-loop",
			Type: runbook.StepTypeLoop,
			Config: map[string]interface{}{
				"items":       []interface{}{"a", "b", "c", "d"},
				"maxParallel": 2,
				"steps": []interface{}{
					map[string]interface{}{
						"name":   "process",
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
		h := NewLoopHandler(executor)

		varCtx := newMockVarContextWithCondition()

		ctx, cancel := context.WithCancel(context.Background())
		cancel() // Cancel immediately

		step := &runbook.Step{
			Name: "test-loop",
			Type: runbook.StepTypeLoop,
			Config: map[string]interface{}{
				"items": []interface{}{"a", "b", "c"},
				"steps": []interface{}{
					map[string]interface{}{
						"name":   "process",
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

func TestLoopVariableContext(t *testing.T) {
	parent := newMockVarContextWithCondition()
	parent.inputs["global"] = "value"

	loopCtx := &loopVariableContext{
		parent:   parent,
		itemVar:  "item",
		item:     "test-item",
		indexVar: "idx",
		index:    2,
		total:    5,
	}

	t.Run("get_item_variable", func(t *testing.T) {
		val, ok := loopCtx.GetInput("item")
		if !ok {
			t.Error("expected item variable to exist")
		}
		if val != "test-item" {
			t.Errorf("expected 'test-item', got %v", val)
		}
	})

	t.Run("get_index_variable", func(t *testing.T) {
		val, ok := loopCtx.GetInput("idx")
		if !ok {
			t.Error("expected index variable to exist")
		}
		if val != 2 {
			t.Errorf("expected 2, got %v", val)
		}
	})

	t.Run("get_loop_first", func(t *testing.T) {
		val, ok := loopCtx.GetInput("loop_first")
		if !ok {
			t.Error("expected loop_first variable to exist")
		}
		if val != false {
			t.Errorf("expected false (index=2), got %v", val)
		}
	})

	t.Run("get_loop_last", func(t *testing.T) {
		val, ok := loopCtx.GetInput("loop_last")
		if !ok {
			t.Error("expected loop_last variable to exist")
		}
		if val != false {
			t.Errorf("expected false (index=2, total=5), got %v", val)
		}
	})

	t.Run("get_loop_total", func(t *testing.T) {
		val, ok := loopCtx.GetInput("loop_total")
		if !ok {
			t.Error("expected loop_total variable to exist")
		}
		if val != 5 {
			t.Errorf("expected 5, got %v", val)
		}
	})

	t.Run("get_parent_input", func(t *testing.T) {
		val, ok := loopCtx.GetInput("global")
		if !ok {
			t.Error("expected global variable to exist")
		}
		if val != "value" {
			t.Errorf("expected 'value', got %v", val)
		}
	})

	t.Run("delegates_to_parent", func(t *testing.T) {
		if loopCtx.ExecutionID() != "test-exec-id" {
			t.Error("expected ExecutionID to delegate to parent")
		}
		if loopCtx.RunbookName() != "test-runbook" {
			t.Error("expected RunbookName to delegate to parent")
		}
	})
}

func TestToSlice(t *testing.T) {
	tests := []struct {
		name     string
		input    interface{}
		wantLen  int
		wantErr  bool
	}{
		{
			name:    "interface slice",
			input:   []interface{}{"a", "b", "c"},
			wantLen: 3,
		},
		{
			name:    "string slice",
			input:   []string{"x", "y"},
			wantLen: 2,
		},
		{
			name:    "int slice",
			input:   []int{1, 2, 3, 4},
			wantLen: 4,
		},
		{
			name:    "float64 slice",
			input:   []float64{1.1, 2.2},
			wantLen: 2,
		},
		{
			name:    "map to key-value pairs",
			input:   map[string]interface{}{"a": 1, "b": 2},
			wantLen: 2,
		},
		{
			name:    "string to characters",
			input:   "abc",
			wantLen: 3,
		},
		{
			name:    "invalid type",
			input:   42,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := toSlice(tt.input)

			if (err != nil) != tt.wantErr {
				t.Errorf("toSlice() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if !tt.wantErr && len(result) != tt.wantLen {
				t.Errorf("toSlice() len = %d, want %d", len(result), tt.wantLen)
			}
		})
	}
}
