// Package service implements the `service` stdlib state module —
// Linux service management per PROJECT-DETAILS §4.8.
//
// "Running" and "enabled at boot" are orthogonal axes, so the
// module models them as a mandatory `state` (running | stopped) and
// an optional `enable:` bool (unset → leave boot-state alone). Apply
// runs up to two independent Provider operations — Start/Stop if the
// active state drifts, Enable/Disable if the enable state drifts —
// mirroring the Mod/SetGroups split in the `user` module.
//
// Backends:
//
//	v1.0: systemd
//	v1.x (V1X-BACKLOG): OpenRC, sysvinit, launchd (macOS)
//
// v1.0 out of scope (V1X candidates):
//   - OpenRC / sysvinit / Upstart / launchd backends
//   - mask / unmask separately from disable / enable
//   - reload (`systemctl reload`) — useful for watch handlers
//   - restart_on_change flag
//   - listing / inspecting all units
package service

import (
	"context"
	"fmt"
	"strings"
	"time"

	"go.keystone-core.io/keystone-core/internal/statemgmt"
)

type Module struct {
	provider Provider
}

// New selects the platform's real Provider via auto-detection.
func New() statemgmt.Module { return &Module{provider: defaultProvider()} }

// NewWithProvider is the test injection point.
func NewWithProvider(p Provider) statemgmt.Module { return &Module{provider: p} }

func (m *Module) Name() string { return "service" }

func (m *Module) ValidStates() []string {
	return []string{StateRunning, StateStopped}
}

func (m *Module) Validate(decl *statemgmt.Declaration) error {
	p, err := parseParams(decl)
	if err != nil {
		return err
	}
	return p.validate()
}

// DriftSeverity: a service that should be running but isn't (or
// shouldn't be running but is) is a production-health / security
// concern → HIGH. A boot-enablement mismatch alone → MEDIUM.
func (m *Module) DriftSeverity(decl *statemgmt.Declaration, check *statemgmt.ModuleCheckResult) statemgmt.DriftSeverity {
	if decl == nil {
		return statemgmt.DriftSeverityMedium
	}
	// If the diff mentions "active" the running state drifted →
	// HIGH. If it only mentions enablement → MEDIUM. Fall back to
	// HIGH for safety when we can't tell (the common case is the
	// running-state matters).
	if check != nil && check.Diff != "" {
		if strings.Contains(check.Diff, "active") {
			return statemgmt.DriftSeverityHigh
		}
		return statemgmt.DriftSeverityMedium
	}
	return statemgmt.DriftSeverityHigh
}

func (m *Module) Check(_ context.Context, decl *statemgmt.Declaration) (*statemgmt.ModuleCheckResult, error) {
	p, err := parseParams(decl)
	if err != nil {
		return nil, err
	}
	if err := p.validate(); err != nil {
		return nil, err
	}
	live, err := m.provider.Lookup(p.Name)
	if err != nil {
		return nil, err
	}
	if live == nil || !live.Exists {
		return nil, fmt.Errorf("%w: %s", ErrUnitNotFound, p.Name)
	}
	return diffCheck(p, live), nil
}

func (m *Module) Apply(ctx context.Context, decl *statemgmt.Declaration) (*statemgmt.StateResult, error) {
	start := time.Now()
	p, err := parseParams(decl)
	if err != nil {
		return nil, err
	}
	if err := p.validate(); err != nil {
		return nil, err
	}
	live, err := m.provider.Lookup(p.Name)
	if err != nil {
		return nil, err
	}
	if live == nil || !live.Exists {
		return &statemgmt.StateResult{
			Success:  false,
			Duration: time.Since(start),
		}, fmt.Errorf("%w: %s", ErrUnitNotFound, p.Name)
	}
	pre := diffCheck(p, live)
	if pre.Matches {
		return &statemgmt.StateResult{
			Success:  true,
			Comment:  "already converged",
			Duration: time.Since(start),
		}, nil
	}

	if err := m.doApply(ctx, p, live); err != nil {
		return &statemgmt.StateResult{
			Success:  false,
			Diff:     pre.Diff,
			Duration: time.Since(start),
		}, err
	}
	return &statemgmt.StateResult{
		Success:  true,
		Changed:  true,
		Diff:     pre.Diff,
		Comment:  "applied",
		Duration: time.Since(start),
	}, nil
}

func (m *Module) doApply(ctx context.Context, p *params, live *ServiceInfo) error {
	wantActive := p.State == StateRunning
	if live.Active != wantActive {
		if wantActive {
			if err := m.provider.Start(ctx, p.Name); err != nil {
				return err
			}
		} else {
			if err := m.provider.Stop(ctx, p.Name); err != nil {
				return err
			}
		}
	}
	if p.HasEnable && live.Enabled != p.Enable {
		if p.Enable {
			if err := m.provider.Enable(ctx, p.Name); err != nil {
				return err
			}
		} else {
			if err := m.provider.Disable(ctx, p.Name); err != nil {
				return err
			}
		}
	}
	return nil
}

func (m *Module) Test(ctx context.Context, decl *statemgmt.Declaration) (bool, error) {
	res, err := m.Check(ctx, decl)
	if err != nil {
		return false, err
	}
	return res.Matches, nil
}

// diffCheck is the pure-function compare. Caller has already
// verified live.Exists; never errors.
func diffCheck(p *params, live *ServiceInfo) *statemgmt.ModuleCheckResult {
	var diffs []string
	wantActive := p.State == StateRunning
	if live.Active != wantActive {
		diffs = append(diffs, fmt.Sprintf("active %v → %v", live.Active, wantActive))
	}
	if p.HasEnable && live.Enabled != p.Enable {
		diffs = append(diffs, fmt.Sprintf("enabled %v → %v", live.Enabled, p.Enable))
	}
	if len(diffs) == 0 {
		return &statemgmt.ModuleCheckResult{Matches: true}
	}
	return &statemgmt.ModuleCheckResult{
		Matches: false,
		Diff:    strings.Join(diffs, "; "),
	}
}
