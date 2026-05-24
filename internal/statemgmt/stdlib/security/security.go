// SPDX-License-Identifier: Apache-2.0

// Package security implements the `security` stdlib state module —
// the v1.0 surface manages SELinux settings, per PROJECT-DETAILS §4.8
// (SSH & security category).
//
// Operations per declaration (exactly one):
//
//   - **Mode** — `mode: enforcing|permissive|disabled` — sets the
//     global SELinux mode in `/etc/selinux/config` (`SELINUX=<mode>`,
//     persistent) and, when feasible, runs `setenforce 1/0` to match
//     at runtime. Going to `disabled` cannot happen at runtime
//     (kernel must be re-init'd); the persistent edit takes effect at
//     next reboot, and the Apply Comment says so.
//
//   - **Boolean** — `boolean: NAME` + `value: on|off` (or
//     `true|false`, `yes|no`, `1|0`) — toggles a named SELinux
//     boolean persistently and at runtime via `setsebool -P
//     NAME=on|off`.
//
// Declaration.Name is just a human label (the decl ID); the operation
// is identified by which params are set. `state: present` only —
// these are settings, not items; an `absent` semantics ("ensure NOT
// this value") is ambiguous and is V1X.
//
// v0.1 out of scope (v0.x candidates):
//   - **AppArmor** in any form — per-profile `enforce|complain|disable`
//     modes (`aa-enforce`/`aa-complain`/`aa-disable`), framework
//     on/off. The module is structured so a second Provider can be
//     added cleanly in v1.x.
//   - SELinux file contexts (`semanage fcontext` + `restorecon`),
//     port labels (`semanage port`), module install (`semodule`),
//     login / user mappings (`semanage login`).
//   - A runtime-only mode set without touching the persistent config
//     (a `persist: false` opt-out).
//   - Managing SELinux on hosts where `getenforce` isn't installed
//     but `/etc/selinux/config` is (we currently treat the tools as a
//     packaged unit).
//   - `state: absent` for booleans (toggle to off via `value: off`
//     instead).
package security

import (
	"context"
	"fmt"
	"strings"
	"time"

	"go.keystone-core.io/keystone-core/internal/statemgmt"
)

// New selects the platform's real Provider via auto-detection.
func New() statemgmt.Module { return &Module{provider: defaultProvider()} }

// NewWithProvider is the test injection point.
func NewWithProvider(p Provider) statemgmt.Module { return &Module{provider: p} }

type Module struct {
	provider Provider
}

func (m *Module) Name() string { return "security" }

// ValidStates is present-only; these are settings, not items. See
// the V1X note in the package comment.
func (m *Module) ValidStates() []string { return []string{StatePresent} }

func (m *Module) Validate(decl *statemgmt.Declaration) error {
	p, err := parseParams(decl)
	if err != nil {
		return err
	}
	return p.validate()
}

// DriftSeverity: the wrong global SELinux mode or a flipped boolean
// is a real security gap. HIGH always; MEDIUM for nil. Operators
// override via `severity:`.
func (m *Module) DriftSeverity(decl *statemgmt.Declaration, _ *statemgmt.ModuleCheckResult) statemgmt.DriftSeverity {
	if decl == nil {
		return statemgmt.DriftSeverityMedium
	}
	return statemgmt.DriftSeverityHigh
}

func (m *Module) Check(ctx context.Context, decl *statemgmt.Declaration) (*statemgmt.ModuleCheckResult, error) {
	p, err := m.parsed(decl)
	if err != nil {
		return nil, err
	}
	switch p.Op {
	case OpMode:
		return m.checkMode(ctx, p)
	case OpBoolean:
		return m.checkBoolean(ctx, p)
	}
	return nil, fmt.Errorf("unknown op %v", p.Op)
}

func (m *Module) Apply(ctx context.Context, decl *statemgmt.Declaration) (*statemgmt.StateResult, error) {
	start := time.Now()
	p, err := m.parsed(decl)
	if err != nil {
		return nil, err
	}
	switch p.Op {
	case OpMode:
		return m.applyMode(ctx, start, p)
	case OpBoolean:
		return m.applyBoolean(ctx, start, p)
	}
	return nil, fmt.Errorf("unknown op %v", p.Op)
}

func (m *Module) Test(ctx context.Context, decl *statemgmt.Declaration) (bool, error) {
	res, err := m.Check(ctx, decl)
	if err != nil {
		return false, err
	}
	return res.Matches, nil
}

func (m *Module) parsed(decl *statemgmt.Declaration) (*params, error) {
	p, err := parseParams(decl)
	if err != nil {
		return nil, err
	}
	if err := p.validate(); err != nil {
		return nil, err
	}
	return p, nil
}

// --- mode op -----------------------------------------------------------

func (m *Module) checkMode(ctx context.Context, p *params) (*statemgmt.ModuleCheckResult, error) {
	persistent, runtime, err := m.readMode(ctx)
	if err != nil {
		return nil, err
	}
	var diffs []string
	if persistent != p.Mode {
		diffs = append(diffs, fmt.Sprintf("persistent SELINUX=%s → %s", persistent, p.Mode))
	}
	if needRuntimeChange(p.Mode, runtime) {
		diffs = append(diffs, fmt.Sprintf("runtime SELinux mode %s → %s", runtime, p.Mode))
	}
	if len(diffs) == 0 {
		return &statemgmt.ModuleCheckResult{Matches: true}, nil
	}
	return &statemgmt.ModuleCheckResult{Matches: false, Diff: strings.Join(diffs, "; ")}, nil
}

func (m *Module) applyMode(ctx context.Context, start time.Time, p *params) (*statemgmt.StateResult, error) {
	persistent, runtime, err := m.readMode(ctx)
	if err != nil {
		return failure(start), err
	}
	needPersist := persistent != p.Mode
	needRuntime := needRuntimeChange(p.Mode, runtime)
	if !needPersist && !needRuntime {
		return ok(start, false, "", "already converged"), nil
	}
	var changes []string
	if needPersist {
		if err := m.provider.SetPersistentMode(ctx, p.Mode); err != nil {
			return failure(start), fmt.Errorf("set persistent mode: %w", err)
		}
		changes = append(changes, fmt.Sprintf("persistent SELINUX=%s → %s", persistent, p.Mode))
	}
	if needRuntime {
		if err := m.provider.SetRuntimeMode(ctx, p.Mode); err != nil {
			return failure(start), fmt.Errorf("set runtime mode: %w", err)
		}
		changes = append(changes, fmt.Sprintf("runtime SELinux mode %s → %s", runtime, p.Mode))
	}
	comment := "applied"
	switch {
	case p.Mode == ModeDisabled && runtime != ModeDisabled:
		comment = "persistent updated; reboot required for runtime to go disabled"
	case runtime == ModeDisabled && (p.Mode == ModeEnforcing || p.Mode == ModePermissive):
		comment = "persistent updated; SELinux is disabled at runtime — reboot to apply"
	}
	return ok(start, true, strings.Join(changes, "; "), comment), nil
}

func (m *Module) readMode(ctx context.Context) (persistent, runtime string, err error) {
	persistent, err = m.provider.GetPersistentMode(ctx)
	if err != nil {
		return "", "", err
	}
	runtime, err = m.provider.GetRuntimeMode(ctx)
	if err != nil {
		return "", "", err
	}
	return persistent, runtime, nil
}

// needRuntimeChange reports whether `setenforce` should be called to
// transition the runtime to target. setenforce cannot disable SELinux
// (the kernel must be re-init'd) and cannot move from disabled to
// enforcing/permissive (SELinux isn't loaded). In both cases the
// persistent edit takes effect at next reboot.
func needRuntimeChange(target, runtime string) bool {
	if target == ModeDisabled {
		return false
	}
	if runtime == ModeDisabled {
		return false
	}
	return runtime != target
}

// --- boolean op --------------------------------------------------------

func (m *Module) checkBoolean(ctx context.Context, p *params) (*statemgmt.ModuleCheckResult, error) {
	current, err := m.provider.GetBoolean(ctx, p.BooleanName)
	if err != nil {
		return nil, err
	}
	if current == p.BooleanValue {
		return &statemgmt.ModuleCheckResult{Matches: true}, nil
	}
	return &statemgmt.ModuleCheckResult{Matches: false, Diff: fmt.Sprintf("%s: %s → %s", p.BooleanName, onOff(current), onOff(p.BooleanValue))}, nil
}

func (m *Module) applyBoolean(ctx context.Context, start time.Time, p *params) (*statemgmt.StateResult, error) {
	current, err := m.provider.GetBoolean(ctx, p.BooleanName)
	if err != nil {
		return failure(start), err
	}
	if current == p.BooleanValue {
		return ok(start, false, "", "already converged"), nil
	}
	if err := m.provider.SetBoolean(ctx, p.BooleanName, p.BooleanValue); err != nil {
		return failure(start), fmt.Errorf("set boolean: %w", err)
	}
	return ok(start, true, fmt.Sprintf("%s: %s → %s", p.BooleanName, onOff(current), onOff(p.BooleanValue)), "applied"), nil
}

func onOff(b bool) string {
	if b {
		return "on"
	}
	return "off"
}

func ok(start time.Time, changed bool, diff, comment string) *statemgmt.StateResult {
	return &statemgmt.StateResult{Success: true, Changed: changed, Diff: diff, Comment: comment, Duration: time.Since(start)}
}
func failure(start time.Time) *statemgmt.StateResult {
	return &statemgmt.StateResult{Success: false, Changed: false, Duration: time.Since(start)}
}
