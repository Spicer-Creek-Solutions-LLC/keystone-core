// Package docs provides automatic documentation generation for Keystone modules.
package docs

import (
	"bytes"
	"fmt"
	"go/ast"
	"go/doc"
	"go/parser"
	"go/token"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"text/template" // nosemgrep: go.lang.security.audit.xss.import-text-template.import-text-template -- documentation templates render markdown/text, not HTML responses
	"time"
)

// ModuleInfo contains information about a module for documentation.
type ModuleInfo struct {
	Name        string            `json:"name"`
	Package     string            `json:"package"`
	ImportPath  string            `json:"importPath"`
	Description string            `json:"description"`
	Version     string            `json:"version,omitempty"`
	Author      string            `json:"author,omitempty"`
	License     string            `json:"license,omitempty"`
	Repository  string            `json:"repository,omitempty"`
	Types       []TypeInfo        `json:"types,omitempty"`
	Functions   []FunctionInfo    `json:"functions,omitempty"`
	Constants   []ConstantInfo    `json:"constants,omitempty"`
	Variables   []VariableInfo    `json:"variables,omitempty"`
	Examples    []ExampleInfo     `json:"examples,omitempty"`
	Subpackages []string          `json:"subpackages,omitempty"`
	Metadata    map[string]string `json:"metadata,omitempty"`
}

// TypeInfo contains information about a type.
type TypeInfo struct {
	Name        string       `json:"name"`
	Kind        string       `json:"kind"` // struct, interface, alias
	Description string       `json:"description"`
	Fields      []FieldInfo  `json:"fields,omitempty"`
	Methods     []MethodInfo `json:"methods,omitempty"`
	Implements  []string     `json:"implements,omitempty"`
	Source      string       `json:"source,omitempty"`
}

// FieldInfo contains information about a struct field.
type FieldInfo struct {
	Name        string `json:"name"`
	Type        string `json:"type"`
	Tag         string `json:"tag,omitempty"`
	Description string `json:"description"`
	Required    bool   `json:"required,omitempty"`
	Deprecated  bool   `json:"deprecated,omitempty"`
}

// MethodInfo contains information about a method.
type MethodInfo struct {
	Name        string      `json:"name"`
	Receiver    string      `json:"receiver"`
	Description string      `json:"description"`
	Parameters  []ParamInfo `json:"parameters,omitempty"`
	Returns     []ParamInfo `json:"returns,omitempty"`
	Example     string      `json:"example,omitempty"`
	Deprecated  bool        `json:"deprecated,omitempty"`
}

// FunctionInfo contains information about a function.
type FunctionInfo struct {
	Name        string      `json:"name"`
	Description string      `json:"description"`
	Parameters  []ParamInfo `json:"parameters,omitempty"`
	Returns     []ParamInfo `json:"returns,omitempty"`
	Example     string      `json:"example,omitempty"`
	Deprecated  bool        `json:"deprecated,omitempty"`
}

// ParamInfo contains information about a parameter.
type ParamInfo struct {
	Name string `json:"name"`
	Type string `json:"type"`
}

// ConstantInfo contains information about a constant.
type ConstantInfo struct {
	Name        string `json:"name"`
	Type        string `json:"type,omitempty"`
	Value       string `json:"value"`
	Description string `json:"description"`
}

// VariableInfo contains information about a variable.
type VariableInfo struct {
	Name        string `json:"name"`
	Type        string `json:"type"`
	Description string `json:"description"`
}

// ExampleInfo contains information about an example.
type ExampleInfo struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Code        string `json:"code"`
	Output      string `json:"output,omitempty"`
}

// OutputFormat represents the output format for documentation.
type OutputFormat string

const (
	// FormatMarkdown generates Markdown documentation.
	FormatMarkdown OutputFormat = "markdown"
	// FormatHTML generates HTML documentation.
	FormatHTML OutputFormat = "html"
	// FormatJSON generates JSON documentation.
	FormatJSON OutputFormat = "json"
)

// GeneratorConfig configures the documentation generator.
type GeneratorConfig struct {
	// Title for the documentation
	Title string

	// Output format
	Format OutputFormat

	// Include private types/functions
	IncludePrivate bool

	// Include source code
	IncludeSource bool

	// Include examples
	IncludeExamples bool

	// Custom templates
	Templates map[string]string

	// Metadata to include
	Metadata map[string]string

	// BaseURL for links
	BaseURL string
}

// Generator generates documentation for Go modules.
type Generator struct {
	config *GeneratorConfig
	fset   *token.FileSet
}

// NewGenerator creates a new documentation generator.
func NewGenerator(config *GeneratorConfig) *Generator {
	if config == nil {
		config = &GeneratorConfig{
			Format:          FormatMarkdown,
			IncludeExamples: true,
		}
	}
	return &Generator{
		config: config,
		fset:   token.NewFileSet(),
	}
}

// ParsePackage parses a Go package and returns module info.
func (g *Generator) ParsePackage(dir string) (*ModuleInfo, error) {
	pkgs, err := parser.ParseDir(g.fset, dir, nil, parser.ParseComments)
	if err != nil {
		return nil, fmt.Errorf("failed to parse package: %w", err)
	}

	for name, pkg := range pkgs {
		if strings.HasSuffix(name, "_test") {
			continue
		}

		// Build AST package
		astPkg := &ast.Package{ //nolint:staticcheck // SA1019: ast.Package is deprecated but requires major refactoring to use go/types
			Name:  name,
			Files: pkg.Files,
		}

		// Use go/doc to extract documentation
		docPkg := doc.New(astPkg, dir, doc.AllDecls)

		info := &ModuleInfo{
			Name:        name,
			Package:     name,
			ImportPath:  dir,
			Description: docPkg.Doc,
			Metadata:    g.config.Metadata,
		}

		// Parse types
		for _, t := range docPkg.Types {
			if !g.config.IncludePrivate && !ast.IsExported(t.Name) {
				continue
			}
			typeInfo := g.parseType(t)
			info.Types = append(info.Types, typeInfo)
		}

		// Parse functions
		for _, f := range docPkg.Funcs {
			if !g.config.IncludePrivate && !ast.IsExported(f.Name) {
				continue
			}
			funcInfo := g.parseFunc(f)
			info.Functions = append(info.Functions, funcInfo)
		}

		// Parse constants
		for _, c := range docPkg.Consts {
			for _, name := range c.Names {
				if !g.config.IncludePrivate && !ast.IsExported(name) {
					continue
				}
				info.Constants = append(info.Constants, ConstantInfo{
					Name:        name,
					Description: c.Doc,
				})
			}
		}

		// Parse variables
		for _, v := range docPkg.Vars {
			for _, name := range v.Names {
				if !g.config.IncludePrivate && !ast.IsExported(name) {
					continue
				}
				info.Variables = append(info.Variables, VariableInfo{
					Name:        name,
					Description: v.Doc,
				})
			}
		}

		return info, nil
	}

	return nil, fmt.Errorf("no package found in %s", dir)
}

func (g *Generator) parseType(t *doc.Type) TypeInfo {
	info := TypeInfo{
		Name:        t.Name,
		Description: t.Doc,
	}

	// Determine kind and parse fields for structs
	if t.Decl != nil && len(t.Decl.Specs) > 0 {
		if spec, ok := t.Decl.Specs[0].(*ast.TypeSpec); ok {
			switch typ := spec.Type.(type) {
			case *ast.StructType:
				info.Kind = "struct"
				info.Fields = g.parseFields(typ.Fields)
			case *ast.InterfaceType:
				info.Kind = "interface"
				info.Methods = g.parseInterfaceMethods(typ.Methods)
			default:
				info.Kind = "alias"
			}
		}
	}

	// Parse methods
	for _, m := range t.Methods {
		if !g.config.IncludePrivate && !ast.IsExported(m.Name) {
			continue
		}
		method := g.parseMethod(m, t.Name)
		info.Methods = append(info.Methods, method)
	}

	return info
}

func (g *Generator) parseFields(fields *ast.FieldList) []FieldInfo {
	if fields == nil {
		return nil
	}

	var result []FieldInfo
	for _, f := range fields.List {
		// Get field names
		var names []string
		for _, name := range f.Names {
			if !g.config.IncludePrivate && !ast.IsExported(name.Name) {
				continue
			}
			names = append(names, name.Name)
		}

		if len(names) == 0 {
			continue
		}

		// Get type as string
		typeStr := g.typeString(f.Type)

		// Get tag if present
		var tag string
		if f.Tag != nil {
			tag = f.Tag.Value
		}

		// Get description from comment
		var desc string
		if f.Doc != nil {
			desc = f.Doc.Text()
		} else if f.Comment != nil {
			desc = f.Comment.Text()
		}

		for _, name := range names {
			fi := FieldInfo{
				Name:        name,
				Type:        typeStr,
				Tag:         tag,
				Description: strings.TrimSpace(desc),
				Required:    !strings.Contains(tag, "omitempty"),
				Deprecated:  strings.Contains(strings.ToLower(desc), "deprecated"),
			}
			result = append(result, fi)
		}
	}

	return result
}

func (g *Generator) parseInterfaceMethods(methods *ast.FieldList) []MethodInfo {
	if methods == nil {
		return nil
	}

	var result []MethodInfo
	for _, m := range methods.List {
		if len(m.Names) == 0 {
			continue
		}

		name := m.Names[0].Name
		if !g.config.IncludePrivate && !ast.IsExported(name) {
			continue
		}

		mi := MethodInfo{
			Name: name,
		}

		if m.Doc != nil {
			mi.Description = m.Doc.Text()
		}

		if funcType, ok := m.Type.(*ast.FuncType); ok {
			mi.Parameters = g.parseParams(funcType.Params)
			mi.Returns = g.parseParams(funcType.Results)
		}

		result = append(result, mi)
	}

	return result
}

func (g *Generator) parseFunc(f *doc.Func) FunctionInfo {
	info := FunctionInfo{
		Name:        f.Name,
		Description: f.Doc,
		Deprecated:  strings.Contains(strings.ToLower(f.Doc), "deprecated"),
	}

	if f.Decl != nil && f.Decl.Type != nil {
		info.Parameters = g.parseParams(f.Decl.Type.Params)
		info.Returns = g.parseParams(f.Decl.Type.Results)
	}

	return info
}

func (g *Generator) parseMethod(m *doc.Func, receiver string) MethodInfo {
	info := MethodInfo{
		Name:        m.Name,
		Receiver:    receiver,
		Description: m.Doc,
		Deprecated:  strings.Contains(strings.ToLower(m.Doc), "deprecated"),
	}

	if m.Decl != nil && m.Decl.Type != nil {
		info.Parameters = g.parseParams(m.Decl.Type.Params)
		info.Returns = g.parseParams(m.Decl.Type.Results)
	}

	return info
}

func (g *Generator) parseParams(fields *ast.FieldList) []ParamInfo {
	if fields == nil {
		return nil
	}

	var result []ParamInfo
	for _, f := range fields.List {
		typeStr := g.typeString(f.Type)

		if len(f.Names) == 0 {
			result = append(result, ParamInfo{Type: typeStr})
		} else {
			for _, name := range f.Names {
				result = append(result, ParamInfo{
					Name: name.Name,
					Type: typeStr,
				})
			}
		}
	}

	return result
}

func (g *Generator) typeString(expr ast.Expr) string {
	switch t := expr.(type) {
	case *ast.Ident:
		return t.Name
	case *ast.StarExpr:
		return "*" + g.typeString(t.X)
	case *ast.ArrayType:
		if t.Len == nil {
			return "[]" + g.typeString(t.Elt)
		}
		return fmt.Sprintf("[%v]%s", t.Len, g.typeString(t.Elt))
	case *ast.MapType:
		return fmt.Sprintf("map[%s]%s", g.typeString(t.Key), g.typeString(t.Value))
	case *ast.SelectorExpr:
		return fmt.Sprintf("%s.%s", g.typeString(t.X), t.Sel.Name)
	case *ast.InterfaceType:
		return "interface{}"
	case *ast.FuncType:
		return "func(...)"
	case *ast.ChanType:
		switch t.Dir {
		case ast.SEND:
			return "chan<- " + g.typeString(t.Value)
		case ast.RECV:
			return "<-chan " + g.typeString(t.Value)
		default:
			return "chan " + g.typeString(t.Value)
		}
	case *ast.Ellipsis:
		return "..." + g.typeString(t.Elt)
	default:
		return "unknown"
	}
}

// Generate generates documentation and writes it to the writer.
func (g *Generator) Generate(info *ModuleInfo, w io.Writer) error {
	switch g.config.Format {
	case FormatMarkdown:
		return g.generateMarkdown(info, w)
	case FormatHTML:
		return g.generateHTML(info, w)
	case FormatJSON:
		return g.generateJSON(info, w)
	default:
		return fmt.Errorf("unsupported format: %s", g.config.Format)
	}
}

func (g *Generator) generateMarkdown(info *ModuleInfo, w io.Writer) error {
	tmpl := `# {{ .Name }}

{{ if .Description }}{{ .Description }}{{ end }}

{{ if .ImportPath }}**Import:** ` + "`{{ .ImportPath }}`" + `{{ end }}

{{ if .Types }}
## Types

{{ range .Types }}
### {{ .Name }}

{{ if .Description }}{{ .Description }}{{ end }}

{{ if .Fields }}
**Fields:**

| Name | Type | Description |
|------|------|-------------|
{{ range .Fields }}| {{ .Name }} | ` + "`{{ .Type }}`" + ` | {{ .Description }} |
{{ end }}
{{ end }}

{{ if .Methods }}
**Methods:**

{{ range .Methods }}
#### {{ .Name }}

{{ if .Description }}{{ .Description }}{{ end }}

{{ if .Parameters }}
**Parameters:**
{{ range .Parameters }}- ` + "`{{ .Name }}`" + ` ({{ .Type }})
{{ end }}{{ end }}

{{ if .Returns }}
**Returns:**
{{ range .Returns }}- {{ .Type }}
{{ end }}{{ end }}

{{ end }}
{{ end }}

{{ end }}
{{ end }}

{{ if .Functions }}
## Functions

{{ range .Functions }}
### {{ .Name }}

{{ if .Description }}{{ .Description }}{{ end }}

{{ if .Parameters }}
**Parameters:**
{{ range .Parameters }}- ` + "`{{ .Name }}`" + ` ({{ .Type }})
{{ end }}{{ end }}

{{ if .Returns }}
**Returns:**
{{ range .Returns }}- {{ .Type }}
{{ end }}{{ end }}

{{ end }}
{{ end }}

{{ if .Constants }}
## Constants

{{ range .Constants }}
### {{ .Name }}

{{ if .Description }}{{ .Description }}{{ end }}
{{ end }}
{{ end }}

---
*Generated on {{ now }}*
`

	funcMap := template.FuncMap{
		"now": func() string {
			return time.Now().Format("2006-01-02 15:04:05")
		},
	}

	t, err := template.New("markdown").Funcs(funcMap).Parse(tmpl)
	if err != nil {
		return fmt.Errorf("failed to parse template: %w", err)
	}

	return t.Execute(w, info)
}

func (g *Generator) generateHTML(info *ModuleInfo, w io.Writer) error {
	tmpl := `<!DOCTYPE html>
<html>
<head>
    <meta charset="UTF-8">
    <title>{{ .Name }} - Documentation</title>
    <style>
        body { font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, Oxygen, Ubuntu, sans-serif; max-width: 900px; margin: 0 auto; padding: 20px; }
        h1 { border-bottom: 2px solid #333; padding-bottom: 10px; }
        h2 { border-bottom: 1px solid #ccc; padding-bottom: 5px; margin-top: 30px; }
        h3 { color: #0366d6; }
        code { background: #f6f8fa; padding: 2px 6px; border-radius: 3px; }
        pre { background: #f6f8fa; padding: 16px; overflow-x: auto; border-radius: 6px; }
        table { border-collapse: collapse; width: 100%; margin: 20px 0; }
        th, td { border: 1px solid #ddd; padding: 8px 12px; text-align: left; }
        th { background: #f6f8fa; }
        .type-header { display: flex; align-items: center; gap: 10px; }
        .kind-badge { font-size: 12px; padding: 2px 8px; border-radius: 12px; background: #e1e4e8; }
        .deprecated { color: #cb2431; text-decoration: line-through; }
        footer { margin-top: 50px; padding-top: 20px; border-top: 1px solid #ccc; color: #666; font-size: 14px; }
    </style>
</head>
<body>
    <h1>{{ .Name }}</h1>
    {{ if .Description }}<p>{{ .Description }}</p>{{ end }}
    {{ if .ImportPath }}<p><strong>Import:</strong> <code>{{ .ImportPath }}</code></p>{{ end }}

    {{ if .Types }}
    <h2>Types</h2>
    {{ range .Types }}
    <div class="type">
        <div class="type-header">
            <h3>{{ .Name }}</h3>
            {{ if .Kind }}<span class="kind-badge">{{ .Kind }}</span>{{ end }}
        </div>
        {{ if .Description }}<p>{{ .Description }}</p>{{ end }}

        {{ if .Fields }}
        <h4>Fields</h4>
        <table>
            <thead><tr><th>Name</th><th>Type</th><th>Description</th></tr></thead>
            <tbody>
            {{ range .Fields }}
            <tr{{ if .Deprecated }} class="deprecated"{{ end }}>
                <td><code>{{ .Name }}</code></td>
                <td><code>{{ .Type }}</code></td>
                <td>{{ .Description }}</td>
            </tr>
            {{ end }}
            </tbody>
        </table>
        {{ end }}

        {{ if .Methods }}
        <h4>Methods</h4>
        {{ range .Methods }}
        <div class="method">
            <h5>{{ .Name }}</h5>
            {{ if .Description }}<p>{{ .Description }}</p>{{ end }}
        </div>
        {{ end }}
        {{ end }}
    </div>
    {{ end }}
    {{ end }}

    {{ if .Functions }}
    <h2>Functions</h2>
    {{ range .Functions }}
    <div class="function">
        <h3{{ if .Deprecated }} class="deprecated"{{ end }}>{{ .Name }}</h3>
        {{ if .Description }}<p>{{ .Description }}</p>{{ end }}
    </div>
    {{ end }}
    {{ end }}

    <footer>Generated on {{ now }}</footer>
</body>
</html>
`

	funcMap := template.FuncMap{
		"now": func() string {
			return time.Now().Format("2006-01-02 15:04:05")
		},
	}

	t, err := template.New("html").Funcs(funcMap).Parse(tmpl)
	if err != nil {
		return fmt.Errorf("failed to parse template: %w", err)
	}

	return t.Execute(w, info)
}

func (g *Generator) generateJSON(info *ModuleInfo, w io.Writer) error {
	// Simple JSON generation without external packages
	var buf bytes.Buffer
	buf.WriteString("{\n")
	buf.WriteString(fmt.Sprintf("  \"name\": %q,\n", info.Name))
	buf.WriteString(fmt.Sprintf("  \"package\": %q,\n", info.Package))
	buf.WriteString(fmt.Sprintf("  \"importPath\": %q,\n", info.ImportPath))
	buf.WriteString(fmt.Sprintf("  \"description\": %q,\n", escapeJSON(info.Description)))
	buf.WriteString(fmt.Sprintf("  \"typesCount\": %d,\n", len(info.Types)))
	buf.WriteString(fmt.Sprintf("  \"functionsCount\": %d,\n", len(info.Functions)))
	buf.WriteString(fmt.Sprintf("  \"generated\": %q\n", time.Now().Format(time.RFC3339)))
	buf.WriteString("}")
	_, err := w.Write(buf.Bytes())
	return err
}

func escapeJSON(s string) string {
	s = strings.ReplaceAll(s, "\\", "\\\\")
	s = strings.ReplaceAll(s, "\"", "\\\"")
	s = strings.ReplaceAll(s, "\n", "\\n")
	s = strings.ReplaceAll(s, "\r", "\\r")
	s = strings.ReplaceAll(s, "\t", "\\t")
	return s
}

// ModuleDoc represents generated documentation.
type ModuleDoc struct {
	Info      *ModuleInfo
	Content   string
	Format    OutputFormat
	Generated time.Time
}

// GenerateString generates documentation as a string.
func (g *Generator) GenerateString(info *ModuleInfo) (string, error) {
	var buf bytes.Buffer
	if err := g.Generate(info, &buf); err != nil {
		return "", err
	}
	return buf.String(), nil
}

// DocIndex represents an index of all module documentation.
type DocIndex struct {
	Title       string     `json:"title"`
	Description string     `json:"description"`
	Modules     []DocEntry `json:"modules"`
	Generated   time.Time  `json:"generated"`
}

// DocEntry represents an entry in the documentation index.
type DocEntry struct {
	Name        string   `json:"name"`
	Package     string   `json:"package"`
	Path        string   `json:"path"`
	Description string   `json:"description"`
	Types       int      `json:"types"`
	Functions   int      `json:"functions"`
	Keywords    []string `json:"keywords,omitempty"`
}

// IndexGenerator generates documentation indexes.
type IndexGenerator struct {
	config *GeneratorConfig
}

// NewIndexGenerator creates a new index generator.
func NewIndexGenerator(config *GeneratorConfig) *IndexGenerator {
	return &IndexGenerator{config: config}
}

// GenerateIndex generates an index from multiple modules.
func (ig *IndexGenerator) GenerateIndex(modules []*ModuleInfo) *DocIndex {
	index := &DocIndex{
		Title:     ig.config.Title,
		Generated: time.Now(),
	}

	for _, mod := range modules {
		entry := DocEntry{
			Name:        mod.Name,
			Package:     mod.Package,
			Path:        mod.ImportPath,
			Description: truncateDescription(mod.Description, 200),
			Types:       len(mod.Types),
			Functions:   len(mod.Functions),
			Keywords:    extractKeywords(mod),
		}
		index.Modules = append(index.Modules, entry)
	}

	// Sort by name
	sort.Slice(index.Modules, func(i, j int) bool {
		return index.Modules[i].Name < index.Modules[j].Name
	})

	return index
}

func truncateDescription(s string, maxLen int) string {
	s = strings.TrimSpace(s)
	// Take first sentence or paragraph
	if idx := strings.Index(s, "\n\n"); idx > 0 && idx < maxLen {
		s = s[:idx]
	}
	if idx := strings.Index(s, ". "); idx > 0 && idx < maxLen {
		s = s[:idx+1]
	}
	if len(s) > maxLen {
		s = s[:maxLen-3] + "..."
	}
	return strings.TrimSpace(s)
}

func extractKeywords(mod *ModuleInfo) []string {
	keywords := make(map[string]bool)

	// Add type names as keywords
	for i := range mod.Types {
		keywords[strings.ToLower(mod.Types[i].Name)] = true
	}

	// Add function names as keywords
	for _, f := range mod.Functions {
		keywords[strings.ToLower(f.Name)] = true
	}

	// Extract keywords from description
	if mod.Description != "" {
		words := regexp.MustCompile(`\b[a-zA-Z]{3,}\b`).FindAllString(mod.Description, -1)
		for _, w := range words {
			if len(w) >= 4 && len(w) <= 20 {
				keywords[strings.ToLower(w)] = true
			}
		}
	}

	result := make([]string, 0, len(keywords))
	for k := range keywords {
		result = append(result, k)
	}
	sort.Strings(result)

	// Limit keywords
	if len(result) > 20 {
		result = result[:20]
	}

	return result
}

// GenerateIndexMarkdown generates a markdown index.
func (ig *IndexGenerator) GenerateIndexMarkdown(index *DocIndex, w io.Writer) error {
	tmpl := `# {{ .Title }}

{{ if .Description }}{{ .Description }}{{ end }}

## Modules

| Module | Description | Types | Functions |
|--------|-------------|-------|-----------|
{{ range .Modules }}| [{{ .Name }}]({{ .Path }}) | {{ .Description }} | {{ .Types }} | {{ .Functions }} |
{{ end }}

---
*Generated on {{ .Generated.Format "2006-01-02 15:04:05" }}*
`

	t, err := template.New("index").Parse(tmpl)
	if err != nil {
		return err
	}

	return t.Execute(w, index)
}

// APIDoc represents API documentation.
type APIDoc struct {
	Endpoints []EndpointDoc `json:"endpoints"`
}

// EndpointDoc documents an API endpoint.
type EndpointDoc struct {
	Method      string        `json:"method"`
	Path        string        `json:"path"`
	Description string        `json:"description"`
	Parameters  []APIParam    `json:"parameters,omitempty"`
	RequestBody *APIBody      `json:"requestBody,omitempty"`
	Responses   []APIResponse `json:"responses,omitempty"`
	Tags        []string      `json:"tags,omitempty"`
	Deprecated  bool          `json:"deprecated,omitempty"`
}

// APIParam documents an API parameter.
type APIParam struct {
	Name        string `json:"name"`
	In          string `json:"in"` // path, query, header
	Type        string `json:"type"`
	Description string `json:"description"`
	Required    bool   `json:"required"`
}

// APIBody documents a request body.
type APIBody struct {
	ContentType string `json:"contentType"`
	Schema      string `json:"schema"`
	Description string `json:"description"`
	Example     string `json:"example,omitempty"`
}

// APIResponse documents an API response.
type APIResponse struct {
	StatusCode  int    `json:"statusCode"`
	Description string `json:"description"`
	ContentType string `json:"contentType,omitempty"`
	Schema      string `json:"schema,omitempty"`
}

// BatchGenerator generates documentation for multiple packages.
type BatchGenerator struct {
	generator *Generator
}

// NewBatchGenerator creates a new batch generator.
func NewBatchGenerator(config *GeneratorConfig) *BatchGenerator {
	return &BatchGenerator{
		generator: NewGenerator(config),
	}
}

// GenerateForDirectory generates documentation for all packages in a directory tree.
func (bg *BatchGenerator) GenerateForDirectory(rootDir string) ([]*ModuleDoc, error) {
	var docs []*ModuleDoc

	// Find all Go packages
	packages, err := findGoPackages(rootDir)
	if err != nil {
		return nil, err
	}

	for _, pkg := range packages {
		info, err := bg.generator.ParsePackage(pkg)
		if err != nil {
			continue // Skip packages that can't be parsed
		}

		content, err := bg.generator.GenerateString(info)
		if err != nil {
			continue
		}

		docs = append(docs, &ModuleDoc{
			Info:      info,
			Content:   content,
			Format:    bg.generator.config.Format,
			Generated: time.Now(),
		})
	}

	return docs, nil
}

func findGoPackages(rootDir string) ([]string, error) {
	var packages []string

	err := filepath.Walk(rootDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil //nolint:nilerr // skip errors in walk to continue
		}
		// Check if path ends with .go
		if strings.HasSuffix(path, ".go") {
			dir := filepath.Dir(path)
			// Check if we already have this directory
			found := false
			for _, p := range packages {
				if p == dir {
					found = true
					break
				}
			}
			if !found {
				packages = append(packages, dir)
			}
		}
		return nil
	})

	return packages, err
}
