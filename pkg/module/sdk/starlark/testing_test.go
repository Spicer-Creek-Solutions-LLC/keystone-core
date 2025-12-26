package starlark

import (
	"os"
	"path/filepath"
	"testing"
)

func TestTestRunner_RunTestFile(t *testing.T) {
	tmpDir := t.TempDir()

	// Create a test file
	testFile := filepath.Join(tmpDir, "example_test.star")
	testContent := `
def test_passing():
    assert.eq(1 + 1, 2)

def test_string():
    assert.eq("hello", "hello")

def test_true():
    assert.true(True)

def test_false():
    assert.false(False)
`
	if err := os.WriteFile(testFile, []byte(testContent), 0644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	runner := NewTestRunner()
	suite, err := runner.RunTestFile(testFile)
	if err != nil {
		t.Fatalf("RunTestFile() error = %v", err)
	}

	if suite.Total != 4 {
		t.Errorf("Total = %d, want 4", suite.Total)
	}

	if suite.Passed != 4 {
		t.Errorf("Passed = %d, want 4", suite.Passed)
	}

	if suite.Failed != 0 {
		t.Errorf("Failed = %d, want 0", suite.Failed)
	}
}

func TestTestRunner_RunTestFile_Failures(t *testing.T) {
	tmpDir := t.TempDir()

	// Create a test file with failures
	testFile := filepath.Join(tmpDir, "failing_test.star")
	testContent := `
def test_failing_eq():
    assert.eq(1, 2)

def test_failing_true():
    assert.true(False)
`
	if err := os.WriteFile(testFile, []byte(testContent), 0644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	runner := NewTestRunner()
	suite, err := runner.RunTestFile(testFile)
	if err != nil {
		t.Fatalf("RunTestFile() error = %v", err)
	}

	if suite.Total != 2 {
		t.Errorf("Total = %d, want 2", suite.Total)
	}

	if suite.Passed != 0 {
		t.Errorf("Passed = %d, want 0", suite.Passed)
	}

	if suite.Failed != 2 {
		t.Errorf("Failed = %d, want 2", suite.Failed)
	}

	for _, test := range suite.Tests {
		if test.Passed {
			t.Errorf("Test %s should have failed", test.Name)
		}
		if test.Error == "" {
			t.Errorf("Test %s should have error message", test.Name)
		}
	}
}

func TestTestRunner_RunTestsInDir(t *testing.T) {
	tmpDir := t.TempDir()

	// Create multiple test files
	testFiles := map[string]string{
		"test1_test.star": `
def test_one():
    assert.eq(1, 1)
`,
		"test2_test.star": `
def test_two():
    assert.eq(2, 2)

def test_three():
    assert.eq(3, 3)
`,
	}

	for name, content := range testFiles {
		path := filepath.Join(tmpDir, name)
		if err := os.WriteFile(path, []byte(content), 0644); err != nil {
			t.Fatalf("Failed to create test file %s: %v", name, err)
		}
	}

	runner := NewTestRunner()
	suites, err := runner.RunTestsInDir(tmpDir)
	if err != nil {
		t.Fatalf("RunTestsInDir() error = %v", err)
	}

	if len(suites) != 2 {
		t.Errorf("Suites count = %d, want 2", len(suites))
	}

	totalTests := 0
	totalPassed := 0
	for _, suite := range suites {
		totalTests += suite.Total
		totalPassed += suite.Passed
	}

	if totalTests != 3 {
		t.Errorf("Total tests = %d, want 3", totalTests)
	}

	if totalPassed != 3 {
		t.Errorf("Total passed = %d, want 3", totalPassed)
	}
}

func TestAssertContains(t *testing.T) {
	tmpDir := t.TempDir()

	testFile := filepath.Join(tmpDir, "contains_test.star")
	testContent := `
def test_list_contains():
    assert.contains([1, 2, 3], 2)

def test_string_contains():
    assert.contains("hello world", "world")
`
	if err := os.WriteFile(testFile, []byte(testContent), 0644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	runner := NewTestRunner()
	suite, err := runner.RunTestFile(testFile)
	if err != nil {
		t.Fatalf("RunTestFile() error = %v", err)
	}

	if suite.Passed != 2 {
		t.Errorf("Passed = %d, want 2", suite.Passed)
	}
}

func TestFormatResults(t *testing.T) {
	suites := []*TestSuite{
		{
			File: "test1.star",
			Tests: []*TestResult{
				{Name: "test_pass", Passed: true},
				{Name: "test_fail", Passed: false, Error: "assertion failed"},
			},
			Passed: 1,
			Failed: 1,
			Total:  2,
		},
	}

	result := FormatResults(suites)

	expectedSubstrings := []string{
		"test1.star",
		"✓ test_pass",
		"✗ test_fail",
		"Error: assertion failed",
		"Total: 2 tests, 1 passed, 1 failed",
	}

	for _, expected := range expectedSubstrings {
		if !contains(result, expected) {
			t.Errorf("FormatResults missing expected substring: %s", expected)
		}
	}
}
