// SPDX-License-Identifier: Apache-2.0

package cmd

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"strings"
	"time"
)

// outputCap bounds how much stdout/stderr the module captures per
// command. Matches Epic 07's per-command output policy (PROJECT-
// DETAILS §4.7 — stdout 1 MiB / stderr 256 KiB) at a smaller scale
// suitable for declarative state, where a command's output is only
// surfaced as a Comment/Diff for humans, not piped onward.
const outputCap = 64 * 1024 // 64 KiB

// commandOutcome is the result of one /bin/sh -c invocation.
type commandOutcome struct {
	ExitCode int
	Stdout   string
	Stderr   string
	TimedOut bool
}

// String produces a short multi-line summary suitable for the
// StateResult.Comment / Diff fields. Truncates long stdout/stderr.
func (o commandOutcome) String() string {
	var b strings.Builder
	fmt.Fprintf(&b, "exit=%d", o.ExitCode)
	if o.TimedOut {
		b.WriteString(" (timed out)")
	}
	if s := strings.TrimSpace(o.Stdout); s != "" {
		fmt.Fprintf(&b, " stdout=%q", truncate(s, 200))
	}
	if s := strings.TrimSpace(o.Stderr); s != "" {
		fmt.Fprintf(&b, " stderr=%q", truncate(s, 200))
	}
	return b.String()
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	if n <= 3 {
		return s[:n]
	}
	return s[:n-3] + "..."
}

// runShell executes `sh -c <command>` honouring cwd / env / timeout.
// Returns a populated commandOutcome (even when the command failed)
// alongside a Go error for setup-time failures (cwd missing, etc).
// Non-zero exits are NOT Go errors — the caller branches on
// outcome.ExitCode.
func runShell(ctx context.Context, p *params, command string) (commandOutcome, error) {
	var out commandOutcome

	timeout := time.Duration(p.TimeoutSeconds) * time.Second
	if timeout <= 0 {
		timeout = defaultTimeoutSeconds * time.Second
	}
	cmdCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	c := exec.CommandContext(cmdCtx, p.Shell, "-c", command) //nolint:gosec // command is operator-declared; running it IS the contract
	if p.Cwd != "" {
		c.Dir = p.Cwd
	}
	if len(p.Env) > 0 {
		// Merge operator-supplied env onto the inherited process
		// env. Operator entries win on key collision.
		env := os.Environ()
		for k, v := range p.Env {
			env = append(env, k+"="+v)
		}
		c.Env = env
	}

	var stdout, stderr cappedBuffer
	stdout.cap = outputCap
	stderr.cap = outputCap
	c.Stdout = &stdout
	c.Stderr = &stderr

	err := c.Run()
	out.Stdout = stdout.String()
	out.Stderr = stderr.String()

	// Detect timeout vs other errors.
	if errors.Is(cmdCtx.Err(), context.DeadlineExceeded) {
		out.TimedOut = true
		out.ExitCode = -1
		return out, fmt.Errorf("command timed out after %s", timeout)
	}

	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			out.ExitCode = exitErr.ExitCode()
			return out, nil
		}
		// Setup-time failure: shell not found, cwd missing, etc.
		if errors.Is(err, fs.ErrNotExist) {
			return out, fmt.Errorf("exec setup: %w", err)
		}
		out.ExitCode = -1
		return out, fmt.Errorf("exec: %w", err)
	}
	out.ExitCode = 0
	return out, nil
}

// guardDecision is the consolidated answer from evaluating the
// three guard types for one Check.
type guardDecision int

const (
	guardRun  guardDecision = iota // command must run (Matches=false)
	guardSkip                      // command must NOT run (Matches=true)
)

// evaluateGuards runs creates / onlyif / unless in that order
// (cheapest first). Returns guardSkip on first guard that says
// "don't run"; guardRun only when ALL guards permit.
//
// Semantics:
//
//	creates X: skip iff X exists
//	onlyif C:  skip iff C exits non-zero (precondition unmet)
//	unless C:  skip iff C exits zero (target already converged)
//
// The detail string accumulates each guard's verdict so an
// operator inspecting Check output sees WHY the decision came out
// the way it did.
func evaluateGuards(ctx context.Context, p *params) (guardDecision, string, error) {
	var notes []string

	if p.Creates != "" {
		exists, err := pathExists(p.Creates)
		if err != nil {
			return guardRun, "", fmt.Errorf("creates: %w", err)
		}
		if exists {
			return guardSkip, fmt.Sprintf("creates %q exists", p.Creates), nil
		}
		notes = append(notes, fmt.Sprintf("creates %q missing", p.Creates))
	}

	if p.OnlyIf != "" {
		out, err := runShell(ctx, p, p.OnlyIf)
		if err != nil && !out.TimedOut && out.ExitCode == -1 {
			return guardRun, "", fmt.Errorf("onlyif: %w", err)
		}
		if out.ExitCode != 0 {
			return guardSkip, fmt.Sprintf("onlyif failed (exit=%d)", out.ExitCode), nil
		}
		notes = append(notes, "onlyif passed")
	}

	if p.Unless != "" {
		out, err := runShell(ctx, p, p.Unless)
		if err != nil && !out.TimedOut && out.ExitCode == -1 {
			return guardRun, "", fmt.Errorf("unless: %w", err)
		}
		if out.ExitCode == 0 {
			return guardSkip, "unless succeeded (already converged)", nil
		}
		notes = append(notes, fmt.Sprintf("unless failed (exit=%d)", out.ExitCode))
	}

	return guardRun, strings.Join(notes, "; "), nil
}

// pathExists reports whether path exists. Distinguishes "not
// exist" from "permission denied" (latter surfaces as error so the
// operator hears about it rather than silently treating as missing).
func pathExists(path string) (bool, error) {
	_, err := os.Lstat(path)
	if err == nil {
		return true, nil
	}
	if errors.Is(err, fs.ErrNotExist) {
		return false, nil
	}
	return false, err
}

// cappedBuffer is a bytes.Buffer that drops writes once it hits
// `cap` bytes. Subsequent writes still "succeed" (no error) so the
// child process doesn't block on pipe back-pressure, but only the
// first `cap` bytes appear in String().
type cappedBuffer struct {
	buf bytes.Buffer
	cap int
}

func (b *cappedBuffer) Write(p []byte) (int, error) {
	remaining := b.cap - b.buf.Len()
	if remaining <= 0 {
		return len(p), nil
	}
	if len(p) > remaining {
		_, _ = b.buf.Write(p[:remaining])
		return len(p), nil
	}
	return b.buf.Write(p)
}

func (b *cappedBuffer) String() string {
	if b.buf.Len() >= b.cap {
		return b.buf.String() + "\n[output truncated]"
	}
	return b.buf.String()
}
