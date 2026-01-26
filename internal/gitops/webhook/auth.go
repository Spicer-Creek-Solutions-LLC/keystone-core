package webhook

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"strings"
)

// Authenticator validates webhook requests
type Authenticator interface {
	Authenticate(r *http.Request, body []byte) error
}

// NoneAuthenticator allows all requests (no authentication)
type NoneAuthenticator struct{}

func (a *NoneAuthenticator) Authenticate(r *http.Request, body []byte) error {
	return nil
}

// HMACAuthenticator validates HMAC signatures
type HMACAuthenticator struct {
	Secret string
	Header string // Header containing the signature (e.g., "X-Hub-Signature-256")
}

func (a *HMACAuthenticator) Authenticate(r *http.Request, body []byte) error {
	signature := r.Header.Get(a.Header)
	if signature == "" {
		return fmt.Errorf("missing signature header: %s", a.Header)
	}

	// GitHub uses "sha256=" prefix, strip it if present
	signature = strings.TrimPrefix(signature, "sha256=")

	// Compute expected signature
	mac := hmac.New(sha256.New, []byte(a.Secret))
	mac.Write(body)
	expected := hex.EncodeToString(mac.Sum(nil))

	if !hmac.Equal([]byte(signature), []byte(expected)) {
		return fmt.Errorf("invalid signature")
	}

	return nil
}

// BearerAuthenticator validates bearer tokens
type BearerAuthenticator struct {
	Token string
}

func (a *BearerAuthenticator) Authenticate(r *http.Request, body []byte) error {
	authHeader := r.Header.Get("Authorization")
	if authHeader == "" {
		return fmt.Errorf("missing authorization header")
	}

	// Extract bearer token
	parts := strings.Split(authHeader, " ")
	if len(parts) != 2 || strings.ToLower(parts[0]) != "bearer" {
		return fmt.Errorf("invalid authorization header format")
	}

	token := parts[1]
	if token != a.Token {
		return fmt.Errorf("invalid bearer token")
	}

	return nil
}

// NewAuthenticator creates an authenticator based on config
func NewAuthenticator(config AuthConfig) Authenticator {
	switch config.Type {
	case AuthTypeHMAC:
		return &HMACAuthenticator{
			Secret: config.Secret,
			Header: "X-Hub-Signature-256",
		}
	case AuthTypeBearer:
		return &BearerAuthenticator{
			Token: config.Token,
		}
	default:
		return &NoneAuthenticator{}
	}
}
