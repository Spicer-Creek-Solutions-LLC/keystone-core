package blueprint

import (
	"fmt"
	"sort"
	"strings"
	"sync"
)

// DependencyType indicates how a dependency should be handled during execution.
type DependencyType int

const (
	// DependencyTypeSoft (requires) - must be included, can execute concurrently.
	DependencyTypeSoft DependencyType = iota

	// DependencyTypeHard (requires_before) - must complete before dependent starts.
	DependencyTypeHard
)

// String returns a human-readable name for the dependency type.
func (d DependencyType) String() string {
	switch d {
	case DependencyTypeSoft:
		return "soft"
	case DependencyTypeHard:
		return "hard"
	default:
		return "unknown"
	}
}

// BlueprintDependency represents a dependency on another blueprint.
type BlueprintDependency struct {
	// Blueprint is the reference (e.g., "blueprints/community/ssl-certificates@^2.0")
	Blueprint string

	// Type indicates soft (requires) or hard (requires_before) dependency
	Type DependencyType

	// Resolved is the resolved blueprint after loading
	Resolved *Blueprint

	// Namespace is the instance namespace (from "as:" directive)
	Namespace string
}

// BlueprintInstance represents a blueprint with a specific namespace for multi-instance support.
type BlueprintInstance struct {
	// Blueprint is the resolved blueprint
	Blueprint *Blueprint

	// Namespace is the instance namespace (empty for default instance)
	Namespace string

	// Parameters are the resolved parameters for this instance
	Parameters map[string]interface{}

	// EnabledFeatures tracks which features are enabled
	EnabledFeatures map[string]bool

	// EnabledStates tracks which state files are conditionally enabled
	// These are populated from Feature.Enables when features are enabled
	EnabledStates []string

	// Dependencies are the resolved dependencies
	Dependencies []*BlueprintDependency

	// EntryPoint is the selected entry point (default or named)
	EntryPoint string
}

// InstanceID returns a unique identifier for this instance.
func (i *BlueprintInstance) InstanceID() string {
	if i.Namespace != "" {
		return fmt.Sprintf("%s:%s@%s", i.Blueprint.Metadata.Name, i.Namespace, i.Blueprint.Metadata.Version)
	}
	return fmt.Sprintf("%s@%s", i.Blueprint.Metadata.Name, i.Blueprint.Metadata.Version)
}

// FullName returns the blueprint name with namespace prefix if applicable.
func (i *BlueprintInstance) FullName() string {
	if i.Namespace != "" {
		return fmt.Sprintf("%s:%s", i.Namespace, i.Blueprint.Metadata.Name)
	}
	return i.Blueprint.Metadata.Name
}

// IsStateEnabled returns true if the given state file is conditionally enabled.
// If no features enable states, this returns false (base states are always included).
func (i *BlueprintInstance) IsStateEnabled(statePath string) bool {
	for _, s := range i.EnabledStates {
		if s == statePath {
			return true
		}
	}
	return false
}

// GetEnabledStates returns a copy of the enabled state files.
func (i *BlueprintInstance) GetEnabledStates() []string {
	if len(i.EnabledStates) == 0 {
		return nil
	}
	result := make([]string, len(i.EnabledStates))
	copy(result, i.EnabledStates)
	return result
}

// DependencyNode represents a node in the dependency graph.
type DependencyNode struct {
	// Instance is the blueprint instance
	Instance *BlueprintInstance

	// HardDependencies are blueprints that must complete before this one
	HardDependencies []string

	// SoftDependencies are blueprints that can run concurrently
	SoftDependencies []string
}

// DependencyGraph manages blueprint dependencies and execution ordering.
type DependencyGraph struct {
	nodes      map[string]*DependencyNode
	edges      map[string][]string // hard edges: dependency -> dependents
	softEdges  map[string][]string // soft edges for reference
	mu         sync.RWMutex
}

// NewDependencyGraph creates a new dependency graph.
func NewDependencyGraph() *DependencyGraph {
	return &DependencyGraph{
		nodes:     make(map[string]*DependencyNode),
		edges:     make(map[string][]string),
		softEdges: make(map[string][]string),
	}
}

// AddInstance adds a blueprint instance to the graph.
func (g *DependencyGraph) AddInstance(instance *BlueprintInstance) error {
	if instance == nil {
		return fmt.Errorf("instance cannot be nil")
	}

	if instance.Blueprint == nil {
		return fmt.Errorf("instance blueprint cannot be nil")
	}

	g.mu.Lock()
	defer g.mu.Unlock()

	instanceID := instance.InstanceID()

	// Create node
	node := &DependencyNode{
		Instance:         instance,
		HardDependencies: make([]string, 0),
		SoftDependencies: make([]string, 0),
	}

	// Process dependencies
	for _, dep := range instance.Dependencies {
		if dep.Resolved == nil {
			continue
		}

		depID := dep.Resolved.Metadata.Name
		if dep.Namespace != "" {
			depID = fmt.Sprintf("%s:%s", dep.Namespace, dep.Resolved.Metadata.Name)
		}
		depID = fmt.Sprintf("%s@%s", depID, dep.Resolved.Metadata.Version)

		switch dep.Type {
		case DependencyTypeHard:
			node.HardDependencies = append(node.HardDependencies, depID)
			// Add edge for topological sort
			g.edges[depID] = appendUnique(g.edges[depID], instanceID)
		case DependencyTypeSoft:
			node.SoftDependencies = append(node.SoftDependencies, depID)
			// Track soft edges for reference
			g.softEdges[depID] = appendUnique(g.softEdges[depID], instanceID)
		}
	}

	g.nodes[instanceID] = node
	return nil
}

// GetNode returns a node by instance ID.
func (g *DependencyGraph) GetNode(instanceID string) (*DependencyNode, bool) {
	g.mu.RLock()
	defer g.mu.RUnlock()

	node, exists := g.nodes[instanceID]
	return node, exists
}

// GetAllNodes returns all nodes in the graph.
func (g *DependencyGraph) GetAllNodes() []*DependencyNode {
	g.mu.RLock()
	defer g.mu.RUnlock()

	nodes := make([]*DependencyNode, 0, len(g.nodes))
	for _, node := range g.nodes {
		nodes = append(nodes, node)
	}

	// Sort by instance ID for consistent ordering
	sort.Slice(nodes, func(i, j int) bool {
		return nodes[i].Instance.InstanceID() < nodes[j].Instance.InstanceID()
	})

	return nodes
}

// HasCycle detects if the graph has a cycle in hard dependencies.
// Returns true if a cycle exists, along with the cycle path.
func (g *DependencyGraph) HasCycle() (bool, []string) {
	g.mu.RLock()
	defer g.mu.RUnlock()

	visited := make(map[string]bool)
	recStack := make(map[string]bool)
	parent := make(map[string]string)

	for instanceID := range g.nodes {
		if !visited[instanceID] {
			hasCycle, cyclePath := g.dfs(instanceID, visited, recStack, parent)
			if hasCycle {
				return true, cyclePath
			}
		}
	}

	return false, nil
}

// dfs performs depth-first search to detect cycles.
func (g *DependencyGraph) dfs(instanceID string, visited, recStack map[string]bool, parent map[string]string) (bool, []string) {
	visited[instanceID] = true
	recStack[instanceID] = true

	// Check all dependents (instances that depend on this one)
	for _, dep := range g.edges[instanceID] {
		if !visited[dep] {
			parent[dep] = instanceID
			hasCycle, cyclePath := g.dfs(dep, visited, recStack, parent)
			if hasCycle {
				return true, cyclePath
			}
		} else if recStack[dep] {
			// Found a cycle - reconstruct the path
			cycle := []string{dep}
			current := instanceID
			for current != dep && current != "" {
				cycle = append([]string{current}, cycle...)
				current = parent[current]
			}
			cycle = append(cycle, dep) // Complete the cycle
			return true, cycle
		}
	}

	recStack[instanceID] = false
	return false, nil
}

// ExecutionLevel represents a group of blueprints that can execute concurrently.
type ExecutionLevel struct {
	// Level is the execution order (0 = first, higher = later)
	Level int

	// Instances are the blueprint instances at this level
	Instances []*BlueprintInstance

	// CanRunConcurrently indicates all instances at this level can run in parallel
	CanRunConcurrently bool
}

// GetExecutionOrder returns the execution order respecting hard dependencies.
// Blueprints at the same level can potentially run concurrently.
func (g *DependencyGraph) GetExecutionOrder() ([]*ExecutionLevel, error) {
	g.mu.RLock()
	defer g.mu.RUnlock()

	// Check for cycles
	if hasCycle, cyclePath := g.HasCycle(); hasCycle {
		return nil, fmt.Errorf("circular dependency detected: %s", strings.Join(cyclePath, " -> "))
	}

	// Calculate in-degree for each node (only considering hard dependencies)
	inDegree := make(map[string]int)
	for instanceID := range g.nodes {
		inDegree[instanceID] = 0
	}

	for _, dependents := range g.edges {
		for _, dep := range dependents {
			inDegree[dep]++
		}
	}

	// Build execution levels
	levels := make([]*ExecutionLevel, 0)
	processed := make(map[string]bool)

	for len(processed) < len(g.nodes) {
		// Find all nodes with in-degree 0
		currentLevel := make([]*BlueprintInstance, 0)

		for instanceID, degree := range inDegree {
			if degree == 0 && !processed[instanceID] {
				if node, exists := g.nodes[instanceID]; exists {
					currentLevel = append(currentLevel, node.Instance)
					processed[instanceID] = true
				}
			}
		}

		if len(currentLevel) == 0 && len(processed) < len(g.nodes) {
			// This shouldn't happen if cycle detection works
			return nil, fmt.Errorf("unable to resolve execution order: remaining unprocessed nodes")
		}

		if len(currentLevel) > 0 {
			// Sort for deterministic ordering
			sort.Slice(currentLevel, func(i, j int) bool {
				return currentLevel[i].InstanceID() < currentLevel[j].InstanceID()
			})

			levels = append(levels, &ExecutionLevel{
				Level:              len(levels),
				Instances:          currentLevel,
				CanRunConcurrently: true,
			})

			// Reduce in-degree for dependents
			for _, instance := range currentLevel {
				for _, dependent := range g.edges[instance.InstanceID()] {
					inDegree[dependent]--
				}
			}
		}
	}

	return levels, nil
}

// GetHardDependencies returns the hard dependencies for an instance.
func (g *DependencyGraph) GetHardDependencies(instanceID string) ([]string, error) {
	g.mu.RLock()
	defer g.mu.RUnlock()

	node, exists := g.nodes[instanceID]
	if !exists {
		return nil, fmt.Errorf("instance not found: %s", instanceID)
	}

	result := make([]string, len(node.HardDependencies))
	copy(result, node.HardDependencies)
	return result, nil
}

// GetSoftDependencies returns the soft dependencies for an instance.
func (g *DependencyGraph) GetSoftDependencies(instanceID string) ([]string, error) {
	g.mu.RLock()
	defer g.mu.RUnlock()

	node, exists := g.nodes[instanceID]
	if !exists {
		return nil, fmt.Errorf("instance not found: %s", instanceID)
	}

	result := make([]string, len(node.SoftDependencies))
	copy(result, node.SoftDependencies)
	return result, nil
}

// GetDependents returns all instances that depend on the given instance.
func (g *DependencyGraph) GetDependents(instanceID string) []string {
	g.mu.RLock()
	defer g.mu.RUnlock()

	// Combine hard and soft dependents
	result := make([]string, 0)
	seen := make(map[string]bool)

	for _, dep := range g.edges[instanceID] {
		if !seen[dep] {
			result = append(result, dep)
			seen[dep] = true
		}
	}

	for _, dep := range g.softEdges[instanceID] {
		if !seen[dep] {
			result = append(result, dep)
			seen[dep] = true
		}
	}

	sort.Strings(result)
	return result
}

// Size returns the number of nodes in the graph.
func (g *DependencyGraph) Size() int {
	g.mu.RLock()
	defer g.mu.RUnlock()
	return len(g.nodes)
}

// Clear removes all nodes and edges from the graph.
func (g *DependencyGraph) Clear() {
	g.mu.Lock()
	defer g.mu.Unlock()

	g.nodes = make(map[string]*DependencyNode)
	g.edges = make(map[string][]string)
	g.softEdges = make(map[string][]string)
}

// DependencyResolver resolves blueprint dependencies and builds execution order.
type DependencyResolver struct {
	// loader loads blueprints by reference
	loader BlueprintLoader

	// graph is the dependency graph
	graph *DependencyGraph

	// instances tracks resolved instances by ID
	instances map[string]*BlueprintInstance

	// mu protects concurrent access
	mu sync.Mutex
}

// BlueprintLoader loads blueprints by reference.
type BlueprintLoader interface {
	// Load loads a blueprint by reference (e.g., "blueprints/community/ssl-certificates@^2.0")
	Load(reference string) (*Blueprint, error)
}

// NewDependencyResolver creates a new dependency resolver.
func NewDependencyResolver(loader BlueprintLoader) *DependencyResolver {
	return &DependencyResolver{
		loader:    loader,
		graph:     NewDependencyGraph(),
		instances: make(map[string]*BlueprintInstance),
	}
}

// ResolutionResult contains the result of dependency resolution.
type ResolutionResult struct {
	// Instances are all resolved blueprint instances
	Instances []*BlueprintInstance

	// ExecutionLevels are the execution order levels
	ExecutionLevels []*ExecutionLevel

	// Errors encountered during resolution
	Errors []error
}

// Resolve resolves all blueprint dependencies from a list of includes.
func (r *DependencyResolver) Resolve(includes []BlueprintInclude) (*ResolutionResult, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	// Reset state
	r.graph.Clear()
	r.instances = make(map[string]*BlueprintInstance)

	result := &ResolutionResult{
		Instances: make([]*BlueprintInstance, 0),
		Errors:    make([]error, 0),
	}

	// First pass: resolve all directly included blueprints
	for _, include := range includes {
		instance, err := r.resolveInclude(include)
		if err != nil {
			result.Errors = append(result.Errors, err)
			continue
		}

		if instance != nil {
			result.Instances = append(result.Instances, instance)
		}
	}

	// Second pass: resolve transitive dependencies
	for _, instance := range result.Instances {
		if err := r.resolveTransitiveDependencies(instance); err != nil {
			result.Errors = append(result.Errors, err)
		}
	}

	// Add all instances to graph
	for _, instance := range r.instances {
		if err := r.graph.AddInstance(instance); err != nil {
			result.Errors = append(result.Errors, err)
		}
	}

	// Get execution order
	levels, err := r.graph.GetExecutionOrder()
	if err != nil {
		result.Errors = append(result.Errors, err)
	} else {
		result.ExecutionLevels = levels
	}

	// Update instances from graph order
	result.Instances = make([]*BlueprintInstance, 0)
	for _, level := range result.ExecutionLevels {
		result.Instances = append(result.Instances, level.Instances...)
	}

	return result, nil
}

// resolveInclude resolves a single blueprint include.
func (r *DependencyResolver) resolveInclude(include BlueprintInclude) (*BlueprintInstance, error) {
	// Build full reference
	ref := include.Blueprint
	if include.Version != "" {
		ref = fmt.Sprintf("%s@%s", ref, include.Version)
	}

	// Load blueprint
	bp, err := r.loader.Load(ref)
	if err != nil {
		return nil, fmt.Errorf("failed to load blueprint %s: %w", ref, err)
	}

	// Create instance
	instance := &BlueprintInstance{
		Blueprint:       bp,
		Namespace:       include.As,
		Parameters:      include.Parameters,
		EnabledFeatures: make(map[string]bool),
		Dependencies:    make([]*BlueprintDependency, 0),
		EntryPoint:      include.Entrypoint,
	}

	// Evaluate features
	if err := r.evaluateFeatures(instance, include.Features); err != nil {
		return nil, err
	}

	// Extract dependencies from blueprint
	if err := r.extractDependencies(instance); err != nil {
		return nil, err
	}

	// Register instance
	r.instances[instance.InstanceID()] = instance

	return instance, nil
}

// evaluateFeatures evaluates feature flags for an instance.
func (r *DependencyResolver) evaluateFeatures(instance *BlueprintInstance, userFeatures map[string]bool) error {
	bp := instance.Blueprint

	// Start with default feature values
	for name, feature := range bp.Features {
		instance.EnabledFeatures[name] = feature.Default
	}

	// Apply user overrides
	for name, enabled := range userFeatures {
		if _, exists := bp.Features[name]; !exists {
			return fmt.Errorf("unknown feature flag: %s", name)
		}
		instance.EnabledFeatures[name] = enabled
	}

	// Compute enabled states from feature flags
	instance.EnabledStates = r.computeEnabledStates(instance)

	return nil
}

// computeEnabledStates computes which state files are enabled based on feature flags.
func (r *DependencyResolver) computeEnabledStates(instance *BlueprintInstance) []string {
	bp := instance.Blueprint
	enabledStates := make(map[string]bool)

	for name, enabled := range instance.EnabledFeatures {
		if !enabled {
			continue
		}

		feature, exists := bp.Features[name]
		if !exists {
			continue
		}

		// Add states enabled by this feature
		for _, state := range feature.Enables {
			enabledStates[state] = true
		}
	}

	// Convert map to sorted slice for deterministic ordering
	result := make([]string, 0, len(enabledStates))
	for state := range enabledStates {
		result = append(result, state)
	}
	sort.Strings(result)

	return result
}

// extractDependencies extracts dependencies from a blueprint.
func (r *DependencyResolver) extractDependencies(instance *BlueprintInstance) error {
	bp := instance.Blueprint

	// Handle nil Dependencies (blueprint has no dependencies)
	if bp.Dependencies != nil {
		// Extract from dependencies.requires (soft)
		for _, req := range bp.Dependencies.Requires {
			instance.Dependencies = append(instance.Dependencies, &BlueprintDependency{
				Blueprint: req,
				Type:      DependencyTypeSoft,
			})
		}

		// Extract from dependencies.requires_before (hard)
		for _, req := range bp.Dependencies.RequiresBefore {
			instance.Dependencies = append(instance.Dependencies, &BlueprintDependency{
				Blueprint: req,
				Type:      DependencyTypeHard,
			})
		}
	}

	// Extract from features that add dependencies
	for name, enabled := range instance.EnabledFeatures {
		if !enabled {
			continue
		}

		feature, exists := bp.Features[name]
		if !exists {
			continue
		}

		for _, req := range feature.Requires {
			// Check if dependency already exists
			found := false
			for _, dep := range instance.Dependencies {
				if dep.Blueprint == req {
					found = true
					break
				}
			}
			if !found {
				instance.Dependencies = append(instance.Dependencies, &BlueprintDependency{
					Blueprint: req,
					Type:      DependencyTypeSoft, // Feature dependencies are soft by default
				})
			}
		}
	}

	return nil
}

// resolveTransitiveDependencies resolves transitive dependencies for an instance.
func (r *DependencyResolver) resolveTransitiveDependencies(instance *BlueprintInstance) error {
	for _, dep := range instance.Dependencies {
		// Load dependency blueprint
		bp, err := r.loader.Load(dep.Blueprint)
		if err != nil {
			return fmt.Errorf("failed to load dependency %s: %w", dep.Blueprint, err)
		}

		dep.Resolved = bp

		// Create instance for dependency if not already resolved
		depInstance := &BlueprintInstance{
			Blueprint:       bp,
			Namespace:       dep.Namespace,
			Parameters:      make(map[string]interface{}),
			EnabledFeatures: make(map[string]bool),
			Dependencies:    make([]*BlueprintDependency, 0),
		}

		depID := depInstance.InstanceID()
		if _, exists := r.instances[depID]; !exists {
			// Set default features
			for name, feature := range bp.Features {
				depInstance.EnabledFeatures[name] = feature.Default
			}

			// Extract nested dependencies
			if err := r.extractDependencies(depInstance); err != nil {
				return err
			}

			r.instances[depID] = depInstance

			// Recursively resolve transitive dependencies
			if err := r.resolveTransitiveDependencies(depInstance); err != nil {
				return err
			}
		}
	}

	return nil
}

// GetGraph returns the dependency graph.
func (r *DependencyResolver) GetGraph() *DependencyGraph {
	return r.graph
}

// appendUnique appends an item to a slice if it doesn't already exist.
func appendUnique(slice []string, item string) []string {
	for _, s := range slice {
		if s == item {
			return slice
		}
	}
	return append(slice, item)
}
