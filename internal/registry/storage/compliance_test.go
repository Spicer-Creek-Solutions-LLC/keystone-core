package storage

import (
	"bytes"
	"context"
	"errors"
	"io"
	"strings"
	"testing"
)

// RunBackendCompliance exercises all Backend operations against a concrete implementation.
// Call with a factory that returns a fresh, empty backend each time.
func RunBackendCompliance(t *testing.T, newBackend func(t *testing.T) Backend) {
	t.Helper()

	t.Run("ListVersions_Empty", func(t *testing.T) {
		b := newBackend(t)
		defer b.Close()
		_, err := b.ListVersions(context.Background(), "no/such/module")
		if !errors.Is(err, ErrModuleNotFound) {
			t.Fatalf("expected ErrModuleNotFound, got %v", err)
		}
	})

	t.Run("GetInfo_NotFound", func(t *testing.T) {
		b := newBackend(t)
		defer b.Close()
		_, err := b.GetInfo(context.Background(), "no/mod", "1.0.0")
		if !errors.Is(err, ErrVersionNotFound) {
			t.Fatalf("expected ErrVersionNotFound, got %v", err)
		}
	})

	t.Run("GetManifest_NotFound", func(t *testing.T) {
		b := newBackend(t)
		defer b.Close()
		_, _, err := b.GetManifest(context.Background(), "no/mod", "1.0.0")
		if !errors.Is(err, ErrVersionNotFound) {
			t.Fatalf("expected ErrVersionNotFound, got %v", err)
		}
	})

	t.Run("GetZip_NotFound", func(t *testing.T) {
		b := newBackend(t)
		defer b.Close()
		_, _, err := b.GetZip(context.Background(), "no/mod", "1.0.0")
		if !errors.Is(err, ErrVersionNotFound) {
			t.Fatalf("expected ErrVersionNotFound, got %v", err)
		}
	})

	t.Run("VersionExists_NotFound", func(t *testing.T) {
		b := newBackend(t)
		defer b.Close()
		exists, err := b.VersionExists(context.Background(), "no/mod", "1.0.0")
		if err != nil {
			t.Fatalf("VersionExists: %v", err)
		}
		if exists {
			t.Fatal("expected false for nonexistent version")
		}
	})

	t.Run("Delete_NotFound", func(t *testing.T) {
		b := newBackend(t)
		defer b.Close()
		err := b.Delete(context.Background(), "no/mod", "1.0.0")
		if !errors.Is(err, ErrVersionNotFound) {
			t.Fatalf("expected ErrVersionNotFound, got %v", err)
		}
	})

	t.Run("Publish_And_Retrieve", func(t *testing.T) {
		b := newBackend(t)
		defer b.Close()
		ctx := context.Background()

		zipData := []byte("fake-zip-bytes")
		manifest := []byte("name: test/mod\nversion: 1.0.0\n")

		req := &PublishRequest{
			ModuleName:   "test/mod",
			Version:      "1.0.0",
			ZipData:      bytes.NewReader(zipData),
			Manifest:     manifest,
			Signature:    "sig-data",
			Hash:         "sha256:abc123",
			Description:  "Test module",
			Dependencies: map[string]string{"dep/a": ">=1.0.0"},
			Tags:         []string{"stable"},
			ReleaseNotes: "Initial release",
		}

		result, err := b.Publish(ctx, req)
		if err != nil {
			t.Fatalf("Publish: %v", err)
		}
		if result.Hash != "sha256:abc123" {
			t.Fatalf("expected hash sha256:abc123, got %s", result.Hash)
		}

		// VersionExists
		exists, err := b.VersionExists(ctx, "test/mod", "1.0.0")
		if err != nil {
			t.Fatalf("VersionExists: %v", err)
		}
		if !exists {
			t.Fatal("expected version to exist")
		}

		// ListVersions
		versions, err := b.ListVersions(ctx, "test/mod")
		if err != nil {
			t.Fatalf("ListVersions: %v", err)
		}
		if len(versions) != 1 || versions[0] != "1.0.0" {
			t.Fatalf("expected [1.0.0], got %v", versions)
		}

		// GetInfo
		info, err := b.GetInfo(ctx, "test/mod", "1.0.0")
		if err != nil {
			t.Fatalf("GetInfo: %v", err)
		}
		if info.Hash != "sha256:abc123" {
			t.Fatalf("info hash: expected sha256:abc123, got %s", info.Hash)
		}
		if info.Description != "Test module" {
			t.Fatalf("info description: expected 'Test module', got %q", info.Description)
		}
		if info.ReleaseNotes != "Initial release" {
			t.Fatalf("info release_notes: expected 'Initial release', got %q", info.ReleaseNotes)
		}
		if len(info.Tags) != 1 || info.Tags[0] != "stable" {
			t.Fatalf("info tags: expected [stable], got %v", info.Tags)
		}
		if info.Dependencies["dep/a"] != ">=1.0.0" {
			t.Fatalf("info dependencies: expected dep/a>=1.0.0, got %v", info.Dependencies)
		}

		// GetManifest
		mRC, mSize, err := b.GetManifest(ctx, "test/mod", "1.0.0")
		if err != nil {
			t.Fatalf("GetManifest: %v", err)
		}
		mData, _ := io.ReadAll(mRC)
		mRC.Close()
		if mSize != int64(len(manifest)) {
			t.Fatalf("manifest size: expected %d, got %d", len(manifest), mSize)
		}
		if string(mData) != string(manifest) {
			t.Fatalf("manifest data mismatch")
		}

		// GetZip
		zRC, zSize, err := b.GetZip(ctx, "test/mod", "1.0.0")
		if err != nil {
			t.Fatalf("GetZip: %v", err)
		}
		zData, _ := io.ReadAll(zRC)
		zRC.Close()
		if zSize != int64(len(zipData)) {
			t.Fatalf("zip size: expected %d, got %d", len(zipData), zSize)
		}
		if !bytes.Equal(zData, zipData) {
			t.Fatal("zip data mismatch")
		}
	})

	t.Run("Publish_Conflict", func(t *testing.T) {
		b := newBackend(t)
		defer b.Close()
		ctx := context.Background()

		req := &PublishRequest{
			ModuleName: "test/conflict",
			Version:    "1.0.0",
			ZipData:    bytes.NewReader([]byte("zip")),
			Manifest:   []byte("manifest"),
			Hash:       "sha256:first",
		}
		if _, err := b.Publish(ctx, req); err != nil {
			t.Fatalf("first Publish: %v", err)
		}

		req.ZipData = bytes.NewReader([]byte("zip2"))
		_, err := b.Publish(ctx, req)
		if !errors.Is(err, ErrVersionExists) {
			t.Fatalf("expected ErrVersionExists, got %v", err)
		}
	})

	t.Run("Publish_Force", func(t *testing.T) {
		b := newBackend(t)
		defer b.Close()
		ctx := context.Background()

		req := &PublishRequest{
			ModuleName: "test/force",
			Version:    "1.0.0",
			ZipData:    bytes.NewReader([]byte("old")),
			Manifest:   []byte("manifest"),
			Hash:       "sha256:old",
		}
		if _, err := b.Publish(ctx, req); err != nil {
			t.Fatalf("first Publish: %v", err)
		}

		req.ZipData = bytes.NewReader([]byte("new"))
		req.Hash = "sha256:new"
		req.Force = true
		result, err := b.Publish(ctx, req)
		if err != nil {
			t.Fatalf("force Publish: %v", err)
		}
		if result.Hash != "sha256:new" {
			t.Fatalf("expected hash sha256:new, got %s", result.Hash)
		}
	})

	t.Run("Publish_NoSignature", func(t *testing.T) {
		b := newBackend(t)
		defer b.Close()
		ctx := context.Background()

		req := &PublishRequest{
			ModuleName: "test/nosig",
			Version:    "1.0.0",
			ZipData:    bytes.NewReader([]byte("zip")),
			Manifest:   []byte("manifest"),
			Hash:       "sha256:abc",
		}
		_, err := b.Publish(ctx, req)
		if err != nil {
			t.Fatalf("Publish: %v", err)
		}

		info, err := b.GetInfo(ctx, "test/nosig", "1.0.0")
		if err != nil {
			t.Fatalf("GetInfo: %v", err)
		}
		if info.Signature != "" {
			t.Fatalf("expected empty signature, got %q", info.Signature)
		}
	})

	t.Run("Delete", func(t *testing.T) {
		b := newBackend(t)
		defer b.Close()
		ctx := context.Background()

		req := &PublishRequest{
			ModuleName: "test/del",
			Version:    "1.0.0",
			ZipData:    bytes.NewReader([]byte("zip")),
			Manifest:   []byte("manifest"),
			Hash:       "sha256:abc",
		}
		if _, err := b.Publish(ctx, req); err != nil {
			t.Fatalf("Publish: %v", err)
		}

		if err := b.Delete(ctx, "test/del", "1.0.0"); err != nil {
			t.Fatalf("Delete: %v", err)
		}

		exists, err := b.VersionExists(ctx, "test/del", "1.0.0")
		if err != nil {
			t.Fatalf("VersionExists: %v", err)
		}
		if exists {
			t.Fatal("expected version to not exist after delete")
		}
	})

	t.Run("MultipleVersions", func(t *testing.T) {
		b := newBackend(t)
		defer b.Close()
		ctx := context.Background()

		for _, v := range []string{"1.0.0", "2.0.0", "3.0.0"} {
			req := &PublishRequest{
				ModuleName: "test/multi",
				Version:    v,
				ZipData:    bytes.NewReader([]byte("zip-" + v)),
				Manifest:   []byte("name: test/multi\nversion: " + v + "\n"),
				Hash:       "sha256:" + v,
			}
			if _, err := b.Publish(ctx, req); err != nil {
				t.Fatalf("Publish %s: %v", v, err)
			}
		}

		versions, err := b.ListVersions(ctx, "test/multi")
		if err != nil {
			t.Fatalf("ListVersions: %v", err)
		}
		if len(versions) != 3 {
			t.Fatalf("expected 3 versions, got %d", len(versions))
		}

		// Delete middle version
		if err := b.Delete(ctx, "test/multi", "2.0.0"); err != nil {
			t.Fatalf("Delete: %v", err)
		}

		versions, err = b.ListVersions(ctx, "test/multi")
		if err != nil {
			t.Fatalf("ListVersions after delete: %v", err)
		}
		if len(versions) != 2 {
			t.Fatalf("expected 2 versions, got %d", len(versions))
		}
		for _, v := range versions {
			if v == "2.0.0" {
				t.Fatal("deleted version still listed")
			}
		}
	})

	t.Run("Health", func(t *testing.T) {
		b := newBackend(t)
		defer b.Close()
		if err := b.Health(context.Background()); err != nil {
			t.Fatalf("Health: %v", err)
		}
	})

	t.Run("Close", func(t *testing.T) {
		b := newBackend(t)
		if err := b.Close(); err != nil {
			t.Fatalf("Close: %v", err)
		}
	})

	t.Run("NestedModuleName", func(t *testing.T) {
		b := newBackend(t)
		defer b.Close()
		ctx := context.Background()

		req := &PublishRequest{
			ModuleName: "org/team/submod",
			Version:    "1.0.0",
			ZipData:    bytes.NewReader([]byte("zip")),
			Manifest:   []byte("name: org/team/submod\nversion: 1.0.0\n"),
			Hash:       "sha256:nested",
		}
		if _, err := b.Publish(ctx, req); err != nil {
			t.Fatalf("Publish: %v", err)
		}

		versions, err := b.ListVersions(ctx, "org/team/submod")
		if err != nil {
			t.Fatalf("ListVersions: %v", err)
		}
		if len(versions) != 1 || versions[0] != "1.0.0" {
			t.Fatalf("expected [1.0.0], got %v", versions)
		}
	})
}

// TestFilesystemBackend_Compliance runs the compliance suite against FilesystemBackend.
func TestFilesystemBackend_Compliance(t *testing.T) {
	RunBackendCompliance(t, func(t *testing.T) Backend {
		t.Helper()
		b, err := NewFilesystemBackend(t.TempDir())
		if err != nil {
			t.Fatalf("NewFilesystemBackend: %v", err)
		}
		return b
	})
}

// TestBackendAdapter_Compliance runs the compliance suite against BackendAdapter with a mock backend.
func TestBackendAdapter_Compliance(t *testing.T) {
	RunBackendCompliance(t, func(t *testing.T) Backend {
		t.Helper()
		return NewBackendAdapter(newMockBackend())
	})
}

// TestBackendAdapter_ListVersions_PathParsing verifies that the adapter correctly
// parses version directories from the mock backend's list results.
func TestBackendAdapter_ListVersions_PathParsing(t *testing.T) {
	mb := newMockBackend()
	// Simulate objects as stored by the adapter
	mb.objects["deep/nested/mod/1.0.0/module.zip"] = []byte("zip")
	mb.objects["deep/nested/mod/1.0.0/module.info"] = []byte(`{"name":"deep/nested/mod","version":"1.0.0","hash":"sha256:x"}`)
	mb.objects["deep/nested/mod/2.0.0/module.zip"] = []byte("zip2")
	mb.objects["deep/nested/mod/2.0.0/module.info"] = []byte(`{"name":"deep/nested/mod","version":"2.0.0","hash":"sha256:y"}`)

	adapter := NewBackendAdapter(mb)
	versions, err := adapter.ListVersions(context.Background(), "deep/nested/mod")
	if err != nil {
		t.Fatalf("ListVersions: %v", err)
	}
	if len(versions) != 2 {
		t.Fatalf("expected 2 versions, got %d: %v", len(versions), versions)
	}

	// Both versions should be present
	found := map[string]bool{}
	for _, v := range versions {
		found[v] = true
	}
	if !found["1.0.0"] || !found["2.0.0"] {
		t.Fatalf("expected 1.0.0 and 2.0.0, got %v", versions)
	}
}

// TestMigrate_DryRun verifies dry-run mode logs without writing.
func TestMigrate_DryRun(t *testing.T) {
	src, err := NewFilesystemBackend(t.TempDir())
	if err != nil {
		t.Fatalf("NewFilesystemBackend: %v", err)
	}
	ctx := context.Background()

	// Publish a module to source
	req := &PublishRequest{
		ModuleName: "test/migrate",
		Version:    "1.0.0",
		ZipData:    bytes.NewReader([]byte("zip")),
		Manifest:   []byte("manifest"),
		Hash:       "sha256:mig",
	}
	if _, err := src.Publish(ctx, req); err != nil {
		t.Fatalf("Publish: %v", err)
	}

	dest, err := NewFilesystemBackend(t.TempDir())
	if err != nil {
		t.Fatalf("NewFilesystemBackend: %v", err)
	}

	// Dry run — dest should remain empty
	if err := Migrate(ctx, src, dest, &MigrateOptions{DryRun: true}); err != nil {
		t.Fatalf("Migrate dry-run: %v", err)
	}

	exists, _ := dest.VersionExists(ctx, "test/migrate", "1.0.0")
	if exists {
		t.Fatal("dry-run should not have written to destination")
	}
}

// TestMigrate_FilesystemToFilesystem verifies actual migration between filesystem backends.
func TestMigrate_FilesystemToFilesystem(t *testing.T) {
	src, err := NewFilesystemBackend(t.TempDir())
	if err != nil {
		t.Fatalf("NewFilesystemBackend: %v", err)
	}
	ctx := context.Background()

	// Publish modules to source
	for _, v := range []string{"1.0.0", "2.0.0"} {
		req := &PublishRequest{
			ModuleName:   "test/migrate",
			Version:      v,
			ZipData:      bytes.NewReader([]byte("zip-" + v)),
			Manifest:     []byte("name: test/migrate\nversion: " + v + "\n"),
			Signature:    "sig-" + v,
			Hash:         "sha256:" + v,
			Description:  "v" + v,
			ReleaseNotes: "release " + v,
		}
		if _, err := src.Publish(ctx, req); err != nil {
			t.Fatalf("Publish %s: %v", v, err)
		}
	}

	dest, err := NewFilesystemBackend(t.TempDir())
	if err != nil {
		t.Fatalf("NewFilesystemBackend: %v", err)
	}

	if err := Migrate(ctx, src, dest, nil); err != nil {
		t.Fatalf("Migrate: %v", err)
	}

	// Verify both versions exist in dest
	versions, err := dest.ListVersions(ctx, "test/migrate")
	if err != nil {
		t.Fatalf("ListVersions: %v", err)
	}
	if len(versions) != 2 {
		t.Fatalf("expected 2 versions, got %d", len(versions))
	}

	// Verify data integrity
	for _, v := range []string{"1.0.0", "2.0.0"} {
		info, err := dest.GetInfo(ctx, "test/migrate", v)
		if err != nil {
			t.Fatalf("GetInfo %s: %v", v, err)
		}
		if info.Hash != "sha256:"+v {
			t.Fatalf("hash mismatch for %s: expected sha256:%s, got %s", v, v, info.Hash)
		}
		if info.Description != "v"+v {
			t.Fatalf("description mismatch for %s: %q", v, info.Description)
		}

		zRC, _, err := dest.GetZip(ctx, "test/migrate", v)
		if err != nil {
			t.Fatalf("GetZip %s: %v", v, err)
		}
		zData, _ := io.ReadAll(zRC)
		zRC.Close()
		if !strings.Contains(string(zData), v) {
			t.Fatalf("zip data for %s doesn't contain version string", v)
		}
	}
}

// TestMigrate_NonFilesystemSource verifies error when source is not filesystem.
func TestMigrate_NonFilesystemSource(t *testing.T) {
	adapter := NewBackendAdapter(newMockBackend())
	dest, err := NewFilesystemBackend(t.TempDir())
	if err != nil {
		t.Fatalf("NewFilesystemBackend: %v", err)
	}

	err = Migrate(context.Background(), adapter, dest, nil)
	if err == nil {
		t.Fatal("expected error for non-filesystem source")
	}
	if !strings.Contains(err.Error(), "filesystem") {
		t.Fatalf("expected filesystem error, got: %v", err)
	}
}
