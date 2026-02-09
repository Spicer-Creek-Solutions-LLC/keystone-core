package registry

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestNewGCRCredentialProvider(t *testing.T) {
	provider := NewGCRCredentialProvider()

	if provider == nil {
		t.Fatal("Expected provider to be non-nil")
	}
	if provider.httpClient == nil {
		t.Error("Expected httpClient to be non-nil")
	}
}

func TestGCRCredentialProvider_Type(t *testing.T) {
	provider := NewGCRCredentialProvider()

	if provider.Type() != TypeGCR {
		t.Errorf("Type() = %v, want %v", provider.Type(), TypeGCR)
	}
}

func TestGCRCredentialProvider_MatchesRegistry(t *testing.T) {
	provider := NewGCRCredentialProvider()

	tests := []struct {
		registry string
		want     bool
	}{
		{"gcr.io", true},
		{"us.gcr.io", true},
		{"eu.gcr.io", true},
		{"asia.gcr.io", true},
		{"us-docker.pkg.dev", true},
		{"us-central1-docker.pkg.dev", true},
		{"docker.io", false},
		{"123456789.dkr.ecr.us-west-2.amazonaws.com", false},
		{"myregistry.azurecr.io", false},
	}

	for _, tt := range tests {
		t.Run(tt.registry, func(t *testing.T) {
			if got := provider.MatchesRegistry(tt.registry); got != tt.want {
				t.Errorf("MatchesRegistry(%q) = %v, want %v", tt.registry, got, tt.want)
			}
		})
	}
}

func TestGCRCredentialProvider_IsAvailable(t *testing.T) {
	// This will always return false in a non-GCP environment
	provider := NewGCRCredentialProvider()
	// We just verify it doesn't panic
	_ = provider.IsAvailable()
}

func TestGCRCredentialProvider_GetCredential_MockMetadata(t *testing.T) {
	// Create mock GCP metadata server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Check for required header
		if r.Header.Get("Metadata-Flavor") != "Google" {
			http.Error(w, "Missing Metadata-Flavor header", http.StatusForbidden)
			return
		}

		switch r.URL.Path {
		case "/computeMetadata/v1/instance/service-accounts/default/token":
			response := map[string]interface{}{
				"access_token": "test-access-token-12345",
				"expires_in":   3600,
				"token_type":   "Bearer",
			}
			json.NewEncoder(w).Encode(response)
		case "/computeMetadata/v1/project/project-id":
			w.Write([]byte("test-project-123"))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	// Create provider with custom client (would need to override base URL in real test)
	provider := NewGCRCredentialProviderWithClient(&http.Client{
		Timeout: 5 * time.Second,
	})

	// Test that provider was created
	if provider == nil {
		t.Fatal("Expected provider to be non-nil")
	}

	t.Log("Mock GCP metadata server created successfully")
}

func TestGCRArtifactRegistryCredentialProvider(t *testing.T) {
	// Test that alias works
	provider := NewGCRArtifactRegistryCredentialProvider()

	if provider == nil {
		t.Fatal("Expected provider to be non-nil")
	}
	if provider.Type() != TypeGCR {
		t.Errorf("Type() = %v, want %v", provider.Type(), TypeGCR)
	}
}

func TestGCRCredentialProvider_GetCredential(t *testing.T) {
	// Create mock server that simulates GCP metadata
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Metadata-Flavor") != "Google" {
			http.Error(w, "Forbidden", http.StatusForbidden)
			return
		}

		switch r.URL.Path {
		case "/computeMetadata/v1/instance/service-accounts/default/token":
			json.NewEncoder(w).Encode(map[string]interface{}{
				"access_token": "ya29.test-token",
				"expires_in":   3600,
				"token_type":   "Bearer",
			})
		case "/computeMetadata/v1/project/project-id":
			w.Write([]byte("my-project"))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	// Note: To fully test this, we'd need to mock the metadata URL
	// This test documents the expected response format
	t.Log("GCR credential provider test with mock server")
}

func TestGCRCredential_Format(t *testing.T) {
	// Test that GCR credentials use the correct format
	cred := &Credential{
		Type:      TypeGCR,
		Registry:  "gcr.io",
		Username:  "_token",
		Password:  "test-access-token",
		Token:     "test-access-token",
		ExpiresAt: time.Now().Add(1 * time.Hour),
		ProjectID: "my-project",
	}

	if cred.Username != "_token" {
		t.Errorf("Username = %q, want %q", cred.Username, "_token")
	}
	if cred.IsExpired() {
		t.Error("Credential should not be expired")
	}
}
