// Package cluster provides high-availability clustering for Keystone Core.
// It implements etcd-based distributed coordination, leader election,
// work distribution, and automatic failover.
package cluster

import (
	"context"
	"errors"
	"time"
)

// Common errors for the cluster package.
var (
	// ErrNotLeader indicates the operation requires leader status.
	ErrNotLeader = errors.New("cluster: not the leader")

	// ErrNoQuorum indicates the cluster has lost quorum.
	ErrNoQuorum = errors.New("cluster: no quorum")

	// ErrMemberNotFound indicates the member was not found.
	ErrMemberNotFound = errors.New("cluster: member not found")

	// ErrMemberExists indicates the member already exists.
	ErrMemberExists = errors.New("cluster: member already exists")

	// ErrClusterNotReady indicates the cluster is not ready for operations.
	ErrClusterNotReady = errors.New("cluster: not ready")

	// ErrLeaderElectionFailed indicates leader election failed.
	ErrLeaderElectionFailed = errors.New("cluster: leader election failed")

	// ErrEtcdNotConnected indicates etcd is not connected.
	ErrEtcdNotConnected = errors.New("cluster: etcd not connected")

	// ErrShutdown indicates the cluster is shutting down.
	ErrShutdown = errors.New("cluster: shutting down")
)

// MemberStatus represents the health status of a cluster member.
type MemberStatus string

const (
	// MemberStatusHealthy indicates the member is healthy and operational.
	MemberStatusHealthy MemberStatus = "healthy"

	// MemberStatusDegraded indicates the member is operational but experiencing issues.
	MemberStatusDegraded MemberStatus = "degraded"

	// MemberStatusUnhealthy indicates the member is not operational.
	MemberStatusUnhealthy MemberStatus = "unhealthy"

	// MemberStatusUnknown indicates the member status is unknown.
	MemberStatusUnknown MemberStatus = "unknown"

	// MemberStatusLeaving indicates the member is gracefully leaving the cluster.
	MemberStatusLeaving MemberStatus = "leaving"
)

// String returns the string representation of MemberStatus.
func (s MemberStatus) String() string {
	return string(s)
}

// IsHealthy returns true if the member is healthy or degraded (still operational).
func (s MemberStatus) IsHealthy() bool {
	return s == MemberStatusHealthy || s == MemberStatusDegraded
}

// Status represents the overall cluster health status.
type Status string

const (
	// StatusHealthy indicates the cluster is healthy with quorum.
	StatusHealthy Status = "healthy"

	// StatusDegraded indicates the cluster has quorum but some members are unhealthy.
	StatusDegraded Status = "degraded"

	// StatusUnhealthy indicates the cluster has lost quorum.
	StatusUnhealthy Status = "unhealthy"

	// StatusForming indicates the cluster is still forming.
	StatusForming Status = "forming"
)

// String returns the string representation of Status.
func (s Status) String() string {
	return string(s)
}

// EtcdMode represents the etcd deployment mode.
type EtcdMode string

const (
	// EtcdModeEmbedded runs etcd embedded within the server process.
	// Suitable for development and small deployments.
	EtcdModeEmbedded EtcdMode = "embedded"

	// EtcdModeExternal connects to an external etcd cluster.
	// Recommended for production deployments.
	EtcdModeExternal EtcdMode = "external"
)

// String returns the string representation of EtcdMode.
func (m EtcdMode) String() string {
	return string(m)
}

// Member represents a cluster member.
type Member struct {
	// ID is the unique identifier for this member.
	ID string `json:"id"`

	// Name is the human-readable name for this member.
	Name string `json:"name"`

	// Address is the network address (host:port) for API communication.
	Address string `json:"address"`

	// GRPCAddress is the gRPC endpoint address.
	GRPCAddress string `json:"grpc_address"`

	// NATSAddress is the NATS endpoint for agent connections.
	NATSAddress string `json:"nats_address"`

	// Status is the current health status.
	Status MemberStatus `json:"status"`

	// IsLeader indicates if this member is the current leader.
	IsLeader bool `json:"is_leader"`

	// Version is the Keystone Core version running on this member.
	Version string `json:"version"`

	// JoinedAt is when this member joined the cluster.
	JoinedAt time.Time `json:"joined_at"`

	// LastHeartbeat is the last time a heartbeat was received.
	LastHeartbeat time.Time `json:"last_heartbeat"`

	// LeaseID is the etcd lease ID for this member's registration.
	LeaseID int64 `json:"lease_id,omitempty"`

	// Metadata contains additional member information.
	Metadata map[string]string `json:"metadata,omitempty"`

	// AgentCount is the number of agents connected to this member.
	AgentCount int `json:"agent_count"`

	// JobCount is the number of active jobs on this member.
	JobCount int `json:"job_count"`
}

// Clone creates a deep copy of the Member.
func (m *Member) Clone() *Member {
	if m == nil {
		return nil
	}
	clone := *m
	if m.Metadata != nil {
		clone.Metadata = make(map[string]string, len(m.Metadata))
		for k, v := range m.Metadata {
			clone.Metadata[k] = v
		}
	}
	return &clone
}

// Info represents the current cluster state.
type Info struct {
	// Name is the cluster name.
	Name string `json:"name"`

	// Status is the overall cluster status.
	Status Status `json:"status"`

	// LeaderID is the ID of the current leader.
	LeaderID string `json:"leader_id"`

	// Members is the list of cluster members.
	Members []*Member `json:"members"`

	// MemberCount is the total number of members.
	MemberCount int `json:"member_count"`

	// HealthyCount is the number of healthy members.
	HealthyCount int `json:"healthy_count"`

	// QuorumSize is the number of members required for quorum.
	QuorumSize int `json:"quorum_size"`

	// HasQuorum indicates if the cluster has quorum.
	HasQuorum bool `json:"has_quorum"`

	// CreatedAt is when the cluster was created.
	CreatedAt time.Time `json:"created_at"`

	// UpdatedAt is when the cluster info was last updated.
	UpdatedAt time.Time `json:"updated_at"`
}

// LeadershipEvent represents a leadership change event.
type LeadershipEvent struct {
	// Type is the type of leadership event.
	Type LeadershipEventType `json:"type"`

	// LeaderID is the new leader's ID (empty if no leader).
	LeaderID string `json:"leader_id"`

	// PreviousLeaderID is the previous leader's ID.
	PreviousLeaderID string `json:"previous_leader_id"`

	// Timestamp is when the event occurred.
	Timestamp time.Time `json:"timestamp"`

	// Reason provides additional context for the event.
	Reason string `json:"reason,omitempty"`
}

// LeadershipEventType represents the type of leadership event.
type LeadershipEventType string

const (
	// LeadershipEventElected indicates a new leader was elected.
	LeadershipEventElected LeadershipEventType = "elected"

	// LeadershipEventResigned indicates the leader resigned.
	LeadershipEventResigned LeadershipEventType = "resigned"

	// LeadershipEventLost indicates leadership was lost (e.g., network partition).
	LeadershipEventLost LeadershipEventType = "lost"

	// LeadershipEventTransferred indicates leadership was transferred.
	LeadershipEventTransferred LeadershipEventType = "transferred"
)

// MembershipEvent represents a membership change event.
type MembershipEvent struct {
	// Type is the type of membership event.
	Type MembershipEventType `json:"type"`

	// Member is the affected member.
	Member *Member `json:"member"`

	// Timestamp is when the event occurred.
	Timestamp time.Time `json:"timestamp"`

	// Reason provides additional context for the event.
	Reason string `json:"reason,omitempty"`
}

// MembershipEventType represents the type of membership event.
type MembershipEventType string

const (
	// MembershipEventJoined indicates a member joined the cluster.
	MembershipEventJoined MembershipEventType = "joined"

	// MembershipEventLeft indicates a member left the cluster gracefully.
	MembershipEventLeft MembershipEventType = "left"

	// MembershipEventFailed indicates a member failed (timeout, crash).
	MembershipEventFailed MembershipEventType = "failed"

	// MembershipEventRecovered indicates a failed member recovered.
	MembershipEventRecovered MembershipEventType = "recovered"

	// MembershipEventUpdated indicates member info was updated.
	MembershipEventUpdated MembershipEventType = "updated"
)

// LeadershipObserver is called when leadership changes occur.
type LeadershipObserver func(event LeadershipEvent)

// MembershipObserver is called when membership changes occur.
type MembershipObserver func(event MembershipEvent)

// LeaderElection provides leader election functionality.
type LeaderElection interface {
	// Campaign starts a campaign for leadership.
	// Blocks until leadership is acquired or context is cancelled.
	Campaign(ctx context.Context) error

	// Resign voluntarily gives up leadership.
	Resign(ctx context.Context) error

	// IsLeader returns true if this instance is the current leader.
	IsLeader() bool

	// GetLeader returns the current leader's member ID.
	GetLeader(ctx context.Context) (string, error)

	// TransferLeadership transfers leadership to another member.
	TransferLeadership(ctx context.Context, targetID string) error

	// AddObserver adds an observer for leadership changes.
	AddObserver(observer LeadershipObserver)

	// RemoveObserver removes a leadership observer.
	RemoveObserver(observer LeadershipObserver)
}

// MemberRegistry manages cluster membership.
type MemberRegistry interface {
	// Register registers this instance as a cluster member.
	Register(ctx context.Context, member *Member) error

	// Deregister removes this instance from the cluster.
	Deregister(ctx context.Context) error

	// Heartbeat sends a heartbeat to maintain membership.
	Heartbeat(ctx context.Context) error

	// GetMember returns a member by ID.
	GetMember(ctx context.Context, id string) (*Member, error)

	// ListMembers returns all cluster members.
	ListMembers(ctx context.Context) ([]*Member, error)

	// UpdateMember updates member information.
	UpdateMember(ctx context.Context, member *Member) error

	// AddObserver adds an observer for membership changes.
	AddObserver(observer MembershipObserver)

	// RemoveObserver removes a membership observer.
	RemoveObserver(observer MembershipObserver)
}

// State stores cluster state in etcd.
type State interface {
	// Get retrieves a value by key.
	Get(ctx context.Context, key string) ([]byte, error)

	// Set stores a value with optional TTL.
	Set(ctx context.Context, key string, value []byte, ttl time.Duration) error

	// Delete removes a key.
	Delete(ctx context.Context, key string) error

	// Watch watches for changes to a key prefix.
	Watch(ctx context.Context, prefix string, handler func(key string, value []byte, deleted bool)) error

	// List returns all keys with a given prefix.
	List(ctx context.Context, prefix string) (map[string][]byte, error)

	// CompareAndSwap atomically updates a key if it matches the expected value.
	CompareAndSwap(ctx context.Context, key string, expected, value []byte) (bool, error)
}

// ShardAssignment represents the assignment of work to a member.
type ShardAssignment struct {
	// AgentID is the agent being assigned.
	AgentID string `json:"agent_id"`

	// MemberID is the member responsible for this agent.
	MemberID string `json:"member_id"`

	// AssignedAt is when the assignment was made.
	AssignedAt time.Time `json:"assigned_at"`

	// Version is used for optimistic locking.
	Version int64 `json:"version"`
}

// RebalanceEvent represents a work rebalancing event.
type RebalanceEvent struct {
	// TriggerMemberID is the member that triggered rebalancing.
	TriggerMemberID string `json:"trigger_member_id"`

	// Reason is why rebalancing was triggered.
	Reason string `json:"reason"`

	// MovedAgents is the count of agents that were reassigned.
	MovedAgents int `json:"moved_agents"`

	// StartTime is when rebalancing started.
	StartTime time.Time `json:"start_time"`

	// EndTime is when rebalancing completed.
	EndTime time.Time `json:"end_time"`

	// Duration is how long rebalancing took.
	Duration time.Duration `json:"duration"`
}
