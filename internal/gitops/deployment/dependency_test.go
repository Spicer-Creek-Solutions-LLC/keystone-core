package deployment

import (
	"context"
	"strings"
	"testing"
	"time"
)

func TestDependencyType(t *testing.T) {
	types := []DependencyType{
		DependencyTypeHard,
		DependencyTypeSoft,
		DependencyTypeOptional,
	}

	for _, dt := range types {
		if dt == "" {
			t.Error("Dependency type should not be empty")
		}
	}
}

func TestDeploymentState(t *testing.T) {
	states := []State{
		StateUnknown,
		StatePending,
		StateDeploying,
		StateHealthy,
		StateDegraded,
		StateFailed,
	}

	for _, s := range states {
		if s == "" {
			t.Error("State should not be empty")
		}
	}
}

func TestNewGraph(t *testing.T) {
	g := NewGraph()

	if g == nil {
		t.Fatal("Expected non-nil graph")
	}
	if g.nodes == nil {
		t.Error("nodes should be initialized")
	}
	if g.edges == nil {
		t.Error("edges should be initialized")
	}
}

func TestGraph_AddDeployment(t *testing.T) {
	g := NewGraph()

	d := &Deployment{
		ID:        "svc-1",
		Name:      "service-1",
		Namespace: "default",
		State:     StatePending,
		Dependencies: []Dependency{
			{TargetID: "svc-2", Type: DependencyTypeHard},
		},
	}

	err := g.AddDeployment(d)
	if err != nil {
		t.Fatalf("AddDeployment failed: %v", err)
	}

	retrieved, ok := g.GetDeployment("svc-1")
	if !ok {
		t.Error("Expected to find deployment")
	}
	if retrieved.Name != "service-1" {
		t.Errorf("Name = %s, want service-1", retrieved.Name)
	}
}

func TestGraph_RemoveDeployment(t *testing.T) {
	g := NewGraph()

	g.AddDeployment(&Deployment{ID: "svc-1", Name: "service-1"})
	g.AddDeployment(&Deployment{ID: "svc-2", Name: "service-2"})

	err := g.RemoveDeployment("svc-1")
	if err != nil {
		t.Fatalf("RemoveDeployment failed: %v", err)
	}

	_, ok := g.GetDeployment("svc-1")
	if ok {
		t.Error("Expected deployment to be removed")
	}

	// Check svc-2 still exists
	_, ok = g.GetDeployment("svc-2")
	if !ok {
		t.Error("svc-2 should still exist")
	}
}

func TestGraph_GetDependencies(t *testing.T) {
	g := NewGraph()

	g.AddDeployment(&Deployment{ID: "db", Name: "database", State: StateHealthy})
	g.AddDeployment(&Deployment{ID: "cache", Name: "cache", State: StateHealthy})
	g.AddDeployment(&Deployment{
		ID:   "api",
		Name: "api",
		Dependencies: []Dependency{
			{TargetID: "db", Type: DependencyTypeHard},
			{TargetID: "cache", Type: DependencyTypeSoft},
		},
	})

	deps := g.GetDependencies("api")
	if len(deps) != 2 {
		t.Errorf("Dependencies = %d, want 2", len(deps))
	}
}

func TestGraph_GetDependents(t *testing.T) {
	g := NewGraph()

	g.AddDeployment(&Deployment{ID: "db", Name: "database"})
	g.AddDeployment(&Deployment{
		ID:   "api",
		Name: "api",
		Dependencies: []Dependency{
			{TargetID: "db", Type: DependencyTypeHard},
		},
	})
	g.AddDeployment(&Deployment{
		ID:   "worker",
		Name: "worker",
		Dependencies: []Dependency{
			{TargetID: "db", Type: DependencyTypeHard},
		},
	})

	dependents := g.GetDependents("db")
	if len(dependents) != 2 {
		t.Errorf("Dependents = %d, want 2", len(dependents))
	}
}

func TestGraph_GetAllDependencies(t *testing.T) {
	g := NewGraph()

	g.AddDeployment(&Deployment{ID: "db", Name: "database"})
	g.AddDeployment(&Deployment{
		ID:   "cache",
		Name: "cache",
		Dependencies: []Dependency{
			{TargetID: "db", Type: DependencyTypeHard},
		},
	})
	g.AddDeployment(&Deployment{
		ID:   "api",
		Name: "api",
		Dependencies: []Dependency{
			{TargetID: "cache", Type: DependencyTypeHard},
		},
	})

	allDeps := g.GetAllDependencies("api")
	if len(allDeps) != 2 {
		t.Errorf("All dependencies = %d, want 2", len(allDeps))
	}
}

func TestGraph_HasCycle_NoCycle(t *testing.T) {
	g := NewGraph()

	g.AddDeployment(&Deployment{ID: "a", Name: "a"})
	g.AddDeployment(&Deployment{
		ID:           "b",
		Name:         "b",
		Dependencies: []Dependency{{TargetID: "a", Type: DependencyTypeHard}},
	})
	g.AddDeployment(&Deployment{
		ID:           "c",
		Name:         "c",
		Dependencies: []Dependency{{TargetID: "b", Type: DependencyTypeHard}},
	})

	if g.HasCycle() {
		t.Error("Expected no cycle")
	}
}

func TestGraph_HasCycle_WithCycle(t *testing.T) {
	g := NewGraph()

	g.AddDeployment(&Deployment{
		ID:           "a",
		Name:         "a",
		Dependencies: []Dependency{{TargetID: "c", Type: DependencyTypeHard}},
	})
	g.AddDeployment(&Deployment{
		ID:           "b",
		Name:         "b",
		Dependencies: []Dependency{{TargetID: "a", Type: DependencyTypeHard}},
	})
	g.AddDeployment(&Deployment{
		ID:           "c",
		Name:         "c",
		Dependencies: []Dependency{{TargetID: "b", Type: DependencyTypeHard}},
	})

	if !g.HasCycle() {
		t.Error("Expected cycle to be detected")
	}
}

func TestGraph_TopologicalSort(t *testing.T) {
	g := NewGraph()

	g.AddDeployment(&Deployment{ID: "db", Name: "database"})
	g.AddDeployment(&Deployment{ID: "cache", Name: "cache"})
	g.AddDeployment(&Deployment{
		ID:   "api",
		Name: "api",
		Dependencies: []Dependency{
			{TargetID: "db", Type: DependencyTypeHard},
			{TargetID: "cache", Type: DependencyTypeHard},
		},
	})
	g.AddDeployment(&Deployment{
		ID:   "frontend",
		Name: "frontend",
		Dependencies: []Dependency{
			{TargetID: "api", Type: DependencyTypeHard},
		},
	})

	order, err := g.TopologicalSort()
	if err != nil {
		t.Fatalf("TopologicalSort failed: %v", err)
	}

	if len(order) != 4 {
		t.Errorf("Order length = %d, want 4", len(order))
	}

	// Check that dependencies come before dependents
	positions := make(map[string]int)
	for i, d := range order {
		positions[d.ID] = i
	}

	if positions["db"] > positions["api"] {
		t.Error("db should come before api")
	}
	if positions["cache"] > positions["api"] {
		t.Error("cache should come before api")
	}
	if positions["api"] > positions["frontend"] {
		t.Error("api should come before frontend")
	}
}

func TestGraph_TopologicalSort_Cycle(t *testing.T) {
	g := NewGraph()

	g.AddDeployment(&Deployment{
		ID:           "a",
		Dependencies: []Dependency{{TargetID: "b", Type: DependencyTypeHard}},
	})
	g.AddDeployment(&Deployment{
		ID:           "b",
		Dependencies: []Dependency{{TargetID: "a", Type: DependencyTypeHard}},
	})

	_, err := g.TopologicalSort()
	if err == nil {
		t.Error("Expected error for cyclic graph")
	}
}

func TestGraph_CanDeploy(t *testing.T) {
	g := NewGraph()

	g.AddDeployment(&Deployment{ID: "db", Name: "database", State: StateHealthy})
	g.AddDeployment(&Deployment{ID: "cache", Name: "cache", State: StateFailed})
	g.AddDeployment(&Deployment{
		ID:   "api",
		Name: "api",
		Dependencies: []Dependency{
			{TargetID: "db", Type: DependencyTypeHard},
			{TargetID: "cache", Type: DependencyTypeSoft},
		},
	})

	canDeploy, blocking := g.CanDeploy("api")
	if !canDeploy {
		t.Errorf("Should be able to deploy, blocking: %v", blocking)
	}

	// Now make db fail
	g.UpdateState("db", StateFailed)
	canDeploy, blocking = g.CanDeploy("api")
	if canDeploy {
		t.Error("Should not be able to deploy with failed hard dependency")
	}
	if len(blocking) != 1 {
		t.Errorf("Blocking = %d, want 1", len(blocking))
	}
}

func TestGraph_GetReadyDeployments(t *testing.T) {
	g := NewGraph()

	g.AddDeployment(&Deployment{ID: "db", Name: "database", State: StateHealthy})
	g.AddDeployment(&Deployment{ID: "cache", Name: "cache", State: StateHealthy})
	g.AddDeployment(&Deployment{
		ID:    "api",
		Name:  "api",
		State: StatePending,
		Dependencies: []Dependency{
			{TargetID: "db", Type: DependencyTypeHard},
		},
	})
	g.AddDeployment(&Deployment{
		ID:    "worker",
		Name:  "worker",
		State: StatePending,
		Dependencies: []Dependency{
			{TargetID: "api", Type: DependencyTypeHard}, // api is pending, so worker isn't ready
		},
	})

	ready := g.GetReadyDeployments()
	if len(ready) != 1 {
		t.Errorf("Ready = %d, want 1", len(ready))
	}
	if ready[0].ID != "api" {
		t.Errorf("Ready[0] = %s, want api", ready[0].ID)
	}
}

func TestGraph_GetStats(t *testing.T) {
	g := NewGraph()

	g.AddDeployment(&Deployment{ID: "db", Name: "database", State: StateHealthy})
	g.AddDeployment(&Deployment{ID: "cache", Name: "cache", State: StateHealthy})
	g.AddDeployment(&Deployment{
		ID:    "api",
		Name:  "api",
		State: StatePending,
		Dependencies: []Dependency{
			{TargetID: "db", Type: DependencyTypeHard},
			{TargetID: "cache", Type: DependencyTypeHard},
		},
	})

	stats := g.GetStats()

	if stats.TotalDeployments != 3 {
		t.Errorf("TotalDeployments = %d, want 3", stats.TotalDeployments)
	}
	if stats.TotalDependencies != 2 {
		t.Errorf("TotalDependencies = %d, want 2", stats.TotalDependencies)
	}
	if stats.RootNodes != 2 { // db and cache have no dependencies
		t.Errorf("RootNodes = %d, want 2", stats.RootNodes)
	}
	if stats.LeafNodes != 1 { // api has no dependents
		t.Errorf("LeafNodes = %d, want 1", stats.LeafNodes)
	}
}

func TestGraph_GetVisualization(t *testing.T) {
	g := NewGraph()

	g.AddDeployment(&Deployment{ID: "a", Name: "service-a", State: StateHealthy})
	g.AddDeployment(&Deployment{
		ID:           "b",
		Name:         "service-b",
		State:        StatePending,
		Dependencies: []Dependency{{TargetID: "a", Type: DependencyTypeHard}},
	})

	viz := g.GetVisualization()

	if len(viz.Nodes) != 2 {
		t.Errorf("Nodes = %d, want 2", len(viz.Nodes))
	}
	if len(viz.Edges) != 1 {
		t.Errorf("Edges = %d, want 1", len(viz.Edges))
	}
}

func TestGraph_ToDOT(t *testing.T) {
	g := NewGraph()

	g.AddDeployment(&Deployment{ID: "db", Name: "database", State: StateHealthy})
	g.AddDeployment(&Deployment{
		ID:           "api",
		Name:         "api",
		State:        StatePending,
		Dependencies: []Dependency{{TargetID: "db", Type: DependencyTypeHard}},
	})

	dot := g.ToDOT()

	if !strings.Contains(dot, "digraph deployments") {
		t.Error("DOT should contain digraph declaration")
	}
	if !strings.Contains(dot, "database") {
		t.Error("DOT should contain node labels")
	}
	if !strings.Contains(dot, "->") {
		t.Error("DOT should contain edges")
	}
}

type mockDeployer struct {
	deployed   map[string]bool
	rolledBack map[string]bool
	failOn     string
}

func newMockDeployer() *mockDeployer {
	return &mockDeployer{
		deployed:   make(map[string]bool),
		rolledBack: make(map[string]bool),
	}
}

func (m *mockDeployer) Deploy(ctx context.Context, d *Deployment) error {
	if m.failOn == d.ID {
		return context.DeadlineExceeded
	}
	m.deployed[d.ID] = true
	return nil
}

func (m *mockDeployer) Rollback(ctx context.Context, d *Deployment) error {
	m.rolledBack[d.ID] = true
	return nil
}

func (m *mockDeployer) CheckHealth(ctx context.Context, d *Deployment) (State, error) {
	if m.deployed[d.ID] {
		return StateHealthy, nil
	}
	return StatePending, nil
}

func TestNewOrchestrator(t *testing.T) {
	g := NewGraph()
	deployer := newMockDeployer()

	o := NewOrchestrator(g, deployer)
	if o == nil {
		t.Fatal("Expected non-nil orchestrator")
	}
}

func TestOrchestrator_DeployAll(t *testing.T) {
	g := NewGraph()
	deployer := newMockDeployer()
	o := NewOrchestrator(g, deployer)

	g.AddDeployment(&Deployment{ID: "db", Name: "database", State: StatePending})
	g.AddDeployment(&Deployment{
		ID:           "api",
		Name:         "api",
		State:        StatePending,
		Dependencies: []Dependency{{TargetID: "db", Type: DependencyTypeHard}},
	})

	var events []*OrchestratorEvent
	o.AddListener(func(e *OrchestratorEvent) {
		events = append(events, e)
	})

	ctx := context.Background()
	if err := o.DeployAll(ctx); err != nil {
		t.Fatalf("DeployAll failed: %v", err)
	}

	if !deployer.deployed["db"] {
		t.Error("db should be deployed")
	}
	if !deployer.deployed["api"] {
		t.Error("api should be deployed")
	}

	// Check events
	if len(events) < 2 {
		t.Errorf("Events = %d, want at least 2", len(events))
	}
}

func TestOrchestrator_RollbackAll(t *testing.T) {
	g := NewGraph()
	deployer := newMockDeployer()
	o := NewOrchestrator(g, deployer)

	g.AddDeployment(&Deployment{ID: "db", Name: "database", State: StateHealthy})
	g.AddDeployment(&Deployment{
		ID:           "api",
		Name:         "api",
		State:        StateHealthy,
		Dependencies: []Dependency{{TargetID: "db", Type: DependencyTypeHard}},
	})

	ctx := context.Background()
	if err := o.RollbackAll(ctx); err != nil {
		t.Fatalf("RollbackAll failed: %v", err)
	}

	if !deployer.rolledBack["db"] {
		t.Error("db should be rolled back")
	}
	if !deployer.rolledBack["api"] {
		t.Error("api should be rolled back")
	}
}

func TestDeployment(t *testing.T) {
	now := time.Now()
	d := &Deployment{
		ID:        "svc-1",
		Name:      "my-service",
		Namespace: "production",
		Version:   "v1.2.3",
		State:     StateHealthy,
		Dependencies: []Dependency{
			{
				TargetID:   "db",
				TargetName: "database",
				Type:       DependencyTypeHard,
				MinVersion: "v1.0.0",
			},
		},
		Metadata: map[string]string{
			"team": "platform",
		},
		CreatedAt:  now,
		UpdatedAt:  now,
		DeployedAt: &now,
	}

	if d.Version != "v1.2.3" {
		t.Errorf("Version = %s, want v1.2.3", d.Version)
	}
	if len(d.Dependencies) != 1 {
		t.Errorf("Dependencies = %d, want 1", len(d.Dependencies))
	}
}

func TestDependency(t *testing.T) {
	dep := Dependency{
		TargetID:   "db",
		TargetName: "database",
		Type:       DependencyTypeHard,
		MinVersion: "v1.0.0",
		MaxVersion: "v2.0.0",
		Timeout:    30 * time.Second,
		HealthCheck: &HealthCheck{
			Type:     "http",
			Endpoint: "/health",
			Interval: 10 * time.Second,
			Timeout:  5 * time.Second,
			Retries:  3,
		},
	}

	if dep.HealthCheck == nil {
		t.Error("HealthCheck should not be nil")
	}
	if dep.HealthCheck.Type != "http" {
		t.Errorf("HealthCheck.Type = %s, want http", dep.HealthCheck.Type)
	}
}

func TestHealthCheck(t *testing.T) {
	tests := []struct {
		name string
		hc   HealthCheck
	}{
		{
			name: "http",
			hc: HealthCheck{
				Type:     "http",
				Endpoint: "http://localhost:8080/health",
				Interval: 10 * time.Second,
				Timeout:  5 * time.Second,
				Retries:  3,
			},
		},
		{
			name: "tcp",
			hc: HealthCheck{
				Type:     "tcp",
				Endpoint: "localhost:5432",
				Interval: 5 * time.Second,
				Timeout:  2 * time.Second,
				Retries:  5,
			},
		},
		{
			name: "exec",
			hc: HealthCheck{
				Type:     "exec",
				Command:  []string{"pg_isready", "-h", "localhost"},
				Interval: 10 * time.Second,
				Timeout:  10 * time.Second,
				Retries:  3,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.hc.Type != tt.name {
				t.Errorf("Type = %s, want %s", tt.hc.Type, tt.name)
			}
		})
	}
}

func TestGraphStats(t *testing.T) {
	stats := &GraphStats{
		TotalDeployments:    10,
		TotalDependencies:   25,
		ByState:             map[State]int{StateHealthy: 8, StatePending: 2},
		AverageDependencies: 2.5,
		MaxDependencies:     5,
		RootNodes:           3,
		LeafNodes:           4,
		CriticalPathLen:     5,
	}

	if stats.TotalDeployments != 10 {
		t.Errorf("TotalDeployments = %d, want 10", stats.TotalDeployments)
	}
	if stats.ByState[StateHealthy] != 8 {
		t.Errorf("ByState[healthy] = %d, want 8", stats.ByState[StateHealthy])
	}
}

func TestGraph_GetCriticalPath(t *testing.T) {
	g := NewGraph()

	// Create a chain: a -> b -> c -> d
	g.AddDeployment(&Deployment{ID: "d", Name: "d"})
	g.AddDeployment(&Deployment{
		ID:           "c",
		Name:         "c",
		Dependencies: []Dependency{{TargetID: "d", Type: DependencyTypeHard}},
	})
	g.AddDeployment(&Deployment{
		ID:           "b",
		Name:         "b",
		Dependencies: []Dependency{{TargetID: "c", Type: DependencyTypeHard}},
	})
	g.AddDeployment(&Deployment{
		ID:           "a",
		Name:         "a",
		Dependencies: []Dependency{{TargetID: "b", Type: DependencyTypeHard}},
	})

	// Add a shorter chain: e -> d
	g.AddDeployment(&Deployment{
		ID:           "e",
		Name:         "e",
		Dependencies: []Dependency{{TargetID: "d", Type: DependencyTypeHard}},
	})

	path := g.GetCriticalPath()
	if len(path) < 4 {
		t.Errorf("Critical path length = %d, want at least 4", len(path))
	}
}

func TestGraph_UpdateState(t *testing.T) {
	g := NewGraph()

	g.AddDeployment(&Deployment{ID: "svc", Name: "service", State: StatePending})

	err := g.UpdateState("svc", StateDeploying)
	if err != nil {
		t.Fatalf("UpdateState failed: %v", err)
	}

	d, _ := g.GetDeployment("svc")
	if d.State != StateDeploying {
		t.Errorf("State = %s, want deploying", d.State)
	}
}

func TestGraph_UpdateState_NotFound(t *testing.T) {
	g := NewGraph()

	err := g.UpdateState("nonexistent", StateHealthy)
	if err == nil {
		t.Error("Expected error for nonexistent deployment")
	}
}

func TestGraph_GetImpactedDeployments(t *testing.T) {
	g := NewGraph()

	g.AddDeployment(&Deployment{ID: "db", Name: "database"})
	g.AddDeployment(&Deployment{
		ID:           "api",
		Dependencies: []Dependency{{TargetID: "db", Type: DependencyTypeHard}},
	})
	g.AddDeployment(&Deployment{
		ID:           "worker",
		Dependencies: []Dependency{{TargetID: "db", Type: DependencyTypeHard}},
	})
	g.AddDeployment(&Deployment{
		ID:           "frontend",
		Dependencies: []Dependency{{TargetID: "api", Type: DependencyTypeHard}},
	})

	impacted := g.GetImpactedDeployments("db")
	if len(impacted) != 3 { // api, worker, frontend
		t.Errorf("Impacted = %d, want 3", len(impacted))
	}
}

func TestOrchestratorEvent(t *testing.T) {
	event := &OrchestratorEvent{
		Type:         "deploying",
		DeploymentID: "svc-1",
		Timestamp:    time.Now(),
		Message:      "Deploying service-1",
	}

	if event.Type != "deploying" {
		t.Errorf("Type = %s, want deploying", event.Type)
	}
}

func TestNodeViz(t *testing.T) {
	node := NodeViz{
		ID:    "svc-1",
		Label: "Service 1",
		State: StateHealthy,
		Metadata: map[string]string{
			"version": "v1.0.0",
		},
	}

	if node.ID != "svc-1" {
		t.Errorf("ID = %s, want svc-1", node.ID)
	}
}

func TestEdgeViz(t *testing.T) {
	edge := EdgeViz{
		Source: "api",
		Target: "db",
		Type:   DependencyTypeHard,
	}

	if edge.Type != DependencyTypeHard {
		t.Errorf("Type = %s, want hard", edge.Type)
	}
}

func TestGraph_GetRollbackOrder(t *testing.T) {
	g := NewGraph()

	g.AddDeployment(&Deployment{ID: "db", Name: "database"})
	g.AddDeployment(&Deployment{
		ID:           "api",
		Dependencies: []Dependency{{TargetID: "db", Type: DependencyTypeHard}},
	})
	g.AddDeployment(&Deployment{
		ID:           "frontend",
		Dependencies: []Dependency{{TargetID: "api", Type: DependencyTypeHard}},
	})

	order, err := g.GetRollbackOrder()
	if err != nil {
		t.Fatalf("GetRollbackOrder failed: %v", err)
	}

	// Rollback order should be reverse of deploy order
	positions := make(map[string]int)
	for i, d := range order {
		positions[d.ID] = i
	}

	if positions["frontend"] > positions["api"] {
		t.Error("frontend should be rolled back before api")
	}
	if positions["api"] > positions["db"] {
		t.Error("api should be rolled back before db")
	}
}
