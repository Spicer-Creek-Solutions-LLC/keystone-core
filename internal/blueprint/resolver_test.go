package blueprint

import (
	"errors"
	"strings"
	"testing"
)

func bp(name string, requires, requiresBefore []string) *Manifest {
	return &Manifest{
		Metadata:     Metadata{Name: name, Version: "1.0.0"},
		Entrypoints:  Entrypoints{Default: "x"},
		Dependencies: Dependencies{Requires: requires, RequiresBefore: requiresBefore},
	}
}

// indexOf returns the position of name in order, or -1.
func indexOf(order []string, name string) int {
	for i, n := range order {
		if n == name {
			return i
		}
	}
	return -1
}

func TestGraphResolve_Order(t *testing.T) {
	g := NewGraph()
	g.Add(bp("app", []string{"db"}, []string{"net"}))
	g.Add(bp("db", []string{"base"}, nil))
	g.Add(bp("base", nil, nil))
	g.Add(bp("net", nil, nil))

	order, err := g.Resolve()
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if len(order) != 4 {
		t.Fatalf("order=%v", order)
	}
	// Hard + soft edges both ordered before the dependent.
	if indexOf(order, "base") > indexOf(order, "db") {
		t.Errorf("base must precede db: %v", order)
	}
	if indexOf(order, "db") > indexOf(order, "app") {
		t.Errorf("db must precede app: %v", order)
	}
	if indexOf(order, "net") > indexOf(order, "app") {
		t.Errorf("net (requires_before) must precede app: %v", order)
	}
}

func TestGraphResolve_SoftMissingIgnored(t *testing.T) {
	g := NewGraph()
	g.Add(bp("app", nil, []string{"absent"}))
	order, err := g.Resolve()
	if err != nil {
		t.Fatalf("soft missing should not error: %v", err)
	}
	if len(order) != 1 || order[0] != "app" {
		t.Fatalf("order=%v", order)
	}
}

func TestGraphResolve_HardMissing(t *testing.T) {
	g := NewGraph()
	g.Add(bp("app", []string{"absent"}, nil))
	_, err := g.Resolve()
	if !errors.Is(err, ErrMissingDependency) {
		t.Fatalf("err=%v want ErrMissingDependency", err)
	}
	if !strings.Contains(err.Error(), "app requires absent") {
		t.Errorf("err=%q missing detail", err)
	}
}

func TestGraphResolve_Cycle(t *testing.T) {
	g := NewGraph()
	g.Add(bp("a", []string{"b"}, nil))
	g.Add(bp("b", []string{"c"}, nil))
	g.Add(bp("c", []string{"a"}, nil))

	_, err := g.Resolve()
	if !errors.Is(err, ErrDependencyCycle) {
		t.Fatalf("err=%v want ErrDependencyCycle", err)
	}
	// Path is present and closes the loop.
	msg := err.Error()
	if !strings.Contains(msg, "->") || !strings.Contains(msg, "a -> b -> c -> a") {
		t.Errorf("cycle path missing/!closed: %q", msg)
	}
}

func TestGraphResolve_SelfCycle(t *testing.T) {
	g := NewGraph()
	g.Add(bp("solo", []string{"solo"}, nil))
	_, err := g.Resolve()
	if !errors.Is(err, ErrDependencyCycle) {
		t.Fatalf("err=%v want ErrDependencyCycle", err)
	}
	if !strings.Contains(err.Error(), "solo -> solo") {
		t.Errorf("err=%q want self-cycle path", err)
	}
}

func TestGraphResolve_Deterministic(t *testing.T) {
	build := func() []string {
		g := NewGraph()
		g.Add(bp("a", nil, nil))
		g.Add(bp("b", nil, nil))
		g.Add(bp("c", nil, nil))
		o, err := g.Resolve()
		if err != nil {
			t.Fatal(err)
		}
		return o
	}
	first := build()
	for i := 0; i < 5; i++ {
		got := build()
		for j := range first {
			if got[j] != first[j] {
				t.Fatalf("non-deterministic order: %v vs %v", first, got)
			}
		}
	}
}
