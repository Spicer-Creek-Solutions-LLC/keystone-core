// Package testing provides a testing framework for Keystone Core blueprints.
package testing

import (
	"time"
)

// TestSuite represents a collection of tests for a blueprint.
type TestSuite struct {
	// Name is the test suite name.
	Name string `yaml:"name" json:"name"`

	// Description describes what the test suite validates.
	Description string `yaml:"description,omitempty" json:"description,omitempty"`

	// Blueprint is the blueprint being tested (optional, defaults to parent directory).
	Blueprint string `yaml:"blueprint,omitempty" json:"blueprint,omitempty"`

	// Version is the blueprint version to test (optional, defaults to current).
	Version string `yaml:"version,omitempty" json:"version,omitempty"`

	// Setup contains steps to run before all tests.
	Setup *TestSetup `yaml:"setup,omitempty" json:"setup,omitempty"`

	// Teardown contains steps to run after all tests.
	Teardown *TestTeardown `yaml:"teardown,omitempty" json:"teardown,omitempty"`

	// Defaults provides default values for all tests.
	Defaults *TestDefaults `yaml:"defaults,omitempty" json:"defaults,omitempty"`

	// Tests is the list of test cases.
	Tests []TestCase `yaml:"tests" json:"tests"`

	// Tags for filtering tests.
	Tags []string `yaml:"tags,omitempty" json:"tags,omitempty"`
}

// TestSetup defines setup steps before tests run.
type TestSetup struct {
	// States to apply before testing.
	States []string `yaml:"states,omitempty" json:"states,omitempty"`

	// Commands to execute before testing.
	Commands []TestCommand `yaml:"commands,omitempty" json:"commands,omitempty"`

	// Files to create before testing.
	Files []TestFile `yaml:"files,omitempty" json:"files,omitempty"`
}

// TestTeardown defines cleanup steps after tests complete.
type TestTeardown struct {
	// Always run teardown even on test failure.
	Always bool `yaml:"always,omitempty" json:"always,omitempty"`

	// States to apply for cleanup.
	States []string `yaml:"states,omitempty" json:"states,omitempty"`

	// Commands to execute for cleanup.
	Commands []TestCommand `yaml:"commands,omitempty" json:"commands,omitempty"`

	// Files to remove after testing.
	Files []string `yaml:"files,omitempty" json:"files,omitempty"`
}

// TestDefaults provides default values for test cases.
type TestDefaults struct {
	// Timeout for each test.
	Timeout Duration `yaml:"timeout,omitempty" json:"timeout,omitempty"`

	// DryRun executes in dry-run mode.
	DryRun bool `yaml:"dry_run,omitempty" json:"dry_run,omitempty"`

	// Parameters are default parameter values.
	Parameters map[string]interface{} `yaml:"parameters,omitempty" json:"parameters,omitempty"`

	// Mocks are default mock configurations.
	Mocks []MockConfig `yaml:"mocks,omitempty" json:"mocks,omitempty"`
}

// TestCase represents a single test case.
type TestCase struct {
	// Name is the test case name (required).
	Name string `yaml:"name" json:"name"`

	// Description describes what this test validates.
	Description string `yaml:"description,omitempty" json:"description,omitempty"`

	// Skip marks the test as skipped with a reason.
	Skip string `yaml:"skip,omitempty" json:"skip,omitempty"`

	// Tags for filtering this specific test.
	Tags []string `yaml:"tags,omitempty" json:"tags,omitempty"`

	// Parameters to use for this test.
	Parameters map[string]interface{} `yaml:"parameters,omitempty" json:"parameters,omitempty"`

	// Secrets to use for this test (mock values).
	Secrets map[string]string `yaml:"secrets,omitempty" json:"secrets,omitempty"`

	// Features to enable for this test.
	Features []string `yaml:"features,omitempty" json:"features,omitempty"`

	// Entrypoint to use (defaults to "default").
	Entrypoint string `yaml:"entrypoint,omitempty" json:"entrypoint,omitempty"`

	// Setup for this specific test.
	Setup *TestSetup `yaml:"setup,omitempty" json:"setup,omitempty"`

	// Teardown for this specific test.
	Teardown *TestTeardown `yaml:"teardown,omitempty" json:"teardown,omitempty"`

	// Mocks for external dependencies.
	Mocks []MockConfig `yaml:"mocks,omitempty" json:"mocks,omitempty"`

	// Timeout for this test.
	Timeout Duration `yaml:"timeout,omitempty" json:"timeout,omitempty"`

	// DryRun executes in dry-run mode.
	DryRun bool `yaml:"dry_run,omitempty" json:"dry_run,omitempty"`

	// Assertions to validate after execution.
	Assertions []Assertion `yaml:"assertions" json:"assertions"`

	// ExpectFailure expects the blueprint to fail.
	ExpectFailure bool `yaml:"expect_failure,omitempty" json:"expect_failure,omitempty"`

	// ExpectError expects a specific error pattern.
	ExpectError string `yaml:"expect_error,omitempty" json:"expect_error,omitempty"`
}

// Assertion defines a validation to perform after test execution.
type Assertion struct {
	// Type is the assertion type.
	Type AssertionType `yaml:"type" json:"type"`

	// Description of what this assertion validates.
	Description string `yaml:"description,omitempty" json:"description,omitempty"`

	// Target is the subject of the assertion (path, state ID, etc.).
	Target string `yaml:"target,omitempty" json:"target,omitempty"`

	// Operator for comparison assertions.
	Operator ComparisonOperator `yaml:"operator,omitempty" json:"operator,omitempty"`

	// Expected value for comparison.
	Expected interface{} `yaml:"expected,omitempty" json:"expected,omitempty"`

	// Pattern for regex matching.
	Pattern string `yaml:"pattern,omitempty" json:"pattern,omitempty"`

	// Negate inverts the assertion result.
	Negate bool `yaml:"negate,omitempty" json:"negate,omitempty"`

	// State-specific assertions.
	State *StateAssertion `yaml:"state,omitempty" json:"state,omitempty"`

	// File-specific assertions.
	File *FileAssertion `yaml:"file,omitempty" json:"file,omitempty"`

	// Command-specific assertions.
	Command *CommandAssertion `yaml:"command,omitempty" json:"command,omitempty"`

	// Output-specific assertions.
	Output *OutputAssertion `yaml:"output,omitempty" json:"output,omitempty"`
}

// AssertionType defines the type of assertion.
type AssertionType string

const (
	// AssertStateApplied checks that a state was applied.
	AssertStateApplied AssertionType = "state_applied"

	// AssertStateChanged checks that a state made changes.
	AssertStateChanged AssertionType = "state_changed"

	// AssertStateUnchanged checks that a state made no changes.
	AssertStateUnchanged AssertionType = "state_unchanged"

	// AssertStateFailed checks that a state failed.
	AssertStateFailed AssertionType = "state_failed"

	// AssertFileExists checks that a file exists.
	AssertFileExists AssertionType = "file_exists"

	// AssertFileNotExists checks that a file does not exist.
	AssertFileNotExists AssertionType = "file_not_exists"

	// AssertFileContains checks file content.
	AssertFileContains AssertionType = "file_contains"

	// AssertFileMode checks file permissions.
	AssertFileMode AssertionType = "file_mode"

	// AssertFileOwner checks file ownership.
	AssertFileOwner AssertionType = "file_owner"

	// AssertDirectoryExists checks that a directory exists.
	AssertDirectoryExists AssertionType = "directory_exists"

	// AssertCommandSuccess runs a command expecting success.
	AssertCommandSuccess AssertionType = "command_success"

	// AssertCommandFailure runs a command expecting failure.
	AssertCommandFailure AssertionType = "command_failure"

	// AssertCommandOutput checks command output.
	AssertCommandOutput AssertionType = "command_output"

	// AssertOutputContains checks blueprint output contains value.
	AssertOutputContains AssertionType = "output_contains"

	// AssertOutputEquals checks blueprint output equals value.
	AssertOutputEquals AssertionType = "output_equals"

	// AssertOutputMatches checks blueprint output matches pattern.
	AssertOutputMatches AssertionType = "output_matches"

	// AssertExpression evaluates a CEL expression.
	AssertExpression AssertionType = "expression"

	// AssertStatesApplied checks total states applied count.
	AssertStatesApplied AssertionType = "states_applied"

	// AssertStatesChanged checks total states changed count.
	AssertStatesChanged AssertionType = "states_changed"

	// AssertStatesFailed checks total states failed count.
	AssertStatesFailed AssertionType = "states_failed"

	// AssertNoFailures checks that no states failed.
	AssertNoFailures AssertionType = "no_failures"

	// AssertIdempotent checks that running again makes no changes.
	AssertIdempotent AssertionType = "idempotent"
)

// ComparisonOperator defines comparison operations.
type ComparisonOperator string

const (
	OpEquals      ComparisonOperator = "equals"
	OpNotEquals   ComparisonOperator = "not_equals"
	OpContains    ComparisonOperator = "contains"
	OpNotContains ComparisonOperator = "not_contains"
	OpMatches     ComparisonOperator = "matches"
	OpGreaterThan ComparisonOperator = "greater_than"
	OpLessThan    ComparisonOperator = "less_than"
	OpGreaterOrEq ComparisonOperator = "greater_or_equal"
	OpLessOrEq    ComparisonOperator = "less_or_equal"
)

// StateAssertion contains state-specific assertion details.
type StateAssertion struct {
	// ID of the state to check.
	ID string `yaml:"id" json:"id"`

	// Module type to check.
	Module string `yaml:"module,omitempty" json:"module,omitempty"`

	// Changed expects the state to have made changes.
	Changed *bool `yaml:"changed,omitempty" json:"changed,omitempty"`

	// Result expected (e.g., "success", "failure").
	Result string `yaml:"result,omitempty" json:"result,omitempty"`

	// Comment expected in the result.
	Comment string `yaml:"comment,omitempty" json:"comment,omitempty"`
}

// FileAssertion contains file-specific assertion details.
type FileAssertion struct {
	// Path to the file.
	Path string `yaml:"path" json:"path"`

	// Content expected (exact match).
	Content string `yaml:"content,omitempty" json:"content,omitempty"`

	// Contains checks for substring.
	Contains string `yaml:"contains,omitempty" json:"contains,omitempty"`

	// NotContains checks absence of substring.
	NotContains string `yaml:"not_contains,omitempty" json:"not_contains,omitempty"`

	// Matches checks regex pattern.
	Matches string `yaml:"matches,omitempty" json:"matches,omitempty"`

	// Mode expected (e.g., "0644").
	Mode string `yaml:"mode,omitempty" json:"mode,omitempty"`

	// Owner expected.
	Owner string `yaml:"owner,omitempty" json:"owner,omitempty"`

	// Group expected.
	Group string `yaml:"group,omitempty" json:"group,omitempty"`

	// Size expected.
	Size *int64 `yaml:"size,omitempty" json:"size,omitempty"`

	// IsSymlink checks if file is a symlink.
	IsSymlink *bool `yaml:"is_symlink,omitempty" json:"is_symlink,omitempty"`

	// LinkTarget expected for symlinks.
	LinkTarget string `yaml:"link_target,omitempty" json:"link_target,omitempty"`
}

// CommandAssertion contains command-specific assertion details.
type CommandAssertion struct {
	// Command to execute.
	Command string `yaml:"command" json:"command"`

	// Args for the command.
	Args []string `yaml:"args,omitempty" json:"args,omitempty"`

	// Shell to use (defaults to /bin/sh).
	Shell string `yaml:"shell,omitempty" json:"shell,omitempty"`

	// ExitCode expected.
	ExitCode *int `yaml:"exit_code,omitempty" json:"exit_code,omitempty"`

	// Stdout expected (exact match).
	Stdout string `yaml:"stdout,omitempty" json:"stdout,omitempty"`

	// StdoutContains checks stdout substring.
	StdoutContains string `yaml:"stdout_contains,omitempty" json:"stdout_contains,omitempty"`

	// StdoutMatches checks stdout regex.
	StdoutMatches string `yaml:"stdout_matches,omitempty" json:"stdout_matches,omitempty"`

	// Stderr expected.
	Stderr string `yaml:"stderr,omitempty" json:"stderr,omitempty"`

	// StderrContains checks stderr substring.
	StderrContains string `yaml:"stderr_contains,omitempty" json:"stderr_contains,omitempty"`

	// Timeout for command execution.
	Timeout Duration `yaml:"timeout,omitempty" json:"timeout,omitempty"`
}

// OutputAssertion contains output-specific assertion details.
type OutputAssertion struct {
	// Name of the output to check.
	Name string `yaml:"name" json:"name"`

	// Value expected (exact match).
	Value interface{} `yaml:"value,omitempty" json:"value,omitempty"`

	// Contains checks for substring (if string).
	Contains string `yaml:"contains,omitempty" json:"contains,omitempty"`

	// Matches checks regex pattern (if string).
	Matches string `yaml:"matches,omitempty" json:"matches,omitempty"`
}

// MockConfig defines a mock for external dependencies.
type MockConfig struct {
	// Type of mock (command, file, http, etc.).
	Type MockType `yaml:"type" json:"type"`

	// Name identifies this mock.
	Name string `yaml:"name,omitempty" json:"name,omitempty"`

	// Command mock configuration.
	Command *CommandMock `yaml:"command,omitempty" json:"command,omitempty"`

	// File mock configuration.
	File *FileMock `yaml:"file,omitempty" json:"file,omitempty"`

	// HTTP mock configuration.
	HTTP *HTTPMock `yaml:"http,omitempty" json:"http,omitempty"`

	// Package mock configuration.
	Package *PackageMock `yaml:"package,omitempty" json:"package,omitempty"`

	// Service mock configuration.
	Service *ServiceMock `yaml:"service,omitempty" json:"service,omitempty"`
}

// MockType defines the type of mock.
type MockType string

const (
	MockTypeCommand MockType = "command"
	MockTypeFile    MockType = "file"
	MockTypeHTTP    MockType = "http"
	MockTypePackage MockType = "package"
	MockTypeService MockType = "service"
)

// CommandMock mocks command execution.
type CommandMock struct {
	// Pattern to match command (glob or regex).
	Pattern string `yaml:"pattern" json:"pattern"`

	// IsRegex indicates pattern is a regex.
	IsRegex bool `yaml:"is_regex,omitempty" json:"is_regex,omitempty"`

	// Stdout to return.
	Stdout string `yaml:"stdout,omitempty" json:"stdout,omitempty"`

	// Stderr to return.
	Stderr string `yaml:"stderr,omitempty" json:"stderr,omitempty"`

	// ExitCode to return.
	ExitCode int `yaml:"exit_code,omitempty" json:"exit_code,omitempty"`

	// Delay before returning.
	Delay Duration `yaml:"delay,omitempty" json:"delay,omitempty"`

	// Times specifies how many times this mock should match.
	Times int `yaml:"times,omitempty" json:"times,omitempty"`
}

// FileMock mocks file system operations.
type FileMock struct {
	// Path to mock.
	Path string `yaml:"path" json:"path"`

	// Exists indicates if file exists.
	Exists bool `yaml:"exists,omitempty" json:"exists,omitempty"`

	// Content to return on read.
	Content string `yaml:"content,omitempty" json:"content,omitempty"`

	// Mode to report.
	Mode string `yaml:"mode,omitempty" json:"mode,omitempty"`

	// Owner to report.
	Owner string `yaml:"owner,omitempty" json:"owner,omitempty"`

	// Group to report.
	Group string `yaml:"group,omitempty" json:"group,omitempty"`

	// IsDir indicates if path is a directory.
	IsDir bool `yaml:"is_dir,omitempty" json:"is_dir,omitempty"`
}

// HTTPMock mocks HTTP requests.
type HTTPMock struct {
	// URL pattern to match.
	URL string `yaml:"url" json:"url"`

	// Method to match (GET, POST, etc.).
	Method string `yaml:"method,omitempty" json:"method,omitempty"`

	// StatusCode to return.
	StatusCode int `yaml:"status_code,omitempty" json:"status_code,omitempty"`

	// Body to return.
	Body string `yaml:"body,omitempty" json:"body,omitempty"`

	// Headers to return.
	Headers map[string]string `yaml:"headers,omitempty" json:"headers,omitempty"`

	// Delay before responding.
	Delay Duration `yaml:"delay,omitempty" json:"delay,omitempty"`
}

// PackageMock mocks package manager operations.
type PackageMock struct {
	// Name of the package.
	Name string `yaml:"name" json:"name"`

	// Installed indicates if package is installed.
	Installed bool `yaml:"installed,omitempty" json:"installed,omitempty"`

	// Version installed.
	Version string `yaml:"version,omitempty" json:"version,omitempty"`

	// AvailableVersions from repository.
	AvailableVersions []string `yaml:"available_versions,omitempty" json:"available_versions,omitempty"`
}

// ServiceMock mocks service operations.
type ServiceMock struct {
	// Name of the service.
	Name string `yaml:"name" json:"name"`

	// Running indicates if service is running.
	Running bool `yaml:"running,omitempty" json:"running,omitempty"`

	// Enabled indicates if service is enabled.
	Enabled bool `yaml:"enabled,omitempty" json:"enabled,omitempty"`
}

// TestCommand defines a command to run.
type TestCommand struct {
	// Command to execute.
	Command string `yaml:"command" json:"command"`

	// Args for the command.
	Args []string `yaml:"args,omitempty" json:"args,omitempty"`

	// Shell to use.
	Shell string `yaml:"shell,omitempty" json:"shell,omitempty"`

	// IgnoreErrors continues on failure.
	IgnoreErrors bool `yaml:"ignore_errors,omitempty" json:"ignore_errors,omitempty"`
}

// TestFile defines a file to create.
type TestFile struct {
	// Path to the file.
	Path string `yaml:"path" json:"path"`

	// Content of the file.
	Content string `yaml:"content,omitempty" json:"content,omitempty"`

	// Source file to copy from.
	Source string `yaml:"source,omitempty" json:"source,omitempty"`

	// Mode for the file.
	Mode string `yaml:"mode,omitempty" json:"mode,omitempty"`

	// Owner for the file.
	Owner string `yaml:"owner,omitempty" json:"owner,omitempty"`

	// Group for the file.
	Group string `yaml:"group,omitempty" json:"group,omitempty"`

	// IsDir creates a directory instead.
	IsDir bool `yaml:"is_dir,omitempty" json:"is_dir,omitempty"`
}

// Duration wraps time.Duration for YAML marshaling.
type Duration time.Duration

// UnmarshalYAML implements yaml.Unmarshaler.
func (d *Duration) UnmarshalYAML(unmarshal func(interface{}) error) error {
	var s string
	if err := unmarshal(&s); err != nil {
		return err
	}
	if s == "" {
		*d = 0
		return nil
	}
	dur, err := time.ParseDuration(s)
	if err != nil {
		return err
	}
	*d = Duration(dur)
	return nil
}

// MarshalYAML implements yaml.Marshaler.
func (d Duration) MarshalYAML() (interface{}, error) {
	if d == 0 {
		return "", nil
	}
	return time.Duration(d).String(), nil
}

// Duration returns the time.Duration value.
func (d Duration) Duration() time.Duration {
	return time.Duration(d)
}
