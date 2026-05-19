package steps

import (
	"context"
	"fmt"

	"go.keystone-core.io/keystone-core/internal/runbook"
)

// commandStep runs config.command with config.args/env/working_dir/
// timeout via the injected CommandRunner. A non-zero exit code is a
// step failure (stderr surfaced in the error).
func (d Deps) commandStep(ctx context.Context, sc runbook.StepContext) (runbook.StepOutput, error) {
	if d.Command == nil {
		return runbook.StepOutput{}, fmt.Errorf("%w: command", ErrStepNotConfigured)
	}
	cmd, err := cfgString(sc.Config, "command")
	if err != nil {
		return runbook.StepOutput{}, err
	}
	args, err := cfgStringSlice(sc.Config, "args")
	if err != nil {
		return runbook.StepOutput{}, err
	}
	env, err := cfgStringMap(sc.Config, "env")
	if err != nil {
		return runbook.StepOutput{}, err
	}
	to, err := cfgDurationOpt(sc.Config, "timeout")
	if err != nil {
		return runbook.StepOutput{}, err
	}
	req := CommandRequest{
		Command:    cmd,
		Args:       args,
		Env:        env,
		WorkingDir: cfgStringOpt(sc.Config, "working_dir", ""),
		Timeout:    to,
	}
	return runCommand(ctx, d.Command, req)
}

// scriptStep runs an inline config.script body through an interpreter
// (config.interpreter, default "sh"; config.interpreter_args, default
// ["-c"]) via the injected CommandRunner.
func (d Deps) scriptStep(ctx context.Context, sc runbook.StepContext) (runbook.StepOutput, error) {
	if d.Command == nil {
		return runbook.StepOutput{}, fmt.Errorf("%w: script", ErrStepNotConfigured)
	}
	script, err := cfgString(sc.Config, "script")
	if err != nil {
		return runbook.StepOutput{}, err
	}
	interp := cfgStringOpt(sc.Config, "interpreter", "sh")
	iargs, err := cfgStringSlice(sc.Config, "interpreter_args")
	if err != nil {
		return runbook.StepOutput{}, err
	}
	if iargs == nil {
		iargs = []string{"-c"}
	}
	to, err := cfgDurationOpt(sc.Config, "timeout")
	if err != nil {
		return runbook.StepOutput{}, err
	}
	req := CommandRequest{
		Command: interp,
		Args:    append(append([]string{}, iargs...), script),
		Timeout: to,
	}
	return runCommand(ctx, d.Command, req)
}

func runCommand(ctx context.Context, runner CommandRunner, req CommandRequest) (runbook.StepOutput, error) {
	res, err := runner.Run(ctx, req)
	if err != nil {
		return runbook.StepOutput{}, fmt.Errorf("steps: command runner: %w", err)
	}
	out := runbook.StepOutput{Outputs: map[string]any{
		"exit_code": res.ExitCode,
		"stdout":    res.Stdout,
		"stderr":    res.Stderr,
	}}
	if res.ExitCode != 0 {
		return out, fmt.Errorf("steps: command exited %d: %s", res.ExitCode, res.Stderr)
	}
	return out, nil
}
