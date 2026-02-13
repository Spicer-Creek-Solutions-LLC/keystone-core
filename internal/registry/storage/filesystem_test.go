package storage

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"
)

func TestNewFilesystemBackend(t *testing.T) {
	root := filepath.Join(t.TempDir(), "registry")
	fb, err := NewFilesystemBackend(root)
	if err != nil {
		t.Fatalf("NewFilesystemBackend: %v", err)
	}
	if fb == nil {
		t.Fatal("expected non-nil backend")
	}

	// Root directory should exist
	if _, err := os.Stat(root); err != nil {
		t.Fatalf("root directory not created: %v", err)
	}
}

func TestFilesystemBackend_ListVersions(t *testing.T) {
	root := t.TempDir()
	fb, _ := NewFilesystemBackend(root)
	ctx := context.Background()

	// Module not found
	_, err := fb.ListVersions(ctx, "no/such")
	if !errors.Is(err, ErrModuleNotFound) {
		t.Fatalf("expected ErrModuleNotFound, got %v", err)
	}

	// Create versions
	for _, v := range []string{"1.0.0", "2.0.0", "1.1.0"} {
		dir := filepath.Join(root, "test/mod", v)
		os.MkdirAll(dir, 0o755)
		os.WriteFile(filepath.Join(dir, "module.zip"), []byte("zip"), 0o644)
	}
	// Create a directory without module.zip (should be skipped)
	os.MkdirAll(filepath.Join(root, "test/mod", "incomplete"), 0o755)

	versions, err := fb.ListVersions(ctx, "test/mod")
	if err != nil {
		t.Fatalf("ListVersions: %v", err)
	}
	if len(versions) != 3 {
		t.Fatalf("expected 3 versions, got %d", len(versions))
	}
	// Should be reverse sorted
	if versions[0] != "2.0.0" || versions[1] != "1.1.0" || versions[2] != "1.0.0" {
		t.Fatalf("unexpected version order: %v", versions)
	}
}

func TestFilesystemBackend_GetInfo(t *testing.T) {
	root := t.TempDir()
	fb, _ := NewFilesystemBackend(root)
	ctx := context.Background()

	// Not found
	_, err := fb.GetInfo(ctx, "no/mod", "1.0.0")
	if !errors.Is(err, ErrVersionNotFound) {
		t.Fatalf("expected ErrVersionNotFound, got %v", err)
	}

	// Create info
	dir := filepath.Join(root, "test/mod", "1.0.0")
	os.MkdirAll(dir, 0o755)
	info := StoredModule{Name: "test/mod", Version: "1.0.0", Hash: "sha256:abc"}
	data, _ := json.Marshal(info)
	os.WriteFile(filepath.Join(dir, "module.info"), data, 0o644)

	got, err := fb.GetInfo(ctx, "test/mod", "1.0.0")
	if err != nil {
		t.Fatalf("GetInfo: %v", err)
	}
	if got.Name != "test/mod" || got.Version != "1.0.0" || got.Hash != "sha256:abc" {
		t.Fatalf("unexpected info: %+v", got)
	}
}

func TestFilesystemBackend_GetManifest(t *testing.T) {
	root := t.TempDir()
	fb, _ := NewFilesystemBackend(root)
	ctx := context.Background()

	// Not found
	_, _, err := fb.GetManifest(ctx, "no/mod", "1.0.0")
	if !errors.Is(err, ErrVersionNotFound) {
		t.Fatalf("expected ErrVersionNotFound, got %v", err)
	}

	// Create manifest
	dir := filepath.Join(root, "test/mod", "1.0.0")
	os.MkdirAll(dir, 0o755)
	manifest := "name: test/mod\nversion: 1.0.0\n"
	os.WriteFile(filepath.Join(dir, "module.yaml"), []byte(manifest), 0o644)

	rc, size, err := fb.GetManifest(ctx, "test/mod", "1.0.0")
	if err != nil {
		t.Fatalf("GetManifest: %v", err)
	}
	defer rc.Close()

	if size != int64(len(manifest)) {
		t.Fatalf("expected size %d, got %d", len(manifest), size)
	}
	data, _ := io.ReadAll(rc)
	if string(data) != manifest {
		t.Fatalf("expected %q, got %q", manifest, string(data))
	}
}

func TestFilesystemBackend_GetZip(t *testing.T) {
	root := t.TempDir()
	fb, _ := NewFilesystemBackend(root)
	ctx := context.Background()

	// Not found
	_, _, err := fb.GetZip(ctx, "no/mod", "1.0.0")
	if !errors.Is(err, ErrVersionNotFound) {
		t.Fatalf("expected ErrVersionNotFound, got %v", err)
	}

	// Create zip
	dir := filepath.Join(root, "test/mod", "1.0.0")
	os.MkdirAll(dir, 0o755)
	zipContent := []byte("fake zip content")
	os.WriteFile(filepath.Join(dir, "module.zip"), zipContent, 0o644)

	rc, size, err := fb.GetZip(ctx, "test/mod", "1.0.0")
	if err != nil {
		t.Fatalf("GetZip: %v", err)
	}
	defer rc.Close()

	if size != int64(len(zipContent)) {
		t.Fatalf("expected size %d, got %d", len(zipContent), size)
	}
	data, _ := io.ReadAll(rc)
	if !bytes.Equal(data, zipContent) {
		t.Fatal("zip content mismatch")
	}
}

func TestFilesystemBackend_Publish(t *testing.T) {
	root := t.TempDir()
	fb, _ := NewFilesystemBackend(root)
	ctx := context.Background()

	req := &PublishRequest{
		ModuleName:   "test/pub",
		Version:      "1.0.0",
		ZipData:      bytes.NewReader([]byte("zip bytes")),
		Manifest:     []byte("name: test/pub\nversion: 1.0.0\n"),
		Signature:    "sig-data",
		Hash:         "sha256:deadbeef",
		Description:  "A test module",
		Dependencies: map[string]string{"dep/a": ">=1.0.0"},
		Tags:         []string{"stable"},
		ReleaseNotes: "First release",
	}

	result, err := fb.Publish(ctx, req)
	if err != nil {
		t.Fatalf("Publish: %v", err)
	}
	if result.Hash != "sha256:deadbeef" {
		t.Fatalf("unexpected hash: %s", result.Hash)
	}
	if result.Size != 9 {
		t.Fatalf("unexpected size: %d", result.Size)
	}

	// Files should exist
	versionDir := filepath.Join(root, "test/pub", "1.0.0")
	for _, f := range []string{"module.zip", "module.yaml", "module.sig", "module.info"} {
		if _, err := os.Stat(filepath.Join(versionDir, f)); err != nil {
			t.Fatalf("expected %s to exist: %v", f, err)
		}
	}

	// Info should be valid JSON with correct fields
	info, _ := fb.GetInfo(ctx, "test/pub", "1.0.0")
	if info.Description != "A test module" {
		t.Fatalf("unexpected description: %s", info.Description)
	}
	if len(info.Tags) != 1 || info.Tags[0] != "stable" {
		t.Fatalf("unexpected tags: %v", info.Tags)
	}
}

func TestFilesystemBackend_Publish_Conflict(t *testing.T) {
	root := t.TempDir()
	fb, _ := NewFilesystemBackend(root)
	ctx := context.Background()

	// Create existing version
	dir := filepath.Join(root, "test/mod", "1.0.0")
	os.MkdirAll(dir, 0o755)
	os.WriteFile(filepath.Join(dir, "module.zip"), []byte("old"), 0o644)

	req := &PublishRequest{
		ModuleName: "test/mod",
		Version:    "1.0.0",
		ZipData:    bytes.NewReader([]byte("new")),
		Manifest:   []byte("manifest"),
		Hash:       "sha256:abc",
	}

	_, err := fb.Publish(ctx, req)
	if !errors.Is(err, ErrVersionExists) {
		t.Fatalf("expected ErrVersionExists, got %v", err)
	}
}

func TestFilesystemBackend_Publish_Force(t *testing.T) {
	root := t.TempDir()
	fb, _ := NewFilesystemBackend(root)
	ctx := context.Background()

	// Create existing version
	dir := filepath.Join(root, "test/mod", "1.0.0")
	os.MkdirAll(dir, 0o755)
	os.WriteFile(filepath.Join(dir, "module.zip"), []byte("old"), 0o644)

	req := &PublishRequest{
		ModuleName: "test/mod",
		Version:    "1.0.0",
		ZipData:    bytes.NewReader([]byte("new content")),
		Manifest:   []byte("manifest"),
		Hash:       "sha256:new",
		Force:      true,
	}

	result, err := fb.Publish(ctx, req)
	if err != nil {
		t.Fatalf("Publish with force: %v", err)
	}
	if result.Size != 11 {
		t.Fatalf("unexpected size: %d", result.Size)
	}
}

func TestFilesystemBackend_Publish_NoSignature(t *testing.T) {
	root := t.TempDir()
	fb, _ := NewFilesystemBackend(root)
	ctx := context.Background()

	req := &PublishRequest{
		ModuleName: "test/nosig",
		Version:    "1.0.0",
		ZipData:    bytes.NewReader([]byte("zip")),
		Manifest:   []byte("manifest"),
		Hash:       "sha256:abc",
	}

	_, err := fb.Publish(ctx, req)
	if err != nil {
		t.Fatalf("Publish: %v", err)
	}

	// Signature file should NOT exist
	sigPath := filepath.Join(root, "test/nosig", "1.0.0", "module.sig")
	if _, err := os.Stat(sigPath); !os.IsNotExist(err) {
		t.Fatal("signature file should not exist when not provided")
	}
}

func TestFilesystemBackend_Delete(t *testing.T) {
	root := t.TempDir()
	fb, _ := NewFilesystemBackend(root)
	ctx := context.Background()

	// Not found
	err := fb.Delete(ctx, "no/mod", "1.0.0")
	if !errors.Is(err, ErrVersionNotFound) {
		t.Fatalf("expected ErrVersionNotFound, got %v", err)
	}

	// Create version
	dir := filepath.Join(root, "test/del", "1.0.0")
	os.MkdirAll(dir, 0o755)
	os.WriteFile(filepath.Join(dir, "module.zip"), []byte("zip"), 0o644)

	if err := fb.Delete(ctx, "test/del", "1.0.0"); err != nil {
		t.Fatalf("Delete: %v", err)
	}

	// Version dir should be gone
	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Fatal("version directory should be deleted")
	}

	// Module dir should be cleaned up (was empty)
	moduleDir := filepath.Join(root, "test/del")
	if _, err := os.Stat(moduleDir); !os.IsNotExist(err) {
		t.Fatal("empty module directory should be cleaned up")
	}
}

func TestFilesystemBackend_Delete_KeepsOtherVersions(t *testing.T) {
	root := t.TempDir()
	fb, _ := NewFilesystemBackend(root)
	ctx := context.Background()

	// Create two versions
	for _, v := range []string{"1.0.0", "2.0.0"} {
		dir := filepath.Join(root, "test/mod", v)
		os.MkdirAll(dir, 0o755)
		os.WriteFile(filepath.Join(dir, "module.zip"), []byte("zip"), 0o644)
	}

	if err := fb.Delete(ctx, "test/mod", "1.0.0"); err != nil {
		t.Fatalf("Delete: %v", err)
	}

	// Module dir should still exist (2.0.0 remains)
	moduleDir := filepath.Join(root, "test/mod")
	if _, err := os.Stat(moduleDir); err != nil {
		t.Fatal("module directory should still exist with remaining versions")
	}
}

func TestFilesystemBackend_VersionExists(t *testing.T) {
	root := t.TempDir()
	fb, _ := NewFilesystemBackend(root)
	ctx := context.Background()

	exists, err := fb.VersionExists(ctx, "no/mod", "1.0.0")
	if err != nil {
		t.Fatalf("VersionExists: %v", err)
	}
	if exists {
		t.Fatal("expected false for nonexistent version")
	}

	dir := filepath.Join(root, "test/mod", "1.0.0")
	os.MkdirAll(dir, 0o755)
	os.WriteFile(filepath.Join(dir, "module.zip"), []byte("zip"), 0o644)

	exists, err = fb.VersionExists(ctx, "test/mod", "1.0.0")
	if err != nil {
		t.Fatalf("VersionExists: %v", err)
	}
	if !exists {
		t.Fatal("expected true for existing version")
	}
}

func TestFilesystemBackend_Health(t *testing.T) {
	root := t.TempDir()
	fb, _ := NewFilesystemBackend(root)

	if err := fb.Health(context.Background()); err != nil {
		t.Fatalf("Health: %v", err)
	}

	// With nonexistent root
	fb2 := &FilesystemBackend{root: "/nonexistent/path/xyz"}
	if err := fb2.Health(context.Background()); err == nil {
		t.Fatal("expected error for nonexistent root")
	}
}

func TestFilesystemBackend_Close(t *testing.T) {
	root := t.TempDir()
	fb, _ := NewFilesystemBackend(root)
	if err := fb.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
}

// Verify interface compliance.
var _ Backend = (*FilesystemBackend)(nil)
