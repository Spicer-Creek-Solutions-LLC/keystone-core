package cluster

import (
	"context"
	"sync"
	"testing"
	"time"
)

// PartitionSimulator simulates network partitions for testing
type PartitionSimulator struct {
	// partitions maps member IDs to the set of members they can communicate with
	partitions map[string]map[string]bool
	// delays maps (source, target) pairs to artificial delays
	delays map[string]map[string]time.Duration
	mu     sync.RWMutex
}

// NewPartitionSimulator creates a new partition simulator
func NewPartitionSimulator() *PartitionSimulator {
	return &PartitionSimulator{
		partitions: make(map[string]map[string]bool),
		delays:     make(map[string]map[string]time.Duration),
	}
}

// AllowCommunication allows communication between two members
func (ps *PartitionSimulator) AllowCommunication(from, to string) {
	ps.mu.Lock()
	defer ps.mu.Unlock()

	if ps.partitions[from] == nil {
		ps.partitions[from] = make(map[string]bool)
	}
	ps.partitions[from][to] = true
}

// BlockCommunication blocks communication between two members
func (ps *PartitionSimulator) BlockCommunication(from, to string) {
	ps.mu.Lock()
	defer ps.mu.Unlock()

	if ps.partitions[from] == nil {
		ps.partitions[from] = make(map[string]bool)
	}
	ps.partitions[from][to] = false
}

// SetDelay sets an artificial delay for communication between two members
func (ps *PartitionSimulator) SetDelay(from, to string, delay time.Duration) {
	ps.mu.Lock()
	defer ps.mu.Unlock()

	if ps.delays[from] == nil {
		ps.delays[from] = make(map[string]time.Duration)
	}
	ps.delays[from][to] = delay
}

// CanCommunicate checks if two members can communicate
func (ps *PartitionSimulator) CanCommunicate(from, to string) bool {
	ps.mu.RLock()
	defer ps.mu.RUnlock()

	// If no explicit partition rules, allow by default
	if ps.partitions[from] == nil {
		return true
	}
	allowed, exists := ps.partitions[from][to]
	if !exists {
		return true
	}
	return allowed
}

// GetDelay returns the artificial delay for communication between two members
func (ps *PartitionSimulator) GetDelay(from, to string) time.Duration {
	ps.mu.RLock()
	defer ps.mu.RUnlock()

	if ps.delays[from] == nil {
		return 0
	}
	return ps.delays[from][to]
}

// CreateSymmetricPartition creates a symmetric network partition between two groups
// Members in group1 cannot communicate with members in group2 and vice versa
func (ps *PartitionSimulator) CreateSymmetricPartition(group1, group2 []string) {
	ps.mu.Lock()
	defer ps.mu.Unlock()

	// Block communication from group1 to group2
	for _, m1 := range group1 {
		if ps.partitions[m1] == nil {
			ps.partitions[m1] = make(map[string]bool)
		}
		for _, m2 := range group2 {
			ps.partitions[m1][m2] = false
		}
	}

	// Block communication from group2 to group1
	for _, m2 := range group2 {
		if ps.partitions[m2] == nil {
			ps.partitions[m2] = make(map[string]bool)
		}
		for _, m1 := range group1 {
			ps.partitions[m2][m1] = false
		}
	}
}

// HealPartition removes all partition rules
func (ps *PartitionSimulator) HealPartition() {
	ps.mu.Lock()
	defer ps.mu.Unlock()

	ps.partitions = make(map[string]map[string]bool)
	ps.delays = make(map[string]map[string]time.Duration)
}

// SimulatedMemberRegistry simulates a member registry with partition support
type SimulatedMemberRegistry struct {
	members   map[string]*Member
	simulator *PartitionSimulator
	localID   string
	observers []MembershipObserver
	mu        sync.RWMutex
}

// NewSimulatedMemberRegistry creates a new simulated registry
func NewSimulatedMemberRegistry(localID string, simulator *PartitionSimulator) *SimulatedMemberRegistry {
	return &SimulatedMemberRegistry{
		members:   make(map[string]*Member),
		simulator: simulator,
		localID:   localID,
		observers: make([]MembershipObserver, 0),
	}
}

// Register adds a member to the registry
func (r *SimulatedMemberRegistry) Register(ctx context.Context, member *Member) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.members[member.ID] = member.Clone()
	return nil
}

// Deregister removes a member from the registry
func (r *SimulatedMemberRegistry) Deregister(ctx context.Context) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.members, r.localID)
	return nil
}

// Heartbeat simulates sending a heartbeat
func (r *SimulatedMemberRegistry) Heartbeat(ctx context.Context) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if member, ok := r.members[r.localID]; ok {
		member.LastHeartbeat = time.Now()
	}
	return nil
}

// GetMember returns a member by ID
func (r *SimulatedMemberRegistry) GetMember(ctx context.Context, id string) (*Member, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	// Check if we can communicate with this member
	if !r.simulator.CanCommunicate(r.localID, id) {
		return nil, ErrMemberNotFound
	}

	// Apply delay if any
	if delay := r.simulator.GetDelay(r.localID, id); delay > 0 {
		select {
		case <-time.After(delay):
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}

	member, ok := r.members[id]
	if !ok {
		return nil, ErrMemberNotFound
	}
	return member.Clone(), nil
}

// ListMembers returns all visible members
func (r *SimulatedMemberRegistry) ListMembers(ctx context.Context) ([]*Member, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var members []*Member
	for id, member := range r.members {
		// Only include members we can communicate with
		if r.simulator.CanCommunicate(r.localID, id) {
			members = append(members, member.Clone())
		}
	}
	return members, nil
}

// UpdateMember updates member information
func (r *SimulatedMemberRegistry) UpdateMember(ctx context.Context, member *Member) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, ok := r.members[member.ID]; ok {
		r.members[member.ID] = member.Clone()
	}
	return nil
}

// AddObserver adds a membership observer
func (r *SimulatedMemberRegistry) AddObserver(observer MembershipObserver) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.observers = append(r.observers, observer)
}

// RemoveObserver removes a membership observer
func (r *SimulatedMemberRegistry) RemoveObserver(observer MembershipObserver) {
	r.mu.Lock()
	defer r.mu.Unlock()

	for i, o := range r.observers {
		if &o == &observer {
			r.observers = append(r.observers[:i], r.observers[i+1:]...)
			return
		}
	}
}

// TestPartitionSimulator_Basic tests basic partition simulator functionality
func TestPartitionSimulator_Basic(t *testing.T) {
	ps := NewPartitionSimulator()

	// By default, all communication is allowed
	if !ps.CanCommunicate("member1", "member2") {
		t.Error("Expected default communication to be allowed")
	}

	// Block communication
	ps.BlockCommunication("member1", "member2")
	if ps.CanCommunicate("member1", "member2") {
		t.Error("Expected communication to be blocked")
	}

	// Allow communication again
	ps.AllowCommunication("member1", "member2")
	if !ps.CanCommunicate("member1", "member2") {
		t.Error("Expected communication to be allowed")
	}
}

// TestPartitionSimulator_SymmetricPartition tests symmetric partition creation
func TestPartitionSimulator_SymmetricPartition(t *testing.T) {
	ps := NewPartitionSimulator()

	group1 := []string{"member1", "member2"}
	group2 := []string{"member3", "member4", "member5"}

	ps.CreateSymmetricPartition(group1, group2)

	// Members in the same group should be able to communicate
	if !ps.CanCommunicate("member1", "member2") {
		t.Error("Expected intra-group1 communication to be allowed")
	}
	if !ps.CanCommunicate("member3", "member4") {
		t.Error("Expected intra-group2 communication to be allowed")
	}

	// Members across groups should not be able to communicate
	if ps.CanCommunicate("member1", "member3") {
		t.Error("Expected cross-group communication to be blocked")
	}
	if ps.CanCommunicate("member3", "member1") {
		t.Error("Expected cross-group communication to be blocked (reverse)")
	}

	// Heal partition
	ps.HealPartition()
	if !ps.CanCommunicate("member1", "member3") {
		t.Error("Expected communication to be restored after healing")
	}
}

// TestPartitionSimulator_Delays tests communication delays
func TestPartitionSimulator_Delays(t *testing.T) {
	ps := NewPartitionSimulator()

	delay := 50 * time.Millisecond
	ps.SetDelay("member1", "member2", delay)

	got := ps.GetDelay("member1", "member2")
	if got != delay {
		t.Errorf("GetDelay() = %v, want %v", got, delay)
	}

	// No delay in the reverse direction
	if ps.GetDelay("member2", "member1") != 0 {
		t.Error("Expected no delay in reverse direction")
	}
}

// TestSimulatedRegistry_PartitionedMembers tests member visibility during partition
func TestSimulatedRegistry_PartitionedMembers(t *testing.T) {
	ps := NewPartitionSimulator()

	// Create two registries for two members
	registry1 := NewSimulatedMemberRegistry("member1", ps)
	registry2 := NewSimulatedMemberRegistry("member2", ps)

	// Register members
	m1 := &Member{ID: "member1", Name: "Member 1", Status: MemberStatusHealthy}
	m2 := &Member{ID: "member2", Name: "Member 2", Status: MemberStatusHealthy}

	registry1.Register(context.Background(), m1)
	registry1.Register(context.Background(), m2)
	registry2.Register(context.Background(), m1)
	registry2.Register(context.Background(), m2)

	// Before partition, both can see each other
	members, _ := registry1.ListMembers(context.Background())
	if len(members) != 2 {
		t.Errorf("Expected 2 members, got %d", len(members))
	}

	// Create partition
	ps.BlockCommunication("member1", "member2")

	// After partition, member1 cannot see member2
	members, _ = registry1.ListMembers(context.Background())
	if len(members) != 1 {
		t.Errorf("Expected 1 visible member after partition, got %d", len(members))
	}
	if members[0].ID != "member1" {
		t.Error("Expected only member1 to be visible")
	}

	// member2 can still see member1 (asymmetric partition)
	members, _ = registry2.ListMembers(context.Background())
	if len(members) != 2 {
		t.Errorf("Expected 2 members from registry2 (asymmetric partition), got %d", len(members))
	}
}

// TestNetworkPartition_QuorumLoss tests quorum loss during network partition
func TestNetworkPartition_QuorumLoss(t *testing.T) {
	// Simulate a 5-node cluster
	members := []*Member{
		{ID: "m1", Name: "Member 1", Status: MemberStatusHealthy},
		{ID: "m2", Name: "Member 2", Status: MemberStatusHealthy},
		{ID: "m3", Name: "Member 3", Status: MemberStatusHealthy},
		{ID: "m4", Name: "Member 4", Status: MemberStatusHealthy},
		{ID: "m5", Name: "Member 5", Status: MemberStatusHealthy},
	}

	// Calculate quorum (majority)
	quorumSize := CalculateQuorumSize(len(members))
	if quorumSize != 3 {
		t.Errorf("Expected quorum of 3 for 5-node cluster, got %d", quorumSize)
	}

	// Simulate partition: minority group (2 members) loses quorum
	minorityGroup := map[string]bool{"m1": true, "m2": true}
	majorityGroup := map[string]bool{"m3": true, "m4": true, "m5": true}

	// Count visible members from minority perspective
	minorityVisibleCount := 0
	for _, m := range members {
		if minorityGroup[m.ID] {
			minorityVisibleCount++
		}
	}

	// Minority should not have quorum
	if minorityVisibleCount >= quorumSize {
		t.Errorf("Minority group should not have quorum: visible=%d, quorum=%d",
			minorityVisibleCount, quorumSize)
	}

	// Count visible members from majority perspective
	majorityVisibleCount := 0
	for _, m := range members {
		if majorityGroup[m.ID] {
			majorityVisibleCount++
		}
	}

	// Majority should have quorum
	if majorityVisibleCount < quorumSize {
		t.Errorf("Majority group should have quorum: visible=%d, quorum=%d",
			majorityVisibleCount, quorumSize)
	}
}

// TestNetworkPartition_SplitBrain tests split-brain scenario detection
func TestNetworkPartition_SplitBrain(t *testing.T) {
	ps := NewPartitionSimulator()

	// Create equal split - this is a split-brain scenario for a 4-node cluster
	group1 := []string{"m1", "m2"}
	group2 := []string{"m3", "m4"}

	ps.CreateSymmetricPartition(group1, group2)

	// Neither group has quorum in a 4-node cluster (need 3)
	quorumSize := CalculateQuorumSize(4)

	group1Visible := len(group1)
	group2Visible := len(group2)

	// Detect split-brain: both groups see < quorum
	isSplitBrain := group1Visible < quorumSize && group2Visible < quorumSize
	if !isSplitBrain {
		t.Error("Expected split-brain scenario to be detected")
	}

	// Verify neither group should perform writes
	group1CanWrite := group1Visible >= quorumSize
	group2CanWrite := group2Visible >= quorumSize

	if group1CanWrite || group2CanWrite {
		t.Error("Neither group should be able to write during split-brain")
	}
}

// TestNetworkPartition_LeaderIsolation tests leader isolation scenario
func TestNetworkPartition_LeaderIsolation(t *testing.T) {
	// Simulate leader becoming isolated
	members := map[string]*Member{
		"leader":    {ID: "leader", Name: "Leader", Status: MemberStatusHealthy, IsLeader: true},
		"follower1": {ID: "follower1", Name: "Follower 1", Status: MemberStatusHealthy},
		"follower2": {ID: "follower2", Name: "Follower 2", Status: MemberStatusHealthy},
	}

	ps := NewPartitionSimulator()

	// Isolate leader from all followers
	ps.BlockCommunication("leader", "follower1")
	ps.BlockCommunication("leader", "follower2")
	ps.BlockCommunication("follower1", "leader")
	ps.BlockCommunication("follower2", "leader")

	// Create registry from leader's perspective
	leaderRegistry := NewSimulatedMemberRegistry("leader", ps)
	for _, m := range members {
		leaderRegistry.Register(context.Background(), m)
	}

	// Leader should only see itself
	visibleMembers, _ := leaderRegistry.ListMembers(context.Background())
	if len(visibleMembers) != 1 {
		t.Errorf("Isolated leader should only see itself, got %d members", len(visibleMembers))
	}

	// Leader should lose quorum
	quorumSize := CalculateQuorumSize(len(members))
	if len(visibleMembers) >= quorumSize {
		t.Error("Isolated leader should not have quorum")
	}

	// Create registry from follower's perspective
	followerRegistry := NewSimulatedMemberRegistry("follower1", ps)
	for _, m := range members {
		followerRegistry.Register(context.Background(), m)
	}

	// Followers can see each other but not the leader
	visibleFromFollower, _ := followerRegistry.ListMembers(context.Background())
	hasLeaderVisible := false
	for _, m := range visibleFromFollower {
		if m.ID == "leader" {
			hasLeaderVisible = true
			break
		}
	}
	if hasLeaderVisible {
		t.Error("Followers should not see isolated leader")
	}
}

// TestNetworkPartition_PartialPartition tests partial network partition
func TestNetworkPartition_PartialPartition(t *testing.T) {
	ps := NewPartitionSimulator()

	// Create 5-node cluster where only m3 is partitioned from m1
	// m1 <-> m2, m4, m5
	// m2 <-> m1, m3, m4, m5
	// m3 <-> m2, m4, m5 (cannot reach m1)
	// m4 <-> m1, m2, m3, m5
	// m5 <-> m1, m2, m3, m4

	// Only block m1 <-> m3
	ps.BlockCommunication("m1", "m3")
	ps.BlockCommunication("m3", "m1")

	// m1 should see 4 members (itself + m2, m4, m5)
	// m3 should see 4 members (itself + m2, m4, m5)
	// All other members should see all 5 members

	members := []string{"m1", "m2", "m3", "m4", "m5"}

	// Check visibility from m1's perspective
	m1Visible := 0
	for _, target := range members {
		if target == "m1" || ps.CanCommunicate("m1", target) {
			m1Visible++
		}
	}
	if m1Visible != 4 {
		t.Errorf("m1 should see 4 members, got %d", m1Visible)
	}

	// Check visibility from m2's perspective (should see all)
	m2Visible := 0
	for _, target := range members {
		if target == "m2" || ps.CanCommunicate("m2", target) {
			m2Visible++
		}
	}
	if m2Visible != 5 {
		t.Errorf("m2 should see 5 members, got %d", m2Visible)
	}
}

// TestNetworkPartition_AsymmetricPartition tests asymmetric network partition
func TestNetworkPartition_AsymmetricPartition(t *testing.T) {
	ps := NewPartitionSimulator()

	// m1 can send to m2, but m2 cannot send to m1
	// This simulates a unidirectional network failure
	ps.BlockCommunication("m2", "m1")

	// m1 -> m2: allowed
	if !ps.CanCommunicate("m1", "m2") {
		t.Error("m1 should be able to reach m2")
	}

	// m2 -> m1: blocked
	if ps.CanCommunicate("m2", "m1") {
		t.Error("m2 should not be able to reach m1")
	}

	// This asymmetric partition can cause confusing behavior:
	// - m1 thinks m2 is healthy (heartbeats arrive)
	// - m2 thinks m1 is unhealthy (no heartbeats received)
}

// TestNetworkPartition_HealingSequence tests partition healing
func TestNetworkPartition_HealingSequence(t *testing.T) {
	ps := NewPartitionSimulator()

	group1 := []string{"m1", "m2"}
	group2 := []string{"m3", "m4"}

	// Create partition
	ps.CreateSymmetricPartition(group1, group2)

	// Verify partition
	if ps.CanCommunicate("m1", "m3") {
		t.Error("Expected partition to be in effect")
	}

	// Simulate gradual healing - first allow m1 to reach m3
	ps.AllowCommunication("m1", "m3")

	if !ps.CanCommunicate("m1", "m3") {
		t.Error("Expected m1 -> m3 to be healed")
	}
	if ps.CanCommunicate("m3", "m1") {
		t.Error("Expected m3 -> m1 to still be partitioned")
	}

	// Complete healing
	ps.HealPartition()

	if !ps.CanCommunicate("m1", "m3") || !ps.CanCommunicate("m3", "m1") {
		t.Error("Expected full partition healing")
	}
}

// TestNetworkPartition_WithDelay tests partition with high latency
func TestNetworkPartition_WithDelay(t *testing.T) {
	ps := NewPartitionSimulator()

	// Set high latency between m1 and m2
	latency := 100 * time.Millisecond
	ps.SetDelay("m1", "m2", latency)

	// Create registry
	registry := NewSimulatedMemberRegistry("m1", ps)
	m2 := &Member{ID: "m2", Name: "Member 2", Status: MemberStatusHealthy}
	registry.Register(context.Background(), m2)

	// Getting m2 should take at least the latency time
	start := time.Now()
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	_, err := registry.GetMember(ctx, "m2")
	elapsed := time.Since(start)

	if err != nil {
		t.Fatalf("GetMember failed: %v", err)
	}

	if elapsed < latency {
		t.Errorf("Expected at least %v delay, got %v", latency, elapsed)
	}
}

// TestNetworkPartition_ContextTimeout tests timeout during partition delay
func TestNetworkPartition_ContextTimeout(t *testing.T) {
	ps := NewPartitionSimulator()

	// Set very high latency
	ps.SetDelay("m1", "m2", 5*time.Second)

	registry := NewSimulatedMemberRegistry("m1", ps)
	m2 := &Member{ID: "m2", Name: "Member 2", Status: MemberStatusHealthy}
	registry.Register(context.Background(), m2)

	// Context with short timeout
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	_, err := registry.GetMember(ctx, "m2")
	if err == nil {
		t.Error("Expected timeout error")
	}
	if err != context.DeadlineExceeded {
		t.Errorf("Expected DeadlineExceeded, got %v", err)
	}
}

// TestNetworkPartition_MemberHealthTransitions tests health status changes during partition
func TestNetworkPartition_MemberHealthTransitions(t *testing.T) {
	// Simulate health transitions based on heartbeat visibility
	type healthTracker struct {
		lastHeartbeat time.Time
		status        MemberStatus
	}

	members := map[string]*healthTracker{
		"m1": {lastHeartbeat: time.Now(), status: MemberStatusHealthy},
		"m2": {lastHeartbeat: time.Now(), status: MemberStatusHealthy},
		"m3": {lastHeartbeat: time.Now(), status: MemberStatusHealthy},
	}

	heartbeatTimeout := 100 * time.Millisecond

	// Simulate m3 becoming partitioned (no heartbeats received)
	members["m3"].lastHeartbeat = time.Now().Add(-200 * time.Millisecond)

	// Check health based on heartbeat timeout
	now := time.Now()
	for id, tracker := range members {
		if now.Sub(tracker.lastHeartbeat) > heartbeatTimeout {
			tracker.status = MemberStatusUnhealthy
		}
		members[id] = tracker
	}

	// m3 should be unhealthy
	if members["m3"].status != MemberStatusUnhealthy {
		t.Error("Expected m3 to be marked unhealthy after heartbeat timeout")
	}

	// m1 and m2 should still be healthy
	if members["m1"].status != MemberStatusHealthy {
		t.Error("Expected m1 to remain healthy")
	}
	if members["m2"].status != MemberStatusHealthy {
		t.Error("Expected m2 to remain healthy")
	}
}

// TestNetworkPartition_ConcurrentAccess tests concurrent access during partition changes
func TestNetworkPartition_ConcurrentAccess(t *testing.T) {
	ps := NewPartitionSimulator()

	var wg sync.WaitGroup
	iterations := 100

	// Concurrent readers
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < iterations; j++ {
				ps.CanCommunicate("m1", "m2")
				ps.GetDelay("m1", "m2")
			}
		}()
	}

	// Concurrent writers
	for i := 0; i < 3; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < iterations; j++ {
				if j%2 == 0 {
					ps.BlockCommunication("m1", "m2")
				} else {
					ps.AllowCommunication("m1", "m2")
				}
				ps.SetDelay("m1", "m2", time.Duration(j)*time.Millisecond)
			}
		}()
	}

	wg.Wait()
	// Test passes if no race conditions or deadlocks
}

// QuorumScenario represents a network partition scenario
type QuorumScenario struct {
	name           string
	totalMembers   int
	partitionSizes []int // sizes of each partition group
	expectedQuorum []bool
}

// TestNetworkPartition_QuorumScenarios tests various quorum scenarios
func TestNetworkPartition_QuorumScenarios(t *testing.T) {
	scenarios := []QuorumScenario{
		{
			name:           "3-node: majority partition",
			totalMembers:   3,
			partitionSizes: []int{2, 1},
			expectedQuorum: []bool{true, false},
		},
		{
			name:           "5-node: 3-2 split",
			totalMembers:   5,
			partitionSizes: []int{3, 2},
			expectedQuorum: []bool{true, false},
		},
		{
			name:           "5-node: 2-2-1 split",
			totalMembers:   5,
			partitionSizes: []int{2, 2, 1},
			expectedQuorum: []bool{false, false, false},
		},
		{
			name:           "7-node: 4-3 split",
			totalMembers:   7,
			partitionSizes: []int{4, 3},
			expectedQuorum: []bool{true, false},
		},
		{
			name:           "7-node: 3-2-2 split",
			totalMembers:   7,
			partitionSizes: []int{3, 2, 2},
			expectedQuorum: []bool{false, false, false},
		},
	}

	for _, scenario := range scenarios {
		t.Run(scenario.name, func(t *testing.T) {
			quorumSize := CalculateQuorumSize(scenario.totalMembers)

			for i, partitionSize := range scenario.partitionSizes {
				hasQuorum := partitionSize >= quorumSize
				expected := scenario.expectedQuorum[i]

				if hasQuorum != expected {
					t.Errorf("Partition %d (size=%d): hasQuorum=%v, expected=%v (quorum=%d)",
						i+1, partitionSize, hasQuorum, expected, quorumSize)
				}
			}
		})
	}
}
