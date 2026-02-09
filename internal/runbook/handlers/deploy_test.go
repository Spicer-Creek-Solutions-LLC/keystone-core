package handlers

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/shawnbutts/keystone-core/internal/gitops/orchestration"
	"github.com/shawnbutts/keystone-core/internal/runbook"
)

// mockDeployOrchestrator implements DeployOrchestrator for testing.
type mockDeployOrchestrator struct {
	plans        map[string]*orchestration.Plan
	executeFunc  func(ctx context.Context, req *orchestration.Request) (*orchestration.Result, error)
	rollbackFunc func(ctx context.Context, orchestrationID string) (*orchestration.Result, error)
}

func newMockDeployOrchestrator() *mockDeployOrchestrator {
	return &mockDeployOrchestrator{
		plans: map[string]*orchestration.Plan{
			"production-deploy": {
				Name:    "production-deploy",
				Version: "v1",
			},
			"staging-deploy": {
				Name:    "staging-deploy",
				Version: "v1",
			},
		},
	}
}

func (m *mockDeployOrchestrator) Execute(ctx context.Context, req *orchestration.Request) (*orchestration.Result, error) {
	if m.executeFunc != nil {
		return m.executeFunc(ctx, req)
	}
	return &orchestration.Result{
		ID:     "orch-123",
		Plan:   m.plans[req.PlanName],
		Status: orchestration.StatusCompleted,
		GroupResults: []*orchestration.GroupResult{
			{
				GroupName: "group-1",
				Status:    orchestration.StatusCompleted,
				Duration:  time.Second,
			},
		},
		ExecutionOrder: []string{"group-1"},
		Duration:       time.Second,
	}, nil
}

func (m *mockDeployOrchestrator) GetPlan(name string) (*orchestration.Plan, bool) {
	plan, ok := m.plans[name]
	return plan, ok
}

func (m *mockDeployOrchestrator) Rollback(ctx context.Context, orchestrationID string) (*orchestration.Result, error) {
	if m.rollbackFunc != nil {
		return m.rollbackFunc(ctx, orchestrationID)
	}
	return &orchestration.Result{
		ID:     "rollback-123",
		Status: orchestration.StatusCompleted,
	}, nil
}

func TestDeployHandler_Type(t *testing.T) {
	h := NewDeployHandler(nil)
	if got := h.Type(); got != runbook.StepTypeDeploy {
		t.Errorf("Type() = %v, want %v", got, runbook.StepTypeDeploy)
	}
}

func TestDeployHandler_Validate(t *testing.T) {
	h := NewDeployHandler(nil)

	tests := []struct {
		name    string
		config  map[string]interface{}
		wantErr bool
	}{
		{
			name: "valid minimal config",
			config: map[string]interface{}{
				"plan": "production-deploy",
			},
			wantErr: false,
		},
		{
			name: "valid full config",
			config: map[string]interface{}{
				"plan": "production-deploy",
				"revisions": map[string]interface{}{
					"frontend": "v2.0.0",
					"backend":  "v1.5.0",
				},
				"dry_run":             false,
				"force":               false,
				"rollback_on_failure": true,
				"timeout":             "30m",
			},
			wantErr: false,
		},
		{
			name:    "missing plan",
			config:  map[string]interface{}{},
			wantErr: true,
		},
		{
			name: "plan not string",
			config: map[string]interface{}{
				"plan": 123,
			},
			wantErr: true,
		},
		{
			name: "revisions not map",
			config: map[string]interface{}{
				"plan":      "production-deploy",
				"revisions": "invalid",
			},
			wantErr: true,
		},
		{
			name: "groups_to_skip not list",
			config: map[string]interface{}{
				"plan":           "production-deploy",
				"groups_to_skip": "not-a-list",
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			step := &runbook.Step{
				Name:   "test",
				Type:   runbook.StepTypeDeploy,
				Config: tt.config,
			}

			err := h.Validate(step)
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestDeployHandler_Execute(t *testing.T) {
	t.Run("successful deployment", func(t *testing.T) {
		orch := newMockDeployOrchestrator()
		h := NewDeployHandler(orch)
		varCtx := newMockVariableContext()

		step := &runbook.Step{
			Name: "deploy-app",
			Type: runbook.StepTypeDeploy,
			Config: map[string]interface{}{
				"plan": "production-deploy",
			},
		}

		result, err := h.Execute(context.Background(), step, varCtx)
		if err != nil {
			t.Fatalf("Execute() error = %v", err)
		}

		if !result.Success {
			t.Errorf("Expected success, got failure: %s", result.Message)
		}

		if result.Outputs["orchestration_id"] != "orch-123" {
			t.Errorf("Expected orchestration_id = orch-123, got %v", result.Outputs["orchestration_id"])
		}

		if result.Outputs["status"] != string(orchestration.StatusCompleted) {
			t.Errorf("Expected status = completed, got %v", result.Outputs["status"])
		}
	})

	t.Run("plan not found", func(t *testing.T) {
		orch := newMockDeployOrchestrator()
		h := NewDeployHandler(orch)
		varCtx := newMockVariableContext()

		step := &runbook.Step{
			Name: "deploy-app",
			Type: runbook.StepTypeDeploy,
			Config: map[string]interface{}{
				"plan": "nonexistent-plan",
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

	t.Run("deployment failure", func(t *testing.T) {
		orch := newMockDeployOrchestrator()
		orch.executeFunc = func(ctx context.Context, req *orchestration.Request) (*orchestration.Result, error) {
			return &orchestration.Result{
				ID:           "orch-failed",
				Status:       orchestration.StatusFailed,
				FailedGroups: []string{"group-1"},
				GroupResults: []*orchestration.GroupResult{
					{
						GroupName: "group-1",
						Status:    orchestration.StatusFailed,
						Error:     "deployment failed",
					},
				},
			}, errors.New("deployment failed")
		}
		h := NewDeployHandler(orch)
		varCtx := newMockVariableContext()

		step := &runbook.Step{
			Name: "deploy-app",
			Type: runbook.StepTypeDeploy,
			Config: map[string]interface{}{
				"plan": "production-deploy",
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

	t.Run("deployment failure with rollback", func(t *testing.T) {
		orch := newMockDeployOrchestrator()
		orch.executeFunc = func(ctx context.Context, req *orchestration.Request) (*orchestration.Result, error) {
			return &orchestration.Result{
				ID:           "orch-failed",
				Status:       orchestration.StatusFailed,
				FailedGroups: []string{"group-1"},
			}, nil
		}
		orch.rollbackFunc = func(ctx context.Context, orchestrationID string) (*orchestration.Result, error) {
			return &orchestration.Result{
				ID:     "rollback-123",
				Status: orchestration.StatusCompleted,
			}, nil
		}
		h := NewDeployHandler(orch)
		varCtx := newMockVariableContext()

		step := &runbook.Step{
			Name: "deploy-app",
			Type: runbook.StepTypeDeploy,
			Config: map[string]interface{}{
				"plan":                "production-deploy",
				"rollback_on_failure": true,
			},
		}

		result, err := h.Execute(context.Background(), step, varCtx)
		if err == nil {
			t.Fatal("Expected error, got nil")
		}

		if result.Success {
			t.Error("Expected failure, got success")
		}

		if result.Outputs["rollback_attempted"] != true {
			t.Error("Expected rollback_attempted = true")
		}

		if result.Outputs["rollback_success"] != true {
			t.Error("Expected rollback_success = true")
		}
	})

	t.Run("no orchestrator configured", func(t *testing.T) {
		h := NewDeployHandler(nil)
		varCtx := newMockVariableContext()

		step := &runbook.Step{
			Name: "deploy-app",
			Type: runbook.StepTypeDeploy,
			Config: map[string]interface{}{
				"plan": "production-deploy",
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

	t.Run("with revisions", func(t *testing.T) {
		var capturedRequest *orchestration.Request
		orch := newMockDeployOrchestrator()
		orch.executeFunc = func(ctx context.Context, req *orchestration.Request) (*orchestration.Result, error) {
			capturedRequest = req
			return &orchestration.Result{
				ID:     "orch-123",
				Plan:   orch.plans[req.PlanName],
				Status: orchestration.StatusCompleted,
			}, nil
		}
		h := NewDeployHandler(orch)
		varCtx := newMockVariableContext()

		step := &runbook.Step{
			Name: "deploy-app",
			Type: runbook.StepTypeDeploy,
			Config: map[string]interface{}{
				"plan": "production-deploy",
				"revisions": map[string]interface{}{
					"frontend": "v2.0.0",
					"backend":  "v1.5.0",
				},
			},
		}

		_, err := h.Execute(context.Background(), step, varCtx)
		if err != nil {
			t.Fatalf("Execute() error = %v", err)
		}

		if capturedRequest.Revisions["frontend"] != "v2.0.0" {
			t.Errorf("Expected frontend revision v2.0.0, got %s", capturedRequest.Revisions["frontend"])
		}

		if capturedRequest.Revisions["backend"] != "v1.5.0" {
			t.Errorf("Expected backend revision v1.5.0, got %s", capturedRequest.Revisions["backend"])
		}
	})

	t.Run("with groups to skip", func(t *testing.T) {
		var capturedRequest *orchestration.Request
		orch := newMockDeployOrchestrator()
		orch.executeFunc = func(ctx context.Context, req *orchestration.Request) (*orchestration.Result, error) {
			capturedRequest = req
			return &orchestration.Result{
				ID:     "orch-123",
				Plan:   orch.plans[req.PlanName],
				Status: orchestration.StatusCompleted,
			}, nil
		}
		h := NewDeployHandler(orch)
		varCtx := newMockVariableContext()

		step := &runbook.Step{
			Name: "deploy-app",
			Type: runbook.StepTypeDeploy,
			Config: map[string]interface{}{
				"plan":           "production-deploy",
				"groups_to_skip": []interface{}{"database", "cache"},
			},
		}

		_, err := h.Execute(context.Background(), step, varCtx)
		if err != nil {
			t.Fatalf("Execute() error = %v", err)
		}

		if len(capturedRequest.GroupsToSkip) != 2 {
			t.Errorf("Expected 2 groups to skip, got %d", len(capturedRequest.GroupsToSkip))
		}
	})

	t.Run("context cancellation", func(t *testing.T) {
		orch := newMockDeployOrchestrator()
		orch.executeFunc = func(ctx context.Context, req *orchestration.Request) (*orchestration.Result, error) {
			return nil, ctx.Err()
		}
		h := NewDeployHandler(orch)
		varCtx := newMockVariableContext()

		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		step := &runbook.Step{
			Name: "deploy-app",
			Type: runbook.StepTypeDeploy,
			Config: map[string]interface{}{
				"plan": "production-deploy",
			},
		}

		_, err := h.Execute(ctx, step, varCtx)
		if err == nil {
			t.Fatal("Expected error for cancelled context")
		}
	})
}

func TestRollbackHandler_Type(t *testing.T) {
	h := NewRollbackHandler(nil)
	if got := h.Type(); got != runbook.StepTypeRollback {
		t.Errorf("Type() = %v, want %v", got, runbook.StepTypeRollback)
	}
}

func TestRollbackHandler_Validate(t *testing.T) {
	h := NewRollbackHandler(nil)

	tests := []struct {
		name    string
		config  map[string]interface{}
		wantErr bool
	}{
		{
			name: "valid config",
			config: map[string]interface{}{
				"orchestration_id": "orch-123",
			},
			wantErr: false,
		},
		{
			name: "with template reference",
			config: map[string]interface{}{
				"orchestration_id": "{{ steps.deploy.outputs.orchestration_id }}",
			},
			wantErr: false,
		},
		{
			name:    "missing orchestration_id",
			config:  map[string]interface{}{},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			step := &runbook.Step{
				Name:   "test",
				Type:   runbook.StepTypeRollback,
				Config: tt.config,
			}

			err := h.Validate(step)
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestRollbackHandler_Execute(t *testing.T) {
	t.Run("successful rollback", func(t *testing.T) {
		orch := newMockDeployOrchestrator()
		h := NewRollbackHandler(orch)
		varCtx := newMockVariableContext()

		step := &runbook.Step{
			Name: "rollback-deploy",
			Type: runbook.StepTypeRollback,
			Config: map[string]interface{}{
				"orchestration_id": "orch-123",
			},
		}

		result, err := h.Execute(context.Background(), step, varCtx)
		if err != nil {
			t.Fatalf("Execute() error = %v", err)
		}

		if !result.Success {
			t.Errorf("Expected success, got failure: %s", result.Message)
		}

		if result.Outputs["original_orchestration_id"] != "orch-123" {
			t.Errorf("Expected original_orchestration_id = orch-123, got %v", result.Outputs["original_orchestration_id"])
		}
	})

	t.Run("rollback failure", func(t *testing.T) {
		orch := newMockDeployOrchestrator()
		orch.rollbackFunc = func(ctx context.Context, orchestrationID string) (*orchestration.Result, error) {
			return nil, errors.New("rollback failed")
		}
		h := NewRollbackHandler(orch)
		varCtx := newMockVariableContext()

		step := &runbook.Step{
			Name: "rollback-deploy",
			Type: runbook.StepTypeRollback,
			Config: map[string]interface{}{
				"orchestration_id": "orch-123",
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

	t.Run("no orchestrator configured", func(t *testing.T) {
		h := NewRollbackHandler(nil)
		varCtx := newMockVariableContext()

		step := &runbook.Step{
			Name: "rollback-deploy",
			Type: runbook.StepTypeRollback,
			Config: map[string]interface{}{
				"orchestration_id": "orch-123",
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
}
