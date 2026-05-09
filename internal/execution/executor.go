package execution

import (
	"context"
	"time"
)

// ExecuteRequest is the input to an Executor. Field shape mirrors
// internal/agent.ExecuteRequest so an existing Executor implementation
// adapts via a free conversion.
type ExecuteRequest struct {
	Command      string
	Args         []string
	Env          map[string]string
	EnvAllowlist []string
	WorkingDir   string
	User         string
	Timeout      time.Duration
	StdinInput   []byte
}

// ExecuteResult is the output of an Executor. System-level errors live
// in Error rather than a Go error return so the value can be serialized
// straight onto a response subject without losing information about a
// failed-but-completed exec.
type ExecuteResult struct {
	ExitCode        int
	Stdout          []byte
	Stderr          []byte
	Duration        time.Duration
	TimedOut        bool
	StdoutTruncated bool
	StderrTruncated bool
	Error           string
}

// Succeeded reports whether r is a clean success: exit 0, no system
// error, no timeout. Non-zero exits are failures even if Error is
// empty.
func (r ExecuteResult) Succeeded() bool {
	return r.ExitCode == 0 && r.Error == "" && !r.TimedOut
}

// Executor runs a single command attempt synchronously. Implementations
// must always return a populated ExecuteResult — see ExecuteResult.Error.
type Executor interface {
	Execute(ctx context.Context, req ExecuteRequest) ExecuteResult
}

// ExecutorFunc adapts a plain function into an Executor.
type ExecutorFunc func(ctx context.Context, req ExecuteRequest) ExecuteResult

// Execute satisfies Executor.
func (f ExecutorFunc) Execute(ctx context.Context, req ExecuteRequest) ExecuteResult {
	return f(ctx, req)
}
