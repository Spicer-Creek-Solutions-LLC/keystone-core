package cluster

import (
	"testing"
	"time"
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
		AgentID:  "agent-1",
		MemberID: "member-1",
		Version:  5,
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

func TestConsistentHash_EmptyMemberID(t *testing.T) {
	ch := NewConsistentHash(10)

	// Add empty member ID (edge case)
	ch.AddMember("")
	if ch.MemberCount() != 1 {
		t.Errorf("MemberCount() = %v, want 1", ch.MemberCount())
	}

	// Remove empty member ID
	ch.RemoveMember("")
	if ch.MemberCount() != 0 {
		t.Errorf("MemberCount() after remove = %v, want 0", ch.MemberCount())
	}
}

func TestConsistentHash_SingleMember(t *testing.T) {
	ch := NewConsistentHash(50)

	ch.AddMember("member-1")

	// All keys should go to the single member
	for i := 0; i < 100; i++ {
		member := ch.GetMember(string(rune(i)))
		if member != "member-1" {
			t.Errorf("GetMember() = %v, want 'member-1'", member)
		}
	}

	// GetMembers should return single member
	members := ch.GetMembers()
	if len(members) != 1 || members[0] != "member-1" {
		t.Errorf("GetMembers() = %v, want ['member-1']", members)
	}
}

func TestConsistentHash_ManyMembers(t *testing.T) {
	ch := NewConsistentHash(100)

	// Add many members
	for i := 0; i < 20; i++ {
		ch.AddMember(string(rune('a' + i)))
	}

	if ch.MemberCount() != 20 {
		t.Errorf("MemberCount() = %v, want 20", ch.MemberCount())
	}

	// Verify ring size
	expectedRingSize := 20 * 100
	if len(ch.ring) != expectedRingSize {
		t.Errorf("ring size = %v, want %v", len(ch.ring), expectedRingSize)
	}

	// Remove some members
	for i := 0; i < 10; i++ {
		ch.RemoveMember(string(rune('a' + i)))
	}

	if ch.MemberCount() != 10 {
		t.Errorf("MemberCount() after removes = %v, want 10", ch.MemberCount())
	}
}

func TestConsistentHash_RingOrder(t *testing.T) {
	ch := NewConsistentHash(10)

	ch.AddMember("member-1")
	ch.AddMember("member-2")

	// Ring should be sorted
	for i := 1; i < len(ch.ring); i++ {
		if ch.ring[i] < ch.ring[i-1] {
			t.Errorf("Ring not sorted at index %d: %v < %v", i, ch.ring[i], ch.ring[i-1])
		}
	}
}

func TestConsistentHash_GetAffectedKeys_NoChange(t *testing.T) {
	ch := NewConsistentHash(100)

	ch.AddMember("member-1")
	ch.AddMember("member-2")
	ch.AddMember("member-3")

	keys := []string{"key-1", "key-2", "key-3"}

	// No change if adding empty and removing empty
	affected := ch.GetAffectedKeys(keys, "", "")
	if len(affected) != 0 {
		t.Errorf("Expected no affected keys for no-op, got %d", len(affected))
	}
}

func TestConsistentHash_GetAffectedKeys_RemoveAll(t *testing.T) {
	ch := NewConsistentHash(100)

	ch.AddMember("member-1")

	keys := []string{"key-1", "key-2"}

	// Removing only member affects all keys
	affected := ch.GetAffectedKeys(keys, "", "member-1")
	if len(affected) != len(keys) {
		t.Errorf("Expected %d affected keys, got %d", len(keys), len(affected))
	}
}

func TestShardChangeEvent_Timestamp(t *testing.T) {
	now := time.Now()
	event := ShardChangeEvent{
		AgentID:     "agent-1",
		OldMemberID: "member-1",
		NewMemberID: "member-2",
		Reason:      "failover",
		Timestamp:   now,
	}

	if event.Timestamp.IsZero() {
		t.Error("Timestamp should not be zero")
	}

	if event.Timestamp != now {
		t.Errorf("Timestamp = %v, want %v", event.Timestamp, now)
	}
}

func TestRebalanceEvent_Fields(t *testing.T) {
	start := time.Now()
	end := start.Add(100 * time.Millisecond)

	event := RebalanceEvent{
		TriggerMemberID: "member-1",
		Reason:          "member joined",
		MovedAgents:     15,
		StartTime:       start,
		EndTime:         end,
		Duration:        end.Sub(start),
	}

	if event.TriggerMemberID != "member-1" {
		t.Errorf("TriggerMemberID = %v, want 'member-1'", event.TriggerMemberID)
	}

	if event.Reason != "member joined" {
		t.Errorf("Reason = %v, want 'member joined'", event.Reason)
	}

	if event.MovedAgents != 15 {
		t.Errorf("MovedAgents = %v, want 15", event.MovedAgents)
	}

	if event.Duration != 100*time.Millisecond {
		t.Errorf("Duration = %v, want 100ms", event.Duration)
	}
}

func TestShardingStrategy_Unique(t *testing.T) {
	strategies := []ShardingStrategy{
		ShardingStrategyConsistentHash,
		ShardingStrategyRoundRobin,
		ShardingStrategyLeastConnections,
	}

	seen := make(map[ShardingStrategy]bool)
	for _, s := range strategies {
		if seen[s] {
			t.Errorf("Duplicate strategy: %v", s)
		}
		seen[s] = true
	}
}

func TestNewShardManager_Validation(t *testing.T) {
	config := &Config{ClusterName: "test", HeartbeatInterval: time.Second}
	etcd := &EtcdClient{}
	mm := &MembershipManager{
		config:   config,
		etcd:     etcd,
		members:  make(map[string]*Member),
		stopChan: make(chan struct{}),
		doneChan: make(chan struct{}),
	}
	ss, _ := NewShardStore(etcd)

	tests := []struct {
		name       string
		config     *Config
		membership *MembershipManager
		shardStore *ShardStore
		wantErr    bool
	}{
		{
			name:       "nil config",
			config:     nil,
			membership: mm,
			shardStore: ss,
			wantErr:    true,
		},
		{
			name:       "nil membership",
			config:     config,
			membership: nil,
			shardStore: ss,
			wantErr:    true,
		},
		{
			name:       "nil shard store",
			config:     config,
			membership: mm,
			shardStore: nil,
			wantErr:    true,
		},
		{
			name:       "valid",
			config:     config,
			membership: mm,
			shardStore: ss,
			wantErr:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sm, err := NewShardManager(tt.config, tt.membership, tt.shardStore)
			if (err != nil) != tt.wantErr {
				t.Errorf("NewShardManager() error = %v, wantErr %v", err, tt.wantErr)
			}
			if !tt.wantErr && sm == nil {
				t.Error("NewShardManager() returned nil")
			}
		})
	}
}

func TestShardManager_SetStrategy(t *testing.T) {
	config := &Config{ClusterName: "test", HeartbeatInterval: time.Second}
	etcd := &EtcdClient{}
	mm := &MembershipManager{
		config:   config,
		etcd:     etcd,
		members:  make(map[string]*Member),
		stopChan: make(chan struct{}),
		doneChan: make(chan struct{}),
	}
	ss, _ := NewShardStore(etcd)

	sm, _ := NewShardManager(config, mm, ss)

	// Default strategy
	if sm.strategy != ShardingStrategyConsistentHash {
		t.Errorf("Default strategy = %v, want %v", sm.strategy, ShardingStrategyConsistentHash)
	}

	// Set different strategy
	sm.SetStrategy(ShardingStrategyRoundRobin)
	if sm.strategy != ShardingStrategyRoundRobin {
		t.Errorf("Strategy after set = %v, want %v", sm.strategy, ShardingStrategyRoundRobin)
	}

	sm.SetStrategy(ShardingStrategyLeastConnections)
	if sm.strategy != ShardingStrategyLeastConnections {
		t.Errorf("Strategy after second set = %v, want %v", sm.strategy, ShardingStrategyLeastConnections)
	}
}

func TestShardManager_GetAssignment_NotFound(t *testing.T) {
	config := &Config{ClusterName: "test", HeartbeatInterval: time.Second}
	etcd := &EtcdClient{}
	mm := &MembershipManager{
		config:   config,
		etcd:     etcd,
		members:  make(map[string]*Member),
		stopChan: make(chan struct{}),
		doneChan: make(chan struct{}),
	}
	ss, _ := NewShardStore(etcd)

	sm, _ := NewShardManager(config, mm, ss)

	// Non-existent assignment
	member, exists := sm.GetAssignment("agent-1")
	if exists {
		t.Error("GetAssignment() should return false for non-existent agent")
	}
	if member != "" {
		t.Errorf("GetAssignment() member = %v, want empty", member)
	}
}

func TestShardManager_GetAllAssignments_Empty(t *testing.T) {
	config := &Config{ClusterName: "test", HeartbeatInterval: time.Second}
	etcd := &EtcdClient{}
	mm := &MembershipManager{
		config:   config,
		etcd:     etcd,
		members:  make(map[string]*Member),
		stopChan: make(chan struct{}),
		doneChan: make(chan struct{}),
	}
	ss, _ := NewShardStore(etcd)

	sm, _ := NewShardManager(config, mm, ss)

	assignments := sm.GetAllAssignments()
	if len(assignments) != 0 {
		t.Errorf("GetAllAssignments() = %d, want 0", len(assignments))
	}
}

func TestShardManager_GetAgentCountForMember_NotFound(t *testing.T) {
	config := &Config{ClusterName: "test", HeartbeatInterval: time.Second}
	etcd := &EtcdClient{}
	mm := &MembershipManager{
		config:   config,
		etcd:     etcd,
		members:  make(map[string]*Member),
		stopChan: make(chan struct{}),
		doneChan: make(chan struct{}),
	}
	ss, _ := NewShardStore(etcd)

	sm, _ := NewShardManager(config, mm, ss)

	count := sm.GetAgentCountForMember("member-1")
	if count != 0 {
		t.Errorf("GetAgentCountForMember() = %d, want 0", count)
	}
}

func TestShardManager_GetAssignmentsForMember_Empty(t *testing.T) {
	config := &Config{ClusterName: "test", HeartbeatInterval: time.Second}
	etcd := &EtcdClient{}
	mm := &MembershipManager{
		config:   config,
		etcd:     etcd,
		members:  make(map[string]*Member),
		stopChan: make(chan struct{}),
		doneChan: make(chan struct{}),
	}
	ss, _ := NewShardStore(etcd)

	sm, _ := NewShardManager(config, mm, ss)

	agents := sm.GetAssignmentsForMember("member-1")
	if len(agents) != 0 {
		t.Errorf("GetAssignmentsForMember() = %d, want 0", len(agents))
	}
}

func TestShardManager_AddObserver(t *testing.T) {
	config := &Config{ClusterName: "test", HeartbeatInterval: time.Second}
	etcd := &EtcdClient{}
	mm := &MembershipManager{
		config:   config,
		etcd:     etcd,
		members:  make(map[string]*Member),
		stopChan: make(chan struct{}),
		doneChan: make(chan struct{}),
	}
	ss, _ := NewShardStore(etcd)

	sm, _ := NewShardManager(config, mm, ss)

	observerCalled := false
	sm.AddObserver(func(event ShardChangeEvent) {
		observerCalled = true
	})

	if len(sm.observers) != 1 {
		t.Errorf("observers count = %d, want 1", len(sm.observers))
	}

	// Suppress warning
	_ = observerCalled
}

func TestShardManager_GetAgentCountsPerMember_Empty(t *testing.T) {
	config := &Config{ClusterName: "test", HeartbeatInterval: time.Second}
	etcd := &EtcdClient{}
	mm := &MembershipManager{
		config:   config,
		etcd:     etcd,
		members:  make(map[string]*Member),
		stopChan: make(chan struct{}),
		doneChan: make(chan struct{}),
	}
	ss, _ := NewShardStore(etcd)

	sm, _ := NewShardManager(config, mm, ss)

	counts := sm.GetAgentCountsPerMember()
	if len(counts) != 0 {
		t.Errorf("GetAgentCountsPerMember() = %d, want 0", len(counts))
	}
}

func TestShardManager_ManualAssignments(t *testing.T) {
	config := &Config{ClusterName: "test", HeartbeatInterval: time.Second}
	etcd := &EtcdClient{}
	mm := &MembershipManager{
		config:   config,
		etcd:     etcd,
		members:  make(map[string]*Member),
		stopChan: make(chan struct{}),
		doneChan: make(chan struct{}),
	}
	ss, _ := NewShardStore(etcd)

	sm, _ := NewShardManager(config, mm, ss)

	// Manually add assignments
	sm.mu.Lock()
	sm.assignments["agent-1"] = "member-1"
	sm.assignments["agent-2"] = "member-1"
	sm.assignments["agent-3"] = "member-2"
	sm.agentCounts["member-1"] = 2
	sm.agentCounts["member-2"] = 1
	sm.mu.Unlock()

	// Test GetAssignment
	member, exists := sm.GetAssignment("agent-1")
	if !exists || member != "member-1" {
		t.Errorf("GetAssignment(agent-1) = %v, %v, want member-1, true", member, exists)
	}

	// Test GetAllAssignments
	assignments := sm.GetAllAssignments()
	if len(assignments) != 3 {
		t.Errorf("GetAllAssignments() = %d, want 3", len(assignments))
	}

	// Test GetAgentCountForMember
	count := sm.GetAgentCountForMember("member-1")
	if count != 2 {
		t.Errorf("GetAgentCountForMember(member-1) = %d, want 2", count)
	}

	// Test GetAgentCountsPerMember
	counts := sm.GetAgentCountsPerMember()
	if counts["member-1"] != 2 || counts["member-2"] != 1 {
		t.Errorf("GetAgentCountsPerMember() = %v", counts)
	}

	// Test GetAssignmentsForMember
	agents := sm.GetAssignmentsForMember("member-1")
	if len(agents) != 2 {
		t.Errorf("GetAssignmentsForMember(member-1) = %d, want 2", len(agents))
	}
}
