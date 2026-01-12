package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/shawnbutts/keystone-core/pkg/blueprint"
)

var (
	validateStrict bool
	validateFormat string
)

var validateCmd = &cobra.Command{
	Use:   "validate [path]",
	Short: "Validate blueprint manifest",
	Long: `Validate a blueprint manifest for syntax and structural correctness.

Validates:
  - YAML syntax
  - Required fields (apiVersion, kind, metadata.name, metadata.version)
  - Parameter schema validity
  - Dependency references
  - Entry point file existence

Examples:
  # Validate blueprint in current directory
  kscorectl blueprint validate .

  # Validate specific blueprint directory
  kscorectl blueprint validate ./blueprints/web-stack

  # Strict validation (fail on warnings)
  kscorectl blueprint validate . --strict`,
	Args: cobra.MaximumNArgs(1),
	RunE: validateExecute,
}

func init() {
	validateCmd.Flags().BoolVar(&validateStrict, "strict", false, "Treat warnings as errors")
	validateCmd.Flags().StringVar(&validateFormat, "format", "text", "Output format (text, json)")
}

func validateExecute(cmd *cobra.Command, args []string) error {
	path := "."
	if len(args) > 0 {
		path = args[0]
	}

	// Find blueprint.yaml
	manifestPath := filepath.Join(path, "blueprint.yaml")
	if _, err := os.Stat(manifestPath); os.IsNotExist(err) {
		return fmt.Errorf("blueprint.yaml not found in %s", path)
	}

	fmt.Printf("Validating blueprint at %s\n\n", path)

	// Parse the manifest
	bp, err := blueprint.ParseManifestFile(manifestPath)
	if err != nil {
		return fmt.Errorf("failed to parse blueprint.yaml: %w", err)
	}

	// Run validation
	validator := blueprint.NewValidator()
	result := validator.Validate(bp)

	// Print results
	hasErrors := false
	hasWarnings := false

	if len(result.Errors) > 0 {
		hasErrors = true
		fmt.Println("Errors:")
		for _, e := range result.Errors {
			fmt.Printf("  ✗ %s\n", e)
		}
		fmt.Println()
	}

	if len(result.Warnings) > 0 {
		hasWarnings = true
		fmt.Println("Warnings:")
		for _, w := range result.Warnings {
			fmt.Printf("  ⚠ %s\n", w)
		}
		fmt.Println()
	}

	// Validate entry points exist
	entryErrors := validateEntrypoints(bp, path)
	if len(entryErrors) > 0 {
		hasErrors = true
		fmt.Println("Entry point errors:")
		for _, e := range entryErrors {
			fmt.Printf("  ✗ %s\n", e)
		}
		fmt.Println()
	}

	// Validate required directories exist
	dirErrors := validateDirectoryStructure(path)
	if len(dirErrors) > 0 {
		hasWarnings = true
		fmt.Println("Structure warnings:")
		for _, w := range dirErrors {
			fmt.Printf("  ⚠ %s\n", w)
		}
		fmt.Println()
	}

	// Summary
	if hasErrors {
		fmt.Println("✗ Validation failed")
		return fmt.Errorf("validation failed with errors")
	}

	if hasWarnings && validateStrict {
		fmt.Println("✗ Validation failed (strict mode)")
		return fmt.Errorf("validation failed with warnings (strict mode)")
	}

	if hasWarnings {
		fmt.Println("✓ Validation passed with warnings")
	} else {
		fmt.Println("✓ Validation passed")
	}

	return nil
}

func validateEntrypoints(bp *blueprint.Blueprint, basePath string) []string {
	var errors []string

	for name, path := range bp.Entrypoints {
		fullPath := filepath.Join(basePath, path)
		if _, err := os.Stat(fullPath); os.IsNotExist(err) {
			errors = append(errors, fmt.Sprintf("entry point '%s' references missing file: %s", name, path))
		}
	}

	// Also check default if no entrypoints defined
	if len(bp.Entrypoints) == 0 {
		defaultPath := filepath.Join(basePath, "states", "init.yaml")
		if _, err := os.Stat(defaultPath); os.IsNotExist(err) {
			errors = append(errors, "default entry point 'states/init.yaml' not found")
		}
	}

	return errors
}

func validateDirectoryStructure(basePath string) []string {
	var warnings []string

	expectedDirs := []string{"states", "vars", "templates", "files", "tests"}
	for _, dir := range expectedDirs {
		fullPath := filepath.Join(basePath, dir)
		if _, err := os.Stat(fullPath); os.IsNotExist(err) {
			warnings = append(warnings, fmt.Sprintf("recommended directory '%s' not found", dir))
		}
	}

	return warnings
}
