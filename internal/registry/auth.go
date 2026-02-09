// Package registry provides container registry authentication support for Keystone.
package registry

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

// Common errors.
var (
	ErrInvalidCredentials  = errors.New("invalid credentials")
	ErrTokenExpired        = errors.New("token expired")
	ErrUnsupportedRegistry = errors.New("unsupported registry type")
	ErrAuthFailed          = errors.New("authentication failed")
)

// Type represents the type of container registry.
type Type string

const (
	// TypeDocker represents Docker Hub.
	TypeDocker Type = "docker"
	// TypeECR represents AWS Elastic Container Registry.
	TypeECR Type = "ecr"
	// TypeGCR represents Google Container Registry.
	TypeGCR Type = "gcr"
	// TypeACR represents Azure Container Registry.
	TypeACR Type = "acr"
	// TypeGitHub represents GitHub Container Registry.
	TypeGitHub Type = "ghcr"
	// TypeQuay represents Quay.io registry.
	TypeQuay Type = "quay"
	// TypeGeneric represents a generic OCI registry.
	TypeGeneric Type = "generic"
)

// Credential represents registry authentication credentials.
type Credential struct {
	Type         Type `json:"type"`
	Registry     string       `json:"registry"`
	Username     string       `json:"username,omitempty"`
	Password     string       `json:"password,omitempty"`
	Token        string       `json:"token,omitempty"`
	RefreshToken string       `json:"refreshToken,omitempty"`
	ExpiresAt    time.Time    `json:"expiresAt,omitempty"`

	// Cloud provider specific
	Region         string `json:"region,omitempty"`         // AWS ECR region
	AccountID      string `json:"accountId,omitempty"`      // AWS account ID
	ProjectID      string `json:"projectId,omitempty"`      // GCP project ID
	TenantID       string `json:"tenantId,omitempty"`       // Azure tenant ID
	SubscriptionID string `json:"subscriptionId,omitempty"` // Azure subscription
}

// IsExpired returns true if the credential has expired.
func (c *Credential) IsExpired() bool {
	if c.ExpiresAt.IsZero() {
		return false
	}
	return time.Now().After(c.ExpiresAt)
}

// AuthConfig represents Docker-style authentication configuration.
type AuthConfig struct {
	Username      string `json:"username,omitempty"`
	Password      string `json:"password,omitempty"`
	Auth          string `json:"auth,omitempty"`
	Email         string `json:"email,omitempty"`
	ServerAddress string `json:"serveraddress,omitempty"`
	IdentityToken string `json:"identitytoken,omitempty"`
	RegistryToken string `json:"registrytoken,omitempty"`
}

// DockerConfig represents a Docker config.json file structure.
type DockerConfig struct {
	Auths       map[string]AuthConfig `json:"auths,omitempty"`
	CredsStore  string                `json:"credsStore,omitempty"`
	CredHelpers map[string]string     `json:"credHelpers,omitempty"`
}

// TokenResponse represents an OAuth2 token response from a registry.
type TokenResponse struct {
	Token        string `json:"token"`
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token,omitempty"`
	ExpiresIn    int    `json:"expires_in,omitempty"`
	IssuedAt     string `json:"issued_at,omitempty"`
}

// Authenticator provides registry authentication services.
type Authenticator interface {
	// Authenticate authenticates with the registry and returns a credential.
	Authenticate(ctx context.Context, cred *Credential) (*Credential, error)
	// GetAuthConfig returns Docker-style AuthConfig for the credential.
	GetAuthConfig(ctx context.Context, cred *Credential) (*AuthConfig, error)
	// Refresh refreshes an expired credential.
	Refresh(ctx context.Context, cred *Credential) (*Credential, error)
	// Type returns the registry type this authenticator handles.
	Type() Type
}

// AuthManager manages registry authentication.
type AuthManager struct {
	authenticators map[Type]Authenticator
	credentials    map[string]*Credential
	httpClient     *http.Client
	mu             sync.RWMutex
	listeners      []AuthEventListener
}

// AuthEvent represents an authentication event.
type AuthEvent struct {
	Type         string    `json:"type"`
	Registry     string    `json:"registry"`
	RegistryType Type      `json:"registryType"`
	Success      bool      `json:"success"`
	Error        string    `json:"error,omitempty"`
	Timestamp    time.Time `json:"timestamp"`
}

// AuthEventListener is called when authentication events occur.
type AuthEventListener func(*AuthEvent)

// NewAuthManager creates a new authentication manager.
func NewAuthManager() *AuthManager {
	am := &AuthManager{
		authenticators: make(map[Type]Authenticator),
		credentials:    make(map[string]*Credential),
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}

	// Register default authenticators
	am.RegisterAuthenticator(NewDockerAuthenticator(am.httpClient))
	am.RegisterAuthenticator(NewECRAuthenticator(am.httpClient))
	am.RegisterAuthenticator(NewGCRAuthenticator(am.httpClient))
	am.RegisterAuthenticator(NewACRAuthenticator(am.httpClient))
	am.RegisterAuthenticator(NewGitHubAuthenticator(am.httpClient))
	am.RegisterAuthenticator(NewQuayAuthenticator(am.httpClient))
	am.RegisterAuthenticator(NewGenericAuthenticator(am.httpClient))

	return am
}

// RegisterAuthenticator registers an authenticator for a registry type.
func (am *AuthManager) RegisterAuthenticator(auth Authenticator) {
	am.mu.Lock()
	defer am.mu.Unlock()
	am.authenticators[auth.Type()] = auth
}

// AddListener adds an event listener.
func (am *AuthManager) AddListener(listener AuthEventListener) {
	am.mu.Lock()
	defer am.mu.Unlock()
	am.listeners = append(am.listeners, listener)
}

// Authenticate authenticates with a registry.
func (am *AuthManager) Authenticate(ctx context.Context, cred *Credential) (*Credential, error) {
	am.mu.RLock()
	auth, ok := am.authenticators[cred.Type]
	am.mu.RUnlock()

	if !ok {
		am.emit(&AuthEvent{
			Type:         "authenticate",
			Registry:     cred.Registry,
			RegistryType: cred.Type,
			Success:      false,
			Error:        ErrUnsupportedRegistry.Error(),
			Timestamp:    time.Now(),
		})
		return nil, ErrUnsupportedRegistry
	}

	result, err := auth.Authenticate(ctx, cred)
	if err != nil {
		am.emit(&AuthEvent{
			Type:         "authenticate",
			Registry:     cred.Registry,
			RegistryType: cred.Type,
			Success:      false,
			Error:        err.Error(),
			Timestamp:    time.Now(),
		})
		return nil, err
	}

	// Cache the credential
	am.mu.Lock()
	am.credentials[cred.Registry] = result
	am.mu.Unlock()

	am.emit(&AuthEvent{
		Type:         "authenticate",
		Registry:     cred.Registry,
		RegistryType: cred.Type,
		Success:      true,
		Timestamp:    time.Now(),
	})

	return result, nil
}

// GetCredential returns a cached credential, refreshing if expired.
func (am *AuthManager) GetCredential(ctx context.Context, registry string) (*Credential, error) {
	am.mu.RLock()
	cred, ok := am.credentials[registry]
	am.mu.RUnlock()

	if !ok {
		return nil, fmt.Errorf("no credential found for registry %s", registry)
	}

	if cred.IsExpired() {
		return am.Refresh(ctx, cred)
	}

	return cred, nil
}

// Refresh refreshes an expired credential.
func (am *AuthManager) Refresh(ctx context.Context, cred *Credential) (*Credential, error) {
	am.mu.RLock()
	auth, ok := am.authenticators[cred.Type]
	am.mu.RUnlock()

	if !ok {
		return nil, ErrUnsupportedRegistry
	}

	result, err := auth.Refresh(ctx, cred)
	if err != nil {
		am.emit(&AuthEvent{
			Type:         "refresh",
			Registry:     cred.Registry,
			RegistryType: cred.Type,
			Success:      false,
			Error:        err.Error(),
			Timestamp:    time.Now(),
		})
		return nil, err
	}

	// Update cache
	am.mu.Lock()
	am.credentials[cred.Registry] = result
	am.mu.Unlock()

	am.emit(&AuthEvent{
		Type:         "refresh",
		Registry:     cred.Registry,
		RegistryType: cred.Type,
		Success:      true,
		Timestamp:    time.Now(),
	})

	return result, nil
}

// GetAuthConfig returns Docker-style AuthConfig for a registry.
func (am *AuthManager) GetAuthConfig(ctx context.Context, registry string) (*AuthConfig, error) {
	cred, err := am.GetCredential(ctx, registry)
	if err != nil {
		return nil, err
	}

	am.mu.RLock()
	auth, ok := am.authenticators[cred.Type]
	am.mu.RUnlock()

	if !ok {
		return nil, ErrUnsupportedRegistry
	}

	return auth.GetAuthConfig(ctx, cred)
}

// LoadDockerConfig loads credentials from a Docker config.json.
func (am *AuthManager) LoadDockerConfig(ctx context.Context, config *DockerConfig) error {
	for registry, authConfig := range config.Auths {
		cred := &Credential{
			Type:     DetectType(registry),
			Registry: registry,
		}

		if authConfig.Auth != "" {
			decoded, err := base64.StdEncoding.DecodeString(authConfig.Auth)
			if err != nil {
				return fmt.Errorf("invalid auth encoding for %s: %w", registry, err)
			}
			parts := strings.SplitN(string(decoded), ":", 2)
			if len(parts) == 2 {
				cred.Username = parts[0]
				cred.Password = parts[1]
			}
		} else {
			cred.Username = authConfig.Username
			cred.Password = authConfig.Password
		}

		if authConfig.IdentityToken != "" {
			cred.Token = authConfig.IdentityToken
		}

		am.mu.Lock()
		am.credentials[registry] = cred
		am.mu.Unlock()
	}

	return nil
}

// ExportDockerConfig exports credentials as a Docker config.json structure.
func (am *AuthManager) ExportDockerConfig(ctx context.Context) (*DockerConfig, error) {
	am.mu.RLock()
	defer am.mu.RUnlock()

	config := &DockerConfig{
		Auths: make(map[string]AuthConfig),
	}

	for registry, cred := range am.credentials {
		auth, ok := am.authenticators[cred.Type]
		if !ok {
			continue
		}

		authConfig, err := auth.GetAuthConfig(ctx, cred)
		if err != nil {
			continue
		}

		config.Auths[registry] = *authConfig
	}

	return config, nil
}

func (am *AuthManager) emit(event *AuthEvent) {
	am.mu.RLock()
	listeners := am.listeners
	am.mu.RUnlock()

	for _, listener := range listeners {
		listener(event)
	}
}

// DetectType detects the registry type from a registry URL.
func DetectType(registry string) Type {
	registry = strings.ToLower(registry)

	if strings.Contains(registry, "docker.io") || registry == "docker.io" || registry == "index.docker.io" {
		return TypeDocker
	}
	if strings.Contains(registry, ".dkr.ecr.") && strings.Contains(registry, ".amazonaws.com") {
		return TypeECR
	}
	if strings.Contains(registry, "gcr.io") || strings.Contains(registry, "pkg.dev") {
		return TypeGCR
	}
	if strings.Contains(registry, ".azurecr.io") {
		return TypeACR
	}
	if strings.Contains(registry, "ghcr.io") {
		return TypeGitHub
	}
	if strings.Contains(registry, "quay.io") {
		return TypeQuay
	}

	return TypeGeneric
}

// DockerAuthenticator handles Docker Hub authentication.
type DockerAuthenticator struct {
	client *http.Client
}

// NewDockerAuthenticator creates a new Docker Hub authenticator.
func NewDockerAuthenticator(client *http.Client) *DockerAuthenticator {
	return &DockerAuthenticator{client: client}
}

// Type returns the registry type.
func (a *DockerAuthenticator) Type() Type {
	return TypeDocker
}

// Authenticate authenticates with Docker Hub.
func (a *DockerAuthenticator) Authenticate(ctx context.Context, cred *Credential) (*Credential, error) {
	authURL := "https://auth.docker.io/token?service=registry.docker.io&scope=repository:library/alpine:pull"

	req, err := http.NewRequestWithContext(ctx, "GET", authURL, http.NoBody)
	if err != nil {
		return nil, err
	}

	if cred.Username != "" && cred.Password != "" {
		req.SetBasicAuth(cred.Username, cred.Password)
	}

	resp, err := a.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, ErrAuthFailed
	}

	var tokenResp TokenResponse
	if err := json.NewDecoder(resp.Body).Decode(&tokenResp); err != nil {
		return nil, err
	}

	result := *cred
	result.Token = tokenResp.Token
	if result.Token == "" {
		result.Token = tokenResp.AccessToken
	}
	result.RefreshToken = tokenResp.RefreshToken
	if tokenResp.ExpiresIn > 0 {
		result.ExpiresAt = time.Now().Add(time.Duration(tokenResp.ExpiresIn) * time.Second)
	}

	return &result, nil
}

// GetAuthConfig returns Docker-style AuthConfig.
func (a *DockerAuthenticator) GetAuthConfig(ctx context.Context, cred *Credential) (*AuthConfig, error) {
	auth := base64.StdEncoding.EncodeToString([]byte(cred.Username + ":" + cred.Password))
	return &AuthConfig{
		Username:      cred.Username,
		Password:      cred.Password,
		Auth:          auth,
		ServerAddress: "https://index.docker.io/v1/",
	}, nil
}

// Refresh refreshes an expired credential.
func (a *DockerAuthenticator) Refresh(ctx context.Context, cred *Credential) (*Credential, error) {
	return a.Authenticate(ctx, cred)
}

// ECRAuthenticator handles AWS ECR authentication.
type ECRAuthenticator struct {
	client *http.Client
}

// NewECRAuthenticator creates a new ECR authenticator.
func NewECRAuthenticator(client *http.Client) *ECRAuthenticator {
	return &ECRAuthenticator{client: client}
}

// Type returns the registry type.
func (a *ECRAuthenticator) Type() Type {
	return TypeECR
}

// Authenticate authenticates with AWS ECR.
func (a *ECRAuthenticator) Authenticate(ctx context.Context, cred *Credential) (*Credential, error) {
	// In a real implementation, this would use AWS SDK to get an authorization token.
	// For now, we validate the credential structure.
	if cred.Region == "" {
		return nil, errors.New("ECR region is required")
	}

	result := *cred
	// ECR tokens are valid for 12 hours
	result.ExpiresAt = time.Now().Add(12 * time.Hour)

	return &result, nil
}

// GetAuthConfig returns Docker-style AuthConfig.
func (a *ECRAuthenticator) GetAuthConfig(ctx context.Context, cred *Credential) (*AuthConfig, error) {
	// ECR uses "AWS" as username and the authorization token as password
	registry := cred.Registry
	if registry == "" {
		registry = fmt.Sprintf("%s.dkr.ecr.%s.amazonaws.com", cred.AccountID, cred.Region)
	}

	auth := base64.StdEncoding.EncodeToString([]byte("AWS:" + cred.Token))
	return &AuthConfig{
		Username:      "AWS",
		Password:      cred.Token,
		Auth:          auth,
		ServerAddress: registry,
	}, nil
}

// Refresh refreshes an expired credential.
func (a *ECRAuthenticator) Refresh(ctx context.Context, cred *Credential) (*Credential, error) {
	return a.Authenticate(ctx, cred)
}

// GCRAuthenticator handles Google Container Registry authentication.
type GCRAuthenticator struct {
	client *http.Client
}

// NewGCRAuthenticator creates a new GCR authenticator.
func NewGCRAuthenticator(client *http.Client) *GCRAuthenticator {
	return &GCRAuthenticator{client: client}
}

// Type returns the registry type.
func (a *GCRAuthenticator) Type() Type {
	return TypeGCR
}

// Authenticate authenticates with GCR.
func (a *GCRAuthenticator) Authenticate(ctx context.Context, cred *Credential) (*Credential, error) {
	// GCR uses service account JSON key or access token
	if cred.Token == "" && cred.Password == "" {
		return nil, errors.New("GCR requires token or service account key")
	}

	result := *cred
	// GCR tokens typically expire in 1 hour
	result.ExpiresAt = time.Now().Add(1 * time.Hour)

	return &result, nil
}

// GetAuthConfig returns Docker-style AuthConfig.
func (a *GCRAuthenticator) GetAuthConfig(ctx context.Context, cred *Credential) (*AuthConfig, error) {
	// GCR uses "_token" as username and the access token as password
	// Or "oauth2accesstoken" as username
	password := cred.Token
	if password == "" {
		password = cred.Password
	}

	auth := base64.StdEncoding.EncodeToString([]byte("_token:" + password))
	return &AuthConfig{
		Username:      "_token",
		Password:      password,
		Auth:          auth,
		ServerAddress: cred.Registry,
	}, nil
}

// Refresh refreshes an expired credential.
func (a *GCRAuthenticator) Refresh(ctx context.Context, cred *Credential) (*Credential, error) {
	return a.Authenticate(ctx, cred)
}

// ACRAuthenticator handles Azure Container Registry authentication.
type ACRAuthenticator struct {
	client *http.Client
}

// NewACRAuthenticator creates a new ACR authenticator.
func NewACRAuthenticator(client *http.Client) *ACRAuthenticator {
	return &ACRAuthenticator{client: client}
}

// Type returns the registry type.
func (a *ACRAuthenticator) Type() Type {
	return TypeACR
}

// Authenticate authenticates with ACR.
func (a *ACRAuthenticator) Authenticate(ctx context.Context, cred *Credential) (*Credential, error) {
	// ACR supports multiple authentication methods:
	// - Admin credentials (username/password)
	// - Service principal (client ID/secret)
	// - Managed identity
	// - Azure AD tokens

	if cred.Token == "" && (cred.Username == "" || cred.Password == "") {
		return nil, errors.New("ACR requires token or username/password")
	}

	result := *cred
	// ACR tokens typically expire in 3 hours
	result.ExpiresAt = time.Now().Add(3 * time.Hour)

	return &result, nil
}

// GetAuthConfig returns Docker-style AuthConfig.
func (a *ACRAuthenticator) GetAuthConfig(ctx context.Context, cred *Credential) (*AuthConfig, error) {
	username := cred.Username
	password := cred.Password

	if cred.Token != "" {
		username = "00000000-0000-0000-0000-000000000000"
		password = cred.Token
	}

	auth := base64.StdEncoding.EncodeToString([]byte(username + ":" + password))
	return &AuthConfig{
		Username:      username,
		Password:      password,
		Auth:          auth,
		ServerAddress: cred.Registry,
	}, nil
}

// Refresh refreshes an expired credential.
func (a *ACRAuthenticator) Refresh(ctx context.Context, cred *Credential) (*Credential, error) {
	return a.Authenticate(ctx, cred)
}

// GitHubAuthenticator handles GitHub Container Registry authentication.
type GitHubAuthenticator struct {
	client *http.Client
}

// NewGitHubAuthenticator creates a new GitHub authenticator.
func NewGitHubAuthenticator(client *http.Client) *GitHubAuthenticator {
	return &GitHubAuthenticator{client: client}
}

// Type returns the registry type.
func (a *GitHubAuthenticator) Type() Type {
	return TypeGitHub
}

// Authenticate authenticates with GitHub Container Registry.
func (a *GitHubAuthenticator) Authenticate(ctx context.Context, cred *Credential) (*Credential, error) {
	// GHCR uses personal access tokens
	if cred.Token == "" && cred.Password == "" {
		return nil, errors.New("GHCR requires a personal access token")
	}

	result := *cred
	// PATs don't expire by default, but we set a reasonable refresh window
	result.ExpiresAt = time.Now().Add(24 * time.Hour)

	return &result, nil
}

// GetAuthConfig returns Docker-style AuthConfig.
func (a *GitHubAuthenticator) GetAuthConfig(ctx context.Context, cred *Credential) (*AuthConfig, error) {
	username := cred.Username
	if username == "" {
		username = "token"
	}

	password := cred.Token
	if password == "" {
		password = cred.Password
	}

	auth := base64.StdEncoding.EncodeToString([]byte(username + ":" + password))
	return &AuthConfig{
		Username:      username,
		Password:      password,
		Auth:          auth,
		ServerAddress: "https://ghcr.io",
	}, nil
}

// Refresh refreshes an expired credential.
func (a *GitHubAuthenticator) Refresh(ctx context.Context, cred *Credential) (*Credential, error) {
	return a.Authenticate(ctx, cred)
}

// QuayAuthenticator handles Quay.io authentication.
type QuayAuthenticator struct {
	client *http.Client
}

// NewQuayAuthenticator creates a new Quay authenticator.
func NewQuayAuthenticator(client *http.Client) *QuayAuthenticator {
	return &QuayAuthenticator{client: client}
}

// Type returns the registry type.
func (a *QuayAuthenticator) Type() Type {
	return TypeQuay
}

// Authenticate authenticates with Quay.io.
func (a *QuayAuthenticator) Authenticate(ctx context.Context, cred *Credential) (*Credential, error) {
	// Quay uses robot accounts or encrypted passwords
	if cred.Token == "" && (cred.Username == "" || cred.Password == "") {
		return nil, errors.New("quay requires token or username/password")
	}

	result := *cred
	result.ExpiresAt = time.Now().Add(24 * time.Hour)

	return &result, nil
}

// GetAuthConfig returns Docker-style AuthConfig.
func (a *QuayAuthenticator) GetAuthConfig(ctx context.Context, cred *Credential) (*AuthConfig, error) {
	password := cred.Password
	if cred.Token != "" {
		password = cred.Token
	}

	auth := base64.StdEncoding.EncodeToString([]byte(cred.Username + ":" + password))
	return &AuthConfig{
		Username:      cred.Username,
		Password:      password,
		Auth:          auth,
		ServerAddress: "https://quay.io",
	}, nil
}

// Refresh refreshes an expired credential.
func (a *QuayAuthenticator) Refresh(ctx context.Context, cred *Credential) (*Credential, error) {
	return a.Authenticate(ctx, cred)
}

// GenericAuthenticator handles generic OCI registry authentication.
type GenericAuthenticator struct {
	client *http.Client
}

// NewGenericAuthenticator creates a new generic authenticator.
func NewGenericAuthenticator(client *http.Client) *GenericAuthenticator {
	return &GenericAuthenticator{client: client}
}

// Type returns the registry type.
func (a *GenericAuthenticator) Type() Type {
	return TypeGeneric
}

// Authenticate authenticates with a generic OCI registry.
func (a *GenericAuthenticator) Authenticate(ctx context.Context, cred *Credential) (*Credential, error) {
	// Try to authenticate using the OCI distribution spec
	registryURL := cred.Registry
	if !strings.HasPrefix(registryURL, "http") {
		registryURL = "https://" + registryURL
	}

	// Check if the registry supports token authentication
	pingURL := registryURL + "/v2/"
	req, err := http.NewRequestWithContext(ctx, "GET", pingURL, http.NoBody)
	if err != nil {
		return nil, err
	}

	resp, err := a.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	// Check for WWW-Authenticate header
	authHeader := resp.Header.Get("WWW-Authenticate")
	if authHeader != "" && strings.HasPrefix(authHeader, "Bearer") {
		// Parse the bearer challenge and get a token
		return a.handleBearerAuth(ctx, cred, authHeader)
	}

	// Fall back to basic auth
	result := *cred
	result.ExpiresAt = time.Now().Add(24 * time.Hour)
	return &result, nil
}

func (a *GenericAuthenticator) handleBearerAuth(ctx context.Context, cred *Credential, authHeader string) (*Credential, error) {
	// Parse bearer challenge: Bearer realm="...",service="...",scope="..."
	params := parseBearerChallenge(authHeader)
	realm := params["realm"]
	if realm == "" {
		return nil, errors.New("missing realm in bearer challenge")
	}

	// Build token request URL
	tokenURL, err := url.Parse(realm)
	if err != nil {
		return nil, err
	}

	query := tokenURL.Query()
	if service := params["service"]; service != "" {
		query.Set("service", service)
	}
	if scope := params["scope"]; scope != "" {
		query.Set("scope", scope)
	}
	tokenURL.RawQuery = query.Encode()

	req, err := http.NewRequestWithContext(ctx, "GET", tokenURL.String(), http.NoBody)
	if err != nil {
		return nil, err
	}

	if cred.Username != "" && cred.Password != "" {
		req.SetBasicAuth(cred.Username, cred.Password)
	}

	resp, err := a.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, ErrAuthFailed
	}

	var tokenResp TokenResponse
	if err := json.NewDecoder(resp.Body).Decode(&tokenResp); err != nil {
		return nil, err
	}

	result := *cred
	result.Token = tokenResp.Token
	if result.Token == "" {
		result.Token = tokenResp.AccessToken
	}
	result.RefreshToken = tokenResp.RefreshToken
	if tokenResp.ExpiresIn > 0 {
		result.ExpiresAt = time.Now().Add(time.Duration(tokenResp.ExpiresIn) * time.Second)
	}

	return &result, nil
}

// GetAuthConfig returns Docker-style AuthConfig.
func (a *GenericAuthenticator) GetAuthConfig(ctx context.Context, cred *Credential) (*AuthConfig, error) {
	password := cred.Password
	if cred.Token != "" {
		password = cred.Token
	}

	auth := base64.StdEncoding.EncodeToString([]byte(cred.Username + ":" + password))
	return &AuthConfig{
		Username:      cred.Username,
		Password:      password,
		Auth:          auth,
		ServerAddress: cred.Registry,
	}, nil
}

// Refresh refreshes an expired credential.
func (a *GenericAuthenticator) Refresh(ctx context.Context, cred *Credential) (*Credential, error) {
	return a.Authenticate(ctx, cred)
}

func parseBearerChallenge(header string) map[string]string {
	params := make(map[string]string)

	// Remove "Bearer " prefix
	header = strings.TrimPrefix(header, "Bearer ")

	// Parse key="value" pairs
	for _, part := range strings.Split(header, ",") {
		part = strings.TrimSpace(part)
		if idx := strings.Index(part, "="); idx > 0 {
			key := strings.TrimSpace(part[:idx])
			value := strings.Trim(strings.TrimSpace(part[idx+1:]), "\"")
			params[key] = value
		}
	}

	return params
}

// CredentialStore stores and retrieves registry credentials.
type CredentialStore interface {
	Get(ctx context.Context, registry string) (*Credential, error)
	Store(ctx context.Context, cred *Credential) error
	Delete(ctx context.Context, registry string) error
	List(ctx context.Context) ([]*Credential, error)
}

// InMemoryCredentialStore is an in-memory credential store.
type InMemoryCredentialStore struct {
	credentials map[string]*Credential
	mu          sync.RWMutex
}

// NewInMemoryCredentialStore creates a new in-memory credential store.
func NewInMemoryCredentialStore() *InMemoryCredentialStore {
	return &InMemoryCredentialStore{
		credentials: make(map[string]*Credential),
	}
}

// Get retrieves a credential.
func (s *InMemoryCredentialStore) Get(ctx context.Context, registry string) (*Credential, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	cred, ok := s.credentials[registry]
	if !ok {
		return nil, fmt.Errorf("credential not found for registry %s", registry)
	}

	return cred, nil
}

// Store stores a credential.
func (s *InMemoryCredentialStore) Store(ctx context.Context, cred *Credential) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.credentials[cred.Registry] = cred
	return nil
}

// Delete deletes a credential.
func (s *InMemoryCredentialStore) Delete(ctx context.Context, registry string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	delete(s.credentials, registry)
	return nil
}

// List lists all credentials.
func (s *InMemoryCredentialStore) List(ctx context.Context) ([]*Credential, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	result := make([]*Credential, 0, len(s.credentials))
	for _, cred := range s.credentials {
		result = append(result, cred)
	}

	return result, nil
}
