package manifest

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

const validManifestYAML = `
schemaVersion: 1
name: vendor/pkg_apt
version: v1.2.3

dependencies:
  - module: std/files
    version: ">=1.0 <2.0"
  - module: std/exec
    version: "^1.5.0"

capabilities:
  fs.read:
    allowed_paths:
      - /etc/apt/*.list
      - /var/lib/apt/lists/**
  fs.write:
    allowed_paths:
      - /etc/apt/sources.list.d/**
  exec:
    allowed_commands:
      - /usr/bin/apt-get
      - /usr/bin/dpkg

limits:
  time_ms: 5000
  mem_pages: 512
  cpu_shares: 100

starlark:
  entrypoints:
    check: "states/verify.star:check"
    apply: "states/apply.star:apply"

signatures:
  - keyid: "vendor-signing-key-2024"
    algorithm: cosign
    signature: "MEUCIQC..."
`

func TestParse_Valid(t *testing.T) {
	manifest, err := Parse([]byte(validManifestYAML))
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}

	// Check basic fields
	if manifest.Name != "vendor/pkg_apt" {
		t.Errorf("expected name 'vendor/pkg_apt', got %s", manifest.Name)
	}
	if manifest.Version != "v1.2.3" {
		t.Errorf("expected version 'v1.2.3', got %s", manifest.Version)
	}
	if manifest.SchemaVersion != 1 {
		t.Errorf("expected schema version 1, got %d", manifest.SchemaVersion)
	}

	// Check dependencies
	if len(manifest.Dependencies) != 2 {
		t.Fatalf("expected 2 dependencies, got %d", len(manifest.Dependencies))
	}
	if manifest.Dependencies[0].Module != "std/files" {
		t.Errorf("unexpected dependency: %s", manifest.Dependencies[0].Module)
	}

	// Check capabilities
	if manifest.Capabilities.FSRead == nil {
		t.Error("expected fs.read capability")
	}
	if manifest.Capabilities.FSWrite == nil {
		t.Error("expected fs.write capability")
	}
	if manifest.Capabilities.Exec == nil {
		t.Error("expected exec capability")
	}

	// Check resource limits
	if manifest.Limits.TimeMS != 5000 {
		t.Errorf("expected time_ms 5000, got %d", manifest.Limits.TimeMS)
	}

	// Check Starlark config
	if manifest.Starlark == nil {
		t.Fatal("expected Starlark config")
	}
	if len(manifest.Starlark.Entrypoints) != 2 {
		t.Errorf("expected 2 entrypoints, got %d", len(manifest.Starlark.Entrypoints))
	}

	// Check signatures
	if len(manifest.Signatures) != 1 {
		t.Fatalf("expected 1 signature, got %d", len(manifest.Signatures))
	}
	if manifest.Signatures[0].KeyID != "vendor-signing-key-2024" {
		t.Errorf("unexpected key ID: %s", manifest.Signatures[0].KeyID)
	}
}

func TestParse_MissingName(t *testing.T) {
	yaml := `
schemaVersion: 1
version: v1.0.0
starlark:
  entrypoints:
    check: "check.star:check"
`
	_, err := Parse([]byte(yaml))
	if err != ErrMissingName {
		t.Errorf("expected ErrMissingName, got %v", err)
	}
}

func TestParse_MissingVersion(t *testing.T) {
	yaml := `
schemaVersion: 1
name: test/module
starlark:
  entrypoints:
    check: "check.star:check"
`
	_, err := Parse([]byte(yaml))
	if err != ErrMissingVersion {
		t.Errorf("expected ErrMissingVersion, got %v", err)
	}
}

func TestParse_InvalidSchemaVersion(t *testing.T) {
	yaml := `
schemaVersion: 999
name: test/module
version: v1.0.0
starlark:
  entrypoints:
    check: "check.star:check"
`
	_, err := Parse([]byte(yaml))
	if err != ErrInvalidSchemaVersion {
		t.Errorf("expected ErrInvalidSchemaVersion, got %v", err)
	}
}

func TestParse_NoRuntime(t *testing.T) {
	yaml := `
schemaVersion: 1
name: test/module
version: v1.0.0
`
	_, err := Parse([]byte(yaml))
	if err != ErrNoRuntime {
		t.Errorf("expected ErrNoRuntime, got %v", err)
	}
}

func TestParse_InvalidYAML(t *testing.T) {
	yaml := `this is not valid YAML: [[[`
	_, err := Parse([]byte(yaml))
	if !strings.Contains(err.Error(), "invalid YAML") {
		t.Errorf("expected invalid YAML error, got %v", err)
	}
}

func TestParseFile(t *testing.T) {
	// Create a temporary file
	tmpDir := t.TempDir()
	filename := filepath.Join(tmpDir, "module.yaml")

	if err := os.WriteFile(filename, []byte(validManifestYAML), 0644); err != nil {
		t.Fatalf("failed to write temp file: %v", err)
	}

	manifest, err := ParseFile(filename)
	if err != nil {
		t.Fatalf("ParseFile failed: %v", err)
	}

	if manifest.Name != "vendor/pkg_apt" {
		t.Errorf("expected name 'vendor/pkg_apt', got %s", manifest.Name)
	}
}

func TestMarshal(t *testing.T) {
	manifest := &Manifest{
		SchemaVersion: 1,
		Name:          "test/module",
		Version:       "v1.0.0",
		Starlark: &StarlarkConfig{
			Entrypoints: map[string]string{
				"check": "check.star:check",
			},
		},
	}

	data, err := Marshal(manifest)
	if err != nil {
		t.Fatalf("Marshal failed: %v", err)
	}

	// Parse it back
	parsed, err := Parse(data)
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}

	if parsed.Name != manifest.Name {
		t.Errorf("round-trip failed: name mismatch")
	}
}

func TestWriteFile(t *testing.T) {
	tmpDir := t.TempDir()
	filename := filepath.Join(tmpDir, "module.yaml")

	manifest := &Manifest{
		SchemaVersion: 1,
		Name:          "test/module",
		Version:       "v1.0.0",
		Starlark: &StarlarkConfig{
			Entrypoints: map[string]string{
				"check": "check.star:check",
			},
		},
	}

	if err := WriteFile(filename, manifest); err != nil {
		t.Fatalf("WriteFile failed: %v", err)
	}

	// Read it back
	parsed, err := ParseFile(filename)
	if err != nil {
		t.Fatalf("ParseFile failed: %v", err)
	}

	if parsed.Name != manifest.Name {
		t.Errorf("round-trip failed: name mismatch")
	}
}

func TestParseLockFile(t *testing.T) {
	lockYAML := `
schemaVersion: 1
modules:
  - name: vendor/pkg_apt
    version: v1.2.3
    hash: sha256:abc123
    resolved: 2024-01-15T10:30:00Z
  - name: std/files
    version: v1.4.2
    hash: sha256:def456
    resolved: 2024-01-15T10:30:00Z
`

	lockfile, err := ParseLockFile([]byte(lockYAML))
	if err != nil {
		t.Fatalf("ParseLockFile failed: %v", err)
	}

	if len(lockfile.Modules) != 2 {
		t.Fatalf("expected 2 modules, got %d", len(lockfile.Modules))
	}

	if lockfile.Modules[0].Name != "vendor/pkg_apt" {
		t.Errorf("unexpected module name: %s", lockfile.Modules[0].Name)
	}
	if lockfile.Modules[0].Hash != "sha256:abc123" {
		t.Errorf("unexpected hash: %s", lockfile.Modules[0].Hash)
	}
}

func TestMarshalLockFile(t *testing.T) {
	lockfile := &LockFile{
		SchemaVersion: 1,
		Modules: []LockedModule{
			{
				Name:     "test/module",
				Version:  "v1.0.0",
				Hash:     "sha256:test123",
				Resolved: time.Now(),
			},
		},
	}

	data, err := MarshalLockFile(lockfile)
	if err != nil {
		t.Fatalf("MarshalLockFile failed: %v", err)
	}

	// Parse it back
	parsed, err := ParseLockFile(data)
	if err != nil {
		t.Fatalf("ParseLockFile failed: %v", err)
	}

	if len(parsed.Modules) != 1 {
		t.Fatalf("expected 1 module, got %d", len(parsed.Modules))
	}
	if parsed.Modules[0].Name != "test/module" {
		t.Errorf("round-trip failed: name mismatch")
	}
}

func TestGetDependencyNames(t *testing.T) {
	manifest, _ := Parse([]byte(validManifestYAML))

	names := manifest.GetDependencyNames()
	if len(names) != 2 {
		t.Fatalf("expected 2 names, got %d", len(names))
	}

	if names[0] != "std/files" || names[1] != "std/exec" {
		t.Errorf("unexpected dependency names: %v", names)
	}
}

func TestHasCapability(t *testing.T) {
	caps := Capabilities{
		FSRead: &FSCapability{},
		Exec:   &ExecCapability{},
		Time:   true,
	}

	tests := []struct {
		name     string
		expected bool
	}{
		{"fs.read", true},
		{"fs.write", false},
		{"exec", true},
		{"time", true},
		{"kv", false},
		{"nonexistent", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := caps.HasCapability(tt.name)
			if result != tt.expected {
				t.Errorf("HasCapability(%s) = %v, expected %v", tt.name, result, tt.expected)
			}
		})
	}
}

func TestListCapabilities(t *testing.T) {
	caps := Capabilities{
		FSRead:  &FSCapability{},
		HTTPGet: &HTTPCapability{},
		Log:     &LogCapability{},
	}

	list := caps.ListCapabilities()
	if len(list) != 3 {
		t.Fatalf("expected 3 capabilities, got %d", len(list))
	}

	// Check that all expected capabilities are present
	expected := map[string]bool{
		"fs.read":  false,
		"http.get": false,
		"log":      false,
	}

	for _, cap := range list {
		if _, ok := expected[cap]; ok {
			expected[cap] = true
		}
	}

	for cap, found := range expected {
		if !found {
			t.Errorf("capability %s not found in list", cap)
		}
	}
}

func TestWASMConfig(t *testing.T) {
	yaml := `
schemaVersion: 1
name: test/wasm-module
version: v1.0.0

wasm:
  binary: "executor.wasm"
  exports:
    - check
    - apply
    - rollback
`

	manifest, err := Parse([]byte(yaml))
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}

	if manifest.WASM == nil {
		t.Fatal("expected WASM config")
	}
	if manifest.WASM.Binary != "executor.wasm" {
		t.Errorf("unexpected binary: %s", manifest.WASM.Binary)
	}
	if len(manifest.WASM.Exports) != 3 {
		t.Errorf("expected 3 exports, got %d", len(manifest.WASM.Exports))
	}
}
