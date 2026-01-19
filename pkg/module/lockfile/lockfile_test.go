package lockfile

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestNew(t *testing.T) {
	lf := New()

	if lf.SchemaVersion != CurrentSchemaVersion {
		t.Errorf("SchemaVersion = %d, want %d", lf.SchemaVersion, CurrentSchemaVersion)
	}

	if lf.Metadata == nil {
		t.Fatal("Metadata should not be nil")
	}

	if lf.Modules == nil {
		t.Fatal("Modules should not be nil")
	}

	if lf.Checksums == nil {
		t.Fatal("Checksums should not be nil")
	}

	if lf.Checksums.Algorithm != "sha256" {
		t.Errorf("Checksums.Algorithm = %s, want sha256", lf.Checksums.Algorithm)
	}
}

func TestAddModule(t *testing.T) {
	lf := New()

	module := &LockedModule{
		Version: "1.2.3",
		Hash:    "sha256:abc123def456",
		Source: &ModuleSource{
			Type: "registry",
			URL:  "https://registry.example.com",
		},
	}

	lf.AddModule("example/module", module)

	if !lf.HasModule("example/module") {
		t.Error("Module should exist after AddModule")
	}

	retrieved, ok := lf.GetModule("example/module")
	if !ok {
		t.Fatal("GetModule failed")
	}

	if retrieved.Version != "1.2.3" {
		t.Errorf("Version = %s, want 1.2.3", retrieved.Version)
	}

	// Check checksum was added
	key := "example/module@1.2.3"
	if lf.Checksums.Entries[key] != "sha256:abc123def456" {
		t.Errorf("Checksum = %s, want sha256:abc123def456", lf.Checksums.Entries[key])
	}
}

func TestRemoveModule(t *testing.T) {
	lf := New()

	lf.AddModule("example/module", &LockedModule{
		Version: "1.0.0",
		Hash:    "sha256:abc123",
	})

	if !lf.HasModule("example/module") {
		t.Fatal("Module should exist before removal")
	}

	lf.RemoveModule("example/module")

	if lf.HasModule("example/module") {
		t.Error("Module should not exist after removal")
	}

	// Check checksum was removed
	key := "example/module@1.0.0"
	if _, ok := lf.Checksums.Entries[key]; ok {
		t.Error("Checksum should be removed")
	}
}

func TestListModules(t *testing.T) {
	lf := New()

	lf.AddModule("c/module", &LockedModule{Version: "1.0.0", Hash: "sha256:c"})
	lf.AddModule("a/module", &LockedModule{Version: "1.0.0", Hash: "sha256:a"})
	lf.AddModule("b/module", &LockedModule{Version: "1.0.0", Hash: "sha256:b"})

	names := lf.ListModules()

	if len(names) != 3 {
		t.Fatalf("ListModules returned %d modules, want 3", len(names))
	}

	// Should be sorted alphabetically
	expected := []string{"a/module", "b/module", "c/module"}
	for i, name := range names {
		if name != expected[i] {
			t.Errorf("ListModules[%d] = %s, want %s", i, name, expected[i])
		}
	}
}

func TestValidate(t *testing.T) {
	tests := []struct {
		name    string
		lf      *LockFile
		wantErr bool
	}{
		{
			name: "valid lock file",
			lf: &LockFile{
				SchemaVersion: CurrentSchemaVersion,
				Modules: map[string]*LockedModule{
					"example/module": {
						Version: "1.0.0",
						Hash:    "sha256:abc123def456789012345678901234567890123456789012345678901234abcd",
					},
				},
			},
			wantErr: false,
		},
		{
			name: "invalid schema version",
			lf: &LockFile{
				SchemaVersion: 999,
				Modules:       map[string]*LockedModule{},
			},
			wantErr: true,
		},
		{
			name: "missing version",
			lf: &LockFile{
				SchemaVersion: CurrentSchemaVersion,
				Modules: map[string]*LockedModule{
					"example/module": {
						Hash: "sha256:abc123",
					},
				},
			},
			wantErr: true,
		},
		{
			name: "missing hash",
			lf: &LockFile{
				SchemaVersion: CurrentSchemaVersion,
				Modules: map[string]*LockedModule{
					"example/module": {
						Version: "1.0.0",
					},
				},
			},
			wantErr: true,
		},
		{
			name: "invalid hash format",
			lf: &LockFile{
				SchemaVersion: CurrentSchemaVersion,
				Modules: map[string]*LockedModule{
					"example/module": {
						Version: "1.0.0",
						Hash:    "invalid-hash",
					},
				},
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.lf.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestSaveAndLoad(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "module.lock")

	// Create and save
	lf := New()
	lf.Metadata.GeneratedBy = "test"
	lf.Metadata.KeystoneVersion = "0.1.0"
	lf.Metadata.Comment = "Test lock file"

	lf.AddModule("example/module", &LockedModule{
		Version: "1.2.3",
		Hash:    "sha256:abc123def456789012345678901234567890123456789012345678901234",
		Source: &ModuleSource{
			Type: "registry",
			URL:  "https://registry.example.com",
		},
		Dependencies: map[string]string{
			"dep/one": "^1.0.0",
		},
	})

	if err := lf.Save(path); err != nil {
		t.Fatalf("Save failed: %v", err)
	}

	// Verify file exists
	if _, err := os.Stat(path); os.IsNotExist(err) {
		t.Fatal("Lock file was not created")
	}

	// Load and verify
	loaded, err := Load(path)
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	if loaded.SchemaVersion != CurrentSchemaVersion {
		t.Errorf("SchemaVersion = %d, want %d", loaded.SchemaVersion, CurrentSchemaVersion)
	}

	if loaded.Metadata.GeneratedBy != "test" {
		t.Errorf("GeneratedBy = %s, want test", loaded.Metadata.GeneratedBy)
	}

	module, ok := loaded.GetModule("example/module")
	if !ok {
		t.Fatal("Module not found after load")
	}

	if module.Version != "1.2.3" {
		t.Errorf("Version = %s, want 1.2.3", module.Version)
	}

	if len(module.Dependencies) != 1 {
		t.Errorf("Dependencies count = %d, want 1", len(module.Dependencies))
	}
}

func TestLoadNotFound(t *testing.T) {
	_, err := Load("/nonexistent/path/module.lock")
	if err == nil {
		t.Fatal("Expected error for nonexistent file")
	}

	if _, ok := err.(*LockFileNotFoundError); !ok {
		t.Errorf("Expected LockFileNotFoundError, got %T", err)
	}
}

func TestDiff(t *testing.T) {
	old := New()
	old.AddModule("unchanged/module", &LockedModule{Version: "1.0.0", Hash: "sha256:unchanged1234567890123456789012345678901234567890123456789012"})
	old.AddModule("changed/module", &LockedModule{Version: "1.0.0", Hash: "sha256:old12345678901234567890123456789012345678901234567890123456"})
	old.AddModule("removed/module", &LockedModule{Version: "1.0.0", Hash: "sha256:removed12345678901234567890123456789012345678901234567890"})

	new := New()
	new.AddModule("unchanged/module", &LockedModule{Version: "1.0.0", Hash: "sha256:unchanged1234567890123456789012345678901234567890123456789012"})
	new.AddModule("changed/module", &LockedModule{Version: "2.0.0", Hash: "sha256:new12345678901234567890123456789012345678901234567890123456"})
	new.AddModule("added/module", &LockedModule{Version: "1.0.0", Hash: "sha256:added12345678901234567890123456789012345678901234567890123"})

	diff := old.Diff(new)

	if len(diff.Added) != 1 {
		t.Errorf("Added count = %d, want 1", len(diff.Added))
	}
	if _, ok := diff.Added["added/module"]; !ok {
		t.Error("added/module should be in Added")
	}

	if len(diff.Removed) != 1 {
		t.Errorf("Removed count = %d, want 1", len(diff.Removed))
	}
	if _, ok := diff.Removed["removed/module"]; !ok {
		t.Error("removed/module should be in Removed")
	}

	if len(diff.Changed) != 1 {
		t.Errorf("Changed count = %d, want 1", len(diff.Changed))
	}
	change, ok := diff.Changed["changed/module"]
	if !ok {
		t.Fatal("changed/module should be in Changed")
	}
	if change.OldVersion != "1.0.0" || change.NewVersion != "2.0.0" {
		t.Errorf("Changed version = %s -> %s, want 1.0.0 -> 2.0.0",
			change.OldVersion, change.NewVersion)
	}

	if len(diff.Unchanged) != 1 {
		t.Errorf("Unchanged count = %d, want 1", len(diff.Unchanged))
	}

	if diff.IsEmpty() {
		t.Error("Diff should not be empty")
	}
}

func TestDiffEmpty(t *testing.T) {
	lf1 := New()
	lf1.AddModule("example/module", &LockedModule{Version: "1.0.0", Hash: "sha256:abc12345678901234567890123456789012345678901234567890123456"})

	lf2 := New()
	lf2.AddModule("example/module", &LockedModule{Version: "1.0.0", Hash: "sha256:abc12345678901234567890123456789012345678901234567890123456"})

	diff := lf1.Diff(lf2)

	if !diff.IsEmpty() {
		t.Error("Diff should be empty for identical lock files")
	}
}

func TestNeedsMigration(t *testing.T) {
	lf := New()
	if lf.NeedsMigration() {
		t.Error("New lock file should not need migration")
	}

	lf.SchemaVersion = 1
	if !lf.NeedsMigration() {
		t.Error("V1 lock file should need migration")
	}
}

func TestMigrate(t *testing.T) {
	// Create a v1 lock file
	lf := &LockFile{
		SchemaVersion: 1,
		Modules: map[string]*LockedModule{
			"example/module": {
				Version: "1.0.0",
				Hash:    "sha256:abc123def456789012345678901234567890123456789012345678901234",
			},
		},
	}

	result, err := lf.Migrate()
	if err != nil {
		t.Fatalf("Migrate failed: %v", err)
	}

	if result.FromVersion != 1 {
		t.Errorf("FromVersion = %d, want 1", result.FromVersion)
	}

	if result.ToVersion != CurrentSchemaVersion {
		t.Errorf("ToVersion = %d, want %d", result.ToVersion, CurrentSchemaVersion)
	}

	if lf.SchemaVersion != CurrentSchemaVersion {
		t.Errorf("SchemaVersion after migration = %d, want %d", lf.SchemaVersion, CurrentSchemaVersion)
	}

	if lf.Metadata == nil {
		t.Error("Metadata should be set after migration")
	}

	if lf.Checksums == nil {
		t.Error("Checksums should be set after migration")
	}

	// Verify checksum was populated
	key := "example/module@1.0.0"
	if _, ok := lf.Checksums.Entries[key]; !ok {
		t.Error("Checksum should be populated after migration")
	}
}

func TestMigrateAlreadyCurrent(t *testing.T) {
	lf := New()

	result, err := lf.Migrate()
	if err != nil {
		t.Fatalf("Migrate failed: %v", err)
	}

	if !result.Skipped {
		t.Error("Migration should be skipped for current version")
	}
}

func TestParseV1(t *testing.T) {
	data := []byte(`
schema_version: 1
modules:
  example/module:
    version: "1.0.0"
    hash: "sha256:abc123def456789012345678901234567890123456789012345678901234"
  another/module:
    version: "2.0.0"
    hash: "sha256:def456abc789012345678901234567890123456789012345678901234567"
`)

	lf, err := Parse(data)
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}

	if lf.SchemaVersion != 1 {
		t.Errorf("SchemaVersion = %d, want 1", lf.SchemaVersion)
	}

	if len(lf.Modules) != 2 {
		t.Errorf("Modules count = %d, want 2", len(lf.Modules))
	}

	module, ok := lf.GetModule("example/module")
	if !ok {
		t.Fatal("example/module not found")
	}

	if module.Version != "1.0.0" {
		t.Errorf("Version = %s, want 1.0.0", module.Version)
	}
}

func TestParseV2(t *testing.T) {
	data := []byte(`
schema_version: 2
metadata:
  generated_at: 2024-01-01T00:00:00Z
  generated_by: test
  keystone_version: "0.1.0"
modules:
  example/module:
    version: "1.0.0"
    hash: "sha256:abc123def456789012345678901234567890123456789012345678901234"
    source:
      type: registry
      url: https://registry.example.com
    dependencies:
      dep/one: "^1.0.0"
checksums:
  algorithm: sha256
  entries:
    example/module@1.0.0: "sha256:abc123def456789012345678901234567890123456789012345678901234"
`)

	lf, err := Parse(data)
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}

	if lf.SchemaVersion != 2 {
		t.Errorf("SchemaVersion = %d, want 2", lf.SchemaVersion)
	}

	if lf.Metadata == nil {
		t.Fatal("Metadata should not be nil")
	}

	if lf.Metadata.GeneratedBy != "test" {
		t.Errorf("GeneratedBy = %s, want test", lf.Metadata.GeneratedBy)
	}

	module, ok := lf.GetModule("example/module")
	if !ok {
		t.Fatal("example/module not found")
	}

	if module.Source == nil {
		t.Fatal("Source should not be nil")
	}

	if module.Source.Type != "registry" {
		t.Errorf("Source.Type = %s, want registry", module.Source.Type)
	}

	if len(module.Dependencies) != 1 {
		t.Errorf("Dependencies count = %d, want 1", len(module.Dependencies))
	}
}

func TestParseUnsupportedVersion(t *testing.T) {
	data := []byte(`
schema_version: 999
modules: {}
`)

	_, err := Parse(data)
	if err == nil {
		t.Fatal("Expected error for unsupported version")
	}

	if _, ok := err.(*UnsupportedSchemaError); !ok {
		t.Errorf("Expected UnsupportedSchemaError, got %T", err)
	}
}

func TestComputeHash(t *testing.T) {
	content := []byte("test content")
	hash := ComputeHash(content)

	if !isValidHash(hash) {
		t.Errorf("ComputeHash returned invalid hash: %s", hash)
	}

	// Same content should produce same hash
	hash2 := ComputeHash(content)
	if hash != hash2 {
		t.Error("ComputeHash should be deterministic")
	}

	// Different content should produce different hash
	hash3 := ComputeHash([]byte("different content"))
	if hash == hash3 {
		t.Error("Different content should produce different hash")
	}
}

func TestVerifyChecksums(t *testing.T) {
	lf := New()
	lf.AddModule("example/module", &LockedModule{
		Version: "1.0.0",
		Hash:    "sha256:abc123def456789012345678901234567890123456789012345678901234",
	})

	// Valid checksums
	mismatches := lf.VerifyChecksums()
	if len(mismatches) != 0 {
		t.Errorf("VerifyChecksums found %d mismatches, want 0", len(mismatches))
	}

	// Tamper with checksum
	lf.Checksums.Entries["example/module@1.0.0"] = "sha256:tampered"
	mismatches = lf.VerifyChecksums()
	if len(mismatches) != 1 {
		t.Errorf("VerifyChecksums found %d mismatches, want 1", len(mismatches))
	}
}

func TestGetMigrationPath(t *testing.T) {
	path := GetMigrationPath(1)

	if len(path) != CurrentSchemaVersion {
		t.Errorf("Migration path length = %d, want %d", len(path), CurrentSchemaVersion)
	}

	if path[0] != 1 {
		t.Errorf("First version = %d, want 1", path[0])
	}

	if path[len(path)-1] != CurrentSchemaVersion {
		t.Errorf("Last version = %d, want %d", path[len(path)-1], CurrentSchemaVersion)
	}
}

func TestDescribeMigration(t *testing.T) {
	changes, err := DescribeMigration(1)
	if err != nil {
		t.Fatalf("DescribeMigration failed: %v", err)
	}

	if len(changes) == 0 {
		t.Error("DescribeMigration should return changes")
	}
}

func TestMarshal(t *testing.T) {
	lf := New()
	lf.Metadata.GeneratedAt = time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	lf.Metadata.GeneratedBy = "test"
	lf.AddModule("example/module", &LockedModule{
		Version: "1.0.0",
		Hash:    "sha256:abc123def456789012345678901234567890123456789012345678901234",
	})

	data, err := lf.Marshal()
	if err != nil {
		t.Fatalf("Marshal failed: %v", err)
	}

	// Check header comment
	str := string(data)
	if len(str) == 0 {
		t.Fatal("Marshal returned empty data")
	}

	if str[0] != '#' {
		t.Error("Marshal should include header comment")
	}
}

func TestDiffSummary(t *testing.T) {
	diff := &LockFileDiff{
		Added:     map[string]*LockedModule{"a": {}},
		Removed:   map[string]*LockedModule{"b": {}},
		Changed:   map[string]*ModuleChange{"c": {}},
		Unchanged: []string{"d", "e"},
	}

	summary := diff.Summary()
	if summary == "" {
		t.Error("Summary should not be empty")
	}

	// Empty diff
	emptyDiff := &LockFileDiff{
		Added:     map[string]*LockedModule{},
		Removed:   map[string]*LockedModule{},
		Changed:   map[string]*ModuleChange{},
		Unchanged: []string{},
	}

	if emptyDiff.Summary() != "No changes" {
		t.Errorf("Empty diff summary = %s, want 'No changes'", emptyDiff.Summary())
	}
}
