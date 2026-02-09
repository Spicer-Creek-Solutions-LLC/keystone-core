package repogen

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestNewGenerator(t *testing.T) {
	config := &Config{
		Version:   "0.1.0",
		OutputDir: "/tmp/test-repos",
	}

	gen := NewGenerator(config)

	if gen.config.BlueprintsDir == "" {
		t.Error("expected default BlueprintsDir")
	}
	if gen.config.ModulesDir == "" {
		t.Error("expected default ModulesDir")
	}
}

func TestGenerateDNF(t *testing.T) {
	tmpDir := t.TempDir()

	config := &Config{
		Version:   "0.1.0",
		OutputDir: tmpDir,
	}

	gen := NewGenerator(config)
	if err := gen.GenerateDNF(); err != nil {
		t.Fatalf("GenerateDNF failed: %v", err)
	}

	// Check directory structure
	dnfConfig := DefaultDNFConfig()
	for _, dist := range dnfConfig.Distributions {
		for _, arch := range dnfConfig.Architectures {
			packagesDir := filepath.Join(tmpDir, "dnf", dist, arch, "Packages")
			if _, err := os.Stat(packagesDir); os.IsNotExist(err) {
				t.Errorf("expected Packages dir: %s", packagesDir)
			}

			repodataDir := filepath.Join(tmpDir, "dnf", dist, arch, "repodata")
			if _, err := os.Stat(repodataDir); os.IsNotExist(err) {
				t.Errorf("expected repodata dir: %s", repodataDir)
			}

			// Check repomd.xml exists
			repomd := filepath.Join(repodataDir, "repomd.xml")
			if _, err := os.Stat(repomd); os.IsNotExist(err) {
				t.Errorf("expected repomd.xml: %s", repomd)
			}

			// Check .repo file exists
			repoFile := filepath.Join(tmpDir, "dnf", dist, arch, "keystonecore.repo")
			if _, err := os.Stat(repoFile); os.IsNotExist(err) {
				t.Errorf("expected .repo file: %s", repoFile)
			}
		}
	}
}

func TestGenerateAPT(t *testing.T) {
	tmpDir := t.TempDir()

	config := &Config{
		Version:   "0.1.0",
		OutputDir: tmpDir,
	}

	gen := NewGenerator(config)
	if err := gen.GenerateAPT(); err != nil {
		t.Fatalf("GenerateAPT failed: %v", err)
	}

	// Check pool directory
	poolDir := filepath.Join(tmpDir, "apt", "pool", "main", "k", "keystonecore")
	if _, err := os.Stat(poolDir); os.IsNotExist(err) {
		t.Errorf("expected pool dir: %s", poolDir)
	}

	// Check dist directories
	aptConfig := DefaultAPTConfig()
	for _, dist := range aptConfig.Distributions {
		releaseFile := filepath.Join(tmpDir, "apt", "dists", dist, "Release")
		if _, err := os.Stat(releaseFile); os.IsNotExist(err) {
			t.Errorf("expected Release file: %s", releaseFile)
		}

		for _, arch := range aptConfig.Architectures {
			packagesFile := filepath.Join(tmpDir, "apt", "dists", dist, "main", "binary-"+arch, "Packages")
			if _, err := os.Stat(packagesFile); os.IsNotExist(err) {
				t.Errorf("expected Packages file: %s", packagesFile)
			}
		}
	}

	// Check sources.list example
	sourcesList := filepath.Join(tmpDir, "apt", "keystonecore.list")
	if _, err := os.Stat(sourcesList); os.IsNotExist(err) {
		t.Error("expected keystonecore.list")
	}
}

func TestGenerateWindows(t *testing.T) {
	tmpDir := t.TempDir()

	config := &Config{
		Version:   "0.1.0",
		OutputDir: tmpDir,
	}

	gen := NewGenerator(config)
	if err := gen.GenerateWindows(); err != nil {
		t.Fatalf("GenerateWindows failed: %v", err)
	}

	// Check architecture directories
	winConfig := DefaultWindowsConfig()
	for _, arch := range winConfig.Architectures {
		archDir := filepath.Join(tmpDir, "windows", arch)
		if _, err := os.Stat(archDir); os.IsNotExist(err) {
			t.Errorf("expected arch dir: %s", archDir)
		}

		// Check manifest.json
		manifestFile := filepath.Join(archDir, "manifest.json")
		data, err := os.ReadFile(manifestFile)
		if err != nil {
			t.Errorf("read manifest.json: %v", err)
			continue
		}

		var manifest map[string]interface{}
		if err := json.Unmarshal(data, &manifest); err != nil {
			t.Errorf("unmarshal manifest: %v", err)
			continue
		}

		if manifest["version"] != "0.1.0" {
			t.Errorf("expected version 0.1.0, got %v", manifest["version"])
		}

		// Check install.ps1
		installScript := filepath.Join(archDir, "install.ps1")
		if _, err := os.Stat(installScript); os.IsNotExist(err) {
			t.Errorf("expected install.ps1: %s", installScript)
		}
	}
}

func TestGenerateBlueprints(t *testing.T) {
	tmpDir := t.TempDir()

	// Create a test blueprint
	blueprintsDir := filepath.Join(tmpDir, "blueprints-src", "test-blueprint")
	if err := os.MkdirAll(blueprintsDir, 0755); err != nil {
		t.Fatalf("create blueprints dir: %v", err)
	}

	manifest := `apiVersion: blueprints.keystone-core.io/v1
kind: Blueprint
metadata:
  name: test-blueprint
  version: 0.1.0
`
	if err := os.WriteFile(filepath.Join(blueprintsDir, "blueprint.yaml"), []byte(manifest), 0644); err != nil {
		t.Fatalf("write manifest: %v", err)
	}

	config := &Config{
		Version:       "0.1.0",
		OutputDir:     filepath.Join(tmpDir, "output"),
		BlueprintsDir: filepath.Join(tmpDir, "blueprints-src"),
	}

	gen := NewGenerator(config)
	if err := gen.GenerateBlueprints(); err != nil {
		t.Fatalf("GenerateBlueprints failed: %v", err)
	}

	// Check index.json
	indexFile := filepath.Join(tmpDir, "output", "blueprints", "index.json")
	data, err := os.ReadFile(indexFile)
	if err != nil {
		t.Fatalf("read index.json: %v", err)
	}

	var index map[string]interface{}
	if err := json.Unmarshal(data, &index); err != nil {
		t.Fatalf("unmarshal index: %v", err)
	}

	if index["count"].(float64) != 1 {
		t.Errorf("expected count 1, got %v", index["count"])
	}

	// Check blueprint directory structure
	listFile := filepath.Join(tmpDir, "output", "blueprints", "kscore", "test-blueprint", "@v", "list")
	if _, err := os.Stat(listFile); os.IsNotExist(err) {
		t.Error("expected list file")
	}

	infoFile := filepath.Join(tmpDir, "output", "blueprints", "kscore", "test-blueprint", "@v", "0.1.0.info")
	if _, err := os.Stat(infoFile); os.IsNotExist(err) {
		t.Error("expected .info file")
	}
}

func TestGenerateModules(t *testing.T) {
	tmpDir := t.TempDir()

	config := &Config{
		Version:   "0.1.0",
		OutputDir: tmpDir,
	}

	gen := NewGenerator(config)
	if err := gen.GenerateModules(); err != nil {
		t.Fatalf("GenerateModules failed: %v", err)
	}

	// Check index.json
	indexFile := filepath.Join(tmpDir, "modules", "index.json")
	if _, err := os.Stat(indexFile); os.IsNotExist(err) {
		t.Error("expected index.json")
	}

	// Check README
	readme := filepath.Join(tmpDir, "modules", "README.md")
	if _, err := os.Stat(readme); os.IsNotExist(err) {
		t.Error("expected README.md")
	}
}

func TestGenerateAll(t *testing.T) {
	tmpDir := t.TempDir()

	// Create minimal blueprints source
	blueprintsDir := filepath.Join(tmpDir, "blueprints-src")
	if err := os.MkdirAll(blueprintsDir, 0755); err != nil {
		t.Fatalf("create blueprints dir: %v", err)
	}

	config := &Config{
		Version:       "0.1.0",
		OutputDir:     filepath.Join(tmpDir, "repos"),
		BlueprintsDir: blueprintsDir,
	}

	gen := NewGenerator(config)
	if err := gen.GenerateAll(); err != nil {
		t.Fatalf("GenerateAll failed: %v", err)
	}

	// Check all repositories exist
	repos := []string{"dnf", "apt", "windows", "blueprints", "modules"}
	for _, repo := range repos {
		repoDir := filepath.Join(tmpDir, "repos", repo)
		if _, err := os.Stat(repoDir); os.IsNotExist(err) {
			t.Errorf("expected repo dir: %s", repoDir)
		}
	}

	// Check master index.json
	indexFile := filepath.Join(tmpDir, "repos", "index.json")
	if _, err := os.Stat(indexFile); os.IsNotExist(err) {
		t.Error("expected master index.json")
	}

	// Check root README
	readme := filepath.Join(tmpDir, "repos", "README.md")
	if _, err := os.Stat(readme); os.IsNotExist(err) {
		t.Error("expected root README.md")
	}
}

func TestDefaultConfigs(t *testing.T) {
	dnfConfig := DefaultDNFConfig()
	if len(dnfConfig.Distributions) == 0 {
		t.Error("expected DNF distributions")
	}
	if len(dnfConfig.Architectures) == 0 {
		t.Error("expected DNF architectures")
	}

	aptConfig := DefaultAPTConfig()
	if len(aptConfig.Distributions) == 0 {
		t.Error("expected APT distributions")
	}
	if aptConfig.Origin == "" {
		t.Error("expected APT origin")
	}

	winConfig := DefaultWindowsConfig()
	if len(winConfig.Architectures) == 0 {
		t.Error("expected Windows architectures")
	}
}

func TestKeystonePackages(t *testing.T) {
	packages := KeystonePackages("0.1.0")

	if len(packages) < 3 {
		t.Errorf("expected at least 3 packages, got %d", len(packages))
	}

	// Check expected packages exist
	expectedPackages := map[string]bool{
		"kscore-server": false,
		"kscore-agent":  false,
		"kscorectl":     false,
	}

	for _, pkg := range packages {
		if _, ok := expectedPackages[pkg.Name]; ok {
			expectedPackages[pkg.Name] = true
		}
		if pkg.Version != "0.1.0" {
			t.Errorf("expected version 0.1.0 for %s, got %s", pkg.Name, pkg.Version)
		}
	}

	for name, found := range expectedPackages {
		if !found {
			t.Errorf("expected package %s not found", name)
		}
	}
}

func TestStringHelpers(t *testing.T) {
	// Test splitLines
	lines := splitLines("line1\nline2\nline3")
	if len(lines) != 3 {
		t.Errorf("expected 3 lines, got %d", len(lines))
	}

	// Test splitOnColon
	parts := splitOnColon("key: value")
	if len(parts) != 2 {
		t.Errorf("expected 2 parts, got %d", len(parts))
	}
	if parts[0] != "key" {
		t.Errorf("expected 'key', got '%s'", parts[0])
	}

	// Test trim
	if trim("  hello  ") != "hello" {
		t.Errorf("trim failed")
	}

	// Test trimQuotes
	if trimQuotes("\"hello\"") != "hello" {
		t.Errorf("trimQuotes double quotes failed")
	}
	if trimQuotes("'hello'") != "hello" {
		t.Errorf("trimQuotes single quotes failed")
	}
	if trimQuotes("hello") != "hello" {
		t.Errorf("trimQuotes no quotes failed")
	}
}
