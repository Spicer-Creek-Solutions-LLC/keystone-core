package registry

import (
	"context"
	"testing"
)

func TestNewExternalCredentialHelper(t *testing.T) {
	helper := NewExternalCredentialHelper("ecr-login")

	if helper == nil {
		t.Fatal("Expected helper to be non-nil")
	}
	if helper.helperName != "ecr-login" {
		t.Errorf("helperName = %q, want %q", helper.helperName, "ecr-login")
	}
}

func TestExternalCredentialHelper_BinaryName(t *testing.T) {
	tests := []struct {
		helperName string
		want       string
	}{
		{"ecr-login", "docker-credential-ecr-login"},
		{"gcloud", "docker-credential-gcloud"},
		{"osxkeychain", "docker-credential-osxkeychain"},
		{"secretservice", "docker-credential-secretservice"},
	}

	for _, tt := range tests {
		t.Run(tt.helperName, func(t *testing.T) {
			helper := NewExternalCredentialHelper(tt.helperName)
			if got := helper.binaryName(); got != tt.want {
				t.Errorf("binaryName() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestExternalCredentialHelper_IsAvailable(t *testing.T) {
	// Test with a helper that likely doesn't exist
	helper := NewExternalCredentialHelper("nonexistent-helper-xyz")
	if helper.IsAvailable() {
		t.Error("Expected IsAvailable to be false for nonexistent helper")
	}

	// Test with a common helper (may or may not exist)
	helper = NewExternalCredentialHelper("pass")
	// Just verify it doesn't panic
	_ = helper.IsAvailable()
}

func TestNewCredentialHelperRegistry(t *testing.T) {
	registry := NewCredentialHelperRegistry()

	if registry == nil {
		t.Fatal("Expected registry to be non-nil")
	}
	if registry.helpers == nil {
		t.Error("Expected helpers map to be initialized")
	}
}

func TestCredentialHelperRegistry_RegisterHelper(t *testing.T) {
	registry := NewCredentialHelperRegistry()
	helper := NewExternalCredentialHelper("test")

	registry.RegisterHelper("test.registry.io", helper)

	got := registry.GetHelper("test.registry.io")
	if got == nil {
		t.Error("Expected to get registered helper")
	}
}

func TestCredentialHelperRegistry_SetDefaultStore(t *testing.T) {
	registry := NewCredentialHelperRegistry()
	registry.SetDefaultStore("osxkeychain")

	if registry.defaultStore != "osxkeychain" {
		t.Errorf("defaultStore = %q, want %q", registry.defaultStore, "osxkeychain")
	}

	// Test that default store is used for unknown registries
	helper := registry.GetHelper("unknown.registry.io")
	if helper == nil {
		t.Error("Expected to get default store helper")
	}
}

func TestCredentialHelperRegistry_LoadFromDockerConfig(t *testing.T) {
	registry := NewCredentialHelperRegistry()

	config := &DockerConfig{
		CredsStore: "osxkeychain",
		CredHelpers: map[string]string{
			"gcr.io":                                      "gcloud",
			"123456789.dkr.ecr.us-west-2.amazonaws.com":   "ecr-login",
		},
	}

	registry.LoadFromDockerConfig(config)

	if registry.defaultStore != "osxkeychain" {
		t.Errorf("defaultStore = %q, want %q", registry.defaultStore, "osxkeychain")
	}

	// Check that helpers were registered
	if !registry.HasHelper("gcr.io") {
		t.Error("Expected gcr.io helper to be registered")
	}
	if !registry.HasHelper("123456789.dkr.ecr.us-west-2.amazonaws.com") {
		t.Error("Expected ECR helper to be registered")
	}
}

func TestCredentialHelperRegistry_GetHelper_Patterns(t *testing.T) {
	registry := NewCredentialHelperRegistry()

	// Register with wildcard pattern
	registry.RegisterHelper("*.gcr.io", NewExternalCredentialHelper("gcloud"))

	tests := []struct {
		registry string
		hasHelper bool
	}{
		{"us.gcr.io", true},
		{"eu.gcr.io", true},
		{"gcr.io", false}, // Wildcard doesn't match root
		{"docker.io", false},
	}

	for _, tt := range tests {
		t.Run(tt.registry, func(t *testing.T) {
			got := registry.GetHelper(tt.registry)
			hasHelper := got != nil
			if hasHelper != tt.hasHelper {
				t.Errorf("HasHelper(%q) = %v, want %v", tt.registry, hasHelper, tt.hasHelper)
			}
		})
	}
}

func TestMatchRegistryPattern(t *testing.T) {
	tests := []struct {
		pattern  string
		registry string
		want     bool
	}{
		{"*.gcr.io", "us.gcr.io", true},
		{"*.gcr.io", "eu.gcr.io", true},
		{"*.gcr.io", "gcr.io", false}, // No prefix
		{"gcr.io", "gcr.io", true},    // Exact match
		{"gcr.io", "us.gcr.io", false},
		{"docker*", "docker.io", true},
		{"docker*", "dockerhub.com", true},
	}

	for _, tt := range tests {
		name := tt.pattern + "_" + tt.registry
		t.Run(name, func(t *testing.T) {
			if got := matchRegistryPattern(tt.pattern, tt.registry); got != tt.want {
				t.Errorf("matchRegistryPattern(%q, %q) = %v, want %v", tt.pattern, tt.registry, got, tt.want)
			}
		})
	}
}

func TestCommonCredentialHelpers(t *testing.T) {
	expected := map[string]string{
		"ecr":           "ecr-login",
		"gcr":           "gcr",
		"gcloud":        "gcloud",
		"acr":           "acr-env",
		"osxkeychain":   "osxkeychain",
		"wincred":       "wincred",
		"secretservice": "secretservice",
		"pass":          "pass",
	}

	for key, want := range expected {
		if got := CommonCredentialHelpers[key]; got != want {
			t.Errorf("CommonCredentialHelpers[%q] = %q, want %q", key, got, want)
		}
	}
}

func TestGetCommonHelper(t *testing.T) {
	tests := []struct {
		helperType string
		wantNil    bool
	}{
		{"ecr", false},
		{"gcloud", false},
		{"nonexistent", true},
	}

	for _, tt := range tests {
		t.Run(tt.helperType, func(t *testing.T) {
			helper := GetCommonHelper(tt.helperType)
			isNil := helper == nil
			if isNil != tt.wantNil {
				t.Errorf("GetCommonHelper(%q) nil = %v, want %v", tt.helperType, isNil, tt.wantNil)
			}
		})
	}
}

func TestCredentialHelperRegistry_GetCredential_NoHelper(t *testing.T) {
	registry := NewCredentialHelperRegistry()
	ctx := context.Background()

	_, err := registry.GetCredential(ctx, "unknown.registry.io")
	if err == nil {
		t.Error("Expected error for registry with no helper")
	}
}
