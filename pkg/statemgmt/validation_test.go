package statemgmt

import (
	"strings"
	"testing"
)

func TestValidator_ValidateEmptyStateFile(t *testing.T) {
	validator := NewValidator()
	stateFile := &StateFile{
		States: make(map[string][]StateDeclaration),
	}

	result := validator.Validate(stateFile)

	// Should warn about empty state file
	if result.Warnings == 0 {
		t.Error("Expected warning for empty state file")
	}

	found := false
	for _, issue := range result.Issues {
		if issue.Code == "EMPTY_STATE_FILE" {
			found = true
			break
		}
	}
	if !found {
		t.Error("Expected EMPTY_STATE_FILE warning")
	}
}

func TestValidator_ValidateUnknownModule(t *testing.T) {
	validator := NewValidator()
	validator.StrictMode = true

	stateFile := &StateFile{
		States: map[string][]StateDeclaration{
			"unknown_module": {
				{
					ID:     "test",
					Module: "unknown_module",
					State:  "present",
				},
			},
		},
	}

	result := validator.Validate(stateFile)

	if result.Valid {
		t.Error("Expected validation to fail with unknown module in strict mode")
	}

	found := false
	for _, issue := range result.Issues {
		if issue.Code == "UNKNOWN_MODULE" {
			found = true
			break
		}
	}
	if !found {
		t.Error("Expected UNKNOWN_MODULE error")
	}
}

func TestValidator_ValidateInvalidState(t *testing.T) {
	validator := NewValidator()

	stateFile := &StateFile{
		States: map[string][]StateDeclaration{
			"file": {
				{
					ID:     "/tmp/test.txt",
					Module: "file",
					State:  "invalid_state",
				},
			},
		},
	}

	result := validator.Validate(stateFile)

	if result.Valid {
		t.Error("Expected validation to fail with invalid state")
	}

	found := false
	for _, issue := range result.Issues {
		if issue.Code == "INVALID_STATE" {
			found = true
			if !strings.Contains(issue.Suggestion, "present") {
				t.Error("Expected suggestion to include valid states")
			}
			break
		}
	}
	if !found {
		t.Error("Expected INVALID_STATE error")
	}
}

func TestValidator_ValidateMissingRequiredField(t *testing.T) {
	validator := NewValidator()

	stateFile := &StateFile{
		States: map[string][]StateDeclaration{
			"git": {
				{
					ID:         "/opt/repo",
					Module:     "git",
					State:      "present",
					Parameters: map[string]interface{}{},
				},
			},
		},
	}

	result := validator.Validate(stateFile)

	if result.Valid {
		t.Error("Expected validation to fail with missing required field")
	}

	found := false
	for _, issue := range result.Issues {
		if issue.Code == "MISSING_REQUIRED_FIELD" && issue.Field == "source" {
			found = true
			break
		}
	}
	if !found {
		t.Error("Expected MISSING_REQUIRED_FIELD error for 'source'")
	}
}

func TestValidator_ValidateMutuallyExclusiveFields(t *testing.T) {
	validator := NewValidator()

	stateFile := &StateFile{
		States: map[string][]StateDeclaration{
			"file": {
				{
					ID:     "/tmp/test.txt",
					Module: "file",
					State:  "present",
					Parameters: map[string]interface{}{
						"source":   "/etc/source.txt",
						"contents": "test content",
					},
				},
			},
		},
	}

	result := validator.Validate(stateFile)

	if result.Valid {
		t.Error("Expected validation to fail with mutually exclusive fields")
	}

	found := false
	for _, issue := range result.Issues {
		if issue.Code == "MUTUALLY_EXCLUSIVE_FIELDS" {
			found = true
			break
		}
	}
	if !found {
		t.Error("Expected MUTUALLY_EXCLUSIVE_FIELDS error")
	}
}

func TestValidator_ValidateInvalidFileMode(t *testing.T) {
	validator := NewValidator()

	stateFile := &StateFile{
		States: map[string][]StateDeclaration{
			"file": {
				{
					ID:     "/tmp/test.txt",
					Module: "file",
					State:  "present",
					Parameters: map[string]interface{}{
						"mode": "abc",
					},
				},
			},
		},
	}

	result := validator.Validate(stateFile)

	if result.Valid {
		t.Error("Expected validation to fail with invalid file mode")
	}

	found := false
	for _, issue := range result.Issues {
		if issue.Code == "INVALID_FILE_MODE" {
			found = true
			break
		}
	}
	if !found {
		t.Error("Expected INVALID_FILE_MODE error")
	}
}

func TestValidator_ValidateValidFileMode(t *testing.T) {
	validator := NewValidator()

	testCases := []string{"0644", "0755", "644", "755", "0777"}

	for _, mode := range testCases {
		stateFile := &StateFile{
			States: map[string][]StateDeclaration{
				"file": {
					{
						ID:     "/tmp/test.txt",
						Module: "file",
						State:  "present",
						Parameters: map[string]interface{}{
							"mode": mode,
						},
					},
				},
			},
		}

		result := validator.Validate(stateFile)

		for _, issue := range result.Issues {
			if issue.Code == "INVALID_FILE_MODE" {
				t.Errorf("Mode %s should be valid", mode)
			}
		}
	}
}

func TestValidator_ValidateInvalidRequisiteReference(t *testing.T) {
	validator := NewValidator()

	// Create a state file with a service that requires a package
	// Include a package state so the module exists, but reference a different ID
	stateFile := &StateFile{
		States: map[string][]StateDeclaration{
			"package": {
				{
					ID:     "existing-package",
					Module: "package",
					State:  "installed",
				},
			},
			"service": {
				{
					ID:     "nginx",
					Module: "service",
					State:  "running",
					Requisites: Requisites{
						Require: []StateReference{
							{Module: "package", ID: "nonexistent"}, // This ID doesn't exist
						},
					},
				},
			},
		},
	}

	result := validator.Validate(stateFile)

	if result.Valid {
		t.Error("Expected validation to fail with invalid requisite reference")
	}

	found := false
	for _, issue := range result.Issues {
		if issue.Code == "INVALID_REQUISITE_REFERENCE" {
			found = true
			break
		}
	}
	if !found {
		t.Error("Expected INVALID_REQUISITE_REFERENCE error")
	}
}

func TestValidator_ValidateCircularDependency(t *testing.T) {
	validator := NewValidator()

	stateFile := &StateFile{
		States: map[string][]StateDeclaration{
			"file": {
				{
					ID:     "file_a",
					Module: "file",
					State:  "present",
					Requisites: Requisites{
						Require: []StateReference{{Module: "file", ID: "file_b"}},
					},
				},
				{
					ID:     "file_b",
					Module: "file",
					State:  "present",
					Requisites: Requisites{
						Require: []StateReference{{Module: "file", ID: "file_a"}},
					},
				},
			},
		},
	}

	result := validator.Validate(stateFile)

	if result.Valid {
		t.Error("Expected validation to fail with circular dependency")
	}

	found := false
	for _, issue := range result.Issues {
		if issue.Code == "CIRCULAR_DEPENDENCY" {
			found = true
			break
		}
	}
	if !found {
		t.Error("Expected CIRCULAR_DEPENDENCY error")
	}
}

func TestValidator_ValidateFieldType(t *testing.T) {
	validator := NewValidator()

	stateFile := &StateFile{
		States: map[string][]StateDeclaration{
			"file": {
				{
					ID:     "/tmp/test.txt",
					Module: "file",
					State:  "present",
					Parameters: map[string]interface{}{
						"makedirs": "yes", // Should be bool
					},
				},
			},
		},
	}

	result := validator.Validate(stateFile)

	if result.Valid {
		t.Error("Expected validation to fail with invalid field type")
	}

	found := false
	for _, issue := range result.Issues {
		if issue.Code == "INVALID_FIELD_TYPE" {
			found = true
			break
		}
	}
	if !found {
		t.Error("Expected INVALID_FIELD_TYPE error")
	}
}

func TestValidator_ValidateRetryConfig(t *testing.T) {
	validator := NewValidator()

	stateFile := &StateFile{
		States: map[string][]StateDeclaration{
			"file": {
				{
					ID:     "/tmp/test.txt",
					Module: "file",
					State:  "present",
					Retry: &RetryConfig{
						Attempts: -1,
					},
				},
			},
		},
	}

	result := validator.Validate(stateFile)

	if result.Valid {
		t.Error("Expected validation to fail with invalid retry config")
	}

	found := false
	for _, issue := range result.Issues {
		if issue.Code == "INVALID_RETRY_ATTEMPTS" {
			found = true
			break
		}
	}
	if !found {
		t.Error("Expected INVALID_RETRY_ATTEMPTS error")
	}
}

func TestValidator_ValidateValidStateFile(t *testing.T) {
	validator := NewValidator()

	stateFile := &StateFile{
		Metadata: StateMetadata{
			Name:        "test-state",
			Description: "Test state file",
			Version:     "1.0.0",
		},
		States: map[string][]StateDeclaration{
			"package": {
				{
					ID:     "nginx",
					Module: "package",
					State:  "installed",
				},
			},
			"file": {
				{
					ID:     "/etc/nginx/nginx.conf",
					Module: "file",
					State:  "present",
					Parameters: map[string]interface{}{
						"source": "/tmp/nginx.conf",
						"mode":   "0644",
						"owner":  "root",
					},
					Requisites: Requisites{
						Require: []StateReference{{Module: "package", ID: "nginx"}},
					},
				},
			},
			"service": {
				{
					ID:     "nginx",
					Module: "service",
					State:  "running",
					Parameters: map[string]interface{}{
						"enabled": true,
					},
					Requisites: Requisites{
						Require: []StateReference{
							{Module: "package", ID: "nginx"},
							{Module: "file", ID: "/etc/nginx/nginx.conf"},
						},
					},
				},
			},
		},
	}

	result := validator.Validate(stateFile)

	if !result.Valid {
		t.Errorf("Expected validation to pass, got errors: %v", result.ErrorMessages())
	}
}

func TestValidationResult_Summary(t *testing.T) {
	result := &ValidationResult{Valid: true}

	if result.Summary() != "Validation passed" {
		t.Errorf("Expected 'Validation passed', got '%s'", result.Summary())
	}

	result.AddIssue(&ValidationIssue{
		Level:   ValidationLevelWarning,
		Message: "test warning",
	})

	if !strings.Contains(result.Summary(), "warning") {
		t.Errorf("Expected summary to mention warnings: %s", result.Summary())
	}

	result.AddIssue(&ValidationIssue{
		Level:   ValidationLevelError,
		Message: "test error",
	})

	if !strings.Contains(result.Summary(), "failed") {
		t.Errorf("Expected summary to mention failure: %s", result.Summary())
	}
}

func TestValidateBeforeApply(t *testing.T) {
	// Valid state file
	valid := &StateFile{
		States: map[string][]StateDeclaration{
			"file": {
				{
					ID:     "/tmp/test.txt",
					Module: "file",
					State:  "present",
				},
			},
		},
	}

	err := ValidateBeforeApply(valid)
	if err != nil {
		t.Errorf("Expected no error for valid state file, got: %v", err)
	}

	// Invalid state file
	invalid := &StateFile{
		States: map[string][]StateDeclaration{
			"file": {
				{
					ID:     "",
					Module: "file",
					State:  "present",
				},
			},
		},
	}

	err = ValidateBeforeApply(invalid)
	if err == nil {
		t.Error("Expected error for invalid state file")
	}
}

func TestIsValidVersion(t *testing.T) {
	validVersions := []string{
		"1.0.0",
		"v1.0.0",
		"1.2.3",
		"1.0.0-alpha",
		"1.0.0-beta.1",
		"1.0.0+build.123",
	}

	invalidVersions := []string{
		"1.0",
		"1",
		"abc",
		"1.0.0.0",
	}

	for _, v := range validVersions {
		if !isValidVersion(v) {
			t.Errorf("Expected %s to be valid version", v)
		}
	}

	for _, v := range invalidVersions {
		if isValidVersion(v) {
			t.Errorf("Expected %s to be invalid version", v)
		}
	}
}

func TestIsValidFileMode(t *testing.T) {
	validModes := []string{
		"0644",
		"0755",
		"644",
		"755",
		"0777",
		"0600",
	}

	invalidModes := []string{
		"abc",
		"9999",
		"0888",
		"rw-r--r--",
	}

	for _, m := range validModes {
		if !isValidFileMode(m) {
			t.Errorf("Expected %s to be valid file mode", m)
		}
	}

	for _, m := range invalidModes {
		if isValidFileMode(m) {
			t.Errorf("Expected %s to be invalid file mode", m)
		}
	}
}

func BenchmarkValidator_Validate(b *testing.B) {
	validator := NewValidator()

	stateFile := &StateFile{
		States: map[string][]StateDeclaration{
			"file": make([]StateDeclaration, 100),
		},
	}

	for i := 0; i < 100; i++ {
		stateFile.States["file"][i] = StateDeclaration{
			ID:     "/tmp/test-" + string(rune(i)) + ".txt",
			Module: "file",
			State:  "present",
			Parameters: map[string]interface{}{
				"mode":  "0644",
				"owner": "root",
			},
		}
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		validator.Validate(stateFile)
	}
}
