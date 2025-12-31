// Package auth provides authentication and authorization for the Keystone Core API.
// It supports multiple authentication methods: API keys, JWT tokens, and mTLS.
package auth

import (
	"context"
	"crypto/subtle"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/shawnbutts/keystone-core/pkg/config"
)

// Standard errors returned by authenticators
var (
	ErrNoCredentials      = errors.New("no credentials provided")
	ErrInvalidCredentials = errors.New("invalid credentials")
	ErrExpiredCredentials = errors.New("credentials expired")
	ErrDisabledKey        = errors.New("API key is disabled")
	ErrInsufficientRole   = errors.New("insufficient role for this operation")
	ErrAuthRequired       = errors.New("authentication required")
)

// Role represents an authorization role
type Role string

const (
	// RoleAdmin has full access to all operations
	RoleAdmin Role = "admin"
	// RoleOperator can execute commands and manage agents, but not configure the system
	RoleOperator Role = "operator"
	// RoleReadonly can only view agents, commands, and status
	RoleReadonly Role = "readonly"
)

// Principal represents an authenticated identity
type Principal struct {
	// ID is a unique identifier for the principal
	ID string
	// Name is a human-readable name
	Name string
	// Role is the authorization role
	Role Role
	// AuthMethod indicates how the principal was authenticated
	AuthMethod string
	// Metadata contains additional authentication-specific data
	Metadata map[string]string
	// AuthenticatedAt is when the authentication occurred
	AuthenticatedAt time.Time
}

// HasRole checks if the principal has at least the specified role level
func (p *Principal) HasRole(required Role) bool {
	switch required {
	case RoleReadonly:
		// Everyone can do readonly operations
		return true
	case RoleOperator:
		return p.Role == RoleOperator || p.Role == RoleAdmin
	case RoleAdmin:
		return p.Role == RoleAdmin
	default:
		return false
	}
}

// Authenticator is the interface for authentication providers
type Authenticator interface {
	// Authenticate validates credentials and returns a Principal
	// The credentials format depends on the authenticator type
	Authenticate(ctx context.Context, credentials string) (*Principal, error)

	// Name returns the authenticator name for logging/metrics
	Name() string
}

// Authorizer checks if a principal can perform an operation
type Authorizer interface {
	// Authorize checks if the principal can perform the given method
	// method is the full gRPC method name (e.g., "/kscore.v1.ControlPlaneService/ExecuteCommand")
	Authorize(ctx context.Context, principal *Principal, method string) error
}

// contextKey is a custom type for context keys to avoid collisions
type contextKey string

const (
	// PrincipalContextKey is the context key for storing the authenticated principal
	PrincipalContextKey contextKey = "kscore-principal"
)

// PrincipalFromContext retrieves the authenticated principal from context
func PrincipalFromContext(ctx context.Context) (*Principal, bool) {
	principal, ok := ctx.Value(PrincipalContextKey).(*Principal)
	return principal, ok
}

// ContextWithPrincipal adds a principal to the context
func ContextWithPrincipal(ctx context.Context, principal *Principal) context.Context {
	return context.WithValue(ctx, PrincipalContextKey, principal)
}

// APIKeyAuthenticator authenticates using API keys
type APIKeyAuthenticator struct {
	mu   sync.RWMutex
	keys map[string]*apiKeyEntry
}

type apiKeyEntry struct {
	config    config.APIKeyConfig
	expiresAt *time.Time
}

// NewAPIKeyAuthenticator creates a new API key authenticator from config
func NewAPIKeyAuthenticator(cfg config.APIKeyAuthConfig) (*APIKeyAuthenticator, error) {
	auth := &APIKeyAuthenticator{
		keys: make(map[string]*apiKeyEntry),
	}

	for key, keyCfg := range cfg.Keys {
		entry := &apiKeyEntry{
			config: keyCfg,
		}

		// Parse expiration time if set
		if keyCfg.ExpiresAt != "" {
			t, err := time.Parse(time.RFC3339, keyCfg.ExpiresAt)
			if err != nil {
				return nil, fmt.Errorf("invalid expiration time for key %q: %w", keyCfg.Name, err)
			}
			entry.expiresAt = &t
		}

		auth.keys[key] = entry
	}

	return auth, nil
}

// Name returns the authenticator name
func (a *APIKeyAuthenticator) Name() string {
	return "apikey"
}

// Authenticate validates an API key and returns a Principal
func (a *APIKeyAuthenticator) Authenticate(ctx context.Context, credentials string) (*Principal, error) {
	if credentials == "" {
		return nil, ErrNoCredentials
	}

	a.mu.RLock()
	defer a.mu.RUnlock()

	// Use constant-time comparison to prevent timing attacks
	var matchedEntry *apiKeyEntry
	var matchedKey string
	for key, entry := range a.keys {
		if subtle.ConstantTimeCompare([]byte(key), []byte(credentials)) == 1 {
			matchedEntry = entry
			matchedKey = key
			break
		}
	}

	if matchedEntry == nil {
		return nil, ErrInvalidCredentials
	}

	// Check if key is enabled
	if !matchedEntry.config.Enabled {
		return nil, ErrDisabledKey
	}

	// Check expiration
	if matchedEntry.expiresAt != nil && time.Now().After(*matchedEntry.expiresAt) {
		return nil, ErrExpiredCredentials
	}

	// Create principal
	principal := &Principal{
		ID:              fmt.Sprintf("apikey:%s", matchedEntry.config.Name),
		Name:            matchedEntry.config.Name,
		Role:            Role(matchedEntry.config.Role),
		AuthMethod:      "apikey",
		AuthenticatedAt: time.Now(),
		Metadata: map[string]string{
			"key_hash": fmt.Sprintf("%x", []byte(matchedKey[:8])), // Only store partial hash for audit
		},
	}

	return principal, nil
}

// AddKey adds a new API key at runtime
func (a *APIKeyAuthenticator) AddKey(key string, cfg config.APIKeyConfig) error {
	if key == "" {
		return fmt.Errorf("API key value cannot be empty")
	}
	if len(key) < 32 {
		return fmt.Errorf("API key must be at least 32 characters")
	}

	a.mu.Lock()
	defer a.mu.Unlock()

	entry := &apiKeyEntry{
		config: cfg,
	}

	if cfg.ExpiresAt != "" {
		t, err := time.Parse(time.RFC3339, cfg.ExpiresAt)
		if err != nil {
			return fmt.Errorf("invalid expiration time: %w", err)
		}
		entry.expiresAt = &t
	}

	a.keys[key] = entry
	return nil
}

// RemoveKey removes an API key
func (a *APIKeyAuthenticator) RemoveKey(key string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	delete(a.keys, key)
}

// DisableKey disables an API key without removing it
func (a *APIKeyAuthenticator) DisableKey(keyName string) bool {
	a.mu.Lock()
	defer a.mu.Unlock()

	for _, entry := range a.keys {
		if entry.config.Name == keyName {
			entry.config.Enabled = false
			return true
		}
	}
	return false
}
