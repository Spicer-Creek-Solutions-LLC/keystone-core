package execution

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	pb "github.com/shawnbutts/keystone-core/pkg/api/v1"
)

func TestExecuteRequestStructure(t *testing.T) {
	req := ExecuteRequest{
		Targets: []string{"agent-1", "agent-2"},
		Target:  "agent-1",
		Command: "uname",
		Args:    []string{"-a"},
		Env: map[string]string{
			"PATH": "/usr/bin",
		},
		WorkingDir: "/tmp",
		User:       "root",
		Shell:      "/bin/bash",
		Timeout:    30,
		Async:      true,
	}

	if len(req.Targets) != 2 {
		t.Errorf("Targets count = %d", len(req.Targets))
	}
	if req.Command != "uname" {
		t.Errorf("Command = %v", req.Command)
	}
	if req.Timeout != 30 {
		t.Errorf("Timeout = %d", req.Timeout)
	}
	if !req.Async {
		t.Error("Async should be true")
	}
}

func TestExecuteResponseStructure(t *testing.T) {
	resp := ExecuteResponse{
		JobID:   "job-123",
		Status:  "dispatched",
		Targets: []string{"agent-1", "agent-2"},
		Results: map[string]Result{
			"agent-1": {
				AgentID:  "agent-1",
				Status:   "success",
				ExitCode: 0,
				Stdout:   "Linux server1 5.4.0",
			},
		},
		CreatedAt: time.Now(),
	}

	if resp.JobID != "job-123" {
		t.Errorf("JobID = %v", resp.JobID)
	}
	if resp.Status != "dispatched" {
		t.Errorf("Status = %v", resp.Status)
	}
	if len(resp.Targets) != 2 {
		t.Errorf("Targets count = %d", len(resp.Targets))
	}
	if resp.Results["agent-1"].ExitCode != 0 {
		t.Errorf("Results[agent-1].ExitCode = %d", resp.Results["agent-1"].ExitCode)
	}
}

func TestResultStructure(t *testing.T) {
	now := time.Now()
	result := Result{
		AgentID:     "agent-123",
		Status:      "success",
		ExitCode:    0,
		Stdout:      "command output",
		Stderr:      "",
		Error:       "",
		StartedAt:   now.Add(-5 * time.Second),
		CompletedAt: now,
		DurationMs:  5000,
	}

	if result.AgentID != "agent-123" {
		t.Errorf("AgentID = %v", result.AgentID)
	}
	if result.Status != "success" {
		t.Errorf("Status = %v", result.Status)
	}
	if result.DurationMs != 5000 {
		t.Errorf("DurationMs = %d", result.DurationMs)
	}
}

func TestJobResponseStructure(t *testing.T) {
	now := time.Now()
	job := JobResponse{
		ID:          "job-456",
		AgentID:     "agent-123",
		Command:     "ls",
		Args:        []string{"-la", "/tmp"},
		Status:      "success",
		ExitCode:    0,
		Stdout:      "total 0",
		Stderr:      "",
		Error:       "",
		CreatedAt:   now.Add(-10 * time.Second),
		StartedAt:   now.Add(-9 * time.Second),
		CompletedAt: now,
		DurationMs:  9000,
	}

	if job.ID != "job-456" {
		t.Errorf("ID = %v", job.ID)
	}
	if job.Command != "ls" {
		t.Errorf("Command = %v", job.Command)
	}
	if len(job.Args) != 2 {
		t.Errorf("Args count = %d", len(job.Args))
	}
}

func TestJobListResponseStructure(t *testing.T) {
	resp := JobListResponse{
		Jobs: []JobResponse{
			{ID: "job-1", Command: "ls"},
			{ID: "job-2", Command: "cat"},
		},
		Total:       100,
		Limit:       50,
		Offset:      0,
		RetrievedAt: time.Now(),
	}

	if len(resp.Jobs) != 2 {
		t.Errorf("Jobs count = %d", len(resp.Jobs))
	}
	if resp.Total != 100 {
		t.Errorf("Total = %d", resp.Total)
	}
	if resp.Limit != 50 {
		t.Errorf("Limit = %d", resp.Limit)
	}
}

func TestCommandStatusToString(t *testing.T) {
	tests := []struct {
		name     string
		status   pb.CommandStatus
		expected string
	}{
		{"pending", pb.CommandStatus_COMMAND_STATUS_PENDING, "pending"},
		{"running", pb.CommandStatus_COMMAND_STATUS_RUNNING, "running"},
		{"completed", pb.CommandStatus_COMMAND_STATUS_COMPLETED, "success"},
		{"failed", pb.CommandStatus_COMMAND_STATUS_FAILED, "failed"},
		{"timeout", pb.CommandStatus_COMMAND_STATUS_TIMEOUT, "timeout"},
		{"cancelled", pb.CommandStatus_COMMAND_STATUS_CANCELLED, "cancelled"},
		{"unspecified", pb.CommandStatus_COMMAND_STATUS_UNSPECIFIED, "unknown"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := commandStatusToString(tt.status)
			if result != tt.expected {
				t.Errorf("commandStatusToString(%v) = %v, want %v", tt.status, result, tt.expected)
			}
		})
	}
}

func TestStringToCommandStatus(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected pb.CommandStatus
	}{
		{"pending", "pending", pb.CommandStatus_COMMAND_STATUS_PENDING},
		{"running", "running", pb.CommandStatus_COMMAND_STATUS_RUNNING},
		{"success", "success", pb.CommandStatus_COMMAND_STATUS_COMPLETED},
		{"completed", "completed", pb.CommandStatus_COMMAND_STATUS_COMPLETED},
		{"failed", "failed", pb.CommandStatus_COMMAND_STATUS_FAILED},
		{"timeout", "timeout", pb.CommandStatus_COMMAND_STATUS_TIMEOUT},
		{"cancelled", "cancelled", pb.CommandStatus_COMMAND_STATUS_CANCELLED},
		{"UPPERCASE", "PENDING", pb.CommandStatus_COMMAND_STATUS_PENDING},
		{"unknown", "invalid", pb.CommandStatus_COMMAND_STATUS_PENDING},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := stringToCommandStatus(tt.input)
			if result != tt.expected {
				t.Errorf("stringToCommandStatus(%q) = %v, want %v", tt.input, result, tt.expected)
			}
		})
	}
}

func TestWriteJSON(t *testing.T) {
	w := httptest.NewRecorder()
	data := map[string]interface{}{
		"job_id": "test-123",
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
	if result["job_id"] != "test-123" {
		t.Errorf("result[job_id] = %v", result["job_id"])
	}
}

func TestWriteError(t *testing.T) {
	w := httptest.NewRecorder()

	writeError(w, http.StatusBadRequest, "command is required")

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}

	var result map[string]string
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
		t.Fatalf("failed to unmarshal response: %v", err)
	}
	if result["error"] != "command is required" {
		t.Errorf("result[error] = %v", result["error"])
	}
}

func TestNewHandler(t *testing.T) {
	handler := NewHandler(nil, nil)
	if handler == nil {
		t.Fatal("handler should not be nil")
	}
}

func TestRegisterRoutes(t *testing.T) {
	handler := NewHandler(nil, nil)
	mux := http.NewServeMux()

	handler.RegisterRoutes(mux)

	// Test that routes are registered
	t.Run("exec endpoint", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/exec", nil)
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)

		// GET should return method not allowed
		if w.Code != http.StatusMethodNotAllowed {
			t.Errorf("GET /api/v1/exec status = %d, want %d", w.Code, http.StatusMethodNotAllowed)
		}
	})

	t.Run("jobs endpoint", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/api/v1/jobs", nil)
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)

		// POST should return method not allowed
		if w.Code != http.StatusMethodNotAllowed {
			t.Errorf("POST /api/v1/jobs status = %d, want %d", w.Code, http.StatusMethodNotAllowed)
		}
	})

	t.Run("job by id endpoint", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/api/v1/jobs/123", nil)
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)

		// POST should return method not allowed
		if w.Code != http.StatusMethodNotAllowed {
			t.Errorf("POST /api/v1/jobs/123 status = %d, want %d", w.Code, http.StatusMethodNotAllowed)
		}
	})
}

func TestExecuteRequestJSONMarshal(t *testing.T) {
	req := ExecuteRequest{
		Command: "echo",
		Args:    []string{"hello"},
		Targets: []string{"agent-1"},
		Timeout: 60,
	}

	data, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("failed to marshal: %v", err)
	}

	var unmarshaled ExecuteRequest
	if err := json.Unmarshal(data, &unmarshaled); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	if unmarshaled.Command != req.Command {
		t.Errorf("Command = %v, want %v", unmarshaled.Command, req.Command)
	}
	if unmarshaled.Timeout != req.Timeout {
		t.Errorf("Timeout = %d, want %d", unmarshaled.Timeout, req.Timeout)
	}
}

func TestExecuteResponseJSONMarshal(t *testing.T) {
	resp := ExecuteResponse{
		JobID:   "job-123",
		Status:  "completed",
		Targets: []string{"agent-1"},
		Results: map[string]Result{
			"agent-1": {Status: "success", ExitCode: 0},
		},
		CreatedAt: time.Now().UTC(),
	}

	data, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("failed to marshal: %v", err)
	}

	var unmarshaled ExecuteResponse
	if err := json.Unmarshal(data, &unmarshaled); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	if unmarshaled.JobID != resp.JobID {
		t.Errorf("JobID = %v, want %v", unmarshaled.JobID, resp.JobID)
	}
	if unmarshaled.Status != resp.Status {
		t.Errorf("Status = %v, want %v", unmarshaled.Status, resp.Status)
	}
}

func TestResultJSONMarshal(t *testing.T) {
	result := Result{
		AgentID:  "agent-1",
		Status:   "success",
		ExitCode: 0,
		Stdout:   "output",
		Stderr:   "",
	}

	data, err := json.Marshal(result)
	if err != nil {
		t.Fatalf("failed to marshal: %v", err)
	}

	var unmarshaled Result
	if err := json.Unmarshal(data, &unmarshaled); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	if unmarshaled.AgentID != result.AgentID {
		t.Errorf("AgentID = %v, want %v", unmarshaled.AgentID, result.AgentID)
	}
	if unmarshaled.ExitCode != result.ExitCode {
		t.Errorf("ExitCode = %d, want %d", unmarshaled.ExitCode, result.ExitCode)
	}
}
