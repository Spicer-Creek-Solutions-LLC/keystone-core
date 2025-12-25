package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/nats-io/nats.go"
	natsmgr "github.com/titananvil/titan-anvil/pkg/nats"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"

	pb "github.com/titananvil/titan-anvil/pkg/api/v1"
)

// Agent represents a TitanAnvil agent
type Agent struct {
	id                string
	nats              *natsmgr.Manager
	executor          *Executor
	metadata          *Metadata
	heartbeatInterval time.Duration
	metadataInterval  time.Duration
	commandTimeout    time.Duration

	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup

	mu           sync.RWMutex
	registered   bool
	lastMetadata time.Time
}

// NewAgent creates a new agent instance
func NewAgent(id string, natsManager *natsmgr.Manager, heartbeatInterval, metadataInterval, commandTimeout time.Duration) (*Agent, error) {
	// Generate ID if not provided
	if id == "" {
		id = uuid.New().String()
	}

	// Collect initial metadata
	metadata, err := CollectMetadata()
	if err != nil {
		return nil, fmt.Errorf("failed to collect metadata: %w", err)
	}

	ctx, cancel := context.WithCancel(context.Background())

	return &Agent{
		id:                id,
		nats:              natsManager,
		executor:          NewExecutor(),
		metadata:          metadata,
		heartbeatInterval: heartbeatInterval,
		metadataInterval:  metadataInterval,
		commandTimeout:    commandTimeout,
		ctx:               ctx,
		cancel:            cancel,
	}, nil
}

// Start starts the agent services
func (a *Agent) Start() error {
	fmt.Printf("Starting agent %s\n", a.id)

	// Register with control plane
	if err := a.register(); err != nil {
		return fmt.Errorf("failed to register agent: %w", err)
	}

	// Subscribe to command execution requests
	if err := a.subscribeToCommands(); err != nil {
		return fmt.Errorf("failed to subscribe to commands: %w", err)
	}

	// Start heartbeat ticker
	a.wg.Add(1)
	go a.heartbeatLoop()

	// Start metadata update loop
	a.wg.Add(1)
	go a.metadataUpdateLoop()

	fmt.Printf("Agent %s started successfully\n", a.id)
	return nil
}

// Stop stops the agent gracefully
func (a *Agent) Stop() error {
	fmt.Println("Stopping agent...")
	a.cancel()
	a.wg.Wait()
	fmt.Println("Agent stopped")
	return nil
}

// ID returns the agent ID
func (a *Agent) ID() string {
	return a.id
}

// register sends registration request to control plane
func (a *Agent) register() error {
	fmt.Printf("Registering agent with control plane...\n")

	// Build registration request
	req := &pb.RegisterRequest{
		AgentId: a.id,
		Metadata: &pb.AgentMetadata{
			Hostname:        a.metadata.Hostname,
			Os:              a.metadata.OS,
			Arch:            a.metadata.Architecture,
			IpAddresses:     a.metadata.IPAddresses,
			PlatformVersion: a.metadata.PlatformVersion,
			AgentVersion:    a.metadata.AgentVersion,
			Labels:          a.metadata.Labels,
		},
	}

	// Serialize to protobuf
	data, err := proto.Marshal(req)
	if err != nil {
		return fmt.Errorf("failed to marshal registration request: %w", err)
	}

	// Send registration via NATS
	subject := "titan.agent.register"
	msg, err := a.nats.PublishRequest(subject, data, 10*time.Second)
	if err != nil {
		// If no response, we might be in development mode or control plane is not ready
		// Don't fail, just warn
		fmt.Printf("Warning: No response from control plane (may not be running): %v\n", err)
		a.mu.Lock()
		a.registered = true
		a.mu.Unlock()
		return nil
	}

	// Parse response
	var resp pb.RegisterResponse
	if err := proto.Unmarshal(msg.Data, &resp); err != nil {
		return fmt.Errorf("failed to unmarshal registration response: %w", err)
	}

	// Update configuration from response
	if resp.Config != nil {
		fmt.Printf("Received configuration from control plane\n")
		if resp.Config.HeartbeatInterval > 0 {
			a.heartbeatInterval = time.Duration(resp.Config.HeartbeatInterval) * time.Second
		}
		if resp.Config.CommandTimeout > 0 {
			a.commandTimeout = time.Duration(resp.Config.CommandTimeout) * time.Second
		}
	}

	a.mu.Lock()
	a.registered = true
	a.mu.Unlock()

	fmt.Printf("Agent registered successfully with ID: %s\n", resp.AgentId)
	return nil
}

// heartbeatLoop sends periodic heartbeats
func (a *Agent) heartbeatLoop() {
	defer a.wg.Done()

	ticker := time.NewTicker(a.heartbeatInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			if err := a.sendHeartbeat(); err != nil {
				fmt.Printf("Failed to send heartbeat: %v\n", err)
			}
		case <-a.ctx.Done():
			return
		}
	}
}

// sendHeartbeat sends a heartbeat to the control plane
func (a *Agent) sendHeartbeat() error {
	// Collect current metrics
	metrics, err := CollectMetrics()
	if err != nil {
		fmt.Printf("Warning: failed to collect metrics: %v\n", err)
		metrics = &SystemMetrics{}
	}

	// Build heartbeat request
	req := &pb.HeartbeatRequest{
		AgentId:   a.id,
		Timestamp: timestamppb.Now(),
		Status:    pb.AgentStatus_AGENT_STATUS_ONLINE,
		Metrics: &pb.SystemMetrics{
			CpuPercent:    metrics.CPUPercent,
			MemoryPercent: metrics.MemoryPercent,
			DiskPercent:   metrics.DiskPercent,
			LoadAverage:   metrics.LoadAverage,
		},
	}

	// Serialize to protobuf
	data, err := proto.Marshal(req)
	if err != nil {
		return fmt.Errorf("failed to marshal heartbeat: %w", err)
	}

	// Send via NATS (fire and forget, or request for config updates)
	subject := "titan.agent.heartbeat"
	if err := a.nats.Publish(subject, data); err != nil {
		return fmt.Errorf("failed to publish heartbeat: %w", err)
	}

	return nil
}

// metadataUpdateLoop periodically updates system metadata
func (a *Agent) metadataUpdateLoop() {
	defer a.wg.Done()

	ticker := time.NewTicker(a.metadataInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			a.updateMetadata()
		case <-a.ctx.Done():
			return
		}
	}
}

// updateMetadata refreshes system metadata
func (a *Agent) updateMetadata() {
	metadata, err := CollectMetadata()
	if err != nil {
		fmt.Printf("Warning: failed to update metadata: %v\n", err)
		return
	}

	a.mu.Lock()
	a.metadata = metadata
	a.lastMetadata = time.Now()
	a.mu.Unlock()
}

// subscribeToCommands subscribes to command execution requests
func (a *Agent) subscribeToCommands() error {
	// Subscribe to agent-specific commands
	subject := fmt.Sprintf("titan.agent.%s.command", a.id)

	_, err := a.nats.Subscribe(subject, func(msg *nats.Msg) {
		a.handleCommandRequest(msg)
	})

	if err != nil {
		return fmt.Errorf("failed to subscribe to commands: %w", err)
	}

	fmt.Printf("Subscribed to commands on subject: %s\n", subject)
	return nil
}

// handleCommandRequest processes a command execution request
func (a *Agent) handleCommandRequest(msg *nats.Msg) {
	// Parse command request
	var req pb.ExecuteCommandRequest
	if err := proto.Unmarshal(msg.Data, &req); err != nil {
		fmt.Printf("Failed to unmarshal command request: %v\n", err)
		a.sendCommandError(msg, "failed to unmarshal request", err)
		return
	}

	fmt.Printf("Received command: %s %v (ID: %s)\n", req.Command, req.Args, req.CommandId)

	// Execute command
	execReq := &ExecuteCommandRequest{
		CommandID:  req.CommandId,
		Command:    req.Command,
		Args:       req.Args,
		Env:        req.Env,
		WorkingDir: req.WorkingDir,
		Timeout:    time.Duration(req.Timeout) * time.Second,
		User:       req.User,
	}

	// Stream output back
	outputHandler := func(commandID string, isStderr bool, data []byte) {
		respType := pb.CommandResponseType_COMMAND_RESPONSE_TYPE_STDOUT
		if isStderr {
			respType = pb.CommandResponseType_COMMAND_RESPONSE_TYPE_STDERR
		}

		resp := &pb.ExecuteCommandResponse{
			CommandId: commandID,
			Type:      respType,
			Data:      data,
			Timestamp: timestamppb.Now(),
		}

		a.sendCommandResponse(msg, resp)
	}

	// Execute the command
	result, err := a.executor.Execute(a.ctx, execReq, outputHandler)

	// Send completion response
	if err != nil {
		resp := &pb.ExecuteCommandResponse{
			CommandId: req.CommandId,
			Type:      pb.CommandResponseType_COMMAND_RESPONSE_TYPE_FAILED,
			Error:     err.Error(),
			Timestamp: timestamppb.Now(),
		}
		a.sendCommandResponse(msg, resp)
	} else {
		respType := pb.CommandResponseType_COMMAND_RESPONSE_TYPE_COMPLETED
		if result.Error != nil {
			respType = pb.CommandResponseType_COMMAND_RESPONSE_TYPE_FAILED
		}

		resp := &pb.ExecuteCommandResponse{
			CommandId: req.CommandId,
			Type:      respType,
			ExitCode:  int32(result.ExitCode),
			Timestamp: timestamppb.Now(),
		}

		if result.Error != nil {
			resp.Error = result.Error.Error()
		}

		a.sendCommandResponse(msg, resp)
	}
}

// sendCommandResponse sends a command response back
func (a *Agent) sendCommandResponse(originalMsg *nats.Msg, resp *pb.ExecuteCommandResponse) {
	data, err := proto.Marshal(resp)
	if err != nil {
		fmt.Printf("Failed to marshal command response: %v\n", err)
		return
	}

	// Reply to the original message if it has a reply subject
	if originalMsg.Reply != "" {
		if err := a.nats.Publish(originalMsg.Reply, data); err != nil {
			fmt.Printf("Failed to send command response: %v\n", err)
		}
	}
}

// sendCommandError sends an error response
func (a *Agent) sendCommandError(originalMsg *nats.Msg, message string, err error) {
	errorResp := map[string]string{
		"error":   message,
		"details": err.Error(),
	}

	data, _ := json.Marshal(errorResp)

	if originalMsg.Reply != "" {
		a.nats.Publish(originalMsg.Reply, data)
	}
}
