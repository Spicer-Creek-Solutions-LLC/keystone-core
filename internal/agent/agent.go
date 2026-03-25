// Package agent implements the Keystone Core agent that runs on managed nodes.
// It handles:
//   - Registration and heartbeat with the control plane
//   - Command execution (shell, scripts)
//   - State module application
//   - System metadata collection
//   - Optional embedded NATS server for hybrid deployments
//
// The agent connects to the control plane over NATS and can operate in various
// modes including standalone client, embedded NATS host, or leaf node.
package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/nats-io/nats.go"
	"github.com/shawnbutts/keystone-core/internal/logging"
	natsmgr "github.com/shawnbutts/keystone-core/internal/nats"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"

	pb "github.com/shawnbutts/keystone-core/pkg/api/v1"
)

// Agent represents a Keystone Core agent
type Agent struct {
	id                string
	nats              *natsmgr.Manager
	executor          *Executor
	metadata          *Metadata
	heartbeatInterval time.Duration
	metadataInterval  time.Duration
	commandTimeout    time.Duration

	// Security enforcement
	security *SecurityEnforcer

	// NATS subject management
	subjects *natsmgr.SubjectBuilder
	cluster  string
	logger   logging.Logger

	ctx      context.Context
	cancel   context.CancelFunc
	wg       sync.WaitGroup
	stopOnce sync.Once

	mu           sync.RWMutex
	registered   bool
	lastMetadata time.Time
}

// Config holds configuration for an agent
type Config struct {
	// ID is the unique identifier for this agent (optional, auto-generated if empty)
	ID string
	// Cluster is the logical cluster name for subject namespacing (defaults to "default")
	Cluster string
	// HeartbeatInterval is how often to send heartbeats
	HeartbeatInterval time.Duration
	// MetadataInterval is how often to refresh metadata
	MetadataInterval time.Duration
	// CommandTimeout is the default timeout for commands
	CommandTimeout time.Duration
	// Labels are key-value pairs for agent categorization and targeting
	Labels map[string]string
	// Security configuration for authorization and command filtering
	Security *SecurityConfig
}

// NewAgent creates a new agent instance (legacy constructor)
func NewAgent(id string, natsManager *natsmgr.Manager, heartbeatInterval, metadataInterval, commandTimeout time.Duration) (*Agent, error) {
	return NewAgentWithConfig(natsManager, &Config{
		ID:                id,
		HeartbeatInterval: heartbeatInterval,
		MetadataInterval:  metadataInterval,
		CommandTimeout:    commandTimeout,
	})
}

// NewAgentWithConfig creates a new agent instance with configuration
func NewAgentWithConfig(natsManager *natsmgr.Manager, cfg *Config) (*Agent, error) {
	if cfg == nil {
		cfg = &Config{}
	}

	// Generate ID if not provided
	id := cfg.ID
	if id == "" {
		id = uuid.New().String()
	}

	// Set cluster default
	cluster := cfg.Cluster
	if cluster == "" {
		cluster = natsmgr.DefaultCluster
	}

	// Initialize security enforcer
	securityConfig := cfg.Security
	if securityConfig == nil {
		securityConfig = DefaultSecurityConfig()
	}
	security, err := NewSecurityEnforcer(securityConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize security enforcer: %w", err)
	}

	// Collect initial metadata
	metadata, err := CollectMetadata()
	if err != nil {
		return nil, fmt.Errorf("failed to collect metadata: %w", err)
	}

	// Merge labels from config into metadata
	if cfg.Labels != nil {
		for k, v := range cfg.Labels {
			metadata.Labels[k] = v
		}
	}

	ctx, cancel := context.WithCancel(context.Background())

	return &Agent{
		id:                id,
		nats:              natsManager,
		executor:          NewExecutor(),
		metadata:          metadata,
		security:          security,
		heartbeatInterval: cfg.HeartbeatInterval,
		metadataInterval:  cfg.MetadataInterval,
		commandTimeout:    cfg.CommandTimeout,
		subjects:          natsmgr.NewSubjectBuilder(cluster),
		cluster:           cluster,
		logger:            logging.WithFields(logging.Field{Key: "component", Value: "agent"}, logging.Field{Key: "agent_id", Value: id}),
		ctx:               ctx,
		cancel:            cancel,
	}, nil
}

// Cluster returns the cluster name
func (a *Agent) Cluster() string {
	return a.cluster
}

// Start starts the agent services
func (a *Agent) Start() error {
	a.logger.Info("starting agent", logging.Field{Key: "cluster", Value: a.cluster})

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

	a.logger.Info("agent started successfully")
	return nil
}

// Stop stops the agent gracefully. It is safe to call multiple times.
func (a *Agent) Stop() error {
	a.stopOnce.Do(func() {
		a.logger.Info("stopping agent")
		a.cancel()
		a.wg.Wait()
		a.logger.Info("agent stopped")
	})
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
			IpAddresses:     a.metadata.IPAddresses, // Deprecated, kept for backward compatibility
			Ipv4Addresses:   a.metadata.IPv4Addresses,
			Ipv6Addresses:   a.metadata.IPv6Addresses,
			IsDualStack:     a.metadata.IsDualStack,
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
	subject := a.subjects.AgentRegister()
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
				a.logger.Warn("failed to send heartbeat", logging.Field{Key: "error", Value: err})
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
		a.logger.Warn("failed to collect metrics", logging.Field{Key: "error", Value: err})
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
	subject := a.subjects.AgentHeartbeat()
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
		a.logger.Warn("failed to update metadata", logging.Field{Key: "error", Value: err})
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
	subject := a.subjects.AgentCommand(a.id)

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

	// Security: Extract principal and signature from NATS headers
	principal := "unknown"
	signature := ""
	if msg.Header != nil {
		if p := msg.Header.Get("X-Keystone-Principal"); p != "" {
			principal = p
		}
		signature = msg.Header.Get("X-Keystone-Signature")
	}

	// Security: Check authorization
	if err := a.security.AuthorizeCommand(principal, req.Command, signature); err != nil {
		fmt.Printf("Authorization failed for command %s: %v\n", req.CommandId, err)
		a.sendCommandError(msg, "authorization failed", err)
		return
	}

	// Security: Validate command against filters
	if err := a.security.ValidateCommand(req.Command, req.Args, req.Env, req.WorkingDir); err != nil {
		fmt.Printf("Command validation failed for %s: %v\n", req.CommandId, err)
		a.sendCommandError(msg, "command validation failed", err)
		return
	}

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
			//nolint:gosec // G115: exit codes are 0-255 on Unix, -1 to 255 on Windows, fits in int32
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
