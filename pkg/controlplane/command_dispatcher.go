package controlplane

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/nats-io/nats.go"
	pb "github.com/shawnbutts/keystone-core/pkg/api/v1"
	"github.com/shawnbutts/keystone-core/pkg/events"
	"github.com/shawnbutts/keystone-core/pkg/state"
	"github.com/shawnbutts/keystone-core/pkg/tracing"
	"google.golang.org/protobuf/proto"
)

// CommandDispatcher handles command execution and tracking
type CommandDispatcher struct {
	connMgr *ConnectionManager
	store   state.Store

	// Track in-flight commands
	mu               sync.RWMutex
	pendingCommands  map[string]*CommandExecution
	commandCallbacks map[string][]chan *pb.ExecuteCommandResponse

	// Event publishing
	eventPublisher events.EventPublisher

	// NATS subscription for command responses
	responseSub *nats.Subscription
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

// Start starts the command dispatcher and subscribes to command responses
func (cd *CommandDispatcher) Start() error {
	// Subscribe to command responses from agents
	// Subject pattern: kscore.controlplane.command.*.response (wildcard for command ID)
	sub, err := cd.connMgr.nats.Conn().Subscribe("kscore.controlplane.command.*.response", func(msg *nats.Msg) {
		var resp pb.ExecuteCommandResponse
		if err := proto.Unmarshal(msg.Data, &resp); err != nil {
			fmt.Printf("Failed to unmarshal command response: %v\n", err)
			return
		}
		cd.HandleCommandResponse(&resp)
	})
	if err != nil {
		return fmt.Errorf("failed to subscribe to command responses: %w", err)
	}
	cd.responseSub = sub
	fmt.Println("Command dispatcher started, subscribed to command responses")
	return nil
}

// Stop stops the command dispatcher and unsubscribes from responses
func (cd *CommandDispatcher) Stop() error {
	if cd.responseSub != nil {
		if err := cd.responseSub.Unsubscribe(); err != nil {
			return fmt.Errorf("failed to unsubscribe from responses: %w", err)
		}
		cd.responseSub = nil
	}
	return nil
}

// ExecuteCommand dispatches a command to an agent
func (cd *CommandDispatcher) ExecuteCommand(ctx context.Context, req *pb.ExecuteCommandRequest) (<-chan *pb.ExecuteCommandResponse, error) {
	// Generate command ID if not provided
	if req.CommandId == "" {
		req.CommandId = uuid.New().String()
	}

	// Start tracing span
	ctx, span := tracing.StartControlPlaneSpan(ctx, tracing.SpanCommandExecute,
		tracing.StringAttr(tracing.AttrAgentID, req.AgentId),
		tracing.StringAttr(tracing.AttrJobID, req.CommandId),
		tracing.StringAttr(tracing.AttrJobCommand, req.Command),
	)
	defer span.End()

	// Add additional attributes
	if len(req.Args) > 0 {
		tracing.SetAttributes(span, tracing.StringSliceAttr("command.args", req.Args))
	}
	if req.WorkingDir != "" {
		tracing.SetAttributes(span, tracing.StringAttr("command.working_dir", req.WorkingDir))
	}
	if req.Timeout > 0 {
		tracing.SetAttributes(span, tracing.Int64Attr("command.timeout", int64(req.Timeout)))
	}

	// Validate agent exists and is online
	agent, err := cd.connMgr.GetAgent(req.AgentId)
	if err != nil {
		tracing.RecordError(span, err)
		return nil, fmt.Errorf("agent not found: %w", err)
	}

	if agent.Status == pb.AgentStatus_AGENT_STATUS_OFFLINE {
		err := fmt.Errorf("agent %s is offline", req.AgentId)
		tracing.RecordError(span, err)
		return nil, err
	}

	tracing.SetAttributes(span,
		tracing.StringAttr(tracing.AttrAgentHostname, agent.Metadata.GetHostname()),
		tracing.StringAttr("agent.status", agent.Status.String()),
	)

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
		err = fmt.Errorf("failed to save command: %w", err)
		tracing.RecordError(span, err)
		return nil, err
	}
	tracing.AddEvent(span, "command_saved_to_db")

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

	// Send command to agent (pass context for tracing)
	if err := cd.connMgr.SendCommandWithContext(ctx, req.AgentId, req); err != nil {
		cd.mu.Lock()
		delete(cd.pendingCommands, req.CommandId)
		delete(cd.commandCallbacks, req.CommandId)
		cd.mu.Unlock()
		close(responseChan)
		tracing.RecordError(span, err)
		return nil, fmt.Errorf("failed to send command: %w", err)
	}
	tracing.AddEvent(span, "command_sent_to_agent")

	// Update status to running
	cd.store.UpdateCommandStatus(ctx, req.CommandId, pb.CommandStatus_COMMAND_STATUS_RUNNING)
	tracing.RecordSuccess(span, "command dispatched successfully")

	// Emit job.start event
	cd.emitEvent(events.EventTypeJobStart, events.SeverityInfo, map[string]interface{}{
		"job_id":      req.CommandId,
		"agent_id":    req.AgentId,
		"command":     req.Command,
		"args":        req.Args,
		"working_dir": req.WorkingDir,
		"user":        req.User,
		"timeout":     req.Timeout,
	})

	fmt.Printf("Command %s dispatched to agent %s\n", req.CommandId, req.AgentId)

	return responseChan, nil
}

// HandleCommandResponse processes command responses from agents
func (cd *CommandDispatcher) HandleCommandResponse(resp *pb.ExecuteCommandResponse) {
	cd.HandleCommandResponseWithContext(context.Background(), resp)
}

// HandleCommandResponseWithContext processes command responses from agents with context
func (cd *CommandDispatcher) HandleCommandResponseWithContext(ctx context.Context, resp *pb.ExecuteCommandResponse) {
	ctx, span := tracing.StartControlPlaneSpan(ctx, "command.response",
		tracing.StringAttr(tracing.AttrJobID, resp.CommandId),
		tracing.StringAttr("response.type", resp.Type.String()),
	)
	defer span.End()

	cd.mu.Lock()
	exec, exists := cd.pendingCommands[resp.CommandId]
	if !exists {
		cd.mu.Unlock()
		fmt.Printf("Received response for unknown command: %s\n", resp.CommandId)
		tracing.AddEvent(span, "unknown_command")
		return
	}

	tracing.SetAttributes(span,
		tracing.StringAttr(tracing.AttrAgentID, exec.AgentID),
	)

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

		tracing.AddEvent(span, "command_completed")

		// Update database
		status := pb.CommandStatus_COMMAND_STATUS_COMPLETED
		if resp.Type == pb.CommandResponseType_COMMAND_RESPONSE_TYPE_FAILED {
			status = pb.CommandStatus_COMMAND_STATUS_FAILED
		} else if resp.Type == pb.CommandResponseType_COMMAND_RESPONSE_TYPE_TIMEOUT {
			status = pb.CommandStatus_COMMAND_STATUS_TIMEOUT
		}

		tracing.SetAttributes(span,
			tracing.StringAttr(tracing.AttrJobStatus, status.String()),
			tracing.IntAttr(tracing.AttrJobExitCode, int(resp.ExitCode)),
		)

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

		tracing.SetAttributes(span,
			tracing.Int64Attr(tracing.AttrJobDuration, result.DurationMs),
		)

		if err := cd.store.UpdateCommandResult(ctx, resp.CommandId, result); err != nil {
			fmt.Printf("Failed to update command result: %v\n", err)
			tracing.RecordError(span, err)
		}

		// Emit job completion or failure event
		if status == pb.CommandStatus_COMMAND_STATUS_COMPLETED {
			cd.emitEvent(events.EventTypeJobComplete, events.SeverityInfo, map[string]interface{}{
				"job_id":      resp.CommandId,
				"agent_id":    exec.AgentID,
				"exit_code":   resp.ExitCode,
				"duration_ms": result.DurationMs,
				"stdout_len":  len(stdout),
				"stderr_len":  len(stderr),
			})
			tracing.RecordSuccess(span, "command completed successfully")
		} else {
			severity := events.SeverityError
			if status == pb.CommandStatus_COMMAND_STATUS_TIMEOUT {
				severity = events.SeverityWarning
			}

			cd.emitEvent(events.EventTypeJobFail, severity, map[string]interface{}{
				"job_id":      resp.CommandId,
				"agent_id":    exec.AgentID,
				"exit_code":   resp.ExitCode,
				"error":       resp.Error,
				"status":      status.String(),
				"duration_ms": result.DurationMs,
			})

			// Record error for failed/timeout commands
			if status == pb.CommandStatus_COMMAND_STATUS_FAILED {
				if resp.Error != "" {
					tracing.RecordErrorWithMessage(span, fmt.Errorf("command failed"), resp.Error)
				} else {
					tracing.RecordErrorWithMessage(span, fmt.Errorf("command failed"), fmt.Sprintf("exit code %d", resp.ExitCode))
				}
			} else if status == pb.CommandStatus_COMMAND_STATUS_TIMEOUT {
				tracing.RecordErrorWithMessage(span, fmt.Errorf("command timeout"), "command exceeded timeout")
			}
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

// SetEventPublisher sets the event publisher for emitting events
func (cd *CommandDispatcher) SetEventPublisher(publisher events.EventPublisher) {
	cd.mu.Lock()
	defer cd.mu.Unlock()
	cd.eventPublisher = publisher
}

// emitEvent emits an event if EventPublisher is configured
func (cd *CommandDispatcher) emitEvent(eventType events.EventType, severity events.Severity, data map[string]interface{}) {
	if cd.eventPublisher == nil {
		return
	}

	// Extract correlation ID from data if present (use job_id as correlation)
	correlationID := ""
	if jid, ok := data["job_id"].(string); ok {
		correlationID = jid
	}

	eventBuilder := events.NewEvent(eventType).
		Source("/control-plane").
		Severity(severity).
		DataMap(data)

	// Set correlation ID if available
	if correlationID != "" {
		eventBuilder = eventBuilder.CorrelationID(correlationID)
	}

	event := eventBuilder.Build()

	// Use async publish to avoid blocking
	if err := cd.eventPublisher.PublishAsync(event); err != nil {
		fmt.Printf("Warning: failed to emit event %s: %v\n", eventType, err)
	}
}
