// SPDX-License-Identifier: Apache-2.0

package statemgmt

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"go.keystone-core.io/keystone-core/internal/state"
	"go.keystone-core.io/keystone-core/pkg/saga"
)

// SagaConfig enables saga mode for a [Runner.RunSaga] call.
//
// History is the source of prior-state lookups for compensation. The
// runner queries the most-recent completed [state.StateRunRecord]
// whose DeclarationsJSON contains a declaration with the matching
// decl ID; that declaration's params are re-applied via the
// module's Apply to roll the resource back. History MUST be
// non-nil.
//
// AgentID scopes the history search to runs that targeted a
// specific agent. Empty matches every run regardless of agent —
// fine for single-host setups and CI; production multi-agent
// deployments should set it so a rollback doesn't pull in a state
// last applied on a different host.
//
// ClusterID is reserved for future scoping (the
// [state.StateRunRecord] carries the field, but the v0.1
// [state.StateRunFilter] does not yet expose it for query-time
// filtering). The runner accepts it for forward-compatibility; it
// is currently ignored.
type SagaConfig struct {
	History   state.StateHistoryStore
	AgentID   string
	ClusterID string // reserved — not yet exposed by StateRunFilter
}

// HistorySearchLimit caps the number of historical runs the
// compensation lookup walks for each declaration. v0.1 default is
// generous; a v0.x roadmap entry (saga checkpoint-resume) will let
// the runner index history more intelligently.
const HistorySearchLimit = 200

// RunSaga executes decls in saga mode: on first failure, walks
// completed declarations in reverse re-applying the most recent
// successful prior state from cfg.History. The returned [RunReport]
// has the same shape as [Run]; per-decl [DeclarationResult] gains
// Compensated / CompensateError flags reflecting what the saga
// unwound. The aggregate saga outcome is encoded as:
//
//   - all decls succeeded → report identical to a clean [Run].
//   - failure mid-run, every compensation succeeded → returned
//     error is the originating decl's failure.
//   - failure mid-run, at least one compensation failed → returned
//     error joins the originating failure with every Compensate's
//     error (use [errors.Is] / [errors.As] to traverse).
//
// Cancellation: a canceled ctx is treated as the current decl's
// failure and triggers compensation. Compensation itself does NOT
// bail on ctx cancellation — every prior decl's Compensate runs.
//
// Returns an error if cfg.History is nil.
func (r *Runner) RunSaga(ctx context.Context, decls []*Declaration, cfg SagaConfig) (*RunReport, error) {
	if cfg.History == nil {
		return nil, errors.New("statemgmt: RunSaga requires SagaConfig.History")
	}
	obs := r.observer()
	reg := r.registry()

	report := &RunReport{
		Mode:      ModeApply,
		StartedAt: time.Now(),
	}
	defer func() {
		report.EndedAt = time.Now()
		report.Total = len(report.Results)
	}()

	// resultByID tracks each step's DeclarationResult so the
	// Compensate closure can mutate it (Compensated /
	// CompensateError).
	resultByID := make(map[string]*DeclarationResult, len(decls))

	// Build saga.Step per declaration. Action runs the existing
	// per-decl Check→Apply→Test; Compensate looks up + re-applies.
	steps := make([]saga.Step, 0, len(decls))
	for _, decl := range decls {
		if decl == nil {
			continue
		}
		decl := decl
		steps = append(steps, saga.Step{
			Name: decl.ID,
			Action: func(actCtx context.Context, _ any) (any, error) {
				res := r.runOneDecl(actCtx, decl, ModeApply, reg, obs)
				resultByID[decl.ID] = &res
				if res.Outcome == OutcomeFailed {
					// Make the saga see a real error so the
					// compensation walk fires. The error mirrors
					// res.Error.
					return nil, res.Error
				}
				return nil, nil
			},
			Compensate: func(compCtx context.Context, _ any) error {
				res := resultByID[decl.ID]
				if res == nil {
					// Shouldn't happen — Action records before
					// returning — but be defensive.
					return nil
				}
				res.Compensated = true
				prior, err := lookupPriorDecl(compCtx, cfg, decl.ID)
				if err != nil {
					res.CompensateError = fmt.Errorf("lookup prior state: %w", err)
					return res.CompensateError
				}
				if prior == nil {
					// No prior state to roll back to. Record the
					// attempt (Compensated=true) but report success
					// — there's nothing to undo.
					return nil
				}
				mod, err := reg.Get(prior.Module)
				if err != nil {
					res.CompensateError = fmt.Errorf("rollback module %q: %w", prior.Module, err)
					return res.CompensateError
				}
				if _, err := mod.Apply(compCtx, prior.moduleView()); err != nil {
					res.CompensateError = fmt.Errorf("rollback apply: %w", err)
					return res.CompensateError
				}
				return nil
			},
		})
	}

	coord := &saga.Coordinator{}
	exec, sagaErr := coord.Run(ctx, "statemgmt-run", steps, nil)

	// Stitch the saga's per-step outcomes back into the report.
	for _, decl := range decls {
		if decl == nil {
			continue
		}
		res, ok := resultByID[decl.ID]
		if !ok {
			// Step never ran (saga skipped it because an earlier
			// step failed). Synthesize the cascade-skipped row, same
			// as [Runner.Run] would.
			res = &DeclarationResult{
				DeclID:  decl.ID,
				Module:  decl.Module,
				Outcome: OutcomeSkipped,
				Error:   exec.Error,
			}
			obs.Skip(ctx, decl, exec.Error)
			resultByID[decl.ID] = res
		}
		report.Results = append(report.Results, *res)
		switch res.Outcome {
		case OutcomeChanged:
			report.Changed++
		case OutcomeUnchanged, OutcomeNoOp:
			report.Unchanged++
		case OutcomeFailed:
			report.Failed++
		case OutcomeSkipped:
			report.Skipped++
		}
	}

	return report, sagaErr
}

// runOneDecl is [Runner.runOne]'s body extracted so [RunSaga] can
// share the Check → Apply → Test pipeline. The two callers
// (runOne, RunSaga's Action) wrap it differently: runOne records
// straight into a report, RunSaga maps the result onto a
// [saga.Step].
func (r *Runner) runOneDecl(ctx context.Context, decl *Declaration, mode RunMode, reg *Registry, obs RunObserver) DeclarationResult {
	return r.runOne(ctx, decl, mode, reg, obs)
}

// lookupPriorDecl walks history newest-first for the most recent
// completed run whose DeclarationsJSON contains a declaration with
// the given ID. Returns (nil, nil) when no prior state exists —
// that's a normal case (the resource is being introduced for the
// first time). A real I/O error from the history store surfaces as
// (nil, error).
func lookupPriorDecl(ctx context.Context, cfg SagaConfig, declID string) (*Declaration, error) {
	runs, err := cfg.History.ListStateRuns(ctx, state.StateRunFilter{
		AgentID:    cfg.AgentID,
		Status:     state.StateRunStatusCompleted,
		Limit:      HistorySearchLimit,
		SortColumn: "started_at",
		SortDesc:   true, // newest-first
	})
	if err != nil {
		return nil, err
	}
	for _, run := range runs {
		// ClusterID post-hoc filter — StateRunFilter doesn't expose
		// the column query-side in v0.1. Empty cfg.ClusterID accepts
		// all runs.
		if cfg.ClusterID != "" && run.ClusterID != cfg.ClusterID {
			continue
		}
		if run.DeclarationsJSON == "" {
			continue
		}
		var decls []*Declaration
		if err := json.Unmarshal([]byte(run.DeclarationsJSON), &decls); err != nil {
			// A historical run with malformed JSON shouldn't stop
			// the rollback search; skip it and keep walking.
			continue
		}
		for _, d := range decls {
			if d != nil && d.ID == declID {
				return d, nil
			}
		}
	}
	return nil, nil
}
