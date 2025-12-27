package cluster

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestMemberClone(t *testing.T) {
	t.Run("clone with metadata", func(t *testing.T) {
		member := &Member{
			ID:            "member-1",
			Name:          "test-member",
			Address:       "192.168.1.100",
			GRPCAddress:   "192.168.1.100:9090",
			Status:        MemberStatusHealthy,
			IsLeader:      true,
			Version:       "0.11.0",
			JoinedAt:      time.Now().UTC(),
			LastHeartbeat: time.Now().UTC(),
			Metadata:      map[string]string{"env": "prod", "dc": "us-east-1"},
			AgentCount:    100,
			JobCount:      50,
		}

		clone := member.Clone()

		assert.Equal(t, member.ID, clone.ID)
		assert.Equal(t, member.Name, clone.Name)
		assert.Equal(t, member.Address, clone.Address)
		assert.Equal(t, member.GRPCAddress, clone.GRPCAddress)
		assert.Equal(t, member.Status, clone.Status)
		assert.Equal(t, member.IsLeader, clone.IsLeader)
		assert.Equal(t, member.Version, clone.Version)
		assert.Equal(t, member.JoinedAt, clone.JoinedAt)
		assert.Equal(t, member.LastHeartbeat, clone.LastHeartbeat)
		assert.Equal(t, member.Metadata, clone.Metadata)
		assert.Equal(t, member.AgentCount, clone.AgentCount)
		assert.Equal(t, member.JobCount, clone.JobCount)

		// Ensure metadata is a deep copy
		clone.Metadata["env"] = "staging"
		assert.Equal(t, "prod", member.Metadata["env"])
	})

	t.Run("clone without metadata", func(t *testing.T) {
		member := &Member{
			ID:      "member-1",
			Name:    "test-member",
			Address: "192.168.1.100",
		}

		clone := member.Clone()
		assert.Equal(t, member.ID, clone.ID)
		assert.Nil(t, clone.Metadata)
	})

	t.Run("clone nil member", func(t *testing.T) {
		var member *Member
		clone := member.Clone()
		assert.Nil(t, clone)
	})
}

func TestLeadershipEventType(t *testing.T) {
	eventTypes := []LeadershipEventType{
		LeadershipEventElected,
		LeadershipEventResigned,
		LeadershipEventLost,
		LeadershipEventTransferred,
	}

	for _, et := range eventTypes {
		assert.NotEmpty(t, string(et))
	}
}

func TestMembershipEventType(t *testing.T) {
	eventTypes := []MembershipEventType{
		MembershipEventJoined,
		MembershipEventLeft,
		MembershipEventFailed,
		MembershipEventRecovered,
		MembershipEventUpdated,
	}

	for _, et := range eventTypes {
		assert.NotEmpty(t, string(et))
	}
}

func TestLeadershipEvent(t *testing.T) {
	event := LeadershipEvent{
		Type:             LeadershipEventElected,
		LeaderID:         "member-1",
		PreviousLeaderID: "member-2",
		Timestamp:        time.Now().UTC(),
		Reason:           "election completed",
	}

	assert.Equal(t, LeadershipEventElected, event.Type)
	assert.Equal(t, "member-1", event.LeaderID)
	assert.Equal(t, "member-2", event.PreviousLeaderID)
	assert.Equal(t, "election completed", event.Reason)
}

func TestMembershipEvent(t *testing.T) {
	member := &Member{
		ID:     "member-1",
		Name:   "test-member",
		Status: MemberStatusHealthy,
	}

	event := MembershipEvent{
		Type:      MembershipEventJoined,
		Member:    member,
		Timestamp: time.Now().UTC(),
		Reason:    "new member",
	}

	assert.Equal(t, MembershipEventJoined, event.Type)
	assert.Equal(t, "member-1", event.Member.ID)
	assert.Equal(t, "new member", event.Reason)
}

func TestClusterInfo(t *testing.T) {
	members := []*Member{
		{ID: "m1", Status: MemberStatusHealthy, IsLeader: true},
		{ID: "m2", Status: MemberStatusHealthy},
		{ID: "m3", Status: MemberStatusDegraded},
	}

	info := &ClusterInfo{
		Name:         "test-cluster",
		Status:       ClusterStatusHealthy,
		LeaderID:     "m1",
		Members:      members,
		MemberCount:  3,
		HealthyCount: 3,
		QuorumSize:   2,
		HasQuorum:    true,
		CreatedAt:    time.Now().UTC(),
		UpdatedAt:    time.Now().UTC(),
	}

	assert.Equal(t, "test-cluster", info.Name)
	assert.Equal(t, ClusterStatusHealthy, info.Status)
	assert.Equal(t, "m1", info.LeaderID)
	assert.Len(t, info.Members, 3)
	assert.Equal(t, 3, info.MemberCount)
	assert.Equal(t, 3, info.HealthyCount)
	assert.Equal(t, 2, info.QuorumSize)
	assert.True(t, info.HasQuorum)
}

func TestShardAssignment(t *testing.T) {
	assignment := &ShardAssignment{
		AgentID:    "agent-1",
		MemberID:   "member-1",
		AssignedAt: time.Now().UTC(),
		Version:    1,
	}

	assert.Equal(t, "agent-1", assignment.AgentID)
	assert.Equal(t, "member-1", assignment.MemberID)
	assert.Equal(t, int64(1), assignment.Version)
}

func TestRebalanceEvent(t *testing.T) {
	startTime := time.Now().UTC()
	endTime := startTime.Add(5 * time.Second)

	event := &RebalanceEvent{
		TriggerMemberID: "member-1",
		Reason:          "member left",
		MovedAgents:     50,
		StartTime:       startTime,
		EndTime:         endTime,
		Duration:        5 * time.Second,
	}

	assert.Equal(t, "member-1", event.TriggerMemberID)
	assert.Equal(t, "member left", event.Reason)
	assert.Equal(t, 50, event.MovedAgents)
	assert.Equal(t, 5*time.Second, event.Duration)
}

func TestErrors(t *testing.T) {
	errors := []error{
		ErrNotLeader,
		ErrNoQuorum,
		ErrMemberNotFound,
		ErrMemberExists,
		ErrClusterNotReady,
		ErrLeaderElectionFailed,
		ErrEtcdNotConnected,
		ErrShutdown,
	}

	for _, err := range errors {
		assert.Error(t, err)
		assert.NotEmpty(t, err.Error())
	}
}
