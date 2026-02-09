package identity

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sync"
	"time"
)

// HARole represents the role of a node in the HA cluster.
type HARole string

const (
	// HARoleLeader is the active leader that can issue SVIDs and manage CAs.
	HARoleLeader HARole = "leader"
	// HARoleFollower is a follower that replicates state from the leader.
	HARoleFollower HARole = "follower"
	// HARoleCandidate is a node trying to become leader.
	HARoleCandidate HARole = "candidate"
	// HARoleStandby is a standby node not participating in elections.
	HARoleStandby HARole = "standby"
)

// HAConfig configures high availability for the identity provider.
type HAConfig struct {
	// Enabled enables HA mode.
	Enabled bool

	// ClusterID is the unique identifier for this cluster.
	ClusterID string

	// NodeID is the unique identifier for this node.
	NodeID string

	// Peers is the list of peer node addresses.
	Peers []string

	// LeaderElection configures leader election.
	LeaderElection *LeaderElectionConfig

	// Replication configures state replication.
	Replication *ReplicationConfig

	// HealthCheck configures health checking.
	HealthCheck *HAHealthConfig
}

// LeaderElectionConfig configures leader election.
type LeaderElectionConfig struct {
	// ElectionTimeout is the timeout for leader election.
	ElectionTimeout time.Duration

	// HeartbeatInterval is how often the leader sends heartbeats.
	HeartbeatInterval time.Duration

	// LeaderLeaseDuration is how long a leader lease is valid.
	LeaderLeaseDuration time.Duration

	// RetryInterval is how long to wait before retrying election.
	RetryInterval time.Duration
}

// ReplicationConfig configures state replication.
type ReplicationConfig struct {
	// Mode is the replication mode.
	Mode ReplicationMode

	// SyncInterval is how often to sync state with peers.
	SyncInterval time.Duration

	// MaxLag is the maximum replication lag allowed.
	MaxLag time.Duration

	// BatchSize is the maximum number of items to replicate at once.
	BatchSize int
}

// ReplicationMode defines the replication mode.
type ReplicationMode string

const (
	// ReplicationModeSync requires all nodes to acknowledge before commit.
	ReplicationModeSync ReplicationMode = "sync"
	// ReplicationModeAsync replicates asynchronously after commit.
	ReplicationModeAsync ReplicationMode = "async"
	// ReplicationModeSemiSync requires majority acknowledgment.
	ReplicationModeSemiSync ReplicationMode = "semi-sync"
)

// HAHealthConfig configures HA health checking.
type HAHealthConfig struct {
	// CheckInterval is how often to check peer health.
	CheckInterval time.Duration

	// Timeout is the health check timeout.
	Timeout time.Duration

	// FailureThreshold is the number of failures before marking unhealthy.
	FailureThreshold int

	// RecoveryThreshold is the number of successes before marking healthy.
	RecoveryThreshold int
}

// DefaultHAConfig returns a default HA configuration.
func DefaultHAConfig() *HAConfig {
	return &HAConfig{
		Enabled:   false,
		ClusterID: "kscore-identity",
		LeaderElection: &LeaderElectionConfig{
			ElectionTimeout:     10 * time.Second,
			HeartbeatInterval:   1 * time.Second,
			LeaderLeaseDuration: 30 * time.Second,
			RetryInterval:       5 * time.Second,
		},
		Replication: &ReplicationConfig{
			Mode:         ReplicationModeSemiSync,
			SyncInterval: 5 * time.Second,
			MaxLag:       30 * time.Second,
			BatchSize:    100,
		},
		HealthCheck: &HAHealthConfig{
			CheckInterval:     5 * time.Second,
			Timeout:           3 * time.Second,
			FailureThreshold:  3,
			RecoveryThreshold: 2,
		},
	}
}

// HAState represents the current HA state of a node.
type HAState struct {
	// Role is the current role.
	Role HARole

	// LeaderID is the ID of the current leader.
	LeaderID string

	// LeaderAddress is the address of the current leader.
	LeaderAddress string

	// Term is the current election term.
	Term uint64

	// LastHeartbeat is when the last heartbeat was received.
	LastHeartbeat time.Time

	// Peers contains peer node states.
	Peers map[string]*PeerState

	// ReplicationLag is the current replication lag.
	ReplicationLag time.Duration

	// LastSyncTime is when state was last synced.
	LastSyncTime time.Time
}

// PeerState represents the state of a peer node.
type PeerState struct {
	// NodeID is the peer's node ID.
	NodeID string

	// Address is the peer's address.
	Address string

	// Role is the peer's role.
	Role HARole

	// Healthy indicates if the peer is healthy.
	Healthy bool

	// LastSeen is when the peer was last seen.
	LastSeen time.Time

	// ReplicationIndex is the peer's replication index.
	ReplicationIndex uint64

	// Version is the peer's software version.
	Version string
}

// HAIdentityProvider wraps an embedded provider with HA capabilities.
type HAIdentityProvider struct {
	config   *HAConfig
	provider *EmbeddedProvider
	state    *HAState
	mu       sync.RWMutex

	// Leader election
	leaderElector LeaderElector

	// State replication
	replicator StateReplicator

	// Trust bundle synchronizer
	bundleSync *TrustBundleSynchronizer

	// Callbacks
	onLeaderChange func(oldLeader, newLeader string)
	onRoleChange   func(oldRole, newRole HARole)

	// Control
	stopCh chan struct{}
	wg     sync.WaitGroup
}

// LeaderElector is an interface for leader election.
type LeaderElector interface {
	// Campaign starts a campaign to become leader.
	Campaign(ctx context.Context) error

	// Resign resigns from leadership.
	Resign(ctx context.Context) error

	// IsLeader returns true if this node is the leader.
	IsLeader() bool

	// GetLeader returns the current leader ID.
	GetLeader() (string, error)

	// Watch watches for leader changes.
	Watch(ctx context.Context) (<-chan string, error)
}

// StateReplicator is an interface for state replication.
type StateReplicator interface {
	// Replicate replicates state to peers.
	Replicate(ctx context.Context, state *ReplicatedState) error

	// Sync synchronizes state from leader.
	Sync(ctx context.Context) (*ReplicatedState, error)

	// GetReplicationStatus returns current replication status.
	GetReplicationStatus() *ReplicationStatus
}

// ReplicatedState represents state that needs to be replicated.
type ReplicatedState struct {
	// Version is the state version.
	Version uint64

	// Timestamp is when the state was created.
	Timestamp time.Time

	// CAState contains CA certificate and key (encrypted).
	CAState *CAReplicatedState

	// TrustBundle is the current trust bundle.
	TrustBundle *TrustBundleState

	// IssuedSVIDs tracks issued SVIDs for auditing.
	IssuedSVIDs []SVIDRecord

	// JoinTokens contains active join tokens.
	JoinTokens []JoinTokenRecord

	// Checksum is the state checksum for integrity.
	Checksum string
}

// CAReplicatedState contains replicated CA state.
type CAReplicatedState struct {
	// RootCA is the root CA certificate PEM.
	RootCA []byte

	// RootCAKey is the encrypted root CA private key.
	RootCAKey []byte

	// SigningCA is the signing CA certificate PEM.
	SigningCA []byte

	// SigningCAKey is the encrypted signing CA private key.
	SigningCAKey []byte

	// SigningCAExpiry is when the signing CA expires.
	SigningCAExpiry time.Time

	// NextSigningCA is the next signing CA (during rotation).
	NextSigningCA []byte

	// NextSigningCAKey is the encrypted next signing CA key.
	NextSigningCAKey []byte
}

// TrustBundleState contains replicated trust bundle state.
type TrustBundleState struct {
	// TrustDomain is the trust domain.
	TrustDomain string

	// X509Authorities is the serialized X.509 authorities.
	X509Authorities [][]byte

	// JWTAuthorities is the serialized JWT authorities.
	JWTAuthorities [][]byte

	// SequenceNumber is the bundle sequence number.
	SequenceNumber uint64

	// RefreshHint is the refresh hint duration.
	RefreshHint time.Duration
}

// SVIDRecord records an issued SVID.
type SVIDRecord struct {
	SPIFFEID  string
	IssuedAt  time.Time
	ExpiresAt time.Time
	Serial    string
}

// JoinTokenRecord records a join token.
type JoinTokenRecord struct {
	TokenHash string
	AgentID   string
	ExpiresAt time.Time
	Used      bool
	UsedAt    time.Time
}

// ReplicationStatus contains replication status information.
type ReplicationStatus struct {
	// Mode is the current replication mode.
	Mode ReplicationMode

	// CurrentVersion is the current state version.
	CurrentVersion uint64

	// LeaderVersion is the leader's state version.
	LeaderVersion uint64

	// Lag is the replication lag.
	Lag time.Duration

	// PeerStatuses contains per-peer replication status.
	PeerStatuses map[string]*PeerReplicationStatus

	// LastError is the last replication error.
	LastError string

	// LastErrorTime is when the last error occurred.
	LastErrorTime time.Time
}

// PeerReplicationStatus contains replication status for a peer.
type PeerReplicationStatus struct {
	NodeID       string
	Version      uint64
	Lag          time.Duration
	InSync       bool
	LastSyncTime time.Time
}

// NewHAIdentityProvider creates a new HA identity provider.
func NewHAIdentityProvider(config *HAConfig, provider *EmbeddedProvider) (*HAIdentityProvider, error) {
	if config == nil {
		config = DefaultHAConfig()
	}

	if !config.Enabled {
		return nil, fmt.Errorf("HA mode not enabled")
	}

	if config.NodeID == "" {
		return nil, fmt.Errorf("node ID required")
	}

	if provider == nil {
		return nil, fmt.Errorf("embedded provider required")
	}

	ha := &HAIdentityProvider{
		config:   config,
		provider: provider,
		state: &HAState{
			Role:  HARoleFollower,
			Peers: make(map[string]*PeerState),
		},
		stopCh: make(chan struct{}),
	}

	// Initialize trust bundle synchronizer
	ha.bundleSync = &TrustBundleSynchronizer{
		ha: ha,
	}

	return ha, nil
}

// SetLeaderElector sets the leader elector implementation.
func (ha *HAIdentityProvider) SetLeaderElector(elector LeaderElector) {
	ha.mu.Lock()
	defer ha.mu.Unlock()
	ha.leaderElector = elector
}

// SetStateReplicator sets the state replicator implementation.
func (ha *HAIdentityProvider) SetStateReplicator(replicator StateReplicator) {
	ha.mu.Lock()
	defer ha.mu.Unlock()
	ha.replicator = replicator
}

// Start starts the HA identity provider.
func (ha *HAIdentityProvider) Start(ctx context.Context) error {
	ha.mu.Lock()
	if ha.leaderElector == nil {
		ha.mu.Unlock()
		return fmt.Errorf("leader elector not set")
	}
	ha.mu.Unlock()

	// Start leader election watcher
	ha.wg.Add(1)
	go ha.watchLeadership(ctx)

	// Start replication loop
	if ha.replicator != nil {
		ha.wg.Add(1)
		go ha.replicationLoop(ctx)
	}

	// Start health check loop
	ha.wg.Add(1)
	go ha.healthCheckLoop(ctx)

	return nil
}

// Stop stops the HA identity provider.
func (ha *HAIdentityProvider) Stop() error {
	close(ha.stopCh)
	ha.wg.Wait()

	// Resign if leader
	if ha.IsLeader() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		return ha.leaderElector.Resign(ctx)
	}

	return nil
}

// IsLeader returns true if this node is the leader.
func (ha *HAIdentityProvider) IsLeader() bool {
	ha.mu.RLock()
	defer ha.mu.RUnlock()
	return ha.state.Role == HARoleLeader
}

// GetState returns the current HA state.
func (ha *HAIdentityProvider) GetState() HAState {
	ha.mu.RLock()
	defer ha.mu.RUnlock()
	return *ha.state
}

// GetProvider returns the underlying provider (only for leader operations).
func (ha *HAIdentityProvider) GetProvider() (*EmbeddedProvider, error) {
	if !ha.IsLeader() {
		return nil, fmt.Errorf("not leader")
	}
	return ha.provider, nil
}

// OnLeaderChange registers a callback for leader changes.
func (ha *HAIdentityProvider) OnLeaderChange(callback func(oldLeader, newLeader string)) {
	ha.mu.Lock()
	defer ha.mu.Unlock()
	ha.onLeaderChange = callback
}

// OnRoleChange registers a callback for role changes.
func (ha *HAIdentityProvider) OnRoleChange(callback func(oldRole, newRole HARole)) {
	ha.mu.Lock()
	defer ha.mu.Unlock()
	ha.onRoleChange = callback
}

// Campaign starts a campaign to become leader.
func (ha *HAIdentityProvider) Campaign(ctx context.Context) error {
	ha.setRole(HARoleCandidate)

	err := ha.leaderElector.Campaign(ctx)
	if err != nil {
		ha.setRole(HARoleFollower)
		return err
	}

	ha.setRole(HARoleLeader)
	return nil
}

// Resign resigns from leadership.
func (ha *HAIdentityProvider) Resign(ctx context.Context) error {
	if !ha.IsLeader() {
		return nil
	}

	err := ha.leaderElector.Resign(ctx)
	if err != nil {
		return err
	}

	ha.setRole(HARoleFollower)
	return nil
}

// IssueX509SVID issues an X.509 SVID (leader only).
func (ha *HAIdentityProvider) IssueX509SVID(ctx context.Context, req *X509SVIDRequest) (*X509SVID, error) {
	if !ha.IsLeader() {
		return nil, fmt.Errorf("not leader: cannot issue SVIDs")
	}

	svid, err := ha.provider.IssueX509SVID(ctx, req)
	if err != nil {
		return nil, err
	}

	// Replicate the issued SVID record
	if ha.replicator != nil {
		record := SVIDRecord{
			SPIFFEID:  svid.SPIFFEID.String(),
			IssuedAt:  svid.IssuedAt,
			ExpiresAt: svid.ExpiresAt,
			Serial:    fmt.Sprintf("%x", svid.Certificates[0].SerialNumber),
		}

		state := &ReplicatedState{
			Version:     ha.getNextVersion(),
			Timestamp:   time.Now(),
			IssuedSVIDs: []SVIDRecord{record},
		}
		state.Checksum = ha.computeChecksum(state)

		_ = ha.replicator.Replicate(ctx, state) // best-effort replication, don't fail SVID issuance
	}

	return svid, nil
}

// GetTrustBundle returns the current trust bundle.
func (ha *HAIdentityProvider) GetTrustBundle(ctx context.Context) (*TrustBundle, error) {
	return ha.provider.GetTrustBundle(ctx)
}

// Attest performs attestation.
func (ha *HAIdentityProvider) Attest(ctx context.Context, evidence *AttestationEvidence) (*AttestationResult, error) {
	if !ha.IsLeader() {
		return nil, fmt.Errorf("not leader: cannot perform attestation")
	}

	return ha.provider.Attest(ctx, evidence)
}

// watchLeadership watches for leadership changes.
func (ha *HAIdentityProvider) watchLeadership(ctx context.Context) {
	defer ha.wg.Done()

	leaderCh, err := ha.leaderElector.Watch(ctx)
	if err != nil {
		return
	}

	for {
		select {
		case <-ctx.Done():
			return
		case <-ha.stopCh:
			return
		case newLeader := <-leaderCh:
			ha.handleLeaderChange(ctx, newLeader)
		}
	}
}

// handleLeaderChange handles a leadership change.
func (ha *HAIdentityProvider) handleLeaderChange(ctx context.Context, newLeaderID string) {
	ha.mu.Lock()
	oldLeader := ha.state.LeaderID
	ha.state.LeaderID = newLeaderID
	ha.state.LastHeartbeat = time.Now()

	// Update role based on new leader
	var newRole HARole
	if newLeaderID == ha.config.NodeID {
		newRole = HARoleLeader
	} else {
		newRole = HARoleFollower
	}

	oldRole := ha.state.Role
	ha.state.Role = newRole

	callback := ha.onLeaderChange
	roleCallback := ha.onRoleChange
	ha.mu.Unlock()

	if callback != nil && oldLeader != newLeaderID {
		callback(oldLeader, newLeaderID)
	}

	if roleCallback != nil && oldRole != newRole {
		roleCallback(oldRole, newRole)
	}

	// If we became leader, sync state from previous leader
	if newRole == HARoleLeader && oldRole != HARoleLeader {
		if ha.replicator != nil {
			syncCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
			defer cancel()
			if state, err := ha.replicator.Sync(syncCtx); err == nil {
				ha.applyReplicatedState(state)
			}
		}
	}
}

// replicationLoop runs the replication loop.
func (ha *HAIdentityProvider) replicationLoop(ctx context.Context) {
	defer ha.wg.Done()

	ticker := time.NewTicker(ha.config.Replication.SyncInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ha.stopCh:
			return
		case <-ticker.C:
			if ha.IsLeader() {
				// Leader replicates state to followers
				ha.replicateToFollowers(ctx)
			} else {
				// Follower syncs from leader
				ha.syncFromLeader(ctx)
			}
		}
	}
}

// replicateToFollowers replicates state to followers.
func (ha *HAIdentityProvider) replicateToFollowers(ctx context.Context) {
	state := ha.buildReplicatedState(ctx)
	_ = ha.replicator.Replicate(ctx, state) // best-effort replication
}

// syncFromLeader syncs state from the leader.
func (ha *HAIdentityProvider) syncFromLeader(ctx context.Context) {
	state, err := ha.replicator.Sync(ctx)
	if err != nil {
		return
	}

	ha.applyReplicatedState(state)
}

// buildReplicatedState builds the current state for replication.
func (ha *HAIdentityProvider) buildReplicatedState(ctx context.Context) *ReplicatedState {
	ha.mu.RLock()
	defer ha.mu.RUnlock()

	state := &ReplicatedState{
		Version:   ha.getNextVersion(),
		Timestamp: time.Now(),
	}

	// Get trust bundle
	if bundle, err := ha.provider.GetTrustBundle(ctx); err == nil {
		state.TrustBundle = &TrustBundleState{
			TrustDomain:    bundle.TrustDomain,
			SequenceNumber: bundle.SequenceNumber,
			RefreshHint:    bundle.RefreshHint,
		}
		for _, cert := range bundle.X509Authorities {
			state.TrustBundle.X509Authorities = append(state.TrustBundle.X509Authorities, cert.Raw)
		}
	}

	state.Checksum = ha.computeChecksum(state)
	return state
}

// applyReplicatedState applies replicated state from leader.
func (ha *HAIdentityProvider) applyReplicatedState(state *ReplicatedState) {
	if state == nil {
		return
	}

	// Verify checksum
	if ha.computeChecksum(state) != state.Checksum {
		return // Corrupted state
	}

	ha.mu.Lock()
	ha.state.LastSyncTime = time.Now()
	ha.mu.Unlock()

	// Note: In a full implementation, this would:
	// - Update local CA state
	// - Update trust bundle
	// - Update join token records
}

// healthCheckLoop runs the health check loop.
func (ha *HAIdentityProvider) healthCheckLoop(ctx context.Context) {
	defer ha.wg.Done()

	ticker := time.NewTicker(ha.config.HealthCheck.CheckInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ha.stopCh:
			return
		case <-ticker.C:
			ha.checkPeerHealth(ctx)
		}
	}
}

// checkPeerHealth checks the health of all peers.
func (ha *HAIdentityProvider) checkPeerHealth(ctx context.Context) {
	ha.mu.Lock()
	defer ha.mu.Unlock()

	for _, peer := range ha.state.Peers {
		// Check if peer has been seen recently
		if time.Since(peer.LastSeen) > ha.config.HealthCheck.Timeout*time.Duration(ha.config.HealthCheck.FailureThreshold) {
			peer.Healthy = false
		}
	}
}

// setRole sets the current role.
func (ha *HAIdentityProvider) setRole(role HARole) {
	ha.mu.Lock()
	oldRole := ha.state.Role
	ha.state.Role = role
	callback := ha.onRoleChange
	ha.mu.Unlock()

	if callback != nil && oldRole != role {
		callback(oldRole, role)
	}
}

// getNextVersion returns the next state version.
func (ha *HAIdentityProvider) getNextVersion() uint64 {
	// In production, this would use a distributed counter
	//nolint:gosec // G115: UnixNano is positive and fits in uint64 until year 2262
	return uint64(time.Now().UnixNano())
}

// computeChecksum computes a checksum for replicated state.
func (ha *HAIdentityProvider) computeChecksum(state *ReplicatedState) string {
	// Create a copy without the checksum field
	stateCopy := *state
	stateCopy.Checksum = ""

	data, _ := json.Marshal(stateCopy)
	hash := sha256.Sum256(data)
	return hex.EncodeToString(hash[:])
}

// TrustBundleSynchronizer ensures trust bundle consistency across nodes.
type TrustBundleSynchronizer struct {
	ha *HAIdentityProvider
	mu sync.RWMutex

	// Local bundle cache
	localBundle *TrustBundle

	// Callbacks
	onBundleUpdate func(bundle *TrustBundle)
}

// SyncBundle synchronizes the trust bundle from the leader.
func (tbs *TrustBundleSynchronizer) SyncBundle(ctx context.Context) error {
	if tbs.ha.IsLeader() {
		// Leader doesn't need to sync
		return nil
	}

	// Get bundle from local provider (which should be kept in sync via replication)
	bundle, err := tbs.ha.provider.GetTrustBundle(ctx)
	if err != nil {
		return err
	}

	tbs.mu.Lock()
	oldBundle := tbs.localBundle
	tbs.localBundle = bundle
	callback := tbs.onBundleUpdate
	tbs.mu.Unlock()

	// Notify if bundle changed
	if callback != nil && (oldBundle == nil || oldBundle.SequenceNumber != bundle.SequenceNumber) {
		callback(bundle)
	}

	return nil
}

// GetBundle returns the current trust bundle.
func (tbs *TrustBundleSynchronizer) GetBundle() *TrustBundle {
	tbs.mu.RLock()
	defer tbs.mu.RUnlock()
	return tbs.localBundle
}

// OnBundleUpdate registers a callback for bundle updates.
func (tbs *TrustBundleSynchronizer) OnBundleUpdate(callback func(bundle *TrustBundle)) {
	tbs.mu.Lock()
	defer tbs.mu.Unlock()
	tbs.onBundleUpdate = callback
}
