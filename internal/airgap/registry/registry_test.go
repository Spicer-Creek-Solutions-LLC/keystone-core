package registry

import (
	"os"
	"path/filepath"
	"testing"
)

func TestNew_RequiresRootDir(t *testing.T) {
	_, err := New(Config{})
	if err == nil {
		t.Fatal("expected error for empty root dir")
	}
}

func TestNew_RequiresExistingDir(t *testing.T) {
	_, err := New(Config{RootDir: "/nonexistent/path/airgap-reg"})
	if err == nil {
		t.Fatal("expected error for nonexistent dir")
	}
}

func TestNew_RejectsFile(t *testing.T) {
	tmp := t.TempDir()
	f := filepath.Join(tmp, "notadir")
	if err := os.WriteFile(f, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := New(Config{RootDir: f})
	if err == nil {
		t.Fatal("expected error for file path")
	}
}

func TestInit_CreatesDirectoryStructure(t *testing.T) {
	root := filepath.Join(t.TempDir(), "registry")

	reg, err := Init(Config{RootDir: root})
	if err != nil {
		t.Fatalf("Init: %v", err)
	}
	defer reg.Close()

	for _, sub := range []string{"modules", "blueprints"} {
		info, err := os.Stat(filepath.Join(root, sub))
		if err != nil {
			t.Errorf("expected %s dir: %v", sub, err)
		} else if !info.IsDir() {
			t.Errorf("%s is not a directory", sub)
		}
	}

	// Index file should exist
	if _, err := os.Stat(filepath.Join(root, "index.json")); err != nil {
		t.Errorf("expected index.json: %v", err)
	}
}

func TestInit_RequiresRootDir(t *testing.T) {
	_, err := Init(Config{})
	if err == nil {
		t.Fatal("expected error for empty root dir")
	}
}

func TestInit_LoadsIndex(t *testing.T) {
	root := filepath.Join(t.TempDir(), "registry")
	reg, err := Init(Config{RootDir: root})
	if err != nil {
		t.Fatalf("Init: %v", err)
	}
	defer reg.Close()

	idx := reg.Index()
	if idx == nil {
		t.Fatal("expected index to be loaded")
	}
	if idx.SchemaVersion != indexSchemaVersion {
		t.Errorf("schema version = %q, want %q", idx.SchemaVersion, indexSchemaVersion)
	}
}

func TestRegistry_Accessors(t *testing.T) {
	root := filepath.Join(t.TempDir(), "registry")
	reg, err := Init(Config{RootDir: root})
	if err != nil {
		t.Fatalf("Init: %v", err)
	}
	defer reg.Close()

	if reg.Backend() == nil {
		t.Error("Backend() should not be nil")
	}
	if got := reg.ModulesDir(); got != filepath.Join(root, "modules") {
		t.Errorf("ModulesDir() = %q, want %q", got, filepath.Join(root, "modules"))
	}
	if got := reg.BlueprintsDir(); got != filepath.Join(root, "blueprints") {
		t.Errorf("BlueprintsDir() = %q, want %q", got, filepath.Join(root, "blueprints"))
	}
}

func TestRegistry_Reindex(t *testing.T) {
	root := filepath.Join(t.TempDir(), "registry")
	reg, err := Init(Config{RootDir: root})
	if err != nil {
		t.Fatalf("Init: %v", err)
	}
	defer reg.Close()

	if err := reg.Reindex(); err != nil {
		t.Fatalf("Reindex: %v", err)
	}

	idx := reg.Index()
	if idx == nil {
		t.Fatal("expected index after reindex")
	}
	if len(idx.Modules) != 0 {
		t.Errorf("expected 0 modules in empty registry, got %d", len(idx.Modules))
	}
}

func TestRegistry_Close(t *testing.T) {
	root := filepath.Join(t.TempDir(), "registry")
	reg, err := Init(Config{RootDir: root})
	if err != nil {
		t.Fatalf("Init: %v", err)
	}
	if err := reg.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
}
