package runbook

import (
	"errors"
	"strings"
	"testing"
)

func idx(order []string, name string) int {
	for i, n := range order {
		if n == name {
			return i
		}
	}
	return -1
}

func TestResolveOrder(t *testing.T) {
	steps := []Step{
		{Name: "c", DependsOn: []string{"b"}},
		{Name: "b", DependsOn: []string{"a"}},
		{Name: "a"},
		{Name: "d"},
	}
	order, err := resolveOrder(steps)
	if err != nil {
		t.Fatalf("resolveOrder: %v", err)
	}
	if idx(order, "a") > idx(order, "b") || idx(order, "b") > idx(order, "c") {
		t.Fatalf("dependency order broken: %v", order)
	}
	if len(order) != 4 {
		t.Fatalf("order=%v", order)
	}
}

func TestResolveOrder_Cycle(t *testing.T) {
	steps := []Step{
		{Name: "a", DependsOn: []string{"c"}},
		{Name: "b", DependsOn: []string{"a"}},
		{Name: "c", DependsOn: []string{"b"}},
	}
	_, err := resolveOrder(steps)
	if !errors.Is(err, ErrStepCycle) {
		t.Fatalf("err=%v want ErrStepCycle", err)
	}
	if !strings.Contains(err.Error(), "->") {
		t.Fatalf("cycle path missing: %v", err)
	}
}

func TestResolveOrder_Deterministic(t *testing.T) {
	steps := []Step{{Name: "x"}, {Name: "y"}, {Name: "z"}}
	first, _ := resolveOrder(steps)
	for i := 0; i < 5; i++ {
		got, _ := resolveOrder(steps)
		for j := range first {
			if got[j] != first[j] {
				t.Fatalf("non-deterministic: %v vs %v", first, got)
			}
		}
	}
}
