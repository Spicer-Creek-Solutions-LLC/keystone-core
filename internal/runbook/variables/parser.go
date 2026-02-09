package variables

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/shawnbutts/keystone-core/internal/runbook"
)

// OutputParser extracts values from step outputs.
type OutputParser struct{}

// NewOutputParser creates a new output parser.
func NewOutputParser() *OutputParser {
	return &OutputParser{}
}

// Parse extracts a value from the given data according to the output definition.
func (p *OutputParser) Parse(data interface{}, output *runbook.OutputDef) (interface{}, error) {
	// Get source data
	sourceData, err := p.getSourceData(data, output.Source)
	if err != nil {
		return nil, err
	}

	// Parse according to parser type
	switch output.Parser {
	case runbook.OutputParserJSON:
		return p.parseJSON(sourceData, output.Path)
	case runbook.OutputParserRegex:
		return p.parseRegex(sourceData, output.Path)
	case runbook.OutputParserLine:
		return p.parseLine(sourceData, output.Path)
	case runbook.OutputParserJSONPath:
		// JSONPath uses same logic as JSON but with path navigation
		return p.parseJSON(sourceData, output.Path)
	default:
		// No parser - return raw data
		return sourceData, nil
	}
}

// getSourceData extracts the source data based on source type.
func (p *OutputParser) getSourceData(data interface{}, source runbook.OutputSource) (string, error) {
	// If data is already a map with the expected keys
	if m, ok := data.(map[string]interface{}); ok {
		switch source {
		case runbook.OutputSourceStdout:
			if v, ok := m["stdout"].(string); ok {
				return v, nil
			}
		case runbook.OutputSourceStderr:
			if v, ok := m["stderr"].(string); ok {
				return v, nil
			}
		case runbook.OutputSourceBody:
			if v, ok := m["body"].(string); ok {
				return v, nil
			}
		case runbook.OutputSourceHeader:
			if headers, ok := m["headers"].(map[string]interface{}); ok {
				// Serialize headers as JSON for further parsing
				b, _ := json.Marshal(headers)
				return string(b), nil
			}
		case runbook.OutputSourceJSON:
			if v, ok := m["json"]; ok {
				// Re-serialize for JSON parsing
				b, _ := json.Marshal(v)
				return string(b), nil
			}
			// Fall back to body
			if v, ok := m["body"].(string); ok {
				return v, nil
			}
		case runbook.OutputSourceExitCode:
			if v, ok := m["exit_code"].(int); ok {
				return fmt.Sprintf("%d", v), nil
			}
			// Also check status_code for API responses
			if v, ok := m["status_code"].(int); ok {
				return fmt.Sprintf("%d", v), nil
			}
		}
	}

	// Try to convert data directly to string
	switch v := data.(type) {
	case string:
		return v, nil
	case []byte:
		return string(v), nil
	default:
		b, err := json.Marshal(data)
		if err != nil {
			return "", fmt.Errorf("cannot convert data to string: %w", err)
		}
		return string(b), nil
	}
}

// parseJSON extracts a value from JSON using a JSONPath-like expression.
func (p *OutputParser) parseJSON(data, path string) (interface{}, error) {
	if path == "" {
		// No path - parse entire JSON
		var v interface{}
		if err := json.Unmarshal([]byte(data), &v); err != nil {
			return nil, fmt.Errorf("failed to parse JSON: %w", err)
		}
		return v, nil
	}

	var root interface{}
	if err := json.Unmarshal([]byte(data), &root); err != nil {
		return nil, fmt.Errorf("failed to parse JSON: %w", err)
	}

	return p.navigatePath(root, path)
}

// navigatePath navigates a JSONPath-like expression.
// Supports: .field, [index], .field.subfield, .field[0].subfield
func (p *OutputParser) navigatePath(data interface{}, path string) (interface{}, error) {
	if path == "" || path == "." {
		return data, nil
	}

	// Remove leading dot
	path = strings.TrimPrefix(path, ".")

	current := data
	parts := splitPath(path)

	for _, part := range parts {
		if current == nil {
			return nil, fmt.Errorf("cannot navigate path: nil value")
		}

		// Check if part is an array index
		if strings.HasPrefix(part, "[") && strings.HasSuffix(part, "]") {
			indexStr := part[1 : len(part)-1]
			index, err := strconv.Atoi(indexStr)
			if err != nil {
				return nil, fmt.Errorf("invalid array index: %s", indexStr)
			}

			arr, ok := current.([]interface{})
			if !ok {
				return nil, fmt.Errorf("expected array, got %T", current)
			}

			if index < 0 || index >= len(arr) {
				return nil, fmt.Errorf("array index %d out of bounds (len=%d)", index, len(arr))
			}

			current = arr[index]
		} else {
			// Object field access
			m, ok := current.(map[string]interface{})
			if !ok {
				return nil, fmt.Errorf("expected object, got %T", current)
			}

			val, ok := m[part]
			if !ok {
				return nil, fmt.Errorf("field %q not found", part)
			}

			current = val
		}
	}

	return current, nil
}

// splitPath splits a path into parts, handling array indices.
func splitPath(path string) []string {
	var parts []string
	var current strings.Builder

	for i := 0; i < len(path); i++ {
		c := path[i]

		switch c {
		case '.':
			if current.Len() > 0 {
				parts = append(parts, current.String())
				current.Reset()
			}
		case '[':
			if current.Len() > 0 {
				parts = append(parts, current.String())
				current.Reset()
			}
			// Find closing bracket
			end := strings.Index(path[i:], "]")
			if end == -1 {
				current.WriteByte(c)
				continue
			}
			parts = append(parts, path[i:i+end+1])
			i += end
		default:
			current.WriteByte(c)
		}
	}

	if current.Len() > 0 {
		parts = append(parts, current.String())
	}

	return parts
}

// parseRegex extracts a value using a regular expression.
// The path is the regex pattern. Named groups or first capture group is returned.
func (p *OutputParser) parseRegex(data, pattern string) (interface{}, error) {
	if pattern == "" {
		return data, nil
	}

	re, err := regexp.Compile(pattern)
	if err != nil {
		return nil, fmt.Errorf("invalid regex pattern: %w", err)
	}

	// Check for named groups
	names := re.SubexpNames()
	hasNamedGroups := false
	for _, name := range names {
		if name != "" {
			hasNamedGroups = true
			break
		}
	}

	matches := re.FindStringSubmatch(data)
	if matches == nil {
		return nil, fmt.Errorf("regex pattern did not match")
	}

	// If named groups, return map
	if hasNamedGroups {
		result := make(map[string]string)
		for i, name := range names {
			if name != "" && i < len(matches) {
				result[name] = matches[i]
			}
		}
		return result, nil
	}

	// Return first capture group or entire match
	if len(matches) > 1 {
		return matches[1], nil
	}
	return matches[0], nil
}

// parseLine extracts lines from data.
// Path can be:
// - empty: return all lines as array
// - number: return specific line (1-indexed)
// - "first": return first line
// - "last": return last line
// - "count": return line count
func (p *OutputParser) parseLine(data, path string) (interface{}, error) {
	lines := strings.Split(strings.TrimSuffix(data, "\n"), "\n")

	switch path {
	case "", "all":
		return lines, nil

	case "first":
		if len(lines) > 0 {
			return lines[0], nil
		}
		return "", nil

	case "last":
		if len(lines) > 0 {
			return lines[len(lines)-1], nil
		}
		return "", nil

	case "count":
		return len(lines), nil

	default:
		// Try to parse as line number
		lineNum, err := strconv.Atoi(path)
		if err != nil {
			return nil, fmt.Errorf("invalid line specifier: %s", path)
		}

		// Support negative indexing
		if lineNum < 0 {
			lineNum = len(lines) + lineNum + 1
		}

		// 1-indexed
		if lineNum < 1 || lineNum > len(lines) {
			return nil, fmt.Errorf("line %d out of range (1-%d)", lineNum, len(lines))
		}

		return lines[lineNum-1], nil
	}
}

// ParseOutputs parses all output definitions for a step result.
func (p *OutputParser) ParseOutputs(data interface{}, outputs []runbook.OutputDef) (map[string]interface{}, error) {
	result := make(map[string]interface{})

	for _, output := range outputs {
		val, err := p.Parse(data, &output)
		if err != nil {
			return nil, fmt.Errorf("failed to parse output %q: %w", output.Name, err)
		}
		result[output.Name] = val
	}

	return result, nil
}
