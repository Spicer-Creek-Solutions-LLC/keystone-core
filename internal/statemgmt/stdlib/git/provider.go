// SPDX-License-Identifier: Apache-2.0

package git

import (
	"context"
	"errors"
)

// ErrGitNotFound is returned when the `git` binary is not on the
// agent's PATH. Operators should install git (typically via a
// `require: [package: git]` relationship in the state file).
var ErrGitNotFound = errors.New("git: the git binary was not found on PATH (install git first)")

// ErrNotARepo is returned when the declaration's directory exists
// but is not a git working tree, in a context where it must be one
// (e.g. `state: absent` refuses to remove it).
var ErrNotARepo = errors.New("git: directory exists but is not a git repository")

// IsGitNotFound / IsNotARepo expose the sentinel matchers so the
// gRPC server + CLI can render friendlier operator-facing messages.
func IsGitNotFound(err error) bool { return errors.Is(err, ErrGitNotFound) }
func IsNotARepo(err error) bool    { return errors.Is(err, ErrNotARepo) }

// RepoState is the inspected state of a working tree. Exists==false
// → the directory is not a git repo (RemoteURL / HeadSHA are then
// meaningless and left zero).
type RepoState struct {
	Exists    bool
	RemoteURL string // configured URL of the queried remote ("" if the remote isn't configured)
	HeadSHA   string // current checked-out commit
}

// CloneOptions parameterises a fresh clone.
type CloneOptions struct {
	URL    string
	Dir    string
	Rev    string // branch or tag name; "HEAD" means clone the default branch
	Depth  int    // 0 = full clone
	Remote string
}

// SyncOptions parameterises bringing an existing working tree up to
// date with the remote.
type SyncOptions struct {
	Dir    string
	Rev    string // branch / tag / sha; "HEAD" means the remote default branch
	Depth  int
	Remote string
	Force  bool // hard-reset (discard local changes) vs fast-forward only
}

// Provider abstracts the git operations the module needs. The
// production implementation shells out to the git binary; tests
// inject a fake so Check / Apply paths run without a real repo.
type Provider interface {
	// Inspect reports the state of the working tree at dir, querying
	// the named remote's URL. A missing or non-repo dir is
	// (RepoState{Exists:false}, nil) — not an error.
	Inspect(ctx context.Context, dir, remote string) (*RepoState, error)

	// RemoteSHA resolves rev on the remote to a commit SHA. dir is
	// the working tree to run from (so its configured remote URL is
	// used); rev "HEAD" resolves the remote's default branch.
	RemoteSHA(ctx context.Context, dir, remote, rev string) (string, error)

	// Clone creates a fresh working tree per opts. dir must not
	// already exist (or must be empty).
	Clone(ctx context.Context, opts CloneOptions) error

	// Sync fetches and checks out rev in an existing working tree.
	Sync(ctx context.Context, opts SyncOptions) error

	// Remove deletes the working tree at dir (recursively). It is in
	// the Provider only so the module's Apply path stays mockable.
	Remove(dir string) error
}

// commandRunner is the injection point that lets the git CLI
// backend's tests pin arg formation without invoking the real git
// binary. It returns combined stdout for commands whose output is
// parsed; mutating commands ignore the string.
type commandRunner func(ctx context.Context, bin string, args ...string) (string, error)
