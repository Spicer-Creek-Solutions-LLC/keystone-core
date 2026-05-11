package statemgmt

import (
	"sort"
	"strings"
)

// Resolver builds the requisite DAG, detects cycles, and topologically
// orders a StateFile's Declarations for the runner (Task 6).
//
// Edge direction is uniform across all eight requisite keys:
//
//	<key>: [B]    on A   ⇒  B before A   ⇒  edge B → A
//	<key>_in: [B] on A   ⇒  A before B   ⇒  edge A → B
//
// This deliberately deviates from Salt's prereq semantics (Salt's
// "prereq: [B] on A" reads "A is prerequisite for B" — A first). One
// rule for all eight keys is easier to teach than per-key direction
// tables; if a real use case demands Salt-faithful prereq we'll add a
// per-key direction policy in v1.x (tracked in V1X-BACKLOG).
//
// Runtime semantics of watch / onchanges / prereq (extra-handler
// triggers, conditional firing) are runner concerns. The Resolver
// only computes ordering.
//
// Resolve assumes the StateFile has been validated. Dangling refs and
// malformed requisite shapes — both caught by the Validator — are
// silently dropped here; re-validating would duplicate work and slow
// the hot path.
type Resolver struct{}

// NewResolver returns a Resolver. v1.0 has no configuration knobs;
// the struct exists so callers can wire options later without a
// signature break.
func NewResolver() *Resolver {
	return &Resolver{}
}

// CycleError is returned by Resolve when the requisite graph contains
// a cycle. Path is the chain of declaration IDs forming the cycle;
// the first and last entries are equal so it prints as a closed loop.
type CycleError struct {
	Path []string
}

func (e *CycleError) Error() string {
	return "statemgmt: resolve: cycle: " + strings.Join(e.Path, " → ")
}

// Resolve returns sf.Declarations in topological execution order.
func (r *Resolver) Resolve(sf *StateFile) ([]*Declaration, error) {
	if sf == nil || len(sf.Declarations) == 0 {
		return nil, nil
	}

	// Index by ID + remember source order. Source order is the
	// stable tiebreak for Kahn's ready set so the output is
	// deterministic.
	idIndex := make(map[string]int, len(sf.Declarations))
	for i, d := range sf.Declarations {
		if d == nil {
			continue
		}
		// Validator catches duplicates; if one slips through we
		// keep the first occurrence so source-order tiebreaks
		// stay stable.
		if _, dup := idIndex[d.ID]; !dup {
			idIndex[d.ID] = i
		}
	}

	edges := extractEdges(sf, idIndex)

	// DFS cycle check first so a clean cycle path comes out before
	// Kahn's strips edges.
	if cyc := detectCycle(sf.Declarations, idIndex, edges); cyc != nil {
		return nil, cyc
	}

	return topoSort(sf.Declarations, idIndex, edges), nil
}

// edge expresses "From must execute before To".
type edge struct {
	From string
	To   string
}

// extractEdges walks every Declaration's requisite Params and produces
// directed edges. Edges whose endpoints don't exist in idIndex are
// silently dropped — Validator catches dangling refs, and dropping
// here keeps the resolver's hot path simple.
func extractEdges(sf *StateFile, idIndex map[string]int) []edge {
	var edges []edge
	for _, decl := range sf.Declarations {
		if decl == nil {
			continue
		}
		for _, key := range RequisiteKeys {
			raw, ok := decl.Params[key]
			if !ok {
				continue
			}
			refs, _ := parseRequisiteList(raw) // shape errors are Validator's job
			reverse := strings.HasSuffix(key, "_in")
			for _, target := range refs {
				if _, exists := idIndex[target]; !exists {
					continue
				}
				if reverse {
					edges = append(edges, edge{From: decl.ID, To: target})
				} else {
					edges = append(edges, edge{From: target, To: decl.ID})
				}
			}
		}
	}
	return edges
}

// dfsFrame is a single iterative-DFS stack frame.
type dfsFrame struct {
	node string
	idx  int // next neighbour to visit
}

const (
	colorWhite = 0
	colorGray  = 1
	colorBlack = 2
)

// detectCycle runs DFS with white/gray/black coloring and returns the
// first cycle as a *CycleError. Path starts and ends at the same ID.
func detectCycle(decls []*Declaration, idIndex map[string]int, edges []edge) *CycleError {
	color := make(map[string]int, len(decls))
	for id := range idIndex {
		color[id] = colorWhite
	}

	// Outgoing adjacency: id → []targets, sorted by source order so
	// the DFS walk is deterministic and the cycle we report is
	// stable across runs.
	out := make(map[string][]string, len(decls))
	for _, e := range edges {
		out[e.From] = append(out[e.From], e.To)
	}
	for id, targets := range out {
		sort.SliceStable(targets, func(i, j int) bool {
			return idIndex[targets[i]] < idIndex[targets[j]]
		})
		out[id] = targets
	}

	// Iterative DFS so deep requisite chains don't risk Go's
	// goroutine stack-growth costs on recursive calls.
	stack := make([]dfsFrame, 0, len(decls))
	for _, d := range decls {
		if d == nil || color[d.ID] != colorWhite {
			continue
		}
		stack = append(stack[:0], dfsFrame{node: d.ID})
		color[d.ID] = colorGray
		for len(stack) > 0 {
			top := &stack[len(stack)-1]
			neighbours := out[top.node]
			if top.idx >= len(neighbours) {
				color[top.node] = colorBlack
				stack = stack[:len(stack)-1]
				continue
			}
			next := neighbours[top.idx]
			top.idx++
			switch color[next] {
			case colorWhite:
				color[next] = colorGray
				stack = append(stack, dfsFrame{node: next})
			case colorGray:
				return cycleFromStack(stack, next)
			case colorBlack:
				// already settled, no cycle reachable here
			}
		}
	}
	return nil
}

// cycleFromStack extracts the cycle path from the current DFS stack.
// `repeated` is the gray node we just re-encountered; we slice from
// its first occurrence in the stack and append it again to close the
// loop.
func cycleFromStack(stack []dfsFrame, repeated string) *CycleError {
	start := 0
	for i, f := range stack {
		if f.node == repeated {
			start = i
			break
		}
	}
	path := make([]string, 0, len(stack)-start+1)
	for _, f := range stack[start:] {
		path = append(path, f.node)
	}
	path = append(path, repeated)
	return &CycleError{Path: path}
}

// topoSort runs Kahn's algorithm. The ready set is processed in
// source-file order each round so output is deterministic.
func topoSort(decls []*Declaration, idIndex map[string]int, edges []edge) []*Declaration {
	inDegree := make(map[string]int, len(decls))
	adj := make(map[string][]string, len(decls))
	for id := range idIndex {
		inDegree[id] = 0
	}
	for _, e := range edges {
		adj[e.From] = append(adj[e.From], e.To)
		inDegree[e.To]++
	}

	// Sort adjacency lists once by source order so neighbours
	// enter the ready set in a stable order.
	for id, targets := range adj {
		sort.SliceStable(targets, func(i, j int) bool {
			return idIndex[targets[i]] < idIndex[targets[j]]
		})
		adj[id] = targets
	}

	// Initial ready set built in source-traversal order keeps the
	// stable tiebreak without a separate sort.
	queue := make([]*Declaration, 0, len(decls))
	for _, d := range decls {
		if d == nil {
			continue
		}
		if inDegree[d.ID] == 0 {
			queue = append(queue, d)
		}
	}

	out := make([]*Declaration, 0, len(decls))
	for head := 0; head < len(queue); head++ {
		d := queue[head]
		out = append(out, d)
		for _, to := range adj[d.ID] {
			inDegree[to]--
			if inDegree[to] == 0 {
				idx, ok := idIndex[to]
				if !ok {
					continue
				}
				queue = append(queue, decls[idx])
			}
		}
	}
	return out
}
