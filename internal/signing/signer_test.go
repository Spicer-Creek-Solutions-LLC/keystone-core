package signing

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestNewKeySigner(t *testing.T) {
	ecdsaKey, _ := GenerateKeyPair(KeyTypeECDSA, 256)
	rsaKey, _ := GenerateKeyPair(KeyTypeRSA, 2048)
	ed25519Key, _ := GenerateKeyPair(KeyTypeEd25519, 0)

	tests := []struct {
		name    string
		config  *KeySignerConfig
		wantErr bool
	}{
		{
			name: "ECDSA key from PEM",
			config: &KeySignerConfig{
				PrivateKeyPEM: ecdsaKey.PrivateKey,
			},
		},
		{
			name: "RSA key from PEM",
			config: &KeySignerConfig{
				PrivateKeyPEM: rsaKey.PrivateKey,
			},
		},
		{
			name: "Ed25519 key from PEM",
			config: &KeySignerConfig{
				PrivateKeyPEM: ed25519Key.PrivateKey,
			},
		},
		{
			name: "with custom hash algorithm",
			config: &KeySignerConfig{
				PrivateKeyPEM: ecdsaKey.PrivateKey,
				HashAlgorithm: HashSHA512,
			},
		},
		{
			name: "with bundle format",
			config: &KeySignerConfig{
				PrivateKeyPEM: ecdsaKey.PrivateKey,
				Format:        FormatBundle,
			},
		},
		{
			name: "with annotations",
			config: &KeySignerConfig{
				PrivateKeyPEM: ecdsaKey.PrivateKey,
				Annotations: map[string]string{
					"author": "test",
					"ref":    "main",
				},
			},
		},
		{
			name:    "nil config",
			config:  nil,
			wantErr: true,
		},
		{
			name:    "missing key",
			config:  &KeySignerConfig{},
			wantErr: true,
		},
		{
			name: "invalid key",
			config: &KeySignerConfig{
				PrivateKeyPEM: []byte("not a key"),
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			signer, err := NewKeySigner(tt.config)
			if (err != nil) != tt.wantErr {
				t.Errorf("NewKeySigner() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if tt.wantErr {
				return
			}
			if signer == nil {
				t.Error("expected non-nil signer")
			}
		})
	}
}

func TestNewKeySignerFromFile(t *testing.T) {
	tmpDir := t.TempDir()

	// Create a test key file
	keyPair, _ := GenerateKeyPair(KeyTypeECDSA, 256)
	keyPath := filepath.Join(tmpDir, "test.key")
	if err := os.WriteFile(keyPath, keyPair.PrivateKey, 0600); err != nil {
		t.Fatal(err)
	}

	t.Run("load from file", func(t *testing.T) {
		signer, err := NewKeySigner(&KeySignerConfig{
			PrivateKeyPath: keyPath,
		})
		if err != nil {
			t.Errorf("NewKeySigner() error = %v", err)
			return
		}
		if signer == nil {
			t.Error("expected non-nil signer")
		}
	})

	t.Run("file not found", func(t *testing.T) {
		_, err := NewKeySigner(&KeySignerConfig{
			PrivateKeyPath: filepath.Join(tmpDir, "nonexistent.key"),
		})
		if err == nil {
			t.Error("expected error for nonexistent file")
		}
	})
}

func TestKeySignerSign(t *testing.T) {
	ctx := context.Background()
	testData := []byte("test data to sign")

	tests := []struct {
		name    string
		keyType KeyType
		hash    HashAlgorithm
	}{
		{"ECDSA SHA256", KeyTypeECDSA, HashSHA256},
		{"ECDSA SHA384", KeyTypeECDSA, HashSHA384},
		{"ECDSA SHA512", KeyTypeECDSA, HashSHA512},
		{"RSA SHA256", KeyTypeRSA, HashSHA256},
		{"RSA SHA384", KeyTypeRSA, HashSHA384},
		{"RSA SHA512", KeyTypeRSA, HashSHA512},
		{"Ed25519 SHA256", KeyTypeEd25519, HashSHA256},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			keyPair, _ := GenerateKeyPair(tt.keyType, 256)
			signer, err := NewKeySigner(&KeySignerConfig{
				PrivateKeyPEM: keyPair.PrivateKey,
				HashAlgorithm: tt.hash,
			})
			if err != nil {
				t.Fatalf("failed to create signer: %v", err)
			}

			result, err := signer.Sign(ctx, testData)
			if err != nil {
				t.Errorf("Sign() error = %v", err)
				return
			}

			if result == nil {
				t.Fatal("expected non-nil result")
			}

			// Verify result fields
			if len(result.Signature) == 0 {
				t.Error("expected non-empty signature")
			}
			if result.SignatureBase64 == "" {
				t.Error("expected non-empty base64 signature")
			}
			if result.Digest == "" {
				t.Error("expected non-empty digest")
			}
			if result.DigestAlgorithm != tt.hash {
				t.Errorf("digest algorithm = %v, want %v", result.DigestAlgorithm, tt.hash)
			}
			if result.Timestamp.IsZero() {
				t.Error("expected non-zero timestamp")
			}
			if result.SignerIdentity == "" {
				t.Error("expected non-empty signer identity")
			}
			if !strings.HasPrefix(result.SignerIdentity, "sha256:") {
				t.Error("signer identity should be a key fingerprint")
			}
		})
	}
}

func TestKeySignerSignWithBundle(t *testing.T) {
	ctx := context.Background()
	keyPair, _ := GenerateKeyPair(KeyTypeECDSA, 256)

	signer, _ := NewKeySigner(&KeySignerConfig{
		PrivateKeyPEM: keyPair.PrivateKey,
		Format:        FormatBundle,
		Annotations: map[string]string{
			"version": "1.0.0",
		},
	})

	result, err := signer.Sign(ctx, []byte("test data"))
	if err != nil {
		t.Fatalf("Sign() error = %v", err)
	}

	if result.Bundle == nil {
		t.Fatal("expected bundle to be created")
	}

	if result.Bundle.MediaType == "" {
		t.Error("expected non-empty media type")
	}
	if result.Bundle.PayloadType == "" {
		t.Error("expected non-empty payload type")
	}
	if len(result.Bundle.Signatures) == 0 {
		t.Error("expected at least one signature in bundle")
	}
	if result.Bundle.Signatures[0].Sig == "" {
		t.Error("expected non-empty signature in bundle")
	}
	if result.Bundle.Signatures[0].KeyID == "" {
		t.Error("expected non-empty key ID in bundle")
	}
}

func TestKeySignerSignFile(t *testing.T) {
	ctx := context.Background()
	tmpDir := t.TempDir()

	// Create test file
	testFile := filepath.Join(tmpDir, "test.txt")
	testData := []byte("file content to sign")
	if err := os.WriteFile(testFile, testData, 0644); err != nil {
		t.Fatal(err)
	}

	keyPair, _ := GenerateKeyPair(KeyTypeECDSA, 256)
	signer, _ := NewKeySigner(&KeySignerConfig{
		PrivateKeyPEM: keyPair.PrivateKey,
	})

	t.Run("sign file", func(t *testing.T) {
		result, err := signer.SignFile(ctx, testFile)
		if err != nil {
			t.Errorf("SignFile() error = %v", err)
			return
		}
		if result == nil {
			t.Fatal("expected non-nil result")
		}
		if len(result.Signature) == 0 {
			t.Error("expected non-empty signature")
		}
	})

	t.Run("file not found", func(t *testing.T) {
		_, err := signer.SignFile(ctx, filepath.Join(tmpDir, "nonexistent.txt"))
		if err == nil {
			t.Error("expected error for nonexistent file")
		}
	})
}

func TestKeySignerKeyType(t *testing.T) {
	tests := []struct {
		keyType KeyType
	}{
		{KeyTypeECDSA},
		{KeyTypeRSA},
		{KeyTypeEd25519},
	}

	for _, tt := range tests {
		t.Run(string(tt.keyType), func(t *testing.T) {
			keyPair, _ := GenerateKeyPair(tt.keyType, 256)
			signer, _ := NewKeySigner(&KeySignerConfig{
				PrivateKeyPEM: keyPair.PrivateKey,
			})

			if signer.KeyType() != tt.keyType {
				t.Errorf("KeyType() = %v, want %v", signer.KeyType(), tt.keyType)
			}
		})
	}
}

func TestKeySignerPublicKey(t *testing.T) {
	keyPair, _ := GenerateKeyPair(KeyTypeECDSA, 256)
	signer, _ := NewKeySigner(&KeySignerConfig{
		PrivateKeyPEM: keyPair.PrivateKey,
	})

	pubKey, err := signer.PublicKey()
	if err != nil {
		t.Errorf("PublicKey() error = %v", err)
		return
	}

	if len(pubKey) == 0 {
		t.Error("expected non-empty public key")
	}

	// Should be valid PEM
	_, _, err = LoadPublicKey(pubKey)
	if err != nil {
		t.Errorf("returned public key is invalid: %v", err)
	}
}

func TestSignatureWriter(t *testing.T) {
	ctx := context.Background()
	tmpDir := t.TempDir()

	keyPair, _ := GenerateKeyPair(KeyTypeECDSA, 256)
	signer, _ := NewKeySigner(&KeySignerConfig{
		PrivateKeyPEM: keyPair.PrivateKey,
	})

	writer := NewSignatureWriter(signer)

	t.Run("SignToFile", func(t *testing.T) {
		sigPath := filepath.Join(tmpDir, "data.sig")

		err := writer.SignToFile(ctx, []byte("test data"), sigPath)
		if err != nil {
			t.Errorf("SignToFile() error = %v", err)
			return
		}

		// Verify signature file was created
		sigData, err := os.ReadFile(sigPath)
		if err != nil {
			t.Errorf("failed to read signature file: %v", err)
			return
		}
		if len(sigData) == 0 {
			t.Error("expected non-empty signature file")
		}
	})

	t.Run("SignFileToFile", func(t *testing.T) {
		dataPath := filepath.Join(tmpDir, "data.txt")
		sigPath := filepath.Join(tmpDir, "data2.sig")

		if err := os.WriteFile(dataPath, []byte("test file data"), 0644); err != nil {
			t.Fatal(err)
		}

		err := writer.SignFileToFile(ctx, dataPath, sigPath)
		if err != nil {
			t.Errorf("SignFileToFile() error = %v", err)
			return
		}

		// Verify signature file was created
		sigData, err := os.ReadFile(sigPath)
		if err != nil {
			t.Errorf("failed to read signature file: %v", err)
			return
		}
		if len(sigData) == 0 {
			t.Error("expected non-empty signature file")
		}
	})

	t.Run("SignFileToBundleFile", func(t *testing.T) {
		dataPath := filepath.Join(tmpDir, "data3.txt")
		bundlePath := filepath.Join(tmpDir, "data.bundle.json")

		if err := os.WriteFile(dataPath, []byte("test file data for bundle"), 0644); err != nil {
			t.Fatal(err)
		}

		err := writer.SignFileToBundleFile(ctx, dataPath, bundlePath)
		if err != nil {
			t.Errorf("SignFileToBundleFile() error = %v", err)
			return
		}

		// Verify bundle file was created
		bundleData, err := os.ReadFile(bundlePath)
		if err != nil {
			t.Errorf("failed to read bundle file: %v", err)
			return
		}
		if len(bundleData) == 0 {
			t.Error("expected non-empty bundle file")
		}

		// Should be valid JSON
		if !strings.HasPrefix(string(bundleData), "{") {
			t.Error("bundle should be JSON")
		}
	})
}

func TestSignVerifyRoundTrip(t *testing.T) {
	ctx := context.Background()
	testData := []byte("test data for round trip verification")

	tests := []struct {
		name    string
		keyType KeyType
		hash    HashAlgorithm
	}{
		{"ECDSA SHA256", KeyTypeECDSA, HashSHA256},
		{"ECDSA SHA384", KeyTypeECDSA, HashSHA384},
		{"ECDSA SHA512", KeyTypeECDSA, HashSHA512},
		{"RSA SHA256", KeyTypeRSA, HashSHA256},
		{"RSA SHA384", KeyTypeRSA, HashSHA384},
		{"RSA SHA512", KeyTypeRSA, HashSHA512},
		{"Ed25519", KeyTypeEd25519, HashSHA256},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			keyPair, _ := GenerateKeyPair(tt.keyType, 256)

			// Create signer
			signer, err := NewKeySigner(&KeySignerConfig{
				PrivateKeyPEM: keyPair.PrivateKey,
				HashAlgorithm: tt.hash,
			})
			if err != nil {
				t.Fatalf("failed to create signer: %v", err)
			}

			// Sign data
			result, err := signer.Sign(ctx, testData)
			if err != nil {
				t.Fatalf("Sign() error = %v", err)
			}

			// Create verifier
			verifier, err := NewKeyVerifier(&KeyVerifierConfig{
				PublicKeyPEM:  keyPair.PublicKey,
				HashAlgorithm: tt.hash,
			})
			if err != nil {
				t.Fatalf("failed to create verifier: %v", err)
			}

			// Verify signature
			valid, err := verifier.Verify(ctx, testData, result.Signature)
			if err != nil {
				t.Fatalf("Verify() error = %v", err)
			}
			if !valid {
				t.Error("signature should be valid")
			}

			// Verify with wrong data should fail
			valid, err = verifier.Verify(ctx, []byte("wrong data"), result.Signature)
			if err != nil {
				t.Fatalf("Verify() error = %v", err)
			}
			if valid {
				t.Error("signature should be invalid for wrong data")
			}
		})
	}
}
