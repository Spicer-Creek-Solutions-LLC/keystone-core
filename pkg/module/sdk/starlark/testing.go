package starlark

import (
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"go.starlark.net/starlark"
	"go.starlark.net/starlarkstruct"
)

// TestResult represents the result of a single test
type TestResult struct {
	Name     string
	Passed   bool
	Error    string
	Duration time.Duration
}

// TestSuite represents a collection of test results
type TestSuite struct {
	File    string
	Tests   []*TestResult
	Passed  int
	Failed  int
	Skipped int
	Total   int
}

// TestRunner runs Starlark tests
type TestRunner struct {
	thread *starlark.Thread
	env    starlark.StringDict
}

// NewTestRunner creates a new test runner
func NewTestRunner() *TestRunner {
	return &TestRunner{
		thread: &starlark.Thread{Name: "test"},
		env:    make(starlark.StringDict),
	}
}

// RunTestFile runs all tests in a Starlark test file
func (r *TestRunner) RunTestFile(path string) (*TestSuite, error) {
	suite := &TestSuite{
		File:  path,
		Tests: make([]*TestResult, 0),
	}

	// Create environment with assert module
	env := make(starlark.StringDict)
	env["assert"] = assertModule()

	// Load the test file
	globals, err := starlark.ExecFile(r.thread, path, nil, env) //nolint:staticcheck // SA1019: starlark.ExecFile is deprecated but requires API migration to starlark.ExecFileOptions
	if err != nil {
		return nil, fmt.Errorf("failed to load test file: %w", err)
	}

	// Find all test functions
	for name, value := range globals {
		if !strings.HasPrefix(name, "test_") {
			continue
		}

		fn, ok := value.(starlark.Callable)
		if !ok {
			continue
		}

		// Run the test
		result := r.runTest(name, fn)
		suite.Tests = append(suite.Tests, result)
		suite.Total++

		if result.Passed {
			suite.Passed++
		} else {
			suite.Failed++
		}
	}

	return suite, nil
}

func (r *TestRunner) runTest(name string, fn starlark.Callable) *TestResult {
	result := &TestResult{
		Name:   name,
		Passed: true,
	}

	start := time.Now()
	defer func() {
		result.Duration = time.Since(start)
	}()

	// Create test environment
	thread := &starlark.Thread{
		Name: name,
		Print: func(_ *starlark.Thread, msg string) {
			// Capture print output for testing
		},
	}

	// Run the test (assert is already available in globals from file load)
	_, err := starlark.Call(thread, fn, nil, nil)
	if err != nil {
		result.Passed = false
		result.Error = err.Error()
	}

	return result
}

// assertModule returns the assertion module for tests
func assertModule() *starlarkstruct.Module {
	return &starlarkstruct.Module{
		Name: "assert",
		Members: starlark.StringDict{
			"eq":       starlark.NewBuiltin("assert.eq", assertEq),
			"ne":       starlark.NewBuiltin("assert.ne", assertNe),
			"true":     starlark.NewBuiltin("assert.true", assertTrue),
			"false":    starlark.NewBuiltin("assert.false", assertFalse),
			"fail":     starlark.NewBuiltin("assert.fail", assertFail),
			"contains": starlark.NewBuiltin("assert.contains", assertContains),
		},
	}
}

// assertEq asserts that two values are equal
func assertEq(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var got, want starlark.Value
	if err := starlark.UnpackArgs("assert.eq", args, kwargs, "got", &got, "want", &want); err != nil {
		return nil, err
	}

	eq, err := starlark.Equal(got, want)
	if err != nil {
		return nil, err
	}
	if !eq {
		return nil, fmt.Errorf("assertion failed: got %v, want %v", got, want)
	}

	return starlark.None, nil
}

// assertNe asserts that two values are not equal
func assertNe(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var got, want starlark.Value
	if err := starlark.UnpackArgs("assert.ne", args, kwargs, "got", &got, "want", &want); err != nil {
		return nil, err
	}

	eq, err := starlark.Equal(got, want)
	if err != nil {
		return nil, err
	}
	if eq {
		return nil, fmt.Errorf("assertion failed: values should not be equal: %v", got)
	}

	return starlark.None, nil
}

// assertTrue asserts that a value is truthy
func assertTrue(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var val starlark.Value
	if err := starlark.UnpackArgs("assert.true", args, kwargs, "val", &val); err != nil {
		return nil, err
	}

	if !bool(val.Truth()) {
		return nil, fmt.Errorf("assertion failed: expected truthy value, got %v", val)
	}

	return starlark.None, nil
}

// assertFalse asserts that a value is falsy
func assertFalse(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var val starlark.Value
	if err := starlark.UnpackArgs("assert.false", args, kwargs, "val", &val); err != nil {
		return nil, err
	}

	if bool(val.Truth()) {
		return nil, fmt.Errorf("assertion failed: expected falsy value, got %v", val)
	}

	return starlark.None, nil
}

// assertFail fails a test with a message
func assertFail(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var msg string
	if err := starlark.UnpackArgs("assert.fail", args, kwargs, "msg", &msg); err != nil {
		return nil, err
	}

	return nil, fmt.Errorf("test failed: %s", msg)
}

// assertContains asserts that a collection contains a value
func assertContains(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var collection, item starlark.Value
	if err := starlark.UnpackArgs("assert.contains", args, kwargs, "collection", &collection, "item", &item); err != nil {
		return nil, err
	}

	// Handle different collection types
	switch coll := collection.(type) {
	case *starlark.List:
		iter := coll.Iterate()
		defer iter.Done()
		var val starlark.Value
		for iter.Next(&val) {
			eq, err := starlark.Equal(val, item)
			if err != nil {
				return nil, err
			}
			if eq {
				return starlark.None, nil
			}
		}
		return nil, fmt.Errorf("assertion failed: %v not found in list", item)

	case *starlark.Dict:
		if _, found, _ := coll.Get(item); found {
			return starlark.None, nil
		}
		return nil, fmt.Errorf("assertion failed: key %v not found in dict", item)

	case starlark.String:
		itemStr, ok := item.(starlark.String)
		if !ok {
			return nil, fmt.Errorf("assertion failed: item must be string when collection is string")
		}
		if strings.Contains(string(coll), string(itemStr)) {
			return starlark.None, nil
		}
		return nil, fmt.Errorf("assertion failed: %v not found in string", item)

	default:
		return nil, fmt.Errorf("assertion failed: unsupported collection type %T", collection)
	}
}

// RunTestsInDir runs all test files in a directory
func (r *TestRunner) RunTestsInDir(dir string) ([]*TestSuite, error) {
	pattern := filepath.Join(dir, "*_test.star")
	matches, err := filepath.Glob(pattern)
	if err != nil {
		return nil, fmt.Errorf("failed to find test files: %w", err)
	}

	suites := make([]*TestSuite, 0, len(matches))
	for _, path := range matches {
		suite, err := r.RunTestFile(path)
		if err != nil {
			return nil, fmt.Errorf("failed to run test file %s: %w", path, err)
		}
		suites = append(suites, suite)
	}

	return suites, nil
}

// FormatResults formats test results for display
func FormatResults(suites []*TestSuite) string {
	var sb strings.Builder

	totalPassed := 0
	totalFailed := 0
	totalTests := 0

	for _, suite := range suites {
		sb.WriteString(fmt.Sprintf("\n%s:\n", suite.File))
		for _, test := range suite.Tests {
			if test.Passed {
				sb.WriteString(fmt.Sprintf("  ✓ %s (%v)\n", test.Name, test.Duration))
			} else {
				sb.WriteString(fmt.Sprintf("  ✗ %s (%v)\n", test.Name, test.Duration))
				sb.WriteString(fmt.Sprintf("    Error: %s\n", test.Error))
			}
		}

		totalPassed += suite.Passed
		totalFailed += suite.Failed
		totalTests += suite.Total
	}

	sb.WriteString(fmt.Sprintf("\nTotal: %d tests, %d passed, %d failed\n",
		totalTests, totalPassed, totalFailed))

	return sb.String()
}
