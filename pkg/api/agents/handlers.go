// Package agents provides HTTP handlers for agent REST API endpoints.
package agents

import (
	"encoding/json"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/shawnbutts/keystone-core/internal/controlplane"
)

// Handler provides HTTP handlers for agent API endpoints.
type Handler struct {
	connMgr *controlplane.ConnectionManager
}

// NewHandler creates a new agent API handler.
func NewHandler(connMgr *controlplane.ConnectionManager) *Handler {
	return &Handler{
		connMgr: connMgr,
	}
}

// RegisterRoutes registers the agent API routes with the given mux.
func (h *Handler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/api/v1/agents", h.handleAgents)
	mux.HandleFunc("/api/v1/agents/", h.handleAgent)
}

// AgentResponse represents an agent in API responses.
type AgentResponse struct {
	ID              string            `json:"id"`
	Hostname        string            `json:"hostname"`
	OS              string            `json:"os"`
	Arch            string            `json:"arch"`
	AgentVersion    string            `json:"agent_version,omitempty"`
	PlatformVersion string            `json:"platform_version,omitempty"`
	Status          string            `json:"status"`
	Labels          map[string]string `json:"labels,omitempty"`
	IPAddresses     []string          `json:"ip_addresses,omitempty"`
	IPv4Addresses   []string          `json:"ipv4_addresses,omitempty"`
	IPv6Addresses   []string          `json:"ipv6_addresses,omitempty"`
	IsDualStack     bool              `json:"is_dual_stack,omitempty"`
	RegisteredAt    time.Time         `json:"registered_at"`
	LastSeen        time.Time         `json:"last_seen"`
}

// AgentListResponse represents the list agents API response.
type AgentListResponse struct {
	Agents      []AgentResponse `json:"agents"`
	Total       int             `json:"total"`
	Online      int             `json:"online"`
	Offline     int             `json:"offline"`
	RetrievedAt time.Time       `json:"retrieved_at"`
}

// TagsUpdateRequest represents the request body for updating agent tags.
type TagsUpdateRequest struct {
	Tags map[string]string `json:"tags"`
}

// handleAgents handles GET /api/v1/agents
func (h *Handler) handleAgents(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Parse query parameters for filtering
	query := r.URL.Query()
	statusFilter := query.Get("status")
	labelFilter := query.Get("labels") // format: key=value,key2=value2
	sortBy := query.Get("sort")
	if sortBy == "" {
		sortBy = "hostname"
	}

	// Get all agents from connection manager
	agents := h.connMgr.ListAgents()

	// Parse label filters
	var labelFilters map[string]string
	if labelFilter != "" {
		labelFilters = parseLabelFilter(labelFilter)
	}

	// Build response with filtering
	var agentResponses []AgentResponse
	onlineCount := 0
	offlineCount := 0

	for _, agent := range agents {
		// Apply status filter
		status := agentStatusToString(agent.Status)
		if statusFilter != "" && status != statusFilter {
			continue
		}

		// Apply label filter
		if labelFilters != nil && !matchesLabels(agent.Metadata.GetLabels(), labelFilters) {
			continue
		}

		// Count online/offline
		if agent.Status == 1 { // ONLINE
			onlineCount++
		} else {
			offlineCount++
		}

		resp := AgentResponse{
			ID:           agent.ID,
			Status:       status,
			RegisteredAt: agent.RegisteredAt,
			LastSeen:     agent.LastHeartbeat,
		}

		// Fill in metadata if available
		if agent.Metadata != nil {
			resp.Hostname = agent.Metadata.Hostname
			resp.OS = agent.Metadata.Os
			resp.Arch = agent.Metadata.Arch
			resp.AgentVersion = agent.Metadata.AgentVersion
			resp.PlatformVersion = agent.Metadata.PlatformVersion
			resp.Labels = agent.Metadata.Labels
			resp.IPAddresses = agent.Metadata.IpAddresses
			resp.IPv4Addresses = agent.Metadata.Ipv4Addresses
			resp.IPv6Addresses = agent.Metadata.Ipv6Addresses
			resp.IsDualStack = agent.Metadata.IsDualStack
		}

		agentResponses = append(agentResponses, resp)
	}

	// Sort results
	sortAgents(agentResponses, sortBy)

	response := AgentListResponse{
		Agents:      agentResponses,
		Total:       len(agentResponses),
		Online:      onlineCount,
		Offline:     offlineCount,
		RetrievedAt: time.Now().UTC(),
	}

	writeJSON(w, http.StatusOK, response)
}

// handleAgent handles routes under /api/v1/agents/{id}
func (h *Handler) handleAgent(w http.ResponseWriter, r *http.Request) {
	// Extract agent ID from path
	path := strings.TrimPrefix(r.URL.Path, "/api/v1/agents/")
	parts := strings.Split(path, "/")
	if len(parts) == 0 || parts[0] == "" {
		http.Error(w, "Agent ID required", http.StatusBadRequest)
		return
	}

	agentID := parts[0]

	// Check sub-resource routes
	if len(parts) >= 2 && parts[1] == "tags" {
		h.handleAgentTags(w, r, agentID)
		return
	}
	if len(parts) >= 2 && parts[1] == "metrics" {
		h.handleAgentMetrics(w, r, agentID)
		return
	}

	// Handle GET /api/v1/agents/{id}
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	agent, err := h.connMgr.GetAgent(agentID)
	if err != nil {
		writeError(w, http.StatusNotFound, "Agent not found")
		return
	}

	resp := AgentResponse{
		ID:           agent.ID,
		Status:       agentStatusToString(agent.Status),
		RegisteredAt: agent.RegisteredAt,
		LastSeen:     agent.LastHeartbeat,
	}

	if agent.Metadata != nil {
		resp.Hostname = agent.Metadata.Hostname
		resp.OS = agent.Metadata.Os
		resp.Arch = agent.Metadata.Arch
		resp.AgentVersion = agent.Metadata.AgentVersion
		resp.PlatformVersion = agent.Metadata.PlatformVersion
		resp.Labels = agent.Metadata.Labels
		resp.IPAddresses = agent.Metadata.IpAddresses
		resp.IPv4Addresses = agent.Metadata.Ipv4Addresses
		resp.IPv6Addresses = agent.Metadata.Ipv6Addresses
		resp.IsDualStack = agent.Metadata.IsDualStack
	}

	writeJSON(w, http.StatusOK, resp)
}

// handleAgentTags handles PATCH /api/v1/agents/{id}/tags
func (h *Handler) handleAgentTags(w http.ResponseWriter, r *http.Request, agentID string) {
	if r.Method != http.MethodPatch {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Parse request body
	var req TagsUpdateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	// Get agent
	agent, err := h.connMgr.GetAgent(agentID)
	if err != nil {
		writeError(w, http.StatusNotFound, "Agent not found")
		return
	}

	// Update tags in metadata
	if agent.Metadata != nil {
		if agent.Metadata.Labels == nil {
			agent.Metadata.Labels = make(map[string]string)
		}
		for k, v := range req.Tags {
			if v == "" {
				delete(agent.Metadata.Labels, k)
			} else {
				agent.Metadata.Labels[k] = v
			}
		}
	}

	// Note: In a full implementation, we would persist this change and
	// potentially notify the agent. For now, we just return success.

	resp := map[string]interface{}{
		"agent_id": agentID,
		"tags":     agent.Metadata.GetLabels(),
		"updated":  true,
	}

	writeJSON(w, http.StatusOK, resp)
}

// AgentMetricsResponse represents the agent metrics API response.
type AgentMetricsResponse struct {
	AgentID           string    `json:"agent_id"`
	CPUPercent        float32   `json:"cpu_percent"`
	MemoryPercent     float32   `json:"memory_percent"`
	DiskPercent       float32   `json:"disk_percent"`
	LoadAverage       []float32 `json:"load_average"`
	ActiveConnections int32     `json:"active_connections"`
	CollectedAt       time.Time `json:"collected_at"`
}

// handleAgentMetrics handles GET /api/v1/agents/{id}/metrics
func (h *Handler) handleAgentMetrics(w http.ResponseWriter, r *http.Request, agentID string) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	agent, err := h.connMgr.GetAgent(agentID)
	if err != nil {
		writeError(w, http.StatusNotFound, "Agent not found")
		return
	}

	resp := AgentMetricsResponse{
		AgentID:     agentID,
		CollectedAt: agent.LastHeartbeat,
	}

	if agent.LastMetrics != nil {
		resp.CPUPercent = agent.LastMetrics.CpuPercent
		resp.MemoryPercent = agent.LastMetrics.MemoryPercent
		resp.DiskPercent = agent.LastMetrics.DiskPercent
		resp.LoadAverage = agent.LastMetrics.LoadAverage
		resp.ActiveConnections = agent.LastMetrics.ActiveConnections
	}

	writeJSON(w, http.StatusOK, resp)
}

// Helper functions

func agentStatusToString(status interface{}) string {
	switch s := status.(type) {
	case int32:
		switch s {
		case 0:
			return "unknown"
		case 1:
			return "online"
		case 2:
			return "offline"
		case 3:
			return "stale"
		default:
			return "unknown"
		}
	default:
		return "unknown"
	}
}

func parseLabelFilter(filter string) map[string]string {
	labels := make(map[string]string)
	pairs := strings.Split(filter, ",")
	for _, pair := range pairs {
		parts := strings.SplitN(pair, "=", 2)
		if len(parts) == 2 {
			labels[strings.TrimSpace(parts[0])] = strings.TrimSpace(parts[1])
		}
	}
	return labels
}

func matchesLabels(agentLabels, filterLabels map[string]string) bool {
	if agentLabels == nil {
		return len(filterLabels) == 0
	}
	for k, v := range filterLabels {
		if agentLabels[k] != v {
			return false
		}
	}
	return true
}

func sortAgents(agents []AgentResponse, sortBy string) {
	switch sortBy {
	case "hostname":
		sort.Slice(agents, func(i, j int) bool {
			return agents[i].Hostname < agents[j].Hostname
		})
	case "status":
		sort.Slice(agents, func(i, j int) bool {
			return agents[i].Status < agents[j].Status
		})
	case "last_seen":
		sort.Slice(agents, func(i, j int) bool {
			return agents[i].LastSeen.After(agents[j].LastSeen)
		})
	case "registered_at":
		sort.Slice(agents, func(i, j int) bool {
			return agents[i].RegisteredAt.Before(agents[j].RegisteredAt)
		})
	}
}

func writeJSON(w http.ResponseWriter, status int, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]string{"error": message})
}
