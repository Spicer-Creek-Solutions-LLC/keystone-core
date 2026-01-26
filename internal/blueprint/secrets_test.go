package blueprint

import (
	"testing"
)

func TestParseSecretReference(t *testing.T) {
	tests := []struct {
		name        string
		input       string
		wantPath    string
		wantBackend string
		wantVersion string
		wantErr     bool
	}{
		{
			name:     "simple path",
			input:    "database/password",
			wantPath: "database/password",
		},
		{
			name:        "with backend",
			input:       "vault:database/password",
			wantPath:    "database/password",
			wantBackend: "vault",
		},
		{
			name:        "with version",
			input:       "database/password@v2",
			wantPath:    "database/password",
			wantVersion: "v2",
		},
		{
			name:        "with backend and version",
			input:       "vault:database/password@v2",
			wantPath:    "database/password",
			wantBackend: "vault",
			wantVersion: "v2",
		},
		{
			name:     "nested path",
			input:    "apps/myapp/secrets/api-key",
			wantPath: "apps/myapp/secrets/api-key",
		},
		{
			name:        "k8s backend",
			input:       "k8s:namespace/secret-name",
			wantPath:    "namespace/secret-name",
			wantBackend: "k8s",
		},
		{
			name:    "empty reference",
			input:   "",
			wantErr: true,
		},
		{
			name:    "whitespace only",
			input:   "   ",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ref, err := ParseSecretReference(tt.input)

			if tt.wantErr {
				if err == nil {
					t.Errorf("ParseSecretReference(%q) expected error, got nil", tt.input)
				}
				return
			}

			if err != nil {
				t.Errorf("ParseSecretReference(%q) unexpected error: %v", tt.input, err)
				return
			}

			if ref.Path != tt.wantPath {
				t.Errorf("Path = %q, want %q", ref.Path, tt.wantPath)
			}
			if ref.Backend != tt.wantBackend {
				t.Errorf("Backend = %q, want %q", ref.Backend, tt.wantBackend)
			}
			if ref.Version != tt.wantVersion {
				t.Errorf("Version = %q, want %q", ref.Version, tt.wantVersion)
			}
		})
	}
}

func TestSecretReferenceString(t *testing.T) {
	tests := []struct {
		name     string
		ref      SecretReference
		expected string
	}{
		{
			name:     "simple path",
			ref:      SecretReference{Path: "database/password"},
			expected: "database/password",
		},
		{
			name:     "with backend",
			ref:      SecretReference{Path: "database/password", Backend: "vault"},
			expected: "vault:database/password",
		},
		{
			name:     "with version",
			ref:      SecretReference{Path: "database/password", Version: "v2"},
			expected: "database/password@v2",
		},
		{
			name:     "with backend and version",
			ref:      SecretReference{Path: "database/password", Backend: "vault", Version: "v2"},
			expected: "vault:database/password@v2",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.ref.String()
			if result != tt.expected {
				t.Errorf("String() = %q, want %q", result, tt.expected)
			}
		})
	}
}

func TestInMemorySecretResolver(t *testing.T) {
	resolver := NewInMemorySecretResolver()

	// Set a secret
	resolver.SetSecret("database/password", "secret123")

	// Test Resolve
	value, err := resolver.Resolve("database/password")
	if err != nil {
		t.Errorf("Resolve() unexpected error: %v", err)
	}
	if value != "secret123" {
		t.Errorf("Resolve() = %q, want %q", value, "secret123")
	}

	// Test Exists
	exists, err := resolver.Exists("database/password")
	if err != nil {
		t.Errorf("Exists() unexpected error: %v", err)
	}
	if !exists {
		t.Error("Exists() = false, want true")
	}

	// Test non-existent secret
	exists, err = resolver.Exists("nonexistent")
	if err != nil {
		t.Errorf("Exists() unexpected error: %v", err)
	}
	if exists {
		t.Error("Exists() = true, want false")
	}

	_, err = resolver.Resolve("nonexistent")
	if err == nil {
		t.Error("Resolve() expected error for non-existent secret")
	}
}

func TestInMemorySecretResolverVersions(t *testing.T) {
	resolver := NewInMemorySecretResolver()

	// Set versioned secrets
	resolver.SetSecretVersion("api/key", "v1", "key-v1")
	resolver.SetSecretVersion("api/key", "v2", "key-v2")

	// Test latest version
	value, err := resolver.Resolve("api/key")
	if err != nil {
		t.Errorf("Resolve() unexpected error: %v", err)
	}
	if value != "key-v2" {
		t.Errorf("Resolve() = %q, want %q (latest)", value, "key-v2")
	}

	// Test specific version
	value, err = resolver.ResolveWithVersion("api/key", "v1")
	if err != nil {
		t.Errorf("ResolveWithVersion() unexpected error: %v", err)
	}
	if value != "key-v1" {
		t.Errorf("ResolveWithVersion() = %q, want %q", value, "key-v1")
	}

	// Test non-existent version
	_, err = resolver.ResolveWithVersion("api/key", "v999")
	if err == nil {
		t.Error("ResolveWithVersion() expected error for non-existent version")
	}
}

func TestMultiBackendResolver(t *testing.T) {
	// Create backends
	vaultResolver := NewInMemorySecretResolver()
	vaultResolver.SetSecret("database/password", "vault-secret")

	envResolver := NewInMemorySecretResolver()
	envResolver.SetSecret("api/key", "env-secret")

	// Create multi-backend resolver
	resolver := NewMultiBackendResolver()
	resolver.RegisterBackend("vault", vaultResolver)
	resolver.RegisterBackend("env", envResolver)
	resolver.SetDefaultBackend("vault")

	// Test default backend
	ref := &SecretReference{Path: "database/password"}
	value, err := resolver.Resolve(ref)
	if err != nil {
		t.Errorf("Resolve() unexpected error: %v", err)
	}
	if value != "vault-secret" {
		t.Errorf("Resolve() = %q, want %q", value, "vault-secret")
	}

	// Test explicit backend
	ref = &SecretReference{Path: "api/key", Backend: "env"}
	value, err = resolver.Resolve(ref)
	if err != nil {
		t.Errorf("Resolve() unexpected error: %v", err)
	}
	if value != "env-secret" {
		t.Errorf("Resolve() = %q, want %q", value, "env-secret")
	}

	// Test unknown backend
	ref = &SecretReference{Path: "test", Backend: "unknown"}
	_, err = resolver.Resolve(ref)
	if err == nil {
		t.Error("Resolve() expected error for unknown backend")
	}
}

func TestSecretParameterProcessor(t *testing.T) {
	// Set up resolver
	memResolver := NewInMemorySecretResolver()
	memResolver.SetSecret("database/password", "db-pass-123")
	memResolver.SetSecret("api/key", "api-key-456")

	resolver := NewMultiBackendResolver()
	resolver.RegisterBackend("default", memResolver)
	resolver.SetDefaultBackend("default")

	processor := NewSecretParameterProcessor(resolver)

	// Test processing parameters with secrets
	params := map[string]interface{}{
		"normal_value": "hello",
		"secret_value": "!secret database/password",
		"nested": map[string]interface{}{
			"api_key": "!secret api/key",
			"regular": "world",
		},
		"list": []interface{}{
			"item1",
			"!secret database/password",
		},
	}

	result, err := processor.ProcessParameters(params)
	if err != nil {
		t.Fatalf("ProcessParameters() unexpected error: %v", err)
	}

	// Check normal value unchanged
	if result["normal_value"] != "hello" {
		t.Errorf("normal_value = %v, want %q", result["normal_value"], "hello")
	}

	// Check secret resolved
	if result["secret_value"] != "db-pass-123" {
		t.Errorf("secret_value = %v, want %q", result["secret_value"], "db-pass-123")
	}

	// Check nested secret resolved
	nested, ok := result["nested"].(map[string]interface{})
	if !ok {
		t.Fatalf("nested is not a map")
	}
	if nested["api_key"] != "api-key-456" {
		t.Errorf("nested.api_key = %v, want %q", nested["api_key"], "api-key-456")
	}
	if nested["regular"] != "world" {
		t.Errorf("nested.regular = %v, want %q", nested["regular"], "world")
	}

	// Check list with secret resolved
	list, ok := result["list"].([]interface{})
	if !ok {
		t.Fatalf("list is not a slice")
	}
	if list[0] != "item1" {
		t.Errorf("list[0] = %v, want %q", list[0], "item1")
	}
	if list[1] != "db-pass-123" {
		t.Errorf("list[1] = %v, want %q", list[1], "db-pass-123")
	}
}

func TestCollectSecretReferences(t *testing.T) {
	params := map[string]interface{}{
		"normal": "value",
		"secret1": "!secret database/password",
		"nested": map[string]interface{}{
			"secret2": "!secret api/key",
		},
		"list": []interface{}{
			"!secret list/secret",
		},
	}

	refs := CollectSecretReferences(params)

	if len(refs) != 3 {
		t.Errorf("CollectSecretReferences() returned %d refs, want 3", len(refs))
	}

	// Verify the paths are collected
	paths := make(map[string]bool)
	for _, ref := range refs {
		paths[ref.Path] = true
	}

	expected := []string{"database/password", "api/key", "list/secret"}
	for _, path := range expected {
		if !paths[path] {
			t.Errorf("CollectSecretReferences() missing path %q", path)
		}
	}
}

func TestValidateSecretReferences(t *testing.T) {
	// Set up resolver with some secrets
	memResolver := NewInMemorySecretResolver()
	memResolver.SetSecret("database/password", "secret")

	resolver := NewMultiBackendResolver()
	resolver.RegisterBackend("default", memResolver)
	resolver.SetDefaultBackend("default")

	// Test valid references
	validParams := map[string]interface{}{
		"secret": "!secret database/password",
	}

	err := ValidateSecretReferences(validParams, resolver)
	if err != nil {
		t.Errorf("ValidateSecretReferences() unexpected error: %v", err)
	}

	// Test invalid reference (secret doesn't exist)
	invalidParams := map[string]interface{}{
		"secret": "!secret nonexistent/secret",
	}

	err = ValidateSecretReferences(invalidParams, resolver)
	if err == nil {
		t.Error("ValidateSecretReferences() expected error for non-existent secret")
	}
}

func TestEnvironmentSecretResolver(t *testing.T) {
	// Create a mock environment getter
	envVars := map[string]string{
		"KSCORE_SECRET_DATABASE_PASSWORD": "env-db-pass",
		"KSCORE_SECRET_API_KEY":           "env-api-key",
	}

	getter := func(name string) string {
		return envVars[name]
	}

	resolver := NewEnvironmentSecretResolver(getter)

	// Test resolving a secret
	value, err := resolver.Resolve("database/password")
	if err != nil {
		t.Errorf("Resolve() unexpected error: %v", err)
	}
	if value != "env-db-pass" {
		t.Errorf("Resolve() = %q, want %q", value, "env-db-pass")
	}

	// Test with dashes and dots
	envVars["KSCORE_SECRET_MY_APP_API_KEY"] = "my-app-key"
	value, err = resolver.Resolve("my-app/api.key")
	if err != nil {
		t.Errorf("Resolve() unexpected error: %v", err)
	}
	if value != "my-app-key" {
		t.Errorf("Resolve() = %q, want %q", value, "my-app-key")
	}

	// Test non-existent env var
	_, err = resolver.Resolve("nonexistent/secret")
	if err == nil {
		t.Error("Resolve() expected error for non-existent env var")
	}

	// Test Exists
	exists, err := resolver.Exists("database/password")
	if err != nil {
		t.Errorf("Exists() unexpected error: %v", err)
	}
	if !exists {
		t.Error("Exists() = false, want true")
	}

	exists, err = resolver.Exists("nonexistent")
	if err != nil {
		t.Errorf("Exists() unexpected error: %v", err)
	}
	if exists {
		t.Error("Exists() = true, want false")
	}
}

func TestEnvironmentSecretResolverCustomPrefix(t *testing.T) {
	envVars := map[string]string{
		"MYAPP_DB_PASSWORD": "custom-pass",
	}

	getter := func(name string) string {
		return envVars[name]
	}

	resolver := NewEnvironmentSecretResolver(getter)
	resolver.SetPrefix("MYAPP")

	value, err := resolver.Resolve("db/password")
	if err != nil {
		t.Errorf("Resolve() unexpected error: %v", err)
	}
	if value != "custom-pass" {
		t.Errorf("Resolve() = %q, want %q", value, "custom-pass")
	}
}

func TestIsSecretReference(t *testing.T) {
	tests := []struct {
		name     string
		value    interface{}
		expected bool
	}{
		{
			name:     "!secret prefix",
			value:    "!secret database/password",
			expected: true,
		},
		{
			name:     "secret: prefix",
			value:    "secret:database/password",
			expected: true,
		},
		{
			name:     "normal string",
			value:    "just a value",
			expected: false,
		},
		{
			name:     "number",
			value:    123,
			expected: false,
		},
		{
			name:     "nil",
			value:    nil,
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := IsSecretReference(tt.value)
			if result != tt.expected {
				t.Errorf("IsSecretReference(%v) = %v, want %v", tt.value, result, tt.expected)
			}
		})
	}
}
