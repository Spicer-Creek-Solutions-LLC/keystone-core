// Package registry provides container registry authentication support for Keystone.
package registry

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"k8s.io/client-go/kubernetes"
)

// CloudCredentialProvider is the interface for cloud-native credential providers.
type CloudCredentialProvider interface {
	// GetCredential retrieves credentials for a registry.
	GetCredential(ctx context.Context, registry string) (*Credential, error)
	// IsAvailable checks if this provider is available (e.g., running on the cloud).
	IsAvailable() bool
	// Type returns the registry type this provider handles.
	Type() Type
	// MatchesRegistry checks if this provider can handle the given registry.
	MatchesRegistry(registry string) bool
}

// CredentialResolver provides unified credential resolution from multiple sources.
type CredentialResolver struct {
	authManager      *AuthManager
	helperRegistry   *CredentialHelperRegistry
	cloudProviders   []CloudCredentialProvider
	k8sProvider      *K8sSecretCredentialProvider
	dockerConfigPath string

	// Cache for resolved credentials
	cache   map[string]*cachedCredential
	cacheMu sync.RWMutex

	// Configuration
	cacheTimeout    time.Duration
	enableCache     bool
	autoDetectCloud bool
}

// cachedCredential holds a credential with cache metadata.
type cachedCredential struct {
	credential *Credential
	cachedAt   time.Time
}

// ResolverOption configures the credential resolver.
type ResolverOption func(*CredentialResolver)

// WithAuthManager sets the auth manager.
func WithAuthManager(am *AuthManager) ResolverOption {
	return func(r *CredentialResolver) {
		r.authManager = am
	}
}

// WithHelperRegistry sets the credential helper registry.
func WithHelperRegistry(hr *CredentialHelperRegistry) ResolverOption {
	return func(r *CredentialResolver) {
		r.helperRegistry = hr
	}
}

// WithCloudProviders sets the cloud credential providers.
func WithCloudProviders(providers ...CloudCredentialProvider) ResolverOption {
	return func(r *CredentialResolver) {
		r.cloudProviders = providers
	}
}

// WithK8sClient sets up the Kubernetes secret provider.
func WithK8sClient(clientset kubernetes.Interface) ResolverOption {
	return func(r *CredentialResolver) {
		r.k8sProvider = NewK8sSecretCredentialProvider(clientset)
	}
}

// WithDockerConfigPath sets the Docker config path.
func WithDockerConfigPath(path string) ResolverOption {
	return func(r *CredentialResolver) {
		r.dockerConfigPath = path
	}
}

// WithCacheTimeout sets the credential cache timeout.
func WithCacheTimeout(timeout time.Duration) ResolverOption {
	return func(r *CredentialResolver) {
		r.cacheTimeout = timeout
	}
}

// WithCacheEnabled enables or disables credential caching.
func WithCacheEnabled(enabled bool) ResolverOption {
	return func(r *CredentialResolver) {
		r.enableCache = enabled
	}
}

// WithAutoDetectCloud enables automatic cloud provider detection.
func WithAutoDetectCloud(enabled bool) ResolverOption {
	return func(r *CredentialResolver) {
		r.autoDetectCloud = enabled
	}
}

// NewCredentialResolver creates a new credential resolver.
func NewCredentialResolver(opts ...ResolverOption) *CredentialResolver {
	r := &CredentialResolver{
		authManager:      NewAuthManager(),
		helperRegistry:   NewCredentialHelperRegistry(),
		cache:            make(map[string]*cachedCredential),
		cacheTimeout:     5 * time.Minute,
		enableCache:      true,
		autoDetectCloud:  true,
		dockerConfigPath: defaultDockerConfigPath(),
	}

	for _, opt := range opts {
		opt(r)
	}

	// Auto-detect and register cloud providers if enabled
	if r.autoDetectCloud {
		r.detectCloudProviders()
	}

	// Load Docker config if available
	r.loadDockerConfig()

	return r
}

// detectCloudProviders detects and registers available cloud providers.
func (r *CredentialResolver) detectCloudProviders() {
	// Try ECR
	ecr := NewECRCredentialProvider("")
	if ecr.IsAvailable() {
		r.cloudProviders = append(r.cloudProviders, ecr)
	}

	// Try GCR
	gcr := NewGCRCredentialProvider()
	if gcr.IsAvailable() {
		r.cloudProviders = append(r.cloudProviders, gcr)
	}

	// Try ACR
	acr := NewACRCredentialProvider()
	if acr.IsAvailable() {
		r.cloudProviders = append(r.cloudProviders, acr)
	}
}

// loadDockerConfig loads credentials from Docker config.json.
func (r *CredentialResolver) loadDockerConfig() {
	if r.dockerConfigPath == "" {
		return
	}

	data, err := os.ReadFile(r.dockerConfigPath)
	if err != nil {
		return // File doesn't exist or can't be read
	}

	var config DockerConfig
	if err := json.Unmarshal(data, &config); err != nil {
		return
	}

	// Load into auth manager
	_ = r.authManager.LoadDockerConfig(context.Background(), &config)

	// Load credential helpers
	r.helperRegistry.LoadFromDockerConfig(&config)
}

// Resolve retrieves credentials for a registry using all available sources.
func (r *CredentialResolver) Resolve(ctx context.Context, registry string) (*Credential, error) {
	// Check cache first
	if r.enableCache {
		if cred := r.getFromCache(registry); cred != nil {
			return cred, nil
		}
	}

	// Resolution order:
	// 1. Auth manager cache
	// 2. Credential helpers
	// 3. Cloud providers (auto-detected)
	// 4. Docker config.json
	// 5. K8s secrets (if configured)

	// 1. Check auth manager cache
	cred, err := r.authManager.GetCredential(ctx, registry)
	if err == nil && cred != nil {
		r.cacheCredential(registry, cred)
		return cred, nil
	}

	// 2. Try credential helpers
	if r.helperRegistry.HasHelper(registry) {
		cred, err := r.helperRegistry.GetCredential(ctx, registry)
		if err == nil && cred != nil {
			r.cacheCredential(registry, cred)
			return cred, nil
		}
	}

	// 3. Try cloud providers
	for _, provider := range r.cloudProviders {
		if provider.MatchesRegistry(registry) {
			cred, err := provider.GetCredential(ctx, registry)
			if err == nil && cred != nil {
				r.cacheCredential(registry, cred)
				return cred, nil
			}
		}
	}

	return nil, fmt.Errorf("no credentials found for registry %s", registry)
}

// ResolveWithK8sSecret retrieves credentials from a Kubernetes secret.
func (r *CredentialResolver) ResolveWithK8sSecret(ctx context.Context, registry, namespace, secretName string) (*Credential, error) {
	if r.k8sProvider == nil {
		return nil, fmt.Errorf("kubernetes provider not configured")
	}

	return r.k8sProvider.GetForRegistry(ctx, namespace, secretName, registry)
}

// GetDockerAuthConfig returns a Docker auth config string for use with docker pull --config.
func (r *CredentialResolver) GetDockerAuthConfig(ctx context.Context, image string) (string, error) {
	registry := extractRegistryFromImage(image)
	if registry == "" {
		registry = "docker.io" // Default to Docker Hub
	}

	cred, err := r.Resolve(ctx, registry)
	if err != nil {
		return "", err
	}

	return r.credentialToDockerAuthConfig(cred)
}

// GetDockerAuthConfigJSON returns the full Docker config.json content for authenticated pulls.
func (r *CredentialResolver) GetDockerAuthConfigJSON(ctx context.Context, image string) ([]byte, error) {
	registry := extractRegistryFromImage(image)
	if registry == "" {
		registry = "docker.io"
	}

	cred, err := r.Resolve(ctx, registry)
	if err != nil {
		return nil, err
	}

	config := DockerConfig{
		Auths: map[string]AuthConfig{
			registry: {
				Username: cred.Username,
				Password: cred.Password,
				Auth:     base64.StdEncoding.EncodeToString([]byte(cred.Username + ":" + cred.Password)),
			},
		},
	}

	return json.Marshal(config)
}

// credentialToDockerAuthConfig converts a credential to base64-encoded auth.
func (r *CredentialResolver) credentialToDockerAuthConfig(cred *Credential) (string, error) {
	if cred.Username == "" && cred.Password == "" && cred.Token == "" {
		return "", fmt.Errorf("credential has no authentication information")
	}

	password := cred.Password
	if password == "" {
		password = cred.Token
	}

	auth := base64.StdEncoding.EncodeToString([]byte(cred.Username + ":" + password))
	return auth, nil
}

// getFromCache retrieves a credential from cache if valid.
func (r *CredentialResolver) getFromCache(registry string) *Credential {
	r.cacheMu.RLock()
	defer r.cacheMu.RUnlock()

	cached, ok := r.cache[registry]
	if !ok {
		return nil
	}

	// Check if cache is expired
	if time.Since(cached.cachedAt) > r.cacheTimeout {
		return nil
	}

	// Check if credential is expired
	if cached.credential.IsExpired() {
		return nil
	}

	return cached.credential
}

// cacheCredential adds a credential to the cache.
func (r *CredentialResolver) cacheCredential(registry string, cred *Credential) {
	if !r.enableCache {
		return
	}

	r.cacheMu.Lock()
	defer r.cacheMu.Unlock()

	r.cache[registry] = &cachedCredential{
		credential: cred,
		cachedAt:   time.Now(),
	}
}

// ClearCache clears the credential cache.
func (r *CredentialResolver) ClearCache() {
	r.cacheMu.Lock()
	defer r.cacheMu.Unlock()
	r.cache = make(map[string]*cachedCredential)
}

// AddCloudProvider adds a cloud credential provider.
func (r *CredentialResolver) AddCloudProvider(provider CloudCredentialProvider) {
	r.cloudProviders = append(r.cloudProviders, provider)
}

// SetK8sProvider sets the Kubernetes secret provider.
func (r *CredentialResolver) SetK8sProvider(provider *K8sSecretCredentialProvider) {
	r.k8sProvider = provider
}

// extractRegistryFromImage extracts the registry from a container image reference.
func extractRegistryFromImage(image string) string {
	// Handle images with no registry (implicit docker.io)
	// e.g., "nginx:latest" or "library/nginx:latest"
	if !strings.Contains(image, "/") {
		return "docker.io"
	}

	// Split image into parts
	parts := strings.SplitN(image, "/", 2)
	firstPart := parts[0]

	// Check if first part looks like a registry (contains . or :)
	if strings.Contains(firstPart, ".") || strings.Contains(firstPart, ":") {
		return firstPart
	}

	// Otherwise it's probably a Docker Hub user/repo
	return "docker.io"
}

// defaultDockerConfigPath returns the default Docker config path.
func defaultDockerConfigPath() string {
	// Check DOCKER_CONFIG environment variable
	if dockerConfig := os.Getenv("DOCKER_CONFIG"); dockerConfig != "" {
		return filepath.Join(dockerConfig, "config.json")
	}

	// Default to ~/.docker/config.json
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}

	return filepath.Join(home, ".docker", "config.json")
}

// ResolverStats contains resolver statistics.
type ResolverStats struct {
	CloudProviders     int
	CachedCredentials  int
	HasHelperRegistry  bool
	HasK8sProvider     bool
	HasDockerConfig    bool
	AvailableProviders []string
}

// Stats returns resolver statistics.
func (r *CredentialResolver) Stats() *ResolverStats {
	r.cacheMu.RLock()
	cachedCount := len(r.cache)
	r.cacheMu.RUnlock()

	var providers []string
	for _, p := range r.cloudProviders {
		if p.IsAvailable() {
			providers = append(providers, string(p.Type()))
		}
	}

	return &ResolverStats{
		CloudProviders:     len(r.cloudProviders),
		CachedCredentials:  cachedCount,
		HasHelperRegistry:  r.helperRegistry != nil,
		HasK8sProvider:     r.k8sProvider != nil,
		HasDockerConfig:    r.dockerConfigPath != "" && fileExists(r.dockerConfigPath),
		AvailableProviders: providers,
	}
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}
