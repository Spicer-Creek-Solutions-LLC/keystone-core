// SPDX-License-Identifier: Apache-2.0

package agent

import (
	"bytes"
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"strings"
	"sync"
	"syscall"
	"time"
)

// ExecuteRequest is the agent-side command-execution input. Task 5
// will wire this from the inbound NATS command envelope; Task 4's
// SecurityEnforcer pre-filters the request before it reaches
// Execute.
type ExecuteRequest struct {
	Command      string // executable name or absolute path
	Args         []string
	Env          map[string]string // injected env (overlaid on inherited)
	EnvAllowlist []string          // if non-empty, ONLY these inherited keys pass through
	WorkingDir   string            // pwd; empty inherits parent
	User         string            // username to switch to; empty = current user
	Timeout      time.Duration     // 0 = ExecutorConfig.DefaultTimeout
	StdinInput   []byte            // optional stdin payload
}

// ExecuteResult carries the structured outcome. Real Go errors from
// the wrapper itself (lookup failure, fork failure, SysProcAttr
// denied) populate Error so the caller can serialize the result
// onto the response subject without losing information about a
// failed-but-completed exec.
type ExecuteResult struct {
	ExitCode        int    // process exit code; -1 on signal kill or fork failure
	Stdout          []byte // captured stdout (truncated if exceeded MaxStdoutBytes)
	Stderr          []byte // captured stderr (truncated if exceeded MaxStderrBytes)
	Duration        time.Duration
	TimedOut        bool // true if killed by timeout
	StdoutTruncated bool
	StderrTruncated bool
	Error           string // system-level error string; empty on natural exit
}

// ExecutorConfig configures an Executor. Defaults match
// PROJECT-DETAILS §4.7: 5s kill grace; 1 MiB stdout cap; 256 KiB
// stderr cap.
type ExecutorConfig struct {
	Logger         *slog.Logger
	KillGrace      time.Duration
	DefaultTimeout time.Duration
	MaxStdoutBytes int
	MaxStderrBytes int
	Now            func() time.Time
}

const (
	defaultKillGrace      = 5 * time.Second
	defaultExecTimeout    = 5 * time.Minute
	defaultMaxStdoutBytes = 1 << 20   // 1 MiB
	defaultMaxStderrBytes = 256 << 10 // 256 KiB
)

// Executor wraps os/exec with the v1.0 agent contract: timeout, env
// injection, working dir, user switching (Linux uid/gid), output
// capture with hard-cap truncation, and the SIGTERM-grace-then-
// SIGKILL kill protocol.
//
// Stateless except for configuration; safe for concurrent use.
// Task 5 wires this into the Agent's command-handler pipeline.
type Executor struct {
	log            *slog.Logger
	killGrace      time.Duration
	defaultTimeout time.Duration
	maxStdoutBytes int
	maxStderrBytes int
	now            func() time.Time
}

// NewExecutor returns an Executor with the given config. Zero-value
// fields fall back to PROJECT-DETAILS §4.7 defaults.
func NewExecutor(cfg ExecutorConfig) *Executor {
	if cfg.Logger == nil {
		cfg.Logger = slog.Default()
	}
	if cfg.KillGrace == 0 {
		cfg.KillGrace = defaultKillGrace
	}
	if cfg.DefaultTimeout == 0 {
		cfg.DefaultTimeout = defaultExecTimeout
	}
	if cfg.MaxStdoutBytes == 0 {
		cfg.MaxStdoutBytes = defaultMaxStdoutBytes
	}
	if cfg.MaxStderrBytes == 0 {
		cfg.MaxStderrBytes = defaultMaxStderrBytes
	}
	if cfg.Now == nil {
		cfg.Now = time.Now
	}
	return &Executor{
		log:            cfg.Logger,
		killGrace:      cfg.KillGrace,
		defaultTimeout: cfg.DefaultTimeout,
		maxStdoutBytes: cfg.MaxStdoutBytes,
		maxStderrBytes: cfg.MaxStderrBytes,
		now:            cfg.Now,
	}
}

// Execute runs req synchronously and returns the structured result.
// Always returns a populated ExecuteResult — system-level errors
// surface in Result.Error rather than as a Go error so callers can
// serialize the outcome onto a response subject without an out-of-
// band path.
func (e *Executor) Execute(ctx context.Context, req ExecuteRequest) ExecuteResult {
	start := e.now()
	timeout := req.Timeout
	if timeout == 0 {
		timeout = e.defaultTimeout
	}

	if req.Command == "" {
		return ExecuteResult{
			ExitCode: -1,
			Duration: e.now().Sub(start),
			Error:    "executor: Command is required",
		}
	}

	stdoutBuf := newCappedBuffer(e.maxStdoutBytes)
	stderrBuf := newCappedBuffer(e.maxStderrBytes)

	execCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	cmd := exec.Command(req.Command, req.Args...) //nolint:gosec // command + args are policed by SecurityEnforcer (Task 4) upstream
	cmd.Env = buildEnv(req.Env, req.EnvAllowlist)
	if req.WorkingDir != "" {
		cmd.Dir = req.WorkingDir
	}
	cmd.Stdout = stdoutBuf
	cmd.Stderr = stderrBuf
	if len(req.StdinInput) > 0 {
		cmd.Stdin = bytes.NewReader(req.StdinInput)
	}

	if req.User != "" {
		if err := applyUserCredential(cmd, req.User); err != nil {
			return ExecuteResult{
				ExitCode: -1,
				Duration: e.now().Sub(start),
				Error:    err.Error(),
			}
		}
	}

	if err := cmd.Start(); err != nil {
		return ExecuteResult{
			ExitCode:        -1,
			Stdout:          stdoutBuf.bytes(),
			Stderr:          stderrBuf.bytes(),
			Duration:        e.now().Sub(start),
			StdoutTruncated: stdoutBuf.truncated,
			StderrTruncated: stderrBuf.truncated,
			Error:           fmt.Sprintf("executor: start: %v", err),
		}
	}

	timedOut := e.waitWithKillProtocol(execCtx, cmd)

	exitCode := -1
	if cmd.ProcessState != nil {
		exitCode = cmd.ProcessState.ExitCode()
	}

	result := ExecuteResult{
		ExitCode:        exitCode,
		Stdout:          stdoutBuf.bytes(),
		Stderr:          stderrBuf.bytes(),
		Duration:        e.now().Sub(start),
		TimedOut:        timedOut,
		StdoutTruncated: stdoutBuf.truncated,
		StderrTruncated: stderrBuf.truncated,
	}
	return result
}

// waitWithKillProtocol blocks on cmd.Wait. On execCtx cancellation
// (timeout or parent cancel), sends SIGTERM, waits killGrace, then
// SIGKILL if still running. Returns true if the kill came from
// execCtx (i.e., a timeout / cancel kill rather than natural exit).
func (e *Executor) waitWithKillProtocol(execCtx context.Context, cmd *exec.Cmd) bool {
	done := make(chan struct{})
	go func() {
		_ = cmd.Wait()
		close(done)
	}()

	select {
	case <-done:
		return false
	case <-execCtx.Done():
		// SIGTERM first, give the process a chance to clean up.
		if cmd.Process != nil {
			_ = cmd.Process.Signal(syscall.SIGTERM)
		}
		select {
		case <-done:
			return true
		case <-time.After(e.killGrace):
			if cmd.Process != nil {
				_ = cmd.Process.Kill()
			}
			<-done
			return true
		}
	}
}

// buildEnv composes the subprocess environment. EnvAllowlist filters
// the inherited env; req.Env overlays on top (req.Env wins on
// collision).
func buildEnv(injected map[string]string, allowlist []string) []string {
	var inherited []string
	if len(allowlist) == 0 {
		inherited = os.Environ()
	} else {
		want := make(map[string]struct{}, len(allowlist))
		for _, k := range allowlist {
			want[k] = struct{}{}
		}
		for _, kv := range os.Environ() {
			if i := strings.IndexByte(kv, '='); i > 0 {
				if _, ok := want[kv[:i]]; ok {
					inherited = append(inherited, kv)
				}
			}
		}
	}

	// Overlay injected vars; later entries win in os/exec, so put
	// injected after inherited.
	out := make([]string, 0, len(inherited)+len(injected))
	out = append(out, inherited...)
	for k, v := range injected {
		out = append(out, k+"="+v)
	}
	return out
}

// cappedBuffer is a bytes.Buffer with a hard write cap. Writes
// beyond the cap are silently discarded and the truncated flag is
// set. Returned to the caller as []byte via bytes().
type cappedBuffer struct {
	mu        sync.Mutex
	buf       bytes.Buffer
	cap       int
	truncated bool
}

func newCappedBuffer(cap int) *cappedBuffer {
	return &cappedBuffer{cap: cap}
}

// Write satisfies io.Writer. Returns len(p) even when bytes are
// dropped — exec/os doesn't care, and reporting a short write would
// trigger spurious "io.ErrShortWrite" in some pipelines.
func (b *cappedBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	remaining := b.cap - b.buf.Len()
	if remaining <= 0 {
		b.truncated = true
		return len(p), nil
	}
	if len(p) > remaining {
		b.buf.Write(p[:remaining])
		b.truncated = true
		return len(p), nil
	}
	return b.buf.Write(p)
}

func (b *cappedBuffer) bytes() []byte {
	b.mu.Lock()
	defer b.mu.Unlock()
	out := make([]byte, b.buf.Len())
	copy(out, b.buf.Bytes())
	return out
}
