package upgrade

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/shawnbutts/keystone-core/internal/airgap/bootstrap"
)

func validManifest() *Manifest {
	return &Manifest{
		SchemaVersion: "1.0",
		FromVersion:   "1.0.0",
		ToVersion:     "1.1.0",
		Platform:      bootstrap.Platform{OS: "linux", Arch: "amd64"},
		Created:       time.Now().UTC(),
		Components: []bootstrap.ComponentEntry{
			{Name: "kscore-server", Version: "1.1.0", Path: "bin/kscore-server", SHA256: "a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2"},
		},
	}
}

func TestManifest_Validate(t *testing.T) {
	m := validManifest()
	if err := m.Validate(); err != nil {
		t.Fatalf("valid manifest failed: %v", err)
	}
}

func TestManifest_ValidateRequired(t *testing.T) {
	tests := []struct {
		name   string
		modify func(*Manifest)
	}{
		{"missing schema_version", func(m *Manifest) { m.SchemaVersion = "" }},
		{"missing from_version", func(m *Manifest) { m.FromVersion = "" }},
		{"missing to_version", func(m *Manifest) { m.ToVersion = "" }},
		{"missing platform os", func(m *Manifest) { m.Platform.OS = "" }},
		{"no components", func(m *Manifest) { m.Components = nil }},
		{"component missing name", func(m *Manifest) { m.Components[0].Name = "" }},
		{"component missing path", func(m *Manifest) { m.Components[0].Path = "" }},
		{"duplicate component", func(m *Manifest) {
			m.Components = append(m.Components, m.Components[0])
		}},
		{"absolute component path", func(m *Manifest) { m.Components[0].Path = "/bin/server" }},
		{"invalid sha256", func(m *Manifest) { m.Components[0].SHA256 = "notahash" }},
		{"invalid checksum", func(m *Manifest) { m.Checksum = "bad" }},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := validManifest()
			tt.modify(m)
			if err := m.Validate(); err == nil {
				t.Error("expected validation error")
			}
		})
	}
}

func TestManifest_ValidateMigrations(t *testing.T) {
	m := validManifest()
	m.Migrations = []MigrationEntry{
		{Name: "", Path: "migrations/001.sql"},
	}
	if err := m.Validate(); err == nil {
		t.Error("expected error for migration without name")
	}

	m = validManifest()
	m.Migrations = []MigrationEntry{
		{Name: "001", Path: ""},
	}
	if err := m.Validate(); err == nil {
		t.Error("expected error for migration without path")
	}
}

func TestManifest_ValidateScripts(t *testing.T) {
	m := validManifest()
	m.PreScripts = []ScriptEntry{{Path: ""}}
	if err := m.Validate(); err == nil {
		t.Error("expected error for pre-script without path")
	}

	m = validManifest()
	m.PostScripts = []ScriptEntry{{Path: "/absolute/path.sh"}}
	if err := m.Validate(); err == nil {
		t.Error("expected error for absolute post-script path")
	}
}

func TestWriteReadManifest(t *testing.T) {
	m := validManifest()
	m.BreakingChanges = []string{"API v1 removed"}
	m.Migrations = []MigrationEntry{
		{Name: "001-add-column", Path: "migrations/001.sql", Order: 1},
	}
	m.ConfigChanges = []ConfigChange{
		{Key: "server.port", OldDefault: "8080", NewDefault: "9090", Breaking: true},
	}

	dir := t.TempDir()
	path := filepath.Join(dir, "manifest.json")

	if err := WriteManifest(m, path); err != nil {
		t.Fatalf("WriteManifest: %v", err)
	}

	loaded, err := ReadManifest(path)
	if err != nil {
		t.Fatalf("ReadManifest: %v", err)
	}

	if loaded.FromVersion != "1.0.0" {
		t.Errorf("FromVersion = %q, want 1.0.0", loaded.FromVersion)
	}
	if loaded.ToVersion != "1.1.0" {
		t.Errorf("ToVersion = %q, want 1.1.0", loaded.ToVersion)
	}
	if len(loaded.BreakingChanges) != 1 {
		t.Errorf("BreakingChanges count = %d, want 1", len(loaded.BreakingChanges))
	}
	if len(loaded.Migrations) != 1 || loaded.Migrations[0].Name != "001-add-column" {
		t.Errorf("unexpected migrations: %v", loaded.Migrations)
	}
	if len(loaded.ConfigChanges) != 1 || !loaded.ConfigChanges[0].Breaking {
		t.Errorf("unexpected config changes: %v", loaded.ConfigChanges)
	}
}

func TestReadManifest_NotFound(t *testing.T) {
	_, err := ReadManifest(filepath.Join(t.TempDir(), "missing.json"))
	if err == nil {
		t.Error("expected error for missing file")
	}
}

func TestReadManifest_InvalidJSON(t *testing.T) {
	path := filepath.Join(t.TempDir(), "bad.json")
	os.WriteFile(path, []byte("{bad json"), 0o644)
	_, err := ReadManifest(path)
	if err == nil {
		t.Error("expected error for invalid JSON")
	}
}

func TestValidatePath_Traversal(t *testing.T) {
	if err := validatePath("../escape"); err == nil {
		t.Error("expected error for path traversal")
	}
	if err := validatePath(".."); err == nil {
		t.Error("expected error for bare ..")
	}
	if err := validatePath("bin/server"); err != nil {
		t.Errorf("valid relative path should pass: %v", err)
	}
}
