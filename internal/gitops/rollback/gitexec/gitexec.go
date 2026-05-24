// SPDX-License-Identifier: Apache-2.0

// Package gitexec is the go-git v5 implementation of the rollback
// [rollback.GitClient] seam. It is the only place the go-git
// dependency is imported, keeping internal/gitops/rollback
// dependency-light and fake-testable.
package gitexec

import (
	"context"
	"fmt"
	"os"
	"sort"
	"time"

	gogit "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/config"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"
	githttp "github.com/go-git/go-git/v5/plumbing/transport/http"

	"go.keystone-core.io/keystone-core/internal/gitops/rollback"
)

// Client implements [rollback.GitClient] with go-git v5. The
// signature identifies the revert commits.
type Client struct {
	// AuthorName / AuthorEmail stamp the revert commit. Defaults
	// "keystone-core" / "keystone-core@localhost" when empty.
	AuthorName  string
	AuthorEmail string
}

var _ rollback.GitClient = (*Client)(nil)

func auth(token string) *githttp.BasicAuth {
	if token == "" {
		return nil
	}
	// go-git HTTP token auth: any non-empty username + token as password.
	return &githttp.BasicAuth{Username: "keystone", Password: token}
}

// cloneBranch clones repoURL's branch into a fresh temp dir and
// returns the repo plus a cleanup func.
func cloneBranch(ctx context.Context, repoURL, branch, token string) (*gogit.Repository, func(), error) {
	dir, err := os.MkdirTemp("", "kscore-gitexec-")
	if err != nil {
		return nil, nil, fmt.Errorf("tempdir: %w", err)
	}
	cleanup := func() { _ = os.RemoveAll(dir) }
	repo, err := gogit.PlainCloneContext(ctx, dir, false, &gogit.CloneOptions{
		URL:           repoURL,
		Auth:          auth(token),
		ReferenceName: plumbing.NewBranchReferenceName(branch),
		SingleBranch:  true,
	})
	if err != nil {
		cleanup()
		return nil, nil, fmt.Errorf("clone %s#%s: %w", repoURL, branch, err)
	}
	return repo, cleanup, nil
}

// Revert restores branch to req.ToRevision by writing a new commit
// whose tree is that revision's tree (parent = current head, so
// history is preserved — no force push) and pushing it.
func (c *Client) Revert(ctx context.Context, req rollback.GitRevertRequest) (rollback.GitRevertResult, error) {
	repo, cleanup, err := cloneBranch(ctx, req.RepoURL, req.Branch, req.AuthToken)
	if err != nil {
		return rollback.GitRevertResult{}, err
	}
	defer cleanup()

	headRef, err := repo.Head()
	if err != nil {
		return rollback.GitRevertResult{}, fmt.Errorf("head: %w", err)
	}
	fromHash := headRef.Hash()

	targetHash, err := repo.ResolveRevision(plumbing.Revision(req.ToRevision))
	if err != nil {
		return rollback.GitRevertResult{}, fmt.Errorf("resolve %q: %w", req.ToRevision, err)
	}
	targetCommit, err := repo.CommitObject(*targetHash)
	if err != nil {
		return rollback.GitRevertResult{}, fmt.Errorf("commit %s: %w", targetHash, err)
	}

	now := time.Now()
	sig := object.Signature{Name: c.authorName(), Email: c.authorEmail(), When: now}
	revert := &object.Commit{
		Author:       sig,
		Committer:    sig,
		Message:      req.Message + "\n",
		TreeHash:     targetCommit.TreeHash,
		ParentHashes: []plumbing.Hash{fromHash},
	}
	obj := repo.Storer.NewEncodedObject()
	if err := revert.Encode(obj); err != nil {
		return rollback.GitRevertResult{}, fmt.Errorf("encode commit: %w", err)
	}
	newHash, err := repo.Storer.SetEncodedObject(obj)
	if err != nil {
		return rollback.GitRevertResult{}, fmt.Errorf("store commit: %w", err)
	}
	branchRef := plumbing.NewBranchReferenceName(req.Branch)
	if err := repo.Storer.SetReference(plumbing.NewHashReference(branchRef, newHash)); err != nil {
		return rollback.GitRevertResult{}, fmt.Errorf("update ref: %w", err)
	}

	if err := repo.PushContext(ctx, &gogit.PushOptions{
		Auth:     auth(req.AuthToken),
		RefSpecs: []config.RefSpec{config.RefSpec(fmt.Sprintf("%s:%s", branchRef, branchRef))},
	}); err != nil {
		return rollback.GitRevertResult{}, fmt.Errorf("push: %w", err)
	}
	return rollback.GitRevertResult{
		FromRevision: fromHash.String(),
		ToRevision:   targetHash.String(),
		NewCommit:    newHash.String(),
	}, nil
}

// PreviousRevision returns the first parent of branch HEAD.
func (c *Client) PreviousRevision(ctx context.Context, repoURL, branch, token string) (string, error) {
	repo, cleanup, err := cloneBranch(ctx, repoURL, branch, token)
	if err != nil {
		return "", err
	}
	defer cleanup()

	headRef, err := repo.Head()
	if err != nil {
		return "", fmt.Errorf("head: %w", err)
	}
	head, err := repo.CommitObject(headRef.Hash())
	if err != nil {
		return "", fmt.Errorf("head commit: %w", err)
	}
	if head.NumParents() == 0 {
		return "", fmt.Errorf("branch %q has no previous commit", branch)
	}
	parent, err := head.Parent(0)
	if err != nil {
		return "", fmt.Errorf("parent: %w", err)
	}
	return parent.Hash.String(), nil
}

// LastKnownGood returns the newest tag's commit (best-effort v1.0
// signal); falls back to the previous commit when the repo has no
// tags.
func (c *Client) LastKnownGood(ctx context.Context, repoURL, branch, token string) (string, error) {
	repo, cleanup, err := cloneBranch(ctx, repoURL, branch, token)
	if err != nil {
		return "", err
	}
	defer cleanup()

	iter, err := repo.Tags()
	if err != nil {
		return "", fmt.Errorf("tags: %w", err)
	}
	type tagCommit struct {
		hash string
		when time.Time
	}
	var tags []tagCommit
	_ = iter.ForEach(func(ref *plumbing.Reference) error {
		// Resolve annotated or lightweight tag to its commit.
		if to, terr := repo.TagObject(ref.Hash()); terr == nil {
			if cm, cerr := to.Commit(); cerr == nil {
				tags = append(tags, tagCommit{cm.Hash.String(), cm.Committer.When})
				return nil
			}
		}
		if cm, cerr := repo.CommitObject(ref.Hash()); cerr == nil {
			tags = append(tags, tagCommit{cm.Hash.String(), cm.Committer.When})
		}
		return nil
	})
	if len(tags) > 0 {
		sort.Slice(tags, func(i, j int) bool { return tags[i].when.After(tags[j].when) })
		return tags[0].hash, nil
	}
	// No tags: fall back to the previous commit.
	return c.PreviousRevision(ctx, repoURL, branch, token)
}

func (c *Client) authorName() string {
	if c.AuthorName != "" {
		return c.AuthorName
	}
	return "keystone-core"
}

func (c *Client) authorEmail() string {
	if c.AuthorEmail != "" {
		return c.AuthorEmail
	}
	return "keystone-core@localhost"
}
