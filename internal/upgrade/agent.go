package upgrade

import (
	"context"
	"fmt"
	"sort"
	"sync"
	"time"

	"github.com/shawnbutts/keystone-core/pkg/wait"
)

// AgentUpgrader handles batch upgrades of agents.
type AgentUpgrader struct {
	nodeManager NodeManager
	logger      Logger
	config      *AgentBatchConfig

	mu           sync.Mutex
	currentBatch int
	totalBatches int
	completed    int
	failed       int
	skipped      int
	inProgress   map[string]bool
}

// NewAgentUpgrader creates a new agent upgrader.
func NewAgentUpgrader(nodeManager NodeManager, logger Logger, config *AgentBatchConfig) *AgentUpgrader {
	if config == nil {
		config = DefaultAgentBatchConfig()
	}
	if logger == nil {
		logger = &noopLogger{}
	}
	return &AgentUpgrader{
		nodeManager: nodeManager,
		logger:      logger,
		config:      config,
		inProgress:  make(map[string]bool),
	}
}

// UpgradeAgents upgrades all agents matching the selectors.
func (u *AgentUpgrader) UpgradeAgents(ctx context.Context, targetVersion string, progressFn func(AgentUpgradeProgress)) error {
	u.logger.Info("Starting agent upgrade", "version", targetVersion, "batch_size", u.config.BatchSize)

	// Get all agents
	agents, err := u.nodeManager.GetNodes(ctx, ComponentAgent)
	if err != nil {
		return fmt.Errorf("getting agents: %w", err)
	}

	// Filter agents
	filtered := u.filterAgents(agents)
	if len(filtered) == 0 {
		u.logger.Info("No agents to upgrade")
		return nil
	}

	// Sort agents by priority
	sorted := u.sortAgents(filtered)

	// Calculate batches
	u.totalBatches = (len(sorted) + u.config.BatchSize - 1) / u.config.BatchSize
	u.currentBatch = 0
	u.completed = 0
	u.failed = 0
	u.skipped = len(agents) - len(filtered)

	u.logger.Info("Agent upgrade plan",
		"total_agents", len(agents),
		"filtered_agents", len(filtered),
		"skipped_agents", u.skipped,
		"batches", u.totalBatches,
	)

	// Process batches
	for i := 0; i < len(sorted); i += u.config.BatchSize {
		select {
		case <-ctx.Done():
			return ErrUpgradeCancelled
		default:
		}

		end := i + u.config.BatchSize
		if end > len(sorted) {
			end = len(sorted)
		}

		batch := sorted[i:end]
		u.currentBatch++

		u.logger.Info("Starting batch",
			"batch", u.currentBatch,
			"total_batches", u.totalBatches,
			"agents", len(batch),
		)

		// Upgrade batch in parallel
		batchFailed := u.upgradeBatch(ctx, batch, targetVersion)

		if batchFailed > 0 && u.failed >= u.config.MaxFailures {
			return fmt.Errorf("exceeded maximum failures (%d), aborting agent upgrade", u.config.MaxFailures)
		}

		// Report progress
		if progressFn != nil {
			progressFn(u.GetProgress())
		}

		// Delay before next batch
		if u.config.BatchDelay > 0 && end < len(sorted) {
			u.logger.Debug("Waiting before next batch", "delay", u.config.BatchDelay)
			if err := wait.ForContext(ctx, u.config.BatchDelay); err != nil {
				return ErrUpgradeCancelled
			}
		}
	}

	u.logger.Info("Agent upgrade completed",
		"completed", u.completed,
		"failed", u.failed,
		"skipped", u.skipped,
	)

	return nil
}

// upgradeBatch upgrades a batch of agents in parallel.
func (u *AgentUpgrader) upgradeBatch(ctx context.Context, batch []NodeInfo, targetVersion string) int {
	var wg sync.WaitGroup
	var batchFailed int
	var mu sync.Mutex

	for _, agent := range batch {
		wg.Add(1)
		go func(a NodeInfo) {
			defer wg.Done()

			u.mu.Lock()
			u.inProgress[a.ID] = true
			u.mu.Unlock()

			defer func() {
				u.mu.Lock()
				delete(u.inProgress, a.ID)
				u.mu.Unlock()
			}()

			if err := u.upgradeAgent(ctx, a, targetVersion); err != nil {
				u.logger.Warn("Agent upgrade failed", "agent", a.ID, "error", err)
				mu.Lock()
				batchFailed++
				u.failed++
				mu.Unlock()
			} else {
				mu.Lock()
				u.completed++
				mu.Unlock()
			}
		}(agent)
	}

	wg.Wait()
	return batchFailed
}

// upgradeAgent upgrades a single agent.
func (u *AgentUpgrader) upgradeAgent(ctx context.Context, agent NodeInfo, targetVersion string) error {
	u.logger.Debug("Upgrading agent", "id", agent.ID, "from", agent.Version, "to", targetVersion)

	// Check current version
	if agent.Version.String() == targetVersion {
		u.logger.Debug("Agent already at target version", "id", agent.ID)
		return nil
	}

	// Drain agent (stop accepting new work)
	if err := u.nodeManager.DrainNode(ctx, agent.ID, 30*time.Second); err != nil {
		u.logger.Debug("Failed to drain agent", "id", agent.ID, "error", err)
		// Continue anyway - drain is best effort for agents
	}

	// Perform upgrade
	if err := u.nodeManager.UpgradeNode(ctx, agent.ID, targetVersion); err != nil {
		return fmt.Errorf("upgrading agent %s: %w", agent.ID, err)
	}

	// Uncordon agent (resume accepting work)
	if err := u.nodeManager.UncordonNode(ctx, agent.ID); err != nil {
		u.logger.Warn("Failed to uncordon agent", "id", agent.ID, "error", err)
	}

	// Wait for agent to become healthy
	healthy := u.waitForAgentHealth(ctx, agent.ID, 2*time.Minute)
	if !healthy {
		return fmt.Errorf("agent %s did not become healthy after upgrade", agent.ID)
	}

	// Verify version
	version, err := u.nodeManager.GetNodeVersion(ctx, agent.ID)
	if err != nil {
		u.logger.Warn("Could not verify agent version", "id", agent.ID, "error", err)
	} else {
		targetVer, _ := ParseVersion(targetVersion)
		if version.Compare(targetVer) != 0 {
			return fmt.Errorf("agent %s version mismatch: expected %s, got %s",
				agent.ID, targetVersion, version)
		}
	}

	u.logger.Debug("Agent upgrade completed", "id", agent.ID)
	return nil
}

// waitForAgentHealth waits for an agent to become healthy.
func (u *AgentUpgrader) waitForAgentHealth(ctx context.Context, agentID string, timeout time.Duration) bool {
	deadline := time.Now().Add(timeout)

	for time.Now().Before(deadline) {
		select {
		case <-ctx.Done():
			return false
		default:
		}

		health, err := u.nodeManager.GetNodeHealth(ctx, agentID)
		if err == nil && health == HealthHealthy {
			return true
		}

		select {
		case <-ctx.Done():
			return false
		}
		if err := wait.ForContext(ctx, 5*time.Second); err != nil {
			return false
		}
	}

	return false
}

// filterAgents filters agents based on selectors.
func (u *AgentUpgrader) filterAgents(agents []NodeInfo) []NodeInfo {
	var filtered []NodeInfo

	for _, agent := range agents {
		// Check include selectors
		if len(u.config.Selectors) > 0 {
			match := true
			for key, value := range u.config.Selectors {
				if agentValue, ok := agent.Labels[key]; !ok || agentValue != value {
					match = false
					break
				}
			}
			if !match {
				continue
			}
		}

		// Check exclude selectors
		if len(u.config.ExcludeSelectors) > 0 {
			excluded := false
			for key, value := range u.config.ExcludeSelectors {
				if agentValue, ok := agent.Labels[key]; ok && agentValue == value {
					excluded = true
					break
				}
			}
			if excluded {
				continue
			}
		}

		filtered = append(filtered, agent)
	}

	return filtered
}

// sortAgents sorts agents based on priority labels.
func (u *AgentUpgrader) sortAgents(agents []NodeInfo) []NodeInfo {
	if len(u.config.Priority) == 0 {
		return agents
	}

	sorted := make([]NodeInfo, len(agents))
	copy(sorted, agents)

	sort.Slice(sorted, func(i, j int) bool {
		for _, key := range u.config.Priority {
			vi := sorted[i].Labels[key]
			vj := sorted[j].Labels[key]
			if vi != vj {
				return vi < vj
			}
		}
		return sorted[i].ID < sorted[j].ID
	})

	return sorted
}

// GetProgress returns current upgrade progress.
func (u *AgentUpgrader) GetProgress() AgentUpgradeProgress {
	u.mu.Lock()
	defer u.mu.Unlock()

	inProgress := make([]string, 0, len(u.inProgress))
	for id := range u.inProgress {
		inProgress = append(inProgress, id)
	}

	return AgentUpgradeProgress{
		CurrentBatch: u.currentBatch,
		TotalBatches: u.totalBatches,
		Completed:    u.completed,
		Failed:       u.failed,
		Skipped:      u.skipped,
		InProgress:   inProgress,
	}
}

// AgentUpgradeProgress contains agent upgrade progress information.
type AgentUpgradeProgress struct {
	CurrentBatch int      `json:"current_batch"`
	TotalBatches int      `json:"total_batches"`
	Completed    int      `json:"completed"`
	Failed       int      `json:"failed"`
	Skipped      int      `json:"skipped"`
	InProgress   []string `json:"in_progress"`
}

// PercentComplete returns the completion percentage.
func (p AgentUpgradeProgress) PercentComplete() int {
	total := p.Completed + p.Failed + len(p.InProgress) + (p.TotalBatches-p.CurrentBatch)*1 // Approximate remaining
	if total == 0 {
		return 100
	}
	return int(float64(p.Completed) / float64(total) * 100)
}

// AgentVersionReport contains version information for all agents.
type AgentVersionReport struct {
	TotalAgents     int                 `json:"total_agents"`
	VersionCounts   map[string]int      `json:"version_counts"`
	AgentsByVersion map[string][]string `json:"agents_by_version"`
	OutdatedAgents  []AgentVersionInfo  `json:"outdated_agents"`
	HealthyAgents   int                 `json:"healthy_agents"`
	UnhealthyAgents int                 `json:"unhealthy_agents"`
}

// AgentVersionInfo contains version info for a single agent.
type AgentVersionInfo struct {
	AgentID        string            `json:"agent_id"`
	CurrentVersion string            `json:"current_version"`
	Health         HealthStatus      `json:"health"`
	Labels         map[string]string `json:"labels,omitempty"`
}

// GetAgentVersionReport generates a report of agent versions.
func (u *AgentUpgrader) GetAgentVersionReport(ctx context.Context, targetVersion string) (*AgentVersionReport, error) {
	agents, err := u.nodeManager.GetNodes(ctx, ComponentAgent)
	if err != nil {
		return nil, fmt.Errorf("getting agents: %w", err)
	}

	report := &AgentVersionReport{
		TotalAgents:     len(agents),
		VersionCounts:   make(map[string]int),
		AgentsByVersion: make(map[string][]string),
		OutdatedAgents:  make([]AgentVersionInfo, 0),
	}

	targetVer, _ := ParseVersion(targetVersion)

	for _, agent := range agents {
		version := agent.Version.String()
		report.VersionCounts[version]++
		report.AgentsByVersion[version] = append(report.AgentsByVersion[version], agent.ID)

		if agent.Health == HealthHealthy {
			report.HealthyAgents++
		} else {
			report.UnhealthyAgents++
		}

		if agent.Version.Compare(targetVer) < 0 {
			report.OutdatedAgents = append(report.OutdatedAgents, AgentVersionInfo{
				AgentID:        agent.ID,
				CurrentVersion: version,
				Health:         agent.Health,
				Labels:         agent.Labels,
			})
		}
	}

	return report, nil
}

// AgentUpgradeResult contains the result of an agent upgrade operation.
type AgentUpgradeResult struct {
	Success          bool                `json:"success"`
	TotalAgents      int                 `json:"total_agents"`
	UpgradedAgents   int                 `json:"upgraded_agents"`
	FailedAgents     int                 `json:"failed_agents"`
	SkippedAgents    int                 `json:"skipped_agents"`
	Duration         time.Duration       `json:"duration"`
	FailedAgentIDs   []string            `json:"failed_agent_ids,omitempty"`
	UpgradedAgentIDs []string            `json:"upgraded_agent_ids,omitempty"`
	Errors           []AgentUpgradeError `json:"errors,omitempty"`
}

// AgentUpgradeError contains error information for a failed agent upgrade.
type AgentUpgradeError struct {
	AgentID string `json:"agent_id"`
	Error   string `json:"error"`
	Phase   string `json:"phase"`
}

// UpgradeAgentsWithResult upgrades agents and returns a detailed result.
func (u *AgentUpgrader) UpgradeAgentsWithResult(ctx context.Context, targetVersion string) (*AgentUpgradeResult, error) {
	start := time.Now()

	result := &AgentUpgradeResult{
		FailedAgentIDs:   make([]string, 0),
		UpgradedAgentIDs: make([]string, 0),
		Errors:           make([]AgentUpgradeError, 0),
	}

	// Get all agents
	agents, err := u.nodeManager.GetNodes(ctx, ComponentAgent)
	if err != nil {
		return nil, fmt.Errorf("getting agents: %w", err)
	}

	result.TotalAgents = len(agents)

	// Filter agents
	filtered := u.filterAgents(agents)
	result.SkippedAgents = len(agents) - len(filtered)

	// Sort agents by priority
	sorted := u.sortAgents(filtered)

	// Track results
	var mu sync.Mutex
	var wg sync.WaitGroup

	// Process in batches
	for i := 0; i < len(sorted); i += u.config.BatchSize {
		select {
		case <-ctx.Done():
			result.Duration = time.Since(start)
			return result, ErrUpgradeCancelled
		default:
		}

		end := i + u.config.BatchSize
		if end > len(sorted) {
			end = len(sorted)
		}

		batch := sorted[i:end]

		for _, agent := range batch {
			wg.Add(1)
			go func(a NodeInfo) {
				defer wg.Done()

				if err := u.upgradeAgent(ctx, a, targetVersion); err != nil {
					mu.Lock()
					result.FailedAgents++
					result.FailedAgentIDs = append(result.FailedAgentIDs, a.ID)
					result.Errors = append(result.Errors, AgentUpgradeError{
						AgentID: a.ID,
						Error:   err.Error(),
						Phase:   "upgrade",
					})
					mu.Unlock()
				} else {
					mu.Lock()
					result.UpgradedAgents++
					result.UpgradedAgentIDs = append(result.UpgradedAgentIDs, a.ID)
					mu.Unlock()
				}
			}(agent)
		}

		wg.Wait()

		// Check if we exceeded max failures
		if result.FailedAgents >= u.config.MaxFailures {
			break
		}

		// Delay before next batch
		if u.config.BatchDelay > 0 && end < len(sorted) {
			if err := wait.ForContext(ctx, u.config.BatchDelay); err != nil {
				result.Duration = time.Since(start)
				return result, ErrUpgradeCancelled
			}
		}
	}

	result.Duration = time.Since(start)
	result.Success = result.FailedAgents == 0

	return result, nil
}
