package agents

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestAgentResponseStructure(t *testing.T) {
	resp := AgentResponse{
		ID:              "agent-123",
		Hostname:        "server1.example.com",
		OS:              "linux",
		Arch:            "amd64",
		AgentVersion:    "1.0.0",
		PlatformVersion: "ubuntu-22.04",
		Status:          "online",
		Labels: map[string]string{
			"env":  "prod",
			"role": "web",
		},
		IPAddresses:   []string{"192.168.1.100", "10.0.0.50"},
		IPv4Addresses: []string{"192.168.1.100", "10.0.0.50"},
		IPv6Addresses: []string{"::1"},
		IsDualStack:   true,
		RegisteredAt:  time.Now().Add(-24 * time.Hour),
		LastSeen:      time.Now().Add(-5 * time.Minute),
	}

	if resp.ID != "agent-123" {
		t.Errorf("ID = %v", resp.ID)
	}
	if resp.Hostname != "server1.example.com" {
		t.Errorf("Hostname = %v", resp.Hostname)
	}
	if resp.OS != "linux" {
		t.Errorf("OS = %v", resp.OS)
	}
	if resp.Status != "online" {
		t.Errorf("Status = %v", resp.Status)
	}
	if !resp.IsDualStack {
		t.Error("IsDualStack should be true")
	}
	if len(resp.Labels) != 2 {
		t.Errorf("Labels count = %d", len(resp.Labels))
	}
}

func TestAgentListResponseStructure(t *testing.T) {
	resp := AgentListResponse{
		Agents: []AgentResponse{
			{ID: "agent-1", Hostname: "server1"},
			{ID: "agent-2", Hostname: "server2"},
		},
		Total:       2,
		Online:      2,
		Offline:     0,
		RetrievedAt: time.Now(),
	}

	if resp.Total != 2 {
		t.Errorf("Total = %d", resp.Total)
	}
	if resp.Online != 2 {
		t.Errorf("Online = %d", resp.Online)
	}
	if len(resp.Agents) != 2 {
		t.Errorf("Agents count = %d", len(resp.Agents))
	}
}

func TestTagsUpdateRequestStructure(t *testing.T) {
	req := TagsUpdateRequest{
		Tags: map[string]string{
			"env":    "staging",
			"remove": "", // empty value to remove
			"newkey": "newvalue",
		},
	}

	if req.Tags["env"] != "staging" {
		t.Errorf("Tags[env] = %v", req.Tags["env"])
	}
	if req.Tags["remove"] != "" {
		t.Errorf("Tags[remove] = %v", req.Tags["remove"])
	}
}

func TestAgentStatusToString(t *testing.T) {
	tests := []struct {
		name     string
		status   interface{}
		expected string
	}{
		{"unknown", int32(0), "unknown"},
		{"online", int32(1), "online"},
		{"offline", int32(2), "offline"},
		{"stale", int32(3), "stale"},
		{"invalid", int32(99), "unknown"},
		{"non-int32", "invalid", "unknown"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := agentStatusToString(tt.status)
			if result != tt.expected {
				t.Errorf("agentStatusToString(%v) = %v, want %v", tt.status, result, tt.expected)
			}
		})
	}
}

func TestParseLabelFilter(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected map[string]string
	}{
		{
			name:  "single label",
			input: "env=prod",
			expected: map[string]string{
				"env": "prod",
			},
		},
		{
			name:  "multiple labels",
			input: "env=prod,role=web,region=us-east",
			expected: map[string]string{
				"env":    "prod",
				"role":   "web",
				"region": "us-east",
			},
		},
		{
			name:  "with spaces",
			input: " env = prod , role = web ",
			expected: map[string]string{
				"env":  "prod",
				"role": "web",
			},
		},
		{
			name:     "empty",
			input:    "",
			expected: map[string]string{},
		},
		{
			name:     "malformed",
			input:    "invalid",
			expected: map[string]string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := parseLabelFilter(tt.input)
			if len(result) != len(tt.expected) {
				t.Errorf("parseLabelFilter(%q) = %v, want %v", tt.input, result, tt.expected)
				return
			}
			for k, v := range tt.expected {
				if result[k] != v {
					t.Errorf("parseLabelFilter(%q)[%q] = %q, want %q", tt.input, k, result[k], v)
				}
			}
		})
	}
}

func TestMatchesLabels(t *testing.T) {
	tests := []struct {
		name         string
		agentLabels  map[string]string
		filterLabels map[string]string
		expected     bool
	}{
		{
			name:         "match all",
			agentLabels:  map[string]string{"env": "prod", "role": "web"},
			filterLabels: map[string]string{"env": "prod"},
			expected:     true,
		},
		{
			name:         "match multiple",
			agentLabels:  map[string]string{"env": "prod", "role": "web", "region": "us"},
			filterLabels: map[string]string{"env": "prod", "role": "web"},
			expected:     true,
		},
		{
			name:         "no match",
			agentLabels:  map[string]string{"env": "staging"},
			filterLabels: map[string]string{"env": "prod"},
			expected:     false,
		},
		{
			name:         "missing label",
			agentLabels:  map[string]string{"env": "prod"},
			filterLabels: map[string]string{"role": "web"},
			expected:     false,
		},
		{
			name:         "nil agent labels with filter",
			agentLabels:  nil,
			filterLabels: map[string]string{"env": "prod"},
			expected:     false,
		},
		{
			name:         "nil agent labels empty filter",
			agentLabels:  nil,
			filterLabels: map[string]string{},
			expected:     true,
		},
		{
			name:         "empty filter",
			agentLabels:  map[string]string{"env": "prod"},
			filterLabels: map[string]string{},
			expected:     true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := matchesLabels(tt.agentLabels, tt.filterLabels)
			if result != tt.expected {
				t.Errorf("matchesLabels(%v, %v) = %v, want %v",
					tt.agentLabels, tt.filterLabels, result, tt.expected)
			}
		})
	}
}

func TestSortAgents(t *testing.T) {
	agents := []AgentResponse{
		{Hostname: "server-c", Status: "online", LastSeen: time.Now().Add(-3 * time.Hour), RegisteredAt: time.Now().Add(-48 * time.Hour)},
		{Hostname: "server-a", Status: "offline", LastSeen: time.Now().Add(-1 * time.Hour), RegisteredAt: time.Now().Add(-24 * time.Hour)},
		{Hostname: "server-b", Status: "online", LastSeen: time.Now().Add(-2 * time.Hour), RegisteredAt: time.Now().Add(-72 * time.Hour)},
	}

	t.Run("sort by hostname", func(t *testing.T) {
		sorted := make([]AgentResponse, len(agents))
		copy(sorted, agents)
		sortAgents(sorted, "hostname")
		if sorted[0].Hostname != "server-a" {
			t.Errorf("first hostname = %v, want server-a", sorted[0].Hostname)
		}
		if sorted[2].Hostname != "server-c" {
			t.Errorf("last hostname = %v, want server-c", sorted[2].Hostname)
		}
	})

	t.Run("sort by status", func(t *testing.T) {
		sorted := make([]AgentResponse, len(agents))
		copy(sorted, agents)
		sortAgents(sorted, "status")
		if sorted[0].Status != "offline" {
			t.Errorf("first status = %v, want offline", sorted[0].Status)
		}
	})

	t.Run("sort by last_seen", func(t *testing.T) {
		sorted := make([]AgentResponse, len(agents))
		copy(sorted, agents)
		sortAgents(sorted, "last_seen")
		// Most recent should be first
		if sorted[0].Hostname != "server-a" {
			t.Errorf("first by last_seen = %v, want server-a", sorted[0].Hostname)
		}
	})

	t.Run("sort by registered_at", func(t *testing.T) {
		sorted := make([]AgentResponse, len(agents))
		copy(sorted, agents)
		sortAgents(sorted, "registered_at")
		// Oldest registration first
		if sorted[0].Hostname != "server-b" {
			t.Errorf("first by registered_at = %v, want server-b", sorted[0].Hostname)
		}
	})
}

func TestWriteJSON(t *testing.T) {
	w := httptest.NewRecorder()
	data := map[string]string{"status": "ok"}

	writeJSON(w, http.StatusOK, data)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", w.Code, http.StatusOK)
	}
	if w.Header().Get("Content-Type") != "application/json" {
		t.Errorf("Content-Type = %v", w.Header().Get("Content-Type"))
	}

	var result map[string]string
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
		t.Fatalf("failed to unmarshal response: %v", err)
	}
	if result["status"] != "ok" {
		t.Errorf("result[status] = %v", result["status"])
	}
}

func TestWriteError(t *testing.T) {
	w := httptest.NewRecorder()

	writeError(w, http.StatusBadRequest, "test error message")

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}

	var result map[string]string
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
		t.Fatalf("failed to unmarshal response: %v", err)
	}
	if result["error"] != "test error message" {
		t.Errorf("result[error] = %v", result["error"])
	}
}

func TestNewHandler(t *testing.T) {
	// Test with nil connection manager
	handler := NewHandler(nil)
	if handler == nil {
		t.Fatal("handler should not be nil")
	}
}

func TestRegisterRoutes(t *testing.T) {
	handler := NewHandler(nil)
	mux := http.NewServeMux()

	handler.RegisterRoutes(mux)

	// Test that routes are registered by making a request
	// The handler will fail due to nil connMgr, but routes are registered
	t.Run("agents endpoint", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/api/v1/agents", nil)
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)

		// POST should return method not allowed
		if w.Code != http.StatusMethodNotAllowed {
			t.Errorf("POST /api/v1/agents status = %d, want %d", w.Code, http.StatusMethodNotAllowed)
		}
	})
}

func TestAgentResponseJSONMarshal(t *testing.T) {
	now := time.Now().UTC()
	resp := AgentResponse{
		ID:           "test-agent",
		Hostname:     "test-host",
		Status:       "online",
		RegisteredAt: now,
		LastSeen:     now,
	}

	data, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("failed to marshal: %v", err)
	}

	var unmarshaled AgentResponse
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
