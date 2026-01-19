package blueprint

import (
	"fmt"
	"sort"
	"strings"
)

// ValidationErrorKind categorizes validation errors
type ValidationErrorKind string

const (
	// ErrorKindRequired indicates a required parameter is missing
	ErrorKindRequired ValidationErrorKind = "required"
	// ErrorKindType indicates a type mismatch
	ErrorKindType ValidationErrorKind = "type"
	// ErrorKindFormat indicates a format validation failure
	ErrorKindFormat ValidationErrorKind = "format"
	// ErrorKindConstraint indicates a constraint violation
	ErrorKindConstraint ValidationErrorKind = "constraint"
	// ErrorKindEnum indicates a value not in allowed enum
	ErrorKindEnum ValidationErrorKind = "enum"
	// ErrorKindPattern indicates a regex pattern mismatch
	ErrorKindPattern ValidationErrorKind = "pattern"
	// ErrorKindDependency indicates a dependency requirement failure
	ErrorKindDependency ValidationErrorKind = "dependency"
	// ErrorKindConflict indicates conflicting parameter values
	ErrorKindConflict ValidationErrorKind = "conflict"
	// ErrorKindUnknown indicates an unknown parameter
	ErrorKindUnknown ValidationErrorKind = "unknown"
)

// ValidationError represents a detailed validation error
type ValidationError struct {
	// Parameter is the parameter name (may include path for nested params)
	Parameter string `json:"parameter"`

	// Kind categorizes the error
	Kind ValidationErrorKind `json:"kind"`

	// Message is the human-readable error message
	Message string `json:"message"`

	// Value is the problematic value (may be redacted for sensitive params)
	Value interface{} `json:"value,omitempty"`

	// Expected describes what was expected
	Expected string `json:"expected,omitempty"`

	// Got describes what was received
	Got string `json:"got,omitempty"`

	// Suggestion provides actionable fix advice
	Suggestion string `json:"suggestion,omitempty"`

	// Examples provides example valid values
	Examples []string `json:"examples,omitempty"`

	// Documentation links to relevant docs
	Documentation string `json:"documentation,omitempty"`

	// Related lists related parameters (for dependency/conflict errors)
	Related []string `json:"related,omitempty"`

	// Context provides additional context
	Context map[string]interface{} `json:"context,omitempty"`
}

// Error implements the error interface
func (e *ValidationError) Error() string {
	return e.Message
}

// DetailedMessage returns a multi-line detailed error message
func (e *ValidationError) DetailedMessage() string {
	var sb strings.Builder

	sb.WriteString(fmt.Sprintf("Parameter '%s': %s\n", e.Parameter, e.Message))

	if e.Expected != "" {
		sb.WriteString(fmt.Sprintf("  Expected: %s\n", e.Expected))
	}
	if e.Got != "" {
		sb.WriteString(fmt.Sprintf("  Got: %s\n", e.Got))
	}
	if e.Suggestion != "" {
		sb.WriteString(fmt.Sprintf("  Suggestion: %s\n", e.Suggestion))
	}
	if len(e.Examples) > 0 {
		sb.WriteString(fmt.Sprintf("  Examples: %s\n", strings.Join(e.Examples, ", ")))
	}
	if e.Documentation != "" {
		sb.WriteString(fmt.Sprintf("  See: %s\n", e.Documentation))
	}

	return sb.String()
}

// ValidationErrors collects multiple validation errors
type ValidationErrors struct {
	Errors []*ValidationError
}

// NewValidationErrors creates a new ValidationErrors collection
func NewValidationErrors() *ValidationErrors {
	return &ValidationErrors{
		Errors: make([]*ValidationError, 0),
	}
}

// Add adds a validation error
func (v *ValidationErrors) Add(err *ValidationError) {
	v.Errors = append(v.Errors, err)
}

// AddError adds an error with basic info
func (v *ValidationErrors) AddError(param string, kind ValidationErrorKind, message string) *ValidationError {
	err := &ValidationError{
		Parameter: param,
		Kind:      kind,
		Message:   message,
	}
	v.Add(err)
	return err
}

// HasErrors returns true if there are any errors
func (v *ValidationErrors) HasErrors() bool {
	return len(v.Errors) > 0
}

// Count returns the number of errors
func (v *ValidationErrors) Count() int {
	return len(v.Errors)
}

// ByKind returns errors filtered by kind
func (v *ValidationErrors) ByKind(kind ValidationErrorKind) []*ValidationError {
	var result []*ValidationError
	for _, err := range v.Errors {
		if err.Kind == kind {
			result = append(result, err)
		}
	}
	return result
}

// ByParameter returns errors filtered by parameter
func (v *ValidationErrors) ByParameter(param string) []*ValidationError {
	var result []*ValidationError
	for _, err := range v.Errors {
		if err.Parameter == param || strings.HasPrefix(err.Parameter, param+".") {
			result = append(result, err)
		}
	}
	return result
}

// Error implements the error interface
func (v *ValidationErrors) Error() string {
	if len(v.Errors) == 0 {
		return "no validation errors"
	}
	if len(v.Errors) == 1 {
		return v.Errors[0].Error()
	}
	return fmt.Sprintf("%d validation errors", len(v.Errors))
}

// Format returns a formatted error summary
func (v *ValidationErrors) Format() string {
	if len(v.Errors) == 0 {
		return "No validation errors"
	}

	var sb strings.Builder

	sb.WriteString(fmt.Sprintf("Found %d validation error(s):\n\n", len(v.Errors)))

	// Group by kind for better organization
	byKind := make(map[ValidationErrorKind][]*ValidationError)
	for _, err := range v.Errors {
		byKind[err.Kind] = append(byKind[err.Kind], err)
	}

	// Sort kinds for consistent output
	kinds := make([]ValidationErrorKind, 0, len(byKind))
	for k := range byKind {
		kinds = append(kinds, k)
	}
	sort.Slice(kinds, func(i, j int) bool { return kindPriority(kinds[i]) < kindPriority(kinds[j]) })

	for _, kind := range kinds {
		errors := byKind[kind]
		sb.WriteString(fmt.Sprintf("%s (%d):\n", kindLabel(kind), len(errors)))
		for _, err := range errors {
			sb.WriteString(fmt.Sprintf("  • %s: %s\n", err.Parameter, err.Message))
			if err.Suggestion != "" {
				sb.WriteString(fmt.Sprintf("    → %s\n", err.Suggestion))
			}
		}
		sb.WriteString("\n")
	}

	return sb.String()
}

// FormatCompact returns a compact single-line summary
func (v *ValidationErrors) FormatCompact() string {
	if len(v.Errors) == 0 {
		return "OK"
	}

	params := make([]string, 0, len(v.Errors))
	for _, err := range v.Errors {
		params = append(params, err.Parameter)
	}

	return fmt.Sprintf("validation failed for %d parameter(s): %s", len(v.Errors), strings.Join(params, ", "))
}

// kindPriority returns the display priority of an error kind (lower = higher priority)
func kindPriority(kind ValidationErrorKind) int {
	priorities := map[ValidationErrorKind]int{
		ErrorKindRequired:   1,
		ErrorKindType:       2,
		ErrorKindFormat:     3,
		ErrorKindConstraint: 4,
		ErrorKindEnum:       5,
		ErrorKindPattern:    6,
		ErrorKindDependency: 7,
		ErrorKindConflict:   8,
		ErrorKindUnknown:    9,
	}
	if p, ok := priorities[kind]; ok {
		return p
	}
	return 99
}

// kindLabel returns a human-readable label for an error kind
func kindLabel(kind ValidationErrorKind) string {
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
	if label, ok := labels[kind]; ok {
		return label
	}
	return string(kind)
}

// ValidationErrorBuilder helps construct validation errors with context
type ValidationErrorBuilder struct {
	err *ValidationError
}

// NewValidationError creates a new validation error builder
func NewValidationError(param string, kind ValidationErrorKind) *ValidationErrorBuilder {
	return &ValidationErrorBuilder{
		err: &ValidationError{
			Parameter: param,
			Kind:      kind,
		},
	}
}

// Message sets the error message
func (b *ValidationErrorBuilder) Message(msg string) *ValidationErrorBuilder {
	b.err.Message = msg
	return b
}

// Messagef sets the error message with formatting
func (b *ValidationErrorBuilder) Messagef(format string, args ...interface{}) *ValidationErrorBuilder {
	b.err.Message = fmt.Sprintf(format, args...)
	return b
}

// Value sets the problematic value
func (b *ValidationErrorBuilder) Value(val interface{}) *ValidationErrorBuilder {
	b.err.Value = val
	return b
}

// Expected sets what was expected
func (b *ValidationErrorBuilder) Expected(exp string) *ValidationErrorBuilder {
	b.err.Expected = exp
	return b
}

// Got sets what was received
func (b *ValidationErrorBuilder) Got(got string) *ValidationErrorBuilder {
	b.err.Got = got
	return b
}

// Suggestion sets actionable fix advice
func (b *ValidationErrorBuilder) Suggestion(sug string) *ValidationErrorBuilder {
	b.err.Suggestion = sug
	return b
}

// Examples sets example valid values
func (b *ValidationErrorBuilder) Examples(examples ...string) *ValidationErrorBuilder {
	b.err.Examples = examples
	return b
}

// Documentation sets the documentation link
func (b *ValidationErrorBuilder) Documentation(doc string) *ValidationErrorBuilder {
	b.err.Documentation = doc
	return b
}

// Related sets related parameters
func (b *ValidationErrorBuilder) Related(params ...string) *ValidationErrorBuilder {
	b.err.Related = params
	return b
}

// Context adds context information
func (b *ValidationErrorBuilder) Context(key string, value interface{}) *ValidationErrorBuilder {
	if b.err.Context == nil {
		b.err.Context = make(map[string]interface{})
	}
	b.err.Context[key] = value
	return b
}

// Build returns the constructed error
func (b *ValidationErrorBuilder) Build() *ValidationError {
	return b.err
}

// Common error constructors for convenience

// RequiredError creates an error for a missing required parameter
func RequiredError(param string) *ValidationError {
	return NewValidationError(param, ErrorKindRequired).
		Message("This parameter is required but was not provided").
		Suggestion("Provide a value for this parameter").
		Build()
}

// RequiredErrorWithDefault creates an error suggesting a default value
func RequiredErrorWithDefault(param string, defaultValue interface{}) *ValidationError {
	return NewValidationError(param, ErrorKindRequired).
		Messagef("This parameter is required but was not provided").
		Suggestion(fmt.Sprintf("Provide a value, or use the default: %v", defaultValue)).
		Build()
}

// TypeError creates an error for a type mismatch
func TypeError(param string, expected, got string) *ValidationError {
	suggestion := ""
	switch expected {
	case "string":
		suggestion = "Wrap the value in quotes"
	case "integer":
		suggestion = "Use a whole number without quotes"
	case "number":
		suggestion = "Use a numeric value"
	case "boolean":
		suggestion = "Use true or false"
	case "array":
		suggestion = "Use [value1, value2] syntax"
	case "object":
		suggestion = "Use {key: value} syntax"
	}

	return NewValidationError(param, ErrorKindType).
		Messagef("Expected %s but got %s", expected, got).
		Expected(expected).
		Got(got).
		Suggestion(suggestion).
		Build()
}

// FormatError creates an error for format validation failure
func FormatError(param, format, value, reason string) *ValidationError {
	examples := formatExamples(format)

	return NewValidationError(param, ErrorKindFormat).
		Messagef("Value does not match expected %s format: %s", format, reason).
		Value(value).
		Expected(format).
		Suggestion(fmt.Sprintf("Ensure the value is a valid %s", format)).
		Examples(examples...).
		Build()
}

// formatExamples returns example values for common formats
func formatExamples(format string) []string {
	examples := map[string][]string{
		"hostname":  {"example.com", "server-01.internal"},
		"uri":       {"https://example.com/path", "file:///local/path"},
		"url":       {"https://example.com", "http://localhost:8080"},
		"email":     {"user@example.com", "admin@company.org"},
		"ipv4":      {"192.168.1.1", "10.0.0.1"},
		"ipv6":      {"2001:0db8:85a3::8a2e:0370:7334", "::1"},
		"cidr":      {"192.168.0.0/24", "10.0.0.0/8"},
		"date-time": {"2024-01-15T14:30:00Z", "2024-01-15T14:30:00+05:00"},
		"date":      {"2024-01-15", "2024-12-31"},
		"time":      {"14:30:00", "09:00"},
		"uuid":      {"550e8400-e29b-41d4-a716-446655440000"},
		"port":      {"80", "443", "8080"},
		"semver":    {"1.0.0", "2.1.3-beta.1"},
		"dns-name":  {"example.com", "_dmarc.example.com"},
	}
	if ex, ok := examples[format]; ok {
		return ex
	}
	return nil
}

// RangeError creates an error for a value outside allowed range
func RangeError(param string, value, min, max interface{}) *ValidationError {
	builder := NewValidationError(param, ErrorKindConstraint).
		Value(value)

	if min != nil && max != nil {
		builder.Messagef("Value %v is outside allowed range [%v, %v]", value, min, max).
			Expected(fmt.Sprintf("between %v and %v", min, max)).
			Suggestion(fmt.Sprintf("Use a value between %v and %v", min, max))
	} else if min != nil {
		builder.Messagef("Value %v is below minimum %v", value, min).
			Expected(fmt.Sprintf("at least %v", min)).
			Suggestion(fmt.Sprintf("Use a value of at least %v", min))
	} else if max != nil {
		builder.Messagef("Value %v exceeds maximum %v", value, max).
			Expected(fmt.Sprintf("at most %v", max)).
			Suggestion(fmt.Sprintf("Use a value of at most %v", max))
	}

	return builder.Build()
}

// LengthError creates an error for string/array length violations
func LengthError(param string, actualLen int, minLen, maxLen *int) *ValidationError {
	builder := NewValidationError(param, ErrorKindConstraint).
		Context("actual_length", actualLen)

	if minLen != nil && maxLen != nil {
		builder.Messagef("Length %d is outside allowed range [%d, %d]", actualLen, *minLen, *maxLen).
			Expected(fmt.Sprintf("between %d and %d characters", *minLen, *maxLen)).
			Suggestion(fmt.Sprintf("Adjust length to between %d and %d", *minLen, *maxLen))
	} else if minLen != nil {
		builder.Messagef("Length %d is below minimum of %d", actualLen, *minLen).
			Expected(fmt.Sprintf("at least %d characters", *minLen)).
			Suggestion(fmt.Sprintf("Provide at least %d characters", *minLen))
	} else if maxLen != nil {
		builder.Messagef("Length %d exceeds maximum of %d", actualLen, *maxLen).
			Expected(fmt.Sprintf("at most %d characters", *maxLen)).
			Suggestion(fmt.Sprintf("Reduce to at most %d characters", *maxLen))
	}

	return builder.Build()
}

// EnumError creates an error for an invalid enum value
func EnumError(param string, value interface{}, allowed []interface{}) *ValidationError {
	// Format allowed values nicely
	allowedStrs := make([]string, len(allowed))
	for i, v := range allowed {
		allowedStrs[i] = fmt.Sprintf("%v", v)
	}

	return NewValidationError(param, ErrorKindEnum).
		Messagef("Value '%v' is not one of the allowed values", value).
		Value(value).
		Expected(fmt.Sprintf("one of: %s", strings.Join(allowedStrs, ", "))).
		Got(fmt.Sprintf("%v", value)).
		Suggestion("Choose one of the allowed values").
		Examples(allowedStrs...).
		Build()
}

// PatternError creates an error for pattern mismatch
func PatternError(param, value, pattern string) *ValidationError {
	return NewValidationError(param, ErrorKindPattern).
		Messagef("Value does not match required pattern").
		Value(value).
		Expected(fmt.Sprintf("matching pattern: %s", pattern)).
		Got(value).
		Suggestion("Ensure the value matches the expected pattern").
		Context("pattern", pattern).
		Build()
}

// DependencyError creates an error for unmet parameter dependencies
func DependencyError(param, dependsOn string) *ValidationError {
	return NewValidationError(param, ErrorKindDependency).
		Messagef("This parameter requires '%s' to also be set", dependsOn).
		Related(dependsOn).
		Suggestion(fmt.Sprintf("Either set '%s' or remove '%s'", dependsOn, param)).
		Build()
}

// ConflictError creates an error for conflicting parameters
func ConflictError(param1, param2 string) *ValidationError {
	return NewValidationError(param1, ErrorKindConflict).
		Messagef("Cannot be used together with '%s'", param2).
		Related(param2).
		Suggestion(fmt.Sprintf("Use either '%s' or '%s', but not both", param1, param2)).
		Build()
}

// UnknownParameterError creates an error for unknown parameters
func UnknownParameterError(param string, validParams []string) *ValidationError {
	builder := NewValidationError(param, ErrorKindUnknown).
		Messagef("Unknown parameter '%s'", param)

	// Find similar parameter names for suggestions
	similar := findSimilarStrings(param, validParams, 3)
	if len(similar) > 0 {
		builder.Suggestion(fmt.Sprintf("Did you mean: %s?", strings.Join(similar, ", ")))
	} else if len(validParams) > 0 && len(validParams) <= 10 {
		builder.Suggestion(fmt.Sprintf("Valid parameters are: %s", strings.Join(validParams, ", ")))
	}

	return builder.Build()
}

// findSimilarStrings finds strings similar to the target using Levenshtein distance
func findSimilarStrings(target string, candidates []string, maxResults int) []string {
	type scored struct {
		value    string
		distance int
	}

	var scored_candidates []scored
	target = strings.ToLower(target)

	for _, c := range candidates {
		dist := levenshteinDistance(target, strings.ToLower(c))
		// Only include if distance is reasonably small (less than half the length)
		if dist <= len(target)/2+2 {
			scored_candidates = append(scored_candidates, scored{c, dist})
		}
	}

	// Sort by distance
	sort.Slice(scored_candidates, func(i, j int) bool {
		return scored_candidates[i].distance < scored_candidates[j].distance
	})

	// Return top results
	result := make([]string, 0, maxResults)
	for i := 0; i < len(scored_candidates) && i < maxResults; i++ {
		result = append(result, scored_candidates[i].value)
	}

	return result
}

// levenshteinDistance calculates the edit distance between two strings
func levenshteinDistance(s1, s2 string) int {
	if len(s1) == 0 {
		return len(s2)
	}
	if len(s2) == 0 {
		return len(s1)
	}

	// Create matrix
	matrix := make([][]int, len(s1)+1)
	for i := range matrix {
		matrix[i] = make([]int, len(s2)+1)
		matrix[i][0] = i
	}
	for j := range matrix[0] {
		matrix[0][j] = j
	}

	// Fill matrix
	for i := 1; i <= len(s1); i++ {
		for j := 1; j <= len(s2); j++ {
			cost := 1
			if s1[i-1] == s2[j-1] {
				cost = 0
			}
			matrix[i][j] = min(
				matrix[i-1][j]+1,      // deletion
				matrix[i][j-1]+1,      // insertion
				matrix[i-1][j-1]+cost, // substitution
			)
		}
	}

	return matrix[len(s1)][len(s2)]
}

func min(a, b, c int) int {
	if a < b {
		if a < c {
			return a
		}
		return c
	}
	if b < c {
		return b
	}
	return c
}
