package git

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// defaultProvider resolves the git binary at construction time. When
// git is not on PATH it returns a notInstalledProvider whose mutating
// operations fail with ErrGitNotFound; Inspect still works via a
// plain filesystem check so `state: absent` (which only needs
// os.RemoveAll) behaves correctly.
func defaultProvider() Provider {
	if bin, err := exec.LookPath("git"); err == nil {
		return &cliProvider{git: bin, run: execRun}
	}
	return &notInstalledProvider{}
}

// dirNonEmpty reports whether dir exists and contains at least one
// entry. A missing dir is "empty" (false) — clone will create it.
func dirNonEmpty(dir string) bool {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return false
	}
	return len(entries) > 0
}

// repoDirExists reports whether dir looks like a git working-tree
// root (it contains a `.git` entry — a directory for ordinary
// clones, a file for worktrees / submodules). Nested-repo and
// detached-worktree edge cases are out of scope for v1.0.
func repoDirExists(dir string) bool {
	_, err := os.Stat(filepath.Join(dir, ".git"))
	return err == nil
}

// --- cliProvider: shells out to the git binary --------------------

type cliProvider struct {
	git string
	run commandRunner
}

func (p *cliProvider) Inspect(ctx context.Context, dir, remote string) (*RepoState, error) {
	if !repoDirExists(dir) {
		return &RepoState{Exists: false}, nil
	}
	st := &RepoState{Exists: true}
	// remote URL — a missing remote.<name>.url is not an error,
	// just "no configured URL".
	if out, err := p.run(ctx, p.git, "-C", dir, "config", "--get", "remote."+remote+".url"); err == nil {
		st.RemoteURL = strings.TrimSpace(out)
	}
	// HEAD — empty in a freshly-init'd repo with no commits; not an
	// error for our purposes.
	if out, err := p.run(ctx, p.git, "-C", dir, "rev-parse", "HEAD"); err == nil {
		st.HeadSHA = strings.TrimSpace(out)
	}
	return st, nil
}

func (p *cliProvider) RemoteSHA(ctx context.Context, dir, remote, rev string) (string, error) {
	out, err := p.run(ctx, p.git, "-C", dir, "ls-remote", remote, rev)
	if err != nil {
		return "", fmt.Errorf("git ls-remote %s %s: %w", remote, rev, err)
	}
	sha := parseLsRemote(out, rev)
	if sha == "" {
		return "", fmt.Errorf("could not resolve %q on remote %q", rev, remote)
	}
	return sha, nil
}

func (p *cliProvider) Clone(ctx context.Context, opts CloneOptions) error {
	args := []string{"clone"}
	if opts.Depth > 0 {
		args = append(args, "--depth", fmt.Sprintf("%d", opts.Depth))
	}
	if opts.Remote != "" && opts.Remote != defaultRemote {
		args = append(args, "--origin", opts.Remote)
	}
	// --branch accepts a branch or tag name but not an arbitrary
	// SHA; for a SHA rev we clone the default branch and check out
	// the commit afterwards (best-effort with shallow clones).
	checkoutSHA := ""
	switch {
	case opts.Rev == "" || opts.Rev == revHEAD:
		// clone the default branch
	case isFullSHA(opts.Rev):
		checkoutSHA = opts.Rev
	default:
		args = append(args, "--branch", opts.Rev)
	}
	args = append(args, "--", opts.URL, opts.Dir)
	if _, err := p.run(ctx, p.git, args...); err != nil {
		return fmt.Errorf("git clone: %w", err)
	}
	if checkoutSHA != "" {
		if _, err := p.run(ctx, p.git, "-C", opts.Dir, "checkout", checkoutSHA); err != nil {
			return fmt.Errorf("git checkout %s: %w", checkoutSHA, err)
		}
	}
	return nil
}

func (p *cliProvider) Sync(ctx context.Context, opts SyncOptions) error {
	fetchArgs := []string{"-C", opts.Dir, "fetch"}
	if opts.Depth > 0 {
		fetchArgs = append(fetchArgs, "--depth", fmt.Sprintf("%d", opts.Depth))
	}
	fetchArgs = append(fetchArgs, "--", opts.Remote, opts.Rev)
	if _, err := p.run(ctx, p.git, fetchArgs...); err != nil {
		return fmt.Errorf("git fetch %s %s: %w", opts.Remote, opts.Rev, err)
	}
	if opts.Force {
		if _, err := p.run(ctx, p.git, "-C", opts.Dir, "reset", "--hard", "FETCH_HEAD"); err != nil {
			return fmt.Errorf("git reset --hard FETCH_HEAD: %w", err)
		}
		return nil
	}
	if _, err := p.run(ctx, p.git, "-C", opts.Dir, "merge", "--ff-only", "FETCH_HEAD"); err != nil {
		return fmt.Errorf("git merge --ff-only FETCH_HEAD (local changes present? set force: true): %w", err)
	}
	return nil
}

func (p *cliProvider) Remove(dir string) error {
	if err := os.RemoveAll(dir); err != nil {
		return fmt.Errorf("remove %s: %w", dir, err)
	}
	return nil
}

// parseLsRemote extracts the commit SHA for rev from `git ls-remote`
// output. Lines are "<sha>\t<refname>". For an annotated tag the
// dereferenced "<refname>^{}" line carries the commit (not the tag
// object) — prefer it. For rev "HEAD" the line whose refname is
// exactly "HEAD" is authoritative. Otherwise the first line wins.
func parseLsRemote(out, rev string) string {
	var first, headLine, derefLine string
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		sha, ref, ok := strings.Cut(line, "\t")
		if !ok {
			continue
		}
		sha = strings.TrimSpace(sha)
		if first == "" {
			first = sha
		}
		if ref == "HEAD" {
			headLine = sha
		}
		if strings.HasSuffix(ref, "^{}") {
			derefLine = sha
		}
	}
	switch {
	case rev == revHEAD && headLine != "":
		return headLine
	case derefLine != "":
		return derefLine
	default:
		return first
	}
}

// execRun is the production commandRunner. It captures stdout (so
// parsed output isn't polluted by progress messages on stderr) and,
// on a non-zero exit, surfaces the exit code plus trimmed stderr.
func execRun(ctx context.Context, bin string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, bin, args...) //nolint:gosec // bin resolved via exec.LookPath at construction; args are fixed verbs + operator-supplied url/rev/path from a validated state declaration
	out, err := cmd.Output()
	if err == nil {
		return string(out), nil
	}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		return "", fmt.Errorf("%s %s: exit %d: %s", bin, strings.Join(args, " "), exitErr.ExitCode(), strings.TrimSpace(string(exitErr.Stderr)))
	}
	return "", fmt.Errorf("%s %s: %w", bin, strings.Join(args, " "), err)
}

// --- notInstalledProvider: git binary absent ----------------------

type notInstalledProvider struct{}

func (*notInstalledProvider) Inspect(_ context.Context, dir, _ string) (*RepoState, error) {
	// Without git we can't read the remote URL or HEAD, but a
	// filesystem check still tells `state: absent` whether there's
	// something to remove.
	return &RepoState{Exists: repoDirExists(dir)}, nil
}
func (*notInstalledProvider) RemoteSHA(context.Context, string, string, string) (string, error) {
	return "", ErrGitNotFound
}
func (*notInstalledProvider) Clone(context.Context, CloneOptions) error { return ErrGitNotFound }
func (*notInstalledProvider) Sync(context.Context, SyncOptions) error   { return ErrGitNotFound }
func (*notInstalledProvider) Remove(dir string) error {
	if err := os.RemoveAll(dir); err != nil {
		return fmt.Errorf("remove %s: %w", dir, err)
	}
	return nil
}
