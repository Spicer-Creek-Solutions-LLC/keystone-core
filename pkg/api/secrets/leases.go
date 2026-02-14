package secrets

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/shawnbutts/keystone-core/internal/secrets"
)

// handleLeasesList handles GET /api/v1/leases and POST /api/v1/leases/bulk-revoke.
func (h *Handler) handleLeasesList(w http.ResponseWriter, r *http.Request) {
	// Exact match on /api/v1/leases only
	if r.URL.Path != "/api/v1/leases" {
		h.handleLeasesRoute(w, r)
		return
	}

	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	if h.leaseManager == nil {
		writeError(w, http.StatusServiceUnavailable, "lease manager not available")
		return
	}

	query := r.URL.Query()
	pathFilter := query.Get("path")
	backendFilter := query.Get("backend")

	var leases []*secrets.Lease
	var err error

	switch {
	case pathFilter != "":
		leases, err = h.leaseManager.ListByPath(r.Context(), pathFilter)
	case backendFilter != "":
		bt, parseErr := secrets.ParseBackendType(backendFilter)
		if parseErr != nil {
			writeError(w, http.StatusBadRequest, "invalid backend filter: "+parseErr.Error())
			return
		}
		leases, err = h.leaseManager.ListByBackend(r.Context(), bt)
	default:
		leases, err = h.leaseManager.List(r.Context())
	}

	if err != nil {
		writeSecretError(w, err)
		return
	}

	if leases == nil {
		leases = []*secrets.Lease{}
	}

	// Apply limit/offset
	limit := 0
	offset := 0
	if v := query.Get("limit"); v != "" {
		var parseErr error
		limit, parseErr = strconv.Atoi(v)
		if parseErr != nil {
			writeError(w, http.StatusBadRequest, "invalid limit parameter")
			return
		}
	}
	if v := query.Get("offset"); v != "" {
		var parseErr error
		offset, parseErr = strconv.Atoi(v)
		if parseErr != nil {
			writeError(w, http.StatusBadRequest, "invalid offset parameter")
			return
		}
	}

	total := len(leases)
	if offset > 0 && offset < len(leases) {
		leases = leases[offset:]
	} else if offset >= len(leases) {
		leases = []*secrets.Lease{}
	}
	if limit > 0 && limit < len(leases) {
		leases = leases[:limit]
	}

	resp := LeaseListResponse{
		Leases: make([]LeaseResponse, 0, len(leases)),
		Total:  total,
	}
	for _, l := range leases {
		resp.Leases = append(resp.Leases, leaseToResponse(l))
	}

	writeJSON(w, http.StatusOK, resp)
}

// handleLeasesRoute handles routes under /api/v1/leases/{id}...
func (h *Handler) handleLeasesRoute(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/api/v1/leases/")
	if path == "" {
		writeError(w, http.StatusBadRequest, "lease ID is required")
		return
	}

	parts := strings.SplitN(path, "/", 2)
	leaseID := parts[0]

	// Check for sub-routes
	if leaseID == "bulk-revoke" {
		h.handleBulkRevoke(w, r)
		return
	}

	if len(parts) == 2 {
		switch parts[1] {
		case "renew":
			h.handleLeaseRenew(w, r, leaseID)
			return
		case "revoke":
			h.handleLeaseRevoke(w, r, leaseID)
			return
		}
	}

	// GET /api/v1/leases/{id}
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	if h.leaseManager == nil {
		writeError(w, http.StatusServiceUnavailable, "lease manager not available")
		return
	}

	lease, err := h.leaseManager.Get(r.Context(), leaseID)
	if err != nil {
		writeSecretError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, leaseToResponse(lease))
}

func (h *Handler) handleLeaseRenew(w http.ResponseWriter, r *http.Request, leaseID string) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	if h.leaseManager == nil {
		writeError(w, http.StatusServiceUnavailable, "lease manager not available")
		return
	}

	var req LeaseRenewRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	increment := time.Duration(req.IncrementSeconds) * time.Second
	if increment <= 0 {
		increment = 30 * time.Minute // default increment
	}

	lease, err := h.leaseManager.Renew(r.Context(), leaseID, increment)
	if err != nil {
		writeSecretError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, leaseToResponse(lease))
}

func (h *Handler) handleLeaseRevoke(w http.ResponseWriter, r *http.Request, leaseID string) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	if h.leaseManager == nil {
		writeError(w, http.StatusServiceUnavailable, "lease manager not available")
		return
	}

	if err := h.leaseManager.Revoke(r.Context(), leaseID); err != nil {
		writeSecretError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"lease_id": leaseID,
		"revoked":  true,
	})
}

func (h *Handler) handleBulkRevoke(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	if h.leaseManager == nil {
		writeError(w, http.StatusServiceUnavailable, "lease manager not available")
		return
	}

	var req BulkRevokeRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.Path == "" {
		writeError(w, http.StatusBadRequest, "path filter is required for bulk revoke")
		return
	}

	count, err := h.leaseManager.RevokeByPath(r.Context(), req.Path)
	if err != nil {
		writeSecretError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, BulkRevokeResponse{
		Revoked: count,
		Path:    req.Path,
	})
}

func leaseToResponse(l *secrets.Lease) LeaseResponse {
	return LeaseResponse{
		ID:            l.ID,
		SecretPath:    l.SecretPath,
		Backend:       string(l.Backend),
		State:         string(l.State),
		TTLSeconds:    int64(l.TTL.Seconds()),
		MaxTTLSeconds: int64(l.MaxTTL.Seconds()),
		IssuedAt:      l.IssuedAt,
		ExpiresAt:     l.ExpiresAt,
		LastRenewal:   l.LastRenewal,
		RenewalCount:  l.RenewalCount,
		Renewable:     l.Renewable,
		Revocable:     l.Revocable,
		Metadata:      l.Metadata,
	}
}
