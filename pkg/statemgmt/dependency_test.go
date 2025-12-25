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
