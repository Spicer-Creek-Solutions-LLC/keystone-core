package statemgmt

import (
	"fmt"
	"regexp"
	"strings"
)

// Validator validates state files
type Validator struct {
	// ModuleSchemas defines validation schemas for each module type
	ModuleSchemas map[string]*ModuleSchema
}

// ModuleSchema defines the validation schema for a state module
type ModuleSchema struct {
	// Name of the module
	Name string

	// ValidStates are the valid state values for this module
	ValidStates []string

	// RequiredParameters are parameters that must be present
	RequiredParameters []string

	// OptionalParameters are parameters that may be present
	OptionalParameters []string

	// ParameterValidators validate specific parameters
	ParameterValidators map[string]ParameterValidator
}

// ParameterValidator validates a parameter value
type ParameterValidator func(value interface{}) error

// NewValidator creates a new state validator with default schemas
func NewValidator() *Validator {
	return &Validator{
		ModuleSchemas: getDefaultModuleSchemas(),
	}
}

// Validate validates a state file
func (v *Validator) Validate(stateFile *StateFile) []ValidationError {
	var errors []ValidationError

	// Validate each state declaration
	for module, declarations := range stateFile.States {
		schema, ok := v.ModuleSchemas[module]
		if !ok {
			errors = append(errors, ValidationError{
				Module:  module,
				Message: fmt.Sprintf("unknown module type: %s", module),
			})
			continue
		}

		for _, decl := range declarations {
			declErrors := v.validateDeclaration(&decl, schema)
			errors = append(errors, declErrors...)
		}
	}

	// Validate requisite references
	refErrors := v.validateRequisites(stateFile)
	errors = append(errors, refErrors...)

	return errors
}

// validateDeclaration validates a single state declaration
func (v *Validator) validateDeclaration(decl *StateDeclaration, schema *ModuleSchema) []ValidationError {
	var errors []ValidationError

	// Validate state value
	if !isValidState(decl.State, schema.ValidStates) {
		errors = append(errors, ValidationError{
			StateID: decl.ID,
			Module:  decl.Module,
			Field:   "state",
			Message: fmt.Sprintf("invalid state '%s', must be one of: %s",
				decl.State, strings.Join(schema.ValidStates, ", ")),
		})
	}

	// Check required parameters
	for _, required := range schema.RequiredParameters {
		if _, ok := decl.Parameters[required]; !ok {
			errors = append(errors, ValidationError{
				StateID: decl.ID,
				Module:  decl.Module,
				Field:   required,
				Message: fmt.Sprintf("required parameter '%s' is missing", required),
			})
		}
	}

	// Validate parameters
	for param, value := range decl.Parameters {
		// Check if parameter is known
		if !isKnownParameter(param, schema) {
			errors = append(errors, ValidationError{
				StateID: decl.ID,
				Module:  decl.Module,
				Field:   param,
				Message: fmt.Sprintf("unknown parameter '%s'", param),
			})
			continue
		}

		// Run parameter-specific validator if exists
		if validator, ok := schema.ParameterValidators[param]; ok {
			if err := validator(value); err != nil {
				errors = append(errors, ValidationError{
					StateID: decl.ID,
					Module:  decl.Module,
					Field:   param,
					Message: err.Error(),
				})
			}
		}
	}

	return errors
}

// validateRequisites validates that all state references exist
func (v *Validator) validateRequisites(stateFile *StateFile) []ValidationError {
	var errors []ValidationError

	// Build index of all states
	stateIndex := make(map[string]bool)
	for module, declarations := range stateFile.States {
		for _, decl := range declarations {
			key := module + ":" + decl.ID
			stateIndex[key] = true
		}
	}

	// Validate all references
	for module, declarations := range stateFile.States {
		for _, decl := range declarations {
			// Check all requisite types
			refs := collectAllReferences(&decl.Requisites)
			for _, ref := range refs {
				key := ref.Module + ":" + ref.ID
				if !stateIndex[key] {
					errors = append(errors, ValidationError{
						StateID: decl.ID,
						Module:  module,
						Message: fmt.Sprintf("references non-existent state: %s.%s", ref.Module, ref.ID),
					})
				}
			}
		}
	}

	return errors
}

// collectAllReferences collects all state references from requisites
func collectAllReferences(reqs *Requisites) []StateReference {
	var refs []StateReference
	refs = append(refs, reqs.Require...)
	refs = append(refs, reqs.RequireIn...)
	refs = append(refs, reqs.Watch...)
	refs = append(refs, reqs.WatchIn...)
	refs = append(refs, reqs.Prereq...)
	refs = append(refs, reqs.PrereqIn...)
	refs = append(refs, reqs.Onchanges...)
	refs = append(refs, reqs.OnchangesIn...)
	return refs
}

// isValidState checks if a state value is valid
func isValidState(state string, validStates []string) bool {
	for _, valid := range validStates {
		if state == valid {
			return true
		}
	}
	return false
}

// isKnownParameter checks if a parameter is known
func isKnownParameter(param string, schema *ModuleSchema) bool {
	for _, p := range schema.RequiredParameters {
		if param == p {
			return true
		}
	}
	for _, p := range schema.OptionalParameters {
		if param == p {
			return true
		}
	}
	return false
}

// getDefaultModuleSchemas returns default validation schemas
func getDefaultModuleSchemas() map[string]*ModuleSchema {
	return map[string]*ModuleSchema{
		"file": {
			Name:        "file",
			ValidStates: []string{"present", "absent", "directory", "symlink"},
			RequiredParameters: []string{},
			OptionalParameters: []string{
				"source", "contents", "mode", "user", "group",
				"makedirs", "replace", "backup", "template",
			},
			ParameterValidators: map[string]ParameterValidator{
				"mode": validateFileMode,
			},
		},
		"package": {
			Name:        "package",
			ValidStates: []string{"installed", "removed", "latest", "purged"},
			RequiredParameters: []string{},
			OptionalParameters: []string{
				"version", "refresh", "hold", "skip_verify",
			},
		},
		"service": {
			Name:        "service",
			ValidStates: []string{"running", "stopped", "enabled", "disabled", "dead"},
			RequiredParameters: []string{},
			OptionalParameters: []string{
				"enable", "reload", "force_reload", "full_restart",
			},
		},
		"user": {
			Name:        "user",
			ValidStates: []string{"present", "absent"},
			RequiredParameters: []string{},
			OptionalParameters: []string{
				"uid", "gid", "groups", "home", "shell",
				"createhome", "system", "password",
			},
		},
		"group": {
			Name:        "group",
			ValidStates: []string{"present", "absent"},
			RequiredParameters: []string{},
			OptionalParameters: []string{
				"gid", "system", "members",
			},
		},
		"cmd": {
			Name:        "cmd",
			ValidStates: []string{"run", "wait"},
			RequiredParameters: []string{},
			OptionalParameters: []string{
				"creates", "removes", "cwd", "env", "timeout",
				"runas", "shell", "stateful",
			},
		},
	}
}

// validateFileMode validates file mode format
func validateFileMode(value interface{}) error {
	mode, ok := value.(string)
	if !ok {
		return fmt.Errorf("mode must be a string")
	}

	// Validate octal mode format (e.g., "0644", "0755")
	matched, _ := regexp.MatchString(`^0?[0-7]{3,4}$`, mode)
	if !matched {
		return fmt.Errorf("invalid file mode format: %s (expected octal like '0644')", mode)
	}

	return nil
}

// ValidateStateID validates that a state ID is valid
func ValidateStateID(id string) error {
	if id == "" {
		return fmt.Errorf("state ID cannot be empty")
	}

	// State IDs should not contain certain characters
	invalidChars := []string{"\n", "\t", "\r"}
	for _, char := range invalidChars {
		if strings.Contains(id, char) {
			return fmt.Errorf("state ID contains invalid character")
		}
	}

	return nil
}
