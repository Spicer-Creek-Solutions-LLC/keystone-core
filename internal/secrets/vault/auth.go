// Package vault provides a HashiCorp Vault backend for the secrets broker.
package vault

import (
	"context"
	"fmt"
	"os"
	"time"
)

// Authenticator handles Vault authentication.
type Authenticator interface {
	// Authenticate performs authentication and returns a token.
	Authenticate(ctx context.Context, client *Client) (*AuthResponse, error)

	// Type returns the authentication method type.
	Type() string
}

// AuthResponse contains the authentication response.
type AuthResponse struct {
	// Token is the authentication token.
	Token string

	// ExpiresAt is when the token expires.
	ExpiresAt time.Time

	// Renewable indicates if the token can be renewed.
	Renewable bool

	// Policies are the policies attached to the token.
	Policies []string

	// Metadata contains additional metadata from auth.
	Metadata map[string]string
}

// AuthConfig configures authentication methods.
type AuthConfig struct {
	// Method is the authentication method (token, approle, kubernetes).
	Method string `json:"method"`

	// Token authentication settings.
	Token *TokenAuthConfig `json:"token,omitempty"`

	// AppRole authentication settings.
	AppRole *AppRoleAuthConfig `json:"approle,omitempty"`

	// Kubernetes authentication settings.
	Kubernetes *KubernetesAuthConfig `json:"kubernetes,omitempty"`
}

// NewAuthenticator creates an Authenticator from AuthConfig.
func NewAuthenticator(config *AuthConfig) (Authenticator, error) {
	if config == nil {
		return nil, fmt.Errorf("auth config is required")
	}

	switch config.Method {
	case "token":
		if config.Token == nil {
			return nil, fmt.Errorf("token auth config is required")
		}
		return NewTokenAuth(config.Token), nil

	case "approle":
		if config.AppRole == nil {
			return nil, fmt.Errorf("approle auth config is required")
		}
		return NewAppRoleAuth(config.AppRole), nil

	case "kubernetes", "k8s":
		if config.Kubernetes == nil {
			return nil, fmt.Errorf("kubernetes auth config is required")
		}
		return NewKubernetesAuth(config.Kubernetes)

	default:
		return nil, fmt.Errorf("unsupported auth method: %s", config.Method)
	}
}

// TokenAuthConfig configures token authentication.
type TokenAuthConfig struct {
	// Token is the Vault token to use.
	Token string `json:"token,omitempty"`

	// TokenFile is the path to a file containing the token.
	TokenFile string `json:"token_file,omitempty"`

	// TokenEnv is the environment variable containing the token.
	TokenEnv string `json:"token_env,omitempty"`
}

// TokenAuth implements token-based authentication.
type TokenAuth struct {
	config *TokenAuthConfig
}

// NewTokenAuth creates a new token authenticator.
func NewTokenAuth(config *TokenAuthConfig) *TokenAuth {
	return &TokenAuth{config: config}
}

// Type returns the authentication method type.
func (a *TokenAuth) Type() string {
	return "token"
}

// Authenticate performs token authentication.
func (a *TokenAuth) Authenticate(ctx context.Context, client *Client) (*AuthResponse, error) {
	token := a.resolveToken()
	if token == "" {
		return nil, fmt.Errorf("no token provided")
	}

	// Set token on client for the lookup call
	client.SetToken(token)

	// Look up the token to get its properties
	resp, err := client.Read(ctx, "auth/token/lookup-self")
	if err != nil {
		return nil, fmt.Errorf("failed to lookup token: %w", err)
	}

	return parseAuthResponse(resp)
}

// resolveToken resolves the token from various sources.
func (a *TokenAuth) resolveToken() string {
	// Direct token takes precedence
	if a.config.Token != "" {
		return a.config.Token
	}

	// Try environment variable
	if a.config.TokenEnv != "" {
		if token := os.Getenv(a.config.TokenEnv); token != "" {
			return token
		}
	}

	// Default environment variable
	if token := os.Getenv("VAULT_TOKEN"); token != "" {
		return token
	}

	// Try token file
	if a.config.TokenFile != "" {
		if data, err := os.ReadFile(a.config.TokenFile); err == nil {
			return string(data)
		}
	}

	// Try default token file location
	if home, err := os.UserHomeDir(); err == nil {
		tokenFile := home + "/.vault-token"
		if data, err := os.ReadFile(tokenFile); err == nil {
			return string(data)
		}
	}

	return ""
}

// AppRoleAuthConfig configures AppRole authentication.
type AppRoleAuthConfig struct {
	// MountPath is the auth mount path (default: "approle").
	MountPath string `json:"mount_path,omitempty"`

	// RoleID is the AppRole role ID.
	RoleID string `json:"role_id,omitempty"`

	// RoleIDFile is the path to a file containing the role ID.
	RoleIDFile string `json:"role_id_file,omitempty"`

	// RoleIDEnv is the environment variable containing the role ID.
	RoleIDEnv string `json:"role_id_env,omitempty"`

	// SecretID is the AppRole secret ID.
	SecretID string `json:"secret_id,omitempty"`

	// SecretIDFile is the path to a file containing the secret ID.
	SecretIDFile string `json:"secret_id_file,omitempty"`

	// SecretIDEnv is the environment variable containing the secret ID.
	SecretIDEnv string `json:"secret_id_env,omitempty"`

	// RemoveSecretIDFile removes the secret ID file after reading.
	RemoveSecretIDFile bool `json:"remove_secret_id_file,omitempty"`
}

// AppRoleAuth implements AppRole authentication.
type AppRoleAuth struct {
	config *AppRoleAuthConfig
}

// NewAppRoleAuth creates a new AppRole authenticator.
func NewAppRoleAuth(config *AppRoleAuthConfig) *AppRoleAuth {
	if config.MountPath == "" {
		config.MountPath = "approle"
	}
	return &AppRoleAuth{config: config}
}

// Type returns the authentication method type.
func (a *AppRoleAuth) Type() string {
	return "approle"
}

// Authenticate performs AppRole authentication.
func (a *AppRoleAuth) Authenticate(ctx context.Context, client *Client) (*AuthResponse, error) {
	roleID := a.resolveRoleID()
	if roleID == "" {
		return nil, fmt.Errorf("no role_id provided")
	}

	secretID := a.resolveSecretID()

	// Build login data
	data := map[string]interface{}{
		"role_id": roleID,
	}
	if secretID != "" {
		data["secret_id"] = secretID
	}

	// Remove secret ID file if configured
	if a.config.RemoveSecretIDFile && a.config.SecretIDFile != "" {
		defer func() {
			_ = os.Remove(a.config.SecretIDFile)
		}()
	}

	// Login
	path := fmt.Sprintf("auth/%s/login", a.config.MountPath)
	resp, err := client.Write(ctx, path, data)
	if err != nil {
		return nil, fmt.Errorf("approle login failed: %w", err)
	}

	authResp, err := parseLoginResponse(resp)
	if err != nil {
		return nil, err
	}

	return authResp, nil
}

// resolveRoleID resolves the role ID from various sources.
func (a *AppRoleAuth) resolveRoleID() string {
	if a.config.RoleID != "" {
		return a.config.RoleID
	}

	if a.config.RoleIDEnv != "" {
		if roleID := os.Getenv(a.config.RoleIDEnv); roleID != "" {
			return roleID
		}
	}

	if roleID := os.Getenv("VAULT_APPROLE_ROLE_ID"); roleID != "" {
		return roleID
	}

	if a.config.RoleIDFile != "" {
		if data, err := os.ReadFile(a.config.RoleIDFile); err == nil {
			return string(data)
		}
	}

	return ""
}

// resolveSecretID resolves the secret ID from various sources.
func (a *AppRoleAuth) resolveSecretID() string {
	if a.config.SecretID != "" {
		return a.config.SecretID
	}

	if a.config.SecretIDEnv != "" {
		if secretID := os.Getenv(a.config.SecretIDEnv); secretID != "" {
			return secretID
		}
	}

	if secretID := os.Getenv("VAULT_APPROLE_SECRET_ID"); secretID != "" {
		return secretID
	}

	if a.config.SecretIDFile != "" {
		if data, err := os.ReadFile(a.config.SecretIDFile); err == nil {
			return string(data)
		}
	}

	return ""
}

// KubernetesAuthConfig configures Kubernetes authentication.
type KubernetesAuthConfig struct {
	// MountPath is the auth mount path (default: "kubernetes").
	MountPath string `json:"mount_path,omitempty"`

	// Role is the Vault role to authenticate as.
	Role string `json:"role"`

	// ServiceAccountToken is the Kubernetes service account token.
	ServiceAccountToken string `json:"service_account_token,omitempty"`

	// ServiceAccountTokenFile is the path to the service account token file.
	// Default: /var/run/secrets/kubernetes.io/serviceaccount/token
	ServiceAccountTokenFile string `json:"service_account_token_file,omitempty"`
}

// KubernetesAuth implements Kubernetes authentication.
type KubernetesAuth struct {
	config *KubernetesAuthConfig
}

// NewKubernetesAuth creates a new Kubernetes authenticator.
func NewKubernetesAuth(config *KubernetesAuthConfig) (*KubernetesAuth, error) {
	if config.Role == "" {
		return nil, fmt.Errorf("kubernetes auth requires a role")
	}

	if config.MountPath == "" {
		config.MountPath = "kubernetes"
	}

	if config.ServiceAccountTokenFile == "" {
		config.ServiceAccountTokenFile = "/var/run/secrets/kubernetes.io/serviceaccount/token"
	}

	return &KubernetesAuth{config: config}, nil
}

// Type returns the authentication method type.
func (a *KubernetesAuth) Type() string {
	return "kubernetes"
}

// Authenticate performs Kubernetes authentication.
func (a *KubernetesAuth) Authenticate(ctx context.Context, client *Client) (*AuthResponse, error) {
	jwt := a.resolveJWT()
	if jwt == "" {
		return nil, fmt.Errorf("no service account token available")
	}

	// Build login data
	data := map[string]interface{}{
		"role": a.config.Role,
		"jwt":  jwt,
	}

	// Login
	path := fmt.Sprintf("auth/%s/login", a.config.MountPath)
	resp, err := client.Write(ctx, path, data)
	if err != nil {
		return nil, fmt.Errorf("kubernetes login failed: %w", err)
	}

	return parseLoginResponse(resp)
}

// resolveJWT resolves the service account JWT token.
func (a *KubernetesAuth) resolveJWT() string {
	if a.config.ServiceAccountToken != "" {
		return a.config.ServiceAccountToken
	}

	if data, err := os.ReadFile(a.config.ServiceAccountTokenFile); err == nil {
		return string(data)
	}

	return ""
}

// parseAuthResponse parses a token lookup response.
func parseAuthResponse(resp map[string]interface{}) (*AuthResponse, error) {
	if resp == nil {
		return nil, fmt.Errorf("empty response")
	}

	data, ok := resp["data"].(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("invalid response format")
	}

	authResp := &AuthResponse{
		Metadata: make(map[string]string),
	}

	if id, ok := data["id"].(string); ok {
		authResp.Token = id
	}

	if ttl, ok := data["ttl"].(float64); ok {
		authResp.ExpiresAt = time.Now().Add(time.Duration(ttl) * time.Second)
	}

	if renewable, ok := data["renewable"].(bool); ok {
		authResp.Renewable = renewable
	}

	if policies, ok := data["policies"].([]interface{}); ok {
		for _, p := range policies {
			if s, ok := p.(string); ok {
				authResp.Policies = append(authResp.Policies, s)
			}
		}
	}

	if meta, ok := data["meta"].(map[string]interface{}); ok {
		for k, v := range meta {
			if s, ok := v.(string); ok {
				authResp.Metadata[k] = s
			}
		}
	}

	return authResp, nil
}

// parseLoginResponse parses a login response.
func parseLoginResponse(resp map[string]interface{}) (*AuthResponse, error) {
	if resp == nil {
		return nil, fmt.Errorf("empty response")
	}

	auth, ok := resp["auth"].(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("invalid login response: missing auth")
	}

	authResp := &AuthResponse{
		Metadata: make(map[string]string),
	}

	if token, ok := auth["client_token"].(string); ok {
		authResp.Token = token
	}

	if leaseDuration, ok := auth["lease_duration"].(float64); ok {
		authResp.ExpiresAt = time.Now().Add(time.Duration(leaseDuration) * time.Second)
	}

	if renewable, ok := auth["renewable"].(bool); ok {
		authResp.Renewable = renewable
	}

	if policies, ok := auth["policies"].([]interface{}); ok {
		for _, p := range policies {
			if s, ok := p.(string); ok {
				authResp.Policies = append(authResp.Policies, s)
			}
		}
	}

	if meta, ok := auth["metadata"].(map[string]interface{}); ok {
		for k, v := range meta {
			if s, ok := v.(string); ok {
				authResp.Metadata[k] = s
			}
		}
	}

	return authResp, nil
}
