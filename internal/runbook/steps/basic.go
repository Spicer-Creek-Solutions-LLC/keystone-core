// SPDX-License-Identifier: Apache-2.0

package steps

import (
	"context"
	"errors"
	"fmt"

	"go.keystone-core.io/keystone-core/internal/runbook"
)

// noopStep does nothing and succeeds. If config.outputs is a map it
// is returned verbatim — useful as a templating anchor or placeholder.
func noopStep(_ context.Context, sc runbook.StepContext) (runbook.StepOutput, error) {
	if v, ok := sc.Config["outputs"]; ok {
		if m, ok := v.(map[string]any); ok {
			return runbook.StepOutput{Outputs: m}, nil
		}
	}
	return runbook.StepOutput{}, nil
}

// failStep always fails with config.message (default generic). Used
// for explicit aborts and exercising the OnFailure chain.
func failStep(_ context.Context, sc runbook.StepContext) (runbook.StepOutput, error) {
	msg := cfgStringOpt(sc.Config, "message", "runbook: fail step")
	return runbook.StepOutput{}, errors.New(msg)
}

// waitStep sleeps for config.duration ("30s"), honouring ctx.
func (d Deps) waitStep(ctx context.Context, sc runbook.StepContext) (runbook.StepOutput, error) {
	dur, err := cfgDurationOpt(sc.Config, "duration")
	if err != nil {
		return runbook.StepOutput{}, err
	}
	if dur <= 0 {
		return runbook.StepOutput{}, fmt.Errorf("%w: %q is required and must be > 0", ErrStepConfig, "duration")
	}
	if err := d.sleep(ctx, dur); err != nil {
		return runbook.StepOutput{}, err
	}
	return runbook.StepOutput{Outputs: map[string]any{"waited": dur.String()}}, nil
}
