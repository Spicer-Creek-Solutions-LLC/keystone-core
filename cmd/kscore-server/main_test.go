// Copyright 2024 Keystone Core Contributors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"math/big"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/spf13/cobra"

	"github.com/prometheus/client_golang/prometheus/collectors"

	"github.com/shawnbutts/keystone-core/internal/config"
	"github.com/shawnbutts/keystone-core/internal/metrics"
	"github.com/shawnbutts/keystone-core/internal/ratelimit"
	internaltracing "github.com/shawnbutts/keystone-core/internal/tracing"
	"github.com/shawnbutts/keystone-core/pkg/api/auth"
)

func TestRootCommand(t *testing.T) {
	cmd := newRootCmd()
	if cmd == nil {
		t.Fatal("expected root command to not be nil")
	}

	// Check basic properties
	if cmd.Use != "kscore-server" {
		t.Errorf("expected Use to be 'kscore-server', got %s", cmd.Use)
	}

	if !strings.Contains(cmd.Short, "control plane") {
		t.Errorf("expected Short to contain 'control plane', got %s", cmd.Short)
	}

	if !strings.Contains(cmd.Long, "remote execution") {
		t.Errorf("expected Long to contain 'remote execution', got %s", cmd.Long)
	}

	// Check that version subcommand exists
	found := false
	for _, sub := range cmd.Commands() {
		if sub.Name() == "version" {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected version subcommand not found")
	}
}

func TestVersionCommand(t *testing.T) {
	cmd := newRootCmd()
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetArgs([]string{"version"})

	err := cmd.Execute()
	if err != nil {
		t.Fatalf("version command failed: %v", err)
	}

	output := buf.String()
	if !strings.Contains(output, "Keystone Core") {
		t.Errorf("expected version output to contain 'Keystone Core', got: %s", output)
	}
}

func TestHelpCommand(t *testing.T) {
	cmd := newRootCmd()
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetArgs([]string{"--help"})

	err := cmd.Execute()
	if err != nil {
		t.Fatalf("help command failed: %v", err)
	}

	output := buf.String()
	if !strings.Contains(output, "Usage:") {
		t.Errorf("expected help output to contain 'Usage:', got: %s", output)
	}
	if !strings.Contains(output, "kscore-server") {
		t.Errorf("expected help output to contain 'kscore-server', got: %s", output)
	}
	if !strings.Contains(output, "control plane") {
		t.Errorf("expected help output to mention 'control plane', got: %s", output)
	}
}

func TestConfigFlag(t *testing.T) {
	cmd := newRootCmd()

	// Check config flag
	configFlag := cmd.PersistentFlags().Lookup("config")
	if configFlag == nil {
		t.Fatal("expected --config flag")
	}
	if configFlag.DefValue != "" {
		t.Errorf("expected config default to be empty, got %s", configFlag.DefValue)
	}
}

func TestVersionCommandExists(t *testing.T) {
	cmd := newRootCmd()
	versionCmd := findSubcommand(cmd, "version")
	if versionCmd == nil {
		t.Fatal("version subcommand not found")
	}

	if versionCmd.Use != "version" {
		t.Errorf("expected Use to be 'version', got %s", versionCmd.Use)
	}

	if !strings.Contains(versionCmd.Short, "version") {
		t.Errorf("expected Short to contain 'version', got %s", versionCmd.Short)
	}
}

func TestVersionCommandHelp(t *testing.T) {
	cmd := newRootCmd()
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetArgs([]string{"version", "--help"})

	err := cmd.Execute()
	if err != nil {
		t.Fatalf("version --help failed: %v", err)
	}

	output := buf.String()
	if !strings.Contains(output, "Usage:") {
		t.Errorf("expected help output to contain 'Usage:', got: %s", output)
	}
}

func TestCommandStructure(t *testing.T) {
	tests := []struct {
		name        string
		cmdFactory  func() *cobra.Command
		expectedUse string
	}{
		{
			name:        "root command",
			cmdFactory:  newRootCmd,
			expectedUse: "kscore-server",
		},
		{
			name:        "version command",
			cmdFactory:  newVersionCmd,
			expectedUse: "version",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd := tt.cmdFactory()
			if cmd.Use != tt.expectedUse {
				t.Errorf("expected Use to be %s, got %s", tt.expectedUse, cmd.Use)
			}
		})
	}
}

func TestMultipleCommandCreations(t *testing.T) {
	// Test that we can create multiple command instances
	// This tests for state isolation between instances
	for i := 0; i < 3; i++ {
		cmd := newRootCmd()
		buf := new(bytes.Buffer)
		cmd.SetOut(buf)
		cmd.SetArgs([]string{"version"})

		err := cmd.Execute()
		if err != nil {
			t.Fatalf("execution %d failed: %v", i, err)
		}
	}
}

func TestSilenceSettings(t *testing.T) {
	cmd := newRootCmd()

	if !cmd.SilenceUsage {
		t.Error("expected SilenceUsage to be true")
	}

	if !cmd.SilenceErrors {
		t.Error("expected SilenceErrors to be true")
	}
}

func TestRootHasRunFunction(t *testing.T) {
	cmd := newRootCmd()

	if cmd.Run == nil {
		t.Error("expected root command to have a Run function")
	}
}

// findSubcommand finds a subcommand by name
func findSubcommand(cmd *cobra.Command, name string) *cobra.Command {
	for _, sub := range cmd.Commands() {
		if sub.Name() == name {
			return sub
		}
	}
	return nil
}

func TestRateLimitMiddleware_AllowsRequests(t *testing.T) {
	limiter := ratelimit.NewTokenBucket(&ratelimit.Config{
		Strategy:  ratelimit.StrategyTokenBucket,
		Limit:     100,
		Window:    time.Minute,
		BurstSize: 100,
	})
	defer limiter.Stop()

	cfg := config.RateLimitConfig{
		Enabled:           true,
		RequestsPerMinute: 100,
		Burst:             100,
		KeyExtractor:      "ip",
	}

	handler := rateLimitMiddleware(
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		}),
		limiter,
		cfg,
	)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/agents", nil)
	req.RemoteAddr = "192.168.1.1:12345"
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rec.Code)
	}
	if rec.Header().Get("X-RateLimit-Limit") == "" {
		t.Error("expected X-RateLimit-Limit header")
	}
	if rec.Header().Get("X-RateLimit-Remaining") == "" {
		t.Error("expected X-RateLimit-Remaining header")
	}
}

func TestRateLimitMiddleware_RejectsExcessRequests(t *testing.T) {
	limiter := ratelimit.NewTokenBucket(&ratelimit.Config{
		Strategy:  ratelimit.StrategyTokenBucket,
		Limit:     2,
		Window:    time.Minute,
		BurstSize: 2,
	})
	defer limiter.Stop()

	cfg := config.RateLimitConfig{
		Enabled:           true,
		RequestsPerMinute: 2,
		Burst:             2,
		KeyExtractor:      "ip",
	}

	handler := rateLimitMiddleware(
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		}),
		limiter,
		cfg,
	)

	// Exhaust the bucket
	for i := 0; i < 2; i++ {
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.RemoteAddr = "10.0.0.1:9999"
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("request %d: expected 200, got %d", i, rec.Code)
		}
	}

	// Third request should be rejected
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.RemoteAddr = "10.0.0.1:9999"
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusTooManyRequests {
		t.Errorf("expected 429, got %d", rec.Code)
	}
	if rec.Header().Get("Retry-After") == "" {
		t.Error("expected Retry-After header on 429 response")
	}
	if rec.Header().Get("X-RateLimit-Reset") == "" {
		t.Error("expected X-RateLimit-Reset header on 429 response")
	}
	if ct := rec.Header().Get("Content-Type"); ct != "application/json" {
		t.Errorf("expected Content-Type application/json, got %q", ct)
	}
	var errResp map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &errResp); err != nil {
		t.Fatalf("failed to unmarshal 429 response: %v", err)
	}
	if errResp["error"] != "rate_limit_exceeded" {
		t.Errorf("error code = %q, want %q", errResp["error"], "rate_limit_exceeded")
	}
	if errResp["message"] != "Rate limit exceeded" {
		t.Errorf("message = %q, want %q", errResp["message"], "Rate limit exceeded")
	}
}

func TestExtractRateLimitKey(t *testing.T) {
	tests := []struct {
		name     string
		cfg      config.RateLimitConfig
		headers  map[string]string
		remote   string
		expected string
	}{
		{
			name:     "ip extractor uses RemoteAddr",
			cfg:      config.RateLimitConfig{KeyExtractor: "ip"},
			remote:   "192.168.1.100:5555",
			expected: "ip:192.168.1.100",
		},
		{
			name:     "ip extractor uses X-Forwarded-For",
			cfg:      config.RateLimitConfig{KeyExtractor: "ip"},
			headers:  map[string]string{"X-Forwarded-For": "10.0.0.5, 172.16.0.1"},
			remote:   "127.0.0.1:8080",
			expected: "ip:10.0.0.5",
		},
		{
			name:     "ip extractor uses X-Real-IP",
			cfg:      config.RateLimitConfig{KeyExtractor: "ip"},
			headers:  map[string]string{"X-Real-IP": "203.0.113.50"},
			remote:   "127.0.0.1:8080",
			expected: "ip:203.0.113.50",
		},
		{
			name:     "apikey extractor uses X-API-Key header",
			cfg:      config.RateLimitConfig{KeyExtractor: "apikey"},
			headers:  map[string]string{"X-API-Key": "key-abc123"},
			remote:   "10.0.0.1:1234",
			expected: "apikey:key-abc123",
		},
		{
			name:     "apikey extractor returns anonymous when missing",
			cfg:      config.RateLimitConfig{KeyExtractor: "apikey"},
			remote:   "10.0.0.1:1234",
			expected: "apikey:anonymous",
		},
		{
			name:     "header extractor uses configured header",
			cfg:      config.RateLimitConfig{KeyExtractor: "header", HeaderName: "X-Tenant-ID"},
			headers:  map[string]string{"X-Tenant-ID": "tenant-42"},
			remote:   "10.0.0.1:1234",
			expected: "header:tenant-42",
		},
		{
			name:     "header extractor returns unknown when missing",
			cfg:      config.RateLimitConfig{KeyExtractor: "header", HeaderName: "X-Tenant-ID"},
			remote:   "10.0.0.1:1234",
			expected: "header:unknown",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/", nil)
			req.RemoteAddr = tt.remote
			for k, v := range tt.headers {
				req.Header.Set(k, v)
			}

			got := extractRateLimitKey(req, tt.cfg)
			if got != tt.expected {
				t.Errorf("extractRateLimitKey() = %q, want %q", got, tt.expected)
			}
		})
	}
}

func TestClientIP(t *testing.T) {
	tests := []struct {
		name     string
		remote   string
		headers  map[string]string
		expected string
	}{
		{
			name:     "standard RemoteAddr",
			remote:   "192.168.1.1:12345",
			expected: "192.168.1.1",
		},
		{
			name:     "RemoteAddr without port",
			remote:   "192.168.1.1",
			expected: "192.168.1.1",
		},
		{
			name:     "X-Forwarded-For single IP",
			remote:   "127.0.0.1:8080",
			headers:  map[string]string{"X-Forwarded-For": "10.0.0.1"},
			expected: "10.0.0.1",
		},
		{
			name:     "X-Forwarded-For chain takes first",
			remote:   "127.0.0.1:8080",
			headers:  map[string]string{"X-Forwarded-For": "10.0.0.1, 172.16.0.1, 192.168.1.1"},
			expected: "10.0.0.1",
		},
		{
			name:     "X-Real-IP preferred over RemoteAddr",
			remote:   "127.0.0.1:8080",
			headers:  map[string]string{"X-Real-IP": "203.0.113.5"},
			expected: "203.0.113.5",
		},
		{
			name:     "X-Forwarded-For preferred over X-Real-IP",
			remote:   "127.0.0.1:8080",
			headers:  map[string]string{"X-Forwarded-For": "10.0.0.1", "X-Real-IP": "203.0.113.5"},
			expected: "10.0.0.1",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/", nil)
			req.RemoteAddr = tt.remote
			for k, v := range tt.headers {
				req.Header.Set(k, v)
			}

			got := clientIP(req)
			if got != tt.expected {
				t.Errorf("clientIP() = %q, want %q", got, tt.expected)
			}
		})
	}
}

func TestNewRateLimiterFromConfig(t *testing.T) {
	cfg := config.RateLimitConfig{
		Enabled:           true,
		RequestsPerMinute: 60,
		Burst:             10,
		KeyExtractor:      "ip",
	}

	limiter := newRateLimiterFromConfig(cfg)
	defer limiter.Stop()

	if limiter == nil {
		t.Fatal("expected non-nil limiter")
	}
}

func TestRateLimitMiddleware_DifferentClientsIndependent(t *testing.T) {
	limiter := ratelimit.NewTokenBucket(&ratelimit.Config{
		Strategy:  ratelimit.StrategyTokenBucket,
		Limit:     1,
		Window:    time.Minute,
		BurstSize: 1,
	})
	defer limiter.Stop()

	cfg := config.RateLimitConfig{
		Enabled:           true,
		RequestsPerMinute: 1,
		Burst:             1,
		KeyExtractor:      "ip",
	}

	handler := rateLimitMiddleware(
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		}),
		limiter,
		cfg,
	)

	// Client A exhausts its limit
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.RemoteAddr = "10.0.0.1:1111"
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("client A first request: expected 200, got %d", rec.Code)
	}

	// Client B should still be allowed
	req = httptest.NewRequest(http.MethodGet, "/", nil)
	req.RemoteAddr = "10.0.0.2:2222"
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Errorf("client B first request: expected 200, got %d", rec.Code)
	}
}

func newOKHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
}

// testAuthenticator is a simple authenticator for testing HTTP auth middleware.
type testAuthenticator struct {
	validKey  string
	principal *auth.Principal
}

func (a *testAuthenticator) Name() string { return "test" }
func (a *testAuthenticator) Authenticate(_ context.Context, creds string) (*auth.Principal, error) {
	if creds == a.validKey {
		return a.principal, nil
	}
	return nil, auth.ErrInvalidCredentials
}

func newTestAuthInterceptorCfg(validKey string) *auth.InterceptorConfig {
	return &auth.InterceptorConfig{
		Authenticators: []auth.Authenticator{
			&testAuthenticator{
				validKey: validKey,
				principal: &auth.Principal{
					ID:         "test-user",
					Name:       "Test User",
					Role:       auth.RoleAdmin,
					AuthMethod: "test",
				},
			},
		},
		MetadataKey: "x-api-key",
	}
}

func TestHTTPAuthMiddleware_ValidBearerToken(t *testing.T) {
	interceptorCfg := newTestAuthInterceptorCfg("secret-key-123")
	authCfg := config.AuthConfig{Enabled: true, Type: "apikey"}

	handler := httpAuthMiddleware(newOKHandler(), interceptorCfg, authCfg)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/agents", nil)
	req.Header.Set("Authorization", "Bearer secret-key-123")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("valid Bearer token: expected 200, got %d, body: %s", rec.Code, rec.Body.String())
	}
}

func TestHTTPAuthMiddleware_ValidAPIKeyHeader(t *testing.T) {
	interceptorCfg := newTestAuthInterceptorCfg("secret-key-123")
	authCfg := config.AuthConfig{Enabled: true, Type: "apikey"}

	handler := httpAuthMiddleware(newOKHandler(), interceptorCfg, authCfg)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/agents", nil)
	req.Header.Set("X-API-Key", "secret-key-123")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("valid X-API-Key: expected 200, got %d, body: %s", rec.Code, rec.Body.String())
	}
}

func TestHTTPAuthMiddleware_MissingCredentials(t *testing.T) {
	interceptorCfg := newTestAuthInterceptorCfg("secret-key-123")
	authCfg := config.AuthConfig{Enabled: true, Type: "apikey"}

	handler := httpAuthMiddleware(newOKHandler(), interceptorCfg, authCfg)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/agents", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("missing credentials: expected 401, got %d", rec.Code)
	}
	var errResp map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &errResp); err != nil {
		t.Fatalf("failed to unmarshal 401 response: %v", err)
	}
	if errResp["error"] != "unauthorized" {
		t.Errorf("error code = %q, want %q", errResp["error"], "unauthorized")
	}
}

func TestHTTPAuthMiddleware_InvalidCredentials(t *testing.T) {
	interceptorCfg := newTestAuthInterceptorCfg("secret-key-123")
	authCfg := config.AuthConfig{Enabled: true, Type: "apikey"}

	handler := httpAuthMiddleware(newOKHandler(), interceptorCfg, authCfg)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/agents", nil)
	req.Header.Set("Authorization", "Bearer wrong-key")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("invalid credentials: expected 401, got %d", rec.Code)
	}
}

func TestHTTPAuthMiddleware_HealthBypass(t *testing.T) {
	interceptorCfg := newTestAuthInterceptorCfg("secret-key-123")
	authCfg := config.AuthConfig{Enabled: true, Type: "apikey"}

	handler := httpAuthMiddleware(newOKHandler(), interceptorCfg, authCfg)

	paths := []string{"/health/ready", "/health/status", "/api/status"}
	for _, path := range paths {
		t.Run(path, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, path, nil)
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, req)

			if rec.Code != http.StatusOK {
				t.Errorf("health bypass %s: expected 200, got %d", path, rec.Code)
			}
		})
	}
}

func TestHTTPAuthMiddleware_CustomHeaderName(t *testing.T) {
	interceptorCfg := newTestAuthInterceptorCfg("my-custom-key")
	authCfg := config.AuthConfig{
		Enabled: true,
		Type:    "apikey",
		APIKey:  config.APIKeyAuthConfig{HeaderName: "X-Custom-Auth"},
	}

	handler := httpAuthMiddleware(newOKHandler(), interceptorCfg, authCfg)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/agents", nil)
	req.Header.Set("X-Custom-Auth", "my-custom-key")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("custom header: expected 200, got %d, body: %s", rec.Code, rec.Body.String())
	}
}

func TestHTTPAuthMiddleware_PrincipalInContext(t *testing.T) {
	interceptorCfg := newTestAuthInterceptorCfg("secret-key-123")
	authCfg := config.AuthConfig{Enabled: true, Type: "apikey"}

	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		principal, ok := auth.PrincipalFromContext(r.Context())
		if !ok || principal == nil {
			t.Error("expected principal in context")
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		if principal.Name != "Test User" {
			t.Errorf("principal name = %q, want %q", principal.Name, "Test User")
		}
		w.WriteHeader(http.StatusOK)
	})

	handler := httpAuthMiddleware(inner, interceptorCfg, authCfg)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/agents", nil)
	req.Header.Set("Authorization", "Bearer secret-key-123")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("principal context: expected 200, got %d", rec.Code)
	}
}

func TestCORSMiddleware_PreflightWildcard(t *testing.T) {
	cfg := config.CORSConfig{
		Enabled:          true,
		AllowedOrigins:   []string{"*"},
		AllowedMethods:   []string{"GET", "POST", "DELETE"},
		AllowedHeaders:   []string{"Content-Type", "Authorization"},
		AllowCredentials: false,
		MaxAge:           3600,
	}

	handler := corsMiddleware(newOKHandler(), cfg)

	req := httptest.NewRequest(http.MethodOptions, "/api/v1/agents", nil)
	req.Header.Set("Origin", "https://example.com")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Errorf("expected 204, got %d", rec.Code)
	}
	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "*" {
		t.Errorf("expected Allow-Origin *, got %q", got)
	}
	if got := rec.Header().Get("Access-Control-Allow-Methods"); got != "GET, POST, DELETE" {
		t.Errorf("expected Allow-Methods 'GET, POST, DELETE', got %q", got)
	}
	if got := rec.Header().Get("Access-Control-Allow-Headers"); got != "Content-Type, Authorization" {
		t.Errorf("expected Allow-Headers 'Content-Type, Authorization', got %q", got)
	}
	if got := rec.Header().Get("Access-Control-Max-Age"); got != "3600" {
		t.Errorf("expected Max-Age 3600, got %q", got)
	}
	if got := rec.Header().Get("Access-Control-Allow-Credentials"); got != "" {
		t.Errorf("expected no Allow-Credentials header, got %q", got)
	}
}

func TestCORSMiddleware_PreflightSpecificOrigin(t *testing.T) {
	cfg := config.CORSConfig{
		Enabled:          true,
		AllowedOrigins:   []string{"https://app.example.com", "https://admin.example.com"},
		AllowedMethods:   []string{"GET", "POST"},
		AllowedHeaders:   []string{"Content-Type"},
		AllowCredentials: true,
		MaxAge:           600,
	}

	handler := corsMiddleware(newOKHandler(), cfg)

	req := httptest.NewRequest(http.MethodOptions, "/api/v1/agents", nil)
	req.Header.Set("Origin", "https://app.example.com")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Errorf("expected 204, got %d", rec.Code)
	}
	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "https://app.example.com" {
		t.Errorf("expected origin echo, got %q", got)
	}
	if got := rec.Header().Get("Vary"); got != "Origin" {
		t.Errorf("expected Vary: Origin, got %q", got)
	}
	if got := rec.Header().Get("Access-Control-Allow-Credentials"); got != "true" {
		t.Errorf("expected Allow-Credentials true, got %q", got)
	}
}

func TestCORSMiddleware_DisallowedOrigin(t *testing.T) {
	cfg := config.CORSConfig{
		Enabled:        true,
		AllowedOrigins: []string{"https://trusted.example.com"},
		AllowedMethods: []string{"GET"},
		AllowedHeaders: []string{"Content-Type"},
		MaxAge:         600,
	}

	handler := corsMiddleware(newOKHandler(), cfg)

	req := httptest.NewRequest(http.MethodOptions, "/api/v1/agents", nil)
	req.Header.Set("Origin", "https://evil.example.com")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	// Disallowed origin: request passes through without CORS headers
	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "" {
		t.Errorf("expected no Allow-Origin header for disallowed origin, got %q", got)
	}
}

func TestCORSMiddleware_ActualRequest(t *testing.T) {
	cfg := config.CORSConfig{
		Enabled:        true,
		AllowedOrigins: []string{"*"},
		AllowedMethods: []string{"GET"},
		AllowedHeaders: []string{"Content-Type"},
		MaxAge:         600,
	}

	handler := corsMiddleware(newOKHandler(), cfg)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/agents", nil)
	req.Header.Set("Origin", "https://example.com")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rec.Code)
	}
	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "*" {
		t.Errorf("expected Allow-Origin * on actual request, got %q", got)
	}
	// Actual requests should NOT have Allow-Methods/Allow-Headers (those are preflight-only)
	if got := rec.Header().Get("Access-Control-Allow-Methods"); got != "" {
		t.Errorf("expected no Allow-Methods on actual request, got %q", got)
	}
}

func TestCORSMiddleware_NoOriginHeader(t *testing.T) {
	cfg := config.CORSConfig{
		Enabled:        true,
		AllowedOrigins: []string{"*"},
		AllowedMethods: []string{"GET"},
		AllowedHeaders: []string{"Content-Type"},
		MaxAge:         600,
	}

	handler := corsMiddleware(newOKHandler(), cfg)

	// Request without Origin header — no CORS headers should be set
	req := httptest.NewRequest(http.MethodGet, "/api/v1/agents", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rec.Code)
	}
	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "" {
		t.Errorf("expected no Allow-Origin without Origin header, got %q", got)
	}
}

// generateTestCert creates a self-signed certificate and key in the given directory.
// Returns paths to the cert and key files.
func generateTestCert(t *testing.T, dir string) (certPath, keyPath string) {
	t.Helper()

	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}

	template := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{Organization: []string{"Test"}},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth, x509.ExtKeyUsageClientAuth},
		IsCA:         true,
		BasicConstraintsValid: true,
	}

	certDER, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	if err != nil {
		t.Fatalf("create certificate: %v", err)
	}

	certPath = filepath.Join(dir, "cert.pem")
	keyPath = filepath.Join(dir, "key.pem")

	certFile, err := os.Create(certPath)
	if err != nil {
		t.Fatalf("create cert file: %v", err)
	}
	pem.Encode(certFile, &pem.Block{Type: "CERTIFICATE", Bytes: certDER})
	certFile.Close()

	keyBytes, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		t.Fatalf("marshal key: %v", err)
	}
	keyFile, err := os.Create(keyPath)
	if err != nil {
		t.Fatalf("create key file: %v", err)
	}
	pem.Encode(keyFile, &pem.Block{Type: "EC PRIVATE KEY", Bytes: keyBytes})
	keyFile.Close()

	return certPath, keyPath
}

func TestBuildTLSConfig_BasicCert(t *testing.T) {
	dir := t.TempDir()
	certPath, keyPath := generateTestCert(t, dir)

	cfg := config.TLSConfig{
		Enabled:    true,
		CertFile:   certPath,
		KeyFile:    keyPath,
		MinVersion: "1.3",
	}

	tlsCfg, err := buildTLSConfig(cfg)
	if err != nil {
		t.Fatalf("buildTLSConfig() error: %v", err)
	}
	if tlsCfg == nil {
		t.Fatal("expected non-nil TLS config")
	}
	if len(tlsCfg.Certificates) != 1 {
		t.Errorf("expected 1 certificate, got %d", len(tlsCfg.Certificates))
	}
	if tlsCfg.MinVersion != tls.VersionTLS13 {
		t.Errorf("expected TLS 1.3, got %d", tlsCfg.MinVersion)
	}
	if tlsCfg.ClientAuth != tls.NoClientCert {
		t.Errorf("expected NoClientCert without CA, got %d", tlsCfg.ClientAuth)
	}
}

func TestBuildTLSConfig_MinVersionTLS12(t *testing.T) {
	dir := t.TempDir()
	certPath, keyPath := generateTestCert(t, dir)

	cfg := config.TLSConfig{
		Enabled:    true,
		CertFile:   certPath,
		KeyFile:    keyPath,
		MinVersion: "1.2",
	}

	tlsCfg, err := buildTLSConfig(cfg)
	if err != nil {
		t.Fatalf("buildTLSConfig() error: %v", err)
	}
	if tlsCfg.MinVersion != tls.VersionTLS12 {
		t.Errorf("expected TLS 1.2 (%d), got %d", tls.VersionTLS12, tlsCfg.MinVersion)
	}
}

func TestBuildTLSConfig_WithCAFile(t *testing.T) {
	dir := t.TempDir()
	certPath, keyPath := generateTestCert(t, dir)

	// Use the same cert as the CA for simplicity (self-signed)
	cfg := config.TLSConfig{
		Enabled:    true,
		CertFile:   certPath,
		KeyFile:    keyPath,
		CAFile:     certPath,
		MinVersion: "1.3",
	}

	tlsCfg, err := buildTLSConfig(cfg)
	if err != nil {
		t.Fatalf("buildTLSConfig() error: %v", err)
	}
	if tlsCfg.ClientCAs == nil {
		t.Error("expected ClientCAs to be set when CAFile provided")
	}
	if tlsCfg.ClientAuth != tls.RequireAndVerifyClientCert {
		t.Errorf("expected RequireAndVerifyClientCert, got %d", tlsCfg.ClientAuth)
	}
}

func TestBuildTLSConfig_MissingCertFile(t *testing.T) {
	cfg := config.TLSConfig{
		Enabled:    true,
		CertFile:   "/nonexistent/cert.pem",
		KeyFile:    "/nonexistent/key.pem",
		MinVersion: "1.3",
	}

	_, err := buildTLSConfig(cfg)
	if err == nil {
		t.Fatal("expected error for missing cert file")
	}
	if !strings.Contains(err.Error(), "loading TLS certificate") {
		t.Errorf("expected 'loading TLS certificate' error, got: %v", err)
	}
}

func TestBuildTLSConfig_MissingCAFile(t *testing.T) {
	dir := t.TempDir()
	certPath, keyPath := generateTestCert(t, dir)

	cfg := config.TLSConfig{
		Enabled:    true,
		CertFile:   certPath,
		KeyFile:    keyPath,
		CAFile:     "/nonexistent/ca.pem",
		MinVersion: "1.3",
	}

	_, err := buildTLSConfig(cfg)
	if err == nil {
		t.Fatal("expected error for missing CA file")
	}
	if !strings.Contains(err.Error(), "reading CA certificate") {
		t.Errorf("expected 'reading CA certificate' error, got: %v", err)
	}
}

func TestBuildTLSConfig_InvalidCAFile(t *testing.T) {
	dir := t.TempDir()
	certPath, keyPath := generateTestCert(t, dir)

	// Create a file with invalid PEM content
	badCAPath := filepath.Join(dir, "bad-ca.pem")
	os.WriteFile(badCAPath, []byte("not a valid PEM"), 0644)

	cfg := config.TLSConfig{
		Enabled:    true,
		CertFile:   certPath,
		KeyFile:    keyPath,
		CAFile:     badCAPath,
		MinVersion: "1.3",
	}

	_, err := buildTLSConfig(cfg)
	if err == nil {
		t.Fatal("expected error for invalid CA file")
	}
	if !strings.Contains(err.Error(), "failed to parse CA certificate") {
		t.Errorf("expected 'failed to parse CA certificate' error, got: %v", err)
	}
}

func TestMetricsEndpoint_Registered(t *testing.T) {
	mux := http.NewServeMux()

	collector := metrics.NewPrometheusCollector()
	registry := collector.Registry()
	registry.MustRegister(collectors.NewGoCollector())
	registry.MustRegister(collectors.NewProcessCollector(collectors.ProcessCollectorOpts{}))
	mux.Handle("/metrics", collector.Handler())

	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	body := rec.Body.String()
	// Go metrics should be present
	if !strings.Contains(body, "go_goroutines") {
		t.Error("expected go_goroutines metric in output")
	}
	// Process metrics should be present
	if !strings.Contains(body, "process_") {
		t.Error("expected process_* metrics in output")
	}
}

func TestMetricsEndpoint_GoMetricsOnly(t *testing.T) {
	mux := http.NewServeMux()

	collector := metrics.NewPrometheusCollector()
	registry := collector.Registry()
	registry.MustRegister(collectors.NewGoCollector())
	// No process collector
	mux.Handle("/metrics", collector.Handler())

	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	body := rec.Body.String()
	if !strings.Contains(body, "go_goroutines") {
		t.Error("expected go_goroutines metric")
	}
}

func TestMetricsEndpoint_CustomPath(t *testing.T) {
	mux := http.NewServeMux()

	collector := metrics.NewPrometheusCollector()
	mux.Handle("/custom-metrics", collector.Handler())

	// Should respond at custom path
	req := httptest.NewRequest(http.MethodGet, "/custom-metrics", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected 200 at custom path, got %d", rec.Code)
	}

	// Default path should 404
	req = httptest.NewRequest(http.MethodGet, "/metrics", nil)
	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Errorf("expected 404 at /metrics when custom path used, got %d", rec.Code)
	}
}

func TestMetricsEndpoint_EmptyRegistry(t *testing.T) {
	mux := http.NewServeMux()

	collector := metrics.NewPrometheusCollector()
	// No collectors registered — should still serve an empty response
	mux.Handle("/metrics", collector.Handler())

	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
}

// --- Health endpoint tests ---

// mockHealthChecker implements healthChecker for testing.
type mockHealthChecker struct {
	err error
}

func (m *mockHealthChecker) Health() error { return m.err }

// mockDBPinger implements dbPinger for testing.
type mockDBPinger struct {
	err error
}

func (m *mockDBPinger) Ping(ctx context.Context) error { return m.err }

// mockAgentCounter implements agentCounter for testing.
type mockAgentCounter struct {
	total  int
	online int
}

func (m *mockAgentCounter) GetAgentCount() int       { return m.total }
func (m *mockAgentCounter) GetOnlineAgentCount() int  { return m.online }

func TestHealthReady_AllChecksPass(t *testing.T) {
	cfg := config.HealthConfig{
		Enabled: true,
		Checks: config.HealthChecksConfig{
			NATS:     config.HealthCheckConfig{Enabled: true, Timeout: time.Second},
			Database: config.HealthCheckConfig{Enabled: true, Timeout: time.Second},
		},
	}

	handler := healthReadyHandler(cfg, time.Now(), &mockHealthChecker{}, &mockDBPinger{})

	req := httptest.NewRequest(http.MethodGet, "/health/ready", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), `"status":"ready"`) {
		t.Errorf("expected status ready, got %s", rec.Body.String())
	}
}

func TestHealthReady_NATSFails(t *testing.T) {
	cfg := config.HealthConfig{
		Enabled: true,
		Checks: config.HealthChecksConfig{
			NATS:     config.HealthCheckConfig{Enabled: true},
			Database: config.HealthCheckConfig{Enabled: true},
		},
	}

	handler := healthReadyHandler(cfg, time.Now(), &mockHealthChecker{err: fmt.Errorf("nats down")}, &mockDBPinger{})

	req := httptest.NewRequest(http.MethodGet, "/health/ready", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Errorf("expected 503, got %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "nats") {
		t.Errorf("expected nats error in body, got %s", rec.Body.String())
	}
}

func TestHealthReady_DatabaseFails(t *testing.T) {
	cfg := config.HealthConfig{
		Enabled: true,
		Checks: config.HealthChecksConfig{
			NATS:     config.HealthCheckConfig{Enabled: true},
			Database: config.HealthCheckConfig{Enabled: true},
		},
	}

	handler := healthReadyHandler(cfg, time.Now(), &mockHealthChecker{}, &mockDBPinger{err: fmt.Errorf("db connection refused")})

	req := httptest.NewRequest(http.MethodGet, "/health/ready", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Errorf("expected 503, got %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "database") {
		t.Errorf("expected database error in body, got %s", rec.Body.String())
	}
}

func TestHealthReady_GracePeriodOverridesFailure(t *testing.T) {
	cfg := config.HealthConfig{
		Enabled:            true,
		StartupGracePeriod: 10 * time.Minute,
		Checks: config.HealthChecksConfig{
			NATS:     config.HealthCheckConfig{Enabled: true},
			Database: config.HealthCheckConfig{Enabled: true},
		},
	}

	// Start time is now, so we are within the grace period
	handler := healthReadyHandler(cfg, time.Now(), &mockHealthChecker{err: fmt.Errorf("nats down")}, &mockDBPinger{err: fmt.Errorf("db down")})

	req := httptest.NewRequest(http.MethodGet, "/health/ready", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected 200 during grace period, got %d", rec.Code)
	}
}

func TestHealthReady_GracePeriodExpired(t *testing.T) {
	cfg := config.HealthConfig{
		Enabled:            true,
		StartupGracePeriod: time.Millisecond,
		Checks: config.HealthChecksConfig{
			NATS: config.HealthCheckConfig{Enabled: true},
		},
	}

	// Start time was well in the past
	handler := healthReadyHandler(cfg, time.Now().Add(-time.Hour), &mockHealthChecker{err: fmt.Errorf("nats down")}, &mockDBPinger{})

	req := httptest.NewRequest(http.MethodGet, "/health/ready", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Errorf("expected 503 after grace period, got %d", rec.Code)
	}
}

func TestHealthReady_DisabledChecksSkipped(t *testing.T) {
	cfg := config.HealthConfig{
		Enabled: true,
		Checks: config.HealthChecksConfig{
			NATS:     config.HealthCheckConfig{Enabled: false},
			Database: config.HealthCheckConfig{Enabled: false},
		},
	}

	// Even though both would fail, they are disabled
	handler := healthReadyHandler(cfg, time.Now(), &mockHealthChecker{err: fmt.Errorf("nats down")}, &mockDBPinger{err: fmt.Errorf("db down")})

	req := httptest.NewRequest(http.MethodGet, "/health/ready", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected 200 with disabled checks, got %d", rec.Code)
	}
}

func TestHealthStatus_AllHealthy(t *testing.T) {
	cfg := config.HealthConfig{
		Enabled: true,
		Checks: config.HealthChecksConfig{
			NATS:     config.HealthCheckConfig{Enabled: true},
			Database: config.HealthCheckConfig{Enabled: true},
			Agents:   config.AgentHealthCheckConfig{Enabled: true, MinHealthy: 0.5},
		},
	}

	startTime := time.Now().Add(-time.Hour)
	handler := healthStatusHandler(cfg, startTime, &mockHealthChecker{}, &mockDBPinger{}, &mockAgentCounter{total: 10, online: 8})

	req := httptest.NewRequest(http.MethodGet, "/health/status", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rec.Code)
	}

	body := rec.Body.String()
	if !strings.Contains(body, `"status":"healthy"`) {
		t.Errorf("expected healthy status, got %s", body)
	}
	if !strings.Contains(body, `"nats"`) {
		t.Errorf("expected nats component, got %s", body)
	}
	if !strings.Contains(body, `"database"`) {
		t.Errorf("expected database component, got %s", body)
	}
	if !strings.Contains(body, `"agent_pool"`) {
		t.Errorf("expected agent_pool component, got %s", body)
	}
	if !strings.Contains(body, `"uptime_seconds"`) {
		t.Errorf("expected uptime_seconds, got %s", body)
	}
}

func TestHealthStatus_NATSUnhealthy(t *testing.T) {
	cfg := config.HealthConfig{
		Enabled: true,
		Checks: config.HealthChecksConfig{
			NATS:     config.HealthCheckConfig{Enabled: true},
			Database: config.HealthCheckConfig{Enabled: true},
		},
	}

	handler := healthStatusHandler(cfg, time.Now(), &mockHealthChecker{err: fmt.Errorf("timeout")}, &mockDBPinger{}, &mockAgentCounter{})

	req := httptest.NewRequest(http.MethodGet, "/health/status", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Errorf("expected 503, got %d", rec.Code)
	}
	body := rec.Body.String()
	if !strings.Contains(body, `"status":"degraded"`) {
		t.Errorf("expected degraded status, got %s", body)
	}
	if !strings.Contains(body, `"unhealthy"`) {
		t.Errorf("expected unhealthy nats component, got %s", body)
	}
}

func TestHealthStatus_AgentPoolDegraded(t *testing.T) {
	cfg := config.HealthConfig{
		Enabled: true,
		Checks: config.HealthChecksConfig{
			NATS:   config.HealthCheckConfig{Enabled: false},
			Agents: config.AgentHealthCheckConfig{Enabled: true, MinHealthy: 0.9},
		},
	}

	// Only 5 of 10 online = 50%, below 90% threshold
	handler := healthStatusHandler(cfg, time.Now(), &mockHealthChecker{}, &mockDBPinger{}, &mockAgentCounter{total: 10, online: 5})

	req := httptest.NewRequest(http.MethodGet, "/health/status", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Errorf("expected 503, got %d", rec.Code)
	}
	body := rec.Body.String()
	if !strings.Contains(body, `"degraded"`) {
		t.Errorf("expected degraded in body, got %s", body)
	}
}

func TestHealthStatus_DisabledChecksOmitted(t *testing.T) {
	cfg := config.HealthConfig{
		Enabled: true,
		Checks: config.HealthChecksConfig{
			NATS:     config.HealthCheckConfig{Enabled: false},
			Database: config.HealthCheckConfig{Enabled: false},
			Agents:   config.AgentHealthCheckConfig{Enabled: false},
		},
	}

	handler := healthStatusHandler(cfg, time.Now(), &mockHealthChecker{}, &mockDBPinger{}, &mockAgentCounter{})

	req := httptest.NewRequest(http.MethodGet, "/health/status", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rec.Code)
	}
	body := rec.Body.String()
	if strings.Contains(body, `"nats"`) {
		t.Errorf("expected no nats component when disabled, got %s", body)
	}
	if strings.Contains(body, `"database"`) {
		t.Errorf("expected no database component when disabled, got %s", body)
	}
	if strings.Contains(body, `"agent_pool"`) {
		t.Errorf("expected no agent_pool component when disabled, got %s", body)
	}
}

func TestTracingConfigFromServerConfig_OTLP(t *testing.T) {
	cfg := config.TracingConfig{
		Enabled:  true,
		Exporter: "otlp",
		Endpoint: "localhost:4317",
		Sampling: config.TracingSamplingConfig{
			Strategy: "ratio",
			Ratio:    0.5,
		},
		Resource: map[string]string{
			"service.name":            "test-server",
			"service.version":         "2.0.0",
			"deployment.environment":  "staging",
			"custom.attr":             "value1",
		},
	}

	result := tracingConfigFromServerConfig(cfg)

	if !result.Enabled {
		t.Error("expected Enabled=true")
	}
	if result.ServiceName != "test-server" {
		t.Errorf("expected ServiceName 'test-server', got %q", result.ServiceName)
	}
	if result.ServiceVersion != "2.0.0" {
		t.Errorf("expected ServiceVersion '2.0.0', got %q", result.ServiceVersion)
	}
	if result.Environment != "staging" {
		t.Errorf("expected Environment 'staging', got %q", result.Environment)
	}
	if result.Sampling.Type != internaltracing.SamplingProbabilistic {
		t.Errorf("expected SamplingProbabilistic, got %q", result.Sampling.Type)
	}
	if result.Sampling.Rate != 0.5 {
		t.Errorf("expected sampling rate 0.5, got %f", result.Sampling.Rate)
	}
	if len(result.Exporters) != 1 {
		t.Fatalf("expected 1 exporter, got %d", len(result.Exporters))
	}
	if result.Exporters[0].Type != internaltracing.ExporterOTLP {
		t.Errorf("expected ExporterOTLP, got %q", result.Exporters[0].Type)
	}
	if result.Exporters[0].Endpoint != "localhost:4317" {
		t.Errorf("expected endpoint 'localhost:4317', got %q", result.Exporters[0].Endpoint)
	}
	if result.ResourceAttributes["custom.attr"] != "value1" {
		t.Errorf("expected custom.attr=value1, got %q", result.ResourceAttributes["custom.attr"])
	}
}

func TestTracingConfigFromServerConfig_Zipkin(t *testing.T) {
	cfg := config.TracingConfig{
		Enabled:  true,
		Exporter: "zipkin",
		Endpoint: "http://zipkin:9411/api/v2/spans",
		Sampling: config.TracingSamplingConfig{
			Strategy: "always",
		},
	}

	result := tracingConfigFromServerConfig(cfg)

	if result.Sampling.Type != internaltracing.SamplingAlwaysOn {
		t.Errorf("expected SamplingAlwaysOn, got %q", result.Sampling.Type)
	}
	if len(result.Exporters) != 1 {
		t.Fatalf("expected 1 exporter, got %d", len(result.Exporters))
	}
	if result.Exporters[0].Type != internaltracing.ExporterZipkin {
		t.Errorf("expected ExporterZipkin, got %q", result.Exporters[0].Type)
	}
}

func TestTracingConfigFromServerConfig_NeverSample(t *testing.T) {
	cfg := config.TracingConfig{
		Enabled:  true,
		Exporter: "otlp",
		Endpoint: "localhost:4317",
		Sampling: config.TracingSamplingConfig{
			Strategy: "never",
		},
	}

	result := tracingConfigFromServerConfig(cfg)

	if result.Sampling.Type != internaltracing.SamplingAlwaysOff {
		t.Errorf("expected SamplingAlwaysOff, got %q", result.Sampling.Type)
	}
}

func TestTracingConfigFromServerConfig_ParentBased(t *testing.T) {
	cfg := config.TracingConfig{
		Enabled:  true,
		Exporter: "otlp",
		Endpoint: "localhost:4317",
		Sampling: config.TracingSamplingConfig{
			Strategy: "parent_based",
			Ratio:    0.25,
		},
	}

	result := tracingConfigFromServerConfig(cfg)

	if result.Sampling.Type != internaltracing.SamplingParentBased {
		t.Errorf("expected SamplingParentBased, got %q", result.Sampling.Type)
	}
	if result.Sampling.Rate != 0.25 {
		t.Errorf("expected rate 0.25, got %f", result.Sampling.Rate)
	}
}

func TestTracingConfigFromServerConfig_Defaults(t *testing.T) {
	cfg := config.TracingConfig{
		Enabled: true,
	}

	result := tracingConfigFromServerConfig(cfg)

	if result.ServiceName != "kscore-server" {
		t.Errorf("expected default ServiceName 'kscore-server', got %q", result.ServiceName)
	}
	if len(result.Exporters) != 0 {
		t.Errorf("expected no exporters when endpoint empty, got %d", len(result.Exporters))
	}
}
