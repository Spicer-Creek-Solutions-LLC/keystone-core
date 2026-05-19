package steps

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"go.keystone-core.io/keystone-core/internal/runbook"
)

func sc(cfg map[string]any) runbook.StepContext {
	return runbook.StepContext{Config: cfg}
}

// --- fakes ------------------------------------------------------------

type fakeCmd struct {
	got CommandRequest
	res CommandResult
	err error
}

func (f *fakeCmd) Run(_ context.Context, req CommandRequest) (CommandResult, error) {
	f.got = req
	return f.res, f.err
}

type fakeState struct {
	res StateResult
	err error
}

func (f *fakeState) Apply(context.Context, StateRequest) (StateResult, error) {
	return f.res, f.err
}

type fakeNotifier struct {
	got Notification
	err error
}

func (f *fakeNotifier) Notify(_ context.Context, n Notification) error {
	f.got = n
	return f.err
}

type fakeQuerier struct {
	rows []map[string]any
	err  error
}

func (f *fakeQuerier) Query(context.Context, string, ...any) ([]map[string]any, error) {
	return f.rows, f.err
}

// --- noop / fail ------------------------------------------------------

func TestNoopStep(t *testing.T) {
	out, err := noopStep(context.Background(), sc(nil))
	if err != nil || out.Outputs != nil {
		t.Fatalf("empty noop: out=%v err=%v", out, err)
	}
	out, err = noopStep(context.Background(), sc(map[string]any{"outputs": map[string]any{"k": 1}}))
	if err != nil || out.Outputs["k"] != 1 {
		t.Fatalf("noop outputs passthrough: %v %v", out, err)
	}
}

func TestFailStep(t *testing.T) {
	_, err := failStep(context.Background(), sc(nil))
	if err == nil || err.Error() != "runbook: fail step" {
		t.Fatalf("default: %v", err)
	}
	_, err = failStep(context.Background(), sc(map[string]any{"message": "boom"}))
	if err == nil || err.Error() != "boom" {
		t.Fatalf("custom: %v", err)
	}
}

// --- wait -------------------------------------------------------------

func TestWaitStep(t *testing.T) {
	var slept time.Duration
	d := Deps{Sleep: func(_ context.Context, dur time.Duration) error { slept = dur; return nil }}

	out, err := d.waitStep(context.Background(), sc(map[string]any{"duration": "50ms"}))
	if err != nil || slept != 50*time.Millisecond || out.Outputs["waited"] != "50ms" {
		t.Fatalf("ok wait: slept=%v out=%v err=%v", slept, out, err)
	}
	if _, err := d.waitStep(context.Background(), sc(nil)); !errors.Is(err, ErrStepConfig) {
		t.Fatalf("missing duration: %v", err)
	}
	if _, err := d.waitStep(context.Background(), sc(map[string]any{"duration": "nope"})); !errors.Is(err, ErrStepConfig) {
		t.Fatalf("bad duration: %v", err)
	}
	derr := Deps{Sleep: func(context.Context, time.Duration) error { return context.Canceled }}
	if _, err := derr.waitStep(context.Background(), sc(map[string]any{"duration": "1s"})); !errors.Is(err, context.Canceled) {
		t.Fatalf("ctx cancel: %v", err)
	}
}

// --- command / script -------------------------------------------------

func TestCommandStep(t *testing.T) {
	if _, err := (Deps{}).commandStep(context.Background(), sc(map[string]any{"command": "x"})); !errors.Is(err, ErrStepNotConfigured) {
		t.Fatalf("nil runner: %v", err)
	}
	d := Deps{Command: &fakeCmd{res: CommandResult{ExitCode: 0, Stdout: "hi"}}}
	if _, err := d.commandStep(context.Background(), sc(nil)); !errors.Is(err, ErrStepConfig) {
		t.Fatalf("missing command: %v", err)
	}
	out, err := d.commandStep(context.Background(), sc(map[string]any{
		"command": "echo", "args": []any{"hi"}, "env": map[string]any{"A": "B"}, "timeout": "5s",
	}))
	if err != nil || out.Outputs["stdout"] != "hi" || out.Outputs["exit_code"] != 0 {
		t.Fatalf("ok command: %v %v", out, err)
	}
	fc := d.Command.(*fakeCmd)
	if fc.got.Command != "echo" || fc.got.Args[0] != "hi" || fc.got.Env["A"] != "B" || fc.got.Timeout != 5*time.Second {
		t.Fatalf("request not built: %+v", fc.got)
	}

	nz := Deps{Command: &fakeCmd{res: CommandResult{ExitCode: 2, Stderr: "bad"}}}
	out, err = nz.commandStep(context.Background(), sc(map[string]any{"command": "x"}))
	if err == nil || out.Outputs["exit_code"] != 2 {
		t.Fatalf("non-zero exit: out=%v err=%v", out, err)
	}

	re := Deps{Command: &fakeCmd{err: errors.New("spawn fail")}}
	if _, err := re.commandStep(context.Background(), sc(map[string]any{"command": "x"})); err == nil {
		t.Fatal("runner error must surface")
	}
}

func TestScriptStep(t *testing.T) {
	if _, err := (Deps{}).scriptStep(context.Background(), sc(map[string]any{"script": "x"})); !errors.Is(err, ErrStepNotConfigured) {
		t.Fatalf("nil runner: %v", err)
	}
	d := Deps{Command: &fakeCmd{res: CommandResult{ExitCode: 0}}}
	if _, err := d.scriptStep(context.Background(), sc(nil)); !errors.Is(err, ErrStepConfig) {
		t.Fatalf("missing script: %v", err)
	}
	if _, err := d.scriptStep(context.Background(), sc(map[string]any{"script": "echo hi"})); err != nil {
		t.Fatalf("ok script: %v", err)
	}
	fc := d.Command.(*fakeCmd)
	if fc.got.Command != "sh" || len(fc.got.Args) != 2 || fc.got.Args[0] != "-c" || fc.got.Args[1] != "echo hi" {
		t.Fatalf("default interpreter wiring: %+v", fc.got)
	}
	_, _ = d.scriptStep(context.Background(), sc(map[string]any{
		"script": "run", "interpreter": "bash", "interpreter_args": []any{"-eu", "-c"},
	}))
	if fc.got.Command != "bash" || fc.got.Args[0] != "-eu" || fc.got.Args[2] != "run" {
		t.Fatalf("custom interpreter wiring: %+v", fc.got)
	}
}

// --- state ------------------------------------------------------------

func TestStateStep(t *testing.T) {
	if _, err := (Deps{}).stateStep(context.Background(), sc(map[string]any{"state": "x"})); !errors.Is(err, ErrStepNotConfigured) {
		t.Fatalf("nil applier: %v", err)
	}
	d := Deps{State: &fakeState{res: StateResult{Changed: 2, Summary: "ok"}}}
	if _, err := d.stateStep(context.Background(), sc(nil)); !errors.Is(err, ErrStepConfig) {
		t.Fatalf("missing state: %v", err)
	}
	out, err := d.stateStep(context.Background(), sc(map[string]any{"state": "decls: []"}))
	if err != nil || out.Outputs["changed"] != 2 {
		t.Fatalf("ok state: %v %v", out, err)
	}
	bad := Deps{State: &fakeState{res: StateResult{Failed: 1}}}
	if out, err := bad.stateStep(context.Background(), sc(map[string]any{"state": "x"})); err == nil || out.Outputs["failed"] != 1 {
		t.Fatalf("failed decl: out=%v err=%v", out, err)
	}
	er := Deps{State: &fakeState{err: errors.New("apply boom")}}
	if _, err := er.stateStep(context.Background(), sc(map[string]any{"state": "x"})); err == nil {
		t.Fatal("applier error must surface")
	}
}

// --- api --------------------------------------------------------------

func TestAPIStep(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Test") == "yes" && r.Method == http.MethodPost {
			w.WriteHeader(201)
		}
		_, _ = w.Write([]byte("pong"))
	}))
	defer srv.Close()

	d := Deps{}
	out, err := d.apiStep(context.Background(), sc(map[string]any{
		"url": srv.URL, "method": "post", "headers": map[string]any{"X-Test": "yes"}, "body": "ping",
	}))
	if err != nil || out.Outputs["status"] != 201 || out.Outputs["body"] != "pong" {
		t.Fatalf("ok api: out=%v err=%v", out, err)
	}

	if _, err := d.apiStep(context.Background(), sc(nil)); !errors.Is(err, ErrStepConfig) {
		t.Fatalf("missing url: %v", err)
	}
	if out, err := d.apiStep(context.Background(), sc(map[string]any{
		"url": srv.URL, "expect_status": 500,
	})); err == nil || out.Outputs["status"] != 200 {
		t.Fatalf("expect_status mismatch: out=%v err=%v", out, err)
	}
	if _, err := d.apiStep(context.Background(), sc(map[string]any{
		"url": srv.URL, "expect_status": "five",
	})); !errors.Is(err, ErrStepConfig) {
		t.Fatalf("bad expect_status: %v", err)
	}
	if _, err := d.apiStep(context.Background(), sc(map[string]any{"url": "http://127.0.0.1:0/x"})); err == nil {
		t.Fatal("transport error must surface")
	}
	// expect_status that matches → success.
	if _, err := d.apiStep(context.Background(), sc(map[string]any{"url": srv.URL, "expect_status": 200})); err != nil {
		t.Fatalf("expect_status match: %v", err)
	}
}

// --- notification -----------------------------------------------------

func TestNotificationStep(t *testing.T) {
	if _, err := (Deps{}).notificationStep(context.Background(), sc(map[string]any{"channel": "c", "message": "m"})); !errors.Is(err, ErrStepNotConfigured) {
		t.Fatalf("nil notifier: %v", err)
	}
	fn := &fakeNotifier{}
	d := Deps{Notifier: fn}
	if _, err := d.notificationStep(context.Background(), sc(map[string]any{"message": "m"})); !errors.Is(err, ErrStepConfig) {
		t.Fatalf("missing channel: %v", err)
	}
	out, err := d.notificationStep(context.Background(), sc(map[string]any{
		"channel": "ops", "message": "hi", "severity": "warn", "fields": map[string]any{"x": 1},
	}))
	if err != nil || out.Outputs["delivered"] != true || fn.got.Severity != "warn" || fn.got.Fields["x"] != 1 {
		t.Fatalf("ok notify: out=%v err=%v got=%+v", out, err, fn.got)
	}
	if _, err := d.notificationStep(context.Background(), sc(map[string]any{
		"channel": "c", "message": "m", "fields": "notamap",
	})); !errors.Is(err, ErrStepConfig) {
		t.Fatalf("bad fields: %v", err)
	}
	er := Deps{Notifier: &fakeNotifier{err: errors.New("send boom")}}
	if _, err := er.notificationStep(context.Background(), sc(map[string]any{"channel": "c", "message": "m"})); err == nil {
		t.Fatal("notifier error must surface")
	}
}

// --- query ------------------------------------------------------------

func TestQueryStep(t *testing.T) {
	if _, err := (Deps{}).queryStep(context.Background(), sc(map[string]any{"query": "q"})); !errors.Is(err, ErrStepNotConfigured) {
		t.Fatalf("nil querier: %v", err)
	}
	d := Deps{Querier: &fakeQuerier{rows: []map[string]any{{"id": 1}, {"id": 2}}}}
	if _, err := d.queryStep(context.Background(), sc(nil)); !errors.Is(err, ErrStepConfig) {
		t.Fatalf("missing query: %v", err)
	}
	out, err := d.queryStep(context.Background(), sc(map[string]any{"query": "select 1", "args": []any{1, "x"}}))
	if err != nil || out.Outputs["count"] != 2 {
		t.Fatalf("ok query: out=%v err=%v", out, err)
	}
	if _, err := d.queryStep(context.Background(), sc(map[string]any{"query": "q", "args": "notalist"})); !errors.Is(err, ErrStepConfig) {
		t.Fatalf("bad args: %v", err)
	}
	er := Deps{Querier: &fakeQuerier{err: errors.New("db boom")}}
	if _, err := er.queryStep(context.Background(), sc(map[string]any{"query": "q"})); err == nil {
		t.Fatal("querier error must surface")
	}
}

// --- RegisterAll ------------------------------------------------------

func TestRegisterAll(t *testing.T) {
	reg := runbook.NewRegistry()
	if err := RegisterAll(reg, Deps{}); err != nil {
		t.Fatal(err)
	}
	for _, typ := range []string{"noop", "fail", "wait", "command", "script", "state", "api", "notification", "query"} {
		if _, ok := reg.Lookup(typ); !ok {
			t.Errorf("type %q not registered", typ)
		}
	}
}
