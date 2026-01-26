package testing

import (
	"testing"
	"time"
)

func TestTestSuiteResult_IsPassing(t *testing.T) {
	tests := []struct {
		name   string
		result TestSuiteResult
		want   bool
	}{
		{
			name: "all passed",
			result: TestSuiteResult{
				Summary: TestSummary{
					Total:  3,
					Passed: 3,
				},
			},
			want: true,
		},
		{
			name: "with failures",
			result: TestSuiteResult{
				Summary: TestSummary{
					Total:  3,
					Passed: 2,
					Failed: 1,
				},
			},
			want: false,
		},
		{
			name: "with errors",
			result: TestSuiteResult{
				Summary: TestSummary{
					Total:  3,
					Passed: 2,
					Errors: 1,
				},
			},
			want: false,
		},
		{
			name: "with suite error",
			result: TestSuiteResult{
				Summary: TestSummary{
					Total:  3,
					Passed: 3,
				},
				Error: "suite failed to initialize",
			},
			want: false,
		},
		{
			name: "all skipped",
			result: TestSuiteResult{
				Summary: TestSummary{
					Total:   3,
					Skipped: 3,
				},
			},
			want: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.result.IsPassing()
			if got != tt.want {
				t.Errorf("IsPassing() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestTestSuiteResult_CalculateSummary(t *testing.T) {
	result := &TestSuiteResult{
		Tests: []TestResult{
			{Name: "test1", Status: StatusPassed, Duration: 100 * time.Millisecond},
			{Name: "test2", Status: StatusPassed, Duration: 200 * time.Millisecond},
			{Name: "test3", Status: StatusFailed, Duration: 150 * time.Millisecond},
			{Name: "test4", Status: StatusSkipped, Duration: 0},
			{Name: "test5", Status: StatusError, Duration: 50 * time.Millisecond},
		},
	}

	result.CalculateSummary()

	if result.Summary.Total != 5 {
		t.Errorf("Total = %d, want 5", result.Summary.Total)
	}
	if result.Summary.Passed != 2 {
		t.Errorf("Passed = %d, want 2", result.Summary.Passed)
	}
	if result.Summary.Failed != 1 {
		t.Errorf("Failed = %d, want 1", result.Summary.Failed)
	}
	if result.Summary.Skipped != 1 {
		t.Errorf("Skipped = %d, want 1", result.Summary.Skipped)
	}
	if result.Summary.Errors != 1 {
		t.Errorf("Errors = %d, want 1", result.Summary.Errors)
	}

	// Pass rate should be 50% (2 passed out of 4 non-skipped)
	expectedPassRate := 50.0
	if result.Summary.PassRate != expectedPassRate {
		t.Errorf("PassRate = %f, want %f", result.Summary.PassRate, expectedPassRate)
	}

	// Check total duration
	expectedDuration := 500 * time.Millisecond
	if result.Summary.TotalDuration != expectedDuration {
		t.Errorf("TotalDuration = %v, want %v", result.Summary.TotalDuration, expectedDuration)
	}

	// Check failed test names
	if len(result.Summary.FailedTests) != 1 || result.Summary.FailedTests[0] != "test3" {
		t.Errorf("FailedTests = %v, want [test3]", result.Summary.FailedTests)
	}

	// Check skipped test names
	if len(result.Summary.SkippedTests) != 1 || result.Summary.SkippedTests[0] != "test4" {
		t.Errorf("SkippedTests = %v, want [test4]", result.Summary.SkippedTests)
	}

	// Check error test names
	if len(result.Summary.ErrorTests) != 1 || result.Summary.ErrorTests[0] != "test5" {
		t.Errorf("ErrorTests = %v, want [test5]", result.Summary.ErrorTests)
	}
}

func TestTestSuiteResult_CalculateSummary_AllSkipped(t *testing.T) {
	result := &TestSuiteResult{
		Tests: []TestResult{
			{Name: "test1", Status: StatusSkipped},
			{Name: "test2", Status: StatusSkipped},
		},
	}

	result.CalculateSummary()

	// Pass rate should be 0 when all tests are skipped (no runnable tests)
	if result.Summary.PassRate != 0 {
		t.Errorf("PassRate = %f, want 0 (all skipped)", result.Summary.PassRate)
	}
}

func TestTestSuiteResult_CalculateSummary_Empty(t *testing.T) {
	result := &TestSuiteResult{
		Tests: []TestResult{},
	}

	result.CalculateSummary()

	if result.Summary.Total != 0 {
		t.Errorf("Total = %d, want 0", result.Summary.Total)
	}
	if result.Summary.PassRate != 0 {
		t.Errorf("PassRate = %f, want 0", result.Summary.PassRate)
	}
}

func TestNewTestResult(t *testing.T) {
	testCase := &TestCase{
		Name:        "my_test",
		Description: "Test description",
	}

	result := NewTestResult(testCase)
	if result == nil {
		t.Fatal("NewTestResult returned nil")
	}
	if result.Name != "my_test" {
		t.Errorf("Name = %q, want %q", result.Name, "my_test")
	}
	if result.Description != "Test description" {
		t.Errorf("Description = %q, want %q", result.Description, "Test description")
	}
	if result.Status != StatusPending {
		t.Errorf("Status = %q, want %q", result.Status, StatusPending)
	}
}

func TestTestResult_Pass(t *testing.T) {
	result := &TestResult{Status: StatusPending}
	result.Pass()

	if result.Status != StatusPassed {
		t.Errorf("Status = %q, want %q", result.Status, StatusPassed)
	}
}

func TestTestResult_Fail(t *testing.T) {
	result := &TestResult{Status: StatusPending}
	result.Fail("assertion failed")

	if result.Status != StatusFailed {
		t.Errorf("Status = %q, want %q", result.Status, StatusFailed)
	}
	if result.Error != "assertion failed" {
		t.Errorf("Error = %q, want %q", result.Error, "assertion failed")
	}
}

func TestTestResult_Skip(t *testing.T) {
	result := &TestResult{Status: StatusPending}
	result.Skip("not implemented")

	if result.Status != StatusSkipped {
		t.Errorf("Status = %q, want %q", result.Status, StatusSkipped)
	}
	if result.SkipReason != "not implemented" {
		t.Errorf("SkipReason = %q, want %q", result.SkipReason, "not implemented")
	}
}

func TestTestResult_SetError(t *testing.T) {
	result := &TestResult{Status: StatusPending}
	result.SetError("unexpected error")

	if result.Status != StatusError {
		t.Errorf("Status = %q, want %q", result.Status, StatusError)
	}
	if result.Error != "unexpected error" {
		t.Errorf("Error = %q, want %q", result.Error, "unexpected error")
	}
}

func TestTestResult_AllAssertionsPassed(t *testing.T) {
	tests := []struct {
		name       string
		assertions []AssertionResult
		want       bool
	}{
		{
			name:       "no assertions",
			assertions: []AssertionResult{},
			want:       true,
		},
		{
			name: "all passed",
			assertions: []AssertionResult{
				{Passed: true},
				{Passed: true},
				{Passed: true},
			},
			want: true,
		},
		{
			name: "one failed",
			assertions: []AssertionResult{
				{Passed: true},
				{Passed: false},
				{Passed: true},
			},
			want: false,
		},
		{
			name: "all failed",
			assertions: []AssertionResult{
				{Passed: false},
				{Passed: false},
			},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := &TestResult{AssertionResults: tt.assertions}
			got := result.AllAssertionsPassed()
			if got != tt.want {
				t.Errorf("AllAssertionsPassed() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestTestResult_FailedAssertions(t *testing.T) {
	result := &TestResult{
		AssertionResults: []AssertionResult{
			{Type: AssertNoFailures, Passed: true, Message: "ok"},
			{Type: AssertFileExists, Passed: false, Message: "file not found"},
			{Type: AssertStateApplied, Passed: true, Message: "applied"},
			{Type: AssertCommandSuccess, Passed: false, Message: "exit code 1"},
		},
	}

	failed := result.FailedAssertions()
	if len(failed) != 2 {
		t.Errorf("len(FailedAssertions) = %d, want 2", len(failed))
	}

	if failed[0].Type != AssertFileExists {
		t.Errorf("failed[0].Type = %q, want %q", failed[0].Type, AssertFileExists)
	}
	if failed[1].Type != AssertCommandSuccess {
		t.Errorf("failed[1].Type = %q, want %q", failed[1].Type, AssertCommandSuccess)
	}
}

func TestTestResult_FailedAssertions_None(t *testing.T) {
	result := &TestResult{
		AssertionResults: []AssertionResult{
			{Passed: true},
			{Passed: true},
		},
	}

	failed := result.FailedAssertions()
	if len(failed) != 0 {
		t.Errorf("len(FailedAssertions) = %d, want 0", len(failed))
	}
}

func TestTestStatus_Values(t *testing.T) {
	// Verify status constants have expected values
	statuses := map[TestStatus]string{
		StatusPassed:  "passed",
		StatusFailed:  "failed",
		StatusSkipped: "skipped",
		StatusError:   "error",
		StatusRunning: "running",
		StatusPending: "pending",
	}

	for status, expected := range statuses {
		if string(status) != expected {
			t.Errorf("Status %v = %q, want %q", status, string(status), expected)
		}
	}
}

func TestSetupTeardownResult_Structure(t *testing.T) {
	result := SetupTeardownResult{
		Success:  true,
		Duration: 500 * time.Millisecond,
		CommandResults: []CommandResult{
			{Command: "echo hello", ExitCode: 0, Stdout: "hello"},
		},
		StateResults: []StateResult{
			{ID: "state1", Success: true},
		},
		FilesCreated: []string{"/tmp/test1", "/tmp/test2"},
	}

	if !result.Success {
		t.Error("Success should be true")
	}
	if len(result.CommandResults) != 1 {
		t.Errorf("len(CommandResults) = %d, want 1", len(result.CommandResults))
	}
	if len(result.StateResults) != 1 {
		t.Errorf("len(StateResults) = %d, want 1", len(result.StateResults))
	}
	if len(result.FilesCreated) != 2 {
		t.Errorf("len(FilesCreated) = %d, want 2", len(result.FilesCreated))
	}
}

func TestExecutionResult_Structure(t *testing.T) {
	result := ExecutionResult{
		Success:         true,
		DryRun:          false,
		Duration:        2 * time.Second,
		StatesApplied:   10,
		StatesChanged:   5,
		StatesFailed:    0,
		StatesUnchanged: 5,
		Outputs: map[string]interface{}{
			"port": 8080,
			"host": "localhost",
		},
	}

	if !result.Success {
		t.Error("Success should be true")
	}
	if result.StatesApplied != 10 {
		t.Errorf("StatesApplied = %d, want 10", result.StatesApplied)
	}
	if result.StatesChanged != 5 {
		t.Errorf("StatesChanged = %d, want 5", result.StatesChanged)
	}
	if result.Outputs["port"] != 8080 {
		t.Errorf("Outputs[port] = %v, want 8080", result.Outputs["port"])
	}
}

func TestAssertionResult_Structure(t *testing.T) {
	result := AssertionResult{
		Type:        AssertFileExists,
		Description: "Check config file exists",
		Passed:      false,
		Target:      "/etc/app/config.yaml",
		Expected:    true,
		Actual:      false,
		Message:     "File not found",
		Duration:    10 * time.Millisecond,
	}

	if result.Type != AssertFileExists {
		t.Errorf("Type = %q, want %q", result.Type, AssertFileExists)
	}
	if result.Passed {
		t.Error("Passed should be false")
	}
	if result.Target != "/etc/app/config.yaml" {
		t.Errorf("Target = %q, want %q", result.Target, "/etc/app/config.yaml")
	}
}

func TestStateResult_Structure(t *testing.T) {
	result := StateResult{
		ID:       "install_nginx",
		Module:   "package",
		Success:  true,
		Changed:  true,
		Duration: 5 * time.Second,
		Comment:  "Package nginx installed successfully",
	}

	if result.ID != "install_nginx" {
		t.Errorf("ID = %q, want %q", result.ID, "install_nginx")
	}
	if result.Module != "package" {
		t.Errorf("Module = %q, want %q", result.Module, "package")
	}
	if !result.Changed {
		t.Error("Changed should be true")
	}
}

func TestCommandResult_Structure(t *testing.T) {
	result := CommandResult{
		Command:  "ls -la /tmp",
		ExitCode: 0,
		Stdout:   "total 0\ndrwxrwxrwt 2 root root 40 Jan 1 00:00 .",
		Stderr:   "",
		Duration: 50 * time.Millisecond,
	}

	if result.Command != "ls -la /tmp" {
		t.Errorf("Command = %q, want %q", result.Command, "ls -la /tmp")
	}
	if result.ExitCode != 0 {
		t.Errorf("ExitCode = %d, want 0", result.ExitCode)
	}
}
