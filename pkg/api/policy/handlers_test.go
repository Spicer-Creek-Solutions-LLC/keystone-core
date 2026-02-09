package policy

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/shawnbutts/keystone-core/internal/policy"
)

func TestEvaluateRequestStructure(t *testing.T) {
	req := EvaluateRequest{
		PolicyID:     "policy-123",
		PolicySetID:  "set-456",
		ResourceType: "deployment",
		Resource: map[string]interface{}{
			"name":      "myapp",
			"namespace": "default",
			"replicas":  3,
		},
		Action: "create",
		User:   "admin@example.com",
		Context: map[string]interface{}{
			"cluster": "prod-us-east",
			"env":     "production",
		},
	}

	if req.PolicyID != "policy-123" {
		t.Errorf("PolicyID = %v", req.PolicyID)
	}
	if req.Action != "create" {
		t.Errorf("Action = %v", req.Action)
	}
	if req.Resource.(map[string]interface{})["name"] != "myapp" {
		t.Errorf("Resource[name] = %v", req.Resource)
	}
	if req.Context["cluster"] != "prod-us-east" {
		t.Errorf("Context[cluster] = %v", req.Context["cluster"])
	}
}

func TestEvaluateResponseStructure(t *testing.T) {
	resp := EvaluateResponse{
		Allowed: true,
		Results: []EvaluationResultResponse{
			{
				PolicyID:   "policy-1",
				PolicyName: "require-labels",
				Allowed:    true,
				Message:    "All required labels present",
				Duration:   "5ms",
			},
		},
		Summary: &policy.Summary{
			TotalPolicies:   1,
			AllowedPolicies: 1,
			DeniedPolicies:  0,
		},
		TotalDuration: "10ms",
		EvaluatedAt:   time.Now(),
	}

	if !resp.Allowed {
		t.Error("Allowed should be true")
	}
	if len(resp.Results) != 1 {
		t.Errorf("Results count = %d", len(resp.Results))
	}
	if resp.Summary.TotalPolicies != 1 {
		t.Errorf("Summary.TotalPolicies = %d", resp.Summary.TotalPolicies)
	}
}

func TestEvaluationResultResponseStructure(t *testing.T) {
	result := EvaluationResultResponse{
		PolicyID:   "policy-123",
		PolicyName: "max-replicas",
		Allowed:    false,
		Violations: []policy.Violation{
			{
				Rule:     "replicas-limit",
				Message:  "Replicas exceed maximum of 10",
				Severity: policy.SeverityHigh,
			},
		},
		Warnings:    []string{"Consider using autoscaling"},
		Message:     "Policy violated",
		Duration:    "3ms",
		EvaluatedAt: time.Now(),
	}

	if result.Allowed {
		t.Error("Allowed should be false")
	}
	if len(result.Violations) != 1 {
		t.Errorf("Violations count = %d", len(result.Violations))
	}
	if result.Violations[0].Severity != policy.SeverityHigh {
		t.Errorf("Violations[0].Severity = %v", result.Violations[0].Severity)
	}
}

func TestViolationsResponseStructure(t *testing.T) {
	resp := ViolationsResponse{
		Violations: []ViolationEntry{
			{
				ID:           "v-123",
				Timestamp:    time.Now(),
				PolicyID:     "policy-1",
				PolicyName:   "require-labels",
				ResourceType: "deployment",
				User:         "user@example.com",
				Action:       "create",
				Violations: []policy.Violation{
					{Rule: "labels", Message: "Missing required label"},
				},
				EnforcementMode: "enforce",
			},
		},
		Summary: &policy.AuditSummary{
			TotalEvaluations:   100,
			AllowedEvaluations: 95,
			DeniedEvaluations:  5,
		},
		Total:       10,
		Limit:       100,
		RetrievedAt: time.Now(),
	}

	if len(resp.Violations) != 1 {
		t.Errorf("Violations count = %d", len(resp.Violations))
	}
	if resp.Total != 10 {
		t.Errorf("Total = %d", resp.Total)
	}
	if resp.Summary.TotalEvaluations != 100 {
		t.Errorf("Summary.TotalEvaluations = %d", resp.Summary.TotalEvaluations)
	}
}

func TestViolationEntryStructure(t *testing.T) {
	entry := ViolationEntry{
		ID:              "violation-456",
		Timestamp:       time.Now(),
		PolicyID:        "policy-secure",
		PolicyName:      "container-security",
		ResourceType:    "pod",
		User:            "developer@example.com",
		Action:          "create",
		EnforcementMode: "warn",
	}

	if entry.ID != "violation-456" {
		t.Errorf("ID = %v", entry.ID)
	}
	if entry.EnforcementMode != "warn" {
		t.Errorf("EnforcementMode = %v", entry.EnforcementMode)
	}
}

func TestComplianceResponseStructure(t *testing.T) {
	resp := ComplianceResponse{
		Report: &ComplianceReportResponse{
			TotalPolicies:     20,
			CompliantPolicies: 18,
			ComplianceRate:    0.90,
			TopViolations: []ViolationSummaryResponse{
				{PolicyID: "p1", PolicyName: "labels", Count: 5, Severity: "high"},
				{PolicyID: "p2", PolicyName: "resources", Count: 3, Severity: "medium"},
			},
			SeverityBreakdown: map[string]int{
				"high":   5,
				"medium": 3,
				"low":    2,
			},
		},
		Period: PeriodResponse{
			Start: time.Now().Add(-24 * time.Hour),
			End:   time.Now(),
		},
		GeneratedAt: time.Now(),
	}

	if resp.Report.ComplianceRate != 0.90 {
		t.Errorf("ComplianceRate = %v", resp.Report.ComplianceRate)
	}
	if len(resp.Report.TopViolations) != 2 {
		t.Errorf("TopViolations count = %d", len(resp.Report.TopViolations))
	}
	if resp.Report.SeverityBreakdown["high"] != 5 {
		t.Errorf("SeverityBreakdown[high] = %d", resp.Report.SeverityBreakdown["high"])
	}
}

func TestComplianceReportResponseStructure(t *testing.T) {
	report := ComplianceReportResponse{
		TotalPolicies:     50,
		CompliantPolicies: 45,
		ComplianceRate:    0.90,
		SeverityBreakdown: map[string]int{
			"critical": 1,
			"high":     4,
		},
	}

	if report.TotalPolicies != 50 {
		t.Errorf("TotalPolicies = %d", report.TotalPolicies)
	}
	if report.ComplianceRate != 0.90 {
		t.Errorf("ComplianceRate = %v", report.ComplianceRate)
	}
}

func TestViolationSummaryResponseStructure(t *testing.T) {
	summary := ViolationSummaryResponse{
		PolicyID:   "policy-123",
		PolicyName: "require-annotations",
		Count:      15,
		Severity:   "medium",
	}

	if summary.PolicyID != "policy-123" {
		t.Errorf("PolicyID = %v", summary.PolicyID)
	}
	if summary.Count != 15 {
		t.Errorf("Count = %d", summary.Count)
	}
}

func TestPeriodResponseStructure(t *testing.T) {
	now := time.Now()
	period := PeriodResponse{
		Start: now.Add(-7 * 24 * time.Hour),
		End:   now,
	}

	if period.End.Before(period.Start) {
		t.Error("End should be after Start")
	}
}

func TestWriteJSON(t *testing.T) {
	w := httptest.NewRecorder()
	data := map[string]interface{}{
		"allowed": true,
		"message": "Policy evaluation passed",
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
	if result["allowed"] != true {
		t.Errorf("result[allowed] = %v", result["allowed"])
	}
}

func TestWriteError(t *testing.T) {
	w := httptest.NewRecorder()

	writeError(w, http.StatusBadRequest, "action is required")

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}

	var result map[string]string
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
		t.Fatalf("failed to unmarshal response: %v", err)
	}
	if result["error"] != "action is required" {
		t.Errorf("result[error] = %v", result["error"])
	}
}

func TestNewHandler(t *testing.T) {
	handler := NewHandler(nil, nil, nil)
	if handler == nil {
		t.Fatal("handler should not be nil")
	}
}

func TestRegisterRoutes(t *testing.T) {
	handler := NewHandler(nil, nil, nil)
	mux := http.NewServeMux()

	handler.RegisterRoutes(mux)

	// Test that routes are registered
	t.Run("evaluate endpoint", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/policies/evaluate", nil)
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)

		// GET should return method not allowed
		if w.Code != http.StatusMethodNotAllowed {
			t.Errorf("GET /api/v1/policies/evaluate status = %d, want %d", w.Code, http.StatusMethodNotAllowed)
		}
	})

	t.Run("violations endpoint", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/api/v1/policies/violations", nil)
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)

		// POST should return method not allowed
		if w.Code != http.StatusMethodNotAllowed {
			t.Errorf("POST /api/v1/policies/violations status = %d, want %d", w.Code, http.StatusMethodNotAllowed)
		}
	})

	t.Run("compliance endpoint", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/api/v1/policies/compliance", nil)
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)

		// POST should return method not allowed
		if w.Code != http.StatusMethodNotAllowed {
			t.Errorf("POST /api/v1/policies/compliance status = %d, want %d", w.Code, http.StatusMethodNotAllowed)
		}
	})
}

func TestEvaluateRequestJSONMarshal(t *testing.T) {
	req := EvaluateRequest{
		PolicyID: "policy-123",
		Action:   "create",
		Resource: map[string]interface{}{
			"name": "test",
		},
	}

	data, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("failed to marshal: %v", err)
	}

	var unmarshaled EvaluateRequest
	if err := json.Unmarshal(data, &unmarshaled); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	if unmarshaled.PolicyID != req.PolicyID {
		t.Errorf("PolicyID = %v, want %v", unmarshaled.PolicyID, req.PolicyID)
	}
	if unmarshaled.Action != req.Action {
		t.Errorf("Action = %v, want %v", unmarshaled.Action, req.Action)
	}
}

func TestEvaluateResponseJSONMarshal(t *testing.T) {
	resp := EvaluateResponse{
		Allowed:       true,
		TotalDuration: "5ms",
		EvaluatedAt:   time.Now().UTC(),
	}

	data, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("failed to marshal: %v", err)
	}

	var unmarshaled EvaluateResponse
	if err := json.Unmarshal(data, &unmarshaled); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	if !unmarshaled.Allowed {
		t.Error("Allowed should be true")
	}
	if unmarshaled.TotalDuration != resp.TotalDuration {
		t.Errorf("TotalDuration = %v, want %v", unmarshaled.TotalDuration, resp.TotalDuration)
	}
}

func TestComplianceResponseJSONMarshal(t *testing.T) {
	resp := ComplianceResponse{
		Report: &ComplianceReportResponse{
			TotalPolicies:     10,
			CompliantPolicies: 9,
			ComplianceRate:    0.9,
			SeverityBreakdown: map[string]int{"low": 1},
		},
		Period: PeriodResponse{
			Start: time.Now().Add(-24 * time.Hour).UTC(),
			End:   time.Now().UTC(),
		},
		GeneratedAt: time.Now().UTC(),
	}

	data, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("failed to marshal: %v", err)
	}

	var unmarshaled ComplianceResponse
	if err := json.Unmarshal(data, &unmarshaled); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	if unmarshaled.Report.ComplianceRate != resp.Report.ComplianceRate {
		t.Errorf("ComplianceRate = %v, want %v", unmarshaled.Report.ComplianceRate, resp.Report.ComplianceRate)
	}
}
