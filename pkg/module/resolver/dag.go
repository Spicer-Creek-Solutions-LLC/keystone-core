package resolver

import (
	"fmt"
	"sort"
	"sync"
)

// DefaultDependencyGraph implements DependencyGraph
type DefaultDependencyGraph struct {
	nodes        map[string]*DependencyNode
	edges        map[string][]string // adjacency list for topological sort: dep -> []dependents
	dependencies map[string][]string // reverse edges: module -> []dependencies
	mu           sync.RWMutex
}

// NewDependencyGraph creates a new dependency graph
func NewDependencyGraph() *DefaultDependencyGraph {
	return &DefaultDependencyGraph{
		nodes:        make(map[string]*DependencyNode),
		edges:        make(map[string][]string),
		dependencies: make(map[string][]string),
	}
}

// AddNode adds a node to the graph
func (g *DefaultDependencyGraph) AddNode(node *DependencyNode) error {
	if node == nil {
		return fmt.Errorf("node cannot be nil")
	}

	if node.Module.Name == "" {
		return fmt.Errorf("module name cannot be empty")
	}

	g.mu.Lock()
	defer g.mu.Unlock()

	// Add or update the node
	g.nodes[node.Module.Name] = node

	// Initialize edges if not present
	if _, exists := g.edges[node.Module.Name]; !exists {
		g.edges[node.Module.Name] = make([]string, 0)
	}
	if _, exists := g.dependencies[node.Module.Name]; !exists {
		g.dependencies[node.Module.Name] = make([]string, 0)
	}

	// Add edges for dependencies
	// edges: dep -> node (for topological sort)
	// dependencies: node -> dep (for GetDependencies)
	for _, dep := range node.Dependencies {
		if dep != nil && dep.Module.Name != "" {
			// Initialize edges for dependency if not present
			if _, exists := g.edges[dep.Module.Name]; !exists {
				g.edges[dep.Module.Name] = make([]string, 0)
			}
			// Add edge from dependency to this node (dep -> node)
			if !contains(g.edges[dep.Module.Name], node.Module.Name) {
				g.edges[dep.Module.Name] = append(g.edges[dep.Module.Name], node.Module.Name)
			}
			// Add to dependencies map (node -> dep)
			if !contains(g.dependencies[node.Module.Name], dep.Module.Name) {
				g.dependencies[node.Module.Name] = append(g.dependencies[node.Module.Name], dep.Module.Name)
			}
		}
	}

	return nil
}

// GetNode returns a node by module name
func (g *DefaultDependencyGraph) GetNode(moduleName string) (*DependencyNode, error) {
	g.mu.RLock()
	defer g.mu.RUnlock()

	node, exists := g.nodes[moduleName]
	if !exists {
		return nil, fmt.Errorf("%w: %s", ErrModuleNotFound, moduleName)
	}

	return node, nil
}

// GetAllNodes returns all nodes in the graph
func (g *DefaultDependencyGraph) GetAllNodes() []*DependencyNode {
	g.mu.RLock()
	defer g.mu.RUnlock()

	nodes := make([]*DependencyNode, 0, len(g.nodes))
	for _, node := range g.nodes {
		nodes = append(nodes, node)
	}

	// Sort by module name for consistent ordering
	sort.Slice(nodes, func(i, j int) bool {
		return nodes[i].Module.Name < nodes[j].Module.Name
	})

	return nodes
}

// HasCycle detects if the graph has a cycle using DFS
// Returns true if a cycle exists, along with the cycle path
func (g *DefaultDependencyGraph) HasCycle() (bool, []string) {
	g.mu.RLock()
	defer g.mu.RUnlock()

	visited := make(map[string]bool)
	recStack := make(map[string]bool)
	parent := make(map[string]string)

	for moduleName := range g.nodes {
		if !visited[moduleName] {
			hasCycle, cyclePath := g.dfs(moduleName, visited, recStack, parent)
			if hasCycle {
				return hasCycle, cyclePath
			}
		}
	}

	return false, nil
}

// dfs performs depth-first search to detect cycles
func (g *DefaultDependencyGraph) dfs(moduleName string, visited, recStack map[string]bool, parent map[string]string) (bool, []string) {
	visited[moduleName] = true
	recStack[moduleName] = true

	// Check all dependencies
	for _, dep := range g.edges[moduleName] {
		if !visited[dep] {
			parent[dep] = moduleName
			hasCycle, cyclePath := g.dfs(dep, visited, recStack, parent)
			if hasCycle {
				return hasCycle, cyclePath
			}
		} else if recStack[dep] {
			// Found a cycle - reconstruct the path
			cycle := []string{dep}
			current := moduleName
			for current != dep {
				cycle = append([]string{current}, cycle...)
				current = parent[current]
			}
			cycle = append(cycle, dep) // Complete the cycle
			return true, cycle
		}
	}

	recStack[moduleName] = false
	return false, nil
}

// TopologicalSort returns a topologically sorted list of modules
// Uses Kahn's algorithm
func (g *DefaultDependencyGraph) TopologicalSort() ([]*DependencyNode, error) {
	g.mu.RLock()
	defer g.mu.RUnlock()

	// First check for cycles
	if hasCycle, cyclePath := g.HasCycle(); hasCycle {
		return nil, &CircularDependencyError{Cycle: cyclePath}
	}

	// Calculate in-degree for each node
	inDegree := make(map[string]int)
	for moduleName := range g.nodes {
		inDegree[moduleName] = 0
	}

	for _, deps := range g.edges {
		for _, dep := range deps {
			inDegree[dep]++
		}
	}

	// Find all nodes with in-degree 0
	queue := make([]string, 0)
	for moduleName, degree := range inDegree {
		if degree == 0 {
			queue = append(queue, moduleName)
		}
	}

	// Sort queue for deterministic ordering
	sort.Strings(queue)

	// Process nodes in topological order
	sorted := make([]*DependencyNode, 0, len(g.nodes))

	for len(queue) > 0 {
		// Pop from queue
		moduleName := queue[0]
		queue = queue[1:]

		// Add to sorted list
		if node, exists := g.nodes[moduleName]; exists {
			sorted = append(sorted, node)
		}

		// Reduce in-degree for dependencies
		for _, dep := range g.edges[moduleName] {
			inDegree[dep]--
			if inDegree[dep] == 0 {
				queue = append(queue, dep)
				sort.Strings(queue) // Keep queue sorted for determinism
			}
		}
	}

	// If we didn't process all nodes, there's a cycle (shouldn't happen due to earlier check)
	if len(sorted) != len(g.nodes) {
		return nil, ErrCircularDependency
	}

	return sorted, nil
}

// Flatten returns a flattened list of all dependencies
func (g *DefaultDependencyGraph) Flatten() []ModuleReference {
	g.mu.RLock()
	defer g.mu.RUnlock()

	// Use topological sort to get correct order, but handle errors gracefully
	sorted, err := g.TopologicalSort()
	if err != nil {
		// If topological sort fails (cycle), just return all modules
		modules := make([]ModuleReference, 0, len(g.nodes))
		for _, node := range g.nodes {
			modules = append(modules, node.Module)
		}
		// Sort by name for consistent ordering
		sort.Slice(modules, func(i, j int) bool {
			return modules[i].Name < modules[j].Name
		})
		return modules
	}

	modules := make([]ModuleReference, 0, len(sorted))
	for _, node := range sorted {
		modules = append(modules, node.Module)
	}

	return modules
}

// GetDependencies returns the direct dependencies of a module
func (g *DefaultDependencyGraph) GetDependencies(moduleName string) ([]string, error) {
	g.mu.RLock()
	defer g.mu.RUnlock()

	if _, exists := g.nodes[moduleName]; !exists {
		return nil, fmt.Errorf("%w: %s", ErrModuleNotFound, moduleName)
	}

	deps := g.dependencies[moduleName]
	result := make([]string, len(deps))
	copy(result, deps)

	return result, nil
}

// GetDependents returns all modules that depend on the given module
func (g *DefaultDependencyGraph) GetDependents(moduleName string) ([]string, error) {
	g.mu.RLock()
	defer g.mu.RUnlock()

	if _, exists := g.nodes[moduleName]; !exists {
		return nil, fmt.Errorf("%w: %s", ErrModuleNotFound, moduleName)
	}

	// edges map already contains dep -> dependents
	dependents := g.edges[moduleName]
	result := make([]string, len(dependents))
	copy(result, dependents)
	sort.Strings(result)

	return result, nil
}

// Size returns the number of nodes in the graph
func (g *DefaultDependencyGraph) Size() int {
	g.mu.RLock()
	defer g.mu.RUnlock()

	return len(g.nodes)
}

// Clear removes all nodes and edges from the graph
func (g *DefaultDependencyGraph) Clear() {
	g.mu.Lock()
	defer g.mu.Unlock()

	g.nodes = make(map[string]*DependencyNode)
	g.edges = make(map[string][]string)
	g.dependencies = make(map[string][]string)
}

// contains checks if a slice contains a string
func contains(slice []string, item string) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
}
