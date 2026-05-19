package steps

import (
	"context"
	"fmt"

	"go.keystone-core.io/keystone-core/internal/runbook"
)

// queryStep runs a read-only lookup via the injected Querier.
// config: query (required), args (optional list passed through).
// Outputs: rows ([]map[string]any) and count.
func (d Deps) queryStep(ctx context.Context, sc runbook.StepContext) (runbook.StepOutput, error) {
	if d.Querier == nil {
		return runbook.StepOutput{}, fmt.Errorf("%w: query", ErrStepNotConfigured)
	}
	q, err := cfgString(sc.Config, "query")
	if err != nil {
		return runbook.StepOutput{}, err
	}
	var args []any
	if v, ok := sc.Config["args"]; ok {
		list, ok := v.([]any)
		if !ok {
			return runbook.StepOutput{}, fmt.Errorf("%w: args must be a list, got %T", ErrStepConfig, v)
		}
		args = list
	}
	rows, err := d.Querier.Query(ctx, q, args...)
	if err != nil {
		return runbook.StepOutput{}, fmt.Errorf("steps: query: %w", err)
	}
	return runbook.StepOutput{Outputs: map[string]any{
		"rows":  rows,
		"count": len(rows),
	}}, nil
}
