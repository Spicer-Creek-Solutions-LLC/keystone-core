package events

import (
	"testing"
	"time"
)

// Test comparison operators
func TestComparisonExpr_Equal(t *testing.T) {
	expr := &ComparisonExpr{
		Field:    "type",
		Operator: OpEqual,
		Value:    "agent.connect",
	}

	event := NewEvent(EventTypeAgentConnect).
		Source("/test").
		Build()

	if !expr.Matches(event) {
		t.Error("Expected match for type == agent.connect")
	}

	event2 := NewEvent(EventTypeJobStart).
		Source("/test").
		Build()

	if expr.Matches(event2) {
		t.Error("Expected no match for type == agent.connect with job.start event")
	}
}

func TestComparisonExpr_NotEqual(t *testing.T) {
	expr := &ComparisonExpr{
		Field:    "type",
		Operator: OpNotEqual,
		Value:    "agent.heartbeat",
	}

	event := NewEvent(EventTypeAgentConnect).
		Source("/test").
		Build()

	if !expr.Matches(event) {
		t.Error("Expected match for type != agent.heartbeat")
	}

	event2 := NewEvent(EventTypeAgentHeartbeat).
		Source("/test").
		Build()

	if expr.Matches(event2) {
		t.Error("Expected no match for type != agent.heartbeat with heartbeat event")
	}
}

func TestComparisonExpr_SeverityComparison(t *testing.T) {
	tests := []struct {
		name     string
		operator ComparisonOp
		value    string
		severity Severity
		expected bool
	}{
		{"warning >= warning", OpGreaterThanOrEqual, "warning", SeverityWarning, true},
		{"error >= warning", OpGreaterThanOrEqual, "warning", SeverityError, true},
		{"info >= warning", OpGreaterThanOrEqual, "warning", SeverityInfo, false},
		{"error > warning", OpGreaterThan, "warning", SeverityError, true},
		{"warning > warning", OpGreaterThan, "warning", SeverityWarning, false},
		{"info < warning", OpLessThan, "warning", SeverityInfo, true},
		{"warning < warning", OpLessThan, "warning", SeverityWarning, false},
		{"info <= warning", OpLessThanOrEqual, "warning", SeverityInfo, true},
		{"warning <= warning", OpLessThanOrEqual, "warning", SeverityWarning, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			expr := &ComparisonExpr{
				Field:    "severity",
				Operator: tt.operator,
				Value:    tt.value,
			}

			event := NewEvent(EventTypeAgentConnect).
				Source("/test").
				Severity(tt.severity).
				Build()

			if expr.Matches(event) != tt.expected {
				t.Errorf("Expected %v for %s", tt.expected, tt.name)
			}
		})
	}
}

func TestComparisonExpr_RegexMatch(t *testing.T) {
	expr := &ComparisonExpr{
		Field:    "source",
		Operator: OpRegexMatch,
		Value:    "^/agents/.*",
	}

	event := NewEvent(EventTypeAgentConnect).
		Source("/agents/test-agent").
		Build()

	if !expr.Matches(event) {
		t.Error("Expected match for source =~ ^/agents/.*")
	}

	event2 := NewEvent(EventTypeAgentConnect).
		Source("/control-plane").
		Build()

	if expr.Matches(event2) {
		t.Error("Expected no match for source =~ ^/agents/.* with /control-plane")
	}
}

func TestComparisonExpr_GlobMatch(t *testing.T) {
	expr := &ComparisonExpr{
		Field:    "source",
		Operator: OpGlobMatch,
		Value:    "/agents/*",
	}

	event := NewEvent(EventTypeAgentConnect).
		Source("/agents/test-agent").
		Build()

	if !expr.Matches(event) {
		t.Error("Expected match for source ~~ /agents/*")
	}

	event2 := NewEvent(EventTypeAgentConnect).
		Source("/control-plane/test").
		Build()

	if expr.Matches(event2) {
		t.Error("Expected no match for source ~~ /agents/* with /control-plane/test")
	}
}

func TestComparisonExpr_Contains(t *testing.T) {
	expr := &ComparisonExpr{
		Field:    "source",
		Operator: OpContains,
		Value:    "agent",
	}

	event := NewEvent(EventTypeAgentConnect).
		Source("/agents/test-agent").
		Build()

	if !expr.Matches(event) {
		t.Error("Expected match for source contains 'agent'")
	}

	event2 := NewEvent(EventTypeJobStart).
		Source("/control-plane").
		Build()

	if expr.Matches(event2) {
		t.Error("Expected no match for source contains 'agent' with /control-plane")
	}
}

func TestComparisonExpr_TagsAccess(t *testing.T) {
	expr := &ComparisonExpr{
		Field:    "tags.env",
		Operator: OpEqual,
		Value:    "production",
	}

	event := NewEvent(EventTypeAgentConnect).
		Source("/test").
		Tag("env", "production").
		Build()

	if !expr.Matches(event) {
		t.Error("Expected match for tags.env == production")
	}

	event2 := NewEvent(EventTypeAgentConnect).
		Source("/test").
		Tag("env", "staging").
		Build()

	if expr.Matches(event2) {
		t.Error("Expected no match for tags.env == production with staging")
	}
}

func TestComparisonExpr_DataAccess(t *testing.T) {
	expr := &ComparisonExpr{
		Field:    "data.exit_code",
		Operator: OpEqual,
		Value:    "0",
	}

	event := NewEvent(EventTypeJobComplete).
		Source("/test").
		Data("exit_code", 0).
		Build()

	if !expr.Matches(event) {
		t.Error("Expected match for data.exit_code == 0")
	}

	event2 := NewEvent(EventTypeJobFail).
		Source("/test").
		Data("exit_code", 1).
		Build()

	if expr.Matches(event2) {
		t.Error("Expected no match for data.exit_code == 0 with exit_code=1")
	}
}

func TestComparisonExpr_CorrelationID(t *testing.T) {
	expr := &ComparisonExpr{
		Field:    "correlation_id",
		Operator: OpEqual,
		Value:    "test-correlation",
	}

	event := NewEvent(EventTypeAgentConnect).
		Source("/test").
		CorrelationID("test-correlation").
		Build()

	if !expr.Matches(event) {
		t.Error("Expected match for correlation_id == test-correlation")
	}

	event2 := NewEvent(EventTypeAgentConnect).
		Source("/test").
		CorrelationID("other-correlation").
		Build()

	if expr.Matches(event2) {
		t.Error("Expected no match for correlation_id == test-correlation with other-correlation")
	}
}

// Test logical operators
func TestLogicalExpr_AND(t *testing.T) {
	expr := &LogicalExpr{
		Operator: OpAnd,
		Left: &ComparisonExpr{
			Field:    "type",
			Operator: OpEqual,
			Value:    "agent.connect",
		},
		Right: &ComparisonExpr{
			Field:    "severity",
			Operator: OpGreaterThanOrEqual,
			Value:    "warning",
		},
	}

	// Both conditions true
	event1 := NewEvent(EventTypeAgentConnect).
		Source("/test").
		Severity(SeverityWarning).
		Build()
	if !expr.Matches(event1) {
		t.Error("Expected match for (type == agent.connect AND severity >= warning)")
	}

	// First true, second false
	event2 := NewEvent(EventTypeAgentConnect).
		Source("/test").
		Severity(SeverityInfo).
		Build()
	if expr.Matches(event2) {
		t.Error("Expected no match when severity < warning")
	}

	// First false, second true
	event3 := NewEvent(EventTypeJobStart).
		Source("/test").
		Severity(SeverityWarning).
		Build()
	if expr.Matches(event3) {
		t.Error("Expected no match when type != agent.connect")
	}
}

func TestLogicalExpr_OR(t *testing.T) {
	expr := &LogicalExpr{
		Operator: OpOr,
		Left: &ComparisonExpr{
			Field:    "type",
			Operator: OpEqual,
			Value:    "agent.connect",
		},
		Right: &ComparisonExpr{
			Field:    "type",
			Operator: OpEqual,
			Value:    "agent.disconnect",
		},
	}

	// First condition true
	event1 := NewEvent(EventTypeAgentConnect).
		Source("/test").
		Build()
	if !expr.Matches(event1) {
		t.Error("Expected match for agent.connect")
	}

	// Second condition true
	event2 := NewEvent(EventTypeAgentDisconnect).
		Source("/test").
		Build()
	if !expr.Matches(event2) {
		t.Error("Expected match for agent.disconnect")
	}

	// Both false
	event3 := NewEvent(EventTypeJobStart).
		Source("/test").
		Build()
	if expr.Matches(event3) {
		t.Error("Expected no match for job.start")
	}
}

func TestLogicalExpr_NOT(t *testing.T) {
	expr := &LogicalExpr{
		Operator: OpNot,
		Left: &ComparisonExpr{
			Field:    "type",
			Operator: OpEqual,
			Value:    "agent.heartbeat",
		},
	}

	// Should match everything except heartbeat
	event1 := NewEvent(EventTypeAgentConnect).
		Source("/test").
		Build()
	if !expr.Matches(event1) {
		t.Error("Expected match for NOT (type == agent.heartbeat) with agent.connect")
	}

	event2 := NewEvent(EventTypeAgentHeartbeat).
		Source("/test").
		Build()
	if expr.Matches(event2) {
		t.Error("Expected no match for NOT (type == agent.heartbeat) with agent.heartbeat")
	}
}

// Test expression parser
func TestParseFilterExpression_SimpleComparison(t *testing.T) {
	tests := []struct {
		expr     string
		expected string
	}{
		{`type == "agent.connect"`, `type == agent.connect`},
		{`severity >= "warning"`, `severity >= warning`},
		{`source =~ "^/agents/.*"`, `source =~ ^/agents/.*`},
		{`tags.env == "production"`, `tags.env == production`},
	}

	for _, tt := range tests {
		t.Run(tt.expr, func(t *testing.T) {
			expr, err := ParseFilterExpression(tt.expr)
			if err != nil {
				t.Fatalf("Failed to parse: %v", err)
			}

			if expr.String() != tt.expected {
				t.Errorf("Expected %s, got %s", tt.expected, expr.String())
			}
		})
	}
}

func TestParseFilterExpression_AND(t *testing.T) {
	expr, err := ParseFilterExpression(`type == "agent.connect" AND severity >= "warning"`)
	if err != nil {
		t.Fatalf("Failed to parse: %v", err)
	}

	event := NewEvent(EventTypeAgentConnect).
		Source("/test").
		Severity(SeverityWarning).
		Build()

	if !expr.Matches(event) {
		t.Error("Expected match for parsed AND expression")
	}
}

func TestParseFilterExpression_OR(t *testing.T) {
	expr, err := ParseFilterExpression(`type == "agent.connect" OR type == "agent.disconnect"`)
	if err != nil {
		t.Fatalf("Failed to parse: %v", err)
	}

	event1 := NewEvent(EventTypeAgentConnect).
		Source("/test").
		Build()
	if !expr.Matches(event1) {
		t.Error("Expected match for agent.connect")
	}

	event2 := NewEvent(EventTypeAgentDisconnect).
		Source("/test").
		Build()
	if !expr.Matches(event2) {
		t.Error("Expected match for agent.disconnect")
	}
}

func TestParseFilterExpression_NOT(t *testing.T) {
	expr, err := ParseFilterExpression(`NOT (type == "agent.heartbeat")`)
	if err != nil {
		t.Fatalf("Failed to parse: %v", err)
	}

	event1 := NewEvent(EventTypeAgentConnect).
		Source("/test").
		Build()
	if !expr.Matches(event1) {
		t.Error("Expected match for NOT heartbeat with agent.connect")
	}

	event2 := NewEvent(EventTypeAgentHeartbeat).
		Source("/test").
		Build()
	if expr.Matches(event2) {
		t.Error("Expected no match for NOT heartbeat with heartbeat")
	}
}

func TestParseFilterExpression_Complex(t *testing.T) {
	// (type == "agent.connect" OR type == "agent.disconnect") AND severity >= "warning"
	expr, err := ParseFilterExpression(`type == "agent.connect" OR type == "agent.disconnect" AND severity >= "warning"`)
	if err != nil {
		t.Fatalf("Failed to parse: %v", err)
	}

	// Note: Without parentheses, AND has higher precedence
	// This parses as: type == "agent.connect" OR (type == "agent.disconnect" AND severity >= "warning")

	event1 := NewEvent(EventTypeAgentConnect).
		Source("/test").
		Severity(SeverityInfo).
		Build()
	if !expr.Matches(event1) {
		t.Error("Expected match for agent.connect with any severity")
	}

	event2 := NewEvent(EventTypeAgentDisconnect).
		Source("/test").
		Severity(SeverityWarning).
		Build()
	if !expr.Matches(event2) {
		t.Error("Expected match for agent.disconnect with warning")
	}

	event3 := NewEvent(EventTypeAgentDisconnect).
		Source("/test").
		Severity(SeverityInfo).
		Build()
	if expr.Matches(event3) {
		t.Error("Expected no match for agent.disconnect with info severity")
	}
}

func TestParseFilterExpression_AllOperators(t *testing.T) {
	tests := []struct {
		expr     string
		operator ComparisonOp
	}{
		{`type == "test"`, OpEqual},
		{`type != "test"`, OpNotEqual},
		{`severity > "info"`, OpGreaterThan},
		{`severity >= "info"`, OpGreaterThanOrEqual},
		{`severity < "error"`, OpLessThan},
		{`severity <= "error"`, OpLessThanOrEqual},
		{`source =~ "pattern"`, OpRegexMatch},
		{`source ~~ "glob"`, OpGlobMatch},
		{`source contains "text"`, OpContains},
	}

	for _, tt := range tests {
		t.Run(string(tt.operator), func(t *testing.T) {
			expr, err := ParseFilterExpression(tt.expr)
			if err != nil {
				t.Fatalf("Failed to parse %s: %v", tt.expr, err)
			}

			compExpr, ok := expr.(*ComparisonExpr)
			if !ok {
				t.Fatal("Expected ComparisonExpr")
			}

			if compExpr.Operator != tt.operator {
				t.Errorf("Expected operator %s, got %s", tt.operator, compExpr.Operator)
			}
		})
	}
}

func TestParseFilterExpression_Errors(t *testing.T) {
	tests := []struct {
		expr        string
		shouldError bool
	}{
		{"", true},                    // Empty expression
		{"   ", true},                 // Whitespace only
		{"type", true},                // Incomplete expression
		{"type ==", true},             // Missing value
		{"== value", true},            // Missing field
		{"type unknown value", true},  // Invalid operator
	}

	for _, tt := range tests {
		t.Run(tt.expr, func(t *testing.T) {
			_, err := ParseFilterExpression(tt.expr)
			if tt.shouldError && err == nil {
				t.Error("Expected error but got none")
			}
			if !tt.shouldError && err != nil {
				t.Errorf("Unexpected error: %v", err)
			}
		})
	}
}

// Benchmark filter matching
func BenchmarkComparisonExpr_Matches(b *testing.B) {
	expr := &ComparisonExpr{
		Field:    "type",
		Operator: OpEqual,
		Value:    "agent.connect",
	}

	event := NewEvent(EventTypeAgentConnect).
		Source("/test").
		Build()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		expr.Matches(event)
	}
}

func BenchmarkLogicalExpr_Matches(b *testing.B) {
	expr := &LogicalExpr{
		Operator: OpAnd,
		Left: &ComparisonExpr{
			Field:    "type",
			Operator: OpEqual,
			Value:    "agent.connect",
		},
		Right: &ComparisonExpr{
			Field:    "severity",
			Operator: OpGreaterThanOrEqual,
			Value:    "warning",
		},
	}

	event := NewEvent(EventTypeAgentConnect).
		Source("/test").
		Severity(SeverityWarning).
		Build()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		expr.Matches(event)
	}
}

func BenchmarkRegexMatch(b *testing.B) {
	expr := &ComparisonExpr{
		Field:    "source",
		Operator: OpRegexMatch,
		Value:    "^/agents/.*",
	}

	event := NewEvent(EventTypeAgentConnect).
		Source("/agents/test-agent").
		Build()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		expr.Matches(event)
	}
}

func BenchmarkParseFilterExpression(b *testing.B) {
	expr := `type == "agent.connect" AND severity >= "warning"`

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		ParseFilterExpression(expr)
	}
}

// Test comprehensive real-world scenarios
func TestFilterExpression_RealWorldScenarios(t *testing.T) {
	scenarios := []struct {
		name     string
		expr     string
		event    *Event
		expected bool
	}{
		{
			name: "Production agent connections",
			expr: `type == "agent.connect" AND tags.env == "production"`,
			event: NewEvent(EventTypeAgentConnect).
				Source("/agents/prod-01").
				Tag("env", "production").
				Build(),
			expected: true,
		},
		{
			name: "High severity state changes",
			expr: `type == "state.change" AND severity >= "error"`,
			event: NewEvent(EventTypeStateChange).
				Source("/state-manager").
				Severity(SeverityError).
				Build(),
			expected: true,
		},
		{
			name: "Job failures excluding timeouts",
			expr: `type == "job.fail" AND NOT (data.status contains "timeout")`,
			event: NewEvent(EventTypeJobFail).
				Source("/control-plane").
				Data("status", "COMMAND_STATUS_FAILED").
				Build(),
			expected: true,
		},
		{
			name: "Critical drift in specific module",
			expr: `type == "state.drift" AND severity == "critical" AND tags.module == "file"`,
			event: NewEvent(EventTypeStateDrift).
				Source("/state-manager").
				Severity(SeverityCritical).
				Tag("module", "file").
				Build(),
			expected: true,
		},
		{
			name: "All events from agents except heartbeats",
			expr: `source =~ "^/agents/.*" AND NOT (type == "agent.heartbeat")`,
			event: NewEvent(EventTypeAgentConnect).
				Source("/agents/edge-device-01").
				Build(),
			expected: true,
		},
	}

	for _, scenario := range scenarios {
		t.Run(scenario.name, func(t *testing.T) {
			expr, err := ParseFilterExpression(scenario.expr)
			if err != nil {
				t.Fatalf("Failed to parse expression: %v", err)
			}

			if expr.Matches(scenario.event) != scenario.expected {
				t.Errorf("Expected %v for scenario %s", scenario.expected, scenario.name)
			}
		})
	}
}

// Test thread safety
func TestFilterExpression_Concurrent(t *testing.T) {
	expr, err := ParseFilterExpression(`type == "agent.connect" AND severity >= "warning"`)
	if err != nil {
		t.Fatalf("Failed to parse: %v", err)
	}

	event := NewEvent(EventTypeAgentConnect).
		Source("/test").
		Severity(SeverityWarning).
		Build()

	// Run matches concurrently
	done := make(chan bool)
	for i := 0; i < 100; i++ {
		go func() {
			for j := 0; j < 1000; j++ {
				expr.Matches(event)
			}
			done <- true
		}()
	}

	// Wait for all goroutines
	for i := 0; i < 100; i++ {
		<-done
	}
}

// Test edge cases
func TestFilterExpression_EdgeCases(t *testing.T) {
	t.Run("Nil field values", func(t *testing.T) {
		expr := &ComparisonExpr{
			Field:    "data.missing",
			Operator: OpEqual,
			Value:    "value",
		}

		event := NewEvent(EventTypeAgentConnect).
			Source("/test").
			Build()

		if expr.Matches(event) {
			t.Error("Expected no match for missing field")
		}
	})

	t.Run("Empty string comparisons", func(t *testing.T) {
		expr := &ComparisonExpr{
			Field:    "source",
			Operator: OpEqual,
			Value:    "",
		}

		event := NewEvent(EventTypeAgentConnect).
			Source("").
			Build()

		if !expr.Matches(event) {
			t.Error("Expected match for empty string == empty string")
		}
	})

	t.Run("Invalid regex pattern", func(t *testing.T) {
		expr := &ComparisonExpr{
			Field:    "source",
			Operator: OpRegexMatch,
			Value:    "[invalid(regex",
		}

		event := NewEvent(EventTypeAgentConnect).
			Source("/test").
			Build()

		// Should return false, not panic
		if expr.Matches(event) {
			t.Error("Expected no match for invalid regex")
		}
	})

	t.Run("Timestamp field access", func(t *testing.T) {
		// Time is not directly accessible through field path, but shouldn't panic
		expr := &ComparisonExpr{
			Field:    "time",
			Operator: OpEqual,
			Value:    time.Now().String(),
		}

		event := NewEvent(EventTypeAgentConnect).
			Source("/test").
			Build()

		// Should not panic, just return false
		expr.Matches(event)
	})
}
