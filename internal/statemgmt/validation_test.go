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

func TestValidator_ValidateRetryConfigDelay(t *testing.T) {
	validator := NewValidator()

	stateFile := &StateFile{
		States: map[string][]StateDeclaration{
			"file": {
				{
					ID:     "/tmp/test.txt",
					Module: "file",
					State:  "present",
					Retry: &RetryConfig{
						Attempts: 3,
						Delay:    -1, // Invalid negative delay
					},
				},
			},
		},
	}

	result := validator.Validate(stateFile)

	if result.Valid {
		t.Error("Expected validation to fail with invalid retry delay")
	}

	found := false
	for _, issue := range result.Issues {
		if issue.Code == "INVALID_RETRY_DELAY" {
			found = true
			break
		}
	}
	if !found {
		t.Error("Expected INVALID_RETRY_DELAY error")
	}
}

func TestValidator_ValidateRetryConfigBackoffMultiplier(t *testing.T) {
	validator := NewValidator()

	// Test negative backoff multiplier (error)
	stateFile := &StateFile{
		States: map[string][]StateDeclaration{
			"file": {
				{
					ID:     "/tmp/test.txt",
					Module: "file",
					State:  "present",
					Retry: &RetryConfig{
						Attempts:          3,
						Delay:             1,
						BackoffMultiplier: -2, // Invalid negative multiplier
					},
				},
			},
		},
	}

	result := validator.Validate(stateFile)

	if result.Valid {
		t.Error("Expected validation to fail with invalid backoff multiplier")
	}

	found := false
	for _, issue := range result.Issues {
		if issue.Code == "INVALID_BACKOFF_MULTIPLIER" {
			found = true
			break
		}
	}
	if !found {
		t.Error("Expected INVALID_BACKOFF_MULTIPLIER error")
	}
}

func TestValidator_ValidateRetryConfigBackoffMultiplierWarning(t *testing.T) {
	validator := NewValidator()

	// Test backoff multiplier between 0 and 1 (warning - reduces delay over time)
	stateFile := &StateFile{
		States: map[string][]StateDeclaration{
			"file": {
				{
					ID:     "/tmp/test.txt",
					Module: "file",
					State:  "present",
					Retry: &RetryConfig{
						Attempts:          3,
						Delay:             1,
						BackoffMultiplier: 0.5, // Unusual - reduces delay over time
					},
				},
			},
		},
	}

	result := validator.Validate(stateFile)

	// This should be valid but have a warning
	foundWarning := false
	for _, issue := range result.Issues {
		if issue.Level == ValidationLevelWarning && issue.Field == "retry.backoff_multiplier" {
			foundWarning = true
			break
		}
	}
	if !foundWarning {
		t.Error("Expected warning for backoff multiplier between 0 and 1")
	}
}

func TestValidator_ValidateRetryConfigValid(t *testing.T) {
	validator := NewValidator()

	// Test valid retry config
	stateFile := &StateFile{
		States: map[string][]StateDeclaration{
			"file": {
				{
					ID:     "/tmp/test.txt",
					Module: "file",
					State:  "present",
					Retry: &RetryConfig{
						Attempts:          3,
						Delay:             5,
						BackoffMultiplier: 2.0, // Valid exponential backoff
					},
				},
			},
		},
	}

	result := validator.Validate(stateFile)

	// Should be valid with no retry-related errors
	for _, issue := range result.Issues {
		if issue.Level == ValidationLevelError && (issue.Code == "INVALID_RETRY_ATTEMPTS" ||
			issue.Code == "INVALID_RETRY_DELAY" ||
			issue.Code == "INVALID_BACKOFF_MULTIPLIER") {
			t.Errorf("Unexpected retry validation error: %s", issue.Code)
		}
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

	result.AddIssue(&StateValidationError{
		Level:   ValidationLevelWarning,
		Message: "test warning",
	})

	if !strings.Contains(result.Summary(), "warning") {
		t.Errorf("Expected summary to mention warnings: %s", result.Summary())
	}

	result.AddIssue(&StateValidationError{
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

func TestValidationResult_ErrorMessages(t *testing.T) {
	tests := []struct {
		name     string
		issues   []*StateValidationError
		expected []string
	}{
		{
			name:     "no issues",
			issues:   nil,
			expected: nil,
		},
		{
			name: "only warnings and info",
			issues: []*StateValidationError{
				{Level: ValidationLevelWarning, Message: "warning message"},
				{Level: ValidationLevelInfo, Message: "info message"},
			},
			expected: nil,
		},
		{
			name: "only errors",
			issues: []*StateValidationError{
				{Level: ValidationLevelError, Message: "error 1"},
				{Level: ValidationLevelError, Message: "error 2"},
			},
			expected: []string{"error: error 1", "error: error 2"},
		},
		{
			name: "mixed issues",
			issues: []*StateValidationError{
				{Level: ValidationLevelError, Message: "error message"},
				{Level: ValidationLevelWarning, Message: "warning message"},
				{Level: ValidationLevelInfo, Message: "info message"},
			},
			expected: []string{"error: error message"},
		},
		{
			name: "error with module and state ID",
			issues: []*StateValidationError{
				{Level: ValidationLevelError, Module: "file", StateID: "test", Message: "error message"},
			},
			expected: []string{"[file.test] error: error message"},
		},
		{
			name: "error with field",
			issues: []*StateValidationError{
				{Level: ValidationLevelError, Message: "error message", Field: "mode"},
			},
			expected: []string{"error: error message (field: mode)"},
		},
		{
			name: "error with line and column",
			issues: []*StateValidationError{
				{Level: ValidationLevelError, Message: "error message", Line: 10, Column: 5},
			},
			expected: []string{"error: error message at line 10, column 5"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := &ValidationResult{Valid: true}
			for _, issue := range tt.issues {
				result.AddIssue(issue)
			}

			msgs := result.ErrorMessages()

			if len(msgs) != len(tt.expected) {
				t.Errorf("Expected %d error messages, got %d", len(tt.expected), len(msgs))
				return
			}

			for i, msg := range msgs {
				if msg != tt.expected[i] {
					t.Errorf("Expected error message %d to be %q, got %q", i, tt.expected[i], msg)
				}
			}
		})
	}
}

func TestValidateFieldType(t *testing.T) {
	tests := []struct {
		name      string
		field     string
		value     interface{}
		expected  FieldType
		wantError bool
	}{
		// String type
		{
			name:      "valid string",
			field:     "source",
			value:     "/path/to/file",
			expected:  FieldTypeString,
			wantError: false,
		},
		{
			name:      "invalid string - int",
			field:     "source",
			value:     123,
			expected:  FieldTypeString,
			wantError: true,
		},
		{
			name:      "invalid string - bool",
			field:     "source",
			value:     true,
			expected:  FieldTypeString,
			wantError: true,
		},
		// Bool type
		{
			name:      "valid bool",
			field:     "enabled",
			value:     true,
			expected:  FieldTypeBool,
			wantError: false,
		},
		{
			name:      "invalid bool - string",
			field:     "enabled",
			value:     "true",
			expected:  FieldTypeBool,
			wantError: true,
		},
		{
			name:      "invalid bool - int",
			field:     "enabled",
			value:     1,
			expected:  FieldTypeBool,
			wantError: true,
		},
		// Int type
		{
			name:      "valid int",
			field:     "uid",
			value:     1000,
			expected:  FieldTypeInt,
			wantError: false,
		},
		{
			name:      "valid int - int32",
			field:     "uid",
			value:     int32(1000),
			expected:  FieldTypeInt,
			wantError: false,
		},
		{
			name:      "valid int - int64",
			field:     "uid",
			value:     int64(1000),
			expected:  FieldTypeInt,
			wantError: false,
		},
		{
			name:      "valid int - float64",
			field:     "uid",
			value:     float64(1000),
			expected:  FieldTypeInt,
			wantError: false,
		},
		{
			name:      "invalid int - string",
			field:     "uid",
			value:     "1000",
			expected:  FieldTypeInt,
			wantError: true,
		},
		// Float type
		{
			name:      "valid float - float64",
			field:     "ratio",
			value:     3.14,
			expected:  FieldTypeFloat,
			wantError: false,
		},
		{
			name:      "valid float - float32",
			field:     "ratio",
			value:     float32(3.14),
			expected:  FieldTypeFloat,
			wantError: false,
		},
		{
			name:      "valid float - int",
			field:     "ratio",
			value:     3,
			expected:  FieldTypeFloat,
			wantError: false,
		},
		{
			name:      "valid float - int64",
			field:     "ratio",
			value:     int64(3),
			expected:  FieldTypeFloat,
			wantError: false,
		},
		{
			name:      "invalid float - string",
			field:     "ratio",
			value:     "3.14",
			expected:  FieldTypeFloat,
			wantError: true,
		},
		// List type
		{
			name:      "valid list",
			field:     "groups",
			value:     []interface{}{"admin", "users"},
			expected:  FieldTypeList,
			wantError: false,
		},
		{
			name:      "invalid list - string slice",
			field:     "groups",
			value:     []string{"admin", "users"},
			expected:  FieldTypeList,
			wantError: true,
		},
		{
			name:      "invalid list - string",
			field:     "groups",
			value:     "admin,users",
			expected:  FieldTypeList,
			wantError: true,
		},
		// Map type
		{
			name:      "valid map",
			field:     "metadata",
			value:     map[string]interface{}{"key": "value"},
			expected:  FieldTypeMap,
			wantError: false,
		},
		{
			name:      "invalid map - string map",
			field:     "metadata",
			value:     map[string]string{"key": "value"},
			expected:  FieldTypeMap,
			wantError: true,
		},
		{
			name:      "invalid map - slice",
			field:     "metadata",
			value:     []interface{}{"key", "value"},
			expected:  FieldTypeMap,
			wantError: true,
		},
		// Any type (always valid)
		{
			name:      "any type - string",
			field:     "data",
			value:     "string value",
			expected:  FieldTypeAny,
			wantError: false,
		},
		{
			name:      "any type - int",
			field:     "data",
			value:     123,
			expected:  FieldTypeAny,
			wantError: false,
		},
		{
			name:      "any type - nil",
			field:     "data",
			value:     nil,
			expected:  FieldTypeAny,
			wantError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateFieldType(tt.field, tt.value, tt.expected)
			if tt.wantError && err == nil {
				t.Errorf("Expected error but got none")
			}
			if !tt.wantError && err != nil {
				t.Errorf("Expected no error but got: %v", err)
			}
		})
	}
}

func TestContainsStr(t *testing.T) {
	tests := []struct {
		name     string
		slice    []string
		item     string
		expected bool
	}{
		{
			name:     "found in slice",
			slice:    []string{"apple", "banana", "cherry"},
			item:     "banana",
			expected: true,
		},
		{
			name:     "not found in slice",
			slice:    []string{"apple", "banana", "cherry"},
			item:     "grape",
			expected: false,
		},
		{
			name:     "empty slice",
			slice:    []string{},
			item:     "apple",
			expected: false,
		},
		{
			name:     "nil slice",
			slice:    nil,
			item:     "apple",
			expected: false,
		},
		{
			name:     "first element",
			slice:    []string{"apple", "banana", "cherry"},
			item:     "apple",
			expected: true,
		},
		{
			name:     "last element",
			slice:    []string{"apple", "banana", "cherry"},
			item:     "cherry",
			expected: true,
		},
		{
			name:     "case sensitive",
			slice:    []string{"Apple", "Banana"},
			item:     "apple",
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := containsStr(tt.slice, tt.item)
			if result != tt.expected {
				t.Errorf("Expected %v, got %v", tt.expected, result)
			}
		})
	}
}

func TestKnownModuleNames(t *testing.T) {
	tests := []struct {
		name     string
		modules  map[string]*ModuleSchema
		expected []string
	}{
		{
			name:     "empty map",
			modules:  map[string]*ModuleSchema{},
			expected: []string{},
		},
		{
			name: "single module",
			modules: map[string]*ModuleSchema{
				"file": {Name: "file"},
			},
			expected: []string{"file"},
		},
		{
			name: "multiple modules sorted",
			modules: map[string]*ModuleSchema{
				"service": {Name: "service"},
				"file":    {Name: "file"},
				"package": {Name: "package"},
			},
			expected: []string{"file", "package", "service"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := knownModuleNames(tt.modules)

			if len(result) != len(tt.expected) {
				t.Errorf("Expected %d names, got %d", len(tt.expected), len(result))
				return
			}

			for i, name := range result {
				if name != tt.expected[i] {
					t.Errorf("Expected name %d to be %q, got %q", i, tt.expected[i], name)
				}
			}
		})
	}
}

func TestStateValidationError_Error(t *testing.T) {
	tests := []struct {
		name     string
		issue    *StateValidationError
		expected string
	}{
		{
			name: "basic error",
			issue: &StateValidationError{
				Level:   ValidationLevelError,
				Message: "test error",
			},
			expected: "error: test error",
		},
		{
			name: "with module only",
			issue: &StateValidationError{
				Level:   ValidationLevelWarning,
				Module:  "file",
				Message: "test warning",
			},
			expected: "[file] warning: test warning",
		},
		{
			name: "with module and state ID",
			issue: &StateValidationError{
				Level:   ValidationLevelError,
				Module:  "file",
				StateID: "test-file",
				Message: "test error",
			},
			expected: "[file.test-file] error: test error",
		},
		{
			name: "with field",
			issue: &StateValidationError{
				Level:   ValidationLevelError,
				Message: "test error",
				Field:   "mode",
			},
			expected: "error: test error (field: mode)",
		},
		{
			name: "with line only",
			issue: &StateValidationError{
				Level:   ValidationLevelError,
				Message: "test error",
				Line:    10,
			},
			expected: "error: test error at line 10",
		},
		{
			name: "with line and column",
			issue: &StateValidationError{
				Level:   ValidationLevelError,
				Message: "test error",
				Line:    10,
				Column:  5,
			},
			expected: "error: test error at line 10, column 5",
		},
		{
			name: "full details",
			issue: &StateValidationError{
				Level:   ValidationLevelError,
				Module:  "file",
				StateID: "test-file",
				Message: "test error",
				Field:   "mode",
				Line:    10,
				Column:  5,
			},
			expected: "[file.test-file] error: test error (field: mode) at line 10, column 5",
		},
		{
			name: "info level",
			issue: &StateValidationError{
				Level:   ValidationLevelInfo,
				Message: "info message",
			},
			expected: "info: info message",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.issue.Error()
			if result != tt.expected {
				t.Errorf("Expected %q, got %q", tt.expected, result)
			}
		})
	}
}

func TestValidationResult_AddIssue(t *testing.T) {
	tests := []struct {
		name         string
		levels       []ValidationLevel
		wantErrors   int
		wantWarnings int
		wantInfos    int
		wantValid    bool
	}{
		{
			name:         "no issues",
			levels:       nil,
			wantErrors:   0,
			wantWarnings: 0,
			wantInfos:    0,
			wantValid:    true,
		},
		{
			name:         "single error",
			levels:       []ValidationLevel{ValidationLevelError},
			wantErrors:   1,
			wantWarnings: 0,
			wantInfos:    0,
			wantValid:    false,
		},
		{
			name:         "single warning",
			levels:       []ValidationLevel{ValidationLevelWarning},
			wantErrors:   0,
			wantWarnings: 1,
			wantInfos:    0,
			wantValid:    true,
		},
		{
			name:         "single info",
			levels:       []ValidationLevel{ValidationLevelInfo},
			wantErrors:   0,
			wantWarnings: 0,
			wantInfos:    1,
			wantValid:    true,
		},
		{
			name: "mixed issues",
			levels: []ValidationLevel{
				ValidationLevelError,
				ValidationLevelWarning,
				ValidationLevelInfo,
				ValidationLevelError,
				ValidationLevelWarning,
			},
			wantErrors:   2,
			wantWarnings: 2,
			wantInfos:    1,
			wantValid:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := &ValidationResult{Valid: true}

			for i, level := range tt.levels {
				result.AddIssue(&StateValidationError{
					Level:   level,
					Message: "test",
					Code:    "TEST" + string(rune('0'+i)),
				})
			}

			if result.Errors != tt.wantErrors {
				t.Errorf("Expected %d errors, got %d", tt.wantErrors, result.Errors)
			}
			if result.Warnings != tt.wantWarnings {
				t.Errorf("Expected %d warnings, got %d", tt.wantWarnings, result.Warnings)
			}
			if result.Infos != tt.wantInfos {
				t.Errorf("Expected %d infos, got %d", tt.wantInfos, result.Infos)
			}
			if result.Valid != tt.wantValid {
				t.Errorf("Expected valid=%v, got valid=%v", tt.wantValid, result.Valid)
			}
		})
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
