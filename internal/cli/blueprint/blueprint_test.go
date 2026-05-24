// SPDX-License-Identifier: Apache-2.0

package blueprint_test

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	bp "go.keystone-core.io/keystone-core/internal/blueprint"
	clibp "go.keystone-core.io/keystone-core/internal/cli/blueprint"
	"go.keystone-core.io/keystone-core/internal/statemgmt"
)

type fakeSR struct{ decls int }

func (f *fakeSR) Run(_ context.Context, d []*statemgmt.Declaration) (*statemgmt.RunReport, error) {
	f.decls = len(d)
	return &statemgmt.RunReport{Total: len(d)}, nil
}

// runCLI executes the kscore-blueprint command with args, returning
// stdout, stderr, and the error.
func runCLI(d clibp.Deps, args ...string) (string, string, error) {
	c := clibp.NewCommand(d)
	var out, errb bytes.Buffer
	c.SetArgs(args)
	c.SetOut(&out)
	c.SetErr(&errb)
	err := c.Execute()
	return out.String(), errb.String(), err
}

func TestInitValidateInfoLintBundle(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "bp")

	if out, _, err := runCLI(clibp.Deps{}, "init", dir, "--name", "demo"); err != nil {
		t.Fatalf("init: %v (%s)", err, out)
	}
	if _, err := os.Stat(filepath.Join(dir, "blueprint.yaml")); err != nil {
		t.Fatalf("scaffold missing: %v", err)
	}

	out, _, err := runCLI(clibp.Deps{}, "validate", dir)
	if err != nil || !strings.Contains(out, "ok: demo@0.1.0") {
		t.Fatalf("validate: out=%q err=%v", out, err)
	}

	out, _, err = runCLI(clibp.Deps{}, "info", dir)
	if err != nil || !strings.Contains(out, "name:        demo") {
		t.Fatalf("info: out=%q err=%v", out, err)
	}

	out, _, err = runCLI(clibp.Deps{}, "lint", dir)
	if err != nil || !strings.Contains(out, "lint ok") {
		t.Fatalf("lint: out=%q err=%v", out, err)
	}

	// Break an entrypoint reference → lint fails.
	bad := filepath.Join(dir, "blueprint.yaml")
	body, _ := os.ReadFile(bad)
	_ = os.WriteFile(bad, bytes.Replace(body, []byte("default: apply.yaml"), []byte("default: missing.yaml"), 1), 0o600)
	if _, _, err = runCLI(clibp.Deps{}, "lint", dir); err == nil {
		t.Fatal("lint should fail for a missing entrypoint file")
	}
	_ = os.WriteFile(bad, body, 0o600) // restore

	outPath := filepath.Join(t.TempDir(), "demo.tgz")
	if _, _, err = runCLI(clibp.Deps{}, "bundle", dir, "-o", outPath); err != nil {
		t.Fatalf("bundle: %v", err)
	}
	if st, err := os.Stat(outPath); err != nil || st.Size() == 0 {
		t.Fatalf("bundle output missing/empty: %v", err)
	}
}

func TestValidateBadManifest(t *testing.T) {
	dir := t.TempDir()
	_ = os.WriteFile(filepath.Join(dir, "blueprint.yaml"), []byte("metadata:\n  name: Bad Name\n"), 0o600)
	if _, _, err := runCLI(clibp.Deps{}, "validate", dir); err == nil {
		t.Fatal("expected validation error")
	}
}

func TestInstallRemove(t *testing.T) {
	src := filepath.Join(t.TempDir(), "src")
	if _, _, err := runCLI(clibp.Deps{}, "init", src, "--name", "lib-bp"); err != nil {
		t.Fatal(err)
	}
	lib := t.TempDir()

	if _, _, err := runCLI(clibp.Deps{}, "install", src, "--library", lib); err != nil {
		t.Fatalf("install: %v", err)
	}
	if _, err := os.Stat(filepath.Join(lib, "lib-bp", "blueprint.yaml")); err != nil {
		t.Fatalf("not installed: %v", err)
	}
	// install again without update → error.
	if _, _, err := runCLI(clibp.Deps{}, "install", src, "--library", lib); err == nil {
		t.Fatal("re-install should require update")
	}
	if _, _, err := runCLI(clibp.Deps{}, "update", src, "--library", lib); err != nil {
		t.Fatalf("update: %v", err)
	}
	if _, _, err := runCLI(clibp.Deps{}, "remove", "lib-bp", "--library", lib); err != nil {
		t.Fatalf("remove: %v", err)
	}
	if _, err := os.Stat(filepath.Join(lib, "lib-bp")); !os.IsNotExist(err) {
		t.Fatalf("still present after remove: %v", err)
	}
}

func TestApplyRollbackApplied(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "bp")
	if _, _, err := runCLI(clibp.Deps{}, "init", dir, "--name", "demo"); err != nil {
		t.Fatal(err)
	}
	ex := &bp.Executor{
		StateRunner: &fakeSR{},
		Store:       bp.NewMemoryAppliedStore(),
		NewID:       func() string { return "run-1" },
	}
	d := clibp.Deps{Executor: ex}

	out, _, err := runCLI(d, "apply", dir)
	if err != nil || !strings.Contains(out, "demo: succeeded (run run-1)") {
		t.Fatalf("apply: out=%q err=%v", out, err)
	}
	if !strings.Contains(out, "output summary = deployed demo") {
		t.Fatalf("apply output missing: %q", out)
	}

	out, _, err = runCLI(d, "applied")
	if err != nil || !strings.Contains(out, "run-1") {
		t.Fatalf("applied: out=%q err=%v", out, err)
	}

	out, _, err = runCLI(d, "rollback", "run-1")
	// demo scaffold has no rollback entrypoint → executor errors;
	// the CLI must surface it, not panic.
	if err == nil {
		t.Fatalf("expected rollback error (no rollback entrypoint), got out=%q", out)
	}
}

func TestApplyGuards(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "bp")
	if _, _, err := runCLI(clibp.Deps{}, "init", dir, "--name", "demo"); err != nil {
		t.Fatal(err)
	}
	// No executor wired.
	if _, _, err := runCLI(clibp.Deps{}, "apply", dir); !errors.Is(err, clibp.ErrEngineNotConfigured) {
		t.Fatalf("want ErrEngineNotConfigured, got %v", err)
	}
	// Remote target.
	d := clibp.Deps{Executor: &bp.Executor{StateRunner: &fakeSR{}, Store: bp.NewMemoryAppliedStore()}}
	if _, _, err := runCLI(d, "apply", dir, "--target", "id:agent-1"); !errors.Is(err, clibp.ErrRemoteNotConfigured) {
		t.Fatalf("want ErrRemoteNotConfigured, got %v", err)
	}
}
