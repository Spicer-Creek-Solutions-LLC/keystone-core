package statemgmt

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// Condition represents a conditional expression for state execution
type Condition struct {
	// Expression is the conditional expression (e.g., "facts.os == 'Ubuntu'")
	Expression string

	// Parsed is the parsed condition tree
	parsed *ConditionNode
}

// ConditionNode represents a node in the condition parse tree
type ConditionNode struct {
	Type     ConditionType
	Operator string
	Left     *ConditionNode
	Right    *ConditionNode
	Value    interface{}
	Field    string
}

// ConditionType represents the type of condition node
type ConditionType int

// ConditionTypeValue constants define the supported types.
const (
	ConditionTypeValue ConditionType = iota
	ConditionTypeField
	ConditionTypeComparison
	ConditionTypeLogical
	ConditionTypeLiteral
)

// ConditionContext provides values for condition evaluation
type ConditionContext struct {
	// Facts contains system facts (os, arch, hostname, etc.)
	Facts map[string]interface{}

	// Variables contains user-defined variables
	Variables map[string]interface{}

	// Grains contains grain data (similar to facts but user-defined)
	Grains map[string]interface{}

	// Pillar contains pillar data (secret/config values)
	Pillar map[string]interface{}
}

// NewConditionContext creates a new context with the given facts
func NewConditionContext(facts map[string]interface{}) *ConditionContext {
	if facts == nil {
		facts = make(map[string]interface{})
	}
	return &ConditionContext{
		Facts:     facts,
		Variables: make(map[string]interface{}),
		Grains:    make(map[string]interface{}),
		Pillar:    make(map[string]interface{}),
	}
}

// ConditionEvaluator evaluates conditional expressions
type ConditionEvaluator struct {
	// DefaultContext is used when no context is provided
	DefaultContext *ConditionContext
}

// NewConditionEvaluator creates a new condition evaluator
func NewConditionEvaluator() *ConditionEvaluator {
	return &ConditionEvaluator{
		DefaultContext: NewConditionContext(nil),
	}
}

// Parse parses a condition expression into a Condition
func (ce *ConditionEvaluator) Parse(expression string) (*Condition, error) {
	expression = strings.TrimSpace(expression)
	if expression == "" {
		return nil, fmt.Errorf("empty condition expression")
	}

	node, err := parseExpression(expression)
	if err != nil {
		return nil, fmt.Errorf("failed to parse condition: %w", err)
	}

	return &Condition{
		Expression: expression,
		parsed:     node,
	}, nil
}

// Evaluate evaluates a condition expression with the given context
func (ce *ConditionEvaluator) Evaluate(condition *Condition, ctx *ConditionContext) (bool, error) {
	if condition == nil {
		return true, nil // No condition means always true
	}

	if ctx == nil {
		ctx = ce.DefaultContext
	}

	return evaluateNode(condition.parsed, ctx)
}

// EvaluateExpression parses and evaluates an expression in one call
func (ce *ConditionEvaluator) EvaluateExpression(expression string, ctx *ConditionContext) (bool, error) {
	condition, err := ce.Parse(expression)
	if err != nil {
		return false, err
	}
	return ce.Evaluate(condition, ctx)
}

// parseExpression parses an expression string into a node tree
func parseExpression(expr string) (*ConditionNode, error) {
	expr = strings.TrimSpace(expr)

	// Handle parentheses
	if strings.HasPrefix(expr, "(") && strings.HasSuffix(expr, ")") {
		inner := expr[1 : len(expr)-1]
		// Check if the parentheses are balanced
		if areParenthesesBalanced(inner) {
			return parseExpression(inner)
		}
	}

	// Check for logical operators (lowest precedence)
	// Split on 'or' first, then 'and'
	if idx := findLogicalOperator(expr, "or"); idx != -1 {
		left, err := parseExpression(expr[:idx])
		if err != nil {
			return nil, err
		}
		right, err := parseExpression(expr[idx+2:])
		if err != nil {
			return nil, err
		}
		return &ConditionNode{
			Type:     ConditionTypeLogical,
			Operator: "or",
			Left:     left,
			Right:    right,
		}, nil
	}

	if idx := findLogicalOperator(expr, "and"); idx != -1 {
		left, err := parseExpression(expr[:idx])
		if err != nil {
			return nil, err
		}
		right, err := parseExpression(expr[idx+3:])
		if err != nil {
			return nil, err
		}
		return &ConditionNode{
			Type:     ConditionTypeLogical,
			Operator: "and",
			Left:     left,
			Right:    right,
		}, nil
	}

	// Check for 'not' prefix
	if strings.HasPrefix(expr, "not ") {
		inner, err := parseExpression(expr[4:])
		if err != nil {
			return nil, err
		}
		return &ConditionNode{
			Type:     ConditionTypeLogical,
			Operator: "not",
			Left:     inner,
		}, nil
	}

	// Check for comparison operators (order matters - longer/compound operators first)
	operators := []string{"==", "!=", ">=", "<=", ">", "<", "=~", "!~", "not in", "in", "contains", "startswith", "endswith"}
	for _, op := range operators {
		idx := findComparisonOperator(expr, op)
		if idx == -1 {
			continue
		}
		left, err := parseExpression(expr[:idx])
		if err != nil {
			return nil, err
		}
		right, err := parseExpression(expr[idx+len(op):])
		if err != nil {
			return nil, err
		}
		return &ConditionNode{
			Type:     ConditionTypeComparison,
			Operator: op,
			Left:     left,
			Right:    right,
		}, nil
	}

	// It's a value/field reference
	return parseValue(expr)
}

// findLogicalOperator finds a logical operator not inside parentheses
func findLogicalOperator(expr, op string) int {
	depth := 0
	searchLen := len(op)
	for i := 0; i < len(expr)-searchLen+1; i++ {
		switch expr[i] {
		case '(':
			depth++
		case ')':
			depth--
		default:
			if depth == 0 {
				// Check for operator with surrounding spaces
				if expr[i:i+searchLen] == op {
					// Check for surrounding spaces or start/end
					before := i == 0 || expr[i-1] == ' '
					after := i+searchLen >= len(expr) || expr[i+searchLen] == ' '
					if before && after {
						return i
					}
				}
			}
		}
	}
	return -1
}

// findComparisonOperator finds a comparison operator not inside parentheses or strings
func findComparisonOperator(expr, op string) int {
	depth := 0
	inString := false
	stringChar := byte(0)
	searchLen := len(op)

	for i := 0; i < len(expr)-searchLen+1; i++ {
		ch := expr[i]

		if inString {
			if ch == stringChar && (i == 0 || expr[i-1] != '\\') {
				inString = false
			}
			continue
		}

		switch ch {
		case '(':
			depth++
		case ')':
			depth--
		case '"', '\'':
			inString = true
			stringChar = ch
		default:
			if depth == 0 && strings.HasPrefix(expr[i:], op) {
				// Check for word boundaries for word operators
				if op == "in" || op == "not in" || op == "contains" || op == "startswith" || op == "endswith" {
					before := i == 0 || expr[i-1] == ' '
					after := i+searchLen >= len(expr) || expr[i+searchLen] == ' '
					if before && after {
						return i
					}
				} else {
					return i
				}
			}
		}
	}
	return -1
}

// parseValue parses a value expression (field reference, literal, etc.)
func parseValue(expr string) (*ConditionNode, error) {
	expr = strings.TrimSpace(expr)

	// String literal (single or double quoted)
	if (strings.HasPrefix(expr, "'") && strings.HasSuffix(expr, "'")) ||
		(strings.HasPrefix(expr, "\"") && strings.HasSuffix(expr, "\"")) {
		value := expr[1 : len(expr)-1]
		return &ConditionNode{
			Type:  ConditionTypeLiteral,
			Value: value,
		}, nil
	}

	// Boolean literals
	if expr == "true" || expr == "True" {
		return &ConditionNode{Type: ConditionTypeLiteral, Value: true}, nil
	}
	if expr == "false" || expr == "False" {
		return &ConditionNode{Type: ConditionTypeLiteral, Value: false}, nil
	}

	// Nil/null literal
	if expr == "nil" || expr == "null" || expr == "None" {
		return &ConditionNode{Type: ConditionTypeLiteral, Value: nil}, nil
	}

	// Number literal
	if num, err := strconv.ParseFloat(expr, 64); err == nil {
		return &ConditionNode{Type: ConditionTypeLiteral, Value: num}, nil
	}

	// Integer literal
	if num, err := strconv.ParseInt(expr, 10, 64); err == nil {
		return &ConditionNode{Type: ConditionTypeLiteral, Value: num}, nil
	}

	// List literal [a, b, c]
	if strings.HasPrefix(expr, "[") && strings.HasSuffix(expr, "]") {
		return parseListLiteral(expr)
	}

	// Field reference (e.g., facts.os, grains.environment)
	if isValidFieldReference(expr) {
		return &ConditionNode{
			Type:  ConditionTypeField,
			Field: expr,
		}, nil
	}

	return nil, fmt.Errorf("invalid value expression: %s", expr)
}

// parseListLiteral parses a list literal [a, b, c]
func parseListLiteral(expr string) (*ConditionNode, error) {
	inner := strings.TrimSpace(expr[1 : len(expr)-1])
	if inner == "" {
		return &ConditionNode{Type: ConditionTypeLiteral, Value: []interface{}{}}, nil
	}

	items := splitListItems(inner)
	var values []interface{}

	for _, item := range items {
		node, err := parseValue(strings.TrimSpace(item))
		if err != nil {
			return nil, err
		}
		if node.Type == ConditionTypeLiteral {
			values = append(values, node.Value)
		} else {
			// For field references, we store the field name
			values = append(values, node)
		}
	}

	return &ConditionNode{Type: ConditionTypeLiteral, Value: values}, nil
}

// splitListItems splits list items respecting strings and nested structures
func splitListItems(s string) []string {
	var items []string
	var current strings.Builder
	depth := 0
	inString := false
	stringChar := byte(0)

	for i := 0; i < len(s); i++ {
		ch := s[i]

		if inString {
			current.WriteByte(ch)
			if ch == stringChar && (i == 0 || s[i-1] != '\\') {
				inString = false
			}
			continue
		}

		switch ch {
		case '"', '\'':
			inString = true
			stringChar = ch
			current.WriteByte(ch)
		case '[', '(':
			depth++
			current.WriteByte(ch)
		case ']', ')':
			depth--
			current.WriteByte(ch)
		case ',':
			if depth == 0 {
				items = append(items, current.String())
				current.Reset()
			} else {
				current.WriteByte(ch)
			}
		default:
			current.WriteByte(ch)
		}
	}

	if current.Len() > 0 {
		items = append(items, current.String())
	}

	return items
}

// isValidFieldReference checks if an expression is a valid field reference
func isValidFieldReference(expr string) bool {
	// Must start with a known prefix
	prefixes := []string{"facts.", "grains.", "pillar.", "vars.", "variables."}
	for _, prefix := range prefixes {
		if strings.HasPrefix(expr, prefix) {
			return true
		}
	}

	// Or be a simple identifier
	matched, _ := regexp.MatchString(`^[a-zA-Z_][a-zA-Z0-9_]*(\.[a-zA-Z_][a-zA-Z0-9_]*)*$`, expr)
	return matched
}

// areParenthesesBalanced checks if parentheses are balanced
func areParenthesesBalanced(s string) bool {
	depth := 0
	for _, ch := range s {
		switch ch {
		case '(':
			depth++
		case ')':
			depth--
			if depth < 0 {
				return false
			}
		}
	}
	return depth == 0
}

// evaluateNode evaluates a condition node with the given context
func evaluateNode(node *ConditionNode, ctx *ConditionContext) (bool, error) {
	switch node.Type {
	case ConditionTypeLiteral:
		return toBool(node.Value), nil

	case ConditionTypeField:
		value := resolveField(node.Field, ctx)
		return toBool(value), nil

	case ConditionTypeLogical:
		return evaluateLogical(node, ctx)

	case ConditionTypeComparison:
		return evaluateComparison(node, ctx)

	default:
		return false, fmt.Errorf("unknown node type: %v", node.Type)
	}
}

// evaluateLogical evaluates a logical operation
func evaluateLogical(node *ConditionNode, ctx *ConditionContext) (bool, error) {
	switch node.Operator {
	case "and":
		left, err := evaluateNode(node.Left, ctx)
		if err != nil {
			return false, err
		}
		if !left {
			return false, nil // Short circuit
		}
		return evaluateNode(node.Right, ctx)

	case "or":
		left, err := evaluateNode(node.Left, ctx)
		if err != nil {
			return false, err
		}
		if left {
			return true, nil // Short circuit
		}
		return evaluateNode(node.Right, ctx)

	case "not":
		result, err := evaluateNode(node.Left, ctx)
		if err != nil {
			return false, err
		}
		return !result, nil

	default:
		return false, fmt.Errorf("unknown logical operator: %s", node.Operator)
	}
}

// evaluateComparison evaluates a comparison operation
func evaluateComparison(node *ConditionNode, ctx *ConditionContext) (bool, error) {
	leftVal := getValue(node.Left, ctx)
	rightVal := getValue(node.Right, ctx)

	switch node.Operator {
	case "==":
		return compareEqual(leftVal, rightVal), nil

	case "!=":
		return !compareEqual(leftVal, rightVal), nil

	case ">":
		return compareGreater(leftVal, rightVal), nil

	case "<":
		return compareLess(leftVal, rightVal), nil

	case ">=":
		return compareGreater(leftVal, rightVal) || compareEqual(leftVal, rightVal), nil

	case "<=":
		return compareLess(leftVal, rightVal) || compareEqual(leftVal, rightVal), nil

	case "=~":
		return matchRegex(leftVal, rightVal), nil

	case "!~":
		return !matchRegex(leftVal, rightVal), nil

	case "in":
		return containsValue(rightVal, leftVal), nil

	case "not in":
		return !containsValue(rightVal, leftVal), nil

	case "contains":
		return containsValue(leftVal, rightVal), nil

	case "startswith":
		return startsWithValue(leftVal, rightVal), nil

	case "endswith":
		return endsWithValue(leftVal, rightVal), nil

	default:
		return false, fmt.Errorf("unknown comparison operator: %s", node.Operator)
	}
}

// getValue gets the value from a node
func getValue(node *ConditionNode, ctx *ConditionContext) interface{} {
	switch node.Type {
	case ConditionTypeLiteral:
		return node.Value
	case ConditionTypeField:
		return resolveField(node.Field, ctx)
	default:
		return nil
	}
}

// resolveField resolves a field reference like "facts.os" from the context
func resolveField(field string, ctx *ConditionContext) interface{} {
	parts := strings.SplitN(field, ".", 2)
	if len(parts) < 2 {
		return nil
	}

	source := parts[0]
	key := parts[1]

	var data map[string]interface{}
	switch source {
	case "facts":
		data = ctx.Facts
	case "grains":
		data = ctx.Grains
	case "pillar":
		data = ctx.Pillar
	case "vars", "variables":
		data = ctx.Variables
	default:
		return nil
	}

	return getNestedValue(data, key)
}

// getNestedValue gets a nested value from a map using dot notation
func getNestedValue(data map[string]interface{}, key string) interface{} {
	parts := strings.Split(key, ".")
	current := interface{}(data)

	for _, part := range parts {
		switch v := current.(type) {
		case map[string]interface{}:
			current = v[part]
		case map[interface{}]interface{}:
			current = v[part]
		default:
			return nil
		}
	}

	return current
}

// Helper functions for comparisons

func toBool(v interface{}) bool {
	if v == nil {
		return false
	}
	switch val := v.(type) {
	case bool:
		return val
	case string:
		return val != ""
	case int:
		return val != 0
	case int64:
		return val != 0
	case float64:
		return val != 0
	case []interface{}:
		return len(val) > 0
	case map[string]interface{}:
		return len(val) > 0
	default:
		return true
	}
}

func compareEqual(a, b interface{}) bool {
	// Handle nil cases
	if a == nil && b == nil {
		return true
	}
	if a == nil || b == nil {
		return false
	}

	// Convert to comparable types
	aStr, aIsStr := toString(a)
	bStr, bIsStr := toString(b)
	if aIsStr && bIsStr {
		return aStr == bStr
	}

	aNum, aIsNum := toFloat64(a)
	bNum, bIsNum := toFloat64(b)
	if aIsNum && bIsNum {
		return aNum == bNum
	}

	aBool, aIsBool := a.(bool)
	bBool, bIsBool := b.(bool)
	if aIsBool && bIsBool {
		return aBool == bBool
	}

	return fmt.Sprintf("%v", a) == fmt.Sprintf("%v", b)
}

func compareGreater(a, b interface{}) bool {
	aNum, aIsNum := toFloat64(a)
	bNum, bIsNum := toFloat64(b)
	if aIsNum && bIsNum {
		return aNum > bNum
	}

	aStr, aIsStr := toString(a)
	bStr, bIsStr := toString(b)
	if aIsStr && bIsStr {
		return aStr > bStr
	}

	return false
}

func compareLess(a, b interface{}) bool {
	aNum, aIsNum := toFloat64(a)
	bNum, bIsNum := toFloat64(b)
	if aIsNum && bIsNum {
		return aNum < bNum
	}

	aStr, aIsStr := toString(a)
	bStr, bIsStr := toString(b)
	if aIsStr && bIsStr {
		return aStr < bStr
	}

	return false
}

func matchRegex(value, pattern interface{}) bool {
	valueStr, _ := toString(value)
	patternStr, _ := toString(pattern)

	re, err := regexp.Compile(patternStr)
	if err != nil {
		return false
	}

	return re.MatchString(valueStr)
}

func containsValue(container, value interface{}) bool {
	// Check if container is a list
	if list, ok := container.([]interface{}); ok {
		for _, item := range list {
			if compareEqual(item, value) {
				return true
			}
		}
		return false
	}

	// Check if container is a string
	if containerStr, ok := toString(container); ok {
		if valueStr, ok := toString(value); ok {
			return strings.Contains(containerStr, valueStr)
		}
	}

	// Check if container is a map (check for key)
	if m, ok := container.(map[string]interface{}); ok {
		if key, ok := toString(value); ok {
			_, exists := m[key]
			return exists
		}
	}

	return false
}

func startsWithValue(a, b interface{}) bool {
	aStr, aOk := toString(a)
	bStr, bOk := toString(b)
	if aOk && bOk {
		return strings.HasPrefix(aStr, bStr)
	}
	return false
}

func endsWithValue(a, b interface{}) bool {
	aStr, aOk := toString(a)
	bStr, bOk := toString(b)
	if aOk && bOk {
		return strings.HasSuffix(aStr, bStr)
	}
	return false
}

func toString(v interface{}) (string, bool) {
	switch val := v.(type) {
	case string:
		return val, true
	case int:
		return strconv.Itoa(val), true
	case int64:
		return strconv.FormatInt(val, 10), true
	case float64:
		return strconv.FormatFloat(val, 'f', -1, 64), true
	case bool:
		return strconv.FormatBool(val), true
	default:
		return fmt.Sprintf("%v", v), true
	}
}

func toFloat64(v interface{}) (float64, bool) {
	switch val := v.(type) {
	case float64:
		return val, true
	case float32:
		return float64(val), true
	case int:
		return float64(val), true
	case int64:
		return float64(val), true
	case int32:
		return float64(val), true
	case string:
		if f, err := strconv.ParseFloat(val, 64); err == nil {
			return f, true
		}
	}
	return 0, false
}

// ConditionalStateDeclaration extends StateDeclaration with When clause
type ConditionalStateDeclaration struct {
	StateDeclaration

	// When is a list of conditions that must all be true for this state to execute
	When []string

	// WhenAny is a list of conditions where at least one must be true
	WhenAny []string

	// WhenNot is a list of conditions that must all be false
	WhenNot []string
}

// ShouldExecute checks if the state should execute based on its conditions
func (csd *ConditionalStateDeclaration) ShouldExecute(ctx *ConditionContext) (shouldExecute bool, reason string, err error) {
	evaluator := NewConditionEvaluator()

	// Check When conditions (all must be true)
	for _, expr := range csd.When {
		result, err := evaluator.EvaluateExpression(expr, ctx)
		if err != nil {
			return false, "", fmt.Errorf("error evaluating 'when' condition '%s': %w", expr, err)
		}
		if !result {
			return false, fmt.Sprintf("when condition failed: %s", expr), nil
		}
	}

	// Check WhenAny conditions (at least one must be true)
	if len(csd.WhenAny) > 0 {
		anyTrue := false
		for _, expr := range csd.WhenAny {
			result, err := evaluator.EvaluateExpression(expr, ctx)
			if err != nil {
				return false, "", fmt.Errorf("error evaluating 'when_any' condition '%s': %w", expr, err)
			}
			if result {
				anyTrue = true
				break
			}
		}
		if !anyTrue {
			return false, "no 'when_any' condition was true", nil
		}
	}

	// Check WhenNot conditions (all must be false)
	for _, expr := range csd.WhenNot {
		result, err := evaluator.EvaluateExpression(expr, ctx)
		if err != nil {
			return false, "", fmt.Errorf("error evaluating 'when_not' condition '%s': %w", expr, err)
		}
		if result {
			return false, fmt.Sprintf("when_not condition was true: %s", expr), nil
		}
	}

	return true, "", nil
}

// ConditionalBlock represents a block of states with shared conditions
type ConditionalBlock struct {
	// Conditions that apply to all states in this block
	When    []string
	WhenAny []string
	WhenNot []string

	// States within this block
	States []StateDeclaration
}

// FilterStates filters a list of states based on conditions
func FilterStates(states []ConditionalStateDeclaration, ctx *ConditionContext) ([]StateDeclaration, []SkippedState) {
	var executed []StateDeclaration
	var skipped []SkippedState

	for i := range states {
		state := &states[i]
		shouldExec, reason, err := state.ShouldExecute(ctx)
		if err != nil {
			skipped = append(skipped, SkippedState{
				StateID: state.ID,
				Module:  state.Module,
				Reason:  fmt.Sprintf("condition evaluation error: %v", err),
			})
			continue
		}

		if shouldExec {
			executed = append(executed, state.StateDeclaration)
		} else {
			skipped = append(skipped, SkippedState{
				StateID: state.ID,
				Module:  state.Module,
				Reason:  reason,
			})
		}
	}

	return executed, skipped
}

// SkippedState represents a state that was skipped due to conditions
type SkippedState struct {
	StateID string
	Module  string
	Reason  string
}
