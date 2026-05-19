package verification

import (
	"context"
	"fmt"
	"time"
)

// maxCmdSnippet caps stdout/stderr retained in Result.Data.
const maxCmdSnippet = 4 << 10 // 4 KiB

// CommandRunner runs a single command. Implementations honour ctx
// cancellation and report command-level failure via
// CommandResult.ExitCode, not a Go error (a Go error means the runner
// itself failed to launch the process). A local copy of the runbook
// runner shape — this package owns no os/exec; the production runner
// is wired at the kscore-gitops CLI / boot layer (task 10).
type CommandRunner interface {
	Run(ctx context.Context, req CommandRequest) (CommandResult, error)
}

// CommandRequest is the input to a [CommandRunner].
type CommandRequest struct {
	Command    string
	Args       []string
	Env        map[string]string
	WorkingDir string
}

// CommandResult is the output of a [CommandRunner].
type CommandResult struct {
	ExitCode int
	Stdout   string
	Stderr   string
}

// CommandVerifier asserts a command exits with an expected code.
// Config:
//
//	command          (required) executable
//	args             ([]string)
//	env              (map[string]string)
//	working_dir      (string)
//	expect_exit_code (int, default 0)
//
// Runner is required; a nil Runner yields a failed Result (the
// verifier owns no process execution).
type CommandVerifier struct {
	Runner CommandRunner
}

// Type implements [Verifier].
func (CommandVerifier) Type() string { return "command" }

// Verify implements [Verifier]. Success when the command launches and
// its exit code equals expect_exit_code.
func (v CommandVerifier) Verify(ctx context.Context, step Step) Result {
	start := time.Now()

	if v.Runner == nil {
		return failf(start, ErrConfig, "command: no runner configured")
	}
	command, err := cfgString(step.Config, "command")
	if err != nil {
		return failf(start, err, "command: %v", err)
	}
	args, err := cfgStringSlice(step.Config, "args")
	if err != nil {
		return failf(start, err, "command: %v", err)
	}
	env, err := cfgStringMap(step.Config, "env")
	if err != nil {
		return failf(start, err, "command: %v", err)
	}
	expectCode, err := cfgIntOpt(step.Config, "expect_exit_code", 0)
	if err != nil {
		return failf(start, err, "command: %v", err)
	}

	res, runErr := v.Runner.Run(ctx, CommandRequest{
		Command:    command,
		Args:       args,
		Env:        env,
		WorkingDir: cfgStringOpt(step.Config, "working_dir", ""),
	})
	if runErr != nil {
		return failf(start, runErr, "command: launch failed: %v", runErr)
	}

	data := map[string]any{
		"exit_code": res.ExitCode,
		"stdout":    snippet(res.Stdout),
		"stderr":    snippet(res.Stderr),
	}
	if res.ExitCode != expectCode {
		r := failf(start, nil, "command: exit code %d, want %d", res.ExitCode, expectCode)
		r.Data = data
		return r
	}
	return Result{
		Success:  true,
		Message:  fmt.Sprintf("command: %s exited %d", command, res.ExitCode),
		Data:     data,
		Duration: time.Since(start),
	}
}

func snippet(s string) string {
	if len(s) > maxCmdSnippet {
		return s[:maxCmdSnippet]
	}
	return s
}
