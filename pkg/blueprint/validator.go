package blueprint

import (
	"fmt"
	"regexp"
	"strings"
)

// ValidationResult holds the results of blueprint validation.
type ValidationResult struct {
	// Valid is true if the blueprint is valid
	Valid bool

	// Errors contains validation errors
	Errors []ValidationError

	// Warnings contains non-fatal validation warnings
	Warnings []ValidationError
}

// AddError adds a validation error.
func (r *ValidationResult) AddError(field, message string, value interface{}) {
	r.Errors = append(r.Errors, ValidationError{Parameter: field, Message: message, Value: value})
	r.Valid = false
}

// AddWarning adds a validation warning.
func (r *ValidationResult) AddWarning(field, message string, value interface{}) {
	r.Warnings = append(r.Warnings, ValidationError{Parameter: field, Message: message, Value: value})
}

// Error returns a combined error message or nil if valid.
func (r *ValidationResult) Error() error {
	if r.Valid {
		return nil
	}
	var msgs []string
	for _, err := range r.Errors {
		msgs = append(msgs, err.Error())
	}
	return fmt.Errorf("blueprint validation failed:\n  - %s", strings.Join(msgs, "\n  - "))
}

// Validator validates blueprint manifests.
type Validator struct {
	// StrictMode enables additional strict validation
	StrictMode bool
}

// NewValidator creates a new blueprint validator.
func NewValidator() *Validator {
	return &Validator{}
}

// Validate validates a blueprint manifest.
func (v *Validator) Validate(bp *Blueprint) *ValidationResult {
	result := &ValidationResult{Valid: true}

	// Validate API version
	v.validateAPIVersion(bp, result)

	// Validate kind
	v.validateKind(bp, result)

	// Validate metadata
	v.validateMetadata(bp, result)

	// Validate compatibility
	if bp.Compatibility != nil {
		v.validateCompatibility(bp.Compatibility, result)
	}

	// Validate dependencies
	if bp.Dependencies != nil {
		v.validateDependencies(bp.Dependencies, result)
	}

	// Validate features
	v.validateFeatures(bp, result)

	// Validate entrypoints
	v.validateEntrypoints(bp, result)

	// Validate parameters
	v.validateParameters(bp, result)

	// Validate outputs
	v.validateOutputs(bp, result)

	// Validate hooks
	if bp.Hooks != nil {
		v.validateHooks(bp.Hooks, result)
	}

	return result
}

// validateAPIVersion validates the API version field.
func (v *Validator) validateAPIVersion(bp *Blueprint, result *ValidationResult) {
	if bp.APIVersion == "" {
		result.AddError("apiVersion", "required field is missing", nil)
		return
	}

	if bp.APIVersion != APIVersion {
		result.AddWarning("apiVersion", fmt.Sprintf("unknown API version, expected %s", APIVersion), bp.APIVersion)
	}
}

// validateKind validates the kind field.
func (v *Validator) validateKind(bp *Blueprint, result *ValidationResult) {
	if bp.Kind == "" {
		result.AddError("kind", "required field is missing", nil)
		return
	}

	if bp.Kind != Kind {
		result.AddError("kind", fmt.Sprintf("must be '%s'", Kind), bp.Kind)
	}
}

// validateMetadata validates the metadata section.
func (v *Validator) validateMetadata(bp *Blueprint, result *ValidationResult) {
	if bp.Metadata.Name == "" {
		result.AddError("metadata.name", "required field is missing", nil)
	} else if !isValidBlueprintName(bp.Metadata.Name) {
		result.AddError("metadata.name", "must be lowercase alphanumeric with hyphens", bp.Metadata.Name)
	}

	if bp.Metadata.Version == "" {
		result.AddError("metadata.version", "required field is missing", nil)
	} else if !isValidSemVer(bp.Metadata.Version) {
		result.AddError("metadata.version", "must be a valid semantic version", bp.Metadata.Version)
	}

	// Validate maintainers
	for i, maintainer := range bp.Metadata.Maintainers {
		if maintainer.Name == "" {
			result.AddError(fmt.Sprintf("metadata.maintainers[%d].name", i), "required field is missing", nil)
		}
		if maintainer.Email != "" && !isValidEmail(maintainer.Email) {
			result.AddWarning(fmt.Sprintf("metadata.maintainers[%d].email", i), "invalid email format", maintainer.Email)
		}
	}

	// Validate license
	if bp.Metadata.License != "" && v.StrictMode {
		if !isValidSPDXLicense(bp.Metadata.License) {
			result.AddWarning("metadata.license", "not a recognized SPDX license identifier", bp.Metadata.License)
		}
	}
}

// validateCompatibility validates the compatibility section.
func (v *Validator) validateCompatibility(compat *Compatibility, result *ValidationResult) {
	// Validate Kscore version constraint
	if compat.Kscore != "" && !isValidVersionConstraint(compat.Kscore) {
		result.AddError("compatibility.kscore", "invalid version constraint", compat.Kscore)
	}

	// Validate module references
	for i, mod := range compat.Modules {
		if !isValidModuleReference(mod) {
			result.AddError(fmt.Sprintf("compatibility.modules[%d]", i), "invalid module reference format", mod)
		}
	}

	// Validate platforms
	for i, platform := range compat.Platforms {
		if platform.OS == "" {
			result.AddError(fmt.Sprintf("compatibility.platforms[%d].os", i), "required field is missing", nil)
		} else if !isValidOS(platform.OS) {
			result.AddWarning(fmt.Sprintf("compatibility.platforms[%d].os", i), "unknown OS value", platform.OS)
		}
	}
}

// validateDependencies validates the dependencies section.
func (v *Validator) validateDependencies(deps *Dependencies, result *ValidationResult) {
	for i, dep := range deps.Requires {
		if !isValidBlueprintReference(dep) {
			result.AddError(fmt.Sprintf("dependencies.requires[%d]", i), "invalid blueprint reference format", dep)
		}
	}

	for i, dep := range deps.RequiresBefore {
		if !isValidBlueprintReference(dep) {
			result.AddError(fmt.Sprintf("dependencies.requires_before[%d]", i), "invalid blueprint reference format", dep)
		}
	}

	// Check for duplicates
	allDeps := make(map[string]bool)
	for _, dep := range deps.Requires {
		name := extractBlueprintName(dep)
		if allDeps[name] {
			result.AddWarning("dependencies", "duplicate dependency", dep)
		}
		allDeps[name] = true
	}
	for _, dep := range deps.RequiresBefore {
		name := extractBlueprintName(dep)
		if allDeps[name] {
			result.AddWarning("dependencies", "duplicate dependency", dep)
		}
		allDeps[name] = true
	}
}

// validateFeatures validates the features section.
func (v *Validator) validateFeatures(bp *Blueprint, result *ValidationResult) {
	for name, feature := range bp.Features {
		prefix := fmt.Sprintf("features.%s", name)

		// Validate enables paths
		for i, path := range feature.Enables {
			if path == "" {
				result.AddError(fmt.Sprintf("%s.enables[%d]", prefix, i), "empty path", nil)
			}
		}

		// Validate requires references
		for i, req := range feature.Requires {
			if !isValidBlueprintReference(req) {
				result.AddError(fmt.Sprintf("%s.requires[%d]", prefix, i), "invalid blueprint reference format", req)
			}
		}

		// Validate parameter references
		for i, param := range feature.Parameters {
			// Parameter wildcards like "monitoring.*" are valid
			if param == "" {
				result.AddError(fmt.Sprintf("%s.parameters[%d]", prefix, i), "empty parameter reference", nil)
			}
		}
	}
}

// validateEntrypoints validates the entrypoints section.
func (v *Validator) validateEntrypoints(bp *Blueprint, result *ValidationResult) {
	if bp.Entrypoints == nil {
		return
	}

	for name, path := range bp.Entrypoints {
		if path == "" {
			result.AddError(fmt.Sprintf("entrypoints.%s", name), "empty path", nil)
		}
		if !strings.HasSuffix(path, ".yaml") && !strings.HasSuffix(path, ".yml") {
			result.AddWarning(fmt.Sprintf("entrypoints.%s", name), "path should end with .yaml or .yml", path)
		}
	}

	// Check for default entrypoint
	if _, ok := bp.Entrypoints["default"]; !ok && v.StrictMode {
		result.AddWarning("entrypoints", "missing 'default' entrypoint", nil)
	}
}

// validateParameters validates the parameters section.
func (v *Validator) validateParameters(bp *Blueprint, result *ValidationResult) {
	v.validateParameterSchemas("parameters", bp.Parameters, bp, result)
}

// validateParameterSchemas recursively validates parameter schemas.
func (v *Validator) validateParameterSchemas(prefix string, params map[string]ParameterSchema, bp *Blueprint, result *ValidationResult) {
	for name, schema := range params {
		path := fmt.Sprintf("%s.%s", prefix, name)
		v.validateParameterSchema(path, &schema, bp, result)
	}
}

// validateParameterSchema validates a single parameter schema.
func (v *Validator) validateParameterSchema(path string, schema *ParameterSchema, bp *Blueprint, result *ValidationResult) {
	// Validate type
	validTypes := map[string]bool{
		"string":  true,
		"integer": true,
		"number":  true,
		"boolean": true,
		"array":   true,
		"object":  true,
	}
	if schema.Type == "" {
		result.AddError(path+".type", "required field is missing", nil)
	} else if !validTypes[schema.Type] {
		result.AddError(path+".type", "invalid type", schema.Type)
	}

	// Validate feature reference
	if schema.Feature != "" && !bp.HasFeature(schema.Feature) {
		result.AddError(path+".feature", "references non-existent feature", schema.Feature)
	}

	// Validate source
	if schema.Source != "" {
		validSources := map[string]bool{"secret": true, "env": true, "file": true}
		if !validSources[schema.Source] {
			result.AddWarning(path+".source", "unknown source type", schema.Source)
		}
	}

	// Type-specific validation
	switch schema.Type {
	case "string":
		v.validateStringSchema(path, schema, result)
	case "integer", "number":
		v.validateNumericSchema(path, schema, result)
	case "array":
		v.validateArraySchema(path, schema, bp, result)
	case "object":
		v.validateObjectSchema(path, schema, bp, result)
	}

	// Validate enum values match type
	if len(schema.Enum) > 0 {
		v.validateEnumValues(path, schema, result)
	}

	// Validate default value matches schema
	if schema.Default != nil {
		v.validateDefaultValue(path, schema, result)
	}
}

// validateStringSchema validates string-specific schema properties.
func (v *Validator) validateStringSchema(path string, schema *ParameterSchema, result *ValidationResult) {
	if schema.Pattern != "" {
		if _, err := regexp.Compile(schema.Pattern); err != nil {
			result.AddError(path+".pattern", "invalid regex pattern", schema.Pattern)
		}
	}

	if schema.MinLength != nil && schema.MaxLength != nil {
		if *schema.MinLength > *schema.MaxLength {
			result.AddError(path, "minLength cannot be greater than maxLength", nil)
		}
	}

	// Validate format
	validFormats := map[string]bool{
		"hostname": true,
		"uri":      true,
		"email":    true,
		"ipv4":     true,
		"ipv6":     true,
		"date":     true,
		"datetime": true,
	}
	if schema.Format != "" && !validFormats[schema.Format] {
		result.AddWarning(path+".format", "unknown format", schema.Format)
	}
}

// validateNumericSchema validates numeric-specific schema properties.
func (v *Validator) validateNumericSchema(path string, schema *ParameterSchema, result *ValidationResult) {
	if schema.Minimum != nil && schema.Maximum != nil {
		if *schema.Minimum > *schema.Maximum {
			result.AddError(path, "minimum cannot be greater than maximum", nil)
		}
	}
}

// validateArraySchema validates array-specific schema properties.
func (v *Validator) validateArraySchema(path string, schema *ParameterSchema, bp *Blueprint, result *ValidationResult) {
	if schema.Items == nil && v.StrictMode {
		result.AddWarning(path, "array type should define items schema", nil)
	}

	if schema.Items != nil {
		v.validateParameterSchema(path+".items", schema.Items, bp, result)
	}

	if schema.MinItems != nil && schema.MaxItems != nil {
		if *schema.MinItems > *schema.MaxItems {
			result.AddError(path, "minItems cannot be greater than maxItems", nil)
		}
	}
}

// validateObjectSchema validates object-specific schema properties.
func (v *Validator) validateObjectSchema(path string, schema *ParameterSchema, bp *Blueprint, result *ValidationResult) {
	if schema.Properties != nil {
		v.validateParameterSchemas(path+".properties", schema.Properties, bp, result)
	}

	if schema.AdditionalProperties != nil {
		v.validateParameterSchema(path+".additionalProperties", schema.AdditionalProperties, bp, result)
	}
}

// validateEnumValues validates enum values match the declared type.
func (v *Validator) validateEnumValues(path string, schema *ParameterSchema, result *ValidationResult) {
	for i, val := range schema.Enum {
		switch schema.Type {
		case "string":
			if _, ok := val.(string); !ok {
				result.AddError(fmt.Sprintf("%s.enum[%d]", path, i), "value must be a string", val)
			}
		case "integer":
			switch val.(type) {
			case int, int32, int64, float64:
				// Accept numeric types (YAML parses integers as various types)
			default:
				result.AddError(fmt.Sprintf("%s.enum[%d]", path, i), "value must be an integer", val)
			}
		case "number":
			switch val.(type) {
			case int, int32, int64, float64:
				// Accept numeric types
			default:
				result.AddError(fmt.Sprintf("%s.enum[%d]", path, i), "value must be a number", val)
			}
		case "boolean":
			if _, ok := val.(bool); !ok {
				result.AddError(fmt.Sprintf("%s.enum[%d]", path, i), "value must be a boolean", val)
			}
		}
	}
}

// validateDefaultValue validates the default value matches the schema.
func (v *Validator) validateDefaultValue(path string, schema *ParameterSchema, result *ValidationResult) {
	val := schema.Default
	switch schema.Type {
	case "string":
		if _, ok := val.(string); !ok {
			result.AddError(path+".default", "default value must be a string", val)
		}
	case "integer":
		switch val.(type) {
		case int, int32, int64, float64:
			// Accept numeric types
		default:
			result.AddError(path+".default", "default value must be an integer", val)
		}
	case "number":
		switch val.(type) {
		case int, int32, int64, float64:
			// Accept numeric types
		default:
			result.AddError(path+".default", "default value must be a number", val)
		}
	case "boolean":
		if _, ok := val.(bool); !ok {
			result.AddError(path+".default", "default value must be a boolean", val)
		}
	case "array":
		if _, ok := val.([]interface{}); !ok {
			// YAML might parse as different slice types
			result.AddWarning(path+".default", "default value should be an array", val)
		}
	case "object":
		if _, ok := val.(map[string]interface{}); !ok {
			result.AddWarning(path+".default", "default value should be an object", val)
		}
	}
}

// validateOutputs validates the outputs section.
func (v *Validator) validateOutputs(bp *Blueprint, result *ValidationResult) {
	for name, output := range bp.Outputs {
		if output.Value == "" {
			result.AddError(fmt.Sprintf("outputs.%s.value", name), "required field is missing", nil)
		}
	}
}

// validateHooks validates the hooks section.
func (v *Validator) validateHooks(hooks *Hooks, result *ValidationResult) {
	validateHookPaths := func(name string, paths []string) {
		for i, path := range paths {
			if path == "" {
				result.AddError(fmt.Sprintf("hooks.%s[%d]", name, i), "empty path", nil)
			}
			if !strings.HasSuffix(path, ".yaml") && !strings.HasSuffix(path, ".yml") {
				result.AddWarning(fmt.Sprintf("hooks.%s[%d]", name, i), "path should end with .yaml or .yml", path)
			}
		}
	}

	validateHookPaths("pre_apply", hooks.PreApply)
	validateHookPaths("post_apply", hooks.PostApply)
	validateHookPaths("pre_rollback", hooks.PreRollback)
	validateHookPaths("post_rollback", hooks.PostRollback)
}

// Helper validation functions

var blueprintNameRegex = regexp.MustCompile(`^[a-z][a-z0-9-]*$`)

func isValidBlueprintName(name string) bool {
	return blueprintNameRegex.MatchString(name)
}

// SemVer regex - simplified version
var semverRegex = regexp.MustCompile(`^(\d+)\.(\d+)\.(\d+)(-[a-zA-Z0-9.-]+)?(\+[a-zA-Z0-9.-]+)?$`)

func isValidSemVer(version string) bool {
	return semverRegex.MatchString(version)
}

var emailRegex = regexp.MustCompile(`^[a-zA-Z0-9._%+-]+@[a-zA-Z0-9.-]+\.[a-zA-Z]{2,}$`)

func isValidEmail(email string) bool {
	return emailRegex.MatchString(email)
}

// Common SPDX license identifiers
var spdxLicenses = map[string]bool{
	"MIT":        true,
	"Apache-2.0": true,
	"GPL-3.0":    true,
	"BSD-3-Clause": true,
	"BSD-2-Clause": true,
	"ISC":        true,
	"MPL-2.0":    true,
	"LGPL-3.0":   true,
	"AGPL-3.0":   true,
	"Unlicense":  true,
	"CC0-1.0":    true,
}

func isValidSPDXLicense(license string) bool {
	return spdxLicenses[license]
}

// Version constraint regex - supports >=, <=, >, <, =, ^, ~, x.y.z
var versionConstraintRegex = regexp.MustCompile(`^([>=<^~]*)(\d+(\.\d+){0,2}|\*)$`)

func isValidVersionConstraint(constraint string) bool {
	return versionConstraintRegex.MatchString(constraint)
}

// Module reference format: modules/vendor/name@version
var moduleRefRegex = regexp.MustCompile(`^modules/[a-z][a-z0-9-]*/[a-z][a-z0-9-]*(@[^@]+)?$`)

func isValidModuleReference(ref string) bool {
	return moduleRefRegex.MatchString(ref)
}

// Blueprint reference format: blueprints/vendor/name@version
var blueprintRefRegex = regexp.MustCompile(`^blueprints/[a-z][a-z0-9-]*/[a-z][a-z0-9-]*(@[^@]+)?$`)

func isValidBlueprintReference(ref string) bool {
	return blueprintRefRegex.MatchString(ref)
}

func extractBlueprintName(ref string) string {
	// Remove version suffix
	if idx := strings.Index(ref, "@"); idx != -1 {
		return ref[:idx]
	}
	return ref
}

var validOSValues = map[string]bool{
	"linux":   true,
	"darwin":  true,
	"windows": true,
	"freebsd": true,
	"openbsd": true,
	"netbsd":  true,
}

func isValidOS(os string) bool {
	return validOSValues[os]
}
