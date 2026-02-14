package bootstrap

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestNewInstaller_Validation(t *testing.T) {
	tests := []struct {
		name    string
		opts    InstallOptions
		wantErr bool
	}{
		{
			name:    "missing archive path",
			opts:    InstallOptions{},
			wantErr: true,
		},
		{
			name: "valid with defaults",
			opts: InstallOptions{ArchivePath: "/tmp/test.tar.gz"},
		},
		{
			name: "valid with all options",
			opts: InstallOptions{
				ArchivePath: "/tmp/test.tar.gz",
				TargetDir:   "/opt/bin",
				ConfigDir:   "/opt/etc",
				DataDir:     "/opt/data",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			inst, err := NewInstaller(tt.opts)
			if tt.wantErr {
				if err == nil {
					t.Error("expected error")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if inst == nil {
				t.Error("expected non-nil installer")
			}
		})
	}
}

func TestInstaller_Defaults(t *testing.T) {
	inst, err := NewInstaller(InstallOptions{ArchivePath: "/tmp/test.tar.gz"})
	if err != nil {
		t.Fatal(err)
	}
	if inst.opts.TargetDir != "/usr/local/bin" {
		t.Errorf("expected default target dir /usr/local/bin, got %s", inst.opts.TargetDir)
	}
	if inst.opts.ConfigDir != "/etc/keystone-core" {
		t.Errorf("expected default config dir /etc/keystone-core, got %s", inst.opts.ConfigDir)
	}
	if inst.opts.DataDir != "/var/lib/keystone-core" {
		t.Errorf("expected default data dir /var/lib/keystone-core, got %s", inst.opts.DataDir)
	}
}

func TestInstaller_Install_EndToEnd(t *testing.T) {
	// Build a package first
	buildDir := t.TempDir()
	binDir := filepath.Join(buildDir, "linux", "amd64")
	if err := os.MkdirAll(binDir, 0o750); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"kscore-server", "kscore-agent", "kscorectl"} {
		writeTestFile(t, filepath.Join(binDir, name), "#!/bin/sh\necho "+name)
	}

	// Create module and blueprint source dirs
	modulesDir := t.TempDir()
	writeTestFile(t, filepath.Join(modulesDir, "std-files-0.1.0.tar.gz"), "module-data")

	blueprintsDir := t.TempDir()
	bpDir := filepath.Join(blueprintsDir, "web-app")
	if err := os.MkdirAll(bpDir, 0o750); err != nil {
		t.Fatal(err)
	}
	writeTestFile(t, filepath.Join(bpDir, "blueprint.yaml"), "name: web-app")

	outputDir := t.TempDir()
	archivePath := filepath.Join(outputDir, "test-package.tar.gz")

	builder, err := NewBuilder(BuilderConfig{
		Version:           "1.0.0",
		Platform:          Platform{OS: "linux", Arch: "amd64"},
		BuildDir:          buildDir,
		OutputPath:        archivePath,
		IncludeModules:    true,
		IncludeBlueprints: true,
		ModulesDir:        modulesDir,
		BlueprintsDir:     blueprintsDir,
	})
	if err != nil {
		t.Fatal(err)
	}

	ctx := context.Background()
	_, err = builder.Build(ctx)
	if err != nil {
		t.Fatalf("Build() error: %v", err)
	}

	// Now install it
	targetDir := filepath.Join(t.TempDir(), "bin")
	configDir := filepath.Join(t.TempDir(), "etc")
	dataDir := filepath.Join(t.TempDir(), "data")

	installer, err := NewInstaller(InstallOptions{
		ArchivePath: archivePath,
		TargetDir:   targetDir,
		ConfigDir:   configDir,
		DataDir:     dataDir,
	})
	if err != nil {
		t.Fatal(err)
	}

	result, err := installer.Install(ctx)
	if err != nil {
		t.Fatalf("Install() error: %v", err)
	}

	// Verify results
	if result.BinariesCount != 3 {
		t.Errorf("expected 3 binaries installed, got %d", result.BinariesCount)
	}
	if result.Manifest == nil {
		t.Fatal("expected manifest in result")
	}

	// Verify binaries are installed and executable
	for _, name := range []string{"kscore-server", "kscore-agent", "kscorectl"} {
		binPath := filepath.Join(targetDir, name)
		info, err := os.Stat(binPath)
		if err != nil {
			t.Errorf("binary %s not found: %v", name, err)
			continue
		}
		if info.Mode().Perm()&0o111 == 0 {
			t.Errorf("binary %s should be executable", name)
		}
	}

	// Verify modules installed
	if result.ModulesCount != 1 {
		t.Errorf("expected 1 module installed, got %d", result.ModulesCount)
	}

	// Verify blueprints installed
	if result.BlueprintsCount != 1 {
		t.Errorf("expected 1 blueprint installed, got %d", result.BlueprintsCount)
	}
}

func TestInstaller_Install_SkipModules(t *testing.T) {
	buildDir := t.TempDir()
	binDir := filepath.Join(buildDir, "linux", "amd64")
	if err := os.MkdirAll(binDir, 0o750); err != nil {
		t.Fatal(err)
	}
	writeTestFile(t, filepath.Join(binDir, "kscore-server"), "binary")
	writeTestFile(t, filepath.Join(binDir, "kscore-agent"), "binary")
	writeTestFile(t, filepath.Join(binDir, "kscorectl"), "binary")

	modulesDir := t.TempDir()
	writeTestFile(t, filepath.Join(modulesDir, "mod.tar.gz"), "data")

	outputDir := t.TempDir()
	archivePath := filepath.Join(outputDir, "test.tar.gz")

	builder, err := NewBuilder(BuilderConfig{
		Version:        "1.0.0",
		Platform:       Platform{OS: "linux", Arch: "amd64"},
		BuildDir:       buildDir,
		OutputPath:     archivePath,
		IncludeModules: true,
		ModulesDir:     modulesDir,
	})
	if err != nil {
		t.Fatal(err)
	}

	ctx := context.Background()
	if _, err = builder.Build(ctx); err != nil {
		t.Fatal(err)
	}

	installer, err := NewInstaller(InstallOptions{
		ArchivePath: archivePath,
		TargetDir:   filepath.Join(t.TempDir(), "bin"),
		ConfigDir:   filepath.Join(t.TempDir(), "etc"),
		DataDir:     filepath.Join(t.TempDir(), "data"),
		SkipModules: true,
	})
	if err != nil {
		t.Fatal(err)
	}

	result, err := installer.Install(ctx)
	if err != nil {
		t.Fatalf("Install() error: %v", err)
	}

	if result.ModulesCount != 0 {
		t.Errorf("expected 0 modules (skipped), got %d", result.ModulesCount)
	}
}

func TestInstaller_Install_WithVerification(t *testing.T) {
	buildDir := t.TempDir()
	binDir := filepath.Join(buildDir, "linux", "amd64")
	if err := os.MkdirAll(binDir, 0o750); err != nil {
		t.Fatal(err)
	}
	writeTestFile(t, filepath.Join(binDir, "kscore-server"), "binary")
	writeTestFile(t, filepath.Join(binDir, "kscore-agent"), "binary")
	writeTestFile(t, filepath.Join(binDir, "kscorectl"), "binary")

	// Generate key pair
	kp := generateTestKeyPair(t)
	privKey, pubKey := kp.PrivateKey, kp.PublicKey

	outputDir := t.TempDir()
	archivePath := filepath.Join(outputDir, "signed.tar.gz")

	builder, err := NewBuilder(BuilderConfig{
		Version:    "1.0.0",
		Platform:   Platform{OS: "linux", Arch: "amd64"},
		BuildDir:   buildDir,
		OutputPath: archivePath,
		SigningKey:  privKey,
	})
	if err != nil {
		t.Fatal(err)
	}

	ctx := context.Background()
	if _, err = builder.Build(ctx); err != nil {
		t.Fatal(err)
	}

	// Install with verification
	installer, err := NewInstaller(InstallOptions{
		ArchivePath: archivePath,
		TargetDir:   filepath.Join(t.TempDir(), "bin"),
		ConfigDir:   filepath.Join(t.TempDir(), "etc"),
		DataDir:     filepath.Join(t.TempDir(), "data"),
		Verify:      true,
		TrustedKeys: [][]byte{pubKey},
	})
	if err != nil {
		t.Fatal(err)
	}

	result, err := installer.Install(ctx)
	if err != nil {
		t.Fatalf("Install() with verification error: %v", err)
	}
	if result.BinariesCount != 3 {
		t.Errorf("expected 3 binaries, got %d", result.BinariesCount)
	}
}

func TestInstaller_Install_VerificationFails(t *testing.T) {
	buildDir := t.TempDir()
	binDir := filepath.Join(buildDir, "linux", "amd64")
	if err := os.MkdirAll(binDir, 0o750); err != nil {
		t.Fatal(err)
	}
	writeTestFile(t, filepath.Join(binDir, "kscore-server"), "binary")
	writeTestFile(t, filepath.Join(binDir, "kscore-agent"), "binary")
	writeTestFile(t, filepath.Join(binDir, "kscorectl"), "binary")

	// Build unsigned package
	outputDir := t.TempDir()
	archivePath := filepath.Join(outputDir, "unsigned.tar.gz")

	builder, err := NewBuilder(BuilderConfig{
		Version:    "1.0.0",
		Platform:   Platform{OS: "linux", Arch: "amd64"},
		BuildDir:   buildDir,
		OutputPath: archivePath,
	})
	if err != nil {
		t.Fatal(err)
	}

	ctx := context.Background()
	if _, err = builder.Build(ctx); err != nil {
		t.Fatal(err)
	}

	// Try to install with verification — should fail (no signature)
	kp := generateTestKeyPair(t)
	pubKey := kp.PublicKey

	installer, err := NewInstaller(InstallOptions{
		ArchivePath: archivePath,
		TargetDir:   filepath.Join(t.TempDir(), "bin"),
		ConfigDir:   filepath.Join(t.TempDir(), "etc"),
		DataDir:     filepath.Join(t.TempDir(), "data"),
		Verify:      true,
		TrustedKeys: [][]byte{pubKey},
	})
	if err != nil {
		t.Fatal(err)
	}

	_, err = installer.Install(ctx)
	if err == nil {
		t.Error("expected verification error for unsigned package")
	}
}

func TestInstaller_ConfigNoOverwrite(t *testing.T) {
	buildDir := t.TempDir()
	binDir := filepath.Join(buildDir, "linux", "amd64")
	if err := os.MkdirAll(binDir, 0o750); err != nil {
		t.Fatal(err)
	}
	writeTestFile(t, filepath.Join(binDir, "kscore-server"), "binary")
	writeTestFile(t, filepath.Join(binDir, "kscore-agent"), "binary")
	writeTestFile(t, filepath.Join(binDir, "kscorectl"), "binary")

	outputDir := t.TempDir()
	archivePath := filepath.Join(outputDir, "test.tar.gz")

	builder, err := NewBuilder(BuilderConfig{
		Version:    "1.0.0",
		Platform:   Platform{OS: "linux", Arch: "amd64"},
		BuildDir:   buildDir,
		OutputPath: archivePath,
	})
	if err != nil {
		t.Fatal(err)
	}

	ctx := context.Background()
	if _, err = builder.Build(ctx); err != nil {
		t.Fatal(err)
	}

	configDir := filepath.Join(t.TempDir(), "etc")
	if err := os.MkdirAll(configDir, 0o750); err != nil {
		t.Fatal(err)
	}

	// Pre-create a config file
	existingContent := "existing-config"
	writeTestFile(t, filepath.Join(configDir, "server.yaml.tmpl"), existingContent)

	installer, err := NewInstaller(InstallOptions{
		ArchivePath: archivePath,
		TargetDir:   filepath.Join(t.TempDir(), "bin"),
		ConfigDir:   configDir,
		DataDir:     filepath.Join(t.TempDir(), "data"),
	})
	if err != nil {
		t.Fatal(err)
	}

	if _, err = installer.Install(ctx); err != nil {
		t.Fatalf("Install() error: %v", err)
	}

	// Verify existing config was not overwritten
	data, err := os.ReadFile(filepath.Join(configDir, "server.yaml.tmpl"))
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != existingContent {
		t.Error("existing config file should not be overwritten")
	}
}
