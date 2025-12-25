package targeting

import (
	"fmt"
	"strings"

	"github.com/expr-lang/expr"
	"github.com/expr-lang/expr/vm"
	"github.com/gobwas/glob"
)

// TargetExpression represents a compiled target expression that can be evaluated
// against agent metadata to determine if an agent matches.
type TargetExpression struct {
	program *vm.Program
	raw     string
}

// Parse parses a target expression string into a compiled TargetExpression.
//
// Supported syntax:
//   - Simple matching: "os:linux", "role:web"
//   - Glob patterns: "name:prod-*", "hostname:*.example.com"
//   - Logical operators: "os:linux and role:web", "name:prod-* or name:staging-*"
//   - Negation: "not os:windows", "os:linux and not role:db"
//   - Grouping: "(os:linux and role:web) or name:special"
//
// Examples:
//   - "os:linux" - matches agents with os="linux"
//   - "name:web-*" - matches agents with names starting with "web-"
//   - "os:linux and role:web" - matches Linux agents with web role
//   - "(os:linux or os:darwin) and not role:db" - complex expression
func Parse(expression string) (*TargetExpression, error) {
	if expression == "" {
		return nil, fmt.Errorf("empty target expression")
	}

	// Convert target expression syntax to expr-compatible syntax
	exprCode := convertToExprSyntax(expression)

	// Compile the expression - match function will be provided at runtime
	program, err := expr.Compile(exprCode)

	if err != nil {
		return nil, fmt.Errorf("failed to compile expression: %w", err)
	}

	return &TargetExpression{
		program: program,
		raw:     expression,
	}, nil
}

// Matches evaluates the target expression against the provided agent metadata
// and returns true if the agent matches the target criteria.
func (te *TargetExpression) Matches(metadata map[string]string) (bool, error) {
	// Create environment with metadata and custom match function
	env := map[string]interface{}{
		"match": func(key, pattern string) bool {
			value, exists := metadata[key]
			if !exists {
				return false
			}
			return matchValue(value, pattern)
		},
	}

	// Add all metadata fields directly to the environment for convenience
	for k, v := range metadata {
		env[k] = v
	}

	result, err := expr.Run(te.program, env)
	if err != nil {
		return false, fmt.Errorf("failed to evaluate expression: %w", err)
	}

	matched, ok := result.(bool)
	if !ok {
		return false, fmt.Errorf("expression did not return boolean result")
	}

	return matched, nil
}

// String returns the original target expression string.
func (te *TargetExpression) String() string {
	return te.raw
}

// convertToExprSyntax converts target expression syntax to expr-compatible syntax.
//
// Converts:
//   - "key:value" -> "match('key', 'value')"
//   - "and" -> "&&"
//   - "or" -> "||"
//   - "not" -> "!"
func convertToExprSyntax(target string) string {
	var result strings.Builder
	tokens := tokenize(target)

	for i := 0; i < len(tokens); i++ {
		token := tokens[i]

		switch {
		case token == "and":
			result.WriteString(" && ")
		case token == "or":
			result.WriteString(" || ")
		case token == "not":
			result.WriteString("!")
		case token == "(":
			result.WriteString("(")
		case token == ")":
			result.WriteString(")")
		case strings.Contains(token, ":"):
			// Convert "key:value" to "match('key', 'value')"
			parts := strings.SplitN(token, ":", 2)
			if len(parts) == 2 {
				key := strings.TrimSpace(parts[0])
				value := strings.TrimSpace(parts[1])
				// Escape single quotes in key and value
				key = strings.ReplaceAll(key, "'", "\\'")
				value = strings.ReplaceAll(value, "'", "\\'")
				result.WriteString(fmt.Sprintf("match('%s', '%s')", key, value))
			} else {
				result.WriteString(token)
			}
		default:
			result.WriteString(token)
		}
	}

	return result.String()
}

// tokenize splits the target expression into tokens while preserving parentheses
// and handling quoted strings.
func tokenize(expression string) []string {
	var tokens []string
	var current strings.Builder

	for i := 0; i < len(expression); i++ {
		ch := expression[i]

		switch ch {
		case ' ', '\t', '\n':
			if current.Len() > 0 {
				tokens = append(tokens, current.String())
				current.Reset()
			}
		case '(':
			if current.Len() > 0 {
				tokens = append(tokens, current.String())
				current.Reset()
			}
			tokens = append(tokens, "(")
		case ')':
			if current.Len() > 0 {
				tokens = append(tokens, current.String())
				current.Reset()
			}
			tokens = append(tokens, ")")
		default:
			current.WriteByte(ch)
		}
	}

	if current.Len() > 0 {
		tokens = append(tokens, current.String())
	}

	return tokens
}

// matchValue checks if a value matches a pattern, supporting glob patterns.
func matchValue(value, pattern string) bool {
	// If pattern contains glob characters, use glob matching
	if strings.ContainsAny(pattern, "*?[]{}") {
		g, err := glob.Compile(pattern)
		if err != nil {
			return false
		}
		return g.Match(value)
	}

	// Otherwise, exact match
	return value == pattern
}
