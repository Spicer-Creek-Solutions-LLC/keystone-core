// Package deployment provides deployment management including dependency graphs.
package deployment

import (
	"context"
	"fmt"
	"sort"
	"sync"
	"time"
)

// DependencyType represents the type of dependency.
type DependencyType string

const (
	// DependencyTypeHard indicates a hard dependency (must exist and be healthy).
	DependencyTypeHard DependencyType = "hard"
	// DependencyTypeSoft indicates a soft dependency (should exist but not blocking).
	DependencyTypeSoft DependencyType = "soft"
	// DependencyTypeOptional indicates an optional dependency.
	DependencyTypeOptional DependencyType = "optional"
)

// DeploymentState represents the state of a deployment.
type DeploymentState string

const (
	// StateUnknown indicates unknown state.
	StateUnknown DeploymentState = "unknown"
	// StatePending indicates the deployment is pending.
	StatePending DeploymentState = "pending"
	// StateDeploying indicates the deployment is in progress.
	StateDeploying DeploymentState = "deploying"
	// StateHealthy indicates the deployment is healthy.
	StateHealthy DeploymentState = "healthy"
	// StateDegraded indicates the deployment is degraded.
	StateDegraded DeploymentState = "degraded"
	// StateFailed indicates the deployment failed.
	StateFailed DeploymentState = "failed"
)

// Deployment represents a deployment in the dependency graph.
type Deployment struct {
	ID           string            `json:"id"`
	Name         string            `json:"name"`
	Namespace    string            `json:"namespace"`
	Version      string            `json:"version"`
	State        DeploymentState   `json:"state"`
	Dependencies []Dependency      `json:"dependencies,omitempty"`
	Dependents   []string          `json:"dependents,omitempty"`
	Metadata     map[string]string `json:"metadata,omitempty"`
	CreatedAt    time.Time         `json:"createdAt"`
	UpdatedAt    time.Time         `json:"updatedAt"`
	DeployedAt   *time.Time        `json:"deployedAt,omitempty"`
}

// Dependency represents a dependency relationship.
type Dependency struct {
	TargetID    string         `json:"targetId"`
	TargetName  string         `json:"targetName,omitempty"`
	Type        DependencyType `json:"type"`
	MinVersion  string         `json:"minVersion,omitempty"`
	MaxVersion  string         `json:"maxVersion,omitempty"`
	Timeout     time.Duration  `json:"timeout,omitempty"`
	HealthCheck *HealthCheck   `json:"healthCheck,omitempty"`
}

// HealthCheck configures health checking for dependencies.
type HealthCheck struct {
	Type     string        `json:"type"` // http, tcp, exec
	Endpoint string        `json:"endpoint,omitempty"`
	Command  []string      `json:"command,omitempty"`
	Interval time.Duration `json:"interval"`
	Timeout  time.Duration `json:"timeout"`
	Retries  int           `json:"retries"`
}

// Graph represents a deployment dependency graph.
type Graph struct {
	nodes     map[string]*Deployment
	edges     map[string]map[string]*Dependency
	reverseEdges map[string]map[string]bool
	mu        sync.RWMutex
}

// NewGraph creates a new dependency graph.
func NewGraph() *Graph {
	return &Graph{
		nodes:        make(map[string]*Deployment),
		edges:        make(map[string]map[string]*Dependency),
		reverseEdges: make(map[string]map[string]bool),
	}
}

// AddDeployment adds a deployment to the graph.
func (g *Graph) AddDeployment(d *Deployment) error {
	g.mu.Lock()
	defer g.mu.Unlock()

	g.nodes[d.ID] = d

	// Initialize edges
	if g.edges[d.ID] == nil {
		g.edges[d.ID] = make(map[string]*Dependency)
	}
	if g.reverseEdges[d.ID] == nil {
		g.reverseEdges[d.ID] = make(map[string]bool)
	}

	// Add dependency edges
	for i := range d.Dependencies {
		dep := &d.Dependencies[i]
		g.edges[d.ID][dep.TargetID] = dep

		if g.reverseEdges[dep.TargetID] == nil {
			g.reverseEdges[dep.TargetID] = make(map[string]bool)
		}
		g.reverseEdges[dep.TargetID][d.ID] = true
	}

	return nil
}

// RemoveDeployment removes a deployment from the graph.
func (g *Graph) RemoveDeployment(id string) error {
	g.mu.Lock()
	defer g.mu.Unlock()

	d, ok := g.nodes[id]
	if !ok {
		return fmt.Errorf("deployment not found: %s", id)
	}

	// Remove edges to dependencies
	for _, dep := range d.Dependencies {
		delete(g.reverseEdges[dep.TargetID], id)
	}

	// Remove reverse edges from dependents
	for depID := range g.reverseEdges[id] {
		delete(g.edges[depID], id)
	}

	delete(g.nodes, id)
	delete(g.edges, id)
	delete(g.reverseEdges, id)

	return nil
}

// GetDeployment retrieves a deployment by ID.
func (g *Graph) GetDeployment(id string) (*Deployment, bool) {
	g.mu.RLock()
	defer g.mu.RUnlock()

	d, ok := g.nodes[id]
	return d, ok
}

// GetDependencies returns the direct dependencies of a deployment.
func (g *Graph) GetDependencies(id string) []*Deployment {
	g.mu.RLock()
	defer g.mu.RUnlock()

	var deps []*Deployment
	for targetID := range g.edges[id] {
		if d, ok := g.nodes[targetID]; ok {
			deps = append(deps, d)
		}
	}
	return deps
}

// GetDependents returns deployments that depend on this one.
func (g *Graph) GetDependents(id string) []*Deployment {
	g.mu.RLock()
	defer g.mu.RUnlock()

	var deps []*Deployment
	for depID := range g.reverseEdges[id] {
		if d, ok := g.nodes[depID]; ok {
			deps = append(deps, d)
		}
	}
	return deps
}

// GetAllDependencies returns all dependencies recursively.
func (g *Graph) GetAllDependencies(id string) []*Deployment {
	g.mu.RLock()
	defer g.mu.RUnlock()

	visited := make(map[string]bool)
	var result []*Deployment

	var visit func(nodeID string)
	visit = func(nodeID string) {
		for targetID := range g.edges[nodeID] {
			if !visited[targetID] {
				visited[targetID] = true
				if d, ok := g.nodes[targetID]; ok {
					result = append(result, d)
				}
				visit(targetID)
			}
		}
	}

	visit(id)
	return result
}

// GetAllDependents returns all dependents recursively.
func (g *Graph) GetAllDependents(id string) []*Deployment {
	g.mu.RLock()
	defer g.mu.RUnlock()

	visited := make(map[string]bool)
	var result []*Deployment

	var visit func(nodeID string)
	visit = func(nodeID string) {
		for depID := range g.reverseEdges[nodeID] {
			if !visited[depID] {
				visited[depID] = true
				if d, ok := g.nodes[depID]; ok {
					result = append(result, d)
				}
				visit(depID)
			}
		}
	}

	visit(id)
	return result
}

// HasCycle checks if adding a dependency would create a cycle.
func (g *Graph) HasCycle() bool {
	g.mu.RLock()
	defer g.mu.RUnlock()

	visited := make(map[string]bool)
	recStack := make(map[string]bool)

	var hasCycle func(nodeID string) bool
	hasCycle = func(nodeID string) bool {
		visited[nodeID] = true
		recStack[nodeID] = true

		for targetID := range g.edges[nodeID] {
			if !visited[targetID] {
				if hasCycle(targetID) {
					return true
				}
			} else if recStack[targetID] {
				return true
			}
		}

		recStack[nodeID] = false
		return false
	}

	for nodeID := range g.nodes {
		if !visited[nodeID] {
			if hasCycle(nodeID) {
				return true
			}
		}
	}

	return false
}

// TopologicalSort returns deployments in dependency order.
func (g *Graph) TopologicalSort() ([]*Deployment, error) {
	g.mu.RLock()
	defer g.mu.RUnlock()

	if g.hasCycleUnsafe() {
		return nil, fmt.Errorf("graph has a cycle")
	}

	visited := make(map[string]bool)
	var order []*Deployment

	var visit func(nodeID string)
	visit = func(nodeID string) {
		if visited[nodeID] {
			return
		}
		visited[nodeID] = true

		// Visit dependencies first
		for targetID := range g.edges[nodeID] {
			visit(targetID)
		}

		if d, ok := g.nodes[nodeID]; ok {
			order = append(order, d)
		}
	}

	// Sort node IDs for deterministic order
	nodeIDs := make([]string, 0, len(g.nodes))
	for id := range g.nodes {
		nodeIDs = append(nodeIDs, id)
	}
	sort.Strings(nodeIDs)

	for _, id := range nodeIDs {
		visit(id)
	}

	return order, nil
}

func (g *Graph) hasCycleUnsafe() bool {
	visited := make(map[string]bool)
	recStack := make(map[string]bool)

	var hasCycle func(nodeID string) bool
	hasCycle = func(nodeID string) bool {
		visited[nodeID] = true
		recStack[nodeID] = true

		for targetID := range g.edges[nodeID] {
			if !visited[targetID] {
				if hasCycle(targetID) {
					return true
				}
			} else if recStack[targetID] {
				return true
			}
		}

		recStack[nodeID] = false
		return false
	}

	for nodeID := range g.nodes {
		if !visited[nodeID] {
			if hasCycle(nodeID) {
				return true
			}
		}
	}

	return false
}

// GetDeploymentOrder returns the order in which deployments should be deployed.
func (g *Graph) GetDeploymentOrder() ([]*Deployment, error) {
	return g.TopologicalSort()
}

// GetRollbackOrder returns the order in which deployments should be rolled back.
func (g *Graph) GetRollbackOrder() ([]*Deployment, error) {
	order, err := g.TopologicalSort()
	if err != nil {
		return nil, err
	}

	// Reverse the order
	for i, j := 0, len(order)-1; i < j; i, j = i+1, j-1 {
		order[i], order[j] = order[j], order[i]
	}

	return order, nil
}

// CanDeploy checks if a deployment can be deployed based on dependencies.
func (g *Graph) CanDeploy(id string) (bool, []string) {
	g.mu.RLock()
	defer g.mu.RUnlock()

	var blocking []string

	for targetID, dep := range g.edges[id] {
		target, ok := g.nodes[targetID]
		if !ok {
			if dep.Type == DependencyTypeHard {
				blocking = append(blocking, fmt.Sprintf("%s: not found", targetID))
			}
			continue
		}

		if dep.Type == DependencyTypeHard {
			if target.State != StateHealthy && target.State != StateDegraded {
				blocking = append(blocking, fmt.Sprintf("%s: %s", targetID, target.State))
			}
		}
	}

	return len(blocking) == 0, blocking
}

// UpdateState updates the state of a deployment.
func (g *Graph) UpdateState(id string, state DeploymentState) error {
	g.mu.Lock()
	defer g.mu.Unlock()

	d, ok := g.nodes[id]
	if !ok {
		return fmt.Errorf("deployment not found: %s", id)
	}

	d.State = state
	d.UpdatedAt = time.Now()
	return nil
}

// GetReadyDeployments returns deployments that are ready to deploy.
func (g *Graph) GetReadyDeployments() []*Deployment {
	g.mu.RLock()
	defer g.mu.RUnlock()

	var ready []*Deployment

	for id, d := range g.nodes {
		if d.State != StatePending {
			continue
		}

		canDeploy := true
		for targetID, dep := range g.edges[id] {
			target, ok := g.nodes[targetID]
			if !ok {
				if dep.Type == DependencyTypeHard {
					canDeploy = false
					break
				}
				continue
			}

			if dep.Type == DependencyTypeHard {
				if target.State != StateHealthy && target.State != StateDegraded {
					canDeploy = false
					break
				}
			}
		}

		if canDeploy {
			ready = append(ready, d)
		}
	}

	return ready
}

// GetImpactedDeployments returns deployments that would be impacted if a deployment fails.
func (g *Graph) GetImpactedDeployments(id string) []*Deployment {
	return g.GetAllDependents(id)
}

// GetCriticalPath returns the longest dependency chain.
func (g *Graph) GetCriticalPath() []*Deployment {
	g.mu.RLock()
	defer g.mu.RUnlock()

	var longestPath []*Deployment
	visited := make(map[string]bool)

	var dfs func(nodeID string, path []*Deployment)
	dfs = func(nodeID string, path []*Deployment) {
		if visited[nodeID] {
			return
		}
		visited[nodeID] = true

		if d, ok := g.nodes[nodeID]; ok {
			path = append(path, d)
		}

		// If no outgoing edges, check if this is the longest path
		if len(g.edges[nodeID]) == 0 {
			if len(path) > len(longestPath) {
				longestPath = make([]*Deployment, len(path))
				copy(longestPath, path)
			}
		} else {
			for targetID := range g.edges[nodeID] {
				dfs(targetID, path)
			}
		}

		visited[nodeID] = false
	}

	for nodeID := range g.nodes {
		dfs(nodeID, nil)
	}

	return longestPath
}

// Stats returns statistics about the graph.
type GraphStats struct {
	TotalDeployments int            `json:"totalDeployments"`
	TotalDependencies int           `json:"totalDependencies"`
	ByState          map[DeploymentState]int `json:"byState"`
	AverageDependencies float64     `json:"averageDependencies"`
	MaxDependencies  int            `json:"maxDependencies"`
	RootNodes        int            `json:"rootNodes"` // Nodes with no dependencies
	LeafNodes        int            `json:"leafNodes"` // Nodes with no dependents
	CriticalPathLen  int            `json:"criticalPathLength"`
}

// GetStats returns graph statistics.
func (g *Graph) GetStats() *GraphStats {
	g.mu.RLock()
	defer g.mu.RUnlock()

	stats := &GraphStats{
		TotalDeployments: len(g.nodes),
		ByState:          make(map[DeploymentState]int),
	}

	totalDeps := 0
	maxDeps := 0

	for id, d := range g.nodes {
		stats.ByState[d.State]++

		depCount := len(g.edges[id])
		totalDeps += depCount
		if depCount > maxDeps {
			maxDeps = depCount
		}

		// Root node: no dependencies
		if depCount == 0 {
			stats.RootNodes++
		}

		// Leaf node: no dependents
		if len(g.reverseEdges[id]) == 0 {
			stats.LeafNodes++
		}
	}

	stats.TotalDependencies = totalDeps
	stats.MaxDependencies = maxDeps
	if len(g.nodes) > 0 {
		stats.AverageDependencies = float64(totalDeps) / float64(len(g.nodes))
	}

	critPath := g.getCriticalPathUnsafe()
	stats.CriticalPathLen = len(critPath)

	return stats
}

func (g *Graph) getCriticalPathUnsafe() []*Deployment {
	var longestPath []*Deployment
	visited := make(map[string]bool)

	var dfs func(nodeID string, path []*Deployment)
	dfs = func(nodeID string, path []*Deployment) {
		if visited[nodeID] {
			return
		}
		visited[nodeID] = true

		if d, ok := g.nodes[nodeID]; ok {
			path = append(path, d)
		}

		if len(g.edges[nodeID]) == 0 {
			if len(path) > len(longestPath) {
				longestPath = make([]*Deployment, len(path))
				copy(longestPath, path)
			}
		} else {
			for targetID := range g.edges[nodeID] {
				dfs(targetID, path)
			}
		}

		visited[nodeID] = false
	}

	for nodeID := range g.nodes {
		dfs(nodeID, nil)
	}

	return longestPath
}

// Orchestrator manages deployment orchestration based on the dependency graph.
type Orchestrator struct {
	graph     *Graph
	deployer  Deployer
	listeners []OrchestratorListener
	mu        sync.RWMutex
}

// Deployer is the interface for deploying deployments.
type Deployer interface {
	Deploy(ctx context.Context, deployment *Deployment) error
	Rollback(ctx context.Context, deployment *Deployment) error
	CheckHealth(ctx context.Context, deployment *Deployment) (DeploymentState, error)
}

// OrchestratorListener is called when orchestration events occur.
type OrchestratorListener func(event *OrchestratorEvent)

// OrchestratorEvent represents an orchestration event.
type OrchestratorEvent struct {
	Type         string    `json:"type"`
	DeploymentID string    `json:"deploymentId,omitempty"`
	Timestamp    time.Time `json:"timestamp"`
	Message      string    `json:"message"`
	Error        string    `json:"error,omitempty"`
}

// NewOrchestrator creates a new orchestrator.
func NewOrchestrator(graph *Graph, deployer Deployer) *Orchestrator {
	return &Orchestrator{
		graph:    graph,
		deployer: deployer,
	}
}

// AddListener adds an orchestration event listener.
func (o *Orchestrator) AddListener(listener OrchestratorListener) {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.listeners = append(o.listeners, listener)
}

// emit sends an event to all listeners.
func (o *Orchestrator) emit(event *OrchestratorEvent) {
	o.mu.RLock()
	listeners := make([]OrchestratorListener, len(o.listeners))
	copy(listeners, o.listeners)
	o.mu.RUnlock()

	for _, listener := range listeners {
		listener(event)
	}
}

// DeployAll deploys all deployments in dependency order.
func (o *Orchestrator) DeployAll(ctx context.Context) error {
	order, err := o.graph.GetDeploymentOrder()
	if err != nil {
		return err
	}

	o.emit(&OrchestratorEvent{
		Type:      "deploy_started",
		Timestamp: time.Now(),
		Message:   fmt.Sprintf("Starting deployment of %d services", len(order)),
	})

	for _, d := range order {
		if d.State == StateHealthy {
			continue
		}

		// Check if can deploy
		canDeploy, blocking := o.graph.CanDeploy(d.ID)
		if !canDeploy {
			o.emit(&OrchestratorEvent{
				Type:         "deploy_blocked",
				DeploymentID: d.ID,
				Timestamp:    time.Now(),
				Message:      fmt.Sprintf("Deployment blocked by: %v", blocking),
			})
			continue
		}

		o.emit(&OrchestratorEvent{
			Type:         "deploying",
			DeploymentID: d.ID,
			Timestamp:    time.Now(),
			Message:      fmt.Sprintf("Deploying %s", d.Name),
		})

		o.graph.UpdateState(d.ID, StateDeploying)

		if err := o.deployer.Deploy(ctx, d); err != nil {
			o.graph.UpdateState(d.ID, StateFailed)
			o.emit(&OrchestratorEvent{
				Type:         "deploy_failed",
				DeploymentID: d.ID,
				Timestamp:    time.Now(),
				Message:      fmt.Sprintf("Deployment failed: %s", d.Name),
				Error:        err.Error(),
			})
			return err
		}

		o.graph.UpdateState(d.ID, StateHealthy)
		o.emit(&OrchestratorEvent{
			Type:         "deployed",
			DeploymentID: d.ID,
			Timestamp:    time.Now(),
			Message:      fmt.Sprintf("Deployed %s", d.Name),
		})
	}

	o.emit(&OrchestratorEvent{
		Type:      "deploy_completed",
		Timestamp: time.Now(),
		Message:   "All deployments completed",
	})

	return nil
}

// RollbackAll rolls back all deployments in reverse dependency order.
func (o *Orchestrator) RollbackAll(ctx context.Context) error {
	order, err := o.graph.GetRollbackOrder()
	if err != nil {
		return err
	}

	o.emit(&OrchestratorEvent{
		Type:      "rollback_started",
		Timestamp: time.Now(),
		Message:   fmt.Sprintf("Starting rollback of %d services", len(order)),
	})

	for _, d := range order {
		o.emit(&OrchestratorEvent{
			Type:         "rolling_back",
			DeploymentID: d.ID,
			Timestamp:    time.Now(),
			Message:      fmt.Sprintf("Rolling back %s", d.Name),
		})

		if err := o.deployer.Rollback(ctx, d); err != nil {
			o.emit(&OrchestratorEvent{
				Type:         "rollback_failed",
				DeploymentID: d.ID,
				Timestamp:    time.Now(),
				Message:      fmt.Sprintf("Rollback failed: %s", d.Name),
				Error:        err.Error(),
			})
			// Continue with other rollbacks
		}

		o.graph.UpdateState(d.ID, StatePending)
		o.emit(&OrchestratorEvent{
			Type:         "rolled_back",
			DeploymentID: d.ID,
			Timestamp:    time.Now(),
			Message:      fmt.Sprintf("Rolled back %s", d.Name),
		})
	}

	o.emit(&OrchestratorEvent{
		Type:      "rollback_completed",
		Timestamp: time.Now(),
		Message:   "All rollbacks completed",
	})

	return nil
}

// DeployParallel deploys independent deployments in parallel.
func (o *Orchestrator) DeployParallel(ctx context.Context, maxConcurrency int) error {
	if maxConcurrency <= 0 {
		maxConcurrency = 5
	}

	sem := make(chan struct{}, maxConcurrency)
	var wg sync.WaitGroup
	errCh := make(chan error, 1)

	o.emit(&OrchestratorEvent{
		Type:      "parallel_deploy_started",
		Timestamp: time.Now(),
		Message:   fmt.Sprintf("Starting parallel deployment with concurrency %d", maxConcurrency),
	})

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		ready := o.graph.GetReadyDeployments()
		if len(ready) == 0 {
			break
		}

		for _, d := range ready {
			wg.Add(1)
			sem <- struct{}{}

			go func(dep *Deployment) {
				defer wg.Done()
				defer func() { <-sem }()

				o.graph.UpdateState(dep.ID, StateDeploying)

				if err := o.deployer.Deploy(ctx, dep); err != nil {
					o.graph.UpdateState(dep.ID, StateFailed)
					select {
					case errCh <- err:
					default:
					}
					return
				}

				o.graph.UpdateState(dep.ID, StateHealthy)
			}(d)
		}

		wg.Wait()

		select {
		case err := <-errCh:
			return err
		default:
		}
	}

	o.emit(&OrchestratorEvent{
		Type:      "parallel_deploy_completed",
		Timestamp: time.Now(),
		Message:   "Parallel deployment completed",
	})

	return nil
}

// GraphVisualization represents a visualization of the graph.
type GraphVisualization struct {
	Nodes []NodeViz `json:"nodes"`
	Edges []EdgeViz `json:"edges"`
}

// NodeViz represents a node in the visualization.
type NodeViz struct {
	ID       string          `json:"id"`
	Label    string          `json:"label"`
	State    DeploymentState `json:"state"`
	Metadata map[string]string `json:"metadata,omitempty"`
}

// EdgeViz represents an edge in the visualization.
type EdgeViz struct {
	Source string         `json:"source"`
	Target string         `json:"target"`
	Type   DependencyType `json:"type"`
}

// GetVisualization returns a visualization of the graph.
func (g *Graph) GetVisualization() *GraphVisualization {
	g.mu.RLock()
	defer g.mu.RUnlock()

	viz := &GraphVisualization{}

	for id, d := range g.nodes {
		viz.Nodes = append(viz.Nodes, NodeViz{
			ID:       id,
			Label:    d.Name,
			State:    d.State,
			Metadata: d.Metadata,
		})
	}

	for sourceID, targets := range g.edges {
		for targetID, dep := range targets {
			viz.Edges = append(viz.Edges, EdgeViz{
				Source: sourceID,
				Target: targetID,
				Type:   dep.Type,
			})
		}
	}

	return viz
}

// ToDOT exports the graph in DOT format for Graphviz.
func (g *Graph) ToDOT() string {
	g.mu.RLock()
	defer g.mu.RUnlock()

	var result string
	result += "digraph deployments {\n"
	result += "  rankdir=LR;\n"
	result += "  node [shape=box];\n\n"

	// Define nodes with colors based on state
	for id, d := range g.nodes {
		color := "white"
		switch d.State {
		case StateHealthy:
			color = "lightgreen"
		case StateDeploying:
			color = "lightyellow"
		case StateFailed:
			color = "lightcoral"
		case StateDegraded:
			color = "lightorange"
		}
		result += fmt.Sprintf("  \"%s\" [label=\"%s\\n%s\" style=filled fillcolor=%s];\n",
			id, d.Name, d.State, color)
	}

	result += "\n"

	// Define edges
	for sourceID, targets := range g.edges {
		for targetID, dep := range targets {
			style := "solid"
			if dep.Type == DependencyTypeSoft {
				style = "dashed"
			} else if dep.Type == DependencyTypeOptional {
				style = "dotted"
			}
			result += fmt.Sprintf("  \"%s\" -> \"%s\" [style=%s];\n", sourceID, targetID, style)
		}
	}

	result += "}\n"
	return result
}
