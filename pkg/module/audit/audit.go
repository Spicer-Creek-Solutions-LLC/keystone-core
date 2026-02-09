// Package audit provides module dependency auditing and breaking change detection
package audit

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"sort"
	"strings"
)

// ChangeType represents the type of API change
type ChangeType string

const (
	// ChangeAdded indicates something was added
	ChangeAdded ChangeType = "added"
	// ChangeRemoved indicates something was removed
	ChangeRemoved ChangeType = "removed"
	// ChangeModified indicates something was modified
	ChangeModified ChangeType = "modified"
	// ChangeDeprecated indicates something was deprecated
	ChangeDeprecated ChangeType = "deprecated"
)

// Severity represents the severity of a change
type Severity string

const (
	// SeverityPatch indicates a patch-level change (backwards compatible)
	SeverityPatch Severity = "patch"
	// SeverityMinor indicates a minor version change (backwards compatible addition)
	SeverityMinor Severity = "minor"
	// SeverityMajor indicates a major version change (breaking change)
	SeverityMajor Severity = "major"
)

// APIElement represents a public API element
type APIElement struct {
	Name       string            `json:"name"`
	Kind       string            `json:"kind"` // function, type, method, field, const, var
	Signature  string            `json:"signature,omitempty"`
	Package    string            `json:"package"`
	Exported   bool              `json:"exported"`
	Deprecated bool              `json:"deprecated,omitempty"`
	Doc        string            `json:"doc,omitempty"`
	Metadata   map[string]string `json:"metadata,omitempty"`
}

// APISnapshot represents a snapshot of a module's public API
type APISnapshot struct {
	Module    string                 `json:"module"`
	Version   string                 `json:"version"`
	Elements  map[string]*APIElement `json:"elements"`
	Timestamp int64                  `json:"timestamp,omitempty"`
}

// NewAPISnapshot creates a new empty API snapshot
func NewAPISnapshot(module, version string) *APISnapshot {
	return &APISnapshot{
		Module:   module,
		Version:  version,
		Elements: make(map[string]*APIElement),
	}
}

// Add adds an API element to the snapshot
func (s *APISnapshot) Add(elem *APIElement) {
	key := fmt.Sprintf("%s.%s", elem.Kind, elem.Name)
	s.Elements[key] = elem
}

// Change represents a detected API change
type Change struct {
	Type        ChangeType  `json:"type"`
	Severity    Severity    `json:"severity"`
	Element     string      `json:"element"`
	Kind        string      `json:"kind"`
	Description string      `json:"description"`
	Before      *APIElement `json:"before,omitempty"`
	After       *APIElement `json:"after,omitempty"`
	Suggestion  string      `json:"suggestion,omitempty"`
}

// Result contains the result of an API audit
type Result struct {
	Module        string    `json:"module"`
	OldVersion    string    `json:"old_version"`
	NewVersion    string    `json:"new_version"`
	Changes       []*Change `json:"changes"`
	BreakingCount int       `json:"breaking_count"`
	MinorCount    int       `json:"minor_count"`
	PatchCount    int       `json:"patch_count"`
	Compatible    bool      `json:"compatible"`
}

// Auditor audits module APIs for breaking changes
type Auditor struct {
	// IgnorePrivate ignores non-exported elements
	IgnorePrivate bool

	// IgnoreInternal ignores internal packages
	IgnoreInternal bool

	// StrictMode enables strict checking (doc changes, etc.)
	StrictMode bool
}

// NewAuditor creates a new auditor
func NewAuditor() *Auditor {
	return &Auditor{
		IgnorePrivate:  true,
		IgnoreInternal: true,
		StrictMode:     false,
	}
}

// CompareSnapshots compares two API snapshots and returns detected changes
func (a *Auditor) CompareSnapshots(old, updated *APISnapshot) *Result {
	result := &Result{
		Module:     updated.Module,
		OldVersion: old.Version,
		NewVersion: updated.Version,
		Changes:    make([]*Change, 0),
		Compatible: true,
	}

	// Check for removed elements (breaking)
	for key, oldElem := range old.Elements {
		if a.IgnorePrivate && !oldElem.Exported {
			continue
		}
		if newElem, exists := updated.Elements[key]; !exists {
			change := &Change{
				Type:        ChangeRemoved,
				Severity:    SeverityMajor,
				Element:     oldElem.Name,
				Kind:        oldElem.Kind,
				Description: fmt.Sprintf("Removed %s %s", oldElem.Kind, oldElem.Name),
				Before:      oldElem,
				Suggestion:  fmt.Sprintf("Consider deprecating %s instead of removing it", oldElem.Name),
			}
			result.Changes = append(result.Changes, change)
			result.BreakingCount++
			result.Compatible = false
		} else {
			// Check for modifications
			changes := a.compareElements(oldElem, newElem)
			result.Changes = append(result.Changes, changes...)
			for _, c := range changes {
				switch c.Severity {
				case SeverityMajor:
					result.BreakingCount++
					result.Compatible = false
				case SeverityMinor:
					result.MinorCount++
				case SeverityPatch:
					result.PatchCount++
				}
			}
		}
	}

	// Check for added elements (minor)
	for key, newElem := range updated.Elements {
		if a.IgnorePrivate && !newElem.Exported {
			continue
		}
		if _, exists := old.Elements[key]; !exists {
			change := &Change{
				Type:        ChangeAdded,
				Severity:    SeverityMinor,
				Element:     newElem.Name,
				Kind:        newElem.Kind,
				Description: fmt.Sprintf("Added %s %s", newElem.Kind, newElem.Name),
				After:       newElem,
			}
			result.Changes = append(result.Changes, change)
			result.MinorCount++
		}
	}

	// Sort changes by severity
	sort.Slice(result.Changes, func(i, j int) bool {
		sevOrder := map[Severity]int{SeverityMajor: 0, SeverityMinor: 1, SeverityPatch: 2}
		return sevOrder[result.Changes[i].Severity] < sevOrder[result.Changes[j].Severity]
	})

	return result
}

func (a *Auditor) compareElements(old, updated *APIElement) []*Change {
	var changes []*Change

	// Check for signature changes (breaking for functions/methods)
	if old.Signature != updated.Signature && old.Signature != "" && updated.Signature != "" {
		change := &Change{
			Type:        ChangeModified,
			Severity:    SeverityMajor,
			Element:     old.Name,
			Kind:        old.Kind,
			Description: fmt.Sprintf("Signature changed for %s %s", old.Kind, old.Name),
			Before:      old,
			After: updated,
			Suggestion:  "Add a new function with the new signature and deprecate the old one",
		}
		changes = append(changes, change)
	}

	// Check for deprecation changes
	if !old.Deprecated && updated.Deprecated {
		change := &Change{
			Type:        ChangeDeprecated,
			Severity:    SeverityPatch,
			Element:     old.Name,
			Kind:        old.Kind,
			Description: fmt.Sprintf("Deprecated %s %s", old.Kind, old.Name),
			Before:      old,
			After: updated,
		}
		changes = append(changes, change)
	}

	// In strict mode, check doc changes
	if a.StrictMode && old.Doc != updated.Doc {
		change := &Change{
			Type:        ChangeModified,
			Severity:    SeverityPatch,
			Element:     old.Name,
			Kind:        old.Kind,
			Description: fmt.Sprintf("Documentation changed for %s %s", old.Kind, old.Name),
			Before:      old,
			After: updated,
		}
		changes = append(changes, change)
	}

	return changes
}

// ExtractSnapshot extracts an API snapshot from Go source code
func (a *Auditor) ExtractSnapshot(module, version, srcPath string) (*APISnapshot, error) {
	snapshot := NewAPISnapshot(module, version)

	fset := token.NewFileSet()
	pkgs, err := parser.ParseDir(fset, srcPath, nil, parser.ParseComments)
	if err != nil {
		return nil, fmt.Errorf("failed to parse package: %w", err)
	}

	for pkgName, pkg := range pkgs {
		// Skip test packages
		if strings.HasSuffix(pkgName, "_test") {
			continue
		}

		for _, file := range pkg.Files {
			a.extractFromFile(snapshot, pkgName, file)
		}
	}

	return snapshot, nil
}

func (a *Auditor) extractFromFile(snapshot *APISnapshot, pkgName string, file *ast.File) {
	for _, decl := range file.Decls {
		switch d := decl.(type) {
		case *ast.FuncDecl:
			a.extractFunction(snapshot, pkgName, d)
		case *ast.GenDecl:
			a.extractGenDecl(snapshot, pkgName, d)
		}
	}
}

func (a *Auditor) extractFunction(snapshot *APISnapshot, pkgName string, fn *ast.FuncDecl) {
	if fn.Name == nil {
		return
	}

	name := fn.Name.Name
	exported := ast.IsExported(name)

	if a.IgnorePrivate && !exported {
		return
	}

	kind := "function"
	fullName := name

	// Check if this is a method
	if fn.Recv != nil && len(fn.Recv.List) > 0 {
		kind = "method"
		recvType := formatFieldType(fn.Recv.List[0].Type)
		// Strip pointer prefix for cleaner method names
		recvType = strings.TrimPrefix(recvType, "*")
		fullName = fmt.Sprintf("%s.%s", recvType, name)
	}

	doc := ""
	deprecated := false
	if fn.Doc != nil {
		doc = fn.Doc.Text()
		deprecated = strings.Contains(strings.ToLower(doc), "deprecated")
	}

	elem := &APIElement{
		Name:       fullName,
		Kind:       kind,
		Signature:  formatFuncSignature(fn),
		Package:    pkgName,
		Exported:   exported,
		Deprecated: deprecated,
		Doc:        doc,
	}

	snapshot.Add(elem)
}

func (a *Auditor) extractGenDecl(snapshot *APISnapshot, pkgName string, decl *ast.GenDecl) {
	for _, spec := range decl.Specs {
		switch s := spec.(type) {
		case *ast.TypeSpec:
			a.extractType(snapshot, pkgName, decl, s)
		case *ast.ValueSpec:
			a.extractValue(snapshot, pkgName, decl, s)
		}
	}
}

func (a *Auditor) extractType(snapshot *APISnapshot, pkgName string, decl *ast.GenDecl, spec *ast.TypeSpec) {
	if spec.Name == nil {
		return
	}

	name := spec.Name.Name
	exported := ast.IsExported(name)

	if a.IgnorePrivate && !exported {
		return
	}

	doc := ""
	deprecated := false
	if decl.Doc != nil {
		doc = decl.Doc.Text()
		deprecated = strings.Contains(strings.ToLower(doc), "deprecated")
	}

	kind := "type"
	switch spec.Type.(type) {
	case *ast.StructType:
		kind = "struct"
	case *ast.InterfaceType:
		kind = "interface"
	}

	elem := &APIElement{
		Name:       name,
		Kind:       kind,
		Package:    pkgName,
		Exported:   exported,
		Deprecated: deprecated,
		Doc:        doc,
	}

	snapshot.Add(elem)

	// Extract struct fields
	if st, ok := spec.Type.(*ast.StructType); ok {
		a.extractStructFields(snapshot, pkgName, name, st)
	}

	// Extract interface methods
	if it, ok := spec.Type.(*ast.InterfaceType); ok {
		a.extractInterfaceMethods(snapshot, pkgName, name, it)
	}
}

func (a *Auditor) extractStructFields(snapshot *APISnapshot, pkgName, typeName string, st *ast.StructType) {
	if st.Fields == nil {
		return
	}

	for _, field := range st.Fields.List {
		for _, name := range field.Names {
			if !ast.IsExported(name.Name) && a.IgnorePrivate {
				continue
			}

			fieldName := fmt.Sprintf("%s.%s", typeName, name.Name)
			fieldType := formatFieldType(field.Type)

			doc := ""
			deprecated := false
			if field.Doc != nil {
				doc = field.Doc.Text()
				deprecated = strings.Contains(strings.ToLower(doc), "deprecated")
			}

			elem := &APIElement{
				Name:       fieldName,
				Kind:       "field",
				Signature:  fieldType,
				Package:    pkgName,
				Exported:   ast.IsExported(name.Name),
				Deprecated: deprecated,
				Doc:        doc,
			}

			snapshot.Add(elem)
		}
	}
}

func (a *Auditor) extractInterfaceMethods(snapshot *APISnapshot, pkgName, typeName string, it *ast.InterfaceType) {
	if it.Methods == nil {
		return
	}

	for _, method := range it.Methods.List {
		for _, name := range method.Names {
			if !ast.IsExported(name.Name) && a.IgnorePrivate {
				continue
			}

			methodName := fmt.Sprintf("%s.%s", typeName, name.Name)

			doc := ""
			deprecated := false
			if method.Doc != nil {
				doc = method.Doc.Text()
				deprecated = strings.Contains(strings.ToLower(doc), "deprecated")
			}

			elem := &APIElement{
				Name:       methodName,
				Kind:       "interface_method",
				Signature:  formatFieldType(method.Type),
				Package:    pkgName,
				Exported:   ast.IsExported(name.Name),
				Deprecated: deprecated,
				Doc:        doc,
			}

			snapshot.Add(elem)
		}
	}
}

func (a *Auditor) extractValue(snapshot *APISnapshot, pkgName string, decl *ast.GenDecl, spec *ast.ValueSpec) {
	for _, name := range spec.Names {
		if !ast.IsExported(name.Name) && a.IgnorePrivate {
			continue
		}

		kind := "var"
		if decl.Tok == token.CONST {
			kind = "const"
		}

		doc := ""
		deprecated := false
		if decl.Doc != nil {
			doc = decl.Doc.Text()
			deprecated = strings.Contains(strings.ToLower(doc), "deprecated")
		}

		sig := ""
		if spec.Type != nil {
			sig = formatFieldType(spec.Type)
		}

		elem := &APIElement{
			Name:       name.Name,
			Kind:       kind,
			Signature:  sig,
			Package:    pkgName,
			Exported:   ast.IsExported(name.Name),
			Deprecated: deprecated,
			Doc:        doc,
		}

		snapshot.Add(elem)
	}
}

func formatFuncSignature(fn *ast.FuncDecl) string {
	var parts []string

	// Parameters
	if fn.Type.Params != nil {
		var params []string
		for _, p := range fn.Type.Params.List {
			paramType := formatFieldType(p.Type)
			if len(p.Names) > 0 {
				for _, n := range p.Names {
					params = append(params, fmt.Sprintf("%s %s", n.Name, paramType))
				}
			} else {
				params = append(params, paramType)
			}
		}
		parts = append(parts, "("+strings.Join(params, ", ")+")")
	} else {
		parts = append(parts, "()")
	}

	// Results
	if fn.Type.Results != nil && len(fn.Type.Results.List) > 0 {
		results := make([]string, 0, len(fn.Type.Results.List))
		for _, r := range fn.Type.Results.List {
			results = append(results, formatFieldType(r.Type))
		}
		if len(results) == 1 {
			parts = append(parts, results[0])
		} else {
			parts = append(parts, "("+strings.Join(results, ", ")+")")
		}
	}

	return strings.Join(parts, " ")
}

func formatFieldType(expr ast.Expr) string {
	switch t := expr.(type) {
	case *ast.Ident:
		return t.Name
	case *ast.StarExpr:
		return "*" + formatFieldType(t.X)
	case *ast.ArrayType:
		if t.Len == nil {
			return "[]" + formatFieldType(t.Elt)
		}
		return fmt.Sprintf("[%s]%s", formatFieldType(t.Len), formatFieldType(t.Elt))
	case *ast.MapType:
		return fmt.Sprintf("map[%s]%s", formatFieldType(t.Key), formatFieldType(t.Value))
	case *ast.SelectorExpr:
		return fmt.Sprintf("%s.%s", formatFieldType(t.X), t.Sel.Name)
	case *ast.InterfaceType:
		return "interface{}"
	case *ast.FuncType:
		return "func(...)"
	case *ast.ChanType:
		var dir string
		switch t.Dir {
		case ast.SEND:
			dir = "chan<- "
		case ast.RECV:
			dir = "<-chan "
		default:
			dir = "chan "
		}
		return dir + formatFieldType(t.Value)
	case *ast.Ellipsis:
		return "..." + formatFieldType(t.Elt)
	case *ast.BasicLit:
		return t.Value
	default:
		return "unknown"
	}
}

// Format formats the audit result as a human-readable string
func (r *Result) Format() string {
	var sb strings.Builder

	sb.WriteString(fmt.Sprintf("API Audit: %s\n", r.Module))
	sb.WriteString(fmt.Sprintf("Comparing: %s -> %s\n\n", r.OldVersion, r.NewVersion))

	if len(r.Changes) == 0 {
		sb.WriteString("No API changes detected.\n")
		return sb.String()
	}

	// Group by severity
	var breaking, minor, patch []*Change
	for _, c := range r.Changes {
		switch c.Severity {
		case SeverityMajor:
			breaking = append(breaking, c)
		case SeverityMinor:
			minor = append(minor, c)
		case SeverityPatch:
			patch = append(patch, c)
		}
	}

	if len(breaking) > 0 {
		sb.WriteString("⚠️  BREAKING CHANGES:\n")
		for _, c := range breaking {
			sb.WriteString(fmt.Sprintf("  ✗ [%s] %s\n", c.Type, c.Description))
			if c.Suggestion != "" {
				sb.WriteString(fmt.Sprintf("    💡 %s\n", c.Suggestion))
			}
		}
		sb.WriteString("\n")
	}

	if len(minor) > 0 {
		sb.WriteString("📦 Minor Changes (backwards compatible):\n")
		for _, c := range minor {
			sb.WriteString(fmt.Sprintf("  + [%s] %s\n", c.Type, c.Description))
		}
		sb.WriteString("\n")
	}

	if len(patch) > 0 {
		sb.WriteString("📝 Patch Changes:\n")
		for _, c := range patch {
			sb.WriteString(fmt.Sprintf("  ○ [%s] %s\n", c.Type, c.Description))
		}
		sb.WriteString("\n")
	}

	// Summary
	sb.WriteString("Summary:\n")
	sb.WriteString(fmt.Sprintf("  Breaking: %d, Minor: %d, Patch: %d\n", r.BreakingCount, r.MinorCount, r.PatchCount))

	if r.Compatible {
		sb.WriteString("  ✓ API is backwards compatible\n")
	} else {
		sb.WriteString("  ✗ API has breaking changes - requires major version bump\n")
	}

	return sb.String()
}

// SuggestedVersion suggests the minimum version bump based on changes
func (r *Result) SuggestedVersion() string {
	if r.BreakingCount > 0 {
		return "major"
	}
	if r.MinorCount > 0 {
		return "minor"
	}
	return "patch"
}
