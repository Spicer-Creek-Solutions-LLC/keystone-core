package registry

import (
	"os"
	"path/filepath"
	"testing"
)

// TestOfficialBlueprintsBuild validates that all official kscore/* blueprints
// can be built successfully. This is part of the 0.1.0 release validation.
func TestOfficialBlueprintsBuild(t *testing.T) {
	// Find the examples/blueprints/kscore directory
	blueprintsDir := findBlueprintsDir(t)
	if blueprintsDir == "" {
		t.Skip("official blueprints directory not found")
	}

	entries, err := os.ReadDir(blueprintsDir)
	if err != nil {
		t.Fatalf("failed to read blueprints directory: %v", err)
	}

	publisher := NewPublisher(nil, nil)

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		blueprintName := entry.Name()
		// Skip registry directory - it contains registry metadata, not a blueprint
		if blueprintName == "registry" {
			continue
		}
		blueprintPath := filepath.Join(blueprintsDir, blueprintName)

		t.Run(blueprintName, func(t *testing.T) {
			// Check for manifest
			manifestPath := filepath.Join(blueprintPath, "blueprint.yaml")
			if _, err := os.Stat(manifestPath); os.IsNotExist(err) {
				// Try blueprint.yml
				manifestPath = filepath.Join(blueprintPath, "blueprint.yml")
				if _, err := os.Stat(manifestPath); os.IsNotExist(err) {
					t.Fatalf("no blueprint manifest found in %s", blueprintPath)
				}
			}

			// Build the blueprint
			result, err := publisher.Build(blueprintPath)
			if err != nil {
				t.Fatalf("failed to build blueprint: %v", err)
			}

			// Validate result
			if result.Blueprint == nil {
				t.Error("blueprint manifest should not be nil")
			}

			if result.Blueprint.Metadata.Name == "" {
				t.Error("blueprint name should not be empty")
			}

			if result.Blueprint.Metadata.Version == "" {
				t.Error("blueprint version should not be empty")
			}

			if len(result.Archive) == 0 {
				t.Error("archive should not be empty")
			}

			if result.Checksum == "" {
				t.Error("checksum should not be empty")
			}

			// Log success with details
			t.Logf("✓ kscore/%s@%s - %d bytes, %d files",
				result.Blueprint.Metadata.Name,
				result.Blueprint.Metadata.Version,
				result.Size,
				len(result.Files))
		})
	}
}

// TestOfficialBlueprintsMetadata validates that all official blueprints have
// required metadata fields for publishing.
func TestOfficialBlueprintsMetadata(t *testing.T) {
	blueprintsDir := findBlueprintsDir(t)
	if blueprintsDir == "" {
		t.Skip("official blueprints directory not found")
	}

	entries, err := os.ReadDir(blueprintsDir)
	if err != nil {
		t.Fatalf("failed to read blueprints directory: %v", err)
	}

	publisher := NewPublisher(nil, nil)

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		blueprintName := entry.Name()
		// Skip registry directory - it contains registry metadata, not a blueprint
		if blueprintName == "registry" {
			continue
		}
		blueprintPath := filepath.Join(blueprintsDir, blueprintName)

		t.Run(blueprintName, func(t *testing.T) {
			result, err := publisher.Build(blueprintPath)
			if err != nil {
				t.Fatalf("failed to build blueprint: %v", err)
			}

			meta := result.Blueprint.Metadata

			// Required fields for official blueprints
			if meta.Description == "" {
				t.Error("description is required for official blueprints")
			}

			if len(meta.Maintainers) == 0 {
				t.Error("at least one maintainer is required for official blueprints")
			}

			if meta.License == "" {
				t.Error("license is required for official blueprints")
			}

			// Version should be 0.1.0 for initial release
			if meta.Version != "0.1.0" {
				t.Logf("note: version is %s (expected 0.1.0 for initial release)", meta.Version)
			}
		})
	}
}

// TestOfficialBlueprintsStatesExist validates that all official blueprints
// have at least one state file.
func TestOfficialBlueprintsStatesExist(t *testing.T) {
	blueprintsDir := findBlueprintsDir(t)
	if blueprintsDir == "" {
		t.Skip("official blueprints directory not found")
	}

	entries, err := os.ReadDir(blueprintsDir)
	if err != nil {
		t.Fatalf("failed to read blueprints directory: %v", err)
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		blueprintName := entry.Name()
		// Skip registry directory - it contains registry metadata, not a blueprint
		if blueprintName == "registry" {
			continue
		}
		blueprintPath := filepath.Join(blueprintsDir, blueprintName)

		t.Run(blueprintName, func(t *testing.T) {
			statesDir := filepath.Join(blueprintPath, "states")
			stateEntries, err := os.ReadDir(statesDir)
			if err != nil {
				t.Fatalf("failed to read states directory: %v", err)
			}

			stateCount := 0
			for _, se := range stateEntries {
				if se.IsDir() {
					continue
				}
				name := se.Name()
				if filepath.Ext(name) == ".yaml" || filepath.Ext(name) == ".yml" {
					stateCount++
				}
			}

			if stateCount == 0 {
				t.Error("at least one state file (.yaml/.yml) is required")
			}

			t.Logf("✓ %d state files", stateCount)
		})
	}
}

// findBlueprintsDir searches for the official blueprints directory.
func findBlueprintsDir(t *testing.T) string {
	t.Helper()

	// Try relative paths from likely test execution locations
	candidates := []string{
		"../../../examples/blueprints/kscore",
		"../../../../examples/blueprints/kscore",
		"examples/blueprints/kscore",
	}

	// Try to find repository root by looking for go.mod
	if cwd, err := os.Getwd(); err == nil {
		dir := cwd
		for i := 0; i < 10; i++ {
			if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
				candidates = append([]string{filepath.Join(dir, "examples/blueprints/kscore")}, candidates...)
				break
			}
			parent := filepath.Dir(dir)
			if parent == dir {
				break
			}
			dir = parent
		}
	}

	for _, candidate := range candidates {
		absPath, err := filepath.Abs(candidate)
		if err != nil {
			continue
		}
		if info, err := os.Stat(absPath); err == nil && info.IsDir() {
			return absPath
		}
	}

	return ""
}
