package gitexec

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	gogit "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/config"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"

	"go.keystone-core.io/keystone-core/internal/gitops/rollback"
)

func sig() *object.Signature {
	return &object.Signature{Name: "t", Email: "t@x", When: time.Now()}
}

func commit(t *testing.T, wt *gogit.Worktree, dir, file, content, msg string) plumbing.Hash {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, file), []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", file, err)
	}
	if _, err := wt.Add(file); err != nil {
		t.Fatalf("add %s: %v", file, err)
	}
	h, err := wt.Commit(msg, &gogit.CommitOptions{Author: sig()})
	if err != nil {
		t.Fatalf("commit %s: %v", msg, err)
	}
	return h
}

func push(t *testing.T, repo *gogit.Repository, refspec string) {
	t.Helper()
	err := repo.Push(&gogit.PushOptions{
		RemoteName: "origin",
		RefSpecs:   []config.RefSpec{config.RefSpec(refspec)},
	})
	if err != nil && err != gogit.NoErrAlreadyUpToDate {
		t.Fatalf("push %s: %v", refspec, err)
	}
}

// seedRemote builds a bare repo with two commits (c1 adds a.txt=v1;
// c2 adds b.txt) on the default branch plus a tag v1 on c1, and
// returns the remote path, branch, and the two commit hashes.
func seedRemote(t *testing.T) (remote, branch string, c1, c2 plumbing.Hash) {
	t.Helper()
	remote = filepath.Join(t.TempDir(), "origin.git")
	if _, err := gogit.PlainInit(remote, true); err != nil {
		t.Fatalf("init bare: %v", err)
	}
	work := t.TempDir()
	repo, err := gogit.PlainInit(work, false)
	if err != nil {
		t.Fatalf("init work: %v", err)
	}
	if _, err := repo.CreateRemote(&config.RemoteConfig{Name: "origin", URLs: []string{remote}}); err != nil {
		t.Fatalf("create remote: %v", err)
	}
	wt, err := repo.Worktree()
	if err != nil {
		t.Fatalf("worktree: %v", err)
	}
	c1 = commit(t, wt, work, "a.txt", "v1", "c1")
	c2 = commit(t, wt, work, "b.txt", "added b", "c2")

	head, err := repo.Head()
	if err != nil {
		t.Fatalf("head: %v", err)
	}
	branch = head.Name().Short()
	if _, err := repo.CreateTag("v1", c1, nil); err != nil {
		t.Fatalf("tag: %v", err)
	}
	push(t, repo, "refs/heads/"+branch+":refs/heads/"+branch)
	push(t, repo, "refs/tags/v1:refs/tags/v1")
	return remote, branch, c1, c2
}

func TestClient_Revert_CommitsAndPushes(t *testing.T) {
	t.Parallel()
	remote, branch, c1, c2 := seedRemote(t)

	res, err := (&Client{}).Revert(context.Background(), rollback.GitRevertRequest{
		RepoURL:    remote,
		Branch:     branch,
		ToRevision: c1.String(),
		Message:    "Revert: rollback to c1",
	})
	if err != nil {
		t.Fatalf("Revert: %v", err)
	}
	if res.FromRevision != c2.String() || res.ToRevision != c1.String() {
		t.Errorf("result revisions = %s→%s, want %s→%s", res.FromRevision, res.ToRevision, c2, c1)
	}

	// Re-clone the remote and assert the pushed revert commit:
	// parent == c2 (history preserved, no force-push), tree == c1's
	// tree (state restored), b.txt gone, a.txt back to v1.
	verifyDir := t.TempDir()
	vr, err := gogit.PlainClone(verifyDir, false, &gogit.CloneOptions{
		URL:           remote,
		ReferenceName: plumbing.NewBranchReferenceName(branch),
		SingleBranch:  true,
	})
	if err != nil {
		t.Fatalf("verify clone: %v", err)
	}
	vh, err := vr.Head()
	if err != nil {
		t.Fatalf("verify head: %v", err)
	}
	if vh.Hash().String() != res.NewCommit {
		t.Errorf("remote head = %s, want pushed revert %s", vh.Hash(), res.NewCommit)
	}
	revertCommit, err := vr.CommitObject(vh.Hash())
	if err != nil {
		t.Fatalf("revert commit: %v", err)
	}
	if revertCommit.NumParents() != 1 || revertCommit.ParentHashes[0] != c2 {
		t.Errorf("revert parent = %v, want [%s] (history preserved)", revertCommit.ParentHashes, c2)
	}
	c1Commit, err := vr.CommitObject(c1)
	if err != nil {
		t.Fatalf("c1 commit: %v", err)
	}
	if revertCommit.TreeHash != c1Commit.TreeHash {
		t.Errorf("revert tree = %s, want c1 tree %s", revertCommit.TreeHash, c1Commit.TreeHash)
	}
	if b, err := os.ReadFile(filepath.Join(verifyDir, "a.txt")); err != nil || string(b) != "v1" {
		t.Errorf("a.txt = %q,%v want v1", b, err)
	}
	if _, err := os.Stat(filepath.Join(verifyDir, "b.txt")); !os.IsNotExist(err) {
		t.Errorf("b.txt should be gone after revert to c1, stat err = %v", err)
	}
}

func TestClient_PreviousRevision(t *testing.T) {
	t.Parallel()
	remote, branch, _, c2 := seedRemote(t)
	got, err := (&Client{}).PreviousRevision(context.Background(), remote, branch, "")
	if err != nil {
		t.Fatalf("PreviousRevision: %v", err)
	}
	// HEAD is c2; its parent is c1 — but we assert it's c2's parent,
	// i.e. the first commit.
	head, _ := gogit.PlainClone(t.TempDir(), false, &gogit.CloneOptions{URL: remote})
	hc, _ := head.Head()
	c, _ := head.CommitObject(hc.Hash())
	parent, _ := c.Parent(0)
	if got != parent.Hash.String() {
		t.Errorf("PreviousRevision = %s, want %s", got, parent.Hash)
	}
	_ = c2
}

func TestClient_LastKnownGood_PrefersTag(t *testing.T) {
	t.Parallel()
	remote, branch, c1, _ := seedRemote(t)
	got, err := (&Client{}).LastKnownGood(context.Background(), remote, branch, "")
	if err != nil {
		t.Fatalf("LastKnownGood: %v", err)
	}
	if got != c1.String() {
		t.Errorf("LastKnownGood = %s, want tagged c1 %s", got, c1)
	}
}

func TestClient_Revert_BadRevision(t *testing.T) {
	t.Parallel()
	remote, branch, _, _ := seedRemote(t)
	_, err := (&Client{}).Revert(context.Background(), rollback.GitRevertRequest{
		RepoURL: remote, Branch: branch, ToRevision: "deadbeefdeadbeefdeadbeefdeadbeefdeadbeef",
	})
	if err == nil {
		t.Error("Revert with unknown revision = nil error, want failure")
	}
}

func TestAuth(t *testing.T) {
	t.Parallel()
	if auth("") != nil {
		t.Error("auth(\"\") should be nil (unauthenticated remote)")
	}
	a := auth("tok")
	if a == nil || a.Password != "tok" || a.Username == "" {
		t.Errorf("auth(tok) = %+v, want non-nil basic-auth with password tok", a)
	}
}

func TestClient_CustomSignature(t *testing.T) {
	t.Parallel()
	c := &Client{AuthorName: "Ops", AuthorEmail: "ops@corp"}
	if c.authorName() != "Ops" || c.authorEmail() != "ops@corp" {
		t.Errorf("custom signature not used: %q / %q", c.authorName(), c.authorEmail())
	}
	d := &Client{}
	if d.authorName() != "keystone-core" || d.authorEmail() != "keystone-core@localhost" {
		t.Errorf("default signature wrong: %q / %q", d.authorName(), d.authorEmail())
	}

	// The custom signature lands on the pushed revert commit.
	remote, branch, c1, _ := seedRemote(t)
	res, err := c.Revert(context.Background(), rollback.GitRevertRequest{
		RepoURL: remote, Branch: branch, ToRevision: c1.String(), Message: "m",
	})
	if err != nil {
		t.Fatalf("Revert: %v", err)
	}
	vr, _ := gogit.PlainClone(t.TempDir(), false, &gogit.CloneOptions{
		URL: remote, ReferenceName: plumbing.NewBranchReferenceName(branch), SingleBranch: true,
	})
	cm, _ := vr.CommitObject(plumbing.NewHash(res.NewCommit))
	if cm.Author.Name != "Ops" || cm.Author.Email != "ops@corp" {
		t.Errorf("revert author = %s <%s>, want Ops <ops@corp>", cm.Author.Name, cm.Author.Email)
	}
}

func TestClient_CloneErrors(t *testing.T) {
	t.Parallel()
	c := &Client{}
	missing := filepath.Join(t.TempDir(), "does-not-exist.git")
	if _, err := c.Revert(context.Background(), rollback.GitRevertRequest{RepoURL: missing, Branch: "main", ToRevision: "x"}); err == nil {
		t.Error("Revert on missing remote = nil, want clone error")
	}
	if _, err := c.PreviousRevision(context.Background(), missing, "main", ""); err == nil {
		t.Error("PreviousRevision on missing remote = nil, want clone error")
	}
	if _, err := c.LastKnownGood(context.Background(), missing, "main", ""); err == nil {
		t.Error("LastKnownGood on missing remote = nil, want clone error")
	}
}

// seedRemoteNoTag is seedRemote without the v1 tag, for the
// LastKnownGood tag-fallback path.
func seedRemoteNoTag(t *testing.T) (remote, branch string, c1 plumbing.Hash) {
	t.Helper()
	remote = filepath.Join(t.TempDir(), "origin.git")
	if _, err := gogit.PlainInit(remote, true); err != nil {
		t.Fatalf("init bare: %v", err)
	}
	work := t.TempDir()
	repo, err := gogit.PlainInit(work, false)
	if err != nil {
		t.Fatalf("init work: %v", err)
	}
	if _, err := repo.CreateRemote(&config.RemoteConfig{Name: "origin", URLs: []string{remote}}); err != nil {
		t.Fatalf("remote: %v", err)
	}
	wt, _ := repo.Worktree()
	c1 = commit(t, wt, work, "a.txt", "v1", "c1")
	commit(t, wt, work, "b.txt", "b", "c2")
	head, _ := repo.Head()
	branch = head.Name().Short()
	push(t, repo, "refs/heads/"+branch+":refs/heads/"+branch)
	return remote, branch, c1
}

func TestClient_LastKnownGood_NoTagsFallsBackToPrevious(t *testing.T) {
	t.Parallel()
	remote, branch, c1 := seedRemoteNoTag(t)
	got, err := (&Client{}).LastKnownGood(context.Background(), remote, branch, "")
	if err != nil {
		t.Fatalf("LastKnownGood: %v", err)
	}
	if got != c1.String() {
		t.Errorf("LastKnownGood = %s, want previous-commit fallback %s", got, c1)
	}
}

func TestClient_PreviousRevision_NoParent(t *testing.T) {
	t.Parallel()
	remote := filepath.Join(t.TempDir(), "origin.git")
	if _, err := gogit.PlainInit(remote, true); err != nil {
		t.Fatalf("init bare: %v", err)
	}
	work := t.TempDir()
	repo, _ := gogit.PlainInit(work, false)
	_, _ = repo.CreateRemote(&config.RemoteConfig{Name: "origin", URLs: []string{remote}})
	wt, _ := repo.Worktree()
	commit(t, wt, work, "only.txt", "1", "root")
	head, _ := repo.Head()
	branch := head.Name().Short()
	push(t, repo, "refs/heads/"+branch+":refs/heads/"+branch)

	if _, err := (&Client{}).PreviousRevision(context.Background(), remote, branch, ""); err == nil {
		t.Error("PreviousRevision on root-only branch = nil, want 'no previous commit' error")
	}
}
