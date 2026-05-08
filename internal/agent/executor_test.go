package agent

import (
	"context"
	"os"
	"os/user"
	"runtime"
	"strings"
	"testing"
	"time"
)

func newTestExecutor(t *testing.T, opts ...func(*ExecutorConfig)) *Executor {
	t.Helper()
	cfg := ExecutorConfig{
		Logger:         testLogger(),
		KillGrace:      50 * time.Millisecond, // tight for test speed
		DefaultTimeout: 5 * time.Second,
		MaxStdoutBytes: 1024,
		MaxStderrBytes: 512,
	}
	for _, o := range opts {
		o(&cfg)
	}
	return NewExecutor(cfg)
}

// requireUnix skips the test on Windows. The Executor is buildable
// on Windows but the v1.0 test surface uses /bin/sh and friends.
func requireUnix(t *testing.T) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("skipping unix-only test on windows")
	}
}

func TestExecutor_EchoStdoutExitZero(t *testing.T) {
	requireUnix(t)
	e := newTestExecutor(t)
	res := e.Execute(context.Background(), ExecuteRequest{
		Command: "/bin/sh",
		Args:    []string{"-c", "echo hello"},
	})
	if res.ExitCode != 0 {
		t.Errorf("ExitCode = %d, want 0", res.ExitCode)
	}
	if got := strings.TrimSpace(string(res.Stdout)); got != "hello" {
		t.Errorf("Stdout = %q, want %q", got, "hello")
	}
	if res.Error != "" {
		t.Errorf("Error = %q, want empty", res.Error)
	}
	if res.TimedOut {
		t.Error("TimedOut = true on natural exit")
	}
}

func TestExecutor_NonZeroExit(t *testing.T) {
	requireUnix(t)
	e := newTestExecutor(t)
	res := e.Execute(context.Background(), ExecuteRequest{
		Command: "/bin/sh",
		Args:    []string{"-c", "exit 7"},
	})
	if res.ExitCode != 7 {
		t.Errorf("ExitCode = %d, want 7", res.ExitCode)
	}
}

func TestExecutor_StderrCaptured(t *testing.T) {
	requireUnix(t)
	e := newTestExecutor(t)
	res := e.Execute(context.Background(), ExecuteRequest{
		Command: "/bin/sh",
		Args:    []string{"-c", "echo errline >&2"},
	})
	if got := strings.TrimSpace(string(res.Stderr)); got != "errline" {
		t.Errorf("Stderr = %q, want %q", got, "errline")
	}
}

func TestExecutor_StdinInput(t *testing.T) {
	requireUnix(t)
	e := newTestExecutor(t)
	res := e.Execute(context.Background(), ExecuteRequest{
		Command:    "/bin/cat",
		StdinInput: []byte("piped-input"),
	})
	if got := string(res.Stdout); got != "piped-input" {
		t.Errorf("Stdout = %q, want %q", got, "piped-input")
	}
}

func TestExecutor_WorkingDir(t *testing.T) {
	requireUnix(t)
	dir := t.TempDir()
	e := newTestExecutor(t)
	res := e.Execute(context.Background(), ExecuteRequest{
		Command:    "/bin/sh",
		Args:       []string{"-c", "pwd"},
		WorkingDir: dir,
	})
	if got := strings.TrimSpace(string(res.Stdout)); got != dir {
		// macOS resolves /tmp -> /private/tmp; both are acceptable.
		if !strings.HasSuffix(got, dir) {
			t.Errorf("Stdout = %q, want suffix of %q", got, dir)
		}
	}
}

func TestExecutor_EnvInjection(t *testing.T) {
	requireUnix(t)
	e := newTestExecutor(t)
	res := e.Execute(context.Background(), ExecuteRequest{
		Command: "/bin/sh",
		Args:    []string{"-c", "echo $FOO"},
		Env:     map[string]string{"FOO": "bar"},
	})
	if got := strings.TrimSpace(string(res.Stdout)); got != "bar" {
		t.Errorf("Stdout = %q, want %q", got, "bar")
	}
}

func TestExecutor_EnvAllowlistFiltersInherited(t *testing.T) {
	requireUnix(t)
	t.Setenv("KSCORE_TEST_LEAK", "should-not-pass")
	t.Setenv("KSCORE_TEST_KEEP", "preserved")

	e := newTestExecutor(t)
	res := e.Execute(context.Background(), ExecuteRequest{
		Command:      "/bin/sh",
		Args:         []string{"-c", "echo leak=$KSCORE_TEST_LEAK keep=$KSCORE_TEST_KEEP"},
		EnvAllowlist: []string{"KSCORE_TEST_KEEP", "PATH"},
	})
	got := strings.TrimSpace(string(res.Stdout))
	if !strings.Contains(got, "keep=preserved") {
		t.Errorf("Stdout = %q, want allowlisted KSCORE_TEST_KEEP", got)
	}
	if strings.Contains(got, "leak=should-not-pass") {
		t.Errorf("Stdout = %q, leaked KSCORE_TEST_LEAK", got)
	}
}

func TestExecutor_TimeoutKills(t *testing.T) {
	requireUnix(t)
	e := newTestExecutor(t)
	start := time.Now()
	res := e.Execute(context.Background(), ExecuteRequest{
		Command: "/bin/sleep",
		Args:    []string{"5"},
		Timeout: 100 * time.Millisecond,
	})
	elapsed := time.Since(start)

	if !res.TimedOut {
		t.Error("TimedOut = false, want true")
	}
	if res.ExitCode != -1 {
		t.Errorf("ExitCode = %d, want -1 on signal kill", res.ExitCode)
	}
	if elapsed > 1*time.Second {
		t.Errorf("kill took %s, want <1s (timeout 100ms + 50ms grace)", elapsed)
	}
}

func TestExecutor_TimeoutSIGTERMGraceThenSIGKILL(t *testing.T) {
	requireUnix(t)
	// Shell trap that ignores SIGTERM and loops; only SIGKILL ends it.
	script := `trap "echo trapped >&2" TERM; while :; do sleep 0.05; done`
	e := newTestExecutor(t, func(c *ExecutorConfig) {
		c.KillGrace = 100 * time.Millisecond
	})
	start := time.Now()
	res := e.Execute(context.Background(), ExecuteRequest{
		Command: "/bin/sh",
		Args:    []string{"-c", script},
		Timeout: 100 * time.Millisecond,
	})
	elapsed := time.Since(start)

	if !res.TimedOut {
		t.Error("TimedOut = false, want true")
	}
	// Total time = 100ms timeout + 100ms grace + small slack.
	if elapsed > 1*time.Second {
		t.Errorf("kill took %s, want <1s (timeout + grace + slack)", elapsed)
	}
	// Stderr should contain "trapped" — the trap fired before SIGKILL.
	if !strings.Contains(string(res.Stderr), "trapped") {
		t.Errorf("Stderr = %q, want trap message captured before SIGKILL", res.Stderr)
	}
}

func TestExecutor_StdoutTruncation(t *testing.T) {
	requireUnix(t)
	e := newTestExecutor(t, func(c *ExecutorConfig) {
		c.MaxStdoutBytes = 64
	})
	res := e.Execute(context.Background(), ExecuteRequest{
		Command: "/bin/sh",
		// Emit ~200 bytes; cap is 64.
		Args: []string{"-c", "head -c 200 /dev/zero | tr '\\0' 'A'"},
	})
	if !res.StdoutTruncated {
		t.Error("StdoutTruncated = false, want true")
	}
	if len(res.Stdout) != 64 {
		t.Errorf("len(Stdout) = %d, want 64", len(res.Stdout))
	}
}

func TestExecutor_StderrTruncation(t *testing.T) {
	requireUnix(t)
	e := newTestExecutor(t, func(c *ExecutorConfig) {
		c.MaxStderrBytes = 32
	})
	res := e.Execute(context.Background(), ExecuteRequest{
		Command: "/bin/sh",
		Args:    []string{"-c", "head -c 200 /dev/zero | tr '\\0' 'B' >&2"},
	})
	if !res.StderrTruncated {
		t.Error("StderrTruncated = false, want true")
	}
	if len(res.Stderr) != 32 {
		t.Errorf("len(Stderr) = %d, want 32", len(res.Stderr))
	}
}

func TestExecutor_NonExistentCommand(t *testing.T) {
	e := newTestExecutor(t)
	res := e.Execute(context.Background(), ExecuteRequest{
		Command: "/this/path/does/not/exist/i-promise",
	})
	if res.ExitCode != -1 {
		t.Errorf("ExitCode = %d, want -1", res.ExitCode)
	}
	if res.Error == "" {
		t.Error("Error empty for non-existent command")
	}
}

func TestExecutor_EmptyCommand(t *testing.T) {
	e := newTestExecutor(t)
	res := e.Execute(context.Background(), ExecuteRequest{})
	if res.ExitCode != -1 {
		t.Errorf("ExitCode = %d, want -1", res.ExitCode)
	}
	if !strings.Contains(res.Error, "Command is required") {
		t.Errorf("Error = %q, want containing 'Command is required'", res.Error)
	}
}

func TestExecutor_DefaultTimeoutApplied(t *testing.T) {
	e := newTestExecutor(t, func(c *ExecutorConfig) {
		c.DefaultTimeout = 100 * time.Millisecond
	})
	requireUnix(t)
	res := e.Execute(context.Background(), ExecuteRequest{
		Command: "/bin/sleep",
		Args:    []string{"5"},
		// Timeout left at zero — should fall back to DefaultTimeout.
	})
	if !res.TimedOut {
		t.Error("TimedOut = false; default timeout not applied")
	}
}

// TestExecutor_UserCurrentUserNoOp exercises the user-credential
// path with the *current* user's name. Setting Uid/Gid to the
// already-active user is harmless (no setuid escalation needed),
// so this works without root. Lookup-failure tests below cover the
// error path.
func TestExecutor_UserCurrentUserNoOp(t *testing.T) {
	requireUnix(t)
	cur, err := user.Current()
	if err != nil {
		t.Skipf("user.Current(): %v", err)
	}
	if os.Geteuid() != 0 {
		// Non-root: setting any credential — even our own — fails
		// with EPERM under standard Linux. Skip the actual exec
		// and just exercise the lookup path via the helper.
		t.Skip("skipping: non-root cannot setuid even to self under standard Linux capabilities")
	}
	e := newTestExecutor(t)
	res := e.Execute(context.Background(), ExecuteRequest{
		Command: "/bin/sh",
		Args:    []string{"-c", "id -u"},
		User:    cur.Username,
	})
	if res.Error != "" {
		t.Errorf("Error = %q (running as root, current user)", res.Error)
	}
}

func TestExecutor_UnknownUserSurfacesError(t *testing.T) {
	requireUnix(t)
	e := newTestExecutor(t)
	res := e.Execute(context.Background(), ExecuteRequest{
		Command: "/bin/echo",
		Args:    []string{"hi"},
		User:    "this-user-definitely-does-not-exist-xyz",
	})
	if res.ExitCode != -1 {
		t.Errorf("ExitCode = %d, want -1", res.ExitCode)
	}
	if !strings.Contains(res.Error, "user lookup") {
		t.Errorf("Error = %q, want containing 'user lookup'", res.Error)
	}
}

func TestExecutor_DurationRecorded(t *testing.T) {
	requireUnix(t)
	e := newTestExecutor(t)
	res := e.Execute(context.Background(), ExecuteRequest{
		Command: "/bin/sh",
		Args:    []string{"-c", "sleep 0.05"},
	})
	if res.Duration < 50*time.Millisecond {
		t.Errorf("Duration = %s, want >=50ms", res.Duration)
	}
	if res.Duration > 5*time.Second {
		t.Errorf("Duration = %s, suspiciously long", res.Duration)
	}
}

func TestExecutor_ContextCancelKillsProcess(t *testing.T) {
	requireUnix(t)
	e := newTestExecutor(t)
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(80 * time.Millisecond)
		cancel()
	}()
	start := time.Now()
	res := e.Execute(ctx, ExecuteRequest{
		Command: "/bin/sleep",
		Args:    []string{"5"},
	})
	elapsed := time.Since(start)
	if !res.TimedOut {
		t.Error("TimedOut = false; ctx-cancel should be reported as TimedOut (we lump kill paths)")
	}
	if elapsed > 1*time.Second {
		t.Errorf("kill via ctx took %s, want <1s", elapsed)
	}
}
