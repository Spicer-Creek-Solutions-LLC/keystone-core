package cmd

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"go.keystone-core.io/keystone-core/internal/statemgmt"
)

func declFor(name, command string, extra map[string]any) *statemgmt.Declaration {
	params := map[string]any{paramCommand: command}
	for k, v := range extra {
		params[k] = v
	}
	return &statemgmt.Declaration{
		ID:     "cmd:" + name,
		Module: "cmd",
		Name:   name,
		State:  StateRun,
		Params: params,
	}
}

// skipIfWindows aborts the test on Windows since the module is
// Linux + macOS only in v1.0.
func skipIfWindows(t *testing.T) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("cmd module is /bin/sh-based; Windows is v1.x")
	}
}

func newModule() *Module { return &Module{} }

// ---- parseParams / validate ---------------------------------------

func TestParseParams_RejectsUnknownKey(t *testing.T) {
	t.Parallel()
	_, err := parseParams(declFor("x", "true", map[string]any{"comand": "typo"}))
	if err == nil || !strings.Contains(err.Error(), "unknown param") {
		t.Errorf("err = %v, want unknown-param error", err)
	}
}

func TestValidate_RequiresCommand(t *testing.T) {
	t.Parallel()
	decl := &statemgmt.Declaration{
		ID:     "cmd:x",
		Module: "cmd",
		Name:   "x",
		State:  StateRun,
		Params: map[string]any{"creates": "/tmp/x"},
	}
	p, err := parseParams(decl)
	if err != nil {
		t.Fatalf("parseParams: %v", err)
	}
	if err := p.validate(); err == nil || !strings.Contains(err.Error(), "command is required") {
		t.Errorf("want command-required error, got %v", err)
	}
}

func TestValidate_RequiresGuard(t *testing.T) {
	t.Parallel()
	p, err := parseParams(declFor("x", "true", nil))
	if err != nil {
		t.Fatalf("parseParams: %v", err)
	}
	err = p.validate()
	if err == nil || !strings.Contains(err.Error(), "at least one guard") {
		t.Errorf("want mandatory-guard error, got %v", err)
	}
	// The error should point operators at the always-run pattern.
	if !strings.Contains(err.Error(), "/bin/true") {
		t.Errorf("error should suggest onlyif: /bin/true workaround; got %v", err)
	}
}

func TestValidate_TimeoutOutOfRange(t *testing.T) {
	t.Parallel()
	cases := []int{-1, maxTimeoutSeconds + 1, 99999}
	for _, v := range cases {
		p, err := parseParams(declFor("x", "true", map[string]any{
			"creates":         "/tmp/x",
			"timeout_seconds": v,
		}))
		if err != nil {
			t.Fatalf("parseParams %d: %v", v, err)
		}
		if err := p.validate(); err == nil {
			t.Errorf("timeout %d should be rejected", v)
		}
	}
}

func TestValidate_ShellOnlyBinSh(t *testing.T) {
	t.Parallel()
	p, _ := parseParams(declFor("x", "true", map[string]any{
		"creates": "/tmp/x",
		"shell":   "/bin/bash",
	}))
	if err := p.validate(); err == nil || !strings.Contains(err.Error(), "v1.0") {
		t.Errorf("want shell-rejected error, got %v", err)
	}
}

func TestValidate_CreatesMustBeAbsolute(t *testing.T) {
	t.Parallel()
	p, _ := parseParams(declFor("x", "true", map[string]any{"creates": "relative/path"}))
	if err := p.validate(); err == nil || !strings.Contains(err.Error(), "absolute") {
		t.Errorf("want absolute-path error, got %v", err)
	}
}

func TestParseParams_NonStringValues(t *testing.T) {
	t.Parallel()
	cases := map[string]map[string]any{
		"command not string": {"command": 42, "creates": "/tmp/x"},
		"cwd not string":     {"command": "true", "cwd": 1, "creates": "/tmp/x"},
		"creates not string": {"command": "true", "creates": 1},
		"shell not string":   {"command": "true", "shell": 1, "creates": "/tmp/x"},
		"unless not string":  {"command": "true", "unless": 1},
		"onlyif not string":  {"command": "true", "onlyif": 1},
	}
	for name, params := range cases {
		t.Run(name, func(t *testing.T) {
			decl := &statemgmt.Declaration{
				ID: "cmd:x", Module: "cmd", Name: "x", State: StateRun, Params: params,
			}
			_, err := parseParams(decl)
			if err == nil {
				t.Errorf("expected error for %s", name)
			}
		})
	}
}

func TestParseParams_EnvNotMap(t *testing.T) {
	t.Parallel()
	decl := &statemgmt.Declaration{
		ID: "cmd:x", Module: "cmd", Name: "x", State: StateRun,
		Params: map[string]any{
			"command": "true", "creates": "/tmp/x", "env": "not-a-map",
		},
	}
	_, err := parseParams(decl)
	if err == nil || !strings.Contains(err.Error(), "env") {
		t.Errorf("want env-type error, got %v", err)
	}
}

func TestParseParams_EnvValueNotString(t *testing.T) {
	t.Parallel()
	decl := &statemgmt.Declaration{
		ID: "cmd:x", Module: "cmd", Name: "x", State: StateRun,
		Params: map[string]any{
			"command": "true", "creates": "/tmp/x",
			"env": map[string]any{"FOO": 42},
		},
	}
	_, err := parseParams(decl)
	if err == nil {
		t.Error("expected env-value-type error")
	}
}

func TestParseParams_TimeoutCoercion(t *testing.T) {
	t.Parallel()
	cases := map[string]any{
		"int":   30,
		"int64": int64(30),
		"float": float64(30),
	}
	for name, v := range cases {
		t.Run(name, func(t *testing.T) {
			p, err := parseParams(declFor("x", "true", map[string]any{
				"creates":         "/tmp/x",
				"timeout_seconds": v,
			}))
			if err != nil {
				t.Errorf("err = %v", err)
				return
			}
			if p.TimeoutSeconds != 30 {
				t.Errorf("TimeoutSeconds = %d, want 30", p.TimeoutSeconds)
			}
		})
	}
}

func TestParseParams_FractionalFloatRejected(t *testing.T) {
	t.Parallel()
	_, err := parseParams(declFor("x", "true", map[string]any{
		"creates":         "/tmp/x",
		"timeout_seconds": 30.5,
	}))
	if err == nil {
		t.Error("expected fractional-timeout rejection")
	}
}

// ---- Module surface ----------------------------------------------

func TestModule_NameAndValidStates(t *testing.T) {
	t.Parallel()
	m := newModule()
	if m.Name() != "cmd" {
		t.Errorf("Name = %q", m.Name())
	}
	if len(m.ValidStates()) != 1 || m.ValidStates()[0] != StateRun {
		t.Errorf("ValidStates = %v, want [run]", m.ValidStates())
	}
}

func TestModule_ImplementsValidatableAndDriftSeverity(t *testing.T) {
	t.Parallel()
	var _ statemgmt.ValidatableModule = newModule()
	var _ statemgmt.DriftSeverityModule = newModule()
}

func TestModule_DriftSeverity_Medium(t *testing.T) {
	t.Parallel()
	m := newModule()
	if got := m.DriftSeverity(nil, nil); got != statemgmt.DriftSeverityMedium {
		t.Errorf("DriftSeverity = %v, want medium", got)
	}
}

func TestNew_ReturnsModule(t *testing.T) {
	t.Parallel()
	m := New()
	if m == nil || m.Name() != "cmd" {
		t.Errorf("New() = %+v, want cmd module", m)
	}
}

// ---- Guard evaluation --------------------------------------------

func TestCheck_CreatesMissing_ReportsDrift(t *testing.T) {
	skipIfWindows(t)
	t.Parallel()
	dir := t.TempDir()
	marker := filepath.Join(dir, "marker")
	m := newModule()
	decl := declFor("setup", "touch "+marker, map[string]any{"creates": marker})
	res, err := m.Check(context.Background(), decl)
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	if res.Matches {
		t.Error("missing creates path should be drift")
	}
	if !strings.Contains(res.Diff, "missing") {
		t.Errorf("Diff should mention missing; got %q", res.Diff)
	}
}

func TestCheck_CreatesPresent_ReportsConverged(t *testing.T) {
	skipIfWindows(t)
	t.Parallel()
	dir := t.TempDir()
	marker := filepath.Join(dir, "marker")
	if err := os.WriteFile(marker, []byte("x"), 0o644); err != nil {
		t.Fatalf("seed: %v", err)
	}
	m := newModule()
	decl := declFor("setup", "touch "+marker, map[string]any{"creates": marker})
	res, _ := m.Check(context.Background(), decl)
	if !res.Matches {
		t.Errorf("existing creates path should be converged; diff = %q", res.Diff)
	}
}

func TestCheck_OnlyIfTrue_ReportsDrift(t *testing.T) {
	skipIfWindows(t)
	t.Parallel()
	m := newModule()
	decl := declFor("x", "echo hi", map[string]any{"onlyif": "/bin/true"})
	res, _ := m.Check(context.Background(), decl)
	if res.Matches {
		t.Error("onlyif true means precondition met → drift (must run)")
	}
}

func TestCheck_OnlyIfFalse_ReportsConverged(t *testing.T) {
	skipIfWindows(t)
	t.Parallel()
	m := newModule()
	decl := declFor("x", "echo hi", map[string]any{"onlyif": "/bin/false"})
	res, _ := m.Check(context.Background(), decl)
	if !res.Matches {
		t.Errorf("onlyif false means precondition unmet → skip; diff = %q", res.Diff)
	}
}

func TestCheck_UnlessTrue_ReportsConverged(t *testing.T) {
	skipIfWindows(t)
	t.Parallel()
	m := newModule()
	decl := declFor("x", "echo hi", map[string]any{"unless": "/bin/true"})
	res, _ := m.Check(context.Background(), decl)
	if !res.Matches {
		t.Errorf("unless true means already converged → skip; diff = %q", res.Diff)
	}
}

func TestCheck_UnlessFalse_ReportsDrift(t *testing.T) {
	skipIfWindows(t)
	t.Parallel()
	m := newModule()
	decl := declFor("x", "echo hi", map[string]any{"unless": "/bin/false"})
	res, _ := m.Check(context.Background(), decl)
	if res.Matches {
		t.Error("unless false means must run → drift")
	}
}

func TestCheck_GuardShortCircuit(t *testing.T) {
	skipIfWindows(t)
	t.Parallel()
	dir := t.TempDir()
	marker := filepath.Join(dir, "marker")
	if err := os.WriteFile(marker, []byte("x"), 0o644); err != nil {
		t.Fatalf("seed: %v", err)
	}
	m := newModule()
	// creates exists → skip, even though unless/onlyif would say run.
	decl := declFor("x", "true", map[string]any{
		"creates": marker,
		"unless":  "/bin/false", // would say "must run"
		"onlyif":  "/bin/true",  // would say "must run"
	})
	res, _ := m.Check(context.Background(), decl)
	if !res.Matches {
		t.Error("creates-exists should short-circuit other guards")
	}
}

func TestCheck_AllGuardsMustPermit(t *testing.T) {
	skipIfWindows(t)
	t.Parallel()
	dir := t.TempDir()
	marker := filepath.Join(dir, "missing")
	m := newModule()
	// creates missing (→ run) + onlyif true (→ run) + unless false (→ run): drift.
	decl := declFor("x", "true", map[string]any{
		"creates": marker,
		"onlyif":  "/bin/true",
		"unless":  "/bin/false",
	})
	res, _ := m.Check(context.Background(), decl)
	if res.Matches {
		t.Error("all guards permit run → drift")
	}
}

// ---- Apply -------------------------------------------------------

func TestApply_SuccessCreatesMarker(t *testing.T) {
	skipIfWindows(t)
	t.Parallel()
	dir := t.TempDir()
	marker := filepath.Join(dir, "marker")
	m := newModule()
	decl := declFor("setup", "touch "+marker, map[string]any{"creates": marker})

	res, err := m.Apply(context.Background(), decl)
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if !res.Success || !res.Changed {
		t.Errorf("Success=%v Changed=%v, want true/true", res.Success, res.Changed)
	}
	if _, err := os.Stat(marker); err != nil {
		t.Errorf("marker not created: %v", err)
	}
	// Idempotency: Check after Apply should match.
	check, _ := m.Check(context.Background(), decl)
	if !check.Matches {
		t.Errorf("re-Check should match after successful Apply; diff = %q", check.Diff)
	}
}

func TestApply_NonZeroExit(t *testing.T) {
	skipIfWindows(t)
	t.Parallel()
	m := newModule()
	decl := declFor("fail", "exit 7", map[string]any{"onlyif": "/bin/true"})
	res, err := m.Apply(context.Background(), decl)
	if err == nil || !strings.Contains(err.Error(), "exited 7") {
		t.Errorf("expected exit-7 error, got %v", err)
	}
	if res.Success {
		t.Error("Success should be false on non-zero exit")
	}
	if !strings.Contains(res.Diff, "exit=7") {
		t.Errorf("Diff should mention exit code; got %q", res.Diff)
	}
}

func TestApply_TimeoutKillsProcess(t *testing.T) {
	skipIfWindows(t)
	t.Parallel()
	m := newModule()
	decl := declFor("slow", "sleep 10", map[string]any{
		"onlyif":          "/bin/true",
		"timeout_seconds": 1,
	})
	res, err := m.Apply(context.Background(), decl)
	if err == nil || !strings.Contains(err.Error(), "timed out") {
		t.Errorf("expected timeout error, got %v", err)
	}
	if res.Success {
		t.Error("Success should be false on timeout")
	}
}

func TestApply_CwdHonored(t *testing.T) {
	skipIfWindows(t)
	t.Parallel()
	dir := t.TempDir()
	m := newModule()
	decl := declFor("pwd", "pwd", map[string]any{
		"onlyif": "/bin/true",
		"cwd":    dir,
	})
	res, err := m.Apply(context.Background(), decl)
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}
	// macOS resolves /var/folders to /private/var/folders; check
	// the trailing path component instead of a strict equality.
	if !strings.Contains(res.Comment, filepath.Base(dir)) {
		t.Errorf("Comment should include cwd basename; got %q (cwd=%s)", res.Comment, dir)
	}
}

func TestApply_EnvHonored(t *testing.T) {
	skipIfWindows(t)
	t.Parallel()
	m := newModule()
	decl := declFor("env", "echo $FOO", map[string]any{
		"onlyif": "/bin/true",
		"env":    map[string]any{"FOO": "bar"},
	})
	res, err := m.Apply(context.Background(), decl)
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if !strings.Contains(res.Comment, "bar") {
		t.Errorf("Comment should include env value; got %q", res.Comment)
	}
}

func TestApply_BadCwd(t *testing.T) {
	skipIfWindows(t)
	t.Parallel()
	m := newModule()
	decl := declFor("pwd", "pwd", map[string]any{
		"onlyif": "/bin/true",
		"cwd":    "/no/such/directory",
	})
	_, err := m.Apply(context.Background(), decl)
	if err == nil {
		t.Error("expected error from bad cwd")
	}
}

// ---- Test (post-Apply re-check) ----------------------------------

func TestTest_ReturnsTrueAfterApply(t *testing.T) {
	skipIfWindows(t)
	t.Parallel()
	dir := t.TempDir()
	marker := filepath.Join(dir, "marker")
	m := newModule()
	decl := declFor("setup", "touch "+marker, map[string]any{"creates": marker})

	if _, err := m.Apply(context.Background(), decl); err != nil {
		t.Fatalf("Apply: %v", err)
	}
	ok, err := m.Test(context.Background(), decl)
	if err != nil || !ok {
		t.Errorf("Test = %v err=%v, want true/nil", ok, err)
	}
}

func TestTest_ReturnsFalseWhenGuardsStillSayRun(t *testing.T) {
	skipIfWindows(t)
	t.Parallel()
	dir := t.TempDir()
	marker := filepath.Join(dir, "neverexists")
	m := newModule()
	// Command that doesn't actually create the marker → Test
	// should report drift remains.
	decl := declFor("nope", "true", map[string]any{"creates": marker})
	ok, err := m.Test(context.Background(), decl)
	if err != nil {
		t.Fatalf("Test: %v", err)
	}
	if ok {
		t.Error("Test should return false when creates path still missing")
	}
}

// ---- runShell / commandOutcome helpers ---------------------------

func TestCommandOutcome_StringFormat(t *testing.T) {
	t.Parallel()
	o := commandOutcome{ExitCode: 0, Stdout: "hello\nworld"}
	if !strings.Contains(o.String(), "exit=0") {
		t.Errorf("missing exit code; got %q", o.String())
	}
	if !strings.Contains(o.String(), "stdout=") {
		t.Errorf("missing stdout marker; got %q", o.String())
	}

	o2 := commandOutcome{ExitCode: -1, TimedOut: true}
	if !strings.Contains(o2.String(), "timed out") {
		t.Errorf("missing timeout indicator; got %q", o2.String())
	}
}

func TestTruncate(t *testing.T) {
	t.Parallel()
	if got := truncate("hello", 10); got != "hello" {
		t.Errorf("short: got %q", got)
	}
	if got := truncate("hello world", 5); got != "he..." {
		t.Errorf("long: got %q", got)
	}
	if got := truncate("hello", 2); got != "he" {
		t.Errorf("n=2: got %q", got)
	}
}

func TestCappedBuffer_DropsOverflow(t *testing.T) {
	t.Parallel()
	var b cappedBuffer
	b.cap = 5
	n, err := b.Write([]byte("abcdefghij"))
	if err != nil {
		t.Fatalf("write: %v", err)
	}
	if n != 10 {
		t.Errorf("returned n = %d, want 10 (write reports caller's bytes)", n)
	}
	if !strings.HasPrefix(b.String(), "abcde") {
		t.Errorf("first 5 bytes lost; got %q", b.String())
	}
	if !strings.Contains(b.String(), "truncated") {
		t.Errorf("missing truncation marker; got %q", b.String())
	}
}

func TestRunShell_DefaultTimeoutApplied(t *testing.T) {
	skipIfWindows(t)
	t.Parallel()
	p := &params{Command: "true", Shell: defaultShell, TimeoutSeconds: 0}
	out, err := runShell(context.Background(), p, "true")
	if err != nil {
		t.Fatalf("runShell: %v", err)
	}
	if out.ExitCode != 0 {
		t.Errorf("ExitCode = %d, want 0", out.ExitCode)
	}
}

func TestPathExists(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	exists, err := pathExists(filepath.Join(dir, "missing"))
	if err != nil || exists {
		t.Errorf("missing: exists=%v err=%v, want false/nil", exists, err)
	}
	present := filepath.Join(dir, "present")
	_ = os.WriteFile(present, []byte("x"), 0o644)
	exists, err = pathExists(present)
	if err != nil || !exists {
		t.Errorf("present: exists=%v err=%v, want true/nil", exists, err)
	}
}

func TestRunShell_ContextCancelled(t *testing.T) {
	skipIfWindows(t)
	t.Parallel()
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // pre-cancel
	p := &params{Command: "sleep 5", Shell: defaultShell, TimeoutSeconds: 10}
	_, err := runShell(ctx, p, "sleep 5")
	if err == nil {
		t.Error("expected error from cancelled context")
	}
	// Either DeadlineExceeded (if our wrapper observed it) or
	// context.Canceled wrapped — accept either.
	if !errors.Is(err, context.Canceled) && !strings.Contains(err.Error(), "timed out") && !strings.Contains(err.Error(), "exec") {
		t.Errorf("err = %v, want cancel-shaped error", err)
	}
}
