package state

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/shawnbutts/keystone-core/internal/statemgmt"
)

func TestHandleApply_MethodNotAllowed(t *testing.T) {
	handler := NewHandler()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/state/apply", nil)
	rec := httptest.NewRecorder()

	handler.handleApply(rec, req)

	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("Expected status 405, got %d", rec.Code)
	}
}

func TestHandleApply_InvalidJSON(t *testing.T) {
	handler := NewHandler()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/state/apply", strings.NewReader("{invalid"))
	rec := httptest.NewRecorder()

	handler.handleApply(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("Expected status 400, got %d", rec.Code)
	}
}

func TestHandleApply_Success(t *testing.T) {
	handler := NewHandler()
	tmpDir := t.TempDir()
	target := filepath.Join(tmpDir, "apply.txt")

	content := `
file:
  ` + target + `:
    state: present
    contents: hello
`

	body := StateRequest{
		Content: content,
		DryRun:  false,
	}
	payload, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("Failed to marshal request: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/api/v1/state/apply", bytes.NewReader(payload))
	rec := httptest.NewRecorder()

	handler.handleApply(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("Expected status 200, got %d", rec.Code)
	}

	var resp StateApplyResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	if resp.Summary == nil || resp.Summary.Total != 1 {
		t.Fatalf("Expected summary total 1, got %#v", resp.Summary)
	}
	if resp.Status != "changed" && resp.Status != "success" {
		t.Fatalf("Unexpected status: %s", resp.Status)
	}

	data, err := os.ReadFile(target)
	if err != nil {
		t.Fatalf("Expected file to be created: %v", err)
	}
	if string(data) != "hello" {
		t.Fatalf("Unexpected file contents: %q", string(data))
	}
}

func TestHandleCheck_Success(t *testing.T) {
	handler := NewHandler()

	content := `
git:
  /opt/repo:
    state: present
`

	body := StateRequest{
		Content: content,
	}
	payload, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("Failed to marshal request: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/api/v1/state/check", bytes.NewReader(payload))
	rec := httptest.NewRecorder()

	handler.handleCheck(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("Expected status 200, got %d", rec.Code)
	}

	var resp StateCheckResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	if resp.Valid {
		t.Fatal("Expected validation to fail due to missing fields")
	}
	if len(resp.Errors) == 0 {
		t.Fatal("Expected validation errors")
	}
	if resp.States != 1 {
		t.Fatalf("Expected 1 module, got %d", resp.States)
	}
	if !containsString(resp.Modules, "git") {
		t.Fatalf("Expected modules to include git, got %v", resp.Modules)
	}
}

func TestHandleDrift_Success(t *testing.T) {
	handler := NewHandler()
	tmpDir := t.TempDir()
	target := filepath.Join(tmpDir, "drift.txt")
	if err := os.WriteFile(target, []byte("old"), 0644); err != nil {
		t.Fatalf("Failed to create file: %v", err)
	}

	content := `
file:
  ` + target + `:
    state: present
    contents: new
`

	body := StateRequest{
		Content: content,
	}
	payload, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("Failed to marshal request: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/api/v1/state/drift", bytes.NewReader(payload))
	rec := httptest.NewRecorder()

	handler.handleDrift(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("Expected status 200, got %d", rec.Code)
	}

	var resp DriftResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	if resp.Summary == nil || resp.Summary.Total != 1 {
		t.Fatalf("Expected summary total 1, got %#v", resp.Summary)
	}
	if !resp.HasDrift {
		t.Fatal("Expected drift to be detected")
	}
}

func TestLoadStateFile_Errors(t *testing.T) {
	handler := NewHandler()

	if _, err := handler.loadStateFile(StateRequest{}); err == nil {
		t.Fatal("Expected error when no content or path is provided")
	}

	if _, err := handler.loadStateFile(StateRequest{Path: "does-not-exist.yaml"}); err == nil {
		t.Fatal("Expected error when path does not exist")
	}
}

func TestLoadStateFile_Content(t *testing.T) {
	handler := NewHandler()
	content := `
file:
  /tmp/example.txt:
    state: present
    contents: hello
`
	stateFile, err := handler.loadStateFile(StateRequest{Content: content})
	if err != nil {
		t.Fatalf("loadStateFile failed: %v", err)
	}
	if len(stateFile.States) != 1 {
		t.Fatalf("Expected 1 module, got %d", len(stateFile.States))
	}
}

func TestParseStateID(t *testing.T) {
	parts := parseStateID("file./tmp/test.txt")
	if len(parts) != 2 || parts[0] != "file" {
		t.Fatalf("Unexpected parseStateID result: %v", parts)
	}
	if parseStateID("file") != nil {
		t.Fatal("Expected nil for unqualified state ID")
	}
}

func TestConvertHelpers(t *testing.T) {
	summary := convertSummary(&statemgmt.RunSummary{
		Total:     2,
		Succeeded: 1,
		Failed:    1,
		Changed:   1,
		Unchanged: 1,
	})
	if summary.Total != 2 || summary.Failed != 1 {
		t.Fatalf("Unexpected summary conversion: %+v", summary)
	}

	results := convertResults([]*statemgmt.StateResult{
		{
			StateID:  "one",
			Module:   "file",
			Success:  false,
			Changed:  false,
			Comment:  "failed",
			Error:    fmt.Errorf("boom"),
			Duration: 0,
		},
	})
	if len(results) != 1 || results[0].Status != "failed" {
		t.Fatalf("Unexpected results conversion: %+v", results)
	}

	drift := convertDriftSummary(&statemgmt.DriftSummary{
		Total:         1,
		NoDrift:       0,
		LowDrift:      1,
		HighDrift:     0,
		MediumDrift:   0,
		CriticalDrift: 0,
	})
	if drift.Total != 1 || drift.Low != 1 {
		t.Fatalf("Unexpected drift summary conversion: %+v", drift)
	}

	diffs := convertDifferences([]statemgmt.Difference{
		{
			Path:     "contents",
			Desired:  "old",
			Actual:   "new",
			Severity: statemgmt.DriftMedium,
			Message:  "changed",
		},
	})
	if len(diffs) != 1 || diffs[0].Severity != "medium" {
		t.Fatalf("Unexpected drift diff conversion: %+v", diffs)
	}
}

func containsString(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}
