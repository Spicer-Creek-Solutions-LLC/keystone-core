package resolver

import (
	"fmt"
	"testing"
	"time"

	"github.com/titananvil/titan-anvil/pkg/module/manifest"
)

// Mock registry client for testing
type mockRegistryClient struct {
	modules map[string]map[string]*ModuleInfo // moduleName -> version -> info
}

func newMockRegistry() *mockRegistryClient {
	return &mockRegistryClient{
		modules: make(map[string]map[string]*ModuleInfo),
	}
}

func (m *mockRegistryClient) addModule(name, version, hash string, deps map[string]string) {
	if m.modules[name] == nil {
		m.modules[name] = make(map[string]*ModuleInfo)
	}
	if deps == nil {
		deps = make(map[string]string)
	}
	m.modules[name][version] = &ModuleInfo{
		Name:         name,
		Version:      version,
		Hash:         hash,
		PublishedAt:  time.Now(),
		Dependencies: deps,
	}
}

func (m *mockRegistryClient) ListVersions(moduleName string) ([]string, error) {
	versions, ok := m.modules[moduleName]
	if !ok {
		return nil, fmt.Errorf("module %s not found", moduleName)
	}

	result := []string{}
	for v := range versions {
		result = append(result, v)
	}
	return result, nil
}

func (m *mockRegistryClient) GetModuleInfo(moduleName, version string) (*ModuleInfo, error) {
	versions, ok := m.modules[moduleName]
	if !ok {
		return nil, fmt.Errorf("module %s not found", moduleName)
	}

	info, ok := versions[version]
	if !ok {
		return nil, fmt.Errorf("version %s not found for module %s", version, moduleName)
	}

	return info, nil
}

func (m *mockRegistryClient) GetModuleManifest(moduleName, version string) (*manifest.Manifest, error) {
	info, err := m.GetModuleInfo(moduleName, version)
	if err != nil {
		return nil, err
	}

	return &manifest.Manifest{
		Version:      version,
		Name:         moduleName,
		Dependencies: info.Dependencies,
	}, nil
}

func (m *mockRegistryClient) DownloadModule(moduleName, version, destPath string) error {
	_, err := m.GetModuleInfo(moduleName, version)
	return err
}

func TestNewModuleResolver(t *testing.T) {
	tmpDir := t.TempDir()
	registry := newMockRegistry()

	resolver, err := NewModuleResolver(registry, tmpDir)
	if err != nil {
		t.Fatalf("NewModuleResolver failed: %v", err)
	}

	if resolver.Registry != registry {
		t.Error("Registry not set correctly")
	}

	if resolver.Cache == nil {
		t.Error("Cache not initialized")
	}

	if resolver.ConstraintParser == nil {
		t.Error("ConstraintParser not initialized")
	}

	if resolver.VersionSelector == nil {
		t.Error("VersionSelector not initialized")
	}

	if resolver.ConflictResolver == nil {
		t.Error("ConflictResolver not initialized")
	}
}

func TestResolveFromManifest_Simple(t *testing.T) {
	tmpDir := t.TempDir()
	registry := newMockRegistry()

	// Add modules to registry
	registry.addModule("myapp", "1.0.0", "hash-myapp-1.0.0", map[string]string{
		"std/files": "^1.0.0",
	})
	registry.addModule("std/files", "1.0.0", "hash-files-1.0.0", nil)
	registry.addModule("std/files", "1.1.0", "hash-files-1.1.0", nil)

	resolver, err := NewModuleResolver(registry, tmpDir)
	if err != nil {
		t.Fatalf("NewModuleResolver failed: %v", err)
	}

	// Create manifest
	m := &manifest.Manifest{
		Version:      "1.0.0",
		Name:         "myapp",
		Dependencies: map[string]string{
			"std/files": "^1.0.0",
		},
	}

	// Resolve
	result, err := resolver.ResolveFromManifest(m)
	if err != nil {
		t.Fatalf("Resolve failed: %v", err)
	}

	t.Logf("Resolution result: %d resolved nodes, %d warnings, %d errors",
		len(result.Resolved), len(result.Warnings), len(result.Errors))
	for _, w := range result.Warnings {
		t.Logf("Warning: %s", w)
	}

	if result.RootModule.Name != "myapp" {
		t.Errorf("Expected root module 'myapp', got '%s'", result.RootModule.Name)
	}

	if len(result.Resolved) == 0 {
		t.Error("Expected resolved dependencies, got none")
	}

	if result.LockFile == nil {
		t.Error("Expected lock file to be generated")
	}

	if result.Duration == 0 {
		t.Error("Expected duration to be recorded")
	}
}

func TestValidateLockFile(t *testing.T) {
	tmpDir := t.TempDir()
	registry := newMockRegistry()

	registry.addModule("std/files", "1.0.0", "hash-files-1.0.0", nil)

	resolver, err := NewModuleResolver(registry, tmpDir)
	if err != nil {
		t.Fatalf("NewModuleResolver failed: %v", err)
	}

	tests := []struct {
		name     string
		lockFile *manifest.LockFile
		wantErr  bool
	}{
		{
			name:     "nil lock file",
			lockFile: nil,
			wantErr:  true,
		},
		{
			name: "invalid version",
			lockFile: &manifest.LockFile{
				SchemaVersion: 99,
			},
			wantErr: true,
		},
		{
			name: "valid lock file",
			lockFile: &manifest.LockFile{
				SchemaVersion: 1,
				Modules: map[string]manifest.LockedModule{
					"std/files": {Version: "1.0.0", Hash: "hash-files-1.0.0"},
				},
			},
			wantErr: false,
		},
		{
			name: "module not found",
			lockFile: &manifest.LockFile{
				SchemaVersion: 1,
				Modules: map[string]manifest.LockedModule{
					"std/nonexistent": {Version: "1.0.0", Hash: "hash"},
				},
			},
			wantErr: true,
		},
		{
			name: "hash mismatch",
			lockFile: &manifest.LockFile{
				SchemaVersion: 1,
				Modules: map[string]manifest.LockedModule{
					"std/files": {Version: "1.0.0", Hash: "wrong-hash"},
				},
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := resolver.ValidateLockFile(tt.lockFile)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateLockFile() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestFilterPrereleases(t *testing.T) {
	versions := []string{
		"1.0.0",
		"1.1.0-alpha",
		"1.1.0-beta.1",
		"1.2.0",
		"2.0.0-rc.1",
	}

	filtered := filterPrereleases(versions)

	expected := []string{"1.0.0", "1.2.0"}
	if len(filtered) != len(expected) {
		t.Errorf("Expected %d versions, got %d", len(expected), len(filtered))
	}

	for i, v := range filtered {
		if v != expected[i] {
			t.Errorf("Expected version %s at index %d, got %s", expected[i], i, v)
		}
	}
}
