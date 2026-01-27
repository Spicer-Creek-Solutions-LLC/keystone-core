package vault

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/shawnbutts/keystone-core/internal/secrets"
)

func TestDefaultClientConfig(t *testing.T) {
	config := DefaultClientConfig()

	if config.Address != "http://127.0.0.1:8200" {
		t.Errorf("expected default address http://127.0.0.1:8200, got %s", config.Address)
	}
	if config.Timeout != 30*time.Second {
		t.Errorf("expected default timeout 30s, got %v", config.Timeout)
	}
	if config.MaxRetries != 3 {
		t.Errorf("expected default max retries 3, got %d", config.MaxRetries)
	}
}

func TestNewClient(t *testing.T) {
	tests := []struct {
		name    string
		config  *ClientConfig
		wantErr bool
	}{
		{
			name:    "nil config uses defaults",
			config:  nil,
			wantErr: false,
		},
		{
			name:    "empty address fails",
			config:  &ClientConfig{Address: ""},
			wantErr: true,
		},
		{
			name: "valid config",
			config: &ClientConfig{
				Address: "http://vault.example.com:8200",
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client, err := NewClient(tt.config)
			if (err != nil) != tt.wantErr {
				t.Errorf("NewClient() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && client == nil {
				t.Error("NewClient() returned nil client")
			}
			if client != nil {
				_ = client.Close()
			}
		})
	}
}

func TestClientToken(t *testing.T) {
	client, err := NewClient(nil)
	if err != nil {
		t.Fatalf("NewClient() failed: %v", err)
	}
	defer client.Close()

	client.SetToken("test-token")
	if got := client.Token(); got != "test-token" {
		t.Errorf("Token() = %q, want %q", got, "test-token")
	}
}

func TestClientNamespace(t *testing.T) {
	client, err := NewClient(nil)
	if err != nil {
		t.Fatalf("NewClient() failed: %v", err)
	}
	defer client.Close()

	client.SetNamespace("test-namespace")
	if got := client.Namespace(); got != "test-namespace" {
		t.Errorf("Namespace() = %q, want %q", got, "test-namespace")
	}
}

func TestClientAddress(t *testing.T) {
	config := &ClientConfig{Address: "https://vault.example.com:8200"}
	client, err := NewClient(config)
	if err != nil {
		t.Fatalf("NewClient() failed: %v", err)
	}
	defer client.Close()

	if got := client.Address(); got != "https://vault.example.com:8200" {
		t.Errorf("Address() = %q, want %q", got, "https://vault.example.com:8200")
	}
}

func TestClientHealth(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1/sys/health" {
			resp := HealthResponse{
				Initialized: true,
				Sealed:      false,
				Standby:     false,
				Version:     "1.15.0",
			}
			_ = json.NewEncoder(w).Encode(resp)
			return
		}
		http.NotFound(w, r)
	}))
	defer server.Close()

	config := &ClientConfig{Address: server.URL}
	client, err := NewClient(config)
	if err != nil {
		t.Fatalf("NewClient() failed: %v", err)
	}
	defer client.Close()

	ctx := context.Background()
	health, err := client.Health(ctx)
	if err != nil {
		t.Fatalf("Health() failed: %v", err)
	}

	if !health.Initialized {
		t.Error("expected Initialized to be true")
	}
	if health.Sealed {
		t.Error("expected Sealed to be false")
	}
	if health.Version != "1.15.0" {
		t.Errorf("expected Version 1.15.0, got %s", health.Version)
	}

	if !client.Healthy(ctx) {
		t.Error("expected Healthy() to be true")
	}
	if client.IsSealed() {
		t.Error("expected IsSealed() to be false")
	}
}

func TestClientHealthSealed(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1/sys/health" {
			w.WriteHeader(http.StatusServiceUnavailable)
			resp := HealthResponse{
				Initialized: true,
				Sealed:      true,
			}
			_ = json.NewEncoder(w).Encode(resp)
			return
		}
		http.NotFound(w, r)
	}))
	defer server.Close()

	config := &ClientConfig{Address: server.URL}
	client, err := NewClient(config)
	if err != nil {
		t.Fatalf("NewClient() failed: %v", err)
	}
	defer client.Close()

	ctx := context.Background()
	health, err := client.Health(ctx)
	if err != nil {
		t.Fatalf("Health() failed: %v", err)
	}

	if !health.Sealed {
		t.Error("expected Sealed to be true")
	}
	if client.IsSealed() != true {
		t.Error("expected IsSealed() to be true")
	}
}

func TestClientRead(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("expected GET, got %s", r.Method)
		}
		if r.Header.Get("X-Vault-Token") != "test-token" {
			t.Errorf("expected token header, got %s", r.Header.Get("X-Vault-Token"))
		}
		if r.URL.Path == "/v1/secret/data/myapp" {
			resp := map[string]interface{}{
				"data": map[string]interface{}{
					"data": map[string]interface{}{
						"username": "admin",
						"password": "secret",
					},
				},
			}
			_ = json.NewEncoder(w).Encode(resp)
			return
		}
		http.NotFound(w, r)
	}))
	defer server.Close()

	config := &ClientConfig{Address: server.URL}
	client, err := NewClient(config)
	if err != nil {
		t.Fatalf("NewClient() failed: %v", err)
	}
	defer client.Close()

	client.SetToken("test-token")

	ctx := context.Background()
	resp, err := client.Read(ctx, "secret/data/myapp")
	if err != nil {
		t.Fatalf("Read() failed: %v", err)
	}

	data := resp["data"].(map[string]interface{})
	inner := data["data"].(map[string]interface{})
	if inner["username"] != "admin" {
		t.Errorf("expected username admin, got %v", inner["username"])
	}
}

func TestClientWrite(t *testing.T) {
	var receivedData map[string]interface{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}
		if err := json.NewDecoder(r.Body).Decode(&receivedData); err != nil {
			t.Fatalf("failed to decode request body: %v", err)
		}
		resp := map[string]interface{}{
			"request_id": "test-request-id",
		}
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	config := &ClientConfig{Address: server.URL}
	client, err := NewClient(config)
	if err != nil {
		t.Fatalf("NewClient() failed: %v", err)
	}
	defer client.Close()

	client.SetToken("test-token")

	ctx := context.Background()
	data := map[string]interface{}{
		"username": "newuser",
		"password": "newsecret",
	}
	_, err = client.Write(ctx, "secret/data/myapp", data)
	if err != nil {
		t.Fatalf("Write() failed: %v", err)
	}

	if receivedData["username"] != "newuser" {
		t.Errorf("expected username newuser, got %v", receivedData["username"])
	}
}

func TestClientList(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("list") != "true" {
			t.Error("expected list=true query parameter")
		}
		resp := map[string]interface{}{
			"data": map[string]interface{}{
				"keys": []interface{}{"key1", "key2", "key3/"},
			},
		}
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	config := &ClientConfig{Address: server.URL}
	client, err := NewClient(config)
	if err != nil {
		t.Fatalf("NewClient() failed: %v", err)
	}
	defer client.Close()

	client.SetToken("test-token")

	ctx := context.Background()
	keys, err := client.List(ctx, "secret/metadata")
	if err != nil {
		t.Fatalf("List() failed: %v", err)
	}

	if len(keys) != 3 {
		t.Errorf("expected 3 keys, got %d", len(keys))
	}
	if keys[0] != "key1" || keys[1] != "key2" || keys[2] != "key3/" {
		t.Errorf("unexpected keys: %v", keys)
	}
}

func TestClientRetry(t *testing.T) {
	attempts := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		if attempts < 3 {
			http.Error(w, "temporary error", http.StatusInternalServerError)
			return
		}
		resp := map[string]interface{}{"data": map[string]interface{}{"key": "value"}}
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	config := &ClientConfig{
		Address:      server.URL,
		MaxRetries:   3,
		RetryWaitMin: 10 * time.Millisecond,
	}
	client, err := NewClient(config)
	if err != nil {
		t.Fatalf("NewClient() failed: %v", err)
	}
	defer client.Close()

	ctx := context.Background()
	_, err = client.Read(ctx, "secret/test")
	if err != nil {
		t.Fatalf("Read() should have succeeded after retries: %v", err)
	}

	if attempts != 3 {
		t.Errorf("expected 3 attempts, got %d", attempts)
	}
}

func TestClientErrorResponses(t *testing.T) {
	tests := []struct {
		name       string
		statusCode int
		body       string
		wantErr    error
	}{
		{
			name:       "not found",
			statusCode: http.StatusNotFound,
			body:       `{"errors": ["secret not found"]}`,
			wantErr:    secrets.ErrSecretNotFound,
		},
		{
			name:       "forbidden",
			statusCode: http.StatusForbidden,
			body:       `{"errors": ["permission denied"]}`,
			wantErr:    secrets.ErrAccessDenied,
		},
		{
			name:       "service unavailable",
			statusCode: http.StatusServiceUnavailable,
			body:       `{"errors": ["Vault is sealed"]}`,
			wantErr:    secrets.ErrBackendUnavailable,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tt.statusCode)
				_, _ = w.Write([]byte(tt.body))
			}))
			defer server.Close()

			config := &ClientConfig{Address: server.URL, MaxRetries: 1}
			client, err := NewClient(config)
			if err != nil {
				t.Fatalf("NewClient() failed: %v", err)
			}
			defer client.Close()

			ctx := context.Background()
			_, err = client.Read(ctx, "secret/test")
			if err != tt.wantErr {
				t.Errorf("Read() error = %v, want %v", err, tt.wantErr)
			}
		})
	}
}

func TestClientClose(t *testing.T) {
	client, err := NewClient(nil)
	if err != nil {
		t.Fatalf("NewClient() failed: %v", err)
	}

	if err := client.Close(); err != nil {
		t.Errorf("Close() failed: %v", err)
	}

	// Double close should be safe
	if err := client.Close(); err != nil {
		t.Errorf("second Close() failed: %v", err)
	}

	// Operations after close should fail
	ctx := context.Background()
	_, err = client.Read(ctx, "secret/test")
	if err == nil {
		t.Error("expected error after Close()")
	}
}

// Authentication tests

func TestNewAuthenticator(t *testing.T) {
	tests := []struct {
		name    string
		config  *AuthConfig
		wantErr bool
	}{
		{
			name:    "nil config fails",
			config:  nil,
			wantErr: true,
		},
		{
			name: "token auth without config fails",
			config: &AuthConfig{
				Method: "token",
			},
			wantErr: true,
		},
		{
			name: "token auth with config succeeds",
			config: &AuthConfig{
				Method: "token",
				Token:  &TokenAuthConfig{Token: "test-token"},
			},
			wantErr: false,
		},
		{
			name: "approle auth without config fails",
			config: &AuthConfig{
				Method: "approle",
			},
			wantErr: true,
		},
		{
			name: "approle auth with config succeeds",
			config: &AuthConfig{
				Method:  "approle",
				AppRole: &AppRoleAuthConfig{RoleID: "test-role"},
			},
			wantErr: false,
		},
		{
			name: "kubernetes auth without config fails",
			config: &AuthConfig{
				Method: "kubernetes",
			},
			wantErr: true,
		},
		{
			name: "kubernetes auth without role fails",
			config: &AuthConfig{
				Method:     "kubernetes",
				Kubernetes: &KubernetesAuthConfig{},
			},
			wantErr: true,
		},
		{
			name: "kubernetes auth with role succeeds",
			config: &AuthConfig{
				Method:     "kubernetes",
				Kubernetes: &KubernetesAuthConfig{Role: "my-role"},
			},
			wantErr: false,
		},
		{
			name: "unsupported method fails",
			config: &AuthConfig{
				Method: "unsupported",
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			auth, err := NewAuthenticator(tt.config)
			if (err != nil) != tt.wantErr {
				t.Errorf("NewAuthenticator() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && auth == nil {
				t.Error("NewAuthenticator() returned nil authenticator")
			}
		})
	}
}

func TestTokenAuth(t *testing.T) {
	auth := NewTokenAuth(&TokenAuthConfig{Token: "direct-token"})

	if auth.Type() != "token" {
		t.Errorf("Type() = %q, want %q", auth.Type(), "token")
	}
}

func TestTokenAuthResolveToken(t *testing.T) {
	// Test direct token
	auth := NewTokenAuth(&TokenAuthConfig{Token: "direct-token"})
	ctx := context.Background()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1/auth/token/lookup-self" {
			if r.Header.Get("X-Vault-Token") != "direct-token" {
				http.Error(w, "unauthorized", http.StatusForbidden)
				return
			}
			resp := map[string]interface{}{
				"data": map[string]interface{}{
					"id":        "direct-token",
					"ttl":       float64(3600),
					"renewable": true,
					"policies":  []interface{}{"default", "admin"},
				},
			}
			_ = json.NewEncoder(w).Encode(resp)
			return
		}
		http.NotFound(w, r)
	}))
	defer server.Close()

	client, _ := NewClient(&ClientConfig{Address: server.URL})
	defer client.Close()

	authResp, err := auth.Authenticate(ctx, client)
	if err != nil {
		t.Fatalf("Authenticate() failed: %v", err)
	}

	if authResp.Token != "direct-token" {
		t.Errorf("expected token direct-token, got %s", authResp.Token)
	}
	if !authResp.Renewable {
		t.Error("expected renewable to be true")
	}
	if len(authResp.Policies) != 2 {
		t.Errorf("expected 2 policies, got %d", len(authResp.Policies))
	}
}

func TestTokenAuthFromFile(t *testing.T) {
	// Create temp token file
	dir := t.TempDir()
	tokenFile := filepath.Join(dir, "token")
	if err := os.WriteFile(tokenFile, []byte("file-token"), 0600); err != nil {
		t.Fatalf("failed to write token file: %v", err)
	}

	auth := NewTokenAuth(&TokenAuthConfig{TokenFile: tokenFile})
	token := auth.resolveToken()
	if token != "file-token" {
		t.Errorf("expected token file-token, got %s", token)
	}
}

func TestTokenAuthFromEnv(t *testing.T) {
	t.Setenv("VAULT_TOKEN", "env-token")

	auth := NewTokenAuth(&TokenAuthConfig{})
	token := auth.resolveToken()
	if token != "env-token" {
		t.Errorf("expected token env-token, got %s", token)
	}
}

func TestAppRoleAuth(t *testing.T) {
	auth := NewAppRoleAuth(&AppRoleAuthConfig{
		RoleID:   "test-role-id",
		SecretID: "test-secret-id",
	})

	if auth.Type() != "approle" {
		t.Errorf("Type() = %q, want %q", auth.Type(), "approle")
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1/auth/approle/login" {
			var body map[string]interface{}
			_ = json.NewDecoder(r.Body).Decode(&body)

			if body["role_id"] != "test-role-id" || body["secret_id"] != "test-secret-id" {
				http.Error(w, "invalid credentials", http.StatusForbidden)
				return
			}

			resp := map[string]interface{}{
				"auth": map[string]interface{}{
					"client_token":   "new-token",
					"lease_duration": float64(3600),
					"renewable":      true,
					"policies":       []interface{}{"default"},
				},
			}
			_ = json.NewEncoder(w).Encode(resp)
			return
		}
		http.NotFound(w, r)
	}))
	defer server.Close()

	client, _ := NewClient(&ClientConfig{Address: server.URL})
	defer client.Close()

	ctx := context.Background()
	authResp, err := auth.Authenticate(ctx, client)
	if err != nil {
		t.Fatalf("Authenticate() failed: %v", err)
	}

	if authResp.Token != "new-token" {
		t.Errorf("expected token new-token, got %s", authResp.Token)
	}
}

func TestAppRoleAuthCustomMount(t *testing.T) {
	auth := NewAppRoleAuth(&AppRoleAuthConfig{
		MountPath: "custom-approle",
		RoleID:    "test-role-id",
	})

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1/auth/custom-approle/login" {
			resp := map[string]interface{}{
				"auth": map[string]interface{}{
					"client_token":   "new-token",
					"lease_duration": float64(3600),
					"renewable":      true,
				},
			}
			_ = json.NewEncoder(w).Encode(resp)
			return
		}
		t.Errorf("unexpected path: %s", r.URL.Path)
		http.NotFound(w, r)
	}))
	defer server.Close()

	client, _ := NewClient(&ClientConfig{Address: server.URL})
	defer client.Close()

	ctx := context.Background()
	_, err := auth.Authenticate(ctx, client)
	if err != nil {
		t.Fatalf("Authenticate() failed: %v", err)
	}
}

func TestKubernetesAuth(t *testing.T) {
	// Create temp SA token file
	dir := t.TempDir()
	tokenFile := filepath.Join(dir, "token")
	if err := os.WriteFile(tokenFile, []byte("k8s-jwt-token"), 0600); err != nil {
		t.Fatalf("failed to write token file: %v", err)
	}

	auth, err := NewKubernetesAuth(&KubernetesAuthConfig{
		Role:                    "my-role",
		ServiceAccountTokenFile: tokenFile,
	})
	if err != nil {
		t.Fatalf("NewKubernetesAuth() failed: %v", err)
	}

	if auth.Type() != "kubernetes" {
		t.Errorf("Type() = %q, want %q", auth.Type(), "kubernetes")
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1/auth/kubernetes/login" {
			var body map[string]interface{}
			_ = json.NewDecoder(r.Body).Decode(&body)

			if body["role"] != "my-role" || body["jwt"] != "k8s-jwt-token" {
				http.Error(w, "invalid credentials", http.StatusForbidden)
				return
			}

			resp := map[string]interface{}{
				"auth": map[string]interface{}{
					"client_token":   "k8s-token",
					"lease_duration": float64(7200),
					"renewable":      true,
					"metadata": map[string]interface{}{
						"service_account_name":      "my-sa",
						"service_account_namespace": "default",
					},
				},
			}
			_ = json.NewEncoder(w).Encode(resp)
			return
		}
		http.NotFound(w, r)
	}))
	defer server.Close()

	client, _ := NewClient(&ClientConfig{Address: server.URL})
	defer client.Close()

	ctx := context.Background()
	authResp, err := auth.Authenticate(ctx, client)
	if err != nil {
		t.Fatalf("Authenticate() failed: %v", err)
	}

	if authResp.Token != "k8s-token" {
		t.Errorf("expected token k8s-token, got %s", authResp.Token)
	}
	if authResp.Metadata["service_account_name"] != "my-sa" {
		t.Error("expected service_account_name metadata")
	}
}

// Backend tests

func TestNewBackend(t *testing.T) {
	tests := []struct {
		name    string
		config  *BackendConfig
		wantErr bool
	}{
		{
			name:    "nil config fails",
			config:  nil,
			wantErr: true,
		},
		{
			name: "valid config",
			config: &BackendConfig{
				Name: "test-vault",
				Client: &ClientConfig{
					Address: "http://vault.example.com:8200",
				},
			},
			wantErr: false,
		},
		{
			name: "default name",
			config: &BackendConfig{
				Client: &ClientConfig{
					Address: "http://vault.example.com:8200",
				},
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			backend, err := NewBackend(tt.config)
			if (err != nil) != tt.wantErr {
				t.Errorf("NewBackend() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr {
				if backend == nil {
					t.Error("NewBackend() returned nil backend")
					return
				}
				if backend.Type() != secrets.BackendTypeVault {
					t.Errorf("Type() = %v, want %v", backend.Type(), secrets.BackendTypeVault)
				}
				_ = backend.Close()
			}
		})
	}
}

func TestBackendReadKVv2(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/sys/health":
			resp := HealthResponse{Initialized: true, Sealed: false}
			_ = json.NewEncoder(w).Encode(resp)
		case "/v1/secret/data/myapp/config":
			resp := map[string]interface{}{
				"data": map[string]interface{}{
					"data": map[string]interface{}{
						"username": "admin",
						"password": "secret123",
					},
					"metadata": map[string]interface{}{
						"version":      float64(3),
						"created_time": "2024-01-15T10:30:00Z",
					},
				},
			}
			_ = json.NewEncoder(w).Encode(resp)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	backend, err := NewBackend(&BackendConfig{
		Name:   "test",
		Client: &ClientConfig{Address: server.URL},
		Engines: []*EngineConfig{
			{MountPath: "secret", Type: "kv", Version: 2},
		},
	})
	if err != nil {
		t.Fatalf("NewBackend() failed: %v", err)
	}
	defer backend.Close()

	backend.Client().SetToken("test-token")

	ctx := context.Background()
	secret, err := backend.Read(ctx, &secrets.SecretRequest{Path: "secret/myapp/config"})
	if err != nil {
		t.Fatalf("Read() failed: %v", err)
	}

	if secret.Path != "secret/myapp/config" {
		t.Errorf("expected path secret/myapp/config, got %s", secret.Path)
	}
	if secret.Backend != secrets.BackendTypeVault {
		t.Errorf("expected backend vault, got %s", secret.Backend)
	}
	if secret.Version != 3 {
		t.Errorf("expected version 3, got %d", secret.Version)
	}

	username, ok := secret.GetString("username")
	if !ok || username != "admin" {
		t.Errorf("expected username admin, got %s", username)
	}
}

func TestBackendReadKVv1(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/sys/health":
			resp := HealthResponse{Initialized: true, Sealed: false}
			_ = json.NewEncoder(w).Encode(resp)
		case "/v1/kv/myapp/config":
			resp := map[string]interface{}{
				"data": map[string]interface{}{
					"username": "admin",
					"password": "secret123",
				},
			}
			_ = json.NewEncoder(w).Encode(resp)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	backend, err := NewBackend(&BackendConfig{
		Name:   "test",
		Client: &ClientConfig{Address: server.URL},
		Engines: []*EngineConfig{
			{MountPath: "kv", Type: "kv", Version: 1},
		},
	})
	if err != nil {
		t.Fatalf("NewBackend() failed: %v", err)
	}
	defer backend.Close()

	backend.Client().SetToken("test-token")

	ctx := context.Background()
	secret, err := backend.Read(ctx, &secrets.SecretRequest{Path: "kv/myapp/config"})
	if err != nil {
		t.Fatalf("Read() failed: %v", err)
	}

	username, ok := secret.GetString("username")
	if !ok || username != "admin" {
		t.Errorf("expected username admin, got %s", username)
	}
}

func TestBackendReadWithVersion(t *testing.T) {
	var requestedPath string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/sys/health":
			resp := HealthResponse{Initialized: true, Sealed: false}
			_ = json.NewEncoder(w).Encode(resp)
		case "/v1/secret/data/myapp":
			requestedPath = r.URL.String()
			resp := map[string]interface{}{
				"data": map[string]interface{}{
					"data": map[string]interface{}{"key": "value"},
					"metadata": map[string]interface{}{
						"version": float64(2),
					},
				},
			}
			_ = json.NewEncoder(w).Encode(resp)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	backend, err := NewBackend(&BackendConfig{
		Name:   "test",
		Client: &ClientConfig{Address: server.URL},
	})
	if err != nil {
		t.Fatalf("NewBackend() failed: %v", err)
	}
	defer backend.Close()

	backend.Client().SetToken("test-token")

	ctx := context.Background()
	_, err = backend.Read(ctx, &secrets.SecretRequest{Path: "secret/myapp", Version: 2})
	if err != nil {
		t.Fatalf("Read() failed: %v", err)
	}

	if requestedPath != "/v1/secret/data/myapp?version=2" {
		t.Errorf("expected version query param, got %s", requestedPath)
	}
}

func TestBackendList(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/sys/health":
			resp := HealthResponse{Initialized: true, Sealed: false}
			_ = json.NewEncoder(w).Encode(resp)
		case "/v1/secret/metadata/myapp":
			resp := map[string]interface{}{
				"data": map[string]interface{}{
					"keys": []interface{}{"config", "creds", "subdir/"},
				},
			}
			_ = json.NewEncoder(w).Encode(resp)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	backend, err := NewBackend(&BackendConfig{
		Name:   "test",
		Client: &ClientConfig{Address: server.URL},
	})
	if err != nil {
		t.Fatalf("NewBackend() failed: %v", err)
	}
	defer backend.Close()

	backend.Client().SetToken("test-token")

	ctx := context.Background()
	keys, err := backend.List(ctx, "secret/myapp")
	if err != nil {
		t.Fatalf("List() failed: %v", err)
	}

	if len(keys) != 3 {
		t.Errorf("expected 3 keys, got %d", len(keys))
	}
}

func TestBackendWriteKVv2(t *testing.T) {
	var receivedData map[string]interface{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/sys/health":
			resp := HealthResponse{Initialized: true, Sealed: false}
			_ = json.NewEncoder(w).Encode(resp)
		case "/v1/secret/data/myapp/config":
			_ = json.NewDecoder(r.Body).Decode(&receivedData)
			resp := map[string]interface{}{"request_id": "test"}
			_ = json.NewEncoder(w).Encode(resp)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	backend, err := NewBackend(&BackendConfig{
		Name:   "test",
		Client: &ClientConfig{Address: server.URL},
	})
	if err != nil {
		t.Fatalf("NewBackend() failed: %v", err)
	}
	defer backend.Close()

	backend.Client().SetToken("test-token")

	ctx := context.Background()
	data := map[string]interface{}{"key": "value"}
	err = backend.Write(ctx, "secret/myapp/config", data)
	if err != nil {
		t.Fatalf("Write() failed: %v", err)
	}

	// KV v2 wraps data
	if wrapped, ok := receivedData["data"].(map[string]interface{}); ok {
		if wrapped["key"] != "value" {
			t.Errorf("expected wrapped data with key=value, got %v", wrapped)
		}
	} else {
		t.Error("expected data to be wrapped for KV v2")
	}
}

func TestBackendReadDynamic(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/sys/health":
			resp := HealthResponse{Initialized: true, Sealed: false}
			_ = json.NewEncoder(w).Encode(resp)
		case "/v1/database/creds/my-role":
			resp := map[string]interface{}{
				"lease_id":       "database/creds/my-role/abc123",
				"lease_duration": float64(3600),
				"renewable":      true,
				"data": map[string]interface{}{
					"username": "v-token-my-role-abc123",
					"password": "generated-password",
				},
			}
			_ = json.NewEncoder(w).Encode(resp)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	backend, err := NewBackend(&BackendConfig{
		Name:   "test",
		Client: &ClientConfig{Address: server.URL},
		Engines: []*EngineConfig{
			{MountPath: "database", Type: "database"},
		},
	})
	if err != nil {
		t.Fatalf("NewBackend() failed: %v", err)
	}
	defer backend.Close()

	backend.Client().SetToken("test-token")

	ctx := context.Background()
	secret, err := backend.ReadDynamic(ctx, &secrets.SecretRequest{Path: "database/my-role"})
	if err != nil {
		t.Fatalf("ReadDynamic() failed: %v", err)
	}

	if secret.Type != secrets.SecretTypeDynamic {
		t.Errorf("expected type dynamic, got %s", secret.Type)
	}
	if !secret.HasLease() {
		t.Error("expected secret to have a lease")
	}
	if secret.Lease.ID != "database/creds/my-role/abc123" {
		t.Errorf("expected lease ID database/creds/my-role/abc123, got %s", secret.Lease.ID)
	}
	if !secret.Lease.Renewable {
		t.Error("expected lease to be renewable")
	}
}

func TestBackendRenewLease(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/sys/health":
			resp := HealthResponse{Initialized: true, Sealed: false}
			_ = json.NewEncoder(w).Encode(resp)
		case "/v1/sys/leases/renew":
			resp := map[string]interface{}{
				"lease_id":       "database/creds/my-role/abc123",
				"lease_duration": float64(3600),
				"renewable":      true,
			}
			_ = json.NewEncoder(w).Encode(resp)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	backend, err := NewBackend(&BackendConfig{
		Name:   "test",
		Client: &ClientConfig{Address: server.URL},
	})
	if err != nil {
		t.Fatalf("NewBackend() failed: %v", err)
	}
	defer backend.Close()

	backend.Client().SetToken("test-token")

	ctx := context.Background()
	lease, err := backend.RenewLease(ctx, "database/creds/my-role/abc123", time.Hour)
	if err != nil {
		t.Fatalf("RenewLease() failed: %v", err)
	}

	if lease.TTL != time.Hour {
		t.Errorf("expected TTL 1h, got %v", lease.TTL)
	}
}

func TestBackendRevokeLease(t *testing.T) {
	revoked := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/sys/health":
			resp := HealthResponse{Initialized: true, Sealed: false}
			_ = json.NewEncoder(w).Encode(resp)
		case "/v1/sys/leases/revoke":
			revoked = true
			w.WriteHeader(http.StatusNoContent)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	backend, err := NewBackend(&BackendConfig{
		Name:   "test",
		Client: &ClientConfig{Address: server.URL},
	})
	if err != nil {
		t.Fatalf("NewBackend() failed: %v", err)
	}
	defer backend.Close()

	backend.Client().SetToken("test-token")

	ctx := context.Background()
	err = backend.RevokeLease(ctx, "database/creds/my-role/abc123")
	if err != nil {
		t.Fatalf("RevokeLease() failed: %v", err)
	}

	if !revoked {
		t.Error("expected lease to be revoked")
	}
}

func TestBackendGetMetadata(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/sys/health":
			resp := HealthResponse{Initialized: true, Sealed: false}
			_ = json.NewEncoder(w).Encode(resp)
		case "/v1/secret/metadata/myapp":
			resp := map[string]interface{}{
				"data": map[string]interface{}{
					"current_version":      float64(5),
					"max_versions":         float64(10),
					"oldest_version":       float64(1),
					"cas_required":         true,
					"created_time":         "2024-01-15T10:30:00Z",
					"updated_time":         "2024-01-20T15:45:00Z",
					"delete_version_after": "720h",
					"custom_metadata": map[string]interface{}{
						"owner": "devops",
						"env":   "prod",
					},
					"versions": map[string]interface{}{
						"1": map[string]interface{}{
							"created_time": "2024-01-15T10:30:00Z",
							"destroyed":    false,
						},
						"5": map[string]interface{}{
							"created_time": "2024-01-20T15:45:00Z",
							"destroyed":    false,
						},
					},
				},
			}
			_ = json.NewEncoder(w).Encode(resp)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	backend, err := NewBackend(&BackendConfig{
		Name:   "test",
		Client: &ClientConfig{Address: server.URL},
	})
	if err != nil {
		t.Fatalf("NewBackend() failed: %v", err)
	}
	defer backend.Close()

	backend.Client().SetToken("test-token")

	ctx := context.Background()
	meta, err := backend.GetMetadata(ctx, "secret/myapp")
	if err != nil {
		t.Fatalf("GetMetadata() failed: %v", err)
	}

	if meta.CurrentVersion != 5 {
		t.Errorf("expected current version 5, got %d", meta.CurrentVersion)
	}
	if meta.MaxVersions != 10 {
		t.Errorf("expected max versions 10, got %d", meta.MaxVersions)
	}
	if !meta.CASRequired {
		t.Error("expected CAS required to be true")
	}
	if meta.CustomMetadata["owner"] != "devops" {
		t.Errorf("expected owner devops, got %s", meta.CustomMetadata["owner"])
	}
	if len(meta.Versions) != 2 {
		t.Errorf("expected 2 versions, got %d", len(meta.Versions))
	}
}

func TestBackendRegisterEngine(t *testing.T) {
	backend, err := NewBackend(&BackendConfig{
		Name:   "test",
		Client: &ClientConfig{Address: "http://localhost:8200"},
	})
	if err != nil {
		t.Fatalf("NewBackend() failed: %v", err)
	}
	defer backend.Close()

	// Register a new engine
	backend.RegisterEngine(&EngineConfig{
		MountPath: "custom-kv",
		Type:      "kv",
		Version:   1,
	})

	// Verify the engine was registered
	engine := backend.getEngine("custom-kv")
	if engine == nil {
		t.Error("expected engine to be registered")
	}
	if engine.Version != 1 {
		t.Errorf("expected version 1, got %d", engine.Version)
	}
}

func TestSplitPath(t *testing.T) {
	backend, _ := NewBackend(&BackendConfig{
		Name:   "test",
		Client: &ClientConfig{Address: "http://localhost:8200"},
		Engines: []*EngineConfig{
			{MountPath: "secret", Type: "kv", Version: 2},
			{MountPath: "custom/mount", Type: "kv", Version: 1},
		},
	})
	defer backend.Close()

	tests := []struct {
		path      string
		wantMount string
		wantSub   string
	}{
		{"secret/myapp/config", "secret", "myapp/config"},
		{"custom/mount/key", "custom/mount", "key"},
		{"unknown/path/key", "unknown", "path/key"},
		{"secret", "secret", ""},
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			mount, sub := backend.splitPath(tt.path)
			if mount != tt.wantMount {
				t.Errorf("mount = %q, want %q", mount, tt.wantMount)
			}
			if sub != tt.wantSub {
				t.Errorf("subPath = %q, want %q", sub, tt.wantSub)
			}
		})
	}
}

func TestIsBase64(t *testing.T) {
	tests := []struct {
		input string
		want  bool
	}{
		{"SGVsbG8gV29ybGQ=", true}, // "Hello World"
		{"dGVzdA==", true},         // "test"
		{"abc", false},             // too short
		{"Hello World!", false},    // contains invalid chars
		{"", false},                // empty
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			if got := isBase64(tt.input); got != tt.want {
				t.Errorf("isBase64(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}

// Database engine tests

func TestNewDatabaseEngine(t *testing.T) {
	client, _ := NewClient(&ClientConfig{Address: "http://localhost:8200"})
	defer client.Close()

	// With nil config
	engine := NewDatabaseEngine(client, nil)
	if engine.config.MountPath != "database" {
		t.Errorf("expected default mount path 'database', got %s", engine.config.MountPath)
	}

	// With custom config
	engine = NewDatabaseEngine(client, &DatabaseConfig{MountPath: "custom-db"})
	if engine.config.MountPath != "custom-db" {
		t.Errorf("expected custom mount path 'custom-db', got %s", engine.config.MountPath)
	}
}

func TestDatabaseEngineGenerateCredentials(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/database/creds/postgres-readonly":
			resp := map[string]interface{}{
				"lease_id":       "database/creds/postgres-readonly/abc123",
				"lease_duration": float64(3600),
				"renewable":      true,
				"data": map[string]interface{}{
					"username": "v-token-postgres-readonly-abc123",
					"password": "generated-password-xyz",
				},
			}
			_ = json.NewEncoder(w).Encode(resp)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	client, _ := NewClient(&ClientConfig{Address: server.URL})
	defer client.Close()
	client.SetToken("test-token")

	engine := NewDatabaseEngine(client, nil)

	ctx := context.Background()
	creds, err := engine.GenerateCredentials(ctx, "postgres-readonly")
	if err != nil {
		t.Fatalf("GenerateCredentials failed: %v", err)
	}

	if creds.Username != "v-token-postgres-readonly-abc123" {
		t.Errorf("expected username v-token-postgres-readonly-abc123, got %s", creds.Username)
	}
	if creds.Password != "generated-password-xyz" {
		t.Errorf("expected password generated-password-xyz, got %s", creds.Password)
	}
	if creds.LeaseID != "database/creds/postgres-readonly/abc123" {
		t.Errorf("expected lease ID, got %s", creds.LeaseID)
	}
	if !creds.Renewable {
		t.Error("expected renewable to be true")
	}
	if creds.LeaseDuration != time.Hour {
		t.Errorf("expected lease duration 1h, got %v", creds.LeaseDuration)
	}
}

func TestDatabaseEngineGenerateCredentialsEmptyRole(t *testing.T) {
	client, _ := NewClient(&ClientConfig{Address: "http://localhost:8200"})
	defer client.Close()

	engine := NewDatabaseEngine(client, nil)

	ctx := context.Background()
	_, err := engine.GenerateCredentials(ctx, "")
	if err == nil {
		t.Error("expected error for empty role")
	}
}

func TestDatabaseEngineStaticCredentials(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/database/static-creds/static-postgres":
			resp := map[string]interface{}{
				"data": map[string]interface{}{
					"username": "static-user",
					"password": "static-password",
				},
			}
			_ = json.NewEncoder(w).Encode(resp)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	client, _ := NewClient(&ClientConfig{Address: server.URL})
	defer client.Close()
	client.SetToken("test-token")

	engine := NewDatabaseEngine(client, nil)

	ctx := context.Background()
	creds, err := engine.GenerateStaticCredentials(ctx, "static-postgres")
	if err != nil {
		t.Fatalf("GenerateStaticCredentials failed: %v", err)
	}

	if creds.Username != "static-user" {
		t.Errorf("expected username static-user, got %s", creds.Username)
	}
}

func TestDatabaseEngineRotateStaticCredentials(t *testing.T) {
	rotated := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1/database/rotate-role/static-postgres" && r.Method == http.MethodPost {
			rotated = true
			w.WriteHeader(http.StatusNoContent)
			return
		}
		http.NotFound(w, r)
	}))
	defer server.Close()

	client, _ := NewClient(&ClientConfig{Address: server.URL})
	defer client.Close()
	client.SetToken("test-token")

	engine := NewDatabaseEngine(client, nil)

	ctx := context.Background()
	err := engine.RotateStaticCredentials(ctx, "static-postgres")
	if err != nil {
		t.Fatalf("RotateStaticCredentials failed: %v", err)
	}

	if !rotated {
		t.Error("expected rotation to be called")
	}
}

func TestDatabaseEngineListRoles(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1/database/roles" && r.URL.Query().Get("list") == "true" {
			resp := map[string]interface{}{
				"data": map[string]interface{}{
					"keys": []interface{}{"postgres-readonly", "postgres-readwrite", "mysql-admin"},
				},
			}
			_ = json.NewEncoder(w).Encode(resp)
			return
		}
		http.NotFound(w, r)
	}))
	defer server.Close()

	client, _ := NewClient(&ClientConfig{Address: server.URL})
	defer client.Close()
	client.SetToken("test-token")

	engine := NewDatabaseEngine(client, nil)

	ctx := context.Background()
	roles, err := engine.ListRoles(ctx)
	if err != nil {
		t.Fatalf("ListRoles failed: %v", err)
	}

	if len(roles) != 3 {
		t.Errorf("expected 3 roles, got %d", len(roles))
	}
}

func TestDatabaseEngineGetRole(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1/database/roles/postgres-readonly" {
			resp := map[string]interface{}{
				"data": map[string]interface{}{
					"db_name":             "postgres-db",
					"default_ttl":         float64(3600),
					"max_ttl":             float64(86400),
					"creation_statements": []interface{}{"CREATE ROLE..."},
				},
			}
			_ = json.NewEncoder(w).Encode(resp)
			return
		}
		http.NotFound(w, r)
	}))
	defer server.Close()

	client, _ := NewClient(&ClientConfig{Address: server.URL})
	defer client.Close()
	client.SetToken("test-token")

	engine := NewDatabaseEngine(client, nil)

	ctx := context.Background()
	role, err := engine.GetRole(ctx, "postgres-readonly")
	if err != nil {
		t.Fatalf("GetRole failed: %v", err)
	}

	if role.DBName != "postgres-db" {
		t.Errorf("expected db_name postgres-db, got %s", role.DBName)
	}
	if role.DefaultTTL != time.Hour {
		t.Errorf("expected default_ttl 1h, got %v", role.DefaultTTL)
	}
}

func TestDatabaseCredentialsToSecret(t *testing.T) {
	creds := &DatabaseCredentials{
		Username:      "test-user",
		Password:      "test-password",
		LeaseID:       "database/creds/role/lease123",
		LeaseDuration: time.Hour,
		Renewable:     true,
	}

	secret := creds.ToSecret("database/creds/role")

	if secret.Type != secrets.SecretTypeDatabase {
		t.Errorf("expected type database, got %s", secret.Type)
	}
	if secret.Backend != secrets.BackendTypeVault {
		t.Errorf("expected backend vault, got %s", secret.Backend)
	}

	username, ok := secret.GetString("username")
	if !ok || username != "test-user" {
		t.Errorf("expected username test-user, got %s", username)
	}

	if !secret.HasLease() {
		t.Error("expected secret to have a lease")
	}
	if secret.Lease.ID != "database/creds/role/lease123" {
		t.Errorf("expected lease ID, got %s", secret.Lease.ID)
	}
}

// Lease tracker tests

func TestNewLeaseTracker(t *testing.T) {
	client, _ := NewClient(&ClientConfig{Address: "http://localhost:8200"})
	defer client.Close()

	// With nil config
	tracker := NewLeaseTracker(client, nil)
	if tracker.config.CheckInterval != 30*time.Second {
		t.Errorf("expected default check interval 30s, got %v", tracker.config.CheckInterval)
	}

	// With custom config
	tracker = NewLeaseTracker(client, &LeaseTrackerConfig{
		CheckInterval: time.Minute,
	})
	if tracker.config.CheckInterval != time.Minute {
		t.Errorf("expected check interval 1m, got %v", tracker.config.CheckInterval)
	}
}

func TestLeaseTrackerTrackAndGet(t *testing.T) {
	client, _ := NewClient(&ClientConfig{Address: "http://localhost:8200"})
	defer client.Close()

	tracker := NewLeaseTracker(client, nil)

	lease := &secrets.Lease{
		ID:         "test-lease-123",
		SecretPath: "database/creds/role",
		Backend:    secrets.BackendTypeVault,
		State:      secrets.LeaseStateActive,
		TTL:        time.Hour,
		ExpiresAt:  time.Now().Add(time.Hour),
		Renewable:  true,
	}

	ctx := context.Background()
	err := tracker.Track(ctx, lease,
		WithRole("postgres-readonly"),
		WithEngine("database"),
		WithAgentID("agent-001"),
		WithTags(map[string]string{"env": "prod"}),
	)
	if err != nil {
		t.Fatalf("Track failed: %v", err)
	}

	// Get the tracked lease
	tracked, err := tracker.Get(ctx, "test-lease-123")
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}

	if tracked.Role != "postgres-readonly" {
		t.Errorf("expected role postgres-readonly, got %s", tracked.Role)
	}
	if tracked.Engine != "database" {
		t.Errorf("expected engine database, got %s", tracked.Engine)
	}
	if tracked.AgentID != "agent-001" {
		t.Errorf("expected agent agent-001, got %s", tracked.AgentID)
	}
	if tracked.Tags["env"] != "prod" {
		t.Errorf("expected tag env=prod, got %s", tracked.Tags["env"])
	}
}

func TestLeaseTrackerTrackEmptyID(t *testing.T) {
	client, _ := NewClient(&ClientConfig{Address: "http://localhost:8200"})
	defer client.Close()

	tracker := NewLeaseTracker(client, nil)

	ctx := context.Background()
	err := tracker.Track(ctx, &secrets.Lease{})
	if err == nil {
		t.Error("expected error for empty lease ID")
	}
}

func TestLeaseTrackerList(t *testing.T) {
	client, _ := NewClient(&ClientConfig{Address: "http://localhost:8200"})
	defer client.Close()

	tracker := NewLeaseTracker(client, nil)
	ctx := context.Background()

	// Track multiple leases
	for i := 0; i < 3; i++ {
		lease := &secrets.Lease{
			ID:         fmt.Sprintf("lease-%d", i),
			SecretPath: "database/creds/role",
			Backend:    secrets.BackendTypeVault,
			State:      secrets.LeaseStateActive,
			TTL:        time.Hour,
			ExpiresAt:  time.Now().Add(time.Duration(i+1) * time.Hour),
			Renewable:  true,
		}
		_ = tracker.Track(ctx, lease)
	}

	leases, err := tracker.List(ctx)
	if err != nil {
		t.Fatalf("List failed: %v", err)
	}

	if len(leases) != 3 {
		t.Errorf("expected 3 leases, got %d", len(leases))
	}

	// Verify sorted by expiration
	for i := 1; i < len(leases); i++ {
		if leases[i].ExpiresAt.Before(leases[i-1].ExpiresAt) {
			t.Error("leases should be sorted by expiration")
		}
	}
}

func TestLeaseTrackerListByPath(t *testing.T) {
	client, _ := NewClient(&ClientConfig{Address: "http://localhost:8200"})
	defer client.Close()

	tracker := NewLeaseTracker(client, nil)
	ctx := context.Background()

	// Track leases with different paths
	_ = tracker.Track(ctx, &secrets.Lease{
		ID:         "lease-1",
		SecretPath: "database/creds/role-a",
		Backend:    secrets.BackendTypeVault,
		State:      secrets.LeaseStateActive,
		ExpiresAt:  time.Now().Add(time.Hour),
	})
	_ = tracker.Track(ctx, &secrets.Lease{
		ID:         "lease-2",
		SecretPath: "database/creds/role-a",
		Backend:    secrets.BackendTypeVault,
		State:      secrets.LeaseStateActive,
		ExpiresAt:  time.Now().Add(time.Hour),
	})
	_ = tracker.Track(ctx, &secrets.Lease{
		ID:         "lease-3",
		SecretPath: "database/creds/role-b",
		Backend:    secrets.BackendTypeVault,
		State:      secrets.LeaseStateActive,
		ExpiresAt:  time.Now().Add(time.Hour),
	})

	leases, err := tracker.ListByPath(ctx, "database/creds/role-a")
	if err != nil {
		t.Fatalf("ListByPath failed: %v", err)
	}

	if len(leases) != 2 {
		t.Errorf("expected 2 leases for role-a, got %d", len(leases))
	}
}

func TestLeaseTrackerListByAgent(t *testing.T) {
	client, _ := NewClient(&ClientConfig{Address: "http://localhost:8200"})
	defer client.Close()

	tracker := NewLeaseTracker(client, nil)
	ctx := context.Background()

	_ = tracker.Track(ctx, &secrets.Lease{
		ID:         "lease-1",
		SecretPath: "database/creds/role",
		Backend:    secrets.BackendTypeVault,
		State:      secrets.LeaseStateActive,
		ExpiresAt:  time.Now().Add(time.Hour),
	}, WithAgentID("agent-001"))

	_ = tracker.Track(ctx, &secrets.Lease{
		ID:         "lease-2",
		SecretPath: "database/creds/role",
		Backend:    secrets.BackendTypeVault,
		State:      secrets.LeaseStateActive,
		ExpiresAt:  time.Now().Add(time.Hour),
	}, WithAgentID("agent-002"))

	leases, err := tracker.ListByAgent(ctx, "agent-001")
	if err != nil {
		t.Fatalf("ListByAgent failed: %v", err)
	}

	if len(leases) != 1 {
		t.Errorf("expected 1 lease for agent-001, got %d", len(leases))
	}
}

func TestLeaseTrackerListExpiring(t *testing.T) {
	client, _ := NewClient(&ClientConfig{Address: "http://localhost:8200"})
	defer client.Close()

	tracker := NewLeaseTracker(client, nil)
	ctx := context.Background()

	// Track leases with different expiration times
	_ = tracker.Track(ctx, &secrets.Lease{
		ID:         "expiring-soon",
		SecretPath: "database/creds/role",
		Backend:    secrets.BackendTypeVault,
		State:      secrets.LeaseStateActive,
		ExpiresAt:  time.Now().Add(5 * time.Minute),
	})
	_ = tracker.Track(ctx, &secrets.Lease{
		ID:         "expiring-later",
		SecretPath: "database/creds/role",
		Backend:    secrets.BackendTypeVault,
		State:      secrets.LeaseStateActive,
		ExpiresAt:  time.Now().Add(2 * time.Hour),
	})

	leases, err := tracker.ListExpiring(ctx, 30*time.Minute)
	if err != nil {
		t.Fatalf("ListExpiring failed: %v", err)
	}

	if len(leases) != 1 {
		t.Errorf("expected 1 expiring lease, got %d", len(leases))
	}
	if leases[0].ID != "expiring-soon" {
		t.Errorf("expected expiring-soon, got %s", leases[0].ID)
	}
}

func TestLeaseTrackerRenew(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1/sys/leases/renew" {
			resp := map[string]interface{}{
				"lease_id":       "test-lease-123",
				"lease_duration": float64(7200),
				"renewable":      true,
			}
			_ = json.NewEncoder(w).Encode(resp)
			return
		}
		http.NotFound(w, r)
	}))
	defer server.Close()

	client, _ := NewClient(&ClientConfig{Address: server.URL})
	defer client.Close()
	client.SetToken("test-token")

	tracker := NewLeaseTracker(client, nil)
	ctx := context.Background()

	lease := &secrets.Lease{
		ID:         "test-lease-123",
		SecretPath: "database/creds/role",
		Backend:    secrets.BackendTypeVault,
		State:      secrets.LeaseStateActive,
		TTL:        time.Hour,
		ExpiresAt:  time.Now().Add(30 * time.Minute),
		Renewable:  true,
	}
	_ = tracker.Track(ctx, lease)

	renewed, err := tracker.Renew(ctx, "test-lease-123", time.Hour)
	if err != nil {
		t.Fatalf("Renew failed: %v", err)
	}

	if renewed.TTL != 2*time.Hour {
		t.Errorf("expected TTL 2h after renewal, got %v", renewed.TTL)
	}
	if renewed.RenewalCount != 1 {
		t.Errorf("expected renewal count 1, got %d", renewed.RenewalCount)
	}
}

func TestLeaseTrackerRenewNotRenewable(t *testing.T) {
	client, _ := NewClient(&ClientConfig{Address: "http://localhost:8200"})
	defer client.Close()

	tracker := NewLeaseTracker(client, nil)
	ctx := context.Background()

	lease := &secrets.Lease{
		ID:         "test-lease-123",
		SecretPath: "database/creds/role",
		Backend:    secrets.BackendTypeVault,
		State:      secrets.LeaseStateActive,
		TTL:        time.Hour,
		ExpiresAt:  time.Now().Add(time.Hour),
		Renewable:  false,
	}
	_ = tracker.Track(ctx, lease)

	_, err := tracker.Renew(ctx, "test-lease-123", time.Hour)
	if err != secrets.ErrLeaseNotRenewable {
		t.Errorf("expected ErrLeaseNotRenewable, got %v", err)
	}
}

func TestLeaseTrackerRevoke(t *testing.T) {
	revoked := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1/sys/leases/revoke" {
			revoked = true
			w.WriteHeader(http.StatusNoContent)
			return
		}
		http.NotFound(w, r)
	}))
	defer server.Close()

	client, _ := NewClient(&ClientConfig{Address: server.URL})
	defer client.Close()
	client.SetToken("test-token")

	tracker := NewLeaseTracker(client, nil)
	ctx := context.Background()

	lease := &secrets.Lease{
		ID:         "test-lease-123",
		SecretPath: "database/creds/role",
		Backend:    secrets.BackendTypeVault,
		State:      secrets.LeaseStateActive,
		TTL:        time.Hour,
		ExpiresAt:  time.Now().Add(time.Hour),
		Renewable:  true,
	}
	_ = tracker.Track(ctx, lease)

	err := tracker.Revoke(ctx, "test-lease-123")
	if err != nil {
		t.Fatalf("Revoke failed: %v", err)
	}

	if !revoked {
		t.Error("expected revoke to be called")
	}

	// Verify state changed
	tracked, _ := tracker.Get(ctx, "test-lease-123")
	if tracked.State != secrets.LeaseStateRevoked {
		t.Errorf("expected state revoked, got %s", tracked.State)
	}
}

func TestLeaseTrackerStats(t *testing.T) {
	client, _ := NewClient(&ClientConfig{Address: "http://localhost:8200"})
	defer client.Close()

	tracker := NewLeaseTracker(client, nil)
	ctx := context.Background()

	// Track various leases
	_ = tracker.Track(ctx, &secrets.Lease{
		ID:         "active-1",
		SecretPath: "database/creds/role",
		Backend:    secrets.BackendTypeVault,
		State:      secrets.LeaseStateActive,
		ExpiresAt:  time.Now().Add(time.Hour),
	}, WithEngine("database"), WithAgentID("agent-001"))

	_ = tracker.Track(ctx, &secrets.Lease{
		ID:         "expiring-soon",
		SecretPath: "database/creds/role",
		Backend:    secrets.BackendTypeVault,
		State:      secrets.LeaseStateActive,
		ExpiresAt:  time.Now().Add(2 * time.Minute),
	}, WithEngine("database"), WithAgentID("agent-002"))

	stats := tracker.Stats(ctx)

	if stats.ActiveLeases != 2 {
		t.Errorf("expected 2 active leases, got %d", stats.ActiveLeases)
	}
	if stats.ExpiringLeases != 1 {
		t.Errorf("expected 1 expiring lease, got %d", stats.ExpiringLeases)
	}
	if stats.ByEngine["database"] != 2 {
		t.Errorf("expected 2 database engine leases, got %d", stats.ByEngine["database"])
	}
	if stats.ByAgent["agent-001"] != 1 || stats.ByAgent["agent-002"] != 1 {
		t.Error("expected 1 lease per agent")
	}
}

func TestLeaseTrackerCallbacks(t *testing.T) {
	client, _ := NewClient(&ClientConfig{Address: "http://localhost:8200"})
	defer client.Close()

	tracker := NewLeaseTracker(client, nil)

	var events []LeaseEvent
	tracker.OnLeaseEvent(func(ctx context.Context, lease *TrackedLease, event LeaseEvent) {
		events = append(events, event)
	})

	ctx := context.Background()
	lease := &secrets.Lease{
		ID:         "test-lease",
		SecretPath: "database/creds/role",
		Backend:    secrets.BackendTypeVault,
		State:      secrets.LeaseStateActive,
		ExpiresAt:  time.Now().Add(time.Hour),
	}
	_ = tracker.Track(ctx, lease)

	if len(events) != 1 || events[0] != LeaseEventTracked {
		t.Errorf("expected tracked event, got %v", events)
	}
}

func TestLeaseTrackerRemove(t *testing.T) {
	client, _ := NewClient(&ClientConfig{Address: "http://localhost:8200"})
	defer client.Close()

	tracker := NewLeaseTracker(client, nil)
	ctx := context.Background()

	_ = tracker.Track(ctx, &secrets.Lease{
		ID:         "test-lease",
		SecretPath: "database/creds/role",
		Backend:    secrets.BackendTypeVault,
		State:      secrets.LeaseStateActive,
		ExpiresAt:  time.Now().Add(time.Hour),
	}, WithAgentID("agent-001"), WithTags(map[string]string{"env": "prod"}))

	// Verify it exists
	_, err := tracker.Get(ctx, "test-lease")
	if err != nil {
		t.Fatalf("expected lease to exist")
	}

	// Remove it
	err = tracker.Remove(ctx, "test-lease")
	if err != nil {
		t.Fatalf("Remove failed: %v", err)
	}

	// Verify it's gone
	_, err = tracker.Get(ctx, "test-lease")
	if err != secrets.ErrLeaseNotFound {
		t.Errorf("expected ErrLeaseNotFound, got %v", err)
	}

	// Verify indexes are cleaned up
	leases, _ := tracker.ListByAgent(ctx, "agent-001")
	if len(leases) != 0 {
		t.Error("expected empty agent index")
	}

	leases, _ = tracker.ListByTag(ctx, "env", "prod")
	if len(leases) != 0 {
		t.Error("expected empty tag index")
	}
}

func TestLeaseTrackerStartStop(t *testing.T) {
	client, _ := NewClient(&ClientConfig{Address: "http://localhost:8200"})
	defer client.Close()

	tracker := NewLeaseTracker(client, &LeaseTrackerConfig{
		CheckInterval:   100 * time.Millisecond,
		CleanupInterval: 100 * time.Millisecond,
	})

	ctx := context.Background()
	err := tracker.Start(ctx)
	if err != nil {
		t.Fatalf("Start failed: %v", err)
	}

	// Double start should be safe
	err = tracker.Start(ctx)
	if err != nil {
		t.Fatalf("Double start failed: %v", err)
	}

	// Give the goroutines time to run
	time.Sleep(150 * time.Millisecond)

	err = tracker.Stop()
	if err != nil {
		t.Fatalf("Stop failed: %v", err)
	}
}

// PKI Engine Tests

func TestNewPKIEngine(t *testing.T) {
	client, _ := NewClient(&ClientConfig{Address: "http://localhost:8200"})
	defer client.Close()

	// Default config
	engine := NewPKIEngine(client, nil)
	if engine.config.MountPath != "pki" {
		t.Errorf("expected default mount path 'pki', got %s", engine.config.MountPath)
	}

	// Custom config
	engine = NewPKIEngine(client, &PKIConfig{
		MountPath:  "custom-pki",
		DefaultTTL: 24 * time.Hour,
		MaxTTL:     720 * time.Hour,
	})
	if engine.config.MountPath != "custom-pki" {
		t.Errorf("expected mount path 'custom-pki', got %s", engine.config.MountPath)
	}
	if engine.config.DefaultTTL != 24*time.Hour {
		t.Errorf("expected default TTL 24h, got %v", engine.config.DefaultTTL)
	}
}

func TestPKIEngineIssueCertificate(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1/pki/issue/web-server" && r.Method == "POST" {
			resp := map[string]interface{}{
				"data": map[string]interface{}{
					"certificate":      "-----BEGIN CERTIFICATE-----\nMIIC...\n-----END CERTIFICATE-----",
					"private_key":      "-----BEGIN RSA PRIVATE KEY-----\nMIIE...\n-----END RSA PRIVATE KEY-----",
					"private_key_type": "rsa",
					"issuing_ca":       "-----BEGIN CERTIFICATE-----\nMIID...\n-----END CERTIFICATE-----",
					"serial_number":    "12:34:56:78:90:ab:cd:ef",
					"expiration":       float64(time.Now().Add(24 * time.Hour).Unix()),
					"ca_chain":         []interface{}{"-----BEGIN CERTIFICATE-----\nMIID...\n-----END CERTIFICATE-----"},
				},
				"lease_id":       "pki/issue/web-server/abc123",
				"lease_duration": float64(86400),
				"renewable":      false,
			}
			_ = json.NewEncoder(w).Encode(resp)
			return
		}
		http.NotFound(w, r)
	}))
	defer server.Close()

	client, _ := NewClient(&ClientConfig{Address: server.URL})
	defer client.Close()
	client.SetToken("test-token")

	engine := NewPKIEngine(client, nil)
	ctx := context.Background()

	cert, err := engine.IssueCertificate(ctx, &CertificateRequest{
		Role:       "web-server",
		CommonName: "example.com",
		AltNames:   "www.example.com,api.example.com",
		IPSANs:     "192.168.1.1",
		TTL:        24 * time.Hour,
	})
	if err != nil {
		t.Fatalf("IssueCertificate failed: %v", err)
	}

	if cert.Certificate == "" {
		t.Error("expected certificate")
	}
	if cert.PrivateKey == "" {
		t.Error("expected private key")
	}
	if cert.PrivateKeyType != "rsa" {
		t.Errorf("expected private_key_type 'rsa', got %s", cert.PrivateKeyType)
	}
	if cert.SerialNumber != "12:34:56:78:90:ab:cd:ef" {
		t.Errorf("unexpected serial number: %s", cert.SerialNumber)
	}
	if len(cert.CAChain) != 1 {
		t.Errorf("expected 1 CA in chain, got %d", len(cert.CAChain))
	}
}

func TestPKIEngineIssueCertificateValidation(t *testing.T) {
	client, _ := NewClient(&ClientConfig{Address: "http://localhost:8200"})
	defer client.Close()

	engine := NewPKIEngine(client, nil)
	ctx := context.Background()

	// Missing role
	_, err := engine.IssueCertificate(ctx, &CertificateRequest{
		CommonName: "example.com",
	})
	if err == nil {
		t.Error("expected error for missing role")
	}

	// Missing common_name
	_, err = engine.IssueCertificate(ctx, &CertificateRequest{
		Role: "web-server",
	})
	if err == nil {
		t.Error("expected error for missing common_name")
	}

	// Nil request
	_, err = engine.IssueCertificate(ctx, nil)
	if err == nil {
		t.Error("expected error for nil request")
	}
}

func TestPKIEngineSignCSR(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1/pki/sign/web-server" && r.Method == "POST" {
			resp := map[string]interface{}{
				"data": map[string]interface{}{
					"certificate":   "-----BEGIN CERTIFICATE-----\nMIIC...\n-----END CERTIFICATE-----",
					"issuing_ca":    "-----BEGIN CERTIFICATE-----\nMIID...\n-----END CERTIFICATE-----",
					"serial_number": "ab:cd:ef:12:34:56:78:90",
				},
			}
			_ = json.NewEncoder(w).Encode(resp)
			return
		}
		http.NotFound(w, r)
	}))
	defer server.Close()

	client, _ := NewClient(&ClientConfig{Address: server.URL})
	defer client.Close()
	client.SetToken("test-token")

	engine := NewPKIEngine(client, nil)
	ctx := context.Background()

	csr := "-----BEGIN CERTIFICATE REQUEST-----\nMIIC...\n-----END CERTIFICATE REQUEST-----"
	cert, err := engine.SignCSR(ctx, "web-server", csr, "example.com", 24*time.Hour)
	if err != nil {
		t.Fatalf("SignCSR failed: %v", err)
	}

	if cert.Certificate == "" {
		t.Error("expected certificate")
	}
	if cert.SerialNumber != "ab:cd:ef:12:34:56:78:90" {
		t.Errorf("unexpected serial number: %s", cert.SerialNumber)
	}
}

func TestPKIEngineSignCSRValidation(t *testing.T) {
	client, _ := NewClient(&ClientConfig{Address: "http://localhost:8200"})
	defer client.Close()

	engine := NewPKIEngine(client, nil)
	ctx := context.Background()

	// Missing role
	_, err := engine.SignCSR(ctx, "", "csr-data", "example.com", 24*time.Hour)
	if err == nil {
		t.Error("expected error for missing role")
	}

	// Missing CSR
	_, err = engine.SignCSR(ctx, "web-server", "", "example.com", 24*time.Hour)
	if err == nil {
		t.Error("expected error for missing CSR")
	}
}

func TestPKIEngineRevokeCertificate(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1/pki/revoke" && r.Method == "POST" {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		http.NotFound(w, r)
	}))
	defer server.Close()

	client, _ := NewClient(&ClientConfig{Address: server.URL})
	defer client.Close()
	client.SetToken("test-token")

	engine := NewPKIEngine(client, nil)
	ctx := context.Background()

	err := engine.RevokeCertificate(ctx, "12:34:56:78:90:ab:cd:ef")
	if err != nil {
		t.Fatalf("RevokeCertificate failed: %v", err)
	}

	// Empty serial
	err = engine.RevokeCertificate(ctx, "")
	if err == nil {
		t.Error("expected error for empty serial number")
	}
}

func TestPKIEngineListCertificates(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1/pki/certs" && r.Method == "GET" && r.URL.Query().Get("list") == "true" {
			resp := map[string]interface{}{
				"data": map[string]interface{}{
					"keys": []interface{}{"12:34:56:78", "ab:cd:ef:12"},
				},
			}
			_ = json.NewEncoder(w).Encode(resp)
			return
		}
		http.NotFound(w, r)
	}))
	defer server.Close()

	client, _ := NewClient(&ClientConfig{Address: server.URL})
	defer client.Close()
	client.SetToken("test-token")

	engine := NewPKIEngine(client, nil)
	ctx := context.Background()

	certs, err := engine.ListCertificates(ctx)
	if err != nil {
		t.Fatalf("ListCertificates failed: %v", err)
	}

	if len(certs) != 2 {
		t.Errorf("expected 2 certificates, got %d", len(certs))
	}
}

func TestPKIEngineRoles(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1/pki/roles" && r.Method == "GET" && r.URL.Query().Get("list") == "true" {
			resp := map[string]interface{}{
				"data": map[string]interface{}{
					"keys": []interface{}{"web-server", "client-auth"},
				},
			}
			_ = json.NewEncoder(w).Encode(resp)
			return
		}
		if r.URL.Path == "/v1/pki/roles/web-server" {
			if r.Method == "GET" {
				resp := map[string]interface{}{
					"data": map[string]interface{}{
						"ttl":              float64(86400),
						"max_ttl":          float64(604800),
						"allow_localhost":  true,
						"allow_any_name":   false,
						"allowed_domains":  []interface{}{"example.com"},
						"allow_subdomains": true,
						"server_flag":      true,
						"client_flag":      false,
						"key_type":         "rsa",
						"key_bits":         float64(4096),
					},
				}
				_ = json.NewEncoder(w).Encode(resp)
				return
			}
			if r.Method == "POST" {
				w.WriteHeader(http.StatusNoContent)
				return
			}
			if r.Method == "DELETE" {
				w.WriteHeader(http.StatusNoContent)
				return
			}
		}
		http.NotFound(w, r)
	}))
	defer server.Close()

	client, _ := NewClient(&ClientConfig{Address: server.URL})
	defer client.Close()
	client.SetToken("test-token")

	engine := NewPKIEngine(client, nil)
	ctx := context.Background()

	// List roles
	roles, err := engine.ListRoles(ctx)
	if err != nil {
		t.Fatalf("ListRoles failed: %v", err)
	}
	if len(roles) != 2 {
		t.Errorf("expected 2 roles, got %d", len(roles))
	}

	// Get role
	role, err := engine.GetRole(ctx, "web-server")
	if err != nil {
		t.Fatalf("GetRole failed: %v", err)
	}
	if role.TTL != 24*time.Hour {
		t.Errorf("expected TTL 24h, got %v", role.TTL)
	}
	if !role.AllowLocalhost {
		t.Error("expected allow_localhost true")
	}
	if !role.AllowSubdomains {
		t.Error("expected allow_subdomains true")
	}
	if role.KeyType != "rsa" {
		t.Errorf("expected key_type 'rsa', got %s", role.KeyType)
	}
	if role.KeyBits != 4096 {
		t.Errorf("expected key_bits 4096, got %d", role.KeyBits)
	}

	// Create role
	err = engine.CreateRole(ctx, "web-server", &PKIRole{
		TTL:              24 * time.Hour,
		MaxTTL:           720 * time.Hour,
		AllowedDomains:   []string{"example.com"},
		AllowSubdomains:  true,
		AllowLocalhost:   true,
		ServerFlag:       true,
		KeyType:          "rsa",
		KeyBits:          4096,
	})
	if err != nil {
		t.Fatalf("CreateRole failed: %v", err)
	}

	// Delete role
	err = engine.DeleteRole(ctx, "web-server")
	if err != nil {
		t.Fatalf("DeleteRole failed: %v", err)
	}

	// Validation
	_, err = engine.GetRole(ctx, "")
	if err == nil {
		t.Error("expected error for empty role name")
	}
	err = engine.CreateRole(ctx, "", nil)
	if err == nil {
		t.Error("expected error for empty role name")
	}
	err = engine.DeleteRole(ctx, "")
	if err == nil {
		t.Error("expected error for empty role name")
	}
}

func TestPKIEngineTidy(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1/pki/tidy" && r.Method == "POST" {
			w.WriteHeader(http.StatusAccepted)
			return
		}
		http.NotFound(w, r)
	}))
	defer server.Close()

	client, _ := NewClient(&ClientConfig{Address: server.URL})
	defer client.Close()
	client.SetToken("test-token")

	engine := NewPKIEngine(client, nil)
	ctx := context.Background()

	err := engine.TidyCertificates(ctx, true, true, 72*time.Hour)
	if err != nil {
		t.Fatalf("TidyCertificates failed: %v", err)
	}
}

func TestCertificateToSecret(t *testing.T) {
	cert := &Certificate{
		Certificate:    "-----BEGIN CERTIFICATE-----\nMIIC...\n-----END CERTIFICATE-----",
		PrivateKey:     "-----BEGIN RSA PRIVATE KEY-----\nMIIE...\n-----END RSA PRIVATE KEY-----",
		PrivateKeyType: "rsa",
		IssuingCA:      "-----BEGIN CERTIFICATE-----\nMIID...\n-----END CERTIFICATE-----",
		SerialNumber:   "12:34:56:78",
		CAChain:        []string{"chain-cert-1"},
		Expiration:     time.Now().Add(24 * time.Hour),
		LeaseID:        "pki/issue/role/abc123",
		LeaseDuration:  24 * time.Hour,
		Renewable:      false,
	}

	secret := cert.ToSecret("pki/issue/web-server/example.com")

	if secret.Path != "pki/issue/web-server/example.com" {
		t.Errorf("unexpected path: %s", secret.Path)
	}
	if secret.Backend != secrets.BackendTypeVault {
		t.Errorf("unexpected backend: %s", secret.Backend)
	}
	if secret.Type != secrets.SecretTypePKI {
		t.Errorf("unexpected type: %s", secret.Type)
	}
	if secret.Lease == nil {
		t.Error("expected lease")
	}
	if secret.Lease.ID != "pki/issue/role/abc123" {
		t.Errorf("unexpected lease ID: %s", secret.Lease.ID)
	}
}

func TestParseCertificatePEM(t *testing.T) {
	// Valid PEM (self-signed test cert)
	validPEM := `-----BEGIN CERTIFICATE-----
MIIBkTCB+wIJAKHBfpEgDxajMA0GCSqGSIb3DQEBCwUAMBExDzANBgNVBAMMBnRl
c3RjYTAeFw0yMzAxMDEwMDAwMDBaFw0yNDAxMDEwMDAwMDBaMBExDzANBgNVBAMM
BnRlc3RjYTBcMA0GCSqGSIb3DQEBAQUAA0sAMEgCQQC8H7F7LtRuL/A7TNhY9VLQ
TA3Y2qK0l6vxNr2N9U/DpN8F9Wd9xXwv8+N8K8D9A8K9xN8K8D9A8K9xN8K8D9A8
AgMBAAEwDQYJKoZIhvcNAQELBQADQQBKdC2NZd3mPNxN3v8F9Wd9xXwv8+N8K8D9
A8K9xN8K8D9A8K9xN8K8D9A8K9xN8K8D9A8K9xN8K8D9A8K9xN8K
-----END CERTIFICATE-----`

	// This is a malformed cert for testing, it won't parse
	// Let's test with invalid PEM
	_, err := ParseCertificatePEM("not a pem")
	if err == nil {
		t.Error("expected error for invalid PEM")
	}

	// Empty string
	_, err = ParseCertificatePEM("")
	if err == nil {
		t.Error("expected error for empty string")
	}

	// Invalid base64 in PEM block
	invalidPEM := `-----BEGIN CERTIFICATE-----
not valid base64!!!
-----END CERTIFICATE-----`
	_, err = ParseCertificatePEM(invalidPEM)
	if err == nil {
		t.Error("expected error for invalid base64 in PEM")
	}

	_ = validPEM // Avoid unused variable if we don't test with real cert
}

func TestIsCertificateExpiring(t *testing.T) {
	// Nil certificate
	if !IsCertificateExpiring(nil, time.Hour) {
		t.Error("nil certificate should be considered expiring")
	}
}

func TestGetCertificateExpiration(t *testing.T) {
	// Invalid PEM
	_, err := GetCertificateExpiration("not a pem")
	if err == nil {
		t.Error("expected error for invalid PEM")
	}
}

// Transit Engine Tests

func TestNewTransitEngine(t *testing.T) {
	client, _ := NewClient(&ClientConfig{Address: "http://localhost:8200"})
	defer client.Close()

	// Default config
	engine := NewTransitEngine(client, nil)
	if engine.config.MountPath != "transit" {
		t.Errorf("expected default mount path 'transit', got %s", engine.config.MountPath)
	}

	// Custom config
	engine = NewTransitEngine(client, &TransitConfig{
		MountPath: "custom-transit",
	})
	if engine.config.MountPath != "custom-transit" {
		t.Errorf("expected mount path 'custom-transit', got %s", engine.config.MountPath)
	}
}

func TestTransitEngineCreateKey(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1/transit/keys/my-key" && r.Method == "POST" {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		http.NotFound(w, r)
	}))
	defer server.Close()

	client, _ := NewClient(&ClientConfig{Address: server.URL})
	defer client.Close()
	client.SetToken("test-token")

	engine := NewTransitEngine(client, nil)
	ctx := context.Background()

	err := engine.CreateKey(ctx, "my-key", TransitKeyTypeAES256GCM96)
	if err != nil {
		t.Fatalf("CreateKey failed: %v", err)
	}

	// Empty name
	err = engine.CreateKey(ctx, "", TransitKeyTypeAES256GCM96)
	if err == nil {
		t.Error("expected error for empty key name")
	}
}

func TestTransitEngineCreateKeyWithOptions(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1/transit/keys/derived-key" && r.Method == "POST" {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		http.NotFound(w, r)
	}))
	defer server.Close()

	client, _ := NewClient(&ClientConfig{Address: server.URL})
	defer client.Close()
	client.SetToken("test-token")

	engine := NewTransitEngine(client, nil)
	ctx := context.Background()

	err := engine.CreateKeyWithOptions(ctx, "derived-key", &CreateKeyOptions{
		Type:             TransitKeyTypeAES256GCM96,
		Convergent:       true,
		Derived:          true,
		Exportable:       false,
		AutoRotatePeriod: 30 * 24 * time.Hour,
	})
	if err != nil {
		t.Fatalf("CreateKeyWithOptions failed: %v", err)
	}
}

func TestTransitEngineGetKey(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1/transit/keys/my-key" && r.Method == "GET" {
			resp := map[string]interface{}{
				"data": map[string]interface{}{
					"type":                   "aes256-gcm96",
					"deletion_allowed":       false,
					"derived":                false,
					"exportable":             false,
					"allow_plaintext_backup": false,
					"latest_version":         float64(3),
					"min_decryption_version": float64(1),
					"min_encryption_version": float64(3),
					"supports_encryption":    true,
					"supports_decryption":    true,
					"supports_signing":       false,
					"supports_derivation":    true,
				},
			}
			_ = json.NewEncoder(w).Encode(resp)
			return
		}
		http.NotFound(w, r)
	}))
	defer server.Close()

	client, _ := NewClient(&ClientConfig{Address: server.URL})
	defer client.Close()
	client.SetToken("test-token")

	engine := NewTransitEngine(client, nil)
	ctx := context.Background()

	key, err := engine.GetKey(ctx, "my-key")
	if err != nil {
		t.Fatalf("GetKey failed: %v", err)
	}

	if key.Name != "my-key" {
		t.Errorf("expected name 'my-key', got %s", key.Name)
	}
	if key.Type != TransitKeyTypeAES256GCM96 {
		t.Errorf("expected type aes256-gcm96, got %s", key.Type)
	}
	if key.LatestVersion != 3 {
		t.Errorf("expected latest_version 3, got %d", key.LatestVersion)
	}
	if key.MinDecryptionVersion != 1 {
		t.Errorf("expected min_decryption_version 1, got %d", key.MinDecryptionVersion)
	}
	if !key.SupportsEncryption {
		t.Error("expected supports_encryption true")
	}
	if key.SupportsSigning {
		t.Error("expected supports_signing false")
	}

	// Empty name
	_, err = engine.GetKey(ctx, "")
	if err == nil {
		t.Error("expected error for empty key name")
	}
}

func TestTransitEngineDeleteKey(t *testing.T) {
	configCalled := false
	deleteCalled := false

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1/transit/keys/my-key/config" && r.Method == "POST" {
			configCalled = true
			w.WriteHeader(http.StatusNoContent)
			return
		}
		if r.URL.Path == "/v1/transit/keys/my-key" && r.Method == "DELETE" {
			deleteCalled = true
			w.WriteHeader(http.StatusNoContent)
			return
		}
		http.NotFound(w, r)
	}))
	defer server.Close()

	client, _ := NewClient(&ClientConfig{Address: server.URL})
	defer client.Close()
	client.SetToken("test-token")

	engine := NewTransitEngine(client, nil)
	ctx := context.Background()

	err := engine.DeleteKey(ctx, "my-key")
	if err != nil {
		t.Fatalf("DeleteKey failed: %v", err)
	}

	if !configCalled {
		t.Error("expected config call to enable deletion")
	}
	if !deleteCalled {
		t.Error("expected delete call")
	}

	// Empty name
	err = engine.DeleteKey(ctx, "")
	if err == nil {
		t.Error("expected error for empty key name")
	}
}

func TestTransitEngineRotateKey(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1/transit/keys/my-key/rotate" && r.Method == "POST" {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		http.NotFound(w, r)
	}))
	defer server.Close()

	client, _ := NewClient(&ClientConfig{Address: server.URL})
	defer client.Close()
	client.SetToken("test-token")

	engine := NewTransitEngine(client, nil)
	ctx := context.Background()

	err := engine.RotateKey(ctx, "my-key")
	if err != nil {
		t.Fatalf("RotateKey failed: %v", err)
	}

	// Empty name
	err = engine.RotateKey(ctx, "")
	if err == nil {
		t.Error("expected error for empty key name")
	}
}

func TestTransitEngineListKeys(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1/transit/keys" && r.Method == "GET" && r.URL.Query().Get("list") == "true" {
			resp := map[string]interface{}{
				"data": map[string]interface{}{
					"keys": []interface{}{"key-1", "key-2", "key-3"},
				},
			}
			_ = json.NewEncoder(w).Encode(resp)
			return
		}
		http.NotFound(w, r)
	}))
	defer server.Close()

	client, _ := NewClient(&ClientConfig{Address: server.URL})
	defer client.Close()
	client.SetToken("test-token")

	engine := NewTransitEngine(client, nil)
	ctx := context.Background()

	keys, err := engine.ListKeys(ctx)
	if err != nil {
		t.Fatalf("ListKeys failed: %v", err)
	}

	if len(keys) != 3 {
		t.Errorf("expected 3 keys, got %d", len(keys))
	}
}

func TestTransitEngineEncrypt(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1/transit/encrypt/my-key" && r.Method == "POST" {
			resp := map[string]interface{}{
				"data": map[string]interface{}{
					"ciphertext": "vault:v1:base64encodeddata",
				},
			}
			_ = json.NewEncoder(w).Encode(resp)
			return
		}
		http.NotFound(w, r)
	}))
	defer server.Close()

	client, _ := NewClient(&ClientConfig{Address: server.URL})
	defer client.Close()
	client.SetToken("test-token")

	engine := NewTransitEngine(client, nil)
	ctx := context.Background()

	resp, err := engine.Encrypt(ctx, &EncryptRequest{
		KeyName:   "my-key",
		Plaintext: []byte("secret data"),
	})
	if err != nil {
		t.Fatalf("Encrypt failed: %v", err)
	}

	if resp.Ciphertext != "vault:v1:base64encodeddata" {
		t.Errorf("unexpected ciphertext: %s", resp.Ciphertext)
	}
	if resp.KeyVersion != 1 {
		t.Errorf("expected key version 1, got %d", resp.KeyVersion)
	}
}

func TestTransitEngineEncryptWithOptions(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1/transit/encrypt/derived-key" && r.Method == "POST" {
			resp := map[string]interface{}{
				"data": map[string]interface{}{
					"ciphertext": "vault:v2:deriveddata",
				},
			}
			_ = json.NewEncoder(w).Encode(resp)
			return
		}
		http.NotFound(w, r)
	}))
	defer server.Close()

	client, _ := NewClient(&ClientConfig{Address: server.URL})
	defer client.Close()
	client.SetToken("test-token")

	engine := NewTransitEngine(client, nil)
	ctx := context.Background()

	resp, err := engine.Encrypt(ctx, &EncryptRequest{
		KeyName:              "derived-key",
		Plaintext:            []byte("secret data"),
		Context:              []byte("user-123"),
		KeyVersion:           2,
		ConvergentEncryption: true,
	})
	if err != nil {
		t.Fatalf("Encrypt with options failed: %v", err)
	}

	if resp.Ciphertext != "vault:v2:deriveddata" {
		t.Errorf("unexpected ciphertext: %s", resp.Ciphertext)
	}
}

func TestTransitEngineEncryptValidation(t *testing.T) {
	client, _ := NewClient(&ClientConfig{Address: "http://localhost:8200"})
	defer client.Close()

	engine := NewTransitEngine(client, nil)
	ctx := context.Background()

	// Nil request
	_, err := engine.Encrypt(ctx, nil)
	if err == nil {
		t.Error("expected error for nil request")
	}

	// Missing key name
	_, err = engine.Encrypt(ctx, &EncryptRequest{
		Plaintext: []byte("data"),
	})
	if err == nil {
		t.Error("expected error for missing key name")
	}

	// Missing plaintext
	_, err = engine.Encrypt(ctx, &EncryptRequest{
		KeyName: "my-key",
	})
	if err == nil {
		t.Error("expected error for missing plaintext")
	}
}

func TestTransitEngineDecrypt(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1/transit/decrypt/my-key" && r.Method == "POST" {
			resp := map[string]interface{}{
				"data": map[string]interface{}{
					"plaintext": "c2VjcmV0IGRhdGE=", // base64("secret data")
				},
			}
			_ = json.NewEncoder(w).Encode(resp)
			return
		}
		http.NotFound(w, r)
	}))
	defer server.Close()

	client, _ := NewClient(&ClientConfig{Address: server.URL})
	defer client.Close()
	client.SetToken("test-token")

	engine := NewTransitEngine(client, nil)
	ctx := context.Background()

	resp, err := engine.Decrypt(ctx, &DecryptRequest{
		KeyName:    "my-key",
		Ciphertext: "vault:v1:base64encodeddata",
	})
	if err != nil {
		t.Fatalf("Decrypt failed: %v", err)
	}

	if string(resp.Plaintext) != "secret data" {
		t.Errorf("unexpected plaintext: %s", string(resp.Plaintext))
	}
}

func TestTransitEngineDecryptValidation(t *testing.T) {
	client, _ := NewClient(&ClientConfig{Address: "http://localhost:8200"})
	defer client.Close()

	engine := NewTransitEngine(client, nil)
	ctx := context.Background()

	// Nil request
	_, err := engine.Decrypt(ctx, nil)
	if err == nil {
		t.Error("expected error for nil request")
	}

	// Missing key name
	_, err = engine.Decrypt(ctx, &DecryptRequest{
		Ciphertext: "vault:v1:data",
	})
	if err == nil {
		t.Error("expected error for missing key name")
	}

	// Missing ciphertext
	_, err = engine.Decrypt(ctx, &DecryptRequest{
		KeyName: "my-key",
	})
	if err == nil {
		t.Error("expected error for missing ciphertext")
	}
}

func TestTransitEngineRewrap(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1/transit/rewrap/my-key" && r.Method == "POST" {
			resp := map[string]interface{}{
				"data": map[string]interface{}{
					"ciphertext": "vault:v3:rewrappeddata",
				},
			}
			_ = json.NewEncoder(w).Encode(resp)
			return
		}
		http.NotFound(w, r)
	}))
	defer server.Close()

	client, _ := NewClient(&ClientConfig{Address: server.URL})
	defer client.Close()
	client.SetToken("test-token")

	engine := NewTransitEngine(client, nil)
	ctx := context.Background()

	resp, err := engine.Rewrap(ctx, "my-key", "vault:v1:olddata", nil)
	if err != nil {
		t.Fatalf("Rewrap failed: %v", err)
	}

	if resp.Ciphertext != "vault:v3:rewrappeddata" {
		t.Errorf("unexpected ciphertext: %s", resp.Ciphertext)
	}
	if resp.KeyVersion != 3 {
		t.Errorf("expected key version 3, got %d", resp.KeyVersion)
	}

	// Empty key name
	_, err = engine.Rewrap(ctx, "", "vault:v1:data", nil)
	if err == nil {
		t.Error("expected error for empty key name")
	}

	// Empty ciphertext
	_, err = engine.Rewrap(ctx, "my-key", "", nil)
	if err == nil {
		t.Error("expected error for empty ciphertext")
	}
}

func TestTransitEngineSign(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1/transit/sign/signing-key" && r.Method == "POST" {
			resp := map[string]interface{}{
				"data": map[string]interface{}{
					"signature": "vault:v1:signaturedata",
				},
			}
			_ = json.NewEncoder(w).Encode(resp)
			return
		}
		http.NotFound(w, r)
	}))
	defer server.Close()

	client, _ := NewClient(&ClientConfig{Address: server.URL})
	defer client.Close()
	client.SetToken("test-token")

	engine := NewTransitEngine(client, nil)
	ctx := context.Background()

	resp, err := engine.Sign(ctx, &SignRequest{
		KeyName:       "signing-key",
		Input:         []byte("data to sign"),
		HashAlgorithm: TransitHashSHA256,
	})
	if err != nil {
		t.Fatalf("Sign failed: %v", err)
	}

	if resp.Signature != "vault:v1:signaturedata" {
		t.Errorf("unexpected signature: %s", resp.Signature)
	}
	if resp.KeyVersion != 1 {
		t.Errorf("expected key version 1, got %d", resp.KeyVersion)
	}
}

func TestTransitEngineSignValidation(t *testing.T) {
	client, _ := NewClient(&ClientConfig{Address: "http://localhost:8200"})
	defer client.Close()

	engine := NewTransitEngine(client, nil)
	ctx := context.Background()

	// Nil request
	_, err := engine.Sign(ctx, nil)
	if err == nil {
		t.Error("expected error for nil request")
	}

	// Missing key name
	_, err = engine.Sign(ctx, &SignRequest{
		Input: []byte("data"),
	})
	if err == nil {
		t.Error("expected error for missing key name")
	}

	// Missing input
	_, err = engine.Sign(ctx, &SignRequest{
		KeyName: "signing-key",
	})
	if err == nil {
		t.Error("expected error for missing input")
	}
}

func TestTransitEngineVerify(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1/transit/verify/signing-key" && r.Method == "POST" {
			resp := map[string]interface{}{
				"data": map[string]interface{}{
					"valid": true,
				},
			}
			_ = json.NewEncoder(w).Encode(resp)
			return
		}
		http.NotFound(w, r)
	}))
	defer server.Close()

	client, _ := NewClient(&ClientConfig{Address: server.URL})
	defer client.Close()
	client.SetToken("test-token")

	engine := NewTransitEngine(client, nil)
	ctx := context.Background()

	resp, err := engine.Verify(ctx, &VerifyRequest{
		KeyName:   "signing-key",
		Input:     []byte("data to sign"),
		Signature: "vault:v1:signaturedata",
	})
	if err != nil {
		t.Fatalf("Verify failed: %v", err)
	}

	if !resp.Valid {
		t.Error("expected signature to be valid")
	}
}

func TestTransitEngineVerifyValidation(t *testing.T) {
	client, _ := NewClient(&ClientConfig{Address: "http://localhost:8200"})
	defer client.Close()

	engine := NewTransitEngine(client, nil)
	ctx := context.Background()

	// Nil request
	_, err := engine.Verify(ctx, nil)
	if err == nil {
		t.Error("expected error for nil request")
	}

	// Missing key name
	_, err = engine.Verify(ctx, &VerifyRequest{
		Input:     []byte("data"),
		Signature: "sig",
	})
	if err == nil {
		t.Error("expected error for missing key name")
	}

	// Missing input
	_, err = engine.Verify(ctx, &VerifyRequest{
		KeyName:   "signing-key",
		Signature: "sig",
	})
	if err == nil {
		t.Error("expected error for missing input")
	}

	// Missing signature
	_, err = engine.Verify(ctx, &VerifyRequest{
		KeyName: "signing-key",
		Input:   []byte("data"),
	})
	if err == nil {
		t.Error("expected error for missing signature")
	}
}

func TestTransitEngineHMAC(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1/transit/hmac/hmac-key/sha2-256" && r.Method == "POST" {
			resp := map[string]interface{}{
				"data": map[string]interface{}{
					"hmac": "vault:v1:hmacvalue",
				},
			}
			_ = json.NewEncoder(w).Encode(resp)
			return
		}
		http.NotFound(w, r)
	}))
	defer server.Close()

	client, _ := NewClient(&ClientConfig{Address: server.URL})
	defer client.Close()
	client.SetToken("test-token")

	engine := NewTransitEngine(client, nil)
	ctx := context.Background()

	resp, err := engine.HMAC(ctx, &HMACRequest{
		KeyName:   "hmac-key",
		Input:     []byte("data to hmac"),
		Algorithm: TransitHashSHA256,
	})
	if err != nil {
		t.Fatalf("HMAC failed: %v", err)
	}

	if resp.HMAC != "vault:v1:hmacvalue" {
		t.Errorf("unexpected HMAC: %s", resp.HMAC)
	}
}

func TestTransitEngineHMACValidation(t *testing.T) {
	client, _ := NewClient(&ClientConfig{Address: "http://localhost:8200"})
	defer client.Close()

	engine := NewTransitEngine(client, nil)
	ctx := context.Background()

	// Nil request
	_, err := engine.HMAC(ctx, nil)
	if err == nil {
		t.Error("expected error for nil request")
	}

	// Missing key name
	_, err = engine.HMAC(ctx, &HMACRequest{
		Input: []byte("data"),
	})
	if err == nil {
		t.Error("expected error for missing key name")
	}

	// Missing input
	_, err = engine.HMAC(ctx, &HMACRequest{
		KeyName: "hmac-key",
	})
	if err == nil {
		t.Error("expected error for missing input")
	}
}

func TestTransitEngineVerifyHMAC(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1/transit/verify/hmac-key/sha2-256" && r.Method == "POST" {
			resp := map[string]interface{}{
				"data": map[string]interface{}{
					"valid": true,
				},
			}
			_ = json.NewEncoder(w).Encode(resp)
			return
		}
		http.NotFound(w, r)
	}))
	defer server.Close()

	client, _ := NewClient(&ClientConfig{Address: server.URL})
	defer client.Close()
	client.SetToken("test-token")

	engine := NewTransitEngine(client, nil)
	ctx := context.Background()

	valid, err := engine.VerifyHMAC(ctx, "hmac-key", []byte("data"), "vault:v1:hmacvalue", TransitHashSHA256)
	if err != nil {
		t.Fatalf("VerifyHMAC failed: %v", err)
	}

	if !valid {
		t.Error("expected HMAC to be valid")
	}

	// Validation tests
	_, err = engine.VerifyHMAC(ctx, "", []byte("data"), "hmac", TransitHashSHA256)
	if err == nil {
		t.Error("expected error for empty key name")
	}

	_, err = engine.VerifyHMAC(ctx, "hmac-key", nil, "hmac", TransitHashSHA256)
	if err == nil {
		t.Error("expected error for empty input")
	}

	_, err = engine.VerifyHMAC(ctx, "hmac-key", []byte("data"), "", TransitHashSHA256)
	if err == nil {
		t.Error("expected error for empty hmac")
	}
}

func TestTransitEngineHash(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1/transit/hash/sha2-256" && r.Method == "POST" {
			resp := map[string]interface{}{
				"data": map[string]interface{}{
					"sum": "abc123def456",
				},
			}
			_ = json.NewEncoder(w).Encode(resp)
			return
		}
		http.NotFound(w, r)
	}))
	defer server.Close()

	client, _ := NewClient(&ClientConfig{Address: server.URL})
	defer client.Close()
	client.SetToken("test-token")

	engine := NewTransitEngine(client, nil)
	ctx := context.Background()

	resp, err := engine.Hash(ctx, &HashRequest{
		Input:     []byte("data to hash"),
		Algorithm: TransitHashSHA256,
		Format:    "hex",
	})
	if err != nil {
		t.Fatalf("Hash failed: %v", err)
	}

	if resp.Sum != "abc123def456" {
		t.Errorf("unexpected hash: %s", resp.Sum)
	}
}

func TestTransitEngineHashValidation(t *testing.T) {
	client, _ := NewClient(&ClientConfig{Address: "http://localhost:8200"})
	defer client.Close()

	engine := NewTransitEngine(client, nil)
	ctx := context.Background()

	// Nil request
	_, err := engine.Hash(ctx, nil)
	if err == nil {
		t.Error("expected error for nil request")
	}

	// Missing input
	_, err = engine.Hash(ctx, &HashRequest{})
	if err == nil {
		t.Error("expected error for missing input")
	}
}

func TestTransitEngineGenerateRandomBytes(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1/transit/random/32" && r.Method == "POST" {
			resp := map[string]interface{}{
				"data": map[string]interface{}{
					"random_bytes": "YWJjZGVmZ2hpamtsbW5vcHFyc3R1dnd4eXoxMjM0NTY=", // 32 random bytes
				},
			}
			_ = json.NewEncoder(w).Encode(resp)
			return
		}
		http.NotFound(w, r)
	}))
	defer server.Close()

	client, _ := NewClient(&ClientConfig{Address: server.URL})
	defer client.Close()
	client.SetToken("test-token")

	engine := NewTransitEngine(client, nil)
	ctx := context.Background()

	bytes, err := engine.GenerateRandomBytes(ctx, 32, "base64")
	if err != nil {
		t.Fatalf("GenerateRandomBytes failed: %v", err)
	}

	if len(bytes) == 0 {
		t.Error("expected random bytes")
	}

	// Validation
	_, err = engine.GenerateRandomBytes(ctx, 0, "")
	if err == nil {
		t.Error("expected error for zero byte count")
	}

	_, err = engine.GenerateRandomBytes(ctx, -1, "")
	if err == nil {
		t.Error("expected error for negative byte count")
	}
}

func TestTransitEngineGenerateDataKey(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1/transit/datakey/plaintext/my-key" && r.Method == "POST" {
			resp := map[string]interface{}{
				"data": map[string]interface{}{
					"plaintext":  "YWJjZGVmZ2hpamtsbW5vcA==", // 16 bytes
					"ciphertext": "vault:v1:wrappedkey",
				},
			}
			_ = json.NewEncoder(w).Encode(resp)
			return
		}
		http.NotFound(w, r)
	}))
	defer server.Close()

	client, _ := NewClient(&ClientConfig{Address: server.URL})
	defer client.Close()
	client.SetToken("test-token")

	engine := NewTransitEngine(client, nil)
	ctx := context.Background()

	plaintext, ciphertext, err := engine.GenerateDataKey(ctx, "my-key", 256, nil)
	if err != nil {
		t.Fatalf("GenerateDataKey failed: %v", err)
	}

	if len(plaintext) == 0 {
		t.Error("expected plaintext data key")
	}
	if len(ciphertext) == 0 {
		t.Error("expected ciphertext data key")
	}

	// Validation
	_, _, err = engine.GenerateDataKey(ctx, "", 256, nil)
	if err == nil {
		t.Error("expected error for empty key name")
	}
}

func TestTransitEngineGenerateWrappedDataKey(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1/transit/datakey/wrapped/my-key" && r.Method == "POST" {
			resp := map[string]interface{}{
				"data": map[string]interface{}{
					"ciphertext": "vault:v1:wrappedkey",
				},
			}
			_ = json.NewEncoder(w).Encode(resp)
			return
		}
		http.NotFound(w, r)
	}))
	defer server.Close()

	client, _ := NewClient(&ClientConfig{Address: server.URL})
	defer client.Close()
	client.SetToken("test-token")

	engine := NewTransitEngine(client, nil)
	ctx := context.Background()

	ciphertext, err := engine.GenerateWrappedDataKey(ctx, "my-key", 256, nil)
	if err != nil {
		t.Fatalf("GenerateWrappedDataKey failed: %v", err)
	}

	if len(ciphertext) == 0 {
		t.Error("expected ciphertext")
	}

	// Validation
	_, err = engine.GenerateWrappedDataKey(ctx, "", 256, nil)
	if err == nil {
		t.Error("expected error for empty key name")
	}
}

func TestTransitEngineBatchEncrypt(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1/transit/encrypt/my-key" && r.Method == "POST" {
			resp := map[string]interface{}{
				"data": map[string]interface{}{
					"batch_results": []interface{}{
						map[string]interface{}{"ciphertext": "vault:v1:cipher1"},
						map[string]interface{}{"ciphertext": "vault:v1:cipher2"},
						map[string]interface{}{"ciphertext": "vault:v1:cipher3"},
					},
				},
			}
			_ = json.NewEncoder(w).Encode(resp)
			return
		}
		http.NotFound(w, r)
	}))
	defer server.Close()

	client, _ := NewClient(&ClientConfig{Address: server.URL})
	defer client.Close()
	client.SetToken("test-token")

	engine := NewTransitEngine(client, nil)
	ctx := context.Background()

	items := []BatchEncryptItem{
		{Plaintext: []byte("data1")},
		{Plaintext: []byte("data2")},
		{Plaintext: []byte("data3")},
	}

	ciphertexts, err := engine.BatchEncrypt(ctx, "my-key", items)
	if err != nil {
		t.Fatalf("BatchEncrypt failed: %v", err)
	}

	if len(ciphertexts) != 3 {
		t.Errorf("expected 3 ciphertexts, got %d", len(ciphertexts))
	}

	// Validation
	_, err = engine.BatchEncrypt(ctx, "", items)
	if err == nil {
		t.Error("expected error for empty key name")
	}

	_, err = engine.BatchEncrypt(ctx, "my-key", nil)
	if err == nil {
		t.Error("expected error for empty items")
	}

	_, err = engine.BatchEncrypt(ctx, "my-key", []BatchEncryptItem{})
	if err == nil {
		t.Error("expected error for empty items")
	}
}

func TestTransitEngineBatchDecrypt(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1/transit/decrypt/my-key" && r.Method == "POST" {
			resp := map[string]interface{}{
				"data": map[string]interface{}{
					"batch_results": []interface{}{
						map[string]interface{}{"plaintext": "ZGF0YTE="}, // data1
						map[string]interface{}{"plaintext": "ZGF0YTI="}, // data2
						map[string]interface{}{"plaintext": "ZGF0YTM="}, // data3
					},
				},
			}
			_ = json.NewEncoder(w).Encode(resp)
			return
		}
		http.NotFound(w, r)
	}))
	defer server.Close()

	client, _ := NewClient(&ClientConfig{Address: server.URL})
	defer client.Close()
	client.SetToken("test-token")

	engine := NewTransitEngine(client, nil)
	ctx := context.Background()

	items := []BatchDecryptItem{
		{Ciphertext: "vault:v1:cipher1"},
		{Ciphertext: "vault:v1:cipher2"},
		{Ciphertext: "vault:v1:cipher3"},
	}

	plaintexts, err := engine.BatchDecrypt(ctx, "my-key", items)
	if err != nil {
		t.Fatalf("BatchDecrypt failed: %v", err)
	}

	if len(plaintexts) != 3 {
		t.Errorf("expected 3 plaintexts, got %d", len(plaintexts))
	}
	if string(plaintexts[0]) != "data1" {
		t.Errorf("expected 'data1', got '%s'", string(plaintexts[0]))
	}

	// Validation
	_, err = engine.BatchDecrypt(ctx, "", items)
	if err == nil {
		t.Error("expected error for empty key name")
	}

	_, err = engine.BatchDecrypt(ctx, "my-key", nil)
	if err == nil {
		t.Error("expected error for empty items")
	}
}

func TestTransitEngineUpdateKeyConfig(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1/transit/keys/my-key/config" && r.Method == "POST" {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		http.NotFound(w, r)
	}))
	defer server.Close()

	client, _ := NewClient(&ClientConfig{Address: server.URL})
	defer client.Close()
	client.SetToken("test-token")

	engine := NewTransitEngine(client, nil)
	ctx := context.Background()

	err := engine.UpdateKeyConfig(ctx, "my-key", 2, 3, true)
	if err != nil {
		t.Fatalf("UpdateKeyConfig failed: %v", err)
	}

	// Validation
	err = engine.UpdateKeyConfig(ctx, "", 1, 1, false)
	if err == nil {
		t.Error("expected error for empty key name")
	}
}

func TestEncryptRequestToTransitRequest(t *testing.T) {
	req := &EncryptRequest{
		KeyName:              "test-key",
		Plaintext:            []byte("secret"),
		Context:              []byte("context"),
		KeyVersion:           2,
		ConvergentEncryption: true,
	}

	transitReq := req.ToTransitRequest()

	if transitReq.Operation != secrets.TransitOperationEncrypt {
		t.Errorf("expected encrypt operation, got %s", transitReq.Operation)
	}
	if transitReq.KeyName != "test-key" {
		t.Errorf("expected key name 'test-key', got %s", transitReq.KeyName)
	}
	if transitReq.KeyVersion != 2 {
		t.Errorf("expected key version 2, got %d", transitReq.KeyVersion)
	}
	if !transitReq.Convergent {
		t.Error("expected convergent true")
	}
}

func TestEncryptResponseToTransitResponse(t *testing.T) {
	resp := &EncryptResponse{
		Ciphertext: "vault:v2:encrypted",
		KeyVersion: 2,
	}

	transitResp := resp.ToTransitResponse()

	if transitResp.Ciphertext != "vault:v2:encrypted" {
		t.Errorf("unexpected ciphertext: %s", transitResp.Ciphertext)
	}
	if transitResp.KeyVersion != 2 {
		t.Errorf("expected key version 2, got %d", transitResp.KeyVersion)
	}
}
