// SPDX-License-Identifier: Apache-2.0

package blueprint

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"

	"go.keystone-core.io/keystone-core/internal/statemgmt"
)

// ErrNoStateRunner is returned when Apply/Rollback is called without a
// StateRunner configured.
var ErrNoStateRunner = errors.New("blueprint: no state runner configured")

// ErrApplyFailed wraps a state run that ended with failed
// declarations. The *ApplyResult is still returned.
var ErrApplyFailed = errors.New("blueprint: apply failed")

// StateRunner applies a resolved declaration list. *statemgmt.Runner
// satisfies it.
type StateRunner interface {
	Run(ctx context.Context, decls []*statemgmt.Declaration) (*statemgmt.RunReport, error)
}

// ApplyOptions configures one Apply.
type ApplyOptions struct {
	Inputs     map[string]string // raw param inputs (secret:// allowed for source:secret)
	Enable     []string          // feature overrides on
	Disable    []string          // feature overrides off
	As         string            // multi-instance namespace ("" = none)
	Entrypoint string             // "" = entrypoints.default
}

// ApplyResult is the outcome of Apply/Rollback.
type ApplyResult struct {
	RunID   string
	Report  *statemgmt.RunReport
	Outputs map[string]any
	Status  string
}

// Executor applies and rolls back blueprints. All collaborators are
// interfaces so the executor is unit-testable and acyclic.
type Executor struct {
	StateRunner StateRunner
	Secrets     SecretResolver
	Hooks       HookRunner
	Store       AppliedStore

	Clock func() time.Time
	NewID func() string
}

func (e *Executor) now() time.Time {
	if e.Clock != nil {
		return e.Clock()
	}
	return time.Now()
}

func (e *Executor) newID() string {
	if e.NewID != nil {
		return e.NewID()
	}
	return uuid.NewString()
}

func (e *Executor) store() AppliedStore {
	if e.Store != nil {
		return e.Store
	}
	// A throwaway store keeps Apply usable without rollback support.
	return NewMemoryAppliedStore()
}

// Apply resolves parameters (substituting source:secret values via
// the SecretResolver), evaluates features, runs pre_apply hooks,
// renders + filters + (optionally) namespaces + resolves the
// entrypoint state collection, runs it, runs post_apply hooks,
// renders outputs, and records an AppliedRun.
func (e *Executor) Apply(ctx context.Context, m *Manifest, opts ApplyOptions) (*ApplyResult, error) {
	if e.StateRunner == nil {
		return nil, ErrNoStateRunner
	}

	inputs, err := e.substituteSecrets(ctx, m, opts.Inputs)
	if err != nil {
		return nil, err
	}
	resolved, err := m.ResolveParams(inputs)
	if err != nil {
		return nil, err
	}
	features, err := EvaluateFeatures(m, opts.Enable, opts.Disable)
	if err != nil {
		return nil, err
	}

	run := AppliedRun{
		ID:         e.newID(),
		Blueprint:  m.Metadata.Name,
		Version:    m.Metadata.Version,
		SourcePath: m.SourcePath,
		Namespace:  opts.As,
		Params:     resolved.Values,
		Features:   features,
		StartedAt:  e.now(),
		Status:     "failed",
	}

	if err := e.runHooks(ctx, m, PhasePreApply, m.Hooks.PreApply, resolved, features, opts.As); err != nil {
		return nil, err
	}

	report, rel, err := e.runEntrypoint(ctx, m, opts.Entrypoint, resolved, features, opts.As)
	run.Entrypoint = rel
	if err != nil {
		run.EndedAt = e.now()
		_ = e.store().Save(ctx, run)
		return nil, err
	}
	if report.Failed > 0 {
		run.EndedAt = e.now()
		_ = e.store().Save(ctx, run)
		return &ApplyResult{RunID: run.ID, Report: report, Status: "failed"},
			fmt.Errorf("%w: %d declaration(s) failed", ErrApplyFailed, report.Failed)
	}

	if err := e.runHooks(ctx, m, PhasePostApply, m.Hooks.PostApply, resolved, features, opts.As); err != nil {
		run.EndedAt = e.now()
		_ = e.store().Save(ctx, run)
		return nil, err
	}

	outputs, err := e.renderOutputs(m, resolved, features)
	if err != nil {
		return nil, err
	}

	run.Status = "succeeded"
	run.EndedAt = e.now()
	if err := e.store().Save(ctx, run); err != nil {
		return nil, fmt.Errorf("blueprint: record applied run: %w", err)
	}
	return &ApplyResult{RunID: run.ID, Report: report, Outputs: outputs, Status: "succeeded"}, nil
}

// Rollback reloads the blueprint recorded for runID and applies its
// rollback entrypoint with the original resolved params/features.
func (e *Executor) Rollback(ctx context.Context, runID string) (*ApplyResult, error) {
	if e.StateRunner == nil {
		return nil, ErrNoStateRunner
	}
	prev, err := e.store().Get(ctx, runID)
	if err != nil {
		return nil, err
	}
	m, err := Load(prev.SourcePath)
	if err != nil {
		return nil, fmt.Errorf("blueprint: rollback %s: reload manifest: %w", runID, err)
	}
	resolved := ResolvedParams{Values: prev.Params}

	run := AppliedRun{
		ID:         e.newID(),
		ParentID:   prev.ID,
		Blueprint:  m.Metadata.Name,
		Version:    m.Metadata.Version,
		SourcePath: m.SourcePath,
		Namespace:  prev.Namespace,
		Params:     prev.Params,
		Features:   prev.Features,
		StartedAt:  e.now(),
		Status:     "failed",
	}

	if err := e.runHooks(ctx, m, PhasePreRollback, m.Hooks.PreRollback, resolved, prev.Features, prev.Namespace); err != nil {
		return nil, err
	}
	report, rel, err := e.runEntrypoint(ctx, m, "rollback", resolved, prev.Features, prev.Namespace)
	run.Entrypoint = rel
	if err != nil {
		run.EndedAt = e.now()
		_ = e.store().Save(ctx, run)
		return nil, err
	}
	if report.Failed > 0 {
		run.EndedAt = e.now()
		_ = e.store().Save(ctx, run)
		return &ApplyResult{RunID: run.ID, Report: report, Status: "failed"},
			fmt.Errorf("%w: %d declaration(s) failed", ErrApplyFailed, report.Failed)
	}
	if err := e.runHooks(ctx, m, PhasePostRollback, m.Hooks.PostRollback, resolved, prev.Features, prev.Namespace); err != nil {
		run.EndedAt = e.now()
		_ = e.store().Save(ctx, run)
		return nil, err
	}
	run.Status = "succeeded"
	run.EndedAt = e.now()
	_ = e.store().Save(ctx, run)
	return &ApplyResult{RunID: run.ID, Report: report, Status: "succeeded"}, nil
}

// runEntrypoint renders → parses → feature-filters → namespaces →
// resolves → runs the named entrypoint. Returns the run report and
// the resolved relative path.
func (e *Executor) runEntrypoint(ctx context.Context, m *Manifest, name string, rp ResolvedParams, features map[string]bool, ns string) (*statemgmt.RunReport, string, error) {
	rel, raw, err := resolveEntrypoint(m, name)
	if err != nil {
		return nil, "", err
	}
	rendered, err := RenderState(string(raw), NewRenderContext(rp, features))
	if err != nil {
		return nil, rel, err
	}
	sf, err := statemgmt.Parse([]byte(rendered))
	if err != nil {
		return nil, rel, fmt.Errorf("blueprint: parse entrypoint %q: %w", name, err)
	}
	sf, err = FilterStateFile(sf, m, features)
	if err != nil {
		return nil, rel, err
	}
	if ns != "" {
		sf, err = Namespace(sf, ns)
		if err != nil {
			return nil, rel, err
		}
	}
	decls, err := statemgmt.NewResolver().Resolve(sf)
	if err != nil {
		return nil, rel, fmt.Errorf("blueprint: resolve entrypoint %q: %w", name, err)
	}
	report, err := e.StateRunner.Run(ctx, decls)
	if err != nil {
		return nil, rel, fmt.Errorf("blueprint: run entrypoint %q: %w", name, err)
	}
	return report, rel, nil
}

func (e *Executor) runHooks(ctx context.Context, m *Manifest, phase string, names []string, rp ResolvedParams, features map[string]bool, ns string) error {
	if len(names) == 0 {
		return nil
	}
	if e.Hooks == nil {
		return fmt.Errorf("%w (%s)", ErrHookRunnerRequired, phase)
	}
	for _, name := range names {
		hc := HookContext{
			Manifest:  m,
			Phase:     phase,
			Name:      name,
			Params:    rp.Values,
			Features:  features,
			Namespace: ns,
		}
		if err := e.Hooks.RunHook(ctx, hc); err != nil {
			return fmt.Errorf("blueprint: %s hook %q: %w", phase, name, err)
		}
	}
	return nil
}

// substituteSecrets replaces a source:secret parameter's secret://
// input with its resolved cleartext before coercion/validation.
func (e *Executor) substituteSecrets(ctx context.Context, m *Manifest, in map[string]string) (map[string]string, error) {
	if len(in) == 0 {
		return in, nil
	}
	out := make(map[string]string, len(in))
	for k, v := range in {
		out[k] = v
	}
	for name, spec := range m.Parameters {
		if spec.Source != SourceSecret {
			continue
		}
		v, ok := out[name]
		if !ok || !IsSecretRef(v) {
			continue
		}
		if e.Secrets == nil {
			return nil, fmt.Errorf("%w (parameter %q)", ErrSecretResolverRequired, name)
		}
		resolvedVal, err := e.Secrets.ResolveSecret(ctx, v)
		if err != nil {
			return nil, err
		}
		out[name] = resolvedVal
	}
	return out, nil
}

func (e *Executor) renderOutputs(m *Manifest, rp ResolvedParams, features map[string]bool) (map[string]any, error) {
	if len(m.Outputs) == 0 {
		return nil, nil
	}
	rc := NewRenderContext(rp, features)
	out := make(map[string]any, len(m.Outputs))
	for name, spec := range m.Outputs {
		if spec.Value == "" {
			continue
		}
		v, err := RenderState(spec.Value, rc)
		if err != nil {
			return nil, fmt.Errorf("blueprint: render output %q: %w", name, err)
		}
		out[name] = v
	}
	return out, nil
}
