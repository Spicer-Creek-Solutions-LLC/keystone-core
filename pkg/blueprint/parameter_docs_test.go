package blueprint

import (
	"strings"
	"testing"
)

func TestNewParameterDocGenerator(t *testing.T) {
	gen := NewParameterDocGenerator()
	if gen == nil {
		t.Fatal("NewParameterDocGenerator returned nil")
	}

	// Check defaults
	if gen.format != DocFormatMarkdown {
		t.Errorf("default format = %s, want markdown", gen.format)
	}
	if !gen.includeExamples {
		t.Error("includeExamples should default to true")
	}
	if !gen.groupByCategory {
		t.Error("groupByCategory should default to true")
	}
	if !gen.showRequiredFirst {
		t.Error("showRequiredFirst should default to true")
	}
}

func TestParameterDocGenerator_SetFormat(t *testing.T) {
	gen := NewParameterDocGenerator()

	gen.SetFormat(DocFormatHTML)
	if gen.format != DocFormatHTML {
		t.Errorf("format = %s, want html", gen.format)
	}

	gen.SetFormat(DocFormatPlainText)
	if gen.format != DocFormatPlainText {
		t.Errorf("format = %s, want plaintext", gen.format)
	}
}

func TestParameterDocGenerator_GenerateMarkdown(t *testing.T) {
	gen := NewParameterDocGenerator()
	gen.SetFormat(DocFormatMarkdown)

	minVal := float64(1)
	maxVal := float64(65535)
	minLen := 1
	maxLen := 255

	bp := &Blueprint{
		Metadata: Metadata{
			Name:        "test-blueprint",
			Version:     "1.0.0",
			Description: "A test blueprint for documentation generation.",
		},
		Parameters: map[string]ParameterSchema{
			"port": {
				Type:        "integer",
				Description: "The port number to use.",
				Default:     8080,
				Examples:    []interface{}{3000},
				Minimum:     &minVal,
				Maximum:     &maxVal,
			},
			"hostname": {
				Type:        "string",
				Description: "The hostname for the server.",
				Required:    true,
				Format:      "hostname",
				MinLength:   &minLen,
				MaxLength:   &maxLen,
			},
			"password": {
				Type:        "string",
				Description: "The admin password.",
				Sensitive:   true,
			},
			"log_level": {
				Type:        "string",
				Description: "Logging level.",
				Default:     "info",
				Enum:        []interface{}{"debug", "info", "warn", "error"},
			},
		},
	}

	doc, err := gen.GenerateDocsString(bp)
	if err != nil {
		t.Fatalf("GenerateDocsString() error = %v", err)
	}

	// Check that key sections are present
	if !strings.Contains(doc, "# test-blueprint Parameters") {
		t.Error("missing title")
	}
	if !strings.Contains(doc, "A test blueprint for documentation generation.") {
		t.Error("missing description")
	}
	if !strings.Contains(doc, "### hostname") {
		t.Error("missing hostname parameter")
	}
	if !strings.Contains(doc, "### port") {
		t.Error("missing port parameter")
	}
	if !strings.Contains(doc, "| Required | Yes |") {
		t.Error("missing required indication for hostname")
	}
	if !strings.Contains(doc, "| Sensitive | Yes") {
		t.Error("missing sensitive indication for password")
	}
	if !strings.Contains(doc, "Must be >= 1") {
		t.Error("missing minimum constraint")
	}
	if !strings.Contains(doc, "debug, info, warn, error") {
		t.Error("missing enum values")
	}
}

func TestParameterDocGenerator_GeneratePlainText(t *testing.T) {
	gen := NewParameterDocGenerator()
	gen.SetFormat(DocFormatPlainText)

	bp := &Blueprint{
		Metadata: Metadata{
			Name:        "test-blueprint",
			Version:     "1.0.0",
			Description: "Test description.",
		},
		Parameters: map[string]ParameterSchema{
			"port": {
				Type:        "integer",
				Description: "Port number.",
				Default:     8080,
			},
		},
	}

	doc, err := gen.GenerateDocsString(bp)
	if err != nil {
		t.Fatalf("GenerateDocsString() error = %v", err)
	}

	if !strings.Contains(doc, "test-blueprint Parameters") {
		t.Error("missing title")
	}
	if !strings.Contains(doc, "Type: integer") {
		t.Error("missing type")
	}
	if !strings.Contains(doc, "Default: 8080") {
		t.Error("missing default")
	}
}

func TestParameterDocGenerator_GenerateHTML(t *testing.T) {
	gen := NewParameterDocGenerator()
	gen.SetFormat(DocFormatHTML)

	bp := &Blueprint{
		Metadata: Metadata{
			Name:    "test-blueprint",
			Version: "1.0.0",
		},
		Parameters: map[string]ParameterSchema{
			"enabled": {
				Type:        "boolean",
				Description: "Enable the feature.",
				Required:    true,
				Default:     true,
			},
		},
	}

	doc, err := gen.GenerateDocsString(bp)
	if err != nil {
		t.Fatalf("GenerateDocsString() error = %v", err)
	}

	if !strings.Contains(doc, "<!DOCTYPE html>") {
		t.Error("missing DOCTYPE")
	}
	if !strings.Contains(doc, "<title>test-blueprint Parameters</title>") {
		t.Error("missing title")
	}
	if !strings.Contains(doc, "<code>enabled</code>") {
		t.Error("missing parameter name")
	}
	if !strings.Contains(doc, "class=\"required\"") {
		t.Error("missing required class")
	}
}

func TestParameterDocGenerator_GenerateJSON(t *testing.T) {
	gen := NewParameterDocGenerator()
	gen.SetFormat(DocFormatJSON)

	bp := &Blueprint{
		Metadata: Metadata{
			Name:        "test-blueprint",
			Version:     "1.0.0",
			Description: "A test blueprint.",
		},
		Parameters: map[string]ParameterSchema{
			"count": {
				Type:        "integer",
				Description: "Item count.",
				Default:     10,
			},
		},
	}

	doc, err := gen.GenerateDocsString(bp)
	if err != nil {
		t.Fatalf("GenerateDocsString() error = %v", err)
	}

	if !strings.Contains(doc, `"name": "test-blueprint"`) {
		t.Error("missing name in JSON")
	}
	if !strings.Contains(doc, `"version": "1.0.0"`) {
		t.Error("missing version in JSON")
	}
	if !strings.Contains(doc, `"type": "integer"`) {
		t.Error("missing parameter type in JSON")
	}
}

func TestParameterDocGenerator_NestedParameters(t *testing.T) {
	gen := NewParameterDocGenerator()
	gen.SetFormat(DocFormatMarkdown)

	bp := &Blueprint{
		Metadata: Metadata{
			Name:    "test-blueprint",
			Version: "1.0.0",
		},
		Parameters: map[string]ParameterSchema{
			"database": {
				Type:        "object",
				Description: "Database configuration.",
				Properties: map[string]ParameterSchema{
					"host": {
						Type:        "string",
						Description: "Database host.",
						Required:    true,
						Default:     "localhost",
					},
					"port": {
						Type:        "integer",
						Description: "Database port.",
						Default:     5432,
					},
				},
			},
		},
	}

	doc, err := gen.GenerateDocsString(bp)
	if err != nil {
		t.Fatalf("GenerateDocsString() error = %v", err)
	}

	if !strings.Contains(doc, "### database") {
		t.Error("missing parent parameter")
	}
	if !strings.Contains(doc, "#### Nested Parameters") {
		t.Error("missing nested parameters section")
	}
	if !strings.Contains(doc, "database.host") {
		t.Error("missing nested host parameter")
	}
	if !strings.Contains(doc, "database.port") {
		t.Error("missing nested port parameter")
	}
}

func TestParameterDocGenerator_Constraints(t *testing.T) {
	gen := NewParameterDocGenerator()

	minVal := float64(0)
	maxVal := float64(100)
	minLen := 1
	maxLen := 100
	minItems := 1
	maxItems := 10

	tests := []struct {
		name     string
		schema   ParameterSchema
		expected []string
	}{
		{
			name: "numeric constraints",
			schema: ParameterSchema{
				Type:    "integer",
				Minimum: &minVal,
				Maximum: &maxVal,
			},
			expected: []string{"Must be >= 0", "Must be <= 100"},
		},
		{
			name: "string constraints",
			schema: ParameterSchema{
				Type:      "string",
				MinLength: &minLen,
				MaxLength: &maxLen,
				Pattern:   "^[a-z]+$",
			},
			expected: []string{"Minimum length: 1", "Maximum length: 100", "Must match pattern: ^[a-z]+$"},
		},
		{
			name: "array constraints",
			schema: ParameterSchema{
				Type:     "array",
				MinItems: &minItems,
				MaxItems: &maxItems,
			},
			expected: []string{"Minimum items: 1", "Maximum items: 10"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			constraints := gen.buildConstraints(tt.schema)
			for _, exp := range tt.expected {
				found := false
				for _, c := range constraints {
					if c == exp {
						found = true
						break
					}
				}
				if !found {
					t.Errorf("constraint %q not found in %v", exp, constraints)
				}
			}
		})
	}
}

func TestParameterDocGenerator_Sorting(t *testing.T) {
	gen := NewParameterDocGenerator()
	gen.SetShowRequiredFirst(true)

	docs := []ParameterDoc{
		{Name: "zebra", Required: false},
		{Name: "apple", Required: true},
		{Name: "banana", Required: false},
		{Name: "cherry", Required: true},
	}

	sorted := gen.sortParameters(docs)

	// Required should come first
	if !sorted[0].Required || !sorted[1].Required {
		t.Error("required parameters should come first")
	}

	// Within each group, should be alphabetical
	if sorted[0].Name != "apple" {
		t.Errorf("sorted[0].Name = %s, want apple", sorted[0].Name)
	}
	if sorted[1].Name != "cherry" {
		t.Errorf("sorted[1].Name = %s, want cherry", sorted[1].Name)
	}
	if sorted[2].Name != "banana" {
		t.Errorf("sorted[2].Name = %s, want banana", sorted[2].Name)
	}
	if sorted[3].Name != "zebra" {
		t.Errorf("sorted[3].Name = %s, want zebra", sorted[3].Name)
	}
}

func TestParameterDocGenerator_NoExamples(t *testing.T) {
	gen := NewParameterDocGenerator()
	gen.SetFormat(DocFormatMarkdown)
	gen.SetIncludeExamples(false)

	bp := &Blueprint{
		Metadata: Metadata{
			Name:    "test-blueprint",
			Version: "1.0.0",
		},
		Parameters: map[string]ParameterSchema{
			"port": {
				Type:     "integer",
				Examples: []interface{}{3000},
			},
		},
	}

	doc, err := gen.GenerateDocsString(bp)
	if err != nil {
		t.Fatalf("GenerateDocsString() error = %v", err)
	}

	if strings.Contains(doc, "**Example:**") {
		t.Error("examples should be excluded")
	}
}

func TestParameterDocGenerator_GenerateParameterSummary(t *testing.T) {
	gen := NewParameterDocGenerator()

	bp := &Blueprint{
		Parameters: map[string]ParameterSchema{
			"host": {
				Type:     "string",
				Required: true,
				Default:  "localhost",
			},
			"port": {
				Type:    "integer",
				Default: 8080,
			},
			"enabled": {
				Type:     "boolean",
				Required: true,
			},
		},
	}

	summary := gen.GenerateParameterSummary(bp)

	// Check header
	if !strings.Contains(summary, "| Parameter | Type | Required | Default |") {
		t.Error("missing table header")
	}

	// Check rows
	if !strings.Contains(summary, "| `host` | `string` | Yes | `localhost` |") {
		t.Error("missing host row")
	}
	if !strings.Contains(summary, "| `port` | `integer` | No | `8080` |") {
		t.Error("missing port row")
	}
	if !strings.Contains(summary, "| `enabled` | `boolean` | Yes | - |") {
		t.Error("missing enabled row")
	}
}

func TestParameterDocGenerator_UnsupportedFormat(t *testing.T) {
	gen := NewParameterDocGenerator()
	gen.SetFormat(DocFormat("unsupported"))

	bp := &Blueprint{
		Metadata: Metadata{
			Name:    "test",
			Version: "1.0.0",
		},
	}

	_, err := gen.GenerateDocsString(bp)
	if err == nil {
		t.Error("expected error for unsupported format")
	}
	if !strings.Contains(err.Error(), "unsupported documentation format") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestJsonValue(t *testing.T) {
	tests := []struct {
		value    interface{}
		expected string
	}{
		{"hello", `"hello"`},
		{nil, "null"},
		{true, "true"},
		{false, "false"},
		{42, "42"},
		{int64(42), "42"},
		{3.14, "3.14"},
		{[]string{"a", "b"}, `"[a b]"`}, // Complex types converted to string
	}

	for _, tt := range tests {
		result := jsonValue(tt.value)
		if result != tt.expected {
			t.Errorf("jsonValue(%v) = %s, want %s", tt.value, result, tt.expected)
		}
	}
}
