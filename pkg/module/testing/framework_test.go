package testing

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	stdtesting "testing"
	"time"
)

func TestDefaultConfig(t *stdtesting.T) {
	config := DefaultConfig()
	if config.Timeout != 30*time.Second {
		t.Errorf("Timeout = %v, want 30s", config.Timeout)
	}
	if config.MaxParallel != 4 {
		t.Errorf("MaxParallel = %d, want 4", config.MaxParallel)
	}
	if !config.Parallel {
		t.Error("Parallel should be true by default")
	}
}

func TestNewFramework(t *stdtesting.T) {
	f := NewFramework(nil)
	if f.Config == nil {
		t.Error("Config should not be nil")
	}
	if len(f.reporters) != 1 {
		t.Errorf("Expected 1 reporter, got %d", len(f.reporters))
	}
}

func TestFramework_RegisterMock(t *stdtesting.T) {
	f := NewFramework(nil)

	mockCalled := false
	f.RegisterMock("test_mock", func(args ...interface{}) (interface{}, error) {
		mockCalled = true
		return "result", nil
	})

	fn, ok := f.GetMock("test_mock")
	if !ok {
		t.Fatal("Mock not found")
	}

	result, err := fn()
	if err != nil {
		t.Errorf("Mock returned error: %v", err)
	}
	if result != "result" {
		t.Errorf("Mock result = %v, want 'result'", result)
	}
	if !mockCalled {
		t.Error("Mock was not called")
	}
}

func TestFramework_LoadFixture(t *stdtesting.T) {
	f := NewFramework(nil)

	// Create temp fixture file
	dir := t.TempDir()
	path := filepath.Join(dir, "fixture.json")
	os.WriteFile(path, []byte(`{"key": "value"}`), 0644)

	err := f.LoadFixture("test_fixture", path)
	if err != nil {
		t.Fatalf("LoadFixture failed: %v", err)
	}

	fixture, ok := f.GetFixture("test_fixture")
	if !ok {
		t.Fatal("Fixture not found")
	}

	if fixture == nil {
		t.Error("Fixture is nil")
	}
}

func TestFramework_Run_Passing(t *stdtesting.T) {
	config := DefaultConfig()
	config.Parallel = false // Make test deterministic
	f := NewFramework(config)

	// Replace reporter to suppress output
	f.reporters = []Reporter{}

	tests := []*TestCase{
		{
			Name: "test_pass",
			Run: func(t *T) error {
				return nil
			},
		},
	}

	result := f.Run("test_suite", tests)

	if result.Passed != 1 {
		t.Errorf("Passed = %d, want 1", result.Passed)
	}
	if result.Failed != 0 {
		t.Errorf("Failed = %d, want 0", result.Failed)
	}
	if result.Total != 1 {
		t.Errorf("Total = %d, want 1", result.Total)
	}
}

func TestFramework_Run_Failing(t *stdtesting.T) {
	config := DefaultConfig()
	config.Parallel = false
	f := NewFramework(config)
	f.reporters = []Reporter{}

	tests := []*TestCase{
		{
			Name: "test_fail",
			Run: func(t *T) error {
				return errors.New("test error")
			},
		},
	}

	result := f.Run("test_suite", tests)

	if result.Passed != 0 {
		t.Errorf("Passed = %d, want 0", result.Passed)
	}
	if result.Failed != 1 {
		t.Errorf("Failed = %d, want 1", result.Failed)
	}
}

func TestFramework_Run_Skipped(t *stdtesting.T) {
	config := DefaultConfig()
	config.Parallel = false
	f := NewFramework(config)
	f.reporters = []Reporter{}

	tests := []*TestCase{
		{
			Name: "test_skip",
			Skip: true,
			Run: func(t *T) error {
				return nil
			},
		},
	}

	result := f.Run("test_suite", tests)

	if result.Skipped != 1 {
		t.Errorf("Skipped = %d, want 1", result.Skipped)
	}
}

func TestFramework_Run_SetupTeardown(t *stdtesting.T) {
	config := DefaultConfig()
	config.Parallel = false
	f := NewFramework(config)
	f.reporters = []Reporter{}

	setupCalled := false
	teardownCalled := false
	testCalled := false

	tests := []*TestCase{
		{
			Name: "test_with_setup",
			Setup: func(t *T) error {
				setupCalled = true
				return nil
			},
			Teardown: func(t *T) error {
				teardownCalled = true
				return nil
			},
			Run: func(t *T) error {
				testCalled = true
				return nil
			},
		},
	}

	f.Run("test_suite", tests)

	if !setupCalled {
		t.Error("Setup was not called")
	}
	if !testCalled {
		t.Error("Test was not called")
	}
	if !teardownCalled {
		t.Error("Teardown was not called")
	}
}

func TestFramework_Run_Timeout(t *stdtesting.T) {
	config := DefaultConfig()
	config.Parallel = false
	config.Timeout = 100 * time.Millisecond
	f := NewFramework(config)
	f.reporters = []Reporter{}

	tests := []*TestCase{
		{
			Name: "test_timeout",
			Run: func(t *T) error {
				time.Sleep(500 * time.Millisecond)
				return nil
			},
		},
	}

	result := f.Run("test_suite", tests)

	if result.Errors != 1 {
		t.Errorf("Errors = %d, want 1", result.Errors)
	}
	if result.Tests[0].Status != TestStatusError {
		t.Errorf("Status = %v, want %v", result.Tests[0].Status, TestStatusError)
	}
}

func TestFramework_Run_FailFast(t *stdtesting.T) {
	config := DefaultConfig()
	config.Parallel = false
	config.FailFast = true
	f := NewFramework(config)
	f.reporters = []Reporter{}

	secondTestRan := false

	tests := []*TestCase{
		{
			Name: "test_fail_first",
			Run: func(t *T) error {
				return errors.New("fail")
			},
		},
		{
			Name: "test_second",
			Run: func(t *T) error {
				secondTestRan = true
				return nil
			},
		},
	}

	result := f.Run("test_suite", tests)

	if secondTestRan {
		t.Error("Second test should not have run due to FailFast")
	}
	if result.Failed != 1 {
		t.Errorf("Failed = %d, want 1", result.Failed)
	}
	if result.Skipped != 1 {
		t.Errorf("Skipped = %d, want 1", result.Skipped)
	}
}

func TestFramework_FilterTests_Pattern(t *stdtesting.T) {
	config := DefaultConfig()
	config.Pattern = "test_a.*"
	f := NewFramework(config)

	tests := []*TestCase{
		{Name: "test_alpha", Run: func(t *T) error { return nil }},
		{Name: "test_beta", Run: func(t *T) error { return nil }},
		{Name: "test_abc", Run: func(t *T) error { return nil }},
	}

	filtered := f.filterTests(tests)

	if len(filtered) != 2 {
		t.Errorf("Expected 2 tests, got %d", len(filtered))
	}
}

func TestFramework_FilterTests_Tags(t *stdtesting.T) {
	config := DefaultConfig()
	config.Tags = []string{"important"}
	f := NewFramework(config)

	tests := []*TestCase{
		{Name: "test1", Tags: []string{"important"}, Run: func(t *T) error { return nil }},
		{Name: "test2", Tags: []string{"optional"}, Run: func(t *T) error { return nil }},
		{Name: "test3", Tags: []string{"important", "slow"}, Run: func(t *T) error { return nil }},
	}

	filtered := f.filterTests(tests)

	if len(filtered) != 2 {
		t.Errorf("Expected 2 tests, got %d", len(filtered))
	}
}

func TestFramework_FilterTests_SkipTags(t *stdtesting.T) {
	config := DefaultConfig()
	config.SkipTags = []string{"slow"}
	f := NewFramework(config)

	tests := []*TestCase{
		{Name: "test1", Tags: []string{"fast"}, Run: func(t *T) error { return nil }},
		{Name: "test2", Tags: []string{"slow"}, Run: func(t *T) error { return nil }},
	}

	filtered := f.filterTests(tests)

	if len(filtered) != 1 {
		t.Errorf("Expected 1 test, got %d", len(filtered))
	}
}

func TestT_Methods(t *stdtesting.T) {
	ft := &T{
		Name:    "test",
		context: make(map[string]interface{}),
	}

	ft.Log("test log")
	if len(ft.logs) != 1 {
		t.Errorf("Expected 1 log, got %d", len(ft.logs))
	}

	ft.Logf("formatted %s", "log")
	if len(ft.logs) != 2 {
		t.Errorf("Expected 2 logs, got %d", len(ft.logs))
	}

	ft.Error("test error")
	if !ft.Failed() {
		t.Error("Expected test to be failed")
	}
	if len(ft.errors) != 1 {
		t.Errorf("Expected 1 error, got %d", len(ft.errors))
	}

	ft.Skip("skip reason")
	if !ft.Skipped() {
		t.Error("Expected test to be skipped")
	}
}

func TestT_Context(t *stdtesting.T) {
	ft := &T{
		Name:    "test",
		context: make(map[string]interface{}),
	}

	ctx := ft.Context()
	ctx["key"] = "value"

	if ft.context["key"] != "value" {
		t.Error("Context not properly shared")
	}
}

func TestAssertions(t *stdtesting.T) {
	t.Run("AssertEqual_Pass", func(t *stdtesting.T) {
		ft := &T{Name: "test", context: make(map[string]interface{})}
		AssertEqual(ft, 1, 1)
		if ft.Failed() {
			t.Error("AssertEqual should pass for equal values")
		}
	})

	t.Run("AssertEqual_Fail", func(t *stdtesting.T) {
		ft := &T{Name: "test", context: make(map[string]interface{})}
		AssertEqual(ft, 1, 2)
		if !ft.Failed() {
			t.Error("AssertEqual should fail for unequal values")
		}
	})

	t.Run("AssertNotEqual_Pass", func(t *stdtesting.T) {
		ft := &T{Name: "test", context: make(map[string]interface{})}
		AssertNotEqual(ft, 1, 2)
		if ft.Failed() {
			t.Error("AssertNotEqual should pass for unequal values")
		}
	})

	t.Run("AssertTrue_Pass", func(t *stdtesting.T) {
		ft := &T{Name: "test", context: make(map[string]interface{})}
		AssertTrue(ft, true)
		if ft.Failed() {
			t.Error("AssertTrue should pass for true")
		}
	})

	t.Run("AssertFalse_Pass", func(t *stdtesting.T) {
		ft := &T{Name: "test", context: make(map[string]interface{})}
		AssertFalse(ft, false)
		if ft.Failed() {
			t.Error("AssertFalse should pass for false")
		}
	})

	t.Run("AssertContains_Pass", func(t *stdtesting.T) {
		ft := &T{Name: "test", context: make(map[string]interface{})}
		AssertContains(ft, "hello world", "world")
		if ft.Failed() {
			t.Error("AssertContains should pass when substring exists")
		}
	})

	t.Run("AssertNoError_Pass", func(t *stdtesting.T) {
		ft := &T{Name: "test", context: make(map[string]interface{})}
		AssertNoError(ft, nil)
		if ft.Failed() {
			t.Error("AssertNoError should pass for nil error")
		}
	})

	t.Run("AssertNoError_Fail", func(t *stdtesting.T) {
		ft := &T{Name: "test", context: make(map[string]interface{})}
		AssertNoError(ft, errors.New("test error"))
		if !ft.Failed() {
			t.Error("AssertNoError should fail for non-nil error")
		}
	})

	t.Run("AssertError_Pass", func(t *stdtesting.T) {
		ft := &T{Name: "test", context: make(map[string]interface{})}
		AssertError(ft, errors.New("test error"))
		if ft.Failed() {
			t.Error("AssertError should pass for non-nil error")
		}
	})
}

func TestConsoleReporter(t *stdtesting.T) {
	// Just ensure it doesn't panic
	r := &ConsoleReporter{Verbose: true}
	r.OnSuiteStart("test")
	r.OnTestStart("test")
	r.OnTestComplete(&TestResult{Name: "test", Status: TestStatusPassed})
	r.OnTestComplete(&TestResult{Name: "test", Status: TestStatusFailed, Error: "error"})
	r.OnTestComplete(&TestResult{Name: "test", Status: TestStatusSkipped})
	r.OnTestComplete(&TestResult{Name: "test", Status: TestStatusError, Error: "error"})
	r.OnSuiteComplete(&TestSuiteResult{Passed: 1, Failed: 1})
}

func TestJUnitReporter(t *stdtesting.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "results.xml")

	r := &JUnitReporter{OutputPath: path}
	r.OnSuiteComplete(&TestSuiteResult{
		Name: "test_suite",
		Tests: []*TestResult{
			{Name: "test1", Status: TestStatusPassed, Duration: time.Second},
			{Name: "test2", Status: TestStatusFailed, Error: "error", Duration: time.Second},
			{Name: "test3", Status: TestStatusSkipped, Duration: time.Second},
		},
		Total:   3,
		Passed:  1,
		Failed:  1,
		Skipped: 1,
	})

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("Failed to read output: %v", err)
	}

	content := string(data)
	if !strings.Contains(content, "testsuite") {
		t.Error("Expected testsuite in output")
	}
	if !strings.Contains(content, "testcase") {
		t.Error("Expected testcase in output")
	}
}

func TestJSONReporter(t *stdtesting.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "results.json")

	r := &JSONReporter{OutputPath: path}
	r.OnSuiteComplete(&TestSuiteResult{
		Name:   "test_suite",
		Passed: 1,
	})

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("Failed to read output: %v", err)
	}

	if !strings.Contains(string(data), "test_suite") {
		t.Error("Expected test_suite in output")
	}
}

func TestDiscoverTests(t *stdtesting.T) {
	dir := t.TempDir()

	// Create some test files
	os.WriteFile(filepath.Join(dir, "foo_test.go"), []byte("test"), 0644)
	os.WriteFile(filepath.Join(dir, "bar_test.go"), []byte("test"), 0644)
	os.WriteFile(filepath.Join(dir, "other.go"), []byte("test"), 0644)

	files, err := DiscoverTests(dir, "*_test.go")
	if err != nil {
		t.Fatalf("DiscoverTests failed: %v", err)
	}

	if len(files) != 2 {
		t.Errorf("Expected 2 files, got %d", len(files))
	}
}

func TestHasAnyTag(t *stdtesting.T) {
	tests := []struct {
		testTags   []string
		filterTags []string
		expected   bool
	}{
		{[]string{"a", "b"}, []string{"a"}, true},
		{[]string{"a", "b"}, []string{"c"}, false},
		{[]string{"a", "b"}, []string{"b", "c"}, true},
		{[]string{}, []string{"a"}, false},
		{[]string{"a"}, []string{}, false},
	}

	for _, tt := range tests {
		result := hasAnyTag(tt.testTags, tt.filterTags)
		if result != tt.expected {
			t.Errorf("hasAnyTag(%v, %v) = %v, want %v",
				tt.testTags, tt.filterTags, result, tt.expected)
		}
	}
}

func TestEscapeXML(t *stdtesting.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"hello", "hello"},
		{"<test>", "&lt;test&gt;"},
		{"a & b", "a &amp; b"},
		{`"quoted"`, "&quot;quoted&quot;"},
	}

	for _, tt := range tests {
		result := escapeXML(tt.input)
		if result != tt.expected {
			t.Errorf("escapeXML(%q) = %q, want %q", tt.input, result, tt.expected)
		}
	}
}

func TestFramework_Run_Parallel(t *stdtesting.T) {
	config := DefaultConfig()
	config.Parallel = true
	config.MaxParallel = 2
	f := NewFramework(config)
	f.reporters = []Reporter{}

	tests := []*TestCase{
		{
			Name:     "test1",
			Parallel: true,
			Run: func(t *T) error {
				time.Sleep(10 * time.Millisecond)
				return nil
			},
		},
		{
			Name:     "test2",
			Parallel: true,
			Run: func(t *T) error {
				time.Sleep(10 * time.Millisecond)
				return nil
			},
		},
	}

	result := f.Run("test_suite", tests)

	if result.Total != 2 {
		t.Errorf("Total = %d, want 2", result.Total)
	}
	if result.Passed != 2 {
		t.Errorf("Passed = %d, want 2", result.Passed)
	}
}
