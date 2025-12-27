package starlark

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDefaultTemplate(t *testing.T) {
	template := DefaultTemplate("test-module")

	if template.Name != "test-module" {
		t.Errorf("Name = %s, want test-module", template.Name)
	}

	if template.Version != "0.1.0" {
		t.Errorf("Version = %s, want 0.1.0", template.Version)
	}

	if template.Description != "A Keystone Core module" {
		t.Errorf("Description = %s, want 'A Keystone Core module'", template.Description)
	}
}

func TestModuleTemplate_Generate(t *testing.T) {
	tmpDir := t.TempDir()

	template := &ModuleTemplate{
		Name:         "test-module",
		Description:  "Test module description",
		Version:      "1.0.0",
		Author:       "Test Author",
		Capabilities: []string{"fs.read", "fs.write"},
		Dependencies: map[string]string{
			"std/files": "^1.0.0",
		},
	}

	err := template.Generate(tmpDir)
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}

	moduleDir := filepath.Join(tmpDir, "test-module")

	// Check directory structure
	expectedDirs := []string{
		moduleDir,
		filepath.Join(moduleDir, "states"),
		filepath.Join(moduleDir, "tests"),
	}

	for _, dir := range expectedDirs {
		if _, err := os.Stat(dir); os.IsNotExist(err) {
			t.Errorf("Expected directory not created: %s", dir)
		}
	}

	// Check files
	expectedFiles := []string{
		filepath.Join(moduleDir, "module.yaml"),
		filepath.Join(moduleDir, "states", "main.star"),
		filepath.Join(moduleDir, "tests", "main_test.star"),
		filepath.Join(moduleDir, "README.md"),
	}

	for _, file := range expectedFiles {
		if _, err := os.Stat(file); os.IsNotExist(err) {
			t.Errorf("Expected file not created: %s", file)
		}
	}

	// Check manifest content
	manifestPath := filepath.Join(moduleDir, "module.yaml")
	content, err := os.ReadFile(manifestPath)
	if err != nil {
		t.Fatalf("Failed to read manifest: %v", err)
	}

	manifestStr := string(content)
	expectedContents := []string{
		"name: test-module",
		"version: 1.0.0",
		"description: Test module description",
		"fs.read",
		"fs.write",
		"std/files: ^1.0.0",
		"author: Test Author",
	}

	for _, expected := range expectedContents {
		if !contains(manifestStr, expected) {
			t.Errorf("Manifest missing expected content: %s", expected)
		}
	}
}

func TestModuleTemplate_GenerateNoCapabilities(t *testing.T) {
	tmpDir := t.TempDir()

	template := &ModuleTemplate{
		Name:        "simple-module",
		Description: "Simple module",
		Version:     "1.0.0",
	}

	err := template.Generate(tmpDir)
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}

	manifestPath := filepath.Join(tmpDir, "simple-module", "module.yaml")
	content, err := os.ReadFile(manifestPath)
	if err != nil {
		t.Fatalf("Failed to read manifest: %v", err)
	}

	manifestStr := string(content)
	if !contains(manifestStr, "[] # No capabilities required") {
		t.Error("Manifest should indicate no capabilities required")
	}

	if !contains(manifestStr, "{} # No dependencies") {
		t.Error("Manifest should indicate no dependencies")
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > len(substr) && findSubstring(s, substr))
}

func findSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
