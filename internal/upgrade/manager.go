package upgrade

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/shawnbutts/keystone-core/pkg/wait"
)

var (
	// ErrUpgradeInProgress indicates an upgrade is already running.
	ErrUpgradeInProgress = errors.New("upgrade already in progress")

	// ErrNoUpgradeInProgress indicates no upgrade is running.
	ErrNoUpgradeInProgress = errors.New("no upgrade in progress")

	// ErrUpgradeCancelled indicates the upgrade was cancelled.
	ErrUpgradeCancelled = errors.New("upgrade cancelled")

	// ErrRollbackFailed indicates rollback failed.
	ErrRollbackFailed = errors.New("rollback failed")
)

// DefaultManager is the default implementation of Manager.
type DefaultManager struct {
	versionProvider  VersionProvider
	versionChecker   *VersionChecker
	nodeManager      NodeManager
	logger           Logger
	progressCallback ProgressCallback

	// State
	mu            sync.RWMutex
	currentState  *State
	upgradeCancel context.CancelFunc
	history       []*State
	maxHistory    int
}

// NewDefaultManager creates a new upgrade manager.
func NewDefaultManager(
	versionProvider VersionProvider,
	nodeManager NodeManager,
	logger Logger,
) *DefaultManager {
	if logger == nil {
		logger = &noopLogger{}
	}
	return &DefaultManager{
		versionProvider: versionProvider,
		versionChecker:  NewVersionChecker(logger),
		nodeManager:     nodeManager,
		logger:          logger,
		maxHistory:      100,
		history:         make([]*State, 0),
	}
}

// SetProgressCallback sets the progress callback.
func (m *DefaultManager) SetProgressCallback(cb ProgressCallback) {
	m.progressCallback = cb
}

// SetVersionChecker sets the version checker.
func (m *DefaultManager) SetVersionChecker(checker *VersionChecker) {
	m.versionChecker = checker
}

// CheckUpgrade checks if an upgrade is available and compatible.
func (m *DefaultManager) CheckUpgrade(ctx context.Context, targetVersion string) (*Check, error) {
	m.logger.Info("Checking upgrade compatibility", "target", targetVersion)

	// Parse target version
	toVersion, err := ParseVersion(targetVersion)
	if err != nil {
		return nil, fmt.Errorf("parsing target version: %w", err)
	}

	// Get current version
	fromVersion, err := m.versionProvider.GetCurrentVersion(ctx, ComponentServer)
	if err != nil {
		return nil, fmt.Errorf("getting current version: %w", err)
	}

	check := &Check{
		CurrentVersion: fromVersion,
		TargetVersion:  toVersion,
		Compatible:     true,
	}

	// Get version info
	info, err := m.versionProvider.GetVersionInfo(ctx, ComponentServer, targetVersion)
	if err != nil {
		if errors.Is(err, ErrVersionNotFound) {
			check.Compatible = false
			check.Blockers = append(check.Blockers, fmt.Sprintf("Version %s not found", targetVersion))
			return check, nil
		}
		return nil, fmt.Errorf("getting version info: %w", err)
	}
	check.VersionInfo = info

	// Check compatibility
	result, err := m.versionChecker.CheckCompatibility(ComponentServer, fromVersion, toVersion)
	if err != nil {
		return nil, fmt.Errorf("checking compatibility: %w", err)
	}

	check.Compatible = result.Compatible
	check.Warnings = result.Warnings
	check.Blockers = result.Blockers
	check.RequiredSteps = result.RequiredSteps

	// Estimate duration based on component count
	nodes, err := m.nodeManager.GetNodes(ctx, ComponentServer)
	if err == nil {
		// Rough estimate: 2 minutes per server node
		check.EstimatedDuration = time.Duration(len(nodes)) * 2 * time.Minute
	}

	return check, nil
}

// PlanUpgrade creates an upgrade plan without executing it.
func (m *DefaultManager) PlanUpgrade(ctx context.Context, config *Config) (*Plan, error) {
	m.logger.Info("Planning upgrade", "target", config.TargetVersion, "strategy", config.Strategy)

	// Validate configuration
	if err := m.validateConfig(config); err != nil {
		return nil, fmt.Errorf("invalid configuration: %w", err)
	}

	// Check upgrade compatibility
	check, err := m.CheckUpgrade(ctx, config.TargetVersion)
	if err != nil {
		return nil, fmt.Errorf("checking upgrade: %w", err)
	}

	if !check.Compatible && !config.Force {
		return nil, fmt.Errorf("upgrade not compatible: %v", check.Blockers)
	}

	plan := &Plan{
		ID:        uuid.New().String(),
		Config:    config,
		Check:     check,
		CreatedAt: time.Now(),
	}

	// Build upgrade steps based on strategy
	switch config.Strategy {
	case StrategyRolling:
		steps, err := m.buildRollingSteps(ctx, config)
		if err != nil {
			return nil, fmt.Errorf("building rolling steps: %w", err)
		}
		plan.Steps = steps

	case StrategyCanary:
		steps, err := m.buildCanarySteps(ctx, config)
		if err != nil {
			return nil, fmt.Errorf("building canary steps: %w", err)
		}
		plan.Steps = steps

	case StrategyInPlace:
		steps, err := m.buildInPlaceSteps(ctx, config)
		if err != nil {
			return nil, fmt.Errorf("building in-place steps: %w", err)
		}
		plan.Steps = steps

	default:
		return nil, fmt.Errorf("unsupported strategy: %s", config.Strategy)
	}

	// Calculate totals
	for _, step := range plan.Steps {
		plan.TotalNodes += len(step.Nodes)
		plan.EstimatedDuration += step.EstimatedDuration
	}

	return plan, nil
}

// StartUpgrade begins an upgrade operation.
func (m *DefaultManager) StartUpgrade(ctx context.Context, config *Config) (*State, error) {
	m.mu.Lock()
	if m.currentState != nil && m.currentState.Status == StatusInProgress {
		m.mu.Unlock()
		return nil, ErrUpgradeInProgress
	}

	// Parse versions
	toVersion, err := ParseVersion(config.TargetVersion)
	if err != nil {
		m.mu.Unlock()
		return nil, fmt.Errorf("parsing target version: %w", err)
	}

	fromVersion, err := m.versionProvider.GetCurrentVersion(ctx, ComponentServer)
	if err != nil {
		m.mu.Unlock()
		return nil, fmt.Errorf("getting current version: %w", err)
	}

	// Create upgrade state
	state := &State{
		ID:          uuid.New().String(),
		Phase:       PhasePending,
		Status:      StatusPending,
		Config:      config,
		FromVersion: fromVersion,
		ToVersion:   toVersion,
		StartTime:   time.Now(),
		NodeStates:  make(map[string]*NodeUpgradeState),
	}

	m.currentState = state

	// Create cancellable context
	upgradeCtx, cancel := context.WithCancel(ctx)
	m.upgradeCancel = cancel
	m.mu.Unlock()

	m.logger.Info("Starting upgrade",
		"id", state.ID,
		"from", fromVersion,
		"to", toVersion,
		"strategy", config.Strategy,
	)

	// Run upgrade in background
	go m.runUpgrade(upgradeCtx, state)

	return state, nil
}

// runUpgrade executes the upgrade process.
func (m *DefaultManager) runUpgrade(ctx context.Context, state *State) {
	defer func() {
		m.mu.Lock()
		m.addToHistory(state)
		m.currentState = nil
		m.upgradeCancel = nil
		m.mu.Unlock()
	}()

	// Update state helper
	updateState := func(phase Phase, status Status, msg string, progress int) {
		m.mu.Lock()
		state.Phase = phase
		state.Status = status
		state.Message = msg
		state.Progress = progress
		m.mu.Unlock()
		m.notifyProgress(state)
	}

	// Dry run check
	if state.Config.DryRun {
		m.logger.Info("Dry run mode - no changes will be made")
		updateState(PhaseCompleted, StatusCompleted, "Dry run completed successfully", 100)
		return
	}

	// Phase 1: Validation
	updateState(PhaseValidating, StatusInProgress, "Validating upgrade prerequisites", 5)
	if err := m.validatePrerequisites(ctx, state); err != nil {
		m.handleUpgradeError(state, PhaseValidating, err, true)
		return
	}

	// Phase 2: Preparation
	updateState(PhasePreparing, StatusInProgress, "Preparing for upgrade", 10)
	if err := m.prepareUpgrade(ctx, state); err != nil {
		m.handleUpgradeError(state, PhasePreparing, err, true)
		return
	}

	// Phase 3: Execute upgrade based on strategy
	updateState(PhaseUpgrading, StatusInProgress, "Executing upgrade", 15)
	var upgradeErr error
	switch state.Config.Strategy {
	case StrategyRolling:
		upgradeErr = m.executeRollingUpgrade(ctx, state)
	case StrategyCanary:
		upgradeErr = m.executeCanaryUpgrade(ctx, state)
	case StrategyInPlace:
		upgradeErr = m.executeInPlaceUpgrade(ctx, state)
	default:
		upgradeErr = fmt.Errorf("unsupported strategy: %s", state.Config.Strategy)
	}

	if upgradeErr != nil {
		// Check if we should rollback
		if state.Config.Rollback != nil && state.Config.Rollback.Automatic {
			m.logger.Warn("Upgrade failed, initiating automatic rollback", "error", upgradeErr)
			updateState(PhaseRollingBack, StatusInProgress, "Rolling back due to upgrade failure", 70)

			if rollbackErr := m.executeRollback(ctx, state, upgradeErr.Error()); rollbackErr != nil {
				m.handleUpgradeError(state, PhaseRollingBack, rollbackErr, false)
				return
			}

			updateState(PhaseRolledBack, StatusRolledBack, "Upgrade rolled back", 100)
			return
		}

		m.handleUpgradeError(state, PhaseUpgrading, upgradeErr, false)
		return
	}

	// Phase 4: Verification
	updateState(PhaseVerifying, StatusInProgress, "Verifying upgrade", 90)
	if err := m.verifyUpgrade(ctx, state); err != nil {
		m.handleUpgradeError(state, PhaseVerifying, err, false)
		return
	}

	// Success
	now := time.Now()
	m.mu.Lock()
	state.Phase = PhaseCompleted
	state.Status = StatusCompleted
	state.Message = "Upgrade completed successfully"
	state.Progress = 100
	state.EndTime = &now
	m.mu.Unlock()

	m.notifyProgress(state)
	m.logger.Info("Upgrade completed successfully",
		"id", state.ID,
		"duration", time.Since(state.StartTime),
	)
}

// handleUpgradeError handles an error during upgrade.
func (m *DefaultManager) handleUpgradeError(state *State, phase Phase, err error, recoverable bool) {
	now := time.Now()
	m.mu.Lock()
	state.Phase = PhaseFailed
	state.Status = StatusFailed
	state.Message = err.Error()
	state.EndTime = &now
	state.Errors = append(state.Errors, Error{
		Time:        now,
		Phase:       phase,
		Message:     err.Error(),
		Recoverable: recoverable,
	})
	m.mu.Unlock()

	m.notifyProgress(state)
	m.logger.Error("Upgrade failed", "phase", phase, "error", err)
}

// validatePrerequisites validates upgrade prerequisites.
func (m *DefaultManager) validatePrerequisites(ctx context.Context, state *State) error {
	// Check version compatibility
	check, err := m.CheckUpgrade(ctx, state.Config.TargetVersion)
	if err != nil {
		return fmt.Errorf("checking upgrade: %w", err)
	}

	if !check.Compatible && !state.Config.Force {
		return fmt.Errorf("upgrade not compatible: %v", check.Blockers)
	}

	// Verify we can reach all nodes
	components := state.Config.Components
	if len(components) == 0 {
		components = []ComponentType{ComponentServer, ComponentAgent}
	}

	for _, comp := range components {
		nodes, err := m.nodeManager.GetNodes(ctx, comp)
		if err != nil {
			return fmt.Errorf("getting %s nodes: %w", comp, err)
		}

		for i := range nodes {
			node := &nodes[i]
			health, err := m.nodeManager.GetNodeHealth(ctx, node.ID)
			if err != nil {
				return fmt.Errorf("checking health of node %s: %w", node.ID, err)
			}

			if health == HealthUnhealthy {
				return fmt.Errorf("node %s is unhealthy, cannot proceed with upgrade", node.ID)
			}
		}
	}

	return nil
}

// prepareUpgrade prepares for the upgrade.
func (m *DefaultManager) prepareUpgrade(ctx context.Context, state *State) error {
	// Download new version
	_, err := m.versionProvider.DownloadVersion(ctx, ComponentServer, state.Config.TargetVersion)
	if err != nil {
		return fmt.Errorf("downloading version: %w", err)
	}

	// Verify download
	// Note: Path is cached, so we'd need to track it
	return nil
}

// executeRollingUpgrade executes a rolling upgrade.
func (m *DefaultManager) executeRollingUpgrade(ctx context.Context, state *State) error {
	config := state.Config.Rolling
	if config == nil {
		config = DefaultRollingConfig()
	}

	// Get nodes to upgrade
	components := state.Config.Components
	if len(components) == 0 {
		components = []ComponentType{ComponentServer, ComponentAgent}
	}

	totalNodes := 0
	completedNodes := 0

	for _, comp := range components {
		nodes, err := m.nodeManager.GetNodes(ctx, comp)
		if err != nil {
			return fmt.Errorf("getting %s nodes: %w", comp, err)
		}

		// Sort nodes based on order preference
		sortedNodes := m.sortNodesForUpgrade(nodes, config.Order)
		totalNodes += len(sortedNodes)

		for j := range sortedNodes {
			node := &sortedNodes[j]
			select {
			case <-ctx.Done():
				return ErrUpgradeCancelled
			default:
			}

			m.logger.Info("Upgrading node", "id", node.ID, "component", comp)

			// Initialize node state
			m.mu.Lock()
			state.NodeStates[node.ID] = &NodeUpgradeState{
				NodeID:      node.ID,
				Component:   comp,
				Status:      StatusInProgress,
				FromVersion: node.Version,
				ToVersion:   state.ToVersion,
				StartTime:   time.Now(),
			}
			m.mu.Unlock()

			// Drain node
			if err := m.nodeManager.DrainNode(ctx, node.ID, config.DrainTimeout); err != nil {
				m.updateNodeState(state, node.ID, StatusFailed, HealthUnhealthy, err.Error())
				return fmt.Errorf("draining node %s: %w", node.ID, err)
			}

			// Upgrade node
			if err := m.nodeManager.UpgradeNode(ctx, node.ID, state.Config.TargetVersion); err != nil {
				m.updateNodeState(state, node.ID, StatusFailed, HealthUnhealthy, err.Error())
				return fmt.Errorf("upgrading node %s: %w", node.ID, err)
			}

			// Uncordon node
			if err := m.nodeManager.UncordonNode(ctx, node.ID); err != nil {
				m.updateNodeState(state, node.ID, StatusFailed, HealthUnhealthy, err.Error())
				return fmt.Errorf("uncordoning node %s: %w", node.ID, err)
			}

			// Wait for health
			healthy, err := m.waitForNodeHealth(ctx, node.ID, config.HealthCheckTimeout, config.HealthCheckInterval)
			if err != nil {
				m.updateNodeState(state, node.ID, StatusFailed, HealthUnhealthy, err.Error())
				return fmt.Errorf("waiting for node %s health: %w", node.ID, err)
			}

			if !healthy {
				m.updateNodeState(state, node.ID, StatusFailed, HealthUnhealthy, "node did not become healthy")
				return fmt.Errorf("node %s did not become healthy after upgrade", node.ID)
			}

			// Mark node complete
			m.updateNodeState(state, node.ID, StatusCompleted, HealthHealthy, "")
			completedNodes++

			// Update progress
			progress := 15 + int(float64(completedNodes)/float64(totalNodes)*75)
			m.mu.Lock()
			state.Progress = progress
			state.Message = fmt.Sprintf("Upgraded %d/%d nodes", completedNodes, totalNodes)
			m.mu.Unlock()
			m.notifyProgress(state)

			// Delay between nodes
			if config.NodeDelay > 0 {
				if err := wait.ForContext(ctx, config.NodeDelay); err != nil {
					return ErrUpgradeCancelled
				}
			}
		}
	}

	return nil
}

// executeCanaryUpgrade executes a canary upgrade.
func (m *DefaultManager) executeCanaryUpgrade(ctx context.Context, state *State) error {
	config := state.Config.Canary
	if config == nil {
		config = DefaultCanaryConfig()
	}

	currentPercentage := config.InitialPercentage
	successCount := 0

	for currentPercentage <= 100 {
		select {
		case <-ctx.Done():
			return ErrUpgradeCancelled
		default:
		}

		m.logger.Info("Canary step", "percentage", currentPercentage)

		// Upgrade percentage of nodes
		// This is a simplified implementation
		m.mu.Lock()
		state.Message = fmt.Sprintf("Canary at %d%%", currentPercentage)
		state.Progress = 15 + int(float64(currentPercentage)/100*75)
		m.mu.Unlock()
		m.notifyProgress(state)

		// Wait and check metrics
		if err := wait.ForContext(ctx, config.Interval); err != nil {
			return ErrUpgradeCancelled
		}

		// Check success criteria (simplified)
		successCount++
		if successCount >= config.SuccessThreshold {
			currentPercentage += config.Increment
			successCount = 0
		}

		if currentPercentage > 100 {
			currentPercentage = 100
		}
	}

	return nil
}

// executeInPlaceUpgrade executes an in-place upgrade (with downtime).
func (m *DefaultManager) executeInPlaceUpgrade(ctx context.Context, state *State) error {
	components := state.Config.Components
	if len(components) == 0 {
		components = []ComponentType{ComponentServer, ComponentAgent}
	}

	for _, comp := range components {
		nodes, err := m.nodeManager.GetNodes(ctx, comp)
		if err != nil {
			return fmt.Errorf("getting %s nodes: %w", comp, err)
		}

		// Stop all, upgrade all, start all
		for j := range nodes {
			if err := m.nodeManager.DrainNode(ctx, nodes[j].ID, 30*time.Second); err != nil {
				m.logger.Warn("Failed to drain node", "id", nodes[j].ID, "error", err)
			}
		}

		for j := range nodes {
			if err := m.nodeManager.UpgradeNode(ctx, nodes[j].ID, state.Config.TargetVersion); err != nil {
				return fmt.Errorf("upgrading node %s: %w", nodes[j].ID, err)
			}
		}

		for j := range nodes {
			if err := m.nodeManager.UncordonNode(ctx, nodes[j].ID); err != nil {
				m.logger.Warn("Failed to uncordon node", "id", nodes[j].ID, "error", err)
			}
		}
	}

	return nil
}

// executeRollback rolls back an upgrade.
func (m *DefaultManager) executeRollback(ctx context.Context, state *State, reason string) error {
	m.logger.Info("Executing rollback", "upgrade_id", state.ID, "reason", reason)

	state.RollbackState = &RollbackState{
		Reason:    reason,
		Automatic: true,
		StartTime: time.Now(),
		Status:    StatusInProgress,
	}

	// Rollback each node that was upgraded
	for nodeID, nodeState := range state.NodeStates {
		if nodeState.Status == StatusCompleted {
			m.logger.Info("Rolling back node", "id", nodeID)

			// Drain, downgrade, uncordon
			if err := m.nodeManager.DrainNode(ctx, nodeID, 30*time.Second); err != nil {
				m.logger.Warn("Failed to drain during rollback", "id", nodeID, "error", err)
			}

			if err := m.nodeManager.UpgradeNode(ctx, nodeID, state.FromVersion.String()); err != nil {
				m.logger.Error("Failed to rollback node", "id", nodeID, "error", err)
				state.RollbackState.Status = StatusFailed
				state.RollbackState.Error = err.Error()
				return fmt.Errorf("rolling back node %s: %w", nodeID, err)
			}

			if err := m.nodeManager.UncordonNode(ctx, nodeID); err != nil {
				m.logger.Warn("Failed to uncordon after rollback", "id", nodeID, "error", err)
			}
		}
	}

	now := time.Now()
	state.RollbackState.Status = StatusCompleted
	state.RollbackState.EndTime = &now

	return nil
}

// verifyUpgrade verifies the upgrade was successful.
func (m *DefaultManager) verifyUpgrade(ctx context.Context, state *State) error {
	// Verify all nodes are healthy and running correct version
	for nodeID, nodeState := range state.NodeStates {
		if nodeState.Status != StatusCompleted {
			continue
		}

		health, err := m.nodeManager.GetNodeHealth(ctx, nodeID)
		if err != nil {
			return fmt.Errorf("checking health of node %s: %w", nodeID, err)
		}

		if health != HealthHealthy {
			return fmt.Errorf("node %s is not healthy after upgrade", nodeID)
		}

		version, err := m.nodeManager.GetNodeVersion(ctx, nodeID)
		if err != nil {
			return fmt.Errorf("checking version of node %s: %w", nodeID, err)
		}

		if version.Compare(state.ToVersion) != 0 {
			return fmt.Errorf("node %s has wrong version: expected %s, got %s",
				nodeID, state.ToVersion, version)
		}
	}

	return nil
}

// GetUpgradeStatus returns the current upgrade status.
func (m *DefaultManager) GetUpgradeStatus(ctx context.Context, upgradeID string) (*State, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if m.currentState != nil && m.currentState.ID == upgradeID {
		return m.currentState, nil
	}

	for _, state := range m.history {
		if state.ID == upgradeID {
			return state, nil
		}
	}

	return nil, fmt.Errorf("upgrade %s not found", upgradeID)
}

// CancelUpgrade cancels an in-progress upgrade.
func (m *DefaultManager) CancelUpgrade(ctx context.Context, upgradeID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.currentState == nil || m.currentState.ID != upgradeID {
		return ErrNoUpgradeInProgress
	}

	if m.upgradeCancel != nil {
		m.upgradeCancel()
	}

	return nil
}

// Rollback rolls back to the previous version.
func (m *DefaultManager) Rollback(ctx context.Context, upgradeID string) (*RollbackState, error) {
	m.mu.Lock()
	state := m.currentState
	if state == nil || state.ID != upgradeID {
		// Look in history
		for _, s := range m.history {
			if s.ID == upgradeID {
				state = s
				break
			}
		}
	}
	m.mu.Unlock()

	if state == nil {
		return nil, fmt.Errorf("upgrade %s not found", upgradeID)
	}

	if err := m.executeRollback(ctx, state, "manual rollback"); err != nil {
		return nil, err
	}

	return state.RollbackState, nil
}

// GetUpgradeHistory returns upgrade history.
func (m *DefaultManager) GetUpgradeHistory(ctx context.Context, limit int) ([]*State, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if limit <= 0 || limit > len(m.history) {
		limit = len(m.history)
	}

	// Return most recent first
	result := make([]*State, limit)
	for i := 0; i < limit; i++ {
		result[i] = m.history[len(m.history)-1-i]
	}

	return result, nil
}

// GetAvailableVersions returns available versions.
func (m *DefaultManager) GetAvailableVersions(ctx context.Context, channel string) ([]VersionInfo, error) {
	return m.versionProvider.GetAvailableVersions(ctx, ComponentServer, channel)
}

// validateConfig validates the upgrade configuration.
func (m *DefaultManager) validateConfig(config *Config) error {
	if config.TargetVersion == "" {
		return errors.New("target_version is required")
	}

	if _, err := ParseVersion(config.TargetVersion); err != nil {
		return fmt.Errorf("invalid target_version: %w", err)
	}

	if config.Strategy == "" {
		config.Strategy = StrategyRolling
	}

	switch config.Strategy {
	case StrategyRolling, StrategyCanary, StrategyBlueGreen, StrategyInPlace:
		// Valid
	default:
		return fmt.Errorf("invalid strategy: %s", config.Strategy)
	}

	return nil
}

// buildRollingSteps builds steps for a rolling upgrade.
func (m *DefaultManager) buildRollingSteps(ctx context.Context, config *Config) ([]Step, error) {
	var steps []Step
	order := 1

	components := config.Components
	if len(components) == 0 {
		components = []ComponentType{ComponentServer, ComponentAgent}
	}

	rollingConfig := config.Rolling
	if rollingConfig == nil {
		rollingConfig = DefaultRollingConfig()
	}

	for _, comp := range components {
		nodes, err := m.nodeManager.GetNodes(ctx, comp)
		if err != nil {
			return nil, fmt.Errorf("getting %s nodes: %w", comp, err)
		}

		sortedNodes := m.sortNodesForUpgrade(nodes, rollingConfig.Order)

		for j := range sortedNodes {
			node := &sortedNodes[j]
			steps = append(steps, Step{
				Order:             order,
				Name:              fmt.Sprintf("Upgrade %s %s", comp, node.ID),
				Description:       fmt.Sprintf("Upgrade %s on node %s", comp, node.ID),
				Component:         comp,
				Nodes:             []string{node.ID},
				EstimatedDuration: 2 * time.Minute,
				Rollbackable:      true,
			})
			order++
		}
	}

	return steps, nil
}

// buildCanarySteps builds steps for a canary upgrade.
func (m *DefaultManager) buildCanarySteps(ctx context.Context, config *Config) ([]Step, error) {
	canaryConfig := config.Canary
	if canaryConfig == nil {
		canaryConfig = DefaultCanaryConfig()
	}

	var steps []Step
	order := 1
	percentage := canaryConfig.InitialPercentage

	for percentage <= 100 {
		steps = append(steps, Step{
			Order:             order,
			Name:              fmt.Sprintf("Canary %d%%", percentage),
			Description:       fmt.Sprintf("Route %d%% of traffic to new version", percentage),
			EstimatedDuration: canaryConfig.Interval,
			Rollbackable:      true,
		})
		order++
		percentage += canaryConfig.Increment
	}

	return steps, nil
}

// buildInPlaceSteps builds steps for an in-place upgrade.
func (m *DefaultManager) buildInPlaceSteps(ctx context.Context, config *Config) ([]Step, error) {
	return []Step{
		{
			Order:             1,
			Name:              "Stop services",
			Description:       "Stop all services",
			EstimatedDuration: 1 * time.Minute,
			Rollbackable:      false,
		},
		{
			Order:             2,
			Name:              "Upgrade binaries",
			Description:       "Upgrade all binaries",
			EstimatedDuration: 2 * time.Minute,
			Rollbackable:      true,
		},
		{
			Order:             3,
			Name:              "Start services",
			Description:       "Start all services",
			EstimatedDuration: 1 * time.Minute,
			Rollbackable:      false,
		},
	}, nil
}

// sortNodesForUpgrade sorts nodes based on upgrade order preference.
func (m *DefaultManager) sortNodesForUpgrade(nodes []NodeInfo, order string) []NodeInfo {
	// Simple implementation - in production would do actual sorting
	sorted := slices.Clone(nodes)

	switch order {
	case "leader_last":
		// Move leader to end
		for i := range sorted {
			if sorted[i].IsLeader {
				node := sorted[i]
				sorted = slices.Delete(sorted, i, i+1)
				sorted = append(sorted, node)
				break
			}
		}
	case "leader_first":
		// Move leader to front
		for i := range sorted {
			if sorted[i].IsLeader {
				node := sorted[i]
				sorted = slices.Delete(sorted, i, i+1)
				sorted = slices.Insert(sorted, 0, node)
				break
			}
		}
	}

	return sorted
}

// waitForNodeHealth waits for a node to become healthy.
func (m *DefaultManager) waitForNodeHealth(ctx context.Context, nodeID string, timeout, interval time.Duration) (bool, error) {
	deadline := time.Now().Add(timeout)

	for time.Now().Before(deadline) {
		select {
		case <-ctx.Done():
			return false, ctx.Err()
		default:
		}

		health, err := m.nodeManager.GetNodeHealth(ctx, nodeID)
		if err != nil {
			m.logger.Warn("Failed to get node health", "id", nodeID, "error", err)
		} else if health == HealthHealthy {
			return true, nil
		}

		if err := wait.ForContext(ctx, interval); err != nil {
			return false, err
		}
	}

	return false, nil
}

// updateNodeState updates the state of a node.
func (m *DefaultManager) updateNodeState(state *State, nodeID string, status Status, health HealthStatus, errMsg string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if ns, ok := state.NodeStates[nodeID]; ok {
		ns.Status = status
		ns.Health = health
		ns.Error = errMsg
		if status == StatusCompleted || status == StatusFailed {
			now := time.Now()
			ns.EndTime = &now
		}
	}
}

// notifyProgress calls the progress callback if set.
func (m *DefaultManager) notifyProgress(state *State) {
	if m.progressCallback != nil {
		m.progressCallback(state)
	}
}

// addToHistory adds a completed upgrade to history.
func (m *DefaultManager) addToHistory(state *State) {
	m.history = append(m.history, state)

	// Trim history if too long
	if len(m.history) > m.maxHistory {
		m.history = m.history[len(m.history)-m.maxHistory:]
	}
}
