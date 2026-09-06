// SPDX-License-Identifier: Apache-2.0

package controlplane

import (
	"context"
	"errors"
	"fmt"

	bp "go.keystone-core.io/keystone-core/internal/blueprint"
)

// BlueprintApplier applies a blueprint either on the control-plane
// host or across a resolved set of agents.
//
// Which one is decided by a single fact: whether the caller resolved a
// target to any agents. The applier does not parse targets or consult
// the agent store -- the gRPC layer already did that, because it is
// the layer that speaks the wire types. Keeping resolution there means
// there is exactly one place that decides which hosts an operator
// meant, shared with `state apply`.
type BlueprintApplier struct {
	// Catalog resolves a blueprint name to its manifest.
	Catalog BlueprintManifestSource
	// Local runs declarations on this host. Used when no agents are
	// targeted; nil disables local apply.
	Local bp.StateRunner
	// Converge carries a rendered blueprint to agents. Nil disables
	// remote apply.
	Converge ConvergeFanout
	// Store persists AppliedRun records for rollback.
	Store bp.AppliedStore
	// Secrets and Hooks are passed through to the executor.
	Secrets bp.SecretResolver
	Hooks   bp.HookRunner
}

// BlueprintManifestSource is the slice of the catalog this needs.
type BlueprintManifestSource interface {
	Get(ctx context.Context, name string) (*bp.Manifest, error)
}

var (
	// ErrNoCatalog means no blueprint catalog is configured.
	ErrNoCatalog = errors.New("controlplane: blueprint catalog not configured")
	// ErrRemoteApplyDisabled means a targeted apply arrived but no
	// converge path is wired.
	ErrRemoteApplyDisabled = errors.New("controlplane: remote blueprint apply is not wired on this server")
	// ErrLocalApplyDisabled means an untargeted apply arrived but no
	// local runner is wired.
	ErrLocalApplyDisabled = errors.New("controlplane: local blueprint apply is not wired on this server")
	// ErrNoAppliedStore means a rollback was requested with no store
	// to read the original run from.
	ErrNoAppliedStore = errors.New("controlplane: blueprint applied-run store is not configured")
)

// Apply runs the blueprint named name.
//
// opts.Agents decides the path. Non-empty means the rendered state
// file goes to those agents, each compiling it against its own facts;
// empty means the control-plane host converges itself.
func (a *BlueprintApplier) Apply(ctx context.Context, name string, opts bp.ApplyOptions) (*bp.ApplyResult, error) {
	if a.Catalog == nil {
		return nil, ErrNoCatalog
	}
	m, err := a.Catalog.Get(ctx, name)
	if err != nil {
		return nil, fmt.Errorf("controlplane: blueprint %q: %w", name, err)
	}

	ex := &bp.Executor{
		Secrets: a.Secrets,
		Hooks:   a.Hooks,
		Store:   a.Store,
	}

	if len(opts.Agents) > 0 {
		if a.Converge == nil {
			return nil, ErrRemoteApplyDisabled
		}
		runner, err := NewBlueprintConvergeRunner(BlueprintConvergeConfig{
			Converge:  a.Converge,
			Agents:    opts.Agents,
			Principal: convergePrincipal(ctx),
			Source:    name,
		})
		if err != nil {
			return nil, err
		}
		ex.FileRunner = runner
	} else {
		if a.Local == nil {
			return nil, ErrLocalApplyDisabled
		}
		ex.StateRunner = a.Local
	}

	return ex.Apply(ctx, m, opts)
}

// Rollback reverts a recorded apply, on the hosts that received it.
//
// The target comes from the run record, never from the caller. A
// rollback that re-resolved the original expression could reach an
// agent that joined after the apply -- handing it a revert for
// something it never got -- or miss one whose labels have since
// changed. The caller supplies a run id and nothing else, which is
// also why this cannot be expressed as an Apply with a target.
func (a *BlueprintApplier) Rollback(ctx context.Context, runID string) (*bp.ApplyResult, error) {
	if a.Store == nil {
		return nil, ErrNoAppliedStore
	}
	prev, err := a.Store.Get(ctx, runID)
	if err != nil {
		return nil, fmt.Errorf("controlplane: rollback %s: %w", runID, err)
	}

	ex := &bp.Executor{
		Secrets: a.Secrets,
		Hooks:   a.Hooks,
		Store:   a.Store,
	}

	if len(prev.Agents) > 0 {
		if a.Converge == nil {
			return nil, ErrRemoteApplyDisabled
		}
		runner, err := NewBlueprintConvergeRunner(BlueprintConvergeConfig{
			Converge:  a.Converge,
			Agents:    prev.Agents,
			Principal: convergePrincipal(ctx),
			Source:    prev.Blueprint + " (rollback)",
		})
		if err != nil {
			return nil, err
		}
		ex.FileRunner = runner
	} else {
		if a.Local == nil {
			return nil, ErrLocalApplyDisabled
		}
		ex.StateRunner = a.Local
	}

	return ex.Rollback(ctx, runID)
}
