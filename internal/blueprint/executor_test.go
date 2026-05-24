// SPDX-License-Identifier: Apache-2.0

package blueprint

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"go.keystone-core.io/keystone-core/internal/statemgmt"
)

type fakeStateRunner struct {
	decls  []*statemgmt.Declaration
	report *statemgmt.RunReport
	err    error
}

func (f *fakeStateRunner) Run(_ context.Context, decls []*statemgmt.Declaration) (*statemgmt.RunReport, error) {
	f.decls = decls
	if f.report == nil {
		f.report = &statemgmt.RunReport{}
	}
	return f.report, f.err
}

type fakeHooks struct {
	calls []string
	err   error
}

func (f *fakeHooks) RunHook(_ context.Context, hc HookContext) error {
	f.calls = append(f.calls, hc.Phase+"/"+hc.Name)
	return f.err
}

type fakeSecrets struct{ val string }

func (f fakeSecrets) ResolveSecret(context.Context, string) (string, error) { return f.val, nil }

// blueprintDir writes a blueprint.yaml + named extra files, returns dir.
func blueprintDir(t *testing.T, manifest string, files map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, ManifestFilename), []byte(manifest), 0o600); err != nil {
		t.Fatal(err)
	}
	for name, body := range files {
		p := filepath.Join(dir, name)
		if err := os.MkdirAll(filepath.Dir(p), 0o750); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

const applyManifest = `
metadata:
  name: demo
  version: 1.0.0
entrypoints:
  default: apply.yaml
  rollback: rollback.yaml
parameters:
  name:
    type: string
    required: true
outputs:
  greeting:
    value: "hello {{ .Params.name }}"
`

func loadBP(t *testing.T, dir string) *Manifest {
	t.Helper()
	m, err := Load(dir)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	return m
}

func TestExecutor_ApplyHappyPath(t *testing.T) {
	dir := blueprintDir(t, applyManifest, map[string]string{
		"apply.yaml":    "file:\n  /tmp/{{ .Params.name }}:\n    state: present\n",
		"rollback.yaml": "file:\n  /tmp/{{ .Params.name }}:\n    state: absent\n",
	})
	m := loadBP(t, dir)
	sr := &fakeStateRunner{}
	store := NewMemoryAppliedStore()
	e := &Executor{StateRunner: sr, Store: store, NewID: func() string { return "run-1" }}

	res, err := e.Apply(context.Background(), m, ApplyOptions{Inputs: map[string]string{"name": "web"}})
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if res.RunID != "run-1" || res.Status != "succeeded" {
		t.Fatalf("result=%+v", res)
	}
	if res.Outputs["greeting"] != "hello web" {
		t.Fatalf("outputs=%v", res.Outputs)
	}
	if len(sr.decls) != 1 || sr.decls[0].ID != "file:/tmp/web" {
		t.Fatalf("decls not rendered/resolved: %+v", sr.decls)
	}
	got, err := store.Get(context.Background(), "run-1")
	if err != nil || got.Status != "succeeded" || got.Entrypoint != "apply.yaml" {
		t.Fatalf("stored run wrong: %+v err=%v", got, err)
	}
}

func TestExecutor_NoStateRunner(t *testing.T) {
	if _, err := (&Executor{}).Apply(context.Background(), &Manifest{}, ApplyOptions{}); !errors.Is(err, ErrNoStateRunner) {
		t.Fatalf("err=%v", err)
	}
	if _, err := (&Executor{}).Rollback(context.Background(), "x"); !errors.Is(err, ErrNoStateRunner) {
		t.Fatalf("rollback err=%v", err)
	}
}

func TestExecutor_SecretSubstitution(t *testing.T) {
	man := `
metadata: {name: demo, version: 1.0.0}
entrypoints: {default: apply.yaml}
parameters:
  pw:
    type: string
    sensitive: true
    source: secret
`
	dir := blueprintDir(t, man, map[string]string{
		"apply.yaml": "file:\n  /tmp/secret-{{ .Params.pw }}:\n    state: present\n",
	})
	m := loadBP(t, dir)
	sr := &fakeStateRunner{}

	// With resolver: secret:// substituted before render.
	e := &Executor{StateRunner: sr, Secrets: fakeSecrets{val: "s3cr3t"}, NewID: func() string { return "r" }}
	if _, err := e.Apply(context.Background(), m, ApplyOptions{Inputs: map[string]string{"pw": "secret://kv/db"}}); err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if sr.decls[0].ID != "file:/tmp/secret-s3cr3t" {
		t.Fatalf("secret not substituted: %s", sr.decls[0].ID)
	}

	// Without resolver: secret:// input but no SecretResolver.
	e2 := &Executor{StateRunner: &fakeStateRunner{}}
	_, err := e2.Apply(context.Background(), m, ApplyOptions{Inputs: map[string]string{"pw": "secret://kv/db"}})
	if !errors.Is(err, ErrSecretResolverRequired) {
		t.Fatalf("err=%v want ErrSecretResolverRequired", err)
	}
}

func TestExecutor_ParamValidationPropagates(t *testing.T) {
	dir := blueprintDir(t, applyManifest, map[string]string{"apply.yaml": "file:\n  /tmp/x:\n    state: present\n"})
	m := loadBP(t, dir)
	e := &Executor{StateRunner: &fakeStateRunner{}}
	// "name" is required; omit it.
	_, err := e.Apply(context.Background(), m, ApplyOptions{})
	if !errors.Is(err, ErrParamValidation) {
		t.Fatalf("err=%v want ErrParamValidation", err)
	}
}

func TestExecutor_Namespaced(t *testing.T) {
	dir := blueprintDir(t, applyManifest, map[string]string{
		"apply.yaml":    "file:\n  /tmp/{{ .Params.name }}:\n    state: present\n",
		"rollback.yaml": "file:\n  /tmp/{{ .Params.name }}:\n    state: absent\n",
	})
	m := loadBP(t, dir)
	sr := &fakeStateRunner{}
	e := &Executor{StateRunner: sr, NewID: func() string { return "r" }}
	if _, err := e.Apply(context.Background(), m, ApplyOptions{Inputs: map[string]string{"name": "web"}, As: "inst1"}); err != nil {
		t.Fatal(err)
	}
	if sr.decls[0].ID != "file:inst1//tmp/web" {
		t.Fatalf("namespace not applied: %s", sr.decls[0].ID)
	}
}

func TestExecutor_ApplyFailedReport(t *testing.T) {
	dir := blueprintDir(t, applyManifest, map[string]string{
		"apply.yaml":    "file:\n  /tmp/{{ .Params.name }}:\n    state: present\n",
		"rollback.yaml": "x:\n",
	})
	m := loadBP(t, dir)
	store := NewMemoryAppliedStore()
	sr := &fakeStateRunner{report: &statemgmt.RunReport{Failed: 2}}
	e := &Executor{StateRunner: sr, Store: store, NewID: func() string { return "rf" }}
	res, err := e.Apply(context.Background(), m, ApplyOptions{Inputs: map[string]string{"name": "w"}})
	if !errors.Is(err, ErrApplyFailed) || res.Status != "failed" {
		t.Fatalf("err=%v res=%+v", err, res)
	}
	got, _ := store.Get(context.Background(), "rf")
	if got.Status != "failed" {
		t.Fatalf("stored status=%s want failed", got.Status)
	}
}

func TestExecutor_Hooks(t *testing.T) {
	man := `
metadata: {name: demo, version: 1.0.0}
entrypoints: {default: apply.yaml}
parameters: {name: {type: string, required: true}}
hooks:
  pre_apply: [warm.yaml]
  post_apply: [cool.yaml]
`
	dir := blueprintDir(t, man, map[string]string{"apply.yaml": "file:\n  /tmp/{{ .Params.name }}:\n    state: present\n"})
	m := loadBP(t, dir)

	fh := &fakeHooks{}
	e := &Executor{StateRunner: &fakeStateRunner{}, Hooks: fh, NewID: func() string { return "r" }}
	if _, err := e.Apply(context.Background(), m, ApplyOptions{Inputs: map[string]string{"name": "w"}}); err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if len(fh.calls) != 2 || fh.calls[0] != "pre_apply/warm.yaml" || fh.calls[1] != "post_apply/cool.yaml" {
		t.Fatalf("hook order wrong: %v", fh.calls)
	}

	// Hook'd manifest without a HookRunner fails loud.
	e2 := &Executor{StateRunner: &fakeStateRunner{}}
	_, err := e2.Apply(context.Background(), m, ApplyOptions{Inputs: map[string]string{"name": "w"}})
	if !errors.Is(err, ErrHookRunnerRequired) {
		t.Fatalf("err=%v want ErrHookRunnerRequired", err)
	}

	// Hookless manifest is fine without a HookRunner.
	dir2 := blueprintDir(t, applyManifest, map[string]string{
		"apply.yaml": "file:\n  /tmp/{{ .Params.name }}:\n    state: present\n", "rollback.yaml": "x:\n",
	})
	if _, err := (&Executor{StateRunner: &fakeStateRunner{}}).Apply(
		context.Background(), loadBP(t, dir2), ApplyOptions{Inputs: map[string]string{"name": "w"}}); err != nil {
		t.Fatalf("hookless apply: %v", err)
	}
}

func TestExecutor_RollbackRoundTrip(t *testing.T) {
	dir := blueprintDir(t, applyManifest, map[string]string{
		"apply.yaml":    "file:\n  /tmp/{{ .Params.name }}:\n    state: present\n",
		"rollback.yaml": "file:\n  /tmp/{{ .Params.name }}:\n    state: absent\n",
	})
	m := loadBP(t, dir)
	store := NewMemoryAppliedStore()
	sr := &fakeStateRunner{}
	ids := []string{"apply-1", "rb-1"}
	i := 0
	e := &Executor{StateRunner: sr, Store: store, NewID: func() string { id := ids[i]; i++; return id }}

	if _, err := e.Apply(context.Background(), m, ApplyOptions{Inputs: map[string]string{"name": "web"}}); err != nil {
		t.Fatal(err)
	}
	res, err := e.Rollback(context.Background(), "apply-1")
	if err != nil {
		t.Fatalf("Rollback: %v", err)
	}
	if res.RunID != "rb-1" {
		t.Fatalf("rollback run id=%s", res.RunID)
	}
	if sr.decls[0].State != "absent" {
		t.Fatalf("rollback entrypoint not used: %+v", sr.decls[0])
	}
	rb, _ := store.Get(context.Background(), "rb-1")
	if rb.ParentID != "apply-1" || rb.Entrypoint != "rollback.yaml" {
		t.Fatalf("rollback record wrong: %+v", rb)
	}

	if _, err := e.Rollback(context.Background(), "nope"); !errors.Is(err, ErrRunNotFound) {
		t.Fatalf("err=%v want ErrRunNotFound", err)
	}
}

func TestExecutor_MissingEntrypoint(t *testing.T) {
	man := "metadata: {name: demo, version: 1.0.0}\nentrypoints: {default: nope.yaml}\n"
	dir := blueprintDir(t, man, nil)
	m := loadBP(t, dir)
	_, err := (&Executor{StateRunner: &fakeStateRunner{}}).Apply(context.Background(), m, ApplyOptions{})
	if err == nil {
		t.Fatal("expected read-entrypoint error")
	}
}
