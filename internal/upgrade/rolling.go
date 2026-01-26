package upgrade

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/shawnbutts/keystone-core/internal/query"
	"github.com/shawnbutts/keystone-core/pkg/wait"
)

// RollingStrategy implements rolling upgrade logic.
type RollingStrategy struct {
	nodeManager NodeManager
	logger      Logger
	config      *RollingConfig

	mu             sync.Mutex
	currentBatch   int
	completedNodes int
	failedNodes    int
	healthyNodes   map[string]bool
}

// NewRollingStrategy creates a new rolling upgrade strategy.
func NewRollingStrategy(nodeManager NodeManager, logger Logger, config *RollingConfig) *RollingStrategy {
	if config == nil {
		config = DefaultRollingConfig()
	}
	if logger == nil {
		logger = &noopLogger{}
	}
	return &RollingStrategy{
		nodeManager:  nodeManager,
		logger:       logger,
		config:       config,
		healthyNodes: make(map[string]bool),
	}
}

// Execute executes the rolling upgrade.
func (s *RollingStrategy) Execute(ctx context.Context, state *UpgradeState, progressFn func(*UpgradeState)) error {
	components := state.Config.Components
	if len(components) == 0 {
		components = []ComponentType{ComponentServer, ComponentAgent}
	}

	totalNodes := 0
	s.completedNodes = 0
	s.failedNodes = 0

	// Count total nodes
	for _, comp := range components {
		nodes, err := s.nodeManager.GetNodes(ctx, comp)
		if err != nil {
			return fmt.Errorf("getting %s nodes: %w", comp, err)
		}
		totalNodes += len(nodes)
	}

	if totalNodes == 0 {
		s.logger.Info("No nodes to upgrade")
		return nil
	}

	// Process each component
	for _, comp := range components {
		if err := s.upgradeComponent(ctx, state, comp, totalNodes, progressFn); err != nil {
			return err
		}
	}

	return nil
}

// upgradeComponent upgrades all nodes of a component type.
func (s *RollingStrategy) upgradeComponent(
	ctx context.Context,
	state *UpgradeState,
	component ComponentType,
	totalNodes int,
	progressFn func(*UpgradeState),
) error {
	nodes, err := s.nodeManager.GetNodes(ctx, component)
	if err != nil {
		return fmt.Errorf("getting %s nodes: %w", component, err)
	}

	// Sort nodes based on upgrade order
	sortedNodes := s.sortNodes(nodes)

	// Process nodes respecting maxUnavailable
	for i := 0; i < len(sortedNodes); {
		select {
		case <-ctx.Done():
			return ErrUpgradeCancelled
		default:
		}

		// Calculate batch size based on maxUnavailable
		batchSize := s.config.MaxUnavailable
		if batchSize > len(sortedNodes)-i {
			batchSize = len(sortedNodes) - i
		}

		batch := sortedNodes[i : i+batchSize]
		s.currentBatch++

		s.logger.Info("Starting upgrade batch",
			"batch", s.currentBatch,
			"size", len(batch),
			"component", component,
		)

		// Upgrade batch
		for _, node := range batch {
			if err := s.upgradeNode(ctx, state, node, progressFn); err != nil {
				s.failedNodes++
				// Check if we should continue or abort
				if s.failedNodes >= s.config.MaxUnavailable {
					return fmt.Errorf("too many failed nodes (%d), aborting upgrade", s.failedNodes)
				}
				s.logger.Warn("Node upgrade failed, continuing with next node",
					"node", node.ID,
					"error", err,
				)
				continue
			}
			s.completedNodes++

			// Update progress
			progress := 15 + int(float64(s.completedNodes)/float64(totalNodes)*75)
			state.Progress = progress
			state.Message = fmt.Sprintf("Upgraded %d/%d nodes", s.completedNodes, totalNodes)
			if progressFn != nil {
				progressFn(state)
			}
		}

		// Wait before next batch
		if s.config.NodeDelay > 0 && i+batchSize < len(sortedNodes) {
			s.logger.Debug("Waiting before next batch", "delay", s.config.NodeDelay)
			if err := wait.ForContext(ctx, s.config.NodeDelay); err != nil {
				return ErrUpgradeCancelled
			}
		}

		i += batchSize
	}

	return nil
}

// upgradeNode upgrades a single node.
func (s *RollingStrategy) upgradeNode(
	ctx context.Context,
	state *UpgradeState,
	node NodeInfo,
	progressFn func(*UpgradeState),
) error {
	s.logger.Info("Upgrading node", "id", node.ID, "component", node.Component)

	// Initialize node state
	nodeState := &NodeUpgradeState{
		NodeID:      node.ID,
		Component:   node.Component,
		Status:      StatusInProgress,
		FromVersion: node.Version,
		ToVersion:   state.ToVersion,
		StartTime:   time.Now(),
	}
	state.NodeStates[node.ID] = nodeState

	// Step 1: Pre-upgrade health check
	health, err := s.nodeManager.GetNodeHealth(ctx, node.ID)
	if err != nil {
		return s.nodeError(nodeState, fmt.Errorf("pre-upgrade health check: %w", err))
	}
	if health == HealthUnhealthy {
		return s.nodeError(nodeState, fmt.Errorf("node is unhealthy before upgrade"))
	}

	// Step 2: Drain connections
	s.logger.Debug("Draining node", "id", node.ID)
	if err := s.nodeManager.DrainNode(ctx, node.ID, s.config.DrainTimeout); err != nil {
		return s.nodeError(nodeState, fmt.Errorf("draining: %w", err))
	}

	// Step 3: Perform upgrade
	s.logger.Debug("Performing upgrade", "id", node.ID, "version", state.Config.TargetVersion)
	if err := s.nodeManager.UpgradeNode(ctx, node.ID, state.Config.TargetVersion); err != nil {
		// Try to uncordon on failure
		_ = s.nodeManager.UncordonNode(ctx, node.ID)
		return s.nodeError(nodeState, fmt.Errorf("upgrade: %w", err))
	}

	// Step 4: Uncordon node
	s.logger.Debug("Uncordoning node", "id", node.ID)
	if err := s.nodeManager.UncordonNode(ctx, node.ID); err != nil {
		return s.nodeError(nodeState, fmt.Errorf("uncordoning: %w", err))
	}

	// Step 5: Wait for node to become healthy
	s.logger.Debug("Waiting for node health", "id", node.ID)
	healthy, err := s.waitForHealth(ctx, node.ID)
	if err != nil {
		return s.nodeError(nodeState, fmt.Errorf("health check: %w", err))
	}
	if !healthy {
		return s.nodeError(nodeState, fmt.Errorf("node did not become healthy within timeout"))
	}

	// Step 6: Verify version
	version, err := s.nodeManager.GetNodeVersion(ctx, node.ID)
	if err != nil {
		s.logger.Warn("Could not verify version", "id", node.ID, "error", err)
	} else if version.Compare(state.ToVersion) != 0 {
		return s.nodeError(nodeState, fmt.Errorf("version mismatch: expected %s, got %s",
			state.ToVersion, version))
	}

	// Success
	now := time.Now()
	nodeState.Status = StatusCompleted
	nodeState.Health = HealthHealthy
	nodeState.EndTime = &now

	s.mu.Lock()
	s.healthyNodes[node.ID] = true
	s.mu.Unlock()

	s.logger.Info("Node upgrade completed", "id", node.ID, "duration", time.Since(nodeState.StartTime))
	return nil
}

// nodeError marks a node as failed and returns the error.
func (s *RollingStrategy) nodeError(nodeState *NodeUpgradeState, err error) error {
	now := time.Now()
	nodeState.Status = StatusFailed
	nodeState.Health = HealthUnhealthy
	nodeState.Error = err.Error()
	nodeState.EndTime = &now
	return err
}

// waitForHealth waits for a node to become healthy.
func (s *RollingStrategy) waitForHealth(ctx context.Context, nodeID string) (bool, error) {
	deadline := time.Now().Add(s.config.HealthCheckTimeout)
	successCount := 0
	requiredSuccesses := 3 // Number of consecutive successful checks

	for time.Now().Before(deadline) {
		select {
		case <-ctx.Done():
			return false, ctx.Err()
		default:
		}

		health, err := s.nodeManager.GetNodeHealth(ctx, nodeID)
		if err != nil {
			s.logger.Debug("Health check error", "node", nodeID, "error", err)
			successCount = 0
		} else if health == HealthHealthy {
			successCount++
			if successCount >= requiredSuccesses {
				return true, nil
			}
		} else {
			s.logger.Debug("Node not healthy yet", "node", nodeID, "health", health)
			successCount = 0
		}

		select {
		case <-ctx.Done():
			return false, ctx.Err()
		}
		if err := wait.ForContext(ctx, s.config.HealthCheckInterval); err != nil {
			return false, err
		}
	}

	return false, nil
}

// sortNodes sorts nodes for upgrade order.
func (s *RollingStrategy) sortNodes(nodes []NodeInfo) []NodeInfo {
	sorted := make([]NodeInfo, len(nodes))
	copy(sorted, nodes)

	switch s.config.Order {
	case "leader_last":
		// Move leaders to the end
		nonLeaders := make([]NodeInfo, 0)
		leaders := make([]NodeInfo, 0)
		for _, node := range sorted {
			if node.IsLeader {
				leaders = append(leaders, node)
			} else {
				nonLeaders = append(nonLeaders, node)
			}
		}
		sorted = append(nonLeaders, leaders...)

	case "leader_first":
		// Move leaders to the front
		leaders := make([]NodeInfo, 0)
		nonLeaders := make([]NodeInfo, 0)
		for _, node := range sorted {
			if node.IsLeader {
				leaders = append(leaders, node)
			} else {
				nonLeaders = append(nonLeaders, node)
			}
		}
		sorted = append(leaders, nonLeaders...)

	case "any":
		// Keep original order
	}

	return sorted
}

// GetStats returns upgrade statistics.
func (s *RollingStrategy) GetStats() RollingStats {
	s.mu.Lock()
	defer s.mu.Unlock()

	return RollingStats{
		CurrentBatch:   s.currentBatch,
		CompletedNodes: s.completedNodes,
		FailedNodes:    s.failedNodes,
		HealthyNodes:   len(s.healthyNodes),
	}
}

// RollingStats contains rolling upgrade statistics.
type RollingStats struct {
	CurrentBatch   int `json:"current_batch"`
	CompletedNodes int `json:"completed_nodes"`
	FailedNodes    int `json:"failed_nodes"`
	HealthyNodes   int `json:"healthy_nodes"`
}

// CanaryStrategy implements canary deployment logic.
type CanaryStrategy struct {
	nodeManager NodeManager
	logger      Logger
	config      *CanaryConfig

	mu                sync.Mutex
	currentPercentage int
	successfulChecks  int
	failedChecks      int
	metrics           map[string]float64
	metricsQuerier    metricsQuerier
	initErr           error
}

type metricsQuerier interface {
	Query(ctx context.Context, query *query.MetricsQuery) (*query.MetricsResult, error)
}

// NewCanaryStrategy creates a new canary upgrade strategy.
func NewCanaryStrategy(nodeManager NodeManager, logger Logger, config *CanaryConfig) *CanaryStrategy {
	if config == nil {
		config = DefaultCanaryConfig()
	}
	if logger == nil {
		logger = &noopLogger{}
	}
	strategy := &CanaryStrategy{
		nodeManager: nodeManager,
		logger:      logger,
		config:      config,
		metrics:     make(map[string]float64),
	}

	if config.PrometheusAddress != "" {
		querier, err := query.NewPrometheusQuerier(config.PrometheusAddress)
		if err != nil {
			strategy.initErr = fmt.Errorf("init Prometheus querier: %w", err)
		} else {
			strategy.metricsQuerier = querier
		}
	}

	return strategy
}

// Execute executes the canary upgrade.
func (s *CanaryStrategy) Execute(ctx context.Context, state *UpgradeState, progressFn func(*UpgradeState)) error {
	if s.initErr != nil {
		return s.initErr
	}
	if len(s.config.Metrics) > 0 && s.metricsQuerier == nil {
		return fmt.Errorf("prometheus address is required for canary metrics")
	}

	s.currentPercentage = s.config.InitialPercentage
	s.successfulChecks = 0
	s.failedChecks = 0

	components := state.Config.Components
	if len(components) == 0 {
		components = []ComponentType{ComponentServer, ComponentAgent}
	}

	// Get all nodes
	var allNodes []NodeInfo
	for _, comp := range components {
		nodes, err := s.nodeManager.GetNodes(ctx, comp)
		if err != nil {
			return fmt.Errorf("getting %s nodes: %w", comp, err)
		}
		allNodes = append(allNodes, nodes...)
	}

	if len(allNodes) == 0 {
		s.logger.Info("No nodes to upgrade")
		return nil
	}

	for s.currentPercentage <= 100 {
		select {
		case <-ctx.Done():
			return ErrUpgradeCancelled
		default:
		}

		s.logger.Info("Canary step", "percentage", s.currentPercentage)

		// Calculate nodes to upgrade at this percentage
		nodesToUpgrade := int(float64(len(allNodes)) * float64(s.currentPercentage) / 100)
		if nodesToUpgrade == 0 {
			nodesToUpgrade = 1 // At least one node
		}

		// Upgrade nodes for this step
		for i := 0; i < nodesToUpgrade; i++ {
			if i >= len(allNodes) {
				break
			}
			node := allNodes[i]

			// Skip already upgraded nodes
			if nodeState, ok := state.NodeStates[node.ID]; ok && nodeState.Status == StatusCompleted {
				continue
			}

			if err := s.upgradeNode(ctx, state, node); err != nil {
				s.failedChecks++
				if s.failedChecks >= s.config.FailureThreshold {
					return fmt.Errorf("canary failed: too many failures (%d)", s.failedChecks)
				}
				s.logger.Warn("Node upgrade failed during canary", "node", node.ID, "error", err)
			}
		}

		// Update progress
		progress := 15 + int(float64(s.currentPercentage)/100*75)
		state.Progress = progress
		state.Message = fmt.Sprintf("Canary at %d%%", s.currentPercentage)
		if progressFn != nil {
			progressFn(state)
		}

		// Wait and monitor
		if err := s.monitorCanary(ctx, state); err != nil {
			return err
		}

		// Check success criteria
		if s.successfulChecks >= s.config.SuccessThreshold {
			s.currentPercentage += s.config.Increment
			s.successfulChecks = 0
			s.failedChecks = 0
		}

		if s.currentPercentage > 100 {
			s.currentPercentage = 100
			break
		}
	}

	return nil
}

// upgradeNode upgrades a single node during canary.
func (s *CanaryStrategy) upgradeNode(ctx context.Context, state *UpgradeState, node NodeInfo) error {
	nodeState := &NodeUpgradeState{
		NodeID:      node.ID,
		Component:   node.Component,
		Status:      StatusInProgress,
		FromVersion: node.Version,
		ToVersion:   state.ToVersion,
		StartTime:   time.Now(),
	}
	state.NodeStates[node.ID] = nodeState

	// Drain, upgrade, uncordon
	if err := s.nodeManager.DrainNode(ctx, node.ID, 30*time.Second); err != nil {
		nodeState.Status = StatusFailed
		nodeState.Error = err.Error()
		return err
	}

	if err := s.nodeManager.UpgradeNode(ctx, node.ID, state.Config.TargetVersion); err != nil {
		_ = s.nodeManager.UncordonNode(ctx, node.ID)
		nodeState.Status = StatusFailed
		nodeState.Error = err.Error()
		return err
	}

	if err := s.nodeManager.UncordonNode(ctx, node.ID); err != nil {
		nodeState.Status = StatusFailed
		nodeState.Error = err.Error()
		return err
	}

	// Wait for health
	deadline := time.NewTimer(2 * time.Minute)
	defer deadline.Stop()
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-deadline.C:
			nodeState.Status = StatusFailed
			nodeState.Error = "node did not become healthy"
			return fmt.Errorf("node %s did not become healthy", node.ID)
		case <-ticker.C:
			health, _ := s.nodeManager.GetNodeHealth(ctx, node.ID)
			if health == HealthHealthy {
				now := time.Now()
				nodeState.Status = StatusCompleted
				nodeState.Health = HealthHealthy
				nodeState.EndTime = &now
				return nil
			}
		}
	}

}

// monitorCanary monitors canary metrics during the interval.
func (s *CanaryStrategy) monitorCanary(ctx context.Context, state *UpgradeState) error {
	timer := time.NewTimer(s.config.Interval)
	defer timer.Stop()

	select {
	case <-ctx.Done():
		return ErrUpgradeCancelled
	case <-timer.C:
	}

	// Check configured metrics (simplified - in production would query Prometheus)
	allHealthy := true
	for _, metric := range s.config.Metrics {
		value, err := s.checkMetric(ctx, metric)
		if err != nil {
			allHealthy = false
			s.logger.Warn("Canary metric query failed",
				"metric", metric.Name,
				"error", err,
			)
			continue
		}
		s.metrics[metric.Name] = value

		if !s.evaluateMetric(metric, value) {
			allHealthy = false
			s.logger.Warn("Canary metric check failed",
				"metric", metric.Name,
				"value", value,
				"threshold", metric.Threshold,
			)
		}
	}

	if allHealthy {
		s.successfulChecks++
	} else {
		s.failedChecks++
	}

	return nil
}

// checkMetric queries Prometheus for the metric and extracts a numeric value.
func (s *CanaryStrategy) checkMetric(ctx context.Context, metric CanaryMetric) (float64, error) {
	if metric.Query == "" {
		return 0, fmt.Errorf("metric query is required")
	}
	if s.metricsQuerier == nil {
		return 0, fmt.Errorf("metrics querier is not configured")
	}

	queryReq := &query.MetricsQuery{
		Query:   metric.Query,
		Timeout: s.config.QueryTimeout,
	}
	result, err := s.metricsQuerier.Query(ctx, queryReq)
	if err != nil {
		return 0, err
	}

	return extractMetricValue(result)
}

// evaluateMetric evaluates if a metric passes the threshold.
func (s *CanaryStrategy) evaluateMetric(metric CanaryMetric, value float64) bool {
	switch metric.Comparison {
	case "lt":
		return value < metric.Threshold
	case "le":
		return value <= metric.Threshold
	case "gt":
		return value > metric.Threshold
	case "ge":
		return value >= metric.Threshold
	case "eq":
		return value == metric.Threshold
	case "ne":
		return value != metric.Threshold
	default:
		return true
	}
}

func extractMetricValue(result *query.MetricsResult) (float64, error) {
	if result == nil {
		return 0, fmt.Errorf("metrics result is nil")
	}

	switch result.ResultType {
	case "scalar":
		valueMap, ok := result.Result.(map[string]interface{})
		if !ok {
			return 0, fmt.Errorf("unexpected scalar result format")
		}
		value, ok := valueMap["value"].(float64)
		if !ok {
			return 0, fmt.Errorf("unexpected scalar value type")
		}
		return value, nil
	case "vector":
		vector, ok := result.Result.([]map[string]interface{})
		if !ok {
			return 0, fmt.Errorf("unexpected vector result format")
		}
		if len(vector) == 0 {
			return 0, fmt.Errorf("vector result is empty")
		}

		var sum float64
		for _, sample := range vector {
			value, ok := sample["value"].(float64)
			if !ok {
				return 0, fmt.Errorf("unexpected vector value type")
			}
			sum += value
		}
		return sum / float64(len(vector)), nil
	default:
		return 0, fmt.Errorf("unsupported result type: %s", result.ResultType)
	}
}

// GetStats returns canary statistics.
func (s *CanaryStrategy) GetStats() CanaryStats {
	s.mu.Lock()
	defer s.mu.Unlock()

	metrics := make(map[string]float64)
	for k, v := range s.metrics {
		metrics[k] = v
	}

	return CanaryStats{
		CurrentPercentage: s.currentPercentage,
		SuccessfulChecks:  s.successfulChecks,
		FailedChecks:      s.failedChecks,
		Metrics:           metrics,
	}
}

// CanaryStats contains canary upgrade statistics.
type CanaryStats struct {
	CurrentPercentage int                `json:"current_percentage"`
	SuccessfulChecks  int                `json:"successful_checks"`
	FailedChecks      int                `json:"failed_checks"`
	Metrics           map[string]float64 `json:"metrics"`
}
