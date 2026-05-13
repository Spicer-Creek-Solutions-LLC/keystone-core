// Package kmod implements the `kernel_module` stdlib state module —
// kernel-module management per PROJECT-DETAILS §4.8.
//
// State semantics:
//
//	present — the module <Name> is loaded; when persist:true (the
//	          default) a keystone-managed /etc/modules-load.d entry
//	          ensures it loads at boot.
//	absent  — the module is not loaded; when persist:true, the
//	          keystone-managed modules-load entry is removed so a
//	          reboot doesn't bring it back.
//
// Name accepts dashed (br-netfilter) or underscored (br_netfilter)
// forms; the module normalises dashes → underscores (the kernel's
// internal form) so /proc/modules comparisons and persist filenames
// are stable.
//
// v0.1 out of scope (v0.x candidates):
//   - modprobe options / module load-time parameters
//     (modprobe <name> opt=val)
//   - /etc/modprobe.d blacklist management
//   - Non-Linux (no kernel-module concept)
package kmod

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

func New() statemgmt.Module { return &Module{provider: defaultProvider()} }

// NewWithProvider is the test injection point.
func NewWithProvider(p Provider) statemgmt.Module { return &Module{provider: p} }

func (m *Module) Name() string { return "kernel_module" }

func (m *Module) ValidStates() []string { return []string{StatePresent, StateAbsent} }

func (m *Module) Validate(decl *statemgmt.Declaration) error {
	p, err := parseParams(decl)
	if err != nil {
		return err
	}
	return p.validate()
}

// DriftSeverity: a required module being absent (or an unexpected
// one loaded) is a functional break — HIGH. A persist-only mismatch
// → MEDIUM.
func (m *Module) DriftSeverity(decl *statemgmt.Declaration, check *statemgmt.ModuleCheckResult) statemgmt.DriftSeverity {
	if check != nil && check.Diff != "" && !strings.Contains(check.Diff, "loaded") {
		// Only persist drift mentioned.
		return statemgmt.DriftSeverityMedium
	}
	_ = decl
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
	return m.check(p)
}

func (m *Module) check(p *params) (*statemgmt.ModuleCheckResult, error) {
	loaded, err := m.provider.Loaded(p.Name)
	if err != nil {
		return nil, err
	}
	var persistExists bool
	if p.Persist {
		persistExists, err = m.provider.PersistExists(p.Name)
		if err != nil {
			return nil, err
		}
	}

	var diffs []string
	switch p.State {
	case StatePresent:
		if !loaded {
			diffs = append(diffs, "not loaded; want loaded")
		}
		if p.Persist && !persistExists {
			diffs = append(diffs, "persist entry missing")
		}
	case StateAbsent:
		if loaded {
			diffs = append(diffs, "loaded; want not loaded")
		}
		if p.Persist && persistExists {
			diffs = append(diffs, "persist entry present (would reload at boot)")
		}
	}
	if len(diffs) == 0 {
		return &statemgmt.ModuleCheckResult{Matches: true}, nil
	}
	return &statemgmt.ModuleCheckResult{Matches: false, Diff: strings.Join(diffs, "; ")}, nil
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
	if err := m.doApply(ctx, p); err != nil {
		return &statemgmt.StateResult{Success: false, Diff: pre.Diff, Duration: time.Since(start)}, err
	}
	return &statemgmt.StateResult{Success: true, Changed: true, Diff: pre.Diff, Comment: "applied", Duration: time.Since(start)}, nil
}

func (m *Module) doApply(ctx context.Context, p *params) error {
	loaded, err := m.provider.Loaded(p.Name)
	if err != nil {
		return err
	}
	switch p.State {
	case StatePresent:
		if !loaded {
			if err := m.provider.Load(ctx, p.Name); err != nil {
				return err
			}
		}
		if p.Persist {
			exists, err := m.provider.PersistExists(p.Name)
			if err != nil {
				return err
			}
			if !exists {
				if err := m.provider.AddPersist(p.Name); err != nil {
					return err
				}
			}
		}
		return nil
	case StateAbsent:
		if loaded {
			if err := m.provider.Unload(ctx, p.Name); err != nil {
				return err
			}
		}
		if p.Persist {
			exists, err := m.provider.PersistExists(p.Name)
			if err != nil {
				return err
			}
			if exists {
				if err := m.provider.RemovePersist(p.Name); err != nil {
					return err
				}
			}
		}
		return nil
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
