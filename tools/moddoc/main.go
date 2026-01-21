// Command moddoc generates documentation from Go module source code.
//
// Usage:
//
//	moddoc [flags] <package-path>
//
// Flags:
//
//	-o, --output    Output file (default: stdout)
//	-f, --format    Output format: markdown, json (default: markdown)
//	-t, --title     Document title
//	-p, --prefix    Type name prefix to include (e.g., "Module")
//
// Example:
//
//	moddoc -o docs/modules.md ./pkg/plugin/stdlib
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"go/ast"
	"go/doc"
	"go/parser"
	"go/token"
	"io"
	"os"
	"sort"
	"strings"
)

// ModuleDoc represents documentation for a module.
type ModuleDoc struct {
	Name        string       `json:"name"`
	Doc         string       `json:"doc"`
	Package     string       `json:"package"`
	ImportPath  string       `json:"import_path"`
	Fields      []FieldDoc   `json:"fields,omitempty"`
	Methods     []MethodDoc  `json:"methods,omitempty"`
	Constructor *MethodDoc   `json:"constructor,omitempty"`
	Examples    []ExampleDoc `json:"examples,omitempty"`
}

// FieldDoc represents documentation for a field.
type FieldDoc struct {
	Name string `json:"name"`
	Type string `json:"type"`
	Doc  string `json:"doc"`
	Tag  string `json:"tag,omitempty"`
}

// MethodDoc represents documentation for a method.
type MethodDoc struct {
	Name       string     `json:"name"`
	Doc        string     `json:"doc"`
	Signature  string     `json:"signature"`
	Parameters []ParamDoc `json:"parameters,omitempty"`
	Returns    []ParamDoc `json:"returns,omitempty"`
}

// ParamDoc represents documentation for a parameter.
type ParamDoc struct {
	Name string `json:"name"`
	Type string `json:"type"`
}

// ExampleDoc represents an example.
type ExampleDoc struct {
	Name   string `json:"name"`
	Doc    string `json:"doc"`
	Code   string `json:"code"`
	Output string `json:"output,omitempty"`
}

// PackageDoc represents documentation for a package.
type PackageDoc struct {
	Name       string      `json:"name"`
	ImportPath string      `json:"import_path"`
	Doc        string      `json:"doc"`
	Modules    []ModuleDoc `json:"modules"`
	Constants  []ConstDoc  `json:"constants,omitempty"`
	Variables  []VarDoc    `json:"variables,omitempty"`
	Functions  []MethodDoc `json:"functions,omitempty"`
}

// ConstDoc represents a constant.
type ConstDoc struct {
	Name  string `json:"name"`
	Type  string `json:"type"`
	Value string `json:"value"`
	Doc   string `json:"doc"`
}

// VarDoc represents a variable.
type VarDoc struct {
	Name string `json:"name"`
	Type string `json:"type"`
	Doc  string `json:"doc"`
}

var (
	outputFile   = flag.String("o", "", "Output file (default: stdout)")
	outputFormat = flag.String("f", "markdown", "Output format: markdown, json")
	title        = flag.String("t", "", "Document title")
	typePrefix   = flag.String("p", "Module", "Type name suffix to include")
)

func main() {
	flag.Parse()

	if flag.NArg() < 1 {
		fmt.Fprintln(os.Stderr, "Usage: moddoc [flags] <package-path>")
		flag.PrintDefaults()
		os.Exit(1)
	}

	pkgPath := flag.Arg(0)

	pkgDoc, err := parsePackage(pkgPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error parsing package: %v\n", err)
		os.Exit(1)
	}

	var out io.Writer = os.Stdout
	if *outputFile != "" {
		f, err := os.Create(*outputFile)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error creating output file: %v\n", err)
			os.Exit(1)
		}
		defer f.Close()
		out = f
	}

	switch *outputFormat {
	case "json":
		enc := json.NewEncoder(out)
		enc.SetIndent("", "  ")
		if err := enc.Encode(pkgDoc); err != nil {
			fmt.Fprintf(os.Stderr, "Error encoding JSON: %v\n", err)
			os.Exit(1)
		}
	case "markdown":
		if err := writeMarkdown(out, pkgDoc); err != nil {
			fmt.Fprintf(os.Stderr, "Error writing markdown: %v\n", err)
			os.Exit(1)
		}
	default:
		fmt.Fprintf(os.Stderr, "Unknown format: %s\n", *outputFormat)
		os.Exit(1)
	}
}

func parsePackage(pkgPath string) (*PackageDoc, error) {
	fset := token.NewFileSet()

	// Parse the package
	pkgs, err := parser.ParseDir(fset, pkgPath, func(fi os.FileInfo) bool {
		return !strings.HasSuffix(fi.Name(), "_test.go")
	}, parser.ParseComments)
	if err != nil {
		return nil, fmt.Errorf("parsing package: %w", err)
	}

	if len(pkgs) == 0 {
		return nil, fmt.Errorf("no packages found in %s", pkgPath)
	}

	// Get the first (and usually only) package
	var pkg *ast.Package
	for _, p := range pkgs {
		pkg = p
		break
	}

	// Create documentation
	docPkg := doc.New(pkg, pkgPath, doc.AllDecls)

	pkgDoc := &PackageDoc{
		Name:       docPkg.Name,
		ImportPath: pkgPath,
		Doc:        docPkg.Doc,
	}

	// Extract modules (types ending with the prefix)
	for _, t := range docPkg.Types {
		if *typePrefix != "" && !strings.HasSuffix(t.Name, *typePrefix) {
			continue
		}

		modDoc := ModuleDoc{
			Name:       t.Name,
			Doc:        t.Doc,
			Package:    docPkg.Name,
			ImportPath: pkgPath,
		}

		// Extract fields from struct
		if t.Decl != nil {
			for _, spec := range t.Decl.Specs {
				if ts, ok := spec.(*ast.TypeSpec); ok {
					if st, ok := ts.Type.(*ast.StructType); ok {
						modDoc.Fields = extractFields(st, fset)
					}
				}
			}
		}

		// Find constructor (New<TypeName>)
		constructorName := "New" + t.Name
		for _, fn := range docPkg.Funcs {
			if fn.Name == constructorName {
				modDoc.Constructor = extractFunc(fn, fset)
				break
			}
		}

		// Extract methods
		for _, m := range t.Methods {
			modDoc.Methods = append(modDoc.Methods, *extractFunc(m, fset))
		}

		// Sort methods by name
		sort.Slice(modDoc.Methods, func(i, j int) bool {
			return modDoc.Methods[i].Name < modDoc.Methods[j].Name
		})

		pkgDoc.Modules = append(pkgDoc.Modules, modDoc)
	}

	// Sort modules by name
	sort.Slice(pkgDoc.Modules, func(i, j int) bool {
		return pkgDoc.Modules[i].Name < pkgDoc.Modules[j].Name
	})

	// Extract package-level constants
	for _, c := range docPkg.Consts {
		for _, spec := range c.Decl.Specs {
			if vs, ok := spec.(*ast.ValueSpec); ok {
				for i, name := range vs.Names {
					constDoc := ConstDoc{
						Name: name.Name,
						Doc:  c.Doc,
					}
					if vs.Type != nil {
						constDoc.Type = formatNode(fset, vs.Type)
					}
					if i < len(vs.Values) {
						constDoc.Value = formatNode(fset, vs.Values[i])
					}
					pkgDoc.Constants = append(pkgDoc.Constants, constDoc)
				}
			}
		}
	}

	// Extract package-level variables
	for _, v := range docPkg.Vars {
		for _, spec := range v.Decl.Specs {
			if vs, ok := spec.(*ast.ValueSpec); ok {
				for _, name := range vs.Names {
					varDoc := VarDoc{
						Name: name.Name,
						Doc:  v.Doc,
					}
					if vs.Type != nil {
						varDoc.Type = formatNode(fset, vs.Type)
					}
					pkgDoc.Variables = append(pkgDoc.Variables, varDoc)
				}
			}
		}
	}

	// Extract package-level functions (excluding constructors)
	for _, fn := range docPkg.Funcs {
		if strings.HasPrefix(fn.Name, "New") {
			continue
		}
		pkgDoc.Functions = append(pkgDoc.Functions, *extractFunc(fn, fset))
	}

	return pkgDoc, nil
}

func extractFields(st *ast.StructType, fset *token.FileSet) []FieldDoc {
	var fields []FieldDoc
	for _, f := range st.Fields.List {
		for _, name := range f.Names {
			fieldDoc := FieldDoc{
				Name: name.Name,
				Type: formatNode(fset, f.Type),
			}
			if f.Doc != nil {
				fieldDoc.Doc = strings.TrimSpace(f.Doc.Text())
			} else if f.Comment != nil {
				fieldDoc.Doc = strings.TrimSpace(f.Comment.Text())
			}
			if f.Tag != nil {
				fieldDoc.Tag = f.Tag.Value
			}
			fields = append(fields, fieldDoc)
		}
	}
	return fields
}

func extractFunc(fn *doc.Func, fset *token.FileSet) *MethodDoc {
	m := &MethodDoc{
		Name: fn.Name,
		Doc:  fn.Doc,
	}

	if fn.Decl != nil && fn.Decl.Type != nil {
		m.Signature = formatFuncSignature(fn.Decl, fset)

		// Extract parameters
		if fn.Decl.Type.Params != nil {
			for _, p := range fn.Decl.Type.Params.List {
				paramType := formatNode(fset, p.Type)
				if len(p.Names) == 0 {
					m.Parameters = append(m.Parameters, ParamDoc{Type: paramType})
				} else {
					for _, name := range p.Names {
						m.Parameters = append(m.Parameters, ParamDoc{
							Name: name.Name,
							Type: paramType,
						})
					}
				}
			}
		}

		// Extract return values
		if fn.Decl.Type.Results != nil {
			for _, r := range fn.Decl.Type.Results.List {
				retType := formatNode(fset, r.Type)
				if len(r.Names) == 0 {
					m.Returns = append(m.Returns, ParamDoc{Type: retType})
				} else {
					for _, name := range r.Names {
						m.Returns = append(m.Returns, ParamDoc{
							Name: name.Name,
							Type: retType,
						})
					}
				}
			}
		}
	}

	return m
}

func formatFuncSignature(fn *ast.FuncDecl, fset *token.FileSet) string {
	var buf strings.Builder
	buf.WriteString("func ")

	if fn.Recv != nil && len(fn.Recv.List) > 0 {
		buf.WriteString("(")
		for i, r := range fn.Recv.List {
			if i > 0 {
				buf.WriteString(", ")
			}
			if len(r.Names) > 0 {
				buf.WriteString(r.Names[0].Name)
				buf.WriteString(" ")
			}
			buf.WriteString(formatNode(fset, r.Type))
		}
		buf.WriteString(") ")
	}

	buf.WriteString(fn.Name.Name)
	buf.WriteString("(")

	if fn.Type.Params != nil {
		for i, p := range fn.Type.Params.List {
			if i > 0 {
				buf.WriteString(", ")
			}
			for j, name := range p.Names {
				if j > 0 {
					buf.WriteString(", ")
				}
				buf.WriteString(name.Name)
			}
			if len(p.Names) > 0 {
				buf.WriteString(" ")
			}
			buf.WriteString(formatNode(fset, p.Type))
		}
	}

	buf.WriteString(")")

	if fn.Type.Results != nil && len(fn.Type.Results.List) > 0 {
		buf.WriteString(" ")
		if len(fn.Type.Results.List) > 1 || len(fn.Type.Results.List[0].Names) > 0 {
			buf.WriteString("(")
		}
		for i, r := range fn.Type.Results.List {
			if i > 0 {
				buf.WriteString(", ")
			}
			for j, name := range r.Names {
				if j > 0 {
					buf.WriteString(", ")
				}
				buf.WriteString(name.Name)
				buf.WriteString(" ")
			}
			buf.WriteString(formatNode(fset, r.Type))
		}
		if len(fn.Type.Results.List) > 1 || len(fn.Type.Results.List[0].Names) > 0 {
			buf.WriteString(")")
		}
	}

	return buf.String()
}

func formatNode(fset *token.FileSet, node ast.Node) string {
	if node == nil {
		return ""
	}

	switch n := node.(type) {
	case *ast.Ident:
		return n.Name
	case *ast.StarExpr:
		return "*" + formatNode(fset, n.X)
	case *ast.SelectorExpr:
		return formatNode(fset, n.X) + "." + n.Sel.Name
	case *ast.ArrayType:
		if n.Len == nil {
			return "[]" + formatNode(fset, n.Elt)
		}
		return "[" + formatNode(fset, n.Len) + "]" + formatNode(fset, n.Elt)
	case *ast.MapType:
		return "map[" + formatNode(fset, n.Key) + "]" + formatNode(fset, n.Value)
	case *ast.InterfaceType:
		if n.Methods == nil || len(n.Methods.List) == 0 {
			return "interface{}"
		}
		return "interface{...}"
	case *ast.FuncType:
		return "func(...)"
	case *ast.ChanType:
		switch n.Dir {
		case ast.SEND:
			return "chan<- " + formatNode(fset, n.Value)
		case ast.RECV:
			return "<-chan " + formatNode(fset, n.Value)
		default:
			return "chan " + formatNode(fset, n.Value)
		}
	case *ast.Ellipsis:
		return "..." + formatNode(fset, n.Elt)
	case *ast.BasicLit:
		return n.Value
	default:
		return fmt.Sprintf("%T", node)
	}
}

func writeMarkdown(w io.Writer, pkg *PackageDoc) error {
	docTitle := *title
	if docTitle == "" {
		docTitle = fmt.Sprintf("Package %s", pkg.Name)
	}

	fmt.Fprintf(w, "# %s\n\n", docTitle)

	if pkg.Doc != "" {
		fmt.Fprintf(w, "%s\n\n", strings.TrimSpace(pkg.Doc))
	}

	fmt.Fprintf(w, "**Import:** `%s`\n\n", pkg.ImportPath)

	// Table of contents
	if len(pkg.Modules) > 0 {
		fmt.Fprint(w, "## Contents\n\n")
		for _, mod := range pkg.Modules {
			anchor := strings.ToLower(strings.ReplaceAll(mod.Name, " ", "-"))
			fmt.Fprintf(w, "- [%s](#%s)\n", mod.Name, anchor)
		}
		fmt.Fprintln(w)
	}

	// Constants
	if len(pkg.Constants) > 0 {
		fmt.Fprint(w, "## Constants\n\n")
		fmt.Fprintln(w, "```go")
		for _, c := range pkg.Constants {
			if c.Value != "" {
				fmt.Fprintf(w, "%s = %s\n", c.Name, c.Value)
			} else {
				fmt.Fprintf(w, "%s %s\n", c.Name, c.Type)
			}
		}
		fmt.Fprint(w, "```\n\n")
	}

	// Variables
	if len(pkg.Variables) > 0 {
		fmt.Fprint(w, "## Variables\n\n")
		for _, v := range pkg.Variables {
			fmt.Fprintf(w, "### %s\n\n", v.Name)
			if v.Doc != "" {
				fmt.Fprintf(w, "%s\n\n", strings.TrimSpace(v.Doc))
			}
			fmt.Fprintf(w, "**Type:** `%s`\n\n", v.Type)
		}
	}

	// Modules
	for _, mod := range pkg.Modules {
		fmt.Fprintf(w, "## %s\n\n", mod.Name)

		if mod.Doc != "" {
			fmt.Fprintf(w, "%s\n\n", strings.TrimSpace(mod.Doc))
		}

		// Fields
		if len(mod.Fields) > 0 {
			fmt.Fprint(w, "### Fields\n\n")
			fmt.Fprintln(w, "| Field | Type | Description |")
			fmt.Fprintln(w, "|-------|------|-------------|")
			for _, f := range mod.Fields {
				doc := strings.ReplaceAll(f.Doc, "\n", " ")
				fmt.Fprintf(w, "| `%s` | `%s` | %s |\n", f.Name, f.Type, doc)
			}
			fmt.Fprintln(w)
		}

		// Constructor
		if mod.Constructor != nil {
			fmt.Fprint(w, "### Constructor\n\n")
			fmt.Fprintf(w, "```go\n%s\n```\n\n", mod.Constructor.Signature)
			if mod.Constructor.Doc != "" {
				fmt.Fprintf(w, "%s\n\n", strings.TrimSpace(mod.Constructor.Doc))
			}
		}

		// Methods
		if len(mod.Methods) > 0 {
			fmt.Fprint(w, "### Methods\n\n")
			for _, m := range mod.Methods {
				fmt.Fprintf(w, "#### %s\n\n", m.Name)
				fmt.Fprintf(w, "```go\n%s\n```\n\n", m.Signature)
				if m.Doc != "" {
					fmt.Fprintf(w, "%s\n\n", strings.TrimSpace(m.Doc))
				}

				// Parameters
				if len(m.Parameters) > 0 {
					fmt.Fprint(w, "**Parameters:**\n\n")
					for _, p := range m.Parameters {
						if p.Name != "" {
							fmt.Fprintf(w, "- `%s` (`%s`)\n", p.Name, p.Type)
						} else {
							fmt.Fprintf(w, "- `%s`\n", p.Type)
						}
					}
					fmt.Fprintln(w)
				}

				// Returns
				if len(m.Returns) > 0 {
					fmt.Fprint(w, "**Returns:**\n\n")
					for _, r := range m.Returns {
						if r.Name != "" {
							fmt.Fprintf(w, "- `%s` (`%s`)\n", r.Name, r.Type)
						} else {
							fmt.Fprintf(w, "- `%s`\n", r.Type)
						}
					}
					fmt.Fprintln(w)
				}
			}
		}

		fmt.Fprint(w, "---\n\n")
	}

	// Package-level functions
	if len(pkg.Functions) > 0 {
		fmt.Fprint(w, "## Functions\n\n")
		for _, fn := range pkg.Functions {
			fmt.Fprintf(w, "### %s\n\n", fn.Name)
			fmt.Fprintf(w, "```go\n%s\n```\n\n", fn.Signature)
			if fn.Doc != "" {
				fmt.Fprintf(w, "%s\n\n", strings.TrimSpace(fn.Doc))
			}
		}
	}

	// Footer
	fmt.Fprintf(w, "\n---\n\n*Generated by moddoc*\n")

	return nil
}

func init() {
	flag.StringVar(outputFile, "output", "", "Output file (default: stdout)")
	flag.StringVar(outputFormat, "format", "markdown", "Output format: markdown, json")
	flag.StringVar(title, "title", "", "Document title")
	flag.StringVar(typePrefix, "prefix", "Module", "Type name suffix to include")
}
