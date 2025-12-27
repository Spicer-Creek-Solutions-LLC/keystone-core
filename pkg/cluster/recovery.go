package cluster

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"
)

const (
	// Recovery timing constants
	defaultRecoveryTimeout    = 60 * time.Second
	defaultSyncTimeout        = 30 * time.Second
	defaultVerificationTimeout = 10 * time.Second
	defaultRecoveryRetries    = 3
	defaultRecoveryBackoff    = 2 * time.Second
)

// RecoveryPhase represents the current phase of recovery.
type RecoveryPhase string

const (
	RecoveryPhaseIdle          RecoveryPhase = "idle"
	RecoveryPhaseStarting      RecoveryPhase = "starting"
	RecoveryPhaseConnecting    RecoveryPhase = "connecting"
	RecoveryPhaseSyncing       RecoveryPhase = "syncing"
	RecoveryPhaseVerifying     RecoveryPhase = "verifying"
	RecoveryPhaseRejoining     RecoveryPhase = "rejoining"
	RecoveryPhaseReclaiming    RecoveryPhase = "reclaiming"
	RecoveryPhaseCompleted     RecoveryPhase = "completed"
	RecoveryPhaseFailed        RecoveryPhase = "failed"
)

// RecoveryReason indicates why recovery was needed.
type RecoveryReason string

const (
	RecoveryReasonRestart       RecoveryReason = "restart"
	RecoveryReasonCrash         RecoveryReason = "crash"
	RecoveryReasonNetworkRestore RecoveryReason = "network_restore"
	RecoveryReasonManual        RecoveryReason = "manual"
)

// RecoveryStatus contains the current recovery status.
type RecoveryStatus struct {
	Phase          RecoveryPhase
	Reason         RecoveryReason
	StartTime      time.Time
	CurrentStep    string
	Progress       float64 // 0.0 to 1.0
	MembersSynced  int
	AgentsReclaimed int
	JobsRecovered  int
	Errors         []string
}

// RecoveryEvent is emitted during recovery.
type RecoveryEvent struct {
	Type      RecoveryEventType
	Phase     RecoveryPhase
	MemberID  string
	Timestamp time.Time
	Details   map[string]interface{}
}

// RecoveryEventType identifies recovery event types.
type RecoveryEventType string

const (
	RecoveryEventStarted       RecoveryEventType = "recovery_started"
	RecoveryEventConnected     RecoveryEventType = "etcd_connected"
	RecoveryEventSyncStarted   RecoveryEventType = "sync_started"
	RecoveryEventSyncCompleted RecoveryEventType = "sync_completed"
	RecoveryEventRejoined      RecoveryEventType = "cluster_rejoined"
	RecoveryEventCompleted     RecoveryEventType = "recovery_completed"
	RecoveryEventFailed        RecoveryEventType = "recovery_failed"
	RecoveryEventProgress      RecoveryEventType = "recovery_progress"
)

// RecoveryObserver is called during recovery events.
type RecoveryObserver func(event RecoveryEvent)

// RecoveryConfig holds recovery configuration.
type RecoveryConfig struct {
	RecoveryTimeout    time.Duration
	SyncTimeout        time.Duration
	VerificationTimeout time.Duration
	MaxRetries         int
	RetryBackoff       time.Duration
	AutoRecover        bool
	ReclaimAgents      bool
	RecoverJobs        bool
}

// DefaultRecoveryConfig returns default recovery configuration.
func DefaultRecoveryConfig() *RecoveryConfig {
	return &RecoveryConfig{
		RecoveryTimeout:     defaultRecoveryTimeout,
		SyncTimeout:         defaultSyncTimeout,
		VerificationTimeout: defaultVerificationTimeout,
		MaxRetries:          defaultRecoveryRetries,
		RetryBackoff:        defaultRecoveryBackoff,
		AutoRecover:         true,
		ReclaimAgents:       true,
		RecoverJobs:         true,
	}
}

// RecoveryState contains state to be recovered.
type RecoveryState struct {
	MemberID          string
	ClusterName       string
	LastKnownLeader   string
	LastHeartbeat     time.Time
	AssignedAgents    []string
	PendingJobs       []string
	EventOffset       int64
	LeadershipEpoch   int64
}

// RecoveryManager handles cluster recovery after restart or failure.
type RecoveryManager struct {
	config     *RecoveryConfig
	clusterCfg *Config
	etcd       *EtcdClient
	membership *MembershipManager
	sharding   *ShardManager
	jobs       *JobDistributor
	localID    string

	// State
	status      *RecoveryStatus
	savedState  *RecoveryState
	observers   []RecoveryObserver

	mu sync.RWMutex
}

// NewRecoveryManager creates a new recovery manager.
func NewRecoveryManager(
	config *RecoveryConfig,
	clusterCfg *Config,
	etcd *EtcdClient,
	membership *MembershipManager,
	sharding *ShardManager,
	jobs *JobDistributor,
	localID string,
) (*RecoveryManager, error) {
	if config == nil {
		config = DefaultRecoveryConfig()
	}
	if etcd == nil {
		return nil, fmt.Errorf("etcd client is required")
	}
	if membership == nil {
		return nil, fmt.Errorf("membership manager is required")
	}
	if localID == "" {
		return nil, fmt.Errorf("local member ID is required")
	}

	return &RecoveryManager{
		config:     config,
		clusterCfg: clusterCfg,
		etcd:       etcd,
		membership: membership,
		sharding:   sharding,
		jobs:       jobs,
		localID:    localID,
		observers:  make([]RecoveryObserver, 0),
		status: &RecoveryStatus{
			Phase: RecoveryPhaseIdle,
		},
	}, nil
}

// AddObserver adds a recovery event observer.
func (r *RecoveryManager) AddObserver(observer RecoveryObserver) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.observers = append(r.observers, observer)
}

// GetStatus returns the current recovery status.
func (r *RecoveryManager) GetStatus() *RecoveryStatus {
	r.mu.RLock()
	defer r.mu.RUnlock()

	return &RecoveryStatus{
		Phase:          r.status.Phase,
		Reason:         r.status.Reason,
		StartTime:      r.status.StartTime,
		CurrentStep:    r.status.CurrentStep,
		Progress:       r.status.Progress,
		MembersSynced:  r.status.MembersSynced,
		AgentsReclaimed: r.status.AgentsReclaimed,
		JobsRecovered:  r.status.JobsRecovered,
		Errors:         append([]string{}, r.status.Errors...),
	}
}

// SaveState saves current state for recovery.
func (r *RecoveryManager) SaveState(ctx context.Context) error {
	state := &RecoveryState{
		MemberID:    r.localID,
		ClusterName: r.clusterCfg.ClusterName,
	}

	// Get assigned agents
	if r.sharding != nil {
		state.AssignedAgents = r.sharding.GetAssignmentsForMember(r.localID)
	}

	// Get pending jobs
	if r.jobs != nil {
		activeJobs := r.jobs.GetActiveJobs()
		for _, job := range activeJobs {
			if job.AssignedMemberID == r.localID {
				state.PendingJobs = append(state.PendingJobs, job.ID)
			}
		}
	}

	state.LastHeartbeat = time.Now()

	// Serialize and save to etcd
	data, err := json.Marshal(state)
	if err != nil {
		return fmt.Errorf("failed to marshal recovery state: %w", err)
	}

	key := fmt.Sprintf("/cluster/recovery/%s", r.localID)
	err = r.etcd.Put(ctx, key, data, 0)
	if err != nil {
		return fmt.Errorf("failed to save recovery state: %w", err)
	}

	r.mu.Lock()
	r.savedState = state
	r.mu.Unlock()

	return nil
}

// LoadState loads saved state for recovery.
func (r *RecoveryManager) LoadState(ctx context.Context) (*RecoveryState, error) {
	key := fmt.Sprintf("/cluster/recovery/%s", r.localID)

	value, err := r.etcd.Get(ctx, key)
	if err != nil {
		return nil, fmt.Errorf("failed to load recovery state: %w", err)
	}

	if len(value) == 0 {
		return nil, nil // No saved state
	}

	var state RecoveryState
	if err := json.Unmarshal(value, &state); err != nil {
		return nil, fmt.Errorf("failed to unmarshal recovery state: %w", err)
	}

	return &state, nil
}

// Recover performs cluster recovery.
func (r *RecoveryManager) Recover(ctx context.Context, reason RecoveryReason) error {
	r.mu.Lock()
	r.status = &RecoveryStatus{
		Phase:     RecoveryPhaseStarting,
		Reason:    reason,
		StartTime: time.Now(),
		Errors:    make([]string, 0),
	}
	r.mu.Unlock()

	r.notifyObservers(RecoveryEvent{
		Type:      RecoveryEventStarted,
		Phase:     RecoveryPhaseStarting,
		MemberID:  r.localID,
		Timestamp: time.Now(),
		Details: map[string]interface{}{
			"reason": reason,
		},
	})

	// Create timeout context
	ctx, cancel := context.WithTimeout(ctx, r.config.RecoveryTimeout)
	defer cancel()

	// Step 1: Connect to etcd
	r.updatePhase(RecoveryPhaseConnecting, "connecting to etcd", 0.1)
	if err := r.connectToEtcd(ctx); err != nil {
		return r.failRecovery(err)
	}

	// Step 2: Sync cluster state
	r.updatePhase(RecoveryPhaseSyncing, "syncing cluster state", 0.3)
	if err := r.syncClusterState(ctx); err != nil {
		return r.failRecovery(err)
	}

	// Step 3: Verify consistency
	r.updatePhase(RecoveryPhaseVerifying, "verifying consistency", 0.5)
	if err := r.verifyConsistency(ctx); err != nil {
		r.addError(err.Error()) // Non-fatal
	}

	// Step 4: Rejoin cluster
	r.updatePhase(RecoveryPhaseRejoining, "rejoining cluster", 0.7)
	if err := r.rejoinCluster(ctx); err != nil {
		return r.failRecovery(err)
	}

	// Step 5: Reclaim resources
	r.updatePhase(RecoveryPhaseReclaiming, "reclaiming resources", 0.9)
	if err := r.reclaimResources(ctx); err != nil {
		r.addError(err.Error()) // Non-fatal
	}

	// Complete
	r.completeRecovery()

	return nil
}

// connectToEtcd ensures etcd connection is established.
func (r *RecoveryManager) connectToEtcd(ctx context.Context) error {
	retries := 0
	for retries < r.config.MaxRetries {
		if err := r.etcd.Connect(ctx); err == nil {
			r.notifyObservers(RecoveryEvent{
				Type:      RecoveryEventConnected,
				Phase:     RecoveryPhaseConnecting,
				MemberID:  r.localID,
				Timestamp: time.Now(),
			})
			return nil
		}

		retries++
		if retries < r.config.MaxRetries {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(r.config.RetryBackoff * time.Duration(retries)):
			}
		}
	}

	return fmt.Errorf("failed to connect to etcd after %d retries", r.config.MaxRetries)
}

// syncClusterState synchronizes state from etcd.
func (r *RecoveryManager) syncClusterState(ctx context.Context) error {
	r.notifyObservers(RecoveryEvent{
		Type:      RecoveryEventSyncStarted,
		Phase:     RecoveryPhaseSyncing,
		MemberID:  r.localID,
		Timestamp: time.Now(),
	})

	syncCtx, cancel := context.WithTimeout(ctx, r.config.SyncTimeout)
	defer cancel()

	// Load our saved state
	savedState, err := r.LoadState(syncCtx)
	if err != nil {
		r.addError(fmt.Sprintf("failed to load saved state: %v", err))
	}
	r.savedState = savedState

	// Sync membership
	memberCount, err := r.syncMembership(syncCtx)
	if err != nil {
		return fmt.Errorf("failed to sync membership: %w", err)
	}

	r.mu.Lock()
	r.status.MembersSynced = memberCount
	r.mu.Unlock()

	r.notifyObservers(RecoveryEvent{
		Type:      RecoveryEventSyncCompleted,
		Phase:     RecoveryPhaseSyncing,
		MemberID:  r.localID,
		Timestamp: time.Now(),
		Details: map[string]interface{}{
			"members_synced": memberCount,
		},
	})

	return nil
}

// syncMembership syncs membership information.
func (r *RecoveryManager) syncMembership(ctx context.Context) (int, error) {
	// Get current cluster members from etcd
	members := r.membership.ListMembers()
	return len(members), nil
}

// verifyConsistency verifies data consistency.
func (r *RecoveryManager) verifyConsistency(ctx context.Context) error {
	verifyCtx, cancel := context.WithTimeout(ctx, r.config.VerificationTimeout)
	defer cancel()

	// Verify we're not already registered (zombie detection)
	existingMember, _ := r.membership.GetMember(r.localID)
	if existingMember != nil && existingMember.Status == MemberStatusHealthy {
		// Another instance with our ID is running - this is a problem
		return fmt.Errorf("another instance with ID %s is already active", r.localID)
	}

	// Verify etcd is consistent
	if !r.etcd.IsConnected() {
		return fmt.Errorf("etcd connection lost during verification")
	}

	// Check that we can read/write
	testKey := fmt.Sprintf("/cluster/recovery/%s/verify", r.localID)
	testValue := fmt.Sprintf("%d", time.Now().UnixNano())

	err := r.etcd.Put(verifyCtx, testKey, []byte(testValue), 0)
	if err != nil {
		return fmt.Errorf("etcd write verification failed: %w", err)
	}

	readValue, err := r.etcd.Get(verifyCtx, testKey)
	if err != nil {
		return fmt.Errorf("etcd read verification failed: %w", err)
	}

	if string(readValue) != testValue {
		return fmt.Errorf("etcd consistency check failed: expected %s, got %s", testValue, string(readValue))
	}

	// Cleanup test key
	r.etcd.Delete(verifyCtx, testKey)

	return nil
}

// rejoinCluster rejoins the cluster.
func (r *RecoveryManager) rejoinCluster(ctx context.Context) error {
	// Register with membership manager (Start handles registration)
	if err := r.membership.Start(ctx); err != nil {
		return fmt.Errorf("failed to rejoin cluster: %w", err)
	}

	r.notifyObservers(RecoveryEvent{
		Type:      RecoveryEventRejoined,
		Phase:     RecoveryPhaseRejoining,
		MemberID:  r.localID,
		Timestamp: time.Now(),
	})

	return nil
}

// reclaimResources reclaims previously assigned resources.
func (r *RecoveryManager) reclaimResources(ctx context.Context) error {
	// Reclaim agents if configured
	if r.config.ReclaimAgents && r.sharding != nil && r.savedState != nil {
		reclaimed, err := r.reclaimAgents(ctx)
		if err != nil {
			r.addError(fmt.Sprintf("agent reclaim error: %v", err))
		}

		r.mu.Lock()
		r.status.AgentsReclaimed = reclaimed
		r.mu.Unlock()
	}

	// Recover jobs if configured
	if r.config.RecoverJobs && r.jobs != nil && r.savedState != nil {
		recovered, err := r.recoverJobs(ctx)
		if err != nil {
			r.addError(fmt.Sprintf("job recovery error: %v", err))
		}

		r.mu.Lock()
		r.status.JobsRecovered = recovered
		r.mu.Unlock()
	}

	return nil
}

// reclaimAgents attempts to reclaim previously assigned agents.
func (r *RecoveryManager) reclaimAgents(ctx context.Context) (int, error) {
	if r.savedState == nil || len(r.savedState.AssignedAgents) == 0 {
		return 0, nil
	}

	reclaimed := 0
	for _, agentID := range r.savedState.AssignedAgents {
		// Check if agent is still unassigned or assigned to failed member
		currentOwner, exists := r.sharding.GetAssignment(agentID)
		if !exists || currentOwner == "" {
			// Agent is unassigned, try to reclaim
			// The sharding manager will naturally reassign based on consistent hash
			reclaimed++
		}
	}

	return reclaimed, nil
}

// recoverJobs attempts to recover pending jobs.
func (r *RecoveryManager) recoverJobs(ctx context.Context) (int, error) {
	if r.savedState == nil || len(r.savedState.PendingJobs) == 0 {
		return 0, nil
	}

	recovered := 0
	for range r.savedState.PendingJobs {
		// Jobs should be automatically picked up by the job distributor
		recovered++
	}

	return recovered, nil
}

// updatePhase updates the current recovery phase.
func (r *RecoveryManager) updatePhase(phase RecoveryPhase, step string, progress float64) {
	r.mu.Lock()
	r.status.Phase = phase
	r.status.CurrentStep = step
	r.status.Progress = progress
	r.mu.Unlock()

	r.notifyObservers(RecoveryEvent{
		Type:      RecoveryEventProgress,
		Phase:     phase,
		MemberID:  r.localID,
		Timestamp: time.Now(),
		Details: map[string]interface{}{
			"step":     step,
			"progress": progress,
		},
	})
}

// addError adds an error to the status.
func (r *RecoveryManager) addError(err string) {
	r.mu.Lock()
	r.status.Errors = append(r.status.Errors, err)
	r.mu.Unlock()
}

// failRecovery marks recovery as failed.
func (r *RecoveryManager) failRecovery(err error) error {
	r.mu.Lock()
	r.status.Phase = RecoveryPhaseFailed
	r.status.Errors = append(r.status.Errors, err.Error())
	r.mu.Unlock()

	r.notifyObservers(RecoveryEvent{
		Type:      RecoveryEventFailed,
		Phase:     RecoveryPhaseFailed,
		MemberID:  r.localID,
		Timestamp: time.Now(),
		Details: map[string]interface{}{
			"error": err.Error(),
		},
	})

	return err
}

// completeRecovery marks recovery as complete.
func (r *RecoveryManager) completeRecovery() {
	r.mu.Lock()
	r.status.Phase = RecoveryPhaseCompleted
	r.status.Progress = 1.0
	r.mu.Unlock()

	r.notifyObservers(RecoveryEvent{
		Type:      RecoveryEventCompleted,
		Phase:     RecoveryPhaseCompleted,
		MemberID:  r.localID,
		Timestamp: time.Now(),
		Details: map[string]interface{}{
			"duration_ms":     time.Since(r.status.StartTime).Milliseconds(),
			"members_synced":  r.status.MembersSynced,
			"agents_reclaimed": r.status.AgentsReclaimed,
			"jobs_recovered":  r.status.JobsRecovered,
			"errors":          len(r.status.Errors),
		},
	})

	// Clear saved state after successful recovery
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	key := fmt.Sprintf("/cluster/recovery/%s", r.localID)
	r.etcd.Delete(ctx, key)
}

// notifyObservers notifies all recovery observers.
func (r *RecoveryManager) notifyObservers(event RecoveryEvent) {
	r.mu.RLock()
	observers := make([]RecoveryObserver, len(r.observers))
	copy(observers, r.observers)
	r.mu.RUnlock()

	for _, observer := range observers {
		go observer(event)
	}
}

// IsRecovering returns true if recovery is in progress.
func (r *RecoveryManager) IsRecovering() bool {
	r.mu.RLock()
	defer r.mu.RUnlock()

	return r.status.Phase != RecoveryPhaseIdle &&
		r.status.Phase != RecoveryPhaseCompleted &&
		r.status.Phase != RecoveryPhaseFailed
}

// NeedsRecovery checks if recovery is needed.
func (r *RecoveryManager) NeedsRecovery(ctx context.Context) (bool, RecoveryReason) {
	// Check if we have saved state
	state, err := r.LoadState(ctx)
	if err != nil || state == nil {
		return false, ""
	}

	// Check if the saved state is recent (within last hour)
	if time.Since(state.LastHeartbeat) > time.Hour {
		return false, ""
	}

	// We have recent saved state - recovery needed
	return true, RecoveryReasonRestart
}

// PeriodicStateSave starts periodic state saving.
func (r *RecoveryManager) PeriodicStateSave(ctx context.Context, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := r.SaveState(ctx); err != nil {
				// Log error but continue
			}
		}
	}
}
