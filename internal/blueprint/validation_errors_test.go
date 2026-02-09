package blueprint

import (
	"strings"
	"testing"
)

func TestValidationError_Error(t *testing.T) {
	err := &ValidationError{
		Parameter: "name",
		Kind:      ErrorKindRequired,
		Message:   "This parameter is required",
	}

	if err.Error() != "name: This parameter is required" {
		t.Errorf("Error() = %s, want 'name: This parameter is required'", err.Error())
	}
}

func TestValidationError_DetailedMessage(t *testing.T) {
	err := &ValidationError{
		Parameter:     "port",
		Kind:          ErrorKindConstraint,
		Message:       "Value must be between 1 and 65535",
		Expected:      "1-65535",
		Got:           "70000",
		Suggestion:    "Use a valid port number",
		Examples:      []string{"80", "443", "8080"},
		Documentation: "https://docs.example.com/ports",
	}

	detailed := err.DetailedMessage()

	expectedParts := []string{
		"port",
		"Value must be between 1 and 65535",
		"Expected: 1-65535",
		"Got: 70000",
		"Suggestion: Use a valid port number",
		"Examples: 80, 443, 8080",
		"See: https://docs.example.com/ports",
	}

	for _, part := range expectedParts {
		if !strings.Contains(detailed, part) {
			t.Errorf("DetailedMessage missing: %s", part)
		}
	}
}

func TestNewBlueprintValidationError(t *testing.T) {
	errs := NewBlueprintValidationError()

	if errs.HasErrors() {
		t.Error("New BlueprintValidationError should have no errors")
	}
	if errs.Count() != 0 {
		t.Errorf("Count() = %d, want 0", errs.Count())
	}
}

func TestBlueprintValidationError_Add(t *testing.T) {
	errs := NewBlueprintValidationError()

	err := &ValidationError{Parameter: "test", Kind: ErrorKindRequired}
	errs.Add(err)

	if !errs.HasErrors() {
		t.Error("Should have errors after Add")
	}
	if errs.Count() != 1 {
		t.Errorf("Count() = %d, want 1", errs.Count())
	}
}

func TestBlueprintValidationError_AddError(t *testing.T) {
	errs := NewBlueprintValidationError()

	err := errs.AddError("name", ErrorKindRequired, "Name is required")

	if err.Parameter != "name" {
		t.Errorf("Parameter = %s, want name", err.Parameter)
	}
	if err.Kind != ErrorKindRequired {
		t.Errorf("Kind = %s, want required", err.Kind)
	}
	if errs.Count() != 1 {
		t.Errorf("Count() = %d, want 1", errs.Count())
	}
}

func TestBlueprintValidationError_ByKind(t *testing.T) {
	errs := NewBlueprintValidationError()
	errs.AddError("param1", ErrorKindRequired, "msg1")
	errs.AddError("param2", ErrorKindType, "msg2")
	errs.AddError("param3", ErrorKindRequired, "msg3")

	required := errs.ByKind(ErrorKindRequired)
	if len(required) != 2 {
		t.Errorf("ByKind(required) returned %d errors, want 2", len(required))
	}

	typeErrors := errs.ByKind(ErrorKindType)
	if len(typeErrors) != 1 {
		t.Errorf("ByKind(type) returned %d errors, want 1", len(typeErrors))
	}
}

func TestBlueprintValidationError_ByParameter(t *testing.T) {
	errs := NewBlueprintValidationError()
	errs.AddError("config.server.port", ErrorKindConstraint, "msg1")
	errs.AddError("config.server.host", ErrorKindFormat, "msg2")
	errs.AddError("config.database.url", ErrorKindRequired, "msg3")

	serverErrors := errs.ByParameter("config.server")
	if len(serverErrors) != 2 {
		t.Errorf("ByParameter(config.server) returned %d errors, want 2", len(serverErrors))
	}

	dbErrors := errs.ByParameter("config.database")
	if len(dbErrors) != 1 {
		t.Errorf("ByParameter(config.database) returned %d errors, want 1", len(dbErrors))
	}
}

func TestBlueprintValidationError_Error(t *testing.T) {
	errs := NewBlueprintValidationError()

	if errs.Error() != "no validation errors" {
		t.Errorf("Empty errors Error() = %s", errs.Error())
	}

	errs.AddError("param1", ErrorKindRequired, "message1")
	if errs.Error() != "param1: message1" {
		t.Errorf("Single error Error() = %s", errs.Error())
	}

	errs.AddError("param2", ErrorKindType, "message2")
	if !strings.Contains(errs.Error(), "2 validation errors") {
		t.Errorf("Multiple errors Error() = %s", errs.Error())
	}
}

func TestBlueprintValidationError_Format(t *testing.T) {
	errs := NewBlueprintValidationError()
	errs.Add(&ValidationError{
		Parameter:  "name",
		Kind:       ErrorKindRequired,
		Message:    "Name is required",
		Suggestion: "Provide a name value",
	})
	errs.Add(&ValidationError{
		Parameter: "port",
		Kind:      ErrorKindConstraint,
		Message:   "Port must be between 1 and 65535",
	})

	output := errs.Format()

	expectedParts := []string{
		"2 validation error(s)",
		"Missing Required Parameters (1)",
		"name: Name is required",
		"Provide a name value",
		"Constraint Violations (1)",
		"port: Port must be between 1 and 65535",
	}

	for _, part := range expectedParts {
		if !strings.Contains(output, part) {
			t.Errorf("Format output missing: %s", part)
		}
	}
}

func TestBlueprintValidationError_FormatCompact(t *testing.T) {
	errs := NewBlueprintValidationError()

	if errs.FormatCompact() != "OK" {
		t.Error("Empty errors should format as 'OK'")
	}

	errs.AddError("param1", ErrorKindRequired, "msg1")
	errs.AddError("param2", ErrorKindType, "msg2")

	compact := errs.FormatCompact()
	if !strings.Contains(compact, "2 parameter(s)") {
		t.Errorf("FormatCompact() = %s", compact)
	}
	if !strings.Contains(compact, "param1") || !strings.Contains(compact, "param2") {
		t.Error("FormatCompact should list parameter names")
	}
}

func TestValidationErrorBuilder(t *testing.T) {
	err := NewValidationError("config.port", ErrorKindConstraint).
		Message("Port is out of range").
		Value(70000).
		Expected("1-65535").
		Got("70000").
		Suggestion("Use a valid port number").
		Examples("80", "443", "8080").
		Documentation("https://docs.example.com").
		Related("config.host").
		Context("min", 1).
		Build()

	if err.Parameter != "config.port" {
		t.Errorf("Parameter = %s", err.Parameter)
	}
	if err.Kind != ErrorKindConstraint {
		t.Errorf("Kind = %s", err.Kind)
	}
	if err.Message != "Port is out of range" {
		t.Errorf("Message = %s", err.Message)
	}
	if err.Value != 70000 {
		t.Errorf("Value = %v", err.Value)
	}
	if err.Expected != "1-65535" {
		t.Errorf("Expected = %s", err.Expected)
	}
	if err.Got != "70000" {
		t.Errorf("Got = %s", err.Got)
	}
	if err.Suggestion != "Use a valid port number" {
		t.Errorf("Suggestion = %s", err.Suggestion)
	}
	if len(err.Examples) != 3 {
		t.Errorf("Examples count = %d", len(err.Examples))
	}
	if err.Documentation != "https://docs.example.com" {
		t.Errorf("Documentation = %s", err.Documentation)
	}
	if len(err.Related) != 1 || err.Related[0] != "config.host" {
		t.Errorf("Related = %v", err.Related)
	}
	if err.Context["min"] != 1 {
		t.Errorf("Context[min] = %v", err.Context["min"])
	}
}

func TestValidationErrorBuilder_Messagef(t *testing.T) {
	err := NewValidationError("test", ErrorKindType).
		Messagef("Expected %s but got %s", "string", "integer").
		Build()

	if err.Message != "Expected string but got integer" {
		t.Errorf("Message = %s", err.Message)
	}
}

func TestRequiredError(t *testing.T) {
	err := RequiredError("username")

	if err.Parameter != "username" {
		t.Errorf("Parameter = %s", err.Parameter)
	}
	if err.Kind != ErrorKindRequired {
		t.Errorf("Kind = %s", err.Kind)
	}
	if !strings.Contains(err.Message, "required") {
		t.Errorf("Message should mention 'required': %s", err.Message)
	}
	if err.Suggestion == "" {
		t.Error("Should have suggestion")
	}
}

func TestRequiredErrorWithDefault(t *testing.T) {
	err := RequiredErrorWithDefault("timeout", 30)

	if !strings.Contains(err.Suggestion, "30") {
		t.Errorf("Suggestion should mention default value: %s", err.Suggestion)
	}
}

func TestTypeError(t *testing.T) {
	tests := []struct {
		expected string
		got      string
	}{
		{"string", "integer"},
		{"integer", "string"},
		{"number", "boolean"},
		{"boolean", "string"},
		{"array", "object"},
		{"object", "array"},
	}

	for _, tt := range tests {
		err := TypeError("param", tt.expected, tt.got)

		if err.Kind != ErrorKindType {
			t.Errorf("Kind = %s, want type", err.Kind)
		}
		if !strings.Contains(err.Message, tt.expected) {
			t.Errorf("Message should mention expected type: %s", err.Message)
		}
		if !strings.Contains(err.Message, tt.got) {
			t.Errorf("Message should mention got type: %s", err.Message)
		}
		if err.Suggestion == "" {
			t.Errorf("TypeError for %s should have suggestion", tt.expected)
		}
	}
}

func TestFormatError(t *testing.T) {
	err := FormatError("email", "email", "invalid", "missing @ symbol")

	if err.Kind != ErrorKindFormat {
		t.Errorf("Kind = %s", err.Kind)
	}
	if !strings.Contains(err.Message, "email") {
		t.Error("Message should mention format")
	}
	if len(err.Examples) == 0 {
		t.Error("Should have examples for known format")
	}
}

func TestFormatExamples(t *testing.T) {
	formats := []string{
		"hostname", "uri", "url", "email", "ipv4", "ipv6",
		"cidr", "date-time", "date", "time", "uuid", "port", "semver", "dns-name",
	}

	for _, format := range formats {
		examples := formatExamples(format)
		if len(examples) == 0 {
			t.Errorf("No examples for format: %s", format)
		}
	}

	// Unknown format should return nil
	if examples := formatExamples("unknown"); examples != nil {
		t.Error("Unknown format should return nil examples")
	}
}

func TestRangeError(t *testing.T) {
	// Both min and max
	err := RangeError("port", 70000, 1, 65535)
	if !strings.Contains(err.Message, "70000") {
		t.Error("Should mention value")
	}
	if !strings.Contains(err.Message, "range") {
		t.Error("Should mention range")
	}

	// Only min
	err = RangeError("age", 0, 1, nil)
	if !strings.Contains(err.Message, "below") || !strings.Contains(err.Message, "minimum") {
		t.Errorf("Min-only error message: %s", err.Message)
	}

	// Only max
	err = RangeError("count", 100, nil, 50)
	if !strings.Contains(err.Message, "exceeds") || !strings.Contains(err.Message, "maximum") {
		t.Errorf("Max-only error message: %s", err.Message)
	}
}

func TestLengthError(t *testing.T) {
	minLen := 5
	maxLen := 10

	// Both min and max
	err := LengthError("name", 3, &minLen, &maxLen)
	if !strings.Contains(err.Message, "3") {
		t.Error("Should mention actual length")
	}

	// Only min
	err = LengthError("name", 2, &minLen, nil)
	if !strings.Contains(err.Message, "below") {
		t.Errorf("Min-only length error: %s", err.Message)
	}

	// Only max
	err = LengthError("name", 15, nil, &maxLen)
	if !strings.Contains(err.Message, "exceeds") {
		t.Errorf("Max-only length error: %s", err.Message)
	}
}

func TestEnumError(t *testing.T) {
	allowed := []interface{}{"debug", "info", "warn", "error"}
	err := EnumError("log_level", "trace", allowed)

	if err.Kind != ErrorKindEnum {
		t.Errorf("Kind = %s", err.Kind)
	}
	if !strings.Contains(err.Message, "trace") {
		t.Error("Should mention invalid value")
	}
	if !strings.Contains(err.Expected, "debug") {
		t.Error("Should list allowed values")
	}
	if len(err.Examples) != 4 {
		t.Errorf("Examples count = %d", len(err.Examples))
	}
}

func TestPatternError(t *testing.T) {
	err := PatternError("id", "abc123", "^[A-Z]+$")

	if err.Kind != ErrorKindPattern {
		t.Errorf("Kind = %s", err.Kind)
	}
	if err.Context["pattern"] != "^[A-Z]+$" {
		t.Errorf("Pattern not in context: %v", err.Context)
	}
}

func TestDependencyError(t *testing.T) {
	err := DependencyError("ssl_cert", "ssl_enabled")

	if err.Kind != ErrorKindDependency {
		t.Errorf("Kind = %s", err.Kind)
	}
	if !strings.Contains(err.Message, "ssl_enabled") {
		t.Error("Should mention dependency")
	}
	if len(err.Related) != 1 || err.Related[0] != "ssl_enabled" {
		t.Errorf("Related = %v", err.Related)
	}
}

func TestConflictError(t *testing.T) {
	err := ConflictError("password", "oauth_token")

	if err.Kind != ErrorKindConflict {
		t.Errorf("Kind = %s", err.Kind)
	}
	if !strings.Contains(err.Message, "oauth_token") {
		t.Error("Should mention conflicting param")
	}
	if len(err.Related) != 1 || err.Related[0] != "oauth_token" {
		t.Errorf("Related = %v", err.Related)
	}
}

func TestUnknownParameterError(t *testing.T) {
	validParams := []string{"name", "email", "password", "username"}

	// Similar parameter should suggest alternatives
	err := UnknownParameterError("naem", validParams)
	if !strings.Contains(err.Suggestion, "name") {
		t.Errorf("Should suggest similar: %s", err.Suggestion)
	}

	// No similar params
	err = UnknownParameterError("xyz123", validParams)
	if err.Suggestion == "" {
		t.Error("Should still have a suggestion listing valid params")
	}
}

func TestLevenshteinDistance(t *testing.T) {
	tests := []struct {
		s1       string
		s2       string
		expected int
	}{
		{"", "", 0},
		{"a", "", 1},
		{"", "a", 1},
		{"abc", "abc", 0},
		{"abc", "ab", 1},
		{"abc", "abd", 1},
		{"abc", "adc", 1},
		{"kitten", "sitting", 3},
	}

	for _, tt := range tests {
		dist := levenshteinDistance(tt.s1, tt.s2)
		if dist != tt.expected {
			t.Errorf("levenshteinDistance(%q, %q) = %d, want %d", tt.s1, tt.s2, dist, tt.expected)
		}
	}
}

func TestFindSimilarStrings(t *testing.T) {
	candidates := []string{"name", "email", "password", "username", "timeout"}

	// Exact match should be first
	similar := findSimilarStrings("name", candidates, 3)
	if len(similar) == 0 || similar[0] != "name" {
		t.Errorf("Exact match should be first: %v", similar)
	}

	// Typo should find similar
	similar = findSimilarStrings("naem", candidates, 3)
	if len(similar) == 0 {
		t.Error("Should find similar for typo")
	}
	found := false
	for _, s := range similar {
		if s == "name" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("Similar should include 'name': %v", similar)
	}

	// Very different string should return fewer/no matches
	similar = findSimilarStrings("xyzabc123", candidates, 3)
	if len(similar) > 0 {
		t.Errorf("Very different string should have no similar: %v", similar)
	}
}

func TestKindPriority(t *testing.T) {
	// Required errors should have highest priority (lowest number)
	if kindPriority(ErrorKindRequired) >= kindPriority(ErrorKindType) {
		t.Error("Required should have higher priority than Type")
	}
	if kindPriority(ErrorKindType) >= kindPriority(ErrorKindFormat) {
		t.Error("Type should have higher priority than Format")
	}
}

func TestKindLabel(t *testing.T) {
	labels := map[ValidationErrorKind]string{
		ErrorKindRequired:   "Missing Required Parameters",
		ErrorKindType:       "Type Mismatches",
		ErrorKindFormat:     "Format Errors",
		ErrorKindConstraint: "Constraint Violations",
		ErrorKindEnum:       "Invalid Enum Values",
		ErrorKindPattern:    "Pattern Mismatches",
		ErrorKindDependency: "Dependency Errors",
		ErrorKindConflict:   "Conflicting Parameters",
		ErrorKindUnknown:    "Unknown Parameters",
	}

	for kind, expected := range labels {
		if kindLabel(kind) != expected {
			t.Errorf("kindLabel(%s) = %s, want %s", kind, kindLabel(kind), expected)
		}
	}

	// Unknown kind should return string representation
	unknownKind := ValidationErrorKind("custom")
	if kindLabel(unknownKind) != "custom" {
		t.Errorf("Unknown kind should return string: %s", kindLabel(unknownKind))
	}
}
