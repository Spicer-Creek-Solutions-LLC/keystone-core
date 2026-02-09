// Package health provides health check management including HTTP endpoints,
// circuit breakers, and readiness/liveness probes.
package health

import (
	"context"
	"sync"
	"time"

	"github.com/shawnbutts/keystone-core/pkg/version"
)

// Manager manages health checks for the application
type Manager struct {
	config     *Config
	checkers   map[string]Checker
	results    map[string]CheckResult
	mu         sync.RWMutex
	startTime  time.Time
	stopCh     chan struct{}
	stoppedCh  chan struct{}
	isReady    bool
	readinessM sync.RWMutex
}

// NewManager creates a new health check manager
func NewManager(config *Config) *Manager {
	if config == nil {
		config = DefaultConfig()
	}

	return &Manager{
		config:    config,
		checkers:  make(map[string]Checker),
		results:   make(map[string]CheckResult),
		startTime: time.Now(),
		stopCh:    make(chan struct{}),
		stoppedCh: make(chan struct{}),
		isReady:   false,
	}
}

// RegisterChecker registers a health checker
func (m *Manager) RegisterChecker(checker Checker) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.checkers[checker.Name()] = checker
}

// UnregisterChecker unregisters a health checker
func (m *Manager) UnregisterChecker(name string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.checkers, name)
	delete(m.results, name)
}

// Start begins background health checking
func (m *Manager) Start(ctx context.Context) {
	// Perform initial checks
	m.runAllChecks(ctx)

	// Start background check loop
	go m.checkLoop(ctx)
}

// Stop stops background health checking
func (m *Manager) Stop() {
	close(m.stopCh)
	<-m.stoppedCh
}

// checkLoop runs health checks periodically
func (m *Manager) checkLoop(ctx context.Context) {
	defer close(m.stoppedCh)

	ticker := time.NewTicker(m.config.CheckInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-m.stopCh:
			return
		case <-ticker.C:
			m.runAllChecks(ctx)
		}
	}
}

// runAllChecks runs all registered health checks
func (m *Manager) runAllChecks(ctx context.Context) {
	m.mu.RLock()
	checkers := make([]Checker, 0, len(m.checkers))
	for _, checker := range m.checkers {
		checkers = append(checkers, checker)
	}
	m.mu.RUnlock()

	// Run checks concurrently
	var wg sync.WaitGroup
	resultsCh := make(chan CheckResult, len(checkers))

	for _, checker := range checkers {
		wg.Add(1)
		go func(c Checker) {
			defer wg.Done()

			checkCtx, cancel := context.WithTimeout(ctx, m.config.CheckTimeout)
			defer cancel()

			result := c.Check(checkCtx)
			resultsCh <- result
		}(checker)
	}

	wg.Wait()
	close(resultsCh)

	// Collect results
	m.mu.Lock()
	for result := range resultsCh {
		// Find checker name by matching result timestamp
		for name, checker := range m.checkers {
			testResult := checker.Check(ctx)
			if testResult.Timestamp.Sub(result.Timestamp).Abs() < time.Millisecond {
				m.results[name] = result
				break
			}
		}
	}
	m.mu.Unlock()

	// Update readiness status
	m.updateReadiness()
}

// updateReadiness updates the readiness status based on required checks
func (m *Manager) updateReadiness() {
	m.readinessM.Lock()
	defer m.readinessM.Unlock()

	// Check if we're past the startup grace period
	if time.Since(m.startTime) < m.config.StartupGracePeriod {
		m.isReady = false
		return
	}

	// Check all required readiness checks
	m.mu.RLock()
	defer m.mu.RUnlock()

	ready := true
	for _, checkName := range m.config.ReadinessChecks {
		result, exists := m.results[checkName]
		if !exists || result.Status == StatusUnhealthy {
			ready = false
			break
		}
	}

	m.isReady = ready
}

// IsReady returns whether the application is ready to serve traffic
func (m *Manager) IsReady() bool {
	m.readinessM.RLock()
	defer m.readinessM.RUnlock()
	return m.isReady
}

// SetReady manually sets the readiness status (for testing)
func (m *Manager) SetReady(ready bool) {
	m.readinessM.Lock()
	defer m.readinessM.Unlock()
	m.isReady = ready
}

// Liveness returns the liveness status (always healthy if process is running)
func (m *Manager) Liveness() LivenessResponse {
	return LivenessResponse{
		Status: StatusHealthy,
	}
}

// Readiness returns the readiness status
func (m *Manager) Readiness() ReadinessResponse {
	m.mu.RLock()
	results := make(map[string]CheckResult, len(m.results))
	for k, v := range m.results {
		results[k] = v
	}
	m.mu.RUnlock()

	checks := make(map[string]ComponentStatus)
	overallStatus := StatusHealthy

	// Check required readiness checks
	for _, checkName := range m.config.ReadinessChecks {
		result, exists := results[checkName]
		if !exists {
			checks[checkName] = ComponentStatus{
				Status:  StatusUnknown,
				Message: "Check not registered",
			}
			overallStatus = StatusUnhealthy
			continue
		}

		checks[checkName] = ComponentStatus{
			Status:  result.Status,
			Message: result.Message,
			Details: result.Details,
		}

		if result.Status == StatusUnhealthy {
			overallStatus = StatusUnhealthy
		} else if result.Status == StatusDegraded && overallStatus != StatusUnhealthy {
			overallStatus = StatusDegraded
		}
	}

	// If not ready, override status
	if !m.IsReady() {
		overallStatus = StatusUnhealthy
	}

	return ReadinessResponse{
		Status: overallStatus,
		Checks: checks,
	}
}

// Status returns the detailed status
func (m *Manager) Status() StatusResponse {
	m.mu.RLock()
	results := make(map[string]CheckResult, len(m.results))
	for k, v := range m.results {
		results[k] = v
	}
	m.mu.RUnlock()

	components := make(map[string]ComponentStatus)
	overallStatus := StatusHealthy

	for name, result := range results {
		components[name] = ComponentStatus{
			Status:  result.Status,
			Message: result.Message,
			Details: result.Details,
		}

		if result.Status == StatusUnhealthy {
			overallStatus = StatusUnhealthy
		} else if result.Status == StatusDegraded && overallStatus != StatusUnhealthy {
			overallStatus = StatusDegraded
		}
	}

	uptime := time.Since(m.startTime)

	return StatusResponse{
		Status:     overallStatus,
		Version:    version.Version,
		Uptime:     uptime.String(),
		StartTime:  m.startTime,
		Components: components,
	}
}

// GetCheckResult returns the latest result for a specific check
func (m *Manager) GetCheckResult(name string) (CheckResult, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	result, exists := m.results[name]
	return result, exists
}
