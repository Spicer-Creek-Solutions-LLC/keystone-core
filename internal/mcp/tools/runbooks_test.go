package tools

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)


func TestRunbookList(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/runbooks" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode([]map[string]string{
			{"id": "rb-1", "name": "deploy"},
			{"id": "rb-2", "name": "rollback"},
		})
	}))
	defer srv.Close()

	client := testClient(nil, nil, nil, srv.URL)
	registrar := runbookRegistrar(client)
	result := callTool(t, registrar, "runbook_list", nil)

	if result.IsError {
		t.Errorf("unexpected error: %s", textContent(t, result))
	}
	text := textContent(t, result)
	if !strings.Contains(text, "rb-1") || !strings.Contains(text, "deploy") {
		t.Errorf("expected runbook data, got: %s", text)
	}
}

func TestRunbookList_NoBaseURL(t *testing.T) {
	client := testClient(nil, nil, nil, "")
	registrar := runbookRegistrar(client)
	result := callTool(t, registrar, "runbook_list", nil)
	if !result.IsError {
		t.Error("expected error when REST base URL not configured")
	}
}

func TestRunbookList_ServerError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte("internal error"))
	}))
	defer srv.Close()

	client := testClient(nil, nil, nil, srv.URL)
	registrar := runbookRegistrar(client)
	result := callTool(t, registrar, "runbook_list", nil)
	if !result.IsError {
		t.Error("expected error for 500 response")
	}
}

func TestRunbookExecute(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}
		if !strings.HasSuffix(r.URL.Path, "/rb-1/execute") {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		w.WriteHeader(http.StatusAccepted)
		json.NewEncoder(w).Encode(map[string]string{
			"execution_id": "exec-1",
			"status":       "running",
		})
	}))
	defer srv.Close()

	client := testClient(nil, nil, nil, srv.URL)
	registrar := runbookRegistrar(client)
	result := callTool(t, registrar, "runbook_execute", map[string]any{
		"runbook_id": "rb-1",
	})

	if result.IsError {
		t.Errorf("unexpected error: %s", textContent(t, result))
	}
	text := textContent(t, result)
	if !strings.Contains(text, "exec-1") {
		t.Errorf("expected execution ID, got: %s", text)
	}
}

func TestRunbookExecute_MissingID(t *testing.T) {
	client := testClient(nil, nil, nil, "http://localhost")
	registrar := runbookRegistrar(client)
	err := callToolExpectError(t, registrar, "runbook_execute", map[string]any{})
	if !strings.Contains(err.Error(), "runbook_id") {
		t.Errorf("expected error about runbook_id, got: %v", err)
	}
}

func TestRunbookStatus(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.URL.Path, "/rb-1/executions/exec-1") {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		json.NewEncoder(w).Encode(map[string]string{
			"execution_id": "exec-1",
			"status":       "completed",
		})
	}))
	defer srv.Close()

	client := testClient(nil, nil, nil, srv.URL)
	registrar := runbookRegistrar(client)
	result := callTool(t, registrar, "runbook_status", map[string]any{
		"runbook_id":   "rb-1",
		"execution_id": "exec-1",
	})

	if result.IsError {
		t.Errorf("unexpected error: %s", textContent(t, result))
	}
	text := textContent(t, result)
	if !strings.Contains(text, "completed") {
		t.Errorf("expected status, got: %s", text)
	}
}

func TestRunbookStatus_MissingFields(t *testing.T) {
	client := testClient(nil, nil, nil, "http://localhost")
	registrar := runbookRegistrar(client)
	err := callToolExpectError(t, registrar, "runbook_status", map[string]any{
		"runbook_id": "rb-1",
	})
	if !strings.Contains(err.Error(), "execution_id") {
		t.Errorf("expected error about execution_id, got: %v", err)
	}
}
