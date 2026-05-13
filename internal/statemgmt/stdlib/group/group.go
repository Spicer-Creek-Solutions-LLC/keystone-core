// Package group implements the `group` stdlib state module —
// Unix group management per PROJECT-DETAILS §4.8.
//
// State semantics:
//
//	present — group exists with the declared gid (when set)
//	absent  — group does not exist
//
// Platform support: Linux for the full Check / Apply / Test
// pipeline. Cross-platform Lookup (read-only) works on macOS / BSD
// because the underlying NSS surface (os/user.LookupGroup) does.
// Mutating Apply paths on non-Linux return ErrUnsupportedOS — the
// module is genuinely a Linux v1.0 surface and we don't pretend
// otherwise.
//
// v0.1 out of scope (v0.x candidates):
//   - macOS via `dscl` (different beast; needs a parallel provider)
//   - `force` deletion when the group is a user's primary group
//   - `members:` (better handled by user.groups:)
//   - NIS / LDAP / SSSD mutations (NSS Lookup already works through them)
//   - Windows group management
package group

import (
	"context"
	"errors"
	"fmt"
	"time"

	"go.keystone-core.io/keystone-core/internal/statemgmt"
)

// Module is the group state module.
type Module struct {
	provider Provider
}

// New is the Factory registered with the engine Registry. Selects
// the platform's real Provider.
func New() statemgmt.Module {
	return &Module{provider: defaultProvider()}
}

// NewWithProvider is the test injection point. Production callers
// use New; tests construct *Module directly with a fakeProvider so
// Apply paths exercise without needing root.
func NewWithProvider(p Provider) statemgmt.Module {
	return &Module{provider: p}
}

func (m *Module) Name() string { return "group" }

func (m *Module) ValidStates() []string {
	return []string{StatePresent, StateAbsent}
}

// Validate enforces the cross-field shape checks the engine
// Validator can't infer. Engine catches non-empty Name + State ∈
// ValidStates() before this fires.
func (m *Module) Validate(decl *statemgmt.Declaration) error {
	p, err := parseParams(decl)
	if err != nil {
		return err
	}
	return p.validate()
}

// DriftSeverity:
//   - state=absent but the group exists → HIGH (unexpected resource)
//   - any other drift → MEDIUM
func (m *Module) DriftSeverity(decl *statemgmt.Declaration, _ *statemgmt.ModuleCheckResult) statemgmt.DriftSeverity {
	if decl != nil && decl.State == StateAbsent {
		return statemgmt.DriftSeverityHigh
	}
	return statemgmt.DriftSeverityMedium
}

// Check inspects the live system. Read-only.
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

// Apply converges the system to the Declaration. Lookups still
// happen pre-mutation so we can detect already-converged + pick
// add-vs-modify.
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

func (m *Module) doApply(ctx context.Context, p *params, live *GroupInfo) error {
	switch p.State {
	case StatePresent:
		if live == nil {
			return m.provider.Add(ctx, p.Name, p.GID, p.System)
		}
		// Live group exists; reconcile gid if declared.
		if p.GID != nil && live.GID != *p.GID {
			return m.provider.Mod(ctx, p.Name, *p.GID)
		}
		return nil
	case StateAbsent:
		if live == nil {
			return nil
		}
		return m.provider.Del(ctx, p.Name)
	default:
		return fmt.Errorf("apply: unknown state %q", p.State)
	}
}

// Test re-Checks post-Apply.
func (m *Module) Test(ctx context.Context, decl *statemgmt.Declaration) (bool, error) {
	res, err := m.Check(ctx, decl)
	if err != nil {
		return false, err
	}
	return res.Matches, nil
}

// diffCheck is the pure-function compare between declared params
// and live group info. Returns a populated ModuleCheckResult; never
// errors.
func diffCheck(p *params, live *GroupInfo) *statemgmt.ModuleCheckResult {
	switch p.State {
	case StateAbsent:
		if live == nil {
			return &statemgmt.ModuleCheckResult{Matches: true}
		}
		return &statemgmt.ModuleCheckResult{
			Matches: false,
			Diff:    fmt.Sprintf("group %s exists (gid=%d); want absent", live.Name, live.GID),
		}
	case StatePresent:
		if live == nil {
			return &statemgmt.ModuleCheckResult{
				Matches: false,
				Diff:    fmt.Sprintf("group %s missing; want present", p.Name),
			}
		}
		if p.GID != nil && live.GID != *p.GID {
			return &statemgmt.ModuleCheckResult{
				Matches: false,
				Diff:    fmt.Sprintf("gid %d → %d", live.GID, *p.GID),
			}
		}
		return &statemgmt.ModuleCheckResult{Matches: true}
	}
	return &statemgmt.ModuleCheckResult{Matches: false, Diff: "unknown state"}
}

// IsUnsupportedOS reports whether err originated in the non-Linux
// provider's mutation-blocking path. Exposed so the gRPC server +
// CLI can surface a clearer message than the raw error chain.
func IsUnsupportedOS(err error) bool { return errors.Is(err, ErrUnsupportedOS) }
