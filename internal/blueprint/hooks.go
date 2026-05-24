// SPDX-License-Identifier: Apache-2.0

package blueprint

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"

	"go.keystone-core.io/keystone-core/internal/runbook"
)

// Hook phase names (also the audit/log phase label).
const (
	PhasePreApply     = "pre_apply"
	PhasePostApply    = "post_apply"
	PhasePreRollback  = "pre_rollback"
	PhasePostRollback = "post_rollback"
)

// ErrHookRunnerRequired is returned by Apply/Rollback when a manifest
// declares hooks but no HookRunner is configured.
var ErrHookRunnerRequired = errors.New("blueprint: hook runner required (manifest declares hooks)")

// HookContext is passed to a HookRunner for one hook invocation. Name
// is the runbook reference from the manifest hook list; Phase is one
// of the Phase* constants.
type HookContext struct {
	Manifest  *Manifest
	Phase     string
	Name      string
	Params    map[string]any
	Features  map[string]bool
	Namespace string
}

// HookRunner runs one blueprint hook. Blueprint hooks ARE runbooks
// (§4.17); the runbook-backed implementation is RunbookHookRunner.
type HookRunner interface {
	RunHook(ctx context.Context, hc HookContext) error
}

// RunbookHookRunner runs a hook by loading the named runbook (a path
// relative to the blueprint's SourcePath) and executing it on the
// injected runbook.Executor. The executor's Registry must already
// have the v1.0 step types registered (server wiring).
type RunbookHookRunner struct {
	Exec *runbook.Executor
}

// NewRunbookHookRunner wraps exec.
func NewRunbookHookRunner(exec *runbook.Executor) *RunbookHookRunner {
	return &RunbookHookRunner{Exec: exec}
}

// RunHook implements HookRunner. The hook runbook receives the
// resolved blueprint params + features as runbook inputs (keys
// "params" and "features").
func (h *RunbookHookRunner) RunHook(ctx context.Context, hc HookContext) error {
	if h == nil || h.Exec == nil {
		return ErrHookRunnerRequired
	}
	path := filepath.Join(hc.Manifest.SourcePath, hc.Name)
	rb, err := runbook.Load(path)
	if err != nil {
		return fmt.Errorf("blueprint: hook %s/%q: %w", hc.Phase, hc.Name, err)
	}
	// The blueprint context is offered as runbook inputs "params" /
	// "features"; pass only those the hook runbook actually declares
	// so a hook that ignores them is not rejected for "unknown input".
	offered := map[string]any{"params": hc.Params, "features": hc.Features}
	inputs := make(map[string]any, len(offered))
	for _, in := range rb.Spec.Inputs {
		if v, ok := offered[in.Name]; ok {
			inputs[in.Name] = v
		}
	}
	exec, err := h.Exec.Execute(ctx, rb, inputs)
	if err != nil {
		return fmt.Errorf("blueprint: hook %s/%q: %w", hc.Phase, hc.Name, err)
	}
	if exec.Status != runbook.StatusSucceeded {
		return fmt.Errorf("blueprint: hook %s/%q ended %s", hc.Phase, hc.Name, exec.Status)
	}
	return nil
}
