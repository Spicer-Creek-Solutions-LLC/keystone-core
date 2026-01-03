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

func TestMemberStatus_String(t *testing.T) {
	tests := []struct {
		status   MemberStatus
		expected string
	}{
		{MemberStatusHealthy, "healthy"},
		{MemberStatusDegraded, "degraded"},
		{MemberStatusUnhealthy, "unhealthy"},
		{MemberStatusUnknown, "unknown"},
		{MemberStatusLeaving, "leaving"},
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			assert.Equal(t, tt.expected, tt.status.String())
		})
	}
}

func TestMemberStatus_IsHealthy(t *testing.T) {
	tests := []struct {
		status    MemberStatus
		isHealthy bool
	}{
		{MemberStatusHealthy, true},
		{MemberStatusDegraded, true},
		{MemberStatusUnhealthy, false},
		{MemberStatusUnknown, false},
		{MemberStatusLeaving, false},
	}

	for _, tt := range tests {
		t.Run(string(tt.status), func(t *testing.T) {
			assert.Equal(t, tt.isHealthy, tt.status.IsHealthy())
		})
	}
}

func TestClusterStatus_String(t *testing.T) {
	tests := []struct {
		status   ClusterStatus
		expected string
	}{
		{ClusterStatusHealthy, "healthy"},
		{ClusterStatusDegraded, "degraded"},
		{ClusterStatusUnhealthy, "unhealthy"},
		{ClusterStatusForming, "forming"},
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			assert.Equal(t, tt.expected, tt.status.String())
		})
	}
}

func TestEtcdMode_String(t *testing.T) {
	tests := []struct {
		mode     EtcdMode
		expected string
	}{
		{EtcdModeEmbedded, "embedded"},
		{EtcdModeExternal, "external"},
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			assert.Equal(t, tt.expected, tt.mode.String())
		})
	}
}

func TestMember_AllFields(t *testing.T) {
	now := time.Now().UTC()
	member := &Member{
		ID:            "member-1",
		Name:          "test-member",
		Address:       "192.168.1.100:8080",
		GRPCAddress:   "192.168.1.100:9090",
		NATSAddress:   "192.168.1.100:4222",
		Status:        MemberStatusHealthy,
		IsLeader:      true,
		Version:       "1.0.0",
		JoinedAt:      now,
		LastHeartbeat: now,
		LeaseID:       12345,
		Metadata:      map[string]string{"env": "prod", "region": "us-east"},
		AgentCount:    100,
		JobCount:      50,
	}

	assert.Equal(t, "member-1", member.ID)
	assert.Equal(t, "test-member", member.Name)
	assert.Equal(t, "192.168.1.100:8080", member.Address)
	assert.Equal(t, "192.168.1.100:9090", member.GRPCAddress)
	assert.Equal(t, "192.168.1.100:4222", member.NATSAddress)
	assert.Equal(t, MemberStatusHealthy, member.Status)
	assert.True(t, member.IsLeader)
	assert.Equal(t, "1.0.0", member.Version)
	assert.Equal(t, now, member.JoinedAt)
	assert.Equal(t, now, member.LastHeartbeat)
	assert.Equal(t, int64(12345), member.LeaseID)
	assert.Equal(t, "prod", member.Metadata["env"])
	assert.Equal(t, "us-east", member.Metadata["region"])
	assert.Equal(t, 100, member.AgentCount)
	assert.Equal(t, 50, member.JobCount)
}

func TestMemberClone_EmptyMetadata(t *testing.T) {
	member := &Member{
		ID:       "member-1",
		Name:     "test",
		Metadata: map[string]string{},
	}

	clone := member.Clone()
	assert.NotNil(t, clone.Metadata)
	assert.Len(t, clone.Metadata, 0)
}

func TestClusterInfo_AllFields(t *testing.T) {
	now := time.Now().UTC()
	members := []*Member{
		{ID: "m1", Status: MemberStatusHealthy, IsLeader: true},
		{ID: "m2", Status: MemberStatusHealthy},
		{ID: "m3", Status: MemberStatusDegraded},
		{ID: "m4", Status: MemberStatusUnhealthy},
	}

	info := &ClusterInfo{
		Name:         "production-cluster",
		Status:       ClusterStatusDegraded,
		LeaderID:     "m1",
		Members:      members,
		MemberCount:  4,
		HealthyCount: 3,
		QuorumSize:   3,
		HasQuorum:    true,
		CreatedAt:    now,
		UpdatedAt:    now,
	}

	assert.Equal(t, "production-cluster", info.Name)
	assert.Equal(t, ClusterStatusDegraded, info.Status)
	assert.Equal(t, "m1", info.LeaderID)
	assert.Len(t, info.Members, 4)
	assert.Equal(t, 4, info.MemberCount)
	assert.Equal(t, 3, info.HealthyCount)
	assert.Equal(t, 3, info.QuorumSize)
	assert.True(t, info.HasQuorum)
	assert.Equal(t, now, info.CreatedAt)
	assert.Equal(t, now, info.UpdatedAt)
}

func TestLeadershipEvent_AllTypes(t *testing.T) {
	now := time.Now().UTC()

	tests := []struct {
		eventType LeadershipEventType
		leaderID  string
		prevID    string
		reason    string
	}{
		{LeadershipEventElected, "m1", "", "initial election"},
		{LeadershipEventResigned, "", "m1", "graceful shutdown"},
		{LeadershipEventLost, "", "m1", "network partition"},
		{LeadershipEventTransferred, "m2", "m1", "manual transfer"},
	}

	for _, tt := range tests {
		t.Run(string(tt.eventType), func(t *testing.T) {
			event := LeadershipEvent{
				Type:             tt.eventType,
				LeaderID:         tt.leaderID,
				PreviousLeaderID: tt.prevID,
				Timestamp:        now,
				Reason:           tt.reason,
			}

			assert.Equal(t, tt.eventType, event.Type)
			assert.Equal(t, tt.leaderID, event.LeaderID)
			assert.Equal(t, tt.prevID, event.PreviousLeaderID)
			assert.Equal(t, now, event.Timestamp)
			assert.Equal(t, tt.reason, event.Reason)
		})
	}
}

func TestMembershipEvent_AllTypes(t *testing.T) {
	now := time.Now().UTC()
	member := &Member{
		ID:     "m1",
		Name:   "test-member",
		Status: MemberStatusHealthy,
	}

	tests := []struct {
		eventType MembershipEventType
		reason    string
	}{
		{MembershipEventJoined, "new member"},
		{MembershipEventLeft, "graceful shutdown"},
		{MembershipEventFailed, "heartbeat timeout"},
		{MembershipEventRecovered, "connection restored"},
		{MembershipEventUpdated, "metadata updated"},
	}

	for _, tt := range tests {
		t.Run(string(tt.eventType), func(t *testing.T) {
			event := MembershipEvent{
				Type:      tt.eventType,
				Member:    member,
				Timestamp: now,
				Reason:    tt.reason,
			}

			assert.Equal(t, tt.eventType, event.Type)
			assert.Equal(t, member, event.Member)
			assert.Equal(t, now, event.Timestamp)
			assert.Equal(t, tt.reason, event.Reason)
		})
	}
}

func TestShardAssignment_AllFields(t *testing.T) {
	now := time.Now().UTC()

	assignment := &ShardAssignment{
		AgentID:    "agent-123",
		MemberID:   "member-456",
		AssignedAt: now,
		Version:    5,
	}

	assert.Equal(t, "agent-123", assignment.AgentID)
	assert.Equal(t, "member-456", assignment.MemberID)
	assert.Equal(t, now, assignment.AssignedAt)
	assert.Equal(t, int64(5), assignment.Version)
}

func TestRebalanceEvent_AllFields(t *testing.T) {
	startTime := time.Now().UTC()
	endTime := startTime.Add(10 * time.Second)

	event := &RebalanceEvent{
		TriggerMemberID: "member-1",
		Reason:          "member failure",
		MovedAgents:     150,
		StartTime:       startTime,
		EndTime:         endTime,
		Duration:        10 * time.Second,
	}

	assert.Equal(t, "member-1", event.TriggerMemberID)
	assert.Equal(t, "member failure", event.Reason)
	assert.Equal(t, 150, event.MovedAgents)
	assert.Equal(t, startTime, event.StartTime)
	assert.Equal(t, endTime, event.EndTime)
	assert.Equal(t, 10*time.Second, event.Duration)
}

func TestLeadershipEventType_Values(t *testing.T) {
	// Ensure all values are distinct
	types := []LeadershipEventType{
		LeadershipEventElected,
		LeadershipEventResigned,
		LeadershipEventLost,
		LeadershipEventTransferred,
	}

	seen := make(map[LeadershipEventType]bool)
	for _, typ := range types {
		assert.NotEmpty(t, string(typ))
		if seen[typ] {
			t.Errorf("duplicate LeadershipEventType: %s", typ)
		}
		seen[typ] = true
	}
}

func TestMembershipEventType_Values(t *testing.T) {
	// Ensure all values are distinct
	types := []MembershipEventType{
		MembershipEventJoined,
		MembershipEventLeft,
		MembershipEventFailed,
		MembershipEventRecovered,
		MembershipEventUpdated,
	}

	seen := make(map[MembershipEventType]bool)
	for _, typ := range types {
		assert.NotEmpty(t, string(typ))
		if seen[typ] {
			t.Errorf("duplicate MembershipEventType: %s", typ)
		}
		seen[typ] = true
	}
}

func TestClusterInfo_NoQuorum(t *testing.T) {
	info := &ClusterInfo{
		Name:         "small-cluster",
		Status:       ClusterStatusUnhealthy,
		LeaderID:     "",
		Members:      []*Member{{ID: "m1", Status: MemberStatusUnhealthy}},
		MemberCount:  3,
		HealthyCount: 1,
		QuorumSize:   2,
		HasQuorum:    false,
	}

	assert.False(t, info.HasQuorum)
	assert.Empty(t, info.LeaderID)
	assert.Equal(t, ClusterStatusUnhealthy, info.Status)
}

func TestMemberClone_ModifyOriginal(t *testing.T) {
	original := &Member{
		ID:       "member-1",
		Name:     "original",
		Metadata: map[string]string{"key": "value"},
	}

	clone := original.Clone()

	// Modify original
	original.Name = "modified"
	original.Metadata["key"] = "modified"

	// Clone should be unchanged
	assert.Equal(t, "original", clone.Name)
	assert.Equal(t, "value", clone.Metadata["key"])
}

func TestMemberStatusConstants(t *testing.T) {
	// Verify the constants have expected values
	assert.Equal(t, MemberStatus("healthy"), MemberStatusHealthy)
	assert.Equal(t, MemberStatus("degraded"), MemberStatusDegraded)
	assert.Equal(t, MemberStatus("unhealthy"), MemberStatusUnhealthy)
	assert.Equal(t, MemberStatus("unknown"), MemberStatusUnknown)
	assert.Equal(t, MemberStatus("leaving"), MemberStatusLeaving)
}

func TestClusterStatusConstants(t *testing.T) {
	// Verify the constants have expected values
	assert.Equal(t, ClusterStatus("healthy"), ClusterStatusHealthy)
	assert.Equal(t, ClusterStatus("degraded"), ClusterStatusDegraded)
	assert.Equal(t, ClusterStatus("unhealthy"), ClusterStatusUnhealthy)
	assert.Equal(t, ClusterStatus("forming"), ClusterStatusForming)
}

func TestEtcdModeConstants(t *testing.T) {
	// Verify the constants have expected values
	assert.Equal(t, EtcdMode("embedded"), EtcdModeEmbedded)
	assert.Equal(t, EtcdMode("external"), EtcdModeExternal)
}

func TestErrorMessages(t *testing.T) {
	// Verify error messages contain descriptive text
	assert.Contains(t, ErrNotLeader.Error(), "not the leader")
	assert.Contains(t, ErrNoQuorum.Error(), "no quorum")
	assert.Contains(t, ErrMemberNotFound.Error(), "not found")
	assert.Contains(t, ErrMemberExists.Error(), "already exists")
	assert.Contains(t, ErrClusterNotReady.Error(), "not ready")
	assert.Contains(t, ErrLeaderElectionFailed.Error(), "leader election")
	assert.Contains(t, ErrEtcdNotConnected.Error(), "not connected")
	assert.Contains(t, ErrShutdown.Error(), "shutting down")
}
