// Package timer implements the `systemd_timer` stdlib state module —
// systemd timer units per PROJECT-DETAILS §4.8 (Scheduled tasks
// category). The Go package is named `timer`; it registers under the
// operator-facing name "systemd_timer" (mirroring kmod →
// kernel_module).
//
// It generates a `.timer` unit from structured parameters and
// manages its enabled/active state. The `.service` the timer
// triggers is the operator's responsibility — compose with the
// `file` and `service` modules, or point `service:` at an existing
// unit. Generating the paired service too is a v1.x enhancement.
//
// Declaration.Name is the timer base name ("backup" → the unit
// "backup.timer", file /etc/systemd/system/backup.timer).
//
// State semantics:
//
//	present — /etc/systemd/system/<name>.timer exists with the
//	          generated content; `enable: true` (the default) also
//	          requires the timer enabled at boot and currently armed;
//	          `enable: false` requires it disabled and inactive.
//	absent  — the unit file is removed (after a best-effort
//	          disable+stop) and systemd reloaded.
//
// The generated unit contains [Unit] Description, [Timer] OnCalendar
// (+ optional Persistent) + Unit=<service>, and [Install]
// WantedBy=timers.target — so the on-disk file is compared byte-for-
// byte against the rendered content, no parsing required. The module
// runs `systemctl daemon-reload` after writing or removing the file.
//
// Backend: systemd only. On a Linux host without `systemctl` the
// systemctl-backed operations fail with ErrNoBackend; on non-Linux,
// ErrUnsupportedOS.
//
// v1.0 out of scope (V1X candidates):
//   - Generating the paired `.service` unit (currently the
//     operator's job).
//   - `--user` (per-user) timers.
//   - OnBootSec / OnUnitActiveSec / OnStartupSec / RandomizedDelaySec
//     and other [Timer] knobs (v1.0 takes OnCalendar + Persistent).
//   - Calendar-expression validation (systemctl rejects malformed
//     expressions at enable time).
package timer

import (
	"context"
	"fmt"
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

func (m *Module) Name() string { return "systemd_timer" }

func (m *Module) ValidStates() []string { return []string{StatePresent, StateAbsent} }

func (m *Module) Validate(decl *statemgmt.Declaration) error {
	p, err := parseParams(decl)
	if err != nil {
		return err
	}
	return p.validate()
}

// DriftSeverity: a misconfigured or missing timer is a config-level
// mismatch (MEDIUM) — we can't know whether the job it runs is
// critical, so we don't claim HIGH. A timer declared absent but
// present is more suspicious — HIGH, mirroring the file/link modules.
// Operators override via `severity:`.
func (m *Module) DriftSeverity(decl *statemgmt.Declaration, _ *statemgmt.ModuleCheckResult) statemgmt.DriftSeverity {
	if decl != nil && decl.State == StateAbsent {
		return statemgmt.DriftSeverityHigh
	}
	return statemgmt.DriftSeverityMedium
}

func (m *Module) Check(ctx context.Context, decl *statemgmt.Declaration) (*statemgmt.ModuleCheckResult, error) {
	p, err := parseParams(decl)
	if err != nil {
		return nil, err
	}
	if err := p.validate(); err != nil {
		return nil, err
	}
	return m.check(ctx, p)
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
	changed, diff, err := m.apply(ctx, p)
	if err != nil {
		return &statemgmt.StateResult{
			Success:  false,
			Changed:  false,
			Diff:     diff,
			Duration: time.Since(start),
		}, err
	}
	if !changed {
		return &statemgmt.StateResult{
			Success:  true,
			Changed:  false,
			Comment:  "already converged",
			Duration: time.Since(start),
		}, nil
	}
	return &statemgmt.StateResult{
		Success:  true,
		Changed:  true,
		Diff:     diff,
		Comment:  "applied",
		Duration: time.Since(start),
	}, nil
}

func (m *Module) Test(ctx context.Context, decl *statemgmt.Declaration) (bool, error) {
	res, err := m.Check(ctx, decl)
	if err != nil {
		return false, err
	}
	return res.Matches, nil
}

func (m *Module) check(ctx context.Context, p *params) (*statemgmt.ModuleCheckResult, error) {
	unit := timerUnitName(p)
	content, exists, err := m.provider.ReadUnit(unit)
	if err != nil {
		return nil, err
	}

	if p.State == StateAbsent {
		if !exists {
			return &statemgmt.ModuleCheckResult{Matches: true}, nil
		}
		return &statemgmt.ModuleCheckResult{Matches: false, Diff: "timer unit " + unit + " present; want absent"}, nil
	}

	// state == present
	want := renderTimerUnit(p)
	if !exists {
		return &statemgmt.ModuleCheckResult{Matches: false, Diff: "timer unit " + unit + " missing"}, nil
	}
	if content != want {
		return &statemgmt.ModuleCheckResult{Matches: false, Diff: "timer unit " + unit + " content differs"}, nil
	}
	st, err := m.provider.Status(ctx, unit)
	if err != nil {
		return nil, err
	}
	if p.Enable {
		if !st.Enabled || !st.Active {
			return &statemgmt.ModuleCheckResult{
				Matches: false,
				Diff:    fmt.Sprintf("timer %s: enabled=%v active=%v; want enabled+active", unit, st.Enabled, st.Active),
			}, nil
		}
	} else {
		if st.Enabled || st.Active {
			return &statemgmt.ModuleCheckResult{
				Matches: false,
				Diff:    fmt.Sprintf("timer %s: enabled=%v active=%v; want disabled+inactive", unit, st.Enabled, st.Active),
			}, nil
		}
	}
	return &statemgmt.ModuleCheckResult{Matches: true}, nil
}

func (m *Module) apply(ctx context.Context, p *params) (changed bool, diff string, err error) {
	unit := timerUnitName(p)
	content, exists, err := m.provider.ReadUnit(unit)
	if err != nil {
		return false, "", err
	}

	if p.State == StateAbsent {
		if !exists {
			return false, "", nil
		}
		// Best-effort disable+stop before removal so a running timer
		// doesn't linger; the file exists, so a real error here is a
		// real error.
		if err := m.provider.DisableStop(ctx, unit); err != nil {
			return false, "", fmt.Errorf("disable timer %s: %w", unit, err)
		}
		if err := m.provider.RemoveUnit(unit); err != nil {
			return false, "", err
		}
		if err := m.provider.DaemonReload(ctx); err != nil {
			return false, "", err
		}
		return true, "removed timer unit " + unit, nil
	}

	// state == present
	want := renderTimerUnit(p)
	changedFile := false
	if !exists || content != want {
		if err := m.provider.WriteUnit(unit, want); err != nil {
			return false, "", err
		}
		if err := m.provider.DaemonReload(ctx); err != nil {
			return false, "", err
		}
		changedFile = true
	}
	st, err := m.provider.Status(ctx, unit)
	if err != nil {
		return false, "", err
	}
	changedState := false
	if p.Enable {
		if !st.Enabled || !st.Active {
			if err := m.provider.EnableNow(ctx, unit); err != nil {
				return false, "", err
			}
			changedState = true
		}
	} else {
		if st.Enabled || st.Active {
			if err := m.provider.DisableStop(ctx, unit); err != nil {
				return false, "", err
			}
			changedState = true
		}
	}
	if !changedFile && !changedState {
		return false, "", nil
	}
	switch {
	case changedFile && changedState:
		if p.Enable {
			return true, "wrote " + unit + " + enabled+started", nil
		}
		return true, "wrote " + unit + " + disabled+stopped", nil
	case changedFile:
		return true, "wrote " + unit, nil
	default:
		if p.Enable {
			return true, "enabled+started " + unit, nil
		}
		return true, "disabled+stopped " + unit, nil
	}
}
