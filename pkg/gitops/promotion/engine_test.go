package promotion

import (
	"context"
	"testing"
	"time"
)

// MockDeployer for testing
type MockDeployer struct {
	deployErr        error
	currentRevision  string
	setWeightErr     error
	deployCalls      int
	setWeightCalls   int
	lastWeight       int
}

func (m *MockDeployer) Deploy(ctx context.Context, env *Environment, revision string) error {
	m.deployCalls++
	if m.deployErr != nil {
		return m.deployErr
	}
	m.currentRevision = revision
	return nil
}

func (m *MockDeployer) GetCurrentRevision(ctx context.Context, env *Environment) (string, error) {
	return m.currentRevision, nil
}

func (m *MockDeployer) SetTrafficWeight(ctx context.Context, env *Environment, weight int) error {
	m.setWeightCalls++
	m.lastWeight = weight
	if m.setWeightErr != nil {
		return m.setWeightErr
	}
	return nil
}

func TestEngineRegisterPipeline(t *testing.T) {
	deployer := &MockDeployer{}
	engine := NewEngine(deployer)

	pipeline := &Pipeline{
		Name:        "test-pipeline",
		Application: "test-app",
		Environments: []*Environment{
			{Name: "dev"},
			{Name: "prod"},
		},
		Strategy: StrategyImmediate,
	}

	err := engine.RegisterPipeline(pipeline)
	if err != nil {
		t.Fatalf("RegisterPipeline failed: %v", err)
	}

	retrieved, ok := engine.GetPipeline("test-pipeline")
	if !ok {
		t.Error("Pipeline not found")
	}

	if retrieved.Name != "test-pipeline" {
		t.Errorf("Pipeline name = %s, want test-pipeline", retrieved.Name)
	}
}

func TestEngineRegisterPipelineValidation(t *testing.T) {
	deployer := &MockDeployer{}
	engine := NewEngine(deployer)

	tests := []struct {
		name     string
		pipeline *Pipeline
		wantErr  bool
	}{
		{
			name: "valid pipeline",
			pipeline: &Pipeline{
				Name:         "test",
				Environments: []*Environment{{Name: "dev"}},
			},
			wantErr: false,
		},
		{
			name: "missing name",
			pipeline: &Pipeline{
				Environments: []*Environment{{Name: "dev"}},
			},
			wantErr: true,
		},
		{
			name: "missing environments",
			pipeline: &Pipeline{
				Name:         "test",
				Environments: []*Environment{},
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := engine.RegisterPipeline(tt.pipeline)
			if (err != nil) != tt.wantErr {
				t.Errorf("RegisterPipeline() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestEnginePromoteImmediate(t *testing.T) {
	deployer := &MockDeployer{}
	engine := NewEngine(deployer)

	pipeline := &Pipeline{
		Name:        "test-pipeline",
		Application: "test-app",
		Environments: []*Environment{
			{Name: "dev", AutoPromote: true},
		},
		Strategy: StrategyImmediate,
		Timeout:  30 * time.Second,
	}

	err := engine.RegisterPipeline(pipeline)
	if err != nil {
		t.Fatalf("RegisterPipeline failed: %v", err)
	}

	req := &PromotionRequest{
		Pipeline:      "test-pipeline",
		ToEnvironment: "dev",
		Revision:      "abc123",
		RequestedBy:   "test-user",
		Reason:        "Test deployment",
	}

	ctx := context.Background()
	result, err := engine.Promote(ctx, req)
	if err != nil {
		t.Fatalf("Promote failed: %v", err)
	}

	if result.Status != StatusCompleted {
		t.Errorf("Status = %s, want %s", result.Status, StatusCompleted)
	}

	if deployer.deployCalls != 1 {
		t.Errorf("Deploy calls = %d, want 1", deployer.deployCalls)
	}

	if deployer.currentRevision != "abc123" {
		t.Errorf("Deployed revision = %s, want abc123", deployer.currentRevision)
	}
}

func TestEnginePromoteWithApproval(t *testing.T) {
	deployer := &MockDeployer{}
	engine := NewEngine(deployer)

	pipeline := &Pipeline{
		Name:        "test-pipeline",
		Application: "test-app",
		Environments: []*Environment{
			{
				Name:            "prod",
				RequireApproval: true,
				Approvers:       []string{"admin"},
			},
		},
		Strategy: StrategyImmediate,
		Timeout:  30 * time.Second,
	}

	err := engine.RegisterPipeline(pipeline)
	if err != nil {
		t.Fatalf("RegisterPipeline failed: %v", err)
	}

	req := &PromotionRequest{
		Pipeline:      "test-pipeline",
		ToEnvironment: "prod",
		Revision:      "abc123",
		RequestedBy:   "test-user",
		Reason:        "Test deployment",
	}

	ctx := context.Background()
	result, err := engine.Promote(ctx, req)
	if err != nil {
		t.Fatalf("Promote failed: %v", err)
	}

	if result.Status != StatusPending {
		t.Errorf("Status = %s, want %s", result.Status, StatusPending)
	}

	if result.ApprovalInfo == nil {
		t.Fatal("ApprovalInfo should not be nil")
	}

	if !result.ApprovalInfo.Required {
		t.Error("Approval should be required")
	}

	if deployer.deployCalls != 0 {
		t.Errorf("Deploy should not be called yet, got %d calls", deployer.deployCalls)
	}
}

func TestEngineApprovePromotion(t *testing.T) {
	deployer := &MockDeployer{}
	engine := NewEngine(deployer)

	pipeline := &Pipeline{
		Name:        "test-pipeline",
		Application: "test-app",
		Environments: []*Environment{
			{
				Name:            "prod",
				RequireApproval: true,
			},
		},
		Strategy: StrategyImmediate,
		Timeout:  30 * time.Second,
	}

	err := engine.RegisterPipeline(pipeline)
	if err != nil {
		t.Fatalf("RegisterPipeline failed: %v", err)
	}

	ctx := context.Background()
	result, err := engine.Promote(ctx, &PromotionRequest{
		Pipeline:      "test-pipeline",
		ToEnvironment: "prod",
		Revision:      "abc123",
		RequestedBy:   "test-user",
	})
	if err != nil {
		t.Fatalf("Promote failed: %v", err)
	}

	// Approve promotion
	approvalReq := &ApprovalRequest{
		PromotionID: result.ID,
		Approved:    true,
		ApprovedBy:  "admin",
		Reason:      "Approved for testing",
	}

	err = engine.ApprovePromotion(ctx, approvalReq)
	if err != nil {
		t.Fatalf("ApprovePromotion failed: %v", err)
	}

	// Check result was updated
	updated, ok := engine.GetPromotion(result.ID)
	if !ok {
		t.Fatal("Promotion not found")
	}

	if updated.Status != StatusCompleted {
		t.Errorf("Status = %s, want %s", updated.Status, StatusCompleted)
	}

	if updated.ApprovalInfo.ApprovedBy != "admin" {
		t.Errorf("ApprovedBy = %s, want admin", updated.ApprovalInfo.ApprovedBy)
	}

	if deployer.deployCalls != 1 {
		t.Errorf("Deploy calls = %d, want 1", deployer.deployCalls)
	}
}

func TestEngineRejectPromotion(t *testing.T) {
	deployer := &MockDeployer{}
	engine := NewEngine(deployer)

	pipeline := &Pipeline{
		Name:        "test-pipeline",
		Application: "test-app",
		Environments: []*Environment{
			{
				Name:            "prod",
				RequireApproval: true,
			},
		},
		Strategy: StrategyImmediate,
		Timeout:  30 * time.Second,
	}

	err := engine.RegisterPipeline(pipeline)
	if err != nil {
		t.Fatalf("RegisterPipeline failed: %v", err)
	}

	ctx := context.Background()
	result, err := engine.Promote(ctx, &PromotionRequest{
		Pipeline:      "test-pipeline",
		ToEnvironment: "prod",
		Revision:      "abc123",
		RequestedBy:   "test-user",
	})
	if err != nil {
		t.Fatalf("Promote failed: %v", err)
	}

	// Reject promotion
	approvalReq := &ApprovalRequest{
		PromotionID: result.ID,
		Approved:    false,
		ApprovedBy:  "admin",
		Reason:      "Not ready for production",
	}

	err = engine.ApprovePromotion(ctx, approvalReq)
	if err != nil {
		t.Fatalf("ApprovePromotion failed: %v", err)
	}

	// Check result was updated
	updated, ok := engine.GetPromotion(result.ID)
	if !ok {
		t.Fatal("Promotion not found")
	}

	if updated.Status != StatusRejected {
		t.Errorf("Status = %s, want %s", updated.Status, StatusRejected)
	}

	if updated.ApprovalInfo.RejectedBy != "admin" {
		t.Errorf("RejectedBy = %s, want admin", updated.ApprovalInfo.RejectedBy)
	}

	if deployer.deployCalls != 0 {
		t.Errorf("Deploy should not be called, got %d calls", deployer.deployCalls)
	}
}

func TestEngineCanaryDeployment(t *testing.T) {
	deployer := &MockDeployer{}
	engine := NewEngine(deployer)

	pipeline := &Pipeline{
		Name:        "test-pipeline",
		Application: "test-app",
		Environments: []*Environment{
			{Name: "prod"},
		},
		Strategy: StrategyCanary,
		CanarySteps: []CanaryStep{
			{Weight: 25, Duration: 100 * time.Millisecond},
			{Weight: 50, Duration: 100 * time.Millisecond},
			{Weight: 75, Duration: 100 * time.Millisecond},
		},
		Timeout: 10 * time.Second,
	}

	err := engine.RegisterPipeline(pipeline)
	if err != nil {
		t.Fatalf("RegisterPipeline failed: %v", err)
	}

	ctx := context.Background()
	result, err := engine.Promote(ctx, &PromotionRequest{
		Pipeline:      "test-pipeline",
		ToEnvironment: "prod",
		Revision:      "abc123",
		RequestedBy:   "test-user",
	})
	if err != nil {
		t.Fatalf("Promote failed: %v", err)
	}

	if result.Status != StatusCompleted {
		t.Errorf("Status = %s, want %s", result.Status, StatusCompleted)
	}

	// Should have 4 weight calls (25%, 50%, 75%, 100%)
	if deployer.setWeightCalls != 4 {
		t.Errorf("SetTrafficWeight calls = %d, want 4", deployer.setWeightCalls)
	}

	// Final weight should be 100%
	if deployer.lastWeight != 100 {
		t.Errorf("Last weight = %d, want 100", deployer.lastWeight)
	}

	// Check canary progress
	if len(result.Stages) != 1 {
		t.Fatalf("Stages count = %d, want 1", len(result.Stages))
	}

	stage := result.Stages[0]
	if len(stage.CanaryProgress) != 3 {
		t.Errorf("Canary progress count = %d, want 3", len(stage.CanaryProgress))
	}
}

func TestEngineListPromotions(t *testing.T) {
	deployer := &MockDeployer{}
	engine := NewEngine(deployer)

	pipeline := &Pipeline{
		Name:        "test-pipeline",
		Application: "test-app",
		Environments: []*Environment{
			{Name: "dev"},
		},
		Strategy: StrategyImmediate,
	}

	err := engine.RegisterPipeline(pipeline)
	if err != nil {
		t.Fatalf("RegisterPipeline failed: %v", err)
	}

	ctx := context.Background()

	// Create multiple promotions
	for i := 0; i < 3; i++ {
		_, err := engine.Promote(ctx, &PromotionRequest{
			Pipeline:      "test-pipeline",
			ToEnvironment: "dev",
			Revision:      "abc123",
			RequestedBy:   "test-user",
		})
		if err != nil {
			t.Fatalf("Promote failed: %v", err)
		}
	}

	promotions := engine.ListPromotions()
	if len(promotions) != 3 {
		t.Errorf("Promotions count = %d, want 3", len(promotions))
	}
}

func TestEngineListPendingPromotions(t *testing.T) {
	deployer := &MockDeployer{}
	engine := NewEngine(deployer)

	// Create pipeline with approval
	approvalPipeline := &Pipeline{
		Name:        "approval-pipeline",
		Application: "test-app",
		Environments: []*Environment{
			{Name: "prod", RequireApproval: true},
		},
		Strategy: StrategyImmediate,
	}

	// Create pipeline without approval
	immediatePipeline := &Pipeline{
		Name:        "immediate-pipeline",
		Application: "test-app",
		Environments: []*Environment{
			{Name: "dev"},
		},
		Strategy: StrategyImmediate,
	}

	engine.RegisterPipeline(approvalPipeline)
	engine.RegisterPipeline(immediatePipeline)

	ctx := context.Background()

	// Create pending promotion
	_, err := engine.Promote(ctx, &PromotionRequest{
		Pipeline:      "approval-pipeline",
		ToEnvironment: "prod",
		Revision:      "abc123",
		RequestedBy:   "test-user",
	})
	if err != nil {
		t.Fatalf("Promote failed: %v", err)
	}

	// Create completed promotion
	_, err = engine.Promote(ctx, &PromotionRequest{
		Pipeline:      "immediate-pipeline",
		ToEnvironment: "dev",
		Revision:      "abc123",
		RequestedBy:   "test-user",
	})
	if err != nil {
		t.Fatalf("Promote failed: %v", err)
	}

	pending := engine.ListPendingPromotions()
	if len(pending) != 1 {
		t.Errorf("Pending promotions count = %d, want 1", len(pending))
	}

	if pending[0].Pipeline.Name != "approval-pipeline" {
		t.Errorf("Pending promotion pipeline = %s, want approval-pipeline", pending[0].Pipeline.Name)
	}
}
