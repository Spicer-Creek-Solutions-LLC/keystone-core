package blueprint

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"text/template"

	"gopkg.in/yaml.v3"
)

// Loader loads and processes blueprints from storage.
type Loader struct {
	storage   Storage
	validator *Validator
	cache     map[string]*Blueprint
}

// NewLoader creates a new blueprint loader.
func NewLoader(storage Storage) *Loader {
	return &Loader{
		storage:   storage,
		validator: NewValidator(),
		cache:     make(map[string]*Blueprint),
	}
}

// LoadConfig holds the configuration for loading a blueprint.
type LoadConfig struct {
	// Name is the blueprint name (e.g., "blueprints/community/web-app-stack")
	Name string

	// Version is the version constraint (optional)
	Version string

	// Parameters are the parameter values
	Parameters map[string]interface{}

	// Features overrides for enabled features
	Features map[string]bool

	// Entrypoint specifies a specific entry point (optional)
	Entrypoint string

	// As is the namespace for this instance (for multiple instances)
	As string

	// Validate whether to validate parameters
	Validate bool
}

// LoadResult holds the result of loading a blueprint.
type LoadResult struct {
	// Blueprint is the loaded blueprint
	Blueprint *Blueprint

	// ResolvedVersion is the actual version loaded
	ResolvedVersion string

	// EnabledFeatures lists which features are enabled
	EnabledFeatures []string

	// ResolvedParameters are the final parameter values with defaults applied
	ResolvedParameters map[string]interface{}

	// StateFiles lists the state files to apply
	StateFiles []string

	// ValidationResult contains validation results
	ValidationResult *ValidationResult
}

// Load loads a blueprint from storage.
func (l *Loader) Load(ctx context.Context, config *LoadConfig) (*LoadResult, error) {
	// Check cache
	cacheKey := fmt.Sprintf("%s@%s", config.Name, config.Version)
	if bp, ok := l.cache[cacheKey]; ok {
		return l.processBlueprint(ctx, bp, config)
	}

	// Load from storage
	bp, err := l.storage.Get(ctx, config.Name, config.Version)
	if err != nil {
		return nil, fmt.Errorf("failed to load blueprint %s: %w", config.Name, err)
	}

	// Cache the blueprint
	l.cache[cacheKey] = bp

	return l.processBlueprint(ctx, bp, config)
}

// LoadFromPath loads a blueprint from a filesystem path.
func (l *Loader) LoadFromPath(ctx context.Context, path string, config *LoadConfig) (*LoadResult, error) {
	// Find blueprint.yaml
	manifestPath := path
	if stat, err := os.Stat(path); err == nil && stat.IsDir() {
		manifestPath = filepath.Join(path, "blueprint.yaml")
	}

	// Parse the manifest
	bp, err := ParseManifestFile(manifestPath)
	if err != nil {
		return nil, fmt.Errorf("failed to parse blueprint: %w", err)
	}

	// Update config name if not set
	if config.Name == "" {
		config.Name = bp.FullName()
	}

	return l.processBlueprint(ctx, bp, config)
}

// processBlueprint validates and processes a loaded blueprint.
func (l *Loader) processBlueprint(ctx context.Context, bp *Blueprint, config *LoadConfig) (*LoadResult, error) {
	result := &LoadResult{
		Blueprint:       bp,
		ResolvedVersion: bp.Metadata.Version,
	}

	// Validate the blueprint
	result.ValidationResult = l.validator.Validate(bp)
	if !result.ValidationResult.Valid {
		return result, result.ValidationResult.Error()
	}

	// Determine enabled features
	result.EnabledFeatures = l.resolveFeatures(bp, config.Features)

	// Resolve parameters with defaults
	resolved, err := l.resolveParameters(bp, config.Parameters, result.EnabledFeatures)
	if err != nil {
		return result, fmt.Errorf("parameter resolution failed: %w", err)
	}
	result.ResolvedParameters = resolved

	// Validate parameters if requested
	if config.Validate {
		if err := l.validateParameters(bp, resolved, result.EnabledFeatures); err != nil {
			result.ValidationResult.AddError("parameters", err.Error(), nil)
			return result, err
		}
	}

	// Determine state files to apply
	stateFiles, err := l.resolveStateFiles(bp, config.Entrypoint, result.EnabledFeatures)
	if err != nil {
		return result, fmt.Errorf("failed to resolve state files: %w", err)
	}
	result.StateFiles = stateFiles

	return result, nil
}

// resolveFeatures determines which features are enabled.
func (l *Loader) resolveFeatures(bp *Blueprint, overrides map[string]bool) []string {
	var enabled []string

	for name, feature := range bp.Features {
		// Check override first
		if override, ok := overrides[name]; ok {
			if override {
				enabled = append(enabled, name)
			}
			continue
		}

		// Use default
		if feature.Default {
			enabled = append(enabled, name)
		}
	}

	return enabled
}

// resolveParameters merges user parameters with defaults.
func (l *Loader) resolveParameters(bp *Blueprint, userParams map[string]interface{}, enabledFeatures []string) (map[string]interface{}, error) {
	result := make(map[string]interface{})

	// Apply defaults recursively
	l.applyDefaults("", bp.Parameters, result, enabledFeatures)

	// Merge user parameters
	if userParams != nil {
		mergeParameters(result, userParams)
	}

	return result, nil
}

// applyDefaults recursively applies default values.
func (l *Loader) applyDefaults(prefix string, schemas map[string]ParameterSchema, result map[string]interface{}, enabledFeatures []string) {
	for name, schema := range schemas {
		// Skip feature-gated parameters if feature not enabled
		if schema.Feature != "" && !containsString(enabledFeatures, schema.Feature) {
			continue
		}

		if schema.Default != nil {
			setNestedValue(result, name, schema.Default)
		}

		// Recurse into object properties
		if schema.Type == "object" && schema.Properties != nil {
			nested := make(map[string]interface{})
			l.applyDefaults(name+".", schema.Properties, nested, enabledFeatures)
			if len(nested) > 0 {
				current, _ := result[name].(map[string]interface{})
				if current == nil {
					current = make(map[string]interface{})
				}
				mergeParameters(current, nested)
				result[name] = current
			}
		}
	}
}

// validateParameters validates parameter values against schemas.
func (l *Loader) validateParameters(bp *Blueprint, params map[string]interface{}, enabledFeatures []string) error {
	return l.validateParameterValues("", bp.Parameters, params, enabledFeatures)
}

// validateParameterValues recursively validates parameter values.
func (l *Loader) validateParameterValues(prefix string, schemas map[string]ParameterSchema, values map[string]interface{}, enabledFeatures []string) error {
	for name, schema := range schemas {
		fullName := name
		if prefix != "" {
			fullName = prefix + "." + name
		}

		// Skip feature-gated parameters if feature not enabled
		if schema.Feature != "" && !containsString(enabledFeatures, schema.Feature) {
			continue
		}

		value, exists := values[name]

		// Check required
		if schema.Required && !exists {
			return fmt.Errorf("required parameter missing: %s", fullName)
		}

		if !exists {
			continue
		}

		// Type validation
		if err := validateType(fullName, schema.Type, value); err != nil {
			return err
		}

		// Enum validation
		if len(schema.Enum) > 0 {
			if !containsValue(schema.Enum, value) {
				return fmt.Errorf("parameter %s: value %v not in allowed values %v", fullName, value, schema.Enum)
			}
		}

		// String validation
		if schema.Type == "string" {
			strVal, _ := value.(string)
			if schema.Pattern != "" {
				if matched, _ := regexp.MatchString(schema.Pattern, strVal); !matched {
					return fmt.Errorf("parameter %s: value does not match pattern %s", fullName, schema.Pattern)
				}
			}
			if schema.MinLength != nil && len(strVal) < *schema.MinLength {
				return fmt.Errorf("parameter %s: string length %d is less than minimum %d", fullName, len(strVal), *schema.MinLength)
			}
			if schema.MaxLength != nil && len(strVal) > *schema.MaxLength {
				return fmt.Errorf("parameter %s: string length %d exceeds maximum %d", fullName, len(strVal), *schema.MaxLength)
			}
		}

		// Numeric validation
		if schema.Type == "integer" || schema.Type == "number" {
			numVal := toFloat64(value)
			if schema.Minimum != nil && numVal < *schema.Minimum {
				return fmt.Errorf("parameter %s: value %v is less than minimum %v", fullName, value, *schema.Minimum)
			}
			if schema.Maximum != nil && numVal > *schema.Maximum {
				return fmt.Errorf("parameter %s: value %v exceeds maximum %v", fullName, value, *schema.Maximum)
			}
		}

		// Array validation
		if schema.Type == "array" {
			arrVal, ok := value.([]interface{})
			if ok {
				if schema.MinItems != nil && len(arrVal) < *schema.MinItems {
					return fmt.Errorf("parameter %s: array length %d is less than minimum %d", fullName, len(arrVal), *schema.MinItems)
				}
				if schema.MaxItems != nil && len(arrVal) > *schema.MaxItems {
					return fmt.Errorf("parameter %s: array length %d exceeds maximum %d", fullName, len(arrVal), *schema.MaxItems)
				}
			}
		}

		// Recurse into objects
		if schema.Type == "object" && schema.Properties != nil {
			nested, ok := value.(map[string]interface{})
			if ok {
				if err := l.validateParameterValues(fullName, schema.Properties, nested, enabledFeatures); err != nil {
					return err
				}
			}
		}
	}

	return nil
}

// resolveStateFiles determines which state files to apply.
func (l *Loader) resolveStateFiles(bp *Blueprint, entrypoint string, enabledFeatures []string) ([]string, error) {
	var stateFiles []string

	// Get the entry point
	ep := bp.GetEntrypoint(entrypoint)
	if ep == "" {
		return nil, fmt.Errorf("entry point not found: %s", entrypoint)
	}
	stateFiles = append(stateFiles, ep)

	// Add feature-enabled states
	for _, featureName := range enabledFeatures {
		feature, ok := bp.Features[featureName]
		if !ok {
			continue
		}
		stateFiles = append(stateFiles, feature.Enables...)
	}

	return stateFiles, nil
}

// RenderState renders a state file with parameter substitution.
func (l *Loader) RenderState(ctx context.Context, bp *Blueprint, statePath string, params map[string]interface{}) ([]byte, error) {
	// Get the state file
	var reader io.ReadCloser
	var err error

	// Try storage first
	if fs, ok := l.storage.(FileStorage); ok {
		reader, err = fs.GetFile(ctx, bp.FullName(), bp.Metadata.Version, statePath)
	}

	// Fall back to source path
	if err != nil && bp.SourcePath != "" {
		fullPath := filepath.Join(bp.SourcePath, statePath)
		reader, err = os.Open(fullPath)
	}

	if err != nil {
		return nil, fmt.Errorf("failed to open state file %s: %w", statePath, err)
	}
	defer reader.Close()

	data, err := io.ReadAll(reader)
	if err != nil {
		return nil, fmt.Errorf("failed to read state file: %w", err)
	}

	// Render template
	rendered, err := l.renderTemplate(string(data), params, bp)
	if err != nil {
		return nil, fmt.Errorf("failed to render state file %s: %w", statePath, err)
	}

	return []byte(rendered), nil
}

// renderTemplate renders a template string with parameters.
func (l *Loader) renderTemplate(content string, params map[string]interface{}, bp *Blueprint) (string, error) {
	// Create template with custom functions
	tmpl, err := template.New("state").Funcs(templateFuncs()).Parse(content)
	if err != nil {
		return "", fmt.Errorf("template parse error: %w", err)
	}

	// Create context with params and blueprint metadata
	ctx := map[string]interface{}{
		"params":    params,
		"blueprint": bp,
	}

	// Also flatten params to top level for simpler access
	for k, v := range params {
		ctx[k] = v
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, ctx); err != nil {
		return "", fmt.Errorf("template execution error: %w", err)
	}

	return buf.String(), nil
}

// LoadStateWithBlueprint parses a state file that may include blueprints.
func (l *Loader) LoadStateWithBlueprint(ctx context.Context, stateData []byte) (*StateWithBlueprints, error) {
	var state StateWithBlueprints
	if err := yaml.Unmarshal(stateData, &state); err != nil {
		return nil, fmt.Errorf("failed to parse state file: %w", err)
	}

	return &state, nil
}

// StateWithBlueprints represents a state file that can include blueprints.
type StateWithBlueprints struct {
	// Include lists blueprints and other files to include
	Include []BlueprintInclude `yaml:"include,omitempty"`

	// States contains the state declarations
	States map[string]interface{} `yaml:"states,omitempty"`
}

// BlueprintInclude represents a blueprint include directive.
type BlueprintInclude struct {
	// Blueprint is the blueprint reference (e.g., "blueprints/community/web-app-stack")
	Blueprint string `yaml:"blueprint,omitempty"`

	// Version is the version constraint
	Version string `yaml:"version,omitempty"`

	// As is the namespace for this instance
	As string `yaml:"as,omitempty"`

	// Entrypoint specifies a specific entry point
	Entrypoint string `yaml:"entrypoint,omitempty"`

	// Features enables/disables optional features
	Features map[string]bool `yaml:"features,omitempty"`

	// Parameters are the values for the blueprint's parameters
	Parameters map[string]interface{} `yaml:"parameters,omitempty"`

	// File is a regular state file to include (mutually exclusive with Blueprint)
	File string `yaml:"file,omitempty"`
}

// IsBlueprint returns true if this is a blueprint include.
func (i *BlueprintInclude) IsBlueprint() bool {
	return i.Blueprint != ""
}

// ToLoadConfig converts the include to a LoadConfig.
func (i *BlueprintInclude) ToLoadConfig() *LoadConfig {
	return &LoadConfig{
		Name:       i.Blueprint,
		Version:    i.Version,
		Parameters: i.Parameters,
		Features:   i.Features,
		Entrypoint: i.Entrypoint,
		As:         i.As,
		Validate:   true,
	}
}

// Helper functions

func mergeParameters(dst, src map[string]interface{}) {
	for k, v := range src {
		if dstMap, ok := dst[k].(map[string]interface{}); ok {
			if srcMap, ok := v.(map[string]interface{}); ok {
				mergeParameters(dstMap, srcMap)
				continue
			}
		}
		dst[k] = v
	}
}

func setNestedValue(m map[string]interface{}, key string, value interface{}) {
	parts := strings.Split(key, ".")
	current := m

	for i, part := range parts {
		if i == len(parts)-1 {
			current[part] = value
			return
		}

		next, ok := current[part].(map[string]interface{})
		if !ok {
			next = make(map[string]interface{})
			current[part] = next
		}
		current = next
	}
}

func containsString(slice []string, s string) bool {
	for _, item := range slice {
		if item == s {
			return true
		}
	}
	return false
}

func containsValue(slice []interface{}, value interface{}) bool {
	for _, item := range slice {
		if item == value {
			return true
		}
	}
	return false
}

func validateType(name, expectedType string, value interface{}) error {
	switch expectedType {
	case "string":
		if _, ok := value.(string); !ok {
			return fmt.Errorf("parameter %s: expected string, got %T", name, value)
		}
	case "integer":
		switch value.(type) {
		case int, int32, int64, float64:
			// Accept numeric types
		default:
			return fmt.Errorf("parameter %s: expected integer, got %T", name, value)
		}
	case "number":
		switch value.(type) {
		case int, int32, int64, float64:
			// Accept numeric types
		default:
			return fmt.Errorf("parameter %s: expected number, got %T", name, value)
		}
	case "boolean":
		if _, ok := value.(bool); !ok {
			return fmt.Errorf("parameter %s: expected boolean, got %T", name, value)
		}
	case "array":
		if _, ok := value.([]interface{}); !ok {
			return fmt.Errorf("parameter %s: expected array, got %T", name, value)
		}
	case "object":
		if _, ok := value.(map[string]interface{}); !ok {
			return fmt.Errorf("parameter %s: expected object, got %T", name, value)
		}
	}
	return nil
}

func toFloat64(value interface{}) float64 {
	switch v := value.(type) {
	case int:
		return float64(v)
	case int32:
		return float64(v)
	case int64:
		return float64(v)
	case float64:
		return v
	case float32:
		return float64(v)
	default:
		return 0
	}
}

func templateFuncs() template.FuncMap {
	return template.FuncMap{
		"default": func(def, val interface{}) interface{} {
			if val == nil || val == "" {
				return def
			}
			return val
		},
		"required": func(val interface{}) interface{} {
			if val == nil || val == "" {
				panic("required value is missing")
			}
			return val
		},
		"quote": func(val interface{}) string {
			return fmt.Sprintf("%q", val)
		},
		"upper": strings.ToUpper,
		"lower": strings.ToLower,
		"title": strings.Title,
		"trim":  strings.TrimSpace,
		"join": func(sep string, items []string) string {
			return strings.Join(items, sep)
		},
		"split": func(sep, s string) []string {
			return strings.Split(s, sep)
		},
		"contains": strings.Contains,
		"replace": func(old, new, s string) string {
			return strings.ReplaceAll(s, old, new)
		},
		"indent": func(spaces int, s string) string {
			pad := strings.Repeat(" ", spaces)
			return pad + strings.ReplaceAll(s, "\n", "\n"+pad)
		},
		"nindent": func(spaces int, s string) string {
			pad := strings.Repeat(" ", spaces)
			return "\n" + pad + strings.ReplaceAll(s, "\n", "\n"+pad)
		},
		"toYaml": func(v interface{}) string {
			data, err := yaml.Marshal(v)
			if err != nil {
				return ""
			}
			return strings.TrimSuffix(string(data), "\n")
		},
	}
}
