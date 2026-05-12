// Package ssh implements the `ssh` stdlib state module — managing an
// entry in a user's ~/.ssh/authorized_keys (the classic
// authorized_key.present / .absent), per PROJECT-DETAILS §4.8 (SSH &
// security category).
//
// An authorized_keys line is "[options ]<keytype> <blob>[ comment]".
// The entry's identity is the key material — <keytype> <blob> — so a
// `present` declaration is matched against the existing line with
// that material (its options / comment are then rewritten to match
// the declaration exactly).
//
// Declaration.Name is just a human label (the decl ID); the key
// itself comes from the `key` param and the target user from `user`.
//
// State semantics:
//
//	present — <user>'s authorized_keys contains the line
//	          "[options ]<keytype> <blob>[ comment]" (comment = the
//	          `comment` param if set, else the comment carried in
//	          `key`, else none). Apply ensures ~/.ssh (0700) and
//	          authorized_keys (0600) exist, owned by <user>, then
//	          upserts the line. An existing line with the same key
//	          material but different options/comment is rewritten in
//	          place; otherwise the line is appended.
//	absent  — no line with that key material is present (all
//	          matching lines are removed). The file/dir are left as
//	          they are (only the line is removed); a missing file is
//	          already converged.
//
// Managing another user's keys needs root (or running the agent as
// that user) — the chown step surfaces an EPERM with a hint.
//
// v1.0 out of scope (V1X candidates):
//   - Validating the key blob as a well-formed SSH public key (via
//     golang.org/x/crypto/ssh) — v1.0 only charset-checks it, to
//     avoid a new direct dependency.
//   - Quote-aware comma-splitting / set-comparison of the options
//     field (v1.0 compares it verbatim, after collapsing whitespace).
//   - An authorized_keys2 / custom-path override; managing the whole
//     file (replace all keys); AuthorizedKeysCommand / cert auth.
package ssh

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/user"
	"path/filepath"
	"strconv"
	"time"

	"go.keystone-core.io/keystone-core/internal/statemgmt"
)

const (
	sshDirMode   = 0o700
	authFileMode = 0o600
)

// homeDirFor resolves the home directory and uid/gid of a user. It is
// a package var so tests can point it at a tempdir and the current
// uid/gid (which makes the chown step a no-op).
var homeDirFor = func(name string) (homeDir string, uid, gid int, err error) {
	u, err := user.Lookup(name)
	if err != nil {
		return "", 0, 0, fmt.Errorf("look up user %q: %w", name, err)
	}
	if u.HomeDir == "" {
		return "", 0, 0, fmt.Errorf("user %q has no home directory", name)
	}
	uid, err = strconv.Atoi(u.Uid)
	if err != nil {
		return "", 0, 0, fmt.Errorf("user %q uid: %w", name, err)
	}
	gid, err = strconv.Atoi(u.Gid)
	if err != nil {
		return "", 0, 0, fmt.Errorf("user %q gid: %w", name, err)
	}
	return u.HomeDir, uid, gid, nil
}

func authorizedKeysPath(homeDir string) string {
	return filepath.Join(homeDir, ".ssh", "authorized_keys")
}

// New is the Factory registered with the engine Registry.
func New() statemgmt.Module { return &Module{} }

// Module is the ssh state module. It is stateless; concurrent
// Check/Apply/Test calls on different Declarations are safe.
type Module struct{}

func (m *Module) Name() string { return "ssh" }

func (m *Module) ValidStates() []string { return []string{StatePresent, StateAbsent} }

func (m *Module) Validate(decl *statemgmt.Declaration) error {
	p, err := parseParams(decl)
	if err != nil {
		return err
	}
	return p.validate()
}

// DriftSeverity: an SSH authorized key is an access boundary —
// missing one that should be there (a deploy bot locked out) or a
// stray one that should be gone is HIGH. nil → MEDIUM. Operators
// override via `severity:`.
func (m *Module) DriftSeverity(decl *statemgmt.Declaration, _ *statemgmt.ModuleCheckResult) statemgmt.DriftSeverity {
	if decl == nil {
		return statemgmt.DriftSeverityMedium
	}
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
	home, _, _, err := homeDirFor(p.User)
	if err != nil {
		return nil, err
	}
	content, _, err := readAuthKeys(authorizedKeysPath(home))
	if err != nil {
		return nil, err
	}
	want := p.desiredKey()
	got, found := findLine(content, want.KeyType, want.Blob)
	switch p.State {
	case StatePresent:
		if !found {
			return &statemgmt.ModuleCheckResult{Matches: false, Diff: fmt.Sprintf("key not present in %s's authorized_keys → add", p.User)}, nil
		}
		if got.render() != want.render() {
			return &statemgmt.ModuleCheckResult{Matches: false, Diff: fmt.Sprintf("authorized_keys line differs: %q → %q", got.render(), want.render())}, nil
		}
		return &statemgmt.ModuleCheckResult{Matches: true}, nil
	case StateAbsent:
		if !found {
			return &statemgmt.ModuleCheckResult{Matches: true}, nil
		}
		return &statemgmt.ModuleCheckResult{Matches: false, Diff: fmt.Sprintf("key present in %s's authorized_keys; want absent", p.User)}, nil
	}
	return nil, fmt.Errorf("unknown state %q", p.State)
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
	home, uid, gid, err := homeDirFor(p.User)
	if err != nil {
		return failure(start), err
	}
	path := authorizedKeysPath(home)
	content, existed, err := readAuthKeys(path)
	if err != nil {
		return failure(start), err
	}
	want := p.desiredKey()
	got, found := findLine(content, want.KeyType, want.Blob)

	switch p.State {
	case StatePresent:
		if found && got.render() == want.render() {
			return ok(start, false, "", "already converged"), nil
		}
		if err := ensureSSHDir(home, uid, gid); err != nil {
			return failure(start), err
		}
		newContent := upsertLine(content, want)
		if err := writeAuthKeys(path, newContent, existed, uid, gid); err != nil {
			return failure(start), err
		}
		if found {
			return ok(start, true, fmt.Sprintf("updated authorized_keys line for %s", p.User), "applied"), nil
		}
		return ok(start, true, fmt.Sprintf("added authorized_keys line for %s", p.User), "applied"), nil

	case StateAbsent:
		if !found {
			return ok(start, false, "", "already converged"), nil
		}
		newContent := removeLines(content, want.KeyType, want.Blob)
		if err := writeAuthKeys(path, newContent, existed, uid, gid); err != nil {
			return failure(start), err
		}
		return ok(start, true, fmt.Sprintf("removed authorized_keys line for %s", p.User), "applied"), nil
	}
	return nil, fmt.Errorf("unknown state %q", p.State)
}

func (m *Module) Test(ctx context.Context, decl *statemgmt.Declaration) (bool, error) {
	res, err := m.Check(ctx, decl)
	if err != nil {
		return false, err
	}
	return res.Matches, nil
}

func ok(start time.Time, changed bool, diff, comment string) *statemgmt.StateResult {
	return &statemgmt.StateResult{Success: true, Changed: changed, Diff: diff, Comment: comment, Duration: time.Since(start)}
}
func failure(start time.Time) *statemgmt.StateResult {
	return &statemgmt.StateResult{Success: false, Changed: false, Duration: time.Since(start)}
}

// readAuthKeys reads the authorized_keys file; a missing file is
// ("", false, nil).
func readAuthKeys(path string) (content string, existed bool, err error) {
	data, err := os.ReadFile(path) //nolint:gosec // path is <home>/.ssh/authorized_keys for a validated user from a state declaration
	if errors.Is(err, fs.ErrNotExist) {
		return "", false, nil
	}
	if err != nil {
		return "", false, fmt.Errorf("read %s: %w", path, err)
	}
	return string(data), true, nil
}

// ensureSSHDir makes sure <home>/.ssh exists with mode 0700 and is
// owned by uid/gid.
func ensureSSHDir(home string, uid, gid int) error {
	dir := filepath.Join(home, ".ssh")
	if err := os.MkdirAll(dir, sshDirMode); err != nil { //nolint:gosec // ~/.ssh must be 0700 and is the documented mode
		return fmt.Errorf("mkdir %s: %w", dir, err)
	}
	if err := os.Chmod(dir, sshDirMode); err != nil {
		return fmt.Errorf("chmod %s: %w", dir, err)
	}
	return chownIfNeeded(dir, uid, gid)
}

// writeAuthKeys atomically writes the authorized_keys file with mode
// 0600 (preserving the existing mode if it existed) and ownership
// uid/gid.
func writeAuthKeys(path, content string, existed bool, uid, gid int) error {
	mode := os.FileMode(authFileMode)
	if existed {
		if fi, err := os.Stat(path); err == nil {
			mode = fi.Mode().Perm()
		}
	}
	tmp := path + ".keystone.tmp"
	if err := os.WriteFile(tmp, []byte(content), mode); err != nil { //nolint:gosec // authorized_keys is 0600; mode mirrors the existing file or is 0600
		return fmt.Errorf("write tmp: %w", err)
	}
	if err := os.Chmod(tmp, mode); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("chmod tmp: %w", err)
	}
	if err := chownIfNeeded(tmp, uid, gid); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("rename %s → %s: %w", tmp, path, err)
	}
	return nil
}

// chownIfNeeded chowns path to uid/gid unless that is already the
// running process's own uid/gid (in which case it is a no-op — and
// the common case when the agent runs as the target user).
func chownIfNeeded(path string, uid, gid int) error {
	if uid == os.Getuid() && gid == os.Getgid() {
		return nil
	}
	if err := os.Chown(path, uid, gid); err != nil {
		if errors.Is(err, os.ErrPermission) {
			return fmt.Errorf("chown %s to %d:%d: %w (managing another user's keys needs root)", path, uid, gid, err)
		}
		return fmt.Errorf("chown %s: %w", path, err)
	}
	return nil
}
