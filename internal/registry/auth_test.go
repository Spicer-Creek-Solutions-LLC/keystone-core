package registry

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestRegistryType(t *testing.T) {
	tests := []struct {
		registry string
		expected RegistryType
	}{
		{"docker.io", RegistryTypeDocker},
		{"index.docker.io", RegistryTypeDocker},
		{"123456789.dkr.ecr.us-east-1.amazonaws.com", RegistryTypeECR},
		{"gcr.io", RegistryTypeGCR},
		{"us-docker.pkg.dev", RegistryTypeGCR},
		{"myregistry.azurecr.io", RegistryTypeACR},
		{"ghcr.io", RegistryTypeGitHub},
		{"quay.io", RegistryTypeQuay},
		{"my-private-registry.example.com", RegistryTypeGeneric},
	}

	for _, tt := range tests {
		t.Run(tt.registry, func(t *testing.T) {
			got := DetectRegistryType(tt.registry)
			if got != tt.expected {
				t.Errorf("DetectRegistryType(%s) = %v, want %v", tt.registry, got, tt.expected)
			}
		})
	}
}

func TestCredential_IsExpired(t *testing.T) {
	tests := []struct {
		name     string
		cred     *Credential
		expected bool
	}{
		{
			name:     "no expiry",
			cred:     &Credential{},
			expected: false,
		},
		{
			name: "not expired",
			cred: &Credential{
				ExpiresAt: time.Now().Add(time.Hour),
			},
			expected: false,
		},
		{
			name: "expired",
			cred: &Credential{
				ExpiresAt: time.Now().Add(-time.Hour),
			},
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.cred.IsExpired(); got != tt.expected {
				t.Errorf("IsExpired() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestAuthManager_Authenticate(t *testing.T) {
	// Create a test server that returns a token
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(TokenResponse{
			Token:     "test-token",
			ExpiresIn: 3600,
		})
	}))
	defer server.Close()

	am := NewAuthManager()

	t.Run("ecr auth", func(t *testing.T) {
		cred := &Credential{
			Type:      RegistryTypeECR,
			Registry:  "123456789.dkr.ecr.us-east-1.amazonaws.com",
			Region:    "us-east-1",
			AccountID: "123456789",
		}

		result, err := am.Authenticate(context.Background(), cred)
		if err != nil {
			t.Fatalf("Authenticate failed: %v", err)
		}

		if result.Type != RegistryTypeECR {
			t.Errorf("Type = %v, want %v", result.Type, RegistryTypeECR)
		}
	})

	t.Run("unsupported registry", func(t *testing.T) {
		cred := &Credential{
			Type:     "unsupported",
			Registry: "unsupported.example.com",
		}

		_, err := am.Authenticate(context.Background(), cred)
		if err != ErrUnsupportedRegistry {
			t.Errorf("Expected ErrUnsupportedRegistry, got %v", err)
		}
	})
}

func TestAuthManager_GetCredential(t *testing.T) {
	am := NewAuthManager()
	ctx := context.Background()

	// Cache a credential
	cred := &Credential{
		Type:      RegistryTypeDocker,
		Registry:  "docker.io",
		Username:  "testuser",
		Password:  "testpass",
		ExpiresAt: time.Now().Add(time.Hour),
	}
	am.mu.Lock()
	am.credentials["docker.io"] = cred
	am.mu.Unlock()

	t.Run("get existing credential", func(t *testing.T) {
		result, err := am.GetCredential(ctx, "docker.io")
		if err != nil {
			t.Fatalf("GetCredential failed: %v", err)
		}
		if result.Username != "testuser" {
			t.Errorf("Username = %v, want testuser", result.Username)
		}
	})

	t.Run("get non-existent credential", func(t *testing.T) {
		_, err := am.GetCredential(ctx, "nonexistent.io")
		if err == nil {
			t.Error("Expected error for non-existent credential")
		}
	})
}

func TestAuthManager_LoadDockerConfig(t *testing.T) {
	am := NewAuthManager()
	ctx := context.Background()

	config := &DockerConfig{
		Auths: map[string]AuthConfig{
			"docker.io": {
				Auth: base64.StdEncoding.EncodeToString([]byte("user:pass")),
			},
			"gcr.io": {
				Username: "gcr-user",
				Password: "gcr-pass",
			},
		},
	}

	if err := am.LoadDockerConfig(ctx, config); err != nil {
		t.Fatalf("LoadDockerConfig failed: %v", err)
	}

	// Verify credentials were loaded
	am.mu.RLock()
	dockerCred := am.credentials["docker.io"]
	gcrCred := am.credentials["gcr.io"]
	am.mu.RUnlock()

	if dockerCred == nil {
		t.Fatal("Docker credential not loaded")
	}
	if dockerCred.Username != "user" || dockerCred.Password != "pass" {
		t.Errorf("Docker credential incorrect: %v:%v", dockerCred.Username, dockerCred.Password)
	}

	if gcrCred == nil {
		t.Fatal("GCR credential not loaded")
	}
	if gcrCred.Username != "gcr-user" {
		t.Errorf("GCR username = %v, want gcr-user", gcrCred.Username)
	}
}

func TestAuthManager_ExportDockerConfig(t *testing.T) {
	am := NewAuthManager()
	ctx := context.Background()

	// Add credentials
	am.mu.Lock()
	am.credentials["docker.io"] = &Credential{
		Type:     RegistryTypeDocker,
		Registry: "docker.io",
		Username: "user",
		Password: "pass",
	}
	am.mu.Unlock()

	config, err := am.ExportDockerConfig(ctx)
	if err != nil {
		t.Fatalf("ExportDockerConfig failed: %v", err)
	}

	if len(config.Auths) == 0 {
		t.Fatal("Expected at least one auth config")
	}
}

func TestAuthManager_Events(t *testing.T) {
	am := NewAuthManager()
	ctx := context.Background()

	var receivedEvent *AuthEvent
	am.AddListener(func(event *AuthEvent) {
		receivedEvent = event
	})

	cred := &Credential{
		Type:     RegistryTypeDocker,
		Registry: "docker.io",
		Username: "testuser",
		Password: "testpass",
	}

	am.Authenticate(ctx, cred)

	if receivedEvent == nil {
		t.Fatal("No event received")
	}
	if receivedEvent.Type != "authenticate" {
		t.Errorf("Event type = %v, want authenticate", receivedEvent.Type)
	}
	if receivedEvent.Registry != "docker.io" {
		t.Errorf("Registry = %v, want docker.io", receivedEvent.Registry)
	}
}

func TestDockerAuthenticator(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Check for basic auth
		user, pass, ok := r.BasicAuth()
		if !ok || user != "testuser" || pass != "testpass" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}

		json.NewEncoder(w).Encode(TokenResponse{
			Token:     "docker-token",
			ExpiresIn: 3600,
		})
	}))
	defer server.Close()

	auth := NewDockerAuthenticator(http.DefaultClient)

	t.Run("type", func(t *testing.T) {
		if auth.Type() != RegistryTypeDocker {
			t.Errorf("Type() = %v, want %v", auth.Type(), RegistryTypeDocker)
		}
	})

	t.Run("get auth config", func(t *testing.T) {
		cred := &Credential{
			Username: "user",
			Password: "pass",
		}
		config, err := auth.GetAuthConfig(context.Background(), cred)
		if err != nil {
			t.Fatalf("GetAuthConfig failed: %v", err)
		}
		if config.Username != "user" {
			t.Errorf("Username = %v, want user", config.Username)
		}
		if config.ServerAddress != "https://index.docker.io/v1/" {
			t.Errorf("ServerAddress = %v", config.ServerAddress)
		}
	})
}

func TestECRAuthenticator(t *testing.T) {
	auth := NewECRAuthenticator(http.DefaultClient)
	ctx := context.Background()

	t.Run("type", func(t *testing.T) {
		if auth.Type() != RegistryTypeECR {
			t.Errorf("Type() = %v, want %v", auth.Type(), RegistryTypeECR)
		}
	})

	t.Run("authenticate without region", func(t *testing.T) {
		cred := &Credential{
			Type: RegistryTypeECR,
		}
		_, err := auth.Authenticate(ctx, cred)
		if err == nil {
			t.Error("Expected error for missing region")
		}
	})

	t.Run("authenticate with region", func(t *testing.T) {
		cred := &Credential{
			Type:      RegistryTypeECR,
			Region:    "us-east-1",
			AccountID: "123456789",
		}
		result, err := auth.Authenticate(ctx, cred)
		if err != nil {
			t.Fatalf("Authenticate failed: %v", err)
		}
		if result.ExpiresAt.IsZero() {
			t.Error("ExpiresAt should be set")
		}
	})

	t.Run("get auth config", func(t *testing.T) {
		cred := &Credential{
			Region:    "us-east-1",
			AccountID: "123456789",
			Token:     "ecr-token",
		}
		config, err := auth.GetAuthConfig(ctx, cred)
		if err != nil {
			t.Fatalf("GetAuthConfig failed: %v", err)
		}
		if config.Username != "AWS" {
			t.Errorf("Username = %v, want AWS", config.Username)
		}
	})
}

func TestGCRAuthenticator(t *testing.T) {
	auth := NewGCRAuthenticator(http.DefaultClient)
	ctx := context.Background()

	t.Run("type", func(t *testing.T) {
		if auth.Type() != RegistryTypeGCR {
			t.Errorf("Type() = %v, want %v", auth.Type(), RegistryTypeGCR)
		}
	})

	t.Run("authenticate without token", func(t *testing.T) {
		cred := &Credential{
			Type: RegistryTypeGCR,
		}
		_, err := auth.Authenticate(ctx, cred)
		if err == nil {
			t.Error("Expected error for missing token")
		}
	})

	t.Run("authenticate with token", func(t *testing.T) {
		cred := &Credential{
			Type:  RegistryTypeGCR,
			Token: "gcr-token",
		}
		result, err := auth.Authenticate(ctx, cred)
		if err != nil {
			t.Fatalf("Authenticate failed: %v", err)
		}
		if result.ExpiresAt.IsZero() {
			t.Error("ExpiresAt should be set")
		}
	})
}

func TestACRAuthenticator(t *testing.T) {
	auth := NewACRAuthenticator(http.DefaultClient)
	ctx := context.Background()

	t.Run("type", func(t *testing.T) {
		if auth.Type() != RegistryTypeACR {
			t.Errorf("Type() = %v, want %v", auth.Type(), RegistryTypeACR)
		}
	})

	t.Run("authenticate without credentials", func(t *testing.T) {
		cred := &Credential{
			Type: RegistryTypeACR,
		}
		_, err := auth.Authenticate(ctx, cred)
		if err == nil {
			t.Error("Expected error for missing credentials")
		}
	})

	t.Run("authenticate with token", func(t *testing.T) {
		cred := &Credential{
			Type:     RegistryTypeACR,
			Registry: "myregistry.azurecr.io",
			Token:    "acr-token",
		}
		result, err := auth.Authenticate(ctx, cred)
		if err != nil {
			t.Fatalf("Authenticate failed: %v", err)
		}
		if result.ExpiresAt.IsZero() {
			t.Error("ExpiresAt should be set")
		}
	})
}

func TestGitHubAuthenticator(t *testing.T) {
	auth := NewGitHubAuthenticator(http.DefaultClient)
	ctx := context.Background()

	t.Run("type", func(t *testing.T) {
		if auth.Type() != RegistryTypeGitHub {
			t.Errorf("Type() = %v, want %v", auth.Type(), RegistryTypeGitHub)
		}
	})

	t.Run("authenticate without token", func(t *testing.T) {
		cred := &Credential{
			Type: RegistryTypeGitHub,
		}
		_, err := auth.Authenticate(ctx, cred)
		if err == nil {
			t.Error("Expected error for missing token")
		}
	})

	t.Run("authenticate with token", func(t *testing.T) {
		cred := &Credential{
			Type:  RegistryTypeGitHub,
			Token: "ghp_xxxxxxxxxxxx",
		}
		result, err := auth.Authenticate(ctx, cred)
		if err != nil {
			t.Fatalf("Authenticate failed: %v", err)
		}
		if result.ExpiresAt.IsZero() {
			t.Error("ExpiresAt should be set")
		}
	})
}

func TestQuayAuthenticator(t *testing.T) {
	auth := NewQuayAuthenticator(http.DefaultClient)
	ctx := context.Background()

	t.Run("type", func(t *testing.T) {
		if auth.Type() != RegistryTypeQuay {
			t.Errorf("Type() = %v, want %v", auth.Type(), RegistryTypeQuay)
		}
	})

	t.Run("authenticate without credentials", func(t *testing.T) {
		cred := &Credential{
			Type: RegistryTypeQuay,
		}
		_, err := auth.Authenticate(ctx, cred)
		if err == nil {
			t.Error("Expected error for missing credentials")
		}
	})

	t.Run("authenticate with credentials", func(t *testing.T) {
		cred := &Credential{
			Type:     RegistryTypeQuay,
			Username: "quay+robot",
			Password: "robot-token",
		}
		result, err := auth.Authenticate(ctx, cred)
		if err != nil {
			t.Fatalf("Authenticate failed: %v", err)
		}
		if result.ExpiresAt.IsZero() {
			t.Error("ExpiresAt should be set")
		}
	})
}

func TestGenericAuthenticator(t *testing.T) {
	auth := NewGenericAuthenticator(http.DefaultClient)
	ctx := context.Background()

	t.Run("type", func(t *testing.T) {
		if auth.Type() != RegistryTypeGeneric {
			t.Errorf("Type() = %v, want %v", auth.Type(), RegistryTypeGeneric)
		}
	})

	t.Run("get auth config", func(t *testing.T) {
		cred := &Credential{
			Registry: "private.example.com",
			Username: "user",
			Password: "pass",
		}
		config, err := auth.GetAuthConfig(ctx, cred)
		if err != nil {
			t.Fatalf("GetAuthConfig failed: %v", err)
		}
		if config.Username != "user" {
			t.Errorf("Username = %v, want user", config.Username)
		}
	})
}

func TestInMemoryCredentialStore(t *testing.T) {
	store := NewInMemoryCredentialStore()
	ctx := context.Background()

	cred := &Credential{
		Type:     RegistryTypeDocker,
		Registry: "docker.io",
		Username: "user",
		Password: "pass",
	}

	t.Run("store credential", func(t *testing.T) {
		if err := store.Store(ctx, cred); err != nil {
			t.Fatalf("Store failed: %v", err)
		}
	})

	t.Run("get credential", func(t *testing.T) {
		result, err := store.Get(ctx, "docker.io")
		if err != nil {
			t.Fatalf("Get failed: %v", err)
		}
		if result.Username != "user" {
			t.Errorf("Username = %v, want user", result.Username)
		}
	})

	t.Run("list credentials", func(t *testing.T) {
		creds, err := store.List(ctx)
		if err != nil {
			t.Fatalf("List failed: %v", err)
		}
		if len(creds) != 1 {
			t.Errorf("Expected 1 credential, got %d", len(creds))
		}
	})

	t.Run("delete credential", func(t *testing.T) {
		if err := store.Delete(ctx, "docker.io"); err != nil {
			t.Fatalf("Delete failed: %v", err)
		}

		_, err := store.Get(ctx, "docker.io")
		if err == nil {
			t.Error("Expected error after delete")
		}
	})

	t.Run("get non-existent", func(t *testing.T) {
		_, err := store.Get(ctx, "nonexistent.io")
		if err == nil {
			t.Error("Expected error for non-existent credential")
		}
	})
}

func TestParseBearerChallenge(t *testing.T) {
	tests := []struct {
		header string
		realm  string
		service string
		scope  string
	}{
		{
			header:  `Bearer realm="https://auth.example.com/token",service="registry",scope="repository:myimage:pull"`,
			realm:   "https://auth.example.com/token",
			service: "registry",
			scope:   "repository:myimage:pull",
		},
		{
			header: `Bearer realm="https://auth.docker.io/token"`,
			realm:  "https://auth.docker.io/token",
		},
	}

	for _, tt := range tests {
		t.Run(tt.header, func(t *testing.T) {
			params := parseBearerChallenge(tt.header)
			if params["realm"] != tt.realm {
				t.Errorf("realm = %v, want %v", params["realm"], tt.realm)
			}
			if tt.service != "" && params["service"] != tt.service {
				t.Errorf("service = %v, want %v", params["service"], tt.service)
			}
			if tt.scope != "" && params["scope"] != tt.scope {
				t.Errorf("scope = %v, want %v", params["scope"], tt.scope)
			}
		})
	}
}

func TestGenericAuthenticator_BearerAuth(t *testing.T) {
	// Create a test server that implements bearer auth
	tokenServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		user, pass, ok := r.BasicAuth()
		if !ok || user != "user" || pass != "pass" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}

		json.NewEncoder(w).Encode(TokenResponse{
			Token:     "bearer-token",
			ExpiresIn: 3600,
		})
	}))
	defer tokenServer.Close()

	registryServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("WWW-Authenticate", `Bearer realm="`+tokenServer.URL+`",service="registry"`)
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer registryServer.Close()

	auth := NewGenericAuthenticator(http.DefaultClient)
	ctx := context.Background()

	cred := &Credential{
		Type:     RegistryTypeGeneric,
		Registry: registryServer.URL,
		Username: "user",
		Password: "pass",
	}

	result, err := auth.Authenticate(ctx, cred)
	if err != nil {
		t.Fatalf("Authenticate failed: %v", err)
	}

	if result.Token != "bearer-token" {
		t.Errorf("Token = %v, want bearer-token", result.Token)
	}
}
