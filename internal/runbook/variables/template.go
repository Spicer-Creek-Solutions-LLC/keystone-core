package variables

import (
	"bytes"
	"encoding/json"
	"fmt"
	"reflect"
	"regexp"
	"strings"
	"text/template"
)

// TemplateEngine evaluates Go templates with runbook context.
type TemplateEngine struct {
	ctx       *Context
	funcMap   template.FuncMap
	delims    [2]string
	maxNested int
}

// NewTemplateEngine creates a new template engine.
func NewTemplateEngine(ctx *Context) *TemplateEngine {
	e := &TemplateEngine{
		ctx:       ctx,
		delims:    [2]string{"{{", "}}"},
		maxNested: 10,
	}

	e.funcMap = template.FuncMap{
		// String functions
		"upper":      strings.ToUpper,
		"lower":      strings.ToLower,
		"trim":       strings.TrimSpace,
		"trimPrefix": strings.TrimPrefix,
		"trimSuffix": strings.TrimSuffix,
		"replace":    strings.ReplaceAll,
		"contains":   strings.Contains,
		"hasPrefix":  strings.HasPrefix,
		"hasSuffix":  strings.HasSuffix,
		"split":      strings.Split,
		"join":       strings.Join,

		// Type conversion
		"toString": toString,
		"toInt":    toInt,
		"toFloat":  toFloat,
		"toBool":   toBool,
		"toJSON":   toJSON,
		"fromJSON": fromJSON,

		// Comparison
		"eq": reflect.DeepEqual,
		"ne": func(a, b interface{}) bool { return !reflect.DeepEqual(a, b) },
		"lt": lt,
		"le": le,
		"gt": gt,
		"ge": ge,

		// Default value
		"default":  defaultValue,
		"coalesce": coalesce,

		// Collections
		"first":  first,
		"last":   last,
		"index":  index,
		"len":    length,
		"keys":   keys,
		"values": values,

		// Regex
		"regexMatch":   regexMatch,
		"regexFind":    regexFind,
		"regexReplace": regexReplace,

		// Encoding
		"b64enc": b64enc,
		"b64dec": b64dec,
	}

	return e
}

// Execute evaluates the template and returns the result as a string.
func (e *TemplateEngine) Execute(templateStr string) (string, error) {
	// Quick check for no templates
	if !hasTemplateSyntax(templateStr) {
		return templateStr, nil
	}

	data := e.ctx.ToData()

	tmpl, err := template.New("runbook").
		Delims(e.delims[0], e.delims[1]).
		Funcs(e.funcMap).
		Parse(templateStr)
	if err != nil {
		return "", fmt.Errorf("template parse error: %w", err)
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		return "", fmt.Errorf("template execution error: %w", err)
	}

	return buf.String(), nil
}

// ExecuteValue evaluates the template and returns the typed result.
// If the template evaluates to a simple value (not containing other text),
// returns the typed value. Otherwise returns the string result.
func (e *TemplateEngine) ExecuteValue(templateStr string) (interface{}, error) {
	// Quick check for no templates
	if !hasTemplateSyntax(templateStr) {
		return templateStr, nil
	}

	// Check if template is just a single expression
	trimmed := strings.TrimSpace(templateStr)
	if strings.HasPrefix(trimmed, "{{") && strings.HasSuffix(trimmed, "}}") {
		// Count delimiters to see if it's a single expression
		inner := trimmed[2 : len(trimmed)-2]
		if !strings.Contains(inner, "{{") && !strings.Contains(inner, "}}") {
			// Single expression - try to get typed value
			return e.executeTypedValue(inner)
		}
	}

	// Multiple expressions or mixed content - return string
	return e.Execute(templateStr)
}

// executeTypedValue executes a single template expression and returns typed value.
func (e *TemplateEngine) executeTypedValue(expr string) (interface{}, error) {
	// Wrap in delimiters for parsing
	templateStr := fmt.Sprintf("{{ %s }}", strings.TrimSpace(expr))

	data := e.ctx.ToData()

	// Use a special template that captures the value
	tmpl, err := template.New("runbook").
		Delims(e.delims[0], e.delims[1]).
		Funcs(e.funcMap).
		Parse(templateStr)
	if err != nil {
		return nil, fmt.Errorf("template parse error: %w", err)
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		return nil, fmt.Errorf("template execution error: %w", err)
	}

	result := strings.TrimSpace(buf.String())

	// Try to parse as JSON value for typed result
	var jsonVal interface{}
	if err := json.Unmarshal([]byte(result), &jsonVal); err == nil {
		return jsonVal, nil
	}

	// Return as string
	return result, nil
}

// Template helper functions

func toString(v interface{}) string {
	switch val := v.(type) {
	case string:
		return val
	case []byte:
		return string(val)
	case nil:
		return ""
	default:
		return fmt.Sprintf("%v", v)
	}
}

func toInt(v interface{}) int {
	switch val := v.(type) {
	case int:
		return val
	case int64:
		return int(val)
	case float64:
		return int(val)
	case string:
		var i int
		fmt.Sscanf(val, "%d", &i)
		return i
	default:
		return 0
	}
}

func toFloat(v interface{}) float64 {
	switch val := v.(type) {
	case float64:
		return val
	case float32:
		return float64(val)
	case int:
		return float64(val)
	case int64:
		return float64(val)
	case string:
		var f float64
		fmt.Sscanf(val, "%f", &f)
		return f
	default:
		return 0
	}
}

func toBool(v interface{}) bool {
	switch val := v.(type) {
	case bool:
		return val
	case string:
		return val == "true" || val == "1" || val == "yes"
	case int:
		return val != 0
	case float64:
		return val != 0
	default:
		return false
	}
}

func toJSON(v interface{}) string {
	b, err := json.Marshal(v)
	if err != nil {
		return ""
	}
	return string(b)
}

func fromJSON(s string) interface{} {
	var v interface{}
	if err := json.Unmarshal([]byte(s), &v); err != nil {
		return nil
	}
	return v
}

func lt(a, b interface{}) bool {
	return toFloat(a) < toFloat(b)
}

func le(a, b interface{}) bool {
	return toFloat(a) <= toFloat(b)
}

func gt(a, b interface{}) bool {
	return toFloat(a) > toFloat(b)
}

func ge(a, b interface{}) bool {
	return toFloat(a) >= toFloat(b)
}

func defaultValue(defaultVal, val interface{}) interface{} {
	if val == nil || val == "" {
		return defaultVal
	}
	if s, ok := val.(string); ok && s == "" {
		return defaultVal
	}
	return val
}

func coalesce(vals ...interface{}) interface{} {
	for _, v := range vals {
		if v != nil && v != "" {
			if s, ok := v.(string); ok && s == "" {
				continue
			}
			return v
		}
	}
	return nil
}

func first(v interface{}) interface{} {
	switch val := v.(type) {
	case []interface{}:
		if len(val) > 0 {
			return val[0]
		}
	case []string:
		if len(val) > 0 {
			return val[0]
		}
	}
	return nil
}

func last(v interface{}) interface{} {
	switch val := v.(type) {
	case []interface{}:
		if len(val) > 0 {
			return val[len(val)-1]
		}
	case []string:
		if len(val) > 0 {
			return val[len(val)-1]
		}
	}
	return nil
}

func index(v interface{}, i int) interface{} {
	switch val := v.(type) {
	case []interface{}:
		if i >= 0 && i < len(val) {
			return val[i]
		}
	case []string:
		if i >= 0 && i < len(val) {
			return val[i]
		}
	case map[string]interface{}:
		// Support negative indices as string keys
		return val[fmt.Sprintf("%d", i)]
	}
	return nil
}

func length(v interface{}) int {
	switch val := v.(type) {
	case string:
		return len(val)
	case []interface{}:
		return len(val)
	case []string:
		return len(val)
	case map[string]interface{}:
		return len(val)
	default:
		return 0
	}
}

func keys(v interface{}) []string {
	if m, ok := v.(map[string]interface{}); ok {
		result := make([]string, 0, len(m))
		for k := range m {
			result = append(result, k)
		}
		return result
	}
	return nil
}

func values(v interface{}) []interface{} {
	if m, ok := v.(map[string]interface{}); ok {
		result := make([]interface{}, 0, len(m))
		for _, val := range m {
			result = append(result, val)
		}
		return result
	}
	return nil
}

func regexMatch(pattern, s string) bool {
	matched, _ := regexp.MatchString(pattern, s)
	return matched
}

func regexFind(pattern, s string) string {
	re, err := regexp.Compile(pattern)
	if err != nil {
		return ""
	}
	return re.FindString(s)
}

func regexReplace(pattern, replacement, s string) string {
	re, err := regexp.Compile(pattern)
	if err != nil {
		return s
	}
	return re.ReplaceAllString(s, replacement)
}

func b64enc(s string) string {
	return b64encode([]byte(s))
}

func b64dec(s string) string {
	b, err := b64decode(s)
	if err != nil {
		return ""
	}
	return string(b)
}

// b64encode encodes bytes to base64.
func b64encode(b []byte) string {
	const base64Chars = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789+/"
	result := make([]byte, 0, ((len(b)+2)/3)*4)

	for i := 0; i < len(b); i += 3 {
		var n uint32
		var padding int

		n = uint32(b[i]) << 16
		if i+1 < len(b) {
			n |= uint32(b[i+1]) << 8
		} else {
			padding++
		}
		if i+2 < len(b) {
			n |= uint32(b[i+2])
		} else {
			padding++
		}

		result = append(result, base64Chars[(n>>18)&0x3f], base64Chars[(n>>12)&0x3f])
		if padding < 2 {
			result = append(result, base64Chars[(n>>6)&0x3f])
		} else {
			result = append(result, '=')
		}
		if padding < 1 {
			result = append(result, base64Chars[n&0x3f])
		} else {
			result = append(result, '=')
		}
	}

	return string(result)
}

// b64decode decodes base64 to bytes.
func b64decode(s string) ([]byte, error) {
	const base64Chars = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789+/"
	charMap := make(map[byte]uint32)
	for i, c := range []byte(base64Chars) {
		charMap[c] = uint32(i) //nolint:gosec // G115: i is 0-63 for base64 chars
	}

	// Remove padding
	s = strings.TrimRight(s, "=")

	result := make([]byte, 0, len(s)*3/4)

	for i := 0; i < len(s); i += 4 {
		var n uint32
		count := 0

		for j := 0; j < 4 && i+j < len(s); j++ {
			c := s[i+j]
			if v, ok := charMap[c]; ok {
				n = n<<6 | v
				count++
			}
		}

		// Pad remaining with zeros
		for count < 4 {
			n <<= 6
			count++
		}

		if count >= 2 {
			result = append(result, byte(n>>16))
		}
		if count >= 3 {
			result = append(result, byte(n>>8))
		}
		if count >= 4 {
			result = append(result, byte(n))
		}
	}

	// Trim trailing zeros added by padding
	padding := 0
	if len(s)%4 == 2 {
		padding = 2
	} else if len(s)%4 == 3 {
		padding = 1
	}
	if padding > 0 && len(result) >= padding {
		result = result[:len(result)-padding]
	}

	return result, nil
}
