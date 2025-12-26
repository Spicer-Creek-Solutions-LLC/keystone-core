package starlark

import (
	"fmt"
	"os"
	"path/filepath"
)

// ModuleTemplate represents a Starlark module template
type ModuleTemplate struct {
	Name         string
	Description  string
	Version      string
	Author       string
	Capabilities []string
	Dependencies map[string]string
}

// DefaultTemplate returns a default module template
func DefaultTemplate(name string) *ModuleTemplate {
	return &ModuleTemplate{
		Name:         name,
		Description:  "A TitanAnvil module",
		Version:      "0.1.0",
		Author:       "",
		Capabilities: []string{},
		Dependencies: make(map[string]string),
	}
}

// Generate creates the module structure on disk
func (t *ModuleTemplate) Generate(outputDir string) error {
	// Create module directory
	moduleDir := filepath.Join(outputDir, t.Name)
	if err := os.MkdirAll(moduleDir, 0755); err != nil {
		return fmt.Errorf("failed to create module directory: %w", err)
	}

	// Create subdirectories
	dirs := []string{"states", "tests"}
	for _, dir := range dirs {
		if err := os.MkdirAll(filepath.Join(moduleDir, dir), 0755); err != nil {
			return fmt.Errorf("failed to create %s directory: %w", dir, err)
		}
	}

	// Generate module.yaml
	manifestPath := filepath.Join(moduleDir, "module.yaml")
	if err := t.generateManifest(manifestPath); err != nil {
		return err
	}

	// Generate main module file
	mainPath := filepath.Join(moduleDir, "states", "main.star")
	if err := t.generateMainFile(mainPath); err != nil {
		return err
	}

	// Generate test file
	testPath := filepath.Join(moduleDir, "tests", "main_test.star")
	if err := t.generateTestFile(testPath); err != nil {
		return err
	}

	// Generate README
	readmePath := filepath.Join(moduleDir, "README.md")
	if err := t.generateReadme(readmePath); err != nil {
		return err
	}

	return nil
}

func (t *ModuleTemplate) generateManifest(path string) error {
	content := fmt.Sprintf(`name: %s
version: %s
description: %s

capabilities:
`, t.Name, t.Version, t.Description)

	if len(t.Capabilities) > 0 {
		for _, cap := range t.Capabilities {
			content += fmt.Sprintf("  - %s\n", cap)
		}
	} else {
		content += "  [] # No capabilities required\n"
	}

	content += "\ndependencies:\n"
	if len(t.Dependencies) > 0 {
		for name, version := range t.Dependencies {
			content += fmt.Sprintf("  %s: %s\n", name, version)
		}
	} else {
		content += "  {} # No dependencies\n"
	}

	content += `
metadata:
  repository: ""
  homepage: ""
  license: "MIT"
`

	if t.Author != "" {
		content += fmt.Sprintf("  author: %s\n", t.Author)
	}

	return os.WriteFile(path, []byte(content), 0644)
}

func (t *ModuleTemplate) generateMainFile(path string) error {
	content := fmt.Sprintf(`"""
%s

%s
"""

def hello(name="World"):
    """Say hello to someone."""
    return "Hello, " + name + "!"

def main():
    """Main entry point for the module."""
    print(hello())
    return {"status": "success", "message": "Module executed successfully"}
`, t.Name, t.Description)

	return os.WriteFile(path, []byte(content), 0644)
}

func (t *ModuleTemplate) generateTestFile(path string) error {
	content := `"""
Tests for main module
"""

load("//states/main.star", "hello", "main")

def test_hello_default():
    """Test hello with default argument."""
    result = hello()
    assert.eq(result, "Hello, World!")

def test_hello_custom():
    """Test hello with custom name."""
    result = hello("TitanAnvil")
    assert.eq(result, "Hello, TitanAnvil!")

def test_main():
    """Test main function."""
    result = main()
    assert.eq(result["status"], "success")
    assert.true("message" in result)
`

	return os.WriteFile(path, []byte(content), 0644)
}

func (t *ModuleTemplate) generateReadme(path string) error {
	content := fmt.Sprintf(`# %s

%s

## Installation

`+"```bash"+`
titanctl module install %s
`+"```"+`

## Usage

`+"```starlark"+`
load("%s", "hello")

hello("World")
`+"```"+`

## Development

### Running Tests

`+"```bash"+`
titanctl module test
`+"```"+`

### Building

`+"```bash"+`
titanctl module build
`+"```"+`

## License

MIT
`, t.Name, t.Description, t.Name, t.Name)

	return os.WriteFile(path, []byte(content), 0644)
}
