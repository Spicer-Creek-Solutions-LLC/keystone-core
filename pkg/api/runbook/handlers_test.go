package runbook

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/shawnbutts/keystone-core/internal/runbook/approval"
	"github.com/shawnbutts/keystone-core/internal/runbook/intervention"

	_ "modernc.org/sqlite"
)

func setupTestDB(t *testing.T) (*sql.DB, string, func()) {
	t.Helper()
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}

	return db, dbPath, func() {
		db.Close()
		os.RemoveAll(tmpDir)
	}
}

func setupApprovalHandler(t *testing.T) (*Handler, func()) {
	t.Helper()

	db, _, cleanup := setupTestDB(t)

	approvalStorage, err := approval.NewSQLiteStorage(db)
	if err != nil {
		cleanup()
		t.Fatalf("init approval storage: %v", err)
	}

	interventionStorage, err := intervention.NewSQLiteStorage(db)
	if err != nil {
		cleanup()
		t.Fatalf("init intervention storage: %v", err)
	}

	approvalManager := approval.NewManager(approvalStorage)
	interventionManager := intervention.NewManager(interventionStorage)

	h := NewHandler(approvalStorage, interventionStorage, approvalManager, interventionManager)
	return h, cleanup
}

func TestListApprovals(t *testing.T) {
	h, cleanup := setupApprovalHandler(t)
	defer cleanup()

	// Create a test approval
	ctx := context.Background()
	now := time.Now()
	req := &approval.Request{
		ID:          "req-test-1",
		ExecutionID: "exec-1",
		StepName:    "step1",
		State:       approval.RequestStatePending,
		Title:       "Test Approval",
		Approvers:   []string{"user1", "user2"},
		Mode:        approval.ApprovalModeAny,
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	h.approvalStorage.SaveRequest(ctx, req)

	// Test list approvals
	r := httptest.NewRequest(http.MethodGet, "/api/v1/runbook/approvals", nil)
	w := httptest.NewRecorder()

	h.handleApprovals(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp ApprovalListResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}

	if len(resp.Approvals) != 1 {
		t.Fatalf("expected 1 approval, got %d", len(resp.Approvals))
	}
	if resp.Approvals[0].ID != "req-test-1" {
		t.Errorf("ID = %q, want %q", resp.Approvals[0].ID, "req-test-1")
	}
}

func TestListApprovals_FilterByState(t *testing.T) {
	h, cleanup := setupApprovalHandler(t)
	defer cleanup()

	ctx := context.Background()
	now := time.Now()

	// Create pending and approved requests
	h.approvalStorage.SaveRequest(ctx, &approval.Request{
		ID: "req-pending", ExecutionID: "exec-1", StepName: "step1",
		State: approval.RequestStatePending, Title: "Pending",
		Approvers: []string{"user1"}, Mode: approval.ApprovalModeAny,
		CreatedAt: now, UpdatedAt: now,
	})
	h.approvalStorage.SaveRequest(ctx, &approval.Request{
		ID: "req-approved", ExecutionID: "exec-2", StepName: "step1",
		State: approval.RequestStateApproved, Title: "Approved",
		Approvers: []string{"user1"}, Mode: approval.ApprovalModeAny,
		CreatedAt: now, UpdatedAt: now,
	})

	// Filter by pending state
	r := httptest.NewRequest(http.MethodGet, "/api/v1/runbook/approvals?state=pending", nil)
	w := httptest.NewRecorder()

	h.handleApprovals(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", w.Code)
	}

	var resp ApprovalListResponse
	json.Unmarshal(w.Body.Bytes(), &resp)

	if len(resp.Approvals) != 1 {
		t.Fatalf("expected 1 approval, got %d", len(resp.Approvals))
	}
	if resp.Approvals[0].ID != "req-pending" {
		t.Errorf("ID = %q, want %q", resp.Approvals[0].ID, "req-pending")
	}
}

func TestGetApproval(t *testing.T) {
	h, cleanup := setupApprovalHandler(t)
	defer cleanup()

	ctx := context.Background()
	now := time.Now()
	req := &approval.Request{
		ID:          "req-get-1",
		ExecutionID: "exec-1",
		StepName:    "step1",
		State:       approval.RequestStatePending,
		Title:       "Get Test",
		Approvers:   []string{"user1"},
		Mode:        approval.ApprovalModeAny,
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	h.approvalStorage.SaveRequest(ctx, req)

	r := httptest.NewRequest(http.MethodGet, "/api/v1/runbook/approvals/req-get-1", nil)
	w := httptest.NewRecorder()

	h.handleApproval(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp ApprovalResponse
	json.Unmarshal(w.Body.Bytes(), &resp)

	if resp.ID != "req-get-1" {
		t.Errorf("ID = %q, want %q", resp.ID, "req-get-1")
	}
	if resp.Title != "Get Test" {
		t.Errorf("Title = %q, want %q", resp.Title, "Get Test")
	}
}

func TestGetApproval_NotFound(t *testing.T) {
	h, cleanup := setupApprovalHandler(t)
	defer cleanup()

	r := httptest.NewRequest(http.MethodGet, "/api/v1/runbook/approvals/nonexistent", nil)
	w := httptest.NewRecorder()

	h.handleApproval(w, r)

	if w.Code != http.StatusNotFound {
		t.Errorf("expected status 404, got %d", w.Code)
	}
}

func TestApproveRequest(t *testing.T) {
	h, cleanup := setupApprovalHandler(t)
	defer cleanup()

	ctx := context.Background()
	now := time.Now()
	req := &approval.Request{
		ID:          "req-approve-1",
		ExecutionID: "exec-1",
		StepName:    "step1",
		State:       approval.RequestStatePending,
		Title:       "Approve Test",
		Approvers:   []string{"user1"},
		Mode:        approval.ApprovalModeAny,
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	h.approvalStorage.SaveRequest(ctx, req)

	body := ApproveRequest{
		Approver: "user1",
		Comment:  "Looks good",
	}
	bodyBytes, _ := json.Marshal(body)

	r := httptest.NewRequest(http.MethodPost, "/api/v1/runbook/approvals/req-approve-1/approve", bytes.NewReader(bodyBytes))
	r.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	h.handleApproval(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp ApprovalResponse
	json.Unmarshal(w.Body.Bytes(), &resp)

	if resp.State != "approved" {
		t.Errorf("State = %q, want %q", resp.State, "approved")
	}
}

func TestRejectRequest(t *testing.T) {
	h, cleanup := setupApprovalHandler(t)
	defer cleanup()

	ctx := context.Background()
	now := time.Now()
	req := &approval.Request{
		ID:          "req-reject-1",
		ExecutionID: "exec-1",
		StepName:    "step1",
		State:       approval.RequestStatePending,
		Title:       "Reject Test",
		Approvers:   []string{"user1"},
		Mode:        approval.ApprovalModeAny,
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	h.approvalStorage.SaveRequest(ctx, req)

	body := RejectRequest{
		Approver: "user1",
		Comment:  "Not ready",
	}
	bodyBytes, _ := json.Marshal(body)

	r := httptest.NewRequest(http.MethodPost, "/api/v1/runbook/approvals/req-reject-1/reject", bytes.NewReader(bodyBytes))
	r.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	h.handleApproval(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp ApprovalResponse
	json.Unmarshal(w.Body.Bytes(), &resp)

	if resp.State != "rejected" {
		t.Errorf("State = %q, want %q", resp.State, "rejected")
	}
}

func TestRejectRequest_RequiresReason(t *testing.T) {
	h, cleanup := setupApprovalHandler(t)
	defer cleanup()

	body := RejectRequest{
		Approver: "user1",
		Comment:  "", // Missing comment
	}
	bodyBytes, _ := json.Marshal(body)

	r := httptest.NewRequest(http.MethodPost, "/api/v1/runbook/approvals/req-123/reject", bytes.NewReader(bodyBytes))
	r.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	h.handleApproval(w, r)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected status 400, got %d", w.Code)
	}
}

func TestDelegateRequest(t *testing.T) {
	h, cleanup := setupApprovalHandler(t)
	defer cleanup()

	ctx := context.Background()
	now := time.Now()
	req := &approval.Request{
		ID:          "req-delegate-1",
		ExecutionID: "exec-1",
		StepName:    "step1",
		State:       approval.RequestStatePending,
		Title:       "Delegate Test",
		Approvers:   []string{"user1"},
		Mode:        approval.ApprovalModeAny,
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	h.approvalStorage.SaveRequest(ctx, req)

	body := DelegateRequest{
		From: "user1",
		To:   "user2",
	}
	bodyBytes, _ := json.Marshal(body)

	r := httptest.NewRequest(http.MethodPost, "/api/v1/runbook/approvals/req-delegate-1/delegate", bytes.NewReader(bodyBytes))
	r.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	h.handleApproval(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp ApprovalResponse
	json.Unmarshal(w.Body.Bytes(), &resp)

	// Verify user2 was added to approvers
	found := false
	for _, a := range resp.Approvers {
		if a == "user2" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected user2 in approvers: %v", resp.Approvers)
	}
}

func TestListInterventions(t *testing.T) {
	h, cleanup := setupApprovalHandler(t)
	defer cleanup()

	ctx := context.Background()
	now := time.Now()
	req := &intervention.Request{
		ID:          "int-test-1",
		ExecutionID: "exec-1",
		StepName:    "step1",
		Type:        intervention.InterventionTypePrompt,
		State:       intervention.InterventionStatePending,
		Title:       "Test Prompt",
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	h.interventionStorage.SaveRequest(ctx, req)

	r := httptest.NewRequest(http.MethodGet, "/api/v1/runbook/interventions", nil)
	w := httptest.NewRecorder()

	h.handleInterventions(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp InterventionListResponse
	json.Unmarshal(w.Body.Bytes(), &resp)

	if len(resp.Interventions) != 1 {
		t.Fatalf("expected 1 intervention, got %d", len(resp.Interventions))
	}
	if resp.Interventions[0].ID != "int-test-1" {
		t.Errorf("ID = %q, want %q", resp.Interventions[0].ID, "int-test-1")
	}
}

func TestGetIntervention(t *testing.T) {
	h, cleanup := setupApprovalHandler(t)
	defer cleanup()

	ctx := context.Background()
	now := time.Now()
	req := &intervention.Request{
		ID:          "int-get-1",
		ExecutionID: "exec-1",
		StepName:    "step1",
		Type:        intervention.InterventionTypeConfirm,
		State:       intervention.InterventionStatePending,
		Title:       "Get Test",
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	h.interventionStorage.SaveRequest(ctx, req)

	r := httptest.NewRequest(http.MethodGet, "/api/v1/runbook/interventions/int-get-1", nil)
	w := httptest.NewRecorder()

	h.handleIntervention(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp InterventionResponse
	json.Unmarshal(w.Body.Bytes(), &resp)

	if resp.ID != "int-get-1" {
		t.Errorf("ID = %q, want %q", resp.ID, "int-get-1")
	}
	if resp.Type != "confirm" {
		t.Errorf("Type = %q, want %q", resp.Type, "confirm")
	}
}

func TestRespondToIntervention(t *testing.T) {
	h, cleanup := setupApprovalHandler(t)
	defer cleanup()

	ctx := context.Background()
	now := time.Now()
	req := &intervention.Request{
		ID:          "int-respond-1",
		ExecutionID: "exec-1",
		StepName:    "step1",
		Type:        intervention.InterventionTypeConfirm,
		State:       intervention.InterventionStatePending,
		Title:       "Respond Test",
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	h.interventionStorage.SaveRequest(ctx, req)

	body := RespondRequest{
		Operator:  "user1",
		Confirmed: true,
		Comment:   "Confirmed",
	}
	bodyBytes, _ := json.Marshal(body)

	r := httptest.NewRequest(http.MethodPost, "/api/v1/runbook/interventions/int-respond-1/respond", bytes.NewReader(bodyBytes))
	r.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	h.handleIntervention(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp InterventionResponse
	json.Unmarshal(w.Body.Bytes(), &resp)

	if resp.State != "completed" {
		t.Errorf("State = %q, want %q", resp.State, "completed")
	}
	if resp.Response == nil {
		t.Fatal("expected response to be set")
	}
	if !resp.Response.Confirmed {
		t.Error("expected Confirmed = true")
	}
}

func TestRespondToIntervention_WithValues(t *testing.T) {
	h, cleanup := setupApprovalHandler(t)
	defer cleanup()

	ctx := context.Background()
	now := time.Now()
	req := &intervention.Request{
		ID:          "int-values-1",
		ExecutionID: "exec-1",
		StepName:    "step1",
		Type:        intervention.InterventionTypePrompt,
		State:       intervention.InterventionStatePending,
		Title:       "Values Test",
		Prompts: []intervention.PromptField{
			{Name: "version", Type: intervention.FieldTypeText},
		},
		CreatedAt: now,
		UpdatedAt: now,
	}
	h.interventionStorage.SaveRequest(ctx, req)

	body := RespondRequest{
		Operator: "user1",
		Values: map[string]interface{}{
			"version": "1.0.0",
		},
	}
	bodyBytes, _ := json.Marshal(body)

	r := httptest.NewRequest(http.MethodPost, "/api/v1/runbook/interventions/int-values-1/respond", bytes.NewReader(bodyBytes))
	r.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	h.handleIntervention(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp InterventionResponse
	json.Unmarshal(w.Body.Bytes(), &resp)

	if resp.Response == nil {
		t.Fatal("expected response to be set")
	}
	if resp.Response.Values["version"] != "1.0.0" {
		t.Errorf("Values[version] = %v, want %q", resp.Response.Values["version"], "1.0.0")
	}
}

func TestCancelIntervention(t *testing.T) {
	h, cleanup := setupApprovalHandler(t)
	defer cleanup()

	ctx := context.Background()
	now := time.Now()
	req := &intervention.Request{
		ID:          "int-cancel-1",
		ExecutionID: "exec-1",
		StepName:    "step1",
		Type:        intervention.InterventionTypeWaitManual,
		State:       intervention.InterventionStatePending,
		Title:       "Cancel Test",
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	h.interventionStorage.SaveRequest(ctx, req)

	body := struct {
		Reason string `json:"reason"`
	}{Reason: "No longer needed"}
	bodyBytes, _ := json.Marshal(body)

	r := httptest.NewRequest(http.MethodPost, "/api/v1/runbook/interventions/int-cancel-1/cancel", bytes.NewReader(bodyBytes))
	r.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	h.handleIntervention(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp InterventionResponse
	json.Unmarshal(w.Body.Bytes(), &resp)

	if resp.State != "cancelled" {
		t.Errorf("State = %q, want %q", resp.State, "cancelled")
	}
}

func TestMethodNotAllowed(t *testing.T) {
	h, cleanup := setupApprovalHandler(t)
	defer cleanup()

	tests := []struct {
		method string
		path   string
	}{
		{http.MethodPost, "/api/v1/runbook/approvals"},
		{http.MethodPut, "/api/v1/runbook/approvals/req-123"},
		{http.MethodPost, "/api/v1/runbook/interventions"},
		{http.MethodPut, "/api/v1/runbook/interventions/int-123"},
	}

	for _, tc := range tests {
		t.Run(tc.method+"_"+tc.path, func(t *testing.T) {
			r := httptest.NewRequest(tc.method, tc.path, nil)
			w := httptest.NewRecorder()

			if tc.path == "/api/v1/runbook/approvals" {
				h.handleApprovals(w, r)
			} else if tc.path == "/api/v1/runbook/interventions" {
				h.handleInterventions(w, r)
			} else if len(tc.path) > 25 && tc.path[:25] == "/api/v1/runbook/approvals" {
				h.handleApproval(w, r)
			} else {
				h.handleIntervention(w, r)
			}

			if w.Code != http.StatusMethodNotAllowed {
				t.Errorf("expected status 405, got %d", w.Code)
			}
		})
	}
}

func TestRegisterRoutes(t *testing.T) {
	h, cleanup := setupApprovalHandler(t)
	defer cleanup()

	mux := http.NewServeMux()
	h.RegisterRoutes(mux)

	// Just verify registration doesn't panic
}
