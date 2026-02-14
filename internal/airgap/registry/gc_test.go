package registry

import (
	"bytes"
	"context"
	"testing"
	"time"

	"github.com/shawnbutts/keystone-core/internal/registry/storage"
)

func setupRegistryForGC(t *testing.T) *Registry {
	t.Helper()
	root := t.TempDir()
	reg, err := Init(Config{RootDir: root})
	if err != nil {
		t.Fatalf("Init: %v", err)
	}

	// Publish multiple versions of a module
	for _, v := range []string{"1.0.0", "2.0.0", "3.0.0", "4.0.0"} {
		_, err := reg.Backend().Publish(context.Background(), &storage.PublishRequest{
			ModuleName: "test/mod",
			Version:    v,
			ZipData:    bytes.NewReader([]byte("PK\x03\x04data-" + v)),
			Hash:       "hash-" + v,
		})
		if err != nil {
			t.Fatalf("Publish %s: %v", v, err)
		}
	}

	return reg
}

func TestGC_KeepVersions(t *testing.T) {
	reg := setupRegistryForGC(t)
	defer reg.Close()

	result, err := reg.GC(context.Background(), GCConfig{
		KeepVersions: 2,
	})
	if err != nil {
		t.Fatalf("GC: %v", err)
	}

	if len(result.RemovedModules) != 2 {
		t.Errorf("removed %d versions, want 2; removed: %v", len(result.RemovedModules), result.RemovedModules)
	}

	// Verify remaining versions
	versions, err := reg.Backend().ListVersions(context.Background(), "test/mod")
	if err != nil {
		t.Fatalf("ListVersions: %v", err)
	}
	if len(versions) != 2 {
		t.Errorf("remaining versions = %d, want 2: %v", len(versions), versions)
	}
}

func TestGC_DryRun(t *testing.T) {
	reg := setupRegistryForGC(t)
	defer reg.Close()

	result, err := reg.GC(context.Background(), GCConfig{
		KeepVersions: 1,
		DryRun:       true,
	})
	if err != nil {
		t.Fatalf("GC: %v", err)
	}

	if len(result.RemovedModules) != 3 {
		t.Errorf("dry-run would remove %d, want 3", len(result.RemovedModules))
	}

	// Verify nothing was actually deleted
	versions, err := reg.Backend().ListVersions(context.Background(), "test/mod")
	if err != nil {
		t.Fatalf("ListVersions: %v", err)
	}
	if len(versions) != 4 {
		t.Errorf("versions after dry-run = %d, want 4", len(versions))
	}
}

func TestGC_NoPruningConfig(t *testing.T) {
	reg := setupRegistryForGC(t)
	defer reg.Close()

	result, err := reg.GC(context.Background(), GCConfig{})
	if err != nil {
		t.Fatalf("GC: %v", err)
	}

	if len(result.RemovedModules) != 0 {
		t.Errorf("expected no removals with empty config, got %d", len(result.RemovedModules))
	}
}

func TestGC_MaxAge(t *testing.T) {
	reg := setupRegistryForGC(t)
	defer reg.Close()

	// All modules were just published, so MaxAge of 1 hour should remove nothing
	result, err := reg.GC(context.Background(), GCConfig{
		MaxAge: time.Hour,
	})
	if err != nil {
		t.Fatalf("GC: %v", err)
	}
	if len(result.RemovedModules) != 0 {
		t.Errorf("expected no removals for recent modules, got %d", len(result.RemovedModules))
	}

	// MaxAge of 0s (effectively all are older) should remove everything
	result, err = reg.GC(context.Background(), GCConfig{
		MaxAge: time.Nanosecond,
	})
	if err != nil {
		t.Fatalf("GC: %v", err)
	}
	if len(result.RemovedModules) != 4 {
		t.Errorf("expected 4 removals, got %d", len(result.RemovedModules))
	}
}

func TestGC_KeepVersionsAndMaxAge(t *testing.T) {
	reg := setupRegistryForGC(t)
	defer reg.Close()

	// KeepVersions=2 means 2 candidates (oldest 2), MaxAge=1h means none are old enough
	// → intersection is empty, nothing removed
	result, err := reg.GC(context.Background(), GCConfig{
		KeepVersions: 2,
		MaxAge:       time.Hour,
	})
	if err != nil {
		t.Fatalf("GC: %v", err)
	}
	if len(result.RemovedModules) != 0 {
		t.Errorf("expected 0 removals (intersection), got %d", len(result.RemovedModules))
	}
}

func TestGC_ReclaimedBytes(t *testing.T) {
	reg := setupRegistryForGC(t)
	defer reg.Close()

	result, err := reg.GC(context.Background(), GCConfig{
		KeepVersions: 1,
		DryRun:       true,
	})
	if err != nil {
		t.Fatalf("GC: %v", err)
	}
	if result.ReclaimedBytes == 0 {
		t.Error("expected non-zero reclaimed bytes")
	}
}

func TestGC_CancelledContext(t *testing.T) {
	reg := setupRegistryForGC(t)
	defer reg.Close()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := reg.GC(ctx, GCConfig{KeepVersions: 1})
	if err == nil {
		t.Fatal("expected error for cancelled context")
	}
}

func TestGC_AutoReindex(t *testing.T) {
	root := t.TempDir()
	reg, err := Init(Config{RootDir: root, AutoIndex: true})
	if err != nil {
		t.Fatal(err)
	}
	defer reg.Close()

	for _, v := range []string{"1.0.0", "2.0.0"} {
		_, err := reg.Backend().Publish(context.Background(), &storage.PublishRequest{
			ModuleName: "test/mod",
			Version:    v,
			ZipData:    bytes.NewReader([]byte("PK\x03\x04data")),
			Hash:       "hash-" + v,
		})
		if err != nil {
			t.Fatal(err)
		}
	}

	_, err = reg.GC(context.Background(), GCConfig{KeepVersions: 1})
	if err != nil {
		t.Fatal(err)
	}

	idx := reg.Index()
	if idx == nil {
		t.Fatal("expected index after GC with AutoIndex")
	}
	if len(idx.Modules) != 1 {
		t.Errorf("expected 1 module in index after GC, got %d", len(idx.Modules))
	}
	if idx.Modules[0].LatestVersion != "2.0.0" {
		t.Errorf("expected latest version 2.0.0, got %s", idx.Modules[0].LatestVersion)
	}
}
