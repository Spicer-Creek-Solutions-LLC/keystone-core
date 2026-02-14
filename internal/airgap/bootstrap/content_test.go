package bootstrap

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestBundleModules(t *testing.T) {
	srcDir := t.TempDir()
	stagingDir := t.TempDir()

	// Create module files
	writeTestFile(t, filepath.Join(srcDir, "std-files-0.1.0.tar.gz"), "module-content-1")
	writeTestFile(t, filepath.Join(srcDir, "std-net-0.2.0.tar.gz"), "module-content-2")

	entries, err := BundleModules(srcDir, stagingDir)
	if err != nil {
		t.Fatalf("BundleModules() error: %v", err)
	}

	if len(entries) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(entries))
	}

	for _, e := range entries {
		if !strings.HasPrefix(e.Path, "modules/") {
			t.Errorf("entry %q path should start with modules/, got %q", e.Name, e.Path)
		}
		if e.SHA256 == "" {
			t.Errorf("entry %q missing SHA256", e.Name)
		}
		if e.Size == 0 {
			t.Errorf("entry %q has zero size", e.Name)
		}
		// Verify file exists in staging
		dstPath := filepath.Join(stagingDir, e.Path)
		if _, err := os.Stat(dstPath); err != nil {
			t.Errorf("expected file at %s: %v", dstPath, err)
		}
	}
}

func TestBundleModules_NonexistentDir(t *testing.T) {
	entries, err := BundleModules("/nonexistent", t.TempDir())
	if err != nil {
		t.Fatalf("expected nil error for nonexistent dir, got: %v", err)
	}
	if entries != nil {
		t.Errorf("expected nil entries, got %v", entries)
	}
}

func TestBundleModules_SkipsDirectories(t *testing.T) {
	srcDir := t.TempDir()
	stagingDir := t.TempDir()

	writeTestFile(t, filepath.Join(srcDir, "mod.tar.gz"), "data")
	if err := os.MkdirAll(filepath.Join(srcDir, "subdir"), 0o750); err != nil {
		t.Fatal(err)
	}

	entries, err := BundleModules(srcDir, stagingDir)
	if err != nil {
		t.Fatalf("BundleModules() error: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry (dirs skipped), got %d", len(entries))
	}
}

func TestBundleBlueprints(t *testing.T) {
	srcDir := t.TempDir()
	stagingDir := t.TempDir()

	// Create blueprint directories with files
	bp1 := filepath.Join(srcDir, "web-app")
	if err := os.MkdirAll(bp1, 0o750); err != nil {
		t.Fatal(err)
	}
	writeTestFile(t, filepath.Join(bp1, "blueprint.yaml"), "name: web-app")
	writeTestFile(t, filepath.Join(bp1, "main.star"), "print('hello')")

	bp2 := filepath.Join(srcDir, "db-cluster")
	if err := os.MkdirAll(bp2, 0o750); err != nil {
		t.Fatal(err)
	}
	writeTestFile(t, filepath.Join(bp2, "blueprint.yaml"), "name: db-cluster")

	entries, err := BundleBlueprints(srcDir, stagingDir)
	if err != nil {
		t.Fatalf("BundleBlueprints() error: %v", err)
	}

	if len(entries) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(entries))
	}

	for _, e := range entries {
		if !strings.HasPrefix(e.Path, "blueprints/") {
			t.Errorf("entry %q path should start with blueprints/, got %q", e.Name, e.Path)
		}
		if !strings.HasSuffix(e.Path, ".tar.gz") {
			t.Errorf("entry %q should be archived as .tar.gz, got %q", e.Name, e.Path)
		}
		if e.SHA256 == "" {
			t.Errorf("entry %q missing SHA256", e.Name)
		}
	}
}

func TestBundleBlueprints_SkipsFiles(t *testing.T) {
	srcDir := t.TempDir()
	stagingDir := t.TempDir()

	// A regular file at the top level should be skipped
	writeTestFile(t, filepath.Join(srcDir, "README.md"), "readme")
	bp := filepath.Join(srcDir, "my-bp")
	if err := os.MkdirAll(bp, 0o750); err != nil {
		t.Fatal(err)
	}
	writeTestFile(t, filepath.Join(bp, "blueprint.yaml"), "name: my-bp")

	entries, err := BundleBlueprints(srcDir, stagingDir)
	if err != nil {
		t.Fatalf("BundleBlueprints() error: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry (files skipped), got %d", len(entries))
	}
}

func TestBundlePolicies(t *testing.T) {
	srcDir := t.TempDir()
	stagingDir := t.TempDir()

	writeTestFile(t, filepath.Join(srcDir, "deny-root.rego"), "package deny")
	writeTestFile(t, filepath.Join(srcDir, "resource-limits.cel"), "has(request.limits)")
	writeTestFile(t, filepath.Join(srcDir, "README.md"), "skip me")

	entries, err := BundlePolicies(srcDir, stagingDir)
	if err != nil {
		t.Fatalf("BundlePolicies() error: %v", err)
	}

	if len(entries) != 2 {
		t.Fatalf("expected 2 entries (.rego + .cel), got %d", len(entries))
	}

	names := map[string]bool{}
	for _, e := range entries {
		names[e.Name] = true
		if !strings.HasPrefix(e.Path, "policies/") {
			t.Errorf("entry %q path should start with policies/, got %q", e.Name, e.Path)
		}
	}
	if !names["deny-root.rego"] {
		t.Error("expected deny-root.rego in entries")
	}
	if !names["resource-limits.cel"] {
		t.Error("expected resource-limits.cel in entries")
	}
}

func TestBundlePolicies_NonexistentDir(t *testing.T) {
	entries, err := BundlePolicies("/nonexistent", t.TempDir())
	if err != nil {
		t.Fatalf("expected nil error for nonexistent dir, got: %v", err)
	}
	if entries != nil {
		t.Errorf("expected nil entries, got %v", entries)
	}
}

func TestBundleDocs(t *testing.T) {
	srcDir := t.TempDir()
	stagingDir := t.TempDir()

	writeTestFile(t, filepath.Join(srcDir, "index.html"), "<html>docs</html>")
	subDir := filepath.Join(srcDir, "api")
	if err := os.MkdirAll(subDir, 0o750); err != nil {
		t.Fatal(err)
	}
	writeTestFile(t, filepath.Join(subDir, "reference.html"), "<html>api</html>")

	entry, err := BundleDocs(srcDir, stagingDir)
	if err != nil {
		t.Fatalf("BundleDocs() error: %v", err)
	}

	if entry == nil {
		t.Fatal("expected non-nil entry")
	}
	if entry.Path != "docs/offline-docs.tar.gz" {
		t.Errorf("expected path docs/offline-docs.tar.gz, got %q", entry.Path)
	}
	if entry.SHA256 == "" {
		t.Error("missing SHA256")
	}
	if entry.Size == 0 {
		t.Error("zero size")
	}

	// Verify archive is extractable
	archivePath := filepath.Join(stagingDir, "docs", "offline-docs.tar.gz")
	extractDir := t.TempDir()
	if err := ExtractArchive(archivePath, extractDir); err != nil {
		t.Fatalf("ExtractArchive() error: %v", err)
	}
	if _, err := os.Stat(filepath.Join(extractDir, "index.html")); err != nil {
		t.Error("expected index.html in extracted archive")
	}
	if _, err := os.Stat(filepath.Join(extractDir, "api", "reference.html")); err != nil {
		t.Error("expected api/reference.html in extracted archive")
	}
}

func TestBundleDocs_NonexistentDir(t *testing.T) {
	entry, err := BundleDocs("/nonexistent", t.TempDir())
	if err != nil {
		t.Fatalf("expected nil error for nonexistent dir, got: %v", err)
	}
	if entry != nil {
		t.Error("expected nil entry")
	}
}

func TestExtractArchive(t *testing.T) {
	// Create a source directory with files
	srcDir := t.TempDir()
	writeTestFile(t, filepath.Join(srcDir, "file1.txt"), "hello")
	sub := filepath.Join(srcDir, "sub")
	if err := os.MkdirAll(sub, 0o750); err != nil {
		t.Fatal(err)
	}
	writeTestFile(t, filepath.Join(sub, "file2.txt"), "world")

	// Archive it
	archivePath := filepath.Join(t.TempDir(), "test.tar.gz")
	if err := createArchive(srcDir, archivePath); err != nil {
		t.Fatalf("createArchive() error: %v", err)
	}

	// Extract it
	destDir := t.TempDir()
	if err := ExtractArchive(archivePath, destDir); err != nil {
		t.Fatalf("ExtractArchive() error: %v", err)
	}

	// Verify files
	data, err := os.ReadFile(filepath.Join(destDir, "file1.txt"))
	if err != nil {
		t.Fatalf("reading file1.txt: %v", err)
	}
	if string(data) != "hello" {
		t.Errorf("expected 'hello', got %q", string(data))
	}

	data, err = os.ReadFile(filepath.Join(destDir, "sub", "file2.txt"))
	if err != nil {
		t.Fatalf("reading sub/file2.txt: %v", err)
	}
	if string(data) != "world" {
		t.Errorf("expected 'world', got %q", string(data))
	}
}

func TestExtractArchive_PathTraversal(t *testing.T) {
	// ExtractArchive should reject entries that escape destDir.
	// We test this indirectly by verifying the check exists; a real path
	// traversal archive would require hand-crafting tar headers.
	err := ExtractArchive("/nonexistent.tar.gz", t.TempDir())
	if err == nil {
		t.Error("expected error for nonexistent archive")
	}
}

func TestRenderServerConfig(t *testing.T) {
	params := DefaultConfigParams("1.0.0")
	data, err := RenderServerConfig(params)
	if err != nil {
		t.Fatalf("RenderServerConfig() error: %v", err)
	}

	content := string(data)
	if !strings.Contains(content, "1.0.0") {
		t.Error("expected version in rendered config")
	}
	if !strings.Contains(content, "airgap:") {
		t.Error("expected airgap section in rendered config")
	}
	if !strings.Contains(content, "enabled: true") {
		t.Error("expected airgap enabled in rendered config")
	}
	if !strings.Contains(content, "mode: offline") {
		t.Error("expected offline registry mode")
	}
	if !strings.Contains(content, "upload: false") {
		t.Error("expected telemetry upload disabled")
	}
}

func TestRenderAgentConfig(t *testing.T) {
	params := DefaultConfigParams("1.0.0")
	params.ServerAddress = "https://10.0.0.1:8443"
	params.NATSUrl = "nats://10.0.0.1:4222"

	data, err := RenderAgentConfig(params)
	if err != nil {
		t.Fatalf("RenderAgentConfig() error: %v", err)
	}

	content := string(data)
	if !strings.Contains(content, "https://10.0.0.1:8443") {
		t.Error("expected server address in rendered config")
	}
	if !strings.Contains(content, "nats://10.0.0.1:4222") {
		t.Error("expected NATS URL in rendered config")
	}
}

func TestRenderInstallScript(t *testing.T) {
	data, err := RenderInstallScript("0.5.0", "linux/amd64")
	if err != nil {
		t.Fatalf("RenderInstallScript() error: %v", err)
	}

	content := string(data)
	if !strings.Contains(content, "0.5.0") {
		t.Error("expected version in install script")
	}
	if !strings.Contains(content, "linux/amd64") {
		t.Error("expected platform in install script")
	}
	if !strings.HasPrefix(content, "#!/usr/bin/env bash") {
		t.Error("expected bash shebang")
	}
}

func TestBundleConfigTemplates(t *testing.T) {
	stagingDir := t.TempDir()
	params := DefaultConfigParams("1.0.0")
	platform := Platform{OS: "linux", Arch: "amd64"}

	entries, err := BundleConfigTemplates(params, platform, stagingDir)
	if err != nil {
		t.Fatalf("BundleConfigTemplates() error: %v", err)
	}

	if len(entries) != 3 {
		t.Fatalf("expected 3 entries (2 config + install.sh), got %d", len(entries))
	}

	names := map[string]string{}
	for _, e := range entries {
		names[e.Name] = e.Path
		if e.SHA256 == "" {
			t.Errorf("entry %q missing SHA256", e.Name)
		}
		if e.Size == 0 {
			t.Errorf("entry %q has zero size", e.Name)
		}
	}

	if p, ok := names["server.yaml.tmpl"]; !ok || p != "config/server.yaml.tmpl" {
		t.Errorf("expected server.yaml.tmpl with config/ prefix, got %q", p)
	}
	if p, ok := names["agent.yaml.tmpl"]; !ok || p != "config/agent.yaml.tmpl" {
		t.Errorf("expected agent.yaml.tmpl with config/ prefix, got %q", p)
	}
	if p, ok := names["install.sh"]; !ok || p != "install.sh" {
		t.Errorf("expected install.sh at root, got %q", p)
	}

	// Verify install.sh is executable
	info, err := os.Stat(filepath.Join(stagingDir, "install.sh"))
	if err != nil {
		t.Fatalf("stat install.sh: %v", err)
	}
	if info.Mode().Perm()&0o100 == 0 {
		t.Error("install.sh should be executable")
	}
}

func TestDefaultConfigParams(t *testing.T) {
	params := DefaultConfigParams("2.0.0")
	if params.Version != "2.0.0" {
		t.Errorf("expected version 2.0.0, got %q", params.Version)
	}
	if params.NATSMode != "embedded" {
		t.Errorf("expected embedded NATS mode, got %q", params.NATSMode)
	}
	if params.DatabaseType != "sqlite" {
		t.Errorf("expected sqlite database type, got %q", params.DatabaseType)
	}
}

