// Package cmd implements the `cmd` stdlib state module — execute
// arbitrary shell commands with idempotency guards per
// PROJECT-DETAILS §4.8.
//
// State semantics:
//
//	run — execute Params.command via /bin/sh -c, gated by at least
//	      one of:
//	        creates — skip if the path exists
//	        onlyif  — skip if the guard command exits non-zero
//	        unless  — skip if the guard command exits zero
//
// The mandatory-guard rule keeps the Check → Apply → Test loop
// closed: a well-written declaration's guards flip from "run" to
// "skip" once the main command has done its work, making the
// declaration idempotent at the engine level. Operators who really
// want to run a command every time can opt in with
// `onlyif: /bin/true`.
//
// v0.1 out of scope (v0.x candidates):
//   - state: wait (runs only when triggered by a watch requisite)
//   - runas (run as a different user)
//   - non-POSIX shells (only /bin/sh in v1.0)
//   - sandboxing (seccomp / namespace isolation)
//   - Windows / cmd.exe
//   - command-policy integration (Epic 07 CommandPolicy)
package cmd

import (
	"context"
	"fmt"
	"time"

	"go.keystone-core.io/keystone-core/internal/statemgmt"
)

// New is the Factory registered with the engine Registry.
func New() statemgmt.Module { return &Module{} }

// Module is the cmd state module. Stateless; concurrent
// Check/Apply/Test calls on different Declarations are safe.
type Module struct{}

func (m *Module) Name() string { return "cmd" }

func (m *Module) ValidStates() []string {
	return []string{StateRun}
}

// Validate enforces the cross-field shape checks parseParams +
// validate() encode. Engine Validator catches non-empty Name +
// State ∈ ValidStates() before this fires.
func (m *Module) Validate(decl *statemgmt.Declaration) error {
	p, err := parseParams(decl)
	if err != nil {
		return err
	}
	return p.validate()
}

// DriftSeverity is the optional opt-in interface from Task 7.
// Commands are operator-declared work; classifying their drift is
// genuinely ambiguous so we default to MEDIUM. Operators can
// override per-decl via the engine-reserved `severity:` key.
func (m *Module) DriftSeverity(_ *statemgmt.Declaration, _ *statemgmt.ModuleCheckResult) statemgmt.DriftSeverity {
	return statemgmt.DriftSeverityMedium
}

// Check evaluates the declared guards (creates / onlyif / unless).
// Matches=true when every guard says "skip" — the command's work
// is already done.
func (m *Module) Check(ctx context.Context, decl *statemgmt.Declaration) (*statemgmt.ModuleCheckResult, error) {
	p, err := parseParams(decl)
	if err != nil {
		return nil, err
	}
	if err := p.validate(); err != nil {
		return nil, err
	}
	decision, detail, err := evaluateGuards(ctx, p)
	if err != nil {
		return nil, err
	}
	if decision == guardSkip {
		return &statemgmt.ModuleCheckResult{Matches: true, Diff: detail}, nil
	}
	return &statemgmt.ModuleCheckResult{Matches: false, Diff: detail}, nil
}

// Apply runs the declared command. Unlike file.Apply, cmd.Apply
// does NOT early-return on already-converged: it trusts that the
// engine runner only invokes it when Check returned Matches=false.
// Operators bypassing Check (e.g., from a future saga retry path)
// get the command-runs-every-time semantics they asked for.
func (m *Module) Apply(ctx context.Context, decl *statemgmt.Declaration) (*statemgmt.StateResult, error) {
	start := time.Now()
	p, err := parseParams(decl)
	if err != nil {
		return nil, err
	}
	if err := p.validate(); err != nil {
		return nil, err
	}

	outcome, err := runShell(ctx, p, p.Command)
	res := &statemgmt.StateResult{
		Duration: time.Since(start),
		Comment:  outcome.String(),
	}
	if err != nil {
		res.Success = false
		res.Diff = outcome.String()
		return res, err
	}
	if outcome.ExitCode != 0 {
		res.Success = false
		res.Diff = outcome.String()
		return res, fmt.Errorf("command exited %d", outcome.ExitCode)
	}
	res.Success = true
	res.Changed = true // a successful Apply always counts as a change
	res.Diff = outcome.String()
	return res, nil
}

// Test re-evaluates the guards. Returns true iff every guard now
// says "skip" — i.e., the declaration is idempotent and the main
// command's effect has taken hold. A well-written cmd declaration
// converges on the second Check.
func (m *Module) Test(ctx context.Context, decl *statemgmt.Declaration) (bool, error) {
	res, err := m.Check(ctx, decl)
	if err != nil {
		return false, err
	}
	return res.Matches, nil
}
