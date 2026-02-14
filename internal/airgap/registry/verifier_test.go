package registry

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/shawnbutts/keystone-core/internal/registry/storage"
	"github.com/shawnbutts/keystone-core/internal/signing"
)

func generateTestKeys(t *testing.T) *signing.KeyPair {
	t.Helper()
	kp, err := signing.GenerateKeyPair(signing.KeyTypeEd25519, 0)
	if err != nil {
		t.Fatalf("GenerateKeyPair: %v", err)
	}
	return kp
}

func signModuleZip(t *testing.T, zipPath string, privateKeyPEM []byte) []byte {
	t.Helper()
	signer, err := signing.NewKeySigner(&signing.KeySignerConfig{
		PrivateKeyPEM: privateKeyPEM,
		HashAlgorithm: signing.HashSHA256,
	})
	if err != nil {
		t.Fatalf("NewKeySigner: %v", err)
	}

	result, err := signer.SignFile(context.Background(), zipPath)
	if err != nil {
		t.Fatalf("SignFile: %v", err)
	}
	return result.Signature
}

func setupSignedRegistry(t *testing.T) (*Registry, *signing.KeyPair) {
	t.Helper()
	root := filepath.Join(t.TempDir(), "registry")
	reg, err := Init(Config{RootDir: root})
	if err != nil {
		t.Fatalf("Init: %v", err)
	}

	kp := generateTestKeys(t)

	// Publish a module
	_, err = reg.Backend().Publish(context.Background(), &storage.PublishRequest{
		ModuleName: "test/signed",
		Version:    "1.0.0",
		ZipData:    bytes.NewReader([]byte("PK\x03\x04signed-data")),
		Hash:       "hash1",
		Signature:  "placeholder",
	})
	if err != nil {
		t.Fatalf("Publish: %v", err)
	}

	// Sign the module.zip with the private key
	zipPath := filepath.Join(reg.ModulesDir(), "test/signed/1.0.0/module.zip")
	sig := signModuleZip(t, zipPath, kp.PrivateKey)

	// Write the real signature to module.sig
	sigPath := filepath.Join(reg.ModulesDir(), "test/signed/1.0.0/module.sig")
	if err := os.WriteFile(sigPath, sig, 0o644); err != nil {
		t.Fatalf("write sig: %v", err)
	}

	return reg, kp
}

func TestVerifyModule_SignedValid(t *testing.T) {
	reg, kp := setupSignedRegistry(t)
	defer reg.Close()

	trustDir := filepath.Join(t.TempDir(), "trust")
	ts, err := NewTrustStore(TrustConfig{TrustDir: trustDir, RequireSignatures: true})
	if err != nil {
		t.Fatal(err)
	}
	if err := ts.Init(); err != nil {
		t.Fatal(err)
	}
	if err := ts.AddRoot(TrustRoot{
		Name:      "test-signer",
		PublicKey: kp.PublicKey,
		Algorithm: "ed25519",
	}); err != nil {
		t.Fatal(err)
	}

	v := NewVerifier(ts)
	result, err := v.VerifyModule(reg.ModulesDir(), "test/signed", "1.0.0")
	if err != nil {
		t.Fatalf("VerifyModule: %v", err)
	}
	if !result.Valid {
		t.Errorf("expected valid, got error: %v", result.Error)
	}
	if result.SignedBy != "test-signer" {
		t.Errorf("SignedBy = %q, want test-signer", result.SignedBy)
	}
}

func TestVerifyModule_TamperedData(t *testing.T) {
	reg, kp := setupSignedRegistry(t)
	defer reg.Close()

	// Tamper with the zip
	zipPath := filepath.Join(reg.ModulesDir(), "test/signed/1.0.0/module.zip")
	if err := os.WriteFile(zipPath, []byte("PK\x03\x04tampered"), 0o644); err != nil {
		t.Fatal(err)
	}

	trustDir := filepath.Join(t.TempDir(), "trust")
	ts, _ := NewTrustStore(TrustConfig{TrustDir: trustDir, RequireSignatures: true})
	ts.Init()
	ts.AddRoot(TrustRoot{Name: "signer", PublicKey: kp.PublicKey, Algorithm: "ed25519"})

	v := NewVerifier(ts)
	result, err := v.VerifyModule(reg.ModulesDir(), "test/signed", "1.0.0")
	if err != nil {
		t.Fatal(err)
	}
	if result.Valid {
		t.Error("expected verification to fail for tampered data")
	}
}

func TestVerifyModule_UnsignedWithRequire(t *testing.T) {
	root := filepath.Join(t.TempDir(), "registry")
	reg, err := Init(Config{RootDir: root})
	if err != nil {
		t.Fatal(err)
	}
	defer reg.Close()

	// Publish unsigned module
	_, err = reg.Backend().Publish(context.Background(), &storage.PublishRequest{
		ModuleName: "test/unsigned",
		Version:    "1.0.0",
		ZipData:    bytes.NewReader([]byte("PK\x03\x04data")),
		Hash:       "hash1",
	})
	if err != nil {
		t.Fatal(err)
	}

	trustDir := filepath.Join(t.TempDir(), "trust")
	ts, _ := NewTrustStore(TrustConfig{TrustDir: trustDir, RequireSignatures: true})
	ts.Init()

	v := NewVerifier(ts)
	result, _ := v.VerifyModule(reg.ModulesDir(), "test/unsigned", "1.0.0")
	if result.Valid {
		t.Error("expected invalid for unsigned module with RequireSignatures")
	}
	if result.Error == nil {
		t.Error("expected error for unsigned module with RequireSignatures")
	}
}

func TestVerifyModule_UnsignedWithoutRequire(t *testing.T) {
	root := filepath.Join(t.TempDir(), "registry")
	reg, err := Init(Config{RootDir: root})
	if err != nil {
		t.Fatal(err)
	}
	defer reg.Close()

	_, err = reg.Backend().Publish(context.Background(), &storage.PublishRequest{
		ModuleName: "test/unsigned",
		Version:    "1.0.0",
		ZipData:    bytes.NewReader([]byte("PK\x03\x04data")),
		Hash:       "hash1",
	})
	if err != nil {
		t.Fatal(err)
	}

	trustDir := filepath.Join(t.TempDir(), "trust")
	ts, _ := NewTrustStore(TrustConfig{TrustDir: trustDir, RequireSignatures: false})
	ts.Init()

	v := NewVerifier(ts)
	result, _ := v.VerifyModule(reg.ModulesDir(), "test/unsigned", "1.0.0")
	if !result.Valid {
		t.Error("expected valid for unsigned module without RequireSignatures")
	}
}

func TestVerifyAll(t *testing.T) {
	reg, kp := setupSignedRegistry(t)
	defer reg.Close()

	// Add an unsigned module
	_, err := reg.Backend().Publish(context.Background(), &storage.PublishRequest{
		ModuleName: "test/other",
		Version:    "1.0.0",
		ZipData:    bytes.NewReader([]byte("PK\x03\x04other")),
		Hash:       "hash2",
	})
	if err != nil {
		t.Fatal(err)
	}

	trustDir := filepath.Join(t.TempDir(), "trust")
	ts, _ := NewTrustStore(TrustConfig{TrustDir: trustDir, RequireSignatures: false})
	ts.Init()
	ts.AddRoot(TrustRoot{Name: "signer", PublicKey: kp.PublicKey, Algorithm: "ed25519"})

	v := NewVerifier(ts)
	results, err := v.VerifyAll(reg.ModulesDir())
	if err != nil {
		t.Fatalf("VerifyAll: %v", err)
	}

	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}

	valid := 0
	for _, r := range results {
		if r.Valid {
			valid++
		}
	}
	if valid != 2 {
		t.Errorf("expected 2 valid (1 signed, 1 unsigned-ok), got %d", valid)
	}
}

func TestVerifyModule_NonexistentModule(t *testing.T) {
	trustDir := filepath.Join(t.TempDir(), "trust")
	ts, _ := NewTrustStore(TrustConfig{TrustDir: trustDir})
	ts.Init()

	v := NewVerifier(ts)
	result, _ := v.VerifyModule(t.TempDir(), "nonexistent/mod", "1.0.0")
	if result.Valid {
		t.Error("expected invalid for nonexistent module")
	}
}

func TestVerifyAll_EmptyRegistry(t *testing.T) {
	root := filepath.Join(t.TempDir(), "registry")
	reg, _ := Init(Config{RootDir: root})
	defer reg.Close()

	trustDir := filepath.Join(t.TempDir(), "trust")
	ts, _ := NewTrustStore(TrustConfig{TrustDir: trustDir})
	ts.Init()

	v := NewVerifier(ts)
	results, err := v.VerifyAll(reg.ModulesDir())
	if err != nil {
		t.Fatalf("VerifyAll: %v", err)
	}
	if len(results) != 0 {
		t.Errorf("expected 0 results for empty registry, got %d", len(results))
	}
}
