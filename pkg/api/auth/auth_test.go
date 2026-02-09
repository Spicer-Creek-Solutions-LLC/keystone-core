package auth

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/shawnbutts/keystone-core/internal/config"
)

func TestRoleHierarchy(t *testing.T) {
	tests := []struct {
		name          string
		principalRole Role
		requiredRole  Role
		shouldPass    bool
	}{
		{"admin can do admin", RoleAdmin, RoleAdmin, true},
		{"admin can do operator", RoleAdmin, RoleOperator, true},
		{"admin can do readonly", RoleAdmin, RoleReadonly, true},
		{"operator cannot do admin", RoleOperator, RoleAdmin, false},
		{"operator can do operator", RoleOperator, RoleOperator, true},
		{"operator can do readonly", RoleOperator, RoleReadonly, true},
		{"readonly cannot do admin", RoleReadonly, RoleAdmin, false},
		{"readonly cannot do operator", RoleReadonly, RoleOperator, false},
		{"readonly can do readonly", RoleReadonly, RoleReadonly, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := &Principal{Role: tt.principalRole}
			result := p.HasRole(tt.requiredRole)
			if result != tt.shouldPass {
				t.Errorf("HasRole(%v) = %v, want %v", tt.requiredRole, result, tt.shouldPass)
			}
		})
	}
}

func TestAPIKeyAuthenticator_Basic(t *testing.T) {
	cfg := config.APIKeyAuthConfig{
		HeaderName:  "X-API-Key",
		MetadataKey: "x-api-key",
		Keys: map[string]config.APIKeyConfig{
			"test-key-that-is-at-least-32-chars-long": {
				Name:    "test-key",
				Role:    "admin",
				Enabled: true,
			},
		},
	}

	auth, err := NewAPIKeyAuthenticator(cfg)
	if err != nil {
		t.Fatalf("NewAPIKeyAuthenticator() error = %v", err)
	}

	ctx := context.Background()

	// Test valid key
	principal, err := auth.Authenticate(ctx, "test-key-that-is-at-least-32-chars-long")
	if err != nil {
		t.Fatalf("Authenticate() error = %v", err)
	}
	if principal.Name != "test-key" {
		t.Errorf("principal.Name = %v, want test-key", principal.Name)
	}
	if principal.Role != RoleAdmin {
		t.Errorf("principal.Role = %v, want admin", principal.Role)
	}
	if principal.AuthMethod != "apikey" {
		t.Errorf("principal.AuthMethod = %v, want apikey", principal.AuthMethod)
	}

	// Test invalid key
	_, err = auth.Authenticate(ctx, "wrong-key")
	if !errors.Is(err, ErrInvalidCredentials) {
		t.Errorf("Authenticate(wrong-key) error = %v, want ErrInvalidCredentials", err)
	}

	// Test empty key
	_, err = auth.Authenticate(ctx, "")
	if !errors.Is(err, ErrNoCredentials) {
		t.Errorf("Authenticate('') error = %v, want ErrNoCredentials", err)
	}
}

func TestAPIKeyAuthenticator_DisabledKey(t *testing.T) {
	cfg := config.APIKeyAuthConfig{
		Keys: map[string]config.APIKeyConfig{
			"disabled-key-that-is-at-least-32-chars": {
				Name:    "disabled-key",
				Role:    "admin",
				Enabled: false,
			},
		},
	}

	auth, err := NewAPIKeyAuthenticator(cfg)
	if err != nil {
		t.Fatalf("NewAPIKeyAuthenticator() error = %v", err)
	}

	ctx := context.Background()
	_, err = auth.Authenticate(ctx, "disabled-key-that-is-at-least-32-chars")
	if !errors.Is(err, ErrDisabledKey) {
		t.Errorf("Authenticate(disabled-key) error = %v, want ErrDisabledKey", err)
	}
}

func TestAPIKeyAuthenticator_ExpiredKey(t *testing.T) {
	expiredTime := time.Now().Add(-1 * time.Hour).Format(time.RFC3339)
	cfg := config.APIKeyAuthConfig{
		Keys: map[string]config.APIKeyConfig{
			"expired-key-that-is-at-least-32-chars!": {
				Name:      "expired-key",
				Role:      "admin",
				Enabled:   true,
				ExpiresAt: expiredTime,
			},
		},
	}

	auth, err := NewAPIKeyAuthenticator(cfg)
	if err != nil {
		t.Fatalf("NewAPIKeyAuthenticator() error = %v", err)
	}

	ctx := context.Background()
	_, err = auth.Authenticate(ctx, "expired-key-that-is-at-least-32-chars!")
	if !errors.Is(err, ErrExpiredCredentials) {
		t.Errorf("Authenticate(expired-key) error = %v, want ErrExpiredCredentials", err)
	}
}

func TestAPIKeyAuthenticator_ValidExpiration(t *testing.T) {
	futureTime := time.Now().Add(1 * time.Hour).Format(time.RFC3339)
	cfg := config.APIKeyAuthConfig{
		Keys: map[string]config.APIKeyConfig{
			"valid-expiring-key-at-least-32-chars!!": {
				Name:      "valid-key",
				Role:      "admin",
				Enabled:   true,
				ExpiresAt: futureTime,
			},
		},
	}

	auth, err := NewAPIKeyAuthenticator(cfg)
	if err != nil {
		t.Fatalf("NewAPIKeyAuthenticator() error = %v", err)
	}

	ctx := context.Background()
	principal, err := auth.Authenticate(ctx, "valid-expiring-key-at-least-32-chars!!")
	if err != nil {
		t.Errorf("Authenticate() error = %v, want nil", err)
	}
	if principal == nil {
		t.Fatal("principal is nil")
	}
	if principal.Name != "valid-key" {
		t.Errorf("principal.Name = %v, want valid-key", principal.Name)
	}
}

func TestAPIKeyAuthenticator_AddRemoveKey(t *testing.T) {
	cfg := config.APIKeyAuthConfig{
		Keys: map[string]config.APIKeyConfig{},
	}

	auth, err := NewAPIKeyAuthenticator(cfg)
	if err != nil {
		t.Fatalf("NewAPIKeyAuthenticator() error = %v", err)
	}

	ctx := context.Background()

	// Initially no keys
	_, err = auth.Authenticate(ctx, "new-key-that-is-at-least-32-characters")
	if !errors.Is(err, ErrInvalidCredentials) {
		t.Errorf("Authenticate() before add error = %v, want ErrInvalidCredentials", err)
	}

	// Add key
	err = auth.AddKey("new-key-that-is-at-least-32-characters", config.APIKeyConfig{
		Name:    "new-key",
		Role:    "operator",
		Enabled: true,
	})
	if err != nil {
		t.Fatalf("AddKey() error = %v", err)
	}

	// Now should work
	principal, err := auth.Authenticate(ctx, "new-key-that-is-at-least-32-characters")
	if err != nil {
		t.Errorf("Authenticate() after add error = %v", err)
	}
	if principal == nil || principal.Name != "new-key" {
		t.Error("principal not returned correctly after add")
	}

	// Remove key
	auth.RemoveKey("new-key-that-is-at-least-32-characters")

	// Should fail again
	_, err = auth.Authenticate(ctx, "new-key-that-is-at-least-32-characters")
	if !errors.Is(err, ErrInvalidCredentials) {
		t.Errorf("Authenticate() after remove error = %v, want ErrInvalidCredentials", err)
	}
}

func TestAPIKeyAuthenticator_DisableKey(t *testing.T) {
	cfg := config.APIKeyAuthConfig{
		Keys: map[string]config.APIKeyConfig{
			"key-to-disable-at-least-32-characters!": {
				Name:    "key-to-disable",
				Role:    "admin",
				Enabled: true,
			},
		},
	}

	auth, err := NewAPIKeyAuthenticator(cfg)
	if err != nil {
		t.Fatalf("NewAPIKeyAuthenticator() error = %v", err)
	}

	ctx := context.Background()

	// Should work initially
	_, err = auth.Authenticate(ctx, "key-to-disable-at-least-32-characters!")
	if err != nil {
		t.Errorf("Authenticate() before disable error = %v", err)
	}

	// Disable by name
	if !auth.DisableKey("key-to-disable") {
		t.Error("DisableKey() returned false")
	}

	// Should fail now
	_, err = auth.Authenticate(ctx, "key-to-disable-at-least-32-characters!")
	if !errors.Is(err, ErrDisabledKey) {
		t.Errorf("Authenticate() after disable error = %v, want ErrDisabledKey", err)
	}
}

func TestAPIKeyAuthenticator_ShortKeyRejected(t *testing.T) {
	cfg := config.APIKeyAuthConfig{
		Keys: map[string]config.APIKeyConfig{},
	}

	auth, err := NewAPIKeyAuthenticator(cfg)
	if err != nil {
		t.Fatalf("NewAPIKeyAuthenticator() error = %v", err)
	}

	// Try to add a short key
	err = auth.AddKey("short", config.APIKeyConfig{
		Name:    "short-key",
		Role:    "admin",
		Enabled: true,
	})
	if err == nil {
		t.Error("AddKey(short) should fail")
	}
}

func TestPrincipalContext(t *testing.T) {
	ctx := context.Background()

	// No principal initially
	_, ok := PrincipalFromContext(ctx)
	if ok {
		t.Error("PrincipalFromContext() should return false for empty context")
	}

	// Add principal
	principal := &Principal{
		ID:   "test-id",
		Name: "test-user",
		Role: RoleAdmin,
	}
	ctx = ContextWithPrincipal(ctx, principal)

	// Now should find it
	p, ok := PrincipalFromContext(ctx)
	if !ok {
		t.Fatal("PrincipalFromContext() should return true")
	}
	if p.ID != "test-id" {
		t.Errorf("principal.ID = %v, want test-id", p.ID)
	}
}

func TestName(t *testing.T) {
	cfg := config.APIKeyAuthConfig{
		Keys: map[string]config.APIKeyConfig{},
	}

	auth, _ := NewAPIKeyAuthenticator(cfg)
	if auth.Name() != "apikey" {
		t.Errorf("Name() = %v, want apikey", auth.Name())
	}
}
