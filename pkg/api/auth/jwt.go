package auth

import (
	"context"
	"crypto/ecdsa"
	"crypto/rsa"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"

	"github.com/shawnbutts/keystone-core/internal/config"
)

// Standard JWT errors
var (
	ErrInvalidToken      = errors.New("invalid token")
	ErrTokenExpired      = errors.New("token expired")
	ErrInvalidIssuer     = errors.New("invalid issuer")
	ErrInvalidAudience   = errors.New("invalid audience")
	ErrInvalidSignature  = errors.New("invalid signature")
	ErrUnsupportedAlg    = errors.New("unsupported signing algorithm")
	ErrMissingRoleClaim  = errors.New("missing role claim")
	ErrInvalidRoleClaim  = errors.New("invalid role claim")
	ErrNoSigningKey      = errors.New("no signing key configured")
)

// JWTClaims represents the expected JWT claims structure
type JWTClaims struct {
	jwt.RegisteredClaims
	// Role is the user's role (admin, operator, readonly)
	Role string `json:"role,omitempty"`
	// Name is the user's name (optional)
	Name string `json:"name,omitempty"`
	// Email is the user's email (optional)
	Email string `json:"email,omitempty"`
}

// JWTAuthenticator authenticates using JWT tokens
type JWTAuthenticator struct {
	// config holds the JWT configuration
	config config.JWTAuthConfig
	// hmacSecret for HS256/HS384/HS512
	hmacSecret []byte
	// rsaPublicKey for RS256/RS384/RS512
	rsaPublicKey *rsa.PublicKey
	// ecdsaPublicKey for ES256/ES384/ES512
	ecdsaPublicKey *ecdsa.PublicKey
	// roleClaim is the claim name containing the role
	roleClaim string
}

// NewJWTAuthenticator creates a new JWT authenticator from config
func NewJWTAuthenticator(cfg config.JWTAuthConfig) (*JWTAuthenticator, error) {
	auth := &JWTAuthenticator{
		config:    cfg,
		roleClaim: cfg.RoleClaim,
	}

	// Default role claim
	if auth.roleClaim == "" {
		auth.roleClaim = "role"
	}

	// Configure signing key
	if cfg.Secret != "" {
		// HMAC secret
		auth.hmacSecret = []byte(cfg.Secret)
	} else if cfg.PublicKeyFile != "" {
		// Load public key from file
		keyData, err := os.ReadFile(cfg.PublicKeyFile)
		if err != nil {
			return nil, fmt.Errorf("failed to read public key file: %w", err)
		}

		// Try parsing as RSA first
		rsaKey, err := jwt.ParseRSAPublicKeyFromPEM(keyData)
		if err == nil {
			auth.rsaPublicKey = rsaKey
		} else {
			// Try parsing as ECDSA
			ecdsaKey, err := jwt.ParseECPublicKeyFromPEM(keyData)
			if err != nil {
				return nil, fmt.Errorf("failed to parse public key (tried RSA and ECDSA): %w", err)
			}
			auth.ecdsaPublicKey = ecdsaKey
		}
	} else {
		return nil, ErrNoSigningKey
	}

	return auth, nil
}

// Name returns the authenticator name
func (a *JWTAuthenticator) Name() string {
	return "jwt"
}

// Authenticate validates a JWT token and returns a Principal
func (a *JWTAuthenticator) Authenticate(ctx context.Context, credentials string) (*Principal, error) {
	if credentials == "" {
		return nil, ErrNoCredentials
	}

	// Parse and validate the token
	token, err := jwt.ParseWithClaims(credentials, &JWTClaims{}, a.keyFunc)
	if err != nil {
		return nil, a.mapJWTError(err)
	}

	if !token.Valid {
		return nil, ErrInvalidToken
	}

	// Extract claims
	claims, ok := token.Claims.(*JWTClaims)
	if !ok {
		return nil, ErrInvalidToken
	}

	// Validate issuer if configured
	if a.config.Issuer != "" {
		issuer, err := claims.GetIssuer()
		if err != nil || issuer != a.config.Issuer {
			return nil, ErrInvalidIssuer
		}
	}

	// Validate audience if configured
	if a.config.Audience != "" {
		audiences, err := claims.GetAudience()
		if err != nil {
			return nil, ErrInvalidAudience
		}
		found := false
		for _, aud := range audiences {
			if aud == a.config.Audience {
				found = true
				break
			}
		}
		if !found {
			return nil, ErrInvalidAudience
		}
	}

	// Extract role from claims
	role, err := a.extractRole(claims)
	if err != nil {
		return nil, err
	}

	// Build subject ID
	subjectID := "jwt:"
	if sub, err := claims.GetSubject(); err == nil && sub != "" {
		subjectID += sub
	} else if claims.Email != "" {
		subjectID += claims.Email
	} else if claims.Name != "" {
		subjectID += claims.Name
	} else {
		subjectID += "unknown"
	}

	// Build display name
	name := claims.Name
	if name == "" {
		name = claims.Email
		if name == "" {
			if sub, err := claims.GetSubject(); err == nil {
				name = sub
			}
		}
	}

	// Create principal
	principal := &Principal{
		ID:              subjectID,
		Name:            name,
		Role:            role,
		AuthMethod:      "jwt",
		AuthenticatedAt: time.Now(),
		Metadata:        make(map[string]string),
	}

	// Add metadata from claims
	if claims.Email != "" {
		principal.Metadata["email"] = claims.Email
	}
	if iss, err := claims.GetIssuer(); err == nil && iss != "" {
		principal.Metadata["issuer"] = iss
	}
	if claims.ID != "" {
		principal.Metadata["jti"] = claims.ID
	}

	return principal, nil
}

// keyFunc returns the key for validating the token signature
func (a *JWTAuthenticator) keyFunc(token *jwt.Token) (interface{}, error) {
	// Check signing method
	switch token.Method.Alg() {
	case "HS256", "HS384", "HS512":
		if a.hmacSecret == nil {
			return nil, ErrNoSigningKey
		}
		return a.hmacSecret, nil

	case "RS256", "RS384", "RS512":
		if a.rsaPublicKey == nil {
			return nil, ErrNoSigningKey
		}
		return a.rsaPublicKey, nil

	case "ES256", "ES384", "ES512":
		if a.ecdsaPublicKey == nil {
			return nil, ErrNoSigningKey
		}
		return a.ecdsaPublicKey, nil

	default:
		return nil, fmt.Errorf("%w: %s", ErrUnsupportedAlg, token.Method.Alg())
	}
}

// extractRole extracts the role from JWT claims
func (a *JWTAuthenticator) extractRole(claims *JWTClaims) (Role, error) {
	// First check the typed Role field (if roleClaim is "role")
	if a.roleClaim == "role" && claims.Role != "" {
		return a.parseRole(claims.Role)
	}

	// For custom role claims, we'd need to parse the raw claims
	// The standard JWTClaims struct has a Role field, so this covers most cases
	if claims.Role != "" {
		return a.parseRole(claims.Role)
	}

	// Default to readonly if no role specified
	return RoleReadonly, nil
}

// parseRole converts a string role to Role type
func (a *JWTAuthenticator) parseRole(roleStr string) (Role, error) {
	switch strings.ToLower(roleStr) {
	case "admin":
		return RoleAdmin, nil
	case "operator":
		return RoleOperator, nil
	case "readonly", "read-only", "viewer":
		return RoleReadonly, nil
	default:
		return "", fmt.Errorf("%w: %s", ErrInvalidRoleClaim, roleStr)
	}
}

// mapJWTError converts jwt library errors to our error types
func (a *JWTAuthenticator) mapJWTError(err error) error {
	if err == nil {
		return nil
	}

	// Check for specific JWT errors
	if errors.Is(err, jwt.ErrTokenExpired) {
		return ErrTokenExpired
	}
	if errors.Is(err, jwt.ErrSignatureInvalid) {
		return ErrInvalidSignature
	}
	if errors.Is(err, jwt.ErrTokenMalformed) {
		return ErrInvalidToken
	}
	if errors.Is(err, jwt.ErrTokenNotValidYet) {
		return ErrInvalidToken
	}

	// Check error message for common cases
	errStr := err.Error()
	if strings.Contains(errStr, "expired") {
		return ErrTokenExpired
	}
	if strings.Contains(errStr, "signature") {
		return ErrInvalidSignature
	}

	return fmt.Errorf("%w: %v", ErrInvalidToken, err)
}

// GenerateToken creates a new JWT token (useful for testing and token issuance)
func GenerateToken(secret []byte, claims *JWTClaims, method jwt.SigningMethod) (string, error) {
	if method == nil {
		method = jwt.SigningMethodHS256
	}

	token := jwt.NewWithClaims(method, claims)
	return token.SignedString(secret)
}

// GenerateTokenWithKey creates a new JWT token with an RSA or ECDSA private key
func GenerateTokenWithKey(key interface{}, claims *JWTClaims, method jwt.SigningMethod) (string, error) {
	token := jwt.NewWithClaims(method, claims)
	return token.SignedString(key)
}
