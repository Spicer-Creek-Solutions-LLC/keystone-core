// SPDX-License-Identifier: Apache-2.0

package rollback

import (
	"context"
	"errors"
	"testing"
)

type fakeGit struct {
	revertReq GitRevertRequest
	revertRes GitRevertResult
	revertErr error
	prev      string
	lkg       string
	resErr    error
}

func (f *fakeGit) Revert(_ context.Context, req GitRevertRequest) (GitRevertResult, error) {
	f.revertReq = req
	return f.revertRes, f.revertErr
}
func (f *fakeGit) PreviousRevision(context.Context, string, string, string) (string, error) {
	return f.prev, f.resErr
}
func (f *fakeGit) LastKnownGood(context.Context, string, string, string) (string, error) {
	return f.lkg, f.resErr
}

func TestGitRevertExecutor_Execute(t *testing.T) {
	t.Parallel()

	t.Run("specific success", func(t *testing.T) {
		t.Parallel()
		fg := &fakeGit{revertRes: GitRevertResult{FromRevision: "c2", ToRevision: "c1", NewCommit: "c3"}}
		r := GitRevertExecutor{Client: fg}.Execute(context.Background(),
			Config{"repo_url": "https://r", "branch": "dev"},
			Request{Strategy: StrategySpecific, Revision: "c1", Reason: "hotfix"})
		if !r.Success {
			t.Fatalf("Success=false: %q %v", r.Message, r.Error)
		}
		if r.FromRevision != "c2" || r.ToRevision != "c1" || r.Data["new_commit"] != "c3" {
			t.Errorf("unexpected result: %+v", r)
		}
		if fg.revertReq.Branch != "dev" || fg.revertReq.ToRevision != "c1" {
			t.Errorf("revert req wrong: %+v", fg.revertReq)
		}
		if fg.revertReq.Message == "" || fg.revertReq.Message == "Revert: " {
			t.Errorf("revert message should include reason: %q", fg.revertReq.Message)
		}
	})

	t.Run("previous strategy resolves via client", func(t *testing.T) {
		t.Parallel()
		fg := &fakeGit{prev: "prevsha", revertRes: GitRevertResult{ToRevision: "prevsha"}}
		r := GitRevertExecutor{Client: fg}.Execute(context.Background(),
			Config{"repo_url": "https://r"}, Request{Strategy: StrategyPrevious})
		if !r.Success || fg.revertReq.ToRevision != "prevsha" {
			t.Errorf("previous strategy not resolved: %+v / %+v", r, fg.revertReq)
		}
	})

	t.Run("nil client", func(t *testing.T) {
		t.Parallel()
		r := GitRevertExecutor{}.Execute(context.Background(), Config{"repo_url": "x"}, Request{Strategy: StrategySpecific, Revision: "a"})
		if r.Success || !errors.Is(r.Error, ErrNotConfigured) {
			t.Errorf("want ErrNotConfigured, got %+v", r)
		}
	})

	t.Run("missing repo_url", func(t *testing.T) {
		t.Parallel()
		r := GitRevertExecutor{Client: &fakeGit{}}.Execute(context.Background(), Config{}, Request{Strategy: StrategySpecific, Revision: "a"})
		if r.Success || !errors.Is(r.Error, ErrConfig) {
			t.Errorf("want ErrConfig, got %+v", r)
		}
	})

	t.Run("revert error", func(t *testing.T) {
		t.Parallel()
		r := GitRevertExecutor{Client: &fakeGit{revertErr: errors.New("push denied")}}.
			Execute(context.Background(), Config{"repo_url": "x"}, Request{Strategy: StrategySpecific, Revision: "a"})
		if r.Success || r.Error == nil {
			t.Errorf("want failed result, got %+v", r)
		}
	})
}

func TestGitRevertExecutor_Resolvers(t *testing.T) {
	t.Parallel()
	fg := &fakeGit{prev: "p", lkg: "g"}
	e := GitRevertExecutor{Client: fg}
	if v, _ := e.GetPreviousRevision(context.Background(), Config{"repo_url": "x"}, Request{}); v != "p" {
		t.Errorf("prev = %q, want p", v)
	}
	if v, _ := e.GetLastKnownGood(context.Background(), Config{"repo_url": "x"}, Request{}); v != "g" {
		t.Errorf("lkg = %q, want g", v)
	}
	if _, err := (GitRevertExecutor{}).GetPreviousRevision(context.Background(), Config{"repo_url": "x"}, Request{}); !errors.Is(err, ErrNotConfigured) {
		t.Errorf("nil client err = %v, want ErrNotConfigured", err)
	}
	if _, err := e.GetPreviousRevision(context.Background(), Config{}, Request{}); !errors.Is(err, ErrConfig) {
		t.Errorf("missing repo_url err = %v, want ErrConfig", err)
	}
}

func TestGitRevertExecutor_Type(t *testing.T) {
	t.Parallel()
	if (GitRevertExecutor{}).Type() != "git" {
		t.Error("Type() != git")
	}
}
