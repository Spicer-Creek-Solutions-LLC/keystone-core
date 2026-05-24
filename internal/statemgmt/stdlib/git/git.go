// SPDX-License-Identifier: Apache-2.0

// Package git implements the `git` stdlib state module — managing a
// git working tree on the agent per PROJECT-DETAILS §4.8 (Files &
// VCS category).
//
// Declaration.Name is the working-tree path. `url` is the repository
// URL (required for present/latest). `rev` is the branch / tag / SHA
// to track (default: the remote's default branch). `depth` requests
// a shallow clone. `remote` names the remote (default "origin").
// `force` lets `latest` discard local changes (it defaults to true
// for `latest`, since that state is a declarative "match the remote"
// instruction).
//
// States:
//
//	present — a working tree at Name, cloned from `url`, with the
//	          remote pointing at `url`. The checked-out revision is
//	          set from `rev` on the initial clone but not enforced
//	          thereafter.
//	latest  — like present, and HEAD equals `rev` on the remote.
//	          Apply fetches and (when force) hard-resets the current
//	          ref to the fetched commit. It does not switch branches.
//	absent  — Name does not exist. A non-repo directory at Name is
//	          left in place with ErrNotARepo.
//
// The module shells out to the `git` binary via a Provider; when git
// is not installed, mutating operations fail with ErrGitNotFound.
//
// v0.1 out of scope (v0.x candidates):
//   - Authentication: deploy keys, credential helpers, token-in-URL
//     rotation, SSH host-key management (v1.0 relies on whatever the
//     agent's git/SSH config already provides).
//   - Submodules, sparse checkout, partial clone, bare repos.
//   - Branch tracking on `latest` (it updates the current ref, not
//     a named local branch); reliable shallow + arbitrary-SHA clones.
//   - Server-side rev resolution for the `present` up-to-date check
//     (present deliberately does not contact the remote).
package git

import (
	"context"
	"fmt"
	"strings"
	"time"

	"go.keystone-core.io/keystone-core/internal/statemgmt"
)

// New is the Factory registered with the engine Registry.
func New() statemgmt.Module { return &Module{provider: defaultProvider()} }

// NewWithProvider is the test injection point.
func NewWithProvider(p Provider) statemgmt.Module { return &Module{provider: p} }

// Module is the git state module. The Provider is resolved per
// instance (the Registry hands out a fresh module per run).
type Module struct {
	provider Provider
}

func (m *Module) Name() string { return "git" }

func (m *Module) ValidStates() []string { return []string{StatePresent, StateLatest, StateAbsent} }

func (m *Module) Validate(decl *statemgmt.Declaration) error {
	p, err := parseParams(decl)
	if err != nil {
		return err
	}
	return p.validate()
}

// DriftSeverity: a missing / wrong-remote working tree usually means
// deployed code is wrong — HIGH. A repo declared absent but present
// is also HIGH (unexpected resource). The remaining cases are at
// most a stale checkout — MEDIUM.
func (m *Module) DriftSeverity(decl *statemgmt.Declaration, check *statemgmt.ModuleCheckResult) statemgmt.DriftSeverity {
	if decl == nil {
		return statemgmt.DriftSeverityMedium
	}
	switch decl.State {
	case StateAbsent:
		return statemgmt.DriftSeverityHigh
	case StatePresent:
		return statemgmt.DriftSeverityHigh
	case StateLatest:
		// "not cloned" / "wrong remote" is HIGH; "behind" is MEDIUM.
		if check != nil && (strings.Contains(check.Diff, "not cloned") || strings.Contains(check.Diff, "remote url")) {
			return statemgmt.DriftSeverityHigh
		}
		return statemgmt.DriftSeverityMedium
	default:
		return statemgmt.DriftSeverityMedium
	}
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

	pre, err := m.check(ctx, p)
	if err != nil {
		return nil, err
	}
	if pre.Matches {
		return &statemgmt.StateResult{
			Success:  true,
			Changed:  false,
			Comment:  "already converged",
			Duration: time.Since(start),
		}, nil
	}

	if err := m.apply(ctx, p); err != nil {
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

func (m *Module) check(ctx context.Context, p *params) (*statemgmt.ModuleCheckResult, error) {
	st, err := m.provider.Inspect(ctx, p.Dir, p.Remote)
	if err != nil {
		return nil, err
	}

	if p.State == StateAbsent {
		if !st.Exists {
			return &statemgmt.ModuleCheckResult{Matches: true}, nil
		}
		return &statemgmt.ModuleCheckResult{Matches: false, Diff: "repo present; want absent"}, nil
	}

	// present / latest
	if !st.Exists {
		return &statemgmt.ModuleCheckResult{Matches: false, Diff: "not cloned; want " + p.URL}, nil
	}
	if st.RemoteURL != p.URL {
		return &statemgmt.ModuleCheckResult{
			Matches: false,
			Diff:    fmt.Sprintf("remote url %q → %q", st.RemoteURL, p.URL),
		}, nil
	}
	if p.State == StatePresent {
		return &statemgmt.ModuleCheckResult{Matches: true}, nil
	}

	// state == latest: compare HEAD against the target commit.
	target := p.Rev
	if !isFullSHA(p.Rev) {
		sha, err := m.provider.RemoteSHA(ctx, p.Dir, p.Remote, p.Rev)
		if err != nil {
			return nil, err
		}
		target = sha
	}
	if st.HeadSHA == target {
		return &statemgmt.ModuleCheckResult{Matches: true}, nil
	}
	return &statemgmt.ModuleCheckResult{
		Matches: false,
		Diff:    fmt.Sprintf("HEAD %s → %s (%s)", shortSHA(st.HeadSHA), shortSHA(target), p.Rev),
	}, nil
}

func (m *Module) apply(ctx context.Context, p *params) error {
	st, err := m.provider.Inspect(ctx, p.Dir, p.Remote)
	if err != nil {
		return err
	}

	if p.State == StateAbsent {
		if !st.Exists {
			return nil
		}
		return m.provider.Remove(p.Dir)
	}

	// present / latest
	if !st.Exists {
		// A non-repo directory blocking the clone path: refuse
		// (we don't know what's in it). Inspect reports Exists
		// false for both "missing" and "not a repo"; distinguish
		// via the filesystem so the operator gets a clear error.
		if dirNonEmpty(p.Dir) {
			return fmt.Errorf("%w: %s (move it aside first)", ErrNotARepo, p.Dir)
		}
		return m.provider.Clone(ctx, CloneOptions{
			URL:    p.URL,
			Dir:    p.Dir,
			Rev:    p.Rev,
			Depth:  p.Depth,
			Remote: p.Remote,
		})
	}
	if st.RemoteURL != p.URL {
		// A repo with the wrong remote URL: don't silently
		// re-point it (could be a different project entirely).
		return fmt.Errorf("repo at %s has remote %q, want %q (resolve manually or declare it absent first)", p.Dir, st.RemoteURL, p.URL)
	}
	if p.State == StatePresent {
		return nil // already converged (Check would have matched)
	}
	return m.provider.Sync(ctx, SyncOptions{
		Dir:    p.Dir,
		Rev:    p.Rev,
		Depth:  p.Depth,
		Remote: p.Remote,
		Force:  p.Force,
	})
}

func shortSHA(s string) string {
	if len(s) > 8 {
		return s[:8]
	}
	if s == "" {
		return "(none)"
	}
	return s
}
