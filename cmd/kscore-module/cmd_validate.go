package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/shawnbutts/keystone-core/pkg/module/manifest"
)

var (
	validateStrict bool
)

var validateCmd = &cobra.Command{
	Use:   "validate [path]",
	Short: "Validate module configuration",
	Long: `Validate a module's manifest (module.yaml) for correctness.

Checks:
  - YAML syntax
  - Required fields (name, version, type)
  - Capability declarations
  - Dependency format
  - Resource limit values

Examples:
  # Validate current directory
  kscorectl module validate

  # Validate specific directory
  kscorectl module validate ./my-module

  # Validate specific file
  kscorectl module validate ./my-module/module.yaml

  # Strict mode (treat warnings as errors)
  kscorectl module validate --strict`,
	Args: cobra.MaximumNArgs(1),
	RunE: validateExecute,
}

func init() {
	validateCmd.Flags().BoolVar(&validateStrict, "strict", false, "Treat warnings as errors")
}

func validateExecute(cmd *cobra.Command, args []string) error {
	// Determine path
	path := "."
	if len(args) > 0 {
		path = args[0]
	}

	// Find module.yaml
	manifestPath := path
	info, err := os.Stat(path)
	if err != nil {
		return fmt.Errorf("path not found: %s", path)
	}

	if info.IsDir() {
		manifestPath = filepath.Join(path, "module.yaml")
	}

	fmt.Printf("Validating: %s\n\n", manifestPath)

	// Parse manifest
	m, err := manifest.ParseFile(manifestPath)
	if err != nil {
		fmt.Printf("✗ Parse error: %v\n", err)
		return fmt.Errorf("validation failed")
	}

	// Collect validation issues
	var errors []string
	var warnings []string

	// Check required fields
	if m.Name == "" {
		errors = append(errors, "name is required")
	}
	if m.Version == "" {
		errors = append(errors, "version is required")
	}
	if m.Type == "" {
		errors = append(errors, "type is required")
	}

	// Validate module type
	validTypes := map[string]bool{
		"starlark": true,
		"wasm":     true,
		"hybrid":   true,
	}
	if m.Type != "" && !validTypes[m.Type] {
		errors = append(errors, fmt.Sprintf("invalid type '%s' (use: starlark, wasm, hybrid)", m.Type))
	}

	// Validate version format (basic semver check)
	if m.Version != "" {
		if !isValidSemver(m.Version) {
			warnings = append(warnings, fmt.Sprintf("version '%s' may not be valid semver", m.Version))
		}
	}

	// Validate capabilities
	validCaps := map[string]bool{
		"fs.read":       true,
		"fs.write":      true,
		"http.get":      true,
		"http.post":     true,
		"exec":          true,
		"secrets.read":  true,
		"secrets.write": true,
		"log":           true,
		"time":          true,
		"kv":            true,
	}
	for _, cap := range m.Capabilities {
		if !validCaps[cap] {
			warnings = append(warnings, fmt.Sprintf("unknown capability '%s'", cap))
		}
	}

	// Check for dangerous capabilities
	dangerousCaps := []string{"exec", "secrets.write", "fs.write"}
	for _, cap := range m.Capabilities {
		for _, dangerous := range dangerousCaps {
			if cap == dangerous {
				warnings = append(warnings, fmt.Sprintf("capability '%s' requires elevated trust level", cap))
			}
		}
	}

	// Validate limits
	if m.Limits.Timeout < 0 {
		errors = append(errors, "limits.timeout cannot be negative")
	}
	if m.Limits.Memory != "" {
		if !isValidMemorySize(m.Limits.Memory) {
			errors = append(errors, fmt.Sprintf("invalid memory limit format: %s", m.Limits.Memory))
		}
	}

	// Validate dependencies
	for depName, constraint := range m.Dependencies {
		if depName == "" {
			errors = append(errors, "dependency name cannot be empty")
		}
		if constraint == "" {
			warnings = append(warnings, fmt.Sprintf("dependency '%s' has no version constraint", depName))
		}
	}

	// Check for entrypoint
	if m.Entrypoint == "" {
		warnings = append(warnings, "no entrypoint specified")
	} else {
		// Check if entrypoint file exists
		entrypointPath := filepath.Join(filepath.Dir(manifestPath), m.Entrypoint)
		if _, err := os.Stat(entrypointPath); os.IsNotExist(err) {
			warnings = append(warnings, fmt.Sprintf("entrypoint file not found: %s", m.Entrypoint))
		}
	}

	// Check for description and author (nice to have)
	if m.Description == "" {
		warnings = append(warnings, "no description specified")
	}
	if m.Author == "" {
		warnings = append(warnings, "no author specified")
	}

	// Print results
	if len(errors) > 0 {
		fmt.Println("Errors:")
		for _, e := range errors {
			fmt.Printf("  ✗ %s\n", e)
		}
		fmt.Println()
	}

	if len(warnings) > 0 {
		fmt.Println("Warnings:")
		for _, w := range warnings {
			fmt.Printf("  ⚠ %s\n", w)
		}
		fmt.Println()
	}

	// Print summary
	if len(errors) == 0 && len(warnings) == 0 {
		fmt.Println("✓ Module is valid!")
		printManifestSummary(m)
		return nil
	}

	if len(errors) == 0 {
		if validateStrict {
			fmt.Println("✗ Validation failed (strict mode: warnings treated as errors)")
			return fmt.Errorf("validation failed with %d warnings", len(warnings))
		}
		fmt.Println("✓ Module is valid (with warnings)")
		printManifestSummary(m)
		return nil
	}

	fmt.Printf("✗ Validation failed (%d errors, %d warnings)\n", len(errors), len(warnings))
	return fmt.Errorf("validation failed")
}

func printManifestSummary(m *manifest.Manifest) {
	fmt.Println()
	fmt.Println("Module Summary:")
	fmt.Printf("  Name:         %s\n", m.Name)
	fmt.Printf("  Version:      %s\n", m.Version)
	fmt.Printf("  Type:         %s\n", m.Type)
	if m.Description != "" {
		fmt.Printf("  Description:  %s\n", m.Description)
	}
	if m.Author != "" {
		fmt.Printf("  Author:       %s\n", m.Author)
	}
	if len(m.Capabilities) > 0 {
		fmt.Printf("  Capabilities: %v\n", m.Capabilities)
	}
	if len(m.Dependencies) > 0 {
		fmt.Printf("  Dependencies: %d\n", len(m.Dependencies))
	}
}

func isValidSemver(version string) bool {
	// Basic semver validation: X.Y.Z with optional prerelease
	// For full validation, use the resolver package
	parts := 0
loop:
	for _, c := range version {
		switch {
		case c == '.':
			parts++
		case c >= '0' && c <= '9':
			continue
		case c == '-' || c == '+':
			// Prerelease or build metadata - stop counting parts
			break loop
		case c >= 'a' && c <= 'z':
			continue
		default:
			return false
		}
	}
	return parts >= 1 // At least X.Y
}

func isValidMemorySize(size string) bool {
	// Valid formats: 64MB, 128MB, 1GB, etc.
	if len(size) < 2 {
		return false
	}

	// Find where the unit starts
	unitStart := len(size)
	for i := len(size) - 1; i >= 0; i-- {
		if size[i] >= '0' && size[i] <= '9' {
			unitStart = i + 1
			break
		}
	}

	if unitStart == 0 || unitStart == len(size) {
		return false
	}

	unit := size[unitStart:]
	validUnits := map[string]bool{
		"B": true, "KB": true, "MB": true, "GB": true,
		"b": true, "kb": true, "mb": true, "gb": true,
		"K": true, "M": true, "G": true,
		"k": true, "m": true, "g": true,
	}

	return validUnits[unit]
}
