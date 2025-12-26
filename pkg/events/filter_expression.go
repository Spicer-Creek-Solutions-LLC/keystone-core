package events

import (
	"fmt"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

// FilterExpression represents a parsed filter expression
type FilterExpression interface {
	Matches(event *Event) bool
	String() string
}

// ComparisonOp represents comparison operators
type ComparisonOp string

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

const (
	OpAnd LogicalOp = "AND"
	OpOr  LogicalOp = "OR"
	OpNot LogicalOp = "NOT"
)

// ComparisonExpr represents a comparison expression (e.g., type == "agent.connect")
type ComparisonExpr struct {
	Field    string
	Operator ComparisonOp
	Value    interface{}
}

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
	case "tags":
		if len(parts) == 2 {
			return event.Tags[parts[1]]
		}
		return nil
	case "data":
		if len(parts) == 2 {
			return event.Data[parts[1]]
		}
		return nil
	default:
		return nil
	}
}

// compareEqual checks equality
func (e *ComparisonExpr) compareEqual(a, b interface{}) bool {
	if a == nil || b == nil {
		return a == b
	}

	// Convert to strings for comparison
	aStr := fmt.Sprintf("%v", a)
	bStr := fmt.Sprintf("%v", b)
	return aStr == bStr
}

// compareGreater checks if a > b (or >= if orEqual is true)
func (e *ComparisonExpr) compareGreater(a, b interface{}, orEqual bool) bool {
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
	} else {
		if orEqual {
			return aLevel <= bLevel
		}
		return aLevel < bLevel
	}
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
		if expr[i] == '(' {
			depth++
		} else if expr[i] == ')' {
			depth--
		} else if depth == 0 && strings.HasPrefix(expr[i:], op) {
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

			// Remove quotes
			valueStr = strings.Trim(valueStr, "\"'")

			return &ComparisonExpr{
				Field:    field,
				Operator: op,
				Value:    valueStr,
			}, nil
		}
	}

	return nil, fmt.Errorf("invalid comparison expression: %s", expr)
}
