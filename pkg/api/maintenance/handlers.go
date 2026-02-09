package maintenance

import (
	"encoding/json"
	"net/http"
	"time"
)

// Handler provides HTTP handlers for maintenance mode API endpoints.
type Handler struct {
	store Store
}

// NewHandler creates a new maintenance API handler.
func NewHandler(store Store) *Handler {
	return &Handler{store: store}
}

// RegisterRoutes registers the maintenance API routes with the given mux.
func (h *Handler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/api/v1/maintenance/enable", h.handleEnable)
	mux.HandleFunc("/api/v1/maintenance/disable", h.handleDisable)
	mux.HandleFunc("/api/v1/maintenance/status", h.handleStatus)
	mux.HandleFunc("/api/v1/maintenance/queue", h.handleQueue)
	mux.HandleFunc("/api/v1/maintenance/cleanup", h.handleCleanup)
}

// EnableRequest represents the request body for enabling maintenance mode.
type EnableRequest struct {
	Reason   string `json:"reason"`
	Duration string `json:"duration"`
	Force    bool   `json:"force"`
}

// StatusResponse represents the maintenance status API response.
type StatusResponse struct {
	Enabled   bool      `json:"enabled"`
	Reason    string    `json:"reason,omitempty"`
	Since     time.Time `json:"since,omitempty"`
	Duration  string    `json:"duration,omitempty"`
	EnabledBy string    `json:"enabled_by,omitempty"`
}

// QueueResponse represents the maintenance queue API response.
type QueueResponse struct {
	QueuedCommands int           `json:"queued_commands"`
	Commands       []interface{} `json:"commands"`
}

// handleEnable handles POST /api/v1/maintenance/enable
func (h *Handler) handleEnable(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req EnableRequest
	if r.Body != nil && r.ContentLength > 0 {
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, "Invalid request body")
			return
		}
	}

	current, err := h.store.GetState(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to get maintenance state")
		return
	}

	if current.Enabled && !req.Force {
		writeError(w, http.StatusConflict, "Maintenance mode already enabled. Use force=true to override.")
		return
	}

	state := &State{
		Enabled:   true,
		Reason:    req.Reason,
		EnabledAt: time.Now().UTC(),
		Duration:  req.Duration,
	}

	if err := h.store.SetState(r.Context(), state); err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to enable maintenance mode")
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"enabled": true,
		"reason":  state.Reason,
		"since":   state.EnabledAt,
	})
}

// handleDisable handles POST /api/v1/maintenance/disable
func (h *Handler) handleDisable(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	current, err := h.store.GetState(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to get maintenance state")
		return
	}

	if !current.Enabled {
		writeError(w, http.StatusConflict, "Maintenance mode is not enabled")
		return
	}

	if err := h.store.ClearState(r.Context()); err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to disable maintenance mode")
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"enabled":     false,
		"disabled_at": time.Now().UTC(),
	})
}

// handleStatus handles GET /api/v1/maintenance/status
func (h *Handler) handleStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	state, err := h.store.GetState(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to get maintenance state")
		return
	}

	resp := StatusResponse{
		Enabled:   state.Enabled,
		Reason:    state.Reason,
		Since:     state.EnabledAt,
		Duration:  state.Duration,
		EnabledBy: state.EnabledBy,
	}

	writeJSON(w, http.StatusOK, resp)
}

// handleQueue handles GET /api/v1/maintenance/queue
func (h *Handler) handleQueue(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	resp := QueueResponse{
		QueuedCommands: 0,
		Commands:       []interface{}{},
	}

	writeJSON(w, http.StatusOK, resp)
}

// handleCleanup handles POST /api/v1/maintenance/cleanup
func (h *Handler) handleCleanup(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	olderThan := r.URL.Query().Get("older_than")
	dryRun := r.URL.Query().Get("dry_run") == "true"

	resp := map[string]interface{}{
		"dry_run":   dryRun,
		"older_than": olderThan,
		"deleted":   0,
	}

	writeJSON(w, http.StatusOK, resp)
}

func writeJSON(w http.ResponseWriter, status int, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]string{"error": message})
}
