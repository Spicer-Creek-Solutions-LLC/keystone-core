package bootstrap

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/shawnbutts/keystone-core/internal/signing"
)

func generateTestKeyPair(t *testing.T) *signing.KeyPair {
	t.Helper()
	kp, err := signing.GenerateKeyPair(signing.KeyTypeEd25519, 0)
	if err != nil {
		t.Fatalf("GenerateKeyPair: %v", err)
	}
	return kp
}

func TestNewPackageSigner(t *testing.T) {
	kp := generateTestKeyPair(t)

	s, err := NewPackageSigner(kp.PrivateKey)
	if err != nil {
		t.Fatalf("NewPackageSigner: %v", err)
	}
	if s == nil {
		t.Fatal("signer should not be nil")
	}
}

func TestNewPackageSigner_InvalidKey(t *testing.T) {
	_, err := NewPackageSigner([]byte("not-a-key"))
	if err == nil {
		t.Fatal("expected error for invalid key")
	}
}

func TestNewPackageSignerFromFile(t *testing.T) {
	kp := generateTestKeyPair(t)
	dir := t.TempDir()
	keyPath := filepath.Join(dir, "key.pem")
	if err := os.WriteFile(keyPath, kp.PrivateKey, 0o600); err != nil {
		t.Fatalf("write key: %v", err)
	}

	s, err := NewPackageSignerFromFile(keyPath)
	if err != nil {
		t.Fatalf("NewPackageSignerFromFile: %v", err)
	}
	if s == nil {
		t.Fatal("signer should not be nil")
	}
}

func TestNewPackageSignerFromFile_NotFound(t *testing.T) {
	_, err := NewPackageSignerFromFile("/nonexistent/key.pem")
	if err == nil {
		t.Fatal("expected error for missing key file")
	}
}

func TestPackageSigner_SignManifest(t *testing.T) {
	kp := generateTestKeyPair(t)
	s, err := NewPackageSigner(kp.PrivateKey)
	if err != nil {
		t.Fatalf("NewPackageSigner: %v", err)
	}

	dir := t.TempDir()
	manifestPath := filepath.Join(dir, "manifest.json")
	if err := os.WriteFile(manifestPath, []byte(`{"version":"0.1.0"}`), 0o644); err != nil {
		t.Fatalf("write manifest: %v", err)
	}

	ctx := context.Background()
	if err := s.SignManifest(ctx, dir); err != nil {
		t.Fatalf("SignManifest: %v", err)
	}

	// Verify sig file exists
	sigPath := filepath.Join(dir, "signatures", "manifest.json.sig")
	if _, err := os.Stat(sigPath); err != nil {
		t.Fatalf("signature file should exist: %v", err)
	}

	// Verify sig file is non-empty
	data, err := os.ReadFile(sigPath)
	if err != nil {
		t.Fatalf("read sig: %v", err)
	}
	if len(data) == 0 {
		t.Error("signature file should not be empty")
	}
}

func TestPackageSigner_WritePublicKey(t *testing.T) {
	kp := generateTestKeyPair(t)
	s, err := NewPackageSigner(kp.PrivateKey)
	if err != nil {
		t.Fatalf("NewPackageSigner: %v", err)
	}

	dir := t.TempDir()
	if err := s.WritePublicKey(dir); err != nil {
		t.Fatalf("WritePublicKey: %v", err)
	}

	pubPath := filepath.Join(dir, "signatures", "cosign.pub")
	data, err := os.ReadFile(pubPath)
	if err != nil {
		t.Fatalf("read public key: %v", err)
	}
	if len(data) == 0 {
		t.Error("public key file should not be empty")
	}
}

func TestSignAndVerifyRoundtrip(t *testing.T) {
	kp := generateTestKeyPair(t)

	// Sign
	signer, err := NewPackageSigner(kp.PrivateKey)
	if err != nil {
		t.Fatalf("NewPackageSigner: %v", err)
	}

	dir := t.TempDir()
	manifestPath := filepath.Join(dir, "manifest.json")
	if err := os.WriteFile(manifestPath, []byte(`{"version":"0.1.0","schema_version":"1.0"}`), 0o644); err != nil {
		t.Fatalf("write manifest: %v", err)
	}

	ctx := context.Background()
	if err := signer.SignManifest(ctx, dir); err != nil {
		t.Fatalf("SignManifest: %v", err)
	}
	if err := signer.WritePublicKey(dir); err != nil {
		t.Fatalf("WritePublicKey: %v", err)
	}

	// Verify with correct key
	verifier := NewPackageVerifier([][]byte{kp.PublicKey})
	if err := verifier.VerifyManifestSignature(ctx, dir); err != nil {
		t.Fatalf("VerifyManifestSignature should succeed: %v", err)
	}
}

func TestSignAndVerify_WrongKey(t *testing.T) {
	kp1 := generateTestKeyPair(t)
	kp2 := generateTestKeyPair(t)

	signer, err := NewPackageSigner(kp1.PrivateKey)
	if err != nil {
		t.Fatalf("NewPackageSigner: %v", err)
	}

	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "manifest.json"), []byte(`{"v":1}`), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	ctx := context.Background()
	if err := signer.SignManifest(ctx, dir); err != nil {
		t.Fatalf("SignManifest: %v", err)
	}

	// Verify with wrong key should fail
	verifier := NewPackageVerifier([][]byte{kp2.PublicKey})
	if err := verifier.VerifyManifestSignature(ctx, dir); err == nil {
		t.Fatal("verification should fail with wrong key")
	}
}

func TestSignAndVerify_TamperedManifest(t *testing.T) {
	kp := generateTestKeyPair(t)

	signer, err := NewPackageSigner(kp.PrivateKey)
	if err != nil {
		t.Fatalf("NewPackageSigner: %v", err)
	}

	dir := t.TempDir()
	manifestPath := filepath.Join(dir, "manifest.json")
	if err := os.WriteFile(manifestPath, []byte(`{"version":"0.1.0"}`), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	ctx := context.Background()
	if err := signer.SignManifest(ctx, dir); err != nil {
		t.Fatalf("SignManifest: %v", err)
	}

	// Tamper with manifest after signing
	if err := os.WriteFile(manifestPath, []byte(`{"version":"0.2.0"}`), 0o644); err != nil {
		t.Fatalf("write tampered: %v", err)
	}

	verifier := NewPackageVerifier([][]byte{kp.PublicKey})
	if err := verifier.VerifyManifestSignature(ctx, dir); err == nil {
		t.Fatal("verification should fail for tampered manifest")
	}
}
