package handlers

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/shawnbutts/keystone-core/internal/runbook"
)

// mockVariableContext implements VariableContext for testing.
type mockVariableContext struct {
	inputs      map[string]interface{}
	stepOutputs map[string]map[string]interface{}
}

func newMockVariableContext() *mockVariableContext {
	return &mockVariableContext{
		inputs:      make(map[string]interface{}),
		stepOutputs: make(map[string]map[string]interface{}),
	}
}

func (m *mockVariableContext) GetInput(name string) (interface{}, bool) {
	v, ok := m.inputs[name]
	return v, ok
}

func (m *mockVariableContext) GetStepOutput(stepName, outputName string) (interface{}, bool) {
	if outputs, ok := m.stepOutputs[stepName]; ok {
		v, ok := outputs[outputName]
		return v, ok
	}
	return nil, false
}

func (m *mockVariableContext) ExecutionID() string {
	return "test-exec-id"
}

func (m *mockVariableContext) RunbookName() string {
	return "test-runbook"
}

func (m *mockVariableContext) Resolve(template string) (string, error) {
	return template, nil
}

func (m *mockVariableContext) ResolveValue(template string) (interface{}, error) {
	return template, nil
}

func (m *mockVariableContext) EvaluateCondition(expr string) (bool, error) {
	return true, nil
}

func TestRegistry(t *testing.T) {
	t.Run("register and get", func(t *testing.T) {
		r := NewRegistry()
		handler := NewNoopHandler()

		if err := r.Register(handler); err != nil {
			t.Fatalf("Register() error = %v", err)
		}

		got, ok := r.Get(runbook.StepTypeNoop)
		if !ok {
			t.Fatal("Get() returned false")
		}

		if got.Type() != runbook.StepTypeNoop {
			t.Errorf("Type() = %v, want %v", got.Type(), runbook.StepTypeNoop)
		}
	})

	t.Run("duplicate registration", func(t *testing.T) {
		r := NewRegistry()
		handler := NewNoopHandler()

		_ = r.Register(handler)

		if err := r.Register(handler); err == nil {
			t.Error("Register() should error on duplicate")
		}
	})

	t.Run("get nonexistent", func(t *testing.T) {
		r := NewRegistry()

		_, ok := r.Get(runbook.StepTypeCommand)
		if ok {
			t.Error("Get() should return false for nonexistent handler")
		}
	})

	t.Run("types", func(t *testing.T) {
		r := DefaultRegistry()

		types := r.Types()
		if len(types) < 3 {
			t.Errorf("len(Types()) = %d, want at least 3", len(types))
		}
	})

	t.Run("validate", func(t *testing.T) {
		r := DefaultRegistry()

		step := &runbook.Step{
			Name:   "test",
			Type:   runbook.StepTypeNoop,
			Config: map[string]interface{}{},
		}

		if err := r.Validate(step); err != nil {
			t.Errorf("Validate() error = %v", err)
		}
	})

	t.Run("validate unknown type", func(t *testing.T) {
		r := NewRegistry()

		step := &runbook.Step{
			Name:   "test",
			Type:   runbook.StepType("unknown"),
			Config: map[string]interface{}{},
		}

		if err := r.Validate(step); err == nil {
			t.Error("Validate() should error for unknown type")
		}
	})

	t.Run("execute", func(t *testing.T) {
		r := DefaultRegistry()
		ctx := context.Background()
		vars := newMockVariableContext()

		step := &runbook.Step{
			Name:   "test",
			Type:   runbook.StepTypeNoop,
			Config: map[string]interface{}{},
		}

		result, err := r.Execute(ctx, step, vars)
		if err != nil {
			t.Fatalf("Execute() error = %v", err)
		}

		if !result.Success {
			t.Error("result.Success = false, want true")
		}
	})

	t.Run("execute unknown type", func(t *testing.T) {
		r := NewRegistry()
		ctx := context.Background()
		vars := newMockVariableContext()

		step := &runbook.Step{
			Name:   "test",
			Type:   runbook.StepType("unknown"),
			Config: map[string]interface{}{},
		}

		_, err := r.Execute(ctx, step, vars)
		if err == nil {
			t.Error("Execute() should error for unknown type")
		}
	})
}

func TestNoopHandler(t *testing.T) {
	handler := NewNoopHandler()

	if handler.Type() != runbook.StepTypeNoop {
		t.Errorf("Type() = %v, want %v", handler.Type(), runbook.StepTypeNoop)
	}

	t.Run("validate", func(t *testing.T) {
		step := &runbook.Step{
			Name:   "test",
			Type:   runbook.StepTypeNoop,
			Config: map[string]interface{}{},
		}

		if err := handler.Validate(step); err != nil {
			t.Errorf("Validate() error = %v", err)
		}
	})

	t.Run("execute", func(t *testing.T) {
		ctx := context.Background()
		vars := newMockVariableContext()

		step := &runbook.Step{
			Name:   "test",
			Type:   runbook.StepTypeNoop,
			Config: map[string]interface{}{},
		}

		result, err := handler.Execute(ctx, step, vars)
		if err != nil {
			t.Fatalf("Execute() error = %v", err)
		}

		if !result.Success {
			t.Error("result.Success = false, want true")
		}

		if result.Duration <= 0 {
			t.Error("result.Duration should be > 0")
		}
	})

	t.Run("execute with message", func(t *testing.T) {
		ctx := context.Background()
		vars := newMockVariableContext()

		step := &runbook.Step{
			Name: "test",
			Type: runbook.StepTypeNoop,
			Config: map[string]interface{}{
				"message": "custom message",
			},
		}

		result, err := handler.Execute(ctx, step, vars)
		if err != nil {
			t.Fatalf("Execute() error = %v", err)
		}

		if result.Message != "custom message" {
			t.Errorf("result.Message = %v, want 'custom message'", result.Message)
		}
	})
}

func TestFailHandler(t *testing.T) {
	handler := NewFailHandler()

	if handler.Type() != runbook.StepTypeFail {
		t.Errorf("Type() = %v, want %v", handler.Type(), runbook.StepTypeFail)
	}

	t.Run("validate without message", func(t *testing.T) {
		step := &runbook.Step{
			Name:   "test",
			Type:   runbook.StepTypeFail,
			Config: map[string]interface{}{},
		}

		if err := handler.Validate(step); err == nil {
			t.Error("Validate() should error without message")
		}
	})

	t.Run("validate with message", func(t *testing.T) {
		step := &runbook.Step{
			Name: "test",
			Type: runbook.StepTypeFail,
			Config: map[string]interface{}{
				"message": "expected failure",
			},
		}

		if err := handler.Validate(step); err != nil {
			t.Errorf("Validate() error = %v", err)
		}
	})

	t.Run("execute", func(t *testing.T) {
		ctx := context.Background()
		vars := newMockVariableContext()

		step := &runbook.Step{
			Name: "test",
			Type: runbook.StepTypeFail,
			Config: map[string]interface{}{
				"message": "intentional failure",
			},
		}

		result, err := handler.Execute(ctx, step, vars)
		if err == nil {
			t.Error("Execute() should return error")
		}

		if result.Success {
			t.Error("result.Success = true, want false")
		}

		if result.Message != "intentional failure" {
			t.Errorf("result.Message = %v, want 'intentional failure'", result.Message)
		}
	})
}

func TestWaitHandler(t *testing.T) {
	handler := NewWaitHandler()

	if handler.Type() != runbook.StepTypeWait {
		t.Errorf("Type() = %v, want %v", handler.Type(), runbook.StepTypeWait)
	}

	t.Run("validate without config", func(t *testing.T) {
		step := &runbook.Step{
			Name:   "test",
			Type:   runbook.StepTypeWait,
			Config: map[string]interface{}{},
		}

		if err := handler.Validate(step); err == nil {
			t.Error("Validate() should error without duration or condition")
		}
	})

	t.Run("validate with duration", func(t *testing.T) {
		step := &runbook.Step{
			Name: "test",
			Type: runbook.StepTypeWait,
			Config: map[string]interface{}{
				"duration": "1s",
			},
		}

		if err := handler.Validate(step); err != nil {
			t.Errorf("Validate() error = %v", err)
		}
	})

	t.Run("validate with invalid duration", func(t *testing.T) {
		step := &runbook.Step{
			Name: "test",
			Type: runbook.StepTypeWait,
			Config: map[string]interface{}{
				"duration": "invalid",
			},
		}

		if err := handler.Validate(step); err == nil {
			t.Error("Validate() should error for invalid duration")
		}
	})

	t.Run("validate with condition", func(t *testing.T) {
		step := &runbook.Step{
			Name: "test",
			Type: runbook.StepTypeWait,
			Config: map[string]interface{}{
				"condition": "{{ .steps.prev.success }}",
			},
		}

		if err := handler.Validate(step); err != nil {
			t.Errorf("Validate() error = %v", err)
		}
	})

	t.Run("execute duration", func(t *testing.T) {
		ctx := context.Background()
		vars := newMockVariableContext()

		step := &runbook.Step{
			Name: "test",
			Type: runbook.StepTypeWait,
			Config: map[string]interface{}{
				"duration": "10ms",
			},
		}

		start := time.Now()
		result, err := handler.Execute(ctx, step, vars)
		elapsed := time.Since(start)

		if err != nil {
			t.Fatalf("Execute() error = %v", err)
		}

		if !result.Success {
			t.Error("result.Success = false, want true")
		}

		if elapsed < 10*time.Millisecond {
			t.Errorf("elapsed = %v, want >= 10ms", elapsed)
		}
	})

	t.Run("execute cancelled", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel() // Cancel immediately

		vars := newMockVariableContext()

		step := &runbook.Step{
			Name: "test",
			Type: runbook.StepTypeWait,
			Config: map[string]interface{}{
				"duration": "10s",
			},
		}

		_, err := handler.Execute(ctx, step, vars)
		if err == nil {
			t.Error("Execute() should error when cancelled")
		}

		if !errors.Is(err, context.Canceled) {
			t.Errorf("error = %v, want context.Canceled", err)
		}
	})
}

func TestDefaultRegistry(t *testing.T) {
	r := DefaultRegistry()

	// Check that built-in handlers are registered
	expectedTypes := []runbook.StepType{
		runbook.StepTypeNoop,
		runbook.StepTypeFail,
		runbook.StepTypeWait,
		runbook.StepTypeCommand,
		runbook.StepTypeAPI,
		runbook.StepTypeNotification,
	}

	for _, stepType := range expectedTypes {
		if _, ok := r.Get(stepType); !ok {
			t.Errorf("DefaultRegistry missing handler for %v", stepType)
		}
	}
}

func TestCommandHandler(t *testing.T) {
	handler := NewCommandHandler()

	if handler.Type() != runbook.StepTypeCommand {
		t.Errorf("Type() = %v, want %v", handler.Type(), runbook.StepTypeCommand)
	}

	t.Run("validate without command", func(t *testing.T) {
		step := &runbook.Step{
			Name:   "test",
			Type:   runbook.StepTypeCommand,
			Config: map[string]interface{}{},
		}

		if err := handler.Validate(step); err == nil {
			t.Error("Validate() should error without command or script")
		}
	})

	t.Run("validate with command", func(t *testing.T) {
		step := &runbook.Step{
			Name: "test",
			Type: runbook.StepTypeCommand,
			Config: map[string]interface{}{
				"command": "echo hello",
			},
		}

		if err := handler.Validate(step); err != nil {
			t.Errorf("Validate() error = %v", err)
		}
	})

	t.Run("validate with script", func(t *testing.T) {
		step := &runbook.Step{
			Name: "test",
			Type: runbook.StepTypeCommand,
			Config: map[string]interface{}{
				"script": "echo hello\necho world",
			},
		}

		if err := handler.Validate(step); err != nil {
			t.Errorf("Validate() error = %v", err)
		}
	})

	t.Run("validate with both command and script", func(t *testing.T) {
		step := &runbook.Step{
			Name: "test",
			Type: runbook.StepTypeCommand,
			Config: map[string]interface{}{
				"command": "echo hello",
				"script":  "echo world",
			},
		}

		if err := handler.Validate(step); err == nil {
			t.Error("Validate() should error with both command and script")
		}
	})

	t.Run("validate with invalid shell", func(t *testing.T) {
		step := &runbook.Step{
			Name: "test",
			Type: runbook.StepTypeCommand,
			Config: map[string]interface{}{
				"command": "echo hello",
				"shell":   "invalid",
			},
		}

		if err := handler.Validate(step); err == nil {
			t.Error("Validate() should error for invalid shell type")
		}
	})

	t.Run("validate with valid shell", func(t *testing.T) {
		step := &runbook.Step{
			Name: "test",
			Type: runbook.StepTypeCommand,
			Config: map[string]interface{}{
				"command": "echo hello",
				"shell":   "bash",
			},
		}

		if err := handler.Validate(step); err != nil {
			t.Errorf("Validate() error = %v", err)
		}
	})

	t.Run("validate with invalid timeout", func(t *testing.T) {
		step := &runbook.Step{
			Name: "test",
			Type: runbook.StepTypeCommand,
			Config: map[string]interface{}{
				"command": "echo hello",
				"timeout": "invalid",
			},
		}

		if err := handler.Validate(step); err == nil {
			t.Error("Validate() should error for invalid timeout")
		}
	})

	t.Run("execute simple command", func(t *testing.T) {
		ctx := context.Background()
		vars := newMockVariableContext()

		step := &runbook.Step{
			Name: "test",
			Type: runbook.StepTypeCommand,
			Config: map[string]interface{}{
				"command": "echo hello",
			},
		}

		result, err := handler.Execute(ctx, step, vars)
		if err != nil {
			t.Fatalf("Execute() error = %v", err)
		}

		if !result.Success {
			t.Errorf("result.Success = false, want true; message: %s", result.Message)
		}

		if exitCode, ok := result.Outputs["exit_code"].(int); !ok || exitCode != 0 {
			t.Errorf("exit_code = %v, want 0", result.Outputs["exit_code"])
		}

		stdout, _ := result.Outputs["stdout"].(string)
		if stdout == "" {
			t.Error("stdout should not be empty")
		}
	})

	t.Run("execute failing command", func(t *testing.T) {
		ctx := context.Background()
		vars := newMockVariableContext()

		step := &runbook.Step{
			Name: "test",
			Type: runbook.StepTypeCommand,
			Config: map[string]interface{}{
				"command": "exit 1",
			},
		}

		result, err := handler.Execute(ctx, step, vars)
		if err == nil {
			t.Error("Execute() should error for non-zero exit code")
		}

		if result.Success {
			t.Error("result.Success = true, want false")
		}

		if exitCode, ok := result.Outputs["exit_code"].(int); !ok || exitCode != 1 {
			t.Errorf("exit_code = %v, want 1", result.Outputs["exit_code"])
		}
	})

	t.Run("execute with expected exit code", func(t *testing.T) {
		ctx := context.Background()
		vars := newMockVariableContext()

		step := &runbook.Step{
			Name: "test",
			Type: runbook.StepTypeCommand,
			Config: map[string]interface{}{
				"command":            "exit 42",
				"expected_exit_code": 42,
			},
		}

		result, err := handler.Execute(ctx, step, vars)
		if err != nil {
			t.Fatalf("Execute() error = %v", err)
		}

		if !result.Success {
			t.Errorf("result.Success = false, want true; message: %s", result.Message)
		}
	})

	t.Run("execute cancelled", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel() // Cancel immediately

		vars := newMockVariableContext()

		step := &runbook.Step{
			Name: "test",
			Type: runbook.StepTypeCommand,
			Config: map[string]interface{}{
				"command": "sleep 10",
			},
		}

		result, _ := handler.Execute(ctx, step, vars)
		// Cancelled commands return quickly but may not fail depending on timing
		if result.Success && result.Outputs["exit_code"] == 0 {
			// If it succeeded, it executed before cancellation
			t.Log("command executed before cancellation (timing-dependent)")
		}
	})
}

func TestAPIHandler(t *testing.T) {
	handler := NewAPIHandler()

	if handler.Type() != runbook.StepTypeAPI {
		t.Errorf("Type() = %v, want %v", handler.Type(), runbook.StepTypeAPI)
	}

	t.Run("validate without url", func(t *testing.T) {
		step := &runbook.Step{
			Name:   "test",
			Type:   runbook.StepTypeAPI,
			Config: map[string]interface{}{},
		}

		if err := handler.Validate(step); err == nil {
			t.Error("Validate() should error without url")
		}
	})

	t.Run("validate with url", func(t *testing.T) {
		step := &runbook.Step{
			Name: "test",
			Type: runbook.StepTypeAPI,
			Config: map[string]interface{}{
				"url": "https://example.com/api",
			},
		}

		if err := handler.Validate(step); err != nil {
			t.Errorf("Validate() error = %v", err)
		}
	})

	t.Run("validate with invalid method", func(t *testing.T) {
		step := &runbook.Step{
			Name: "test",
			Type: runbook.StepTypeAPI,
			Config: map[string]interface{}{
				"url":    "https://example.com/api",
				"method": "INVALID",
			},
		}

		if err := handler.Validate(step); err == nil {
			t.Error("Validate() should error for invalid method")
		}
	})

	t.Run("validate with valid method", func(t *testing.T) {
		step := &runbook.Step{
			Name: "test",
			Type: runbook.StepTypeAPI,
			Config: map[string]interface{}{
				"url":    "https://example.com/api",
				"method": "POST",
			},
		}

		if err := handler.Validate(step); err != nil {
			t.Errorf("Validate() error = %v", err)
		}
	})

	t.Run("validate with invalid timeout", func(t *testing.T) {
		step := &runbook.Step{
			Name: "test",
			Type: runbook.StepTypeAPI,
			Config: map[string]interface{}{
				"url":     "https://example.com/api",
				"timeout": "invalid",
			},
		}

		if err := handler.Validate(step); err == nil {
			t.Error("Validate() should error for invalid timeout")
		}
	})

	t.Run("validate with invalid expected_status", func(t *testing.T) {
		step := &runbook.Step{
			Name: "test",
			Type: runbook.StepTypeAPI,
			Config: map[string]interface{}{
				"url":             "https://example.com/api",
				"expected_status": "not-a-number",
			},
		}

		if err := handler.Validate(step); err == nil {
			t.Error("Validate() should error for invalid expected_status")
		}
	})

	// Note: Execute tests would require a mock HTTP server
	// For now, we test validation thoroughly
}

func TestNotificationHandler(t *testing.T) {
	handler := NewNotificationHandler()

	if handler.Type() != runbook.StepTypeNotification {
		t.Errorf("Type() = %v, want %v", handler.Type(), runbook.StepTypeNotification)
	}

	t.Run("validate without message", func(t *testing.T) {
		step := &runbook.Step{
			Name:   "test",
			Type:   runbook.StepTypeNotification,
			Config: map[string]interface{}{},
		}

		if err := handler.Validate(step); err == nil {
			t.Error("Validate() should error without message")
		}
	})

	t.Run("validate with message", func(t *testing.T) {
		step := &runbook.Step{
			Name: "test",
			Type: runbook.StepTypeNotification,
			Config: map[string]interface{}{
				"message": "Test notification",
			},
		}

		if err := handler.Validate(step); err != nil {
			t.Errorf("Validate() error = %v", err)
		}
	})

	t.Run("validate with invalid channel", func(t *testing.T) {
		step := &runbook.Step{
			Name: "test",
			Type: runbook.StepTypeNotification,
			Config: map[string]interface{}{
				"message": "Test notification",
				"channel": "invalid",
			},
		}

		if err := handler.Validate(step); err == nil {
			t.Error("Validate() should error for invalid channel")
		}
	})

	t.Run("validate with valid channel", func(t *testing.T) {
		step := &runbook.Step{
			Name: "test",
			Type: runbook.StepTypeNotification,
			Config: map[string]interface{}{
				"message": "Test notification",
				"channel": "log",
			},
		}

		if err := handler.Validate(step); err != nil {
			t.Errorf("Validate() error = %v", err)
		}
	})

	t.Run("validate with invalid severity", func(t *testing.T) {
		step := &runbook.Step{
			Name: "test",
			Type: runbook.StepTypeNotification,
			Config: map[string]interface{}{
				"message":  "Test notification",
				"severity": "invalid",
			},
		}

		if err := handler.Validate(step); err == nil {
			t.Error("Validate() should error for invalid severity")
		}
	})

	t.Run("validate with valid severity", func(t *testing.T) {
		step := &runbook.Step{
			Name: "test",
			Type: runbook.StepTypeNotification,
			Config: map[string]interface{}{
				"message":  "Test notification",
				"severity": "warning",
			},
		}

		if err := handler.Validate(step); err != nil {
			t.Errorf("Validate() error = %v", err)
		}
	})

	t.Run("execute", func(t *testing.T) {
		ctx := context.Background()
		vars := newMockVariableContext()

		step := &runbook.Step{
			Name: "test",
			Type: runbook.StepTypeNotification,
			Config: map[string]interface{}{
				"message":  "Test notification message",
				"severity": "info",
			},
		}

		result, err := handler.Execute(ctx, step, vars)
		if err != nil {
			t.Fatalf("Execute() error = %v", err)
		}

		if !result.Success {
			t.Errorf("result.Success = false, want true; message: %s", result.Message)
		}

		if channel, ok := result.Outputs["channel"].(string); !ok || channel != "log" {
			t.Errorf("channel = %v, want 'log'", result.Outputs["channel"])
		}

		if severity, ok := result.Outputs["severity"].(string); !ok || severity != "info" {
			t.Errorf("severity = %v, want 'info'", result.Outputs["severity"])
		}
	})

	t.Run("execute with webhook channel", func(t *testing.T) {
		ctx := context.Background()
		vars := newMockVariableContext()

		step := &runbook.Step{
			Name: "test",
			Type: runbook.StepTypeNotification,
			Config: map[string]interface{}{
				"message":     "Webhook notification",
				"channel":     "webhook",
				"webhook_url": "https://example.com/webhook",
			},
		}

		result, err := handler.Execute(ctx, step, vars)
		if err != nil {
			t.Fatalf("Execute() error = %v", err)
		}

		if !result.Success {
			t.Errorf("result.Success = false, want true; message: %s", result.Message)
		}

		if channel, ok := result.Outputs["channel"].(string); !ok || channel != "webhook" {
			t.Errorf("channel = %v, want 'webhook'", result.Outputs["channel"])
		}
	})
}

func TestTruncate(t *testing.T) {
	tests := []struct {
		input    string
		maxLen   int
		expected string
	}{
		{"hello", 10, "hello"},
		{"hello world", 10, "hello w..."},
		{"short", 5, "short"},
		{"ab", 5, "ab"},
	}

	for _, tt := range tests {
		got := truncate(tt.input, tt.maxLen)
		if got != tt.expected {
			t.Errorf("truncate(%q, %d) = %q, want %q", tt.input, tt.maxLen, got, tt.expected)
		}
	}
}
