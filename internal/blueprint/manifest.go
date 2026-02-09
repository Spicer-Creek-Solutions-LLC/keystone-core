// Package blueprint provides composable infrastructure patterns for Keystone Core.
// Blueprints are pre-packaged, reusable collections of states that implement
// common infrastructure patterns, similar to Salt Project's Formulas.
package blueprint

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// APIVersion is the current blueprint API version.
const APIVersion = "blueprints.keystone-core.io/v1"

// Kind is the resource kind for blueprints.
const Kind = "Blueprint"

// Blueprint represents a complete blueprint manifest.
type Blueprint struct {
	// APIVersion specifies the blueprint API version (e.g., "blueprints.keystone-core.io/v1")
	APIVersion string `yaml:"apiVersion" json:"apiVersion"`

	// Kind is always "Blueprint"
	Kind string `yaml:"kind" json:"kind"`

	// Metadata contains blueprint identification and documentation
	Metadata Metadata `yaml:"metadata" json:"metadata"`

	// Compatibility defines version and platform requirements
	Compatibility *Compatibility `yaml:"compatibility,omitempty" json:"compatibility,omitempty"`

	// Dependencies defines other blueprints this one depends on
	Dependencies *Dependencies `yaml:"dependencies,omitempty" json:"dependencies,omitempty"`

	// Features defines optional feature flags
	Features map[string]Feature `yaml:"features,omitempty" json:"features,omitempty"`

	// Entrypoints defines entry points for different operations
	Entrypoints map[string]string `yaml:"entrypoints,omitempty" json:"entrypoints,omitempty"`

	// Parameters defines the parameter schema
	Parameters map[string]ParameterSchema `yaml:"parameters,omitempty" json:"parameters,omitempty"`

	// Outputs defines values exposed after application
	Outputs map[string]Output `yaml:"outputs,omitempty" json:"outputs,omitempty"`

	// Hooks defines lifecycle hooks
	Hooks *Hooks `yaml:"hooks,omitempty" json:"hooks,omitempty"`

	// SourcePath is the path to the blueprint directory (not in YAML)
	SourcePath string `yaml:"-" json:"-"`
}

// Metadata contains blueprint identification and documentation.
type Metadata struct {
	// Name is the blueprint name (e.g., "web-app-stack")
	Name string `yaml:"name" json:"name"`

	// Version is the semantic version (e.g., "1.2.0")
	Version string `yaml:"version" json:"version"`

	// Description is a human-readable description
	Description string `yaml:"description,omitempty" json:"description,omitempty"`

	// Maintainers lists the blueprint maintainers
	Maintainers []Maintainer `yaml:"maintainers,omitempty" json:"maintainers,omitempty"`

	// License specifies the license (e.g., "Apache-2.0")
	License string `yaml:"license,omitempty" json:"license,omitempty"`

	// Repository is the source repository URL
	Repository string `yaml:"repository,omitempty" json:"repository,omitempty"`

	// Documentation is the documentation URL
	Documentation string `yaml:"documentation,omitempty" json:"documentation,omitempty"`

	// Keywords for search and discovery
	Keywords []string `yaml:"keywords,omitempty" json:"keywords,omitempty"`

	// Categories for classification
	Categories []string `yaml:"categories,omitempty" json:"categories,omitempty"`
}

// Maintainer represents a blueprint maintainer.
type Maintainer struct {
	// Name of the maintainer
	Name string `yaml:"name" json:"name"`

	// Email of the maintainer
	Email string `yaml:"email,omitempty" json:"email,omitempty"`

	// URL of the maintainer's profile
	URL string `yaml:"url,omitempty" json:"url,omitempty"`
}

// Compatibility defines version and platform requirements.
type Compatibility struct {
	// Kscore is the required Keystone Core version constraint (e.g., ">=1.5.0")
	Kscore string `yaml:"kscore,omitempty" json:"kscore,omitempty"`

	// Modules lists required modules with version constraints
	Modules []string `yaml:"modules,omitempty" json:"modules,omitempty"`

	// Platforms lists supported platforms
	Platforms []Platform `yaml:"platforms,omitempty" json:"platforms,omitempty"`
}

// Platform defines a supported platform.
type Platform struct {
	// OS is the operating system (e.g., "linux", "windows", "darwin")
	OS string `yaml:"os" json:"os"`

	// Family is the OS family (e.g., "debian", "rhel", "alpine")
	Family string `yaml:"family,omitempty" json:"family,omitempty"`

	// Versions lists supported versions
	Versions []string `yaml:"versions,omitempty" json:"versions,omitempty"`

	// Arch is the CPU architecture (e.g., "amd64", "arm64")
	Arch string `yaml:"arch,omitempty" json:"arch,omitempty"`
}

// Dependencies defines blueprint dependencies.
type Dependencies struct {
	// Requires lists soft dependencies (can run concurrently)
	Requires []string `yaml:"requires,omitempty" json:"requires,omitempty"`

	// RequiresBefore lists hard dependencies (must complete first)
	RequiresBefore []string `yaml:"requires_before,omitempty" json:"requires_before,omitempty"`
}

// Feature defines an optional feature flag.
type Feature struct {
	// Description of the feature
	Description string `yaml:"description,omitempty" json:"description,omitempty"`

	// Default value for the feature
	Default bool `yaml:"default,omitempty" json:"default,omitempty"`

	// Enables lists states to include when feature is enabled
	Enables []string `yaml:"enables,omitempty" json:"enables,omitempty"`

	// Requires lists blueprints required when feature is enabled
	Requires []string `yaml:"requires,omitempty" json:"requires,omitempty"`

	// Parameters lists parameters only valid when feature is enabled
	Parameters []string `yaml:"parameters,omitempty" json:"parameters,omitempty"`
}

// ParameterSchema defines the schema for a parameter using JSON Schema-like syntax.
type ParameterSchema struct {
	// Type is the parameter type: string, integer, number, boolean, array, object
	Type string `yaml:"type" json:"type"`

	// Description explains the parameter's purpose
	Description string `yaml:"description,omitempty" json:"description,omitempty"`

	// Required indicates if the parameter must be provided
	Required bool `yaml:"required,omitempty" json:"required,omitempty"`

	// Default is the default value
	Default interface{} `yaml:"default,omitempty" json:"default,omitempty"`

	// Sensitive indicates the value should not be logged
	Sensitive bool `yaml:"sensitive,omitempty" json:"sensitive,omitempty"`

	// Source indicates where the value should come from (e.g., "secret")
	Source string `yaml:"source,omitempty" json:"source,omitempty"`

	// Feature restricts this parameter to when a feature is enabled
	Feature string `yaml:"feature,omitempty" json:"feature,omitempty"`

	// RequiredIf specifies conditions under which this parameter becomes required.
	// Each element is a map of parameter name to expected value.
	// The parameter is required if ANY condition matches (OR logic).
	// Example: required_if: [{ssl_provider: custom}] means this parameter
	// is required when ssl_provider equals "custom".
	RequiredIf []map[string]interface{} `yaml:"required_if,omitempty" json:"required_if,omitempty"`

	// Examples shows example values
	Examples []interface{} `yaml:"examples,omitempty" json:"examples,omitempty"`

	// --- String validation ---

	// Pattern is a regex pattern for string validation
	Pattern string `yaml:"pattern,omitempty" json:"pattern,omitempty"`

	// Format is a predefined format (e.g., "hostname", "uri", "email")
	Format string `yaml:"format,omitempty" json:"format,omitempty"`

	// MinLength is the minimum string length
	MinLength *int `yaml:"minLength,omitempty" json:"minLength,omitempty"`

	// MaxLength is the maximum string length
	MaxLength *int `yaml:"maxLength,omitempty" json:"maxLength,omitempty"`

	// --- Numeric validation ---

	// Minimum is the minimum numeric value
	Minimum *float64 `yaml:"minimum,omitempty" json:"minimum,omitempty"`

	// Maximum is the maximum numeric value
	Maximum *float64 `yaml:"maximum,omitempty" json:"maximum,omitempty"`

	// --- Enum validation ---

	// Enum lists allowed values
	Enum []interface{} `yaml:"enum,omitempty" json:"enum,omitempty"`

	// --- Array validation ---

	// Items defines the schema for array items
	Items *ParameterSchema `yaml:"items,omitempty" json:"items,omitempty"`

	// MinItems is the minimum array length
	MinItems *int `yaml:"minItems,omitempty" json:"minItems,omitempty"`

	// MaxItems is the maximum array length
	MaxItems *int `yaml:"maxItems,omitempty" json:"maxItems,omitempty"`

	// --- Object validation ---

	// Properties defines nested property schemas for objects
	Properties map[string]ParameterSchema `yaml:"properties,omitempty" json:"properties,omitempty"`

	// AdditionalProperties controls additional properties for objects
	AdditionalProperties *ParameterSchema `yaml:"additionalProperties,omitempty" json:"additionalProperties,omitempty"`
}

// Output defines a value exposed after blueprint application.
type Output struct {
	// Description explains the output's purpose
	Description string `yaml:"description,omitempty" json:"description,omitempty"`

	// Value is a template for the output value
	Value string `yaml:"value" json:"value"`

	// Sensitive indicates the value should not be logged
	Sensitive bool `yaml:"sensitive,omitempty" json:"sensitive,omitempty"`
}

// Hooks defines lifecycle hooks.
type Hooks struct {
	// PreApply runs before applying the blueprint
	PreApply []string `yaml:"pre_apply,omitempty" json:"pre_apply,omitempty"`

	// PostApply runs after applying the blueprint
	PostApply []string `yaml:"post_apply,omitempty" json:"post_apply,omitempty"`

	// PreRollback runs before rolling back
	PreRollback []string `yaml:"pre_rollback,omitempty" json:"pre_rollback,omitempty"`

	// PostRollback runs after rolling back
	PostRollback []string `yaml:"post_rollback,omitempty" json:"post_rollback,omitempty"`
}

// Reference represents a reference to a blueprint with version constraint.
type Reference struct {
	// Name is the blueprint name (e.g., "blueprints/community/web-app-stack")
	Name string `yaml:"blueprint" json:"blueprint"`

	// Version is the version constraint (e.g., "^1.2")
	Version string `yaml:"version,omitempty" json:"version,omitempty"`

	// As is the namespace for this instance when using multiple instances
	As string `yaml:"as,omitempty" json:"as,omitempty"`

	// Entrypoint specifies a specific entry point to use
	Entrypoint string `yaml:"entrypoint,omitempty" json:"entrypoint,omitempty"`

	// Features enables/disables optional features
	Features map[string]bool `yaml:"features,omitempty" json:"features,omitempty"`

	// Parameters are the values for the blueprint's parameters
	Parameters map[string]interface{} `yaml:"parameters,omitempty" json:"parameters,omitempty"`
}

// ParseManifest parses a blueprint manifest from YAML bytes.
func ParseManifest(data []byte) (*Blueprint, error) {
	var bp Blueprint
	if err := yaml.Unmarshal(data, &bp); err != nil {
		return nil, fmt.Errorf("failed to parse blueprint manifest: %w", err)
	}
	return &bp, nil
}

// ParseManifestFile parses a blueprint manifest from a file.
func ParseManifestFile(path string) (*Blueprint, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read blueprint manifest file: %w", err)
	}

	bp, err := ParseManifest(data)
	if err != nil {
		return nil, err
	}

	// Set the source path to the directory containing the manifest
	bp.SourcePath = filepath.Dir(path)

	return bp, nil
}

// FullName returns the full blueprint name with vendor (e.g., "blueprints/community/web-app-stack").
func (b *Blueprint) FullName() string {
	// If SourcePath contains a vendor path, extract it
	// Expected format: .../blueprints/<vendor>/<name>/
	if b.SourcePath != "" {
		parts := strings.Split(filepath.ToSlash(b.SourcePath), "/")
		for i, part := range parts {
			if part == "blueprints" && i+2 < len(parts) {
				return fmt.Sprintf("blueprints/%s/%s", parts[i+1], b.Metadata.Name)
			}
		}
	}
	return fmt.Sprintf("blueprints/%s", b.Metadata.Name)
}

// DefaultEntrypoint returns the default entry point path.
func (b *Blueprint) DefaultEntrypoint() string {
	if ep, ok := b.Entrypoints["default"]; ok {
		return ep
	}
	return "states/init.yaml"
}

// GetEntrypoint returns the specified entry point path or default if not found.
func (b *Blueprint) GetEntrypoint(name string) string {
	if name == "" || name == "default" {
		return b.DefaultEntrypoint()
	}
	if ep, ok := b.Entrypoints[name]; ok {
		return ep
	}
	return ""
}

// HasFeature checks if a feature is defined in the blueprint.
func (b *Blueprint) HasFeature(name string) bool {
	_, ok := b.Features[name]
	return ok
}

// IsFeatureEnabled checks if a feature is enabled by default.
func (b *Blueprint) IsFeatureEnabled(name string) bool {
	if feature, ok := b.Features[name]; ok {
		return feature.Default
	}
	return false
}

// GetParameter returns a parameter schema by name, supporting dot notation for nested params.
func (b *Blueprint) GetParameter(name string) (*ParameterSchema, bool) {
	parts := strings.Split(name, ".")
	if len(parts) == 0 {
		return nil, false
	}

	// Get the root parameter
	param, ok := b.Parameters[parts[0]]
	if !ok {
		return nil, false
	}

	// Navigate to nested parameters
	for _, part := range parts[1:] {
		if param.Properties == nil {
			return nil, false
		}
		nested, ok := param.Properties[part]
		if !ok {
			return nil, false
		}
		param = nested
	}

	return &param, true
}

// RequiredParameters returns a list of required parameter names.
func (b *Blueprint) RequiredParameters() []string {
	var required []string
	collectRequired("", b.Parameters, &required)
	return required
}

// collectRequired recursively collects required parameter names.
func collectRequired(prefix string, params map[string]ParameterSchema, required *[]string) {
	for name := range params {
		schema := params[name]
		fullName := name
		if prefix != "" {
			fullName = prefix + "." + name
		}

		if schema.Required {
			*required = append(*required, fullName)
		}

		// Check nested properties
		if schema.Properties != nil {
			collectRequired(fullName, schema.Properties, required)
		}
	}
}

// SensitiveParameters returns a list of sensitive parameter names.
func (b *Blueprint) SensitiveParameters() []string {
	var sensitive []string
	collectSensitive("", b.Parameters, &sensitive)
	return sensitive
}

// collectSensitive recursively collects sensitive parameter names.
func collectSensitive(prefix string, params map[string]ParameterSchema, sensitive *[]string) {
	for name := range params {
		schema := params[name]
		fullName := name
		if prefix != "" {
			fullName = prefix + "." + name
		}

		if schema.Sensitive {
			*sensitive = append(*sensitive, fullName)
		}

		// Check nested properties
		if schema.Properties != nil {
			collectSensitive(fullName, schema.Properties, sensitive)
		}
	}
}

// Marshal serializes the blueprint to YAML.
func (b *Blueprint) Marshal() ([]byte, error) {
	return yaml.Marshal(b)
}
