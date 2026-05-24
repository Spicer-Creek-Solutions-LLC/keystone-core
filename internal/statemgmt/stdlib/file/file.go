// SPDX-License-Identifier: Apache-2.0

// Package file implements the `file` stdlib state module — managing
// files, directories, and symlinks on the agent's filesystem per
// PROJECT-DETAILS §4.8.
//
// State semantics:
//
//	present   — regular file with declared content + mode + owner + group
//	absent    — path does not exist
//	directory — directory at path with declared mode + owner + group
//	symlink   — symlink at path pointing to declared target
//
// v0.1 out of scope (v0.x candidates):
//   - HTTP/HTTPS source URLs (content / source are local-only)
//   - Recursive directory state (set perms across a tree)
//   - Backup-on-overwrite (.bak files)
//   - SELinux context / xattr / ACLs
//   - Line-by-line content diff (we report SHA prefixes)
//   - Windows path support (Linux + macOS only in v1.0)
package file

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"go.keystone-core.io/keystone-core/internal/statemgmt"
)

// New is the Factory registered with the engine Registry.
func New() statemgmt.Module { return &Module{} }

// Module is the file state module. It is stateless; concurrent
// Check/Apply/Test calls on different Declarations are safe.
type Module struct{}

func (m *Module) Name() string { return "file" }

func (m *Module) ValidStates() []string {
	return []string{StatePresent, StateAbsent, StateDirectory, StateSymlink}
}

// Validate runs at engine-validate time. Engine already enforces
// non-empty Name + State and State ∈ ValidStates(); we add the
// cross-field shape checks parseParams+validate() encode.
func (m *Module) Validate(decl *statemgmt.Declaration) error {
	p, err := parseParams(decl)
	if err != nil {
		return err
	}
	return p.validate()
}

// DriftSeverity is the optional opt-in interface from Task 7.
// Reasonable defaults so authors don't have to annotate every decl
// with severity:.
func (m *Module) DriftSeverity(decl *statemgmt.Declaration, _ *statemgmt.ModuleCheckResult) statemgmt.DriftSeverity {
	if decl == nil {
		return statemgmt.DriftSeverityMedium
	}
	switch decl.State {
	case StateAbsent:
		// File declared absent but present → a leaked or
		// unexpected resource. Treat as high — operator likely
		// wants to know.
		return statemgmt.DriftSeverityHigh
	case StatePresent, StateDirectory, StateSymlink:
		return statemgmt.DriftSeverityMedium
	default:
		return statemgmt.DriftSeverityMedium
	}
}

// Check inspects the live filesystem against the Declaration. Read-
// only; mutations are exclusively in Apply.
func (m *Module) Check(_ context.Context, decl *statemgmt.Declaration) (*statemgmt.ModuleCheckResult, error) {
	p, err := parseParams(decl)
	if err != nil {
		return nil, err
	}
	if err := p.validate(); err != nil {
		return nil, err
	}
	live, err := readMeta(p.Path)
	if err != nil {
		return nil, err
	}
	return diffCheck(p, live)
}

// Apply converges the filesystem to the Declaration. Apply only
// runs when Check.Matches==false — but it must still be idempotent
// because the engine may invoke it in pipelines that don't
// pre-check (e.g., when the previous Run's Test already verified).
func (m *Module) Apply(_ context.Context, decl *statemgmt.Declaration) (*statemgmt.StateResult, error) {
	start := time.Now()
	p, err := parseParams(decl)
	if err != nil {
		return nil, err
	}
	if err := p.validate(); err != nil {
		return nil, err
	}
	live, err := readMeta(p.Path)
	if err != nil {
		return nil, err
	}

	// Diff before mutation so the result can report what changed.
	pre, err := diffCheck(p, live)
	if err != nil {
		return nil, err
	}
	if pre.Matches {
		// Apply called despite no drift — runner does this when
		// it skips Check (e.g., on retry). Honour idempotency.
		return &statemgmt.StateResult{
			Success:  true,
			Changed:  false,
			Comment:  "already converged",
			Duration: time.Since(start),
		}, nil
	}

	if err := applyOne(p, live); err != nil {
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

// Test re-Checks post-Apply. Returns false (without error) if any
// drift remains — caller treats that as "Test returned false" and
// fails the Declaration.
func (m *Module) Test(ctx context.Context, decl *statemgmt.Declaration) (bool, error) {
	res, err := m.Check(ctx, decl)
	if err != nil {
		return false, err
	}
	return res.Matches, nil
}

// diffCheck is the pure-function compare. Returns a populated
// ModuleCheckResult; never errors.
func diffCheck(p *params, live *meta) (*statemgmt.ModuleCheckResult, error) {
	switch p.State {
	case StateAbsent:
		if live.Type == typeMissing {
			return &statemgmt.ModuleCheckResult{Matches: true}, nil
		}
		return &statemgmt.ModuleCheckResult{
			Matches: false,
			Diff:    fmt.Sprintf("exists as %s; want absent", live.Type),
		}, nil

	case StatePresent:
		if live.Type == typeMissing {
			return &statemgmt.ModuleCheckResult{Matches: false, Diff: "missing; want present"}, nil
		}
		if live.Type != typeRegular {
			return &statemgmt.ModuleCheckResult{
				Matches: false,
				Diff:    fmt.Sprintf("type %s; want regular file", live.Type),
			}, nil
		}
		return diffAttributes(p, live, true), nil

	case StateDirectory:
		if live.Type == typeMissing {
			return &statemgmt.ModuleCheckResult{Matches: false, Diff: "missing; want directory"}, nil
		}
		if live.Type != typeDirectory {
			return &statemgmt.ModuleCheckResult{
				Matches: false,
				Diff:    fmt.Sprintf("type %s; want directory", live.Type),
			}, nil
		}
		return diffAttributes(p, live, false), nil

	case StateSymlink:
		if live.Type == typeMissing {
			return &statemgmt.ModuleCheckResult{Matches: false, Diff: "missing; want symlink"}, nil
		}
		if live.Type != typeSymlink {
			return &statemgmt.ModuleCheckResult{
				Matches: false,
				Diff:    fmt.Sprintf("type %s; want symlink", live.Type),
			}, nil
		}
		if live.SymlinkTarget != p.Target {
			return &statemgmt.ModuleCheckResult{
				Matches: false,
				Diff:    fmt.Sprintf("symlink target %q → %q", live.SymlinkTarget, p.Target),
			}, nil
		}
		return diffAttributes(p, live, false), nil
	}
	return &statemgmt.ModuleCheckResult{Matches: false, Diff: "unknown state"}, nil
}

// diffAttributes compares mode + owner + group + (when checkContent
// is true) content. Returns a Matches=false result if any attribute
// is off; concatenated diff lists every mismatch so an operator
// sees the full picture at once.
func diffAttributes(p *params, live *meta, checkContent bool) *statemgmt.ModuleCheckResult {
	var diffs []string

	if p.Mode != modeUnset && live.Mode != p.Mode {
		diffs = append(diffs, fmt.Sprintf("mode %#o → %#o", live.Mode, p.Mode))
	}
	if !ownerMatches(p.Owner, live) {
		diffs = append(diffs, fmt.Sprintf("owner %s → %s", live.OwnerName, p.Owner))
	}
	if !groupMatches(p.Group, live) {
		diffs = append(diffs, fmt.Sprintf("group %s → %s", live.GroupName, p.Group))
	}
	if checkContent {
		want, err := wantContentHash(p)
		if err == nil && want != "" && live.ContentHash != want {
			diffs = append(diffs,
				fmt.Sprintf("content sha %s → %s", short(live.ContentHash), short(want)))
		}
		if err != nil {
			diffs = append(diffs, fmt.Sprintf("source error: %v", err))
		}
	}
	if len(diffs) == 0 {
		return &statemgmt.ModuleCheckResult{Matches: true}
	}
	return &statemgmt.ModuleCheckResult{
		Matches: false,
		Diff:    strings.Join(diffs, "; "),
	}
}

// wantContentHash returns the SHA-256 of the declared content
// (literal Content or source-file bytes). Returns "" when neither
// content nor source is set (mode/owner-only declaration).
func wantContentHash(p *params) (string, error) {
	if p.HasContent {
		return hashBytes([]byte(p.Content)), nil
	}
	if p.Source != "" {
		data, err := os.ReadFile(p.Source) //nolint:gosec // operator-managed source path
		if err != nil {
			return "", err
		}
		return hashBytes(data), nil
	}
	return "", nil
}

func short(hash string) string {
	if len(hash) > 8 {
		return hash[:8]
	}
	return hash
}

// applyOne dispatches to the per-state mutator. Each mutator
// produces the declared filesystem shape from whatever live state
// happens to be there.
func applyOne(p *params, live *meta) error {
	switch p.State {
	case StateAbsent:
		return applyAbsent(p, live)
	case StatePresent:
		return applyPresent(p, live)
	case StateDirectory:
		return applyDirectory(p, live)
	case StateSymlink:
		return applySymlink(p, live)
	default:
		return fmt.Errorf("apply: unknown state %q", p.State)
	}
}

func applyAbsent(p *params, live *meta) error {
	if live.Type == typeMissing {
		return nil
	}
	if live.Type == typeDirectory {
		// v1.0: refuse to recursively remove a directory. Operator
		// must explicitly nuke the tree before declaring absent.
		// V1X candidate: a `recursive: true` opt-in.
		return fmt.Errorf("cannot remove %s: is a directory (recursive remove is v1.x)", p.Path)
	}
	return os.Remove(p.Path)
}

func applyPresent(p *params, live *meta) error {
	// Existing-but-wrong-type collision: file declared present
	// but path is a dir / symlink. Don't auto-replace — risk of
	// data loss. Operator must resolve.
	if live.Type != typeMissing && live.Type != typeRegular {
		return fmt.Errorf("cannot create regular file at %s: exists as %s", p.Path, live.Type)
	}
	mode := os.FileMode(0o644)
	if p.Mode != modeUnset {
		mode = os.FileMode(p.Mode)
	}

	// Resolve content. When neither content nor source is set
	// and the file already exists, leave bytes alone (we're just
	// adjusting permissions). When the file doesn't exist and no
	// content/source, create empty.
	var content []byte
	switch {
	case p.HasContent:
		content = []byte(p.Content)
	case p.Source != "":
		data, err := os.ReadFile(p.Source) //nolint:gosec // operator-managed
		if err != nil {
			return fmt.Errorf("read source %s: %w", p.Source, err)
		}
		content = data
	case live.Type == typeRegular:
		// no content change — skip the write
	default:
		// missing + no content → create empty
		content = nil
	}

	if content != nil || live.Type == typeMissing {
		if err := writeFileAtomic(p.Path, content, mode); err != nil {
			return err
		}
	} else if p.Mode != modeUnset && live.Mode != p.Mode {
		if err := os.Chmod(p.Path, mode); err != nil {
			return fmt.Errorf("chmod %s: %w", p.Path, err)
		}
	}

	return applyOwnership(p, live)
}

func applyDirectory(p *params, live *meta) error {
	if live.Type != typeMissing && live.Type != typeDirectory {
		return fmt.Errorf("cannot create directory at %s: exists as %s", p.Path, live.Type)
	}
	mode := os.FileMode(0o755)
	if p.Mode != modeUnset {
		mode = os.FileMode(p.Mode)
	}
	if live.Type == typeMissing {
		if err := os.MkdirAll(p.Path, mode); err != nil {
			return fmt.Errorf("mkdir %s: %w", p.Path, err)
		}
		// MkdirAll honours umask, so chmod explicitly to the
		// declared mode.
		if err := os.Chmod(p.Path, mode); err != nil {
			return fmt.Errorf("chmod %s: %w", p.Path, err)
		}
	} else if p.Mode != modeUnset && live.Mode != p.Mode {
		if err := os.Chmod(p.Path, mode); err != nil {
			return fmt.Errorf("chmod %s: %w", p.Path, err)
		}
	}
	return applyOwnership(p, live)
}

func applySymlink(p *params, live *meta) error {
	switch {
	case live.Type == typeMissing:
		// straight create
	case live.Type == typeSymlink && live.SymlinkTarget == p.Target:
		return nil // already matches
	case live.Type == typeSymlink:
		// wrong target — replace
		if err := os.Remove(p.Path); err != nil {
			return fmt.Errorf("remove old symlink %s: %w", p.Path, err)
		}
	default:
		return fmt.Errorf("cannot create symlink at %s: exists as %s", p.Path, live.Type)
	}
	if err := os.Symlink(p.Target, p.Path); err != nil {
		return fmt.Errorf("symlink %s → %s: %w", p.Path, p.Target, err)
	}
	return applyOwnership(p, live)
}

// applyOwnership chowns the path to the declared owner/group. When
// either is empty the existing value is preserved. Chown of
// symlinks uses Lchown so we change the link's metadata, not the
// target's. A user.Lookup failure surfaces with the offending name;
// running as non-root with a real owner change returns the
// underlying "operation not permitted" error.
func applyOwnership(p *params, _ *meta) error {
	if p.Owner == "" && p.Group == "" {
		return nil
	}
	uid := -1
	gid := -1
	if p.Owner != "" {
		u, err := resolveOwner(p.Owner)
		if err != nil {
			return err
		}
		uid = u
	}
	if p.Group != "" {
		g, err := resolveGroup(p.Group)
		if err != nil {
			return err
		}
		gid = g
	}
	// Lchown is symlink-safe: it operates on the link, not the
	// target. For regular files / directories it behaves the same
	// as Chown.
	if err := os.Lchown(p.Path, uid, gid); err != nil {
		if errors.Is(err, os.ErrPermission) {
			return fmt.Errorf("chown %s: %w (chown typically requires root)", p.Path, err)
		}
		return fmt.Errorf("chown %s: %w", p.Path, err)
	}
	return nil
}
