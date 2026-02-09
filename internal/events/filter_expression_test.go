package events

import (
	"fmt"
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
		{"", true},                   // Empty expression
		{"   ", true},                // Whitespace only
		{"type", true},               // Incomplete expression
		{"type ==", true},            // Missing value
		{"== value", true},           // Missing field
		{"type unknown value", true}, // Invalid operator
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

// Test timestamp() and duration() functions
func TestFilterExpression_TimestampFunction(t *testing.T) {
	// Create an event with a specific timestamp
	baseTime := time.Date(2024, 1, 15, 12, 0, 0, 0, time.UTC)
	event := &Event{
		ID:     "test-1",
		Type:   EventTypeAgentConnect,
		Source: "/test",
		Time:   baseTime,
	}

	tests := []struct {
		name     string
		expr     string
		expected bool
	}{
		{
			name:     "timestamp after earlier date",
			expr:     `timestamp > timestamp('2024-01-14T00:00:00Z')`,
			expected: true,
		},
		{
			name:     "timestamp before later date",
			expr:     `timestamp < timestamp('2024-01-16T00:00:00Z')`,
			expected: true,
		},
		{
			name:     "timestamp equal to exact time",
			expr:     `timestamp == timestamp('2024-01-15T12:00:00Z')`,
			expected: true,
		},
		{
			name:     "timestamp not equal to different time",
			expr:     `timestamp != timestamp('2024-01-15T13:00:00Z')`,
			expected: true,
		},
		{
			name:     "timestamp greater than or equal to exact time",
			expr:     `timestamp >= timestamp('2024-01-15T12:00:00Z')`,
			expected: true,
		},
		{
			name:     "timestamp less than or equal to exact time",
			expr:     `timestamp <= timestamp('2024-01-15T12:00:00Z')`,
			expected: true,
		},
		{
			name:     "timestamp not after later date",
			expr:     `timestamp > timestamp('2024-01-16T00:00:00Z')`,
			expected: false,
		},
		{
			name:     "timestamp not before earlier date",
			expr:     `timestamp < timestamp('2024-01-14T00:00:00Z')`,
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			expr, err := ParseFilterExpression(tt.expr)
			if err != nil {
				t.Fatalf("Failed to parse expression: %v", err)
			}

			if expr.Matches(event) != tt.expected {
				t.Errorf("Expected %v for %s", tt.expected, tt.name)
			}
		})
	}
}

func TestFilterExpression_DurationFunction(t *testing.T) {
	// Create an event with duration data
	event := &Event{
		ID:     "test-1",
		Type:   EventTypeJobComplete,
		Source: "/test",
		Data: map[string]interface{}{
			"duration": DurationValue{Duration: 5 * time.Minute},
		},
	}

	tests := []struct {
		name     string
		expr     string
		expected bool
	}{
		{
			name:     "duration greater than shorter duration",
			expr:     `data.duration > duration('3m')`,
			expected: true,
		},
		{
			name:     "duration less than longer duration",
			expr:     `data.duration < duration('10m')`,
			expected: true,
		},
		{
			name:     "duration equal to exact duration",
			expr:     `data.duration == duration('5m')`,
			expected: true,
		},
		{
			name:     "duration equal with different format",
			expr:     `data.duration == duration('300s')`,
			expected: true,
		},
		{
			name:     "duration not equal to different duration",
			expr:     `data.duration != duration('6m')`,
			expected: true,
		},
		{
			name:     "duration greater than or equal to exact",
			expr:     `data.duration >= duration('5m')`,
			expected: true,
		},
		{
			name:     "duration less than or equal to exact",
			expr:     `data.duration <= duration('5m')`,
			expected: true,
		},
		{
			name:     "duration not greater than longer",
			expr:     `data.duration > duration('10m')`,
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			expr, err := ParseFilterExpression(tt.expr)
			if err != nil {
				t.Fatalf("Failed to parse expression: %v", err)
			}

			if expr.Matches(event) != tt.expected {
				t.Errorf("Expected %v for %s", tt.expected, tt.name)
			}
		})
	}
}

func TestParseValue_Functions(t *testing.T) {
	tests := []struct {
		name        string
		input       string
		expectType  string
		expectError bool
	}{
		{
			name:       "valid timestamp",
			input:      "timestamp('2024-01-15T12:00:00Z')",
			expectType: "TimestampValue",
		},
		{
			name:       "valid duration minutes",
			input:      "duration('5m')",
			expectType: "DurationValue",
		},
		{
			name:       "valid duration hours",
			input:      "duration('2h30m')",
			expectType: "DurationValue",
		},
		{
			name:       "valid duration seconds",
			input:      "duration('90s')",
			expectType: "DurationValue",
		},
		{
			name:       "plain string value",
			input:      `"hello"`,
			expectType: "string",
		},
		{
			name:        "invalid timestamp format",
			input:       "timestamp('not-a-date')",
			expectError: true,
		},
		{
			name:        "invalid duration format",
			input:       "duration('not-a-duration')",
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			val, err := parseValue(tt.input)
			if tt.expectError {
				if err == nil {
					t.Error("Expected error but got none")
				}
				return
			}
			if err != nil {
				t.Fatalf("Unexpected error: %v", err)
			}

			switch tt.expectType {
			case "TimestampValue":
				if _, ok := val.(TimestampValue); !ok {
					t.Errorf("Expected TimestampValue, got %T", val)
				}
			case "DurationValue":
				if _, ok := val.(DurationValue); !ok {
					t.Errorf("Expected DurationValue, got %T", val)
				}
			case "string":
				if _, ok := val.(string); !ok {
					t.Errorf("Expected string, got %T", val)
				}
			}
		})
	}
}

func TestTimestampValue_String(t *testing.T) {
	ts := TimestampValue{Time: time.Date(2024, 1, 15, 12, 30, 0, 0, time.UTC)}
	expected := "2024-01-15T12:30:00Z"
	if ts.String() != expected {
		t.Errorf("Expected %s, got %s", expected, ts.String())
	}
}

func TestDurationValue_String(t *testing.T) {
	dv := DurationValue{Duration: 5*time.Minute + 30*time.Second}
	expected := "5m30s"
	if dv.String() != expected {
		t.Errorf("Expected %s, got %s", expected, dv.String())
	}
}

// Test nested data field filtering
func TestFilterExpression_NestedDataFields(t *testing.T) {
	event := &Event{
		ID:     "test-1",
		Type:   EventTypeJobComplete,
		Source: "/test",
		Data: map[string]interface{}{
			"results": map[string]interface{}{
				"success":  true,
				"exitCode": 0,
				"outputs":  []interface{}{"line1", "line2"},
				"nested": map[string]interface{}{
					"deep": "value",
				},
			},
			"items": []interface{}{
				map[string]interface{}{"name": "first"},
				map[string]interface{}{"name": "second"},
			},
			"simple": "top-level",
		},
	}

	tests := []struct {
		name     string
		expr     string
		expected bool
	}{
		{
			name:     "single level access still works",
			expr:     `data.simple == "top-level"`,
			expected: true,
		},
		{
			name:     "two level nested access",
			expr:     `data.results.success == "true"`,
			expected: true,
		},
		{
			name:     "two level nested numeric",
			expr:     `data.results.exitCode == "0"`,
			expected: true,
		},
		{
			name:     "three level nested access",
			expr:     `data.results.nested.deep == "value"`,
			expected: true,
		},
		{
			name:     "array index access",
			expr:     `data.results.outputs.0 == "line1"`,
			expected: true,
		},
		{
			name:     "array index access second element",
			expr:     `data.results.outputs.1 == "line2"`,
			expected: true,
		},
		{
			name:     "array of objects access",
			expr:     `data.items.0.name == "first"`,
			expected: true,
		},
		{
			name:     "array of objects access second",
			expr:     `data.items.1.name == "second"`,
			expected: true,
		},
		{
			name:     "non-existent nested field",
			expr:     `data.results.nonexistent == "value"`,
			expected: false,
		},
		{
			name:     "invalid array index",
			expr:     `data.results.outputs.99 == "value"`,
			expected: false,
		},
		{
			name:     "nested not equal",
			expr:     `data.results.success != "false"`,
			expected: true,
		},
		{
			name:     "nested contains",
			expr:     `data.results.nested.deep contains "val"`,
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			expr, err := ParseFilterExpression(tt.expr)
			if err != nil {
				t.Fatalf("Failed to parse expression: %v", err)
			}

			if expr.Matches(event) != tt.expected {
				t.Errorf("Expected %v for %s", tt.expected, tt.name)
			}
		})
	}
}

func TestGetNestedValue(t *testing.T) {
	data := map[string]interface{}{
		"level1": map[string]interface{}{
			"level2": map[string]interface{}{
				"level3": "deep-value",
			},
		},
		"array": []interface{}{"a", "b", "c"},
		"mixed": []interface{}{
			map[string]interface{}{"key": "val1"},
			map[string]interface{}{"key": "val2"},
		},
	}

	tests := []struct {
		name     string
		path     []string
		expected interface{}
	}{
		{
			name:     "single level",
			path:     []string{"level1"},
			expected: map[string]interface{}{"level2": map[string]interface{}{"level3": "deep-value"}},
		},
		{
			name:     "two levels",
			path:     []string{"level1", "level2"},
			expected: map[string]interface{}{"level3": "deep-value"},
		},
		{
			name:     "three levels",
			path:     []string{"level1", "level2", "level3"},
			expected: "deep-value",
		},
		{
			name:     "array index",
			path:     []string{"array", "1"},
			expected: "b",
		},
		{
			name:     "mixed access",
			path:     []string{"mixed", "0", "key"},
			expected: "val1",
		},
		{
			name:     "non-existent key",
			path:     []string{"nonexistent"},
			expected: nil,
		},
		{
			name:     "invalid array index",
			path:     []string{"array", "10"},
			expected: nil,
		},
		{
			name:     "negative array index",
			path:     []string{"array", "-1"},
			expected: nil,
		},
		{
			name:     "non-numeric array access",
			path:     []string{"array", "abc"},
			expected: nil,
		},
		{
			name:     "empty path",
			path:     []string{},
			expected: data,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := getNestedValue(data, tt.path)
			if !compareInterfaces(result, tt.expected) {
				t.Errorf("Expected %v, got %v", tt.expected, result)
			}
		})
	}
}

// compareInterfaces is a helper to compare interface{} values
func compareInterfaces(a, b interface{}) bool {
	if a == nil && b == nil {
		return true
	}
	if a == nil || b == nil {
		return false
	}
	return fmt.Sprintf("%v", a) == fmt.Sprintf("%v", b)
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
