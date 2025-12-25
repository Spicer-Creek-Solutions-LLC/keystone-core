package controlplane

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
	pb "github.com/titananvil/titan-anvil/pkg/api/v1"
	"github.com/titananvil/titan-anvil/pkg/state"
)

// CommandDispatcher handles command execution and tracking
type CommandDispatcher struct {
	connMgr *ConnectionManager
	store   state.Store

	// Track in-flight commands
	mu               sync.RWMutex
	pendingCommands  map[string]*CommandExecution
	commandCallbacks map[string][]chan *pb.ExecuteCommandResponse
}

// CommandExecution tracks a command's execution state
type CommandExecution struct {
	CommandID string
	AgentID   string
	Command   *pb.ExecuteCommandRequest
	Status    pb.CommandStatus
	CreatedAt time.Time
	StartedAt *time.Time
	Results   []*pb.ExecuteCommandResponse
}

// NewCommandDispatcher creates a new command dispatcher
func NewCommandDispatcher(connMgr *ConnectionManager, store state.Store) *CommandDispatcher {
	return &CommandDispatcher{
		connMgr:          connMgr,
		store:            store,
		pendingCommands:  make(map[string]*CommandExecution),
		commandCallbacks: make(map[string][]chan *pb.ExecuteCommandResponse),
	}
}

// ExecuteCommand dispatches a command to an agent
func (cd *CommandDispatcher) ExecuteCommand(ctx context.Context, req *pb.ExecuteCommandRequest) (<-chan *pb.ExecuteCommandResponse, error) {
	// Generate command ID if not provided
	if req.CommandId == "" {
		req.CommandId = uuid.New().String()
	}

	// Validate agent exists and is online
	agent, err := cd.connMgr.GetAgent(req.AgentId)
	if err != nil {
		return nil, fmt.Errorf("agent not found: %w", err)
	}

	if agent.Status == pb.AgentStatus_AGENT_STATUS_OFFLINE {
		return nil, fmt.Errorf("agent %s is offline", req.AgentId)
	}

	// Create command record
	cmdRecord := &state.CommandRecord{
		ID:         req.CommandId,
		AgentID:    req.AgentId,
		Command:    req.Command,
		Args:       req.Args,
		Env:        req.Env,
		WorkingDir: req.WorkingDir,
		User:       req.User,
		Timeout:    req.Timeout,
		Status:     pb.CommandStatus_COMMAND_STATUS_PENDING,
		CreatedAt:  time.Now(),
	}

	// Save to database
	if err := cd.store.SaveCommand(ctx, cmdRecord); err != nil {
		return nil, fmt.Errorf("failed to save command: %w", err)
	}

	// Track execution
	exec := &CommandExecution{
		CommandID: req.CommandId,
		AgentID:   req.AgentId,
		Command:   req,
		Status:    pb.CommandStatus_COMMAND_STATUS_PENDING,
		CreatedAt: time.Now(),
		Results:   []*pb.ExecuteCommandResponse{},
	}

	cd.mu.Lock()
	cd.pendingCommands[req.CommandId] = exec
	cd.mu.Unlock()

	// Create response channel
	responseChan := make(chan *pb.ExecuteCommandResponse, 100)
	cd.mu.Lock()
	cd.commandCallbacks[req.CommandId] = append(cd.commandCallbacks[req.CommandId], responseChan)
	cd.mu.Unlock()

	// Send command to agent
	if err := cd.connMgr.SendCommand(req.AgentId, req); err != nil {
		cd.mu.Lock()
		delete(cd.pendingCommands, req.CommandId)
		delete(cd.commandCallbacks, req.CommandId)
		cd.mu.Unlock()
		close(responseChan)
		return nil, fmt.Errorf("failed to send command: %w", err)
	}

	// Update status to running
	cd.store.UpdateCommandStatus(ctx, req.CommandId, pb.CommandStatus_COMMAND_STATUS_RUNNING)

	fmt.Printf("Command %s dispatched to agent %s\n", req.CommandId, req.AgentId)

	return responseChan, nil
}

// HandleCommandResponse processes command responses from agents
func (cd *CommandDispatcher) HandleCommandResponse(resp *pb.ExecuteCommandResponse) {
	cd.mu.Lock()
	exec, exists := cd.pendingCommands[resp.CommandId]
	if !exists {
		cd.mu.Unlock()
		fmt.Printf("Received response for unknown command: %s\n", resp.CommandId)
		return
	}

	exec.Results = append(exec.Results, resp)

	// Send to all subscribers
	callbacks := cd.commandCallbacks[resp.CommandId]
	for _, ch := range callbacks {
		select {
		case ch <- resp:
		default:
			// Channel full, skip
		}
	}

	// Check if command is complete
	if resp.Type == pb.CommandResponseType_COMMAND_RESPONSE_TYPE_COMPLETED ||
		resp.Type == pb.CommandResponseType_COMMAND_RESPONSE_TYPE_FAILED ||
		resp.Type == pb.CommandResponseType_COMMAND_RESPONSE_TYPE_TIMEOUT {

		// Mark as complete
		delete(cd.pendingCommands, resp.CommandId)

		// Close all channels
		for _, ch := range callbacks {
			close(ch)
		}
		delete(cd.commandCallbacks, resp.CommandId)

		cd.mu.Unlock()

		// Update database
		status := pb.CommandStatus_COMMAND_STATUS_COMPLETED
		if resp.Type == pb.CommandResponseType_COMMAND_RESPONSE_TYPE_FAILED {
			status = pb.CommandStatus_COMMAND_STATUS_FAILED
		} else if resp.Type == pb.CommandResponseType_COMMAND_RESPONSE_TYPE_TIMEOUT {
			status = pb.CommandStatus_COMMAND_STATUS_TIMEOUT
		}

		// Collect all output
		var stdout, stderr string
		for _, r := range exec.Results {
			if r.Type == pb.CommandResponseType_COMMAND_RESPONSE_TYPE_STDOUT {
				stdout += string(r.Data)
			} else if r.Type == pb.CommandResponseType_COMMAND_RESPONSE_TYPE_STDERR {
				stderr += string(r.Data)
			}
		}

		result := &state.CommandResult{
			Status:      status,
			ExitCode:    resp.ExitCode,
			Stdout:      stdout,
			Stderr:      stderr,
			Error:       resp.Error,
			StartedAt:   exec.CreatedAt, // Approximate
			CompletedAt: time.Now(),
			DurationMs:  time.Since(exec.CreatedAt).Milliseconds(),
		}

		ctx := context.Background()
		if err := cd.store.UpdateCommandResult(ctx, resp.CommandId, result); err != nil {
			fmt.Printf("Failed to update command result: %v\n", err)
		}

		fmt.Printf("Command %s completed with status: %s\n", resp.CommandId, status)
	} else {
		cd.mu.Unlock()
	}
}

// GetCommand retrieves command status
func (cd *CommandDispatcher) GetCommand(ctx context.Context, commandID string) (*state.CommandRecord, error) {
	return cd.store.GetCommand(ctx, commandID)
}

// ListCommands lists command history
func (cd *CommandDispatcher) ListCommands(ctx context.Context, filter *state.CommandFilter) ([]*state.CommandRecord, error) {
	return cd.store.ListCommands(ctx, filter)
}
