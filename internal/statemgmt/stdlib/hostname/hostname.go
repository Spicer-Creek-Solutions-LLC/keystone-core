// SPDX-License-Identifier: Apache-2.0

// Package hostname implements the `hostname` stdlib state module —
// the system static hostname per PROJECT-DETAILS §4.8.
//
// The system has exactly one hostname, so Declaration.Name IS the
// desired value. One state: present.
//
// Linux is fully supported: hostnamectl is used when present
// (handles dbus / SELinux), otherwise the module writes /etc/hostname
// and execs hostname(1) — Alpine / OpenRC hosts manage hostname via
// the file anyway. Non-Linux → ErrUnsupportedOS.
//
// v0.1 out of scope (v0.x candidates):
//   - pretty hostname (/etc/machine-info)
//   - transient-only mode
//   - divergent running-vs-static hostname repair
//   - macOS (systemsetup -setcomputername)
package hostname

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

func (m *Module) Name() string { return "hostname" }

func (m *Module) ValidStates() []string { return []string{StatePresent} }

func (m *Module) Validate(decl *statemgmt.Declaration) error {
	p, err := parseParams(decl)
	if err != nil {
		return err
	}
	return p.validate()
}

// DriftSeverity: a wrong hostname is mostly cosmetic — MEDIUM.
// Operator overrides via severity:.
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
		return &statemgmt.ModuleCheckResult{Matches: false, Diff: fmt.Sprintf("no static hostname set; want %q", p.Hostname)}, nil
	}
	if cur != p.Hostname {
		return &statemgmt.ModuleCheckResult{Matches: false, Diff: fmt.Sprintf("hostname %q → %q", cur, p.Hostname)}, nil
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
	if err := m.provider.Set(ctx, p.Hostname); err != nil {
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
