package runbook_test

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	clirb "go.keystone-core.io/keystone-core/internal/cli/runbook"
	rb "go.keystone-core.io/keystone-core/internal/runbook"
)

func runCLI(d clirb.Deps, args ...string) (string, string, error) {
	c := clirb.NewCommand(d)
	var out, errb bytes.Buffer
	c.SetArgs(args)
	c.SetOut(&out)
	c.SetErr(&errb)
	err := c.Execute()
	return out.String(), errb.String(), err
}

const okRunbook = `metadata:
  name: db-restart
spec:
  inputs:
    - name: agent_id
  steps:
    - type: noop
      name: stop
    - type: noop
      name: start
      depends_on: [stop]
`

const cycleRunbook = `metadata:
  name: cyclic
spec:
  steps:
    - {type: noop, name: a, depends_on: [b]}
    - {type: noop, name: b, depends_on: [a]}
`

func write(t *testing.T, name, body string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(p, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	return p
}

func noopExecutor() *rb.Executor {
	reg := rb.NewRegistry()
	_ = reg.Register("noop", rb.StepFunc(func(context.Context, rb.StepContext) (rb.StepOutput, error) {
		return rb.StepOutput{}, nil
	}))
	return &rb.Executor{Registry: reg, NewID: func() string { return "exec-1" }}
}

func TestTestAndList(t *testing.T) {
	p := write(t, "rb.yaml", okRunbook)
	out, _, err := runCLI(clirb.Deps{}, "test", p)
	if err != nil || !strings.Contains(out, "ok: db-restart (2 steps)") {
		t.Fatalf("test: out=%q err=%v", out, err)
	}

	cp := write(t, "cyc.yaml", cycleRunbook)
	if _, _, err := runCLI(clirb.Deps{}, "test", cp); err == nil {
		t.Fatal("test should fail on a dependency cycle")
	}

	out, _, err = runCLI(clirb.Deps{}, "list", filepath.Dir(p))
	if err != nil || !strings.Contains(out, "db-restart") {
		t.Fatalf("list: out=%q err=%v", out, err)
	}
}

func TestExecuteStatusAuditListExecutions(t *testing.T) {
	p := write(t, "rb.yaml", okRunbook)
	d := clirb.Deps{Executor: noopExecutor(), Store: clirb.NewMemoryExecutionStore()}

	out, _, err := runCLI(d, "execute", p, "--input", "agent_id=a1")
	if err != nil || !strings.Contains(out, "status=succeeded") {
		t.Fatalf("execute: out=%q err=%v", out, err)
	}

	out, _, err = runCLI(d, "status", "exec-1")
	if err != nil || !strings.Contains(out, "exec-1") {
		t.Fatalf("status: out=%q err=%v", out, err)
	}

	out, _, err = runCLI(d, "list-executions")
	if err != nil || !strings.Contains(out, "exec-1") {
		t.Fatalf("list-executions: out=%q err=%v", out, err)
	}

	out, _, err = runCLI(d, "audit", "exec-1")
	if err != nil || !strings.Contains(out, "audit trail for execution exec-1") {
		t.Fatalf("audit: out=%q err=%v", out, err)
	}

	if _, _, err := runCLI(d, "status", "nope"); err == nil {
		t.Fatal("status of unknown id should error")
	}
}

func TestExecuteGuards(t *testing.T) {
	p := write(t, "rb.yaml", okRunbook)
	if _, _, err := runCLI(clirb.Deps{}, "execute", p); !errors.Is(err, clirb.ErrEngineNotConfigured) {
		t.Fatalf("want ErrEngineNotConfigured, got %v", err)
	}
	if _, _, err := runCLI(clirb.Deps{Executor: noopExecutor()}, "execute", p, "--input", "bad"); err == nil {
		t.Fatal("want invalid --input error")
	}
}
