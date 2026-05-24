// SPDX-License-Identifier: Apache-2.0

// Package link implements the `link` stdlib state module — managing
// symbolic and hard links on the agent's filesystem per
// PROJECT-DETAILS §4.8 (Storage category).
//
// It complements the `file` module: `file` can manage a symlink as
// one of its states, but `link` adds hard-link support and a
// dedicated, two-state model when the resource you care about is the
// link itself.
//
// State semantics:
//
//	present — a link at Name pointing at `target`. `kind: symlink`
//	          (default) creates a symbolic link; `kind: hard`
//	          creates a hard link (target must exist, must be on the
//	          same filesystem). `force: true` allows replacing an
//	          existing non-matching file at Name; a directory at
//	          Name is never auto-removed.
//	absent  — nothing exists at Name. A directory at Name is left
//	          in place with an error (use the `file` module).
//
// Symlink targets are compared and stored verbatim — a relative
// target is not resolved against Name's directory. v1.0 out of
// scope (v0.x candidates):
//   - Relative-target normalisation / canonicalisation
//   - owner/group on the link itself (use the `file` module's
//     symlink state when link ownership matters)
//   - Windows links (Linux + macOS only in v1.0)
package link

import (
	"context"
	"fmt"
	"os"
	"time"

	"go.keystone-core.io/keystone-core/internal/statemgmt"
)

// New is the Factory registered with the engine Registry.
func New() statemgmt.Module { return &Module{} }

// Module is the link state module. It is stateless; concurrent
// Check/Apply/Test calls on different Declarations are safe.
type Module struct{}

func (m *Module) Name() string { return "link" }

func (m *Module) ValidStates() []string { return []string{StatePresent, StateAbsent} }

func (m *Module) Validate(decl *statemgmt.Declaration) error {
	p, err := parseParams(decl)
	if err != nil {
		return err
	}
	return p.validate()
}

// DriftSeverity: a missing or wrong link is usually a config-level
// mismatch (MEDIUM). A link declared absent but present is more
// suspicious — treat as HIGH, mirroring the file module.
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
	return checkLink(p)
}

func (m *Module) Apply(_ context.Context, decl *statemgmt.Declaration) (*statemgmt.StateResult, error) {
	start := time.Now()
	p, err := parseParams(decl)
	if err != nil {
		return nil, err
	}
	if err := p.validate(); err != nil {
		return nil, err
	}

	pre, err := checkLink(p)
	if err != nil {
		return nil, err
	}
	if pre.Matches {
		// Apply called despite no drift (runner skipped Check, e.g.
		// on retry) — honour idempotency.
		return &statemgmt.StateResult{
			Success:  true,
			Changed:  false,
			Comment:  "already converged",
			Duration: time.Since(start),
		}, nil
	}

	if err := applyLink(p); err != nil {
		return &statemgmt.StateResult{
			Success:  false,
			Changed:  false,
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

func (m *Module) Test(ctx context.Context, decl *statemgmt.Declaration) (bool, error) {
	res, err := m.Check(ctx, decl)
	if err != nil {
		return false, err
	}
	return res.Matches, nil
}

// checkLink is the pure compare. It performs no mutations.
func checkLink(p *params) (*statemgmt.ModuleCheckResult, error) {
	li, err := inspect(p.Path)
	if err != nil {
		return nil, err
	}

	if p.State == StateAbsent {
		if li.Kind == kindMissing {
			return &statemgmt.ModuleCheckResult{Matches: true}, nil
		}
		return &statemgmt.ModuleCheckResult{
			Matches: false,
			Diff:    fmt.Sprintf("exists as %s; want absent", li.Kind),
		}, nil
	}

	// state == present
	switch p.Kind {
	case KindSymlink:
		switch li.Kind {
		case kindMissing:
			return &statemgmt.ModuleCheckResult{Matches: false, Diff: "missing; want symlink → " + p.Target}, nil
		case kindSymlink:
			if li.SymlinkTarget == p.Target {
				return &statemgmt.ModuleCheckResult{Matches: true}, nil
			}
			return &statemgmt.ModuleCheckResult{
				Matches: false,
				Diff:    fmt.Sprintf("symlink target %q → %q", li.SymlinkTarget, p.Target),
			}, nil
		default:
			return &statemgmt.ModuleCheckResult{
				Matches: false,
				Diff:    fmt.Sprintf("exists as %s; want symlink → %s", li.Kind, p.Target),
			}, nil
		}

	case KindHard:
		same, err := sameInode(p.Path, p.Target)
		if err != nil {
			return nil, err
		}
		if same {
			return &statemgmt.ModuleCheckResult{Matches: true}, nil
		}
		if li.Kind == kindMissing {
			return &statemgmt.ModuleCheckResult{Matches: false, Diff: "missing; want hard link → " + p.Target}, nil
		}
		if li.Kind == kindRegular {
			return &statemgmt.ModuleCheckResult{
				Matches: false,
				Diff:    fmt.Sprintf("regular file, not a hard link to %s", p.Target),
			}, nil
		}
		return &statemgmt.ModuleCheckResult{
			Matches: false,
			Diff:    fmt.Sprintf("exists as %s; want hard link → %s", li.Kind, p.Target),
		}, nil
	}
	return &statemgmt.ModuleCheckResult{Matches: false, Diff: "unknown kind"}, nil
}

// applyLink converges the filesystem to p. Only called when checkLink
// reported drift.
func applyLink(p *params) error {
	li, err := inspect(p.Path)
	if err != nil {
		return err
	}

	if p.State == StateAbsent {
		switch li.Kind {
		case kindMissing:
			return nil
		case kindDirectory:
			return fmt.Errorf("refusing to remove directory %s; use the file module", p.Path)
		default:
			if err := os.Remove(p.Path); err != nil {
				return fmt.Errorf("remove %s: %w", p.Path, err)
			}
			return nil
		}
	}

	// state == present
	switch p.Kind {
	case KindSymlink:
		return applySymlink(p, li)
	case KindHard:
		return applyHardLink(p, li)
	default:
		return fmt.Errorf("apply: unknown kind %q", p.Kind)
	}
}

func applySymlink(p *params, li *linkInfo) error {
	switch li.Kind {
	case kindMissing:
		// straight create
	case kindSymlink:
		if li.SymlinkTarget == p.Target {
			return nil
		}
		// wrong target — always safe to replace a symlink
		if err := os.Remove(p.Path); err != nil {
			return fmt.Errorf("remove old symlink %s: %w", p.Path, err)
		}
	default:
		if !p.Force {
			return fmt.Errorf("cannot create symlink at %s: exists as %s (set force: true to replace)", p.Path, li.Kind)
		}
		if err := removeForReplace(p.Path, li); err != nil {
			return err
		}
	}
	if err := os.Symlink(p.Target, p.Path); err != nil {
		return fmt.Errorf("symlink %s → %s: %w", p.Path, p.Target, err)
	}
	return nil
}

func applyHardLink(p *params, li *linkInfo) error {
	// Validate the target up front so we fail with a clear message
	// rather than os.Link's terse errno.
	if _, err := os.Lstat(p.Target); err != nil {
		return fmt.Errorf("hard-link target %s: %w", p.Target, err)
	}
	switch li.Kind {
	case kindMissing:
		// straight create
	case kindRegular:
		same, err := sameInode(p.Path, p.Target)
		if err != nil {
			return err
		}
		if same {
			return nil
		}
		if !p.Force {
			return fmt.Errorf("cannot create hard link at %s: a different regular file is there (set force: true to replace)", p.Path)
		}
		if err := os.Remove(p.Path); err != nil {
			return fmt.Errorf("remove %s: %w", p.Path, err)
		}
	default:
		if !p.Force {
			return fmt.Errorf("cannot create hard link at %s: exists as %s (set force: true to replace)", p.Path, li.Kind)
		}
		if err := removeForReplace(p.Path, li); err != nil {
			return err
		}
	}
	if err := os.Link(p.Target, p.Path); err != nil {
		// EXDEV (cross-device) is the common, confusing failure —
		// surface it with context.
		return fmt.Errorf("hard link %s → %s: %w (hard links must be on the same filesystem)", p.Path, p.Target, err)
	}
	return nil
}
