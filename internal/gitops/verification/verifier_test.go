// SPDX-License-Identifier: Apache-2.0

package verification

import (
	"context"
	"errors"
	"testing"
	"time"
)

type stubVerifier struct {
	typ string
	res Result
}

func (s stubVerifier) Type() string                        { return s.typ }
func (s stubVerifier) Verify(context.Context, Step) Result { return s.res }

func TestRegistry_RegisterLookup(t *testing.T) {
	t.Parallel()
	reg := NewRegistry()
	if err := reg.Register(stubVerifier{typ: "http"}); err != nil {
		t.Fatalf("Register: %v", err)
	}
	v, ok := reg.Lookup("http")
	if !ok || v.Type() != "http" {
		t.Fatalf("Lookup(http) = %v, %v", v, ok)
	}
	if _, ok := reg.Lookup("grpc"); ok {
		t.Error("Lookup(grpc) = ok, want !ok")
	}
}

func TestRegistry_Register_Errors(t *testing.T) {
	t.Parallel()
	reg := NewRegistry()
	if err := reg.Register(nil); err == nil {
		t.Error("Register(nil) = nil, want error")
	}
	if err := reg.Register(stubVerifier{typ: ""}); err == nil {
		t.Error("Register(empty type) = nil, want error")
	}
}

func TestRegistry_Register_Overwrites(t *testing.T) {
	t.Parallel()
	reg := NewRegistry()
	_ = reg.Register(stubVerifier{typ: "http", res: Result{Message: "v1"}})
	_ = reg.Register(stubVerifier{typ: "http", res: Result{Message: "v2"}})
	v, _ := reg.Lookup("http")
	if got := v.Verify(context.Background(), Step{}); got.Message != "v2" {
		t.Errorf("overwrite failed: message = %q, want v2", got.Message)
	}
}

func TestFailf(t *testing.T) {
	t.Parallel()
	start := time.Now().Add(-5 * time.Millisecond)
	sentinel := errors.New("boom")
	r := failf(start, sentinel, "thing %d failed", 7)
	if r.Success {
		t.Error("Success = true, want false")
	}
	if r.Message != "thing 7 failed" {
		t.Errorf("Message = %q", r.Message)
	}
	if !errors.Is(r.Error, sentinel) {
		t.Errorf("Error = %v, want sentinel", r.Error)
	}
	if r.Duration <= 0 {
		t.Errorf("Duration = %v, want > 0", r.Duration)
	}
}
