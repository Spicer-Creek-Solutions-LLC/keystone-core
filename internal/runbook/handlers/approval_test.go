package handlers

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/shawnbutts/keystone-core/internal/runbook"
	"github.com/shawnbutts/keystone-core/internal/runbook/approval"
)

// mockApprovalManager implements ApprovalManager for testing.
type mockApprovalManager struct {
	mu       sync.Mutex
	requests map[string]*approval.Request
	waitCh   chan *approval.Request
}

func newMockApprovalManager() *mockApprovalManager {
	return &mockApprovalManager{
		requests: make(map[string]*approval.Request),
		waitCh:   make(chan *approval.Request, 1),
	}
}

func (m *mockApprovalManager) CreateRequest(ctx context.Context, config *approval.Config, executionID, stepName string, metadata map[string]interface{}) (*approval.Request, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	req := &approval.Request{
		ID:            fmt.Sprintf("%s-%s", executionID, stepName),
		ExecutionID:   executionID,
		StepName:      stepName,
		State:         approval.RequestStatePending,
		Title:         config.Title,
		Description:   config.Description,
		Approvers:     config.Approvers,
		Mode:          config.GetMode(),
		RequiredCount: config.GetRequiredCount(),
		Timeout:       config.GetTimeout(),
		Metadata:      metadata,
		CreatedAt:     time.Now(),
		UpdatedAt:     time.Now(),
	}

	if req.Timeout > 0 {
		expiresAt := time.Now().Add(req.Timeout)
		req.ExpiresAt = &expiresAt
	}

	m.requests[req.ID] = req
	return req, nil
}

func (m *mockApprovalManager) WaitForApproval(ctx context.Context, requestID string) (*approval.Request, error) {
	m.mu.Lock()
	req, ok := m.requests[requestID]
	m.mu.Unlock()

	if !ok {
		return nil, fmt.Errorf("request not found")
	}

	if req.State.IsTerminal() {
		return req, nil
	}

	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case result := <-m.waitCh:
		return result, nil
	}
}

func (m *mockApprovalManager) GetRequestByExecution(ctx context.Context, executionID, stepName string) (*approval.Request, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	id := fmt.Sprintf("%s-%s", executionID, stepName)
	if req, ok := m.requests[id]; ok {
		return req, nil
	}
	return nil, nil
}

func (m *mockApprovalManager) Cancel(ctx context.Context, requestID string, reason string) (*approval.Request, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	req, ok := m.requests[requestID]
	if !ok {
		return nil, fmt.Errorf("request not found")
	}

	req.State = approval.RequestStateCancelled
	now := time.Now()
	req.CompletedAt = &now

	return req, nil
}

func (m *mockApprovalManager) Approve(requestID string) {
	m.mu.Lock()
	req, ok := m.requests[requestID]
	m.mu.Unlock()

	if ok {
		req.State = approval.RequestStateApproved
		req.Responses = append(req.Responses, approval.Response{
			Approver:    "test-approver",
			Decision:    approval.DecisionApproved,
			RespondedAt: time.Now(),
		})
		now := time.Now()
		req.CompletedAt = &now

		select {
		case m.waitCh <- req:
		default:
		}
	}
}

func (m *mockApprovalManager) Reject(requestID string) {
	m.mu.Lock()
	req, ok := m.requests[requestID]
	m.mu.Unlock()

	if ok {
		req.State = approval.RequestStateRejected
		req.Responses = append(req.Responses, approval.Response{
			Approver:    "test-approver",
			Decision:    approval.DecisionRejected,
			RespondedAt: time.Now(),
		})
		now := time.Now()
		req.CompletedAt = &now

		select {
		case m.waitCh <- req:
		default:
		}
	}
}

func TestApprovalHandler_Type(t *testing.T) {
	h := NewApprovalHandler(nil)
	if h.Type() != runbook.StepTypeApproval {
		t.Errorf("Type() = %v, want %v", h.Type(), runbook.StepTypeApproval)
	}
}

func TestApprovalHandler_Validate(t *testing.T) {
	h := NewApprovalHandler(nil)

	t.Run("valid_config", func(t *testing.T) {
		step := &runbook.Step{
			Name: "approve-deploy",
			Type: runbook.StepTypeApproval,
			Config: map[string]interface{}{
				"title":     "Deploy to production",
				"approvers": []interface{}{"admin@example.com", "ops-team"},
			},
		}

		if err := h.Validate(step); err != nil {
			t.Errorf("unexpected error: %v", err)
		}
	})

	t.Run("missing_title", func(t *testing.T) {
		step := &runbook.Step{
			Name: "approve-deploy",
			Type: runbook.StepTypeApproval,
			Config: map[string]interface{}{
				"approvers": []interface{}{"admin@example.com"},
			},
		}

		if err := h.Validate(step); err == nil {
			t.Error("expected error for missing title")
		}
	})

	t.Run("missing_approvers", func(t *testing.T) {
		step := &runbook.Step{
			Name: "approve-deploy",
			Type: runbook.StepTypeApproval,
			Config: map[string]interface{}{
				"title": "Deploy",
			},
		}

		if err := h.Validate(step); err == nil {
			t.Error("expected error for missing approvers")
		}
	})

	t.Run("empty_approvers", func(t *testing.T) {
		step := &runbook.Step{
			Name: "approve-deploy",
			Type: runbook.StepTypeApproval,
			Config: map[string]interface{}{
				"title":     "Deploy",
				"approvers": []interface{}{},
			},
		}

		if err := h.Validate(step); err == nil {
			t.Error("expected error for empty approvers")
		}
	})

	t.Run("invalid_mode", func(t *testing.T) {
		step := &runbook.Step{
			Name: "approve-deploy",
			Type: runbook.StepTypeApproval,
			Config: map[string]interface{}{
				"title":     "Deploy",
				"approvers": []interface{}{"user1"},
				"mode":      "invalid",
			},
		}

		if err := h.Validate(step); err == nil {
			t.Error("expected error for invalid mode")
		}
	})

	t.Run("count_mode_without_required_count", func(t *testing.T) {
		step := &runbook.Step{
			Name: "approve-deploy",
			Type: runbook.StepTypeApproval,
			Config: map[string]interface{}{
				"title":     "Deploy",
				"approvers": []interface{}{"user1", "user2"},
				"mode":      "count",
			},
		}

		if err := h.Validate(step); err == nil {
			t.Error("expected error for count mode without requiredCount")
		}
	})

	t.Run("invalid_timeout", func(t *testing.T) {
		step := &runbook.Step{
			Name: "approve-deploy",
			Type: runbook.StepTypeApproval,
			Config: map[string]interface{}{
				"title":     "Deploy",
				"approvers": []interface{}{"user1"},
				"timeout":   "not-a-duration",
			},
		}

		if err := h.Validate(step); err == nil {
			t.Error("expected error for invalid timeout")
		}
	})
}

func TestApprovalHandler_Execute_NoManager(t *testing.T) {
	h := NewApprovalHandler(nil)
	varCtx := newMockVarContextWithCondition()

	step := &runbook.Step{
		Name: "approve-deploy",
		Type: runbook.StepTypeApproval,
		Config: map[string]interface{}{
			"title":     "Deploy",
			"approvers": []interface{}{"user1"},
		},
	}

	result, err := h.Execute(context.Background(), step, varCtx)

	if err == nil {
		t.Error("expected error for missing manager")
	}
	if result.Success {
		t.Error("expected failure")
	}
}

func TestApprovalHandler_Execute_Approved(t *testing.T) {
	manager := newMockApprovalManager()
	h := NewApprovalHandler(manager)
	varCtx := newMockVarContextWithCondition()

	step := &runbook.Step{
		Name: "approve-deploy",
		Type: runbook.StepTypeApproval,
		Config: map[string]interface{}{
			"title":       "Deploy to production",
			"description": "Please review the deployment",
			"approvers":   []interface{}{"admin@example.com"},
		},
	}

	// Start execution in goroutine
	var result *runbook.StepResult
	var execErr error
	done := make(chan struct{})

	go func() {
		result, execErr = h.Execute(context.Background(), step, varCtx)
		close(done)
	}()

	// Give time for request to be created
	time.Sleep(10 * time.Millisecond)

	// Approve the request
	requestID := fmt.Sprintf("%s-%s", varCtx.ExecutionID(), step.Name)
	manager.Approve(requestID)

	// Wait for completion
	select {
	case <-done:
		if execErr != nil {
			t.Fatalf("Execute error: %v", execErr)
		}
		if !result.Success {
			t.Errorf("expected success, got failure: %s", result.Message)
		}
		if result.Outputs["state"] != "approved" {
			t.Errorf("state = %v, want %q", result.Outputs["state"], "approved")
		}
	case <-time.After(time.Second):
		t.Fatal("Execute timed out")
	}
}

func TestApprovalHandler_Execute_Rejected(t *testing.T) {
	manager := newMockApprovalManager()
	h := NewApprovalHandler(manager)
	varCtx := newMockVarContextWithCondition()

	step := &runbook.Step{
		Name: "approve-deploy",
		Type: runbook.StepTypeApproval,
		Config: map[string]interface{}{
			"title":     "Deploy",
			"approvers": []interface{}{"user1"},
		},
	}

	var result *runbook.StepResult
	done := make(chan struct{})

	go func() {
		result, _ = h.Execute(context.Background(), step, varCtx)
		close(done)
	}()

	time.Sleep(10 * time.Millisecond)

	requestID := fmt.Sprintf("%s-%s", varCtx.ExecutionID(), step.Name)
	manager.Reject(requestID)

	select {
	case <-done:
		if result.Success {
			t.Error("expected failure for rejected approval")
		}
		if result.Outputs["state"] != "rejected" {
			t.Errorf("state = %v, want %q", result.Outputs["state"], "rejected")
		}
	case <-time.After(time.Second):
		t.Fatal("Execute timed out")
	}
}

func TestApprovalHandler_Execute_ContextCancelled(t *testing.T) {
	manager := newMockApprovalManager()
	h := NewApprovalHandler(manager)
	varCtx := newMockVarContextWithCondition()

	step := &runbook.Step{
		Name: "approve-deploy",
		Type: runbook.StepTypeApproval,
		Config: map[string]interface{}{
			"title":     "Deploy",
			"approvers": []interface{}{"user1"},
		},
	}

	ctx, cancel := context.WithCancel(context.Background())

	var execErr error
	done := make(chan struct{})

	go func() {
		_, execErr = h.Execute(ctx, step, varCtx)
		close(done)
	}()

	time.Sleep(10 * time.Millisecond)
	cancel()

	select {
	case <-done:
		if execErr == nil {
			t.Error("expected error when context cancelled")
		}
	case <-time.After(time.Second):
		t.Fatal("Execute should have returned after context cancellation")
	}
}

func TestApprovalHandler_Execute_WithTemplates(t *testing.T) {
	manager := newMockApprovalManager()
	h := NewApprovalHandler(manager)
	varCtx := newMockVarContextWithCondition()
	varCtx.inputs["environment"] = "production"
	varCtx.inputs["approver_email"] = "admin@example.com"

	step := &runbook.Step{
		Name: "approve-deploy",
		Type: runbook.StepTypeApproval,
		Config: map[string]interface{}{
			"title":       "Deploy to {{ .inputs.environment }}",
			"approvers":   []interface{}{"{{ .inputs.approver_email }}"},
		},
	}

	var result *runbook.StepResult
	done := make(chan struct{})

	go func() {
		result, _ = h.Execute(context.Background(), step, varCtx)
		close(done)
	}()

	time.Sleep(10 * time.Millisecond)

	// Approve
	requestID := fmt.Sprintf("%s-%s", varCtx.ExecutionID(), step.Name)
	manager.Approve(requestID)

	select {
	case <-done:
		if !result.Success {
			t.Errorf("expected success, got failure: %s", result.Message)
		}
		// Verify the request was created (handler called buildConfig with template values)
		manager.mu.Lock()
		req := manager.requests[requestID]
		manager.mu.Unlock()
		if req == nil {
			t.Error("request should have been created")
		}
	case <-time.After(time.Second):
		t.Fatal("Execute timed out")
	}
}

func TestApprovalHandler_Execute_WithMode(t *testing.T) {
	tests := []struct {
		name          string
		mode          string
		requiredCount int
	}{
		{"any", "any", 0},
		{"all", "all", 0},
		{"count", "count", 2},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			manager := newMockApprovalManager()
			h := NewApprovalHandler(manager)
			varCtx := newMockVarContextWithCondition()

			config := map[string]interface{}{
				"title":     "Deploy",
				"approvers": []interface{}{"user1", "user2", "user3"},
				"mode":      tt.mode,
			}
			if tt.requiredCount > 0 {
				config["requiredCount"] = tt.requiredCount
			}

			step := &runbook.Step{
				Name:   "approve",
				Type:   runbook.StepTypeApproval,
				Config: config,
			}

			var result *runbook.StepResult
			done := make(chan struct{})

			go func() {
				result, _ = h.Execute(context.Background(), step, varCtx)
				close(done)
			}()

			time.Sleep(10 * time.Millisecond)

			requestID := fmt.Sprintf("%s-%s", varCtx.ExecutionID(), step.Name)
			manager.Approve(requestID)

			select {
			case <-done:
				if !result.Success {
					t.Errorf("expected success")
				}
			case <-time.After(time.Second):
				t.Fatal("Execute timed out")
			}
		})
	}
}
