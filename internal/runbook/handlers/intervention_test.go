package handlers

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/shawnbutts/keystone-core/internal/runbook"
	"github.com/shawnbutts/keystone-core/internal/runbook/intervention"
)

// mockInterventionManager implements InterventionManager for testing.
type mockInterventionManager struct {
	createRequest      func(ctx context.Context, config *intervention.Config, executionID, stepName string, metadata map[string]interface{}) (*intervention.Request, error)
	waitForResponse    func(ctx context.Context, requestID string) (*intervention.Request, error)
	getByExecution     func(ctx context.Context, executionID, stepName string) (*intervention.Request, error)
	cancel             func(ctx context.Context, requestID string, reason string) (*intervention.Request, error)
}

func (m *mockInterventionManager) CreateRequest(ctx context.Context, config *intervention.Config, executionID, stepName string, metadata map[string]interface{}) (*intervention.Request, error) {
	if m.createRequest != nil {
		return m.createRequest(ctx, config, executionID, stepName, metadata)
	}
	return &intervention.Request{
		ID:          "req-123",
		ExecutionID: executionID,
		StepName:    stepName,
		Type:        config.Type,
		State:       intervention.InterventionStatePending,
		Title:       config.Title,
	}, nil
}

func (m *mockInterventionManager) WaitForResponse(ctx context.Context, requestID string) (*intervention.Request, error) {
	if m.waitForResponse != nil {
		return m.waitForResponse(ctx, requestID)
	}
	return &intervention.Request{
		ID:    requestID,
		State: intervention.InterventionStateCompleted,
		Response: &intervention.Response{
			Operator:    "operator@example.com",
			Confirmed:   true,
			RespondedAt: time.Now(),
		},
	}, nil
}

func (m *mockInterventionManager) GetRequestByExecution(ctx context.Context, executionID, stepName string) (*intervention.Request, error) {
	if m.getByExecution != nil {
		return m.getByExecution(ctx, executionID, stepName)
	}
	return nil, nil
}

func (m *mockInterventionManager) Cancel(ctx context.Context, requestID string, reason string) (*intervention.Request, error) {
	if m.cancel != nil {
		return m.cancel(ctx, requestID, reason)
	}
	return &intervention.Request{
		ID:    requestID,
		State: intervention.InterventionStateCancelled,
	}, nil
}

func TestPromptHandler_Type(t *testing.T) {
	h := NewPromptHandler(nil)
	if h.Type() != runbook.StepTypePrompt {
		t.Errorf("Type() = %q, want %q", h.Type(), runbook.StepTypePrompt)
	}
}

func TestWaitManualHandler_Type(t *testing.T) {
	h := NewWaitManualHandler(nil)
	if h.Type() != runbook.StepTypeWaitManual {
		t.Errorf("Type() = %q, want %q", h.Type(), runbook.StepTypeWaitManual)
	}
}

func TestConfirmHandler_Type(t *testing.T) {
	h := NewConfirmHandler(nil)
	if h.Type() != runbook.StepTypeConfirm {
		t.Errorf("Type() = %q, want %q", h.Type(), runbook.StepTypeConfirm)
	}
}

func TestInterventionHandler_Validate_Prompt(t *testing.T) {
	h := NewPromptHandler(nil)

	tests := []struct {
		name    string
		config  map[string]interface{}
		wantErr bool
	}{
		{
			name:    "missing title",
			config:  map[string]interface{}{},
			wantErr: true,
		},
		{
			name: "missing prompts",
			config: map[string]interface{}{
				"title": "Test",
			},
			wantErr: true,
		},
		{
			name: "empty prompts",
			config: map[string]interface{}{
				"title":   "Test",
				"prompts": []interface{}{},
			},
			wantErr: true,
		},
		{
			name: "prompt without name",
			config: map[string]interface{}{
				"title": "Test",
				"prompts": []interface{}{
					map[string]interface{}{"type": "text"},
				},
			},
			wantErr: true,
		},
		{
			name: "invalid prompt type",
			config: map[string]interface{}{
				"title": "Test",
				"prompts": []interface{}{
					map[string]interface{}{"name": "field1", "type": "invalid"},
				},
			},
			wantErr: true,
		},
		{
			name: "valid prompt config",
			config: map[string]interface{}{
				"title": "Test",
				"prompts": []interface{}{
					map[string]interface{}{"name": "field1", "type": "text"},
					map[string]interface{}{"name": "field2", "type": "number"},
				},
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			step := &runbook.Step{
				Type:   runbook.StepTypePrompt,
				Config: tt.config,
			}
			err := h.Validate(step)
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestInterventionHandler_Validate_Confirm(t *testing.T) {
	h := NewConfirmHandler(nil)

	tests := []struct {
		name    string
		config  map[string]interface{}
		wantErr bool
	}{
		{
			name:    "missing title",
			config:  map[string]interface{}{},
			wantErr: true,
		},
		{
			name: "valid confirm config",
			config: map[string]interface{}{
				"title": "Confirm deployment",
			},
			wantErr: false,
		},
		{
			name: "with optional confirmMessage",
			config: map[string]interface{}{
				"title":          "Confirm deployment",
				"confirmMessage": "Are you sure?",
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			step := &runbook.Step{
				Type:   runbook.StepTypeConfirm,
				Config: tt.config,
			}
			err := h.Validate(step)
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestInterventionHandler_Validate_WaitManual(t *testing.T) {
	h := NewWaitManualHandler(nil)

	tests := []struct {
		name    string
		config  map[string]interface{}
		wantErr bool
	}{
		{
			name:    "missing title",
			config:  map[string]interface{}{},
			wantErr: true,
		},
		{
			name: "valid wait_manual config",
			config: map[string]interface{}{
				"title":       "Manual verification",
				"description": "Please verify the deployment",
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			step := &runbook.Step{
				Type:   runbook.StepTypeWaitManual,
				Config: tt.config,
			}
			err := h.Validate(step)
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestInterventionHandler_Execute_Confirm_Success(t *testing.T) {
	manager := &mockInterventionManager{
		waitForResponse: func(ctx context.Context, requestID string) (*intervention.Request, error) {
			return &intervention.Request{
				ID:    requestID,
				Type:  intervention.InterventionTypeConfirm,
				State: intervention.InterventionStateCompleted,
				Response: &intervention.Response{
					Operator:    "operator@example.com",
					Confirmed:   true,
					Comment:     "Approved",
					RespondedAt: time.Now(),
				},
			}, nil
		},
	}

	h := NewConfirmHandler(manager)
	varCtx := newMockVarContextWithCondition()

	step := &runbook.Step{
		Name: "confirm-step",
		Type: runbook.StepTypeConfirm,
		Config: map[string]interface{}{
			"title":       "Confirm deployment",
			"description": "Please confirm to proceed",
		},
	}

	result, err := h.Execute(context.Background(), step, varCtx)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}

	if !result.Success {
		t.Errorf("Success = false, want true")
	}
	if result.Outputs["request_id"] != "req-123" {
		t.Errorf("request_id = %v, want %q", result.Outputs["request_id"], "req-123")
	}
	if result.Outputs["confirmed"] != true {
		t.Errorf("confirmed = %v, want true", result.Outputs["confirmed"])
	}
}

func TestInterventionHandler_Execute_Confirm_Declined(t *testing.T) {
	manager := &mockInterventionManager{
		waitForResponse: func(ctx context.Context, requestID string) (*intervention.Request, error) {
			return &intervention.Request{
				ID:    requestID,
				Type:  intervention.InterventionTypeConfirm,
				State: intervention.InterventionStateCompleted,
				Response: &intervention.Response{
					Operator:    "operator@example.com",
					Confirmed:   false,
					Comment:     "Not ready",
					RespondedAt: time.Now(),
				},
			}, nil
		},
	}

	h := NewConfirmHandler(manager)
	varCtx := newMockVarContextWithCondition()

	step := &runbook.Step{
		Name: "confirm-step",
		Type: runbook.StepTypeConfirm,
		Config: map[string]interface{}{
			"title": "Confirm deployment",
		},
	}

	result, err := h.Execute(context.Background(), step, varCtx)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}

	if result.Success {
		t.Errorf("Success = true, want false (declined)")
	}
	if result.Outputs["confirmed"] != false {
		t.Errorf("confirmed = %v, want false", result.Outputs["confirmed"])
	}
}

func TestInterventionHandler_Execute_Prompt_Success(t *testing.T) {
	manager := &mockInterventionManager{
		waitForResponse: func(ctx context.Context, requestID string) (*intervention.Request, error) {
			return &intervention.Request{
				ID:    requestID,
				Type:  intervention.InterventionTypePrompt,
				State: intervention.InterventionStateCompleted,
				Response: &intervention.Response{
					Operator: "operator@example.com",
					Values: map[string]interface{}{
						"version":  "1.0.0",
						"replicas": float64(3),
					},
					RespondedAt: time.Now(),
				},
			}, nil
		},
	}

	h := NewPromptHandler(manager)
	varCtx := newMockVarContextWithCondition()

	step := &runbook.Step{
		Name: "prompt-step",
		Type: runbook.StepTypePrompt,
		Config: map[string]interface{}{
			"title": "Enter configuration",
			"prompts": []interface{}{
				map[string]interface{}{"name": "version", "type": "text", "required": true},
				map[string]interface{}{"name": "replicas", "type": "number"},
			},
		},
	}

	result, err := h.Execute(context.Background(), step, varCtx)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}

	if !result.Success {
		t.Errorf("Success = false, want true")
	}

	// Check values are exposed
	values, ok := result.Outputs["values"].(map[string]interface{})
	if !ok {
		t.Fatal("expected values in outputs")
	}
	if values["version"] != "1.0.0" {
		t.Errorf("values[version] = %v, want %q", values["version"], "1.0.0")
	}

	// Check values also at top level
	if result.Outputs["version"] != "1.0.0" {
		t.Errorf("outputs[version] = %v, want %q", result.Outputs["version"], "1.0.0")
	}
	if result.Outputs["replicas"] != float64(3) {
		t.Errorf("outputs[replicas] = %v, want 3", result.Outputs["replicas"])
	}
}

func TestInterventionHandler_Execute_WaitManual_Success(t *testing.T) {
	manager := &mockInterventionManager{
		waitForResponse: func(ctx context.Context, requestID string) (*intervention.Request, error) {
			return &intervention.Request{
				ID:    requestID,
				Type:  intervention.InterventionTypeWaitManual,
				State: intervention.InterventionStateCompleted,
				Response: &intervention.Response{
					Operator:    "operator@example.com",
					Confirmed:   true,
					Comment:     "Verified manually",
					RespondedAt: time.Now(),
				},
			}, nil
		},
	}

	h := NewWaitManualHandler(manager)
	varCtx := newMockVarContextWithCondition()

	step := &runbook.Step{
		Name: "wait-step",
		Type: runbook.StepTypeWaitManual,
		Config: map[string]interface{}{
			"title":       "Manual verification",
			"description": "Please verify the deployment",
		},
	}

	result, err := h.Execute(context.Background(), step, varCtx)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}

	if !result.Success {
		t.Errorf("Success = false, want true")
	}
}

func TestInterventionHandler_Execute_Expired(t *testing.T) {
	manager := &mockInterventionManager{
		waitForResponse: func(ctx context.Context, requestID string) (*intervention.Request, error) {
			return &intervention.Request{
				ID:    requestID,
				Type:  intervention.InterventionTypeConfirm,
				State: intervention.InterventionStateExpired,
			}, nil
		},
	}

	h := NewConfirmHandler(manager)
	varCtx := newMockVarContextWithCondition()

	step := &runbook.Step{
		Name: "confirm-step",
		Type: runbook.StepTypeConfirm,
		Config: map[string]interface{}{
			"title":   "Confirm",
			"timeout": "1h",
		},
	}

	result, err := h.Execute(context.Background(), step, varCtx)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}

	if result.Success {
		t.Errorf("Success = true, want false (expired)")
	}
	if result.Message != "intervention request expired" {
		t.Errorf("Message = %q, want %q", result.Message, "intervention request expired")
	}
}

func TestInterventionHandler_Execute_Cancelled(t *testing.T) {
	manager := &mockInterventionManager{
		waitForResponse: func(ctx context.Context, requestID string) (*intervention.Request, error) {
			return &intervention.Request{
				ID:    requestID,
				Type:  intervention.InterventionTypeConfirm,
				State: intervention.InterventionStateCancelled,
			}, nil
		},
	}

	h := NewConfirmHandler(manager)
	varCtx := newMockVarContextWithCondition()

	step := &runbook.Step{
		Name: "confirm-step",
		Type: runbook.StepTypeConfirm,
		Config: map[string]interface{}{
			"title": "Confirm",
		},
	}

	result, err := h.Execute(context.Background(), step, varCtx)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}

	if result.Success {
		t.Errorf("Success = true, want false (cancelled)")
	}
	if result.Message != "intervention request cancelled" {
		t.Errorf("Message = %q, want %q", result.Message, "intervention request cancelled")
	}
}

func TestInterventionHandler_Execute_NoManager(t *testing.T) {
	h := NewConfirmHandler(nil)
	varCtx := newMockVarContextWithCondition()

	step := &runbook.Step{
		Name: "confirm-step",
		Type: runbook.StepTypeConfirm,
		Config: map[string]interface{}{
			"title": "Confirm",
		},
	}

	result, err := h.Execute(context.Background(), step, varCtx)
	if err == nil {
		t.Error("expected error when manager is nil")
	}
	if result.Success {
		t.Error("expected Success = false")
	}
}

func TestInterventionHandler_Execute_CreateError(t *testing.T) {
	manager := &mockInterventionManager{
		createRequest: func(ctx context.Context, config *intervention.Config, executionID, stepName string, metadata map[string]interface{}) (*intervention.Request, error) {
			return nil, errors.New("create failed")
		},
	}

	h := NewConfirmHandler(manager)
	varCtx := newMockVarContextWithCondition()

	step := &runbook.Step{
		Name: "confirm-step",
		Type: runbook.StepTypeConfirm,
		Config: map[string]interface{}{
			"title": "Confirm",
		},
	}

	result, err := h.Execute(context.Background(), step, varCtx)
	if err == nil {
		t.Error("expected error")
	}
	if result.Success {
		t.Error("expected Success = false")
	}
}

func TestInterventionHandler_Execute_WaitError(t *testing.T) {
	manager := &mockInterventionManager{
		waitForResponse: func(ctx context.Context, requestID string) (*intervention.Request, error) {
			return nil, errors.New("wait failed")
		},
	}

	h := NewConfirmHandler(manager)
	varCtx := newMockVarContextWithCondition()

	step := &runbook.Step{
		Name: "confirm-step",
		Type: runbook.StepTypeConfirm,
		Config: map[string]interface{}{
			"title": "Confirm",
		},
	}

	result, err := h.Execute(context.Background(), step, varCtx)
	if err == nil {
		t.Error("expected error")
	}
	if result.Success {
		t.Error("expected Success = false")
	}
	if result.Outputs["error"] != "wait failed" {
		t.Errorf("error = %v, want %q", result.Outputs["error"], "wait failed")
	}
}

func TestInterventionHandler_Execute_ContextCancelled(t *testing.T) {
	cancelled := false
	manager := &mockInterventionManager{
		waitForResponse: func(ctx context.Context, requestID string) (*intervention.Request, error) {
			return nil, ctx.Err()
		},
		cancel: func(ctx context.Context, requestID string, reason string) (*intervention.Request, error) {
			cancelled = true
			return &intervention.Request{
				ID:    requestID,
				State: intervention.InterventionStateCancelled,
			}, nil
		},
	}

	h := NewConfirmHandler(manager)
	varCtx := newMockVarContextWithCondition()

	step := &runbook.Step{
		Name: "confirm-step",
		Type: runbook.StepTypeConfirm,
		Config: map[string]interface{}{
			"title": "Confirm",
		},
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	result, err := h.Execute(ctx, step, varCtx)
	if err == nil {
		t.Error("expected error when context cancelled")
	}
	if result.Success {
		t.Error("expected Success = false")
	}
	if !cancelled {
		t.Error("expected Cancel to be called")
	}
}

func TestInterventionHandler_Execute_ResumeExisting(t *testing.T) {
	manager := &mockInterventionManager{
		getByExecution: func(ctx context.Context, executionID, stepName string) (*intervention.Request, error) {
			return &intervention.Request{
				ID:          "existing-req",
				ExecutionID: executionID,
				StepName:    stepName,
				Type:        intervention.InterventionTypeConfirm,
				State:       intervention.InterventionStatePending,
			}, nil
		},
		waitForResponse: func(ctx context.Context, requestID string) (*intervention.Request, error) {
			if requestID != "existing-req" {
				t.Errorf("requestID = %q, want %q", requestID, "existing-req")
			}
			return &intervention.Request{
				ID:    requestID,
				Type:  intervention.InterventionTypeConfirm,
				State: intervention.InterventionStateCompleted,
				Response: &intervention.Response{
					Operator:    "operator@example.com",
					Confirmed:   true,
					RespondedAt: time.Now(),
				},
			}, nil
		},
	}

	h := NewConfirmHandler(manager)
	varCtx := newMockVarContextWithCondition()

	step := &runbook.Step{
		Name: "confirm-step",
		Type: runbook.StepTypeConfirm,
		Config: map[string]interface{}{
			"title": "Confirm",
		},
	}

	result, err := h.Execute(context.Background(), step, varCtx)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}

	if !result.Success {
		t.Errorf("Success = false, want true")
	}
	if result.Outputs["request_id"] != "existing-req" {
		t.Errorf("request_id = %v, want %q", result.Outputs["request_id"], "existing-req")
	}
}

func TestInterventionHandler_Execute_AlreadyComplete(t *testing.T) {
	manager := &mockInterventionManager{
		getByExecution: func(ctx context.Context, executionID, stepName string) (*intervention.Request, error) {
			return &intervention.Request{
				ID:          "existing-req",
				ExecutionID: executionID,
				StepName:    stepName,
				Type:        intervention.InterventionTypeConfirm,
				State:       intervention.InterventionStateCompleted,
				Response: &intervention.Response{
					Operator:    "operator@example.com",
					Confirmed:   true,
					RespondedAt: time.Now(),
				},
			}, nil
		},
	}

	h := NewConfirmHandler(manager)
	varCtx := newMockVarContextWithCondition()

	step := &runbook.Step{
		Name: "confirm-step",
		Type: runbook.StepTypeConfirm,
		Config: map[string]interface{}{
			"title": "Confirm",
		},
	}

	result, err := h.Execute(context.Background(), step, varCtx)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}

	if !result.Success {
		t.Errorf("Success = false, want true")
	}
}

func TestInterventionHandler_Execute_WithSelectOptions(t *testing.T) {
	manager := &mockInterventionManager{
		createRequest: func(ctx context.Context, config *intervention.Config, executionID, stepName string, metadata map[string]interface{}) (*intervention.Request, error) {
			// Verify prompts were parsed correctly
			if len(config.Prompts) != 1 {
				t.Errorf("len(Prompts) = %d, want 1", len(config.Prompts))
			}
			if len(config.Prompts[0].Options) != 2 {
				t.Errorf("len(Options) = %d, want 2", len(config.Prompts[0].Options))
			}
			return &intervention.Request{
				ID:    "req-123",
				Type:  config.Type,
				State: intervention.InterventionStatePending,
			}, nil
		},
		waitForResponse: func(ctx context.Context, requestID string) (*intervention.Request, error) {
			return &intervention.Request{
				ID:    requestID,
				Type:  intervention.InterventionTypePrompt,
				State: intervention.InterventionStateCompleted,
				Response: &intervention.Response{
					Operator: "op",
					Values:   map[string]interface{}{"region": "us-east-1"},
					RespondedAt: time.Now(),
				},
			}, nil
		},
	}

	h := NewPromptHandler(manager)
	varCtx := newMockVarContextWithCondition()

	step := &runbook.Step{
		Name: "prompt-step",
		Type: runbook.StepTypePrompt,
		Config: map[string]interface{}{
			"title": "Select region",
			"prompts": []interface{}{
				map[string]interface{}{
					"name": "region",
					"type": "select",
					"options": []interface{}{
						map[string]interface{}{"value": "us-east-1", "label": "US East"},
						map[string]interface{}{"value": "us-west-2", "label": "US West"},
					},
				},
			},
		},
	}

	result, err := h.Execute(context.Background(), step, varCtx)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !result.Success {
		t.Errorf("Success = false, want true")
	}
}

func TestInterventionHandler_Execute_WithValidation(t *testing.T) {
	manager := &mockInterventionManager{
		createRequest: func(ctx context.Context, config *intervention.Config, executionID, stepName string, metadata map[string]interface{}) (*intervention.Request, error) {
			// Verify validation was parsed correctly
			if len(config.Prompts) != 1 {
				t.Errorf("len(Prompts) = %d, want 1", len(config.Prompts))
			}
			if config.Prompts[0].Validation == nil {
				t.Error("expected Validation to be set")
			}
			if *config.Prompts[0].Validation.Min != 1 {
				t.Errorf("Min = %v, want 1", *config.Prompts[0].Validation.Min)
			}
			if *config.Prompts[0].Validation.Max != 100 {
				t.Errorf("Max = %v, want 100", *config.Prompts[0].Validation.Max)
			}
			return &intervention.Request{
				ID:    "req-123",
				Type:  config.Type,
				State: intervention.InterventionStatePending,
			}, nil
		},
		waitForResponse: func(ctx context.Context, requestID string) (*intervention.Request, error) {
			return &intervention.Request{
				ID:    requestID,
				Type:  intervention.InterventionTypePrompt,
				State: intervention.InterventionStateCompleted,
				Response: &intervention.Response{
					Operator:    "op",
					Values:      map[string]interface{}{"count": float64(50)},
					RespondedAt: time.Now(),
				},
			}, nil
		},
	}

	h := NewPromptHandler(manager)
	varCtx := newMockVarContextWithCondition()

	step := &runbook.Step{
		Name: "prompt-step",
		Type: runbook.StepTypePrompt,
		Config: map[string]interface{}{
			"title": "Enter count",
			"prompts": []interface{}{
				map[string]interface{}{
					"name": "count",
					"type": "number",
					"validation": map[string]interface{}{
						"min": float64(1),
						"max": float64(100),
					},
				},
			},
		},
	}

	result, err := h.Execute(context.Background(), step, varCtx)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !result.Success {
		t.Errorf("Success = false, want true")
	}
}
