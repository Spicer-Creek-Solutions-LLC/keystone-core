package blueprint

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestLocalStorage_CreateAndGet(t *testing.T) {
	// Create temp directory
	tmpDir, err := os.MkdirTemp("", "blueprint-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Create storage
	storage, err := NewLocalStorage(tmpDir, false)
	if err != nil {
		t.Fatalf("NewLocalStorage failed: %v", err)
	}
	defer storage.Close()

	ctx := context.Background()

	// Create a blueprint
	bp := &Blueprint{
		APIVersion: APIVersion,
		Kind:       Kind,
		Metadata: Metadata{
			Name:    "test-blueprint",
			Version: "1.0.0",
		},
		SourcePath: filepath.Join(tmpDir, "blueprints", "test"),
	}

	// Put the blueprint
	if err := storage.Put(ctx, bp); err != nil {
		t.Fatalf("Put failed: %v", err)
	}

	// Get the blueprint (vendor is "test" because SourcePath contains "blueprints/test")
	got, err := storage.Get(ctx, "test/test-blueprint", "1.0.0")
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}

	if got.Metadata.Name != bp.Metadata.Name {
		t.Errorf("Got name %s, want %s", got.Metadata.Name, bp.Metadata.Name)
	}
	if got.Metadata.Version != bp.Metadata.Version {
		t.Errorf("Got version %s, want %s", got.Metadata.Version, bp.Metadata.Version)
	}
}

func TestLocalStorage_GetLatest(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "blueprint-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	storage, err := NewLocalStorage(tmpDir, false)
	if err != nil {
		t.Fatalf("NewLocalStorage failed: %v", err)
	}
	defer storage.Close()

	ctx := context.Background()

	// Create multiple versions
	versions := []string{"1.0.0", "1.1.0", "2.0.0", "1.2.0"}
	for _, v := range versions {
		bp := &Blueprint{
			APIVersion: APIVersion,
			Kind:       Kind,
			Metadata: Metadata{
				Name:    "versioned-bp",
				Version: v,
			},
		}
		if err := storage.Put(ctx, bp); err != nil {
			t.Fatalf("Put %s failed: %v", v, err)
		}
	}

	// Get without version should return latest (2.0.0)
	got, err := storage.Get(ctx, "local/versioned-bp", "")
	if err != nil {
		t.Fatalf("Get latest failed: %v", err)
	}

	if got.Metadata.Version != "2.0.0" {
		t.Errorf("Got version %s, want 2.0.0 (latest)", got.Metadata.Version)
	}
}

func TestLocalStorage_Versions(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "blueprint-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	storage, err := NewLocalStorage(tmpDir, false)
	if err != nil {
		t.Fatalf("NewLocalStorage failed: %v", err)
	}
	defer storage.Close()

	ctx := context.Background()

	// Create multiple versions
	versions := []string{"1.0.0", "1.1.0", "2.0.0"}
	for _, v := range versions {
		bp := &Blueprint{
			APIVersion: APIVersion,
			Kind:       Kind,
			Metadata: Metadata{
				Name:    "multi-version",
				Version: v,
			},
		}
		if err := storage.Put(ctx, bp); err != nil {
			t.Fatalf("Put %s failed: %v", v, err)
		}
	}

	// Get versions
	gotVersions, err := storage.Versions(ctx, "local/multi-version")
	if err != nil {
		t.Fatalf("Versions failed: %v", err)
	}

	if len(gotVersions) != 3 {
		t.Errorf("Got %d versions, want 3", len(gotVersions))
	}

	// Should be sorted descending (newest first)
	if gotVersions[0] != "2.0.0" {
		t.Errorf("First version = %s, want 2.0.0", gotVersions[0])
	}
}

func TestLocalStorage_Exists(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "blueprint-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	storage, err := NewLocalStorage(tmpDir, false)
	if err != nil {
		t.Fatalf("NewLocalStorage failed: %v", err)
	}
	defer storage.Close()

	ctx := context.Background()

	// Create a blueprint
	bp := &Blueprint{
		APIVersion: APIVersion,
		Kind:       Kind,
		Metadata: Metadata{
			Name:    "exists-test",
			Version: "1.0.0",
		},
	}
	if err := storage.Put(ctx, bp); err != nil {
		t.Fatalf("Put failed: %v", err)
	}

	// Check exists
	exists, err := storage.Exists(ctx, "local/exists-test", "1.0.0")
	if err != nil {
		t.Fatalf("Exists failed: %v", err)
	}
	if !exists {
		t.Error("Exists = false, want true")
	}

	// Check non-existent
	exists, err = storage.Exists(ctx, "local/nonexistent", "1.0.0")
	if err != nil {
		t.Fatalf("Exists failed: %v", err)
	}
	if exists {
		t.Error("Exists = true, want false for non-existent blueprint")
	}
}

func TestLocalStorage_Delete(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "blueprint-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	storage, err := NewLocalStorage(tmpDir, false)
	if err != nil {
		t.Fatalf("NewLocalStorage failed: %v", err)
	}
	defer storage.Close()

	ctx := context.Background()

	// Create a blueprint
	bp := &Blueprint{
		APIVersion: APIVersion,
		Kind:       Kind,
		Metadata: Metadata{
			Name:    "delete-test",
			Version: "1.0.0",
		},
	}
	if err := storage.Put(ctx, bp); err != nil {
		t.Fatalf("Put failed: %v", err)
	}

	// Delete
	if err := storage.Delete(ctx, "local/delete-test", "1.0.0"); err != nil {
		t.Fatalf("Delete failed: %v", err)
	}

	// Verify deleted
	exists, _ := storage.Exists(ctx, "local/delete-test", "1.0.0")
	if exists {
		t.Error("Blueprint still exists after delete")
	}
}

func TestLocalStorage_ReadOnly(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "blueprint-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Create read-only storage
	storage, err := NewLocalStorage(tmpDir, true)
	if err != nil {
		t.Fatalf("NewLocalStorage failed: %v", err)
	}
	defer storage.Close()

	ctx := context.Background()

	// Try to put - should fail
	bp := &Blueprint{
		APIVersion: APIVersion,
		Kind:       Kind,
		Metadata: Metadata{
			Name:    "test",
			Version: "1.0.0",
		},
	}

	err = storage.Put(ctx, bp)
	if err != ErrStorageReadOnly {
		t.Errorf("Put on read-only storage: got %v, want ErrStorageReadOnly", err)
	}
}

func TestLocalStorage_List(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "blueprint-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	storage, err := NewLocalStorage(tmpDir, false)
	if err != nil {
		t.Fatalf("NewLocalStorage failed: %v", err)
	}
	defer storage.Close()

	ctx := context.Background()

	// Create blueprints
	blueprints := []struct {
		name     string
		version  string
		keywords []string
	}{
		{"web-app", "1.0.0", []string{"web", "nginx"}},
		{"database", "1.0.0", []string{"database", "postgres"}},
		{"monitoring", "1.0.0", []string{"monitoring", "prometheus"}},
	}

	for _, b := range blueprints {
		bp := &Blueprint{
			APIVersion: APIVersion,
			Kind:       Kind,
			Metadata: Metadata{
				Name:     b.name,
				Version:  b.version,
				Keywords: b.keywords,
			},
		}
		if err := storage.Put(ctx, bp); err != nil {
			t.Fatalf("Put %s failed: %v", b.name, err)
		}
	}

	// List all
	all, err := storage.List(ctx, nil)
	if err != nil {
		t.Fatalf("List failed: %v", err)
	}
	if len(all) != 3 {
		t.Errorf("List all: got %d, want 3", len(all))
	}

	// List with keyword filter
	filtered, err := storage.List(ctx, &ListFilter{Keywords: []string{"web"}})
	if err != nil {
		t.Fatalf("List with filter failed: %v", err)
	}
	if len(filtered) != 1 {
		t.Errorf("List with keyword filter: got %d, want 1", len(filtered))
	}

	// List with limit
	limited, err := storage.List(ctx, &ListFilter{Limit: 2})
	if err != nil {
		t.Fatalf("List with limit failed: %v", err)
	}
	if len(limited) != 2 {
		t.Errorf("List with limit: got %d, want 2", len(limited))
	}
}

func TestLocalStorage_NotFound(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "blueprint-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	storage, err := NewLocalStorage(tmpDir, false)
	if err != nil {
		t.Fatalf("NewLocalStorage failed: %v", err)
	}
	defer storage.Close()

	ctx := context.Background()

	_, err = storage.Get(ctx, "local/nonexistent", "")
	if err != ErrBlueprintNotFound {
		t.Errorf("Get nonexistent: got %v, want ErrBlueprintNotFound", err)
	}
}

func TestMultiStorage(t *testing.T) {
	// Create two temp directories
	tmpDir1, err := os.MkdirTemp("", "blueprint-test1-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir 1: %v", err)
	}
	defer os.RemoveAll(tmpDir1)

	tmpDir2, err := os.MkdirTemp("", "blueprint-test2-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir 2: %v", err)
	}
	defer os.RemoveAll(tmpDir2)

	storage1, _ := NewLocalStorage(tmpDir1, false)
	storage2, _ := NewLocalStorage(tmpDir2, false)

	multi := NewMultiStorage(storage1, storage2)
	defer multi.Close()

	ctx := context.Background()

	// Add blueprint to storage1
	bp1 := &Blueprint{
		APIVersion: APIVersion,
		Kind:       Kind,
		Metadata:   Metadata{Name: "bp1", Version: "1.0.0"},
	}
	storage1.Put(ctx, bp1)

	// Add blueprint to storage2
	bp2 := &Blueprint{
		APIVersion: APIVersion,
		Kind:       Kind,
		Metadata:   Metadata{Name: "bp2", Version: "1.0.0"},
	}
	storage2.Put(ctx, bp2)

	// Get from storage1 through multi
	got1, err := multi.Get(ctx, "local/bp1", "1.0.0")
	if err != nil {
		t.Fatalf("Get bp1 failed: %v", err)
	}
	if got1.Metadata.Name != "bp1" {
		t.Errorf("Got %s, want bp1", got1.Metadata.Name)
	}

	// Get from storage2 through multi
	got2, err := multi.Get(ctx, "local/bp2", "1.0.0")
	if err != nil {
		t.Fatalf("Get bp2 failed: %v", err)
	}
	if got2.Metadata.Name != "bp2" {
		t.Errorf("Got %s, want bp2", got2.Metadata.Name)
	}

	// List combines both
	all, err := multi.List(ctx, nil)
	if err != nil {
		t.Fatalf("List failed: %v", err)
	}
	if len(all) != 2 {
		t.Errorf("List: got %d, want 2", len(all))
	}

	// Exists checks both
	exists, _ := multi.Exists(ctx, "local/bp1", "1.0.0")
	if !exists {
		t.Error("Exists(bp1) = false, want true")
	}

	exists, _ = multi.Exists(ctx, "local/bp2", "1.0.0")
	if !exists {
		t.Error("Exists(bp2) = false, want true")
	}
}

func TestStorageFactory(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "blueprint-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	factory := NewStorageFactory()

	// Create local storage via factory
	storage, err := factory.Create(&StorageConfig{
		Type: "local",
		Path: tmpDir,
	})
	if err != nil {
		t.Fatalf("Create local storage failed: %v", err)
	}
	defer storage.Close()

	// Verify it works
	_, ok := storage.(*LocalStorage)
	if !ok {
		t.Error("Factory did not create LocalStorage")
	}

	// Unknown type should fail
	_, err = factory.Create(&StorageConfig{Type: "unknown"})
	if err == nil {
		t.Error("Create unknown type should fail")
	}
}

func TestParseBlueprintName(t *testing.T) {
	tests := []struct {
		name       string
		input      string
		wantVendor string
		wantName   string
		wantErr    bool
	}{
		{"with prefix", "blueprints/community/web-app", "community", "web-app", false},
		{"without prefix", "community/web-app", "community", "web-app", false},
		{"invalid single", "web-app", "", "", true},
		{"invalid empty", "", "", "", true},
		{"too many parts", "blueprints/a/b/c", "", "", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			vendor, name, err := parseBlueprintName(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("parseBlueprintName(%s) error = %v, wantErr %v", tt.input, err, tt.wantErr)
				return
			}
			if vendor != tt.wantVendor {
				t.Errorf("vendor = %s, want %s", vendor, tt.wantVendor)
			}
			if name != tt.wantName {
				t.Errorf("name = %s, want %s", name, tt.wantName)
			}
		})
	}
}

func TestParseVersionedDir(t *testing.T) {
	tests := []struct {
		input       string
		wantName    string
		wantVersion string
	}{
		{"web-app-1.0.0", "web-app", "1.0.0"},
		{"web-app-1.2.3-alpha", "web-app", "1.2.3-alpha"},
		{"web-app", "web-app", ""},
		{"my-bp-2.0.0", "my-bp", "2.0.0"},
		{"single", "single", ""},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			name, version := parseVersionedDir(tt.input)
			if name != tt.wantName {
				t.Errorf("name = %s, want %s", name, tt.wantName)
			}
			if version != tt.wantVersion {
				t.Errorf("version = %s, want %s", version, tt.wantVersion)
			}
		})
	}
}

func TestCompareVersions(t *testing.T) {
	tests := []struct {
		a, b string
		want int
	}{
		{"1.0.0", "1.0.0", 0},
		{"1.0.0", "1.0.1", -1},
		{"1.1.0", "1.0.0", 1},
		{"2.0.0", "1.9.9", 1},
		{"1.0.0-alpha", "1.0.0-beta", -1},
		{"1.0.0", "1.0.0-alpha", 1}, // release > prerelease
	}

	for _, tt := range tests {
		t.Run(tt.a+"_vs_"+tt.b, func(t *testing.T) {
			got := compareVersions(tt.a, tt.b)
			if got != tt.want {
				t.Errorf("compareVersions(%s, %s) = %d, want %d", tt.a, tt.b, got, tt.want)
			}
		})
	}
}
