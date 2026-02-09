package blueprint

import (
	"strings"
	"testing"
)

func TestValidator_ValidBlueprint(t *testing.T) {
	bp := &Blueprint{
		APIVersion: APIVersion,
		Kind:       Kind,
		Metadata: Metadata{
			Name:    "valid-blueprint",
			Version: "1.0.0",
		},
	}

	v := NewValidator()
	result := v.Validate(bp)

	if !result.Valid {
		t.Errorf("Valid blueprint marked invalid: %v", result.Errors)
	}
}

func TestValidator_MissingAPIVersion(t *testing.T) {
	bp := &Blueprint{
		Kind: Kind,
		Metadata: Metadata{
			Name:    "test",
			Version: "1.0.0",
		},
	}

	v := NewValidator()
	result := v.Validate(bp)

	if result.Valid {
		t.Error("Blueprint with missing apiVersion should be invalid")
	}

	found := false
	for _, err := range result.Errors {
		if err.Parameter == "apiVersion" {
			found = true
			break
		}
	}
	if !found {
		t.Error("Expected error for apiVersion field")
	}
}

func TestValidator_MissingKind(t *testing.T) {
	bp := &Blueprint{
		APIVersion: APIVersion,
		Metadata: Metadata{
			Name:    "test",
			Version: "1.0.0",
		},
	}

	v := NewValidator()
	result := v.Validate(bp)

	if result.Valid {
		t.Error("Blueprint with missing kind should be invalid")
	}
}

func TestValidator_InvalidKind(t *testing.T) {
	bp := &Blueprint{
		APIVersion: APIVersion,
		Kind:       "InvalidKind",
		Metadata: Metadata{
			Name:    "test",
			Version: "1.0.0",
		},
	}

	v := NewValidator()
	result := v.Validate(bp)

	if result.Valid {
		t.Error("Blueprint with invalid kind should be invalid")
	}
}

func TestValidator_MissingMetadataName(t *testing.T) {
	bp := &Blueprint{
		APIVersion: APIVersion,
		Kind:       Kind,
		Metadata: Metadata{
			Version: "1.0.0",
		},
	}

	v := NewValidator()
	result := v.Validate(bp)

	if result.Valid {
		t.Error("Blueprint with missing metadata.name should be invalid")
	}
}

func TestValidator_InvalidMetadataName(t *testing.T) {
	testCases := []string{
		"InvalidName",     // uppercase
		"123-invalid",     // starts with number
		"with spaces",     // contains spaces
		"with_underscore", // contains underscore
	}

	for _, name := range testCases {
		bp := &Blueprint{
			APIVersion: APIVersion,
			Kind:       Kind,
			Metadata: Metadata{
				Name:    name,
				Version: "1.0.0",
			},
		}

		v := NewValidator()
		result := v.Validate(bp)

		if result.Valid {
			t.Errorf("Blueprint with invalid name '%s' should be invalid", name)
		}
	}
}

func TestValidator_MissingMetadataVersion(t *testing.T) {
	bp := &Blueprint{
		APIVersion: APIVersion,
		Kind:       Kind,
		Metadata: Metadata{
			Name: "test",
		},
	}

	v := NewValidator()
	result := v.Validate(bp)

	if result.Valid {
		t.Error("Blueprint with missing metadata.version should be invalid")
	}
}

func TestValidator_InvalidSemVer(t *testing.T) {
	testCases := []string{
		"1",          // missing minor and patch
		"1.0",        // missing patch
		"v1.0.0",     // has v prefix
		"1.0.0.0",    // too many parts
		"not-semver", // not a version
	}

	for _, version := range testCases {
		bp := &Blueprint{
			APIVersion: APIVersion,
			Kind:       Kind,
			Metadata: Metadata{
				Name:    "test",
				Version: version,
			},
		}

		v := NewValidator()
		result := v.Validate(bp)

		if result.Valid {
			t.Errorf("Blueprint with invalid version '%s' should be invalid", version)
		}
	}
}

func TestValidator_ValidSemVer(t *testing.T) {
	testCases := []string{
		"1.0.0",
		"0.1.0",
		"10.20.30",
		"1.0.0-alpha",
		"1.0.0-alpha.1",
		"1.0.0+build",
		"1.0.0-beta+build.123",
	}

	for _, version := range testCases {
		bp := &Blueprint{
			APIVersion: APIVersion,
			Kind:       Kind,
			Metadata: Metadata{
				Name:    "test",
				Version: version,
			},
		}

		v := NewValidator()
		result := v.Validate(bp)

		if !result.Valid {
			t.Errorf("Blueprint with valid version '%s' should be valid: %v", version, result.Errors)
		}
	}
}

func TestValidator_InvalidCompatibilityModuleRef(t *testing.T) {
	bp := &Blueprint{
		APIVersion: APIVersion,
		Kind:       Kind,
		Metadata: Metadata{
			Name:    "test",
			Version: "1.0.0",
		},
		Compatibility: &Compatibility{
			Modules: []string{
				"invalid-ref",
				"modules/valid/ref@^1.0",
			},
		},
	}

	v := NewValidator()
	result := v.Validate(bp)

	if result.Valid {
		t.Error("Blueprint with invalid module reference should be invalid")
	}
}

func TestValidator_InvalidDependencyRef(t *testing.T) {
	bp := &Blueprint{
		APIVersion: APIVersion,
		Kind:       Kind,
		Metadata: Metadata{
			Name:    "test",
			Version: "1.0.0",
		},
		Dependencies: &Dependencies{
			Requires: []string{
				"invalid-dep",
			},
		},
	}

	v := NewValidator()
	result := v.Validate(bp)

	if result.Valid {
		t.Error("Blueprint with invalid dependency reference should be invalid")
	}
}

func TestValidator_ParameterValidation(t *testing.T) {
	tests := []struct {
		name    string
		params  map[string]ParameterSchema
		wantErr bool
		errMsg  string
	}{
		{
			name: "valid string parameter",
			params: map[string]ParameterSchema{
				"name": {Type: "string", Required: true},
			},
			wantErr: false,
		},
		{
			name: "missing type",
			params: map[string]ParameterSchema{
				"name": {Required: true},
			},
			wantErr: true,
			errMsg:  "type",
		},
		{
			name: "invalid type",
			params: map[string]ParameterSchema{
				"name": {Type: "invalid"},
			},
			wantErr: true,
			errMsg:  "invalid type",
		},
		{
			name: "invalid pattern",
			params: map[string]ParameterSchema{
				"name": {Type: "string", Pattern: "[invalid"},
			},
			wantErr: true,
			errMsg:  "pattern",
		},
		{
			name: "minLength > maxLength",
			params: map[string]ParameterSchema{
				"name": {
					Type:      "string",
					MinLength: intPtr(10),
					MaxLength: intPtr(5),
				},
			},
			wantErr: true,
			errMsg:  "minLength",
		},
		{
			name: "minimum > maximum",
			params: map[string]ParameterSchema{
				"count": {
					Type:    "integer",
					Minimum: float64Ptr(10),
					Maximum: float64Ptr(5),
				},
			},
			wantErr: true,
			errMsg:  "minimum",
		},
		{
			name: "minItems > maxItems",
			params: map[string]ParameterSchema{
				"items": {
					Type:     "array",
					MinItems: intPtr(10),
					MaxItems: intPtr(5),
				},
			},
			wantErr: true,
			errMsg:  "minItems",
		},
		{
			name: "non-existent feature reference",
			params: map[string]ParameterSchema{
				"config": {Type: "string", Feature: "nonexistent"},
			},
			wantErr: true,
			errMsg:  "feature",
		},
		{
			name: "enum type mismatch",
			params: map[string]ParameterSchema{
				"level": {
					Type: "string",
					Enum: []interface{}{1, 2, 3}, // integers instead of strings
				},
			},
			wantErr: true,
			errMsg:  "enum",
		},
		{
			name: "default type mismatch",
			params: map[string]ParameterSchema{
				"name": {
					Type:    "string",
					Default: 123, // integer instead of string
				},
			},
			wantErr: true,
			errMsg:  "default",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			bp := &Blueprint{
				APIVersion: APIVersion,
				Kind:       Kind,
				Metadata: Metadata{
					Name:    "test",
					Version: "1.0.0",
				},
				Parameters: tt.params,
			}

			v := NewValidator()
			result := v.Validate(bp)

			if tt.wantErr && result.Valid {
				t.Errorf("Expected validation to fail for %s", tt.name)
			}
			if !tt.wantErr && !result.Valid {
				t.Errorf("Unexpected validation failure for %s: %v", tt.name, result.Errors)
			}
			if tt.wantErr && tt.errMsg != "" {
				found := false
				for _, err := range result.Errors {
					if strings.Contains(err.Error(), tt.errMsg) {
						found = true
						break
					}
				}
				if !found {
					t.Errorf("Expected error containing '%s', got: %v", tt.errMsg, result.Errors)
				}
			}
		})
	}
}

func TestValidator_EntrypointValidation(t *testing.T) {
	bp := &Blueprint{
		APIVersion: APIVersion,
		Kind:       Kind,
		Metadata: Metadata{
			Name:    "test",
			Version: "1.0.0",
		},
		Entrypoints: map[string]string{
			"default":   "states/init.yaml",
			"empty":     "",
			"no_suffix": "states/init",
		},
	}

	v := NewValidator()
	result := v.Validate(bp)

	// Should have error for empty path
	foundEmpty := false
	foundWarning := false
	for _, err := range result.Errors {
		if strings.Contains(err.Parameter, "empty") {
			foundEmpty = true
		}
	}
	for _, warn := range result.Warnings {
		if strings.Contains(warn.Parameter, "no_suffix") {
			foundWarning = true
		}
	}

	if !foundEmpty {
		t.Error("Expected error for empty entrypoint path")
	}
	if !foundWarning {
		t.Error("Expected warning for entrypoint without .yaml suffix")
	}
}

func TestValidator_HookValidation(t *testing.T) {
	bp := &Blueprint{
		APIVersion: APIVersion,
		Kind:       Kind,
		Metadata: Metadata{
			Name:    "test",
			Version: "1.0.0",
		},
		Hooks: &Hooks{
			PreApply:  []string{"states/hooks/pre.yaml", ""},
			PostApply: []string{"states/hooks/post"},
		},
	}

	v := NewValidator()
	result := v.Validate(bp)

	// Should have error for empty path in PreApply
	foundEmpty := false
	foundWarning := false
	for _, err := range result.Errors {
		if strings.Contains(err.Parameter, "pre_apply") {
			foundEmpty = true
		}
	}
	for _, warn := range result.Warnings {
		if strings.Contains(warn.Parameter, "post_apply") {
			foundWarning = true
		}
	}

	if !foundEmpty {
		t.Error("Expected error for empty hook path")
	}
	if !foundWarning {
		t.Error("Expected warning for hook without .yaml suffix")
	}
}

func TestValidator_OutputValidation(t *testing.T) {
	bp := &Blueprint{
		APIVersion: APIVersion,
		Kind:       Kind,
		Metadata: Metadata{
			Name:    "test",
			Version: "1.0.0",
		},
		Outputs: map[string]Output{
			"valid": {Value: "{{ app_url }}", Description: "Valid output"},
			"empty": {Description: "Empty value"},
		},
	}

	v := NewValidator()
	result := v.Validate(bp)

	if result.Valid {
		t.Error("Blueprint with empty output value should be invalid")
	}

	found := false
	for _, err := range result.Errors {
		if strings.Contains(err.Parameter, "empty.value") {
			found = true
			break
		}
	}
	if !found {
		t.Error("Expected error for empty output value")
	}
}

func TestValidationResult_Error(t *testing.T) {
	result := &ValidationResult{Valid: true}
	if result.Error() != nil {
		t.Error("Valid result should return nil error")
	}

	result.AddError("field1", "error1", nil)
	result.AddError("field2", "error2", "value")

	err := result.Error()
	if err == nil {
		t.Error("Invalid result should return error")
	}

	errStr := err.Error()
	if !strings.Contains(errStr, "field1") || !strings.Contains(errStr, "error1") {
		t.Error("Error message should contain field1 error")
	}
	if !strings.Contains(errStr, "field2") || !strings.Contains(errStr, "error2") {
		t.Error("Error message should contain field2 error")
	}
}

func TestValidationHelpers(t *testing.T) {
	// Test isValidBlueprintName
	validNames := []string{"test", "test-name", "a1-b2-c3"}
	for _, name := range validNames {
		if !isValidBlueprintName(name) {
			t.Errorf("isValidBlueprintName(%s) = false, want true", name)
		}
	}

	invalidNames := []string{"Test", "123", "-test", "test_name"}
	for _, name := range invalidNames {
		if isValidBlueprintName(name) {
			t.Errorf("isValidBlueprintName(%s) = true, want false", name)
		}
	}

	// Test isValidVersionConstraint
	validConstraints := []string{">=1.0.0", "^1.0", "~1.2", "1.0.0", "*"}
	for _, c := range validConstraints {
		if !isValidVersionConstraint(c) {
			t.Errorf("isValidVersionConstraint(%s) = false, want true", c)
		}
	}

	// Test isValidModuleReference
	validModules := []string{"modules/std/files", "modules/community/nginx@^1.0"}
	for _, m := range validModules {
		if !isValidModuleReference(m) {
			t.Errorf("isValidModuleReference(%s) = false, want true", m)
		}
	}

	invalidModules := []string{"files", "std/files", "modules/files"}
	for _, m := range invalidModules {
		if isValidModuleReference(m) {
			t.Errorf("isValidModuleReference(%s) = true, want false", m)
		}
	}

	// Test isValidBlueprintReference
	validBlueprints := []string{"blueprints/community/web-stack", "blueprints/myorg/my-bp@^1.0"}
	for _, b := range validBlueprints {
		if !isValidBlueprintReference(b) {
			t.Errorf("isValidBlueprintReference(%s) = false, want true", b)
		}
	}

	invalidBlueprints := []string{"web-stack", "community/web-stack", "blueprints/web-stack"}
	for _, b := range invalidBlueprints {
		if isValidBlueprintReference(b) {
			t.Errorf("isValidBlueprintReference(%s) = true, want false", b)
		}
	}
}

// Helper functions for tests
func intPtr(i int) *int {
	return &i
}

func float64Ptr(f float64) *float64 {
	return &f
}
