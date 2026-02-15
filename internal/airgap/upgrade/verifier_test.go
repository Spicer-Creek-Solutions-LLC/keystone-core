package upgrade

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/shawnbutts/keystone-core/internal/airgap/bootstrap"
	"github.com/shawnbutts/keystone-core/internal/signing"
)

func setupSignedUpgradePackage(t *testing.T) (string, []byte) {
	t.Helper()

	kp, err := signing.GenerateKeyPair(signing.KeyTypeEd25519, 0)
	if err != nil {
		t.Fatalf("GenerateKeyPair: %v", err)
	}

	// Build a real upgrade package
	buildDir := setupBuildDir(t, "1.1.0")
	outputPath := filepath.Join(t.TempDir(), "upgrade.tar.gz")

	builder, err := NewBuilder(BuilderConfig{
		FromVersion: "1.0.0",
		ToVersion:   "1.1.0",
		Platform:    bootstrap.Platform{OS: "linux", Arch: "amd64"},
		BuildDir:    buildDir,
		OutputPath:  outputPath,
		SigningKey:   kp.PrivateKey,
	})
	if err != nil {
		t.Fatalf("NewBuilder: %v", err)
	}

	if _, err := builder.Build(context.Background()); err != nil {
		t.Fatalf("Build: %v", err)
	}

	// Extract so we can verify against the directory
	extractDir := t.TempDir()
	if err := bootstrap.ExtractArchive(outputPath, extractDir); err != nil {
		t.Fatalf("ExtractArchive: %v", err)
	}

	return extractDir, kp.PublicKey
}

func TestPackageVerifier_Verify_Signed(t *testing.T) {
	packageDir, pubKey := setupSignedUpgradePackage(t)

	v := NewPackageVerifier([][]byte{pubKey})
	result, err := v.Verify(context.Background(), packageDir)
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}
	if !result.Valid {
		t.Errorf("expected valid, got error: %v", result.Error)
	}
	if !result.SignaturePresent {
		t.Error("expected signature present")
	}
	if !result.ManifestValid {
		t.Error("expected manifest valid")
	}
	if !result.ChecksumsValid {
		t.Error("expected checksums valid")
	}
}

func TestPackageVerifier_Verify_Tampered(t *testing.T) {
	packageDir, pubKey := setupSignedUpgradePackage(t)

	// Tamper with a binary
	binPath := filepath.Join(packageDir, "bin", "kscore-server")
	os.WriteFile(binPath, []byte("tampered"), 0o755)

	v := NewPackageVerifier([][]byte{pubKey})
	result, _ := v.Verify(context.Background(), packageDir)
	if result.Valid {
		t.Error("expected invalid for tampered package")
	}
}

func TestPackageVerifier_Verify_WrongKey(t *testing.T) {
	packageDir, _ := setupSignedUpgradePackage(t)

	// Use a different key
	kp2, _ := signing.GenerateKeyPair(signing.KeyTypeEd25519, 0)
	v := NewPackageVerifier([][]byte{kp2.PublicKey})
	result, _ := v.Verify(context.Background(), packageDir)
	if result.Valid {
		t.Error("expected invalid for wrong key")
	}
}

func TestPackageVerifier_Verify_Unsigned(t *testing.T) {
	buildDir := setupBuildDir(t, "1.1.0")
	outputPath := filepath.Join(t.TempDir(), "upgrade.tar.gz")

	builder, _ := NewBuilder(BuilderConfig{
		FromVersion: "1.0.0",
		ToVersion:   "1.1.0",
		Platform:    bootstrap.Platform{OS: "linux", Arch: "amd64"},
		BuildDir:    buildDir,
		OutputPath:  outputPath,
	})
	builder.Build(context.Background())

	extractDir := t.TempDir()
	bootstrap.ExtractArchive(outputPath, extractDir)

	v := NewPackageVerifier(nil)
	result, _ := v.Verify(context.Background(), extractDir)
	if !result.Valid {
		t.Errorf("unsigned package should be valid when no keys required: %v", result.Error)
	}
	if result.SignaturePresent {
		t.Error("expected no signature")
	}
}

func TestPackageVerifier_Verify_MissingManifest(t *testing.T) {
	dir := t.TempDir()
	v := NewPackageVerifier(nil)
	result, _ := v.Verify(context.Background(), dir)
	if result.Valid {
		t.Error("expected invalid for missing manifest")
	}
}

func TestPackageVerifier_CheckCompatibility(t *testing.T) {
	v := NewPackageVerifier(nil)

	manifest := &Manifest{
		SchemaVersion: "1.0",
		FromVersion:   "1.0.0",
		ToVersion:     "1.1.0",
		Platform:      bootstrap.Platform{OS: "linux", Arch: "amd64"},
		Created:       time.Now(),
		Components:    []bootstrap.ComponentEntry{{Name: "server", Path: "bin/server"}},
		BreakingChanges: []string{"removed API v1"},
	}

	tests := []struct {
		name       string
		current    string
		compatible bool
	}{
		{"exact from version", "1.0.0", true},
		{"newer than from", "1.0.5", true},
		{"already at target", "1.1.0", false},
		{"newer than target", "1.2.0", false},
		{"older than from", "0.9.0", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := v.CheckCompatibility(manifest, tt.current)
			if err != nil {
				t.Fatalf("CheckCompatibility: %v", err)
			}
			if result.Compatible != tt.compatible {
				t.Errorf("Compatible = %v, want %v (blockers: %v)", result.Compatible, tt.compatible, result.Blockers)
			}
		})
	}
}

func TestPackageVerifier_CheckCompatibility_InvalidVersions(t *testing.T) {
	v := NewPackageVerifier(nil)

	manifest := &Manifest{
		FromVersion: "notaversion",
		ToVersion:   "1.1.0",
	}
	result, _ := v.CheckCompatibility(manifest, "1.0.0")
	if result.Compatible {
		t.Error("expected incompatible for invalid from_version")
	}

	result, _ = v.CheckCompatibility(manifest, "alsobad")
	if result.Compatible {
		t.Error("expected incompatible for invalid current version")
	}
}
