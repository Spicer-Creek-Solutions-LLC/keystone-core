package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/shawnbutts/keystone-core/pkg/module/manifest"
	"github.com/shawnbutts/keystone-core/pkg/module/sdk/starlark"
)

var (
	testVerbose  bool
	testFilter   string
	testTimeout  time.Duration
	testCoverage bool
)

var testCmd = &cobra.Command{
	Use:   "test [path]",
	Short: "Run module tests",
	Long: `Run tests for a Starlark module.

Tests are discovered in the tests/ directory and must:
  - Have filenames ending in _test.star
  - Contain functions starting with test_

Available assertions:
  - assert_eq(actual, expected)      Assert equality
  - assert_ne(actual, expected)      Assert inequality
  - assert_true(value)               Assert truthy
  - assert_false(value)              Assert falsy
  - assert_fail(fn)                  Assert function raises error
  - assert_contains(haystack, needle) Assert string contains

Examples:
  # Run all tests
  kscorectl module test

  # Run tests in specific directory
  kscorectl module test ./my-module

  # Run specific test
  kscorectl module test --filter test_my_function

  # Verbose output
  kscorectl module test -v`,
	Args: cobra.MaximumNArgs(1),
	RunE: testExecute,
}

func init() {
	testCmd.Flags().BoolVarP(&testVerbose, "verbose", "v", false, "Verbose output")
	testCmd.Flags().StringVar(&testFilter, "filter", "", "Filter tests by name pattern")
	testCmd.Flags().DurationVar(&testTimeout, "timeout", 5*time.Minute, "Test timeout")
	testCmd.Flags().BoolVar(&testCoverage, "coverage", false, "Enable coverage (not yet implemented)")
}

func testExecute(cmd *cobra.Command, args []string) error {
	if testCoverage {
		return fmt.Errorf("starlark code coverage tracking not yet implemented")
	}

	// Determine path
	modulePath := "."
	if len(args) > 0 {
		modulePath = args[0]
	}

	absPath, err := filepath.Abs(modulePath)
	if err != nil {
		return fmt.Errorf("invalid path: %w", err)
	}

	// Parse module.yaml
	manifestPath := filepath.Join(absPath, "module.yaml")
	m, err := manifest.ParseFile(manifestPath)
	if err != nil {
		return fmt.Errorf("failed to parse module.yaml: %w", err)
	}

	// Check module type
	if m.Type != "starlark" && m.Type != "hybrid" {
		return fmt.Errorf("test command only supports Starlark modules (got type: %s)", m.Type)
	}

	fmt.Printf("Running tests for: %s v%s\n", m.Name, m.Version)

	// Find test files
	testsDir := filepath.Join(absPath, "tests")
	if _, err := os.Stat(testsDir); os.IsNotExist(err) {
		fmt.Println("\n⚠ No tests directory found")
		fmt.Printf("Create tests in: %s\n", testsDir)
		return nil
	}

	testFiles, err := findTestFiles(testsDir)
	if err != nil {
		return fmt.Errorf("failed to find test files: %w", err)
	}

	if len(testFiles) == 0 {
		fmt.Println("\n⚠ No test files found (files must end with _test.star)")
		return nil
	}

	fmt.Printf("Found %d test file(s)\n\n", len(testFiles))

	// Create test runner
	runner := starlark.NewTestRunner()

	// Run tests
	startTime := time.Now()
	var totalTests, passed, failed, skipped int

	for _, testFile := range testFiles {
		relPath, _ := filepath.Rel(absPath, testFile)
		if testVerbose {
			fmt.Printf("=== %s ===\n", relPath)
		}

		suite, err := runner.RunTestFile(testFile)
		if err != nil {
			fmt.Printf("✗ %s: %v\n", relPath, err)
			failed++
			continue
		}

		for _, tc := range suite.Tests {
			totalTests++

			// Apply filter
			if testFilter != "" && !strings.Contains(tc.Name, testFilter) {
				skipped++
				continue
			}

			if tc.Passed {
				passed++
				if testVerbose {
					fmt.Printf("  ✓ %s\n", tc.Name)
				}
			} else {
				failed++
				fmt.Printf("  ✗ %s\n", tc.Name)
				if tc.Error != "" {
					fmt.Printf("    Error: %s\n", tc.Error)
				}
			}
		}

		if !testVerbose && suite.Failed == 0 {
			fmt.Printf("✓ %s (%d tests)\n", relPath, len(suite.Tests))
		} else if suite.Failed > 0 {
			fmt.Printf("✗ %s (%d passed, %d failed)\n", relPath, suite.Passed, suite.Failed)
		}
	}

	duration := time.Since(startTime)

	// Print summary
	fmt.Println()
	fmt.Println("=== Summary ===")
	fmt.Printf("Total:   %d\n", totalTests)
	fmt.Printf("Passed:  %d\n", passed)
	fmt.Printf("Failed:  %d\n", failed)
	if skipped > 0 {
		fmt.Printf("Skipped: %d\n", skipped)
	}
	fmt.Printf("Time:    %s\n", duration.Round(time.Millisecond))

	if failed > 0 {
		fmt.Println("\n✗ Tests failed!")
		os.Exit(1)
	}

	fmt.Println("\n✓ All tests passed!")
	return nil
}

func findTestFiles(dir string) ([]string, error) {
	var files []string

	err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		if strings.HasSuffix(info.Name(), "_test.star") {
			files = append(files, path)
		}
		return nil
	})

	return files, err
}
