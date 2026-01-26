package testing

import (
	"time"
)

// TestSuiteResult contains the results of running a test suite.
type TestSuiteResult struct {
	// Name of the test suite.
	Name string `json:"name"`

	// Blueprint tested.
	Blueprint string `json:"blueprint"`

	// Version tested.
	Version string `json:"version,omitempty"`

	// StartTime when the suite started.
	StartTime time.Time `json:"start_time"`

	// EndTime when the suite completed.
	EndTime time.Time `json:"end_time"`

	// Duration of the entire suite.
	Duration time.Duration `json:"duration"`

	// Tests is the list of test results.
	Tests []TestResult `json:"tests"`

	// SetupResult from suite setup.
	SetupResult *SetupTeardownResult `json:"setup_result,omitempty"`

	// TeardownResult from suite teardown.
	TeardownResult *SetupTeardownResult `json:"teardown_result,omitempty"`

	// Summary statistics.
	Summary TestSummary `json:"summary"`

	// Error if suite failed to run.
	Error string `json:"error,omitempty"`
}

// TestResult contains the result of a single test case.
type TestResult struct {
	// Name of the test.
	Name string `json:"name"`

	// Description of the test.
	Description string `json:"description,omitempty"`

	// Status of the test.
	Status TestStatus `json:"status"`

	// StartTime when the test started.
	StartTime time.Time `json:"start_time"`

	// EndTime when the test completed.
	EndTime time.Time `json:"end_time"`

	// Duration of the test.
	Duration time.Duration `json:"duration"`

	// SetupResult from test setup.
	SetupResult *SetupTeardownResult `json:"setup_result,omitempty"`

	// TeardownResult from test teardown.
	TeardownResult *SetupTeardownResult `json:"teardown_result,omitempty"`

	// ExecutionResult from blueprint execution.
	ExecutionResult *ExecutionResult `json:"execution_result,omitempty"`

	// AssertionResults from all assertions.
	AssertionResults []AssertionResult `json:"assertion_results,omitempty"`

	// Error if test failed.
	Error string `json:"error,omitempty"`

	// SkipReason if test was skipped.
	SkipReason string `json:"skip_reason,omitempty"`

	// Output captured during test.
	Output string `json:"output,omitempty"`
}

// TestStatus represents the status of a test.
type TestStatus string

const (
	// StatusPassed indicates the test passed.
	StatusPassed TestStatus = "passed"

	// StatusFailed indicates the test failed.
	StatusFailed TestStatus = "failed"

	// StatusSkipped indicates the test was skipped.
	StatusSkipped TestStatus = "skipped"

	// StatusError indicates an error occurred (not a test failure).
	StatusError TestStatus = "error"

	// StatusRunning indicates the test is currently running.
	StatusRunning TestStatus = "running"

	// StatusPending indicates the test has not yet run.
	StatusPending TestStatus = "pending"
)

// SetupTeardownResult contains setup or teardown results.
type SetupTeardownResult struct {
	// Success indicates if setup/teardown succeeded.
	Success bool `json:"success"`

	// Duration of the setup/teardown.
	Duration time.Duration `json:"duration"`

	// Commands executed.
	CommandResults []CommandResult `json:"command_results,omitempty"`

	// States applied.
	StateResults []StateResult `json:"state_results,omitempty"`

	// Files created.
	FilesCreated []string `json:"files_created,omitempty"`

	// Error if setup/teardown failed.
	Error string `json:"error,omitempty"`
}

// ExecutionResult contains the result of blueprint execution.
type ExecutionResult struct {
	// Success indicates if execution succeeded.
	Success bool `json:"success"`

	// DryRun indicates if this was a dry-run.
	DryRun bool `json:"dry_run"`

	// Duration of execution.
	Duration time.Duration `json:"duration"`

	// StatesApplied count.
	StatesApplied int `json:"states_applied"`

	// StatesChanged count.
	StatesChanged int `json:"states_changed"`

	// StatesFailed count.
	StatesFailed int `json:"states_failed"`

	// StatesUnchanged count.
	StatesUnchanged int `json:"states_unchanged"`

	// StateResults for individual states.
	StateResults []StateResult `json:"state_results,omitempty"`

	// Outputs from the blueprint.
	Outputs map[string]interface{} `json:"outputs,omitempty"`

	// Error if execution failed.
	Error string `json:"error,omitempty"`
}

// StateResult contains the result of a single state.
type StateResult struct {
	// ID of the state.
	ID string `json:"id"`

	// Module type.
	Module string `json:"module"`

	// Success indicates if the state succeeded.
	Success bool `json:"success"`

	// Changed indicates if the state made changes.
	Changed bool `json:"changed"`

	// Duration of the state.
	Duration time.Duration `json:"duration"`

	// Comment from the state.
	Comment string `json:"comment,omitempty"`

	// Error if the state failed.
	Error string `json:"error,omitempty"`
}

// CommandResult contains the result of a command execution.
type CommandResult struct {
	// Command executed.
	Command string `json:"command"`

	// ExitCode returned.
	ExitCode int `json:"exit_code"`

	// Stdout output.
	Stdout string `json:"stdout,omitempty"`

	// Stderr output.
	Stderr string `json:"stderr,omitempty"`

	// Duration of execution.
	Duration time.Duration `json:"duration"`

	// Error if command failed.
	Error string `json:"error,omitempty"`
}

// AssertionResult contains the result of a single assertion.
type AssertionResult struct {
	// Type of assertion.
	Type AssertionType `json:"type"`

	// Description of the assertion.
	Description string `json:"description,omitempty"`

	// Passed indicates if the assertion passed.
	Passed bool `json:"passed"`

	// Target of the assertion.
	Target string `json:"target,omitempty"`

	// Expected value.
	Expected interface{} `json:"expected,omitempty"`

	// Actual value.
	Actual interface{} `json:"actual,omitempty"`

	// Message explaining the result.
	Message string `json:"message,omitempty"`

	// Duration of the assertion check.
	Duration time.Duration `json:"duration,omitempty"`
}

// TestSummary contains summary statistics for a test suite.
type TestSummary struct {
	// Total tests.
	Total int `json:"total"`

	// Passed tests.
	Passed int `json:"passed"`

	// Failed tests.
	Failed int `json:"failed"`

	// Skipped tests.
	Skipped int `json:"skipped"`

	// Errors (not failures).
	Errors int `json:"errors"`

	// PassRate as percentage.
	PassRate float64 `json:"pass_rate"`

	// TotalDuration of all tests.
	TotalDuration time.Duration `json:"total_duration"`

	// FailedTests names.
	FailedTests []string `json:"failed_tests,omitempty"`

	// SkippedTests names.
	SkippedTests []string `json:"skipped_tests,omitempty"`

	// ErrorTests names.
	ErrorTests []string `json:"error_tests,omitempty"`
}

// IsPassing returns true if the test suite passed.
func (r *TestSuiteResult) IsPassing() bool {
	return r.Summary.Failed == 0 && r.Summary.Errors == 0 && r.Error == ""
}

// CalculateSummary calculates summary statistics from test results.
func (r *TestSuiteResult) CalculateSummary() {
	r.Summary = TestSummary{
		Total: len(r.Tests),
	}

	for _, test := range r.Tests {
		switch test.Status {
		case StatusPassed:
			r.Summary.Passed++
		case StatusFailed:
			r.Summary.Failed++
			r.Summary.FailedTests = append(r.Summary.FailedTests, test.Name)
		case StatusSkipped:
			r.Summary.Skipped++
			r.Summary.SkippedTests = append(r.Summary.SkippedTests, test.Name)
		case StatusError:
			r.Summary.Errors++
			r.Summary.ErrorTests = append(r.Summary.ErrorTests, test.Name)
		}
		r.Summary.TotalDuration += test.Duration
	}

	// Calculate pass rate (excluding skipped)
	runTests := r.Summary.Total - r.Summary.Skipped
	if runTests > 0 {
		r.Summary.PassRate = float64(r.Summary.Passed) / float64(runTests) * 100
	}
}

// NewTestResult creates a new test result for a test case.
func NewTestResult(test *TestCase) *TestResult {
	return &TestResult{
		Name:        test.Name,
		Description: test.Description,
		Status:      StatusPending,
	}
}

// Pass marks the test as passed.
func (r *TestResult) Pass() {
	r.Status = StatusPassed
}

// Fail marks the test as failed with an error message.
func (r *TestResult) Fail(err string) {
	r.Status = StatusFailed
	r.Error = err
}

// Skip marks the test as skipped with a reason.
func (r *TestResult) Skip(reason string) {
	r.Status = StatusSkipped
	r.SkipReason = reason
}

// SetError marks the test as having an error.
func (r *TestResult) SetError(err string) {
	r.Status = StatusError
	r.Error = err
}

// AllAssertionsPassed returns true if all assertions passed.
func (r *TestResult) AllAssertionsPassed() bool {
	for _, ar := range r.AssertionResults {
		if !ar.Passed {
			return false
		}
	}
	return true
}

// FailedAssertions returns the list of failed assertions.
func (r *TestResult) FailedAssertions() []AssertionResult {
	var failed []AssertionResult
	for _, ar := range r.AssertionResults {
		if !ar.Passed {
			failed = append(failed, ar)
		}
	}
	return failed
}
