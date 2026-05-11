// Package pkg implements the `package` stdlib state module — Linux
// package management per PROJECT-DETAILS §4.8.
//
// State semantics:
//
//	installed — package present at any version (or pinned via `version:`)
//	absent    — package not installed
//
// Backends:
//
//	v1.0: apt (Debian / Ubuntu)
//	v1.x (V1X-BACKLOG): dnf, apk, zypper, pacman
//
// The Go directory + package is `pkg` because `package` is a Go
// keyword; the stdlib registers under the operator-facing name
// "package". Salt convention.
//
// v1.0 out of scope (V1X candidates):
//   - `latest` state (cache-refresh logic)
//   - `refresh: true` for explicit `apt-get update`
//   - Package holds / pinning (`apt-mark hold`)
//   - Purge (vs remove)
//   - Repository / source management (a separate module)
//   - macOS (Homebrew) / Windows (Chocolatey, winget)
package pkg

import (
	"context"
	"fmt"
	"time"

	"go.keystone-core.io/keystone-core/internal/statemgmt"
)

// Module is the package state module.
type Module struct {
	provider Provider
}

// New selects the platform's real Provider via auto-detection.
func New() statemgmt.Module { return &Module{provider: defaultProvider()} }

// NewWithProvider is the test injection point.
func NewWithProvider(p Provider) statemgmt.Module { return &Module{provider: p} }

func (m *Module) Name() string { return "package" }

func (m *Module) ValidStates() []string {
	return []string{StateInstalled, StateAbsent}
}

func (m *Module) Validate(decl *statemgmt.Declaration) error {
	p, err := parseParams(decl)
	if err != nil {
		return err
	}
	return p.validate()
}

func (m *Module) DriftSeverity(decl *statemgmt.Declaration, _ *statemgmt.ModuleCheckResult) statemgmt.DriftSeverity {
	if decl != nil && decl.State == StateAbsent {
		return statemgmt.DriftSeverityHigh
	}
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
	live, err := m.provider.Lookup(p.Name)
	if err != nil {
		return nil, err
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

func (m *Module) doApply(ctx context.Context, p *params, live *PkgInfo) error {
	switch p.State {
	case StateInstalled:
		// Install is also the upgrade/downgrade path — apt-get
		// install <pkg>=<version> handles both. Live state is
		// passed for symmetry with other modules but not
		// consulted (the version-mismatch decision happens in
		// diffCheck).
		_ = live
		return m.provider.Install(ctx, p.Name, p.Version)
	case StateAbsent:
		if live == nil || !live.Installed {
			return nil
		}
		return m.provider.Remove(ctx, p.Name)
	default:
		return fmt.Errorf("apply: unknown state %q", p.State)
	}
}

func (m *Module) Test(ctx context.Context, decl *statemgmt.Declaration) (bool, error) {
	res, err := m.Check(ctx, decl)
	if err != nil {
		return false, err
	}
	return res.Matches, nil
}

// diffCheck is the pure-function compare. Returns a populated
// ModuleCheckResult; never errors.
func diffCheck(p *params, live *PkgInfo) *statemgmt.ModuleCheckResult {
	switch p.State {
	case StateAbsent:
		if live == nil || !live.Installed {
			return &statemgmt.ModuleCheckResult{Matches: true}
		}
		return &statemgmt.ModuleCheckResult{
			Matches: false,
			Diff:    fmt.Sprintf("package %s installed (version=%s); want absent", live.Name, live.Version),
		}
	case StateInstalled:
		if live == nil || !live.Installed {
			return &statemgmt.ModuleCheckResult{
				Matches: false,
				Diff:    fmt.Sprintf("package %s not installed; want present", p.Name),
			}
		}
		if p.Version != "" && live.Version != p.Version {
			return &statemgmt.ModuleCheckResult{
				Matches: false,
				Diff:    fmt.Sprintf("version %s → %s", live.Version, p.Version),
			}
		}
		return &statemgmt.ModuleCheckResult{Matches: true}
	}
	return &statemgmt.ModuleCheckResult{Matches: false, Diff: "unknown state"}
}
