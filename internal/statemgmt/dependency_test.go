package statemgmt

import (
	"strings"
	"testing"
)

func TestDependencyResolver_SimpleChain(t *testing.T) {
	resolver := NewDependencyResolver()

	// Create chain: A -> B -> C
	stateA := &StateDeclaration{
		Module: "file",
		ID:     "A",
	}
	stateB := &StateDeclaration{
		Module: "file",
		ID:     "B",
		Requisites: Requisites{
			Require: []StateReference{{Module: "file", ID: "A"}},
		},
	}
	stateC := &StateDeclaration{
		Module: "file",
		ID:     "C",
		Requisites: Requisites{
			Require: []StateReference{{Module: "file", ID: "B"}},
		},
	}

	resolver.AddState(stateA)
	resolver.AddState(stateB)
	resolver.AddState(stateC)

	if err := resolver.BuildGraph(); err != nil {
		t.Fatalf("BuildGraph failed: %v", err)
	}

	order, err := resolver.TopologicalSort()
	if err != nil {
		t.Fatalf("TopologicalSort failed: %v", err)
	}

	if len(order) != 3 {
		t.Fatalf("Expected 3 states, got %d", len(order))
	}

	// A should come before B, B before C
	if order[0].ID != "A" {
		t.Errorf("Expected first state to be A, got %s", order[0].ID)
	}
	if order[1].ID != "B" {
		t.Errorf("Expected second state to be B, got %s", order[1].ID)
	}
	if order[2].ID != "C" {
		t.Errorf("Expected third state to be C, got %s", order[2].ID)
	}
}

func TestDependencyResolver_ParallelExecution(t *testing.T) {
	resolver := NewDependencyResolver()

	// Create: A, B (independent), then C depends on both
	stateA := &StateDeclaration{
		Module: "file",
		ID:     "A",
	}
	stateB := &StateDeclaration{
		Module: "file",
		ID:     "B",
	}
	stateC := &StateDeclaration{
		Module: "file",
		ID:     "C",
		Requisites: Requisites{
			Require: []StateReference{
				{Module: "file", ID: "A"},
				{Module: "file", ID: "B"},
			},
		},
	}

	resolver.AddState(stateA)
	resolver.AddState(stateB)
	resolver.AddState(stateC)

	if err := resolver.BuildGraph(); err != nil {
		t.Fatalf("BuildGraph failed: %v", err)
	}

	order, err := resolver.TopologicalSort()
	if err != nil {
		t.Fatalf("TopologicalSort failed: %v", err)
	}

	if len(order) != 3 {
		t.Fatalf("Expected 3 states, got %d", len(order))
	}

	// Check execution levels
	levels := resolver.GetExecutionLevels()
	if len(levels) != 2 {
		t.Fatalf("Expected 2 execution levels, got %d", len(levels))
	}

	// Level 0 should have A and B (parallel)
	if len(levels[0]) != 2 {
		t.Errorf("Expected 2 states in level 0, got %d", len(levels[0]))
	}

	// Level 1 should have C
	if len(levels[1]) != 1 {
		t.Errorf("Expected 1 state in level 1, got %d", len(levels[1]))
	}
}

func TestDependencyResolver_CircularDependency(t *testing.T) {
	resolver := NewDependencyResolver()

	// Create cycle: A -> B -> C -> A
	stateA := &StateDeclaration{
		Module: "file",
		ID:     "A",
		Requisites: Requisites{
			Require: []StateReference{{Module: "file", ID: "C"}},
		},
	}
	stateB := &StateDeclaration{
		Module: "file",
		ID:     "B",
		Requisites: Requisites{
			Require: []StateReference{{Module: "file", ID: "A"}},
		},
	}
	stateC := &StateDeclaration{
		Module: "file",
		ID:     "C",
		Requisites: Requisites{
			Require: []StateReference{{Module: "file", ID: "B"}},
		},
	}

	resolver.AddState(stateA)
	resolver.AddState(stateB)
	resolver.AddState(stateC)

	if err := resolver.BuildGraph(); err != nil {
		t.Fatalf("BuildGraph failed: %v", err)
	}

	_, err := resolver.TopologicalSort()
	if err == nil {
		t.Fatal("Expected error for circular dependency")
	}

	if !strings.Contains(err.Error(), "circular dependency") {
		t.Errorf("Expected 'circular dependency' in error, got: %v", err)
	}
}

func TestDependencyResolver_RequireIn(t *testing.T) {
	resolver := NewDependencyResolver()

	// A has require_in for B (A must run before B)
	stateA := &StateDeclaration{
		Module: "file",
		ID:     "A",
		Requisites: Requisites{
			RequireIn: []StateReference{{Module: "file", ID: "B"}},
		},
	}
	stateB := &StateDeclaration{
		Module: "file",
		ID:     "B",
	}

	resolver.AddState(stateA)
	resolver.AddState(stateB)

	if err := resolver.BuildGraph(); err != nil {
		t.Fatalf("BuildGraph failed: %v", err)
	}

	order, err := resolver.TopologicalSort()
	if err != nil {
		t.Fatalf("TopologicalSort failed: %v", err)
	}

	if len(order) != 2 {
		t.Fatalf("Expected 2 states, got %d", len(order))
	}

	// A should come before B
	if order[0].ID != "A" {
		t.Errorf("Expected first state to be A, got %s", order[0].ID)
	}
	if order[1].ID != "B" {
		t.Errorf("Expected second state to be B, got %s", order[1].ID)
	}
}

func TestDependencyResolver_MissingRequisite(t *testing.T) {
	resolver := NewDependencyResolver()

	state := &StateDeclaration{
		Module: "file",
		ID:     "missing",
		Requisites: Requisites{
			Require: []StateReference{{Module: "file", ID: "nonexistent"}},
		},
	}

	resolver.AddState(state)

	if err := resolver.BuildGraph(); err == nil {
		t.Fatal("Expected BuildGraph to fail when requisite is missing")
	}
}

func TestDependencyResolver_RequisiteVariants(t *testing.T) {
	resolver := NewDependencyResolver()

	depRequire := &StateDeclaration{Module: "file", ID: "dep-require"}
	depWatch := &StateDeclaration{Module: "file", ID: "dep-watch"}
	depPrereq := &StateDeclaration{Module: "file", ID: "dep-prereq"}
	depOnchanges := &StateDeclaration{Module: "file", ID: "dep-onchanges"}

	targetRequireIn := &StateDeclaration{Module: "file", ID: "target-require-in"}
	targetWatchIn := &StateDeclaration{Module: "file", ID: "target-watch-in"}
	targetPrereqIn := &StateDeclaration{Module: "file", ID: "target-prereq-in"}
	targetOnchangesIn := &StateDeclaration{Module: "file", ID: "target-onchanges-in"}

	dependent := &StateDeclaration{
		Module: "file",
		ID:     "dependent",
		Requisites: Requisites{
			Require:   []StateReference{{Module: "file", ID: depRequire.ID}},
			Watch:     []StateReference{{Module: "file", ID: depWatch.ID}},
			Prereq:    []StateReference{{Module: "file", ID: depPrereq.ID}},
			Onchanges: []StateReference{{Module: "file", ID: depOnchanges.ID}},
		},
	}

	reverse := &StateDeclaration{
		Module: "file",
		ID:     "reverse",
		Requisites: Requisites{
			RequireIn:   []StateReference{{Module: "file", ID: targetRequireIn.ID}},
			WatchIn:     []StateReference{{Module: "file", ID: targetWatchIn.ID}},
			PrereqIn:    []StateReference{{Module: "file", ID: targetPrereqIn.ID}},
			OnchangesIn: []StateReference{{Module: "file", ID: targetOnchangesIn.ID}},
		},
	}

	resolver.AddState(depRequire)
	resolver.AddState(depWatch)
	resolver.AddState(depPrereq)
	resolver.AddState(depOnchanges)
	resolver.AddState(targetRequireIn)
	resolver.AddState(targetWatchIn)
	resolver.AddState(targetPrereqIn)
	resolver.AddState(targetOnchangesIn)
	resolver.AddState(dependent)
	resolver.AddState(reverse)

	if err := resolver.BuildGraph(); err != nil {
		t.Fatalf("BuildGraph failed: %v", err)
	}

	order, err := resolver.TopologicalSort()
	if err != nil {
		t.Fatalf("TopologicalSort failed: %v", err)
	}

	index := make(map[string]int)
	for i, decl := range order {
		index[decl.Module+":"+decl.ID] = i
	}

	checkBefore := func(a, b *StateDeclaration) {
		if index[a.Module+":"+a.ID] >= index[b.Module+":"+b.ID] {
			t.Errorf("Expected %s to run before %s", a.ID, b.ID)
		}
	}

	checkBefore(depRequire, dependent)
	checkBefore(depWatch, dependent)
	checkBefore(depPrereq, dependent)
	checkBefore(depOnchanges, dependent)

	checkBefore(reverse, targetRequireIn)
	checkBefore(reverse, targetWatchIn)
	checkBefore(reverse, targetPrereqIn)
	checkBefore(reverse, targetOnchangesIn)
}

func TestDependencyResolver_Watch(t *testing.T) {
	resolver := NewDependencyResolver()

	// B watches A (A must run before B)
	stateA := &StateDeclaration{
		Module: "file",
		ID:     "A",
	}
	stateB := &StateDeclaration{
		Module: "service",
		ID:     "B",
		Requisites: Requisites{
			Watch: []StateReference{{Module: "file", ID: "A"}},
		},
	}

	resolver.AddState(stateA)
	resolver.AddState(stateB)

	if err := resolver.BuildGraph(); err != nil {
		t.Fatalf("BuildGraph failed: %v", err)
	}

	order, err := resolver.TopologicalSort()
	if err != nil {
		t.Fatalf("TopologicalSort failed: %v", err)
	}

	// A should come before B
	if order[0].ID != "A" {
		t.Errorf("Expected first state to be A, got %s", order[0].ID)
	}
	if order[1].ID != "B" {
		t.Errorf("Expected second state to be B, got %s", order[1].ID)
	}
}

func TestDependencyResolver_NonExistentDependency(t *testing.T) {
	resolver := NewDependencyResolver()

	stateA := &StateDeclaration{
		Module: "file",
		ID:     "A",
		Requisites: Requisites{
			Require: []StateReference{{Module: "file", ID: "NonExistent"}},
		},
	}

	resolver.AddState(stateA)

	err := resolver.BuildGraph()
	if err == nil {
		t.Fatal("Expected error for non-existent dependency")
	}

	if !strings.Contains(err.Error(), "non-existent") {
		t.Errorf("Expected 'non-existent' in error, got: %v", err)
	}
}

func TestDependencyResolver_ComplexGraph(t *testing.T) {
	resolver := NewDependencyResolver()

	// Create complex graph:
	//     A   B
	//     |\ /|
	//     | X |
	//     |/ \|
	//     C   D
	//      \ /
	//       E

	stateA := &StateDeclaration{Module: "file", ID: "A"}
	stateB := &StateDeclaration{Module: "file", ID: "B"}
	stateC := &StateDeclaration{
		Module: "file",
		ID:     "C",
		Requisites: Requisites{
			Require: []StateReference{
				{Module: "file", ID: "A"},
				{Module: "file", ID: "B"},
			},
		},
	}
	stateD := &StateDeclaration{
		Module: "file",
		ID:     "D",
		Requisites: Requisites{
			Require: []StateReference{
				{Module: "file", ID: "A"},
				{Module: "file", ID: "B"},
			},
		},
	}
	stateE := &StateDeclaration{
		Module: "file",
		ID:     "E",
		Requisites: Requisites{
			Require: []StateReference{
				{Module: "file", ID: "C"},
				{Module: "file", ID: "D"},
			},
		},
	}

	resolver.AddState(stateA)
	resolver.AddState(stateB)
	resolver.AddState(stateC)
	resolver.AddState(stateD)
	resolver.AddState(stateE)

	if err := resolver.BuildGraph(); err != nil {
		t.Fatalf("BuildGraph failed: %v", err)
	}

	order, err := resolver.TopologicalSort()
	if err != nil {
		t.Fatalf("TopologicalSort failed: %v", err)
	}

	if len(order) != 5 {
		t.Fatalf("Expected 5 states, got %d", len(order))
	}

	// Check levels
	levels := resolver.GetExecutionLevels()
	if len(levels) != 3 {
		t.Fatalf("Expected 3 execution levels, got %d", len(levels))
	}

	// Level 0: A, B
	if len(levels[0]) != 2 {
		t.Errorf("Expected 2 states in level 0, got %d", len(levels[0]))
	}

	// Level 1: C, D
	if len(levels[1]) != 2 {
		t.Errorf("Expected 2 states in level 1, got %d", len(levels[1]))
	}

	// Level 2: E
	if len(levels[2]) != 1 {
		t.Errorf("Expected 1 state in level 2, got %d", len(levels[2]))
	}

	// E should be last
	if order[4].ID != "E" {
		t.Errorf("Expected last state to be E, got %s", order[4].ID)
	}
}

func TestResolveExecutionOrder(t *testing.T) {
	stateFile := &StateFile{
		Path: "test.yaml",
		States: map[string][]StateDeclaration{
			"file": {
				{Module: "file", ID: "A"},
				{
					Module: "file",
					ID:     "B",
					Requisites: Requisites{
						Require: []StateReference{{Module: "file", ID: "A"}},
					},
				},
			},
		},
	}

	order, err := ResolveExecutionOrder(stateFile)
	if err != nil {
		t.Fatalf("ResolveExecutionOrder failed: %v", err)
	}

	if len(order) != 2 {
		t.Fatalf("Expected 2 states, got %d", len(order))
	}

	if order[0].ID != "A" {
		t.Errorf("Expected first state to be A, got %s", order[0].ID)
	}
	if order[1].ID != "B" {
		t.Errorf("Expected second state to be B, got %s", order[1].ID)
	}
}

func TestDependencyResolver_GetParallelizableGroups(t *testing.T) {
	resolver := NewDependencyResolver()

	stateA := &StateDeclaration{Module: "file", ID: "A"}
	stateB := &StateDeclaration{Module: "file", ID: "B"}
	stateC := &StateDeclaration{
		Module: "file",
		ID:     "C",
		Requisites: Requisites{
			Require: []StateReference{
				{Module: "file", ID: "A"},
				{Module: "file", ID: "B"},
			},
		},
	}

	resolver.AddState(stateA)
	resolver.AddState(stateB)
	resolver.AddState(stateC)

	if err := resolver.BuildGraph(); err != nil {
		t.Fatalf("BuildGraph failed: %v", err)
	}

	_, err := resolver.TopologicalSort()
	if err != nil {
		t.Fatalf("TopologicalSort failed: %v", err)
	}

	groups := resolver.GetParallelizableGroups()
	if len(groups) != 2 {
		t.Fatalf("Expected 2 groups, got %d", len(groups))
	}

	// First group should have 2 states (A, B)
	if len(groups[0]) != 2 {
		t.Errorf("Expected 2 states in first group, got %d", len(groups[0]))
	}

	// Second group should have 1 state (C)
	if len(groups[1]) != 1 {
		t.Errorf("Expected 1 state in second group, got %d", len(groups[1]))
	}
}

func TestDependencyResolver_MixedRequisitesLevels(t *testing.T) {
	resolver := NewDependencyResolver()

	stateA := &StateDeclaration{Module: "file", ID: "A"}
	stateB := &StateDeclaration{
		Module: "file",
		ID:     "B",
		Requisites: Requisites{
			Require: []StateReference{{Module: "file", ID: "A"}},
		},
	}
	stateC := &StateDeclaration{
		Module: "file",
		ID:     "C",
		Requisites: Requisites{
			RequireIn: []StateReference{{Module: "file", ID: "B"}},
		},
	}
	stateD := &StateDeclaration{
		Module: "service",
		ID:     "D",
		Requisites: Requisites{
			Watch: []StateReference{{Module: "file", ID: "B"}},
		},
	}
	stateE := &StateDeclaration{
		Module: "file",
		ID:     "E",
		Requisites: Requisites{
			OnchangesIn: []StateReference{{Module: "service", ID: "D"}},
		},
	}

	resolver.AddState(stateA)
	resolver.AddState(stateB)
	resolver.AddState(stateC)
	resolver.AddState(stateD)
	resolver.AddState(stateE)

	if err := resolver.BuildGraph(); err != nil {
		t.Fatalf("BuildGraph failed: %v", err)
	}

	_, err := resolver.TopologicalSort()
	if err != nil {
		t.Fatalf("TopologicalSort failed: %v", err)
	}

	levels := resolver.GetExecutionLevels()
	if len(levels) != 3 {
		t.Fatalf("Expected 3 execution levels, got %d", len(levels))
	}

	assertLevelStateIDs(t, levels[0], []string{"A", "C", "E"})
	assertLevelStateIDs(t, levels[1], []string{"B"})
	assertLevelStateIDs(t, levels[2], []string{"D"})
}

func TestDependencyResolver_CycleWithRequireIn(t *testing.T) {
	resolver := NewDependencyResolver()

	stateA := &StateDeclaration{
		Module: "file",
		ID:     "A",
		Requisites: Requisites{
			RequireIn: []StateReference{{Module: "file", ID: "B"}},
		},
	}
	stateB := &StateDeclaration{
		Module: "file",
		ID:     "B",
		Requisites: Requisites{
			RequireIn: []StateReference{{Module: "file", ID: "C"}},
		},
	}
	stateC := &StateDeclaration{
		Module: "file",
		ID:     "C",
		Requisites: Requisites{
			RequireIn: []StateReference{{Module: "file", ID: "A"}},
		},
	}

	resolver.AddState(stateA)
	resolver.AddState(stateB)
	resolver.AddState(stateC)

	if err := resolver.BuildGraph(); err != nil {
		t.Fatalf("BuildGraph failed: %v", err)
	}

	_, err := resolver.TopologicalSort()
	if err == nil {
		t.Fatal("Expected error for circular dependency")
	}
	if !strings.Contains(err.Error(), "circular dependency") {
		t.Fatalf("Expected 'circular dependency' in error, got: %v", err)
	}
}

func assertLevelStateIDs(t *testing.T, level []string, expectedIDs []string) {
	t.Helper()

	expected := make(map[string]bool, len(expectedIDs))
	for _, id := range expectedIDs {
		expected[id] = true
	}

	if len(level) != len(expectedIDs) {
		t.Fatalf("Expected %d states in level, got %d", len(expectedIDs), len(level))
	}

	for _, key := range level {
		parts := strings.SplitN(key, ":", 2)
		id := key
		if len(parts) == 2 {
			id = parts[1]
		}
		if !expected[id] {
			t.Fatalf("Unexpected state in level: %s", id)
		}
	}
}

func TestDependencyResolver_WatchIn(t *testing.T) {
	resolver := NewDependencyResolver()

	// A has watch_in for B (A must run before B)
	stateA := &StateDeclaration{
		Module: "file",
		ID:     "A",
		Requisites: Requisites{
			WatchIn: []StateReference{{Module: "file", ID: "B"}},
		},
	}
	stateB := &StateDeclaration{
		Module: "file",
		ID:     "B",
	}

	resolver.AddState(stateA)
	resolver.AddState(stateB)

	if err := resolver.BuildGraph(); err != nil {
		t.Fatalf("BuildGraph failed: %v", err)
	}

	order, err := resolver.TopologicalSort()
	if err != nil {
		t.Fatalf("TopologicalSort failed: %v", err)
	}

	if len(order) != 2 {
		t.Fatalf("Expected 2 states, got %d", len(order))
	}

	// A should come before B
	if order[0].ID != "A" {
		t.Errorf("Expected first state to be A, got %s", order[0].ID)
	}
	if order[1].ID != "B" {
		t.Errorf("Expected second state to be B, got %s", order[1].ID)
	}
}

func TestDependencyResolver_Prereq(t *testing.T) {
	resolver := NewDependencyResolver()

	// B has prereq on A (A must run before B)
	stateA := &StateDeclaration{
		Module: "file",
		ID:     "A",
	}
	stateB := &StateDeclaration{
		Module: "service",
		ID:     "B",
		Requisites: Requisites{
			Prereq: []StateReference{{Module: "file", ID: "A"}},
		},
	}

	resolver.AddState(stateA)
	resolver.AddState(stateB)

	if err := resolver.BuildGraph(); err != nil {
		t.Fatalf("BuildGraph failed: %v", err)
	}

	order, err := resolver.TopologicalSort()
	if err != nil {
		t.Fatalf("TopologicalSort failed: %v", err)
	}

	// A should come before B
	if order[0].ID != "A" {
		t.Errorf("Expected first state to be A, got %s", order[0].ID)
	}
	if order[1].ID != "B" {
		t.Errorf("Expected second state to be B, got %s", order[1].ID)
	}
}

func TestDependencyResolver_PrereqIn(t *testing.T) {
	resolver := NewDependencyResolver()

	// A has prereq_in for B (A must run before B)
	stateA := &StateDeclaration{
		Module: "file",
		ID:     "A",
		Requisites: Requisites{
			PrereqIn: []StateReference{{Module: "file", ID: "B"}},
		},
	}
	stateB := &StateDeclaration{
		Module: "file",
		ID:     "B",
	}

	resolver.AddState(stateA)
	resolver.AddState(stateB)

	if err := resolver.BuildGraph(); err != nil {
		t.Fatalf("BuildGraph failed: %v", err)
	}

	order, err := resolver.TopologicalSort()
	if err != nil {
		t.Fatalf("TopologicalSort failed: %v", err)
	}

	if len(order) != 2 {
		t.Fatalf("Expected 2 states, got %d", len(order))
	}

	// A should come before B
	if order[0].ID != "A" {
		t.Errorf("Expected first state to be A, got %s", order[0].ID)
	}
	if order[1].ID != "B" {
		t.Errorf("Expected second state to be B, got %s", order[1].ID)
	}
}

func TestDependencyResolver_Onchanges(t *testing.T) {
	resolver := NewDependencyResolver()

	// B has onchanges on A (A must run before B)
	stateA := &StateDeclaration{
		Module: "file",
		ID:     "A",
	}
	stateB := &StateDeclaration{
		Module: "service",
		ID:     "B",
		Requisites: Requisites{
			Onchanges: []StateReference{{Module: "file", ID: "A"}},
		},
	}

	resolver.AddState(stateA)
	resolver.AddState(stateB)

	if err := resolver.BuildGraph(); err != nil {
		t.Fatalf("BuildGraph failed: %v", err)
	}

	order, err := resolver.TopologicalSort()
	if err != nil {
		t.Fatalf("TopologicalSort failed: %v", err)
	}

	// A should come before B
	if order[0].ID != "A" {
		t.Errorf("Expected first state to be A, got %s", order[0].ID)
	}
	if order[1].ID != "B" {
		t.Errorf("Expected second state to be B, got %s", order[1].ID)
	}
}

func TestDependencyResolver_NonExistentRequireIn(t *testing.T) {
	resolver := NewDependencyResolver()

	stateA := &StateDeclaration{
		Module: "file",
		ID:     "A",
		Requisites: Requisites{
			RequireIn: []StateReference{{Module: "file", ID: "NonExistent"}},
		},
	}

	resolver.AddState(stateA)

	err := resolver.BuildGraph()
	if err == nil {
		t.Fatal("Expected error for non-existent require_in dependency")
	}

	if !strings.Contains(err.Error(), "non-existent") {
		t.Errorf("Expected 'non-existent' in error, got: %v", err)
	}
}

func TestDependencyResolver_NonExistentWatch(t *testing.T) {
	resolver := NewDependencyResolver()

	stateA := &StateDeclaration{
		Module: "file",
		ID:     "A",
		Requisites: Requisites{
			Watch: []StateReference{{Module: "file", ID: "NonExistent"}},
		},
	}

	resolver.AddState(stateA)

	err := resolver.BuildGraph()
	if err == nil {
		t.Fatal("Expected error for non-existent watch dependency")
	}

	if !strings.Contains(err.Error(), "non-existent") {
		t.Errorf("Expected 'non-existent' in error, got: %v", err)
	}
}

func TestDependencyResolver_NonExistentWatchIn(t *testing.T) {
	resolver := NewDependencyResolver()

	stateA := &StateDeclaration{
		Module: "file",
		ID:     "A",
		Requisites: Requisites{
			WatchIn: []StateReference{{Module: "file", ID: "NonExistent"}},
		},
	}

	resolver.AddState(stateA)

	err := resolver.BuildGraph()
	if err == nil {
		t.Fatal("Expected error for non-existent watch_in dependency")
	}

	if !strings.Contains(err.Error(), "non-existent") {
		t.Errorf("Expected 'non-existent' in error, got: %v", err)
	}
}

func TestDependencyResolver_NonExistentPrereq(t *testing.T) {
	resolver := NewDependencyResolver()

	stateA := &StateDeclaration{
		Module: "file",
		ID:     "A",
		Requisites: Requisites{
			Prereq: []StateReference{{Module: "file", ID: "NonExistent"}},
		},
	}

	resolver.AddState(stateA)

	err := resolver.BuildGraph()
	if err == nil {
		t.Fatal("Expected error for non-existent prereq dependency")
	}

	if !strings.Contains(err.Error(), "non-existent") {
		t.Errorf("Expected 'non-existent' in error, got: %v", err)
	}
}

func TestDependencyResolver_NonExistentPrereqIn(t *testing.T) {
	resolver := NewDependencyResolver()

	stateA := &StateDeclaration{
		Module: "file",
		ID:     "A",
		Requisites: Requisites{
			PrereqIn: []StateReference{{Module: "file", ID: "NonExistent"}},
		},
	}

	resolver.AddState(stateA)

	err := resolver.BuildGraph()
	if err == nil {
		t.Fatal("Expected error for non-existent prereq_in dependency")
	}

	if !strings.Contains(err.Error(), "non-existent") {
		t.Errorf("Expected 'non-existent' in error, got: %v", err)
	}
}

func TestDependencyResolver_NonExistentOnchanges(t *testing.T) {
	resolver := NewDependencyResolver()

	stateA := &StateDeclaration{
		Module: "file",
		ID:     "A",
		Requisites: Requisites{
			Onchanges: []StateReference{{Module: "file", ID: "NonExistent"}},
		},
	}

	resolver.AddState(stateA)

	err := resolver.BuildGraph()
	if err == nil {
		t.Fatal("Expected error for non-existent onchanges dependency")
	}

	if !strings.Contains(err.Error(), "non-existent") {
		t.Errorf("Expected 'non-existent' in error, got: %v", err)
	}
}

func TestDependencyResolver_NonExistentOnchangesIn(t *testing.T) {
	resolver := NewDependencyResolver()

	stateA := &StateDeclaration{
		Module: "file",
		ID:     "A",
		Requisites: Requisites{
			OnchangesIn: []StateReference{{Module: "file", ID: "NonExistent"}},
		},
	}

	resolver.AddState(stateA)

	err := resolver.BuildGraph()
	if err == nil {
		t.Fatal("Expected error for non-existent onchanges_in dependency")
	}

	if !strings.Contains(err.Error(), "non-existent") {
		t.Errorf("Expected 'non-existent' in error, got: %v", err)
	}
}

func TestDependencyResolver_MakeStateKey(t *testing.T) {
	resolver := NewDependencyResolver()
	key := resolver.makeStateKey("file", "test-id")
	expected := "file:test-id"
	if key != expected {
		t.Errorf("Expected key %q, got %q", expected, key)
	}
}

func TestDependencyResolver_StateExists(t *testing.T) {
	resolver := NewDependencyResolver()

	// State doesn't exist initially
	if resolver.stateExists("file:A") {
		t.Error("Expected state to not exist")
	}

	// Add a state
	stateA := &StateDeclaration{Module: "file", ID: "A"}
	resolver.AddState(stateA)

	// Now it should exist
	if !resolver.stateExists("file:A") {
		t.Error("Expected state to exist")
	}

	// Different key should not exist
	if resolver.stateExists("file:B") {
		t.Error("Expected state B to not exist")
	}
}

func TestDependencyResolver_EmptyGraph(t *testing.T) {
	resolver := NewDependencyResolver()

	// Build empty graph
	if err := resolver.BuildGraph(); err != nil {
		t.Fatalf("BuildGraph failed: %v", err)
	}

	order, err := resolver.TopologicalSort()
	if err != nil {
		t.Fatalf("TopologicalSort failed: %v", err)
	}

	if len(order) != 0 {
		t.Errorf("Expected empty order, got %d states", len(order))
	}
}

func TestDependencyResolver_SingleState(t *testing.T) {
	resolver := NewDependencyResolver()

	stateA := &StateDeclaration{Module: "file", ID: "A"}
	resolver.AddState(stateA)

	if err := resolver.BuildGraph(); err != nil {
		t.Fatalf("BuildGraph failed: %v", err)
	}

	order, err := resolver.TopologicalSort()
	if err != nil {
		t.Fatalf("TopologicalSort failed: %v", err)
	}

	if len(order) != 1 {
		t.Fatalf("Expected 1 state, got %d", len(order))
	}

	if order[0].ID != "A" {
		t.Errorf("Expected state A, got %s", order[0].ID)
	}
}

func TestDependencyResolver_AllRequisiteTypes(t *testing.T) {
	// Test that all 8 requisite types can be used together
	resolver := NewDependencyResolver()

	// Create states with different requisite types
	stateBase := &StateDeclaration{Module: "file", ID: "base"}
	stateReq := &StateDeclaration{
		Module: "file",
		ID:     "req",
		Requisites: Requisites{
			Require: []StateReference{{Module: "file", ID: "base"}},
		},
	}
	stateReqIn := &StateDeclaration{
		Module: "file",
		ID:     "reqin",
		Requisites: Requisites{
			RequireIn: []StateReference{{Module: "file", ID: "req"}},
		},
	}
	stateWatch := &StateDeclaration{
		Module: "file",
		ID:     "watch",
		Requisites: Requisites{
			Watch: []StateReference{{Module: "file", ID: "base"}},
		},
	}
	stateWatchIn := &StateDeclaration{
		Module: "file",
		ID:     "watchin",
		Requisites: Requisites{
			WatchIn: []StateReference{{Module: "file", ID: "watch"}},
		},
	}
	statePrereq := &StateDeclaration{
		Module: "file",
		ID:     "prereq",
		Requisites: Requisites{
			Prereq: []StateReference{{Module: "file", ID: "base"}},
		},
	}
	statePrereqIn := &StateDeclaration{
		Module: "file",
		ID:     "prereqin",
		Requisites: Requisites{
			PrereqIn: []StateReference{{Module: "file", ID: "prereq"}},
		},
	}
	stateOnchanges := &StateDeclaration{
		Module: "file",
		ID:     "onchanges",
		Requisites: Requisites{
			Onchanges: []StateReference{{Module: "file", ID: "base"}},
		},
	}
	stateOnchangesIn := &StateDeclaration{
		Module: "file",
		ID:     "onchangesin",
		Requisites: Requisites{
			OnchangesIn: []StateReference{{Module: "file", ID: "onchanges"}},
		},
	}

	resolver.AddState(stateBase)
	resolver.AddState(stateReq)
	resolver.AddState(stateReqIn)
	resolver.AddState(stateWatch)
	resolver.AddState(stateWatchIn)
	resolver.AddState(statePrereq)
	resolver.AddState(statePrereqIn)
	resolver.AddState(stateOnchanges)
	resolver.AddState(stateOnchangesIn)

	if err := resolver.BuildGraph(); err != nil {
		t.Fatalf("BuildGraph failed: %v", err)
	}

	order, err := resolver.TopologicalSort()
	if err != nil {
		t.Fatalf("TopologicalSort failed: %v", err)
	}

	if len(order) != 9 {
		t.Fatalf("Expected 9 states, got %d", len(order))
	}
}
