// Package statemgmt provides state management for Keystone.
package statemgmt

import (
	"context"
	"strings"
	"testing"

	"github.com/shawnbutts/keystone-core/pkg/registry"
)

// TestExtractRegistryFromImage tests the extractRegistryFromImage helper function.
func TestExtractRegistryFromImage(t *testing.T) {
	tests := []struct {
		name     string
		image    string
		expected string
	}{
		{
			name:     "simple image without registry",
			image:    "nginx",
			expected: "docker.io",
		},
		{
			name:     "library image without registry",
			image:    "library/nginx",
			expected: "docker.io",
		},
		{
			name:     "user image without registry",
			image:    "myuser/myapp",
			expected: "docker.io",
		},
		{
			name:     "gcr.io registry",
			image:    "gcr.io/my-project/my-app:latest",
			expected: "gcr.io",
		},
		{
			name:     "ecr registry",
			image:    "123456789.dkr.ecr.us-east-1.amazonaws.com/my-app",
			expected: "123456789.dkr.ecr.us-east-1.amazonaws.com",
		},
		{
			name:     "localhost with port",
			image:    "localhost:5000/my-app",
			expected: "localhost:5000",
		},
		{
			name:     "private registry with port",
			image:    "registry.example.com:5000/namespace/app:v1",
			expected: "registry.example.com:5000",
		},
		{
			name:     "quay.io registry",
			image:    "quay.io/organization/image",
			expected: "quay.io",
		},
		{
			name:     "ghcr.io registry",
			image:    "ghcr.io/owner/repo",
			expected: "ghcr.io",
		},
		{
			name:     "azure container registry",
			image:    "myregistry.azurecr.io/samples/nginx",
			expected: "myregistry.azurecr.io",
		},
		{
			name:     "nested path without registry",
			image:    "myorg/myteam/myapp",
			expected: "docker.io",
		},
		{
			name:     "registry with nested path",
			image:    "registry.example.com/org/team/app",
			expected: "registry.example.com",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := extractRegistryFromImage(tt.image)
			if result != tt.expected {
				t.Errorf("extractRegistryFromImage(%q) = %q, want %q", tt.image, result, tt.expected)
			}
		})
	}
}

// TestCredentialToConfigJSON tests the credentialToConfigJSON helper function.
func TestCredentialToConfigJSON(t *testing.T) {
	tests := []struct {
		name        string
		registryURL string
		cred        *registry.Credential
		expectErr   bool
		checkOutput func(t *testing.T, output []byte)
	}{
		{
			name:        "nil credential",
			registryURL: "docker.io",
			cred:        nil,
			expectErr:   true,
		},
		{
			name:        "credential with password",
			registryURL: "gcr.io",
			cred: &registry.Credential{
				Username: "user",
				Password: "pass123",
			},
			expectErr: false,
			checkOutput: func(t *testing.T, output []byte) {
				s := string(output)
				if s == "" {
					t.Error("expected non-empty output")
				}
				// Should contain registry, username, and password
				if !strings.Contains(s, "gcr.io") {
					t.Error("output should contain registry")
				}
				if !strings.Contains(s, "user") {
					t.Error("output should contain username")
				}
				if !strings.Contains(s, "pass123") {
					t.Error("output should contain password")
				}
			},
		},
		{
			name:        "credential with token instead of password",
			registryURL: "ghcr.io",
			cred: &registry.Credential{
				Username: "tokenuser",
				Token:    "ghp_token123",
			},
			expectErr: false,
			checkOutput: func(t *testing.T, output []byte) {
				s := string(output)
				if !strings.Contains(s, "ghcr.io") {
					t.Error("output should contain registry")
				}
				if !strings.Contains(s, "tokenuser") {
					t.Error("output should contain username")
				}
				if !strings.Contains(s, "ghp_token123") {
					t.Error("output should contain token as password")
				}
			},
		},
		{
			name:        "credential with both password and token prefers password",
			registryURL: "docker.io",
			cred: &registry.Credential{
				Username: "user",
				Password: "actualpass",
				Token:    "shouldnotuse",
			},
			expectErr: false,
			checkOutput: func(t *testing.T, output []byte) {
				s := string(output)
				if !strings.Contains(s, "actualpass") {
					t.Error("output should contain password")
				}
				if strings.Contains(s, "shouldnotuse") {
					t.Error("output should not contain token when password is set")
				}
			},
		},
		{
			name:        "empty username and password",
			registryURL: "registry.example.com",
			cred: &registry.Credential{
				Username: "",
				Password: "",
			},
			expectErr: false,
			checkOutput: func(t *testing.T, output []byte) {
				// Should still produce valid JSON structure
				if len(output) == 0 {
					t.Error("expected output even with empty credentials")
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			output, err := credentialToConfigJSON(tt.registryURL, tt.cred)
			if tt.expectErr {
				if err == nil {
					t.Error("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Errorf("unexpected error: %v", err)
				return
			}
			if tt.checkOutput != nil {
				tt.checkOutput(t, output)
			}
		})
	}
}

// TestJsonMarshal tests the jsonMarshal helper.
func TestJsonMarshal(t *testing.T) {
	tests := []struct {
		name  string
		input interface{}
	}{
		{
			name: "simple auth config",
			input: map[string]interface{}{
				"auths": map[string]interface{}{
					"docker.io": map[string]string{
						"username": "user",
						"password": "pass",
					},
				},
			},
		},
		{
			name:  "nil input",
			input: nil,
		},
		{
			name:  "wrong type input",
			input: "not a map",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			output, err := jsonMarshal(tt.input)
			if err != nil {
				t.Errorf("unexpected error: %v", err)
				return
			}
			// Should always return valid JSON structure
			if len(output) == 0 {
				t.Error("expected non-empty output")
			}
		})
	}
}

// TestFormatAuths tests the formatAuths helper.
func TestFormatAuths(t *testing.T) {
	tests := []struct {
		name     string
		input    interface{}
		expected string
	}{
		{
			name:     "nil input",
			input:    nil,
			expected: "",
		},
		{
			name:     "wrong type string",
			input:    "not a map",
			expected: "",
		},
		{
			name: "map without auths key",
			input: map[string]interface{}{
				"other": "value",
			},
			expected: "",
		},
		{
			name: "map with wrong auths type",
			input: map[string]interface{}{
				"auths": "not a map",
			},
			expected: "",
		},
		{
			name: "map with wrong auth entry type",
			input: map[string]interface{}{
				"auths": map[string]interface{}{
					"docker.io": "not a map[string]string",
				},
			},
			expected: "",
		},
		{
			name: "valid single auth",
			input: map[string]interface{}{
				"auths": map[string]interface{}{
					"docker.io": map[string]string{
						"username": "user",
						"password": "pass",
					},
				},
			},
			expected: `"docker.io":{"username":"user","password":"pass"}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := formatAuths(tt.input)
			if result != tt.expected {
				t.Errorf("formatAuths() = %q, want %q", result, tt.expected)
			}
		})
	}
}

// TestGetAuthMethodFromDeclaration tests the GetAuthMethodFromDeclaration helper.
func TestGetAuthMethodFromDeclaration(t *testing.T) {
	tests := []struct {
		name     string
		decl     *StateDeclaration
		expected string
	}{
		{
			name:     "nil declaration",
			decl:     nil,
			expected: "",
		},
		{
			name:     "nil parameters",
			decl:     &StateDeclaration{Parameters: nil},
			expected: "",
		},
		{
			name: "empty parameters",
			decl: &StateDeclaration{
				Parameters: map[string]interface{}{},
			},
			expected: "",
		},
		{
			name: "no registry_auth key",
			decl: &StateDeclaration{
				Parameters: map[string]interface{}{
					"image": "nginx:latest",
				},
			},
			expected: "",
		},
		{
			name: "registry_auth is not a string",
			decl: &StateDeclaration{
				Parameters: map[string]interface{}{
					"registry_auth": 123,
				},
			},
			expected: "",
		},
		{
			name: "registry_auth is empty string",
			decl: &StateDeclaration{
				Parameters: map[string]interface{}{
					"registry_auth": "",
				},
			},
			expected: "",
		},
		{
			name: "registry_auth is none",
			decl: &StateDeclaration{
				Parameters: map[string]interface{}{
					"registry_auth": "none",
				},
			},
			expected: "none",
		},
		{
			name: "registry_auth is docker-config",
			decl: &StateDeclaration{
				Parameters: map[string]interface{}{
					"registry_auth": "docker-config",
				},
			},
			expected: "docker-config",
		},
		{
			name: "registry_auth is cloud-auto",
			decl: &StateDeclaration{
				Parameters: map[string]interface{}{
					"registry_auth": "cloud-auto",
				},
			},
			expected: "cloud-auto",
		},
		{
			name: "registry_auth is k8s secret reference",
			decl: &StateDeclaration{
				Parameters: map[string]interface{}{
					"registry_auth": "k8s:default/my-secret",
				},
			},
			expected: "k8s:default/my-secret",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := GetAuthMethodFromDeclaration(tt.decl)
			if result != tt.expected {
				t.Errorf("GetAuthMethodFromDeclaration() = %q, want %q", result, tt.expected)
			}
		})
	}
}

// TestNewImagePuller tests the ImagePuller constructor.
func TestNewImagePuller(t *testing.T) {
	t.Run("nil config creates defaults", func(t *testing.T) {
		p := NewImagePuller(nil)
		if p == nil {
			t.Fatal("expected non-nil puller")
		}
		if p.config == nil {
			t.Fatal("expected non-nil config")
		}
		if p.config.Resolver == nil {
			t.Error("expected non-nil resolver")
		}
	})

	t.Run("config with nil resolver creates default resolver", func(t *testing.T) {
		config := &RegistryAuthConfig{
			DockerConfigPath: "/custom/path",
		}
		p := NewImagePuller(config)
		if p.config.Resolver == nil {
			t.Error("expected non-nil resolver")
		}
		if p.config.DockerConfigPath != "/custom/path" {
			t.Error("expected custom path to be preserved")
		}
	})

	t.Run("config with resolver preserves it", func(t *testing.T) {
		resolver := registry.NewCredentialResolver()
		config := &RegistryAuthConfig{
			Resolver: resolver,
		}
		p := NewImagePuller(config)
		if p.config.Resolver != resolver {
			t.Error("expected resolver to be preserved")
		}
	})
}

// TestNewPodmanPuller tests the PodmanPuller constructor.
func TestNewPodmanPuller(t *testing.T) {
	t.Run("nil config creates defaults", func(t *testing.T) {
		p := NewPodmanPuller(nil)
		if p == nil {
			t.Fatal("expected non-nil puller")
		}
		if p.config == nil {
			t.Fatal("expected non-nil config")
		}
		if p.config.Resolver == nil {
			t.Error("expected non-nil resolver")
		}
	})

	t.Run("config with nil resolver creates default resolver", func(t *testing.T) {
		config := &RegistryAuthConfig{
			K8sSecretRef: "default/my-secret",
		}
		p := NewPodmanPuller(config)
		if p.config.Resolver == nil {
			t.Error("expected non-nil resolver")
		}
		if p.config.K8sSecretRef != "default/my-secret" {
			t.Error("expected k8s secret ref to be preserved")
		}
	})

	t.Run("config with resolver preserves it", func(t *testing.T) {
		resolver := registry.NewCredentialResolver()
		config := &RegistryAuthConfig{
			Resolver: resolver,
		}
		p := NewPodmanPuller(config)
		if p.config.Resolver != resolver {
			t.Error("expected resolver to be preserved")
		}
	})
}

// TestImagePullerPullImageAuthMethodRouting tests auth method routing without actual pulls.
func TestImagePullerPullImageAuthMethodRouting(t *testing.T) {
	// These tests verify error handling and routing logic
	// without requiring docker to be installed

	p := NewImagePuller(nil)
	ctx := context.Background()

	t.Run("unknown auth method returns error", func(t *testing.T) {
		_, err := p.PullImage(ctx, "nginx:latest", "unknown-method")
		if err == nil {
			t.Error("expected error for unknown auth method")
		}
		if !strings.Contains(err.Error(), "unknown auth method") {
			t.Errorf("expected 'unknown auth method' in error, got: %v", err)
		}
	})

	t.Run("invalid k8s secret reference returns error", func(t *testing.T) {
		_, err := p.PullImage(ctx, "nginx:latest", "k8s:invalid-no-slash")
		if err == nil {
			t.Error("expected error for invalid k8s secret reference")
		}
		if !strings.Contains(err.Error(), "invalid k8s secret reference") {
			t.Errorf("expected 'invalid k8s secret reference' in error, got: %v", err)
		}
	})

	t.Run("empty k8s namespace returns error", func(t *testing.T) {
		_, err := p.PullImage(ctx, "nginx:latest", "k8s:/secret")
		if err == nil {
			t.Error("expected error for empty namespace")
		}
	})
}

// TestPodmanPullerPullImageAuthMethodRouting tests auth method routing for Podman.
func TestPodmanPullerPullImageAuthMethodRouting(t *testing.T) {
	p := NewPodmanPuller(nil)
	ctx := context.Background()

	t.Run("unknown auth method returns error", func(t *testing.T) {
		_, err := p.PullImage(ctx, "nginx:latest", "custom-unknown")
		if err == nil {
			t.Error("expected error for unknown auth method")
		}
		if !strings.Contains(err.Error(), "unknown auth method") {
			t.Errorf("expected 'unknown auth method' in error, got: %v", err)
		}
	})

	t.Run("invalid k8s secret reference returns error", func(t *testing.T) {
		_, err := p.PullImage(ctx, "nginx:latest", "k8s:no-namespace")
		if err == nil {
			t.Error("expected error for invalid k8s secret reference")
		}
		if !strings.Contains(err.Error(), "invalid k8s secret reference") {
			t.Errorf("expected 'invalid k8s secret reference' in error, got: %v", err)
		}
	})
}

// TestRegistryAuthConfig tests the RegistryAuthConfig struct.
func TestRegistryAuthConfig(t *testing.T) {
	t.Run("default values", func(t *testing.T) {
		config := &RegistryAuthConfig{}
		if config.Resolver != nil {
			t.Error("expected nil resolver by default")
		}
		if config.K8sSecretRef != "" {
			t.Error("expected empty K8sSecretRef by default")
		}
		if config.DockerConfigPath != "" {
			t.Error("expected empty DockerConfigPath by default")
		}
	})

	t.Run("all fields set", func(t *testing.T) {
		resolver := registry.NewCredentialResolver()
		config := &RegistryAuthConfig{
			Resolver:         resolver,
			K8sSecretRef:     "namespace/secret",
			DockerConfigPath: "/home/user/.docker/config.json",
		}
		if config.Resolver != resolver {
			t.Error("resolver not set correctly")
		}
		if config.K8sSecretRef != "namespace/secret" {
			t.Error("K8sSecretRef not set correctly")
		}
		if config.DockerConfigPath != "/home/user/.docker/config.json" {
			t.Error("DockerConfigPath not set correctly")
		}
	})
}

// TestK8sSecretReferenceValidation tests validation of k8s: auth method.
func TestK8sSecretReferenceValidation(t *testing.T) {
	tests := []struct {
		name      string
		secretRef string
		wantErr   bool
	}{
		{
			name:      "valid reference",
			secretRef: "k8s:default/my-registry-secret",
			wantErr:   false, // Will fail on resolution, not parsing
		},
		{
			name:      "valid reference with dashes",
			secretRef: "k8s:my-namespace/my-secret-name",
			wantErr:   false,
		},
		{
			name:      "no slash",
			secretRef: "k8s:invalid",
			wantErr:   true,
		},
		{
			name:      "empty after k8s prefix",
			secretRef: "k8s:",
			wantErr:   true,
		},
	}

	p := NewImagePuller(nil)
	ctx := context.Background()

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := p.PullImage(ctx, "nginx:latest", tt.secretRef)
			// All will return errors since we can't resolve secrets in tests
			// but the error message differs based on validation vs resolution
			if err == nil && tt.wantErr {
				t.Error("expected validation error")
			}
			if tt.wantErr && err != nil {
				if !strings.Contains(err.Error(), "invalid k8s secret reference") {
					t.Errorf("expected validation error, got resolution error: %v", err)
				}
			}
		})
	}
}

// TestExtractRegistryEdgeCases tests edge cases in registry extraction.
func TestExtractRegistryEdgeCases(t *testing.T) {
	tests := []struct {
		name     string
		image    string
		expected string
	}{
		{
			name:     "empty string",
			image:    "",
			expected: "docker.io",
		},
		{
			name:     "just a tag",
			image:    ":latest",
			expected: "docker.io",
		},
		{
			name:     "sha256 digest",
			image:    "nginx@sha256:abc123",
			expected: "docker.io",
		},
		{
			name:     "registry with sha256 digest",
			image:    "gcr.io/project/image@sha256:abc123",
			expected: "gcr.io",
		},
		{
			name:     "ip address registry",
			image:    "192.168.1.100:5000/app",
			expected: "192.168.1.100:5000",
		},
		{
			name:     "registry with only dots",
			image:    "my.registry/image",
			expected: "my.registry",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := extractRegistryFromImage(tt.image)
			if result != tt.expected {
				t.Errorf("extractRegistryFromImage(%q) = %q, want %q", tt.image, result, tt.expected)
			}
		})
	}
}
