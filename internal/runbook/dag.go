package runbook

import (
	"errors"
	"fmt"
	"sort"
	"strings"
)

// ErrStepCycle is returned when the DependsOn graph contains a cycle.
// The error message includes the offending path (e.g. "a -> b -> a").
var ErrStepCycle = errors.New("runbook: step dependency cycle")

// CheckDAG validates the step dependency graph: it builds the
// topological order and reports ErrStepCycle (with the offending
// path) if the DependsOn graph contains a cycle. Used by
// `kscore-runbook test` for a static, non-executing check on top of
// Validate. Assumes Validate has confirmed DependsOn closure.
func (rb *Runbook) CheckDAG() error {
	_, err := resolveOrder(rb.Spec.Steps)
	return err
}

// resolveOrder returns step names in dependencies-first order: a step
// always appears after every step it DependsOn. Traversal is
// deterministic (sorted) so a given runbook always runs its steps in
// the same order. Assumes Validate has already confirmed DependsOn
// closure (every ref is a declared step).
func resolveOrder(steps []Step) ([]string, error) {
	deps := make(map[string][]string, len(steps))
	order := make([]string, 0, len(steps))
	for _, s := range steps {
		d := append([]string(nil), s.DependsOn...)
		sort.Strings(d)
		deps[s.Name] = d
	}

	const (
		white = 0
		gray  = 1
		black = 2
	)
	color := make(map[string]int, len(steps))
	var stack []string

	roots := make([]string, 0, len(steps))
	for _, s := range steps {
		roots = append(roots, s.Name)
	}
	sort.Strings(roots)

	var visit func(name string) error
	visit = func(name string) error {
		switch color[name] {
		case black:
			return nil
		case gray:
			start := 0
			for i, n := range stack {
				if n == name {
					start = i
					break
				}
			}
			cycle := append(append([]string{}, stack[start:]...), name)
			return fmt.Errorf("%w: %s", ErrStepCycle, strings.Join(cycle, " -> "))
		}
		color[name] = gray
		stack = append(stack, name)
		for _, d := range deps[name] {
			if err := visit(d); err != nil {
				return err
			}
		}
		stack = stack[:len(stack)-1]
		color[name] = black
		order = append(order, name)
		return nil
	}

	for _, r := range roots {
		if err := visit(r); err != nil {
			return nil, err
		}
	}
	return order, nil
}
