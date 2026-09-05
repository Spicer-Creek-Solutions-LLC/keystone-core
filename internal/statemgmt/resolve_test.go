// SPDX-License-Identifier: Apache-2.0

package statemgmt

import (
	"errors"
	"strings"
	"testing"
)

func decl(module, name string, params map[string]any) *Declaration {
	return &Declaration{
		ID:     module + ":" + name,
		Module: module,
		Name:   name,
		State:  "present",
		Params: params,
	}
}

func req(targetModule, targetName string) []any {
	return []any{map[string]any{targetModule: targetName}}
}

func ids(decls []*Declaration) []string {
	out := make([]string, len(decls))
	for i, d := range decls {
		out[i] = d.ID
	}
	return out
}

func mustResolve(t *testing.T, sf *StateFile) []*Declaration {
	t.Helper()
	out, err := NewResolver().Resolve(sf)
	if err != nil {
		t.Fatalf("Resolve: unexpected error: %v", err)
	}
	return out
}

func assertOrder(t *testing.T, got []*Declaration, want []string) {
	t.Helper()
	gotIDs := ids(got)
	if len(gotIDs) != len(want) {
		t.Fatalf("order len = %d, want %d (got %v)", len(gotIDs), len(want), gotIDs)
	}
	for i := range want {
		if gotIDs[i] != want[i] {
			t.Errorf("[%d] = %q, want %q (full: %v)", i, gotIDs[i], want[i], gotIDs)
		}
	}
}

func TestResolver_Nil(t *testing.T) {
	t.Parallel()
	out, err := NewResolver().Resolve(nil)
	if err != nil {
		t.Fatalf("Resolve(nil): %v", err)
	}
	if out != nil {
		t.Errorf("Resolve(nil) = %v, want nil", out)
	}
}

func TestResolver_Empty(t *testing.T) {
	t.Parallel()
	out := mustResolve(t, &StateFile{})
	if out != nil {
		t.Errorf("Resolve(empty) = %v, want nil", out)
	}
}

func TestResolver_NoRequisites_PreservesSourceOrder(t *testing.T) {
	t.Parallel()
	sf := &StateFile{Declarations: []*Declaration{
		decl("file", "/a", nil),
		decl("file", "/b", nil),
		decl("file", "/c", nil),
	}}
	out := mustResolve(t, sf)
	assertOrder(t, out, []string{"file:/a", "file:/b", "file:/c"})
}

func TestResolver_LinearChain_Require(t *testing.T) {
	t.Parallel()
	// A requires B requires C → expect [C, B, A].
	sf := &StateFile{Declarations: []*Declaration{
		decl("file", "a", map[string]any{"require": req("file", "b")}),
		decl("file", "b", map[string]any{"require": req("file", "c")}),
		decl("file", "c", nil),
	}}
	out := mustResolve(t, sf)
	assertOrder(t, out, []string{"file:c", "file:b", "file:a"})
}

func TestResolver_LinearChain_RequireIn(t *testing.T) {
	t.Parallel()
	// A require_in B → A before B → expect [A, B].
	sf := &StateFile{Declarations: []*Declaration{
		decl("file", "a", map[string]any{"require_in": req("file", "b")}),
		decl("file", "b", nil),
	}}
	out := mustResolve(t, sf)
	assertOrder(t, out, []string{"file:a", "file:b"})
}

func TestResolver_MixedRequireAndRequireIn(t *testing.T) {
	t.Parallel()
	// A require B; A require_in C → B before A before C → [B, A, C].
	sf := &StateFile{Declarations: []*Declaration{
		decl("file", "a", map[string]any{
			"require":    req("file", "b"),
			"require_in": req("file", "c"),
		}),
		decl("file", "b", nil),
		decl("file", "c", nil),
	}}
	out := mustResolve(t, sf)
	assertOrder(t, out, []string{"file:b", "file:a", "file:c"})
}

func TestResolver_Diamond(t *testing.T) {
	t.Parallel()
	// A requires {B, C}; B requires D; C requires D → D, then B/C
	// in source order, then A.
	sf := &StateFile{Declarations: []*Declaration{
		decl("file", "a", map[string]any{"require": []any{
			map[string]any{"file": "b"},
			map[string]any{"file": "c"},
		}}),
		decl("file", "b", map[string]any{"require": req("file", "d")}),
		decl("file", "c", map[string]any{"require": req("file", "d")}),
		decl("file", "d", nil),
	}}
	out := mustResolve(t, sf)
	assertOrder(t, out, []string{"file:d", "file:b", "file:c", "file:a"})
}

func TestResolver_SourceOrderTiebreak(t *testing.T) {
	t.Parallel()
	// C is required by both A and B. Source order is [C, A, B].
	// Tiebreak among independent ready nodes should preserve that.
	sf := &StateFile{Declarations: []*Declaration{
		decl("file", "c", nil),
		decl("file", "a", map[string]any{"require": req("file", "c")}),
		decl("file", "b", map[string]any{"require": req("file", "c")}),
	}}
	out := mustResolve(t, sf)
	assertOrder(t, out, []string{"file:c", "file:a", "file:b"})
}

func TestResolver_DeterministicAcrossRuns(t *testing.T) {
	t.Parallel()
	// Map iteration order would scramble adjacency lists; we sort
	// internally. Run the same resolve 50 times and assert the
	// order is identical every time.
	build := func() *StateFile {
		return &StateFile{Declarations: []*Declaration{
			decl("file", "root", map[string]any{"require": []any{
				map[string]any{"file": "x"},
				map[string]any{"file": "y"},
				map[string]any{"file": "z"},
			}}),
			decl("file", "x", nil),
			decl("file", "y", nil),
			decl("file", "z", nil),
		}}
	}
	first := ids(mustResolve(t, build()))
	for i := 0; i < 50; i++ {
		got := ids(mustResolve(t, build()))
		if len(got) != len(first) {
			t.Fatalf("iteration %d: len mismatch", i)
		}
		for j := range first {
			if got[j] != first[j] {
				t.Errorf("iteration %d non-deterministic at [%d]: %v vs %v", i, j, got, first)
				return
			}
		}
	}
}

func TestResolver_AllRequisiteKeys(t *testing.T) {
	t.Parallel()
	// Y has key:[X]; expect ordering per the uniform direction rule.
	for _, key := range RequisiteKeys {
		t.Run(key, func(t *testing.T) {
			t.Parallel()
			sf := &StateFile{Declarations: []*Declaration{
				decl("file", "y", map[string]any{key: req("file", "x")}),
				decl("file", "x", nil),
			}}
			out := mustResolve(t, sf)
			var want []string
			if strings.HasSuffix(key, "_in") {
				want = []string{"file:y", "file:x"} // Y first
			} else {
				want = []string{"file:x", "file:y"} // X first
			}
			assertOrder(t, out, want)
		})
	}
}

func TestResolver_Cycle_Self(t *testing.T) {
	t.Parallel()
	sf := &StateFile{Declarations: []*Declaration{
		decl("file", "a", map[string]any{"require": req("file", "a")}),
	}}
	_, err := NewResolver().Resolve(sf)
	if err == nil {
		t.Fatal("expected CycleError")
	}
	var ce *CycleError
	if !errors.As(err, &ce) {
		t.Fatalf("err = %T (%v), want *CycleError", err, err)
	}
	if len(ce.Path) < 2 || ce.Path[0] != ce.Path[len(ce.Path)-1] {
		t.Errorf("cycle Path = %v, must start and end at the same ID", ce.Path)
	}
	if ce.Path[0] != "file:a" {
		t.Errorf("cycle Path = %v, want starts at file:a", ce.Path)
	}
}

func TestResolver_Cycle_TwoStep(t *testing.T) {
	t.Parallel()
	sf := &StateFile{Declarations: []*Declaration{
		decl("file", "a", map[string]any{"require": req("file", "b")}),
		decl("file", "b", map[string]any{"require": req("file", "a")}),
	}}
	_, err := NewResolver().Resolve(sf)
	var ce *CycleError
	if !errors.As(err, &ce) {
		t.Fatalf("err = %T (%v), want *CycleError", err, err)
	}
	// The cycle path must contain both IDs and be closed.
	want := map[string]bool{"file:a": false, "file:b": false}
	for _, id := range ce.Path {
		if _, ok := want[id]; ok {
			want[id] = true
		}
	}
	for id, hit := range want {
		if !hit {
			t.Errorf("cycle Path = %v, missing %q", ce.Path, id)
		}
	}
	if ce.Path[0] != ce.Path[len(ce.Path)-1] {
		t.Errorf("cycle Path = %v, must be closed", ce.Path)
	}
}

func TestResolver_Cycle_ThreeStep(t *testing.T) {
	t.Parallel()
	sf := &StateFile{Declarations: []*Declaration{
		decl("file", "a", map[string]any{"require": req("file", "b")}),
		decl("file", "b", map[string]any{"require": req("file", "c")}),
		decl("file", "c", map[string]any{"require": req("file", "a")}),
	}}
	_, err := NewResolver().Resolve(sf)
	var ce *CycleError
	if !errors.As(err, &ce) {
		t.Fatalf("err = %T (%v), want *CycleError", err, err)
	}
	if len(ce.Path) < 4 {
		t.Errorf("cycle Path = %v, want at least 4 nodes (3 distinct + closing)", ce.Path)
	}
}

func TestResolver_Cycle_MixedRequisiteTypes(t *testing.T) {
	t.Parallel()
	// A on its own creates the cycle: require:[B] adds B→A; the
	// same declaration's watch_in:[B] adds A→B. The two edges form
	// A ↔ B.
	sf := &StateFile{Declarations: []*Declaration{
		decl("file", "a", map[string]any{
			"require":  req("file", "b"),
			"watch_in": req("file", "b"),
		}),
		decl("file", "b", nil),
	}}
	_, err := NewResolver().Resolve(sf)
	var ce *CycleError
	if !errors.As(err, &ce) {
		t.Fatalf("err = %T, want *CycleError; err = %v", err, err)
	}
	// Make sure Error() formats with arrow separators.
	if !strings.Contains(ce.Error(), " → ") {
		t.Errorf("CycleError.Error() = %q, want arrow-formatted", ce.Error())
	}
	if !strings.HasPrefix(ce.Error(), "statemgmt: resolve: cycle:") {
		t.Errorf("CycleError.Error() = %q, want \"statemgmt: resolve: cycle:\" prefix", ce.Error())
	}
}

func TestResolver_DanglingRefIsDropped(t *testing.T) {
	t.Parallel()
	// "file:absent" doesn't exist in the StateFile. Resolver
	// silently drops the edge (Validator catches dangling refs).
	sf := &StateFile{Declarations: []*Declaration{
		decl("file", "a", map[string]any{"require": req("file", "absent")}),
		decl("file", "b", nil),
	}}
	out := mustResolve(t, sf)
	assertOrder(t, out, []string{"file:a", "file:b"})
}

func TestResolver_MalformedRequisiteShape_Dropped(t *testing.T) {
	t.Parallel()
	// Malformed: require is a string, not a list. Validator caught
	// it; resolver silently skips and yields source order.
	sf := &StateFile{Declarations: []*Declaration{
		decl("file", "a", map[string]any{"require": "not-a-list"}),
		decl("file", "b", nil),
	}}
	out := mustResolve(t, sf)
	assertOrder(t, out, []string{"file:a", "file:b"})
}

func TestResolver_NilDeclarationsSkipped(t *testing.T) {
	t.Parallel()
	sf := &StateFile{Declarations: []*Declaration{
		nil,
		decl("file", "a", nil),
		nil,
		decl("file", "b", map[string]any{"require": req("file", "a")}),
	}}
	out := mustResolve(t, sf)
	assertOrder(t, out, []string{"file:a", "file:b"})
}

func TestResolver_DuplicateIDsTolerated(t *testing.T) {
	t.Parallel()
	// Validator catches duplicates; if one slips through, the
	// resolver keeps the first occurrence and yields a usable order
	// rather than crashing.
	dup := decl("file", "a", nil)
	sf := &StateFile{Declarations: []*Declaration{dup, dup}}
	_, err := NewResolver().Resolve(sf)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
}

func TestCycleError_Error_Formatting(t *testing.T) {
	t.Parallel()
	ce := &CycleError{Path: []string{"file:a", "file:b", "file:a"}}
	want := "statemgmt: resolve: cycle: file:a → file:b → file:a"
	if got := ce.Error(); got != want {
		t.Errorf("Error() = %q, want %q", got, want)
	}
}

// Bigger DAG smoke: 10 declarations, several layers, verify the
// result is a valid topological order (every dep precedes its
// dependent).
func TestResolver_LargerGraph_ValidTopo(t *testing.T) {
	t.Parallel()
	sf := &StateFile{Declarations: []*Declaration{
		decl("file", "n1", nil),
		decl("file", "n2", map[string]any{"require": req("file", "n1")}),
		decl("file", "n3", map[string]any{"require": req("file", "n1")}),
		decl("file", "n4", map[string]any{"require": []any{
			map[string]any{"file": "n2"},
			map[string]any{"file": "n3"},
		}}),
		decl("file", "n5", map[string]any{"require": req("file", "n4")}),
		decl("file", "n6", map[string]any{"watch": req("file", "n4")}),
		decl("file", "n7", map[string]any{"require_in": req("file", "n8")}),
		decl("file", "n8", nil),
		decl("file", "n9", map[string]any{"onchanges": req("file", "n5")}),
		decl("file", "n10", nil),
	}}
	out := mustResolve(t, sf)
	if len(out) != 10 {
		t.Fatalf("result has %d nodes, want 10", len(out))
	}
	// Build position index and verify each edge's From < To.
	pos := map[string]int{}
	for i, d := range out {
		pos[d.ID] = i
	}
	// We don't know all edges symbolically; assert a few critical ones.
	mustBefore := [][2]string{
		{"file:n1", "file:n2"},
		{"file:n1", "file:n3"},
		{"file:n2", "file:n4"},
		{"file:n3", "file:n4"},
		{"file:n4", "file:n5"},
		{"file:n4", "file:n6"}, // watch ordering
		{"file:n7", "file:n8"}, // require_in
		{"file:n5", "file:n9"}, // onchanges
	}
	for _, ab := range mustBefore {
		if pos[ab[0]] >= pos[ab[1]] {
			t.Errorf("ordering violation: %s should come before %s; got positions %d and %d (full order: %v)",
				ab[0], ab[1], pos[ab[0]], pos[ab[1]], ids(out))
		}
	}
}
