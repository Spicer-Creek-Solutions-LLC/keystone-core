package nats

import "testing"

func TestNoopBreaker_AlwaysAllows(t *testing.T) {
	b := noopBreaker{}
	if !b.Allow() {
		t.Error("Allow = false, want true")
	}
	b.OnFailure() // noop
	if !b.Allow() {
		t.Error("Allow after OnFailure = false; noop must not advance state")
	}
	b.OnSuccess() // noop
	if got := b.Status(); got != CircuitClosed {
		t.Errorf("Status = %q, want closed", got)
	}
}

func TestNewBreaker_ReturnsNoop(t *testing.T) {
	b := newBreaker()
	if got := b.Status(); got != CircuitClosed {
		t.Errorf("newBreaker default Status = %q, want closed", got)
	}
}
