// Package stdlib provides standard library modules for plugins.
package stdlib

import (
	"bytes"
	"encoding/json"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"strings"
)

// Errors returned by encoding modules.
var (
	ErrInvalidInput  = errors.New("invalid input")
	ErrParseError    = errors.New("parse error")
	ErrEncodeError   = errors.New("encode error")
	ErrUnsupportedOp = errors.New("unsupported operation")
)

// YAMLModule provides YAML parsing and encoding capabilities.
type YAMLModule struct {
	// StrictMode enables strict YAML parsing.
	StrictMode bool
	// MaxDepth limits nesting depth.
	MaxDepth int
}

// NewYAMLModule creates a new YAML module.
func NewYAMLModule() *YAMLModule {
	return &YAMLModule{
		StrictMode: false,
		MaxDepth:   100,
	}
}

// Parse parses YAML content into a map or slice.
func (m *YAMLModule) Parse(data []byte) (interface{}, error) {
	if len(data) == 0 {
		return nil, ErrInvalidInput
	}

	result, err := parseYAML(data, m.MaxDepth)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrParseError, err)
	}

	return result, nil
}

// ParseFile parses YAML from a reader.
func (m *YAMLModule) ParseFile(r io.Reader) (interface{}, error) {
	data, err := io.ReadAll(r)
	if err != nil {
		return nil, err
	}
	return m.Parse(data)
}

// Encode encodes a value to YAML.
func (m *YAMLModule) Encode(v interface{}) ([]byte, error) {
	return encodeYAML(v)
}

// EncodeIndent encodes with custom indentation.
func (m *YAMLModule) EncodeIndent(v interface{}, indent int) ([]byte, error) {
	return encodeYAMLIndent(v, indent)
}

// ParseMulti parses multiple YAML documents.
func (m *YAMLModule) ParseMulti(data []byte) ([]interface{}, error) {
	docs := bytes.Split(data, []byte("\n---\n"))
	results := make([]interface{}, 0, len(docs))

	for _, doc := range docs {
		doc = bytes.TrimSpace(doc)
		if len(doc) == 0 {
			continue
		}
		result, err := m.Parse(doc)
		if err != nil {
			return nil, err
		}
		results = append(results, result)
	}

	return results, nil
}

// Merge merges multiple YAML documents.
func (m *YAMLModule) Merge(docs ...interface{}) (interface{}, error) {
	if len(docs) == 0 {
		return nil, nil
	}

	result := make(map[string]interface{})

	for _, doc := range docs {
		switch v := doc.(type) {
		case map[string]interface{}:
			mergeMaps(result, v)
		default:
			return nil, fmt.Errorf("%w: can only merge maps", ErrUnsupportedOp)
		}
	}

	return result, nil
}

// Get gets a value by path (e.g., "foo.bar.baz").
func (m *YAMLModule) Get(doc interface{}, path string) (interface{}, error) {
	return getPath(doc, path)
}

// Set sets a value by path.
func (m *YAMLModule) Set(doc interface{}, path string, value interface{}) error {
	return setPath(doc, path, value)
}

// Simple YAML parser (handles basic YAML without external dependencies)
func parseYAML(data []byte, maxDepth int) (interface{}, error) {
	lines := strings.Split(string(data), "\n")
	return parseYAMLLines(lines, 0, maxDepth, 0)
}

func parseYAMLLines(lines []string, startIndent, maxDepth, depth int) (interface{}, error) {
	if depth > maxDepth {
		return nil, errors.New("max depth exceeded")
	}

	result := make(map[string]interface{})
	var currentKey string
	var listItems []interface{}
	isList := false

	i := 0
	for i < len(lines) {
		line := lines[i]
		if len(strings.TrimSpace(line)) == 0 || strings.HasPrefix(strings.TrimSpace(line), "#") {
			i++
			continue
		}

		indent := countIndent(line)
		if indent < startIndent && i > 0 {
			break
		}

		trimmed := strings.TrimSpace(line)

		// Check for list item
		if strings.HasPrefix(trimmed, "- ") {
			isList = true
			itemValue := strings.TrimPrefix(trimmed, "- ")
			itemValue = strings.TrimSpace(itemValue)

			if strings.Contains(itemValue, ": ") {
				// Inline map in list
				parts := strings.SplitN(itemValue, ": ", 2)
				item := map[string]interface{}{
					parts[0]: parseValue(parts[1]),
				}
				listItems = append(listItems, item)
			} else if itemValue == "" {
				// Nested structure
				subLines := collectBlock(lines[i+1:], indent+2)
				subValue, err := parseYAMLLines(subLines, indent+2, maxDepth, depth+1)
				if err != nil {
					return nil, err
				}
				listItems = append(listItems, subValue)
				i += len(subLines)
			} else {
				listItems = append(listItems, parseValue(itemValue))
			}
			i++
			continue
		}

		// Check for key: value
		if strings.Contains(trimmed, ": ") || strings.HasSuffix(trimmed, ":") {
			colonIdx := strings.Index(trimmed, ":")
			key := trimmed[:colonIdx]
			value := ""
			if colonIdx < len(trimmed)-1 {
				value = strings.TrimSpace(trimmed[colonIdx+1:])
			}

			if value == "" || value == "|" || value == ">" {
				// Nested structure or multiline
				subLines := collectBlock(lines[i+1:], indent+2)
				if len(subLines) > 0 {
					if value == "|" || value == ">" {
						// Literal block
						result[key] = strings.Join(subLines, "\n")
					} else {
						subValue, err := parseYAMLLines(subLines, indent+2, maxDepth, depth+1)
						if err != nil {
							return nil, err
						}
						result[key] = subValue
					}
					i += len(subLines)
				} else {
					result[key] = nil
				}
			} else {
				result[key] = parseValue(value)
			}
			currentKey = key
		}

		i++
	}

	if isList {
		return listItems, nil
	}

	_ = currentKey // unused but kept for clarity
	return result, nil
}

func collectBlock(lines []string, minIndent int) []string {
	var result []string
	for _, line := range lines {
		if len(strings.TrimSpace(line)) == 0 {
			result = append(result, line)
			continue
		}
		indent := countIndent(line)
		if indent < minIndent {
			break
		}
		result = append(result, line)
	}
	return result
}

func countIndent(line string) int {
	count := 0
	for _, c := range line {
		if c == ' ' {
			count++
		} else if c == '\t' {
			count += 2
		} else {
			break
		}
	}
	return count
}

func parseValue(s string) interface{} {
	s = strings.TrimSpace(s)

	// Handle quotes
	if (strings.HasPrefix(s, "\"") && strings.HasSuffix(s, "\"")) ||
		(strings.HasPrefix(s, "'") && strings.HasSuffix(s, "'")) {
		return s[1 : len(s)-1]
	}

	// Handle booleans
	lower := strings.ToLower(s)
	if lower == "true" || lower == "yes" || lower == "on" {
		return true
	}
	if lower == "false" || lower == "no" || lower == "off" {
		return false
	}

	// Handle null
	if lower == "null" || lower == "~" || s == "" {
		return nil
	}

	// Handle numbers
	var i int64
	if _, err := fmt.Sscanf(s, "%d", &i); err == nil {
		return i
	}

	var f float64
	if _, err := fmt.Sscanf(s, "%f", &f); err == nil {
		return f
	}

	return s
}

func encodeYAML(v interface{}) ([]byte, error) {
	return encodeYAMLIndent(v, 2)
}

func encodeYAMLIndent(v interface{}, indent int) ([]byte, error) {
	var buf bytes.Buffer
	err := writeYAML(&buf, v, 0, indent)
	return buf.Bytes(), err
}

func writeYAML(w io.Writer, v interface{}, level, indent int) error {
	prefix := strings.Repeat(" ", level*indent)

	switch val := v.(type) {
	case nil:
		fmt.Fprintf(w, "null")
	case bool:
		fmt.Fprintf(w, "%t", val)
	case int, int64, int32, int16, int8:
		fmt.Fprintf(w, "%d", val)
	case uint, uint64, uint32, uint16, uint8:
		fmt.Fprintf(w, "%d", val)
	case float32, float64:
		fmt.Fprintf(w, "%v", val)
	case string:
		if needsQuotes(val) {
			fmt.Fprintf(w, "%q", val)
		} else {
			fmt.Fprintf(w, "%s", val)
		}
	case []interface{}:
		for i, item := range val {
			if i > 0 {
				fmt.Fprintf(w, "\n")
			}
			fmt.Fprintf(w, "%s- ", prefix)
			if m, ok := item.(map[string]interface{}); ok && len(m) > 0 {
				first := true
				for k, v := range m {
					if !first {
						fmt.Fprintf(w, "\n%s  ", prefix)
					}
					first = false
					fmt.Fprintf(w, "%s: ", k)
					writeYAML(w, v, level+2, indent)
				}
			} else {
				writeYAML(w, item, level+1, indent)
			}
		}
	case map[string]interface{}:
		first := true
		for k, v := range val {
			if !first {
				fmt.Fprintf(w, "\n")
			}
			first = false
			fmt.Fprintf(w, "%s%s: ", prefix, k)
			if m, ok := v.(map[string]interface{}); ok && len(m) > 0 {
				fmt.Fprintf(w, "\n")
				writeYAML(w, m, level+1, indent)
			} else if s, ok := v.([]interface{}); ok && len(s) > 0 {
				fmt.Fprintf(w, "\n")
				writeYAML(w, s, level+1, indent)
			} else {
				writeYAML(w, v, level, indent)
			}
		}
	default:
		// Try JSON marshaling for complex types
		data, err := json.Marshal(val)
		if err != nil {
			return err
		}
		fmt.Fprintf(w, "%s", string(data))
	}

	return nil
}

func needsQuotes(s string) bool {
	if s == "" {
		return true
	}
	if strings.ContainsAny(s, ":#{}[]|>&*!?,") {
		return true
	}
	lower := strings.ToLower(s)
	if lower == "true" || lower == "false" || lower == "null" ||
		lower == "yes" || lower == "no" {
		return true
	}
	return false
}

// XMLModule provides XML parsing and encoding capabilities.
type XMLModule struct {
	// StrictMode enables strict XML parsing.
	StrictMode bool
	// PreserveWhitespace preserves whitespace in text content.
	PreserveWhitespace bool
}

// NewXMLModule creates a new XML module.
func NewXMLModule() *XMLModule {
	return &XMLModule{
		StrictMode:         true,
		PreserveWhitespace: false,
	}
}

// XMLNode represents an XML node.
type XMLNode struct {
	XMLName    xml.Name
	Attrs      []xml.Attr `xml:",any,attr"`
	Content    string     `xml:",chardata"`
	Children   []*XMLNode `xml:",any"`
	parent     *XMLNode
}

// Name returns the element name.
func (n *XMLNode) Name() string {
	return n.XMLName.Local
}

// Namespace returns the namespace.
func (n *XMLNode) Namespace() string {
	return n.XMLName.Space
}

// Text returns the text content.
func (n *XMLNode) Text() string {
	return strings.TrimSpace(n.Content)
}

// Attr returns an attribute value.
func (n *XMLNode) Attr(name string) string {
	for _, attr := range n.Attrs {
		if attr.Name.Local == name {
			return attr.Value
		}
	}
	return ""
}

// Child returns the first child with the given name.
func (n *XMLNode) Child(name string) *XMLNode {
	for _, child := range n.Children {
		if child.XMLName.Local == name {
			return child
		}
	}
	return nil
}

// ChildrenByName returns all children with the given name.
func (n *XMLNode) ChildrenByName(name string) []*XMLNode {
	var result []*XMLNode
	for _, child := range n.Children {
		if child.XMLName.Local == name {
			result = append(result, child)
		}
	}
	return result
}

// Parse parses XML content into an XMLNode.
func (m *XMLModule) Parse(data []byte) (*XMLNode, error) {
	if len(data) == 0 {
		return nil, ErrInvalidInput
	}

	var root XMLNode
	if err := xml.Unmarshal(data, &root); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrParseError, err)
	}

	// Set parent references
	setParents(&root, nil)

	return &root, nil
}

func setParents(node *XMLNode, parent *XMLNode) {
	node.parent = parent
	for _, child := range node.Children {
		setParents(child, node)
	}
}

// ParseFile parses XML from a reader.
func (m *XMLModule) ParseFile(r io.Reader) (*XMLNode, error) {
	data, err := io.ReadAll(r)
	if err != nil {
		return nil, err
	}
	return m.Parse(data)
}

// Encode encodes an XMLNode to bytes.
func (m *XMLModule) Encode(node *XMLNode) ([]byte, error) {
	return xml.MarshalIndent(node, "", "  ")
}

// EncodeWithDeclaration includes XML declaration.
func (m *XMLModule) EncodeWithDeclaration(node *XMLNode) ([]byte, error) {
	data, err := m.Encode(node)
	if err != nil {
		return nil, err
	}
	return append([]byte(xml.Header), data...), nil
}

// ToMap converts an XMLNode to a map.
func (m *XMLModule) ToMap(node *XMLNode) map[string]interface{} {
	result := make(map[string]interface{})

	// Add attributes
	if len(node.Attrs) > 0 {
		attrs := make(map[string]string)
		for _, attr := range node.Attrs {
			attrs[attr.Name.Local] = attr.Value
		}
		result["@attrs"] = attrs
	}

	// Add text content
	text := strings.TrimSpace(node.Content)
	if text != "" && len(node.Children) == 0 {
		result["#text"] = text
	}

	// Add children
	for _, child := range node.Children {
		childMap := m.ToMap(child)
		name := child.XMLName.Local

		if existing, ok := result[name]; ok {
			// Convert to array
			if arr, isArr := existing.([]interface{}); isArr {
				result[name] = append(arr, childMap)
			} else {
				result[name] = []interface{}{existing, childMap}
			}
		} else {
			result[name] = childMap
		}
	}

	return result
}

// FromMap creates an XMLNode from a map.
func (m *XMLModule) FromMap(name string, data map[string]interface{}) *XMLNode {
	node := &XMLNode{
		XMLName: xml.Name{Local: name},
	}

	for key, value := range data {
		if key == "@attrs" {
			if attrs, ok := value.(map[string]string); ok {
				for k, v := range attrs {
					node.Attrs = append(node.Attrs, xml.Attr{
						Name:  xml.Name{Local: k},
						Value: v,
					})
				}
			}
		} else if key == "#text" {
			if text, ok := value.(string); ok {
				node.Content = text
			}
		} else {
			switch v := value.(type) {
			case map[string]interface{}:
				child := m.FromMap(key, v)
				node.Children = append(node.Children, child)
			case []interface{}:
				for _, item := range v {
					if itemMap, ok := item.(map[string]interface{}); ok {
						child := m.FromMap(key, itemMap)
						node.Children = append(node.Children, child)
					}
				}
			case string:
				child := &XMLNode{
					XMLName: xml.Name{Local: key},
					Content: v,
				}
				node.Children = append(node.Children, child)
			}
		}
	}

	return node
}

// XPath performs a simple XPath-like query.
func (m *XMLModule) XPath(node *XMLNode, path string) []*XMLNode {
	parts := strings.Split(strings.TrimPrefix(path, "/"), "/")
	return xpathSearch([]*XMLNode{node}, parts)
}

func xpathSearch(nodes []*XMLNode, parts []string) []*XMLNode {
	if len(parts) == 0 || len(nodes) == 0 {
		return nodes
	}

	part := parts[0]
	var matches []*XMLNode

	for _, node := range nodes {
		if part == "*" {
			matches = append(matches, node.Children...)
		} else if strings.HasPrefix(part, "@") {
			// Attribute access - not returning nodes
			continue
		} else if strings.Contains(part, "[") {
			// Predicate
			name := part[:strings.Index(part, "[")]
			for _, child := range node.Children {
				if child.XMLName.Local == name {
					matches = append(matches, child)
				}
			}
		} else {
			for _, child := range node.Children {
				if child.XMLName.Local == part {
					matches = append(matches, child)
				}
			}
		}
	}

	if len(parts) > 1 {
		return xpathSearch(matches, parts[1:])
	}

	return matches
}

// Helper functions

func mergeMaps(dst, src map[string]interface{}) {
	for k, v := range src {
		if dstVal, exists := dst[k]; exists {
			if dstMap, ok := dstVal.(map[string]interface{}); ok {
				if srcMap, ok := v.(map[string]interface{}); ok {
					mergeMaps(dstMap, srcMap)
					continue
				}
			}
		}
		dst[k] = v
	}
}

func getPath(doc interface{}, path string) (interface{}, error) {
	parts := strings.Split(path, ".")
	current := doc

	for _, part := range parts {
		if part == "" {
			continue
		}

		switch v := current.(type) {
		case map[string]interface{}:
			val, ok := v[part]
			if !ok {
				return nil, fmt.Errorf("key not found: %s", part)
			}
			current = val
		default:
			return nil, fmt.Errorf("cannot access %s on non-map type", part)
		}
	}

	return current, nil
}

func setPath(doc interface{}, path string, value interface{}) error {
	parts := strings.Split(path, ".")
	if len(parts) == 0 {
		return errors.New("empty path")
	}

	current := doc
	for i, part := range parts[:len(parts)-1] {
		if part == "" {
			continue
		}

		switch v := current.(type) {
		case map[string]interface{}:
			val, ok := v[part]
			if !ok {
				// Create intermediate maps
				newMap := make(map[string]interface{})
				v[part] = newMap
				current = newMap
			} else {
				current = val
			}
		default:
			return fmt.Errorf("cannot access %s on non-map type at index %d", part, i)
		}
	}

	lastPart := parts[len(parts)-1]
	if m, ok := current.(map[string]interface{}); ok {
		m[lastPart] = value
		return nil
	}

	return errors.New("cannot set value on non-map type")
}
