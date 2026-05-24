// SPDX-License-Identifier: Apache-2.0

package link

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"go.keystone-core.io/keystone-core/internal/statemgmt"
)

func decl(name, state string, params map[string]any) *statemgmt.Declaration {
	return &statemgmt.Declaration{
		ID:     "link:" + name,
		Module: "link",
		State:  state,
		Name:   name,
		Params: params,
	}
}

// --- params / validate ------------------------------------------------

func TestParse_UnknownKey(t *testing.T) {
	t.Parallel()
	_, err := parseParams(decl("/x", StatePresent, map[string]any{"taget": "/y"}))
	if err == nil {
		t.Fatal("expected unknown-key error")
	}
}

func TestParse_TypeErrors(t *testing.T) {
	t.Parallel()
	cases := []map[string]any{
		{"target": 7},
		{"kind": true},
		{"force": "yes"},
	}
	for _, p := range cases {
		if _, err := parseParams(decl("/x", StatePresent, p)); err == nil {
			t.Errorf("params %v: expected type error", p)
		}
	}
}

func TestValidate(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name    string
		d       *statemgmt.Declaration
		wantErr bool
	}{
		{"present needs target", decl("/x", StatePresent, nil), true},
		{"present ok symlink", decl("/x", StatePresent, map[string]any{"target": "/y"}), false},
		{"present ok hard", decl("/x", StatePresent, map[string]any{"target": "/y", "kind": "hard"}), false},
		{"bad kind", decl("/x", StatePresent, map[string]any{"target": "/y", "kind": "soft"}), true},
		{"absent ok", decl("/x", StateAbsent, nil), false},
		{"absent rejects target", decl("/x", StateAbsent, map[string]any{"target": "/y"}), true},
		{"absent rejects kind", decl("/x", StateAbsent, map[string]any{"kind": "hard"}), true},
		{"bad state", decl("/x", "weird", nil), true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			p, err := parseParams(tc.d)
			if err == nil {
				err = p.validate()
			}
			if (err != nil) != tc.wantErr {
				t.Fatalf("err=%v, wantErr=%v", err, tc.wantErr)
			}
		})
	}
}

// --- Check -------------------------------------------------------------

func TestCheck_Absent(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	missing := filepath.Join(dir, "nope")
	m := New()
	res, err := m.Check(context.Background(), decl(missing, StateAbsent, nil))
	if err != nil {
		t.Fatal(err)
	}
	if !res.Matches {
		t.Error("missing path should match absent")
	}

	present := filepath.Join(dir, "f")
	if err := os.WriteFile(present, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	res, err = m.Check(context.Background(), decl(present, StateAbsent, nil))
	if err != nil {
		t.Fatal(err)
	}
	if res.Matches {
		t.Error("existing path should drift from absent")
	}
}

func TestCheck_PresentSymlink(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	target := filepath.Join(dir, "target")
	if err := os.WriteFile(target, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	linkPath := filepath.Join(dir, "ln")
	m := New()

	// missing → drift
	res, _ := m.Check(context.Background(), decl(linkPath, StatePresent, map[string]any{"target": target}))
	if res.Matches {
		t.Error("missing symlink should drift")
	}

	// right target → match
	if err := os.Symlink(target, linkPath); err != nil {
		t.Fatal(err)
	}
	res, err := m.Check(context.Background(), decl(linkPath, StatePresent, map[string]any{"target": target}))
	if err != nil {
		t.Fatal(err)
	}
	if !res.Matches {
		t.Errorf("correct symlink should match, diff=%q", res.Diff)
	}

	// wrong target → drift
	res, _ = m.Check(context.Background(), decl(linkPath, StatePresent, map[string]any{"target": "/elsewhere"}))
	if res.Matches {
		t.Error("wrong-target symlink should drift")
	}

	// path is a regular file → drift
	res, _ = m.Check(context.Background(), decl(target, StatePresent, map[string]any{"target": linkPath}))
	if res.Matches {
		t.Error("regular file should drift from symlink state")
	}
}

func TestCheck_PresentHard(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	target := filepath.Join(dir, "target")
	if err := os.WriteFile(target, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	hl := filepath.Join(dir, "hl")
	m := New()
	p := map[string]any{"target": target, "kind": "hard"}

	// missing → drift
	res, _ := m.Check(context.Background(), decl(hl, StatePresent, p))
	if res.Matches {
		t.Error("missing hard link should drift")
	}

	// same inode → match
	if err := os.Link(target, hl); err != nil {
		t.Fatal(err)
	}
	res, err := m.Check(context.Background(), decl(hl, StatePresent, p))
	if err != nil {
		t.Fatal(err)
	}
	if !res.Matches {
		t.Errorf("hard link should match, diff=%q", res.Diff)
	}

	// different regular file → drift
	other := filepath.Join(dir, "other")
	if err := os.WriteFile(other, []byte("y"), 0o644); err != nil {
		t.Fatal(err)
	}
	res, _ = m.Check(context.Background(), decl(other, StatePresent, p))
	if res.Matches {
		t.Error("unrelated regular file should drift from hard-link state")
	}

	// target missing → Check errors (cannot validate)
	bogus := map[string]any{"target": filepath.Join(dir, "ghost"), "kind": "hard"}
	if _, err := m.Check(context.Background(), decl(hl, StatePresent, bogus)); err == nil {
		t.Error("hard link to nonexistent target should error in Check")
	}
}

// --- Apply -------------------------------------------------------------

func TestApply_Symlink(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	target := filepath.Join(dir, "target")
	if err := os.WriteFile(target, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	linkPath := filepath.Join(dir, "ln")
	m := New()
	d := decl(linkPath, StatePresent, map[string]any{"target": target})

	r, err := m.Apply(context.Background(), d)
	if err != nil {
		t.Fatal(err)
	}
	if !r.Changed {
		t.Error("first apply should change")
	}
	got, err := os.Readlink(linkPath)
	if err != nil || got != target {
		t.Fatalf("readlink=%q,%v want %q", got, err, target)
	}

	// idempotent
	r, err = m.Apply(context.Background(), d)
	if err != nil {
		t.Fatal(err)
	}
	if r.Changed {
		t.Error("second apply should be no-op")
	}

	// wrong target → replaced
	d2 := decl(linkPath, StatePresent, map[string]any{"target": "/somewhere/else"})
	r, err = m.Apply(context.Background(), d2)
	if err != nil {
		t.Fatal(err)
	}
	if !r.Changed {
		t.Error("retarget should change")
	}
	if got, _ := os.Readlink(linkPath); got != "/somewhere/else" {
		t.Errorf("readlink=%q, want /somewhere/else", got)
	}
}

func TestApply_SymlinkOverRegularFile(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	target := filepath.Join(dir, "target")
	if err := os.WriteFile(target, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	occupied := filepath.Join(dir, "occupied")
	if err := os.WriteFile(occupied, []byte("data"), 0o644); err != nil {
		t.Fatal(err)
	}
	m := New()

	// without force → error
	if _, err := m.Apply(context.Background(), decl(occupied, StatePresent, map[string]any{"target": target})); err == nil {
		t.Fatal("expected error replacing a regular file without force")
	}
	// with force → ok
	r, err := m.Apply(context.Background(), decl(occupied, StatePresent, map[string]any{"target": target, "force": true}))
	if err != nil {
		t.Fatal(err)
	}
	if !r.Changed {
		t.Error("forced replace should change")
	}
	if got, _ := os.Readlink(occupied); got != target {
		t.Errorf("readlink=%q want %q", got, target)
	}
}

func TestApply_OverDirectoryAlwaysErrors(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	sub := filepath.Join(dir, "sub")
	if err := os.Mkdir(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	m := New()
	if _, err := m.Apply(context.Background(), decl(sub, StatePresent, map[string]any{"target": "/x", "force": true})); err == nil {
		t.Error("replacing a directory should always error, even with force")
	}
}

func TestApply_HardLink(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	target := filepath.Join(dir, "target")
	if err := os.WriteFile(target, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	hl := filepath.Join(dir, "hl")
	m := New()
	d := decl(hl, StatePresent, map[string]any{"target": target, "kind": "hard"})

	r, err := m.Apply(context.Background(), d)
	if err != nil {
		t.Fatal(err)
	}
	if !r.Changed {
		t.Error("first apply should change")
	}
	same, err := sameInode(hl, target)
	if err != nil || !same {
		t.Fatalf("hard link not established: same=%v err=%v", same, err)
	}

	// idempotent
	r, err = m.Apply(context.Background(), d)
	if err != nil {
		t.Fatal(err)
	}
	if r.Changed {
		t.Error("second apply should be no-op")
	}

	// over a different regular file: without force → error; with → ok
	other := filepath.Join(dir, "other")
	if err := os.WriteFile(other, []byte("y"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := m.Apply(context.Background(), decl(other, StatePresent, map[string]any{"target": target, "kind": "hard"})); err == nil {
		t.Fatal("expected error hard-linking over a different file without force")
	}
	if _, err := m.Apply(context.Background(), decl(other, StatePresent, map[string]any{"target": target, "kind": "hard", "force": true})); err != nil {
		t.Fatalf("forced hard-link replace: %v", err)
	}
	if same, _ := sameInode(other, target); !same {
		t.Error("forced replace did not establish the hard link")
	}

	// target missing → apply errors
	if _, err := m.Apply(context.Background(), decl(hl, StatePresent, map[string]any{"target": filepath.Join(dir, "ghost"), "kind": "hard"})); err == nil {
		t.Error("hard link to nonexistent target should error")
	}
}

func TestApply_Absent(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	m := New()

	// missing → no-op
	r, err := m.Apply(context.Background(), decl(filepath.Join(dir, "nope"), StateAbsent, nil))
	if err != nil {
		t.Fatal(err)
	}
	if r.Changed {
		t.Error("absent on a missing path should be a no-op")
	}

	// symlink → removed
	target := filepath.Join(dir, "t")
	if err := os.WriteFile(target, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	ln := filepath.Join(dir, "ln")
	if err := os.Symlink(target, ln); err != nil {
		t.Fatal(err)
	}
	r, err = m.Apply(context.Background(), decl(ln, StateAbsent, nil))
	if err != nil {
		t.Fatal(err)
	}
	if !r.Changed {
		t.Error("removing a symlink should change")
	}
	if _, err := os.Lstat(ln); !os.IsNotExist(err) {
		t.Error("symlink still present after absent apply")
	}

	// regular file (a hard link looks like one) → removed
	hl := filepath.Join(dir, "hl")
	if err := os.Link(target, hl); err != nil {
		t.Fatal(err)
	}
	if _, err := m.Apply(context.Background(), decl(hl, StateAbsent, nil)); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Lstat(hl); !os.IsNotExist(err) {
		t.Error("hard link still present after absent apply")
	}

	// directory → error
	sub := filepath.Join(dir, "sub")
	if err := os.Mkdir(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := m.Apply(context.Background(), decl(sub, StateAbsent, nil)); err == nil {
		t.Error("absent on a directory should error")
	}
}

func TestApply_IdempotentWhenCheckMatchesOnEntry(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	target := filepath.Join(dir, "t")
	if err := os.WriteFile(target, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	ln := filepath.Join(dir, "ln")
	if err := os.Symlink(target, ln); err != nil {
		t.Fatal(err)
	}
	m := New()
	r, err := m.Apply(context.Background(), decl(ln, StatePresent, map[string]any{"target": target}))
	if err != nil {
		t.Fatal(err)
	}
	if r.Changed {
		t.Error("apply on an already-converged link should report no change")
	}
	if r.Comment != "already converged" {
		t.Errorf("comment=%q", r.Comment)
	}
}

func TestValidate_ViaModule(t *testing.T) {
	t.Parallel()
	vm := New().(statemgmt.ValidatableModule)
	if err := vm.Validate(decl("/x", StatePresent, map[string]any{"target": "/y"})); err != nil {
		t.Errorf("valid decl rejected: %v", err)
	}
	if err := vm.Validate(decl("/x", StatePresent, nil)); err == nil {
		t.Error("present-without-target should be rejected")
	}
	if err := vm.Validate(decl("/x", StatePresent, map[string]any{"bogus": 1})); err == nil {
		t.Error("unknown param should be rejected")
	}
}

func TestApply_HardLinkOverSymlink_Force(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	target := filepath.Join(dir, "target")
	if err := os.WriteFile(target, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	p := filepath.Join(dir, "p")
	if err := os.Symlink(target, p); err != nil {
		t.Fatal(err)
	}
	m := New()
	// without force → error (it's a symlink, not a hard link)
	if _, err := m.Apply(context.Background(), decl(p, StatePresent, map[string]any{"target": target, "kind": "hard"})); err == nil {
		t.Fatal("expected error replacing a symlink with a hard link without force")
	}
	// with force → replaced by a real hard link
	if _, err := m.Apply(context.Background(), decl(p, StatePresent, map[string]any{"target": target, "kind": "hard", "force": true})); err != nil {
		t.Fatal(err)
	}
	if li, _ := inspect(p); li.Kind != kindRegular {
		t.Errorf("expected a regular file (hard link), got %s", li.Kind)
	}
	if same, _ := sameInode(p, target); !same {
		t.Error("hard link not established")
	}
}

func TestApply_SymlinkOverDir_NoForce(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	sub := filepath.Join(dir, "sub")
	if err := os.Mkdir(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := New().Apply(context.Background(), decl(sub, StatePresent, map[string]any{"target": "/x"})); err == nil {
		t.Error("symlink over a directory without force should error")
	}
}

// --- module surface ----------------------------------------------------

func TestModuleSurface(t *testing.T) {
	t.Parallel()
	m := New()
	if m.Name() != "link" {
		t.Errorf("Name=%q", m.Name())
	}
	if got := m.ValidStates(); len(got) != 2 || got[0] != StatePresent || got[1] != StateAbsent {
		t.Errorf("ValidStates=%v", got)
	}
	if _, ok := m.(statemgmt.ValidatableModule); !ok {
		t.Error("link should implement ValidatableModule")
	}
	dsm, ok := m.(statemgmt.DriftSeverityModule)
	if !ok {
		t.Fatal("link should implement DriftSeverityModule")
	}
	if dsm.DriftSeverity(decl("/x", StateAbsent, nil), nil) != statemgmt.DriftSeverityHigh {
		t.Error("absent drift should be HIGH")
	}
	if dsm.DriftSeverity(decl("/x", StatePresent, map[string]any{"target": "/y"}), nil) != statemgmt.DriftSeverityMedium {
		t.Error("present drift should be MEDIUM")
	}
	if dsm.DriftSeverity(nil, nil) != statemgmt.DriftSeverityMedium {
		t.Error("nil decl → MEDIUM")
	}

	ok2, err := m.Test(context.Background(), decl("/definitely/missing", StateAbsent, nil))
	if err != nil || !ok2 {
		t.Errorf("Test on absent-and-missing: ok=%v err=%v", ok2, err)
	}
}
