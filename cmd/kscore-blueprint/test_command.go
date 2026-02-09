package main

import (
	"context"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/shawnbutts/keystone-core/internal/blueprint"
	bptesting "github.com/shawnbutts/keystone-core/internal/blueprint/testing"
)

var (
	testVerbose     bool
	testDryRun      bool
	testTimeout     time.Duration
	testParallel    bool
	testMaxParallel int
	testTags        []string
	testExcludeTags []string
	testPattern     string
	testFormat      string
	testOutput      string
	testStopOnFail  bool
)

var testCmd = &cobra.Command{
	Use:   "test [path]",
	Short: "Run blueprint tests",
	Long: `Run tests for a blueprint.

Tests are defined in YAML files in the 'tests/' directory of a blueprint.
Each test file can contain multiple test cases with setup, assertions,
and teardown steps.

Test Structure:
  tests/
    ├── basic_test.yaml       # Basic functionality tests
    ├── validation_test.yaml  # Parameter validation tests
    └── integration_test.yaml # Integration tests

Test File Format:
  name: Basic Tests
  description: Test basic blueprint functionality
  tests:
    - name: default parameters
      assertions:
        - type: no_failures
    - name: custom port
      parameters:
        port: 8080
      assertions:
        - type: state_applied
          target: configure_nginx

Output Formats:
  - text:  Human-readable console output (default)
  - json:  JSON report for CI/CD integration
  - junit: JUnit XML for test reporting tools

Examples:
  # Run all tests in current blueprint
  kscorectl blueprint test .

  # Run with verbose output
  kscorectl blueprint test . -v

  # Run specific test pattern
  kscorectl blueprint test . --pattern "validation*"

  # Run only tagged tests
  kscorectl blueprint test . --tags "quick,smoke"

  # Output JUnit XML for CI
  kscorectl blueprint test . --format junit -o results.xml

  # Dry run (no actual changes)
  kscorectl blueprint test . --dry-run`,
	Args: cobra.MaximumNArgs(1),
	RunE: testExecute,
}

func init() {
	testCmd.Flags().BoolVarP(&testVerbose, "verbose", "v", false, "Verbose output")
	testCmd.Flags().BoolVar(&testDryRun, "dry-run", false, "Dry run (no actual changes)")
	testCmd.Flags().DurationVar(&testTimeout, "timeout", 5*time.Minute, "Default test timeout")
	testCmd.Flags().BoolVar(&testParallel, "parallel", false, "Run tests in parallel")
	testCmd.Flags().IntVar(&testMaxParallel, "max-parallel", 4, "Maximum parallel tests")
	testCmd.Flags().StringSliceVar(&testTags, "tags", nil, "Only run tests with these tags")
	testCmd.Flags().StringSliceVar(&testExcludeTags, "exclude-tags", nil, "Exclude tests with these tags")
	testCmd.Flags().StringVar(&testPattern, "pattern", "", "Only run tests matching pattern")
	testCmd.Flags().StringVar(&testFormat, "format", "text", "Output format (text, json, junit)")
	testCmd.Flags().StringVarP(&testOutput, "output", "o", "", "Write output to file")
	testCmd.Flags().BoolVar(&testStopOnFail, "stop-on-failure", false, "Stop on first failure")
}

func testExecute(cmd *cobra.Command, args []string) error {
	path := "."
	if len(args) > 0 {
		path = args[0]
	}

	// Verify blueprint exists
	manifestPath := filepath.Join(path, "blueprint.yaml")
	if _, err := os.Stat(manifestPath); os.IsNotExist(err) {
		return fmt.Errorf("blueprint.yaml not found in %s", path)
	}

	// Find test files
	testsDir := filepath.Join(path, "tests")
	if _, err := os.Stat(testsDir); os.IsNotExist(err) {
		fmt.Println("No tests directory found")
		return nil
	}

	// Load test suites
	loader := bptesting.NewTestLoader(testsDir)
	suites, err := loader.LoadDirectory(".")
	if err != nil {
		return fmt.Errorf("failed to load tests: %w", err)
	}

	if len(suites) == 0 {
		fmt.Println("No test suites found")
		return nil
	}

	// Create storage and runner
	storage, err := blueprint.NewLocalStorage(path, false)
	if err != nil {
		return fmt.Errorf("failed to create storage: %w", err)
	}

	runnerConfig := &bptesting.RunnerConfig{
		BlueprintPath: path,
		DryRun:        testDryRun,
		Timeout:       testTimeout,
		Parallel:      testParallel,
		MaxParallel:   testMaxParallel,
		StopOnFailure: testStopOnFail,
		Verbose:       testVerbose,
		MockRegistry:  bptesting.NewMockRegistry(),
	}

	// Add event handler for verbose output
	if testVerbose {
		runnerConfig.EventHandler = &verboseEventHandler{out: os.Stdout}
	}

	runner, err := bptesting.NewRunner(runnerConfig)
	if err != nil {
		return fmt.Errorf("failed to create test runner: %w", err)
	}
	_ = storage // Used by runner internally

	// Run tests
	ctx := context.Background()
	var allResults []*bptesting.TestSuiteResult

	fmt.Printf("Running blueprint tests from %s\n\n", testsDir)

	for _, suite := range suites {
		// Filter tests by tags
		if len(testTags) > 0 || len(testExcludeTags) > 0 {
			suite.Tests = bptesting.FilterTests(suite.Tests, testTags, testExcludeTags)
		}

		// Filter tests by pattern
		if testPattern != "" {
			suite.Tests = bptesting.FilterTestsByName(suite.Tests, testPattern)
		}

		if len(suite.Tests) == 0 {
			continue
		}

		if !testVerbose {
			fmt.Printf("Suite: %s (%d tests)\n", suite.Name, len(suite.Tests))
		}

		result := runner.RunSuite(ctx, suite)
		allResults = append(allResults, result)

		// Print summary for this suite if not verbose
		if !testVerbose {
			printSuiteSummary(result)
		}
	}

	// Calculate overall summary
	overallSummary := calculateOverallSummary(allResults)

	// Output results
	switch testFormat {
	case "json":
		return outputJSON(allResults, overallSummary)
	case "junit":
		return outputJUnit(allResults)
	default:
		fmt.Println()
		printOverallSummary(overallSummary)
	}

	// Exit with error if tests failed
	if overallSummary.Failed > 0 || overallSummary.Errors > 0 {
		return fmt.Errorf("%d test(s) failed", overallSummary.Failed+overallSummary.Errors)
	}

	return nil
}

// verboseEventHandler prints detailed test events
type verboseEventHandler struct {
	out *os.File
}

func (h *verboseEventHandler) OnSuiteStart(suite *bptesting.TestSuite) {
	fmt.Fprintf(h.out, "\n=== SUITE: %s ===\n", suite.Name)
	if suite.Description != "" {
		fmt.Fprintf(h.out, "    %s\n", suite.Description)
	}
	fmt.Fprintf(h.out, "    Tests: %d\n\n", len(suite.Tests))
}

func (h *verboseEventHandler) OnSuiteEnd(result *bptesting.TestSuiteResult) {
	fmt.Fprintf(h.out, "\n--- Suite %s completed ---\n", result.Name)
	fmt.Fprintf(h.out, "    Duration: %v\n", result.Duration.Round(time.Millisecond))
	fmt.Fprintf(h.out, "    Passed: %d, Failed: %d, Skipped: %d\n\n",
		result.Summary.Passed, result.Summary.Failed, result.Summary.Skipped)
}

func (h *verboseEventHandler) OnTestStart(test *bptesting.TestCase) {
	fmt.Fprintf(h.out, "  TEST: %s\n", test.Name)
	if test.Description != "" {
		fmt.Fprintf(h.out, "        %s\n", test.Description)
	}
}

func (h *verboseEventHandler) OnTestEnd(result *bptesting.TestResult) {
	status := "✓ PASS"
	switch result.Status {
	case bptesting.StatusFailed:
		status = "✗ FAIL"
	case bptesting.StatusSkipped:
		status = "○ SKIP"
	case bptesting.StatusError:
		status = "✗ ERROR"
	default:
	}

	fmt.Fprintf(h.out, "        %s (%v)\n", status, result.Duration.Round(time.Millisecond))

	if result.Error != "" {
		fmt.Fprintf(h.out, "        Error: %s\n", result.Error)
	}
	if result.SkipReason != "" {
		fmt.Fprintf(h.out, "        Reason: %s\n", result.SkipReason)
	}
}

func (h *verboseEventHandler) OnAssertionResult(result *bptesting.AssertionResult) {
	if result.Passed {
		fmt.Fprintf(h.out, "          ✓ %s\n", result.Message)
	} else {
		fmt.Fprintf(h.out, "          ✗ %s\n", result.Message)
		if result.Expected != nil {
			fmt.Fprintf(h.out, "            Expected: %v\n", result.Expected)
		}
		if result.Actual != nil {
			fmt.Fprintf(h.out, "            Actual: %v\n", result.Actual)
		}
	}
}

func printSuiteSummary(result *bptesting.TestSuiteResult) {
	for i := range result.Tests {
		test := &result.Tests[i]
		status := "✓"
		switch test.Status {
		case bptesting.StatusFailed:
			status = "✗"
		case bptesting.StatusSkipped:
			status = "○"
		case bptesting.StatusError:
			status = "!"
		default:
		}
		fmt.Printf("  %s %s (%v)\n", status, test.Name, test.Duration.Round(time.Millisecond))

		if test.Error != "" {
			fmt.Printf("    Error: %s\n", test.Error)
		}
	}
}

type overallSummary struct {
	Total    int
	Passed   int
	Failed   int
	Skipped  int
	Errors   int
	Duration time.Duration
	PassRate float64
}

func calculateOverallSummary(results []*bptesting.TestSuiteResult) overallSummary {
	summary := overallSummary{}
	for _, r := range results {
		summary.Total += r.Summary.Total
		summary.Passed += r.Summary.Passed
		summary.Failed += r.Summary.Failed
		summary.Skipped += r.Summary.Skipped
		summary.Errors += r.Summary.Errors
		summary.Duration += r.Duration
	}
	if summary.Total-summary.Skipped > 0 {
		summary.PassRate = float64(summary.Passed) / float64(summary.Total-summary.Skipped) * 100
	}
	return summary
}

func printOverallSummary(summary overallSummary) {
	fmt.Println("═══════════════════════════════════════════")
	fmt.Println("Test Summary")
	fmt.Println("═══════════════════════════════════════════")
	fmt.Printf("  Total:   %d\n", summary.Total)
	fmt.Printf("  Passed:  %d\n", summary.Passed)
	fmt.Printf("  Failed:  %d\n", summary.Failed)
	fmt.Printf("  Skipped: %d\n", summary.Skipped)
	fmt.Printf("  Errors:  %d\n", summary.Errors)
	fmt.Printf("  Duration: %v\n", summary.Duration.Round(time.Millisecond))
	fmt.Printf("  Pass Rate: %.1f%%\n", summary.PassRate)
	fmt.Println("═══════════════════════════════════════════")

	if summary.Failed == 0 && summary.Errors == 0 {
		fmt.Println("\n✓ All tests passed!")
	} else {
		fmt.Printf("\n✗ %d test(s) failed\n", summary.Failed+summary.Errors)
	}
}

func outputJSON(results []*bptesting.TestSuiteResult, summary overallSummary) error {
	output := struct {
		Summary overallSummary               `json:"summary"`
		Suites  []*bptesting.TestSuiteResult `json:"suites"`
	}{
		Summary: summary,
		Suites:  results,
	}

	data, err := json.MarshalIndent(output, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal JSON: %w", err)
	}

	if testOutput != "" {
		//nolint:gosec // G306: test output files need to be readable by CI systems
		return os.WriteFile(testOutput, data, 0o644)
	}

	fmt.Println(string(data))
	return nil
}

// JUnit XML types for test reporting
type junitTestSuites struct {
	XMLName  xml.Name         `xml:"testsuites"`
	Tests    int              `xml:"tests,attr"`
	Failures int              `xml:"failures,attr"`
	Errors   int              `xml:"errors,attr"`
	Skipped  int              `xml:"skipped,attr"`
	Time     float64          `xml:"time,attr"`
	Suites   []junitTestSuite `xml:"testsuite"`
}

type junitTestSuite struct {
	Name     string          `xml:"name,attr"`
	Tests    int             `xml:"tests,attr"`
	Failures int             `xml:"failures,attr"`
	Errors   int             `xml:"errors,attr"`
	Skipped  int             `xml:"skipped,attr"`
	Time     float64         `xml:"time,attr"`
	Cases    []junitTestCase `xml:"testcase"`
}

type junitTestCase struct {
	Name      string        `xml:"name,attr"`
	Classname string        `xml:"classname,attr"`
	Time      float64       `xml:"time,attr"`
	Failure   *junitFailure `xml:"failure,omitempty"`
	Error     *junitError   `xml:"error,omitempty"`
	Skipped   *junitSkipped `xml:"skipped,omitempty"`
}

type junitFailure struct {
	Message string `xml:"message,attr"`
	Type    string `xml:"type,attr"`
	Content string `xml:",chardata"`
}

type junitError struct {
	Message string `xml:"message,attr"`
	Type    string `xml:"type,attr"`
	Content string `xml:",chardata"`
}

type junitSkipped struct {
	Message string `xml:"message,attr,omitempty"`
}

func outputJUnit(results []*bptesting.TestSuiteResult) error {
	suites := junitTestSuites{}

	for _, result := range results {
		suite := junitTestSuite{
			Name:     result.Name,
			Tests:    result.Summary.Total,
			Failures: result.Summary.Failed,
			Errors:   result.Summary.Errors,
			Skipped:  result.Summary.Skipped,
			Time:     result.Duration.Seconds(),
		}

		for i := range result.Tests {
			test := &result.Tests[i]
			tc := junitTestCase{
				Name:      test.Name,
				Classname: result.Name,
				Time:      test.Duration.Seconds(),
			}

			switch test.Status {
			case bptesting.StatusFailed:
				var messages []string
				for _, a := range test.AssertionResults {
					if !a.Passed {
						messages = append(messages, a.Message)
					}
				}
				tc.Failure = &junitFailure{
					Message: test.Error,
					Type:    "AssertionFailure",
					Content: strings.Join(messages, "\n"),
				}
			case bptesting.StatusError:
				tc.Error = &junitError{
					Message: test.Error,
					Type:    "TestError",
				}
			case bptesting.StatusSkipped:
				tc.Skipped = &junitSkipped{
					Message: test.SkipReason,
				}
			default:
			}

			suite.Cases = append(suite.Cases, tc)
		}

		suites.Suites = append(suites.Suites, suite)
		suites.Tests += suite.Tests
		suites.Failures += suite.Failures
		suites.Errors += suite.Errors
		suites.Skipped += suite.Skipped
		suites.Time += suite.Time
	}

	data, err := xml.MarshalIndent(suites, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal JUnit XML: %w", err)
	}

	output := xml.Header + string(data)

	if testOutput != "" {
		//nolint:gosec // G306: test output files need to be readable by CI systems
		return os.WriteFile(testOutput, []byte(output), 0o644)
	}

	fmt.Println(output)
	return nil
}
