package upgrade

import (
	"os"
	"path/filepath"
	"testing"
)

func setupScanDirs(t *testing.T) (oldDir, newDir string) {
	t.Helper()
	oldDir = t.TempDir()
	newDir = t.TempDir()

	// Old version binaries
	os.WriteFile(filepath.Join(oldDir, "kscore-server"), []byte("server-v1"), 0o755)
	os.WriteFile(filepath.Join(oldDir, "kscore-agent"), []byte("agent-v1"), 0o755)
	os.WriteFile(filepath.Join(oldDir, "kscorectl"), []byte("ctl-v1"), 0o755)

	// New version binaries (server changed, agent same, ctl removed, new binary added)
	os.WriteFile(filepath.Join(newDir, "kscore-server"), []byte("server-v2-changed"), 0o755)
	os.WriteFile(filepath.Join(newDir, "kscore-agent"), []byte("agent-v1"), 0o755)
	os.WriteFile(filepath.Join(newDir, "kscore-monitor"), []byte("monitor-new"), 0o755)

	return oldDir, newDir
}

func TestScanChanges_Binaries(t *testing.T) {
	oldDir, newDir := setupScanDirs(t)

	result, err := ScanChanges(oldDir, newDir)
	if err != nil {
		t.Fatalf("ScanChanges: %v", err)
	}

	if len(result.ChangedBinaries) != 1 || result.ChangedBinaries[0] != "kscore-server" {
		t.Errorf("ChangedBinaries = %v, want [kscore-server]", result.ChangedBinaries)
	}
	if len(result.NewBinaries) != 1 || result.NewBinaries[0] != "kscore-monitor" {
		t.Errorf("NewBinaries = %v, want [kscore-monitor]", result.NewBinaries)
	}
	if len(result.RemovedBinaries) != 1 || result.RemovedBinaries[0] != "kscorectl" {
		t.Errorf("RemovedBinaries = %v, want [kscorectl]", result.RemovedBinaries)
	}
}

func TestScanChanges_Modules(t *testing.T) {
	oldDir := t.TempDir()
	newDir := t.TempDir()

	// Add some binaries (required for both dirs)
	os.WriteFile(filepath.Join(oldDir, "kscore-server"), []byte("bin"), 0o755)
	os.WriteFile(filepath.Join(newDir, "kscore-server"), []byte("bin"), 0o755)

	// Old modules
	oldModDir := filepath.Join(oldDir, "modules")
	os.MkdirAll(oldModDir, 0o755)
	os.WriteFile(filepath.Join(oldModDir, "std-files.zip"), []byte("files-v1"), 0o644)
	os.WriteFile(filepath.Join(oldModDir, "std-core.zip"), []byte("core-v1"), 0o644)

	// New modules (files changed, core same, dns new)
	newModDir := filepath.Join(newDir, "modules")
	os.MkdirAll(newModDir, 0o755)
	os.WriteFile(filepath.Join(newModDir, "std-files.zip"), []byte("files-v2-updated"), 0o644)
	os.WriteFile(filepath.Join(newModDir, "std-core.zip"), []byte("core-v1"), 0o644)
	os.WriteFile(filepath.Join(newModDir, "vendor-dns.zip"), []byte("dns-new"), 0o644)

	result, err := ScanChanges(oldDir, newDir)
	if err != nil {
		t.Fatalf("ScanChanges: %v", err)
	}

	if len(result.ChangedModules) != 1 || result.ChangedModules[0] != "std-files.zip" {
		t.Errorf("ChangedModules = %v, want [std-files.zip]", result.ChangedModules)
	}
	if len(result.NewModules) != 1 || result.NewModules[0] != "vendor-dns.zip" {
		t.Errorf("NewModules = %v, want [vendor-dns.zip]", result.NewModules)
	}
}

func TestScanChanges_Migrations(t *testing.T) {
	oldDir := t.TempDir()
	newDir := t.TempDir()

	os.WriteFile(filepath.Join(oldDir, "bin"), []byte("x"), 0o755)
	os.WriteFile(filepath.Join(newDir, "bin"), []byte("x"), 0o755)

	migDir := filepath.Join(newDir, "migrations")
	os.MkdirAll(migDir, 0o755)
	os.WriteFile(filepath.Join(migDir, "001.sql"), []byte("CREATE TABLE"), 0o644)
	os.WriteFile(filepath.Join(migDir, "002.sql"), []byte("ALTER TABLE"), 0o644)

	result, err := ScanChanges(oldDir, newDir)
	if err != nil {
		t.Fatalf("ScanChanges: %v", err)
	}

	if len(result.MigrationFiles) != 2 {
		t.Errorf("MigrationFiles count = %d, want 2", len(result.MigrationFiles))
	}
}

func TestScanChanges_NoChanges(t *testing.T) {
	dir1 := t.TempDir()
	dir2 := t.TempDir()
	os.WriteFile(filepath.Join(dir1, "kscore-server"), []byte("same"), 0o755)
	os.WriteFile(filepath.Join(dir2, "kscore-server"), []byte("same"), 0o755)

	result, err := ScanChanges(dir1, dir2)
	if err != nil {
		t.Fatalf("ScanChanges: %v", err)
	}
	if result.HasChanges() {
		t.Error("expected no changes")
	}
	if result.Summary() != "no changes" {
		t.Errorf("Summary = %q, want 'no changes'", result.Summary())
	}
}

func TestScanChanges_HasChanges(t *testing.T) {
	oldDir, newDir := setupScanDirs(t)
	result, err := ScanChanges(oldDir, newDir)
	if err != nil {
		t.Fatal(err)
	}
	if !result.HasChanges() {
		t.Error("expected changes")
	}
	summary := result.Summary()
	if summary == "" || summary == "no changes" {
		t.Errorf("unexpected summary: %q", summary)
	}
}

func TestScanChanges_EmptyDirs(t *testing.T) {
	dir1 := t.TempDir()
	dir2 := t.TempDir()

	result, err := ScanChanges(dir1, dir2)
	if err != nil {
		t.Fatalf("ScanChanges: %v", err)
	}
	if result.HasChanges() {
		t.Error("expected no changes for empty dirs")
	}
}

func TestScanChanges_MissingDir(t *testing.T) {
	_, err := ScanChanges("/nonexistent", t.TempDir())
	if err == nil {
		t.Error("expected error for missing directory")
	}
}
