package statemgmt

import (
	"fmt"
	"sort"
)

// DependencyResolver resolves state dependencies and creates execution order
type DependencyResolver struct {
	states map[string]*StateDeclaration
	graph  map[string][]string // adjacency list: state -> dependencies
	levels [][]string          // states grouped by dependency level
}

// NewDependencyResolver creates a new dependency resolver
func NewDependencyResolver() *DependencyResolver {
	return &DependencyResolver{
		states: make(map[string]*StateDeclaration),
		graph:  make(map[string][]string),
		levels: make([][]string, 0),
	}
}

// AddState adds a state to the resolver
func (r *DependencyResolver) AddState(decl *StateDeclaration) {
	stateKey := r.makeStateKey(decl.Module, decl.ID)
	r.states[stateKey] = decl
}

// BuildGraph builds the dependency graph from state requisites
func (r *DependencyResolver) BuildGraph() error {
	// First pass: create nodes
	for key := range r.states {
		if r.graph[key] == nil {
			r.graph[key] = make([]string, 0)
		}
	}

	// Second pass: add edges based on requisites
	for key, decl := range r.states {
		// Process all requisite types
		if err := r.addRequisites(key, decl); err != nil {
			return err
		}
	}

	return nil
}

// addRequisites processes requisites and adds edges to the graph
func (r *DependencyResolver) addRequisites(stateKey string, decl *StateDeclaration) error {
	// Require: this state depends on these states
	// Edge: dependency -> state (dependency must run before state)
	for _, ref := range decl.Requisites.Require {
		depKey := r.makeStateKey(ref.Module, ref.ID)
		if !r.stateExists(depKey) {
			return fmt.Errorf("state %s requires non-existent state %s.%s", stateKey, ref.Module, ref.ID)
		}
		r.graph[depKey] = append(r.graph[depKey], stateKey)
	}

	// RequireIn: this state must run before other states
	// Edge: state -> target (state must run before target)
	for _, ref := range decl.Requisites.RequireIn {
		targetKey := r.makeStateKey(ref.Module, ref.ID)
		if !r.stateExists(targetKey) {
			return fmt.Errorf("state %s has require_in for non-existent state %s.%s", stateKey, ref.Module, ref.ID)
		}
		r.graph[stateKey] = append(r.graph[stateKey], targetKey)
	}

	// Watch: same as require for execution order
	// Edge: dependency -> state (dependency must run before state)
	for _, ref := range decl.Requisites.Watch {
		depKey := r.makeStateKey(ref.Module, ref.ID)
		if !r.stateExists(depKey) {
			return fmt.Errorf("state %s watches non-existent state %s.%s", stateKey, ref.Module, ref.ID)
		}
		r.graph[depKey] = append(r.graph[depKey], stateKey)
	}

	// WatchIn: reverse watch
	// Edge: state -> target (state must run before target)
	for _, ref := range decl.Requisites.WatchIn {
		targetKey := r.makeStateKey(ref.Module, ref.ID)
		if !r.stateExists(targetKey) {
			return fmt.Errorf("state %s has watch_in for non-existent state %s.%s", stateKey, ref.Module, ref.ID)
		}
		r.graph[stateKey] = append(r.graph[stateKey], targetKey)
	}

	// Prereq: prerequisite dependency
	// Edge: dependency -> state (dependency must run before state)
	for _, ref := range decl.Requisites.Prereq {
		depKey := r.makeStateKey(ref.Module, ref.ID)
		if !r.stateExists(depKey) {
			return fmt.Errorf("state %s has prereq for non-existent state %s.%s", stateKey, ref.Module, ref.ID)
		}
		r.graph[depKey] = append(r.graph[depKey], stateKey)
	}

	// PrereqIn: reverse prereq
	// Edge: state -> target (state must run before target)
	for _, ref := range decl.Requisites.PrereqIn {
		targetKey := r.makeStateKey(ref.Module, ref.ID)
		if !r.stateExists(targetKey) {
			return fmt.Errorf("state %s has prereq_in for non-existent state %s.%s", stateKey, ref.Module, ref.ID)
		}
		r.graph[stateKey] = append(r.graph[stateKey], targetKey)
	}

	// Onchanges: only run if dependency changed (still creates dependency)
	// Edge: dependency -> state (dependency must run before state)
	for _, ref := range decl.Requisites.Onchanges {
		depKey := r.makeStateKey(ref.Module, ref.ID)
		if !r.stateExists(depKey) {
			return fmt.Errorf("state %s has onchanges for non-existent state %s.%s", stateKey, ref.Module, ref.ID)
		}
		r.graph[depKey] = append(r.graph[depKey], stateKey)
	}

	// OnchangesIn: reverse onchanges
	// Edge: state -> target (state must run before target)
	for _, ref := range decl.Requisites.OnchangesIn {
		targetKey := r.makeStateKey(ref.Module, ref.ID)
		if !r.stateExists(targetKey) {
			return fmt.Errorf("state %s has onchanges_in for non-existent state %s.%s", stateKey, ref.Module, ref.ID)
		}
		r.graph[stateKey] = append(r.graph[stateKey], targetKey)
	}

	return nil
}

// TopologicalSort performs topological sort to get execution order
func (r *DependencyResolver) TopologicalSort() ([]*StateDeclaration, error) {
	// Kahn's algorithm for topological sort
	inDegree := make(map[string]int)

	// Calculate in-degrees
	for node := range r.graph {
		if _, exists := inDegree[node]; !exists {
			inDegree[node] = 0
		}
	}

	for _, deps := range r.graph {
		for _, dep := range deps {
			inDegree[dep]++
		}
	}

	// Find all nodes with in-degree 0
	queue := make([]string, 0)
	for node, degree := range inDegree {
		if degree == 0 {
			queue = append(queue, node)
		}
	}

	// Sort queue for deterministic output
	sort.Strings(queue)

	result := make([]*StateDeclaration, 0)
	processed := 0

	for len(queue) > 0 {
		// Process all nodes at current level (for parallel execution)
		currentLevel := make([]string, len(queue))
		copy(currentLevel, queue)
		r.levels = append(r.levels, currentLevel)

		queue = make([]string, 0)

		for _, node := range currentLevel {
			result = append(result, r.states[node])
			processed++

			// Reduce in-degree for neighbors
			for _, dep := range r.graph[node] {
				inDegree[dep]--
				if inDegree[dep] == 0 {
					queue = append(queue, dep)
				}
			}
		}

		// Sort queue for deterministic output
		sort.Strings(queue)
	}

	// Check for cycles
	if processed != len(r.states) {
		return nil, r.findCycle()
	}

	return result, nil
}

// findCycle detects and reports a circular dependency
func (r *DependencyResolver) findCycle() error {
	visited := make(map[string]bool)
	recStack := make(map[string]bool)
	path := make([]string, 0)

	for node := range r.graph {
		if !visited[node] {
			if cycle := r.dfsCycle(node, visited, recStack, path); cycle != nil {
				return fmt.Errorf("circular dependency detected: %v", cycle)
			}
		}
	}

	return fmt.Errorf("circular dependency detected but cycle not found")
}

// dfsCycle performs DFS to find cycles
func (r *DependencyResolver) dfsCycle(node string, visited, recStack map[string]bool, path []string) []string {
	visited[node] = true
	recStack[node] = true
	path = append(path, node)

	for _, dep := range r.graph[node] {
		if !visited[dep] {
			if cycle := r.dfsCycle(dep, visited, recStack, path); cycle != nil {
				return cycle
			}
		} else if recStack[dep] {
			// Found cycle, extract it
			cycleStart := 0
			for i, p := range path {
				if p == dep {
					cycleStart = i
					break
				}
			}
			cycle := make([]string, 0)
			cycle = append(cycle, path[cycleStart:]...)
			cycle = append(cycle, dep)
			return cycle
		}
	}

	recStack[node] = false
	return nil
}

// GetExecutionLevels returns states grouped by dependency level for parallel execution
func (r *DependencyResolver) GetExecutionLevels() [][]string {
	return r.levels
}

// GetParallelizableGroups returns states that can be executed in parallel
func (r *DependencyResolver) GetParallelizableGroups() [][]*StateDeclaration {
	groups := make([][]*StateDeclaration, len(r.levels))

	for i, level := range r.levels {
		group := make([]*StateDeclaration, len(level))
		for j, stateKey := range level {
			group[j] = r.states[stateKey]
		}
		groups[i] = group
	}

	return groups
}

// makeStateKey creates a unique key for a state
func (r *DependencyResolver) makeStateKey(module, id string) string {
	return module + ":" + id
}

// stateExists checks if a state exists
func (r *DependencyResolver) stateExists(key string) bool {
	_, exists := r.states[key]
	return exists
}

// ResolveExecutionOrder resolves the execution order for a state file
func ResolveExecutionOrder(stateFile *StateFile) ([]*StateDeclaration, error) {
	resolver := NewDependencyResolver()

	// Add all states to resolver
	for _, declarations := range stateFile.States {
		for i := range declarations {
			resolver.AddState(&declarations[i])
		}
	}

	// Build dependency graph
	if err := resolver.BuildGraph(); err != nil {
		return nil, err
	}

	// Perform topological sort
	return resolver.TopologicalSort()
}
