package statemgmt

import (
	"fmt"
	"regexp"
	"sort"
	"strings"
)

// ValidationLevel defines the severity of a validation issue
type ValidationLevel string

const (
	// ValidationLevelError indicates a fatal error that prevents apply
	ValidationLevelError ValidationLevel = "error"
	// ValidationLevelWarning indicates a potential issue that doesn't block apply
	ValidationLevelWarning ValidationLevel = "warning"
	// ValidationLevelInfo indicates informational messages
	ValidationLevelInfo ValidationLevel = "info"
)

// ValidationIssue represents a single validation issue
type ValidationIssue struct {
	Level   ValidationLevel
	Code    string
	Message string
	Detail  string
	StateID string
	Module  string
	Field   string
	Line    int
	Column  int
	Suggestion string
}

// Error implements the error interface
func (v *ValidationIssue) Error() string {
	var parts []string

	if v.StateID != "" {
		parts = append(parts, fmt.Sprintf("[%s.%s]", v.Module, v.StateID))
	} else if v.Module != "" {
		parts = append(parts, fmt.Sprintf("[%s]", v.Module))
	}

	parts = append(parts, string(v.Level)+":")
	parts = append(parts, v.Message)

	if v.Field != "" {
		parts = append(parts, fmt.Sprintf("(field: %s)", v.Field))
	}

	if v.Line > 0 {
		if v.Column > 0 {
			parts = append(parts, fmt.Sprintf("at line %d, column %d", v.Line, v.Column))
		} else {
			parts = append(parts, fmt.Sprintf("at line %d", v.Line))
		}
	}

	return strings.Join(parts, " ")
}

// ValidationResult holds the result of state validation
type ValidationResult struct {
	Valid   bool
	Issues  []*ValidationIssue
	Errors  int
	Warnings int
	Infos   int
}

// AddIssue adds a validation issue
func (r *ValidationResult) AddIssue(issue *ValidationIssue) {
	r.Issues = append(r.Issues, issue)

	switch issue.Level {
	case ValidationLevelError:
		r.Errors++
		r.Valid = false
	case ValidationLevelWarning:
		r.Warnings++
	case ValidationLevelInfo:
		r.Infos++
	}
}

// ErrorMessages returns all error messages
func (r *ValidationResult) ErrorMessages() []string {
	var msgs []string
	for _, issue := range r.Issues {
		if issue.Level == ValidationLevelError {
			msgs = append(msgs, issue.Error())
		}
	}
	return msgs
}

// Summary returns a human-readable summary
func (r *ValidationResult) Summary() string {
	if r.Valid && r.Warnings == 0 {
		return "Validation passed"
	}

	var parts []string
	if r.Errors > 0 {
		parts = append(parts, fmt.Sprintf("%d error(s)", r.Errors))
	}
	if r.Warnings > 0 {
		parts = append(parts, fmt.Sprintf("%d warning(s)", r.Warnings))
	}
	if r.Infos > 0 {
		parts = append(parts, fmt.Sprintf("%d info(s)", r.Infos))
	}

	status := "Validation passed with warnings"
	if !r.Valid {
		status = "Validation failed"
	}

	return fmt.Sprintf("%s: %s", status, strings.Join(parts, ", "))
}

// Validator validates state files
type Validator struct {
	// KnownModules is the list of known/supported modules
	KnownModules map[string]*ModuleSchema

	// StrictMode enables strict validation (unknown fields cause errors)
	StrictMode bool

	// SkipRequisiteValidation skips checking if referenced states exist
	SkipRequisiteValidation bool
}

// ModuleSchema defines the schema for a state module
type ModuleSchema struct {
	Name           string
	Description    string
	ValidStates    []string
	RequiredFields []string
	OptionalFields []string
	FieldTypes     map[string]FieldType
	FieldValidators map[string]FieldValidator
}

// FieldType defines the expected type of a field
type FieldType string

const (
	FieldTypeString  FieldType = "string"
	FieldTypeBool    FieldType = "bool"
	FieldTypeInt     FieldType = "int"
	FieldTypeFloat   FieldType = "float"
	FieldTypeList    FieldType = "list"
	FieldTypeMap     FieldType = "map"
	FieldTypeAny     FieldType = "any"
)

// FieldValidator is a function that validates a field value
type FieldValidator func(value interface{}) *ValidationIssue

// NewValidator creates a new validator with default module schemas
func NewValidator() *Validator {
	return &Validator{
		KnownModules: defaultModuleSchemas(),
		StrictMode:   false,
	}
}

// defaultModuleSchemas returns schemas for built-in modules
func defaultModuleSchemas() map[string]*ModuleSchema {
	return map[string]*ModuleSchema{
		"file": {
			Name:           "file",
			Description:    "Manages files and directories",
			ValidStates:    []string{"present", "absent", "directory", "symlink"},
			RequiredFields: []string{},
			OptionalFields: []string{
				"source", "contents", "template", "mode", "owner", "group",
				"makedirs", "follow", "force", "backup", "encoding", "selinux",
			},
			FieldTypes: map[string]FieldType{
				"source":   FieldTypeString,
				"contents": FieldTypeString,
				"template": FieldTypeString,
				"mode":     FieldTypeString,
				"owner":    FieldTypeString,
				"group":    FieldTypeString,
				"makedirs": FieldTypeBool,
				"follow":   FieldTypeBool,
				"force":    FieldTypeBool,
				"backup":   FieldTypeBool,
				"encoding": FieldTypeString,
			},
		},
		"package": {
			Name:           "package",
			Description:    "Manages software packages",
			ValidStates:    []string{"installed", "latest", "removed", "purged"},
			RequiredFields: []string{},
			OptionalFields: []string{
				"version", "source", "repository", "allow_downgrade",
				"refresh", "pkgs", "skip_verify",
			},
			FieldTypes: map[string]FieldType{
				"version":         FieldTypeString,
				"source":          FieldTypeString,
				"repository":      FieldTypeString,
				"allow_downgrade": FieldTypeBool,
				"refresh":         FieldTypeBool,
				"pkgs":            FieldTypeList,
				"skip_verify":     FieldTypeBool,
			},
		},
		"service": {
			Name:           "service",
			Description:    "Manages system services",
			ValidStates:    []string{"running", "stopped", "restarted", "reloaded"},
			RequiredFields: []string{},
			OptionalFields: []string{"enabled", "reload", "restart", "mask", "unmask"},
			FieldTypes: map[string]FieldType{
				"enabled": FieldTypeBool,
				"reload":  FieldTypeBool,
				"restart": FieldTypeBool,
				"mask":    FieldTypeBool,
				"unmask":  FieldTypeBool,
			},
		},
		"user": {
			Name:           "user",
			Description:    "Manages user accounts",
			ValidStates:    []string{"present", "absent"},
			RequiredFields: []string{},
			OptionalFields: []string{
				"uid", "gid", "groups", "home", "shell", "password",
				"createhome", "system", "remove", "force",
			},
			FieldTypes: map[string]FieldType{
				"uid":        FieldTypeInt,
				"gid":        FieldTypeInt,
				"groups":     FieldTypeList,
				"home":       FieldTypeString,
				"shell":      FieldTypeString,
				"password":   FieldTypeString,
				"createhome": FieldTypeBool,
				"system":     FieldTypeBool,
				"remove":     FieldTypeBool,
				"force":      FieldTypeBool,
			},
		},
		"group": {
			Name:           "group",
			Description:    "Manages groups",
			ValidStates:    []string{"present", "absent"},
			RequiredFields: []string{},
			OptionalFields: []string{"gid", "system"},
			FieldTypes: map[string]FieldType{
				"gid":    FieldTypeInt,
				"system": FieldTypeBool,
			},
		},
		"cmd": {
			Name:           "cmd",
			Description:    "Executes commands",
			ValidStates:    []string{"run"},
			RequiredFields: []string{},
			OptionalFields: []string{
				"name", "args", "env", "cwd", "user", "group",
				"shell", "timeout", "creates", "unless", "onlyif",
			},
			FieldTypes: map[string]FieldType{
				"name":    FieldTypeString,
				"args":    FieldTypeList,
				"env":     FieldTypeMap,
				"cwd":     FieldTypeString,
				"user":    FieldTypeString,
				"group":   FieldTypeString,
				"shell":   FieldTypeString,
				"timeout": FieldTypeInt,
				"creates": FieldTypeString,
				"unless":  FieldTypeString,
				"onlyif":  FieldTypeString,
			},
		},
		"cron": {
			Name:           "cron",
			Description:    "Manages cron jobs",
			ValidStates:    []string{"present", "absent"},
			RequiredFields: []string{},
			OptionalFields: []string{
				"name", "user", "minute", "hour", "daymonth",
				"month", "dayweek", "command", "commented",
			},
			FieldTypes: map[string]FieldType{
				"name":      FieldTypeString,
				"user":      FieldTypeString,
				"minute":    FieldTypeString,
				"hour":      FieldTypeString,
				"daymonth":  FieldTypeString,
				"month":     FieldTypeString,
				"dayweek":   FieldTypeString,
				"command":   FieldTypeString,
				"commented": FieldTypeBool,
			},
		},
		"git": {
			Name:           "git",
			Description:    "Manages git repositories",
			ValidStates:    []string{"present", "latest", "absent"},
			RequiredFields: []string{"source"},
			OptionalFields: []string{
				"branch", "rev", "tag", "depth", "force",
				"user", "identity", "https_user", "https_password",
			},
			FieldTypes: map[string]FieldType{
				"source":         FieldTypeString,
				"branch":         FieldTypeString,
				"rev":            FieldTypeString,
				"tag":            FieldTypeString,
				"depth":          FieldTypeInt,
				"force":          FieldTypeBool,
				"user":           FieldTypeString,
				"identity":       FieldTypeString,
				"https_user":     FieldTypeString,
				"https_password": FieldTypeString,
			},
		},
	}
}

// Validate validates a state file
func (v *Validator) Validate(stateFile *StateFile) *ValidationResult {
	result := &ValidationResult{Valid: true}

	// Validate metadata
	v.validateMetadata(stateFile, result)

	// Collect all state IDs for requisite validation
	stateIDs := make(map[string]map[string]bool) // module -> ids
	for module, declarations := range stateFile.States {
		if stateIDs[module] == nil {
			stateIDs[module] = make(map[string]bool)
		}
		for _, decl := range declarations {
			stateIDs[module][decl.ID] = true
		}
	}

	// Validate each state declaration
	for _, declarations := range stateFile.States {
		for i, decl := range declarations {
			v.validateStateDeclaration(&decl, i, stateIDs, result)
		}
	}

	// Check for circular dependencies
	v.validateDependencyGraph(stateFile, result)

	return result
}

// validateMetadata validates state file metadata
func (v *Validator) validateMetadata(stateFile *StateFile, result *ValidationResult) {
	// Check for empty state file
	if len(stateFile.States) == 0 && len(stateFile.Includes) == 0 && len(stateFile.BlueprintIncludes) == 0 {
		result.AddIssue(&ValidationIssue{
			Level:      ValidationLevelWarning,
			Code:       "EMPTY_STATE_FILE",
			Message:    "State file contains no states, includes, or blueprints",
			Suggestion: "Add state declarations or includes to define desired configuration",
		})
	}

	// Check metadata version
	if stateFile.Metadata.Version != "" {
		if !isValidVersion(stateFile.Metadata.Version) {
			result.AddIssue(&ValidationIssue{
				Level:   ValidationLevelWarning,
				Code:    "INVALID_VERSION",
				Message: fmt.Sprintf("Invalid version format: %s", stateFile.Metadata.Version),
				Field:   "metadata.version",
				Suggestion: "Use semantic versioning (e.g., 1.0.0)",
			})
		}
	}
}

// validateStateDeclaration validates a single state declaration
func (v *Validator) validateStateDeclaration(decl *StateDeclaration, index int, stateIDs map[string]map[string]bool, result *ValidationResult) {
	// Check for empty ID
	if decl.ID == "" {
		result.AddIssue(&ValidationIssue{
			Level:   ValidationLevelError,
			Code:    "MISSING_STATE_ID",
			Message: "State declaration missing ID",
			Module:  decl.Module,
			Suggestion: "Add a unique identifier for this state",
		})
		return
	}

	// Check for duplicate ID within the same module
	// (This is handled during parsing, but check again for safety)

	// Validate module
	schema, ok := v.KnownModules[decl.Module]
	if !ok {
		if v.StrictMode {
			result.AddIssue(&ValidationIssue{
				Level:   ValidationLevelError,
				Code:    "UNKNOWN_MODULE",
				Message: fmt.Sprintf("Unknown module type: %s", decl.Module),
				StateID: decl.ID,
				Module:  decl.Module,
				Suggestion: fmt.Sprintf("Use one of: %s", strings.Join(knownModuleNames(v.KnownModules), ", ")),
			})
		} else {
			result.AddIssue(&ValidationIssue{
				Level:   ValidationLevelWarning,
				Code:    "UNKNOWN_MODULE",
				Message: fmt.Sprintf("Unknown module type: %s", decl.Module),
				StateID: decl.ID,
				Module:  decl.Module,
				Suggestion: "This may be a custom module. Ensure it's registered.",
			})
		}
		// Skip further validation for unknown modules
		return
	}

	// Validate state value
	if decl.State != "" && !containsStr(schema.ValidStates, decl.State) {
		result.AddIssue(&ValidationIssue{
			Level:   ValidationLevelError,
			Code:    "INVALID_STATE",
			Message: fmt.Sprintf("Invalid state '%s' for module %s", decl.State, decl.Module),
			StateID: decl.ID,
			Module:  decl.Module,
			Field:   "state",
			Suggestion: fmt.Sprintf("Valid states: %s", strings.Join(schema.ValidStates, ", ")),
		})
	}

	// Validate required fields
	for _, field := range schema.RequiredFields {
		if _, ok := decl.Parameters[field]; !ok {
			result.AddIssue(&ValidationIssue{
				Level:   ValidationLevelError,
				Code:    "MISSING_REQUIRED_FIELD",
				Message: fmt.Sprintf("Missing required field '%s'", field),
				StateID: decl.ID,
				Module:  decl.Module,
				Field:   field,
			})
		}
	}

	// Validate field types
	allValidFields := append(schema.RequiredFields, schema.OptionalFields...)
	for field, value := range decl.Parameters {
		// Check for unknown fields in strict mode
		if !containsStr(allValidFields, field) {
			if v.StrictMode {
				result.AddIssue(&ValidationIssue{
					Level:   ValidationLevelError,
					Code:    "UNKNOWN_FIELD",
					Message: fmt.Sprintf("Unknown field '%s'", field),
					StateID: decl.ID,
					Module:  decl.Module,
					Field:   field,
				})
			} else {
				result.AddIssue(&ValidationIssue{
					Level:   ValidationLevelInfo,
					Code:    "UNKNOWN_FIELD",
					Message: fmt.Sprintf("Unknown field '%s' (may be module-specific)", field),
					StateID: decl.ID,
					Module:  decl.Module,
					Field:   field,
				})
			}
			continue
		}

		// Validate field type
		if expectedType, ok := schema.FieldTypes[field]; ok {
			if err := validateFieldType(field, value, expectedType); err != nil {
				result.AddIssue(&ValidationIssue{
					Level:   ValidationLevelError,
					Code:    "INVALID_FIELD_TYPE",
					Message: err.Error(),
					StateID: decl.ID,
					Module:  decl.Module,
					Field:   field,
				})
			}
		}

		// Run custom validators
		if validator, ok := schema.FieldValidators[field]; ok {
			if issue := validator(value); issue != nil {
				issue.StateID = decl.ID
				issue.Module = decl.Module
				issue.Field = field
				result.AddIssue(issue)
			}
		}
	}

	// Validate module-specific constraints
	v.validateModuleConstraints(decl, schema, result)

	// Validate requisites
	if !v.SkipRequisiteValidation {
		v.validateRequisites(decl, stateIDs, result)
	}

	// Validate retry configuration
	if decl.Retry != nil {
		v.validateRetryConfig(decl, result)
	}
}

// validateModuleConstraints validates module-specific constraints
func (v *Validator) validateModuleConstraints(decl *StateDeclaration, schema *ModuleSchema, result *ValidationResult) {
	switch decl.Module {
	case "file":
		// Check for mutually exclusive fields
		hasSource := decl.Parameters["source"] != nil
		hasContents := decl.Parameters["contents"] != nil
		hasTemplate := decl.Parameters["template"] != nil

		count := 0
		if hasSource {
			count++
		}
		if hasContents {
			count++
		}
		if hasTemplate {
			count++
		}

		if count > 1 {
			result.AddIssue(&ValidationIssue{
				Level:   ValidationLevelError,
				Code:    "MUTUALLY_EXCLUSIVE_FIELDS",
				Message: "Only one of 'source', 'contents', or 'template' can be specified",
				StateID: decl.ID,
				Module:  decl.Module,
				Suggestion: "Remove all but one of these fields",
			})
		}

		// Validate file mode format
		if mode, ok := decl.Parameters["mode"].(string); ok {
			if !isValidFileMode(mode) {
				result.AddIssue(&ValidationIssue{
					Level:   ValidationLevelError,
					Code:    "INVALID_FILE_MODE",
					Message: fmt.Sprintf("Invalid file mode: %s", mode),
					StateID: decl.ID,
					Module:  decl.Module,
					Field:   "mode",
					Suggestion: "Use octal format like '0644' or '0755'",
				})
			}
		}

	case "service":
		// Warn if reload and restart are both set
		if decl.Parameters["reload"] == true && decl.Parameters["restart"] == true {
			result.AddIssue(&ValidationIssue{
				Level:   ValidationLevelWarning,
				Code:    "CONFLICTING_OPTIONS",
				Message: "Both 'reload' and 'restart' are set; 'restart' will take precedence",
				StateID: decl.ID,
				Module:  decl.Module,
			})
		}

	case "user":
		// Validate shell path
		if shell, ok := decl.Parameters["shell"].(string); ok {
			if !strings.HasPrefix(shell, "/") {
				result.AddIssue(&ValidationIssue{
					Level:   ValidationLevelWarning,
					Code:    "RELATIVE_SHELL_PATH",
					Message: "Shell path should be absolute",
					StateID: decl.ID,
					Module:  decl.Module,
					Field:   "shell",
					Suggestion: "Use an absolute path like '/bin/bash'",
				})
			}
		}

	case "git":
		// Warn if both branch and rev/tag are specified
		hasBranch := decl.Parameters["branch"] != nil
		hasRev := decl.Parameters["rev"] != nil
		hasTag := decl.Parameters["tag"] != nil

		if hasBranch && (hasRev || hasTag) {
			result.AddIssue(&ValidationIssue{
				Level:   ValidationLevelWarning,
				Code:    "CONFLICTING_GIT_OPTIONS",
				Message: "Both 'branch' and 'rev'/'tag' are specified; 'rev'/'tag' will take precedence",
				StateID: decl.ID,
				Module:  decl.Module,
			})
		}
	}
}

// validateRequisites validates state requisites
func (v *Validator) validateRequisites(decl *StateDeclaration, stateIDs map[string]map[string]bool, result *ValidationResult) {
	validateRefs := func(refs []StateReference, reqType string) {
		for _, ref := range refs {
			if ids, ok := stateIDs[ref.Module]; ok {
				if !ids[ref.ID] {
					result.AddIssue(&ValidationIssue{
						Level:   ValidationLevelError,
						Code:    "INVALID_REQUISITE_REFERENCE",
						Message: fmt.Sprintf("%s references non-existent state: %s.%s", reqType, ref.Module, ref.ID),
						StateID: decl.ID,
						Module:  decl.Module,
						Field:   strings.ToLower(reqType),
						Suggestion: "Check that the referenced state ID and module are correct",
					})
				}
			} else {
				result.AddIssue(&ValidationIssue{
					Level:   ValidationLevelWarning,
					Code:    "UNKNOWN_REQUISITE_MODULE",
					Message: fmt.Sprintf("%s references unknown module: %s", reqType, ref.Module),
					StateID: decl.ID,
					Module:  decl.Module,
					Field:   strings.ToLower(reqType),
				})
			}
		}
	}

	validateRefs(decl.Requisites.Require, "require")
	validateRefs(decl.Requisites.RequireIn, "require_in")
	validateRefs(decl.Requisites.Watch, "watch")
	validateRefs(decl.Requisites.WatchIn, "watch_in")
	validateRefs(decl.Requisites.Prereq, "prereq")
	validateRefs(decl.Requisites.PrereqIn, "prereq_in")
	validateRefs(decl.Requisites.Onchanges, "onchanges")
	validateRefs(decl.Requisites.OnchangesIn, "onchanges_in")
}

// validateRetryConfig validates retry configuration
func (v *Validator) validateRetryConfig(decl *StateDeclaration, result *ValidationResult) {
	retry := decl.Retry

	if retry.Attempts < 0 {
		result.AddIssue(&ValidationIssue{
			Level:   ValidationLevelError,
			Code:    "INVALID_RETRY_ATTEMPTS",
			Message: "Retry attempts must be non-negative",
			StateID: decl.ID,
			Module:  decl.Module,
			Field:   "retry.attempts",
		})
	}

	if retry.Delay < 0 {
		result.AddIssue(&ValidationIssue{
			Level:   ValidationLevelError,
			Code:    "INVALID_RETRY_DELAY",
			Message: "Retry delay must be non-negative",
			StateID: decl.ID,
			Module:  decl.Module,
			Field:   "retry.delay",
		})
	}

	if retry.BackoffMultiplier < 0 {
		result.AddIssue(&ValidationIssue{
			Level:   ValidationLevelError,
			Code:    "INVALID_BACKOFF_MULTIPLIER",
			Message: "Backoff multiplier must be non-negative",
			StateID: decl.ID,
			Module:  decl.Module,
			Field:   "retry.backoff_multiplier",
		})
	}

	if retry.BackoffMultiplier > 0 && retry.BackoffMultiplier < 1 {
		result.AddIssue(&ValidationIssue{
			Level:   ValidationLevelWarning,
			Code:    "LOW_BACKOFF_MULTIPLIER",
			Message: "Backoff multiplier less than 1 will decrease delay over time",
			StateID: decl.ID,
			Module:  decl.Module,
			Field:   "retry.backoff_multiplier",
			Suggestion: "Use a multiplier >= 1 for exponential backoff",
		})
	}
}

// validateDependencyGraph checks for circular dependencies
func (v *Validator) validateDependencyGraph(stateFile *StateFile, result *ValidationResult) {
	// Build adjacency list
	graph := make(map[string][]string)
	stateKey := func(module, id string) string {
		return module + "." + id
	}

	for module, declarations := range stateFile.States {
		for _, decl := range declarations {
			key := stateKey(module, decl.ID)
			if _, ok := graph[key]; !ok {
				graph[key] = []string{}
			}

			// Add edges from requires
			for _, ref := range decl.Requisites.Require {
				graph[key] = append(graph[key], stateKey(ref.Module, ref.ID))
			}
		}
	}

	// DFS for cycle detection
	visited := make(map[string]int) // 0: not visited, 1: visiting, 2: visited
	var path []string

	var dfs func(node string) bool
	dfs = func(node string) bool {
		if visited[node] == 1 {
			// Found cycle
			cycleStart := -1
			for i, n := range path {
				if n == node {
					cycleStart = i
					break
				}
			}
			if cycleStart >= 0 {
				cycle := append(path[cycleStart:], node)
				result.AddIssue(&ValidationIssue{
					Level:   ValidationLevelError,
					Code:    "CIRCULAR_DEPENDENCY",
					Message: fmt.Sprintf("Circular dependency detected: %s", strings.Join(cycle, " -> ")),
					Suggestion: "Remove one of the dependencies to break the cycle",
				})
			}
			return true
		}

		if visited[node] == 2 {
			return false
		}

		visited[node] = 1
		path = append(path, node)

		for _, neighbor := range graph[node] {
			if dfs(neighbor) {
				return true
			}
		}

		path = path[:len(path)-1]
		visited[node] = 2
		return false
	}

	for node := range graph {
		if visited[node] == 0 {
			dfs(node)
		}
	}
}

// Helper functions

func validateFieldType(field string, value interface{}, expected FieldType) error {
	switch expected {
	case FieldTypeString:
		if _, ok := value.(string); !ok {
			return fmt.Errorf("field '%s' must be a string, got %T", field, value)
		}
	case FieldTypeBool:
		if _, ok := value.(bool); !ok {
			return fmt.Errorf("field '%s' must be a boolean, got %T", field, value)
		}
	case FieldTypeInt:
		switch value.(type) {
		case int, int32, int64, float64:
			// Accept numeric types
		default:
			return fmt.Errorf("field '%s' must be an integer, got %T", field, value)
		}
	case FieldTypeFloat:
		switch value.(type) {
		case float32, float64, int, int64:
			// Accept numeric types
		default:
			return fmt.Errorf("field '%s' must be a number, got %T", field, value)
		}
	case FieldTypeList:
		if _, ok := value.([]interface{}); !ok {
			return fmt.Errorf("field '%s' must be a list, got %T", field, value)
		}
	case FieldTypeMap:
		if _, ok := value.(map[string]interface{}); !ok {
			return fmt.Errorf("field '%s' must be a map, got %T", field, value)
		}
	}
	return nil
}

func containsStr(slice []string, item string) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
}

func knownModuleNames(modules map[string]*ModuleSchema) []string {
	names := make([]string, 0, len(modules))
	for name := range modules {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func isValidVersion(version string) bool {
	// Simple semver check
	pattern := `^v?\d+\.\d+\.\d+(-[\w.]+)?(\+[\w.]+)?$`
	matched, _ := regexp.MatchString(pattern, version)
	return matched
}

func isValidFileMode(mode string) bool {
	// Check for octal format (e.g., "0644", "755")
	pattern := `^0?[0-7]{3,4}$`
	matched, _ := regexp.MatchString(pattern, mode)
	return matched
}

// ValidateBeforeApply is a convenience function to validate a state file
// and return a formatted error if validation fails
func ValidateBeforeApply(stateFile *StateFile) error {
	validator := NewValidator()
	result := validator.Validate(stateFile)

	if !result.Valid {
		var errMsgs []string
		errMsgs = append(errMsgs, result.Summary())
		errMsgs = append(errMsgs, "")
		for _, issue := range result.Issues {
			if issue.Level == ValidationLevelError {
				msg := fmt.Sprintf("  - %s", issue.Error())
				if issue.Suggestion != "" {
					msg += fmt.Sprintf("\n    Suggestion: %s", issue.Suggestion)
				}
				errMsgs = append(errMsgs, msg)
			}
		}
		return fmt.Errorf("%s", strings.Join(errMsgs, "\n"))
	}

	return nil
}
