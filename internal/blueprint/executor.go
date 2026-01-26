// Package blueprint provides the blueprint executor for integrating
// blueprints with the state management system.
package blueprint

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"gopkg.in/yaml.v3"
)

// ExecutorConfig configures the blueprint executor.
type ExecutorConfig struct {
	// Loader is the blueprint loader to use.
	Loader *Loader

	// LocalPaths are local directories to search for blueprints.
	LocalPaths []string

	// CachePath is where to cache downloaded blueprints.
	CachePath string

	// DryRun if true, doesn't apply states but reports what would change.
	DryRun bool

	// ConcurrentBlueprints is the max number of blueprints to process concurrently.
	ConcurrentBlueprints int

	// Timeout for processing each blueprint.
	Timeout time.Duration
}

// DefaultExecutorConfig returns default executor configuration.
func DefaultExecutorConfig() *ExecutorConfig {
	return &ExecutorConfig{
		ConcurrentBlueprints: 1,
		Timeout:              5 * time.Minute,
	}
}

// Executor processes blueprints and integrates them with state execution.
type Executor struct {
	config *ExecutorConfig
	loader *Loader
	mu     sync.RWMutex

	// Applied blueprints indexed by namespace
	appliedBlueprints map[string]*AppliedBlueprint
}

// AppliedBlueprint tracks a blueprint that has been applied.
type AppliedBlueprint struct {
	// Name is the blueprint name (vendor/name).
	Name string `json:"name"`

	// Version is the resolved version.
	Version string `json:"version"`

	// Namespace is the "as" namespace used.
	Namespace string `json:"namespace"`

	// Parameters are the resolved parameters used.
	Parameters map[string]interface{} `json:"parameters"`

	// EnabledFeatures lists which features were enabled.
	EnabledFeatures []string `json:"enabled_features"`

	// AppliedAt is when the blueprint was applied.
	AppliedAt time.Time `json:"applied_at"`

	// StateCount is the number of states in this blueprint.
	StateCount int `json:"state_count"`

	// LastError is the last error if any.
	LastError string `json:"last_error,omitempty"`
}

// ExecuteResult contains the result of executing blueprints.
type ExecuteResult struct {
	// AppliedBlueprints lists the blueprints that were applied.
	AppliedBlueprints []*AppliedBlueprint `json:"applied_blueprints"`

	// TotalStates is the total number of states processed.
	TotalStates int `json:"total_states"`

	// ChangedStates is the number of states that made changes.
	ChangedStates int `json:"changed_states"`

	// FailedStates is the number of states that failed.
	FailedStates int `json:"failed_states"`

	// Duration is how long execution took.
	Duration time.Duration `json:"duration"`

	// Errors are any errors that occurred.
	Errors []string `json:"errors,omitempty"`
}

// NewExecutor creates a new blueprint executor.
func NewExecutor(config *ExecutorConfig) (*Executor, error) {
	if config == nil {
		config = DefaultExecutorConfig()
	}

	loader := config.Loader
	if loader == nil {
		loader = NewLoader(nil)
	}

	return &Executor{
		config:            config,
		loader:            loader,
		appliedBlueprints: make(map[string]*AppliedBlueprint),
	}, nil
}

// ExpandBlueprintIncludes expands blueprint includes in a state file into
// state declarations that can be executed by the state executor.
func (e *Executor) ExpandBlueprintIncludes(ctx context.Context, stateFile *BlueprintStateFile) (*BlueprintStateFile, error) {
	if len(stateFile.BlueprintIncludes) == 0 {
		return stateFile, nil
	}

	// Create a new state file with expanded states
	expanded := &BlueprintStateFile{
		Path:      stateFile.Path,
		Includes:  stateFile.Includes,
		Variables: make(map[string]interface{}),
		States:    make(map[string][]BlueprintStateDeclaration),
		Metadata:  stateFile.Metadata,
		// Don't copy BlueprintIncludes - we're expanding them
	}

	// Copy existing variables
	for k, v := range stateFile.Variables {
		expanded.Variables[k] = v
	}

	// Copy existing states
	for module, declarations := range stateFile.States {
		expanded.States[module] = append(expanded.States[module], declarations...)
	}

	// Process each blueprint include
	for _, include := range stateFile.BlueprintIncludes {
		states, appliedBP, err := e.expandBlueprintInclude(ctx, &include)
		if err != nil {
			return nil, fmt.Errorf("failed to expand blueprint %s: %w", include.Blueprint, err)
		}

		// Track the applied blueprint
		namespace := include.As
		if namespace == "" {
			namespace = include.Blueprint
		}
		e.mu.Lock()
		e.appliedBlueprints[namespace] = appliedBP
		e.mu.Unlock()

		// Merge the expanded states
		for module, declarations := range states {
			// Apply namespace prefix if "as" was specified
			if include.As != "" {
				for i := range declarations {
					declarations[i].ID = fmt.Sprintf("%s.%s", include.As, declarations[i].ID)
				}
			}
			expanded.States[module] = append(expanded.States[module], declarations...)
		}
	}

	return expanded, nil
}

// expandBlueprintInclude expands a single blueprint include.
func (e *Executor) expandBlueprintInclude(ctx context.Context, include *BlueprintInclude) (map[string][]BlueprintStateDeclaration, *AppliedBlueprint, error) {
	// Build load config from include
	loadConfig := &LoadConfig{
		Name:    include.Blueprint,
		Version: include.Version,
	}

	// Set features from include
	if include.Features != nil {
		loadConfig.Features = include.Features
	}

	// Set parameters
	loadConfig.Parameters = include.Parameters

	// Load the blueprint
	result, err := e.loader.Load(ctx, loadConfig)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to load blueprint: %w", err)
	}

	bp := result.Blueprint

	// Determine which state files to use
	var stateFiles []string
	if include.Entrypoint != "" {
		// Use specified entrypoint
		if entrypoint, ok := bp.Entrypoints[include.Entrypoint]; ok {
			stateFiles = []string{entrypoint}
		} else {
			return nil, nil, fmt.Errorf("entrypoint %q not found in blueprint", include.Entrypoint)
		}
	} else {
		// Use default entrypoint or all state files
		if defaultEntry, ok := bp.Entrypoints["default"]; ok {
			stateFiles = []string{defaultEntry}
		} else if mainEntry, ok := bp.Entrypoints["main"]; ok {
			stateFiles = []string{mainEntry}
		} else {
			// Use all state files from the result
			stateFiles = result.StateFiles
		}
	}

	// Render and parse each state file
	allStates := make(map[string][]BlueprintStateDeclaration)
	totalStateCount := 0

	for _, stateFile := range stateFiles {
		// Render the state file with parameters
		rendered, err := e.loader.RenderState(ctx, bp, stateFile, result.ResolvedParameters)
		if err != nil {
			return nil, nil, fmt.Errorf("failed to render state file %s: %w", stateFile, err)
		}

		// Parse the rendered YAML into state declarations
		states, err := e.parseRenderedState(rendered)
		if err != nil {
			return nil, nil, fmt.Errorf("failed to parse rendered state from %s: %w", stateFile, err)
		}

		// Merge states
		for module, declarations := range states {
			allStates[module] = append(allStates[module], declarations...)
			totalStateCount += len(declarations)
		}
	}

	applied := &AppliedBlueprint{
		Name:            include.Blueprint,
		Version:         result.ResolvedVersion,
		Namespace:       include.As,
		Parameters:      result.ResolvedParameters,
		EnabledFeatures: result.EnabledFeatures,
		AppliedAt:       time.Now(),
		StateCount:      totalStateCount,
	}

	return allStates, applied, nil
}

// parseRenderedState parses rendered YAML into state declarations.
func (e *Executor) parseRenderedState(rendered []byte) (map[string][]BlueprintStateDeclaration, error) {
	var rawState map[string]interface{}
	if err := yaml.Unmarshal(rendered, &rawState); err != nil {
		return nil, fmt.Errorf("failed to parse YAML: %w", err)
	}

	states := make(map[string][]BlueprintStateDeclaration)

	// Skip metadata, include, and variables - these are handled separately
	for module, declarations := range rawState {
		if module == "metadata" || module == "include" || module == "variables" {
			continue
		}

		if declarations == nil {
			continue
		}

		declMap, ok := declarations.(map[string]interface{})
		if !ok {
			continue
		}

		for stateID, params := range declMap {
			paramsMap, ok := params.(map[string]interface{})
			if !ok {
				continue
			}

			decl := BlueprintStateDeclaration{
				ID:         stateID,
				Module:     module,
				Parameters: make(map[string]interface{}),
			}

			// Extract standard fields
			if state, ok := paramsMap["state"].(string); ok {
				decl.State = state
				delete(paramsMap, "state")
			} else {
				decl.State = getDefaultState(module)
			}

			if order, ok := paramsMap["order"].(int); ok {
				decl.Order = order
				delete(paramsMap, "order")
			}

			if failHard, ok := paramsMap["fail_hard"].(bool); ok {
				decl.FailHard = failHard
				delete(paramsMap, "fail_hard")
			}

			if unless, ok := paramsMap["unless"].(string); ok {
				decl.Unless = unless
				delete(paramsMap, "unless")
			}

			if onlyIf, ok := paramsMap["only_if"].(string); ok {
				decl.OnlyIf = onlyIf
				delete(paramsMap, "only_if")
			}

			// Parse requisites
			decl.Requisites = parseRequisites(paramsMap)

			// Remaining fields are module-specific parameters
			for key, value := range paramsMap {
				if !isRequisiteField(key) {
					decl.Parameters[key] = value
				}
			}

			states[module] = append(states[module], decl)
		}
	}

	return states, nil
}

// GetAppliedBlueprints returns all applied blueprints.
func (e *Executor) GetAppliedBlueprints() []*AppliedBlueprint {
	e.mu.RLock()
	defer e.mu.RUnlock()

	result := make([]*AppliedBlueprint, 0, len(e.appliedBlueprints))
	for _, bp := range e.appliedBlueprints {
		result = append(result, bp)
	}
	return result
}

// GetAppliedBlueprint returns an applied blueprint by namespace.
func (e *Executor) GetAppliedBlueprint(namespace string) *AppliedBlueprint {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.appliedBlueprints[namespace]
}

// ClearAppliedBlueprints clears the applied blueprints tracking.
func (e *Executor) ClearAppliedBlueprints() {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.appliedBlueprints = make(map[string]*AppliedBlueprint)
}

// Helper functions

func getDefaultState(module string) string {
	defaults := map[string]string{
		"file":    "present",
		"package": "installed",
		"service": "running",
		"user":    "present",
		"group":   "present",
	}
	if state, ok := defaults[module]; ok {
		return state
	}
	return "present"
}

func parseRequisites(params map[string]interface{}) BlueprintRequisites {
	reqs := BlueprintRequisites{}

	if require, ok := params["require"].([]interface{}); ok {
		reqs.Require = parseStateReferences(require)
		delete(params, "require")
	}

	if requireIn, ok := params["require_in"].([]interface{}); ok {
		reqs.RequireIn = parseStateReferences(requireIn)
		delete(params, "require_in")
	}

	if watch, ok := params["watch"].([]interface{}); ok {
		reqs.Watch = parseStateReferences(watch)
		delete(params, "watch")
	}

	if watchIn, ok := params["watch_in"].([]interface{}); ok {
		reqs.WatchIn = parseStateReferences(watchIn)
		delete(params, "watch_in")
	}

	if prereq, ok := params["prereq"].([]interface{}); ok {
		reqs.Prereq = parseStateReferences(prereq)
		delete(params, "prereq")
	}

	if prereqIn, ok := params["prereq_in"].([]interface{}); ok {
		reqs.PrereqIn = parseStateReferences(prereqIn)
		delete(params, "prereq_in")
	}

	if onchanges, ok := params["onchanges"].([]interface{}); ok {
		reqs.Onchanges = parseStateReferences(onchanges)
		delete(params, "onchanges")
	}

	if onchangesIn, ok := params["onchanges_in"].([]interface{}); ok {
		reqs.OnchangesIn = parseStateReferences(onchangesIn)
		delete(params, "onchanges_in")
	}

	return reqs
}

func parseStateReferences(refs []interface{}) []BlueprintStateReference {
	var result []BlueprintStateReference

	for _, ref := range refs {
		refMap, ok := ref.(map[string]interface{})
		if !ok {
			continue
		}

		for module, id := range refMap {
			if idStr, ok := id.(string); ok {
				result = append(result, BlueprintStateReference{
					Module: module,
					ID:     idStr,
				})
			}
		}
	}

	return result
}

func isRequisiteField(field string) bool {
	requisiteFields := []string{
		"require", "require_in",
		"watch", "watch_in",
		"prereq", "prereq_in",
		"onchanges", "onchanges_in",
	}

	for _, rf := range requisiteFields {
		if field == rf {
			return true
		}
	}
	return false
}

// ParseBlueprintReference parses a blueprint reference string like "vendor/name@1.2.3"
// into name and version components.
func ParseBlueprintReference(ref string) (name, version string) {
	parts := strings.SplitN(ref, "@", 2)
	name = parts[0]
	if len(parts) > 1 {
		version = parts[1]
	}
	return
}
