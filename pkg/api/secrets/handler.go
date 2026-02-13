package secrets

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/shawnbutts/keystone-core/internal/secrets"
	"github.com/shawnbutts/keystone-core/pkg/api/apierror"
)

// Handler provides HTTP handlers for secrets API endpoints.
type Handler struct {
	broker       *secrets.SecretBroker
	leaseManager secrets.LeaseManager
	orchestrator *secrets.RotationOrchestrator
	transit      secrets.TransitBackend
	auditLogger  *secrets.InMemorySecretAuditLogger
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

// RegisterRoutes registers the secrets API routes with the given mux.
func (h *Handler) RegisterRoutes(mux *http.ServeMux) {
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
