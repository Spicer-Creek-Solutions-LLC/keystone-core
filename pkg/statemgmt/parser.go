package statemgmt

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// Parser parses state files
type Parser struct {
	// BaseDir is the base directory for resolving relative paths
	BaseDir string

	// Format is the state file format
	Format StateFileFormat
}

// NewParser creates a new state parser
func NewParser(baseDir string) *Parser {
	return &Parser{
		BaseDir: baseDir,
		Format:  FormatYAML,
	}
}

// ParseFile parses a state file
func (p *Parser) ParseFile(path string) (*StateFile, error) {
	// Read file
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read state file: %w", err)
	}

	// Parse based on format
	var rawState map[string]interface{}
	if err := yaml.Unmarshal(data, &rawState); err != nil {
		return nil, fmt.Errorf("failed to parse YAML: %w", err)
	}

	// Convert to StateFile
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
				// Simple file include
				stateFile.Includes = append(stateFile.Includes, v)
			case map[string]interface{}:
				// Check if this is a blueprint include or file include
				if blueprintRef, ok := v["blueprint"].(string); ok {
					// Blueprint include
					bpInclude := parseBlueprintInclude(blueprintRef, v)
					stateFile.BlueprintIncludes = append(stateFile.BlueprintIncludes, bpInclude)
				} else if fileRef, ok := v["file"].(string); ok {
					// File include with extra options (treat as simple include for now)
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
			return nil, fmt.Errorf("invalid state declarations for module %s: expected map", module)
		}

		for stateID, params := range declMap {
			paramsMap, ok := params.(map[string]interface{})
			if !ok {
				return nil, fmt.Errorf("invalid parameters for state %s.%s: expected map", module, stateID)
			}

			decl, err := p.parseStateDeclaration(module, stateID, paramsMap)
			if err != nil {
				return nil, fmt.Errorf("failed to parse state %s.%s: %w", module, stateID, err)
			}

			stateFile.States[module] = append(stateFile.States[module], *decl)
		}
	}

	return stateFile, nil
}

// parseMetadata parses metadata section
func parseMetadata(raw map[string]interface{}) StateMetadata {
	metadata := StateMetadata{}

	if name, ok := raw["name"].(string); ok {
		metadata.Name = name
	}
	if desc, ok := raw["description"].(string); ok {
		metadata.Description = desc
	}
	if version, ok := raw["version"].(string); ok {
		metadata.Version = version
	}
	if tags, ok := raw["tags"].([]interface{}); ok {
		for _, tag := range tags {
			if tagStr, ok := tag.(string); ok {
				metadata.Tags = append(metadata.Tags, tagStr)
			}
		}
	}

	return metadata
}

// parseStateDeclaration parses a single state declaration
func (p *Parser) parseStateDeclaration(module, stateID string, params map[string]interface{}) (*StateDeclaration, error) {
	decl := &StateDeclaration{
		ID:         stateID,
		Module:     module,
		Parameters: make(map[string]interface{}),
	}

	// Parse standard fields
	if state, ok := params["state"].(string); ok {
		decl.State = state
		delete(params, "state")
	} else {
		// Default state varies by module
		decl.State = getDefaultState(module)
	}

	// Parse requisites
	decl.Requisites = parseRequisites(params)

	// Parse order
	if order, ok := params["order"].(int); ok {
		decl.Order = order
		delete(params, "order")
	}

	// Parse fail_hard
	if failHard, ok := params["fail_hard"].(bool); ok {
		decl.FailHard = failHard
		delete(params, "fail_hard")
	}

	// Parse unless
	if unless, ok := params["unless"].(string); ok {
		decl.Unless = unless
		delete(params, "unless")
	}

	// Parse only_if
	if onlyIf, ok := params["only_if"].(string); ok {
		decl.OnlyIf = onlyIf
		delete(params, "only_if")
	}

	// Parse retry
	if retry, ok := params["retry"].(map[string]interface{}); ok {
		decl.Retry = parseRetry(retry)
		delete(params, "retry")
	}

	// Remaining fields are module-specific parameters
	for key, value := range params {
		// Skip requisite fields (already parsed)
		if isRequisiteField(key) {
			continue
		}
		decl.Parameters[key] = value
	}

	return decl, nil
}

// parseRequisites parses requisite declarations
func parseRequisites(params map[string]interface{}) Requisites {
	reqs := Requisites{}

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

// parseStateReferences parses state reference lists
func parseStateReferences(refs []interface{}) []StateReference {
	var result []StateReference

	for _, ref := range refs {
		refMap, ok := ref.(map[string]interface{})
		if !ok {
			continue
		}

		// Each reference should have one key (module type) with value (state ID)
		for module, id := range refMap {
			if idStr, ok := id.(string); ok {
				result = append(result, StateReference{
					Module: module,
					ID:     idStr,
				})
			}
		}
	}

	return result
}

// parseRetry parses retry configuration
func parseRetry(retry map[string]interface{}) *RetryConfig {
	config := &RetryConfig{}

	if attempts, ok := retry["attempts"].(int); ok {
		config.Attempts = attempts
	}

	if delay, ok := retry["delay"].(string); ok {
		if d, err := parseDuration(delay); err == nil {
			config.Delay = d
		}
	}

	if multiplier, ok := retry["backoff_multiplier"].(float64); ok {
		config.BackoffMultiplier = multiplier
	}

	if maxDelay, ok := retry["max_delay"].(string); ok {
		if d, err := parseDuration(maxDelay); err == nil {
			config.MaxDelay = d
		}
	}

	return config
}

// isRequisiteField checks if a field is a requisite field
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

// getDefaultState returns the default state for a module
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

// parseDuration parses a duration string (e.g., "10s", "1m", "2h")
func parseDuration(s string) (time.Duration, error) {
	// Simple duration parsing - could use time.ParseDuration
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, fmt.Errorf("empty duration")
	}

	// For now, just use standard Go duration format
	return time.ParseDuration(s)
}

// ParseSource parses a source string and returns the type and path
func ParseSource(source string) (SourceType, string) {
	if strings.HasPrefix(source, "http://") || strings.HasPrefix(source, "https://") {
		return SourceTypeHTTP, source
	}

	if strings.HasPrefix(source, "file://") {
		return SourceTypeFile, strings.TrimPrefix(source, "file://")
	}

	if strings.HasPrefix(source, "template://") {
		return SourceTypeTemplate, strings.TrimPrefix(source, "template://")
	}

	// Default to file path
	return SourceTypeFile, source
}

// parseBlueprintInclude parses a blueprint include from a map
func parseBlueprintInclude(blueprintRef string, data map[string]interface{}) BlueprintInclude {
	include := BlueprintInclude{
		Blueprint: blueprintRef,
	}

	// Parse version
	if version, ok := data["version"].(string); ok {
		include.Version = version
	}

	// Parse as (namespace)
	if as, ok := data["as"].(string); ok {
		include.As = as
	}

	// Parse entrypoint
	if entrypoint, ok := data["entrypoint"].(string); ok {
		include.Entrypoint = entrypoint
	}

	// Parse features
	if features, ok := data["features"].(map[string]interface{}); ok {
		include.Features = make(map[string]bool)
		for name, enabled := range features {
			if enabledBool, ok := enabled.(bool); ok {
				include.Features[name] = enabledBool
			}
		}
	}

	// Parse parameters
	if params, ok := data["params"].(map[string]interface{}); ok {
		include.Parameters = params
	} else if params, ok := data["parameters"].(map[string]interface{}); ok {
		// Also support "parameters" key
		include.Parameters = params
	}

	return include
}

// LoadStateFiles loads multiple state files and resolves includes
func (p *Parser) LoadStateFiles(paths []string) ([]*StateFile, error) {
	var stateFiles []*StateFile
	visited := make(map[string]bool)

	for _, path := range paths {
		files, err := p.loadStateFileRecursive(path, visited)
		if err != nil {
			return nil, err
		}
		stateFiles = append(stateFiles, files...)
	}

	return stateFiles, nil
}

// loadStateFileRecursive loads a state file and recursively loads includes
func (p *Parser) loadStateFileRecursive(path string, visited map[string]bool) ([]*StateFile, error) {
	// Resolve absolute path
	absPath, err := filepath.Abs(path)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve path %s: %w", path, err)
	}

	// Check if already visited (prevent circular includes)
	if visited[absPath] {
		return nil, nil
	}
	visited[absPath] = true

	// Parse the file
	stateFile, err := p.ParseFile(absPath)
	if err != nil {
		return nil, err
	}

	result := []*StateFile{stateFile}

	// Recursively load includes
	for _, include := range stateFile.Includes {
		// Resolve include path relative to current file's directory
		includeDir := filepath.Dir(absPath)
		includePath := filepath.Join(includeDir, include)

		included, err := p.loadStateFileRecursive(includePath, visited)
		if err != nil {
			return nil, fmt.Errorf("failed to load include %s: %w", include, err)
		}

		result = append(result, included...)
	}

	return result, nil
}
