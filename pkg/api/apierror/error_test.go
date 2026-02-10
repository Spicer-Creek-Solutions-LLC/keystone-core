package apierror

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestWrite(t *testing.T) {
	tests := []struct {
		name         string
		status       int
		message      string
		wantCode     string
		wantMessage  string
	}{
		{
			name:        "bad request",
			status:      http.StatusBadRequest,
			message:     "name is required",
			wantCode:    "invalid_request",
			wantMessage: "name is required",
		},
		{
			name:        "not found",
			status:      http.StatusNotFound,
			message:     "Agent not found",
			wantCode:    "not_found",
			wantMessage: "Agent not found",
		},
		{
			name:        "conflict",
			status:      http.StatusConflict,
			message:     "Resource already exists",
			wantCode:    "conflict",
			wantMessage: "Resource already exists",
		},
		{
			name:        "internal error",
			status:      http.StatusInternalServerError,
			message:     "Something went wrong",
			wantCode:    "internal_error",
			wantMessage: "Something went wrong",
		},
		{
			name:        "unauthorized",
			status:      http.StatusUnauthorized,
			message:     "Invalid API key",
			wantCode:    "unauthorized",
			wantMessage: "Invalid API key",
		},
		{
			name:        "rate limited",
			status:      http.StatusTooManyRequests,
			message:     "Too many requests",
			wantCode:    "rate_limit_exceeded",
			wantMessage: "Too many requests",
		},
		{
			name:        "service unavailable",
			status:      http.StatusServiceUnavailable,
			message:     "Service down",
			wantCode:    "service_unavailable",
			wantMessage: "Service down",
		},
		{
			name:        "method not allowed",
			status:      http.StatusMethodNotAllowed,
			message:     "POST not supported",
			wantCode:    "method_not_allowed",
			wantMessage: "POST not supported",
		},
		{
			name:        "forbidden",
			status:      http.StatusForbidden,
			message:     "Access denied",
			wantCode:    "forbidden",
			wantMessage: "Access denied",
		},
		{
			name:        "unknown status defaults to internal_error",
			status:      http.StatusTeapot,
			message:     "I'm a teapot",
			wantCode:    "internal_error",
			wantMessage: "I'm a teapot",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			Write(rec, tt.status, tt.message)

			if rec.Code != tt.status {
				t.Errorf("status = %d, want %d", rec.Code, tt.status)
			}
			if ct := rec.Header().Get("Content-Type"); ct != "application/json" {
				t.Errorf("Content-Type = %q, want application/json", ct)
			}

			var resp Response
			if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
				t.Fatalf("failed to decode response: %v", err)
			}
			if resp.Error != tt.wantCode {
				t.Errorf("error code = %q, want %q", resp.Error, tt.wantCode)
			}
			if resp.Message != tt.wantMessage {
				t.Errorf("message = %q, want %q", resp.Message, tt.wantMessage)
			}
			if resp.Details != nil {
				t.Errorf("details should be nil, got %v", resp.Details)
			}
		})
	}
}

func TestStatusCode(t *testing.T) {
	tests := []struct {
		status int
		want   string
	}{
		{http.StatusBadRequest, "invalid_request"},
		{http.StatusUnauthorized, "unauthorized"},
		{http.StatusForbidden, "forbidden"},
		{http.StatusNotFound, "not_found"},
		{http.StatusMethodNotAllowed, "method_not_allowed"},
		{http.StatusConflict, "conflict"},
		{http.StatusTooManyRequests, "rate_limit_exceeded"},
		{http.StatusInternalServerError, "internal_error"},
		{http.StatusServiceUnavailable, "service_unavailable"},
		{999, "internal_error"},
	}

	for _, tt := range tests {
		got := StatusCode(tt.status)
		if got != tt.want {
			t.Errorf("StatusCode(%d) = %q, want %q", tt.status, got, tt.want)
		}
	}
}
