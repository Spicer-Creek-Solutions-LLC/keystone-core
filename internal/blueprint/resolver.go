// SPDX-License-Identifier: Apache-2.0

package blueprint

import (
	"errors"
	"fmt"
	"sort"
	"strings"
)

// ErrMissingDependency is returned by Graph.Resolve when a hard
// `requires` edge points at a blueprint not in the graph.
var ErrMissingDependency = errors.New("blueprint: missing required dependency")

// ErrDependencyCycle is returned by Graph.Resolve when the dependency
// graph contains a cycle. The error message includes the cycle path.
var ErrDependencyCycle = errors.New("blueprint: dependency cycle")

// Graph is a set of blueprints resolved together. Edges come from
// each manifest's Dependencies: `requires` is a hard edge (the target
// must be present and is ordered before the dependent) and
// `requires_before` is a soft ordering edge (honoured only if the
// target is present; its absence is not an error).
type Graph struct {
	nodes map[string]*Manifest
}

// NewGraph returns an empty graph.
func NewGraph() *Graph {
	return &Graph{nodes: make(map[string]*Manifest)}
}

// Add registers a manifest under its metadata name. Re-adding the
// same name overwrites.
func (g *Graph) Add(m *Manifest) {
	g.nodes[m.Metadata.Name] = m
}

// Resolve returns the blueprint names in dependencies-first order: a
// blueprint always appears after every blueprint it requires or
// requires-before. Missing hard dependencies yield
// ErrMissingDependency; a cycle yields ErrDependencyCycle with the
// offending path (e.g. "a -> b -> c -> a").
func (g *Graph) Resolve() ([]string, error) {
	// Hard requires must resolve to present nodes.
	var missing []string
	for name, m := range g.nodes {
		for _, dep := range m.Dependencies.Requires {
			if _, ok := g.nodes[dep]; !ok {
				missing = append(missing, fmt.Sprintf("%s requires %s", name, dep))
			}
		}
	}
	if len(missing) > 0 {
		sort.Strings(missing)
		return nil, fmt.Errorf("%w: %s", ErrMissingDependency, strings.Join(missing, "; "))
	}

	const (
		white = 0 // unvisited
		gray  = 1 // on the current DFS stack
		black = 2 // fully processed
	)
	color := make(map[string]int, len(g.nodes))
	var order []string
	var stack []string

	// Deterministic traversal: visit roots in sorted order.
	roots := make([]string, 0, len(g.nodes))
	for name := range g.nodes {
		roots = append(roots, name)
	}
	sort.Strings(roots)

	var visit func(name string) error
	visit = func(name string) error {
		switch color[name] {
		case black:
			return nil
		case gray:
			// Cycle: slice the stack from the first occurrence.
			start := 0
			for i, n := range stack {
				if n == name {
					start = i
					break
				}
			}
			cycle := append(append([]string{}, stack[start:]...), name)
			return fmt.Errorf("%w: %s", ErrDependencyCycle, strings.Join(cycle, " -> "))
		}

		color[name] = gray
		stack = append(stack, name)

		// Edges = hard requires + soft requires_before that are
		// present in the graph.
		m := g.nodes[name]
		deps := append([]string{}, m.Dependencies.Requires...)
		for _, d := range m.Dependencies.RequiresBefore {
			if _, ok := g.nodes[d]; ok {
				deps = append(deps, d)
			}
		}
		sort.Strings(deps)
		for _, d := range deps {
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
