package statemgmt

import (
	"testing"
)

func TestNewConditionEvaluator(t *testing.T) {
	ce := NewConditionEvaluator()
	if ce == nil {
		t.Fatal("expected non-nil evaluator")
	}
	if ce.DefaultContext == nil {
		t.Fatal("expected non-nil default context")
	}
}

func TestNewConditionContext(t *testing.T) {
	ctx := NewConditionContext(nil)
	if ctx.Facts == nil {
		t.Error("expected non-nil Facts map")
	}

	facts := map[string]interface{}{"os": "Linux"}
	ctx = NewConditionContext(facts)
	if ctx.Facts["os"] != "Linux" {
		t.Error("expected Facts to be initialized with provided values")
	}
}

func TestConditionEvaluator_Parse(t *testing.T) {
	ce := NewConditionEvaluator()

	tests := []struct {
		name       string
		expression string
		wantErr    bool
	}{
		{"simple equality", "facts.os == 'Ubuntu'", false},
		{"not equal", "facts.os != 'Windows'", false},
		{"greater than", "facts.cpu_count > 2", false},
		{"less than", "facts.memory_gb < 16", false},
		{"and operator", "facts.os == 'Linux' and facts.arch == 'amd64'", false},
		{"or operator", "facts.os == 'Linux' or facts.os == 'Darwin'", false},
		{"not operator", "not facts.is_container", false},
		{"in operator", "facts.os in ['Linux', 'Darwin']", false},
		{"contains", "facts.packages contains 'nginx'", false},
		{"regex match", "facts.hostname =~ 'web-.*'", false},
		{"parentheses", "(facts.os == 'Linux') and (facts.arch == 'amd64')", false},
		{"empty expression", "", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			condition, err := ce.Parse(tt.expression)
			if (err != nil) != tt.wantErr {
				t.Errorf("Parse() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && condition == nil {
				t.Error("expected non-nil condition for valid expression")
			}
		})
	}
}

func TestConditionEvaluator_EvaluateExpression_Equality(t *testing.T) {
	ce := NewConditionEvaluator()
	ctx := NewConditionContext(map[string]interface{}{
		"os":      "Ubuntu",
		"version": "22.04",
		"arch":    "amd64",
	})

	tests := []struct {
		name       string
		expression string
		want       bool
	}{
		{"string equality true", "facts.os == 'Ubuntu'", true},
		{"string equality false", "facts.os == 'CentOS'", false},
		{"string inequality true", "facts.os != 'CentOS'", true},
		{"string inequality false", "facts.os != 'Ubuntu'", false},
		{"double quoted string", `facts.os == "Ubuntu"`, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := ce.EvaluateExpression(tt.expression, ctx)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if result != tt.want {
				t.Errorf("EvaluateExpression() = %v, want %v", result, tt.want)
			}
		})
	}
}

func TestConditionEvaluator_EvaluateExpression_Numeric(t *testing.T) {
	ce := NewConditionEvaluator()
	ctx := NewConditionContext(map[string]interface{}{
		"cpu_count": 4,
		"memory_gb": 16.5,
	})

	tests := []struct {
		name       string
		expression string
		want       bool
	}{
		{"int greater than true", "facts.cpu_count > 2", true},
		{"int greater than false", "facts.cpu_count > 4", false},
		{"int less than true", "facts.cpu_count < 8", true},
		{"int less than false", "facts.cpu_count < 2", false},
		{"float greater equal true", "facts.memory_gb >= 16", true},
		{"float less equal true", "facts.memory_gb <= 16.5", true},
		{"int equality", "facts.cpu_count == 4", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := ce.EvaluateExpression(tt.expression, ctx)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if result != tt.want {
				t.Errorf("EvaluateExpression() = %v, want %v", result, tt.want)
			}
		})
	}
}

func TestConditionEvaluator_EvaluateExpression_Boolean(t *testing.T) {
	ce := NewConditionEvaluator()
	ctx := NewConditionContext(map[string]interface{}{
		"is_container": true,
		"is_virtual":   false,
	})

	tests := []struct {
		name       string
		expression string
		want       bool
	}{
		{"bool field true", "facts.is_container", true},
		{"bool field false", "facts.is_virtual", false},
		{"bool equality true", "facts.is_container == true", true},
		{"bool equality false", "facts.is_container == false", false},
		{"not bool true", "not facts.is_virtual", true},
		{"not bool false", "not facts.is_container", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := ce.EvaluateExpression(tt.expression, ctx)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if result != tt.want {
				t.Errorf("EvaluateExpression() = %v, want %v", result, tt.want)
			}
		})
	}
}

func TestConditionEvaluator_EvaluateExpression_Logical(t *testing.T) {
	ce := NewConditionEvaluator()
	ctx := NewConditionContext(map[string]interface{}{
		"os":   "Linux",
		"arch": "amd64",
		"env":  "production",
	})

	tests := []struct {
		name       string
		expression string
		want       bool
	}{
		{"and both true", "facts.os == 'Linux' and facts.arch == 'amd64'", true},
		{"and first false", "facts.os == 'Windows' and facts.arch == 'amd64'", false},
		{"and second false", "facts.os == 'Linux' and facts.arch == 'arm64'", false},
		{"or first true", "facts.os == 'Linux' or facts.os == 'Darwin'", true},
		{"or second true", "facts.os == 'Darwin' or facts.os == 'Linux'", true},
		{"or both false", "facts.os == 'Darwin' or facts.os == 'Windows'", false},
		{"complex and/or", "(facts.os == 'Linux' or facts.os == 'Darwin') and facts.arch == 'amd64'", true},
		{"three conditions", "facts.os == 'Linux' and facts.arch == 'amd64' and facts.env == 'production'", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := ce.EvaluateExpression(tt.expression, ctx)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if result != tt.want {
				t.Errorf("EvaluateExpression() = %v, want %v", result, tt.want)
			}
		})
	}
}

func TestConditionEvaluator_EvaluateExpression_In(t *testing.T) {
	ce := NewConditionEvaluator()
	ctx := NewConditionContext(map[string]interface{}{
		"os":       "Ubuntu",
		"packages": []interface{}{"nginx", "redis", "postgresql"},
	})

	tests := []struct {
		name       string
		expression string
		want       bool
	}{
		{"in list true", "facts.os in ['Ubuntu', 'Debian', 'CentOS']", true},
		{"in list false", "facts.os in ['Windows', 'macOS']", false},
		{"not in list true", "facts.os not in ['Windows', 'macOS']", true},
		{"not in list false", "facts.os not in ['Ubuntu', 'Debian']", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := ce.EvaluateExpression(tt.expression, ctx)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if result != tt.want {
				t.Errorf("EvaluateExpression() = %v, want %v", result, tt.want)
			}
		})
	}
}

func TestConditionEvaluator_EvaluateExpression_Contains(t *testing.T) {
	ce := NewConditionEvaluator()
	ctx := NewConditionContext(map[string]interface{}{
		"packages": []interface{}{"nginx", "redis", "postgresql"},
		"hostname": "web-server-01",
	})

	tests := []struct {
		name       string
		expression string
		want       bool
	}{
		{"list contains true", "facts.packages contains 'nginx'", true},
		{"list contains false", "facts.packages contains 'mysql'", false},
		{"string contains true", "facts.hostname contains 'server'", true},
		{"string contains false", "facts.hostname contains 'database'", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := ce.EvaluateExpression(tt.expression, ctx)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if result != tt.want {
				t.Errorf("EvaluateExpression() = %v, want %v", result, tt.want)
			}
		})
	}
}

func TestConditionEvaluator_EvaluateExpression_Regex(t *testing.T) {
	ce := NewConditionEvaluator()
	ctx := NewConditionContext(map[string]interface{}{
		"hostname": "web-prod-001",
		"ip":       "192.168.1.100",
	})

	tests := []struct {
		name       string
		expression string
		want       bool
	}{
		{"regex match true", "facts.hostname =~ 'web-.*'", true},
		{"regex match false", "facts.hostname =~ 'db-.*'", false},
		{"regex not match true", "facts.hostname !~ 'db-.*'", true},
		{"regex not match false", "facts.hostname !~ 'web-.*'", false},
		{"regex ip pattern", "facts.ip =~ '^192\\.168\\..*'", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := ce.EvaluateExpression(tt.expression, ctx)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if result != tt.want {
				t.Errorf("EvaluateExpression() = %v, want %v", result, tt.want)
			}
		})
	}
}

func TestConditionEvaluator_EvaluateExpression_StringFunctions(t *testing.T) {
	ce := NewConditionEvaluator()
	ctx := NewConditionContext(map[string]interface{}{
		"hostname": "web-prod-001",
		"path":     "/var/log/nginx/access.log",
	})

	tests := []struct {
		name       string
		expression string
		want       bool
	}{
		{"startswith true", "facts.hostname startswith 'web'", true},
		{"startswith false", "facts.hostname startswith 'db'", false},
		{"endswith true", "facts.path endswith '.log'", true},
		{"endswith false", "facts.path endswith '.txt'", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := ce.EvaluateExpression(tt.expression, ctx)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if result != tt.want {
				t.Errorf("EvaluateExpression() = %v, want %v", result, tt.want)
			}
		})
	}
}

func TestConditionEvaluator_EvaluateExpression_NestedFields(t *testing.T) {
	ce := NewConditionEvaluator()
	ctx := NewConditionContext(map[string]interface{}{
		"network": map[string]interface{}{
			"interfaces": map[string]interface{}{
				"eth0": map[string]interface{}{
					"ipv4": "192.168.1.100",
					"mtu":  1500,
				},
			},
		},
	})

	tests := []struct {
		name       string
		expression string
		want       bool
	}{
		{"nested field equality", "facts.network.interfaces.eth0.ipv4 == '192.168.1.100'", true},
		{"nested field numeric", "facts.network.interfaces.eth0.mtu == 1500", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := ce.EvaluateExpression(tt.expression, ctx)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if result != tt.want {
				t.Errorf("EvaluateExpression() = %v, want %v", result, tt.want)
			}
		})
	}
}

func TestConditionEvaluator_EvaluateExpression_NilHandling(t *testing.T) {
	ce := NewConditionEvaluator()
	ctx := NewConditionContext(map[string]interface{}{
		"existing": "value",
	})

	tests := []struct {
		name       string
		expression string
		want       bool
	}{
		{"nil field is falsy", "facts.nonexistent", false},
		{"nil equality nil", "facts.nonexistent == nil", true},
		{"nil inequality nil", "facts.nonexistent != nil", false},
		{"existing not nil", "facts.existing != nil", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := ce.EvaluateExpression(tt.expression, ctx)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if result != tt.want {
				t.Errorf("EvaluateExpression() = %v, want %v", result, tt.want)
			}
		})
	}
}

func TestConditionEvaluator_DifferentContextSources(t *testing.T) {
	ce := NewConditionEvaluator()
	ctx := &ConditionContext{
		Facts: map[string]interface{}{
			"os": "Linux",
		},
		Grains: map[string]interface{}{
			"environment": "production",
		},
		Pillar: map[string]interface{}{
			"db_host": "db.example.com",
		},
		Variables: map[string]interface{}{
			"version": "1.0.0",
		},
	}

	tests := []struct {
		name       string
		expression string
		want       bool
	}{
		{"facts source", "facts.os == 'Linux'", true},
		{"grains source", "grains.environment == 'production'", true},
		{"pillar source", "pillar.db_host == 'db.example.com'", true},
		{"vars source", "vars.version == '1.0.0'", true},
		{"variables source", "variables.version == '1.0.0'", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := ce.EvaluateExpression(tt.expression, ctx)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if result != tt.want {
				t.Errorf("EvaluateExpression() = %v, want %v", result, tt.want)
			}
		})
	}
}

func TestConditionalStateDeclaration_ShouldExecute(t *testing.T) {
	ctx := NewConditionContext(map[string]interface{}{
		"os":   "Ubuntu",
		"arch": "amd64",
		"env":  "production",
	})

	tests := []struct {
		name        string
		state       ConditionalStateDeclaration
		wantExecute bool
		wantReason  bool
	}{
		{
			name: "no conditions",
			state: ConditionalStateDeclaration{
				StateDeclaration: StateDeclaration{ID: "test"},
			},
			wantExecute: true,
			wantReason:  false,
		},
		{
			name: "when all true",
			state: ConditionalStateDeclaration{
				StateDeclaration: StateDeclaration{ID: "test"},
				When:             []string{"facts.os == 'Ubuntu'", "facts.arch == 'amd64'"},
			},
			wantExecute: true,
			wantReason:  false,
		},
		{
			name: "when one false",
			state: ConditionalStateDeclaration{
				StateDeclaration: StateDeclaration{ID: "test"},
				When:             []string{"facts.os == 'Ubuntu'", "facts.arch == 'arm64'"},
			},
			wantExecute: false,
			wantReason:  true,
		},
		{
			name: "when_any one true",
			state: ConditionalStateDeclaration{
				StateDeclaration: StateDeclaration{ID: "test"},
				WhenAny:          []string{"facts.os == 'CentOS'", "facts.os == 'Ubuntu'"},
			},
			wantExecute: true,
			wantReason:  false,
		},
		{
			name: "when_any none true",
			state: ConditionalStateDeclaration{
				StateDeclaration: StateDeclaration{ID: "test"},
				WhenAny:          []string{"facts.os == 'CentOS'", "facts.os == 'Debian'"},
			},
			wantExecute: false,
			wantReason:  true,
		},
		{
			name: "when_not all false",
			state: ConditionalStateDeclaration{
				StateDeclaration: StateDeclaration{ID: "test"},
				WhenNot:          []string{"facts.os == 'Windows'", "facts.arch == 'arm64'"},
			},
			wantExecute: true,
			wantReason:  false,
		},
		{
			name: "when_not one true",
			state: ConditionalStateDeclaration{
				StateDeclaration: StateDeclaration{ID: "test"},
				WhenNot:          []string{"facts.os == 'Ubuntu'"},
			},
			wantExecute: false,
			wantReason:  true,
		},
		{
			name: "combined conditions",
			state: ConditionalStateDeclaration{
				StateDeclaration: StateDeclaration{ID: "test"},
				When:             []string{"facts.os == 'Ubuntu'"},
				WhenAny:          []string{"facts.env == 'production'", "facts.env == 'staging'"},
				WhenNot:          []string{"facts.arch == 'arm64'"},
			},
			wantExecute: true,
			wantReason:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			shouldExec, reason, err := tt.state.ShouldExecute(ctx)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if shouldExec != tt.wantExecute {
				t.Errorf("ShouldExecute() = %v, want %v", shouldExec, tt.wantExecute)
			}
			hasReason := reason != ""
			if hasReason != tt.wantReason {
				t.Errorf("has reason = %v (reason: %s), want hasReason %v", hasReason, reason, tt.wantReason)
			}
		})
	}
}

func TestFilterStates(t *testing.T) {
	ctx := NewConditionContext(map[string]interface{}{
		"os": "Ubuntu",
	})

	states := []ConditionalStateDeclaration{
		{
			StateDeclaration: StateDeclaration{ID: "package1", Module: "package"},
			When:             []string{"facts.os == 'Ubuntu'"},
		},
		{
			StateDeclaration: StateDeclaration{ID: "package2", Module: "package"},
			When:             []string{"facts.os == 'CentOS'"},
		},
		{
			StateDeclaration: StateDeclaration{ID: "service1", Module: "service"},
		},
	}

	executed, skipped := FilterStates(states, ctx)

	if len(executed) != 2 {
		t.Errorf("expected 2 executed states, got %d", len(executed))
	}

	if len(skipped) != 1 {
		t.Errorf("expected 1 skipped state, got %d", len(skipped))
	}

	// Verify correct states are in each list
	executedIDs := make(map[string]bool)
	for _, s := range executed {
		executedIDs[s.ID] = true
	}

	if !executedIDs["package1"] {
		t.Error("expected package1 to be executed")
	}
	if !executedIDs["service1"] {
		t.Error("expected service1 to be executed")
	}

	if skipped[0].StateID != "package2" {
		t.Errorf("expected package2 to be skipped, got %s", skipped[0].StateID)
	}
}

func TestConditionEvaluator_EvaluateNilCondition(t *testing.T) {
	ce := NewConditionEvaluator()
	ctx := NewConditionContext(nil)

	result, err := ce.Evaluate(nil, ctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result {
		t.Error("expected nil condition to return true")
	}
}

func TestConditionEvaluator_EvaluateWithNilContext(t *testing.T) {
	ce := NewConditionEvaluator()

	condition, err := ce.Parse("facts.test == 'value'")
	if err != nil {
		t.Fatalf("unexpected parse error: %v", err)
	}

	// Should use default context
	result, err := ce.Evaluate(condition, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Will be false because facts.test doesn't exist in default context
	if result {
		t.Error("expected false when field doesn't exist")
	}
}

func TestParseListLiteral(t *testing.T) {
	ce := NewConditionEvaluator()
	ctx := NewConditionContext(map[string]interface{}{
		"value": "b",
	})

	tests := []struct {
		name       string
		expression string
		want       bool
	}{
		{"empty list", "facts.value in []", false},
		{"single item list", "facts.value in ['b']", true},
		{"multiple items", "facts.value in ['a', 'b', 'c']", true},
		{"not in list", "facts.value in ['x', 'y', 'z']", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := ce.EvaluateExpression(tt.expression, ctx)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if result != tt.want {
				t.Errorf("EvaluateExpression() = %v, want %v", result, tt.want)
			}
		})
	}
}

// Tests for utility functions

func TestToBool(t *testing.T) {
	tests := []struct {
		name  string
		input interface{}
		want  bool
	}{
		// nil
		{"nil", nil, false},

		// bool
		{"bool true", true, true},
		{"bool false", false, false},

		// string
		{"empty string", "", false},
		{"non-empty string", "hello", true},
		{"whitespace string", " ", true},

		// int
		{"int zero", int(0), false},
		{"int positive", int(42), true},
		{"int negative", int(-1), true},

		// int64
		{"int64 zero", int64(0), false},
		{"int64 positive", int64(100), true},
		{"int64 negative", int64(-100), true},

		// float64
		{"float64 zero", float64(0), false},
		{"float64 positive", float64(3.14), true},
		{"float64 negative", float64(-1.5), true},

		// slice
		{"empty slice", []interface{}{}, false},
		{"non-empty slice", []interface{}{"a", "b"}, true},
		{"slice with nil", []interface{}{nil}, true},

		// map
		{"empty map", map[string]interface{}{}, false},
		{"non-empty map", map[string]interface{}{"key": "value"}, true},

		// other types (default case)
		{"struct", struct{ name string }{"test"}, true},
		{"pointer", new(int), true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := toBool(tt.input)
			if got != tt.want {
				t.Errorf("toBool(%v) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}

func TestToString(t *testing.T) {
	tests := []struct {
		name   string
		input  interface{}
		want   string
		wantOk bool
	}{
		{"string", "hello", "hello", true},
		{"empty string", "", "", true},
		{"int", 42, "42", true},
		{"int negative", -10, "-10", true},
		{"int64", int64(12345678901234), "12345678901234", true},
		{"float64", 3.14, "3.14", true},
		{"float64 whole", float64(5), "5", true},
		{"bool true", true, "true", true},
		{"bool false", false, "false", true},
		{"nil", nil, "<nil>", true},
		{"slice", []int{1, 2, 3}, "[1 2 3]", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := toString(tt.input)
			if ok != tt.wantOk {
				t.Errorf("toString(%v) ok = %v, want %v", tt.input, ok, tt.wantOk)
			}
			if got != tt.want {
				t.Errorf("toString(%v) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestToFloat64(t *testing.T) {
	tests := []struct {
		name   string
		input  interface{}
		want   float64
		wantOk bool
	}{
		{"float64", 3.14, 3.14, true},
		{"float32", float32(2.5), 2.5, true},
		{"int", 42, 42.0, true},
		{"int64", int64(100), 100.0, true},
		{"int32", int32(50), 50.0, true},
		{"string number", "123.45", 123.45, true},
		{"string int", "42", 42.0, true},
		{"string invalid", "not a number", 0, false},
		{"empty string", "", 0, false},
		{"bool", true, 0, false},
		{"nil", nil, 0, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := toFloat64(tt.input)
			if ok != tt.wantOk {
				t.Errorf("toFloat64(%v) ok = %v, want %v", tt.input, ok, tt.wantOk)
			}
			if ok && got != tt.want {
				t.Errorf("toFloat64(%v) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}

func TestCompareEqual(t *testing.T) {
	tests := []struct {
		name string
		a    interface{}
		b    interface{}
		want bool
	}{
		// nil cases
		{"both nil", nil, nil, true},
		{"a nil", nil, "value", false},
		{"b nil", "value", nil, false},

		// string comparisons
		{"strings equal", "hello", "hello", true},
		{"strings not equal", "hello", "world", false},
		{"empty strings", "", "", true},

		// numeric comparisons
		{"ints equal", 42, 42, true},
		{"ints not equal", 42, 43, false},
		{"int and float equal", 42, 42.0, true},
		{"int and float not equal", 42, 42.5, false},
		{"int64 equal", int64(100), int64(100), true},
		{"float64 equal", 3.14, 3.14, true},

		// bool comparisons
		{"bools equal true", true, true, true},
		{"bools equal false", false, false, true},
		{"bools not equal", true, false, false},

		// string and number
		{"string number equal", "42", 42, true},
		{"string number not equal", "42", 43, false},

		// mixed types fallback
		{"slice equal", []int{1, 2}, []int{1, 2}, true},
		{"slice not equal", []int{1, 2}, []int{1, 3}, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := compareEqual(tt.a, tt.b)
			if got != tt.want {
				t.Errorf("compareEqual(%v, %v) = %v, want %v", tt.a, tt.b, got, tt.want)
			}
		})
	}
}

func TestCompareGreater(t *testing.T) {
	tests := []struct {
		name string
		a    interface{}
		b    interface{}
		want bool
	}{
		// numeric comparisons
		{"int greater", 10, 5, true},
		{"int not greater equal", 5, 5, false},
		{"int not greater less", 3, 5, false},
		{"float greater", 3.14, 2.71, true},
		{"float not greater", 2.71, 3.14, false},
		{"int float greater", 10, 5.5, true},
		{"string numbers", "100", "50", true},

		// string comparisons (lexicographic)
		{"string greater", "b", "a", true},
		{"string not greater", "a", "b", false},
		{"string equal", "a", "a", false},
		{"string longer", "abc", "ab", true},

		// string comparison fallback (when numeric fails)
		// Note: bools/mixed types convert to strings and compare lexicographically
		{"bool true > false lexicographic", true, false, true}, // "true" > "false"
		{"nil vs nil", nil, nil, false},
		{"string vs int", "hello", 42, true}, // "hello" > "42" lexicographically
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := compareGreater(tt.a, tt.b)
			if got != tt.want {
				t.Errorf("compareGreater(%v, %v) = %v, want %v", tt.a, tt.b, got, tt.want)
			}
		})
	}
}

func TestCompareLess(t *testing.T) {
	tests := []struct {
		name string
		a    interface{}
		b    interface{}
		want bool
	}{
		// numeric comparisons
		{"int less", 5, 10, true},
		{"int not less equal", 5, 5, false},
		{"int not less greater", 10, 5, false},
		{"float less", 2.71, 3.14, true},
		{"float not less", 3.14, 2.71, false},
		{"int float less", 5, 10.5, true},

		// string comparisons (lexicographic)
		{"string less", "a", "b", true},
		{"string not less", "b", "a", false},
		{"string equal", "a", "a", false},
		{"string shorter", "ab", "abc", true},

		// incomparable types
		{"bool false", true, false, false},
		{"nil false", nil, nil, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := compareLess(tt.a, tt.b)
			if got != tt.want {
				t.Errorf("compareLess(%v, %v) = %v, want %v", tt.a, tt.b, got, tt.want)
			}
		})
	}
}

func TestGetNestedValue(t *testing.T) {
	data := map[string]interface{}{
		"level1": map[string]interface{}{
			"level2": map[string]interface{}{
				"value": "deep",
			},
			"simple": "shallow",
		},
		"top": "surface",
	}

	tests := []struct {
		name string
		key  string
		want interface{}
	}{
		{"top level", "top", "surface"},
		{"one level deep", "level1.simple", "shallow"},
		{"two levels deep", "level1.level2.value", "deep"},
		{"nonexistent key", "missing", nil},
		{"nonexistent nested key", "level1.missing", nil},
		{"path through non-map", "top.invalid", nil},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := getNestedValue(data, tt.key)
			if got != tt.want {
				t.Errorf("getNestedValue(data, %q) = %v, want %v", tt.key, got, tt.want)
			}
		})
	}
}

func TestMatchRegex(t *testing.T) {
	tests := []struct {
		name    string
		value   interface{}
		pattern interface{}
		want    bool
	}{
		{"simple match", "hello world", "hello.*", true},
		{"no match", "hello world", "^goodbye", false},
		{"exact match", "test", "^test$", true},
		{"partial match", "testing", "test", true},
		{"case sensitive no match", "Hello", "hello", false},
		{"numeric value", 12345, "123", true},
		{"invalid regex", "test", "[invalid", false},
		{"empty pattern", "test", "", true},
		{"empty value", "", ".*", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := matchRegex(tt.value, tt.pattern)
			if got != tt.want {
				t.Errorf("matchRegex(%v, %v) = %v, want %v", tt.value, tt.pattern, got, tt.want)
			}
		})
	}
}

func TestContainsValue(t *testing.T) {
	tests := []struct {
		name      string
		container interface{}
		value     interface{}
		want      bool
	}{
		// list contains
		{"list contains string", []interface{}{"a", "b", "c"}, "b", true},
		{"list not contains", []interface{}{"a", "b", "c"}, "d", false},
		{"list contains int", []interface{}{1, 2, 3}, 2, true},
		{"list contains mixed", []interface{}{"a", 1, true}, true, true},
		{"empty list", []interface{}{}, "a", false},

		// string contains
		{"string contains substring", "hello world", "world", true},
		{"string not contains", "hello world", "foo", false},
		{"string contains empty", "hello", "", true},

		// map contains key
		{"map contains key", map[string]interface{}{"a": 1, "b": 2}, "a", true},
		{"map not contains key", map[string]interface{}{"a": 1, "b": 2}, "c", false},

		// incompatible types
		{"int container", 42, "test", false},
		{"nil container", nil, "test", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := containsValue(tt.container, tt.value)
			if got != tt.want {
				t.Errorf("containsValue(%v, %v) = %v, want %v", tt.container, tt.value, got, tt.want)
			}
		})
	}
}

func TestGetNestedValueWithMapInterfaceInterface(t *testing.T) {
	// Test with map[interface{}]interface{} which can come from YAML parsing
	data := map[string]interface{}{
		"level1": map[interface{}]interface{}{
			"level2": "value",
		},
	}

	got := getNestedValue(data, "level1.level2")
	if got != "value" {
		t.Errorf("getNestedValue with map[interface{}]interface{} = %v, want 'value'", got)
	}
}

func TestCompareEqualBooleans(t *testing.T) {
	tests := []struct {
		name string
		a    interface{}
		b    interface{}
		want bool
	}{
		// bool == bool
		{"true == true", true, true, true},
		{"false == false", false, false, true},
		{"true == false", true, false, false},
		{"false == true", false, true, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := compareEqual(tt.a, tt.b)
			if got != tt.want {
				t.Errorf("compareEqual(%v, %v) = %v, want %v", tt.a, tt.b, got, tt.want)
			}
		})
	}
}

func TestCompareEqualFallback(t *testing.T) {
	// Test the fmt.Sprintf fallback case (line 656 in conditional.go)
	// This happens when neither string, numeric, nor bool comparison works

	// Using slices which aren't directly comparable
	tests := []struct {
		name string
		a    interface{}
		b    interface{}
		want bool
	}{
		{"slice equal", []interface{}{"a", "b"}, []interface{}{"a", "b"}, true},
		{"slice not equal", []interface{}{"a", "b"}, []interface{}{"c", "d"}, false},
		{"map equal", map[string]interface{}{"k": "v"}, map[string]interface{}{"k": "v"}, true},
		{"map not equal", map[string]interface{}{"k": "v"}, map[string]interface{}{"k": "x"}, false},
		// Struct types
		{"struct equal", struct{ x int }{1}, struct{ x int }{1}, true},
		{"struct not equal", struct{ x int }{1}, struct{ x int }{2}, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := compareEqual(tt.a, tt.b)
			if got != tt.want {
				t.Errorf("compareEqual(%v, %v) = %v, want %v", tt.a, tt.b, got, tt.want)
			}
		})
	}
}

func TestContainsValueEdgeCases(t *testing.T) {
	tests := []struct {
		name      string
		container interface{}
		value     interface{}
		want      bool
	}{
		// string container with non-string value - converts value to string
		{"string container with int value", "abc123", 123, true},
		{"string container with int value not found", "abcdef", 123, false},

		// map with non-string key value
		{"map with int value as key", map[string]interface{}{"a": 1}, 123, false},

		// list with numeric type matching
		{"list contains float", []interface{}{1.5, 2.5, 3.5}, 2.5, true},
		{"list contains string numeric", []interface{}{"1", "2", "3"}, 2, true}, // "2" == 2 via compareEqual
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := containsValue(tt.container, tt.value)
			if got != tt.want {
				t.Errorf("containsValue(%v, %v) = %v, want %v", tt.container, tt.value, got, tt.want)
			}
		})
	}
}

func TestStartsWithValue(t *testing.T) {
	tests := []struct {
		name string
		a    interface{}
		b    interface{}
		want bool
	}{
		// string starts with
		{"string starts with", "hello world", "hello", true},
		{"string not starts with", "hello world", "world", false},
		{"empty prefix", "hello", "", true},
		{"equal strings", "test", "test", true},
		{"prefix longer than string", "hi", "hello", false},

		// numeric values (converted to strings)
		{"int starts with", 12345, 12, true},
		{"int not starts with", 12345, 34, false},

		// non-string types that can't be converted
		{"bool values", true, true, true},      // "true" starts with "true"
		{"bool with string", true, "tr", true}, // "true" starts with "tr"
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := startsWithValue(tt.a, tt.b)
			if got != tt.want {
				t.Errorf("startsWithValue(%v, %v) = %v, want %v", tt.a, tt.b, got, tt.want)
			}
		})
	}
}

func TestEndsWithValue(t *testing.T) {
	tests := []struct {
		name string
		a    interface{}
		b    interface{}
		want bool
	}{
		// string ends with
		{"string ends with", "hello world", "world", true},
		{"string not ends with", "hello world", "hello", false},
		{"empty suffix", "hello", "", true},
		{"equal strings", "test", "test", true},
		{"suffix longer than string", "hi", "hello", false},

		// numeric values (converted to strings)
		{"int ends with", 12345, 45, true},
		{"int not ends with", 12345, 12, false},

		// bool and string combinations
		{"bool values", true, "true", true},    // "true" ends with "true"
		{"bool with string", true, "ue", true}, // "true" ends with "ue"
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := endsWithValue(tt.a, tt.b)
			if got != tt.want {
				t.Errorf("endsWithValue(%v, %v) = %v, want %v", tt.a, tt.b, got, tt.want)
			}
		})
	}
}

func TestIsValidFieldReference(t *testing.T) {
	tests := []struct {
		name  string
		field string
		want  bool
	}{
		// valid field references
		{"simple field", "name", true},
		{"dotted field", "vars.name", true},
		{"deep field", "vars.database.host", true},
		{"underscore", "my_var", true},
		{"number in field", "var1", true},
		{"mixed case", "MyVar", true},

		// invalid field references
		{"empty string", "", false},
		{"starts with number", "1var", false},
		{"starts with dot", ".var", false},
		{"has space", "my var", false},
		{"special chars", "my@var", false},
		{"ends with dot", "var.", false},
		{"consecutive dots", "var..name", false},
		{"hyphen", "my-var", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isValidFieldReference(tt.field)
			if got != tt.want {
				t.Errorf("isValidFieldReference(%q) = %v, want %v", tt.field, got, tt.want)
			}
		})
	}
}

func TestAreParenthesesBalanced(t *testing.T) {
	tests := []struct {
		name string
		expr string
		want bool
	}{
		{"no parens", "a == b", true},
		{"balanced single", "(a == b)", true},
		{"balanced nested", "((a == b))", true},
		{"balanced complex", "(a == b) and (c == d)", true},
		{"unbalanced open", "(a == b", false},
		{"unbalanced close", "a == b)", false},
		{"unbalanced nested", "((a == b)", false},
		{"empty parens", "()", true},
		{"empty string", "", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := areParenthesesBalanced(tt.expr)
			if got != tt.want {
				t.Errorf("areParenthesesBalanced(%q) = %v, want %v", tt.expr, got, tt.want)
			}
		})
	}
}

func TestCompareGreaterEdgeCases(t *testing.T) {
	// Note: toString always returns true (via fmt.Sprintf fallback), so
	// comparison always falls back to string comparison for non-numeric types.
	tests := []struct {
		name string
		a    interface{}
		b    interface{}
		want bool
	}{
		// Falls back to string comparison: "[1]" vs "[2]" - "[1]" < "[2]"
		{"slice vs slice [1] > [2]", []interface{}{1}, []interface{}{2}, false},
		// nil converts to "<nil>" via fmt.Sprintf
		{"nil vs nil", nil, nil, false}, // "<nil>" > "<nil>" is false
		// "<nil>" (60) > "1" (49) - '<' has higher ASCII than '1'
		{"nil vs value", nil, 1, true}, // "<nil>" > "1" is true
		// "1" (49) > "<nil>" (60) - '1' has lower ASCII than '<'
		{"value vs nil", 1, nil, false}, // "1" > "<nil>" is false
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := compareGreater(tt.a, tt.b)
			if got != tt.want {
				t.Errorf("compareGreater(%v, %v) = %v, want %v", tt.a, tt.b, got, tt.want)
			}
		})
	}
}

func TestCompareLessEdgeCases(t *testing.T) {
	// Note: toString always returns true (via fmt.Sprintf fallback), so
	// comparison always falls back to string comparison for non-numeric types.
	tests := []struct {
		name string
		a    interface{}
		b    interface{}
		want bool
	}{
		// Falls back to string comparison: "[1]" vs "[2]" - "[1]" < "[2]"
		{"slice vs slice [1] < [2]", []interface{}{1}, []interface{}{2}, true},
		// nil converts to "<nil>" via fmt.Sprintf
		{"nil vs nil", nil, nil, false}, // "<nil>" < "<nil>" is false
		// "<nil>" (60) < "1" (49) - '<' has higher ASCII than '1'
		{"nil vs value", nil, 1, false}, // "<nil>" < "1" is false
		// "1" (49) < "<nil>" (60) - '1' has lower ASCII than '<'
		{"value vs nil", 1, nil, true}, // "1" < "<nil>" is true
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := compareLess(tt.a, tt.b)
			if got != tt.want {
				t.Errorf("compareLess(%v, %v) = %v, want %v", tt.a, tt.b, got, tt.want)
			}
		})
	}
}
