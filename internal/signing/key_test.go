package signing

import (
	"crypto/ecdsa"
	"crypto/ed25519"
	"crypto/rsa"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestGenerateKeyPair(t *testing.T) {
	tests := []struct {
		name    string
		keyType KeyType
		bits    int
		wantErr bool
	}{
		{
			name:    "ECDSA P-256",
			keyType: KeyTypeECDSA,
			bits:    256,
		},
		{
			name:    "ECDSA P-384",
			keyType: KeyTypeECDSA,
			bits:    384,
		},
		{
			name:    "ECDSA P-521",
			keyType: KeyTypeECDSA,
			bits:    521,
		},
		{
			name:    "RSA 2048",
			keyType: KeyTypeRSA,
			bits:    2048,
		},
		{
			name:    "RSA 4096",
			keyType: KeyTypeRSA,
			bits:    4096,
		},
		{
			name:    "RSA minimum enforced",
			keyType: KeyTypeRSA,
			bits:    1024, // Should be upgraded to 2048
		},
		{
			name:    "Ed25519",
			keyType: KeyTypeEd25519,
			bits:    0, // Ignored for Ed25519
		},
		{
			name:    "unsupported key type",
			keyType: "invalid",
			bits:    256,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			keyPair, err := GenerateKeyPair(tt.keyType, tt.bits)
			if (err != nil) != tt.wantErr {
				t.Errorf("GenerateKeyPair() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if tt.wantErr {
				return
			}

			if keyPair == nil {
				t.Fatal("expected non-nil key pair")
			}
			if keyPair.Type != tt.keyType {
				t.Errorf("key type = %v, want %v", keyPair.Type, tt.keyType)
			}
			if len(keyPair.PrivateKey) == 0 {
				t.Error("expected non-empty private key")
			}
			if len(keyPair.PublicKey) == 0 {
				t.Error("expected non-empty public key")
			}

			// Verify we can load the keys back
			priv, kt, err := LoadPrivateKey(keyPair.PrivateKey, "")
			if err != nil {
				t.Errorf("failed to load private key: %v", err)
			}
			if kt != tt.keyType {
				t.Errorf("loaded key type = %v, want %v", kt, tt.keyType)
			}
			if priv == nil {
				t.Error("expected non-nil private key")
			}

			pub, kt, err := LoadPublicKey(keyPair.PublicKey)
			if err != nil {
				t.Errorf("failed to load public key: %v", err)
			}
			if kt != tt.keyType {
				t.Errorf("loaded key type = %v, want %v", kt, tt.keyType)
			}
			if pub == nil {
				t.Error("expected non-nil public key")
			}
		})
	}
}

func TestLoadPrivateKey(t *testing.T) {
	// Generate test keys
	ecdsaKey, _ := GenerateKeyPair(KeyTypeECDSA, 256)
	rsaKey, _ := GenerateKeyPair(KeyTypeRSA, 2048)
	ed25519Key, _ := GenerateKeyPair(KeyTypeEd25519, 0)

	tests := []struct {
		name        string
		pemData     []byte
		password    string
		wantKeyType KeyType
		wantErr     bool
	}{
		{
			name:        "ECDSA key",
			pemData:     ecdsaKey.PrivateKey,
			wantKeyType: KeyTypeECDSA,
		},
		{
			name:        "RSA key",
			pemData:     rsaKey.PrivateKey,
			wantKeyType: KeyTypeRSA,
		},
		{
			name:        "Ed25519 key",
			pemData:     ed25519Key.PrivateKey,
			wantKeyType: KeyTypeEd25519,
		},
		{
			name:    "invalid PEM",
			pemData: []byte("not a pem"),
			wantErr: true,
		},
		{
			name:    "empty PEM",
			pemData: []byte{},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			priv, kt, err := LoadPrivateKey(tt.pemData, tt.password)
			if (err != nil) != tt.wantErr {
				t.Errorf("LoadPrivateKey() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if tt.wantErr {
				return
			}
			if priv == nil {
				t.Error("expected non-nil private key")
			}
			if kt != tt.wantKeyType {
				t.Errorf("key type = %v, want %v", kt, tt.wantKeyType)
			}
		})
	}
}

func TestLoadPrivateKeyFromFile(t *testing.T) {
	tmpDir := t.TempDir()

	// Generate and save a test key
	keyPair, _ := GenerateKeyPair(KeyTypeECDSA, 256)
	keyPath := filepath.Join(tmpDir, "test.key")
	if err := os.WriteFile(keyPath, keyPair.PrivateKey, 0600); err != nil {
		t.Fatal(err)
	}

	t.Run("load from file", func(t *testing.T) {
		priv, kt, err := LoadPrivateKeyFromFile(keyPath, "")
		if err != nil {
			t.Errorf("LoadPrivateKeyFromFile() error = %v", err)
			return
		}
		if priv == nil {
			t.Error("expected non-nil private key")
		}
		if kt != KeyTypeECDSA {
			t.Errorf("key type = %v, want %v", kt, KeyTypeECDSA)
		}
	})

	t.Run("file not found", func(t *testing.T) {
		_, _, err := LoadPrivateKeyFromFile(filepath.Join(tmpDir, "nonexistent.key"), "")
		if err == nil {
			t.Error("expected error for nonexistent file")
		}
	})
}

func TestLoadPublicKey(t *testing.T) {
	// Generate test keys
	ecdsaKey, _ := GenerateKeyPair(KeyTypeECDSA, 256)
	rsaKey, _ := GenerateKeyPair(KeyTypeRSA, 2048)
	ed25519Key, _ := GenerateKeyPair(KeyTypeEd25519, 0)

	tests := []struct {
		name        string
		pemData     []byte
		wantKeyType KeyType
		wantErr     bool
	}{
		{
			name:        "ECDSA public key",
			pemData:     ecdsaKey.PublicKey,
			wantKeyType: KeyTypeECDSA,
		},
		{
			name:        "RSA public key",
			pemData:     rsaKey.PublicKey,
			wantKeyType: KeyTypeRSA,
		},
		{
			name:        "Ed25519 public key",
			pemData:     ed25519Key.PublicKey,
			wantKeyType: KeyTypeEd25519,
		},
		{
			name:    "invalid PEM",
			pemData: []byte("not a pem"),
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pub, kt, err := LoadPublicKey(tt.pemData)
			if (err != nil) != tt.wantErr {
				t.Errorf("LoadPublicKey() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if tt.wantErr {
				return
			}
			if pub == nil {
				t.Error("expected non-nil public key")
			}
			if kt != tt.wantKeyType {
				t.Errorf("key type = %v, want %v", kt, tt.wantKeyType)
			}
		})
	}
}

func TestLoadPublicKeyFromFile(t *testing.T) {
	tmpDir := t.TempDir()

	// Generate and save a test key
	keyPair, _ := GenerateKeyPair(KeyTypeECDSA, 256)
	keyPath := filepath.Join(tmpDir, "test.pub")
	if err := os.WriteFile(keyPath, keyPair.PublicKey, 0644); err != nil {
		t.Fatal(err)
	}

	t.Run("load from file", func(t *testing.T) {
		pub, kt, err := LoadPublicKeyFromFile(keyPath)
		if err != nil {
			t.Errorf("LoadPublicKeyFromFile() error = %v", err)
			return
		}
		if pub == nil {
			t.Error("expected non-nil public key")
		}
		if kt != KeyTypeECDSA {
			t.Errorf("key type = %v, want %v", kt, KeyTypeECDSA)
		}
	})

	t.Run("file not found", func(t *testing.T) {
		_, _, err := LoadPublicKeyFromFile(filepath.Join(tmpDir, "nonexistent.pub"))
		if err == nil {
			t.Error("expected error for nonexistent file")
		}
	})
}

func TestEncodePublicKey(t *testing.T) {
	tests := []struct {
		name    string
		keyType KeyType
	}{
		{"ECDSA", KeyTypeECDSA},
		{"RSA", KeyTypeRSA},
		{"Ed25519", KeyTypeEd25519},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			keyPair, _ := GenerateKeyPair(tt.keyType, 256)

			// Load private key to get the public key
			priv, _, _ := LoadPrivateKey(keyPair.PrivateKey, "")
			pub, _ := GetPublicKeyFromPrivate(priv)

			// Encode the public key
			encoded, err := EncodePublicKey(pub)
			if err != nil {
				t.Errorf("EncodePublicKey() error = %v", err)
				return
			}

			// Verify the encoded key can be loaded
			loadedPub, kt, err := LoadPublicKey(encoded)
			if err != nil {
				t.Errorf("failed to load encoded key: %v", err)
				return
			}
			if kt != tt.keyType {
				t.Errorf("key type = %v, want %v", kt, tt.keyType)
			}
			if loadedPub == nil {
				t.Error("expected non-nil public key")
			}
		})
	}
}

func TestEncryptPrivateKey(t *testing.T) {
	keyPair, _ := GenerateKeyPair(KeyTypeECDSA, 256)

	t.Run("encrypt and decrypt", func(t *testing.T) {
		password := "test-password-123"

		encrypted, err := EncryptPrivateKey(keyPair.PrivateKey, password)
		if err != nil {
			t.Errorf("EncryptPrivateKey() error = %v", err)
			return
		}

		if len(encrypted) == 0 {
			t.Error("expected non-empty encrypted key")
		}

		// Should be different from original
		if string(encrypted) == string(keyPair.PrivateKey) {
			t.Error("encrypted key should differ from original")
		}

		// Should be able to load with correct password
		priv, kt, err := LoadPrivateKey(encrypted, password)
		if err != nil {
			t.Errorf("failed to load encrypted key: %v", err)
			return
		}
		if priv == nil {
			t.Error("expected non-nil private key")
		}
		if kt != KeyTypeECDSA {
			t.Errorf("key type = %v, want %v", kt, KeyTypeECDSA)
		}
	})

	t.Run("wrong password fails", func(t *testing.T) {
		encrypted, _ := EncryptPrivateKey(keyPair.PrivateKey, "correct-password")

		_, _, err := LoadPrivateKey(encrypted, "wrong-password")
		if err == nil {
			t.Error("expected error with wrong password")
		}
	})

	t.Run("no password fails", func(t *testing.T) {
		encrypted, _ := EncryptPrivateKey(keyPair.PrivateKey, "some-password")

		_, _, err := LoadPrivateKey(encrypted, "")
		if err == nil {
			t.Error("expected error when password not provided")
		}
	})
}

func TestKeyFingerprint(t *testing.T) {
	keyPair, _ := GenerateKeyPair(KeyTypeECDSA, 256)

	fingerprint := KeyFingerprint(keyPair.PublicKey)

	if fingerprint == "" {
		t.Error("expected non-empty fingerprint")
	}

	if !strings.HasPrefix(fingerprint, "sha256:") {
		t.Errorf("fingerprint should start with sha256:, got %s", fingerprint)
	}

	// Should be consistent
	fingerprint2 := KeyFingerprint(keyPair.PublicKey)
	if fingerprint != fingerprint2 {
		t.Error("fingerprint should be consistent")
	}

	// Different keys should have different fingerprints
	keyPair2, _ := GenerateKeyPair(KeyTypeECDSA, 256)
	fingerprint3 := KeyFingerprint(keyPair2.PublicKey)
	if fingerprint == fingerprint3 {
		t.Error("different keys should have different fingerprints")
	}
}

func TestGetPublicKeyFromPrivate(t *testing.T) {
	tests := []struct {
		name    string
		keyType KeyType
	}{
		{"ECDSA", KeyTypeECDSA},
		{"RSA", KeyTypeRSA},
		{"Ed25519", KeyTypeEd25519},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			keyPair, _ := GenerateKeyPair(tt.keyType, 256)
			priv, _, _ := LoadPrivateKey(keyPair.PrivateKey, "")

			pub, err := GetPublicKeyFromPrivate(priv)
			if err != nil {
				t.Errorf("GetPublicKeyFromPrivate() error = %v", err)
				return
			}

			switch tt.keyType {
			case KeyTypeECDSA:
				if _, ok := pub.(*ecdsa.PublicKey); !ok {
					t.Error("expected *ecdsa.PublicKey")
				}
			case KeyTypeRSA:
				if _, ok := pub.(*rsa.PublicKey); !ok {
					t.Error("expected *rsa.PublicKey")
				}
			case KeyTypeEd25519:
				if _, ok := pub.(ed25519.PublicKey); !ok {
					t.Error("expected ed25519.PublicKey")
				}
			}
		})
	}

	t.Run("unsupported key type", func(t *testing.T) {
		_, err := GetPublicKeyFromPrivate("invalid")
		if err == nil {
			t.Error("expected error for unsupported key type")
		}
	})
}
