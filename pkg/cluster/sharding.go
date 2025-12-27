package cluster

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"sort"
	"sync"
	"time"
)

const (
	// DefaultVirtualNodes is the default number of virtual nodes per member.
	DefaultVirtualNodes = 150

	// RebalanceMinInterval is the minimum time between rebalances.
	RebalanceMinInterval = 5 * time.Second
)

// ShardingStrategy defines how agents are distributed across members.
type ShardingStrategy string

const (
	// ShardingStrategyConsistentHash uses consistent hashing.
	ShardingStrategyConsistentHash ShardingStrategy = "consistent_hash"

	// ShardingStrategyRoundRobin distributes agents round-robin.
	ShardingStrategyRoundRobin ShardingStrategy = "round_robin"

	// ShardingStrategyLeastConnections assigns to member with fewest agents.
	ShardingStrategyLeastConnections ShardingStrategy = "least_connections"
)

// ConsistentHash implements consistent hashing for agent distribution.
type ConsistentHash struct {
	virtualNodes int
	ring         []uint32
	members      map[uint32]string // hash -> memberID
	memberSet    map[string]bool   // memberID -> exists
	mu           sync.RWMutex
}

// NewConsistentHash creates a new consistent hash ring.
func NewConsistentHash(virtualNodes int) *ConsistentHash {
	if virtualNodes <= 0 {
		virtualNodes = DefaultVirtualNodes
	}

	return &ConsistentHash{
		virtualNodes: virtualNodes,
		ring:         make([]uint32, 0),
		members:      make(map[uint32]string),
		memberSet:    make(map[string]bool),
	}
}

// AddMember adds a member to the hash ring.
func (c *ConsistentHash) AddMember(memberID string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.memberSet[memberID] {
		return // Already exists
	}

	c.memberSet[memberID] = true

	// Add virtual nodes
	for i := 0; i < c.virtualNodes; i++ {
		hash := c.hash(fmt.Sprintf("%s-%d", memberID, i))
		c.ring = append(c.ring, hash)
		c.members[hash] = memberID
	}

	// Sort the ring
	sort.Slice(c.ring, func(i, j int) bool {
		return c.ring[i] < c.ring[j]
	})
}

// RemoveMember removes a member from the hash ring.
func (c *ConsistentHash) RemoveMember(memberID string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if !c.memberSet[memberID] {
		return // Doesn't exist
	}

	delete(c.memberSet, memberID)

	// Remove virtual nodes
	newRing := make([]uint32, 0, len(c.ring)-c.virtualNodes)
	for _, hash := range c.ring {
		if c.members[hash] != memberID {
			newRing = append(newRing, hash)
		} else {
			delete(c.members, hash)
		}
	}
	c.ring = newRing
}

// GetMember returns the member responsible for a given key.
func (c *ConsistentHash) GetMember(key string) string {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if len(c.ring) == 0 {
		return ""
	}

	hash := c.hash(key)

	// Binary search for the first node >= hash
	idx := sort.Search(len(c.ring), func(i int) bool {
		return c.ring[i] >= hash
	})

	// Wrap around if necessary
	if idx >= len(c.ring) {
		idx = 0
	}

	return c.members[c.ring[idx]]
}

// GetMembers returns all members in the ring.
func (c *ConsistentHash) GetMembers() []string {
	c.mu.RLock()
	defer c.mu.RUnlock()

	members := make([]string, 0, len(c.memberSet))
	for member := range c.memberSet {
		members = append(members, member)
	}
	return members
}

// MemberCount returns the number of members in the ring.
func (c *ConsistentHash) MemberCount() int {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return len(c.memberSet)
}

// hash computes a hash value for a key.
func (c *ConsistentHash) hash(key string) uint32 {
	h := sha256.Sum256([]byte(key))
	return binary.BigEndian.Uint32(h[:4])
}

// GetAffectedKeys returns the keys that would be affected by adding/removing a member.
// This is useful for knowing which agents need to be migrated during rebalancing.
func (c *ConsistentHash) GetAffectedKeys(keys []string, addMember, removeMember string) []string {
	c.mu.RLock()
	defer c.mu.RUnlock()

	affected := make([]string, 0)

	// Get current assignments
	currentAssignments := make(map[string]string)
	for _, key := range keys {
		currentAssignments[key] = c.GetMember(key)
	}

	// Create a temporary copy of the ring with the changes
	tempHash := NewConsistentHash(c.virtualNodes)
	for member := range c.memberSet {
		if member != removeMember {
			tempHash.AddMember(member)
		}
	}
	if addMember != "" {
		tempHash.AddMember(addMember)
	}

	// Check which keys would change
	for _, key := range keys {
		newMember := tempHash.GetMember(key)
		if newMember != currentAssignments[key] {
			affected = append(affected, key)
		}
	}

	return affected
}

// ShardManager manages agent sharding across cluster members.
type ShardManager struct {
	config       *Config
	membership   *MembershipManager
	shardStore   *ShardStore
	hashRing     *ConsistentHash
	strategy     ShardingStrategy
	assignments  map[string]string // agentID -> memberID
	agentCounts  map[string]int    // memberID -> count
	observers    []func(ShardChangeEvent)
	mu           sync.RWMutex
	stopChan     chan struct{}
	doneChan     chan struct{}
	started      bool
	lastRebalance time.Time
}

// ShardChangeEvent represents a change in shard assignments.
type ShardChangeEvent struct {
	AgentID       string    `json:"agent_id"`
	OldMemberID   string    `json:"old_member_id"`
	NewMemberID   string    `json:"new_member_id"`
	Reason        string    `json:"reason"`
	Timestamp     time.Time `json:"timestamp"`
}

// NewShardManager creates a new shard manager.
func NewShardManager(config *Config, membership *MembershipManager, shardStore *ShardStore) (*ShardManager, error) {
	if config == nil {
		return nil, fmt.Errorf("config is required")
	}
	if membership == nil {
		return nil, fmt.Errorf("membership manager is required")
	}
	if shardStore == nil {
		return nil, fmt.Errorf("shard store is required")
	}

	return &ShardManager{
		config:      config,
		membership:  membership,
		shardStore:  shardStore,
		hashRing:    NewConsistentHash(DefaultVirtualNodes),
		strategy:    ShardingStrategyConsistentHash,
		assignments: make(map[string]string),
		agentCounts: make(map[string]int),
		stopChan:    make(chan struct{}),
		doneChan:    make(chan struct{}),
	}, nil
}

// SetStrategy sets the sharding strategy.
func (m *ShardManager) SetStrategy(strategy ShardingStrategy) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.strategy = strategy
}

// Start starts the shard manager.
func (m *ShardManager) Start(ctx context.Context) error {
	m.mu.Lock()
	if m.started {
		m.mu.Unlock()
		return fmt.Errorf("shard manager already started")
	}
	m.started = true
	m.mu.Unlock()

	// Initialize the hash ring with current members
	members := m.membership.GetHealthyMembers()
	for _, member := range members {
		m.hashRing.AddMember(member.ID)
	}

	// Load existing assignments from store
	if err := m.loadAssignments(ctx); err != nil {
		return fmt.Errorf("failed to load assignments: %w", err)
	}

	// Subscribe to membership changes
	m.membership.AddObserver(m.onMembershipChange)

	// Watch for assignment changes from other nodes
	go m.watchAssignments(ctx)

	return nil
}

// Stop stops the shard manager.
func (m *ShardManager) Stop(ctx context.Context) error {
	m.mu.Lock()
	if !m.started {
		m.mu.Unlock()
		return nil
	}
	m.started = false
	m.mu.Unlock()

	close(m.stopChan)

	select {
	case <-m.doneChan:
	case <-time.After(5 * time.Second):
	}

	return nil
}

// AssignAgent assigns an agent to a cluster member.
func (m *ShardManager) AssignAgent(ctx context.Context, agentID string) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Check if already assigned
	if memberID, exists := m.assignments[agentID]; exists {
		// Check if the member is still healthy
		member, err := m.membership.GetMember(memberID)
		if err == nil && member.Status.IsHealthy() {
			return memberID, nil
		}
		// Member is gone or unhealthy, reassign
	}

	// Determine the member to assign to
	var memberID string
	switch m.strategy {
	case ShardingStrategyConsistentHash:
		memberID = m.hashRing.GetMember(agentID)
	case ShardingStrategyRoundRobin:
		memberID = m.roundRobinAssign()
	case ShardingStrategyLeastConnections:
		memberID = m.leastConnectionsAssign()
	default:
		memberID = m.hashRing.GetMember(agentID)
	}

	if memberID == "" {
		return "", fmt.Errorf("no available members")
	}

	// Store the assignment
	assignment := &ShardAssignment{
		AgentID:    agentID,
		MemberID:   memberID,
		AssignedAt: time.Now().UTC(),
		Version:    1,
	}

	if err := m.shardStore.SetAssignment(ctx, assignment); err != nil {
		return "", fmt.Errorf("failed to store assignment: %w", err)
	}

	m.assignments[agentID] = memberID
	m.agentCounts[memberID]++

	return memberID, nil
}

// GetAssignment returns the member assigned to an agent.
func (m *ShardManager) GetAssignment(agentID string) (string, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	memberID, exists := m.assignments[agentID]
	return memberID, exists
}

// RemoveAgent removes an agent's assignment.
func (m *ShardManager) RemoveAgent(ctx context.Context, agentID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	memberID, exists := m.assignments[agentID]
	if !exists {
		return nil
	}

	if err := m.shardStore.DeleteAssignment(ctx, agentID); err != nil {
		return fmt.Errorf("failed to delete assignment: %w", err)
	}

	delete(m.assignments, agentID)
	if m.agentCounts[memberID] > 0 {
		m.agentCounts[memberID]--
	}

	return nil
}

// ReassignAgent reassigns an agent to a new member.
func (m *ShardManager) ReassignAgent(ctx context.Context, agentID string, newMemberID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	oldMemberID := m.assignments[agentID]

	// Verify new member is healthy
	member, err := m.membership.GetMember(newMemberID)
	if err != nil {
		return fmt.Errorf("member not found: %w", err)
	}
	if !member.Status.IsHealthy() {
		return fmt.Errorf("member %s is not healthy", newMemberID)
	}

	// Update assignment
	assignment := &ShardAssignment{
		AgentID:    agentID,
		MemberID:   newMemberID,
		AssignedAt: time.Now().UTC(),
	}

	if err := m.shardStore.SetAssignment(ctx, assignment); err != nil {
		return fmt.Errorf("failed to update assignment: %w", err)
	}

	// Update local state
	m.assignments[agentID] = newMemberID
	if m.agentCounts[oldMemberID] > 0 {
		m.agentCounts[oldMemberID]--
	}
	m.agentCounts[newMemberID]++

	// Notify observers
	m.notifyObserversLocked(ShardChangeEvent{
		AgentID:     agentID,
		OldMemberID: oldMemberID,
		NewMemberID: newMemberID,
		Reason:      "manual reassignment",
		Timestamp:   time.Now().UTC(),
	})

	return nil
}

// Rebalance triggers a rebalancing of agent assignments.
func (m *ShardManager) Rebalance(ctx context.Context, reason string) (*RebalanceEvent, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Rate limit rebalancing
	if time.Since(m.lastRebalance) < RebalanceMinInterval {
		return nil, fmt.Errorf("rebalance rate limited, last rebalance was %v ago", time.Since(m.lastRebalance))
	}

	startTime := time.Now()
	movedAgents := 0

	// Get current healthy members
	members := m.membership.GetHealthyMembers()
	if len(members) == 0 {
		return nil, fmt.Errorf("no healthy members available")
	}

	// Update hash ring
	m.hashRing = NewConsistentHash(DefaultVirtualNodes)
	for _, member := range members {
		m.hashRing.AddMember(member.ID)
	}

	// Recalculate assignments for all agents
	for agentID, currentMemberID := range m.assignments {
		newMemberID := m.hashRing.GetMember(agentID)
		if newMemberID != currentMemberID {
			// Update assignment
			assignment := &ShardAssignment{
				AgentID:    agentID,
				MemberID:   newMemberID,
				AssignedAt: time.Now().UTC(),
			}

			if err := m.shardStore.SetAssignment(ctx, assignment); err != nil {
				continue // Skip on error
			}

			// Update local state
			if m.agentCounts[currentMemberID] > 0 {
				m.agentCounts[currentMemberID]--
			}
			m.agentCounts[newMemberID]++
			m.assignments[agentID] = newMemberID
			movedAgents++

			// Notify observers
			m.notifyObserversLocked(ShardChangeEvent{
				AgentID:     agentID,
				OldMemberID: currentMemberID,
				NewMemberID: newMemberID,
				Reason:      reason,
				Timestamp:   time.Now().UTC(),
			})
		}
	}

	m.lastRebalance = time.Now()
	endTime := time.Now()

	return &RebalanceEvent{
		TriggerMemberID: m.membership.LocalMember().ID,
		Reason:          reason,
		MovedAgents:     movedAgents,
		StartTime:       startTime,
		EndTime:         endTime,
		Duration:        endTime.Sub(startTime),
	}, nil
}

// GetAgentCountForMember returns the number of agents assigned to a member.
func (m *ShardManager) GetAgentCountForMember(memberID string) int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.agentCounts[memberID]
}

// GetAgentCountsPerMember returns the agent counts per member.
func (m *ShardManager) GetAgentCountsPerMember() map[string]int {
	m.mu.RLock()
	defer m.mu.RUnlock()

	counts := make(map[string]int, len(m.agentCounts))
	for memberID, count := range m.agentCounts {
		counts[memberID] = count
	}
	return counts
}

// GetAllAssignments returns all agent assignments.
func (m *ShardManager) GetAllAssignments() map[string]string {
	m.mu.RLock()
	defer m.mu.RUnlock()

	assignments := make(map[string]string, len(m.assignments))
	for agentID, memberID := range m.assignments {
		assignments[agentID] = memberID
	}
	return assignments
}

// GetAssignmentsForMember returns all agents assigned to a member.
func (m *ShardManager) GetAssignmentsForMember(memberID string) []string {
	m.mu.RLock()
	defer m.mu.RUnlock()

	agents := make([]string, 0)
	for agentID, assignedMember := range m.assignments {
		if assignedMember == memberID {
			agents = append(agents, agentID)
		}
	}
	return agents
}

// AddObserver adds an observer for shard changes.
func (m *ShardManager) AddObserver(observer func(ShardChangeEvent)) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.observers = append(m.observers, observer)
}

// loadAssignments loads existing assignments from the store.
func (m *ShardManager) loadAssignments(ctx context.Context) error {
	assignments, err := m.shardStore.ListAssignments(ctx)
	if err != nil {
		return err
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	for _, assignment := range assignments {
		m.assignments[assignment.AgentID] = assignment.MemberID
		m.agentCounts[assignment.MemberID]++
	}

	return nil
}

// watchAssignments watches for assignment changes from other nodes.
func (m *ShardManager) watchAssignments(ctx context.Context) {
	defer func() {
		select {
		case m.doneChan <- struct{}{}:
		default:
		}
	}()

	err := m.shardStore.WatchAssignments(ctx, func(assignment *ShardAssignment, deleted bool) {
		m.mu.Lock()
		defer m.mu.Unlock()

		if deleted {
			if memberID, exists := m.assignments[assignment.AgentID]; exists {
				delete(m.assignments, assignment.AgentID)
				if m.agentCounts[memberID] > 0 {
					m.agentCounts[memberID]--
				}
			}
		} else {
			oldMemberID := m.assignments[assignment.AgentID]
			if oldMemberID != assignment.MemberID {
				if m.agentCounts[oldMemberID] > 0 {
					m.agentCounts[oldMemberID]--
				}
				m.agentCounts[assignment.MemberID]++
				m.assignments[assignment.AgentID] = assignment.MemberID
			}
		}
	})

	if err != nil {
		// Log error
	}
}

// onMembershipChange handles membership change events.
func (m *ShardManager) onMembershipChange(event MembershipEvent) {
	m.mu.Lock()
	defer m.mu.Unlock()

	switch event.Type {
	case MembershipEventJoined:
		// Add member to hash ring
		m.hashRing.AddMember(event.Member.ID)
		// Trigger rebalance in background
		go m.triggerRebalance("member joined: " + event.Member.ID)

	case MembershipEventLeft, MembershipEventFailed:
		// Remove member from hash ring
		m.hashRing.RemoveMember(event.Member.ID)
		// Trigger rebalance in background
		go m.triggerRebalance("member left: " + event.Member.ID)
	}
}

// triggerRebalance triggers a rebalance operation.
func (m *ShardManager) triggerRebalance(reason string) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	m.Rebalance(ctx, reason)
}

// roundRobinAssign assigns using round-robin strategy.
func (m *ShardManager) roundRobinAssign() string {
	members := m.membership.GetHealthyMembers()
	if len(members) == 0 {
		return ""
	}

	// Find member with fewest assignments (simplified round-robin)
	minCount := -1
	var selectedMember string

	for _, member := range members {
		count := m.agentCounts[member.ID]
		if minCount == -1 || count < minCount {
			minCount = count
			selectedMember = member.ID
		}
	}

	return selectedMember
}

// leastConnectionsAssign assigns to the member with fewest connections.
func (m *ShardManager) leastConnectionsAssign() string {
	members := m.membership.GetHealthyMembers()
	if len(members) == 0 {
		return ""
	}

	minConnections := -1
	var selectedMember string

	for _, member := range members {
		if minConnections == -1 || member.AgentCount < minConnections {
			minConnections = member.AgentCount
			selectedMember = member.ID
		}
	}

	return selectedMember
}

// notifyObserversLocked notifies observers. Must be called with m.mu held.
func (m *ShardManager) notifyObserversLocked(event ShardChangeEvent) {
	observers := make([]func(ShardChangeEvent), len(m.observers))
	copy(observers, m.observers)

	go func() {
		for _, observer := range observers {
			observer(event)
		}
	}()
}

// IsLocalAgent returns true if the agent is assigned to this member.
func (m *ShardManager) IsLocalAgent(agentID string) bool {
	m.mu.RLock()
	defer m.mu.RUnlock()

	memberID, exists := m.assignments[agentID]
	if !exists {
		return false
	}

	localMember := m.membership.LocalMember()
	if localMember == nil {
		return false
	}

	return memberID == localMember.ID
}

// GetLocalAgents returns all agents assigned to this member.
func (m *ShardManager) GetLocalAgents() []string {
	localMember := m.membership.LocalMember()
	if localMember == nil {
		return nil
	}

	return m.GetAssignmentsForMember(localMember.ID)
}
