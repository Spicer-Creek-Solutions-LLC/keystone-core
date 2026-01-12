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
		{"", "", true}, // empty pattern matches empty string
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
