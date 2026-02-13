package storage

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/shawnbutts/keystone-core/internal/files/backend"
)

// mockBackend implements backend.Backend for testing.
type mockBackend struct {
	objects map[string][]byte // path → content
}

func newMockBackend() *mockBackend {
	return &mockBackend{objects: make(map[string][]byte)}
}

func (m *mockBackend) Name() string            { return "mock" }
func (m *mockBackend) Type() backend.Type       { return "mock" }
func (m *mockBackend) BaseConfig() *backend.Config { return &backend.Config{} }
func (m *mockBackend) Health(_ context.Context) error { return nil }
func (m *mockBackend) Close() error                    { return nil }

func (m *mockBackend) Get(_ context.Context, path string, _ *backend.GetOptions) (*backend.GetResult, error) {
	data, ok := m.objects[path]
	if !ok {
		return nil, backend.ErrNotFound
	}
	return &backend.GetResult{
		Reader: io.NopCloser(bytes.NewReader(data)),
		Info:   &backend.FileInfo{Path: path, Size: int64(len(data))},
	}, nil
}

func (m *mockBackend) Put(_ context.Context, path string, reader io.Reader, _ *backend.PutOptions) (*backend.PutResult, error) {
	data, err := io.ReadAll(reader)
	if err != nil {
		return nil, err
	}
	m.objects[path] = data
	return &backend.PutResult{Size: int64(len(data))}, nil
}

func (m *mockBackend) Delete(_ context.Context, path string) error {
	if _, ok := m.objects[path]; !ok {
		return nil // idempotent
	}
	delete(m.objects, path)
	return nil
}

func (m *mockBackend) Exists(_ context.Context, path string) (bool, error) {
	_, ok := m.objects[path]
	return ok, nil
}

func (m *mockBackend) Stat(_ context.Context, path string) (*backend.FileInfo, error) {
	data, ok := m.objects[path]
	if !ok {
		return nil, backend.ErrNotFound
	}
	return &backend.FileInfo{Path: path, Size: int64(len(data))}, nil
}

func (m *mockBackend) List(_ context.Context, prefix string, _ *backend.ListOptions) (*backend.ListResult, error) {
	// Ensure prefix ends with /
	if !strings.HasSuffix(prefix, "/") {
		prefix += "/"
	}
	var files []backend.FileInfo
	for p, data := range m.objects {
		if strings.HasPrefix(p, prefix) || strings.HasPrefix("/"+p, prefix) {
			files = append(files, backend.FileInfo{
				Path: "/" + p,
				Size: int64(len(data)),
			})
		}
	}
	return &backend.ListResult{Files: files}, nil
}

// Verify mockBackend implements backend.Backend.
var _ backend.Backend = (*mockBackend)(nil)

func TestBackendAdapter_ListVersions(t *testing.T) {
	mb := newMockBackend()
	mb.objects["test/mod/1.0.0/module.zip"] = []byte("zip1")
	mb.objects["test/mod/2.0.0/module.zip"] = []byte("zip2")
	mb.objects["test/mod/2.0.0/module.yaml"] = []byte("yaml")

	adapter := NewBackendAdapter(mb)
	ctx := context.Background()

	versions, err := adapter.ListVersions(ctx, "test/mod")
	if err != nil {
		t.Fatalf("ListVersions: %v", err)
	}
	if len(versions) != 2 {
		t.Fatalf("expected 2 versions, got %d", len(versions))
	}
}

func TestBackendAdapter_ListVersions_NotFound(t *testing.T) {
	adapter := NewBackendAdapter(newMockBackend())
	_, err := adapter.ListVersions(context.Background(), "no/mod")
	if !errors.Is(err, ErrModuleNotFound) {
		t.Fatalf("expected ErrModuleNotFound, got %v", err)
	}
}

func TestBackendAdapter_GetInfo(t *testing.T) {
	mb := newMockBackend()
	info := StoredModule{Name: "test/mod", Version: "1.0.0", Hash: "sha256:abc"}
	data, _ := json.Marshal(info)
	mb.objects["test/mod/1.0.0/module.info"] = data

	adapter := NewBackendAdapter(mb)
	got, err := adapter.GetInfo(context.Background(), "test/mod", "1.0.0")
	if err != nil {
		t.Fatalf("GetInfo: %v", err)
	}
	if got.Hash != "sha256:abc" {
		t.Fatalf("unexpected hash: %s", got.Hash)
	}
}

func TestBackendAdapter_GetInfo_NotFound(t *testing.T) {
	adapter := NewBackendAdapter(newMockBackend())
	_, err := adapter.GetInfo(context.Background(), "no/mod", "1.0.0")
	if !errors.Is(err, ErrVersionNotFound) {
		t.Fatalf("expected ErrVersionNotFound, got %v", err)
	}
}

func TestBackendAdapter_GetManifest(t *testing.T) {
	mb := newMockBackend()
	manifest := "name: test/mod\nversion: 1.0.0\n"
	mb.objects["test/mod/1.0.0/module.yaml"] = []byte(manifest)

	adapter := NewBackendAdapter(mb)
	rc, size, err := adapter.GetManifest(context.Background(), "test/mod", "1.0.0")
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

func TestBackendAdapter_GetManifest_NotFound(t *testing.T) {
	adapter := NewBackendAdapter(newMockBackend())
	_, _, err := adapter.GetManifest(context.Background(), "no/mod", "1.0.0")
	if !errors.Is(err, ErrVersionNotFound) {
		t.Fatalf("expected ErrVersionNotFound, got %v", err)
	}
}

func TestBackendAdapter_GetZip(t *testing.T) {
	mb := newMockBackend()
	mb.objects["test/mod/1.0.0/module.zip"] = []byte("zipdata")

	adapter := NewBackendAdapter(mb)
	rc, size, err := adapter.GetZip(context.Background(), "test/mod", "1.0.0")
	if err != nil {
		t.Fatalf("GetZip: %v", err)
	}
	defer rc.Close()

	if size != 7 {
		t.Fatalf("expected size 7, got %d", size)
	}
}

func TestBackendAdapter_GetZip_NotFound(t *testing.T) {
	adapter := NewBackendAdapter(newMockBackend())
	_, _, err := adapter.GetZip(context.Background(), "no/mod", "1.0.0")
	if !errors.Is(err, ErrVersionNotFound) {
		t.Fatalf("expected ErrVersionNotFound, got %v", err)
	}
}

func TestBackendAdapter_Publish(t *testing.T) {
	mb := newMockBackend()
	adapter := NewBackendAdapter(mb)

	req := &PublishRequest{
		ModuleName:   "test/pub",
		Version:      "1.0.0",
		ZipData:      bytes.NewReader([]byte("zip bytes")),
		Manifest:     []byte("manifest data"),
		Signature:    "sig-data",
		Hash:         "sha256:deadbeef",
		Description:  "Test",
		Dependencies: map[string]string{"dep/a": ">=1.0.0"},
		Tags:         []string{"stable"},
		ReleaseNotes: "First",
	}

	result, err := adapter.Publish(context.Background(), req)
	if err != nil {
		t.Fatalf("Publish: %v", err)
	}
	if result.Hash != "sha256:deadbeef" {
		t.Fatalf("unexpected hash: %s", result.Hash)
	}

	// All 4 files should exist
	for _, key := range []string{
		"test/pub/1.0.0/module.zip",
		"test/pub/1.0.0/module.yaml",
		"test/pub/1.0.0/module.sig",
		"test/pub/1.0.0/module.info",
	} {
		if _, ok := mb.objects[key]; !ok {
			t.Fatalf("expected %s to exist", key)
		}
	}

	// Verify info content
	var stored StoredModule
	json.Unmarshal(mb.objects["test/pub/1.0.0/module.info"], &stored)
	if stored.Description != "Test" || stored.ReleaseNotes != "First" {
		t.Fatalf("unexpected stored info: %+v", stored)
	}
}

func TestBackendAdapter_Publish_NoSignature(t *testing.T) {
	mb := newMockBackend()
	adapter := NewBackendAdapter(mb)

	req := &PublishRequest{
		ModuleName: "test/nosig",
		Version:    "1.0.0",
		ZipData:    bytes.NewReader([]byte("zip")),
		Manifest:   []byte("manifest"),
		Hash:       "sha256:abc",
	}

	_, err := adapter.Publish(context.Background(), req)
	if err != nil {
		t.Fatalf("Publish: %v", err)
	}

	if _, ok := mb.objects["test/nosig/1.0.0/module.sig"]; ok {
		t.Fatal("signature file should not exist when not provided")
	}
}

func TestBackendAdapter_Publish_Conflict(t *testing.T) {
	mb := newMockBackend()
	mb.objects["test/mod/1.0.0/module.zip"] = []byte("existing")
	adapter := NewBackendAdapter(mb)

	req := &PublishRequest{
		ModuleName: "test/mod",
		Version:    "1.0.0",
		ZipData:    bytes.NewReader([]byte("new")),
		Manifest:   []byte("manifest"),
		Hash:       "sha256:abc",
	}

	_, err := adapter.Publish(context.Background(), req)
	if !errors.Is(err, ErrVersionExists) {
		t.Fatalf("expected ErrVersionExists, got %v", err)
	}
}

func TestBackendAdapter_Publish_Force(t *testing.T) {
	mb := newMockBackend()
	mb.objects["test/mod/1.0.0/module.zip"] = []byte("old")
	adapter := NewBackendAdapter(mb)

	req := &PublishRequest{
		ModuleName: "test/mod",
		Version:    "1.0.0",
		ZipData:    bytes.NewReader([]byte("new content")),
		Manifest:   []byte("manifest"),
		Hash:       "sha256:new",
		Force:      true,
	}

	_, err := adapter.Publish(context.Background(), req)
	if err != nil {
		t.Fatalf("Publish with force: %v", err)
	}

	if string(mb.objects["test/mod/1.0.0/module.zip"]) != "new content" {
		t.Fatal("zip content not updated")
	}
}

func TestBackendAdapter_Delete(t *testing.T) {
	mb := newMockBackend()
	mb.objects["test/mod/1.0.0/module.zip"] = []byte("zip")
	mb.objects["test/mod/1.0.0/module.yaml"] = []byte("yaml")
	mb.objects["test/mod/1.0.0/module.info"] = []byte("{}")
	adapter := NewBackendAdapter(mb)

	if err := adapter.Delete(context.Background(), "test/mod", "1.0.0"); err != nil {
		t.Fatalf("Delete: %v", err)
	}

	// All files should be gone
	for key := range mb.objects {
		if strings.HasPrefix(key, "test/mod/1.0.0/") {
			t.Fatalf("file %s should have been deleted", key)
		}
	}
}

func TestBackendAdapter_Delete_NotFound(t *testing.T) {
	adapter := NewBackendAdapter(newMockBackend())
	err := adapter.Delete(context.Background(), "no/mod", "1.0.0")
	if !errors.Is(err, ErrVersionNotFound) {
		t.Fatalf("expected ErrVersionNotFound, got %v", err)
	}
}

func TestBackendAdapter_VersionExists(t *testing.T) {
	mb := newMockBackend()
	adapter := NewBackendAdapter(mb)
	ctx := context.Background()

	exists, err := adapter.VersionExists(ctx, "no/mod", "1.0.0")
	if err != nil {
		t.Fatalf("VersionExists: %v", err)
	}
	if exists {
		t.Fatal("expected false")
	}

	mb.objects["test/mod/1.0.0/module.zip"] = []byte("zip")
	exists, err = adapter.VersionExists(ctx, "test/mod", "1.0.0")
	if err != nil {
		t.Fatalf("VersionExists: %v", err)
	}
	if !exists {
		t.Fatal("expected true")
	}
}

func TestBackendAdapter_Health(t *testing.T) {
	adapter := NewBackendAdapter(newMockBackend())
	if err := adapter.Health(context.Background()); err != nil {
		t.Fatalf("Health: %v", err)
	}
}

func TestBackendAdapter_Close(t *testing.T) {
	adapter := NewBackendAdapter(newMockBackend())
	if err := adapter.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
}
