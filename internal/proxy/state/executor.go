// Package state provides proxy-aware state execution for proxied devices.
package state

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/shawnbutts/keystone-core/internal/proxy"
	"github.com/shawnbutts/keystone-core/internal/statemgmt"
)

// ProxyStateExecutor executes state declarations on proxied devices.
type ProxyStateExecutor struct {
	// proxyAgent is the proxy agent managing devices
	proxyAgent *proxy.Manager

	// modules is the registry of proxy state modules
	modules *ProxyModuleRegistry

	// config holds executor configuration
	config ExecutorConfig

	// stats tracks execution statistics
	stats ExecutorStats
	statsMu sync.RWMutex

	// eventEmitter emits state events
	eventEmitter EventEmitter
}

// ExecutorConfig configures the proxy state executor.
type ExecutorConfig struct {
	// Timeout for individual state operations
	Timeout time.Duration

	// MaxConcurrent is the maximum concurrent state applications
	MaxConcurrent int

	// RetryCount is the number of retries for failed operations
	RetryCount int

	// RetryDelay is the delay between retries
	RetryDelay time.Duration

	// DryRun mode checks without applying
	DryRun bool

	// FailFast stops on first error
	FailFast bool
}

// DefaultExecutorConfig returns default executor configuration.
func DefaultExecutorConfig() ExecutorConfig {
	return ExecutorConfig{
		Timeout:       5 * time.Minute,
		MaxConcurrent: 10,
		RetryCount:    3,
		RetryDelay:    5 * time.Second,
		DryRun:        false,
		FailFast:      false,
	}
}

// ExecutorStats tracks execution statistics.
type ExecutorStats struct {
	TotalExecutions    int64
	SuccessfulExecutions int64
	FailedExecutions   int64
	TotalStates        int64
	AppliedStates      int64
	FailedStates       int64
	SkippedStates      int64
	TotalDuration      time.Duration
	LastExecution      time.Time
}

// EventEmitter emits state execution events.
type EventEmitter interface {
	Emit(event StateEvent)
}

// StateEvent represents a state execution event.
type StateEvent struct {
	Type        StateEventType
	DeviceID    string
	StateName   string
	ModuleType  string
	Result      StateResult
	Timestamp   time.Time
	Duration    time.Duration
	Error       error
}

// StateEventType represents the type of state event.
type StateEventType string

const (
	EventStateStart    StateEventType = "state.start"
	EventStateComplete StateEventType = "state.complete"
	EventStateFailed   StateEventType = "state.failed"
	EventStateSkipped  StateEventType = "state.skipped"
	EventStateDrift    StateEventType = "state.drift"
)

// StateResult represents the result of a state operation.
type StateResult string

const (
	ResultSuccess   StateResult = "success"
	ResultFailed    StateResult = "failed"
	ResultSkipped   StateResult = "skipped"
	ResultNoChange  StateResult = "no_change"
	ResultChanged   StateResult = "changed"
)

// ProxyStateRun represents a state run on a proxied device.
type ProxyStateRun struct {
	// RunID is a unique identifier for this run
	RunID string

	// DeviceID is the target device
	DeviceID string

	// States are the state declarations to apply
	States []ProxyStateDeclaration

	// StartTime is when the run started
	StartTime time.Time

	// EndTime is when the run ended
	EndTime time.Time

	// Duration is the total run duration
	Duration time.Duration

	// Results are the results for each state
	Results []ProxyStateResult

	// Summary summarizes the run
	Summary ProxyRunSummary

	// Error is the overall run error, if any
	Error error
}

// ProxyStateDeclaration represents a state declaration for a proxied device.
type ProxyStateDeclaration struct {
	// ID is the state identifier
	ID string

	// Name is the human-readable name
	Name string

	// Module is the state module type
	Module string

	// Parameters are the module parameters
	Parameters map[string]interface{}

	// Requisites are dependencies on other states
	Requisites []ProxyRequisite

	// Conditions are execution conditions
	Conditions ProxyConditions
}

// ProxyRequisite represents a state dependency.
type ProxyRequisite struct {
	Type  string // require, watch, prereq
	State string // state ID
}

// ProxyConditions represents execution conditions.
type ProxyConditions struct {
	OnlyIf string // condition command that must succeed
	Unless string // condition command that must fail
}

// ProxyStateResult represents the result of applying a single state.
type ProxyStateResult struct {
	// StateID is the state identifier
	StateID string

	// StateName is the human-readable name
	StateName string

	// Module is the module type
	Module string

	// Result is the outcome
	Result StateResult

	// Changed indicates if changes were made
	Changed bool

	// Comment describes what happened
	Comment string

	// Duration is how long it took
	Duration time.Duration

	// Error is the error, if any
	Error error

	// Details contains module-specific details
	Details map[string]interface{}
}

// ProxyRunSummary summarizes a state run.
type ProxyRunSummary struct {
	TotalStates   int
	Succeeded     int
	Failed        int
	Changed       int
	Unchanged     int
	Skipped       int
	Duration      time.Duration
}

// NewProxyStateExecutor creates a new proxy state executor.
func NewProxyStateExecutor(proxyAgent *proxy.Manager, config ExecutorConfig) (*ProxyStateExecutor, error) {
	if proxyAgent == nil {
		return nil, fmt.Errorf("proxy agent is required")
	}

	executor := &ProxyStateExecutor{
		proxyAgent: proxyAgent,
		modules:    NewProxyModuleRegistry(),
		config:     config,
	}

	// Register default modules
	executor.registerDefaultModules()

	return executor, nil
}

// registerDefaultModules registers the default proxy state modules.
func (e *ProxyStateExecutor) registerDefaultModules() {
	// SSH modules
	e.modules.Register("ssh_file", NewSSHFileModule())
	e.modules.Register("ssh_cmd", NewSSHCmdModule())
	e.modules.Register("ssh_service", NewSSHServiceModule())
	e.modules.Register("ssh_package", NewSSHPackageModule())
	e.modules.Register("ssh_user", NewSSHUserModule())
	e.modules.Register("ssh_group", NewSSHGroupModule())

	// SNMP modules
	e.modules.Register("snmp_value", NewSNMPValueModule())
	e.modules.Register("snmp_table", NewSNMPTableModule())

	// REST modules
	e.modules.Register("http_config", NewHTTPConfigModule())
	e.modules.Register("http_resource", NewHTTPResourceModule())

	// Network device modules
	e.modules.Register("ios_config", NewIOSConfigModule())
	e.modules.Register("nxos_config", NewNXOSConfigModule())
	e.modules.Register("junos_config", NewJUNOSConfigModule())
	e.modules.Register("eos_config", NewEOSConfigModule())
	e.modules.Register("vyos_config", NewVyOSConfigModule())
	e.modules.Register("pfsense_config", NewPfSenseConfigModule())
	e.modules.Register("opnsense_config", NewOPNsenseConfigModule())

	// WinRM modules
	e.modules.Register("winrm_file", NewWinRMFileModule())
	e.modules.Register("winrm_service", NewWinRMServiceModule())
	e.modules.Register("winrm_registry", NewWinRMRegistryModule())
	e.modules.Register("winrm_package", NewWinRMPackageModule())
}

// SetEventEmitter sets the event emitter.
func (e *ProxyStateExecutor) SetEventEmitter(emitter EventEmitter) {
	e.eventEmitter = emitter
}

// Execute runs states on a proxied device.
func (e *ProxyStateExecutor) Execute(ctx context.Context, deviceID string, states []ProxyStateDeclaration) (*ProxyStateRun, error) {
	run := &ProxyStateRun{
		RunID:     generateRunID(),
		DeviceID:  deviceID,
		States:    states,
		StartTime: time.Now(),
		Results:   make([]ProxyStateResult, 0, len(states)),
	}

	// Get the device
	device, err := e.proxyAgent.Registry().Get(ctx, deviceID)
	if err != nil {
		run.Error = fmt.Errorf("device not found: %w", err)
		return run, run.Error
	}

	// Get executor
	executor := e.proxyAgent.Executor()
	if executor == nil {
		run.Error = fmt.Errorf("no executor available")
		return run, run.Error
	}

	// Order states by dependencies
	orderedStates, err := e.orderStates(states)
	if err != nil {
		run.Error = fmt.Errorf("failed to order states: %w", err)
		return run, run.Error
	}

	// Execute each state
	for _, state := range orderedStates {
		result := e.executeState(ctx, device, executor, state)
		run.Results = append(run.Results, result)

		// Emit event
		if e.eventEmitter != nil {
			eventType := EventStateComplete
			if result.Error != nil {
				eventType = EventStateFailed
			} else if result.Result == ResultSkipped {
				eventType = EventStateSkipped
			}

			e.eventEmitter.Emit(StateEvent{
				Type:       eventType,
				DeviceID:   deviceID,
				StateName:  state.Name,
				ModuleType: state.Module,
				Result:     result.Result,
				Timestamp:  time.Now(),
				Duration:   result.Duration,
				Error:      result.Error,
			})
		}

		// Check fail fast
		if e.config.FailFast && result.Error != nil {
			run.Error = fmt.Errorf("state '%s' failed: %w", state.Name, result.Error)
			break
		}
	}

	// Calculate summary
	run.EndTime = time.Now()
	run.Duration = run.EndTime.Sub(run.StartTime)
	run.Summary = e.calculateSummary(run.Results)

	// Update stats
	e.updateStats(run)

	return run, nil
}

// executeState executes a single state on a device.
func (e *ProxyStateExecutor) executeState(ctx context.Context, device *proxy.ProxiedDevice, executor proxy.ProxiedExecutor, state ProxyStateDeclaration) ProxyStateResult {
	startTime := time.Now()

	result := ProxyStateResult{
		StateID:   state.ID,
		StateName: state.Name,
		Module:    state.Module,
	}

	// Get the module
	module, err := e.modules.Get(state.Module)
	if err != nil {
		result.Result = ResultFailed
		result.Error = fmt.Errorf("unknown module '%s': %w", state.Module, err)
		result.Duration = time.Since(startTime)
		return result
	}

	// Check conditions
	if state.Conditions.OnlyIf != "" {
		if !e.checkCondition(ctx, executor, device.ID, state.Conditions.OnlyIf) {
			result.Result = ResultSkipped
			result.Comment = "only_if condition not met"
			result.Duration = time.Since(startTime)
			return result
		}
	}

	if state.Conditions.Unless != "" {
		if e.checkCondition(ctx, executor, device.ID, state.Conditions.Unless) {
			result.Result = ResultSkipped
			result.Comment = "unless condition met"
			result.Duration = time.Since(startTime)
			return result
		}
	}

	// Create timeout context
	execCtx, cancel := context.WithTimeout(ctx, e.config.Timeout)
	defer cancel()

	// Execute module
	moduleResult, err := module.Execute(execCtx, ModuleContext{
		Device:     device,
		Executor:   executor,
		Parameters: state.Parameters,
		DryRun:     e.config.DryRun,
	})

	result.Duration = time.Since(startTime)

	if err != nil {
		result.Result = ResultFailed
		result.Error = err
		return result
	}

	result.Changed = moduleResult.Changed
	result.Comment = moduleResult.Comment
	result.Details = moduleResult.Details

	if result.Changed {
		result.Result = ResultChanged
	} else {
		result.Result = ResultNoChange
	}

	return result
}

// checkCondition checks if a condition is met.
func (e *ProxyStateExecutor) checkCondition(ctx context.Context, executor proxy.ProxiedExecutor, deviceID string, condition string) bool {
	result, err := executor.Execute(ctx, &proxy.ProxiedExecuteRequest{
		DeviceID: deviceID,
		Command:  condition,
	})
	if err != nil {
		return false
	}
	return result.ExitCode == 0
}

// orderStates orders states by dependencies using topological sort.
func (e *ProxyStateExecutor) orderStates(states []ProxyStateDeclaration) ([]ProxyStateDeclaration, error) {
	// Build dependency graph
	graph := make(map[string][]string)
	stateMap := make(map[string]ProxyStateDeclaration)

	for _, state := range states {
		stateMap[state.ID] = state
		graph[state.ID] = []string{}
		for _, req := range state.Requisites {
			if req.Type == "require" || req.Type == "prereq" {
				graph[state.ID] = append(graph[state.ID], req.State)
			}
		}
	}

	// Topological sort using Kahn's algorithm
	inDegree := make(map[string]int)
	for id := range graph {
		inDegree[id] = 0
	}
	for _, deps := range graph {
		for _, dep := range deps {
			inDegree[dep]++
		}
	}

	// Find nodes with no dependencies
	queue := make([]string, 0)
	for id, degree := range inDegree {
		if degree == 0 {
			queue = append(queue, id)
		}
	}

	ordered := make([]ProxyStateDeclaration, 0, len(states))
	for len(queue) > 0 {
		id := queue[0]
		queue = queue[1:]

		if state, ok := stateMap[id]; ok {
			ordered = append(ordered, state)
		}

		for _, dep := range graph[id] {
			inDegree[dep]--
			if inDegree[dep] == 0 {
				queue = append(queue, dep)
			}
		}
	}

	// Check for cycles
	if len(ordered) != len(states) {
		return nil, fmt.Errorf("circular dependency detected in states")
	}

	return ordered, nil
}

// calculateSummary calculates the run summary.
func (e *ProxyStateExecutor) calculateSummary(results []ProxyStateResult) ProxyRunSummary {
	summary := ProxyRunSummary{
		TotalStates: len(results),
	}

	for _, result := range results {
		summary.Duration += result.Duration

		switch result.Result {
		case ResultSuccess, ResultNoChange:
			summary.Succeeded++
			summary.Unchanged++
		case ResultChanged:
			summary.Succeeded++
			summary.Changed++
		case ResultFailed:
			summary.Failed++
		case ResultSkipped:
			summary.Skipped++
		}
	}

	return summary
}

// updateStats updates executor statistics.
func (e *ProxyStateExecutor) updateStats(run *ProxyStateRun) {
	e.statsMu.Lock()
	defer e.statsMu.Unlock()

	e.stats.TotalExecutions++
	if run.Error == nil && run.Summary.Failed == 0 {
		e.stats.SuccessfulExecutions++
	} else {
		e.stats.FailedExecutions++
	}

	e.stats.TotalStates += int64(run.Summary.TotalStates)
	e.stats.AppliedStates += int64(run.Summary.Succeeded)
	e.stats.FailedStates += int64(run.Summary.Failed)
	e.stats.SkippedStates += int64(run.Summary.Skipped)
	e.stats.TotalDuration += run.Duration
	e.stats.LastExecution = run.EndTime
}

// GetStats returns executor statistics.
func (e *ProxyStateExecutor) GetStats() ExecutorStats {
	e.statsMu.RLock()
	defer e.statsMu.RUnlock()
	return e.stats
}

// generateRunID generates a unique run ID.
func generateRunID() string {
	return fmt.Sprintf("run-%d", time.Now().UnixNano())
}

// ExecuteFile executes states from a state file on a proxied device.
func (e *ProxyStateExecutor) ExecuteFile(ctx context.Context, deviceID string, stateFile *statemgmt.StateFile) (*ProxyStateRun, error) {
	// Convert state file to proxy state declarations
	// stateFile.States is map[string][]StateDeclaration
	var states []ProxyStateDeclaration

	for _, decls := range stateFile.States {
		for _, decl := range decls {
			proxyDecl := ProxyStateDeclaration{
				ID:         decl.ID,
				Name:       decl.ID, // Use ID as Name since StateDeclaration doesn't have a Name field
				Module:     decl.Module,
				Parameters: decl.Parameters,
			}

			// Convert requisites
			for _, req := range decl.Requisites.Require {
				proxyDecl.Requisites = append(proxyDecl.Requisites, ProxyRequisite{
					Type:  "require",
					State: req.ID,
				})
			}

			// Convert conditions
			if onlyIf, ok := decl.Parameters["only_if"].(string); ok {
				proxyDecl.Conditions.OnlyIf = onlyIf
			}
			if unless, ok := decl.Parameters["unless"].(string); ok {
				proxyDecl.Conditions.Unless = unless
			}

			states = append(states, proxyDecl)
		}
	}

	return e.Execute(ctx, deviceID, states)
}

// Check performs a dry-run check of states on a device.
func (e *ProxyStateExecutor) Check(ctx context.Context, deviceID string, states []ProxyStateDeclaration) (*ProxyStateRun, error) {
	// Create a copy of config with DryRun enabled
	origDryRun := e.config.DryRun
	e.config.DryRun = true
	defer func() {
		e.config.DryRun = origDryRun
	}()

	return e.Execute(ctx, deviceID, states)
}

// ExecuteBatch executes states on multiple devices.
func (e *ProxyStateExecutor) ExecuteBatch(ctx context.Context, deviceIDs []string, states []ProxyStateDeclaration) (map[string]*ProxyStateRun, error) {
	results := make(map[string]*ProxyStateRun)
	resultsMu := sync.Mutex{}

	// Use semaphore for concurrency control
	sem := make(chan struct{}, e.config.MaxConcurrent)
	var wg sync.WaitGroup

	for _, deviceID := range deviceIDs {
		wg.Add(1)
		go func(id string) {
			defer wg.Done()

			sem <- struct{}{}
			defer func() { <-sem }()

			run, err := e.Execute(ctx, id, states)
			if err != nil {
				run = &ProxyStateRun{
					DeviceID: id,
					Error:    err,
				}
			}

			resultsMu.Lock()
			results[id] = run
			resultsMu.Unlock()
		}(deviceID)
	}

	wg.Wait()
	return results, nil
}
