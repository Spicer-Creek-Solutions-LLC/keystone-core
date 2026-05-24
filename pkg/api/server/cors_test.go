// SPDX-License-Identifier: Apache-2.0

package server

import (
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"go.keystone-core.io/keystone-core/internal/config"
)

func newCORSWithCounter() (http.Handler, *atomic.Int64, config.CORSConfig) {
	cfg := config.CORSConfig{
		Enabled:        true,
		AllowedOrigins: []string{"https://example.com", "https://app.example.com"},
		AllowedMethods: []string{"GET", "POST", "OPTIONS"},
		AllowedHeaders: []string{"Authorization", "Content-Type"},
	}
	var calls atomic.Int64
	next := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls.Add(1)
		w.WriteHeader(http.StatusOK)
	})
	return corsMiddleware(cfg)(next), &calls, cfg
}

func TestCORS_PreflightAllowedOrigin(t *testing.T) {
	h, calls, _ := newCORSWithCounter()
	req := httptest.NewRequest(http.MethodOptions, "/foo", nil)
	req.Header.Set("Origin", "https://example.com")
	rr := httptest.NewRecorder()

	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusNoContent {
		t.Errorf("status = %d, want 204", rr.Code)
	}
	if got := rr.Header().Get("Access-Control-Allow-Origin"); got != "https://example.com" {
		t.Errorf("Allow-Origin = %q, want echoed origin", got)
	}
	if got := rr.Header().Get("Access-Control-Allow-Methods"); got == "" {
		t.Error("Allow-Methods missing on preflight")
	}
	if got := rr.Header().Get("Access-Control-Allow-Headers"); got == "" {
		t.Error("Allow-Headers missing on preflight")
	}
	if got := rr.Header().Get("Access-Control-Max-Age"); got != "600" {
		t.Errorf("Max-Age = %q, want 600", got)
	}
	if calls.Load() != 0 {
		t.Errorf("next handler invoked %d times on preflight; want 0", calls.Load())
	}
}

func TestCORS_PreflightDisallowedOriginFallsThrough(t *testing.T) {
	h, calls, _ := newCORSWithCounter()
	req := httptest.NewRequest(http.MethodOptions, "/foo", nil)
	req.Header.Set("Origin", "https://evil.example.org")
	rr := httptest.NewRecorder()

	h.ServeHTTP(rr, req)

	if got := rr.Header().Get("Access-Control-Allow-Origin"); got != "" {
		t.Errorf("Allow-Origin = %q, want empty for disallowed origin", got)
	}
	if calls.Load() != 1 {
		t.Errorf("next not invoked for disallowed origin; calls=%d", calls.Load())
	}
}

func TestCORS_GETAllowedOrigin(t *testing.T) {
	h, calls, _ := newCORSWithCounter()
	req := httptest.NewRequest(http.MethodGet, "/foo", nil)
	req.Header.Set("Origin", "https://app.example.com")
	rr := httptest.NewRecorder()

	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", rr.Code)
	}
	if got := rr.Header().Get("Access-Control-Allow-Origin"); got != "https://app.example.com" {
		t.Errorf("Allow-Origin = %q", got)
	}
	if got := rr.Header().Get("Vary"); got != "Origin" {
		t.Errorf("Vary = %q, want Origin", got)
	}
	if got := rr.Header().Get("Access-Control-Allow-Credentials"); got != "true" {
		t.Errorf("Allow-Credentials = %q", got)
	}
	if calls.Load() != 1 {
		t.Errorf("next not invoked; calls=%d", calls.Load())
	}
}

func TestCORS_NoOriginHeader(t *testing.T) {
	h, calls, _ := newCORSWithCounter()
	req := httptest.NewRequest(http.MethodGet, "/foo", nil)
	rr := httptest.NewRecorder()

	h.ServeHTTP(rr, req)

	if got := rr.Header().Get("Access-Control-Allow-Origin"); got != "" {
		t.Errorf("Allow-Origin = %q, want empty when no Origin sent", got)
	}
	if calls.Load() != 1 {
		t.Errorf("next not invoked; calls=%d", calls.Load())
	}
}

func TestCORS_WildcardOmitsCredentials(t *testing.T) {
	cfg := config.CORSConfig{
		Enabled:        true,
		AllowedOrigins: []string{"*"},
		AllowedMethods: []string{"GET"},
		AllowedHeaders: []string{"Authorization"},
	}
	next := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	h := corsMiddleware(cfg)(next)

	req := httptest.NewRequest(http.MethodGet, "/foo", nil)
	req.Header.Set("Origin", "https://anything.example.com")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if got := rr.Header().Get("Access-Control-Allow-Origin"); got != "*" {
		t.Errorf("Allow-Origin = %q, want \"*\"", got)
	}
	// Credentials must not be set when origin is wildcard.
	if got := rr.Header().Get("Access-Control-Allow-Credentials"); got != "" {
		t.Errorf("Allow-Credentials = %q with wildcard origin", got)
	}
}

func TestOriginAllowed(t *testing.T) {
	cases := []struct {
		allowed []string
		origin  string
		want    bool
	}{
		{[]string{"*"}, "anything", true},
		{[]string{"https://example.com"}, "https://example.com", true},
		{[]string{"https://example.com"}, "https://other.com", false},
		{[]string{"https://a", "https://b"}, "https://b", true},
		{nil, "https://x", false},
	}
	for _, tc := range cases {
		got := originAllowed(tc.allowed, tc.origin)
		if got != tc.want {
			t.Errorf("originAllowed(%v, %q) = %v, want %v", tc.allowed, tc.origin, got, tc.want)
		}
	}
}
