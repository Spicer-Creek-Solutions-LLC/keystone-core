package upgrade

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/shawnbutts/keystone-core/internal/airgap/bootstrap"
)

func buildUpgradePackage(t *testing.T) string {
	t.Helper()
	buildDir := setupBuildDir(t, "1.1.0")
	outputPath := filepath.Join(t.TempDir(), "upgrade.tar.gz")

	builder, err := NewBuilder(BuilderConfig{
		FromVersion: "1.0.0",
		ToVersion:   "1.1.0",
		Platform:    bootstrap.Platform{OS: "linux", Arch: "amd64"},
		BuildDir:    buildDir,
		OutputPath:  outputPath,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := builder.Build(context.Background()); err != nil {
		t.Fatal(err)
	}
	return outputPath
}

func setupInstallDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	for _, name := range []string{"kscore-server", "kscore-agent", "kscorectl"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("old-"+name), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

func TestNewInstaller_Validation(t *testing.T) {
	tests := []struct {
		name   string
		config InstallerConfig
	}{
		{"missing package", InstallerConfig{InstallDir: "/tmp", BackupDir: "/tmp"}},
		{"missing install dir", InstallerConfig{PackagePath: "/tmp/pkg.tar.gz", BackupDir: "/tmp"}},
		{"missing backup dir", InstallerConfig{PackagePath: "/tmp/pkg.tar.gz", InstallDir: "/tmp"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := NewInstaller(tt.config)
			if err == nil {
				t.Error("expected validation error")
			}
		})
	}
}

func TestNewInstaller_SkipBackup(t *testing.T) {
	_, err := NewInstaller(InstallerConfig{
		PackagePath: "/tmp/pkg.tar.gz",
		InstallDir:  "/tmp",
		SkipBackup:  true,
	})
	if err != nil {
		t.Errorf("should not require backup dir when SkipBackup=true: %v", err)
	}
}

func TestInstaller_Install(t *testing.T) {
	packagePath := buildUpgradePackage(t)
	installDir := setupInstallDir(t)
	backupDir := t.TempDir()

	inst, err := NewInstaller(InstallerConfig{
		PackagePath: packagePath,
		InstallDir:  installDir,
		BackupDir:   backupDir,
	})
	if err != nil {
		t.Fatal(err)
	}

	result, err := inst.Install(context.Background())
	if err != nil {
		t.Fatalf("Install: %v", err)
	}

	if len(result.UpgradedComponents) != 3 {
		t.Errorf("UpgradedComponents = %d, want 3", len(result.UpgradedComponents))
	}
	if result.BackupPath == "" {
		t.Error("BackupPath should be set")
	}
	if result.FromVersion != "1.0.0" {
		t.Errorf("FromVersion = %q, want 1.0.0", result.FromVersion)
	}
	if result.ToVersion != "1.1.0" {
		t.Errorf("ToVersion = %q, want 1.1.0", result.ToVersion)
	}

	// Verify binaries were replaced
	data, _ := os.ReadFile(filepath.Join(installDir, "kscore-server"))
	if string(data) == "old-kscore-server" {
		t.Error("binary should have been replaced")
	}

	// Verify backup exists
	backupManifest := filepath.Join(result.BackupPath, "manifest.json")
	if _, err := os.Stat(backupManifest); err != nil {
		t.Errorf("backup manifest missing: %v", err)
	}
}

func TestInstaller_Install_DryRun(t *testing.T) {
	packagePath := buildUpgradePackage(t)
	installDir := setupInstallDir(t)

	inst, err := NewInstaller(InstallerConfig{
		PackagePath: packagePath,
		InstallDir:  installDir,
		BackupDir:   t.TempDir(),
		DryRun:      true,
	})
	if err != nil {
		t.Fatal(err)
	}

	result, err := inst.Install(context.Background())
	if err != nil {
		t.Fatalf("Install dry run: %v", err)
	}

	if len(result.UpgradedComponents) != 3 {
		t.Errorf("dry run should list 3 components, got %d", len(result.UpgradedComponents))
	}

	// Verify binaries were NOT replaced
	data, _ := os.ReadFile(filepath.Join(installDir, "kscore-server"))
	if string(data) != "old-kscore-server" {
		t.Error("dry run should not modify binaries")
	}
}

func TestInstaller_Install_SkipBackup(t *testing.T) {
	packagePath := buildUpgradePackage(t)
	installDir := setupInstallDir(t)

	inst, err := NewInstaller(InstallerConfig{
		PackagePath: packagePath,
		InstallDir:  installDir,
		SkipBackup:  true,
	})
	if err != nil {
		t.Fatal(err)
	}

	result, err := inst.Install(context.Background())
	if err != nil {
		t.Fatalf("Install: %v", err)
	}
	if result.BackupPath != "" {
		t.Error("BackupPath should be empty when backup skipped")
	}
}

func TestInstaller_Install_WithProgress(t *testing.T) {
	packagePath := buildUpgradePackage(t)
	installDir := setupInstallDir(t)

	var phases []string
	inst, _ := NewInstaller(InstallerConfig{
		PackagePath: packagePath,
		InstallDir:  installDir,
		BackupDir:   t.TempDir(),
		ProgressFunc: func(phase string, _ int) {
			phases = append(phases, phase)
		},
	})

	inst.Install(context.Background())

	if len(phases) == 0 {
		t.Error("progress callback should have been called")
	}
}

func TestInstaller_Install_InvalidPackage(t *testing.T) {
	badPkg := filepath.Join(t.TempDir(), "bad.tar.gz")
	os.WriteFile(badPkg, []byte("not a tar.gz"), 0o644)

	inst, _ := NewInstaller(InstallerConfig{
		PackagePath: badPkg,
		InstallDir:  t.TempDir(),
		BackupDir:   t.TempDir(),
	})

	_, err := inst.Install(context.Background())
	if err == nil {
		t.Error("expected error for invalid package")
	}
}
