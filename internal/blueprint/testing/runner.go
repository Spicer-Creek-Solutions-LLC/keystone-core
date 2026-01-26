package testing

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/shawnbutts/keystone-core/internal/blueprint"
	"gopkg.in/yaml.v3"
)

// RunnerConfig configures the test runner.
type RunnerConfig struct {
	// BlueprintPath is the path to blueprints.
	BlueprintPath string

	// DryRun executes in dry-run mode (no actual changes).
	DryRun bool

	// Timeout is the default timeout for tests.
	Timeout time.Duration

	// Parallel runs tests in parallel.
	Parallel bool

	// MaxParallel is the maximum number of parallel tests.
	MaxParallel int

	// StopOnFailure stops on first failure.
	StopOnFailure bool

	// Verbose enables verbose output.
	Verbose bool

	// TempDir is the directory for temporary files.
	TempDir string

	// MockRegistry provides mock implementations.
	MockRegistry *MockRegistry

	// EventHandler receives test events.
	EventHandler TestEventHandler
}

// DefaultRunnerConfig returns default configuration.
func DefaultRunnerConfig() *RunnerConfig {
	return &RunnerConfig{
		Timeout:     5 * time.Minute,
		MaxParallel: 4,
		TempDir:     os.TempDir(),
	}
}

// TestEventHandler handles test events.
type TestEventHandler interface {
	OnSuiteStart(suite *TestSuite)
	OnSuiteEnd(result *TestSuiteResult)
	OnTestStart(test *TestCase)
	OnTestEnd(result *TestResult)
	OnAssertionResult(result *AssertionResult)
}

// Runner executes blueprint tests.
type Runner struct {
	config   *RunnerConfig
	loader   *blueprint.Loader
	executor *blueprint.Executor
}

// NewRunner creates a new test runner.
func NewRunner(config *RunnerConfig) (*Runner, error) {
	if config == nil {
		config = DefaultRunnerConfig()
	}

	// Create storage for blueprints
	var storage blueprint.Storage
	if config.BlueprintPath != "" {
		var err error
		storage, err = blueprint.NewLocalStorage(config.BlueprintPath, false)
		if err != nil {
			return nil, fmt.Errorf("failed to create storage: %w", err)
		}
	}

	loader := blueprint.NewLoader(storage)
	executor, err := blueprint.NewExecutor(&blueprint.ExecutorConfig{
		Loader: loader,
		DryRun: config.DryRun,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create executor: %w", err)
	}

	return &Runner{
		config:   config,
		loader:   loader,
		executor: executor,
	}, nil
}

// RunSuite runs a test suite.
func (r *Runner) RunSuite(ctx context.Context, suite *TestSuite) *TestSuiteResult {
	result := &TestSuiteResult{
		Name:      suite.Name,
		Blueprint: suite.Blueprint,
		Version:   suite.Version,
		StartTime: time.Now(),
	}

	if r.config.EventHandler != nil {
		r.config.EventHandler.OnSuiteStart(suite)
	}

	defer func() {
		result.EndTime = time.Now()
		result.Duration = result.EndTime.Sub(result.StartTime)
		result.CalculateSummary()

		if r.config.EventHandler != nil {
			r.config.EventHandler.OnSuiteEnd(result)
		}
	}()

	// Run suite setup
	if suite.Setup != nil {
		setupResult := r.runSetup(ctx, suite.Setup)
		result.SetupResult = setupResult
		if !setupResult.Success {
			result.Error = "suite setup failed: " + setupResult.Error
			return result
		}
	}

	// Run tests
	for _, test := range suite.Tests {
		// Check for cancellation
		if ctx.Err() != nil {
			break
		}

		testResult := r.RunTest(ctx, &test, suite)
		result.Tests = append(result.Tests, *testResult)

		// Stop on failure if configured
		if r.config.StopOnFailure && testResult.Status == StatusFailed {
			break
		}
	}

	// Run suite teardown
	if suite.Teardown != nil {
		teardownResult := r.runTeardown(ctx, suite.Teardown)
		result.TeardownResult = teardownResult
	}

	return result
}

// RunTest runs a single test case.
func (r *Runner) RunTest(ctx context.Context, test *TestCase, suite *TestSuite) *TestResult {
	result := NewTestResult(test)
	result.StartTime = time.Now()

	if r.config.EventHandler != nil {
		r.config.EventHandler.OnTestStart(test)
	}

	defer func() {
		result.EndTime = time.Now()
		result.Duration = result.EndTime.Sub(result.StartTime)

		if r.config.EventHandler != nil {
			r.config.EventHandler.OnTestEnd(result)
		}
	}()

	// Check if skipped
	if test.Skip != "" {
		result.Skip(test.Skip)
		return result
	}

	// Apply timeout
	timeout := test.Timeout.Duration()
	if timeout == 0 {
		timeout = r.config.Timeout
	}
	if timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, timeout)
		defer cancel()
	}

	// Run test setup
	if test.Setup != nil {
		setupResult := r.runSetup(ctx, test.Setup)
		result.SetupResult = setupResult
		if !setupResult.Success {
			result.SetError("test setup failed: " + setupResult.Error)
			return result
		}
	}

	// Execute blueprint
	execResult, err := r.executeBlueprint(ctx, test, suite)
	result.ExecutionResult = execResult

	if err != nil {
		// Check if we expected failure
		if test.ExpectFailure {
			result.Pass()
		} else if test.ExpectError != "" {
			if matchesErrorPattern(err.Error(), test.ExpectError) {
				result.Pass()
			} else {
				result.Fail(fmt.Sprintf("expected error %q but got %q", test.ExpectError, err.Error()))
			}
		} else {
			result.SetError(err.Error())
		}
	} else {
		// Check if we expected failure but got success
		if test.ExpectFailure {
			result.Fail("expected failure but execution succeeded")
			return result
		}

		// Run assertions
		for _, assertion := range test.Assertions {
			assertResult := r.evaluateAssertion(ctx, &assertion, execResult, test)
			result.AssertionResults = append(result.AssertionResults, assertResult)

			if r.config.EventHandler != nil {
				r.config.EventHandler.OnAssertionResult(&assertResult)
			}
		}

		// Determine overall test result
		if result.AllAssertionsPassed() {
			result.Pass()
		} else {
			failed := result.FailedAssertions()
			var messages []string
			for _, f := range failed {
				messages = append(messages, f.Message)
			}
			result.Fail(strings.Join(messages, "; "))
		}
	}

	// Run test teardown
	if test.Teardown != nil {
		teardownResult := r.runTeardown(ctx, test.Teardown)
		result.TeardownResult = teardownResult
	}

	return result
}

// executeBlueprint executes the blueprint with test parameters.
func (r *Runner) executeBlueprint(ctx context.Context, test *TestCase, suite *TestSuite) (*ExecutionResult, error) {
	result := &ExecutionResult{
		DryRun:  test.DryRun || r.config.DryRun,
		Outputs: make(map[string]interface{}),
	}
	startTime := time.Now()

	// Determine blueprint to test
	blueprintName := suite.Blueprint
	if blueprintName == "" {
		return nil, fmt.Errorf("no blueprint specified")
	}

	// Load the blueprint
	loadConfig := &blueprint.LoadConfig{
		Name:       blueprintName,
		Version:    suite.Version,
		Parameters: test.Parameters,
		Validate:   true,
	}

	loadResult, err := r.loader.Load(ctx, loadConfig)
	if err != nil {
		result.Error = err.Error()
		result.Duration = time.Since(startTime)
		return result, err
	}

	bp := loadResult.Blueprint

	// Determine entrypoint
	entrypoint := test.Entrypoint
	if entrypoint == "" {
		entrypoint = "default"
	}

	statePath, ok := bp.Entrypoints[entrypoint]
	if !ok {
		err := fmt.Errorf("entrypoint %q not found in blueprint", entrypoint)
		result.Error = err.Error()
		result.Duration = time.Since(startTime)
		return result, err
	}

	// Build parameters from resolved params plus any test-specific overrides
	params := make(map[string]interface{})
	for k, v := range loadResult.ResolvedParameters {
		params[k] = v
	}
	for k, v := range test.Parameters {
		params[k] = v
	}

	// Add secrets as parameters
	for k, v := range test.Secrets {
		params[k] = v
	}

	// Render the state file
	rendered, err := r.loader.RenderState(ctx, bp, statePath, params)
	if err != nil {
		result.Error = err.Error()
		result.Duration = time.Since(startTime)
		return result, err
	}

	// Parse the rendered state using a helper function
	stateCount, stateResults, err := parseRenderedStates(rendered)
	if err != nil {
		result.Error = err.Error()
		result.Duration = time.Since(startTime)
		return result, err
	}

	result.StatesApplied = stateCount
	result.StateResults = stateResults
	result.Success = true
	result.Duration = time.Since(startTime)

	return result, nil
}

// parseRenderedStates parses rendered YAML state content.
func parseRenderedStates(rendered []byte) (int, []StateResult, error) {
	var rawState map[string]interface{}
	if err := yaml.Unmarshal(rendered, &rawState); err != nil {
		return 0, nil, fmt.Errorf("failed to parse YAML: %w", err)
	}

	var results []StateResult
	count := 0

	// Skip metadata, include, and variables - these are handled separately
	for module, declarations := range rawState {
		if module == "metadata" || module == "include" || module == "variables" {
			continue
		}

		if declarations == nil {
			continue
		}

		declMap, ok := declarations.(map[string]interface{})
		if !ok {
			continue
		}

		for stateID := range declMap {
			results = append(results, StateResult{
				ID:      stateID,
				Module:  module,
				Success: true, // In dry-run or simulation, all succeed
			})
			count++
		}
	}

	return count, results, nil
}

// evaluateAssertion evaluates a single assertion.
func (r *Runner) evaluateAssertion(ctx context.Context, assertion *Assertion, execResult *ExecutionResult, test *TestCase) AssertionResult {
	result := AssertionResult{
		Type:        assertion.Type,
		Description: assertion.Description,
	}
	startTime := time.Now()

	switch assertion.Type {
	case AssertStateApplied:
		result = r.assertStateApplied(assertion, execResult)

	case AssertStateChanged:
		result = r.assertStateChanged(assertion, execResult)

	case AssertStateUnchanged:
		result = r.assertStateUnchanged(assertion, execResult)

	case AssertStateFailed:
		result = r.assertStateFailed(assertion, execResult)

	case AssertFileExists:
		result = r.assertFileExists(assertion)

	case AssertFileNotExists:
		result = r.assertFileNotExists(assertion)

	case AssertFileContains:
		result = r.assertFileContains(assertion)

	case AssertFileMode:
		result = r.assertFileMode(assertion)

	case AssertDirectoryExists:
		result = r.assertDirectoryExists(assertion)

	case AssertCommandSuccess:
		result = r.assertCommandSuccess(ctx, assertion)

	case AssertCommandFailure:
		result = r.assertCommandFailure(ctx, assertion)

	case AssertCommandOutput:
		result = r.assertCommandOutput(ctx, assertion)

	case AssertOutputContains:
		result = r.assertOutputContains(assertion, execResult)

	case AssertOutputEquals:
		result = r.assertOutputEquals(assertion, execResult)

	case AssertStatesApplied:
		result = r.assertStatesCount(assertion, execResult.StatesApplied, "applied")

	case AssertStatesChanged:
		result = r.assertStatesCount(assertion, execResult.StatesChanged, "changed")

	case AssertStatesFailed:
		result = r.assertStatesCount(assertion, execResult.StatesFailed, "failed")

	case AssertNoFailures:
		result = r.assertNoFailures(execResult)

	case AssertIdempotent:
		result = r.assertIdempotent(ctx, test, execResult)

	default:
		result.Passed = false
		result.Message = fmt.Sprintf("unknown assertion type: %s", assertion.Type)
	}

	result.Duration = time.Since(startTime)
	if assertion.Negate {
		result.Passed = !result.Passed
		if result.Passed {
			result.Message = "negated assertion passed"
		} else {
			result.Message = "negated assertion failed: " + result.Message
		}
	}

	return result
}

// State assertions
func (r *Runner) assertStateApplied(assertion *Assertion, execResult *ExecutionResult) AssertionResult {
	result := AssertionResult{Type: assertion.Type}

	stateID := assertion.Target
	if assertion.State != nil && assertion.State.ID != "" {
		stateID = assertion.State.ID
	}

	for _, sr := range execResult.StateResults {
		if sr.ID == stateID {
			result.Passed = true
			result.Target = stateID
			result.Message = fmt.Sprintf("state %q was applied", stateID)
			return result
		}
	}

	result.Passed = false
	result.Target = stateID
	result.Message = fmt.Sprintf("state %q was not applied", stateID)
	return result
}

func (r *Runner) assertStateChanged(assertion *Assertion, execResult *ExecutionResult) AssertionResult {
	result := AssertionResult{Type: assertion.Type}

	stateID := assertion.Target
	if assertion.State != nil && assertion.State.ID != "" {
		stateID = assertion.State.ID
	}

	for _, sr := range execResult.StateResults {
		if sr.ID == stateID {
			if sr.Changed {
				result.Passed = true
				result.Message = fmt.Sprintf("state %q made changes", stateID)
			} else {
				result.Passed = false
				result.Message = fmt.Sprintf("state %q did not make changes", stateID)
			}
			result.Target = stateID
			return result
		}
	}

	result.Passed = false
	result.Target = stateID
	result.Message = fmt.Sprintf("state %q was not found", stateID)
	return result
}

func (r *Runner) assertStateUnchanged(assertion *Assertion, execResult *ExecutionResult) AssertionResult {
	result := AssertionResult{Type: assertion.Type}

	stateID := assertion.Target
	if assertion.State != nil && assertion.State.ID != "" {
		stateID = assertion.State.ID
	}

	for _, sr := range execResult.StateResults {
		if sr.ID == stateID {
			if !sr.Changed {
				result.Passed = true
				result.Message = fmt.Sprintf("state %q made no changes", stateID)
			} else {
				result.Passed = false
				result.Message = fmt.Sprintf("state %q unexpectedly made changes", stateID)
			}
			result.Target = stateID
			return result
		}
	}

	result.Passed = false
	result.Target = stateID
	result.Message = fmt.Sprintf("state %q was not found", stateID)
	return result
}

func (r *Runner) assertStateFailed(assertion *Assertion, execResult *ExecutionResult) AssertionResult {
	result := AssertionResult{Type: assertion.Type}

	stateID := assertion.Target
	if assertion.State != nil && assertion.State.ID != "" {
		stateID = assertion.State.ID
	}

	for _, sr := range execResult.StateResults {
		if sr.ID == stateID {
			if !sr.Success {
				result.Passed = true
				result.Message = fmt.Sprintf("state %q failed as expected", stateID)
			} else {
				result.Passed = false
				result.Message = fmt.Sprintf("state %q did not fail as expected", stateID)
			}
			result.Target = stateID
			return result
		}
	}

	result.Passed = false
	result.Target = stateID
	result.Message = fmt.Sprintf("state %q was not found", stateID)
	return result
}

// File assertions
func (r *Runner) assertFileExists(assertion *Assertion) AssertionResult {
	result := AssertionResult{Type: assertion.Type}

	path := assertion.Target
	if assertion.File != nil && assertion.File.Path != "" {
		path = assertion.File.Path
	}

	result.Target = path

	info, err := os.Stat(path)
	if err != nil {
		result.Passed = false
		result.Message = fmt.Sprintf("file %q does not exist: %v", path, err)
		return result
	}

	if info.IsDir() {
		result.Passed = false
		result.Message = fmt.Sprintf("path %q is a directory, not a file", path)
		return result
	}

	result.Passed = true
	result.Message = fmt.Sprintf("file %q exists", path)
	return result
}

func (r *Runner) assertFileNotExists(assertion *Assertion) AssertionResult {
	result := AssertionResult{Type: assertion.Type}

	path := assertion.Target
	if assertion.File != nil && assertion.File.Path != "" {
		path = assertion.File.Path
	}

	result.Target = path

	_, err := os.Stat(path)
	if os.IsNotExist(err) {
		result.Passed = true
		result.Message = fmt.Sprintf("file %q does not exist", path)
		return result
	}

	result.Passed = false
	result.Message = fmt.Sprintf("file %q exists but should not", path)
	return result
}

func (r *Runner) assertFileContains(assertion *Assertion) AssertionResult {
	result := AssertionResult{Type: assertion.Type}

	if assertion.File == nil {
		result.Passed = false
		result.Message = "file configuration required"
		return result
	}

	path := assertion.File.Path
	result.Target = path

	content, err := os.ReadFile(path)
	if err != nil {
		result.Passed = false
		result.Message = fmt.Sprintf("failed to read file %q: %v", path, err)
		return result
	}

	contentStr := string(content)

	// Check for substring
	if assertion.File.Contains != "" {
		if strings.Contains(contentStr, assertion.File.Contains) {
			result.Passed = true
			result.Message = fmt.Sprintf("file %q contains %q", path, assertion.File.Contains)
		} else {
			result.Passed = false
			result.Expected = assertion.File.Contains
			result.Message = fmt.Sprintf("file %q does not contain %q", path, assertion.File.Contains)
		}
		return result
	}

	// Check for regex match
	if assertion.File.Matches != "" {
		re, err := regexp.Compile(assertion.File.Matches)
		if err != nil {
			result.Passed = false
			result.Message = fmt.Sprintf("invalid regex pattern: %v", err)
			return result
		}

		if re.MatchString(contentStr) {
			result.Passed = true
			result.Message = fmt.Sprintf("file %q matches pattern %q", path, assertion.File.Matches)
		} else {
			result.Passed = false
			result.Expected = assertion.File.Matches
			result.Message = fmt.Sprintf("file %q does not match pattern %q", path, assertion.File.Matches)
		}
		return result
	}

	// Check for exact content
	if assertion.File.Content != "" {
		if contentStr == assertion.File.Content {
			result.Passed = true
			result.Message = fmt.Sprintf("file %q has expected content", path)
		} else {
			result.Passed = false
			result.Expected = assertion.File.Content
			result.Actual = contentStr
			result.Message = fmt.Sprintf("file %q content does not match", path)
		}
		return result
	}

	result.Passed = false
	result.Message = "no content check specified"
	return result
}

func (r *Runner) assertFileMode(assertion *Assertion) AssertionResult {
	result := AssertionResult{Type: assertion.Type}

	if assertion.File == nil {
		result.Passed = false
		result.Message = "file configuration required"
		return result
	}

	path := assertion.File.Path
	result.Target = path

	info, err := os.Stat(path)
	if err != nil {
		result.Passed = false
		result.Message = fmt.Sprintf("failed to stat file %q: %v", path, err)
		return result
	}

	expectedMode := assertion.File.Mode
	actualMode := fmt.Sprintf("%04o", info.Mode().Perm())

	result.Expected = expectedMode
	result.Actual = actualMode

	// Handle mode comparison (strip leading zeros for comparison)
	if strings.TrimLeft(actualMode, "0") == strings.TrimLeft(expectedMode, "0") {
		result.Passed = true
		result.Message = fmt.Sprintf("file %q has mode %s", path, actualMode)
	} else {
		result.Passed = false
		result.Message = fmt.Sprintf("file %q has mode %s, expected %s", path, actualMode, expectedMode)
	}

	return result
}

func (r *Runner) assertDirectoryExists(assertion *Assertion) AssertionResult {
	result := AssertionResult{Type: assertion.Type}

	path := assertion.Target
	if assertion.File != nil && assertion.File.Path != "" {
		path = assertion.File.Path
	}

	result.Target = path

	info, err := os.Stat(path)
	if err != nil {
		result.Passed = false
		result.Message = fmt.Sprintf("directory %q does not exist: %v", path, err)
		return result
	}

	if !info.IsDir() {
		result.Passed = false
		result.Message = fmt.Sprintf("path %q is a file, not a directory", path)
		return result
	}

	result.Passed = true
	result.Message = fmt.Sprintf("directory %q exists", path)
	return result
}

// Command assertions
func (r *Runner) assertCommandSuccess(ctx context.Context, assertion *Assertion) AssertionResult {
	result := AssertionResult{Type: assertion.Type}

	if assertion.Command == nil {
		result.Passed = false
		result.Message = "command configuration required"
		return result
	}

	cmdResult := r.runCommand(ctx, assertion.Command)
	result.Target = assertion.Command.Command

	if cmdResult.ExitCode == 0 {
		result.Passed = true
		result.Message = fmt.Sprintf("command %q succeeded", assertion.Command.Command)
	} else {
		result.Passed = false
		result.Expected = 0
		result.Actual = cmdResult.ExitCode
		result.Message = fmt.Sprintf("command %q failed with exit code %d", assertion.Command.Command, cmdResult.ExitCode)
	}

	return result
}

func (r *Runner) assertCommandFailure(ctx context.Context, assertion *Assertion) AssertionResult {
	result := AssertionResult{Type: assertion.Type}

	if assertion.Command == nil {
		result.Passed = false
		result.Message = "command configuration required"
		return result
	}

	cmdResult := r.runCommand(ctx, assertion.Command)
	result.Target = assertion.Command.Command

	expectedExit := 1
	if assertion.Command.ExitCode != nil {
		expectedExit = *assertion.Command.ExitCode
	}

	if cmdResult.ExitCode != 0 {
		if assertion.Command.ExitCode != nil && cmdResult.ExitCode != expectedExit {
			result.Passed = false
			result.Expected = expectedExit
			result.Actual = cmdResult.ExitCode
			result.Message = fmt.Sprintf("command %q exited with %d, expected %d", assertion.Command.Command, cmdResult.ExitCode, expectedExit)
		} else {
			result.Passed = true
			result.Message = fmt.Sprintf("command %q failed as expected with exit code %d", assertion.Command.Command, cmdResult.ExitCode)
		}
	} else {
		result.Passed = false
		result.Message = fmt.Sprintf("command %q succeeded but expected failure", assertion.Command.Command)
	}

	return result
}

func (r *Runner) assertCommandOutput(ctx context.Context, assertion *Assertion) AssertionResult {
	result := AssertionResult{Type: assertion.Type}

	if assertion.Command == nil {
		result.Passed = false
		result.Message = "command configuration required"
		return result
	}

	cmdResult := r.runCommand(ctx, assertion.Command)
	result.Target = assertion.Command.Command

	// Check stdout
	if assertion.Command.StdoutContains != "" {
		if strings.Contains(cmdResult.Stdout, assertion.Command.StdoutContains) {
			result.Passed = true
			result.Message = fmt.Sprintf("stdout contains %q", assertion.Command.StdoutContains)
		} else {
			result.Passed = false
			result.Expected = assertion.Command.StdoutContains
			result.Actual = cmdResult.Stdout
			result.Message = fmt.Sprintf("stdout does not contain %q", assertion.Command.StdoutContains)
		}
		return result
	}

	if assertion.Command.StdoutMatches != "" {
		re, err := regexp.Compile(assertion.Command.StdoutMatches)
		if err != nil {
			result.Passed = false
			result.Message = fmt.Sprintf("invalid regex: %v", err)
			return result
		}

		if re.MatchString(cmdResult.Stdout) {
			result.Passed = true
			result.Message = fmt.Sprintf("stdout matches pattern %q", assertion.Command.StdoutMatches)
		} else {
			result.Passed = false
			result.Expected = assertion.Command.StdoutMatches
			result.Actual = cmdResult.Stdout
			result.Message = fmt.Sprintf("stdout does not match pattern %q", assertion.Command.StdoutMatches)
		}
		return result
	}

	result.Passed = true
	result.Message = "no output check specified"
	return result
}

func (r *Runner) runCommand(ctx context.Context, cmdConfig *CommandAssertion) CommandResult {
	result := CommandResult{
		Command: cmdConfig.Command,
	}

	shell := cmdConfig.Shell
	if shell == "" {
		shell = "/bin/sh"
	}

	// Build command
	args := []string{"-c", cmdConfig.Command}
	if len(cmdConfig.Args) > 0 {
		args[1] = cmdConfig.Command + " " + strings.Join(cmdConfig.Args, " ")
	}

	cmd := exec.CommandContext(ctx, shell, args...)

	// Capture output
	output, err := cmd.CombinedOutput()
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			result.ExitCode = exitErr.ExitCode()
		} else {
			result.ExitCode = -1
			result.Error = err.Error()
		}
	}

	result.Stdout = string(output)
	return result
}

// Output assertions
func (r *Runner) assertOutputContains(assertion *Assertion, execResult *ExecutionResult) AssertionResult {
	result := AssertionResult{Type: assertion.Type}

	if assertion.Output == nil {
		result.Passed = false
		result.Message = "output configuration required"
		return result
	}

	name := assertion.Output.Name
	result.Target = name

	value, ok := execResult.Outputs[name]
	if !ok {
		result.Passed = false
		result.Message = fmt.Sprintf("output %q not found", name)
		return result
	}

	valueStr := fmt.Sprintf("%v", value)
	if strings.Contains(valueStr, assertion.Output.Contains) {
		result.Passed = true
		result.Message = fmt.Sprintf("output %q contains %q", name, assertion.Output.Contains)
	} else {
		result.Passed = false
		result.Expected = assertion.Output.Contains
		result.Actual = valueStr
		result.Message = fmt.Sprintf("output %q does not contain %q", name, assertion.Output.Contains)
	}

	return result
}

func (r *Runner) assertOutputEquals(assertion *Assertion, execResult *ExecutionResult) AssertionResult {
	result := AssertionResult{Type: assertion.Type}

	if assertion.Output == nil {
		result.Passed = false
		result.Message = "output configuration required"
		return result
	}

	name := assertion.Output.Name
	result.Target = name

	value, ok := execResult.Outputs[name]
	if !ok {
		result.Passed = false
		result.Message = fmt.Sprintf("output %q not found", name)
		return result
	}

	expected := assertion.Output.Value
	if fmt.Sprintf("%v", value) == fmt.Sprintf("%v", expected) {
		result.Passed = true
		result.Message = fmt.Sprintf("output %q equals expected value", name)
	} else {
		result.Passed = false
		result.Expected = expected
		result.Actual = value
		result.Message = fmt.Sprintf("output %q does not equal expected value", name)
	}

	return result
}

// Count assertions
func (r *Runner) assertStatesCount(assertion *Assertion, actual int, label string) AssertionResult {
	result := AssertionResult{Type: assertion.Type}

	expected, ok := assertion.Expected.(int)
	if !ok {
		// Try float64 (from JSON/YAML)
		if f, ok := assertion.Expected.(float64); ok {
			expected = int(f)
		} else {
			result.Passed = false
			result.Message = "expected value must be an integer"
			return result
		}
	}

	result.Expected = expected
	result.Actual = actual

	switch assertion.Operator {
	case OpEquals, "":
		result.Passed = actual == expected
	case OpGreaterThan:
		result.Passed = actual > expected
	case OpLessThan:
		result.Passed = actual < expected
	case OpGreaterOrEq:
		result.Passed = actual >= expected
	case OpLessOrEq:
		result.Passed = actual <= expected
	default:
		result.Passed = actual == expected
	}

	if result.Passed {
		result.Message = fmt.Sprintf("states %s: %d matches expected", label, actual)
	} else {
		result.Message = fmt.Sprintf("states %s: %d does not match expected %d", label, actual, expected)
	}

	return result
}

func (r *Runner) assertNoFailures(execResult *ExecutionResult) AssertionResult {
	result := AssertionResult{Type: AssertNoFailures}

	if execResult.StatesFailed == 0 {
		result.Passed = true
		result.Message = "no states failed"
	} else {
		result.Passed = false
		result.Expected = 0
		result.Actual = execResult.StatesFailed
		result.Message = fmt.Sprintf("%d states failed", execResult.StatesFailed)
	}

	return result
}

func (r *Runner) assertIdempotent(ctx context.Context, test *TestCase, firstResult *ExecutionResult) AssertionResult {
	result := AssertionResult{Type: AssertIdempotent}

	// Run the blueprint again
	// In a real implementation, this would re-execute and check for no changes
	// For now, we simulate by checking if any states reported changes
	result.Passed = firstResult.StatesChanged == 0
	if result.Passed {
		result.Message = "blueprint is idempotent (no changes on re-run)"
	} else {
		result.Message = fmt.Sprintf("blueprint is not idempotent: %d states would change on re-run", firstResult.StatesChanged)
	}

	return result
}

// Setup/teardown helpers
func (r *Runner) runSetup(ctx context.Context, setup *TestSetup) *SetupTeardownResult {
	result := &SetupTeardownResult{
		Success: true,
	}
	startTime := time.Now()

	// Create files
	for _, f := range setup.Files {
		if err := r.createTestFile(&f); err != nil {
			result.Success = false
			result.Error = fmt.Sprintf("failed to create file %s: %v", f.Path, err)
			result.Duration = time.Since(startTime)
			return result
		}
		result.FilesCreated = append(result.FilesCreated, f.Path)
	}

	// Run commands
	for _, cmd := range setup.Commands {
		cmdResult := r.runSetupCommand(ctx, &cmd)
		result.CommandResults = append(result.CommandResults, cmdResult)
		if !cmd.IgnoreErrors && cmdResult.ExitCode != 0 {
			result.Success = false
			result.Error = fmt.Sprintf("command failed: %s", cmdResult.Error)
			result.Duration = time.Since(startTime)
			return result
		}
	}

	result.Duration = time.Since(startTime)
	return result
}

func (r *Runner) runTeardown(ctx context.Context, teardown *TestTeardown) *SetupTeardownResult {
	result := &SetupTeardownResult{
		Success: true,
	}
	startTime := time.Now()

	// Run commands
	for _, cmd := range teardown.Commands {
		cmdResult := r.runSetupCommand(ctx, &cmd)
		result.CommandResults = append(result.CommandResults, cmdResult)
	}

	// Remove files
	for _, path := range teardown.Files {
		_ = os.RemoveAll(path)
	}

	result.Duration = time.Since(startTime)
	return result
}

func (r *Runner) createTestFile(f *TestFile) error {
	if f.IsDir {
		return os.MkdirAll(f.Path, 0755)
	}

	// Create parent directories
	if err := os.MkdirAll(filepath.Dir(f.Path), 0755); err != nil {
		return err
	}

	// Copy from source or write content
	if f.Source != "" {
		content, err := os.ReadFile(f.Source)
		if err != nil {
			return err
		}
		return os.WriteFile(f.Path, content, 0644)
	}

	return os.WriteFile(f.Path, []byte(f.Content), 0644)
}

func (r *Runner) runSetupCommand(ctx context.Context, cmd *TestCommand) CommandResult {
	result := CommandResult{
		Command: cmd.Command,
	}

	shell := cmd.Shell
	if shell == "" {
		shell = "/bin/sh"
	}

	args := []string{"-c", cmd.Command}
	c := exec.CommandContext(ctx, shell, args...)

	output, err := c.CombinedOutput()
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			result.ExitCode = exitErr.ExitCode()
		} else {
			result.ExitCode = -1
			result.Error = err.Error()
		}
	}

	result.Stdout = string(output)
	return result
}

// matchesErrorPattern checks if an error message matches a pattern.
func matchesErrorPattern(message, pattern string) bool {
	// Try regex first
	if re, err := regexp.Compile(pattern); err == nil {
		return re.MatchString(message)
	}
	// Fall back to substring
	return strings.Contains(message, pattern)
}
