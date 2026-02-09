package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/shawnbutts/keystone-core/pkg/module/sdk/starlark"
)

var (
	initType        string
	initAuthor      string
	initDescription string
	initOutput      string
)

var initCmd = &cobra.Command{
	Use:   "init <name>",
	Short: "Initialize a new module",
	Long: `Initialize a new Keystone Core module from a template.

Creates a module directory with:
  - module.yaml    Module manifest with metadata and capabilities
  - states/        Starlark state definitions
  - tests/         Test files
  - README.md      Documentation

Module names follow the format: vendor/name (e.g., myorg/webserver)

Examples:
  # Create a Starlark module
  kscorectl module init myorg/webserver

  # Create with author and description
  kscorectl module init myorg/webserver --author "John Doe" --description "Web server configuration"

  # Create in specific directory
  kscorectl module init myorg/webserver --output ./modules/webserver`,
	Args: cobra.ExactArgs(1),
	RunE: initExecute,
}

func init() {
	initCmd.Flags().StringVar(&initType, "type", "starlark", "Module type (starlark, wasm)")
	initCmd.Flags().StringVar(&initAuthor, "author", "", "Module author")
	initCmd.Flags().StringVar(&initDescription, "description", "", "Module description")
	initCmd.Flags().StringVar(&initOutput, "output", "", "Output directory (defaults to module name)")
}

func initExecute(cmd *cobra.Command, args []string) error {
	moduleName := args[0]

	// Validate module name format
	parts := strings.Split(moduleName, "/")
	if len(parts) != 2 {
		return fmt.Errorf("module name must be in format 'vendor/name', got: %s", moduleName)
	}
	vendor, name := parts[0], parts[1]
	if vendor == "" || name == "" {
		return fmt.Errorf("module name must be in format 'vendor/name', got: %s", moduleName)
	}

	// Determine output directory
	outputDir := initOutput
	if outputDir == "" {
		outputDir = name
	}

	// Check if directory exists
	if _, err := os.Stat(outputDir); err == nil {
		return fmt.Errorf("directory already exists: %s", outputDir)
	}

	fmt.Printf("Creating module: %s\n", moduleName)
	fmt.Printf("Output directory: %s\n", outputDir)
	fmt.Printf("Type: %s\n", initType)

	switch initType {
	case "starlark":
		return initStarlarkModule(moduleName, outputDir)
	case "wasm":
		return initWasmModule(moduleName, outputDir)
	default:
		return fmt.Errorf("unsupported module type: %s (use 'starlark' or 'wasm')", initType)
	}
}

func initStarlarkModule(moduleName, outputDir string) error {
	// Use the SDK template generator
	template := starlark.DefaultTemplate(moduleName)
	template.Version = "0.1.0"
	if initAuthor != "" {
		template.Author = initAuthor
	}
	if initDescription != "" {
		template.Description = initDescription
	}

	// Generate the module
	if err := template.Generate(outputDir); err != nil {
		return fmt.Errorf("failed to generate module: %w", err)
	}

	fmt.Printf("\n✓ Module created successfully!\n\n")
	fmt.Printf("Next steps:\n")
	fmt.Printf("  cd %s\n", outputDir)
	fmt.Printf("  # Edit states/main.star to add your state definitions\n")
	fmt.Printf("  # Edit module.yaml to configure capabilities\n")
	fmt.Printf("  kscorectl module validate .\n")
	fmt.Printf("  kscorectl module test\n")
	fmt.Printf("  kscorectl module build\n")

	return nil
}

func initWasmModule(moduleName, outputDir string) error {
	// Create directory structure for WASM module
	dirs := []string{
		outputDir,
		filepath.Join(outputDir, "src"),
	}

	for _, dir := range dirs {
		//nolint:gosec // G301: module directory needs to be accessible by users
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return fmt.Errorf("failed to create directory %s: %w", dir, err)
		}
	}

	// Create module.yaml
	parts := strings.Split(moduleName, "/")
	name := parts[1]

	manifest := fmt.Sprintf(`# Module manifest for %s
schema_version: 1
name: %s
version: 0.1.0
type: wasm
description: %s
author: %s

# WASM module entrypoint
entrypoint: %s.wasm

# Required capabilities
capabilities:
  - fs.read
  - log

# Resource limits
limits:
  timeout: 30s
  memory: 64MB

# Dependencies (if any)
dependencies: {}
`, moduleName, moduleName, coalesce(initDescription, "A WASM module for Keystone Core"), coalesce(initAuthor, ""), name)

	//nolint:gosec // G306: module manifest needs to be readable by operators and tools
	if err := os.WriteFile(filepath.Join(outputDir, "module.yaml"), []byte(manifest), 0o644); err != nil {
		return fmt.Errorf("failed to write module.yaml: %w", err)
	}

	// Create README
	bt := "`" // backtick for code blocks
	readme := fmt.Sprintf("# %s\n\n%s\n\n## Building\n\n### Rust\n\n%s%s%sbash\ncargo build --target wasm32-wasi --release\ncp target/wasm32-wasi/release/%s.wasm .\n%s%s%s\n\n### Go (TinyGo)\n\n%s%s%sbash\ntinygo build -target wasi -o %s.wasm .\n%s%s%s\n\n## Usage\n\n%s%s%syaml\n# In your state file\nstates:\n  - id: my-state\n    module: %s\n    params:\n      # Add parameters here\n%s%s%s\n",
		moduleName, coalesce(initDescription, "A WASM module for Keystone Core"),
		bt, bt, bt, name, bt, bt, bt,
		bt, bt, bt, name, bt, bt, bt,
		bt, bt, bt, moduleName, bt, bt, bt)

	//nolint:gosec // G306: README needs to be readable by users
	if err := os.WriteFile(filepath.Join(outputDir, "README.md"), []byte(readme), 0o644); err != nil {
		return fmt.Errorf("failed to write README.md: %w", err)
	}

	// Create a simple Rust example
	cargoToml := fmt.Sprintf(`[package]
name = "%s"
version = "0.1.0"
edition = "2021"

[lib]
crate-type = ["cdylib"]

[dependencies]
# Add kscore-sdk when available
# kscore-sdk = "0.1"

[profile.release]
opt-level = "z"
lto = true
strip = true
`, name)

	//nolint:gosec // G306: Cargo.toml needs to be readable for builds
	if err := os.WriteFile(filepath.Join(outputDir, "Cargo.toml"), []byte(cargoToml), 0o644); err != nil {
		return fmt.Errorf("failed to write Cargo.toml: %w", err)
	}

	// Create lib.rs
	libRs := `//! Keystone Core WASM Module

// Module entrypoint
#[no_mangle]
pub extern "C" fn apply() -> i32 {
    // Implement state application logic
    0 // Return 0 for success
}

#[no_mangle]
pub extern "C" fn check() -> i32 {
    // Implement state check logic
    0
}
`
	//nolint:gosec // G301: source directory needs to be accessible by users
	if err := os.MkdirAll(filepath.Join(outputDir, "src"), 0o755); err != nil {
		return fmt.Errorf("failed to create src directory: %w", err)
	}
	//nolint:gosec // G306: source files need to be readable for builds
	if err := os.WriteFile(filepath.Join(outputDir, "src", "lib.rs"), []byte(libRs), 0o644); err != nil {
		return fmt.Errorf("failed to write src/lib.rs: %w", err)
	}

	fmt.Printf("\n✓ WASM module created successfully!\n\n")
	fmt.Printf("Next steps:\n")
	fmt.Printf("  cd %s\n", outputDir)
	fmt.Printf("  # Edit src/lib.rs to implement your module\n")
	fmt.Printf("  cargo build --target wasm32-wasi --release\n")
	fmt.Printf("  kscorectl module validate .\n")
	fmt.Printf("  kscorectl module build\n")

	return nil
}

func coalesce(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}
