package rollback

import (
	"context"
	"errors"
	"testing"
	"time"
)

// fakeExec is a configurable rollback.Executor for engine tests. It
// captures the Config it was called with so tests can assert the
// engine forwarded the rollback's persisted Config across the
// approval gate.
type fakeExec struct {
	typ    string
	res    Result
	gotCfg Config
}

func (f *fakeExec) Type() string { return f.typ }
func (f *fakeExec) Execute(_ context.Context, cfg Config, _ Request) Result {
	f.gotCfg = cfg
	return f.res
}
func (f *fakeExec) GetPreviousRevision(context.Context, Config, Request) (string, error) {
	return "prev", nil
}
func (f *fakeExec) GetLastKnownGood(context.Context, Config, Request) (string, error) {
	return "lkg", nil
}

type fakeVerifier struct {
	ok  bool
	err error
}

func (v fakeVerifier) Verify(context.Context, *Rollback) (bool, error) { return v.ok, v.err }

func newEngine(t *testing.T, x Executor, opts ...Option) *Engine {
	t.Helper()
	e := NewEngine(NewMemoryStore(), opts...)
	if x != nil {
		if err := e.RegisterExecutor(x); err != nil {
			t.Fatalf("RegisterExecutor: %v", err)
		}
	}
	return e
}

func okSpec() RollbackSpec {
	return RollbackSpec{
		ExecutorType: "git",
		Config:       Config{"repo_url": "https://r"},
		Request:      Request{Application: "web", Strategy: StrategySpecific, Revision: "c1", Reason: "hotfix"},
	}
}

func TestEngine_Execute_NoApproval_Completed(t *testing.T) {
	t.Parallel()
	e := newEngine(t, &fakeExec{typ: "git", res: Result{Success: true, FromRevision: "c2", ToRevision: "c1"}})
	rb, err := e.Execute(context.Background(), okSpec())
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if rb.State != StateCompleted {
		t.Fatalf("State = %s, want completed", rb.State)
	}
	if rb.FromRevision != "c2" || rb.ToRevision != "c1" {
		t.Errorf("revisions = %s→%s", rb.FromRevision, rb.ToRevision)
	}
	// Pending→Approved→InProgress→Completed = 3 transitions.
	if len(rb.Transitions) != 3 {
		t.Errorf("transitions = %d, want 3 (%+v)", len(rb.Transitions), rb.Transitions)
	}
	got, ok, _ := e.GetRollback(context.Background(), rb.ID)
	if !ok || got.State != StateCompleted {
		t.Errorf("stored state = %v ok=%v", got, ok)
	}
}

func TestEngine_ApprovalGate(t *testing.T) {
	t.Parallel()
	e := newEngine(t, &fakeExec{typ: "git", res: Result{Success: true}})
	spec := okSpec()
	spec.RequireApproval = true

	rb, err := e.Execute(context.Background(), spec)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if rb.State != StatePending {
		t.Fatalf("State = %s, want pending (approval gate)", rb.State)
	}

	done, err := e.ApproveRollback(context.Background(), rb.ID, "alice")
	if err != nil {
		t.Fatalf("Approve: %v", err)
	}
	if done.State != StateCompleted || done.Approver != "alice" {
		t.Errorf("after approve: state=%s approver=%s", done.State, done.Approver)
	}
}

func TestEngine_Reject(t *testing.T) {
	t.Parallel()
	e := newEngine(t, &fakeExec{typ: "git", res: Result{Success: true}})
	spec := okSpec()
	spec.RequireApproval = true
	rb, _ := e.Execute(context.Background(), spec)

	got, err := e.RejectRollback(context.Background(), rb.ID, "bob", "not now")
	if err != nil {
		t.Fatalf("Reject: %v", err)
	}
	if got.State != StateRejected || got.Approver != "bob" {
		t.Errorf("state=%s approver=%s, want rejected/bob", got.State, got.Approver)
	}
	if got.Error == "" {
		t.Error("reject reason not recorded")
	}
}

func TestEngine_ExecutorFailure(t *testing.T) {
	t.Parallel()
	e := newEngine(t, &fakeExec{typ: "git", res: Result{Success: false, Error: errors.New("push denied")}})
	rb, err := e.Execute(context.Background(), okSpec())
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if rb.State != StateFailed {
		t.Fatalf("State = %s, want failed", rb.State)
	}
	if rb.Error != "push denied" {
		t.Errorf("Error = %q, want push denied", rb.Error)
	}
}

func TestEngine_PostVerification(t *testing.T) {
	t.Parallel()

	t.Run("verified", func(t *testing.T) {
		t.Parallel()
		e := newEngine(t, &fakeExec{typ: "git", res: Result{Success: true}}, WithPostVerifier(fakeVerifier{ok: true}))
		rb, _ := e.Execute(context.Background(), okSpec())
		if rb.State != StateVerified {
			t.Fatalf("State = %s, want verified", rb.State)
		}
	})

	t.Run("verification failed", func(t *testing.T) {
		t.Parallel()
		e := newEngine(t, &fakeExec{typ: "git", res: Result{Success: true}}, WithPostVerifier(fakeVerifier{ok: false}))
		rb, _ := e.Execute(context.Background(), okSpec())
		if rb.State != StateVerificationFailed {
			t.Fatalf("State = %s, want verification_failed", rb.State)
		}
	})

	t.Run("verifier error", func(t *testing.T) {
		t.Parallel()
		e := newEngine(t, &fakeExec{typ: "git", res: Result{Success: true}}, WithPostVerifier(fakeVerifier{err: errors.New("probe down")}))
		rb, _ := e.Execute(context.Background(), okSpec())
		if rb.State != StateVerificationFailed || rb.Error == "" {
			t.Fatalf("state=%s err=%q, want verification_failed + error", rb.State, rb.Error)
		}
	})
}

func TestEngine_UnknownExecutor(t *testing.T) {
	t.Parallel()
	e := newEngine(t, nil)
	if _, err := e.Execute(context.Background(), okSpec()); !errors.Is(err, ErrUnknownExecutor) {
		t.Errorf("err = %v, want ErrUnknownExecutor", err)
	}
}

func TestEngine_DoubleApprove(t *testing.T) {
	t.Parallel()
	e := newEngine(t, &fakeExec{typ: "git", res: Result{Success: true}})
	rb, _ := e.Execute(context.Background(), okSpec()) // no approval → already Completed
	_, err := e.ApproveRollback(context.Background(), rb.ID, "x")
	if !errors.Is(err, ErrInvalidTransition) {
		t.Errorf("approve on completed: err = %v, want ErrInvalidTransition", err)
	}
}

func TestEngine_GetUnknown(t *testing.T) {
	t.Parallel()
	e := newEngine(t, nil)
	if _, err := e.ApproveRollback(context.Background(), "nope", "x"); !errors.Is(err, ErrRollbackNotFound) {
		t.Errorf("err = %v, want ErrRollbackNotFound", err)
	}
	_, ok, _ := e.GetRollback(context.Background(), "nope")
	if ok {
		t.Error("GetRollback(nope) ok = true")
	}
}

func TestEngine_ListAndDeterministicIDClock(t *testing.T) {
	t.Parallel()
	fixed := time.Date(2026, 5, 19, 12, 0, 0, 0, time.UTC)
	n := 0
	e := newEngine(t, &fakeExec{typ: "git", res: Result{Success: true}},
		WithClock(func() time.Time { return fixed }),
		WithIDGenerator(func() string { n++; return "rb-" + string(rune('0'+n)) }))
	r1, _ := e.Execute(context.Background(), okSpec())
	r2, _ := e.Execute(context.Background(), okSpec())
	if r1.ID != "rb-1" || r2.ID != "rb-2" {
		t.Errorf("ids = %s,%s", r1.ID, r2.ID)
	}
	if !r1.CreatedAt.Equal(fixed) {
		t.Errorf("CreatedAt = %v, want injected clock", r1.CreatedAt)
	}
	list, _ := e.ListRollbacks(context.Background())
	if len(list) != 2 {
		t.Errorf("List len = %d, want 2", len(list))
	}
}

func TestMemoryStore_ReturnsCopies(t *testing.T) {
	t.Parallel()
	s := NewMemoryStore()
	rb := &Rollback{ID: "a", State: StatePending, Transitions: []TransitionRecord{{Event: EventApprove}}}
	if err := s.Save(context.Background(), rb); err != nil {
		t.Fatal(err)
	}
	got, _, _ := s.Get(context.Background(), "a")
	got.State = StateFailed
	got.Transitions[0].Event = EventReject
	again, _, _ := s.Get(context.Background(), "a")
	if again.State != StatePending || again.Transitions[0].Event != EventApprove {
		t.Errorf("store aliased: mutation leaked (%s / %s)", again.State, again.Transitions[0].Event)
	}
	if err := s.Save(context.Background(), &Rollback{}); err == nil {
		t.Error("Save(empty id) = nil, want error")
	}
}

func TestEngine_ApprovalGate_UsesPersistedConfig(t *testing.T) {
	t.Parallel()
	// Bug surfaced by task 10: an approval-gated rollback that returns
	// at Pending must, on Approve, re-drive the executor with the
	// Config supplied at Execute (otherwise the rollback can't run —
	// the executor needs e.g. repo_url). Task 10 fix: persist Config
	// on the Rollback record and reuse it in ApproveRollback.
	fe := &fakeExec{typ: "git", res: Result{Success: true}}
	e := newEngine(t, fe)
	spec := okSpec()
	spec.RequireApproval = true

	rb, _ := e.Execute(context.Background(), spec)
	if rb.State != StatePending {
		t.Fatalf("State = %s, want pending", rb.State)
	}
	if rb.Config["repo_url"] != "https://r" {
		t.Errorf("Config not persisted on Pending record: %+v", rb.Config)
	}

	done, err := e.ApproveRollback(context.Background(), rb.ID, "alice")
	if err != nil {
		t.Fatalf("Approve: %v", err)
	}
	if done.State != StateCompleted {
		t.Fatalf("State = %s, want completed", done.State)
	}
	if fe.gotCfg["repo_url"] != "https://r" {
		t.Errorf("executor on Approve got cfg %+v, want repo_url=https://r (Config dropped across approval gate)", fe.gotCfg)
	}
}
