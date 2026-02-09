package events

import (
	"fmt"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// FilterExpression represents a parsed filter expression
type FilterExpression interface {
	Matches(event *Event) bool
	String() string
}

// ComparisonOp represents comparison operators
type ComparisonOp string

// OpEqual constants define the operators.
const (
	OpEqual              ComparisonOp = "=="
	OpNotEqual           ComparisonOp = "!="
	OpGreaterThan        ComparisonOp = ">"
	OpGreaterThanOrEqual ComparisonOp = ">="
	OpLessThan           ComparisonOp = "<"
	OpLessThanOrEqual    ComparisonOp = "<="
	OpRegexMatch         ComparisonOp = "=~"
	OpGlobMatch          ComparisonOp = "~~"
	OpContains           ComparisonOp = "contains"
)

// LogicalOp represents logical operators
type LogicalOp string

// OpAnd constants define the operators.
const (
	OpAnd LogicalOp = "AND"
	OpOr  LogicalOp = "OR"
	OpNot LogicalOp = "NOT"
)

// TimestampValue wraps a time.Time for filter expressions
type TimestampValue struct {
	Time time.Time
}

// String returns the RFC3339 representation
func (t TimestampValue) String() string {
	return t.Time.Format(time.RFC3339)
}

// DurationValue wraps a time.Duration for filter expressions
type DurationValue struct {
	Duration time.Duration
}

// String returns the duration string representation
func (d DurationValue) String() string {
	return d.Duration.String()
}

// ComparisonExpr represents a comparison expression (e.g., type == "agent.connect")
type ComparisonExpr struct {
	Field    string
	Operator ComparisonOp
	Value    interface{}
}

// Matches tests whether the expression matches the given event.
func (e *ComparisonExpr) Matches(event *Event) bool {
	fieldValue := e.getFieldValue(event)

	switch e.Operator {
	case OpEqual:
		return e.compareEqual(fieldValue, e.Value)
	case OpNotEqual:
		return !e.compareEqual(fieldValue, e.Value)
	case OpGreaterThan:
		return e.compareGreater(fieldValue, e.Value, false)
	case OpGreaterThanOrEqual:
		return e.compareGreater(fieldValue, e.Value, true)
	case OpLessThan:
		return e.compareLess(fieldValue, e.Value, false)
	case OpLessThanOrEqual:
		return e.compareLess(fieldValue, e.Value, true)
	case OpRegexMatch:
		return e.matchRegex(fieldValue, e.Value)
	case OpGlobMatch:
		return e.matchGlob(fieldValue, e.Value)
	case OpContains:
		return e.matchContains(fieldValue, e.Value)
	default:
		return false
	}
}

func (e *ComparisonExpr) String() string {
	return fmt.Sprintf("%s %s %v", e.Field, e.Operator, e.Value)
}

// getFieldValue extracts field value from event
func (e *ComparisonExpr) getFieldValue(event *Event) interface{} {
	parts := strings.Split(e.Field, ".")

	switch parts[0] {
	case "type":
		return string(event.Type)
	case "source":
		return event.Source
	case "severity":
		return string(event.Severity)
	case "correlation_id":
		return event.CorrelationID
	case "timestamp":
		return TimestampValue{Time: event.Time}
	case "tags":
		if len(parts) == 2 {
			return event.Tags[parts[1]]
		}
		return nil
	case "data":
		if len(parts) >= 2 {
			return getNestedValue(event.Data, parts[1:])
		}
		return nil
	default:
		return nil
	}
}

// getNestedValue navigates through nested maps and slices to retrieve a value
// path is a slice of keys to navigate, e.g., ["results", "success"]
func getNestedValue(data interface{}, path []string) interface{} {
	if len(path) == 0 || data == nil {
		return data
	}

	key := path[0]
	remaining := path[1:]

	// Handle map[string]interface{} (most common for JSON data)
	if m, ok := data.(map[string]interface{}); ok {
		value, exists := m[key]
		if !exists {
			return nil
		}
		if len(remaining) == 0 {
			return value
		}
		return getNestedValue(value, remaining)
	}

	// Handle map[interface{}]interface{} (YAML-style maps)
	if m, ok := data.(map[interface{}]interface{}); ok {
		value, exists := m[key]
		if !exists {
			return nil
		}
		if len(remaining) == 0 {
			return value
		}
		return getNestedValue(value, remaining)
	}

	// Handle slices with numeric index
	if slice, ok := data.([]interface{}); ok {
		idx, err := strconv.Atoi(key)
		if err != nil || idx < 0 || idx >= len(slice) {
			return nil
		}
		if len(remaining) == 0 {
			return slice[idx]
		}
		return getNestedValue(slice[idx], remaining)
	}

	return nil
}

// compareEqual checks equality
func (e *ComparisonExpr) compareEqual(a, b interface{}) bool {
	if a == nil || b == nil {
		return a == b
	}

	// For timestamp comparison
	if aTS, ok := a.(TimestampValue); ok {
		if bTS, ok := b.(TimestampValue); ok {
			return aTS.Time.Equal(bTS.Time)
		}
	}

	// For duration comparison
	if aDur, ok := a.(DurationValue); ok {
		if bDur, ok := b.(DurationValue); ok {
			return aDur.Duration == bDur.Duration
		}
	}

	// Convert to strings for comparison
	aStr := fmt.Sprintf("%v", a)
	bStr := fmt.Sprintf("%v", b)
	return aStr == bStr
}

// compareGreater checks if a > b (or >= if orEqual is true)
func (e *ComparisonExpr) compareGreater(a, b interface{}, orEqual bool) bool {
	// For timestamp comparison
	if aTS, ok := a.(TimestampValue); ok {
		if bTS, ok := b.(TimestampValue); ok {
			if orEqual {
				return !aTS.Time.Before(bTS.Time)
			}
			return aTS.Time.After(bTS.Time)
		}
	}

	// For duration comparison
	if aDur, ok := a.(DurationValue); ok {
		if bDur, ok := b.(DurationValue); ok {
			if orEqual {
				return aDur.Duration >= bDur.Duration
			}
			return aDur.Duration > bDur.Duration
		}
	}

	// For severity comparison
	if aSev, ok := a.(string); ok {
		if bSev, ok := b.(string); ok {
			return e.compareSeverity(Severity(aSev), Severity(bSev), orEqual, true)
		}
	}

	// For numeric comparison
	aNum, aErr := e.toNumber(a)
	bNum, bErr := e.toNumber(b)
	if aErr == nil && bErr == nil {
		if orEqual {
			return aNum >= bNum
		}
		return aNum > bNum
	}

	return false
}

// compareLess checks if a < b (or <= if orEqual is true)
func (e *ComparisonExpr) compareLess(a, b interface{}, orEqual bool) bool {
	// For timestamp comparison
	if aTS, ok := a.(TimestampValue); ok {
		if bTS, ok := b.(TimestampValue); ok {
			if orEqual {
				return !aTS.Time.After(bTS.Time)
			}
			return aTS.Time.Before(bTS.Time)
		}
	}

	// For duration comparison
	if aDur, ok := a.(DurationValue); ok {
		if bDur, ok := b.(DurationValue); ok {
			if orEqual {
				return aDur.Duration <= bDur.Duration
			}
			return aDur.Duration < bDur.Duration
		}
	}

	// For severity comparison
	if aSev, ok := a.(string); ok {
		if bSev, ok := b.(string); ok {
			return e.compareSeverity(Severity(aSev), Severity(bSev), orEqual, false)
		}
	}

	// For numeric comparison
	aNum, aErr := e.toNumber(a)
	bNum, bErr := e.toNumber(b)
	if aErr == nil && bErr == nil {
		if orEqual {
			return aNum <= bNum
		}
		return aNum < bNum
	}

	return false
}

// compareSeverity compares severity levels
func (e *ComparisonExpr) compareSeverity(a, b Severity, orEqual, greater bool) bool {
	levels := map[Severity]int{
		SeverityDebug:    0,
		SeverityInfo:     1,
		SeverityWarning:  2,
		SeverityError:    3,
		SeverityCritical: 4,
	}

	aLevel := levels[a]
	bLevel := levels[b]

	if greater {
		if orEqual {
			return aLevel >= bLevel
		}
		return aLevel > bLevel
	}
	if orEqual {
		return aLevel <= bLevel
	}
	return aLevel < bLevel
}

// toNumber converts interface{} to float64
func (e *ComparisonExpr) toNumber(v interface{}) (float64, error) {
	switch val := v.(type) {
	case int:
		return float64(val), nil
	case int64:
		return float64(val), nil
	case float64:
		return val, nil
	case string:
		return strconv.ParseFloat(val, 64)
	default:
		return 0, fmt.Errorf("not a number")
	}
}

// matchRegex checks regex match
func (e *ComparisonExpr) matchRegex(fieldValue, pattern interface{}) bool {
	fieldStr := fmt.Sprintf("%v", fieldValue)
	patternStr := fmt.Sprintf("%v", pattern)

	re, err := regexp.Compile(patternStr)
	if err != nil {
		return false
	}

	return re.MatchString(fieldStr)
}

// matchGlob checks glob pattern match
func (e *ComparisonExpr) matchGlob(fieldValue, pattern interface{}) bool {
	fieldStr := fmt.Sprintf("%v", fieldValue)
	patternStr := fmt.Sprintf("%v", pattern)

	matched, err := filepath.Match(patternStr, fieldStr)
	if err != nil {
		return false
	}

	return matched
}

// matchContains checks if field contains value
func (e *ComparisonExpr) matchContains(fieldValue, searchValue interface{}) bool {
	fieldStr := fmt.Sprintf("%v", fieldValue)
	searchStr := fmt.Sprintf("%v", searchValue)

	return strings.Contains(fieldStr, searchStr)
}

// LogicalExpr represents a logical expression (AND, OR, NOT)
type LogicalExpr struct {
	Operator LogicalOp
	Left     FilterExpression
	Right    FilterExpression // nil for NOT
}

// Matches tests whether the expression matches the given event.
func (e *LogicalExpr) Matches(event *Event) bool {
	switch e.Operator {
	case OpAnd:
		return e.Left.Matches(event) && e.Right.Matches(event)
	case OpOr:
		return e.Left.Matches(event) || e.Right.Matches(event)
	case OpNot:
		return !e.Left.Matches(event)
	default:
		return false
	}
}

func (e *LogicalExpr) String() string {
	switch e.Operator {
	case OpNot:
		return fmt.Sprintf("NOT (%s)", e.Left)
	default:
		return fmt.Sprintf("(%s %s %s)", e.Left, e.Operator, e.Right)
	}
}

// ParseFilterExpression parses a filter expression string
// Supported syntax:
//   - type == "agent.connect"
//   - severity >= "warning"
//   - source =~ "^/agents/.*"
//   - tags.env == "production"
//   - type == "agent.connect" AND severity >= "warning"
//   - type == "job.start" OR type == "job.fail"
//   - NOT (type == "agent.heartbeat")
func ParseFilterExpression(expr string) (FilterExpression, error) {
	expr = strings.TrimSpace(expr)
	if expr == "" {
		return nil, fmt.Errorf("empty expression")
	}

	// Parse logical expressions
	if result := parseLogical(expr); result != nil {
		return result, nil
	}

	// Parse comparison expression
	return parseComparison(expr)
}

// parseLogical parses logical expressions (AND, OR, NOT)
func parseLogical(expr string) FilterExpression {
	// Handle NOT
	if strings.HasPrefix(expr, "NOT ") {
		rest := strings.TrimSpace(expr[4:])
		rest = strings.Trim(rest, "()")

		left, err := ParseFilterExpression(rest)
		if err != nil {
			return nil
		}

		return &LogicalExpr{
			Operator: OpNot,
			Left:     left,
		}
	}

	// Handle OR first (lower precedence, so we want to split on it first)
	if idx := findLogicalOp(expr, " OR "); idx != -1 {
		left := expr[:idx]
		right := expr[idx+4:]

		leftExpr, err1 := ParseFilterExpression(left)
		rightExpr, err2 := ParseFilterExpression(right)

		if err1 == nil && err2 == nil {
			return &LogicalExpr{
				Operator: OpOr,
				Left:     leftExpr,
				Right:    rightExpr,
			}
		}
	}

	// Handle AND (higher precedence than OR)
	if idx := findLogicalOp(expr, " AND "); idx != -1 {
		left := expr[:idx]
		right := expr[idx+5:]

		leftExpr, err1 := ParseFilterExpression(left)
		rightExpr, err2 := ParseFilterExpression(right)

		if err1 == nil && err2 == nil {
			return &LogicalExpr{
				Operator: OpAnd,
				Left:     leftExpr,
				Right:    rightExpr,
			}
		}
	}

	return nil
}

// findLogicalOp finds logical operator outside of parentheses
func findLogicalOp(expr, op string) int {
	depth := 0
	for i := 0; i < len(expr)-len(op)+1; i++ {
		switch {
		case expr[i] == '(':
			depth++
		case expr[i] == ')':
			depth--
		case depth == 0 && strings.HasPrefix(expr[i:], op):
			return i
		}
	}
	return -1
}

// parseComparison parses comparison expression
func parseComparison(expr string) (*ComparisonExpr, error) {
	ops := []ComparisonOp{
		OpRegexMatch, OpGlobMatch, OpGreaterThanOrEqual, OpLessThanOrEqual,
		OpNotEqual, OpEqual, OpGreaterThan, OpLessThan, OpContains,
	}

	for _, op := range ops {
		parts := strings.SplitN(expr, " "+string(op)+" ", 2)
		if len(parts) == 2 {
			field := strings.TrimSpace(parts[0])
			valueStr := strings.TrimSpace(parts[1])

			// Parse function calls
			value, err := parseValue(valueStr)
			if err != nil {
				return nil, fmt.Errorf("invalid value in expression: %w", err)
			}

			return &ComparisonExpr{
				Field:    field,
				Operator: op,
				Value:    value,
			}, nil
		}
	}

	return nil, fmt.Errorf("invalid comparison expression: %s", expr)
}

// parseValue parses a value, which can be a string literal, number, or function call
func parseValue(s string) (interface{}, error) {
	s = strings.TrimSpace(s)

	// Check for timestamp() function
	if strings.HasPrefix(s, "timestamp(") && strings.HasSuffix(s, ")") {
		inner := s[10 : len(s)-1]
		inner = strings.Trim(inner, "\"'")
		t, err := time.Parse(time.RFC3339, inner)
		if err != nil {
			return nil, fmt.Errorf("invalid timestamp format: %w", err)
		}
		return TimestampValue{Time: t}, nil
	}

	// Check for duration() function
	if strings.HasPrefix(s, "duration(") && strings.HasSuffix(s, ")") {
		inner := s[9 : len(s)-1]
		inner = strings.Trim(inner, "\"'")
		d, err := time.ParseDuration(inner)
		if err != nil {
			return nil, fmt.Errorf("invalid duration format: %w", err)
		}
		return DurationValue{Duration: d}, nil
	}

	// Remove quotes for string literals
	s = strings.Trim(s, "\"'")
	return s, nil
}
