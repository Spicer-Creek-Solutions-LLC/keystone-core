package approval

import (
	"context"
	"database/sql"
	"testing"
	"time"

	_ "modernc.org/sqlite"
)

func setupTestDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

func TestSQLiteStorage_SaveAndGetRequest(t *testing.T) {
	db := setupTestDB(t)
	storage, err := NewSQLiteStorage(db)
	if err != nil {
		t.Fatalf("NewSQLiteStorage: %v", err)
	}

	now := time.Now().Truncate(time.Second)
	expiresAt := now.Add(time.Hour)

	req := &Request{
		ID:            "req-123",
		ExecutionID:   "exec-456",
		StepName:      "deploy-approval",
		State:         RequestStatePending,
		Title:         "Deploy to production",
		Description:   "Please review and approve",
		Approvers:     []string{"admin@example.com", "ops-team"},
		Mode:          ModeAny,
		RequiredCount: 1,
		Timeout:       time.Hour,
		ExpiresAt:     &expiresAt,
		Metadata: map[string]interface{}{
			"environment": "production",
		},
		CreatedAt: now,
		UpdatedAt: now,
	}

	// Save
	if err := storage.SaveRequest(context.Background(), req); err != nil {
		t.Fatalf("SaveRequest: %v", err)
	}

	// Get
	retrieved, err := storage.GetRequest(context.Background(), "req-123")
	if err != nil {
		t.Fatalf("GetRequest: %v", err)
	}
	if retrieved == nil {
		t.Fatal("GetRequest returned nil")
	}

	if retrieved.ID != "req-123" {
		t.Errorf("ID = %q, want %q", retrieved.ID, "req-123")
	}
	if retrieved.ExecutionID != "exec-456" {
		t.Errorf("ExecutionID = %q, want %q", retrieved.ExecutionID, "exec-456")
	}
	if retrieved.StepName != "deploy-approval" {
		t.Errorf("StepName = %q, want %q", retrieved.StepName, "deploy-approval")
	}
	if retrieved.State != RequestStatePending {
		t.Errorf("State = %q, want %q", retrieved.State, RequestStatePending)
	}
	if retrieved.Title != "Deploy to production" {
		t.Errorf("Title = %q, want %q", retrieved.Title, "Deploy to production")
	}
	if len(retrieved.Approvers) != 2 {
		t.Errorf("Approvers count = %d, want 2", len(retrieved.Approvers))
	}
	if retrieved.Mode != ModeAny {
		t.Errorf("Mode = %q, want %q", retrieved.Mode, ModeAny)
	}
	if retrieved.Metadata["environment"] != "production" {
		t.Errorf("Metadata[environment] = %v, want %q", retrieved.Metadata["environment"], "production")
	}
}

func TestSQLiteStorage_GetRequest_NotFound(t *testing.T) {
	db := setupTestDB(t)
	storage, _ := NewSQLiteStorage(db)

	req, err := storage.GetRequest(context.Background(), "nonexistent")
	if err != nil {
		t.Fatalf("GetRequest error: %v", err)
	}
	if req != nil {
		t.Errorf("expected nil, got %v", req)
	}
}

func TestSQLiteStorage_GetRequestByExecution(t *testing.T) {
	db := setupTestDB(t)
	storage, _ := NewSQLiteStorage(db)

	now := time.Now()
	req := &Request{
		ID:          "req-123",
		ExecutionID: "exec-456",
		StepName:    "step1",
		State:       RequestStatePending,
		Title:       "Test",
		Approvers:   []string{"user1"},
		Mode:        ModeAny,
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	storage.SaveRequest(context.Background(), req)

	retrieved, err := storage.GetRequestByExecution(context.Background(), "exec-456", "step1")
	if err != nil {
		t.Fatalf("GetRequestByExecution: %v", err)
	}
	if retrieved == nil {
		t.Fatal("expected request, got nil")
	}
	if retrieved.ID != "req-123" {
		t.Errorf("ID = %q, want %q", retrieved.ID, "req-123")
	}
}

func TestSQLiteStorage_UpdateRequest(t *testing.T) {
	db := setupTestDB(t)
	storage, _ := NewSQLiteStorage(db)

	now := time.Now()
	req := &Request{
		ID:          "req-123",
		ExecutionID: "exec-456",
		StepName:    "step1",
		State:       RequestStatePending,
		Title:       "Test",
		Approvers:   []string{"user1"},
		Mode:        ModeAny,
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	storage.SaveRequest(context.Background(), req)

	// Update with response
	req.State = RequestStateApproved
	req.Responses = []Response{
		{
			Approver:    "user1",
			Decision:    DecisionApproved,
			Comment:     "LGTM",
			RespondedAt: now,
		},
	}
	completedAt := now.Add(time.Minute)
	req.CompletedAt = &completedAt
	req.UpdatedAt = completedAt

	if err := storage.SaveRequest(context.Background(), req); err != nil {
		t.Fatalf("SaveRequest update: %v", err)
	}

	retrieved, _ := storage.GetRequest(context.Background(), "req-123")
	if retrieved.State != RequestStateApproved {
		t.Errorf("State = %q, want %q", retrieved.State, RequestStateApproved)
	}
	if len(retrieved.Responses) != 1 {
		t.Errorf("Responses count = %d, want 1", len(retrieved.Responses))
	}
	if retrieved.Responses[0].Comment != "LGTM" {
		t.Errorf("Comment = %q, want %q", retrieved.Responses[0].Comment, "LGTM")
	}
	if retrieved.CompletedAt == nil {
		t.Error("CompletedAt should be set")
	}
}

func TestSQLiteStorage_ListRequests(t *testing.T) {
	db := setupTestDB(t)
	storage, _ := NewSQLiteStorage(db)

	now := time.Now()

	// Create multiple requests
	for i, state := range []RequestState{RequestStatePending, RequestStateApproved, RequestStatePending} {
		req := &Request{
			ID:          string(rune('a' + i)),
			ExecutionID: "exec-1",
			StepName:    string(rune('1' + i)),
			State:       state,
			Title:       "Test",
			Approvers:   []string{"user1"},
			Mode:        ModeAny,
			CreatedAt:   now.Add(time.Duration(i) * time.Minute),
			UpdatedAt:   now,
		}
		storage.SaveRequest(context.Background(), req)
	}

	t.Run("all", func(t *testing.T) {
		requests, err := storage.ListRequests(context.Background(), ListOptions{})
		if err != nil {
			t.Fatalf("ListRequests: %v", err)
		}
		if len(requests) != 3 {
			t.Errorf("count = %d, want 3", len(requests))
		}
	})

	t.Run("by_state", func(t *testing.T) {
		requests, err := storage.ListRequests(context.Background(), ListOptions{
			State: RequestStatePending,
		})
		if err != nil {
			t.Fatalf("ListRequests: %v", err)
		}
		if len(requests) != 2 {
			t.Errorf("count = %d, want 2", len(requests))
		}
	})

	t.Run("by_execution", func(t *testing.T) {
		requests, err := storage.ListRequests(context.Background(), ListOptions{
			ExecutionID: "exec-1",
		})
		if err != nil {
			t.Fatalf("ListRequests: %v", err)
		}
		if len(requests) != 3 {
			t.Errorf("count = %d, want 3", len(requests))
		}
	})

	t.Run("with_limit", func(t *testing.T) {
		requests, err := storage.ListRequests(context.Background(), ListOptions{
			Limit: 2,
		})
		if err != nil {
			t.Fatalf("ListRequests: %v", err)
		}
		if len(requests) != 2 {
			t.Errorf("count = %d, want 2", len(requests))
		}
	})
}

func TestSQLiteStorage_DeleteRequest(t *testing.T) {
	db := setupTestDB(t)
	storage, _ := NewSQLiteStorage(db)

	now := time.Now()
	req := &Request{
		ID:          "req-123",
		ExecutionID: "exec-456",
		StepName:    "step1",
		State:       RequestStatePending,
		Title:       "Test",
		Approvers:   []string{"user1"},
		Mode:        ModeAny,
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	storage.SaveRequest(context.Background(), req)

	if err := storage.DeleteRequest(context.Background(), "req-123"); err != nil {
		t.Fatalf("DeleteRequest: %v", err)
	}

	retrieved, _ := storage.GetRequest(context.Background(), "req-123")
	if retrieved != nil {
		t.Error("request should have been deleted")
	}
}

func TestSQLiteStorage_UniqueConstraint(t *testing.T) {
	db := setupTestDB(t)
	storage, _ := NewSQLiteStorage(db)

	now := time.Now()

	// First request
	req1 := &Request{
		ID:          "req-1",
		ExecutionID: "exec-1",
		StepName:    "step1",
		State:       RequestStatePending,
		Title:       "Test 1",
		Approvers:   []string{"user1"},
		Mode:        ModeAny,
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	if err := storage.SaveRequest(context.Background(), req1); err != nil {
		t.Fatalf("SaveRequest 1: %v", err)
	}

	// Second request with same execution_id and step_name but different id should fail
	// due to unique constraint (or update if using ON CONFLICT)
	req2 := &Request{
		ID:          "req-2",
		ExecutionID: "exec-1",
		StepName:    "step1",
		State:       RequestStatePending,
		Title:       "Test 2",
		Approvers:   []string{"user2"},
		Mode:        ModeAny,
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	err := storage.SaveRequest(context.Background(), req2)
	// This should fail due to unique constraint on (execution_id, step_name)
	if err == nil {
		t.Error("expected error for duplicate execution_id + step_name")
	}
}
