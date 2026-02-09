// Package statemgmt provides state management for Keystone.
package statemgmt

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/shawnbutts/keystone-core/internal/registry"
)

// RegistryAuthConfig holds configuration for authenticated registry access.
type RegistryAuthConfig struct {
	// Resolver is the credential resolver to use
	Resolver *registry.CredentialResolver

	// K8sSecretRef is a reference to a Kubernetes secret (namespace/name)
	K8sSecretRef string

	// DockerConfigPath overrides the default docker config path
	DockerConfigPath string
}

// ImagePuller handles authenticated container image pulls.
type ImagePuller struct {
	config *RegistryAuthConfig
}

// NewImagePuller creates a new image puller with auth support.
func NewImagePuller(config *RegistryAuthConfig) *ImagePuller {
	if config == nil {
		config = &RegistryAuthConfig{}
	}
	if config.Resolver == nil {
		config.Resolver = registry.NewCredentialResolver()
	}
	return &ImagePuller{config: config}
}

// PullImage pulls a container image with authentication if needed.
func (p *ImagePuller) PullImage(ctx context.Context, image, authMethod string) (string, error) {
	switch authMethod {
	case "", "none":
		return p.pullWithoutAuth(ctx, image)
	case "docker-config":
		return p.pullWithDockerConfig(ctx, image)
	case "cloud-auto":
		return p.pullWithCloudAuth(ctx, image)
	default:
		// Check for k8s secret reference (k8s:namespace/secret)
		if strings.HasPrefix(authMethod, "k8s:") {
			return p.pullWithK8sSecret(ctx, image, authMethod[4:])
		}
		return "", fmt.Errorf("unknown auth method: %s", authMethod)
	}
}

// pullWithoutAuth pulls without authentication (default behavior).
func (p *ImagePuller) pullWithoutAuth(ctx context.Context, image string) (string, error) {
	cmd := exec.CommandContext(ctx, "docker", "pull", image)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("failed to pull image: %w - %s", err, string(output))
	}
	return string(output), nil
}

// pullWithDockerConfig pulls using the default Docker config.
func (p *ImagePuller) pullWithDockerConfig(ctx context.Context, image string) (string, error) {
	// Just use docker pull - it will use ~/.docker/config.json
	return p.pullWithoutAuth(ctx, image)
}

// pullWithCloudAuth pulls using auto-detected cloud credentials.
func (p *ImagePuller) pullWithCloudAuth(ctx context.Context, image string) (string, error) {
	// Get credentials from resolver
	configJSON, err := p.config.Resolver.GetDockerAuthConfigJSON(ctx, image)
	if err != nil {
		// Fall back to no auth if resolution fails
		return p.pullWithoutAuth(ctx, image)
	}

	return p.pullWithTempConfig(ctx, image, configJSON)
}

// pullWithK8sSecret pulls using credentials from a Kubernetes secret.
func (p *ImagePuller) pullWithK8sSecret(ctx context.Context, image, secretRef string) (string, error) {
	parts := strings.SplitN(secretRef, "/", 2)
	if len(parts) != 2 {
		return "", fmt.Errorf("invalid k8s secret reference: %s (expected namespace/name)", secretRef)
	}

	namespace, secretName := parts[0], parts[1]

	// Get registry from image
	registryURL := extractRegistryFromImage(image)

	cred, err := p.config.Resolver.ResolveWithK8sSecret(ctx, registryURL, namespace, secretName)
	if err != nil {
		return "", fmt.Errorf("failed to get credentials from secret: %w", err)
	}

	// Build config JSON
	configJSON, err := credentialToConfigJSON(registryURL, cred)
	if err != nil {
		return "", err
	}

	return p.pullWithTempConfig(ctx, image, configJSON)
}

// pullWithTempConfig pulls using a temporary Docker config file.
func (p *ImagePuller) pullWithTempConfig(ctx context.Context, image string, configJSON []byte) (string, error) {
	// Create temp directory for Docker config
	tempDir, err := os.MkdirTemp("", "docker-config-*")
	if err != nil {
		return "", fmt.Errorf("failed to create temp directory: %w", err)
	}
	defer os.RemoveAll(tempDir)

	// Write config.json
	configPath := filepath.Join(tempDir, "config.json")
	if err := os.WriteFile(configPath, configJSON, 0o600); err != nil {
		return "", fmt.Errorf("failed to write config: %w", err)
	}

	// Pull with --config flag
	cmd := exec.CommandContext(ctx, "docker", "--config", tempDir, "pull", image)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("failed to pull image: %w - %s", err, string(output))
	}

	return string(output), nil
}

// PodmanPuller handles authenticated Podman image pulls.
type PodmanPuller struct {
	config *RegistryAuthConfig
}

// NewPodmanPuller creates a new Podman puller with auth support.
func NewPodmanPuller(config *RegistryAuthConfig) *PodmanPuller {
	if config == nil {
		config = &RegistryAuthConfig{}
	}
	if config.Resolver == nil {
		config.Resolver = registry.NewCredentialResolver()
	}
	return &PodmanPuller{config: config}
}

// PullImage pulls a container image with Podman.
func (p *PodmanPuller) PullImage(ctx context.Context, image, authMethod string) (string, error) {
	switch authMethod {
	case "", "none":
		return p.pullWithoutAuth(ctx, image)
	case "docker-config":
		return p.pullWithDockerConfig(ctx, image)
	case "cloud-auto":
		return p.pullWithCloudAuth(ctx, image)
	default:
		if strings.HasPrefix(authMethod, "k8s:") {
			return p.pullWithK8sSecret(ctx, image, authMethod[4:])
		}
		return "", fmt.Errorf("unknown auth method: %s", authMethod)
	}
}

func (p *PodmanPuller) pullWithoutAuth(ctx context.Context, image string) (string, error) {
	cmd := exec.CommandContext(ctx, "podman", "pull", image)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("failed to pull image: %w - %s", err, string(output))
	}
	return string(output), nil
}

func (p *PodmanPuller) pullWithDockerConfig(ctx context.Context, image string) (string, error) {
	return p.pullWithoutAuth(ctx, image)
}

func (p *PodmanPuller) pullWithCloudAuth(ctx context.Context, image string) (string, error) {
	configJSON, err := p.config.Resolver.GetDockerAuthConfigJSON(ctx, image)
	if err != nil {
		return p.pullWithoutAuth(ctx, image)
	}
	return p.pullWithTempConfig(ctx, image, configJSON)
}

func (p *PodmanPuller) pullWithK8sSecret(ctx context.Context, image, secretRef string) (string, error) {
	parts := strings.SplitN(secretRef, "/", 2)
	if len(parts) != 2 {
		return "", fmt.Errorf("invalid k8s secret reference: %s", secretRef)
	}

	namespace, secretName := parts[0], parts[1]
	registryURL := extractRegistryFromImage(image)

	cred, err := p.config.Resolver.ResolveWithK8sSecret(ctx, registryURL, namespace, secretName)
	if err != nil {
		return "", fmt.Errorf("failed to get credentials from secret: %w", err)
	}

	configJSON, err := credentialToConfigJSON(registryURL, cred)
	if err != nil {
		return "", err
	}

	return p.pullWithTempConfig(ctx, image, configJSON)
}

func (p *PodmanPuller) pullWithTempConfig(ctx context.Context, image string, configJSON []byte) (string, error) {
	// Create temp file for auth
	tempFile, err := os.CreateTemp("", "podman-auth-*.json")
	if err != nil {
		return "", fmt.Errorf("failed to create temp file: %w", err)
	}
	defer os.Remove(tempFile.Name())

	if _, err := tempFile.Write(configJSON); err != nil {
		tempFile.Close()
		return "", fmt.Errorf("failed to write auth file: %w", err)
	}
	tempFile.Close()

	// Podman uses --authfile flag
	cmd := exec.CommandContext(ctx, "podman", "pull", "--authfile", tempFile.Name(), image)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("failed to pull image: %w - %s", err, string(output))
	}

	return string(output), nil
}

// extractRegistryFromImage extracts the registry from an image reference.
func extractRegistryFromImage(image string) string {
	if !strings.Contains(image, "/") {
		return "docker.io"
	}

	parts := strings.SplitN(image, "/", 2)
	firstPart := parts[0]

	if strings.Contains(firstPart, ".") || strings.Contains(firstPart, ":") {
		return firstPart
	}

	return "docker.io"
}

// credentialToConfigJSON converts a credential to Docker config JSON.
func credentialToConfigJSON(registryURL string, cred *registry.Credential) ([]byte, error) {
	if cred == nil {
		return nil, fmt.Errorf("credential is nil")
	}

	password := cred.Password
	if password == "" {
		password = cred.Token
	}

	config := map[string]interface{}{
		"auths": map[string]interface{}{
			registryURL: map[string]string{
				"username": cred.Username,
				"password": password,
			},
		},
	}

	return jsonMarshal(config)
}

// jsonMarshal is a simple JSON marshaler.
func jsonMarshal(v interface{}) ([]byte, error) {
	return []byte(fmt.Sprintf(`{"auths":{%s}}`, formatAuths(v))), nil
}

func formatAuths(v interface{}) string {
	config, ok := v.(map[string]interface{})
	if !ok {
		return ""
	}
	auths, ok := config["auths"].(map[string]interface{})
	if !ok {
		return ""
	}

	var parts []string
	for registry, auth := range auths {
		authMap, ok := auth.(map[string]string)
		if !ok {
			continue
		}
		parts = append(parts, fmt.Sprintf(`%q:{"username":%q,"password":%q}`,
			registry, authMap["username"], authMap["password"]))
	}
	return strings.Join(parts, ",")
}

// GetAuthMethodFromDeclaration extracts the registry_auth parameter from a state declaration.
func GetAuthMethodFromDeclaration(decl *StateDeclaration) string {
	if decl == nil || decl.Parameters == nil {
		return ""
	}

	if auth, ok := decl.Parameters["registry_auth"].(string); ok {
		return auth
	}
	return ""
}
