package intervention

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
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

func TestSQLiteStorage_SaveAndGet(t *testing.T) {
	db := setupTestDB(t)
	storage, err := NewSQLiteStorage(db)
	if err != nil {
		t.Fatalf("NewSQLiteStorage: %v", err)
	}

	now := time.Now().Truncate(time.Second)
	expiresAt := now.Add(time.Hour)
	req := &Request{
		ID:          "req-123",
		ExecutionID: "exec-456",
		StepName:    "confirm-step",
		Type:        TypeConfirm,
		State:       StatePending,
		Title:       "Confirm deployment",
		Description: "Please confirm to proceed",
		Timeout:     time.Hour,
		ExpiresAt:   &expiresAt,
		Metadata: map[string]interface{}{
			"key": "value",
		},
		CreatedAt: now,
		UpdatedAt: now,
	}

	if err := storage.SaveRequest(context.Background(), req); err != nil {
		t.Fatalf("SaveRequest: %v", err)
	}

	loaded, err := storage.GetRequest(context.Background(), "req-123")
	if err != nil {
		t.Fatalf("GetRequest: %v", err)
	}
	if loaded == nil {
		t.Fatal("expected request to be found")
	}

	if loaded.ID != req.ID {
		t.Errorf("ID = %q, want %q", loaded.ID, req.ID)
	}
	if loaded.ExecutionID != req.ExecutionID {
		t.Errorf("ExecutionID = %q, want %q", loaded.ExecutionID, req.ExecutionID)
	}
	if loaded.StepName != req.StepName {
		t.Errorf("StepName = %q, want %q", loaded.StepName, req.StepName)
	}
	if loaded.Type != req.Type {
		t.Errorf("Type = %q, want %q", loaded.Type, req.Type)
	}
	if loaded.State != req.State {
		t.Errorf("State = %q, want %q", loaded.State, req.State)
	}
	if loaded.Title != req.Title {
		t.Errorf("Title = %q, want %q", loaded.Title, req.Title)
	}
	if loaded.Timeout != req.Timeout {
		t.Errorf("Timeout = %v, want %v", loaded.Timeout, req.Timeout)
	}
	if loaded.Metadata["key"] != "value" {
		t.Errorf("Metadata[key] = %v, want %q", loaded.Metadata["key"], "value")
	}
}

func TestSQLiteStorage_SaveWithPrompts(t *testing.T) {
	db := setupTestDB(t)
	storage, err := NewSQLiteStorage(db)
	if err != nil {
		t.Fatalf("NewSQLiteStorage: %v", err)
	}

	now := time.Now().Truncate(time.Second)
	minVal := float64(1)
	maxVal := float64(100)
	req := &Request{
		ID:          "req-123",
		ExecutionID: "exec-456",
		StepName:    "prompt-step",
		Type:        TypePrompt,
		State:       StatePending,
		Title:       "Enter values",
		Prompts: []PromptField{
			{
				Name:     "version",
				Label:    "Version",
				Type:     FieldTypeText,
				Required: true,
			},
			{
				Name:    "replicas",
				Label:   "Replicas",
				Type:    FieldTypeNumber,
				Default: 3,
				Validation: &FieldValidation{
					Min: &minVal,
					Max: &maxVal,
				},
			},
			{
				Name:  "region",
				Label: "Region",
				Type:  FieldTypeSelect,
				Options: []Option{
					{Value: "us-east-1", Label: "US East"},
					{Value: "us-west-2", Label: "US West"},
				},
			},
		},
		CreatedAt: now,
		UpdatedAt: now,
	}

	if err := storage.SaveRequest(context.Background(), req); err != nil {
		t.Fatalf("SaveRequest: %v", err)
	}

	loaded, err := storage.GetRequest(context.Background(), "req-123")
	if err != nil {
		t.Fatalf("GetRequest: %v", err)
	}

	if len(loaded.Prompts) != 3 {
		t.Fatalf("len(Prompts) = %d, want 3", len(loaded.Prompts))
	}

	// Check first prompt
	if loaded.Prompts[0].Name != "version" {
		t.Errorf("Prompts[0].Name = %q, want %q", loaded.Prompts[0].Name, "version")
	}
	if loaded.Prompts[0].Type != FieldTypeText {
		t.Errorf("Prompts[0].Type = %q, want %q", loaded.Prompts[0].Type, FieldTypeText)
	}
	if !loaded.Prompts[0].Required {
		t.Error("Prompts[0].Required = false, want true")
	}

	// Check validation
	if loaded.Prompts[1].Validation == nil {
		t.Fatal("Prompts[1].Validation is nil")
	}
	if *loaded.Prompts[1].Validation.Min != 1 {
		t.Errorf("Prompts[1].Validation.Min = %v, want 1", *loaded.Prompts[1].Validation.Min)
	}

	// Check options
	if len(loaded.Prompts[2].Options) != 2 {
		t.Errorf("len(Prompts[2].Options) = %d, want 2", len(loaded.Prompts[2].Options))
	}
}

func TestSQLiteStorage_SaveWithResponse(t *testing.T) {
	db := setupTestDB(t)
	storage, err := NewSQLiteStorage(db)
	if err != nil {
		t.Fatalf("NewSQLiteStorage: %v", err)
	}

	now := time.Now().Truncate(time.Second)
	completedAt := now.Add(time.Minute)
	req := &Request{
		ID:          "req-123",
		ExecutionID: "exec-456",
		StepName:    "confirm-step",
		Type:        TypeConfirm,
		State:       StateCompleted,
		Title:       "Confirm",
		Response: &Response{
			Operator:    "operator@example.com",
			Confirmed:   true,
			Comment:     "Approved",
			RespondedAt: completedAt,
		},
		CreatedAt:   now,
		UpdatedAt:   completedAt,
		CompletedAt: &completedAt,
	}

	if err := storage.SaveRequest(context.Background(), req); err != nil {
		t.Fatalf("SaveRequest: %v", err)
	}

	loaded, err := storage.GetRequest(context.Background(), "req-123")
	if err != nil {
		t.Fatalf("GetRequest: %v", err)
	}

	if loaded.Response == nil {
		t.Fatal("Response is nil")
	}
	if loaded.Response.Operator != "operator@example.com" {
		t.Errorf("Response.Operator = %q, want %q", loaded.Response.Operator, "operator@example.com")
	}
	if !loaded.Response.Confirmed {
		t.Error("Response.Confirmed = false, want true")
	}
	if loaded.Response.Comment != "Approved" {
		t.Errorf("Response.Comment = %q, want %q", loaded.Response.Comment, "Approved")
	}
	if loaded.CompletedAt == nil {
		t.Error("CompletedAt is nil")
	}
}

func TestSQLiteStorage_SaveWithPromptValues(t *testing.T) {
	db := setupTestDB(t)
	storage, err := NewSQLiteStorage(db)
	if err != nil {
		t.Fatalf("NewSQLiteStorage: %v", err)
	}

	now := time.Now().Truncate(time.Second)
	req := &Request{
		ID:          "req-123",
		ExecutionID: "exec-456",
		StepName:    "prompt-step",
		Type:        TypePrompt,
		State:       StateCompleted,
		Title:       "Values",
		Response: &Response{
			Operator: "op",
			Values: map[string]interface{}{
				"version":  "1.0.0",
				"replicas": float64(5), // JSON numbers are float64
				"enabled":  true,
			},
			RespondedAt: now,
		},
		CreatedAt: now,
		UpdatedAt: now,
	}

	if err := storage.SaveRequest(context.Background(), req); err != nil {
		t.Fatalf("SaveRequest: %v", err)
	}

	loaded, err := storage.GetRequest(context.Background(), "req-123")
	if err != nil {
		t.Fatalf("GetRequest: %v", err)
	}

	if loaded.Response.Values["version"] != "1.0.0" {
		t.Errorf("Values[version] = %v, want %q", loaded.Response.Values["version"], "1.0.0")
	}
	if loaded.Response.Values["replicas"] != float64(5) {
		t.Errorf("Values[replicas] = %v, want 5", loaded.Response.Values["replicas"])
	}
	if loaded.Response.Values["enabled"] != true {
		t.Errorf("Values[enabled] = %v, want true", loaded.Response.Values["enabled"])
	}
}

func TestSQLiteStorage_GetByExecution(t *testing.T) {
	db := setupTestDB(t)
	storage, err := NewSQLiteStorage(db)
	if err != nil {
		t.Fatalf("NewSQLiteStorage: %v", err)
	}

	now := time.Now().Truncate(time.Second)
	req := &Request{
		ID:          "req-123",
		ExecutionID: "exec-456",
		StepName:    "step1",
		Type:        TypeConfirm,
		State:       StatePending,
		Title:       "Test",
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	storage.SaveRequest(context.Background(), req)

	// Found
	loaded, err := storage.GetRequestByExecution(context.Background(), "exec-456", "step1")
	if err != nil {
		t.Fatalf("GetRequestByExecution: %v", err)
	}
	if loaded == nil {
		t.Fatal("expected request to be found")
	}
	if loaded.ID != "req-123" {
		t.Errorf("ID = %q, want %q", loaded.ID, "req-123")
	}

	// Not found
	loaded, err = storage.GetRequestByExecution(context.Background(), "exec-456", "step2")
	if err != nil {
		t.Fatalf("GetRequestByExecution: %v", err)
	}
	if loaded != nil {
		t.Error("expected request not to be found")
	}
}

func TestSQLiteStorage_GetNotFound(t *testing.T) {
	db := setupTestDB(t)
	storage, err := NewSQLiteStorage(db)
	if err != nil {
		t.Fatalf("NewSQLiteStorage: %v", err)
	}

	loaded, err := storage.GetRequest(context.Background(), "nonexistent")
	if err != nil {
		t.Fatalf("GetRequest: %v", err)
	}
	if loaded != nil {
		t.Error("expected nil for nonexistent request")
	}
}

func TestSQLiteStorage_Update(t *testing.T) {
	db := setupTestDB(t)
	storage, err := NewSQLiteStorage(db)
	if err != nil {
		t.Fatalf("NewSQLiteStorage: %v", err)
	}

	now := time.Now().Truncate(time.Second)
	req := &Request{
		ID:          "req-123",
		ExecutionID: "exec-456",
		StepName:    "step1",
		Type:        TypeConfirm,
		State:       StatePending,
		Title:       "Test",
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	storage.SaveRequest(context.Background(), req)

	// Update
	completedAt := now.Add(time.Minute)
	req.State = StateCompleted
	req.UpdatedAt = completedAt
	req.CompletedAt = &completedAt
	req.Response = &Response{
		Operator:    "op",
		Confirmed:   true,
		RespondedAt: completedAt,
	}
	storage.SaveRequest(context.Background(), req)

	loaded, _ := storage.GetRequest(context.Background(), "req-123")
	if loaded.State != StateCompleted {
		t.Errorf("State = %q, want %q", loaded.State, StateCompleted)
	}
	if loaded.Response == nil {
		t.Error("expected Response to be set")
	}
}

func TestSQLiteStorage_ListRequests(t *testing.T) {
	db := setupTestDB(t)
	storage, err := NewSQLiteStorage(db)
	if err != nil {
		t.Fatalf("NewSQLiteStorage: %v", err)
	}

	now := time.Now().Truncate(time.Second)

	// Create test requests
	for i := 0; i < 5; i++ {
		state := StatePending
		if i%2 == 0 {
			state = StateCompleted
		}
		reqType := TypeConfirm
		if i > 2 {
			reqType = TypePrompt
		}
		execID := "exec-1"
		if i >= 3 {
			execID = "exec-2"
		}
		req := &Request{
			ID:          "req-" + string(rune('0'+i)),
			ExecutionID: execID,
			StepName:    "step" + string(rune('0'+i)),
			Type:        reqType,
			State:       state,
			Title:       "Test",
			CreatedAt:   now.Add(time.Duration(i) * time.Second),
			UpdatedAt:   now,
		}
		storage.SaveRequest(context.Background(), req)
	}

	// List all
	requests, err := storage.ListRequests(context.Background(), ListOptions{})
	if err != nil {
		t.Fatalf("ListRequests: %v", err)
	}
	if len(requests) != 5 {
		t.Errorf("len(requests) = %d, want 5", len(requests))
	}

	// Filter by execution
	requests, _ = storage.ListRequests(context.Background(), ListOptions{ExecutionID: "exec-1"})
	if len(requests) != 3 {
		t.Errorf("len(requests) filtered by exec = %d, want 3", len(requests))
	}

	// Filter by state
	requests, _ = storage.ListRequests(context.Background(), ListOptions{State: StatePending})
	if len(requests) != 2 {
		t.Errorf("len(requests) filtered by state = %d, want 2", len(requests))
	}

	// Filter by type
	requests, _ = storage.ListRequests(context.Background(), ListOptions{Type: TypePrompt})
	if len(requests) != 2 {
		t.Errorf("len(requests) filtered by type = %d, want 2", len(requests))
	}

	// Limit
	requests, _ = storage.ListRequests(context.Background(), ListOptions{Limit: 2})
	if len(requests) != 2 {
		t.Errorf("len(requests) with limit = %d, want 2", len(requests))
	}

	// Offset
	requests, _ = storage.ListRequests(context.Background(), ListOptions{Limit: 10, Offset: 3})
	if len(requests) != 2 {
		t.Errorf("len(requests) with offset = %d, want 2", len(requests))
	}

	// Since/Until
	since := now.Add(2 * time.Second)
	requests, _ = storage.ListRequests(context.Background(), ListOptions{Since: &since})
	if len(requests) != 3 {
		t.Errorf("len(requests) since = %d, want 3", len(requests))
	}
}

func TestSQLiteStorage_Delete(t *testing.T) {
	db := setupTestDB(t)
	storage, err := NewSQLiteStorage(db)
	if err != nil {
		t.Fatalf("NewSQLiteStorage: %v", err)
	}

	now := time.Now().Truncate(time.Second)
	req := &Request{
		ID:          "req-123",
		ExecutionID: "exec-456",
		StepName:    "step1",
		Type:        TypeConfirm,
		State:       StatePending,
		Title:       "Test",
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	storage.SaveRequest(context.Background(), req)

	// Delete
	if err := storage.DeleteRequest(context.Background(), "req-123"); err != nil {
		t.Fatalf("DeleteRequest: %v", err)
	}

	// Verify deleted
	loaded, _ := storage.GetRequest(context.Background(), "req-123")
	if loaded != nil {
		t.Error("expected request to be deleted")
	}
}

func TestSQLiteStorage_UniqueConstraint(t *testing.T) {
	db := setupTestDB(t)
	storage, err := NewSQLiteStorage(db)
	if err != nil {
		t.Fatalf("NewSQLiteStorage: %v", err)
	}

	now := time.Now().Truncate(time.Second)

	// Create first request
	req1 := &Request{
		ID:          "req-1",
		ExecutionID: "exec-1",
		StepName:    "step1",
		Type:        TypeConfirm,
		State:       StatePending,
		Title:       "Test 1",
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	if err := storage.SaveRequest(context.Background(), req1); err != nil {
		t.Fatalf("SaveRequest 1: %v", err)
	}

	// Try to create another with same execution_id + step_name
	req2 := &Request{
		ID:          "req-2",
		ExecutionID: "exec-1",
		StepName:    "step1",
		Type:        TypeConfirm,
		State:       StatePending,
		Title:       "Test 2",
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	err = storage.SaveRequest(context.Background(), req2)
	if err == nil {
		t.Error("expected error for duplicate execution_id + step_name")
	}
}
