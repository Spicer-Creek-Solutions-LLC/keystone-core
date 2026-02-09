package auth

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"

	"github.com/shawnbutts/keystone-core/internal/config"
)

func TestJWTAuthenticator_NewWithSecret(t *testing.T) {
	cfg := config.JWTAuthConfig{
		Secret: "my-super-secret-key-for-testing-purposes",
	}

	auth, err := NewJWTAuthenticator(cfg)
	if err != nil {
		t.Fatalf("NewJWTAuthenticator failed: %v", err)
	}

	if auth.Name() != "jwt" {
		t.Errorf("Expected name 'jwt', got '%s'", auth.Name())
	}
}

func TestJWTAuthenticator_NewWithoutKey(t *testing.T) {
	cfg := config.JWTAuthConfig{}

	_, err := NewJWTAuthenticator(cfg)
	if !errors.Is(err, ErrNoSigningKey) {
		t.Errorf("Expected ErrNoSigningKey, got %v", err)
	}
}

func TestJWTAuthenticator_AuthenticateHMAC(t *testing.T) {
	secret := []byte("my-super-secret-key-for-testing-purposes")
	cfg := config.JWTAuthConfig{
		Secret: string(secret),
	}

	auth, err := NewJWTAuthenticator(cfg)
	if err != nil {
		t.Fatalf("NewJWTAuthenticator failed: %v", err)
	}

	// Create a valid token
	claims := &JWTClaims{
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   "user123",
			Issuer:    "test-issuer",
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
		},
		Role:  "admin",
		Name:  "Test User",
		Email: "test@example.com",
	}

	token, err := GenerateToken(secret, claims, jwt.SigningMethodHS256)
	if err != nil {
		t.Fatalf("GenerateToken failed: %v", err)
	}

	ctx := context.Background()
	principal, err := auth.Authenticate(ctx, token)
	if err != nil {
		t.Fatalf("Authenticate failed: %v", err)
	}

	if principal.ID != "jwt:user123" {
		t.Errorf("Expected ID 'jwt:user123', got '%s'", principal.ID)
	}
	if principal.Name != "Test User" {
		t.Errorf("Expected Name 'Test User', got '%s'", principal.Name)
	}
	if principal.Role != RoleAdmin {
		t.Errorf("Expected Role 'admin', got '%s'", principal.Role)
	}
	if principal.AuthMethod != "jwt" {
		t.Errorf("Expected AuthMethod 'jwt', got '%s'", principal.AuthMethod)
	}
	if principal.Metadata["email"] != "test@example.com" {
		t.Errorf("Expected email in metadata, got '%v'", principal.Metadata)
	}
}

func TestJWTAuthenticator_AuthenticateExpired(t *testing.T) {
	secret := []byte("my-super-secret-key-for-testing-purposes")
	cfg := config.JWTAuthConfig{
		Secret: string(secret),
	}

	auth, err := NewJWTAuthenticator(cfg)
	if err != nil {
		t.Fatalf("NewJWTAuthenticator failed: %v", err)
	}

	// Create an expired token
	claims := &JWTClaims{
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   "user123",
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(-time.Hour)), // Expired 1 hour ago
			IssuedAt:  jwt.NewNumericDate(time.Now().Add(-2 * time.Hour)),
		},
		Role: "admin",
	}

	token, err := GenerateToken(secret, claims, jwt.SigningMethodHS256)
	if err != nil {
		t.Fatalf("GenerateToken failed: %v", err)
	}

	ctx := context.Background()
	_, err = auth.Authenticate(ctx, token)
	if !errors.Is(err, ErrTokenExpired) {
		t.Errorf("Expected ErrTokenExpired, got %v", err)
	}
}

func TestJWTAuthenticator_AuthenticateInvalidSignature(t *testing.T) {
	cfg := config.JWTAuthConfig{
		Secret: "correct-secret",
	}

	auth, err := NewJWTAuthenticator(cfg)
	if err != nil {
		t.Fatalf("NewJWTAuthenticator failed: %v", err)
	}

	// Create a token with a different secret
	claims := &JWTClaims{
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   "user123",
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
		},
		Role: "admin",
	}

	wrongSecret := []byte("wrong-secret")
	token, err := GenerateToken(wrongSecret, claims, jwt.SigningMethodHS256)
	if err != nil {
		t.Fatalf("GenerateToken failed: %v", err)
	}

	ctx := context.Background()
	_, err = auth.Authenticate(ctx, token)
	if !errors.Is(err, ErrInvalidSignature) {
		t.Errorf("Expected ErrInvalidSignature, got %v", err)
	}
}

func TestJWTAuthenticator_ValidateIssuer(t *testing.T) {
	secret := []byte("my-super-secret-key-for-testing-purposes")
	cfg := config.JWTAuthConfig{
		Secret: string(secret),
		Issuer: "expected-issuer",
	}

	auth, err := NewJWTAuthenticator(cfg)
	if err != nil {
		t.Fatalf("NewJWTAuthenticator failed: %v", err)
	}

	// Token with wrong issuer
	claims := &JWTClaims{
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   "user123",
			Issuer:    "wrong-issuer",
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
		},
		Role: "admin",
	}

	token, err := GenerateToken(secret, claims, jwt.SigningMethodHS256)
	if err != nil {
		t.Fatalf("GenerateToken failed: %v", err)
	}

	ctx := context.Background()
	_, err = auth.Authenticate(ctx, token)
	if !errors.Is(err, ErrInvalidIssuer) {
		t.Errorf("Expected ErrInvalidIssuer, got %v", err)
	}

	// Token with correct issuer
	claims.Issuer = "expected-issuer"
	token, err = GenerateToken(secret, claims, jwt.SigningMethodHS256)
	if err != nil {
		t.Fatalf("GenerateToken failed: %v", err)
	}

	principal, err := auth.Authenticate(ctx, token)
	if err != nil {
		t.Fatalf("Expected success with correct issuer, got: %v", err)
	}
	if principal.Metadata["issuer"] != "expected-issuer" {
		t.Errorf("Expected issuer in metadata")
	}
}

func TestJWTAuthenticator_ValidateAudience(t *testing.T) {
	secret := []byte("my-super-secret-key-for-testing-purposes")
	cfg := config.JWTAuthConfig{
		Secret:   string(secret),
		Audience: "my-api",
	}

	auth, err := NewJWTAuthenticator(cfg)
	if err != nil {
		t.Fatalf("NewJWTAuthenticator failed: %v", err)
	}

	// Token with wrong audience
	claims := &JWTClaims{
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   "user123",
			Audience:  jwt.ClaimStrings{"wrong-api"},
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
		},
		Role: "admin",
	}

	token, err := GenerateToken(secret, claims, jwt.SigningMethodHS256)
	if err != nil {
		t.Fatalf("GenerateToken failed: %v", err)
	}

	ctx := context.Background()
	_, err = auth.Authenticate(ctx, token)
	if !errors.Is(err, ErrInvalidAudience) {
		t.Errorf("Expected ErrInvalidAudience, got %v", err)
	}

	// Token with correct audience
	claims.Audience = jwt.ClaimStrings{"my-api", "other-api"}
	token, err = GenerateToken(secret, claims, jwt.SigningMethodHS256)
	if err != nil {
		t.Fatalf("GenerateToken failed: %v", err)
	}

	_, err = auth.Authenticate(ctx, token)
	if err != nil {
		t.Fatalf("Expected success with correct audience, got: %v", err)
	}
}

func TestJWTAuthenticator_Roles(t *testing.T) {
	secret := []byte("my-super-secret-key-for-testing-purposes")
	cfg := config.JWTAuthConfig{
		Secret: string(secret),
	}

	auth, err := NewJWTAuthenticator(cfg)
	if err != nil {
		t.Fatalf("NewJWTAuthenticator failed: %v", err)
	}

	tests := []struct {
		name     string
		role     string
		expected Role
	}{
		{"admin", "admin", RoleAdmin},
		{"Admin uppercase", "Admin", RoleAdmin},
		{"ADMIN all caps", "ADMIN", RoleAdmin},
		{"operator", "operator", RoleOperator},
		{"readonly", "readonly", RoleReadonly},
		{"read-only hyphen", "read-only", RoleReadonly},
		{"viewer alias", "viewer", RoleReadonly},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			claims := &JWTClaims{
				RegisteredClaims: jwt.RegisteredClaims{
					Subject:   "user123",
					ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
				},
				Role: tt.role,
			}

			token, err := GenerateToken(secret, claims, jwt.SigningMethodHS256)
			if err != nil {
				t.Fatalf("GenerateToken failed: %v", err)
			}

			ctx := context.Background()
			principal, err := auth.Authenticate(ctx, token)
			if err != nil {
				t.Fatalf("Authenticate failed: %v", err)
			}

			if principal.Role != tt.expected {
				t.Errorf("Expected role %s, got %s", tt.expected, principal.Role)
			}
		})
	}
}

func TestJWTAuthenticator_DefaultRole(t *testing.T) {
	secret := []byte("my-super-secret-key-for-testing-purposes")
	cfg := config.JWTAuthConfig{
		Secret: string(secret),
	}

	auth, err := NewJWTAuthenticator(cfg)
	if err != nil {
		t.Fatalf("NewJWTAuthenticator failed: %v", err)
	}

	// Token without role claim
	claims := &JWTClaims{
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   "user123",
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
		},
	}

	token, err := GenerateToken(secret, claims, jwt.SigningMethodHS256)
	if err != nil {
		t.Fatalf("GenerateToken failed: %v", err)
	}

	ctx := context.Background()
	principal, err := auth.Authenticate(ctx, token)
	if err != nil {
		t.Fatalf("Authenticate failed: %v", err)
	}

	// Should default to readonly
	if principal.Role != RoleReadonly {
		t.Errorf("Expected default role 'readonly', got '%s'", principal.Role)
	}
}

func TestJWTAuthenticator_EmptyCredentials(t *testing.T) {
	cfg := config.JWTAuthConfig{
		Secret: "secret",
	}

	auth, err := NewJWTAuthenticator(cfg)
	if err != nil {
		t.Fatalf("NewJWTAuthenticator failed: %v", err)
	}

	ctx := context.Background()
	_, err = auth.Authenticate(ctx, "")
	if !errors.Is(err, ErrNoCredentials) {
		t.Errorf("Expected ErrNoCredentials, got %v", err)
	}
}

func TestJWTAuthenticator_MalformedToken(t *testing.T) {
	cfg := config.JWTAuthConfig{
		Secret: "secret",
	}

	auth, err := NewJWTAuthenticator(cfg)
	if err != nil {
		t.Fatalf("NewJWTAuthenticator failed: %v", err)
	}

	ctx := context.Background()
	_, err = auth.Authenticate(ctx, "not.a.valid.jwt.token")
	if !errors.Is(err, ErrInvalidToken) {
		t.Errorf("Expected ErrInvalidToken, got %v", err)
	}
}

func TestJWTAuthenticator_RSAKey(t *testing.T) {
	// Generate RSA key pair
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("Failed to generate RSA key: %v", err)
	}

	// Write public key to temp file
	pubKeyBytes, err := x509.MarshalPKIXPublicKey(&privateKey.PublicKey)
	if err != nil {
		t.Fatalf("Failed to marshal public key: %v", err)
	}

	pubKeyPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "PUBLIC KEY",
		Bytes: pubKeyBytes,
	})

	tmpFile, err := os.CreateTemp("", "pubkey*.pem")
	if err != nil {
		t.Fatalf("Failed to create temp file: %v", err)
	}
	defer os.Remove(tmpFile.Name())

	if _, err := tmpFile.Write(pubKeyPEM); err != nil {
		t.Fatalf("Failed to write public key: %v", err)
	}
	tmpFile.Close()

	// Create authenticator with public key
	cfg := config.JWTAuthConfig{
		PublicKeyFile: tmpFile.Name(),
	}

	auth, err := NewJWTAuthenticator(cfg)
	if err != nil {
		t.Fatalf("NewJWTAuthenticator failed: %v", err)
	}

	// Create a token signed with private key
	claims := &JWTClaims{
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   "user123",
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
		},
		Role: "admin",
		Name: "RSA User",
	}

	token, err := GenerateTokenWithKey(privateKey, claims, jwt.SigningMethodRS256)
	if err != nil {
		t.Fatalf("GenerateTokenWithKey failed: %v", err)
	}

	ctx := context.Background()
	principal, err := auth.Authenticate(ctx, token)
	if err != nil {
		t.Fatalf("Authenticate failed: %v", err)
	}

	if principal.Name != "RSA User" {
		t.Errorf("Expected Name 'RSA User', got '%s'", principal.Name)
	}
	if principal.Role != RoleAdmin {
		t.Errorf("Expected Role 'admin', got '%s'", principal.Role)
	}
}

func TestJWTAuthenticator_ECDSAKey(t *testing.T) {
	// Generate ECDSA key pair
	privateKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("Failed to generate ECDSA key: %v", err)
	}

	// Write public key to temp file
	pubKeyBytes, err := x509.MarshalPKIXPublicKey(&privateKey.PublicKey)
	if err != nil {
		t.Fatalf("Failed to marshal public key: %v", err)
	}

	pubKeyPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "PUBLIC KEY",
		Bytes: pubKeyBytes,
	})

	tmpFile, err := os.CreateTemp("", "ecpubkey*.pem")
	if err != nil {
		t.Fatalf("Failed to create temp file: %v", err)
	}
	defer os.Remove(tmpFile.Name())

	if _, err := tmpFile.Write(pubKeyPEM); err != nil {
		t.Fatalf("Failed to write public key: %v", err)
	}
	tmpFile.Close()

	// Create authenticator with public key
	cfg := config.JWTAuthConfig{
		PublicKeyFile: tmpFile.Name(),
	}

	auth, err := NewJWTAuthenticator(cfg)
	if err != nil {
		t.Fatalf("NewJWTAuthenticator failed: %v", err)
	}

	// Create a token signed with private key
	claims := &JWTClaims{
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   "user456",
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
		},
		Role: "operator",
		Name: "ECDSA User",
	}

	token, err := GenerateTokenWithKey(privateKey, claims, jwt.SigningMethodES256)
	if err != nil {
		t.Fatalf("GenerateTokenWithKey failed: %v", err)
	}

	ctx := context.Background()
	principal, err := auth.Authenticate(ctx, token)
	if err != nil {
		t.Fatalf("Authenticate failed: %v", err)
	}

	if principal.Name != "ECDSA User" {
		t.Errorf("Expected Name 'ECDSA User', got '%s'", principal.Name)
	}
	if principal.Role != RoleOperator {
		t.Errorf("Expected Role 'operator', got '%s'", principal.Role)
	}
}

func TestJWTAuthenticator_SubjectFallback(t *testing.T) {
	secret := []byte("my-super-secret-key-for-testing-purposes")
	cfg := config.JWTAuthConfig{
		Secret: string(secret),
	}

	auth, err := NewJWTAuthenticator(cfg)
	if err != nil {
		t.Fatalf("NewJWTAuthenticator failed: %v", err)
	}

	tests := []struct {
		name         string
		subject      string
		email        string
		userName     string
		expectedID   string
		expectedName string
	}{
		{
			name:         "subject only",
			subject:      "user123",
			expectedID:   "jwt:user123",
			expectedName: "user123",
		},
		{
			name:         "email fallback",
			email:        "test@example.com",
			expectedID:   "jwt:test@example.com",
			expectedName: "test@example.com",
		},
		{
			name:         "name fallback",
			userName:     "Test User",
			expectedID:   "jwt:Test User",
			expectedName: "Test User",
		},
		{
			name:         "subject with name",
			subject:      "user123",
			userName:     "Test User",
			expectedID:   "jwt:user123",
			expectedName: "Test User",
		},
		{
			name:         "no identifiers",
			expectedID:   "jwt:unknown",
			expectedName: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			claims := &JWTClaims{
				RegisteredClaims: jwt.RegisteredClaims{
					Subject:   tt.subject,
					ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
				},
				Name:  tt.userName,
				Email: tt.email,
			}

			token, err := GenerateToken(secret, claims, jwt.SigningMethodHS256)
			if err != nil {
				t.Fatalf("GenerateToken failed: %v", err)
			}

			ctx := context.Background()
			principal, err := auth.Authenticate(ctx, token)
			if err != nil {
				t.Fatalf("Authenticate failed: %v", err)
			}

			if principal.ID != tt.expectedID {
				t.Errorf("Expected ID '%s', got '%s'", tt.expectedID, principal.ID)
			}
			if principal.Name != tt.expectedName {
				t.Errorf("Expected Name '%s', got '%s'", tt.expectedName, principal.Name)
			}
		})
	}
}
