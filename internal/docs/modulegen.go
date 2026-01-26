// Package docs provides documentation generation utilities
package docs

import (
	"bytes"
	"fmt"
	"go/ast"
	"go/doc"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"text/template"
)

// ModuleDocGenerator generates documentation for Keystone modules from Go source code
type ModuleDocGenerator struct {
	// OutputDir is the output directory for generated docs
	OutputDir string

	// TemplateDir is the directory containing custom templates
	TemplateDir string

	// Format is the output format (markdown, html, json)
	Format string

	// IncludePrivate includes private (unexported) functions
	IncludePrivate bool

	// Verbose enables verbose output
	Verbose bool
}

// ModuleRefDoc represents reference documentation for a module
type ModuleRefDoc struct {
	// Name is the module name
	Name string `json:"name"`

	// Package is the Go package name
	Package string `json:"package"`

	// Description is the module description
	Description string `json:"description"`

	// Version is the module version (from annotations)
	Version string `json:"version,omitempty"`

	// Author is the module author
	Author string `json:"author,omitempty"`

	// Since indicates when the module was added
	Since string `json:"since,omitempty"`

	// Deprecated indicates if the module is deprecated
	Deprecated string `json:"deprecated,omitempty"`

	// Types are the exported types
	Types []TypeDoc `json:"types,omitempty"`

	// Functions are the exported functions
	Functions []FunctionDoc `json:"functions,omitempty"`

	// Examples are code examples
	Examples []ExampleDoc `json:"examples,omitempty"`

	// See also references
	SeeAlso []string `json:"see_also,omitempty"`
}

// TypeDoc represents documentation for a type
type TypeDoc struct {
	// Name is the type name
	Name string `json:"name"`

	// Description is the type description
	Description string `json:"description"`

	// Fields are struct fields (if struct)
	Fields []FieldDoc `json:"fields,omitempty"`

	// Methods are the type's methods
	Methods []FunctionDoc `json:"methods,omitempty"`

	// Since indicates when this type was added
	Since string `json:"since,omitempty"`

	// Deprecated indicates if the type is deprecated
	Deprecated string `json:"deprecated,omitempty"`
}

// FieldDoc represents documentation for a struct field
type FieldDoc struct {
	// Name is the field name
	Name string `json:"name"`

	// Type is the field type
	Type string `json:"type"`

	// Description is the field description
	Description string `json:"description"`

	// Required indicates if the field is required
	Required bool `json:"required,omitempty"`

	// Default is the default value
	Default string `json:"default,omitempty"`

	// Tags are the struct tags
	Tags map[string]string `json:"tags,omitempty"`
}

// FunctionDoc represents documentation for a function or method
type FunctionDoc struct {
	// Name is the function name
	Name string `json:"name"`

	// Signature is the function signature
	Signature string `json:"signature"`

	// Description is the function description
	Description string `json:"description"`

	// Parameters are the function parameters
	Parameters []ParamDoc `json:"parameters,omitempty"`

	// Returns are the return values
	Returns []ParamDoc `json:"returns,omitempty"`

	// Errors lists possible errors
	Errors []string `json:"errors,omitempty"`

	// Example is an example usage
	Example string `json:"example,omitempty"`

	// Since indicates when this function was added
	Since string `json:"since,omitempty"`

	// Deprecated indicates if the function is deprecated
	Deprecated string `json:"deprecated,omitempty"`
}

// ParamDoc represents documentation for a parameter
type ParamDoc struct {
	// Name is the parameter name
	Name string `json:"name"`

	// Type is the parameter type
	Type string `json:"type"`

	// Description is the parameter description
	Description string `json:"description"`

	// Optional indicates if the parameter is optional
	Optional bool `json:"optional,omitempty"`

	// Default is the default value
	Default string `json:"default,omitempty"`
}

// ExampleDoc represents an example
type ExampleDoc struct {
	// Name is the example name
	Name string `json:"name"`

	// Description is the example description
	Description string `json:"description"`

	// Code is the example code
	Code string `json:"code"`

	// Output is the expected output
	Output string `json:"output,omitempty"`
}

// NewModuleRefDocGenerator creates a new generator with default settings
func NewModuleRefDocGenerator() *ModuleDocGenerator {
	return &ModuleDocGenerator{
		OutputDir: "docs/modules",
		Format:    "markdown",
	}
}

// GenerateFromPackage generates documentation from a Go package
func (g *ModuleDocGenerator) GenerateFromPackage(pkgPath string) (*ModuleRefDoc, error) {
	fset := token.NewFileSet()

	// Parse the package
	pkgs, err := parser.ParseDir(fset, pkgPath, nil, parser.ParseComments)
	if err != nil {
		return nil, fmt.Errorf("failed to parse package: %w", err)
	}

	if len(pkgs) == 0 {
		return nil, fmt.Errorf("no packages found in %s", pkgPath)
	}

	// Get the first non-test package
	var pkg *ast.Package
	for name, p := range pkgs {
		if !strings.HasSuffix(name, "_test") {
			pkg = p
			break
		}
	}
	if pkg == nil {
		return nil, fmt.Errorf("no non-test package found")
	}

	// Create doc.Package for documentation
	docPkg := doc.New(pkg, pkgPath, doc.AllDecls)

	modDoc := &ModuleRefDoc{
		Name:        docPkg.Name,
		Package:     pkgPath,
		Description: cleanDoc(docPkg.Doc),
	}

	// Extract annotations from package doc
	modDoc.Version = extractAnnotation(docPkg.Doc, "version")
	modDoc.Author = extractAnnotation(docPkg.Doc, "author")
	modDoc.Since = extractAnnotation(docPkg.Doc, "since")
	modDoc.Deprecated = extractAnnotation(docPkg.Doc, "deprecated")

	// Process types
	for _, t := range docPkg.Types {
		if !g.IncludePrivate && !ast.IsExported(t.Name) {
			continue
		}

		typeDoc := TypeDoc{
			Name:        t.Name,
			Description: cleanDoc(t.Doc),
			Since:       extractAnnotation(t.Doc, "since"),
			Deprecated:  extractAnnotation(t.Doc, "deprecated"),
		}

		// Extract struct fields
		if t.Decl != nil {
			for _, spec := range t.Decl.Specs {
				if ts, ok := spec.(*ast.TypeSpec); ok {
					if st, ok := ts.Type.(*ast.StructType); ok {
						typeDoc.Fields = extractFields(st, fset)
					}
				}
			}
		}

		// Extract methods
		for _, m := range t.Methods {
			if !g.IncludePrivate && !ast.IsExported(m.Name) {
				continue
			}
			typeDoc.Methods = append(typeDoc.Methods, extractFunctionDoc(m))
		}

		modDoc.Types = append(modDoc.Types, typeDoc)
	}

	// Process functions
	for _, f := range docPkg.Funcs {
		if !g.IncludePrivate && !ast.IsExported(f.Name) {
			continue
		}
		modDoc.Functions = append(modDoc.Functions, extractFunctionDoc(f))
	}

	// Sort for consistent output
	sort.Slice(modDoc.Types, func(i, j int) bool {
		return modDoc.Types[i].Name < modDoc.Types[j].Name
	})
	sort.Slice(modDoc.Functions, func(i, j int) bool {
		return modDoc.Functions[i].Name < modDoc.Functions[j].Name
	})

	return modDoc, nil
}

// GenerateFromDirectory generates documentation for all packages in a directory
func (g *ModuleDocGenerator) GenerateFromDirectory(dir string) ([]*ModuleRefDoc, error) {
	var docs []*ModuleRefDoc

	err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() {
			return nil
		}

		// Check if directory contains Go files
		matches, _ := filepath.Glob(filepath.Join(path, "*.go"))
		if len(matches) == 0 {
			return nil
		}

		// Skip test directories
		if strings.HasSuffix(path, "_test") {
			return nil
		}

		modDoc, err := g.GenerateFromPackage(path)
		if err != nil {
			if g.Verbose {
				fmt.Printf("Warning: skipping %s: %v\n", path, err)
			}
			return nil
		}

		docs = append(docs, modDoc)
		return nil
	})

	return docs, err
}

// WriteMarkdown writes documentation as Markdown
func (g *ModuleDocGenerator) WriteMarkdown(modDoc *ModuleRefDoc, w *bytes.Buffer) error {
	tmpl := `# {{ .Name }}

{{ .Description }}

{{ if .Version }}**Version:** {{ .Version }}{{ end }}
{{ if .Author }}**Author:** {{ .Author }}{{ end }}
{{ if .Since }}**Since:** {{ .Since }}{{ end }}
{{ if .Deprecated }}
> **Deprecated:** {{ .Deprecated }}
{{ end }}

{{ if .Types }}
## Types

{{ range .Types }}
### {{ .Name }}

{{ .Description }}

{{ if .Deprecated }}> **Deprecated:** {{ .Deprecated }}{{ end }}

{{ if .Fields }}
#### Fields

| Name | Type | Description |
|------|------|-------------|
{{ range .Fields }}| {{ .Name }} | ` + "`{{ .Type }}`" + ` | {{ .Description }} |
{{ end }}
{{ end }}

{{ if .Methods }}
#### Methods

{{ range .Methods }}
##### {{ .Name }}

` + "```go" + `
{{ .Signature }}
` + "```" + `

{{ .Description }}

{{ if .Parameters }}
**Parameters:**
{{ range .Parameters }}
- ` + "`{{ .Name }}`" + ` ({{ .Type }}): {{ .Description }}{{ if .Optional }} *(optional)*{{ end }}{{ if .Default }} Default: ` + "`{{ .Default }}`" + `{{ end }}
{{ end }}
{{ end }}

{{ if .Returns }}
**Returns:**
{{ range .Returns }}
- ` + "`{{ .Type }}`" + `: {{ .Description }}
{{ end }}
{{ end }}

{{ if .Errors }}
**Errors:**
{{ range .Errors }}
- {{ . }}
{{ end }}
{{ end }}

{{ if .Example }}
**Example:**
` + "```go" + `
{{ .Example }}
` + "```" + `
{{ end }}

{{ end }}
{{ end }}

{{ end }}
{{ end }}

{{ if .Functions }}
## Functions

{{ range .Functions }}
### {{ .Name }}

` + "```go" + `
{{ .Signature }}
` + "```" + `

{{ .Description }}

{{ if .Deprecated }}> **Deprecated:** {{ .Deprecated }}{{ end }}

{{ if .Parameters }}
**Parameters:**
{{ range .Parameters }}
- ` + "`{{ .Name }}`" + ` ({{ .Type }}): {{ .Description }}{{ if .Optional }} *(optional)*{{ end }}{{ if .Default }} Default: ` + "`{{ .Default }}`" + `{{ end }}
{{ end }}
{{ end }}

{{ if .Returns }}
**Returns:**
{{ range .Returns }}
- ` + "`{{ .Type }}`" + `: {{ .Description }}
{{ end }}
{{ end }}

{{ if .Errors }}
**Errors:**
{{ range .Errors }}
- {{ . }}
{{ end }}
{{ end }}

{{ if .Example }}
**Example:**
` + "```go" + `
{{ .Example }}
` + "```" + `
{{ end }}

{{ end }}
{{ end }}

{{ if .Examples }}
## Examples

{{ range .Examples }}
### {{ .Name }}

{{ .Description }}

` + "```go" + `
{{ .Code }}
` + "```" + `

{{ if .Output }}
**Output:**
` + "```" + `
{{ .Output }}
` + "```" + `
{{ end }}

{{ end }}
{{ end }}

{{ if .SeeAlso }}
## See Also

{{ range .SeeAlso }}
- {{ . }}
{{ end }}
{{ end }}
`

	t, err := template.New("markdown").Parse(tmpl)
	if err != nil {
		return fmt.Errorf("failed to parse template: %w", err)
	}

	return t.Execute(w, modDoc)
}

// WriteToFile writes documentation to a file
func (g *ModuleDocGenerator) WriteToFile(modDoc *ModuleRefDoc) error {
	if err := os.MkdirAll(g.OutputDir, 0755); err != nil {
		return fmt.Errorf("failed to create output directory: %w", err)
	}

	var buf bytes.Buffer
	var filename string

	switch g.Format {
	case "markdown", "md":
		if err := g.WriteMarkdown(modDoc, &buf); err != nil {
			return err
		}
		filename = modDoc.Name + ".md"
	default:
		return fmt.Errorf("unsupported format: %s", g.Format)
	}

	outputPath := filepath.Join(g.OutputDir, filename)
	if err := os.WriteFile(outputPath, buf.Bytes(), 0644); err != nil {
		return fmt.Errorf("failed to write file: %w", err)
	}

	if g.Verbose {
		fmt.Printf("Generated: %s\n", outputPath)
	}

	return nil
}

// Helper functions

func extractFunctionDoc(f *doc.Func) FunctionDoc {
	funcDoc := FunctionDoc{
		Name:        f.Name,
		Description: cleanDoc(f.Doc),
		Since:       extractAnnotation(f.Doc, "since"),
		Deprecated:  extractAnnotation(f.Doc, "deprecated"),
	}

	// Build signature
	if f.Decl != nil {
		var sig bytes.Buffer
		sig.WriteString("func ")
		if f.Recv != "" {
			sig.WriteString("(" + f.Recv + ") ")
		}
		sig.WriteString(f.Name)
		if f.Decl.Type != nil {
			sig.WriteString(formatFuncType(f.Decl.Type))
		}
		funcDoc.Signature = sig.String()

		// Extract parameters
		if f.Decl.Type.Params != nil {
			funcDoc.Parameters = extractParams(f.Decl.Type.Params, f.Doc)
		}

		// Extract returns
		if f.Decl.Type.Results != nil {
			funcDoc.Returns = extractReturns(f.Decl.Type.Results, f.Doc)
		}
	}

	// Extract errors from doc
	funcDoc.Errors = extractErrors(f.Doc)

	// Extract example from doc
	funcDoc.Example = extractAnnotation(f.Doc, "example")

	return funcDoc
}

func extractFields(st *ast.StructType, fset *token.FileSet) []FieldDoc {
	var fields []FieldDoc

	for _, field := range st.Fields.List {
		if len(field.Names) == 0 {
			continue // embedded field
		}

		for _, name := range field.Names {
			if !ast.IsExported(name.Name) {
				continue
			}

			fieldDoc := FieldDoc{
				Name:        name.Name,
				Type:        formatType(field.Type),
				Description: cleanDoc(field.Doc.Text()),
				Tags:        extractTags(field.Tag),
			}

			// Check for required tag
			if fieldDoc.Tags != nil {
				if _, ok := fieldDoc.Tags["required"]; ok {
					fieldDoc.Required = true
				}
				if def, ok := fieldDoc.Tags["default"]; ok {
					fieldDoc.Default = def
				}
			}

			fields = append(fields, fieldDoc)
		}
	}

	return fields
}

func extractParams(fields *ast.FieldList, docText string) []ParamDoc {
	var params []ParamDoc

	for _, field := range fields.List {
		typeName := formatType(field.Type)
		for _, name := range field.Names {
			param := ParamDoc{
				Name:        name.Name,
				Type:        typeName,
				Description: extractParamDoc(docText, name.Name),
			}
			params = append(params, param)
		}
		if len(field.Names) == 0 {
			// Anonymous parameter
			params = append(params, ParamDoc{
				Type: typeName,
			})
		}
	}

	return params
}

func extractReturns(fields *ast.FieldList, docText string) []ParamDoc {
	var returns []ParamDoc

	for _, field := range fields.List {
		typeName := formatType(field.Type)
		if len(field.Names) > 0 {
			for _, name := range field.Names {
				returns = append(returns, ParamDoc{
					Name:        name.Name,
					Type:        typeName,
					Description: extractReturnDoc(docText, name.Name),
				})
			}
		} else {
			returns = append(returns, ParamDoc{
				Type:        typeName,
				Description: extractReturnDoc(docText, typeName),
			})
		}
	}

	return returns
}

func formatType(expr ast.Expr) string {
	switch t := expr.(type) {
	case *ast.Ident:
		return t.Name
	case *ast.StarExpr:
		return "*" + formatType(t.X)
	case *ast.ArrayType:
		return "[]" + formatType(t.Elt)
	case *ast.MapType:
		return "map[" + formatType(t.Key) + "]" + formatType(t.Value)
	case *ast.SelectorExpr:
		return formatType(t.X) + "." + t.Sel.Name
	case *ast.InterfaceType:
		return "interface{}"
	case *ast.FuncType:
		return "func" + formatFuncType(t)
	case *ast.ChanType:
		return "chan " + formatType(t.Value)
	case *ast.Ellipsis:
		return "..." + formatType(t.Elt)
	default:
		return "interface{}"
	}
}

func formatFuncType(ft *ast.FuncType) string {
	var buf bytes.Buffer

	buf.WriteString("(")
	if ft.Params != nil {
		for i, p := range ft.Params.List {
			if i > 0 {
				buf.WriteString(", ")
			}
			for j, n := range p.Names {
				if j > 0 {
					buf.WriteString(", ")
				}
				buf.WriteString(n.Name)
			}
			if len(p.Names) > 0 {
				buf.WriteString(" ")
			}
			buf.WriteString(formatType(p.Type))
		}
	}
	buf.WriteString(")")

	if ft.Results != nil && len(ft.Results.List) > 0 {
		buf.WriteString(" ")
		if len(ft.Results.List) > 1 || len(ft.Results.List[0].Names) > 0 {
			buf.WriteString("(")
		}
		for i, r := range ft.Results.List {
			if i > 0 {
				buf.WriteString(", ")
			}
			for j, n := range r.Names {
				if j > 0 {
					buf.WriteString(", ")
				}
				buf.WriteString(n.Name)
			}
			if len(r.Names) > 0 {
				buf.WriteString(" ")
			}
			buf.WriteString(formatType(r.Type))
		}
		if len(ft.Results.List) > 1 || len(ft.Results.List[0].Names) > 0 {
			buf.WriteString(")")
		}
	}

	return buf.String()
}

func extractTags(tag *ast.BasicLit) map[string]string {
	if tag == nil {
		return nil
	}

	tags := make(map[string]string)
	tagStr := strings.Trim(tag.Value, "`")

	// Simple tag parsing
	re := regexp.MustCompile(`(\w+):"([^"]*)"`)
	matches := re.FindAllStringSubmatch(tagStr, -1)
	for _, match := range matches {
		tags[match[1]] = match[2]
	}

	return tags
}

var annotationRe = regexp.MustCompile(`@(\w+)\s+(.+)`)

func extractAnnotation(doc string, name string) string {
	lines := strings.Split(doc, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "@"+name) {
			parts := strings.SplitN(line, " ", 2)
			if len(parts) > 1 {
				return strings.TrimSpace(parts[1])
			}
			return ""
		}
	}
	return ""
}

func extractParamDoc(doc string, paramName string) string {
	lines := strings.Split(doc, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		// Look for patterns like "paramName - description" or "@param paramName description"
		if strings.HasPrefix(line, paramName+" ") || strings.HasPrefix(line, paramName+":") {
			return strings.TrimSpace(strings.TrimPrefix(strings.TrimPrefix(line, paramName), ":"))
		}
		if strings.HasPrefix(line, "@param "+paramName) {
			return strings.TrimSpace(strings.TrimPrefix(line, "@param "+paramName))
		}
	}
	return ""
}

func extractReturnDoc(doc string, typeName string) string {
	lines := strings.Split(doc, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "@return") || strings.HasPrefix(line, "@returns") {
			return strings.TrimSpace(strings.TrimPrefix(strings.TrimPrefix(line, "@returns"), "@return"))
		}
	}
	return ""
}

func extractErrors(doc string) []string {
	var errors []string
	lines := strings.Split(doc, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "@error") || strings.HasPrefix(line, "@throws") {
			errDesc := strings.TrimSpace(strings.TrimPrefix(strings.TrimPrefix(line, "@throws"), "@error"))
			if errDesc != "" {
				errors = append(errors, errDesc)
			}
		}
	}
	return errors
}

func cleanDoc(doc string) string {
	lines := strings.Split(doc, "\n")
	var cleaned []string
	for _, line := range lines {
		line = strings.TrimSpace(line)
		// Skip annotation lines
		if strings.HasPrefix(line, "@") {
			continue
		}
		cleaned = append(cleaned, line)
	}
	return strings.TrimSpace(strings.Join(cleaned, "\n"))
}
