package cluster

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/shawnbutts/keystone-core/pkg/wait"
)

const (
	// Failover timing constants
	defaultFailoverTimeout    = 30 * time.Second
	defaultAgentReassignBatch = 100
	defaultJobReassignBatch   = 50
	defaultFailoverCooldown   = 10 * time.Second
	defaultMaxConcurrentFails = 2
)

// FailoverState represents the current state of a failover operation.
type FailoverState string

const (
	FailoverStateIdle       FailoverState = "idle"
	FailoverStateDetecting  FailoverState = "detecting"
	FailoverStateInitiated  FailoverState = "initiated"
	FailoverStateInProgress FailoverState = "in_progress"
	FailoverStateCompleted  FailoverState = "completed"
	FailoverStateFailed     FailoverState = "failed"
	FailoverStateRolledBack FailoverState = "rolled_back"
)

// FailoverReason indicates why failover was triggered.
type FailoverReason string

const (
	FailoverReasonHeartbeatLoss    FailoverReason = "heartbeat_loss"
	FailoverReasonHealthCheck      FailoverReason = "health_check_failed"
	FailoverReasonManualTrigger    FailoverReason = "manual_trigger"
	FailoverReasonGracefulDrain    FailoverReason = "graceful_drain"
	FailoverReasonNetworkPartition FailoverReason = "network_partition"
)

// FailoverOperation represents a single failover operation.
type FailoverOperation struct {
	ID               string
	FailedMemberID   string
	Reason           FailoverReason
	State            FailoverState
	StartTime        time.Time
	EndTime          *time.Time
	AgentsReassigned int
	JobsReassigned   int
	EventsReplayed   int
	Error            error
	Steps            []*FailoverStep
}

// FailoverStep represents a single step in the failover process.
type FailoverStep struct {
	Name      string
	Status    FailoverState
	StartTime time.Time
	EndTime   *time.Time
	Error     error
	Details   map[string]interface{}
}

// FailoverEvent is emitted during failover operations.
type FailoverEvent struct {
	Type        FailoverEventType
	OperationID string
	MemberID    string
	State       FailoverState
	Reason      FailoverReason
	Timestamp   time.Time
	Details     map[string]interface{}
}

// FailoverEventType identifies failover event types.
type FailoverEventType string

const (
	FailoverEventStarted    FailoverEventType = "failover_started"
	FailoverEventProgress   FailoverEventType = "failover_progress"
	FailoverEventCompleted  FailoverEventType = "failover_completed"
	FailoverEventFailed     FailoverEventType = "failover_failed"
	FailoverEventAgentMoved FailoverEventType = "agent_moved"
	FailoverEventJobMoved   FailoverEventType = "job_moved"
)

// FailoverObserver is called during failover events.
type FailoverObserver func(event FailoverEvent)

// FailoverManager handles automatic failover when members fail.
type FailoverManager struct {
	config     *Config
	etcd       *EtcdClient
	membership *MembershipManager
	health     *HealthMonitor
	sharding   *ShardManager
	jobs       *JobDistributor
	localID    string

	// Active failovers
	activeFailovers map[string]*FailoverOperation
	failoverHistory []*FailoverOperation

	// Observers
	observers []FailoverObserver

	// Rate limiting
	lastFailoverTime time.Time
	failoverCount    int

	mu       sync.RWMutex
	stopChan chan struct{}
	doneChan chan struct{}
	started  bool
}

// NewFailoverManager creates a new failover manager.
func NewFailoverManager(
	config *Config,
	etcd *EtcdClient,
	membership *MembershipManager,
	health *HealthMonitor,
	sharding *ShardManager,
	jobs *JobDistributor,
	localID string,
) (*FailoverManager, error) {
	if config == nil {
		return nil, fmt.Errorf("config is required")
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

	return &FailoverManager{
		config:          config,
		etcd:            etcd,
		membership:      membership,
		health:          health,
		sharding:        sharding,
		jobs:            jobs,
		localID:         localID,
		activeFailovers: make(map[string]*FailoverOperation),
		failoverHistory: make([]*FailoverOperation, 0),
		observers:       make([]FailoverObserver, 0),
		stopChan:        make(chan struct{}),
		doneChan:        make(chan struct{}),
	}, nil
}

// Start begins failover monitoring.
func (f *FailoverManager) Start(ctx context.Context) error {
	f.mu.Lock()
	if f.started {
		f.mu.Unlock()
		return fmt.Errorf("failover manager already started")
	}
	f.started = true
	f.mu.Unlock()

	// Subscribe to health events
	if f.health != nil {
		f.health.AddObserver(f.onHealthEvent)
	}

	// Subscribe to membership events
	f.membership.AddObserver(f.onMembershipEvent)

	go f.monitorLoop(ctx)

	return nil
}

// Stop stops the failover manager.
func (f *FailoverManager) Stop() error {
	f.mu.Lock()
	if !f.started {
		f.mu.Unlock()
		return nil
	}
	f.started = false
	f.mu.Unlock()

	close(f.stopChan)

	wait.ForSignal(f.doneChan, 5*time.Second)

	return nil
}

// AddObserver adds a failover event observer.
func (f *FailoverManager) AddObserver(observer FailoverObserver) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.observers = append(f.observers, observer)
}

// TriggerFailover manually triggers failover for a member.
func (f *FailoverManager) TriggerFailover(ctx context.Context, memberID string, reason FailoverReason) (*FailoverOperation, error) {
	f.mu.Lock()

	// Check if failover already in progress for this member
	if _, exists := f.activeFailovers[memberID]; exists {
		f.mu.Unlock()
		return nil, fmt.Errorf("failover already in progress for member %s", memberID)
	}

	// Check cooldown
	if time.Since(f.lastFailoverTime) < defaultFailoverCooldown {
		f.mu.Unlock()
		return nil, fmt.Errorf("failover cooldown in effect, wait %v", defaultFailoverCooldown-time.Since(f.lastFailoverTime))
	}

	// Check concurrent failover limit
	if len(f.activeFailovers) >= defaultMaxConcurrentFails {
		f.mu.Unlock()
		return nil, fmt.Errorf("too many concurrent failovers (%d)", len(f.activeFailovers))
	}

	// Create failover operation
	op := &FailoverOperation{
		ID:             fmt.Sprintf("fo-%s-%d", memberID, time.Now().UnixNano()),
		FailedMemberID: memberID,
		Reason:         reason,
		State:          FailoverStateInitiated,
		StartTime:      time.Now(),
		Steps:          make([]*FailoverStep, 0),
	}

	f.activeFailovers[memberID] = op
	f.lastFailoverTime = time.Now()
	f.failoverCount++
	f.mu.Unlock()

	// Notify observers
	f.notifyObservers(FailoverEvent{
		Type:        FailoverEventStarted,
		OperationID: op.ID,
		MemberID:    memberID,
		State:       op.State,
		Reason:      reason,
		Timestamp:   time.Now(),
	})

	// Execute failover
	go f.executeFailover(ctx, op)

	return op, nil
}

// GetActiveFailovers returns currently active failover operations.
func (f *FailoverManager) GetActiveFailovers() []*FailoverOperation {
	f.mu.RLock()
	defer f.mu.RUnlock()

	result := make([]*FailoverOperation, 0, len(f.activeFailovers))
	for _, op := range f.activeFailovers {
		result = append(result, op)
	}

	return result
}

// GetFailoverHistory returns recent failover operations.
func (f *FailoverManager) GetFailoverHistory(limit int) []*FailoverOperation {
	f.mu.RLock()
	defer f.mu.RUnlock()

	if limit <= 0 || limit > len(f.failoverHistory) {
		limit = len(f.failoverHistory)
	}

	// Return most recent first
	result := make([]*FailoverOperation, limit)
	start := len(f.failoverHistory) - limit
	copy(result, f.failoverHistory[start:])

	return result
}

// onHealthEvent handles health events.
func (f *FailoverManager) onHealthEvent(event HealthEvent) {
	if event.Type == HealthEventMemberFailed {
		// Trigger automatic failover
		ctx, cancel := context.WithTimeout(context.Background(), defaultFailoverTimeout)
		defer cancel()

		_, err := f.TriggerFailover(ctx, event.MemberID, FailoverReasonHeartbeatLoss)
		if err != nil {
			// Log error but don't fail - failover may already be in progress
		}
	}
}

// onMembershipEvent handles membership events.
func (f *FailoverManager) onMembershipEvent(event MembershipEvent) {
	if event.Type == MembershipEventFailed {
		// Member was removed due to failure - ensure failover is triggered
		ctx, cancel := context.WithTimeout(context.Background(), defaultFailoverTimeout)
		defer cancel()

		f.TriggerFailover(ctx, event.Member.ID, FailoverReasonHeartbeatLoss)
	}
}

// monitorLoop monitors for failover completion.
func (f *FailoverManager) monitorLoop(ctx context.Context) {
	defer close(f.doneChan)

	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-f.stopChan:
			return
		case <-ticker.C:
			f.checkFailoverTimeouts()
		}
	}
}

// checkFailoverTimeouts checks for stuck failovers.
func (f *FailoverManager) checkFailoverTimeouts() {
	f.mu.Lock()
	defer f.mu.Unlock()

	for memberID, op := range f.activeFailovers {
		if time.Since(op.StartTime) > defaultFailoverTimeout {
			op.State = FailoverStateFailed
			op.Error = fmt.Errorf("failover timed out after %v", defaultFailoverTimeout)
			now := time.Now()
			op.EndTime = &now

			f.failoverHistory = append(f.failoverHistory, op)
			delete(f.activeFailovers, memberID)

			go f.notifyObservers(FailoverEvent{
				Type:        FailoverEventFailed,
				OperationID: op.ID,
				MemberID:    memberID,
				State:       op.State,
				Timestamp:   time.Now(),
				Details: map[string]interface{}{
					"error": op.Error.Error(),
				},
			})
		}
	}
}

// executeFailover runs the failover operation.
func (f *FailoverManager) executeFailover(ctx context.Context, op *FailoverOperation) {
	defer f.completeFailover(op)

	op.State = FailoverStateInProgress

	// Step 1: Verify member is actually failed
	step := f.addStep(op, "verify_failure")
	if !f.verifyMemberFailed(ctx, op.FailedMemberID) {
		step.Status = FailoverStateFailed
		step.Error = fmt.Errorf("member %s is not failed", op.FailedMemberID)
		now := time.Now()
		step.EndTime = &now
		op.State = FailoverStateRolledBack
		op.Error = step.Error
		return
	}
	f.completeStep(step)

	// Step 2: Acquire distributed lock for failover
	step = f.addStep(op, "acquire_lock")
	lock, err := f.acquireFailoverLock(ctx, op.FailedMemberID)
	if err != nil {
		f.failStep(step, err)
		op.State = FailoverStateFailed
		op.Error = err
		return
	}
	defer lock.Unlock(ctx)
	f.completeStep(step)

	// Step 3: Reassign agents
	step = f.addStep(op, "reassign_agents")
	agentsReassigned, err := f.reassignAgents(ctx, op)
	if err != nil {
		f.failStep(step, err)
		op.State = FailoverStateFailed
		op.Error = err
		return
	}
	op.AgentsReassigned = agentsReassigned
	step.Details = map[string]interface{}{"count": agentsReassigned}
	f.completeStep(step)

	// Step 4: Reassign jobs
	step = f.addStep(op, "reassign_jobs")
	jobsReassigned, err := f.reassignJobs(ctx, op)
	if err != nil {
		f.failStep(step, err)
		// Continue despite job reassignment errors - agents are more important
	} else {
		op.JobsReassigned = jobsReassigned
		step.Details = map[string]interface{}{"count": jobsReassigned}
		f.completeStep(step)
	}

	// Step 5: Handle event partitions
	step = f.addStep(op, "reassign_events")
	eventsReplayed, err := f.reassignEventPartitions(ctx, op)
	if err != nil {
		f.failStep(step, err)
		// Continue despite event errors
	} else {
		op.EventsReplayed = eventsReplayed
		step.Details = map[string]interface{}{"count": eventsReplayed}
		f.completeStep(step)
	}

	// Step 6: Update cluster state
	step = f.addStep(op, "update_state")
	if err := f.updateClusterState(ctx, op); err != nil {
		f.failStep(step, err)
		// Non-fatal - state will eventually be consistent
	} else {
		f.completeStep(step)
	}

	op.State = FailoverStateCompleted
}

// verifyMemberFailed checks if the member is truly failed.
func (f *FailoverManager) verifyMemberFailed(ctx context.Context, memberID string) bool {
	// Check health status
	if f.health != nil {
		health, err := f.health.GetMemberHealth(memberID)
		if err == nil && health.Status != HealthStatusUnhealthy {
			return false
		}
	}

	// Check if member still exists in membership
	member, _ := f.membership.GetMember(memberID)
	if member != nil && member.Status == MemberStatusHealthy {
		return false
	}

	return true
}

// acquireFailoverLock acquires a distributed lock for failover.
func (f *FailoverManager) acquireFailoverLock(ctx context.Context, memberID string) (*DistributedLock, error) {
	lockKey := fmt.Sprintf("failover/%s", memberID)
	lock := NewDistributedLock(f.etcd, lockKey)

	if err := lock.Lock(ctx); err != nil {
		return nil, fmt.Errorf("failed to acquire failover lock: %w", err)
	}

	return lock, nil
}

// reassignAgents reassigns agents from the failed member.
func (f *FailoverManager) reassignAgents(ctx context.Context, op *FailoverOperation) (int, error) {
	if f.sharding == nil {
		return 0, nil
	}

	// Get agents assigned to failed member
	assignments := f.sharding.GetAssignmentsForMember(op.FailedMemberID)
	if len(assignments) == 0 {
		return 0, nil
	}

	// Find a healthy member to reassign to
	members := f.membership.ListMembers()
	var targetMemberID string
	for _, member := range members {
		if member.ID != op.FailedMemberID && member.Status == MemberStatusHealthy {
			targetMemberID = member.ID
			break
		}
	}

	if targetMemberID == "" {
		return 0, fmt.Errorf("no healthy member to reassign agents to")
	}

	reassigned := 0
	for _, agentID := range assignments {
		// Reassign to healthy member
		if err := f.sharding.ReassignAgent(ctx, agentID, targetMemberID); err != nil {
			continue // Skip this agent but continue with others
		}

		reassigned++

		// Notify progress
		if reassigned%defaultAgentReassignBatch == 0 {
			f.notifyObservers(FailoverEvent{
				Type:        FailoverEventProgress,
				OperationID: op.ID,
				MemberID:    op.FailedMemberID,
				State:       op.State,
				Timestamp:   time.Now(),
				Details: map[string]interface{}{
					"step":       "reassign_agents",
					"reassigned": reassigned,
					"total":      len(assignments),
				},
			})
		}
	}

	return reassigned, nil
}

// reassignJobs reassigns pending jobs from the failed member.
func (f *FailoverManager) reassignJobs(ctx context.Context, op *FailoverOperation) (int, error) {
	if f.jobs == nil {
		return 0, nil
	}

	// Get active jobs on failed member
	activeJobs := f.jobs.GetActiveJobs()
	reassigned := 0

	for _, job := range activeJobs {
		if job.AssignedMemberID == op.FailedMemberID {
			// Reset job to pending for redistribution
			job.Status = JobStatusPending
			job.AssignedMemberID = ""
			job.RetryCount++

			// The job distributor will pick it up and reassign
			reassigned++

			f.notifyObservers(FailoverEvent{
				Type:        FailoverEventJobMoved,
				OperationID: op.ID,
				MemberID:    op.FailedMemberID,
				Timestamp:   time.Now(),
				Details: map[string]interface{}{
					"job_id": job.ID,
				},
			})
		}
	}

	return reassigned, nil
}

// reassignEventPartitions handles event partition reassignment.
func (f *FailoverManager) reassignEventPartitions(ctx context.Context, op *FailoverOperation) (int, error) {
	// Event partitions will be automatically rebalanced by the EventDistributor
	// when it detects the member is gone. We just need to ensure offsets are preserved.
	return 0, nil
}

// updateClusterState updates cluster state after failover.
func (f *FailoverManager) updateClusterState(ctx context.Context, op *FailoverOperation) error {
	// Record failover in etcd for audit
	key := fmt.Sprintf("/cluster/failovers/%s", op.ID)
	data := fmt.Sprintf(`{"member_id":"%s","reason":"%s","agents":%d,"jobs":%d,"time":"%s"}`,
		op.FailedMemberID, op.Reason, op.AgentsReassigned, op.JobsReassigned, op.StartTime.Format(time.RFC3339))

	return f.etcd.Put(ctx, key, []byte(data), 0)
}

// completeFailover marks failover as complete.
func (f *FailoverManager) completeFailover(op *FailoverOperation) {
	now := time.Now()
	op.EndTime = &now

	f.mu.Lock()
	f.failoverHistory = append(f.failoverHistory, op)
	delete(f.activeFailovers, op.FailedMemberID)

	// Trim history if too large
	if len(f.failoverHistory) > 100 {
		f.failoverHistory = f.failoverHistory[len(f.failoverHistory)-100:]
	}
	f.mu.Unlock()

	eventType := FailoverEventCompleted
	if op.State == FailoverStateFailed {
		eventType = FailoverEventFailed
	}

	f.notifyObservers(FailoverEvent{
		Type:        eventType,
		OperationID: op.ID,
		MemberID:    op.FailedMemberID,
		State:       op.State,
		Timestamp:   now,
		Details: map[string]interface{}{
			"duration_ms":       now.Sub(op.StartTime).Milliseconds(),
			"agents_reassigned": op.AgentsReassigned,
			"jobs_reassigned":   op.JobsReassigned,
		},
	})
}

// addStep adds a step to the failover operation.
func (f *FailoverManager) addStep(op *FailoverOperation, name string) *FailoverStep {
	step := &FailoverStep{
		Name:      name,
		Status:    FailoverStateInProgress,
		StartTime: time.Now(),
		Details:   make(map[string]interface{}),
	}
	op.Steps = append(op.Steps, step)
	return step
}

// completeStep marks a step as completed.
func (f *FailoverManager) completeStep(step *FailoverStep) {
	step.Status = FailoverStateCompleted
	now := time.Now()
	step.EndTime = &now
}

// failStep marks a step as failed.
func (f *FailoverManager) failStep(step *FailoverStep, err error) {
	step.Status = FailoverStateFailed
	step.Error = err
	now := time.Now()
	step.EndTime = &now
}

// notifyObservers notifies all failover observers.
func (f *FailoverManager) notifyObservers(event FailoverEvent) {
	f.mu.RLock()
	observers := make([]FailoverObserver, len(f.observers))
	copy(observers, f.observers)
	f.mu.RUnlock()

	for _, observer := range observers {
		go observer(event)
	}
}

// GetFailoverStats returns failover statistics.
func (f *FailoverManager) GetFailoverStats() map[string]interface{} {
	f.mu.RLock()
	defer f.mu.RUnlock()

	completedCount := 0
	failedCount := 0
	totalDuration := time.Duration(0)

	for _, op := range f.failoverHistory {
		if op.State == FailoverStateCompleted {
			completedCount++
			if op.EndTime != nil {
				totalDuration += op.EndTime.Sub(op.StartTime)
			}
		} else if op.State == FailoverStateFailed {
			failedCount++
		}
	}

	avgDuration := time.Duration(0)
	if completedCount > 0 {
		avgDuration = totalDuration / time.Duration(completedCount)
	}

	return map[string]interface{}{
		"total_failovers":     f.failoverCount,
		"active_failovers":    len(f.activeFailovers),
		"completed_failovers": completedCount,
		"failed_failovers":    failedCount,
		"avg_duration_ms":     avgDuration.Milliseconds(),
		"last_failover_time":  f.lastFailoverTime,
	}
}
