package testing

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestDefaultRunnerConfig(t *testing.T) {
	config := DefaultRunnerConfig()
	if config == nil {
		t.Fatal("DefaultRunnerConfig returned nil")
	}
	if config.Timeout != 5*time.Minute {
		t.Errorf("Timeout = %v, want 5m", config.Timeout)
	}
	if config.MaxParallel != 4 {
		t.Errorf("MaxParallel = %d, want 4", config.MaxParallel)
	}
	if config.TempDir != os.TempDir() {
		t.Errorf("TempDir = %q, want %q", config.TempDir, os.TempDir())
	}
}

func TestRunnerConfig_Defaults(t *testing.T) {
	config := &RunnerConfig{}
	// Zero values should be set
	if config.DryRun != false {
		t.Error("DryRun should default to false")
	}
	if config.Parallel != false {
		t.Error("Parallel should default to false")
	}
	if config.StopOnFailure != false {
		t.Error("StopOnFailure should default to false")
	}
	if config.Verbose != false {
		t.Error("Verbose should default to false")
	}
}

func TestMatchesErrorPattern_Substring(t *testing.T) {
	tests := []struct {
		message string
		pattern string
		want    bool
	}{
		{"file not found", "not found", true},
		{"file not found", "file", true},
		{"file not found", "found", true},
		{"file not found", "missing", false},
		{"connection timeout", "timeout", true},
		{"connection timeout", "connect", true},
		{"", "", true},           // empty pattern matches empty string
		{"some error", "", true}, // empty pattern matches anything
	}

	for _, tt := range tests {
		got := matchesErrorPattern(tt.message, tt.pattern)
		if got != tt.want {
			t.Errorf("matchesErrorPattern(%q, %q) = %v, want %v", tt.message, tt.pattern, got, tt.want)
		}
	}
}

func TestMatchesErrorPattern_Regex(t *testing.T) {
	tests := []struct {
		message string
		pattern string
		want    bool
	}{
		{"error code 123", "code \\d+", true},
		{"error code abc", "code \\d+", false},
		{"file.txt", `\.txt$`, true},
		{"file.log", `\.txt$`, false},
		{"prefix_middle_suffix", "^prefix", true},
		{"prefix_middle_suffix", "suffix$", true},
		{"prefix_middle_suffix", "^prefix.*suffix$", true},
	}

	for _, tt := range tests {
		got := matchesErrorPattern(tt.message, tt.pattern)
		if got != tt.want {
			t.Errorf("matchesErrorPattern(%q, %q) = %v, want %v", tt.message, tt.pattern, got, tt.want)
		}
	}
}

func TestNewRunner_NilConfig(t *testing.T) {
	// NewRunner with nil config should use defaults
	// This will fail because there's no storage, but it tests the config path
	runner, err := NewRunner(nil)
	if err != nil {
		t.Logf("Expected error with nil config: %v", err)
	}
	// With nil config, it uses defaults which have no BlueprintPath
	// so it creates a nil storage which is acceptable
	if runner == nil && err == nil {
		t.Error("NewRunner returned nil runner without error")
	}
}

func TestNewRunner_WithBlueprintPath(t *testing.T) {
	// Create a temporary directory
	tmpDir := t.TempDir()

	config := &RunnerConfig{
		BlueprintPath: tmpDir,
		Timeout:       1 * time.Minute,
		MaxParallel:   2,
	}

	runner, err := NewRunner(config)
	if err != nil {
		t.Fatalf("NewRunner failed: %v", err)
	}
	if runner == nil {
		t.Fatal("NewRunner returned nil")
	}
	if runner.config != config {
		t.Error("Runner config not set correctly")
	}
}

func TestNewRunner_InvalidBlueprintPath(t *testing.T) {
	config := &RunnerConfig{
		BlueprintPath: "/nonexistent/path/to/blueprints",
	}

	runner, err := NewRunner(config)
	if err == nil {
		t.Error("Expected error for invalid blueprint path")
	}
	if runner != nil {
		t.Error("Expected nil runner for invalid path")
	}
}

// MockEventHandler implements TestEventHandler for testing
type MockEventHandler struct {
	SuiteStartCalled int
	SuiteEndCalled   int
	TestStartCalled  int
	TestEndCalled    int
	AssertionCalled  int
	LastSuite        *TestSuite
	LastSuiteResult  *TestSuiteResult
	LastTest         *TestCase
	LastTestResult   *TestResult
	LastAssertion    *AssertionResult
}

func (m *MockEventHandler) OnSuiteStart(suite *TestSuite) {
	m.SuiteStartCalled++
	m.LastSuite = suite
}

func (m *MockEventHandler) OnSuiteEnd(result *TestSuiteResult) {
	m.SuiteEndCalled++
	m.LastSuiteResult = result
}

func (m *MockEventHandler) OnTestStart(test *TestCase) {
	m.TestStartCalled++
	m.LastTest = test
}

func (m *MockEventHandler) OnTestEnd(result *TestResult) {
	m.TestEndCalled++
	m.LastTestResult = result
}

func (m *MockEventHandler) OnAssertionResult(result *AssertionResult) {
	m.AssertionCalled++
	m.LastAssertion = result
}

func TestMockEventHandler(t *testing.T) {
	handler := &MockEventHandler{}

	suite := &TestSuite{Name: "test-suite"}
	suiteResult := &TestSuiteResult{Name: "test-suite"}
	test := &TestCase{Name: "test1"}
	testResult := &TestResult{Name: "test1"}
	assertion := &AssertionResult{Type: AssertFileExists, Passed: true}

	handler.OnSuiteStart(suite)
	if handler.SuiteStartCalled != 1 {
		t.Errorf("SuiteStartCalled = %d, want 1", handler.SuiteStartCalled)
	}
	if handler.LastSuite != suite {
		t.Error("LastSuite not set")
	}

	handler.OnSuiteEnd(suiteResult)
	if handler.SuiteEndCalled != 1 {
		t.Errorf("SuiteEndCalled = %d, want 1", handler.SuiteEndCalled)
	}

	handler.OnTestStart(test)
	if handler.TestStartCalled != 1 {
		t.Errorf("TestStartCalled = %d, want 1", handler.TestStartCalled)
	}

	handler.OnTestEnd(testResult)
	if handler.TestEndCalled != 1 {
		t.Errorf("TestEndCalled = %d, want 1", handler.TestEndCalled)
	}

	handler.OnAssertionResult(assertion)
	if handler.AssertionCalled != 1 {
		t.Errorf("AssertionCalled = %d, want 1", handler.AssertionCalled)
	}
}

func TestRunner_CreateTestFile(t *testing.T) {
	tmpDir := t.TempDir()

	config := &RunnerConfig{
		BlueprintPath: tmpDir,
		TempDir:       tmpDir,
	}

	runner, err := NewRunner(config)
	if err != nil {
		t.Fatalf("NewRunner failed: %v", err)
	}

	// Test creating a file with content
	testFile := &TestFile{
		Path:    filepath.Join(tmpDir, "test.txt"),
		Content: "hello world",
	}

	if err := runner.createTestFile(testFile); err != nil {
		t.Fatalf("createTestFile failed: %v", err)
	}

	content, err := os.ReadFile(testFile.Path)
	if err != nil {
		t.Fatalf("Failed to read created file: %v", err)
	}
	if string(content) != "hello world" {
		t.Errorf("Content = %q, want %q", string(content), "hello world")
	}
}

func TestRunner_CreateTestFile_Directory(t *testing.T) {
	tmpDir := t.TempDir()

	config := &RunnerConfig{
		BlueprintPath: tmpDir,
		TempDir:       tmpDir,
	}

	runner, err := NewRunner(config)
	if err != nil {
		t.Fatalf("NewRunner failed: %v", err)
	}

	// Test creating a directory
	testDir := &TestFile{
		Path:  filepath.Join(tmpDir, "subdir"),
		IsDir: true,
	}

	if err := runner.createTestFile(testDir); err != nil {
		t.Fatalf("createTestFile (dir) failed: %v", err)
	}

	info, err := os.Stat(testDir.Path)
	if err != nil {
		t.Fatalf("Failed to stat created directory: %v", err)
	}
	if !info.IsDir() {
		t.Error("Created path is not a directory")
	}
}

func TestRunner_CreateTestFile_WithSource(t *testing.T) {
	tmpDir := t.TempDir()

	// Create source file
	sourceFile := filepath.Join(tmpDir, "source.txt")
	if err := os.WriteFile(sourceFile, []byte("source content"), 0644); err != nil {
		t.Fatalf("Failed to create source file: %v", err)
	}

	config := &RunnerConfig{
		BlueprintPath: tmpDir,
		TempDir:       tmpDir,
	}

	runner, err := NewRunner(config)
	if err != nil {
		t.Fatalf("NewRunner failed: %v", err)
	}

	// Test creating a file from source
	testFile := &TestFile{
		Path:   filepath.Join(tmpDir, "dest.txt"),
		Source: sourceFile,
	}

	if err := runner.createTestFile(testFile); err != nil {
		t.Fatalf("createTestFile (source) failed: %v", err)
	}

	content, err := os.ReadFile(testFile.Path)
	if err != nil {
		t.Fatalf("Failed to read created file: %v", err)
	}
	if string(content) != "source content" {
		t.Errorf("Content = %q, want %q", string(content), "source content")
	}
}

func TestRunner_RunSetup(t *testing.T) {
	tmpDir := t.TempDir()

	config := &RunnerConfig{
		BlueprintPath: tmpDir,
		TempDir:       tmpDir,
	}

	runner, err := NewRunner(config)
	if err != nil {
		t.Fatalf("NewRunner failed: %v", err)
	}

	ctx := context.Background()

	setup := &TestSetup{
		Files: []TestFile{
			{
				Path:    filepath.Join(tmpDir, "setup-file.txt"),
				Content: "setup content",
			},
		},
		Commands: []TestCommand{
			{
				Command: "echo 'setup ran'",
			},
		},
	}

	result := runner.runSetup(ctx, setup)
	if !result.Success {
		t.Errorf("Setup failed: %s", result.Error)
	}
	if len(result.FilesCreated) != 1 {
		t.Errorf("FilesCreated = %d, want 1", len(result.FilesCreated))
	}
	if len(result.CommandResults) != 1 {
		t.Errorf("CommandResults = %d, want 1", len(result.CommandResults))
	}

	// Verify file was created
	if _, err := os.Stat(filepath.Join(tmpDir, "setup-file.txt")); os.IsNotExist(err) {
		t.Error("Setup file was not created")
	}
}

func TestRunner_RunTeardown(t *testing.T) {
	tmpDir := t.TempDir()

	config := &RunnerConfig{
		BlueprintPath: tmpDir,
		TempDir:       tmpDir,
	}

	runner, err := NewRunner(config)
	if err != nil {
		t.Fatalf("NewRunner failed: %v", err)
	}

	ctx := context.Background()

	// Create a file to be removed
	testFile := filepath.Join(tmpDir, "to-remove.txt")
	if err := os.WriteFile(testFile, []byte("test"), 0644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	teardown := &TestTeardown{
		Files: []string{testFile},
		Commands: []TestCommand{
			{
				Command: "echo 'teardown ran'",
			},
		},
	}

	result := runner.runTeardown(ctx, teardown)
	if !result.Success {
		t.Errorf("Teardown failed: %s", result.Error)
	}

	// Verify file was removed
	if _, err := os.Stat(testFile); !os.IsNotExist(err) {
		t.Error("Teardown file was not removed")
	}
}

func TestRunner_RunSetupCommand(t *testing.T) {
	tmpDir := t.TempDir()

	config := &RunnerConfig{
		BlueprintPath: tmpDir,
		TempDir:       tmpDir,
	}

	runner, err := NewRunner(config)
	if err != nil {
		t.Fatalf("NewRunner failed: %v", err)
	}

	ctx := context.Background()

	tests := []struct {
		name    string
		cmd     *TestCommand
		wantOut string
		wantErr int
	}{
		{
			name:    "simple echo",
			cmd:     &TestCommand{Command: "echo hello"},
			wantOut: "hello\n",
			wantErr: 0,
		},
		{
			name:    "failing command",
			cmd:     &TestCommand{Command: "exit 1"},
			wantErr: 1,
		},
		{
			name:    "custom exit code",
			cmd:     &TestCommand{Command: "exit 42"},
			wantErr: 42,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := runner.runSetupCommand(ctx, tt.cmd)
			if result.ExitCode != tt.wantErr {
				t.Errorf("ExitCode = %d, want %d", result.ExitCode, tt.wantErr)
			}
			if tt.wantOut != "" && result.Stdout != tt.wantOut {
				t.Errorf("Stdout = %q, want %q", result.Stdout, tt.wantOut)
			}
		})
	}
}

func TestParseRenderedStates(t *testing.T) {
	tests := []struct {
		name      string
		yaml      string
		wantCount int
		wantErr   bool
	}{
		{
			name: "single state",
			yaml: `
file:
  /etc/app/config.yaml:
    content: test
`,
			wantCount: 1,
			wantErr:   false,
		},
		{
			name: "multiple states",
			yaml: `
file:
  /etc/file1: {}
  /etc/file2: {}
package:
  nginx: {}
  redis: {}
`,
			wantCount: 4,
			wantErr:   false,
		},
		{
			name: "with metadata skipped",
			yaml: `
metadata:
  name: test
variables:
  key: value
include:
  - other.yaml
file:
  /etc/app.conf: {}
`,
			wantCount: 1,
			wantErr:   false,
		},
		{
			name:      "empty yaml",
			yaml:      ``,
			wantCount: 0,
			wantErr:   false,
		},
		{
			name: "invalid yaml",
			yaml: `
this is not valid yaml: [
`,
			wantCount: 0,
			wantErr:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			count, _, err := parseRenderedStates([]byte(tt.yaml))
			if (err != nil) != tt.wantErr {
				t.Errorf("parseRenderedStates() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if count != tt.wantCount {
				t.Errorf("parseRenderedStates() count = %d, want %d", count, tt.wantCount)
			}
		})
	}
}

func TestAssertionTypes_Defined(t *testing.T) {
	// Verify all assertion types are defined
	types := []AssertionType{
		AssertStateApplied,
		AssertStateChanged,
		AssertStateUnchanged,
		AssertStateFailed,
		AssertFileExists,
		AssertFileNotExists,
		AssertFileContains,
		AssertFileMode,
		AssertDirectoryExists,
		AssertCommandSuccess,
		AssertCommandFailure,
		AssertCommandOutput,
		AssertOutputContains,
		AssertOutputEquals,
		AssertStatesApplied,
		AssertStatesChanged,
		AssertStatesFailed,
		AssertNoFailures,
		AssertIdempotent,
	}

	for _, at := range types {
		if at == "" {
			t.Error("Found empty assertion type")
		}
	}

	// Verify they are distinct
	seen := make(map[AssertionType]bool)
	for _, at := range types {
		if seen[at] {
			t.Errorf("Duplicate assertion type: %s", at)
		}
		seen[at] = true
	}
}

func TestRunner_AssertionHelpers(t *testing.T) {
	tmpDir := t.TempDir()
	runner, err := NewRunner(&RunnerConfig{TempDir: tmpDir})
	if err != nil {
		t.Fatalf("NewRunner failed: %v", err)
	}

	execResult := &ExecutionResult{
		StatesApplied:   2,
		StatesChanged:   1,
		StatesFailed:    1,
		StatesUnchanged: 1,
		StateResults: []StateResult{
			{ID: "file1", Module: "file", Success: true, Changed: true},
			{ID: "file2", Module: "file", Success: false, Changed: false},
		},
		Outputs: map[string]interface{}{
			"version": "1.2.3",
			"count":   3,
		},
	}

	assertion := &Assertion{Type: AssertStateApplied, Target: "file1"}
	if !runner.assertStateApplied(assertion, execResult).Passed {
		t.Fatal("expected state applied assertion to pass")
	}

	assertion = &Assertion{Type: AssertStateChanged, Target: "file1"}
	if !runner.assertStateChanged(assertion, execResult).Passed {
		t.Fatal("expected state changed assertion to pass")
	}

	assertion = &Assertion{Type: AssertStateUnchanged, Target: "file2"}
	if !runner.assertStateUnchanged(assertion, execResult).Passed {
		t.Fatal("expected state unchanged assertion to pass")
	}

	assertion = &Assertion{Type: AssertStateFailed, Target: "file2"}
	if !runner.assertStateFailed(assertion, execResult).Passed {
		t.Fatal("expected state failed assertion to pass")
	}

	assertion = &Assertion{Type: AssertStatesApplied, Expected: 2}
	if !runner.assertStatesCount(assertion, execResult.StatesApplied, "applied").Passed {
		t.Fatal("expected states applied count to pass")
	}

	assertion = &Assertion{Type: AssertStatesChanged, Operator: OpGreaterThan, Expected: 0}
	if !runner.assertStatesCount(assertion, execResult.StatesChanged, "changed").Passed {
		t.Fatal("expected states changed count to pass")
	}

	assertion = &Assertion{Type: AssertStatesFailed, Operator: OpGreaterOrEq, Expected: 1}
	if !runner.assertStatesCount(assertion, execResult.StatesFailed, "failed").Passed {
		t.Fatal("expected states failed count to pass")
	}

	assertion = &Assertion{Type: AssertStatesApplied, Expected: "bad"}
	if runner.assertStatesCount(assertion, execResult.StatesApplied, "applied").Passed {
		t.Fatal("expected states count to fail for invalid expected type")
	}

	if runner.assertNoFailures(execResult).Passed {
		t.Fatal("expected no-failures assertion to fail when failures exist")
	}

	if runner.assertIdempotent(context.Background(), &TestCase{}, execResult).Passed {
		t.Fatal("expected idempotent assertion to fail when changes exist")
	}

	assertion = &Assertion{Type: AssertOutputContains, Output: &OutputAssertion{Name: "version", Contains: "1.2"}}
	if !runner.assertOutputContains(assertion, execResult).Passed {
		t.Fatal("expected output contains to pass")
	}

	assertion = &Assertion{Type: AssertOutputEquals, Output: &OutputAssertion{Name: "count", Value: 3}}
	if !runner.assertOutputEquals(assertion, execResult).Passed {
		t.Fatal("expected output equals to pass")
	}

	assertion = &Assertion{Type: AssertOutputEquals, Output: &OutputAssertion{Name: "missing", Value: 1}}
	if runner.assertOutputEquals(assertion, execResult).Passed {
		t.Fatal("expected output equals to fail for missing output")
	}

	filePath := filepath.Join(tmpDir, "file.txt")
	if err := os.WriteFile(filePath, []byte("hello world"), 0644); err != nil {
		t.Fatalf("failed to write file: %v", err)
	}

	assertion = &Assertion{Type: AssertFileExists, Target: filePath}
	if !runner.assertFileExists(assertion).Passed {
		t.Fatal("expected file exists to pass")
	}

	assertion = &Assertion{Type: AssertFileNotExists, Target: filepath.Join(tmpDir, "missing.txt")}
	if !runner.assertFileNotExists(assertion).Passed {
		t.Fatal("expected file not exists to pass")
	}

	assertion = &Assertion{Type: AssertFileContains, File: &FileAssertion{Path: filePath, Contains: "hello"}}
	if !runner.assertFileContains(assertion).Passed {
		t.Fatal("expected file contains to pass")
	}

	assertion = &Assertion{Type: AssertFileContains, File: &FileAssertion{Path: filePath, Matches: "("}}
	if runner.assertFileContains(assertion).Passed {
		t.Fatal("expected file contains to fail on invalid regex")
	}

	assertion = &Assertion{Type: AssertFileMode, File: &FileAssertion{Path: filePath, Mode: "0644"}}
	if !runner.assertFileMode(assertion).Passed {
		t.Fatal("expected file mode to pass")
	}

	dirPath := filepath.Join(tmpDir, "dir")
	if err := os.MkdirAll(dirPath, 0755); err != nil {
		t.Fatalf("failed to create dir: %v", err)
	}
	assertion = &Assertion{Type: AssertDirectoryExists, Target: dirPath}
	if !runner.assertDirectoryExists(assertion).Passed {
		t.Fatal("expected directory exists to pass")
	}

	assertion = &Assertion{Type: AssertCommandSuccess, Command: &CommandAssertion{Command: "true"}}
	if !runner.assertCommandSuccess(context.Background(), assertion).Passed {
		t.Fatal("expected command success to pass")
	}

	assertion = &Assertion{Type: AssertCommandFailure, Command: &CommandAssertion{Command: "false"}}
	if !runner.assertCommandFailure(context.Background(), assertion).Passed {
		t.Fatal("expected command failure to pass")
	}

	assertion = &Assertion{Type: AssertCommandOutput, Command: &CommandAssertion{Command: "echo hello", StdoutContains: "hello"}}
	if !runner.assertCommandOutput(context.Background(), assertion).Passed {
		t.Fatal("expected command output contains to pass")
	}
}

func TestRunner_RunSuite_StopOnFailure(t *testing.T) {
	blueprintsDir := setupTestBlueprint(t)

	runner, err := NewRunner(&RunnerConfig{
		BlueprintPath: blueprintsDir,
		StopOnFailure: true,
	})
	if err != nil {
		t.Fatalf("NewRunner failed: %v", err)
	}

	suite := &TestSuite{
		Name:      "stop-on-failure",
		Blueprint: "acme/demo",
		Tests: []TestCase{
			{
				Name:          "expected-failure-but-success",
				ExpectFailure: true,
			},
			{
				Name: "should-not-run",
			},
		},
	}

	result := runner.RunSuite(context.Background(), suite)
	if len(result.Tests) != 1 {
		t.Fatalf("Expected 1 test to run, got %d", len(result.Tests))
	}
	if result.Tests[0].Status != StatusFailed {
		t.Fatalf("Expected first test to fail, got %s", result.Tests[0].Status)
	}
}

func TestRunner_RunTest_ExpectErrorAndFailure(t *testing.T) {
	blueprintsDir := setupTestBlueprint(t)
	runner, err := NewRunner(&RunnerConfig{BlueprintPath: blueprintsDir})
	if err != nil {
		t.Fatalf("NewRunner failed: %v", err)
	}

	suite := &TestSuite{
		Name:      "expect-error",
		Blueprint: "acme/demo",
	}

	test := &TestCase{
		Name:        "expect-error",
		Entrypoint:  "missing",
		ExpectError: "entrypoint",
	}

	result := runner.RunTest(context.Background(), test, suite)
	if result.Status != StatusPassed {
		t.Fatalf("Expected test to pass, got %s (assertions: %+v)", result.Status, result.AssertionResults)
	}

	test = &TestCase{
		Name:          "expect-failure",
		Entrypoint:    "missing",
		ExpectFailure: true,
	}
	result = runner.RunTest(context.Background(), test, suite)
	if result.Status != StatusPassed {
		t.Fatalf("Expected test to pass, got %s", result.Status)
	}
}

func TestRunner_RunTest_NegatedAssertion(t *testing.T) {
	blueprintsDir := setupTestBlueprint(t)
	runner, err := NewRunner(&RunnerConfig{BlueprintPath: blueprintsDir})
	if err != nil {
		t.Fatalf("NewRunner failed: %v", err)
	}

	suite := &TestSuite{
		Name:      "negate",
		Blueprint: "acme/demo",
	}

	test := &TestCase{
		Name: "negated-assertion",
		Assertions: []Assertion{
			{
				Type:   AssertOutputEquals,
				Negate: true,
				Output: &OutputAssertion{
					Name:  "missing",
					Value: "x",
				},
			},
			{
				Type:   AssertionType("unknown"),
				Negate: true,
			},
		},
	}

	result := runner.RunTest(context.Background(), test, suite)
	if result.Status != StatusPassed {
		t.Fatalf("Expected test to pass, got %s (assertions: %+v)", result.Status, result.AssertionResults)
	}
	if len(result.AssertionResults) != 2 {
		t.Fatalf("Expected 2 assertion results, got %d", len(result.AssertionResults))
	}
	if !result.AssertionResults[0].Passed || !result.AssertionResults[1].Passed {
		t.Fatal("Expected negated assertions to pass")
	}
}

func TestRunner_EvaluateAssertion_Negate(t *testing.T) {
	runner := &Runner{}
	execResult := &ExecutionResult{Outputs: map[string]interface{}{}}

	assertion := &Assertion{
		Type:   AssertOutputEquals,
		Negate: true,
		Output: &OutputAssertion{
			Name:  "missing",
			Value: "x",
		},
	}

	result := runner.evaluateAssertion(context.Background(), assertion, execResult, &TestCase{})
	if !result.Passed {
		t.Fatalf("Expected negated assertion to pass, got %+v", result)
	}
}

func setupTestBlueprint(t *testing.T) string {
	t.Helper()
	tmpDir := t.TempDir()
	blueprintsDir := filepath.Join(tmpDir, "blueprints")
	bpDir := filepath.Join(blueprintsDir, "acme", "demo")
	statesDir := filepath.Join(bpDir, "states")

	if err := os.MkdirAll(statesDir, 0755); err != nil {
		t.Fatalf("Failed to create blueprint dirs: %v", err)
	}

	manifest := `
apiVersion: blueprints.keystone-core.io/v1
kind: Blueprint
metadata:
  name: demo
  version: 1.0.0
entrypoints:
  default: states/main.yaml
`
	if err := os.WriteFile(filepath.Join(bpDir, "blueprint.yaml"), []byte(manifest), 0644); err != nil {
		t.Fatalf("Failed to write blueprint.yaml: %v", err)
	}

	state := `
file:
  /tmp/demo.txt:
    state: present
    contents: "demo"
`
	if err := os.WriteFile(filepath.Join(statesDir, "main.yaml"), []byte(state), 0644); err != nil {
		t.Fatalf("Failed to write main.yaml: %v", err)
	}

	return blueprintsDir
}
