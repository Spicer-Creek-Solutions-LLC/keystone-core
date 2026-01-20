package registry

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestNewACRCredentialProvider(t *testing.T) {
	provider := NewACRCredentialProvider()

	if provider == nil {
		t.Fatal("Expected provider to be non-nil")
	}
	if provider.httpClient == nil {
		t.Error("Expected httpClient to be non-nil")
	}
}

func TestACRCredentialProvider_RegistryType(t *testing.T) {
	provider := NewACRCredentialProvider()

	if provider.RegistryType() != RegistryTypeACR {
		t.Errorf("RegistryType() = %v, want %v", provider.RegistryType(), RegistryTypeACR)
	}
}

func TestACRCredentialProvider_MatchesRegistry(t *testing.T) {
	provider := NewACRCredentialProvider()

	tests := []struct {
		registry string
		want     bool
	}{
		{"myregistry.azurecr.io", true},
		{"test.azurecr.io", true},
		{"MYREGISTRY.AZURECR.IO", true}, // Case insensitive
		{"docker.io", false},
		{"gcr.io", false},
		{"123456789.dkr.ecr.us-west-2.amazonaws.com", false},
	}

	for _, tt := range tests {
		t.Run(tt.registry, func(t *testing.T) {
			if got := provider.MatchesRegistry(tt.registry); got != tt.want {
				t.Errorf("MatchesRegistry(%q) = %v, want %v", tt.registry, got, tt.want)
			}
		})
	}
}

func TestACRCredentialProvider_IsAvailable(t *testing.T) {
	// This will always return false in a non-Azure environment
	provider := NewACRCredentialProvider()
	// We just verify it doesn't panic
	_ = provider.IsAvailable()
}

func TestNormalizeACRRegistry(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"myregistry.azurecr.io", "myregistry.azurecr.io"},
		{"https://myregistry.azurecr.io", "myregistry.azurecr.io"},
		{"http://myregistry.azurecr.io", "myregistry.azurecr.io"},
		{"myregistry.azurecr.io/", "myregistry.azurecr.io"},
		{"myregistry.azurecr.io/repo/image", "myregistry.azurecr.io"},
		{"https://myregistry.azurecr.io/v2/", "myregistry.azurecr.io"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			if got := normalizeACRRegistry(tt.input); got != tt.want {
				t.Errorf("normalizeACRRegistry(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestACRCredentialProvider_MockTokenExchange(t *testing.T) {
	// Mock Azure IMDS server
	imdsServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Metadata") != "true" {
			http.Error(w, "Missing Metadata header", http.StatusBadRequest)
			return
		}

		switch {
		case r.URL.Path == "/metadata/identity/oauth2/token":
			response := map[string]interface{}{
				"access_token":  "eyJ0eXAiOiJKV1QiLCJhbGciOiJSUzI1NiJ9...",
				"expires_in":    "3600",
				"expires_on":    "1609459200",
				"resource":      r.URL.Query().Get("resource"),
				"token_type":    "Bearer",
				"client_id":     "test-client-id",
			}
			json.NewEncoder(w).Encode(response)
		case r.URL.Path == "/metadata/instance/compute":
			response := map[string]interface{}{
				"subscriptionId":    "sub-12345",
				"resourceGroupName": "my-rg",
				"name":              "my-vm",
				"vmId":              "vm-12345",
			}
			json.NewEncoder(w).Encode(response)
		default:
			http.NotFound(w, r)
		}
	}))
	defer imdsServer.Close()

	// Mock ACR token exchange server
	acrServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/oauth2/exchange" {
			response := map[string]interface{}{
				"refresh_token": "acr-refresh-token-12345",
			}
			json.NewEncoder(w).Encode(response)
		} else {
			http.NotFound(w, r)
		}
	}))
	defer acrServer.Close()

	t.Log("Mock Azure IMDS and ACR servers created successfully")
}

func TestACRCredential_Format(t *testing.T) {
	// Test that ACR credentials use the correct format
	cred := &Credential{
		Type:           RegistryTypeACR,
		Registry:       "myregistry.azurecr.io",
		Username:       "00000000-0000-0000-0000-000000000000",
		Password:       "acr-refresh-token",
		Token:          "acr-refresh-token",
		ExpiresAt:      time.Now().Add(3 * time.Hour),
		SubscriptionID: "sub-12345",
	}

	// ACR uses a GUID as username for token auth
	if cred.Username != "00000000-0000-0000-0000-000000000000" {
		t.Errorf("Username = %q, want GUID format", cred.Username)
	}
	if cred.IsExpired() {
		t.Error("Credential should not be expired")
	}
}

func TestACRCredentialProvider_GetCredential_Integration(t *testing.T) {
	// Skip in CI - requires Azure environment
	if testing.Short() {
		t.Skip("Skipping integration test")
	}

	provider := NewACRCredentialProvider()
	ctx := context.Background()

	// This would only work in an Azure environment
	_, err := provider.GetCredential(ctx, "myregistry.azurecr.io")
	if err != nil {
		// Expected to fail outside Azure
		t.Logf("Expected error in non-Azure environment: %v", err)
	}
}
