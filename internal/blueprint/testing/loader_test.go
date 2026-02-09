package testing

import (
	"os"
	"path/filepath"
	"testing"
)

func TestNewTestLoader(t *testing.T) {
	loader := NewTestLoader("/test/path")
	if loader == nil {
		t.Fatal("NewTestLoader returned nil")
	}
	if loader.BasePath != "/test/path" {
		t.Errorf("BasePath = %q, want %q", loader.BasePath, "/test/path")
	}
}

func TestTestLoader_LoadFile(t *testing.T) {
	// Create a temporary directory and test file
	tempDir := t.TempDir()

	testContent := `
name: Basic Tests
description: Test basic blueprint functionality
tests:
  - name: test_one
    assertions:
      - type: no_failures
  - name: test_two
    assertions:
      - type: no_failures
`
	testFile := filepath.Join(tempDir, "basic_test.yaml")
	if err := os.WriteFile(testFile, []byte(testContent), 0644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	loader := NewTestLoader(tempDir)
	suite, err := loader.LoadFile(testFile)
	if err != nil {
		t.Fatalf("LoadFile failed: %v", err)
	}

	if suite.Name != "Basic Tests" {
		t.Errorf("Name = %q, want %q", suite.Name, "Basic Tests")
	}
	if suite.Description != "Test basic blueprint functionality" {
		t.Errorf("Description = %q, want %q", suite.Description, "Test basic blueprint functionality")
	}
	if len(suite.Tests) != 2 {
		t.Errorf("len(Tests) = %d, want 2", len(suite.Tests))
	}
}

func TestTestLoader_LoadFile_NonExistent(t *testing.T) {
	loader := NewTestLoader("/tmp")
	_, err := loader.LoadFile("/nonexistent/file.yaml")
	if err == nil {
		t.Error("LoadFile should fail for nonexistent file")
	}
}

func TestTestLoader_LoadFile_InvalidYAML(t *testing.T) {
	tempDir := t.TempDir()
	testFile := filepath.Join(tempDir, "invalid.yaml")
	if err := os.WriteFile(testFile, []byte("this is not: valid: yaml:"), 0644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	loader := NewTestLoader(tempDir)
	_, err := loader.LoadFile(testFile)
	if err == nil {
		t.Error("LoadFile should fail for invalid YAML")
	}
}

func TestTestLoader_LoadDirectory(t *testing.T) {
	tempDir := t.TempDir()

	// Create multiple test files
	test1Content := `
name: Suite One
tests:
  - name: test_a
    assertions:
      - type: no_failures
`
	test2Content := `
name: Suite Two
tests:
  - name: test_b
    assertions:
      - type: no_failures
`

	if err := os.WriteFile(filepath.Join(tempDir, "suite1_test.yaml"), []byte(test1Content), 0644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}
	if err := os.WriteFile(filepath.Join(tempDir, "suite2_test.yml"), []byte(test2Content), 0644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}
	// Create a non-test file that should be ignored
	if err := os.WriteFile(filepath.Join(tempDir, "readme.txt"), []byte("not a test"), 0644); err != nil {
		t.Fatalf("Failed to create readme file: %v", err)
	}

	loader := NewTestLoader(tempDir)
	suites, err := loader.LoadDirectory(".")
	if err != nil {
		t.Fatalf("LoadDirectory failed: %v", err)
	}

	if len(suites) != 2 {
		t.Errorf("len(suites) = %d, want 2", len(suites))
	}

	// Suites should be sorted by name
	if len(suites) >= 2 {
		if suites[0].Name != "Suite One" {
			t.Errorf("suites[0].Name = %q, want %q", suites[0].Name, "Suite One")
		}
		if suites[1].Name != "Suite Two" {
			t.Errorf("suites[1].Name = %q, want %q", suites[1].Name, "Suite Two")
		}
	}
}

func TestTestLoader_LoadDirectory_NonExistent(t *testing.T) {
	loader := NewTestLoader("/nonexistent")
	_, err := loader.LoadDirectory(".")
	if err == nil {
		t.Error("LoadDirectory should fail for nonexistent directory")
	}
}

func TestTestLoader_LoadBlueprint(t *testing.T) {
	tempDir := t.TempDir()
	testsDir := filepath.Join(tempDir, "tests")
	if err := os.MkdirAll(testsDir, 0755); err != nil {
		t.Fatalf("Failed to create tests directory: %v", err)
	}

	testContent := `
name: Blueprint Tests
tests:
  - name: test_bp
    assertions:
      - type: no_failures
`
	if err := os.WriteFile(filepath.Join(testsDir, "bp_test.yaml"), []byte(testContent), 0644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	loader := NewTestLoader("")
	suites, err := loader.LoadBlueprint(tempDir)
	if err != nil {
		t.Fatalf("LoadBlueprint failed: %v", err)
	}

	if len(suites) != 1 {
		t.Errorf("len(suites) = %d, want 1", len(suites))
	}
}

func TestTestLoader_LoadBlueprint_NoTestsDir(t *testing.T) {
	tempDir := t.TempDir()

	loader := NewTestLoader("")
	suites, err := loader.LoadBlueprint(tempDir)
	if err != nil {
		t.Fatalf("LoadBlueprint should not fail for missing tests directory: %v", err)
	}
	if suites != nil {
		t.Errorf("suites should be nil when tests directory does not exist")
	}
}

func TestTestLoader_Validation_NoTests(t *testing.T) {
	tempDir := t.TempDir()
	testContent := `
name: Empty Suite
tests: []
`
	testFile := filepath.Join(tempDir, "empty_test.yaml")
	if err := os.WriteFile(testFile, []byte(testContent), 0644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	loader := NewTestLoader(tempDir)
	_, err := loader.LoadFile(testFile)
	if err == nil {
		t.Error("LoadFile should fail for suite with no tests")
	}
}

func TestTestLoader_Validation_TestWithoutName(t *testing.T) {
	tempDir := t.TempDir()
	testContent := `
name: Bad Suite
tests:
  - assertions:
      - type: no_failures
`
	testFile := filepath.Join(tempDir, "bad_test.yaml")
	if err := os.WriteFile(testFile, []byte(testContent), 0644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	loader := NewTestLoader(tempDir)
	_, err := loader.LoadFile(testFile)
	if err == nil {
		t.Error("LoadFile should fail for test without name")
	}
}

func TestTestLoader_Validation_TestWithoutAssertions(t *testing.T) {
	tempDir := t.TempDir()
	testContent := `
name: Bad Suite
tests:
  - name: no_assertions_test
    assertions: []
`
	testFile := filepath.Join(tempDir, "bad_test.yaml")
	if err := os.WriteFile(testFile, []byte(testContent), 0644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	loader := NewTestLoader(tempDir)
	_, err := loader.LoadFile(testFile)
	if err == nil {
		t.Error("LoadFile should fail for test without assertions")
	}
}

func TestTestLoader_Validation_TestWithExpectFailure(t *testing.T) {
	tempDir := t.TempDir()
	testContent := `
name: Expect Failure Suite
tests:
  - name: expect_fail_test
    expect_failure: true
`
	testFile := filepath.Join(tempDir, "expect_fail_test.yaml")
	if err := os.WriteFile(testFile, []byte(testContent), 0644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	loader := NewTestLoader(tempDir)
	suite, err := loader.LoadFile(testFile)
	if err != nil {
		t.Fatalf("LoadFile should succeed for test with expect_failure: %v", err)
	}
	if !suite.Tests[0].ExpectFailure {
		t.Error("ExpectFailure should be true")
	}
}

func TestTestLoader_Validation_TestWithSkip(t *testing.T) {
	tempDir := t.TempDir()
	testContent := `
name: Skip Suite
tests:
  - name: skip_test
    skip: "not implemented yet"
`
	testFile := filepath.Join(tempDir, "skip_test.yaml")
	if err := os.WriteFile(testFile, []byte(testContent), 0644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	loader := NewTestLoader(tempDir)
	suite, err := loader.LoadFile(testFile)
	if err != nil {
		t.Fatalf("LoadFile should succeed for skipped test: %v", err)
	}
	if suite.Tests[0].Skip != "not implemented yet" {
		t.Errorf("Skip = %q, want %q", suite.Tests[0].Skip, "not implemented yet")
	}
}

func TestTestLoader_ApplyDefaults(t *testing.T) {
	tempDir := t.TempDir()
	testContent := `
name: Defaults Suite
defaults:
  timeout: 60s
  dry_run: true
  parameters:
    default_param: value1
tests:
  - name: test_with_defaults
    assertions:
      - type: no_failures
  - name: test_with_override
    timeout: 120s
    parameters:
      override_param: value2
    assertions:
      - type: no_failures
`
	testFile := filepath.Join(tempDir, "defaults_test.yaml")
	if err := os.WriteFile(testFile, []byte(testContent), 0644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	loader := NewTestLoader(tempDir)
	suite, err := loader.LoadFile(testFile)
	if err != nil {
		t.Fatalf("LoadFile failed: %v", err)
	}

	// First test should have defaults applied
	if suite.Tests[0].Timeout != 60000000000 { // 60s in nanoseconds
		t.Errorf("Tests[0].Timeout = %v, want 60s", suite.Tests[0].Timeout)
	}
	if !suite.Tests[0].DryRun {
		t.Error("Tests[0].DryRun should be true from defaults")
	}
	if suite.Tests[0].Parameters["default_param"] != "value1" {
		t.Errorf("Tests[0].Parameters[default_param] = %v, want %q", suite.Tests[0].Parameters["default_param"], "value1")
	}

	// Second test should have override
	if suite.Tests[1].Timeout != 120000000000 { // 120s in nanoseconds
		t.Errorf("Tests[1].Timeout = %v, want 120s", suite.Tests[1].Timeout)
	}
	// Override param should be present
	if suite.Tests[1].Parameters["override_param"] != "value2" {
		t.Errorf("Tests[1].Parameters[override_param] = %v, want %q", suite.Tests[1].Parameters["override_param"], "value2")
	}
	// Default param should also be present (merged)
	if suite.Tests[1].Parameters["default_param"] != "value1" {
		t.Errorf("Tests[1].Parameters[default_param] = %v, want %q", suite.Tests[1].Parameters["default_param"], "value1")
	}
}

func TestFilterTests(t *testing.T) {
	tests := []TestCase{
		{Name: "test1", Tags: []string{"quick", "smoke"}},
		{Name: "test2", Tags: []string{"slow", "integration"}},
		{Name: "test3", Tags: []string{"quick", "integration"}},
		{Name: "test4", Tags: []string{}},
	}

	// Filter by include tags
	filtered := FilterTests(tests, []string{"quick"}, nil)
	if len(filtered) != 2 {
		t.Errorf("FilterTests with include tag 'quick': len = %d, want 2", len(filtered))
	}

	// Filter by exclude tags
	filtered = FilterTests(tests, nil, []string{"slow"})
	if len(filtered) != 3 {
		t.Errorf("FilterTests with exclude tag 'slow': len = %d, want 3", len(filtered))
	}

	// Filter with both include and exclude
	filtered = FilterTests(tests, []string{"integration"}, []string{"slow"})
	if len(filtered) != 1 {
		t.Errorf("FilterTests with include 'integration' exclude 'slow': len = %d, want 1", len(filtered))
	}
	if len(filtered) > 0 && filtered[0].Name != "test3" {
		t.Errorf("Expected test3 to pass filter, got %s", filtered[0].Name)
	}

	// No filters
	filtered = FilterTests(tests, nil, nil)
	if len(filtered) != 4 {
		t.Errorf("FilterTests with no filters: len = %d, want 4", len(filtered))
	}
}

func TestFilterTestsByName(t *testing.T) {
	tests := []TestCase{
		{Name: "validation_basic"},
		{Name: "validation_advanced"},
		{Name: "integration_test"},
		{Name: "smoke_test"},
	}

	// Exact match
	filtered := FilterTestsByName(tests, "smoke_test")
	if len(filtered) != 1 {
		t.Errorf("FilterTestsByName exact match: len = %d, want 1", len(filtered))
	}

	// Prefix wildcard
	filtered = FilterTestsByName(tests, "validation*")
	if len(filtered) != 2 {
		t.Errorf("FilterTestsByName prefix 'validation*': len = %d, want 2", len(filtered))
	}

	// Suffix wildcard
	filtered = FilterTestsByName(tests, "*_test")
	if len(filtered) != 2 {
		t.Errorf("FilterTestsByName suffix '*_test': len = %d, want 2", len(filtered))
	}

	// Substring wildcard
	filtered = FilterTestsByName(tests, "*ation*")
	if len(filtered) != 3 {
		t.Errorf("FilterTestsByName substring '*ation*': len = %d, want 3", len(filtered))
	}

	// Match all
	filtered = FilterTestsByName(tests, "*")
	if len(filtered) != 4 {
		t.Errorf("FilterTestsByName '*': len = %d, want 4", len(filtered))
	}

	// Empty pattern
	filtered = FilterTestsByName(tests, "")
	if len(filtered) != 4 {
		t.Errorf("FilterTestsByName empty: len = %d, want 4", len(filtered))
	}
}

func TestMatchesPattern(t *testing.T) {
	testCases := []struct {
		name    string
		pattern string
		want    bool
	}{
		{"exact", "exact", true},
		{"exact", "different", false},
		{"anything", "*", true},
		{"prefix_match", "prefix*", true},
		{"no_prefix", "prefix*", false},
		{"match_suffix", "*suffix", true},
		{"no_suffix", "*suffix", false},
		{"contains_middle", "*middle*", true},
		{"no_middle", "*middle*", false},
		{"complex_match", "test*case", true},
		{"complex_no_match", "test*case", false},
	}

	inputs := map[string]string{
		"exact":            "exact",
		"anything":         "anything",
		"prefix_match":     "prefix_something",
		"no_prefix":        "not_prefix",
		"match_suffix":     "end_with_suffix",
		"no_suffix":        "suffix_not_at_end",
		"contains_middle":  "has_middle_inside",
		"no_middle":        "no_mid_here",
		"complex_match":    "test_my_case",
		"complex_no_match": "testno_case_at_end",
	}

	for _, tc := range testCases {
		input := inputs[tc.name]
		got := matchesPattern(input, tc.pattern)
		if got != tc.want {
			t.Errorf("matchesPattern(%q, %q) = %v, want %v", input, tc.pattern, got, tc.want)
		}
	}
}

func TestIsTestFile(t *testing.T) {
	testCases := []struct {
		name string
		want bool
	}{
		{"test.yaml", true},
		{"test.yml", true},
		{"basic_test.yaml", true},
		{"test.json", false},
		{"readme.md", false},
		{"config.toml", false},
		{".yaml", true}, // edge case
	}

	for _, tc := range testCases {
		got := isTestFile(tc.name)
		if got != tc.want {
			t.Errorf("isTestFile(%q) = %v, want %v", tc.name, got, tc.want)
		}
	}
}

func TestTestLoader_ResolvePath(t *testing.T) {
	loader := NewTestLoader("/base/path")

	// Relative path should be resolved
	resolved := loader.resolvePath("relative/file.yaml")
	expected := "/base/path/relative/file.yaml"
	if resolved != expected {
		t.Errorf("resolvePath(relative) = %q, want %q", resolved, expected)
	}

	// Absolute path should be returned as-is
	resolved = loader.resolvePath("/absolute/file.yaml")
	if resolved != "/absolute/file.yaml" {
		t.Errorf("resolvePath(absolute) = %q, want %q", resolved, "/absolute/file.yaml")
	}
}
