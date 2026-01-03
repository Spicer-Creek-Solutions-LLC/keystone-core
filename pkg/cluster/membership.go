package cluster

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"sync"
	"time"

	"github.com/google/uuid"
)

const (
	// Key prefixes for etcd storage
	memberKeyPrefix = "/members/"
	memberMetaKey   = "/cluster/meta"
)

// MembershipManager manages cluster membership.
type MembershipManager struct {
	config      *Config
	etcd        *EtcdClient
	localMember *Member
	members     map[string]*Member
	observers   []MembershipObserver
	mu          sync.RWMutex
	stopChan    chan struct{}
	doneChan    chan struct{}
	started     bool
}

// NewMembershipManager creates a new membership manager.
func NewMembershipManager(config *Config, etcd *EtcdClient) (*MembershipManager, error) {
	if config == nil {
		return nil, fmt.Errorf("config is required")
	}
	if etcd == nil {
		return nil, fmt.Errorf("etcd client is required")
	}

	return &MembershipManager{
		config:   config,
		etcd:     etcd,
		members:  make(map[string]*Member),
		stopChan: make(chan struct{}),
		doneChan: make(chan struct{}),
	}, nil
}

// Start starts the membership manager.
func (m *MembershipManager) Start(ctx context.Context) error {
	m.mu.Lock()
	if m.started {
		m.mu.Unlock()
		return fmt.Errorf("membership manager already started")
	}
	m.started = true
	m.mu.Unlock()

	// Create the local member
	member, err := m.createLocalMember()
	if err != nil {
		return fmt.Errorf("failed to create local member: %w", err)
	}

	m.mu.Lock()
	m.localMember = member
	m.mu.Unlock()

	// Create a session for ephemeral member registration
	_, err = m.etcd.CreateSession(ctx, int(m.config.Etcd.LeasesTTL))
	if err != nil {
		return fmt.Errorf("failed to create etcd session: %w", err)
	}

	// Register the local member
	if err := m.register(ctx); err != nil {
		return fmt.Errorf("failed to register member: %w", err)
	}

	// Load existing members
	if err := m.loadMembers(ctx); err != nil {
		return fmt.Errorf("failed to load members: %w", err)
	}

	// Start watching for member changes
	if err := m.watchMembers(ctx); err != nil {
		return fmt.Errorf("failed to start member watch: %w", err)
	}

	// Start heartbeat loop
	go m.heartbeatLoop(ctx)

	// Start member health check loop
	go m.healthCheckLoop(ctx)

	return nil
}

// Stop stops the membership manager.
func (m *MembershipManager) Stop(ctx context.Context) error {
	m.mu.Lock()
	if !m.started {
		m.mu.Unlock()
		return nil
	}
	m.started = false
	m.mu.Unlock()

	close(m.stopChan)

	// Deregister the local member
	if err := m.deregister(ctx); err != nil {
		return fmt.Errorf("failed to deregister member: %w", err)
	}

	// Wait for goroutines to finish with timeout
	select {
	case <-m.doneChan:
	case <-time.After(5 * time.Second):
	}

	return nil
}

// createLocalMember creates the local member record.
func (m *MembershipManager) createLocalMember() (*Member, error) {
	memberID := m.config.MemberID
	if memberID == "" {
		memberID = uuid.New().String()
	}

	memberName := m.config.MemberName
	if memberName == "" {
		hostname, err := os.Hostname()
		if err != nil {
			memberName = memberID[:8]
		} else {
			memberName = hostname
		}
	}

	address, err := m.config.GetAdvertiseAddress()
	if err != nil {
		return nil, fmt.Errorf("failed to get advertise address: %w", err)
	}

	grpcAddress, err := m.config.GetGRPCAddress()
	if err != nil {
		return nil, fmt.Errorf("failed to get gRPC address: %w", err)
	}

	return &Member{
		ID:            memberID,
		Name:          memberName,
		Address:       address,
		GRPCAddress:   grpcAddress,
		Status:        MemberStatusHealthy,
		IsLeader:      false,
		Version:       getVersion(),
		JoinedAt:      time.Now().UTC(),
		LastHeartbeat: time.Now().UTC(),
		Metadata:      make(map[string]string),
	}, nil
}

// register registers the local member in etcd.
func (m *MembershipManager) register(ctx context.Context) error {
	m.mu.RLock()
	member := m.localMember
	m.mu.RUnlock()

	if member == nil {
		return fmt.Errorf("local member not initialized")
	}

	data, err := json.Marshal(member)
	if err != nil {
		return fmt.Errorf("failed to marshal member: %w", err)
	}

	key := memberKeyPrefix + member.ID
	if err := m.etcd.PutWithLease(ctx, key, data); err != nil {
		return fmt.Errorf("failed to register member: %w", err)
	}

	// Add to local cache
	m.mu.Lock()
	m.members[member.ID] = member
	m.mu.Unlock()

	// Notify observers
	m.notifyObservers(MembershipEvent{
		Type:      MembershipEventJoined,
		Member:    member.Clone(),
		Timestamp: time.Now().UTC(),
	})

	return nil
}

// deregister removes the local member from etcd.
func (m *MembershipManager) deregister(ctx context.Context) error {
	m.mu.RLock()
	member := m.localMember
	m.mu.RUnlock()

	if member == nil {
		return nil
	}

	// Update status to leaving
	member.Status = MemberStatusLeaving
	data, err := json.Marshal(member)
	if err != nil {
		return fmt.Errorf("failed to marshal member: %w", err)
	}

	key := memberKeyPrefix + member.ID
	// Put without lease so other members see the leaving status
	if err := m.etcd.Put(ctx, key, data, 5*time.Second); err != nil {
		// Log but don't fail - the lease will expire anyway
	}

	// Revoke the session to immediately remove the member
	if err := m.etcd.RevokeSession(ctx); err != nil {
		return fmt.Errorf("failed to revoke session: %w", err)
	}

	// Notify observers
	m.notifyObservers(MembershipEvent{
		Type:      MembershipEventLeft,
		Member:    member.Clone(),
		Timestamp: time.Now().UTC(),
		Reason:    "graceful shutdown",
	})

	return nil
}

// loadMembers loads all current members from etcd.
func (m *MembershipManager) loadMembers(ctx context.Context) error {
	data, err := m.etcd.List(ctx, memberKeyPrefix)
	if err != nil {
		return fmt.Errorf("failed to list members: %w", err)
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	for _, value := range data {
		var member Member
		if err := json.Unmarshal(value, &member); err != nil {
			continue // Skip invalid entries
		}
		m.members[member.ID] = &member
	}

	return nil
}

// watchMembers watches for member changes in etcd.
func (m *MembershipManager) watchMembers(ctx context.Context) error {
	handler := func(key string, value []byte, deleted bool) {
		// Extract member ID from key
		if len(key) <= len(memberKeyPrefix) {
			return
		}
		memberID := key[len(memberKeyPrefix):]

		m.mu.Lock()
		defer m.mu.Unlock()

		if deleted {
			// Member removed
			if member, ok := m.members[memberID]; ok {
				delete(m.members, memberID)
				m.notifyObserversLocked(MembershipEvent{
					Type:      MembershipEventFailed,
					Member:    member.Clone(),
					Timestamp: time.Now().UTC(),
					Reason:    "member removed from etcd",
				})
			}
			return
		}

		// Member added or updated
		var member Member
		if err := json.Unmarshal(value, &member); err != nil {
			return
		}

		existing, exists := m.members[memberID]
		m.members[memberID] = &member

		if !exists {
			m.notifyObserversLocked(MembershipEvent{
				Type:      MembershipEventJoined,
				Member:    member.Clone(),
				Timestamp: time.Now().UTC(),
			})
		} else if existing.Status != member.Status {
			eventType := MembershipEventUpdated
			if member.Status == MemberStatusHealthy && existing.Status == MemberStatusUnhealthy {
				eventType = MembershipEventRecovered
			}
			m.notifyObserversLocked(MembershipEvent{
				Type:      eventType,
				Member:    member.Clone(),
				Timestamp: time.Now().UTC(),
			})
		}
	}

	return m.etcd.Watch(ctx, memberKeyPrefix, handler)
}

// heartbeatLoop sends periodic heartbeats.
func (m *MembershipManager) heartbeatLoop(ctx context.Context) {
	ticker := time.NewTicker(m.config.HeartbeatInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-m.stopChan:
			close(m.doneChan)
			return
		case <-ticker.C:
			if err := m.heartbeat(ctx); err != nil {
				// Log error but continue
			}
		}
	}
}

// heartbeat sends a heartbeat for the local member.
func (m *MembershipManager) heartbeat(ctx context.Context) error {
	m.mu.Lock()
	member := m.localMember
	if member == nil {
		m.mu.Unlock()
		return nil
	}
	member.LastHeartbeat = time.Now().UTC()
	m.mu.Unlock()

	data, err := json.Marshal(member)
	if err != nil {
		return fmt.Errorf("failed to marshal member: %w", err)
	}

	key := memberKeyPrefix + member.ID
	if err := m.etcd.PutWithLease(ctx, key, data); err != nil {
		return fmt.Errorf("failed to send heartbeat: %w", err)
	}

	return nil
}

// healthCheckLoop checks the health of cluster members.
func (m *MembershipManager) healthCheckLoop(ctx context.Context) {
	ticker := time.NewTicker(m.config.HeartbeatInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-m.stopChan:
			return
		case <-ticker.C:
			m.checkMemberHealth()
		}
	}
}

// checkMemberHealth checks the health of all members.
func (m *MembershipManager) checkMemberHealth() {
	m.mu.Lock()
	defer m.mu.Unlock()

	now := time.Now().UTC()
	timeout := m.config.HeartbeatTimeout

	for id, member := range m.members {
		if id == m.localMember.ID {
			continue // Skip self
		}

		// Check if member is stale
		if now.Sub(member.LastHeartbeat) > timeout {
			if member.Status == MemberStatusHealthy {
				member.Status = MemberStatusUnhealthy
				m.notifyObserversLocked(MembershipEvent{
					Type:      MembershipEventFailed,
					Member:    member.Clone(),
					Timestamp: now,
					Reason:    "heartbeat timeout",
				})
			}
		} else if now.Sub(member.LastHeartbeat) > timeout/2 {
			if member.Status == MemberStatusHealthy {
				member.Status = MemberStatusDegraded
				m.notifyObserversLocked(MembershipEvent{
					Type:      MembershipEventUpdated,
					Member:    member.Clone(),
					Timestamp: now,
					Reason:    "delayed heartbeat",
				})
			}
		} else if member.Status == MemberStatusDegraded || member.Status == MemberStatusUnhealthy {
			member.Status = MemberStatusHealthy
			m.notifyObserversLocked(MembershipEvent{
				Type:      MembershipEventRecovered,
				Member:    member.Clone(),
				Timestamp: now,
			})
		}
	}
}

// LocalMember returns the local member.
func (m *MembershipManager) LocalMember() *Member {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if m.localMember == nil {
		return nil
	}
	return m.localMember.Clone()
}

// GetMember returns a member by ID.
func (m *MembershipManager) GetMember(id string) (*Member, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	member, ok := m.members[id]
	if !ok {
		return nil, ErrMemberNotFound
	}
	return member.Clone(), nil
}

// ListMembers returns all cluster members.
func (m *MembershipManager) ListMembers() []*Member {
	m.mu.RLock()
	defer m.mu.RUnlock()

	members := make([]*Member, 0, len(m.members))
	for _, member := range m.members {
		members = append(members, member.Clone())
	}
	return members
}

// GetHealthyMembers returns all healthy members.
func (m *MembershipManager) GetHealthyMembers() []*Member {
	m.mu.RLock()
	defer m.mu.RUnlock()

	members := make([]*Member, 0, len(m.members))
	for _, member := range m.members {
		if member.Status.IsHealthy() {
			members = append(members, member.Clone())
		}
	}
	return members
}

// MemberCount returns the total number of members.
func (m *MembershipManager) MemberCount() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return len(m.members)
}

// HealthyMemberCount returns the number of healthy members.
func (m *MembershipManager) HealthyMemberCount() int {
	m.mu.RLock()
	defer m.mu.RUnlock()

	count := 0
	for _, member := range m.members {
		if member.Status.IsHealthy() {
			count++
		}
	}
	return count
}

// HasQuorum returns true if the cluster has quorum.
func (m *MembershipManager) HasQuorum() bool {
	healthyCount := m.HealthyMemberCount()
	memberCount := m.MemberCount()

	// Empty cluster has no quorum
	if memberCount == 0 {
		return false
	}

	quorumSize := m.config.QuorumSize
	if quorumSize == 0 {
		quorumSize = CalculateQuorumSize(memberCount)
	}

	return healthyCount >= quorumSize
}

// UpdateLocalMember updates the local member's information.
func (m *MembershipManager) UpdateLocalMember(ctx context.Context, updateFn func(*Member)) error {
	m.mu.Lock()
	member := m.localMember
	if member == nil {
		m.mu.Unlock()
		return fmt.Errorf("local member not initialized")
	}
	updateFn(member)
	m.mu.Unlock()

	data, err := json.Marshal(member)
	if err != nil {
		return fmt.Errorf("failed to marshal member: %w", err)
	}

	key := memberKeyPrefix + member.ID
	if err := m.etcd.PutWithLease(ctx, key, data); err != nil {
		return fmt.Errorf("failed to update member: %w", err)
	}

	return nil
}

// SetLeader marks a member as the leader.
func (m *MembershipManager) SetLeader(leaderID string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	for id, member := range m.members {
		member.IsLeader = (id == leaderID)
	}
}

// GetLeader returns the current leader.
func (m *MembershipManager) GetLeader() *Member {
	m.mu.RLock()
	defer m.mu.RUnlock()

	for _, member := range m.members {
		if member.IsLeader {
			return member.Clone()
		}
	}
	return nil
}

// AddObserver adds an observer for membership changes.
func (m *MembershipManager) AddObserver(observer MembershipObserver) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.observers = append(m.observers, observer)
}

// RemoveObserver removes a membership observer.
func (m *MembershipManager) RemoveObserver(observer MembershipObserver) {
	m.mu.Lock()
	defer m.mu.Unlock()

	for i, o := range m.observers {
		if &o == &observer {
			m.observers = append(m.observers[:i], m.observers[i+1:]...)
			return
		}
	}
}

// notifyObservers notifies all observers of a membership event.
func (m *MembershipManager) notifyObservers(event MembershipEvent) {
	m.mu.RLock()
	observers := make([]MembershipObserver, len(m.observers))
	copy(observers, m.observers)
	m.mu.RUnlock()

	for _, observer := range observers {
		observer(event)
	}
}

// notifyObserversLocked notifies observers while holding the lock.
// Must be called while holding m.mu.
func (m *MembershipManager) notifyObserversLocked(event MembershipEvent) {
	observers := make([]MembershipObserver, len(m.observers))
	copy(observers, m.observers)

	// Release lock before notifying to avoid deadlocks
	go func() {
		for _, observer := range observers {
			observer(event)
		}
	}()
}

// GetClusterInfo returns the current cluster information.
func (m *MembershipManager) GetClusterInfo() *ClusterInfo {
	m.mu.RLock()
	defer m.mu.RUnlock()

	members := make([]*Member, 0, len(m.members))
	healthyCount := 0
	var leaderID string

	for _, member := range m.members {
		members = append(members, member.Clone())
		if member.Status.IsHealthy() {
			healthyCount++
		}
		if member.IsLeader {
			leaderID = member.ID
		}
	}

	memberCount := len(members)
	quorumSize := m.config.QuorumSize
	if quorumSize == 0 {
		quorumSize = CalculateQuorumSize(memberCount)
	}

	hasQuorum := memberCount > 0 && healthyCount >= quorumSize

	status := ClusterStatusHealthy
	if memberCount == 0 || healthyCount < quorumSize {
		status = ClusterStatusUnhealthy
	} else if healthyCount < memberCount {
		status = ClusterStatusDegraded
	}

	return &ClusterInfo{
		Name:         m.config.ClusterName,
		Status:       status,
		LeaderID:     leaderID,
		Members:      members,
		MemberCount:  memberCount,
		HealthyCount: healthyCount,
		QuorumSize:   quorumSize,
		HasQuorum:    hasQuorum,
		UpdatedAt:    time.Now().UTC(),
	}
}

// AddMemberRequest contains the information needed to add a new member via API.
type AddMemberRequest struct {
	// ID is the unique identifier for this member (optional, generated if empty)
	ID string `json:"id,omitempty"`

	// Name is the human-readable name for this member (optional)
	Name string `json:"name,omitempty"`

	// Address is the network address (host:port) for API communication
	Address string `json:"address"`

	// GRPCAddress is the gRPC endpoint address (optional, derived from Address if empty)
	GRPCAddress string `json:"grpc_address,omitempty"`

	// NATSAddress is the NATS endpoint for agent connections (optional)
	NATSAddress string `json:"nats_address,omitempty"`

	// Metadata contains additional member information (optional)
	Metadata map[string]string `json:"metadata,omitempty"`
}

// Validate validates the AddMemberRequest.
func (r *AddMemberRequest) Validate() error {
	if r.Address == "" {
		return fmt.Errorf("address is required")
	}
	return nil
}

// AddMember adds a new member to the cluster via API.
// This allows pre-seeding the cluster with expected members or adding members
// that haven't started yet. The member will be in "unknown" status until it
// actually starts and reports in.
func (m *MembershipManager) AddMember(ctx context.Context, req *AddMemberRequest) (*Member, error) {
	if req == nil {
		return nil, fmt.Errorf("request is required")
	}

	if err := req.Validate(); err != nil {
		return nil, fmt.Errorf("invalid request: %w", err)
	}

	// Generate ID if not provided
	memberID := req.ID
	if memberID == "" {
		memberID = uuid.New().String()
	}

	// Generate name if not provided
	memberName := req.Name
	if memberName == "" {
		if len(memberID) >= 8 {
			memberName = "member-" + memberID[:8]
		} else {
			memberName = "member-" + memberID
		}
	}

	// Derive gRPC address from address if not provided
	grpcAddress := req.GRPCAddress
	if grpcAddress == "" {
		grpcAddress = req.Address
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	// Check if member already exists
	if _, exists := m.members[memberID]; exists {
		return nil, fmt.Errorf("member %s already exists", memberID)
	}

	// Check if address is already in use
	for _, existing := range m.members {
		if existing.Address == req.Address {
			return nil, fmt.Errorf("address %s is already in use by member %s", req.Address, existing.ID)
		}
	}

	// Create the member
	member := &Member{
		ID:            memberID,
		Name:          memberName,
		Address:       req.Address,
		GRPCAddress:   grpcAddress,
		NATSAddress:   req.NATSAddress,
		Status:        MemberStatusUnknown, // Unknown until it starts and reports in
		IsLeader:      false,
		Version:       "", // Will be set when member starts
		JoinedAt:      time.Now().UTC(),
		LastHeartbeat: time.Time{}, // No heartbeat yet
		Metadata:      req.Metadata,
	}

	if member.Metadata == nil {
		member.Metadata = make(map[string]string)
	}
	member.Metadata["added_via"] = "api"

	// Store in etcd (without lease - this is a persistent entry)
	data, err := json.Marshal(member)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal member: %w", err)
	}

	key := memberKeyPrefix + memberID
	if err := m.etcd.Put(ctx, key, data, 5*time.Second); err != nil {
		return nil, fmt.Errorf("failed to store member in etcd: %w", err)
	}

	// Add to local cache
	m.members[memberID] = member

	// Notify observers
	m.notifyObserversLocked(MembershipEvent{
		Type:      MembershipEventJoined,
		Member:    member.Clone(),
		Timestamp: time.Now().UTC(),
		Reason:    "member added via API",
	})

	return member.Clone(), nil
}

// RemoveMember removes a member from the cluster.
// If force is false, the member must be unhealthy to be removed.
// If force is true, the member is removed regardless of health status.
func (m *MembershipManager) RemoveMember(ctx context.Context, memberID string, force bool) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Check if member exists
	member, ok := m.members[memberID]
	if !ok {
		return fmt.Errorf("member %s not found", memberID)
	}

	// Check if this is the local member
	if m.localMember != nil && m.localMember.ID == memberID {
		return fmt.Errorf("cannot remove local member %s", memberID)
	}

	// If not force, check if member is unhealthy
	if !force && member.Status != MemberStatusUnhealthy {
		return fmt.Errorf("member %s is not unhealthy (status: %s), use force=true to remove", memberID, member.Status)
	}

	// Delete from etcd
	key := memberKeyPrefix + memberID
	if err := m.etcd.Delete(ctx, key); err != nil {
		return fmt.Errorf("failed to delete member from etcd: %w", err)
	}

	// Remove from local cache
	delete(m.members, memberID)

	// Notify observers
	m.notifyObserversLocked(MembershipEvent{
		Type:      MembershipEventLeft,
		Member:    member,
		Timestamp: time.Now().UTC(),
		Reason:    "member removed via API",
	})

	return nil
}

// getVersion returns the current version.
// This should be replaced with the actual version from pkg/version.
func getVersion() string {
	// TODO: Import from pkg/version when available
	return "0.11.0-dev"
}
