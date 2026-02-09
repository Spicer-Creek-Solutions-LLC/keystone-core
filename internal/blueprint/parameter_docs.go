package blueprint

import (
	"fmt"
	"io"
	"sort"
	"strings"
	"text/template" // nosemgrep: go.lang.security.audit.xss.import-text-template.import-text-template -- markdown docs generation, not HTML responses
)

// DocFormat represents the output format for parameter documentation.
type DocFormat string

const (
	// DocFormatMarkdown generates Markdown documentation.
	DocFormatMarkdown DocFormat = "markdown"
	// DocFormatPlainText generates plain text documentation.
	DocFormatPlainText DocFormat = "plaintext"
	// DocFormatHTML generates HTML documentation.
	DocFormatHTML DocFormat = "html"
	// DocFormatJSON generates JSON documentation.
	DocFormatJSON DocFormat = "json"
)

// ParameterDocGenerator generates documentation for blueprint parameters.
type ParameterDocGenerator struct {
	// format is the output format
	format DocFormat

	// includeExamples includes example values in documentation
	includeExamples bool

	// groupByCategory groups parameters by category if available
	groupByCategory bool

	// showRequiredFirst shows required parameters before optional ones
	showRequiredFirst bool
}

// NewParameterDocGenerator creates a new documentation generator.
func NewParameterDocGenerator() *ParameterDocGenerator {
	return &ParameterDocGenerator{
		format:            DocFormatMarkdown,
		includeExamples:   true,
		groupByCategory:   true,
		showRequiredFirst: true,
	}
}

// SetFormat sets the output format.
func (g *ParameterDocGenerator) SetFormat(format DocFormat) {
	g.format = format
}

// SetIncludeExamples controls whether examples are included.
func (g *ParameterDocGenerator) SetIncludeExamples(include bool) {
	g.includeExamples = include
}

// SetGroupByCategory controls whether parameters are grouped by category.
func (g *ParameterDocGenerator) SetGroupByCategory(group bool) {
	g.groupByCategory = group
}

// SetShowRequiredFirst controls whether required parameters are shown first.
func (g *ParameterDocGenerator) SetShowRequiredFirst(show bool) {
	g.showRequiredFirst = show
}

// ParameterDoc represents documentation for a single parameter.
type ParameterDoc struct {
	Name           string
	Type           string
	Description    string
	Required       bool
	Default        interface{}
	Example        interface{}
	IncludeExample bool // Whether to include example in output
	Constraints    []string
	Sensitive      bool
	Deprecated     bool
	Category       string
	Format         string
	Enum           []interface{}
	Children       []ParameterDoc // For nested object parameters
}

// GenerateDocs generates documentation for a blueprint's parameters.
func (g *ParameterDocGenerator) GenerateDocs(bp *Blueprint, w io.Writer) error {
	// Get required parameters using the method
	requiredParams := bp.RequiredParameters()
	docs := g.buildParameterDocs(bp.Parameters, requiredParams, "")

	// Sort parameters
	docs = g.sortParameters(docs)

	switch g.format {
	case DocFormatMarkdown:
		return g.writeMarkdown(bp, docs, w)
	case DocFormatPlainText:
		return g.writePlainText(bp, docs, w)
	case DocFormatHTML:
		return g.writeHTML(bp, docs, w)
	case DocFormatJSON:
		return g.writeJSON(bp, docs, w)
	default:
		return fmt.Errorf("unsupported documentation format: %s", g.format)
	}
}

// GenerateDocsString generates documentation and returns it as a string.
func (g *ParameterDocGenerator) GenerateDocsString(bp *Blueprint) (string, error) {
	var buf strings.Builder
	err := g.GenerateDocs(bp, &buf)
	return buf.String(), err
}

// buildParameterDocs converts schema parameters to documentation format.
func (g *ParameterDocGenerator) buildParameterDocs(schemas map[string]ParameterSchema, required []string, prefix string) []ParameterDoc {
	docs := make([]ParameterDoc, 0, len(schemas))

	requiredSet := make(map[string]bool)
	for _, r := range required {
		requiredSet[r] = true
	}

	for name := range schemas {
		schema := schemas[name]
		fullName := name
		if prefix != "" {
			fullName = prefix + "." + name
		}

		// Get first example if available
		var example interface{}
		if len(schema.Examples) > 0 {
			example = schema.Examples[0]
		}

		doc := ParameterDoc{
			Name:           fullName,
			Type:           schema.Type,
			Description:    schema.Description,
			Required:       requiredSet[name] || schema.Required,
			Default:        schema.Default,
			Example:        example,
			IncludeExample: g.includeExamples && example != nil,
			Constraints:    g.buildConstraints(schema),
			Sensitive:      schema.Sensitive,
			Format:         schema.Format,
			Enum:           schema.Enum,
		}

		// Handle nested object parameters
		if schema.Type == "object" && schema.Properties != nil {
			// For nested objects, gather required fields from the properties themselves
			var nestedRequired []string
			for propName := range schema.Properties {
				if schema.Properties[propName].Required {
					nestedRequired = append(nestedRequired, propName)
				}
			}
			doc.Children = g.buildParameterDocs(schema.Properties, nestedRequired, fullName)
		}

		docs = append(docs, doc)
	}

	return docs
}

// buildConstraints extracts constraint descriptions from a schema.
func (g *ParameterDocGenerator) buildConstraints(schema ParameterSchema) []string {
	var constraints []string

	// Numeric constraints
	if schema.Minimum != nil {
		constraints = append(constraints, fmt.Sprintf("Must be >= %v", *schema.Minimum))
	}
	if schema.Maximum != nil {
		constraints = append(constraints, fmt.Sprintf("Must be <= %v", *schema.Maximum))
	}

	// String constraints
	if schema.MinLength != nil && *schema.MinLength > 0 {
		constraints = append(constraints, fmt.Sprintf("Minimum length: %d", *schema.MinLength))
	}
	if schema.MaxLength != nil {
		constraints = append(constraints, fmt.Sprintf("Maximum length: %d", *schema.MaxLength))
	}
	if schema.Pattern != "" {
		constraints = append(constraints, fmt.Sprintf("Must match pattern: %s", schema.Pattern))
	}

	// Array constraints
	if schema.MinItems != nil && *schema.MinItems > 0 {
		constraints = append(constraints, fmt.Sprintf("Minimum items: %d", *schema.MinItems))
	}
	if schema.MaxItems != nil {
		constraints = append(constraints, fmt.Sprintf("Maximum items: %d", *schema.MaxItems))
	}

	// Enum constraint
	if len(schema.Enum) > 0 {
		values := make([]string, len(schema.Enum))
		for i, v := range schema.Enum {
			values[i] = fmt.Sprintf("%v", v)
		}
		constraints = append(constraints, fmt.Sprintf("Allowed values: %s", strings.Join(values, ", ")))
	}

	return constraints
}

// sortParameters sorts parameters based on configuration.
func (g *ParameterDocGenerator) sortParameters(docs []ParameterDoc) []ParameterDoc {
	sorted := make([]ParameterDoc, len(docs))
	copy(sorted, docs)

	sort.Slice(sorted, func(i, j int) bool {
		// Required parameters first if configured
		if g.showRequiredFirst && sorted[i].Required != sorted[j].Required {
			return sorted[i].Required
		}

		// Group by category if configured
		if g.groupByCategory && sorted[i].Category != sorted[j].Category {
			return sorted[i].Category < sorted[j].Category
		}

		// Finally, sort alphabetically by name
		return sorted[i].Name < sorted[j].Name
	})

	// Recursively sort children
	for i := range sorted {
		if len(sorted[i].Children) > 0 {
			sorted[i].Children = g.sortParameters(sorted[i].Children)
		}
	}

	return sorted
}

// Markdown template for parameter documentation
const markdownTemplate = `# {{.Blueprint.Metadata.Name}} Parameters

{{if .Blueprint.Metadata.Description}}{{.Blueprint.Metadata.Description}}

{{end}}## Parameters

{{range .Docs}}{{template "param" .}}{{end}}
{{define "param"}}### {{.Name}}

{{if .Deprecated}}> **Deprecated**: This parameter is deprecated.

{{end}}{{if .Description}}{{.Description}}

{{end}}| Property | Value |
|----------|-------|
| Type | ` + "`{{.Type}}`" + ` |
| Required | {{if .Required}}Yes{{else}}No{{end}} |
{{if .Format}}| Format | ` + "`{{.Format}}`" + ` |
{{end}}{{if .Default}}| Default | ` + "`{{printf \"%v\" .Default}}`" + ` |
{{end}}{{if .Sensitive}}| Sensitive | Yes (value will be masked in logs) |
{{end}}
{{if .Constraints}}**Constraints:**
{{range .Constraints}}- {{.}}
{{end}}
{{end}}{{if .IncludeExample}}**Example:**
` + "```yaml" + `
{{.Name}}: {{printf "%v" .Example}}
` + "```" + `

{{end}}{{if .Children}}#### Nested Parameters

{{range .Children}}{{template "nestedparam" .}}{{end}}{{end}}{{end}}
{{define "nestedparam"}}##### {{.Name}}

{{if .Deprecated}}> **Deprecated**: This parameter is deprecated.

{{end}}{{if .Description}}{{.Description}}

{{end}}| Property | Value |
|----------|-------|
| Type | ` + "`{{.Type}}`" + ` |
| Required | {{if .Required}}Yes{{else}}No{{end}} |
{{if .Format}}| Format | ` + "`{{.Format}}`" + ` |
{{end}}{{if .Default}}| Default | ` + "`{{printf \"%v\" .Default}}`" + ` |
{{end}}{{if .Sensitive}}| Sensitive | Yes |
{{end}}
{{if .Constraints}}**Constraints:**
{{range .Constraints}}- {{.}}
{{end}}
{{end}}{{end}}`

// writeMarkdown writes documentation in Markdown format.
func (g *ParameterDocGenerator) writeMarkdown(bp *Blueprint, docs []ParameterDoc, w io.Writer) error {
	tmpl, err := template.New("markdown").Parse(markdownTemplate)
	if err != nil {
		return fmt.Errorf("failed to parse markdown template: %w", err)
	}

	data := struct {
		Blueprint       *Blueprint
		Docs            []ParameterDoc
		IncludeExamples bool
	}{
		Blueprint:       bp,
		Docs:            docs,
		IncludeExamples: g.includeExamples,
	}

	return tmpl.Execute(w, data)
}

// writePlainText writes documentation in plain text format.
func (g *ParameterDocGenerator) writePlainText(bp *Blueprint, docs []ParameterDoc, w io.Writer) error {
	fmt.Fprintf(w, "%s Parameters\n", bp.Metadata.Name)
	fmt.Fprintln(w, strings.Repeat("=", len(bp.Metadata.Name)+11))
	fmt.Fprintln(w)

	if bp.Metadata.Description != "" {
		fmt.Fprintln(w, bp.Metadata.Description)
		fmt.Fprintln(w)
	}

	for i := range docs {
		g.writeParameterPlainText(docs[i], w, 0)
	}

	return nil
}

func (g *ParameterDocGenerator) writeParameterPlainText(doc ParameterDoc, w io.Writer, indent int) {
	prefix := strings.Repeat("  ", indent)

	fmt.Fprintf(w, "%s%s\n", prefix, doc.Name)
	fmt.Fprintf(w, "%s%s\n", prefix, strings.Repeat("-", len(doc.Name)))

	if doc.Deprecated {
		fmt.Fprintf(w, "%s  [DEPRECATED]\n", prefix)
	}

	if doc.Description != "" {
		fmt.Fprintf(w, "%s  %s\n", prefix, doc.Description)
	}

	fmt.Fprintf(w, "%s  Type: %s\n", prefix, doc.Type)
	if doc.Required {
		fmt.Fprintf(w, "%s  Required: Yes\n", prefix)
	} else {
		fmt.Fprintf(w, "%s  Required: No\n", prefix)
	}

	if doc.Format != "" {
		fmt.Fprintf(w, "%s  Format: %s\n", prefix, doc.Format)
	}

	if doc.Default != nil {
		fmt.Fprintf(w, "%s  Default: %v\n", prefix, doc.Default)
	}

	if doc.Sensitive {
		fmt.Fprintf(w, "%s  Sensitive: Yes\n", prefix)
	}

	if len(doc.Constraints) > 0 {
		fmt.Fprintf(w, "%s  Constraints:\n", prefix)
		for _, c := range doc.Constraints {
			fmt.Fprintf(w, "%s    - %s\n", prefix, c)
		}
	}

	if g.includeExamples && doc.Example != nil {
		fmt.Fprintf(w, "%s  Example: %v\n", prefix, doc.Example)
	}

	fmt.Fprintln(w)

	// Handle nested parameters
	for i := range doc.Children {
		g.writeParameterPlainText(doc.Children[i], w, indent+1)
	}
}

// writeHTML writes documentation in HTML format.
func (g *ParameterDocGenerator) writeHTML(bp *Blueprint, docs []ParameterDoc, w io.Writer) error {
	fmt.Fprintln(w, "<!DOCTYPE html>")
	fmt.Fprintln(w, "<html>")
	fmt.Fprintln(w, "<head>")
	fmt.Fprintf(w, "  <title>%s Parameters</title>\n", bp.Metadata.Name)
	fmt.Fprintln(w, "  <style>")
	fmt.Fprintln(w, "    body { font-family: sans-serif; max-width: 800px; margin: 0 auto; padding: 20px; }")
	fmt.Fprintln(w, "    h1 { color: #333; }")
	fmt.Fprintln(w, "    .parameter { border: 1px solid #ddd; padding: 15px; margin: 10px 0; border-radius: 4px; }")
	fmt.Fprintln(w, "    .parameter h3 { margin-top: 0; color: #0066cc; }")
	fmt.Fprintln(w, "    .required { color: #cc0000; font-weight: bold; }")
	fmt.Fprintln(w, "    .deprecated { background: #fff3cd; color: #856404; padding: 5px 10px; border-radius: 3px; }")
	fmt.Fprintln(w, "    .sensitive { color: #dc3545; }")
	fmt.Fprintln(w, "    code { background: #f4f4f4; padding: 2px 6px; border-radius: 3px; }")
	fmt.Fprintln(w, "    table { border-collapse: collapse; width: 100%; }")
	fmt.Fprintln(w, "    th, td { border: 1px solid #ddd; padding: 8px; text-align: left; }")
	fmt.Fprintln(w, "    th { background: #f4f4f4; }")
	fmt.Fprintln(w, "    .nested { margin-left: 20px; }")
	fmt.Fprintln(w, "  </style>")
	fmt.Fprintln(w, "</head>")
	fmt.Fprintln(w, "<body>")
	fmt.Fprintf(w, "  <h1>%s Parameters</h1>\n", bp.Metadata.Name)

	if bp.Metadata.Description != "" {
		fmt.Fprintf(w, "  <p>%s</p>\n", bp.Metadata.Description)
	}

	fmt.Fprintln(w, "  <h2>Parameters</h2>")

	for i := range docs {
		g.writeParameterHTML(docs[i], w, false)
	}

	fmt.Fprintln(w, "</body>")
	fmt.Fprintln(w, "</html>")

	return nil
}

func (g *ParameterDocGenerator) writeParameterHTML(doc ParameterDoc, w io.Writer, nested bool) {
	class := "parameter"
	if nested {
		class += " nested"
	}

	fmt.Fprintf(w, "  <div class=\"%s\">\n", class)
	fmt.Fprintf(w, "    <h3><code>%s</code></h3>\n", doc.Name)

	if doc.Deprecated {
		fmt.Fprintln(w, "    <span class=\"deprecated\">Deprecated</span>")
	}

	if doc.Description != "" {
		fmt.Fprintf(w, "    <p>%s</p>\n", doc.Description)
	}

	fmt.Fprintln(w, "    <table>")
	fmt.Fprintf(w, "      <tr><th>Type</th><td><code>%s</code></td></tr>\n", doc.Type)

	if doc.Required {
		fmt.Fprintln(w, "      <tr><th>Required</th><td class=\"required\">Yes</td></tr>")
	} else {
		fmt.Fprintln(w, "      <tr><th>Required</th><td>No</td></tr>")
	}

	if doc.Format != "" {
		fmt.Fprintf(w, "      <tr><th>Format</th><td><code>%s</code></td></tr>\n", doc.Format)
	}

	if doc.Default != nil {
		fmt.Fprintf(w, "      <tr><th>Default</th><td><code>%v</code></td></tr>\n", doc.Default)
	}

	if doc.Sensitive {
		fmt.Fprintln(w, "      <tr><th>Sensitive</th><td class=\"sensitive\">Yes</td></tr>")
	}

	fmt.Fprintln(w, "    </table>")

	if len(doc.Constraints) > 0 {
		fmt.Fprintln(w, "    <p><strong>Constraints:</strong></p>")
		fmt.Fprintln(w, "    <ul>")
		for _, c := range doc.Constraints {
			fmt.Fprintf(w, "      <li>%s</li>\n", c)
		}
		fmt.Fprintln(w, "    </ul>")
	}

	if g.includeExamples && doc.Example != nil {
		fmt.Fprintln(w, "    <p><strong>Example:</strong></p>")
		fmt.Fprintf(w, "    <pre><code>%s: %v</code></pre>\n", doc.Name, doc.Example)
	}

	// Handle nested parameters
	if len(doc.Children) > 0 {
		fmt.Fprintln(w, "    <h4>Nested Parameters:</h4>")
		for j := range doc.Children {
			g.writeParameterHTML(doc.Children[j], w, true)
		}
	}

	fmt.Fprintln(w, "  </div>")
}

// writeJSON writes documentation in JSON format.
func (g *ParameterDocGenerator) writeJSON(bp *Blueprint, docs []ParameterDoc, w io.Writer) error {
	fmt.Fprintln(w, "{")
	fmt.Fprintf(w, "  \"name\": %q,\n", bp.Metadata.Name)
	fmt.Fprintf(w, "  \"version\": %q,\n", bp.Metadata.Version)

	if bp.Metadata.Description != "" {
		fmt.Fprintf(w, "  \"description\": %q,\n", bp.Metadata.Description)
	}

	fmt.Fprintln(w, "  \"parameters\": [")

	for i := range docs {
		g.writeParameterJSON(docs[i], w, 2)
		if i < len(docs)-1 {
			fmt.Fprintln(w, ",")
		} else {
			fmt.Fprintln(w)
		}
	}

	fmt.Fprintln(w, "  ]")
	fmt.Fprintln(w, "}")

	return nil
}

func (g *ParameterDocGenerator) writeParameterJSON(doc ParameterDoc, w io.Writer, indent int) {
	prefix := strings.Repeat("  ", indent)

	fmt.Fprintf(w, "%s{\n", prefix)
	fmt.Fprintf(w, "%s  \"name\": %q,\n", prefix, doc.Name)
	fmt.Fprintf(w, "%s  \"type\": %q,\n", prefix, doc.Type)
	fmt.Fprintf(w, "%s  \"required\": %t,\n", prefix, doc.Required)

	if doc.Description != "" {
		fmt.Fprintf(w, "%s  \"description\": %q,\n", prefix, doc.Description)
	}

	if doc.Format != "" {
		fmt.Fprintf(w, "%s  \"format\": %q,\n", prefix, doc.Format)
	}

	if doc.Default != nil {
		fmt.Fprintf(w, "%s  \"default\": %v,\n", prefix, jsonValue(doc.Default))
	}

	if doc.Sensitive {
		fmt.Fprintf(w, "%s  \"sensitive\": true,\n", prefix)
	}

	if doc.Deprecated {
		fmt.Fprintf(w, "%s  \"deprecated\": true,\n", prefix)
	}

	if doc.Category != "" {
		fmt.Fprintf(w, "%s  \"category\": %q,\n", prefix, doc.Category)
	}

	if len(doc.Constraints) > 0 {
		fmt.Fprintf(w, "%s  \"constraints\": [\n", prefix)
		for i, c := range doc.Constraints {
			fmt.Fprintf(w, "%s    %q", prefix, c)
			if i < len(doc.Constraints)-1 {
				fmt.Fprintln(w, ",")
			} else {
				fmt.Fprintln(w)
			}
		}
		fmt.Fprintf(w, "%s  ],\n", prefix)
	}

	if len(doc.Enum) > 0 {
		fmt.Fprintf(w, "%s  \"enum\": [", prefix)
		for i, v := range doc.Enum {
			fmt.Fprintf(w, "%v", jsonValue(v))
			if i < len(doc.Enum)-1 {
				fmt.Fprint(w, ", ")
			}
		}
		fmt.Fprintln(w, "],")
	}

	if g.includeExamples && doc.Example != nil {
		fmt.Fprintf(w, "%s  \"example\": %v,\n", prefix, jsonValue(doc.Example))
	}

	// Handle nested parameters
	if len(doc.Children) > 0 {
		fmt.Fprintf(w, "%s  \"children\": [\n", prefix)
		for i := range doc.Children {
			g.writeParameterJSON(doc.Children[i], w, indent+2)
			if i < len(doc.Children)-1 {
				fmt.Fprintln(w, ",")
			} else {
				fmt.Fprintln(w)
			}
		}
		fmt.Fprintf(w, "%s  ],\n", prefix)
	}

	// Remove trailing comma from last property by writing a final property without comma
	fmt.Fprintf(w, "%s  \"_\": null\n", prefix)
	fmt.Fprintf(w, "%s}", prefix)
}

// jsonValue converts a value to JSON representation.
func jsonValue(v interface{}) string {
	switch val := v.(type) {
	case string:
		return fmt.Sprintf("%q", val)
	case nil:
		return "null"
	case bool, int, int64, float64:
		return fmt.Sprintf("%v", val)
	default:
		return fmt.Sprintf("%q", fmt.Sprintf("%v", val))
	}
}

// GenerateParameterSummary generates a brief summary table of parameters.
func (g *ParameterDocGenerator) GenerateParameterSummary(bp *Blueprint) string {
	var buf strings.Builder

	buf.WriteString("| Parameter | Type | Required | Default |\n")
	buf.WriteString("|-----------|------|----------|--------|\n")

	requiredParams := bp.RequiredParameters()
	requiredSet := make(map[string]bool)
	for _, r := range requiredParams {
		requiredSet[r] = true
	}

	// Sort parameter names
	names := make([]string, 0, len(bp.Parameters))
	for name := range bp.Parameters {
		names = append(names, name)
	}
	sort.Strings(names)

	for _, name := range names {
		schema := bp.Parameters[name]
		required := "No"
		if requiredSet[name] || schema.Required {
			required = "Yes"
		}

		defaultVal := "-"
		if schema.Default != nil {
			defaultVal = fmt.Sprintf("`%v`", schema.Default)
		}

		buf.WriteString(fmt.Sprintf("| `%s` | `%s` | %s | %s |\n",
			name, schema.Type, required, defaultVal))
	}

	return buf.String()
}
