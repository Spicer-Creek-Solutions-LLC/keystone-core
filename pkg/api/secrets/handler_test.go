package secrets

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/shawnbutts/keystone-core/internal/secrets"
)

func TestNewHandler(t *testing.T) {
	h := NewHandler(nil, nil, nil, nil, nil)
	if h == nil {
		t.Fatal("expected non-nil handler")
	}
	if h.broker != nil {
		t.Error("expected nil broker")
	}
	if h.leaseManager != nil {
		t.Error("expected nil leaseManager")
	}
	if h.orchestrator != nil {
		t.Error("expected nil orchestrator")
	}
	if h.transit != nil {
		t.Error("expected nil transit")
	}
	if h.auditLogger != nil {
		t.Error("expected nil auditLogger")
	}
}

func TestNewHandlerWithDeps(t *testing.T) {
	broker := secrets.NewSecretBroker(nil)
	audit := secrets.NewInMemorySecretAuditLogger(nil)
	h := NewHandler(broker, nil, nil, nil, audit)
	if h.broker == nil {
		t.Error("expected non-nil broker")
	}
	if h.auditLogger == nil {
		t.Error("expected non-nil auditLogger")
	}
}

func TestRegisterRoutes(t *testing.T) {
	mux := http.NewServeMux()
	h := NewHandler(nil, nil, nil, nil, nil)
	h.RegisterRoutes(mux)

	routes := []string{
		"/api/v1/secrets/test",
		"/api/v1/leases",
		"/api/v1/leases/abc",
		"/api/v1/rotations",
		"/api/v1/rotations/abc",
		"/api/v1/transit/encrypt/mykey",
		"/api/v1/compliance/reports",
		"/api/v1/audit/logs",
		"/api/v1/health/secrets",
	}

	for _, route := range routes {
		req := httptest.NewRequest(http.MethodGet, route, nil)
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)
		if w.Code == http.StatusNotFound {
			t.Errorf("route %s returned 404, expected it to be registered", route)
		}
	}
}

func TestSecretErrorToHTTPStatus(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		expected int
	}{
		{
			name:     "not found code",
			err:      secrets.NewSecretError(secrets.ErrorCodeNotFound, "not found", nil),
			expected: http.StatusNotFound,
		},
		{
			name:     "version not found code",
			err:      secrets.NewSecretError(secrets.ErrorCodeVersionNotFound, "version", nil),
			expected: http.StatusNotFound,
		},
		{
			name:     "access denied code",
			err:      secrets.NewSecretError(secrets.ErrorCodeAccessDenied, "denied", nil),
			expected: http.StatusForbidden,
		},
		{
			name:     "authorization code",
			err:      secrets.NewSecretError(secrets.ErrorCodeAuthorization, "unauth", nil),
			expected: http.StatusForbidden,
		},
		{
			name:     "authentication code",
			err:      secrets.NewSecretError(secrets.ErrorCodeAuthentication, "auth", nil),
			expected: http.StatusUnauthorized,
		},
		{
			name:     "invalid request code",
			err:      secrets.NewSecretError(secrets.ErrorCodeInvalidRequest, "bad", nil),
			expected: http.StatusBadRequest,
		},
		{
			name:     "configuration code",
			err:      secrets.NewSecretError(secrets.ErrorCodeConfiguration, "config", nil),
			expected: http.StatusBadRequest,
		},
		{
			name:     "rate limit code",
			err:      secrets.NewSecretError(secrets.ErrorCodeRateLimit, "slow down", nil),
			expected: http.StatusTooManyRequests,
		},
		{
			name:     "backend unavailable code",
			err:      secrets.NewSecretError(secrets.ErrorCodeBackendUnavailable, "down", nil),
			expected: http.StatusServiceUnavailable,
		},
		{
			name:     "timeout code",
			err:      secrets.NewSecretError(secrets.ErrorCodeTimeout, "timeout", nil),
			expected: http.StatusGatewayTimeout,
		},
		{
			name:     "internal code",
			err:      secrets.NewSecretError(secrets.ErrorCodeInternal, "internal", nil),
			expected: http.StatusInternalServerError,
		},
		{
			name:     "sentinel ErrSecretNotFound",
			err:      secrets.ErrSecretNotFound,
			expected: http.StatusNotFound,
		},
		{
			name:     "sentinel ErrLeaseNotFound",
			err:      secrets.ErrLeaseNotFound,
			expected: http.StatusNotFound,
		},
		{
			name:     "sentinel ErrAccessDenied",
			err:      secrets.ErrAccessDenied,
			expected: http.StatusForbidden,
		},
		{
			name:     "sentinel ErrInvalidPath",
			err:      secrets.ErrInvalidPath,
			expected: http.StatusBadRequest,
		},
		{
			name:     "sentinel ErrLeaseExpired",
			err:      secrets.ErrLeaseExpired,
			expected: http.StatusGone,
		},
		{
			name:     "sentinel ErrLeaseNotRenewable",
			err:      secrets.ErrLeaseNotRenewable,
			expected: http.StatusConflict,
		},
		{
			name:     "sentinel ErrBackendUnavailable",
			err:      secrets.ErrBackendUnavailable,
			expected: http.StatusServiceUnavailable,
		},
		{
			name:     "sentinel ErrBackendNotFound",
			err:      secrets.ErrBackendNotFound,
			expected: http.StatusNotFound,
		},
		{
			name:     "unknown error",
			err:      errors.New("something else"),
			expected: http.StatusInternalServerError,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := secretErrorToHTTPStatus(tt.err)
			if got != tt.expected {
				t.Errorf("secretErrorToHTTPStatus() = %d, want %d", got, tt.expected)
			}
		})
	}
}

func TestWriteJSON(t *testing.T) {
	w := httptest.NewRecorder()
	writeJSON(w, http.StatusOK, map[string]string{"key": "value"})

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}
	ct := w.Header().Get("Content-Type")
	if ct != "application/json" {
		t.Errorf("expected Content-Type application/json, got %s", ct)
	}
}

func TestWriteError(t *testing.T) {
	w := httptest.NewRecorder()
	writeError(w, http.StatusBadRequest, "bad request")

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected status 400, got %d", w.Code)
	}
}
