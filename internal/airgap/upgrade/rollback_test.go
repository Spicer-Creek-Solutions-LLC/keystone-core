package upgrade

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/shawnbutts/keystone-core/internal/airgap/bootstrap"
)

func TestRollback(t *testing.T) {
	// First, do an upgrade to create a backup
	packagePath := buildUpgradePackage(t)
	installDir := setupInstallDir(t)
	backupDir := t.TempDir()

	inst, _ := NewInstaller(InstallerConfig{
		PackagePath: packagePath,
		InstallDir:  installDir,
		BackupDir:   backupDir,
	})

	installResult, err := inst.Install(context.Background())
	if err != nil {
		t.Fatalf("Install: %v", err)
	}

	// Verify binaries were changed
	data, _ := os.ReadFile(filepath.Join(installDir, "kscore-server"))
	if string(data) == "old-kscore-server" {
		t.Fatal("binary should have been replaced before rollback test")
	}

	// Now rollback
	rollbackResult, err := Rollback(context.Background(), RollbackConfig{
		BackupDir:  installResult.BackupPath,
		InstallDir: installDir,
	})
	if err != nil {
		t.Fatalf("Rollback: %v", err)
	}

	if len(rollbackResult.RestoredComponents) != 3 {
		t.Errorf("RestoredComponents = %d, want 3", len(rollbackResult.RestoredComponents))
	}

	// Verify binaries were restored
	data, _ = os.ReadFile(filepath.Join(installDir, "kscore-server"))
	if string(data) != "old-kscore-server" {
		t.Error("binary should have been restored to old version")
	}
}

func TestRollback_DryRun(t *testing.T) {
	packagePath := buildUpgradePackage(t)
	installDir := setupInstallDir(t)
	backupDir := t.TempDir()

	inst, _ := NewInstaller(InstallerConfig{
		PackagePath: packagePath,
		InstallDir:  installDir,
		BackupDir:   backupDir,
	})
	installResult, _ := inst.Install(context.Background())

	result, err := Rollback(context.Background(), RollbackConfig{
		BackupDir:  installResult.BackupPath,
		InstallDir: installDir,
		DryRun:     true,
	})
	if err != nil {
		t.Fatalf("Rollback dry run: %v", err)
	}

	if len(result.RestoredComponents) != 3 {
		t.Errorf("dry run should list 3 components, got %d", len(result.RestoredComponents))
	}

	// Verify binaries were NOT restored (still upgraded)
	data, _ := os.ReadFile(filepath.Join(installDir, "kscore-server"))
	if string(data) == "old-kscore-server" {
		t.Error("dry run should not modify binaries")
	}
}

func TestRollback_MissingBackupDir(t *testing.T) {
	_, err := Rollback(context.Background(), RollbackConfig{
		InstallDir: t.TempDir(),
	})
	if err == nil {
		t.Error("expected error for missing backup dir")
	}
}

func TestRollback_MissingInstallDir(t *testing.T) {
	_, err := Rollback(context.Background(), RollbackConfig{
		BackupDir: t.TempDir(),
	})
	if err == nil {
		t.Error("expected error for missing install dir")
	}
}

func TestRollback_MissingManifest(t *testing.T) {
	_, err := Rollback(context.Background(), RollbackConfig{
		BackupDir:  t.TempDir(),
		InstallDir: t.TempDir(),
	})
	if err == nil {
		t.Error("expected error for missing backup manifest")
	}
}

func TestRollback_MissingBackupFile(t *testing.T) {
	// Create a backup manifest pointing to a file that doesn't exist
	backupDir := t.TempDir()
	installDir := t.TempDir()

	manifest := &Manifest{
		SchemaVersion: "1.0",
		FromVersion:   "1.0.0",
		ToVersion:     "1.1.0",
		Platform:      bootstrap.Platform{OS: "linux", Arch: "amd64"},
		Components: []bootstrap.ComponentEntry{
			{Name: "kscore-missing", Path: "kscore-missing"},
		},
	}
	WriteManifest(manifest, filepath.Join(backupDir, "manifest.json"))

	result, err := Rollback(context.Background(), RollbackConfig{
		BackupDir:  backupDir,
		InstallDir: installDir,
	})
	if err != nil {
		t.Fatalf("should skip missing backup files, not error: %v", err)
	}
	if len(result.SkippedComponents) != 1 {
		t.Errorf("SkippedComponents = %d, want 1", len(result.SkippedComponents))
	}
}
