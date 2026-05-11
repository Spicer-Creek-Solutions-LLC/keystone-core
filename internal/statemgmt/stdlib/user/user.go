// Package user implements the `user` stdlib state module — Linux
// user-account management per PROJECT-DETAILS §4.8.
//
// State semantics:
//
//	present — user exists with declared scalar fields + supplementary groups
//	absent  — user does not exist
//
// Platform support: Linux for the full Check / Apply / Test pipeline.
// Cross-platform Lookup (read-only) works wherever os/user does.
// Mutating Apply paths on non-Linux return ErrUnsupportedOS.
//
// Apply runs two Provider methods when needed:
//   - Mod for scalar changes (UID/GID/Group/Home/Shell/Comment)
//   - SetGroups for supplementary-group replacement
//
// Each Provider call is independently testable; partial changes
// fire only the relevant call.
//
// v1.0 out of scope (V1X candidates):
//   - macOS via `dscl`
//   - Password / SSH key management (separate modules)
//   - Account expiration (`chage`)
//   - Sudoers integration
//   - Skeleton-dir overrides
package user

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"go.keystone-core.io/keystone-core/internal/statemgmt"
)

// Module is the user state module.
type Module struct {
	provider Provider
}

// New selects the platform's real Provider.
func New() statemgmt.Module { return &Module{provider: defaultProvider()} }

// NewWithProvider is the test injection point.
func NewWithProvider(p Provider) statemgmt.Module { return &Module{provider: p} }

func (m *Module) Name() string { return "user" }

func (m *Module) ValidStates() []string {
	return []string{StatePresent, StateAbsent}
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

// doApply runs the right Provider operations. Each is independently
// fireable so a partial change (e.g., only supplementary groups
// drifted) doesn't trigger a usermod that touches the scalar fields.
func (m *Module) doApply(ctx context.Context, p *params, live *UserInfo) error {
	switch p.State {
	case StatePresent:
		if live == nil {
			return m.provider.Add(ctx, p.toAddOptions())
		}
		// Scalar diff: anything in (UID, GID, Group, Home, Shell,
		// Comment) that changed?
		modOpts, scalarChanged := scalarDiff(p, live)
		if scalarChanged {
			if err := m.provider.Mod(ctx, modOpts); err != nil {
				return err
			}
		}
		if p.HasGroups && !groupsEqual(p.Groups, live.Groups) {
			if err := m.provider.SetGroups(ctx, p.Name, sortedCopy(p.Groups)); err != nil {
				return err
			}
		}
		return nil
	case StateAbsent:
		if live == nil {
			return nil
		}
		return m.provider.Del(ctx, p.Name, p.RemoveHome)
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

// diffCheck produces the ModuleCheckResult. Never errors.
func diffCheck(p *params, live *UserInfo) *statemgmt.ModuleCheckResult {
	switch p.State {
	case StateAbsent:
		if live == nil {
			return &statemgmt.ModuleCheckResult{Matches: true}
		}
		return &statemgmt.ModuleCheckResult{
			Matches: false,
			Diff:    fmt.Sprintf("user %s exists (uid=%d); want absent", live.Name, live.UID),
		}
	case StatePresent:
		if live == nil {
			return &statemgmt.ModuleCheckResult{
				Matches: false,
				Diff:    fmt.Sprintf("user %s missing; want present", p.Name),
			}
		}
		var diffs []string
		if p.UID != nil && live.UID != *p.UID {
			diffs = append(diffs, fmt.Sprintf("uid %d → %d", live.UID, *p.UID))
		}
		if p.GID != nil && live.GID != *p.GID {
			diffs = append(diffs, fmt.Sprintf("gid %d → %d", live.GID, *p.GID))
		}
		if p.Group != "" {
			// Compare live.GID's group name against declared
			// group name. We don't have the group-name on
			// UserInfo (only the GID); a separate resolve step
			// would couple this module to the group module's
			// lookup. For v1.0, treat any non-empty declared
			// Group as drift unless the GID happens to match —
			// we trust the operator to use either gid OR group,
			// not both, and we surface the comparison as
			// "group %s declared" so they can verify.
			if n, err := strconv.Atoi(p.Group); err == nil {
				if live.GID != n {
					diffs = append(diffs, fmt.Sprintf("gid %d → %d (via group=%s)", live.GID, n, p.Group))
				}
			} else {
				if name, err := groupNameForGID(strconv.Itoa(live.GID)); err == nil {
					if name != p.Group {
						diffs = append(diffs, fmt.Sprintf("group %s → %s", name, p.Group))
					}
				} else {
					diffs = append(diffs, fmt.Sprintf("group → %s (live gid %d unresolved)", p.Group, live.GID))
				}
			}
		}
		if p.Home != "" && live.Home != p.Home {
			diffs = append(diffs, fmt.Sprintf("home %s → %s", live.Home, p.Home))
		}
		if p.Shell != "" && live.Shell != p.Shell {
			diffs = append(diffs, fmt.Sprintf("shell %s → %s", live.Shell, p.Shell))
		}
		if p.Comment != "" && live.Comment != p.Comment {
			diffs = append(diffs, fmt.Sprintf("comment %q → %q", live.Comment, p.Comment))
		}
		if p.HasGroups && !groupsEqual(p.Groups, live.Groups) {
			diffs = append(diffs, fmt.Sprintf("groups %v → %v", live.Groups, sortedCopy(p.Groups)))
		}
		if len(diffs) == 0 {
			return &statemgmt.ModuleCheckResult{Matches: true}
		}
		return &statemgmt.ModuleCheckResult{Matches: false, Diff: strings.Join(diffs, "; ")}
	}
	return &statemgmt.ModuleCheckResult{Matches: false, Diff: "unknown state"}
}

func (p *params) toAddOptions() AddOptions {
	return AddOptions{
		Name:       p.Name,
		UID:        p.UID,
		GID:        p.GID,
		Group:      p.Group,
		Home:       p.Home,
		Shell:      p.Shell,
		Comment:    p.Comment,
		Groups:     sortedCopy(p.Groups),
		System:     p.System,
		CreateHome: p.CreateHome,
	}
}

// scalarDiff builds a ModOptions populated only with fields that
// differ from live. Reports false in the second return when nothing
// changed (caller skips the Mod call entirely).
func scalarDiff(p *params, live *UserInfo) (ModOptions, bool) {
	opts := ModOptions{Name: p.Name}
	changed := false
	if p.UID != nil && live.UID != *p.UID {
		opts.UID = p.UID
		changed = true
	}
	if p.GID != nil && live.GID != *p.GID {
		opts.GID = p.GID
		changed = true
	}
	if p.Group != "" {
		if n, err := strconv.Atoi(p.Group); err == nil {
			if live.GID != n {
				opts.GID = &n
				changed = true
			}
		} else if name, err := groupNameForGID(strconv.Itoa(live.GID)); err == nil && name != p.Group {
			opts.Group = p.Group
			changed = true
		} else if err != nil {
			// Can't resolve live GID → safest to apply the
			// declared group name and let usermod confirm.
			opts.Group = p.Group
			changed = true
		}
	}
	if p.Home != "" && live.Home != p.Home {
		opts.Home = p.Home
		changed = true
	}
	if p.Shell != "" && live.Shell != p.Shell {
		opts.Shell = p.Shell
		changed = true
	}
	if p.Comment != "" && live.Comment != p.Comment {
		opts.Comment = p.Comment
		changed = true
	}
	return opts, changed
}

// groupsEqual reports whether two group lists are set-equal
// (ignores order; ignores nil vs empty distinction).
func groupsEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	sa := sortedCopy(a)
	sb := sortedCopy(b)
	for i := range sa {
		if sa[i] != sb[i] {
			return false
		}
	}
	return true
}

func sortedCopy(in []string) []string {
	if in == nil {
		return nil
	}
	out := append([]string(nil), in...)
	sort.Strings(out)
	return out
}

// IsUnsupportedOS reports whether err originated in the non-Linux
// provider's mutation-blocking path.
func IsUnsupportedOS(err error) bool { return errors.Is(err, ErrUnsupportedOS) }
