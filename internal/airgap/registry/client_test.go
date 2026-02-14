package registry

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/shawnbutts/keystone-core/internal/registry/storage"
	"github.com/shawnbutts/keystone-core/pkg/module/resolver"
)

var _ resolver.RegistryClient = (*LocalClient)(nil)

func setupRegistryWithModule(t *testing.T) (*Registry, string) {
	t.Helper()
	root := filepath.Join(t.TempDir(), "registry")
	reg, err := Init(Config{RootDir: root})
	if err != nil {
		t.Fatalf("Init: %v", err)
	}

	manifest := []byte("name: test/module\nversion: 1.0.0\ntype: starlark\nentrypoint: main.star\n")
	zipData := []byte("PK\x03\x04fake-zip-data")

	_, err = reg.Backend().Publish(context.Background(), &storage.PublishRequest{
		ModuleName:   "test/module",
		Version:      "1.0.0",
		ZipData:      bytes.NewReader(zipData),
		Manifest:     manifest,
		Hash:         "abc123",
		Description:  "A test module",
		Dependencies: map[string]string{"std/files": ">=1.0.0"},
		Tags:         []string{"test", "example"},
	})
	if err != nil {
		t.Fatalf("Publish: %v", err)
	}

	return reg, root
}

func TestLocalClient_ListVersions(t *testing.T) {
	reg, _ := setupRegistryWithModule(t)
	defer reg.Close()

	client := NewLocalClient(reg)
	versions, err := client.ListVersions("test/module")
	if err != nil {
		t.Fatalf("ListVersions: %v", err)
	}
	if len(versions) != 1 || versions[0] != "1.0.0" {
		t.Errorf("versions = %v, want [1.0.0]", versions)
	}
}

func TestLocalClient_ListVersions_NotFound(t *testing.T) {
	reg, _ := setupRegistryWithModule(t)
	defer reg.Close()

	client := NewLocalClient(reg)
	_, err := client.ListVersions("nonexistent/module")
	if err == nil {
		t.Fatal("expected error for nonexistent module")
	}
}

func TestLocalClient_GetModuleInfo(t *testing.T) {
	reg, _ := setupRegistryWithModule(t)
	defer reg.Close()

	client := NewLocalClient(reg)
	info, err := client.GetModuleInfo("test/module", "1.0.0")
	if err != nil {
		t.Fatalf("GetModuleInfo: %v", err)
	}
	if info.Name != "test/module" {
		t.Errorf("Name = %q, want %q", info.Name, "test/module")
	}
	if info.Version != "1.0.0" {
		t.Errorf("Version = %q, want %q", info.Version, "1.0.0")
	}
	if info.Hash != "abc123" {
		t.Errorf("Hash = %q, want %q", info.Hash, "abc123")
	}
	if info.Description != "A test module" {
		t.Errorf("Description = %q, want %q", info.Description, "A test module")
	}
	if len(info.Dependencies) != 1 {
		t.Errorf("Dependencies count = %d, want 1", len(info.Dependencies))
	}
}

func TestLocalClient_GetModuleInfo_NotFound(t *testing.T) {
	reg, _ := setupRegistryWithModule(t)
	defer reg.Close()

	client := NewLocalClient(reg)
	_, err := client.GetModuleInfo("test/module", "9.9.9")
	if err == nil {
		t.Fatal("expected error for nonexistent version")
	}
}

func TestLocalClient_GetModuleManifest(t *testing.T) {
	reg, _ := setupRegistryWithModule(t)
	defer reg.Close()

	client := NewLocalClient(reg)
	m, err := client.GetModuleManifest("test/module", "1.0.0")
	if err != nil {
		t.Fatalf("GetModuleManifest: %v", err)
	}
	if m.Name != "test/module" {
		t.Errorf("manifest Name = %q, want %q", m.Name, "test/module")
	}
	if m.Version != "1.0.0" {
		t.Errorf("manifest Version = %q, want %q", m.Version, "1.0.0")
	}
}

func TestLocalClient_DownloadModule(t *testing.T) {
	reg, _ := setupRegistryWithModule(t)
	defer reg.Close()

	client := NewLocalClient(reg)
	destPath := filepath.Join(t.TempDir(), "downloaded.zip")

	if err := client.DownloadModule("test/module", "1.0.0", destPath); err != nil {
		t.Fatalf("DownloadModule: %v", err)
	}

	data, err := os.ReadFile(destPath)
	if err != nil {
		t.Fatalf("read downloaded: %v", err)
	}
	if !bytes.HasPrefix(data, []byte("PK\x03\x04")) {
		t.Error("downloaded data doesn't start with zip magic bytes")
	}
}

func TestLocalClient_DownloadModule_NotFound(t *testing.T) {
	reg, _ := setupRegistryWithModule(t)
	defer reg.Close()

	client := NewLocalClient(reg)
	destPath := filepath.Join(t.TempDir(), "nope.zip")

	err := client.DownloadModule("test/module", "9.9.9", destPath)
	if err == nil {
		t.Fatal("expected error for nonexistent version")
	}
}

func TestLocalClient_MultipleVersions(t *testing.T) {
	reg, _ := setupRegistryWithModule(t)
	defer reg.Close()

	// Publish a second version
	_, err := reg.Backend().Publish(context.Background(), &storage.PublishRequest{
		ModuleName: "test/module",
		Version:    "2.0.0",
		ZipData:    bytes.NewReader([]byte("PK\x03\x04v2")),
		Manifest:   []byte("name: test/module\nversion: 2.0.0\ntype: starlark\nentrypoint: main.star\n"),
		Hash:       "def456",
	})
	if err != nil {
		t.Fatalf("Publish v2: %v", err)
	}

	client := NewLocalClient(reg)
	versions, err := client.ListVersions("test/module")
	if err != nil {
		t.Fatalf("ListVersions: %v", err)
	}
	if len(versions) != 2 {
		t.Fatalf("expected 2 versions, got %d", len(versions))
	}
	// FilesystemBackend returns versions sorted descending
	if versions[0] != "2.0.0" {
		t.Errorf("versions[0] = %q, want 2.0.0", versions[0])
	}
}
