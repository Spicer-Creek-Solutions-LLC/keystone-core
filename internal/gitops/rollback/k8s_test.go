// SPDX-License-Identifier: Apache-2.0

package rollback

import (
	"context"
	"errors"
	"testing"
)

type fakeK8s struct {
	hist      []K8sRevision
	histErr   error
	undoErr   error
	undoNS    string
	undoDep   string
	undoToRev int64
}

func (f *fakeK8s) RevisionHistory(context.Context, string, string) ([]K8sRevision, error) {
	return f.hist, f.histErr
}
func (f *fakeK8s) RolloutUndo(_ context.Context, ns, dep string, toRev int64) error {
	f.undoNS, f.undoDep, f.undoToRev = ns, dep, toRev
	return f.undoErr
}

func TestK8sRolloutExecutor_Execute(t *testing.T) {
	t.Parallel()

	t.Run("previous → undo to 0", func(t *testing.T) {
		t.Parallel()
		fk := &fakeK8s{}
		r := K8sRolloutExecutor{Client: fk}.Execute(context.Background(),
			Config{"deployment": "api", "namespace": "prod"}, Request{Strategy: StrategyPrevious})
		if !r.Success {
			t.Fatalf("Success=false: %q %v", r.Message, r.Error)
		}
		if fk.undoNS != "prod" || fk.undoDep != "api" || fk.undoToRev != 0 {
			t.Errorf("undo args %+v, want prod/api/0", fk)
		}
		if r.ToRevision != "previous" {
			t.Errorf("ToRevision = %q, want previous", r.ToRevision)
		}
	})

	t.Run("specific → parsed revision number", func(t *testing.T) {
		t.Parallel()
		fk := &fakeK8s{}
		r := K8sRolloutExecutor{Client: fk}.Execute(context.Background(),
			Config{"deployment": "api"}, Request{Strategy: StrategySpecific, Revision: "5"})
		if !r.Success || fk.undoToRev != 5 || fk.undoNS != "default" {
			t.Errorf("specific undo wrong: %+v / %+v", r, fk)
		}
	})

	t.Run("specific non-integer revision", func(t *testing.T) {
		t.Parallel()
		r := K8sRolloutExecutor{Client: &fakeK8s{}}.Execute(context.Background(),
			Config{"deployment": "api"}, Request{Strategy: StrategySpecific, Revision: "abc"})
		if r.Success || !errors.Is(r.Error, ErrConfig) {
			t.Errorf("want ErrConfig, got %+v", r)
		}
	})

	t.Run("last-known-good resolves history[-2]", func(t *testing.T) {
		t.Parallel()
		fk := &fakeK8s{hist: []K8sRevision{{Revision: 7}, {Revision: 8}, {Revision: 9}}}
		r := K8sRolloutExecutor{Client: fk}.Execute(context.Background(),
			Config{"deployment": "api"}, Request{Strategy: StrategyLastKnownGood})
		if !r.Success || fk.undoToRev != 8 {
			t.Errorf("lkg undo = %d, want 8 (%+v)", fk.undoToRev, r)
		}
	})

	t.Run("nil client → ErrNotConfigured (client-go deferred)", func(t *testing.T) {
		t.Parallel()
		r := K8sRolloutExecutor{}.Execute(context.Background(), Config{"deployment": "api"}, Request{Strategy: StrategyPrevious})
		if r.Success || !errors.Is(r.Error, ErrNotConfigured) {
			t.Errorf("want ErrNotConfigured, got %+v", r)
		}
	})

	t.Run("missing deployment", func(t *testing.T) {
		t.Parallel()
		r := K8sRolloutExecutor{Client: &fakeK8s{}}.Execute(context.Background(), Config{}, Request{Strategy: StrategyPrevious})
		if r.Success || !errors.Is(r.Error, ErrConfig) {
			t.Errorf("want ErrConfig, got %+v", r)
		}
	})

	t.Run("undo error", func(t *testing.T) {
		t.Parallel()
		r := K8sRolloutExecutor{Client: &fakeK8s{undoErr: errors.New("forbidden")}}.
			Execute(context.Background(), Config{"deployment": "api"}, Request{Strategy: StrategyPrevious})
		if r.Success || r.Error == nil {
			t.Errorf("want failure, got %+v", r)
		}
	})
}

func TestK8sRolloutExecutor_Previous(t *testing.T) {
	t.Parallel()
	fk := &fakeK8s{hist: []K8sRevision{{Revision: 3}, {Revision: 4}}}
	v, err := (K8sRolloutExecutor{Client: fk}).GetPreviousRevision(context.Background(), Config{"deployment": "d"}, Request{})
	if err != nil || v != "3" {
		t.Errorf("prev = %q,%v want 3,nil", v, err)
	}
	if _, err := (K8sRolloutExecutor{}).GetPreviousRevision(context.Background(), Config{"deployment": "d"}, Request{}); !errors.Is(err, ErrNotConfigured) {
		t.Errorf("nil client err = %v, want ErrNotConfigured", err)
	}
}

func TestK8sRolloutExecutor_Type(t *testing.T) {
	t.Parallel()
	if (K8sRolloutExecutor{}).Type() != "k8s" {
		t.Error("Type() != k8s")
	}
}
