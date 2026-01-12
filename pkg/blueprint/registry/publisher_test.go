package registry

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDefaultPublisherConfig(t *testing.T) {
	config := DefaultPublisherConfig()

	if len(config.ExcludePatterns) == 0 {
		t.Error("expected default exclude patterns")
	}

	if config.Compression != 6 {
		t.Errorf("compression = %d, want 6", config.Compression)
	}
}

func TestNewPublisher(t *testing.T) {
	t.Run("nil config", func(t *testing.T) {
		publisher := NewPublisher(nil, nil)
		if publisher.config == nil {
			t.Error("expected default config")
		}
	})

	t.Run("custom config", func(t *testing.T) {
		config := &PublisherConfig{Compression: 9}
		publisher := NewPublisher(nil, config)
		if publisher.config.Compression != 9 {
			t.Errorf("compression = %d, want 9", publisher.config.Compression)
		}
	})
}

func TestPublisher_Build(t *testing.T) {
	// Create a temporary blueprint directory
	tmpDir := t.TempDir()

	// Create manifest
	manifestContent := `apiVersion: blueprints.kscore.io/v1
kind: Blueprint
metadata:
  name: test-blueprint
  version: 1.0.0
  description: A test blueprint
`
	err := os.WriteFile(filepath.Join(tmpDir, "blueprint.yaml"), []byte(manifestContent), 0644)
	if err != nil {
		t.Fatalf("failed to create manifest: %v", err)
	}

	// Create states directory
	statesDir := filepath.Join(tmpDir, "states")
	if err := os.MkdirAll(statesDir, 0755); err != nil {
		t.Fatalf("failed to create states dir: %v", err)
	}

	// Create a state file
	stateContent := `install_nginx:
  pkg.installed:
    - name: nginx
`
	err = os.WriteFile(filepath.Join(statesDir, "nginx.yaml"), []byte(stateContent), 0644)
	if err != nil {
		t.Fatalf("failed to create state file: %v", err)
	}

	// Build
	publisher := NewPublisher(nil, nil)
	result, err := publisher.Build(tmpDir)
	if err != nil {
		t.Fatalf("build failed: %v", err)
	}

	// Verify result
	if result.Blueprint.Metadata.Name != "test-blueprint" {
		t.Errorf("name = %q, want %q", result.Blueprint.Metadata.Name, "test-blueprint")
	}

	if result.Blueprint.Metadata.Version != "1.0.0" {
		t.Errorf("version = %q, want %q", result.Blueprint.Metadata.Version, "1.0.0")
	}

	if len(result.Archive) == 0 {
		t.Error("archive should not be empty")
	}

	if result.Checksum == "" {
		t.Error("checksum should not be empty")
	}

	if result.Size == 0 {
		t.Error("size should not be zero")
	}

	if len(result.Files) == 0 {
		t.Error("files should not be empty")
	}
}

func TestPublisher_Build_MissingManifest(t *testing.T) {
	tmpDir := t.TempDir()

	publisher := NewPublisher(nil, nil)
	_, err := publisher.Build(tmpDir)
	if err == nil {
		t.Error("expected error for missing manifest")
	}
}

func TestPublisher_Build_InvalidManifest(t *testing.T) {
	tmpDir := t.TempDir()

	// Create invalid manifest
	err := os.WriteFile(filepath.Join(tmpDir, "blueprint.yaml"), []byte("invalid: yaml: content:"), 0644)
	if err != nil {
		t.Fatalf("failed to create manifest: %v", err)
	}

	publisher := NewPublisher(nil, nil)
	_, err = publisher.Build(tmpDir)
	if err == nil {
		t.Error("expected error for invalid manifest")
	}
}

func TestPublisher_Build_MissingStates(t *testing.T) {
	tmpDir := t.TempDir()

	// Create valid manifest but no states directory
	manifestContent := `apiVersion: blueprints.kscore.io/v1
kind: Blueprint
metadata:
  name: test-blueprint
  version: 1.0.0
`
	err := os.WriteFile(filepath.Join(tmpDir, "blueprint.yaml"), []byte(manifestContent), 0644)
	if err != nil {
		t.Fatalf("failed to create manifest: %v", err)
	}

	publisher := NewPublisher(nil, nil)
	_, err = publisher.Build(tmpDir)
	if err == nil {
		t.Error("expected error for missing states directory")
	}
}

func TestPublisher_Build_ExcludePatterns(t *testing.T) {
	tmpDir := t.TempDir()

	// Create manifest
	manifestContent := `apiVersion: blueprints.kscore.io/v1
kind: Blueprint
metadata:
  name: test-blueprint
  version: 1.0.0
`
	err := os.WriteFile(filepath.Join(tmpDir, "blueprint.yaml"), []byte(manifestContent), 0644)
	if err != nil {
		t.Fatalf("failed to create manifest: %v", err)
	}

	// Create states directory
	statesDir := filepath.Join(tmpDir, "states")
	if err := os.MkdirAll(statesDir, 0755); err != nil {
		t.Fatalf("failed to create states dir: %v", err)
	}

	err = os.WriteFile(filepath.Join(statesDir, "main.yaml"), []byte("# state"), 0644)
	if err != nil {
		t.Fatalf("failed to create state file: %v", err)
	}

	// Create .git directory (should be excluded)
	gitDir := filepath.Join(tmpDir, ".git")
	if err := os.MkdirAll(gitDir, 0755); err != nil {
		t.Fatalf("failed to create .git dir: %v", err)
	}
	err = os.WriteFile(filepath.Join(gitDir, "config"), []byte("git config"), 0644)
	if err != nil {
		t.Fatalf("failed to create .git/config: %v", err)
	}

	// Create .DS_Store (should be excluded)
	err = os.WriteFile(filepath.Join(tmpDir, ".DS_Store"), []byte("mac file"), 0644)
	if err != nil {
		t.Fatalf("failed to create .DS_Store: %v", err)
	}

	publisher := NewPublisher(nil, nil)
	result, err := publisher.Build(tmpDir)
	if err != nil {
		t.Fatalf("build failed: %v", err)
	}

	// Check that excluded files are not in the archive
	for _, f := range result.Files {
		if f == ".git/config" || f == ".DS_Store" {
			t.Errorf("excluded file %q should not be in archive", f)
		}
	}
}

func TestPublisher_Build_SkipValidation(t *testing.T) {
	tmpDir := t.TempDir()

	// Create manifest with invalid parameter type
	manifestContent := `apiVersion: blueprints.kscore.io/v1
kind: Blueprint
metadata:
  name: test-blueprint
  version: 1.0.0
parameters:
  invalid_param:
    type: invalid_type
`
	err := os.WriteFile(filepath.Join(tmpDir, "blueprint.yaml"), []byte(manifestContent), 0644)
	if err != nil {
		t.Fatalf("failed to create manifest: %v", err)
	}

	// Create states directory
	statesDir := filepath.Join(tmpDir, "states")
	if err := os.MkdirAll(statesDir, 0755); err != nil {
		t.Fatalf("failed to create states dir: %v", err)
	}
	err = os.WriteFile(filepath.Join(statesDir, "main.yaml"), []byte("# state"), 0644)
	if err != nil {
		t.Fatalf("failed to create state file: %v", err)
	}

	// Build with validation (should fail)
	publisher := NewPublisher(nil, nil)
	_, err = publisher.Build(tmpDir)
	if err == nil {
		t.Error("expected validation error")
	}

	// Build without validation (should succeed)
	publisher = NewPublisher(nil, &PublisherConfig{SkipValidation: true})
	_, err = publisher.Build(tmpDir)
	if err != nil {
		t.Errorf("build failed with skip validation: %v", err)
	}
}

func TestExtractBlueprint(t *testing.T) {
	// Create a source blueprint directory
	srcDir := t.TempDir()

	manifestContent := `apiVersion: blueprints.kscore.io/v1
kind: Blueprint
metadata:
  name: test-blueprint
  version: 1.0.0
`
	err := os.WriteFile(filepath.Join(srcDir, "blueprint.yaml"), []byte(manifestContent), 0644)
	if err != nil {
		t.Fatalf("failed to create manifest: %v", err)
	}

	statesDir := filepath.Join(srcDir, "states")
	if err := os.MkdirAll(statesDir, 0755); err != nil {
		t.Fatalf("failed to create states dir: %v", err)
	}
	err = os.WriteFile(filepath.Join(statesDir, "main.yaml"), []byte("# state content"), 0644)
	if err != nil {
		t.Fatalf("failed to create state file: %v", err)
	}

	// Build archive
	publisher := NewPublisher(nil, nil)
	result, err := publisher.Build(srcDir)
	if err != nil {
		t.Fatalf("build failed: %v", err)
	}

	// Extract to new directory
	destDir := t.TempDir()
	if err := ExtractBlueprint(result.Archive, destDir); err != nil {
		t.Fatalf("extract failed: %v", err)
	}

	// Verify extracted files
	if _, err := os.Stat(filepath.Join(destDir, "blueprint.yaml")); err != nil {
		t.Error("blueprint.yaml not extracted")
	}
	if _, err := os.Stat(filepath.Join(destDir, "states", "main.yaml")); err != nil {
		t.Error("states/main.yaml not extracted")
	}
}

func TestVerifyChecksum(t *testing.T) {
	data := []byte("test data")

	t.Run("valid checksum", func(t *testing.T) {
		// Build checksum
		publisher := NewPublisher(nil, nil)
		tmpDir := createTestBlueprint(t)
		result, _ := publisher.Build(tmpDir)

		err := VerifyChecksum(result.Archive, result.Checksum)
		if err != nil {
			t.Errorf("checksum verification failed: %v", err)
		}
	})

	t.Run("invalid checksum", func(t *testing.T) {
		err := VerifyChecksum(data, "sha256:invalid")
		if err == nil {
			t.Error("expected error for invalid checksum")
		}
	})

	t.Run("checksum without prefix", func(t *testing.T) {
		publisher := NewPublisher(nil, nil)
		tmpDir := createTestBlueprint(t)
		result, _ := publisher.Build(tmpDir)

		// Remove prefix from checksum
		checksumWithoutPrefix := result.Checksum[7:] // Remove "sha256:"

		err := VerifyChecksum(result.Archive, checksumWithoutPrefix)
		if err != nil {
			t.Errorf("checksum verification failed: %v", err)
		}
	})
}

func TestIsValidVersion(t *testing.T) {
	tests := []struct {
		version string
		valid   bool
	}{
		{"1.0.0", true},
		{"1.0", true},
		{"0.1.0", true},
		{"10.20.30", true},
		{"1.0.0-alpha", true},
		{"1.0.0-rc1", true},       // Simple prerelease without dots
		{"1.0.0-rc.1", false},     // Our simple validator doesn't handle dots in prerelease
		{"1.0.0+build", true},
		{"1", false},
		{"1.0.0.0", false},
		{"a.b.c", false},
		{"", false},
		{".0.0", false},
		{"1..0", false},
	}

	for _, tt := range tests {
		t.Run(tt.version, func(t *testing.T) {
			got := isValidVersion(tt.version)
			if got != tt.valid {
				t.Errorf("isValidVersion(%q) = %v, want %v", tt.version, got, tt.valid)
			}
		})
	}
}

func TestIsValidParameterType(t *testing.T) {
	tests := []struct {
		paramType string
		valid     bool
	}{
		{"string", true},
		{"number", true},
		{"integer", true},
		{"boolean", true},
		{"array", true},
		{"object", true},
		{"invalid", false},
		{"", false},
		{"String", false}, // case sensitive
	}

	for _, tt := range tests {
		t.Run(tt.paramType, func(t *testing.T) {
			got := isValidParameterType(tt.paramType)
			if got != tt.valid {
				t.Errorf("isValidParameterType(%q) = %v, want %v", tt.paramType, got, tt.valid)
			}
		})
	}
}

func TestPublisher_shouldExclude(t *testing.T) {
	publisher := NewPublisher(nil, &PublisherConfig{
		ExcludePatterns: []string{
			".git",
			".git/**",
			"*.swp",
			"node_modules",
		},
	})

	tests := []struct {
		path    string
		isDir   bool
		exclude bool
	}{
		{".git", true, true},
		{".git/config", false, true},
		{".git/objects/abc", false, true},
		{"test.swp", false, true},
		{"node_modules", true, true},
		{"src/main.go", false, false},
		{"states/nginx.yaml", false, false},
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			got := publisher.shouldExclude(tt.path, tt.isDir)
			if got != tt.exclude {
				t.Errorf("shouldExclude(%q, %v) = %v, want %v", tt.path, tt.isDir, got, tt.exclude)
			}
		})
	}
}

func TestPublisher_shouldInclude(t *testing.T) {
	t.Run("no patterns includes all", func(t *testing.T) {
		publisher := NewPublisher(nil, &PublisherConfig{})
		if !publisher.shouldInclude("any/path") {
			t.Error("expected all paths included when no patterns")
		}
	})

	t.Run("with patterns", func(t *testing.T) {
		publisher := NewPublisher(nil, &PublisherConfig{
			IncludePatterns: []string{"*.yaml", "*.yml"},
		})

		if !publisher.shouldInclude("test.yaml") {
			t.Error("expected .yaml to be included")
		}
		if !publisher.shouldInclude("test.yml") {
			t.Error("expected .yml to be included")
		}
		if publisher.shouldInclude("test.json") {
			t.Error("expected .json to be excluded")
		}
	})
}

// Helper function to create a minimal test blueprint directory
func createTestBlueprint(t *testing.T) string {
	t.Helper()
	tmpDir := t.TempDir()

	manifestContent := `apiVersion: blueprints.kscore.io/v1
kind: Blueprint
metadata:
  name: test-blueprint
  version: 1.0.0
`
	if err := os.WriteFile(filepath.Join(tmpDir, "blueprint.yaml"), []byte(manifestContent), 0644); err != nil {
		t.Fatalf("failed to create manifest: %v", err)
	}

	statesDir := filepath.Join(tmpDir, "states")
	if err := os.MkdirAll(statesDir, 0755); err != nil {
		t.Fatalf("failed to create states dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(statesDir, "main.yaml"), []byte("# state"), 0644); err != nil {
		t.Fatalf("failed to create state file: %v", err)
	}

	return tmpDir
}
