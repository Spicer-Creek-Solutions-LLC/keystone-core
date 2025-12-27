package cluster

import (
	"testing"
)

func TestConsistentHash_NewConsistentHash(t *testing.T) {
	tests := []struct {
		name         string
		virtualNodes int
		wantNodes    int
	}{
		{"default virtual nodes", 0, DefaultVirtualNodes},
		{"negative virtual nodes", -1, DefaultVirtualNodes},
		{"custom virtual nodes", 100, 100},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ch := NewConsistentHash(tt.virtualNodes)
			if ch == nil {
				t.Fatal("NewConsistentHash() returned nil")
			}
			if ch.virtualNodes != tt.wantNodes {
				t.Errorf("virtualNodes = %v, want %v", ch.virtualNodes, tt.wantNodes)
			}
		})
	}
}

func TestConsistentHash_AddRemoveMember(t *testing.T) {
	ch := NewConsistentHash(10)

	// Add members
	ch.AddMember("member-1")
	ch.AddMember("member-2")
	ch.AddMember("member-3")

	if ch.MemberCount() != 3 {
		t.Errorf("MemberCount() = %v, want 3", ch.MemberCount())
	}

	// Verify ring size (3 members * 10 virtual nodes = 30)
	if len(ch.ring) != 30 {
		t.Errorf("ring size = %v, want 30", len(ch.ring))
	}

	// Adding same member again should be no-op
	ch.AddMember("member-1")
	if ch.MemberCount() != 3 {
		t.Errorf("MemberCount() after duplicate add = %v, want 3", ch.MemberCount())
	}

	// Remove a member
	ch.RemoveMember("member-2")
	if ch.MemberCount() != 2 {
		t.Errorf("MemberCount() after remove = %v, want 2", ch.MemberCount())
	}

	// Removing non-existent member should be no-op
	ch.RemoveMember("member-99")
	if ch.MemberCount() != 2 {
		t.Errorf("MemberCount() after removing non-existent = %v, want 2", ch.MemberCount())
	}
}

func TestConsistentHash_GetMember(t *testing.T) {
	ch := NewConsistentHash(100)

	// Empty ring
	if member := ch.GetMember("key-1"); member != "" {
		t.Errorf("GetMember() on empty ring = %v, want empty", member)
	}

	// Add members
	ch.AddMember("member-1")
	ch.AddMember("member-2")
	ch.AddMember("member-3")

	// All keys should map to some member
	keys := []string{"agent-1", "agent-2", "agent-3", "agent-100", "agent-abc"}
	for _, key := range keys {
		member := ch.GetMember(key)
		if member == "" {
			t.Errorf("GetMember(%v) returned empty", key)
		}
	}

	// Same key should always return same member
	member1 := ch.GetMember("consistent-key")
	member2 := ch.GetMember("consistent-key")
	if member1 != member2 {
		t.Errorf("GetMember() not consistent: %v != %v", member1, member2)
	}
}

func TestConsistentHash_Distribution(t *testing.T) {
	ch := NewConsistentHash(150)

	ch.AddMember("member-1")
	ch.AddMember("member-2")
	ch.AddMember("member-3")

	// Test distribution across 1000 keys
	counts := make(map[string]int)
	for i := 0; i < 1000; i++ {
		member := ch.GetMember(string(rune(i)))
		counts[member]++
	}

	// Each member should get a reasonable share (not exact due to hashing)
	// With 3 members, each should get roughly 333 keys, but allow wide variance
	for member, count := range counts {
		if count == 0 {
			t.Errorf("Member %v got 0 keys", member)
		}
	}

	// All 3 members should be in the distribution
	if len(counts) != 3 {
		t.Errorf("Distribution covers %v members, want 3", len(counts))
	}
}

func TestConsistentHash_GetMembers(t *testing.T) {
	ch := NewConsistentHash(10)

	members := ch.GetMembers()
	if len(members) != 0 {
		t.Errorf("GetMembers() on empty ring = %v, want empty", len(members))
	}

	ch.AddMember("member-1")
	ch.AddMember("member-2")

	members = ch.GetMembers()
	if len(members) != 2 {
		t.Errorf("GetMembers() = %v, want 2", len(members))
	}

	// Verify members are in the list
	memberSet := make(map[string]bool)
	for _, m := range members {
		memberSet[m] = true
	}

	if !memberSet["member-1"] || !memberSet["member-2"] {
		t.Error("GetMembers() missing expected members")
	}
}

func TestConsistentHash_MinimalReassignment(t *testing.T) {
	ch := NewConsistentHash(100)

	ch.AddMember("member-1")
	ch.AddMember("member-2")

	// Get initial assignments for 100 keys
	initialAssignments := make(map[string]string)
	keys := make([]string, 100)
	for i := 0; i < 100; i++ {
		key := string(rune('a' + i))
		keys[i] = key
		initialAssignments[key] = ch.GetMember(key)
	}

	// Add a third member
	ch.AddMember("member-3")

	// Count reassignments
	reassigned := 0
	for _, key := range keys {
		newMember := ch.GetMember(key)
		if newMember != initialAssignments[key] {
			reassigned++
		}
	}

	// With consistent hashing, roughly 1/3 of keys should be reassigned
	// Allow some variance
	expectedMax := 50 // Should be less than half
	if reassigned > expectedMax {
		t.Errorf("Too many reassignments: %v (expected < %v)", reassigned, expectedMax)
	}
}

func TestShardingStrategy_Constants(t *testing.T) {
	tests := []struct {
		strategy ShardingStrategy
		want     string
	}{
		{ShardingStrategyConsistentHash, "consistent_hash"},
		{ShardingStrategyRoundRobin, "round_robin"},
		{ShardingStrategyLeastConnections, "least_connections"},
	}

	for _, tt := range tests {
		t.Run(string(tt.strategy), func(t *testing.T) {
			if string(tt.strategy) != tt.want {
				t.Errorf("ShardingStrategy = %v, want %v", string(tt.strategy), tt.want)
			}
		})
	}
}

func TestShardChangeEvent_Fields(t *testing.T) {
	event := ShardChangeEvent{
		AgentID:     "agent-1",
		OldMemberID: "member-1",
		NewMemberID: "member-2",
		Reason:      "rebalance",
	}

	if event.AgentID != "agent-1" {
		t.Errorf("AgentID = %v, want 'agent-1'", event.AgentID)
	}
	if event.OldMemberID != "member-1" {
		t.Errorf("OldMemberID = %v, want 'member-1'", event.OldMemberID)
	}
	if event.NewMemberID != "member-2" {
		t.Errorf("NewMemberID = %v, want 'member-2'", event.NewMemberID)
	}
	if event.Reason != "rebalance" {
		t.Errorf("Reason = %v, want 'rebalance'", event.Reason)
	}
}

func TestShardAssignment_Clone(t *testing.T) {
	original := &ShardAssignment{
		AgentID:    "agent-1",
		MemberID:   "member-1",
		Version:    5,
	}

	// The ShardAssignment doesn't have a Clone method, but let's test the struct
	if original.AgentID != "agent-1" {
		t.Errorf("AgentID = %v, want 'agent-1'", original.AgentID)
	}
	if original.MemberID != "member-1" {
		t.Errorf("MemberID = %v, want 'member-1'", original.MemberID)
	}
	if original.Version != 5 {
		t.Errorf("Version = %v, want 5", original.Version)
	}
}

func TestConsistentHash_GetAffectedKeys(t *testing.T) {
	ch := NewConsistentHash(100)

	ch.AddMember("member-1")
	ch.AddMember("member-2")

	keys := []string{"key-1", "key-2", "key-3", "key-4", "key-5"}

	// Adding a member should affect some keys
	affected := ch.GetAffectedKeys(keys, "member-3", "")
	// Some keys should be affected (moved to new member)
	// Due to consistent hashing, not all keys will move
	t.Logf("Adding member-3 affects %d/%d keys", len(affected), len(keys))

	// Removing a member should affect that member's keys
	ch.AddMember("member-3")
	affected = ch.GetAffectedKeys(keys, "", "member-3")
	t.Logf("Removing member-3 affects %d/%d keys", len(affected), len(keys))
}

func TestConsistentHash_HashConsistency(t *testing.T) {
	ch1 := NewConsistentHash(100)
	ch2 := NewConsistentHash(100)

	// Add same members in same order
	ch1.AddMember("member-1")
	ch1.AddMember("member-2")
	ch2.AddMember("member-1")
	ch2.AddMember("member-2")

	// Same keys should hash to same members
	keys := []string{"test-key-1", "test-key-2", "agent-abc", "job-xyz"}
	for _, key := range keys {
		member1 := ch1.GetMember(key)
		member2 := ch2.GetMember(key)
		if member1 != member2 {
			t.Errorf("Hash inconsistency for key %v: %v != %v", key, member1, member2)
		}
	}
}

func TestRebalanceMinInterval(t *testing.T) {
	if RebalanceMinInterval != 5*1e9 { // 5 seconds in nanoseconds
		t.Errorf("RebalanceMinInterval = %v, want 5s", RebalanceMinInterval)
	}
}

func TestDefaultVirtualNodes(t *testing.T) {
	if DefaultVirtualNodes != 150 {
		t.Errorf("DefaultVirtualNodes = %v, want 150", DefaultVirtualNodes)
	}
}
