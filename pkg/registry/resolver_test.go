package registry

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestNewCredentialResolver(t *testing.T) {
	resolver := NewCredentialResolver()

	if resolver == nil {
		t.Fatal("Expected resolver to be non-nil")
	}
	if resolver.authManager == nil {
		t.Error("Expected authManager to be non-nil")
	}
	if resolver.helperRegistry == nil {
		t.Error("Expected helperRegistry to be non-nil")
	}
	if resolver.cache == nil {
		t.Error("Expected cache to be initialized")
	}
}

func TestNewCredentialResolver_WithOptions(t *testing.T) {
	authManager := NewAuthManager()
	helperRegistry := NewCredentialHelperRegistry()

	resolver := NewCredentialResolver(
		WithAuthManager(authManager),
		WithHelperRegistry(helperRegistry),
		WithCacheTimeout(10*time.Minute),
		WithCacheEnabled(false),
		WithAutoDetectCloud(false),
	)

	if resolver.authManager != authManager {
		t.Error("Expected custom authManager")
	}
	if resolver.helperRegistry != helperRegistry {
		t.Error("Expected custom helperRegistry")
	}
	if resolver.cacheTimeout != 10*time.Minute {
		t.Errorf("cacheTimeout = %v, want %v", resolver.cacheTimeout, 10*time.Minute)
	}
	if resolver.enableCache {
		t.Error("Expected cache to be disabled")
	}
	if resolver.autoDetectCloud {
		t.Error("Expected auto-detect cloud to be disabled")
	}
}

func TestCredentialResolver_ExtractRegistryFromImage(t *testing.T) {
	tests := []struct {
		image string
		want  string
	}{
		{"nginx:latest", "docker.io"},
		{"library/nginx:latest", "docker.io"},
		{"myuser/myapp:v1", "docker.io"},
		{"gcr.io/myproject/myimage:v1", "gcr.io"},
		{"us.gcr.io/myproject/myimage:v1", "us.gcr.io"},
		{"123456789.dkr.ecr.us-west-2.amazonaws.com/myimage:v1", "123456789.dkr.ecr.us-west-2.amazonaws.com"},
		{"myregistry.azurecr.io/myimage:latest", "myregistry.azurecr.io"},
		{"ghcr.io/owner/repo:tag", "ghcr.io"},
		{"quay.io/org/image:tag", "quay.io"},
		{"localhost:5000/image:tag", "localhost:5000"},
	}

	for _, tt := range tests {
		t.Run(tt.image, func(t *testing.T) {
			if got := extractRegistryFromImage(tt.image); got != tt.want {
				t.Errorf("extractRegistryFromImage(%q) = %q, want %q", tt.image, got, tt.want)
			}
		})
	}
}

func TestCredentialResolver_ClearCache(t *testing.T) {
	resolver := NewCredentialResolver()

	// Add something to cache
	resolver.cacheCredential("test.registry.io", &Credential{
		Registry: "test.registry.io",
		Username: "user",
		Password: "pass",
	})

	// Verify cache has entry
	cred := resolver.getFromCache("test.registry.io")
	if cred == nil {
		t.Error("Expected cached credential")
	}

	// Clear cache
	resolver.ClearCache()

	// Verify cache is empty
	cred = resolver.getFromCache("test.registry.io")
	if cred != nil {
		t.Error("Expected cache to be cleared")
	}
}

func TestCredentialResolver_CacheExpiration(t *testing.T) {
	resolver := NewCredentialResolver(
		WithCacheTimeout(100 * time.Millisecond),
	)

	resolver.cacheCredential("test.registry.io", &Credential{
		Registry: "test.registry.io",
		Username: "user",
		Password: "pass",
	})

	// Verify cache has entry
	cred := resolver.getFromCache("test.registry.io")
	if cred == nil {
		t.Error("Expected cached credential")
	}

	// Wait for cache to expire
	time.Sleep(150 * time.Millisecond)

	// Verify cache entry is expired
	cred = resolver.getFromCache("test.registry.io")
	if cred != nil {
		t.Error("Expected cache entry to be expired")
	}
}

func TestCredentialResolver_CacheExpiredCredential(t *testing.T) {
	resolver := NewCredentialResolver()

	// Add expired credential to cache
	resolver.cacheCredential("test.registry.io", &Credential{
		Registry:  "test.registry.io",
		Username:  "user",
		Password:  "pass",
		ExpiresAt: time.Now().Add(-1 * time.Hour), // Expired
	})

	// Verify expired credential is not returned
	cred := resolver.getFromCache("test.registry.io")
	if cred != nil {
		t.Error("Expected expired credential to not be returned from cache")
	}
}

func TestCredentialResolver_Stats(t *testing.T) {
	resolver := NewCredentialResolver(
		WithAutoDetectCloud(false),
	)

	stats := resolver.Stats()

	if stats == nil {
		t.Fatal("Expected stats to be non-nil")
	}
	if stats.HasHelperRegistry != true {
		t.Error("Expected HasHelperRegistry to be true")
	}
	if stats.HasK8sProvider {
		t.Error("Expected HasK8sProvider to be false (not configured)")
	}
}

func TestCredentialResolver_AddCloudProvider(t *testing.T) {
	resolver := NewCredentialResolver(
		WithAutoDetectCloud(false),
	)

	initialCount := len(resolver.cloudProviders)

	provider := NewGCRCredentialProvider()
	resolver.AddCloudProvider(provider)

	if len(resolver.cloudProviders) != initialCount+1 {
		t.Errorf("cloudProviders count = %d, want %d", len(resolver.cloudProviders), initialCount+1)
	}
}

func TestDefaultDockerConfigPath(t *testing.T) {
	// Test with DOCKER_CONFIG set
	os.Setenv("DOCKER_CONFIG", "/custom/path")
	defer os.Unsetenv("DOCKER_CONFIG")

	path := defaultDockerConfigPath()
	expected := "/custom/path/config.json"
	if path != expected {
		t.Errorf("defaultDockerConfigPath() = %q, want %q", path, expected)
	}

	// Test without DOCKER_CONFIG
	os.Unsetenv("DOCKER_CONFIG")
	path = defaultDockerConfigPath()
	if path == "" {
		t.Error("Expected non-empty path")
	}
	if !filepath.IsAbs(path) {
		t.Error("Expected absolute path")
	}
}

func TestCredentialResolver_Resolve_NoCredentials(t *testing.T) {
	resolver := NewCredentialResolver(
		WithAutoDetectCloud(false),
	)

	ctx := context.Background()
	_, err := resolver.Resolve(ctx, "unknown.private.registry.io")

	if err == nil {
		t.Error("Expected error for unknown registry")
	}
}

func TestCredentialResolver_ResolveWithK8sSecret_NotConfigured(t *testing.T) {
	resolver := NewCredentialResolver()

	ctx := context.Background()
	_, err := resolver.ResolveWithK8sSecret(ctx, "gcr.io", "default", "mysecret")

	if err == nil {
		t.Error("Expected error when K8s provider not configured")
	}
}

func TestCredentialResolver_GetDockerAuthConfig_NoCredentials(t *testing.T) {
	resolver := NewCredentialResolver(
		WithAutoDetectCloud(false),
	)

	ctx := context.Background()
	_, err := resolver.GetDockerAuthConfig(ctx, "private.registry.io/image:tag")

	if err == nil {
		t.Error("Expected error for registry with no credentials")
	}
}

func TestFileExists(t *testing.T) {
	// Test with existing file
	tmpFile, err := os.CreateTemp("", "test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(tmpFile.Name())
	tmpFile.Close()

	if !fileExists(tmpFile.Name()) {
		t.Error("Expected fileExists to return true for existing file")
	}

	// Test with non-existing file
	if fileExists("/nonexistent/file/path") {
		t.Error("Expected fileExists to return false for non-existing file")
	}
}

func TestCredentialResolver_CredentialToDockerAuthConfig(t *testing.T) {
	resolver := NewCredentialResolver()

	tests := []struct {
		name    string
		cred    *Credential
		wantErr bool
	}{
		{
			name: "valid credential",
			cred: &Credential{
				Username: "user",
				Password: "pass",
			},
			wantErr: false,
		},
		{
			name: "token only",
			cred: &Credential{
				Username: "user",
				Token:    "token123",
			},
			wantErr: false,
		},
		{
			name: "empty credential",
			cred: &Credential{
				Username: "",
				Password: "",
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := resolver.credentialToDockerAuthConfig(tt.cred)
			if (err != nil) != tt.wantErr {
				t.Errorf("credentialToDockerAuthConfig() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}
