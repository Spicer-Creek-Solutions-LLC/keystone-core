package visualization

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/shawnbutts/keystone-core/pkg/testing/helpers"
)

// MockAgentProvider is a mock implementation of AgentProvider
type MockAgentProvider struct {
	agents []*Agent
}

func (m *MockAgentProvider) GetAgents() []*Agent {
	return m.agents
}

func (m *MockAgentProvider) GetAgent(id string) (*Agent, error) {
	for _, agent := range m.agents {
		if agent.ID == id {
			return agent, nil
		}
	}
	return nil, fmt.Errorf("agent not found: %s", id)
}

func TestNewServer(t *testing.T) {
	provider := &MockAgentProvider{}
	config := DefaultConfig()

	server := NewServer(config, provider)
	if server == nil {
		t.Fatal("NewServer returned nil")
	}

	if server.provider != provider {
		t.Error("Provider not set correctly")
	}
}

func TestServerStartStop(t *testing.T) {
	provider := &MockAgentProvider{}
	config := &VisualizationConfig{
		ListenAddr:      "localhost:18080",
		EnableWebSocket: false,
	}

	server := NewServer(config, provider)
	ctx := context.Background()

	if err := server.Start(ctx); err != nil {
		t.Fatalf("Failed to start server: %v", err)
	}

	// Test that server is running
	var resp *http.Response
	if err := helpers.WaitForTimeout(2*time.Second, 50*time.Millisecond, func() (bool, error) {
		var reqErr error
		resp, reqErr = http.Get("http://localhost:18080/api/agents")
		if reqErr != nil {
			return false, nil
		}
		return true, nil
	}); err != nil {
		t.Fatalf("Failed to access server: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("Expected status 200, got %d", resp.StatusCode)
	}

	stopCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := server.Stop(stopCtx); err != nil {
		t.Fatalf("Failed to stop server: %v", err)
	}
}

func TestHandleAgents(t *testing.T) {
	now := time.Now()

	provider := &MockAgentProvider{
		agents: []*Agent{
			{
				ID:          "agent-1",
				Hostname:    "web-01",
				Status:      AgentStatusHealthy,
				LastSeen:    now,
				Datacenter:  "us-east-1",
				Environment: "production",
				Role:        "web",
			},
			{
				ID:          "agent-2",
				Hostname:    "db-01",
				Status:      AgentStatusHealthy,
				LastSeen:    now,
				Datacenter:  "us-east-1",
				Environment: "production",
				Role:        "database",
			},
		},
	}

	server := NewServer(nil, provider)

	req := httptest.NewRequest(http.MethodGet, "/api/agents", nil)
	w := httptest.NewRecorder()

	server.handleAgents(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", w.Code)
	}

	var response map[string]interface{}
	if err := json.NewDecoder(w.Body).Decode(&response); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	agents := response["agents"].([]interface{})
	if len(agents) != 2 {
		t.Errorf("Expected 2 agents, got %d", len(agents))
	}
}

func TestHandleAgentsWithFilter(t *testing.T) {
	now := time.Now()

	provider := &MockAgentProvider{
		agents: []*Agent{
			{
				ID:          "agent-1",
				Hostname:    "web-01",
				Status:      AgentStatusHealthy,
				LastSeen:    now,
				Datacenter:  "us-east-1",
				Environment: "production",
				Role:        "web",
			},
			{
				ID:          "agent-2",
				Hostname:    "db-01",
				Status:      AgentStatusHealthy,
				LastSeen:    now,
				Datacenter:  "us-west-2",
				Environment: "production",
				Role:        "database",
			},
		},
	}

	server := NewServer(nil, provider)

	req := httptest.NewRequest(http.MethodGet, "/api/agents?datacenter=us-east-1", nil)
	w := httptest.NewRecorder()

	server.handleAgents(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", w.Code)
	}

	var response map[string]interface{}
	if err := json.NewDecoder(w.Body).Decode(&response); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	agents := response["agents"].([]interface{})
	if len(agents) != 1 {
		t.Errorf("Expected 1 agent (filtered), got %d", len(agents))
	}
}

func TestHandleAgent(t *testing.T) {
	now := time.Now()

	provider := &MockAgentProvider{
		agents: []*Agent{
			{
				ID:       "agent-123",
				Hostname: "web-01",
				Status:   AgentStatusHealthy,
				LastSeen: now,
			},
		},
	}

	server := NewServer(nil, provider)

	req := httptest.NewRequest(http.MethodGet, "/api/agents/agent-123", nil)
	w := httptest.NewRecorder()

	server.handleAgent(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", w.Code)
	}

	var agent Agent
	if err := json.NewDecoder(w.Body).Decode(&agent); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	if agent.ID != "agent-123" {
		t.Errorf("Expected agent ID agent-123, got %s", agent.ID)
	}
}

func TestHandleAgentNotFound(t *testing.T) {
	provider := &MockAgentProvider{
		agents: []*Agent{},
	}

	server := NewServer(nil, provider)

	req := httptest.NewRequest(http.MethodGet, "/api/agents/nonexistent", nil)
	w := httptest.NewRecorder()

	server.handleAgent(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("Expected status 404, got %d", w.Code)
	}
}

func TestHandleTopology(t *testing.T) {
	now := time.Now()

	provider := &MockAgentProvider{
		agents: []*Agent{
			{
				ID:          "agent-1",
				Hostname:    "web-01",
				Status:      AgentStatusHealthy,
				LastSeen:    now,
				Datacenter:  "us-east-1",
				Environment: "production",
				Role:        "web",
			},
			{
				ID:          "agent-2",
				Hostname:    "web-02",
				Status:      AgentStatusHealthy,
				LastSeen:    now,
				Datacenter:  "us-east-1",
				Environment: "production",
				Role:        "web",
			},
		},
	}

	server := NewServer(nil, provider)

	req := httptest.NewRequest(http.MethodGet, "/api/topology", nil)
	w := httptest.NewRecorder()

	server.handleTopology(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", w.Code)
	}

	var topology TopologyNode
	if err := json.NewDecoder(w.Body).Decode(&topology); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	if topology.Type != "root" {
		t.Errorf("Expected root node type, got %s", topology.Type)
	}

	if topology.Stats.Total != 2 {
		t.Errorf("Expected 2 total agents, got %d", topology.Stats.Total)
	}
}

func TestHandleGraph(t *testing.T) {
	now := time.Now()

	provider := &MockAgentProvider{
		agents: []*Agent{
			{
				ID:         "agent-1",
				Hostname:   "web-01",
				Status:     AgentStatusHealthy,
				LastSeen:   now,
				Datacenter: "us-east-1",
			},
			{
				ID:         "agent-2",
				Hostname:   "web-02",
				Status:     AgentStatusHealthy,
				LastSeen:   now,
				Datacenter: "us-east-1",
			},
		},
	}

	server := NewServer(nil, provider)

	req := httptest.NewRequest(http.MethodGet, "/api/graph", nil)
	w := httptest.NewRecorder()

	server.handleGraph(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", w.Code)
	}

	var graph Graph
	if err := json.NewDecoder(w.Body).Decode(&graph); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	if len(graph.Nodes) != 2 {
		t.Errorf("Expected 2 nodes, got %d", len(graph.Nodes))
	}

	if len(graph.Edges) != 1 {
		t.Errorf("Expected 1 edge (same datacenter), got %d", len(graph.Edges))
	}
}

func TestBuildTopology(t *testing.T) {
	now := time.Now()

	agents := []*Agent{
		{
			ID:          "agent-1",
			Hostname:    "web-01",
			Status:      AgentStatusHealthy,
			LastSeen:    now,
			Datacenter:  "us-east-1",
			Environment: "production",
			Role:        "web",
		},
		{
			ID:          "agent-2",
			Hostname:    "db-01",
			Status:      AgentStatusDegraded,
			LastSeen:    now,
			Datacenter:  "us-east-1",
			Environment: "production",
			Role:        "database",
		},
	}

	topology := buildTopology(agents)

	if topology.Type != "root" {
		t.Errorf("Expected root type, got %s", topology.Type)
	}

	if topology.Stats.Total != 2 {
		t.Errorf("Expected 2 total agents, got %d", topology.Stats.Total)
	}

	if topology.Stats.Healthy != 1 {
		t.Errorf("Expected 1 healthy agent, got %d", topology.Stats.Healthy)
	}

	if topology.Stats.Degraded != 1 {
		t.Errorf("Expected 1 degraded agent, got %d", topology.Stats.Degraded)
	}

	// Root should have status degraded (because one agent is degraded)
	if topology.Status != AgentStatusDegraded {
		t.Errorf("Expected root status degraded, got %s", topology.Status)
	}
}

func TestBuildGraph(t *testing.T) {
	now := time.Now()

	agents := []*Agent{
		{
			ID:         "agent-1",
			Hostname:   "web-01",
			Status:     AgentStatusHealthy,
			LastSeen:   now,
			Datacenter: "us-east-1",
		},
		{
			ID:         "agent-2",
			Hostname:   "web-02",
			Status:     AgentStatusHealthy,
			LastSeen:   now,
			Datacenter: "us-east-1",
		},
		{
			ID:         "agent-3",
			Hostname:   "web-03",
			Status:     AgentStatusHealthy,
			LastSeen:   now,
			Datacenter: "us-west-2",
		},
	}

	graph := buildGraph(agents)

	if len(graph.Nodes) != 3 {
		t.Errorf("Expected 3 nodes, got %d", len(graph.Nodes))
	}

	// Should have 1 edge between agent-1 and agent-2 (same datacenter)
	if len(graph.Edges) != 1 {
		t.Errorf("Expected 1 edge, got %d", len(graph.Edges))
	}
}

func TestMatchesFilter(t *testing.T) {
	agent := &Agent{
		ID:          "agent-1",
		Hostname:    "web-01",
		Status:      AgentStatusHealthy,
		Datacenter:  "us-east-1",
		Environment: "production",
		Role:        "web",
		Tags:        []string{"frontend", "nginx"},
	}

	// Test datacenter filter
	if !matchesFilter(agent, &FilterOptions{Datacenter: "us-east-1"}) {
		t.Error("Expected to match datacenter filter")
	}

	if matchesFilter(agent, &FilterOptions{Datacenter: "us-west-2"}) {
		t.Error("Should not match different datacenter")
	}

	// Test environment filter
	if !matchesFilter(agent, &FilterOptions{Environment: "production"}) {
		t.Error("Expected to match environment filter")
	}

	// Test role filter
	if !matchesFilter(agent, &FilterOptions{Role: "web"}) {
		t.Error("Expected to match role filter")
	}

	// Test status filter
	if !matchesFilter(agent, &FilterOptions{Status: AgentStatusHealthy}) {
		t.Error("Expected to match status filter")
	}

	// Test tags filter
	if !matchesFilter(agent, &FilterOptions{Tags: []string{"frontend"}}) {
		t.Error("Expected to match tags filter")
	}

	if matchesFilter(agent, &FilterOptions{Tags: []string{"backend"}}) {
		t.Error("Should not match different tags")
	}
}
