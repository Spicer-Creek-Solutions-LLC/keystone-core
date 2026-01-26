package statemgmt

import (
	"context"
	"fmt"
	"path/filepath"

	"github.com/shawnbutts/keystone-core/internal/blueprint"
	"gopkg.in/yaml.v3"
)

// BlueprintResolver resolves blueprint includes in state files.
type BlueprintResolver struct {
	// loader is the blueprint loader
	loader *blueprint.Loader

	// parser is the state parser for parsing rendered states
	parser *Parser
}

// NewBlueprintResolver creates a new blueprint resolver.
func NewBlueprintResolver(storage blueprint.Storage, baseDir string) *BlueprintResolver {
	return &BlueprintResolver{
		loader: blueprint.NewLoader(storage),
		parser: NewParser(baseDir),
	}
}

// ResolvedStateFile represents a state file with all blueprint includes resolved.
type ResolvedStateFile struct {
	// Original is the original state file
	Original *StateFile

	// BlueprintStates contains states from resolved blueprints
	// Key is the blueprint namespace (or blueprint name if no "as" specified)
	BlueprintStates map[string]*ResolvedBlueprint

	// MergedStates contains all states merged together
	MergedStates map[string][]StateDeclaration

	// MergedVariables contains merged variables from all sources
	MergedVariables map[string]interface{}
}

// ResolvedBlueprint represents a resolved blueprint with its states.
type ResolvedBlueprint struct {
	// Include is the original blueprint include directive
	Include *BlueprintInclude

	// LoadResult is the result from loading the blueprint
	LoadResult *blueprint.LoadResult

	// States contains the parsed states from the blueprint
	States []*StateFile

	// Namespace is the namespace for this blueprint instance
	Namespace string
}

// Resolve resolves all blueprint includes in a state file.
func (r *BlueprintResolver) Resolve(ctx context.Context, stateFile *StateFile) (*ResolvedStateFile, error) {
	resolved := &ResolvedStateFile{
		Original:        stateFile,
		BlueprintStates: make(map[string]*ResolvedBlueprint),
		MergedStates:    make(map[string][]StateDeclaration),
		MergedVariables: make(map[string]interface{}),
	}

	// Copy original variables
	for k, v := range stateFile.Variables {
		resolved.MergedVariables[k] = v
	}

	// Copy original states
	for module, decls := range stateFile.States {
		resolved.MergedStates[module] = append(resolved.MergedStates[module], decls...)
	}

	// Resolve each blueprint include
	for i := range stateFile.BlueprintIncludes {
		include := &stateFile.BlueprintIncludes[i]
		resolvedBp, err := r.resolveBlueprint(ctx, include, stateFile.Path)
		if err != nil {
			return nil, fmt.Errorf("failed to resolve blueprint %s: %w", include.Blueprint, err)
		}

		// Store resolved blueprint
		resolved.BlueprintStates[resolvedBp.Namespace] = resolvedBp

		// Merge states from the blueprint
		for _, bpStateFile := range resolvedBp.States {
			for module, decls := range bpStateFile.States {
				// Namespace state IDs if "as" is specified
				namespacedDecls := r.namespaceStateDeclarations(decls, resolvedBp.Namespace, include.As != "")
				resolved.MergedStates[module] = append(resolved.MergedStates[module], namespacedDecls...)
			}

			// Merge variables with namespace prefix
			for k, v := range bpStateFile.Variables {
				varKey := k
				if include.As != "" {
					varKey = include.As + "." + k
				}
				resolved.MergedVariables[varKey] = v
			}
		}
	}

	return resolved, nil
}

// resolveBlueprint resolves a single blueprint include.
func (r *BlueprintResolver) resolveBlueprint(ctx context.Context, include *BlueprintInclude, sourceStatePath string) (*ResolvedBlueprint, error) {
	// Create load config from include
	loadConfig := &blueprint.LoadConfig{
		Name:       include.Blueprint,
		Version:    include.Version,
		Parameters: include.Parameters,
		Features:   include.Features,
		Entrypoint: include.Entrypoint,
		As:         include.As,
		Validate:   true,
	}

	// Load the blueprint
	loadResult, err := r.loader.Load(ctx, loadConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to load blueprint: %w", err)
	}

	// Determine namespace
	namespace := include.As
	if namespace == "" {
		// Use blueprint name as namespace
		namespace = loadResult.Blueprint.Metadata.Name
	}

	resolved := &ResolvedBlueprint{
		Include:    include,
		LoadResult: loadResult,
		States:     make([]*StateFile, 0),
		Namespace:  namespace,
	}

	// Get the blueprint's base directory for state files
	bpBaseDir := loadResult.Blueprint.SourcePath
	if bpBaseDir == "" {
		// If no source path, try to derive from the source state file
		bpBaseDir = filepath.Dir(sourceStatePath)
	}

	// Load and render each state file from the blueprint
	for _, statePath := range loadResult.StateFiles {
		// Render the state file with parameters
		renderedData, err := r.loader.RenderState(ctx, loadResult.Blueprint, statePath, loadResult.ResolvedParameters)
		if err != nil {
			return nil, fmt.Errorf("failed to render state file %s: %w", statePath, err)
		}

		// Parse the rendered state
		tmpParser := NewParser(bpBaseDir)
		parsedState, err := tmpParser.parseBytes(renderedData, statePath)
		if err != nil {
			return nil, fmt.Errorf("failed to parse rendered state %s: %w", statePath, err)
		}

		resolved.States = append(resolved.States, parsedState)
	}

	return resolved, nil
}

// namespaceStateDeclarations prefixes state IDs with a namespace.
func (r *BlueprintResolver) namespaceStateDeclarations(decls []StateDeclaration, namespace string, applyNamespace bool) []StateDeclaration {
	if !applyNamespace {
		return decls
	}

	result := make([]StateDeclaration, len(decls))
	for i, decl := range decls {
		result[i] = decl
		result[i].ID = namespace + ":" + decl.ID

		// Update requisite references
		result[i].Requisites.Require = r.namespaceReferences(decl.Requisites.Require, namespace)
		result[i].Requisites.RequireIn = r.namespaceReferences(decl.Requisites.RequireIn, namespace)
		result[i].Requisites.Watch = r.namespaceReferences(decl.Requisites.Watch, namespace)
		result[i].Requisites.WatchIn = r.namespaceReferences(decl.Requisites.WatchIn, namespace)
		result[i].Requisites.Prereq = r.namespaceReferences(decl.Requisites.Prereq, namespace)
		result[i].Requisites.PrereqIn = r.namespaceReferences(decl.Requisites.PrereqIn, namespace)
		result[i].Requisites.Onchanges = r.namespaceReferences(decl.Requisites.Onchanges, namespace)
		result[i].Requisites.OnchangesIn = r.namespaceReferences(decl.Requisites.OnchangesIn, namespace)
	}
	return result
}

// namespaceReferences prefixes state reference IDs with a namespace.
func (r *BlueprintResolver) namespaceReferences(refs []StateReference, namespace string) []StateReference {
	if len(refs) == 0 {
		return refs
	}

	result := make([]StateReference, len(refs))
	for i, ref := range refs {
		result[i] = StateReference{
			Module: ref.Module,
			ID:     namespace + ":" + ref.ID,
		}
	}
	return result
}

// parseBytes parses state data from bytes (adds this helper to Parser).
func (p *Parser) parseBytes(data []byte, path string) (*StateFile, error) {
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
				stateFile.Includes = append(stateFile.Includes, v)
			case map[string]interface{}:
				if blueprintRef, ok := v["blueprint"].(string); ok {
					bpInclude := parseBlueprintInclude(blueprintRef, v)
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

