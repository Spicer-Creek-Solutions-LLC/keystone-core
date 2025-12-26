package resolver

import (
	"testing"
)

func TestDAG_AddNode(t *testing.T) {
	g := NewDependencyGraph()

	node := &DependencyNode{
		Module: ModuleReference{
			Name:    "test/module",
			Version: "1.0.0",
		},
	}

	err := g.AddNode(node)
	if err != nil {
		t.Fatalf("AddNode() error = %v", err)
	}

	retrieved, err := g.GetNode("test/module")
	if err != nil {
		t.Fatalf("GetNode() error = %v", err)
	}

	if retrieved.Module.Name != node.Module.Name {
		t.Errorf("GetNode() = %v, want %v", retrieved.Module.Name, node.Module.Name)
	}
}

func TestDAG_AddNodeWithDependencies(t *testing.T) {
	g := NewDependencyGraph()

	dep1 := &DependencyNode{
		Module: ModuleReference{
			Name:    "dep1",
			Version: "1.0.0",
		},
	}

	dep2 := &DependencyNode{
		Module: ModuleReference{
			Name:    "dep2",
			Version: "2.0.0",
		},
	}

	root := &DependencyNode{
		Module: ModuleReference{
			Name:    "root",
			Version: "1.0.0",
		},
		Dependencies: []*DependencyNode{dep1, dep2},
	}

	g.AddNode(dep1)
	g.AddNode(dep2)
	g.AddNode(root)

	deps, err := g.GetDependencies("root")
	if err != nil {
		t.Fatalf("GetDependencies() error = %v", err)
	}

	if len(deps) != 2 {
		t.Errorf("GetDependencies() = %v, want 2 dependencies", len(deps))
	}
}

func TestDAG_GetAllNodes(t *testing.T) {
	g := NewDependencyGraph()

	nodes := []*DependencyNode{
		{Module: ModuleReference{Name: "a", Version: "1.0.0"}},
		{Module: ModuleReference{Name: "b", Version: "1.0.0"}},
		{Module: ModuleReference{Name: "c", Version: "1.0.0"}},
	}

	for _, node := range nodes {
		g.AddNode(node)
	}

	all := g.GetAllNodes()
	if len(all) != 3 {
		t.Errorf("GetAllNodes() = %v nodes, want 3", len(all))
	}
}

func TestDAG_HasCycle_NoCycle(t *testing.T) {
	g := NewDependencyGraph()

	// a -> b -> c (no cycle)
	c := &DependencyNode{Module: ModuleReference{Name: "c", Version: "1.0.0"}}
	b := &DependencyNode{
		Module:       ModuleReference{Name: "b", Version: "1.0.0"},
		Dependencies: []*DependencyNode{c},
	}
	a := &DependencyNode{
		Module:       ModuleReference{Name: "a", Version: "1.0.0"},
		Dependencies: []*DependencyNode{b},
	}

	g.AddNode(c)
	g.AddNode(b)
	g.AddNode(a)

	hasCycle, _ := g.HasCycle()
	if hasCycle {
		t.Error("HasCycle() = true, want false")
	}
}

func TestDAG_HasCycle_WithCycle(t *testing.T) {
	g := NewDependencyGraph()

	// Create a cycle: a -> b -> c -> a
	c := &DependencyNode{Module: ModuleReference{Name: "c", Version: "1.0.0"}}
	b := &DependencyNode{
		Module:       ModuleReference{Name: "b", Version: "1.0.0"},
		Dependencies: []*DependencyNode{c},
	}
	a := &DependencyNode{
		Module:       ModuleReference{Name: "a", Version: "1.0.0"},
		Dependencies: []*DependencyNode{b},
	}

	// Add nodes first
	g.AddNode(c)
	g.AddNode(b)
	g.AddNode(a)

	// Now create the cycle by adding a as a dependency of c
	c.Dependencies = []*DependencyNode{a}
	g.AddNode(c) // Re-add to update edges

	hasCycle, cyclePath := g.HasCycle()
	if !hasCycle {
		t.Error("HasCycle() = false, want true")
	}

	if len(cyclePath) == 0 {
		t.Error("HasCycle() cycle path is empty, want non-empty")
	}
}

func TestDAG_TopologicalSort(t *testing.T) {
	g := NewDependencyGraph()

	// Create dependency chain: a -> b -> c
	c := &DependencyNode{Module: ModuleReference{Name: "c", Version: "1.0.0"}}
	b := &DependencyNode{
		Module:       ModuleReference{Name: "b", Version: "1.0.0"},
		Dependencies: []*DependencyNode{c},
	}
	a := &DependencyNode{
		Module:       ModuleReference{Name: "a", Version: "1.0.0"},
		Dependencies: []*DependencyNode{b},
	}

	g.AddNode(c)
	g.AddNode(b)
	g.AddNode(a)

	sorted, err := g.TopologicalSort()
	if err != nil {
		t.Fatalf("TopologicalSort() error = %v", err)
	}

	if len(sorted) != 3 {
		t.Fatalf("TopologicalSort() = %v nodes, want 3", len(sorted))
	}

	// c should come before b, b should come before a
	cIndex, bIndex, aIndex := -1, -1, -1
	for i, node := range sorted {
		switch node.Module.Name {
		case "c":
			cIndex = i
		case "b":
			bIndex = i
		case "a":
			aIndex = i
		}
	}

	if cIndex >= bIndex {
		t.Errorf("c should come before b in topological sort")
	}
	if bIndex >= aIndex {
		t.Errorf("b should come before a in topological sort")
	}
}

func TestDAG_TopologicalSort_WithCycle(t *testing.T) {
	g := NewDependencyGraph()

	// Create a cycle: a -> b -> a
	b := &DependencyNode{Module: ModuleReference{Name: "b", Version: "1.0.0"}}
	a := &DependencyNode{
		Module:       ModuleReference{Name: "a", Version: "1.0.0"},
		Dependencies: []*DependencyNode{b},
	}

	g.AddNode(b)
	g.AddNode(a)

	// Create cycle
	b.Dependencies = []*DependencyNode{a}
	g.AddNode(b)

	_, err := g.TopologicalSort()
	if err == nil {
		t.Error("TopologicalSort() with cycle should return error")
	}
}

func TestDAG_Flatten(t *testing.T) {
	g := NewDependencyGraph()

	c := &DependencyNode{Module: ModuleReference{Name: "c", Version: "1.0.0"}}
	b := &DependencyNode{
		Module:       ModuleReference{Name: "b", Version: "2.0.0"},
		Dependencies: []*DependencyNode{c},
	}
	a := &DependencyNode{
		Module:       ModuleReference{Name: "a", Version: "3.0.0"},
		Dependencies: []*DependencyNode{b},
	}

	g.AddNode(c)
	g.AddNode(b)
	g.AddNode(a)

	flattened := g.Flatten()
	if len(flattened) != 3 {
		t.Errorf("Flatten() = %v modules, want 3", len(flattened))
	}

	// Check that all modules are present
	found := make(map[string]bool)
	for _, ref := range flattened {
		found[ref.Name] = true
	}

	for _, name := range []string{"a", "b", "c"} {
		if !found[name] {
			t.Errorf("Flatten() missing module %s", name)
		}
	}
}

func TestDAG_GetDependents(t *testing.T) {
	g := NewDependencyGraph()

	// Create: a -> c, b -> c (both a and b depend on c)
	c := &DependencyNode{Module: ModuleReference{Name: "c", Version: "1.0.0"}}
	b := &DependencyNode{
		Module:       ModuleReference{Name: "b", Version: "1.0.0"},
		Dependencies: []*DependencyNode{c},
	}
	a := &DependencyNode{
		Module:       ModuleReference{Name: "a", Version: "1.0.0"},
		Dependencies: []*DependencyNode{c},
	}

	g.AddNode(c)
	g.AddNode(b)
	g.AddNode(a)

	dependents, err := g.GetDependents("c")
	if err != nil {
		t.Fatalf("GetDependents() error = %v", err)
	}

	if len(dependents) != 2 {
		t.Errorf("GetDependents() = %v, want 2 dependents", len(dependents))
	}

	// Check that both a and b are dependents
	found := make(map[string]bool)
	for _, dep := range dependents {
		found[dep] = true
	}

	if !found["a"] || !found["b"] {
		t.Errorf("GetDependents() = %v, want both a and b", dependents)
	}
}

func TestDAG_Size(t *testing.T) {
	g := NewDependencyGraph()

	if g.Size() != 0 {
		t.Errorf("Size() = %v, want 0", g.Size())
	}

	g.AddNode(&DependencyNode{Module: ModuleReference{Name: "a", Version: "1.0.0"}})
	if g.Size() != 1 {
		t.Errorf("Size() = %v, want 1", g.Size())
	}

	g.AddNode(&DependencyNode{Module: ModuleReference{Name: "b", Version: "1.0.0"}})
	if g.Size() != 2 {
		t.Errorf("Size() = %v, want 2", g.Size())
	}
}

func TestDAG_Clear(t *testing.T) {
	g := NewDependencyGraph()

	g.AddNode(&DependencyNode{Module: ModuleReference{Name: "a", Version: "1.0.0"}})
	g.AddNode(&DependencyNode{Module: ModuleReference{Name: "b", Version: "1.0.0"}})

	if g.Size() != 2 {
		t.Errorf("Size() before clear = %v, want 2", g.Size())
	}

	g.Clear()

	if g.Size() != 0 {
		t.Errorf("Size() after clear = %v, want 0", g.Size())
	}
}

func TestDAG_ComplexGraph(t *testing.T) {
	g := NewDependencyGraph()

	/*
	   Create a complex dependency graph:
	       a
	      / \
	     b   c
	      \ / \
	       d   e
	*/

	e := &DependencyNode{Module: ModuleReference{Name: "e", Version: "1.0.0"}}
	d := &DependencyNode{Module: ModuleReference{Name: "d", Version: "1.0.0"}}
	c := &DependencyNode{
		Module:       ModuleReference{Name: "c", Version: "1.0.0"},
		Dependencies: []*DependencyNode{d, e},
	}
	b := &DependencyNode{
		Module:       ModuleReference{Name: "b", Version: "1.0.0"},
		Dependencies: []*DependencyNode{d},
	}
	a := &DependencyNode{
		Module:       ModuleReference{Name: "a", Version: "1.0.0"},
		Dependencies: []*DependencyNode{b, c},
	}

	g.AddNode(e)
	g.AddNode(d)
	g.AddNode(c)
	g.AddNode(b)
	g.AddNode(a)

	// Should have no cycles
	hasCycle, _ := g.HasCycle()
	if hasCycle {
		t.Error("Complex graph should not have cycles")
	}

	// Should be able to topologically sort
	sorted, err := g.TopologicalSort()
	if err != nil {
		t.Fatalf("TopologicalSort() error = %v", err)
	}

	if len(sorted) != 5 {
		t.Errorf("TopologicalSort() = %v nodes, want 5", len(sorted))
	}

	// d and e should come before b and c
	// b and c should come before a
	indices := make(map[string]int)
	for i, node := range sorted {
		indices[node.Module.Name] = i
	}

	if indices["d"] >= indices["b"] || indices["d"] >= indices["c"] {
		t.Error("d should come before b and c")
	}
	if indices["e"] >= indices["c"] {
		t.Error("e should come before c")
	}
	if indices["b"] >= indices["a"] || indices["c"] >= indices["a"] {
		t.Error("b and c should come before a")
	}
}
