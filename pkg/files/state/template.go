// Package state provides integration between the file distribution system and state management.
package state

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"strings"
	"text/template"
)

// TemplateRenderer renders templates with variable substitution.
type TemplateRenderer struct {
	// funcs contains custom template functions.
	funcs template.FuncMap

	// leftDelim is the left template delimiter.
	leftDelim string

	// rightDelim is the right template delimiter.
	rightDelim string

	// missingKey specifies how to handle missing keys.
	// Options: "invalid", "zero", "error"
	missingKey string
}

// TemplateConfig configures template rendering.
type TemplateConfig struct {
	// LeftDelim is the left template delimiter (default "{{").
	LeftDelim string

	// RightDelim is the right template delimiter (default "}}").
	RightDelim string

	// MissingKey specifies how to handle missing keys.
	// "invalid" - print "<no value>"
	// "zero" - print zero value
	// "error" - return an error
	MissingKey string

	// ExtraFuncs contains additional template functions.
	ExtraFuncs template.FuncMap
}

// NewTemplateRenderer creates a new template renderer.
func NewTemplateRenderer(config *TemplateConfig) *TemplateRenderer {
	r := &TemplateRenderer{
		funcs:      make(template.FuncMap),
		leftDelim:  "{{",
		rightDelim: "}}",
		missingKey: "error",
	}

	if config != nil {
		if config.LeftDelim != "" {
			r.leftDelim = config.LeftDelim
		}
		if config.RightDelim != "" {
			r.rightDelim = config.RightDelim
		}
		if config.MissingKey != "" {
			r.missingKey = config.MissingKey
		}
		if config.ExtraFuncs != nil {
			for k, v := range config.ExtraFuncs {
				r.funcs[k] = v
			}
		}
	}

	// Add default template functions.
	r.addDefaultFuncs()

	return r
}

// addDefaultFuncs adds default template functions.
func (r *TemplateRenderer) addDefaultFuncs() {
	// String functions.
	r.funcs["upper"] = strings.ToUpper
	r.funcs["lower"] = strings.ToLower
	r.funcs["title"] = strings.Title
	r.funcs["trim"] = strings.TrimSpace
	r.funcs["trimPrefix"] = strings.TrimPrefix
	r.funcs["trimSuffix"] = strings.TrimSuffix
	r.funcs["replace"] = strings.ReplaceAll
	r.funcs["contains"] = strings.Contains
	r.funcs["hasPrefix"] = strings.HasPrefix
	r.funcs["hasSuffix"] = strings.HasSuffix
	r.funcs["split"] = strings.Split
	r.funcs["join"] = strings.Join

	// Default value function.
	r.funcs["default"] = func(def interface{}, val interface{}) interface{} {
		if val == nil || val == "" {
			return def
		}
		return val
	}

	// Ternary function.
	r.funcs["ternary"] = func(trueVal, falseVal interface{}, condition bool) interface{} {
		if condition {
			return trueVal
		}
		return falseVal
	}

	// Environment variable function.
	r.funcs["env"] = os.Getenv

	// Quote function.
	r.funcs["quote"] = func(s string) string {
		return fmt.Sprintf("%q", s)
	}

	// Indent function.
	r.funcs["indent"] = func(spaces int, s string) string {
		pad := strings.Repeat(" ", spaces)
		return pad + strings.ReplaceAll(s, "\n", "\n"+pad)
	}

	// Nindent function (newline + indent).
	r.funcs["nindent"] = func(spaces int, s string) string {
		pad := strings.Repeat(" ", spaces)
		return "\n" + pad + strings.ReplaceAll(s, "\n", "\n"+pad)
	}
}

// Render renders a template string with the given variables.
func (r *TemplateRenderer) Render(templateStr string, vars map[string]interface{}) (string, error) {
	tmpl := template.New("template").
		Delims(r.leftDelim, r.rightDelim).
		Funcs(r.funcs)

	// Set missing key option.
	switch r.missingKey {
	case "invalid":
		tmpl = tmpl.Option("missingkey=invalid")
	case "zero":
		tmpl = tmpl.Option("missingkey=zero")
	case "error":
		tmpl = tmpl.Option("missingkey=error")
	}

	parsed, err := tmpl.Parse(templateStr)
	if err != nil {
		return "", fmt.Errorf("failed to parse template: %w", err)
	}

	var buf bytes.Buffer
	if err := parsed.Execute(&buf, vars); err != nil {
		return "", fmt.Errorf("failed to execute template: %w", err)
	}

	return buf.String(), nil
}

// RenderFile renders a template file with the given variables.
func (r *TemplateRenderer) RenderFile(path string, vars map[string]interface{}) (string, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("failed to read template file: %w", err)
	}

	return r.Render(string(content), vars)
}

// RenderReader renders a template from a reader with the given variables.
func (r *TemplateRenderer) RenderReader(reader io.Reader, vars map[string]interface{}) (string, error) {
	content, err := io.ReadAll(reader)
	if err != nil {
		return "", fmt.Errorf("failed to read template: %w", err)
	}

	return r.Render(string(content), vars)
}

// TemplateFileSource wraps a FileSource and applies template rendering.
type TemplateFileSource struct {
	// source is the underlying file source.
	source FileSource

	// renderer is the template renderer.
	renderer *TemplateRenderer

	// vars contains the template variables.
	vars map[string]interface{}
}

// NewTemplateFileSource creates a new template file source.
func NewTemplateFileSource(source FileSource, renderer *TemplateRenderer, vars map[string]interface{}) *TemplateFileSource {
	return &TemplateFileSource{
		source:   source,
		renderer: renderer,
		vars:     vars,
	}
}

// Get retrieves the file content and renders it as a template.
func (t *TemplateFileSource) Get(ctx context.Context) (io.ReadCloser, error) {
	// Get the raw content.
	reader, err := t.source.Get(ctx)
	if err != nil {
		return nil, err
	}
	defer reader.Close()

	// Render the template.
	rendered, err := t.renderer.RenderReader(reader, t.vars)
	if err != nil {
		return nil, err
	}

	return io.NopCloser(strings.NewReader(rendered)), nil
}

// GetChecksum returns the expected checksum.
// Note: Returns empty since the rendered content differs from source.
func (t *TemplateFileSource) GetChecksum() string {
	return ""
}

// GetVersion returns the version.
func (t *TemplateFileSource) GetVersion() string {
	return t.source.GetVersion()
}

// TemplateSourceResolver resolves template sources.
type TemplateSourceResolver struct {
	// fileResolver resolves file sources.
	fileResolver *FileSourceResolver

	// renderer is the template renderer.
	renderer *TemplateRenderer
}

// NewTemplateSourceResolver creates a new template source resolver.
func NewTemplateSourceResolver(fileResolver *FileSourceResolver, renderer *TemplateRenderer) *TemplateSourceResolver {
	return &TemplateSourceResolver{
		fileResolver: fileResolver,
		renderer:     renderer,
	}
}

// Resolve resolves a template source and renders it to a local file.
func (r *TemplateSourceResolver) Resolve(ctx context.Context, sourceURL string, vars map[string]interface{}) (string, error) {
	// Resolve the source file.
	localPath, err := r.fileResolver.Resolve(ctx, sourceURL)
	if err != nil {
		return "", err
	}

	// If no vars, return as-is.
	if len(vars) == 0 {
		return localPath, nil
	}

	// Render the template.
	rendered, err := r.renderer.RenderFile(localPath, vars)
	if err != nil {
		return "", fmt.Errorf("failed to render template: %w", err)
	}

	// Write to temp file.
	tmpFile, err := os.CreateTemp("", "kscore-template-*")
	if err != nil {
		return "", fmt.Errorf("failed to create temp file: %w", err)
	}
	defer tmpFile.Close()

	if _, err := tmpFile.WriteString(rendered); err != nil {
		os.Remove(tmpFile.Name())
		return "", fmt.Errorf("failed to write temp file: %w", err)
	}

	return tmpFile.Name(), nil
}

// ResolveToString resolves a template source and returns the rendered content.
func (r *TemplateSourceResolver) ResolveToString(ctx context.Context, sourceURL string, vars map[string]interface{}) (string, error) {
	// Resolve the source file.
	localPath, err := r.fileResolver.Resolve(ctx, sourceURL)
	if err != nil {
		return "", err
	}

	// If no vars, read and return.
	if len(vars) == 0 {
		content, err := os.ReadFile(localPath)
		if err != nil {
			return "", err
		}
		return string(content), nil
	}

	// Render the template.
	return r.renderer.RenderFile(localPath, vars)
}
