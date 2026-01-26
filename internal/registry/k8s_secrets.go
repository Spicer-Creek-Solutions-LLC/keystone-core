// Package registry provides container registry authentication support for Keystone.
package registry

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

// K8sSecretCredentialProvider loads credentials from Kubernetes secrets.
type K8sSecretCredentialProvider struct {
	clientset kubernetes.Interface
}

// NewK8sSecretCredentialProvider creates a new Kubernetes secret credential provider.
func NewK8sSecretCredentialProvider(clientset kubernetes.Interface) *K8sSecretCredentialProvider {
	return &K8sSecretCredentialProvider{
		clientset: clientset,
	}
}

// GetFromSecret retrieves credentials from a Kubernetes secret.
func (p *K8sSecretCredentialProvider) GetFromSecret(ctx context.Context, namespace, secretName string) ([]*Credential, error) {
	secret, err := p.clientset.CoreV1().Secrets(namespace).Get(ctx, secretName, metav1.GetOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to get secret %s/%s: %w", namespace, secretName, err)
	}

	return p.parseSecret(secret)
}

// GetFromServiceAccount retrieves credentials from a service account's imagePullSecrets.
func (p *K8sSecretCredentialProvider) GetFromServiceAccount(ctx context.Context, namespace, serviceAccountName string) ([]*Credential, error) {
	sa, err := p.clientset.CoreV1().ServiceAccounts(namespace).Get(ctx, serviceAccountName, metav1.GetOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to get service account %s/%s: %w", namespace, serviceAccountName, err)
	}

	var allCreds []*Credential

	for _, secretRef := range sa.ImagePullSecrets {
		creds, err := p.GetFromSecret(ctx, namespace, secretRef.Name)
		if err != nil {
			// Log warning but continue with other secrets
			continue
		}
		allCreds = append(allCreds, creds...)
	}

	return allCreds, nil
}

// GetForRegistry retrieves credentials for a specific registry from a secret.
func (p *K8sSecretCredentialProvider) GetForRegistry(ctx context.Context, namespace, secretName, registry string) (*Credential, error) {
	creds, err := p.GetFromSecret(ctx, namespace, secretName)
	if err != nil {
		return nil, err
	}

	// Find credential matching the registry
	for _, cred := range creds {
		if matchesRegistry(cred.Registry, registry) {
			return cred, nil
		}
	}

	return nil, fmt.Errorf("no credential found for registry %s in secret %s/%s", registry, namespace, secretName)
}

// parseSecret parses credentials from a Kubernetes secret.
func (p *K8sSecretCredentialProvider) parseSecret(secret *corev1.Secret) ([]*Credential, error) {
	switch secret.Type {
	case corev1.SecretTypeDockerConfigJson:
		return p.parseDockerConfigJSON(secret.Data[corev1.DockerConfigJsonKey])
	case corev1.SecretTypeDockercfg:
		return p.parseDockercfg(secret.Data[corev1.DockerConfigKey])
	default:
		return nil, fmt.Errorf("unsupported secret type: %s", secret.Type)
	}
}

// dockerConfigJSON represents the structure of .dockerconfigjson
type dockerConfigJSON struct {
	Auths map[string]dockerConfigEntry `json:"auths"`
}

// dockerConfigEntry represents a single registry entry
type dockerConfigEntry struct {
	Username string `json:"username"`
	Password string `json:"password"`
	Email    string `json:"email"`
	Auth     string `json:"auth"`
}

// parseDockerConfigJSON parses kubernetes.io/dockerconfigjson format.
func (p *K8sSecretCredentialProvider) parseDockerConfigJSON(data []byte) ([]*Credential, error) {
	if len(data) == 0 {
		return nil, fmt.Errorf("empty docker config data")
	}

	var config dockerConfigJSON
	if err := json.Unmarshal(data, &config); err != nil {
		return nil, fmt.Errorf("failed to parse docker config JSON: %w", err)
	}

	return p.parseAuths(config.Auths)
}

// parseDockercfg parses kubernetes.io/dockercfg format (legacy).
func (p *K8sSecretCredentialProvider) parseDockercfg(data []byte) ([]*Credential, error) {
	if len(data) == 0 {
		return nil, fmt.Errorf("empty dockercfg data")
	}

	var auths map[string]dockerConfigEntry
	if err := json.Unmarshal(data, &auths); err != nil {
		return nil, fmt.Errorf("failed to parse dockercfg: %w", err)
	}

	return p.parseAuths(auths)
}

// parseAuths converts auth entries to Credentials.
func (p *K8sSecretCredentialProvider) parseAuths(auths map[string]dockerConfigEntry) ([]*Credential, error) {
	var creds []*Credential

	for registry, entry := range auths {
		cred := &Credential{
			Type:     DetectRegistryType(registry),
			Registry: registry,
		}

		// Prefer explicit username/password over auth field
		if entry.Username != "" && entry.Password != "" {
			cred.Username = entry.Username
			cred.Password = entry.Password
		} else if entry.Auth != "" {
			// Decode base64 auth (format: username:password)
			decoded, err := base64.StdEncoding.DecodeString(entry.Auth)
			if err != nil {
				continue // Skip invalid entries
			}
			parts := strings.SplitN(string(decoded), ":", 2)
			if len(parts) == 2 {
				cred.Username = parts[0]
				cred.Password = parts[1]
			}
		}

		if cred.Username != "" || cred.Password != "" {
			creds = append(creds, cred)
		}
	}

	return creds, nil
}

// matchesRegistry checks if a credential registry matches a target registry.
func matchesRegistry(credRegistry, targetRegistry string) bool {
	// Normalize both registries
	credRegistry = normalizeRegistryURL(credRegistry)
	targetRegistry = normalizeRegistryURL(targetRegistry)

	// Exact match
	if credRegistry == targetRegistry {
		return true
	}

	// Handle Docker Hub special cases
	dockerHubAliases := []string{
		"docker.io",
		"index.docker.io",
		"registry-1.docker.io",
		"https://index.docker.io/v1/",
	}

	credIsDockerHub := false
	targetIsDockerHub := false

	for _, alias := range dockerHubAliases {
		if strings.Contains(credRegistry, alias) {
			credIsDockerHub = true
		}
		if strings.Contains(targetRegistry, alias) {
			targetIsDockerHub = true
		}
	}

	if credIsDockerHub && targetIsDockerHub {
		return true
	}

	// Check if target is a subpath of credential registry
	// e.g., "gcr.io" matches "gcr.io/project/image"
	return strings.HasPrefix(targetRegistry, credRegistry)
}

// normalizeRegistryURL normalizes a registry URL for comparison.
func normalizeRegistryURL(registry string) string {
	// Remove protocol
	registry = strings.TrimPrefix(registry, "https://")
	registry = strings.TrimPrefix(registry, "http://")

	// Remove trailing slashes
	registry = strings.TrimSuffix(registry, "/")

	// Remove /v1 or /v2 suffixes
	registry = strings.TrimSuffix(registry, "/v1")
	registry = strings.TrimSuffix(registry, "/v2")

	return strings.ToLower(registry)
}

// CreateDockerConfigSecret creates a Kubernetes secret with Docker config credentials.
func CreateDockerConfigSecret(name, namespace string, creds []*Credential) *corev1.Secret {
	auths := make(map[string]dockerConfigEntry)

	for _, cred := range creds {
		auth := base64.StdEncoding.EncodeToString([]byte(cred.Username + ":" + cred.Password))
		auths[cred.Registry] = dockerConfigEntry{
			Username: cred.Username,
			Password: cred.Password,
			Auth:     auth,
		}
	}

	config := dockerConfigJSON{Auths: auths}
	configData, _ := json.Marshal(config)

	return &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Type: corev1.SecretTypeDockerConfigJson,
		Data: map[string][]byte{
			corev1.DockerConfigJsonKey: configData,
		},
	}
}
