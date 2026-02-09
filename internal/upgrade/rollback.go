package upgrade

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/shawnbutts/keystone-core/pkg/wait"
)

// RollbackManager handles rollback operations.
type RollbackManager struct {
	nodeManager     NodeManager
	versionProvider VersionProvider
	logger          Logger
	config          *RollbackConfig

	mu              sync.Mutex
	activeRollbacks map[string]*RollbackOperation
	history         []*RollbackOperation
	maxHistory      int
}

// NewRollbackManager creates a new rollback manager.
func NewRollbackManager(nodeManager NodeManager, versionProvider VersionProvider, logger Logger, config *RollbackConfig) *RollbackManager {
	if config == nil {
		config = DefaultRollbackConfig()
	}
	if logger == nil {
		logger = &noopLogger{}
	}
	return &RollbackManager{
		nodeManager:     nodeManager,
		versionProvider: versionProvider,
		logger:          logger,
		config:          config,
		activeRollbacks: make(map[string]*RollbackOperation),
		history:         make([]*RollbackOperation, 0),
		maxHistory:      100,
	}
}

// RollbackOperation represents an in-progress or completed rollback.
type RollbackOperation struct {
	ID              string                        `json:"id"`
	UpgradeID       string                        `json:"upgrade_id"`
	Reason          string                        `json:"reason"`
	Automatic       bool                          `json:"automatic"`
	Status          Status                        `json:"status"`
	FromVersion     Version                       `json:"from_version"`
	ToVersion       Version                       `json:"to_version"`
	StartTime       time.Time                     `json:"start_time"`
	EndTime         *time.Time                    `json:"end_time,omitempty"`
	NodesRolledBack int                           `json:"nodes_rolled_back"`
	NodesFailed     int                           `json:"nodes_failed"`
	NodeStates      map[string]*RollbackNodeState `json:"node_states,omitempty"`
	Errors          []RollbackError               `json:"errors,omitempty"`
}

// RollbackNodeState tracks the rollback state of a single node.
type RollbackNodeState struct {
	NodeID      string        `json:"node_id"`
	Component   ComponentType `json:"component"`
	Status      Status        `json:"status"`
	FromVersion Version       `json:"from_version"`
	ToVersion   Version       `json:"to_version"`
	StartTime   time.Time     `json:"start_time"`
	EndTime     *time.Time    `json:"end_time,omitempty"`
	Error       string        `json:"error,omitempty"`
}

// RollbackError represents an error during rollback.
type RollbackError struct {
	Time    time.Time `json:"time"`
	NodeID  string    `json:"node_id,omitempty"`
	Message string    `json:"message"`
}

// RollbackUpgrade performs a rollback for an upgrade.
func (m *RollbackManager) RollbackUpgrade(ctx context.Context, upgradeState *State, reason string, automatic bool) (*RollbackOperation, error) {
	m.mu.Lock()
	if _, exists := m.activeRollbacks[upgradeState.ID]; exists {
		m.mu.Unlock()
		return nil, fmt.Errorf("rollback already in progress for upgrade %s", upgradeState.ID)
	}

	op := &RollbackOperation{
		ID:          fmt.Sprintf("rollback-%s", upgradeState.ID),
		UpgradeID:   upgradeState.ID,
		Reason:      reason,
		Automatic:   automatic,
		Status:      StatusInProgress,
		FromVersion: upgradeState.ToVersion,   // Rolling back from the new version
		ToVersion:   upgradeState.FromVersion, // Back to the original version
		StartTime:   time.Now(),
		NodeStates:  make(map[string]*RollbackNodeState),
	}

	m.activeRollbacks[upgradeState.ID] = op
	m.mu.Unlock()

	m.logger.Info("Starting rollback",
		"id", op.ID,
		"upgrade_id", upgradeState.ID,
		"reason", reason,
		"automatic", automatic,
		"from", op.FromVersion,
		"to", op.ToVersion,
	)

	// Execute rollback
	err := m.executeRollback(ctx, op, upgradeState)

	// Finish up
	m.mu.Lock()
	now := time.Now()
	op.EndTime = &now
	if err != nil {
		op.Status = StatusFailed
		op.Errors = append(op.Errors, RollbackError{
			Time:    now,
			Message: err.Error(),
		})
	} else {
		op.Status = StatusCompleted
	}
	delete(m.activeRollbacks, upgradeState.ID)
	m.addToHistory(op)
	m.mu.Unlock()

	if err != nil {
		m.logger.Error("Rollback failed", "id", op.ID, "error", err)
		return op, err
	}

	m.logger.Info("Rollback completed",
		"id", op.ID,
		"nodes_rolled_back", op.NodesRolledBack,
		"duration", time.Since(op.StartTime),
	)

	return op, nil
}

// executeRollback performs the actual rollback.
func (m *RollbackManager) executeRollback(ctx context.Context, op *RollbackOperation, upgradeState *State) error {
	// Set timeout if configured
	if m.config.Timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, m.config.Timeout)
		defer cancel()
	}

	// Rollback each node that was upgraded
	// Process in reverse order (last upgraded first)
	nodesToRollback := m.getNodesInReverseOrder(upgradeState)

	for _, nodeID := range nodesToRollback {
		select {
		case <-ctx.Done():
			return fmt.Errorf("rollback cancelled: %w", ctx.Err())
		default:
		}

		nodeState := upgradeState.NodeStates[nodeID]
		if nodeState == nil {
			continue
		}

		// Only rollback nodes that were successfully upgraded
		if nodeState.Status != StatusCompleted {
			m.logger.Debug("Skipping node rollback - not completed", "node", nodeID, "status", nodeState.Status)
			continue
		}

		if err := m.rollbackNode(ctx, op, nodeState); err != nil {
			op.NodesFailed++
			m.logger.Error("Failed to rollback node", "node", nodeID, "error", err)

			// Check if we should continue or abort
			if !m.config.Automatic {
				// For manual rollbacks, abort on first failure
				return fmt.Errorf("failed to rollback node %s: %w", nodeID, err)
			}
			// For automatic rollbacks, continue trying other nodes
		} else {
			op.NodesRolledBack++
		}
	}

	if op.NodesFailed > 0 && op.NodesRolledBack == 0 {
		return fmt.Errorf("all rollback attempts failed")
	}

	return nil
}

// rollbackNode rolls back a single node.
func (m *RollbackManager) rollbackNode(ctx context.Context, op *RollbackOperation, upgradeNodeState *NodeUpgradeState) error {
	nodeID := upgradeNodeState.NodeID

	m.logger.Info("Rolling back node",
		"node", nodeID,
		"from", upgradeNodeState.ToVersion,
		"to", upgradeNodeState.FromVersion,
	)

	// Initialize rollback node state
	nodeState := &RollbackNodeState{
		NodeID:      nodeID,
		Component:   upgradeNodeState.Component,
		Status:      StatusInProgress,
		FromVersion: upgradeNodeState.ToVersion,
		ToVersion:   upgradeNodeState.FromVersion,
		StartTime:   time.Now(),
	}
	op.NodeStates[nodeID] = nodeState

	// Step 1: Drain node
	m.logger.Debug("Draining node for rollback", "node", nodeID)
	if err := m.nodeManager.DrainNode(ctx, nodeID, 30*time.Second); err != nil {
		m.logger.Warn("Failed to drain node during rollback", "node", nodeID, "error", err)
		// Continue anyway - drain is best effort
	}

	// Step 2: Downgrade to previous version
	previousVersion := upgradeNodeState.FromVersion.String()
	m.logger.Debug("Downgrading node", "node", nodeID, "version", previousVersion)

	if err := m.nodeManager.UpgradeNode(ctx, nodeID, previousVersion); err != nil {
		nodeState.Status = StatusFailed
		nodeState.Error = err.Error()
		now := time.Now()
		nodeState.EndTime = &now
		return fmt.Errorf("downgrading node: %w", err)
	}

	// Step 3: Uncordon node
	m.logger.Debug("Uncordoning node after rollback", "node", nodeID)
	if err := m.nodeManager.UncordonNode(ctx, nodeID); err != nil {
		m.logger.Warn("Failed to uncordon node after rollback", "node", nodeID, "error", err)
	}

	// Step 4: Wait for health
	m.logger.Debug("Waiting for node health after rollback", "node", nodeID)
	healthy := m.waitForHealth(ctx, nodeID)
	if !healthy {
		nodeState.Status = StatusFailed
		nodeState.Error = "node did not become healthy after rollback"
		now := time.Now()
		nodeState.EndTime = &now
		return fmt.Errorf("node did not become healthy after rollback")
	}

	// Step 5: Verify version
	version, err := m.nodeManager.GetNodeVersion(ctx, nodeID)
	if err != nil {
		m.logger.Warn("Could not verify version after rollback", "node", nodeID, "error", err)
	} else if version.Compare(upgradeNodeState.FromVersion) != 0 {
		nodeState.Status = StatusFailed
		nodeState.Error = fmt.Sprintf("version mismatch: expected %s, got %s",
			upgradeNodeState.FromVersion, version)
		now := time.Now()
		nodeState.EndTime = &now
		return fmt.Errorf("version mismatch after rollback")
	}

	// Success
	now := time.Now()
	nodeState.Status = StatusCompleted
	nodeState.EndTime = &now

	m.logger.Info("Node rollback completed", "node", nodeID)
	return nil
}

// waitForHealth waits for a node to become healthy.
func (m *RollbackManager) waitForHealth(ctx context.Context, nodeID string) bool {
	deadline := time.Now().Add(2 * time.Minute)

	for time.Now().Before(deadline) {
		select {
		case <-ctx.Done():
			return false
		default:
		}

		health, err := m.nodeManager.GetNodeHealth(ctx, nodeID)
		if err == nil && health == HealthHealthy {
			return true
		}

		if err := wait.ForContext(ctx, 5*time.Second); err != nil {
			return false
		}
	}

	return false
}

// getNodesInReverseOrder returns nodes in reverse upgrade order.
func (m *RollbackManager) getNodesInReverseOrder(upgradeState *State) []string {
	// Collect nodes with their completion times
	type nodeTime struct {
		id   string
		time time.Time
	}
	var nodes []nodeTime

	for id, state := range upgradeState.NodeStates {
		if state.EndTime != nil {
			nodes = append(nodes, nodeTime{id: id, time: *state.EndTime})
		} else {
			nodes = append(nodes, nodeTime{id: id, time: state.StartTime})
		}
	}

	// Sort by time (descending - most recent first)
	for i := 0; i < len(nodes)-1; i++ {
		for j := i + 1; j < len(nodes); j++ {
			if nodes[i].time.Before(nodes[j].time) {
				nodes[i], nodes[j] = nodes[j], nodes[i]
			}
		}
	}

	result := make([]string, len(nodes))
	for i, n := range nodes {
		result[i] = n.id
	}

	return result
}

// GetActiveRollback returns an active rollback for an upgrade.
func (m *RollbackManager) GetActiveRollback(upgradeID string) *RollbackOperation {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.activeRollbacks[upgradeID]
}

// GetRollbackHistory returns rollback history.
func (m *RollbackManager) GetRollbackHistory(limit int) []*RollbackOperation {
	m.mu.Lock()
	defer m.mu.Unlock()

	if limit <= 0 || limit > len(m.history) {
		limit = len(m.history)
	}

	// Return most recent first
	result := make([]*RollbackOperation, limit)
	for i := 0; i < limit; i++ {
		result[i] = m.history[len(m.history)-1-i]
	}

	return result
}

// addToHistory adds a completed rollback to history.
func (m *RollbackManager) addToHistory(op *RollbackOperation) {
	m.history = append(m.history, op)

	if len(m.history) > m.maxHistory {
		m.history = m.history[len(m.history)-m.maxHistory:]
	}
}

// RollbackDecision helps decide whether to rollback.
type RollbackDecision struct {
	ShouldRollback bool     `json:"should_rollback"`
	Reasons        []string `json:"reasons"`
	Confidence     float64  `json:"confidence"` // 0-1
}

// EvaluateRollbackNeed evaluates whether a rollback is needed.
func (m *RollbackManager) EvaluateRollbackNeed(ctx context.Context, upgradeState *State) *RollbackDecision {
	decision := &RollbackDecision{
		ShouldRollback: false,
		Reasons:        make([]string, 0),
		Confidence:     0.0,
	}

	// Check failure count
	failedNodes := 0
	for _, nodeState := range upgradeState.NodeStates {
		if nodeState.Status == StatusFailed {
			failedNodes++
		}
	}

	if failedNodes > 0 {
		decision.Reasons = append(decision.Reasons,
			fmt.Sprintf("%d nodes failed during upgrade", failedNodes))
		decision.Confidence += 0.3

		// Check threshold
		if m.config.OnFailureCount > 0 && failedNodes >= m.config.OnFailureCount {
			decision.ShouldRollback = true
			decision.Confidence = 1.0
			decision.Reasons = append(decision.Reasons,
				fmt.Sprintf("Failure count (%d) exceeds threshold (%d)",
					failedNodes, m.config.OnFailureCount))
		}
	}

	// Note: Could add stall detection here by tracking if progress hasn't changed
	// over time. For now, we rely on other health indicators.

	// Check health of upgraded nodes
	unhealthyCount := 0
	for nodeID, nodeState := range upgradeState.NodeStates {
		if nodeState.Status == StatusCompleted {
			health, err := m.nodeManager.GetNodeHealth(ctx, nodeID)
			if err != nil || health != HealthHealthy {
				unhealthyCount++
			}
		}
	}

	if unhealthyCount > 0 {
		decision.Reasons = append(decision.Reasons,
			fmt.Sprintf("%d upgraded nodes are unhealthy", unhealthyCount))
		decision.Confidence += float64(unhealthyCount) * 0.1
	}

	// Cap confidence at 1.0
	if decision.Confidence > 1.0 {
		decision.Confidence = 1.0
	}

	// Auto-rollback threshold
	if m.config.Automatic && decision.Confidence >= 0.7 {
		decision.ShouldRollback = true
	}

	return decision
}

// RollbackPlan contains a plan for rollback.
type RollbackPlan struct {
	UpgradeID       string        `json:"upgrade_id"`
	NodesToRollback []string      `json:"nodes_to_rollback"`
	EstimatedTime   time.Duration `json:"estimated_time"`
	VersionInfo     *VersionInfo  `json:"version_info,omitempty"`
	Risks           []string      `json:"risks,omitempty"`
}

// PlanRollback creates a rollback plan without executing it.
func (m *RollbackManager) PlanRollback(ctx context.Context, upgradeState *State) (*RollbackPlan, error) {
	plan := &RollbackPlan{
		UpgradeID:       upgradeState.ID,
		NodesToRollback: make([]string, 0),
		Risks:           make([]string, 0),
	}

	// Identify nodes to rollback
	for nodeID, nodeState := range upgradeState.NodeStates {
		if nodeState.Status == StatusCompleted {
			plan.NodesToRollback = append(plan.NodesToRollback, nodeID)
		}
	}

	// Estimate time (2 minutes per node)
	plan.EstimatedTime = time.Duration(len(plan.NodesToRollback)) * 2 * time.Minute

	// Get version info for the original version
	versionInfo, err := m.versionProvider.GetVersionInfo(ctx, ComponentServer, upgradeState.FromVersion.String())
	if err == nil {
		plan.VersionInfo = versionInfo
	}

	// Identify risks
	if len(plan.NodesToRollback) == 0 {
		plan.Risks = append(plan.Risks, "No nodes to rollback")
	}

	// Check if rollback version is still available
	_, err = m.versionProvider.DownloadVersion(ctx, ComponentServer, upgradeState.FromVersion.String())
	if err != nil {
		plan.Risks = append(plan.Risks,
			fmt.Sprintf("Previous version (%s) may not be available: %v",
				upgradeState.FromVersion, err))
	}

	return plan, nil
}

// CancelRollback cancels an in-progress rollback.
func (m *RollbackManager) CancelRollback(upgradeID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	op, exists := m.activeRollbacks[upgradeID]
	if !exists {
		return fmt.Errorf("no active rollback for upgrade %s", upgradeID)
	}

	// Note: This doesn't actually cancel ongoing operations
	// A proper implementation would use context cancellation
	op.Status = StatusCancelled
	now := time.Now()
	op.EndTime = &now

	delete(m.activeRollbacks, upgradeID)
	m.addToHistory(op)

	return nil
}
