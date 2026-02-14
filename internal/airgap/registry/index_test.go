package registry

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/shawnbutts/keystone-core/internal/registry/storage"
)

func setupRegistryForIndex(t *testing.T) (*Registry, string) {
	t.Helper()
	root := filepath.Join(t.TempDir(), "registry")
	reg, err := Init(Config{RootDir: root})
	if err != nil {
		t.Fatalf("Init: %v", err)
	}

	modules := []struct {
		name, version, desc string
		tags                []string
	}{
		{"vendor/alpha", "1.0.0", "Alpha module", []string{"networking"}},
		{"vendor/alpha", "2.0.0", "Alpha module v2", []string{"networking", "dns"}},
		{"std/files", "1.0.0", "File operations", []string{"filesystem"}},
	}

	for _, m := range modules {
		manifest := []byte("name: " + m.name + "\nversion: " + m.version + "\ntype: starlark\nentrypoint: main.star\n")
		_, err := reg.Backend().Publish(context.Background(), &storage.PublishRequest{
			ModuleName:  m.name,
			Version:     m.version,
			ZipData:     bytes.NewReader([]byte("PK\x03\x04data")),
			Manifest:    manifest,
			Hash:        "hash-" + m.version,
			Description: m.desc,
			Tags:        m.tags,
		})
		if err != nil {
			t.Fatalf("Publish %s@%s: %v", m.name, m.version, err)
		}
	}

	return reg, root
}

func TestGenerate(t *testing.T) {
	_, root := setupRegistryForIndex(t)

	idx, err := Generate(root)
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}

	if idx.SchemaVersion != indexSchemaVersion {
		t.Errorf("schema = %q, want %q", idx.SchemaVersion, indexSchemaVersion)
	}
	if len(idx.Modules) != 2 {
		t.Fatalf("expected 2 modules, got %d", len(idx.Modules))
	}

	// Modules should be sorted by name
	if idx.Modules[0].Name != "std/files" {
		t.Errorf("first module = %q, want std/files", idx.Modules[0].Name)
	}
	if idx.Modules[1].Name != "vendor/alpha" {
		t.Errorf("second module = %q, want vendor/alpha", idx.Modules[1].Name)
	}

	// vendor/alpha should have 2 versions with latest = 2.0.0
	alpha := idx.Modules[1]
	if alpha.LatestVersion != "2.0.0" {
		t.Errorf("alpha latest = %q, want 2.0.0", alpha.LatestVersion)
	}
	if len(alpha.Versions) != 2 {
		t.Errorf("alpha versions count = %d, want 2", len(alpha.Versions))
	}
}

func TestGenerate_EmptyRegistry(t *testing.T) {
	root := filepath.Join(t.TempDir(), "registry")
	_, err := Init(Config{RootDir: root})
	if err != nil {
		t.Fatalf("Init: %v", err)
	}

	idx, err := Generate(root)
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if len(idx.Modules) != 0 {
		t.Errorf("expected 0 modules, got %d", len(idx.Modules))
	}
}

func TestIndex_SaveAndLoad(t *testing.T) {
	idx := &Index{
		SchemaVersion: indexSchemaVersion,
		Modules: []ModuleEntry{
			{Name: "test/mod", LatestVersion: "1.0.0", Versions: []string{"1.0.0"}, Tags: []string{"test"}},
		},
		Blueprints: []BlueprintEntry{
			{Name: "basic-setup"},
		},
	}

	path := filepath.Join(t.TempDir(), "index.json")
	if err := idx.Save(path); err != nil {
		t.Fatalf("Save: %v", err)
	}

	loaded, err := LoadIndex(path)
	if err != nil {
		t.Fatalf("LoadIndex: %v", err)
	}

	if loaded.SchemaVersion != indexSchemaVersion {
		t.Errorf("schema = %q, want %q", loaded.SchemaVersion, indexSchemaVersion)
	}
	if len(loaded.Modules) != 1 {
		t.Fatalf("modules count = %d, want 1", len(loaded.Modules))
	}
	if loaded.Modules[0].Name != "test/mod" {
		t.Errorf("module name = %q, want test/mod", loaded.Modules[0].Name)
	}
	if len(loaded.Blueprints) != 1 {
		t.Fatalf("blueprints count = %d, want 1", len(loaded.Blueprints))
	}
}

func TestLoadIndex_InvalidJSON(t *testing.T) {
	path := filepath.Join(t.TempDir(), "bad.json")
	if err := os.WriteFile(path, []byte("not json"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := LoadIndex(path)
	if err == nil {
		t.Fatal("expected error for invalid JSON")
	}
}

func TestLoadIndex_NotFound(t *testing.T) {
	_, err := LoadIndex("/nonexistent/index.json")
	if err == nil {
		t.Fatal("expected error for missing file")
	}
}

func TestIndex_Search(t *testing.T) {
	idx := &Index{
		SchemaVersion: indexSchemaVersion,
		Modules: []ModuleEntry{
			{Name: "std/files", Description: "File operations", Tags: []string{"filesystem"}},
			{Name: "vendor/dns", Description: "DNS resolver", Tags: []string{"networking"}},
			{Name: "vendor/http", Description: "HTTP client", Tags: []string{"networking"}},
		},
	}

	tests := []struct {
		query string
		want  int
	}{
		{"", 3},                // empty query returns all
		{"dns", 1},            // matches name
		{"networking", 2},     // matches tag
		{"file", 1},           // matches name "files" and tag "filesystem"
		{"nonexistent", 0},    // no matches
		{"HTTP", 1},           // case-insensitive
	}

	for _, tt := range tests {
		results := idx.Search(tt.query)
		if len(results) != tt.want {
			t.Errorf("Search(%q) returned %d results, want %d", tt.query, len(results), tt.want)
		}
	}
}

func TestIndex_SearchBlueprints(t *testing.T) {
	idx := &Index{
		SchemaVersion: indexSchemaVersion,
		Blueprints: []BlueprintEntry{
			{Name: "web-server", Description: "Web server setup"},
			{Name: "database", Description: "Database cluster"},
		},
	}

	tests := []struct {
		query string
		want  int
	}{
		{"", 2},
		{"web", 1},
		{"cluster", 1},
		{"nope", 0},
	}

	for _, tt := range tests {
		results := idx.SearchBlueprints(tt.query)
		if len(results) != tt.want {
			t.Errorf("SearchBlueprints(%q) returned %d, want %d", tt.query, len(results), tt.want)
		}
	}
}

func TestIndex_JSONRoundtrip(t *testing.T) {
	idx := &Index{
		SchemaVersion: indexSchemaVersion,
		Modules: []ModuleEntry{
			{Name: "a/b", LatestVersion: "1.0.0", Versions: []string{"1.0.0"}, Description: "test", Tags: []string{"t"}},
		},
	}

	data, err := json.MarshalIndent(idx, "", "  ")
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var loaded Index
	if err := json.Unmarshal(data, &loaded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if loaded.Modules[0].Name != "a/b" {
		t.Errorf("name = %q, want a/b", loaded.Modules[0].Name)
	}
}

func TestGenerate_WithBlueprints(t *testing.T) {
	root := filepath.Join(t.TempDir(), "registry")
	_, err := Init(Config{RootDir: root})
	if err != nil {
		t.Fatalf("Init: %v", err)
	}

	// Create blueprint directories
	for _, name := range []string{"web-app", "database"} {
		dir := filepath.Join(root, "blueprints", name)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}

	idx, err := Generate(root)
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if len(idx.Blueprints) != 2 {
		t.Errorf("expected 2 blueprints, got %d", len(idx.Blueprints))
	}
}

func TestRegistry_Reindex_WithModules(t *testing.T) {
	reg, _ := setupRegistryForIndex(t)
	defer reg.Close()

	if err := reg.Reindex(); err != nil {
		t.Fatalf("Reindex: %v", err)
	}

	idx := reg.Index()
	if idx == nil {
		t.Fatal("expected index after reindex")
	}
	if len(idx.Modules) != 2 {
		t.Errorf("expected 2 modules after reindex, got %d", len(idx.Modules))
	}
}
