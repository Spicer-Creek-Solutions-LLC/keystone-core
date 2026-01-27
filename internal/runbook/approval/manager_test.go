package approval

import (
	"context"
	"sync"
	"testing"
	"time"
)

// mockStorage implements Storage for testing.
type mockStorage struct {
	mu       sync.RWMutex
	requests map[string]*Request
}

func newMockStorage() *mockStorage {
	return &mockStorage{
		requests: make(map[string]*Request),
	}
}

func (s *mockStorage) SaveRequest(ctx context.Context, req *Request) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	// Make a copy to avoid mutation
	reqCopy := *req
	reqCopy.Responses = make([]Response, len(req.Responses))
	for i, r := range req.Responses {
		reqCopy.Responses[i] = r
	}
	s.requests[req.ID] = &reqCopy
	return nil
}

func (s *mockStorage) GetRequest(ctx context.Context, id string) (*Request, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if req, ok := s.requests[id]; ok {
		reqCopy := *req
		return &reqCopy, nil
	}
	return nil, nil
}

func (s *mockStorage) GetRequestByExecution(ctx context.Context, executionID, stepName string) (*Request, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, req := range s.requests {
		if req.ExecutionID == executionID && req.StepName == stepName {
			reqCopy := *req
			return &reqCopy, nil
		}
	}
	return nil, nil
}

func (s *mockStorage) ListRequests(ctx context.Context, opts ListOptions) ([]*Request, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var result []*Request
	for _, req := range s.requests {
		if opts.ExecutionID != "" && req.ExecutionID != opts.ExecutionID {
			continue
		}
		if opts.State != "" && req.State != opts.State {
			continue
		}
		reqCopy := *req
		result = append(result, &reqCopy)
	}
	return result, nil
}

func (s *mockStorage) DeleteRequest(ctx context.Context, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.requests, id)
	return nil
}

func TestManager_CreateRequest(t *testing.T) {
	storage := newMockStorage()
	manager := NewManager(storage)

	config := &Config{
		Title:     "Deploy to production",
		Approvers: []string{"admin@example.com", "ops-team"},
		Mode:      ApprovalModeAny,
		Timeout:   "1h",
	}

	req, err := manager.CreateRequest(context.Background(), config, "exec-123", "deploy-approval", nil)
	if err != nil {
		t.Fatalf("CreateRequest failed: %v", err)
	}

	if req.ID == "" {
		t.Error("Request ID should not be empty")
	}
	if req.ExecutionID != "exec-123" {
		t.Errorf("ExecutionID = %q, want %q", req.ExecutionID, "exec-123")
	}
	if req.StepName != "deploy-approval" {
		t.Errorf("StepName = %q, want %q", req.StepName, "deploy-approval")
	}
	if req.State != RequestStatePending {
		t.Errorf("State = %q, want %q", req.State, RequestStatePending)
	}
	if req.Title != "Deploy to production" {
		t.Errorf("Title = %q, want %q", req.Title, "Deploy to production")
	}
	if len(req.Approvers) != 2 {
		t.Errorf("Approvers count = %d, want 2", len(req.Approvers))
	}
	if req.Mode != ApprovalModeAny {
		t.Errorf("Mode = %q, want %q", req.Mode, ApprovalModeAny)
	}
	if req.RequiredCount != 1 {
		t.Errorf("RequiredCount = %d, want 1", req.RequiredCount)
	}
	if req.ExpiresAt == nil {
		t.Error("ExpiresAt should be set when timeout is configured")
	}
}

func TestManager_CreateRequest_Validation(t *testing.T) {
	storage := newMockStorage()
	manager := NewManager(storage)

	t.Run("nil_config", func(t *testing.T) {
		_, err := manager.CreateRequest(context.Background(), nil, "exec-123", "step", nil)
		if err == nil {
			t.Error("expected error for nil config")
		}
	})

	t.Run("no_approvers", func(t *testing.T) {
		config := &Config{
			Title:     "Test",
			Approvers: []string{},
		}
		_, err := manager.CreateRequest(context.Background(), config, "exec-123", "step", nil)
		if err == nil {
			t.Error("expected error for empty approvers")
		}
	})

	t.Run("invalid_mode", func(t *testing.T) {
		config := &Config{
			Title:     "Test",
			Approvers: []string{"user1"},
			Mode:      ApprovalMode("invalid"),
		}
		_, err := manager.CreateRequest(context.Background(), config, "exec-123", "step", nil)
		if err == nil {
			t.Error("expected error for invalid mode")
		}
	})
}

func TestManager_Respond_AnyMode(t *testing.T) {
	storage := newMockStorage()
	manager := NewManager(storage)

	config := &Config{
		Title:     "Test approval",
		Approvers: []string{"user1", "user2"},
		Mode:      ApprovalModeAny,
	}

	req, _ := manager.CreateRequest(context.Background(), config, "exec-123", "step1", nil)

	// Single approval should complete the request
	updated, err := manager.Respond(context.Background(), req.ID, "user1", DecisionApproved, "looks good")
	if err != nil {
		t.Fatalf("Respond failed: %v", err)
	}

	if updated.State != RequestStateApproved {
		t.Errorf("State = %q, want %q", updated.State, RequestStateApproved)
	}
	if len(updated.Responses) != 1 {
		t.Errorf("Responses count = %d, want 1", len(updated.Responses))
	}
	if updated.Responses[0].Comment != "looks good" {
		t.Errorf("Comment = %q, want %q", updated.Responses[0].Comment, "looks good")
	}
}

func TestManager_Respond_AllMode(t *testing.T) {
	storage := newMockStorage()
	manager := NewManager(storage)

	config := &Config{
		Title:     "Test approval",
		Approvers: []string{"user1", "user2"},
		Mode:      ApprovalModeAll,
	}

	req, _ := manager.CreateRequest(context.Background(), config, "exec-123", "step1", nil)

	// First approval - should still be pending
	updated, err := manager.Respond(context.Background(), req.ID, "user1", DecisionApproved, "")
	if err != nil {
		t.Fatalf("First respond failed: %v", err)
	}
	if updated.State != RequestStatePending {
		t.Errorf("State after first approval = %q, want %q", updated.State, RequestStatePending)
	}

	// Second approval - should complete
	updated, err = manager.Respond(context.Background(), req.ID, "user2", DecisionApproved, "")
	if err != nil {
		t.Fatalf("Second respond failed: %v", err)
	}
	if updated.State != RequestStateApproved {
		t.Errorf("State after second approval = %q, want %q", updated.State, RequestStateApproved)
	}
}

func TestManager_Respond_AllMode_Rejection(t *testing.T) {
	storage := newMockStorage()
	manager := NewManager(storage)

	config := &Config{
		Title:     "Test approval",
		Approvers: []string{"user1", "user2"},
		Mode:      ApprovalModeAll,
	}

	req, _ := manager.CreateRequest(context.Background(), config, "exec-123", "step1", nil)

	// Single rejection should reject the entire request in "all" mode
	updated, err := manager.Respond(context.Background(), req.ID, "user1", DecisionRejected, "not ready")
	if err != nil {
		t.Fatalf("Respond failed: %v", err)
	}

	if updated.State != RequestStateRejected {
		t.Errorf("State = %q, want %q", updated.State, RequestStateRejected)
	}
}

func TestManager_Respond_CountMode(t *testing.T) {
	storage := newMockStorage()
	manager := NewManager(storage)

	config := &Config{
		Title:         "Test approval",
		Approvers:     []string{"user1", "user2", "user3"},
		Mode:          ApprovalModeCount,
		RequiredCount: 2,
	}

	req, _ := manager.CreateRequest(context.Background(), config, "exec-123", "step1", nil)

	// First approval - should still be pending
	updated, _ := manager.Respond(context.Background(), req.ID, "user1", DecisionApproved, "")
	if updated.State != RequestStatePending {
		t.Errorf("State after first approval = %q, want %q", updated.State, RequestStatePending)
	}

	// Second approval - should complete (2 required)
	updated, _ = manager.Respond(context.Background(), req.ID, "user2", DecisionApproved, "")
	if updated.State != RequestStateApproved {
		t.Errorf("State after second approval = %q, want %q", updated.State, RequestStateApproved)
	}
}

func TestManager_Respond_Errors(t *testing.T) {
	storage := newMockStorage()
	manager := NewManager(storage)

	config := &Config{
		Title:     "Test approval",
		Approvers: []string{"user1"},
	}

	req, _ := manager.CreateRequest(context.Background(), config, "exec-123", "step1", nil)

	t.Run("not_found", func(t *testing.T) {
		_, err := manager.Respond(context.Background(), "nonexistent", "user1", DecisionApproved, "")
		if err == nil {
			t.Error("expected error for nonexistent request")
		}
	})

	t.Run("not_authorized", func(t *testing.T) {
		_, err := manager.Respond(context.Background(), req.ID, "unauthorized-user", DecisionApproved, "")
		if err == nil {
			t.Error("expected error for unauthorized user")
		}
	})

	t.Run("already_responded", func(t *testing.T) {
		_, _ = manager.Respond(context.Background(), req.ID, "user1", DecisionApproved, "")
		// Try to respond again
		_, err := manager.Respond(context.Background(), req.ID, "user1", DecisionApproved, "")
		if err == nil {
			t.Error("expected error for duplicate response")
		}
	})

	t.Run("not_pending", func(t *testing.T) {
		// Request is now approved, try to respond
		config2 := &Config{
			Title:     "Another",
			Approvers: []string{"user1", "user2"},
			Mode:      ApprovalModeAll,
		}
		req2, _ := manager.CreateRequest(context.Background(), config2, "exec-124", "step1", nil)
		_, _ = manager.Respond(context.Background(), req2.ID, "user1", DecisionApproved, "")
		_, _ = manager.Respond(context.Background(), req2.ID, "user2", DecisionApproved, "")
		// Now try to respond to completed request
		_, err := manager.Respond(context.Background(), req2.ID, "user1", DecisionRejected, "")
		if err == nil {
			t.Error("expected error for responding to non-pending request")
		}
	})
}

func TestManager_Cancel(t *testing.T) {
	storage := newMockStorage()
	manager := NewManager(storage)

	config := &Config{
		Title:     "Test approval",
		Approvers: []string{"user1"},
	}

	req, _ := manager.CreateRequest(context.Background(), config, "exec-123", "step1", nil)

	cancelled, err := manager.Cancel(context.Background(), req.ID, "execution cancelled")
	if err != nil {
		t.Fatalf("Cancel failed: %v", err)
	}

	if cancelled.State != RequestStateCancelled {
		t.Errorf("State = %q, want %q", cancelled.State, RequestStateCancelled)
	}
	if cancelled.CompletedAt == nil {
		t.Error("CompletedAt should be set")
	}
	if cancelled.Metadata["cancel_reason"] != "execution cancelled" {
		t.Errorf("cancel_reason = %v, want %q", cancelled.Metadata["cancel_reason"], "execution cancelled")
	}
}

func TestManager_Cancel_NotPending(t *testing.T) {
	storage := newMockStorage()
	manager := NewManager(storage)

	config := &Config{
		Title:     "Test approval",
		Approvers: []string{"user1"},
	}

	req, _ := manager.CreateRequest(context.Background(), config, "exec-123", "step1", nil)
	_, _ = manager.Respond(context.Background(), req.ID, "user1", DecisionApproved, "")

	_, err := manager.Cancel(context.Background(), req.ID, "too late")
	if err == nil {
		t.Error("expected error when cancelling non-pending request")
	}
}

func TestManager_GetRequest(t *testing.T) {
	storage := newMockStorage()
	manager := NewManager(storage)

	config := &Config{
		Title:     "Test",
		Approvers: []string{"user1"},
	}

	created, _ := manager.CreateRequest(context.Background(), config, "exec-123", "step1", nil)

	retrieved, err := manager.GetRequest(context.Background(), created.ID)
	if err != nil {
		t.Fatalf("GetRequest failed: %v", err)
	}

	if retrieved.ID != created.ID {
		t.Errorf("ID = %q, want %q", retrieved.ID, created.ID)
	}
}

func TestManager_ListPendingRequests(t *testing.T) {
	storage := newMockStorage()
	manager := NewManager(storage)

	// Create a pending request
	config := &Config{
		Title:     "Test",
		Approvers: []string{"user1", "user2"},
	}
	manager.CreateRequest(context.Background(), config, "exec-1", "step1", nil)

	// Create and approve another request
	req2, _ := manager.CreateRequest(context.Background(), config, "exec-2", "step1", nil)
	manager.Respond(context.Background(), req2.ID, "user1", DecisionApproved, "")

	// List pending for user1
	pending, err := manager.ListPendingRequests(context.Background(), "user1")
	if err != nil {
		t.Fatalf("ListPendingRequests failed: %v", err)
	}

	// Should only include the first request (still pending)
	if len(pending) != 1 {
		t.Errorf("Pending count = %d, want 1", len(pending))
	}
}

func TestManager_WaitForApproval(t *testing.T) {
	storage := newMockStorage()
	manager := NewManager(storage)
	defer manager.Stop()

	config := &Config{
		Title:     "Test",
		Approvers: []string{"user1"},
	}

	req, _ := manager.CreateRequest(context.Background(), config, "exec-123", "step1", nil)

	// Start waiting in goroutine
	var waitResult *Request
	var waitErr error
	done := make(chan struct{})

	go func() {
		waitResult, waitErr = manager.WaitForApproval(context.Background(), req.ID)
		close(done)
	}()

	// Give the waiter time to register
	time.Sleep(10 * time.Millisecond)

	// Approve the request
	manager.Respond(context.Background(), req.ID, "user1", DecisionApproved, "")

	// Wait for result
	select {
	case <-done:
		if waitErr != nil {
			t.Fatalf("WaitForApproval failed: %v", waitErr)
		}
		if waitResult.State != RequestStateApproved {
			t.Errorf("State = %q, want %q", waitResult.State, RequestStateApproved)
		}
	case <-time.After(time.Second):
		t.Fatal("WaitForApproval timed out")
	}
}

func TestManager_WaitForApproval_AlreadyComplete(t *testing.T) {
	storage := newMockStorage()
	manager := NewManager(storage)

	config := &Config{
		Title:     "Test",
		Approvers: []string{"user1"},
	}

	req, _ := manager.CreateRequest(context.Background(), config, "exec-123", "step1", nil)
	manager.Respond(context.Background(), req.ID, "user1", DecisionApproved, "")

	// Should return immediately since already complete
	result, err := manager.WaitForApproval(context.Background(), req.ID)
	if err != nil {
		t.Fatalf("WaitForApproval failed: %v", err)
	}
	if result.State != RequestStateApproved {
		t.Errorf("State = %q, want %q", result.State, RequestStateApproved)
	}
}

func TestManager_WaitForApproval_ContextCancelled(t *testing.T) {
	storage := newMockStorage()
	manager := NewManager(storage)
	defer manager.Stop()

	config := &Config{
		Title:     "Test",
		Approvers: []string{"user1"},
	}

	req, _ := manager.CreateRequest(context.Background(), config, "exec-123", "step1", nil)

	ctx, cancel := context.WithCancel(context.Background())

	var waitErr error
	done := make(chan struct{})

	go func() {
		_, waitErr = manager.WaitForApproval(ctx, req.ID)
		close(done)
	}()

	// Give time to register
	time.Sleep(10 * time.Millisecond)

	// Cancel the context
	cancel()

	select {
	case <-done:
		if waitErr != context.Canceled {
			t.Errorf("expected context.Canceled error, got %v", waitErr)
		}
	case <-time.After(time.Second):
		t.Fatal("WaitForApproval should have returned after context cancellation")
	}
}

func TestManager_CheckExpired(t *testing.T) {
	storage := newMockStorage()
	manager := NewManager(storage)

	// Create a request that's already expired
	past := time.Now().Add(-time.Hour)
	req := &Request{
		ID:          "expired-1",
		ExecutionID: "exec-1",
		StepName:    "step1",
		State:       RequestStatePending,
		Approvers:   []string{"user1"},
		ExpiresAt:   &past,
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}
	storage.SaveRequest(context.Background(), req)

	// Create a non-expired request
	future := time.Now().Add(time.Hour)
	req2 := &Request{
		ID:          "not-expired",
		ExecutionID: "exec-2",
		StepName:    "step1",
		State:       RequestStatePending,
		Approvers:   []string{"user1"},
		ExpiresAt:   &future,
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}
	storage.SaveRequest(context.Background(), req2)

	expired, err := manager.CheckExpired(context.Background())
	if err != nil {
		t.Fatalf("CheckExpired failed: %v", err)
	}

	if expired != 1 {
		t.Errorf("expired count = %d, want 1", expired)
	}

	// Verify the expired request was updated
	updated, _ := storage.GetRequest(context.Background(), "expired-1")
	if updated.State != RequestStateExpired {
		t.Errorf("expired request state = %q, want %q", updated.State, RequestStateExpired)
	}
}
