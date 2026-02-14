package bootstrap

import (
	"os"
	"path/filepath"
	"testing"
)

func setupFakeBuildDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	binDir := filepath.Join(dir, "linux", "amd64")
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	for _, name := range []string{"kscore-server", "kscore-agent", "kscorectl", "kscore-exec"} {
		if err := os.WriteFile(filepath.Join(binDir, name), []byte("binary:"+name), 0o755); err != nil {
			t.Fatalf("write: %v", err)
		}
	}
	return dir
}

func TestCollectBinaries(t *testing.T) {
	buildDir := setupFakeBuildDir(t)
	staging := t.TempDir()
	platform := Platform{OS: "linux", Arch: "amd64"}

	result, err := CollectBinaries(buildDir, platform, "0.1.0", staging)
	if err != nil {
		t.Fatalf("CollectBinaries: %v", err)
	}

	if len(result.Entries) != 4 {
		t.Fatalf("expected 4 entries, got %d", len(result.Entries))
	}
	if len(result.Warnings) != 0 {
		t.Errorf("unexpected warnings: %v", result.Warnings)
	}

	// Verify files were copied
	for _, e := range result.Entries {
		path := filepath.Join(staging, e.Path)
		if _, err := os.Stat(path); err != nil {
			t.Errorf("copied file missing: %s", path)
		}
		if e.SHA256 == "" {
			t.Errorf("entry %s missing SHA256", e.Name)
		}
		if e.Size == 0 {
			t.Errorf("entry %s has zero size", e.Name)
		}
		if e.Version != "0.1.0" {
			t.Errorf("entry %s version = %q, want %q", e.Name, e.Version, "0.1.0")
		}
	}
}

func TestCollectBinaries_MissingCoreBinaries(t *testing.T) {
	dir := t.TempDir()
	binDir := filepath.Join(dir, "linux", "amd64")
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	// Only write one binary, missing the core ones
	if err := os.WriteFile(filepath.Join(binDir, "kscore-exec"), []byte("bin"), 0o755); err != nil {
		t.Fatalf("write: %v", err)
	}

	staging := t.TempDir()
	result, err := CollectBinaries(dir, Platform{OS: "linux", Arch: "amd64"}, "0.1.0", staging)
	if err != nil {
		t.Fatalf("CollectBinaries: %v", err)
	}

	if len(result.Entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(result.Entries))
	}
	if len(result.Warnings) != 3 {
		t.Fatalf("expected 3 warnings for missing core binaries, got %d: %v", len(result.Warnings), result.Warnings)
	}
}

func TestCollectBinaries_NonexistentDir(t *testing.T) {
	staging := t.TempDir()
	_, err := CollectBinaries("/nonexistent", Platform{OS: "linux", Arch: "amd64"}, "0.1.0", staging)
	if err == nil {
		t.Fatal("expected error for nonexistent build dir")
	}
}

func TestCollectBinaries_SkipsNonBinaries(t *testing.T) {
	dir := t.TempDir()
	binDir := filepath.Join(dir, "linux", "amd64")
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	// Write a non-kscore file
	os.WriteFile(filepath.Join(binDir, "README.md"), []byte("readme"), 0o644)
	os.WriteFile(filepath.Join(binDir, "some-other-tool"), []byte("tool"), 0o755)
	os.WriteFile(filepath.Join(binDir, "kscorectl"), []byte("cli"), 0o755)

	staging := t.TempDir()
	result, err := CollectBinaries(dir, Platform{OS: "linux", Arch: "amd64"}, "0.1.0", staging)
	if err != nil {
		t.Fatalf("CollectBinaries: %v", err)
	}

	if len(result.Entries) != 1 {
		t.Fatalf("expected 1 entry (kscorectl only), got %d", len(result.Entries))
	}
	if result.Entries[0].Name != "kscorectl" {
		t.Errorf("expected kscorectl, got %s", result.Entries[0].Name)
	}
}

func TestCollectBinaries_SkipsDirectories(t *testing.T) {
	dir := t.TempDir()
	binDir := filepath.Join(dir, "linux", "amd64")
	os.MkdirAll(filepath.Join(binDir, "kscore-subdir"), 0o755)
	os.WriteFile(filepath.Join(binDir, "kscorectl"), []byte("cli"), 0o755)

	staging := t.TempDir()
	result, err := CollectBinaries(dir, Platform{OS: "linux", Arch: "amd64"}, "0.1.0", staging)
	if err != nil {
		t.Fatalf("CollectBinaries: %v", err)
	}

	if len(result.Entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(result.Entries))
	}
}
