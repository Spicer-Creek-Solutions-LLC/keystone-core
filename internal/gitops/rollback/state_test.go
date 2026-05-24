// SPDX-License-Identifier: Apache-2.0

package rollback

import (
	"context"
	"errors"
	"testing"

	"go.keystone-core.io/keystone-core/pkg/statemachine"
)

func TestNewMachine_LegalPath(t *testing.T) {
	t.Parallel()
	m, err := newMachine(StatePending)
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	steps := []struct {
		ev   RollbackEvent
		want RollbackState
	}{
		{EventApprove, StateApproved},
		{EventStart, StateInProgress},
		{EventComplete, StateCompleted},
		{EventVerify, StateVerifying},
		{EventVerifyOK, StateVerified},
	}
	for _, s := range steps {
		if err := m.Fire(context.Background(), s.ev); err != nil {
			t.Fatalf("Fire(%s): %v", s.ev, err)
		}
		if m.Current() != s.want {
			t.Fatalf("after %s state = %s, want %s", s.ev, m.Current(), s.want)
		}
	}
}

func TestNewMachine_ForkAndFailEdges(t *testing.T) {
	t.Parallel()
	// Pending → Rejected
	m, _ := newMachine(StatePending)
	if err := m.Fire(context.Background(), EventReject); err != nil || m.Current() != StateRejected {
		t.Fatalf("reject: state=%s err=%v", m.Current(), err)
	}
	// InProgress → Failed
	m2, _ := newMachine(StateInProgress)
	if err := m2.Fire(context.Background(), EventFail); err != nil || m2.Current() != StateFailed {
		t.Fatalf("fail: state=%s err=%v", m2.Current(), err)
	}
	// Verifying → VerificationFailed
	m3, _ := newMachine(StateVerifying)
	if err := m3.Fire(context.Background(), EventVerifyFail); err != nil || m3.Current() != StateVerificationFailed {
		t.Fatalf("verify_fail: state=%s err=%v", m3.Current(), err)
	}
}

func TestNewMachine_IllegalTransitions(t *testing.T) {
	t.Parallel()
	cases := []struct {
		from RollbackState
		ev   RollbackEvent
	}{
		{StatePending, EventStart},    // must approve first
		{StatePending, EventComplete}, // skip the lifecycle
		{StateApproved, EventApprove}, // double approve
		{StateCompleted, EventStart},  // re-run
		{StateRejected, EventApprove}, // terminal
		{StateFailed, EventComplete},  // terminal
		{StateVerified, EventVerify},  // terminal
	}
	for _, c := range cases {
		m, _ := newMachine(c.from)
		err := m.Fire(context.Background(), c.ev)
		if err == nil {
			t.Errorf("Fire(%s) from %s = nil, want rejection", c.ev, c.from)
		}
		if !errors.Is(err, statemachine.ErrNoTransition) && !errors.Is(err, statemachine.ErrUnknownEvent) {
			t.Errorf("from %s ev %s: err = %v, want NoTransition/UnknownEvent", c.from, c.ev, err)
		}
		if m.Current() != c.from {
			t.Errorf("rejected transition moved state to %s", m.Current())
		}
	}
}

func TestRollbackState_IsTerminal(t *testing.T) {
	t.Parallel()
	term := []RollbackState{StateRejected, StateFailed, StateVerified, StateVerificationFailed}
	for _, s := range term {
		if !s.IsTerminal() {
			t.Errorf("%s.IsTerminal() = false", s)
		}
	}
	for _, s := range []RollbackState{StatePending, StateApproved, StateInProgress, StateCompleted, StateVerifying} {
		if s.IsTerminal() {
			t.Errorf("%s.IsTerminal() = true, want false", s)
		}
	}
}
