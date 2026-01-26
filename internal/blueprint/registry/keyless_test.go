package registry

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/base64"
	"encoding/pem"
	"math/big"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestDefaultKeylessSigningConfig(t *testing.T) {
	config := DefaultKeylessSigningConfig()

	if config.FulcioURL != DefaultFulcioURL {
		t.Errorf("FulcioURL = %s, want %s", config.FulcioURL, DefaultFulcioURL)
	}
	if config.RekorURL != DefaultRekorURL {
		t.Errorf("RekorURL = %s, want %s", config.RekorURL, DefaultRekorURL)
	}
	if config.OIDCIssuer != DefaultOIDCIssuer {
		t.Errorf("OIDCIssuer = %s, want %s", config.OIDCIssuer, DefaultOIDCIssuer)
	}
	if config.OIDCClientID != "sigstore" {
		t.Errorf("OIDCClientID = %s, want sigstore", config.OIDCClientID)
	}
	if config.Timeout != 60*time.Second {
		t.Errorf("Timeout = %v, want 60s", config.Timeout)
	}
}

func TestNewKeylessSigner(t *testing.T) {
	t.Run("nil config uses defaults", func(t *testing.T) {
		signer, err := NewKeylessSigner(nil)
		if err != nil {
			t.Fatalf("NewKeylessSigner failed: %v", err)
		}
		if signer.config.FulcioURL != DefaultFulcioURL {
			t.Errorf("FulcioURL = %s, want %s", signer.config.FulcioURL, DefaultFulcioURL)
		}
	})

	t.Run("custom config preserved", func(t *testing.T) {
		config := &KeylessSigningConfig{
			FulcioURL:    "https://custom-fulcio.example.com",
			RekorURL:     "https://custom-rekor.example.com",
			OIDCIssuer:   "https://custom-issuer.example.com",
			OIDCClientID: "custom-client",
			Timeout:      30 * time.Second,
		}
		signer, err := NewKeylessSigner(config)
		if err != nil {
			t.Fatalf("NewKeylessSigner failed: %v", err)
		}
		if signer.config.FulcioURL != config.FulcioURL {
			t.Errorf("FulcioURL = %s, want %s", signer.config.FulcioURL, config.FulcioURL)
		}
		if signer.config.RekorURL != config.RekorURL {
			t.Errorf("RekorURL = %s, want %s", signer.config.RekorURL, config.RekorURL)
		}
	})

	t.Run("empty URLs filled with defaults", func(t *testing.T) {
		config := &KeylessSigningConfig{
			OIDCToken: "test-token",
		}
		signer, err := NewKeylessSigner(config)
		if err != nil {
			t.Fatalf("NewKeylessSigner failed: %v", err)
		}
		if signer.config.FulcioURL != DefaultFulcioURL {
			t.Errorf("FulcioURL = %s, want %s", signer.config.FulcioURL, DefaultFulcioURL)
		}
		if signer.config.RekorURL != DefaultRekorURL {
			t.Errorf("RekorURL = %s, want %s", signer.config.RekorURL, DefaultRekorURL)
		}
	})
}

func TestKeylessSigner_Sign_RequiresToken(t *testing.T) {
	signer, err := NewKeylessSigner(&KeylessSigningConfig{
		// No OIDC token provided
	})
	if err != nil {
		t.Fatalf("NewKeylessSigner failed: %v", err)
	}

	_, err = signer.Sign(context.Background(), []byte("test data"))
	if err == nil {
		t.Error("expected error for missing OIDC token")
	}
	if err.Error() != "OIDC token required for keyless signing; set OIDCToken in config or use device flow" {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestKeylessSigner_SignBlueprint(t *testing.T) {
	tmpDir := t.TempDir()
	archivePath := filepath.Join(tmpDir, "blueprint.tar.gz")
	if err := os.WriteFile(archivePath, []byte("test archive content"), 0644); err != nil {
		t.Fatalf("failed to create archive: %v", err)
	}

	signer, err := NewKeylessSigner(&KeylessSigningConfig{
		// No token - will fail but at least tests file reading
	})
	if err != nil {
		t.Fatalf("NewKeylessSigner failed: %v", err)
	}

	// Should fail due to missing token, not file reading
	_, err = signer.SignBlueprint(context.Background(), archivePath)
	if err == nil {
		t.Error("expected error for missing OIDC token")
	}
}

func TestKeylessSigner_SignBlueprint_FileNotFound(t *testing.T) {
	signer, err := NewKeylessSigner(&KeylessSigningConfig{
		OIDCToken: "test-token", // Provide token so we test file reading
	})
	if err != nil {
		t.Fatalf("NewKeylessSigner failed: %v", err)
	}

	_, err = signer.SignBlueprint(context.Background(), "/nonexistent/archive.tar.gz")
	if err == nil {
		t.Error("expected error for nonexistent archive")
	}
}

func TestVerifyKeylessSignature(t *testing.T) {
	// Generate a test key pair
	privateKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("failed to generate key: %v", err)
	}

	// Create a self-signed certificate
	template := x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject: pkix.Name{
			CommonName: "test@example.com",
		},
		NotBefore:             time.Now().Add(-1 * time.Hour),
		NotAfter:              time.Now().Add(1 * time.Hour),
		KeyUsage:              x509.KeyUsageDigitalSignature,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageCodeSigning},
		BasicConstraintsValid: true,
	}

	certDER, err := x509.CreateCertificate(rand.Reader, &template, &template, &privateKey.PublicKey, privateKey)
	if err != nil {
		t.Fatalf("failed to create certificate: %v", err)
	}

	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certDER})

	// Sign some data
	data := []byte("test data to sign")
	hash := sha256.Sum256(data)
	signature, err := ecdsa.SignASN1(rand.Reader, privateKey, hash[:])
	if err != nil {
		t.Fatalf("failed to sign: %v", err)
	}
	sigBase64 := base64.StdEncoding.EncodeToString(signature)

	t.Run("valid signature", func(t *testing.T) {
		result, err := VerifyKeylessSignature(context.Background(), data, sigBase64, string(certPEM))
		if err != nil {
			t.Fatalf("VerifyKeylessSignature failed: %v", err)
		}
		if !result.Valid {
			t.Errorf("signature should be valid, errors: %v", result.Errors)
		}
		if result.Digest == "" {
			t.Error("digest should not be empty")
		}
	})

	t.Run("invalid signature", func(t *testing.T) {
		wrongSig := base64.StdEncoding.EncodeToString([]byte("wrong signature"))
		result, err := VerifyKeylessSignature(context.Background(), data, wrongSig, string(certPEM))
		if err != nil {
			t.Fatalf("VerifyKeylessSignature failed: %v", err)
		}
		if result.Valid {
			t.Error("signature should be invalid")
		}
	})

	t.Run("invalid signature base64", func(t *testing.T) {
		result, err := VerifyKeylessSignature(context.Background(), data, "not-base64!", string(certPEM))
		if err != nil {
			t.Fatalf("VerifyKeylessSignature failed: %v", err)
		}
		if result.Valid {
			t.Error("signature should be invalid")
		}
		if len(result.Errors) == 0 {
			t.Error("expected errors")
		}
	})

	t.Run("invalid certificate PEM", func(t *testing.T) {
		result, err := VerifyKeylessSignature(context.Background(), data, sigBase64, "not a pem")
		if err != nil {
			t.Fatalf("VerifyKeylessSignature failed: %v", err)
		}
		if result.Valid {
			t.Error("signature should be invalid")
		}
		if len(result.Errors) == 0 {
			t.Error("expected errors")
		}
	})

	t.Run("expired certificate", func(t *testing.T) {
		// Create an expired certificate
		expiredTemplate := x509.Certificate{
			SerialNumber: big.NewInt(2),
			Subject: pkix.Name{
				CommonName: "expired@example.com",
			},
			NotBefore:             time.Now().Add(-48 * time.Hour),
			NotAfter:              time.Now().Add(-24 * time.Hour),
			KeyUsage:              x509.KeyUsageDigitalSignature,
			BasicConstraintsValid: true,
		}

		expiredCertDER, err := x509.CreateCertificate(rand.Reader, &expiredTemplate, &expiredTemplate, &privateKey.PublicKey, privateKey)
		if err != nil {
			t.Fatalf("failed to create certificate: %v", err)
		}
		expiredCertPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: expiredCertDER})

		result, err := VerifyKeylessSignature(context.Background(), data, sigBase64, string(expiredCertPEM))
		if err != nil {
			t.Fatalf("VerifyKeylessSignature failed: %v", err)
		}
		// Signature is still valid (ECDSA verification succeeds)
		if !result.Valid {
			t.Error("signature should be valid (expired cert is a warning)")
		}
		// But should have warnings about expiration
		if len(result.Warnings) == 0 {
			t.Error("expected warnings about certificate expiration")
		}
	})
}

func TestGetOIDCToken(t *testing.T) {
	// GetOIDCToken is a placeholder that returns an error
	_, err := GetOIDCToken(context.Background(), "https://issuer.example.com", "client-id")
	if err == nil {
		t.Error("expected error from placeholder")
	}
}

func TestSigstoreConstants(t *testing.T) {
	if DefaultFulcioURL != "https://fulcio.sigstore.dev" {
		t.Errorf("DefaultFulcioURL = %s, want https://fulcio.sigstore.dev", DefaultFulcioURL)
	}
	if DefaultRekorURL != "https://rekor.sigstore.dev" {
		t.Errorf("DefaultRekorURL = %s, want https://rekor.sigstore.dev", DefaultRekorURL)
	}
	if DefaultOIDCIssuer != "https://oauth2.sigstore.dev/auth" {
		t.Errorf("DefaultOIDCIssuer = %s, want https://oauth2.sigstore.dev/auth", DefaultOIDCIssuer)
	}
}

func TestRekorEntry(t *testing.T) {
	entry := &RekorEntry{
		LogIndex:       123,
		UUID:           "test-uuid",
		IntegratedTime: 1704067200,
		LogID:          "test-log-id",
	}

	if entry.LogIndex != 123 {
		t.Errorf("LogIndex = %d, want 123", entry.LogIndex)
	}
	if entry.UUID != "test-uuid" {
		t.Errorf("UUID = %s, want test-uuid", entry.UUID)
	}
}

func TestKeylessSigningResult_Fields(t *testing.T) {
	result := &KeylessSigningResult{
		Signature:        "test-sig",
		Certificate:      "test-cert",
		CertificateChain: "test-chain",
		Digest:           "sha256:abc",
		Timestamp:        time.Now(),
		SignerIdentity:   "user@example.com",
		Annotations: map[string]string{
			"key": "value",
		},
	}

	if result.Signature != "test-sig" {
		t.Errorf("Signature = %s, want test-sig", result.Signature)
	}
	if result.Certificate != "test-cert" {
		t.Errorf("Certificate = %s, want test-cert", result.Certificate)
	}
	if result.SignerIdentity != "user@example.com" {
		t.Errorf("SignerIdentity = %s, want user@example.com", result.SignerIdentity)
	}
	if result.Annotations["key"] != "value" {
		t.Error("Annotations not preserved")
	}
}
