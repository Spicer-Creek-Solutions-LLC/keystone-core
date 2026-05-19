// Package gitops implements the `kscore-gitops` CLI (Epic 16 task
// 10). Reachable as `kscorectl gitops …` via the Epic-14 plugin
// dispatch.
package gitops

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os/exec"

	"go.keystone-core.io/keystone-core/internal/gitops/verification"
)

// ExecCommandRunner is the CLI's production implementation of
// [verification.CommandRunner]. It is intentionally kept out of the
// verifier package so that package needs no `os/exec` import. Honors
// ctx cancellation; ExitError is reported via CommandResult.ExitCode
// (a Go error means the runner itself failed to launch).
type ExecCommandRunner struct{}

// Run implements [verification.CommandRunner].
func (ExecCommandRunner) Run(ctx context.Context, req verification.CommandRequest) (verification.CommandResult, error) {
	// G204: this runner deliberately executes the operator-supplied
	// verification command — that is its only job. The risk is owned
	// by the operator writing the workflow; the CLI surface is local
	// to the operator.
	//nolint:gosec
	cmd := exec.CommandContext(ctx, req.Command, req.Args...)
	cmd.Dir = req.WorkingDir
	if len(req.Env) > 0 {
		envSlice := make([]string, 0, len(req.Env))
		for k, v := range req.Env {
			envSlice = append(envSlice, k+"="+v)
		}
		cmd.Env = envSlice
	}
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	res := verification.CommandResult{Stdout: stdout.String(), Stderr: stderr.String()}
	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			res.ExitCode = exitErr.ExitCode()
			return res, nil
		}
		return res, err
	}
	return res, nil
}

// io.Discard alias kept here for callers that want a no-op writer
// (unused in the runner itself).
var _ io.Writer = io.Discard
