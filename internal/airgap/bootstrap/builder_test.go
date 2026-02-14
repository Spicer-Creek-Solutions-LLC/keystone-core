package bootstrap

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/shawnbutts/keystone-core/internal/signing"
)

func TestNewBuilder_Validation(t *testing.T) {
	tests := []struct {
		name    string
		config  BuilderConfig
		wantErr bool
	}{
		{
			"valid",
			BuilderConfig{Version: "0.1.0", Platform: Platform{OS: "linux", Arch: "amd64"}, BuildDir: "/tmp"},
			false,
		},
		{
			"missing version",
			BuilderConfig{Platform: Platform{OS: "linux", Arch: "amd64"}, BuildDir: "/tmp"},
			true,
		},
		{
			"invalid platform",
			BuilderConfig{Version: "0.1.0", Platform: Platform{OS: "freebsd", Arch: "amd64"}, BuildDir: "/tmp"},
			true,
		},
		{
			"missing build dir",
			BuilderConfig{Version: "0.1.0", Platform: Platform{OS: "linux", Arch: "amd64"}},
			true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := NewBuilder(tt.config)
			if (err != nil) != tt.wantErr {
				t.Errorf("NewBuilder() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestBuilder_Build_Unsigned(t *testing.T) {
	buildDir := setupFakeBuildDir(t)
	outputDir := t.TempDir()
	outputPath := filepath.Join(outputDir, "test-package.tar.gz")

	builder, err := NewBuilder(BuilderConfig{
		Version:    "0.1.0",
		Platform:   Platform{OS: "linux", Arch: "amd64"},
		BuildDir:   buildDir,
		OutputPath: outputPath,
		CreatedBy:  "test",
	})
	if err != nil {
		t.Fatalf("NewBuilder: %v", err)
	}

	manifest, err := builder.Build(context.Background())
	if err != nil {
		t.Fatalf("Build: %v", err)
	}

	// Verify manifest
	if manifest.Version != "0.1.0" {
		t.Errorf("Version = %q, want %q", manifest.Version, "0.1.0")
	}
	if manifest.Platform.OS != "linux" {
		t.Errorf("Platform.OS = %q, want %q", manifest.Platform.OS, "linux")
	}
	if len(manifest.Components) != 4 {
		t.Errorf("Components = %d, want 4", len(manifest.Components))
	}
	if manifest.Checksum == "" {
		t.Error("Checksum should not be empty")
	}
	if manifest.RequiresVerification {
		t.Error("RequiresVerification should be false for unsigned")
	}

	// Verify archive exists and can be read
	verifyArchiveContents(t, outputPath, []string{
		"manifest.json",
		"bin/kscore-server",
		"bin/kscore-agent",
		"bin/kscorectl",
		"bin/kscore-exec",
	})
}

func TestBuilder_Build_Signed(t *testing.T) {
	buildDir := setupFakeBuildDir(t)
	outputDir := t.TempDir()
	outputPath := filepath.Join(outputDir, "signed-package.tar.gz")

	kp, err := signing.GenerateKeyPair(signing.KeyTypeEd25519, 0)
	if err != nil {
		t.Fatalf("GenerateKeyPair: %v", err)
	}

	builder, err := NewBuilder(BuilderConfig{
		Version:    "0.1.0",
		Platform:   Platform{OS: "linux", Arch: "amd64"},
		BuildDir:   buildDir,
		OutputPath: outputPath,
		SigningKey:  kp.PrivateKey,
	})
	if err != nil {
		t.Fatalf("NewBuilder: %v", err)
	}

	manifest, err := builder.Build(context.Background())
	if err != nil {
		t.Fatalf("Build: %v", err)
	}

	if !manifest.RequiresVerification {
		t.Error("RequiresVerification should be true for signed package")
	}

	// Verify archive includes signatures
	verifyArchiveContents(t, outputPath, []string{
		"manifest.json",
		"signatures/manifest.json.sig",
		"signatures/cosign.pub",
		"bin/kscore-server",
	})
}

func TestBuilder_Build_DefaultOutputPath(t *testing.T) {
	buildDir := setupFakeBuildDir(t)
	origDir, _ := os.Getwd()
	tmpDir := t.TempDir()
	os.Chdir(tmpDir)
	defer os.Chdir(origDir)

	builder, err := NewBuilder(BuilderConfig{
		Version:  "0.1.0",
		Platform: Platform{OS: "linux", Arch: "amd64"},
		BuildDir: buildDir,
	})
	if err != nil {
		t.Fatalf("NewBuilder: %v", err)
	}

	_, err = builder.Build(context.Background())
	if err != nil {
		t.Fatalf("Build: %v", err)
	}

	expectedPath := filepath.Join(tmpDir, "keystone-bootstrap-0.1.0-linux-amd64.tar.gz")
	if _, err := os.Stat(expectedPath); err != nil {
		t.Fatalf("expected default output at %s: %v", expectedPath, err)
	}
}

func verifyArchiveContents(t *testing.T, archivePath string, expectedFiles []string) {
	t.Helper()

	f, err := os.Open(archivePath)
	if err != nil {
		t.Fatalf("open archive: %v", err)
	}
	defer f.Close()

	gr, err := gzip.NewReader(f)
	if err != nil {
		t.Fatalf("gzip reader: %v", err)
	}
	defer gr.Close()

	tr := tar.NewReader(gr)
	found := make(map[string]bool)

	for {
		header, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatalf("tar reader: %v", err)
		}
		found[header.Name] = true
	}

	for _, expected := range expectedFiles {
		if !found[expected] {
			t.Errorf("expected file %q not found in archive (found: %v)", expected, mapKeys(found))
		}
	}
}

func mapKeys(m map[string]bool) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	return keys
}
