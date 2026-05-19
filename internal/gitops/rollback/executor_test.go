package rollback

import (
	"context"
	"errors"
	"testing"
	"time"
)

type stubExecutor struct {
	typ  string
	prev string
	lkg  string
	err  error
}

func (s stubExecutor) Type() string { return s.typ }
func (s stubExecutor) Execute(context.Context, Config, Request) Result {
	return Result{Success: true}
}
func (s stubExecutor) GetPreviousRevision(context.Context, Config, Request) (string, error) {
	return s.prev, s.err
}
func (s stubExecutor) GetLastKnownGood(context.Context, Config, Request) (string, error) {
	return s.lkg, s.err
}

func TestStrategy_Valid(t *testing.T) {
	t.Parallel()
	for _, s := range []Strategy{StrategyPrevious, StrategySpecific, StrategyLastKnownGood} {
		if !s.Valid() {
			t.Errorf("%q.Valid() = false", s)
		}
	}
	if Strategy("rollforward").Valid() {
		t.Error("bogus strategy reported valid")
	}
}

func TestRegistry(t *testing.T) {
	t.Parallel()
	reg := NewRegistry()
	if err := reg.Register(stubExecutor{typ: "git"}); err != nil {
		t.Fatalf("Register: %v", err)
	}
	if _, ok := reg.Lookup("git"); !ok {
		t.Error("Lookup(git) missing")
	}
	if _, ok := reg.Lookup("k8s"); ok {
		t.Error("Lookup(k8s) unexpectedly present")
	}
	if err := reg.Register(nil); err == nil {
		t.Error("Register(nil) = nil, want error")
	}
	if err := reg.Register(stubExecutor{typ: ""}); err == nil {
		t.Error("Register(empty type) = nil, want error")
	}
}

func TestResolveTarget(t *testing.T) {
	t.Parallel()
	e := stubExecutor{prev: "prevsha", lkg: "lkgsha"}

	got, err := resolveTarget(context.Background(), e, nil, Request{Strategy: StrategySpecific, Revision: "abc"})
	if err != nil || got != "abc" {
		t.Errorf("specific = %q,%v want abc,nil", got, err)
	}
	if _, err := resolveTarget(context.Background(), e, nil, Request{Strategy: StrategySpecific}); !errors.Is(err, ErrConfig) {
		t.Errorf("specific w/o revision err = %v, want ErrConfig", err)
	}
	if got, _ := resolveTarget(context.Background(), e, nil, Request{Strategy: StrategyPrevious}); got != "prevsha" {
		t.Errorf("previous = %q, want prevsha", got)
	}
	if got, _ := resolveTarget(context.Background(), e, nil, Request{Strategy: StrategyLastKnownGood}); got != "lkgsha" {
		t.Errorf("lkg = %q, want lkgsha", got)
	}
	if _, err := resolveTarget(context.Background(), e, nil, Request{Strategy: "bogus"}); !errors.Is(err, ErrConfig) {
		t.Errorf("bogus strategy err = %v, want ErrConfig", err)
	}
}

func TestFailf(t *testing.T) {
	t.Parallel()
	r := failf(time.Now().Add(-time.Millisecond), ErrConfig, "x %d", 1)
	if r.Success || r.Message != "x 1" || !errors.Is(r.Error, ErrConfig) || r.Duration <= 0 {
		t.Errorf("failf bad result: %+v", r)
	}
}
