// SPDX-License-Identifier: Apache-2.0

// Package timezone implements the `timezone` stdlib state module —
// the system timezone per PROJECT-DETAILS §4.8.
//
// Declaration.Name IS the desired timezone (America/New_York, UTC,
// Etc/GMT+5). One state: present.
//
// Linux is fully supported: timedatectl is used when present
// (validates the zone, updates the symlink + running offset);
// otherwise the module symlinks /etc/localtime → the zoneinfo file
// and writes /etc/timezone — which is what Alpine / Debian without
// systemd expect anyway. Non-Linux → ErrUnsupportedOS.
//
// v0.1 out of scope (v0.x candidates):
//   - NTP-sync coupling (timedatectl set-ntp)
//   - localtime: copy mode (copy the zoneinfo file instead of
//     symlinking)
//   - macOS (systemsetup -settimezone)
package timezone

import (
	"context"
	"fmt"
	"time"

	"go.keystone-core.io/keystone-core/internal/statemgmt"
)

type Module struct {
	provider Provider
}

func New() statemgmt.Module { return &Module{provider: defaultProvider()} }

// NewWithProvider is the test injection point.
func NewWithProvider(p Provider) statemgmt.Module { return &Module{provider: p} }

func (m *Module) Name() string { return "timezone" }

func (m *Module) ValidStates() []string { return []string{StatePresent} }

func (m *Module) Validate(decl *statemgmt.Declaration) error {
	p, err := parseParams(decl)
	if err != nil {
		return err
	}
	return p.validate()
}

// DriftSeverity: wrong tz affects log timestamps and cron timing
// but isn't a break — MEDIUM. Operator overrides via severity:.
func (m *Module) DriftSeverity(*statemgmt.Declaration, *statemgmt.ModuleCheckResult) statemgmt.DriftSeverity {
	return statemgmt.DriftSeverityMedium
}

func (m *Module) Check(_ context.Context, decl *statemgmt.Declaration) (*statemgmt.ModuleCheckResult, error) {
	p, err := parseParams(decl)
	if err != nil {
		return nil, err
	}
	if err := p.validate(); err != nil {
		return nil, err
	}
	return m.check(p)
}

func (m *Module) check(p *params) (*statemgmt.ModuleCheckResult, error) {
	cur, set, err := m.provider.Current()
	if err != nil {
		return nil, err
	}
	if !set {
		return &statemgmt.ModuleCheckResult{Matches: false, Diff: fmt.Sprintf("/etc/localtime not a zoneinfo symlink; want %q", p.Zone)}, nil
	}
	if cur != p.Zone {
		return &statemgmt.ModuleCheckResult{Matches: false, Diff: fmt.Sprintf("timezone %q → %q", cur, p.Zone)}, nil
	}
	return &statemgmt.ModuleCheckResult{Matches: true}, nil
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
	pre, err := m.check(p)
	if err != nil {
		return &statemgmt.StateResult{Success: false, Duration: time.Since(start)}, err
	}
	if pre.Matches {
		return &statemgmt.StateResult{Success: true, Comment: "already converged", Duration: time.Since(start)}, nil
	}
	if err := m.provider.Set(ctx, p.Zone); err != nil {
		return &statemgmt.StateResult{Success: false, Diff: pre.Diff, Duration: time.Since(start)}, err
	}
	return &statemgmt.StateResult{Success: true, Changed: true, Diff: pre.Diff, Comment: "applied", Duration: time.Since(start)}, nil
}

func (m *Module) Test(ctx context.Context, decl *statemgmt.Declaration) (bool, error) {
	res, err := m.Check(ctx, decl)
	if err != nil {
		return false, err
	}
	return res.Matches, nil
}
