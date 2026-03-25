package cluster

import (
	"context"
	"fmt"
	"sync"
	"time"
)

const (
	// Default shutdown timing
	defaultDrainTimeout     = 30 * time.Second
	defaultShutdownTimeout  = 60 * time.Second
	defaultJobDrainInterval = 100 * time.Millisecond
)

// ShutdownPhase represents the current phase of shutdown.
type ShutdownPhase string

// ShutdownPhase constants define the phases.
const (
	ShutdownPhaseRunning       ShutdownPhase = "running"
	ShutdownPhaseInitiated     ShutdownPhase = "initiated"
	ShutdownPhaseDraining      ShutdownPhase = "draining"
	ShutdownPhaseTransferring  ShutdownPhase = "transferring"
	ShutdownPhaseDeregistering ShutdownPhase = "deregistering"
	ShutdownPhaseCompleted     ShutdownPhase = "completed"
	ShutdownPhaseFailed        ShutdownPhase = "failed"
)

// ShutdownReason indicates why shutdown was initiated.
type ShutdownReason string

// ShutdownReasonRequested constants define the reasons.
const (
	ShutdownReasonRequested   ShutdownReason = "requested"
	ShutdownReasonMaintenance ShutdownReason = "maintenance"
	ShutdownReasonUpgrade     ShutdownReason = "upgrade"
	ShutdownReasonScaleDown   ShutdownReason = "scale_down"
	ShutdownReasonUnhealthy   ShutdownReason = "unhealthy"
	ShutdownReasonSignal      ShutdownReason = "signal"
)

// ShutdownStatus contains the current shutdown status.
type ShutdownStatus struct {
	Phase                 ShutdownPhase
	Reason                ShutdownReason
	StartTime             time.Time
	CurrentStep           string
	AgentsDrained         int
	AgentsRemaining       int
	JobsCompleted         int
	JobsRemaining         int
	LeadershipTransferred bool
	Error                 error
}

// ShutdownEvent is emitted during shutdown.
type ShutdownEvent struct {
	Type      ShutdownEventType
	Phase     ShutdownPhase
	MemberID  string
	Reason    ShutdownReason
	Timestamp time.Time
	Details   map[string]interface{}
}

// ShutdownEventType identifies shutdown event types.
type ShutdownEventType string

// ShutdownEventStarted constants define the events.
const (
	ShutdownEventStarted        ShutdownEventType = "shutdown_started"
	ShutdownEventDrainStarted   ShutdownEventType = "drain_started"
	ShutdownEventAgentDrained   ShutdownEventType = "agent_drained"
	ShutdownEventJobsCompleted  ShutdownEventType = "jobs_completed"
	ShutdownEventLeaderTransfer ShutdownEventType = "leader_transferred"
	ShutdownEventDeregistered   ShutdownEventType = "deregistered"
	ShutdownEventCompleted      ShutdownEventType = "shutdown_completed"
	ShutdownEventFailed         ShutdownEventType = "shutdown_failed"
	ShutdownEventTimeout        ShutdownEventType = "shutdown_timeout"
)

// ShutdownObserver is called during shutdown events.
type ShutdownObserver func(event ShutdownEvent)

// ShutdownConfig holds shutdown configuration.
type ShutdownConfig struct {
	DrainTimeout       time.Duration
	ShutdownTimeout    time.Duration
	ForceAfterTimeout  bool
	TransferLeadership bool
	WaitForJobs        bool
}

// DefaultShutdownConfig returns default shutdown configuration.
func DefaultShutdownConfig() *ShutdownConfig {
	return &ShutdownConfig{
		DrainTimeout:       defaultDrainTimeout,
		ShutdownTimeout:    defaultShutdownTimeout,
		ForceAfterTimeout:  true,
		TransferLeadership: true,
		WaitForJobs:        true,
	}
}

// GracefulShutdown manages graceful cluster member shutdown.
type GracefulShutdown struct {
	config     *ShutdownConfig
	clusterCfg *Config
	membership *MembershipManager
	leader     *LeaderElector
	sharding   *ShardManager
	jobs       *JobDistributor
	localID    string

	// State
	status    *ShutdownStatus
	observers []ShutdownObserver

	mu           sync.RWMutex
	shutdownChan chan struct{}
	doneChan     chan struct{}
	closeOnce    sync.Once
	inProgress   bool
}

// NewGracefulShutdown creates a new graceful shutdown manager.
func NewGracefulShutdown(
	config *ShutdownConfig,
	clusterCfg *Config,
	membership *MembershipManager,
	leader *LeaderElector,
	sharding *ShardManager,
	jobs *JobDistributor,
	localID string,
) (*GracefulShutdown, error) {
	if config == nil {
		config = DefaultShutdownConfig()
	}
	if membership == nil {
		return nil, fmt.Errorf("membership manager is required")
	}
	if localID == "" {
		return nil, fmt.Errorf("local member ID is required")
	}

	return &GracefulShutdown{
		config:       config,
		clusterCfg:   clusterCfg,
		membership:   membership,
		leader:       leader,
		sharding:     sharding,
		jobs:         jobs,
		localID:      localID,
		observers:    make([]ShutdownObserver, 0),
		shutdownChan: make(chan struct{}),
		doneChan:     make(chan struct{}),
	}, nil
}

// AddObserver adds a shutdown event observer.
func (g *GracefulShutdown) AddObserver(observer ShutdownObserver) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.observers = append(g.observers, observer)
}

// Initiate starts the graceful shutdown process.
func (g *GracefulShutdown) Initiate(ctx context.Context, reason ShutdownReason) error {
	g.mu.Lock()
	if g.inProgress {
		g.mu.Unlock()
		return fmt.Errorf("shutdown already in progress")
	}
	g.inProgress = true

	g.status = &ShutdownStatus{
		Phase:     ShutdownPhaseInitiated,
		Reason:    reason,
		StartTime: time.Now(),
	}
	g.mu.Unlock()

	// Notify observers
	g.notifyObservers(ShutdownEvent{
		Type:      ShutdownEventStarted,
		Phase:     ShutdownPhaseInitiated,
		MemberID:  g.localID,
		Reason:    reason,
		Timestamp: time.Now(),
	})

	// Create timeout context — cancel is deferred inside the goroutine,
	// not here, because Initiate returns immediately while shutdown runs async.
	ctx, cancel := context.WithTimeout(ctx, g.config.ShutdownTimeout)

	// Run shutdown sequence
	go func() {
		defer cancel()
		g.executeShutdown(ctx, reason)
	}()

	return nil
}

// Wait blocks until shutdown is complete.
func (g *GracefulShutdown) Wait() error {
	<-g.doneChan

	g.mu.RLock()
	defer g.mu.RUnlock()

	if g.status != nil && g.status.Error != nil {
		return g.status.Error
	}

	return nil
}

// GetStatus returns the current shutdown status.
func (g *GracefulShutdown) GetStatus() *ShutdownStatus {
	g.mu.RLock()
	defer g.mu.RUnlock()

	if g.status == nil {
		return &ShutdownStatus{Phase: ShutdownPhaseRunning}
	}

	// Return a copy
	return &ShutdownStatus{
		Phase:                 g.status.Phase,
		Reason:                g.status.Reason,
		StartTime:             g.status.StartTime,
		CurrentStep:           g.status.CurrentStep,
		AgentsDrained:         g.status.AgentsDrained,
		AgentsRemaining:       g.status.AgentsRemaining,
		JobsCompleted:         g.status.JobsCompleted,
		JobsRemaining:         g.status.JobsRemaining,
		LeadershipTransferred: g.status.LeadershipTransferred,
		Error:                 g.status.Error,
	}
}

// IsInProgress returns true if shutdown is in progress.
func (g *GracefulShutdown) IsInProgress() bool {
	g.mu.RLock()
	defer g.mu.RUnlock()
	return g.inProgress
}

// executeShutdown runs the shutdown sequence.
func (g *GracefulShutdown) executeShutdown(ctx context.Context, reason ShutdownReason) {
	defer g.closeDone()

	var err error

	// Phase 1: Mark as draining
	g.updatePhase(ShutdownPhaseDraining, "starting drain")

	// Notify cluster we're shutting down
	g.notifyCluster(ctx)

	// Phase 2: Drain agent connections
	g.updateStep("draining agents")
	if err = g.drainAgents(ctx); err != nil {
		if !g.config.ForceAfterTimeout {
			g.failShutdown(err)
			return
		}
		// Log but continue
	}

	// Phase 3: Wait for jobs to complete
	if g.config.WaitForJobs {
		g.updateStep("waiting for jobs")
		if err = g.waitForJobs(ctx); err != nil {
			if !g.config.ForceAfterTimeout {
				g.failShutdown(err)
				return
			}
		}
	}

	// Phase 4: Transfer leadership if we're leader
	g.updatePhase(ShutdownPhaseTransferring, "transferring leadership")
	if g.config.TransferLeadership && g.leader != nil && g.leader.IsLeader() {
		_ = g.transferLeadership(ctx) // best-effort, continue on failure
	}

	// Phase 5: Deregister from cluster
	g.updatePhase(ShutdownPhaseDeregistering, "deregistering")
	_ = g.deregister(ctx) // best-effort deregistration

	// Complete
	g.completeShutdown()
}

// drainAgents drains agent connections to other members.
func (g *GracefulShutdown) drainAgents(ctx context.Context) error {
	if g.sharding == nil {
		return nil
	}

	g.notifyObservers(ShutdownEvent{
		Type:      ShutdownEventDrainStarted,
		Phase:     ShutdownPhaseDraining,
		MemberID:  g.localID,
		Timestamp: time.Now(),
	})

	// Get our assigned agents
	agents := g.sharding.GetAssignmentsForMember(g.localID)
	total := len(agents)

	g.mu.Lock()
	g.status.AgentsRemaining = total
	g.mu.Unlock()

	if total == 0 {
		return nil
	}

	// Find a healthy member to reassign to
	members := g.membership.ListMembers()
	var targetMemberID string
	for _, member := range members {
		if member.ID != g.localID && member.Status == MemberStatusHealthy {
			targetMemberID = member.ID
			break
		}
	}

	if targetMemberID == "" {
		return fmt.Errorf("no healthy member to reassign agents to")
	}

	// Reassign agents to other members
	drained := 0
	for _, agentID := range agents {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		// Reassign to a healthy member
		if err := g.sharding.ReassignAgent(ctx, agentID, targetMemberID); err != nil {
			continue // Skip this agent
		}

		drained++

		g.mu.Lock()
		g.status.AgentsDrained = drained
		g.status.AgentsRemaining = total - drained
		g.mu.Unlock()

		// Notify progress periodically
		if drained%50 == 0 {
			g.notifyObservers(ShutdownEvent{
				Type:      ShutdownEventAgentDrained,
				Phase:     ShutdownPhaseDraining,
				MemberID:  g.localID,
				Timestamp: time.Now(),
				Details: map[string]interface{}{
					"drained":   drained,
					"remaining": total - drained,
				},
			})
		}
	}

	return nil
}

// waitForJobs waits for in-flight jobs to complete.
func (g *GracefulShutdown) waitForJobs(ctx context.Context) error {
	if g.jobs == nil {
		return nil
	}

	ticker := time.NewTicker(defaultJobDrainInterval)
	defer ticker.Stop()

	drainCtx, cancel := context.WithTimeout(ctx, g.config.DrainTimeout)
	defer cancel()

	for {
		select {
		case <-drainCtx.Done():
			return drainCtx.Err()
		case <-ticker.C:
			activeJobs := g.jobs.GetActiveJobs()

			// Count jobs assigned to us
			ourJobs := 0
			for _, job := range activeJobs {
				if job.AssignedMemberID == g.localID {
					ourJobs++
				}
			}

			g.mu.Lock()
			g.status.JobsRemaining = ourJobs
			g.mu.Unlock()

			if ourJobs == 0 {
				g.notifyObservers(ShutdownEvent{
					Type:      ShutdownEventJobsCompleted,
					Phase:     ShutdownPhaseDraining,
					MemberID:  g.localID,
					Timestamp: time.Now(),
				})
				return nil
			}
		}
	}
}

// transferLeadership transfers leadership to another member.
func (g *GracefulShutdown) transferLeadership(ctx context.Context) error {
	if g.leader == nil || !g.leader.IsLeader() {
		return nil
	}

	// Find a healthy member to transfer to
	members := g.membership.ListMembers()
	var targetID string

	for _, member := range members {
		if member.ID != g.localID && member.Status == MemberStatusHealthy {
			targetID = member.ID
			break
		}
	}

	if targetID == "" {
		return fmt.Errorf("no healthy member to transfer leadership to")
	}

	// Transfer leadership
	if err := g.leader.TransferLeadership(ctx, targetID); err != nil {
		return err
	}

	g.mu.Lock()
	g.status.LeadershipTransferred = true
	g.mu.Unlock()

	g.notifyObservers(ShutdownEvent{
		Type:      ShutdownEventLeaderTransfer,
		Phase:     ShutdownPhaseTransferring,
		MemberID:  g.localID,
		Timestamp: time.Now(),
		Details: map[string]interface{}{
			"new_leader": targetID,
		},
	})

	return nil
}

// deregister removes this member from the cluster.
func (g *GracefulShutdown) deregister(ctx context.Context) error {
	if err := g.membership.Stop(ctx); err != nil {
		return err
	}

	g.notifyObservers(ShutdownEvent{
		Type:      ShutdownEventDeregistered,
		Phase:     ShutdownPhaseDeregistering,
		MemberID:  g.localID,
		Timestamp: time.Now(),
	})

	return nil
}

// notifyCluster notifies other members of impending shutdown.
func (g *GracefulShutdown) notifyCluster(ctx context.Context) {
	// Update member status to leaving
	_ = g.membership.UpdateLocalMember(ctx, func(m *Member) { //nolint:errcheck // best-effort notification
		m.Status = MemberStatusLeaving
	})
}

// updatePhase updates the current shutdown phase.
func (g *GracefulShutdown) updatePhase(phase ShutdownPhase, step string) {
	g.mu.Lock()
	g.status.Phase = phase
	g.status.CurrentStep = step
	g.mu.Unlock()
}

// updateStep updates the current step.
func (g *GracefulShutdown) updateStep(step string) {
	g.mu.Lock()
	g.status.CurrentStep = step
	g.mu.Unlock()
}

// failShutdown marks shutdown as failed.
func (g *GracefulShutdown) failShutdown(err error) {
	g.mu.Lock()
	g.status.Phase = ShutdownPhaseFailed
	g.status.Error = err
	g.mu.Unlock()

	g.notifyObservers(ShutdownEvent{
		Type:      ShutdownEventFailed,
		Phase:     ShutdownPhaseFailed,
		MemberID:  g.localID,
		Timestamp: time.Now(),
		Details: map[string]interface{}{
			"error": err.Error(),
		},
	})
}

// completeShutdown marks shutdown as complete.
func (g *GracefulShutdown) completeShutdown() {
	g.mu.Lock()
	g.status.Phase = ShutdownPhaseCompleted
	g.mu.Unlock()

	g.notifyObservers(ShutdownEvent{
		Type:      ShutdownEventCompleted,
		Phase:     ShutdownPhaseCompleted,
		MemberID:  g.localID,
		Timestamp: time.Now(),
		Details: map[string]interface{}{
			"duration_ms":      time.Since(g.status.StartTime).Milliseconds(),
			"agents_drained":   g.status.AgentsDrained,
			"leadership_moved": g.status.LeadershipTransferred,
		},
	})
}

// notifyObservers notifies all shutdown observers with panic recovery.
func (g *GracefulShutdown) notifyObservers(event ShutdownEvent) {
	g.mu.RLock()
	observers := make([]ShutdownObserver, len(g.observers))
	copy(observers, g.observers)
	g.mu.RUnlock()

	safeDispatchObservers(observers, event, func(o ShutdownObserver, e any) {
		o(e.(ShutdownEvent))
	})
}

// ForceShutdown forcibly shuts down without draining.
func (g *GracefulShutdown) ForceShutdown(ctx context.Context) error {
	g.mu.Lock()
	if g.inProgress {
		// Already shutting down, just force completion
		close(g.shutdownChan)
		g.mu.Unlock()
		return nil
	}
	g.inProgress = true
	g.status = &ShutdownStatus{
		Phase:     ShutdownPhaseCompleted,
		Reason:    ShutdownReasonSignal,
		StartTime: time.Now(),
	}
	g.mu.Unlock()

	// Deregister immediately
	g.membership.Stop(ctx)

	g.closeDone()
	return nil
}

// closeDone safely closes doneChan exactly once, preventing double-close panics.
func (g *GracefulShutdown) closeDone() {
	g.closeOnce.Do(func() {
		close(g.doneChan)
	})
}

// DrainAndWait drains connections and waits with custom timeout.
func (g *GracefulShutdown) DrainAndWait(ctx context.Context, timeout time.Duration) error {
	drainCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	// Start draining
	if err := g.drainAgents(drainCtx); err != nil {
		return err
	}

	// Wait for jobs
	if g.config.WaitForJobs {
		if err := g.waitForJobs(drainCtx); err != nil {
			return err
		}
	}

	return nil
}
