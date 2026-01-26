package targeting

import (
	"context"
	"fmt"
	"sync"
	"time"

	pb "github.com/shawnbutts/keystone-core/pkg/api/v1"
)

// AgentInfo represents information about an agent for targeting purposes
type AgentInfo struct {
	ID       string
	Status   pb.AgentStatus
	Metadata *pb.AgentMetadata
}

// BatchResult represents the result of executing a command on a single agent
type BatchResult struct {
	AgentID   string
	CommandID string
	Success   bool
	ExitCode  int32
	Error     error
	Output    []byte // Combined stdout/stderr
	StartTime time.Time
	EndTime   time.Time
	Duration  time.Duration
}

// BatchExecution represents a batch command execution across multiple agents
type BatchExecution struct {
	commandID string
	command   string
	args      []string
	env       map[string]string
	workingDir string
	user       string
	timeout    int32

	matcher    *Matcher
	dispatcher CommandDispatcher

	concurrency int
	results     []*BatchResult
	mu          sync.RWMutex

	// Progress tracking
	total     int
	completed int
	failed    int
}

// CommandDispatcher is an interface for dispatching commands to agents
type CommandDispatcher interface {
	ExecuteCommand(ctx context.Context, req *pb.ExecuteCommandRequest) (<-chan *pb.ExecuteCommandResponse, error)
}

// BatchExecutor executes commands across multiple agents in parallel
type BatchExecutor struct {
	dispatcher   CommandDispatcher
	connManager  ConnectionManager
	concurrency  int
	defaultTimeout int32
}

// ConnectionManager is an interface for getting agent information
type ConnectionManager interface {
	ListAgents() []*AgentInfo
	GetAgent(id string) (*AgentInfo, error)
}

// NewBatchExecutor creates a new batch executor
func NewBatchExecutor(dispatcher CommandDispatcher, connManager ConnectionManager) *BatchExecutor {
	return &BatchExecutor{
		dispatcher:     dispatcher,
		connManager:    connManager,
		concurrency:    10, // Default to 10 parallel executions
		defaultTimeout: 300, // Default 5 minute timeout
	}
}

// SetConcurrency sets the maximum number of parallel executions
func (be *BatchExecutor) SetConcurrency(n int) {
	if n < 1 {
		n = 1
	}
	be.concurrency = n
}

// SetDefaultTimeout sets the default command timeout in seconds
func (be *BatchExecutor) SetDefaultTimeout(seconds int32) {
	be.defaultTimeout = seconds
}

// Execute executes a command across agents matching the target expression
func (be *BatchExecutor) Execute(ctx context.Context, targetExpr, command string, args []string) (*BatchExecution, error) {
	return be.ExecuteWithOptions(ctx, &ExecuteOptions{
		Target:  targetExpr,
		Command: command,
		Args:    args,
	})
}

// ExecuteOptions contains options for batch execution
type ExecuteOptions struct {
	Target     string
	Command    string
	Args       []string
	Env        map[string]string
	WorkingDir string
	User       string
	Timeout    int32
}

// ExecuteWithOptions executes a command with full options
func (be *BatchExecutor) ExecuteWithOptions(ctx context.Context, opts *ExecuteOptions) (*BatchExecution, error) {
	// Parse target expression
	matcher, err := NewMatcher(opts.Target)
	if err != nil {
		return nil, fmt.Errorf("failed to parse target expression: %w", err)
	}

	// Get all agents
	allAgents := be.connManager.ListAgents()

	// Filter agents based on target expression
	matchedAgents, err := matcher.Match(allAgents)
	if err != nil {
		return nil, fmt.Errorf("failed to match agents: %w", err)
	}

	if len(matchedAgents) == 0 {
		return nil, fmt.Errorf("no agents matched target expression: %s", opts.Target)
	}

	// Set timeout if not provided
	timeout := opts.Timeout
	if timeout == 0 {
		timeout = be.defaultTimeout
	}

	// Create batch execution
	batch := &BatchExecution{
		command:     opts.Command,
		args:        opts.Args,
		env:         opts.Env,
		workingDir:  opts.WorkingDir,
		user:        opts.User,
		timeout:     timeout,
		matcher:     matcher,
		dispatcher:  be.dispatcher,
		concurrency: be.concurrency,
		total:       len(matchedAgents),
		results:     make([]*BatchResult, 0, len(matchedAgents)),
	}

	// Execute commands in parallel with concurrency control
	batch.executeParallel(ctx, matchedAgents)

	return batch, nil
}

// executeParallel executes commands in parallel with concurrency control
func (batch *BatchExecution) executeParallel(ctx context.Context, agents []*AgentInfo) {
	// Create semaphore for concurrency control
	sem := make(chan struct{}, batch.concurrency)
	var wg sync.WaitGroup

	for _, agent := range agents {
		// Skip offline agents
		if agent.Status == pb.AgentStatus_AGENT_STATUS_OFFLINE {
			batch.mu.Lock()
			batch.failed++
			batch.completed++
			batch.results = append(batch.results, &BatchResult{
				AgentID: agent.ID,
				Success: false,
				Error:   fmt.Errorf("agent is offline"),
			})
			batch.mu.Unlock()
			continue
		}

		wg.Add(1)
		go func(ag *AgentInfo) {
			defer wg.Done()

			// Acquire semaphore
			sem <- struct{}{}
			defer func() { <-sem }()

			// Execute command on this agent
			result := batch.executeOnAgent(ctx, ag)

			// Store result
			batch.mu.Lock()
			batch.results = append(batch.results, result)
			batch.completed++
			if !result.Success {
				batch.failed++
			}
			batch.mu.Unlock()
		}(agent)
	}

	wg.Wait()
}

// executeOnAgent executes the command on a single agent
func (batch *BatchExecution) executeOnAgent(ctx context.Context, agent *AgentInfo) *BatchResult {
	result := &BatchResult{
		AgentID:   agent.ID,
		StartTime: time.Now(),
	}

	// Create command request
	req := &pb.ExecuteCommandRequest{
		AgentId:    agent.ID,
		Command:    batch.command,
		Args:       batch.args,
		Env:        batch.env,
		WorkingDir: batch.workingDir,
		Timeout:    batch.timeout,
		User:       batch.user,
	}

	// Execute command
	responseChan, err := batch.dispatcher.ExecuteCommand(ctx, req)
	if err != nil {
		result.EndTime = time.Now()
		result.Duration = result.EndTime.Sub(result.StartTime)
		result.Success = false
		result.Error = err
		return result
	}

	result.CommandID = req.CommandId

	// Collect output
	var output []byte
	for response := range responseChan {
		switch response.Type {
		case pb.CommandResponseType_COMMAND_RESPONSE_TYPE_STDOUT,
			pb.CommandResponseType_COMMAND_RESPONSE_TYPE_STDERR:
			output = append(output, response.Data...)

		case pb.CommandResponseType_COMMAND_RESPONSE_TYPE_COMPLETED:
			result.Success = true
			result.ExitCode = response.ExitCode
			result.EndTime = time.Now()
			result.Duration = result.EndTime.Sub(result.StartTime)

		case pb.CommandResponseType_COMMAND_RESPONSE_TYPE_FAILED,
			pb.CommandResponseType_COMMAND_RESPONSE_TYPE_TIMEOUT:
			result.Success = false
			result.ExitCode = response.ExitCode
			result.Error = fmt.Errorf("%s", response.Error)
			result.EndTime = time.Now()
			result.Duration = result.EndTime.Sub(result.StartTime)
		}
	}

	result.Output = output
	return result
}

// Results returns all batch results
func (batch *BatchExecution) Results() []*BatchResult {
	batch.mu.RLock()
	defer batch.mu.RUnlock()

	// Return a copy to prevent concurrent modification
	results := make([]*BatchResult, len(batch.results))
	copy(results, batch.results)
	return results
}

// Progress returns execution progress statistics
func (batch *BatchExecution) Progress() (total, completed, failed int) {
	batch.mu.RLock()
	defer batch.mu.RUnlock()
	return batch.total, batch.completed, batch.failed
}

// IsComplete returns true if all executions have completed
func (batch *BatchExecution) IsComplete() bool {
	batch.mu.RLock()
	defer batch.mu.RUnlock()
	return batch.completed == batch.total
}

// SuccessRate returns the percentage of successful executions
func (batch *BatchExecution) SuccessRate() float64 {
	batch.mu.RLock()
	defer batch.mu.RUnlock()

	if batch.total == 0 {
		return 0.0
	}

	successful := batch.completed - batch.failed
	return float64(successful) / float64(batch.total) * 100.0
}
