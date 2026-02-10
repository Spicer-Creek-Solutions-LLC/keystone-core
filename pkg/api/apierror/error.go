// Package apierror provides standard error response formatting for REST API handlers.
package apierror

import (
	"encoding/json"
	"net/http"
)

// Response is the standard error response format for REST API endpoints.
type Response struct {
	Error   string            `json:"error"`
	Message string            `json:"message"`
	Details map[string]string `json:"details,omitempty"`
}

// Write writes a standard error response with an auto-mapped error code.
func Write(w http.ResponseWriter, status int, message string) {
	resp := Response{
		Error:   StatusCode(status),
		Message: message,
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(resp)
}

// StatusCode maps an HTTP status code to a standard API error code string.
func StatusCode(status int) string {
	switch status {
	case http.StatusBadRequest:
		return "invalid_request"
	case http.StatusUnauthorized:
		return "unauthorized"
	case http.StatusForbidden:
		return "forbidden"
	case http.StatusNotFound:
		return "not_found"
	case http.StatusMethodNotAllowed:
		return "method_not_allowed"
	case http.StatusConflict:
		return "conflict"
	case http.StatusTooManyRequests:
		return "rate_limit_exceeded"
	case http.StatusServiceUnavailable:
		return "service_unavailable"
	default:
		return "internal_error"
	}
}
