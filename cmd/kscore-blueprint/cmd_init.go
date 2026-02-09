package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

var (
	initDescription string
	initAuthor      string
	initEmail       string
	initLicense     string
	initOutput      string
	initCategory    string
	initKeywords    []string
)

var initCmd = &cobra.Command{
	Use:   "init <vendor/name>",
	Short: "Initialize a new blueprint",
	Long: `Initialize a new Keystone Core blueprint from a template.

Creates a blueprint directory with:
  blueprint.yaml    Manifest with metadata, parameters, and dependencies
  README.md         Documentation template
  CHANGELOG.md      Version history
  LICENSE           License file
  states/
    init.yaml       Main entry point
  vars/
    defaults.yaml   Default parameter values
  templates/        Template files
  files/            Static files to distribute
  tests/
    test_basic.yaml Basic test template

Blueprint names follow the format: vendor/name (e.g., myorg/web-stack)

Examples:
  # Create a blueprint
  kscorectl blueprint init myorg/web-stack

  # Create with author and description
  kscorectl blueprint init myorg/web-stack \
    --author "John Doe" \
    --email "john@example.com" \
    --description "Web application stack with nginx and PostgreSQL"

  # Create with keywords for discoverability
  kscorectl blueprint init myorg/web-stack \
    --keywords web,nginx,postgresql,stack`,
	Args: cobra.ExactArgs(1),
	RunE: initExecute,
}

func init() {
	initCmd.Flags().StringVar(&initDescription, "description", "", "Blueprint description")
	initCmd.Flags().StringVar(&initAuthor, "author", "", "Blueprint author name")
	initCmd.Flags().StringVar(&initEmail, "email", "", "Author email address")
	initCmd.Flags().StringVar(&initLicense, "license", "Apache-2.0", "License (Apache-2.0, MIT, BSD-3-Clause)")
	initCmd.Flags().StringVar(&initOutput, "output", "", "Output directory (defaults to blueprint name)")
	initCmd.Flags().StringVar(&initCategory, "category", "general", "Blueprint category")
	initCmd.Flags().StringSliceVar(&initKeywords, "keywords", nil, "Keywords for search (comma-separated)")
}

func initExecute(cmd *cobra.Command, args []string) error {
	blueprintName := args[0]

	// Validate blueprint name format
	parts := strings.Split(blueprintName, "/")
	if len(parts) != 2 {
		return fmt.Errorf("blueprint name must be in format 'vendor/name', got: %s", blueprintName)
	}
	vendor, name := parts[0], parts[1]
	if vendor == "" || name == "" {
		return fmt.Errorf("blueprint name must be in format 'vendor/name', got: %s", blueprintName)
	}

	// Validate name characters
	for _, r := range name {
		if (r < 'a' || r > 'z') && (r < '0' || r > '9') && r != '-' && r != '_' {
			return fmt.Errorf("blueprint name must contain only lowercase letters, numbers, hyphens, and underscores")
		}
	}
	for _, r := range vendor {
		if (r < 'a' || r > 'z') && (r < '0' || r > '9') && r != '-' && r != '_' {
			return fmt.Errorf("vendor name must contain only lowercase letters, numbers, hyphens, and underscores")
		}
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

	fmt.Printf("Creating blueprint: %s\n", blueprintName)
	fmt.Printf("Output directory: %s\n", outputDir)

	// Create directory structure
	dirs := []string{
		outputDir,
		filepath.Join(outputDir, "states"),
		filepath.Join(outputDir, "vars"),
		filepath.Join(outputDir, "templates"),
		filepath.Join(outputDir, "files"),
		filepath.Join(outputDir, "tests"),
	}

	for _, dir := range dirs {
		//nolint:gosec // G301: blueprint directory needs to be accessible by users
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return fmt.Errorf("failed to create directory %s: %w", dir, err)
		}
	}

	// Create blueprint.yaml
	if err := createBlueprintManifest(blueprintName, vendor, name, outputDir); err != nil {
		return err
	}

	// Create README.md
	if err := createReadme(blueprintName, name, outputDir); err != nil {
		return err
	}

	// Create CHANGELOG.md
	if err := createChangelog(name, outputDir); err != nil {
		return err
	}

	// Create LICENSE
	if err := createLicense(outputDir); err != nil {
		return err
	}

	// Create states/init.yaml
	if err := createInitState(name, outputDir); err != nil {
		return err
	}

	// Create vars/defaults.yaml
	if err := createDefaultVars(outputDir); err != nil {
		return err
	}

	// Create .gitkeep files
	//nolint:gosec // G306: gitkeep files need to be readable for version control
	if err := os.WriteFile(filepath.Join(outputDir, "templates", ".gitkeep"), []byte(""), 0o644); err != nil {
		return fmt.Errorf("failed to create templates/.gitkeep: %w", err)
	}
	//nolint:gosec // G306: gitkeep files need to be readable for version control
	if err := os.WriteFile(filepath.Join(outputDir, "files", ".gitkeep"), []byte(""), 0o644); err != nil {
		return fmt.Errorf("failed to create files/.gitkeep: %w", err)
	}

	// Create tests/test_basic.yaml
	if err := createBasicTest(name, outputDir); err != nil {
		return err
	}

	fmt.Printf("\n✓ Blueprint created successfully!\n\n")
	fmt.Printf("Next steps:\n")
	fmt.Printf("  cd %s\n", outputDir)
	fmt.Printf("  # Edit blueprint.yaml to define parameters\n")
	fmt.Printf("  # Add state definitions in states/\n")
	fmt.Printf("  # Add default values in vars/defaults.yaml\n")
	fmt.Printf("  kscorectl blueprint validate .\n")
	fmt.Printf("  kscorectl blueprint lint .\n")

	return nil
}

func createBlueprintManifest(fullName, vendor, name, outputDir string) error {
	// Build maintainers section
	maintainers := ""
	if initAuthor != "" {
		maintainers = "maintainers:\n"
		maintainers += fmt.Sprintf("  - name: %s\n", initAuthor)
		if initEmail != "" {
			maintainers += fmt.Sprintf("    email: %s\n", initEmail)
		}
	}

	// Build keywords section
	keywords := ""
	if len(initKeywords) > 0 {
		keywords = "keywords:\n"
		for _, kw := range initKeywords {
			keywords += fmt.Sprintf("  - %s\n", strings.TrimSpace(kw))
		}
	}

	description := initDescription
	if description == "" {
		description = fmt.Sprintf("A Keystone Core blueprint for %s", name)
	}

	manifest := fmt.Sprintf(`# Blueprint manifest for %s
apiVersion: blueprints.keystone-core.io/v1
kind: Blueprint

metadata:
  name: %s
  version: 0.1.0
  description: %s
%s  license: %s
%s  categories:
    - %s

# Version and platform compatibility
compatibility:
  kscore: ">=1.0.0"
  platforms:
    - os: linux
    - os: darwin

# Dependencies on other blueprints (optional)
# dependencies:
#   requires:
#     - blueprints/community/base@^1.0
#   requires_before:
#     - blueprints/myorg/networking@^1.0

# Optional features (enable with features: { feature_name: true })
# features:
#   monitoring:
#     description: Enable monitoring integration
#     default: false
#     enables:
#       - states/monitoring.yaml
#   ssl:
#     description: Enable SSL/TLS support
#     default: true
#     parameters:
#       - ssl_cert_path
#       - ssl_key_path

# Entry points for different operations
entrypoints:
  default: states/init.yaml
  # install: states/install.yaml
  # configure: states/configure.yaml
  # remove: states/remove.yaml

# Parameter schema (JSON Schema-like)
parameters:
  app_name:
    type: string
    description: Application name
    required: true
    pattern: "^[a-z][a-z0-9-]*$"
    examples:
      - my-app
      - web-service

  app_port:
    type: integer
    description: Application port
    default: 8080
    minimum: 1
    maximum: 65535

  environment:
    type: string
    description: Deployment environment
    default: development
    enum:
      - development
      - staging
      - production

  # Example of a sensitive parameter
  # api_key:
  #   type: string
  #   description: API key for external service
  #   required: true
  #   sensitive: true
  #   source: secret

# Outputs exposed after application
outputs:
  service_url:
    description: URL of the deployed service
    value: "http://{{ .params.app_name }}:{{ .params.app_port }}"

# Lifecycle hooks
# hooks:
#   pre_apply:
#     - states/pre_apply.yaml
#   post_apply:
#     - states/post_apply.yaml
`, fullName, name, description, maintainers, initLicense, keywords, initCategory)

	//nolint:gosec // G306: blueprint manifest needs to be readable by operators and tools
	return os.WriteFile(filepath.Join(outputDir, "blueprint.yaml"), []byte(manifest), 0o644)
}

func createReadme(fullName, name, outputDir string) error {
	description := initDescription
	if description == "" {
		description = fmt.Sprintf("A Keystone Core blueprint for %s", name)
	}

	readme := fmt.Sprintf(`# %s

%s

## Requirements

- Keystone Core >= 1.0.0
- Linux or macOS

## Installation

%s%s%syaml
include:
  - blueprint: %s@0.1.0
    parameters:
      app_name: my-app
      app_port: 8080
%s%s%s

## Parameters

| Name | Type | Required | Default | Description |
|------|------|----------|---------|-------------|
| app_name | string | yes | - | Application name |
| app_port | integer | no | 8080 | Application port |
| environment | string | no | development | Deployment environment |

## Features

This blueprint supports the following optional features:

- (Add features here)

## Examples

### Basic Usage

%s%s%syaml
include:
  - blueprint: %s@0.1.0
    parameters:
      app_name: my-app
%s%s%s

### With Custom Port

%s%s%syaml
include:
  - blueprint: %s@0.1.0
    parameters:
      app_name: my-app
      app_port: 3000
      environment: production
%s%s%s

## Outputs

| Name | Description |
|------|-------------|
| service_url | URL of the deployed service |

## Testing

Run the test suite:

%s%s%sbash
kscorectl blueprint test .
%s%s%s

## Contributing

1. Fork the repository
2. Create a feature branch
3. Make your changes
4. Run tests
5. Submit a pull request

## License

%s
`, name, description,
		"`", "`", "`", fullName, "`", "`", "`",
		"`", "`", "`", fullName, "`", "`", "`",
		"`", "`", "`", fullName, "`", "`", "`",
		"`", "`", "`", "`", "`", "`",
		initLicense)

	//nolint:gosec // G306: README needs to be readable by users
	return os.WriteFile(filepath.Join(outputDir, "README.md"), []byte(readme), 0o644)
}

func createChangelog(name, outputDir string) error {
	date := time.Now().Format("2006-01-02")
	changelog := fmt.Sprintf(`# Changelog

All notable changes to this blueprint will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

## [0.1.0] - %s

### Added
- Initial release
- Basic %s functionality
`, date, name)

	//nolint:gosec // G306: CHANGELOG needs to be readable by users
	return os.WriteFile(filepath.Join(outputDir, "CHANGELOG.md"), []byte(changelog), 0o644)
}

func createLicense(outputDir string) error {
	year := time.Now().Year()
	author := initAuthor
	if author == "" {
		author = "[Author Name]"
	}

	var licenseText string
	switch initLicense {
	case "MIT":
		licenseText = fmt.Sprintf(`MIT License

Copyright (c) %d %s

Permission is hereby granted, free of charge, to any person obtaining a copy
of this software and associated documentation files (the "Software"), to deal
in the Software without restriction, including without limitation the rights
to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
copies of the Software, and to permit persons to whom the Software is
furnished to do so, subject to the following conditions:

The above copyright notice and this permission notice shall be included in all
copies or substantial portions of the Software.

THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
SOFTWARE.
`, year, author)

	case "BSD-3-Clause":
		licenseText = fmt.Sprintf(`BSD 3-Clause License

Copyright (c) %d, %s
All rights reserved.

Redistribution and use in source and binary forms, with or without
modification, are permitted provided that the following conditions are met:

1. Redistributions of source code must retain the above copyright notice, this
   list of conditions and the following disclaimer.

2. Redistributions in binary form must reproduce the above copyright notice,
   this list of conditions and the following disclaimer in the documentation
   and/or other materials provided with the distribution.

3. Neither the name of the copyright holder nor the names of its
   contributors may be used to endorse or promote products derived from
   this software without specific prior written permission.

THIS SOFTWARE IS PROVIDED BY THE COPYRIGHT HOLDERS AND CONTRIBUTORS "AS IS"
AND ANY EXPRESS OR IMPLIED WARRANTIES, INCLUDING, BUT NOT LIMITED TO, THE
IMPLIED WARRANTIES OF MERCHANTABILITY AND FITNESS FOR A PARTICULAR PURPOSE ARE
DISCLAIMED. IN NO EVENT SHALL THE COPYRIGHT HOLDER OR CONTRIBUTORS BE LIABLE
FOR ANY DIRECT, INDIRECT, INCIDENTAL, SPECIAL, EXEMPLARY, OR CONSEQUENTIAL
DAMAGES (INCLUDING, BUT NOT LIMITED TO, PROCUREMENT OF SUBSTITUTE GOODS OR
SERVICES; LOSS OF USE, DATA, OR PROFITS; OR BUSINESS INTERRUPTION) HOWEVER
CAUSED AND ON ANY THEORY OF LIABILITY, WHETHER IN CONTRACT, STRICT LIABILITY,
OR TORT (INCLUDING NEGLIGENCE OR OTHERWISE) ARISING IN ANY WAY OUT OF THE USE
OF THIS SOFTWARE, EVEN IF ADVISED OF THE POSSIBILITY OF SUCH DAMAGE.
`, year, author)

	default: // Apache-2.0
		licenseText = fmt.Sprintf(`                                 Apache License
                           Version 2.0, January 2004
                        http://www.apache.org/licenses/

   Copyright %d %s

   Licensed under the Apache License, Version 2.0 (the "License");
   you may not use this file except in compliance with the License.
   You may obtain a copy of the License at

       http://www.apache.org/licenses/LICENSE-2.0

   Unless required by applicable law or agreed to in writing, software
   distributed under the License is distributed on an "AS IS" BASIS,
   WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
   See the License for the specific language governing permissions and
   limitations under the License.
`, year, author)
	}

	//nolint:gosec // G306: LICENSE needs to be readable by users
	return os.WriteFile(filepath.Join(outputDir, "LICENSE"), []byte(licenseText), 0o644)
}

func createInitState(name, outputDir string) error {
	state := fmt.Sprintf(`# Main entry point for %s blueprint
# This file is executed when the blueprint is applied

# Include common variables
include:
  - vars/defaults.yaml

# States for the %s blueprint
states:
  # Example: Ensure configuration directory exists
  - id: config_directory
    module: file
    params:
      path: "/etc/{{ .params.app_name }}"
      state: directory
      mode: "0o755"

  # Example: Deploy configuration file from template
  - id: config_file
    module: file
    params:
      path: "/etc/{{ .params.app_name }}/config.yaml"
      state: present
      mode: "0o644"
      contents: |
        # Configuration for {{ .params.app_name }}
        environment: {{ .params.environment }}
        port: {{ .params.app_port }}
    require:
      - config_directory

  # Add more states as needed
  # See: https://docs.keystone-core.io/docs/reference/modules/
`, name, name)

	//nolint:gosec // G306: state files need to be readable by the state executor
	return os.WriteFile(filepath.Join(outputDir, "states", "init.yaml"), []byte(state), 0o644)
}

func createDefaultVars(outputDir string) error {
	vars := `# Default variable values for this blueprint
# These can be overridden by parameters when including the blueprint

# Default configuration values
config:
  log_level: info
  retry_count: 3
  timeout: 30s

# Platform-specific defaults (will be merged based on target platform)
# platform:
#   linux:
#     config_dir: /etc
#   darwin:
#     config_dir: /usr/local/etc
`

	//nolint:gosec // G306: vars files need to be readable by operators and tools
	return os.WriteFile(filepath.Join(outputDir, "vars", "defaults.yaml"), []byte(vars), 0o644)
}

func createBasicTest(name, outputDir string) error {
	test := fmt.Sprintf(`# Basic test for %s blueprint
# Run with: kscorectl blueprint test .

name: Basic functionality test
description: Verify the blueprint applies correctly

# Test parameters
parameters:
  app_name: test-app
  app_port: 8080
  environment: development

# Test assertions
assertions:
  # Verify configuration directory was created
  - name: config_directory_exists
    type: file_exists
    path: /etc/test-app
    is_directory: true

  # Verify configuration file was created
  - name: config_file_exists
    type: file_exists
    path: /etc/test-app/config.yaml

  # Verify configuration file contents
  - name: config_file_contents
    type: file_contains
    path: /etc/test-app/config.yaml
    contains:
      - "environment: development"
      - "port: 8080"

# Cleanup after test
cleanup:
  - path: /etc/test-app
    state: absent
    recursive: true
`, name)

	//nolint:gosec // G306: test files need to be readable by the test runner
	return os.WriteFile(filepath.Join(outputDir, "tests", "test_basic.yaml"), []byte(test), 0o644)
}
