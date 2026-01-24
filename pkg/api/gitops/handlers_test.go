package gitops

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestVerificationResponseStructure(t *testing.T) {
	resp := VerificationResponse{
		ID:           "verification-123",
		WorkflowName: "deploy-verification",
		Success:      true,
		Steps: []VerificationStepResponse{
			{
				StepName:  "health-check",
				Success:   true,
				Message:   "All pods healthy",
				Duration:  "5s",
				Timestamp: time.Now(),
			},
		},
		TotalSteps:   3,
		PassedSteps:  3,
		FailedSteps:  0,
		SkippedSteps: 0,
		Duration:     "15s",
		StartTime:    time.Now().Add(-15 * time.Second),
		EndTime:      time.Now(),
	}

	if resp.ID != "verification-123" {
		t.Errorf("ID = %v", resp.ID)
	}
	if !resp.Success {
		t.Error("Success should be true")
	}
	if resp.TotalSteps != 3 {
		t.Errorf("TotalSteps = %d", resp.TotalSteps)
	}
	if resp.PassedSteps != 3 {
		t.Errorf("PassedSteps = %d", resp.PassedSteps)
	}
	if len(resp.Steps) != 1 {
		t.Errorf("Steps count = %d", len(resp.Steps))
	}
}

func TestVerificationStepResponseStructure(t *testing.T) {
	step := VerificationStepResponse{
		StepName:  "connectivity-test",
		Success:   false,
		Message:   "Connection timed out",
		Data:      map[string]interface{}{"host": "api.example.com", "port": 443},
		Duration:  "30s",
		Timestamp: time.Now(),
		Error:     "connection refused",
		Retries:   3,
	}

	if step.StepName != "connectivity-test" {
		t.Errorf("StepName = %v", step.StepName)
	}
	if step.Success {
		t.Error("Success should be false")
	}
	if step.Retries != 3 {
		t.Errorf("Retries = %d", step.Retries)
	}
	if step.Data["port"] != 443 {
		t.Errorf("Data[port] = %v", step.Data["port"])
	}
}

func TestVerificationListResponseStructure(t *testing.T) {
	resp := VerificationListResponse{
		Verifications: []VerificationResponse{
			{ID: "v1", WorkflowName: "deploy", Success: true},
			{ID: "v2", WorkflowName: "rollback", Success: false},
		},
		Total:       10,
		Limit:       50,
		Offset:      0,
		RetrievedAt: time.Now(),
	}

	if len(resp.Verifications) != 2 {
		t.Errorf("Verifications count = %d", len(resp.Verifications))
	}
	if resp.Total != 10 {
		t.Errorf("Total = %d", resp.Total)
	}
}

func TestRollbackRequestStructure(t *testing.T) {
	req := RollbackRequest{
		Application:      "myapp",
		Namespace:        "production",
		Type:             "argocd",
		Strategy:         "previous",
		Revision:         "abc123",
		Reason:           "deployment failed health check",
		RequestedBy:      "admin@example.com",
		SkipVerification: false,
		RequireApproval:  true,
	}

	if req.Application != "myapp" {
		t.Errorf("Application = %v", req.Application)
	}
	if req.Namespace != "production" {
		t.Errorf("Namespace = %v", req.Namespace)
	}
	if req.Type != "argocd" {
		t.Errorf("Type = %v", req.Type)
	}
	if req.Strategy != "previous" {
		t.Errorf("Strategy = %v", req.Strategy)
	}
	if !req.RequireApproval {
		t.Error("RequireApproval should be true")
	}
}

func TestRollbackResponseStructure(t *testing.T) {
	resp := RollbackResponse{
		ID:               "rollback-123",
		Application:      "myapp",
		Namespace:        "production",
		Type:             "argocd",
		Strategy:         "previous",
		Status:           "completed",
		PreviousRevision: "abc123",
		CurrentRevision:  "def456",
		Message:          "Rollback completed successfully",
		Duration:         "45s",
		StartTime:        time.Now().Add(-45 * time.Second),
		EndTime:          time.Now(),
		ApprovalInfo: &ApprovalInfoResponse{
			Required:   true,
			Status:     "approved",
			ApprovedBy: "admin@example.com",
			ApprovedAt: time.Now().Add(-30 * time.Second),
		},
	}

	if resp.ID != "rollback-123" {
		t.Errorf("ID = %v", resp.ID)
	}
	if resp.Status != "completed" {
		t.Errorf("Status = %v", resp.Status)
	}
	if resp.ApprovalInfo == nil {
		t.Fatal("ApprovalInfo should not be nil")
	}
	if resp.ApprovalInfo.Status != "approved" {
		t.Errorf("ApprovalInfo.Status = %v", resp.ApprovalInfo.Status)
	}
}

func TestApprovalInfoResponseStructure(t *testing.T) {
	info := ApprovalInfoResponse{
		Required:   true,
		Status:     "rejected",
		RejectedBy: "security@example.com",
		RejectedAt: time.Now(),
		Reason:     "Insufficient testing evidence",
	}

	if !info.Required {
		t.Error("Required should be true")
	}
	if info.Status != "rejected" {
		t.Errorf("Status = %v", info.Status)
	}
	if info.RejectedBy != "security@example.com" {
		t.Errorf("RejectedBy = %v", info.RejectedBy)
	}
}

func TestRollbackListResponseStructure(t *testing.T) {
	resp := RollbackListResponse{
		Rollbacks: []RollbackResponse{
			{ID: "r1", Application: "app1", Status: "completed"},
			{ID: "r2", Application: "app2", Status: "pending"},
		},
		Total:       50,
		Limit:       20,
		Offset:      0,
		RetrievedAt: time.Now(),
	}

	if len(resp.Rollbacks) != 2 {
		t.Errorf("Rollbacks count = %d", len(resp.Rollbacks))
	}
	if resp.Total != 50 {
		t.Errorf("Total = %d", resp.Total)
	}
	if resp.Limit != 20 {
		t.Errorf("Limit = %d", resp.Limit)
	}
}

func TestApprovalRequestStructure(t *testing.T) {
	req := ApprovalRequest{
		Approved:   true,
		ApprovedBy: "admin@example.com",
		Reason:     "Change approved per CR-123",
	}

	if !req.Approved {
		t.Error("Approved should be true")
	}
	if req.ApprovedBy != "admin@example.com" {
		t.Errorf("ApprovedBy = %v", req.ApprovedBy)
	}
}

func TestWriteJSON(t *testing.T) {
	w := httptest.NewRecorder()
	data := map[string]interface{}{
		"id":     "verification-123",
		"status": "completed",
	}

	writeJSON(w, http.StatusOK, data)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", w.Code, http.StatusOK)
	}
	if w.Header().Get("Content-Type") != "application/json" {
		t.Errorf("Content-Type = %v", w.Header().Get("Content-Type"))
	}

	var result map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
		t.Fatalf("failed to unmarshal response: %v", err)
	}
	if result["id"] != "verification-123" {
		t.Errorf("result[id] = %v", result["id"])
	}
}

func TestWriteError(t *testing.T) {
	w := httptest.NewRecorder()

	writeError(w, http.StatusNotFound, "verification not found")

	if w.Code != http.StatusNotFound {
		t.Errorf("status = %d, want %d", w.Code, http.StatusNotFound)
	}

	var result map[string]string
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
		t.Fatalf("failed to unmarshal response: %v", err)
	}
	if result["error"] != "verification not found" {
		t.Errorf("result[error] = %v", result["error"])
	}
}

func TestNewHandler(t *testing.T) {
	handler := NewHandler(nil, nil)
	if handler == nil {
		t.Fatal("handler should not be nil")
	}
	if handler.verificationResults == nil {
		t.Error("verificationResults map should be initialized")
	}
}

func TestRegisterRoutes(t *testing.T) {
	handler := NewHandler(nil, nil)
	mux := http.NewServeMux()

	handler.RegisterRoutes(mux)

	// Test that routes are registered
	t.Run("verifications endpoint", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/api/v1/gitops/verifications", nil)
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)

		// POST should return method not allowed
		if w.Code != http.StatusMethodNotAllowed {
			t.Errorf("POST /api/v1/gitops/verifications status = %d, want %d", w.Code, http.StatusMethodNotAllowed)
		}
	})

	t.Run("rollbacks endpoint", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodDelete, "/api/v1/gitops/rollbacks", nil)
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)

		// DELETE should return method not allowed
		if w.Code != http.StatusMethodNotAllowed {
			t.Errorf("DELETE /api/v1/gitops/rollbacks status = %d, want %d", w.Code, http.StatusMethodNotAllowed)
		}
	})

	t.Run("rollback endpoint", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/gitops/rollback", nil)
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)

		// GET should return method not allowed
		if w.Code != http.StatusMethodNotAllowed {
			t.Errorf("GET /api/v1/gitops/rollback status = %d, want %d", w.Code, http.StatusMethodNotAllowed)
		}
	})
}

func TestRollbackRequestJSONMarshal(t *testing.T) {
	req := RollbackRequest{
		Application:     "myapp",
		Type:            "argocd",
		Reason:          "test",
		RequireApproval: true,
	}

	data, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("failed to marshal: %v", err)
	}

	var unmarshaled RollbackRequest
	if err := json.Unmarshal(data, &unmarshaled); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	if unmarshaled.Application != req.Application {
		t.Errorf("Application = %v, want %v", unmarshaled.Application, req.Application)
	}
	if unmarshaled.Type != req.Type {
		t.Errorf("Type = %v, want %v", unmarshaled.Type, req.Type)
	}
}

func TestRollbackResponseJSONMarshal(t *testing.T) {
	resp := RollbackResponse{
		ID:          "rollback-789",
		Application: "testapp",
		Status:      "pending",
		StartTime:   time.Now().UTC(),
	}

	data, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("failed to marshal: %v", err)
	}

	var unmarshaled RollbackResponse
	if err := json.Unmarshal(data, &unmarshaled); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	if unmarshaled.ID != resp.ID {
		t.Errorf("ID = %v, want %v", unmarshaled.ID, resp.ID)
	}
	if unmarshaled.Status != resp.Status {
		t.Errorf("Status = %v, want %v", unmarshaled.Status, resp.Status)
	}
}

func TestVerificationListEndpointGetEmpty(t *testing.T) {
	handler := NewHandler(nil, nil)
	mux := http.NewServeMux()
	handler.RegisterRoutes(mux)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/gitops/verifications", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("GET /api/v1/gitops/verifications status = %d, want %d", w.Code, http.StatusOK)
	}

	var resp VerificationListResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to unmarshal response: %v", err)
	}

	if resp.Total != 0 {
		t.Errorf("Total = %d, want 0", resp.Total)
	}
}

func TestVerificationByIDNotFound(t *testing.T) {
	handler := NewHandler(nil, nil)
	mux := http.NewServeMux()
	handler.RegisterRoutes(mux)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/gitops/verifications/nonexistent", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("GET /api/v1/gitops/verifications/nonexistent status = %d, want %d", w.Code, http.StatusNotFound)
	}
}
