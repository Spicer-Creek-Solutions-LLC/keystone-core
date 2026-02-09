package testing

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

// TestLoader loads and parses test suites from files.
type TestLoader struct {
	// BasePath is the base directory for resolving relative paths.
	BasePath string
}

// NewTestLoader creates a new test loader.
func NewTestLoader(basePath string) *TestLoader {
	return &TestLoader{
		BasePath: basePath,
	}
}

// LoadFile loads a test suite from a single YAML file.
func (l *TestLoader) LoadFile(path string) (*TestSuite, error) {
	absPath := l.resolvePath(path)

	data, err := os.ReadFile(absPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read test file %s: %w", path, err)
	}

	suite, err := l.parseTestSuite(data, path)
	if err != nil {
		return nil, fmt.Errorf("failed to parse test file %s: %w", path, err)
	}

	return suite, nil
}

// LoadDirectory loads all test suites from a directory.
func (l *TestLoader) LoadDirectory(dir string) ([]*TestSuite, error) {
	absDir := l.resolvePath(dir)

	entries, err := os.ReadDir(absDir)
	if err != nil {
		return nil, fmt.Errorf("failed to read test directory %s: %w", dir, err)
	}

	var suites []*TestSuite
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		name := entry.Name()
		if !isTestFile(name) {
			continue
		}

		path := filepath.Join(absDir, name)
		suite, err := l.LoadFile(path)
		if err != nil {
			return nil, err
		}

		suites = append(suites, suite)
	}

	// Sort suites by name for consistent ordering
	sort.Slice(suites, func(i, j int) bool {
		return suites[i].Name < suites[j].Name
	})

	return suites, nil
}

// LoadBlueprint loads all tests from a blueprint's tests directory.
func (l *TestLoader) LoadBlueprint(blueprintPath string) ([]*TestSuite, error) {
	testsDir := filepath.Join(blueprintPath, "tests")

	// Check if tests directory exists
	info, err := os.Stat(testsDir)
	if os.IsNotExist(err) {
		return nil, nil // No tests directory is not an error
	}
	if err != nil {
		return nil, fmt.Errorf("failed to stat tests directory: %w", err)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("tests path is not a directory: %s", testsDir)
	}

	return l.LoadDirectory(testsDir)
}

// resolvePath resolves a path relative to BasePath.
func (l *TestLoader) resolvePath(path string) string {
	if filepath.IsAbs(path) {
		return path
	}
	return filepath.Join(l.BasePath, path)
}

// parseTestSuite parses YAML data into a TestSuite.
func (l *TestLoader) parseTestSuite(data []byte, sourcePath string) (*TestSuite, error) {
	var suite TestSuite
	if err := yaml.Unmarshal(data, &suite); err != nil {
		return nil, fmt.Errorf("YAML parse error: %w", err)
	}

	// Validate required fields
	if err := l.validateTestSuite(&suite, sourcePath); err != nil {
		return nil, err
	}

	// Apply defaults to tests
	l.applyDefaults(&suite)

	return &suite, nil
}

// validateTestSuite validates a test suite.
func (l *TestLoader) validateTestSuite(suite *TestSuite, sourcePath string) error {
	if suite.Name == "" {
		suite.Name = filepath.Base(sourcePath)
		// Remove extension
		if ext := filepath.Ext(suite.Name); ext != "" {
			suite.Name = suite.Name[:len(suite.Name)-len(ext)]
		}
	}

	if len(suite.Tests) == 0 {
		return fmt.Errorf("test suite must have at least one test")
	}

	for i := range suite.Tests {
		test := &suite.Tests[i]
		if test.Name == "" {
			return fmt.Errorf("test %d: name is required", i+1)
		}

		// Skip validation for skipped tests
		if test.Skip != "" {
			continue
		}

		// Validate assertions unless expecting failure
		if !test.ExpectFailure && test.ExpectError == "" && len(test.Assertions) == 0 {
			return fmt.Errorf("test %q: must have at least one assertion or expect_failure/expect_error", test.Name)
		}

		// Validate assertion types
		for j := range test.Assertions {
			assertion := &test.Assertions[j]
			if assertion.Type == "" {
				return fmt.Errorf("test %q assertion %d: type is required", test.Name, j+1)
			}
			if err := l.validateAssertion(assertion, test.Name, j+1); err != nil {
				return err
			}
		}
	}

	return nil
}

// validateAssertion validates an assertion.
func (l *TestLoader) validateAssertion(a *Assertion, testName string, index int) error {
	prefix := fmt.Sprintf("test %q assertion %d", testName, index)

	switch a.Type {
	case AssertStateApplied, AssertStateChanged, AssertStateUnchanged, AssertStateFailed:
		if a.State == nil && a.Target == "" {
			return fmt.Errorf("%s: state assertion requires state.id or target", prefix)
		}

	case AssertFileExists, AssertFileNotExists, AssertDirectoryExists:
		if a.File == nil && a.Target == "" {
			return fmt.Errorf("%s: file assertion requires file.path or target", prefix)
		}

	case AssertFileContains, AssertFileMode, AssertFileOwner:
		if a.File == nil {
			return fmt.Errorf("%s: file assertion requires file configuration", prefix)
		}
		if a.File.Path == "" {
			return fmt.Errorf("%s: file.path is required", prefix)
		}

	case AssertCommandSuccess, AssertCommandFailure, AssertCommandOutput:
		if a.Command == nil {
			return fmt.Errorf("%s: command assertion requires command configuration", prefix)
		}
		if a.Command.Command == "" {
			return fmt.Errorf("%s: command.command is required", prefix)
		}

	case AssertOutputContains, AssertOutputEquals, AssertOutputMatches:
		if a.Output == nil {
			return fmt.Errorf("%s: output assertion requires output configuration", prefix)
		}
		if a.Output.Name == "" {
			return fmt.Errorf("%s: output.name is required", prefix)
		}

	case AssertExpression:
		if a.Pattern == "" && a.Expected == nil {
			return fmt.Errorf("%s: expression assertion requires pattern or expected", prefix)
		}

	case AssertStatesApplied, AssertStatesChanged, AssertStatesFailed:
		if a.Expected == nil {
			return fmt.Errorf("%s: count assertion requires expected value", prefix)
		}

	case AssertNoFailures, AssertIdempotent:
		// No additional validation needed

	default:
		return fmt.Errorf("%s: unknown assertion type %q", prefix, a.Type)
	}

	return nil
}

// applyDefaults applies suite defaults to test cases.
func (l *TestLoader) applyDefaults(suite *TestSuite) {
	if suite.Defaults == nil {
		return
	}

	for i := range suite.Tests {
		test := &suite.Tests[i]

		// Apply timeout default
		if test.Timeout == 0 && suite.Defaults.Timeout != 0 {
			test.Timeout = suite.Defaults.Timeout
		}

		// Apply dry_run default
		if !test.DryRun && suite.Defaults.DryRun {
			test.DryRun = suite.Defaults.DryRun
		}

		// Merge parameters (test overrides defaults)
		if len(suite.Defaults.Parameters) > 0 {
			if test.Parameters == nil {
				test.Parameters = make(map[string]interface{})
			}
			for k, v := range suite.Defaults.Parameters {
				if _, exists := test.Parameters[k]; !exists {
					test.Parameters[k] = v
				}
			}
		}

		// Merge mocks (test mocks come after defaults)
		if len(suite.Defaults.Mocks) > 0 {
			test.Mocks = append(suite.Defaults.Mocks, test.Mocks...)
		}
	}
}

// isTestFile checks if a filename is a test file.
func isTestFile(name string) bool {
	// Must be YAML
	if !strings.HasSuffix(name, ".yaml") && !strings.HasSuffix(name, ".yml") {
		return false
	}

	// Prefer *_test.yaml pattern but accept all YAML in tests directory
	return true
}

// FilterTests filters tests by tags.
func FilterTests(tests []TestCase, includeTags, excludeTags []string) []TestCase {
	if len(includeTags) == 0 && len(excludeTags) == 0 {
		return tests
	}

	var filtered []TestCase
	for i := range tests {
		test := &tests[i]
		// Check include tags
		if len(includeTags) > 0 {
			if !hasAnyTag(test.Tags, includeTags) {
				continue
			}
		}

		// Check exclude tags
		if len(excludeTags) > 0 {
			if hasAnyTag(test.Tags, excludeTags) {
				continue
			}
		}

		filtered = append(filtered, tests[i])
	}

	return filtered
}

// hasAnyTag checks if tags contains any of the target tags.
func hasAnyTag(tags, targets []string) bool {
	tagSet := make(map[string]bool)
	for _, t := range tags {
		tagSet[t] = true
	}
	for _, t := range targets {
		if tagSet[t] {
			return true
		}
	}
	return false
}

// FilterTestsByName filters tests by name pattern.
func FilterTestsByName(tests []TestCase, pattern string) []TestCase {
	if pattern == "" {
		return tests
	}

	var filtered []TestCase
	for i := range tests {
		if matchesPattern(tests[i].Name, pattern) {
			filtered = append(filtered, tests[i])
		}
	}

	return filtered
}

// matchesPattern checks if name matches a glob-like pattern.
func matchesPattern(name, pattern string) bool {
	// Simple glob matching with * wildcard
	if pattern == "*" {
		return true
	}

	// Exact match
	if !strings.Contains(pattern, "*") {
		return name == pattern
	}

	// Convert glob to prefix/suffix matching
	if strings.HasPrefix(pattern, "*") && strings.HasSuffix(pattern, "*") {
		// *substring*
		middle := pattern[1 : len(pattern)-1]
		return strings.Contains(name, middle)
	}

	if strings.HasPrefix(pattern, "*") {
		// *suffix
		suffix := pattern[1:]
		return strings.HasSuffix(name, suffix)
	}

	if strings.HasSuffix(pattern, "*") {
		// prefix*
		prefix := pattern[:len(pattern)-1]
		return strings.HasPrefix(name, prefix)
	}

	// Complex pattern with * in middle
	parts := strings.Split(pattern, "*")

	// First part must match at the beginning if non-empty
	if parts[0] != "" {
		if !strings.HasPrefix(name, parts[0]) {
			return false
		}
	}

	// Last part must match at the end if non-empty
	if parts[len(parts)-1] != "" {
		if !strings.HasSuffix(name, parts[len(parts)-1]) {
			return false
		}
	}

	// Middle parts must appear in order
	remaining := name
	for i, part := range parts {
		if part == "" {
			continue
		}
		// First part already checked at start
		if i == 0 {
			remaining = remaining[len(part):]
			continue
		}
		idx := strings.Index(remaining, part)
		if idx == -1 {
			return false
		}
		remaining = remaining[idx+len(part):]
	}

	return true
}
