// Package template provides reusable step templates for runbook automation.
package template

import (
	"fmt"
	"sync"

	"github.com/shawnbutts/keystone-core/internal/runbook"
)

// Template represents a reusable sequence of steps with parameterization.
type Template struct {
	// APIVersion is the template API version (e.g., "runbook.keystone.io/v1")
	APIVersion string `yaml:"apiVersion" json:"api_version"`

	// Kind is always "StepTemplate"
	Kind string `yaml:"kind" json:"kind"`

	// Metadata contains template metadata
	Metadata TemplateMetadata `yaml:"metadata" json:"metadata"`

	// Spec contains the template specification
	Spec TemplateSpec `yaml:"spec" json:"spec"`
}

// TemplateMetadata contains metadata about the template.
type TemplateMetadata struct {
	// Name is the unique template name
	Name string `yaml:"name" json:"name"`

	// Version is the semantic version (e.g., "1.0.0")
	Version string `yaml:"version" json:"version"`

	// Description describes what the template does
	Description string `yaml:"description,omitempty" json:"description,omitempty"`

	// Labels for categorization
	Labels map[string]string `yaml:"labels,omitempty" json:"labels,omitempty"`

	// Annotations for additional metadata
	Annotations map[string]string `yaml:"annotations,omitempty" json:"annotations,omitempty"`
}

// TemplateSpec contains the template specification.
type TemplateSpec struct {
	// Parameters defines the template parameters
	Parameters []TemplateParameter `yaml:"parameters,omitempty" json:"parameters,omitempty"`

	// Steps is the list of steps in the template
	Steps []runbook.Step `yaml:"steps" json:"steps"`

	// Outputs defines which step outputs to expose
	Outputs []TemplateOutput `yaml:"outputs,omitempty" json:"outputs,omitempty"`
}

// TemplateParameter defines a parameter for the template.
type TemplateParameter struct {
	// Name is the parameter name
	Name string `yaml:"name" json:"name"`

	// Type is the parameter type (string, int, bool, list, map)
	Type string `yaml:"type" json:"type"`

	// Required indicates if the parameter must be provided
	Required bool `yaml:"required" json:"required"`

	// Default is the default value if not provided
	Default interface{} `yaml:"default,omitempty" json:"default,omitempty"`

	// Description describes the parameter
	Description string `yaml:"description,omitempty" json:"description,omitempty"`

	// Validation contains validation rules
	Validation *ParameterValidation `yaml:"validation,omitempty" json:"validation,omitempty"`
}

// ParameterValidation defines validation rules for a parameter.
type ParameterValidation struct {
	// Pattern is a regex pattern for string validation
	Pattern string `yaml:"pattern,omitempty" json:"pattern,omitempty"`

	// Min value for numeric types or length for strings/lists
	Min *float64 `yaml:"min,omitempty" json:"min,omitempty"`

	// Max value for numeric types or length for strings/lists
	Max *float64 `yaml:"max,omitempty" json:"max,omitempty"`

	// Enum constrains values to a specific set
	Enum []interface{} `yaml:"enum,omitempty" json:"enum,omitempty"`
}

// TemplateOutput defines an output from the template.
type TemplateOutput struct {
	// Name is the output name
	Name string `yaml:"name" json:"name"`

	// Source references a step output (e.g., "steps.deploy.outputs.revision")
	Source string `yaml:"source" json:"source"`

	// Description describes the output
	Description string `yaml:"description,omitempty" json:"description,omitempty"`
}

// TemplateRef is a reference to a template used in a runbook.
type TemplateRef struct {
	// Name is the template name
	Name string `yaml:"name" json:"name"`

	// Version is the template version (optional, latest if not specified)
	Version string `yaml:"version,omitempty" json:"version,omitempty"`

	// Parameters are the values for template parameters
	Parameters map[string]interface{} `yaml:"parameters,omitempty" json:"parameters,omitempty"`

	// OutputMappings map template outputs to runbook step outputs
	OutputMappings map[string]string `yaml:"outputMappings,omitempty" json:"output_mappings,omitempty"`
}

// Registry manages template storage and retrieval.
type Registry struct {
	mu        sync.RWMutex
	templates map[string]map[string]*Template // name -> version -> template
	latest    map[string]string               // name -> latest version
}

// NewRegistry creates a new template registry.
func NewRegistry() *Registry {
	return &Registry{
		templates: make(map[string]map[string]*Template),
		latest:    make(map[string]string),
	}
}

// Register adds a template to the registry.
func (r *Registry) Register(tmpl *Template) error {
	if tmpl.Metadata.Name == "" {
		return fmt.Errorf("template name is required")
	}
	if tmpl.Metadata.Version == "" {
		return fmt.Errorf("template version is required")
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	// Initialize version map if needed
	if r.templates[tmpl.Metadata.Name] == nil {
		r.templates[tmpl.Metadata.Name] = make(map[string]*Template)
	}

	// Check for duplicate
	if _, exists := r.templates[tmpl.Metadata.Name][tmpl.Metadata.Version]; exists {
		return fmt.Errorf("template %s version %s already registered", tmpl.Metadata.Name, tmpl.Metadata.Version)
	}

	r.templates[tmpl.Metadata.Name][tmpl.Metadata.Version] = tmpl

	// Update latest version (simple string comparison, assumes semver format)
	current := r.latest[tmpl.Metadata.Name]
	if current == "" || tmpl.Metadata.Version > current {
		r.latest[tmpl.Metadata.Name] = tmpl.Metadata.Version
	}

	return nil
}

// Get retrieves a template by name and version.
// If version is empty, returns the latest version.
func (r *Registry) Get(name, version string) (*Template, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	versions, ok := r.templates[name]
	if !ok {
		return nil, false
	}

	if version == "" {
		version = r.latest[name]
	}

	tmpl, ok := versions[version]
	return tmpl, ok
}

// GetLatestVersion returns the latest version string for a template.
func (r *Registry) GetLatestVersion(name string) (string, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	version, ok := r.latest[name]
	return version, ok
}

// List returns all registered templates.
func (r *Registry) List() []*Template {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var result []*Template
	for _, versions := range r.templates {
		for _, tmpl := range versions {
			result = append(result, tmpl)
		}
	}
	return result
}

// ListVersions returns all versions of a template.
func (r *Registry) ListVersions(name string) []string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	versions, ok := r.templates[name]
	if !ok {
		return nil
	}

	result := make([]string, 0, len(versions))
	for v := range versions {
		result = append(result, v)
	}
	return result
}

// Delete removes a specific version of a template.
func (r *Registry) Delete(name, version string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	versions, ok := r.templates[name]
	if !ok {
		return fmt.Errorf("template %s not found", name)
	}

	if _, ok := versions[version]; !ok {
		return fmt.Errorf("template %s version %s not found", name, version)
	}

	delete(versions, version)

	// Clean up if no versions remain
	if len(versions) == 0 {
		delete(r.templates, name)
		delete(r.latest, name)
	} else {
		// Recalculate latest
		r.latest[name] = ""
		for v := range versions {
			if r.latest[name] == "" || v > r.latest[name] {
				r.latest[name] = v
			}
		}
	}

	return nil
}
