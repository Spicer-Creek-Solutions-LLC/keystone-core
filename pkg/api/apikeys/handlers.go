package apikeys

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"
)

// Handler provides HTTP handlers for API key management endpoints.
type Handler struct {
	store Store
}

// NewHandler creates a new API key handler.
func NewHandler(store Store) *Handler {
	return &Handler{store: store}
}

// RegisterRoutes registers the API key routes with the given mux.
func (h *Handler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/api/v1/api-keys", h.handleAPIKeys)
	mux.HandleFunc("/api/v1/api-keys/", h.handleAPIKey)
}

// CreateRequest represents the request body for creating an API key.
type CreateRequest struct {
	Name      string `json:"name"`
	Role      string `json:"role"`
	ExpiresIn string `json:"expires_in"`
}

// CreateResponse represents the response body after creating an API key.
type CreateResponse struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Key       string `json:"key"`
	Role      string `json:"role"`
	ExpiresAt string `json:"expires_at"`
}

// ListEntry represents a single API key in list responses.
type ListEntry struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Role      string `json:"role"`
	CreatedAt string `json:"created_at"`
	ExpiresAt string `json:"expires_at"`
	LastUsed  string `json:"last_used,omitempty"`
}

// handleAPIKeys handles GET/POST /api/v1/api-keys
func (h *Handler) handleAPIKeys(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		h.handleList(w, r)
	case http.MethodPost:
		h.handleCreate(w, r)
	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

// handleAPIKey handles DELETE /api/v1/api-keys/{id}
func (h *Handler) handleAPIKey(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimPrefix(r.URL.Path, "/api/v1/api-keys/")
	if id == "" {
		writeError(w, http.StatusBadRequest, "API key ID required")
		return
	}

	if r.Method != http.MethodDelete {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	h.handleRevoke(w, r, id)
}

func (h *Handler) handleCreate(w http.ResponseWriter, r *http.Request) {
	var req CreateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	if req.Name == "" {
		writeError(w, http.StatusBadRequest, "Name is required")
		return
	}

	role := req.Role
	if role == "" {
		role = "readonly"
	}
	if role != "admin" && role != "operator" && role != "readonly" {
		writeError(w, http.StatusBadRequest, "Invalid role. Must be admin, operator, or readonly")
		return
	}

	expiresIn := req.ExpiresIn
	if expiresIn == "" {
		expiresIn = "365d"
	}
	expiry, err := parseDuration(expiresIn)
	if err != nil {
		writeError(w, http.StatusBadRequest, "Invalid expires_in format")
		return
	}

	id := GenerateID()
	key := GenerateKey()
	now := time.Now().UTC()

	apiKey := &APIKey{
		ID:        id,
		Name:      req.Name,
		KeyHash:   key, // In production, store a hash
		Role:      role,
		CreatedAt: now,
		ExpiresAt: now.Add(expiry),
	}

	if err := h.store.Create(apiKey); err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to create API key")
		return
	}

	resp := CreateResponse{
		ID:        id,
		Name:      req.Name,
		Key:       key,
		Role:      role,
		ExpiresAt: apiKey.ExpiresAt.Format(time.RFC3339),
	}

	writeJSON(w, http.StatusCreated, resp)
}

func (h *Handler) handleList(w http.ResponseWriter, _ *http.Request) {
	keys, err := h.store.List()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to list API keys")
		return
	}

	entries := make([]ListEntry, 0, len(keys))
	for _, k := range keys {
		entry := ListEntry{
			ID:        k.ID,
			Name:      k.Name,
			Role:      k.Role,
			CreatedAt: k.CreatedAt.Format(time.RFC3339),
			ExpiresAt: k.ExpiresAt.Format(time.RFC3339),
		}
		if !k.LastUsed.IsZero() {
			entry.LastUsed = k.LastUsed.Format(time.RFC3339)
		}
		entries = append(entries, entry)
	}

	writeJSON(w, http.StatusOK, entries)
}

func (h *Handler) handleRevoke(w http.ResponseWriter, _ *http.Request, id string) {
	if err := h.store.Delete(id); err != nil {
		writeError(w, http.StatusNotFound, "API key not found")
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"id":         id,
		"revoked":    true,
		"revoked_at": time.Now().UTC().Format(time.RFC3339),
	})
}

// parseDuration parses duration strings like "30d", "1y", "365d".
func parseDuration(s string) (time.Duration, error) {
	if len(s) < 2 {
		return 0, &json.InvalidUnmarshalError{}
	}

	unit := s[len(s)-1]
	value := s[:len(s)-1]

	var num int
	for _, c := range value {
		if c < '0' || c > '9' {
			return 0, &json.InvalidUnmarshalError{}
		}
		num = num*10 + int(c-'0')
	}

	switch unit {
	case 'h':
		return time.Duration(num) * time.Hour, nil
	case 'd':
		return time.Duration(num) * 24 * time.Hour, nil
	case 'y':
		return time.Duration(num) * 365 * 24 * time.Hour, nil
	default:
		return 0, &json.InvalidUnmarshalError{}
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
