package rollback

import (
	"errors"

	"go.keystone-core.io/keystone-core/pkg/statemachine"
)

// ErrInvalidTransition is returned by the engine when an operation is
// attempted from a state that does not allow it (e.g. approving a
// rollback that is not Pending). Wraps the underlying
// statemachine.ErrNoTransition / ErrUnknownEvent.
var ErrInvalidTransition = errors.New("rollback: invalid state transition")

// RollbackState is the rollback lifecycle state.
type RollbackState string

const (
	StatePending            RollbackState = "pending"
	StateApproved           RollbackState = "approved"
	StateRejected           RollbackState = "rejected"
	StateInProgress         RollbackState = "in_progress"
	StateCompleted          RollbackState = "completed"
	StateFailed             RollbackState = "failed"
	StateVerifying          RollbackState = "verifying"
	StateVerified           RollbackState = "verified"
	StateVerificationFailed RollbackState = "verification_failed"
)

// IsTerminal reports whether s is an end state (no outgoing edges in
// the absence of post-verification; Completed is terminal only when
// the engine has no PostVerifier).
func (s RollbackState) IsTerminal() bool {
	switch s {
	case StateRejected, StateFailed, StateVerified, StateVerificationFailed:
		return true
	default:
		return false
	}
}

// RollbackEvent drives the state machine.
type RollbackEvent string

const (
	EventApprove    RollbackEvent = "approve"
	EventReject     RollbackEvent = "reject"
	EventStart      RollbackEvent = "start"
	EventComplete   RollbackEvent = "complete"
	EventFail       RollbackEvent = "fail"
	EventVerify     RollbackEvent = "verify"
	EventVerifyOK   RollbackEvent = "verify_ok"
	EventVerifyFail RollbackEvent = "verify_fail"
)

// newMachine builds the rollback FSM with its starting state set to
// initial — the engine rebuilds a machine from the persisted
// [Rollback.State] for every operation, so approval may arrive long
// after Execute. Topology (epic §16 / PROJECT-DETAILS §4.13):
//
//	Pending  --approve--> Approved --start--> InProgress --complete--> Completed --verify--> Verifying --verify_ok--> Verified
//	Pending  --reject--> Rejected                       --fail-----> Failed                  --verify_fail--> VerificationFailed
func newMachine(initial RollbackState) (*statemachine.Machine[RollbackState, RollbackEvent], error) {
	return statemachine.NewBuilder[RollbackState, RollbackEvent]().
		Initial(initial).
		State(StateRejected, StateFailed, StateVerified, StateVerificationFailed).
		Transition(StatePending, EventApprove, StateApproved).
		Transition(StatePending, EventReject, StateRejected).
		Transition(StateApproved, EventStart, StateInProgress).
		Transition(StateInProgress, EventComplete, StateCompleted).
		Transition(StateInProgress, EventFail, StateFailed).
		Transition(StateCompleted, EventVerify, StateVerifying).
		Transition(StateVerifying, EventVerifyOK, StateVerified).
		Transition(StateVerifying, EventVerifyFail, StateVerificationFailed).
		Build()
}
