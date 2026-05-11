package statemgmt

import (
	"fmt"
	"strings"
	"text/template"
	"unicode"
)

// Renderer evaluates text/template strings using a fixed FuncMap.
// One Renderer is safe to reuse across many state files — the
// FuncMap is read-only after construction and template.Template
// instances are constructed per RenderString call.
//
// The seven custom funcs are the §4.8 set: upper, lower, title,
// trim, join, split, default. missingkey=error is enabled so a
// typo'd variable name fails loud instead of producing the silent
// "<no value>" substitution that text/template ships with.
type Renderer struct {
	funcs template.FuncMap
}

// NewRenderer returns a Renderer pre-populated with the seven §4.8
// custom funcs. Callers do not register their own — the v1.0 surface
// is intentionally fixed; new funcs require an explicit design pass.
func NewRenderer() *Renderer {
	return &Renderer{funcs: defaultFuncMap()}
}

// renderContext is the "." root inside every template. Fields are
// exported so text/template can resolve `.Vars.foo` / `.Facts.bar`.
type renderContext struct {
	Vars  map[string]any
	Facts map[string]any
}

// RenderString evaluates text as a text/template with data as the
// "." root. It is the primitive RenderStateFile calls under the
// hood. Direct use exists for tests and any future caller that needs
// to render an isolated string.
func (r *Renderer) RenderString(text string, data any) (string, error) {
	tpl, err := template.New("statemgmt").Option("missingkey=error").Funcs(r.funcs).Parse(text)
	if err != nil {
		return "", fmt.Errorf("statemgmt: render: parse: %w", err)
	}
	var sb strings.Builder
	if err := tpl.Execute(&sb, data); err != nil {
		return "", fmt.Errorf("statemgmt: render: execute: %w", err)
	}
	return sb.String(), nil
}

// RenderStateFile produces a new StateFile with every templatable
// string in every Declaration rendered against (sf.Variables, facts).
// The original StateFile is not mutated; Declarations are reallocated.
//
// Declaration.Module is intentionally NOT rendered — a typo'd
// module-name template would silently route a declaration to the
// wrong stdlib module. v1.0 forbids module-name templating; v1.x can
// revisit. StateFile.Metadata and StateFile.Variables also pass
// through unrendered (variables are the source of truth — recursive
// expansion is a v1.x feature and a footgun for variable values
// supplied by an attacker-controlled agent).
func (r *Renderer) RenderStateFile(sf *StateFile, facts map[string]any) (*StateFile, error) {
	if sf == nil {
		return nil, nil
	}
	ctx := renderContext{Vars: sf.Variables, Facts: facts}
	out := &StateFile{
		Metadata:     sf.Metadata,
		Includes:     sf.Includes,
		Variables:    sf.Variables,
		Declarations: make([]*Declaration, 0, len(sf.Declarations)),
	}
	for _, decl := range sf.Declarations {
		rendered, err := r.renderDeclaration(decl, ctx)
		if err != nil {
			return nil, err
		}
		out.Declarations = append(out.Declarations, rendered)
	}
	return out, nil
}

func (r *Renderer) renderDeclaration(decl *Declaration, ctx renderContext) (*Declaration, error) {
	name, err := r.renderField(decl.Name, ctx)
	if err != nil {
		return nil, declErr(decl, "Name", err)
	}
	state, err := r.renderField(decl.State, ctx)
	if err != nil {
		return nil, declErr(decl, "State", err)
	}
	params, err := r.renderAny(decl.Params, ctx)
	if err != nil {
		return nil, declErr(decl, "Params", err)
	}
	out := &Declaration{
		Module: decl.Module, // never rendered
		Name:   name,
		State:  state,
	}
	if params != nil {
		if m, ok := params.(map[string]any); ok {
			out.Params = m
		}
	}
	out.ID = decl.Module + ":" + out.Name
	return out, nil
}

// renderField renders a single string. Empty strings short-circuit
// so the template parser does not need to run on every no-op.
func (r *Renderer) renderField(s string, ctx renderContext) (string, error) {
	if s == "" {
		return "", nil
	}
	return r.RenderString(s, ctx)
}

// renderAny walks a value produced by YAML decoding. Strings get
// rendered; lists and maps are descended into; every other type
// passes through.
func (r *Renderer) renderAny(v any, ctx renderContext) (any, error) {
	switch t := v.(type) {
	case nil:
		return nil, nil
	case string:
		return r.renderField(t, ctx)
	case []any:
		out := make([]any, len(t))
		for i, item := range t {
			rendered, err := r.renderAny(item, ctx)
			if err != nil {
				return nil, err
			}
			out[i] = rendered
		}
		return out, nil
	case map[string]any:
		if t == nil {
			return (map[string]any)(nil), nil
		}
		out := make(map[string]any, len(t))
		for k, val := range t {
			rendered, err := r.renderAny(val, ctx)
			if err != nil {
				return nil, err
			}
			out[k] = rendered
		}
		return out, nil
	default:
		return v, nil
	}
}

func declErr(decl *Declaration, field string, err error) error {
	return fmt.Errorf("statemgmt: render: declaration %q: %s: %w", decl.ID, field, err)
}

func defaultFuncMap() template.FuncMap {
	return template.FuncMap{
		"upper":   strings.ToUpper,
		"lower":   strings.ToLower,
		"title":   asciiTitle,
		"trim":    strings.TrimSpace,
		"join":    joinFunc,
		"split":   splitFunc,
		"default": defaultFunc,
	}
}

// asciiTitle capitalizes the first byte of each whitespace-delimited
// word. ASCII-only by design — `strings.Title` is deprecated and
// `x/text/cases.Title` would add a dependency we do not need for the
// v1.0 stdlib's identifier-shaped strings (hostnames, package names,
// service names). Document the ASCII limit; revisit in v1.x if a
// real Unicode case surfaces.
func asciiTitle(s string) string {
	if s == "" {
		return s
	}
	out := []byte(s)
	startOfWord := true
	for i := 0; i < len(out); i++ {
		b := rune(out[i])
		if unicode.IsSpace(b) {
			startOfWord = true
			continue
		}
		if startOfWord && b >= 'a' && b <= 'z' {
			out[i] = byte(b - ('a' - 'A'))
		}
		startOfWord = false
	}
	return string(out)
}

// joinFunc accepts []any because YAML decodes sequences that way.
// Each element is stringified via fmt.Sprint so heterogeneous lists
// ([1, "two", true]) render predictably.
func joinFunc(sep string, items []any) string {
	parts := make([]string, len(items))
	for i, it := range items {
		parts[i] = fmt.Sprint(it)
	}
	return strings.Join(parts, sep)
}

func splitFunc(sep, s string) []string {
	return strings.Split(s, sep)
}

// defaultFunc follows the Sprig convention: return def if v is nil
// or the empty string; pass v through otherwise. Other "empty"
// shapes (empty slice, empty map, zero numbers) are intentionally
// NOT treated as empty — v1.0 keeps the rule simple.
func defaultFunc(def, v any) any {
	if v == nil {
		return def
	}
	if s, ok := v.(string); ok && s == "" {
		return def
	}
	return v
}
