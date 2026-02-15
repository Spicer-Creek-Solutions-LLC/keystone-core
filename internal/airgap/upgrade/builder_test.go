package upgrade

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/shawnbutts/keystone-core/internal/airgap/bootstrap"
)

func setupBuildDir(t *testing.T, version string) string {
	t.Helper()
	dir := t.TempDir()
	binDir := filepath.Join(dir, "linux", "amd64")
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"kscore-server", "kscore-agent", "kscorectl"} {
		path := filepath.Join(binDir, name)
		if err := os.WriteFile(path, []byte("binary-"+name+"-"+version), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

func TestNewBuilder_Validation(t *testing.T) {
	tests := []struct {
		name   string
		config BuilderConfig
	}{
		{"missing from", BuilderConfig{ToVersion: "1.1.0", Platform: bootstrap.Platform{OS: "linux", Arch: "amd64"}, BuildDir: "/tmp"}},
		{"missing to", BuilderConfig{FromVersion: "1.0.0", Platform: bootstrap.Platform{OS: "linux", Arch: "amd64"}, BuildDir: "/tmp"}},
		{"missing platform", BuilderConfig{FromVersion: "1.0.0", ToVersion: "1.1.0", BuildDir: "/tmp"}},
		{"missing build dir", BuilderConfig{FromVersion: "1.0.0", ToVersion: "1.1.0", Platform: bootstrap.Platform{OS: "linux", Arch: "amd64"}}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := NewBuilder(tt.config)
			if err == nil {
				t.Error("expected validation error")
			}
		})
	}
}

func TestBuilder_Build(t *testing.T) {
	buildDir := setupBuildDir(t, "1.1.0")
	outputDir := t.TempDir()
	outputPath := filepath.Join(outputDir, "upgrade.tar.gz")

	builder, err := NewBuilder(BuilderConfig{
		FromVersion: "1.0.0",
		ToVersion:   "1.1.0",
		Platform:    bootstrap.Platform{OS: "linux", Arch: "amd64"},
		BuildDir:    buildDir,
		OutputPath:  outputPath,
		CreatedBy:   "test",
	})
	if err != nil {
		t.Fatalf("NewBuilder: %v", err)
	}

	manifest, err := builder.Build(context.Background())
	if err != nil {
		t.Fatalf("Build: %v", err)
	}

	if manifest.FromVersion != "1.0.0" {
		t.Errorf("FromVersion = %q, want 1.0.0", manifest.FromVersion)
	}
	if manifest.ToVersion != "1.1.0" {
		t.Errorf("ToVersion = %q, want 1.1.0", manifest.ToVersion)
	}
	if len(manifest.Components) != 3 {
		t.Errorf("Components count = %d, want 3", len(manifest.Components))
	}
	if manifest.Checksum == "" {
		t.Error("Checksum should be set")
	}

	// Verify archive was created
	if _, err := os.Stat(outputPath); err != nil {
		t.Errorf("archive not created: %v", err)
	}
}

func TestBuilder_BuildWithMigrations(t *testing.T) {
	buildDir := setupBuildDir(t, "1.1.0")

	// Create migrations dir
	migrationsDir := filepath.Join(t.TempDir(), "migrations")
	os.MkdirAll(migrationsDir, 0o755)
	os.WriteFile(filepath.Join(migrationsDir, "001-add-table.sql"), []byte("CREATE TABLE t;"), 0o644)
	os.WriteFile(filepath.Join(migrationsDir, "002-add-column.sql"), []byte("ALTER TABLE t ADD col;"), 0o644)

	outputPath := filepath.Join(t.TempDir(), "upgrade.tar.gz")

	builder, err := NewBuilder(BuilderConfig{
		FromVersion:   "1.0.0",
		ToVersion:     "1.1.0",
		Platform:      bootstrap.Platform{OS: "linux", Arch: "amd64"},
		BuildDir:      buildDir,
		OutputPath:    outputPath,
		MigrationsDir: migrationsDir,
	})
	if err != nil {
		t.Fatalf("NewBuilder: %v", err)
	}

	manifest, err := builder.Build(context.Background())
	if err != nil {
		t.Fatalf("Build: %v", err)
	}

	if len(manifest.Migrations) != 2 {
		t.Fatalf("Migrations count = %d, want 2", len(manifest.Migrations))
	}
	if manifest.Migrations[0].Name != "001-add-table.sql" {
		t.Errorf("first migration = %q", manifest.Migrations[0].Name)
	}
	if manifest.Migrations[1].Order != 2 {
		t.Errorf("second migration order = %d, want 2", manifest.Migrations[1].Order)
	}
}

func TestBuilder_BuildWithScripts(t *testing.T) {
	buildDir := setupBuildDir(t, "1.1.0")

	preDir := filepath.Join(t.TempDir(), "pre")
	postDir := filepath.Join(t.TempDir(), "post")
	os.MkdirAll(preDir, 0o755)
	os.MkdirAll(postDir, 0o755)
	os.WriteFile(filepath.Join(preDir, "check.sh"), []byte("#!/bin/sh\necho ok"), 0o755)
	os.WriteFile(filepath.Join(postDir, "verify.sh"), []byte("#!/bin/sh\necho done"), 0o755)

	outputPath := filepath.Join(t.TempDir(), "upgrade.tar.gz")

	builder, err := NewBuilder(BuilderConfig{
		FromVersion:    "1.0.0",
		ToVersion:      "1.1.0",
		Platform:       bootstrap.Platform{OS: "linux", Arch: "amd64"},
		BuildDir:       buildDir,
		OutputPath:     outputPath,
		PreScriptsDir:  preDir,
		PostScriptsDir: postDir,
	})
	if err != nil {
		t.Fatal(err)
	}

	manifest, err := builder.Build(context.Background())
	if err != nil {
		t.Fatalf("Build: %v", err)
	}

	if len(manifest.PreScripts) != 1 {
		t.Errorf("PreScripts count = %d, want 1", len(manifest.PreScripts))
	}
	if len(manifest.PostScripts) != 1 {
		t.Errorf("PostScripts count = %d, want 1", len(manifest.PostScripts))
	}
}

func TestBuilder_DefaultOutputPath(t *testing.T) {
	buildDir := setupBuildDir(t, "1.1.0")
	outputDir := t.TempDir()

	// Change working directory to output dir so default path lands there
	orig, _ := os.Getwd()
	os.Chdir(outputDir)
	defer os.Chdir(orig)

	builder, err := NewBuilder(BuilderConfig{
		FromVersion: "1.0.0",
		ToVersion:   "1.1.0",
		Platform:    bootstrap.Platform{OS: "linux", Arch: "amd64"},
		BuildDir:    buildDir,
	})
	if err != nil {
		t.Fatal(err)
	}

	_, err = builder.Build(context.Background())
	if err != nil {
		t.Fatalf("Build: %v", err)
	}

	expected := filepath.Join(outputDir, "keystone-upgrade-1.0.0-to-1.1.0-linux-amd64.tar.gz")
	if _, err := os.Stat(expected); err != nil {
		t.Errorf("default output file not found: %v", err)
	}
}
