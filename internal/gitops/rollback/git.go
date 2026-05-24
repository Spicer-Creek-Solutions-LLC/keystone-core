// SPDX-License-Identifier: Apache-2.0

package rollback

import (
	"context"
	"time"
)

// GitRevertRequest is the input to a [GitClient.Revert]: restore the
// repository's branch tree to ToRevision by committing a new revert
// commit (no force-push, history preserved) and pushing.
type GitRevertRequest struct {
	RepoURL    string
	Branch     string
	ToRevision string
	Message    string
	// AuthToken is an optional bearer/PAT for HTTPS push. Empty = the
	// remote is reachable unauthenticated (e.g. a test fixture).
	AuthToken string
}

// GitRevertResult reports the commit the branch moved from/to.
type GitRevertResult struct {
	FromRevision string
	ToRevision   string
	NewCommit    string
}

// GitClient is the seam the Git executor needs. The concrete
// implementation (gitexec, go-git v5) lives in the gitexec
// subpackage so this package stays dependency-light.
type GitClient interface {
	// Revert restores branch to req.ToRevision via a new commit and
	// pushes it.
	Revert(ctx context.Context, req GitRevertRequest) (GitRevertResult, error)
	// PreviousRevision returns the commit before HEAD on branch.
	PreviousRevision(ctx context.Context, repoURL, branch, authToken string) (string, error)
	// LastKnownGood returns a best-effort last-good revision (v1.0:
	// the newest tag, else the previous commit).
	LastKnownGood(ctx context.Context, repoURL, branch, authToken string) (string, error)
}

// GitRevertExecutor rolls back by committing a revert-to-revision on
// a branch and pushing it, so the GitOps reconciler redeploys the
// prior state. Config: repo_url (required), branch (default "main"),
// auth_token (optional).
type GitRevertExecutor struct {
	Client GitClient
}

// Type implements [Executor].
func (GitRevertExecutor) Type() string { return "git" }

func gitCfg(cfg Config) (repoURL, branch, token string, err error) {
	repoURL, err = cfgString(cfg, "repo_url")
	if err != nil {
		return "", "", "", err
	}
	return repoURL, cfgStringOpt(cfg, "branch", "main"), cfgStringOpt(cfg, "auth_token", ""), nil
}

// Execute implements [Executor].
func (e GitRevertExecutor) Execute(ctx context.Context, cfg Config, req Request) Result {
	start := time.Now()
	if e.Client == nil {
		return failf(start, ErrNotConfigured, "git: no client configured")
	}
	repoURL, branch, token, err := gitCfg(cfg)
	if err != nil {
		return failf(start, err, "git: %v", err)
	}
	target, err := resolveTarget(ctx, e, cfg, req)
	if err != nil {
		return failf(start, err, "git: resolve target: %v", err)
	}

	msg := req.Reason
	if msg == "" {
		msg = "rollback to " + target
	}
	res, err := e.Client.Revert(ctx, GitRevertRequest{
		RepoURL:    repoURL,
		Branch:     branch,
		ToRevision: target,
		Message:    "Revert: " + msg,
		AuthToken:  token,
	})
	if err != nil {
		return failf(start, err, "git: revert failed: %v", err)
	}
	return Result{
		Success:      true,
		Message:      "git: reverted " + branch + " to " + res.ToRevision,
		FromRevision: res.FromRevision,
		ToRevision:   res.ToRevision,
		Data:         map[string]any{"new_commit": res.NewCommit, "branch": branch},
		Duration:     time.Since(start),
	}
}

// GetPreviousRevision implements [Executor].
func (e GitRevertExecutor) GetPreviousRevision(ctx context.Context, cfg Config, _ Request) (string, error) {
	if e.Client == nil {
		return "", ErrNotConfigured
	}
	repoURL, branch, token, err := gitCfg(cfg)
	if err != nil {
		return "", err
	}
	return e.Client.PreviousRevision(ctx, repoURL, branch, token)
}

// GetLastKnownGood implements [Executor].
func (e GitRevertExecutor) GetLastKnownGood(ctx context.Context, cfg Config, _ Request) (string, error) {
	if e.Client == nil {
		return "", ErrNotConfigured
	}
	repoURL, branch, token, err := gitCfg(cfg)
	if err != nil {
		return "", err
	}
	return e.Client.LastKnownGood(ctx, repoURL, branch, token)
}
