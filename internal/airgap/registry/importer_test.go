package registry

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/shawnbutts/keystone-core/internal/registry/storage"
)

func TestImportModulesFromDir_FilesystemLayout(t *testing.T) {
	// Create a source directory with FilesystemBackend layout
	srcDir := filepath.Join(t.TempDir(), "source")
	srcBackend, err := storage.NewFilesystemBackend(srcDir)
	if err != nil {
		t.Fatal(err)
	}

	// Publish some modules to the source
	for _, m := range []struct{ name, version string }{
		{"test/alpha", "1.0.0"},
		{"test/alpha", "2.0.0"},
		{"test/beta", "1.0.0"},
	} {
		_, err := srcBackend.Publish(context.Background(), &storage.PublishRequest{
			ModuleName:  m.name,
			Version:     m.version,
			ZipData:     bytes.NewReader([]byte("PK\x03\x04data")),
			Manifest:    []byte("name: " + m.name + "\nversion: " + m.version + "\n"),
			Hash:        "hash-" + m.name + "-" + m.version,
			Description: m.name + " module",
		})
		if err != nil {
			t.Fatalf("Publish %s@%s: %v", m.name, m.version, err)
		}
	}

	// Create target registry and import
	root := filepath.Join(t.TempDir(), "registry")
	reg, err := Init(Config{RootDir: root})
	if err != nil {
		t.Fatalf("Init: %v", err)
	}
	defer reg.Close()

	result, err := reg.ImportModulesFromDir(context.Background(), srcDir)
	if err != nil {
		t.Fatalf("ImportModulesFromDir: %v", err)
	}

	if result.ModulesImported != 3 {
		t.Errorf("ModulesImported = %d, want 3", result.ModulesImported)
	}
	if len(result.Errors) != 0 {
		t.Errorf("unexpected errors: %v", result.Errors)
	}

	// Verify modules are in the registry
	client := NewLocalClient(reg)
	versions, err := client.ListVersions("test/alpha")
	if err != nil {
		t.Fatalf("ListVersions: %v", err)
	}
	if len(versions) != 2 {
		t.Errorf("test/alpha versions = %d, want 2", len(versions))
	}
}

func TestImportModulesFromDir_SkipExisting(t *testing.T) {
	srcDir := filepath.Join(t.TempDir(), "source")
	srcBackend, err := storage.NewFilesystemBackend(srcDir)
	if err != nil {
		t.Fatal(err)
	}

	_, err = srcBackend.Publish(context.Background(), &storage.PublishRequest{
		ModuleName: "test/mod",
		Version:    "1.0.0",
		ZipData:    bytes.NewReader([]byte("PK\x03\x04data")),
		Hash:       "hash1",
	})
	if err != nil {
		t.Fatal(err)
	}

	root := filepath.Join(t.TempDir(), "registry")
	reg, err := Init(Config{RootDir: root})
	if err != nil {
		t.Fatal(err)
	}
	defer reg.Close()

	// Import once
	_, err = reg.ImportModulesFromDir(context.Background(), srcDir)
	if err != nil {
		t.Fatal(err)
	}

	// Import again — should skip
	result, err := reg.ImportModulesFromDir(context.Background(), srcDir)
	if err != nil {
		t.Fatal(err)
	}
	if result.ModulesImported != 0 {
		t.Errorf("expected 0 imports on second run, got %d", result.ModulesImported)
	}
	if result.Skipped != 1 {
		t.Errorf("expected 1 skip on second run, got %d", result.Skipped)
	}
}

func TestImportModulesFromDir_FlatZips(t *testing.T) {
	dir := t.TempDir()

	// Create a flat .zip file
	zipPath := filepath.Join(dir, "test-module-1.0.0.zip")
	if err := os.WriteFile(zipPath, []byte("PK\x03\x04data"), 0o644); err != nil {
		t.Fatal(err)
	}

	root := filepath.Join(t.TempDir(), "registry")
	reg, err := Init(Config{RootDir: root})
	if err != nil {
		t.Fatal(err)
	}
	defer reg.Close()

	result, err := reg.ImportModulesFromDir(context.Background(), dir)
	if err != nil {
		t.Fatal(err)
	}
	if result.ModulesImported != 1 {
		t.Errorf("ModulesImported = %d, want 1", result.ModulesImported)
	}
}

func TestImportBlueprintsFromDir_Directories(t *testing.T) {
	srcDir := t.TempDir()

	// Create blueprint directories
	for _, name := range []string{"web-app", "database"} {
		bpDir := filepath.Join(srcDir, name)
		if err := os.MkdirAll(bpDir, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(bpDir, "blueprint.yaml"), []byte("name: "+name), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	root := filepath.Join(t.TempDir(), "registry")
	reg, err := Init(Config{RootDir: root})
	if err != nil {
		t.Fatal(err)
	}
	defer reg.Close()

	result, err := reg.ImportBlueprintsFromDir(context.Background(), srcDir)
	if err != nil {
		t.Fatal(err)
	}
	if result.BlueprintsImported != 2 {
		t.Errorf("BlueprintsImported = %d, want 2", result.BlueprintsImported)
	}

	// Verify blueprints exist in registry
	bpDir := reg.BlueprintsDir()
	for _, name := range []string{"web-app", "database"} {
		if _, err := os.Stat(filepath.Join(bpDir, name, "blueprint.yaml")); err != nil {
			t.Errorf("blueprint %s not found: %v", name, err)
		}
	}
}

func TestImportModulesFromDir_EmptyDir(t *testing.T) {
	dir := t.TempDir()

	root := filepath.Join(t.TempDir(), "registry")
	reg, err := Init(Config{RootDir: root})
	if err != nil {
		t.Fatal(err)
	}
	defer reg.Close()

	result, err := reg.ImportModulesFromDir(context.Background(), dir)
	if err != nil {
		t.Fatal(err)
	}
	if result.ModulesImported != 0 {
		t.Errorf("expected 0 imports for empty dir, got %d", result.ModulesImported)
	}
}

func TestParseModuleFilename(t *testing.T) {
	tests := []struct {
		filename     string
		wantName     string
		wantVersion  string
	}{
		{"test-module-1.0.0.zip", "test/module", "1.0.0"},
		{"simple-2.0.0.zip", "simple", "2.0.0"},
		{"noversion.zip", "noversion", "0.0.0"},
		{"notazip.txt", "", ""},
	}

	for _, tt := range tests {
		name, version := parseModuleFilename(tt.filename)
		if name != tt.wantName || version != tt.wantVersion {
			t.Errorf("parseModuleFilename(%q) = (%q, %q), want (%q, %q)",
				tt.filename, name, version, tt.wantName, tt.wantVersion)
		}
	}
}

func TestImportModulesFromDir_AutoIndex(t *testing.T) {
	srcDir := filepath.Join(t.TempDir(), "source")
	srcBackend, err := storage.NewFilesystemBackend(srcDir)
	if err != nil {
		t.Fatal(err)
	}

	_, err = srcBackend.Publish(context.Background(), &storage.PublishRequest{
		ModuleName: "test/mod",
		Version:    "1.0.0",
		ZipData:    bytes.NewReader([]byte("PK\x03\x04data")),
		Hash:       "hash1",
	})
	if err != nil {
		t.Fatal(err)
	}

	root := filepath.Join(t.TempDir(), "registry")
	reg, err := Init(Config{RootDir: root, AutoIndex: true})
	if err != nil {
		t.Fatal(err)
	}
	defer reg.Close()

	_, err = reg.ImportModulesFromDir(context.Background(), srcDir)
	if err != nil {
		t.Fatal(err)
	}

	idx := reg.Index()
	if idx == nil {
		t.Fatal("expected index after import with AutoIndex")
	}
	if len(idx.Modules) != 1 {
		t.Errorf("expected 1 module in index, got %d", len(idx.Modules))
	}
}
