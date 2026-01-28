package statemgmt

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"runtime"
	"strings"
	"text/template" // nosemgrep: go.lang.security.audit.xss.import-text-template.import-text-template -- templates render state files, not HTML responses
)

// TemplateContext holds data for template rendering
type TemplateContext struct {
	// Vars contains configuration variables
	Vars map[string]interface{}

	// Facts contains agent metadata/facts
	Facts map[string]interface{}
}

// TemplateRenderer renders templates with vars and facts
type TemplateRenderer struct {
	funcMap template.FuncMap
}

// NewTemplateRenderer creates a new template renderer
func NewTemplateRenderer() *TemplateRenderer {
	return &TemplateRenderer{
		funcMap: getTemplateFunctions(),
	}
}

// Render renders a template string with the given context
func (r *TemplateRenderer) Render(templateStr string, ctx *TemplateContext) (string, error) {
	// Create template
	tmpl, err := template.New("state").Funcs(r.funcMap).Parse(templateStr)
	if err != nil {
		return "", fmt.Errorf("template parse error: %w", err)
	}

	// Prepare data for template
	data := map[string]interface{}{
		"vars":  ctx.Vars,
		"facts": ctx.Facts,
	}

	// Execute template
	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		return "", fmt.Errorf("template execution error: %w", err)
	}

	return buf.String(), nil
}

// getTemplateFunctions returns custom template functions
func getTemplateFunctions() template.FuncMap {
	return template.FuncMap{
		// String functions
		"upper":     strings.ToUpper,
		"lower":     strings.ToLower,
		"title":     strings.Title,
		"trim":      strings.TrimSpace,
		"split":     strings.Split,
		"join":      strings.Join,
		"replace":   strings.ReplaceAll,
		"contains":  strings.Contains,
		"hasPrefix": strings.HasPrefix,
		"hasSuffix": strings.HasSuffix,

		// Default value
		"default": func(defaultVal, val interface{}) interface{} {
			if val == nil || val == "" {
				return defaultVal
			}
			return val
		},

		// Conditional
		"ternary": func(condition bool, trueVal, falseVal interface{}) interface{} {
			if condition {
				return trueVal
			}
			return falseVal
		},
	}
}

// Vars represents configuration variables
type Vars struct {
	// Data contains the variable data
	Data map[string]interface{}

	// Environment indicates which environment these vars are for
	Environment string
}

// NewVars creates a new Vars instance
func NewVars() *Vars {
	return &Vars{
		Data: make(map[string]interface{}),
	}
}

// Set sets a variable
func (v *Vars) Set(key string, value interface{}) {
	v.Data[key] = value
}

// Get gets a variable
func (v *Vars) Get(key string) (interface{}, bool) {
	val, ok := v.Data[key]
	return val, ok
}

// Merge merges another Vars into this one
func (v *Vars) Merge(other *Vars) {
	for key, value := range other.Data {
		v.Data[key] = value
	}
}

// Facts represents agent metadata and system facts
type Facts struct {
	// Data contains the facts data
	Data map[string]interface{}
}

// NewFacts creates a new Facts instance
func NewFacts() *Facts {
	facts := &Facts{
		Data: make(map[string]interface{}),
	}

	// Collect system facts
	facts.collectSystemFacts()

	return facts
}

// collectSystemFacts collects basic system facts
func (f *Facts) collectSystemFacts() {
	f.Data["os"] = runtime.GOOS
	f.Data["arch"] = runtime.GOARCH
	f.Data["num_cpu"] = runtime.NumCPU()
	f.Data["go_version"] = runtime.Version()
}

// Set sets a fact
func (f *Facts) Set(key string, value interface{}) {
	f.Data[key] = value
}

// Get gets a fact
func (f *Facts) Get(key string) (interface{}, bool) {
	val, ok := f.Data[key]
	return val, ok
}

// GetString gets a fact as a string
func (f *Facts) GetString(key string) string {
	if val, ok := f.Data[key]; ok {
		if str, ok := val.(string); ok {
			return str
		}
		return fmt.Sprintf("%v", val)
	}
	return ""
}

// LoadVarsFromYAML loads vars from YAML data
func LoadVarsFromYAML(data map[string]interface{}) *Vars {
	vars := NewVars()
	vars.Data = data
	return vars
}

// templateContextKey is a context key for passing template context
type templateContextKey struct{}

// WithTemplateContext adds a template context to a context.Context
func WithTemplateContext(ctx context.Context, tplCtx *TemplateContext) context.Context {
	return context.WithValue(ctx, templateContextKey{}, tplCtx)
}

// TemplateContextFromContext retrieves the template context from a context.Context
// Returns nil if no template context is set
func TemplateContextFromContext(ctx context.Context) *TemplateContext {
	if tplCtx, ok := ctx.Value(templateContextKey{}).(*TemplateContext); ok {
		return tplCtx
	}
	return nil
}

// RenderTemplateFile reads a template file from disk and renders it with the given context
func RenderTemplateFile(templatePath string, ctx *TemplateContext) ([]byte, error) {
	// Read the template file
	templateContent, err := os.ReadFile(templatePath)
	if err != nil {
		return nil, fmt.Errorf("failed to read template file: %w", err)
	}

	// Render the template
	renderer := NewTemplateRenderer()
	rendered, err := renderer.Render(string(templateContent), ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to render template: %w", err)
	}

	return []byte(rendered), nil
}

// RenderStateFile renders all templates in a state file
func RenderStateFile(stateFile *StateFile, vars *Vars, facts *Facts) error {
	renderer := NewTemplateRenderer()
	ctx := &TemplateContext{
		Vars:  vars.Data,
		Facts: facts.Data,
	}

	// Render variables in metadata
	if stateFile.Metadata.Description != "" {
		rendered, err := renderer.Render(stateFile.Metadata.Description, ctx)
		if err != nil {
			return fmt.Errorf("failed to render metadata.description: %w", err)
		}
		stateFile.Metadata.Description = rendered
	}

	// Render state parameters
	for module, declarations := range stateFile.States {
		for i := range declarations {
			decl := &declarations[i]

			// Render parameters that might contain templates
			for key, value := range decl.Parameters {
				if strVal, ok := value.(string); ok {
					if strings.Contains(strVal, "{{") {
						rendered, err := renderer.Render(strVal, ctx)
						if err != nil {
							return fmt.Errorf("failed to render %s.%s parameter %s: %w", module, decl.ID, key, err)
						}
						decl.Parameters[key] = rendered
					}
				}
			}
		}
	}

	return nil
}
