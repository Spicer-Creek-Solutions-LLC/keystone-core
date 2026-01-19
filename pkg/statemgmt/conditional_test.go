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
