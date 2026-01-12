package registry

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestGenerateKeyPair(t *testing.T) {
	tests := []struct {
		name    string
		keyType KeyType
		bits    int
	}{
		{"ECDSA P-256", KeyTypeECDSA, 256},
		{"ECDSA P-384", KeyTypeECDSA, 384},
		{"RSA 2048", KeyTypeRSA, 2048},
		{"RSA 4096", KeyTypeRSA, 4096},
		{"Ed25519", KeyTypeEd25519, 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			priv, pub, err := GenerateKeyPair(tt.keyType, tt.bits)
			if err != nil {
				t.Fatalf("GenerateKeyPair failed: %v", err)
			}

			if len(priv) == 0 {
				t.Error("private key is empty")
			}
			if len(pub) == 0 {
				t.Error("public key is empty")
			}

			// Verify PEM format
			if !containsBytes(priv, []byte("-----BEGIN PRIVATE KEY-----")) {
				t.Error("private key is not in PEM format")
			}
			if !containsBytes(pub, []byte("-----BEGIN PUBLIC KEY-----")) {
				t.Error("public key is not in PEM format")
			}
		})
	}
}

func TestGenerateKeyPair_InvalidType(t *testing.T) {
	_, _, err := GenerateKeyPair("invalid", 256)
	if err == nil {
		t.Error("expected error for invalid key type")
	}
}

func TestNewSigner(t *testing.T) {
	// Generate a key pair for testing
	priv, _, err := GenerateKeyPair(KeyTypeECDSA, 256)
	if err != nil {
		t.Fatalf("GenerateKeyPair failed: %v", err)
	}

	// Write to temp file
	tmpDir := t.TempDir()
	keyPath := filepath.Join(tmpDir, "private.key")
	if err := os.WriteFile(keyPath, priv, 0600); err != nil {
		t.Fatalf("failed to write key file: %v", err)
	}

	t.Run("success", func(t *testing.T) {
		signer, err := NewSigner(&SigningConfig{
			KeyPath: keyPath,
		})
		if err != nil {
			t.Fatalf("NewSigner failed: %v", err)
		}
		if signer.KeyType() != KeyTypeECDSA {
			t.Errorf("key type = %s, want %s", signer.KeyType(), KeyTypeECDSA)
		}
	})

	t.Run("nil config", func(t *testing.T) {
		_, err := NewSigner(nil)
		if err == nil {
			t.Error("expected error for nil config")
		}
	})

	t.Run("missing key path", func(t *testing.T) {
		_, err := NewSigner(&SigningConfig{})
		if err == nil {
			t.Error("expected error for missing key path")
		}
	})

	t.Run("nonexistent key file", func(t *testing.T) {
		_, err := NewSigner(&SigningConfig{
			KeyPath: "/nonexistent/path/key.pem",
		})
		if err == nil {
			t.Error("expected error for nonexistent key file")
		}
	})
}

func TestSigner_Sign(t *testing.T) {
	// Test with each key type
	keyTypes := []KeyType{KeyTypeECDSA, KeyTypeRSA, KeyTypeEd25519}

	for _, kt := range keyTypes {
		t.Run(string(kt), func(t *testing.T) {
			priv, _, err := GenerateKeyPair(kt, 2048)
			if err != nil {
				t.Fatalf("GenerateKeyPair failed: %v", err)
			}

			tmpDir := t.TempDir()
			keyPath := filepath.Join(tmpDir, "private.key")
			if err := os.WriteFile(keyPath, priv, 0600); err != nil {
				t.Fatalf("failed to write key file: %v", err)
			}

			signer, err := NewSigner(&SigningConfig{
				KeyPath: keyPath,
				Annotations: map[string]string{
					"vendor": "test",
				},
			})
			if err != nil {
				t.Fatalf("NewSigner failed: %v", err)
			}

			data := []byte("test data to sign")
			result, err := signer.Sign(context.Background(), data)
			if err != nil {
				t.Fatalf("Sign failed: %v", err)
			}

			if result.Signature == "" {
				t.Error("signature is empty")
			}
			if result.Digest == "" {
				t.Error("digest is empty")
			}
			if result.Timestamp.IsZero() {
				t.Error("timestamp is zero")
			}
			if result.Annotations["vendor"] != "test" {
				t.Error("annotations not preserved")
			}
		})
	}
}

func TestSigner_Sign_BundleFormat(t *testing.T) {
	priv, _, err := GenerateKeyPair(KeyTypeECDSA, 256)
	if err != nil {
		t.Fatalf("GenerateKeyPair failed: %v", err)
	}

	tmpDir := t.TempDir()
	keyPath := filepath.Join(tmpDir, "private.key")
	if err := os.WriteFile(keyPath, priv, 0600); err != nil {
		t.Fatalf("failed to write key file: %v", err)
	}

	signer, err := NewSigner(&SigningConfig{
		KeyPath: keyPath,
		Format:  SignatureFormatBundle,
		Annotations: map[string]string{
			"version": "1.0.0",
		},
	})
	if err != nil {
		t.Fatalf("NewSigner failed: %v", err)
	}

	data := []byte("test data to sign")
	result, err := signer.Sign(context.Background(), data)
	if err != nil {
		t.Fatalf("Sign failed: %v", err)
	}

	if result.Bundle == nil {
		t.Fatal("bundle is nil")
	}
	if result.Bundle.PayloadType != "application/vnd.kscore.blueprint.v1+json" {
		t.Errorf("payload type = %s, want application/vnd.kscore.blueprint.v1+json", result.Bundle.PayloadType)
	}
	if len(result.Bundle.Signatures) == 0 {
		t.Error("no signatures in bundle")
	}
	if result.Bundle.Signatures[0].Sig == "" {
		t.Error("signature in bundle is empty")
	}
}

func TestSigner_SignBlueprint(t *testing.T) {
	priv, _, err := GenerateKeyPair(KeyTypeECDSA, 256)
	if err != nil {
		t.Fatalf("GenerateKeyPair failed: %v", err)
	}

	tmpDir := t.TempDir()
	keyPath := filepath.Join(tmpDir, "private.key")
	if err := os.WriteFile(keyPath, priv, 0600); err != nil {
		t.Fatalf("failed to write key file: %v", err)
	}

	// Create a test archive file
	archivePath := filepath.Join(tmpDir, "blueprint.tar.gz")
	if err := os.WriteFile(archivePath, []byte("fake archive content"), 0644); err != nil {
		t.Fatalf("failed to create archive: %v", err)
	}

	signer, err := NewSigner(&SigningConfig{
		KeyPath: keyPath,
	})
	if err != nil {
		t.Fatalf("NewSigner failed: %v", err)
	}

	result, err := signer.SignBlueprint(context.Background(), archivePath)
	if err != nil {
		t.Fatalf("SignBlueprint failed: %v", err)
	}

	if result.Signature == "" {
		t.Error("signature is empty")
	}
}

func TestSigner_SignBlueprint_FileNotFound(t *testing.T) {
	priv, _, err := GenerateKeyPair(KeyTypeECDSA, 256)
	if err != nil {
		t.Fatalf("GenerateKeyPair failed: %v", err)
	}

	tmpDir := t.TempDir()
	keyPath := filepath.Join(tmpDir, "private.key")
	if err := os.WriteFile(keyPath, priv, 0600); err != nil {
		t.Fatalf("failed to write key file: %v", err)
	}

	signer, err := NewSigner(&SigningConfig{
		KeyPath: keyPath,
	})
	if err != nil {
		t.Fatalf("NewSigner failed: %v", err)
	}

	_, err = signer.SignBlueprint(context.Background(), "/nonexistent/archive.tar.gz")
	if err == nil {
		t.Error("expected error for nonexistent archive")
	}
}

func TestSigner_GetPublicKey(t *testing.T) {
	keyTypes := []KeyType{KeyTypeECDSA, KeyTypeRSA, KeyTypeEd25519}

	for _, kt := range keyTypes {
		t.Run(string(kt), func(t *testing.T) {
			priv, _, err := GenerateKeyPair(kt, 2048)
			if err != nil {
				t.Fatalf("GenerateKeyPair failed: %v", err)
			}

			tmpDir := t.TempDir()
			keyPath := filepath.Join(tmpDir, "private.key")
			if err := os.WriteFile(keyPath, priv, 0600); err != nil {
				t.Fatalf("failed to write key file: %v", err)
			}

			signer, err := NewSigner(&SigningConfig{
				KeyPath: keyPath,
			})
			if err != nil {
				t.Fatalf("NewSigner failed: %v", err)
			}

			pubKey, err := signer.GetPublicKey()
			if err != nil {
				t.Fatalf("GetPublicKey failed: %v", err)
			}

			if !containsBytes(pubKey, []byte("-----BEGIN PUBLIC KEY-----")) {
				t.Error("public key is not in PEM format")
			}
		})
	}
}

func TestEncryptPrivateKey(t *testing.T) {
	priv, _, err := GenerateKeyPair(KeyTypeECDSA, 256)
	if err != nil {
		t.Fatalf("GenerateKeyPair failed: %v", err)
	}

	encrypted, err := EncryptPrivateKey(priv, "testpassword")
	if err != nil {
		t.Fatalf("EncryptPrivateKey failed: %v", err)
	}

	if !containsBytes(encrypted, []byte("ENCRYPTED")) {
		t.Error("encrypted key doesn't contain ENCRYPTED marker")
	}

	// Test decryption by creating a signer
	tmpDir := t.TempDir()
	keyPath := filepath.Join(tmpDir, "private.key")
	if err := os.WriteFile(keyPath, encrypted, 0600); err != nil {
		t.Fatalf("failed to write key file: %v", err)
	}

	// Without password should fail
	_, err = NewSigner(&SigningConfig{
		KeyPath: keyPath,
	})
	if err == nil {
		t.Error("expected error when decrypting without password")
	}

	// With password should succeed
	signer, err := NewSigner(&SigningConfig{
		KeyPath:     keyPath,
		KeyPassword: "testpassword",
	})
	if err != nil {
		t.Fatalf("NewSigner with password failed: %v", err)
	}
	if signer.KeyType() != KeyTypeECDSA {
		t.Errorf("key type = %s, want %s", signer.KeyType(), KeyTypeECDSA)
	}
}

func TestEncryptPrivateKey_InvalidPEM(t *testing.T) {
	_, err := EncryptPrivateKey([]byte("not a pem"), "password")
	if err == nil {
		t.Error("expected error for invalid PEM")
	}
}

func TestParseSignature(t *testing.T) {
	original := []byte("test signature data")
	encoded := "dGVzdCBzaWduYXR1cmUgZGF0YQ==" // base64 of "test signature data"

	decoded, err := ParseSignature(encoded)
	if err != nil {
		t.Fatalf("ParseSignature failed: %v", err)
	}

	if string(decoded) != string(original) {
		t.Errorf("decoded = %s, want %s", string(decoded), string(original))
	}
}

func TestParseSignatureBundle(t *testing.T) {
	bundleJSON := `{
		"payloadType": "application/vnd.kscore.blueprint.v1+json",
		"payload": "dGVzdA==",
		"signatures": [
			{
				"keyid": "key1",
				"sig": "c2lnbmF0dXJl"
			}
		]
	}`

	bundle, err := ParseSignatureBundle([]byte(bundleJSON))
	if err != nil {
		t.Fatalf("ParseSignatureBundle failed: %v", err)
	}

	if bundle.PayloadType != "application/vnd.kscore.blueprint.v1+json" {
		t.Errorf("payload type = %s, want application/vnd.kscore.blueprint.v1+json", bundle.PayloadType)
	}
	if len(bundle.Signatures) != 1 {
		t.Errorf("signatures count = %d, want 1", len(bundle.Signatures))
	}
	if bundle.Signatures[0].KeyID != "key1" {
		t.Errorf("keyid = %s, want key1", bundle.Signatures[0].KeyID)
	}
}

func TestParseSignatureBundle_Invalid(t *testing.T) {
	_, err := ParseSignatureBundle([]byte("not json"))
	if err == nil {
		t.Error("expected error for invalid JSON")
	}
}

func TestFormatFingerprint(t *testing.T) {
	_, pub, err := GenerateKeyPair(KeyTypeECDSA, 256)
	if err != nil {
		t.Fatalf("GenerateKeyPair failed: %v", err)
	}

	fingerprint := FormatFingerprint(pub)
	if fingerprint == "" {
		t.Error("fingerprint is empty")
	}
	if len(fingerprint) < 10 {
		t.Error("fingerprint is too short")
	}
	// Should start with sha256:
	if fingerprint[:7] != "sha256:" {
		t.Errorf("fingerprint = %s, should start with sha256:", fingerprint)
	}
}

func TestFormatFingerprint_InvalidPEM(t *testing.T) {
	fingerprint := FormatFingerprint([]byte("not a pem"))
	if fingerprint != "" {
		t.Error("expected empty fingerprint for invalid PEM")
	}
}

func TestSigner_WithCertificate(t *testing.T) {
	priv, pub, err := GenerateKeyPair(KeyTypeECDSA, 256)
	if err != nil {
		t.Fatalf("GenerateKeyPair failed: %v", err)
	}

	tmpDir := t.TempDir()
	keyPath := filepath.Join(tmpDir, "private.key")
	certPath := filepath.Join(tmpDir, "cert.pem")

	if err := os.WriteFile(keyPath, priv, 0600); err != nil {
		t.Fatalf("failed to write key file: %v", err)
	}
	// Use public key as a stand-in for certificate
	if err := os.WriteFile(certPath, pub, 0644); err != nil {
		t.Fatalf("failed to write cert file: %v", err)
	}

	signer, err := NewSigner(&SigningConfig{
		KeyPath:  keyPath,
		CertPath: certPath,
	})
	if err != nil {
		t.Fatalf("NewSigner failed: %v", err)
	}

	data := []byte("test data")
	result, err := signer.Sign(context.Background(), data)
	if err != nil {
		t.Fatalf("Sign failed: %v", err)
	}

	if result.Certificate == "" {
		t.Error("certificate is empty")
	}
}

func TestSignatureFormat_Constants(t *testing.T) {
	// Verify format constants
	if SignatureFormatCosign != "cosign" {
		t.Errorf("SignatureFormatCosign = %s, want cosign", SignatureFormatCosign)
	}
	if SignatureFormatDetached != "detached" {
		t.Errorf("SignatureFormatDetached = %s, want detached", SignatureFormatDetached)
	}
	if SignatureFormatBundle != "bundle" {
		t.Errorf("SignatureFormatBundle = %s, want bundle", SignatureFormatBundle)
	}
}

func TestKeyType_Constants(t *testing.T) {
	if KeyTypeECDSA != "ecdsa" {
		t.Errorf("KeyTypeECDSA = %s, want ecdsa", KeyTypeECDSA)
	}
	if KeyTypeRSA != "rsa" {
		t.Errorf("KeyTypeRSA = %s, want rsa", KeyTypeRSA)
	}
	if KeyTypeEd25519 != "ed25519" {
		t.Errorf("KeyTypeEd25519 = %s, want ed25519", KeyTypeEd25519)
	}
}

// containsBytes checks if data contains substr.
func containsBytes(data, substr []byte) bool {
	return len(data) >= len(substr) && string(data[:len(substr)]) == string(substr) ||
		len(data) > len(substr) && containsBytesHelper(data, substr)
}

func containsBytesHelper(data, substr []byte) bool {
	for i := 0; i <= len(data)-len(substr); i++ {
		if string(data[i:i+len(substr)]) == string(substr) {
			return true
		}
	}
	return false
}
