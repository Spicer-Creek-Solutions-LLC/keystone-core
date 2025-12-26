package webhook

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"testing"
)

func TestNoneAuthenticator(t *testing.T) {
	auth := &NoneAuthenticator{}
	req := &http.Request{}
	body := []byte("test")

	err := auth.Authenticate(req, body)
	if err != nil {
		t.Errorf("NoneAuthenticator should not return error, got: %v", err)
	}
}

func TestHMACAuthenticator(t *testing.T) {
	secret := "my-secret"
	body := []byte("test payload")

	// Compute valid signature
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	validSig := hex.EncodeToString(mac.Sum(nil))

	tests := []struct {
		name      string
		signature string
		wantErr   bool
	}{
		{
			name:      "valid signature",
			signature: validSig,
			wantErr:   false,
		},
		{
			name:      "valid signature with prefix",
			signature: "sha256=" + validSig,
			wantErr:   false,
		},
		{
			name:      "invalid signature",
			signature: "invalid",
			wantErr:   true,
		},
		{
			name:      "missing signature",
			signature: "",
			wantErr:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			auth := &HMACAuthenticator{
				Secret: secret,
				Header: "X-Hub-Signature-256",
			}

			req := &http.Request{
				Header: http.Header{},
			}
			if tt.signature != "" {
				req.Header.Set("X-Hub-Signature-256", tt.signature)
			}

			err := auth.Authenticate(req, body)
			if (err != nil) != tt.wantErr {
				t.Errorf("HMACAuthenticator.Authenticate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestBearerAuthenticator(t *testing.T) {
	token := "my-token"

	tests := []struct {
		name    string
		header  string
		wantErr bool
	}{
		{
			name:    "valid token",
			header:  "Bearer my-token",
			wantErr: false,
		},
		{
			name:    "invalid token",
			header:  "Bearer wrong-token",
			wantErr: true,
		},
		{
			name:    "missing bearer prefix",
			header:  "my-token",
			wantErr: true,
		},
		{
			name:    "missing header",
			header:  "",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			auth := &BearerAuthenticator{
				Token: token,
			}

			req := &http.Request{
				Header: http.Header{},
			}
			if tt.header != "" {
				req.Header.Set("Authorization", tt.header)
			}

			err := auth.Authenticate(req, []byte("test"))
			if (err != nil) != tt.wantErr {
				t.Errorf("BearerAuthenticator.Authenticate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestNewAuthenticator(t *testing.T) {
	tests := []struct {
		name     string
		config   AuthConfig
		wantType string
	}{
		{
			name:     "none authenticator",
			config:   AuthConfig{Type: AuthTypeNone},
			wantType: "*webhook.NoneAuthenticator",
		},
		{
			name:     "hmac authenticator",
			config:   AuthConfig{Type: AuthTypeHMAC, Secret: "secret"},
			wantType: "*webhook.HMACAuthenticator",
		},
		{
			name:     "bearer authenticator",
			config:   AuthConfig{Type: AuthTypeBearer, Token: "token"},
			wantType: "*webhook.BearerAuthenticator",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			auth := NewAuthenticator(tt.config)
			typeName := fmt.Sprintf("%T", auth)
			if typeName != tt.wantType {
				t.Errorf("NewAuthenticator() type = %v, want %v", typeName, tt.wantType)
			}
		})
	}
}
