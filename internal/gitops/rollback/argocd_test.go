package rollback

import (
	"context"
	"errors"
	"testing"
)

type fakeArgo struct {
	app     ArgoApp
	getErr  error
	syncErr error
	synced  struct {
		name string
		rev  string
	}
}

func (f *fakeArgo) GetApplication(context.Context, string) (ArgoApp, error) {
	return f.app, f.getErr
}
func (f *fakeArgo) SyncToRevision(_ context.Context, name, rev string) error {
	f.synced.name, f.synced.rev = name, rev
	return f.syncErr
}

func TestArgoCDExecutor_Execute(t *testing.T) {
	t.Parallel()

	t.Run("previous strategy syncs to history[-2]", func(t *testing.T) {
		t.Parallel()
		fa := &fakeArgo{app: ArgoApp{
			SyncRevision: "r3",
			History: []ArgoHistoryEntry{
				{ID: 1, Revision: "r1"}, {ID: 2, Revision: "r2"}, {ID: 3, Revision: "r3"},
			},
		}}
		r := ArgoCDExecutor{Client: fa}.Execute(context.Background(),
			Config{}, Request{Application: "web", Strategy: StrategyPrevious})
		if !r.Success {
			t.Fatalf("Success=false: %q %v", r.Message, r.Error)
		}
		if fa.synced.name != "web" || fa.synced.rev != "r2" {
			t.Errorf("synced %+v, want web/r2", fa.synced)
		}
		if r.FromRevision != "r3" || r.ToRevision != "r2" {
			t.Errorf("revisions %s→%s, want r3→r2", r.FromRevision, r.ToRevision)
		}
	})

	t.Run("config app overrides request application", func(t *testing.T) {
		t.Parallel()
		fa := &fakeArgo{app: ArgoApp{History: []ArgoHistoryEntry{{Revision: "a"}, {Revision: "b"}}}}
		ArgoCDExecutor{Client: fa}.Execute(context.Background(),
			Config{"app": "billing"}, Request{Application: "ignored", Strategy: StrategySpecific, Revision: "x"})
		if fa.synced.name != "billing" {
			t.Errorf("synced app = %q, want billing", fa.synced.name)
		}
	})

	t.Run("nil client", func(t *testing.T) {
		t.Parallel()
		r := ArgoCDExecutor{}.Execute(context.Background(), Config{}, Request{Application: "w", Strategy: StrategySpecific, Revision: "x"})
		if r.Success || !errors.Is(r.Error, ErrNotConfigured) {
			t.Errorf("want ErrNotConfigured, got %+v", r)
		}
	})

	t.Run("empty app name", func(t *testing.T) {
		t.Parallel()
		r := ArgoCDExecutor{Client: &fakeArgo{}}.Execute(context.Background(), Config{}, Request{Strategy: StrategySpecific, Revision: "x"})
		if r.Success || !errors.Is(r.Error, ErrConfig) {
			t.Errorf("want ErrConfig, got %+v", r)
		}
	})

	t.Run("get application error", func(t *testing.T) {
		t.Parallel()
		r := ArgoCDExecutor{Client: &fakeArgo{getErr: errors.New("404")}}.
			Execute(context.Background(), Config{}, Request{Application: "w", Strategy: StrategySpecific, Revision: "x"})
		if r.Success || r.Error == nil {
			t.Errorf("want failure, got %+v", r)
		}
	})

	t.Run("sync error", func(t *testing.T) {
		t.Parallel()
		r := ArgoCDExecutor{Client: &fakeArgo{syncErr: errors.New("conflict")}}.
			Execute(context.Background(), Config{}, Request{Application: "w", Strategy: StrategySpecific, Revision: "x"})
		if r.Success || r.Error == nil {
			t.Errorf("want failure, got %+v", r)
		}
	})
}

func TestArgoCDExecutor_PreviousAndLKG(t *testing.T) {
	t.Parallel()
	fa := &fakeArgo{app: ArgoApp{History: []ArgoHistoryEntry{{Revision: "r1"}, {Revision: "r2"}}}}
	e := ArgoCDExecutor{Client: fa}
	if v, _ := e.GetPreviousRevision(context.Background(), Config{}, Request{Application: "w"}); v != "r1" {
		t.Errorf("prev = %q, want r1", v)
	}
	if v, _ := e.GetLastKnownGood(context.Background(), Config{}, Request{Application: "w"}); v != "r1" {
		t.Errorf("lkg = %q, want r1 (v1.0 best-effort = previous)", v)
	}

	short := &fakeArgo{app: ArgoApp{History: []ArgoHistoryEntry{{Revision: "only"}}}}
	if _, err := (ArgoCDExecutor{Client: short}).GetPreviousRevision(context.Background(), Config{}, Request{Application: "w"}); !errors.Is(err, ErrConfig) {
		t.Errorf("history<2 err = %v, want ErrConfig", err)
	}
}

func TestArgoCDExecutor_Type(t *testing.T) {
	t.Parallel()
	if (ArgoCDExecutor{}).Type() != "argocd" {
		t.Error("Type() != argocd")
	}
}
