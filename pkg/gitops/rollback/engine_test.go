package rollback

import (
	"context"
	"testing"
	"time"
)

// MockExecutor for testing
type MockExecutor struct {
	rollbackType      RollbackType
	executeErr        error
	executeResult     *RollbackResult
	previousRevision  string
	lastKnownGood     string
	getPrevErr        error
	getLastGoodErr    error
}

func (m *MockExecutor) Type() RollbackType {
	return m.rollbackType
}

func (m *MockExecutor) Execute(ctx context.Context, config *RollbackConfig, request *RollbackRequest) (*RollbackResult, error) {
	if m.executeErr != nil {
		return nil, m.executeErr
	}
	if m.executeResult != nil {
		return m.executeResult, nil
	}

	// Determine revision from request or config
	revision := request.OverrideRevision
	if revision == "" {
		revision = config.Revision
	}
	if revision == "" {
		revision = "def456" // Default fallback
	}

	return &RollbackResult{
		PreviousRevision: "abc123",
		CurrentRevision:  revision,
	}, nil
}

func (m *MockExecutor) GetPreviousRevision(ctx context.Context, config *RollbackConfig) (string, error) {
	if m.getPrevErr != nil {
		return "", m.getPrevErr
	}
	if m.previousRevision != "" {
		return m.previousRevision, nil
	}
	return "prev123", nil
}

func (m *MockExecutor) GetLastKnownGood(ctx context.Context, config *RollbackConfig) (string, error) {
	if m.getLastGoodErr != nil {
		return "", m.getLastGoodErr
	}
	if m.lastKnownGood != "" {
		return m.lastKnownGood, nil
	}
	return "good123", nil
}

func TestEngineRegisterExecutor(t *testing.T) {
	engine := NewEngine()
	executor := &MockExecutor{rollbackType: RollbackTypeArgoCD}

	engine.RegisterExecutor(executor)

	engine.mu.RLock()
	registered, ok := engine.executors[RollbackTypeArgoCD]
	engine.mu.RUnlock()

	if !ok {
		t.Error("Executor not registered")
	}

	if registered != executor {
		t.Error("Wrong executor registered")
	}
}

func TestEngineExecuteImmediate(t *testing.T) {
	engine := NewEngine()
	executor := &MockExecutor{rollbackType: RollbackTypeArgoCD}
	engine.RegisterExecutor(executor)

	config := &RollbackConfig{
		Name:            "test-rollback",
		Type:            RollbackTypeArgoCD,
		Strategy:        StrategyPreviousRevision,
		Trigger:         TriggerAutomatic,
		Application:     "test-app",
		RequireApproval: false,
		Timeout:         30 * time.Second,
	}

	request := &RollbackRequest{
		ConfigName:  "test-rollback",
		Reason:      "Test rollback",
		RequestedBy: "test-user",
	}

	ctx := context.Background()
	result, err := engine.Execute(ctx, config, request)
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	if result.Status != StatusCompleted && result.Status != StatusVerified {
		t.Errorf("Status = %s, want %s or %s", result.Status, StatusCompleted, StatusVerified)
	}

	if result.ID == "" {
		t.Error("Result ID should not be empty")
	}
}

func TestEngineExecuteWithApproval(t *testing.T) {
	engine := NewEngine()
	executor := &MockExecutor{rollbackType: RollbackTypeArgoCD}
	engine.RegisterExecutor(executor)

	config := &RollbackConfig{
		Name:            "test-rollback",
		Type:            RollbackTypeArgoCD,
		Strategy:        StrategyPreviousRevision,
		Trigger:         TriggerManual,
		Application:     "test-app",
		RequireApproval: true,
		Approvers:       []string{"admin"},
		Timeout:         30 * time.Second,
	}

	request := &RollbackRequest{
		ConfigName:  "test-rollback",
		Reason:      "Test rollback",
		RequestedBy: "test-user",
	}

	ctx := context.Background()
	result, err := engine.Execute(ctx, config, request)
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	if result.Status != StatusPending {
		t.Errorf("Status = %s, want %s", result.Status, StatusPending)
	}

	if result.ApprovalInfo == nil {
		t.Error("ApprovalInfo should not be nil")
	}

	if !result.ApprovalInfo.Required {
		t.Error("Approval should be required")
	}
}

func TestEngineApproveRollback(t *testing.T) {
	engine := NewEngine()
	executor := &MockExecutor{rollbackType: RollbackTypeArgoCD}
	engine.RegisterExecutor(executor)

	config := &RollbackConfig{
		Name:            "test-rollback",
		Type:            RollbackTypeArgoCD,
		Strategy:        StrategyPreviousRevision,
		Trigger:         TriggerManual,
		Application:     "test-app",
		RequireApproval: true,
		Timeout:         30 * time.Second,
	}

	request := &RollbackRequest{
		ConfigName:  "test-rollback",
		Reason:      "Test rollback",
		RequestedBy: "test-user",
	}

	ctx := context.Background()
	result, err := engine.Execute(ctx, config, request)
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	// Approve the rollback
	approvalReq := &ApprovalRequest{
		RollbackID: result.ID,
		Approved:   true,
		ApprovedBy: "admin",
		Reason:     "Approved for testing",
	}

	err = engine.ApproveRollback(ctx, approvalReq)
	if err != nil {
		t.Fatalf("ApproveRollback failed: %v", err)
	}

	// Check result was updated
	updated, ok := engine.GetRollback(result.ID)
	if !ok {
		t.Fatal("Rollback not found")
	}

	if updated.Status != StatusCompleted && updated.Status != StatusVerified {
		t.Errorf("Status = %s, want %s or %s", updated.Status, StatusCompleted, StatusVerified)
	}

	if updated.ApprovalInfo.ApprovedBy != "admin" {
		t.Errorf("ApprovedBy = %s, want admin", updated.ApprovalInfo.ApprovedBy)
	}
}

func TestEngineRejectRollback(t *testing.T) {
	engine := NewEngine()
	executor := &MockExecutor{rollbackType: RollbackTypeArgoCD}
	engine.RegisterExecutor(executor)

	config := &RollbackConfig{
		Name:            "test-rollback",
		Type:            RollbackTypeArgoCD,
		Strategy:        StrategyPreviousRevision,
		Trigger:         TriggerManual,
		Application:     "test-app",
		RequireApproval: true,
		Timeout:         30 * time.Second,
	}

	request := &RollbackRequest{
		ConfigName:  "test-rollback",
		Reason:      "Test rollback",
		RequestedBy: "test-user",
	}

	ctx := context.Background()
	result, err := engine.Execute(ctx, config, request)
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	// Reject the rollback
	approvalReq := &ApprovalRequest{
		RollbackID: result.ID,
		Approved:   false,
		ApprovedBy: "admin",
		Reason:     "Not safe to rollback",
	}

	err = engine.ApproveRollback(ctx, approvalReq)
	if err != nil {
		t.Fatalf("ApproveRollback failed: %v", err)
	}

	// Check result was updated
	updated, ok := engine.GetRollback(result.ID)
	if !ok {
		t.Fatal("Rollback not found")
	}

	if updated.Status != StatusRejected {
		t.Errorf("Status = %s, want %s", updated.Status, StatusRejected)
	}

	if updated.ApprovalInfo.RejectedBy != "admin" {
		t.Errorf("RejectedBy = %s, want admin", updated.ApprovalInfo.RejectedBy)
	}
}

func TestEngineListRollbacks(t *testing.T) {
	engine := NewEngine()
	executor := &MockExecutor{rollbackType: RollbackTypeArgoCD}
	engine.RegisterExecutor(executor)

	config := &RollbackConfig{
		Name:            "test-rollback",
		Type:            RollbackTypeArgoCD,
		Strategy:        StrategyPreviousRevision,
		Trigger:         TriggerAutomatic,
		Application:     "test-app",
		RequireApproval: false,
		Timeout:         30 * time.Second,
	}

	ctx := context.Background()

	// Execute multiple rollbacks
	for i := 0; i < 3; i++ {
		request := &RollbackRequest{
			ConfigName:  "test-rollback",
			Reason:      "Test rollback",
			RequestedBy: "test-user",
		}
		_, err := engine.Execute(ctx, config, request)
		if err != nil {
			t.Fatalf("Execute failed: %v", err)
		}
	}

	// List all rollbacks
	rollbacks := engine.ListRollbacks()
	if len(rollbacks) != 3 {
		t.Errorf("ListRollbacks count = %d, want 3", len(rollbacks))
	}
}

func TestEngineListPendingRollbacks(t *testing.T) {
	engine := NewEngine()
	executor := &MockExecutor{rollbackType: RollbackTypeArgoCD}
	engine.RegisterExecutor(executor)

	ctx := context.Background()

	// Create pending rollback
	pendingConfig := &RollbackConfig{
		Name:            "pending-rollback",
		Type:            RollbackTypeArgoCD,
		Strategy:        StrategyPreviousRevision,
		Trigger:         TriggerManual,
		Application:     "test-app",
		RequireApproval: true,
		Timeout:         30 * time.Second,
	}

	_, err := engine.Execute(ctx, pendingConfig, &RollbackRequest{
		ConfigName:  "pending-rollback",
		Reason:      "Test",
		RequestedBy: "user",
	})
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	// Create completed rollback
	completedConfig := &RollbackConfig{
		Name:            "completed-rollback",
		Type:            RollbackTypeArgoCD,
		Strategy:        StrategyPreviousRevision,
		Trigger:         TriggerAutomatic,
		Application:     "test-app",
		RequireApproval: false,
		Timeout:         30 * time.Second,
	}

	_, err = engine.Execute(ctx, completedConfig, &RollbackRequest{
		ConfigName:  "completed-rollback",
		Reason:      "Test",
		RequestedBy: "user",
	})
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	// List pending rollbacks
	pending := engine.ListPendingRollbacks()
	if len(pending) != 1 {
		t.Errorf("ListPendingRollbacks count = %d, want 1", len(pending))
	}

	if pending[0].Config.Name != "pending-rollback" {
		t.Errorf("Pending rollback name = %s, want pending-rollback", pending[0].Config.Name)
	}
}

func TestEngineStrategyPrevious(t *testing.T) {
	engine := NewEngine()
	executor := &MockExecutor{
		rollbackType:     RollbackTypeArgoCD,
		previousRevision: "prev123",
	}
	engine.RegisterExecutor(executor)

	config := &RollbackConfig{
		Name:            "test-rollback",
		Type:            RollbackTypeArgoCD,
		Strategy:        StrategyPreviousRevision,
		Trigger:         TriggerAutomatic,
		Application:     "test-app",
		RequireApproval: false,
		Timeout:         30 * time.Second,
	}

	request := &RollbackRequest{
		ConfigName:  "test-rollback",
		Reason:      "Test",
		RequestedBy: "user",
	}

	ctx := context.Background()
	result, err := engine.Execute(ctx, config, request)
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	if result.CurrentRevision != "prev123" {
		t.Errorf("CurrentRevision = %s, want prev123", result.CurrentRevision)
	}
}

func TestEngineStrategyLastKnownGood(t *testing.T) {
	engine := NewEngine()
	executor := &MockExecutor{
		rollbackType:  RollbackTypeArgoCD,
		lastKnownGood: "good123",
	}
	engine.RegisterExecutor(executor)

	config := &RollbackConfig{
		Name:            "test-rollback",
		Type:            RollbackTypeArgoCD,
		Strategy:        StrategyLastKnownGood,
		Trigger:         TriggerAutomatic,
		Application:     "test-app",
		RequireApproval: false,
		Timeout:         30 * time.Second,
	}

	request := &RollbackRequest{
		ConfigName:  "test-rollback",
		Reason:      "Test",
		RequestedBy: "user",
	}

	ctx := context.Background()
	result, err := engine.Execute(ctx, config, request)
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	if result.CurrentRevision != "good123" {
		t.Errorf("CurrentRevision = %s, want good123", result.CurrentRevision)
	}
}

func TestEngineStrategySpecific(t *testing.T) {
	engine := NewEngine()
	executor := &MockExecutor{rollbackType: RollbackTypeArgoCD}
	engine.RegisterExecutor(executor)

	config := &RollbackConfig{
		Name:            "test-rollback",
		Type:            RollbackTypeArgoCD,
		Strategy:        StrategySpecificRevision,
		Trigger:         TriggerAutomatic,
		Application:     "test-app",
		Revision:        "specific123",
		RequireApproval: false,
		Timeout:         30 * time.Second,
	}

	request := &RollbackRequest{
		ConfigName:  "test-rollback",
		Reason:      "Test",
		RequestedBy: "user",
	}

	ctx := context.Background()
	result, err := engine.Execute(ctx, config, request)
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	if result.CurrentRevision != "specific123" {
		t.Errorf("CurrentRevision = %s, want specific123", result.CurrentRevision)
	}
}
