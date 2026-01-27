package variables

import (
	"testing"
)

func TestExpressionEvaluator_Evaluate(t *testing.T) {
	tests := []struct {
		name     string
		inputs   map[string]interface{}
		steps    map[string]map[string]interface{}
		expr     string
		expected bool
		wantErr  bool
	}{
		// Simple boolean expressions
		{
			name:     "literal true",
			expr:     "{{ true }}",
			expected: true,
		},
		{
			name:     "literal false",
			expr:     "{{ false }}",
			expected: false,
		},
		{
			name:     "simple input boolean true",
			inputs:   map[string]interface{}{"enabled": true},
			expr:     "{{ .inputs.enabled }}",
			expected: true,
		},
		{
			name:     "simple input boolean false",
			inputs:   map[string]interface{}{"enabled": false},
			expr:     "{{ .inputs.enabled }}",
			expected: false,
		},

		// Comparison operators
		{
			name:     "eq true",
			inputs:   map[string]interface{}{"env": "production"},
			expr:     `{{ eq .inputs.env "production" }}`,
			expected: true,
		},
		{
			name:     "eq false",
			inputs:   map[string]interface{}{"env": "staging"},
			expr:     `{{ eq .inputs.env "production" }}`,
			expected: false,
		},
		{
			name:     "ne true",
			inputs:   map[string]interface{}{"env": "staging"},
			expr:     `{{ ne .inputs.env "production" }}`,
			expected: true,
		},
		{
			name:     "lt with numbers",
			inputs:   map[string]interface{}{"count": 5},
			expr:     `{{ lt .inputs.count 10 }}`,
			expected: true,
		},
		{
			name:     "gt with numbers",
			inputs:   map[string]interface{}{"count": 15},
			expr:     `{{ gt .inputs.count 10 }}`,
			expected: true,
		},
		{
			name:     "le equal",
			inputs:   map[string]interface{}{"count": 10},
			expr:     `{{ le .inputs.count 10 }}`,
			expected: true,
		},
		{
			name:     "ge equal",
			inputs:   map[string]interface{}{"count": 10},
			expr:     `{{ ge .inputs.count 10 }}`,
			expected: true,
		},

		// Logical operators
		{
			name:     "and both true",
			inputs:   map[string]interface{}{"a": true, "b": true},
			expr:     `{{ and .inputs.a .inputs.b }}`,
			expected: true,
		},
		{
			name:     "and one false",
			inputs:   map[string]interface{}{"a": true, "b": false},
			expr:     `{{ and .inputs.a .inputs.b }}`,
			expected: false,
		},
		{
			name:     "or both false",
			inputs:   map[string]interface{}{"a": false, "b": false},
			expr:     `{{ or .inputs.a .inputs.b }}`,
			expected: false,
		},
		{
			name:     "or one true",
			inputs:   map[string]interface{}{"a": false, "b": true},
			expr:     `{{ or .inputs.a .inputs.b }}`,
			expected: true,
		},
		{
			name:     "not true becomes false",
			inputs:   map[string]interface{}{"a": true},
			expr:     `{{ not .inputs.a }}`,
			expected: false,
		},
		{
			name:     "not false becomes true",
			inputs:   map[string]interface{}{"a": false},
			expr:     `{{ not .inputs.a }}`,
			expected: true,
		},

		// Complex logical expressions
		{
			name: "complex and/or",
			inputs: map[string]interface{}{
				"env":    "production",
				"deploy": true,
			},
			expr:     `{{ and (eq .inputs.env "production") .inputs.deploy }}`,
			expected: true,
		},
		{
			name: "complex or with comparisons",
			inputs: map[string]interface{}{
				"env":   "staging",
				"force": true,
			},
			expr:     `{{ or (eq .inputs.env "production") .inputs.force }}`,
			expected: true,
		},

		// String functions
		{
			name:     "contains true",
			inputs:   map[string]interface{}{"message": "hello world"},
			expr:     `{{ contains .inputs.message "world" }}`,
			expected: true,
		},
		{
			name:     "contains false",
			inputs:   map[string]interface{}{"message": "hello world"},
			expr:     `{{ contains .inputs.message "foo" }}`,
			expected: false,
		},
		{
			name:     "startsWith true",
			inputs:   map[string]interface{}{"path": "/api/v1/users"},
			expr:     `{{ startsWith .inputs.path "/api" }}`,
			expected: true,
		},
		{
			name:     "endsWith true",
			inputs:   map[string]interface{}{"file": "config.yaml"},
			expr:     `{{ endsWith .inputs.file ".yaml" }}`,
			expected: true,
		},
		{
			name:     "matches regex true",
			inputs:   map[string]interface{}{"version": "v1.2.3"},
			expr:     `{{ matches "v[0-9]+\\.[0-9]+\\.[0-9]+" .inputs.version }}`,
			expected: true,
		},
		{
			name:     "matches regex false",
			inputs:   map[string]interface{}{"version": "invalid"},
			expr:     `{{ matches "v[0-9]+\\.[0-9]+\\.[0-9]+" .inputs.version }}`,
			expected: false,
		},

		// Value functions
		{
			name:     "empty string is true",
			inputs:   map[string]interface{}{"value": ""},
			expr:     `{{ empty .inputs.value }}`,
			expected: true,
		},
		{
			name:     "non-empty string is false",
			inputs:   map[string]interface{}{"value": "hello"},
			expr:     `{{ empty .inputs.value }}`,
			expected: false,
		},
		{
			name:     "defined returns true for existing",
			inputs:   map[string]interface{}{"value": "hello"},
			expr:     `{{ defined .inputs.value }}`,
			expected: true,
		},
		{
			name:     "empty slice is true",
			inputs:   map[string]interface{}{"items": []interface{}{}},
			expr:     `{{ empty .inputs.items }}`,
			expected: true,
		},
		{
			name:     "zero is empty",
			inputs:   map[string]interface{}{"count": 0},
			expr:     `{{ empty .inputs.count }}`,
			expected: true,
		},

		// Step output access
		{
			name:   "step output comparison",
			inputs: map[string]interface{}{},
			steps: map[string]map[string]interface{}{
				"check": {"result": "success"},
			},
			expr:     `{{ eq .steps.check.result "success" }}`,
			expected: true,
		},

		// Type checks
		{
			name:     "isString true",
			inputs:   map[string]interface{}{"value": "hello"},
			expr:     `{{ isString .inputs.value }}`,
			expected: true,
		},

		// Collection functions
		{
			name:     "in collection true",
			inputs:   map[string]interface{}{"env": "staging", "allowed": []interface{}{"dev", "staging", "prod"}},
			expr:     `{{ in .inputs.env .inputs.allowed }}`,
			expected: true,
		},
		{
			name:     "in collection false",
			inputs:   map[string]interface{}{"env": "local", "allowed": []interface{}{"dev", "staging", "prod"}},
			expr:     `{{ in .inputs.env .inputs.allowed }}`,
			expected: false,
		},

		// Expression without delimiters (should work too)
		{
			name:     "bare expression",
			inputs:   map[string]interface{}{"enabled": true},
			expr:     ".inputs.enabled",
			expected: true,
		},

		// Truthy string values
		{
			name:     "non-empty string is truthy",
			inputs:   map[string]interface{}{"value": "yes"},
			expr:     "{{ .inputs.value }}",
			expected: true,
		},
		{
			name:     "string false is falsy",
			inputs:   map[string]interface{}{"value": "false"},
			expr:     "{{ .inputs.value }}",
			expected: false,
		},
		{
			name:     "string 0 is falsy",
			inputs:   map[string]interface{}{"value": "0"},
			expr:     "{{ .inputs.value }}",
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := NewContext("test-exec", "test-runbook", "1.0.0", tt.inputs)

			// Add step outputs if provided
			for stepName, outputs := range tt.steps {
				ctx.SetStepOutputs(stepName, outputs)
			}

			evaluator := NewExpressionEvaluator(ctx)
			result, err := evaluator.Evaluate(tt.expr)

			if (err != nil) != tt.wantErr {
				t.Errorf("Evaluate() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if result != tt.expected {
				t.Errorf("Evaluate() = %v, expected %v", result, tt.expected)
			}
		})
	}
}

func TestExpressionEvaluator_EvaluateValue(t *testing.T) {
	tests := []struct {
		name     string
		inputs   map[string]interface{}
		expr     string
		expected interface{}
		wantErr  bool
	}{
		{
			name:     "string value",
			inputs:   map[string]interface{}{"name": "test"},
			expr:     "{{ .inputs.name }}",
			expected: "test",
		},
		{
			name:     "numeric result",
			inputs:   map[string]interface{}{"count": 42},
			expr:     "{{ .inputs.count }}",
			expected: 42,
		},
		{
			name:     "len function",
			inputs:   map[string]interface{}{"items": []interface{}{"a", "b", "c"}},
			expr:     "{{ len .inputs.items }}",
			expected: 3,
		},
		{
			name:     "upper function",
			inputs:   map[string]interface{}{"name": "hello"},
			expr:     "{{ upper .inputs.name }}",
			expected: "HELLO",
		},
		{
			name:     "lower function",
			inputs:   map[string]interface{}{"name": "HELLO"},
			expr:     "{{ lower .inputs.name }}",
			expected: "hello",
		},
		{
			name:     "ternary true",
			inputs:   map[string]interface{}{"flag": true},
			expr:     `{{ ternary "yes" "no" .inputs.flag }}`,
			expected: "yes",
		},
		{
			name:     "ternary false",
			inputs:   map[string]interface{}{"flag": false},
			expr:     `{{ ternary "yes" "no" .inputs.flag }}`,
			expected: "no",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := NewContext("test-exec", "test-runbook", "1.0.0", tt.inputs)
			evaluator := NewExpressionEvaluator(ctx)

			result, err := evaluator.EvaluateValue(tt.expr)

			if (err != nil) != tt.wantErr {
				t.Errorf("EvaluateValue() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if result != tt.expected {
				t.Errorf("EvaluateValue() = %v (%T), expected %v (%T)", result, result, tt.expected, tt.expected)
			}
		})
	}
}

func TestExpressionEvaluator_EvaluateCondition(t *testing.T) {
	ctx := NewContext("test-exec", "test-runbook", "1.0.0", map[string]interface{}{
		"enabled": true,
		"env":     "production",
	})

	evaluator := NewExpressionEvaluator(ctx)

	t.Run("successful condition", func(t *testing.T) {
		result := evaluator.EvaluateCondition(`{{ eq .inputs.env "production" }}`)

		if result.Error != nil {
			t.Errorf("unexpected error: %v", result.Error)
		}
		if !result.Value {
			t.Error("expected condition to be true")
		}
		if result.Message == "" {
			t.Error("expected non-empty message")
		}
	})

	t.Run("false condition", func(t *testing.T) {
		result := evaluator.EvaluateCondition(`{{ eq .inputs.env "staging" }}`)

		if result.Error != nil {
			t.Errorf("unexpected error: %v", result.Error)
		}
		if result.Value {
			t.Error("expected condition to be false")
		}
	})

	t.Run("invalid expression", func(t *testing.T) {
		result := evaluator.EvaluateCondition(`{{ invalid syntax {{`)

		if result.Error == nil {
			t.Error("expected error for invalid expression")
		}
	})
}

func TestExpressionEvaluator_MustEvaluate(t *testing.T) {
	ctx := NewContext("test-exec", "test-runbook", "1.0.0", map[string]interface{}{
		"enabled": true,
	})

	evaluator := NewExpressionEvaluator(ctx)

	t.Run("valid expression", func(t *testing.T) {
		result := evaluator.MustEvaluate("{{ .inputs.enabled }}", false)
		if !result {
			t.Error("expected true")
		}
	})

	t.Run("invalid expression returns default", func(t *testing.T) {
		result := evaluator.MustEvaluate("{{ invalid syntax {{", true)
		if !result {
			t.Error("expected default value true")
		}

		result = evaluator.MustEvaluate("{{ invalid syntax {{", false)
		if result {
			t.Error("expected default value false")
		}
	})
}

func TestValidateExpression(t *testing.T) {
	tests := []struct {
		name    string
		expr    string
		wantErr bool
	}{
		{
			name:    "valid simple expression",
			expr:    "{{ .inputs.enabled }}",
			wantErr: false,
		},
		{
			name:    "valid comparison",
			expr:    `{{ eq .inputs.env "prod" }}`,
			wantErr: false,
		},
		{
			name:    "valid complex expression",
			expr:    `{{ and (eq .inputs.env "prod") .inputs.deploy }}`,
			wantErr: false,
		},
		{
			name:    "valid bare expression",
			expr:    `.inputs.enabled`,
			wantErr: false,
		},
		{
			name:    "invalid syntax - unclosed",
			expr:    "{{ .inputs.enabled",
			wantErr: true,
		},
		{
			name:    "invalid syntax - nested delimiters",
			expr:    "{{ {{ .inputs.enabled }} }}",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateExpression(tt.expr)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateExpression() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestToBoolValue(t *testing.T) {
	tests := []struct {
		name     string
		value    interface{}
		expected bool
	}{
		{"bool true", true, true},
		{"bool false", false, false},
		{"string true", "true", true},
		{"string false", "false", false},
		{"string empty", "", false},
		{"string 0", "0", false},
		{"string non-empty", "hello", true},
		{"string yes", "yes", true},
		{"int zero", 0, false},
		{"int non-zero", 42, true},
		{"int negative", -1, true},
		{"float zero", 0.0, false},
		{"float non-zero", 3.14, true},
		{"nil", nil, false},
		{"no value", "<no value>", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := toBoolValue(tt.value)
			if result != tt.expected {
				t.Errorf("toBoolValue(%v) = %v, expected %v", tt.value, result, tt.expected)
			}
		})
	}
}

func TestIsEmpty(t *testing.T) {
	tests := []struct {
		name     string
		value    interface{}
		expected bool
	}{
		{"nil", nil, true},
		{"empty string", "", true},
		{"non-empty string", "hello", false},
		{"empty slice", []interface{}{}, true},
		{"non-empty slice", []interface{}{"a"}, false},
		{"empty map", map[string]interface{}{}, true},
		{"non-empty map", map[string]interface{}{"a": 1}, false},
		{"zero int", 0, true},
		{"non-zero int", 42, false},
		{"zero float", 0.0, true},
		{"non-zero float", 3.14, false},
		{"false bool", false, true},
		{"true bool", true, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isEmpty(tt.value)
			if result != tt.expected {
				t.Errorf("isEmpty(%v) = %v, expected %v", tt.value, result, tt.expected)
			}
		})
	}
}

func TestInCollection(t *testing.T) {
	tests := []struct {
		name       string
		item       interface{}
		collection interface{}
		expected   bool
	}{
		{
			name:       "string in interface slice",
			item:       "b",
			collection: []interface{}{"a", "b", "c"},
			expected:   true,
		},
		{
			name:       "string not in interface slice",
			item:       "d",
			collection: []interface{}{"a", "b", "c"},
			expected:   false,
		},
		{
			name:       "string in string slice",
			item:       "staging",
			collection: []string{"dev", "staging", "prod"},
			expected:   true,
		},
		{
			name:       "key in map",
			item:       "name",
			collection: map[string]interface{}{"name": "test", "value": 42},
			expected:   true,
		},
		{
			name:       "key not in map",
			item:       "missing",
			collection: map[string]interface{}{"name": "test"},
			expected:   false,
		},
		{
			name:       "substring in string",
			item:       "world",
			collection: "hello world",
			expected:   true,
		},
		{
			name:       "substring not in string",
			item:       "foo",
			collection: "hello world",
			expected:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := inCollection(tt.item, tt.collection)
			if result != tt.expected {
				t.Errorf("inCollection(%v, %v) = %v, expected %v", tt.item, tt.collection, result, tt.expected)
			}
		})
	}
}

func TestCompare(t *testing.T) {
	tests := []struct {
		name     string
		a        interface{}
		b        interface{}
		expected CompareResult
	}{
		{"int less", 5, 10, CompareLess},
		{"int equal", 10, 10, CompareEqual},
		{"int greater", 15, 10, CompareGreater},
		{"float less", 3.14, 5.0, CompareLess},
		{"float equal", 3.14, 3.14, CompareEqual},
		{"string numbers", "5", "10", CompareLess},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := Compare(tt.a, tt.b)
			if result != tt.expected {
				t.Errorf("Compare(%v, %v) = %v, expected %v", tt.a, tt.b, result, tt.expected)
			}
		})
	}
}

func TestContextEvaluateCondition(t *testing.T) {
	ctx := NewContext("test-exec", "test-runbook", "1.0.0", map[string]interface{}{
		"enabled": true,
		"env":     "production",
	})

	t.Run("true condition", func(t *testing.T) {
		result, err := ctx.EvaluateCondition(`{{ eq .inputs.env "production" }}`)
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}
		if !result {
			t.Error("expected true")
		}
	})

	t.Run("false condition", func(t *testing.T) {
		result, err := ctx.EvaluateCondition(`{{ eq .inputs.env "staging" }}`)
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}
		if result {
			t.Error("expected false")
		}
	})

	t.Run("condition result", func(t *testing.T) {
		result := ctx.EvaluateConditionResult(`{{ .inputs.enabled }}`)
		if result.Error != nil {
			t.Errorf("unexpected error: %v", result.Error)
		}
		if !result.Value {
			t.Error("expected true")
		}
		if result.Message == "" {
			t.Error("expected non-empty message")
		}
	})
}
