package cluster

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/shawnbutts/keystone-core/pkg/wait"
)

const (
	// Default health check intervals
	defaultHealthCheckInterval = 1 * time.Second
	defaultFailureThreshold    = 3 // Number of consecutive failures before marking unhealthy
	defaultSlowThreshold       = 500 * time.Millisecond
)

// HealthStatus represents the health state of a member.
type HealthStatus string

// HealthStatus constants define the possible statuses.
const (
	HealthStatusHealthy   HealthStatus = "healthy"
	HealthStatusDegraded  HealthStatus = "degraded"
	HealthStatusUnhealthy HealthStatus = "unhealthy"
	HealthStatusUnknown   HealthStatus = "unknown"
)

// HealthCheckType identifies different health check types.
type HealthCheckType string

// HealthCheckHeartbeat and related constants.
const (
	HealthCheckHeartbeat   HealthCheckType = "heartbeat"
	HealthCheckEtcd        HealthCheckType = "etcd"
	HealthCheckDatabase    HealthCheckType = "database"
	HealthCheckNATS        HealthCheckType = "nats"
	HealthCheckApplication HealthCheckType = "application"
)

// HealthCheckResult contains the result of a health check.
type HealthCheckResult struct {
	Type      HealthCheckType
	Status    HealthStatus
	Latency   time.Duration
	Message   string
	Timestamp time.Time
	Error     error
}

// MemberHealth tracks the health of a cluster member.
type MemberHealth struct {
	MemberID          string
	Status            HealthStatus
	LastHeartbeat     time.Time
	LastHealthCheck   time.Time
	ConsecutiveFails  int
	LatencyP50        time.Duration
	LatencyP99        time.Duration
	CheckResults      map[HealthCheckType]*HealthCheckResult
	FailureDetectedAt *time.Time
	mu                sync.RWMutex
}

// HealthEvent represents a health-related event.
type HealthEvent struct {
	Type         HealthEventType
	MemberID     string
	OldStatus    HealthStatus
	NewStatus    HealthStatus
	Reason       string
	Timestamp    time.Time
	CheckResults map[HealthCheckType]*HealthCheckResult
}

// HealthEventType identifies different health events.
type HealthEventType string

// HealthEventMember constants define the events.
const (
	HealthEventMemberHealthy   HealthEventType = "member_healthy"
	HealthEventMemberDegraded  HealthEventType = "member_degraded"
	HealthEventMemberUnhealthy HealthEventType = "member_unhealthy"
	HealthEventMemberFailed    HealthEventType = "member_failed"
	HealthEventMemberRecovered HealthEventType = "member_recovered"
	HealthEventPartitionStart  HealthEventType = "partition_start"
	HealthEventPartitionEnd    HealthEventType = "partition_end"
	HealthEventQuorumLost      HealthEventType = "quorum_lost"
	HealthEventQuorumRestored  HealthEventType = "quorum_restored"
)

// HealthObserver is called when health events occur.
type HealthObserver func(event HealthEvent)

// HealthChecker interface for custom health checks.
type HealthChecker interface {
	Name() HealthCheckType
	Check(ctx context.Context) *HealthCheckResult
	Interval() time.Duration
}

// HealthMonitor monitors cluster member health.
type HealthMonitor struct {
	config     *Config
	membership *MembershipManager
	etcd       *EtcdClient
	localID    string

	members   map[string]*MemberHealth
	checkers  []HealthChecker
	observers []HealthObserver

	// Partition detection
	partitionDetected bool
	partitionStart    time.Time

	// Quorum tracking
	quorumLost     bool
	quorumLostTime time.Time

	mu       sync.RWMutex
	stopChan chan struct{}
	doneChan chan struct{}
	started  bool
}

// HealthMonitorConfig holds configuration for the health monitor.
type HealthMonitorConfig struct {
	CheckInterval       time.Duration
	FailureThreshold    int
	SlowThreshold       time.Duration
	PartitionTimeout    time.Duration
	RecoveryGracePeriod time.Duration
}

// DefaultHealthMonitorConfig returns default configuration.
func DefaultHealthMonitorConfig() *HealthMonitorConfig {
	return &HealthMonitorConfig{
		CheckInterval:       defaultHealthCheckInterval,
		FailureThreshold:    defaultFailureThreshold,
		SlowThreshold:       defaultSlowThreshold,
		PartitionTimeout:    10 * time.Second,
		RecoveryGracePeriod: 5 * time.Second,
	}
}

// NewHealthMonitor creates a new health monitor.
func NewHealthMonitor(config *Config, membership *MembershipManager, etcd *EtcdClient, localID string) (*HealthMonitor, error) {
	if config == nil {
		return nil, fmt.Errorf("config is required")
	}
	if membership == nil {
		return nil, fmt.Errorf("membership manager is required")
	}
	if etcd == nil {
		return nil, fmt.Errorf("etcd client is required")
	}
	if localID == "" {
		return nil, fmt.Errorf("local member ID is required")
	}

	return &HealthMonitor{
		config:     config,
		membership: membership,
		etcd:       etcd,
		localID:    localID,
		members:    make(map[string]*MemberHealth),
		checkers:   make([]HealthChecker, 0),
		observers:  make([]HealthObserver, 0),
		stopChan:   make(chan struct{}),
		doneChan:   make(chan struct{}),
	}, nil
}

// Start begins health monitoring.
func (h *HealthMonitor) Start(ctx context.Context) error {
	h.mu.Lock()
	if h.started {
		h.mu.Unlock()
		return fmt.Errorf("health monitor already started")
	}
	h.started = true
	h.mu.Unlock()

	// Subscribe to membership changes
	h.membership.AddObserver(h.onMembershipChange)

	// Initialize health tracking for existing members
	h.initializeMembers()

	// Start the monitoring loop
	go h.monitorLoop(ctx)

	return nil
}

// Stop stops health monitoring.
func (h *HealthMonitor) Stop() error {
	h.mu.Lock()
	if !h.started {
		h.mu.Unlock()
		return nil
	}
	h.started = false
	h.mu.Unlock()

	close(h.stopChan)

	wait.ForSignal(h.doneChan, 5*time.Second)

	return nil
}

// RegisterChecker adds a custom health checker.
func (h *HealthMonitor) RegisterChecker(checker HealthChecker) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.checkers = append(h.checkers, checker)
}

// AddObserver adds a health event observer.
func (h *HealthMonitor) AddObserver(observer HealthObserver) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.observers = append(h.observers, observer)
}

// GetMemberHealth returns the health status of a specific member.
func (h *HealthMonitor) GetMemberHealth(memberID string) (*MemberHealth, error) {
	h.mu.RLock()
	defer h.mu.RUnlock()

	health, exists := h.members[memberID]
	if !exists {
		return nil, fmt.Errorf("member %s not found", memberID)
	}

	// Return a copy
	health.mu.RLock()
	defer health.mu.RUnlock()

	copied := &MemberHealth{
		MemberID:         health.MemberID,
		Status:           health.Status,
		LastHeartbeat:    health.LastHeartbeat,
		LastHealthCheck:  health.LastHealthCheck,
		ConsecutiveFails: health.ConsecutiveFails,
		LatencyP50:       health.LatencyP50,
		LatencyP99:       health.LatencyP99,
		CheckResults:     make(map[HealthCheckType]*HealthCheckResult),
	}

	for k, v := range health.CheckResults {
		copied.CheckResults[k] = v
	}

	if health.FailureDetectedAt != nil {
		t := *health.FailureDetectedAt
		copied.FailureDetectedAt = &t
	}

	return copied, nil
}

// GetAllMemberHealth returns health status for all members.
func (h *HealthMonitor) GetAllMemberHealth() map[string]*MemberHealth {
	h.mu.RLock()
	defer h.mu.RUnlock()

	result := make(map[string]*MemberHealth)
	for id := range h.members {
		if health, err := h.GetMemberHealth(id); err == nil {
			result[id] = health
		}
	}

	return result
}

// GetHealthyMemberCount returns the count of healthy members.
func (h *HealthMonitor) GetHealthyMemberCount() int {
	h.mu.RLock()
	defer h.mu.RUnlock()

	count := 0
	for _, health := range h.members {
		health.mu.RLock()
		if health.Status == HealthStatusHealthy || health.Status == HealthStatusDegraded {
			count++
		}
		health.mu.RUnlock()
	}

	return count
}

// HasQuorum returns true if the cluster has quorum.
func (h *HealthMonitor) HasQuorum() bool {
	healthyCount := h.GetHealthyMemberCount()
	totalCount := h.membership.MemberCount()

	// Quorum requires N/2 + 1 members
	quorumSize := (totalCount / 2) + 1
	return healthyCount >= quorumSize
}

// IsPartitioned returns true if a network partition is detected.
func (h *HealthMonitor) IsPartitioned() bool {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return h.partitionDetected
}

// RecordHeartbeat records a heartbeat from a member.
func (h *HealthMonitor) RecordHeartbeat(memberID string) {
	h.mu.RLock()
	health, exists := h.members[memberID]
	h.mu.RUnlock()

	if !exists {
		return
	}

	health.mu.Lock()
	wasUnhealthy := health.Status == HealthStatusUnhealthy
	health.LastHeartbeat = time.Now()
	health.ConsecutiveFails = 0

	if wasUnhealthy {
		health.Status = HealthStatusHealthy
		health.FailureDetectedAt = nil
		health.mu.Unlock()

		h.notifyObservers(HealthEvent{
			Type:      HealthEventMemberRecovered,
			MemberID:  memberID,
			OldStatus: HealthStatusUnhealthy,
			NewStatus: HealthStatusHealthy,
			Reason:    "heartbeat received after failure",
			Timestamp: time.Now(),
		})
	} else {
		health.mu.Unlock()
	}
}

// CheckMemberHealth performs a health check on a specific member.
func (h *HealthMonitor) CheckMemberHealth(ctx context.Context, memberID string) *MemberHealth {
	h.mu.RLock()
	health, exists := h.members[memberID]
	h.mu.RUnlock()

	if !exists {
		return nil
	}

	// Run all checkers for this member
	results := make(map[HealthCheckType]*HealthCheckResult)

	h.mu.RLock()
	checkers := h.checkers
	h.mu.RUnlock()

	for _, checker := range checkers {
		result := checker.Check(ctx)
		results[checker.Name()] = result
	}

	// Update health based on results
	health.mu.Lock()
	defer health.mu.Unlock()

	health.LastHealthCheck = time.Now()
	health.CheckResults = results

	// Determine overall status
	newStatus := h.calculateOverallStatus(results)
	oldStatus := health.Status

	if newStatus != oldStatus {
		health.Status = newStatus

		if newStatus == HealthStatusUnhealthy && oldStatus != HealthStatusUnhealthy {
			now := time.Now()
			health.FailureDetectedAt = &now
		} else if newStatus == HealthStatusHealthy && oldStatus == HealthStatusUnhealthy {
			health.FailureDetectedAt = nil
		}
	}

	return health
}

// initializeMembers initializes health tracking for existing members.
func (h *HealthMonitor) initializeMembers() {
	members := h.membership.ListMembers()

	h.mu.Lock()
	defer h.mu.Unlock()

	for _, member := range members {
		h.members[member.ID] = &MemberHealth{
			MemberID:      member.ID,
			Status:        HealthStatusUnknown,
			LastHeartbeat: time.Now(),
			CheckResults:  make(map[HealthCheckType]*HealthCheckResult),
		}
	}
}

// onMembershipChange handles membership changes.
func (h *HealthMonitor) onMembershipChange(event MembershipEvent) {
	h.mu.Lock()
	defer h.mu.Unlock()

	switch event.Type {
	case MembershipEventJoined:
		h.members[event.Member.ID] = &MemberHealth{
			MemberID:      event.Member.ID,
			Status:        HealthStatusUnknown,
			LastHeartbeat: time.Now(),
			CheckResults:  make(map[HealthCheckType]*HealthCheckResult),
		}

	case MembershipEventLeft, MembershipEventFailed:
		delete(h.members, event.Member.ID)
	default:
	}
}

// monitorLoop runs the health monitoring loop.
func (h *HealthMonitor) monitorLoop(ctx context.Context) {
	defer close(h.doneChan)

	ticker := time.NewTicker(defaultHealthCheckInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-h.stopChan:
			return
		case <-ticker.C:
			h.performHealthChecks(ctx)
			h.checkForPartition()
			h.checkQuorum()
		}
	}
}

// performHealthChecks runs health checks on all members.
func (h *HealthMonitor) performHealthChecks(ctx context.Context) {
	h.mu.RLock()
	memberIDs := make([]string, 0, len(h.members))
	for id := range h.members {
		memberIDs = append(memberIDs, id)
	}
	h.mu.RUnlock()

	for _, memberID := range memberIDs {
		h.checkMemberHeartbeat(memberID)
	}
}

// checkMemberHeartbeat checks if a member's heartbeat is recent enough.
func (h *HealthMonitor) checkMemberHeartbeat(memberID string) {
	h.mu.RLock()
	health, exists := h.members[memberID]
	h.mu.RUnlock()

	if !exists {
		return
	}

	health.mu.Lock()
	defer health.mu.Unlock()

	// Calculate time since last heartbeat
	timeSinceHeartbeat := time.Since(health.LastHeartbeat)
	heartbeatTimeout := h.config.HeartbeatTimeout
	if heartbeatTimeout == 0 {
		heartbeatTimeout = 30 * time.Second
	}

	oldStatus := health.Status

	switch {
	case timeSinceHeartbeat > heartbeatTimeout:
		// Member has missed too many heartbeats
		health.ConsecutiveFails++

		switch {
		case health.ConsecutiveFails >= defaultFailureThreshold:
			if health.Status != HealthStatusUnhealthy {
				health.Status = HealthStatusUnhealthy
				now := time.Now()
				health.FailureDetectedAt = &now

				// Notify observers outside the lock
				go h.notifyObservers(HealthEvent{
					Type:      HealthEventMemberFailed,
					MemberID:  memberID,
					OldStatus: oldStatus,
					NewStatus: HealthStatusUnhealthy,
					Reason:    fmt.Sprintf("no heartbeat for %v", timeSinceHeartbeat),
					Timestamp: time.Now(),
				})
			}
		case health.Status == HealthStatusHealthy:
			health.Status = HealthStatusDegraded

			go h.notifyObservers(HealthEvent{
				Type:      HealthEventMemberDegraded,
				MemberID:  memberID,
				OldStatus: oldStatus,
				NewStatus: HealthStatusDegraded,
				Reason:    fmt.Sprintf("missed heartbeat (%d/%d)", health.ConsecutiveFails, defaultFailureThreshold),
				Timestamp: time.Now(),
			})
		}
	case timeSinceHeartbeat > (heartbeatTimeout / 2):
		// Heartbeat is getting stale, mark as degraded
		if health.Status == HealthStatusHealthy {
			health.Status = HealthStatusDegraded

			go h.notifyObservers(HealthEvent{
				Type:      HealthEventMemberDegraded,
				MemberID:  memberID,
				OldStatus: oldStatus,
				NewStatus: HealthStatusDegraded,
				Reason:    "heartbeat delay detected",
				Timestamp: time.Now(),
			})
		}
	default:
		// Heartbeat is healthy
		if health.Status == HealthStatusUnknown || health.Status == HealthStatusDegraded {
			health.Status = HealthStatusHealthy
			health.ConsecutiveFails = 0

			go h.notifyObservers(HealthEvent{
				Type:      HealthEventMemberHealthy,
				MemberID:  memberID,
				OldStatus: oldStatus,
				NewStatus: HealthStatusHealthy,
				Reason:    "heartbeat received",
				Timestamp: time.Now(),
			})
		}
	}
}

// checkForPartition checks if a network partition is detected.
func (h *HealthMonitor) checkForPartition() {
	healthyCount := h.GetHealthyMemberCount()
	totalCount := h.membership.MemberCount()

	// If we can only see ourselves or very few members, we might be partitioned
	h.mu.Lock()
	defer h.mu.Unlock()

	wasPartitioned := h.partitionDetected

	if totalCount > 1 && healthyCount <= 1 {
		// We can only see ourselves - likely partitioned
		if !h.partitionDetected {
			h.partitionDetected = true
			h.partitionStart = time.Now()

			go h.notifyObservers(HealthEvent{
				Type:      HealthEventPartitionStart,
				MemberID:  h.localID,
				Reason:    fmt.Sprintf("can only see %d of %d members", healthyCount, totalCount),
				Timestamp: time.Now(),
			})
		}
	} else if wasPartitioned && healthyCount > 1 {
		// Partition appears to be healed
		h.partitionDetected = false

		go h.notifyObservers(HealthEvent{
			Type:      HealthEventPartitionEnd,
			MemberID:  h.localID,
			Reason:    fmt.Sprintf("can now see %d of %d members", healthyCount, totalCount),
			Timestamp: time.Now(),
		})
	}
}

// checkQuorum checks if quorum is maintained.
func (h *HealthMonitor) checkQuorum() {
	hasQuorum := h.HasQuorum()

	h.mu.Lock()
	defer h.mu.Unlock()

	wasLost := h.quorumLost

	if !hasQuorum && !h.quorumLost {
		h.quorumLost = true
		h.quorumLostTime = time.Now()

		go h.notifyObservers(HealthEvent{
			Type:      HealthEventQuorumLost,
			MemberID:  h.localID,
			Reason:    fmt.Sprintf("healthy members: %d, required: %d", h.GetHealthyMemberCount(), (h.membership.MemberCount()/2)+1),
			Timestamp: time.Now(),
		})
	} else if hasQuorum && wasLost {
		h.quorumLost = false

		go h.notifyObservers(HealthEvent{
			Type:      HealthEventQuorumRestored,
			MemberID:  h.localID,
			Reason:    "quorum restored",
			Timestamp: time.Now(),
		})
	}
}

// calculateOverallStatus determines overall health from check results.
func (h *HealthMonitor) calculateOverallStatus(results map[HealthCheckType]*HealthCheckResult) HealthStatus {
	if len(results) == 0 {
		return HealthStatusUnknown
	}

	unhealthyCount := 0
	degradedCount := 0

	for _, result := range results {
		switch result.Status {
		case HealthStatusUnhealthy:
			unhealthyCount++
		case HealthStatusDegraded:
			degradedCount++
		default:
		}
	}

	// If any critical check is unhealthy, member is unhealthy
	if unhealthyCount > 0 {
		return HealthStatusUnhealthy
	}

	// If any check is degraded, member is degraded
	if degradedCount > 0 {
		return HealthStatusDegraded
	}

	return HealthStatusHealthy
}

// notifyObservers notifies all health observers of an event.
func (h *HealthMonitor) notifyObservers(event HealthEvent) {
	h.mu.RLock()
	observers := make([]HealthObserver, len(h.observers))
	copy(observers, h.observers)
	h.mu.RUnlock()

	for _, observer := range observers {
		observer(event)
	}
}

// GetFailedMembers returns a list of failed member IDs.
func (h *HealthMonitor) GetFailedMembers() []string {
	h.mu.RLock()
	defer h.mu.RUnlock()

	failed := make([]string, 0)
	for id, health := range h.members {
		health.mu.RLock()
		if health.Status == HealthStatusUnhealthy {
			failed = append(failed, id)
		}
		health.mu.RUnlock()
	}

	return failed
}

// GetDegradedMembers returns a list of degraded member IDs.
func (h *HealthMonitor) GetDegradedMembers() []string {
	h.mu.RLock()
	defer h.mu.RUnlock()

	degraded := make([]string, 0)
	for id, health := range h.members {
		health.mu.RLock()
		if health.Status == HealthStatusDegraded {
			degraded = append(degraded, id)
		}
		health.mu.RUnlock()
	}

	return degraded
}

// EtcdHealthChecker checks etcd connectivity.
type EtcdHealthChecker struct {
	etcd     *EtcdClient
	interval time.Duration
}

// NewEtcdHealthChecker creates a new etcd health checker.
func NewEtcdHealthChecker(etcd *EtcdClient) *EtcdHealthChecker {
	return &EtcdHealthChecker{
		etcd:     etcd,
		interval: 5 * time.Second,
	}
}

// Name returns the checker name.
func (c *EtcdHealthChecker) Name() HealthCheckType {
	return HealthCheckEtcd
}

// Check performs the health check.
func (c *EtcdHealthChecker) Check(ctx context.Context) *HealthCheckResult {
	start := time.Now()

	result := &HealthCheckResult{
		Type:      HealthCheckEtcd,
		Timestamp: time.Now(),
	}

	if !c.etcd.IsConnected() {
		result.Status = HealthStatusUnhealthy
		result.Message = "etcd not connected"
		result.Error = ErrEtcdNotConnected
		return result
	}

	// Perform a simple get to check responsiveness
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()

	_, err := c.etcd.Get(ctx, "/health-check")
	result.Latency = time.Since(start)

	if err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			result.Status = HealthStatusDegraded
			result.Message = "etcd response slow"
		} else {
			result.Status = HealthStatusUnhealthy
			result.Message = "etcd check failed"
			result.Error = err
		}
		return result
	}

	if result.Latency > defaultSlowThreshold {
		result.Status = HealthStatusDegraded
		result.Message = fmt.Sprintf("etcd response slow: %v", result.Latency)
	} else {
		result.Status = HealthStatusHealthy
		result.Message = "etcd healthy"
	}

	return result
}

// Interval returns the check interval.
func (c *EtcdHealthChecker) Interval() time.Duration {
	return c.interval
}
