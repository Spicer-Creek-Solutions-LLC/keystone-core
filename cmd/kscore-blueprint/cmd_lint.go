package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
	"golang.org/x/text/cases"
	"golang.org/x/text/language"

	"github.com/shawnbutts/keystone-core/internal/blueprint"
)

var (
	lintFix    bool
	lintFormat string
)

var lintCmd = &cobra.Command{
	Use:   "lint [path]",
	Short: "Run best practices checks",
	Long: `Run best practices checks on a blueprint.

Checks for:
  - Documentation quality (README, parameter descriptions)
  - Security best practices (sensitive parameters, secrets handling)
  - Naming conventions
  - Version constraints
  - Test coverage
  - License compliance

Examples:
  # Lint blueprint in current directory
  kscorectl blueprint lint .

  # Lint with auto-fix (where possible)
  kscorectl blueprint lint . --fix`,
	Args: cobra.MaximumNArgs(1),
	RunE: lintExecute,
}

func init() {
	lintCmd.Flags().BoolVar(&lintFix, "fix", false, "Automatically fix issues where possible")
	lintCmd.Flags().StringVar(&lintFormat, "format", "text", "Output format (text, json)")
}

// LintIssue represents a single lint issue
type LintIssue struct {
	Level    string // error, warning, info
	Category string // documentation, security, naming, version, testing, license
	Message  string
	File     string
	Line     int
	Fixable  bool
}

func lintExecute(cmd *cobra.Command, args []string) error {
	path := "."
	if len(args) > 0 {
		path = args[0]
	}

	// Find blueprint.yaml
	manifestPath := filepath.Join(path, "blueprint.yaml")
	if _, err := os.Stat(manifestPath); os.IsNotExist(err) {
		return fmt.Errorf("blueprint.yaml not found in %s", path)
	}

	fmt.Printf("Linting blueprint at %s\n\n", path)

	// Parse the manifest
	bp, err := blueprint.ParseManifestFile(manifestPath)
	if err != nil {
		return fmt.Errorf("failed to parse blueprint.yaml: %w", err)
	}

	// Run linters - collect results
	docIssues := lintDocumentation(bp, path)
	secIssues := lintSecurity(bp)
	nameIssues := lintNaming(bp)
	verIssues := lintVersions(bp)
	testIssues := lintTesting(path)
	licIssues := lintLicense(bp, path)
	paramIssues := lintParameters(bp)

	issues := make([]LintIssue, 0, len(docIssues)+len(secIssues)+len(nameIssues)+len(verIssues)+len(testIssues)+len(licIssues)+len(paramIssues))
	issues = append(issues, docIssues...)
	issues = append(issues, secIssues...)
	issues = append(issues, nameIssues...)
	issues = append(issues, verIssues...)
	issues = append(issues, testIssues...)
	issues = append(issues, licIssues...)
	issues = append(issues, paramIssues...)

	// Group by category
	categories := map[string][]LintIssue{}
	for _, issue := range issues {
		categories[issue.Category] = append(categories[issue.Category], issue)
	}

	// Print results
	errorCount := 0
	warningCount := 0
	infoCount := 0

	titleCaser := cases.Title(language.English)
	for category, catIssues := range categories {
		fmt.Printf("%s:\n", titleCaser.String(category))
		for _, issue := range catIssues {
			var icon string
			switch issue.Level {
			case "error":
				icon = "✗"
				errorCount++
			case "warning":
				icon = "⚠"
				warningCount++
			default:
				icon = "ℹ"
				infoCount++
			}
			fixable := ""
			if issue.Fixable {
				fixable = " [fixable]"
			}
			fmt.Printf("  %s %s%s\n", icon, issue.Message, fixable)
		}
		fmt.Println()
	}

	// Summary
	fmt.Printf("Summary: %d errors, %d warnings, %d info\n\n", errorCount, warningCount, infoCount)

	if errorCount > 0 {
		return fmt.Errorf("linting failed with %d errors", errorCount)
	}

	if errorCount == 0 && warningCount == 0 && infoCount == 0 {
		fmt.Println("✓ No issues found")
	}

	return nil
}

func lintDocumentation(bp *blueprint.Blueprint, basePath string) []LintIssue {
	var issues []LintIssue

	// Check README exists
	readmePath := filepath.Join(basePath, "README.md")
	if _, err := os.Stat(readmePath); os.IsNotExist(err) {
		issues = append(issues, LintIssue{
			Level:    "warning",
			Category: "documentation",
			Message:  "README.md not found - add documentation for users",
			Fixable:  true,
		})
	}

	// Check description
	if bp.Metadata.Description == "" {
		issues = append(issues, LintIssue{
			Level:    "warning",
			Category: "documentation",
			Message:  "Blueprint has no description",
			File:     "blueprint.yaml",
			Fixable:  false,
		})
	} else if len(bp.Metadata.Description) < 20 {
		issues = append(issues, LintIssue{
			Level:    "info",
			Category: "documentation",
			Message:  "Description is very short - consider adding more detail",
			File:     "blueprint.yaml",
			Fixable:  false,
		})
	}

	// Check parameter descriptions
	for name := range bp.Parameters {
		if bp.Parameters[name].Description == "" {
			issues = append(issues, LintIssue{
				Level:    "warning",
				Category: "documentation",
				Message:  fmt.Sprintf("Parameter '%s' has no description", name),
				File:     "blueprint.yaml",
				Fixable:  false,
			})
		}
	}

	// Check for CHANGELOG
	changelogPath := filepath.Join(basePath, "CHANGELOG.md")
	if _, err := os.Stat(changelogPath); os.IsNotExist(err) {
		issues = append(issues, LintIssue{
			Level:    "info",
			Category: "documentation",
			Message:  "CHANGELOG.md not found - consider adding version history",
			Fixable:  true,
		})
	}

	return issues
}

func lintSecurity(bp *blueprint.Blueprint) []LintIssue {
	var issues []LintIssue

	// Check for sensitive parameters
	sensitiveWords := []string{"password", "secret", "key", "token", "credential", "api_key", "apikey"}

	for name := range bp.Parameters {
		param := bp.Parameters[name]
		nameLower := strings.ToLower(name)
		for _, word := range sensitiveWords {
			if strings.Contains(nameLower, word) && !param.Sensitive {
				issues = append(issues, LintIssue{
					Level:    "warning",
					Category: "security",
					Message:  fmt.Sprintf("Parameter '%s' appears sensitive but is not marked as sensitive: true", name),
					File:     "blueprint.yaml",
					Fixable:  true,
				})
				break
			}
		}

		// Check that sensitive params recommend using secrets
		if param.Sensitive && param.Source != "secret" {
			issues = append(issues, LintIssue{
				Level:    "info",
				Category: "security",
				Message:  fmt.Sprintf("Sensitive parameter '%s' should use 'source: secret' for secure storage", name),
				File:     "blueprint.yaml",
				Fixable:  true,
			})
		}
	}

	return issues
}

func lintNaming(bp *blueprint.Blueprint) []LintIssue {
	var issues []LintIssue

	// Check blueprint name
	if bp.Metadata.Name == "" {
		issues = append(issues, LintIssue{
			Level:    "error",
			Category: "naming",
			Message:  "Blueprint name is required",
			File:     "blueprint.yaml",
		})
	} else {
		// Check for valid characters
		for _, r := range bp.Metadata.Name {
			if (r < 'a' || r > 'z') && (r < '0' || r > '9') && r != '-' && r != '_' {
				issues = append(issues, LintIssue{
					Level:    "error",
					Category: "naming",
					Message:  "Blueprint name should use only lowercase letters, numbers, hyphens, and underscores",
					File:     "blueprint.yaml",
				})
				break
			}
		}
	}

	// Check parameter names
	for name := range bp.Parameters {
		// Check for camelCase vs snake_case consistency
		if strings.Contains(name, "-") {
			issues = append(issues, LintIssue{
				Level:    "info",
				Category: "naming",
				Message:  fmt.Sprintf("Parameter '%s' uses hyphens - consider snake_case for consistency", name),
				File:     "blueprint.yaml",
			})
		}
	}

	return issues
}

func lintVersions(bp *blueprint.Blueprint) []LintIssue {
	var issues []LintIssue

	// Check version format
	version := bp.Metadata.Version
	if version == "" {
		issues = append(issues, LintIssue{
			Level:    "error",
			Category: "version",
			Message:  "Blueprint version is required",
			File:     "blueprint.yaml",
		})
	} else if !isValidSemVer(version) {
		issues = append(issues, LintIssue{
			Level:    "warning",
			Category: "version",
			Message:  fmt.Sprintf("Version '%s' may not follow semantic versioning (e.g., 1.2.3)", version),
			File:     "blueprint.yaml",
		})
	}

	// Check Keystone Core version constraint
	if bp.Compatibility != nil && bp.Compatibility.Kscore == "" {
		issues = append(issues, LintIssue{
			Level:    "warning",
			Category: "version",
			Message:  "No Keystone Core version constraint specified - blueprint may not work with all versions",
			File:     "blueprint.yaml",
			Fixable:  true,
		})
	}

	return issues
}

func lintTesting(basePath string) []LintIssue {
	var issues []LintIssue

	testsDir := filepath.Join(basePath, "tests")
	if _, err := os.Stat(testsDir); os.IsNotExist(err) {
		issues = append(issues, LintIssue{
			Level:    "warning",
			Category: "testing",
			Message:  "No tests/ directory found - consider adding tests",
			Fixable:  true,
		})
		return issues
	}

	// Check for test files
	files, err := os.ReadDir(testsDir)
	if err != nil {
		return issues
	}

	testCount := 0
	for _, f := range files {
		if strings.HasSuffix(f.Name(), ".yaml") || strings.HasSuffix(f.Name(), ".yml") {
			testCount++
		}
	}

	if testCount == 0 {
		issues = append(issues, LintIssue{
			Level:    "warning",
			Category: "testing",
			Message:  "tests/ directory is empty - add test files",
			Fixable:  true,
		})
	}

	return issues
}

func lintLicense(bp *blueprint.Blueprint, basePath string) []LintIssue {
	var issues []LintIssue

	// Check license in manifest
	if bp.Metadata.License == "" {
		issues = append(issues, LintIssue{
			Level:    "warning",
			Category: "license",
			Message:  "No license specified in blueprint.yaml",
			File:     "blueprint.yaml",
			Fixable:  true,
		})
	}

	// Check LICENSE file
	licensePath := filepath.Join(basePath, "LICENSE")
	if _, err := os.Stat(licensePath); os.IsNotExist(err) {
		issues = append(issues, LintIssue{
			Level:    "warning",
			Category: "license",
			Message:  "LICENSE file not found",
			Fixable:  true,
		})
	}

	return issues
}

func lintParameters(bp *blueprint.Blueprint) []LintIssue {
	var issues []LintIssue

	for name := range bp.Parameters {
		param := bp.Parameters[name]
		// Check for defaults on required parameters
		if param.Required && param.Default != nil {
			issues = append(issues, LintIssue{
				Level:    "info",
				Category: "parameters",
				Message:  fmt.Sprintf("Parameter '%s' is required but has a default value", name),
				File:     "blueprint.yaml",
			})
		}

		// Check for examples
		if len(param.Examples) == 0 && param.Enum == nil && param.Default == nil {
			issues = append(issues, LintIssue{
				Level:    "info",
				Category: "parameters",
				Message:  fmt.Sprintf("Parameter '%s' has no examples - consider adding for documentation", name),
				File:     "blueprint.yaml",
			})
		}

		// Check string parameters have validation
		if param.Type == "string" && param.Pattern == "" && param.Format == "" && param.Enum == nil {
			issues = append(issues, LintIssue{
				Level:    "info",
				Category: "parameters",
				Message:  fmt.Sprintf("String parameter '%s' has no validation (pattern, format, or enum)", name),
				File:     "blueprint.yaml",
			})
		}
	}

	return issues
}

func isValidSemVer(v string) bool {
	// Simple semver check: x.y.z or x.y.z-prerelease
	parts := strings.Split(v, ".")
	if len(parts) < 2 || len(parts) > 3 {
		return false
	}
	// Check first two parts are numeric
	for _, p := range parts[:2] {
		for _, r := range p {
			if r < '0' || r > '9' {
				return false
			}
		}
	}
	return true
}
