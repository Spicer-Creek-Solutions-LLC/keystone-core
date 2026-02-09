package variables

import (
	"bytes"
	"fmt"
	"regexp"
	"strings"
	"text/template"
)

// ExpressionEvaluator evaluates boolean expressions for step conditions.
// It supports:
// - Comparison operators: ==, !=, <, >, <=, >=
// - Logical operators: &&, ||, !
// - String functions: contains, startsWith, endsWith, matches
// - Value functions: empty, defined
type ExpressionEvaluator struct {
	ctx     *Context
	funcMap template.FuncMap
}

// NewExpressionEvaluator creates a new expression evaluator.
func NewExpressionEvaluator(ctx *Context) *ExpressionEvaluator {
	e := &ExpressionEvaluator{
		ctx: ctx,
	}

	// Build function map with expression-specific functions
	e.funcMap = template.FuncMap{
		// Comparison operators
		"eq": eq,
		"ne": ne,
		"lt": lt,
		"le": le,
		"gt": gt,
		"ge": ge,

		// Logical operators
		"and": and,
		"or":  or,
		"not": not,

		// String functions
		"contains":   strings.Contains,
		"startsWith": strings.HasPrefix,
		"endsWith":   strings.HasSuffix,
		"matches":    matches,
		"hasPrefix":  strings.HasPrefix,
		"hasSuffix":  strings.HasSuffix,

		// Type checks and conversions
		"empty":   isEmpty,
		"defined": isDefined,
		"isNil":   isNil,
		"isBool":  isBool,
		"isInt":   isInt,
		"isFloat": isFloat,
		"isString": func(v interface{}) bool {
			_, ok := v.(string)
			return ok
		},
		"isList": isList,
		"isMap":  isMap,

		// Value functions
		"default":  defaultValue,
		"coalesce": coalesce,
		"ternary":  ternary,

		// String manipulation
		"upper":      strings.ToUpper,
		"lower":      strings.ToLower,
		"trim":       strings.TrimSpace,
		"trimPrefix": strings.TrimPrefix,
		"trimSuffix": strings.TrimSuffix,
		"replace":    strings.ReplaceAll,
		"split":      strings.Split,
		"join":       strings.Join,

		// Type conversion
		"toString": toString,
		"toInt":    toInt,
		"toFloat":  toFloat,
		"toBool":   toBool,

		// Collections
		"len":    length,
		"first":  first,
		"last":   last,
		"index":  index,
		"keys":   keys,
		"values": values,
		"in":     inCollection,
	}

	return e
}

// Evaluate evaluates an expression and returns the result as a boolean.
// Expressions can be:
// - Simple Go template expressions: {{ .inputs.enabled }}
// - Comparison expressions: {{ eq .inputs.env "production" }}
// - Logical expressions: {{ and (eq .inputs.env "prod") .inputs.deploy }}
func (e *ExpressionEvaluator) Evaluate(expr string) (bool, error) {
	result, err := e.EvaluateValue(expr)
	if err != nil {
		return false, err
	}

	return toBoolValue(result), nil
}

// EvaluateValue evaluates an expression and returns the raw result.
func (e *ExpressionEvaluator) EvaluateValue(expr string) (interface{}, error) {
	// Wrap expression in template delimiters if needed
	templateExpr := expr
	if !strings.Contains(expr, "{{") {
		templateExpr = fmt.Sprintf("{{ %s }}", expr)
	}

	data := e.ctx.ToData()

	tmpl, err := template.New("expr").
		Funcs(e.funcMap).
		Parse(templateExpr)
	if err != nil {
		return nil, fmt.Errorf("expression parse error: %w", err)
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		return nil, fmt.Errorf("expression evaluation error: %w", err)
	}

	result := strings.TrimSpace(buf.String())

	// Try to parse as typed value
	return parseTypedValue(result), nil
}

// EvaluateString evaluates an expression and returns the result as a string.
func (e *ExpressionEvaluator) EvaluateString(expr string) (string, error) {
	result, err := e.EvaluateValue(expr)
	if err != nil {
		return "", err
	}

	return toString(result), nil
}

// parseTypedValue attempts to parse a string into its typed value.
func parseTypedValue(s string) interface{} {
	// Boolean
	if s == "true" {
		return true
	}
	if s == "false" {
		return false
	}

	// Try as number
	if num := toInt(s); num != 0 || s == "0" {
		return num
	}

	// Return as string
	return s
}

// toBoolValue converts any value to a boolean.
func toBoolValue(v interface{}) bool {
	switch val := v.(type) {
	case bool:
		return val
	case string:
		// "true", non-empty strings (except "false", "0", "") are true
		if val == "" || val == "false" || val == "0" || val == "<no value>" {
			return false
		}
		return true
	case int:
		return val != 0
	case int64:
		return val != 0
	case float64:
		return val != 0
	case nil:
		return false
	default:
		return v != nil
	}
}

// Expression helper functions

// eq returns true if a equals b (deep equality).
func eq(a, b interface{}) bool {
	return fmt.Sprintf("%v", a) == fmt.Sprintf("%v", b)
}

// ne returns true if a does not equal b.
func ne(a, b interface{}) bool {
	return !eq(a, b)
}

// and returns true if all arguments are truthy.
func and(args ...interface{}) bool {
	for _, arg := range args {
		if !toBoolValue(arg) {
			return false
		}
	}
	return true
}

// or returns true if any argument is truthy.
func or(args ...interface{}) bool {
	for _, arg := range args {
		if toBoolValue(arg) {
			return true
		}
	}
	return false
}

// not returns the boolean negation of the value.
func not(v interface{}) bool {
	return !toBoolValue(v)
}

// matches returns true if the string matches the regex pattern.
func matches(pattern, s string) bool {
	matched, err := regexp.MatchString(pattern, s)
	if err != nil {
		return false
	}
	return matched
}

// isEmpty returns true if the value is empty (nil, empty string, empty slice/map, zero).
func isEmpty(v interface{}) bool {
	if v == nil {
		return true
	}

	switch val := v.(type) {
	case string:
		return val == ""
	case []interface{}:
		return len(val) == 0
	case []string:
		return len(val) == 0
	case map[string]interface{}:
		return len(val) == 0
	case int:
		return val == 0
	case int64:
		return val == 0
	case float64:
		return val == 0
	case bool:
		return !val
	}

	return false
}

// isDefined returns true if the value is not nil.
func isDefined(v interface{}) bool {
	return v != nil
}

// isNil returns true if the value is nil.
func isNil(v interface{}) bool {
	return v == nil
}

// isBool returns true if the value is a boolean.
func isBool(v interface{}) bool {
	_, ok := v.(bool)
	return ok
}

// isInt returns true if the value is an integer.
func isInt(v interface{}) bool {
	switch v.(type) {
	case int, int8, int16, int32, int64:
		return true
	}
	return false
}

// isFloat returns true if the value is a float.
func isFloat(v interface{}) bool {
	switch v.(type) {
	case float32, float64:
		return true
	}
	return false
}

// isList returns true if the value is a slice/array.
func isList(v interface{}) bool {
	switch v.(type) {
	case []interface{}, []string, []int, []float64:
		return true
	}
	return false
}

// isMap returns true if the value is a map.
func isMap(v interface{}) bool {
	_, ok := v.(map[string]interface{})
	return ok
}

// ternary returns trueVal if condition is true, otherwise falseVal.
func ternary(trueVal, falseVal, condition interface{}) interface{} {
	if toBoolValue(condition) {
		return trueVal
	}
	return falseVal
}

// inCollection returns true if item is in the collection.
func inCollection(item, collection interface{}) bool {
	itemStr := fmt.Sprintf("%v", item)

	switch col := collection.(type) {
	case []interface{}:
		for _, v := range col {
			if fmt.Sprintf("%v", v) == itemStr {
				return true
			}
		}
	case []string:
		for _, v := range col {
			if v == itemStr {
				return true
			}
		}
	case map[string]interface{}:
		_, ok := col[itemStr]
		return ok
	case string:
		return strings.Contains(col, itemStr)
	}

	return false
}

// CompareResult represents the result of a comparison.
type CompareResult int

// CompareLess and related constants.
const (
	CompareLess    CompareResult = -1
	CompareEqual   CompareResult = 0
	CompareGreater CompareResult = 1
)

// Compare compares two values and returns the comparison result.
func Compare(a, b interface{}) CompareResult {
	// Convert to comparable types
	aFloat := toFloat(a)
	bFloat := toFloat(b)

	if aFloat < bFloat {
		return CompareLess
	}
	if aFloat > bFloat {
		return CompareGreater
	}
	return CompareEqual
}

// ConditionResult represents the result of evaluating a condition.
type ConditionResult struct {
	Value   bool
	Error   error
	Message string
}

// EvaluateCondition evaluates a condition expression and returns a detailed result.
func (e *ExpressionEvaluator) EvaluateCondition(expr string) *ConditionResult {
	result := &ConditionResult{}

	value, err := e.Evaluate(expr)
	if err != nil {
		result.Error = err
		result.Message = fmt.Sprintf("condition evaluation failed: %v", err)
		return result
	}

	result.Value = value
	if value {
		result.Message = "condition evaluated to true"
	} else {
		result.Message = "condition evaluated to false"
	}

	return result
}

// MustEvaluate evaluates an expression and returns the boolean result.
// If evaluation fails, it returns the default value.
func (e *ExpressionEvaluator) MustEvaluate(expr string, defaultValue bool) bool {
	result, err := e.Evaluate(expr)
	if err != nil {
		return defaultValue
	}
	return result
}

// ValidateExpression checks if an expression is syntactically valid.
func ValidateExpression(expr string) error {
	// Wrap expression in template delimiters if needed
	templateExpr := expr
	if !strings.Contains(expr, "{{") {
		templateExpr = fmt.Sprintf("{{ %s }}", expr)
	}

	_, err := template.New("validate").
		Funcs(template.FuncMap{
			"eq":         func(a, b interface{}) bool { return true },
			"ne":         func(a, b interface{}) bool { return true },
			"lt":         func(a, b interface{}) bool { return true },
			"le":         func(a, b interface{}) bool { return true },
			"gt":         func(a, b interface{}) bool { return true },
			"ge":         func(a, b interface{}) bool { return true },
			"and":        func(args ...interface{}) bool { return true },
			"or":         func(args ...interface{}) bool { return true },
			"not":        func(v interface{}) bool { return true },
			"contains":   func(a, b string) bool { return true },
			"startsWith": func(a, b string) bool { return true },
			"endsWith":   func(a, b string) bool { return true },
			"matches":    func(a, b string) bool { return true },
			"hasPrefix":  func(a, b string) bool { return true },
			"hasSuffix":  func(a, b string) bool { return true },
			"empty":      func(v interface{}) bool { return true },
			"defined":    func(v interface{}) bool { return true },
			"isNil":      func(v interface{}) bool { return true },
			"isBool":     func(v interface{}) bool { return true },
			"isInt":      func(v interface{}) bool { return true },
			"isFloat":    func(v interface{}) bool { return true },
			"isString":   func(v interface{}) bool { return true },
			"isList":     func(v interface{}) bool { return true },
			"isMap":      func(v interface{}) bool { return true },
			"default":    func(a, b interface{}) interface{} { return a },
			"coalesce":   func(args ...interface{}) interface{} { return nil },
			"ternary":    func(a, b, c interface{}) interface{} { return a },
			"upper":      func(s string) string { return s },
			"lower":      func(s string) string { return s },
			"trim":       func(s string) string { return s },
			"trimPrefix": func(s, p string) string { return s },
			"trimSuffix": func(s, p string) string { return s },
			"replace":    func(s, o, n string) string { return s },
			"split":      func(s, sep string) []string { return nil },
			"join":       func(a []string, sep string) string { return "" },
			"toString":   func(v interface{}) string { return "" },
			"toInt":      func(v interface{}) int { return 0 },
			"toFloat":    func(v interface{}) float64 { return 0 },
			"toBool":     func(v interface{}) bool { return true },
			"len":        func(v interface{}) int { return 0 },
			"first":      func(v interface{}) interface{} { return nil },
			"last":       func(v interface{}) interface{} { return nil },
			"index":      func(v interface{}, i int) interface{} { return nil },
			"keys":       func(v interface{}) []string { return nil },
			"values":     func(v interface{}) []interface{} { return nil },
			"in":         func(a, b interface{}) bool { return true },
		}).
		Parse(templateExpr)

	if err != nil {
		return fmt.Errorf("invalid expression syntax: %w", err)
	}

	return nil
}
