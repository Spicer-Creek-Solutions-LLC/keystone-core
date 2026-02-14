package registry

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/shawnbutts/keystone-core/pkg/module/manifest"
	"github.com/shawnbutts/keystone-core/pkg/module/resolver"
)

// mockRegistryClient implements resolver.RegistryClient for testing.
type mockRegistryClient struct {
	modules map[string]map[string][]byte // moduleName -> version -> zipData
}

func (m *mockRegistryClient) ListVersions(moduleName string) ([]string, error) {
	versions, ok := m.modules[moduleName]
	if !ok {
		return nil, fmt.Errorf("module not found: %s", moduleName)
	}
	var result []string
	for v := range versions {
		result = append(result, v)
	}
	return result, nil
}

func (m *mockRegistryClient) GetModuleInfo(moduleName, version string) (*resolver.ModuleInfo, error) {
	versions, ok := m.modules[moduleName]
	if !ok {
		return nil, fmt.Errorf("module not found: %s", moduleName)
	}
	data, ok := versions[version]
	if !ok {
		return nil, fmt.Errorf("version not found: %s@%s", moduleName, version)
	}
	return &resolver.ModuleInfo{
		Name:    moduleName,
		Version: version,
		Size:    int64(len(data)),
	}, nil
}

func (m *mockRegistryClient) GetModuleManifest(moduleName, version string) (*manifest.Manifest, error) {
	return &manifest.Manifest{Name: moduleName, Version: version}, nil
}

func (m *mockRegistryClient) DownloadModule(moduleName, version, destPath string) error {
	versions, ok := m.modules[moduleName]
	if !ok {
		return fmt.Errorf("module not found: %s", moduleName)
	}
	data, ok := versions[version]
	if !ok {
		return fmt.Errorf("version not found: %s@%s", moduleName, version)
	}
	return os.WriteFile(destPath, data, 0o644)
}

func TestExport(t *testing.T) {
	client := &mockRegistryClient{
		modules: map[string]map[string][]byte{
			"test/alpha": {
				"1.0.0": []byte("PK\x03\x04alpha-v1"),
				"2.0.0": []byte("PK\x03\x04alpha-v2"),
			},
			"test/beta": {
				"1.0.0": []byte("PK\x03\x04beta-v1"),
			},
		},
	}

	outputDir := filepath.Join(t.TempDir(), "export")

	result, err := Export(context.Background(), ExportConfig{
		Modules:   []string{"test/alpha", "test/beta"},
		OutputDir: outputDir,
		Client:    client,
	})
	if err != nil {
		t.Fatalf("Export: %v", err)
	}

	if result.ModulesExported != 2 {
		t.Errorf("ModulesExported = %d, want 2", result.ModulesExported)
	}
	if result.VersionsExported != 3 {
		t.Errorf("VersionsExported = %d, want 3", result.VersionsExported)
	}
	if result.TotalSize == 0 {
		t.Error("TotalSize should be > 0")
	}

	// Verify files exist in FilesystemBackend layout
	for _, path := range []string{
		"test/alpha/1.0.0/module.zip",
		"test/alpha/2.0.0/module.zip",
		"test/beta/1.0.0/module.zip",
	} {
		full := filepath.Join(outputDir, path)
		if _, err := os.Stat(full); err != nil {
			t.Errorf("expected %s to exist: %v", path, err)
		}
	}
}

func TestExport_NilClient(t *testing.T) {
	_, err := Export(context.Background(), ExportConfig{
		Modules:   []string{"test/mod"},
		OutputDir: t.TempDir(),
	})
	if err == nil {
		t.Fatal("expected error for nil client")
	}
}

func TestExport_EmptyOutputDir(t *testing.T) {
	_, err := Export(context.Background(), ExportConfig{
		Modules: []string{"test/mod"},
		Client:  &mockRegistryClient{},
	})
	if err == nil {
		t.Fatal("expected error for empty output dir")
	}
}

func TestExport_ModuleNotFound(t *testing.T) {
	client := &mockRegistryClient{
		modules: map[string]map[string][]byte{},
	}

	result, err := Export(context.Background(), ExportConfig{
		Modules:   []string{"nonexistent/mod"},
		OutputDir: t.TempDir(),
		Client:    client,
	})
	if err != nil {
		t.Fatalf("Export: %v", err)
	}
	if len(result.Errors) != 1 {
		t.Errorf("expected 1 error, got %d", len(result.Errors))
	}
}

func TestExport_CancelledContext(t *testing.T) {
	client := &mockRegistryClient{
		modules: map[string]map[string][]byte{
			"test/mod": {"1.0.0": []byte("data")},
		},
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	_, err := Export(ctx, ExportConfig{
		Modules:   []string{"test/mod"},
		OutputDir: t.TempDir(),
		Client:    client,
	})
	if err == nil {
		t.Fatal("expected error for cancelled context")
	}
}

func TestExport_ThenImport(t *testing.T) {
	client := &mockRegistryClient{
		modules: map[string]map[string][]byte{
			"test/mod": {
				"1.0.0": []byte("PK\x03\x04data"),
			},
		},
	}

	exportDir := filepath.Join(t.TempDir(), "export")
	_, err := Export(context.Background(), ExportConfig{
		Modules:   []string{"test/mod"},
		OutputDir: exportDir,
		Client:    client,
	})
	if err != nil {
		t.Fatalf("Export: %v", err)
	}

	// Import into registry
	root := filepath.Join(t.TempDir(), "registry")
	reg, err := Init(Config{RootDir: root})
	if err != nil {
		t.Fatal(err)
	}
	defer reg.Close()

	result, err := reg.ImportModulesFromDir(context.Background(), exportDir)
	if err != nil {
		t.Fatalf("Import: %v", err)
	}
	if result.ModulesImported != 1 {
		t.Errorf("ModulesImported = %d, want 1", result.ModulesImported)
	}

	// Verify via LocalClient
	lc := NewLocalClient(reg)
	versions, err := lc.ListVersions("test/mod")
	if err != nil {
		t.Fatalf("ListVersions: %v", err)
	}
	if len(versions) != 1 || versions[0] != "1.0.0" {
		t.Errorf("versions = %v, want [1.0.0]", versions)
	}
}
