package controlplane

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/nats-io/nats.go"
	natsmgr "github.com/shawnbutts/keystone-core/pkg/nats"
	"github.com/shawnbutts/keystone-core/pkg/events"
	"github.com/shawnbutts/keystone-core/pkg/tracing"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"

	pb "github.com/shawnbutts/keystone-core/pkg/api/v1"
)

// AgentInfo represents information about a connected agent
type AgentInfo struct {
	ID              string
	Metadata        *pb.AgentMetadata
	Status          pb.AgentStatus
	LastHeartbeat   time.Time
	RegisteredAt    time.Time
	LastMetrics     *pb.SystemMetrics
	HeartbeatMissed int
}

// ConnectionManager manages agent connections and state
type ConnectionManager struct {
	nats   *natsmgr.Manager
	agents map[string]*AgentInfo
	mu     sync.RWMutex

	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup

	// Configuration
	heartbeatTimeout time.Duration
	staleThreshold   int

	// Event publishing
	eventPublisher events.EventPublisher
}

// NewConnectionManager creates a new connection manager
func NewConnectionManager(natsManager *natsmgr.Manager) *ConnectionManager {
	ctx, cancel := context.WithCancel(context.Background())

	return &ConnectionManager{
		nats:             natsManager,
		agents:           make(map[string]*AgentInfo),
		ctx:              ctx,
		cancel:           cancel,
		heartbeatTimeout: 60 * time.Second, // Consider agent stale after 60s
		staleThreshold:   3,                // Mark offline after 3 missed heartbeats
	}
}

// Start starts the connection manager
func (cm *ConnectionManager) Start() error {
	fmt.Println("Starting connection manager...")

	// Subscribe to agent registration requests
	if err := cm.subscribeToRegistrations(); err != nil {
		return fmt.Errorf("failed to subscribe to registrations: %w", err)
	}

	// Subscribe to agent heartbeats
	if err := cm.subscribeToHeartbeats(); err != nil {
		return fmt.Errorf("failed to subscribe to heartbeats: %w", err)
	}

	// Start agent monitoring loop
	cm.wg.Add(1)
	go cm.monitorAgents()

	fmt.Println("Connection manager started")
	return nil
}

// Stop stops the connection manager
func (cm *ConnectionManager) Stop() error {
	fmt.Println("Stopping connection manager...")
	cm.cancel()
	cm.wg.Wait()
	fmt.Println("Connection manager stopped")
	return nil
}

// subscribeToRegistrations subscribes to agent registration requests
func (cm *ConnectionManager) subscribeToRegistrations() error {
	subject := "titan.agent.register"

	_, err := cm.nats.Subscribe(subject, func(msg *nats.Msg) {
		cm.handleRegistration(msg)
	})

	if err != nil {
		return fmt.Errorf("failed to subscribe to %s: %w", subject, err)
	}

	fmt.Printf("Subscribed to agent registrations on %s\n", subject)
	return nil
}

// subscribeToHeartbeats subscribes to agent heartbeats
func (cm *ConnectionManager) subscribeToHeartbeats() error {
	subject := "titan.agent.heartbeat"

	_, err := cm.nats.Subscribe(subject, func(msg *nats.Msg) {
		cm.handleHeartbeat(msg)
	})

	if err != nil {
		return fmt.Errorf("failed to subscribe to %s: %w", subject, err)
	}

	fmt.Printf("Subscribed to agent heartbeats on %s\n", subject)
	return nil
}

// handleRegistration processes agent registration requests
func (cm *ConnectionManager) handleRegistration(msg *nats.Msg) {
	// Extract trace context from NATS message
	ctx := tracing.ExtractTraceContext(cm.ctx, msg)
	ctx, span := tracing.StartControlPlaneSpan(ctx, tracing.SpanAgentConnect)
	defer span.End()

	// Parse registration request
	var req pb.RegisterRequest
	if err := proto.Unmarshal(msg.Data, &req); err != nil {
		fmt.Printf("Failed to unmarshal registration request: %v\n", err)
		tracing.RecordError(span, err)
		return
	}

	// Add span attributes
	tracing.SetAttributes(span,
		tracing.StringAttr(tracing.AttrAgentID, req.AgentId),
		tracing.StringAttr(tracing.AttrAgentHostname, req.Metadata.GetHostname()),
		tracing.StringAttr(tracing.AttrAgentOS, req.Metadata.GetOs()),
	)

	fmt.Printf("Agent registration request: ID=%s, Hostname=%s\n", req.AgentId, req.Metadata.Hostname)

	// Register the agent
	cm.mu.Lock()
	info, exists := cm.agents[req.AgentId]
	if !exists {
		info = &AgentInfo{
			ID:           req.AgentId,
			RegisteredAt: time.Now(),
		}
		cm.agents[req.AgentId] = info
		fmt.Printf("New agent registered: %s\n", req.AgentId)
	} else {
		fmt.Printf("Agent re-registered: %s\n", req.AgentId)
	}

	info.Metadata = req.Metadata
	info.Status = pb.AgentStatus_AGENT_STATUS_ONLINE
	info.LastHeartbeat = time.Now()
	info.HeartbeatMissed = 0
	cm.mu.Unlock()

	// Emit agent.connect event
	if !exists {
		cm.emitEvent(events.EventTypeAgentConnect, events.SeverityInfo, map[string]interface{}{
			"agent_id":         req.AgentId,
			"hostname":         req.Metadata.Hostname,
			"os":               req.Metadata.Os,
			"arch":             req.Metadata.Arch,
			"ip_addresses":     req.Metadata.IpAddresses,
			"platform_version": req.Metadata.PlatformVersion,
		})
	}

	// Send registration response
	resp := &pb.RegisterResponse{
		AgentId:      req.AgentId,
		RegisteredAt: timestamppb.New(info.RegisteredAt),
		Config: &pb.AgentConfig{
			HeartbeatInterval: 30, // 30 seconds
			CommandTimeout:    300, // 5 minutes
			MetadataInterval:  300, // 5 minutes
		},
	}

	data, err := proto.Marshal(resp)
	if err != nil {
		fmt.Printf("Failed to marshal registration response: %v\n", err)
		return
	}

	// Reply to registration request
	if msg.Reply != "" {
		// Create reply message with trace context
		replyMsg := &nats.Msg{
			Subject: msg.Reply,
			Data:    data,
			Header:  nats.Header{},
		}
		tracing.InjectTraceContext(ctx, replyMsg)

		if err := cm.nats.Conn().PublishMsg(replyMsg); err != nil {
			fmt.Printf("Failed to send registration response: %v\n", err)
			tracing.RecordError(span, err)
		} else {
			tracing.RecordSuccess(span, "agent registered successfully")
		}
	}
}

// handleHeartbeat processes agent heartbeat messages
func (cm *ConnectionManager) handleHeartbeat(msg *nats.Msg) {
	// Extract trace context from NATS message
	ctx := tracing.ExtractTraceContext(cm.ctx, msg)
	ctx, span := tracing.StartControlPlaneSpan(ctx, tracing.SpanAgentHeartbeat)
	defer span.End()

	// Parse heartbeat request
	var req pb.HeartbeatRequest
	if err := proto.Unmarshal(msg.Data, &req); err != nil {
		fmt.Printf("Failed to unmarshal heartbeat: %v\n", err)
		tracing.RecordError(span, err)
		return
	}

	// Add span attributes
	tracing.SetAttributes(span,
		tracing.StringAttr(tracing.AttrAgentID, req.AgentId),
		tracing.StringAttr("agent.status", req.Status.String()),
	)

	cm.mu.Lock()
	info, exists := cm.agents[req.AgentId]
	if !exists {
		cm.mu.Unlock()
		fmt.Printf("Heartbeat from unknown agent: %s\n", req.AgentId)
		tracing.AddEvent(span, "unknown_agent")
		return
	}

	// Update agent info
	info.LastHeartbeat = time.Now()
	info.Status = req.Status
	info.LastMetrics = req.Metrics
	info.HeartbeatMissed = 0
	cm.mu.Unlock()

	tracing.RecordSuccess(span, "heartbeat processed")
}

// monitorAgents periodically checks agent health
func (cm *ConnectionManager) monitorAgents() {
	defer cm.wg.Done()

	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			cm.checkAgentHealth()
		case <-cm.ctx.Done():
			return
		}
	}
}

// checkAgentHealth checks if agents are still alive
func (cm *ConnectionManager) checkAgentHealth() {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	now := time.Now()
	for id, info := range cm.agents {
		timeSinceHeartbeat := now.Sub(info.LastHeartbeat)

		if timeSinceHeartbeat > cm.heartbeatTimeout {
			info.HeartbeatMissed++

			if info.HeartbeatMissed >= cm.staleThreshold {
				if info.Status != pb.AgentStatus_AGENT_STATUS_OFFLINE {
					fmt.Printf("Agent %s marked as OFFLINE (no heartbeat for %s)\n", id, timeSinceHeartbeat)
					previousStatus := info.Status
					info.Status = pb.AgentStatus_AGENT_STATUS_OFFLINE

					// Emit agent.disconnect event
					cm.emitEvent(events.EventTypeAgentDisconnect, events.SeverityWarning, map[string]interface{}{
						"agent_id":             id,
						"hostname":             info.Metadata.GetHostname(),
						"previous_status":      previousStatus.String(),
						"heartbeat_missed":     info.HeartbeatMissed,
						"time_since_heartbeat": timeSinceHeartbeat.Seconds(),
					})
				}
			} else if info.Status != pb.AgentStatus_AGENT_STATUS_DEGRADED {
				fmt.Printf("Agent %s marked as DEGRADED (heartbeat delayed)\n", id)
				info.Status = pb.AgentStatus_AGENT_STATUS_DEGRADED
			}
		}
	}
}

// GetAgent returns information about a specific agent
func (cm *ConnectionManager) GetAgent(agentID string) (*AgentInfo, error) {
	cm.mu.RLock()
	defer cm.mu.RUnlock()

	info, exists := cm.agents[agentID]
	if !exists {
		return nil, fmt.Errorf("agent %s not found", agentID)
	}

	// Return a copy
	infoCopy := *info
	return &infoCopy, nil
}

// ListAgents returns all registered agents
func (cm *ConnectionManager) ListAgents() []*AgentInfo {
	cm.mu.RLock()
	defer cm.mu.RUnlock()

	agents := make([]*AgentInfo, 0, len(cm.agents))
	for _, info := range cm.agents {
		infoCopy := *info
		agents = append(agents, &infoCopy)
	}

	return agents
}

// GetAgentCount returns the number of registered agents
func (cm *ConnectionManager) GetAgentCount() int {
	cm.mu.RLock()
	defer cm.mu.RUnlock()
	return len(cm.agents)
}

// GetOnlineAgentCount returns the number of online agents
func (cm *ConnectionManager) GetOnlineAgentCount() int {
	cm.mu.RLock()
	defer cm.mu.RUnlock()

	count := 0
	for _, info := range cm.agents {
		if info.Status == pb.AgentStatus_AGENT_STATUS_ONLINE {
			count++
		}
	}
	return count
}

// SendCommand sends a command to a specific agent
func (cm *ConnectionManager) SendCommand(agentID string, command *pb.ExecuteCommandRequest) error {
	return cm.SendCommandWithContext(cm.ctx, agentID, command)
}

// SendCommandWithContext sends a command to a specific agent with context
func (cm *ConnectionManager) SendCommandWithContext(ctx context.Context, agentID string, command *pb.ExecuteCommandRequest) error {
	ctx, span := tracing.StartControlPlaneSpan(ctx, tracing.SpanCommandDispatch,
		tracing.StringAttr(tracing.AttrAgentID, agentID),
		tracing.StringAttr(tracing.AttrJobID, command.CommandId),
		tracing.StringAttr(tracing.AttrJobCommand, command.Command),
	)
	defer span.End()

	// Check if agent exists and is online
	info, err := cm.GetAgent(agentID)
	if err != nil {
		tracing.RecordError(span, err)
		return err
	}

	if info.Status == pb.AgentStatus_AGENT_STATUS_OFFLINE {
		err := fmt.Errorf("agent %s is offline", agentID)
		tracing.RecordError(span, err)
		return err
	}

	tracing.SetAttributes(span,
		tracing.StringAttr(tracing.AttrAgentHostname, info.Metadata.GetHostname()),
		tracing.StringAttr("agent.status", info.Status.String()),
	)

	// Serialize command
	data, err := proto.Marshal(command)
	if err != nil {
		err = fmt.Errorf("failed to marshal command: %w", err)
		tracing.RecordError(span, err)
		return err
	}

	// Send command to agent-specific subject with trace context
	subject := fmt.Sprintf("titan.agent.%s.command", agentID)
	msg := &nats.Msg{
		Subject: subject,
		Data:    data,
		Header:  nats.Header{},
	}
	tracing.InjectTraceContext(ctx, msg)

	if err := cm.nats.Conn().PublishMsg(msg); err != nil {
		err = fmt.Errorf("failed to send command: %w", err)
		tracing.RecordError(span, err)
		return err
	}

	fmt.Printf("Command %s sent to agent %s\n", command.CommandId, agentID)
	tracing.RecordSuccess(span, "command dispatched to agent")
	return nil
}

// SetEventPublisher sets the event publisher for emitting events
func (cm *ConnectionManager) SetEventPublisher(publisher events.EventPublisher) {
	cm.mu.Lock()
	defer cm.mu.Unlock()
	cm.eventPublisher = publisher
}

// emitEvent emits an event if EventPublisher is configured
func (cm *ConnectionManager) emitEvent(eventType events.EventType, severity events.Severity, data map[string]interface{}) {
	if cm.eventPublisher == nil {
		return
	}

	// Extract correlation ID from data if present (use agent_id as correlation for agent events)
	correlationID := ""
	if aid, ok := data["agent_id"].(string); ok {
		correlationID = "agent-" + aid
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
	if err := cm.eventPublisher.PublishAsync(event); err != nil {
		fmt.Printf("Warning: failed to emit event %s: %v\n", eventType, err)
	}
}
