package secrets

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/shawnbutts/keystone-core/internal/secrets"
	"github.com/shawnbutts/keystone-core/pkg/api/apierror"
)

// Handler provides HTTP handlers for secrets API endpoints.
type Handler struct {
	broker               *secrets.SecretBroker
	leaseManager         secrets.LeaseManager
	orchestrator         *secrets.RotationOrchestrator
	transit              secrets.TransitBackend
	auditLogger          *secrets.InMemorySecretAuditLogger
	rotationPolicyEngine RotationPolicyManager
}

// NewHandler creates a new secrets API handler.
// Any dependency may be nil — handlers return 503 if the required dep is nil.
func NewHandler(
	broker *secrets.SecretBroker,
	leaseManager secrets.LeaseManager,
	orchestrator *secrets.RotationOrchestrator,
	transit secrets.TransitBackend,
	auditLogger *secrets.InMemorySecretAuditLogger,
) *Handler {
	return &Handler{
		broker:       broker,
		leaseManager: leaseManager,
		orchestrator: orchestrator,
		transit:      transit,
		auditLogger:  auditLogger,
	}
}

// SetRotationPolicyEngine sets the rotation policy engine dependency.
func (h *Handler) SetRotationPolicyEngine(engine RotationPolicyManager) {
	h.rotationPolicyEngine = engine
}

// RegisterRoutes registers the secrets API routes with the given mux.
func (h *Handler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/api/v1/secrets/rotation/policies/", h.handleRotationPolicyRoute)
	mux.HandleFunc("/api/v1/secrets/rotation/policies", h.handleRotationPoliciesList)
	mux.HandleFunc("/api/v1/secrets/backends/", h.handleBackendRoute)
	mux.HandleFunc("/api/v1/secrets/backends", h.handleBackendsList)
	mux.HandleFunc("/api/v1/secrets/cache/stats", h.handleCacheStats)
	mux.HandleFunc("/api/v1/secrets/cache", h.handleCacheRoute)
	mux.HandleFunc("/api/v1/secrets/", h.handleSecrets)
	mux.HandleFunc("/api/v1/leases", h.handleLeasesList)
	mux.HandleFunc("/api/v1/leases/", h.handleLeasesRoute)
	mux.HandleFunc("/api/v1/rotations", h.handleRotationsList)
	mux.HandleFunc("/api/v1/rotations/", h.handleRotationsRoute)
	mux.HandleFunc("/api/v1/transit/", h.handleTransit)
	mux.HandleFunc("/api/v1/compliance/reports", h.handleComplianceReports)
	mux.HandleFunc("/api/v1/audit/logs", h.handleAuditLogs)
	mux.HandleFunc("/api/v1/health/secrets", h.handleSecretsHealth)
}

// handleBackendsList handles GET /api/v1/secrets/backends.
func (h *Handler) handleBackendsList(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	if h.broker == nil {
		writeError(w, http.StatusServiceUnavailable, "secrets broker not available")
		return
	}

	ctx := r.Context()
	names := h.broker.ListBackends()
	health := h.broker.BackendHealth(ctx)

	backends := make([]*BackendInfoResponse, 0, len(names))
	for _, name := range names {
		backend, err := h.broker.GetBackend(name)
		backendType := ""
		if err == nil {
			backendType = backend.Type().String()
		}
		backends = append(backends, &BackendInfoResponse{
			Name:    name,
			Type:    backendType,
			Healthy: health[name],
		})
	}

	writeJSON(w, http.StatusOK, BackendListResponse{
		Backends: backends,
		Total:    len(backends),
	})
}

// handleBackendRoute handles GET /api/v1/secrets/backends/{name}.
func (h *Handler) handleBackendRoute(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	if h.broker == nil {
		writeError(w, http.StatusServiceUnavailable, "secrets broker not available")
		return
	}

	name := strings.TrimPrefix(r.URL.Path, "/api/v1/secrets/backends/")
	if name == "" {
		h.handleBackendsList(w, r)
		return
	}

	ctx := r.Context()
	backend, err := h.broker.GetBackend(name)
	if err != nil {
		writeSecretError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, BackendInfoResponse{
		Name:    name,
		Type:    backend.Type().String(),
		Healthy: backend.Healthy(ctx),
	})
}

// handleCacheStats handles GET /api/v1/secrets/cache/stats.
func (h *Handler) handleCacheStats(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	if h.broker == nil {
		writeError(w, http.StatusServiceUnavailable, "secrets broker not available")
		return
	}

	stats := h.broker.Stats(r.Context())
	if stats.CacheStats == nil {
		writeJSON(w, http.StatusOK, CacheStatsResponse{})
		return
	}

	writeJSON(w, http.StatusOK, CacheStatsResponse{
		Entries:      stats.CacheStats.Entries,
		MaxEntries:   stats.CacheStats.MaxEntries,
		Hits:         stats.CacheStats.Hits,
		Misses:       stats.CacheStats.Misses,
		Evictions:    stats.CacheStats.Evictions,
		ExpiredCount: stats.CacheStats.ExpiredCount,
		MemoryBytes:  stats.CacheStats.MemoryBytes,
	})
}

// handleCacheRoute handles DELETE /api/v1/secrets/cache (clear cache).
func (h *Handler) handleCacheRoute(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodDelete {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	if h.broker == nil {
		writeError(w, http.StatusServiceUnavailable, "secrets broker not available")
		return
	}

	stats := h.broker.Stats(r.Context())
	count := 0
	if stats.CacheStats != nil {
		count = stats.CacheStats.Entries
	}

	_, err := h.broker.InvalidatePrefix(r.Context(), "")
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to clear cache: "+err.Error())
		return
	}

	writeJSON(w, http.StatusOK, CacheClearResponse{
		Message: "cache cleared",
		Cleared: count,
	})
}

func writeJSON(w http.ResponseWriter, status int, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, message string) {
	apierror.Write(w, status, message)
}

// secretErrorToHTTPStatus maps a secret error to an HTTP status code.
func secretErrorToHTTPStatus(err error) int {
	var secretErr *secrets.SecretError
	if errors.As(err, &secretErr) {
		switch secretErr.Code {
		case secrets.ErrorCodeNotFound, secrets.ErrorCodeVersionNotFound:
			return http.StatusNotFound
		case secrets.ErrorCodeAccessDenied, secrets.ErrorCodeAuthorization:
			return http.StatusForbidden
		case secrets.ErrorCodeAuthentication:
			return http.StatusUnauthorized
		case secrets.ErrorCodeInvalidRequest, secrets.ErrorCodeConfiguration:
			return http.StatusBadRequest
		case secrets.ErrorCodeRateLimit:
			return http.StatusTooManyRequests
		case secrets.ErrorCodeBackendUnavailable:
			return http.StatusServiceUnavailable
		case secrets.ErrorCodeTimeout:
			return http.StatusGatewayTimeout
		default:
			return http.StatusInternalServerError
		}
	}

	switch {
	case errors.Is(err, secrets.ErrSecretNotFound), errors.Is(err, secrets.ErrLeaseNotFound):
		return http.StatusNotFound
	case errors.Is(err, secrets.ErrAccessDenied):
		return http.StatusForbidden
	case errors.Is(err, secrets.ErrInvalidPath):
		return http.StatusBadRequest
	case errors.Is(err, secrets.ErrLeaseExpired):
		return http.StatusGone
	case errors.Is(err, secrets.ErrLeaseNotRenewable):
		return http.StatusConflict
	case errors.Is(err, secrets.ErrBackendUnavailable):
		return http.StatusServiceUnavailable
	case errors.Is(err, secrets.ErrBackendNotFound):
		return http.StatusNotFound
	default:
		return http.StatusInternalServerError
	}
}

func writeSecretError(w http.ResponseWriter, err error) {
	status := secretErrorToHTTPStatus(err)
	writeError(w, status, err.Error())
}
