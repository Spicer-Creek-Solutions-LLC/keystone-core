package steps

import (
	"context"
	"fmt"

	"go.keystone-core.io/keystone-core/internal/runbook"
)

// stateStep applies the state collection in config.state (inline YAML)
// via the injected StateApplier. Any failed declaration fails the
// step.
func (d Deps) stateStep(ctx context.Context, sc runbook.StepContext) (runbook.StepOutput, error) {
	if d.State == nil {
		return runbook.StepOutput{}, fmt.Errorf("%w: state", ErrStepNotConfigured)
	}
	src, err := cfgString(sc.Config, "state")
	if err != nil {
		return runbook.StepOutput{}, err
	}
	res, err := d.State.Apply(ctx, StateRequest{Source: []byte(src)})
	if err != nil {
		return runbook.StepOutput{}, fmt.Errorf("steps: state apply: %w", err)
	}
	out := runbook.StepOutput{Outputs: map[string]any{
		"changed": res.Changed,
		"failed":  res.Failed,
		"summary": res.Summary,
	}}
	if res.Failed > 0 {
		return out, fmt.Errorf("steps: state apply: %d declaration(s) failed", res.Failed)
	}
	return out, nil
}
