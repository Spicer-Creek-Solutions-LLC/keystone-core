package rest

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestNoAuth(t *testing.T) {
	auth := &NoAuth{}

	t.Run("Type", func(t *testing.T) {
		if auth.Type() != "none" {
			t.Errorf("Type() = %v, want 'none'", auth.Type())
		}
	})

	t.Run("Authenticate", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/test", nil)
		err := auth.Authenticate(req)
		if err != nil {
			t.Errorf("Authenticate() error = %v", err)
		}
		// Should not add any headers
		if req.Header.Get("Authorization") != "" {
			t.Error("NoAuth should not add Authorization header")
		}
	})

	t.Run("Refresh", func(t *testing.T) {
		err := auth.Refresh(context.Background())
		if err != nil {
			t.Errorf("Refresh() error = %v", err)
		}
	})
}

func TestBasicAuth(t *testing.T) {
	auth := NewBasicAuth("testuser", "testpass")

	t.Run("Type", func(t *testing.T) {
		if auth.Type() != "basic" {
			t.Errorf("Type() = %v, want 'basic'", auth.Type())
		}
	})

	t.Run("Authenticate", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/test", nil)
		err := auth.Authenticate(req)
		if err != nil {
			t.Errorf("Authenticate() error = %v", err)
		}

		authHeader := req.Header.Get("Authorization")
		if !strings.HasPrefix(authHeader, "Basic ") {
			t.Errorf("Authorization header = %v, want 'Basic ...'", authHeader)
		}

		// Decode and verify
		// Base64("testuser:testpass") = "dGVzdHVzZXI6dGVzdHBhc3M="
		expected := "Basic dGVzdHVzZXI6dGVzdHBhc3M="
		if authHeader != expected {
			t.Errorf("Authorization header = %v, want %v", authHeader, expected)
		}
	})

	t.Run("Refresh", func(t *testing.T) {
		err := auth.Refresh(context.Background())
		if err != nil {
			t.Errorf("Refresh() error = %v", err)
		}
	})
}

func TestBearerAuth(t *testing.T) {
	auth := NewBearerAuth("my-token-123")

	t.Run("Type", func(t *testing.T) {
		if auth.Type() != "bearer" {
			t.Errorf("Type() = %v, want 'bearer'", auth.Type())
		}
	})

	t.Run("Authenticate", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/test", nil)
		err := auth.Authenticate(req)
		if err != nil {
			t.Errorf("Authenticate() error = %v", err)
		}

		authHeader := req.Header.Get("Authorization")
		expected := "Bearer my-token-123"
		if authHeader != expected {
			t.Errorf("Authorization header = %v, want %v", authHeader, expected)
		}
	})

	t.Run("SetToken", func(t *testing.T) {
		auth.SetToken("new-token")

		req := httptest.NewRequest("GET", "/test", nil)
		auth.Authenticate(req)

		authHeader := req.Header.Get("Authorization")
		expected := "Bearer new-token"
		if authHeader != expected {
			t.Errorf("Authorization header = %v, want %v", authHeader, expected)
		}
	})

	t.Run("Refresh", func(t *testing.T) {
		err := auth.Refresh(context.Background())
		if err != nil {
			t.Errorf("Refresh() error = %v", err)
		}
	})
}

func TestAPIKeyAuth(t *testing.T) {
	tests := []struct {
		name     string
		key      string
		value    string
		location APIKeyLocation
		check    func(*testing.T, *http.Request)
	}{
		{
			name:     "header location",
			key:      "X-API-Key",
			value:    "secret-key",
			location: APIKeyHeader,
			check: func(t *testing.T, req *http.Request) {
				if req.Header.Get("X-API-Key") != "secret-key" {
					t.Errorf("X-API-Key header = %v, want 'secret-key'", req.Header.Get("X-API-Key"))
				}
			},
		},
		{
			name:     "query location",
			key:      "api_key",
			value:    "secret-key",
			location: APIKeyQuery,
			check: func(t *testing.T, req *http.Request) {
				if req.URL.Query().Get("api_key") != "secret-key" {
					t.Errorf("api_key query = %v, want 'secret-key'", req.URL.Query().Get("api_key"))
				}
			},
		},
		{
			name:     "cookie location",
			key:      "session",
			value:    "session-token",
			location: APIKeyCookie,
			check: func(t *testing.T, req *http.Request) {
				cookie, err := req.Cookie("session")
				if err != nil {
					t.Errorf("Cookie not found: %v", err)
					return
				}
				if cookie.Value != "session-token" {
					t.Errorf("Cookie value = %v, want 'session-token'", cookie.Value)
				}
			},
		},
		{
			name:     "default location (header)",
			key:      "X-Custom",
			value:    "custom-value",
			location: "",
			check: func(t *testing.T, req *http.Request) {
				if req.Header.Get("X-Custom") != "custom-value" {
					t.Errorf("X-Custom header = %v, want 'custom-value'", req.Header.Get("X-Custom"))
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			auth := NewAPIKeyAuth(tt.key, tt.value, tt.location)

			if auth.Type() != "api_key" {
				t.Errorf("Type() = %v, want 'api_key'", auth.Type())
			}

			req := httptest.NewRequest("GET", "/test", nil)
			err := auth.Authenticate(req)
			if err != nil {
				t.Errorf("Authenticate() error = %v", err)
			}

			tt.check(t, req)
		})
	}
}

func TestOAuth2ClientCredentials(t *testing.T) {
	// Create a mock OAuth2 server
	tokenServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			t.Errorf("expected POST, got %s", r.Method)
		}

		contentType := r.Header.Get("Content-Type")
		if contentType != "application/x-www-form-urlencoded" {
			t.Errorf("expected form content type, got %s", contentType)
		}

		// Check basic auth
		user, pass, ok := r.BasicAuth()
		if !ok || user != "client-id" || pass != "client-secret" {
			t.Error("invalid client credentials")
		}

		// Check grant type
		r.ParseForm()
		if r.Form.Get("grant_type") != "client_credentials" {
			t.Errorf("expected grant_type=client_credentials, got %s", r.Form.Get("grant_type"))
		}

		// Return token
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"access_token": "new-access-token",
			"token_type":   "Bearer",
			"expires_in":   3600,
		})
	}))
	defer tokenServer.Close()

	auth := NewOAuth2ClientCredentials("client-id", "client-secret", tokenServer.URL)
	auth.Scopes = []string{"read", "write"}

	t.Run("Type", func(t *testing.T) {
		if auth.Type() != "oauth2_client_credentials" {
			t.Errorf("Type() = %v, want 'oauth2_client_credentials'", auth.Type())
		}
	})

	t.Run("Authenticate fetches token", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/test", nil)
		err := auth.Authenticate(req)
		if err != nil {
			t.Errorf("Authenticate() error = %v", err)
		}

		authHeader := req.Header.Get("Authorization")
		if authHeader != "Bearer new-access-token" {
			t.Errorf("Authorization header = %v, want 'Bearer new-access-token'", authHeader)
		}
	})

	t.Run("GetAccessToken", func(t *testing.T) {
		token := auth.GetAccessToken()
		if token != "new-access-token" {
			t.Errorf("GetAccessToken() = %v, want 'new-access-token'", token)
		}
	})

	t.Run("Refresh", func(t *testing.T) {
		err := auth.Refresh(context.Background())
		if err != nil {
			t.Errorf("Refresh() error = %v", err)
		}
	})
}

func TestOAuth2ClientCredentialsErrors(t *testing.T) {
	t.Run("token request fails", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusUnauthorized)
			w.Write([]byte("invalid credentials"))
		}))
		defer server.Close()

		auth := NewOAuth2ClientCredentials("bad-id", "bad-secret", server.URL)
		req := httptest.NewRequest("GET", "/test", nil)

		err := auth.Authenticate(req)
		if err == nil {
			t.Error("expected error")
		}
	})

	t.Run("invalid token response", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte("not json"))
		}))
		defer server.Close()

		auth := NewOAuth2ClientCredentials("id", "secret", server.URL)
		req := httptest.NewRequest("GET", "/test", nil)

		err := auth.Authenticate(req)
		if err == nil {
			t.Error("expected error")
		}
	})

	t.Run("network error", func(t *testing.T) {
		auth := NewOAuth2ClientCredentials("id", "secret", "http://localhost:99999/token")
		req := httptest.NewRequest("GET", "/test", nil)

		err := auth.Authenticate(req)
		if err == nil {
			t.Error("expected error")
		}
	})
}

func TestOAuth2ClientCredentialsTokenRefresh(t *testing.T) {
	callCount := 0
	tokenServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"access_token": "token-" + string(rune('0'+callCount)),
			"token_type":   "Bearer",
			"expires_in":   3600,
		})
	}))
	defer tokenServer.Close()

	auth := NewOAuth2ClientCredentials("client-id", "client-secret", tokenServer.URL)

	// First request fetches token
	req1 := httptest.NewRequest("GET", "/test", nil)
	auth.Authenticate(req1)
	if callCount != 1 {
		t.Errorf("expected 1 token request, got %d", callCount)
	}

	// Second request uses cached token
	req2 := httptest.NewRequest("GET", "/test", nil)
	auth.Authenticate(req2)
	if callCount != 1 {
		t.Errorf("expected 1 token request (cached), got %d", callCount)
	}

	// Force token expiration
	auth.mu.Lock()
	auth.expiresAt = time.Now().Add(-time.Hour)
	auth.mu.Unlock()

	// Third request refreshes token
	req3 := httptest.NewRequest("GET", "/test", nil)
	auth.Authenticate(req3)
	if callCount != 2 {
		t.Errorf("expected 2 token requests (refresh), got %d", callCount)
	}
}

func TestOAuth2ResourceOwner(t *testing.T) {
	tokenServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		r.ParseForm()

		if r.Form.Get("grant_type") != "password" {
			t.Errorf("expected grant_type=password, got %s", r.Form.Get("grant_type"))
		}
		if r.Form.Get("username") != "testuser" {
			t.Errorf("expected username=testuser, got %s", r.Form.Get("username"))
		}
		if r.Form.Get("password") != "testpass" {
			t.Errorf("expected password=testpass, got %s", r.Form.Get("password"))
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"access_token": "user-token",
			"token_type":   "Bearer",
			"expires_in":   3600,
		})
	}))
	defer tokenServer.Close()

	auth := NewOAuth2ResourceOwner("client-id", "client-secret", tokenServer.URL, "testuser", "testpass")
	auth.Scopes = []string{"profile"}

	t.Run("Type", func(t *testing.T) {
		if auth.Type() != "oauth2_password" {
			t.Errorf("Type() = %v, want 'oauth2_password'", auth.Type())
		}
	})

	t.Run("Authenticate", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/test", nil)
		err := auth.Authenticate(req)
		if err != nil {
			t.Errorf("Authenticate() error = %v", err)
		}

		authHeader := req.Header.Get("Authorization")
		if authHeader != "Bearer user-token" {
			t.Errorf("Authorization header = %v, want 'Bearer user-token'", authHeader)
		}
	})
}

func TestOAuth2ResourceOwnerNoClientCredentials(t *testing.T) {
	tokenServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Should not have basic auth
		_, _, ok := r.BasicAuth()
		if ok {
			t.Error("should not send basic auth when client ID is empty")
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"access_token": "user-token",
			"token_type":   "Bearer",
		})
	}))
	defer tokenServer.Close()

	auth := NewOAuth2ResourceOwner("", "", tokenServer.URL, "testuser", "testpass")

	req := httptest.NewRequest("GET", "/test", nil)
	err := auth.Authenticate(req)
	if err != nil {
		t.Errorf("Authenticate() error = %v", err)
	}
}

func TestDigestAuth(t *testing.T) {
	auth := NewDigestAuth("testuser", "testpass")

	t.Run("Type", func(t *testing.T) {
		if auth.Type() != "digest" {
			t.Errorf("Type() = %v, want 'digest'", auth.Type())
		}
	})

	t.Run("Authenticate without challenge", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/test", nil)
		err := auth.Authenticate(req)
		if err != nil {
			t.Errorf("Authenticate() error = %v", err)
		}
		// Without challenge, should not add auth header
	})

	t.Run("SetChallenge", func(t *testing.T) {
		auth.SetChallenge("testrealm", "testnonce", "auth")

		if auth.realm != "testrealm" {
			t.Errorf("realm = %v, want 'testrealm'", auth.realm)
		}
		if auth.nonce != "testnonce" {
			t.Errorf("nonce = %v, want 'testnonce'", auth.nonce)
		}
		if auth.qop != "auth" {
			t.Errorf("qop = %v, want 'auth'", auth.qop)
		}
	})

	t.Run("Refresh", func(t *testing.T) {
		err := auth.Refresh(context.Background())
		if err != nil {
			t.Errorf("Refresh() error = %v", err)
		}
	})
}

func TestCustomHeaderAuth(t *testing.T) {
	headers := map[string]string{
		"X-Custom-Header": "custom-value",
		"X-Another":       "another-value",
	}
	auth := NewCustomHeaderAuth(headers)

	t.Run("Type", func(t *testing.T) {
		if auth.Type() != "custom_header" {
			t.Errorf("Type() = %v, want 'custom_header'", auth.Type())
		}
	})

	t.Run("Authenticate", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/test", nil)
		err := auth.Authenticate(req)
		if err != nil {
			t.Errorf("Authenticate() error = %v", err)
		}

		if req.Header.Get("X-Custom-Header") != "custom-value" {
			t.Errorf("X-Custom-Header = %v, want 'custom-value'", req.Header.Get("X-Custom-Header"))
		}
		if req.Header.Get("X-Another") != "another-value" {
			t.Errorf("X-Another = %v, want 'another-value'", req.Header.Get("X-Another"))
		}
	})

	t.Run("SetHeader", func(t *testing.T) {
		auth.SetHeader("X-New", "new-value")

		req := httptest.NewRequest("GET", "/test", nil)
		auth.Authenticate(req)

		if req.Header.Get("X-New") != "new-value" {
			t.Errorf("X-New = %v, want 'new-value'", req.Header.Get("X-New"))
		}
	})

	t.Run("RemoveHeader", func(t *testing.T) {
		auth.RemoveHeader("X-Another")

		req := httptest.NewRequest("GET", "/test", nil)
		auth.Authenticate(req)

		if req.Header.Get("X-Another") != "" {
			t.Errorf("X-Another = %v, want ''", req.Header.Get("X-Another"))
		}
	})

	t.Run("Refresh", func(t *testing.T) {
		err := auth.Refresh(context.Background())
		if err != nil {
			t.Errorf("Refresh() error = %v", err)
		}
	})
}

func TestChainAuth(t *testing.T) {
	basic := NewBasicAuth("user", "pass")
	apiKey := NewAPIKeyAuth("X-API-Key", "key123", APIKeyHeader)

	auth := NewChainAuth(basic, apiKey)

	t.Run("Type", func(t *testing.T) {
		if auth.Type() != "chain" {
			t.Errorf("Type() = %v, want 'chain'", auth.Type())
		}
	})

	t.Run("Authenticate applies all authenticators", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/test", nil)
		err := auth.Authenticate(req)
		if err != nil {
			t.Errorf("Authenticate() error = %v", err)
		}

		// Check both headers are set
		if !strings.HasPrefix(req.Header.Get("Authorization"), "Basic ") {
			t.Error("Basic auth not applied")
		}
		if req.Header.Get("X-API-Key") != "key123" {
			t.Errorf("API key not applied: %v", req.Header.Get("X-API-Key"))
		}
	})

	t.Run("Add", func(t *testing.T) {
		bearer := NewBearerAuth("token")
		auth.Add(bearer)

		// This won't work well since both set Authorization header
		// but test that Add works
		if len(auth.authenticators) != 3 {
			t.Errorf("expected 3 authenticators, got %d", len(auth.authenticators))
		}
	})

	t.Run("Refresh", func(t *testing.T) {
		err := auth.Refresh(context.Background())
		if err != nil {
			t.Errorf("Refresh() error = %v", err)
		}
	})
}

func TestChainAuthError(t *testing.T) {
	// Create an authenticator that fails
	tokenServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer tokenServer.Close()

	oauth := NewOAuth2ClientCredentials("bad", "bad", tokenServer.URL)
	basic := NewBasicAuth("user", "pass")

	auth := NewChainAuth(oauth, basic)

	req := httptest.NewRequest("GET", "/test", nil)
	err := auth.Authenticate(req)
	if err == nil {
		t.Error("expected error from OAuth failure")
	}
}

func TestAuthenticatorInterface(t *testing.T) {
	// Verify all auth types implement Authenticator interface
	var _ Authenticator = &NoAuth{}
	var _ Authenticator = &BasicAuth{}
	var _ Authenticator = &BearerAuth{}
	var _ Authenticator = &APIKeyAuth{}
	var _ Authenticator = &OAuth2ClientCredentials{}
	var _ Authenticator = &OAuth2ResourceOwner{}
	var _ Authenticator = &DigestAuth{}
	var _ Authenticator = &CustomHeaderAuth{}
	var _ Authenticator = &ChainAuth{}
}

func TestOAuth2TokenResponseWithoutExpiry(t *testing.T) {
	tokenServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"access_token": "token-no-expiry",
			"token_type":   "Bearer",
			// No expires_in field
		})
	}))
	defer tokenServer.Close()

	auth := NewOAuth2ClientCredentials("client", "secret", tokenServer.URL)

	req := httptest.NewRequest("GET", "/test", nil)
	err := auth.Authenticate(req)
	if err != nil {
		t.Errorf("Authenticate() error = %v", err)
	}

	// Should default to 1 hour expiry
	auth.mu.RLock()
	defer auth.mu.RUnlock()
	if auth.expiresAt.Before(time.Now().Add(50 * time.Minute)) {
		t.Error("expected expiry to be about 1 hour from now")
	}
}

func TestOAuth2TokenTypeDefault(t *testing.T) {
	tokenServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"access_token": "token",
			// No token_type field
		})
	}))
	defer tokenServer.Close()

	auth := NewOAuth2ClientCredentials("client", "secret", tokenServer.URL)

	req := httptest.NewRequest("GET", "/test", nil)
	err := auth.Authenticate(req)
	if err != nil {
		t.Errorf("Authenticate() error = %v", err)
	}

	// Should default to Bearer
	authHeader := req.Header.Get("Authorization")
	if !strings.HasPrefix(authHeader, "Bearer ") {
		t.Errorf("expected Bearer token type, got %s", authHeader)
	}
}

func TestAPIKeyAuthDefaultLocation(t *testing.T) {
	// Test that empty location defaults to header
	auth := NewAPIKeyAuth("X-Test", "value", "")

	req := httptest.NewRequest("GET", "/test", nil)
	auth.Authenticate(req)

	// Should use header by default
	if req.Header.Get("X-Test") != "value" {
		t.Errorf("expected header to be set, got %s", req.Header.Get("X-Test"))
	}
}

func TestAPIKeyAuthUnknownLocation(t *testing.T) {
	// Test unknown location falls back to header
	auth := &APIKeyAuth{
		key:      "X-Test",
		value:    "value",
		location: APIKeyLocation("unknown"),
	}

	req := httptest.NewRequest("GET", "/test", nil)
	auth.Authenticate(req)

	// Should fall back to header
	if req.Header.Get("X-Test") != "value" {
		t.Errorf("expected header fallback, got %s", req.Header.Get("X-Test"))
	}
}
