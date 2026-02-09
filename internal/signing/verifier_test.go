package signing

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestNewKeyVerifier(t *testing.T) {
	ecdsaKey, _ := GenerateKeyPair(KeyTypeECDSA, 256)
	rsaKey, _ := GenerateKeyPair(KeyTypeRSA, 2048)
	ed25519Key, _ := GenerateKeyPair(KeyTypeEd25519, 0)

	tests := []struct {
		name    string
		config  *KeyVerifierConfig
		wantErr bool
	}{
		{
			name: "ECDSA public key",
			config: &KeyVerifierConfig{
				PublicKeyPEM: ecdsaKey.PublicKey,
			},
		},
		{
			name: "RSA public key",
			config: &KeyVerifierConfig{
				PublicKeyPEM: rsaKey.PublicKey,
			},
		},
		{
			name: "Ed25519 public key",
			config: &KeyVerifierConfig{
				PublicKeyPEM: ed25519Key.PublicKey,
			},
		},
		{
			name: "with custom hash",
			config: &KeyVerifierConfig{
				PublicKeyPEM:  ecdsaKey.PublicKey,
				HashAlgorithm: HashSHA512,
			},
		},
		{
			name:    "nil config",
			config:  nil,
			wantErr: true,
		},
		{
			name:    "missing key",
			config:  &KeyVerifierConfig{},
			wantErr: true,
		},
		{
			name: "invalid key",
			config: &KeyVerifierConfig{
				PublicKeyPEM: []byte("not a key"),
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			verifier, err := NewKeyVerifier(tt.config)
			if (err != nil) != tt.wantErr {
				t.Errorf("NewKeyVerifier() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if tt.wantErr {
				return
			}
			if verifier == nil {
				t.Error("expected non-nil verifier")
			}
		})
	}
}

func TestNewKeyVerifierFromFile(t *testing.T) {
	tmpDir := t.TempDir()

	keyPair, _ := GenerateKeyPair(KeyTypeECDSA, 256)
	keyPath := filepath.Join(tmpDir, "test.pub")
	if err := os.WriteFile(keyPath, keyPair.PublicKey, 0644); err != nil {
		t.Fatal(err)
	}

	t.Run("load from file", func(t *testing.T) {
		verifier, err := NewKeyVerifier(&KeyVerifierConfig{
			PublicKeyPath: keyPath,
		})
		if err != nil {
			t.Errorf("NewKeyVerifier() error = %v", err)
			return
		}
		if verifier == nil {
			t.Error("expected non-nil verifier")
		}
	})

	t.Run("file not found", func(t *testing.T) {
		_, err := NewKeyVerifier(&KeyVerifierConfig{
			PublicKeyPath: filepath.Join(tmpDir, "nonexistent.pub"),
		})
		if err == nil {
			t.Error("expected error for nonexistent file")
		}
	})
}

func TestKeyVerifierVerify(t *testing.T) {
	ctx := context.Background()
	testData := []byte("test data for verification")

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

			signer, _ := NewKeySigner(&KeySignerConfig{
				PrivateKeyPEM: keyPair.PrivateKey,
				HashAlgorithm: tt.hash,
			})

			result, _ := signer.Sign(ctx, testData)

			verifier, err := NewKeyVerifier(&KeyVerifierConfig{
				PublicKeyPEM:  keyPair.PublicKey,
				HashAlgorithm: tt.hash,
			})
			if err != nil {
				t.Fatalf("failed to create verifier: %v", err)
			}

			valid, err := verifier.Verify(ctx, testData, result.Signature)
			if err != nil {
				t.Errorf("Verify() error = %v", err)
				return
			}
			if !valid {
				t.Error("signature should be valid")
			}
		})
	}
}

func TestKeyVerifierVerifyInvalidSignature(t *testing.T) {
	ctx := context.Background()
	keyPair, _ := GenerateKeyPair(KeyTypeECDSA, 256)

	verifier, _ := NewKeyVerifier(&KeyVerifierConfig{
		PublicKeyPEM: keyPair.PublicKey,
	})

	t.Run("wrong data", func(t *testing.T) {
		signer, _ := NewKeySigner(&KeySignerConfig{
			PrivateKeyPEM: keyPair.PrivateKey,
		})
		result, _ := signer.Sign(ctx, []byte("original data"))

		valid, err := verifier.Verify(ctx, []byte("different data"), result.Signature)
		if err != nil {
			t.Errorf("Verify() error = %v", err)
			return
		}
		if valid {
			t.Error("signature should be invalid for different data")
		}
	})

	t.Run("wrong key", func(t *testing.T) {
		differentKey, _ := GenerateKeyPair(KeyTypeECDSA, 256)
		signer, _ := NewKeySigner(&KeySignerConfig{
			PrivateKeyPEM: differentKey.PrivateKey,
		})
		result, _ := signer.Sign(ctx, []byte("test data"))

		valid, err := verifier.Verify(ctx, []byte("test data"), result.Signature)
		if err != nil {
			t.Errorf("Verify() error = %v", err)
			return
		}
		if valid {
			t.Error("signature should be invalid for different key")
		}
	})

	t.Run("corrupted signature", func(t *testing.T) {
		signer, _ := NewKeySigner(&KeySignerConfig{
			PrivateKeyPEM: keyPair.PrivateKey,
		})
		result, _ := signer.Sign(ctx, []byte("test data"))

		// Corrupt the signature
		if len(result.Signature) > 0 {
			result.Signature[0] ^= 0xFF
		}

		valid, err := verifier.Verify(ctx, []byte("test data"), result.Signature)
		if err != nil {
			t.Errorf("Verify() error = %v", err)
			return
		}
		if valid {
			t.Error("signature should be invalid when corrupted")
		}
	})
}

func TestKeyVerifierVerifyFile(t *testing.T) {
	ctx := context.Background()
	tmpDir := t.TempDir()

	keyPair, _ := GenerateKeyPair(KeyTypeECDSA, 256)
	testData := []byte("file content to verify")

	// Create test files
	dataPath := filepath.Join(tmpDir, "data.txt")
	sigPath := filepath.Join(tmpDir, "data.sig")
	if err := os.WriteFile(dataPath, testData, 0644); err != nil {
		t.Fatal(err)
	}

	signer, _ := NewKeySigner(&KeySignerConfig{
		PrivateKeyPEM: keyPair.PrivateKey,
	})

	result, _ := signer.Sign(ctx, testData)
	if err := os.WriteFile(sigPath, []byte(result.SignatureBase64), 0644); err != nil {
		t.Fatal(err)
	}

	verifier, _ := NewKeyVerifier(&KeyVerifierConfig{
		PublicKeyPEM: keyPair.PublicKey,
	})

	t.Run("verify file", func(t *testing.T) {
		valid, err := verifier.VerifyFile(ctx, dataPath, sigPath)
		if err != nil {
			t.Errorf("VerifyFile() error = %v", err)
			return
		}
		if !valid {
			t.Error("signature should be valid")
		}
	})

	t.Run("data file not found", func(t *testing.T) {
		_, err := verifier.VerifyFile(ctx, filepath.Join(tmpDir, "nonexistent.txt"), sigPath)
		if err == nil {
			t.Error("expected error for nonexistent data file")
		}
	})

	t.Run("signature file not found", func(t *testing.T) {
		_, err := verifier.VerifyFile(ctx, dataPath, filepath.Join(tmpDir, "nonexistent.sig"))
		if err == nil {
			t.Error("expected error for nonexistent signature file")
		}
	})
}

func TestKeyVerifierGetSignerIdentity(t *testing.T) {
	keyPair, _ := GenerateKeyPair(KeyTypeECDSA, 256)

	verifier, _ := NewKeyVerifier(&KeyVerifierConfig{
		PublicKeyPEM: keyPair.PublicKey,
	})

	identity, err := verifier.GetSignerIdentity(nil)
	if err != nil {
		t.Errorf("GetSignerIdentity() error = %v", err)
		return
	}

	if identity == "" {
		t.Error("expected non-empty identity")
	}

	// Should match the key fingerprint
	expectedFP := KeyFingerprint(keyPair.PublicKey)
	if identity != expectedFP {
		t.Errorf("identity = %v, want %v", identity, expectedFP)
	}
}

func TestBundleVerifier(t *testing.T) {
	ctx := context.Background()
	testData := []byte("test data for bundle verification")

	keyPair, _ := GenerateKeyPair(KeyTypeECDSA, 256)

	// Create a signed bundle
	signer, _ := NewKeySigner(&KeySignerConfig{
		PrivateKeyPEM: keyPair.PrivateKey,
		Format:        FormatBundle,
	})

	result, _ := signer.Sign(ctx, testData)

	// For bundle verification, we need to include the certificate
	// Since we're doing key-based signing, we'll create a bundle with the public key
	// The keyless verifier handles certificates; for key-based, we use KeyVerifier directly

	t.Run("NewBundleVerifier", func(t *testing.T) {
		verifier := NewBundleVerifier()
		if verifier == nil {
			t.Fatal("expected non-nil verifier")
		}
		if verifier.HashAlgorithm != HashSHA256 {
			t.Errorf("default hash = %v, want %v", verifier.HashAlgorithm, HashSHA256)
		}
	})

	t.Run("bundle without certificate errors", func(t *testing.T) {
		verifier := NewBundleVerifier()

		// Bundle from key-based signer doesn't have a certificate
		_, err := verifier.VerifyBundle(ctx, testData, result.Bundle)
		if err == nil {
			t.Error("expected error for bundle without certificate")
		}
	})

	t.Run("empty bundle", func(t *testing.T) {
		verifier := NewBundleVerifier()

		_, err := verifier.VerifyBundle(ctx, testData, &SignatureBundle{})
		if err == nil {
			t.Error("expected error for empty bundle")
		}
	})
}

func TestDecodeSignature(t *testing.T) {
	rawSig := []byte{0x01, 0x02, 0x03, 0x04}
	base64Sig := base64.StdEncoding.EncodeToString(rawSig)

	tests := []struct {
		name    string
		input   []byte
		wantLen int
		wantErr bool
	}{
		{
			name:    "raw bytes",
			input:   rawSig,
			wantLen: 4,
		},
		{
			name:    "base64 standard",
			input:   []byte(base64Sig),
			wantLen: 4,
		},
		{
			name:    "base64 URL encoding",
			input:   []byte(base64.URLEncoding.EncodeToString(rawSig)),
			wantLen: 4,
		},
		{
			name:    "base64 with whitespace",
			input:   []byte("  " + base64Sig + "  \n"),
			wantLen: 4,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sig, err := decodeSignature(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("decodeSignature() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if tt.wantErr {
				return
			}
			if len(sig) != tt.wantLen {
				t.Errorf("decoded length = %v, want %v", len(sig), tt.wantLen)
			}
		})
	}
}

func TestDecodeSignatureBundle(t *testing.T) {
	rawSig := []byte{0x01, 0x02, 0x03, 0x04}
	bundle := SignatureBundle{
		Signatures: []BundleSignature{
			{Sig: base64.StdEncoding.EncodeToString(rawSig)},
		},
	}

	bundleJSON, _ := json.Marshal(bundle)

	t.Run("valid bundle", func(t *testing.T) {
		sig, err := decodeSignatureBundle(bundleJSON)
		if err != nil {
			t.Errorf("decodeSignatureBundle() error = %v", err)
			return
		}
		if len(sig) != 4 {
			t.Errorf("decoded length = %v, want %v", len(sig), 4)
		}
	})

	t.Run("invalid JSON", func(t *testing.T) {
		_, err := decodeSignatureBundle([]byte("not json"))
		if err == nil {
			t.Error("expected error for invalid JSON")
		}
	})

	t.Run("empty signatures", func(t *testing.T) {
		emptyBundle, _ := json.Marshal(SignatureBundle{})
		_, err := decodeSignatureBundle(emptyBundle)
		if err == nil {
			t.Error("expected error for empty signatures")
		}
	})
}

func TestCrossKeyVerification(t *testing.T) {
	ctx := context.Background()
	testData := []byte("cross-key test data")

	// Sign with one key type, verify fails with different key
	ecdsaKey, _ := GenerateKeyPair(KeyTypeECDSA, 256)
	rsaKey, _ := GenerateKeyPair(KeyTypeRSA, 2048)

	signer, _ := NewKeySigner(&KeySignerConfig{
		PrivateKeyPEM: ecdsaKey.PrivateKey,
	})

	result, _ := signer.Sign(ctx, testData)

	// Try to verify with RSA key - should fail
	verifier, _ := NewKeyVerifier(&KeyVerifierConfig{
		PublicKeyPEM: rsaKey.PublicKey,
	})

	valid, err := verifier.Verify(ctx, testData, result.Signature)
	if err != nil {
		t.Errorf("Verify() error = %v", err)
		return
	}
	if valid {
		t.Error("verification should fail with wrong key type")
	}
}

func TestHashMismatchVerification(t *testing.T) {
	ctx := context.Background()
	testData := []byte("hash mismatch test data")

	keyPair, _ := GenerateKeyPair(KeyTypeECDSA, 256)

	// Sign with SHA256
	signer, _ := NewKeySigner(&KeySignerConfig{
		PrivateKeyPEM: keyPair.PrivateKey,
		HashAlgorithm: HashSHA256,
	})

	result, _ := signer.Sign(ctx, testData)

	// Verify with SHA512 - should fail
	verifier, _ := NewKeyVerifier(&KeyVerifierConfig{
		PublicKeyPEM:  keyPair.PublicKey,
		HashAlgorithm: HashSHA512,
	})

	valid, err := verifier.Verify(ctx, testData, result.Signature)
	if err != nil {
		t.Errorf("Verify() error = %v", err)
		return
	}
	if valid {
		t.Error("verification should fail with mismatched hash algorithm")
	}
}
