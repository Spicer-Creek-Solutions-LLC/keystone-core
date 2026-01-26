// Package mirror provides mirror group API handlers.
package mirror

import (
	"encoding/json"
	"net/http"
	"strconv"
	"time"
)

// APIHandler provides HTTP handlers for mirror management.
type APIHandler struct {
	registry   *Registry
	syncEngine *SyncEngine
}

// NewAPIHandler creates a new mirror API handler.
func NewAPIHandler(registry *Registry, syncEngine *SyncEngine) *APIHandler {
	return &APIHandler{
		registry:   registry,
		syncEngine: syncEngine,
	}
}

// RegisterRoutes registers mirror API routes with an HTTP mux.
func (h *APIHandler) RegisterRoutes(mux *http.ServeMux) {
	// Mirror groups
	mux.HandleFunc("/api/v1/mirrors", h.handleMirrors)
	mux.HandleFunc("/api/v1/mirrors/", h.handleMirrorByID)

	// Health
	mux.HandleFunc("/api/v1/mirrors/health", h.handleHealth)

	// Sync operations
	mux.HandleFunc("/api/v1/mirrors/sync", h.handleSyncTrigger)
	mux.HandleFunc("/api/v1/mirrors/sync/status", h.handleSyncStatus)
	mux.HandleFunc("/api/v1/mirrors/sync/operations", h.handleSyncOperations)
	mux.HandleFunc("/api/v1/mirrors/sync/history", h.handleSyncHistory)

	// Conflicts
	mux.HandleFunc("/api/v1/mirrors/conflicts", h.handleConflicts)
	mux.HandleFunc("/api/v1/mirrors/conflicts/", h.handleConflictResolve)
}

// MirrorGroupResponse is the API response for a mirror group.
type MirrorGroupResponse struct {
	ID           string                   `json:"id"`
	Name         string                   `json:"name"`
	Description  string                   `json:"description,omitempty"`
	ReadStrategy ReadStrategy             `json:"read_strategy"`
	WritePolicy  WritePolicy              `json:"write_policy"`
	QuorumSize   int                      `json:"quorum_size,omitempty"`
	Mirrors      []*MirrorResponse        `json:"mirrors"`
	PathPrefixes []string                 `json:"path_prefixes,omitempty"`
	Namespaces   []string                 `json:"namespaces,omitempty"`
}

// MirrorResponse is the API response for a mirror.
type MirrorResponse struct {
	ID        string          `json:"id"`
	Name      string          `json:"name,omitempty"`
	ClusterID string          `json:"cluster_id"`
	Weight    int             `json:"weight"`
	Priority  int             `json:"priority"`
	Enabled   bool            `json:"enabled"`
	ReadOnly  bool            `json:"read_only"`
	IsPrimary bool            `json:"is_primary"`
	Location  *Location       `json:"location,omitempty"`
	Health    *HealthResponse `json:"health,omitempty"`
}

// HealthResponse is the API response for mirror health.
type HealthResponse struct {
	State            MirrorState `json:"state"`
	LastCheck        *time.Time  `json:"last_check,omitempty"`
	LastError        string      `json:"last_error,omitempty"`
	ConsecutiveFails int         `json:"consecutive_fails"`
	AvgLatencyMs     int64       `json:"avg_latency_ms,omitempty"`
}

// SyncStatusResponse is the API response for sync status.
type SyncStatusResponse struct {
	GroupID          string        `json:"group_id"`
	State            SyncState     `json:"state"`
	LastSyncAt       *time.Time    `json:"last_sync_at,omitempty"`
	LastSyncDuration time.Duration `json:"last_sync_duration_ms,omitempty"`
	LastSyncStatus   SyncStatus    `json:"last_sync_status,omitempty"`
	NextSyncAt       *time.Time    `json:"next_sync_at,omitempty"`
	PendingOps       int           `json:"pending_ops"`
	ActiveOps        int           `json:"active_ops"`
	ConflictCount    int           `json:"conflict_count"`
}

// SyncTriggerRequest is the request to trigger a sync.
type SyncTriggerRequest struct {
	GroupID      string `json:"group_id"`
	SourceMirror string `json:"source_mirror,omitempty"`
	TargetMirror string `json:"target_mirror,omitempty"`
	Priority     int    `json:"priority,omitempty"`
	Path         string `json:"path,omitempty"`
}

// SyncTriggerResponse is the response after triggering sync.
type SyncTriggerResponse struct {
	OperationID string `json:"operation_id"`
	Message     string `json:"message"`
}

// ConflictResponse is the API response for a conflict.
type ConflictResponse struct {
	ID           string    `json:"id"`
	GroupID      string    `json:"group_id"`
	Path         string    `json:"path"`
	SourceMirror string    `json:"source_mirror"`
	TargetMirror string    `json:"target_mirror"`
	SourceInfo   FileInfo  `json:"source_info"`
	TargetInfo   FileInfo  `json:"target_info"`
	DetectedAt   time.Time `json:"detected_at"`
}

// ConflictResolveRequest is the request to resolve a conflict.
type ConflictResolveRequest struct {
	Resolution string `json:"resolution"` // source, target, manual
	ResolvedBy string `json:"resolved_by,omitempty"`
}

// handleMirrors handles GET /api/v1/mirrors
func (h *APIHandler) handleMirrors(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	groups := h.registry.List()

	response := make([]*MirrorGroupResponse, 0, len(groups))
	for _, g := range groups {
		response = append(response, h.mirrorGroupToResponse(g))
	}

	writeJSON(w, response)
}

// handleMirrorByID handles /api/v1/mirrors/{id}
func (h *APIHandler) handleMirrorByID(w http.ResponseWriter, r *http.Request) {
	// Extract ID from path
	id := r.URL.Path[len("/api/v1/mirrors/"):]
	if id == "" {
		http.Error(w, "Mirror group ID required", http.StatusBadRequest)
		return
	}

	switch r.Method {
	case http.MethodGet:
		group, ok := h.registry.Get(id)
		if !ok {
			http.Error(w, "Mirror group not found", http.StatusNotFound)
			return
		}
		writeJSON(w, h.mirrorGroupToResponse(group))

	case http.MethodDelete:
		if err := h.registry.Unregister(id); err != nil {
			http.Error(w, err.Error(), http.StatusNotFound)
			return
		}
		w.WriteHeader(http.StatusNoContent)

	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

// handleHealth handles GET /api/v1/mirrors/health
func (h *APIHandler) handleHealth(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	groupID := r.URL.Query().Get("group")

	type healthEntry struct {
		GroupID  string          `json:"group_id"`
		MirrorID string          `json:"mirror_id"`
		Health   *HealthResponse `json:"health"`
	}

	var entries []healthEntry

	allHealth := h.registry.GetAllHealth()
	for gID, groupHealth := range allHealth {
		if groupID != "" && gID != groupID {
			continue
		}
		for mID, health := range groupHealth {
			entry := healthEntry{
				GroupID:  gID,
				MirrorID: mID,
				Health:   healthToResponse(health),
			}
			entries = append(entries, entry)
		}
	}

	writeJSON(w, entries)
}

// handleSyncTrigger handles POST /api/v1/mirrors/sync
func (h *APIHandler) handleSyncTrigger(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if h.syncEngine == nil {
		http.Error(w, "Sync engine not available", http.StatusServiceUnavailable)
		return
	}

	var req SyncTriggerRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	if req.GroupID == "" {
		http.Error(w, "group_id is required", http.StatusBadRequest)
		return
	}

	// Check if specific source/target sync or group sync
	if req.SourceMirror != "" && req.TargetMirror != "" {
		op, err := h.syncEngine.TriggerSync(req.GroupID, req.SourceMirror, req.TargetMirror, req.Priority)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		writeJSON(w, SyncTriggerResponse{
			OperationID: op.ID,
			Message:     "Sync operation triggered",
		})
	} else {
		err := h.syncEngine.ScheduleSync(req.GroupID, req.Path, req.Priority)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		writeJSON(w, SyncTriggerResponse{
			Message: "Sync scheduled for group",
		})
	}
}

// handleSyncStatus handles GET /api/v1/mirrors/sync/status
func (h *APIHandler) handleSyncStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if h.syncEngine == nil {
		http.Error(w, "Sync engine not available", http.StatusServiceUnavailable)
		return
	}

	groupID := r.URL.Query().Get("group")
	if groupID == "" {
		http.Error(w, "group query parameter required", http.StatusBadRequest)
		return
	}

	status := h.syncEngine.GetGroupStatus(groupID)

	response := SyncStatusResponse{
		GroupID:       groupID,
		State:         status.State,
		PendingOps:    status.PendingOps,
		ActiveOps:     status.ActiveOps,
		ConflictCount: status.ConflictCount,
	}

	if !status.LastSyncAt.IsZero() {
		response.LastSyncAt = &status.LastSyncAt
		response.LastSyncDuration = status.LastSyncDuration
		response.LastSyncStatus = status.LastSyncStatus
	}
	if !status.NextSyncAt.IsZero() {
		response.NextSyncAt = &status.NextSyncAt
	}

	writeJSON(w, response)
}

// handleSyncOperations handles GET /api/v1/mirrors/sync/operations
func (h *APIHandler) handleSyncOperations(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if h.syncEngine == nil {
		http.Error(w, "Sync engine not available", http.StatusServiceUnavailable)
		return
	}

	groupID := r.URL.Query().Get("group")
	activeOnly := r.URL.Query().Get("active") == "true"

	var ops []*SyncOperation

	if activeOnly {
		ops = h.syncEngine.GetActiveOperations()
	} else {
		ops = append(h.syncEngine.GetActiveOperations(), h.syncEngine.GetPendingOperations()...)
	}

	// Filter by group if specified
	if groupID != "" {
		var filtered []*SyncOperation
		for _, op := range ops {
			if op.GroupID == groupID {
				filtered = append(filtered, op)
			}
		}
		ops = filtered
	}

	writeJSON(w, ops)
}

// handleSyncHistory handles GET /api/v1/mirrors/sync/history
func (h *APIHandler) handleSyncHistory(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if h.syncEngine == nil {
		http.Error(w, "Sync engine not available", http.StatusServiceUnavailable)
		return
	}

	limitStr := r.URL.Query().Get("limit")
	limit := 20
	if limitStr != "" {
		if l, err := strconv.Atoi(limitStr); err == nil && l > 0 {
			limit = l
		}
	}

	history := h.syncEngine.GetHistory(limit)

	groupID := r.URL.Query().Get("group")
	if groupID != "" {
		var filtered []*SyncHistory
		for _, h := range history {
			if h.GroupID == groupID {
				filtered = append(filtered, h)
			}
		}
		history = filtered
	}

	writeJSON(w, history)
}

// handleConflicts handles GET /api/v1/mirrors/conflicts
func (h *APIHandler) handleConflicts(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if h.syncEngine == nil {
		http.Error(w, "Sync engine not available", http.StatusServiceUnavailable)
		return
	}

	conflicts := h.syncEngine.GetConflicts()

	groupID := r.URL.Query().Get("group")

	var response []*ConflictResponse
	for _, c := range conflicts {
		if groupID != "" && c.GroupID != groupID {
			continue
		}
		response = append(response, &ConflictResponse{
			ID:           c.ID,
			GroupID:      c.GroupID,
			Path:         c.Path,
			SourceMirror: c.SourceMirror,
			TargetMirror: c.TargetMirror,
			SourceInfo:   c.SourceInfo,
			TargetInfo:   c.TargetInfo,
			DetectedAt:   c.DetectedAt,
		})
	}

	writeJSON(w, response)
}

// handleConflictResolve handles POST /api/v1/mirrors/conflicts/{id}
func (h *APIHandler) handleConflictResolve(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if h.syncEngine == nil {
		http.Error(w, "Sync engine not available", http.StatusServiceUnavailable)
		return
	}

	// Extract conflict ID from path
	conflictID := r.URL.Path[len("/api/v1/mirrors/conflicts/"):]
	if conflictID == "" {
		http.Error(w, "Conflict ID required", http.StatusBadRequest)
		return
	}

	var req ConflictResolveRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	if req.Resolution == "" {
		http.Error(w, "resolution is required", http.StatusBadRequest)
		return
	}

	resolvedBy := req.ResolvedBy
	if resolvedBy == "" {
		resolvedBy = "api"
	}

	if err := h.syncEngine.ResolveConflict(conflictID, req.Resolution, resolvedBy); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// mirrorGroupToResponse converts a MirrorGroup to API response.
func (h *APIHandler) mirrorGroupToResponse(g *MirrorGroup) *MirrorGroupResponse {
	config := g.Config()

	response := &MirrorGroupResponse{
		ID:           config.ID,
		Name:         config.Name,
		Description:  config.Description,
		ReadStrategy: config.ReadStrategy,
		WritePolicy:  config.WritePolicy,
		QuorumSize:   config.QuorumSize,
		PathPrefixes: config.PathPrefixes,
		Namespaces:   config.Namespaces,
		Mirrors:      make([]*MirrorResponse, 0, len(config.Mirrors)),
	}

	for _, m := range g.GetMirrors() {
		mr := &MirrorResponse{
			ID:        m.ID,
			Name:      m.Name,
			ClusterID: m.ClusterID,
			Weight:    m.Weight,
			Priority:  m.Priority,
			Enabled:   m.Enabled,
			ReadOnly:  m.ReadOnly,
			IsPrimary: m.IsPrimary,
			Location:  m.Location,
		}

		if health, ok := g.GetHealth(m.ID); ok {
			mr.Health = healthToResponse(health)
		}

		response.Mirrors = append(response.Mirrors, mr)
	}

	return response
}

// healthToResponse converts MirrorHealth to API response.
func healthToResponse(h *MirrorHealth) *HealthResponse {
	if h == nil {
		return nil
	}

	response := &HealthResponse{
		State:            h.State,
		ConsecutiveFails: h.ConsecutiveFails,
		LastError:        h.LastError,
	}

	if !h.LastCheck.IsZero() {
		response.LastCheck = &h.LastCheck
	}
	if h.AvgLatency > 0 {
		response.AvgLatencyMs = h.AvgLatency.Milliseconds()
	}

	return response
}

// writeJSON writes a JSON response.
func writeJSON(w http.ResponseWriter, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(data); err != nil {
		http.Error(w, "Failed to encode response", http.StatusInternalServerError)
	}
}
