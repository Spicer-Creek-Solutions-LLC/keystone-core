package blueprint

import (
	"fmt"
	"testing"
)

func TestNewParameterValidator(t *testing.T) {
	v := NewParameterValidator()
	if v == nil {
		t.Fatal("NewParameterValidator returned nil")
	}
	if !v.coercionEnabled {
		t.Error("coercion should be enabled by default")
	}
	if !v.maskSensitive {
		t.Error("mask sensitive should be enabled by default")
	}
	if len(v.formatValidators) == 0 {
		t.Error("format validators should be registered")
	}
}

func TestParameterValidator_FormatValidators(t *testing.T) {
	tests := []struct {
		format    string
		value     string
		wantError bool
	}{
		// Hostname
		{"hostname", "example.com", false},
		{"hostname", "my-host.example.com", false},
		{"hostname", "localhost", false},
		{"hostname", "-invalid.com", true},
		{"hostname", "invalid-.com", true},
		{"hostname", "", true},

		// URI/URL
		{"uri", "https://example.com", false},
		{"uri", "http://localhost:8080/path", false},
		{"uri", "ftp://files.example.com/file.txt", false},
		{"uri", "example.com", true}, // missing scheme
		{"url", "https://example.com", false},

		// Email
		{"email", "user@example.com", false},
		{"email", "user.name+tag@example.com", false},
		{"email", "invalid", true},
		{"email", "invalid@", true},
		{"email", "@example.com", true},

		// IPv4
		{"ipv4", "192.168.1.1", false},
		{"ipv4", "10.0.0.0", false},
		{"ipv4", "255.255.255.255", false},
		{"ipv4", "192.168.1.256", true},
		{"ipv4", "invalid", true},
		{"ipv4", "::1", true}, // IPv6

		// IPv6
		{"ipv6", "::1", false},
		{"ipv6", "2001:db8::1", false},
		{"ipv6", "fe80::1", false},
		{"ipv6", "192.168.1.1", true}, // IPv4
		// Note: Go's net.ParseIP doesn't support zone IDs (fe80::1%eth0)

		// IP (any)
		{"ip", "192.168.1.1", false},
		{"ip", "::1", false},
		{"ip", "invalid", true},

		// CIDR
		{"cidr", "192.168.1.0/24", false},
		{"cidr", "10.0.0.0/8", false},
		{"cidr", "2001:db8::/32", false},
		{"cidr", "192.168.1.1", true}, // missing prefix
		{"cidr", "192.168.1.0/33", true},

		// Date-Time
		{"date-time", "2024-01-15T10:30:00Z", false},
		{"date-time", "2024-01-15T10:30:00+05:30", false},
		{"datetime", "2024-01-15T10:30:00.123Z", false},
		{"date-time", "2024-01-15", true},
		{"date-time", "invalid", true},

		// Date
		{"date", "2024-01-15", false},
		{"date", "2024-12-31", false},
		{"date", "2024-1-15", true},
		{"date", "2024/01/15", true},

		// Time
		{"time", "10:30:00", false},
		{"time", "23:59:59", false},
		{"time", "10:30", false},
		{"time", "25:00:00", true},
		{"time", "10:60:00", true},

		// UUID
		{"uuid", "550e8400-e29b-41d4-a716-446655440000", false},
		{"uuid", "550E8400-E29B-41D4-A716-446655440000", false},
		{"uuid", "550e8400e29b41d4a716446655440000", true}, // missing hyphens
		{"uuid", "invalid", true},

		// Port
		{"port", "80", false},
		{"port", "443", false},
		{"port", "65535", false},
		{"port", "0", true},
		{"port", "65536", true},
		{"port", "invalid", true},

		// SemVer
		{"semver", "1.0.0", false},
		{"semver", "1.2.3", false},
		{"semver", "1.0.0-alpha", false},
		{"semver", "1.0.0-alpha.1", false},
		{"semver", "1.0.0+build", false},
		{"semver", "1.0.0-alpha+build", false},
		{"semver", "v1.0.0", true}, // no 'v' prefix
		{"semver", "1.0", true},

		// DNS Name
		{"dns-name", "example.com", false},
		{"dns-name", "_dmarc.example.com", false}, // underscore allowed
		{"dns-name", "my-service.default.svc.cluster.local", false},
		{"dns-name", "", true},
	}

	v := NewParameterValidator()

	for _, tt := range tests {
		t.Run(tt.format+"_"+tt.value, func(t *testing.T) {
			schema := ParameterSchema{
				Type:   "string",
				Format: tt.format,
			}
			_, err := v.ValidateParameter("test", schema, tt.value, false)
			if tt.wantError && err == nil {
				t.Errorf("expected error for format %s with value %q", tt.format, tt.value)
			}
			if !tt.wantError && err != nil {
				t.Errorf("unexpected error for format %s with value %q: %v", tt.format, tt.value, err)
			}
		})
	}
}

func TestParameterValidator_TypeCoercion(t *testing.T) {
	v := NewParameterValidator()

	tests := []struct {
		name       string
		targetType string
		input      interface{}
		want       interface{}
		wantError  bool
	}{
		// String coercion
		{"int to string", "string", 123, "123", false},
		{"float to string", "string", 123.456, "123.456", false},
		{"bool to string", "string", true, "true", false},

		// Integer coercion
		{"string to int", "integer", "123", int64(123), false},
		{"float to int", "integer", 123.9, int64(123), false},
		{"bool to int", "integer", true, int64(1), false},
		{"bool false to int", "integer", false, int64(0), false},

		// Number coercion
		{"string to number", "number", "123.456", 123.456, false},
		{"int to number", "number", 123, float64(123), false},

		// Boolean coercion
		{"string true to bool", "boolean", "true", true, false},
		{"string yes to bool", "boolean", "yes", true, false},
		{"string on to bool", "boolean", "on", true, false},
		{"string 1 to bool", "boolean", "1", true, false},
		{"string enabled to bool", "boolean", "enabled", true, false},
		{"string false to bool", "boolean", "false", false, false},
		{"string no to bool", "boolean", "no", false, false},
		{"string off to bool", "boolean", "off", false, false},
		{"string 0 to bool", "boolean", "0", false, false},
		{"string disabled to bool", "boolean", "disabled", false, false},
		{"int 1 to bool", "boolean", 1, true, false},
		{"int 0 to bool", "boolean", 0, false, false},

		// Array coercion
		{"single value to array", "array", "item", []interface{}{"item"}, false},
		{"array unchanged", "array", []interface{}{"a", "b"}, []interface{}{"a", "b"}, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			schema := ParameterSchema{Type: tt.targetType}
			result, err := v.ValidateParameter("test", schema, tt.input, false)
			if tt.wantError {
				if err == nil {
					t.Errorf("expected error")
				}
				return
			}
			if err != nil {
				t.Errorf("unexpected error: %v", err)
				return
			}

			// Compare results
			switch want := tt.want.(type) {
			case int64:
				// For integer coercion, result might be int64 or the coerced value
				switch got := result.(type) {
				case int64:
					if got != want {
						t.Errorf("got %v, want %v", got, want)
					}
				case int:
					if int64(got) != want {
						t.Errorf("got %v, want %v", got, want)
					}
				case float64:
					// YAML often parses numbers as float64
					if int64(got) != want {
						t.Errorf("got %v, want %v", got, want)
					}
				default:
					t.Errorf("unexpected type %T", result)
				}
			case []interface{}:
				got, ok := result.([]interface{})
				if !ok {
					t.Errorf("expected array, got %T", result)
					return
				}
				if len(got) != len(want) {
					t.Errorf("array length: got %d, want %d", len(got), len(want))
				}
			default:
				if result != tt.want {
					t.Errorf("got %v (%T), want %v (%T)", result, result, tt.want, tt.want)
				}
			}
		})
	}
}

func TestParameterValidator_TypeCoercionDisabled(t *testing.T) {
	v := NewParameterValidator()
	v.SetCoercionEnabled(false)

	schema := ParameterSchema{Type: "integer"}
	_, err := v.ValidateParameter("test", schema, "123", false)
	if err == nil {
		t.Error("expected error when coercion disabled and string passed for integer")
	}
}

func TestParameterValidator_Required(t *testing.T) {
	v := NewParameterValidator()

	schema := ParameterSchema{
		Type:     "string",
		Required: true,
	}

	// Missing required parameter
	_, err := v.ValidateParameter("test", schema, nil, false)
	if err == nil {
		t.Error("expected error for missing required parameter")
	}

	// Present required parameter
	_, err = v.ValidateParameter("test", schema, "value", false)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestParameterValidator_Default(t *testing.T) {
	v := NewParameterValidator()

	schema := ParameterSchema{
		Type:    "string",
		Default: "default-value",
	}

	// Missing parameter should get default
	result, err := v.ValidateParameter("test", schema, nil, false)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if result != "default-value" {
		t.Errorf("expected default value, got %v", result)
	}
}

func TestParameterValidator_Enum(t *testing.T) {
	v := NewParameterValidator()

	schema := ParameterSchema{
		Type: "string",
		Enum: []interface{}{"small", "medium", "large"},
	}

	// Valid enum value
	_, err := v.ValidateParameter("test", schema, "medium", false)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	// Invalid enum value
	_, err = v.ValidateParameter("test", schema, "extra-large", false)
	if err == nil {
		t.Error("expected error for invalid enum value")
	}
}

func TestParameterValidator_StringConstraints(t *testing.T) {
	v := NewParameterValidator()

	minLen := 3
	maxLen := 10
	schema := ParameterSchema{
		Type:      "string",
		MinLength: &minLen,
		MaxLength: &maxLen,
		Pattern:   "^[a-z]+$",
	}

	tests := []struct {
		value     string
		wantError bool
		reason    string
	}{
		{"abc", false, "valid"},
		{"ab", true, "too short"},
		{"abcdefghijk", true, "too long"},
		{"ABC", true, "pattern mismatch"},
	}

	for _, tt := range tests {
		t.Run(tt.reason, func(t *testing.T) {
			_, err := v.ValidateParameter("test", schema, tt.value, false)
			if tt.wantError && err == nil {
				t.Errorf("expected error: %s", tt.reason)
			}
			if !tt.wantError && err != nil {
				t.Errorf("unexpected error: %v", err)
			}
		})
	}
}

func TestParameterValidator_NumericConstraints(t *testing.T) {
	v := NewParameterValidator()

	min := 1.0
	max := 100.0
	schema := ParameterSchema{
		Type:    "number",
		Minimum: &min,
		Maximum: &max,
	}

	tests := []struct {
		value     interface{}
		wantError bool
	}{
		{50, false},
		{1, false},
		{100, false},
		{0, true},
		{101, true},
	}

	for _, tt := range tests {
		t.Run("", func(t *testing.T) {
			_, err := v.ValidateParameter("test", schema, tt.value, false)
			if tt.wantError && err == nil {
				t.Errorf("expected error for value %v", tt.value)
			}
			if !tt.wantError && err != nil {
				t.Errorf("unexpected error for value %v: %v", tt.value, err)
			}
		})
	}
}

func TestParameterValidator_ArrayConstraints(t *testing.T) {
	v := NewParameterValidator()

	minItems := 2
	maxItems := 5
	schema := ParameterSchema{
		Type:     "array",
		MinItems: &minItems,
		MaxItems: &maxItems,
		Items: &ParameterSchema{
			Type: "string",
		},
	}

	tests := []struct {
		value     []interface{}
		wantError bool
	}{
		{[]interface{}{"a", "b"}, false},
		{[]interface{}{"a", "b", "c", "d", "e"}, false},
		{[]interface{}{"a"}, true},                          // too few
		{[]interface{}{"a", "b", "c", "d", "e", "f"}, true}, // too many
		// Note: 123 gets coerced to "123" when coercion is enabled
		{[]interface{}{"a", 123}, false}, // coerced to string
	}

	for _, tt := range tests {
		t.Run("", func(t *testing.T) {
			_, err := v.ValidateParameter("test", schema, tt.value, false)
			if tt.wantError && err == nil {
				t.Errorf("expected error for value %v", tt.value)
			}
			if !tt.wantError && err != nil {
				t.Errorf("unexpected error for value %v: %v", tt.value, err)
			}
		})
	}
}

func TestParameterValidator_ArrayItemsNoCoercion(t *testing.T) {
	v := NewParameterValidator()
	v.SetCoercionEnabled(false) // Disable coercion

	schema := ParameterSchema{
		Type: "array",
		Items: &ParameterSchema{
			Type: "string",
		},
	}

	// With coercion disabled, int in string array should fail
	_, err := v.ValidateParameter("test", schema, []interface{}{"a", 123}, false)
	if err == nil {
		t.Error("expected error for int in string array without coercion")
	}
}

func TestParameterValidator_ObjectValidation(t *testing.T) {
	v := NewParameterValidator()

	schema := ParameterSchema{
		Type: "object",
		Properties: map[string]ParameterSchema{
			"name": {
				Type:     "string",
				Required: true,
			},
			"age": {
				Type: "integer",
			},
		},
	}

	// Valid object
	_, err := v.ValidateParameter("test", schema, map[string]interface{}{
		"name": "John",
		"age":  30,
	}, false)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	// Missing required property
	_, err = v.ValidateParameter("test", schema, map[string]interface{}{
		"age": 30,
	}, false)
	if err == nil {
		t.Error("expected error for missing required property")
	}
}

func TestParameterValidator_SensitiveMasking(t *testing.T) {
	v := NewParameterValidator()
	v.SetMaskSensitive(true)

	schema := ParameterSchema{
		Type:      "string",
		Sensitive: true,
		Enum:      []interface{}{"valid"},
	}

	_, err := v.ValidateParameter("test", schema, "secret-value", true)
	if err == nil {
		t.Fatal("expected error")
	}
	if contains(err.Error(), "secret-value") {
		t.Error("error message should not contain sensitive value")
	}
	if !contains(err.Error(), "REDACTED") {
		t.Error("error message should contain REDACTED")
	}
}

func TestParameterValidator_ValidateParameters(t *testing.T) {
	v := NewParameterValidator()

	schemas := map[string]ParameterSchema{
		"name": {
			Type:     "string",
			Required: true,
		},
		"port": {
			Type:    "integer",
			Default: 8080,
		},
		"debug": {
			Type:    "boolean",
			Feature: "debugging",
		},
	}

	values := map[string]interface{}{
		"name": "my-service",
	}

	result, err := v.ValidateParameters(schemas, values, []string{})
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	if result["name"] != "my-service" {
		t.Error("name should be preserved")
	}
	// Note: default is not applied here as we're just validating, not applying defaults
}

func TestParameterValidator_RegisterFormatValidator(t *testing.T) {
	v := NewParameterValidator()

	// Register custom format
	v.RegisterFormatValidator("custom", func(value string) error {
		if value != "valid" {
			return fmt.Errorf("must be 'valid'")
		}
		return nil
	})

	schema := ParameterSchema{
		Type:   "string",
		Format: "custom",
	}

	// Valid custom format
	_, err := v.ValidateParameter("test", schema, "valid", false)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	// Invalid custom format
	_, err = v.ValidateParameter("test", schema, "invalid", false)
	if err == nil {
		t.Error("expected error for invalid custom format")
	}
}

func TestParameterValidator_RequiredIf(t *testing.T) {
	v := NewParameterValidator()

	schemas := map[string]ParameterSchema{
		"ssl_provider": {
			Type:    "string",
			Default: "letsencrypt",
			Enum:    []interface{}{"letsencrypt", "custom", "selfsigned"},
		},
		"ssl_key": {
			Type:      "string",
			Sensitive: true,
			RequiredIf: []map[string]interface{}{
				{"ssl_provider": "custom"},
			},
		},
		"ssl_cert": {
			Type:      "string",
			Sensitive: true,
			RequiredIf: []map[string]interface{}{
				{"ssl_provider": "custom"},
			},
		},
	}

	tests := []struct {
		name      string
		values    map[string]interface{}
		wantError bool
		errorMsg  string
	}{
		{
			name: "ssl_provider letsencrypt - ssl_key not required",
			values: map[string]interface{}{
				"ssl_provider": "letsencrypt",
			},
			wantError: false,
		},
		{
			name: "ssl_provider custom without ssl_key - error",
			values: map[string]interface{}{
				"ssl_provider": "custom",
			},
			wantError: true,
			errorMsg:  "ssl_key is required when ssl_provider=custom",
		},
		{
			name: "ssl_provider custom with ssl_key - ok",
			values: map[string]interface{}{
				"ssl_provider": "custom",
				"ssl_key":      "-----BEGIN PRIVATE KEY-----",
				"ssl_cert":     "-----BEGIN CERTIFICATE-----",
			},
			wantError: false,
		},
		{
			name: "ssl_provider selfsigned - ssl_key not required",
			values: map[string]interface{}{
				"ssl_provider": "selfsigned",
			},
			wantError: false,
		},
		{
			name:      "ssl_provider default (letsencrypt) - ssl_key not required",
			values:    map[string]interface{}{},
			wantError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := v.ValidateParameters(schemas, tt.values, nil)
			if tt.wantError {
				if err == nil {
					t.Errorf("expected error containing %q", tt.errorMsg)
				} else if !containsSubstring(err.Error(), "ssl_key is required") && !containsSubstring(err.Error(), "ssl_cert is required") {
					t.Errorf("expected error about ssl_key or ssl_cert being required, got: %v", err)
				}
			} else if err != nil {
				t.Errorf("unexpected error: %v", err)
			}
		})
	}
}

func TestParameterValidator_RequiredIf_MultipleConditions(t *testing.T) {
	v := NewParameterValidator()

	// Test with multiple conditions (OR logic)
	schemas := map[string]ParameterSchema{
		"db_type": {
			Type: "string",
			Enum: []interface{}{"mysql", "postgres", "sqlite"},
		},
		"db_host": {
			Type: "string",
			RequiredIf: []map[string]interface{}{
				{"db_type": "mysql"},
				{"db_type": "postgres"},
			},
		},
	}

	tests := []struct {
		name      string
		values    map[string]interface{}
		wantError bool
	}{
		{
			name:      "sqlite - db_host not required",
			values:    map[string]interface{}{"db_type": "sqlite"},
			wantError: false,
		},
		{
			name:      "mysql without db_host - error",
			values:    map[string]interface{}{"db_type": "mysql"},
			wantError: true,
		},
		{
			name:      "postgres without db_host - error",
			values:    map[string]interface{}{"db_type": "postgres"},
			wantError: true,
		},
		{
			name:      "mysql with db_host - ok",
			values:    map[string]interface{}{"db_type": "mysql", "db_host": "localhost"},
			wantError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := v.ValidateParameters(schemas, tt.values, nil)
			if tt.wantError && err == nil {
				t.Error("expected error for missing conditionally required parameter")
			}
			if !tt.wantError && err != nil {
				t.Errorf("unexpected error: %v", err)
			}
		})
	}
}

func TestParameterValidator_RequiredIf_WithDefault(t *testing.T) {
	v := NewParameterValidator()

	// If the parameter has a default, required_if should not trigger error
	schemas := map[string]ParameterSchema{
		"mode": {
			Type: "string",
			Enum: []interface{}{"simple", "advanced"},
		},
		"config_file": {
			Type:    "string",
			Default: "/etc/default.conf",
			RequiredIf: []map[string]interface{}{
				{"mode": "advanced"},
			},
		},
	}

	// Even though mode=advanced triggers required_if, the default value satisfies it
	_, err := v.ValidateParameters(schemas, map[string]interface{}{"mode": "advanced"}, nil)
	if err != nil {
		t.Errorf("unexpected error - default should satisfy required_if: %v", err)
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsSubstring(s, substr))
}

func containsSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
