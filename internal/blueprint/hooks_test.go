// SPDX-License-Identifier: Apache-2.0

package blueprint

import (
	"context"
	"testing"

	"go.keystone-core.io/keystone-core/internal/runbook"
)

func newHookExec(t *testing.T) *runbook.Executor {
	t.Helper()
	reg := runbook.NewRegistry()
	if err := reg.Register("noop", runbook.StepFunc(func(context.Context, runbook.StepContext) (runbook.StepOutput, error) {
		return runbook.StepOutput{}, nil
	})); err != nil {
		t.Fatal(err)
	}
	if err := reg.Register("boom", runbook.StepFunc(func(context.Context, runbook.StepContext) (runbook.StepOutput, error) {
		return runbook.StepOutput{}, context.DeadlineExceeded
	})); err != nil {
		t.Fatal(err)
	}
	return &runbook.Executor{Registry: reg}
}

func TestRunbookHookRunner(t *testing.T) {
	man := "metadata: {name: demo, version: 1.0.0}\nentrypoints: {default: a.yaml}\n"
	okRB := `
metadata:
  name: warm
spec:
  inputs:
    - name: params
  steps:
    - type: noop
      name: s1
`
	failRB := "metadata: {name: bad}\nspec:\n  steps:\n    - {type: boom, name: s1}\n"
	dir := blueprintDir(t, man, map[string]string{"warm.yaml": okRB, "bad.yaml": failRB})
	m := loadBP(t, dir)

	h := NewRunbookHookRunner(newHookExec(t))

	t.Run("success", func(t *testing.T) {
		err := h.RunHook(context.Background(), HookContext{
			Manifest: m, Phase: PhasePreApply, Name: "warm.yaml",
			Params: map[string]any{"k": "v"},
		})
		if err != nil {
			t.Fatalf("RunHook: %v", err)
		}
	})

	t.Run("runbook failure surfaces", func(t *testing.T) {
		err := h.RunHook(context.Background(), HookContext{
			Manifest: m, Phase: PhasePostApply, Name: "bad.yaml",
		})
		if err == nil {
			t.Fatal("expected hook failure")
		}
	})

	t.Run("missing runbook file", func(t *testing.T) {
		err := h.RunHook(context.Background(), HookContext{Manifest: m, Name: "ghost.yaml"})
		if err == nil {
			t.Fatal("expected load error")
		}
	})

	t.Run("nil exec", func(t *testing.T) {
		if err := (&RunbookHookRunner{}).RunHook(context.Background(), HookContext{Manifest: m, Name: "warm.yaml"}); err != ErrHookRunnerRequired {
			t.Fatalf("err=%v want ErrHookRunnerRequired", err)
		}
	})
}

func TestExecutor_RunbookBackedHooksEndToEnd(t *testing.T) {
	man := `
metadata: {name: demo, version: 1.0.0}
entrypoints: {default: apply.yaml}
parameters: {name: {type: string, required: true}}
hooks:
  pre_apply: [warm.yaml]
`
	dir := blueprintDir(t, man, map[string]string{
		"apply.yaml": "file:\n  /tmp/{{ .Params.name }}:\n    state: present\n",
		"warm.yaml":  "metadata: {name: warm}\nspec:\n  steps:\n    - {type: noop, name: s1}\n",
	})
	m := loadBP(t, dir)
	sr := &fakeStateRunner{}
	e := &Executor{
		StateRunner: sr,
		Hooks:       NewRunbookHookRunner(newHookExec(t)),
		NewID:       func() string { return "r" },
	}
	if _, err := e.Apply(context.Background(), m, ApplyOptions{Inputs: map[string]string{"name": "web"}}); err != nil {
		t.Fatalf("Apply with runbook-backed hook: %v", err)
	}
	if sr.decls[0].ID != "file:/tmp/web" {
		t.Fatalf("entrypoint not applied: %+v", sr.decls)
	}
}
