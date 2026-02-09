package mirror

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestNewAPIHandler(t *testing.T) {
	registry := NewRegistry()
	syncEngine := NewSyncEngine(registry, DefaultSyncConfig())

	handler := NewAPIHandler(registry, syncEngine)
	if handler == nil {
		t.Fatal("NewAPIHandler returned nil")
	}
	if handler.registry != registry {
		t.Error("handler registry mismatch")
	}
	if handler.syncEngine != syncEngine {
		t.Error("handler syncEngine mismatch")
	}
}

func TestAPIHandler_RegisterRoutes(t *testing.T) {
	registry := NewRegistry()
	syncEngine := NewSyncEngine(registry, DefaultSyncConfig())
	handler := NewAPIHandler(registry, syncEngine)

	mux := http.NewServeMux()
	handler.RegisterRoutes(mux)

	// Verify routes are registered by making test requests
	tests := []struct {
		method string
		path   string
		code   int
	}{
		{"GET", "/api/v1/mirrors", http.StatusOK},
		{"GET", "/api/v1/mirrors/health", http.StatusOK},
		{"GET", "/api/v1/mirrors/sync/status?group=test", http.StatusOK},
		{"GET", "/api/v1/mirrors/sync/operations", http.StatusOK},
		{"GET", "/api/v1/mirrors/sync/history", http.StatusOK},
		{"GET", "/api/v1/mirrors/conflicts", http.StatusOK},
	}

	for _, tt := range tests {
		req := httptest.NewRequest(tt.method, tt.path, nil)
		rr := httptest.NewRecorder()
		mux.ServeHTTP(rr, req)

		if rr.Code != tt.code {
			t.Errorf("%s %s: got status %d, want %d", tt.method, tt.path, rr.Code, tt.code)
		}
	}
}

func TestAPIHandler_ListMirrors(t *testing.T) {
	registry := NewRegistry()
	syncEngine := NewSyncEngine(registry, DefaultSyncConfig())
	handler := NewAPIHandler(registry, syncEngine)

	// Add a test group
	group, _ := NewGroup(&GroupConfig{
		ID:   "test-group",
		Name: "Test Group",
		Mirrors: []*Mirror{
			{ID: "mirror-1", ClusterID: "cluster-1", Enabled: true},
			{ID: "mirror-2", ClusterID: "cluster-2", Enabled: true},
		},
		ReadStrategy: ReadStrategyRoundRobin,
		WritePolicy:  WritePolicyAll,
	})
	registry.Register(group)

	// Test list endpoint
	req := httptest.NewRequest("GET", "/api/v1/mirrors", nil)
	rr := httptest.NewRecorder()
	handler.handleMirrors(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("got status %d, want %d", rr.Code, http.StatusOK)
	}

	var response []*GroupResponse
	if err := json.NewDecoder(rr.Body).Decode(&response); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if len(response) != 1 {
		t.Fatalf("got %d groups, want 1", len(response))
	}

	if response[0].ID != "test-group" {
		t.Errorf("got ID %s, want test-group", response[0].ID)
	}
	if len(response[0].Mirrors) != 2 {
		t.Errorf("got %d mirrors, want 2", len(response[0].Mirrors))
	}
}

func TestAPIHandler_GetMirror(t *testing.T) {
	registry := NewRegistry()
	syncEngine := NewSyncEngine(registry, DefaultSyncConfig())
	handler := NewAPIHandler(registry, syncEngine)

	group, _ := NewGroup(&GroupConfig{
		ID:   "test-group",
		Name: "Test Group",
		Mirrors: []*Mirror{
			{ID: "mirror-1", ClusterID: "cluster-1", Enabled: true},
		},
		ReadStrategy: ReadStrategyRoundRobin,
		WritePolicy:  WritePolicyAll,
	})
	registry.Register(group)

	// Test GET by ID
	req := httptest.NewRequest("GET", "/api/v1/mirrors/test-group", nil)
	rr := httptest.NewRecorder()
	handler.handleMirrorByID(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("got status %d, want %d", rr.Code, http.StatusOK)
	}

	var response GroupResponse
	if err := json.NewDecoder(rr.Body).Decode(&response); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if response.ID != "test-group" {
		t.Errorf("got ID %s, want test-group", response.ID)
	}
}

func TestAPIHandler_GetMirror_NotFound(t *testing.T) {
	registry := NewRegistry()
	syncEngine := NewSyncEngine(registry, DefaultSyncConfig())
	handler := NewAPIHandler(registry, syncEngine)

	req := httptest.NewRequest("GET", "/api/v1/mirrors/nonexistent", nil)
	rr := httptest.NewRecorder()
	handler.handleMirrorByID(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Errorf("got status %d, want %d", rr.Code, http.StatusNotFound)
	}
}

func TestAPIHandler_DeleteMirror(t *testing.T) {
	registry := NewRegistry()
	syncEngine := NewSyncEngine(registry, DefaultSyncConfig())
	handler := NewAPIHandler(registry, syncEngine)

	group, _ := NewGroup(&GroupConfig{
		ID:   "test-group",
		Name: "Test Group",
		Mirrors: []*Mirror{
			{ID: "mirror-1", ClusterID: "cluster-1", Enabled: true},
		},
		ReadStrategy: ReadStrategyRoundRobin,
		WritePolicy:  WritePolicyAll,
	})
	registry.Register(group)

	// Verify it exists
	_, ok := registry.Get("test-group")
	if !ok {
		t.Fatal("group should exist before delete")
	}

	// Delete
	req := httptest.NewRequest("DELETE", "/api/v1/mirrors/test-group", nil)
	rr := httptest.NewRecorder()
	handler.handleMirrorByID(rr, req)

	if rr.Code != http.StatusNoContent {
		t.Errorf("got status %d, want %d", rr.Code, http.StatusNoContent)
	}

	// Verify deleted
	_, ok = registry.Get("test-group")
	if ok {
		t.Error("group should not exist after delete")
	}
}

func TestAPIHandler_Health(t *testing.T) {
	registry := NewRegistry()
	syncEngine := NewSyncEngine(registry, DefaultSyncConfig())
	handler := NewAPIHandler(registry, syncEngine)

	group, _ := NewGroup(&GroupConfig{
		ID:   "test-group",
		Name: "Test Group",
		Mirrors: []*Mirror{
			{ID: "mirror-1", ClusterID: "cluster-1", Enabled: true},
		},
		ReadStrategy: ReadStrategyRoundRobin,
		WritePolicy:  WritePolicyAll,
	})
	registry.Register(group)
	group.UpdateHealth("mirror-1", StateHealthy, 10*time.Millisecond, nil)

	req := httptest.NewRequest("GET", "/api/v1/mirrors/health", nil)
	rr := httptest.NewRecorder()
	handler.handleHealth(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("got status %d, want %d", rr.Code, http.StatusOK)
	}
}

func TestAPIHandler_SyncTrigger(t *testing.T) {
	registry := NewRegistry()
	syncEngine := NewSyncEngine(registry, DefaultSyncConfig())
	handler := NewAPIHandler(registry, syncEngine)

	group, _ := NewGroup(&GroupConfig{
		ID:   "test-group",
		Name: "Test Group",
		Mirrors: []*Mirror{
			{ID: "mirror-1", ClusterID: "cluster-1", Enabled: true, IsPrimary: true},
			{ID: "mirror-2", ClusterID: "cluster-2", Enabled: true},
		},
		ReadStrategy: ReadStrategyRoundRobin,
		WritePolicy:  WritePolicyAll,
	})
	registry.Register(group)

	// Trigger specific sync
	body := `{"group_id":"test-group","source_mirror":"mirror-1","target_mirror":"mirror-2"}`
	req := httptest.NewRequest("POST", "/api/v1/mirrors/sync", bytes.NewBufferString(body))
	rr := httptest.NewRecorder()
	handler.handleSyncTrigger(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("got status %d, want %d, body: %s", rr.Code, http.StatusOK, rr.Body.String())
	}

	var response SyncTriggerResponse
	if err := json.NewDecoder(rr.Body).Decode(&response); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if response.OperationID == "" {
		t.Error("expected operation ID in response")
	}
}

func TestAPIHandler_SyncTrigger_MissingGroupID(t *testing.T) {
	registry := NewRegistry()
	syncEngine := NewSyncEngine(registry, DefaultSyncConfig())
	handler := NewAPIHandler(registry, syncEngine)

	body := `{"source_mirror":"mirror-1","target_mirror":"mirror-2"}`
	req := httptest.NewRequest("POST", "/api/v1/mirrors/sync", bytes.NewBufferString(body))
	rr := httptest.NewRecorder()
	handler.handleSyncTrigger(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("got status %d, want %d", rr.Code, http.StatusBadRequest)
	}
}

func TestAPIHandler_SyncStatus(t *testing.T) {
	registry := NewRegistry()
	syncEngine := NewSyncEngine(registry, DefaultSyncConfig())
	handler := NewAPIHandler(registry, syncEngine)

	group, _ := NewGroup(&GroupConfig{
		ID:   "test-group",
		Name: "Test Group",
		Mirrors: []*Mirror{
			{ID: "mirror-1", ClusterID: "cluster-1", Enabled: true},
		},
		ReadStrategy: ReadStrategyRoundRobin,
		WritePolicy:  WritePolicyAll,
	})
	registry.Register(group)

	req := httptest.NewRequest("GET", "/api/v1/mirrors/sync/status?group=test-group", nil)
	rr := httptest.NewRecorder()
	handler.handleSyncStatus(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("got status %d, want %d", rr.Code, http.StatusOK)
	}

	var response SyncStatusResponse
	if err := json.NewDecoder(rr.Body).Decode(&response); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if response.GroupID != "test-group" {
		t.Errorf("got group ID %s, want test-group", response.GroupID)
	}
}

func TestAPIHandler_Conflicts(t *testing.T) {
	registry := NewRegistry()
	syncEngine := NewSyncEngine(registry, DefaultSyncConfig())
	handler := NewAPIHandler(registry, syncEngine)

	req := httptest.NewRequest("GET", "/api/v1/mirrors/conflicts", nil)
	rr := httptest.NewRecorder()
	handler.handleConflicts(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("got status %d, want %d", rr.Code, http.StatusOK)
	}

	var response []*ConflictResponse
	if err := json.NewDecoder(rr.Body).Decode(&response); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	// Should be empty initially
	if len(response) != 0 {
		t.Errorf("expected empty conflicts, got %d", len(response))
	}
}

func TestAPIHandler_MethodNotAllowed(t *testing.T) {
	registry := NewRegistry()
	syncEngine := NewSyncEngine(registry, DefaultSyncConfig())
	handler := NewAPIHandler(registry, syncEngine)

	tests := []struct {
		method string
		path   string
		handle func(http.ResponseWriter, *http.Request)
	}{
		{"POST", "/api/v1/mirrors", handler.handleMirrors},
		{"POST", "/api/v1/mirrors/health", handler.handleHealth},
		{"GET", "/api/v1/mirrors/sync", handler.handleSyncTrigger},
		{"POST", "/api/v1/mirrors/sync/status", handler.handleSyncStatus},
	}

	for _, tt := range tests {
		req := httptest.NewRequest(tt.method, tt.path, nil)
		rr := httptest.NewRecorder()
		tt.handle(rr, req)

		if rr.Code != http.StatusMethodNotAllowed {
			t.Errorf("%s %s: got status %d, want %d", tt.method, tt.path, rr.Code, http.StatusMethodNotAllowed)
		}
	}
}

func TestHealthToResponse(t *testing.T) {
	now := time.Now()
	h := &Health{
		State:            StateHealthy,
		LastCheck:        now,
		ConsecutiveFails: 0,
		AvgLatency:       50 * time.Millisecond,
	}

	response := healthToResponse(h)
	if response == nil {
		t.Fatal("healthToResponse returned nil")
	}

	if response.State != StateHealthy {
		t.Errorf("got state %s, want healthy", response.State)
	}
	if response.AvgLatencyMs != 50 {
		t.Errorf("got latency %d, want 50", response.AvgLatencyMs)
	}
}

func TestHealthToResponse_Nil(t *testing.T) {
	response := healthToResponse(nil)
	if response != nil {
		t.Error("expected nil response for nil input")
	}
}
