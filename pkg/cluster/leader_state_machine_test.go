package cluster

import (
	"errors"
	"testing"

	"github.com/shawnbutts/keystone-core/pkg/statemachine"
)

func TestManagedLeaderElection_InitialState(t *testing.T) {
	mle := NewManagedLeaderElection("member-1", nil)

	if mle.State() != LeaderStateIdle {
		t.Errorf("expected idle state, got %v", mle.State())
	}
	if !mle.IsIdle() {
		t.Error("expected IsIdle() to be true")
	}
	if mle.IsActive() {
		t.Error("expected IsActive() to be false")
	}
	if mle.IsLeader() {
		t.Error("expected IsLeader() to be false")
	}
	if mle.MemberID() != "member-1" {
		t.Errorf("expected member-1, got %s", mle.MemberID())
	}
}

func TestManagedLeaderElection_StartCampaigning(t *testing.T) {
	mle := NewManagedLeaderElection("member-1", nil)

	// Start campaigning
	if !mle.CanStart() {
		t.Error("expected CanStart() to be true")
	}
	if err := mle.Start(); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if mle.State() != LeaderStateCampaigning {
		t.Errorf("expected campaigning state, got %v", mle.State())
	}
	if !mle.IsCampaigning() {
		t.Error("expected IsCampaigning() to be true")
	}
	if !mle.IsActive() {
		t.Error("expected IsActive() to be true")
	}
}

func TestManagedLeaderElection_BecomeLeader(t *testing.T) {
	mle := NewManagedLeaderElection("member-1", nil)

	mle.Start()

	// Win election
	if err := mle.WonElection(); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if mle.State() != LeaderStateLeader {
		t.Errorf("expected leader state, got %v", mle.State())
	}
	if !mle.IsLeader() {
		t.Error("expected IsLeader() to be true")
	}
	if mle.CurrentLeader() != "member-1" {
		t.Errorf("expected current leader to be member-1, got %s", mle.CurrentLeader())
	}
}

func TestManagedLeaderElection_BecomeFollower(t *testing.T) {
	mle := NewManagedLeaderElection("member-1", nil)

	mle.Start()

	// Lose election to another member
	if err := mle.LostElection("member-2"); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if mle.State() != LeaderStateFollower {
		t.Errorf("expected follower state, got %v", mle.State())
	}
	if !mle.IsFollower() {
		t.Error("expected IsFollower() to be true")
	}
	if mle.CurrentLeader() != "member-2" {
		t.Errorf("expected current leader to be member-2, got %s", mle.CurrentLeader())
	}
}

func TestManagedLeaderElection_LoseLeadership(t *testing.T) {
	mle := NewManagedLeaderElection("member-1", nil)

	mle.Start()
	mle.WonElection()

	// Lose leadership
	if err := mle.LostLeadership("network partition"); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if mle.State() != LeaderStateFollower {
		t.Errorf("expected follower state, got %v", mle.State())
	}
	if mle.IsLeader() {
		t.Error("expected IsLeader() to be false")
	}
}

func TestManagedLeaderElection_SessionExpired(t *testing.T) {
	mle := NewManagedLeaderElection("member-1", nil)

	mle.Start()
	mle.WonElection()

	// Session expires
	if err := mle.SessionExpired(); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if mle.State() != LeaderStateCampaigning {
		t.Errorf("expected campaigning state after session expired, got %v", mle.State())
	}
}

func TestManagedLeaderElection_LeaderLost(t *testing.T) {
	mle := NewManagedLeaderElection("member-1", nil)

	mle.Start()
	mle.LostElection("member-2")

	if mle.State() != LeaderStateFollower {
		t.Errorf("expected follower state, got %v", mle.State())
	}

	// Leader is lost, start new election
	if err := mle.LeaderLost(); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if mle.State() != LeaderStateCampaigning {
		t.Errorf("expected campaigning state after leader lost, got %v", mle.State())
	}
}

func TestManagedLeaderElection_Resign(t *testing.T) {
	mle := NewManagedLeaderElection("member-1", nil)

	mle.Start()
	mle.WonElection()

	// Resign
	if err := mle.Resign(); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if mle.State() != LeaderStateFollower {
		t.Errorf("expected follower state after resign, got %v", mle.State())
	}
	if mle.IsLeader() {
		t.Error("expected IsLeader() to be false after resign")
	}
}

func TestManagedLeaderElection_StopFromVariousStates(t *testing.T) {
	tests := []struct {
		name  string
		setup func(*ManagedLeaderElection)
	}{
		{"stop from campaigning", func(mle *ManagedLeaderElection) { mle.Start() }},
		{"stop from leader", func(mle *ManagedLeaderElection) { mle.Start(); mle.WonElection() }},
		{"stop from follower", func(mle *ManagedLeaderElection) { mle.Start(); mle.LostElection("member-2") }},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mle := NewManagedLeaderElection("member-1", nil)
			tt.setup(mle)

			if !mle.CanStop() {
				t.Error("expected CanStop() to be true")
			}
			if err := mle.Stop(); err != nil {
				t.Errorf("unexpected error: %v", err)
			}
			if mle.State() != LeaderStateStopped {
				t.Errorf("expected stopped state, got %v", mle.State())
			}
			if !mle.IsStopped() {
				t.Error("expected IsStopped() to be true")
			}
		})
	}
}

func TestManagedLeaderElection_InvalidTransitions(t *testing.T) {
	mle := NewManagedLeaderElection("member-1", nil)

	// Cannot win election from idle
	err := mle.WonElection()
	if err == nil {
		t.Error("expected error for invalid transition")
	}
	if !errors.Is(err, statemachine.ErrInvalidTransition) {
		t.Errorf("expected ErrInvalidTransition, got %v", err)
	}

	// State should not have changed
	if mle.State() != LeaderStateIdle {
		t.Errorf("state should not have changed, got %v", mle.State())
	}
}

func TestManagedLeaderElection_FollowerWinsElection(t *testing.T) {
	mle := NewManagedLeaderElection("member-1", nil)

	mle.Start()
	mle.LostElection("member-2")

	if mle.State() != LeaderStateFollower {
		t.Errorf("expected follower state, got %v", mle.State())
	}

	// Follower wins next election
	if err := mle.WonElection(); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if mle.State() != LeaderStateLeader {
		t.Errorf("expected leader state, got %v", mle.State())
	}
}

func TestManagedLeaderElection_Callbacks(t *testing.T) {
	var campaigningCalls, leaderCalls, followerCalls, lostCalls, stoppedCalls int
	var lastLeaderID, lastFollowerLeader string

	callbacks := &LeaderStateCallbacks{
		OnStartCampaigning: func(memberID string) {
			campaigningCalls++
		},
		OnBecameLeader: func(memberID string) {
			leaderCalls++
			lastLeaderID = memberID
		},
		OnBecameFollower: func(memberID, leaderID string) {
			followerCalls++
			lastFollowerLeader = leaderID
		},
		OnLostLeadership: func(memberID, reason string) {
			lostCalls++
		},
		OnStopped: func(memberID string) {
			stoppedCalls++
		},
	}

	mle := NewManagedLeaderElection("member-1", callbacks)

	// Start triggers campaigning callback
	mle.Start()
	if campaigningCalls != 1 {
		t.Errorf("expected OnStartCampaigning called once, got %d", campaigningCalls)
	}

	// Win triggers leader callback
	mle.WonElection()
	if leaderCalls != 1 || lastLeaderID != "member-1" {
		t.Errorf("expected OnBecameLeader called once, got %d", leaderCalls)
	}

	// Lose triggers follower callback (and lost callback)
	mle.LostLeadership("test reason")
	if lostCalls != 1 {
		t.Errorf("expected OnLostLeadership called once, got %d", lostCalls)
	}
	if followerCalls != 1 {
		t.Errorf("expected OnBecameFollower called once, got %d", followerCalls)
	}

	// Stop triggers stopped callback
	mle.Stop()
	if stoppedCalls != 1 {
		t.Errorf("expected OnStopped called once, got %d", stoppedCalls)
	}

	// Test follower callback with leader ID
	mle2 := NewManagedLeaderElection("member-2", callbacks)
	mle2.Start()
	mle2.LostElection("member-3")
	if lastFollowerLeader != "member-3" {
		t.Errorf("expected lastFollowerLeader to be member-3, got %s", lastFollowerLeader)
	}
}

func TestManagedLeaderElection_History(t *testing.T) {
	mle := NewManagedLeaderElection("member-1", nil)

	mle.Start()
	mle.WonElection()
	mle.LostLeadership("test")
	mle.Stop()

	history := mle.History()
	if history == nil {
		t.Fatal("history should not be nil")
	}

	records := history.All()
	if len(records) != 4 {
		t.Errorf("expected 4 history records, got %d", len(records))
	}
}

func TestManagedLeaderElection_AvailableEvents(t *testing.T) {
	mle := NewManagedLeaderElection("member-1", nil)

	// From idle, can only start
	events := mle.AvailableEvents()
	if len(events) != 1 {
		t.Errorf("expected 1 available event from idle, got %d", len(events))
	}

	mle.Start()

	// From campaigning, can win, lose, or stop
	events = mle.AvailableEvents()
	if len(events) != 3 {
		t.Errorf("expected 3 available events from campaigning, got %d", len(events))
	}

	mle.WonElection()

	// From leader, can lose, session expire, resign, or stop
	events = mle.AvailableEvents()
	if len(events) != 4 {
		t.Errorf("expected 4 available events from leader, got %d", len(events))
	}
}

func TestManagedLeaderElection_StateDuration(t *testing.T) {
	mle := NewManagedLeaderElection("member-1", nil)

	duration := mle.StateDuration()
	if duration == 0 {
		t.Error("expected non-zero StateDuration for initial state")
	}
}

func TestManagedLeaderElection_NilCallbacks(t *testing.T) {
	// Empty callbacks struct
	callbacks := &LeaderStateCallbacks{}

	mle := NewManagedLeaderElection("member-1", callbacks)

	// These should not panic
	mle.Start()
	mle.WonElection()
	mle.LostLeadership("test")
	mle.Stop()
}

func TestLeaderStateToString(t *testing.T) {
	tests := []struct {
		state   LeaderState
		display string
	}{
		{LeaderStateIdle, "Idle"},
		{LeaderStateCampaigning, "Campaigning"},
		{LeaderStateLeader, "Leader"},
		{LeaderStateFollower, "Follower"},
		{LeaderStateStopped, "Stopped"},
		{LeaderState("unknown"), "unknown"},
	}

	for _, tt := range tests {
		t.Run(string(tt.state), func(t *testing.T) {
			if got := LeaderStateToString(tt.state); got != tt.display {
				t.Errorf("LeaderStateToString(%v) = %v, want %v", tt.state, got, tt.display)
			}
		})
	}
}

func TestManagedLeaderElection_FullElectionCycle(t *testing.T) {
	mle := NewManagedLeaderElection("member-1", nil)

	// Full cycle: idle -> campaign -> leader -> session expired -> campaign -> leader -> resign -> follower -> leader lost -> campaign -> stop
	mle.Start()
	if !mle.IsCampaigning() {
		t.Error("expected campaigning")
	}

	mle.WonElection()
	if !mle.IsLeader() {
		t.Error("expected leader")
	}

	mle.SessionExpired()
	if !mle.IsCampaigning() {
		t.Error("expected campaigning after session expired")
	}

	mle.WonElection()
	if !mle.IsLeader() {
		t.Error("expected leader again")
	}

	mle.Resign()
	if !mle.IsFollower() {
		t.Error("expected follower after resign")
	}

	mle.LeaderLost()
	if !mle.IsCampaigning() {
		t.Error("expected campaigning after leader lost")
	}

	mle.Stop()
	if !mle.IsStopped() {
		t.Error("expected stopped")
	}
}
