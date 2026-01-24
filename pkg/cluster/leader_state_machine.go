package cluster

import (
	"context"
	"time"

	"github.com/shawnbutts/keystone-core/pkg/statemachine"
)

// LeaderState represents the leadership state of a cluster member.
//
// State diagram:
//
// ```mermaid
// stateDiagram-v2
//     [*] --> Idle
//     Idle --> Campaigning: Start
//     Campaigning --> Leader: WonElection
//     Campaigning --> Follower: LostElection
//     Campaigning --> Stopped: Stop
//     Leader --> Follower: LostLeadership
//     Leader --> Campaigning: SessionExpired
//     Leader --> Stopped: Stop
//     Follower --> Campaigning: LeaderLost
//     Follower --> Stopped: Stop
//     Stopped --> [*]
// ```
type LeaderState string

const (
	// LeaderStateIdle indicates not participating in election
	LeaderStateIdle LeaderState = "idle"
	// LeaderStateCampaigning indicates actively trying to become leader
	LeaderStateCampaigning LeaderState = "campaigning"
	// LeaderStateLeader indicates this member is the current leader
	LeaderStateLeader LeaderState = "leader"
	// LeaderStateFollower indicates following another leader
	LeaderStateFollower LeaderState = "follower"
	// LeaderStateStopped indicates election participation has stopped
	LeaderStateStopped LeaderState = "stopped"
)

// LeaderStateEvent represents events that trigger leadership state transitions.
type LeaderStateEvent string

const (
	// LeaderEventStart starts the election process
	LeaderEventStart LeaderStateEvent = "start"
	// LeaderEventWonElection indicates this member won the election
	LeaderEventWonElection LeaderStateEvent = "won_election"
	// LeaderEventLostElection indicates another member won
	LeaderEventLostElection LeaderStateEvent = "lost_election"
	// LeaderEventLostLeadership indicates leadership was lost
	LeaderEventLostLeadership LeaderStateEvent = "lost_leadership"
	// LeaderEventSessionExpired indicates the session expired
	LeaderEventSessionExpired LeaderStateEvent = "session_expired"
	// LeaderEventLeaderLost indicates the current leader is gone
	LeaderEventLeaderLost LeaderStateEvent = "leader_lost"
	// LeaderEventResign voluntarily resigns leadership
	LeaderEventResign LeaderStateEvent = "resign"
	// LeaderEventStop stops the election process
	LeaderEventStop LeaderStateEvent = "stop"
)

// LeaderStateCallbacks holds callbacks for leadership state transitions.
type LeaderStateCallbacks struct {
	// OnStartCampaigning is called when starting to campaign
	OnStartCampaigning func(memberID string)
	// OnBecameLeader is called when this member becomes leader
	OnBecameLeader func(memberID string)
	// OnBecameFollower is called when this member becomes a follower
	OnBecameFollower func(memberID, leaderID string)
	// OnLostLeadership is called when leadership is lost
	OnLostLeadership func(memberID, reason string)
	// OnStopped is called when election stops
	OnStopped func(memberID string)
}

// ManagedLeaderElection wraps leadership state with a state machine.
type ManagedLeaderElection struct {
	machine *statemachine.Machine[LeaderState, LeaderStateEvent]

	// Tracking
	memberID      string
	currentLeader string
	callbacks     *LeaderStateCallbacks
	lostReason    string
}

// NewManagedLeaderElection creates a new managed leader election with state machine.
func NewManagedLeaderElection(memberID string, callbacks *LeaderStateCallbacks) *ManagedLeaderElection {
	mle := &ManagedLeaderElection{
		memberID:  memberID,
		callbacks: callbacks,
	}

	mle.machine = statemachine.New[LeaderState, LeaderStateEvent](LeaderStateIdle).
		WithName("leader-election-" + memberID).
		WithHistory(20).
		// From Idle
		AddTransition(LeaderStateIdle, LeaderEventStart, LeaderStateCampaigning).
		// From Campaigning
		AddTransition(LeaderStateCampaigning, LeaderEventWonElection, LeaderStateLeader).
		AddTransition(LeaderStateCampaigning, LeaderEventLostElection, LeaderStateFollower).
		AddTransition(LeaderStateCampaigning, LeaderEventStop, LeaderStateStopped).
		// From Leader
		AddTransition(LeaderStateLeader, LeaderEventLostLeadership, LeaderStateFollower).
		AddTransition(LeaderStateLeader, LeaderEventSessionExpired, LeaderStateCampaigning).
		AddTransition(LeaderStateLeader, LeaderEventResign, LeaderStateFollower).
		AddTransition(LeaderStateLeader, LeaderEventStop, LeaderStateStopped).
		// From Follower
		AddTransition(LeaderStateFollower, LeaderEventLeaderLost, LeaderStateCampaigning).
		AddTransition(LeaderStateFollower, LeaderEventWonElection, LeaderStateLeader).
		AddTransition(LeaderStateFollower, LeaderEventStop, LeaderStateStopped).
		// Callbacks
		OnEnter(LeaderStateCampaigning, func(ctx context.Context, state, from LeaderState) {
			if mle.callbacks != nil && mle.callbacks.OnStartCampaigning != nil {
				mle.callbacks.OnStartCampaigning(mle.memberID)
			}
		}).
		OnEnter(LeaderStateLeader, func(ctx context.Context, state, from LeaderState) {
			mle.currentLeader = mle.memberID
			if mle.callbacks != nil && mle.callbacks.OnBecameLeader != nil {
				mle.callbacks.OnBecameLeader(mle.memberID)
			}
		}).
		OnEnter(LeaderStateFollower, func(ctx context.Context, state, from LeaderState) {
			if from == LeaderStateLeader {
				if mle.callbacks != nil && mle.callbacks.OnLostLeadership != nil {
					mle.callbacks.OnLostLeadership(mle.memberID, mle.lostReason)
				}
			}
			if mle.callbacks != nil && mle.callbacks.OnBecameFollower != nil {
				mle.callbacks.OnBecameFollower(mle.memberID, mle.currentLeader)
			}
		}).
		OnEnter(LeaderStateStopped, func(ctx context.Context, state, from LeaderState) {
			if mle.callbacks != nil && mle.callbacks.OnStopped != nil {
				mle.callbacks.OnStopped(mle.memberID)
			}
		}).
		MustBuild()

	return mle
}

// State returns the current leadership state.
func (mle *ManagedLeaderElection) State() LeaderState {
	return mle.machine.State()
}

// Start starts the election process.
func (mle *ManagedLeaderElection) Start() error {
	return mle.machine.Fire(LeaderEventStart)
}

// WonElection marks that this member won the election.
func (mle *ManagedLeaderElection) WonElection() error {
	return mle.machine.Fire(LeaderEventWonElection)
}

// LostElection marks that another member won the election.
func (mle *ManagedLeaderElection) LostElection(leaderID string) error {
	mle.currentLeader = leaderID
	return mle.machine.Fire(LeaderEventLostElection)
}

// LostLeadership marks that leadership was lost.
func (mle *ManagedLeaderElection) LostLeadership(reason string) error {
	mle.lostReason = reason
	mle.currentLeader = ""
	return mle.machine.Fire(LeaderEventLostLeadership)
}

// SessionExpired marks that the session expired.
func (mle *ManagedLeaderElection) SessionExpired() error {
	mle.lostReason = "session expired"
	return mle.machine.Fire(LeaderEventSessionExpired)
}

// LeaderLost marks that the current leader is gone.
func (mle *ManagedLeaderElection) LeaderLost() error {
	mle.currentLeader = ""
	return mle.machine.Fire(LeaderEventLeaderLost)
}

// Resign voluntarily resigns leadership.
func (mle *ManagedLeaderElection) Resign() error {
	mle.lostReason = "voluntary resignation"
	return mle.machine.Fire(LeaderEventResign)
}

// Stop stops the election process.
func (mle *ManagedLeaderElection) Stop() error {
	return mle.machine.Fire(LeaderEventStop)
}

// IsIdle returns true if not participating in election.
func (mle *ManagedLeaderElection) IsIdle() bool {
	return mle.machine.IsInState(LeaderStateIdle)
}

// IsCampaigning returns true if actively campaigning.
func (mle *ManagedLeaderElection) IsCampaigning() bool {
	return mle.machine.IsInState(LeaderStateCampaigning)
}

// IsLeader returns true if this member is the leader.
func (mle *ManagedLeaderElection) IsLeader() bool {
	return mle.machine.IsInState(LeaderStateLeader)
}

// IsFollower returns true if following another leader.
func (mle *ManagedLeaderElection) IsFollower() bool {
	return mle.machine.IsInState(LeaderStateFollower)
}

// IsStopped returns true if election has stopped.
func (mle *ManagedLeaderElection) IsStopped() bool {
	return mle.machine.IsInState(LeaderStateStopped)
}

// IsActive returns true if participating in election (campaigning, leader, or follower).
func (mle *ManagedLeaderElection) IsActive() bool {
	return mle.machine.IsInAnyState(LeaderStateCampaigning, LeaderStateLeader, LeaderStateFollower)
}

// CanStart returns true if election can be started.
func (mle *ManagedLeaderElection) CanStart() bool {
	return mle.machine.CanFire(LeaderEventStart)
}

// CanStop returns true if election can be stopped.
func (mle *ManagedLeaderElection) CanStop() bool {
	return mle.machine.CanFire(LeaderEventStop)
}

// CurrentLeader returns the current leader ID.
func (mle *ManagedLeaderElection) CurrentLeader() string {
	if mle.IsLeader() {
		return mle.memberID
	}
	return mle.currentLeader
}

// MemberID returns this member's ID.
func (mle *ManagedLeaderElection) MemberID() string {
	return mle.memberID
}

// History returns the state transition history.
func (mle *ManagedLeaderElection) History() *statemachine.History[LeaderState, LeaderStateEvent] {
	return mle.machine.History()
}

// AvailableEvents returns events that can be fired from the current state.
func (mle *ManagedLeaderElection) AvailableEvents() []LeaderStateEvent {
	return mle.machine.AvailableEvents()
}

// StateDuration returns how long the election has been in the current state.
func (mle *ManagedLeaderElection) StateDuration() time.Duration {
	return mle.machine.StateDuration()
}

// LeaderStateToString returns a human-readable name for the state.
func LeaderStateToString(state LeaderState) string {
	switch state {
	case LeaderStateIdle:
		return "Idle"
	case LeaderStateCampaigning:
		return "Campaigning"
	case LeaderStateLeader:
		return "Leader"
	case LeaderStateFollower:
		return "Follower"
	case LeaderStateStopped:
		return "Stopped"
	default:
		return string(state)
	}
}
