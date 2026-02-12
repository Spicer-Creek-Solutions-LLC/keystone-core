package config

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/shawnbutts/keystone-core/internal/config"
)

func testConfig() *config.Config {
	return &config.Config{
		Server: config.ServerConfig{
			ListenAddr: "0.0.0.0",
			HTTPPort:   8080,
			GRPCPort:   9090,
		},
		NATS: config.NATSConfig{
			Mode:  config.NATSModeEmbedded,
			Token: "secret-nats-token",
		},
		Auth: config.AuthConfig{
			Enabled: true,
			Type:    "jwt",
			JWT: config.JWTAuthConfig{
				Secret: "jwt-signing-secret",
				Issuer: "kscore",
			},
		},
		Webhook: config.WebhookConfig{
			Enabled:     true,
			HMACSecret:  "hmac-secret",
			BearerToken: "bearer-secret",
		},
	}
}

func TestGetConfig_Success(t *testing.T) {
	h := NewHandler(testConfig())

	req := httptest.NewRequest(http.MethodGet, "/api/v1/config", nil)
	w := httptest.NewRecorder()

	h.handleGetConfig(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	ct := w.Header().Get("Content-Type")
	if ct != "application/json" {
		t.Errorf("expected application/json content type, got %s", ct)
	}

	var result map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	// Verify some expected fields exist
	if _, ok := result["Server"]; !ok {
		t.Error("expected Server field in response")
	}
	if _, ok := result["NATS"]; !ok {
		t.Error("expected NATS field in response")
	}
}

func TestGetConfig_MethodNotAllowed(t *testing.T) {
	h := NewHandler(testConfig())

	for _, method := range []string{http.MethodPost, http.MethodPut, http.MethodDelete, http.MethodPatch} {
		t.Run(method, func(t *testing.T) {
			req := httptest.NewRequest(method, "/api/v1/config", nil)
			w := httptest.NewRecorder()

			h.handleGetConfig(w, req)

			if w.Code != http.StatusMethodNotAllowed {
				t.Errorf("expected 405 for %s, got %d", method, w.Code)
			}
		})
	}
}

func TestGetConfig_RedactsSensitiveFields(t *testing.T) {
	h := NewHandler(testConfig())

	req := httptest.NewRequest(http.MethodGet, "/api/v1/config", nil)
	w := httptest.NewRecorder()

	h.handleGetConfig(w, req)

	body := w.Body.String()

	// Secret values should not appear in the response
	secrets := []string{"secret-nats-token", "jwt-signing-secret", "hmac-secret", "bearer-secret"}
	for _, s := range secrets {
		if containsString(body, s) {
			t.Errorf("response contains unredacted secret: %s", s)
		}
	}

	// [REDACTED] should appear for the non-empty secrets
	if !containsString(body, "[REDACTED]") {
		t.Error("expected [REDACTED] in response")
	}
}

func TestGetConfig_RegisterRoutes(t *testing.T) {
	h := NewHandler(testConfig())
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/config", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
}

func containsString(s, substr string) bool {
	return len(s) >= len(substr) && searchString(s, substr)
}

func searchString(s, substr string) bool {
	for i := 0; i+len(substr) <= len(s); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
