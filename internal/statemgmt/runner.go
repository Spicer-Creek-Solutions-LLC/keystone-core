// SPDX-License-Identifier: Apache-2.0

package statemgmt

import (
	"context"
	"errors"
	"fmt"
	"time"
)

// Runner orchestrates Check → Apply → Test per declaration in the
// topo order produced by Resolver.Resolve. Per PROJECT-DETAILS §4.8:
//
//	6. Check phase   — Module.Check (read-only diff)
//	7. Apply phase   — Module.Apply only when Check.Matches=false
//	8. Test phase    — Module.Test verifies post-apply
//	9. Report        — emit StateResult per state; emit events
//
// The Runner is intentionally narrow. It does not own the upstream
// pipeline (Parse → Load → Render → Validate → Resolve) — the caller
// wires those phases so each one stays inspectable.
//
// Drift severity / DriftReport (Task 7), history (Task 8), gRPC and
// CLI surfaces (9/10), real stdlib modules (11), and saga
// integration (12) layer on top.
type Runner struct {
	Registry    *Registry
	Observer    RunObserver
	DeclTimeout time.Duration // 0 → no per-decl timeout; parent ctx still applies
	Metrics     *Metrics      // optional; nil disables emission
}

// NewRunner returns a Runner. Pass nil for reg to fall back to
// DefaultRegistry; pass nil for obs to get a no-op observer.
func NewRunner(reg *Registry, obs RunObserver) *Runner {
	return &Runner{Registry: reg, Observer: obs}
}

// RunMode tags a RunReport with the phase the runner executed.
// "apply" is the full Check → Apply → Test pipeline; "check" is the
// dry-run that stops at Check.
type RunMode string

const (
	ModeApply RunMode = "apply"
	ModeCheck RunMode = "check"
)

// Outcome classifies what happened to one declaration.
type Outcome int

const (
	OutcomeUnchanged     Outcome = iota // Check matched; Apply skipped
	OutcomeChanged                      // Apply succeeded with Changed=true
	OutcomeNoOp                         // Apply succeeded with Changed=false
	OutcomeFailed                       // Error in Check / Apply / Test
	OutcomeDriftDetected                // Check mode: Check showed drift
	OutcomeSkipped                      // Cascade — earlier failure aborted the run
)

func (o Outcome) String() string {
	switch o {
	case OutcomeUnchanged:
		return "unchanged"
	case OutcomeChanged:
		return "changed"
	case OutcomeNoOp:
		return "no-op"
	case OutcomeFailed:
		return "failed"
	case OutcomeDriftDetected:
		return "drift-detected"
	case OutcomeSkipped:
		return "skipped"
	default:
		return fmt.Sprintf("Outcome(%d)", int(o))
	}
}

// DeclarationResult records what happened to one declaration during a
// run. Check / Apply / Test pointers are nil when the corresponding
// phase was not reached (e.g. Apply is nil if Check matched, or if
// Check itself returned an error).
//
// Compensated and CompensateError are set only by [Runner.RunSaga]
// during the reverse-walk that follows a failed apply. Compensated
// is true when the runner attempted to roll this declaration back —
// even when no prior state exists (and so the rollback was a no-op).
// CompensateError is non-nil when the rollback Apply itself failed.
// Regular [Runner.Run] / [Runner.Check] never set these fields.
type DeclarationResult struct {
	DeclID    string
	Module    string
	Outcome   Outcome
	Check     *ModuleCheckResult
	Apply     *StateResult
	Test      *bool
	Error     error // non-nil iff Outcome == OutcomeFailed
	StartedAt time.Time
	Duration  time.Duration

	Compensated     bool
	CompensateError error
}

// RunReport is the aggregate output of one Runner.Run or Runner.Check
// call. Results are in execution order (the order the runner consumed
// decls). Counters give the CLI a one-line summary without re-walking
// Results.
type RunReport struct {
	Mode      RunMode
	StartedAt time.Time
	EndedAt   time.Time
	Results   []DeclarationResult

	Total     int
	Changed   int
	Unchanged int // OutcomeUnchanged + OutcomeNoOp
	Failed    int
	Skipped   int
	Drifted   int // Check mode only
}

// RunObserver is the runner's observability surface. Implementations
// fan out to the event system (Epic 11), audit log (Epic 12), and
// any local sinks (CLI progress bar, metrics).
//
// Method → event-type mapping:
//
//	Start  → state.apply.start
//	Drift  → state.drift
//	Change → state.change
//	Done   → state.apply.done (Outcome != Failed) OR state.apply.fail (Outcome == Failed)
//	Skip   → state.apply.skip   (new — see ROADMAP for taxonomy sync)
//
// Skip exists because cascade-skipped declarations are part of the
// observability picture: an external subscriber (alerting, audit,
// dashboards) cannot consult the CLI caller's RunReport — they need
// the event stream to be complete.
type RunObserver interface {
	Start(ctx context.Context, decl *Declaration)
	Drift(ctx context.Context, decl *Declaration, check *ModuleCheckResult)
	Change(ctx context.Context, decl *Declaration, result *StateResult)
	Done(ctx context.Context, result *DeclarationResult)
	Skip(ctx context.Context, decl *Declaration, reason error)
}

// Run executes decls in order with the full Check → Apply → Test
// pipeline. Stops at the first failure; subsequent decls fire Skip
// and land in the report as OutcomeSkipped.
func (r *Runner) Run(ctx context.Context, decls []*Declaration) (*RunReport, error) {
	return r.execute(ctx, decls, ModeApply)
}

// Check runs the Check phase only — no Apply, no Test. Reports drift
// per declaration but does not remediate.
func (r *Runner) Check(ctx context.Context, decls []*Declaration) (*RunReport, error) {
	return r.execute(ctx, decls, ModeCheck)
}

// execute is the shared engine behind Run + Check.
func (r *Runner) execute(ctx context.Context, decls []*Declaration, mode RunMode) (*RunReport, error) {
	obs := r.observer()
	reg := r.registry()

	report := &RunReport{
		Mode:      mode,
		StartedAt: time.Now(),
	}
	defer func() {
		report.EndedAt = time.Now()
		report.Total = len(report.Results)
	}()

	var runErr error // first failure that aborts the run

	for i, decl := range decls {
		if decl == nil {
			continue
		}
		if runErr != nil {
			// Cascade: emit Skip + record OutcomeSkipped with the
			// originating error as the reason so an external
			// subscriber can correlate.
			obs.Skip(ctx, decl, runErr)
			report.Results = append(report.Results, DeclarationResult{
				DeclID:  decl.ID,
				Module:  decl.Module,
				Outcome: OutcomeSkipped,
				Error:   runErr,
			})
			report.Skipped++
			continue
		}
		// Honor parent ctx cancellation between decls so a slow
		// run doesn't drag on after the caller's gone.
		if err := ctx.Err(); err != nil {
			runErr = err
			obs.Skip(ctx, decl, runErr)
			report.Results = append(report.Results, DeclarationResult{
				DeclID:  decl.ID,
				Module:  decl.Module,
				Outcome: OutcomeSkipped,
				Error:   runErr,
			})
			report.Skipped++
			continue
		}

		result := r.runOne(ctx, decl, mode, reg, obs)
		report.Results = append(report.Results, result)

		switch result.Outcome {
		case OutcomeChanged:
			report.Changed++
			if mode == ModeApply {
				r.Metrics.RecordApply(ApplyResultSuccess)
			}
		case OutcomeUnchanged, OutcomeNoOp:
			report.Unchanged++
			if mode == ModeApply {
				r.Metrics.RecordApply(ApplyResultNoChange)
			}
		case OutcomeDriftDetected:
			report.Drifted++
		case OutcomeFailed:
			report.Failed++
			runErr = result.Error
			if mode == ModeApply {
				r.Metrics.RecordApply(ApplyResultFailed)
			}
		case OutcomeSkipped:
			report.Skipped++
		}

		_ = i // silenced; loop index reserved for future per-decl context
	}

	return report, runErr
}

func (r *Runner) runOne(ctx context.Context, decl *Declaration, mode RunMode, reg *Registry, obs RunObserver) DeclarationResult {
	res := DeclarationResult{
		DeclID:    decl.ID,
		Module:    decl.Module,
		StartedAt: time.Now(),
	}
	defer func() { res.Duration = time.Since(res.StartedAt) }()

	obs.Start(ctx, decl)

	mod, err := reg.Get(decl.Module)
	if err != nil {
		res.Outcome = OutcomeFailed
		res.Error = fmt.Errorf("module lookup: %w", err)
		obs.Done(ctx, &res)
		return res
	}

	// Modules see Params without the engine-reserved requisite keys;
	// observers still receive the original decl for graph context.
	mdecl := decl.moduleView()

	// --- Check phase ---
	checkCtx, cancel := r.declContext(ctx)
	check, checkErr := mod.Check(checkCtx, mdecl)
	cancel()
	if checkErr != nil {
		res.Outcome = OutcomeFailed
		res.Error = fmt.Errorf("check: %w", checkErr)
		obs.Done(ctx, &res)
		return res
	}
	res.Check = check

	if check != nil && !check.Matches {
		obs.Drift(ctx, decl, check)
	}

	if mode == ModeCheck {
		// Dry-run: stop at Check.
		if check == nil || check.Matches {
			res.Outcome = OutcomeUnchanged
		} else {
			res.Outcome = OutcomeDriftDetected
		}
		obs.Done(ctx, &res)
		return res
	}

	// --- Apply phase (only when Check reports drift) ---
	if check != nil && check.Matches {
		res.Outcome = OutcomeUnchanged
		obs.Done(ctx, &res)
		return res
	}

	applyCtx, cancel := r.declContext(ctx)
	applyResult, applyErr := mod.Apply(applyCtx, mdecl)
	cancel()
	if applyErr != nil {
		res.Outcome = OutcomeFailed
		res.Error = fmt.Errorf("apply: %w", applyErr)
		obs.Done(ctx, &res)
		return res
	}
	res.Apply = applyResult

	if applyResult != nil && applyResult.Changed {
		obs.Change(ctx, decl, applyResult)
	}

	// --- Test phase ---
	testCtx, cancel := r.declContext(ctx)
	testOK, testErr := mod.Test(testCtx, mdecl)
	cancel()
	if testErr != nil {
		res.Outcome = OutcomeFailed
		res.Error = fmt.Errorf("test: %w", testErr)
		obs.Done(ctx, &res)
		return res
	}
	res.Test = &testOK
	if !testOK {
		res.Outcome = OutcomeFailed
		res.Error = errors.New("post-apply Test returned false")
		obs.Done(ctx, &res)
		return res
	}

	if applyResult != nil && applyResult.Changed {
		res.Outcome = OutcomeChanged
	} else {
		res.Outcome = OutcomeNoOp
	}
	obs.Done(ctx, &res)
	return res
}

// declContext returns a per-declaration context that honours
// DeclTimeout when set. When DeclTimeout is zero the parent ctx is
// passed through and the cancel func is a no-op.
func (r *Runner) declContext(parent context.Context) (context.Context, context.CancelFunc) {
	if r.DeclTimeout <= 0 {
		return parent, func() {}
	}
	return context.WithTimeout(parent, r.DeclTimeout)
}

func (r *Runner) registry() *Registry {
	if r.Registry != nil {
		return r.Registry
	}
	return DefaultRegistry
}

func (r *Runner) observer() RunObserver {
	if r.Observer != nil {
		return r.Observer
	}
	return noopObserver{}
}

// noopObserver is the default when Runner.Observer is nil. All
// methods are no-ops so the runner can call without a nil-check on
// every transition.
type noopObserver struct{}

func (noopObserver) Start(context.Context, *Declaration)                          {}
func (noopObserver) Drift(context.Context, *Declaration, *ModuleCheckResult)      {}
func (noopObserver) Change(context.Context, *Declaration, *StateResult)           {}
func (noopObserver) Done(context.Context, *DeclarationResult)                     {}
func (noopObserver) Skip(context.Context, *Declaration, error)                    {}
