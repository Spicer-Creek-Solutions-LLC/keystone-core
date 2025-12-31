package manifest

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

const validManifestYAML = `
name: vendor/pkg_apt
version: 1.2.3
type: starlark
entrypoint: main.star
description: APT package management module
author: Vendor Inc
license: MIT

capabilities:
  - fs.read
  - fs.write
  - exec

dependencies:
  std/files: ">=1.0.0 <2.0.0"
  std/exec: "^1.5.0"

limits:
  timeout: 5s
  memory: 128MB
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
	if manifest.Version != "1.2.3" {
		t.Errorf("expected version '1.2.3', got %s", manifest.Version)
	}
	if manifest.Type != "starlark" {
		t.Errorf("expected type 'starlark', got %s", manifest.Type)
	}
	if manifest.Entrypoint != "main.star" {
		t.Errorf("expected entrypoint 'main.star', got %s", manifest.Entrypoint)
	}
	if manifest.Description != "APT package management module" {
		t.Errorf("unexpected description: %s", manifest.Description)
	}
	if manifest.Author != "Vendor Inc" {
		t.Errorf("unexpected author: %s", manifest.Author)
	}
	if manifest.License != "MIT" {
		t.Errorf("unexpected license: %s", manifest.License)
	}

	// Check capabilities
	if len(manifest.Capabilities) != 3 {
		t.Fatalf("expected 3 capabilities, got %d", len(manifest.Capabilities))
	}
	expectedCaps := []string{"fs.read", "fs.write", "exec"}
	for i, cap := range expectedCaps {
		if manifest.Capabilities[i] != cap {
			t.Errorf("expected capability %s at index %d, got %s", cap, i, manifest.Capabilities[i])
		}
	}

	// Check dependencies
	if len(manifest.Dependencies) != 2 {
		t.Fatalf("expected 2 dependencies, got %d", len(manifest.Dependencies))
	}
	if manifest.Dependencies["std/files"] != ">=1.0.0 <2.0.0" {
		t.Errorf("unexpected std/files constraint: %s", manifest.Dependencies["std/files"])
	}
	if manifest.Dependencies["std/exec"] != "^1.5.0" {
		t.Errorf("unexpected std/exec constraint: %s", manifest.Dependencies["std/exec"])
	}

	// Check limits
	if manifest.Limits.Timeout != 5*time.Second {
		t.Errorf("expected timeout 5s, got %v", manifest.Limits.Timeout)
	}
	if manifest.Limits.Memory != "128MB" {
		t.Errorf("expected memory '128MB', got %s", manifest.Limits.Memory)
	}
}

func TestParse_MissingName(t *testing.T) {
	yaml := `
version: 1.0.0
type: starlark
entrypoint: main.star
`
	_, err := Parse([]byte(yaml))
	if err == nil {
		t.Error("expected error for missing name")
	}
}

func TestParse_MissingVersion(t *testing.T) {
	yaml := `
name: test/module
type: starlark
entrypoint: main.star
`
	_, err := Parse([]byte(yaml))
	if err == nil {
		t.Error("expected error for missing version")
	}
}

func TestParse_InvalidType(t *testing.T) {
	yaml := `
name: test/module
version: 1.0.0
type: invalid
entrypoint: main.star
`
	_, err := Parse([]byte(yaml))
	if err == nil {
		t.Error("expected error for invalid type")
	}
}

func TestParse_InvalidYAML(t *testing.T) {
	yaml := `this is not valid YAML: [[[`
	_, err := Parse([]byte(yaml))
	if err == nil {
		t.Error("expected error for invalid YAML")
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

func TestParseFile_NotFound(t *testing.T) {
	_, err := ParseFile("/nonexistent/path/module.yaml")
	if err == nil {
		t.Error("expected error for nonexistent file")
	}
}

func TestMarshal(t *testing.T) {
	manifest := &Manifest{
		Name:       "test/module",
		Version:    "1.0.0",
		Type:       "starlark",
		Entrypoint: "main.star",
		Capabilities: []string{"fs.read"},
		Dependencies: map[string]string{
			"std/files": "^1.0.0",
		},
		Limits: Limits{
			Timeout: 10 * time.Second,
			Memory:  "64MB",
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
	if parsed.Version != manifest.Version {
		t.Errorf("round-trip failed: version mismatch")
	}
	if parsed.Type != manifest.Type {
		t.Errorf("round-trip failed: type mismatch")
	}
}

func TestWriteFile(t *testing.T) {
	tmpDir := t.TempDir()
	filename := filepath.Join(tmpDir, "module.yaml")

	manifest := &Manifest{
		Name:       "test/module",
		Version:    "1.0.0",
		Type:       "wasm",
		Entrypoint: "module.wasm",
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
	if parsed.Type != manifest.Type {
		t.Errorf("round-trip failed: type mismatch")
	}
}

func TestParseLockFile(t *testing.T) {
	lockYAML := `
schema_version: 1
modules:
  vendor/pkg_apt:
    version: 1.2.3
    hash: sha256:abc123def456
  std/files:
    version: 1.4.2
    hash: sha256:789xyz000111
`

	lockfile, err := ParseLockFile([]byte(lockYAML))
	if err != nil {
		t.Fatalf("ParseLockFile failed: %v", err)
	}

	if lockfile.SchemaVersion != 1 {
		t.Errorf("expected schema version 1, got %d", lockfile.SchemaVersion)
	}

	if len(lockfile.Modules) != 2 {
		t.Fatalf("expected 2 modules, got %d", len(lockfile.Modules))
	}

	aptModule, ok := lockfile.Modules["vendor/pkg_apt"]
	if !ok {
		t.Fatal("vendor/pkg_apt not found in lockfile")
	}
	if aptModule.Version != "1.2.3" {
		t.Errorf("unexpected version: %s", aptModule.Version)
	}
	if aptModule.Hash != "sha256:abc123def456" {
		t.Errorf("unexpected hash: %s", aptModule.Hash)
	}

	filesModule, ok := lockfile.Modules["std/files"]
	if !ok {
		t.Fatal("std/files not found in lockfile")
	}
	if filesModule.Version != "1.4.2" {
		t.Errorf("unexpected version: %s", filesModule.Version)
	}
}

func TestParseLockFile_InvalidSchemaVersion(t *testing.T) {
	lockYAML := `
schema_version: 999
modules:
  test/module:
    version: 1.0.0
    hash: sha256:abc123
`
	_, err := ParseLockFile([]byte(lockYAML))
	if err != ErrInvalidSchemaVersion {
		t.Errorf("expected ErrInvalidSchemaVersion, got %v", err)
	}
}

func TestMarshalLockFile(t *testing.T) {
	lockfile := &LockFile{
		SchemaVersion: 1,
		Modules: map[string]LockedModule{
			"test/module": {
				Version: "1.0.0",
				Hash:    "sha256:test123",
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

	if parsed.SchemaVersion != 1 {
		t.Errorf("expected schema version 1, got %d", parsed.SchemaVersion)
	}
	if len(parsed.Modules) != 1 {
		t.Fatalf("expected 1 module, got %d", len(parsed.Modules))
	}
	if mod, ok := parsed.Modules["test/module"]; !ok {
		t.Error("test/module not found in parsed lockfile")
	} else if mod.Version != "1.0.0" {
		t.Errorf("unexpected version: %s", mod.Version)
	}
}

func TestManifest_Validate(t *testing.T) {
	tests := []struct {
		name      string
		manifest  *Manifest
		expectErr bool
	}{
		{
			name: "valid starlark",
			manifest: &Manifest{
				Name:       "test/module",
				Version:    "1.0.0",
				Type:       "starlark",
				Entrypoint: "main.star",
			},
			expectErr: false,
		},
		{
			name: "valid wasm",
			manifest: &Manifest{
				Name:       "test/module",
				Version:    "1.0.0",
				Type:       "wasm",
				Entrypoint: "module.wasm",
			},
			expectErr: false,
		},
		{
			name: "missing name",
			manifest: &Manifest{
				Version:    "1.0.0",
				Type:       "starlark",
				Entrypoint: "main.star",
			},
			expectErr: true,
		},
		{
			name: "missing version",
			manifest: &Manifest{
				Name:       "test/module",
				Type:       "starlark",
				Entrypoint: "main.star",
			},
			expectErr: true,
		},
		{
			name: "invalid type",
			manifest: &Manifest{
				Name:       "test/module",
				Version:    "1.0.0",
				Type:       "python",
				Entrypoint: "main.py",
			},
			expectErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.manifest.Validate()
			if tt.expectErr && err == nil {
				t.Error("expected error but got none")
			}
			if !tt.expectErr && err != nil {
				t.Errorf("unexpected error: %v", err)
			}
		})
	}
}

func TestLoadManifest(t *testing.T) {
	tmpDir := t.TempDir()
	filename := filepath.Join(tmpDir, "module.yaml")

	if err := os.WriteFile(filename, []byte(validManifestYAML), 0644); err != nil {
		t.Fatalf("failed to write temp file: %v", err)
	}

	manifest, err := LoadManifest(filename)
	if err != nil {
		t.Fatalf("LoadManifest failed: %v", err)
	}

	if manifest.Name != "vendor/pkg_apt" {
		t.Errorf("expected name 'vendor/pkg_apt', got %s", manifest.Name)
	}
}

func TestManifest_ToYAML(t *testing.T) {
	manifest := &Manifest{
		Name:       "test/module",
		Version:    "1.0.0",
		Type:       "starlark",
		Entrypoint: "main.star",
	}

	data, err := manifest.ToYAML()
	if err != nil {
		t.Fatalf("ToYAML failed: %v", err)
	}

	if len(data) == 0 {
		t.Error("ToYAML returned empty data")
	}
}

func TestManifest_SaveManifest(t *testing.T) {
	tmpDir := t.TempDir()
	filename := filepath.Join(tmpDir, "output.yaml")

	manifest := &Manifest{
		Name:       "test/module",
		Version:    "1.0.0",
		Type:       "starlark",
		Entrypoint: "main.star",
	}

	if err := manifest.SaveManifest(filename); err != nil {
		t.Fatalf("SaveManifest failed: %v", err)
	}

	// Verify file exists
	if _, err := os.Stat(filename); err != nil {
		t.Errorf("file not created: %v", err)
	}

	// Load it back
	loaded, err := LoadManifest(filename)
	if err != nil {
		t.Fatalf("LoadManifest failed: %v", err)
	}

	if loaded.Name != manifest.Name {
		t.Errorf("round-trip failed: name mismatch")
	}
}

func TestMinimalManifest(t *testing.T) {
	yaml := `
name: test/minimal
version: 0.1.0
type: starlark
`
	manifest, err := Parse([]byte(yaml))
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}

	if manifest.Name != "test/minimal" {
		t.Errorf("unexpected name: %s", manifest.Name)
	}
	if len(manifest.Capabilities) != 0 {
		t.Errorf("expected no capabilities, got %d", len(manifest.Capabilities))
	}
	if len(manifest.Dependencies) != 0 {
		t.Errorf("expected no dependencies, got %d", len(manifest.Dependencies))
	}
}

func TestManifestWithAllFields(t *testing.T) {
	yaml := `
name: full/module
version: 2.0.0
type: wasm
entrypoint: executor.wasm
description: A full-featured module
author: Test Author <test@example.com>
license: Apache-2.0

capabilities:
  - fs.read
  - fs.write
  - http.get
  - http.post
  - exec
  - log
  - kv

dependencies:
  std/files: "^1.0.0"
  std/http: ">=2.0.0"
  vendor/utils: "~1.5.0"

limits:
  timeout: 30s
  memory: 256MB
  cpu: 0.5
`
	manifest, err := Parse([]byte(yaml))
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}

	if manifest.Name != "full/module" {
		t.Errorf("unexpected name: %s", manifest.Name)
	}
	if len(manifest.Capabilities) != 7 {
		t.Errorf("expected 7 capabilities, got %d", len(manifest.Capabilities))
	}
	if len(manifest.Dependencies) != 3 {
		t.Errorf("expected 3 dependencies, got %d", len(manifest.Dependencies))
	}
	if manifest.Limits.Timeout != 30*time.Second {
		t.Errorf("expected timeout 30s, got %v", manifest.Limits.Timeout)
	}
	if manifest.Limits.CPU != 0.5 {
		t.Errorf("expected CPU 0.5, got %f", manifest.Limits.CPU)
	}
}
