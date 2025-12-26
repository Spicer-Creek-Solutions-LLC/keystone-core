package health

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestLivenessHandler(t *testing.T) {
	m := NewManager(nil)
	h := NewHandler(m)

	req := httptest.NewRequest(http.MethodGet, "/health/live", nil)
	w := httptest.NewRecorder()

	h.LivenessHandler()(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status %d, got %d", http.StatusOK, w.Code)
	}

	var response LivenessResponse
	if err := json.NewDecoder(w.Body).Decode(&response); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	if response.Status != StatusHealthy {
		t.Errorf("Expected status %s, got %s", StatusHealthy, response.Status)
	}
}

func TestLivenessHandlerMethodNotAllowed(t *testing.T) {
	m := NewManager(nil)
	h := NewHandler(m)

	req := httptest.NewRequest(http.MethodPost, "/health/live", nil)
	w := httptest.NewRecorder()

	h.LivenessHandler()(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("Expected status %d, got %d", http.StatusMethodNotAllowed, w.Code)
	}
}

func TestReadinessHandlerHealthy(t *testing.T) {
	config := &Config{
		CheckInterval:      1 * time.Second,
		CheckTimeout:       1 * time.Second,
		ReadinessChecks:    []string{"test"},
		StartupGracePeriod: 0,
	}

	m := NewManager(config)
	checker := NewAlwaysHealthyChecker("test")
	m.RegisterChecker(checker)

	// Run checks to populate results
	ctx := context.Background()
	m.runAllChecks(ctx)
	m.SetReady(true)

	h := NewHandler(m)

	req := httptest.NewRequest(http.MethodGet, "/health/ready", nil)
	w := httptest.NewRecorder()

	h.ReadinessHandler()(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status %d, got %d", http.StatusOK, w.Code)
	}

	var response ReadinessResponse
	if err := json.NewDecoder(w.Body).Decode(&response); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	if response.Status != StatusHealthy {
		t.Errorf("Expected status %s, got %s", StatusHealthy, response.Status)
	}
}

func TestReadinessHandlerUnhealthy(t *testing.T) {
	config := &Config{
		CheckInterval:      1 * time.Second,
		CheckTimeout:       1 * time.Second,
		ReadinessChecks:    []string{"test"},
		StartupGracePeriod: 0,
	}

	m := NewManager(config)
	m.SetReady(false)

	h := NewHandler(m)

	req := httptest.NewRequest(http.MethodGet, "/health/ready", nil)
	w := httptest.NewRecorder()

	h.ReadinessHandler()(w, req)

	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("Expected status %d, got %d", http.StatusServiceUnavailable, w.Code)
	}

	var response ReadinessResponse
	if err := json.NewDecoder(w.Body).Decode(&response); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	if response.Status != StatusUnhealthy {
		t.Errorf("Expected status %s, got %s", StatusUnhealthy, response.Status)
	}
}

func TestStatusHandler(t *testing.T) {
	m := NewManager(nil)
	checker := NewAlwaysHealthyChecker("test")
	m.RegisterChecker(checker)

	h := NewHandler(m)

	req := httptest.NewRequest(http.MethodGet, "/health/status", nil)
	w := httptest.NewRecorder()

	h.StatusHandler()(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status %d, got %d", http.StatusOK, w.Code)
	}

	var response StatusResponse
	if err := json.NewDecoder(w.Body).Decode(&response); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	if response.Status != StatusHealthy {
		t.Errorf("Expected status %s, got %s", StatusHealthy, response.Status)
	}

	if response.Version == "" {
		t.Error("Expected version to be set")
	}

	if response.Uptime == "" {
		t.Error("Expected uptime to be set")
	}
}

func TestRegisterRoutes(t *testing.T) {
	config := &Config{
		CheckInterval:      1 * time.Second,
		CheckTimeout:       1 * time.Second,
		ReadinessChecks:    []string{},
		StartupGracePeriod: 0,
	}
	m := NewManager(config)
	m.SetReady(true)
	h := NewHandler(m)

	mux := http.NewServeMux()
	h.RegisterRoutes(mux)

	// Test that routes are registered
	tests := []struct {
		path   string
		method string
		status int
	}{
		{"/health/live", http.MethodGet, http.StatusOK},
		{"/health/ready", http.MethodGet, http.StatusOK},
		{"/health/status", http.MethodGet, http.StatusOK},
	}

	for _, tt := range tests {
		req := httptest.NewRequest(tt.method, tt.path, nil)
		w := httptest.NewRecorder()

		mux.ServeHTTP(w, req)

		if w.Code != tt.status {
			t.Errorf("Expected status %d for %s, got %d", tt.status, tt.path, w.Code)
		}
	}
}

func TestRegisterRoutesWithPrefix(t *testing.T) {
	config := &Config{
		CheckInterval:      1 * time.Second,
		CheckTimeout:       1 * time.Second,
		ReadinessChecks:    []string{},
		StartupGracePeriod: 0,
	}
	m := NewManager(config)
	m.SetReady(true)
	h := NewHandler(m)

	mux := http.NewServeMux()
	h.RegisterRoutesWithPrefix(mux, "/api/v1/health")

	// Test that routes are registered with prefix
	tests := []struct {
		path   string
		method string
		status int
	}{
		{"/api/v1/health/live", http.MethodGet, http.StatusOK},
		{"/api/v1/health/ready", http.MethodGet, http.StatusOK},
		{"/api/v1/health/status", http.MethodGet, http.StatusOK},
	}

	for _, tt := range tests {
		req := httptest.NewRequest(tt.method, tt.path, nil)
		w := httptest.NewRecorder()

		mux.ServeHTTP(w, req)

		if w.Code != tt.status {
			t.Errorf("Expected status %d for %s, got %d", tt.status, tt.path, w.Code)
		}
	}
}
