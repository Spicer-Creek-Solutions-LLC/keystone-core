package bootstrap

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestPackageVerifier_VerifyManifestSignature_NoSigFile(t *testing.T) {
	dir := t.TempDir()
	writeTestFile(t, filepath.Join(dir, "manifest.json"), `{"v":1}`)

	kp := generateTestKeyPair(t)
	v := NewPackageVerifier([][]byte{kp.PublicKey})

	err := v.VerifyManifestSignature(context.Background(), dir)
	if err == nil {
		t.Fatal("expected error when signature file is missing")
	}
}

func TestPackageVerifier_VerifyManifestSignature_NoTrustedKeys(t *testing.T) {
	kp := generateTestKeyPair(t)
	dir := setupSignedPackage(t, kp.PrivateKey)

	v := NewPackageVerifier(nil)
	err := v.VerifyManifestSignature(context.Background(), dir)
	if err == nil {
		t.Fatal("expected error with no trusted keys")
	}
}

func TestPackageVerifier_VerifyChecksums(t *testing.T) {
	dir := t.TempDir()
	binPath := filepath.Join(dir, "bin", "kscore-server")
	writeTestFile(t, binPath, "binary-content")

	hash, err := HashFile(binPath)
	if err != nil {
		t.Fatalf("HashFile: %v", err)
	}

	checksum, err := CalculateChecksum(dir)
	if err != nil {
		t.Fatalf("CalculateChecksum: %v", err)
	}

	manifest := &Manifest{
		SchemaVersion: SchemaVersion,
		Version:       "0.1.0",
		Platform:      Platform{OS: "linux", Arch: "amd64"},
		Created:       time.Now(),
		Components: []ComponentEntry{
			{Name: "kscore-server", Path: "bin/kscore-server", SHA256: hash, Size: 14},
		},
		Checksum: checksum,
	}

	v := NewPackageVerifier(nil)
	if err := v.VerifyChecksums(dir, manifest); err != nil {
		t.Fatalf("VerifyChecksums should pass: %v", err)
	}
}

func TestPackageVerifier_VerifyChecksums_Mismatch(t *testing.T) {
	dir := t.TempDir()
	writeTestFile(t, filepath.Join(dir, "bin", "kscore-server"), "binary-content")

	manifest := &Manifest{
		SchemaVersion: SchemaVersion,
		Version:       "0.1.0",
		Platform:      Platform{OS: "linux", Arch: "amd64"},
		Created:       time.Now(),
		Components: []ComponentEntry{
			{Name: "kscore-server", Path: "bin/kscore-server", SHA256: repeatChar('f', 64), Size: 14},
		},
	}

	v := NewPackageVerifier(nil)
	if err := v.VerifyChecksums(dir, manifest); err == nil {
		t.Fatal("expected checksum mismatch error")
	}
}

func TestPackageVerifier_VerifyChecksums_MissingFile(t *testing.T) {
	dir := t.TempDir()

	manifest := &Manifest{
		SchemaVersion: SchemaVersion,
		Version:       "0.1.0",
		Platform:      Platform{OS: "linux", Arch: "amd64"},
		Created:       time.Now(),
		Components: []ComponentEntry{
			{Name: "kscore-server", Path: "bin/kscore-server", SHA256: repeatChar('a', 64), Size: 100},
		},
	}

	v := NewPackageVerifier(nil)
	if err := v.VerifyChecksums(dir, manifest); err == nil {
		t.Fatal("expected error for missing file")
	}
}

func TestPackageVerifier_VerifyChecksums_OverallMismatch(t *testing.T) {
	dir := t.TempDir()
	writeTestFile(t, filepath.Join(dir, "bin", "server"), "data")

	manifest := &Manifest{
		SchemaVersion: SchemaVersion,
		Version:       "0.1.0",
		Platform:      Platform{OS: "linux", Arch: "amd64"},
		Created:       time.Now(),
		Components: []ComponentEntry{
			{Name: "server", Path: "bin/server"},
		},
		Checksum: repeatChar('0', 64),
	}

	v := NewPackageVerifier(nil)
	if err := v.VerifyChecksums(dir, manifest); err == nil {
		t.Fatal("expected overall checksum mismatch")
	}
}

func TestPackageVerifier_VerifyChecksums_EmptySHA256Skipped(t *testing.T) {
	dir := t.TempDir()
	writeTestFile(t, filepath.Join(dir, "bin", "server"), "data")

	manifest := &Manifest{
		SchemaVersion: SchemaVersion,
		Version:       "0.1.0",
		Platform:      Platform{OS: "linux", Arch: "amd64"},
		Created:       time.Now(),
		Components: []ComponentEntry{
			{Name: "server", Path: "bin/server", SHA256: ""},
		},
	}

	v := NewPackageVerifier(nil)
	if err := v.VerifyChecksums(dir, manifest); err != nil {
		t.Fatalf("empty SHA256 should be skipped: %v", err)
	}
}

func TestPackageVerifier_VerifyChecksums_ContentEntries(t *testing.T) {
	dir := t.TempDir()
	modPath := filepath.Join(dir, "modules", "std-core.tar.gz")
	writeTestFile(t, modPath, "module-data")
	writeTestFile(t, filepath.Join(dir, "bin", "server"), "data")

	modHash, _ := HashFile(modPath)

	manifest := &Manifest{
		SchemaVersion: SchemaVersion,
		Version:       "0.1.0",
		Platform:      Platform{OS: "linux", Arch: "amd64"},
		Created:       time.Now(),
		Components: []ComponentEntry{
			{Name: "server", Path: "bin/server"},
		},
		Modules: []ContentEntry{
			{Name: "std/core", Path: "modules/std-core.tar.gz", SHA256: modHash},
		},
	}

	v := NewPackageVerifier(nil)
	if err := v.VerifyChecksums(dir, manifest); err != nil {
		t.Fatalf("VerifyChecksums should pass with content entries: %v", err)
	}
}

func setupSignedPackage(t *testing.T, privateKey []byte) string {
	t.Helper()
	dir := t.TempDir()
	writeTestFile(t, filepath.Join(dir, "manifest.json"), `{"v":1}`)

	signer, err := NewPackageSigner(privateKey)
	if err != nil {
		t.Fatalf("NewPackageSigner: %v", err)
	}
	if err := signer.SignManifest(context.Background(), dir); err != nil {
		t.Fatalf("SignManifest: %v", err)
	}
	return dir
}

func TestEndToEnd_SignVerifyChecksums(t *testing.T) {
	kp := generateTestKeyPair(t)
	dir := t.TempDir()

	// Create content
	writeTestFile(t, filepath.Join(dir, "bin", "kscore-server"), "server-bin")
	writeTestFile(t, filepath.Join(dir, "modules", "core.tar.gz"), "mod-data")

	// Hash files
	serverHash, _ := HashFile(filepath.Join(dir, "bin", "kscore-server"))
	modHash, _ := HashFile(filepath.Join(dir, "modules", "core.tar.gz"))
	checksum, _ := CalculateChecksum(dir)

	// Build manifest
	manifest := &Manifest{
		SchemaVersion:        SchemaVersion,
		Version:              "0.1.0",
		Platform:             Platform{OS: "linux", Arch: "amd64"},
		Created:              time.Now(),
		RequiresVerification: true,
		Components: []ComponentEntry{
			{Name: "kscore-server", Version: "0.1.0", Path: "bin/kscore-server", SHA256: serverHash},
		},
		Modules: []ContentEntry{
			{Name: "core", Path: "modules/core.tar.gz", SHA256: modHash},
		},
		Checksum: checksum,
	}

	// Write manifest (must happen after CalculateChecksum since manifest is excluded)
	if err := WriteManifest(manifest, filepath.Join(dir, "manifest.json")); err != nil {
		t.Fatalf("WriteManifest: %v", err)
	}

	// Sign
	signer, err := NewPackageSigner(kp.PrivateKey)
	if err != nil {
		t.Fatalf("NewPackageSigner: %v", err)
	}
	ctx := context.Background()
	if err := signer.SignManifest(ctx, dir); err != nil {
		t.Fatalf("SignManifest: %v", err)
	}

	// Verify signature
	verifier := NewPackageVerifier([][]byte{kp.PublicKey})
	if err := verifier.VerifyManifestSignature(ctx, dir); err != nil {
		t.Fatalf("VerifyManifestSignature: %v", err)
	}

	// Verify checksums
	loaded, err := ReadManifest(filepath.Join(dir, "manifest.json"))
	if err != nil {
		t.Fatalf("ReadManifest: %v", err)
	}
	if err := verifier.VerifyChecksums(dir, loaded); err != nil {
		t.Fatalf("VerifyChecksums: %v", err)
	}

	// Tamper with a file
	if err := os.WriteFile(filepath.Join(dir, "bin", "kscore-server"), []byte("tampered"), 0o644); err != nil {
		t.Fatalf("write tampered: %v", err)
	}

	if err := verifier.VerifyChecksums(dir, loaded); err == nil {
		t.Fatal("VerifyChecksums should fail after tampering")
	}
}
