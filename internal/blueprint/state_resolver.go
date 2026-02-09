// Package blueprint provides the state resolver for integrating
// blueprints with the state management system.
package blueprint

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// StateResolverConfig configures the state resolver.
type StateResolverConfig struct {
	// LocalBlueprintPaths are directories to search for local blueprints.
	LocalBlueprintPaths []string

	// CachePath is where to cache downloaded blueprints.
	CachePath string

	// AllowRemote enables fetching blueprints from remote registries.
	AllowRemote bool

	// StrictVersions requires exact version matches instead of constraints.
	StrictVersions bool
}

// DefaultStateResolverConfig returns default configuration.
func DefaultStateResolverConfig() *StateResolverConfig {
	return &StateResolverConfig{
		LocalBlueprintPaths: []string{
			"./blueprints",
			"/etc/keystone-core/blueprints",
		},
		CachePath:   "~/.kscore/blueprint-cache",
		AllowRemote: true,
	}
}

// StateResolver resolves blueprint includes in state files.
type StateResolver struct {
	config   *StateResolverConfig
	executor *Executor
	loader   *Loader
	storage  Storage
}

// NewStateResolver creates a new state resolver.
func NewStateResolver(config *StateResolverConfig) (*StateResolver, error) {
	if config == nil {
		config = DefaultStateResolverConfig()
	}

	// Create local storage for blueprints
	storage, err := NewLocalStorage(config.LocalBlueprintPaths[0], false)
	if err != nil {
		return nil, fmt.Errorf("failed to create storage: %w", err)
	}

	// Create loader
	loader := NewLoader(storage)

	// Create executor
	execConfig := &ExecutorConfig{
		Loader:     loader,
		LocalPaths: config.LocalBlueprintPaths,
		CachePath:  config.CachePath,
	}
	executor, err := NewExecutor(execConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create executor: %w", err)
	}

	return &StateResolver{
		config:   config,
		executor: executor,
		loader:   loader,
		storage:  storage,
	}, nil
}

// ResolveStateFile resolves all blueprint includes in a state file,
// returning an expanded state file with all blueprints inlined.
func (r *StateResolver) ResolveStateFile(ctx context.Context, stateFile *StateFile) (*StateFile, error) {
	// Use the executor to expand blueprint includes
	expanded, err := r.executor.ExpandIncludes(ctx, stateFile)
	if err != nil {
		return nil, err
	}
	return expanded, nil
}

// ResolveStateFiles resolves multiple state files, expanding all blueprint includes.
func (r *StateResolver) ResolveStateFiles(ctx context.Context, stateFiles []*StateFile) ([]*StateFile, error) {
	var result []*StateFile

	for _, sf := range stateFiles {
		resolved, err := r.ResolveStateFile(ctx, sf)
		if err != nil {
			return nil, fmt.Errorf("failed to resolve state file %s: %w", sf.Path, err)
		}
		result = append(result, resolved)
	}

	return result, nil
}

// ParseAndResolve combines parsing and blueprint resolution into one step.
// It parses the state file and resolves any blueprint includes.
func (r *StateResolver) ParseAndResolve(ctx context.Context, path string) (*StateFile, error) {
	stateFile, err := r.ParseStateFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to parse state file: %w", err)
	}

	return r.ResolveStateFile(ctx, stateFile)
}

// ParseStateFile parses a YAML state file into a StateFile.
func (r *StateResolver) ParseStateFile(path string) (*StateFile, error) {
	data, err := readFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read state file: %w", err)
	}

	return r.parseStateFileBytes(data, path)
}

// parseStateFileBytes parses state file content from bytes.
func (r *StateResolver) parseStateFileBytes(data []byte, path string) (*StateFile, error) {
	var rawState map[string]interface{}
	if err := yaml.Unmarshal(data, &rawState); err != nil {
		return nil, fmt.Errorf("failed to parse YAML: %w", err)
	}

	stateFile := &StateFile{
		Path:      path,
		States:    make(map[string][]StateDeclaration),
		Variables: make(map[string]interface{}),
	}

	// Extract metadata if present
	if metadata, ok := rawState["metadata"].(map[string]interface{}); ok {
		stateFile.Metadata = parseMetadata(metadata)
		delete(rawState, "metadata")
	}

	// Extract includes if present
	if includes, ok := rawState["include"].([]interface{}); ok {
		for _, inc := range includes {
			switch v := inc.(type) {
			case string:
				stateFile.Includes = append(stateFile.Includes, v)
			case map[string]interface{}:
				if blueprintRef, ok := v["blueprint"].(string); ok {
					bpInclude := parseIncludeMap(blueprintRef, v)
					stateFile.BlueprintIncludes = append(stateFile.BlueprintIncludes, bpInclude)
				} else if fileRef, ok := v["file"].(string); ok {
					stateFile.Includes = append(stateFile.Includes, fileRef)
				}
			}
		}
		delete(rawState, "include")
	}

	// Extract variables if present
	if variables, ok := rawState["variables"].(map[string]interface{}); ok {
		stateFile.Variables = variables
		delete(rawState, "variables")
	}

	// Parse state declarations
	for module, declarations := range rawState {
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

			decl := StateDeclaration{
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

			stateFile.States[module] = append(stateFile.States[module], decl)
		}
	}

	return stateFile, nil
}

// parseMetadata parses metadata from a map.
func parseMetadata(metadata map[string]interface{}) *BlueprintMetadata {
	m := &BlueprintMetadata{}
	if name, ok := metadata["name"].(string); ok {
		m.Name = name
	}
	if desc, ok := metadata["description"].(string); ok {
		m.Description = desc
	}
	if version, ok := metadata["version"].(string); ok {
		m.Version = version
	}
	if author, ok := metadata["author"].(string); ok {
		m.Author = author
	}
	if tags, ok := metadata["tags"].([]interface{}); ok {
		for _, t := range tags {
			if tag, ok := t.(string); ok {
				m.Tags = append(m.Tags, tag)
			}
		}
	}
	return m
}

// parseIncludeMap parses a blueprint include from a map.
func parseIncludeMap(blueprintRef string, data map[string]interface{}) Include {
	include := Include{
		Blueprint: blueprintRef,
	}

	if version, ok := data["version"].(string); ok {
		include.Version = version
	}
	if as, ok := data["as"].(string); ok {
		include.As = as
	}
	if entrypoint, ok := data["entrypoint"].(string); ok {
		include.Entrypoint = entrypoint
	}
	if params, ok := data["parameters"].(map[string]interface{}); ok {
		include.Parameters = params
	}
	if features, ok := data["features"].(map[string]interface{}); ok {
		include.Features = make(map[string]bool)
		for k, v := range features {
			if enabled, ok := v.(bool); ok {
				include.Features[k] = enabled
			}
		}
	}

	return include
}

// readFile reads a file and returns its contents.
func readFile(path string) ([]byte, error) {
	absPath, err := filepath.Abs(path)
	if err != nil {
		return nil, err
	}
	// Use os.ReadFile via a simple import
	return readFileContents(absPath)
}

// readFileContents is a helper that reads file contents.
// This avoids importing "os" directly in this file.
var readFileContents = func(path string) ([]byte, error) {
	// This will be set to os.ReadFile in init or can be mocked for testing
	return nil, fmt.Errorf("readFileContents not initialized")
}

// LoadAndResolve loads multiple state files and resolves all includes.
func (r *StateResolver) LoadAndResolve(ctx context.Context, paths []string) ([]*StateFile, error) {
	var stateFiles []*StateFile

	for _, path := range paths {
		resolved, err := r.ParseAndResolve(ctx, path)
		if err != nil {
			return nil, err
		}
		stateFiles = append(stateFiles, resolved)
	}

	return stateFiles, nil
}

// GetAppliedBlueprints returns the list of blueprints that were applied during resolution.
func (r *StateResolver) GetAppliedBlueprints() []*AppliedBlueprint {
	return r.executor.GetAppliedBlueprints()
}

// GetExecutor returns the underlying executor for advanced usage.
func (r *StateResolver) GetExecutor() *Executor {
	return r.executor
}

// GetLoader returns the underlying loader for advanced usage.
func (r *StateResolver) GetLoader() *Loader {
	return r.loader
}

// ValidateBlueprints validates that all referenced blueprints exist and can be loaded.
func (r *StateResolver) ValidateBlueprints(ctx context.Context, stateFile *StateFile) []error {
	var errors []error

	for _, include := range stateFile.BlueprintIncludes {
		// Try to load each blueprint
		loadConfig := &LoadConfig{
			Name:    include.Blueprint,
			Version: include.Version,
		}

		// Set features and parameters
		if include.Features != nil {
			loadConfig.Features = include.Features
		}
		loadConfig.Parameters = include.Parameters

		_, err := r.loader.Load(ctx, loadConfig)
		if err != nil {
			errors = append(errors, fmt.Errorf("blueprint %s: %w", include.Blueprint, err))
		}
	}

	return errors
}

// ResolveDependencies resolves blueprint dependencies recursively.
func (r *StateResolver) ResolveDependencies(ctx context.Context, blueprintName, version string) ([]string, error) {
	loadConfig := &LoadConfig{
		Name:    blueprintName,
		Version: version,
	}

	result, err := r.loader.Load(ctx, loadConfig)
	if err != nil {
		return nil, err
	}

	var deps []string
	if result.Blueprint.Dependencies != nil {
		// Process soft dependencies (Requires)
		for _, dep := range result.Blueprint.Dependencies.Requires {
			deps = append(deps, dep)

			// Recursively resolve dependencies
			subDeps, err := r.ResolveDependencies(ctx, dep, "")
			if err != nil {
				return nil, fmt.Errorf("failed to resolve dependency %s: %w", dep, err)
			}
			deps = append(deps, subDeps...)
		}

		// Process hard dependencies (RequiresBefore)
		for _, dep := range result.Blueprint.Dependencies.RequiresBefore {
			deps = append(deps, dep)

			// Recursively resolve dependencies
			subDeps, err := r.ResolveDependencies(ctx, dep, "")
			if err != nil {
				return nil, fmt.Errorf("failed to resolve dependency %s: %w", dep, err)
			}
			deps = append(deps, subDeps...)
		}
	}

	return deps, nil
}

// Close releases any resources held by the resolver.
func (r *StateResolver) Close() error {
	r.executor.ClearAppliedBlueprints()
	return nil
}

// StateResolverOption is a functional option for configuring the state resolver.
type StateResolverOption func(*StateResolverConfig)

// WithLocalPaths sets the local blueprint paths.
func WithLocalPaths(paths ...string) StateResolverOption {
	return func(c *StateResolverConfig) {
		c.LocalBlueprintPaths = paths
	}
}

// WithCachePath sets the cache path.
func WithCachePath(path string) StateResolverOption {
	return func(c *StateResolverConfig) {
		c.CachePath = path
	}
}

// WithAllowRemote enables remote blueprint fetching.
func WithAllowRemote() StateResolverOption {
	return func(c *StateResolverConfig) {
		c.AllowRemote = true
	}
}

// WithRemoteDisabled disables remote blueprint fetching.
func WithRemoteDisabled() StateResolverOption {
	return func(c *StateResolverConfig) {
		c.AllowRemote = false
	}
}

// WithStrictVersions enables strict version matching.
func WithStrictVersions() StateResolverOption {
	return func(c *StateResolverConfig) {
		c.StrictVersions = true
	}
}

// NewStateResolverWithOptions creates a state resolver with functional options.
func NewStateResolverWithOptions(opts ...StateResolverOption) (*StateResolver, error) {
	config := DefaultStateResolverConfig()
	for _, opt := range opts {
		opt(config)
	}
	return NewStateResolver(config)
}

// init initializes the readFileContents function.
func init() {
	readFileContents = os.ReadFile
}
