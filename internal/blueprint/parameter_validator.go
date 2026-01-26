package blueprint

import (
	"fmt"
	"net"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// ParameterValidator provides enhanced parameter validation including
// format validators, type coercion, and sensitive parameter handling.
type ParameterValidator struct {
	// formatValidators maps format names to validator functions
	formatValidators map[string]FormatValidator

	// coercionEnabled enables automatic type coercion
	coercionEnabled bool

	// maskSensitive enables masking of sensitive parameters in errors
	maskSensitive bool
}

// FormatValidator validates a string value against a specific format.
type FormatValidator func(value string) error

// NewParameterValidator creates a new parameter validator with default settings.
func NewParameterValidator() *ParameterValidator {
	v := &ParameterValidator{
		formatValidators: make(map[string]FormatValidator),
		coercionEnabled:  true,
		maskSensitive:    true,
	}

	// Register built-in format validators
	v.RegisterFormatValidator("hostname", validateHostname)
	v.RegisterFormatValidator("uri", validateURI)
	v.RegisterFormatValidator("url", validateURI) // alias
	v.RegisterFormatValidator("email", validateEmail)
	v.RegisterFormatValidator("ipv4", validateIPv4)
	v.RegisterFormatValidator("ipv6", validateIPv6)
	v.RegisterFormatValidator("ip", validateIP) // accepts both
	v.RegisterFormatValidator("cidr", validateCIDR)
	v.RegisterFormatValidator("date-time", validateDateTime)
	v.RegisterFormatValidator("datetime", validateDateTime) // alias
	v.RegisterFormatValidator("date", validateDate)
	v.RegisterFormatValidator("time", validateTime)
	v.RegisterFormatValidator("uuid", validateUUID)
	v.RegisterFormatValidator("port", validatePort)
	v.RegisterFormatValidator("semver", validateSemVer)
	v.RegisterFormatValidator("dns-name", validateDNSName)

	return v
}

// RegisterFormatValidator registers a custom format validator.
func (v *ParameterValidator) RegisterFormatValidator(name string, validator FormatValidator) {
	v.formatValidators[name] = validator
}

// SetCoercionEnabled enables or disables type coercion.
func (v *ParameterValidator) SetCoercionEnabled(enabled bool) {
	v.coercionEnabled = enabled
}

// SetMaskSensitive enables or disables sensitive parameter masking in errors.
func (v *ParameterValidator) SetMaskSensitive(enabled bool) {
	v.maskSensitive = enabled
}

// ValidateParameter validates a single parameter value against its schema.
// It returns the validated (and possibly coerced) value and any validation error.
func (v *ParameterValidator) ValidateParameter(name string, schema ParameterSchema, value interface{}, sensitive bool) (interface{}, error) {
	// Handle nil/missing value
	if value == nil {
		if schema.Required {
			return nil, fmt.Errorf("required parameter missing: %s", name)
		}
		if schema.Default != nil {
			return schema.Default, nil
		}
		return nil, nil
	}

	// Type coercion if enabled
	if v.coercionEnabled {
		coerced, err := v.coerceValue(schema.Type, value)
		if err == nil {
			value = coerced
		}
		// If coercion fails, continue with original value and let type validation catch it
	}

	// Type validation
	if err := v.validateType(name, schema.Type, value, sensitive); err != nil {
		return nil, err
	}

	// Format validation for strings
	if schema.Type == "string" && schema.Format != "" {
		strVal, _ := value.(string)
		if err := v.validateFormat(name, schema.Format, strVal, sensitive); err != nil {
			return nil, err
		}
	}

	// Enum validation
	if len(schema.Enum) > 0 {
		if !containsValue(schema.Enum, value) {
			displayValue := v.maskValue(value, sensitive)
			return nil, fmt.Errorf("parameter %s: value %v not in allowed values %v", name, displayValue, schema.Enum)
		}
	}

	// String constraints
	if schema.Type == "string" {
		strVal, _ := value.(string)
		if schema.Pattern != "" {
			if matched, _ := regexp.MatchString(schema.Pattern, strVal); !matched {
				displayValue := v.maskValue(strVal, sensitive)
				return nil, fmt.Errorf("parameter %s: value %v does not match pattern %s", name, displayValue, schema.Pattern)
			}
		}
		if schema.MinLength != nil && len(strVal) < *schema.MinLength {
			return nil, fmt.Errorf("parameter %s: string length %d is less than minimum %d", name, len(strVal), *schema.MinLength)
		}
		if schema.MaxLength != nil && len(strVal) > *schema.MaxLength {
			return nil, fmt.Errorf("parameter %s: string length %d exceeds maximum %d", name, len(strVal), *schema.MaxLength)
		}
	}

	// Numeric constraints
	if schema.Type == "integer" || schema.Type == "number" {
		numVal := toFloat64(value)
		if schema.Minimum != nil && numVal < *schema.Minimum {
			return nil, fmt.Errorf("parameter %s: value %v is less than minimum %v", name, value, *schema.Minimum)
		}
		if schema.Maximum != nil && numVal > *schema.Maximum {
			return nil, fmt.Errorf("parameter %s: value %v exceeds maximum %v", name, value, *schema.Maximum)
		}
	}

	// Array constraints
	if schema.Type == "array" {
		arrVal, ok := value.([]interface{})
		if ok {
			if schema.MinItems != nil && len(arrVal) < *schema.MinItems {
				return nil, fmt.Errorf("parameter %s: array length %d is less than minimum %d", name, len(arrVal), *schema.MinItems)
			}
			if schema.MaxItems != nil && len(arrVal) > *schema.MaxItems {
				return nil, fmt.Errorf("parameter %s: array length %d exceeds maximum %d", name, len(arrVal), *schema.MaxItems)
			}
			// Validate array items if item schema provided
			if schema.Items != nil {
				for i, item := range arrVal {
					itemName := fmt.Sprintf("%s[%d]", name, i)
					validated, err := v.ValidateParameter(itemName, *schema.Items, item, sensitive)
					if err != nil {
						return nil, err
					}
					arrVal[i] = validated
				}
			}
		}
	}

	// Object validation
	if schema.Type == "object" && schema.Properties != nil {
		objVal, ok := value.(map[string]interface{})
		if ok {
			for propName, propSchema := range schema.Properties {
				propFullName := name + "." + propName
				propValue := objVal[propName]
				validated, err := v.ValidateParameter(propFullName, propSchema, propValue, sensitive || propSchema.Sensitive)
				if err != nil {
					return nil, err
				}
				if validated != nil {
					objVal[propName] = validated
				}
			}
		}
	}

	return value, nil
}

// ValidateParameters validates all parameters against their schemas.
func (v *ParameterValidator) ValidateParameters(schemas map[string]ParameterSchema, values map[string]interface{}, enabledFeatures []string) (map[string]interface{}, error) {
	result := make(map[string]interface{})

	// First, copy all values
	for k, val := range values {
		result[k] = val
	}

	// Validate each schema
	for name, schema := range schemas {
		// Skip feature-gated parameters if feature not enabled
		if schema.Feature != "" && !containsString(enabledFeatures, schema.Feature) {
			continue
		}

		value := result[name]
		validated, err := v.ValidateParameter(name, schema, value, schema.Sensitive)
		if err != nil {
			return nil, err
		}
		if validated != nil {
			result[name] = validated
		}
	}

	// Second pass: check required_if conditions
	for name, schema := range schemas {
		// Skip feature-gated parameters if feature not enabled
		if schema.Feature != "" && !containsString(enabledFeatures, schema.Feature) {
			continue
		}

		// Check required_if conditions
		if len(schema.RequiredIf) > 0 {
			if err := v.validateRequiredIf(name, schema, result); err != nil {
				return nil, err
			}
		}
	}

	return result, nil
}

// validateRequiredIf checks if a parameter is required based on other parameter values.
// The parameter is required if ANY of the conditions match (OR logic).
func (v *ParameterValidator) validateRequiredIf(name string, schema ParameterSchema, values map[string]interface{}) error {
	// If the parameter already has a value, no need to check
	if values[name] != nil {
		return nil
	}

	// If the parameter has a default, no need to check
	if schema.Default != nil {
		return nil
	}

	// Check each condition (OR logic - any match makes parameter required)
	for _, condition := range schema.RequiredIf {
		if v.conditionMatches(condition, values) {
			// Build a description of the condition for the error message
			condDesc := formatCondition(condition)
			return fmt.Errorf("parameter %s is required when %s", name, condDesc)
		}
	}

	return nil
}

// conditionMatches checks if all key-value pairs in a condition match the current values.
func (v *ParameterValidator) conditionMatches(condition map[string]interface{}, values map[string]interface{}) bool {
	for paramName, expectedValue := range condition {
		actualValue := values[paramName]

		// Compare values - handle type coercion for common cases
		if !valuesEqual(actualValue, expectedValue) {
			return false
		}
	}
	return true
}

// valuesEqual compares two values with type coercion for common cases.
func valuesEqual(a, b interface{}) bool {
	if a == nil && b == nil {
		return true
	}
	if a == nil || b == nil {
		return false
	}

	// Direct comparison
	if a == b {
		return true
	}

	// String comparison
	aStr := fmt.Sprintf("%v", a)
	bStr := fmt.Sprintf("%v", b)
	return aStr == bStr
}

// formatCondition formats a condition map for error messages.
func formatCondition(condition map[string]interface{}) string {
	parts := make([]string, 0, len(condition))
	for k, v := range condition {
		parts = append(parts, fmt.Sprintf("%s=%v", k, v))
	}
	return strings.Join(parts, " and ")
}

// validateType validates that a value matches the expected type.
func (v *ParameterValidator) validateType(name, expectedType string, value interface{}, sensitive bool) error {
	switch expectedType {
	case "string":
		if _, ok := value.(string); !ok {
			return fmt.Errorf("parameter %s: expected string, got %T", name, value)
		}
	case "integer":
		switch value.(type) {
		case int, int32, int64:
			// OK
		case float64:
			// Check if it's a whole number
			f := value.(float64)
			if f != float64(int64(f)) {
				return fmt.Errorf("parameter %s: expected integer, got float", name)
			}
		default:
			return fmt.Errorf("parameter %s: expected integer, got %T", name, value)
		}
	case "number":
		switch value.(type) {
		case int, int32, int64, float32, float64:
			// OK
		default:
			return fmt.Errorf("parameter %s: expected number, got %T", name, value)
		}
	case "boolean":
		if _, ok := value.(bool); !ok {
			return fmt.Errorf("parameter %s: expected boolean, got %T", name, value)
		}
	case "array":
		if _, ok := value.([]interface{}); !ok {
			return fmt.Errorf("parameter %s: expected array, got %T", name, value)
		}
	case "object":
		if _, ok := value.(map[string]interface{}); !ok {
			return fmt.Errorf("parameter %s: expected object, got %T", name, value)
		}
	}
	return nil
}

// validateFormat validates a string value against a named format.
func (v *ParameterValidator) validateFormat(name, format, value string, sensitive bool) error {
	validator, ok := v.formatValidators[format]
	if !ok {
		// Unknown format - skip validation but log warning
		return nil
	}

	if err := validator(value); err != nil {
		displayValue := v.maskValue(value, sensitive)
		return fmt.Errorf("parameter %s: value %v is not a valid %s: %w", name, displayValue, format, err)
	}

	return nil
}

// coerceValue attempts to coerce a value to the target type.
func (v *ParameterValidator) coerceValue(targetType string, value interface{}) (interface{}, error) {
	switch targetType {
	case "string":
		return v.coerceToString(value)
	case "integer":
		return v.coerceToInteger(value)
	case "number":
		return v.coerceToNumber(value)
	case "boolean":
		return v.coerceToBoolean(value)
	case "array":
		return v.coerceToArray(value)
	default:
		// No coercion for object or unknown types
		return value, nil
	}
}

// coerceToString converts a value to string.
func (v *ParameterValidator) coerceToString(value interface{}) (string, error) {
	switch val := value.(type) {
	case string:
		return val, nil
	case int:
		return strconv.Itoa(val), nil
	case int32:
		return strconv.FormatInt(int64(val), 10), nil
	case int64:
		return strconv.FormatInt(val, 10), nil
	case float32:
		return strconv.FormatFloat(float64(val), 'f', -1, 32), nil
	case float64:
		return strconv.FormatFloat(val, 'f', -1, 64), nil
	case bool:
		return strconv.FormatBool(val), nil
	default:
		return "", fmt.Errorf("cannot coerce %T to string", value)
	}
}

// coerceToInteger converts a value to integer.
func (v *ParameterValidator) coerceToInteger(value interface{}) (int64, error) {
	switch val := value.(type) {
	case int:
		return int64(val), nil
	case int32:
		return int64(val), nil
	case int64:
		return val, nil
	case float32:
		return int64(val), nil
	case float64:
		return int64(val), nil
	case string:
		// Try parsing as integer
		if i, err := strconv.ParseInt(val, 10, 64); err == nil {
			return i, nil
		}
		// Try parsing as float and truncating
		if f, err := strconv.ParseFloat(val, 64); err == nil {
			return int64(f), nil
		}
		return 0, fmt.Errorf("cannot parse %q as integer", val)
	case bool:
		if val {
			return 1, nil
		}
		return 0, nil
	default:
		return 0, fmt.Errorf("cannot coerce %T to integer", value)
	}
}

// coerceToNumber converts a value to float64.
func (v *ParameterValidator) coerceToNumber(value interface{}) (float64, error) {
	switch val := value.(type) {
	case int:
		return float64(val), nil
	case int32:
		return float64(val), nil
	case int64:
		return float64(val), nil
	case float32:
		return float64(val), nil
	case float64:
		return val, nil
	case string:
		return strconv.ParseFloat(val, 64)
	case bool:
		if val {
			return 1, nil
		}
		return 0, nil
	default:
		return 0, fmt.Errorf("cannot coerce %T to number", value)
	}
}

// coerceToBoolean converts a value to boolean.
func (v *ParameterValidator) coerceToBoolean(value interface{}) (bool, error) {
	switch val := value.(type) {
	case bool:
		return val, nil
	case int:
		return val != 0, nil
	case int32:
		return val != 0, nil
	case int64:
		return val != 0, nil
	case float32:
		return val != 0, nil
	case float64:
		return val != 0, nil
	case string:
		lower := strings.ToLower(strings.TrimSpace(val))
		switch lower {
		case "true", "yes", "on", "1", "enabled":
			return true, nil
		case "false", "no", "off", "0", "disabled", "":
			return false, nil
		default:
			return false, fmt.Errorf("cannot parse %q as boolean", val)
		}
	default:
		return false, fmt.Errorf("cannot coerce %T to boolean", value)
	}
}

// coerceToArray wraps a non-array value in an array.
func (v *ParameterValidator) coerceToArray(value interface{}) ([]interface{}, error) {
	if arr, ok := value.([]interface{}); ok {
		return arr, nil
	}
	// Wrap single value in array
	return []interface{}{value}, nil
}

// maskValue masks a value for display in error messages if sensitive.
func (v *ParameterValidator) maskValue(value interface{}, sensitive bool) interface{} {
	if !v.maskSensitive || !sensitive {
		return value
	}
	return "***REDACTED***"
}

// Format validators

// validateHostname validates a hostname according to RFC 1123.
func validateHostname(value string) error {
	if len(value) == 0 {
		return fmt.Errorf("hostname cannot be empty")
	}
	if len(value) > 253 {
		return fmt.Errorf("hostname exceeds maximum length of 253 characters")
	}

	// Split into labels
	labels := strings.Split(value, ".")
	for _, label := range labels {
		if len(label) == 0 {
			return fmt.Errorf("hostname contains empty label")
		}
		if len(label) > 63 {
			return fmt.Errorf("label %q exceeds maximum length of 63 characters", label)
		}
		// Must start with alphanumeric
		if !isAlphaNumeric(label[0]) {
			return fmt.Errorf("label %q must start with alphanumeric character", label)
		}
		// Must end with alphanumeric
		if !isAlphaNumeric(label[len(label)-1]) {
			return fmt.Errorf("label %q must end with alphanumeric character", label)
		}
		// Must contain only alphanumeric and hyphens
		for i, c := range label {
			if !isAlphaNumeric(byte(c)) && c != '-' {
				return fmt.Errorf("label %q contains invalid character at position %d", label, i)
			}
		}
	}
	return nil
}

// validateDNSName validates a DNS name (less strict than hostname).
func validateDNSName(value string) error {
	if len(value) == 0 {
		return fmt.Errorf("DNS name cannot be empty")
	}
	if len(value) > 253 {
		return fmt.Errorf("DNS name exceeds maximum length of 253 characters")
	}

	// Allow underscore for SRV records, DMARC, etc.
	// Labels can start with underscore (e.g., _dmarc, _sip)
	pattern := `^[a-zA-Z0-9_]([a-zA-Z0-9\-_]*[a-zA-Z0-9_])?(\.[a-zA-Z0-9_]([a-zA-Z0-9\-_]*[a-zA-Z0-9_])?)*$`
	matched, _ := regexp.MatchString(pattern, value)
	if !matched {
		return fmt.Errorf("invalid DNS name format")
	}
	return nil
}

// validateURI validates a URI according to RFC 3986.
func validateURI(value string) error {
	u, err := url.Parse(value)
	if err != nil {
		return err
	}
	if u.Scheme == "" {
		return fmt.Errorf("URI must have a scheme")
	}
	return nil
}

// validateEmail validates an email address.
func validateEmail(value string) error {
	// Basic email validation - RFC 5322 is very complex
	pattern := `^[a-zA-Z0-9._%+\-]+@[a-zA-Z0-9.\-]+\.[a-zA-Z]{2,}$`
	matched, _ := regexp.MatchString(pattern, value)
	if !matched {
		return fmt.Errorf("invalid email format")
	}
	return nil
}

// validateIPv4 validates an IPv4 address.
func validateIPv4(value string) error {
	ip := net.ParseIP(value)
	if ip == nil {
		return fmt.Errorf("invalid IP address")
	}
	if ip.To4() == nil {
		return fmt.Errorf("not an IPv4 address")
	}
	return nil
}

// validateIPv6 validates an IPv6 address.
func validateIPv6(value string) error {
	ip := net.ParseIP(value)
	if ip == nil {
		return fmt.Errorf("invalid IP address")
	}
	if ip.To4() != nil {
		return fmt.Errorf("not an IPv6 address")
	}
	return nil
}

// validateIP validates an IP address (v4 or v6).
func validateIP(value string) error {
	ip := net.ParseIP(value)
	if ip == nil {
		return fmt.Errorf("invalid IP address")
	}
	return nil
}

// validateCIDR validates a CIDR notation.
func validateCIDR(value string) error {
	_, _, err := net.ParseCIDR(value)
	return err
}

// validateDateTime validates an RFC 3339 date-time.
func validateDateTime(value string) error {
	_, err := time.Parse(time.RFC3339, value)
	if err != nil {
		// Try with nanoseconds
		_, err = time.Parse(time.RFC3339Nano, value)
	}
	return err
}

// validateDate validates a date in YYYY-MM-DD format.
func validateDate(value string) error {
	_, err := time.Parse("2006-01-02", value)
	return err
}

// validateTime validates a time in HH:MM:SS format.
func validateTime(value string) error {
	_, err := time.Parse("15:04:05", value)
	if err != nil {
		// Try without seconds
		_, err = time.Parse("15:04", value)
	}
	return err
}

// validateUUID validates a UUID.
func validateUUID(value string) error {
	// UUID format: xxxxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx
	pattern := `^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`
	matched, _ := regexp.MatchString(pattern, value)
	if !matched {
		return fmt.Errorf("invalid UUID format")
	}
	return nil
}

// validatePort validates a port number.
func validatePort(value string) error {
	port, err := strconv.Atoi(value)
	if err != nil {
		return fmt.Errorf("port must be a number")
	}
	if port < 1 || port > 65535 {
		return fmt.Errorf("port must be between 1 and 65535")
	}
	return nil
}

// validateSemVer validates a semantic version.
func validateSemVer(value string) error {
	// SemVer format: MAJOR.MINOR.PATCH[-prerelease][+build]
	pattern := `^(0|[1-9]\d*)\.(0|[1-9]\d*)\.(0|[1-9]\d*)(-[0-9A-Za-z\-]+(\.[0-9A-Za-z\-]+)*)?(\+[0-9A-Za-z\-]+(\.[0-9A-Za-z\-]+)*)?$`
	matched, _ := regexp.MatchString(pattern, value)
	if !matched {
		return fmt.Errorf("invalid semantic version format")
	}
	return nil
}

// isAlphaNumeric checks if a byte is alphanumeric.
func isAlphaNumeric(c byte) bool {
	return (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9')
}
