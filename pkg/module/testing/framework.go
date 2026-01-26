// Package testing provides a comprehensive testing framework for Keystone modules
package testing

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/shawnbutts/keystone-core/pkg/wait"
)

// Framework is the main module testing framework
type Framework struct {
	// Config holds framework configuration
	Config *FrameworkConfig

	// Mocks holds registered mocks
	mocks map[string]MockFunc

	// Fixtures holds test fixtures
	fixtures map[string]interface{}

	// Reporters holds test reporters
	reporters []Reporter

	// mu protects concurrent access
	mu sync.RWMutex
}

// FrameworkConfig configures the testing framework
type FrameworkConfig struct {
	// Verbose enables verbose output
	Verbose bool `json:"verbose" yaml:"verbose"`

	// Parallel enables parallel test execution
	Parallel bool `json:"parallel" yaml:"parallel"`

	// MaxParallel limits the number of parallel tests
	MaxParallel int `json:"max_parallel" yaml:"max_parallel"`

	// Timeout is the default test timeout
	Timeout time.Duration `json:"timeout" yaml:"timeout"`

	// FailFast stops on first failure
	FailFast bool `json:"fail_fast" yaml:"fail_fast"`

	// Pattern filters tests by name pattern
	Pattern string `json:"pattern" yaml:"pattern"`

	// Tags filters tests by tags
	Tags []string `json:"tags" yaml:"tags"`

	// SkipTags excludes tests with these tags
	SkipTags []string `json:"skip_tags" yaml:"skip_tags"`

	// CoverageEnabled enables coverage tracking
	CoverageEnabled bool `json:"coverage_enabled" yaml:"coverage_enabled"`

	// CoverageDir is where coverage reports are written
	CoverageDir string `json:"coverage_dir" yaml:"coverage_dir"`

	// FixturesDir is where test fixtures are loaded from
	FixturesDir string `json:"fixtures_dir" yaml:"fixtures_dir"`
}

// DefaultConfig returns a default configuration
func DefaultConfig() *FrameworkConfig {
	return &FrameworkConfig{
		Verbose:     false,
		Parallel:    true,
		MaxParallel: 4,
		Timeout:     30 * time.Second,
		FailFast:    false,
	}
}

// NewFramework creates a new testing framework instance
func NewFramework(config *FrameworkConfig) *Framework {
	if config == nil {
		config = DefaultConfig()
	}
	return &Framework{
		Config:    config,
		mocks:     make(map[string]MockFunc),
		fixtures:  make(map[string]interface{}),
		reporters: []Reporter{&ConsoleReporter{Verbose: config.Verbose}},
	}
}

// TestCase represents a single test case
type TestCase struct {
	// Name is the test name
	Name string `json:"name"`

	// Description describes what the test verifies
	Description string `json:"description,omitempty"`

	// Tags are labels for filtering
	Tags []string `json:"tags,omitempty"`

	// Setup runs before the test
	Setup SetupFunc `json:"-"`

	// Teardown runs after the test
	Teardown TeardownFunc `json:"-"`

	// Run is the test function
	Run TestFunc `json:"-"`

	// Timeout overrides the default timeout
	Timeout time.Duration `json:"timeout,omitempty"`

	// Skip marks the test as skipped
	Skip bool `json:"skip,omitempty"`

	// SkipReason explains why the test is skipped
	SkipReason string `json:"skip_reason,omitempty"`

	// Parallel marks the test as safe for parallel execution
	Parallel bool `json:"parallel,omitempty"`

	// Parameters for parameterized tests
	Parameters []map[string]interface{} `json:"parameters,omitempty"`
}

// TestFunc is the signature for test functions
type TestFunc func(t *T) error

// SetupFunc is the signature for setup functions
type SetupFunc func(t *T) error

// TeardownFunc is the signature for teardown functions
type TeardownFunc func(t *T) error

// MockFunc is the signature for mock functions
type MockFunc func(args ...interface{}) (interface{}, error)

// T provides testing utilities within a test
type T struct {
	// Name is the current test name
	Name string

	// framework is the parent framework
	framework *Framework

	// failed indicates if the test has failed
	failed bool

	// skipped indicates if the test was skipped
	skipped bool

	// skipReason explains why the test was skipped
	skipReason string

	// logs contains test logs
	logs []string

	// errors contains test errors
	errors []string

	// startTime is when the test started
	startTime time.Time

	// context holds test-local data
	context map[string]interface{}

	// mu protects concurrent access
	mu sync.Mutex
}

// TestResult represents the result of running a test
type TestResult struct {
	// Name is the test name
	Name string `json:"name"`

	// Status is the test status
	Status TestStatus `json:"status"`

	// Duration is how long the test took
	Duration time.Duration `json:"duration"`

	// Error is the error message if failed
	Error string `json:"error,omitempty"`

	// Logs are test log messages
	Logs []string `json:"logs,omitempty"`

	// Parameters are the test parameters (for parameterized tests)
	Parameters map[string]interface{} `json:"parameters,omitempty"`

	// Coverage is the coverage info for this test
	Coverage *CoverageInfo `json:"coverage,omitempty"`
}

// TestStatus represents the status of a test
type TestStatus string

const (
	TestStatusPassed  TestStatus = "passed"
	TestStatusFailed  TestStatus = "failed"
	TestStatusSkipped TestStatus = "skipped"
	TestStatusError   TestStatus = "error"
)

// TestSuiteResult represents the result of running a test suite
type TestSuiteResult struct {
	// Name is the suite name
	Name string `json:"name"`

	// Tests are the individual test results
	Tests []*TestResult `json:"tests"`

	// Passed is the count of passed tests
	Passed int `json:"passed"`

	// Failed is the count of failed tests
	Failed int `json:"failed"`

	// Skipped is the count of skipped tests
	Skipped int `json:"skipped"`

	// Errors is the count of tests that errored
	Errors int `json:"errors"`

	// Total is the total test count
	Total int `json:"total"`

	// Duration is how long the suite took
	Duration time.Duration `json:"duration"`

	// StartTime is when the suite started
	StartTime time.Time `json:"start_time"`

	// EndTime is when the suite ended
	EndTime time.Time `json:"end_time"`

	// Coverage is the aggregate coverage info
	Coverage *CoverageInfo `json:"coverage,omitempty"`
}

// CoverageInfo holds code coverage information
type CoverageInfo struct {
	// LinesTotal is the total lines
	LinesTotal int `json:"lines_total"`

	// LinesCovered is the covered lines
	LinesCovered int `json:"lines_covered"`

	// Percentage is the coverage percentage
	Percentage float64 `json:"percentage"`

	// Files maps file paths to their coverage
	Files map[string]*FileCoverage `json:"files,omitempty"`
}

// FileCoverage holds coverage for a single file
type FileCoverage struct {
	// Path is the file path
	Path string `json:"path"`

	// LinesTotal is the total lines
	LinesTotal int `json:"lines_total"`

	// LinesCovered is the covered lines
	LinesCovered int `json:"lines_covered"`

	// CoveredLines lists which lines were covered
	CoveredLines []int `json:"covered_lines,omitempty"`
}

// Reporter reports test results
type Reporter interface {
	// OnSuiteStart is called when a suite starts
	OnSuiteStart(name string)

	// OnTestStart is called when a test starts
	OnTestStart(name string)

	// OnTestComplete is called when a test completes
	OnTestComplete(result *TestResult)

	// OnSuiteComplete is called when a suite completes
	OnSuiteComplete(result *TestSuiteResult)
}

// ConsoleReporter reports to the console
type ConsoleReporter struct {
	Verbose bool
}

func (r *ConsoleReporter) OnSuiteStart(name string) {
	fmt.Printf("Running test suite: %s\n", name)
}

func (r *ConsoleReporter) OnTestStart(name string) {
	if r.Verbose {
		fmt.Printf("  Running: %s...\n", name)
	}
}

func (r *ConsoleReporter) OnTestComplete(result *TestResult) {
	switch result.Status {
	case TestStatusPassed:
		fmt.Printf("  ✓ %s (%v)\n", result.Name, result.Duration)
	case TestStatusFailed:
		fmt.Printf("  ✗ %s (%v)\n", result.Name, result.Duration)
		if result.Error != "" {
			fmt.Printf("    Error: %s\n", result.Error)
		}
	case TestStatusSkipped:
		fmt.Printf("  ○ %s (skipped)\n", result.Name)
	case TestStatusError:
		fmt.Printf("  ! %s (error: %s)\n", result.Name, result.Error)
	}
}

func (r *ConsoleReporter) OnSuiteComplete(result *TestSuiteResult) {
	fmt.Printf("\nResults: %d passed, %d failed, %d skipped, %d errors (%v)\n",
		result.Passed, result.Failed, result.Skipped, result.Errors, result.Duration)

	if result.Coverage != nil {
		fmt.Printf("Coverage: %.1f%% (%d/%d lines)\n",
			result.Coverage.Percentage, result.Coverage.LinesCovered, result.Coverage.LinesTotal)
	}
}

// JUnitReporter outputs JUnit XML format
type JUnitReporter struct {
	OutputPath string
}

func (r *JUnitReporter) OnSuiteStart(name string)          {}
func (r *JUnitReporter) OnTestStart(name string)           {}
func (r *JUnitReporter) OnTestComplete(result *TestResult) {}

func (r *JUnitReporter) OnSuiteComplete(result *TestSuiteResult) {
	// Generate JUnit XML format for CI integration
	xml := r.generateXML(result)
	if r.OutputPath != "" {
		os.WriteFile(r.OutputPath, []byte(xml), 0644)
	}
}

func (r *JUnitReporter) generateXML(result *TestSuiteResult) string {
	var sb strings.Builder
	sb.WriteString(`<?xml version="1.0" encoding="UTF-8"?>`)
	sb.WriteString(fmt.Sprintf(`<testsuite name="%s" tests="%d" failures="%d" errors="%d" skipped="%d" time="%.3f">`,
		result.Name, result.Total, result.Failed, result.Errors, result.Skipped, result.Duration.Seconds()))

	for _, test := range result.Tests {
		sb.WriteString(fmt.Sprintf(`<testcase name="%s" time="%.3f">`, test.Name, test.Duration.Seconds()))
		if test.Status == TestStatusFailed {
			sb.WriteString(fmt.Sprintf(`<failure message="%s"/>`, escapeXML(test.Error)))
		} else if test.Status == TestStatusSkipped {
			sb.WriteString(`<skipped/>`)
		} else if test.Status == TestStatusError {
			sb.WriteString(fmt.Sprintf(`<error message="%s"/>`, escapeXML(test.Error)))
		}
		sb.WriteString(`</testcase>`)
	}

	sb.WriteString(`</testsuite>`)
	return sb.String()
}

func escapeXML(s string) string {
	s = strings.ReplaceAll(s, "&", "&amp;")
	s = strings.ReplaceAll(s, "<", "&lt;")
	s = strings.ReplaceAll(s, ">", "&gt;")
	s = strings.ReplaceAll(s, "\"", "&quot;")
	return s
}

// JSONReporter outputs JSON format
type JSONReporter struct {
	OutputPath string
}

func (r *JSONReporter) OnSuiteStart(name string)          {}
func (r *JSONReporter) OnTestStart(name string)           {}
func (r *JSONReporter) OnTestComplete(result *TestResult) {}

func (r *JSONReporter) OnSuiteComplete(result *TestSuiteResult) {
	data, _ := json.MarshalIndent(result, "", "  ")
	if r.OutputPath != "" {
		os.WriteFile(r.OutputPath, data, 0644)
	}
}

// RegisterMock registers a mock function
func (f *Framework) RegisterMock(name string, fn MockFunc) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.mocks[name] = fn
}

// GetMock returns a registered mock
func (f *Framework) GetMock(name string) (MockFunc, bool) {
	f.mu.RLock()
	defer f.mu.RUnlock()
	fn, ok := f.mocks[name]
	return fn, ok
}

// LoadFixture loads a test fixture
func (f *Framework) LoadFixture(name string, path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("failed to load fixture %s: %w", name, err)
	}

	var fixture interface{}
	if strings.HasSuffix(path, ".json") {
		if err := json.Unmarshal(data, &fixture); err != nil {
			return fmt.Errorf("failed to parse fixture %s: %w", name, err)
		}
	} else {
		fixture = string(data)
	}

	f.mu.Lock()
	defer f.mu.Unlock()
	f.fixtures[name] = fixture
	return nil
}

// GetFixture returns a loaded fixture
func (f *Framework) GetFixture(name string) (interface{}, bool) {
	f.mu.RLock()
	defer f.mu.RUnlock()
	fixture, ok := f.fixtures[name]
	return fixture, ok
}

// AddReporter adds a test reporter
func (f *Framework) AddReporter(r Reporter) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.reporters = append(f.reporters, r)
}

// Run runs a test suite
func (f *Framework) Run(name string, tests []*TestCase) *TestSuiteResult {
	result := &TestSuiteResult{
		Name:      name,
		Tests:     make([]*TestResult, 0, len(tests)),
		StartTime: time.Now(),
	}

	// Notify reporters
	for _, r := range f.reporters {
		r.OnSuiteStart(name)
	}

	// Filter tests
	filtered := f.filterTests(tests)

	// Run tests
	if f.Config.Parallel && len(filtered) > 1 {
		f.runParallel(filtered, result)
	} else {
		f.runSequential(filtered, result)
	}

	// Calculate totals
	for _, tr := range result.Tests {
		switch tr.Status {
		case TestStatusPassed:
			result.Passed++
		case TestStatusFailed:
			result.Failed++
		case TestStatusSkipped:
			result.Skipped++
		case TestStatusError:
			result.Errors++
		}
		result.Total++
	}

	result.EndTime = time.Now()
	result.Duration = result.EndTime.Sub(result.StartTime)

	// Notify reporters
	for _, r := range f.reporters {
		r.OnSuiteComplete(result)
	}

	return result
}

// filterTests filters tests based on config
func (f *Framework) filterTests(tests []*TestCase) []*TestCase {
	var filtered []*TestCase

	var pattern *regexp.Regexp
	if f.Config.Pattern != "" {
		pattern, _ = regexp.Compile(f.Config.Pattern)
	}

	for _, tc := range tests {
		// Filter by pattern
		if pattern != nil && !pattern.MatchString(tc.Name) {
			continue
		}

		// Filter by tags
		if len(f.Config.Tags) > 0 && !hasAnyTag(tc.Tags, f.Config.Tags) {
			continue
		}

		// Filter by skip tags
		if len(f.Config.SkipTags) > 0 && hasAnyTag(tc.Tags, f.Config.SkipTags) {
			continue
		}

		filtered = append(filtered, tc)
	}

	return filtered
}

func hasAnyTag(testTags, filterTags []string) bool {
	tagSet := make(map[string]bool)
	for _, t := range testTags {
		tagSet[t] = true
	}
	for _, t := range filterTags {
		if tagSet[t] {
			return true
		}
	}
	return false
}

func (f *Framework) runSequential(tests []*TestCase, result *TestSuiteResult) {
	failedCount := 0
	for _, tc := range tests {
		if f.Config.FailFast && failedCount > 0 {
			// Skip remaining tests
			result.Tests = append(result.Tests, &TestResult{
				Name:   tc.Name,
				Status: TestStatusSkipped,
			})
			continue
		}

		tr := f.runTest(tc)
		result.Tests = append(result.Tests, tr)

		if tr.Status == TestStatusFailed || tr.Status == TestStatusError {
			failedCount++
		}
	}
}

func (f *Framework) runParallel(tests []*TestCase, result *TestSuiteResult) {
	// Separate parallel and sequential tests
	var parallel, sequential []*TestCase
	for _, tc := range tests {
		if tc.Parallel {
			parallel = append(parallel, tc)
		} else {
			sequential = append(sequential, tc)
		}
	}

	// Run sequential tests first
	f.runSequential(sequential, result)

	if f.Config.FailFast && result.Failed > 0 {
		return
	}

	// Run parallel tests
	var wg sync.WaitGroup
	results := make(chan *TestResult, len(parallel))
	sem := make(chan struct{}, f.Config.MaxParallel)

	for _, tc := range parallel {
		wg.Add(1)
		go func(tc *TestCase) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			results <- f.runTest(tc)
		}(tc)
	}

	go func() {
		wg.Wait()
		close(results)
	}()

	for tr := range results {
		result.Tests = append(result.Tests, tr)
	}
}

func (f *Framework) runTest(tc *TestCase) *TestResult {
	t := &T{
		Name:      tc.Name,
		framework: f,
		startTime: time.Now(),
		context:   make(map[string]interface{}),
	}

	tr := &TestResult{
		Name: tc.Name,
	}

	// Notify reporters
	for _, r := range f.reporters {
		r.OnTestStart(tc.Name)
	}

	// Check if skipped
	if tc.Skip {
		tr.Status = TestStatusSkipped
		tr.Duration = time.Since(t.startTime)
		for _, r := range f.reporters {
			r.OnTestComplete(tr)
		}
		return tr
	}

	// Set timeout
	timeout := tc.Timeout
	if timeout == 0 {
		timeout = f.Config.Timeout
	}

	// Run with timeout
	done := make(chan struct{})
	errCh := make(chan error, 1)
	go func() {
		defer func() {
			if r := recover(); r != nil {
				errCh <- fmt.Errorf("panic: %v", r)
				close(done)
			}
		}()

		// Run setup
		if tc.Setup != nil {
			if err := tc.Setup(t); err != nil {
				errCh <- fmt.Errorf("setup failed: %w", err)
				close(done)
				return
			}
		}

		// Run test
		err := tc.Run(t)

		// Run teardown (even if test failed)
		if tc.Teardown != nil {
			if teardownErr := tc.Teardown(t); teardownErr != nil && err == nil {
				err = fmt.Errorf("teardown failed: %w", teardownErr)
			}
		}

		errCh <- err
		close(done)
	}()

	if wait.ForSignal(done, timeout) {
		tr.Status = TestStatusError
		tr.Error = fmt.Sprintf("test timed out after %v", timeout)
	} else {
		err := <-errCh
		if t.skipped {
			tr.Status = TestStatusSkipped
		} else if t.failed || err != nil {
			tr.Status = TestStatusFailed
			if err != nil {
				tr.Error = err.Error()
			} else if len(t.errors) > 0 {
				tr.Error = strings.Join(t.errors, "; ")
			}
		} else {
			tr.Status = TestStatusPassed
		}
	}

	tr.Duration = time.Since(t.startTime)
	tr.Logs = t.logs

	// Notify reporters
	for _, r := range f.reporters {
		r.OnTestComplete(tr)
	}

	return tr
}

// T methods

// Log logs a message
func (t *T) Log(args ...interface{}) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.logs = append(t.logs, fmt.Sprint(args...))
}

// Logf logs a formatted message
func (t *T) Logf(format string, args ...interface{}) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.logs = append(t.logs, fmt.Sprintf(format, args...))
}

// Error marks the test as failed and logs an error
func (t *T) Error(args ...interface{}) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.failed = true
	t.errors = append(t.errors, fmt.Sprint(args...))
}

// Errorf marks the test as failed and logs a formatted error
func (t *T) Errorf(format string, args ...interface{}) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.failed = true
	t.errors = append(t.errors, fmt.Sprintf(format, args...))
}

// Fail marks the test as failed
func (t *T) Fail() {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.failed = true
}

// FailNow marks the test as failed and stops execution
func (t *T) FailNow() {
	t.Fail()
	panic("test failed")
}

// Fatal is equivalent to Log followed by FailNow
func (t *T) Fatal(args ...interface{}) {
	t.Log(args...)
	t.FailNow()
}

// Fatalf is equivalent to Logf followed by FailNow
func (t *T) Fatalf(format string, args ...interface{}) {
	t.Logf(format, args...)
	t.FailNow()
}

// Skip marks the test as skipped
func (t *T) Skip(args ...interface{}) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.skipped = true
	t.skipReason = fmt.Sprint(args...)
}

// Skipf marks the test as skipped with a formatted reason
func (t *T) Skipf(format string, args ...interface{}) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.skipped = true
	t.skipReason = fmt.Sprintf(format, args...)
}

// Failed returns whether the test has failed
func (t *T) Failed() bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.failed
}

// Skipped returns whether the test was skipped
func (t *T) Skipped() bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.skipped
}

// Context returns the test context for storing data
func (t *T) Context() map[string]interface{} {
	return t.context
}

// Mock returns a registered mock
func (t *T) Mock(name string) (MockFunc, bool) {
	return t.framework.GetMock(name)
}

// Fixture returns a loaded fixture
func (t *T) Fixture(name string) (interface{}, bool) {
	return t.framework.GetFixture(name)
}

// DiscoverTests discovers test files in a directory
func DiscoverTests(dir string, pattern string) ([]string, error) {
	if pattern == "" {
		pattern = "*_test.go"
	}

	var files []string
	err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		matched, _ := filepath.Match(pattern, filepath.Base(path))
		if matched {
			files = append(files, path)
		}
		return nil
	})

	sort.Strings(files)
	return files, err
}

// AssertEqual asserts two values are equal
func AssertEqual(t *T, got, want interface{}, msgAndArgs ...interface{}) {
	if got != want {
		msg := formatMessage("values not equal", msgAndArgs...)
		t.Errorf("%s: got %v, want %v", msg, got, want)
	}
}

// AssertNotEqual asserts two values are not equal
func AssertNotEqual(t *T, got, want interface{}, msgAndArgs ...interface{}) {
	if got == want {
		msg := formatMessage("values should not be equal", msgAndArgs...)
		t.Errorf("%s: both are %v", msg, got)
	}
}

// AssertNil asserts a value is nil
func AssertNil(t *T, got interface{}, msgAndArgs ...interface{}) {
	if got != nil {
		msg := formatMessage("expected nil", msgAndArgs...)
		t.Errorf("%s: got %v", msg, got)
	}
}

// AssertNotNil asserts a value is not nil
func AssertNotNil(t *T, got interface{}, msgAndArgs ...interface{}) {
	if got == nil {
		msg := formatMessage("expected non-nil", msgAndArgs...)
		t.Error(msg)
	}
}

// AssertTrue asserts a value is true
func AssertTrue(t *T, val bool, msgAndArgs ...interface{}) {
	if !val {
		msg := formatMessage("expected true", msgAndArgs...)
		t.Error(msg)
	}
}

// AssertFalse asserts a value is false
func AssertFalse(t *T, val bool, msgAndArgs ...interface{}) {
	if val {
		msg := formatMessage("expected false", msgAndArgs...)
		t.Error(msg)
	}
}

// AssertContains asserts a string contains a substring
func AssertContains(t *T, s, substr string, msgAndArgs ...interface{}) {
	if !strings.Contains(s, substr) {
		msg := formatMessage("string does not contain substring", msgAndArgs...)
		t.Errorf("%s: %q does not contain %q", msg, s, substr)
	}
}

// AssertNoError asserts there is no error
func AssertNoError(t *T, err error, msgAndArgs ...interface{}) {
	if err != nil {
		msg := formatMessage("unexpected error", msgAndArgs...)
		t.Errorf("%s: %v", msg, err)
	}
}

// AssertError asserts there is an error
func AssertError(t *T, err error, msgAndArgs ...interface{}) {
	if err == nil {
		msg := formatMessage("expected error", msgAndArgs...)
		t.Error(msg)
	}
}

func formatMessage(defaultMsg string, msgAndArgs ...interface{}) string {
	if len(msgAndArgs) == 0 {
		return defaultMsg
	}
	if format, ok := msgAndArgs[0].(string); ok {
		return fmt.Sprintf(format, msgAndArgs[1:]...)
	}
	return fmt.Sprint(msgAndArgs...)
}
