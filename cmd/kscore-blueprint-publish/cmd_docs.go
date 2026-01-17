package main

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"text/template"

	"github.com/spf13/cobra"

	"github.com/shawnbutts/keystone-core/pkg/blueprint"
)

var (
	docsOutputDir    string
	docsFormat       string
	docsIncludeUsage bool
)

var docsCmd = &cobra.Command{
	Use:   "docs [path]",
	Short: "Generate documentation from blueprint",
	Long: `Generate documentation from a blueprint's manifest.

Creates comprehensive documentation including parameter descriptions,
examples, dependencies, and usage instructions.

Supported output formats:
  - markdown (default): Standard Markdown
  - html: Standalone HTML page
  - json: Machine-readable JSON

Examples:
  # Generate markdown docs in docs/ directory
  kscorectl blueprint-publish docs .

  # Generate to custom directory
  kscorectl blueprint-publish docs . --output ./documentation

  # Generate HTML format
  kscorectl blueprint-publish docs . --format html

  # Include usage examples
  kscorectl blueprint-publish docs . --include-usage`,
	Args: cobra.MaximumNArgs(1),
	RunE: docsExecute,
}

func init() {
	docsCmd.Flags().StringVarP(&docsOutputDir, "output", "o", "docs", "Output directory")
	docsCmd.Flags().StringVar(&docsFormat, "format", "markdown", "Output format (markdown, html, json)")
	docsCmd.Flags().BoolVar(&docsIncludeUsage, "include-usage", true, "Include usage examples")
}

func docsExecute(cmd *cobra.Command, args []string) error {
	path := "."
	if len(args) > 0 {
		path = args[0]
	}

	// Find and parse blueprint.yaml
	manifestPath := filepath.Join(path, "blueprint.yaml")
	if _, err := os.Stat(manifestPath); os.IsNotExist(err) {
		return fmt.Errorf("blueprint.yaml not found in %s", path)
	}

	bp, err := blueprint.ParseManifestFile(manifestPath)
	if err != nil {
		return fmt.Errorf("failed to parse blueprint.yaml: %w", err)
	}

	// Create output directory
	outputDir := filepath.Join(path, docsOutputDir)
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		return fmt.Errorf("failed to create output directory: %w", err)
	}

	fmt.Printf("Generating documentation for %s@%s\n", bp.Metadata.Name, bp.Metadata.Version)

	switch docsFormat {
	case "markdown":
		return generateMarkdownDocs(bp, outputDir, path)
	case "html":
		return generateHTMLDocs(bp, outputDir, path)
	case "json":
		return generateJSONDocs(bp, outputDir)
	default:
		return fmt.Errorf("unsupported format: %s", docsFormat)
	}
}

func generateMarkdownDocs(bp *blueprint.Blueprint, outputDir, basePath string) error {
	// Generate main README
	readmePath := filepath.Join(outputDir, "README.md")
	f, err := os.Create(readmePath)
	if err != nil {
		return err
	}
	defer f.Close()

	tmpl := template.Must(template.New("readme").Funcs(template.FuncMap{
		"join": strings.Join,
	}).Parse(markdownTemplate))

	data := &docData{
		Blueprint:    bp,
		IncludeUsage: docsIncludeUsage,
	}

	if err := tmpl.Execute(f, data); err != nil {
		return fmt.Errorf("failed to generate README: %w", err)
	}

	fmt.Printf("  ✓ %s\n", readmePath)

	// Generate parameters reference if there are parameters
	if len(bp.Parameters) > 0 {
		paramsPath := filepath.Join(outputDir, "PARAMETERS.md")
		if err := generateParametersDoc(bp, paramsPath); err != nil {
			return err
		}
		fmt.Printf("  ✓ %s\n", paramsPath)
	}

	// Generate dependencies reference if there are dependencies
	if bp.Dependencies != nil && (len(bp.Dependencies.Requires) > 0 || len(bp.Dependencies.RequiresBefore) > 0) {
		depsPath := filepath.Join(outputDir, "DEPENDENCIES.md")
		if err := generateDependenciesDoc(bp, depsPath); err != nil {
			return err
		}
		fmt.Printf("  ✓ %s\n", depsPath)
	}

	fmt.Printf("\n✓ Documentation generated in %s\n", outputDir)
	return nil
}

func generateParametersDoc(bp *blueprint.Blueprint, outputPath string) error {
	f, err := os.Create(outputPath)
	if err != nil {
		return err
	}
	defer f.Close()

	tmpl := template.Must(template.New("params").Parse(parametersTemplate))
	return tmpl.Execute(f, bp)
}

func generateDependenciesDoc(bp *blueprint.Blueprint, outputPath string) error {
	f, err := os.Create(outputPath)
	if err != nil {
		return err
	}
	defer f.Close()

	tmpl := template.Must(template.New("deps").Parse(dependenciesTemplate))
	return tmpl.Execute(f, bp)
}

func generateHTMLDocs(bp *blueprint.Blueprint, outputDir, basePath string) error {
	htmlPath := filepath.Join(outputDir, "index.html")
	f, err := os.Create(htmlPath)
	if err != nil {
		return err
	}
	defer f.Close()

	tmpl := template.Must(template.New("html").Funcs(template.FuncMap{
		"join": strings.Join,
	}).Parse(htmlTemplate))

	data := &docData{
		Blueprint:    bp,
		IncludeUsage: docsIncludeUsage,
	}

	if err := tmpl.Execute(f, data); err != nil {
		return fmt.Errorf("failed to generate HTML: %w", err)
	}

	fmt.Printf("  ✓ %s\n", htmlPath)
	fmt.Printf("\n✓ Documentation generated in %s\n", outputDir)
	return nil
}

func generateJSONDocs(bp *blueprint.Blueprint, outputDir string) error {
	jsonPath := filepath.Join(outputDir, "blueprint.json")

	// Create JSON documentation structure
	doc := map[string]interface{}{
		"name":         bp.Metadata.Name,
		"version":      bp.Metadata.Version,
		"description":  bp.Metadata.Description,
		"parameters":   formatParametersForJSON(bp.Parameters),
		"dependencies": bp.Dependencies,
	}

	// Build author from maintainers
	var author string
	if len(bp.Metadata.Maintainers) > 0 {
		author = bp.Metadata.Maintainers[0].Name
	}

	doc["metadata"] = map[string]interface{}{
		"author":     author,
		"license":    bp.Metadata.License,
		"categories": bp.Metadata.Categories,
		"keywords":   bp.Metadata.Keywords,
	}

	f, err := os.Create(jsonPath)
	if err != nil {
		return err
	}
	defer f.Close()

	// Simple JSON serialization
	if err := writeJSON(f, doc); err != nil {
		return fmt.Errorf("failed to generate JSON: %w", err)
	}

	fmt.Printf("  ✓ %s\n", jsonPath)
	fmt.Printf("\n✓ Documentation generated in %s\n", outputDir)
	return nil
}

func formatParametersForJSON(params map[string]blueprint.ParameterSchema) []map[string]interface{} {
	var result []map[string]interface{}

	// Sort parameter names for consistent output
	names := make([]string, 0, len(params))
	for name := range params {
		names = append(names, name)
	}
	sort.Strings(names)

	for _, name := range names {
		param := params[name]
		p := map[string]interface{}{
			"name":        name,
			"type":        param.Type,
			"description": param.Description,
			"required":    param.Required,
		}
		if param.Default != nil {
			p["default"] = param.Default
		}
		if len(param.Enum) > 0 {
			p["enum"] = param.Enum
		}
		if len(param.Examples) > 0 {
			p["examples"] = param.Examples
		}
		result = append(result, p)
	}

	return result
}

func writeJSON(f *os.File, data interface{}) error {
	// Simple JSON writing without external dependencies
	return writeJSONValue(f, data, 0)
}

func writeJSONValue(f *os.File, data interface{}, indent int) error {
	prefix := strings.Repeat("  ", indent)
	switch v := data.(type) {
	case map[string]interface{}:
		f.WriteString("{\n")
		keys := make([]string, 0, len(v))
		for k := range v {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for i, k := range keys {
			f.WriteString(prefix + "  \"" + k + "\": ")
			if err := writeJSONValue(f, v[k], indent+1); err != nil {
				return err
			}
			if i < len(keys)-1 {
				f.WriteString(",")
			}
			f.WriteString("\n")
		}
		f.WriteString(prefix + "}")
	case []interface{}:
		f.WriteString("[\n")
		for i, item := range v {
			f.WriteString(prefix + "  ")
			if err := writeJSONValue(f, item, indent+1); err != nil {
				return err
			}
			if i < len(v)-1 {
				f.WriteString(",")
			}
			f.WriteString("\n")
		}
		f.WriteString(prefix + "]")
	case []map[string]interface{}:
		f.WriteString("[\n")
		for i, item := range v {
			f.WriteString(prefix + "  ")
			if err := writeJSONValue(f, item, indent+1); err != nil {
				return err
			}
			if i < len(v)-1 {
				f.WriteString(",")
			}
			f.WriteString("\n")
		}
		f.WriteString(prefix + "]")
	case []string:
		f.WriteString("[")
		for i, s := range v {
			f.WriteString("\"" + escapeJSON(s) + "\"")
			if i < len(v)-1 {
				f.WriteString(", ")
			}
		}
		f.WriteString("]")
	case string:
		f.WriteString("\"" + escapeJSON(v) + "\"")
	case bool:
		if v {
			f.WriteString("true")
		} else {
			f.WriteString("false")
		}
	case int, int64, float64:
		f.WriteString(fmt.Sprintf("%v", v))
	case nil:
		f.WriteString("null")
	default:
		f.WriteString(fmt.Sprintf("\"%v\"", v))
	}
	return nil
}

func escapeJSON(s string) string {
	s = strings.ReplaceAll(s, "\\", "\\\\")
	s = strings.ReplaceAll(s, "\"", "\\\"")
	s = strings.ReplaceAll(s, "\n", "\\n")
	s = strings.ReplaceAll(s, "\r", "\\r")
	s = strings.ReplaceAll(s, "\t", "\\t")
	return s
}

type docData struct {
	Blueprint    *blueprint.Blueprint
	IncludeUsage bool
}

var markdownTemplate = `# {{ .Blueprint.Metadata.Name }}

{{ if .Blueprint.Metadata.Description }}{{ .Blueprint.Metadata.Description }}{{ end }}

## Overview

| Property | Value |
|----------|-------|
| **Version** | {{ .Blueprint.Metadata.Version }} |
{{ if .Blueprint.Metadata.Maintainers }}| **Maintainers** | {{ range $i, $m := .Blueprint.Metadata.Maintainers }}{{ if $i }}, {{ end }}{{ $m.Name }}{{ end }} |
{{ end }}{{ if .Blueprint.Metadata.License }}| **License** | {{ .Blueprint.Metadata.License }} |
{{ end }}{{ if .Blueprint.Metadata.Categories }}| **Categories** | {{ join .Blueprint.Metadata.Categories ", " }} |
{{ end }}{{ if .Blueprint.Metadata.Keywords }}| **Keywords** | {{ join .Blueprint.Metadata.Keywords ", " }} |
{{ end }}

{{ if .Blueprint.Parameters }}
## Parameters

| Parameter | Type | Required | Default | Description |
|-----------|------|----------|---------|-------------|
{{ range $name, $param := .Blueprint.Parameters }}| ` + "`{{ $name }}`" + ` | {{ $param.Type }} | {{ if $param.Required }}Yes{{ else }}No{{ end }} | {{ if $param.Default }}` + "`{{ $param.Default }}`" + `{{ else }}-{{ end }} | {{ $param.Description }} |
{{ end }}
{{ end }}

{{ if .Blueprint.Dependencies }}
## Dependencies

This blueprint depends on:

{{ range $dep := .Blueprint.Dependencies }}- ` + "`{{ $dep.Name }}`" + ` {{ $dep.Version }}{{ if $dep.Optional }} (optional){{ end }}
{{ end }}
{{ end }}

{{ if .IncludeUsage }}
## Usage

### Basic Installation

` + "```bash" + `
kscorectl blueprint install {{ .Blueprint.Metadata.Name }}
` + "```" + `

### Using in State Files

` + "```yaml" + `
include:
  - blueprint: {{ .Blueprint.Metadata.Name }}@{{ .Blueprint.Metadata.Version }}
    params:
{{ range $name, $param := .Blueprint.Parameters }}{{ if $param.Required }}      {{ $name }}: <value>
{{ end }}{{ end }}
` + "```" + `
{{ end }}

## Support

{{ if .Blueprint.Metadata.Repository }}
- **Repository**: {{ .Blueprint.Metadata.Repository }}
{{ end }}
{{ if .Blueprint.Metadata.Homepage }}
- **Homepage**: {{ .Blueprint.Metadata.Homepage }}
{{ end }}

---
*Documentation generated by kscore-blueprint-publish*
`

var parametersTemplate = `# Parameters Reference

## {{ .Metadata.Name }}

{{ range $name, $param := .Parameters }}
### {{ $name }}

{{ $param.Description }}

| Property | Value |
|----------|-------|
| **Type** | {{ $param.Type }} |
| **Required** | {{ if $param.Required }}Yes{{ else }}No{{ end }} |
{{ if $param.Default }}| **Default** | ` + "`{{ $param.Default }}`" + ` |
{{ end }}{{ if $param.Sensitive }}| **Sensitive** | Yes |
{{ end }}

{{ if $param.Enum }}
**Allowed values:**
{{ range $val := $param.Enum }}
- ` + "`{{ $val }}`" + `
{{ end }}
{{ end }}

{{ if $param.Examples }}
**Examples:**
{{ range $ex := $param.Examples }}
- ` + "`{{ $ex }}`" + `
{{ end }}
{{ end }}

---
{{ end }}
`

var dependenciesTemplate = `# Dependencies

## {{ .Metadata.Name }}

{{ range $dep := .Dependencies }}
### {{ $dep.Name }}

{{ if $dep.Description }}{{ $dep.Description }}{{ end }}

| Property | Value |
|----------|-------|
| **Version** | {{ $dep.Version }} |
| **Optional** | {{ if $dep.Optional }}Yes{{ else }}No{{ end }} |

---
{{ end }}
`

var htmlTemplate = `<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>{{ .Blueprint.Metadata.Name }} - Blueprint Documentation</title>
    <style>
        body {
            font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, Oxygen, Ubuntu, sans-serif;
            line-height: 1.6;
            max-width: 900px;
            margin: 0 auto;
            padding: 20px;
            color: #333;
        }
        h1 { border-bottom: 2px solid #333; padding-bottom: 10px; }
        h2 { border-bottom: 1px solid #ddd; padding-bottom: 5px; margin-top: 30px; }
        code {
            background: #f4f4f4;
            padding: 2px 6px;
            border-radius: 3px;
            font-family: 'Consolas', 'Monaco', monospace;
        }
        pre {
            background: #f4f4f4;
            padding: 15px;
            border-radius: 5px;
            overflow-x: auto;
        }
        table {
            border-collapse: collapse;
            width: 100%;
            margin: 20px 0;
        }
        th, td {
            border: 1px solid #ddd;
            padding: 10px;
            text-align: left;
        }
        th { background: #f4f4f4; }
        .required { color: #d9534f; }
        .optional { color: #5cb85c; }
    </style>
</head>
<body>
    <h1>{{ .Blueprint.Metadata.Name }}</h1>

    {{ if .Blueprint.Metadata.Description }}
    <p>{{ .Blueprint.Metadata.Description }}</p>
    {{ end }}

    <h2>Overview</h2>
    <table>
        <tr><th>Property</th><th>Value</th></tr>
        <tr><td>Version</td><td>{{ .Blueprint.Metadata.Version }}</td></tr>
        {{ if .Blueprint.Metadata.Maintainers }}<tr><td>Maintainers</td><td>{{ range $i, $m := .Blueprint.Metadata.Maintainers }}{{ if $i }}, {{ end }}{{ $m.Name }}{{ end }}</td></tr>{{ end }}
        {{ if .Blueprint.Metadata.License }}<tr><td>License</td><td>{{ .Blueprint.Metadata.License }}</td></tr>{{ end }}
        {{ if .Blueprint.Metadata.Categories }}<tr><td>Categories</td><td>{{ range $i, $c := .Blueprint.Metadata.Categories }}{{ if $i }}, {{ end }}{{ $c }}{{ end }}</td></tr>{{ end }}
    </table>

    {{ if .Blueprint.Parameters }}
    <h2>Parameters</h2>
    <table>
        <tr>
            <th>Parameter</th>
            <th>Type</th>
            <th>Required</th>
            <th>Default</th>
            <th>Description</th>
        </tr>
        {{ range $name, $param := .Blueprint.Parameters }}
        <tr>
            <td><code>{{ $name }}</code></td>
            <td>{{ $param.Type }}</td>
            <td>{{ if $param.Required }}<span class="required">Yes</span>{{ else }}<span class="optional">No</span>{{ end }}</td>
            <td>{{ if $param.Default }}<code>{{ $param.Default }}</code>{{ else }}-{{ end }}</td>
            <td>{{ $param.Description }}</td>
        </tr>
        {{ end }}
    </table>
    {{ end }}

    {{ if .Blueprint.Dependencies }}
    <h2>Dependencies</h2>
    <ul>
        {{ range $dep := .Blueprint.Dependencies }}
        <li><code>{{ $dep.Name }}</code> {{ $dep.Version }}{{ if $dep.Optional }} (optional){{ end }}</li>
        {{ end }}
    </ul>
    {{ end }}

    {{ if .IncludeUsage }}
    <h2>Usage</h2>
    <h3>Basic Installation</h3>
    <pre><code>kscorectl blueprint install {{ .Blueprint.Metadata.Name }}</code></pre>

    <h3>Using in State Files</h3>
    <pre><code>include:
  - blueprint: {{ .Blueprint.Metadata.Name }}@{{ .Blueprint.Metadata.Version }}
    params:{{ range $name, $param := .Blueprint.Parameters }}{{ if $param.Required }}
      {{ $name }}: &lt;value&gt;{{ end }}{{ end }}</code></pre>
    {{ end }}

    <hr>
    <p><em>Documentation generated by kscore-blueprint-publish</em></p>
</body>
</html>
`
